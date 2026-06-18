package config

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	profilesDirName = "profiles"
	profileFileName = "profile.json"
)

// Profile 保存某个用户独立的资源库与偏好设置。
type Profile struct {
	ActiveLibraryID string         `json:"active_library_id,omitempty"`
	StoragePath     string         `json:"storage_path"`
	Libraries       []Library      `json:"libraries,omitempty"` // legacy: migrated into global config
	ThumbnailDir    string         `json:"thumbnail_dir"`
	ThumbnailSize   int            `json:"thumbnail_size"`
	TrashDir        string         `json:"trash_dir"`
	UseSystemPlayer bool           `json:"use_system_player"`
	Preferences     Preferences    `json:"preferences"`
	BatchScan       BatchTaskState `json:"batch_scan,omitempty"`       // legacy: migrated into global config
	BatchThumbnails BatchTaskState `json:"batch_thumbnails,omitempty"` // legacy: migrated into global config
}

type BatchTaskLibraryState struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Imported  int    `json:"imported"`
	Skipped   int    `json:"skipped"`
	Pruned    int    `json:"pruned"`
	Generated int    `json:"generated"`
	Failed    int    `json:"failed"`
}

type BatchTaskState struct {
	SelectedLibraryIDs   []string                `json:"selected_library_ids,omitempty"`
	SelectedPaths        []string                `json:"selected_paths,omitempty"`
	SelectionConfigured  bool                    `json:"selection_configured,omitempty"`
	Status               string                  `json:"status"`
	Message              string                  `json:"message"`
	MoveLegacyThumbnails bool                    `json:"move_legacy_thumbnails,omitempty"`
	CleanThumbnailFiles  bool                    `json:"clean_thumbnail_files,omitempty"`
	BuildPlaybackCaches  bool                    `json:"build_playback_caches,omitempty"`
	CurrentLibraryID     string                  `json:"current_library_id,omitempty"`
	CurrentLibraryName   string                  `json:"current_library_name,omitempty"`
	CurrentLibraryPath   string                  `json:"current_library_path,omitempty"`
	CurrentLibraryIndex  int                     `json:"current_library_index"`
	TotalLibraries       int                     `json:"total_libraries"`
	CompletedLibraries   int                     `json:"completed_libraries"`
	FailedLibraries      int                     `json:"failed_libraries"`
	CurrentPhase         string                  `json:"current_phase,omitempty"`
	CurrentDone          int                     `json:"current_done"`
	CurrentTotal         int                     `json:"current_total"`
	CurrentPercent       float64                 `json:"current_percent"`
	LowResourceMode      bool                    `json:"low_resource_mode"`
	AggressiveMode       bool                    `json:"aggressive_mode"`
	ExitAfterComplete    bool                    `json:"exit_after_complete"`
	StartedAt            string                  `json:"started_at,omitempty"`
	UpdatedAt            string                  `json:"updated_at,omitempty"`
	FinishedAt           string                  `json:"finished_at,omitempty"`
	Error                string                  `json:"error,omitempty"`
	Libraries            []BatchTaskLibraryState `json:"libraries,omitempty"`
}

type profileDefaultsProbe struct {
	Preferences struct {
		LowResourceMode *bool `json:"low_resource_mode"`
	} `json:"preferences"`
}

type persistedProfile struct {
	ActiveLibraryID string      `json:"active_library_id,omitempty"`
	StoragePath     string      `json:"storage_path"`
	ThumbnailDir    string      `json:"thumbnail_dir"`
	ThumbnailSize   int         `json:"thumbnail_size"`
	TrashDir        string      `json:"trash_dir"`
	UseSystemPlayer bool        `json:"use_system_player"`
	Preferences     Preferences `json:"preferences"`
}

// ProfileSlug 将用户名转换为稳定且不可越权的目录名。
func ProfileSlug(username string) string {
	trimmed := strings.TrimSpace(username)
	lower := strings.ToLower(trimmed)
	var b strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == '@' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	base := strings.Trim(b.String(), ".-_")
	if base == "" {
		base = "user"
	}
	sum := sha1.Sum([]byte(trimmed))
	return fmt.Sprintf("%s-%s", base, hex.EncodeToString(sum[:4]))
}

