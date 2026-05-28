package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/service"
)

type videoThumbnailRefreshManager interface {
	StartVideoThumbnailRefresh(userID int64) (service.VideoThumbnailRefreshStatus, error)
	GetVideoThumbnailRefreshStatus(userID int64) service.VideoThumbnailRefreshStatus
	CancelVideoThumbnailRefresh(userID int64) (service.VideoThumbnailRefreshStatus, error)
}

type videoThumbnailRefreshStatusResponse struct {
	Status         string  `json:"status"`
	Message        string  `json:"message"`
	Done           int     `json:"done"`
	Total          int     `json:"total"`
	Refreshed      int     `json:"refreshed"`
	Failed         int     `json:"failed"`
	StartedAt      string  `json:"started_at,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
	FinishedAt     string  `json:"finished_at,omitempty"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
	ETASeconds     int64   `json:"eta_seconds"`
	Percent        float64 `json:"percent"`
	Error          string  `json:"error,omitempty"`
}

func buildVideoThumbnailRefreshStatusResponse(status service.VideoThumbnailRefreshStatus) videoThumbnailRefreshStatusResponse {
	return videoThumbnailRefreshStatusResponse{
		Status:         status.Status,
		Message:        status.Message,
		Done:           status.Done,
		Total:          status.Total,
		Refreshed:      status.Refreshed,
		Failed:         status.Failed,
		StartedAt:      formatVideoThumbnailRefreshTime(status.StartedAt),
		UpdatedAt:      formatVideoThumbnailRefreshTime(status.UpdatedAt),
		FinishedAt:     formatVideoThumbnailRefreshTime(status.FinishedAt),
		ElapsedSeconds: status.ElapsedSeconds,
		ETASeconds:     status.ETASeconds,
		Percent:        status.Percent,
		Error:          status.Error,
	}
}

func formatVideoThumbnailRefreshTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func handleGetVideoThumbnailRefreshStatus(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(videoThumbnailRefreshManager)
		if !ok || manager == nil {
			c.JSON(http.StatusOK, videoThumbnailRefreshStatusResponse{Status: "idle", Message: "当前没有视频缩略图任务"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildVideoThumbnailRefreshStatusResponse(manager.GetVideoThumbnailRefreshStatus(userID)))
	}
}

func handleStartVideoThumbnailRefresh(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(videoThumbnailRefreshManager)
		if !ok || manager == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持刷新视频缩略图"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		status, err := manager.StartVideoThumbnailRefresh(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildVideoThumbnailRefreshStatusResponse(status))
	}
}

func handleCancelVideoThumbnailRefresh(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(videoThumbnailRefreshManager)
		if !ok || manager == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消视频缩略图刷新"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		status, err := manager.CancelVideoThumbnailRefresh(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildVideoThumbnailRefreshStatusResponse(status))
	}
}
