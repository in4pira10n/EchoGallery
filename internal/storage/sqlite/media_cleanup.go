package sqlite

import (
	"context"
	"fmt"

	"echogallery/internal/storage"
)

// HardDeleteMissingPhoto removes the indexed identity and its non-FK references
// atomically. A moved, changed or trashed record must not be deleted by an old audit.
func (s *DB) HardDeleteMissingPhoto(ctx context.Context, photo *storage.Photo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM photos
		WHERE id = ? AND uploaded_by = ? AND uuid = ? AND source_rel_path = ?
		AND size = ? AND source_mod_unix = ? AND deleted_at IS NULL`,
		photo.ID, photo.UploadedBy, photo.UUID, photo.SourceRelPath, photo.Size, photo.SourceModUnix)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("media changed during cleanup: %s", photo.UUID)
	}
	// Album membership and playback preferences cascade; album covers become NULL.
	// Share targets are polymorphic, so they deliberately have no foreign key.
	if _, err := tx.ExecContext(ctx, `DELETE FROM share_links WHERE type = ? AND target_id = ?`, storage.ShareTypePhoto, photo.ID); err != nil {
		return err
	}
	return tx.Commit()
}
