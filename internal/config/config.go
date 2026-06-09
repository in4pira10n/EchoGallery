package config

import (
	crand "crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	configFileName     = "config.json"
	appDataDirName     = "echogallery-data"
	databaseFilePrefix = "echogallery-"
)

// configPathOverride 用于测试时覆盖配置文件路径
var configPathOverride string

// User 表示一个用户
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type Workshop struct {
	AppName       string            `json:"app_name"`
	LogoText      string            `json:"logo_text"`
	TabIcon       string            `json:"tab_icon"`
	LogoAsset     string            `json:"logo_asset"`
	FaviconAsset  string            `json:"favicon_asset"`
	IconOverrides map[string]string `json:"icon_overrides,omitempty"`
}

type Preferences struct {
	Theme                         string `json:"theme"`
	GridSize                      int    `json:"grid_size"`
	GridGap                       int    `json:"grid_gap"`
	ThumbRadius                   int    `json:"thumb_radius"`
	SidebarAutoHide               bool   `json:"sidebar_auto_hide"`
	SlideshowMode                 string `json:"slideshow_mode"`
	SlideshowLoop                 bool   `json:"slideshow_loop"`
	SlideshowInterval             int    `json:"slideshow_interval"`
	LightboxZoom                  int    `json:"lightbox_zoom"`
	ExperimentalAutoplayVideo     bool   `json:"experimental_autoplay_video"`
	VideoAutoplayNext             bool   `json:"video_autoplay_next"`
	VideoSectionMinMinutes        int    `json:"video_section_min_minutes"`
	ExperimentalPrefetchNeighbors bool   `json:"experimental_prefetch_neighbors"`
	ExperimentalRestoreLastView   bool   `json:"experimental_restore_last_view"`
	ContinueLastVideoPosition     bool   `json:"continue_last_video_position"`
	LowResourceMode               bool   `json:"low_resource_mode"`
	PlayerKeymap                  string `json:"player_keymap"`
}

type configDefaultsProbe struct {
	Preferences struct {
		Theme                         *string `json:"theme"`
		GridSize                      *int    `json:"grid_size"`
		GridGap                       *int    `json:"grid_gap"`
		ThumbRadius                   *int    `json:"thumb_radius"`
		SidebarAutoHide               *bool   `json:"sidebar_auto_hide"`
		SlideshowMode                 *string `json:"slideshow_mode"`
		SlideshowLoop                 *bool   `json:"slideshow_loop"`
		SlideshowInterval             *int    `json:"slideshow_interval"`
		LightboxZoom                  *int    `json:"lightbox_zoom"`
		ExperimentalAutoplayVideo     *bool   `json:"experimental_autoplay_video"`
		VideoAutoplayNext             *bool   `json:"video_autoplay_next"`
		VideoSectionMinMinutes        *int    `json:"video_section_min_minutes"`
		ExperimentalPrefetchNeighbors *bool   `json:"experimental_prefetch_neighbors"`
		ExperimentalRestoreLastView   *bool   `json:"experimental_restore_last_view"`
		ContinueLastVideoPosition     *bool   `json:"continue_last_video_position"`
		LowResourceMode               *bool   `json:"low_resource_mode"`
		PlayerKeymap                  *string `json:"player_keymap"`
	} `json:"preferences"`
}

type Library struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	LogoAsset   string `json:"logo_asset,omitempty"`
	AccentColor string `json:"accent_color,omitempty"`
}

type persistedConfig struct {
	Port            int          `json:"port"`
	ActiveProfile   string       `json:"active_profile,omitempty"`
	ActiveLibraryID string       `json:"active_library_id,omitempty"`
	StoragePath     string       `json:"storage_path,omitempty"`
	Libraries       []Library    `json:"libraries,omitempty"`
	ThumbnailDir    string       `json:"thumbnail_dir,omitempty"`
	ThumbnailSize   int          `json:"thumbnail_size,omitempty"`
	TrashDir        string       `json:"trash_dir,omitempty"`
	JWTSecret       string       `json:"jwt_secret"`
	UseSystemPlayer bool         `json:"use_system_player,omitempty"`
	Users           []User       `json:"users"`
	Preferences     *Preferences `json:"preferences,omitempty"`
	Workshop        Workshop     `json:"workshop"`
}

