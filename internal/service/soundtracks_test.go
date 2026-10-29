package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"echogallery/internal/config"
)

func TestSoundtrackCacheBuildReuseAndSourceInvalidation(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := newMockRepo()
	svc := newPhotoServiceSync(repo, t.TempDir(), t.TempDir(), t.TempDir())
	photo := saveTestVideo(t, svc, repo, 1, "two tracks.mp4")
	source := svc.MediaPath(photo)
	out, err := exec.CommandContext(ctx, "ffmpeg", "-y", "-v", "error", "-f", "lavfi", "-i", "color=size=32x32:rate=10:duration=0.5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=0.5", "-f", "lavfi", "-i", "sine=frequency=880:duration=0.5",
		"-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-metadata:s:a:0", "language=zho", "-metadata:s:a:1", "language=eng", "-disposition:a:0", "0", "-disposition:a:1", "default", source).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	before, _ := os.ReadFile(source)
	beforeInfo, _ := os.Stat(source)
	tracks, err := svc.ListVideoSoundtracks(ctx, photo.ID, 1)
	if err != nil || len(tracks) != 2 || !tracks[1].Default {
		t.Fatalf("tracks=%+v error=%v", tracks, err)
	}
	if _, err := svc.BuildVideoSoundtrack(ctx, photo.ID, 2, 0); !errors.Is(err, ErrSoundtrackNotFound) {
		t.Fatalf("cross-user access allowed: %v", err)
	}
	if _, err := svc.BuildVideoSoundtrack(ctx, photo.ID, 1, 9); !errors.Is(err, ErrSoundtrackNotFound) {
		t.Fatalf("invalid track: %v", err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if _, err := svc.BuildVideoSoundtrack(ctx, photo.ID, 1, 0); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	entry, err := svc.BuildVideoSoundtrack(ctx, photo.ID, 1, 0)
	if err != nil || entry.Track.Index != 0 {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
	path, err := svc.SoundtrackPlaybackPath(photo, 0)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(config.LibraryDataRoot(svc.sourcePath), "soundtracks") {
		t.Fatalf("wrong cache location: %s", path)
	}
	info, _ := os.Stat(path)
	if _, err := svc.BuildVideoSoundtrack(ctx, photo.ID, 1, 0); err != nil {
		t.Fatal(err)
	}
	reused, _ := os.Stat(path)
	if !info.ModTime().Equal(reused.ModTime()) {
		t.Fatal("cache rebuilt unnecessarily")
	}
	out, err = exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-of", "json", path).CombinedOutput()
	if err != nil {
		t.Fatalf("probe cache: %v %s", err, out)
	}
	var probe struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatal(err)
	}
	audioCount := 0
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			audioCount++
			if stream.CodecName != "aac" {
				t.Fatal("cache audio is not AAC")
			}
		}
	}
	if audioCount != 1 {
		t.Fatalf("cache contains %d audio tracks", audioCount)
	}
	after, _ := os.ReadFile(source)
	afterInfo, _ := os.Stat(source)
	if sha256.Sum256(before) != sha256.Sum256(after) || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("source modified")
	}
	if err := os.Chtimes(source, beforeInfo.ModTime(), beforeInfo.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SoundtrackPlaybackPath(photo, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale cache served: %v", err)
	}
	if _, err := svc.BuildVideoSoundtrack(ctx, photo.ID, 1, 0); err != nil {
		t.Fatal(err)
	}
	next, _ := svc.SoundtrackPlaybackPath(photo, 0)
	if path == next {
		t.Fatal("source fingerprint did not invalidate cache")
	}
	temps, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".soundtrack-*"))
	if len(temps) != 0 {
		t.Fatalf("temporary files retained: %v", temps)
	}
}
