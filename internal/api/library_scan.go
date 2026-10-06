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
		if scoped := requestRegistrar(c, nil); scoped != nil {
			registrar = scoped
		}
		scanner, ok := registrar.(libraryScanner)
		if !ok || scanner == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "当前实例不支持资源库检查"})
			return
		}
		userID, err := currentLibraryUserID(c, cfg)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		summary, err := scanner.ImportExistingPhotosContext(c.Request.Context(), userID, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"imported": summary.Imported,
			"pruned":   summary.Pruned,
			"skipped":  summary.Skipped,
		})
	}
}
