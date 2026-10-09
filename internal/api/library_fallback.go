package api

import (
	"errors"
	"net/http"
	"strings"

	"echogallery/internal/config"
	"echogallery/internal/sessionlock"
	"echogallery/internal/storage/sqlite"
	"github.com/gin-gonic/gin"
)

// Prefer the account's starred library, then the remaining accessible libraries.
func handleFallbackLibrary(cfg *config.Config, options RouterOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			LibraryID string `json:"library_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的资源库请求"})
			return
		}
		username, sessionID := currentUsername(c), currentPageSessionID(c)
		profile, err := config.EnsureProfile(cfg, username)
		if err != nil {
			c.JSON(500, gin.H{"error": "加载用户配置失败"})
			return
		}
		if err := options.RuntimeProvider.SyncBeforeRelease(options.LockStore, sessionID, ""); err != nil {
			_ = c.Error(err)
			c.JSON(500, gin.H{"error": "资源库同步失败，未切换"})
			return
		}
		user, _, err := currentUser(cfg, username)
		if err != nil {
			c.JSON(401, gin.H{"error": "用户已失效"})
			return
		}
		visible := cfg.VisibleLibrariesForUser(username)
		for i, library := range visible {
			if library.ID == strings.TrimSpace(user.DefaultLibraryID) {
				visible[0], visible[i] = visible[i], visible[0]
				break
			}
		}
		for _, library := range visible {
			if library.ID == req.LibraryID || config.DetectLibraryStatus(library.Path) == config.LibraryStatusMissing {
				continue
			}
			if options.LockStore != nil {
				if _, err := options.LockStore.Acquire(library, username, currentUserRole(cfg, username), sessionID, "browse"); err != nil {
					if _, ok := err.(*sessionlock.ConflictError); ok {
						continue
					}
					_ = c.Error(err)
					c.JSON(500, gin.H{"error": "检查资源库占用失败"})
					return
				}
			}
			if options.RuntimeProvider != nil {
				if _, err := options.RuntimeProvider.ForLibrary(cfg, library); err != nil {
					if options.LockStore != nil {
						_ = options.LockStore.Release(library.ID, sessionID, "browse")
					}
					if errors.Is(err, sqlite.ErrPortableLibraryOccupied) {
						continue
					}
					_ = c.Error(err)
					continue
				}
			}
			profile.ActiveLibraryID, profile.StoragePath = library.ID, library.Path
			if err := config.SaveProfile(cfg, username, profile); err != nil {
				_ = c.Error(err)
				c.JSON(500, gin.H{"error": "保存资源库选择失败"})
				return
			}
			if options.LockStore != nil {
				_ = options.LockStore.ReleaseScopeForSessionExcept(sessionID, "browse", library.ID)
			}
			options.RuntimeProvider.rememberSession(sessionID, library.ID)
			c.JSON(200, gin.H{"library_id": library.ID, "library_name": library.Name})
			return
		}
		writeStructuredAuthError(c, http.StatusForbidden, "no_accessible_library", "星标资源库及其他资源库均不可用，请稍后重试")
	}
}
