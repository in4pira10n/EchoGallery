package media

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	imgpkg "echogallery/internal/image"
)

var ErrPosterGeneratorUnavailable = errors.New("ffmpeg 不可用")

func PosterPath(storagePath, uuid string) string {
	return imgpkg.ThumbnailShardPath(storagePath, uuid)
}

func GeneratePoster(videoPath, posterPath string, maxEdge int) error {
	return GeneratePosterContext(context.Background(), videoPath, posterPath, maxEdge)
}

func GeneratePosterContext(ctx context.Context, videoPath, posterPath string, maxEdge int) error {
	return GeneratePosterVariantsContext(ctx, videoPath, []imgpkg.ThumbnailVariant{{
		DestPath: posterPath,
		MaxEdge:  maxEdge,
	}})
}

// GenerateRandomPosterContext requires a known duration and always samples a
// random frame. Unlike ordinary on-demand posters, it never falls back to a
// fixed timestamp when probing fails.
func GenerateRandomPosterContext(ctx context.Context, videoPath, posterPath string, maxEdge int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	meta, err := ProbeVideoContext(ctx, videoPath)
	if err != nil {
		return fmt.Errorf("无法读取视频时长，未替换旧缩略图: %w", err)
	}
	if meta == nil || meta.DurationMS <= 0 {
		return fmt.Errorf("无法确定视频时长，未替换旧缩略图")
	}
	variants, maxEdge := normalizePosterVariants([]imgpkg.ThumbnailVariant{{DestPath: posterPath, MaxEdge: maxEdge}})
	if len(variants) == 0 {
		return fmt.Errorf("视频缩略图目标路径无效")
	}
	if err := generatePosterVariantsWithRunnerContext(ctx, videoPath, variants, meta, execPosterRunner, maxEdge); err == nil {
		return nil
	} else if errors.Is(err, ErrPosterGeneratorUnavailable) {
		return err
	} else {
		return err
	}
}

func GeneratePosterVariants(videoPath string, variants []imgpkg.ThumbnailVariant) error {
	return GeneratePosterVariantsContext(context.Background(), videoPath, variants)
}

func GeneratePosterVariantsContext(ctx context.Context, videoPath string, variants []imgpkg.ThumbnailVariant) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	filtered, maxEdge := normalizePosterVariants(variants)
	if len(filtered) == 0 {
		return nil
	}
	meta, _ := ProbeVideoContext(ctx, videoPath)
	err := generatePosterVariantsWithRunnerContext(ctx, videoPath, filtered, meta, execPosterRunner, maxEdge)
	if err == nil {
		return nil
	}
	if runtime.GOOS == "darwin" && errors.Is(err, ErrPosterGeneratorUnavailable) {
		if qlErr := generatePosterVariantsWithQuickLookContext(ctx, videoPath, filtered, maxEdge); qlErr == nil {
			return nil
		}
	}
	return err
}

func normalizePosterVariants(variants []imgpkg.ThumbnailVariant) ([]imgpkg.ThumbnailVariant, int) {
	filtered := make([]imgpkg.ThumbnailVariant, 0, len(variants))
	maxEdge := 0
	for _, variant := range variants {
		if strings.TrimSpace(variant.DestPath) == "" {
			continue
		}
		if variant.MaxEdge <= 0 {
			variant.MaxEdge = imgpkg.DefaultThumbnailLongEdge
		}
		if variant.MaxEdge > maxEdge {
			maxEdge = variant.MaxEdge
		}
		filtered = append(filtered, variant)
	}
	if maxEdge <= 0 {
		maxEdge = imgpkg.DefaultThumbnailLongEdge
	}
	return filtered, maxEdge
}

func generatePosterWithRunner(videoPath, posterPath string, meta *VideoMeta, runner commandRunner, maxEdge int) error {
	return generatePosterVariantsWithRunner(videoPath, []imgpkg.ThumbnailVariant{{
		DestPath: posterPath,
		MaxEdge:  maxEdge,
	}}, meta, runner, maxEdge)
}

