package service

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"echogallery/internal/storage"
)

// GetVideoPlaybackPreference 获取单个视频的音量/静音偏好。
func (s *PhotoService) GetVideoPlaybackPreference(photoID int64, userID int64) (*storage.VideoPlaybackPreference, error) {
	photo, err := s.GetPhoto(photoID, userID)
	if err != nil {
		return nil, err
	}
	if photo == nil {
		return nil, fmt.Errorf("照片/视频不存在")
	}
	if photo.MediaKind != storage.MediaKindVideo {
		return nil, fmt.Errorf("当前媒体不是视频")
	}
	pref, err := s.repo.GetVideoPlaybackPreference(photoID, userID)
	if err != nil {
		return nil, err
	}
	volume := 1.0
	if libraryVolume, ok, err := s.repo.GetLibraryVideoVolume(); err == nil && ok {
		volume = normalizeVideoVolume(libraryVolume)
	} else if pref != nil {
		volume = normalizeVideoVolume(pref.Volume)
	}
	if pref != nil {
		pref.Volume = volume
		pref.ResumeTime = normalizeVideoResumeTime(pref.ResumeTime)
		pref.Bookmarks = normalizeVideoBookmarks(pref.Bookmarks)
		return pref, nil
	}
	return &storage.VideoPlaybackPreference{
		PhotoID:    photoID,
		Volume:     volume,
		Muted:      false,
		ResumeTime: 0,
		Bookmarks:  nil,
	}, nil
}

// SaveVideoPlaybackPreference 保存单个视频的音量/静音偏好。
func (s *PhotoService) SaveVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool, resumeTime int64, bookmarks []storage.VideoPlaybackBookmark) (*storage.VideoPlaybackPreference, error) {
	photo, err := s.GetPhoto(photoID, userID)
	if err != nil {
		return nil, err
	}
	if photo == nil {
		return nil, fmt.Errorf("照片/视频不存在")
	}
	if photo.MediaKind != storage.MediaKindVideo {
		return nil, fmt.Errorf("当前媒体不是视频")
	}
	normalizedVolume := normalizeVideoVolume(volume)
	if err := s.repo.SaveLibraryVideoVolume(normalizedVolume); err != nil {
		return nil, err
	}
	return s.repo.UpsertVideoPlaybackPreference(photoID, userID, normalizedVolume, muted, normalizeVideoResumeTime(resumeTime), normalizeVideoBookmarks(bookmarks))
}

func normalizeVideoVolume(volume float64) float64 {
	if math.IsNaN(volume) || math.IsInf(volume, 0) {
		return 1
	}
	if volume < 0 {
		return 0
	}
	if volume > 1 {
		return 1
	}
	return volume
}

func normalizeVideoResumeTime(seconds int64) int64 {
	if seconds < 0 {
		return 0
	}
	return seconds
}

func normalizeVideoBookmarks(bookmarks []storage.VideoPlaybackBookmark) []storage.VideoPlaybackBookmark {
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
