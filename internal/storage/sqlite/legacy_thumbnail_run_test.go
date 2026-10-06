package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyThumbnailRunMarkerSkipsOnlyMatchingCompletedWork(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.db")
	if err := os.WriteFile(oldPath, []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(dir, "metadata.sqlite")
	db, err := New(newPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.SaveLegacyThumbnailRun(ctx, oldPath, false); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	complete, err := LegacyThumbnailRunCompleteAtPath(ctx, newPath, oldPath, false)
	if err != nil || !complete {
		t.Fatalf("相同来源的重复任务应立即完成: complete=%v err=%v", complete, err)
	}
	complete, err = LegacyThumbnailRunCompleteAtPath(ctx, newPath, oldPath, true)
	if err != nil || complete {
		t.Fatalf("新增清理选项应继续任务: complete=%v err=%v", complete, err)
	}
	if err := os.WriteFile(oldPath, []byte("legacy changed"), 0644); err != nil {
		t.Fatal(err)
	}
	complete, err = LegacyThumbnailRunCompleteAtPath(ctx, newPath, oldPath, false)
	if err != nil || complete {
		t.Fatalf("旧库变更后不能跳过: complete=%v err=%v", complete, err)
	}
}
