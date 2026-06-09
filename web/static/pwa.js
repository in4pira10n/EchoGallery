(function () {
  'use strict';

  var STORAGE_KEY = 'echogallery_pwa_settings_v1';
  var RECENT_GALLERIES_KEY = 'echogallery_recent_galleries_v1';
  var MAX_RECENT_GALLERIES = 12;
  var DEFAULTS = {
    name: 'EchoGallery',
    iconSource: 'favicon',
    themeMode: 'accent',
    themeColor: '#2d6a5f',
    startPage: '/',
    cacheStrategy: 'static-only'
  };

  function normalizeHexColor(value) {
    var text = String(value || '').trim();
    if (/^#[0-9a-fA-F]{6}$/.test(text)) return text.toLowerCase();
    if (/^#[0-9a-fA-F]{3}$/.test(text)) {
      return '#' + text.slice(1).split('').map(function (ch) { return ch + ch; }).join('').toLowerCase();
    }
    return DEFAULTS.themeColor;
  }

  function loadSettings() {
    try {
      var parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      if (!parsed || typeof parsed !== 'object') parsed = {};
      return {
        name: String(parsed.name || '').trim() || DEFAULTS.name,
        iconSource: parsed.iconSource === 'svg' ? 'png' : (['favicon', 'library', 'png'].indexOf(parsed.iconSource) >= 0 ? parsed.iconSource : DEFAULTS.iconSource),
        themeMode: ['accent', 'fixed'].indexOf(parsed.themeMode) >= 0 ? parsed.themeMode : DEFAULTS.themeMode,
        themeColor: normalizeHexColor(parsed.themeColor),
        startPage: ['/', '/login', 'auto'].indexOf(parsed.startPage) >= 0 ? parsed.startPage : DEFAULTS.startPage,
        cacheStrategy: ['static-only', 'app-shell', 'disabled'].indexOf(parsed.cacheStrategy) >= 0 ? parsed.cacheStrategy : DEFAULTS.cacheStrategy
      };
    } catch (_) {
      return DEFAULTS;
    }
  }

  function bodyIsGalleryChooser() {
    return !!(document.body && document.body.dataset && document.body.dataset.galleryChooser === '1');
  }

  function normalizeGalleryURL(value) {
    var text = String(value || '').trim();
    if (!text) return '';
    if (!/^[a-z]+:\/\//i.test(text)) text = 'http://' + text;
    try {
      var url = new URL(text);
      if (url.protocol !== 'http:' && url.protocol !== 'https:') return '';
      url.hash = '';
      url.search = '';
      url.pathname = '/';
      return url.toString();
    } catch (_) {
      return '';
    }
  }

  function safeParseJSON(text, fallback) {
    try {
      var parsed = JSON.parse(text);
      return parsed == null ? fallback : parsed;
    } catch (_) {
      return fallback;
    }
  }

  function loadRecentGalleries() {
    var raw = '[]';
    try {
      raw = localStorage.getItem(RECENT_GALLERIES_KEY) || '[]';
    } catch (_) {}
    var list = safeParseJSON(raw, []);
    if (!Array.isArray(list)) return [];
    return list.map(function (item) {
      if (!item || typeof item !== 'object') return null;
      var url = normalizeGalleryURL(item.url || item.origin || '');
      if (!url) return null;
      return {
        url: url,
        origin: safeOrigin(url),
        displayName: String(item.displayName || item.title || item.name || 'EchoGallery').trim() || 'EchoGallery',
        title: String(item.title || item.displayName || 'EchoGallery').trim() || 'EchoGallery',
        icon: typeof item.icon === 'string' ? item.icon : '',
        themeColor: normalizeHexColor(item.themeColor),
        lastSeen: Math.max(0, Number(item.lastSeen) || 0)
      };
    }).filter(Boolean).sort(function (a, b) {
      return (b.lastSeen || 0) - (a.lastSeen || 0);
    }).slice(0, MAX_RECENT_GALLERIES);
  }

  function saveRecentGalleries(list) {
    try {
      localStorage.setItem(RECENT_GALLERIES_KEY, JSON.stringify((Array.isArray(list) ? list : []).slice(0, MAX_RECENT_GALLERIES)));
    } catch (_) {}
  }

  function safeOrigin(url) {
    try {
      return new URL(url).origin;
    } catch (_) {
      return '';
    }
  }

  function currentDocumentIcon() {
    var link = document.querySelector('link[rel="icon"]') || document.querySelector('link[rel="apple-touch-icon"]');
    return link && link.href ? link.href : '/static/pwa-icon.png';
  }

  function buildRecentGalleryRecord(extra) {
    extra = extra && typeof extra === 'object' ? extra : {};
    var url = normalizeGalleryURL(extra.url || location.origin || location.href);
    if (!url) return null;
    var title = String(extra.title || document.title || 'EchoGallery').trim() || 'EchoGallery';
    var displayName = String(extra.displayName || extra.name || title.replace(/\s*-\s*EchoGallery\s*$/i, '') || 'EchoGallery').trim() || 'EchoGallery';
    return {
      url: url,
      origin: safeOrigin(url),
      displayName: displayName,
      title: title,
      icon: String(extra.icon || currentDocumentIcon() || '/static/pwa-icon.png'),
      themeColor: normalizeHexColor(extra.themeColor || currentAccentColor(loadSettings())),
      lastSeen: Date.now()
    };
  }

  function recordCurrentGallery(extra) {
    if (bodyIsGalleryChooser()) return null;
    var record = buildRecentGalleryRecord(extra);
    if (!record || !record.url) return null;
    var list = loadRecentGalleries().filter(function (item) { return item.url !== record.url; });
    list.unshift(record);
    saveRecentGalleries(list);
    return record;
  }

  function removeRecentGallery(url) {
    var normalized = normalizeGalleryURL(url);
    if (!normalized) return;
    saveRecentGalleries(loadRecentGalleries().filter(function (item) { return item.url !== normalized; }));
  }

  function ensureLink(rel, href, attrs) {
    var link = document.querySelector('link[rel="' + rel + '"]');
    if (!link) {
      link = document.createElement('link');
      link.rel = rel;
      document.head.appendChild(link);
    }
    link.href = href;
    Object.keys(attrs || {}).forEach(function (key) {
      link.setAttribute(key, attrs[key]);
    });
    return link;
  }

  function currentAccentColor(settings) {
    if (settings.themeMode === 'fixed') return settings.themeColor;
    var style = getComputedStyle(document.documentElement);
    return normalizeHexColor(style.getPropertyValue('--accent') || settings.themeColor);
  }

  function applyThemeColor(color) {
    var meta = document.querySelector('meta[name="theme-color"]');
    if (!meta) {
      meta = document.createElement('meta');
      meta.name = 'theme-color';
      document.head.appendChild(meta);
    }
    meta.content = document.documentElement.classList.contains('lightbox-open') ? '#000000' : color;
  }

  function manifestIcon(settings) {
    if (settings.iconSource === 'png') return '/static/pwa-icon.png';
    var favicon = document.querySelector('link[rel="icon"]');
    if (favicon && favicon.href && !favicon.href.startsWith('data:')) return favicon.href;
    return '/static/pwa-icon.png';
  }

  var dynamicManifestURL = '';

  function installDynamicManifest(settings) {
    var color = currentAccentColor(settings);
    applyThemeColor(color);
    var icon = manifestIcon(settings);
    var startURL = settings.startPage === 'auto' ? '/' : settings.startPage;
    var manifest = {
      id: '/',
      name: settings.name,
      short_name: settings.name,
      description: 'A local-first gallery for browsing, organizing, and remembering your media library.',
      start_url: startURL,
      scope: '/',
      display: 'standalone',
      display_override: ['window-controls-overlay', 'standalone', 'browser'],
      background_color: '#08151b',
      theme_color: color,
      orientation: 'any',
      categories: ['photo', 'productivity', 'utilities'],
      icons: [{ src: icon, sizes: icon.endsWith('.svg') ? 'any' : '256x256', type: icon.endsWith('.svg') ? 'image/svg+xml' : 'image/png', purpose: 'any maskable' }]
    };
    var blob = new Blob([JSON.stringify(manifest)], { type: 'application/manifest+json' });
    var url = URL.createObjectURL(blob);
    if (dynamicManifestURL) URL.revokeObjectURL(dynamicManifestURL);
    dynamicManifestURL = url;
    ensureLink('manifest', url);
  }

  function registerServiceWorker(settings) {
    if (!('serviceWorker' in navigator)) return;
    if (location.protocol !== 'https:' && location.hostname !== 'localhost' && location.hostname !== '127.0.0.1' && location.hostname !== '::1') return;
    window.addEventListener('load', function () {
      navigator.serviceWorker.register('/sw.js', { scope: '/' }).then(function (registration) {
        if (registration && registration.active) {
          registration.active.postMessage({ type: 'EG_PWA_SETTINGS', cacheStrategy: settings.cacheStrategy });
        }
      }).catch(function () {});
    });
  }

  var settings = loadSettings();
  window.EchoGalleryPWA = {
    loadRecentGalleries: loadRecentGalleries,
    recordCurrentGallery: recordCurrentGallery,
    removeRecentGallery: removeRecentGallery,
    normalizeGalleryURL: normalizeGalleryURL
  };
  ensureLink('manifest', '/manifest.webmanifest');
  ensureLink('icon', '/static/pwa-icon.png', { type: 'image/png' });
  ensureLink('apple-touch-icon', '/static/pwa-icon.png');
  installDynamicManifest(settings);
  registerServiceWorker(settings);
  window.addEventListener('load', function () {
    window.setTimeout(function () {
      recordCurrentGallery();
    }, 120);
  });
  window.addEventListener('eg:pwa-settings-change', function () {
    settings = loadSettings();
    installDynamicManifest(settings);
    recordCurrentGallery({ themeColor: currentAccentColor(settings) });
    if (navigator.serviceWorker && navigator.serviceWorker.controller) {
      navigator.serviceWorker.controller.postMessage({ type: 'EG_PWA_SETTINGS', cacheStrategy: settings.cacheStrategy });
    }
  });
  window.addEventListener('eg:pwa-gallery-meta', function (event) {
    recordCurrentGallery(event && event.detail);
  });
  window.setTimeout(function () {
    settings = loadSettings();
    installDynamicManifest(settings);
    recordCurrentGallery();
  }, 1600);
  window.addEventListener('pagehide', function () {
    if (dynamicManifestURL) URL.revokeObjectURL(dynamicManifestURL);
  }, { once: true });
})();