func (c *Config) ProfileDir(username string) (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, profilesDirName, ProfileSlug(username)), nil
}

func (c *Config) ProfilePath(username string) (string, error) {
	dir, err := c.ProfileDir(username)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, profileFileName), nil
}

func (c *Config) ProfileAvatarPath(username string) (string, error) {
	dir, err := c.ProfileDir(username)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "avatar.png"), nil
}

func DefaultProfileFromConfig(cfg *Config) *Profile {
	profile := &Profile{
		ActiveLibraryID: cfg.ActiveLibraryID,
		StoragePath:     cfg.StoragePath,
		ThumbnailDir:    cfg.ThumbnailDir,
		ThumbnailSize:   cfg.ThumbnailSize,
		TrashDir:        cfg.TrashDir,
		UseSystemPlayer: cfg.UseSystemPlayer,
		Preferences:     cfg.Preferences,
	}
	profile.applyDefaults(cfg)
	return profile
}

func NewUserProfileTemplate(cfg *Config) *Profile {
	profile := &Profile{}
	template := &Config{}
	if cfg != nil {
		template.AppDataDir = cfg.AppDataDir
	}
	profile.applyDefaults(template)
	profile.Preferences.LowResourceMode = false
	return profile
}

func LoadProfile(cfg *Config, username string) (*Profile, error) {
	path, err := cfg.ProfilePath(username)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}
	var probe profileDefaultsProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse profile defaults: %w", err)
	}
	if probe.Preferences.LowResourceMode == nil {
		profile.Preferences.LowResourceMode = false
	}
	if err := profile.validate(cfg); err != nil {
		return nil, err
	}
	return &profile, nil
}

