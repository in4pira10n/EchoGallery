package config

import (
	"encoding/json"
	"testing"
)

func TestEchoFrameProfilePreferences(t *testing.T) {
	p := NormalizeEchoFrame(nil)
	if p.Order != "random" || p.Interval != 30000 || p.TextOpacity != 60 || !p.Muted || !p.Clock {
		t.Fatal("missing playback defaults")
	}
	p.Clock = false
	p.Interval = 45000
	p.IntervalCustom = true
	p.TextOpacity = 73
	p.WakeLock = false
	p.AlbumIDs = []int64{1, 1, -1, 2}
	data, err := json.Marshal(Profile{Preferences: Preferences{EchoFrame: p}})
	if err != nil {
		t.Fatal(err)
	}
	var restored Profile
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	got := NormalizeEchoFrame(restored.Preferences.EchoFrame)
	if got.Clock || got.WakeLock || len(got.AlbumIDs) != 2 || got.Interval != 45000 || got.TextOpacity != 73 || !got.IntervalCustom {
		t.Fatalf("preferences lost: %+v", got)
	}
	if len(p.AlbumIDs) != 4 {
		t.Fatal("normalization mutated original profile")
	}
	p.Interval = 3000
	if NormalizeEchoFrame(p).Interval != 3000 {
		t.Fatal("three-second display interval was not preserved")
	}
}
