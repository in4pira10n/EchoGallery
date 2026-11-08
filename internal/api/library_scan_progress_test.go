package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"echogallery/internal/config"
	"echogallery/internal/service"
	"github.com/gin-gonic/gin"
)

type scanPolicyStub struct {
	stubRegistrar
	smb   bool
	calls int
}

func (s *scanPolicyStub) IsSMBLibrary() bool        { return s.smb }
func (s *scanPolicyStub) LibraryUserID(int64) int64 { return 1 }
func (s *scanPolicyStub) ImportExistingPhotosContext(context.Context, int64, func(int, int)) (*service.ImportSummary, error) {
	s.calls++
	return &service.ImportSummary{Imported: 1}, nil
}
func TestAutomaticRefreshSkipsOnlySMB(t *testing.T) {
	for _, smb := range []bool{true, false} {
		stub := &scanPolicyStub{smb: smb}
		router := gin.New()
		router.POST("/", func(c *gin.Context) { c.Set(string(requestRegistrarContextKey), stub); c.Next() }, handleRefreshCurrentLibrary(&config.Config{}, stub))
		// Use the same scoped registrar key as the runtime middleware.
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("smb=%v: %d %s", smb, recorder.Code, recorder.Body.String())
		}
		if smb && (stub.calls != 0 || !strings.Contains(recorder.Body.String(), `"automatic_scan_skipped":true`)) {
			t.Fatal("SMB entry scanned", recorder.Body.String())
		}
		if !smb && stub.calls != 1 {
			t.Fatal("local entry no longer scans")
		}
	}
}
func TestRetryLibraryBuildHandler(t *testing.T) {
	for _, test := range []struct {
		hooks LibraryBuildHooks
		want  int
	}{
		{LibraryBuildHooks{}, http.StatusNotImplemented},
		{LibraryBuildHooks{Retry: func() (LibraryBuildStatus, error) { return LibraryBuildStatus{}, errors.New("closing") }}, http.StatusConflict},
		{LibraryBuildHooks{Retry: func() (LibraryBuildStatus, error) { return LibraryBuildStatus{Status: "discovering"}, nil }}, http.StatusAccepted},
	} {
		router := gin.New()
		router.POST("/", handleRetryLibraryBuild(test.hooks))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
		if recorder.Code != test.want {
			t.Fatalf("got %d want %d", recorder.Code, test.want)
		}
	}
}
