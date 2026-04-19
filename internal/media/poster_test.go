package media

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	imgpkg "echogallery/internal/image"
)

func TestPosterPath(t *testing.T) {
	got := PosterPath("/tmp/storage", "video-1")
	want := filepath.Join("/tmp/storage", "video-1.jpg")
	if got != want {
		t.Fatalf("期望 %s，得到 %s", want, got)
	}
}

func TestGeneratePosterWithRunner_Success(t *testing.T) {
	dir := t.TempDir()
	poster := filepath.Join(dir, "demo.jpg")
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		framePath := args[len(args)-1]
		if err := os.MkdirAll(filepath.Dir(framePath), 0755); err != nil {
			return nil, err
		}
		f, err := os.Create(framePath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
		if err := jpeg.Encode(f, img, nil); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(poster), 0755); err != nil {
			return nil, err
		}
		return []byte("ok"), nil
	}

	if err := generatePosterWithRunner("demo.mp4", poster, &VideoMeta{DurationMS: 120000}, runner, imgpkg.DefaultThumbnailLongEdge); err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	if _, err := os.Stat(poster); err != nil {
		t.Fatalf("poster 文件应存在: %v", err)
	}
}

func TestGeneratePosterWithRunner_FFmpegUnavailable(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	err := generatePosterWithRunner("demo.mp4", filepath.Join(t.TempDir(), "p.jpg"), &VideoMeta{DurationMS: 120000}, runner, imgpkg.DefaultThumbnailLongEdge)
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("期望 ffmpeg 缺失错误，得到 %v", err)
	}
}

func TestPickFrameTimestamp_WithinPlayableRange(t *testing.T) {
	got := pickFrameTimestamp(&VideoMeta{DurationMS: 10000})
	if got < "00:00:01.000" || got > "00:00:09.000" {
		t.Fatalf("期望随机时间点位于 10%%-90%% 区间，得到 %s", got)
	}
}
