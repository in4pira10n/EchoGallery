package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type LegacyCleanupSummary struct {
	ProfilesMigrated  int
	ProfilesRewritten int
	ConfigRewritten   bool
}

func MigrateAllLegacyProfiles(cfg *Config) (LegacyCleanupSummary, error) {
	var summary LegacyCleanupSummary
	if cfg == nil {
		return summary, nil
	}
	configChanged := false
	legacyProfiles := make(map[string]*Profile, len(cfg.Users))
	for _, user := range cfg.Users {
		username := user.Username
		if username == "" {
			continue
		}
		profile, err := LoadProfile(cfg, username)
		switch {
		case err == nil:
			legacyProfiles[username] = profile
			legacyCount := len(profile.Libraries)
			if migrateErr := cfg.migrateLegacyProfileLibraries(username, profile); migrateErr != nil {
				return summary, migrateErr
			}
			if legacyCount > 0 {
				summary.ProfilesMigrated++
				summary.ProfilesRewritten++
				configChanged = true
			}
		case os.IsNotExist(err):
			profile, ensureErr := EnsureProfile(cfg, username)
			if ensureErr != nil {
				return summary, ensureErr
			}
			if profile != nil {
				summary.ProfilesRewritten++
			}
		default:
			return summary, fmt.Errorf("failed to load profile for %s: %w", username, err)
		}
	}
	if cfg.migrateLegacyGlobalBatchStates(legacyProfiles) {
		configChanged = true
	}
	if cfg.RefreshLibraryStatuses() {
		configChanged = true
	}
	if cfg.RepairLibraryOwnersFromDatabases() {
		configChanged = true
	}
	summary.ConfigRewritten = configChanged
	return summary, nil
}

func (c *Config) migrateLegacyGlobalBatchStates(profiles map[string]*Profile) bool {
	if c == nil || len(profiles) == 0 {
		return false
	}
	changed := false
	if BatchTaskStateIsEmpty(c.BatchScan) {
		if state, ok := selectLegacyBatchState(c, profiles, func(profile *Profile) BatchTaskState { return profile.BatchScan }); ok {
			c.BatchScan = CloneBatchTaskState(state)
			changed = true
		}
	}
	if BatchTaskStateIsEmpty(c.BatchThumbnails) {
		if state, ok := selectLegacyBatchState(c, profiles, func(profile *Profile) BatchTaskState { return profile.BatchThumbnails }); ok {
			c.BatchThumbnails = CloneBatchTaskState(state)
			changed = true
		}
	}
	return changed
}

func selectLegacyBatchState(c *Config, profiles map[string]*Profile, pick func(*Profile) BatchTaskState) (BatchTaskState, bool) {
	if c == nil || len(profiles) == 0 || pick == nil {
		return BatchTaskState{}, false
	}
	if active := strings.TrimSpace(c.ActiveProfile); active != "" {
		if profile := profiles[active]; profile != nil {
			if state := pick(profile); !BatchTaskStateIsEmpty(state) {
				return state, true
			}
		}
	}
	for _, user := range c.Users {
		profile := profiles[user.Username]
		if profile == nil {
			continue
		}
		state := pick(profile)
		if BatchTaskStateIsEmpty(state) {
			continue
		}
		return state, true
	}
	return BatchTaskState{}, false
}

func CleanupLegacyArtifacts(cfg *Config) (LegacyCleanupSummary, error) {
	summary, err := MigrateAllLegacyProfiles(cfg)
	if err != nil {
		return summary, err
	}
	if cfg == nil {
		return summary, nil
	}
	for _, user := range cfg.Users {
		username := user.Username
		if username == "" {
			continue
		}
		profile, err := EnsureProfile(cfg, username)
		if err != nil {
			return summary, err
		}
		if profile == nil {
			continue
		}
		if err := SaveProfile(cfg, username, profile); err != nil {
			return summary, err
		}
	}
	if summary.ConfigRewritten {
		if err := ensureCleanupDoesNotDropLibraries(cfg); err != nil {
			return summary, err
		}
		if err := cfg.Save(); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func ensureCleanupDoesNotDropLibraries(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read existing config before cleanup: %w", err)
	}
	var persisted persistedConfig
	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("failed to parse existing config before cleanup: %w", err)
	}

	current := normalizeLegacyCleanupLibraries(cfg.Libraries, cfg.Users, cfg.ActiveProfile)
	existing := normalizeLegacyCleanupLibraries(persisted.Libraries, persisted.Users, persisted.ActiveProfile)
	if len(existing) == 0 {
		return nil
	}

	currentIDs := make(map[string]struct{}, len(current))
	currentPaths := make(map[string]struct{}, len(current))
	for _, library := range current {
		if id := normalizeLibraryID(library.ID); id != "" {
			currentIDs[id] = struct{}{}
		}
		if path := NormalizeStoragePath(library.Path); path != "" {
			currentPaths[path] = struct{}{}
		}
	}

	missing := make([]string, 0)
	for _, library := range existing {
		id := normalizeLibraryID(library.ID)
		path := NormalizeStoragePath(library.Path)
		if id != "" {
			if _, ok := currentIDs[id]; ok {
				continue
			}
		}
		if path != "" {
			if _, ok := currentPaths[path]; ok {
				continue
			}
		}
		label := library.Name
		if label == "" {
			label = library.Path
		}
		if label == "" {
			label = library.ID
		}
		missing = append(missing, label)
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to rewrite config during cleanup because libraries would be dropped: %v", missing)
}

func normalizeLegacyCleanupLibraries(libraries []Library, users []User, activeProfile string) []Library {
	tmp := &Config{
		Libraries:     append([]Library(nil), libraries...),
		Users:         append([]User(nil), users...),
		ActiveProfile: activeProfile,
	}
	tmp.normalizeUsers()
	tmp.normalizeLibraries()
	return append([]Library(nil), tmp.Libraries...)
}
