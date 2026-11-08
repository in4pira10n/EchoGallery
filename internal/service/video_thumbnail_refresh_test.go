package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"echogallery/internal/storage"
)

func TestRefreshVideoThumbnailReplacesOwnedPosterAndRemovesOldLayouts(t *testing.T) {
	svc, repo := cleanupService(t)
	file := filepath.Join(svc.sourcePath, "clip.mp4")
	if err := os.WriteFile(file, []byte("not a real video"), 0644); err != nil {
		t.Fatal(err)
	}
	photo := &storage.Photo{UUID: "video-refresh-test", OriginalName: "clip.mp4", MediaKind: storage.MediaKindVideo, SourceRelPath: "clip.mp4", MimeType: "video/mp4", UploadedBy: 1}
	if err := repo.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	target := svc.ThumbnailPath(photo)
	old := filepath.Join(svc.thumbnailPath, "video-refresh-test.jpg")
	if err := os.MkdirAll(filepath.Dir(old), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshVideoThumbnail(photo.ID, 1); err == nil {
		t.Fatal("invalid source should fail")
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old" {
		t.Fatalf("old layout removed on failure: %q %v", data, err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("invalid source installed a target poster")
	}
}

func TestRefreshVideoThumbnailSuccessUsesNewRandomPosterAndCleansOldLayout(t *testing.T) {
	svc, repo := cleanupService(t)
	file := filepath.Join(svc.sourcePath, "clip.mp4")
	if err := os.WriteFile(file, []byte("source video remains"), 0644); err != nil {
		t.Fatal(err)
	}
	photo := &storage.Photo{UUID: "video-refresh-success", OriginalName: "clip.mp4", MediaKind: storage.MediaKindVideo, SourceRelPath: "clip.mp4", MimeType: "video/mp4", UploadedBy: 1}
	if err := repo.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	target := svc.ThumbnailPath(photo)
	old := svc.legacyFlatThumbnailPath(photo)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old poster"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old layout"), 0644); err != nil {
		t.Fatal(err)
	}
	svc.randomPosterGenerator = func(_ context.Context, source, destination string, _ int) error {
		if source != file {
			t.Fatalf("generator source = %s", source)
		}
		return os.WriteFile(destination, []byte("new random poster"), 0644)
	}
	version, err := svc.RefreshVideoThumbnail(photo.ID, 1)
	if err != nil || version <= 0 {
		t.Fatalf("refresh failed: version=%d err=%v", version, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "new random poster" {
		t.Fatalf("new poster was not installed: %q %v", data, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old layout still exists: %v", err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "source video remains" {
		t.Fatalf("source video changed: %q %v", data, err)
	}
}

func TestReplaceThumbnailFilePreservesOldOnInstallFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "poster.webp")
	temp := filepath.Join(dir, "missing-temp.webp")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := replaceThumbnailFile(temp, target); err == nil {
		t.Fatal("missing temporary file should fail")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "old" {
		t.Fatalf("old poster not preserved: %q %v", data, err)
	}
}
