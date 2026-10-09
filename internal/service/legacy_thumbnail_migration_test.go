package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	imgpkg "echogallery/internal/image"
	"echogallery/internal/storage"
)

func TestLegacyThumbnailCopyAndCleanupAreSeparate(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	id := "35bd948d-019e-435d-bf41-01c63dd4d945"
	svc.SetThumbnailRoot(t.TempDir())
	svc.SetThumbnailLibraryID("lib_one")
	oldRoot := t.TempDir()
	svc.SetLegacyThumbnailRoot(oldRoot)
	oldPath := imgpkg.ThumbnailShardPath(filepath.Join(oldRoot, "lib_one"), id)
	if err := imgpkg.GenerateThumbnail(bytes.NewReader(createJPEGBytes(80, 40)), "image/jpeg", oldPath, 64); err != nil {
		t.Fatal(err)
	}
	match := []storage.LegacyUUIDMatch{{SourceRelPath: "a.jpg", OldUUID: id, NewUUID: "22222222-2222-4222-8222-222222222222"}}
	result, err := svc.MigrateLegacyThumbnailFiles(context.Background(), match, nil, nil)
	if err != nil || result.Copied != 1 {
		t.Fatalf("复制旧缩略图失败: %+v, %v", result, err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("复制阶段必须保留旧文件: %v", err)
	}
	target := imgpkg.ThumbnailShardPath(svc.managedThumbnailRoot(), id)
	if !validWebPThumbnail(target) {
		t.Fatal("新缩略图应能解码")
	}
	result, err = svc.MigrateLegacyThumbnailFiles(context.Background(), match, map[string]string{id: "a.jpg"}, nil)
	if err != nil || result.Resumed != 1 || result.Copied != 0 {
		t.Fatalf("已验证文件应在续跑时跳过: %+v, %v", result, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	result, err = svc.MigrateLegacyThumbnailFiles(context.Background(), match, map[string]string{id: "a.jpg"}, nil)
	if err != nil || result.Copied != 1 {
		t.Fatalf("目标文件丢失时应重新复制: %+v, %v", result, err)
	}
	cleaned, err := svc.CleanCopiedLegacyThumbnails(context.Background(), match)
	if err != nil || cleaned != 1 {
		t.Fatalf("显式清理旧文件失败: %d, %v", cleaned, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("清理后旧文件仍存在: %v", err)
	}
}

func TestLegacyThumbnailCleanupKeepsOldFileUntilNewThumbnailIsValid(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	id := "35bd948d-019e-435d-bf41-01c63dd4d945"
	svc.SetThumbnailRoot(t.TempDir())
	svc.SetThumbnailLibraryID("lib_one")
	oldRoot := t.TempDir()
	svc.SetLegacyThumbnailRoot(oldRoot)
	oldPath := imgpkg.ThumbnailFlatPath(filepath.Join(oldRoot, "lib_one"), id)
	if err := imgpkg.GenerateThumbnail(bytes.NewReader(createJPEGBytes(80, 40)), "image/jpeg", oldPath, 64); err != nil {
		t.Fatal(err)
	}
	match := []storage.LegacyUUIDMatch{{SourceRelPath: "a.jpg", OldUUID: id}}
	if _, err := svc.CleanCopiedLegacyThumbnails(context.Background(), match); err == nil {
		t.Fatal("新版文件不存在时不应删除旧文件")
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("旧文件必须保留: %v", err)
	}
}

func TestLegacySharedThumbnailRemainsReadableBeforeCopy(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	svc.SetThumbnailRoot(t.TempDir())
	svc.SetThumbnailLibraryID("lib_one")
	oldRoot := t.TempDir()
	svc.SetLegacyThumbnailRoot(oldRoot)
	photo := &storage.Photo{UUID: "35bd948d-019e-435d-bf41-01c63dd4d945"}
	oldPath := imgpkg.ThumbnailShardPath(oldRoot, photo.UUID)
	if err := imgpkg.GenerateThumbnail(bytes.NewReader(createJPEGBytes(80, 40)), "image/jpeg", oldPath, 64); err != nil {
		t.Fatal(err)
	}
	if !svc.thumbnailExistsForTier(photo, thumbnailTierFull) {
		t.Fatal("旧共享目录的缩略图应作为只读回退")
	}
	if _, err := os.Stat(svc.ThumbnailPath(photo)); !os.IsNotExist(err) {
		t.Fatalf("只读回退不应创建新版缩略图: %v", err)
	}
}

func TestLegacyThumbnailProgressBatchesEveryVerifiedItem(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	svc.SetThumbnailRoot(t.TempDir())
	svc.SetThumbnailLibraryID("lib_one")
	svc.SetLegacyThumbnailRoot(t.TempDir())
	seedPath := filepath.Join(t.TempDir(), "seed.webp")
	if err := imgpkg.GenerateThumbnail(bytes.NewReader(createJPEGBytes(12, 12)), "image/jpeg", seedPath, 12); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	const count = 130
	matches := make([]storage.LegacyUUIDMatch, count)
	for i := range matches {
		id := fmt.Sprintf("%08x-1111-4111-8111-%012x", i, i)
		matches[i] = storage.LegacyUUIDMatch{OldUUID: id, SourceRelPath: fmt.Sprintf("%d.jpg", i)}
		target := imgpkg.ThumbnailShardPath(svc.managedThumbnailRoot(), id)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, payload, 0644); err != nil {
			t.Fatal(err)
		}
	}
	verified := make(map[string]string)
	callbacks := 0
	result, err := svc.MigrateLegacyThumbnailFiles(context.Background(), matches, nil, func(progress LegacyThumbnailProgress) error {
		callbacks++
		for id, path := range progress.Verified {
			if _, exists := verified[id]; exists {
				t.Fatalf("重复记录已验证 UUID: %s", id)
			}
			verified[id] = path
		}
		return nil
	})
	if err != nil || result.Existing != count || len(verified) != count || callbacks < 2 || callbacks > 4 {
		t.Fatalf("批量进度不完整: result=%+v verified=%d callbacks=%d err=%v", result, len(verified), callbacks, err)
	}
}

func TestRebuildMissingThumbnailsRepairsCorruptAndMissingFiles(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	svc.SetThumbnailRoot(t.TempDir())
	repo := svc.repo.(*mockRepo)
	for index, name := range []string{"corrupt.jpg", "missing.jpg"} {
		if err := os.WriteFile(filepath.Join(svc.sourcePath, name), createJPEGBytes(80, 40), 0644); err != nil {
			t.Fatal(err)
		}
		photo := &storage.Photo{
			UUID:         fmt.Sprintf("%08x-1111-4111-8111-%012x", index+1, index+1),
			OriginalName: name, SourceRelPath: name, MimeType: "image/jpeg",
			MediaKind: storage.MediaKindImage, UploadedBy: 1, TakenAt: time.Now(), UploadedAt: time.Now(),
		}
		if err := repo.SavePhoto(photo); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			target := svc.ThumbnailPath(photo)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("broken"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	missing, err := svc.MissingNewThumbnailsWithProgress(context.Background(), 1, nil)
	if err != nil || len(missing) != 2 {
		t.Fatalf("应找到两个待补建缩略图: %d, %v", len(missing), err)
	}
	result, err := svc.RebuildMissingThumbnails(context.Background(), missing, nil)
	if err != nil || result.Generated != 2 || result.Failed != 0 {
		t.Fatalf("应补建两个缩略图: %+v, %v", result, err)
	}
	count, err := svc.AuditNewThumbnails(context.Background(), 1)
	if err != nil || count != 0 {
		t.Fatalf("补建后仍有缺失: %d, %v", count, err)
	}
}

func TestRebuildMissingThumbnailsReportsUnrecoverableOriginal(t *testing.T) {
	svc, _ := newTestPhotoService(t)
	svc.SetThumbnailRoot(t.TempDir())
	photo := &storage.Photo{
		UUID: "35bd948d-019e-435d-bf41-01c63dd4d945", OriginalName: "gone.jpg",
		SourceRelPath: "gone.jpg", MimeType: "image/jpeg", MediaKind: storage.MediaKindImage,
		UploadedBy: 1, TakenAt: time.Now(), UploadedAt: time.Now(),
	}
	if err := svc.repo.(*mockRepo).SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	result, err := svc.RebuildMissingThumbnails(context.Background(), []*storage.Photo{photo}, nil)
	if err != nil || result.Failed != 1 || len(result.Errors) != 1 {
		t.Fatalf("原媒体缺失时应报告失败而不是中断任务: %+v, %v", result, err)
	}
	remaining, err := svc.AuditNewThumbnails(context.Background(), 1)
	if err != nil || remaining != 1 {
		t.Fatalf("未修复项必须留在核对结果中: %d, %v", remaining, err)
	}
}
