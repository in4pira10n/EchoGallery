package api

import (
	"path/filepath"
	"testing"

	"echogallery/internal/config"
)

func TestPersistRecoveredLibrarySelection_UpdatesActiveProfile(t *testing.T) {
	dir := t.TempDir()
	oldLibrary := filepath.Join(dir, "old-library")
	newLibrary := filepath.Join(dir, "new-library")
	cfg := &config.Config{
		Port:          8080,
		ActiveProfile: "alice",
		StoragePath:   newLibrary,
		Libraries: []config.Library{
			{Name: "旧资源库", Path: oldLibrary},
			{Name: "恢复资源库", Path: newLibrary},
		},
		JWTSecret:  "testsecret",
		Users:      []config.User{{Username: "alice", PasswordHash: "hash"}},
		AppDataDir: filepath.Join(dir, "echogallery-data"),
	}
	if err := config.SaveProfile(cfg, "alice", &config.Profile{
		StoragePath: oldLibrary,
		Libraries:   []config.Library{{Name: "旧资源库", Path: oldLibrary}},
	}); err != nil {
		t.Fatalf("保存旧 Profile 失败: %v", err)
	}

	if err := persistRecoveredLibrarySelection(cfg); err != nil {
		t.Fatalf("同步恢复资源库到 Profile 失败: %v", err)
	}

	profile, err := config.LoadProfile(cfg, "alice")
	if err != nil {
		t.Fatalf("读取 Profile 失败: %v", err)
	}
	if profile.StoragePath != newLibrary {
		t.Fatalf("期望 Profile 切换到恢复资源库，得到 %s", profile.StoragePath)
	}
	if len(profile.Libraries) != 2 {
		t.Fatalf("期望 Profile 保留恢复后的资源库列表，得到 %d 个", len(profile.Libraries))
	}
	if profile.Libraries[1].Path != newLibrary {
		t.Fatalf("期望新增资源库写入 Profile，得到 %+v", profile.Libraries)
	}
	if cfg.ActiveProfile != "alice" {
		t.Fatalf("期望保持 active_profile=alice，得到 %s", cfg.ActiveProfile)
	}
}

func TestPersistRecoveredLibrarySelection_UsesFirstUserWhenActiveProfileMissing(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "library")
	cfg := &config.Config{
		Port:        8080,
		StoragePath: library,
		Libraries:   []config.Library{{Name: "恢复资源库", Path: library}},
		JWTSecret:   "testsecret",
		Users:       []config.User{{Username: "alice", PasswordHash: "hash"}},
		AppDataDir:  filepath.Join(dir, "echogallery-data"),
	}

	if err := persistRecoveredLibrarySelection(cfg); err != nil {
		t.Fatalf("同步恢复资源库到默认用户 Profile 失败: %v", err)
	}

	if cfg.ActiveProfile != "alice" {
		t.Fatalf("期望补全 active_profile=alice，得到 %s", cfg.ActiveProfile)
	}
	profile, err := config.LoadProfile(cfg, "alice")
	if err != nil {
		t.Fatalf("读取 Profile 失败: %v", err)
	}
	if profile.StoragePath != library {
		t.Fatalf("期望 Profile 写入恢复资源库，得到 %s", profile.StoragePath)
	}
}
