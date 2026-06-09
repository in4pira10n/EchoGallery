package api

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"echogallery/internal/config"
	"echogallery/internal/image"
	"echogallery/internal/media"
	"echogallery/internal/service"
	"echogallery/internal/storage"
)

const authCookieName = "echogallery_token"

func authCookieNameForConfig(cfg *config.Config) string {
	if cfg == nil || cfg.Port <= 0 {
		return authCookieName
	}
	return fmt.Sprintf("%s_%d", authCookieName, cfg.Port)
}

func authCookieNamesForConfig(cfg *config.Config) []string {
	scopedName := authCookieNameForConfig(cfg)
	if scopedName == authCookieName {
		return []string{authCookieName}
	}
	return []string{scopedName, authCookieName}
}

func setAuthCookie(w http.ResponseWriter, cfg *config.Config, token string) {
	http.SetCookie(w, &http.Cookie{Name: authCookieNameForConfig(cfg), Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(7 * 24 * time.Hour)})
	if authCookieNameForConfig(cfg) != authCookieName {
		clearAuthCookieName(w, authCookieName)
	}
}

func clearAuthCookieName(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func clearAuthCookies(w http.ResponseWriter, cfg *config.Config) {
	for _, name := range authCookieNamesForConfig(cfg) {
		clearAuthCookieName(w, name)
	}
}

func readAuthCookie(c *gin.Context, cfg *config.Config) (string, error) {
	var lastErr error
	for _, name := range authCookieNamesForConfig(cfg) {
		value, err := c.Cookie(name)
		if err == nil {
			return value, nil
		}
		lastErr = err
	}
	return "", lastErr
}

type thumbnailRequestTask struct {
	done chan struct{}
	path string
	err  error
}

type thumbnailRequestCoordinator struct {
	sem   chan struct{}
	mu    sync.Mutex
	tasks map[string]*thumbnailRequestTask
}

func newThumbnailRequestCoordinator() *thumbnailRequestCoordinator {
	limit := runtime.GOMAXPROCS(0) / 3
	if limit < 1 {
		limit = 1
	}
	if limit > 3 {
		limit = 3
	}
	return &thumbnailRequestCoordinator{
		sem:   make(chan struct{}, limit),
		tasks: make(map[string]*thumbnailRequestTask),
	}
}

func (c *thumbnailRequestCoordinator) Do(key string, fn func() (string, error)) (string, error) {
	c.mu.Lock()
	if task, ok := c.tasks[key]; ok {
		c.mu.Unlock()
		<-task.done
		return task.path, task.err
	}
	task := &thumbnailRequestTask{done: make(chan struct{})}
	c.tasks[key] = task
	c.mu.Unlock()

	c.sem <- struct{}{}
	path, err := fn()
	<-c.sem

	c.mu.Lock()
	task.path = path
	task.err = err
	close(task.done)
	delete(c.tasks, key)
	c.mu.Unlock()
	return path, err
}

var onDemandThumbnailCoordinator = newThumbnailRequestCoordinator()

type videoRegistrar interface {
	AddPhoto(albumID int64, photoID int64, userID int64) error
	CreateAlbum(name, description string, userID int64) (*storage.Album, error)
	CreateShare(input service.CreateShareInput) (*storage.ShareLink, error)
	DeleteAlbum(id int64, userID int64) error
	DeleteShare(id int64, userID int64) error
	GetAlbum(id int64, userID int64) (*storage.Album, error)
	GetAlbumDownloadEntries(albumID int64, userID int64) (string, []service.DownloadEntry, error)
	GetShareByToken(token string) (*storage.ShareLink, error)
	ListAlbums(userID int64) ([]*storage.Album, error)
	ListAlbumsForPhoto(photoID int64, userID int64) ([]*storage.Album, error)
	ListShares(userID int64) ([]*storage.ShareLink, error)
	RemovePhoto(albumID int64, photoID int64, userID int64) error
	UpdateAlbum(id int64, name, description string, coverPhotoID *int64, userID int64) (*storage.Album, error)
	RegisterUploadedVideo(input service.RegisterUploadedVideoInput) (*storage.Photo, error)
	DeletePhoto(id int64, userID int64) error
	EmptyTrash(userID int64) error
	GetPhoto(id int64, userID int64) (*storage.Photo, error)
	GetPhotoByUUIDAny(uuid string, userID int64) (*storage.Photo, error)
	GetDownloadEntries(photoIDs []int64, userID int64) ([]service.DownloadEntry, error)
	GetAlbumMedia(params storage.ListAlbumPhotosParams) (*storage.PhotoPage, error)
	GetFavorites(params storage.ListPhotosParams) (*storage.PhotoPage, error)
	GetTrash(params storage.ListPhotosParams) (*storage.PhotoPage, error)
	GetRandomMedia(params storage.RandomPhotosParams) (*storage.PhotoPage, error)
	SearchMedia(params storage.SearchPhotosParams) (*storage.PhotoPage, error)
	PermanentlyDeletePhoto(id int64, userID int64) error
	PlayWithSystemPlayer(id int64, userID int64) error
	RevealAlbumInFinder(id int64, userID int64) error
	RevealInFinder(id int64, userID int64) error
	RestorePhoto(id int64, userID int64) error
	RefreshVideoThumbnails(userID int64) (*service.VideoThumbnailRefreshResult, error)
	StartVideoThumbnailRefresh(userID int64) (service.VideoThumbnailRefreshStatus, error)
	GetVideoThumbnailRefreshStatus(userID int64) service.VideoThumbnailRefreshStatus
	CancelVideoThumbnailRefresh(userID int64) (service.VideoThumbnailRefreshStatus, error)
	ListPlaybackCaches(userID int64) ([]service.PlaybackCacheEntry, error)
	BuildBrowserPlaybackCache(photoID int64, userID int64) (*service.PlaybackCacheEntry, error)
	DeletePlaybackCaches(userID int64, uuids []string) (int, error)
	StartPlaybackCacheBuild(userID int64) service.PlaybackCacheBuildStatus
	GetPlaybackCacheBuildStatus(userID int64) service.PlaybackCacheBuildStatus
	CancelPlaybackCacheBuild(userID int64) service.PlaybackCacheBuildStatus
	SetPhotoFavorite(id int64, userID int64, favorite bool, superFavorite bool) error
	GetTimeline(params storage.ListPhotosParams) (*storage.PhotoPage, error)
	Upload(input service.UploadInput) (*service.UploadResult, error)
	MediaPath(photo *storage.Photo) string
	BrowserPlaybackPath(photo *storage.Photo) (string, string, error)
	PosterPath(photo *storage.Photo) string
	ThumbnailPath(photo *storage.Photo) string
	ThumbnailCandidates(photo *storage.Photo) []string
}

type mediaDownloadRequest struct {
	MediaIDs []int64 `json:"media_ids"`
	PhotoIDs []int64 `json:"photo_ids"`
}

type albumMediaRequest struct {
	MediaID int64 `json:"media_id"`
	PhotoID int64 `json:"photo_id"`
}

type favoriteRequest struct {
	Favorite      bool `json:"favorite"`
	SuperFavorite bool `json:"super_favorite"`
}

type videoPlaybackPreferenceRequest struct {
	Volume     *float64                         `json:"volume"`
	Muted      *bool                            `json:"muted"`
	ResumeTime *int64                           `json:"resume_time"`
	Bookmarks  *[]storage.VideoPlaybackBookmark `json:"bookmarks"`
}

type thumbnailWarmRequest struct {
	UUIDs []string `json:"uuids"`
}

type loginHeroMediaItem struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type loginHeroResponse struct {
	Avatar loginHeroMediaItem   `json:"avatar"`
	Photos []loginHeroMediaItem `json:"photos"`
}

type albumRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	CoverPhotoID *int64 `json:"cover_photo_id"`
}

