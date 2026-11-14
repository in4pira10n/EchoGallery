package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"echogallery/internal/storage"
)

// Simulates a whole unreadable SQL page followed by a corrupt media file and a
// normal JPEG. Neither database conversion nor image decoding may stop the job.
type unreadableMediaPageRepo struct {
	*mockRepo
	good, bad *storage.Photo
	calls     []string
}

func (r *unreadableMediaPageRepo) ListPhotos(p storage.ListPhotosParams) (*storage.PhotoPage, error) {
	r.calls = append(r.calls, p.Cursor)
	switch p.Cursor {
	case "":
		return &storage.PhotoPage{Skipped: 320, LastID: 320, HasMore: true, NextCursor: "after-bad-page", Total: 322}, nil
	case "after-bad-page":
		return &storage.PhotoPage{Photos: []*storage.Photo{r.bad, r.good}, LastID: 322}, nil
	default:
		return nil, fmt.Errorf("unexpected cursor: %s", p.Cursor)
	}
}

func (r *unreadableMediaPageRepo) ListPhotosForMetadataBackfillContext(ctx context.Context, userID, afterID int64, limit int, includeTotal bool) (*storage.PhotoPage, error) {
	switch afterID {
	case 0:
		return &storage.PhotoPage{Skipped: 200, LastID: 200, HasMore: true, Total: 202}, nil
	case 200:
		return &storage.PhotoPage{Photos: []*storage.Photo{r.bad, r.good}, LastID: 202}, nil
	default:
		return nil, fmt.Errorf("unexpected repair boundary: %d", afterID)
	}
}

func unreadableMediaService(t *testing.T) (*PhotoService, *unreadableMediaPageRepo) {
	t.Helper()
	repo := &unreadableMediaPageRepo{mockRepo: newMockRepo(),
		good: &storage.Photo{ID: 322, UUID: "readable", OriginalName: "good.jpg", SourceRelPath: "good.jpg", MediaKind: storage.MediaKindImage},
		bad:  &storage.Photo{ID: 321, UUID: "unreadable", OriginalName: "bad.jpg", SourceRelPath: "bad.jpg", MediaKind: storage.MediaKindImage},
	}
	for _, photo := range []*storage.Photo{repo.bad, repo.good} {
		photo.UploadedBy = 1
		repo.photos[photo.ID] = photo
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "good.jpg"), createJPEGBytes(32, 24), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "bad.jpg"), []byte("not a supported media file"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewPhotoServiceWithoutWarmup(repo, source, t.TempDir(), "")
	t.Cleanup(svc.StopBackground)
	return svc, repo
}

func TestTimelineSkipsEntireUnreadablePageBeforeResponding(t *testing.T) {
	svc, repo := unreadableMediaService(t)
	page, err := svc.GetTimeline(storage.ListPhotosParams{UserID: 1, Limit: 320})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Photos) != 2 || page.Total != 322 || page.Skipped != 320 || len(repo.calls) != 2 {
		t.Fatalf("empty conversion page stranded the timeline: %+v, calls=%v", page, repo.calls)
	}
}

func TestThumbnailBuildContinuesPastUnreadableRowsAndFiles(t *testing.T) {
	svc, _ := unreadableMediaService(t)
	task := &thumbnailBuildTask{userID: 1}
	svc.runThumbnailBuild(context.Background(), task)
	status := task.snapshot()
	if status.Status != "completed" || status.Done != 322 || status.Skipped != 320 || status.Failed != 1 || status.Generated != 1 {
		t.Fatalf("bad metadata/file stopped thumbnail building: %+v", status)
	}
}

func TestEXIFRepairContinuesPastUnreadableRowsAndFiles(t *testing.T) {
	svc, _ := unreadableMediaService(t)
	var status EXIFBackfillStatus
	err := svc.backfillPhotoEXIFContext(context.Background(), 1, func(s EXIFBackfillStatus) bool { status = s; return true })
	if err != nil {
		t.Fatal(err)
	}
	if status.Done != 202 || status.Skipped < 200 || status.Updated != 1 {
		t.Fatalf("bad metadata/file stopped EXIF repair: %+v", status)
	}
}

func TestUnreadableMediaRefillRejectsStuckCursor(t *testing.T) {
	_, err := readPastInvalidMediaPages("same", func(c string) (*storage.PhotoPage, error) {
		return &storage.PhotoPage{Skipped: 2, HasMore: true, NextCursor: "same"}, nil
	})
	if err == nil {
		t.Fatal("bad backend cursor caused an infinite retry")
	}
}
