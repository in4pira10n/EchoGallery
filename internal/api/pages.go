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
  <script src="/static/i18n.js?v={{ASSET_VERSION}}"></script>
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
  <title></title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/common.css?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/login.css?v={{ASSET_VERSION}}">
  <script src="/static/i18n.js?v={{ASSET_VERSION}}"></script>
  <script src="/static/pwa.js?v={{ASSET_VERSION}}" defer></script>
</head>
<body>
<div class="login-wrap">
  <div class="login-card">
    <section class="login-form-panel" aria-label="">
      <div class="login-form-head">
        <span id="login-local-access"></span>
        <strong id="login-form-title"></strong>
      </div>
      <div class="form-group">
        <label class="form-label" for="username" id="login-username-label"></label>
        <input class="input" id="username" type="text" autocomplete="username" placeholder="">
      </div>
      <div class="form-group">
        <label class="form-label" for="password" id="login-password-label"></label>
        <input class="input" id="password" type="password" autocomplete="current-password" placeholder="">
      </div>
      <button class="btn btn-primary login-submit" id="login-btn"></button>
      <a class="login-secondary-link" id="login-register-link" href="/register"></a>
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
  var copy = {};
  function applyCopy(remote) {
    copy = remote || {};
    document.title = String(copy.pageTitle || '').replace(/EchoGallery/g, window.__GALLERY_NAME__ || 'EchoGallery');
    var pane = document.querySelector('.login-form-panel');
    if (pane) pane.setAttribute('aria-label', copy.formTitle || '');
    var map = {
      'login-local-access': copy.localAccess,
      'login-form-title': copy.formTitle,
      'login-username-label': copy.usernameLabel,
      'login-password-label': copy.passwordLabel,
      'login-btn': copy.submit,
      'login-register-link': copy.registerLink
    };
    Object.keys(map).forEach(function(id) {
      var node = document.getElementById(id);
      if (node && typeof map[id] === 'string') node.textContent = map[id];
    });
    var username = document.getElementById('username');
    var password = document.getElementById('password');
    if (username) username.placeholder = copy.usernamePlaceholder || '';
    if (password) password.placeholder = copy.passwordPlaceholder || '';
  }
  var copyReady = window.EchoGalleryI18n.load()
    .then(function(catalog) { applyCopy(catalog && catalog.pages && catalog.pages.login); })
    .catch(function() { applyCopy({}); });
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
        return copy.reasons && copy.reasons.duplicate || '';
      case 'expired':
        return copy.reasons && copy.reasons.expired || '';
      case 'no-library':
        return copy.reasons && copy.reasons.noLibrary || '';
      default:
        return '';
    }
  }
  function responseCopy(payload, field, fallback) {
    var key = payload && payload[field + '_key'];
    var localized = key && window.EchoGalleryI18n.text(key);
    return localized || (payload && payload[field]) || fallback || '';
  }
  async function doLogin() {
    await copyReady;
    var u = document.getElementById('username').value.trim();
    var p = document.getElementById('password').value;
    if (!u || !p) { errEl.textContent = copy.errors && copy.errors.missingFields || ''; return; }
    btn.disabled = true; btn.textContent = (copy.submit || '') + (copy.submittingSuffix || '');
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
        errEl.textContent = responseCopy(d, 'message', '');
        markPageSessionTransfer(pageSessionID);
        var target = d.redirect || '/';
        var delay = Math.max(0, Number(d.delay_ms) || 0);
        setTimeout(function () { location.href = target; }, delay);
        return;
      }
      errEl.textContent = responseCopy(d, 'error', loginReasonMessage(d.reason) || copy.errors && copy.errors.failed || '');
    } catch(e) { errEl.textContent = copy.errors && copy.errors.network || ''; }
    btn.disabled = false; btn.textContent = copy.submit || '';
  }
  var initialReason = new URLSearchParams(location.search).get('reason');
  var reasonCopy = loginReasonMessage(initialReason);
  if (reasonCopy) errEl.textContent = reasonCopy;
  copyReady.then(function() {
    var reason = loginReasonMessage(initialReason);
    if (reason) errEl.textContent = reason;
  });
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
  <title></title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/common.css?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/login.css?v={{ASSET_VERSION}}">
  <script src="/static/i18n.js?v={{ASSET_VERSION}}"></script>
  <script src="/static/pwa.js?v={{ASSET_VERSION}}" defer></script>
</head>
<body class="register-page">
<a class="timeline-order-btn" id="register-back-btn" href="/login" aria-label="" title="">
  <svg data-icon-source width="24" height="24" viewBox="0 0 24 24" aria-hidden="true"><use href="/static/svg/back.svg#back-icon"></use></svg>
