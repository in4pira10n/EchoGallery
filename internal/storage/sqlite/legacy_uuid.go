package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"echogallery/internal/storage"
)

type legacyMediaRow struct {
	id   int64
	uuid string
	path string
	kind string
	size int64
}

func normalizedLegacyMediaPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	value = strings.Trim(value, "/")
	if value == "" || value == "." || strings.HasPrefix(value, "../") {
		return ""
	}
	return value
}

func readLegacyMediaRows(ctx context.Context, db *sql.DB) ([]legacyMediaRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, uuid, source_rel_path, media_kind, size FROM photos WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []legacyMediaRow
	for rows.Next() {
		var row legacyMediaRow
		if err := rows.Scan(&row.id, &row.uuid, &row.path, &row.kind, &row.size); err != nil {
			return nil, err
		}
		row.path = normalizedLegacyMediaPath(row.path)
		if row.path == "" {
			return nil, fmt.Errorf("媒体 #%d 缺少可匹配的源路径", row.id)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// RestoreLegacyUUIDs matches media by library-relative path before changing any UUID.
// It leaves the old database read-only and updates the new database atomically.
func (s *DB) RestoreLegacyUUIDs(ctx context.Context, legacyPath string) ([]storage.LegacyUUIDMatch, error) {
	report, err := s.RestoreLegacyUUIDsWithReport(ctx, legacyPath)
	return report.Matches, err
}

func (s *DB) RestoreLegacyUUIDsWithReport(ctx context.Context, legacyPath string) (storage.LegacyUUIDReport, error) {
	var report storage.LegacyUUIDReport
	if s == nil || strings.TrimSpace(legacyPath) == "" {
		return report, fmt.Errorf("旧版数据库路径不可用")
	}
	if info, err := os.Stat(legacyPath); err != nil || info.IsDir() {
		return report, fmt.Errorf("旧版媒体数据库不存在: %s", legacyPath)
	}
	absPath, err := filepath.Abs(legacyPath)
	if err != nil {
		return report, err
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath), RawQuery: "mode=ro"}).String()
	if len(absPath) >= 2 && absPath[1] == ':' {
		uri = (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(absPath), RawQuery: "mode=ro"}).String()
	}
	legacy, err := sql.Open("sqlite", uri)
	if err != nil {
		return report, fmt.Errorf("只读打开旧版数据库失败: %w", err)
	}
	defer legacy.Close()
	var deletedCount int
	if err := legacy.QueryRowContext(ctx, `SELECT COUNT(*) FROM photos WHERE deleted_at IS NOT NULL`).Scan(&deletedCount); err != nil {
		return report, fmt.Errorf("核对旧版回收站记录失败: %w", err)
	}
	if deletedCount > 0 {
		return report, fmt.Errorf("旧版数据库有 %d 条回收站记录，当前无法准确迁移其缩略图；旧文件未改动", deletedCount)
	}
	oldRows, err := readLegacyMediaRows(ctx, legacy)
	if err != nil {
		return report, fmt.Errorf("读取旧版媒体映射失败: %w", err)
	}
	oldByPath := make(map[string]legacyMediaRow, len(oldRows))
	ambiguous := make(map[string]struct{})
	for _, row := range oldRows {
		if _, exists := oldByPath[row.path]; exists {
			ambiguous[row.path] = struct{}{}
			continue
		}
		oldByPath[row.path] = row
	}
	newRows, err := readLegacyMediaRows(ctx, s.db)
	if err != nil {
		return report, fmt.Errorf("读取新版媒体映射失败: %w", err)
	}
	newByPath := make(map[string]legacyMediaRow, len(newRows))
	newByUUID := make(map[string]string, len(newRows))
	for _, row := range newRows {
		newByUUID[row.uuid] = row.path
		if _, exists := newByPath[row.path]; exists {
			ambiguous[row.path] = struct{}{}
			continue
		}
		newByPath[row.path] = row
	}
	for path, old := range oldByPath {
		if _, skip := ambiguous[path]; skip {
			continue
		}
		current, ok := newByPath[path]
		if !ok {
			return report, fmt.Errorf("旧版媒体未出现在新版资源库扫描结果中: %s", path)
		}
		if current.kind != old.kind || current.size != old.size {
			return report, fmt.Errorf("旧版媒体与新版文件不一致: %s", path)
		}
		if _, err := uuid.Parse(old.uuid); err != nil {
			return report, fmt.Errorf("媒体 %s 的旧 UUID 无效: %w", path, err)
		}
		if other, exists := newByUUID[old.uuid]; exists && other != path {
			if _, duplicate := ambiguous[other]; duplicate {
				ambiguous[path] = struct{}{}
				continue
			}
			return report, fmt.Errorf("旧 UUID %s 已属于其他媒体: %s", old.uuid, other)
		}
		report.Matches = append(report.Matches, storage.LegacyUUIDMatch{SourceRelPath: path, OldUUID: old.uuid, NewUUID: current.uuid})
	}
	for path := range ambiguous {
		report.SkippedPaths = append(report.SkippedPaths, path)
	}
	sort.Strings(report.SkippedPaths)
	sort.Slice(report.Matches, func(i, j int) bool { return report.Matches[i].SourceRelPath < report.Matches[j].SourceRelPath })
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	for _, match := range report.Matches {
		if match.OldUUID == match.NewUUID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photos SET uuid = ? WHERE id = ?`, "migration-"+uuid.NewString(), newByPath[match.SourceRelPath].id); err != nil {
			return report, fmt.Errorf("暂存新版 UUID 失败: %w", err)
		}
	}
	for _, match := range report.Matches {
		if match.OldUUID == match.NewUUID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photos SET uuid = ? WHERE id = ?`, match.OldUUID, newByPath[match.SourceRelPath].id); err != nil {
			return report, fmt.Errorf("恢复旧 UUID 失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("提交旧 UUID 映射失败: %w", err)
	}
	return report, nil
}
