package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func TestRandomMediaRefillsSparsePagesWithoutLosingCursor(t *testing.T) {
	svc, repo := cleanupService(t)
	for i := 1; i <= 70; i++ {
		p := saveCleanupPhoto(t, svc, fmt.Sprintf("photo-%03d.mp4", i))
		// Fix the random order so missing files fill the first raw page.
		p.RandomSortKey = int64(i)
		// SavePhoto preserves this key; recreate with a distinct UUID and remove the original row.
		if err := repo.HardDeleteMissingPhoto(t.Context(), p); err != nil {
			t.Fatal(err)
		}
		if err := repo.SavePhoto(p); err != nil {
			t.Fatal(err)
		}
		if i <= 27 {
			if err := os.Remove(svc.MediaPath(p)); err != nil {
				t.Fatal(err)
			}
		}
	}
	cursor := ""
	seen := map[int64]bool{}
	for request := 0; ; request++ {
		if request > 10 {
			t.Fatal("pagination failed to finish")
		}
		page, err := svc.GetRandomMedia(storage.RandomPhotosParams{UserID: 1, Seed: 1, Cursor: cursor, Limit: 28, SkipTotal: cursor != ""})
		if err != nil {
			t.Fatal(err)
		}
		if request == 0 && (len(page.Photos) != 28 || page.Total != 70) {
			t.Fatalf("short first page or corrupted total: %+v", page)
		}
		for _, p := range page.Photos {
			if seen[p.ID] {
				t.Fatalf("duplicate ID %d", p.ID)
			}
			seen[p.ID] = true
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = page.NextCursor
	}
	if len(seen) != 43 {
		t.Fatalf("got %d of 43 available media", len(seen))
	}
}

func TestMediaBrowsingRetainsCardsWhenFilesystemIsUncertain(t *testing.T) {
	for _, mode := range []string{"offline_root", "invalid_parent", "io_slots_busy"} {
		t.Run(mode, func(t *testing.T) {
			svc, _ := cleanupService(t)
			p := saveCleanupPhoto(t, svc, "nested/photo.mp4")
			switch mode {
			case "offline_root":
				if err := os.Rename(svc.sourcePath, svc.sourcePath+"-offline"); err != nil {
					t.Fatal(err)
				}
			case "invalid_parent":
				if err := os.RemoveAll(filepath.Join(svc.sourcePath, "nested")); err != nil {
					t.Fatal(err)
				}
				putCleanupFile(t, filepath.Join(svc.sourcePath, "nested"))
			case "io_slots_busy":
				for i := 0; i < cap(scanIOSlots); i++ {
					scanIOSlots <- struct{}{}
				}
				defer func() {
					for i := 0; i < cap(scanIOSlots); i++ {
						<-scanIOSlots
					}
				}()
			}
			start := time.Now()
			page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 30})
			if err != nil || len(page.Photos) != 1 || page.Photos[0].ID != p.ID {
				t.Fatalf("uncertain media hidden: %+v %v", page, err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("foreground browsing waited for stalled filesystem")
			}
		})
	}
}
