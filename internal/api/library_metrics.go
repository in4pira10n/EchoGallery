package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"echogallery/internal/config"
	"echogallery/internal/service"
)

type libraryMetricsFile struct {
	Libraries []libraryResponse `json:"libraries"`
}

var libraryMetricsMu sync.Mutex

func normalizeMetricsPath(path string) string {
	return config.NormalizeStoragePath(strings.TrimSpace(path))
}

func readLibraryMetrics() (map[string]libraryResponse, string, error) {
	path, err := config.LibraryMetricsPath()
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]libraryResponse{}, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	var file libraryMetricsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, path, err
	}
	metrics := make(map[string]libraryResponse, len(file.Libraries))
	for _, item := range file.Libraries {
		key := strings.TrimSpace(item.ID)
		if key == "" {
			key = normalizeMetricsPath(item.Path)
		}
		if key != "" {
			metrics[key] = item
		}
	}
	return metrics, path, nil
}

// loadLibraryMetricsForSettings keeps settings reads cache-only once the
// snapshot exists. The first run creates a complete snapshot for all configured
// libraries so settings can render without repeatedly opening every database.
func loadLibraryMetricsForSettings(cfg *config.Config) map[string]libraryResponse {
	metrics, path, err := readLibraryMetrics()
	if err != nil {
		return map[string]libraryResponse{}
	}
	if _, err := os.Stat(path); err == nil {
		return metrics
	} else if !os.IsNotExist(err) {
		return metrics
	}

	if cfg == nil || len(cfg.Libraries) == 0 {
		_ = writeLibraryMetrics(metrics)
		return metrics
	}
	for _, library := range cfg.Libraries {
		// Unavailable network libraries must not delay settings for local ones.
		// Their summary is populated on explicit open or scan completion.
		if service.IsSMBPath(library.Path) {
			continue
		}
		stats := refreshLibraryMetric(cfg, library)
		key := strings.TrimSpace(library.ID)
		if key == "" {
			key = normalizeMetricsPath(library.Path)
		}
		if key != "" && stats != (libraryResponse{}) {
			metrics[key] = stats
		}
	}
	// Keep the first-run contract even when a library is currently unavailable
	// or has no readable database yet; a later explicit open/scan can fill it.
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		_ = writeLibraryMetrics(metrics)
	}
	return metrics
}

func writeLibraryMetrics(metrics map[string]libraryResponse) error {
	path, err := config.LibraryMetricsPath()
	if err != nil {
		return err
	}
	file := libraryMetricsFile{Libraries: make([]libraryResponse, 0, len(metrics))}
	for _, item := range metrics {
		file.Libraries = append(file.Libraries, item)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".library-metrics-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeLibraryMetric(library config.Library, stats libraryResponse) error {
	libraryMetricsMu.Lock()
	defer libraryMetricsMu.Unlock()
	metrics, path, err := readLibraryMetrics()
	if err != nil {
		return err
	}
	stats.ID = library.ID
	stats.Name = library.Name
	stats.Path = library.Path
	key := strings.TrimSpace(library.ID)
	if key == "" {
		key = normalizeMetricsPath(library.Path)
	}
	metrics[key] = stats
	return writeLibraryMetricsAtPath(path, metrics)
}

func writeLibraryMetricsAtPath(path string, metrics map[string]libraryResponse) error {
	file := libraryMetricsFile{Libraries: make([]libraryResponse, 0, len(metrics))}
	for _, item := range metrics {
		file.Libraries = append(file.Libraries, item)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".library-metrics-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func cachedLibraryMetric(metrics map[string]libraryResponse, library config.Library) (libraryResponse, bool) {
	key := strings.TrimSpace(library.ID)
	if key == "" {
		key = normalizeMetricsPath(library.Path)
	}
	item, ok := metrics[key]
	return item, ok
}

// Serialize background statistics reads so rapid library switches do not start
// competing HDD scans. Duplicate requests for an already queued library share
// the same refresh. Settings continue reading the last JSON snapshot.
type libraryMetricsRefreshQueue struct {
	mu      sync.Mutex
	keys    map[string]*libraryMetricsRefreshJob
	jobs    []*libraryMetricsRefreshJob
	running bool
	closed  bool
}

type libraryMetricsRefreshJob struct {
	key     string
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
	run     func(context.Context)
}

var backgroundLibraryMetrics libraryMetricsRefreshQueue

func (q *libraryMetricsRefreshQueue) enqueue(key string, run func(context.Context)) {
	q.mu.Lock()
	if q.keys == nil {
		q.keys = make(map[string]*libraryMetricsRefreshJob)
	}
	if q.closed || q.keys[key] != nil {
		q.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &libraryMetricsRefreshJob{key: key, ctx: ctx, cancel: cancel, done: make(chan struct{}), run: run}
	q.keys[key] = job
	q.jobs = append(q.jobs, job)
	start := !q.running
	q.running = true
	q.mu.Unlock()
	if start {
		go q.drain()
	}
}

func (q *libraryMetricsRefreshQueue) drain() {
	for {
		q.mu.Lock()
		if len(q.jobs) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		job := q.jobs[0]
		q.jobs[0] = nil
		q.jobs = q.jobs[1:]
		job.started = true
		q.mu.Unlock()
		if job.ctx.Err() == nil {
			job.run(job.ctx)
		}
		job.cancel()
		q.mu.Lock()
		if q.keys[job.key] == job {
			delete(q.keys, job.key)
		}
		close(job.done)
		q.mu.Unlock()
	}
}

// Cancel before acquiring the runtime's exclusive release gate. An active
// reader closes its SQLite connection before the durable copy is replaced.
func (q *libraryMetricsRefreshQueue) cancel(key string) {
	q.mu.Lock()
	job := q.keys[key]
	if job != nil {
		job.cancel()
		if !job.started {
			for i, queued := range q.jobs {
				if queued == job {
					q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
					break
				}
			}
			delete(q.keys, key)
			close(job.done)
		}
	}
	wait := job != nil && job.started
	q.mu.Unlock()
	if wait {
		<-job.done
	}
}

func (q *libraryMetricsRefreshQueue) stop() {
	q.mu.Lock()
	q.closed = true
	var active []*libraryMetricsRefreshJob
	for _, job := range q.keys {
		job.cancel()
		if job.started {
			active = append(active, job)
		}
	}
	q.mu.Unlock()
	for _, job := range active {
		<-job.done
	}
}

// StopLibraryMetricsRefresh cancels optional statistics work during shutdown.
func StopLibraryMetricsRefresh() { backgroundLibraryMetrics.stop() }

func scheduleLibraryMetricsRefresh(cfg *config.Config, library config.Library, provider *libraryRuntimeProvider) {
	if cfg == nil || provider == nil {
		return
	}
	key := normalizeMetricsPath(library.Path)
	backgroundLibraryMetrics.enqueue(key, func(ctx context.Context) {
		// Pin the working database only while reading, never open a released
		// library again from a stale queued job.
		release, ok := provider.beginBackgroundRequest(ctx, library.ID)
		if !ok {
			return
		}
		defer release()
		if ctx.Err() != nil {
			return
		}
		provider.mu.Lock()
		repo := provider.repos[library.ID]
		provider.mu.Unlock()
		if repo == nil {
			return
		}
		path := repo.WorkingPath()
		if path == "" {
			return
		}
		stats, err := readLibraryStatsFromDBContext(ctx, path)
		if err != nil || ctx.Err() != nil {
			return
		}
		_ = writeLibraryMetric(library, stats)
	})
}
