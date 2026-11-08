package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"
)

const photoSearchIndexVersion = "trigram-v1"

func (s *DB) ensurePhotoSearchIndex() error {
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS photos_search USING fts5(
		original_name, uuid, mime_type, media_kind, tokenize='trigram'
	)`); err != nil {
		return fmt.Errorf("创建媒体搜索索引失败: %w", err)
	}
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS photos_search_insert AFTER INSERT ON photos BEGIN
			INSERT INTO photos_search(rowid, original_name, uuid, mime_type, media_kind)
			VALUES (new.id, new.original_name, new.uuid, new.mime_type, new.media_kind);
		END`,
		`CREATE TRIGGER IF NOT EXISTS photos_search_delete AFTER DELETE ON photos BEGIN
			DELETE FROM photos_search WHERE rowid = old.id;
		END`,
		`CREATE TRIGGER IF NOT EXISTS photos_search_update AFTER UPDATE OF original_name, uuid, mime_type, media_kind ON photos BEGIN
			DELETE FROM photos_search WHERE rowid = old.id;
			INSERT INTO photos_search(rowid, original_name, uuid, mime_type, media_kind)
			VALUES (new.id, new.original_name, new.uuid, new.mime_type, new.media_kind);
		END`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("创建媒体搜索同步触发器失败: %w", err)
		}
	}
	if err := s.ensureMetaTable(); err != nil {
		return err
	}
	var version string
	err := s.db.QueryRow(`SELECT value FROM app_meta WHERE key = 'photos_search_index_version'`).Scan(&version)
	if err == nil && version == photoSearchIndexVersion {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM photos_search`); err != nil {
		return fmt.Errorf("清理媒体搜索索引失败: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO photos_search(rowid, original_name, uuid, mime_type, media_kind)
		SELECT id, original_name, uuid, mime_type, media_kind FROM photos`); err != nil {
		return fmt.Errorf("初始化媒体搜索索引失败: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO app_meta(key, value) VALUES ('photos_search_index_version', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, photoSearchIndexVersion); err != nil {
		return err
	}
	return tx.Commit()
}

func photoSearchMatch(query string) string {
	quoted := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	return "original_name : " + quoted + " OR uuid : " + quoted + " OR mime_type : " + quoted + " OR media_kind : " + quoted
}

func canUsePhotoSearchIndex(query string) bool {
	return utf8.RuneCountInString(query) >= 3
}
