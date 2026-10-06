package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestLegacyThumbnailProgressSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.sqlite")
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.RecordLegacyThumbnailProgress(ctx, map[string]string{"old-a": "folder/a.jpg", "old-b": "b.jpg"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	completed, err := reopened.LegacyThumbnailCompleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 2 || completed["old-a"] != "folder/a.jpg" || completed["old-b"] != "b.jpg" {
		t.Fatalf("迁移记录未能续跑: %+v", completed)
	}
}

func TestPortableLibraryIdentityRejectsConflictingID(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.EnsureLibraryIdentity("lib_original"); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureLibraryIdentity("lib_original"); err != nil {
		t.Fatalf("相同资源库 ID 应可重复打开: %v", err)
	}
	if err := db.EnsureLibraryIdentity("lib_other"); err == nil {
		t.Fatal("不应将已有库绑定到不同 ID")
	}
}