type shareRequest struct {
	Type      string `json:"type"`
	TargetID  int64  `json:"target_id"`
	ExpiresIn *int64 `json:"expires_in_days,omitempty"`
}

type shareDetailResponse struct {
	*storage.ShareLink
	TargetUUID         string `json:"target_uuid,omitempty"`
	TargetOriginalName string `json:"target_original_name,omitempty"`
	TargetMediaKind    string `json:"target_media_kind,omitempty"`
	TargetMimeType     string `json:"target_mime_type,omitempty"`
}

type thumbnailWarmer interface {
	WarmThumbnailsByUUIDs(uuids []string, userID int64) (int, error)
}

type authLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authRegisterRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

type contextKey string

const userContextKey contextKey = "user"

// Claims JWT claims。
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// NewRouter 创建 Gin HTTP 入口。
func NewRouter(cfg *config.Config, registrar videoRegistrar) http.Handler {
	return NewRouterWithStaticWithRestart(cfg, nil, registrar, nil)
}

func NewRouterWithStatic(cfg *config.Config, staticFS fs.FS, registrar videoRegistrar) http.Handler {
	return NewRouterWithStaticWithRestart(cfg, staticFS, registrar, nil)
}

func NewRouterWithStaticWithRestart(cfg *config.Config, staticFS fs.FS, registrar videoRegistrar, restart func() error) http.Handler {
	return NewRouterWithStaticWithLifecycle(cfg, staticFS, registrar, restart, nil)
}

func NewRouterWithStaticWithLifecycle(cfg *config.Config, staticFS fs.FS, registrar videoRegistrar, restart func() error, shutdown func() error) http.Handler {
	return NewRouterWithStaticWithLifecycleAndBuild(cfg, staticFS, registrar, restart, shutdown, LibraryBuildHooks{}, LibraryBatchBuildHooks{}, LibraryBatchThumbnailBuildHooks{})
}