// Config 应用配置
type Config struct {
	Port            int         `json:"port"`
	ActiveProfile   string      `json:"active_profile,omitempty"`
	ActiveLibraryID string      `json:"active_library_id,omitempty"`
	StoragePath     string      `json:"storage_path"`
	Libraries       []Library   `json:"libraries,omitempty"`
	ThumbnailDir    string      `json:"thumbnail_dir"`
	ThumbnailSize   int         `json:"thumbnail_size"`
	TrashDir        string      `json:"trash_dir"`
	JWTSecret       string      `json:"jwt_secret"`
	UseSystemPlayer bool        `json:"use_system_player"`
	Users           []User      `json:"users"`
	Preferences     Preferences `json:"preferences"`
	Workshop        Workshop    `json:"workshop"`
	AppDataDir      string      `json:"-"`
}

// configPath 返回配置文件的绝对路径（与可执行程序同级）
func configPath() (string, error) {
	if configPathOverride != "" {
		return configPathOverride, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), configFileName), nil
}

// Load 加载配置文件，文件不存在时返回错误
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return loadFromPath(path)
}

// loadFromPath 从指定路径加载配置文件
func loadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	var probe configDefaultsProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse config defaults: %w", err)
	}
	cfg.applyMissingDefaults(probe)

	if err := cfg.validateGlobal(); err != nil {
		return nil, err
	}
	if err := cfg.prepareRuntimePaths(); err != nil {
		return nil, err
	}
	if err := cfg.applyActiveProfile(); err != nil {
		return nil, err
	}
	if err := cfg.validateRuntime(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyActiveProfile() error {
	active := strings.TrimSpace(c.ActiveProfile)
	if active == "" {
		return nil
	}
	profile, err := LoadProfile(c, active)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load active profile: %w", err)
	}
	c.ApplyProfile(profile)
	return nil
}

// Save 将配置保存到文件
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	return c.saveToPath(path)
}

// saveToPath 将配置保存到指定路径
func (c *Config) saveToPath(path string) error {
	if err := c.prepareRuntimePaths(); err != nil {
		return err
	}
	if err := c.validateGlobal(); err != nil {
		return err
	}
	c.ensureActiveProfileAssigned()
	if err := c.ensureActiveProfilePersisted(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.persisted(), "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// validate 校验配置合法性
func (c *Config) validate() error {
	if err := c.validateGlobal(); err != nil {
		return err
	}
	return c.validateRuntime()
}

func (c *Config) validateGlobal() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	c.normalizeLibraries()
	c.ThumbnailDir = strings.TrimSpace(c.ThumbnailDir)
	if c.ThumbnailSize != 512 {
		c.ThumbnailSize = 512
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("jwt_secret cannot be empty")
	}
	return nil
}

func (c *Config) validateRuntime() error {
	if strings.TrimSpace(c.StoragePath) == "" && len(c.Libraries) == 0 {
		return nil
	}
	if strings.TrimSpace(c.StoragePath) == "" {
		return fmt.Errorf("storage_path cannot be empty")
	}
	return nil
}

func (c *Config) prepareRuntimePaths() error {
	if c.AppDataDir != "" {
		c.applyDefaults()
		return nil
	}
	dir, err := defaultAppDataDir()
	if err != nil {
		return err
	}
	c.AppDataDir = dir
	c.applyDefaults()
	return nil
}

func (c *Config) applyDefaults() {
	c.normalizeLibraries()
	if c.TrashDir == "" && c.AppDataDir != "" {
		c.TrashDir = filepath.Join(c.AppDataDir, "Trash")
	}
	if c.ThumbnailDir == "" && c.AppDataDir != "" {
		c.ThumbnailDir = filepath.Join(c.AppDataDir, "thumbnails")
	}
	if c.ThumbnailSize != 512 {
		c.ThumbnailSize = 512
	}
	if c.Preferences.Theme == "" {
		c.Preferences.Theme = "light"
	}
	if c.Preferences.GridSize < 72 || c.Preferences.GridSize > 260 {
		c.Preferences.GridSize = 180
	}
	if c.Preferences.GridGap == 0 {
		c.Preferences.GridGap = 2
	}
	if c.Preferences.ThumbRadius == 0 {
		c.Preferences.ThumbRadius = 2
	}
	if c.Preferences.SlideshowMode == "" {
		c.Preferences.SlideshowMode = "random"
	}
	if c.Preferences.SlideshowInterval == 0 {
		c.Preferences.SlideshowInterval = 5000
	}
	if c.Preferences.LightboxZoom == 0 {
		c.Preferences.LightboxZoom = 100
	}
	if c.Preferences.VideoSectionMinMinutes == 0 {
		c.Preferences.VideoSectionMinMinutes = 10
	}
}

func (c *Config) ensureActiveProfileAssigned() {
	if strings.TrimSpace(c.ActiveProfile) != "" {
		return
	}
	for _, user := range c.Users {
		username := strings.TrimSpace(user.Username)
		if username != "" {
			c.ActiveProfile = username
			return
		}
	}
}

func (c *Config) ensureActiveProfilePersisted() error {
	active := strings.TrimSpace(c.ActiveProfile)
	if active == "" {
		return nil
	}
	path, err := c.ProfilePath(active)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect active profile: %w", err)
	}
	if strings.TrimSpace(c.StoragePath) == "" {
		return nil
	}
	profile := DefaultProfileFromConfig(c)
	return SaveProfile(c, active, profile)
}

