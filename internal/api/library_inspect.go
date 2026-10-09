package api

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"echogallery/internal/config"
	"github.com/gin-gonic/gin"
)

func handleInspectPortableLibrary() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请填写资源库路径"})
			return
		}
		library, exists, err := config.InspectPortableLibrary(req.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		logo := ""
		if exists {
			path := config.LibraryLogoPath(req.Path)
			if info, err := os.Stat(path); err == nil && info.Size() <= 5<<20 {
				if data, err := os.ReadFile(path); err == nil {
					logo = "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
					library.LogoAsset = "logo.png"
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"existing": exists, "library": library, "logo_image_url": logo})
	}
}
