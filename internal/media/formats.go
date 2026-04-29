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
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
	".wmv":  "video/x-ms-wmv",
	".wma":  "audio/x-ms-wma",
	".ts":   "video/mp2t",
	".mts":  "video/mp2t",
	".m2ts": "video/mp2t",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
	".3gp":  "video/3gpp",
	".3g2":  "video/3gpp2",
	".ogv":  "video/ogg",
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
