package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 创建临时配置文件用于测试
func writeTempConfig(t *testing.T, dir string, cfg *Config) string {
	t.Helper()
	path := filepath.Join(dir, configFileName)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("序列化配置失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

func validConfig() *Config {
	return &Config{
		Port:        8080,
		StoragePath: "/tmp/photos",
		JWTSecret:   "testsecret",
		Users:       []User{},
	}
}

// --- loadFromPath 测试 ---

func TestLoadFromPath_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	if err := os.WriteFile(path, []byte(`{
  "port": 8080,
  "storage_path": "/tmp/photos",
  "jwt_secret": "testsecret",
  "users": []
}`), 0644); err != nil {
		t.Fatalf("写入旧版配置失败: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("期望加载成功，得到错误: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("期望 Port=8080，得到 %d", cfg.Port)
	}
	if cfg.StoragePath != "/tmp/photos" {
		t.Errorf("期望 StoragePath=/tmp/photos，得到 %s", cfg.StoragePath)
	}
	if !cfg.Preferences.SlideshowLoop {
		t.Errorf("期望旧配置默认开启 slideshow_loop")
	}
	if cfg.Preferences.SlideshowMode != "random" {
		t.Errorf("期望旧配置默认使用随机幻灯片，得到 %s", cfg.Preferences.SlideshowMode)
	}
	if !cfg.Preferences.ExperimentalPrefetchNeighbors {
		t.Errorf("期望旧配置默认开启 experimental_prefetch_neighbors")
	}
	if !cfg.Preferences.WarmEnabled {
		t.Errorf("期望旧配置默认开启 warm_enabled")
	}
}

func TestLoadFromPath_LegacyUsersDefaultToAdminRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	if err := os.WriteFile(path, []byte(`{
  "port": 8080,
  "storage_path": "/tmp/photos",
  "jwt_secret": "testsecret",
  "users": [
    {"username": "alice", "password_hash": "hash"}
  ]
}`), 0644); err != nil {
		t.Fatalf("写入旧版配置失败: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("期望加载成功，得到错误: %v", err)
	}
	if len(cfg.Users) != 1 {
		t.Fatalf("期望加载 1 个用户，得到 %d", len(cfg.Users))
	}
	if cfg.Users[0].Role != UserRoleAdmin {
		t.Fatalf("期望旧版用户默认迁移为 admin，得到 %s", cfg.Users[0].Role)
	}
}

func TestLoadFromPath_FileNotFound(t *testing.T) {
	_, err := loadFromPath("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
	if err != ErrConfigNotFound {
		t.Errorf("期望 ErrConfigNotFound，得到 %v", err)
	}
}

func TestLoadFromPath_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	if err := os.WriteFile(path, []byte("not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadFromPath(path)
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
}

// --- validate 测试 ---

func TestValidate_InvalidPort(t *testing.T) {
	cases := []int{0, -1, 65536, 99999}
	for _, port := range cases {
		cfg := validConfig()
		cfg.Port = port
		if err := cfg.validate(); err == nil {
			t.Errorf("端口 %d 应该校验失败", port)
		}
	}
}

func TestValidate_EmptyStoragePath(t *testing.T) {
	cfg := validConfig()
	cfg.StoragePath = ""
	if err := cfg.validate(); err != nil {
		t.Errorf("无资源库时空 StoragePath 应允许进入初始化流程，得到错误: %v", err)
	}
}

func TestValidate_EmptyJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSecret = ""
	if err := cfg.validate(); err == nil {
		t.Error("空 JWTSecret 应该校验失败")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := validConfig()
	if err := cfg.validate(); err != nil {
		t.Errorf("有效配置不应该校验失败: %v", err)
	}
}

// --- saveToPath 测试 ---

func TestSaveToPath_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.Libraries = []Library{
		{
			Name:        "家庭相册",
			Path:        "/tmp/photos",
			AccentColor: "#3366ff",
		},
	}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 读回验证
	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("读取刚保存的文件失败: %v", err)
	}
	if loaded.Port != cfg.Port {
		t.Errorf("Port 不一致: 期望 %d，得到 %d", cfg.Port, loaded.Port)
	}
	if loaded.StoragePath != cfg.StoragePath {
		t.Errorf("StoragePath 不一致")
	}
	if len(loaded.Libraries) != 1 {
		t.Fatalf("期望保留 1 个资源库，得到 %d", len(loaded.Libraries))
	}
	if loaded.Libraries[0].AccentColor != "#3366ff" {
		t.Errorf("AccentColor 未正确保存: %s", loaded.Libraries[0].AccentColor)
	}
}

func TestSaveToPath_OmitsProfileFieldsFromConfigJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.AppDataDir = filepath.Join(dir, appDataDirName)
	cfg.ActiveProfile = "alice"
	cfg.Libraries = []Library{{Name: "家庭相册", Path: "/tmp/photos", AccentColor: "#3366ff"}}
	cfg.ThumbnailDir = filepath.Join(cfg.AppDataDir, "thumbs")
	cfg.TrashDir = filepath.Join(cfg.AppDataDir, "trash")
	cfg.ThumbnailSize = 512
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	for _, key := range []string{"storage_path", "thumbnail_dir", "thumbnail_size", "trash_dir", "preferences", "use_system_player"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("config.json 不应再保存 Profile 字段 %q", key)
		}
	}
	if _, ok := decoded["libraries"]; !ok {
		t.Fatalf("config.json 应保留全局 libraries 注册表")
	}
}

