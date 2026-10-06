package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"echogallery/internal/config"
)

func TestBatchBuildHandlersRejectPausedStages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name    string
		payload string
		handler gin.HandlerFunc
	}{
		{"scan migration", `{"move_legacy_thumbnails":true}`, handleStartLibraryBatchBuild(testConfig(), LibraryBatchBuildHooks{
			Start: func(*config.Config, *config.Profile, string, int64, bool, bool, bool, bool, bool) (LibraryBatchBuildStatus, error) {
				t.Fatal("迁移任务不应启动")
				return LibraryBatchBuildStatus{}, nil
			},
		})},
		{"thumbnail cleanup", `{"clean_thumbnail_files":true}`, handleStartLibraryBatchThumbnailBuild(testConfig(), LibraryBatchThumbnailBuildHooks{
			Start: func(*config.Config, *config.Profile, string, int64, bool, bool, bool, bool) (LibraryBatchBuildStatus, error) {
				t.Fatal("清理任务不应启动")
				return LibraryBatchBuildStatus{}, nil
			},
		})},
		{"thumbnail playback cache", `{"build_playback_caches":true}`, handleStartLibraryBatchThumbnailBuild(testConfig(), LibraryBatchThumbnailBuildHooks{
			Start: func(*config.Config, *config.Profile, string, int64, bool, bool, bool, bool) (LibraryBatchBuildStatus, error) {
				t.Fatal("批量播放缓存不应启动")
				return LibraryBatchBuildStatus{}, nil
			},
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/batch", test.handler)
			request := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewBufferString(test.payload))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("暂停的阶段应返回 400，得到 %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
