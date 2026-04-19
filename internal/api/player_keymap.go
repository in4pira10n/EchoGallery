package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

type playerKeymapResponse struct {
	Content string `json:"content"`
}

func handleGetPlayerKeymap(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg != nil && cfg.Preferences.PlayerKeymap != "" {
			c.JSON(200, playerKeymapResponse{Content: cfg.Preferences.PlayerKeymap})
			return
		}
		content, err := loadPlayerKeymapContent()
		if err != nil {
			c.JSON(200, playerKeymapResponse{Content: ""})
			return
		}
		c.JSON(200, playerKeymapResponse{Content: content})
	}
}

func loadPlayerKeymapContent() (string, error) {
	candidates := []string{"ZXCWASD.conf"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "ZXCWASD.conf"))
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
		}
	}
	return "", os.ErrNotExist
}
