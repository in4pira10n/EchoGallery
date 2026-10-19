package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"echogallery/internal/config"
	"echogallery/internal/danmaku"
	"github.com/gin-gonic/gin"
)

type danmakuFetchRequest struct {
	URL       string `json:"url"`
	Page      int    `json:"page"`
	Overwrite bool   `json:"overwrite"`
	MediaID   int64  `json:"media_id"`
	MediaUUID string `json:"media_uuid"`
}

func currentDanmakuRuntime(c *gin.Context, options RouterOptions) (*danmaku.Manager, config.Library, string, error) {
	reqCfg := requestConfig(c, nil)
	if reqCfg == nil {
		return nil, config.Library{}, "", os.ErrNotExist
	}
	library := config.Library{ID: strings.TrimSpace(reqCfg.ActiveLibraryID), Path: strings.TrimSpace(reqCfg.StoragePath)}
	for _, candidate := range reqCfg.Libraries {
		if candidate.ID == library.ID || (library.ID == "" && config.NormalizeStoragePath(candidate.Path) == config.NormalizeStoragePath(library.Path)) {
			library = candidate
			break
		}
	}
	libraryID := strings.TrimSpace(library.ID)
	if options.RuntimeProvider == nil {
		return nil, library, libraryID, nil
	}
	if libraryID == "" || strings.TrimSpace(library.Path) == "" {
		return nil, config.Library{}, "", os.ErrNotExist
	}
	return options.RuntimeProvider.Danmaku(libraryID), library, libraryID, nil
}

func danmakuRoot(library config.Library) string {
	return filepath.Join(config.LibraryDataRoot(library.Path), "danmaku")
}

