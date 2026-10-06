package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func saveMigrationPhoto(t *testing.T, db *DB, id, relPath string, size int64) {
	t.Helper()
	if err := db.SavePhoto(&storage.Photo{
		UUID: id, OriginalName: filepath.Base(relPath), MediaKind: storage.MediaKindImage,
		MimeType: "image/jpeg", Size: size, SourceRelPath: relPath,
		TakenAt: time.Now(), UploadedAt: time.Now(), UploadedBy: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreLegacyUUIDsMatchesExistingRecordsAndKeepsOldDBReadOnly(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "echogallery-old.db")
	old, err := New(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	oldID := "35bd948d-019e-435d-bf41-01c63dd4d945"
	saveMigrationPhoto(t, old, oldID, "photos/a.jpg", 128)
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	current := newTestDB(t)
	saveMigrationPhoto(t, current, "22222222-2222-4222-8222-222222222222", "photos/a.jpg", 128)
	matches, err := current.RestoreLegacyUUIDs(context.Background(), oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].OldUUID != oldID {
		t.Fatalf("旧版 UUID 未准确匹配: %+v", matches)
	}
	var got string
	if err := current.db.QueryRow(`SELECT uuid FROM photos WHERE source_rel_path = ?`, "photos/a.jpg").Scan(&got); err != nil || got != oldID {
		t.Fatalf("新版 UUID 未更新: %s, %v", got, err)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("旧数据库不应被改写")
	}
	if _, err := current.RestoreLegacyUUIDs(context.Background(), oldPath); err != nil {
		t.Fatalf("再次迁移应幂等: %v", err)
	}
}

func TestRestoreLegacyUUIDsRejectsMismatchWithoutChangingNewDB(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "echogallery-old.db")
	old, err := New(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	saveMigrationPhoto(t, old, "35bd948d-019e-435d-bf41-01c63dd4d945", "photos/a.jpg", 128)
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	current := newTestDB(t)
	newID := "22222222-2222-4222-8222-222222222222"
	saveMigrationPhoto(t, current, newID, "photos/a.jpg", 129)
	_, err = current.RestoreLegacyUUIDs(context.Background(), oldPath)
	if err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("媒体大小不一致时应拒绝迁移: %v", err)
	}
	var got string
	if err := current.db.QueryRow(`SELECT uuid FROM photos WHERE source_rel_path = ?`, "photos/a.jpg").Scan(&got); err != nil || got != newID {
		t.Fatalf("失败后 UUID 应保持原值: %s, %v", got, err)
	}
}

func TestRestoreLegacyUUIDsRejectsOldMediaMissingFromNewIndex(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "echogallery-old.db")
	old, err := New(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	saveMigrationPhoto(t, old, "35bd948d-019e-435d-bf41-01c63dd4d945", "missing/a.jpg", 128)
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	current := newTestDB(t)
	_, err = current.RestoreLegacyUUIDs(context.Background(), oldPath)
	if err == nil || !strings.Contains(err.Error(), "未出现在新版") {
		t.Fatalf("遗漏旧媒体时不应报告成功: %v", err)
	}
}

func TestRestoreLegacyUUIDsSkipsDuplicatePathsAndKeepsUniqueMatches(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "echogallery-old.db")
	old, err := New(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	saveMigrationPhoto(t, old, "35bd948d-019e-435d-bf41-01c63dd4d945", "dup/a.jpg", 128)
	saveMigrationPhoto(t, old, "invalid-duplicate-uuid", `dup\a.jpg`, 128)
	saveMigrationPhoto(t, old, "55555555-5555-4555-8555-555555555555", "unique/b.jpg", 64)
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	current := newTestDB(t)
	duplicateID := "22222222-2222-4222-8222-222222222222"
	saveMigrationPhoto(t, current, duplicateID, "dup/a.jpg", 128)
	saveMigrationPhoto(t, current, "33333333-3333-4333-8333-333333333333", "unique/b.jpg", 64)
	report, err := current.RestoreLegacyUUIDsWithReport(context.Background(), oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SkippedPaths) != 1 || report.SkippedPaths[0] != "dup/a.jpg" || len(report.Matches) != 1 {
		t.Fatalf("重复路径应只跳过歧义项: %+v", report)
	}
	var duplicateUUID, uniqueUUID string
	if err := current.db.QueryRow(`SELECT uuid FROM photos WHERE source_rel_path = ?`, "dup/a.jpg").Scan(&duplicateUUID); err != nil {
		t.Fatal(err)
	}
	if err := current.db.QueryRow(`SELECT uuid FROM photos WHERE source_rel_path = ?`, "unique/b.jpg").Scan(&uniqueUUID); err != nil {
		t.Fatal(err)
	}
	if duplicateUUID != duplicateID || uniqueUUID != "55555555-5555-4555-8555-555555555555" {
		t.Fatalf("唯一项应迁移，重复项应保留新版 UUID: dup=%s unique=%s", duplicateUUID, uniqueUUID)
	}
}