func NewRouterWithStaticWithLifecycleAndBuild(cfg *config.Config, staticFS fs.FS, registrar videoRegistrar, restart func() error, shutdown func() error, buildHooks LibraryBuildHooks, batchBuildHooks LibraryBatchBuildHooks, batchThumbnailBuildHooks LibraryBatchThumbnailBuildHooks) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(compactGinLogger(), gin.Recovery())

	r.GET("/static/*filepath", gin.WrapH(buildStaticHandler(staticFS)))
	r.GET("/pages/*filepath", gin.WrapH(buildPagesHandler(staticFS)))
	r.GET("/manifest.webmanifest", handleWebManifest(staticFS))
	r.GET("/sw.js", handleServiceWorker(staticFS))

	r.GET("/", pageAuthMiddleware(cfg), handleAppPage(staticFS))
	r.GET("/albums", pageAuthMiddleware(cfg), handleAppPage(staticFS))
	r.GET("/albums/:id", pageAuthMiddleware(cfg), handleAppPage(staticFS))
	r.GET("/trash", pageAuthMiddleware(cfg), handleAppPage(staticFS))
	r.GET("/login", handleLoginPage(staticFS))
	r.GET("/register", handleRegisterPage(staticFS))
	r.GET("/gallery-chooser", handleGalleryChooserPage(staticFS))
	r.GET("/api/login/hero", handleLoginHero(cfg, registrar))
	r.GET("/api/login/hero/:kind/:name", handleServeLoginHeroAsset(cfg, registrar))
	r.POST("/api/auth/login", handleLogin(cfg))
	r.POST("/api/auth/register", handleRegister(cfg))
	r.POST("/api/auth/logout", handleLogout(cfg))
	r.GET("/api/settings", authMiddleware(cfg), handleGetSettings(cfg))
	r.PUT("/api/settings", authMiddleware(cfg), handleUpdateSettings(cfg))
	r.POST("/api/settings/restart", authMiddleware(cfg), handleRestartApp(restart))
	r.POST("/api/settings/shutdown", authMiddleware(cfg), handleShutdownApp(shutdown))
	r.GET("/api/library-build/status", authMiddleware(cfg), handleGetLibraryBuildStatus(buildHooks))
	r.PUT("/api/library-build/exit-after-complete", authMiddleware(cfg), handleSetLibraryBuildExitAfterComplete(buildHooks))
	r.DELETE("/api/library-build", authMiddleware(cfg), handleCancelLibraryBuild(buildHooks))
	r.GET("/api/settings/libraries/build-all", authMiddleware(cfg), handleGetLibraryBatchBuildStatus(cfg, batchBuildHooks))
	r.POST("/api/settings/libraries/build-all", authMiddleware(cfg), handleStartLibraryBatchBuild(cfg, batchBuildHooks))
	r.DELETE("/api/settings/libraries/build-all", authMiddleware(cfg), handleCancelLibraryBatchBuild(cfg, batchBuildHooks))
	r.PUT("/api/settings/libraries/build-all/exit-after-complete", authMiddleware(cfg), handleSetLibraryBatchBuildExitAfterComplete(cfg, batchBuildHooks))
	r.PUT("/api/settings/libraries/build-all/selection", authMiddleware(cfg), handleSetLibraryBatchBuildSelection(cfg, batchBuildHooks))
	r.GET("/api/settings/libraries/thumbnails/build-all", authMiddleware(cfg), handleGetLibraryBatchThumbnailBuildStatus(cfg, batchThumbnailBuildHooks))
	r.POST("/api/settings/libraries/thumbnails/build-all", authMiddleware(cfg), handleStartLibraryBatchThumbnailBuild(cfg, batchThumbnailBuildHooks))
	r.DELETE("/api/settings/libraries/thumbnails/build-all", authMiddleware(cfg), handleCancelLibraryBatchThumbnailBuild(cfg, batchThumbnailBuildHooks))
	r.PUT("/api/settings/libraries/thumbnails/build-all/exit-after-complete", authMiddleware(cfg), handleSetLibraryBatchThumbnailBuildExitAfterComplete(cfg, batchThumbnailBuildHooks))
	r.PUT("/api/settings/libraries/thumbnails/build-all/selection", authMiddleware(cfg), handleSetLibraryBatchThumbnailBuildSelection(cfg, batchThumbnailBuildHooks))
	r.POST("/api/settings/libraries/logos/refresh", authMiddleware(cfg), handleRefreshLibraryLogos(cfg))
	r.POST("/api/settings/libraries/:index/logo", authMiddleware(cfg), handleUploadLibraryLogo(cfg))
	r.DELETE("/api/settings/libraries/:index/logo", authMiddleware(cfg), handleDeleteLibraryLogo(cfg))
	r.GET("/api/settings/libraries/:index/logo", authMiddleware(cfg), handleServeLibraryLogo(cfg))
	r.GET("/api/settings/thumbnails/build", authMiddleware(cfg), handleGetThumbnailBuildStatus(cfg, registrar))
	r.POST("/api/settings/thumbnails/build", authMiddleware(cfg), handleStartThumbnailBuild(cfg, registrar))
	r.DELETE("/api/settings/thumbnails/build", authMiddleware(cfg), handleCancelThumbnailBuild(cfg, registrar))
	r.GET("/api/settings/video-thumbnails/refresh", authMiddleware(cfg), handleGetVideoThumbnailRefreshStatus(cfg, registrar))
	r.POST("/api/settings/video-thumbnails/refresh", authMiddleware(cfg), handleStartVideoThumbnailRefresh(cfg, registrar))
	r.DELETE("/api/settings/video-thumbnails/refresh", authMiddleware(cfg), handleCancelVideoThumbnailRefresh(cfg, registrar))
	r.GET("/api/settings/playback-cache", authMiddleware(cfg), handleListPlaybackCaches(cfg, registrar))
	r.DELETE("/api/settings/playback-cache", authMiddleware(cfg), handleDeletePlaybackCaches(cfg, registrar))
	r.GET("/api/settings/playback-cache/build", authMiddleware(cfg), handleGetPlaybackCacheBuildStatus(cfg, registrar))
	r.POST("/api/settings/playback-cache/build", authMiddleware(cfg), handleStartPlaybackCacheBuild(cfg, registrar))
	r.DELETE("/api/settings/playback-cache/build", authMiddleware(cfg), handleCancelPlaybackCacheBuild(cfg, registrar))
	r.POST("/api/settings/exif/backfill", authMiddleware(cfg), handleBackfillPhotoEXIF(cfg, registrar))
	r.GET("/api/player/keymap", authMiddleware(cfg), handleGetPlayerKeymap(cfg))

	media := r.Group("/api/media")
	{
		media.GET("/albums", authMiddleware(cfg), handleListAlbumsMedia(cfg, registrar))
		media.POST("/albums", authMiddleware(cfg), handleCreateAlbumMedia(cfg, registrar))
		media.GET("/albums/:id/detail", authMiddleware(cfg), handleGetAlbumDetail(cfg, registrar))
		media.GET("/albums/:id/download", authMiddleware(cfg), handleDownloadAlbumMedia(cfg, registrar))
		media.GET("/albums/:id", authMiddleware(cfg), handleListAlbumMedia(cfg, registrar))
		media.POST("/albums/:id/reveal", authMiddleware(cfg), handleRevealAlbumInFinder(cfg, registrar))
		media.POST("/albums/:id", authMiddleware(cfg), handleAddMediaToAlbum(cfg, registrar))
		media.PUT("/albums/:id", authMiddleware(cfg), handleUpdateAlbumMedia(cfg, registrar))
		media.DELETE("/albums/:id", authMiddleware(cfg), handleDeleteAlbumMedia(cfg, registrar))
		media.DELETE("/albums/:id/:mediaId", authMiddleware(cfg), handleRemoveMediaFromAlbum(cfg, registrar))
		media.GET("/shares", authMiddleware(cfg), handleListSharesMedia(cfg, registrar))
		media.POST("/shares", authMiddleware(cfg), handleCreateShareMedia(cfg, registrar))
		media.DELETE("/shares/:id", authMiddleware(cfg), handleDeleteShareMedia(cfg, registrar))
		media.GET("", authMiddleware(cfg), handleListMedia(cfg, registrar))
		media.GET("/search", authMiddleware(cfg), handleSearchMedia(cfg, registrar))
		media.GET("/random", authMiddleware(cfg), handleListRandomMedia(cfg, registrar))
		media.POST("/thumbnails/warm", authMiddleware(cfg), handleWarmVisibleThumbnails(cfg, registrar))
		media.GET("/favorites", authMiddleware(cfg), handleListFavoriteMedia(cfg, registrar))
		media.GET("/trash", authMiddleware(cfg), handleListTrashMedia(cfg, registrar))
		media.GET("/:id/albums", authMiddleware(cfg), handleListMediaAlbums(cfg, registrar))
		media.GET("/:id/playback", authMiddleware(cfg), handleGetVideoPlaybackPreference(cfg, registrar))
		media.PUT("/:id/playback", authMiddleware(cfg), handleSaveVideoPlaybackPreference(cfg, registrar))
		media.POST("/:id/playback-cache", authMiddleware(cfg), handleBuildBrowserPlaybackCache(cfg, registrar))
		media.GET("/:id", authMiddleware(cfg), handleGetMedia(cfg, registrar))
		media.GET("/:id/download", authMiddleware(cfg), handleDownloadMedia(cfg, registrar))
		media.POST("/:id/play", authMiddleware(cfg), handlePlayMediaWithSystemPlayer(cfg, registrar))
		media.POST("/:id/reveal", authMiddleware(cfg), handleRevealMediaInFinder(cfg, registrar))
		media.POST("/download", authMiddleware(cfg), handleDownloadMediaBatch(cfg, registrar))
		media.POST("/:id/restore", authMiddleware(cfg), handleRestoreMedia(cfg, registrar))
		media.PUT("/:id/favorite", authMiddleware(cfg), handleSetMediaFavorite(cfg, registrar))
		media.DELETE("/:id", authMiddleware(cfg), handleDeleteMedia(cfg, registrar))
		media.DELETE("/trash", authMiddleware(cfg), handleEmptyTrashMedia(cfg, registrar))
		media.DELETE("/trash/:id", authMiddleware(cfg), handleHardDeleteMedia(cfg, registrar))
		media.POST("/upload", authMiddleware(cfg), handleUploadPlaceholder(cfg, registrar))
	}

	r.GET("/media/files/:uuid", authMiddleware(cfg), handleServeMediaFile(cfg, registrar))
	r.GET("/media/playback/:uuid", authMiddleware(cfg), handleServeBrowserPlaybackFile(cfg, registrar))
	r.GET("/media/photos/:uuid", authMiddleware(cfg), handleServePhotoFile(cfg, registrar))
	r.GET("/media/thumbnails/:uuid", authMiddleware(cfg), handleServeThumbnailFile(cfg, registrar))
	r.GET("/media/posters/:uuid", authMiddleware(cfg), handleServePoster(cfg, registrar))
	r.GET("/media/s/:token/:uuid", handleServeSharedMediaFile(cfg, registrar))
	r.GET("/api/s/:token", handleGetShareByToken(cfg, registrar))
	r.GET("/api/s/:token/photos", handleGetSharedAlbumMedia(cfg, registrar))
	r.GET("/s/:token/download", handleDownloadSharedMedia(cfg, registrar))
	r.GET("/s/:token", handleSharePage())

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "不支持的请求方法"})
	})

	return r
}

func compactGinLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
		path := compactLogPath(p.Path)
		timeText := p.TimeStamp.Format("15:04:05")
		statusText := fmt.Sprintf("%3d", p.StatusCode)
		methodText := fmt.Sprintf("%-6s", p.Method)
		latencyText := compactLogLatency(p.Latency)
		if p.IsOutputColor() {
			prefixPlain := fmt.Sprintf("%s %s %s %s", timeText, statusText, strings.TrimSpace(methodText), path)
			padding := compactLogLatencyPadding(prefixPlain, latencyText)
			line := fmt.Sprintf("%s %s%s%s %s%s%s %s%s%s",
				timeText,
				p.StatusCodeColor(),
				statusText,
				p.ResetColor(),
				p.MethodColor(),
				methodText,
				p.ResetColor(),
				path,
				padding,
				latencyText,
			)
			if p.ErrorMessage != "" {
				line += " | " + strings.TrimSpace(p.ErrorMessage)
			}
			return line + "\n"
		}
		prefixPlain := fmt.Sprintf("%s %s %s %s", timeText, statusText, strings.TrimSpace(methodText), path)
		padding := compactLogLatencyPadding(prefixPlain, latencyText)
		line := fmt.Sprintf("%s %s %s %s%s%s",
			timeText,
			statusText,
			methodText,
			path,
			padding,
			latencyText,
		)
		if p.ErrorMessage != "" {
			line += " | " + strings.TrimSpace(p.ErrorMessage)
		}
		return line + "\n"
	})
}

func compactLogPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	const maxLen = 46
	if len(path) <= maxLen {
		return path
	}
	const head = 28
	const tail = maxLen - head - 3
	return path[:head] + "..." + path[len(path)-tail:]
}

func compactLogLatency(latency time.Duration) string {
	switch {
	case latency >= time.Second:
		return fmt.Sprintf("%.2fs", latency.Seconds())
	case latency >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(latency)/float64(time.Millisecond))
	case latency >= time.Microsecond:
		return fmt.Sprintf("%dus", latency.Microseconds())
	default:
		return fmt.Sprintf("%dns", latency.Nanoseconds())
	}
}

func compactLogLatencyPadding(prefix, latency string) string {
	const targetWidth = 96
	padding := targetWidth - len(prefix) - len(latency)
	if padding < 2 {
		padding = 2
	}
	return strings.Repeat(" ", padding)
}

func buildStaticHandler(staticFS fs.FS) http.Handler {
	if staticFS != nil {
		sub, err := fs.Sub(staticFS, "web/static")
		if err == nil {
			return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
		}
	}
	return http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))
}

func buildPagesHandler(staticFS fs.FS) http.Handler {
	if staticFS != nil {
		sub, err := fs.Sub(staticFS, "web/pages")
		if err == nil {
			return http.StripPrefix("/pages/", http.FileServer(http.FS(sub)))
		}
	}
	return http.StripPrefix("/pages/", http.FileServer(http.Dir("web/pages")))
}

func handleLogin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req authLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		verifyCfg := &config.Config{Users: cfg.Users}
		if _, err := config.VerifyPassword(verifyCfg, req.Username, req.Password); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}
		token, err := generateAuthToken(cfg.JWTSecret, req.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
			return
		}
		setAuthCookie(c.Writer, cfg, token)
		c.JSON(http.StatusOK, gin.H{"message": "登录成功"})
	}
}

func handleRegister(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req authRegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Password != req.ConfirmPassword {
			c.JSON(http.StatusBadRequest, gin.H{"error": "两次输入的密码不一致"})
			return
		}
		if _, err := config.RegisterUser(cfg, req.Username, req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "注册成功，请登录后继续"})
	}
}

func handleLogout(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		clearAuthCookies(c.Writer, cfg)
		c.JSON(http.StatusOK, gin.H{"message": "已退出登录"})
	}
}

func generateAuthToken(secret, username string) (string, error) {
	claims := Claims{Username: username, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func handleGetMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		photo, err := registrar.GetPhoto(id, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
			return
		}
		c.JSON(http.StatusOK, photo)
	}
}

func handleDownloadMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		photo, err := registrar.GetPhoto(id, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
			return
		}

		c.Header("Content-Type", photo.MimeType)
		c.Header("Content-Disposition", contentDispositionAttachment(photo.OriginalName))
		c.File(registrar.MediaPath(photo))
	}
}

func handleRevealMediaInFinder(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		if err := registrar.RevealInFinder(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已在文件管理器中定位"})
	}
}

func handleRevealAlbumInFinder(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册 ID 无效"})
			return
		}
		if err := registrar.RevealAlbumInFinder(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已在文件管理器中定位"})
	}
}

