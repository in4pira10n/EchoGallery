package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type rootDebugMediaItem struct {
	ID            string
	OriginalName  string
	FilePath      string
	MediaKind     string
	MimeType      string
	ThumbnailPath string
	PlaybackPath  string
}

var rootDebugMediaItems = []rootDebugMediaItem{
	{
		ID:           "img-7688",
		OriginalName: "IMG_7688.JPG",
		FilePath:     "/Users/starfruit/Pictures/Canon/100CANON/IMG_7688.JPG",
		MediaKind:    "image",
		MimeType:     "image/jpeg",
	},
	{
		ID:           "img-7703",
		OriginalName: "IMG_7703.JPG",
		FilePath:     "/Users/starfruit/Pictures/Canon/100CANON/IMG_7703.JPG",
		MediaKind:    "image",
		MimeType:     "image/jpeg",
	},
	{
		ID:            "mvi-8411",
		OriginalName:  "MVI_8411.MOV",
		FilePath:      "/Users/starfruit/Pictures/Canon/100CANON/MVI_8411.MOV",
		MediaKind:     "video",
		MimeType:      "video/quicktime",
		ThumbnailPath: "",
		PlaybackPath:  "",
	},
}

func findRootDebugMediaItem(id string) *rootDebugMediaItem {
	needle := strings.TrimSpace(id)
	for i := range rootDebugMediaItems {
		if strings.EqualFold(rootDebugMediaItems[i].ID, needle) {
			return &rootDebugMediaItems[i]
		}
	}
	return nil
}

func handleServeRootDebugMedia() gin.HandlerFunc {
	return func(c *gin.Context) {
		item := findRootDebugMediaItem(c.Param("id"))
		if item == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "调试媒体不存在"})
			return
		}
		filePath := filepath.Clean(item.FilePath)
		if filePath == "." || !filepath.IsAbs(filePath) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "调试媒体路径无效"})
			return
		}
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{"error": "调试媒体文件不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取调试媒体失败"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.File(filePath)
	}
}
