package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type LibraryBatchThumbnailBuildHooks struct {
	Status               func(profile *config.Profile, username string) LibraryBatchBuildStatus
	Start                func(cfg *config.Config, profile *config.Profile, username string, userID int64) (LibraryBatchBuildStatus, error)
	Cancel               func(cfg *config.Config, username string) (LibraryBatchBuildStatus, error)
	SetExitAfterComplete func(cfg *config.Config, username string, enabled bool) (LibraryBatchBuildStatus, error)
	SetSelection         func(cfg *config.Config, username string, selectedPaths []string) (LibraryBatchBuildStatus, error)
}

func handleGetLibraryBatchThumbnailBuildStatus(cfg *config.Config, hooks LibraryBatchThumbnailBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Status == nil {
			c.JSON(http.StatusOK, LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量缩略图任务"})
			return
		}
		profile, ok := requestProfile(c, cfg)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, hooks.Status(profile, currentUsername(c)))
	}
}

func handleStartLibraryBatchThumbnailBuild(cfg *config.Config, hooks LibraryBatchThumbnailBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Start == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量构建全部资源库缩略图"})
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
		status, err := hooks.Start(cfg, profile, currentUsername(c), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func handleCancelLibraryBatchThumbnailBuild(cfg *config.Config, hooks LibraryBatchThumbnailBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.Cancel == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消批量缩略图任务"})
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

func handleSetLibraryBatchThumbnailBuildExitAfterComplete(cfg *config.Config, hooks LibraryBatchThumbnailBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.SetExitAfterComplete == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量缩略图完成后退出"})
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

func handleSetLibraryBatchThumbnailBuildSelection(cfg *config.Config, hooks LibraryBatchThumbnailBuildHooks) gin.HandlerFunc {
	return func(c *gin.Context) {
		if hooks.SetSelection == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量缩略图勾选"})
			return
		}
		var req libraryBatchBuildSelectionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
			return
		}
		status, err := hooks.SetSelection(cfg, currentUsername(c), req.SelectedPaths)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}
