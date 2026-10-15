(function (root) {
  'use strict';

  var DEFAULT_LOCALE = 'zh-CN';
  var LOCALE_STORAGE_KEY = 'echogallery_locale_v1';
  var activeLocale = DEFAULT_LOCALE;
  var activeCatalog = {};
  var availableLocales = [];
  var availableLocalesPromise = null;

  function normalizeLocale(value) {
    var locale = String(value || '').trim().replace(/_/g, '-');
    if (!locale || !/^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$/.test(locale)) return '';
    if (/^zh(?:-|$)/i.test(locale)) return 'zh-CN';
    if (/^en(?:-|$)/i.test(locale)) return 'en-US';
    var parts = locale.split('-');
    return parts[0].toLowerCase() + (parts.length > 1 ? '-' + parts.slice(1).join('-') : '');
  }

  function readSavedLocale() {
    try {
      return normalizeLocale(root.localStorage.getItem(LOCALE_STORAGE_KEY));
    } catch (_) {
      return '';
    }
  }

  function preferredLocale() {
    var saved = readSavedLocale();
    if (saved) return saved;
    return browserLocale(availableLocales);
  }

  function loadAvailableLocales() {
    if (availableLocalesPromise) return availableLocalesPromise;
    availableLocalesPromise = root.fetch('/static/strings/locales.json', { cache: 'no-cache' })
      .then(function (response) {
        if (!response || !response.ok) return null;
        return response.json();
      })
      .then(function (manifest) {
        var locales = manifest && Array.isArray(manifest.locales) ? manifest.locales : [];
        var normalized = locales.map(function (item) {
          var code = normalizeLocale(item && item.code);
          var label = String(item && item.label || code || '').trim();
          return code && label ? { code: code, label: label } : null;
        }).filter(Boolean);
        if (!normalized.some(function (item) { return item.code === DEFAULT_LOCALE; })) {
          normalized.unshift({ code: DEFAULT_LOCALE, label: DEFAULT_LOCALE });
        }
        return normalized;
      })
      .catch(function () { return null; });
    return availableLocalesPromise;
  }

  function matchAvailableLocale(value, locales) {
    var candidate = normalizeLocale(value);
    if (!candidate) return '';
    var exact = locales.find(function (item) { return item.code.toLowerCase() === candidate.toLowerCase(); });
    if (exact) return exact.code;
    var language = candidate.split('-')[0].toLowerCase();
    var regional = locales.find(function (item) { return item.code.split('-')[0].toLowerCase() === language; });
    return regional ? regional.code : '';
  }

  function browserLocale(locales) {
    var candidates = root.navigator && (root.navigator.languages || [root.navigator.language]);
    for (var index = 0; candidates && index < candidates.length; index += 1) {
      var match = matchAvailableLocale(candidates[index], locales);
      if (match) return match;
    }
    return DEFAULT_LOCALE;
  }

  function merge(target, source) {
    if (!source || typeof source !== 'object') return target;
    Object.keys(source).forEach(function (key) {
      var value = source[key];
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        if (!target[key] || typeof target[key] !== 'object' || Array.isArray(target[key])) target[key] = {};
        merge(target[key], value);
      } else {
        target[key] = value;
      }
    });
    return target;
  }

  function fetchCatalog(locale) {
    return root.fetch('/static/strings/' + encodeURIComponent(locale) + '.json', { cache: 'no-cache' })
      .then(function (response) {
        if (!response || !response.ok) return null;
        return response.json();
      })
      .catch(function () { return null; });
  }

  function load(locale) {
    return loadAvailableLocales().then(function (locales) {
      if (locales) availableLocales = locales;
      var saved = readSavedLocale();
      var requested = normalizeLocale(locale) || (saved ? saved : browserLocale(availableLocales));
      var fallbackPromise = requested === DEFAULT_LOCALE ? Promise.resolve(null) : fetchCatalog(DEFAULT_LOCALE);
      var selectedPromise = fetchCatalog(requested);
      return Promise.all([fallbackPromise, selectedPromise]).then(function (catalogs) {
        var fallback = catalogs[0] || (requested === DEFAULT_LOCALE ? catalogs[1] : null) || {};
        var selected = catalogs[1];
        var actualLocale = selected ? requested : (fallback.locale || DEFAULT_LOCALE);
        activeCatalog = merge(merge({}, fallback), selected || {});
        activeLocale = actualLocale;
        activeCatalog.locale = actualLocale;
        activeCatalog.availableLocales = availableLocales.slice();
        root.__GALLERY_LOCALE__ = actualLocale;
        if (root.document && root.document.documentElement) root.document.documentElement.lang = actualLocale;
        return activeCatalog;
      });
    });
  }

  function setLocale(locale) {
    var normalized = normalizeLocale(locale);
    if (!normalized) return false;
    try {
      root.localStorage.setItem(LOCALE_STORAGE_KEY, normalized);
    } catch (_) {
      return false;
    }
    if (root.location && typeof root.location.reload === 'function') root.location.reload();
    return true;
  }

  function getLocale() {
    return activeLocale || preferredLocale();
  }

  function getCatalog() {
    return activeCatalog;
  }

  function getAvailableLocales() {
    return availableLocales.slice();
  }

  function text(path, params) {
    var value = String(path || '').split('.').reduce(function (current, key) {
      return current && typeof current === 'object' && key in current ? current[key] : undefined;
    }, activeCatalog);
    if (typeof value !== 'string') return '';
    return value.replace(/\{(\w+)\}/g, function (match, key) {
      return params && Object.prototype.hasOwnProperty.call(params, key) ? String(params[key]) : match;
    });
  }

  root.EchoGalleryI18n = {
    defaultLocale: DEFAULT_LOCALE,
    localeStorageKey: LOCALE_STORAGE_KEY,
    getLocale: getLocale,
    getCatalog: getCatalog,
    getAvailableLocales: getAvailableLocales,
    load: load,
    normalizeLocale: normalizeLocale,
    preferredLocale: preferredLocale,
    setLocale: setLocale,
    text: text,
  };

  if (root.document) {
    root.document.addEventListener('change', function (event) {
      var picker = event.target && event.target.closest && event.target.closest('[data-eg-locale-picker]');
      if (picker) setLocale(picker.value);
    });
  }
})(window);
