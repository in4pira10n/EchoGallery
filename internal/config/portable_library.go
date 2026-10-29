package config

import (
	"net/url"
	"path/filepath"
	"strings"
)

const libraryDataDirName = ".echogallery"

// LibraryDataRoot returns the private data directory owned by one library.
// It deliberately does not create the directory.
func LibraryDataRoot(storagePath string) string {
	return filepath.Join(NormalizeStoragePath(storagePath), libraryDataDirName)
}

func LibraryLogoPath(storagePath string) string {
	return filepath.Join(LibraryDataRoot(storagePath), "logo.png")
}

func portableDatabasePath(_ string, root string) (string, error) {
	return filepath.Join(LibraryDataRoot(root), "metadata.sqlite"), nil
}

// sqliteReadOnlyURI opens portable metadata without modifying the library.
func sqliteReadOnlyURI(databasePath string) string {
	path := filepath.ToSlash(databasePath)
	if len(path) >= 2 && path[1] == ':' {
		path = strings.ReplaceAll(path, `\`, "/")
	}
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
}
