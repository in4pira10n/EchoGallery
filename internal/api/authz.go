package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

const rootConsoleUsername = "__root_console__"

func isRootConsoleSession(username string) bool {
	return strings.EqualFold(strings.TrimSpace(username), rootConsoleUsername)
}

func currentUser(cfg *config.Config, username string) (*config.User, int, error) {
	if isRootConsoleSession(username) {
		return &config.User{Username: "root", Role: config.UserRoleRoot}, -1, nil
	}
	user, index := config.FindUser(cfg, username)
	if user == nil || index < 0 {
		return nil, -1, errors.New("用户不存在")
	}
	return user, index, nil
}

func currentUserRole(cfg *config.Config, username string) string {
	user, _, err := currentUser(cfg, username)
	if err != nil {
		return config.UserRoleAdmin
	}
	return config.NormalizeUserRole(user.Role)
}

func isRootRole(role string) bool {
	return config.NormalizeUserRole(role) == config.UserRoleRoot
}

func profileHasVisibleLibraries(cfg *config.Config, username string, profile *config.Profile) bool {
	if cfg == nil || profile == nil {
		return false
	}
	if _, ok := cfg.ResolveUserLibrarySelection(username, profile.ActiveLibraryID, profile.StoragePath); ok {
		return true
	}
	return len(cfg.VisibleLibrariesForUser(username)) > 0
}

func sharedLibraryOwnerUsername(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	candidates := make([]string, 0, len(cfg.Users)+1)
	if active := strings.TrimSpace(cfg.ActiveProfile); active != "" {
		candidates = append(candidates, active)
	}
	for _, user := range cfg.Users {
		if config.NormalizeUserRole(user.Role) == config.UserRoleAdmin {
			candidates = append(candidates, user.Username)
		}
	}
	seen := make(map[string]struct{}, len(candidates))
	adminFallback := ""
	for _, raw := range candidates {
		username := strings.TrimSpace(raw)
		if username == "" {
			continue
		}
		key := strings.ToLower(username)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		user, _, err := currentUser(cfg, username)
		if err != nil || !config.UserIsAdmin(user) {
			continue
		}
		if adminFallback == "" {
			adminFallback = user.Username
		}
		profile, err := config.LoadProfile(cfg, user.Username)
		if err == nil && profileHasVisibleLibraries(cfg, user.Username, profile) {
			return user.Username
		}
		if err != nil && !os.IsNotExist(err) {
			continue
		}
	}
	return adminFallback
}

func currentLibraryOwnerUsername(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	activeLibraryID := strings.TrimSpace(cfg.ActiveLibraryID)
	activePath := config.NormalizeStoragePath(cfg.StoragePath)
	for _, library := range cfg.Libraries {
		if activeLibraryID != "" && strings.TrimSpace(library.ID) == activeLibraryID {
			if owner := strings.TrimSpace(library.OwnerUsername); owner != "" {
				return owner
			}
			break
		}
		if activePath != "" && config.NormalizeStoragePath(library.Path) == activePath {
			if owner := strings.TrimSpace(library.OwnerUsername); owner != "" {
				return owner
			}
			break
		}
	}
	return sharedLibraryOwnerUsername(cfg)
}

func effectiveLibraryUsername(cfg *config.Config, username string) string {
	if owner := currentLibraryOwnerUsername(cfg); strings.TrimSpace(owner) != "" {
		return owner
	}
	if currentUserRole(cfg, username) == config.UserRoleRoot {
		return username
	}
	return username
}

func currentLibraryUserID(c *gin.Context, cfg *config.Config) (int64, error) {
	reqCfg := requestConfig(c, cfg)
	if provider, ok := requestRegistrar(c, nil).(interface{ LibraryUserID(int64) int64 }); ok {
		if owner := provider.LibraryUserID(0); owner > 0 {
			return owner, nil
		}
	}
	return currentUserID(reqCfg, effectiveLibraryUsername(reqCfg, currentUsername(c)))
}

func requireAdmin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentUserRole(cfg, currentUsername(c)) != config.UserRoleAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "访客用户无权执行此操作"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func requireAdminOrRoot(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch currentUserRole(cfg, currentUsername(c)) {
		case config.UserRoleAdmin, config.UserRoleRoot:
			c.Next()
			return
		default:
			c.JSON(http.StatusForbidden, gin.H{"error": "访客用户无权执行此操作"})
			c.Abort()
			return
		}
	}
}

func requireRoot(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentUserRole(cfg, currentUsername(c)) != config.UserRoleRoot {
			c.JSON(http.StatusForbidden, gin.H{"error": "仅 root 可执行此操作"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func requireWritable(cfg *config.Config) gin.HandlerFunc {
	return requireAdmin(cfg)
}

func usernameExists(users []config.User, username string) bool {
	needle := strings.TrimSpace(username)
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Username), needle) {
			return true
		}
	}
	return false
}
