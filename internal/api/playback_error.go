package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/storage"
)

const playbackErrorLogName = "playback-errors.log"

var playbackErrorLogMu sync.Mutex

type browserPlaybackErrorRequest struct {
	Code          int     `json:"code"`
	CodeName      string  `json:"code_name"`
	Message       string  `json:"message"`
	ReadyState    int     `json:"ready_state"`
	NetworkState  int     `json:"network_state"`
	CurrentTime   float64 `json:"current_time"`
	Duration      float64 `json:"duration"`
	VideoWidth    int     `json:"video_width"`
	VideoHeight   int     `json:"video_height"`
	CanPlayMIME   string  `json:"can_play_mime_type"`
	SourceURLPath string  `json:"source_url_path"`
}

type playbackErrorLogEntry struct {
	Timestamp time.Time            `json:"timestamp"`
	Event     string               `json:"event"`
	User      playbackErrorUser    `json:"user"`
	Client    playbackErrorClient  `json:"client"`
	Server    playbackErrorServer  `json:"server"`
	Library   playbackErrorLibrary `json:"library"`
	Media     playbackErrorMedia   `json:"media"`
	Failure   playbackErrorFailure `json:"failure"`
}

type playbackErrorUser struct {
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

type playbackErrorClient struct {
	RemoteIP       string                       `json:"remote_ip,omitempty"`
	UserAgent      string                       `json:"user_agent,omitempty"`
	AcceptLanguage string                       `json:"accept_language,omitempty"`
	Playback       *browserPlaybackErrorRequest `json:"playback,omitempty"`
}

type playbackErrorServer struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

type playbackErrorLibrary struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type playbackErrorMedia struct {
	ID             int64                  `json:"id"`
	UUID           string                 `json:"uuid"`
	OriginalName   string                 `json:"original_name"`
	Kind           string                 `json:"kind"`
	MIMEType       string                 `json:"mime_type"`
	DatabaseSize   int64                  `json:"database_size_bytes"`
	Width          int                    `json:"width"`
	Height         int                    `json:"height"`
	DurationMS     int64                  `json:"duration_ms"`
	VideoCodec     string                 `json:"video_codec,omitempty"`
	FrameRate      float64                `json:"frame_rate,omitempty"`
	SourceRelPath  string                 `json:"source_relative_path,omitempty"`
	StorageRelPath string                 `json:"storage_relative_path,omitempty"`
	FilePath       string                 `json:"server_file_path,omitempty"`
	File           playbackErrorFileState `json:"server_file"`
}

type playbackErrorFileState struct {
	Exists  bool      `json:"exists"`
	Size    int64     `json:"size_bytes,omitempty"`
	Mode    string    `json:"mode,omitempty"`
	ModTime time.Time `json:"modified_at,omitempty"`
	Error   string    `json:"stat_error,omitempty"`
}

type playbackErrorFailure struct {
	Message       string `json:"message,omitempty"`
	SourceURLPath string `json:"source_url_path,omitempty"`
}

func handleReportPlaybackError(cfg *config.Config, registrar videoRegistrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqCfg := requestConfig(c, cfg)
		reqRegistrar := requestRegistrar(c, registrar)
		if reqRegistrar == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "媒体服务未配置"})
			return
		}

		photoID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
		if err != nil || photoID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "媒体编号无效"})
			return
		}
		userID, err := currentLibraryUserID(c, reqCfg)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		photo, err := reqRegistrar.GetPhoto(photoID, userID)
		if err != nil || photo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
			return
		}
		if photo.MediaKind != storage.MediaKindVideo {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持上报视频播放错误"})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var report browserPlaybackErrorRequest
		if err := c.ShouldBindJSON(&report); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "播放错误报告格式无效"})
			return
		}
		report.CodeName = trimPlaybackLogText(report.CodeName, 128)
		report.Message = trimPlaybackLogText(report.Message, 2048)
		report.CanPlayMIME = trimPlaybackLogText(report.CanPlayMIME, 32)
		report.SourceURLPath = safePlaybackURLPath(report.SourceURLPath)
		if report.CurrentTime < 0 || report.CurrentTime > 1e12 {
			report.CurrentTime = 0
		}
		if report.Duration < 0 || report.Duration > 1e12 {
			report.Duration = 0
		}

		entry := newPlaybackErrorLogEntry(c, reqCfg, reqRegistrar, photo, "browser_playback_error")
		entry.Client.Playback = &report
		entry.Failure.SourceURLPath = report.SourceURLPath
		if report.Message != "" {
			entry.Failure.Message = report.Message
		}
		if err := appendPlaybackErrorLog(reqCfg, entry); err != nil {
			writePlaybackErrorFallback(entry, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "无法写入播放错误日志"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func newPlaybackErrorLogEntry(c *gin.Context, cfg *config.Config, registrar videoRegistrar, photo *storage.Photo, event string) playbackErrorLogEntry {
	entry := playbackErrorLogEntry{
		Timestamp: time.Now().UTC(),
		Event:     event,
		User: playbackErrorUser{
			Username: trimPlaybackLogText(currentUsername(c), 256),
			Role:     currentUserRole(cfg, currentUsername(c)),
		},
		Client: playbackErrorClient{
			RemoteIP:       trimPlaybackLogText(c.ClientIP(), 128),
			UserAgent:      trimPlaybackLogText(c.GetHeader("User-Agent"), 1024),
			AcceptLanguage: trimPlaybackLogText(c.GetHeader("Accept-Language"), 512),
		},
		Server: playbackErrorServer{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			GoVersion: runtime.Version(),
		},
		Library: playbackErrorLibrary{
			ID:   cfg.ActiveLibraryID,
			Path: cfg.StoragePath,
		},
		Media: playbackErrorMedia{
			ID:             photo.ID,
			UUID:           photo.UUID,
			OriginalName:   photo.OriginalName,
			Kind:           photo.MediaKind,
			MIMEType:       photo.MimeType,
			DatabaseSize:   photo.Size,
			Width:          photo.Width,
			Height:         photo.Height,
			DurationMS:     photo.DurationMS,
			SourceRelPath:  photo.SourceRelPath,
			StorageRelPath: photo.StorageRelPath,
		},
	}
	for _, library := range cfg.Libraries {
		if (cfg.ActiveLibraryID != "" && library.ID == cfg.ActiveLibraryID) ||
			(library.Path != "" && config.NormalizeStoragePath(library.Path) == config.NormalizeStoragePath(cfg.StoragePath)) {
			entry.Library.ID = library.ID
			entry.Library.Name = library.Name
			entry.Library.Path = library.Path
			break
		}
	}
	if photo.EXIF != nil {
		entry.Media.VideoCodec = photo.EXIF.VideoCodec
		entry.Media.FrameRate = photo.EXIF.VideoFrameRate
	}
	if registrar != nil {
		mediaPath := strings.TrimSpace(registrar.MediaPath(photo))
		if mediaPath != "" {
			if absolutePath, err := filepath.Abs(mediaPath); err == nil {
				mediaPath = absolutePath
			}
			entry.Media.FilePath = mediaPath
			info, err := os.Stat(mediaPath)
			if err != nil {
				entry.Media.File.Error = err.Error()
			} else {
				entry.Media.File.Exists = info.Mode().IsRegular()
				entry.Media.File.Size = info.Size()
				entry.Media.File.Mode = info.Mode().String()
				entry.Media.File.ModTime = info.ModTime().UTC()
			}
		}
	}
	return entry
}

func safePlaybackURLPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return trimPlaybackLogText(raw, 2048)
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return trimPlaybackLogText(path, 2048)
}

func trimPlaybackLogText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func appendPlaybackErrorLog(cfg *config.Config, entry playbackErrorLogEntry) error {
	if cfg == nil || strings.TrimSpace(cfg.AppDataDir) == "" {
		return fmt.Errorf("application data directory is not configured")
	}
	path := filepath.Join(cfg.AppDataDir, playbackErrorLogName)
	playbackErrorLogMu.Lock()
	defer playbackErrorLogMu.Unlock()
	if err := os.MkdirAll(cfg.AppDataDir, 0700); err != nil {
		return fmt.Errorf("create application data directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open playback error log: %w", err)
	}
	encodeErr := json.NewEncoder(file).Encode(entry)
	if encodeErr == nil {
		encodeErr = file.Sync()
	}
	closeErr := file.Close()
	if encodeErr != nil {
		return fmt.Errorf("write playback error log: %w", encodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close playback error log: %w", closeErr)
	}
	return nil
}

func writePlaybackErrorFallback(entry playbackErrorLogEntry, cause error) {
	report, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		log.Printf("EchoGallery playback error log could not be written: %v (also failed to encode report: %v)", cause, err)
		return
	}
	log.Printf("EchoGallery playback error log could not be written: %v\nFull report:\n%s", cause, report)
}

func logServerPlaybackFailure(c *gin.Context, cfg *config.Config, registrar videoRegistrar, photo *storage.Photo, event string, cause error) {
	entry := newPlaybackErrorLogEntry(c, cfg, registrar, photo, event)
	if cause != nil {
		entry.Failure.Message = cause.Error()
	}
	if err := appendPlaybackErrorLog(cfg, entry); err != nil {
		writePlaybackErrorFallback(entry, err)
	}
}
