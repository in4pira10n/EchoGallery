package sqlite

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestVideoAudioTrackPreferenceSurvivesOtherUpdatesAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	photo := makePhoto(1, time.Now())
	photo.MediaKind = storage.MediaKindVideo
	if err := db.SavePhoto(photo); err != nil {
		t.Fatal(err)
	}
	selected := &storage.VideoAudioTrackPreference{Index: 0, ID: "first", Language: "zh", Label: "中文"}
	if _, err := db.UpsertVideoPlaybackPreference(photo.ID, 1, 0.6, false, 42, nil, selected); err != nil {
		t.Fatal(err)
	}
	// A later volume/bookmark update, including an explicit nil selection from the API,
	// must not reset the saved audio track.
	for _, args := range [][]*storage.VideoAudioTrackPreference{nil, {nil}} {
		pref, err := db.UpsertVideoPlaybackPreference(photo.ID, 1, 0.8, true, 50, []storage.VideoPlaybackBookmark{{Slot: 1, Time: 12}}, args...)
		if err != nil || !reflect.DeepEqual(pref.AudioTrack, selected) {
			t.Fatalf("selection lost after preference update: pref=%+v err=%v", pref, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	pref, err := db.GetVideoPlaybackPreference(photo.ID, 99)
	if err != nil || pref == nil || !reflect.DeepEqual(pref.AudioTrack, selected) {
		t.Fatalf("selection lost after reopen: pref=%+v err=%v", pref, err)
	}
	if pref.ResumeTime != 50 || !pref.Muted || len(pref.Bookmarks) != 1 {
		t.Fatalf("other playback preferences changed: %+v", pref)
	}
	second := &storage.VideoAudioTrackPreference{Index: 1, ID: "second"}
	pref, err = db.UpsertVideoPlaybackPreference(photo.ID, 1, pref.Volume, pref.Muted, pref.ResumeTime, pref.Bookmarks, second)
	if err != nil || pref == nil || !reflect.DeepEqual(pref.AudioTrack, second) {
		t.Fatalf("selection not replaced: pref=%+v err=%v", pref, err)
	}
}
