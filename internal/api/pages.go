package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"echogallery/internal/config"

	"github.com/gin-gonic/gin"
)

const assetVersionPlaceholder = "{{ASSET_VERSION}}"

const appPageFallbackHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta name="theme-color" content="#2d6a5f">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-title" content="EchoGallery">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="mobile-web-app-capable" content="yes">
  <title>EchoGallery</title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/app.css?v={{ASSET_VERSION}}">
  <script src="/static/pwa.js?v={{ASSET_VERSION}}" defer></script>
</head>
<body>
  <div id="app"></div>
  <script src="/static/lightbox.js?v={{ASSET_VERSION}}"></script>
  <script src="/static/app.js?v={{ASSET_VERSION}}"></script>
</body>
</html>`

const loginPageFallbackHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover">
  <meta name="theme-color" content="#2d6a5f">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-title" content="EchoGallery">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="mobile-web-app-capable" content="yes">
  <title>登录 - EchoGallery</title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/common.css?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/login.css?v={{ASSET_VERSION}}">
  <script src="/static/pwa.js?v={{ASSET_VERSION}}" defer></script>
</head>
<body>
<div class="login-wrap">
  <div class="login-card">
    <section class="login-form-panel" aria-label="登录">
      <div class="login-form-head">
        <span>本地访问</span>
        <strong>登录 EchoGallery</strong>
      </div>
      <div class="form-group">
        <label class="form-label" for="username">用户名</label>
        <input class="input" id="username" type="text" autocomplete="username" placeholder="请输入用户名">
      </div>
      <div class="form-group">
        <label class="form-label" for="password">密码</label>
        <input class="input" id="password" type="password" autocomplete="current-password" placeholder="请输入密码">
      </div>
      <button class="btn btn-primary login-submit" id="login-btn">登录</button>
      <a class="login-secondary-link" href="/register">注册新用户</a>
      <div class="login-error" id="login-error"></div>
    </section>
  </div>
</div>
<script>
(function() {
  var t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  document.documentElement.dataset.theme = t;
  var btn = document.getElementById('login-btn');
  var errEl = document.getElementById('login-error');
  function ensurePageSessionID() {
    try {
      var key = 'echogallery_page_session_id_v1';
      var current = sessionStorage.getItem(key) || '';
      if (!current) {
        current = (window.crypto && typeof window.crypto.randomUUID === 'function')
          ? window.crypto.randomUUID()
          : ('eg-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 12));
        sessionStorage.setItem(key, current);
      }
      return current;
    } catch (_) {
      return (window.crypto && typeof window.crypto.randomUUID === 'function')
        ? window.crypto.randomUUID()
        : ('eg-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 12));
    }
  }
  function resetPageSessionID() {
    var next = (window.crypto && typeof window.crypto.randomUUID === 'function')
      ? window.crypto.randomUUID()
      : ('eg-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 12));
    try {
      sessionStorage.setItem('echogallery_page_session_id_v1', next);
    } catch (_) {}
    return next;
  }
  function markPageSessionTransfer(sessionID) {
    try {
      sessionStorage.setItem('echogallery_page_session_transfer_v1', sessionID);
    } catch (_) {}
  }
  function loginReasonMessage(reason) {
    switch (String(reason || '').trim()) {
      case 'duplicate':
        return '该账号已在新页面登录，当前页面已退出';
      case 'expired':
        return '页面会话已过期，请重新登录';
      case 'no-library':
        return '当前没有可进入的资源库，请稍后再试';
      default:
        return '';
    }
  }
  async function doLogin() {
    var u = document.getElementById('username').value.trim();
    var p = document.getElementById('password').value;
    if (!u || !p) { errEl.textContent = '请输入用户名和密码'; return; }
    btn.disabled = true; btn.textContent = '登录中…';
    var pageSessionID = resetPageSessionID();
    try {
      var r = await fetch('/api/auth/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-EG-Page-Session': pageSessionID
        },
        body: JSON.stringify({username: u, password: p})
      });
      var d = await r.json().catch(function () { return {}; });
      if (r.ok) {
        errEl.style.color = 'var(--accent)';
        errEl.textContent = d.message || '';
        markPageSessionTransfer(pageSessionID);
        var target = d.redirect || '/';
        var delay = Math.max(0, Number(d.delay_ms) || 0);
        setTimeout(function () { location.href = target; }, delay);
        return;
      }
      errEl.textContent = d.error || loginReasonMessage(d.reason) || '登录失败';
    } catch(e) { errEl.textContent = '网络错误'; }
    btn.disabled = false; btn.textContent = '登录';
  }
  var initialReason = new URLSearchParams(location.search).get('reason');
  var reasonCopy = loginReasonMessage(initialReason);
  if (reasonCopy) errEl.textContent = reasonCopy;
  btn.addEventListener('click', doLogin);
  document.addEventListener('keydown', function(e){ if(e.key === 'Enter') doLogin(); });
})();
</script>
</body>
</html>`

const registerPageFallbackHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover">
  <meta name="theme-color" content="#2d6a5f">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-title" content="EchoGallery">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="mobile-web-app-capable" content="yes">
  <title>注册 - EchoGallery</title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/common.css?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/login.css?v={{ASSET_VERSION}}">
  <script src="/static/pwa.js?v={{ASSET_VERSION}}" defer></script>
</head>
<body class="register-page">
<a class="timeline-order-btn" id="register-back-btn" href="/login" aria-label="返回登录" title="返回登录">
  <svg data-icon-source width="24" height="24" viewBox="0 0 24 24" aria-hidden="true"><use href="/static/svg/back.svg#back-icon"></use></svg>
</a>
<main class="auth-screen">
  <section class="auth-stage" aria-hidden="true"></section>
  <section class="auth-pane" aria-label="注册">
    <div class="auth-brand">
      <img src="/static/pwa-icon.png" alt="">
      <p><strong id="register-form-title">注册 EchoGallery</strong></p>
    </div>
    <div class="auth-fields">
      <div class="auth-field">
        <label for="username">用户名</label>
        <input id="username" type="text" autocomplete="username" placeholder="请输入用户名">
      </div>
      <div class="auth-field">
        <label for="password">密码</label>
        <input id="password" type="password" autocomplete="new-password" placeholder="至少 6 位">
      </div>
      <div class="auth-field">
        <label for="confirm-password">确认密码</label>
        <input id="confirm-password" type="password" autocomplete="new-password" placeholder="再次输入密码">
      </div>
    </div>
    <div class="form-group register-role-group">
      <label class="form-label" for="role">账号类型</label>
      <select class="input" id="role">
        <option value="visitor">访客用户</option>
        <option value="admin">管理员账户</option>
      </select>
    </div>
    <div id="register-admin-fields" hidden>
      <div class="form-group">
        <label class="form-label" for="admin-username">管理员用户名</label>
        <input class="input" id="admin-username" type="text" autocomplete="username" placeholder="输入现有管理员用户名">
      </div>
      <div class="form-group">
        <label class="form-label" for="admin-password">管理员密码</label>
        <input class="input" id="admin-password" type="password" autocomplete="current-password" placeholder="输入现有管理员密码">
      </div>
    </div>
    <button class="btn btn-primary login-submit" id="register-btn">创建账号</button>
    <a class="login-secondary-link" href="/login">已有账号？返回登录</a>
    <div class="login-error" id="register-error"></div>
  </section>
