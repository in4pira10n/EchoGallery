package sqlite

import (
	"context"
	"fmt"
)

func (s *DB) ensureLegacyThumbnailProgressTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS legacy_thumbnail_progress (
		uuid TEXT PRIMARY KEY,
		source_rel_path TEXT NOT NULL
	)`)
	return err
}

// LegacyThumbnailCompleted returns verified copies for this library. The final
// audit still checks the destination files before reporting completion.
func (s *DB) LegacyThumbnailCompleted(ctx context.Context) (map[string]string, error) {
	if err := s.ensureLegacyThumbnailProgressTable(ctx); err != nil {
		return nil, fmt.Errorf("创建缩略图迁移记录失败: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT uuid, source_rel_path FROM legacy_thumbnail_progress`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	completed := make(map[string]string)
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		completed[id] = path
	}
	return completed, rows.Err()
}

func (s *DB) RecordLegacyThumbnailProgress(ctx context.Context, completed map[string]string) error {
	if len(completed) == 0 {
		return nil
	}
	if err := s.ensureLegacyThumbnailProgressTable(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO legacy_thumbnail_progress(uuid, source_rel_path)
		VALUES(?, ?) ON CONFLICT(uuid) DO UPDATE SET source_rel_path = excluded.source_rel_path`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for id, path := range completed {
		if _, err := stmt.ExecContext(ctx, id, path); err != nil {
			return err
		}
	}
	return tx.Commit()
}
