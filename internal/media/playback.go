package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var ErrPlaybackConverterUnavailable = errors.New("ffmpeg unavailable")

func RemuxToMP4ForBrowser(inputPath, outputPath string) error {
	return RemuxToMP4ForBrowserContext(context.Background(), inputPath, outputPath)
}

func RemuxToMP4ForBrowserContext(ctx context.Context, inputPath, outputPath string) error {
	return remuxToMP4ForBrowserWithRunner(ctx, inputPath, outputPath, execRunner)
}

func TranscodeToMP4ForBrowser(inputPath, outputPath string) error {
	return TranscodeToMP4ForBrowserContext(context.Background(), inputPath, outputPath)
}

func TranscodeToMP4ForBrowserContext(ctx context.Context, inputPath, outputPath string) error {
	return transcodeToMP4ForBrowserWithRunner(ctx, inputPath, outputPath, execRunner)
}

func remuxToMP4ForBrowserWithRunner(parent context.Context, inputPath, outputPath string, runner commandRunner) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()

	out, err := runner(ctx, "ffmpeg",
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-sn",
		"-dn",
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-c", "copy",
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath,
	)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: install ffmpeg first", ErrPlaybackConverterUnavailable)
		}
		return fmt.Errorf("failed to prepare browser playback file: %w%s", err, commandOutputSuffix(out))
	}
	return nil
}

func transcodeToMP4ForBrowserWithRunner(parent context.Context, inputPath, outputPath string, runner commandRunner) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Hour)
	defer cancel()

	out, err := runner(ctx, "ffmpeg",
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-sn",
		"-dn",
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "160k",
		"-movflags", "+faststart",
		"-f", "mp4",
		outputPath,
	)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: install ffmpeg first", ErrPlaybackConverterUnavailable)
		}
		return fmt.Errorf("failed to transcode browser playback file: %w%s", err, commandOutputSuffix(out))
	}
	return nil
}

func commandOutputSuffix(out []byte) string {
	message := strings.TrimSpace(string(out))
	if message == "" {
		return ""
	}
	const maxLen = 1800
	if len(message) > maxLen {
		message = message[len(message)-maxLen:]
		message = "..." + message
	}
	return ": " + message
}
