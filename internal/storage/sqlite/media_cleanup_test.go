package sqlite

import (
	"context"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestHardDeleteMissingPhotoCleansReferencesAtomically(t *testing.T) {
	db := newTestDB(t)
	p := makePhoto(1, time.Now())
	p.SourceRelPath = "missing.mp4"
	p.SourceModUnix = 123
	if err := db.SavePhoto(p); err != nil {
		t.Fatal(err)
	}
	a := &storage.Album{Name: "Personal album", CreatedBy: 1, CreatedAt: time.Now(), CoverPhotoID: &p.ID}
	if err := db.CreateAlbum(a); err != nil {
		t.Fatal(err)
	}
	if err := db.AddPhotoToAlbum(a.ID, p.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideoPlaybackPreference(p.ID, 1, 1, false, 10, []storage.VideoPlaybackBookmark{{Slot: 1, Time: 10}}); err != nil {
		t.Fatal(err)
	}
	link := &storage.ShareLink{Token: "cleanup", Type: storage.ShareTypePhoto, TargetID: p.ID, CreatedBy: 1, CreatedAt: time.Now()}
	if err := db.CreateShareLink(link); err != nil {
		t.Fatal(err)
	}
	stale := *p
	stale.SourceModUnix++
	if err := db.HardDeleteMissingPhoto(context.Background(), &stale); err == nil {
		t.Fatal("stale audit must not delete a record")
	}
	if got, err := db.GetShareLinkByToken(link.Token); err != nil || got == nil {
		t.Fatalf("failed transaction removed share: %v", err)
	}
	if err := db.HardDeleteMissingPhoto(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"photos", "album_photos", "video_playback_preferences", "share_links"} {
		var count int
		if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphan rows in %s: %d %v", table, count, err)
		}
	}
	if album, err := db.GetAlbumByID(a.ID, 1); err != nil || album == nil || album.CoverPhotoID != nil {
		t.Fatalf("album must survive with cleared cover: %+v %v", album, err)
	}
}
