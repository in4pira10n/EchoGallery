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
	return filepath.Join(storagePath, uuid+".jpg")
}

func GeneratePoster(videoPath, posterPath string, maxEdge int) error {
	meta, _ := ProbeVideo(videoPath)
	err := generatePosterWithRunner(videoPath, posterPath, meta, execPosterRunner, maxEdge)
	if err == nil {
		return nil
	}
	if runtime.GOOS == "darwin" && errors.Is(err, ErrPosterGeneratorUnavailable) {
		if qlErr := generatePosterWithQuickLook(videoPath, posterPath, maxEdge); qlErr == nil {
			return nil
		}
	}
	return err
}

func generatePosterWithRunner(videoPath, posterPath string, meta *VideoMeta, runner commandRunner, maxEdge int) error {
	if err := os.MkdirAll(filepath.Dir(posterPath), 0755); err != nil {
		return fmt.Errorf("创建 poster 目录失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tmpFrame, err := os.CreateTemp(filepath.Dir(posterPath), "video-frame-*.jpg")
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

	if err := imgpkg.GenerateThumbnail(frameFile, "image/jpeg", posterPath, maxEdge); err != nil {
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
	tmpDir, err := os.MkdirTemp("", "echogallery-quicklook-*")
	if err != nil {
		return fmt.Errorf("创建 Quick Look 临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if maxEdge <= 0 {
		maxEdge = imgpkg.DefaultThumbnailLongEdge
	}
	out, err := exec.Command("qlmanage", "-t", "-s", strconv.Itoa(maxEdge), "-o", tmpDir, videoPath).CombinedOutput()
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

	if err := imgpkg.GenerateThumbnail(f, mimeType, posterPath, maxEdge); err != nil {
		return fmt.Errorf("规范化 Quick Look 缩略图失败: %w", err)
	}
	return nil
}
