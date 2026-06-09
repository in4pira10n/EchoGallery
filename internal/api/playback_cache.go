package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/service"
)

type playbackCacheManager interface {
	ListPlaybackCaches(userID int64) ([]service.PlaybackCacheEntry, error)
	DeletePlaybackCaches(userID int64, uuids []string) (int, error)
	StartPlaybackCacheBuild(userID int64) service.PlaybackCacheBuildStatus
	GetPlaybackCacheBuildStatus(userID int64) service.PlaybackCacheBuildStatus
	CancelPlaybackCacheBuild(userID int64) service.PlaybackCacheBuildStatus
}

type playbackCacheDeleteRequest struct {
	UUIDs []string `json:"uuids"`
}

func handleListPlaybackCaches(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(playbackCacheManager)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持播放缓存管理"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		entries, err := manager.ListPlaybackCaches(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": entries})
	}
}

func handleDeletePlaybackCaches(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(playbackCacheManager)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持播放缓存管理"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		var req playbackCacheDeleteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		deleted, err := manager.DeletePlaybackCaches(userID, req.UUIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": deleted})
	}
}

func handleGetPlaybackCacheBuildStatus(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(playbackCacheManager)
		if !ok {
			c.JSON(http.StatusOK, service.PlaybackCacheBuildStatus{Status: "idle", Message: "当前没有播放兼容缓存任务"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, manager.GetPlaybackCacheBuildStatus(userID))
	}
}

func handleStartPlaybackCacheBuild(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(playbackCacheManager)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持播放缓存构建"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, manager.StartPlaybackCacheBuild(userID))
	}
}

func handleCancelPlaybackCacheBuild(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, ok := registrar.(playbackCacheManager)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持取消播放缓存构建"})
			return
		}
		userID, err := currentUserID(cfg, currentUsername(c))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, manager.CancelPlaybackCacheBuild(userID))
	}
}