func handlePlayMediaWithSystemPlayer(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		if err := registrar.PlayWithSystemPlayer(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已使用系统播放器打开"})
	}
}

func handleDeleteMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		if err := registrar.DeletePhoto(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已移入回收站"})
	}
}

func handleRestoreMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		if err := registrar.RestorePhoto(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "恢复成功"})
	}
}

func handleListTrashMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		page, err := registrar.GetTrash(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    c.Query("cursor"),
			Limit:     mediaPageLimit(c),
			MediaKind: mediaSearchKind(c),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func handleListFavoriteMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		page, err := registrar.GetFavorites(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    c.Query("cursor"),
			Limit:     mediaPageLimit(c),
			MediaKind: mediaSearchKind(c),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func handleSetMediaFavorite(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		var req favoriteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		if req.SuperFavorite {
			req.Favorite = true
		}
		if err := registrar.SetPhotoFavorite(id, userID, req.Favorite, req.SuperFavorite); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		message := "已取消收藏"
		if req.SuperFavorite {
			message = "已加入特别喜欢"
		} else if req.Favorite {
			message = "已加入个人收藏"
		}
		c.JSON(http.StatusOK, gin.H{"message": message})
	}
}

func handleGetVideoPlaybackPreference(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	type playbackGetter interface {
		GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error)
	}
	return func(c *gin.Context) {
		getter, ok := registrar.(playbackGetter)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "视频播放偏好服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		pref, err := getter.GetVideoPlaybackPreference(id, userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, pref)
	}
}

func handleSaveVideoPlaybackPreference(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	type playbackSaver interface {
		GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error)
		SaveVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool, resumeTime int64, bookmarks []storage.VideoPlaybackBookmark) (*storage.VideoPlaybackPreference, error)
	}
	return func(c *gin.Context) {
		saver, ok := registrar.(playbackSaver)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "视频播放偏好服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		var req videoPlaybackPreferenceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		volume := 1.0
		muted := false
		resumeTime := int64(0)
		var bookmarks []storage.VideoPlaybackBookmark
		if existing, err := saver.GetVideoPlaybackPreference(id, userID); err == nil && existing != nil {
			volume = existing.Volume
			muted = existing.Muted
			resumeTime = existing.ResumeTime
			bookmarks = append([]storage.VideoPlaybackBookmark(nil), existing.Bookmarks...)
		}
		if req.Volume != nil {
			volume = *req.Volume
		}
		if req.Muted != nil {
			muted = *req.Muted
		}
		if req.ResumeTime != nil {
			resumeTime = *req.ResumeTime
		}
		if req.Bookmarks != nil {
			bookmarks = append([]storage.VideoPlaybackBookmark(nil), (*req.Bookmarks)...)
		}
		pref, err := saver.SaveVideoPlaybackPreference(id, userID, volume, muted, resumeTime, bookmarks)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, pref)
	}
}

func handleListAlbumMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}
		page, err := registrar.GetAlbumMedia(storage.ListAlbumPhotosParams{
			AlbumID:   albumID,
			UserID:    userID,
			Cursor:    c.Query("cursor"),
			Limit:     mediaPageLimit(c),
			MediaKind: mediaSearchKind(c),
			Sort:      albumMediaSort(c),
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func albumMediaSort(c *gin.Context) string {
	raw := strings.TrimSpace(strings.ToLower(c.Query("sort")))
	switch raw {
	case "name", "media_name", "filename":
		return "name"
	case "size", "file_size":
		return "size"
	case "timeline", "time_desc", "date_desc", "desc":
		return "timeline_desc"
	case "timeline_asc", "time_asc", "date_asc", "asc":
		return "timeline_asc"
	default:
		return "timeline_desc"
	}
}

func handleAddMediaToAlbum(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}

		var req albumMediaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		mediaID := req.MediaID
		if mediaID <= 0 {
			mediaID = req.PhotoID
		}
		if mediaID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "media_id 不能为空"})
			return
		}
		if err := registrar.AddPhoto(albumID, mediaID, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已添加到相册"})
	}
}

func handleRemoveMediaFromAlbum(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}
		mediaID, err := strconv.ParseInt(c.Param("mediaId"), 10, 64)
		if err != nil || mediaID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		if err := registrar.RemovePhoto(albumID, mediaID, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已从相册移除"})
	}
}

func handleGetAlbumDetail(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}
		album, err := registrar.GetAlbum(albumID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if album == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "相册不存在"})
			return
		}
		c.JSON(http.StatusOK, album)
	}
}

func handleListAlbumsMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albums, err := registrar.ListAlbums(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, albums)
	}
}

func handleListMediaAlbums(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		mediaID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || mediaID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		albums, err := registrar.ListAlbumsForPhoto(mediaID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, albums)
	}
}

func handleCreateAlbumMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		var req albumRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		album, err := registrar.CreateAlbum(req.Name, req.Description, userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, album)
	}
}

func handleUpdateAlbumMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}
		var req albumRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		album, err := registrar.UpdateAlbum(albumID, req.Name, req.Description, req.CoverPhotoID, userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, album)
	}
}

func handleDeleteAlbumMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}
		if err := registrar.DeleteAlbum(albumID, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "相册已删除"})
	}
}

func handleListSharesMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		links, err := registrar.ListShares(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, links)
	}
}

func handleCreateShareMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		var req shareRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		var expiresAt *time.Time
		if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
			t := time.Now().Add(time.Duration(*req.ExpiresIn) * 24 * time.Hour)
			expiresAt = &t
		}
		link, err := registrar.CreateShare(service.CreateShareInput{
			Type:      req.Type,
			TargetID:  req.TargetID,
			UserID:    userID,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, link)
	}
}

func handleGetShareByToken(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		link, err := registrar.GetShareByToken(c.Param("token"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if link == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "分享链接不存在或已过期"})
			return
		}
		resp := shareDetailResponse{ShareLink: link}
		if link.Type == storage.ShareTypePhoto {
			photo, err := registrar.GetPhoto(link.TargetID, link.CreatedBy)
			if err == nil && photo != nil {
				resp.TargetUUID = photo.UUID
				resp.TargetOriginalName = photo.OriginalName
				resp.TargetMediaKind = photo.MediaKind
				resp.TargetMimeType = photo.MimeType
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}

func handleGetSharedAlbumMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		link, err := registrar.GetShareByToken(c.Param("token"))
		if err != nil || link == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "分享链接不存在或已过期"})
			return
		}
		if link.Type != storage.ShareTypeAlbum {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前分享不是相册类型"})
			return
		}
		page, err := registrar.GetAlbumMedia(storage.ListAlbumPhotosParams{
			AlbumID: link.TargetID,
			UserID:  link.CreatedBy,
			Cursor:  c.Query("cursor"),
			Limit:   mediaPageLimit(c),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func handleServePhotoFile(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		uuid := strings.TrimSuffix(c.Param("uuid"), filepath.Ext(c.Param("uuid")))
		photo, err := registrar.GetPhotoByUUIDAny(uuid, userID)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
			return
		}
		c.File(registrar.MediaPath(photo))
	}
}

