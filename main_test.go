package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	"echogallery/internal/config"

	_ "modernc.org/sqlite"
)

func TestLegacyDatabaseMatchesStoragePath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("创建 app_meta 失败: %v", err)
	}
	storagePath := filepath.Join(dir, "library-a")
	if _, err := db.Exec(`INSERT INTO app_meta(key, value) VALUES('storage_path', ?)`, config.NormalizeStoragePath(storagePath)); err != nil {
		t.Fatalf("写入 storage_path 失败: %v", err)
	}

	matches, err := legacyDatabaseMatchesStoragePath(dbPath, storagePath)
	if err != nil {
		t.Fatalf("匹配检查失败: %v", err)
	}
	if !matches {
		t.Fatal("期望旧数据库匹配当前 storage_path")
	}
}

func TestLegacyDatabaseMatchesStoragePath_RejectsDifferentStorage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("创建 app_meta 失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO app_meta(key, value) VALUES('storage_path', ?)`, config.NormalizeStoragePath(filepath.Join(dir, "library-a"))); err != nil {
		t.Fatalf("写入 storage_path 失败: %v", err)
	}

	matches, err := legacyDatabaseMatchesStoragePath(dbPath, filepath.Join(dir, "library-b"))
	if err != nil {
		t.Fatalf("匹配检查失败: %v", err)
	}
	if matches {
		t.Fatal("不同 storage_path 不应复用同一个旧数据库")
	}
}
