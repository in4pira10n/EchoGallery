package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

type UserSummary struct {
	Username          string
	Role              string
	AllowedLibraryIDs []string
	DefaultLibraryID  string
}

func normalizeAllowedLibraryIDsForUser(cfg *Config, role string, requested []string) []string {
	allIDs := make([]string, 0, len(cfg.Libraries))
	allowed := make(map[string]struct{}, len(cfg.Libraries))
	for _, library := range cfg.Libraries {
		id := normalizeLibraryID(library.ID)
		if id == "" {
			continue
		}
		allIDs = append(allIDs, id)
		allowed[id] = struct{}{}
	}
	if NormalizeUserRole(role) == UserRoleAdmin {
		return allIDs
	}
	selected := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		id := normalizeLibraryID(raw)
		if id == "" {
			continue
		}
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, id)
	}
	if len(selected) == 0 {
		if primaryID := cfg.PrimaryLibraryID(); primaryID != "" {
			return []string{primaryID}
		}
	}
	return selected
}

func normalizeDefaultLibraryForUser(allowed []string, requested string) string {
	requested = normalizeLibraryID(requested)
	if requested != "" && containsString(allowed, requested) {
		return requested
	}
	if len(allowed) > 0 {
		return allowed[0]
	}
	return ""
}

func applyUserLibraryAccessToProfile(cfg *Config, username string) error {
	profile, err := EnsureProfile(cfg, username)
	if err != nil {
		return err
	}
	if selected, ok := cfg.ResolveUserLibrarySelection(username, profile.ActiveLibraryID, profile.StoragePath); ok {
		profile.ActiveLibraryID = selected.ID
		profile.StoragePath = selected.Path
	} else {
		profile.ActiveLibraryID = ""
		profile.StoragePath = ""
	}
	return SaveProfile(cfg, username, profile)
}

func ListUsersDetailed(cfg *Config) []UserSummary {
	if cfg == nil {
		return nil
	}
	rows := make([]UserSummary, 0, len(cfg.Users))
	for _, user := range cfg.Users {
		rows = append(rows, UserSummary{
			Username:          user.Username,
			Role:              NormalizeUserRole(user.Role),
			AllowedLibraryIDs: append([]string(nil), user.AllowedLibraryIDs...),
			DefaultLibraryID:  user.DefaultLibraryID,
		})
	}
	return rows
}

func SetUserRole(username, role string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("config file not found; start EchoGallery and complete initialization first")
		}
		return err
	}
	user, index := FindUser(cfg, username)
	if user == nil || index < 0 {
		return fmt.Errorf("user %q does not exist", username)
	}
	cfg.Users[index].Role = NormalizeUserRole(role)
	cfg.normalizeUserLibraries()
	if err := applyUserLibraryAccessToProfile(cfg, cfg.Users[index].Username); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

func SetUserLibraryAccess(username string, allowedLibraryIDs []string, defaultLibraryID string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("config file not found; start EchoGallery and complete initialization first")
		}
		return err
	}
	user, index := FindUser(cfg, username)
	if user == nil || index < 0 {
		return fmt.Errorf("user %q does not exist", username)
	}
	role := NormalizeUserRole(cfg.Users[index].Role)
	cfg.Users[index].AllowedLibraryIDs = normalizeAllowedLibraryIDsForUser(cfg, role, allowedLibraryIDs)
	cfg.Users[index].DefaultLibraryID = normalizeDefaultLibraryForUser(cfg.Users[index].AllowedLibraryIDs, defaultLibraryID)
	cfg.normalizeUserLibraries()
	if err := applyUserLibraryAccessToProfile(cfg, cfg.Users[index].Username); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

func RunListUsersCommand() error {
	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("config file not found; start EchoGallery and complete initialization first")
		}
		return err
	}
	fmt.Println("Users")
	fmt.Println("-----------------------------------")
	for _, row := range ListUsersDetailed(cfg) {
		allowed := "none"
		if len(row.AllowedLibraryIDs) > 0 {
			allowed = strings.Join(row.AllowedLibraryIDs, ", ")
		}
		defaultLibrary := row.DefaultLibraryID
		if defaultLibrary == "" {
			defaultLibrary = "(none)"
		}
		fmt.Printf("- %s | role=%s | default=%s | libraries=%s\n", row.Username, row.Role, defaultLibrary, allowed)
	}
	if len(cfg.Libraries) > 0 {
		fmt.Println("")
		fmt.Println("Libraries")
		for _, library := range cfg.Libraries {
			fmt.Printf("- %s | %s | %s\n", library.ID, library.Name, library.Path)
		}
	}
	return nil
}

func RunSetUserRoleWizard(username string) error {
	reader := bufio.NewReader(os.Stdin)
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	fmt.Println("Set user role")
	fmt.Println("-----------------------------------")
	fmt.Printf("User: %s\n", username)
	role, err := promptString(reader, "Role (admin/visitor)", "")
	if err != nil {
		return err
	}
	role = NormalizeUserRole(role)
	if err := SetUserRole(username, role); err != nil {
		return err
	}
	fmt.Printf("User %q role updated to %s\n", username, role)
	return nil
}

func RunSetUserLibrariesWizard(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("config file not found; start EchoGallery and complete initialization first")
		}
		return err
	}
	user, _ := FindUser(cfg, username)
	if user == nil {
		return fmt.Errorf("user %q does not exist", username)
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Set user libraries")
	fmt.Println("-----------------------------------")
	fmt.Printf("User: %s\n", user.Username)
	fmt.Printf("Role: %s\n", NormalizeUserRole(user.Role))
	if len(cfg.Libraries) == 0 {
		return fmt.Errorf("no libraries are configured")
	}
	fmt.Println("Available libraries:")
	for _, library := range cfg.Libraries {
		fmt.Printf("- %s | %s | %s\n", library.ID, library.Name, library.Path)
	}
	if UserIsAdmin(user) {
		fmt.Println("Admins always access all libraries. You can only update the default library here.")
	}
	rawAllowed, err := promptString(reader, "Allowed library IDs (comma separated, leave empty to use primary / all for admin)", strings.Join(user.AllowedLibraryIDs, ","))
	if err != nil {
		return err
	}
	allowed := []string{}
	if UserIsAdmin(user) {
		allowed = nil
	} else {
		for _, part := range strings.Split(rawAllowed, ",") {
			if item := strings.TrimSpace(part); item != "" {
				allowed = append(allowed, item)
			}
		}
	}
	defaultLibrary, err := promptString(reader, "Default library ID", user.DefaultLibraryID)
	if err != nil {
		return err
	}
	if err := SetUserLibraryAccess(user.Username, allowed, defaultLibrary); err != nil {
		return err
	}
	fmt.Printf("User %q library access updated\n", user.Username)
	return nil
}
