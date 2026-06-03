package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type SetupMode string

const (
	SetupModeInit            SetupMode = "init"
	SetupModeLibraryRecovery SetupMode = "library_recovery"
)

type SetupState struct {
	Mode               SetupMode        `json:"mode"`
	Message            string           `json:"message"`
	Libraries          []config.Library `json:"libraries"`
	CurrentStoragePath string           `json:"current_storage_path"`
	Port               int              `json:"port"`
}

type setupController struct {
	state    SetupState
	restart  func() error
	staticFS fs.FS
}

type setupInitRequest struct {
	Port          int              `json:"port"`
	Libraries     []config.Library `json:"libraries"`
	StoragePath   string           `json:"storage_path"`
	ThumbnailDir  string           `json:"thumbnail_dir"`
	ThumbnailSize int              `json:"thumbnail_size"`
	TrashDir      string           `json:"trash_dir"`
	Username      string           `json:"username"`
	Password      string           `json:"password"`
}

type setupSelectLibraryRequest struct {
	Name        string           `json:"name"`
	StoragePath string           `json:"storage_path"`
	Selected    int              `json:"selected_index"`
	Libraries   []config.Library `json:"libraries"`
}

func NewSetupRouterWithStatic(staticFS fs.FS, state SetupState, restart func() error) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/static/*filepath", gin.WrapH(buildStaticHandler(staticFS)))
	r.GET("/pages/*filepath", gin.WrapH(buildPagesHandler(staticFS)))

	ctrl := &setupController{state: state, restart: restart, staticFS: staticFS}
	r.GET("/", ctrl.handleSetupPage())
	r.GET("/login", handleLoginPage(staticFS))
	r.GET("/register", handleRegisterPage(staticFS))
	r.GET("/api/setup/state", ctrl.handleGetSetupState())
	r.POST("/api/setup/init", ctrl.handleInitialize())
	r.POST("/api/setup/select-library", ctrl.handleSelectLibrary())
	r.POST("/api/auth/login", ctrl.handleSetupLogin())
	r.POST("/api/auth/register", ctrl.handleSetupRegister())
	r.POST("/api/auth/logout", handleLogout())

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		c.Redirect(http.StatusSeeOther, "/")
	})
	return r
}

func (s *setupController) loadConfigForAuth(c *gin.Context) (*config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前尚未完成初始化，无法切换用户"})
		return nil, false
	}
	return cfg, true
}

func (s *setupController) handleGetSetupState() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.state)
	}
}

func (s *setupController) handleSetupLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := s.loadConfigForAuth(c)
		if !ok {
			return
		}
		var req authLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		user, err := config.VerifyPassword(&config.Config{Users: cfg.Users}, req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}
		profile, err := config.EnsureProfile(cfg, user.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("加载用户配置失败: %v", err)})
			return
		}
		cfg.ActiveProfile = user.Username
		cfg.ApplyProfile(profile)
		if err := cfg.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("切换用户失败: %v", err)})
			return
		}
		token, err := generateAuthToken(cfg.JWTSecret, user.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
			return
		}
		http.SetCookie(c.Writer, &http.Cookie{Name: authCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(7 * 24 * time.Hour)})
		if strings.TrimSpace(profile.StoragePath) == "" && len(profile.Libraries) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"message":  "已切换到该用户，请继续完成初始化。",
				"redirect": "/",
				"delay_ms": 120,
			})
			return
		}
		if s.restart != nil {
			if err := s.restart(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":  "登录成功，正在切换到该用户的资源库。",
				"redirect": "/",
				"delay_ms": 1400,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":  "登录成功",
			"redirect": "/",
			"delay_ms": 0,
		})
	}
}

func (s *setupController) handleSetupRegister() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := s.loadConfigForAuth(c)
		if !ok {
			return
		}
		handleRegister(cfg)(c)
	}
}

func (s *setupController) handleInitialize() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.state.Mode != SetupModeInit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前不是初始化模式"})
			return
		}
		var req setupInitRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}
		cfg, err := config.CreateInitialConfig(config.InitialConfigParams{
			Port:          req.Port,
			Libraries:     req.Libraries,
			StoragePath:   req.StoragePath,
			ThumbnailDir:  req.ThumbnailDir,
			ThumbnailSize: req.ThumbnailSize,
			TrashDir:      req.TrashDir,
			Username:      req.Username,
			Password:      req.Password,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for _, library := range cfg.Libraries {
			if err := os.MkdirAll(library.Path, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建资源库目录失败: %v", err)})
				return
			}
		}
		if thumbDir, err := cfg.ThumbnailStoragePath(); err == nil {
			if err := os.MkdirAll(thumbDir, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建缩略图目录失败: %v", err)})
				return
			}
		}
		if trashDir, err := cfg.TrashPath(); err == nil {
			if err := os.MkdirAll(trashDir, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建回收站目录失败: %v", err)})
				return
			}
		}
		if err := cfg.Save(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if s.restart != nil {
			if err := s.restart(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "初始化完成，服务正在重启"})
	}
}

func (s *setupController) handleSelectLibrary() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.state.Mode != SetupModeLibraryRecovery {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前不是资源库恢复模式"})
			return
		}
		var req setupSelectLibraryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}
		cfg, err := config.Load()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		selected := strings.TrimSpace(req.StoragePath)
		if len(req.Libraries) > 0 {
			libraries := make([]config.Library, 0, len(req.Libraries))
			for i, library := range req.Libraries {
				path := strings.TrimSpace(library.Path)
				if path == "" {
					continue
				}
				name := strings.TrimSpace(library.Name)
				if name == "" {
					name = fmt.Sprintf("资源库 %d", i+1)
				}
				libraries = append(libraries, config.Library{Name: name, Path: path, LogoAsset: library.LogoAsset, AccentColor: library.AccentColor})
			}
			cfg.Libraries = libraries
			if selected == "" && req.Selected >= 0 && req.Selected < len(req.Libraries) {
				selected = strings.TrimSpace(req.Libraries[req.Selected].Path)
			}
		}
		if selected == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "资源库路径不能为空"})
			return
		}
		found := false
		for _, library := range cfg.Libraries {
			if config.NormalizeStoragePath(library.Path) == config.NormalizeStoragePath(selected) {
				cfg.StoragePath = library.Path
				found = true
				break
			}
		}
		if !found {
			name := strings.TrimSpace(req.Name)
			if name == "" {
				name = fmt.Sprintf("资源库 %d", len(cfg.Libraries)+1)
			}
			if err := os.MkdirAll(selected, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建资源库目录失败: %v", err)})
				return
			}
			cfg.Libraries = append(cfg.Libraries, config.Library{Name: name, Path: selected})
			cfg.StoragePath = selected
		}
		if err := persistRecoveredLibrarySelection(cfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := cfg.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if s.restart != nil {
			if err := s.restart(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "资源库已切换，服务正在重启"})
	}
}

func persistRecoveredLibrarySelection(cfg *config.Config) error {
	username := setupRecoveryProfileUsername(cfg)
	if username == "" {
		return nil
	}
	profile, err := config.EnsureProfile(cfg, username)
	if err != nil {
		return fmt.Errorf("加载 Profile 失败: %w", err)
	}
	profile.StoragePath = cfg.StoragePath
	profile.Libraries = append([]config.Library(nil), cfg.Libraries...)
	profile.ActiveLibraryID = ""
	for _, library := range profile.Libraries {
		if config.NormalizeStoragePath(library.Path) == config.NormalizeStoragePath(profile.StoragePath) {
			profile.ActiveLibraryID = library.ID
			break
		}
	}
	if err := config.SaveProfile(cfg, username, profile); err != nil {
		return err
	}
	cfg.ActiveProfile = username
	cfg.ApplyProfile(profile)
	return nil
}

func setupRecoveryProfileUsername(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if active := strings.TrimSpace(cfg.ActiveProfile); active != "" {
		return active
	}
	for _, user := range cfg.Users {
		if username := strings.TrimSpace(user.Username); username != "" {
			return username
		}
	}
	return ""
}

func (s *setupController) handleSetupPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, _ := json.Marshal(s.state)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(buildSetupPageHTML(s.staticFS, string(payload))))
	}
}

func buildSetupPageHTML(staticFS fs.FS, payload string) string {
	html := pageHTML(staticFS, "setup.html", setupPageFallbackHTML, []string{"__SETUP_PAYLOAD__", "setup-root"})
	return strings.Replace(html, "__SETUP_PAYLOAD__", payload, 1)
}

func validateStartupLibrary(cfg *config.Config) error {
	info, err := os.Stat(cfg.StoragePath)
	if err != nil {
		return fmt.Errorf("当前资源库不可用: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("当前资源库不是目录: %s", filepath.Clean(cfg.StoragePath))
	}
	return nil
}
