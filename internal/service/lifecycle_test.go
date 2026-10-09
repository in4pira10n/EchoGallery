package service

import (
	"testing"
	"time"
)

func TestStopBackgroundWaitsForWriterAndRejectsNewWork(t *testing.T) {
	s := &PhotoService{workerStop: make(chan struct{})}
	started := make(chan struct{})
	finish := make(chan struct{})
	s.background(func() { close(started); <-finish })
	<-started
	stopped := make(chan struct{})
	go func() { s.StopBackground(); close(stopped) }()
	<-s.workerStop
	select {
	case <-stopped:
		t.Fatal("shutdown did not wait for writer")
	default:
	}
	if s.background(func() { t.Error("work started after shutdown") }) {
		t.Fatal("accepted new work after shutdown")
	}
	close(finish)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	s.StopBackground()
}