func TestSaveToPath_PersistsGlobalBatchWorkflowState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	cfg := validConfig()
	cfg.Libraries = []Library{
		{ID: "lib_a", Name: "家庭相册", Path: "/tmp/photos-a"},
		{ID: "lib_b", Name: "旅行相册", Path: "/tmp/photos-b"},
	}
	cfg.BatchScan = BatchTaskState{
		Status:              "running",
		Message:             "Scanning",
		SelectedLibraryIDs:  []string{"lib_a", "lib_b"},
		SelectedPaths:       []string{"/tmp/photos-a", "/tmp/photos-b"},
		SelectionConfigured: true,
		ExitAfterComplete:   true,
	}
	cfg.BatchThumbnails = BatchTaskState{
		Status:              "completed",
		Message:             "Done",
		SelectedLibraryIDs:  []string{"lib_b"},
		SelectedPaths:       []string{"/tmp/photos-b"},
		SelectionConfigured: true,
	}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("读取刚保存的文件失败: %v", err)
	}
	if loaded.BatchScan.Status != "running" || len(loaded.BatchScan.SelectedLibraryIDs) != 2 {
		t.Fatalf("期望全局 batch_scan 被持久化，得到 %#v", loaded.BatchScan)
	}
	if loaded.BatchThumbnails.Status != "completed" || len(loaded.BatchThumbnails.SelectedLibraryIDs) != 1 {
		t.Fatalf("期望全局 batch_thumbnails 被持久化，得到 %#v", loaded.BatchThumbnails)
	}
}

func TestConfigNormalizeLibraries_AssignsStableIDsAndPrefersActiveLibraryID(t *testing.T) {
	cfg := &Config{
		StoragePath:     "/tmp/library-b",
		ActiveLibraryID: "LIB_B",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a"},
			{Name: "资源库 B", Path: "/tmp/library-b", ID: "LIB_B"},
		},
	}

	cfg.normalizeLibraries()

	if len(cfg.Libraries) != 2 {
		t.Fatalf("期望保留 2 个资源库，得到 %d", len(cfg.Libraries))
	}
	if cfg.Libraries[0].ID == "" {
		t.Fatalf("期望旧资源库自动补齐 library_id")
	}
	if cfg.Libraries[1].ID != "lib_b" {
		t.Fatalf("期望规范化已有 library_id，得到 %q", cfg.Libraries[1].ID)
	}
	if cfg.ActiveLibraryID != "lib_b" {
		t.Fatalf("期望优先使用 ActiveLibraryID，得到 %q", cfg.ActiveLibraryID)
	}
	if cfg.StoragePath != "/tmp/library-b" {
		t.Fatalf("期望 ActiveLibraryID 对应的路径成为当前资源库，得到 %q", cfg.StoragePath)
	}
}

func TestConfigNormalizeLibraries_LegacyLibraryIDIsDeterministic(t *testing.T) {
	cfgA := &Config{
		StoragePath: "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a"},
		},
	}
	cfgB := &Config{
		StoragePath: "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a"},
		},
	}

	cfgA.normalizeLibraries()
	cfgB.normalizeLibraries()

	if cfgA.Libraries[0].ID == "" {
		t.Fatal("期望旧资源库补齐稳定 library_id")
	}
	if cfgA.Libraries[0].ID != cfgB.Libraries[0].ID {
		t.Fatalf("期望相同路径反复规范化得到同一个 library_id，得到 %q 和 %q", cfgA.Libraries[0].ID, cfgB.Libraries[0].ID)
	}
}

