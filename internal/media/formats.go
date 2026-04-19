package media

import (
	"path/filepath"
	"strings"
)

var supportedVideoMimeTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".mov":  "video/quicktime",
	".webm": "video/webm",
}

func DetectVideoMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mimeType, ok := supportedVideoMimeTypes[ext]; ok {
		return mimeType
	}
	return ""
}

func IsSupportedVideoFilename(filename string) bool {
	return DetectVideoMimeType(filename) != ""
}