func (c *Config) persisted() persistedConfig {
	cfg := persistedConfig{
		Port:          c.Port,
		ActiveProfile: strings.TrimSpace(c.ActiveProfile),
		JWTSecret:     c.JWTSecret,
		Users:         append([]User(nil), c.Users...),
		Workshop:      c.Workshop,
	}
	if cfg.ActiveProfile == "" {
		cfg.ActiveLibraryID = strings.TrimSpace(c.ActiveLibraryID)
		cfg.StoragePath = c.StoragePath
		cfg.Libraries = append([]Library(nil), c.Libraries...)
		cfg.ThumbnailDir = c.ThumbnailDir
		cfg.ThumbnailSize = c.ThumbnailSize
		cfg.TrashDir = c.TrashDir
		cfg.UseSystemPlayer = c.UseSystemPlayer
		preferences := c.Preferences
		cfg.Preferences = &preferences
	}
	return cfg
}

func (c *Config) normalizeLibraries() {
	if len(c.Libraries) == 0 && strings.TrimSpace(c.StoragePath) != "" {
		usedIDs := make(map[string]struct{}, 1)
		c.Libraries = []Library{{
			ID:   ensureLibraryIDForPath("", c.StoragePath, usedIDs),
			Name: defaultLibraryName(0),
			Path: c.StoragePath,
		}}
	}

	seen := make(map[string]struct{}, len(c.Libraries))
	usedIDs := make(map[string]struct{}, len(c.Libraries))
	normalized := make([]Library, 0, len(c.Libraries))
	for _, lib := range c.Libraries {
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
	c.Libraries = normalized

	activeID := normalizeLibraryID(c.ActiveLibraryID)
	if activeID != "" {
		for _, lib := range c.Libraries {
			if lib.ID == activeID {
				c.ActiveLibraryID = lib.ID
				c.StoragePath = lib.Path
				return
			}
		}
	}
	c.ActiveLibraryID = activeID
	if strings.TrimSpace(c.StoragePath) == "" && len(c.Libraries) > 0 {
		c.StoragePath = c.Libraries[0].Path
		c.ActiveLibraryID = c.Libraries[0].ID
	}
	if c.StoragePath == "" {
		return
	}
	activeKey := NormalizeStoragePath(c.StoragePath)
	for _, lib := range c.Libraries {
		if NormalizeStoragePath(lib.Path) == activeKey {
			c.ActiveLibraryID = lib.ID
			c.StoragePath = lib.Path
			return
		}
	}
	if len(c.Libraries) > 0 {
		c.ActiveLibraryID = c.Libraries[0].ID
		c.StoragePath = c.Libraries[0].Path
	}
}

func normalizeLibraryID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func newLibraryID() string {
	var raw [8]byte
	if _, err := crand.Read(raw[:]); err != nil {
		sum := sha1.Sum([]byte(fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())))
		return "lib_" + hex.EncodeToString(sum[:6])
	}
	return "lib_" + hex.EncodeToString(raw[:])
}

