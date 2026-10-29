package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"echogallery/internal/config"
	"echogallery/internal/media"
	"echogallery/internal/service"
	"github.com/gin-gonic/gin"
)

type subtitleService interface {
	ListVideoSubtitles(context.Context, int64, int64) ([]media.SubtitleTrack, error)
	BuildVideoSubtitle(context.Context, int64, int64, int) (*service.SubtitleCacheEntry, error)
	VideoSubtitlePath(string, int64, int) (string, error)
}

func handleVideoSubtitles(cfg *config.Config, registrar videoRegistrar, build bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		backend, ok := requestRegistrar(c, registrar).(subtitleService)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "app.lightbox.subtitleFailed"})
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
		if !build {
			tracks, err := backend.ListVideoSubtitles(c.Request.Context(), id, userID)
			if err != nil {
				subtitleAPIError(c, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"tracks": tracks})
			return
		}
		var req struct {
			Index *int `json:"index" binding:"required,gte=-1,lte=1023"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error_key": "app.lightbox.subtitleFailed"})
			return
		}
		entry, err := backend.BuildVideoSubtitle(c.Request.Context(), id, userID, *req.Index)
		if err != nil {
			subtitleAPIError(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}

func subtitleAPIError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	key := "app.lightbox.subtitleFailed"
	if errors.Is(err, service.ErrSubtitleNotFound) {
		status = http.StatusNotFound
		key = "app.lightbox.subtitleNone"
	}
	if errors.Is(err, media.ErrProbeUnavailable) || errors.Is(err, media.ErrPlaybackConverterUnavailable) {
		status = http.StatusServiceUnavailable
		key = "app.lightbox.subtitleToolsRequired"
	}
	c.JSON(status, gin.H{"error_key": key, "error": "Subtitle operation failed"})
}

func handleServeSubtitleFile(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		backend, ok := requestRegistrar(c, registrar).(subtitleService)
		if !ok {
			c.Status(http.StatusNotFound)
			return
		}
		userID, err := currentLibraryUserID(c, requestConfig(c, cfg))
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		index, err := strconv.Atoi(c.Query("index"))
		if err != nil || index < -1 || index > 1023 {
			c.Status(http.StatusNotFound)
			return
		}
		path, err := backend.VideoSubtitlePath(c.Param("uuid"), userID, index)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Content-Type", "text/vtt; charset=utf-8")
		c.Header("Cache-Control", "private, max-age=3600")
		c.File(path)
	}
}
