package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type LibraryBatchBuildLibraryStatus struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Imported  int    `json:"imported"`
	Skipped   int    `json:"skipped"`
	Pruned    int    `json:"pruned"`
	Generated int    `json:"generated"`
	Failed    int    `json:"failed"`
}

type LibraryBatchBuildStatus struct {
	Status              string                           `json:"status"`
	Message             string                           `json:"message"`
	CurrentLibraryName  string                           `json:"current_library_name,omitempty"`
	CurrentLibraryPath  string                           `json:"current_library_path,omitempty"`
	CurrentLibraryIndex int                              `json:"current_library_index"`
	TotalLibraries      int                              `json:"total_libraries"`
	CompletedLibraries  int                              `json:"completed_libraries"`
	FailedLibraries     int                              `json:"failed_libraries"`
	CurrentPhase        string                           `json:"current_phase,omitempty"`
	CurrentDone         int                              `json:"current_done"`
	CurrentTotal        int                              `json:"current_total"`
	CurrentPercent      float64                          `json:"current_percent"`
	FastThumbnailBuild  bool                             `json:"fast_thumbnail_build"`
	LowResourceMode     bool                             `json:"low_resource_mode"`
	ExitAfterComplete   bool                             `json:"exit_after_complete"`
	StartedAt           string                           `json:"started_at,omitempty"`
	UpdatedAt           string                           `json:"updated_at,omitempty"`
	FinishedAt          string                           `json:"finished_at,omitempty"`
	ElapsedSeconds      int64                            `json:"elapsed_seconds"`
	Libraries           []LibraryBatchBuildLibraryStatus `json:"libraries,omitempty"`
	Error               string                           `json:"error,omitempty"`
}

type LibraryBatchBuildHooks struct {
	Status               func() LibraryBatchBuildStatus
	Start                func(cfg *config.Config, profile *config.Profile, userID int64) (LibraryBatchBuildStatus, error)
	Cancel               func() (LibraryBatchBuildStatus, error)
	SetExitAfterComplete func(enabled bool) (LibraryBatchBuildStatus, error)
}

type libraryBatchBuildExitRequest struct {
	Enabled bool `json:"enabled"`
}

func handleGetLibraryBatchBuildStatus(hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Status == nil {
			c.JSON(http.StatusOK, LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量构建任务"})
			return
		}
		c.JSON(http.StatusOK, hooks.Status())
	}
}

func handleStartLibraryBatchBuild(cfg *config.Config, hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Start == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量构建资源库"})
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
		status, err := hooks.Start(cfg, profile, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleCancelLibraryBatchBuild(hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Cancel == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消批量构建"})
			return
		}
		status, err := hooks.Cancel()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleSetLibraryBatchBuildExitAfterComplete(hooks LibraryBatchBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.SetExitAfterComplete == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量构建完成后退出"})
			return
		}
		var req libraryBatchBuildExitRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		status, err := hooks.SetExitAfterComplete(req.Enabled)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}