</main>
<script>
(function() {
  var t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  document.documentElement.dataset.theme = t;
  var btn = document.getElementById('register-btn');
  var errEl = document.getElementById('register-error');
  var roleEl = document.getElementById('role');
  var adminFieldsEl = document.getElementById('register-admin-fields');
  function syncRole() {
    var isAdmin = roleEl && roleEl.value === 'admin';
    if (adminFieldsEl) adminFieldsEl.hidden = !isAdmin;
    btn.textContent = isAdmin ? '创建管理员账号' : '创建访客账号';
  }
  syncRole();
  if (roleEl) roleEl.addEventListener('change', syncRole);
  async function doRegister() {
    var u = document.getElementById('username').value.trim();
    var p = document.getElementById('password').value;
    var c = document.getElementById('confirm-password').value;
    var role = roleEl && roleEl.value === 'admin' ? 'admin' : 'visitor';
    var adminUsername = document.getElementById('admin-username') ? document.getElementById('admin-username').value.trim() : '';
    var adminPassword = document.getElementById('admin-password') ? document.getElementById('admin-password').value : '';
    if (!u || !p || !c) { errEl.textContent = '请完整填写注册信息'; return; }
    if (p !== c) { errEl.textContent = '两次输入的密码不一致'; return; }
    if (role === 'admin' && (!adminUsername || !adminPassword)) { errEl.textContent = '请填写管理员用户名和密码'; return; }
    btn.disabled = true; btn.textContent = '创建中…';
    try {
      var r = await fetch('/api/auth/register', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({username: u, password: p, confirm_password: c, role: role, admin_username: adminUsername, admin_password: adminPassword})
      });
      if (r.ok) { location.href = '/'; return; }
      var d = await r.json();
      errEl.textContent = d.error || '注册失败';
    } catch(e) { errEl.textContent = '网络错误'; }
    btn.disabled = false; btn.textContent = '创建账号';
  }
  btn.addEventListener('click', doRegister);
  document.addEventListener('keydown', function(e){ if(e.key === 'Enter') doRegister(); });
})();
</script>
</body>
</html>`

func pageHTML(staticFS fs.FS, relPath string, fallback string, required []string) string {
	content, err := readPageAsset(staticFS, relPath)
	if err != nil {
		log.Printf("警告: 页面资源 %s 不可用，使用内置兜底: %v", relPath, err)
		return applyAssetVersionTemplate(staticFS, fallback)
	}
	html := string(content)
	for _, marker := range required {
		if !strings.Contains(html, marker) {
			log.Printf("警告: 页面资源 %s 缺少关键标记 %q，使用内置兜底", relPath, marker)
			return applyAssetVersionTemplate(staticFS, fallback)
		}
	}
	return applyAssetVersionTemplate(staticFS, html)
}

func assetSourceFS(staticFS fs.FS) fs.FS {
	if staticFS != nil {
		return staticFS
	}
	return os.DirFS(".")
}

func frontendAssetVersion(staticFS fs.FS) string {
	source := assetSourceFS(staticFS)
	hasher := sha256.New()
	var paths []string
	for _, root := range []string{"web/pages", "web/static"} {
		if err := fs.WalkDir(source, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			paths = append(paths, path)
			return nil
		}); err != nil {
			log.Printf("警告: 计算前端资源版本失败: %v", err)
			return "eg-dev"
		}
	}
	if len(paths) == 0 {
		return "eg-dev"
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			log.Printf("警告: 读取前端资源 %s 失败: %v", path, err)
			return "eg-dev"
		}
		_, _ = hasher.Write([]byte(path))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(data)
		_, _ = hasher.Write([]byte{0})
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if len(sum) < 12 {
		return "eg-dev"
	}
	return "eg-" + sum[:12]
}

func applyAssetVersionTemplate(staticFS fs.FS, content string) string {
	if !strings.Contains(content, assetVersionPlaceholder) {
		return content
	}
	return strings.ReplaceAll(content, assetVersionPlaceholder, frontendAssetVersion(staticFS))
}

func readPageAsset(staticFS fs.FS, relPath string) ([]byte, error) {
	path := "web/pages/" + relPath
	if staticFS != nil {
		if data, err := fs.ReadFile(staticFS, path); err == nil {
			return data, nil
		}
	}
	return os.ReadFile(path)
}

func readStaticAsset(staticFS fs.FS, relPath string) ([]byte, error) {
	path := "web/static/" + relPath
	if staticFS != nil {
		if data, err := fs.ReadFile(staticFS, path); err == nil {
			return data, nil
		}
	}
	return os.ReadFile(path)
}

func handleWebManifest(staticFS fs.FS, configs ...*config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := readStaticAsset(staticFS, "manifest.webmanifest")
		if err != nil {
			c.String(http.StatusNotFound, "manifest not found")
			return
		}
		c.Header("Cache-Control", "no-cache")
		if len(configs) > 0 {
			var manifest map[string]any
			if json.Unmarshal(data, &manifest) == nil {
				manifest["name"], manifest["short_name"] = configs[0].DisplayName(), configs[0].DisplayName()
				data, _ = json.Marshal(manifest)
			}
		}
		c.Data(http.StatusOK, "application/manifest+json; charset=utf-8", []byte(applyAssetVersionTemplate(staticFS, string(data))))
	}
}

func handleServiceWorker(staticFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := readStaticAsset(staticFS, "sw.js")
		if err != nil {
			c.String(http.StatusNotFound, "service worker not found")
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Header("Service-Worker-Allowed", "/")
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(applyAssetVersionTemplate(staticFS, string(data))))
	}
}

func handleAppPage(staticFS fs.FS, configs ...*config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := pageHTML(staticFS, "app.html", appPageFallbackHTML, []string{`id="app"`, `/static/app.js`})
		html = applyGalleryName(html, configs)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}

func handleLoginPage(staticFS fs.FS, configs ...*config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := pageHTML(staticFS, "login.html", loginPageFallbackHTML, []string{`id="login-btn"`, `/api/auth/login`})
		html = applyGalleryName(html, configs)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}

func handleRegisterPage(staticFS fs.FS, configs ...*config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := pageHTML(staticFS, "register.html", registerPageFallbackHTML, []string{`id="register-btn"`, `/api/auth/register`})
		html = applyGalleryName(html, configs)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}

func applyGalleryName(content string, configs []*config.Config) string {
	if len(configs) == 0 {
		return content
	}
	name := configs[0].DisplayName()
	encoded, _ := json.Marshal(name)
	content = strings.Replace(content, "<head>", "<head><script>window.__GALLERY_NAME__="+string(encoded)+";</script>", 1)
	escapedName := html.EscapeString(name)
	content = strings.ReplaceAll(content, `content="EchoGallery"`, `content="`+escapedName+`"`)
	content = strings.ReplaceAll(content, `id="login-library-name">EchoGallery`, `id="login-library-name">`+escapedName)
	content = strings.ReplaceAll(content, `id="register-form-title">注册 EchoGallery`, `id="register-form-title">注册 `+escapedName)
	content = strings.ReplaceAll(content, `<strong>登录 EchoGallery</strong>`, `<strong>登录 `+escapedName+`</strong>`)
	content = strings.ReplaceAll(content, `<strong>注册 EchoGallery</strong>`, `<strong>注册 `+escapedName+`</strong>`)
	return strings.ReplaceAll(content, "EchoGallery</title>", escapedName+"</title>")
}
