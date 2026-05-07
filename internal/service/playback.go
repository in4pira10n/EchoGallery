package service

import (
	"fmt"
	"math"

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
	if pref != nil {
		return pref, nil
	}
	return &storage.VideoPlaybackPreference{PhotoID: photoID, Volume: 1, Muted: false}, nil
}

// SaveVideoPlaybackPreference 保存单个视频的音量/静音偏好。
func (s *PhotoService) SaveVideoPlaybackPreference(photoID int64, userID int64, volume float64, muted bool) (*storage.VideoPlaybackPreference, error) {
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
	return s.repo.UpsertVideoPlaybackPreference(photoID, userID, normalizeVideoVolume(volume), muted)
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
