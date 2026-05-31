package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileSlug_IsSafeAndStable(t *testing.T) {
	slug := ProfileSlug("../Alice 王")
	if strings.Contains(slug, "..") || strings.ContainsAny(slug, `/\`) {
		t.Fatalf("ProfileSlug 不应包含路径控制字符: %q", slug)
	}
	if slug != ProfileSlug("../Alice 王") {
		t.Fatalf("ProfileSlug 应保持稳定")
	}
}

func TestEnsureProfile_CreatesDefaultFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig()
	cfg.AppDataDir = dir
	cfg.Libraries = []Library{{Name: "家庭相册", Path: "/tmp/photos", AccentColor: "#3366ff"}}
	cfg.ThumbnailDir = filepath.Join(dir, "thumbs")
	cfg.TrashDir = filepath.Join(dir, "trash")
	cfg.ThumbnailSize = 512
	cfg.UseSystemPlayer = true
	cfg.Preferences.GridGap = 2

	profile, err := EnsureProfile(cfg, "alice")
	if err != nil {
		t.Fatalf("创建 Profile 失败: %v", err)
	}
	if profile.StoragePath != cfg.StoragePath {
		t.Fatalf("期望继承 storage_path，得到 %s", profile.StoragePath)
	}
	if !profile.UseSystemPlayer {
		t.Fatalf("期望继承 UseSystemPlayer")
	}
	path, err := cfg.ProfilePath("alice")
	if err != nil {
		t.Fatalf("获取 Profile 路径失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("期望 Profile 文件已写入: %v", err)
	}
}

func TestLoadFromPath_AppliesActiveProfile(t *testing.T) {
	dir := t.TempDir()
	appDataDir := filepath.Join(dir, appDataDirName)
	cfg := validConfig()
	cfg.AppDataDir = appDataDir
	cfg.ActiveProfile = "alice"
	cfg.Libraries = []Library{{Name: "默认", Path: "/tmp/photos"}}
	if err := SaveProfile(cfg, "alice", &Profile{
		StoragePath:   "/tmp/alice-photos",
		Libraries:     []Library{{Name: "Alice", Path: "/tmp/alice-photos"}},
		ThumbnailDir:  filepath.Join(appDataDir, "alice-thumbs"),
		ThumbnailSize: 640,
		TrashDir:      filepath.Join(appDataDir, "alice-trash"),
		Preferences: Preferences{
			GridSize: 200,
			GridGap:  3,
		},
	}); err != nil {
		t.Fatalf("保存 Profile 失败: %v", err)
	}
	path := writeTempConfig(t, dir, cfg)
	oldOverride := configPathOverride
	configPathOverride = path
	defer func() { configPathOverride = oldOverride }()

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if loaded.StoragePath != "/tmp/alice-photos" {
		t.Fatalf("期望使用活动 Profile 的资源库，得到 %s", loaded.StoragePath)
	}
	if loaded.ThumbnailSize != 512 {
		t.Fatalf("期望活动 Profile 的缩略图尺寸被锁定为 512，得到 %d", loaded.ThumbnailSize)
	}
	if loaded.Preferences.GridSize != 200 {
		t.Fatalf("期望使用活动 Profile 的偏好设置，得到 %d", loaded.Preferences.GridSize)
	}
}
