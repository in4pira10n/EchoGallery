package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/service"
)

type LibraryBatchBuildLibraryStatus struct {
	ID             string   `json:"id,omitempty"`
	Name           string   `json:"name"`
	Path           string   `json:"path"`
	Status         string   `json:"status"`
	Message        string   `json:"message"`
	MessageKey     string   `json:"message_key,omitempty"`
	Imported       int      `json:"imported"`
	Skipped        int      `json:"skipped"`
	Pruned         int      `json:"pruned"`
	Generated      int      `json:"generated"`
	Failed         int      `json:"failed"`
	EXIFScanned    int      `json:"exif_scanned,omitempty"`
	EXIFUpdated    int      `json:"exif_updated,omitempty"`
	EXIFSkipped    int      `json:"exif_skipped,omitempty"`
	EXIFFailed     int      `json:"exif_failed,omitempty"`
	Matched        int      `json:"matched,omitempty"`
	Resumed        int      `json:"resumed,omitempty"`
	Existing       int      `json:"existing,omitempty"`
	MissingOld     int      `json:"missing_old,omitempty"`
	DuplicatePaths []string `json:"duplicate_paths,omitempty"`
}

type LibraryBatchBuildStatus struct {
	Discovery            *service.LibraryScanProgress     `json:"discovery,omitempty"`
	Status               string                           `json:"status"`
	Message              string                           `json:"message"`
	MessageKey           string                           `json:"message_key,omitempty"`
	SelectedLibraryIDs   []string                         `json:"selected_library_ids,omitempty"`
	SelectedPaths        []string                         `json:"selected_paths,omitempty"`
	SelectionConfigured  bool                             `json:"selection_configured,omitempty"`
	MoveLegacyThumbnails bool                             `json:"move_legacy_thumbnails,omitempty"`
	CleanThumbnailFiles  bool                             `json:"clean_thumbnail_files,omitempty"`
	CleanFiles           bool                             `json:"clean_files,omitempty"`
	BuildPlaybackCaches  bool                             `json:"build_playback_caches,omitempty"`
	CurrentLibraryID     string                           `json:"current_library_id,omitempty"`
	CurrentLibraryName   string                           `json:"current_library_name,omitempty"`
	CurrentLibraryPath   string                           `json:"current_library_path,omitempty"`
	CurrentLibraryIndex  int                              `json:"current_library_index"`
	TotalLibraries       int                              `json:"total_libraries"`
	CompletedLibraries   int                              `json:"completed_libraries"`
	FailedLibraries      int                              `json:"failed_libraries"`
	CurrentPhase         string                           `json:"current_phase,omitempty"`
	CurrentDone          int                              `json:"current_done"`
	CurrentTotal         int                              `json:"current_total"`
	CurrentPercent       float64                          `json:"current_percent"`
	LowResourceMode      bool                             `json:"low_resource_mode"`
	AggressiveMode       bool                             `json:"aggressive_mode"`
	ExitAfterComplete    bool                             `json:"exit_after_complete"`
	StartedAt            string                           `json:"started_at,omitempty"`
	UpdatedAt            string                           `json:"updated_at,omitempty"`
	FinishedAt           string                           `json:"finished_at,omitempty"`
	ElapsedSeconds       int64                            `json:"elapsed_seconds"`
	Libraries            []LibraryBatchBuildLibraryStatus `json:"libraries,omitempty"`
	Error                string                           `json:"error,omitempty"`
}

type LibraryBatchBuildHooks struct {
	Status               func(cfg *config.Config, profile *config.Profile, username string) LibraryBatchBuildStatus
	Start                func(cfg *config.Config, profile *config.Profile, username string, userID int64, aggressive bool, buildThumbnailsAfterScan bool, completeEXIF bool, moveLegacyThumbnails bool, cleanThumbnailFiles bool, buildPlaybackCaches bool, cleanFiles bool) (LibraryBatchBuildStatus, error)
	Cancel               func(cfg *config.Config, username string) (LibraryBatchBuildStatus, error)
	SetExitAfterComplete func(cfg *config.Config, username string, enabled bool) (LibraryBatchBuildStatus, error)
	SetSelection         func(cfg *config.Config, username string, selectedLibraryIDs []string, selectedPaths []string) (LibraryBatchBuildStatus, error)
}

type libraryBatchBuildExitRequest struct {
	Enabled bool `json:"enabled"`
}

type libraryBatchBuildStartRequest struct {
	Aggressive               bool `json:"aggressive"`
	BuildThumbnailsAfterScan bool `json:"build_thumbnails_after_scan"`
	CompleteEXIF             bool `json:"complete_exif"`
	CleanFiles               bool `json:"clean_files"`
	MoveLegacyThumbnails     bool `json:"move_legacy_thumbnails"`
	CleanThumbnailFiles      bool `json:"clean_thumbnail_files"`
	BuildPlaybackCaches      bool `json:"build_playback_caches"`
}

type libraryBatchBuildSelectionRequest struct {
	SelectedLibraryIDs []string `json:"selected_library_ids"`
	SelectedPaths      []string `json:"selected_paths"`
}

func handleGetLibraryBatchBuildStatus(cfg *config.Config, hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Status == nil {
			c.JSON(http.StatusOK, LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量扫描任务"})
			return
		}
		profile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		status := hooks.Status(cfg, profile, currentUsername(c))
		if status.Status == "completed" {
			refreshCompletedLibraryMetrics(cfg, status.Libraries)
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleStartLibraryBatchBuild(cfg *config.Config, hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Start == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量扫描资源库"})
			return
		}
		var req libraryBatchBuildStartRequest
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
				return
			}
		}
		if req.MoveLegacyThumbnails || req.CleanThumbnailFiles || req.BuildPlaybackCaches {
			c.JSON(http.StatusBadRequest, gin.H{"error": "批量工作流的迁移、旧文件清理和播放缓存阶段已暂停"})
			return
		}
		profile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		status, err := hooks.Start(cfg, profile, currentUsername(c), userID, req.Aggressive, req.BuildThumbnailsAfterScan, req.CompleteEXIF, req.MoveLegacyThumbnails, req.CleanThumbnailFiles, req.BuildPlaybackCaches, req.CleanFiles)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleCancelLibraryBatchBuild(cfg *config.Config, hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Cancel == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消批量扫描"})
			return
		}
		status, err := hooks.Cancel(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleSetLibraryBatchBuildExitAfterComplete(cfg *config.Config, hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.SetExitAfterComplete == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量扫描完成后退出"})
			return
		}
		var req libraryBatchBuildExitRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		status, err := hooks.SetExitAfterComplete(cfg, currentUsername(c), req.Enabled)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleSetLibraryBatchBuildSelection(cfg *config.Config, hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.SetSelection == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量扫描勾选"})
			return
		}
		var req libraryBatchBuildSelectionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		status, err := hooks.SetSelection(cfg, currentUsername(c), req.SelectedLibraryIDs, req.SelectedPaths)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}
