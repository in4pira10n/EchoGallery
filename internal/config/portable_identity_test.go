package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"echogallery/internal/storage"
	"echogallery/internal/storage/sqlite"
)

func TestExistingPortableLibraryRestoresRecordedID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "moved-library")
	dataRoot := LibraryDataRoot(root)
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.New(filepath.Join(dataRoot, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureLibraryMetadata("lib_original", "Travel Archive", "#123456"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{StoragePath: root, Libraries: []Library{{Name: "Restored", Path: root}}}
	cfg.normalizeLibraries()
	if got := cfg.Libraries[0].ID; got != "lib_original" {
		t.Fatalf("未沿用目录记录的 ID: %s", got)
	}
	if cfg.Libraries[0].Name != "Travel Archive" || cfg.Libraries[0].AccentColor != "#123456" {
		t.Fatalf("未恢复资源库名称和强调色: %+v", cfg.Libraries[0])
	}
}

func TestExistingPortableLibraryInfersOldThumbnailNamespace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "restored-library")
	dataRoot := LibraryDataRoot(root)
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.New(filepath.Join(dataRoot, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	photo := &storage.Photo{
		UUID:         "35bd948d-019e-435d-bf41-01c63dd4d945",
		OriginalName: "a.jpg", MediaKind: storage.MediaKindImage,
		MimeType: "image/jpeg", SourceRelPath: "a.jpg", UploadedBy: 1,
		TakenAt: time.Now(), UploadedAt: time.Now(),
	}
	if err := db.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	thumbPath := filepath.Join(dataRoot, "thumbnails", "lib_before_move", "35", "bd", photo.UUID+".webp")
	if err := os.MkdirAll(filepath.Dir(thumbPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumbPath, []byte("existing thumbnail"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{StoragePath: root, Libraries: []Library{{Name: "Restored", Path: root}}}
	cfg.normalizeLibraries()
	if got := cfg.Libraries[0].ID; got != "lib_before_move" {
		t.Fatalf("旧库未沿用缩略图命名空间: %s", got)
	}
}

func TestExistingPortableLibraryChoosesMatchingThumbnailNamespace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "restored-library")
	dataRoot := LibraryDataRoot(root)
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.New(filepath.Join(dataRoot, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	photo := &storage.Photo{
		UUID:         "35bd948d-019e-435d-bf41-01c63dd4d945",
		OriginalName: "a.jpg", MediaKind: storage.MediaKindImage,
		MimeType: "image/jpeg", SourceRelPath: "a.jpg", UploadedBy: 1,
		TakenAt: time.Now(), UploadedAt: time.Now(),
	}
	if err := db.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	thumbPath := filepath.Join(dataRoot, "thumbnails", "lib_current", "35", "bd", photo.UUID+".webp")
	for _, path := range []string{thumbPath, filepath.Join(dataRoot, "thumbnails", "lib_stale", "aa", "bb", "unused.webp")} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("thumbnail"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &Config{StoragePath: root, Libraries: []Library{{Name: "Restored", Path: root}}}
	cfg.normalizeLibraries()
	if got := cfg.Libraries[0].ID; got != "lib_current" {
		t.Fatalf("多个缩略图目录时未识别实际使用的 ID: %s", got)
	}
}

func TestExistingPortableLibraryRejectsAmbiguousThumbnailNamespaces(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ambiguous-library")
	dataRoot := LibraryDataRoot(root)
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.New(filepath.Join(dataRoot, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	photo := &storage.Photo{
		UUID:         "35bd948d-019e-435d-bf41-01c63dd4d945",
		OriginalName: "a.jpg", MediaKind: storage.MediaKindImage,
		MimeType: "image/jpeg", SourceRelPath: "a.jpg", UploadedBy: 1,
		TakenAt: time.Now(), UploadedAt: time.Now(),
	}
	if err := db.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lib_one", "lib_two"} {
		path := filepath.Join(dataRoot, "thumbnails", id, "35", "bd", photo.UUID+".webp")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("thumbnail"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidatePortableLibraryIdentity(root, "lib_one"); err == nil {
		t.Fatal("多个匹配的缩略图命名空间不应被自动选中")
	}
}
