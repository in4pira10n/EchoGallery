package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestLegacyMediaDatesPreserveTimeZone(t *testing.T) {
	for _, value := range []string{
		"2023-11-10 14:40:59 +0700 +0700",
		"2023-11-10 14:40:59 +07:00 +07:00",
		"2023-11-10 14:40:59 -0700 -0700",
		"2023-11-10 14:40:59.123456789 +0800 CST m=+309.194814101",
	} {
		t.Run(value, func(t *testing.T) {
			got, ok := parseFlexibleSQLiteTime(value)
			if !ok || got.Hour() != 14 || got.Minute() != 40 || got.Second() != 59 {
				t.Fatalf("legacy capture date was lost: %v, %v", got, ok)
			}
			_, offset := got.Zone()
			want := 7 * 3600
			if value[20] == '-' {
				want = -want
			}
			if got.Nanosecond() != 0 {
				want = 8 * 3600
			}
			if offset != want {
				t.Fatalf("zone offset = %d; want %d", offset, want)
			}
		})
	}
}

// This includes the exact date that broke IMG_4334.MOV, repeated same-second
// dates, unknown dates, and two wholly unreadable pages in the numeric-zone group.
func seedLegacyMedia(t *testing.T, db *DB) (int64, map[int64]bool) {
	t.Helper()
	album := &storage.Album{Name: "Legacy media", CreatedBy: 1, CreatedAt: time.Now()}
	if err := db.CreateAlbum(album); err != nil {
		t.Fatal(err)
	}
	valid := make(map[int64]bool)
	for i := 0; i < 12; i++ {
		photo := makePhoto(1, time.Date(2023, 11, 10, 14, 40, 59, 0, time.UTC))
		photo.UUID = fmt.Sprintf("legacy-%d", i)
		photo.OriginalName = fmt.Sprintf("IMG_4334-%d.MOV", i)
		photo.MediaKind, photo.MimeType = storage.MediaKindVideo, "video/quicktime"
		photo.RandomSortKey = int64((i + 1) * 10)
		photo.IsFavorite, photo.IsSuperFavorite = true, i%3 == 0
		photo.SourceRelPath = photo.OriginalName
		if err := db.SavePhoto(photo); err != nil {
			t.Fatal(err)
		}
		if err := db.AddPhotoToAlbum(album.ID, photo.ID, 1); err != nil {
			t.Fatal(err)
		}
		date := "2023-11-10 14:40:59 +0800 CST"
		if i >= 4 && i <= 9 {
			date = "2023-11-10 14:40:59 +0700 +0700"
		}
		if i == 10 {
			date = "unknown-date"
		}
		if i == 11 {
			date = "2023-11-10T14:40:59+07:00"
		}
		if _, err := db.db.Exec(`UPDATE photos SET taken_at = ?, uploaded_at = ? WHERE id = ?`, date,
			"2026-10-08 02:17:37.8185036 +0800 CST m=+309.194814101", photo.ID); err != nil {
			t.Fatal(err)
		}
		if i >= 6 && i <= 9 {
			if _, err := db.db.Exec(`UPDATE photos SET width = 'unknown-width' WHERE id = ?`, photo.ID); err != nil {
				t.Fatal(err)
			}
		} else {
			valid[photo.ID] = true
		}
	}
	if _, err := db.db.Exec(`UPDATE photos SET size = 'unknown-size', is_super_favorite = 'unknown-flag' WHERE id = 8`); err != nil {
		t.Fatal(err)
	}

	return album.ID, valid
}