func resolveExistingThumbnailPath(preferredPath string) string {
	if _, err := os.Stat(preferredPath); err == nil {
		return preferredPath
	}
	return ""
}

func resolveExistingThumbnailCandidates(paths ...string) string {
	for _, path := range paths {
		if resolved := resolveExistingThumbnailPath(path); resolved != "" {
			return resolved
		}
	}
	return ""
}

func thumbnailCandidatePaths(registrar videoRegistrar, photo *storage.Photo) []string {
	if registrar == nil || photo == nil {
		return nil
	}
	return registrar.ThumbnailCandidates(photo)
}

func generateThumbnailOnDemand(cfg *config.Config, registrar videoRegistrar, photo *storage.Photo) (string, error) {
	if photo == nil {
		return "", fmt.Errorf("媒体不存在")
	}
	thumbPath := resolveExistingThumbnailCandidates(thumbnailCandidatePaths(registrar, photo)...)
	if thumbPath != "" {
		return thumbPath, nil
	}
	if photo.MediaKind == storage.MediaKindVideo {
		preferredPosterPath := registrar.ThumbnailPath(photo)
		previewSize := cfg.ThumbnailSize
		if err := media.GeneratePoster(registrar.MediaPath(photo), preferredPosterPath, previewSize); err != nil {
			return "", err
		}
		return resolveExistingThumbnailPath(preferredPosterPath), nil
	}
	preferredThumbPath := registrar.ThumbnailPath(photo)
	file, err := os.Open(registrar.MediaPath(photo))
	if err != nil {
		return "", err
	}
	defer file.Close()
	previewSize := cfg.ThumbnailSize
	if err := image.GenerateThumbnail(file, photo.MimeType, preferredThumbPath, previewSize); err != nil {
		return "", err
	}
	return resolveExistingThumbnailPath(preferredThumbPath), nil
}

func handleWarmVisibleThumbnails(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		warmer, ok := registrar.(thumbnailWarmer)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "缩略图预热不可用"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		var req thumbnailWarmRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		queued, err := warmer.WarmThumbnailsByUUIDs(req.UUIDs, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"queued": queued})
	}
}

func handleServeThumbnailFile(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		uuid := strings.TrimSuffix(c.Param("uuid"), filepath.Ext(c.Param("uuid")))
		photo, err := registrar.GetPhotoByUUIDAny(uuid, userID)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
			return
		}
		thumbPath := resolveExistingThumbnailCandidates(thumbnailCandidatePaths(registrar, photo)...)
		if thumbPath == "" {
			thumbPath, _ = onDemandThumbnailCoordinator.Do(photo.UUID, func() (string, error) {
				return generateThumbnailOnDemand(cfg, registrar, photo)
			})
		}
		if warmer, ok := registrar.(thumbnailWarmer); ok {
			_, _ = warmer.WarmThumbnailsByUUIDs([]string{photo.UUID}, userID)
		}
		if thumbPath == "" {
			if photo.MediaKind == storage.MediaKindImage {
				c.File(registrar.MediaPath(photo))
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "缩略图不存在"})
			return
		}
		c.File(thumbPath)
	}
}

func handleServeSharedMediaFile(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		link, err := registrar.GetShareByToken(c.Param("token"))
		if err != nil || link == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "分享链接不存在或已过期"})
			return
		}

		switch link.Type {
		case storage.ShareTypePhoto:
			photo, err := registrar.GetPhoto(link.TargetID, link.CreatedBy)
			if err != nil || photo == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
				return
			}
			c.File(registrar.MediaPath(photo))
		case storage.ShareTypeAlbum:
			uuid := strings.TrimSuffix(c.Param("uuid"), filepath.Ext(c.Param("uuid")))
			page, err := registrar.GetAlbumMedia(storage.ListAlbumPhotosParams{AlbumID: link.TargetID, UserID: link.CreatedBy, Limit: 10000})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			for _, photo := range page.Photos {
				if photo.UUID == uuid {
					c.File(registrar.MediaPath(photo))
					return
				}
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前分享不支持该媒体访问"})
		}
	}
}

func handleDownloadSharedMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		link, err := registrar.GetShareByToken(c.Param("token"))
		if err != nil || link == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "分享链接不存在或已过期"})
			return
		}
		if link.Type != storage.ShareTypePhoto {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前分享不支持下载"})
			return
		}
		photo, err := registrar.GetPhoto(link.TargetID, link.CreatedBy)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "照片/视频不存在"})
			return
		}
		c.Header("Content-Type", photo.MimeType)
		c.Header("Content-Disposition", contentDispositionAttachment(photo.OriginalName))
		c.File(registrar.MediaPath(photo))
	}
}

func handleDeleteShareMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "分享ID 无效"})
			return
		}
		if err := registrar.DeleteShare(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "分享链接已删除"})
	}
}

func handleDownloadAlbumMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		albumID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || albumID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "相册ID 无效"})
			return
		}
		albumName, entries, err := registrar.GetAlbumDownloadEntries(albumID, userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", contentDispositionAttachment(sanitizeZipName(albumName)))
		if err := writeZipResponse(c.Request.Context(), c.Writer, entries); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "打包下载失败"})
			return
		}
	}
}

func handleEmptyTrashMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		if err := registrar.EmptyTrash(userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "回收站已清空"})
	}
}

func handleHardDeleteMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		if err := registrar.PermanentlyDeletePhoto(id, userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已永久删除"})
	}
}

func handleDownloadMediaBatch(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		var req mediaDownloadRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}

		ids := req.MediaIDs
		if len(ids) == 0 {
			ids = req.PhotoIDs
		}
		if len(ids) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "media_ids 不能为空"})
			return
		}

		entries, err := registrar.GetDownloadEntries(ids, userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		zipName := time.Now().Format("echogallery-selection-20060102-150405.zip")
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", contentDispositionAttachment(zipName))
		if err := writeZipResponse(c.Request.Context(), c.Writer, entries); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "打包下载失败"})
			return
		}
	}
}

