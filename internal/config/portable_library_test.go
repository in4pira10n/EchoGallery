package config

import (
	"net/url"
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
