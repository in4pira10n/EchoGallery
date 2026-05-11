package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/service"
)

type thumbnailBuildManager interface {
	StartThumbnailBuild(userID int64) (service.ThumbnailBuildStatus, error)
	GetThumbnailBuildStatus(userID int64) service.ThumbnailBuildStatus
	CancelThumbnailBuild(userID int64) (service.ThumbnailBuildStatus, error)
}

type thumbnailBuildStatusResponse struct {
	Status         string  `json:"status"`
	Message        string  `json:"message"`
	Done           int     `json:"done"`
	Total          int     `json:"total"`
	Generated      int     `json:"generated"`
	Skipped        int     `json:"skipped"`
	Failed         int     `json:"failed"`
	StartedAt      string  `json:"started_at,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
	FinishedAt     string  `json:"finished_at,omitempty"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
	ETASeconds     int64   `json:"eta_seconds"`
	Percent        float64 `json:"percent"`
	Error          string  `json:"error,omitempty"`
}

func buildThumbnailBuildStatusResponse(status service.ThumbnailBuildStatus) thumbnailBuildStatusResponse {
	return thumbnailBuildStatusResponse{
		Status:         status.Status,
		Message:        status.Message,
		Done:           status.Done,
		Total:          status.Total,
		Generated:      status.Generated,
		Skipped:        status.Skipped,
		Failed:         status.Failed,
		StartedAt:      formatThumbnailBuildTime(status.StartedAt),
		UpdatedAt:      formatThumbnailBuildTime(status.UpdatedAt),
		FinishedAt:     formatThumbnailBuildTime(status.FinishedAt),
		ElapsedSeconds: status.ElapsedSeconds,
		ETASeconds:     status.ETASeconds,
		Percent:        status.Percent,
		Error:          status.Error,
	}
}

func formatThumbnailBuildTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func handleGetThumbnailBuildStatus(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(thumbnailBuildManager)
		if !ok || manager == nil {
			c.JSON(http.StatusOK, thumbnailBuildStatusResponse{Status: "idle", Message: "当前没有缩略图任务"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildThumbnailBuildStatusResponse(manager.GetThumbnailBuildStatus(userID)))
	}
}

func handleStartThumbnailBuild(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(thumbnailBuildManager)
		if !ok || manager == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持批量生成缩略图"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		status, err := manager.StartThumbnailBuild(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildThumbnailBuildStatusResponse(status))
	}
}

func handleCancelThumbnailBuild(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(thumbnailBuildManager)
		if !ok || manager == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消批量缩略图任务"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		status, err := manager.CancelThumbnailBuild(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildThumbnailBuildStatusResponse(status))
	}
}