func EnsureProfile(cfg *Config, username string) (*Profile, error) {
	profile, err := LoadProfile(cfg, username)
	if err == nil {
		if migrateErr := cfg.migrateLegacyProfileLibraries(username, profile); migrateErr != nil {
			return nil, migrateErr
		}
		return profile, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	profile = DefaultProfileFromConfig(cfg)
	if err := SaveProfile(cfg, username, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (c *Config) migrateLegacyProfileLibraries(username string, profile *Profile) error {
	if c == nil || profile == nil || len(profile.Libraries) == 0 {
		return nil
	}
	changedConfig := false
	existingByPath := make(map[string]struct{}, len(c.Libraries))
	existingIDs := make(map[string]struct{}, len(c.Libraries))
	for _, library := range c.Libraries {
		existingByPath[NormalizeStoragePath(library.Path)] = struct{}{}
		if id := strings.TrimSpace(library.ID); id != "" {
			existingIDs[id] = struct{}{}
		}
	}
	for _, library := range profile.Libraries {
		path := strings.TrimSpace(library.Path)
		if path == "" {
			continue
		}
		key := NormalizeStoragePath(path)
		if _, ok := existingByPath[key]; ok {
			continue
		}
		id := ensureLibraryIDForPath(library.ID, path, existingIDs)
		c.Libraries = append(c.Libraries, Library{
			ID:            id,
			Name:          strings.TrimSpace(library.Name),
			Path:          path,
			LogoAsset:     strings.TrimSpace(library.LogoAsset),
			AccentColor:   strings.TrimSpace(library.AccentColor),
			OwnerUsername: c.defaultLibraryOwnerUsername(),
		})
		existingByPath[key] = struct{}{}
		changedConfig = true
	}
	profile.Libraries = nil
	if selected, ok := c.ResolveUserLibrarySelection(username, profile.ActiveLibraryID, profile.StoragePath); ok {
		profile.ActiveLibraryID = selected.ID
		profile.StoragePath = selected.Path
	}
	if changedConfig {
		if err := c.Save(); err != nil {
			return err
		}
	}
	return SaveProfile(c, username, profile)
}

func SaveProfile(cfg *Config, username string, profile *Profile) error {
	if profile == nil {
		return fmt.Errorf("profile cannot be nil")
	}
	next := *profile
	next.Libraries = nil
	next.BatchScan = BatchTaskState{}
	next.BatchThumbnails = BatchTaskState{}
	if err := next.validate(cfg); err != nil {
		return err
	}
	path, err := cfg.ProfilePath(username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create profile directory: %w", err)
	}
	persisted := persistedProfile{
		ActiveLibraryID: next.ActiveLibraryID,
		StoragePath:     next.StoragePath,
		ThumbnailDir:    next.ThumbnailDir,
		ThumbnailSize:   next.ThumbnailSize,
		TrashDir:        next.TrashDir,
		UseSystemPlayer: next.UseSystemPlayer,
		Preferences:     next.Preferences,
	}
	data, err := json.MarshalIndent(&persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize profile: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}
	*profile = next
	return nil
}

func (c *Config) ApplyProfile(profile *Profile) {
	c.ApplyProfileForUser(strings.TrimSpace(c.ActiveProfile), profile)
}

func (c *Config) ApplyProfileForUser(username string, profile *Profile) {
	if profile == nil {
		return
	}
	profile.applyDefaults(c)
	c.ThumbnailDir = profile.ThumbnailDir
	c.ThumbnailSize = profile.ThumbnailSize
	c.TrashDir = profile.TrashDir
	c.UseSystemPlayer = profile.UseSystemPlayer
	c.Preferences = profile.Preferences
	selected, ok := c.ResolveUserLibrarySelection(username, profile.ActiveLibraryID, profile.StoragePath)
	if ok {
		c.ActiveLibraryID = selected.ID
		c.StoragePath = selected.Path
		profile.ActiveLibraryID = selected.ID
		profile.StoragePath = selected.Path
	} else {
		c.ActiveLibraryID = ""
		c.StoragePath = ""
		profile.ActiveLibraryID = ""
		profile.StoragePath = ""
	}
	c.applyDefaults()
}

func (p *Profile) validate(fallback *Config) error {
	p.applyDefaults(fallback)
	return nil
}

func (p *Profile) applyDefaults(fallback *Config) {
	if p == nil {
		return
	}
	if fallback != nil {
		if strings.TrimSpace(p.ThumbnailDir) == "" {
			p.ThumbnailDir = fallback.ThumbnailDir
		}
		if strings.TrimSpace(p.TrashDir) == "" {
			p.TrashDir = fallback.TrashDir
		}
	}
	if fallback != nil && fallback.AppDataDir != "" {
		if strings.TrimSpace(p.ThumbnailDir) == "" {
			p.ThumbnailDir = filepath.Join(fallback.AppDataDir, "thumbnails")
		}
		if strings.TrimSpace(p.TrashDir) == "" {
			p.TrashDir = filepath.Join(fallback.AppDataDir, "Trash")
		}
	}
	if p.ThumbnailSize != 512 {
		p.ThumbnailSize = 512
	}
	if p.Preferences.Theme == "" {
		p.Preferences.Theme = "light"
	}
	if p.Preferences.GridSize < 72 || p.Preferences.GridSize > 260 {
		p.Preferences.GridSize = 180
	}
	if p.Preferences.GridGap == 0 {
		p.Preferences.GridGap = 2
	}
	if p.Preferences.ThumbRadius == 0 {
		p.Preferences.ThumbRadius = 2
	}
	if p.Preferences.SlideshowMode == "" {
		p.Preferences.SlideshowMode = "random"
	}
	if p.Preferences.SlideshowInterval == 0 {
		p.Preferences.SlideshowInterval = 5000
	}
	if p.Preferences.LightboxZoom == 0 {
		p.Preferences.LightboxZoom = 100
	}
	if p.Preferences.VideoSectionMinMinutes == 0 {
		p.Preferences.VideoSectionMinMinutes = 10
	}
	p.normalizeLibraries()
	var libraryScope []Library
	if fallback != nil {
		libraryScope = fallback.Libraries
	}
	if len(libraryScope) == 0 {
		libraryScope = p.Libraries
	}
	p.BatchScan.normalizeAgainstLibraries(libraryScope)
	p.BatchThumbnails.normalizeAgainstLibraries(libraryScope)
}

func (p *Profile) normalizeLibraries() {
	seen := make(map[string]struct{}, len(p.Libraries))
	usedIDs := make(map[string]struct{}, len(p.Libraries))
	normalized := make([]Library, 0, len(p.Libraries))
	for _, lib := range p.Libraries {
		path := strings.TrimSpace(lib.Path)
		if path == "" {
			continue
		}
		key := NormalizeStoragePath(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		name := strings.TrimSpace(lib.Name)
		if name == "" {
			name = defaultLibraryName(len(normalized))
		}
		id := ensureLibraryIDForPath(lib.ID, path, usedIDs)
		normalized = append(normalized, Library{
			ID:          id,
			Name:        name,
			Path:        path,
			LogoAsset:   strings.TrimSpace(lib.LogoAsset),
			AccentColor: strings.TrimSpace(lib.AccentColor),
		})
	}
	p.Libraries = normalized
	activeID := normalizeLibraryID(p.ActiveLibraryID)
	if len(p.Libraries) == 0 {
		p.ActiveLibraryID = activeID
		return
	}
	if activeID != "" {
		for _, lib := range p.Libraries {
			if lib.ID == activeID {
				p.ActiveLibraryID = lib.ID
				p.StoragePath = lib.Path
				return
			}
		}
	}
	p.ActiveLibraryID = activeID
	if strings.TrimSpace(p.StoragePath) == "" && len(p.Libraries) > 0 {
		p.ActiveLibraryID = p.Libraries[0].ID
		p.StoragePath = p.Libraries[0].Path
	}
	if p.StoragePath == "" {
		return
	}
	activeKey := NormalizeStoragePath(p.StoragePath)
	for _, lib := range p.Libraries {
		if NormalizeStoragePath(lib.Path) == activeKey {
			p.ActiveLibraryID = lib.ID
			p.StoragePath = lib.Path
			return
		}
	}
	if len(p.Libraries) > 0 {
		p.ActiveLibraryID = p.Libraries[0].ID
		p.StoragePath = p.Libraries[0].Path
	}
}

func (s *BatchTaskState) normalizeAgainstLibraries(libraries []Library) {
	if s == nil {
		return
	}
	allowed := make(map[string]Library, len(libraries))
	allowedByID := make(map[string]Library, len(libraries))
	for _, library := range libraries {
		path := strings.TrimSpace(library.Path)
		if path == "" {
			continue
		}
		allowed[path] = library
		if id := strings.TrimSpace(library.ID); id != "" {
			allowedByID[id] = library
		}
	}
	if len(s.SelectedLibraryIDs) > 0 {
		selected := make([]string, 0, len(s.SelectedLibraryIDs))
		seen := make(map[string]struct{}, len(s.SelectedLibraryIDs))
		for _, id := range s.SelectedLibraryIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := allowedByID[id]; !ok {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			selected = append(selected, id)
		}
		s.SelectedLibraryIDs = selected
	}
	if len(s.SelectedPaths) > 0 {
		selected := make([]string, 0, len(s.SelectedPaths))
		seen := make(map[string]struct{}, len(s.SelectedPaths))
		for _, path := range s.SelectedPaths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if _, ok := allowed[path]; !ok {
				continue
			}
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			selected = append(selected, path)
		}
		s.SelectedPaths = selected
	}
	if len(s.Libraries) > 0 {
		rows := make([]BatchTaskLibraryState, 0, len(s.Libraries))
		seen := make(map[string]struct{}, len(s.Libraries))
		for _, row := range s.Libraries {
			lib := Library{}
			ok := false
			if id := strings.TrimSpace(row.ID); id != "" {
				lib, ok = allowedByID[id]
			}
			if !ok {
				path := strings.TrimSpace(row.Path)
				lib, ok = allowed[path]
				if !ok {
					continue
				}
			}
			if _, exists := seen[lib.ID]; exists {
				continue
			}
			seen[lib.ID] = struct{}{}
			row.ID = lib.ID
			row.Path = lib.Path
			if strings.TrimSpace(row.Name) == "" {
				row.Name = lib.Name
			}
			rows = append(rows, row)
		}
		s.Libraries = rows
	}
	if len(s.SelectedLibraryIDs) == 0 && len(s.SelectedPaths) == 0 && len(libraries) > 0 {
		s.Status = ""
		s.Message = ""
	}
	if strings.TrimSpace(s.CurrentLibraryID) != "" {
		if lib, ok := allowedByID[strings.TrimSpace(s.CurrentLibraryID)]; ok {
			s.CurrentLibraryID = lib.ID
			s.CurrentLibraryPath = lib.Path
			s.CurrentLibraryName = lib.Name
		}
	}
}
