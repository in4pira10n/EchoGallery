package config

import (
	"encoding/json"
	"testing"
)

func TestDanmakuPreferencesRoundTrip(t *testing.T) {
	p := NormalizeDanmaku(nil)
	if p.FontStep != 1 || p.FontWeightStep != 2 || p.OpacityStep != 2 || p.SpeedStep != 2 {
		t.Fatalf("unexpected defaults: %+v", p)
	}
	original := Preferences{Danmaku: &DanmakuPreferences{FontStep: 5, FontWeightStep: 4, OpacityStep: 0, SpeedStep: 4, BlockList: "spoiler\n/test/i"}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored Preferences
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	got := NormalizeDanmaku(restored.Danmaku)
	if *got != *original.Danmaku {
		t.Fatalf("preferences lost: %+v", got)
	}
	invalid := &DanmakuPreferences{FontStep: 6, FontWeightStep: 5, OpacityStep: -1, SpeedStep: 5}
	got = NormalizeDanmaku(invalid)
	if got.FontStep != 1 || got.FontWeightStep != 2 || got.OpacityStep != 2 || got.SpeedStep != 2 || invalid.FontStep != 6 {
		t.Fatalf("invalid normalization: %+v", got)
	}
}

func TestDanmakuPreferencesLegacyDefaultFontWeight(t *testing.T) {
	var prefs DanmakuPreferences
	if err := json.Unmarshal([]byte(`{"font_step":1,"opacity_step":2,"speed_step":2}`), &prefs); err != nil {
		t.Fatal(err)
	}
	if got := NormalizeDanmaku(&prefs); got.FontWeightStep != 2 {
		t.Fatalf("legacy preferences should keep the default 600 weight: %+v", got)
	}
}

func TestDanmakuPreferencesDefaultInConfigAndProfile(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	if got := cfg.Preferences.Danmaku; got == nil || got.FontStep != 1 || got.FontWeightStep != 2 || got.OpacityStep != 2 || got.SpeedStep != 2 {
		t.Fatalf("config danmaku defaults: %+v", got)
	}

	profile := &Profile{}
	profile.applyDefaults(nil)
	if got := profile.Preferences.Danmaku; got == nil || got.FontStep != 1 || got.FontWeightStep != 2 || got.OpacityStep != 2 || got.SpeedStep != 2 {
		t.Fatalf("profile danmaku defaults: %+v", got)
	}
}
