package config

import "os"

const (
	LibraryStatusReady   = "ready"
	LibraryStatusMissing = "missing"
	LibraryStatusLocked  = "locked"
)

func normalizeLibraryStatus(status string) string {
	switch status {
	case LibraryStatusMissing:
		return LibraryStatusMissing
	case LibraryStatusLocked:
		return LibraryStatusLocked
	default:
		return LibraryStatusReady
	}
}

func DetectLibraryStatus(path string) string {
	if path == "" {
		return LibraryStatusMissing
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return LibraryStatusMissing
	}
	return LibraryStatusReady
}

func (c *Config) RefreshLibraryStatuses() bool {
	if c == nil {
		return false
	}
	changed := false
	for index := range c.Libraries {
		next := DetectLibraryStatus(c.Libraries[index].Path)
		if c.Libraries[index].Status != next {
			c.Libraries[index].Status = next
			changed = true
		}
	}
	return changed
}
