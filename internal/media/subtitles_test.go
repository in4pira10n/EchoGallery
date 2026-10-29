package media

import (
	"context"
	"testing"
)

func TestProbeSubtitleTracksKeepsOnlySubRipAndUsesSubtitleOrdinals(t *testing.T) {
	tracks, err := probeSubtitleTracksWithRunner(context.Background(), "clip.mp4", func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"streams":[{"codec_name":"subrip","tags":{"language":"zh","title":"中文"},"disposition":{"default":1}},{"codec_name":"ass","tags":{"language":"en"}},{"codec_name":"subrip","tags":{"language":"ja"}}]}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].Index != 0 || tracks[1].Index != 2 || !tracks[0].Default || tracks[1].Language != "ja" {
		t.Fatalf("unexpected tracks: %+v", tracks)
	}
}
