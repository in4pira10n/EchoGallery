package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LibraryScanProgress reports measured work, never an estimated discovery percentage.
type LibraryScanProgress struct {
	Phase              string    `json:"phase"`
	Operation          string    `json:"operation"`
	Path               string    `json:"path"`
	Directories        int       `json:"directories"`
	PendingDirectories int       `json:"pending_directories"`
	Media              int       `json:"media"`
	OperationStartedAt time.Time `json:"operation_started_at"`
}
type scanProgressKey struct{}
type scanObserver struct {
	mu     sync.Mutex
	value  LibraryScanProgress
	notify func(LibraryScanProgress)
}

func WithLibraryScanProgress(ctx context.Context, notify func(LibraryScanProgress)) context.Context {
	return context.WithValue(ctx, scanProgressKey{}, &scanObserver{notify: notify})
}
func reportScan(ctx context.Context, phase, operation, path string, dirs, pending, media int) {
	o, _ := ctx.Value(scanProgressKey{}).(*scanObserver)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if phase != "" {
		o.value.Phase = phase
	}
	o.value.Operation = operation
	o.value.Path = path
	o.value.OperationStartedAt = time.Now()
	if dirs >= 0 {
		o.value.Directories = dirs
		o.value.PendingDirectories = pending
		o.value.Media = media
	}
	if o.notify != nil {
		o.notify(o.value)
	}
}

// OS filesystem calls have no context API. A cancelled scan releases its caller
// immediately; at most eight outstanding OS reads can remain process-wide. Their
// results never mutate scan state or SQLite after cancellation.
var scanIOSlots = make(chan struct{}, 8)

func scanIO[T any](ctx context.Context, path string, fn func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	select {
	case scanIOSlots <- struct{}{}:
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() { defer func() { <-scanIOSlots }(); v, e := fn(); done <- result{v, e} }()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case r := <-done:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if r.err != nil {
			return zero, fmt.Errorf("%s: %w", path, r.err)
		}
		return r.value, nil
	}
}
func scanStat(ctx context.Context, path string) (fs.FileInfo, error) {
	reportScan(ctx, "", "metadata", path, -1, 0, 0)
	return scanIO(ctx, path, func() (fs.FileInfo, error) { return os.Stat(path) })
}

// CheckLibraryReachable only performs a read; an abandoned filesystem call can
// never acquire a lease or change the library after its caller times out.
func CheckLibraryReachable(ctx context.Context, path string) error {
	info, err := scanStat(ctx, path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: not a directory", path)
	}
	return nil
}

type scanDirEntry struct {
	fs.DirEntry
	ctx  context.Context
	path string
	info fs.FileInfo
}

func (d *scanDirEntry) Info() (fs.FileInfo, error) {
	if d.info != nil {
		return d.info, nil
	}
	reportScan(d.ctx, "", "metadata", d.path, -1, 0, 0)
	info, err := scanIO(d.ctx, d.path, d.DirEntry.Info)
	if err == nil {
		d.info = info
	}
	return info, err
}
func readScanDir(ctx context.Context, path string) ([]fs.DirEntry, error) {
	entries, err := scanIO(ctx, path, func() ([]fs.DirEntry, error) { return os.ReadDir(path) })
	if err != nil {
		return nil, err
	}
	for i, e := range entries {
		entries[i] = &scanDirEntry{DirEntry: e, ctx: ctx, path: filepath.Join(path, e.Name())}
	}
	return entries, nil
}

// Keep filepath.WalkDir's pre-order and SkipDir semantics while making directory
// reads observable and cancellable. The SMB collector calls this only once.
func walkScanDir(ctx context.Context, root string, visit fs.WalkDirFunc) error {
	info, err := scanStat(ctx, root)
	if err != nil {
		return err
	}
	type item struct {
		path  string
		entry fs.DirEntry
	}
	pending := []item{{root, fs.FileInfoToDirEntry(info)}}
	directories, queued, media := 0, 1, 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if next.entry.IsDir() {
			queued--
		}
		if err := visit(next.path, next.entry, nil); err != nil {
			if err == fs.SkipDir && next.entry.IsDir() {
				continue
			}
			return err
		}
		if !next.entry.IsDir() {
			continue
		}
		reportScan(ctx, "", "read_directory", next.path, directories, queued+1, media)
		entries, err := readScanDir(ctx, next.path)
		if err != nil {
			return err
		}
		directories++
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.IsDir() {
				queued++
			} else if isScanMedia(entry) {
				media++
			}
			pending = append(pending, item{filepath.Join(next.path, entry.Name()), entry})
		}
		reportScan(ctx, "", "directory_complete", next.path, directories, queued, media)
	}
	return nil
}

func (s *PhotoService) IsSMBLibrary() bool {
	s.smbOnce.Do(func() { s.smbLibrary = IsSMBPath(s.sourcePath) })
	return s.smbLibrary
}
func IsSMBPath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	if strings.Contains(strings.ToLower(normalized), "/davwwwroot") || strings.HasPrefix(normalized, "//./") {
		return false
	}
	if strings.HasPrefix(normalized, "//?/") {
		normalized = strings.TrimPrefix(normalized, "//?/")
		if strings.HasPrefix(strings.ToUpper(normalized), "UNC/") {
			return true
		}
	}
	if strings.HasPrefix(normalized, "//") {
		return true
	}
	return platformSMBPath(path)
}

// Existing SMB libraries open from SQLite. Only explicit batch scans recurse;
// a library without a completed snapshot still receives its initial import.
func (s *PhotoService) NeedsAutomaticLibraryBuild(ctx context.Context) (bool, error) {
	if !s.IsSMBLibrary() {
		return s.NeedsLibraryBuild(ctx)
	}
	snapshot, err := s.repo.GetLibraryScanSnapshot()
	if err != nil {
		return false, err
	}
	return snapshot == nil || snapshot.Version != libraryScanSnapshotVersion || snapshot.CompletedAt.IsZero(), nil
}