func contentDispositionAttachment(filename string) string {
	trimmed := strings.ReplaceAll(filename, "\"", "")
	trimmed = strings.ReplaceAll(trimmed, "\n", "")
	trimmed = strings.ReplaceAll(trimmed, "\r", "")
	if trimmed == "" {
		trimmed = "download"
	}
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", trimmed, url.PathEscape(trimmed))
}

func handleListMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		page, err := registrar.GetTimeline(storage.ListPhotosParams{
			UserID:    userID,
			Cursor:    c.Query("cursor"),
			Limit:     mediaPageLimit(c),
			Reverse:   mediaPageReverse(c),
			SkipTotal: strings.TrimSpace(c.Query("cursor")) != "",
			MediaKind: mediaSearchKind(c),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func activeLibraryIndex(cfg *config.Config) int {
	activeID := strings.TrimSpace(cfg.ActiveLibraryID)
	if activeID != "" {
		for index, library := range cfg.Libraries {
			if strings.TrimSpace(library.ID) == activeID {
				return index
			}
		}
	}
	active := config.NormalizeStoragePath(cfg.StoragePath)
	for index, library := range cfg.Libraries {
		if config.NormalizeStoragePath(library.Path) == active {
			return index
		}
	}
	return 0
}

func loginHeroUserID(cfg *config.Config) int64 {
	if len(cfg.Users) == 0 {
		return 0
	}
	return 1
}

func handleLoginHero(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := loginHeroResponse{}
		index := activeLibraryIndex(cfg)
		if index >= 0 && index < len(cfg.Libraries) && strings.TrimSpace(cfg.Libraries[index].LogoAsset) != "" {
			resp.Avatar = loginHeroMediaItem{
				Kind: "avatar",
				URL:  fmt.Sprintf("/api/login/hero/avatar/%d?v=%s", index, url.QueryEscape(cfg.Libraries[index].LogoAsset)),
			}
		}

		if registrar != nil {
			userID := loginHeroUserID(cfg)
			if userID > 0 {
				page, err := registrar.GetRandomMedia(storage.RandomPhotosParams{
					UserID:    userID,
					Seed:      time.Now().UnixNano(),
					Limit:     2,
					SkipTotal: true,
					MediaKind: storage.MediaKindImage,
				})
				if err == nil && page != nil {
					resp.Photos = make([]loginHeroMediaItem, 0, len(page.Photos))
					for _, photo := range page.Photos {
						if photo == nil || strings.TrimSpace(photo.UUID) == "" {
							continue
						}
						resp.Photos = append(resp.Photos, loginHeroMediaItem{
							Kind: "photo",
							URL:  fmt.Sprintf("/api/login/hero/photo/%s", url.PathEscape(photo.UUID)),
						})
						if len(resp.Photos) == 2 {
							break
						}
					}
				}
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

func handleServeLoginHeroAsset(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Param("kind") {
		case "avatar":
			index, err := strconv.Atoi(strings.TrimSuffix(c.Param("name"), filepath.Ext(c.Param("name"))))
			if err != nil || index < 0 || index >= len(cfg.Libraries) {
				c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
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
		case "photo":
			if registrar == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
				return
			}
			userID := loginHeroUserID(cfg)
			if userID <= 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
				return
			}
			uuid := strings.TrimSuffix(c.Param("name"), filepath.Ext(c.Param("name")))
			photo, err := registrar.GetPhotoByUUIDAny(uuid, userID)
			if err != nil || photo == nil || photo.MediaKind != storage.MediaKindImage {
				c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
				return
			}
			thumbPath := resolveExistingThumbnailCandidates(thumbnailCandidatePaths(registrar, photo)...)
			if thumbPath == "" {
				thumbPath, _ = onDemandThumbnailCoordinator.Do(photo.UUID, func() (string, error) {
					return generateThumbnailOnDemand(cfg, registrar, photo)
				})
			}
			if thumbPath != "" {
				c.File(thumbPath)
				return
			}
			c.File(registrar.MediaPath(photo))
		default:
			c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
		}
	}
}

func handleSearchMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		query := strings.TrimSpace(c.Query("q"))
		if len([]rune(query)) > 80 {
			query = string([]rune(query)[:80])
		}
		if query == "" {
			c.JSON(http.StatusOK, &storage.PhotoPage{})
			return
		}
		limit := mediaPageLimit(c)
		if limit > 60 {
			limit = 60
		}
		page, err := registrar.SearchMedia(storage.SearchPhotosParams{
			UserID:       userID,
			Query:        query,
			Cursor:       c.Query("cursor"),
			Limit:        limit,
			MediaKind:    mediaSearchKind(c),
			OnlyFavorite: mediaSearchFavorite(c),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func handleListRandomMedia(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		seed := time.Now().UnixNano()
		if raw := strings.TrimSpace(c.Query("seed")); raw != "" {
			if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
				seed = parsed
			}
		}
		page, err := registrar.GetRandomMedia(storage.RandomPhotosParams{
			UserID:    userID,
			Seed:      seed,
			Cursor:    c.Query("cursor"),
			Limit:     mediaPageLimit(c),
			SkipTotal: strings.TrimSpace(c.Query("cursor")) != "",
			MediaKind: mediaSearchKind(c),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func mediaPageLimit(c *gin.Context) int {
	limit := 30
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		return 1
	}
	if limit > 300 {
		return 300
	}
	return limit
}

func mediaSearchKind(c *gin.Context) string {
	raw := strings.TrimSpace(strings.ToLower(c.Query("kind")))
	if raw == "" {
		raw = strings.TrimSpace(strings.ToLower(c.Query("type")))
	}
	switch raw {
	case "photo", "image", "photos", "images":
		return storage.MediaKindImage
	case "video", "videos":
		return storage.MediaKindVideo
	default:
		return ""
	}
}

func mediaSearchFavorite(c *gin.Context) bool {
	raw := strings.TrimSpace(strings.ToLower(c.Query("favorite")))
	return raw == "1" || raw == "true" || raw == "yes"
}

func mediaPageReverse(c *gin.Context) bool {
	raw := strings.TrimSpace(strings.ToLower(c.Query("order")))
	return raw == "asc" || raw == "oldest"
}

func handleServeMediaFile(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		uuid := strings.TrimSuffix(c.Param("uuid"), filepath.Ext(c.Param("uuid")))
		photo, err := registrar.GetPhotoByUUIDAny(uuid, userID)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
			return
		}
		if strings.TrimSpace(photo.MimeType) != "" {
			c.Header("Content-Type", photo.MimeType)
		}
		c.Header("Accept-Ranges", "bytes")
		c.File(registrar.MediaPath(photo))
	}
}

func handleServeBrowserPlaybackFile(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		uuid := strings.TrimSuffix(c.Param("uuid"), filepath.Ext(c.Param("uuid")))
		photo, err := registrar.GetPhotoByUUIDAny(uuid, userID)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
			return
		}
		playbackPath, mimeType, err := registrar.BrowserPlaybackPath(photo)
		if err != nil {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "当前浏览器暂不支持直接播放该文件，请先在“更多操作”里转换为受支持的格式"})
			return
		}
		if strings.TrimSpace(mimeType) != "" {
			c.Header("Content-Type", mimeType)
		} else if strings.TrimSpace(photo.MimeType) != "" {
			c.Header("Content-Type", photo.MimeType)
		}
		c.Header("Accept-Ranges", "bytes")
		c.File(playbackPath)
	}
}

