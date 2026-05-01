package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestGetDownloadEntries_DeduplicatesNames(t *testing.T) {
	repo := newMockRepo()
	sourceDir := t.TempDir()
	svc := newPhotoServiceSync(repo, sourceDir, t.TempDir(), t.TempDir())

	if err := os.WriteFile(filepath.Join(sourceDir, "same-1.jpg"), []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "same-2.jpg"), []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}

	p1 := &storage.Photo{UUID: "u1", OriginalName: "same.jpg", MimeType: "image/jpeg", SourceRelPath: "same-1.jpg", TakenAt: time.Now(), UploadedAt: time.Now(), UploadedBy: 1}
	p2 := &storage.Photo{UUID: "u2", OriginalName: "same.jpg", MimeType: "image/jpeg", SourceRelPath: "same-2.jpg", TakenAt: time.Now(), UploadedAt: time.Now(), UploadedBy: 1}
	if err := repo.SavePhoto(p1); err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePhoto(p2); err != nil {
		t.Fatal(err)
	}

	entries, err := svc.GetDownloadEntries([]int64{p1.ID, p2.ID}, 1)
	if err != nil {
		t.Fatalf("获取下载条目失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("期望 2 个条目，得到 %d", len(entries))
	}
	if entries[0].FileName != "same.jpg" {
		t.Fatalf("第一个文件名错误: %s", entries[0].FileName)
	}
	if entries[1].FileName != "same (2).jpg" {
		t.Fatalf("第二个文件名错误: %s", entries[1].FileName)
	}
}
