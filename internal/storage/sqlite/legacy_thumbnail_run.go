package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const legacyThumbnailRunKey = "legacy_thumbnail_run"

type legacyFileStamp struct {
	Size     int64 `json:"size"`
	Modified int64 `json:"modified"`
}

type legacyThumbnailRun struct {
	LegacyPath   string          `json:"legacy_path"`
	LegacyDB     legacyFileStamp `json:"legacy_db"`
	LegacyWAL    legacyFileStamp `json:"legacy_wal"`
	ScanSnapshot string          `json:"scan_snapshot"`
	PhotoCount   int64           `json:"photo_count"`
	Cleaned      bool            `json:"cleaned"`
}

func legacyFileState(path string) (legacyFileStamp, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return legacyFileStamp{}, nil
	}
	if err != nil {
		return legacyFileStamp{}, err
	}
	return legacyFileStamp{Size: info.Size(), Modified: info.ModTime().UnixNano()}, nil
}

func currentLegacyThumbnailRun(ctx context.Context, db *sql.DB, legacyPath string, cleaned bool) (legacyThumbnailRun, error) {
	var run legacyThumbnailRun
	path, err := filepath.Abs(legacyPath)
	if err != nil {
		return run, err
	}
	run.LegacyPath = filepath.Clean(path)
	run.LegacyDB, err = legacyFileState(path)
	if err != nil || run.LegacyDB == (legacyFileStamp{}) {
		return run, fmt.Errorf("旧版媒体数据库不存在: %s", path)
	}
	run.LegacyWAL, err = legacyFileState(path + "-wal")
	if err != nil {
		return run, err
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM photos WHERE deleted_at IS NULL`).Scan(&run.PhotoCount); err != nil {
		return run, err
	}
	err = db.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = 'library_scan_snapshot'`).Scan(&run.ScanSnapshot)
	if err != nil && err != sql.ErrNoRows {
		return run, err
	}
	run.Cleaned = cleaned
	return run, nil
}

func (s *DB) SaveLegacyThumbnailRun(ctx context.Context, legacyPath string, cleaned bool) error {
	run, err := currentLegacyThumbnailRun(ctx, s.db, legacyPath, cleaned)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	return s.setMeta(legacyThumbnailRunKey, string(payload))
}

// LegacyThumbnailRunCompleteAtPath only reads metadata; it never opens the
// writable repository or scans thumbnails when checking an exact rerun.
func LegacyThumbnailRunCompleteAtPath(ctx context.Context, newPath, legacyPath string, requireCleaned bool) (bool, error) {
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	path, err := filepath.Abs(newPath)
	if err != nil {
		return false, err
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}).String()
	if len(path) >= 2 && path[1] == ':' {
		uri = (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path), RawQuery: "mode=ro"}).String()
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var payload string
	if err := db.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = ?`, legacyThumbnailRunKey).Scan(&payload); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var previous legacyThumbnailRun
	if err := json.Unmarshal([]byte(payload), &previous); err != nil {
		return false, nil
	}
	current, err := currentLegacyThumbnailRun(ctx, db, legacyPath, previous.Cleaned)
	if err != nil {
		return false, err
	}
	return (!requireCleaned || previous.Cleaned) && previous == current, nil
}
