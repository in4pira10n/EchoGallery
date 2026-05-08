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
	StoragePath     string      `json:"storage_path"`
	Libraries       []Library   `json:"libraries,omitempty"`
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
		StoragePath:     cfg.StoragePath,
		Libraries:       append([]Library(nil), cfg.Libraries...),
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
		return nil, fmt.Errorf("解析 Profile 失败: %w", err)
	}
	if err := profile.validate(cfg); err != nil {
		return nil, err
	}
	return &profile, nil
}

func EnsureProfile(cfg *Config, username string) (*Profile, error) {
	profile, err := LoadProfile(cfg, username)
	if err == nil {
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

func SaveProfile(cfg *Config, username string, profile *Profile) error {
	if profile == nil {
		return fmt.Errorf("Profile 不能为空")
	}
	next := *profile
	next.Libraries = append([]Library(nil), profile.Libraries...)
	if err := next.validate(cfg); err != nil {
		return err
	}
	path, err := cfg.ProfilePath(username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建 Profile 目录失败: %w", err)
	}
	data, err := json.MarshalIndent(&next, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Profile 失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入 Profile 失败: %w", err)
	}
	*profile = next
	return nil
}

func (c *Config) ApplyProfile(profile *Profile) {
	if profile == nil {
		return
	}
	profile.applyDefaults(c)
	c.StoragePath = profile.StoragePath
	c.Libraries = append([]Library(nil), profile.Libraries...)
	c.ThumbnailDir = profile.ThumbnailDir
	c.ThumbnailSize = profile.ThumbnailSize
	c.TrashDir = profile.TrashDir
	c.UseSystemPlayer = profile.UseSystemPlayer
	c.Preferences = profile.Preferences
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
	if p.ThumbnailSize < 96 || p.ThumbnailSize > 1024 {
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
}

func (p *Profile) normalizeLibraries() {
	if len(p.Libraries) == 0 && strings.TrimSpace(p.StoragePath) != "" {
		p.Libraries = []Library{{
			Name: defaultLibraryName(0),
			Path: p.StoragePath,
		}}
	}
	seen := make(map[string]struct{}, len(p.Libraries))
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
		normalized = append(normalized, Library{
			Name:        name,
			Path:        path,
			LogoAsset:   strings.TrimSpace(lib.LogoAsset),
			AccentColor: strings.TrimSpace(lib.AccentColor),
		})
	}
	p.Libraries = normalized
	if strings.TrimSpace(p.StoragePath) == "" && len(p.Libraries) > 0 {
		p.StoragePath = p.Libraries[0].Path
	}
	if p.StoragePath == "" {
		return
	}
	activeKey := NormalizeStoragePath(p.StoragePath)
	for _, lib := range p.Libraries {
		if NormalizeStoragePath(lib.Path) == activeKey {
			p.StoragePath = lib.Path
			return
		}
	}
	if len(p.Libraries) > 0 {
		p.StoragePath = p.Libraries[0].Path
	}
}
