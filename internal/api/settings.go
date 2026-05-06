package api

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	imgpkg "echogallery/internal/image"
	"echogallery/internal/service"
)

type settingsResponse struct {
	Port                          int               `json:"port"`
	StoragePath                   string            `json:"storage_path"`
	Libraries                     []libraryResponse `json:"libraries"`
	ThumbnailDir                  string            `json:"thumbnail_dir"`
	ThumbnailSize                 int               `json:"thumbnail_size"`
	TrashDir                      string            `json:"trash_dir"`
	UseSystemPlayer               bool              `json:"use_system_player"`
	JWTSecretMasked               string            `json:"jwt_secret_masked"`
	Users                         []string          `json:"users"`
	Theme                         string            `json:"theme"`
	GridSize                      int               `json:"grid_size"`
	GridGap                       int               `json:"grid_gap"`
	ThumbRadius                   int               `json:"thumb_radius"`
	SidebarAutoHide               bool              `json:"sidebar_auto_hide"`
	SlideshowMode                 string            `json:"slideshow_mode"`
	SlideshowLoop                 bool              `json:"slideshow_loop"`
	SlideshowInterval             int               `json:"slideshow_interval"`
	LightboxZoom                  int               `json:"lightbox_zoom"`
	ExperimentalAutoplayVideo     bool              `json:"experimental_autoplay_video"`
	ExperimentalPrefetchNeighbors bool              `json:"experimental_prefetch_neighbors"`
	ExperimentalRestoreLastView   bool              `json:"experimental_restore_last_view"`
	ContinueLastVideoPosition     bool              `json:"continue_last_video_position"`
	PlayerKeymap                  string            `json:"player_keymap"`
}

type settingsUpdateRequest struct {
	Port                          int              `json:"port"`
	StoragePath                   string           `json:"storage_path"`
	Libraries                     []config.Library `json:"libraries"`
	ThumbnailDir                  string           `json:"thumbnail_dir"`
	ThumbnailSize                 int              `json:"thumbnail_size"`
	TrashDir                      string           `json:"trash_dir"`
	UseSystemPlayer               bool             `json:"use_system_player"`
	Theme                         string           `json:"theme"`
	GridSize                      int              `json:"grid_size"`
	GridGap                       int              `json:"grid_gap"`
	ThumbRadius                   int              `json:"thumb_radius"`
	SidebarAutoHide               bool             `json:"sidebar_auto_hide"`
	SlideshowMode                 string           `json:"slideshow_mode"`
	SlideshowLoop                 bool             `json:"slideshow_loop"`
	SlideshowInterval             int              `json:"slideshow_interval"`
	LightboxZoom                  int              `json:"lightbox_zoom"`
	ExperimentalAutoplayVideo     bool             `json:"experimental_autoplay_video"`
	ExperimentalPrefetchNeighbors bool             `json:"experimental_prefetch_neighbors"`
	ExperimentalRestoreLastView   bool             `json:"experimental_restore_last_view"`
	ContinueLastVideoPosition     bool             `json:"continue_last_video_position"`
	PlayerKeymap                  string           `json:"player_keymap"`
}

type refreshLibraryLogosRequest struct {
	ReplaceExisting bool `json:"replace_existing"`
}

type refreshLibraryLogosResult struct {
	Scanned int      `json:"scanned"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

type libraryResponse struct {
	Index        int    `json:"index"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	LogoAsset    string `json:"logo_asset,omitempty"`
	LogoImageURL string `json:"logo_image_url,omitempty"`
	AccentColor  string `json:"accent_color,omitempty"`
}

func buildSettingsResponse(cfg *config.Config) settingsResponse {
	users := make([]string, 0, len(cfg.Users))
	for _, user := range cfg.Users {
		users = append(users, user.Username)
	}
	masked := "未设置"
	if cfg.JWTSecret != "" {
		if len(cfg.JWTSecret) <= 8 {
			masked = "********"
		} else {
			masked = cfg.JWTSecret[:4] + "..." + cfg.JWTSecret[len(cfg.JWTSecret)-4:]
		}
	}
	return settingsResponse{
		Port:                          cfg.Port,
		StoragePath:                   cfg.StoragePath,
		Libraries:                     buildLibraryResponses(cfg),
		ThumbnailDir:                  cfg.ThumbnailDir,
		ThumbnailSize:                 cfg.ThumbnailSize,
		TrashDir:                      cfg.TrashDir,
		UseSystemPlayer:               cfg.UseSystemPlayer,
		JWTSecretMasked:               masked,
		Users:                         users,
		Theme:                         cfg.Preferences.Theme,
		GridSize:                      cfg.Preferences.GridSize,
		GridGap:                       cfg.Preferences.GridGap,
		ThumbRadius:                   cfg.Preferences.ThumbRadius,
		SidebarAutoHide:               cfg.Preferences.SidebarAutoHide,
		SlideshowMode:                 cfg.Preferences.SlideshowMode,
		SlideshowLoop:                 cfg.Preferences.SlideshowLoop,
		SlideshowInterval:             cfg.Preferences.SlideshowInterval,
		LightboxZoom:                  cfg.Preferences.LightboxZoom,
		ExperimentalAutoplayVideo:     cfg.Preferences.ExperimentalAutoplayVideo,
		ExperimentalPrefetchNeighbors: cfg.Preferences.ExperimentalPrefetchNeighbors,
		ExperimentalRestoreLastView:   cfg.Preferences.ExperimentalRestoreLastView,
		ContinueLastVideoPosition:     cfg.Preferences.ContinueLastVideoPosition,
		PlayerKeymap:                  cfg.Preferences.PlayerKeymap,
	}
}

