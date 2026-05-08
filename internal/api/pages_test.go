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

func TestBuildSetupPageHTML_ReplacesPayload(t *testing.T) {
	html := buildSetupPageHTML(
		fstest.MapFS{"web/pages/setup.html": {Data: []byte(`window.__SETUP__ = __SETUP_PAYLOAD__; <div id="setup-root"></div>`)}},
		`{"mode":"init"}`,
	)
	if !strings.Contains(html, `{"mode":"init"}`) || strings.Contains(html, "__SETUP_PAYLOAD__") {
		t.Fatalf("setup 页面应替换 payload，得到: %s", html)
	}
}
