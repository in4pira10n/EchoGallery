package service

import (
	"context"
	"testing"
	"time"

	"echogallery/internal/storage"
)

type waitingEXIFRepo struct {
	*mockRepo
	entered chan struct{}
}

func (r *waitingEXIFRepo) ListPhotosForMetadataBackfillContext(ctx context.Context, userID, afterID int64, limit int, includeTotal bool) (*storage.PhotoPage, error) {
	close(r.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestEXIFBackfillCancelWhileReadingMediaList(t *testing.T) {
	repo := &waitingEXIFRepo{mockRepo: newMockRepo(), entered: make(chan struct{})}
	svc := NewPhotoServiceWithoutWarmup(repo, t.TempDir(), t.TempDir(), "")
	defer svc.StopBackground()
	if _, err := svc.StartEXIFBackfill(1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.entered:
	case <-time.After(time.Second):
		t.Fatal("metadata task did not start")
	}
	if _, err := svc.CancelEXIFBackfill(1); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if status := svc.GetEXIFBackfillStatus(1); status.Status == "cancelled" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("task stayed active after cancellation: %+v", svc.GetEXIFBackfillStatus(1))
}

func TestEXIFBackfillRejectsStartAfterShutdown(t *testing.T) {
	svc := NewPhotoServiceWithoutWarmup(newMockRepo(), t.TempDir(), t.TempDir(), "")
	svc.StopBackground()
	status, err := svc.StartEXIFBackfill(1)
	if err == nil || status.Status != "failed" {
		t.Fatalf("shutdown service left an unstarted task running: status=%+v err=%v", status, err)
	}
}
