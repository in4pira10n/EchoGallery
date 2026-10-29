package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SubtitleTrack struct {
	Index    int    `json:"index"`
	Language string `json:"language,omitempty"`
	Label    string `json:"label,omitempty"`
	Default  bool   `json:"default"`
	Format   string `json:"format,omitempty"`
}

func ProbeSubtitleTracks(ctx context.Context, path string) ([]SubtitleTrack, error) {
	return probeSubtitleTracksWithRunner(ctx, path, execRunner)
}

func probeSubtitleTracksWithRunner(parent context.Context, path string, runner commandRunner) ([]SubtitleTrack, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	out, err := runner(ctx, "ffprobe", "-v", "error", "-select_streams", "s", "-show_streams", "-of", "json", path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrProbeUnavailable
		}
		return nil, fmt.Errorf("probe subtitles: %w%s", err, commandOutputSuffix(out))
	}
	var payload struct {
		Streams []struct {
			Codec       string            `json:"codec_name"`
			Tags        map[string]string `json:"tags"`
			Disposition struct {
				Default int `json:"default"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}
	tracks := make([]SubtitleTrack, 0, len(payload.Streams))
	for i, stream := range payload.Streams {
		if !strings.EqualFold(stream.Codec, "subrip") {
			continue
		}
		tracks = append(tracks, SubtitleTrack{Index: i, Language: stream.Tags["language"], Label: stream.Tags["title"], Default: stream.Disposition.Default != 0, Format: "srt"})
	}
	return tracks, nil
}

func ExtractSubtitleWebVTT(ctx context.Context, input, output string, index int) error {
	if index < 0 || index > 1023 {
		return fmt.Errorf("invalid subtitle index")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, err := execRunner(ctx, "ffmpeg", "-y", "-nostdin", "-hide_banner", "-loglevel", "error", "-i", input,
		"-map", fmt.Sprintf("0:s:%d", index), "-c:s", "webvtt", "-f", "webvtt", output)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return ErrPlaybackConverterUnavailable
		}
		return fmt.Errorf("extract subtitle: %w%s", err, commandOutputSuffix(out))
	}
	return addWebVTTCuePosition(output)
}

// addWebVTTCuePosition keeps native browser captions above the video edge.
// It only changes cue positioning; font and color remain system controlled.
func addWebVTTCuePosition(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"), "\n")
	for i, line := range lines {
		if strings.Contains(line, " --> ") && !strings.Contains(line, " line:") {
			lines[i] = line + " line:88 align:center"
		}
	}
	return os.WriteFile(path, []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n")+"\n"), 0644)
}

func ExternalSRTPath(videoPath string) string {
	ext := filepath.Ext(videoPath)
	base := strings.TrimSuffix(videoPath, ext)
	for _, candidateExt := range []string{".srt", ".SRT"} {
		candidate := base + candidateExt
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	if matched := findExternalSRTByNormalizedBase(filepath.Dir(videoPath), filepath.Base(base)); matched != "" {
		return matched
	}
	return base + ".srt"
}

func findExternalSRTByNormalizedBase(dir, videoBase string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	target := normalizeFilename(videoBase)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".srt") {
			continue
		}
		candidateBase := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if normalizeFilename(candidateBase) == target {
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}

func normalizeFilename(value string) string {
	value = strings.TrimSpace(norm.NFC.String(value))
	var b strings.Builder
	for _, r := range value {
		b.WriteRune(r)
	}
	return b.String()
}
