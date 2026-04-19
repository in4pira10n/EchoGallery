package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

const metaStoragePathKey = "storage_path"

func (s *DB) ensureMetaTable() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS app_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
)`)
	return err
}

func (s *DB) getMeta(key string) (string, error) {
	if err := s.ensureMetaTable(); err != nil {
		return "", err
	}
	var value string
	err := s.db.QueryRow(`SELECT value FROM app_meta WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (s *DB) setMeta(key, value string) error {
	if err := s.ensureMetaTable(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO app_meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// CheckStoragePathConsistency 在 storage_path 变化时校验数据库记录与实际文件的对应关系。
func (s *DB) CheckStoragePathConsistency(sourceRoot, managedRoot string) error {
	prev, err := s.getMeta(metaStoragePathKey)
	if err != nil {
		return err
	}
	current := filepath.Clean(sourceRoot)
	if prev == "" {
		return s.setMeta(metaStoragePathKey, current)
	}
	if filepath.Clean(prev) == current {
		return nil
	}

	rows, err := s.db.Query(`
		SELECT id, original_name, storage_rel_path, source_rel_path, uuid
		FROM photos`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id             int64
			originalName   string
			storageRelPath string
			sourceRelPath  string
			uuid           string
		)
		if err := rows.Scan(&id, &originalName, &storageRelPath, &sourceRelPath, &uuid); err != nil {
			return err
		}
		if storageRelPath != "" {
			if _, err := os.Stat(filepath.Join(managedRoot, storageRelPath)); err != nil {
				return fmt.Errorf("数据库记录 #%d 缺少内部文件: %s", id, storageRelPath)
			}
		}
		if sourceRelPath != "" {
			if _, err := os.Stat(filepath.Join(sourceRoot, sourceRelPath)); err != nil {
				return fmt.Errorf("数据库记录 #%d 缺少源文件: %s", id, sourceRelPath)
			}
		} else if uuid == "" && originalName == "" {
			return fmt.Errorf("数据库记录 #%d 数据异常", id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.setMeta(metaStoragePathKey, current)
}
