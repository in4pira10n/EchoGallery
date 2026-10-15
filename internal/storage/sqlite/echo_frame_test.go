package sqlite

import (
	"echogallery/internal/storage"
	"testing"
	"time"
)

func TestEchoFrameRandomAlbumScope(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, name := range []string{"match-a.jpg", "match-b.jpg", "other.jpg"} {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = name
		p.OriginalName = name
		p.RandomSortKey = int64(i + 1)
		if err := db.SavePhoto(p); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if err := db.SetPhotoFavorite(p.ID, 1, true, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	album := &storage.Album{Name: "matches", CreatedBy: 1, CreatedAt: base, SourceKind: "smart", SmartMatchMode: "all", SmartRules: []storage.SmartAlbumRule{{Field: "filename", Operator: "contains", Value: "match"}}}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatal(err)
	}
	params := storage.RandomPhotosParams{UserID: 1, Seed: 1, AlbumIDs: []int64{album.ID, album.ID}, OnlyFavorite: true, Limit: 1}
	seen := map[string]bool{}
	for {
		page, err := db.ListRandomPhotos(params)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range page.Photos {
			if seen[p.UUID] || p.OriginalName == "other.jpg" {
				t.Fatalf("duplicate or nonmatching media: %s", p.UUID)
			}
			seen[p.UUID] = true
		}
		if !page.HasMore {
			break
		}
		params.Cursor, params.SkipTotal = page.NextCursor, true
	}
	if len(seen) != 2 {
		t.Fatalf("missing matching media: %v", seen)
	}
	params.UserID, params.Cursor = 2, ""
	page, err := db.ListRandomPhotos(params)
	if err != nil || len(page.Photos) != 0 {
		t.Fatalf("foreign album leaked: %v %v", page, err)
	}
	page, err = db.ListPhotos(storage.ListPhotosParams{UserID: 1, OnlyFavorite: true, Reverse: true, Limit: 5})
	if err != nil || len(page.Photos) != 2 || page.Photos[0].OriginalName != "match-a.jpg" {
		t.Fatalf("favorite chronological order failed: %v %v", page, err)
	}
}
