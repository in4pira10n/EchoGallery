package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

func TestBatchCleanFilesIsExplicitAndForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enabled := range []bool{false, true} {
		cfg := testConfig()
		cfg.AppDataDir, cfg.StoragePath = t.TempDir(), t.TempDir()
		called := false
		handler := handleStartLibraryBatchBuild(cfg, LibraryBatchBuildHooks{
			Start: func(_ *config.Config, _ *config.Profile, _ string, _ int64, aggressive, thumbnails, exif, migrate, oldCleanup, playback, cleanFiles bool) (LibraryBatchBuildStatus, error) {
				called = true
				if cleanFiles != enabled || migrate || oldCleanup || playback {
					t.Fatalf("unexpected workflow flags: clean=%v old=%v/%v/%v", cleanFiles, migrate, oldCleanup, playback)
				}
				return LibraryBatchBuildStatus{Status: "running"}, nil
			},
		})
		router := gin.New()
		router.POST("/batch", func(c *gin.Context) { c.Set(string(userContextKey), "alice"); handler(c) })
		body := `{}`
		if enabled {
			body = `{"clean_files":true}`
		}
		request := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !called {
			t.Fatalf("clean=%v: %d %s", enabled, response.Code, response.Body.String())
		}
	}
}