func TestConfigNormalizeLibraries_StoragePathOnlyLibraryIDIsDeterministic(t *testing.T) {
	cfgA := &Config{StoragePath: "/tmp/library-a"}
	cfgB := &Config{StoragePath: "/tmp/library-a"}

	cfgA.normalizeLibraries()
	cfgB.normalizeLibraries()

	if len(cfgA.Libraries) != 1 || len(cfgB.Libraries) != 1 {
		t.Fatalf("期望从 storage_path 补齐单个资源库，得到 %d 和 %d", len(cfgA.Libraries), len(cfgB.Libraries))
	}
	if cfgA.Libraries[0].ID == "" {
		t.Fatal("期望 storage_path-only 旧配置补齐稳定 library_id")
	}
	if cfgA.Libraries[0].ID != cfgB.Libraries[0].ID {
		t.Fatalf("期望 storage_path-only 旧配置反复规范化得到同一个 library_id，得到 %q 和 %q", cfgA.Libraries[0].ID, cfgB.Libraries[0].ID)
	}
}

func TestConfigNormalizeLibraries_DuplicateIDFallbackIsDeterministic(t *testing.T) {
	cfgA := &Config{
		StoragePath: "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a", ID: "lib_same"},
			{Name: "资源库 B", Path: "/tmp/library-b", ID: "lib_same"},
		},
	}
	cfgB := &Config{
		StoragePath: "/tmp/library-a",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a", ID: "lib_same"},
			{Name: "资源库 B", Path: "/tmp/library-b", ID: "lib_same"},
		},
	}

	cfgA.normalizeLibraries()
	cfgB.normalizeLibraries()

	if cfgA.Libraries[1].ID == "" || cfgA.Libraries[1].ID == "lib_same" {
		t.Fatalf("期望重复 id 使用稳定兜底 id，得到 %q", cfgA.Libraries[1].ID)
	}
	if cfgA.Libraries[1].ID != cfgB.Libraries[1].ID {
		t.Fatalf("期望重复 id 兜底稳定，得到 %q 和 %q", cfgA.Libraries[1].ID, cfgB.Libraries[1].ID)
	}
}

func TestConfigNormalizeLibraries_FallsBackToPathWhenActiveLibraryIDMissing(t *testing.T) {
	cfg := &Config{
		ActiveLibraryID: "missing",
		StoragePath:     "/tmp/library-b",
		Libraries: []Library{
			{Name: "资源库 A", Path: "/tmp/library-a", ID: "lib_a"},
			{Name: "资源库 B", Path: "/tmp/library-b", ID: "lib_b"},
		},
	}

	cfg.normalizeLibraries()

	if cfg.ActiveLibraryID != "lib_b" {
		t.Fatalf("期望在 id 缺失时回退到 storage_path 对应资源库，得到 %q", cfg.ActiveLibraryID)
	}
	if cfg.StoragePath != "/tmp/library-b" {
		t.Fatalf("期望保持 storage_path 对应的活动资源库，得到 %q", cfg.StoragePath)
	}
}

func TestDatabasePath_IsStablePerStoragePath(t *testing.T) {
	dir := t.TempDir()
	cfg1 := validConfig()
	cfg1.AppDataDir = dir
	cfg1.StoragePath = filepath.Join(dir, "library-a")

	cfg2 := validConfig()
	cfg2.AppDataDir = dir
	cfg2.StoragePath = filepath.Join(dir, "library-a")

	db1, err := cfg1.DatabasePath()
	if err != nil {
		t.Fatalf("获取数据库路径失败: %v", err)
	}
	db2, err := cfg2.DatabasePath()
	if err != nil {
		t.Fatalf("获取数据库路径失败: %v", err)
	}
	if db1 != db2 {
		t.Fatalf("同一个 storage_path 应映射到同一个数据库: %s != %s", db1, db2)
	}
}

