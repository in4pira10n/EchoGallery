package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"echogallery/internal/config"
	"echogallery/internal/storage"
)

func TestPlaybackErrorsAreWrittenToHostDataLog(t *testing.T) {
	root := t.TempDir()
	appDataDir := filepath.Join(root, "echogallery-data")
	libraryPath := filepath.Join(root, "library")
	if err := os.MkdirAll(libraryPath, 0700); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(libraryPath, "clip.mp4")
	if err := os.WriteFile(sourcePath, []byte("video source"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.AppDataDir = appDataDir
	cfg.StoragePath = libraryPath
	cfg.ActiveLibraryID = "library-1"
	cfg.Libraries = []config.Library{{ID: "library-1", Name: "Camera Roll", Path: libraryPath}}
	photo := &storage.Photo{
		ID: 99, UUID: "clip-uuid", OriginalName: "clip.mp4", MediaKind: storage.MediaKindVideo,
		MimeType: "video/mp4", Size: 1234, Width: 1920, Height: 1080, DurationMS: 8000,
		SourceRelPath: "clip.mp4", UploadedBy: 1,
		EXIF: &storage.PhotoEXIF{VideoCodec: "hevc", VideoFrameRate: 29.97},
	}
	registrar := okRegistrar()
	registrar.getPhoto = func(id, userID int64) (*storage.Photo, error) {
		return photo, nil
	}
	registrar.getByUUID = func(uuid string, userID int64) (*storage.Photo, error) {
		return photo, nil
	}
	registrar.mediaPath = func(*storage.Photo) string { return sourcePath }
	registrar.browserPlaybackPath = func(*storage.Photo) (string, string, error) {
		return "", "", fmt.Errorf("browser playback cache is missing")
	}
	router := NewRouter(cfg, registrar)
	token := testToken(t, cfg.JWTSecret, "alice")

	serverRequest := httptest.NewRequest(http.MethodGet, "/media/playback/clip-uuid", nil)
	serverRequest.AddCookie(&http.Cookie{Name: authCookieNameForConfig(cfg), Value: token})
	serverResponse := httptest.NewRecorder()
	router.ServeHTTP(serverResponse, serverRequest)
	if serverResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("server playback error expected %d, got %d", http.StatusUnsupportedMediaType, serverResponse.Code)
	}

	clientRequest := httptest.NewRequest(http.MethodPost, "/api/media/99/playback-error", strings.NewReader(`{"code":3,"code_name":"MEDIA_ERR_DECODE","message":"decoder failed","ready_state":1,"network_state":3,"current_time":2.5,"duration":8,"video_width":0,"video_height":0,"source_url_path":"http://localhost/media/playback/clip-uuid?eg_page_session=secret"}`))
	clientRequest.Header.Set("Content-Type", "application/json")
	clientRequest.Header.Set("User-Agent", "ExampleBrowser/1.0")
	clientRequest.AddCookie(&http.Cookie{Name: authCookieNameForConfig(cfg), Value: token})
	clientResponse := httptest.NewRecorder()
	router.ServeHTTP(clientResponse, clientRequest)
	if clientResponse.Code != http.StatusNoContent {
		t.Fatalf("client error report expected %d, got %d: %s", http.StatusNoContent, clientResponse.Code, clientResponse.Body.String())
	}

	content, err := os.ReadFile(filepath.Join(appDataDir, playbackErrorLogName))
	if err != nil {
		t.Fatalf("read playback error log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two playback reports, got %d: %s", len(lines), content)
	}
	var entries []playbackErrorLogEntry
	for _, line := range lines {
		var entry playbackErrorLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode playback report: %v", err)
		}
		entries = append(entries, entry)
	}
	if entries[0].Event != "server_playback_path_error" || entries[0].Failure.Message != "browser playback cache is missing" {
		t.Fatalf("server failure details missing from first report: %+v", entries[0])
	}
	clientEntry := entries[1]
	if clientEntry.Event != "browser_playback_error" || clientEntry.Media.VideoCodec != "hevc" || clientEntry.Library.Name != "Camera Roll" {
		t.Fatalf("client report is missing media or library context: %+v", clientEntry)
	}
	if clientEntry.Client.Playback == nil || clientEntry.Client.Playback.CodeName != "MEDIA_ERR_DECODE" {
		t.Fatalf("browser media error details missing: %+v", clientEntry.Client)
	}
	if clientEntry.Client.Playback.SourceURLPath != "/media/playback/clip-uuid" {
		t.Fatalf("playback URL query should be omitted, got %q", clientEntry.Client.Playback.SourceURLPath)
	}
	if strings.Contains(string(content), "eg_page_session=secret") {
		t.Fatal("playback error log must not contain the page session query")
	}
	info, err := os.Stat(filepath.Join(appDataDir, playbackErrorLogName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("playback error log should be private, mode is %o", info.Mode().Perm())
	}
}
