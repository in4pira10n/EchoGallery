package api

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"echogallery/internal/config"
)

func TestConfigTransferRoundTripRemapsLibrariesAndPreservesUsers(t *testing.T) {
	sourceData := filepath.Join(t.TempDir(), "source-data")
	source := &config.Config{
		Port: 9090, AppDataDir: sourceData, ActiveProfile: "alice", JWTSecret: "portable-secret",
		Libraries: []config.Library{
			{ID: "lib_a", Name: "A", Path: "/old/A", AccentColor: "#123456", OwnerUsername: "alice"},
			{ID: "lib_b", Name: "B", Path: "/old/B", OwnerUsername: "bob"},
		},
		Users: []config.User{
			{Username: "alice", PasswordHash: "hash-a", Role: config.UserRoleAdmin, DefaultLibraryID: "lib_a"},
			{Username: "bob", PasswordHash: "hash-b", Role: config.UserRoleVisitor, AllowedLibraryIDs: []string{"lib_b"}, DefaultLibraryID: "lib_b"},
		},
	}
	for _, user := range source.Users {
		profile := config.DefaultProfileFromConfig(source)
		profile.ActiveLibraryID = user.DefaultLibraryID
		for _, library := range source.Libraries {
			if library.ID == user.DefaultLibraryID {
				profile.StoragePath = library.Path
			}
		}
		profile.Preferences.Theme = map[bool]string{true: "dark", false: "light"}[user.Username == "alice"]
		if err := config.SaveProfile(source, user.Username, profile); err != nil {
			t.Fatal(err)
		}
		avatar, err := source.ProfileAvatarPath(user.Username)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(avatar, []byte("avatar-"+user.Username), 0644); err != nil {
			t.Fatal(err)
		}
	}
	workshop := filepath.Join(sourceData, "workshop", "brand.png")
	if err := os.MkdirAll(filepath.Dir(workshop), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workshop, []byte("brand"), 0644); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildConfigTransferBundle(source)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := transferZipFiles(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Config.ThumbnailDir != "" || manifest.Config.TrashDir != "" {
		t.Fatalf("不应导出旧电脑缓存绝对路径: thumb=%s trash=%s", manifest.Config.ThumbnailDir, manifest.Config.TrashDir)
	}
	targetData := filepath.Join(t.TempDir(), "target-data")
	newA := filepath.Join(t.TempDir(), "new-A")
	newB := filepath.Join(t.TempDir(), "new-B")
	for _, path := range []string{newA, newB} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	target := &config.Config{
		Port: 8080, AppDataDir: targetData, JWTSecret: "temporary",
		Libraries: []config.Library{{ID: "lib_a", Path: newA}, {ID: "lib_b", Path: newB}},
		Users:     []config.User{{Username: "temporary", PasswordHash: "hash", Role: config.UserRoleAdmin}},
	}
	savedPath := filepath.Join(t.TempDir(), "imported.json")
	keymapPath := filepath.Join(t.TempDir(), "keymap.conf")
	err = importConfigTransferBundleWithSave(target, bundle, func(next *config.Config) error {
		data, marshalErr := json.Marshal(next)
		if marshalErr != nil {
			return marshalErr
		}
		return os.WriteFile(savedPath, data, 0600)
	}, func(content string) error { return os.WriteFile(keymapPath, []byte(content), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	if target.Port != 9090 || target.JWTSecret != "portable-secret" {
		t.Fatalf("主机配置未恢复: port=%d secret=%s", target.Port, target.JWTSecret)
	}
	if len(target.Users) != 2 || target.Users[0].Username != "alice" || target.Users[1].Username != "bob" || target.Users[1].PasswordHash != "hash-b" {
		t.Fatalf("用户顺序或密码哈希未保留: %+v", target.Users)
	}
	if got := []string{target.Libraries[0].Path, target.Libraries[1].Path}; !slices.Equal(got, []string{newA, newB}) {
		t.Fatalf("资源库路径未按 ID 映射: %+v", got)
	}
	metadata, err := sql.Open("sqlite", filepath.Join(newA, ".echogallery", "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer metadata.Close()
	var portableName, portableAccent string
	if err := metadata.QueryRow(`SELECT value FROM app_meta WHERE key = 'library_name'`).Scan(&portableName); err != nil {
		t.Fatal(err)
	}
	if err := metadata.QueryRow(`SELECT value FROM app_meta WHERE key = 'library_accent_color'`).Scan(&portableAccent); err != nil {
		t.Fatal(err)
	}
	if portableName != "A" || portableAccent != "#123456" {
		t.Fatalf("资源库展示元数据未写入: name=%s accent=%s", portableName, portableAccent)
	}
	profile, err := config.LoadProfile(target, "bob")
	if err != nil || profile.ActiveLibraryID != "lib_b" || profile.StoragePath != newB {
		t.Fatalf("用户 profile 未映射: %+v, %v", profile, err)
	}
	avatar, _ := target.ProfileAvatarPath("alice")
	if content, err := os.ReadFile(avatar); err != nil || string(content) != "avatar-alice" {
		t.Fatalf("头像未恢复: %q, %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(targetData, "workshop", "brand.png")); err != nil || string(content) != "brand" {
		t.Fatalf("品牌资源未恢复: %q, %v", content, err)
	}
	backups, err := filepath.Glob(filepath.Join(targetData, "backups", "before-import-*.zip"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("导入前备份未创建: %+v, %v", backups, err)
	}
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("导入配置未保存: %v", err)
	}
}

func TestConfigTransferExcludesRuntimeAndLegacyCaches(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	for _, path := range []string{
		filepath.Join(cfg.AppDataDir, "db", "library-locks.db"),
		filepath.Join(cfg.AppDataDir, "thumbnails", "old.webp"),
		filepath.Join(cfg.AppDataDir, "media", "old.mp4"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("runtime"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bundle, err := buildConfigTransferBundle(cfg)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "db/") || strings.HasPrefix(file.Name, "media/") || strings.HasPrefix(file.Name, "thumbnails/") {
			t.Fatalf("运行时或旧缓存不应进入配置包: %s", file.Name)
		}
	}
}

func TestConfigTransferIgnoresUnsafeArchivePaths(t *testing.T) {
	manifest, err := json.Marshal(configTransferManifest{Version: configTransferVersion, Config: *testConfig()})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string][]byte{"manifest.json": manifest, "../outside.txt": []byte("bad")} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_, files, err := transferZipFiles(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["../outside.txt"]; ok {
		t.Fatal("路径穿越文件不应进入导入列表")
	}
}

func TestConfigTransferCarriesBrowserSettings(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	bundle, err := buildConfigTransferBundleWithBrowser(cfg, json.RawMessage(`{"pwa":"custom","session":"excluded-by-client"}`))
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := transferZipFiles(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]string
	if err := json.Unmarshal(manifest.BrowserSettings, &settings); err != nil || settings["pwa"] != "custom" {
		t.Fatalf("浏览器设置未进入配置包: %+v, %v", settings, err)
	}
}
