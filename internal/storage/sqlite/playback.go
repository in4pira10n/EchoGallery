package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"echogallery/internal/storage"
)

// GetVideoPlaybackPreference 获取单个视频的共享播放偏好。
func (s *DB) GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error) {
	_ = userID
	row := s.db.QueryRow(`
		SELECT photo_id, volume, muted, resume_time, bookmarks_json, updated_at
		FROM video_playback_preferences
		WHERE photo_id = ?`, photoID)

	var pref storage.VideoPlaybackPreference
	var muted int
	var bookmarksJSON string
	if err := row.Scan(&pref.PhotoID, &pref.Volume, &muted, &pref.ResumeTime, &bookmarksJSON, &pref.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	pref.Muted = muted != 0
	pref.Bookmarks = decodePlaybackBookmarks(bookmarksJSON)
	return &pref, nil
}

// UpsertVideoPlaybackPreference 保存单个视频的共享播放偏好。
func (s *DB) UpsertVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool, resumeTime int64, bookmarks []storage.VideoPlaybackBookmark) (*storage.VideoPlaybackPreference, error) {
	_ = userID
	updatedAt := time.Now()
	mutedValue := 0
	if muted {
		mutedValue = 1
	}
	normalizedBookmarks := normalizePlaybackBookmarks(bookmarks)
	bookmarksJSON := encodePlaybackBookmarks(normalizedBookmarks)
	if _, err := s.db.Exec(`
		INSERT INTO video_playback_preferences (photo_id, volume, muted, resume_time, bookmarks_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(photo_id) DO UPDATE SET
		    volume = excluded.volume,
		    muted = excluded.muted,
		    resume_time = excluded.resume_time,
		    bookmarks_json = excluded.bookmarks_json,
		    updated_at = excluded.updated_at`,
		photoID, volume, mutedValue, resumeTime, bookmarksJSON, updatedAt,
	); err != nil {
		return nil, fmt.Errorf("保存视频播放偏好失败: %w", err)
	}
	return &storage.VideoPlaybackPreference{
		PhotoID:    photoID,
		Volume:     volume,
		Muted:      muted,
		ResumeTime: resumeTime,
		Bookmarks:  normalizedBookmarks,
		UpdatedAt:  updatedAt,
	}, nil
}

func encodePlaybackBookmarks(bookmarks []storage.VideoPlaybackBookmark) string {
	if len(bookmarks) == 0 {
		return ""
	}
	data, err := json.Marshal(normalizePlaybackBookmarks(bookmarks))
	if err != nil {
		return ""
	}
	return string(data)
}

func decodePlaybackBookmarks(raw string) []storage.VideoPlaybackBookmark {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var bookmarks []storage.VideoPlaybackBookmark
	if err := json.Unmarshal([]byte(raw), &bookmarks); err != nil {
		return nil
	}
	return normalizePlaybackBookmarks(bookmarks)
}

func normalizePlaybackBookmarks(bookmarks []storage.VideoPlaybackBookmark) []storage.VideoPlaybackBookmark {
	if len(bookmarks) == 0 {
		return nil
	}
	normalized := make([]storage.VideoPlaybackBookmark, 0, len(bookmarks))
	seen := make(map[int]struct{}, len(bookmarks))
	for _, bookmark := range bookmarks {
		slot := bookmark.Slot
		if slot < 1 || slot > 10 {
			continue
		}
		if _, exists := seen[slot]; exists {
			continue
		}
		seen[slot] = struct{}{}
		timeValue := bookmark.Time
		if timeValue < 0 {
			timeValue = 0
		}
		normalized = append(normalized, storage.VideoPlaybackBookmark{
			Slot: slot,
			Time: timeValue,
			Name: strings.TrimSpace(bookmark.Name),
		})
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Slot < normalized[j].Slot
	})
	return normalized
}
