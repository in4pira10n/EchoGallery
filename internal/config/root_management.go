package config

import (
	"fmt"
	"os"
	"strings"
)

type ManagedUserUpdate struct {
	NewUsername       string
	NewPassword       string
	Role              string
	AllowedLibraryIDs []string
	DefaultLibraryID  string
}

func UpdateManagedUser(cfg *Config, username string, update ManagedUserUpdate) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config cannot be nil")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf("username cannot be empty")
	}
	user, index := FindUser(cfg, username)
	if user == nil || index < 0 {
		return "", fmt.Errorf("user %q does not exist", username)
	}

	finalUsername := strings.TrimSpace(user.Username)
	nextRole := NormalizeUserRole(update.Role)
	if nextRole == "" {
		nextRole = NormalizeUserRole(user.Role)
	}
	if UserIsRoot(user) {
		finalUsername = "root"
		nextRole = UserRoleRoot
	} else if strings.TrimSpace(update.NewUsername) != "" {
		finalUsername = strings.TrimSpace(update.NewUsername)
		if strings.EqualFold(finalUsername, "root") {
			return "", fmt.Errorf("username %q is reserved", finalUsername)
		}
		for otherIndex, other := range cfg.Users {
			if otherIndex == index {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(other.Username), finalUsername) {
				return "", fmt.Errorf("username %q already exists", finalUsername)
			}
		}
	}
	if nextRole == UserRoleRoot && !UserIsRoot(user) {
		return "", fmt.Errorf("root role is reserved")
	}

	nextHash := strings.TrimSpace(cfg.Users[index].PasswordHash)
	if strings.TrimSpace(update.NewPassword) != "" {
		hash, err := hashPassword(update.NewPassword)
		if err != nil {
			return "", err
		}
		nextHash = hash
	}

	rollbackProfileMove := func() {}
	if !strings.EqualFold(finalUsername, user.Username) {
		rollback, err := moveUserProfileDir(cfg, user.Username, finalUsername)
		if err != nil {
			return "", err
		}
		rollbackProfileMove = rollback
	}

	cfg.Users[index].Username = finalUsername
	cfg.Users[index].PasswordHash = nextHash
	cfg.Users[index].Role = nextRole

	if strings.EqualFold(strings.TrimSpace(cfg.ActiveProfile), user.Username) {
		cfg.ActiveProfile = finalUsername
	}

	switch nextRole {
	case UserRoleRoot:
		cfg.Users[index].AllowedLibraryIDs = nil
		cfg.Users[index].DefaultLibraryID = ""
	case UserRoleAdmin:
		cfg.Users[index].AllowedLibraryIDs = nil
		cfg.Users[index].DefaultLibraryID = strings.TrimSpace(update.DefaultLibraryID)
	default:
		cfg.Users[index].AllowedLibraryIDs = append([]string(nil), update.AllowedLibraryIDs...)
		cfg.Users[index].DefaultLibraryID = strings.TrimSpace(update.DefaultLibraryID)
	}
	cfg.normalizeUserLibraries()

	profile, err := EnsureProfile(cfg, finalUsername)
	if err != nil {
		rollbackProfileMove()
		return "", err
	}
	if selected, ok := cfg.ResolveUserLibrarySelection(finalUsername, profile.ActiveLibraryID, profile.StoragePath); ok {
		profile.ActiveLibraryID = selected.ID
		profile.StoragePath = selected.Path
	} else {
		profile.ActiveLibraryID = ""
		profile.StoragePath = ""
	}
	if err := SaveProfile(cfg, finalUsername, profile); err != nil {
		rollbackProfileMove()
		return "", err
	}
	if err := cfg.Save(); err != nil {
		rollbackProfileMove()
		return "", fmt.Errorf("failed to save config: %w", err)
	}
	return finalUsername, nil
}

func DeleteManagedUser(cfg *Config, username string) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	user, _ := FindUser(cfg, username)
	if UserIsRoot(user) {
		return fmt.Errorf("root user cannot be deleted")
	}
	if len(cfg.Users) <= 1 {
		return fmt.Errorf("at least one user must remain; cannot delete the last user")
	}
	targetIndex := -1
	targetUsername := ""
	for index, item := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(item.Username), username) {
			targetIndex = index
			targetUsername = item.Username
			break
		}
	}
	if targetIndex < 0 {
		return fmt.Errorf("user %q does not exist", username)
	}
	cfg.Users = append(cfg.Users[:targetIndex], cfg.Users[targetIndex+1:]...)
	if strings.EqualFold(strings.TrimSpace(cfg.ActiveProfile), targetUsername) {
		cfg.ActiveProfile = ""
		cfg.ensureActiveProfileAssigned()
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	profileDir, err := cfg.ProfileDir(targetUsername)
	if err == nil {
		_ = os.RemoveAll(profileDir)
	}
	return nil
}