func handleFetchDanmaku(cfg *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req danmakuFetchRequest
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error_key": "invalid_input", "message_key": "app.danmaku.errors.invalidInput"})
			return
		}
		manager, library, _, err := currentDanmakuRuntime(c, options)
		if err != nil || manager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "library_unavailable", "message_key": "app.danmaku.errors.libraryUnavailable"})
			return
		}
		if err := danmaku.ValidateRoot(danmakuRoot(library)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error_key": "danmaku_storage_failed", "message_key": "app.danmaku.errors.storageFailed"})
			return
		}
		mediaID, mediaUUID := matchDanmakuMedia(c, cfg, req.MediaID, req.MediaUUID)
		task, startErr := manager.Start(context.Background(), danmakuRoot(library), req.URL, req.Page, req.Overwrite, mediaID, mediaUUID)
		if startErr != nil {
			c.JSON(http.StatusConflict, gin.H{"error_key": "danmaku_task_exists", "message_key": "app.danmaku.errors.taskExists", "task": task})
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func handleSearchDanmaku() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL string `json:"url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error_key": "invalid_input", "message_key": "app.danmaku.errors.invalidInput"})
			return
		}
		req.URL = strings.TrimSpace(req.URL)
		client := danmaku.NewClient(nil)
		bvid, _, err := danmaku.ParseBVID(req.URL)
		if err != nil {
			if req.URL != "" && len(req.URL) <= 300 && !strings.Contains(req.URL, "://") {
				items, searchErr := client.Search(c.Request.Context(), req.URL)
				if searchErr != nil {
					c.JSON(http.StatusBadGateway, gin.H{"error_key": "danmaku_search_failed", "message_key": "app.danmaku.errors.searchFailed"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"items": items})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error_key": "invalid_input", "message_key": "app.danmaku.errors.invalidInput"})
			return
		}
		video, err := client.Resolve(c.Request.Context(), bvid)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error_key": "danmaku_lookup_failed", "message_key": "app.danmaku.errors.lookupFailed"})
			return
		}
		c.JSON(http.StatusOK, video)
	}
}

func handleGetDanmakuTask(_ *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, _, _, err := currentDanmakuRuntime(c, options)
		if err != nil || manager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "library_unavailable"})
			return
		}
		task, ok := manager.Snapshot(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error_key": "task_not_found"})
			return
		}
		c.JSON(http.StatusOK, task)
	}
}
func handleCancelDanmakuTask(_ *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, _, _, err := currentDanmakuRuntime(c, options)
		if err != nil || manager == nil || !manager.Cancel(c.Param("id")) {
			c.JSON(http.StatusNotFound, gin.H{"error_key": "task_not_found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": danmaku.StatusCancelled})
	}
}
func handleListDanmaku(_ *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, library, _, err := currentDanmakuRuntime(c, options)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "library_unavailable"})
			return
		}
		items, err := danmaku.ListMetadata(danmakuRoot(library))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error_key": "danmaku_storage_failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}
func handleGetDanmaku(_ *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, library, _, err := currentDanmakuRuntime(c, options)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "library_unavailable"})
			return
		}
		items, err := danmaku.ListMetadata(danmakuRoot(library))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error_key": "danmaku_storage_failed"})
			return
		}
		for _, item := range items {
			if strings.EqualFold(item.BVID, c.Param("bvid")) {
				if item.OwnerAvatar == "" {
					if video, resolveErr := danmaku.NewClient(nil).Resolve(c.Request.Context(), item.BVID); resolveErr == nil {
						item.Owner = video.Owner
						item.OwnerAvatar = video.OwnerAvatar
						if item.Cover == "" {
							item.Cover = video.Cover
						}
						_ = danmaku.WriteMetadata(danmakuRoot(library), item)
					}
				}
				c.JSON(http.StatusOK, item)
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error_key": "danmaku_not_found"})
	}
}
func handleDeleteDanmaku(_ *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, library, _, err := currentDanmakuRuntime(c, options)
		if err != nil || manager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error_key": "library_unavailable"})
			return
		}
		bvid, _, parseErr := danmaku.ParseBVID(c.Param("bvid"))
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error_key": "invalid_input"})
			return
		}
		if err := manager.Delete(danmakuRoot(library), bvid); err != nil {
			if errors.Is(err, danmaku.ErrTaskRunning) {
				c.JSON(http.StatusConflict, gin.H{"message_key": "app.danmaku.errors.matchBusy"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error_key": "danmaku_storage_failed"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func matchDanmakuMedia(c *gin.Context, cfg *config.Config, mediaID int64, mediaUUID string) (int64, string) {
	registrar := requestRegistrar(c, nil)
	if registrar == nil || mediaID <= 0 || strings.TrimSpace(mediaUUID) == "" {
		return 0, ""
	}
	userID, err := currentLibraryUserID(c, cfg)
	if err != nil {
		return 0, ""
	}
	photo, err := registrar.GetPhoto(mediaID, userID)
	if err != nil || photo == nil || !strings.EqualFold(strings.TrimSpace(photo.UUID), strings.TrimSpace(mediaUUID)) || photo.MediaKind != "video" {
		return 0, ""
	}
	return photo.ID, photo.UUID
}

func handleRemoveDanmakuMatch(cfg *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager, library, _, err := currentDanmakuRuntime(c, options)
		registrar := requestRegistrar(c, nil)
		if err != nil || manager == nil || registrar == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"message_key": "app.danmaku.errors.libraryUnavailable"})
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message_key": "app.danmaku.errors.invalidInput"})
			return
		}
		userID, err := currentLibraryUserID(c, cfg)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"message_key": "app.danmaku.errors.removeFailed"})
			return
		}
		photo, err := registrar.GetPhoto(id, userID)
		if err != nil || photo == nil || photo.MediaKind != "video" || photo.UUID == "" {
			c.JSON(http.StatusNotFound, gin.H{"message_key": "app.danmaku.errors.removeFailed"})
			return
		}
		if err := manager.RemoveMatch(danmakuRoot(library), photo.UUID); err != nil {
			if errors.Is(err, danmaku.ErrTaskRunning) {
				c.JSON(http.StatusConflict, gin.H{"message_key": "app.danmaku.errors.matchBusy"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"message_key": "app.danmaku.errors.removeFailed"})
			}
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func handleMediaDanmaku(cfg *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, library, _, err := currentDanmakuRuntime(c, options)
		registrar := requestRegistrar(c, nil)
		if err != nil || registrar == nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		userID, userErr := currentLibraryUserID(c, cfg)
		if err != nil || userErr != nil || id <= 0 {
			c.Status(http.StatusNotFound)
			return
		}
		photo, err := registrar.GetPhoto(id, userID)
		if err != nil || photo == nil || photo.MediaKind != "video" || photo.UUID == "" {
			c.Status(http.StatusNotFound)
			return
		}
		items, err := danmaku.ListMetadata(danmakuRoot(library))
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		for _, item := range items {
			if !strings.EqualFold(item.MediaUUID, photo.UUID) {
				continue
			}
			bvid, _, err := danmaku.ParseBVID(item.BVID)
			if err != nil {
				continue
			}
			// Older records have no selected page. Only infer an unambiguous file.
			if item.MatchedPage == 0 {
				for _, page := range item.Pages {
					if page.Status == "completed" {
						if item.MatchedPage != 0 {
							item.MatchedPage = -1
							break
						}
						item.MatchedPage = page.Page
					}
				}
				if item.MatchedPage <= 0 {
					continue
				}
			}
			for _, page := range item.Pages {
				if item.MatchedPage > 0 && page.Page != item.MatchedPage {
					continue
				}
				if page.File == "" || strings.ContainsAny(page.File, `/\\`) || !strings.HasSuffix(page.File, ".xml") {
					continue
				}
				root, rootErr := filepath.EvalSymlinks(danmakuRoot(library))
				target, pathErr := filepath.EvalSymlinks(filepath.Join(danmakuRoot(library), bvid, page.File))
				if rootErr != nil || pathErr != nil || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
					continue
				}
				file, err := os.Open(target)
				if err != nil {
					continue
				}
				data, readErr := io.ReadAll(io.LimitReader(file, (16<<20)+1))
				file.Close()
				if readErr != nil || len(data) > 16<<20 {
					continue
				}
				data, readErr = danmaku.DecodeXML(data)
				if readErr != nil {
					c.JSON(http.StatusUnprocessableEntity, gin.H{"message_key": "app.danmaku.errors.invalidXML"})
					return
				}
				c.Header("Cache-Control", "no-store")
				c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
				return
			}
		}
		c.Status(http.StatusNoContent)
	}
}
