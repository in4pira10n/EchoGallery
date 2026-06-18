package config

import (
	"encoding/json"
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
	cfg.Libraries = []Library{
		{Name: "默认", Path: "/tmp/photos"},
		{Name: "Alice", Path: "/tmp/alice-photos"},
	}
	if err := SaveProfile(cfg, "alice", &Profile{
		StoragePath:   "/tmp/alice-photos",
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

func TestProfileNormalizeLibraries_AssignsStableIDsAndPrefersActiveLibraryID(t *testing.T) {
	profile := &Profile{
		ActiveLibraryID: "LIB_B",
		StoragePath:     "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a"},
			{Name: "资源库 B", Path: "/tmp/library-b", ID: "LIB_B"},
		},
	}

	profile.normalizeLibraries()

	if len(profile.Libraries) != 2 {
		t.Fatalf("期望保留 2 个资源库，得到 %d", len(profile.Libraries))
	}
	if profile.Libraries[0].ID == "" {
		t.Fatalf("期望旧 Profile 资源库自动补齐 library_id")
	}
	if profile.ActiveLibraryID != "lib_b" {
		t.Fatalf("期望优先使用 ActiveLibraryID，得到 %q", profile.ActiveLibraryID)
	}
	if profile.StoragePath != "/tmp/library-b" {
		t.Fatalf("期望 ActiveLibraryID 对应路径成为当前资源库，得到 %q", profile.StoragePath)
	}
}

func TestProfileNormalizeLibraries_LegacyLibraryIDIsDeterministic(t *testing.T) {
	profileA := &Profile{
		StoragePath: "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a"},
		},
	}
	profileB := &Profile{
		StoragePath: "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a"},
		},
	}

	profileA.normalizeLibraries()
	profileB.normalizeLibraries()

	if profileA.Libraries[0].ID == "" {
		t.Fatal("期望旧 Profile 资源库补齐稳定 library_id")
	}
	if profileA.Libraries[0].ID != profileB.Libraries[0].ID {
		t.Fatalf("期望相同路径反复规范化得到同一个 library_id，得到 %q 和 %q", profileA.Libraries[0].ID, profileB.Libraries[0].ID)
	}
}

func TestProfileNormalizeLibraries_StoragePathOnlyDoesNotRebuildLegacyLibraries(t *testing.T) {
	profileA := &Profile{StoragePath: "/tmp/library-a"}
	profileB := &Profile{StoragePath: "/tmp/library-a"}

	profileA.normalizeLibraries()
	profileB.normalizeLibraries()

	if len(profileA.Libraries) != 0 || len(profileB.Libraries) != 0 {
		t.Fatalf("Profile 不应再仅凭 storage_path 自动重建共享资源库，得到 %d 和 %d", len(profileA.Libraries), len(profileB.Libraries))
	}
}

func TestBatchTaskStateNormalizeAgainstLibraries_PrefersLibraryIDs(t *testing.T) {
	libraries := []Library{
		{Name: "资源库 A", Path: "/tmp/library-a-new", ID: "lib_a"},
		{Name: "资源库 B", Path: "/tmp/library-b", ID: "lib_b"},
	}
	state := &BatchTaskState{
		SelectedLibraryIDs: []string{"lib_a"},
		SelectedPaths:      []string{"/tmp/library-a-old"},
		CurrentLibraryID:   "lib_a",
		CurrentLibraryPath: "/tmp/library-a-old",
		CurrentLibraryName: "旧名称",
		Libraries: []BatchTaskLibraryState{
			{ID: "lib_a", Path: "/tmp/library-a-old", Name: "旧名称", Status: "pending"},
		},
	}

	state.normalizeAgainstLibraries(libraries)

	if len(state.SelectedLibraryIDs) != 1 || state.SelectedLibraryIDs[0] != "lib_a" {
		t.Fatalf("期望按 library_id 保留批量选择，得到 %#v", state.SelectedLibraryIDs)
	}
	if state.CurrentLibraryID != "lib_a" {
		t.Fatalf("期望保留当前 library_id，得到 %q", state.CurrentLibraryID)
	}
	if state.CurrentLibraryPath != "/tmp/library-a-new" {
		t.Fatalf("期望按 library_id 同步新的路径，得到 %q", state.CurrentLibraryPath)
	}
	if len(state.Libraries) != 1 || state.Libraries[0].Path != "/tmp/library-a-new" {
		t.Fatalf("期望批量任务行按 library_id 更新路径，得到 %#v", state.Libraries)
	}
}

func TestSaveProfile_StripsLegacyBatchWorkflowState(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig()
	cfg.AppDataDir = dir

	profile := &Profile{
		StoragePath: "/tmp/photos",
		BatchScan: BatchTaskState{
			Status:              "running",
			SelectedLibraryIDs:  []string{"lib_a"},
			SelectedPaths:       []string{"/tmp/photos"},
			SelectionConfigured: true,
		},
		BatchThumbnails: BatchTaskState{
			Status:              "completed",
			SelectedLibraryIDs:  []string{"lib_b"},
			SelectedPaths:       []string{"/tmp/thumbs"},
			SelectionConfigured: true,
		},
	}

	if err := SaveProfile(cfg, "alice", profile); err != nil {
		t.Fatalf("保存 Profile 失败: %v", err)
	}

	path, err := cfg.ProfilePath("alice")
	if err != nil {
		t.Fatalf("获取 Profile 路径失败: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 Profile 失败: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("解析 Profile 失败: %v", err)
	}
	for _, key := range []string{"batch_scan", "batch_thumbnails", "libraries"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("profile.json 不应再保存旧批量字段 %q", key)
		}
	}
}
