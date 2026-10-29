package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"echogallery/internal/config"
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
