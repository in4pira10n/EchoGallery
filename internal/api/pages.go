package api

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const appPageFallbackHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>EchoGallery</title>
  <link rel="stylesheet" href="/pages/app.css">
</head>
<body>
  <div id="app"></div>
  <script src="/static/app.js"></script>
</body>
</html>`

const loginPageFallbackHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>登录 - EchoGallery</title>
  <link rel="stylesheet" href="/pages/common.css">
  <link rel="stylesheet" href="/pages/login.css">
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
  async function doLogin() {
    var u = document.getElementById('username').value.trim();
    var p = document.getElementById('password').value;
    if (!u || !p) { errEl.textContent = '请输入用户名和密码'; return; }
    btn.disabled = true; btn.textContent = '登录中…';
    try {
      var r = await fetch('/api/auth/login', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({username: u, password: p})
      });
      var d = await r.json().catch(function () { return {}; });
      if (r.ok) {
        errEl.style.color = 'var(--accent)';
        errEl.textContent = d.message || '';
        var target = d.redirect || '/';
        var delay = Math.max(0, Number(d.delay_ms) || 0);
        setTimeout(function () { location.href = target; }, delay);
        return;
      }
      errEl.textContent = d.error || '登录失败';
    } catch(e) { errEl.textContent = '网络错误'; }
    btn.disabled = false; btn.textContent = '登录';
  }
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
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>注册 - EchoGallery</title>
  <link rel="stylesheet" href="/pages/common.css">
  <link rel="stylesheet" href="/pages/login.css">
</head>
<body>
<div class="login-wrap">
  <div class="login-card register-card">
    <section class="login-form-panel" aria-label="注册">
      <div class="login-form-head">
        <span>新用户</span>
        <strong>注册 EchoGallery</strong>
      </div>
      <div class="form-group">
        <label class="form-label" for="username">用户名</label>
        <input class="input" id="username" type="text" autocomplete="username" placeholder="请输入用户名">
      </div>
      <div class="form-group">
        <label class="form-label" for="password">密码</label>
        <input class="input" id="password" type="password" autocomplete="new-password" placeholder="至少 6 位">
      </div>
      <div class="form-group">
        <label class="form-label" for="confirm-password">确认密码</label>
        <input class="input" id="confirm-password" type="password" autocomplete="new-password" placeholder="再次输入密码">
      </div>
      <button class="btn btn-primary login-submit" id="register-btn">创建账号</button>
      <a class="login-secondary-link" href="/login">已有账号？返回登录</a>
      <div class="login-error" id="register-error"></div>
    </section>
  </div>
</div>
<script>
(function() {
  var t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  document.documentElement.dataset.theme = t;
  var btn = document.getElementById('register-btn');
  var errEl = document.getElementById('register-error');
  async function doRegister() {
    var u = document.getElementById('username').value.trim();
    var p = document.getElementById('password').value;
    var c = document.getElementById('confirm-password').value;
    if (!u || !p || !c) { errEl.textContent = '请完整填写注册信息'; return; }
    if (p !== c) { errEl.textContent = '两次输入的密码不一致'; return; }
    btn.disabled = true; btn.textContent = '创建中…';
    try {
      var r = await fetch('/api/auth/register', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({username: u, password: p, confirm_password: c})
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
		return fallback
	}
	html := string(content)
	for _, marker := range required {
		if !strings.Contains(html, marker) {
			log.Printf("警告: 页面资源 %s 缺少关键标记 %q，使用内置兜底", relPath, marker)
			return fallback
		}
	}
	return html
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

func handleAppPage(staticFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := pageHTML(staticFS, "app.html", appPageFallbackHTML, []string{`id="app"`, `/static/app.js`})
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}

func handleLoginPage(staticFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := pageHTML(staticFS, "login.html", loginPageFallbackHTML, []string{`id="login-btn"`, `/api/auth/login`})
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}

func handleRegisterPage(staticFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		html := pageHTML(staticFS, "register.html", registerPageFallbackHTML, []string{`id="register-btn"`, `/api/auth/register`})
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	}
}
