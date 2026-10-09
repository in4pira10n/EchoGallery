package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/storage"
)

type mediaRenamer interface {
	RenameMedia(id int64, userID int64, name string) (*storage.Photo, error)
}

type mediaRenameRequest struct {
	Name string `json:"name"`
}

func handleRenameMedia(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		source := registrar
		if scoped := requestRegistrar(c, nil); scoped != nil {
			source = scoped
		}
		renamer, ok := source.(mediaRenamer)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "媒体重命名服务未配置"})
			return
		}
		userID, err := currentLibraryUserID(c, requestConfig(c, cfg))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "媒体 ID 无效"})
			return
		}
		var req mediaRenameRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		photo, err := renamer.RenameMedia(id, userID, strings.TrimSpace(req.Name))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "媒体已重命名", "data": photo})
	}
}

func handleDeleteMediaPlaybackCache(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		source := registrar
		if scoped := requestRegistrar(c, nil); scoped != nil {
			source = scoped
		}
		manager, ok := source.(playbackCacheManager)
		if !ok {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "播放兼容缓存服务未配置"})
			return
		}
		userID, err := currentLibraryUserID(c, requestConfig(c, cfg))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "媒体 ID 无效"})
			return
		}
		photo, err := registrar.GetPhoto(id, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
			return
		}
		deleted, err := manager.DeletePlaybackCaches(userID, []string{strings.TrimSpace(photo.UUID)})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": deleted})
	}
}
