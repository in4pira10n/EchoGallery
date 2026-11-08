package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLibraryMetricsRefreshDoesNotBlockAndCoalesces(t *testing.T) {
	var queue libraryMetricsRefreshQueue
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var duplicates atomic.Int32
	queue.enqueue("one", func(context.Context) { close(started); <-release })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	// A duplicate is discarded; another library waits behind the HDD reader.
	queue.enqueue("one", func(context.Context) { duplicates.Add(1) })
	queue.enqueue("two", func(context.Context) { close(finished) })
	select {
	case <-finished:
		t.Fatal("statistics reads ran concurrently")
	default:
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("queued refresh did not finish")
	}
	if duplicates.Load() != 0 {
		t.Fatal("duplicate refresh ran")
	}
	// A subsequent explicit open can refresh the same library again.
	again := make(chan struct{})
	queue.enqueue("one", func(context.Context) { close(again) })
	select {
	case <-again:
	case <-time.After(time.Second):
		t.Fatal("subsequent refresh was lost")
	}
}

func TestLibraryMetricsRefreshCancellationWaitsForActiveReader(t *testing.T) {
	var queue libraryMetricsRefreshQueue
	started := make(chan struct{})
	finished := make(chan struct{})
	queue.enqueue("one", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	queue.cancel("one")
	select {
	case <-finished:
	default:
		t.Fatal("cancellation returned before the active reader stopped")
	}
}
