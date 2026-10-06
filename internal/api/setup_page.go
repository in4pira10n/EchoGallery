package api

const setupPageFallbackHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta name="theme-color" content="#2d6a5f">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-title" content="EchoGallery">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="mobile-web-app-capable" content="yes">
  <title>EchoGallery 设置</title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/common.css?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/setup.css?v={{ASSET_VERSION}}">
  <script src="/static/pwa.js?v={{ASSET_VERSION}}" defer></script>
</head>
<body>
<script>
  (function(){
    var t = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    document.documentElement.dataset.theme = t;
    window.__SETUP__ = __SETUP_PAYLOAD__;
  })();
</script>
<div class="login-wrap">
  <div class="login-card setup-card" id="setup-root"></div>
</div>
<script>
(function () {
  var state = window.__SETUP__ || {};
  var root = document.getElementById('setup-root');
  root.innerHTML = '<div class="login-logo">EchoGallery</div>' +
    '<div class="setup-title">网页初始化</div>' +
    '<p class="setup-desc">页面资源缺失，已进入安全兜底模式。请检查 web/pages/setup.html。</p>' +
    '<div class="login-error" id="setup-error"></div>';
  if (state.message) document.getElementById('setup-error').textContent = state.message;
})();
</script>
</body>
</html>`
