package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"echogallery/internal/storage"
)

const metaStoragePathKey = "storage_path"
const metaLibraryScanSnapshotKey = "library_scan_snapshot"
const metaLibraryVideoVolumeKey = "library_video_volume"

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

// GetLibraryScanSnapshot 读取上次成功扫描的资源库目录树快照。
func (s *DB) GetLibraryScanSnapshot() (*storage.LibraryScanSnapshot, error) {
	value, err := s.getMeta(metaLibraryScanSnapshotKey)
	if err != nil || value == "" {
		return nil, err
	}
	var snapshot storage.LibraryScanSnapshot
	if err := json.Unmarshal([]byte(value), &snapshot); err != nil {
		return nil, err
	}
	if snapshot.RootPath == "" || snapshot.RootModUnixNano <= 0 {
		return nil, nil
	}
	return &snapshot, nil
}

// SaveLibraryScanSnapshot 保存本次成功扫描的资源库目录树快照。
func (s *DB) SaveLibraryScanSnapshot(snapshot storage.LibraryScanSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return s.setMeta(metaLibraryScanSnapshotKey, string(data))
}

// GetLibraryVideoVolume 读取当前资源库共享的视频音量。
func (s *DB) GetLibraryVideoVolume() (float64, bool, error) {
	value, err := s.getMeta(metaLibraryVideoVolumeKey)
	if err != nil || value == "" {
		return 1, false, err
	}
	volume, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 1, false, err
	}
	return volume, true, nil
}

// SaveLibraryVideoVolume 保存当前资源库共享的视频音量。
func (s *DB) SaveLibraryVideoVolume(volume float64) error {
	return s.setMeta(metaLibraryVideoVolumeKey, strconv.FormatFloat(volume, 'f', -1, 64))
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
