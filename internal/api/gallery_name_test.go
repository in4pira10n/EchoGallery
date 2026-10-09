package api

import (
	"echogallery/internal/config"
	"strings"
	"testing"
)

func TestGalleryNamePageEscaping(t *testing.T) {
	cfg := &config.Config{Workshop: config.Workshop{AppName: `Family </script><b>`}}
	page := `<head><title>注册 - EchoGallery</title><meta content="EchoGallery"></head>` +
		`<strong id="register-form-title">注册 EchoGallery</strong><strong>注册 EchoGallery</strong><strong>登录 EchoGallery</strong>`
	result := applyGalleryName(page, []*config.Config{cfg})
	if !strings.Contains(result, `Family &lt;/script&gt;&lt;b&gt;</title>`) ||
		!strings.Contains(result, `\u003c/script\u003e`) ||
		!strings.Contains(result, `id="register-form-title">注册 Family &lt;/script&gt;&lt;b&gt;`) ||
		!strings.Contains(result, `<strong>注册 Family &lt;/script&gt;&lt;b&gt;</strong>`) ||
		!strings.Contains(result, `<strong>登录 Family &lt;/script&gt;&lt;b&gt;</strong>`) {
		t.Fatalf("unsafe or missing name: %s", result)
	}
}
