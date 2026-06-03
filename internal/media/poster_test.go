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
	want := filepath.Join("/tmp/storage", "vi", "de", "video-1.webp")
	if got != want {
		t.Fatalf("期望 %s，得到 %s", want, got)
	}
}

func TestGeneratePosterWithRunner_Success(t *testing.T) {
	dir := t.TempDir()
	poster := filepath.Join(dir, "demo.webp")
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

func TestGeneratePosterVariantsWithRunner_SingleFrameMultiOutput(t *testing.T) {
	dir := t.TempDir()
	preview := filepath.Join(dir, "demo.preview.webp")
	full := filepath.Join(dir, "demo.webp")
	calls := 0
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		framePath := args[len(args)-1]
		if err := os.MkdirAll(filepath.Dir(framePath), 0755); err != nil {
			return nil, err
		}
		f, err := os.Create(framePath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
		if err := jpeg.Encode(f, img, nil); err != nil {
			return nil, err
		}
		return []byte("ok"), nil
	}

	err := generatePosterVariantsWithRunner("demo.mp4", []imgpkg.ThumbnailVariant{
		{DestPath: preview, MaxEdge: 256, Quality: 58, Method: 2},
		{DestPath: full, MaxEdge: 512},
	}, &VideoMeta{DurationMS: 120000}, runner, 512)
	if err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	if calls != 1 {
		t.Fatalf("期望只抽帧一次，实际 %d 次", calls)
	}
	if _, err := os.Stat(preview); err != nil {
		t.Fatalf("预览缩略图应存在: %v", err)
	}
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("标准缩略图应存在: %v", err)
	}
}

func TestGeneratePosterWithRunner_FFmpegUnavailable(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	err := generatePosterWithRunner("demo.mp4", filepath.Join(t.TempDir(), "p.webp"), &VideoMeta{DurationMS: 120000}, runner, imgpkg.DefaultThumbnailLongEdge)
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
