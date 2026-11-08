package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"echogallery/internal/config"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

var ErrSoundtrackNotFound = errors.New("soundtrack not found")

type SoundtrackCacheEntry struct {
	Track media.AudioTrack `json:"track"`
	URL   string           `json:"url"`
}

// Builders for the same cache file share a gate. Waiters can cancel independently.
var soundtrackBuilds sync.Map

func (s *PhotoService) soundtrackPhoto(id, userID int64) (*storage.Photo, error) {
	photo, err := s.GetPhoto(id, userID)
	if err != nil {
		return nil, err
	}
	if photo == nil || photo.MediaKind != storage.MediaKindVideo {
		return nil, ErrSoundtrackNotFound
	}
	return photo, nil
}

func (s *PhotoService) ListVideoSoundtracks(ctx context.Context, id, userID int64) ([]media.AudioTrack, error) {
	photo, err := s.soundtrackPhoto(id, userID)
	if err != nil {
		return nil, err
	}
	return media.ProbeAudioTracks(ctx, s.MediaPath(photo))
}

func (s *PhotoService) soundtrackCachePath(photo *storage.Photo, index int) (string, error) {
	if photo == nil || photo.MediaKind != storage.MediaKindVideo || index < 0 || index > 1023 {
		return "", ErrSoundtrackNotFound
	}
	source := s.MediaPath(photo)
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	// Read the current file fingerprint, not possibly stale database metadata.
	// Relative identity lets a portable library reuse its caches on another OS.
	relative := photo.SourceRelPath
	if relative == "" {
		relative = photo.StorageRelPath
	}
	relative = strings.ReplaceAll(filepath.ToSlash(relative), `\`, "/")
	key := fmt.Sprintf("aac-v1:%s:%s:%d:%d:%d", photo.UUID, relative, info.Size(), info.ModTime().UnixNano(), index)
	return identifiedMediaCachePath(filepath.Join(config.LibraryDataRoot(s.sourcePath), "soundtracks"), photo.UUID, fmt.Sprintf("%x.mp4", sha256.Sum256([]byte(key)))), nil
}

func (s *PhotoService) SoundtrackPlaybackPath(photo *storage.Photo, index int) (string, error) {
	path, err := s.soundtrackCachePath(photo, index)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", ErrSoundtrackNotFound
	}
	return path, nil
}

func (s *PhotoService) BuildVideoSoundtrack(ctx context.Context, id, userID int64, index int) (*SoundtrackCacheEntry, error) {
	photo, err := s.soundtrackPhoto(id, userID)
	if err != nil {
		return nil, err
	}
	tracks, err := media.ProbeAudioTracks(ctx, s.MediaPath(photo))
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(tracks) {
		return nil, ErrSoundtrackNotFound
	}
	path, err := s.soundtrackCachePath(photo, index)
	if err != nil {
		return nil, err
	}
	entry := &SoundtrackCacheEntry{Track: tracks[index], URL: fmt.Sprintf("/media/playback/%s?soundtrack=%d&cache=%s", photo.UUID, index, filepath.Base(path))}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := s.SoundtrackPlaybackPath(photo, index); err == nil {
			return entry, nil
		}
		done := make(chan struct{})
		previous, loaded := soundtrackBuilds.LoadOrStore(path, done)
		if loaded {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-previous.(chan struct{}):
				continue
			}
		}
		defer func() { soundtrackBuilds.Delete(path); close(done) }()
		break
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".soundtrack-*.mp4")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	defer os.Remove(tmpPath)
	if err := media.BuildSoundtrackMP4(ctx, s.MediaPath(photo), tmpPath, index, browserPlaybackMode(photo) == "transcode"); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A source changed during conversion must never publish a stale cache.
	currentPath, err := s.soundtrackCachePath(photo, index)
	if err != nil {
		return nil, err
	}
	if currentPath != path {
		return nil, fmt.Errorf("source changed during soundtrack conversion")
	}
	if info, err := os.Stat(tmpPath); err != nil || info.Size() == 0 {
		return nil, fmt.Errorf("empty soundtrack cache")
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return nil, err
	}
	return entry, nil
}
