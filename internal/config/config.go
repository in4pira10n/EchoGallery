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
	configFileName      = "config.json"
	localUpdateFileName = "local-update.toml"
	appDataDirName      = "echogallery-data"
	databaseFilePrefix  = "echogallery-"
)

// configPathOverride 用于测试时覆盖配置文件路径
var configPathOverride string

const (
	UserRoleAdmin   = "admin"
	UserRoleRoot    = "root"
	UserRoleVisitor = "visitor"
)

// User 表示一个用户
type User struct {
	Username          string   `json:"username"`
	PasswordHash      string   `json:"password_hash"`
	Role              string   `json:"role,omitempty"`
	AllowedLibraryIDs []string `json:"allowed_library_ids,omitempty"`
	DefaultLibraryID  string   `json:"default_library_id,omitempty"`
}

type Workshop struct {
	AppName       string            `json:"app_name"`
	LogoText      string            `json:"logo_text"`
	TabIcon       string            `json:"tab_icon"`
	LogoAsset     string            `json:"logo_asset"`
	FaviconAsset  string            `json:"favicon_asset"`
	IconOverrides map[string]string `json:"icon_overrides,omitempty"`
}

func (c *Config) DisplayName() string {
	if c != nil && strings.TrimSpace(c.Workshop.AppName) != "" {
		return strings.TrimSpace(c.Workshop.AppName)
	}
	return "EchoGallery"
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
	WarmEnabled                   bool   `json:"warm_enabled"`
	LightboxUIIdleSeconds         int    `json:"lightbox_ui_idle_seconds"`
	ThrottledVideoSeek            bool   `json:"throttled_video_seek"`
	VideoSeekThrottleMS           int    `json:"video_seek_throttle_ms"`
	VideoVolumeSwipeSensitivity   int    `json:"video_volume_swipe_sensitivity"`
	VideoVolumeMinPercent         int    `json:"video_volume_min_percent"`
	VideoVolumeMaxPercent         int    `json:"video_volume_max_percent"`
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
		WarmEnabled                   *bool   `json:"warm_enabled"`
		LightboxUIIdleSeconds         *int    `json:"lightbox_ui_idle_seconds"`
		ThrottledVideoSeek            *bool   `json:"throttled_video_seek"`
		VideoSeekThrottleMS           *int    `json:"video_seek_throttle_ms"`
		VideoVolumeSwipeSensitivity   *int    `json:"video_volume_swipe_sensitivity"`
		VideoVolumeMinPercent         *int    `json:"video_volume_min_percent"`
		VideoVolumeMaxPercent         *int    `json:"video_volume_max_percent"`
		LowResourceMode               *bool   `json:"low_resource_mode"`
		PlayerKeymap                  *string `json:"player_keymap"`
	} `json:"preferences"`
}

type Library struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	LogoAsset     string `json:"logo_asset,omitempty"`
	AccentColor   string `json:"accent_color,omitempty"`
	Status        string `json:"status,omitempty"`
	OwnerUsername string `json:"owner_username,omitempty"`
}

type persistedConfig struct {
	Port            int            `json:"port"`
	ActiveProfile   string         `json:"active_profile,omitempty"`
	ActiveLibraryID string         `json:"active_library_id,omitempty"`
	StoragePath     string         `json:"storage_path,omitempty"`
	Libraries       []Library      `json:"libraries,omitempty"`
	BatchScan       BatchTaskState `json:"batch_scan,omitempty"`
	BatchThumbnails BatchTaskState `json:"batch_thumbnails,omitempty"`
	ThumbnailDir    string         `json:"thumbnail_dir,omitempty"`
	ThumbnailSize   int            `json:"thumbnail_size,omitempty"`
	TrashDir        string         `json:"trash_dir,omitempty"`
	JWTSecret       string         `json:"jwt_secret"`
	UseSystemPlayer bool           `json:"use_system_player,omitempty"`
	Users           []User         `json:"users"`
	Preferences     *Preferences   `json:"preferences,omitempty"`
	Workshop        Workshop       `json:"workshop"`
}

