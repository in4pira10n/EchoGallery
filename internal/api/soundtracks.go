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

type soundtrackService interface {
	ListVideoSoundtracks(context.Context, int64, int64) ([]media.AudioTrack, error)
	BuildVideoSoundtrack(context.Context, int64, int64, int) (*service.SoundtrackCacheEntry, error)
}

func soundtrackError(c *gin.Context, err error) {
	_ = c.Error(err)
	status, key := http.StatusInternalServerError, "app.lightbox.audioTrackFailed"
	if errors.Is(err, media.ErrProbeUnavailable) || errors.Is(err, media.ErrPlaybackConverterUnavailable) {
		status, key = http.StatusServiceUnavailable, "app.lightbox.audioTrackToolsRequired"
	} else if errors.Is(err, service.ErrSoundtrackNotFound) {
		status, key = http.StatusNotFound, "app.lightbox.audioTrackNone"
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusRequestTimeout
	}
	c.JSON(status, gin.H{"error_key": key, "error": "Soundtrack operation failed"})
}

func handleVideoSoundtracks(cfg *config.Config, registrar videoRegistrar, build bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scopedCfg := requestConfig(c, cfg)
		backend, ok := requestRegistrar(c, registrar).(soundtrackService)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "app.lightbox.audioTrackFailed", "error": "Soundtrack service unavailable"})
			return
		}
		userID, err := currentLibraryUserID(c, scopedCfg)
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
			tracks, err := backend.ListVideoSoundtracks(c.Request.Context(), id, userID)
			if err != nil {
				soundtrackError(c, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"tracks": tracks})
			return
		}
		var input struct {
			Index *int `json:"index" binding:"required,gte=0,lte=1023"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error_key": "app.lightbox.audioTrackFailed", "error": "Invalid soundtrack index"})
			return
		}
		entry, err := backend.BuildVideoSoundtrack(c.Request.Context(), id, userID, *input.Index)
		if err != nil {
			soundtrackError(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}