func buildLibraryResponses(cfg *config.Config) []libraryResponse {
	resp := make([]libraryResponse, 0, len(cfg.Libraries))
	for index, library := range cfg.Libraries {
		item := libraryResponse{
			Index:       index,
			Name:        library.Name,
			Path:        library.Path,
			LogoAsset:   library.LogoAsset,
			AccentColor: library.AccentColor,
		}
		if library.LogoAsset != "" {
			item.LogoImageURL = fmt.Sprintf("/api/settings/libraries/%d/logo?v=%s", index, library.LogoAsset)
		}
		resp = append(resp, item)
	}
	return resp
}

func handleGetSettings(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, buildSettingsResponse(cfg))
	}
}

func handleUpdateSettings(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}

		prevThumbnailDir := strings.TrimSpace(cfg.ThumbnailDir)
		prevThumbnailSize := cfg.ThumbnailSize
		next := *cfg
		next.Port = req.Port
		next.StoragePath = req.StoragePath
		next.Libraries = append([]config.Library(nil), req.Libraries...)
		next.ThumbnailDir = req.ThumbnailDir
		next.ThumbnailSize = req.ThumbnailSize
		next.TrashDir = req.TrashDir
		next.UseSystemPlayer = req.UseSystemPlayer
		next.Preferences.Theme = req.Theme
		next.Preferences.GridSize = req.GridSize
		next.Preferences.GridGap = req.GridGap
		next.Preferences.ThumbRadius = req.ThumbRadius
		next.Preferences.SidebarAutoHide = req.SidebarAutoHide
		next.Preferences.SlideshowMode = req.SlideshowMode
		next.Preferences.SlideshowLoop = req.SlideshowLoop
		next.Preferences.SlideshowInterval = req.SlideshowInterval
		next.Preferences.LightboxZoom = req.LightboxZoom
		next.Preferences.ExperimentalAutoplayVideo = req.ExperimentalAutoplayVideo
		next.Preferences.ExperimentalPrefetchNeighbors = req.ExperimentalPrefetchNeighbors
		next.Preferences.ExperimentalRestoreLastView = req.ExperimentalRestoreLastView
		next.Preferences.ContinueLastVideoPosition = req.ContinueLastVideoPosition
		next.Preferences.PlayerKeymap = req.PlayerKeymap
		if err := next.Save(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cfg.Port = next.Port
		cfg.StoragePath = next.StoragePath
		cfg.Libraries = append([]config.Library(nil), next.Libraries...)
		cfg.ThumbnailDir = next.ThumbnailDir
		cfg.ThumbnailSize = next.ThumbnailSize
		cfg.TrashDir = next.TrashDir
		cfg.UseSystemPlayer = next.UseSystemPlayer
		cfg.Preferences = next.Preferences
		if prevThumbnailSize != cfg.ThumbnailSize && prevThumbnailDir != "" {
			_ = os.RemoveAll(prevThumbnailDir)
			_ = os.MkdirAll(prevThumbnailDir, 0755)
		}
		message := "设置已保存，涉及服务端行为的变更在重启后完全生效"
		if prevThumbnailSize != cfg.ThumbnailSize {
			message = "设置已保存，缩略图缓存已重置，后续浏览会按新尺寸重新生成"
		}
		c.JSON(http.StatusOK, gin.H{
			"message": message,
			"data":    buildSettingsResponse(cfg),
		})
	}
}

func handleRestartApp(restart func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if restart == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持网页重启"})
			return
		}
		if err := restart(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "服务正在重启"})
	}
}

func handleShutdownApp(shutdown func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shutdown == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持全局退出"})
			return
		}
		if err := shutdown(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "正在退出 EchoGallery"})
	}
}

func handleUploadLibraryLogo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		index, err := parseLibraryIndex(c.Param("index"), len(cfg.Libraries))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少上传文件"})
			return
		}
		ext := strings.ToLower(filepath.Ext(file.Filename))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 png/jpg/jpeg/gif/webp"})
			return
		}

		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := os.MkdirAll(assetDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		next := *cfg
		next.Libraries = append([]config.Library(nil), cfg.Libraries...)
		fileName := fmt.Sprintf("library-%d-%d.png", index, time.Now().UnixNano())
		destPath := filepath.Join(assetDir, fileName)
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上传文件失败"})
			return
		}
		defer src.Close()
		if err := imgpkg.SaveSquareLibraryLogo(src, imgpkg.DetectMimeType(file.Filename), destPath, imgpkg.DefaultLibraryLogoEdge); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		previous := next.Libraries[index].LogoAsset
		next.Libraries[index].LogoAsset = fileName
		if err := next.Save(); err != nil {
			_ = os.Remove(destPath)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cfg.Libraries = next.Libraries
		if previous != "" && previous != fileName {
			_ = os.Remove(filepath.Join(assetDir, previous))
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "资源库头像已上传",
			"data":    buildSettingsResponse(cfg),
		})
	}
}

