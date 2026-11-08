package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
	"echogallery/internal/service"
)

type libraryScanner interface {
	ImportExistingPhotosContext(context.Context, int64, func(int, int)) (*service.ImportSummary, error)
}

func handleRefreshCurrentLibrary(cfg *config.Config, registrar interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestService := registrar
		if scoped := requestRegistrar(c, nil); scoped != nil {
			requestService = scoped
		}
		scanner, ok := requestService.(libraryScanner)
		if !ok || scanner == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持资源库检查"})
			return
		}
		userID, err := currentLibraryUserID(c, cfg)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		// This endpoint is an automatic post-entry check. Explicit batch scans
		// remain available for SMB; opening an indexed SMB library never recurses.
		if smb, ok := requestService.(interface{ IsSMBLibrary() bool }); ok && smb.IsSMBLibrary() {
			c.JSON(http.StatusOK, gin.H{"changed": false, "automatic_scan_skipped": true, "reason": "smb"})
			return
		}
		summary, err := scanner.ImportExistingPhotosContext(service.WithBackgroundLibraryScan(c.Request.Context()), userID, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"changed":  summary.Changed || summary.Imported > 0 || summary.Pruned > 0,
			"imported": summary.Imported,
			"pruned":   summary.Pruned,
			"skipped":  summary.Skipped,
		})
	}
}
