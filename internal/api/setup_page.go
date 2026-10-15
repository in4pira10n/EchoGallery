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
  <title id="setup-fallback-page-title"></title>
  <link rel="manifest" href="/manifest.webmanifest?v={{ASSET_VERSION}}">
  <link rel="icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}" type="image/png">
  <link rel="apple-touch-icon" href="/static/pwa-icon.png?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/common.css?v={{ASSET_VERSION}}">
  <link rel="stylesheet" href="/pages/setup.css?v={{ASSET_VERSION}}">
  <script src="/static/i18n.js?v={{ASSET_VERSION}}"></script>
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
  root.innerHTML = '<div class="login-logo"></div>' +
    '<div class="setup-title" id="setup-fallback-title"></div>' +
    '<p class="setup-desc" id="setup-fallback-message"></p>' +
    '<div class="login-error" id="setup-error"></div>';
  if (state.app_name) root.querySelector('.login-logo').textContent = state.app_name;
  if (state.message) document.getElementById('setup-error').textContent = state.message;
  window.EchoGalleryI18n.load()
    .then(function(catalog) {
      var copy = catalog && catalog.pages && catalog.pages.setup && catalog.pages.setup.assetFallback;
      if (!copy) return;
      var name = state.app_name || 'EchoGallery';
      document.title = String(copy.pageTitle || '').replace('{name}', name);
      document.getElementById('setup-fallback-title').textContent = copy.title || '';
      document.getElementById('setup-fallback-message').textContent = copy.message || '';
    })
    .catch(function() {});
})();
</script>
</body>
</html>`