func handleBuildBrowserPlaybackCache(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	type playbackBuilder interface {
		BuildBrowserPlaybackCache(photoID int64, userID int64) (*service.PlaybackCacheEntry, error)
	}
	return func(c *gin.Context) {
		builder, ok := registrar.(playbackBuilder)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "播放兼容转换服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "照片/视频ID 无效"})
			return
		}
		entry, err := builder.BuildBrowserPlaybackCache(id, userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "已转换为受支持的格式",
			"data":    entry,
		})
	}
}

func handleServePoster(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		uuid := strings.TrimSuffix(c.Param("uuid"), filepath.Ext(c.Param("uuid")))
		photo, err := registrar.GetPhotoByUUIDAny(uuid, userID)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
			return
		}
		posterPath := resolveExistingThumbnailCandidates(thumbnailCandidatePaths(registrar, photo)...)
		if posterPath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "poster 不存在"})
			return
		}
		c.File(posterPath)
	}
}

func handleUploadPlaceholder(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		if registrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体上传服务未配置"})
			return
		}
		file, err := c.FormFile("media")
		isVideoUpload := true
		if err != nil {
			file, err = c.FormFile("photo")
			isVideoUpload = false
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少媒体文件字段"})
			return
		}
		if file.Size <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "上传文件不能为空"})
			return
		}
		videoMimeType := media.DetectVideoMimeType(file.Filename)
		if isVideoUpload && videoMimeType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前仅支持常见媒体格式上传（mp4/m4v/mov/webm/mkv/avi/wmv/wma/mpeg/ts/3gp/ogv）"})
			return
		}

		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "打开上传文件失败"})
			return
		}
		defer src.Close()

		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		fileModTime := parseClientLastModified(c.PostForm("client_last_modified_ms"))
		if !isVideoUpload {
			readSeeker, ok := src.(io.ReadSeeker)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "上传文件不支持 seek"})
				return
			}
			result, err := registrar.Upload(service.UploadInput{
				Reader:       readSeeker,
				OriginalName: file.Filename,
				Size:         file.Size,
				UploadedBy:   userID,
				FileModTime:  fileModTime,
			})
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, result.Photo)
			return
		}

		uuid := uuid.NewString()
		sourceRelPath := service.UploadedMediaRelPath(uuid, file.Filename, time.Now())
		finalPath := filepath.Join(cfg.StoragePath, sourceRelPath)
		if err := saveUploadedMedia(finalPath, src); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !fileModTime.IsZero() {
			_ = os.Chtimes(finalPath, fileModTime, fileModTime)
		}

		meta := &media.VideoMeta{FormatName: strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), ".")}
		probeError := ""
		if probed, err := media.ProbeVideo(finalPath); err == nil && probed != nil {
			meta = probed
		} else if err != nil {
			probeError = err.Error()
		}

		photo, err := registrar.RegisterUploadedVideo(service.RegisterUploadedVideoInput{
			UUID:          uuid,
			OriginalName:  file.Filename,
			MimeType:      videoMimeType,
			Size:          file.Size,
			UploadedBy:    userID,
			TakenAt:       fileModTime,
			SourceRelPath: sourceRelPath,
			SourceModUnix: fileModTime.UnixNano(),
			Meta:          meta,
		})
		if err != nil {
			_ = os.Remove(finalPath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		posterPath := ""
		posterError := ""
		if err := media.GeneratePoster(finalPath, registrar.PosterPath(photo), cfg.ThumbnailSize); err == nil {
			posterPath = registrar.PosterPath(photo)
		} else if err != nil {
			posterError = err.Error()
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":      "视频上传成功",
			"filename":     file.Filename,
			"path":         finalPath,
			"poster_path":  posterPath,
			"poster_error": posterError,
			"probe_error":  probeError,
			"meta":         meta,
			"photo":        photo,
		})
	}
}

func saveUploadedMedia(destPath string, src io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建上传目录失败: %w", err)
	}
	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建上传媒体文件失败: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("保存上传媒体文件失败: %w", err)
	}

	return nil
}

func parseClientLastModified(value string) time.Time {
	ms, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || ms <= 0 {
		return time.Now()
	}
	return time.UnixMilli(ms)
}

func authMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := readAuthCookie(c, cfg)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或登录已过期"})
			c.Abort()
			return
		}

		username, err := parseToken(cfg.JWTSecret, cookie)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或登录已过期"})
			c.Abort()
			return
		}
		if !usernameExists(cfg.Users, username) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}

		c.Set(string(userContextKey), username)
		c.Next()
	}
}

func pageAuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := readAuthCookie(c, cfg)
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}
		username, err := parseToken(cfg.JWTSecret, cookie)
		if err != nil || !usernameExists(cfg.Users, username) {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}
		c.Set(string(userContextKey), username)
		c.Next()
	}
}

func currentUsername(c *gin.Context) string {
	username, _ := c.Get(string(userContextKey))
	v, _ := username.(string)
	return v
}

func currentUserID(cfg *config.Config, username string) (int64, error) {
	for i, u := range cfg.Users {
		if u.Username == username {
			return int64(i + 1), nil
		}
	}
	return 0, errors.New("用户不存在")
}

func parseToken(secret, tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token")
	}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return "", errors.New("token expired")
	}
	return claims.Username, nil
}

func usernameExists(users []config.User, username string) bool {
	for _, u := range users {
		if u.Username == username {
			return true
		}
	}
	return false
}
