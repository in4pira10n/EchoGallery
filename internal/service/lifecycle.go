package service

// RunBackground includes startup scans in the same shutdown barrier.
func (s *PhotoService) RunBackground(work func()) bool { return s.background(work) }

// background registers work before launching it so shutdown cannot race Add/Wait.
func (s *PhotoService) background(work func()) bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closing {
		return false
	}
	s.workerWG.Add(1)
	go func() { defer s.workerWG.Done(); work() }()
	return true
}

// StopBackground cancels queued work and waits for in-flight writes to finish.
func (s *PhotoService) StopBackground() {
	s.lifecycleMu.Lock()
	if !s.closing {
		s.closing = true
		if s.workerStop != nil {
			close(s.workerStop)
		}
	}
	s.lifecycleMu.Unlock()
	s.thumbBuildMu.Lock()
	if s.thumbBuild != nil {
		s.thumbBuild.cancel()
	}
	if s.videoThumbRefresh != nil {
		s.videoThumbRefresh.cancel()
	}
	s.thumbBuildMu.Unlock()
	s.exifBackfillMu.Lock()
	if s.exifBackfill != nil {
		s.exifBackfill.cancel()
	}
	s.exifBackfillMu.Unlock()
	s.CancelPlaybackCacheBuild(0)
	s.workerWG.Wait()
	s.scanMu.Lock()
	s.scanMu.Unlock()
}