func TestDatabasePath_DiffersAcrossStoragePaths(t *testing.T) {
	dir := t.TempDir()
	cfg1 := validConfig()
	cfg1.AppDataDir = dir
	cfg1.StoragePath = filepath.Join(dir, "library-a")

	cfg2 := validConfig()
	cfg2.AppDataDir = dir
	cfg2.StoragePath = filepath.Join(dir, "library-b")

	db1, err := cfg1.DatabasePath()
	if err != nil {
		t.Fatalf("获取数据库路径失败: %v", err)
	}
	db2, err := cfg2.DatabasePath()
	if err != nil {
		t.Fatalf("获取数据库路径失败: %v", err)
	}
	if db1 == db2 {
		t.Fatalf("不同 storage_path 不应映射到同一个数据库: %s", db1)
	}
}

func TestDatabasePath_UsesEchoGalleryPrefix(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig()
	cfg.AppDataDir = dir
	cfg.StoragePath = filepath.Join(dir, "library-a")

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		t.Fatalf("获取数据库路径失败: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(dbPath), "echogallery-") {
		t.Fatalf("期望数据库文件前缀为 echogallery，得到 %s", filepath.Base(dbPath))
	}
}

func TestTrashPath_DefaultsToAppDataTrash(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig()
	cfg.AppDataDir = dir

	trashPath, err := cfg.TrashPath()
	if err != nil {
		t.Fatalf("获取回收站目录失败: %v", err)
	}
	want := filepath.Join(dir, "Trash")
	if trashPath != want {
		t.Fatalf("期望默认回收站目录为 %s，得到 %s", want, trashPath)
	}
}

func TestNormalizeStoragePath_EquivalentPathsMatch(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "photos")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	normalizedA := NormalizeStoragePath(base)
	normalizedB := NormalizeStoragePath(filepath.Join(base, "."))
	if normalizedA != normalizedB {
		t.Fatalf("等价路径应规范化为同一个结果: %s != %s", normalizedA, normalizedB)
	}
}

// --- generateSecret 测试 ---

func TestGenerateSecret_Length(t *testing.T) {
	secret, err := generateSecret(32)
	if err != nil {
		t.Fatalf("生成 secret 失败: %v", err)
	}
	// 32 字节 hex 编码后为 64 字符
	if len(secret) != 64 {
		t.Errorf("期望长度 64，得到 %d", len(secret))
	}
}

func TestGenerateSecret_Unique(t *testing.T) {
	s1, _ := generateSecret(32)
	s2, _ := generateSecret(32)
	if s1 == s2 {
		t.Error("两次生成的 secret 不应该相同")
	}
}

func TestRunInitWizardWithReader_CreatesDefaultUser(t *testing.T) {
	storagePath := filepath.Join(t.TempDir(), "photos")
	input := bytes.NewBufferString("9090\n" + storagePath + "\nadmin1\npassword123\n")
	output := &bytes.Buffer{}

	cfg, err := runInitWizardWithReader(bufio.NewReader(input), output)
	if err != nil {
		t.Fatalf("初始化向导失败: %v", err)
	}
	if cfg.Port != 9090 {
		t.Fatalf("期望端口 9090，得到 %d", cfg.Port)
	}
	if cfg.StoragePath != storagePath {
		t.Fatalf("期望存储路径 %s，得到 %s", storagePath, cfg.StoragePath)
	}
	if len(cfg.Users) != 1 {
		t.Fatalf("期望 1 个默认用户，得到 %d", len(cfg.Users))
	}
	if cfg.Users[0].Username != "admin1" {
		t.Fatalf("期望默认用户名 admin1，得到 %s", cfg.Users[0].Username)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cfg.Users[0].PasswordHash), []byte("password123")); err != nil {
		t.Fatalf("默认用户密码哈希校验失败: %v", err)
	}
	if _, err := os.Stat(storagePath); err != nil {
		t.Fatalf("存储目录应已创建: %v", err)
	}
}

func TestRunInitWizardWithReader_InvalidPasswordRetries(t *testing.T) {
	storagePath := filepath.Join(t.TempDir(), "photos")
	input := bytes.NewBufferString("\n" + storagePath + "\n\n123\npassword123\n")
	output := &bytes.Buffer{}

	cfg, err := runInitWizardWithReader(bufio.NewReader(input), output)
	if err != nil {
		t.Fatalf("初始化向导失败: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("期望默认端口 8080，得到 %d", cfg.Port)
	}
	if cfg.Users[0].Username != "admin" {
		t.Fatalf("期望默认用户名 admin，得到 %s", cfg.Users[0].Username)
	}
	if !bytes.Contains(output.Bytes(), []byte("password must be at least 6 characters")) {
		t.Fatal("应提示密码长度不足并重试")
	}
}
