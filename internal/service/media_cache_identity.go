package service

import (
	"os"
	"path/filepath"
	"strings"
)

// New caches carry their media identity so every generation can be cleaned after
// a source disappears. Existing hash-only caches remain readable without rebuilding.
func identifiedMediaCachePath(root, mediaUUID, name string) string {
	legacy := filepath.Join(root, name)
	if !filepath.IsLocal(mediaUUID) || filepath.Base(mediaUUID) != mediaUUID || strings.ContainsAny(mediaUUID, `/\`) {
		return legacy
	}
	path := filepath.Join(root, mediaUUID+"-"+name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return path
}
