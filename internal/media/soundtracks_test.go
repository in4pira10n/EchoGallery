package media

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"testing"
)

func TestSoundtrackProbeUsesAudioOrdinalsAndDefaultDisposition(t *testing.T) {
	tracks, err := probeAudioTracksWithRunner(context.Background(), "clip.mp4", func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "ffprobe" || !slices.Contains(args, "a") {
			t.Fatalf("unexpected probe: %s %v", name, args)
		}
		return []byte(`{"streams":[{"index":1,"codec_name":"aac","tags":{"language":"zh","title":"Main"}},{"index":4,"codec_name":"ac3","tags":{"language":"en"},"disposition":{"default":1}}]}`), nil
	})
	if err != nil || len(tracks) != 2 || tracks[0].Index != 0 || tracks[1].Index != 1 || tracks[1].ID != "4" || !tracks[1].Default {
		t.Fatalf("tracks=%+v err=%v", tracks, err)
	}
}

func TestSoundtrackBuilderMapsExactlyOneTrackAndEncodesAAC(t *testing.T) {
	err := buildSoundtrackMP4WithRunner(context.Background(), "source.mp4", "cache.mp4", 1, false, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		for _, pair := range [][2]string{{"-map", "0:a:1"}, {"-c:v", "copy"}, {"-c:a", "aac"}, {"-ac", "2"}} {
			found := false
			for i := 0; i+1 < len(args); i++ {
				if args[i] == pair[0] && args[i+1] == pair[1] {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %v in %v", pair, args)
			}
		}
		if slices.Contains(args, "0:a:0?") {
			t.Fatal("selected the default track too")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSoundtrackToolsAndCancellationHaveDistinctErrors(t *testing.T) {
	missing := func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }
	if _, err := probeAudioTracksWithRunner(context.Background(), "x", missing); !errors.Is(err, ErrProbeUnavailable) {
		t.Fatal(err)
	}
	if err := buildSoundtrackMP4WithRunner(context.Background(), "x", "y", 0, false, missing); !errors.Is(err, ErrPlaybackConverterUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := buildSoundtrackMP4WithRunner(ctx, "x", "y", 0, false, func(ctx context.Context, _ string, _ ...string) ([]byte, error) { calls++; return nil, ctx.Err() })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancelled build retried: %v calls=%d", err, calls)
	}
}
