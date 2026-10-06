package api

import (
	"encoding/json"
	"fmt"
	"io"
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
	r.Use(compactGinLogger(), gin.Recovery())
	r.GET("/static/*filepath", gin.WrapH(buildStaticHandler(staticFS)))
	r.GET("/pages/*filepath", gin.WrapH(buildPagesHandler(staticFS)))
	r.GET("/manifest.webmanifest", handleWebManifest(staticFS))
	r.GET("/sw.js", handleServiceWorker(staticFS))

	ctrl := &setupController{state: state, restart: restart, staticFS: staticFS}
	r.GET("/", ctrl.handleSetupPage())
	r.GET("/login", handleLoginPage(staticFS))
	r.GET("/register", handleRegisterPage(staticFS))
	r.GET("/api/setup/state", ctrl.handleGetSetupState())
	r.POST("/api/setup/init", ctrl.handleInitialize())
	r.POST("/api/setup/import", ctrl.handleImportTransfer())
	r.POST("/api/setup/select-library", ctrl.handleSelectLibrary())
	r.POST("/api/auth/login", ctrl.handleSetupLogin())
	r.POST("/api/auth/register", ctrl.handleSetupRegister())
	r.POST("/api/auth/logout", handleLogout(&config.Config{Port: state.Port}, RouterOptions{}))

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

func (s *setupController) handleImportTransfer() gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil || file.Size <= 0 || file.Size > 64<<20 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择 64 MB 以内的 EchoGallery 配置包"})
			return
		}
		input, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer input.Close()
		data, err := io.ReadAll(io.LimitReader(input, (64<<20)+1))
		if err != nil || len(data) > 64<<20 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取配置包失败"})
			return
		}
		var target *config.Config
		if loaded, loadErr := config.Load(); loadErr == nil {
			target = loaded
		} else {
			appDataDir, pathErr := config.DefaultAppDataDir()
			if pathErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": pathErr.Error()})
				return
			}
			target = &config.Config{
				Port: s.state.Port, AppDataDir: appDataDir,
				ThumbnailDir: filepath.Join(appDataDir, "thumbnails"),
				TrashDir:     filepath.Join(appDataDir, "Trash"),
			}
		}
		manifest, _, inspectErr := transferZipFiles(data)
		if inspectErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": inspectErr.Error()})
			return
		}
		if err := importConfigTransferBundle(target, data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "配置已导入，应用正在重启", "browser_settings": manifest.BrowserSettings,
		})
		if s.restart != nil {
			go func() {
				time.Sleep(250 * time.Millisecond)
				_ = s.restart()
			}()
		}
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
		setAuthCookie(c.Writer, cfg, token)
		if !profileHasVisibleLibraries(cfg, user.Username, profile) {
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
				"delay_ms": 2400,
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
			if err := os.MkdirAll(config.LibraryDataRoot(library.Path), 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建资源库数据目录失败: %v", err)})
				return
			}
			if err := os.MkdirAll(filepath.Join(config.LibraryDataRoot(library.Path), ".trash"), 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建资源库回收站目录失败: %v", err)})
				return
			}
		}
		for _, library := range cfg.Libraries {
			thumbDir, err := cfg.ThumbnailStoragePathForStorage(library.Path)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("计算缩略图目录失败: %v", err)})
				return
			}
			if err := os.MkdirAll(thumbDir, 0755); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("创建缩略图目录失败: %v", err)})
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
				libraries = append(libraries, config.Library{
					ID:          library.ID,
					Name:        name,
					Path:        path,
					LogoAsset:   library.LogoAsset,
					AccentColor: library.AccentColor,
				})
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
				cfg.ActiveLibraryID = library.ID
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
	profile.ActiveLibraryID = ""
	for _, library := range cfg.VisibleLibrariesForUser(username) {
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
