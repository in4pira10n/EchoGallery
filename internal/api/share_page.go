package api

import (
	"io/fs"

	"github.com/gin-gonic/gin"
)

const sharePageHTML = `<!doctype html>
<html lang="zh-CN" data-theme="">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <title id="share-page-title"></title>
  <link rel="stylesheet" href="/pages/app.css?v={{ASSET_VERSION}}">
  <script src="/static/i18n.js?v={{ASSET_VERSION}}"></script>
</head>
<body>
<script>
  (function(){
    var t = localStorage.getItem('theme') || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
    document.documentElement.dataset.theme = t;
  })();
</script>
<div id="share-root"></div>
<script src="/static/lightbox.js?v={{ASSET_VERSION}}"></script>
<script>
(function() {
  var token = location.pathname.split('/').pop();
  var root = document.getElementById('share-root');
  var bootstrap = function(icons) {
    root.innerHTML = window.EchoGalleryLightbox.renderShell({
      mode: 'share',
      icons: icons || {}
    });
    window.EchoGalleryLightbox.createShareController({ token: token }).init();
  };
  Promise.all([window.EchoGalleryLightbox.loadIcons(), window.EchoGalleryLightbox.loadCopy()])
    .then(function(results) {
      var catalog = results[1] || {};
      if (catalog.locale) document.documentElement.lang = catalog.locale;
      var title = catalog.pages && catalog.pages.share && catalog.pages.share.pageTitle || window.__GALLERY_NAME__ || 'EchoGallery';
      document.title = String(title).replace(/EchoGallery/g, window.__GALLERY_NAME__ || 'EchoGallery');
      bootstrap(results[0]);
    })
    .catch(function() { bootstrap({}); });
})();
</script>
</body>
</html>`

func handleSharePage(staticFS fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(applyAssetVersionTemplate(staticFS, sharePageHTML)))
	}
}
