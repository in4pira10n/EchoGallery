package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", fmt.Errorf("password must be at least 6 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to generate password hash: %w", err)
	}
	return string(hash), nil
}

func newUser(username, password string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, fmt.Errorf("username cannot be empty")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	return User{Username: username, PasswordHash: hash}, nil
}

// AddUser adds a new user to the config file.
func AddUser(username, password string) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("username cannot be empty")
	}

	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return fmt.Errorf("config file not found; start EchoGallery and complete initialization first")
		}
		return err
	}

	for _, u := range cfg.Users {
		if u.Username == username {
			return fmt.Errorf("username %q already exists", username)
		}
	}

	user, err := newUser(username, password)
	if err != nil {
		return err
	}

	cfg.Users = append(cfg.Users, user)

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// RegisterUser creates a new user in an existing config and initializes a separate profile.
func RegisterUser(cfg *Config, username, password string) (*User, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}
	for _, u := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(u.Username), username) {
			return nil, fmt.Errorf("username %q already exists", username)
		}
	}
	user, err := newUser(username, password)
	if err != nil {
		return nil, err
	}
	cfg.Users = append(cfg.Users, user)
	if err := cfg.Save(); err != nil {
		cfg.Users = cfg.Users[:len(cfg.Users)-1]
		return nil, fmt.Errorf("failed to save config: %w", err)
	}
	profile := NewUserProfileTemplate(cfg)
	if err := SaveProfile(cfg, user.Username, profile); err != nil {
		cfg.Users = cfg.Users[:len(cfg.Users)-1]
		_ = cfg.Save()
		return nil, fmt.Errorf("failed to initialize user profile: %w", err)
	}
	return &user, nil
}

func DeleteUser(username string) error {
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
	if len(cfg.Users) <= 1 {
		return fmt.Errorf("at least one user must remain; cannot delete the last user")
	}

	targetIndex := -1
	targetUsername := ""
	for index, user := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(user.Username), username) {
			targetIndex = index
			targetUsername = user.Username
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

// UpdateUserCredentials updates a user's username and/or password, moving the profile directory when renamed.
func UpdateUserCredentials(username, newUsername, newPassword string) (string, error) {
	username = strings.TrimSpace(username)
	newUsername = strings.TrimSpace(newUsername)
	newPassword = strings.TrimSpace(newPassword)
	if username == "" {
		return "", fmt.Errorf("username cannot be empty")
	}
	if newUsername == "" && newPassword == "" {
		return "", fmt.Errorf("nothing to update")
	}

	cfg, err := Load()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return "", fmt.Errorf("config file not found; start EchoGallery and complete initialization first")
		}
		return "", err
	}

	targetIndex := -1
	targetUsername := ""
	for index, user := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(user.Username), username) {
			targetIndex = index
			targetUsername = user.Username
			break
		}
	}
	if targetIndex < 0 {
		return "", fmt.Errorf("user %q does not exist", username)
	}

	finalUsername := targetUsername
	if newUsername != "" {
		finalUsername = newUsername
		for index, user := range cfg.Users {
			if index != targetIndex && strings.EqualFold(strings.TrimSpace(user.Username), finalUsername) {
				return "", fmt.Errorf("username %q already exists", finalUsername)
			}
		}
	}

	nextHash := cfg.Users[targetIndex].PasswordHash
	if newPassword != "" {
		hash, err := hashPassword(newPassword)
		if err != nil {
			return "", err
		}
		nextHash = hash
	}

	rollbackProfileMove := func() {}
	if finalUsername != targetUsername {
		rollback, err := moveUserProfileDir(cfg, targetUsername, finalUsername)
		if err != nil {
			return "", err
		}
		rollbackProfileMove = rollback
	}

	cfg.Users[targetIndex].Username = finalUsername
	cfg.Users[targetIndex].PasswordHash = nextHash
	if strings.EqualFold(strings.TrimSpace(cfg.ActiveProfile), targetUsername) {
		cfg.ActiveProfile = finalUsername
	}

	if err := cfg.Save(); err != nil {
		rollbackProfileMove()
		return "", fmt.Errorf("failed to save config: %w", err)
	}
	return finalUsername, nil
}

func moveUserProfileDir(cfg *Config, oldUsername, newUsername string) (func(), error) {
	oldDir, err := cfg.ProfileDir(oldUsername)
	if err != nil {
		return nil, err
	}
	newDir, err := cfg.ProfileDir(newUsername)
	if err != nil {
		return nil, err
	}
	if oldDir == newDir {
		return func() {}, nil
	}
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return func() {}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to inspect old profile: %w", err)
	}
	if _, err := os.Stat(newDir); err == nil {
		return nil, fmt.Errorf("target user profile already exists: %s", newDir)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to inspect target profile: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0755); err != nil {
		return nil, fmt.Errorf("failed to create profile parent directory: %w", err)
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return nil, fmt.Errorf("failed to move profile: %w", err)
	}
	return func() {
		_ = os.Rename(newDir, oldDir)
	}, nil
}

// VerifyPassword verifies a user's password and returns the matching user.
func VerifyPassword(cfg *Config, username, password string) (*User, error) {
	for _, u := range cfg.Users {
		if u.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
				return nil, fmt.Errorf("incorrect password")
			}
			return &u, nil
		}
	}
	return nil, fmt.Errorf("user does not exist")
}

// RunAddUserWizard runs the command-line adduser wizard.
func RunAddUserWizard() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Add new user")
	fmt.Println("-----------------------------------")

	username, err := promptString(reader, "Username", "")
	if err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	fmt.Print("Password: ")
	password, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password = strings.TrimSpace(password)

	if err := AddUser(username, password); err != nil {
		return err
	}

	fmt.Printf("User %q added successfully\n", username)
	return nil
}

func RunDeleteUserWizard(username string) error {
	if err := DeleteUser(username); err != nil {
		return err
	}
	fmt.Printf("User %q deleted successfully\n", strings.TrimSpace(username))
	return nil
}

func RunModifyUserWizard(username string) error {
	reader := bufio.NewReader(os.Stdin)
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	fmt.Println("Modify user")
	fmt.Println("-----------------------------------")
	fmt.Printf("Current user: %s\n", username)
	nextUsername, err := promptString(reader, "New username (leave empty to keep current)", "")
	if err != nil {
		return err
	}
	fmt.Print("New password (leave empty to keep current): ")
	nextPassword, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	nextPassword = strings.TrimSpace(nextPassword)

	finalUsername, err := UpdateUserCredentials(username, nextUsername, nextPassword)
	if err != nil {
		return err
	}
	fmt.Printf("User %q updated successfully\n", finalUsername)
	return nil
}