// Config 应用配置
type Config struct {
	Port            int            `json:"port"`
	ActiveProfile   string         `json:"active_profile,omitempty"`
	ActiveLibraryID string         `json:"active_library_id,omitempty"`
	StoragePath     string         `json:"storage_path"`
	Libraries       []Library      `json:"libraries,omitempty"`
	BatchScan       BatchTaskState `json:"batch_scan,omitempty"`
	BatchThumbnails BatchTaskState `json:"batch_thumbnails,omitempty"`
	ThumbnailDir    string         `json:"thumbnail_dir"`
	ThumbnailSize   int            `json:"thumbnail_size"`
	TrashDir        string         `json:"trash_dir"`
	JWTSecret       string         `json:"jwt_secret"`
	UseSystemPlayer bool           `json:"use_system_player"`
	Users           []User         `json:"users"`
	Preferences     Preferences    `json:"preferences"`
	Workshop        Workshop       `json:"workshop"`
	AppDataDir      string         `json:"-"`
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

func localUpdatePath() (string, error) {
	if configPathOverride != "" {
		return filepath.Join(filepath.Dir(configPathOverride), localUpdateFileName), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), localUpdateFileName), nil
}

// Load 加载配置文件，文件不存在时返回错误
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return loadFromPath(path)
}

func LocalUpdatePath() (string, error) {
	return localUpdatePath()
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
	c.ApplyProfileForUser(active, profile)
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
	c.normalizeUsers()
	c.normalizeLibraries()
	c.normalizeUserLibraries()
	c.ThumbnailDir = strings.TrimSpace(c.ThumbnailDir)
	if c.ThumbnailSize != 512 {
		c.ThumbnailSize = 512
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("jwt_secret cannot be empty")
	}
	return nil
}

func NormalizeUserRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case UserRoleRoot:
		return UserRoleRoot
	case UserRoleVisitor:
		return UserRoleVisitor
	case UserRoleAdmin:
		return UserRoleAdmin
	default:
		return UserRoleAdmin
	}
}

func (c *Config) normalizeUsers() {
	if len(c.Users) == 0 {
		return
	}
	normalized := make([]User, 0, len(c.Users))
	for _, user := range c.Users {
		username := strings.TrimSpace(user.Username)
		if username == "" {
			continue
		}
		normalized = append(normalized, User{
			Username:          username,
			PasswordHash:      strings.TrimSpace(user.PasswordHash),
			Role:              NormalizeUserRole(user.Role),
			AllowedLibraryIDs: append([]string(nil), user.AllowedLibraryIDs...),
			DefaultLibraryID:  strings.TrimSpace(user.DefaultLibraryID),
		})
	}
	c.Users = normalized
}

