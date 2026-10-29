package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Index is the audio-stream ordinal (0:a:N), not the container stream index.
type AudioTrack struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Language string `json:"language,omitempty"`
	Label    string `json:"label,omitempty"`
	Codec    string `json:"codec,omitempty"`
	Default  bool   `json:"default"`
}

func ProbeAudioTracks(ctx context.Context, path string) ([]AudioTrack, error) {
	return probeAudioTracksWithRunner(ctx, path, execRunner)
}

func probeAudioTracksWithRunner(parent context.Context, path string, runner commandRunner) ([]AudioTrack, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	out, err := runner(ctx, "ffprobe", "-v", "error", "-select_streams", "a", "-show_streams", "-of", "json", path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrProbeUnavailable
		}
		return nil, fmt.Errorf("probe soundtracks: %w%s", err, commandOutputSuffix(out))
	}
	var result struct {
		Streams []struct {
			Index       int               `json:"index"`
			Codec       string            `json:"codec_name"`
			Tags        map[string]string `json:"tags"`
			Disposition struct {
				Default int `json:"default"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("decode soundtracks: %w", err)
	}
	tracks := make([]AudioTrack, 0, len(result.Streams))
	for i, stream := range result.Streams {
		tracks = append(tracks, AudioTrack{Index: i, ID: strconv.Itoa(stream.Index), Codec: stream.Codec,
			Language: stream.Tags["language"], Label: stream.Tags["title"], Default: stream.Disposition.Default != 0})
	}
	return tracks, nil
}

// BuildSoundtrackMP4 keeps one selected audio stream, encoded as browser-safe
// AAC stereo. It copies video whenever possible; the source is never modified.
func BuildSoundtrackMP4(ctx context.Context, input, output string, index int, transcodeVideo bool) error {
	return buildSoundtrackMP4WithRunner(ctx, input, output, index, transcodeVideo, execRunner)
}

func buildSoundtrackMP4WithRunner(parent context.Context, input, output string, index int, transcodeVideo bool, runner commandRunner) error {
	if index < 0 || index > 1023 {
		return fmt.Errorf("invalid soundtrack index")
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Hour)
	defer cancel()
	run := func(transcode bool) error {
		args := []string{"-y", "-nostdin", "-hide_banner", "-loglevel", "error", "-i", input,
			"-map", "0:v:0?", "-map", fmt.Sprintf("0:a:%d", index), "-sn", "-dn",
			"-map_metadata", "-1", "-map_chapters", "-1"}
		if transcode {
			args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p")
		} else {
			args = append(args, "-c:v", "copy")
		}
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k", "-disposition:a:0", "default", "-movflags", "+faststart", "-f", "mp4", output)
		out, err := runner(ctx, "ffmpeg", args...)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return ErrPlaybackConverterUnavailable
		}
		return fmt.Errorf("build soundtrack: %w%s", err, commandOutputSuffix(out))
	}
	err := run(transcodeVideo)
	if err != nil && !transcodeVideo && ctx.Err() == nil && !errors.Is(err, ErrPlaybackConverterUnavailable) {
		err = run(true)
	}
	return err
}