</a>
<main class="auth-screen">
  <section class="auth-stage" aria-hidden="true"></section>
  <section class="auth-pane" aria-label="">
    <div class="auth-brand">
      <img src="/static/pwa-icon.png" alt="">
      <p><strong id="register-form-title"></strong></p>
    </div>
    <div class="auth-fields">
      <div class="auth-field">
        <label for="username" id="register-username-label"></label>
        <input id="username" type="text" autocomplete="username" placeholder="">
      </div>
      <div class="auth-field">
        <label for="password" id="register-password-label"></label>
        <input id="password" type="password" autocomplete="new-password" placeholder="">
      </div>
      <div class="auth-field">
        <label for="confirm-password" id="register-confirm-label"></label>
        <input id="confirm-password" type="password" autocomplete="new-password" placeholder="">
      </div>
    </div>
    <div class="form-group register-role-group">
      <label class="form-label" for="role" id="register-role-label"></label>
      <select class="input" id="role">
        <option value="visitor" id="register-visitor-option"></option>
        <option value="admin" id="register-admin-option"></option>
      </select>
    </div>
    <div id="register-admin-fields" hidden>
      <div class="form-group">
        <label class="form-label" for="admin-username" id="register-admin-username-label"></label>
        <input class="input" id="admin-username" type="text" autocomplete="username" placeholder="">
      </div>
      <div class="form-group">
        <label class="form-label" for="admin-password" id="register-admin-password-label"></label>
        <input class="input" id="admin-password" type="password" autocomplete="current-password" placeholder="">
      </div>
    </div>
    <button class="btn btn-primary login-submit" id="register-btn"></button>
    <a class="login-secondary-link" id="register-login-link" href="/login"></a>
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
  var copy = {};
  function applyCopy(remote) {
    copy = remote || {};
    document.title = String(copy.pageTitle || '').replace(/EchoGallery/g, window.__GALLERY_NAME__ || 'EchoGallery');
    var pane = document.querySelector('.auth-pane');
    if (pane) pane.setAttribute('aria-label', copy.paneLabel || '');
    var map = {
      'register-form-title': String(copy.formTitle || '').replace(/EchoGallery/g, window.__GALLERY_NAME__ || 'EchoGallery'),
      'register-username-label': copy.usernameLabel,
      'register-password-label': copy.passwordLabel,
      'register-confirm-label': copy.confirmLabel,
      'register-role-label': copy.roleLabel,
      'register-visitor-option': copy.visitorRoleTitle,
      'register-admin-option': copy.adminRoleTitle,
      'register-admin-username-label': copy.adminUsernameLabel,
      'register-admin-password-label': copy.adminPasswordLabel,
      'register-login-link': copy.loginLink
    };
    Object.keys(map).forEach(function(id) {
      var node = document.getElementById(id);
      if (node && typeof map[id] === 'string') node.textContent = map[id];
    });
    var username = document.getElementById('username');
    var password = document.getElementById('password');
    var confirm = document.getElementById('confirm-password');
    var adminUsername = document.getElementById('admin-username');
    var adminPassword = document.getElementById('admin-password');
    if (username) username.placeholder = copy.usernamePlaceholder || '';
    if (password) password.placeholder = copy.passwordPlaceholder || '';
    if (confirm) confirm.placeholder = copy.confirmPlaceholder || '';
    if (adminUsername) adminUsername.placeholder = copy.adminUsernamePlaceholder || '';
    if (adminPassword) adminPassword.placeholder = copy.adminPasswordPlaceholder || '';
    var back = document.getElementById('register-back-btn');
    if (back) { back.setAttribute('aria-label', copy.backLabel || ''); back.title = copy.backLabel || ''; }
  }
  function responseError(payload, fallback) {
    var key = payload && payload.error_key;
    var localized = key && window.EchoGalleryI18n.text(key);
    return localized || (payload && payload.error) || fallback || '';
  }
  var copyReady = window.EchoGalleryI18n.load()
    .then(function(catalog) { applyCopy(catalog && catalog.pages && catalog.pages.register); syncRole(); })
    .catch(function() { applyCopy({}); });
  function syncRole() {
    var isAdmin = roleEl && roleEl.value === 'admin';
    if (adminFieldsEl) adminFieldsEl.hidden = !isAdmin;
    btn.textContent = isAdmin ? (copy.adminSubmit || '') : (copy.visitorSubmit || '');
  }
  syncRole();
  if (roleEl) roleEl.addEventListener('change', syncRole);
  async function doRegister() {
    await copyReady;
    var u = document.getElementById('username').value.trim();
    var p = document.getElementById('password').value;
    var c = document.getElementById('confirm-password').value;
    var role = roleEl && roleEl.value === 'admin' ? 'admin' : 'visitor';
    var adminUsername = document.getElementById('admin-username') ? document.getElementById('admin-username').value.trim() : '';
    var adminPassword = document.getElementById('admin-password') ? document.getElementById('admin-password').value : '';
    if (!u || !p || !c) { errEl.textContent = copy.errors && copy.errors.missingFields || ''; return; }
    if (p !== c) { errEl.textContent = copy.errors && copy.errors.passwordMismatch || ''; return; }
    if (role === 'admin' && (!adminUsername || !adminPassword)) { errEl.textContent = copy.errors && copy.errors.missingAdminFields || ''; return; }
    btn.disabled = true; btn.textContent = (role === 'admin' ? copy.adminSubmit : copy.visitorSubmit || '') + (copy.submittingSuffix || '');
    try {
      var r = await fetch('/api/auth/register', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({username: u, password: p, confirm_password: c, role: role, admin_username: adminUsername, admin_password: adminPassword})
      });
      if (r.ok) { location.href = '/'; return; }
      var d = await r.json();
      errEl.textContent = responseError(d, copy.errors && copy.errors.failed);
    } catch(e) { errEl.textContent = copy.errors && copy.errors.network || ''; }
    btn.disabled = false; syncRole();
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