func TestMediaReadersSkipBadRowsWithoutLosingPagination(t *testing.T) {
	db := newTestDB(t)
	albumID, valid := seedLegacyMedia(t, db)
	readers := map[string]func(string) (*storage.PhotoPage, error){
		"timeline desc": func(c string) (*storage.PhotoPage, error) {
			return db.ListPhotos(storage.ListPhotosParams{UserID: 1, Cursor: c, Limit: 2})
		},
		"timeline asc": func(c string) (*storage.PhotoPage, error) {
			return db.ListPhotos(storage.ListPhotosParams{UserID: 1, Cursor: c, Limit: 2, Reverse: true})
		},
		"search": func(c string) (*storage.PhotoPage, error) {
			return db.SearchPhotos(storage.SearchPhotosParams{UserID: 1, Query: "4334", Cursor: c, Limit: 2})
		},
		"favorites": func(c string) (*storage.PhotoPage, error) {
			return db.ListFavoritePhotos(storage.ListPhotosParams{UserID: 1, Cursor: c, Limit: 2})
		},
		"random wrap": func(c string) (*storage.PhotoPage, error) {
			return db.ListRandomPhotos(storage.RandomPhotosParams{UserID: 1, Seed: 65, Cursor: c, Limit: 2})
		},
	}
	for _, sort := range []string{"timeline_asc", "timeline_desc", "name", "size"} {
		readers["album "+sort] = func(c string) (*storage.PhotoPage, error) {
			return db.ListAlbumPhotos(storage.ListAlbumPhotosParams{UserID: 1, AlbumID: albumID, Sort: sort, Cursor: c, Limit: 2})
		}
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			seen := map[int64]bool{}
			cursor := ""
			for attempt := 0; ; attempt++ {
				if attempt > 15 {
					t.Fatal("pagination never finished")
				}
				page, err := read(cursor)
				if err != nil {
					t.Fatal(err)
				}
				for _, photo := range page.Photos {
					if !valid[photo.ID] || seen[photo.ID] {
						t.Fatalf("unexpected or duplicate media: %d", photo.ID)
					}
					seen[photo.ID] = true
					if photo.ID == 5 || photo.ID == 6 {
						_, offset := photo.TakenAt.Zone()
						if photo.TakenAt.Year() != 2023 || offset != 7*3600 {
							t.Fatalf("capture date fell back instead of parsing: %v", photo.TakenAt)
						}
					}
					if photo.TakenAt.IsZero() || photo.UploadedAt.IsZero() {
						t.Fatal("date fallback missing")
					}
				}
				if !page.HasMore {
					break
				}
				if page.NextCursor == "" || page.NextCursor == cursor {
					t.Fatal("cursor did not advance past unreadable rows")
				}
				cursor = page.NextCursor
			}
			if len(seen) != len(valid) {
				t.Fatalf("only found %d of %d readable media: %v", len(seen), len(valid), seen)
			}
		})
	}
	// Neighborhood reads must also survive corrupt records, including a side
	// whose two nearest rows are both unreadable.
	for _, reverse := range []bool{false, true} {
		for id := range valid {
			window, err := db.LocateTimelineWindow(storage.LocateTimelineParams{UserID: 1, PhotoID: id, Limit: 2, Reverse: reverse})
			if err != nil || window == nil || window.Photos[window.TargetIndex].ID != id {
				t.Fatalf("timeline locate %d: %v, %v", id, window, err)
			}
			for _, sort := range []string{"timeline_asc", "timeline_desc", "name", "size"} {
				window, err := db.LocateAlbumWindow(storage.LocateAlbumParams{UserID: 1, AlbumID: albumID, PhotoID: id, Limit: 2, Sort: sort})
				if err != nil || window == nil || window.Photos[window.TargetIndex].ID != id {
					t.Fatalf("album locate %d (%s): %v, %v", id, sort, window, err)
				}
			}
		}
	}
}

func TestMetadataRepairAdvancesPastEntireBadPage(t *testing.T) {
	db := newTestDB(t)
	_, valid := seedLegacyMedia(t, db)
	seen, afterID, skipped := 0, int64(0), 0
	for attempt := 0; ; attempt++ {
		if attempt > 15 {
			t.Fatal("ID paging never finished")
		}
		page, err := db.ListPhotosForMetadataBackfillContext(context.Background(), 1, afterID, 2, true)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(page.Photos)
		skipped += page.Skipped
		if !page.HasMore {
			break
		}
		if page.LastID <= afterID {
			t.Fatal("ID boundary did not advance")
		}
		afterID = page.LastID
	}
	if seen != len(valid) || skipped != 4 {
		t.Fatalf("read %d, skipped %d; want %d, 4", seen, skipped, len(valid))
	}
}

