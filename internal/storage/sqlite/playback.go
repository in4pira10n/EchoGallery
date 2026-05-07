package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"echogallery/internal/storage"
)

// GetVideoPlaybackPreference 获取指定用户在单个视频上的播放偏好。
func (s *DB) GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error) {
	row := s.db.QueryRow(`
		SELECT photo_id, volume, muted, updated_at
		FROM video_playback_preferences
		WHERE photo_id = ? AND user_id = ?`, photoID, userID)

	var pref storage.VideoPlaybackPreference
	var muted int
	if err := row.Scan(&pref.PhotoID, &pref.Volume, &muted, &pref.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	pref.Muted = muted != 0
	return &pref, nil
}

// UpsertVideoPlaybackPreference 保存指定用户在单个视频上的播放偏好。
func (s *DB) UpsertVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool) (*storage.VideoPlaybackPreference, error) {
	updatedAt := time.Now()
	mutedValue := 0
	if muted {
		mutedValue = 1
	}
	if _, err := s.db.Exec(`
		INSERT INTO video_playback_preferences (photo_id, user_id, volume, muted, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(photo_id, user_id) DO UPDATE SET
		    volume = excluded.volume,
		    muted = excluded.muted,
		    updated_at = excluded.updated_at`,
		photoID, userID, volume, mutedValue, updatedAt,
	); err != nil {
		return nil, fmt.Errorf("保存视频播放偏好失败: %w", err)
	}
	return &storage.VideoPlaybackPreference{
		PhotoID:   photoID,
		Volume:    volume,
		Muted:     muted,
		UpdatedAt: updatedAt,
	}, nil
}
