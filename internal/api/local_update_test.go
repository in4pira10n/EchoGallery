package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"echogallery/internal/config"
)

func TestUpdateGalleryUsesAccountStarWithoutMigratingInstall(t *testing.T) {
	root := t.TempDir()
	star := filepath.Join(root, "star")
	logo := config.LibraryLogoPath(star)
	os.MkdirAll(filepath.Dir(logo), 0755)
	os.WriteFile(logo, pngSample(t), 0644)
	metadata := map[string]any{
		"active_library_id": "active",
		"libraries":         []config.Library{{ID: "active", Name: "Active", Path: "active"}, {ID: "star", Name: "Starred", Path: "star"}},
		"users":             []config.User{{Username: "alice", DefaultLibraryID: "star"}},
	}
	data, _ := json.Marshal(metadata)
	path := filepath.Join(root, "config.json")
	os.WriteFile(path, data, 0644)
	name, avatar := readUpdateGalleryIdentity(filepath.Join(root, "EchoGallery"), "alice")
	expectedLogo, _ := filepath.EvalSymlinks(logo)
	if name != "Starred" || avatar != expectedLogo {
		t.Fatalf("wrong star identity: %q %q", name, avatar)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("discovery modified remote config")
	}
	if _, err := os.Stat(filepath.Join(root, "echogallery-data")); !os.IsNotExist(err) {
		t.Fatal("discovery created runtime data")
	}
	name, avatar = readUpdateGalleryIdentity(filepath.Join(root, "EchoGallery"), "other")
	if name != "Active" || avatar != "" {
		t.Fatalf("unexpected identity fallback %q %q", name, avatar)
	}
}

func TestLocalUpdateEndpointsRequireAuthentication(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	router := NewRouter(cfg, okRegistrar())
	for _, route := range []struct{ method, path string }{{"GET", "/api/settings/local-update/scan"}, {"GET", "/api/settings/local-update/check"}, {"GET", "/api/settings/local-update/avatar?id=../../config.json"}, {"POST", "/api/settings/local-update/apply"}, {"PUT", "/api/settings/local-update-config"}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", route.method, route.path, response.Code)
		}
	}
}
