package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type LibraryBuildStatus struct {
	Status            string  `json:"status"`
	Message           string  `json:"message"`
	Done              int     `json:"done"`
	Total             int     `json:"total"`
	Imported          int     `json:"imported"`
	Skipped           int     `json:"skipped"`
	Pruned            int     `json:"pruned"`
	StartedAt         string  `json:"started_at,omitempty"`
	UpdatedAt         string  `json:"updated_at,omitempty"`
	FinishedAt        string  `json:"finished_at,omitempty"`
	ElapsedSeconds    int64   `json:"elapsed_seconds"`
	ETASeconds        int64   `json:"eta_seconds"`
	Percent           float64 `json:"percent"`
	ExitAfterComplete bool    `json:"exit_after_complete"`
	Error             string  `json:"error,omitempty"`
}

type LibraryBuildHooks struct {
	Status               func() LibraryBuildStatus
	SetExitAfterComplete func(enabled bool) (LibraryBuildStatus, error)
	Cancel               func() (LibraryBuildStatus, error)
}

type libraryBuildExitRequest struct {
	Enabled bool `json:"enabled"`
}

func handleGetLibraryBuildStatus(hooks LibraryBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Status == nil {
			c.JSON(http.StatusOK, LibraryBuildStatus{Status: "idle", Message: "当前没有扫描任务"})
			return
		}
		c.JSON(http.StatusOK, hooks.Status())
	}
}

func handleSetLibraryBuildExitAfterComplete(hooks LibraryBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.SetExitAfterComplete == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持完成后退出"})
			return
		}
		var req libraryBuildExitRequest
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

func handleCancelLibraryBuild(hooks LibraryBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Cancel == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持停止资源库构建"})
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
