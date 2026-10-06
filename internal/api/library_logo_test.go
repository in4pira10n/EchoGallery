package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"echogallery/internal/config"
)

func TestLegacyLibraryLogoCopiesIntoPortableDirectory(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	library := config.Library{ID: "lib_one", Name: "One", Path: t.TempDir(), LogoAsset: "library-old.png"}
	assetDir, err := cfg.LibraryAssetsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(assetDir, library.LogoAsset)
	content := pngSample(t)
	if err := os.WriteFile(oldPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	path, asset := resolveLibraryLogoPath(cfg, library)
	if path != config.LibraryLogoPath(library.Path) || asset != portableLibraryLogoName {
		t.Fatalf("旧 Logo 未复制到资源库: path=%s asset=%s", path, asset)
	}
	copied, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(copied, content) {
		t.Fatalf("资源库 Logo 内容不一致: %v", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("旧版程序的 Logo 文件应保留: %v", err)
	}
}

func TestPortableLibraryLogoSurvivesNewServerConfig(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	library := config.Library{ID: "lib_one", Name: "One", Path: t.TempDir()}
	cfg.Libraries = []config.Library{library}
	cfg.StoragePath = library.Path
	cfg.ActiveLibraryID = library.ID
	logoPath := config.LibraryLogoPath(library.Path)
	if err := os.MkdirAll(filepath.Dir(logoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logoPath, pngSample(t), 0644); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(cfg, okRegistrar())
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	settings := httptest.NewRecorder()
	router.ServeHTTP(settings, req)
	if settings.Code != http.StatusOK {
		t.Fatalf("新配置加载资源库 Logo 失败: %d %s", settings.Code, settings.Body.String())
	}
	var response struct {
		Libraries []struct {
			LogoAsset    string `json:"logo_asset"`
			LogoImageURL string `json:"logo_image_url"`
		} `json:"libraries"`
	}
	if err := json.Unmarshal(settings.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Libraries) != 1 || response.Libraries[0].LogoAsset != "logo.png" || response.Libraries[0].LogoImageURL == "" {
		t.Fatalf("未从资源库自动识别 Logo: %+v", response.Libraries)
	}
	heroReq := httptest.NewRequest(http.MethodGet, "/api/login/hero", nil)
	hero := httptest.NewRecorder()
	router.ServeHTTP(hero, heroReq)
	if hero.Code != http.StatusOK || !bytes.Contains(hero.Body.Bytes(), []byte(`/api/login/hero/avatar/0`)) {
		t.Fatalf("登录页未识别资源库 Logo: %d %s", hero.Code, hero.Body.String())
	}
	logoReq := httptest.NewRequest(http.MethodGet, "/api/settings/libraries/0/logo", nil)
	logoReq.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	logo := httptest.NewRecorder()
	router.ServeHTTP(logo, logoReq)
	if logo.Code != http.StatusOK || !bytes.Equal(logo.Body.Bytes(), pngSample(t)) {
		t.Fatalf("现有 Logo 文件无法读取: %d", logo.Code)
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/settings/libraries/0/logo", nil)
	deleteReq.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, deleteReq)
	if deleted.Code != http.StatusOK {
		t.Fatalf("删除资源库 Logo 失败: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(logoPath); !os.IsNotExist(err) {
		t.Fatalf("资源库 Logo 应已删除: %v", err)
	}
}

func TestLoginHeroReturnsAccountDefaultLibraryPreview(t *testing.T) {
	cfg := testConfig()
	primaryPath := t.TempDir()
	starredPath := t.TempDir()
	cfg.Libraries = []config.Library{
		{ID: "lib_primary", Name: "主要资源库", Path: primaryPath},
		{ID: "lib_starred", Name: "旅行记忆", Path: starredPath},
	}
	cfg.Users[0].DefaultLibraryID = "lib_starred"
	logoPath := config.LibraryLogoPath(starredPath)
	if err := os.MkdirAll(filepath.Dir(logoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logoPath, pngSample(t), 0644); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(cfg, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/auth/account-library?username=alice", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("读取登录页账号资源库预览失败: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		AccountLibrary *struct {
			Name   string `json:"name"`
			Avatar struct {
				URL string `json:"url"`
			} `json:"avatar"`
		} `json:"account_library"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccountLibrary == nil || payload.AccountLibrary.Name != "旅行记忆" {
		t.Fatalf("登录页未返回账号默认资源库: %+v", payload.AccountLibrary)
	}
	if !strings.HasPrefix(payload.AccountLibrary.Avatar.URL, "/api/login/hero/avatar/1?v=") {
		t.Fatalf("登录页未返回账号资源库头像: %+v", payload.AccountLibrary.Avatar)
	}
}

func TestUploadLibraryLogoWritesInsideLibrary(t *testing.T) {
	cfg := testConfig()
	cfg.AppDataDir = t.TempDir()
	cfg.Port = 8080
	library := config.Library{ID: "lib_one", Name: "One", Path: t.TempDir()}
	cfg.Libraries = []config.Library{library}
	cfg.StoragePath = library.Path
	cfg.ActiveLibraryID = library.ID
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(pngSample(t)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(cfg, okRegistrar())
	request := httptest.NewRequest(http.MethodPost, "/api/settings/libraries/0/logo", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: testToken(t, cfg.JWTSecret, "alice")})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("上传资源库 Logo 失败: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(config.LibraryLogoPath(library.Path)); err != nil {
		t.Fatalf("Logo 未写入资源库: %v", err)
	}
	assetDir, err := cfg.LibraryAssetsDir()
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(assetDir); err == nil && len(entries) > 0 {
		t.Fatalf("新上传不应写入服务器共享目录: %+v", entries)
	}
}