func TestUnknownDeletionDateStaysDeleted(t *testing.T) {
	db := newTestDB(t)
	_, valid := seedLegacyMedia(t, db)
	if _, err := db.db.Exec(`UPDATE photos SET deleted_at = 'unknown-deletion-date'`); err != nil {
		t.Fatal(err)
	}
	index, err := db.ListSourceMediaIndex(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range index {
		if !item.Deleted {
			t.Fatal("unknown deletion date restored a media record")
		}
	}
	cursor, seen := "", 0
	for attempt := 0; ; attempt++ {
		if attempt > 15 {
			t.Fatal("trash paging never finished")
		}
		page, err := db.ListTrashedPhotos(storage.ListPhotosParams{UserID: 1, Cursor: cursor, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, photo := range page.Photos {
			if photo.DeletedAt == nil {
				t.Fatal("deletion flag lost")
			}
		}
		seen += len(page.Photos)
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if seen != len(valid) {
		t.Fatalf("trash lost media: %d of %d", seen, len(valid))
	}
}

func TestMediaReaderDoesNotHideDatabaseFailure(t *testing.T) {
	db := newTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ListPhotos(storage.ListPhotosParams{UserID: 1}); err == nil {
		t.Fatal("closed database was reported as an empty library")
	}
	if _, err := db.SearchPhotos(storage.SearchPhotosParams{UserID: 1, Query: "4334"}); err == nil {
		t.Fatal("closed database was reported as empty search results")
	}
}

func TestBackwardMediaBrowsingCrossesEntireUnreadableWindow(t *testing.T) {
	db := newTestDB(t)
	albumID, _ := seedLegacyMedia(t, db)
	type reader struct {
		list   func() (*storage.PhotoPage, error)
		before func(int64) (*storage.PhotoPage, error)
	}
	readers := map[string]reader{}
	for _, reverse := range []bool{false, true} {
		readers[fmt.Sprintf("timeline reverse=%v", reverse)] = reader{
			list: func() (*storage.PhotoPage, error) {
				return db.ListPhotos(storage.ListPhotosParams{UserID: 1, Limit: 30, Reverse: reverse})
			},
			before: func(id int64) (*storage.PhotoPage, error) {
				return db.ListPhotosBefore(storage.LocateTimelineParams{UserID: 1, PhotoID: id, Limit: 2, Reverse: reverse})
			},
		}
	}
	for _, sort := range []string{"timeline_asc", "timeline_desc", "size", "name"} {
		readers["album "+sort] = reader{
			list: func() (*storage.PhotoPage, error) {
				return db.ListAlbumPhotos(storage.ListAlbumPhotosParams{UserID: 1, AlbumID: albumID, Limit: 30, Sort: sort})
			},
			before: func(id int64) (*storage.PhotoPage, error) {
				return db.ListAlbumPhotosBefore(storage.LocateAlbumParams{UserID: 1, AlbumID: albumID, PhotoID: id, Limit: 2, Sort: sort})
			},
		}
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			all, err := read.list()
			if err != nil {
				t.Fatal(err)
			}
			anchor := all.Photos[len(all.Photos)-1].ID
			seen := map[int64]bool{anchor: true}
			for attempt := 0; ; attempt++ {
				if attempt > 15 {
					t.Fatal("backwards browsing loop")
				}
				page, err := read.before(anchor)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range page.Photos {
					if seen[p.ID] {
						t.Fatalf("duplicate backward media %d", p.ID)
					}
					seen[p.ID] = true
				}
				if !page.HasMore {
					break
				}
				if len(page.Photos) == 0 {
					t.Fatal("whole skipped window stranded backwards browsing")
				}
				anchor = page.Photos[0].ID
			}
			if len(seen) != len(all.Photos) {
				t.Fatalf("backwards browsing lost media: %d of %d", len(seen), len(all.Photos))
			}
		})
	}
}
