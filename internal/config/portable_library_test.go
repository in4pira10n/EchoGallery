package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryDataPathsAreScopedToStoragePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "photos")
	cfg := &Config{AppDataDir: filepath.Join(t.TempDir(), "echogallery-data")}
	db, err := cfg.DatabasePathForStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	media, err := cfg.ManagedDataDirForStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	thumbs, err := cfg.ThumbnailStoragePathForStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := LibraryDataRoot(root)
	for got, want := range map[string]string{
		db:     filepath.Join(dataRoot, "metadata.sqlite"),
		media:  filepath.Join(dataRoot, "media"),
		thumbs: filepath.Join(dataRoot, "thumbnails"),
	} {
		if got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
	}
	if filepath.Dir(db) == filepath.Join(cfg.AppDataDir, "db") || media == filepath.Join(cfg.AppDataDir, "media") || thumbs == filepath.Join(cfg.AppDataDir, "thumbnails") {
		t.Fatal("library data unexpectedly points to server-local data")
	}
}

func TestLegacyDataLayoutDetectedWithoutOpeningIt(t *testing.T) {
	appDataDir := t.TempDir()
	cfg := &Config{AppDataDir: appDataDir}
	if cfg.LegacyDataLayoutDetected() {
		t.Fatal("empty app data directory should not be legacy layout")
	}
	if err := os.MkdirAll(filepath.Join(appDataDir, "db"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDataDir, "db", "library-locks.db"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if cfg.LegacyDataLayoutDetected() {
		t.Fatal("新版锁数据库不应被误判为旧版媒体数据")
	}
	if err := os.WriteFile(filepath.Join(appDataDir, "db", "echogallery-12345678.db"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if !cfg.LegacyDataLayoutDetected() {
		t.Fatal("旧版媒体数据库未被检测")
	}
}

func TestSQLiteReadOnlyURIKeepsWindowsDriveInPath(t *testing.T) {
	got := sqliteReadOnlyURI(`C:\Users\57219\Documents\ACMPGallery\library.db`)
	want := "file:///C:/Users/57219/Documents/ACMPGallery/library.db?mode=ro"
	if got != want {
		t.Fatalf("URI = %q, want %q", got, want)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "" || u.Query().Get("mode") != "ro" {
		t.Fatalf("invalid read-only URI: %#v", u)
	}
}
