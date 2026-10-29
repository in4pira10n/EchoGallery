package service

import (
	"context"
	"echogallery/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalSRTSubtitleCachePreservesCaptionCommas(t *testing.T) {
	source := t.TempDir()
	repo := newMockRepo()
	svc := newPhotoServiceSync(repo, source, t.TempDir(), t.TempDir())
	photo := saveTestVideo(t, svc, repo, 1, "clip.mp4")
	if err := os.WriteFile(svc.MediaPath(photo), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "clip.srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nHello, world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry, err := svc.BuildVideoSubtitle(context.Background(), photo.ID, 1, -1)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Track.Index != -1 || entry.URL == "" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	cacheDir := filepath.Join(config.LibraryDataRoot(source), "subtitles")
	files, err := filepath.Glob(filepath.Join(cacheDir, "*.vtt"))
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "WEBVTT\n\n1\n00:00:00.000 --> 00:00:01.000 line:88 align:center\nHello, world\n" {
		t.Fatalf("unexpected vtt: %q", data)
	}
}
