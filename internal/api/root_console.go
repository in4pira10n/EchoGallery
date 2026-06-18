package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type rootUserResponse struct {
	Username           string   `json:"username"`
	Role               string   `json:"role"`
	AllowedLibraryIDs  []string `json:"allowed_library_ids,omitempty"`
	DefaultLibraryID   string   `json:"default_library_id,omitempty"`
	Protected          bool     `json:"protected,omitempty"`
	CanLogin           bool     `json:"can_login"`
	LoginBlockedReason string   `json:"login_blocked_reason,omitempty"`
}

type rootCreateUserRequest struct {
	Username          string   `json:"username"`
	Password          string   `json:"password"`
	Role              string   `json:"role"`
	AllowedLibraryIDs []string `json:"allowed_library_ids,omitempty"`
	DefaultLibraryID  string   `json:"default_library_id,omitempty"`
}

type rootUpdateUserRequest struct {
	NewUsername       string   `json:"new_username,omitempty"`
	NewPassword       string   `json:"new_password,omitempty"`
	Role              string   `json:"role,omitempty"`
	AllowedLibraryIDs []string `json:"allowed_library_ids,omitempty"`
	DefaultLibraryID  string   `json:"default_library_id,omitempty"`
}

func buildRootUserResponses(cfg *config.Config) []rootUserResponse {
	if cfg == nil {
		return nil
	}
	rows := make([]rootUserResponse, 0, len(cfg.Users))
	for _, user := range cfg.Users {
		role := config.NormalizeUserRole(user.Role)
		item := rootUserResponse{
			Username:          user.Username,
			Role:              role,
			AllowedLibraryIDs: append([]string(nil), user.AllowedLibraryIDs...),
			DefaultLibraryID:  strings.TrimSpace(user.DefaultLibraryID),
			Protected:         role == config.UserRoleRoot,
			CanLogin:          true,
		}
		if role == config.UserRoleAdmin {
			item.AllowedLibraryIDs = nil
		}
		if role == config.UserRoleVisitor && len(item.AllowedLibraryIDs) == 0 {
			visible := cfg.VisibleLibrariesForUser(user.Username)
			for _, library := range visible {
				if id := strings.TrimSpace(library.ID); id != "" {
					item.AllowedLibraryIDs = append(item.AllowedLibraryIDs, id)
				}
			}
			if item.DefaultLibraryID == "" && len(item.AllowedLibraryIDs) > 0 {
				item.DefaultLibraryID = item.AllowedLibraryIDs[0]
			}
		}
		if role == config.UserRoleRoot {
			item.CanLogin = false
			item.LoginBlockedReason = "仅可通过 ./EchoGallery root 使用"
		}
		if role == config.UserRoleVisitor && len(cfg.VisibleLibrariesForUser(user.Username)) == 0 {
			item.CanLogin = false
			item.LoginBlockedReason = "等待 root 授权资源库"
		}
		rows = append(rows, item)
	}
	slices.SortFunc(rows, func(a, b rootUserResponse) int {
		rank := func(role string) int {
			switch role {
			case config.UserRoleRoot:
				return 0
			case config.UserRoleAdmin:
				return 1
			default:
				return 2
			}
		}
		if diff := rank(a.Role) - rank(b.Role); diff != 0 {
			return diff
		}
		return strings.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
	return rows
}

func handleListRootUsers(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"users": buildRootUserResponses(cfg)})
	}
}

func handleCreateRootUser(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req rootCreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}
		role := config.NormalizeUserRole(req.Role)
		if role != config.UserRoleAdmin && role != config.UserRoleVisitor {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅可创建管理员或访客账号"})
			return
		}
		user, err := config.RegisterUser(cfg, req.Username, req.Password, role)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if _, err := config.UpdateManagedUser(cfg, user.Username, config.ManagedUserUpdate{
			Role:              role,
			AllowedLibraryIDs: req.AllowedLibraryIDs,
			DefaultLibraryID:  req.DefaultLibraryID,
		}); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已创建",
			"users":   buildRootUserResponses(cfg),
		})
	}
}

func handleUpdateRootUser(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req rootUpdateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
			return
		}
		username := strings.TrimSpace(c.Param("username"))
		role := strings.TrimSpace(req.Role)
		if role != "" {
			role = config.NormalizeUserRole(role)
			if role != config.UserRoleAdmin && role != config.UserRoleVisitor && role != config.UserRoleRoot {
				c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户角色"})
				return
			}
		}
		finalUsername, err := config.UpdateManagedUser(cfg, username, config.ManagedUserUpdate{
			NewUsername:       req.NewUsername,
			NewPassword:       req.NewPassword,
			Role:              role,
			AllowedLibraryIDs: req.AllowedLibraryIDs,
			DefaultLibraryID:  req.DefaultLibraryID,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":  "用户已更新",
			"username": finalUsername,
			"users":    buildRootUserResponses(cfg),
		})
	}
}

func handleDeleteRootUser(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := strings.TrimSpace(c.Param("username"))
		if err := config.DeleteManagedUser(cfg, username); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已删除",
			"users":   buildRootUserResponses(cfg),
		})
	}
}
