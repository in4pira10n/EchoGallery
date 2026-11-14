const EG_ASSET_VERSION = '{{ASSET_VERSION}}';
const EG_CACHE_VERSION = `eg-pwa-${EG_ASSET_VERSION}`;
const EG_STATIC_CACHE = `${EG_CACHE_VERSION}-static`;
let cacheStrategy = 'static-only';

const STATIC_ASSETS = [
  '/',
  '/login',
  '/register',
  `/manifest.webmanifest?v=${EG_ASSET_VERSION}`,
  `/pages/common.css?v=${EG_ASSET_VERSION}`,
  `/pages/login.css?v=${EG_ASSET_VERSION}`,
  `/pages/setup.css?v=${EG_ASSET_VERSION}`,
  `/pages/app.css?v=${EG_ASSET_VERSION}`,
  `/static/i18n.js?v=${EG_ASSET_VERSION}`,
  `/static/media-loader.js?v=${EG_ASSET_VERSION}`,
  `/static/photo-data-window.js?v=${EG_ASSET_VERSION}`,
  `/static/photo-wall.js?v=${EG_ASSET_VERSION}`,
  `/static/page-effects.js?v=${EG_ASSET_VERSION}`,
  `/static/app.js?v=${EG_ASSET_VERSION}`,
  `/static/pwa.js?v=${EG_ASSET_VERSION}`,
  `/static/pwa-icon.png?v=${EG_ASSET_VERSION}`,
  `/static/pwa-icon-512.png?v=${EG_ASSET_VERSION}`,
  `/static/pwa-icon-192.png?v=${EG_ASSET_VERSION}`,
  `/static/strings/zh-CN.json?v=${EG_ASSET_VERSION}`,
  `/static/strings/en-US.json?v=${EG_ASSET_VERSION}`,
  `/static/strings/locales.json?v=${EG_ASSET_VERSION}`
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(EG_STATIC_CACHE)
      .then(cache => cache.addAll(STATIC_ASSETS.map(url => new Request(url, { cache: 'reload' }))))
      .catch(() => undefined)
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(keys.filter(key => key.startsWith('eg-pwa-') && key !== EG_STATIC_CACHE).map(key => caches.delete(key))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('message', event => {
  const data = event.data || {};
  if (data.type === 'EG_PWA_SETTINGS') {
    cacheStrategy = data.cacheStrategy || 'static-only';
  }
});

function shouldBypass(request) {
  const url = new URL(request.url);
  if (request.method !== 'GET') return true;
  if (url.origin !== location.origin) return true;
  if (url.pathname.startsWith('/api/')) return true;
  if (url.pathname.startsWith('/media/')) return true;
  if (url.pathname.includes('/thumbnails/')) return true;
  return false;
}

function networkFirst(request, fallbackToShell = true) {
  return fetch(request)
    .then(response => {
      const copy = response.clone();
      if (response.ok) {
        caches.open(EG_STATIC_CACHE).then(cache => cache.put(request, copy)).catch(() => undefined);
      }
      return response;
    })
    .catch(error => caches.match(request).then(cached => {
      if (cached) return cached;
      if (fallbackToShell) return caches.match('/');
      throw error;
    }));
}

function cacheFirst(request) {
  return caches.match(request).then(cached => {
    if (cached) return cached;
    return fetch(request).then(response => {
      const copy = response.clone();
      if (response.ok) {
        caches.open(EG_STATIC_CACHE).then(cache => cache.put(request, copy)).catch(() => undefined);
      }
      return response;
    });
  });
}

self.addEventListener('fetch', event => {
  const request = event.request;
  if (shouldBypass(request) || cacheStrategy === 'disabled') return;
  const url = new URL(request.url);
  if (request.mode === 'navigate') {
    event.respondWith(cacheStrategy === 'app-shell' ? networkFirst(request) : fetch(request).catch(() => caches.match('/')));
    return;
  }
  // Catalogs use stable URLs, while application scripts are versioned.
  // Revalidate translations so new menu labels do not read an old catalog.
  if (url.pathname.startsWith('/static/strings/')) {
    event.respondWith(networkFirst(request, false));
    return;
  }
  if (url.pathname.startsWith('/pages/') || url.pathname.startsWith('/static/') || url.pathname === '/manifest.webmanifest') {
    event.respondWith(cacheFirst(request));
  }
});
