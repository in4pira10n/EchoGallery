package sqlite

// Portable libraries own their media independently of a host's account IDs.
// Keep the oldest UUID and union state when earlier scans created duplicates.
func (s *DB) reconcilePortableMedia() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`UPDATE photos SET source_rel_path = replace(source_rel_path, char(92), '/') WHERE instr(source_rel_path, char(92)) > 0`,
		`CREATE TEMP TABLE portable_duplicates AS
            SELECT p.id AS old_id, k.id AS keep_id FROM photos p
            JOIN (SELECT source_rel_path, MIN(id) AS id FROM photos
                  WHERE source_rel_path <> '' GROUP BY source_rel_path HAVING COUNT(*) > 1) k
            ON p.source_rel_path = k.source_rel_path WHERE p.id <> k.id`,
		`UPDATE photos SET
            deleted_at = COALESCE(deleted_at, (SELECT MAX(p.deleted_at) FROM photos p JOIN portable_duplicates d ON p.id=d.old_id WHERE d.keep_id=photos.id)),
            deleted_by = COALESCE(deleted_by, (SELECT p.deleted_by FROM photos p JOIN portable_duplicates d ON p.id=d.old_id WHERE d.keep_id=photos.id AND p.deleted_at IS NOT NULL LIMIT 1)),
            deleted_group_id = CASE WHEN deleted_group_id <> '' THEN deleted_group_id ELSE COALESCE((SELECT p.deleted_group_id FROM photos p JOIN portable_duplicates d ON p.id=d.old_id WHERE d.keep_id=photos.id AND p.deleted_group_id <> '' LIMIT 1),'') END,
            is_favorite = MAX(is_favorite, COALESCE((SELECT MAX(p.is_favorite) FROM photos p JOIN portable_duplicates d ON p.id=d.old_id WHERE d.keep_id=photos.id),0)),
            is_super_favorite = MAX(is_super_favorite, COALESCE((SELECT MAX(p.is_super_favorite) FROM photos p JOIN portable_duplicates d ON p.id=d.old_id WHERE d.keep_id=photos.id),0))
            WHERE id IN (SELECT keep_id FROM portable_duplicates)`,
		`INSERT OR IGNORE INTO album_photos(album_id, photo_id, added_at)
            SELECT a.album_id, d.keep_id, a.added_at FROM album_photos a JOIN portable_duplicates d ON a.photo_id=d.old_id`,
		`UPDATE albums SET cover_photo_id=(SELECT keep_id FROM portable_duplicates WHERE old_id=cover_photo_id)
            WHERE cover_photo_id IN (SELECT old_id FROM portable_duplicates)`,
		`UPDATE share_links SET target_id=(SELECT keep_id FROM portable_duplicates WHERE old_id=target_id)
            WHERE type='photo' AND target_id IN (SELECT old_id FROM portable_duplicates)`,
		`INSERT OR IGNORE INTO video_playback_preferences(photo_id,volume,muted,resume_time,bookmarks_json,updated_at)
            SELECT d.keep_id,p.volume,p.muted,p.resume_time,p.bookmarks_json,p.updated_at
            FROM video_playback_preferences p JOIN portable_duplicates d ON p.photo_id=d.old_id ORDER BY p.updated_at DESC`,
		`DELETE FROM photos WHERE id IN (SELECT old_id FROM portable_duplicates)`,
		`DROP TABLE portable_duplicates`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_portable_media_source_unique ON photos(source_rel_path) WHERE source_rel_path <> ''`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE photos SET uploaded_by=? WHERE uploaded_by<>?`, s.libraryUserID, s.libraryUserID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE albums SET created_by=? WHERE created_by<>? AND EXISTS(SELECT 1 FROM photos)`, s.libraryUserID, s.libraryUserID); err != nil {
		return err
	}
	return tx.Commit()
}
