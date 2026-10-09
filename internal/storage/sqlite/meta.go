package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"echogallery/internal/storage"
)

const metaStoragePathKey = "storage_path"
const metaLibraryScanSnapshotKey = "library_scan_snapshot"
const metaLibraryVideoVolumeKey = "library_video_volume"
const metaLibraryIDKey = "library_id"

// EnsureLibraryIdentity binds the portable data directory to its thumbnail
// namespace, without replacing an identity already stored in the library.
func (s *DB) EnsureLibraryIdentity(id string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return fmt.Errorf("资源库 ID 不能为空")
	}
	existing, err := s.getMeta(metaLibraryIDKey)
	if err != nil {
		return err
	}
	if existing != "" {
		if existing != id {
			return fmt.Errorf("资源库 ID 不一致：目录记录为 %s，当前配置为 %s", existing, id)
		}
		return nil
	}
	return s.setMeta(metaLibraryIDKey, id)
}

func (s *DB) EnsureLibraryMetadata(id, name, accentColor string) error {
	if err := s.EnsureLibraryIdentity(id); err != nil {
		return err
	}
	for key, value := range map[string]string{
		"library_name":         strings.TrimSpace(name),
		"library_accent_color": strings.TrimSpace(accentColor),
	} {
		if err := s.setMeta(key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *DB) initializePortableMetadata() error {
	if err := s.ensureMetaTable(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO app_meta(key, value)
		VALUES ('library_data_owner_id', COALESCE(
			(SELECT CAST(uploaded_by AS TEXT) FROM photos GROUP BY uploaded_by ORDER BY COUNT(*) DESC, uploaded_by LIMIT 1),
			(SELECT CAST(created_by AS TEXT) FROM albums GROUP BY created_by ORDER BY COUNT(*) DESC, created_by LIMIT 1), '1'))`); err != nil {
		return err
	}
	owner, err := s.getMeta("library_data_owner_id")
	if err != nil {
		return err
	}
	s.libraryUserID, err = strconv.ParseInt(owner, 10, 64)
	if err != nil || s.libraryUserID <= 0 {
		return fmt.Errorf("无效的资源库数据所有者编号: %q", owner)
	}
	if err := s.reconcilePortableMedia(); err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE VIEW IF NOT EXISTS trash_links AS
		SELECT uuid AS media_id, source_rel_path AS relative_path, deleted_at AS trashed_at
		FROM photos WHERE deleted_at IS NOT NULL`)
	return err
}

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