func generatePosterVariantsWithRunner(videoPath string, variants []imgpkg.ThumbnailVariant, meta *VideoMeta, runner commandRunner, maxEdge int) error {
	return generatePosterVariantsWithRunnerContext(context.Background(), videoPath, variants, meta, runner, maxEdge)
}

func generatePosterVariantsWithRunnerContext(parent context.Context, videoPath string, variants []imgpkg.ThumbnailVariant, meta *VideoMeta, runner commandRunner, maxEdge int) error {
	if err := parent.Err(); err != nil {
		return err
	}
	for _, variant := range variants {
		if err := os.MkdirAll(filepath.Dir(variant.DestPath), 0755); err != nil {
			return fmt.Errorf("创建 poster 目录失败: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	tmpFrame, err := os.CreateTemp("", "video-frame-*.jpg")
	if err != nil {
		return fmt.Errorf("创建临时视频帧失败: %w", err)
	}
	tmpFramePath := tmpFrame.Name()
	_ = tmpFrame.Close()
	defer os.Remove(tmpFramePath)

	_, err = runner(ctx, "ffmpeg",
		"-y",
		"-ss", pickFrameTimestamp(meta),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "2",
		tmpFramePath,
	)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: 请先安装 ffmpeg", ErrPosterGeneratorUnavailable)
		}
		return fmt.Errorf("生成视频 poster 失败: %w", err)
	}

	frameFile, err := os.Open(tmpFramePath)
	if err != nil {
		return fmt.Errorf("打开视频帧失败: %w", err)
	}
	defer frameFile.Close()

	if err := imgpkg.GenerateThumbnailVariants(frameFile, "image/jpeg", variants); err != nil {
		return fmt.Errorf("生成视频缩略图失败: %w", err)
	}
	return nil
}

func execPosterRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func pickFrameTimestamp(meta *VideoMeta) string {
	if meta == nil || meta.DurationMS <= 0 {
		return "00:00:01.000"
	}
	durationMS := meta.DurationMS
	startMS := durationMS / 10
	endMS := (durationMS * 9) / 10
	if endMS <= startMS {
		startMS = 0
		endMS = durationMS
	}
	pickedMS := startMS
	if endMS > startMS {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		pickedMS += rng.Int63n(endMS - startMS + 1)
	}
	if pickedMS < 0 {
		pickedMS = 0
	}
	hours := pickedMS / 3600000
	minutes := (pickedMS % 3600000) / 60000
	seconds := (pickedMS % 60000) / 1000
	millis := pickedMS % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

func generatePosterWithQuickLook(videoPath, posterPath string, maxEdge int) error {
	return generatePosterVariantsWithQuickLook(videoPath, []imgpkg.ThumbnailVariant{{
		DestPath: posterPath,
		MaxEdge:  maxEdge,
	}}, maxEdge)
}

func generatePosterVariantsWithQuickLook(videoPath string, variants []imgpkg.ThumbnailVariant, maxEdge int) error {
	return generatePosterVariantsWithQuickLookContext(context.Background(), videoPath, variants, maxEdge)
}

func generatePosterVariantsWithQuickLookContext(parent context.Context, videoPath string, variants []imgpkg.ThumbnailVariant, maxEdge int) error {
	if err := parent.Err(); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "echogallery-quicklook-*")
	if err != nil {
		return fmt.Errorf("创建 Quick Look 临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if maxEdge <= 0 {
		maxEdge = imgpkg.DefaultThumbnailLongEdge
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "qlmanage", "-t", "-s", strconv.Itoa(maxEdge), "-o", tmpDir, videoPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Quick Look 生成缩略图失败: %w: %s", err, strings.TrimSpace(string(out)))
	}

	matches, err := filepath.Glob(filepath.Join(tmpDir, "*"))
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("Quick Look 未输出缩略图")
	}

	srcPath := matches[0]
	mimeType := "image/jpeg"
	if strings.EqualFold(filepath.Ext(srcPath), ".png") {
		mimeType = "image/png"
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("打开 Quick Look 缩略图失败: %w", err)
	}
	defer f.Close()

	if err := imgpkg.GenerateThumbnailVariants(f, mimeType, variants); err != nil {
		return fmt.Errorf("规范化 Quick Look 缩略图失败: %w", err)
	}
	return nil
}
