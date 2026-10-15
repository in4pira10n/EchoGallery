package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStableAPIKeysExistInChineseCatalog(t *testing.T) {
	content, err := os.ReadFile("../../web/static/strings/zh-CN.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, keys := range []map[string]string{stableAPIErrorKeys, stableAPIMessageKeys} {
		for _, key := range keys {
			value := any(catalog)
			for _, part := range strings.Split(key, ".") {
				object, ok := value.(map[string]any)
				if !ok {
					value = nil
					break
				}
				value = object[part]
			}
			if _, ok := value.(string); !ok {
				t.Errorf("catalog is missing string for %q", key)
			}
		}
	}
}

func TestAPIResponseKeyMiddlewareAddsStableKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apiResponseKeyMiddleware())
	r.GET("/api/error", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
	})
	r.GET("/api/message", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "已添加到相册"})
	})
	r.GET("/api/existing", func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "旧客户端文本", "error_key": "pages.login.errors.invalidCredentials"})
	})
	r.GET("/api/progress", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "正在生成标准缩略图… 已完成 42 项"})
	})

	tests := []struct {
		path string
		code int
		key  string
		name string
	}{
		{path: "/api/error", code: http.StatusNotFound, key: "api.errors.mediaNotFound", name: "error"},
		{path: "/api/message", code: http.StatusOK, key: "api.messages.addedToAlbum", name: "message"},
		{path: "/api/existing", code: http.StatusUnauthorized, key: "pages.login.errors.invalidCredentials", name: "existing"},
		{path: "/api/progress", code: http.StatusOK, key: "api.status.thumbnailGeneratingStandard", name: "progress"},
	}
	for _, test := range tests {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
		if w.Code != test.code {
			t.Fatalf("%s: expected status %d, got %d", test.name, test.code, w.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s: decode response: %v", test.name, err)
		}
		field := "error_key"
		if test.name == "message" || test.name == "progress" {
			field = "message_key"
		}
		if payload[field] != test.key {
			t.Fatalf("%s: expected %s=%q, got %#v", test.name, field, test.key, payload[field])
		}
		if test.name == "progress" {
			params, ok := payload["message_params"].(map[string]any)
			if !ok || params["count"] != float64(42) {
				t.Fatalf("progress params were not preserved: %#v", payload["message_params"])
			}
		}
	}
}

func TestAPIResponseKeyMiddlewarePreservesNoContentAndBinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apiResponseKeyMiddleware())
	r.GET("/api/no-content", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	r.GET("/api/download", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/octet-stream", []byte("binary"))
	})
	r.GET("/api/large", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "已添加到相册", "payload": string(make([]byte, maxKeyedAPIResponseBytes))})
	})

	noContent := httptest.NewRecorder()
	r.ServeHTTP(noContent, httptest.NewRequest(http.MethodGet, "/api/no-content", nil))
	if noContent.Code != http.StatusNoContent || noContent.Body.Len() != 0 {
		t.Fatalf("no-content response changed: status=%d body=%q", noContent.Code, noContent.Body.String())
	}

	binary := httptest.NewRecorder()
	r.ServeHTTP(binary, httptest.NewRequest(http.MethodGet, "/api/download", nil))
	if binary.Code != http.StatusOK || binary.Body.String() != "binary" {
		t.Fatalf("binary response changed: status=%d body=%q", binary.Code, binary.Body.String())
	}

	large := httptest.NewRecorder()
	r.ServeHTTP(large, httptest.NewRequest(http.MethodGet, "/api/large", nil))
	if large.Code != http.StatusOK || large.Body.Len() <= maxKeyedAPIResponseBytes {
		t.Fatalf("large response was not streamed intact: status=%d bytes=%d", large.Code, large.Body.Len())
	}
}