func libraryIDFromPath(path string) string {
	normalized := NormalizeStoragePath(path)
	if normalized == "" {
		return ""
	}
	sum := sha1.Sum([]byte(normalized))
	return "lib_" + hex.EncodeToString(sum[:6])
}

func ensureLibraryID(id string, used map[string]struct{}) string {
	normalized := normalizeLibraryID(id)
	if normalized != "" {
		if _, exists := used[normalized]; !exists {
			used[normalized] = struct{}{}
			return normalized
		}
	}
	for {
		candidate := newLibraryID()
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func ensureLibraryIDForPath(id string, path string, used map[string]struct{}) string {
	normalized := normalizeLibraryID(id)
	if normalized != "" {
		if _, exists := used[normalized]; !exists {
			used[normalized] = struct{}{}
			return normalized
		}
	}
	candidate := normalizeLibraryID(libraryIDFromPath(path))
	if candidate != "" {
		return reserveLibraryIDCandidate(candidate, used)
	}
	return ensureLibraryID("", used)
}

func reserveLibraryIDCandidate(candidate string, used map[string]struct{}) string {
	candidate = normalizeLibraryID(candidate)
	if candidate == "" {
		return ensureLibraryID("", used)
	}
	if _, exists := used[candidate]; !exists {
		used[candidate] = struct{}{}
		return candidate
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s_%d", candidate, i)
		if _, exists := used[next]; exists {
			continue
		}
		used[next] = struct{}{}
		return next
	}
}

func defaultLibraryName(index int) string {
	return fmt.Sprintf("资源库 %d", index+1)
}

func (c *Config) applyMissingDefaults(probe configDefaultsProbe) {
	c.applyDefaults()
	if probe.Preferences.SlideshowLoop == nil {
		c.Preferences.SlideshowLoop = true
	}
	if probe.Preferences.ExperimentalPrefetchNeighbors == nil {
		c.Preferences.ExperimentalPrefetchNeighbors = true
	}
	if probe.Preferences.ContinueLastVideoPosition == nil {
		c.Preferences.ContinueLastVideoPosition = true
	}
	if probe.Preferences.LowResourceMode == nil {
		c.Preferences.LowResourceMode = false
	}
}

func defaultAppDataDir() (string, error) {
	baseDir, err := defaultAppBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, appDataDirName), nil
}

func defaultAppBaseDir() (string, error) {
	if configPathOverride != "" {
		return filepath.Dir(configPathOverride), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return filepath.Dir(exe), nil
}

// NormalizeStoragePath 将 storage_path 规范化为稳定、可比较的绝对路径。
func NormalizeStoragePath(path string) string {
	trimmed := filepath.Clean(path)
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil {
		trimmed = resolved
	}
	return filepath.Clean(trimmed)
}

func (c *Config) DatabasePath() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return databasePathForPrefix(c.AppDataDir, c.StoragePath), nil
}

func (c *Config) DatabasePathForStorage(storagePath string) (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return databasePathForPrefix(c.AppDataDir, storagePath), nil
}

func databasePathForPrefix(appDataDir, storagePath string) string {
	storageKey := NormalizeStoragePath(storagePath)
	sum := sha1.Sum([]byte(storageKey))
	fileName := databaseFilePrefix + hex.EncodeToString(sum[:8]) + ".db"
	return filepath.Join(appDataDir, "db", fileName)
}

func (c *Config) ManagedDataDir() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, "media"), nil
}

func (c *Config) TrashPath() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return NormalizeStoragePath(c.TrashDir), nil
}

func (c *Config) WorkshopAssetsDir() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, "workshop"), nil
}

func (c *Config) LibraryAssetsDir() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, "libraries"), nil
}

func (c *Config) ThumbnailStoragePath() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return NormalizeStoragePath(c.ThumbnailDir), nil
}

// ErrConfigNotFound 配置文件不存在错误
var ErrConfigNotFound = fmt.Errorf("config file not found")
