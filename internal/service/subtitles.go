package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"echogallery/internal/config"
	"echogallery/internal/media"
	"echogallery/internal/storage"
)

var ErrSubtitleNotFound = errors.New("subtitle not found")

type SubtitleCacheEntry struct {
	Track media.SubtitleTrack `json:"track"`
	URL   string              `json:"url"`
}

func (s *PhotoService) subtitlePhoto(id, userID int64) (*storage.Photo, error) {
	p, err := s.GetPhoto(id, userID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.MediaKind != storage.MediaKindVideo {
		return nil, ErrSubtitleNotFound
	}
	return p, nil
}

func (s *PhotoService) ListVideoSubtitles(ctx context.Context, id, userID int64) ([]media.SubtitleTrack, error) {
	p, err := s.subtitlePhoto(id, userID)
	if err != nil {
		return nil, err
	}
	source := s.MediaPath(p)
	external := media.ExternalSRTPath(source)
	tracks, probeErr := media.ProbeSubtitleTracks(ctx, source)
	if probeErr != nil && !fileExists(external) {
		return nil, probeErr
	}
	if fileExists(external) {
		tracks = append([]media.SubtitleTrack{{Index: -1, Label: filepath.Base(external), Format: "srt"}}, tracks...)
	}
	return tracks, nil
}

func (s *PhotoService) subtitleCachePath(p *storage.Photo, index int) (string, error) {
	if p == nil {
		return "", ErrSubtitleNotFound
	}
	source := s.MediaPath(p)
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	// Bump the cache generation when cue positioning changes so existing
	// subtitle files are regenerated with the new native WebVTT placement.
	key := fmt.Sprintf("srt-v2-positioned:%s:%s:%d:%d:%d", p.UUID, p.SourceRelPath, info.Size(), info.ModTime().UnixNano(), index)
	return filepath.Join(config.LibraryDataRoot(s.sourcePath), "subtitles", fmt.Sprintf("%x.vtt", sha256.Sum256([]byte(key)))), nil
}

func (s *PhotoService) BuildVideoSubtitle(ctx context.Context, id, userID int64, index int) (*SubtitleCacheEntry, error) {
	p, err := s.subtitlePhoto(id, userID)
	if err != nil {
		return nil, err
	}
	tracks, err := s.ListVideoSubtitles(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	var track media.SubtitleTrack
	for _, candidate := range tracks {
		if candidate.Index == index {
			track = candidate
			break
		}
	}
	if track.Format == "" {
		return nil, ErrSubtitleNotFound
	}
	path, err := s.subtitleCachePath(p, index)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		return &SubtitleCacheEntry{Track: track, URL: fmt.Sprintf("/media/subtitles/%s?index=%d", p.UUID, index)}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".subtitle-*.vtt")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if index == -1 {
		input := media.ExternalSRTPath(s.MediaPath(p))
		data, readErr := os.ReadFile(input)
		if readErr != nil {
			return nil, readErr
		}
		// WebVTT accepts SRT timing after the header; normalize only the required separator.
		content := "WEBVTT\n\n" + strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
		// Convert only SRT timestamp separators; commas in caption text are kept.
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, " --> ") {
				line = strings.ReplaceAll(line, ",", ".")
				if !strings.Contains(line, " line:") {
					line += " line:88 align:center"
				}
				lines[i] = line
			}
		}
		content = strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
		if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
			return nil, err
		}
	} else if err := media.ExtractSubtitleWebVTT(ctx, s.MediaPath(p), tmpPath, index); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return nil, err
	}
	return &SubtitleCacheEntry{Track: track, URL: fmt.Sprintf("/media/subtitles/%s?index=%d", p.UUID, index)}, nil
}

func (s *PhotoService) VideoSubtitlePath(uuid string, userID int64, index int) (string, error) {
	p, err := s.GetPhotoByUUID(uuid, userID)
	if err != nil || p == nil {
		return "", ErrSubtitleNotFound
	}
	path, err := s.subtitleCachePath(p, index)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", ErrSubtitleNotFound
	}
	return path, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