func (c *Config) normalizeUserLibraries() {
	allowed := make(map[string]Library, len(c.Libraries))
	allIDs := make([]string, 0, len(c.Libraries))
	for _, library := range c.Libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		allowed[id] = library
		allIDs = append(allIDs, id)
	}
	for index := range c.Users {
		if NormalizeUserRole(c.Users[index].Role) == UserRoleRoot {
			c.Users[index].AllowedLibraryIDs = nil
			c.Users[index].DefaultLibraryID = ""
			continue
		}
		if NormalizeUserRole(c.Users[index].Role) == UserRoleAdmin {
			c.Users[index].AllowedLibraryIDs = append([]string(nil), allIDs...)
			defaultID := normalizeLibraryID(c.Users[index].DefaultLibraryID)
			if defaultID == "" || !containsString(allIDs, defaultID) {
				if len(allIDs) > 0 {
					defaultID = allIDs[0]
				} else {
					defaultID = ""
				}
			}
			c.Users[index].DefaultLibraryID = defaultID
			continue
		}
		selected := make([]string, 0, len(c.Users[index].AllowedLibraryIDs))
		seen := make(map[string]struct{}, len(c.Users[index].AllowedLibraryIDs))
		for _, id := range c.Users[index].AllowedLibraryIDs {
			id = normalizeLibraryID(id)
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
		c.Users[index].AllowedLibraryIDs = selected
		if len(c.Users[index].AllowedLibraryIDs) == 0 {
			if primaryID := c.PrimaryLibraryID(); primaryID != "" {
				c.Users[index].AllowedLibraryIDs = []string{primaryID}
			}
		}
		defaultID := normalizeLibraryID(c.Users[index].DefaultLibraryID)
		if defaultID == "" || !containsString(c.Users[index].AllowedLibraryIDs, defaultID) {
			if len(c.Users[index].AllowedLibraryIDs) > 0 {
				defaultID = c.Users[index].AllowedLibraryIDs[0]
			} else {
				defaultID = ""
			}
		}
		c.Users[index].DefaultLibraryID = defaultID
	}
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
	if c.Preferences.LightboxUIIdleSeconds < 0 || c.Preferences.LightboxUIIdleSeconds > 15 {
		c.Preferences.LightboxUIIdleSeconds = 0
	}
	if c.Preferences.VideoVolumeSwipeSensitivity < 40 || c.Preferences.VideoVolumeSwipeSensitivity > 220 {
		c.Preferences.VideoVolumeSwipeSensitivity = 100
	}
	if c.Preferences.VideoVolumeMinPercent < 0 || c.Preferences.VideoVolumeMinPercent > 100 {
		c.Preferences.VideoVolumeMinPercent = 0
	}
	if c.Preferences.VideoVolumeMaxPercent < 0 || c.Preferences.VideoVolumeMaxPercent > 100 {
		c.Preferences.VideoVolumeMaxPercent = 100
	}
	if c.Preferences.VideoVolumeMaxPercent < c.Preferences.VideoVolumeMinPercent {
		c.Preferences.VideoVolumeMaxPercent = c.Preferences.VideoVolumeMinPercent
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
		Port:            c.Port,
		ActiveProfile:   strings.TrimSpace(c.ActiveProfile),
		Libraries:       append([]Library(nil), c.Libraries...),
		BatchScan:       c.BatchScan,
		BatchThumbnails: c.BatchThumbnails,
		JWTSecret:       c.JWTSecret,
		Users:           append([]User(nil), c.Users...),
		Workshop:        c.Workshop,
	}
	if cfg.ActiveProfile == "" {
		cfg.ActiveLibraryID = strings.TrimSpace(c.ActiveLibraryID)
		cfg.StoragePath = c.StoragePath
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
		accentColor := strings.TrimSpace(lib.AccentColor)
		if normalizeLibraryID(lib.ID) == "" {
			if metadata, err := existingPortableLibraryMetadata(path); err == nil && metadata.ID != "" {
				if strings.TrimSpace(metadata.Name) != "" {
					name = strings.TrimSpace(metadata.Name)
				}
				if strings.TrimSpace(metadata.AccentColor) != "" {
					accentColor = strings.TrimSpace(metadata.AccentColor)
				}
			}
		}
		id := ensureLibraryIDForPath(lib.ID, path, usedIDs)
		normalized = append(normalized, Library{
			ID:            id,
			Name:          name,
			Path:          path,
			LogoAsset:     strings.TrimSpace(lib.LogoAsset),
			AccentColor:   accentColor,
			Status:        normalizeLibraryStatus(strings.TrimSpace(lib.Status)),
			OwnerUsername: c.normalizeLibraryOwnerUsername(strings.TrimSpace(lib.OwnerUsername)),
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

// NormalizeLibraries resolves stable identities and portable metadata before
// callers persist or activate a newly added library.
func (c *Config) NormalizeLibraries() {
	if c == nil {
		return
	}
	c.normalizeLibraries()
	c.normalizeUserLibraries()
}

func (c *Config) normalizeLibraryOwnerUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return c.defaultLibraryOwnerUsername()
	}
	if user, _ := FindUser(c, username); user != nil {
		return user.Username
	}
	return c.defaultLibraryOwnerUsername()
}

func (c *Config) defaultLibraryOwnerUsername() string {
	if user, _ := FindUser(c, c.ActiveProfile); user != nil && UserIsAdmin(user) {
		return user.Username
	}
	for _, user := range c.Users {
		if UserIsAdmin(&user) {
			return user.Username
		}
	}
	for _, user := range c.Users {
		if strings.TrimSpace(user.Username) != "" {
			return user.Username
		}
	}
	return ""
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
	if existing, err := existingPortableLibraryID(path); err == nil && existing != "" {
		return reserveLibraryIDCandidate(existing, used)
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

func (c *Config) PrimaryLibrary() (Library, bool) {
	if c == nil || len(c.Libraries) == 0 {
		return Library{}, false
	}
	return c.Libraries[0], true
}

func (c *Config) PrimaryLibraryID() string {
	library, ok := c.PrimaryLibrary()
	if !ok {
		return ""
	}
	return strings.TrimSpace(library.ID)
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func (c *Config) VisibleLibrariesForUser(username string) []Library {
	user, _ := FindUser(c, username)
	if user == nil {
		return append([]Library(nil), c.Libraries...)
	}
	if UserIsAdmin(user) || UserIsRoot(user) {
		return append([]Library(nil), c.Libraries...)
	}
	if len(c.Libraries) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(user.AllowedLibraryIDs))
	for _, id := range user.AllowedLibraryIDs {
		id = normalizeLibraryID(id)
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		if primary, ok := c.PrimaryLibrary(); ok {
			return []Library{primary}
		}
		return nil
	}
	visible := make([]Library, 0, len(c.Libraries))
	for _, library := range c.Libraries {
		if _, ok := allowed[strings.TrimSpace(library.ID)]; ok {
			visible = append(visible, library)
		}
	}
	return visible
}

func (c *Config) ResolveUserLibrarySelection(username string, activeLibraryID string, storagePath string) (Library, bool) {
	visible := c.VisibleLibrariesForUser(username)
	if len(visible) == 0 {
		return Library{}, false
	}
	activeLibraryID = normalizeLibraryID(activeLibraryID)
	if activeLibraryID != "" {
		for _, library := range visible {
			if library.ID == activeLibraryID {
				return library, true
			}
		}
	}
	activeKey := NormalizeStoragePath(strings.TrimSpace(storagePath))
	if activeKey != "" {
		for _, library := range visible {
			if NormalizeStoragePath(library.Path) == activeKey {
				return library, true
			}
		}
	}
	if user, _ := FindUser(c, username); user != nil {
		defaultID := normalizeLibraryID(user.DefaultLibraryID)
		if defaultID != "" {
			for _, library := range visible {
				if library.ID == defaultID {
					return library, true
				}
			}
		}
	}
	return visible[0], true
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
	if probe.Preferences.WarmEnabled == nil {
		c.Preferences.WarmEnabled = true
	}
	if probe.Preferences.LightboxUIIdleSeconds == nil {
		c.Preferences.LightboxUIIdleSeconds = 0
	}
	if probe.Preferences.ThrottledVideoSeek == nil {
		c.Preferences.ThrottledVideoSeek = false
	}
	if probe.Preferences.VideoSeekThrottleMS == nil {
		c.Preferences.VideoSeekThrottleMS = 240
	}
	if probe.Preferences.VideoVolumeSwipeSensitivity == nil {
		c.Preferences.VideoVolumeSwipeSensitivity = 100
	}
	if probe.Preferences.VideoVolumeMinPercent == nil {
		c.Preferences.VideoVolumeMinPercent = 0
	}
	if probe.Preferences.VideoVolumeMaxPercent == nil {
		c.Preferences.VideoVolumeMaxPercent = 100
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

func DefaultAppDataDir() (string, error) {
	return defaultAppDataDir()
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
	return c.DatabasePathForStorage(c.StoragePath)
}

func (c *Config) DatabasePathForStorage(storagePath string) (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return portableDatabasePath(c.AppDataDir, storagePath)
}

func databasePathForPrefix(appDataDir, storagePath string) string {
	storageKey := NormalizeStoragePath(storagePath)
	sum := sha1.Sum([]byte(storageKey))
	fileName := databaseFilePrefix + hex.EncodeToString(sum[:8]) + ".db"
	return filepath.Join(appDataDir, "db", fileName)
}

// LegacyDatabasePathForStorage resolves the v2.1 server-local media database.
func (c *Config) LegacyDatabasePathForStorage(storagePath string) string {
	if c == nil || strings.TrimSpace(c.AppDataDir) == "" {
		return ""
	}
	return databasePathForPrefix(c.AppDataDir, storagePath)
}

func (c *Config) ManagedDataDir() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, "media"), nil
}

func (c *Config) ManagedDataDirForStorage(storagePath string) (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(LibraryDataRoot(storagePath), "media"), nil
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

func (c *Config) ThumbnailStoragePathForStorage(storagePath string) (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(LibraryDataRoot(storagePath), "thumbnails"), nil
}

func (c *Config) LibraryLockDBPath() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, "db", "library-locks.db"), nil
}

func (c *Config) PageSessionDBPath() (string, error) {
	if err := c.prepareRuntimePaths(); err != nil {
		return "", err
	}
	return filepath.Join(c.AppDataDir, "db", "page-sessions.db"), nil
}

// ErrConfigNotFound 配置文件不存在错误
var ErrConfigNotFound = fmt.Errorf("config file not found")
