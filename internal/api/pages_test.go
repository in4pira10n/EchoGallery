package api

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestPageHTML_FallsBackWhenRequiredMarkerMissing(t *testing.T) {
	html := pageHTML(
		fstest.MapFS{"web/pages/app.html": {Data: []byte("<html><body>broken</body></html>")}},
		"app.html",
		appPageFallbackHTML,
		[]string{`id="app"`},
	)
	if !strings.Contains(html, `id="app"`) {
		t.Fatalf("缺少关键标记时应使用兜底页面，得到: %s", html)
	}
}

func TestPageHTML_ReplacesAssetVersionPlaceholder(t *testing.T) {
	html := pageHTML(
		fstest.MapFS{
			"web/pages/app.html":      {Data: []byte(`<html><body><div id="app"></div><script src="/static/app.js?v={{ASSET_VERSION}}"></script></body></html>`)},
			"web/static/app.js":       {Data: []byte(`console.log("ok")`)},
			"web/static/pwa.js":       {Data: []byte(`console.log("pwa")`)},
			"web/static/sw.js":        {Data: []byte(`const v = "{{ASSET_VERSION}}"`)},
			"web/static/pwa-icon.png": {Data: []byte(`png`)},
		},
		"app.html",
		appPageFallbackHTML,
		[]string{`id="app"`},
	)
	if strings.Contains(html, assetVersionPlaceholder) {
		t.Fatalf("页面中的资源版本占位符应被替换，得到: %s", html)
	}
	if !strings.Contains(html, `?v=eg-`) {
		t.Fatalf("页面中的资源链接应包含自动版本，得到: %s", html)
	}
}

func TestBuildSetupPageHTML_ReplacesPayload(t *testing.T) {
	html := buildSetupPageHTML(
		fstest.MapFS{
			"web/pages/setup.html":    {Data: []byte(`window.__SETUP__ = __SETUP_PAYLOAD__; <link rel="stylesheet" href="/pages/setup.css?v={{ASSET_VERSION}}"><div id="setup-root"></div>`)},
			"web/static/pwa.js":       {Data: []byte(`console.log("pwa")`)},
			"web/static/sw.js":        {Data: []byte(`const v = "{{ASSET_VERSION}}"`)},
			"web/static/pwa-icon.png": {Data: []byte(`png`)},
		},
		`{"mode":"init"}`,
	)
	if !strings.Contains(html, `{"mode":"init"}`) || strings.Contains(html, "__SETUP_PAYLOAD__") {
		t.Fatalf("setup 页面应替换 payload，得到: %s", html)
	}
	if strings.Contains(html, assetVersionPlaceholder) {
		t.Fatalf("setup 页面中的资源版本占位符应被替换，得到: %s", html)
	}
}
