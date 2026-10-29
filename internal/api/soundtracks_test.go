package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"echogallery/internal/config"
	"echogallery/internal/media"
	"echogallery/internal/service"
	"echogallery/internal/storage"
)

type soundtrackTestRegistrar struct {
	stubRegistrar
	list  func(context.Context, int64, int64) ([]media.AudioTrack, error)
	build func(context.Context, int64, int64, int) (*service.SoundtrackCacheEntry, error)
	cache func(*storage.Photo, int) (string, error)
}

func (s soundtrackTestRegistrar) ListVideoSoundtracks(ctx context.Context, id, user int64) ([]media.AudioTrack, error) {
	return s.list(ctx, id, user)
}
func (s soundtrackTestRegistrar) BuildVideoSoundtrack(ctx context.Context, id, user int64, index int) (*service.SoundtrackCacheEntry, error) {
	return s.build(ctx, id, user, index)
}
func (s soundtrackTestRegistrar) SoundtrackPlaybackPath(photo *storage.Photo, index int) (string, error) {
	return s.cache(photo, index)
}

func TestSoundtrackAPIValidatesIndexAndServesRangeRequests(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.StoragePath = t.TempDir()
	cache := filepath.Join(t.TempDir(), "selected.mp4")
	if err := os.WriteFile(cache, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	builds := 0
	backend := soundtrackTestRegistrar{stubRegistrar: okRegistrar(),
		list: func(_ context.Context, id, user int64) ([]media.AudioTrack, error) {
			if id != 7 || user != 1 {
				t.Fatalf("wrong identity: %d %d", id, user)
			}
			return []media.AudioTrack{{Index: 0}}, nil
		},
		build: func(_ context.Context, id, user int64, index int) (*service.SoundtrackCacheEntry, error) {
			builds++
			if id != 7 || user != 1 || index != 0 {
				t.Fatalf("wrong identity/index: %d %d %d", id, user, index)
			}
			return &service.SoundtrackCacheEntry{Track: media.AudioTrack{Index: 0}, URL: "/media/playback/x?soundtrack=0"}, nil
		},
		cache: func(_ *storage.Photo, index int) (string, error) {
			if index != 0 {
				t.Fatalf("wrong serve index: %d", index)
			}
			return cache, nil
		},
	}
	router := NewRouter(cfg, backend)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/media/7/soundtracks", "", 200},
		{"POST", "/api/media/7/soundtracks", `{"index":0}`, 200},
		{"POST", "/api/media/7/soundtracks", `{}`, 400},
		{"POST", "/api/media/7/soundtracks", `{"index":-1}`, 400},
		{"POST", "/api/media/7/soundtracks", `{"index":1024}`, 400},
		{"GET", "/media/playback/x?soundtrack=0", "", 206},
		{"GET", "/media/playback/x?soundtrack=../../x", "", 404},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		if strings.HasPrefix(tc.path, "/media/") {
			req.Header.Set("Range", "bytes=2-5")
		}
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.status == 206 && (w.Body.String() != "2345" || w.Header().Get("Content-Type") != "video/mp4") {
			t.Fatalf("range response: %s %v", w.Body.String(), w.Header())
		}
	}
	if builds != 1 {
		t.Fatalf("invalid requests reached converter: %d", builds)
	}
	backend.list = func(context.Context, int64, int64) ([]media.AudioTrack, error) { return nil, media.ErrProbeUnavailable }
	router = NewRouter(cfg, backend)
	req := httptest.NewRequest("GET", "/api/media/7/soundtracks", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "app.lightbox.audioTrackToolsRequired") {
		t.Fatalf("tools error lost localization: %d %s", w.Code, w.Body.String())
	}
}

func TestLegacyDirectoriesDoNotRequireLibraryRebuild(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.StoragePath = t.TempDir()
	cfg.Libraries = []config.Library{{ID: "current", Name: "Current", Path: cfg.StoragePath, OwnerUsername: "alice"}}
	for _, name := range []string{"media", "thumbnails", "db"} {
		if err := os.MkdirAll(filepath.Join(cfg.AppDataDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfg.AppDataDir, "db", "echogallery-old.db"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	libraries := buildLibraryResponses(cfg, "alice", LibraryAvailabilityHooks{})
	if len(libraries) != 1 {
		t.Fatalf("library response=%v", libraries)
	}
	data, err := json.Marshal(libraries)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "rebuild_required") || strings.Contains(string(data), "旧版服务器") {
		t.Fatalf("legacy prompt retained: %s", data)
	}
}
