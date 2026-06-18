package config

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCleanupLegacyArtifacts_RefusesToDropLibraries(t *testing.T) {
	dir := t.TempDir()
	restore := overrideConfigPath(filepath.Join(dir, configFileName))
	defer restore()

	libraryAPath := filepath.Join(dir, "library-a")
	libraryBPath := filepath.Join(dir, "library-b")
	for _, path := range []string{libraryAPath, libraryBPath} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("创建测试资源库目录失败: %v", err)
		}
	}

	original := &Config{
		Port:          8080,
		ActiveProfile: "alice",
		JWTSecret:     "testsecret",
		Users: []User{
			{Username: "alice", PasswordHash: "hash", Role: UserRoleAdmin},
		},
		Libraries: []Library{
			{ID: "lib_a", Name: "A", Path: libraryAPath, Status: LibraryStatusMissing},
			{ID: "lib_b", Name: "B", Path: libraryBPath, Status: LibraryStatusMissing},
		},
	}
	if err := original.Save(); err != nil {
		t.Fatalf("保存原始配置失败: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	cfg.Libraries = cfg.Libraries[:1]

	_, err = CleanupLegacyArtifacts(cfg)
	if err == nil {
		t.Fatal("期望拒绝写回丢失资源库的配置，但得到 nil")
	}
	if !strings.Contains(err.Error(), "libraries would be dropped") {
		t.Fatalf("期望返回保护性错误，得到: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("重新加载配置失败: %v", err)
	}
	if len(reloaded.Libraries) != 2 {
		t.Fatalf("磁盘上的配置不应被截断，期望 2 个资源库，得到 %d", len(reloaded.Libraries))
	}
}

func TestCleanupLegacyArtifacts_RewritesConfigWhenRegistryIntact(t *testing.T) {
	dir := t.TempDir()
	restore := overrideConfigPath(filepath.Join(dir, configFileName))
	defer restore()

	libraryPath := filepath.Join(dir, "library")
	if err := os.MkdirAll(libraryPath, 0755); err != nil {
		t.Fatalf("创建测试资源库目录失败: %v", err)
	}

	original := &Config{
		Port:          8080,
		ActiveProfile: "alice",
		JWTSecret:     "testsecret",
		Users: []User{
			{Username: "alice", PasswordHash: "hash", Role: UserRoleAdmin},
		},
		Libraries: []Library{
			{ID: "lib_a", Name: "A", Path: libraryPath, Status: LibraryStatusMissing},
		},
	}
	if err := original.Save(); err != nil {
		t.Fatalf("保存原始配置失败: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	summary, err := CleanupLegacyArtifacts(cfg)
	if err != nil {
		t.Fatalf("期望完整资源库注册表可正常重写，得到错误: %v", err)
	}
	if !summary.ConfigRewritten {
		t.Fatalf("期望本次清理会重写配置")
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("重新加载配置失败: %v", err)
	}
	if len(reloaded.Libraries) != 1 {
		t.Fatalf("期望保留 1 个资源库，得到 %d", len(reloaded.Libraries))
	}
	if reloaded.Libraries[0].Status != LibraryStatusReady {
		t.Fatalf("期望资源库状态刷新为 ready，得到 %s", reloaded.Libraries[0].Status)
	}
}

func TestEnsureCleanupDoesNotDropLibraries_IgnoresMissingConfig(t *testing.T) {
	dir := t.TempDir()
	restore := overrideConfigPath(filepath.Join(dir, configFileName))
	defer restore()

	cfg := &Config{}
	if err := ensureCleanupDoesNotDropLibraries(cfg); err != nil && !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("配置文件不存在时不应报错，得到 %v", err)
	}
}

func TestCleanupLegacyArtifacts_RepairsLibraryOwnerFromDatabase(t *testing.T) {
	dir := t.TempDir()
	restore := overrideConfigPath(filepath.Join(dir, configFileName))
	defer restore()

	libraryPath := filepath.Join(dir, "library")
	if err := os.MkdirAll(libraryPath, 0755); err != nil {
		t.Fatalf("创建测试资源库目录失败: %v", err)
	}

	cfg := &Config{
		Port:          8080,
		ActiveProfile: "Administrator",
		JWTSecret:     "testsecret",
		Users: []User{
			{Username: "admin", PasswordHash: "hash", Role: UserRoleAdmin},
			{Username: "Administrator", PasswordHash: "hash", Role: UserRoleAdmin},
		},
		Libraries: []Library{
			{ID: "lib_a", Name: "A", Path: libraryPath, OwnerUsername: "Administrator"},
		},
	}
	if err := cfg.prepareRuntimePaths(); err != nil {
		t.Fatalf("准备运行目录失败: %v", err)
	}
	dbPath, err := cfg.DatabasePathForStorage(libraryPath)
	if err != nil {
		t.Fatalf("生成数据库路径失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("创建数据库目录失败: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE photos (uploaded_by INTEGER NOT NULL);
		CREATE TABLE albums (created_by INTEGER NOT NULL);
		CREATE TABLE share_links (created_by INTEGER NOT NULL);
		INSERT INTO photos (uploaded_by) VALUES (1), (1), (1);
	`); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("保存原始配置失败: %v", err)
	}

	summary, err := CleanupLegacyArtifacts(cfg)
	if err != nil {
		t.Fatalf("执行清理失败: %v", err)
	}
	if !summary.ConfigRewritten {
		t.Fatalf("期望 owner 修复会触发配置重写")
	}
	if got := cfg.Libraries[0].OwnerUsername; got != "admin" {
		t.Fatalf("期望资源库 owner 修复为 admin，得到 %q", got)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("重新加载配置失败: %v", err)
	}
	if got := reloaded.Libraries[0].OwnerUsername; got != "admin" {
		t.Fatalf("期望磁盘配置中的 owner 修复为 admin，得到 %q", got)
	}
}

func TestCleanupLegacyArtifacts_MigratesLegacyBatchStateToGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	restore := overrideConfigPath(filepath.Join(dir, configFileName))
	defer restore()

	cfg := &Config{
		Port:          8080,
		ActiveProfile: "alice",
		JWTSecret:     "testsecret",
		Users: []User{
			{Username: "alice", PasswordHash: "hash", Role: UserRoleAdmin},
			{Username: "bob", PasswordHash: "hash", Role: UserRoleVisitor},
		},
		Libraries: []Library{
			{ID: "lib_a", Name: "A", Path: filepath.Join(dir, "library-a")},
			{ID: "lib_b", Name: "B", Path: filepath.Join(dir, "library-b")},
		},
	}
	for _, library := range cfg.Libraries {
		if err := os.MkdirAll(library.Path, 0755); err != nil {
			t.Fatalf("创建测试资源库目录失败: %v", err)
		}
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("保存原始配置失败: %v", err)
	}

	aliceProfile := DefaultProfileFromConfig(cfg)
	aliceProfile.BatchScan = BatchTaskState{
		Status:              "running",
		Message:             "Legacy scan",
		SelectedLibraryIDs:  []string{"lib_a"},
		SelectedPaths:       []string{cfg.Libraries[0].Path},
		SelectionConfigured: true,
	}
	if err := SaveProfile(cfg, "alice", aliceProfile); err != nil {
		t.Fatalf("保存 alice Profile 失败: %v", err)
	}
	profilePath, err := cfg.ProfilePath("alice")
	if err != nil {
		t.Fatalf("获取 alice Profile 路径失败: %v", err)
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("读取 alice Profile 失败: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("解析 alice Profile 失败: %v", err)
	}
	decoded["batch_scan"] = map[string]any{
		"status":               "running",
		"message":              "Legacy scan",
		"selected_library_ids": []string{"lib_a"},
		"selected_paths":       []string{cfg.Libraries[0].Path},
		"selection_configured": true,
	}
	legacyData, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatalf("重新编码 legacy profile 失败: %v", err)
	}
	if err := os.WriteFile(profilePath, legacyData, 0644); err != nil {
		t.Fatalf("回写 legacy profile 失败: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	summary, err := CleanupLegacyArtifacts(loaded)
	if err != nil {
		t.Fatalf("执行清理失败: %v", err)
	}
	if !summary.ConfigRewritten {
		t.Fatalf("期望迁移 legacy batch state 会触发配置重写")
	}
	if loaded.BatchScan.Message != "Legacy scan" || len(loaded.BatchScan.SelectedLibraryIDs) != 1 {
		t.Fatalf("期望 legacy batch_scan 迁移到全局 config，得到 %#v", loaded.BatchScan)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("重新加载配置失败: %v", err)
	}
	if reloaded.BatchScan.Message != "Legacy scan" || len(reloaded.BatchScan.SelectedLibraryIDs) != 1 {
		t.Fatalf("期望磁盘配置持久化全局 batch_scan，得到 %#v", reloaded.BatchScan)
	}
	migratedProfile, err := LoadProfile(reloaded, "alice")
	if err != nil {
		t.Fatalf("重新加载 alice Profile 失败: %v", err)
	}
	if !BatchTaskStateIsEmpty(migratedProfile.BatchScan) {
		t.Fatalf("期望 legacy profile batch_scan 被清空，得到 %#v", migratedProfile.BatchScan)
	}
}