func handleDeleteLibraryLogo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		index, err := parseLibraryIndex(c.Param("index"), len(cfg.Libraries))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		next := *cfg
		next.Libraries = append([]config.Library(nil), cfg.Libraries...)
		previous := next.Libraries[index].LogoAsset
		next.Libraries[index].LogoAsset = ""
		if err := next.Save(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cfg.Libraries = next.Libraries
		if previous != "" {
			_ = os.Remove(filepath.Join(assetDir, previous))
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "资源库图像已移除",
			"data":    buildSettingsResponse(cfg),
		})
	}
}

func handleRefreshLibraryLogos(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req refreshLibraryLogosRequest
		_ = c.ShouldBindJSON(&req)

		result := refreshLibraryLogosResult{}
		if len(cfg.Libraries) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"message": "没有可更新的资源库头像",
				"result":  result,
				"data":    buildSettingsResponse(cfg),
			})
			return
		}

		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := os.MkdirAll(assetDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		next := *cfg
		next.Libraries = append([]config.Library(nil), cfg.Libraries...)
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		previousAssets := make([]string, len(next.Libraries))
		for index, library := range next.Libraries {
			result.Scanned++
			if strings.TrimSpace(library.Path) == "" {
				result.Skipped++
				continue
			}
			if strings.TrimSpace(library.LogoAsset) != "" && !req.ReplaceExisting {
				result.Skipped++
				continue
			}
			sourcePath, err := randomLibraryLogoSource(library.Path, rng)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", library.Name, err))
				continue
			}
			file, err := os.Open(sourcePath)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", library.Name, err))
				continue
			}
			fileName := fmt.Sprintf("library-%d-%d.png", index, time.Now().UnixNano())
			destPath := filepath.Join(assetDir, fileName)
			saveErr := imgpkg.SaveSquareLibraryLogo(file, imgpkg.DetectMimeType(sourcePath), destPath, imgpkg.DefaultLibraryLogoEdge)
			_ = file.Close()
			if saveErr != nil {
				_ = os.Remove(destPath)
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", library.Name, saveErr))
				continue
			}
			previousAssets[index] = next.Libraries[index].LogoAsset
			next.Libraries[index].LogoAsset = fileName
			result.Updated++
		}

		if result.Updated > 0 {
			if err := next.Save(); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Libraries = append([]config.Library(nil), next.Libraries...)
			for _, previous := range previousAssets {
				if previous != "" {
					_ = os.Remove(filepath.Join(assetDir, previous))
				}
			}
		}

		message := fmt.Sprintf("资源库头像刷新完成：更新 %d 个，跳过 %d 个，失败 %d 个", result.Updated, result.Skipped, result.Failed)
		c.JSON(http.StatusOK, gin.H{
			"message": message,
			"result":  result,
			"data":    buildSettingsResponse(cfg),
		})
	}
}

func randomLibraryLogoSource(root string, rng *rand.Rand) (string, error) {
	var selected string
	seen := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".thumbnails" || name == ".DS_Store" || strings.HasPrefix(name, ".echogallery") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isLibraryLogoCandidate(path) {
			return nil
		}
		seen++
		if rng.Intn(seen) == 0 {
			selected = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", fmt.Errorf("未找到可用图片")
	}
	return selected, nil
}

func isLibraryLogoCandidate(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func handleServeLibraryLogo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		index, err := parseLibraryIndex(c.Param("index"), len(cfg.Libraries))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		fileName := strings.TrimSpace(cfg.Libraries[index].LogoAsset)
		if fileName == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
			return
		}
		assetDir, err := cfg.LibraryAssetsDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.File(filepath.Join(assetDir, filepath.Base(fileName)))
	}
}

func parseLibraryIndex(raw string, total int) (int, error) {
	index := -1
	if _, err := fmt.Sscanf(raw, "%d", &index); err != nil || index < 0 || index >= total {
		return 0, fmt.Errorf("资源库索引无效")
	}
	return index, nil
}

func handleRefreshVideoThumbnails(cfg *config.Config, registrar interface {
	RefreshVideoThumbnails(userID int64) (*service.VideoThumbnailRefreshResult, error)
}) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		result, err := registrar.RefreshVideoThumbnails(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "视频缩略图刷新完成",
			"data":    result,
		})
	}
}

func handleBackfillPhotoEXIF(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		backfiller, ok := registrar.(interface {
			BackfillPhotoEXIF(userID int64) (*service.EXIFBackfillResult, error)
		})
		if !ok || backfiller == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持 EXIF 回填"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		result, err := backfiller.BackfillPhotoEXIF(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "EXIF 回填完成",
			"result":  result,
		})
	}
}
