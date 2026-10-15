/* ===== EchoGallery 前端 ===== */
'use strict';

// ── 工具函数 ──────────────────────────────────────────
const $ = (sel, ctx = document) => ctx.querySelector(sel);
const $$ = (sel, ctx = document) => [...ctx.querySelectorAll(sel)];
const el = (tag, cls, html = '') => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (html) e.innerHTML = html;
  return e;
};
const pageSessionStorageKey = 'echogallery_page_session_id_v1';
const pageSessionTransferKey = 'echogallery_page_session_transfer_v1';
const pageSessionQueryKey = 'eg_page_session';
let pageSessionHeartbeatTimer = null;
let pageSessionRedirectPending = false;
let pageSessionReleaseRequested = false;

function generatePageSessionID() {
  if (window.crypto && typeof window.crypto.randomUUID === 'function') {
    return window.crypto.randomUUID();
  }
  return `eg-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

function ensurePageSessionID() {
  try {
    let sessionID = sessionStorage.getItem(pageSessionStorageKey) || '';
    if (!sessionID) {
      sessionID = generatePageSessionID();
      sessionStorage.setItem(pageSessionStorageKey, sessionID);
    }
    return sessionID;
  } catch (_) {
    return generatePageSessionID();
  }
}

function resetPageSessionID() {
  const sessionID = generatePageSessionID();
  try {
    sessionStorage.setItem(pageSessionStorageKey, sessionID);
  } catch (_) {}
  return sessionID;
}

function consumeLoginTransferPageSessionID() {
  try {
    const current = sessionStorage.getItem(pageSessionStorageKey) || '';
    const transfer = sessionStorage.getItem(pageSessionTransferKey) || '';
    if (current && transfer && current === transfer) {
      sessionStorage.removeItem(pageSessionTransferKey);
      return current;
    }
    if (transfer) sessionStorage.removeItem(pageSessionTransferKey);
  } catch (_) {}
  return '';
}

function navigationType() {
  try {
    const entry = performance.getEntriesByType && performance.getEntriesByType('navigation')[0];
    if (entry && entry.type) return String(entry.type);
  } catch (_) {}
  try {
    if (performance && performance.navigation) {
      switch (performance.navigation.type) {
        case performance.navigation.TYPE_RELOAD:
          return 'reload';
        case performance.navigation.TYPE_BACK_FORWARD:
          return 'back_forward';
        default:
          return 'navigate';
      }
    }
  } catch (_) {}
  return 'navigate';
}

function buildPageSessionHeaders(headers) {
  const next = new Headers(headers || {});
  const sessionID = ensurePageSessionID();
  if (sessionID) next.set('X-EG-Page-Session', sessionID);
  return next;
}

function withPageSessionRequest(options = {}) {
  return {
    credentials: 'same-origin',
    ...options,
    headers: buildPageSessionHeaders(options.headers),
  };
}

function withPageSessionURL(url) {
  const sessionID = ensurePageSessionID();
  if (!sessionID || !url) return url;
  try {
    const resolved = new URL(url, location.origin);
    resolved.searchParams.set(pageSessionQueryKey, sessionID);
    if (resolved.origin === location.origin) {
      return `${resolved.pathname}${resolved.search}${resolved.hash}`;
    }
    return resolved.toString();
  } catch (_) {
    const joiner = String(url).includes('?') ? '&' : '?';
    return `${url}${joiner}${pageSessionQueryKey}=${encodeURIComponent(sessionID)}`;
  }
}

function appendURLParam(url, key, value) {
  try {
    const resolved = new URL(url, location.origin);
    resolved.searchParams.set(key, value);
    if (resolved.origin === location.origin) {
      return `${resolved.pathname}${resolved.search}${resolved.hash}`;
    }
    return resolved.toString();
  } catch (_) {
    const joiner = String(url).includes('?') ? '&' : '?';
    return `${url}${joiner}${encodeURIComponent(key)}=${encodeURIComponent(value)}`;
  }
}

async function fetchWithPageSession(url, options = {}) {
  return fetch(url, withPageSessionRequest(options));
}

function authRedirectReason(reason, status) {
  switch (String(reason || '').trim()) {
    case 'duplicate_page_session':
    case 'user_already_online':
      return 'duplicate';
    case 'no_accessible_library':
      return 'no-library';
    case 'session_missing':
    case 'session_expired':
      return 'expired';
    default:
      return status === 401 ? 'expired' : '';
  }
}

function redirectToLoginForAuthFailure(reason, status) {
  if (pageSessionRedirectPending) return;
  pageSessionRedirectPending = true;
  stopPageSessionHeartbeat();
  const mapped = authRedirectReason(reason, status);
  const next = mapped ? `/login?reason=${encodeURIComponent(mapped)}` : '/login';
  location.href = next;
}

let libraryFallbackPending = false;
async function fallbackFromOccupiedLibrary(libraryID) {
  if (libraryFallbackPending) return;
  libraryFallbackPending = true;
  try {
    const response = await fetch(withPageSessionURL('/api/auth/library-fallback'), withPageSessionRequest({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ library_id: libraryID || '' }),
    }));
    if (response.ok) {
      location.reload();
      return;
    }
    if (response.status === 403) {
      redirectToLoginForAuthFailure('no_accessible_library', response.status);
    }
  } catch (_) {
  } finally {
    libraryFallbackPending = false;
  }
}

async function buildAPIError(response) {
  const text = await response.text();
  let payload = {};
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch (_) {
      payload = { error: text };
    }
  }
  const error = {
    ...(payload && typeof payload === 'object' ? payload : {}),
    status: response.status,
    error: localizedAPIText(payload, 'error', response.statusText || copyText('app.interface.s_8fdc4112a4')),
    forbidden: response.status === 403,
  };
  const reason = String(error.reason || '').trim();
  if (response.status === 409 && reason === 'library_occupied') {
    error.redirecting = true;
    void fallbackFromOccupiedLibrary(error.library_id || '');
  }
  if (response.status === 401 || (response.status === 403 && reason === 'no_accessible_library')) {
    error.redirecting = true;
    redirectToLoginForAuthFailure(reason, response.status);
    return error;
  }
  if (error.forbidden) {
    error.forbidden = true;
  }
  return error;
}

const api = {
  async get(url, options = {}) {
    const { suppressForbiddenToast, ...fetchOptions } = options || {};
    const r = await fetchWithPageSession(url, fetchOptions);
    return parseAPIResponse(r, { suppressForbiddenToast: !!suppressForbiddenToast });
  },
  async post(url, data, options = {}) {
    const { suppressForbiddenToast, ...fetchOptions } = options || {};
    const r = await fetchWithPageSession(url, {
      method: 'POST',
      headers: buildPageSessionHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(data),
      ...fetchOptions,
    });
    return parseAPIResponse(r, { suppressForbiddenToast: !!suppressForbiddenToast });
  },
  async put(url, data, options = {}) {
    const { suppressForbiddenToast, ...fetchOptions } = options || {};
    const r = await fetchWithPageSession(url, {
      method: 'PUT',
      headers: buildPageSessionHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(data),
      ...fetchOptions,
    });
    return parseAPIResponse(r, { suppressForbiddenToast: !!suppressForbiddenToast });
  },
  async patch(url, data, options = {}) {
    const { suppressForbiddenToast, ...fetchOptions } = options || {};
    const r = await fetchWithPageSession(url, {
      method: 'PATCH',
      headers: buildPageSessionHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(data),
      ...fetchOptions,
    });
    return parseAPIResponse(r, { suppressForbiddenToast: !!suppressForbiddenToast });
  },
  async del(url, data, options = {}) {
    const { suppressForbiddenToast, ...fetchOptions } = options || {};
    const requestOptions = data == null
      ? { method: 'DELETE' }
      : { method: 'DELETE', headers: buildPageSessionHeaders({ 'Content-Type': 'application/json' }), body: JSON.stringify(data) };
    const r = await fetchWithPageSession(url, { ...requestOptions, ...fetchOptions });
    return parseAPIResponse(r, { suppressForbiddenToast: !!suppressForbiddenToast });
  },
  async upload(url, formData, options = {}) {
    const { suppressForbiddenToast, ...fetchOptions } = options || {};
    const r = await fetchWithPageSession(url, { method: 'POST', body: formData, ...fetchOptions });
    return parseAPIResponse(r, { suppressForbiddenToast: !!suppressForbiddenToast });
  },
};

async function parseAPIResponse(response, { suppressForbiddenToast = false } = {}) {
  const payload = await (async () => {
    const text = await response.text();
    if (!text) return {};
    try {
      return JSON.parse(text);
    } catch (_) {
      return { error: text };
    }
  })();
  if (response.ok) return localizeAPIMessage(payload);
  const error = await buildAPIError(new Response(JSON.stringify(payload), {
    status: response.status,
    statusText: response.statusText,
    headers: { 'Content-Type': 'application/json' },
  }));
  if (error.redirecting) throw error;
  if (error.forbidden && !suppressForbiddenToast) {
    showToast(copyText('app.interface.s_8acd52c938'));
    error.forbiddenToastShown = true;
  }
  throw error;
}

function localizeAPIMessage(payload) {
  if (!payload || typeof payload !== 'object') return payload;
  const localized = localizedAPIText(payload, 'message');
  if (localized) payload.message = localized;
  return payload;
}

function localizedAPIText(payload, field, fallback = '') {
  const key = String(payload && payload[`${field}_key`] || '').trim();
  const localized = key && window.EchoGalleryI18n
    ? window.EchoGalleryI18n.text(key, payload[`${field}_params`] || {})
    : '';
  return localized || (payload && payload[field]) || fallback || '';
}

function stopPageSessionHeartbeat() {
  if (pageSessionHeartbeatTimer) {
    clearInterval(pageSessionHeartbeatTimer);
    pageSessionHeartbeatTimer = null;
  }
}

async function pingPageSession() {
  return;
}

function startPageSessionHeartbeat() {
  stopPageSessionHeartbeat();
}

async function releaseCurrentPageSession({ keepalive = false } = {}) {
  if (pageSessionReleaseRequested) return;
  pageSessionReleaseRequested = true;
  try {
    const releaseURL = withPageSessionURL('/api/auth/session/release');
    if (keepalive && navigator.sendBeacon) {
      try {
        if (navigator.sendBeacon(releaseURL, '')) return;
      } catch (_) {
      }
    }
    await fetch(releaseURL, withPageSessionRequest({ method: 'POST', keepalive }));
  } catch (_) {
  } finally {
    pageSessionReleaseRequested = false;
  }
}

function isIOSDevice() {
  const ua = navigator.userAgent || '';
  const platform = navigator.platform || '';
  return /iPad|iPhone|iPod/.test(ua) || (platform === 'MacIntel' && navigator.maxTouchPoints > 1);
}

function isIPadDevice() {
  const ua = navigator.userAgent || '';
  const platform = navigator.platform || '';
  if (/iPad/.test(ua)) return true;
  if (platform !== 'MacIntel' || navigator.maxTouchPoints <= 1) return false;
  const shortSide = Math.min(window.screen.width || 0, window.screen.height || 0);
  return shortSide >= 744;
}

function initDeviceClasses() {
  const root = document.documentElement;
  const isIOS = isIOSDevice();
  const isIPad = isIPadDevice();
  root.classList.toggle('is-ios-device', isIOS);
  root.classList.toggle('is-ipad-device', isIPad);
  root.classList.remove('ios-safe-area-phone', 'ios-home-button-phone', 'ios-ipad-portrait');
  if (!isIOS) return;
  const isIPadPortrait = isIPad && window.matchMedia('(orientation: portrait)').matches;
  const isPhoneSized = Math.max(window.screen.width || 0, window.screen.height || 0) < 1400;
  if (!isPhoneSized && !isIPadPortrait) return;
  const safeArea = measuredSafeAreaInsets();
  const topInset = Math.max(0, Number(safeArea.top) || 0);
  const bottomInset = Math.max(0, Number(safeArea.bottom) || 0);
  const maxTopInset = Math.max(0, Number(safeArea.maxTop) || 0);
  const maxBottomInset = Math.max(0, Number(safeArea.maxBottom) || 0);
  const screenLong = Math.max(window.screen.width || 0, window.screen.height || 0);
  const roundedHeight = Math.round(screenLong);
  const modernIPhoneHeights = new Set([812, 844, 852, 874, 896, 926, 932, 956]);
  const homeButtonIPhoneHeights = new Set([480, 568, 667, 736]);
  const hasKnownModernHeight = modernIPhoneHeights.has(roundedHeight);
  const hasKnownHomeButtonHeight = homeButtonIPhoneHeights.has(roundedHeight);
  const hasMeasuredBottomSafeArea = bottomInset > 0 || maxBottomInset > 0;
  const hasModernSafeArea = hasKnownModernHeight || (!hasKnownHomeButtonHeight && hasMeasuredBottomSafeArea);
  root.dataset.egIosScreenHeight = String(roundedHeight || 0);
  root.dataset.egSafeArea = [
    `top:${Math.round(topInset)}`,
    `bottom:${Math.round(bottomInset)}`,
    `maxTop:${Math.round(maxTopInset)}`,
    `maxBottom:${Math.round(maxBottomInset)}`,
    `ipadPortrait:${isIPadPortrait ? 1 : 0}`,
    `modern:${hasModernSafeArea ? 1 : 0}`,
  ].join(';');
  if (isIPadPortrait) {
    root.classList.add('ios-ipad-portrait');
    return;
  }
  root.classList.add(hasModernSafeArea ? 'ios-safe-area-phone' : 'ios-home-button-phone');
}

let deviceClassRefreshTimer = null;
function refreshDeviceClassesSoon() {
  clearTimeout(deviceClassRefreshTimer);
  deviceClassRefreshTimer = window.setTimeout(() => {
    deviceClassRefreshTimer = null;
    initDeviceClasses();
    applyFixedTouchGridLayout();
    updateGridScaleButtonTitle();
  }, 120);
}

function scheduleInitialDeviceClassRefreshes() {
  [0, 120, 420, 1000].forEach(delay => {
    window.setTimeout(initDeviceClasses, delay);
  });
}

let primaryOrientationSuppressionActive = false;
async function enforcePrimaryScreenOrientation() {
  if (!isTouchLikeDevice()) return;
  const orientation = screen && screen.orientation;
  if (!orientation || typeof orientation.lock !== 'function') return;
  const rawType = String(orientation.type || '');
  const isSecondary = /secondary$/i.test(rawType);
  try {
    if (isSecondary) {
      const primaryType = rawType.startsWith('landscape') ? 'landscape-primary' : 'portrait-primary';
      await orientation.lock(primaryType);
      primaryOrientationSuppressionActive = true;
      return;
    }
    if (primaryOrientationSuppressionActive && typeof orientation.unlock === 'function') {
      orientation.unlock();
    }
  } catch (_) {
  } finally {
    if (!isSecondary) primaryOrientationSuppressionActive = false;
  }
}

function refreshPrimaryScreenOrientationSoon() {
  window.setTimeout(() => {
    void enforcePrimaryScreenOrientation();
  }, 0);
}

const videoPosterPlaceholder = `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 240">
  <rect width="240" height="240" rx="24" fill="#1b2a2f"/>
  <circle cx="120" cy="120" r="54" fill="#2d6a5f"/>
  <polygon points="105,90 105,150 152,120" fill="#f4f1e8"/>
  <text x="120" y="198" font-size="18" text-anchor="middle" fill="#d9e4dd" font-family="sans-serif">VIDEO</text>
</svg>`)} `;
const supportedImageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp', '.tif', '.tiff'];
const supportedVideoExtensions = ['.mp4', '.mov', '.m4v', '.webm', '.mkv', '.avi', '.wmv', '.wma', '.ts', '.mts', '.m2ts', '.mpg', '.mpeg', '.3gp', '.3g2', '.ogv'];
const browserUnsupportedVideoExtensions = ['.wmv', '.wma'];
const memoriesStorageKey = 'echogallery_memories_v1';
const memoriesMigrationStorageKey = 'echogallery_memories_scoped_migrated_v1';
const mediaKindFilterStorageKey = 'echogallery_media_kind_filter_v1';
const albumKindFilterStorageKey = 'echogallery_album_kind_filter_v1';
const timelineOrderStorageKey = 'echogallery_timeline_order_v1';
const albumDetailSortStorageKey = 'echogallery_album_detail_sort_v1';
const videoBookmarksStorageKey = 'echogallery_video_bookmarks_v2';
const legacyVideoBookmarksStorageKey = 'echogallery_video_bookmarks_v1';
const globalVideoVolumeStorageKey = 'echogallery_global_video_volume_v1';
const libraryBatchWorkflowEnabledStorageKey = 'echogallery_library_batch_workflow_enabled_v1';
const libraryBatchWorkflowAggressiveStorageKey = 'echogallery_library_batch_workflow_aggressive_v1';
const libraryBatchWorkflowPhaseStorageKey = 'echogallery_library_batch_workflow_phase_v1';
const settingsPanelExpandedState = new Map();
const pwaSettingsStorageKey = 'echogallery_pwa_settings_v1';
const updateSettingsStorageKey = 'echogallery_update_settings_v1';
const copyCatalog = {};

function mergeCopyCatalog(target, source) {
  if (!source || typeof source !== 'object') return target;
  Object.keys(source).forEach(key => {
    const value = source[key];
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      if (!target[key] || typeof target[key] !== 'object' || Array.isArray(target[key])) {
        target[key] = {};
      }
      mergeCopyCatalog(target[key], value);
      return;
    }
    target[key] = value;
  });
  return target;
}

function interpolateCopy(template, params) {
  if (!params) return template;
  return String(template).replace(/\{(\w+)\}/g, (_, key) => {
    if (Object.prototype.hasOwnProperty.call(params, key)) {
      return String(params[key]);
    }
    return `{${key}}`;
  });
}

function copyText(path, params) {
  const format = value => interpolateCopy(value, params).replace(/EchoGallery/g, () => window.__GALLERY_NAME__ || 'EchoGallery');
  const segments = String(path || '').split('.');
  let current = copyCatalog;
  for (const segment of segments) {
    if (!current || typeof current !== 'object' || !(segment in current)) {
      return '';
    }
    current = current[segment];
  }
  if (typeof current !== 'string') {
    return '';
  }
  return format(current);
}

function copyLocale() {
  return String(copyCatalog.locale || (typeof navigator !== 'undefined' && navigator.language) || 'en');
}

function settingsText(path, params) {
  return copyText(`app.settings.${path}`, params);
}

async function loadCopyCatalog() {
  try {
    const remote = await window.EchoGalleryI18n.load();
    mergeCopyCatalog(copyCatalog, remote);
    state.serverSettings.jwt_secret_masked ||= copyText('app.interface.s_43e203c1de');
    state.libraryBuildStatus.message ||= copyText('app.interface.s_2552c2ae42');
    state.libraryBatchBuildStatus.message ||= copyText('app.interface.s_5ae3799d16');
    state.libraryBatchThumbnailBuildStatus.message ||= copyText('app.interface.s_96a3670f18');
    state.playbackCacheBuildStatus.message ||= copyText('app.interface.s_737edd10ff');
    state.thumbnailBuildStatus.message ||= copyText('app.interface.s_e45f077ef4');
    state.videoThumbnailRefreshStatus.message ||= copyText('app.interface.s_e68748b3f3');
    state.exifBackfillStatus.message ||= copyText('app.interface.s_f53844d2ff');
  } catch (error) {
    console.warn("加载文案资源失败", error);
  }
}

function formatDate(iso) {
  const d = new Date(iso);
  return d.toLocaleDateString(copyLocale(), { year: 'numeric', month: 'long', day: 'numeric' });
}
function formatDateTime(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString(copyLocale(), {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}
function formatMediaDateTime(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString(copyLocale(), {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  });
}
function mediaWeekdayIndex(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return 0;
  const day = d.getDay();
  return day === 0 ? 7 : day;
}
function addPanguSpacing(text) {
  return String(text || '')
    .replace(/([\u4e00-\u9fff])([A-Za-z0-9@#&%+=/.-])/g, '$1 $2')
    .replace(/([A-Za-z0-9@#&%+=/.-])([\u4e00-\u9fff])/g, '$1 $2');
}
function formatSize(bytes) {
  const value = Number(bytes);
  if (!Number.isFinite(value) || value < 0) return '—';
  if (value < 1024) return `${Math.max(0, Math.round(value))} B`;
  if (value < 1048576) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1073741824) return `${(value / 1048576).toFixed(1)} MB`;
  return `${(value / 1073741824).toFixed(1)} GB`;
}
function formatSizeMB(bytes) {
  return formatSize(bytes);
}
function escapeHTML(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}
function autoGrowTextarea(el) {
  if (!el) return;
  const styles = window.getComputedStyle(el);
  const maxHeight = parseFloat(styles.maxHeight);
  el.style.height = 'auto';
  const nextHeight = Math.max(el.scrollHeight, 120);
  if (Number.isFinite(maxHeight) && maxHeight > 0) {
    el.style.height = `${Math.min(nextHeight, maxHeight)}px`;
  } else {
    el.style.height = `${nextHeight}px`;
  }
}
const defaultLibraryAccentPalette = ['#2d6a5f', '#ad7c45', '#c75c3d', '#6b8f71', '#456b9d', '#9466a1', '#cf8a33', '#3d7e90'];
function hashString(value) {
  let hash = 0;
  const input = String(value || '');
  for (let i = 0; i < input.length; i += 1) {
    hash = ((hash << 5) - hash + input.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}
function libraryVisualSeed(library = {}) {
  return String(
    library.id ||
    library.logo_asset ||
    library.logoAsset ||
    library.path ||
    library.storage_path ||
    library.storagePath ||
    'echogallery'
  ).trim().toLowerCase();
}
function libraryLogoAssetURL(library = {}) {
  const asset = String(library.logo_asset || library.logoAsset || '').trim();
  const index = Number.isFinite(Number(library.index)) ? Number(library.index) : Number(library.logo_index);
  if (!asset || !Number.isInteger(index) || index < 0) return '';
  return withPageSessionURL(`/api/settings/libraries/${index}/logo?v=${encodeURIComponent(asset)}`);
}
function sessionScopedURL(url = '') {
  const value = String(url || '').trim();
  if (!value || value.startsWith('blob:') || value.startsWith('data:')) return value;
  return withPageSessionURL(value);
}
function resolveLibraryLogoURL(library = {}) {
  const explicit = library.logo_image_url || library.logoImageUrl || libraryLogoAssetURL(library);
  if (explicit) return sessionScopedURL(explicit);
  const key = libraryVisualSeed(library);
  return sessionScopedURL(state.libraryRandomLogos && state.libraryRandomLogos[key] || '');
}
function normalizeHexColor(value) {
  const input = String(value || '').trim();
  if (!input) return '';
  const hex = input.startsWith('#') ? input.slice(1) : input;
  if (/^[0-9a-fA-F]{3}$/.test(hex)) {
    return '#' + hex.split('').map(ch => ch + ch).join('').toLowerCase();
  }
  if (/^[0-9a-fA-F]{6}$/.test(hex)) return '#' + hex.toLowerCase();
  return '';
}
function resolveLibraryAccentColor(library = {}) {
  const saved = normalizeHexColor(library.accent_color || library.accentColor || '');
  if (saved) return saved;
  const seed = libraryVisualSeed(library);
  return defaultLibraryAccentPalette[hashString(seed) % defaultLibraryAccentPalette.length];
}
function hexToRgb(hex) {
  const normalized = normalizeHexColor(hex);
  if (!normalized) return { r: 45, g: 106, b: 95 };
  return {
    r: parseInt(normalized.slice(1, 3), 16),
    g: parseInt(normalized.slice(3, 5), 16),
    b: parseInt(normalized.slice(5, 7), 16),
  };
}
function rgbToHex({ r, g, b }) {
  const clamp = value => Math.max(0, Math.min(255, Math.round(value)));
  return `#${[clamp(r), clamp(g), clamp(b)].map(value => value.toString(16).padStart(2, '0')).join('')}`;
}
function mixColors(base, target, amount = 0.5) {
  const a = hexToRgb(base);
  const b = hexToRgb(target);
  return rgbToHex({
    r: a.r + (b.r - a.r) * amount,
    g: a.g + (b.g - a.g) * amount,
    b: a.b + (b.b - a.b) * amount,
  });
}
function rgbaColor(hex, alpha) {
  const { r, g, b } = hexToRgb(hex);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}
function luminance(hex) {
  const { r, g, b } = hexToRgb(hex);
  return (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
}
function readableTextColorForBackground(hex) {
  const { r, g, b } = hexToRgb(hex);
  const srgb = [r, g, b].map(value => {
    const normalized = value / 255;
    return normalized <= 0.03928 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
  });
  const relative = 0.2126 * srgb[0] + 0.7152 * srgb[1] + 0.0722 * srgb[2];
  return relative <= 0.72 ? '#ffffff' : '#181410';
}
function normalizeLibraries(libraries, fallbackPath = '') {
  const result = [];
  const seen = new Set();
  (libraries || []).forEach((library, index) => {
    const explicitIndex = Number.isInteger(Number(library && (library.index ?? library.logo_index)))
      ? Number(library.index ?? library.logo_index)
      : index;
    const id = String(library && library.id || '').trim();
    const path = (library && library.path || '').trim();
    if (!path) return;
    const key = id || path.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    const statFields = [
      'created_at', 'createdAt',
      'last_scanned_at', 'lastScannedAt', 'last_scan_at', 'lastScanAt',
      'unsupported_media_count', 'unsupportedMediaCount',
      'total_size_text', 'totalSizeText', 'storage_text', 'storageText',
      'photo_count', 'photoCount', 'image_count', 'imageCount',
      'video_count', 'videoCount',
    ];
    const stats = {};
    statFields.forEach(statKey => {
      if (library && Object.prototype.hasOwnProperty.call(library, statKey)) stats[statKey] = library[statKey];
    });
    result.push({
      index: explicitIndex,
      id,
      name: (library && library.name || '').trim() || copyText('app.interface.libraryNumberedName', { v0: (index + 1) }),
      path,
      logo_asset: library && (library.logo_asset || library.logoAsset) || '',
      logo_image_url: library && (library.logo_image_url || library.logoImageUrl) || '',
      accent_color: normalizeHexColor(library && (library.accent_color || library.accentColor) || ''),
      status: String(library && library.status || '').trim(),
      available: !(library && library.available === false),
      rebuild_required: !!(library && (library.rebuild_required || library.rebuildRequired)),
      unavailable_reason: String(library && (library.unavailable_reason || library.unavailableReason) || '').trim(),
      locked_by_username: String(library && (library.locked_by_username || library.lockedByUsername) || '').trim(),
      ...stats,
    });
  });
  if (!result.length && (fallbackPath || '').trim()) {
    result.push({ index: 0, id: '', name: copyText('app.interface.s_0642c4b8bf'), path: fallbackPath.trim(), logo_asset: '', logo_image_url: '', accent_color: '', status: '', available: true, unavailable_reason: '', locked_by_username: '' });
  }
  return result;
}
function showToast(message, duration = 2200) {
  let stack = document.getElementById('toast-stack');
  if (!stack) {
    stack = document.createElement('div');
    stack.id = 'toast-stack';
    stack.className = 'toast-stack';
    document.body.appendChild(stack);
  }
  const toast = document.createElement('div');
  toast.className = 'toast';
  toast.textContent = message;
  stack.appendChild(toast);
  requestAnimationFrame(() => toast.classList.add('show'));
  setTimeout(() => {
    toast.classList.remove('show');
    setTimeout(() => toast.remove(), 220);
  }, duration);
}

function currentUserRole() {
  return String(state?.serverSettings?.role || 'admin').trim().toLowerCase() || 'admin';
}
function isRootDebugMedia(photo) {
  return !!(photo && photo.debug_media);
}
function currentUsername() {
  return String(state?.serverSettings?.current_username || '').trim();
}
function isWarmEnabled() {
  return state.serverSettings.warm_enabled !== false;
}
function lightboxUIIdleSeconds() {
  return normalizeLightboxUIIdleSeconds(state.serverSettings.lightbox_ui_idle_seconds, 0);
}
function isRootUser() {
  return currentUserRole() === 'root';
}
function isVisitorUser() {
  return currentUserRole() === 'visitor';
}
function canManageLibraries() {
  return currentUserRole() === 'admin';
}
function canRunServerTasks() {
  return currentUserRole() === 'admin';
}
function canAccessBatchWorkflow() {
  return currentUserRole() === 'admin' || currentUserRole() === 'root';
}
function canWriteMedia() {
  return currentUserRole() === 'admin';
}
function canManageShareLinks() {
  return currentUserRole() === 'admin';
}
function canEditBookmarks() {
  return currentUserRole() === 'admin';
}
function rootCanViewSettings() {
  return false;
}

const rootDebugMediaItems = [
  {
    id: 'root-debug-img-7688',
    uuid: 'root-debug-img-7688',
    original_name: 'IMG_7688.JPG',
    media_kind: 'image',
    mime_type: 'image/jpeg',
    debug_media: true,
    file_url: '/api/root/debug-media/img-7688',
    thumbnail_url: '/api/root/debug-media/img-7688',
    taken_at: '',
    width: 0,
    height: 0,
    size: 0,
  },
  {
    id: 'root-debug-img-7703',
    uuid: 'root-debug-img-7703',
    original_name: 'IMG_7703.JPG',
    media_kind: 'image',
    mime_type: 'image/jpeg',
    debug_media: true,
    file_url: '/api/root/debug-media/img-7703',
    thumbnail_url: '/api/root/debug-media/img-7703',
    taken_at: '',
    width: 0,
    height: 0,
    size: 0,
  },
  {
    id: 'root-debug-mvi-8411',
    uuid: 'root-debug-mvi-8411',
    original_name: 'MVI_8411.MOV',
    media_kind: 'video',
    mime_type: 'video/quicktime',
    debug_media: true,
    file_url: '/api/root/debug-media/mvi-8411',
    playback_url: '/api/root/debug-media/mvi-8411',
    thumbnail_url: videoPosterPlaceholder,
    poster_url: videoPosterPlaceholder,
    taken_at: '',
    width: 0,
    height: 0,
    size: 0,
  },
];
function availableRootViews() {
  if (isRootUser()) return ['root-debug', 'settings'];
  const views = ['timeline', 'favorites', 'random-album', 'albums', 'memories'];
  if (!isVisitorUser()) views.push('trash');
  views.push('settings');
  return views;
}
function isAllowedRootView(view) {
  return availableRootViews().includes(view);
}
function forbidVisitorAction() {
  showToast(copyText('app.interface.s_8acd52c938'));
  return false;
}
function isForbiddenError(error) {
  return !!(error && (error.forbidden || Number(error.status) === 403));
}
function removeBlockingDialog(modal) {
  if (!modal) return;
  modal.classList.remove('open');
  setTimeout(() => modal.remove(), 140);
}
function removeTimelineLocateBlocking() {
  const modal = $('#timeline-locate-blocking-modal');
  if (modal) removeBlockingDialog(modal);
  if (state.timelineLocateBlocking) state.timelineLocateBlocking = false;
}
function showTimelineLocateBlocking({
  title = copyText('app.interface.s_6a82f57608'),
  message = copyText('app.interface.s_8c65b73dc8'),
  buttonText = copyText('app.interface.s_e94b248b5f'),
} = {}) {
  removeTimelineLocateBlocking();
  const modal = el('div', 'modal-overlay blocking-dialog-overlay open');
  modal.id = 'timeline-locate-blocking-modal';
  modal.innerHTML = `
    <div class="modal blocking-dialog-modal" role="dialog" aria-modal="true" aria-label="${escapeHTML(title)}">
      <div class="modal-title">${escapeHTML(title)}</div>
      <p class="modal-copy">${escapeHTML(message)}</p>
      <div class="modal-footer">
        <button class="btn btn-primary" type="button" disabled>${escapeHTML(buttonText)}</button>
      </div>
    </div>`;
  document.body.appendChild(modal);
  state.timelineLocateBlocking = true;
}
function showBlockingDialog({
  title = copyText('app.interface.s_ab3656a956'),
  message = '',
  confirmText = copyText('app.interface.s_f526c89937'),
  cancelText = copyText('app.interface.commonCancel'),
  danger = false,
  showCancel = true,
} = {}) {
  return new Promise(resolve => {
    const modal = el('div', 'modal-overlay blocking-dialog-overlay open');
    modal.innerHTML = `
      <div class="modal blocking-dialog-modal" role="dialog" aria-modal="true" aria-label="${escapeHTML(title)}">
        <div class="modal-title">${escapeHTML(title)}</div>
        <p class="modal-copy">${escapeHTML(message)}</p>
        <div class="modal-footer">
          ${showCancel ? `<button class="btn" type="button" data-dialog-cancel>${escapeHTML(cancelText)}</button>` : ''}
          <button class="btn ${danger ? 'btn-danger' : 'btn-primary'}" type="button" data-dialog-confirm>${escapeHTML(confirmText)}</button>
        </div>
      </div>`;
    const finish = accepted => {
      removeBlockingDialog(modal);
      resolve(accepted);
    };
    modal.addEventListener('click', e => {
      if (e.target === modal) finish(false);
    });
    modal.querySelector('[data-dialog-cancel]')?.addEventListener('click', () => finish(false));
    modal.querySelector('[data-dialog-confirm]')?.addEventListener('click', () => finish(true));
    document.body.appendChild(modal);
    requestAnimationFrame(() => modal.querySelector('[data-dialog-confirm]')?.focus());
  });
}
async function appConfirm(message, options = {}) {
  return showBlockingDialog({
    title: options.title || copyText('app.interface.s_a7d28b80b8'),
    message,
    confirmText: options.confirmText || copyText('app.interface.s_b56d9ac6c5'),
    cancelText: options.cancelText || copyText('app.interface.commonCancel'),
    danger: !!options.danger,
    showCancel: options.showCancel !== false,
  });
}
async function appAlert(message, options = {}) {
  await showBlockingDialog({
    title: options.title || copyText('app.interface.s_ab3656a956'),
    message,
    confirmText: options.confirmText || copyText('app.interface.s_cb63c62e50'),
    showCancel: false,
    danger: !!options.danger,
  });
}
function groupByDate(photos) {
  const groups = {};
  for (const p of photos) {
    const key = formatDate(p.taken_at);
    if (!groups[key]) groups[key] = [];
    groups[key].push(p);
  }
  return groups;
}
let _uid = 0;
function uid() { return 'u' + (++_uid); }

// c-3: 长按检测（移动端操作菜单）
function addLongPress(el, callback, delay = 500) {
  let timer = null;
  let moved = false;
  el.addEventListener('touchstart', e => {
    if (e.target?.closest?.('.favorite-toggle')) return;
    moved = false;
    timer = setTimeout(() => {
      if (!moved) { e.preventDefault(); callback(e); }
    }, delay);
  }, { passive: false });
  el.addEventListener('touchmove',  () => { moved = true; clearTimeout(timer); });
  el.addEventListener('touchend',   () => clearTimeout(timer));
  el.addEventListener('touchcancel',() => clearTimeout(timer));
}
function addUniversalLongPress(el, callback, delay = 520) {
  let timer = null;
  let pointerId = null;
  let startX = 0;
  let startY = 0;
  const clear = () => {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
    pointerId = null;
  };
  el.addEventListener('pointerdown', e => {
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    clear();
    pointerId = e.pointerId;
    startX = e.clientX;
    startY = e.clientY;
    timer = setTimeout(() => {
      timer = null;
      pointerId = null;
      callback(e);
    }, delay);
  });
  el.addEventListener('pointermove', e => {
    if (pointerId == null || e.pointerId !== pointerId || !timer) return;
    if (Math.hypot(e.clientX - startX, e.clientY - startY) > 10) clear();
  });
  el.addEventListener('pointerup', e => {
    if (pointerId == null || e.pointerId !== pointerId) return;
    clear();
  });
  el.addEventListener('pointercancel', e => {
    if (pointerId == null || e.pointerId !== pointerId) return;
    clear();
  });
}

// ── SVG 图标 ──────────────────────────────────────────
const icons = {
  rootDebug: '',
  rootDebugActive: '',
  timeline: '',
  timelineBold: '',
  favorite: '',
  favoriteBold: '',
  album: '',
  albumBold: '',
  shuffle: '',
  shuffleBold: '',
  trash: '',
  trashBold: '',
  upload: '',
  sun: '',
  moon: '',
  close: '',
  back: '',
  prev: '',
  next: '',
  share: '',
  check: '',
  photo: '',
  memories: '',
  memoriesBold: '',
  albumIntelligence: '',
  logout: '',
  shutdown: '',
  plus: '',
  libraryCreate: '',
  advancedSettings: '',
  topbarLoadAll: '',
  topbarUpload: '',
  topbarDownloadFavorites: '',
  topbarShuffle: '',
  topbarReturnRandomPosition: '',
  topbarSave: '',
  topbarNewAlbum: '',
  topbarPrevAlbum: '',
  topbarNextAlbum: '',
  topbarBackAlbums: '',
  topbarDownloadAlbum: '',
  topbarDeleteAlbum: '',
  albumDetailEdit: '',
  albumDetailDelete: '',
  albumDetailAdd: '',
  albumDetailDownload: '',
  albumContextEdit: '',
  albumContextDelete: '',
  albumContextAdd: '',
  albumContextDownload: '',
  topbarEmptyTrash: '',
  topbarClearMemories: '',
  libraryCardEdit: '',
  libraryCardDelete: '',
  libraryEditorCancel: '',
  libraryEditorSave: '',
  libraryEditorSwitch: '',
  libraryEditorDelete: '',
  libraryBatchSelectAll: '',
  libraryBatchExplore: '',
  libraryBatchCreate: '',
  libraryBatchCancel: '',
  libraryInfoCreated: '',
  libraryInfoScanned: '',
  libraryInfoUnsupported: '',
  libraryInfoStorage: '',
  libraryInfoPhotos: '',
  libraryInfoVideos: '',
  topbarTimelineOrder: '',
  albumCardFolder: '',
  albumCardUser: '',
  albumCardCount: '',
  albumViewGrid: '',
  albumViewList: '',
  topbarRestoreSelected: '',
  settingsPanelHeadUpdate: '',
  settingsPanelHeadBlank1: '',
  settingsPanelHeadBlank2: '',
  settingsPanelHeadBlank3: '',
  settingsPanelHeadBlank4: '',
  settingsPanelHeadBlank5: '',
  settingsPanelHeadBlank6: '',
  settingsPanelHeadBlank7: '',
  settingsPanelHeadBlank8: '',
  settings: '',
  settingsBold: '',
  shareSmall: '',
  favoriteSmall: '',
  favoriteFilled: '',
  like: '',
  superLike: '',
  dislike: '',
  deselectFav: '',
  selectionClear: '',
  selectionDownload: '',
  selectionDeselectFavorite: '',
  selectionDelete: '',
  nonLike: '',
  github: '',
  pin: '',
  autoplay: '',
  play: '',
  pause: '',
  menu: '',
  more: '',
  download: '',
  fit: '',
  slideshowLoop: '',
  timelineOrder: '',
  infoType: '',
  infoMime: '',
  infoDate: '',
  infoDateDay1: '',
  infoDateDay2: '',
  infoDateDay3: '',
  infoDateDay4: '',
  infoDateDay5: '',
  infoDateDay6: '',
  infoDateDay7: '',
  infoSize: '',
  infoRatio: '',
  infoDimensions: '',
  infoFileSize: '',
  infoVideoCodec: '',
  infoVideoFramerate: '',
  exifCamera: '',
  exifAperture: '',
  exifShutter: '',
  exifISO: '',
  exifFocal: '',
  exifGPS: '',
  exifOrientation: '',
  mediaAll: '',
  mediaImage: '',
  mediaVideo: '',
  bookmark: '',
  bookmarkBlank: '',
  albumSortTimelineDesc: '',
  albumSortTimelineAsc: '',
  albumSortName: '',
  albumSortSize: '',
  floatingSearch: '',
  gridScaleButton: '',
  search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"></circle><path d="m20 20-3.5-3.5"></path></svg>',
  contextSelect: '',
  contextView: '',
  contextTimeline: '',
  contextFavorite: '',
  contextReveal: '',
  contextDownload: '',
  contextAlbum: '',
  contextShare: '',
  contextDelete: '',
  contextRestore: '',
};
const svgIconFiles = {
  rootDebug: 'root-debug.svg',
  rootDebugActive: 'root-debug-active.svg',
  timeline: 'timeline.svg',
  timelineBold: 'timeline-bold.svg',
  favorite: 'favorite.svg',
  favoriteBold: 'favorite-bold.svg',
  album: 'album.svg',
  albumBold: 'album-bold.svg',
  shuffle: 'shuffle.svg',
  shuffleBold: 'shuffle-bold.svg',
  trash: 'trash.svg',
  trashBold: 'trash-bold.svg',
  upload: 'upload.svg',
  share: 'share.svg',
  settings: 'settings.svg',
  settingsBold: 'settings-bold.svg',
  sun: 'sun.svg',
  moon: 'moon.svg',
  close: 'close.svg',
  back: 'back.svg',
  prev: 'prev.svg',
  next: 'next.svg',
  check: 'check.svg',
  photo: 'photo.svg',
  memories: 'memories.svg',
  memoriesBold: 'memories-bold.svg',
  albumIntelligence: 'album-intelligence.svg',
  logout: 'logout-1.svg',
  shutdown: 'shutdown.svg',
  refresh: 'refresh.svg',
  plus: 'plus.svg',
  libraryCreate: 'library-create.svg',
  advancedSettings: 'advanced-settings.svg',
  topbarLoadAll: 'topbar-load-all.svg',
  topbarUpload: 'topbar-upload.svg',
  topbarDownloadFavorites: 'topbar-download-favorites.svg',
  topbarShuffle: 'topbar-shuffle.svg',
  topbarReturnRandomPosition: 'topbar-return-random-position.svg',
  topbarSave: 'topbar-save.svg',
  topbarNewAlbum: 'topbar-new-album.svg',
  topbarPrevAlbum: 'topbar-prev-album.svg',
  topbarNextAlbum: 'topbar-next-album.svg',
  topbarBackAlbums: 'topbar-back-albums.svg',
  topbarDownloadAlbum: 'topbar-download-album.svg',
  topbarDeleteAlbum: 'topbar-delete-album.svg',
  albumDetailEdit: 'album-detail-edit.svg',
  albumDetailDelete: 'album-detail-delete.svg',
  albumDetailAdd: 'album-detail-add.svg',
  albumDetailDownload: 'album-detail-download.svg',
  albumContextEdit: 'album-context-edit.svg',
  albumContextDelete: 'album-context-delete.svg',
  albumContextAdd: 'album-context-add.svg',
  albumContextDownload: 'album-context-download.svg',
  topbarEmptyTrash: 'topbar-empty-trash.svg',
  topbarRestoreAll: 'topbar-restore-all.svg',
  topbarClearMemories: 'topbar-clear-memories-action.svg',
  libraryCardEdit: 'library-card-edit.svg',
  libraryCardDelete: 'library-card-delete.svg',
  libraryEditorCancel: 'library-editor-cancel.svg',
  libraryEditorSave: 'library-editor-save.svg',
  libraryEditorSwitch: 'library-editor-switch.svg',
  libraryEditorDelete: 'library-editor-delete.svg',
  libraryBatchSelectAll: 'library-batch-select-all.svg',
  libraryBatchExplore: 'library-batch-explore.svg',
  libraryBatchCreate: 'library-batch-create.svg',
  libraryBatchCancel: 'library-batch-cancel.svg',
  libraryInfoCreated: 'library-info-created.svg',
  libraryInfoScanned: 'library-info-scanned.svg',
  libraryInfoUnsupported: 'library-info-unsupported.svg',
  libraryInfoStorage: 'library-info-storage.svg',
  libraryInfoPhotos: 'library-info-photos.svg',
  libraryInfoVideos: 'library-info-videos.svg',
  topbarTimelineOrder: 'topbar-timeline-order.svg',
  albumCardFolder: 'album-card-folder.svg',
  albumCardUser: 'album-card-user.svg',
  albumCardCount: 'album-card-count.svg',
  albumNavPrev: 'album-nav-prev.svg',
  albumNavNext: 'album-nav-next.svg',
  albumViewGrid: 'album-view-grid.svg',
  albumViewList: 'album-view-list.svg',
  topbarRestoreSelected: 'topbar-restore-selected.svg',
  settingsPanelHeadUpdate: 'settings-panel-head-update.svg',
  settingsPanelHeadEchoFrame: 'settings-panel-head-echoframe.svg',
  settingsPanelHeadBlank1: 'settings-panel-head-blank-1.svg',
  settingsPanelHeadBlank2: 'settings-panel-head-blank-2.svg',
  settingsPanelHeadBlank3: 'settings-panel-head-blank-3.svg',
  settingsPanelHeadBlank4: 'settings-panel-head-blank-4.svg',
  settingsPanelHeadBlank5: 'settings-panel-head-blank-5.svg',
  settingsPanelHeadBlank6: 'settings-panel-head-blank-6.svg',
  settingsPanelHeadBlank7: 'settings-panel-head-blank-7.svg',
  settingsPanelHeadBlank8: 'settings-panel-head-blank-8.svg',
  floatingSearch: 'floating-search.svg',
  gridScaleButton: 'grid-scale-button.svg',
  shareCardCopy: 'share-card-copy.svg',
  shareCardOpen: 'share-card-open.svg',
  shareCardDelete: 'share-card-delete.svg',
  shareModalLink: 'share-modal-link.svg',
  shareModalCopy: 'context-download.svg',
  shareModalCancel: 'close.svg',
  shareSmall: 'share-small.svg',
  favoriteSmall: 'favorite-small.svg',
  favoriteFilled: 'favorite-filled.svg',
  like: 'like.svg',
  superLike: 'super-like.svg',
  dislike: 'dislike.svg',
  deselectFav: 'deselect-fav.svg',
  selectionClear: 'selection-clear.svg',
  selectionDownload: 'selection-download.svg',
  selectionDeselectFavorite: 'selection-deselect-favorite.svg',
  selectionDelete: 'selection-delete.svg',
  nonLike: 'non-like.svg',
  github: 'github.svg',
  pin: 'pin.svg',
  autoplay: 'autoplay.svg',
  play: 'play.svg',
  pause: 'pause.svg',
  menu: 'menu.svg',
  more: 'more.svg',
  download: 'download.svg',
  fit: 'fit.svg',
  slideshowLoop: 'slideshow-loop.svg',
  timelineOrder: 'timeline-order.svg',
  infoType: 'info-type.svg',
  infoMime: 'info-mime.svg',
  infoDate: 'info-date.svg',
  infoDateDay1: 'library-info-created-day-1.svg',
  infoDateDay2: 'library-info-created-day-2.svg',
  infoDateDay3: 'library-info-created-day-3.svg',
  infoDateDay4: 'library-info-created-day-4.svg',
  infoDateDay5: 'library-info-created-day-5.svg',
  infoDateDay6: 'library-info-created-day-6.svg',
  infoDateDay7: 'library-info-created-day-7.svg',
  infoSize: 'info-size.svg',
  infoRatio: 'info-ratio.svg',
  infoDimensions: 'info-dimensions.svg',
  infoFileSize: 'info-file-size.svg',
  infoVideoCodec: 'info-video-codec.svg',
  infoVideoFramerate: 'info-video-framerate.svg',
  exifCamera: 'exif-camera.svg',
  exifAperture: 'exif-aperture.svg',
  exifShutter: 'exif-shutter.svg',
  exifISO: 'exif-iso.svg',
  exifFocal: 'exif-focal.svg',
  exifGPS: 'exif-gps.svg',
  exifOrientation: 'exif-orientation.svg',
  mediaAll: 'media-all.svg',
  mediaImage: 'media-image.svg',
  mediaVideo: 'media-video.svg',
  bookmark: 'video-bookmark.svg',
  bookmarkBlank: 'video-bookmark-blank.svg',
  albumSortTimelineDesc: 'album-sort-timeline-desc.svg',
  albumSortTimelineAsc: 'album-sort-timeline-asc.svg',
  albumSortName: 'album-sort-name.svg',
  albumSortSize: 'album-sort-size.svg',
  contextSelect: 'context-select.svg',
  contextView: 'context-view.svg',
  contextTimeline: 'context-timeline.svg',
  contextFavorite: 'context-favorite.svg',
  contextReveal: 'context-reveal.svg',
  contextDownload: 'context-download.svg',
  contextAlbum: 'context-album.svg',
  contextShare: 'context-share.svg',
  contextDelete: 'context-delete.svg',
  contextRestore: 'context-restore.svg',
  contextConvertPlayback: 'context-convert-playback.svg',
};

function navIconMarkup(view, active = false) {
  const iconSets = {
    'root-debug': [icons.rootDebug || icons.timeline, icons.rootDebugActive || icons.rootDebug || icons.timelineBold || icons.timeline],
    timeline: [icons.timeline, icons.timelineBold || icons.timeline],
    favorites: [icons.favorite, icons.favoriteBold || icons.favorite],
    'random-album': [icons.shuffle, icons.shuffleBold || icons.shuffle],
    albums: [icons.album, icons.albumBold || icons.album],
    memories: [icons.memories, icons.memoriesBold || icons.memories],
    trash: [icons.trash, icons.trashBold || icons.trash],
    settings: [icons.settings, icons.settingsBold || icons.settings],
  };
  const pair = iconSets[view] || [icons.photo, icons.photo];
  return active ? (pair[1] || pair[0] || '') : (pair[0] || '');
}

function syncNavActiveView(view) {
  $$('.nav-item[data-view]').forEach(a => {
    const active = a.dataset.view === view || (view === 'album-detail' && a.dataset.view === 'albums');
    a.classList.toggle('active', active);
    a.innerHTML = `${navIconMarkup(a.dataset.view, active)}<span class="nav-label">${escapeHTML(a.dataset.label || '')}</span>`;
  });
  syncRoleAwareNavigation();
}

function syncRoleAwareNavigation() {
  $('#nav-logo-btn')?.classList.toggle('hidden', isRootUser());
  if (isRootUser()) {
    const primaryItem = $('#main-nav .nav-item[data-view="timeline"], #main-nav .nav-item[data-view="root-debug"]');
    if (primaryItem) {
      primaryItem.dataset.view = 'root-debug';
      primaryItem.dataset.label = copyText('app.interface.navigationDebug');
      primaryItem.title = copyText('app.interface.navigationDebug');
      primaryItem.setAttribute('aria-label', copyText('app.interface.navigationDebug'));
      const active = state.view === 'root-debug';
      primaryItem.classList.toggle('active', active);
      primaryItem.innerHTML = `${navIconMarkup('root-debug', active)}<span class="nav-label">${copyText('app.interface.navigationDebug')}</span>`;
    }
  }
  $$('#main-nav .nav-item[data-view]').forEach(item => {
    const view = String(item.dataset.view || '').trim();
    const hidden = !isAllowedRootView(view);
    item.hidden = hidden;
    item.classList.toggle('is-role-hidden', hidden);
    item.setAttribute('aria-hidden', hidden ? 'true' : 'false');
    item.tabIndex = hidden ? -1 : 0;
  });
  if (isRootUser()) {
    $$('#main-nav .nav-item[data-view]').forEach(item => {
      const view = String(item.dataset.view || '').trim();
      if (view !== 'root-debug' && view !== 'settings') {
        item.remove();
      }
    });
  }
}
async function loadInlineSVGIcons() {
  await Promise.all(Object.entries(svgIconFiles).map(async ([key, file]) => {
    try {
      const response = await fetch(`/static/svg/${file}`, { cache: 'no-store' });
      if (!response.ok) return;
      const text = await response.text();
      if (text && text.includes('<svg')) {
        icons[key] = sanitizeInlineSVGMarkup(text);
      }
    } catch (_) {
      // 保留现有回退，避免图标文件暂时不可用时阻塞界面。
    }
  }));
}

function sanitizeInlineSVGMarkup(markup) {
  if (typeof DOMParser === 'undefined') return String(markup || '').trim();
  try {
    const doc = new DOMParser().parseFromString(String(markup || '').trim(), 'image/svg+xml');
    const svg = doc.documentElement;
    if (!svg || svg.nodeName.toLowerCase() !== 'svg') return String(markup || '').trim();
    svg.removeAttribute('width');
    svg.removeAttribute('height');
    const walker = doc.createTreeWalker(svg, NodeFilter.SHOW_ELEMENT);
    let node = svg;
    while (node) {
      sanitizeSVGNode(node);
      node = walker.nextNode();
    }
    return svg.outerHTML.trim();
  } catch (_) {
    return String(markup || '').trim();
  }
}

function sanitizeSVGNode(node) {
  ['fill', 'stroke'].forEach(attr => {
    const value = node.getAttribute(attr);
    if (!value) return;
    const normalized = String(value).trim();
    if (normalized === 'none' || normalized === 'currentColor' || normalized.startsWith('url(')) return;
    node.setAttribute(attr, 'currentColor');
  });
  const style = node.getAttribute('style');
  if (!style) return;
  const nextStyle = style.replace(/(fill|stroke)\s*:\s*([^;]+)/gi, (full, prop, rawValue) => {
    const normalized = String(rawValue || '').trim();
    if (normalized === 'none' || normalized === 'currentColor' || normalized.startsWith('url(')) return `${prop}: ${normalized}`;
    return `${prop}: currentColor`;
  });
  node.setAttribute('style', nextStyle);
}

// ── 主题 ──────────────────────────────────────────────
const systemThemeQuery = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
function systemTheme() {
  return systemThemeQuery && systemThemeQuery.matches ? 'dark' : 'light';
}
let themeSessionOverride = '';
let lastKnownSystemTheme = systemTheme();
function currentThemePreference() {
  return themeSessionOverride || lastKnownSystemTheme || systemTheme();
}
function syncThemeWithSystem({ clearOverride = false } = {}) {
  lastKnownSystemTheme = systemTheme();
  if (clearOverride) themeSessionOverride = '';
  if (!themeSessionOverride) setThemeValue(lastKnownSystemTheme);
}
function refreshThemeFromSystem() {
  const nextSystemTheme = systemTheme();
  if (nextSystemTheme !== lastKnownSystemTheme) {
    themeSessionOverride = '';
    lastKnownSystemTheme = nextSystemTheme;
    setThemeValue(nextSystemTheme);
  }
}
function setThemeValue(theme, { manual = false, persist = false } = {}) {
  const next = theme === 'dark' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  state.theme = next;
  updateThemeBtn();
  applyLibraryBranding();
  if (persist) persistSettings().catch(() => {});
}
function initTheme() {
  themeSessionOverride = '';
  syncThemeWithSystem({ clearOverride: true });
  if (systemThemeQuery) {
    if (systemThemeQuery.addEventListener) systemThemeQuery.addEventListener('change', refreshThemeFromSystem);
    else if (systemThemeQuery.addListener) systemThemeQuery.addListener(refreshThemeFromSystem);
  }
  window.addEventListener('focus', refreshThemeFromSystem);
  window.addEventListener('pageshow', refreshThemeFromSystem);
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) refreshThemeFromSystem();
  });
}
function toggleTheme() {
  const t = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
  themeSessionOverride = t;
  setThemeValue(t, { manual: true, persist: true });
}
function updateThemeBtn() {
  const btn = $('#theme-btn');
  if (!btn) return;
  const iconEl = btn.querySelector('.theme-icon');
  if (iconEl) iconEl.innerHTML = document.documentElement.dataset.theme === 'dark' ? icons.sun : icons.moon;
}

function normalizeMediaKindFilter(value) {
  return value === 'image' || value === 'video' ? value : 'all';
}
function normalizeAlbumKindFilter(value) {
  return value === 'user' || value === 'folder' || value === 'smart' ? value : 'all';
}
function normalizeTimelineOrder(value) {
  return value === 'asc' ? 'asc' : 'desc';
}
function normalizeAlbumDetailSort(value) {
  return ['name', 'size', 'timeline_asc', 'timeline_desc'].includes(value) ? value : 'timeline_desc';
}
function normalizePWASettings(raw = {}) {
  const data = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw : {};
  const name = String(data.name || '').trim() || 'EchoGallery';
  const iconSource = data.iconSource === 'svg' ? 'png' : (['favicon', 'library', 'png', 'upload'].includes(data.iconSource) ? data.iconSource : 'favicon');
  const themeMode = ['accent', 'fixed'].includes(data.themeMode) ? data.themeMode : 'accent';
  const themeColor = normalizeHexColor(data.themeColor || '') || '#2d6a5f';
  const startPage = ['/', '/login', 'auto'].includes(data.startPage) ? data.startPage : '/';
  const cacheStrategy = ['static-only', 'app-shell', 'disabled'].includes(data.cacheStrategy) ? data.cacheStrategy : 'static-only';
  const uploadedIconURL = String(data.uploadedIconURL || data.uploaded_icon_url || '').trim();
  return { name, iconSource, themeMode, themeColor, startPage, cacheStrategy, uploadedIconURL };
}
function currentAppDisplayName() {
  return String(state.serverSettings?.app_name || window.__GALLERY_NAME__ || '').trim() || 'EchoGallery';
}
function loadPWASettings() {
  try {
    return normalizePWASettings(JSON.parse(localStorage.getItem(pwaSettingsStorageKey) || '{}'));
  } catch (_) {
    return normalizePWASettings();
  }
}
function persistPWASettings() {
  state.pwaSettings = normalizePWASettings(state.pwaSettings);
  localStorage.setItem(pwaSettingsStorageKey, JSON.stringify(state.pwaSettings));
  window.dispatchEvent(new CustomEvent('eg:pwa-settings-change', { detail: state.pwaSettings }));
}
function normalizeUpdateSettings(input = {}) {
  const data = input && typeof input === 'object' && !Array.isArray(input) ? input : {};
  return {
    localEnabled: !!data.localEnabled,
    githubEnabled: !!data.githubEnabled,
  };
}
function loadUpdateSettings() {
  try {
    return normalizeUpdateSettings(JSON.parse(localStorage.getItem(updateSettingsStorageKey) || '{}'));
  } catch (_) {
    return normalizeUpdateSettings();
  }
}
function persistUpdateSettings() {
  state.updateSettings = normalizeUpdateSettings(state.updateSettings);
  localStorage.setItem(updateSettingsStorageKey, JSON.stringify(state.updateSettings));
}
function normalizeLightboxUIIdleSeconds(value, fallback = 0) {
  const num = Number(value);
  if (!Number.isFinite(num)) return Math.min(15, Math.max(0, fallback));
  return Math.min(15, Math.max(0, Math.round(num)));
}
function renderLocalUpdateConfigPreview() {
  return [
    'distribute = [',
    '  "/path/to/EchoGallery_1",',
    '  "/path/to/EchoGallery_2",',
    ']',
    'update_package_path = "/path/to/latest/EchoGallery"',
  ].join('\n');
}
async function saveLocalUpdateConfig(text) {
  return api.put('/api/settings/local-update-config', { text });
}
async function checkLocalUpdate() {
  return api.get('/api/settings/local-update/check');
}
async function applyLocalUpdate() {
  return api.post('/api/settings/local-update/apply', {});
}
function normalizePathForCompare(value) {
  return String(value || '').trim().replace(/\\/g, '/').replace(/\/+$/g, '');
}
function activeLibraryIdentity(settings = {}) {
  const path = normalizePathForCompare(settings.storage_path || '');
  if (path) return `path:${path}`;
  return `id:${String(settings.active_library_id || '').trim()}`;
}
function normalizeLibrariesForCompare(libraries = []) {
  return normalizeLibraries(libraries).map(library => ({
    id: String(library.id || '').trim(),
    path: normalizePathForCompare(library.path),
  }));
}
function cloneSettingsSnapshot(settings = state.serverSettings || {}) {
  return JSON.parse(JSON.stringify(settings || {}));
}
function settingsPayloadRequiresRestart(payload = buildSettingsPayload(), currentSettings = state.serverSettings || {}) {
  const current = currentSettings || {};
  const restartKeys = [
    ['port', value => Number(value) || 8080],
    ['storage_path', normalizePathForCompare],
    ['thumbnail_dir', normalizePathForCompare],
    ['thumbnail_size', () => 512],
    ['trash_dir', normalizePathForCompare],
  ];
  const changed = restartKeys.some(([key, normalize]) => normalize(payload[key]) !== normalize(current[key]));
  if (changed) return true;
  return JSON.stringify(normalizeLibrariesForCompare(payload.libraries)) !== JSON.stringify(normalizeLibrariesForCompare(current.libraries));
}
function currentMediaKindParam() {
  return state && state.mediaKindFilter !== 'all' ? state.mediaKindFilter : '';
}
function appendMediaKindParam(params) {
  const kind = currentMediaKindParam();
  if (kind) params.set('kind', kind);
  return params;
}
function mediaKindMatchesFilter(photo) {
  const kind = currentMediaKindParam();
  return !kind || (photo && photo.media_kind === kind);
}
function renderMediaKindFilterControl(className = '') {
  const active = normalizeMediaKindFilter(state.mediaKindFilter);
  const items = [
    { key: 'all', icon: icons.mediaAll || icons.photo, label: copyText('app.interface.s_fd83a9bda1') },
    { key: 'image', icon: icons.mediaImage || icons.photo, label: copyText('app.interface.s_f07dd3e2f1') },
    { key: 'video', icon: icons.mediaVideo || icons.play, label: copyText('app.interface.s_d4badd6343') },
  ];
  const classes = ['topbar-media-filter'];
  if (className) classes.push(className);
  return `<div class="${classes.join(' ')}" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.mediaKindFilterAria'))}">
    ${items.map(item => `<button class="media-filter-btn ${active === item.key ? 'active' : ''}" type="button" data-media-kind-filter="${item.key}" title="${item.label}" aria-label="${item.label}" aria-pressed="${active === item.key ? 'true' : 'false'}">${item.icon}</button>`).join('')}
  </div>`;
}
function normalizeTrashContentFilter(value) {
  // The recycle bin no longer exposes a combined "all" tab. Keep the first
  // filter as the safe default for fresh state and for stale in-memory state
  // left by the previous two-tab implementation.
  return value === 'folders' ? 'folders' : 'media';
}
function renderTrashContentFilterControl() {
  const active = normalizeTrashContentFilter(state.trashContentFilter);
  const items = [
    { key: 'media', icon: icons.photo, label: copyText('app.interface.s_05c4b9546e') },
    { key: 'folders', icon: icons.albumCardFolder || icons.album, label: copyText('app.interface.s_7b517c1774') },
  ];
  return `<div class="topbar-media-filter trash-content-filter" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.trashContentFilterAria'))}">
    ${items.map(item => `<button class="media-filter-btn${active === item.key ? ' active' : ''}" type="button" data-trash-content-filter="${item.key}" title="${item.label}" aria-label="${item.label}" aria-pressed="${active === item.key ? 'true' : 'false'}">${item.icon || ''}</button>`).join('')}
  </div>`;
}
function renderAlbumKindFilterControl() {
  const active = normalizeAlbumKindFilter(state.albumKindFilter);
  const items = [
    { key: 'all', icon: icons.album, label: copyText('app.interface.s_aea579df06') },
    { key: 'user', icon: icons.albumCardUser || icons.album, label: copyText('app.interface.s_05b503551a') },
    { key: 'folder', icon: icons.albumCardFolder || icons.album, label: copyText('app.interface.s_46ecac2910') },
    { key: 'smart', icon: icons.albumIntelligence || icons.album, label: copyText('app.interface.albumSmart') },
  ];
  return `<div class="topbar-media-filter album-kind-filter" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.albumKindFilterAria'))}">
    <div class="album-kind-filter-options" id="album-kind-filter-options" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.albumKindOptionsAria'))}">
      ${items.map(item => `<button class="media-filter-btn album-kind-filter-btn ${active === item.key ? 'active' : ''}" type="button" data-album-kind-filter="${item.key}" title="${item.label}${item.disabled ? copyText('app.interface.s_e2ba7e3451') : ''}" aria-label="${item.label}${item.disabled ? copyText('app.interface.s_e2ba7e3451') : ''}" aria-pressed="${active === item.key ? 'true' : 'false'}"${item.disabled ? ' disabled aria-disabled="true"' : ''}>${item.icon}</button>`).join('')}
    </div>
  </div>`;
}
function renderTopbarGlassButton({ id = '', icon = '', label = '', className = '', variant = '', disabled = false, extraAttrs = '' } = {}) {
  const classes = ['topbar-glass-btn'];
  if (variant) classes.push(`topbar-glass-btn-${variant}`);
  if (className) classes.push(className);
  return `<button class="${classes.join(' ')}" ${id ? `id="${id}"` : ''} type="button" title="${escapeHTML(label)}" aria-label="${escapeHTML(label)}" ${disabled ? 'disabled' : ''} ${extraAttrs}>${icon}</button>`;
}
function currentAccentButtonStyle() {
  const accent = resolveLibraryAccentColor(currentLibraryBrand() || {});
  const accentText = readableTextColorForBackground(accent);
  return `--highlight-bg:${accent};--highlight-text:${accentText}`;
}
function renderAlbumDetailActionButton({ id = '', icon = '', label = '', title = '', danger = false } = {}) {
  const buttonStyle = danger ? '--highlight-bg:var(--danger);--highlight-text:#ffffff' : currentAccentButtonStyle();
  const classes = ['btn', 'btn-primary', 'settings-floating-add-library', 'album-detail-action-btn'];
  if (danger) classes.push('album-detail-action-btn-danger');
  return `<button class="${classes.join(' ')}" id="${id}" type="button" title="${escapeHTML(title || label)}" aria-label="${escapeHTML(title || label)}" style="${buttonStyle}">${icon || ''}<span>${escapeHTML(label)}</span></button>`;
}
function renderTopbarLeadingGroup(buttons = []) {
  const items = (buttons || []).filter(Boolean);
  if (!items.length) return '';
  return items.length === 1 ? items[0] : `<div class="topbar-action-group">${items.join('')}</div>`;
}
function bindMediaKindFilterControl() {
  $$('[data-media-kind-filter]').forEach(btn => {
    btn.addEventListener('click', () => {
      const next = normalizeMediaKindFilter(btn.dataset.mediaKindFilter);
      if (next === state.mediaKindFilter) return;
      state.mediaKindFilter = next;
      localStorage.setItem(mediaKindFilterStorageKey, next);
      clearSelection();
      resetMediaFilteredViewState();
      state.viewScrollPositions[viewScrollKeyFor()] = 0;
      renderView();
    });
  });
}
function syncTrashLoadMoreObserver() {
  const canLoadMedia = state.trashContentFilter !== 'folders';
  updateLoadMoreUI('load-more', canLoadMedia && state.trashHasMore);
  if (!canLoadMedia) {
    disconnectSingleLoadMoreObserver('load-more');
    return;
  }
  observeLoadMore('load-more', loadMoreTrash, () => (
    state.trashContentFilter !== 'folders' && state.trashHasMore && !state.trashLoading
  ));
}

function bindTrashContentFilterControl() {
  $$('[data-trash-content-filter]').forEach(btn => {
    btn.addEventListener('click', async () => {
      const requested = normalizeTrashContentFilter(btn.dataset.trashContentFilter);
      const next = requested;
      if (next === state.trashContentFilter) return;
      state.trashContentFilter = next;
      syncTrashContentFilterControl();
      clearSelection();
      syncTrashLoadMoreObserver();
      if (next !== 'folders' && !state.trashLoaded && !state.trashLoading) {
        await loadMoreTrash();
      }
      // Load the media page before rebuilding the DOM. This prevents the
      // temporary empty state from replacing the real single-media results.
      renderTrashGroups([], { reset: true });
    });
  });
}
function syncTrashContentFilterControl() {
  const active = normalizeTrashContentFilter(state.trashContentFilter);
  $$('[data-trash-content-filter]').forEach(btn => {
    const isActive = normalizeTrashContentFilter(btn.dataset.trashContentFilter) === active;
    btn.classList.toggle('active', isActive);
    btn.setAttribute('aria-pressed', isActive ? 'true' : 'false');
  });
}
function bindAlbumKindFilterControl() {
  $$('[data-album-kind-filter]').forEach(btn => {
    btn.addEventListener('click', () => {
      if (btn.disabled) return;
      const next = normalizeAlbumKindFilter(btn.dataset.albumKindFilter);
      if (next === state.albumKindFilter) return;
      state.albumKindFilter = next;
      localStorage.setItem(albumKindFilterStorageKey, next);
      clearSelection();
      state.viewScrollPositions.albums = 0;
      renderAlbums();
    });
  });
}
function syncAlbumToolbarLayout() {
  const meta = $('#topbar-meta');
  if (!meta) return;
  const viewToggle = $('.album-view-toggle', meta);
  const kindFilter = $('.album-kind-filter', meta);
  if (!viewToggle || !kindFilter) return;

  meta.classList.remove('album-kind-hidden');
  const gap = parseFloat(getComputedStyle(meta).gap) || 0;
  const requiredWidth = viewToggle.getBoundingClientRect().width
    + kindFilter.getBoundingClientRect().width
    + gap;
  const availableWidth = Math.max(0, window.innerWidth - 24);
  meta.classList.toggle('album-kind-hidden', requiredWidth > availableWidth + 1);
}
function resetMediaFilteredViewState() {
  if (typeof stopEchoFrame === 'function') stopEchoFrame();
  state.photos = [];
  state.timelineCursor = '';
  state.timelineHasMore = true;
  state.timelineTotal = 0;
  state.timelineLoaded = false;
  state.randomAlbumPhotos = [];
  state.randomAlbumCursor = '';
  state.randomAlbumHasMore = true;
  state.randomAlbumLoaded = false;
  state.randomAlbumViewLoaded = false;
  state.randomAlbumTotal = 0;
  state.favoritePhotos = [];
  state.favoriteCursor = '';
  state.favoriteHasMore = true;
  state.favoriteTotal = 0;
  state.favoriteLoaded = false;
  state.albumPhotos = [];
  state.albumCursor = '';
  state.albumHasMore = true;
  state.albumDetailLoadedKey = '';
  state.albums = [];
  state.albumsLoaded = false;
  clearAlbumChildrenIndex();
  state.trashPhotos = [];
  state.trashCursor = '';
  state.trashHasMore = true;
  state.trashLoaded = false;
  state.memoriesLoaded = false;
}
function videoBookmarkKey(photo) {
  if (!photo) return '';
  return String(photo.uuid || photo.id || '');
}
function videoPlaybackPreferenceKey(photo) {
  return photo && photo.id ? String(photo.id) : '';
}
function normalizeVideoBookmarkState(raw) {
  const next = { bookmarks: [], resumeTime: 0 };
  if (typeof raw === 'number') {
    next.resumeTime = Math.max(0, Math.floor(Number(raw) || 0));
    return next;
  }
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return next;
  const resume = Number(raw.resumeTime ?? raw.resume_time ?? raw.lastTime ?? raw.last_time ?? raw.position ?? 0);
  if (Number.isFinite(resume) && resume >= 0) next.resumeTime = Math.floor(resume);
  const list = Array.isArray(raw.bookmarks)
    ? raw.bookmarks
    : Array.isArray(raw.items)
      ? raw.items
      : Array.isArray(raw.slots)
        ? raw.slots
        : [];
  list.forEach(item => {
    if (!item || typeof item !== 'object') return;
    const slot = Math.max(1, Math.min(10, Number(item.slot ?? item.index ?? 0) || 0));
    const time = Number(item.time ?? item.seconds ?? item.position ?? 0);
    if (!slot || !Number.isFinite(time) || time < 0) return;
    const name = String(item.name ?? item.label ?? '').trim();
    const found = next.bookmarks.findIndex(entry => entry.slot === slot);
    const bookmark = { slot, time: Math.floor(time), name };
    if (found >= 0) next.bookmarks[found] = bookmark;
    else next.bookmarks.push(bookmark);
  });
  next.bookmarks.sort((a, b) => a.slot - b.slot);
  return next;
}
function loadLegacyVideoBookmarks() {
  try {
    const parsed = JSON.parse(localStorage.getItem(videoBookmarksStorageKey) || '{}');
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return Object.fromEntries(Object.entries(parsed).map(([key, value]) => [key, normalizeVideoBookmarkState(value)]));
    }
  } catch (_) {
  }
  try {
    const parsed = JSON.parse(localStorage.getItem(legacyVideoBookmarksStorageKey) || '{}');
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return Object.fromEntries(Object.entries(parsed).map(([key, value]) => [key, normalizeVideoBookmarkState(value)]));
    }
  } catch (_) {}
  return {};
}
function persistLegacyVideoBookmarks(store) {
  try {
    localStorage.setItem(videoBookmarksStorageKey, JSON.stringify(store || {}));
  } catch (_) {
    // 旧缓存迁移失败不应影响当前播放。
  }
}
function clearLegacyVideoBookmarkState(photo) {
  const key = videoBookmarkKey(photo);
  if (!key || !state.legacyVideoBookmarks || !state.legacyVideoBookmarks[key]) return;
  delete state.legacyVideoBookmarks[key];
  persistLegacyVideoBookmarks(state.legacyVideoBookmarks);
}
function legacyVideoBookmarkStateForPhoto(photo) {
  const key = videoBookmarkKey(photo);
  if (!key) return null;
  const entry = state.legacyVideoBookmarks ? state.legacyVideoBookmarks[key] : null;
  return entry ? normalizeVideoBookmarkState(entry) : null;
}
function cloneVideoPlaybackBookmarks(bookmarks = []) {
  return (Array.isArray(bookmarks) ? bookmarks : []).map(bookmark => ({
    slot: Math.max(1, Math.min(10, Number(bookmark && bookmark.slot) || 0)),
    time: Math.max(0, Math.floor(Number(bookmark && bookmark.time) || 0)),
    name: String(bookmark && bookmark.name || '').trim(),
  })).filter(bookmark => bookmark.slot > 0);
}
function cloneVideoPlaybackPreference(pref) {
  const normalized = normalizeVideoPlaybackPreference(pref);
  return {
    volume: normalized.volume,
    muted: normalized.muted,
    resumeTime: normalized.resumeTime,
    bookmarks: cloneVideoPlaybackBookmarks(normalized.bookmarks),
  };
}
function cacheVideoPlaybackPreference(photo, pref) {
  if (!photo || !photo.id) return cloneVideoPlaybackPreference(pref);
  const key = videoPlaybackPreferenceKey(photo);
  const normalized = cloneVideoPlaybackPreference(pref);
  state.videoPlaybackPreferences[key] = normalized;
  photo.video_bookmark_count = normalized.bookmarks.length;
  photo.video_resume_time = normalized.resumeTime;
  updateVideoPlaybackInCollections(photo.id, normalized);
  updateVideoBookmarkThumbIndicators(photo.id);
  return normalized;
}
function syncCachedVideoPlaybackVolume(volume) {
  const sharedVolume = clampVideoVolumeToBounds(volume);
  Object.keys(state.videoPlaybackPreferences || {}).forEach(key => {
    if (!state.videoPlaybackPreferences[key]) return;
    state.videoPlaybackPreferences[key].volume = sharedVolume;
  });
}
function updateVideoPlaybackInCollections(photoId, pref) {
  const targetID = Number(photoId);
  if (!targetID) return;
  const collections = [
    state.photos,
    state.favoritePhotos,
    state.randomAlbumPhotos,
    state.albumPhotos,
    state.trashPhotos,
    state.lightboxPhotos,
    state.memoryPhotos,
    state.searchResults,
  ];
  const bookmarkCount = Array.isArray(pref && pref.bookmarks) ? pref.bookmarks.length : 0;
  const resumeTime = Math.max(0, Math.floor(Number(pref && pref.resumeTime) || 0));
  collections.forEach(list => {
    if (!Array.isArray(list)) return;
    list.forEach(photo => {
      if (!photo || Number(photo.id) !== targetID) return;
      photo.video_bookmark_count = bookmarkCount;
      photo.video_resume_time = resumeTime;
    });
  });
}
function getVideoBookmarkState(photo, { create = false } = {}) {
  const key = videoPlaybackPreferenceKey(photo);
  if (!key) return null;
  let entry = state.videoPlaybackPreferences[key];
  if (!entry && create) {
    entry = normalizeVideoPlaybackPreference(null);
    state.videoPlaybackPreferences[key] = entry;
  }
  if (entry) {
    state.videoPlaybackPreferences[key] = cloneVideoPlaybackPreference(entry);
    entry = state.videoPlaybackPreferences[key];
  }
  return entry || null;
}
function getVideoBookmarkList(photo) {
  const entry = getVideoBookmarkState(photo);
  if (entry) return entry.bookmarks;
  const legacy = legacyVideoBookmarkStateForPhoto(photo);
  return legacy ? legacy.bookmarks : [];
}
function getVideoBookmarkCount(photo) {
  if (!isVideoMedia(photo)) return 0;
  const cached = getVideoBookmarkList(photo).length;
  if (cached > 0) return cached;
  return Math.max(0, Number(photo && photo.video_bookmark_count) || 0);
}
function getVideoBookmarkCountByKey(key) {
  const entry = state.videoPlaybackPreferences[String(key || '')];
  if (entry && Array.isArray(entry.bookmarks)) return entry.bookmarks.length;
  return 0;
}
function getVideoBookmarkSlot(photo, slot) {
  const entry = getVideoBookmarkState(photo);
  const targetSlot = Math.max(1, Math.min(10, Number(slot) || 0));
  if (!entry || !targetSlot) return null;
  return entry.bookmarks.find(item => item.slot === targetSlot) || null;
}
function hasNearbyVideoBookmark(entry, slot, seconds, threshold = 1) {
  if (!entry || !Array.isArray(entry.bookmarks)) return false;
  const targetSlot = Math.max(1, Math.min(10, Number(slot) || 0));
  const targetTime = Math.max(0, Number(seconds) || 0);
  return entry.bookmarks.some(item => item.slot !== targetSlot && Math.abs((Number(item.time) || 0) - targetTime) <= threshold);
}
function setVideoBookmarkSlot(photo, slot, seconds, name = '') {
  const entry = getVideoBookmarkState(photo, { create: true });
  const targetSlot = Math.max(1, Math.min(10, Number(slot) || 0));
  const value = Math.max(0, Number(seconds) || 0);
  if (!entry || !targetSlot) return false;
  const index = entry.bookmarks.findIndex(item => item.slot === targetSlot);
  if (index < 0 && hasNearbyVideoBookmark(entry, targetSlot, value, 1)) return false;
  const bookmark = { slot: targetSlot, time: Math.floor(value), name: String(name || '').trim() };
  if (index >= 0) entry.bookmarks[index] = bookmark;
  else if (entry.bookmarks.length < 10) entry.bookmarks.push(bookmark);
  else return false;
  entry.bookmarks.sort((a, b) => a.slot - b.slot);
  queueVideoPlaybackPreferenceSave(photo, { bookmarks: entry.bookmarks });
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(photo);
  updateVideoBookmarkThumbIndicators(photo);
  clearLegacyVideoBookmarkState(photo);
  return true;
}
function deleteVideoBookmarkSlot(photo, slot) {
  const entry = getVideoBookmarkState(photo);
  const targetSlot = Math.max(1, Math.min(10, Number(slot) || 0));
  if (!entry || !targetSlot) return false;
  const before = entry.bookmarks.length;
  entry.bookmarks = entry.bookmarks.filter(item => item.slot !== targetSlot);
  if (entry.bookmarks.length === before) return false;
  queueVideoPlaybackPreferenceSave(photo, { bookmarks: entry.bookmarks });
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(photo);
  updateVideoBookmarkThumbIndicators(photo);
  clearLegacyVideoBookmarkState(photo);
  return true;
}
function addVideoBookmarkAtCurrentTime(photo, video = $('#lb-video')) {
  if (!isVideoMedia(photo) || !video || video.classList.contains('hidden')) {
    return { ok: false, reason: 'unavailable' };
  }
  const entry = getVideoBookmarkState(photo, { create: true });
  if (!entry) return { ok: false, reason: 'unavailable' };
  if (entry.bookmarks.length >= 10) return { ok: false, reason: 'full' };
  const slot = Array.from({ length: 10 }, (_, i) => i + 1)
    .find(candidate => !entry.bookmarks.some(item => Number(item.slot) === candidate));
  if (!slot) return { ok: false, reason: 'full' };
  const seconds = Math.max(0, Number(video.currentTime) || 0);
  if (hasNearbyVideoBookmark(entry, slot, seconds, 1)) return { ok: false, reason: 'nearby' };
  if (!setVideoBookmarkSlot(photo, slot, seconds)) return { ok: false, reason: 'failed' };
  return { ok: true, slot, seconds };
}
function getVideoResumeTime(photo) {
  const entry = getVideoBookmarkState(photo);
  const value = entry ? Number(entry.resumeTime) : Number(photo && photo.video_resume_time);
  return Number.isFinite(value) && value >= 0 ? Math.floor(value) : 0;
}
function setVideoResumeTime(photo, seconds) {
  const entry = getVideoBookmarkState(photo, { create: true });
  const value = Math.max(0, Number(seconds) || 0);
  if (!entry) return false;
  if (entry.resumeTime === Math.floor(value)) return true;
  entry.resumeTime = Math.floor(value);
  queueVideoPlaybackPreferenceSave(photo, { resume_time: entry.resumeTime });
  clearLegacyVideoBookmarkState(photo);
  return true;
}
function updateVideoBookmarkButton(photo = state.lightboxPhotos[state.lightboxIndex]) {
  const btn = $('#lb-video-bookmark');
  const isVideo = isVideoMedia(photo);
  $('#lightbox-header')?.classList.toggle('lightbox-video-active', isVideo);
  if (!btn) return;
  btn.classList.toggle('hidden', !isVideo || !canEditBookmarks() || isRootDebugMedia(photo));
  if (!isVideo) return;
  const count = getVideoBookmarkList(photo).length;
  btn.classList.toggle('active', count > 0);
  btn.innerHTML = count > 0 ? (icons.bookmark || '') : (icons.bookmarkBlank || icons.bookmark || '');
  btn.title = count > 0 ? copyText('app.interface.d_0da11be0c2', { v0: (count) }) : copyText('app.interface.s_06e011fdd9');
  btn.setAttribute('aria-label', btn.title);
}
function syncTouchPlaybackButton(video = $('#lb-video'), photo = state.lightboxPhotos[state.lightboxIndex]) {
  const btn = $('#lb-touch-playback-toggle');
  const icon = $('#lb-touch-playback-icon');
  if (!btn) return;
  const visible = isVideoMedia(photo)
    && video
    && !video.classList.contains('hidden');
  btn.classList.toggle('hidden', !visible);
  if (!visible) {
    btn.classList.remove('active');
    return;
  }
  const paused = !video || video.paused;
  if (icon) icon.innerHTML = paused ? icons.play : icons.pause;
  const label = paused ? copyText('app.interface.s_af7d6e03da') : copyText('app.interface.s_a8505ca8e2');
  btn.title = label;
  btn.setAttribute('aria-label', label);
  btn.setAttribute('aria-pressed', paused ? 'false' : 'true');
}
function updateMediaSessionPlaybackState(video = $('#lb-video')) {
  if (!('mediaSession' in navigator)) return;
  try {
    navigator.mediaSession.playbackState = video && !video.paused ? 'playing' : 'paused';
  } catch (_) {}
}
function clearLightboxMediaSession() {
  if (!('mediaSession' in navigator)) return;
  try {
    navigator.mediaSession.metadata = null;
    navigator.mediaSession.playbackState = 'none';
    ['play', 'pause', 'seekbackward', 'seekforward'].forEach(action => {
      try { navigator.mediaSession.setActionHandler(action, null); } catch (_) {}
    });
  } catch (_) {}
}
function setMediaSessionAction(action, handler) {
  if (!('mediaSession' in navigator)) return;
  try { navigator.mediaSession.setActionHandler(action, handler); } catch (_) {}
}
function syncLightboxMediaSession(photo, video = $('#lb-video')) {
  if (!('mediaSession' in navigator) || !isVideoMedia(photo) || !video) {
    clearLightboxMediaSession();
    return;
  }
  try {
    if ('MediaMetadata' in window) {
      const artwork = mediaThumbURL(photo)
        ? [{ src: mediaThumbURL(photo), sizes: '512x512', type: 'image/webp' }]
        : [];
      navigator.mediaSession.metadata = new MediaMetadata({
        title: photo.original_name || 'EchoGallery',
        artist: 'EchoGallery',
        album: copyText('app.interface.s_7ae9c3e4bf'),
        artwork,
      });
    }
  } catch (_) {}
  setMediaSessionAction('play', () => {
    if (video && video.paused) video.play().catch(() => {});
  });
  setMediaSessionAction('pause', () => {
    if (video && !video.paused) video.pause();
  });
  setMediaSessionAction('seekbackward', details => {
    adjustVideoTime(-(Number(details && details.seekOffset) || 5));
  });
  setMediaSessionAction('seekforward', details => {
    adjustVideoTime(Number(details && details.seekOffset) || 5);
  });
  updateMediaSessionPlaybackState(video);
}
function updateVideoBookmarkProgress(video = $('#lb-video'), photo = state.lightboxPhotos[state.lightboxIndex]) {
  if (video && !('currentTime' in video) && !video.classList) {
    photo = video;
    video = $('#lb-video');
  }
  const track = $('#lb-video-progress');
  const fill = $('#lb-video-progress-fill');
  const markers = $('#lb-video-progress-markers');
  const currentLabel = $('#lb-video-progress-current');
  const durationLabel = $('#lb-video-progress-duration');
  if (!track || !fill || !markers) return;
  const isVideo = isVideoMedia(photo) && video && !video.classList.contains('hidden');
  track.classList.toggle('hidden', !isVideo);
  if (!isVideo) {
    fill.style.width = '0%';
    markers.innerHTML = '';
    if (currentLabel) currentLabel.textContent = '0:00';
    if (durationLabel) durationLabel.textContent = '0:00';
    return;
  }
  if (!Number.isFinite(video.duration) || video.duration <= 0) {
    fill.style.width = '0%';
    markers.innerHTML = '';
    if (currentLabel) currentLabel.textContent = formatDuration((video.currentTime || 0) * 1000);
    if (durationLabel) durationLabel.textContent = '0:00';
    return;
  }
  const pendingCurrent = pendingVideoSeekTimeFor(video);
  const current = Math.max(0, Number.isFinite(pendingCurrent) ? pendingCurrent : (Number(video.currentTime) || 0));
  const progressPercent = Math.max(0, Math.min(100, current / video.duration * 100));
  fill.style.width = progressPercent <= 0 ? '0%' : `max(var(--video-progress-track-height, 8px), ${progressPercent}%)`;
  if (currentLabel) currentLabel.textContent = formatDuration(current * 1000);
  if (durationLabel) durationLabel.textContent = formatDuration(video.duration * 1000);
  const bookmarks = getVideoBookmarkList(photo);
  const sectionMinSeconds = Math.max(60, (Number(state.videoSectionMinMinutes) || 10) * 60);
  const sections = video.duration >= sectionMinSeconds
    ? Array.from({ length: 9 }, (_, index) => {
      const section = index + 1;
      const time = video.duration / 10 * section;
      const left = Math.max(0, Math.min(100, time / video.duration * 100));
      return `<button type="button" class="lightbox-video-progress-section-marker" data-video-section="${section}" style="left:${left}%" title="${escapeHTML(copyText('app.interface.d_section_title', { section: section + 1 }))}" aria-label="${escapeHTML(copyText('app.interface.d_section_label', { section: section + 1 }))}"></button>`;
    })
    : [];
  const bookmarkMarkers = bookmarks.map(bookmark => {
    const left = Math.max(0, Math.min(100, bookmark.time / video.duration * 100));
    const label = bookmark.name ? `${bookmark.name} · ${formatDuration(bookmark.time * 1000)}` : copyText('app.interface.d_5cf0e311de', { v0: (bookmark.slot), v1: (formatDuration(bookmark.time * 1000)) });
    return `<button type="button" class="lightbox-video-progress-marker" data-video-bookmark-slot="${bookmark.slot}" style="left:${left}%" title="${escapeHTML(label)}" aria-label="${escapeHTML(label)}">${bookmark.slot === 10 ? '0' : bookmark.slot}</button>`;
  });
  markers.innerHTML = sections.concat(bookmarkMarkers).join('');
}
function normalizeVideoVolumeValue(value, fallback = 1) {
  const volume = Number(value);
  if (!Number.isFinite(volume)) return Math.max(0, Math.min(1, Number(fallback) || 1));
  return Math.max(0, Math.min(1, volume));
}
function normalizeVideoVolumeSwipeSensitivity(value, fallback = 100) {
  const numeric = Math.round(Number(value));
  if (!Number.isFinite(numeric)) return Math.max(40, Math.min(220, Number(fallback) || 100));
  return Math.max(40, Math.min(220, numeric));
}
function normalizeVideoVolumePercent(value, fallback = 0) {
  const numeric = Math.round(Number(value));
  if (!Number.isFinite(numeric)) return Math.max(0, Math.min(100, Number(fallback) || 0));
  return Math.max(0, Math.min(100, numeric));
}
function effectiveVideoVolumeBounds() {
  const min = normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0);
  const max = Math.max(min, normalizeVideoVolumePercent(state.videoVolumeMaxPercent, 100));
  return { min, max };
}
function clampVideoVolumeToBounds(value) {
  const normalized = normalizeVideoVolumeValue(value, state.globalVideoVolume);
  const bounds = effectiveVideoVolumeBounds();
  const min = bounds.min / 100;
  const max = bounds.max / 100;
  return Math.max(min, Math.min(max, normalized));
}
function loadGlobalVideoVolume() {
  try {
    return normalizeVideoVolumeValue(localStorage.getItem(globalVideoVolumeStorageKey), 1);
  } catch (_) {
    return 1;
  }
}
function persistGlobalVideoVolume(volume) {
  state.globalVideoVolume = clampVideoVolumeToBounds(volume);
  try {
    localStorage.setItem(globalVideoVolumeStorageKey, String(state.globalVideoVolume));
  } catch (_) {}
  return state.globalVideoVolume;
}
function effectiveVideoVolume(video = $('#lb-video')) {
  const stored = Number(video && video.dataset ? video.dataset.egVolume : NaN);
  if (Number.isFinite(stored)) return clampVideoVolumeToBounds(stored);
  if (video && Number.isFinite(Number(video.volume))) return clampVideoVolumeToBounds(video.volume);
  return clampVideoVolumeToBounds(state.globalVideoVolume);
}
function ensureVideoGainNode(video = $('#lb-video')) {
  if (!video || !isTouchLikeDevice()) return null;
  if (state.lightboxVideoGainNode && state.lightboxVideoAudioElement === video) {
    if (state.lightboxVideoAudioContext && state.lightboxVideoAudioContext.state === 'suspended') {
      state.lightboxVideoAudioContext.resume().catch(() => {});
    }
    return state.lightboxVideoGainNode;
  }
  const AudioContextClass = window.AudioContext || window.webkitAudioContext;
  if (!AudioContextClass) return null;
  try {
    if (!state.lightboxVideoAudioContext) state.lightboxVideoAudioContext = new AudioContextClass();
    if (state.lightboxVideoAudioContext.state === 'suspended') {
      state.lightboxVideoAudioContext.resume().catch(() => {});
    }
    state.lightboxVideoAudioSource = state.lightboxVideoAudioContext.createMediaElementSource(video);
    state.lightboxVideoGainNode = state.lightboxVideoAudioContext.createGain();
    state.lightboxVideoAudioSource.connect(state.lightboxVideoGainNode);
    state.lightboxVideoGainNode.connect(state.lightboxVideoAudioContext.destination);
    state.lightboxVideoAudioElement = video;
    return state.lightboxVideoGainNode;
  } catch (_) {
    return null;
  }
}
async function restoreVideoAudioOutput(video = $('#lb-video')) {
  if (!video || video.classList.contains('hidden')) return;
  const AudioContextClass = window.AudioContext || window.webkitAudioContext;
  if (AudioContextClass && isTouchLikeDevice()) {
    try {
      ensureVideoGainNode(video);
      if (state.lightboxVideoAudioContext && state.lightboxVideoAudioContext.state === 'suspended') {
        await state.lightboxVideoAudioContext.resume();
      }
    } catch (_) {}
  }
  applyVideoVolumeOutput(video, effectiveVideoVolume(video));
  if (video.muted) {
    video.setAttribute('muted', '');
  } else {
    video.removeAttribute('muted');
  }
}
function applyVideoVolumeOutput(video = $('#lb-video'), volume = state.globalVideoVolume) {
  if (!video) return false;
  const value = clampVideoVolumeToBounds(volume);
  if (video.dataset) video.dataset.egVolume = String(value);
  const gain = ensureVideoGainNode(video);
  if (gain) {
    try { video.volume = 1; } catch (_) {}
    gain.gain.value = value;
  } else {
    try { video.volume = value; } catch (_) {}
  }
  return true;
}
function normalizeVideoPlaybackPreference(raw) {
  const legacy = normalizeVideoBookmarkState(raw);
  return {
    volume: clampVideoVolumeToBounds(raw && raw.volume),
    muted: !!(raw && raw.muted),
    resumeTime: Math.max(0, Math.floor(Number(raw && (raw.resume_time ?? raw.resumeTime ?? legacy.resumeTime)) || 0)),
    bookmarks: cloneVideoPlaybackBookmarks(raw && raw.bookmarks || legacy.bookmarks),
  };
}
function shouldMigrateLegacyVideoPlayback(serverPref, legacyPref) {
  if (!legacyPref) return false;
  const legacyHasData = (legacyPref.resumeTime || 0) > 0 || (legacyPref.bookmarks || []).length > 0;
  if (!legacyHasData) return false;
  const serverHasData = (serverPref.resumeTime || 0) > 0 || (serverPref.bookmarks || []).length > 0;
  return !serverHasData;
}
async function loadVideoPlaybackPreference(photo) {
  const key = videoPlaybackPreferenceKey(photo);
  if (!key || !isVideoMedia(photo)) return normalizeVideoPlaybackPreference(null);
  if (state.videoPlaybackPreferences[key]) return state.videoPlaybackPreferences[key];
  if (state.videoPlaybackPreferenceRequests[key]) return state.videoPlaybackPreferenceRequests[key];
  state.videoPlaybackPreferenceRequests[key] = (async () => {
    const legacyPref = legacyVideoBookmarkStateForPhoto(photo);
    try {
      let pref = normalizeVideoPlaybackPreference(await api.get(`/api/media/${photo.id}/playback`));
      if (shouldMigrateLegacyVideoPlayback(pref, legacyPref)) {
        pref = normalizeVideoPlaybackPreference(await api.put(`/api/media/${photo.id}/playback`, {
          resume_time: legacyPref.resumeTime,
          bookmarks: legacyPref.bookmarks,
        }));
        clearLegacyVideoBookmarkState(photo);
      }
      return cacheVideoPlaybackPreference(photo, pref);
    } catch (_) {
      if (legacyPref) return cacheVideoPlaybackPreference(photo, legacyPref);
      return cacheVideoPlaybackPreference(photo, null);
    } finally {
      delete state.videoPlaybackPreferenceRequests[key];
    }
  })();
  return state.videoPlaybackPreferenceRequests[key];
}
async function restoreVideoPlaybackPreference(video, photo) {
  if (!video || !isVideoMedia(photo)) return;
  const pref = await loadVideoPlaybackPreference(photo);
  if (state.lightboxPhotos[state.lightboxIndex] !== photo) return;
  video.dataset.restoringPlaybackPreference = '1';
  if (state.continueLastVideoPosition && Number.isFinite(video.duration) && video.duration > 0) {
    const saved = Math.max(0, Math.floor(Number(pref && pref.resumeTime) || 0));
    if (saved > 0 && saved < video.duration - 2 && Math.abs((Number(video.currentTime) || 0) - saved) > 1) {
      video.currentTime = saved;
    }
  }
  state.globalVideoVolume = normalizeVideoVolumeValue(pref.volume, state.globalVideoVolume);
  applyVideoVolumeOutput(video, state.globalVideoVolume);
  video.muted = pref.muted;
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(video, photo);
  requestAnimationFrame(() => {
    if (video) delete video.dataset.restoringPlaybackPreference;
  });
}
function queueVideoPlaybackPreferenceSave(photo, patch = {}, delay = 380) {
  const key = videoPlaybackPreferenceKey(photo);
  if (!key || !photo || !photo.id) return;
  const queue = state.videoPlaybackPreferenceSaveQueue[key] || {};
  if (Object.prototype.hasOwnProperty.call(patch, 'volume')) queue.volume = normalizeVideoVolumeValue(patch.volume, state.globalVideoVolume);
  if (Object.prototype.hasOwnProperty.call(patch, 'muted')) queue.muted = !!patch.muted;
  if (Object.prototype.hasOwnProperty.call(patch, 'resume_time')) queue.resume_time = Math.max(0, Math.floor(Number(patch.resume_time) || 0));
  if (Object.prototype.hasOwnProperty.call(patch, 'bookmarks')) queue.bookmarks = cloneVideoPlaybackBookmarks(patch.bookmarks);
  state.videoPlaybackPreferenceSaveQueue[key] = queue;
  clearTimeout(state.videoPlaybackPreferenceSaveTimers[key]);
  state.videoPlaybackPreferenceSaveTimers[key] = setTimeout(async () => {
    const payload = state.videoPlaybackPreferenceSaveQueue[key];
    delete state.videoPlaybackPreferenceSaveQueue[key];
    delete state.videoPlaybackPreferenceSaveTimers[key];
    try {
      const pref = await api.put(`/api/media/${photo.id}/playback`, payload);
      cacheVideoPlaybackPreference(photo, pref);
      if (state.lightboxPhotos[state.lightboxIndex] && Number(state.lightboxPhotos[state.lightboxIndex].id) === Number(photo.id)) {
        updateVideoBookmarkButton(photo);
        updateVideoBookmarkProgress($('#lb-video'), photo);
        if (_ctxMenu?.classList.contains('lightbox-bookmark-menu')) refreshVideoBookmarkMenu();
      }
    } catch (_) {
      // 静默失败：下次播放或再次修改时会继续尝试同步。
    }
  }, delay);
}
function persistVideoPlaybackPreferenceSoon(video = $('#lb-video'), photo = state.lightboxPhotos[state.lightboxIndex]) {
  if (!video || !isVideoMedia(photo) || !photo.id || video.dataset.restoringPlaybackPreference === '1') return;
  const key = videoPlaybackPreferenceKey(photo);
  const muted = !!video.muted;
  const hasPrevious = !!state.videoPlaybackPreferences[key];
  const previous = state.videoPlaybackPreferences[key] || normalizeVideoPlaybackPreference(null);
  const previousVolume = normalizeVideoVolumeValue(previous.volume, 1);
  const volume = persistGlobalVideoVolume(effectiveVideoVolume(video));
  if (hasPrevious && previous.muted === muted && Math.abs(previousVolume - volume) < 0.001) return;
  syncCachedVideoPlaybackVolume(volume);
  const pref = normalizeVideoPlaybackPreference({ volume, muted });
  pref.resumeTime = previous.resumeTime;
  pref.bookmarks = cloneVideoPlaybackBookmarks(previous.bookmarks);
  state.videoPlaybackPreferences[key] = pref;
  queueVideoPlaybackPreferenceSave(photo, { volume: pref.volume, muted: pref.muted });
}
function flushPendingVideoPlaybackPreference(photo = state.lightboxPhotos[state.lightboxIndex], options = {}) {
  if (!photo || !photo.id) return;
  const key = videoPlaybackPreferenceKey(photo);
  const pending = state.videoPlaybackPreferenceSaveQueue[key];
  if (!pending) return;
  clearTimeout(state.videoPlaybackPreferenceSaveTimers[key]);
  delete state.videoPlaybackPreferenceSaveTimers[key];
  delete state.videoPlaybackPreferenceSaveQueue[key];
  const request = fetchWithPageSession(`/api/media/${photo.id}/playback`, {
    method: 'PUT',
    headers: buildPageSessionHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(pending),
    keepalive: !!options.keepalive,
  }).then(response => {
    if (!response.ok || options.keepalive) return null;
    return response.json();
  });
  void request.then(pref => {
    if (!pref) return;
    cacheVideoPlaybackPreference(photo, pref);
  }).catch(() => {});
}
function showVideoProgressActivity(duration = 3000) {
  const progress = $('#lb-video-progress');
  if (!progress || progress.classList.contains('hidden')) return;
  const touchPlaybackToggle = $('#lb-touch-playback-toggle');
  progress.classList.add('active');
  touchPlaybackToggle?.classList.add('active');
  clearTimeout(state.lightboxVideoProgressTimer);
  state.lightboxVideoProgressTimer = setTimeout(() => {
    if (!progress.classList.contains('scrubbing')) {
      progress.classList.remove('active');
      touchPlaybackToggle?.classList.remove('active');
    }
  }, duration);
}
function seekVideoFromProgressClientX(track, clientX) {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!track || !video || video.classList.contains('hidden') || !Number.isFinite(video.duration) || video.duration <= 0) return false;
  const rect = track.getBoundingClientRect();
  const ratio = rect.width > 0 ? Math.max(0, Math.min(1, (clientX - rect.left) / rect.width)) : 0;
  return seekVideoToTime(video, video.duration * ratio, { photo });
}
function jumpVideoToProgressMarker(marker) {
  const video = $('#lb-video');
  const photo = state.lightboxPhotos[state.lightboxIndex];
  if (!marker || !video || video.classList.contains('hidden') || !Number.isFinite(video.duration) || video.duration <= 0) return false;
  const bookmarkSlot = Number(marker.dataset.videoBookmarkSlot || 0);
  if (bookmarkSlot) {
    const bookmark = getVideoBookmarkSlot(photo, bookmarkSlot);
    if (!bookmark) return false;
    return seekVideoToTime(video, Number(bookmark.time) || 0, { photo, throttled: false });
  }
  const section = Number(marker.dataset.videoSection || 0);
  if (section > 0) {
    return seekVideoToTime(video, video.duration / 10 * section, { photo, throttled: false });
  }
  return false;
}
function nearestVideoProgressMarker(track, clientX, pointerType = '') {
  if (!track) return null;
  const rect = track.getBoundingClientRect();
  if (!rect.width) return null;
  const markers = [...track.querySelectorAll('[data-video-bookmark-slot]')];
  if (!markers.length) return null;
  const threshold = pointerType === 'touch' || pointerType === 'pen' ? 28 : 18;
  let best = null;
  let bestDistance = Infinity;
  markers.forEach(marker => {
    const leftPercent = parseFloat(marker.style.left || '0');
    if (!Number.isFinite(leftPercent)) return;
    const markerX = rect.left + rect.width * (leftPercent / 100);
    const distance = Math.abs(clientX - markerX);
    if (distance < bestDistance) {
      best = marker;
      bestDistance = distance;
    }
  });
  if (!best || bestDistance > threshold) return null;
  return best;
}
function beginVideoProgressScrub(e, track) {
  if (!track || e.button > 0) return false;
  const marker = e.target.closest('[data-video-bookmark-slot], [data-video-section]') || nearestVideoProgressMarker(track, e.clientX, e.pointerType);
  if (marker && jumpVideoToProgressMarker(marker)) {
    state.videoProgressClickSuppressUntil = Date.now() + 450;
    e.preventDefault();
    e.stopPropagation();
    return true;
  }
  const progress = $('#lb-video-progress');
  if (!progress || !seekVideoFromProgressClientX(track, e.clientX)) return false;
  state.videoProgressScrubPointerId = e.pointerId;
  state.videoProgressClickSuppressUntil = Date.now() + 450;
  progress.classList.add('active', 'scrubbing');
  progress.classList.toggle('touching', e.pointerType === 'touch' || e.pointerType === 'pen');
  if (track.setPointerCapture) {
    try { track.setPointerCapture(e.pointerId); } catch (_) {}
  }
  e.preventDefault();
  e.stopPropagation();
  return true;
}
function updateVideoProgressScrub(e) {
  if (state.videoProgressScrubPointerId !== e.pointerId) return false;
  const track = $('.lightbox-video-progress-track');
  if (!track) return false;
  seekVideoFromProgressClientX(track, e.clientX);
  e.preventDefault();
  return true;
}
function endVideoProgressScrub(e) {
  if (state.videoProgressScrubPointerId !== e.pointerId) return false;
  const progress = $('#lb-video-progress');
  const track = $('.lightbox-video-progress-track');
  if (track && track.releasePointerCapture) {
    try { track.releasePointerCapture(e.pointerId); } catch (_) {}
  }
  state.videoProgressScrubPointerId = null;
  state.videoProgressClickSuppressUntil = Date.now() + 450;
  if (progress) {
    progress.classList.remove('scrubbing', 'touching');
    showVideoProgressActivity();
  }
  e.preventDefault();
  return true;
}
function beginVideoSurfaceSwipe(e) {
  if (e.pointerType !== 'touch' && e.pointerType !== 'pen') return false;
  if (state.slideshowPlaying || state.videoProgressScrubPointerId != null) return false;
  if (e.target.closest('.lightbox-header, .lightbox-info, .lightbox-video-progress, .lightbox-volume-feedback, .lb-nav, .lb-touch-playback-toggle, .context-menu, button, input, select, label, a')) return false;
  if (!e.target.closest('.lightbox-media-frame, #lb-video')) return false;
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!isVideoMedia(photo) || !video || video.classList.contains('hidden')) return false;
  const body = $('.lightbox-body');
  const width = Math.max(1, (body && body.clientWidth) || window.innerWidth || 1);
  const height = Math.max(1, (body && body.clientHeight) || window.innerHeight || 1);
  const centerStart = e.clientX >= width * 0.32 && e.clientX <= width * 0.68 && e.clientY >= height * 0.18 && e.clientY <= height * 0.82;
  state.videoSurfaceSwipe = {
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    startTime: Number(video.currentTime) || 0,
    startVolume: effectiveVideoVolume(video),
    centerStart,
    dismissProgress: 0,
    dragging: false,
    mode: '',
    width,
    height,
  };
  if (body && body.setPointerCapture) {
    try { body.setPointerCapture(e.pointerId); } catch (_) {}
  }
  return true;
}
function updateVideoSurfaceSwipe(e) {
  const swipe = state.videoSurfaceSwipe;
  if (!swipe || swipe.pointerId !== e.pointerId) return false;
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  const dx = e.clientX - swipe.startX;
  const dy = e.clientY - swipe.startY;
  const absX = Math.abs(dx);
  const absY = Math.abs(dy);
  if (!swipe.dragging) {
    if (swipe.centerStart && dy > 0 && absY > 12 && absY > absX * 1.08) {
      swipe.dragging = true;
      swipe.mode = 'dismiss';
    } else if (absY > 22 && absY > absX * 1.2) {
      swipe.dragging = true;
      swipe.mode = 'volume';
      showVolumeOverlay(video);
    } else if (absX >= 14 && absX >= absY * 1.2) {
      swipe.dragging = true;
      swipe.mode = 'seek';
    } else {
      return false;
    }
  }
  if (swipe.mode === 'dismiss') {
    const downY = Math.max(0, dy);
    const amplifiedDownY = downY * 1.22;
    const dismissProgress = clampLightboxValue(amplifiedDownY / swipe.height, 0, 1);
    swipe.dismissProgress = dismissProgress;
    setLightboxDismissGesture(dismissProgress, amplifiedDownY);
    setLightboxUnderzoomProgress(dismissProgress);
    e.preventDefault();
    e.stopPropagation();
    return true;
  }
  if (swipe.mode === 'volume') {
    const sensitivity = normalizeVideoVolumeSwipeSensitivity(state.videoVolumeSwipeSensitivity, 100) / 100;
    const delta = (-dy / Math.max(260, swipe.height * 0.72)) * sensitivity;
    setVideoVolumeValue(swipe.startVolume + delta, video);
    e.preventDefault();
    e.stopPropagation();
    return true;
  }
  const duration = Number(video.duration) || 0;
  const seekRange = Number.isFinite(duration) && duration > 0
    ? Math.max(12, Math.min(90, duration * 0.18))
    : 30;
  const deltaSeconds = dx / swipe.width * seekRange;
  seekVideoToTime(video, swipe.startTime + deltaSeconds);
  e.preventDefault();
  e.stopPropagation();
  return true;
}
function endVideoSurfaceSwipe(e) {
  const swipe = state.videoSurfaceSwipe;
  if (!swipe || swipe.pointerId !== e.pointerId) return false;
  state.videoSurfaceSwipe = null;
  const body = $('.lightbox-body');
  if (body && body.releasePointerCapture) {
    try { body.releasePointerCapture(e.pointerId); } catch (_) {}
  }
  if (!swipe.dragging) return false;
  state.videoProgressClickSuppressUntil = Date.now() + 520;
  if (swipe.mode === 'dismiss') {
    const cancelled = e.type === 'pointercancel';
    const shouldExit = !cancelled && (Number(swipe.dismissProgress) || 0) >= 0.24;
    if (shouldExit) {
      state.lightboxSwipeClickSuppressUntil = Date.now() + 420;
      clearLightboxDismissGesture();
      startLightboxUnderzoomExit();
    } else {
      clearLightboxDismissGesture({ animate: true });
    }
  } else if (swipe.mode === 'volume') showVolumeOverlay();
  else showVideoProgressActivity();
  e.preventDefault();
  e.stopPropagation();
  return true;
}
function saveCurrentVideoResumePosition({ quiet = false } = {}) {
  return false;
}
function handleVideoResumeTimeUpdate(video) {
  return;
}
function restoreVideoResumePosition(video, photo) {
  if (!video || !isVideoMedia(photo)) return;
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(video, photo);
}
function requestVideoPlaybackIndicatorSync(photo) {
  if (!isVideoMedia(photo) || !photo || !photo.id) return;
  void loadVideoPlaybackPreference(photo).then(() => {
    updateVideoBookmarkThumbIndicators(photo.id);
  }).catch(() => {});
}
function getVideoBookmarkSlotFromEvent(e) {
  if (!e || e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return null;
  if (e.location === 3) return null;
  if (e.code === 'Digit0' || e.key === '0') return 10;
  if (/^Digit[1-9]$/.test(e.code || '')) return Number(e.code.slice(5));
  if (/^[1-9]$/.test(e.key || '')) return Number(e.key);
  return null;
}
function handleVideoBookmarkShortcut(e) {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!isVideoMedia(photo) || !video || video.classList.contains('hidden')) return false;
  const slot = getVideoBookmarkSlotFromEvent(e);
  if (!slot) return false;
  const bookmark = getVideoBookmarkSlot(photo, slot);
  e.preventDefault();
  e.stopImmediatePropagation();
  if (bookmark) {
    video.currentTime = bookmark.time;
    showToast(copyText('app.interface.d_87eed94c06', { v0: (slot), v1: (bookmark.name ? `：${bookmark.name}` : '') }));
    updateVideoBookmarkProgress(video, photo);
  } else if (canEditBookmarks() && setVideoBookmarkSlot(photo, slot, video.currentTime || 0)) {
    showToast(copyText('app.interface.d_9dd0a13c2e', { v0: (slot) }));
  } else if (canEditBookmarks()) {
    showToast(copyText('app.interface.s_d92ab6cef4'));
  }
  return true;
}

function initGridScale() {
  state.gridSize = 180;
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  applyFixedTouchGridLayout();
  updateGridScaleProgress();
}
function setGridScale(size) {
  if (isFixedTouchGridLayout()) {
    applyFixedTouchGridLayout();
    updateGridScaleButtonTitle();
    return;
  }
  const anchorPhotoID = isMobileLayout() ? currentViewportCenterPhotoID() : 0;
  state.gridSize = Math.min(260, Math.max(72, Number(size) || 180));
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  updateGridScaleProgress();
  updateGridScaleButtonTitle();
  if (anchorPhotoID) queueGridScaleViewportRecenter(anchorPhotoID);
}
function updateGridScaleProgress() {
  const min = 72;
  const max = 260;
  const value = Math.min(max, Math.max(min, Number(state.gridSize) || 180));
  const progress = ((value - min) / (max - min)) * 100;
  document.documentElement.style.setProperty('--grid-scale-progress', `${progress}%`);
  updateRangeProgress($('#floating-grid-scale-input'));
}
function updateRangeProgress(input) {
  if (!input || input.type !== 'range') return;
  const min = Number(input.min || 0);
  const max = Number(input.max || 100);
  const value = Math.min(max, Math.max(min, Number(input.value) || min));
  const progress = max > min ? ((value - min) / (max - min)) * 100 : 100;
  input.style.setProperty('--range-progress', `${progress}%`);
}
function syncRangeProgress(scope = document) {
  $$('input[type="range"]', scope).forEach(updateRangeProgress);
}
function normalizeRangeValueForStep(input, rawValue) {
  const min = Number(input.min || 0);
  const max = Number(input.max || 100);
  const stepAttr = String(input.step || '').trim();
  const step = stepAttr && stepAttr !== 'any' ? Math.abs(Number(stepAttr)) || 1 : 0;
  let value = Math.min(max, Math.max(min, Number(rawValue) || min));
  if (step > 0) {
    value = min + Math.round((value - min) / step) * step;
    const decimals = Math.max(0, (String(step).split('.')[1] || '').length);
    value = Number(value.toFixed(decimals));
  }
  return Math.min(max, Math.max(min, value));
}
function setRangeValueFromPointer(input, clientX, { commit = false } = {}) {
  if (!input || input.type !== 'range' || input.disabled) return false;
  const rect = input.getBoundingClientRect();
  if (!rect.width) return false;
  const min = Number(input.min || 0);
  const max = Number(input.max || 100);
  const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
  const next = normalizeRangeValueForStep(input, min + (max - min) * ratio);
  if (input.value !== String(next)) {
    input.value = String(next);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  } else {
    updateRangeProgress(input);
  }
  if (commit) input.dispatchEvent(new Event('change', { bubbles: true }));
  return true;
}
function updateGridScaleButtonTitle() {
  const btn = $('#grid-scale-btn');
  const fixedColumns = fixedTouchGridColumns();
  const label = fixedColumns > 0 ? copyText('app.interface.d_c67c918177', { v0: (fixedColumns) }) : copyText('app.interface.d_5e4f5cc905', { v0: (state.gridSize) });
  if (btn) {
    btn.title = label;
    btn.setAttribute('aria-label', label);
  }
  const input = $('#floating-grid-scale-input');
  if (input && input.value !== String(state.gridSize)) input.value = String(state.gridSize);
  updateRangeProgress(input);
}

function photoThumbDistanceFromViewportCenter(thumb) {
  if (!thumb) return Infinity;
  const rect = thumb.getBoundingClientRect();
  const centerX = window.innerWidth / 2;
  const centerY = window.innerHeight / 2;
  const thumbCenterX = rect.left + rect.width / 2;
  const thumbCenterY = rect.top + rect.height / 2;
  return Math.hypot(thumbCenterX - centerX, thumbCenterY - centerY);
}

function currentViewportCenterPhotoID() {
  const visible = visiblePhotoThumbs();
  if (!visible.length) return 0;
  let best = null;
  let bestDistance = Infinity;
  visible.forEach(thumb => {
    const distance = photoThumbDistanceFromViewportCenter(thumb);
    if (distance < bestDistance) {
      best = thumb;
      bestDistance = distance;
    }
  });
  return Number(best && best.dataset && best.dataset.id || 0);
}

function firstVisiblePhotoThumb() {
  const thumbs = visiblePhotoThumbs();
  if (!thumbs.length) return null;
  const viewportTop = 0;
  let best = null;
  let bestOffset = Infinity;
  thumbs.forEach(thumb => {
    const rect = thumb.getBoundingClientRect();
    const offset = Math.abs(rect.top - viewportTop);
    if (offset < bestOffset) {
      best = thumb;
      bestOffset = offset;
    }
  });
  return best || thumbs[0] || null;
}

function timelinePlaceholderCount() {
  return Math.max(0, Number(state.timelineLocateMissingBeforeCount) || 0);
}

function albumPlaceholderCount() {
  return Math.max(0, Number(state.albumLocateMissingBeforeCount) || 0);
}

function makeLocatePlaceholder(kind, index) {
  const div = el('div', 'photo-thumb timeline-placeholder-thumb thumb-loading');
  div.dataset.placeholder = kind;
  div.dataset.placeholderIndex = String(index);
  div.setAttribute('aria-hidden', 'true');
  return div;
}

function makeTimelinePlaceholder(index) {
  return makeLocatePlaceholder('timeline', index);
}

function makeAlbumPlaceholder(index) {
  return makeLocatePlaceholder('album', index);
}

function clearTimelinePlaceholders() {
  state.timelineLocateMissingBeforeCount = 0;
  $$('#timeline-grid .timeline-placeholder-thumb').forEach(node => node.remove());
}

function clearAlbumPlaceholders() {
  state.albumLocateMissingBeforeCount = 0;
  $$('#album-grid .timeline-placeholder-thumb').forEach(node => node.remove());
}

function queueGridScaleViewportRecenter(photoID) {
  const targetID = Number(photoID || 0);
  if (!targetID) return;
  clearTimeout(state.gridScaleViewportRecenterTimer);
  state.gridScaleViewportRecenterTimer = setTimeout(() => {
    state.gridScaleViewportRecenterTimer = null;
    const thumb = findPhotoThumb(targetID);
    if (!thumb) return;
    thumb.scrollIntoView({ behavior: 'auto', block: 'center', inline: 'center' });
  }, 34);
}

function cycleGridScale(direction = 1) {
  if (isFixedTouchGridLayout()) return;
  const presets = [72, 88, 104, 120, 136, 152, 168, 180, 196, 212, 228, 244, 260];
  const current = Math.min(260, Math.max(72, Number(state.gridSize) || 180));
  let index = presets.findIndex(value => value >= current);
  if (index < 0) index = presets.length - 1;
  if (presets[index] !== current && direction < 0) index -= 1;
  else if (presets[index] === current) index += direction;
  if (index >= presets.length) index = 0;
  if (index < 0) index = presets.length - 1;
  setGridScale(presets[index]);
  persistGridScaleSoon();
  updateGridScaleButtonTitle();
}
function persistGridScaleSoon() {
  if (!state.settingsReady) return;
  if (state.gridScalePersistTimer) clearTimeout(state.gridScalePersistTimer);
  state.gridScalePersistTimer = setTimeout(() => {
    state.gridScalePersistTimer = null;
    persistSettings()
      .then(() => {})
      .catch(e => console.error("保存缩放级别失败:", e));
  }, 500);
}
function initDisplaySettings() {
  state.gridGap = 2;
  state.thumbRadius = 2;
  applyDisplaySettings();
}
function applyDisplaySettings() {
  document.documentElement.style.setProperty('--grid-gap', state.gridGap + 'px');
  document.documentElement.style.setProperty('--thumb-radius', state.thumbRadius + 'px');
}
function setGridGap(size) {
  state.gridGap = Math.min(24, Math.max(0, Number(size) || 0));
  applyDisplaySettings();
}
function setThumbRadius(size) {
  state.thumbRadius = Math.min(24, Math.max(0, Number(size) || 0));
  applyDisplaySettings();
}
function initSidebarPreference() {
  const saved = localStorage.getItem('echogallery_sidebar_compact');
  state.sidebarCompact = saved == null ? true : saved !== '0';
  if (saved == null) localStorage.setItem('echogallery_sidebar_compact', '1');
}
function isMobileLayout() {
  return window.matchMedia('(max-width: 640px)').matches;
}
function isLandscapeOrientation() {
  return window.matchMedia('(orientation: landscape)').matches;
}
function isTouchLikeDevice() {
  return window.matchMedia('(hover: none), (pointer: coarse)').matches;
}
function isIPadLayout() {
  return document.documentElement.classList.contains('is-ipad-device');
}
function isPhoneTouchLayout() {
  return isTouchLikeDevice() && !isIPadLayout();
}
function fixedTouchGridColumns() {
  if (isIPadLayout()) return isLandscapeOrientation() ? 7 : 5;
  if (isPhoneTouchLayout()) return isLandscapeOrientation() ? 5 : 3;
  return 0;
}
function isFixedTouchGridLayout() {
  return fixedTouchGridColumns() > 0;
}
function applyFixedTouchGridLayout() {
  const root = document.documentElement;
  const columns = fixedTouchGridColumns();
  root.classList.toggle('fixed-touch-grid', columns > 0);
  if (columns > 0) root.style.setProperty('--photo-grid-columns', String(columns));
  else root.style.removeProperty('--photo-grid-columns');
}
function touchDistance(touches) {
  if (!touches || touches.length < 2) return 0;
  const a = touches[0];
  const b = touches[1];
  return Math.hypot((a.clientX || 0) - (b.clientX || 0), (a.clientY || 0) - (b.clientY || 0));
}
function touchMidpoint(touches) {
  if (!touches || touches.length < 2) return { x: 0, y: 0 };
  return {
    x: ((touches[0].clientX || 0) + (touches[1].clientX || 0)) / 2,
    y: ((touches[0].clientY || 0) + (touches[1].clientY || 0)) / 2,
  };
}
function isPhotoWallPinchView() {
  return ['timeline', 'favorites', 'random-album', 'album-detail', 'memories', 'trash'].includes(state.view);
}
function pinchContextForTouches(touches) {
  if (!touches || touches.length < 2) return '';
  if ($('#lightbox')?.classList.contains('open')) {
    const photo = state.lightboxPhotos[state.lightboxIndex];
    return photo && isVideoMedia(photo) ? 'blocked' : 'lightbox';
  }
  if (state.searchOpen || state.blockingInteraction || document.querySelector('.modal-overlay.open')) return 'blocked';
  if (isPhotoWallPinchView()) return 'grid';
  return 'blocked';
}
function beginTouchPinch(e) {
  if (!e || !e.touches || e.touches.length < 2) return false;
  const context = pinchContextForTouches(e.touches);
  if (context === 'grid' && isFixedTouchGridLayout()) {
    e.preventDefault();
    state.pinchGesture = {
      context: 'blocked',
      startDistance: touchDistance(e.touches),
      startGridSize: state.gridSize,
      startLightboxZoom: state.lightboxZoom,
    };
    return true;
  }
  const distance = touchDistance(e.touches);
  if (distance <= 0) return false;
  if (context === 'blocked') {
    state.pinchGesture = {
      context,
      startDistance: distance,
      startGridSize: state.gridSize,
      startLightboxZoom: state.lightboxZoom,
    };
    e.preventDefault();
    return true;
  }
  state.pinchGesture = {
    context,
    startDistance: distance,
    startGridSize: state.gridSize,
    startLightboxZoom: state.lightboxBoostActive ? 300 : state.lightboxZoom,
  };
  if (context === 'lightbox') {
    state.lightboxSwipe = null;
    state.videoSurfaceSwipe = null;
    resetLightboxSwipeVisual();
    const point = touchMidpoint(e.touches);
    updateLightboxFocusPointFromPointer(point.x, point.y);
  }
  e.preventDefault();
  return true;
}
function updateTouchPinch(e) {
  if (!e || !e.touches || e.touches.length < 2) return false;
  if (!state.pinchGesture) beginTouchPinch(e);
  const gesture = state.pinchGesture;
  if (!gesture || !gesture.startDistance) return false;
  const ratio = touchDistance(e.touches) / gesture.startDistance;
  if (gesture.context === 'lightbox') {
    const point = touchMidpoint(e.touches);
    const nextZoom = Math.round((gesture.startLightboxZoom || 100) * ratio);
    const photo = state.lightboxPhotos[state.lightboxIndex];
    updateLightboxFocusPointFromPointer(point.x, point.y);
    if (photo && !isVideoMedia(photo) && nextZoom < 100) {
      setLightboxUnderzoomProgress(lightboxUnderzoomProgressForZoom(nextZoom));
    } else {
      clearLightboxUnderzoomState();
    }
    setLightboxZoom(nextZoom, { fromPinch: true });
  } else if (gesture.context === 'grid') {
    if (isFixedTouchGridLayout()) return false;
    setGridScale(Math.round((gesture.startGridSize || 180) * ratio));
    persistGridScaleSoon();
  }
  e.preventDefault();
  return true;
}
function endTouchPinch(e) {
  if (!state.pinchGesture) return false;
  if (e && e.touches && e.touches.length >= 2) return false;
  const gesture = state.pinchGesture;
  const currentZoom = Number(state.lightboxZoom) || 100;
  state.pinchGesture = null;
  if (gesture && gesture.context === 'lightbox') {
    const photo = state.lightboxPhotos[state.lightboxIndex];
    if (photo && !isVideoMedia(photo)) {
      if (currentZoom < 75) {
        startLightboxUnderzoomExit();
        return true;
      }
      if (currentZoom < 100) {
        clearLightboxUnderzoomState();
        setLightboxFit();
      } else {
        clearLightboxUnderzoomState();
      }
    }
  }
  return true;
}
function syncSidebarUI() {
  const app = $('#app');
  const nav = $('#main-nav');
  if (!app || !nav) return;

  app.classList.toggle('sidebar-compact', !!state.sidebarCompact && !isMobileLayout());
}

function initSlideshowSettings() {
  state.slideshowMode = 'random';
  state.slideshowLoop = true;
  state.slideshowInterval = 5000;
}
function stopSlideshow() {
  if (state.slideshowTimer) {
    clearTimeout(state.slideshowTimer);
    state.slideshowTimer = null;
  }
  state.slideshowPlaying = false;
  updateSlideshowControls();
}
function updateSlideshowControls() {
  const btn = $('#lb-slideshow-toggle');
  const interval = $('#lb-slideshow-interval');
  const intervalValue = $('#lb-slideshow-interval-value');
  const loop = $('#lb-slideshow-loop');
  if (btn) {
    const iconEl = btn.querySelector('.lightbox-play-icon');
    if (iconEl) iconEl.innerHTML = state.slideshowPlaying ? icons.pause : icons.play;
    btn.title = state.slideshowPlaying ? copyText('app.interface.s_b35d51177b') : copyText('app.interface.s_b8b6abde77');
    btn.setAttribute('aria-label', btn.title);
    btn.classList.toggle('active', state.slideshowPlaying);
  }
  if (interval) {
    interval.value = String(state.slideshowInterval / 1000);
    updateRangeProgress(interval);
  }
  if (intervalValue) intervalValue.textContent = copyText('app.interface.commonSeconds', { v0: (state.slideshowInterval / 1000) });
  if (loop) {
    loop.innerHTML = icons.slideshowLoop;
    loop.title = state.slideshowLoop ? copyText('app.interface.s_3a6ac6d5e4') : copyText('app.interface.s_7db84b247a');
    loop.setAttribute('aria-label', loop.title);
    loop.setAttribute('aria-pressed', state.slideshowLoop ? 'true' : 'false');
    loop.classList.toggle('active', state.slideshowLoop);
  }
  const lightbox = $('#lightbox');
  if (lightbox) lightbox.classList.toggle('slideshow-active', state.slideshowPlaying);
}
function resetRandomQueue() {
  const pool = state.lightboxPhotos
    .map((_, idx) => idx)
    .filter(idx => idx !== state.lightboxIndex);
  for (let i = pool.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }
  state.slideshowRandomQueue = pool;
}
function appendNewRandomQueueIndexes(startIndex, endIndex) {
  if (state.slideshowMode !== 'random' || !state.slideshowPlaying) return;
  const fresh = [];
  for (let idx = Math.max(0, startIndex); idx < endIndex; idx += 1) {
    if (idx !== state.lightboxIndex && !state.slideshowRandomQueue.includes(idx)) fresh.push(idx);
  }
  for (let i = fresh.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [fresh[i], fresh[j]] = [fresh[j], fresh[i]];
  }
  state.slideshowRandomQueue.push(...fresh);
}
function shouldPreloadMoreForSlideshow() {
  if (!state.slideshowPlaying || !lightboxCanLoadMoreForward()) return false;
  if (state.slideshowMode === 'random') return state.slideshowRandomQueue.length <= 1;
  return state.lightboxPhotos.length - state.lightboxIndex <= 1;
}
async function preloadMoreForSlideshow({ pages = 1, minItems = 0 } = {}) {
  if (state.slideshowPreloadPromise) return state.slideshowPreloadPromise;
  state.slideshowPreloadPromise = (async () => {
    let loadedPages = 0;
    const initialLength = state.lightboxPhotos.length;
    while (loadedPages < pages && lightboxCanLoadMoreForward()) {
      const loaded = await loadMoreForLightbox();
      if (!loaded) break;
      loadedPages += 1;
      if (minItems > 0 && state.lightboxPhotos.length - initialLength >= minItems) break;
    }
    updateSlideshowControls();
    return state.lightboxPhotos.length > initialLength;
  })().catch(e => {
    console.warn("幻灯片预加载更多媒体失败", e);
    return false;
  }).finally(() => {
    state.slideshowPreloadPromise = null;
  });
  return state.slideshowPreloadPromise;
}
function ensureSlideshowHasMoreSoon({ force = false, pages = 2 } = {}) {
  if (!state.slideshowPlaying) return;
  if (document.hidden || !lightboxIsInteractive()) return;
  if (!force && !shouldPreloadMoreForSlideshow()) return;
  void preloadMoreForSlideshow({ pages });
}
function getNextSlideshowIndex() {
  const total = state.lightboxPhotos.length;
  if (total <= 1) return state.slideshowLoop ? state.lightboxIndex : null;

  if (state.slideshowMode === 'random') {
    if (!state.slideshowRandomQueue.length) {
      if (!state.slideshowLoop) return null;
      resetRandomQueue();
    }
    return state.slideshowRandomQueue.shift() ?? null;
  }

  const next = state.lightboxIndex + 1;
  if (next < total) return next;
  if (state.slideshowMode !== 'random' && lightboxCanLoadMoreForward()) return total;
  return state.slideshowLoop ? 0 : null;
}
function scheduleSlideshowStep() {
  if (!state.slideshowPlaying) return;
  if (state.slideshowTimer) clearTimeout(state.slideshowTimer);
  state.slideshowTimer = setTimeout(async () => {
    const nextIndex = getNextSlideshowIndex();
    if (nextIndex == null) {
      stopSlideshow();
      return;
    }
    await lbGoTo(nextIndex, { fromSlideshow: true });
  }, state.slideshowInterval);
}
function startSlideshow() {
  if (!state.lightboxPhotos.length) return;
  state.slideshowPlaying = true;
  if (state.slideshowMode === 'random' && !state.slideshowRandomQueue.length) resetRandomQueue();
  updateSlideshowControls();
  ensureSlideshowHasMoreSoon({ force: true, pages: 1 });
  scheduleSlideshowStep();
}
function toggleSlideshow() {
  if (state.slideshowPlaying) stopSlideshow();
  else startSlideshow();
}
function shouldStopSlideshowFromClick(target) {
  if (!state.slideshowPlaying || !target) return false;
  if (!target.closest('#lightbox.open')) return false;
  if (target.closest('.lightbox-header, .lightbox-info, .lb-nav, .lb-touch-playback-toggle, .context-menu, button, input, select, label')) return false;
  return !!target.closest('.lightbox-body');
}
function shouldToggleLightboxUiFromClick(target) {
  if (!target || !target.closest('#lightbox.open')) return false;
  if (target.closest([
    '.lightbox-header',
    '.lightbox-info',
    '.lightbox-video-progress',
    '.lightbox-volume-feedback',
    '.lb-nav',
    '.lb-touch-playback-toggle',
    '.context-menu',
    '.modal-overlay',
    'button',
    'input',
    'select',
    'textarea',
    'label',
    'a',
  ].join(','))) return false;
  return !!target.closest('.lightbox-body, .lightbox-media-frame, #lb-img, #lb-video');
}
function toggleLightboxUiIdle() {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open')) return;
  if (lightboxUIIdleSeconds() > 0) {
    refreshLightboxUiActivity();
    return;
  }
  lightbox.classList.toggle('ui-idle');
}
function updateSlideshowSetting(name, value) {
  if (name === 'mode') {
    state.slideshowMode = value === 'random' ? 'random' : 'sequential';
    resetRandomQueue();
  }
  if (name === 'interval') {
    state.slideshowInterval = Math.min(30000, Math.max(1000, value));
  }
  if (name === 'loop') {
    state.slideshowLoop = !!value;
  }
  updateSlideshowControls();
  if (state.slideshowPlaying) scheduleSlideshowStep();
}
function initLightboxZoom() {
  state.lightboxZoom = 100;
  state.lightboxZoomMode = 'fit';
}
function resetLightboxZoomToDefault() {
  state.lightboxPinchZoomActive = false;
  state.lightboxZoom = 100;
  state.lightboxZoomMode = 'fit';
}
function currentLightboxViewportOrientation() {
  const width = window.visualViewport ? window.visualViewport.width : window.innerWidth;
  const height = window.visualViewport ? window.visualViewport.height : window.innerHeight;
  return width >= height ? 'landscape' : 'portrait';
}
function syncLightboxViewportGeometry() {
  const width = window.visualViewport ? Number(window.visualViewport.width) || 0 : Number(window.innerWidth) || 0;
  const isPortraitMobile = width > 0 && width <= 900 && currentLightboxViewportOrientation() === 'portrait';
  if (!isPortraitMobile) {
    document.documentElement.style.removeProperty('--eg-lightbox-viewport-height');
    return;
  }
  const measuredSafeArea = measuredSafeAreaInsets();
  const visualHeight = window.visualViewport ? Number(window.visualViewport.height) || 0 : 0;
  const layoutHeight = Number(window.innerHeight) || 0;
  const clientHeight = Number(document.documentElement.clientHeight) || 0;
  const standaloneHeight = (window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone)
    ? Number(window.screen && window.screen.height) || 0
    : 0;
  const fullHeight = Math.max(
    layoutHeight,
    clientHeight,
    visualHeight + measuredSafeArea.top + measuredSafeArea.bottom,
    standaloneHeight,
    1,
  );
  document.documentElement.style.setProperty('--eg-lightbox-viewport-height', `${Math.ceil(fullHeight)}px`);
}
function resetLightboxZoomForViewportChange(force = false) {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open')) return;
  syncLightboxViewportGeometry();
  const orientation = currentLightboxViewportOrientation();
  const changed = state.lightboxViewportOrientation && state.lightboxViewportOrientation !== orientation;
  state.lightboxViewportOrientation = orientation;
  if (force || changed) {
    state.lightboxPinchZoomActive = false;
    clearLightboxUnderzoomState();
    resetLightboxFocusPoint();
    resetLightboxTemporaryZoom();
    setLightboxFit();
  } else {
    applyLightboxZoom();
  }
  updateLightboxHeaderLayout();
}
function scheduleLightboxViewportChangeCheck(force = false) {
  if (!$('#lightbox')?.classList.contains('open')) return;
  clearTimeout(state.lightboxViewportResizeTimer);
  state.lightboxViewportResizeTimer = window.setTimeout(() => resetLightboxZoomForViewportChange(force), 120);
}
function resetLightboxFocusPoint() {
  state.lightboxFocusPoint = {
    mediaX: 0.5,
    mediaY: 0.5,
    viewportX: 0.5,
    viewportY: 0.5,
  };
  state.lightboxPointerClientX = 0;
  state.lightboxPointerClientY = 0;
  state.lightboxPointerInside = false;
}
function resetLightboxTemporaryZoom() {
  syncLightboxTemporaryZoom(false);
}
function lightboxUnderzoomProgressForZoom(zoom) {
  return clampLightboxValue((100 - Math.max(0, Number(zoom) || 0)) / 100, 0, 1);
}
function lightboxUnderzoomVisualProgress(progress) {
  const next = clampLightboxValue(Number(progress) || 0, 0, 1);
  return Math.pow(next, 0.92);
}
function setLightboxUnderzoomProgress(progress, options = {}) {
  const lightbox = $('#lightbox');
  if (!lightbox) return;
  const next = clampLightboxValue(Number(progress) || 0, 0, 1);
  const visualProgress = lightboxUnderzoomVisualProgress(next);
  const exiting = !!options.exiting && next > 0.001;
  const visibleRatio = options.keepOverlayTransparent
    ? 0
    : Math.max(0, 1 - Math.pow(next, exiting ? 0.58 : 0.68));
  state.lightboxUnderzoomProgress = next;
  lightbox.classList.toggle('lightbox-underzooming', next > 0.001);
  lightbox.classList.toggle('lightbox-underzoom-exiting', exiting);
  lightbox.style.setProperty('--lightbox-underzoom-progress', visualProgress.toFixed(4));
  lightbox.style.setProperty('--lightbox-underzoom-controls-opacity', visibleRatio.toFixed(4));
  lightbox.style.setProperty('--lightbox-underzoom-overlay-alpha', visibleRatio.toFixed(4));
  lightbox.style.setProperty('--lightbox-underzoom-media-opacity', '1');
}
function setLightboxDismissGesture(progress, translateY = 0) {
  const lightbox = $('#lightbox');
  if (!lightbox) return;
  const next = clampLightboxValue(Number(progress) || 0, 0, 1);
  const offset = Math.max(0, Number(translateY) || 0);
  state.lightboxDismissProgress = next;
  lightbox.classList.toggle('lightbox-dismiss-dragging', next > 0.001);
  lightbox.style.setProperty('--lightbox-dismiss-progress', next.toFixed(4));
  lightbox.style.setProperty('--lightbox-dismiss-translate-y', `${offset.toFixed(2)}px`);
}
function clearLightboxDismissGesture(options = {}) {
  const lightbox = $('#lightbox');
  clearTimeout(state.lightboxDismissResetTimer);
  state.lightboxDismissResetTimer = null;
  state.lightboxDismissProgress = 0;
  if (!lightbox) return;
  if (options.animate) {
    lightbox.classList.remove('lightbox-dismiss-dragging');
    lightbox.style.setProperty('--lightbox-dismiss-progress', '0');
    lightbox.style.setProperty('--lightbox-dismiss-translate-y', '0px');
    setLightboxUnderzoomProgress(0);
    state.lightboxDismissResetTimer = window.setTimeout(() => {
      state.lightboxDismissResetTimer = null;
      lightbox.style.removeProperty('--lightbox-dismiss-progress');
      lightbox.style.removeProperty('--lightbox-dismiss-translate-y');
      clearLightboxUnderzoomState();
    }, 220);
    return;
  }
  lightbox.classList.remove('lightbox-dismiss-dragging');
  lightbox.style.removeProperty('--lightbox-dismiss-progress');
  lightbox.style.removeProperty('--lightbox-dismiss-translate-y');
}
function clearLightboxUnderzoomState() {
  clearTimeout(state.lightboxUnderzoomExitTimer);
  state.lightboxUnderzoomExitTimer = null;
  state.lightboxUnderzoomProgress = 0;
  const lightbox = $('#lightbox');
  if (!lightbox) return;
  lightbox.classList.remove('lightbox-underzooming', 'lightbox-underzoom-exiting');
  lightbox.style.removeProperty('--lightbox-underzoom-progress');
  lightbox.style.removeProperty('--lightbox-underzoom-controls-opacity');
  lightbox.style.removeProperty('--lightbox-underzoom-overlay-alpha');
  lightbox.style.removeProperty('--lightbox-underzoom-media-opacity');
}
function startLightboxUnderzoomExit() {
  clearTimeout(state.lightboxUnderzoomExitTimer);
  state.lightboxUnderzoomExitTimer = null;
  const lightbox = $('#lightbox');
  const currentTranslate = lightbox
    ? Math.max(0, parseFloat(lightbox.style.getPropertyValue('--lightbox-dismiss-translate-y') || '0') || 0)
    : 0;
  const currentProgress = clampLightboxValue(
    Math.max(
      Number(state.lightboxDismissProgress) || 0,
      Number(state.lightboxUnderzoomProgress) || 0,
    ),
    0,
    1,
  );
  if (lightbox) {
    const mediaRect = currentLightboxMediaRect();
    const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 1;
    const mediaHeight = mediaRect && mediaRect.height > 0 ? mediaRect.height : viewportHeight;
    const exitDistance = Math.max(viewportHeight * 0.72, mediaHeight * 0.62, 360);
    lightbox.classList.remove('lightbox-dismiss-dragging');
    lightbox.classList.add('lightbox-underzoom-exiting');
    lightbox.style.setProperty('--lightbox-underzoom-overlay-alpha', '0');
    lightbox.style.setProperty('--lightbox-underzoom-controls-opacity', '0');
    lightbox.style.setProperty('--lightbox-dismiss-progress', String(Math.max(currentProgress, 0.72)));
    lightbox.style.setProperty('--lightbox-dismiss-translate-y', `${Math.round(currentTranslate + exitDistance)}px`);
  }
  setLightboxDismissFade(1);
  setLightboxUnderzoomProgress(1, { exiting: true, keepOverlayTransparent: true });
  state.lightboxUnderzoomExitTimer = window.setTimeout(() => {
    state.lightboxUnderzoomExitTimer = null;
    exitLightboxToContext();
  }, 180);
}
function focusLightboxKeyboardSurface() {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open')) return;
  lightbox.focus({ preventScroll: true });
}
function refocusLightboxAfterVideoControl() {
  window.setTimeout(focusLightboxKeyboardSurface, 0);
}
function lightboxEffectiveZoomState() {
  if (state.lightboxBoostActive) {
    return { zoom: 500, mode: 'scale' };
  }
  return { zoom: state.lightboxZoom, mode: state.lightboxZoomMode };
}
function setLightboxZoom(value, options = {}) {
  state.lightboxPinchZoomActive = !!options.fromPinch;
  if (!options.fromPinch) resetLightboxFocusPoint();
  const minZoom = options.fromPinch ? 0 : 50;
  state.lightboxZoom = Math.min(500, Math.max(minZoom, Number(value) || 0));
  state.lightboxZoomMode = 'scale';
  if (!options.fromPinch) clearLightboxUnderzoomState();
  applyLightboxZoom();
}
function stepLightboxZoom(delta) {
  const current = Number(state.lightboxZoom) || 100;
  setLightboxZoom(current + (Number(delta) || 0));
}
function setLightboxFit() {
  resetLightboxZoomToDefault();
  applyLightboxZoom();
}
function lightboxMediaNaturalSize() {
  const photo = state.lightboxPhotos[state.lightboxIndex] || {};
  const img = $('#lb-img');
  const video = $('#lb-video');
  if (!isVideoMedia(photo)) {
    return {
      width: Number(photo.width) || (img && img.naturalWidth) || 0,
      height: Number(photo.height) || (img && img.naturalHeight) || 0,
    };
  }
  const videoWidth = Number(video && video.videoWidth) || 0;
  const videoHeight = Number(video && video.videoHeight) || 0;
  if (videoWidth > 0 && videoHeight > 0) {
    return {
      width: videoWidth,
      height: videoHeight,
    };
  }
  return {
    width: Number(photo.width) || 0,
    height: Number(photo.height) || 0,
  };
}
function readCSSPixelValue(value) {
  const parsed = parseFloat(value || '0');
  return Number.isFinite(parsed) ? parsed : 0;
}
function measuredSafeAreaInsets() {
  let probe = $('#eg-safe-area-probe');
  let maxProbe = $('#eg-safe-area-max-probe');
  if (!probe) {
    probe = document.createElement('div');
    probe.id = 'eg-safe-area-probe';
    probe.style.cssText = [
      'position:fixed',
      'inset:0',
      'visibility:hidden',
      'pointer-events:none',
      'padding-top:env(safe-area-inset-top, 0px)',
      'padding-bottom:env(safe-area-inset-bottom, 0px)',
      'width:0',
      'height:0',
      'overflow:hidden',
    ].join(';');
    document.body.appendChild(probe);
  }
  if (!maxProbe) {
    maxProbe = document.createElement('div');
    maxProbe.id = 'eg-safe-area-max-probe';
    maxProbe.style.cssText = [
      'position:fixed',
      'inset:0',
      'visibility:hidden',
      'pointer-events:none',
      'padding-top:env(safe-area-max-inset-top, 0px)',
      'padding-bottom:env(safe-area-max-inset-bottom, 0px)',
      'width:0',
      'height:0',
      'overflow:hidden',
    ].join(';');
    document.body.appendChild(maxProbe);
  }
  const style = window.getComputedStyle(probe);
  const maxStyle = window.getComputedStyle(maxProbe);
  return {
    top: readCSSPixelValue(style.paddingTop),
    bottom: readCSSPixelValue(style.paddingBottom),
    maxTop: readCSSPixelValue(maxStyle.paddingTop),
    maxBottom: readCSSPixelValue(maxStyle.paddingBottom),
  };
}
function lightboxViewportMetrics(body) {
  if (!body) return { width: 0, height: 0 };
  const style = window.getComputedStyle(body);
  const rootStyle = window.getComputedStyle(document.documentElement);
  const paddingLeft = readCSSPixelValue(style.paddingLeft);
  const paddingRight = readCSSPixelValue(style.paddingRight);
  const paddingTop = readCSSPixelValue(style.paddingTop);
  const paddingBottom = readCSSPixelValue(style.paddingBottom);
  const measuredSafeArea = measuredSafeAreaInsets();
  const safeTop = readCSSPixelValue(rootStyle.getPropertyValue('--eg-safe-area-top')) || measuredSafeArea.top;
  const safeBottom = readCSSPixelValue(rootStyle.getPropertyValue('--eg-safe-area-bottom')) || measuredSafeArea.bottom;
  return {
    width: Math.max(1, body.clientWidth - paddingLeft - paddingRight),
    height: Math.max(1, body.clientHeight - paddingTop - paddingBottom),
    paddingLeft,
    paddingTop,
    paddingRight,
    paddingBottom,
    safeTop,
    safeBottom,
  };
}
function lightboxViewportSize(body) {
  const { width, height } = lightboxViewportMetrics(body);
  return { width, height };
}
function currentLightboxMediaElement() {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  if (!photo) return null;
  return isVideoMedia(photo) ? $('#lb-video') : $('#lb-img');
}
function lightboxSwipePreviewURL(photo, { current = false } = {}) {
  if (!photo) return '';
  if (isVideoMedia(photo)) return mediaThumbURL(photo);
  if (current) {
    const img = $('#lb-img');
    if (img && img.currentSrc && img.complete) return img.currentSrc;
  }
  return mediaFileURL(photo);
}
function setLightboxSwipePreviewImage(selector, photo, options = {}) {
  const img = $(selector);
  if (!img) return;
  const isCurrent = !!options.current;
  const wantsPosterOnly = !!photo && isVideoMedia(photo);
  const fallbackURL = photo && !isCurrent && !wantsPosterOnly
    ? (currentLoadedPreviewURLForPhoto(photo) || mediaThumbURL(photo))
    : '';
  const url = fallbackURL || lightboxSwipePreviewURL(photo, options);
  img.classList.toggle('is-empty', !url);
  if (!url) {
    img.removeAttribute('src');
    return;
  }
  img.onerror = () => {
    if (isVideoMedia(photo)) {
      img.onerror = null;
      img.src = videoPosterPlaceholder;
      return;
    }
    img.classList.add('is-empty');
  };
  if (img.src !== url) img.src = url;
  if (photo && !isCurrent && !wantsPosterOnly) {
    const fullURL = mediaFileURL(photo);
    const loader = new Image();
    loader.decoding = 'async';
    loader.onload = () => {
      if (!$('#lb-swipe-strip')?.classList.contains('active')) return;
      if (img.dataset.photoId !== String(photo.id || photo.uuid || '')) return;
      img.src = fullURL;
    };
    img.dataset.photoId = String(photo.id || photo.uuid || '');
    loader.src = fullURL;
  } else {
    img.dataset.photoId = String(photo && (photo.id || photo.uuid) || '');
  }
}
const lightboxSwipeAnimationMs = 380;

function lightboxSwipeDividerTranslate(x, width, direction) {
  if (direction > 0) return x + width / 2;
  if (direction < 0) return x - width / 2;
  return 0;
}
function setLightboxSwipePreviewTransform(x, animate = false, direction = 0) {
  const strip = $('#lb-swipe-strip');
  if (!strip) return;
  const width = Math.max(1, Number(strip.dataset.swipeWidth) || strip.clientWidth || window.innerWidth || 1);
  strip.classList.toggle('animating', !!animate);
  strip.classList.toggle('show-divider', !!direction && Math.abs(x) > 6);
  strip.style.setProperty('--lightbox-swipe-divider-x', `${Math.round(lightboxSwipeDividerTranslate(x, width, direction))}px`);
  [
    ['#lb-swipe-prev', -1],
    ['#lb-swipe-current', 0],
    ['#lb-swipe-next', 1],
  ].forEach(([selector, offset]) => {
    const item = $(selector);
    if (item) item.style.transform = `translate3d(${Math.round(x + offset * width)}px, 0, 0)`;
  });
}
function setupLightboxSwipePreview(swipe) {
  const strip = $('#lb-swipe-strip');
  if (!strip || !swipe) return;
  strip.dataset.swipeWidth = String(Math.max(1, Number(swipe.width) || window.innerWidth || 1));
  setLightboxSwipePreviewImage('#lb-swipe-prev', state.lightboxPhotos[state.lightboxIndex - 1]);
  setLightboxSwipePreviewImage('#lb-swipe-current', state.lightboxPhotos[state.lightboxIndex], { current: true });
  setLightboxSwipePreviewImage('#lb-swipe-next', state.lightboxPhotos[state.lightboxIndex + 1]);
  strip.classList.add('active');
  strip.classList.remove('animating');
  setLightboxSwipePreviewTransform(0, false, 0);
}
function refreshLightboxSwipePreviewNeighbor(index) {
  const swipe = state.lightboxSwipe;
  const strip = $('#lb-swipe-strip');
  if (!swipe || !strip || !strip.classList.contains('active')) return;
  if (index === state.lightboxIndex - 1) setLightboxSwipePreviewImage('#lb-swipe-prev', state.lightboxPhotos[index]);
  if (index === state.lightboxIndex + 1) setLightboxSwipePreviewImage('#lb-swipe-next', state.lightboxPhotos[index]);
}
function resetLightboxSwipePreview({ animate = false, direction = 0 } = {}) {
  const strip = $('#lb-swipe-strip');
  if (!strip) return;
  if (animate && strip.classList.contains('active')) {
    setLightboxSwipePreviewTransform(0, true, direction);
    window.setTimeout(() => {
      strip.classList.remove('active', 'animating', 'show-divider');
      setLightboxSwipePreviewTransform(0, false, 0);
    }, lightboxSwipeAnimationMs + 40);
    return;
  }
  strip.classList.remove('active', 'animating', 'show-divider');
  strip.style.removeProperty('--lightbox-swipe-divider-x');
  setLightboxSwipePreviewTransform(0, false, 0);
}
function resetLightboxSwipeVisual({ animate = false, direction = 0 } = {}) {
  const media = currentLightboxMediaElement();
  resetLightboxSwipePreview({ animate, direction });
  if (media) {
    media.classList.toggle('lightbox-swipe-animating', !!animate);
    media.style.removeProperty('--lightbox-swipe-x');
    media.style.removeProperty('--lightbox-swipe-opacity');
    if (animate) {
      window.setTimeout(() => media.classList.remove('lightbox-swipe-active', 'lightbox-swipe-animating'), lightboxSwipeAnimationMs + 40);
    } else {
      media.classList.remove('lightbox-swipe-active', 'lightbox-swipe-animating');
    }
  }
}
function isLightboxSwipeExcludedTarget(target) {
  if (!target) return true;
  return !!target.closest('.lightbox-header, .lightbox-info, .lightbox-video-progress, .lightbox-volume-feedback, .lb-nav, .lb-touch-playback-toggle, .context-menu, button, input, select, label, a');
}
function beginLightboxSwipe(e) {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open')) return false;
  if (e.pointerType !== 'touch' && e.pointerType !== 'pen') return false;
  if (state.videoProgressScrubPointerId != null || isLightboxSwipeExcludedTarget(e.target)) return false;
  const photo = state.lightboxPhotos[state.lightboxIndex];
  if (!photo || isVideoMedia(photo)) return false;
  const body = $('.lightbox-body');
  const media = currentLightboxMediaElement();
  if (!body || !media || media.classList.contains('hidden') || body.classList.contains('zoomed')) return false;
  if (!e.target.closest('.lightbox-media-frame, #lb-img')) return false;
  state.lightboxSwipe = {
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    lastX: e.clientX,
    lastY: e.clientY,
    dragging: false,
    mode: '',
    width: Math.max(1, body.clientWidth || window.innerWidth || 1),
    height: Math.max(1, body.clientHeight || window.innerHeight || 1),
    dismissProgress: 0,
  };
  setupLightboxSwipePreview(state.lightboxSwipe);
  if (state.lightboxIndex + 1 >= state.lightboxPhotos.length && lightboxCanLoadMoreForward()) {
    void ensureLightboxIndexAvailable(state.lightboxIndex + 1)
      .then(ok => { if (ok) refreshLightboxSwipePreviewNeighbor(state.lightboxIndex + 1); })
      .catch(() => {});
  }
  if (body.setPointerCapture) {
    try { body.setPointerCapture(e.pointerId); } catch (_) {}
  }
  return true;
}
function updateLightboxSwipe(e) {
  const swipe = state.lightboxSwipe;
  if (!swipe || swipe.pointerId !== e.pointerId) return false;
  const dx = e.clientX - swipe.startX;
  const dy = e.clientY - swipe.startY;
  const absX = Math.abs(dx);
  const absY = Math.abs(dy);
  if (!swipe.dragging) {
    if (dy > 0 && absY > 10 && absY > absX * 1.06) {
      swipe.dragging = true;
      swipe.mode = 'dismiss';
    } else {
      if (absX < 8 || absX < absY * 1.04) return false;
      swipe.dragging = true;
      swipe.mode = 'horizontal';
    }
  }
  const media = currentLightboxMediaElement();
  if (!media) return false;
  if (swipe.mode === 'dismiss') {
    const downY = Math.max(0, dy);
    const amplifiedDownY = downY * 1.34;
    const dismissProgress = clampLightboxValue(amplifiedDownY / swipe.height, 0, 1);
    swipe.lastY = e.clientY;
    swipe.dismissProgress = dismissProgress;
    setLightboxDismissGesture(dismissProgress, amplifiedDownY);
    setLightboxUnderzoomProgress(dismissProgress);
    setLightboxDismissFade(dismissProgress);
    e.preventDefault();
    e.stopPropagation();
    return true;
  }
  const atStart = state.lightboxIndex <= 0 && dx > 0;
  const atEnd = state.lightboxIndex >= state.lightboxPhotos.length - 1 && !lightboxCanLoadMoreForward() && dx < 0;
  const resistance = atStart || atEnd ? 0.38 : 1;
  const x = dx * resistance;
  const opacity = Math.max(0.72, 1 - Math.min(0.24, absX / swipe.width * 0.28));
  const canRevealNeighbor = !atStart && !atEnd;
  const direction = canRevealNeighbor ? (dx < 0 ? 1 : -1) : 0;
  swipe.horizontalDirection = direction;
  swipe.lastX = e.clientX;
  setLightboxSwipePreviewTransform(x, false, direction);
  media.classList.add('lightbox-swipe-active');
  media.classList.remove('lightbox-swipe-animating');
  media.style.setProperty('--lightbox-swipe-opacity', '0');
  e.preventDefault();
  e.stopPropagation();
  return true;
}
async function endLightboxSwipe(e) {
  const swipe = state.lightboxSwipe;
  if (!swipe || swipe.pointerId !== e.pointerId) return false;
  state.lightboxSwipe = null;
  const body = $('.lightbox-body');
  if (body && body.releasePointerCapture) {
    try { body.releasePointerCapture(e.pointerId); } catch (_) {}
  }
  if (swipe.mode === 'dismiss') {
    const cancelled = e.type === 'pointercancel';
    const shouldExit = !cancelled && (Number(swipe.dismissProgress) || 0) >= 0.22;
    if (shouldExit) {
      state.lightboxSwipeClickSuppressUntil = Date.now() + 420;
      e.preventDefault();
      e.stopPropagation();
      startLightboxUnderzoomExit();
      return true;
    }
    clearLightboxDismissGesture({ animate: !!swipe.dragging });
    if (swipe.dragging) {
      state.lightboxSwipeClickSuppressUntil = Date.now() + 320;
      e.preventDefault();
      e.stopPropagation();
      return true;
    }
    return false;
  }
  const dx = (swipe.lastX || e.clientX) - swipe.startX;
  const threshold = Math.min(88, Math.max(28, swipe.width * 0.1));
  const dir = dx < 0 ? 1 : -1;
  const cancelled = e.type === 'pointercancel';
  const canSwitch = !cancelled && Math.abs(dx) >= threshold && (dir < 0 ? state.lightboxIndex > 0 : (state.lightboxIndex < state.lightboxPhotos.length - 1 || lightboxCanLoadMoreForward()));
  const media = currentLightboxMediaElement();
  if (!swipe.dragging || !canSwitch || !media) {
    resetLightboxSwipeVisual({ animate: !!swipe.dragging, direction: swipe.horizontalDirection || dir });
    if (swipe.dragging) {
      state.lightboxSwipeClickSuppressUntil = Date.now() + 420;
      e.preventDefault();
      e.stopPropagation();
    }
    return !!swipe.dragging;
  }
  state.lightboxSwipeClickSuppressUntil = Date.now() + 520;
  e.preventDefault();
  e.stopPropagation();
  const finalX = dir > 0 ? -swipe.width : swipe.width;
  setLightboxSwipePreviewTransform(finalX, true, dir);
  media.classList.add('lightbox-swipe-animating');
  media.style.setProperty('--lightbox-swipe-opacity', '0');
  await new Promise(resolve => setTimeout(resolve, lightboxSwipeAnimationMs));
  await lbNav(dir);
  resetLightboxSwipeVisual();
  return true;
}
function clampLightboxValue(value, min, max) {
  return Math.max(min, Math.min(max, value));
}
function updateLightboxFocusPointFromPointer(clientX, clientY) {
  const body = $('#lightbox .lightbox-body');
  const media = currentLightboxMediaElement();
  if (!body || !media) return;
  const bodyRect = body.getBoundingClientRect();
  const insideBody = clientX >= bodyRect.left && clientX <= bodyRect.right && clientY >= bodyRect.top && clientY <= bodyRect.bottom;
  state.lightboxPointerClientX = clientX;
  state.lightboxPointerClientY = clientY;
  state.lightboxPointerInside = insideBody;
  if (!insideBody) return;
  const mediaRect = media.getBoundingClientRect();
  const metrics = lightboxViewportMetrics(body);
  const viewportX = clampLightboxValue((clientX - bodyRect.left - metrics.paddingLeft) / metrics.width, 0, 1);
  const viewportY = clampLightboxValue((clientY - bodyRect.top - metrics.paddingTop) / metrics.height, 0, 1);
  const mediaX = mediaRect.width > 0
    ? clampLightboxValue((clientX - mediaRect.left) / mediaRect.width, 0, 1)
    : 0.5;
  const mediaY = mediaRect.height > 0
    ? clampLightboxValue((clientY - mediaRect.top) / mediaRect.height, 0, 1)
    : 0.5;
  state.lightboxFocusPoint = { mediaX, mediaY, viewportX, viewportY };
}
function syncLightboxTemporaryZoom(active) {
  const next = !!active;
  if (state.lightboxBoostActive === next) return;
  if (next && state.lightboxPointerInside) {
    updateLightboxFocusPointFromPointer(state.lightboxPointerClientX, state.lightboxPointerClientY);
  }
  state.lightboxBoostActive = next;
  applyLightboxZoom();
}
function applyLightboxZoom() {
  if ($('#lightbox')?.classList.contains('open')) syncLightboxViewportGeometry();
  const img = $('#lb-img');
  const video = $('#lb-video');
  const label = $('#lb-zoom-value');
  const input = $('#lb-zoom');
  const body = $('#lightbox .lightbox-body');
  const fitBtn = $('#lb-fit-height');
  const currentPhoto = state.lightboxPhotos[state.lightboxIndex] || {};
  const { width, height } = lightboxMediaNaturalSize();
  const focusPoint = state.lightboxFocusPoint || { mediaX: 0.5, mediaY: 0.5, viewportX: 0.5, viewportY: 0.5 };
  const effective = lightboxEffectiveZoomState();
  let zoomed = effective.zoom > 100;
  if (body && width > 0 && height > 0) {
    const { width: viewportWidth, height: viewportHeight } = lightboxViewportSize(body);
    const shouldUpscaleVideoToFit = isVideoMedia(currentPhoto) && effective.mode !== 'fit';
    const fitScale = effective.mode === 'fit' || shouldUpscaleVideoToFit
      ? Math.min(viewportWidth / width, viewportHeight / height)
      : Math.min(viewportWidth / width, viewportHeight / height, 1);
    const displayScale = fitScale * (effective.zoom / 100);
    const targetWidth = Math.max(1, Math.ceil(width * displayScale));
    const targetHeight = Math.max(1, Math.ceil(height * displayScale));
    if (img) {
      img.style.transform = '';
      img.style.maxWidth = 'none';
      img.style.maxHeight = 'none';
      img.style.width = `${targetWidth}px`;
      img.style.height = `${targetHeight}px`;
    }
    if (video) {
      video.style.transform = '';
      video.style.maxWidth = 'none';
      video.style.maxHeight = 'none';
      video.style.width = `${targetWidth}px`;
      video.style.height = `${targetHeight}px`;
    }
    zoomed = targetWidth > viewportWidth || targetHeight > viewportHeight;
    const media = currentLightboxMediaElement();
    if (media) {
      const bodyRect = body.getBoundingClientRect();
      const mediaRect = media.getBoundingClientRect();
      const metrics = lightboxViewportMetrics(body);
      const mediaLeft = mediaRect.left - bodyRect.left - metrics.paddingLeft + body.scrollLeft;
      const mediaTop = mediaRect.top - bodyRect.top - metrics.paddingTop + body.scrollTop;
      const targetScrollLeft = mediaLeft + targetWidth * focusPoint.mediaX - viewportWidth * focusPoint.viewportX;
      const targetScrollTop = mediaTop + targetHeight * focusPoint.mediaY - viewportHeight * focusPoint.viewportY;
      body.scrollLeft = clampLightboxValue(targetScrollLeft, 0, Math.max(0, body.scrollWidth - body.clientWidth));
      body.scrollTop = clampLightboxValue(targetScrollTop, 0, Math.max(0, body.scrollHeight - body.clientHeight));
    }
  } else {
    if (img) {
      img.style.transform = '';
      img.style.width = '';
      img.style.height = '';
      img.style.maxWidth = '100%';
      img.style.maxHeight = '100%';
    }
    if (video) {
      video.style.transform = '';
      video.style.width = '';
      video.style.height = '';
      video.style.maxWidth = '100%';
      video.style.maxHeight = '100%';
    }
  }
  if (body) body.classList.toggle('zoomed', zoomed);
  if (fitBtn) fitBtn.classList.toggle('active', effective.mode === 'fit' && !state.lightboxBoostActive);
  if (label) label.textContent = `${effective.mode === 'fit' && !state.lightboxBoostActive ? 100 : effective.zoom}%`;
  if (input) {
    input.value = String(state.lightboxZoom);
    updateRangeProgress(input);
  }
}
function initExperimentalSettings() {
  state.experimentalAutoplayVideo = true;
  state.videoAutoplayNext = true;
  state.experimentalPrefetchNeighbors = true;
  state.experimentalRestoreLastView = false;
}
function setExperimentalSetting(key, enabled) {
  if (key === 'autoplayVideo') {
    state.experimentalAutoplayVideo = !!enabled;
  }
  if (key === 'prefetchNeighbors') {
    state.experimentalPrefetchNeighbors = !!enabled;
  }
  if (key === 'restoreLastView') {
    state.experimentalRestoreLastView = !!enabled;
  }
}
function normalizeLibrary(data = {}) {
  const statFields = [
    'created_at', 'createdAt',
    'last_scanned_at', 'lastScannedAt', 'last_scan_at', 'lastScanAt',
    'unsupported_media_count', 'unsupportedMediaCount',
    'total_size_text', 'totalSizeText', 'storage_text', 'storageText',
    'photo_count', 'photoCount', 'image_count', 'imageCount',
    'video_count', 'videoCount',
  ];
  const stats = {};
  statFields.forEach(key => {
    if (Object.prototype.hasOwnProperty.call(data, key)) stats[key] = data[key];
  });
  return {
    index: Number.isInteger(Number(data.index)) ? Number(data.index) : (Number.isInteger(Number(data.logo_index)) ? Number(data.logo_index) : -1),
    id: String(data.id || '').trim(),
    name: data.name || '',
    path: data.path || '',
    logo_asset: data.logo_asset || data.logoAsset || '',
    logo_image_url: data.logo_image_url || data.logoImageUrl || '',
    accent_color: normalizeHexColor(data.accent_color || data.accentColor || ''),
    status: String(data.status || '').trim(),
    available: data.available !== false,
    rebuild_required: !!(data.rebuild_required || data.rebuildRequired),
    unavailable_reason: String(data.unavailable_reason || data.unavailableReason || '').trim(),
    locked_by_username: String(data.locked_by_username || data.lockedByUsername || '').trim(),
    ...stats,
  };
}
function libraryIdentityKey(library = {}) {
  const id = String(library.id || library.library_id || '').trim();
  if (id) return `id:${id}`;
  const path = String(library.path || library.storage_path || '').trim().toLowerCase();
  return path ? `path:${path}` : '';
}
function findExistingLibraryState(library = {}) {
  const key = libraryIdentityKey(library);
  if (!key) return null;
  return (state.serverSettings.libraries || []).find(item => libraryIdentityKey(item) === key) || null;
}
function mergeLibraryDraftWithState(draft = {}) {
  const existing = findExistingLibraryState(draft);
  if (!existing) return normalizeLibrary(draft);
  const merged = {
    ...existing,
    ...draft,
    logo_image_url: draft.logo_image_url || existing.logo_image_url || existing.logoImageUrl || '',
    available: Object.prototype.hasOwnProperty.call(draft, 'available') ? draft.available : existing.available,
    unavailable_reason: Object.prototype.hasOwnProperty.call(draft, 'unavailable_reason') ? draft.unavailable_reason : existing.unavailable_reason,
    locked_by_username: Object.prototype.hasOwnProperty.call(draft, 'locked_by_username') ? draft.locked_by_username : existing.locked_by_username,
  };
  return normalizeLibrary(merged);
}
function normalizeLibraryList(libraries, fallbackPath = '') {
  const normalized = normalizeLibraries(libraries, fallbackPath);
  return normalized.map(item => normalizeLibrary(item));
}
function libraryAvailabilitySignature(libraries = []) {
  return normalizeLibraries(libraries).map(library => [
    String(library.id || '').trim() || String(library.path || '').trim(),
    library.available === false ? '0' : '1',
    String(library.status || '').trim(),
    String(library.unavailable_reason || library.unavailableReason || '').trim(),
    String(library.locked_by_username || library.lockedByUsername || '').trim(),
    String(library.logo_asset || library.logoAsset || '').trim(),
    String(library.logo_image_url || library.logoImageUrl || '').trim(),
  ].join('|')).join('||');
}
function libraryAvailabilityMetaText(library = {}) {
  const status = String(library.status || '').trim();
  const reason = String(library.unavailable_reason || library.unavailableReason || '').trim();
  if (!reason) return '';
  if (status === 'visitor_shared' || status === 'admin_locked') {
    return reason;
  }
  return '';
}
function libraryAvailabilityDetailText(library = {}) {
  const reason = String(library.unavailable_reason || library.unavailableReason || '').trim();
  if (!reason) return '';
  const meta = libraryAvailabilityMetaText(library);
  return meta === reason ? '' : reason;
}
function normalizeShareLink(link = {}) {
  return {
    id: Number(link.id) || 0,
    token: link.token || '',
    type: link.type || '',
    target_id: Number(link.target_id) || 0,
    created_at: link.created_at || '',
    expires_at: link.expires_at || '',
  };
}
function rebuildShareMap() {
  state.shareMap = {};
  state.shareLinks.forEach(link => {
    const key = `${link.type}:${link.target_id}`;
    if (!state.shareMap[key]) state.shareMap[key] = [];
    state.shareMap[key].push(link);
  });
}
function setShareLinks(links = []) {
  state.shareLinks = (links || [])
    .map(item => normalizeShareLink(item))
    .sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime());
  rebuildShareMap();
}
function upsertShareLink(link) {
  const normalized = normalizeShareLink(link);
  state.shareLinks = state.shareLinks.filter(item => item.id !== normalized.id);
  state.shareLinks.unshift(normalized);
  state.shareLinks.sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime());
  rebuildShareMap();
  refreshShareManagementUI();
  refreshPhotoShareIndicators(normalized.type, normalized.target_id);
}

async function copyExistingShareLink(type, targetId) {
  const key = `${type}:${targetId}`;
  const links = state.shareMap[key] || [];
  const share = links[0];
  if (!share || !share.token) {
    showToast(copyText('app.interface.s_c7619edcd2'));
    return false;
  }
  await navigator.clipboard.writeText(`${location.origin}/s/${share.token}`);
  showToast(settingsText('shares.copied'));
  return true;
}

function removeShareLinkFromState(id) {
  let removed = null;
  state.shareLinks = state.shareLinks.filter(link => {
    if (link.id !== id) return true;
    removed = link;
    return false;
  });
  rebuildShareMap();
  if (state.view === 'settings' && state.settingsReady && !state.shareLinks.length) {
    renderSettingsContent();
  } else {
    refreshShareManagementUI();
  }
  if (removed) refreshPhotoShareIndicators(removed.type, removed.target_id);
}
function createTextFavicon(text) {
  const glyph = (text || 'PA').trim().slice(0, 2) || 'PA';
  return `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#2d6a5f"/><text x="32" y="41" font-size="28" text-anchor="middle" fill="#fff" font-family="Arial, sans-serif">${glyph}</text></svg>`)}`;
}
function createRoundedImageFavicon(url, accent = '#2d6a5f') {
  const safeURL = escapeHTML(url);
  const safeAccent = normalizeHexColor(accent) || '#2d6a5f';
  return `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><defs><clipPath id="eg-favicon-round"><rect x="0" y="0" width="64" height="64" rx="16"/></clipPath></defs><rect width="64" height="64" rx="16" fill="${safeAccent}"/><image href="${safeURL}" x="0" y="0" width="64" height="64" preserveAspectRatio="xMidYMid slice" clip-path="url(#eg-favicon-round)"/></svg>`)}`;
}
function traceRoundedRect(ctx, x, y, width, height, radius) {
  if (typeof ctx.roundRect === 'function') {
    ctx.roundRect(x, y, width, height, radius);
    return;
  }
  const r = Math.min(radius, width / 2, height / 2);
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + width, y, x + width, y + height, r);
  ctx.arcTo(x + width, y + height, x, y + height, r);
  ctx.arcTo(x, y + height, x, y, r);
  ctx.arcTo(x, y, x + width, y, r);
  ctx.closePath();
}
function applyRoundedImageFavicon(favicon, url, accent, fallbackText) {
  if (!favicon || !url) return;
  const fallback = createRoundedImageFavicon(url, accent);
  favicon.dataset.logoUrl = url;
  favicon.href = fallback;
  if (typeof Image === 'undefined' || typeof document.createElement !== 'function') return;
  const img = new Image();
  img.decoding = 'async';
  img.onload = () => {
    if (favicon.dataset.logoUrl !== url) return;
    const canvas = document.createElement('canvas');
    canvas.width = 64;
    canvas.height = 64;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    if (!img.naturalWidth || !img.naturalHeight) {
      favicon.href = fallback;
      return;
    }
    ctx.fillStyle = normalizeHexColor(accent) || '#2d6a5f';
    ctx.beginPath();
    traceRoundedRect(ctx, 0, 0, 64, 64, 16);
    ctx.fill();
    ctx.save();
    ctx.beginPath();
    traceRoundedRect(ctx, 0, 0, 64, 64, 16);
    ctx.clip();
    const scale = Math.max(64 / img.naturalWidth, 64 / img.naturalHeight);
    const width = img.naturalWidth * scale;
    const height = img.naturalHeight * scale;
    ctx.drawImage(img, (64 - width) / 2, (64 - height) / 2, width, height);
    ctx.restore();
    try {
      favicon.href = canvas.toDataURL('image/png');
    } catch (_) {
      favicon.href = fallback || createTextFavicon(fallbackText);
    }
  };
  img.onerror = () => {
    if (favicon.dataset.logoUrl === url) favicon.href = createTextFavicon(fallbackText);
  };
  img.src = url;
}
function resolveActiveLibrary(libraries, currentID, currentPath) {
  const availableLibraries = Array.isArray(libraries) ? libraries.filter(item => item && item.available !== false) : [];
  const normalizedID = String(currentID || '').trim();
  if (normalizedID) {
    const byID = availableLibraries.find(item => String(item.id || '').trim() === normalizedID);
    if (byID) return byID;
  }
  const normalizedPath = String(currentPath || '').trim();
  return availableLibraries.find(item => item.path === normalizedPath) || availableLibraries[0] || libraries[0] || null;
}
function currentLibraryBrand() {
  const draftLibraries = collectLibraryDrafts();
  if (draftLibraries.length) {
    const activeSelect = $('#settings-active-library');
    const activeValue = activeSelect ? activeSelect.value.trim() : '';
    return resolveActiveLibrary(draftLibraries, activeValue, state.serverSettings.storage_path || '');
  }
  return resolveActiveLibrary(state.serverSettings.libraries || [], state.serverSettings.active_library_id || '', state.serverSettings.storage_path || '');
}
async function ensureRandomLibraryLogo(library = currentLibraryBrand()) {
  if (!library || library.logo_asset || library.logo_image_url || library.logoImageUrl || state.libraryRandomLogoLoading) return;
  const key = libraryVisualSeed(library);
  if (!key || state.libraryRandomLogos[key] || state.libraryRandomLogoTried[key]) return;
  const activePath = String(state.serverSettings.storage_path || '').trim();
  const libraryPath = String(library.path || '').trim();
  if (!libraryPath || libraryPath !== activePath || state.settingsDirty) return;
  const libraries = normalizeLibraries(state.serverSettings.libraries, activePath);
  const index = libraries.findIndex(item => item.path === libraryPath);
  if (index < 0) return;
  state.libraryRandomLogoTried[key] = true;
  state.libraryRandomLogoLoading = true;
  try {
    const resp = await refreshLibraryLogos(false);
    const refreshedLibrary = resolveActiveLibrary((resp && resp.data && resp.data.libraries) || state.serverSettings.libraries || [], library.id || '', libraryPath);
    const logoURL = resolveLibraryLogoURL(refreshedLibrary || {});
    if (logoURL) {
      state.libraryRandomLogos[key] = logoURL;
      if (state.view === 'settings') renderSettings().catch(e => console.warn("刷新资源库头像失败", e));
    }
  } catch (e) {
    console.warn("随机资源库头像加载失败", e);
  } finally {
    state.libraryRandomLogoLoading = false;
  }
}
function applyNavLogoFallback(navLogo, library = {}) {
  if (!navLogo) return;
  const img = $('.nav-logo-image', navLogo);
  const media = $('.nav-logo-media', navLogo);
  if (!img || !media) return;
  const fallback = () => {
    media.innerHTML = `<span class="nav-logo-mark">${icons.photo}</span>`;
  };
  img.addEventListener('error', fallback, { once: true });
}
function applyLibraryPreviewFallback(preview, library = {}) {
  if (!preview) return;
  const img = $('img', preview);
  if (!img) return;
  const fallback = () => {
    preview.classList.remove('has-image');
    preview.innerHTML = `<span class="library-avatar-placeholder">${icons.photo}</span>`;
  };
  img.addEventListener('error', fallback, { once: true });
}
function applyLibraryBranding() {
  const current = currentLibraryBrand();
  const accent = current ? resolveLibraryAccentColor(current) : '#2d6a5f';
  const isDark = document.documentElement.dataset.theme === 'dark';
  const root = document.documentElement;
  root.style.setProperty('--accent', accent);
  root.style.setProperty('--accent-h', mixColors(accent, isDark ? '#ffffff' : '#000000', isDark ? 0.18 : 0.16));
  root.style.setProperty('--accent-soft', rgbaColor(accent, isDark ? 0.24 : 0.14));
  root.style.setProperty('--accent-strong', rgbaColor(accent, isDark ? 0.36 : 0.22));
  root.style.setProperty('--accent-faint', rgbaColor(accent, isDark ? 0.14 : 0.07));
  root.style.setProperty('--accent-surface', rgbaColor(accent, isDark ? 0.24 : 0.10));
  root.style.setProperty('--nav-bg', mixColors(accent, isDark ? '#141414' : '#ffffff', isDark ? 0.82 : 0.92));
  root.style.setProperty('--topbar-bg', mixColors(accent, isDark ? '#181818' : '#ffffff', isDark ? 0.88 : 0.95));
  root.style.setProperty('--nav-border-color', rgbaColor(accent, isDark ? 0.28 : 0.18));
  root.style.setProperty('--topbar-border-color', rgbaColor(accent, isDark ? 0.22 : 0.16));
  root.style.setProperty('--nav-text', isDark ? '#f5efe6' : '#241d18');
  root.style.setProperty('--nav-text-muted', isDark ? 'rgba(245,239,230,.72)' : 'rgba(36,29,24,.82)');
  root.style.setProperty('--topbar-text', isDark ? '#f5efe6' : '#253933');
  root.style.setProperty('--topbar-text-muted', isDark ? 'rgba(245,239,230,.72)' : 'rgba(36,29,24,.78)');
  const appName = currentAppDisplayName();
  document.title = current && current.name ? `${current.name} - ${appName}` : appName;
  let appleTitle = document.querySelector('meta[name="apple-mobile-web-app-title"]');
  if (!appleTitle) {
    appleTitle = document.createElement('meta');
    appleTitle.name = 'apple-mobile-web-app-title';
    document.head.appendChild(appleTitle);
  }
  appleTitle.content = appName;
  let favicon = document.querySelector('link[rel="icon"]');
  if (!favicon) {
    favicon = document.createElement('link');
    favicon.rel = 'icon';
    document.head.appendChild(favicon);
  }
  const logoURL = current ? resolveLibraryLogoURL(current) : '';
  const fallbackText = current && current.name ? current.name : 'PA';
  if (logoURL) applyRoundedImageFavicon(favicon, logoURL, accent, fallbackText);
  else {
    favicon.dataset.logoUrl = '';
    favicon.href = createTextFavicon(fallbackText);
  }
  const navLogo = document.querySelector('.nav-logo');
  if (navLogo) {
    navLogo.innerHTML = renderNavLogo();
    applyNavLogoFallback(navLogo, current || {});
  }
  syncTopbarTitleNote();
  ensureRandomLibraryLogo(current);
}
async function refreshVideoThumbnails() {
  return api.post('/api/settings/video-thumbnails/refresh', {});
}
async function fetchVideoThumbnailRefreshStatus() {
  return api.get('/api/settings/video-thumbnails/refresh');
}
async function cancelVideoThumbnailRefresh() {
  return api.del('/api/settings/video-thumbnails/refresh');
}
async function repairEXIFMetadata() {
  return api.post('/api/settings/exif/backfill', {});
}
async function fetchEXIFBackfillStatus() {
  return api.get('/api/settings/exif/backfill');
}
async function cancelEXIFBackfill() {
  return api.del('/api/settings/exif/backfill');
}
async function fetchThumbnailBuildStatus() {
  return api.get('/api/settings/thumbnails/build');
}
async function startThumbnailBuild() {
  return api.post('/api/settings/thumbnails/build', {});
}
async function cancelThumbnailBuild() {
  return api.del('/api/settings/thumbnails/build');
}
function renderNavLogo() {
  const current = currentLibraryBrand();
  if (current) {
    const label = current.name || 'EchoGallery';
    const logoURL = resolveLibraryLogoURL(current);
    const media = logoURL
      ? `<img class="nav-logo-image" src="${logoURL}" alt="${escapeHTML(label)}">`
      : `<span class="nav-logo-mark">${icons.photo}</span>`;
    return `<span class="nav-logo-media">${media}</span>`;
  }
  return `<span class="nav-logo-media"><span class="nav-logo-mark">${icons.photo}</span></span>`;
}

function topbarNoteText() {
  const current = currentLibraryBrand();
  return current && current.name ? current.name : 'EchoGallery Library';
}

function syncTopbarTitleNote() {
  const title = $('#topbar-title');
  if (title) title.dataset.note = topbarNoteText();
}

function isCompactNavLayout() {
  return window.matchMedia('(max-width: 900px)').matches;
}

function setNavPickerOpen(open) {
  const nav = $('#main-nav');
  if (!nav) return;
  nav.classList.toggle('nav-picker-open', !!open);
}

function closeNavPicker() {
  setNavPickerOpen(false);
}

function renderSearchOverlay() {
  return `<div class="search-overlay" id="search-overlay" aria-hidden="true">
  <div class="search-panel" role="dialog" aria-modal="true" aria-label="${escapeHTML(copyText('app.search.ariaLabel'))}">
    <div class="search-head">
      <div class="search-box">
        <span class="search-box-icon">${icons.search}</span>
        <input id="global-search-input" type="text" autocomplete="off" spellcheck="false" placeholder="${escapeHTML(copyText('app.search.placeholder'))}">
        <button class="btn-icon search-close-btn" id="search-close-btn" type="button" aria-label="${escapeHTML(copyText('app.search.close'))}">${icons.close}</button>
      </div>
      <div class="search-filters" id="search-filters">
        ${renderSearchFilterButton('all', copyText('app.interface.s_778fc8f994'))}
        ${renderSearchFilterButton('photo', copyText('app.interface.s_48e9e53252'))}
        ${renderSearchFilterButton('video', copyText('app.interface.mediaVideo'))}
        ${renderSearchFilterButton('album', copyText('app.interface.s_c657c81e6b'))}
        ${renderSearchFilterButton('favorite', copyText('app.interface.albumFavorites'))}
      </div>
    </div>
    <div class="search-status" id="search-status"></div>
    <div class="search-results" id="search-results"></div>
    <div class="search-actions">
      <button class="btn search-more-btn" id="search-more-btn" type="button">${escapeHTML(copyText('app.search.loadMore'))}</button>
    </div>
  </div>
</div>`;
}

function renderSearchFilterButton(value, label) {
  return `<button class="search-filter${state.searchFilter === value ? ' active' : ''}" type="button" data-search-filter="${value}">${label}</button>`;
}

function syncSearchInputs(value = '') {
  const text = String(value);
  const overlayInput = $('#global-search-input');
  if (overlayInput && overlayInput.value !== text) overlayInput.value = text;
}

function openGlobalSearch(prefill, options = {}) {
  const overlay = $('#search-overlay');
  const input = $('#global-search-input');
  if (!overlay || !input) return;
  const shouldFocus = typeof options.focus === 'boolean' ? options.focus : !isTouchLikeDevice();
  state.searchOpen = true;
  overlay.classList.add('open');
  overlay.setAttribute('aria-hidden', 'false');
  if (prefill === undefined) syncSearchInputs(input.value || state.searchQuery || '');
  else syncSearchInputs(prefill);
  renderSearchResults();
  if (shouldFocus) requestAnimationFrame(() => input.focus({ preventScroll: true }));
  if (input.value.trim() && input.value.trim() !== state.searchQuery) scheduleGlobalSearch();
}

function closeGlobalSearch() {
  state.searchOpen = false;
  if (state.searchAbortController) state.searchAbortController.abort();
  if (state.searchTimer) clearTimeout(state.searchTimer);
  state.searchAbortController = null;
  state.searchTimer = null;
  $('#search-overlay')?.classList.remove('open');
  $('#search-overlay')?.setAttribute('aria-hidden', 'true');
  syncSearchInputs(state.searchQuery || '');
}

function resetSearchResults(query = '') {
  state.searchQuery = query;
  state.searchResults = [];
  state.searchAlbumResults = [];
  state.searchCursor = '';
  state.searchHasMore = false;
  state.searchTotal = 0;
  const resultsEl = $('#search-results');
  if (resultsEl) resultsEl.scrollTop = 0;
}

function scheduleGlobalSearch() {
  const input = $('#global-search-input');
  if (!input) return;
  const query = input.value.trim();
  syncSearchInputs(query);
  if (state.searchTimer) clearTimeout(state.searchTimer);
  if (!query) {
    if (state.searchAbortController) state.searchAbortController.abort();
    resetSearchResults('');
    state.searchLoading = false;
    renderSearchResults();
    return;
  }
  state.searchTimer = setTimeout(() => runGlobalSearch(query, { reset: true }), 260);
}

async function runGlobalSearch(query, { reset = false } = {}) {
  if (!query || (state.searchLoading && !reset)) return;
  if (state.searchAbortController) state.searchAbortController.abort();
  const controller = new AbortController();
  state.searchAbortController = controller;
  if (reset) resetSearchResults(query);
  state.searchLoading = true;
  renderSearchResults();
  try {
    const includeMedia = state.searchFilter !== 'album';
    const includeAlbums = state.searchFilter === 'all' || state.searchFilter === 'album';
    let mediaTotal = 0;
    if (includeMedia) {
      const params = new URLSearchParams({ q: query, limit: '36' });
      if (state.searchFilter === 'photo') params.set('kind', 'image');
      if (state.searchFilter === 'video') params.set('kind', 'video');
      if (state.searchFilter === 'favorite') params.set('favorite', '1');
      if (!reset && state.searchCursor) params.set('cursor', state.searchCursor);
      const response = await fetchWithPageSession(`/api/media/search?${params.toString()}`, { signal: controller.signal });
      if (!response.ok) throw await buildAPIError(response);
      const page = await response.json();
      const photos = page.photos || [];
      state.searchResults = reset ? photos : state.searchResults.concat(photos);
      state.searchCursor = page.next_cursor || '';
      state.searchHasMore = !!page.has_more;
      mediaTotal = Number.isFinite(Number(page.total)) ? Number(page.total) : state.searchResults.length;
      requestVisibleThumbnailWarmup('search', photos);
    } else {
      state.searchResults = [];
      state.searchCursor = '';
      state.searchHasMore = false;
    }
    if (includeAlbums && reset) {
      state.searchAlbumResults = await searchAlbums(query, controller.signal);
    } else if (!includeAlbums) {
      state.searchAlbumResults = [];
    }
    state.searchTotal = mediaTotal + state.searchAlbumResults.length;
  } catch (e) {
    if (!e || e.name !== 'AbortError') {
      console.error(e);
      showToast(copyText('app.interface.s_80f9c9cd1c'));
    }
  } finally {
    if (state.searchAbortController === controller) {
      state.searchLoading = false;
      state.searchAbortController = null;
      renderSearchResults();
    }
  }
}

async function searchAlbums(query, signal) {
  if (!state.searchAlbumCache) {
    const response = await fetchWithPageSession('/api/media/albums', { signal });
    if (!response.ok) throw await buildAPIError(response);
    state.searchAlbumCache = await response.json();
  }
  const needle = query.trim().toLowerCase();
  return (state.searchAlbumCache || []).filter(album => {
    const haystack = `${album.name || ''} ${album.description || ''}`.toLowerCase();
    return haystack.includes(needle);
  }).slice(0, 24);
}

function searchResultMeta(photo) {
  const { kind, size, date } = searchResultDetails(photo);
  return [kind, date, size].filter(value => value && value !== '—').join(' · ');
}

function searchResultDetails(photo) {
  return {
    kind: isVideoMedia(photo) ? copyText('app.interface.mediaVideo') : copyText('app.interface.mediaImage'),
    size: formatSizeMB(photo && photo.size),
    date: formatDateTime(photo && photo.taken_at),
  };
}

function renderSearchResults() {
  const resultsEl = $('#search-results');
  const statusEl = $('#search-status');
  const moreBtn = $('#search-more-btn');
  if (!resultsEl || !statusEl || !moreBtn) return;
  const query = $('#global-search-input')?.value.trim() || state.searchQuery;
  if (!query) {
    statusEl.textContent = '';
    resultsEl.innerHTML = `<div class="search-empty">${escapeHTML(copyText('app.interface.runtime.searchPrompt'))}</div>`;
    moreBtn.classList.remove('visible');
    return;
  }
  if (state.searchLoading && !state.searchResults.length) {
    statusEl.textContent = copyText('app.interface.s_8a88cdc40e');
    resultsEl.innerHTML = `<div class="search-empty"><div class="spinner"></div></div>`;
    moreBtn.classList.remove('visible');
    return;
  }
  statusEl.textContent = state.searchTotal ? copyText('app.interface.d_d21ae305a1', { v0: (state.searchTotal) }) : (state.searchResults.length ? copyText('app.interface.d_d21ae305a1', { v0: (state.searchResults.length) }) : '');
  if (!state.searchResults.length && !state.searchAlbumResults.length) {
    resultsEl.innerHTML = `<div class="search-empty">${escapeHTML(copyText('app.interface.runtime.searchNoResults'))}</div>`;
  } else {
    resultsEl.innerHTML = '';
    const fragment = document.createDocumentFragment();
    state.searchAlbumResults.forEach(album => {
      const card = el('button', 'search-result-card search-result-album');
      card.type = 'button';
      const smartAlbum = String(album && album.source_kind || '').trim() === 'smart';
      const description = String(album.description || '').trim() || (smartAlbum ? smartAlbumDescribeRules(album.smart_rules || []) : '');
      const cover = album.cover_uuid
        ? `<img loading="lazy" src="${mediaThumbURLFromUUID(album.cover_uuid)}" alt="${escapeHTML(album.name || '')}">`
        : `<span class="search-result-album-icon">${smartAlbum ? icons.albumIntelligence : icons.album}</span>`;
      card.innerHTML = `
        <span class="search-result-thumb">
          ${cover}
          <span class="search-result-kind">${smartAlbum ? copyText('app.interface.albumSmart') : copyText('app.interface.s_c657c81e6b')}</span>
        </span>
        <span class="search-result-main">
          <strong>${escapeHTML(album.name || copyText('app.interface.s_6ec77177ba'))}</strong>
          <span>${escapeHTML(copyText('app.interface.runtime.mediaCount', { count: album.photo_count || 0 }))}${description ? ` · ${escapeHTML(description)}` : ''}</span>
        </span>`;
      card.addEventListener('click', () => {
        closeGlobalSearch();
        openAlbumDetail(album);
      });
      card.addEventListener('contextmenu', e => {
        e.preventDefault();
        showAlbumContextMenu(e.clientX, e.clientY, album, { mobile: isTouchLikeDevice() });
      });
      addLongPress(card, e => {
        const touch = e.changedTouches[0];
        showAlbumContextMenu(touch.clientX, touch.clientY, album, { mobile: true });
      });
      fragment.appendChild(card);
    });
    state.searchResults.forEach((photo, index) => {
      const card = el('button', 'search-result-card');
      card.type = 'button';
      card.innerHTML = `
        <span class="search-result-thumb">
          <img loading="lazy" src="${mediaThumbURL(photo)}" alt="${escapeHTML(photo.original_name || '')}">
          ${isVideoMedia(photo) ? `<span class="search-result-kind">${copyText('app.lightbox.video')}</span>` : ''}
        </span>
        <span class="search-result-main">
          <strong>${escapeHTML(photo.original_name || copyText('app.interface.s_90a5096a0c'))}</strong>
          <span>${escapeHTML(searchResultMeta(photo))}</span>
        </span>`;
      const img = $('img', card);
      if (img && isVideoMedia(photo)) img.onerror = () => { img.onerror = null; img.src = videoPosterPlaceholder; };
      if (img && !isVideoMedia(photo)) {
        img.onerror = () => {
          if (img.dataset.fallbackApplied === '1') return;
          img.dataset.fallbackApplied = '1';
          img.src = mediaFileURL(photo);
        };
      }
      card.addEventListener('click', () => {
        closeGlobalSearch();
        openLightbox(state.searchResults, index, { returnView: 'search' });
      });
      card.addEventListener('contextmenu', e => {
        e.preventDefault();
        showPhotoContextMenu(e.clientX, e.clientY, photo, card, state.searchResults, { mobile: isTouchLikeDevice() });
      });
      addLongPress(card, e => {
        const touch = e.changedTouches[0];
        showPhotoContextMenu(touch.clientX, touch.clientY, photo, card, state.searchResults, { mobile: true });
      });
      fragment.appendChild(card);
    });
    resultsEl.appendChild(fragment);
  }
  moreBtn.classList.toggle('visible', state.searchHasMore);
  moreBtn.disabled = state.searchLoading;
  moreBtn.textContent = state.searchLoading ? copyText('app.interface.commonLoading') : copyText('app.interface.s_3a0fab4978');
}

function viewShortcutMap() {
  if (isRootUser()) return ['root-debug', 'settings'];
  return availableRootViews();
}
function shouldIgnoreGlobalShortcut(target) {
  if (!target) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
}
function isMacPlatform() {
  const platform = String(navigator.userAgentData?.platform || navigator.platform || '').toLowerCase();
  return platform.includes('mac') || platform.includes('iphone') || platform.includes('ipad');
}
function handleViewNumberShortcut(e) {
  if (state.blockingInteraction) {
    e.preventDefault();
    e.stopImmediatePropagation();
    return true;
  }
  if (!e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false;
  if (shouldIgnoreGlobalShortcut(e.target)) return false;
  const code = String(e.code || '');
  let index = -1;
  if (/^Digit[1-7]$/.test(code)) {
    index = Number(code.slice(5)) - 1;
  } else if (/^Numpad[1-7]$/.test(code)) {
    index = Number(code.slice(6)) - 1;
  }
  if (index < 0) return false;
  const view = viewShortcutMap()[index];
  if (!view) return false;
  e.preventDefault();
  e.stopImmediatePropagation();
  closeLightbox();
  closeDrawer();
  switchView(view);
  return true;
}
function handleAlbumSwitchShortcut(e) {
  if (state.searchOpen || state.blockingInteraction || document.querySelector('.modal-overlay.open')) return false;
  if ($('#lightbox')?.classList.contains('open')) return false;
  if (state.view !== 'album-detail') return false;
  if (shouldIgnoreGlobalShortcut(e.target)) return false;
  if (e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return false;
  const key = e.key.toLowerCase();
  if (key !== 'q' && key !== 'e') return false;
  const offset = key === 'q' ? -1 : 1;
  if (!adjacentAlbum(offset)) return false;
  e.preventDefault();
  e.stopImmediatePropagation();
  openAdjacentAlbum(offset);
  return true;
}
function handleSearchShortcut(e) {
  if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && (e.key === 'k' || e.key === 'K')) {
    if (shouldIgnoreGlobalShortcut(e.target) && e.target?.id !== 'global-search-input') return false;
    e.preventDefault();
    e.stopImmediatePropagation();
    if (state.searchOpen) closeGlobalSearch();
    else openGlobalSearch();
    return true;
  }
  return false;
}
function handleDeleteShortcut(e) {
  if (state.blockingInteraction) return false;
  if ($('#lightbox')?.classList.contains('open')) return false;
  if (document.querySelector('.modal-overlay.open')) return false;
  if (shouldIgnoreGlobalShortcut(e.target)) return false;
  const isBackspace = e.key === 'Backspace' || e.code === 'Backspace';
  if (!isBackspace || e.ctrlKey || e.shiftKey) return false;
  const matches = isMacPlatform()
    ? (e.metaKey && !e.altKey)
    : (e.altKey && !e.metaKey);
  if (!matches) return false;
  if (state.view === 'trash') return false;
  if (!state.selected.size && !state.focusedPhotoID) return false;
  e.preventDefault();
  e.stopImmediatePropagation();
  if (state.selected.size > 0) {
    deleteSelected();
    return true;
  }
  deleteSinglePhoto(state.focusedPhotoID);
  return true;
}
function normalizeKeyToken(token) {
  const upper = token.toUpperCase();
  if (upper === 'PGUP') return 'PAGEUP';
  if (upper === 'PGDWN') return 'PAGEDOWN';
  if (upper === 'RIGHT') return 'ARROWRIGHT';
  if (upper === 'LEFT') return 'ARROWLEFT';
  if (upper === 'UP') return 'ARROWUP';
  if (upper === 'DOWN') return 'ARROWDOWN';
  if (upper === 'CMD' || upper === 'COMMAND' || upper === 'SUPER' || upper === 'WIN') return 'META';
  if (upper === 'CONTROL') return 'CTRL';
  if (upper === 'OPTION') return 'ALT';
  return upper;
}
function keyTokenWithModifiers(base, modifiers = []) {
  const order = ['CTRL', 'ALT', 'SHIFT', 'META'];
  const uniqueModifiers = Array.from(new Set((modifiers || []).map(normalizeKeyToken)))
    .filter(token => order.includes(token))
    .sort((a, b) => order.indexOf(a) - order.indexOf(b));
  return uniqueModifiers.length ? `${uniqueModifiers.join('+')}+${base}` : base;
}
function parseKeyBindingToken(spec) {
  const parts = String(spec || '').split('+').map(part => part.trim()).filter(Boolean);
  if (!parts.length) return '';
  const modifiers = [];
  let basePart = parts.pop();
  parts.forEach(part => {
    const normalized = normalizeKeyToken(part);
    if (['CTRL', 'ALT', 'SHIFT', 'META'].includes(normalized)) modifiers.push(normalized);
  });
  if (basePart === ' ') basePart = 'SPACE';
  const isSingleLetter = /^[A-Za-z]$/.test(basePart);
  let base = normalizeKeyToken(basePart);
  if (isSingleLetter) {
    if (basePart === basePart.toUpperCase() && basePart !== basePart.toLowerCase() && !modifiers.includes('SHIFT')) {
      modifiers.push('SHIFT');
    }
    base = basePart.toUpperCase();
  }
  return keyTokenWithModifiers(base, modifiers);
}
function parsePlayerKeymap(content) {
  const map = {};
  (content || '').split('\n').forEach(line => {
    const cleaned = line.replace(/\s+#.*$/, '').trim();
    if (!cleaned || cleaned.startsWith('#')) return;
    const parts = cleaned.split(/\s+/);
    if (parts.length < 2) return;
    const key = parts.shift();
    const command = parts.shift();
    const bindingToken = parseKeyBindingToken(key);
    if (!bindingToken) return;
    map[bindingToken] = { command, args: parts };
  });
  return map;
}
async function loadDefaultPlayerKeymap() {
  try {
    const data = await api.get('/api/player/keymap');
    return data.content || '';
  } catch (_) {
    return '';
  }
}
async function initPlayerKeymapSettings() {
  const defaults = await loadDefaultPlayerKeymap();
  state.playerKeymapSource = defaults;
  state.playerKeymap = parsePlayerKeymap(defaults);
}
function updatePlayerKeymapSource(content) {
  state.playerKeymapSource = content;
  state.playerKeymap = parsePlayerKeymap(content);
}
async function resetPlayerKeymapToDefault() {
  const defaults = await loadDefaultPlayerKeymap();
  updatePlayerKeymapSource(defaults);
  const textarea = $('#settings-player-keymap');
  if (textarea) textarea.value = defaults;
}
async function loadServerSettings() {
  try {
    state.serverSettings = await api.get('/api/settings');
    applyServerSettings(state.serverSettings);
    if (Array.isArray(state.serverSettings.libraries) && state.serverSettings.libraries.some(library => library.rebuild_required)) {
      showToast(copyText('app.interface.s_adb9c56298'), 5200);
    }
  } catch (_) {
    if (pageSessionRedirectPending) return state.serverSettings;
    state.serverSettings = {
      current_username: '',
      role: 'admin',
      can_write: true,
      can_admin: true,
      can_root: false,
      port: 8080,
      active_library_id: '',
      storage_path: '',
      libraries: [],
      thumbnail_dir: '',
      thumbnail_size: 512,
      trash_dir: '',
      use_system_player: false,
      jwt_secret_masked: copyText('app.interface.s_43e203c1de'),
      users: [],
      theme: 'light',
      grid_size: 180,
      grid_gap: 2,
      thumb_radius: 2,
      sidebar_auto_hide: false,
      slideshow_mode: 'random',
      slideshow_loop: true,
      slideshow_interval: 5000,
      lightbox_zoom: 100,
      experimental_autoplay_video: true,
      video_autoplay_next: true,
      video_section_min_minutes: 10,
      experimental_prefetch_neighbors: true,
      experimental_restore_last_view: false,
      continue_last_video_position: true,
      warm_enabled: true,
      lightbox_ui_idle_seconds: 0,
      low_resource_mode: false,
      player_keymap: '',
      pwa_icon_url: '',
      local_update_config_text: renderLocalUpdateConfigPreview(),
    };
    applyServerSettings(state.serverSettings);
  }
  state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
  state.settingsDirty = false;
  return state.serverSettings;
}
function stopSettingsAvailabilityPolling() {
  if (state.settingsAvailabilityPollTimer) {
    clearTimeout(state.settingsAvailabilityPollTimer);
    state.settingsAvailabilityPollTimer = null;
  }
}
function scheduleSettingsAvailabilityPolling(delay = 5000) {
  stopSettingsAvailabilityPolling();
}
async function refreshSettingsAvailability() {
  if (state.settingsAvailabilityPollPending || state.view !== 'settings' || pageSessionRedirectPending) return;
  state.settingsAvailabilityPollPending = true;
  try {
    const data = await api.get('/api/settings', { suppressForbiddenToast: true });
    const nextLibraries = normalizeLibraryList(data && data.libraries, data && data.storage_path || '');
    const nextSignature = libraryAvailabilitySignature(nextLibraries);
    if (!state.settingsDirty && nextSignature !== state.settingsAvailabilitySignature) {
      const scrollTop = document.scrollingElement ? document.scrollingElement.scrollTop : window.scrollY;
      state.serverSettings = data || state.serverSettings;
      applyServerSettings(state.serverSettings);
      state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
      if (state.view !== 'settings') {
        return;
      }
      if (isRootUser()) {
        await renderRootConsoleContent();
      } else {
        renderSettingsContent();
        bindSettingsTopSaveButton();
        refreshMissingLibraryLogosOnce();
      }
      if (document.scrollingElement) document.scrollingElement.scrollTop = scrollTop;
      else window.scrollTo(0, scrollTop);
    } else if (!state.settingsDirty) {
      state.settingsAvailabilitySignature = nextSignature;
    }
  } catch (e) {
    if (!pageSessionRedirectPending && !isForbiddenError(e)) {
      console.warn("刷新资源库占用状态失败", e);
    }
  } finally {
    state.settingsAvailabilityPollPending = false;
  }
}
async function saveServerSettings(payload) {
  const resp = await api.put('/api/settings', payload);
  state.serverSettings = resp.data || state.serverSettings;
  applyServerSettings(state.serverSettings);
  state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
  state.settingsDirty = false;
  return resp;
}
async function restartApp() {
  return api.post('/api/settings/restart', {});
}
async function shutdownApp() {
  return api.post('/api/settings/shutdown', {});
}
const portableBrowserSettingKeys = [
  pwaSettingsStorageKey,
  mediaKindFilterStorageKey,
  timelineOrderStorageKey,
  albumDetailSortStorageKey,
  globalVideoVolumeStorageKey,
  libraryBatchWorkflowEnabledStorageKey,
  libraryBatchWorkflowAggressiveStorageKey,
  'echogallery_sidebar_compact',
];
function portableBrowserSettings() {
  const result = {};
  portableBrowserSettingKeys.forEach(key => {
    const value = localStorage.getItem(key);
    if (value != null) result[key] = value;
  });
  return result;
}
function applyPortableBrowserSettings(settings) {
  if (!settings || typeof settings !== 'object' || Array.isArray(settings)) return;
  portableBrowserSettingKeys.forEach(key => {
    if (typeof settings[key] === 'string') localStorage.setItem(key, settings[key]);
  });
}
async function exportPortableConfig() {
  const query = new URLSearchParams({ browser_settings: JSON.stringify(portableBrowserSettings()) });
  const response = await fetchWithPageSession(`/api/settings/transfer/export?${query}`);
  if (!response.ok) throw await buildAPIError(response);
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = 'echogallery-config.zip';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
async function importPortableConfig(file) {
  const form = new FormData();
  form.append('file', file);
  return api.upload('/api/settings/transfer/import', form);
}
async function fetchRootUsers() {
  return api.get('/api/root/users');
}
async function discoverLibraryDirectories(path) {
  return api.post('/api/settings/libraries/discover', { path });
}
async function createRootUser(payload) {
  return api.post('/api/root/users', payload || {});
}
async function updateRootUser(username, payload) {
  return api.put(`/api/root/users/${encodeURIComponent(username)}`, payload || {});
}
async function removeRootUser(username) {
  return api.del(`/api/root/users/${encodeURIComponent(username)}`);
}
async function fetchLibraryBuildStatus() {
  return api.get('/api/library-build/status', { suppressForbiddenToast: true });
}
async function setLibraryBuildExitAfterComplete(enabled) {
  return api.put('/api/library-build/exit-after-complete', { enabled });
}
async function cancelLibraryBuild() {
  return api.del('/api/library-build');
}
async function fetchLibraryBatchBuildStatus() {
  return api.get('/api/settings/libraries/build-all', { suppressForbiddenToast: true });
}
async function startLibraryBatchBuild(options = {}) {
  return api.post('/api/settings/libraries/build-all', {
    aggressive: !!options.aggressive,
    build_thumbnails_after_scan: !!options.buildThumbnailsAfterScan,
  });
}
async function cancelLibraryBatchBuild() {
  return api.del('/api/settings/libraries/build-all');
}
async function setLibraryBatchBuildExitAfterComplete(enabled) {
  return api.put('/api/settings/libraries/build-all/exit-after-complete', { enabled });
}
async function setLibraryBatchBuildSelection(selectedLibraryIDs, selectedPaths = []) {
  return api.put('/api/settings/libraries/build-all/selection', {
    selected_library_ids: Array.isArray(selectedLibraryIDs) ? selectedLibraryIDs : [],
    selected_paths: Array.isArray(selectedPaths) ? selectedPaths : [],
  });
}
async function fetchLibraryBatchThumbnailBuildStatus() {
  return api.get('/api/settings/libraries/thumbnails/build-all', { suppressForbiddenToast: true });
}
async function startLibraryBatchThumbnailBuild(options = {}) {
  return api.post('/api/settings/libraries/thumbnails/build-all', {
    aggressive: !!options.aggressive,
  });
}
async function cancelLibraryBatchThumbnailBuild() {
  return api.del('/api/settings/libraries/thumbnails/build-all');
}
async function fetchPlaybackCaches() {
  return api.get('/api/settings/playback-cache');
}
function playbackCacheExistsForPhoto(photo) {
  if (!photo || !photo.uuid) return false;
  return (state.playbackCacheItems || []).some(item => String(item && item.uuid || '') === String(photo.uuid));
}
async function ensurePlaybackCacheItemsLoaded() {
  if (state.playbackCacheItemsLoaded) return state.playbackCacheItems;
  await refreshPlaybackCacheItems();
  state.playbackCacheItemsLoaded = true;
  return state.playbackCacheItems;
}
async function deletePlaybackCaches(uuids) {
  return api.del('/api/settings/playback-cache', { uuids: Array.isArray(uuids) ? uuids : [] });
}
async function fetchPlaybackCacheBuildStatus() {
  return api.get('/api/settings/playback-cache/build');
}
async function startPlaybackCacheBuild() {
  return api.post('/api/settings/playback-cache/build', {});
}
async function cancelPlaybackCacheBuild() {
  return api.del('/api/settings/playback-cache/build');
}
async function setLibraryBatchThumbnailBuildExitAfterComplete(enabled) {
  return api.put('/api/settings/libraries/thumbnails/build-all/exit-after-complete', { enabled });
}
async function setLibraryBatchThumbnailBuildSelection(selectedLibraryIDs, selectedPaths = []) {
  return api.put('/api/settings/libraries/thumbnails/build-all/selection', {
    selected_library_ids: Array.isArray(selectedLibraryIDs) ? selectedLibraryIDs : [],
    selected_paths: Array.isArray(selectedPaths) ? selectedPaths : [],
  });
}
function applyServerSettings(data = {}) {
  if (typeof stopEchoFrame === 'function' && data.active_library_id !== state.serverSettings.active_library_id) stopEchoFrame();
	const appName = data.app_name || window.__GALLERY_NAME__ || 'EchoGallery';
	state.serverSettings.app_name = appName;
	window.__GALLERY_NAME__ = appName;
	if (state.pwaSettings.name !== appName) {
		state.pwaSettings = normalizePWASettings({ ...state.pwaSettings, name: appName });
		persistPWASettings();
	}
  state.serverSettings.current_username = data.current_username || state.serverSettings.current_username || '';
  state.serverSettings.role = String(data.role || 'admin').trim().toLowerCase() || 'admin';
  state.serverSettings.can_write = data.can_write !== false;
  state.serverSettings.can_admin = data.can_admin !== false;
  state.serverSettings.can_root = data.can_root === true;
  if (isRootUser() && !isAllowedRootView(state.view)) {
    state.view = 'root-debug';
  }
  if (isVisitorUser() && state.view === 'trash') {
    state.view = 'timeline';
  }
  state.serverSettings.port = Number(data.port) || 8080;
  state.serverSettings.active_library_id = data.active_library_id || '';
  state.serverSettings.storage_path = data.storage_path || '';
  state.serverSettings.libraries = normalizeLibraryList(data.libraries, data.storage_path || '');
  state.settingsAvailabilitySignature = libraryAvailabilitySignature(state.serverSettings.libraries);
  state.serverSettings.thumbnail_dir = data.thumbnail_dir || '';
  state.serverSettings.thumbnail_size = 512;
  state.serverSettings.trash_dir = data.trash_dir || '';
  state.serverSettings.use_system_player = !!data.use_system_player;
  state.serverSettings.jwt_secret_masked = data.jwt_secret_masked || copyText('app.interface.s_55a04b58cd');
  state.serverSettings.users = Array.isArray(data.users) ? data.users : [];
  setThemeValue(currentThemePreference());
  state.gridSize = Math.min(260, Math.max(72, Number(data.grid_size) || 180));
  state.gridGap = Math.min(24, Math.max(0, Number(data.grid_gap) || 2));
  state.thumbRadius = Math.min(24, Math.max(0, Number(data.thumb_radius) || 2));
  state.slideshowMode = data.slideshow_mode === 'sequential' ? 'sequential' : 'random';
  state.slideshowLoop = data.slideshow_loop !== false;
  state.slideshowInterval = Math.min(30000, Math.max(1000, Number(data.slideshow_interval) || 5000));
  state.echoFrameSettings = data.echo_frame;
  resetLightboxZoomToDefault();
  state.experimentalAutoplayVideo = data.experimental_autoplay_video !== false;
  state.videoAutoplayNext = data.video_autoplay_next !== false;
  state.videoSectionMinMinutes = Math.min(240, Math.max(1, Number(data.video_section_min_minutes) || 10));
  state.throttledVideoSeek = !!data.throttled_video_seek;
  state.videoSeekThrottleMS = normalizeVideoSeekThrottleMS(data.video_seek_throttle_ms, 240);
  state.videoVolumeSwipeSensitivity = normalizeVideoVolumeSwipeSensitivity(data.video_volume_swipe_sensitivity, 100);
  state.videoVolumeMinPercent = normalizeVideoVolumePercent(data.video_volume_min_percent, 0);
  state.videoVolumeMaxPercent = Math.max(state.videoVolumeMinPercent, normalizeVideoVolumePercent(data.video_volume_max_percent, 100));
  state.experimentalPrefetchNeighbors = data.experimental_prefetch_neighbors !== false;
  state.experimentalRestoreLastView = !!data.experimental_restore_last_view;
  state.continueLastVideoPosition = false;
  state.serverSettings.warm_enabled = data.warm_enabled !== false;
  state.serverSettings.lightbox_ui_idle_seconds = normalizeLightboxUIIdleSeconds(data.lightbox_ui_idle_seconds, 0);
  state.serverSettings.low_resource_mode = !!data.low_resource_mode;
  persistGlobalVideoVolume(state.globalVideoVolume);
  state.serverSettings.pwa_icon_url = data.pwa_icon_url || '';
  state.serverSettings.local_update_config_text = data.local_update_config_text || renderLocalUpdateConfigPreview();
  if (state.serverSettings.pwa_icon_url) {
    state.pwaSettings = normalizePWASettings({
      ...state.pwaSettings,
      uploadedIconURL: state.serverSettings.pwa_icon_url,
    });
    persistPWASettings();
  }
  updatePlayerKeymapSource(data.player_keymap || '');
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  updateGridScaleProgress();
  applyDisplaySettings();
  applyLightboxZoom();
  updateSlideshowControls();
  syncSidebarUI();
  syncNavActiveView(state.view);
  syncRoleAwareNavigation();
  updateThemeBtn();
  applyLibraryBranding();
}
function buildSettingsPayload() {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  const activeLibrary = resolveActiveLibrary(libraries, state.serverSettings.active_library_id || '', state.serverSettings.storage_path || '');
  let activePath = (activeLibrary && activeLibrary.path || state.serverSettings.storage_path || '').trim();
  if (!activePath && libraries.length) activePath = libraries[0].path;
  return {
    app_name: state.serverSettings.app_name || currentAppDisplayName(),
    port: state.serverSettings.port || 8080,
    active_library_id: activeLibrary && activeLibrary.id || '',
    storage_path: activePath,
    libraries,
    thumbnail_dir: state.serverSettings.thumbnail_dir || '',
    thumbnail_size: 512,
    trash_dir: state.serverSettings.trash_dir || '',
    use_system_player: !!state.serverSettings.use_system_player,
    theme: state.theme || 'light',
    grid_size: state.gridSize,
    grid_gap: state.gridGap,
    thumb_radius: state.thumbRadius,
    sidebar_auto_hide: false,
    slideshow_mode: state.slideshowMode,
    slideshow_loop: !!state.slideshowLoop,
    slideshow_interval: state.slideshowInterval,
    echo_frame: typeof normalizeEchoFrameSettings === 'function' ? normalizeEchoFrameSettings(state.echoFrameSettings) : state.echoFrameSettings,
    lightbox_zoom: 100,
    experimental_autoplay_video: !!state.experimentalAutoplayVideo,
    video_autoplay_next: !!state.videoAutoplayNext,
    video_section_min_minutes: state.videoSectionMinMinutes || 10,
    warm_enabled: isWarmEnabled(),
    lightbox_ui_idle_seconds: normalizeLightboxUIIdleSeconds(state.serverSettings.lightbox_ui_idle_seconds, 0),
    throttled_video_seek: !!state.throttledVideoSeek,
    video_seek_throttle_ms: normalizeVideoSeekThrottleMS(state.videoSeekThrottleMS, 240),
    video_volume_swipe_sensitivity: normalizeVideoVolumeSwipeSensitivity(state.videoVolumeSwipeSensitivity, 100),
    video_volume_min_percent: normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0),
    video_volume_max_percent: Math.max(normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0), normalizeVideoVolumePercent(state.videoVolumeMaxPercent, 100)),
    experimental_prefetch_neighbors: !!state.experimentalPrefetchNeighbors,
    experimental_restore_last_view: !!state.experimentalRestoreLastView,
    continue_last_video_position: !!state.continueLastVideoPosition,
    low_resource_mode: !!state.serverSettings.low_resource_mode,
    player_keymap: state.playerKeymapSource || '',
  };
}
async function persistSettings() {
  if (!state.settingsReady) return null;
  return saveServerSettings(buildSettingsPayload());
}
async function uploadLibraryLogo(index, file) {
  const form = new FormData();
  form.append('file', file);
  const resp = await api.upload(`/api/settings/libraries/${index}/logo`, form);
  state.serverSettings = resp.data || state.serverSettings;
  applyServerSettings(state.serverSettings);
  state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
  return resp;
}
async function uploadPWAIcon(file) {
  const form = new FormData();
  form.append('file', file);
  const resp = await api.upload('/api/settings/pwa/icon', form);
  state.serverSettings.pwa_icon_url = (resp && resp.icon_url) || state.serverSettings.pwa_icon_url || '';
  state.pwaSettings = normalizePWASettings({
    ...state.pwaSettings,
    iconSource: 'upload',
    uploadedIconURL: state.serverSettings.pwa_icon_url,
  });
  persistPWASettings();
  return resp;
}
async function deleteLibraryLogo(index) {
  const resp = await api.del(`/api/settings/libraries/${index}/logo`);
  state.serverSettings = resp.data || state.serverSettings;
  applyServerSettings(state.serverSettings);
  state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
  return resp;
}
async function refreshLibraryLogos(replaceExisting = false) {
  const resp = await api.post('/api/settings/libraries/logos/refresh', { replace_existing: !!replaceExisting });
  state.serverSettings = resp.data || state.serverSettings;
  applyServerSettings(state.serverSettings);
  state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
  return resp;
}
async function openWithSystemPlayer(photoId) {
  await api.post(`/api/media/${photoId}/play`, {});
}
let settingsBootstrapPromise = null;
async function ensureSettingsDataLoaded(force = false) {
  if (force) {
    state.settingsReady = false;
    state.settingsMounted = false;
    state.settingsRenderMode = '';
    settingsBootstrapPromise = null;
  }
  if (state.settingsReady) return;
  if (!settingsBootstrapPromise) {
    settingsBootstrapPromise = loadServerSettings().finally(() => {
      state.settingsReady = true;
      settingsBootstrapPromise = null;
    });
  }
  await settingsBootstrapPromise;
}

function toggleVideoPlayback() {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  if (video.paused) video.play().catch(() => {});
  else video.pause();
  syncTouchPlaybackButton(video);
  updateMediaSessionPlaybackState(video);
  showVideoProgressActivity(3000);
  return true;
}
function stepVideoFrame(direction) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  video.pause();
  video.currentTime = Math.max(0, Math.min(video.duration || Infinity, video.currentTime + direction / 30));
  return true;
}
function adjustVideoTime(seconds) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  video.currentTime = Math.max(0, Math.min(video.duration || Infinity, video.currentTime + seconds));
  updateVideoBookmarkProgress(video);
  showVideoProgressActivity();
  return true;
}
function jumpVideoSection(direction) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden') || !Number.isFinite(video.duration) || video.duration <= 0) return false;
  const sectionMinSeconds = Math.max(60, (Number(state.videoSectionMinMinutes) || 10) * 60);
  if (video.duration < sectionMinSeconds) return adjustVideoTime(direction > 0 ? 5 : -5);
  const sectionLength = video.duration / 10;
  const current = Math.max(0, Math.min(video.duration, Number(video.currentTime) || 0));
  let targetIndex;
  if (direction > 0) {
    targetIndex = Math.floor(current / sectionLength) + 1;
  } else {
    targetIndex = Math.ceil(current / sectionLength) - 1;
    if (current - Math.floor(current / sectionLength) * sectionLength < 1.25) targetIndex -= 1;
  }
  targetIndex = Math.max(0, Math.min(9, targetIndex));
  video.currentTime = targetIndex * sectionLength;
  updateVideoBookmarkProgress(video);
  showVideoProgressActivity();
  showToast(copyText('app.interface.d_baa8b5e07c', { v0: (targetIndex + 1) }));
  return true;
}
function adjustVideoVolume(delta) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  const sensitivity = normalizeVideoVolumeSwipeSensitivity(state.videoVolumeSwipeSensitivity, 100) / 100;
  setVideoVolumeValue(effectiveVideoVolume(video) + (delta / 100) * sensitivity, video);
  return true;
}
function setVideoVolumeValue(value, video = $('#lb-video')) {
  if (!video || video.classList.contains('hidden')) return false;
  applyVideoVolumeOutput(video, value);
  video.muted = false;
  showVolumeOverlay(video);
  persistVideoPlaybackPreferenceSoon(video);
  return true;
}
function setVideoSpeed(rate) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  video.playbackRate = Math.max(0.1, Math.min(16, rate));
  return true;
}
function toggleVideoMute() {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  video.muted = !video.muted;
  showVolumeOverlay(video);
  persistVideoPlaybackPreferenceSoon(video);
  return true;
}

function showVolumeOverlay(video = $('#lb-video')) {
  if (!video) return;
  const overlay = $('#lb-volume-feedback');
  const fill = $('#lb-volume-fill');
  const label = $('#lb-volume-label');
  if (!overlay || !fill || !label) return;
  const value = video.muted ? 0 : Math.round(effectiveVideoVolume(video) * 100);
  fill.style.width = `${value}%`;
  label.textContent = video.muted ? copyText('app.interface.s_afdbd1ddec') : `${value}%`;
  overlay.classList.add('show');
  clearTimeout(state.lightboxVolumeTimer);
  state.lightboxVolumeTimer = setTimeout(() => overlay.classList.remove('show'), 1100);
}
function isWindowsClient() {
  const uaDataPlatform = navigator.userAgentData && navigator.userAgentData.platform || '';
  const platform = navigator.platform || '';
  const userAgent = navigator.userAgent || '';
  return /win/i.test(`${uaDataPlatform} ${platform} ${userAgent}`);
}
async function autoplayLightboxVideo(video) {
  if (!video) return false;
  const token = ++state.lightboxPlaybackToken;
  const originalMuted = video.muted;
  const preferMutedFirst = false;
  const setMutedState = muted => {
    video.muted = muted;
    if (muted) video.setAttribute('muted', '');
    else video.removeAttribute('muted');
  };
  const tryPlay = async muted => {
    if (state.lightboxPlaybackToken !== token) return false;
    setMutedState(muted);
    try {
      await video.play();
      return state.lightboxPlaybackToken === token;
    } catch (_) {
      return false;
    }
  };

  let played = false;
  if (preferMutedFirst) played = await tryPlay(true);
  if (!played) played = await tryPlay(originalMuted);
  if (!played && !preferMutedFirst) played = await tryPlay(true);

  if (!played) {
    setMutedState(originalMuted);
    return false;
  }
  if (video.muted && !originalMuted && isWindowsClient() && !state.autoplayMutedHintShown) {
    state.autoplayMutedHintShown = true;
    showToast(copyText('app.interface.s_516fde2c99'));
  }
  return true;
}
function executePlayerKeyAction(binding) {
  if (!binding) return false;
  const value = parseFloat(binding.args[0] || '0');
  switch (binding.command) {
    case 'cycle':
      if (binding.args[0] === 'pause') return toggleVideoPlayback();
      if (binding.args[0] === 'mute') return toggleVideoMute();
      return false;
    case 'seek':
      return adjustVideoTime(value);
    case 'frame-step':
      return stepVideoFrame(1);
    case 'frame-back-step':
      return stepVideoFrame(-1);
    case 'add':
      if (binding.args[0] === 'volume') return adjustVideoVolume(parseFloat(binding.args[1] || '0'));
      return false;
    case 'set':
      if (binding.args[0] === 'speed') return setVideoSpeed(parseFloat(binding.args[1] || '1'));
      return false;
    default:
      return false;
  }
}
function keyEventToken(e) {
  const raw = e.key && e.key.length === 1 ? e.key.toUpperCase() : e.key.toUpperCase();
  const base = raw === ' ' ? 'SPACE' : normalizeKeyToken(raw);
  const modifiers = [];
  if (e.ctrlKey && base !== 'CTRL') modifiers.push('CTRL');
  if (e.altKey && base !== 'ALT') modifiers.push('ALT');
  if (e.shiftKey && base !== 'SHIFT') modifiers.push('SHIFT');
  if (e.metaKey && base !== 'META') modifiers.push('META');
  return keyTokenWithModifiers(base, modifiers);
}
function handleLightboxKeydown(e) {
  if (shouldIgnoreGlobalShortcut(e.target)) return false;
  if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.key === '0') {
    e.preventDefault();
    e.stopImmediatePropagation();
    setLightboxFit();
    return true;
  }
  if (handleVideoBookmarkShortcut(e)) return true;
  if (isVideoMedia(state.lightboxPhotos[state.lightboxIndex]) && (e.key === 'q' || e.key === 'Q' || e.key === 'e' || e.key === 'E')) {
    e.preventDefault();
    e.stopImmediatePropagation();
    return jumpVideoSection(e.key.toLowerCase() === 'e' ? 1 : -1);
  }
  if (e.key === 'Escape') {
    e.preventDefault();
    e.stopImmediatePropagation();
    exitLightboxToContext();
    return true;
  }
  const binding = state.playerKeymap[keyEventToken(e)];
  if (isVideoMedia(state.lightboxPhotos[state.lightboxIndex]) && executePlayerKeyAction(binding)) {
    e.preventDefault();
    e.stopImmediatePropagation();
    return true;
  }
  if (e.key === 'f' || e.key === 'F') {
    e.preventDefault();
    e.stopImmediatePropagation();
    if (e.repeat) return true;
    startLightboxFavoriteKeyHold();
    return true;
  }
  if (e.key === 'a' || e.key === 'A') {
    e.preventDefault();
    e.stopImmediatePropagation();
    void lbNav(-1);
    return true;
  }
  if (e.key === 'd' || e.key === 'D') {
    e.preventDefault();
    e.stopImmediatePropagation();
    void lbNav(1);
    return true;
  }
  if (e.key === ' ') {
    e.preventDefault();
    e.stopImmediatePropagation();
    toggleSlideshow();
    return true;
  }
  return false;
}
function renderGridScaleButton() {
  const label = copyText('app.interface.d_5e4f5cc905', { v0: (state.gridSize) });
  return `<div class="floating-grid-scale-control" id="grid-scale-control">
    <button class="floating-search-btn floating-grid-scale-btn" id="grid-scale-btn" type="button" aria-label="${escapeHTML(label)}" title="${escapeHTML(label)}">${icons.gridScaleButton || icons.search}</button>
    <label class="floating-grid-scale-slider" for="floating-grid-scale-input" aria-label="${escapeHTML(copyText('app.interface.runtime.gridScaleAria'))}">
      <input id="floating-grid-scale-input" type="range" min="72" max="260" step="8" value="${state.gridSize}">
    </label>
  </div>`;
}
function renderTimelineOrderControl() {
  const ascending = state.timelineOrder === 'asc';
  const label = ascending ? copyText('app.interface.s_e359f80620') : copyText('app.interface.s_f0e1d1b31a');
  return renderTopbarGlassButton({
    id: 'timeline-order-btn',
    icon: icons.topbarTimelineOrder || icons.timelineOrder,
    label,
    className: `timeline-order-btn ${ascending ? 'asc' : 'desc'}`,
    variant: 'accent',
    extraAttrs: `aria-pressed="${ascending ? 'true' : 'false'}"`,
  });
}
function renderAlbumViewModeControl() {
  return `<div class="topbar-media-filter album-view-toggle" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.albumViewAria'))}">
    <button class="media-filter-btn album-view-btn ${state.albumViewMode === 'grid' ? 'active' : ''}" id="album-view-grid-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.albumViewGrid'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.albumViewGrid'))}" aria-pressed="${state.albumViewMode === 'grid' ? 'true' : 'false'}">${icons.albumViewGrid}</button>
    <button class="media-filter-btn album-view-btn ${state.albumViewMode === 'list' ? 'active' : ''}" id="album-view-list-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.albumViewList'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.albumViewList'))}" aria-pressed="${state.albumViewMode === 'list' ? 'true' : 'false'}">${icons.albumViewList}</button>
  </div>`;
}
function renderAlbumDetailSortControl() {
  const active = normalizeAlbumDetailSort(state.albumDetailSort);
  const items = [
    { key: 'timeline_desc', label: copyText('app.interface.s_f0e1d1b31a'), icon: icons.albumSortTimelineDesc },
    { key: 'timeline_asc', label: copyText('app.interface.s_e359f80620'), icon: icons.albumSortTimelineAsc },
    { key: 'name', label: copyText('app.interface.s_f0cbb52913'), icon: icons.albumSortName },
    { key: 'size', label: copyText('app.interface.s_2ad8b0b96f'), icon: icons.albumSortSize },
  ];
  return `<div class="topbar-media-filter album-detail-sort-filter" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.albumSortAria'))}">
    ${items.map(item => `<button class="media-filter-btn album-detail-sort-btn ${active === item.key ? 'active' : ''}" type="button" data-album-detail-sort="${item.key}" title="${item.label}" aria-label="${item.label}" aria-pressed="${active === item.key ? 'true' : 'false'}">${item.icon || ''}</button>`).join('')}
  </div>`;
}
function renderAlbumDetailBottomActionControls() {
  return `<div class="album-detail-bottom-center">
    ${renderAlbumDetailSortControl()}
    ${renderMediaKindFilterControl('album-detail-filter')}
  </div>`;
}
function canRevealInFileManager() {
  if (!canManageLibraries()) return false;
  const hostname = window.location.hostname.toLowerCase().replace(/\.$/, '');
  if (hostname === 'localhost' || hostname.endsWith('.localhost') || hostname === '[::1]' || hostname === '::1') return true;
  const parts = hostname.split('.');
  return parts.length === 4 && parts[0] === '127' &&
    parts.every(part => /^\d{1,3}$/.test(part) && Number(part) <= 255);
}

async function revealInFinder(photoId) {
  if (!canRevealInFileManager()) return;
  try {
    await api.post(`/api/media/${photoId}/reveal`, {});
    showToast(copyText('app.interface.s_da62b08006'));
  } catch (e) {
    alert(copyText('app.interface.s_62bf2d195f') + (e.error || e));
  }
}
async function revealAlbumInFinder(albumId) {
  if (!canRevealInFileManager()) return;
  try {
    await api.post(`/api/media/albums/${albumId}/reveal`, {});
    showToast(copyText('app.interface.s_da62b08006'));
  } catch (e) {
    alert(copyText('app.interface.s_62bf2d195f') + (e.error || e));
  }
}

// ── 状态 ──────────────────────────────────────────────
const state = {
  view: 'timeline',
  theme: 'light',
  gridSize: 180,
  photos: [],
  randomAlbumPhotos: [],
  randomAlbumLoading: false,
  randomAlbumCursor: '',
  randomAlbumHasMore: true,
  randomAlbumSeed: Date.now(),
  randomAlbumLoaded: false,
  randomAlbumViewLoaded: false,
  randomAlbumTotal: 0,
  randomAlbumAbortController: null,
  pendingRandomAlbumPhotoID: null,
  randomAlbumLastViewedPhotoID: null,
  timelineOrder: normalizeTimelineOrder(localStorage.getItem(timelineOrderStorageKey)),
  timelineCursor: '',
  timelineHasMore: true,
  timelineHasBefore: false,
  timelineLoading: false,
  timelineLoadingBefore: false,
  timelineTotal: 0,
  timelineLoaded: false,
  timelineLocateWindow: null,
  timelineLocateTargetIndex: -1,
  timelineLocateMissingBeforeCount: 0,
  timelineLocatingActive: false,
  trashPhotos: [],
  trashCursor: '',
  trashHasMore: true,
  trashLoading: false,
  trashLoaded: false,
  trashAlbums: [],
  trashAlbumsLoaded: false,
  trashFolderSelection: new Set(),
  trashContentFilter: 'media',
  albums: [],
  albumsLoaded: false,
  albumViewMode: 'grid',
  albumKindFilter: normalizeAlbumKindFilter(localStorage.getItem(albumKindFilterStorageKey)),
  albumDetailSort: normalizeAlbumDetailSort(localStorage.getItem(albumDetailSortStorageKey)),
  currentAlbum: null,
  albumPhotos: [],
  albumCursor: '',
  albumHasMore: true,
  albumHasBefore: false,
  albumLoading: false,
  albumLoadingBefore: false,
  albumLocateMissingBeforeCount: 0,
  albumLocatingActive: false,
  albumDetailLoadedKey: '',
  pendingAlbumPhotoID: null,
  lastAlbumDetailID: null,
  activeRangePointer: null,
  selected: new Set(),
  selectionAnchorID: null,
  lightboxPhotos: [],
  lightboxIndex: 0,
  lightboxLastRememberedPhotoID: null,
  lightboxSeedPhotoID: null,
  lightboxSeedPreviewURL: '',
  lightboxOriginThumbRect: null,
  lightboxDismissProxyActive: false,
  lightboxDismissProxyRect: null,
  lightboxDismissProxySourceRect: null,
  lightboxDismissProxyTargetRect: null,
  lightboxReturnView: '',
  lightboxReturnAlbumID: null,
  lightboxPageLoading: false,
  lightboxSkipScrollRestore: false,
  // b-2: 当前用户的分享链接，key=`${type}:${targetId}`
  shareMap: {},
  shareLinks: [],
  shareLinksLoaded: false,
  shareLinksLoading: false,
  // d-2: 上传队列状态
  uploadJobs: [],
  uploadRunning: false,
  // h-1: 用于刷新后恢复相册详情页
  currentAlbumID: null,
  sidebarCompact: false,
  slideshowPlaying: false,
  slideshowMode: 'random',
  slideshowLoop: true,
  slideshowInterval: 3000,
  slideshowTimer: null,
  slideshowRandomQueue: [],
  slideshowPreloadPromise: null,
  pendingTimelinePhotoID: null,
  timelineJumpToken: 0,
  timelineJumpCancelRequested: false,
  focusedPhotoID: null,
  lightboxZoom: 100,
  lightboxZoomMode: 'fit',
  lightboxBoostActive: false,
  lightboxFocusPoint: {
    mediaX: 0.5,
    mediaY: 0.5,
    viewportX: 0.5,
    viewportY: 0.5,
  },
  lightboxPointerClientX: 0,
  lightboxPointerClientY: 0,
  lightboxPointerInside: false,
  pinchGesture: null,
  lightboxPinchZoomActive: false,
  lightboxUnderzoomProgress: 0,
  lightboxUnderzoomExitTimer: null,
  lightboxDismissProgress: 0,
  lightboxDismissResetTimer: null,
  lightboxViewportOrientation: '',
  lightboxViewportResizeTimer: null,
  lightboxDebugControls: false,
  gridGap: 2,
  thumbRadius: 2,
  serverSettings: {
    current_username: '',
    role: 'admin',
    can_write: true,
    can_admin: true,
    port: 8080,
    active_library_id: '',
    storage_path: '',
    libraries: [],
    thumbnail_dir: '',
    thumbnail_size: 512,
    trash_dir: '',
    use_system_player: false,
    jwt_secret_masked: '',
    users: [],
    theme: 'light',
    grid_size: 180,
    grid_gap: 2,
    thumb_radius: 2,
    sidebar_auto_hide: false,
    slideshow_mode: 'random',
    slideshow_loop: true,
    slideshow_interval: 5000,
    lightbox_zoom: 100,
    experimental_autoplay_video: true,
    video_autoplay_next: true,
    video_section_min_minutes: 10,
    experimental_prefetch_neighbors: true,
    experimental_restore_last_view: false,
    continue_last_video_position: true,
    warm_enabled: true,
    lightbox_ui_idle_seconds: 0,
    low_resource_mode: false,
    player_keymap: '',
  },
  savedServerSettings: null,
  playerKeymap: {},
  playerKeymapSource: '',
  experimentalAutoplayVideo: true,
  videoAutoplayNext: true,
  videoSectionMinMinutes: 10,
  throttledVideoSeek: false,
  videoSeekThrottleMS: 240,
  videoVolumeSwipeSensitivity: 100,
  videoVolumeMinPercent: 0,
  videoVolumeMaxPercent: 100,
  experimentalPrefetchNeighbors: true,
  experimentalRestoreLastView: false,
  continueLastVideoPosition: true,
  favoritePhotos: [],
  favoriteCursor: '',
  favoriteHasMore: true,
  favoriteLoading: false,
  favoriteTotal: 0,
  favoriteLoaded: false,
  timelineAutoLoadPaused: false,
  randomAlbumAutoLoadPaused: false,
  memoryPhotos: [],
  memoryEntries: [],
  memoriesLoading: false,
  memoriesLoaded: false,
  settingsDirty: false,
  libraryBatchAddModifierHeld: false,
  libraryBatchAddShortcutBound: false,
  settingsReady: false,
  settingsMounted: false,
  settingsRenderMode: '',
  settingsAvailabilityPollTimer: null,
  settingsAvailabilityPollPending: false,
  settingsAvailabilitySignature: '',
  rootConsoleUsers: [],
  rootConsoleLoaded: false,
  settingsFocus: '',
  settingsScrollRestorePending: true,
  lightboxPlaybackToken: 0,
  lightboxMediaLoading: false,
  lightboxPreviousThemeColor: '',
  lightboxHadThemeColorMeta: true,
  lightboxUiIdleTimer: null,
  lightboxInfoInteractionTimer: null,
  lightboxVolumeTimer: null,
  lightboxVideoAudioContext: null,
  lightboxVideoAudioSource: null,
  lightboxVideoGainNode: null,
  lightboxVideoAudioElement: null,
  lightboxVisibilityResumePending: false,
  lightboxVisibilityResumeTime: 0,
  lightboxFavoriteKeyHoldTimer: null,
  lightboxFavoriteKeyHoldActive: false,
  lightboxFavoriteKeyHoldTriggered: false,
  lightboxVideoProgressTimer: null,
  pendingVideoSeek: null,
  pendingVideoSeekCommitTimer: null,
  videoProgressScrubPointerId: null,
  videoProgressClickSuppressUntil: 0,
  videoSurfaceLastTap: null,
  videoSurfaceTapTimer: null,
  videoSurfaceSwipe: null,
  lightboxSwipe: null,
  lightboxSwipeClickSuppressUntil: 0,
  lightboxLastPointerType: '',
  autoplayMutedHintShown: false,
  viewScrollPositions: {},
  loadMoreObservers: {},
  gridScalePersistTimer: null,
  prefetchedMediaKeys: [],
  prefetchedMediaSet: new Set(),
  warmedThumbnailKeys: [],
  warmedThumbnailSet: new Set(),
  searchOpen: false,
  searchQuery: '',
  searchFilter: 'all',
  searchResults: [],
  searchAlbumResults: [],
  searchAlbumCache: null,
  searchCursor: '',
  searchHasMore: false,
  searchLoading: false,
  searchTotal: 0,
  searchTimer: null,
  searchAbortController: null,
  mediaKindFilter: normalizeMediaKindFilter(localStorage.getItem(mediaKindFilterStorageKey)),
  pwaSettings: loadPWASettings(),
  updateSettings: loadUpdateSettings(),
  localUpdateCheckReady: false,
  localUpdateHasUpdate: false,
  legacyVideoBookmarks: loadLegacyVideoBookmarks(),
  videoBookmarkSaveTimer: null,
  globalVideoVolume: loadGlobalVideoVolume(),
  videoPlaybackPreferences: {},
  videoPlaybackPreferenceRequests: {},
  videoPlaybackPreferenceSaveQueue: {},
  videoPlaybackPreferenceSaveTimers: {},
  activePreloadJobs: new Set(),
  visibleThumbnailWarmSignatures: {},
  libraryBuildStatus: { status: 'idle', message: '' },
  libraryBuildPollTimer: null,
  libraryBatchBuildStatus: { status: 'idle', message: '' },
  libraryBatchBuildPollTimer: null,
  libraryBatchBuildCancelPending: false,
  libraryBatchWorkflowEnabled: localStorage.getItem(libraryBatchWorkflowEnabledStorageKey) === '1',
  libraryBatchWorkflowAggressive: localStorage.getItem(libraryBatchWorkflowAggressiveStorageKey) === '1',
  libraryBatchWorkflowPhase: localStorage.getItem(libraryBatchWorkflowPhaseStorageKey) || '',
  libraryBatchWorkflowStartPending: false,
  libraryBatchThumbnailBuildStatus: { status: 'idle', message: '' },
  libraryBatchThumbnailBuildPollTimer: null,
  libraryBatchThumbnailBuildCancelPending: false,
  playbackCacheBuildStatus: { status: 'idle', message: '' },
  playbackCacheBuildPollTimer: null,
  playbackCacheBuildCancelPending: false,
  playbackCacheItems: [],
  playbackCacheItemsLoaded: false,
  playbackCacheSelected: new Set(),
  thumbnailBuildStatus: { status: 'idle', message: '' },
  thumbnailBuildPollTimer: null,
  thumbnailBuildCancelPending: false,
  thumbnailBuildOverlaySuppressed: false,
  thumbnailVisibleRefreshAt: 0,
  videoThumbnailRefreshStatus: { status: 'idle', message: '' },
  videoThumbnailRefreshPollTimer: null,
  videoThumbnailRefreshCancelPending: false,
  exifBackfillStatus: { status: 'idle', message: '' },
  exifBackfillPollTimer: null,
  exifBackfillCancelPending: false,
  blockingInteraction: false,
  timelineBulkLoading: false,
  timelineBulkCancelRequested: false,
  timelineBulkAbortController: null,
  randomAlbumBulkLoading: false,
  randomAlbumBulkCancelRequested: false,
  randomAlbumBulkAbortController: null,
  timelineLoadAbortController: null,
  lightboxPageScrollTop: 0,
  pendingLibraryLogoGuide: null,
  pendingLibraryLogoCrop: null,
  libraryRandomLogos: {},
  libraryRandomLogoTried: {},
  libraryRandomLogoLoading: false,
  libraryLogosRefreshStarted: false,
  albumEntrySourceView: '',
  albumEntrySourcePhotoID: null,
  pendingScrollRestoreID: '',
  pendingScrollRestoreStartedAt: 0,
  ignoreScrollSaveUntil: 0,
  programmaticScrollTargetTop: 0,
  lastManualScrollAt: 0,
  lastManualScrollTop: 0,
  albumChildrenIndex: null,
};

function viewScrollKeyFor(view = state.view, albumID = state.currentAlbumID) {
  if (view === 'album-detail') return `album-detail:${albumID || 0}`;
  return view;
}
function saveViewScroll(view = state.view, albumID = state.currentAlbumID) {
  state.viewScrollPositions[viewScrollKeyFor(view, albumID)] = window.scrollY || window.pageYOffset || 0;
}
function resetTimelineToInitialPage() {
  state.photos = [];
  state.timelineCursor = '';
  state.timelineHasMore = true;
  state.timelineHasBefore = false;
  state.timelineLoading = false;
  state.timelineLoadingBefore = false;
  state.timelineTotal = 0;
  state.timelineLoaded = false;
  state.timelineLocateWindow = null;
  state.timelineLocateTargetIndex = -1;
  state.timelineLocateMissingBeforeCount = 0;
  state.timelineLocatingActive = false;
  state.pendingTimelinePhotoID = null;
  state.timelineJumpCancelRequested = false;
  state.viewScrollPositions[viewScrollKeyFor('timeline')] = 0;
}
function timelineUsesManualBeforeLoading() {
  return timelinePlaceholderCount() > 0;
}
function albumUsesManualBeforeLoading() {
  return albumPlaceholderCount() > 0;
}
function noteManualViewScroll() {
  const top = window.scrollY || window.pageYOffset || 0;
  if (Date.now() < state.ignoreScrollSaveUntil && Math.abs(top - state.programmaticScrollTargetTop) <= 2) {
    state.lastManualScrollTop = top;
    saveViewScroll();
    return;
  }
  const scrollingUp = top < (Number(state.lastManualScrollTop) || 0);
  state.lastManualScrollTop = top;
  state.lastManualScrollAt = Date.now();
  saveViewScroll();
  if (scrollingUp) {
    maybeTriggerTimelineBeforeLoadFromVisibleBoundary({ source: 'manual-scroll' });
    maybeTriggerAlbumBeforeLoadFromVisibleBoundary({ source: 'manual-scroll' });
  }
}

function firstVisibleTimelineRealThumb() {
  const thumbs = $$('#timeline-grid .photo-thumb[data-id]').filter(thumb => thumb.offsetParent !== null && !thumb.dataset.placeholder);
  if (!thumbs.length) return null;
  let best = null;
  let bestTop = Infinity;
  thumbs.forEach(thumb => {
    const rect = thumb.getBoundingClientRect();
    if (rect.bottom <= 0) return;
    if (rect.top < bestTop) {
      best = thumb;
      bestTop = rect.top;
    }
  });
  return best;
}

function firstVisibleTimelinePlaceholder() {
  const placeholders = $$('#timeline-grid .timeline-placeholder-thumb').filter(thumb => thumb.offsetParent !== null);
  if (!placeholders.length) return null;
  let best = null;
  let bestTop = Infinity;
  placeholders.forEach(thumb => {
    const rect = thumb.getBoundingClientRect();
    if (rect.bottom <= 0) return;
    if (rect.top < bestTop) {
      best = thumb;
      bestTop = rect.top;
    }
  });
  return best;
}

function firstVisibleAlbumRealThumb() {
  const thumbs = $$('#album-grid .photo-thumb[data-id]').filter(thumb => thumb.offsetParent !== null && !thumb.dataset.placeholder);
  if (!thumbs.length) return null;
  let best = null;
  let bestTop = Infinity;
  thumbs.forEach(thumb => {
    const rect = thumb.getBoundingClientRect();
    if (rect.bottom <= 0) return;
    if (rect.top < bestTop) {
      best = thumb;
      bestTop = rect.top;
    }
  });
  return best;
}

function firstVisibleAlbumPlaceholder() {
  const placeholders = $$('#album-grid .timeline-placeholder-thumb').filter(thumb => thumb.offsetParent !== null);
  if (!placeholders.length) return null;
  let best = null;
  let bestTop = Infinity;
  placeholders.forEach(thumb => {
    const rect = thumb.getBoundingClientRect();
    if (rect.bottom <= 0) return;
    if (rect.top < bestTop) {
      best = thumb;
      bestTop = rect.top;
    }
  });
  return best;
}

function maybeTriggerTimelineBeforeLoadFromVisibleBoundary(options = {}) {
  if (state.view !== 'timeline') return;
  if (!timelinePlaceholderCount()) return;
  if (state.timelineLoadingBefore || state.timelineLoading || state.timelineBulkLoading || state.timelineAutoLoadPaused || !state.timelineHasBefore) return;
  const manualMode = timelineUsesManualBeforeLoading();
  if (manualMode) {
    if (options.source !== 'manual-scroll') return;
    const placeholder = firstVisibleTimelinePlaceholder();
    if (!placeholder) return;
    const rect = placeholder.getBoundingClientRect();
    if (rect.bottom > 0) {
      void loadMoreTimelineBefore();
    }
    return;
  }
  const firstRealThumb = firstVisibleTimelineRealThumb();
  if (!firstRealThumb) return;
  const rect = firstRealThumb.getBoundingClientRect();
  const threshold = Math.max(220, (window.innerHeight || 0) * 0.72);
  if (rect.top <= threshold) {
    void loadMoreTimelineBefore();
  }
}

function maybeTriggerAlbumBeforeLoadFromVisibleBoundary(options = {}) {
  if (state.view !== 'album-detail') return;
  if (!albumPlaceholderCount()) return;
  if (state.albumLoadingBefore || state.albumLoading || !state.albumHasBefore) return;
  const manualMode = albumUsesManualBeforeLoading();
  if (manualMode) {
    if (options.source !== 'manual-scroll') return;
    const placeholder = firstVisibleAlbumPlaceholder();
    if (!placeholder) return;
    const rect = placeholder.getBoundingClientRect();
    if (rect.bottom > 0) {
      void loadMoreAlbumBefore();
    }
    return;
  }
  const firstRealThumb = firstVisibleAlbumRealThumb();
  if (!firstRealThumb) return;
  const rect = firstRealThumb.getBoundingClientRect();
  const threshold = Math.max(220, (window.innerHeight || 0) * 0.72);
  if (rect.top <= threshold) {
    void loadMoreAlbumBefore();
  }
}
function restoreViewScroll(view = state.view, albumID = state.currentAlbumID) {
  const key = viewScrollKeyFor(view, albumID);
  const top = state.viewScrollPositions[key] || 0;
  const restoreID = `${view}:${albumID || 0}:${Date.now()}:${Math.random()}`;
  state.pendingScrollRestoreID = restoreID;
  state.pendingScrollRestoreStartedAt = Date.now();
  const stillCurrent = () => {
    if (view !== state.view) return;
    if (view === 'album-detail' && albumID !== state.currentAlbumID) return;
    if (state.lastManualScrollAt > state.pendingScrollRestoreStartedAt + 24) return;
    return state.pendingScrollRestoreID === restoreID;
  };
  const apply = () => {
    if (!stillCurrent()) return;
    state.ignoreScrollSaveUntil = Date.now() + 120;
    state.programmaticScrollTargetTop = top;
    state.lastManualScrollTop = top;
    window.scrollTo(0, top);
  };
  requestAnimationFrame(() => {
    apply();
    requestAnimationFrame(apply);
  });
  [80, 180, 360, 700, 1100].forEach(delay => window.setTimeout(apply, delay));
}

// ── 分享状态加载 ─────────────────────────────────────
async function loadShareMap() {
  if (!state.settingsReady) await ensureSettingsDataLoaded();
  if (state.shareLinksLoaded || state.shareLinksLoading) return;
  state.shareLinksLoading = true;
  if (!canManageShareLinks()) {
    setShareLinks([]);
    state.shareLinksLoaded = true;
    state.shareLinksLoading = false;
    return;
  }
  try {
		const links = await api.get('/api/media/shares');
    setShareLinks(links || []);
    state.shareLinksLoaded = true;
  } catch(e) { /* 非关键，忽略 */ }
  finally {
    state.shareLinksLoading = false;
  }
}

function disconnectLoadMoreObserver() {
  Object.values(state.loadMoreObservers || {}).forEach(observer => {
    if (observer && typeof observer.disconnect === 'function') observer.disconnect();
  });
  state.loadMoreObservers = {};
}

function disconnectSingleLoadMoreObserver(id) {
  const observer = state.loadMoreObservers && state.loadMoreObservers[id];
  if (observer && typeof observer.disconnect === 'function') {
    observer.disconnect();
  }
  if (state.loadMoreObservers) delete state.loadMoreObservers[id];
}

function resetLightboxPrefetchCache() {
  state.prefetchedMediaKeys = [];
  state.prefetchedMediaSet.clear();
}

function rememberPrefetchedMedia(key) {
  if (!key || state.prefetchedMediaSet.has(key)) return false;
  state.prefetchedMediaSet.add(key);
  state.prefetchedMediaKeys.push(key);
  while (state.prefetchedMediaKeys.length > 72) {
    const expired = state.prefetchedMediaKeys.shift();
    if (expired) state.prefetchedMediaSet.delete(expired);
  }
  return true;
}

function rememberWarmedThumbnail(key) {
  if (!key || state.warmedThumbnailSet.has(key)) return false;
  state.warmedThumbnailSet.add(key);
  state.warmedThumbnailKeys.push(key);
  while (state.warmedThumbnailKeys.length > 640) {
    const expired = state.warmedThumbnailKeys.shift();
    if (expired) state.warmedThumbnailSet.delete(expired);
  }
  return true;
}

function showBlockingProgress(title, detail = '', options = {}) {
  const nonBlocking = !!options.nonBlocking;
  state.blockingInteraction = !nonBlocking;
  let overlay = $('#blocking-progress');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.id = 'blocking-progress';
    overlay.className = 'blocking-progress';
    overlay.innerHTML = `<div class="blocking-progress-card"><div class="spinner"></div><strong></strong><span></span><div class="blocking-progress-track" hidden><div></div></div><button class="btn btn-sm blocking-progress-cancel" type="button" hidden>${copyText('app.interface.commonCancel')}</button></div>`;
    document.body.appendChild(overlay);
  }
  $('strong', overlay).textContent = title;
  $('span', overlay).textContent = detail;
  const track = $('.blocking-progress-track', overlay);
  if (track) {
    const percent = Math.max(0, Math.min(100, Number(options.percent) || 0));
    track.hidden = !Number.isFinite(Number(options.percent));
    const fill = $('div', track);
    if (fill) fill.style.width = `${percent}%`;
  }
  overlay.classList.toggle('non-blocking', nonBlocking);
  const cancelBtn = $('.blocking-progress-cancel', overlay);
  if (cancelBtn) {
    if (typeof options.onCancel === 'function') {
      cancelBtn.hidden = false;
      cancelBtn.textContent = options.cancelText || copyText('app.interface.commonCancel');
      cancelBtn.onclick = options.onCancel;
    } else {
      cancelBtn.hidden = true;
      cancelBtn.onclick = null;
    }
  }
  overlay.classList.add('show');
}
function updateBlockingProgress(detail = '') {
  const overlay = $('#blocking-progress');
  if (!overlay) return;
  $('span', overlay).textContent = detail;
}
function hideBlockingProgress() {
  state.blockingInteraction = false;
  const overlay = $('#blocking-progress');
  if (overlay) overlay.classList.remove('show', 'non-blocking');
}

function stopThumbnailBuildPolling() {
  if (state.thumbnailBuildPollTimer) {
    clearTimeout(state.thumbnailBuildPollTimer);
    state.thumbnailBuildPollTimer = null;
  }
}

function stopLibraryBatchBuildPolling() {
  if (state.libraryBatchBuildPollTimer) {
    clearTimeout(state.libraryBatchBuildPollTimer);
    state.libraryBatchBuildPollTimer = null;
  }
}

function stopLibraryBatchThumbnailBuildPolling() {
  if (state.libraryBatchThumbnailBuildPollTimer) {
    clearTimeout(state.libraryBatchThumbnailBuildPollTimer);
    state.libraryBatchThumbnailBuildPollTimer = null;
  }
}

function stopVideoThumbnailRefreshPolling() {
  if (state.videoThumbnailRefreshPollTimer) {
    clearTimeout(state.videoThumbnailRefreshPollTimer);
    state.videoThumbnailRefreshPollTimer = null;
  }
}

function stopEXIFBackfillPolling() {
  if (state.exifBackfillPollTimer) {
    clearTimeout(state.exifBackfillPollTimer);
    state.exifBackfillPollTimer = null;
  }
}

function isThumbnailBuildActive(status = state.thumbnailBuildStatus) {
  const value = String(status && status.status || '');
  return value === 'running' || value === 'cancelling';
}

function formatThumbnailBuildDetail(status = state.thumbnailBuildStatus) {
  const done = Math.max(0, Number(status && status.done) || 0);
  const total = Math.max(0, Number(status && status.total) || 0);
  const generated = Math.max(0, Number(status && status.generated) || 0);
  const skipped = Math.max(0, Number(status && status.skipped) || 0);
  const failed = Math.max(0, Number(status && status.failed) || 0);
  const countText = total > 0 ? `${done} / ${total}` : `${done}`;
  const parts = [
    status && status.message ? status.message : copyText('app.interface.s_4ae6039be5'),
    copyText('app.interface.d_6ebe9b85e3', { v0: (countText) }),
    copyText('app.interface.d_924bc386d3', { v0: (generated) }),
  ];
  if (skipped > 0) parts.push(copyText('app.interface.d_5f65af533b', { v0: (skipped) }));
  if (failed > 0) parts.push(copyText('app.interface.d_43be80f0a3', { v0: (failed) }));
  if (Number(status && status.eta_seconds) > 0) parts.push(`ETA ${formatDuration((Number(status.eta_seconds) || 0) * 1000)}`);
  return parts.join(' · ');
}

function syncThumbnailBuildOverlay(status = state.thumbnailBuildStatus) {
  if (!isThumbnailBuildActive(status) || state.thumbnailBuildOverlaySuppressed) {
    hideBlockingProgress();
    return;
  }
  showBlockingProgress(copyText('app.interface.s_1e2142b559'), formatThumbnailBuildDetail(status), {
    cancelText: state.thumbnailBuildCancelPending ? copyText('app.interface.s_6bc0d687ae') : copyText('app.interface.commonCancel'),
    onCancel: () => {
      if (state.thumbnailBuildCancelPending) return;
      state.thumbnailBuildCancelPending = true;
      updateBlockingProgress(copyText('app.interface.s_72d2beaf45'));
      void cancelThumbnailBuild().catch(e => {
        state.thumbnailBuildCancelPending = false;
        updateBlockingProgress(copyText('app.interface.s_f07aa79f9c'));
        console.error("取消缩略图任务失败", e);
      });
    },
  });
}

function isVideoThumbnailRefreshActive(status = state.videoThumbnailRefreshStatus) {
  const value = String(status && status.status || '');
  return value === 'running' || value === 'cancelling';
}

function formatVideoThumbnailRefreshDetail(status = state.videoThumbnailRefreshStatus) {
  const done = Math.max(0, Number(status && status.done) || 0);
  const total = Math.max(0, Number(status && status.total) || 0);
  const refreshed = Math.max(0, Number(status && status.refreshed) || 0);
  const failed = Math.max(0, Number(status && status.failed) || 0);
  const countText = total > 0 ? `${done} / ${total}` : `${done}`;
  const parts = [
    status && status.message ? status.message : copyText('app.interface.s_4eb31cb8f1'),
    copyText('app.interface.d_6ebe9b85e3', { v0: (countText) }),
    copyText('app.interface.d_c444c482b7', { v0: (refreshed) }),
  ];
  if (failed > 0) parts.push(copyText('app.interface.d_43be80f0a3', { v0: (failed) }));
  if (Number(status && status.eta_seconds) > 0) parts.push(`ETA ${formatDuration((Number(status.eta_seconds) || 0) * 1000)}`);
  return parts.join(' · ');
}

function syncVideoThumbnailRefreshOverlay(status = state.videoThumbnailRefreshStatus) {
  if (!isVideoThumbnailRefreshActive(status)) {
    hideBlockingProgress();
    return;
  }
  showBlockingProgress(copyText('app.interface.s_7c23ed20a4'), formatVideoThumbnailRefreshDetail(status), {
    cancelText: state.videoThumbnailRefreshCancelPending ? copyText('app.interface.s_6bc0d687ae') : copyText('app.interface.commonCancel'),
    onCancel: () => {
      if (state.videoThumbnailRefreshCancelPending) return;
      state.videoThumbnailRefreshCancelPending = true;
      updateBlockingProgress(copyText('app.interface.s_72d2beaf45'));
      void cancelVideoThumbnailRefresh().catch(e => {
        state.videoThumbnailRefreshCancelPending = false;
        updateBlockingProgress(copyText('app.interface.s_f07aa79f9c'));
        console.error("取消视频缩略图任务失败", e);
      });
    },
  });
}

function isEXIFBackfillActive(status = state.exifBackfillStatus) {
  const value = String(status && status.status || '');
  return value === 'running' || value === 'cancelling';
}

function formatEXIFBackfillDetail(status = state.exifBackfillStatus) {
  const done = Math.max(0, Number(status && status.done) || Number(status && status.scanned) || 0);
  const total = Math.max(0, Number(status && status.total) || 0);
  const updated = Math.max(0, Number(status && status.updated) || 0);
  const skipped = Math.max(0, Number(status && status.skipped) || 0);
  const failed = Math.max(0, Number(status && status.failed) || 0);
  const countText = total > 0 ? `${done} / ${total}` : `${done}`;
  const parts = [
    status && status.message ? status.message : copyText('app.interface.s_f1939192b1'),
    copyText('app.interface.d_6ebe9b85e3', { v0: (countText) }),
    copyText('app.interface.d_ab6a98cbbd', { v0: (updated) }),
    copyText('app.interface.d_6ffb776acc', { v0: (skipped) }),
  ];
  if (failed > 0) parts.push(copyText('app.interface.d_43be80f0a3', { v0: (failed) }));
  if (Number(status && status.eta_seconds) > 0) parts.push(`ETA ${formatDuration((Number(status.eta_seconds) || 0) * 1000)}`);
  return parts.join(' · ');
}

function syncEXIFBackfillOverlay(status = state.exifBackfillStatus) {
  if (!isEXIFBackfillActive(status)) {
    hideBlockingProgress();
    return;
  }
  showBlockingProgress(copyText('app.interface.s_3b79d3ec78'), formatEXIFBackfillDetail(status), {
    percent: Number(status && status.percent) || 0,
    cancelText: state.exifBackfillCancelPending ? copyText('app.interface.s_6bc0d687ae') : copyText('app.interface.commonCancel'),
    onCancel: () => {
      if (state.exifBackfillCancelPending) return;
      state.exifBackfillCancelPending = true;
      updateBlockingProgress(copyText('app.interface.s_72d2beaf45'));
      void cancelEXIFBackfill().catch(e => {
        state.exifBackfillCancelPending = false;
        updateBlockingProgress(copyText('app.interface.s_f07aa79f9c'));
        console.error("取消 EXIF 修正任务失败", e);
      });
    },
  });
}

function isLibraryBatchBuildActive(status = state.libraryBatchBuildStatus) {
  const value = String(status && status.status || '');
  return value === 'running' || value === 'cancelling';
}

function syncLibraryBatchBuildOverlay(status = state.libraryBatchBuildStatus) {
  return status;
}

function isLibraryBatchThumbnailBuildActive(status = state.libraryBatchThumbnailBuildStatus) {
  const value = String(status && status.status || '');
  return value === 'running' || value === 'cancelling';
}

function isPlaybackCacheBuildActive(status = state.playbackCacheBuildStatus) {
  const value = String(status && status.status || '');
  return value === 'running' || value === 'cancelling';
}

function playbackCacheBuildProgressPercent(status = state.playbackCacheBuildStatus) {
  const done = Math.max(0, Number(status && status.done) || 0);
  const total = Math.max(0, Number(status && status.total) || 0);
  return total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
}

function stopPlaybackCacheBuildPolling() {
  if (state.playbackCacheBuildPollTimer) {
    clearTimeout(state.playbackCacheBuildPollTimer);
    state.playbackCacheBuildPollTimer = null;
  }
}

async function pollPlaybackCacheBuildStatus() {
  stopPlaybackCacheBuildPolling();
  try {
    const status = await fetchPlaybackCacheBuildStatus();
    const previousStatus = String(state.playbackCacheBuildStatus && state.playbackCacheBuildStatus.status || '');
    state.playbackCacheBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_737edd10ff') };
    state.playbackCacheBuildCancelPending = isPlaybackCacheBuildActive(state.playbackCacheBuildStatus) && state.playbackCacheBuildCancelPending;
    syncPlaybackCacheModal();
    const currentStatus = String(state.playbackCacheBuildStatus.status || '');
    if (isPlaybackCacheBuildActive(state.playbackCacheBuildStatus)) {
      state.playbackCacheBuildPollTimer = setTimeout(pollPlaybackCacheBuildStatus, 900);
    } else if (previousStatus === 'running' || previousStatus === 'cancelling') {
      await refreshPlaybackCacheItems();
      syncPlaybackCacheModal();
    } else if (currentStatus === 'completed' || currentStatus === 'failed' || currentStatus === 'cancelled') {
      await refreshPlaybackCacheItems();
      syncPlaybackCacheModal();
    }
  } catch (e) {
    console.error("播放兼容缓存状态获取失败", e);
    state.playbackCacheBuildPollTimer = setTimeout(pollPlaybackCacheBuildStatus, 1800);
  }
}

async function refreshPlaybackCacheItems() {
  const data = await fetchPlaybackCaches();
  const items = Array.isArray(data && data.items) ? data.items : [];
  state.playbackCacheItems = items;
  state.playbackCacheItemsLoaded = true;
  const available = new Set(items.map(item => String(item.uuid || '')).filter(Boolean));
  state.playbackCacheSelected = new Set([...state.playbackCacheSelected].filter(uuid => available.has(uuid)));
  return items;
}

function playbackCacheSelectionAllChecked() {
  const items = Array.isArray(state.playbackCacheItems) ? state.playbackCacheItems : [];
  return items.length > 0 && items.every(item => state.playbackCacheSelected.has(String(item.uuid || '')));
}

function renderPlaybackCacheRows() {
  const items = Array.isArray(state.playbackCacheItems) ? state.playbackCacheItems : [];
  if (!items.length) {
    return `<div class="playback-cache-empty">${escapeHTML(copyText('app.interface.runtime.playbackCacheEmpty'))}</div>`;
  }
  return items.map(item => {
    const uuid = String(item.uuid || '');
    const checked = state.playbackCacheSelected.has(uuid);
    const name = item.original_name || uuid || copyText('app.interface.s_c00806350f');
    const detail = [
      item.mime_type || copyText('app.interface.s_e9649f84f9'),
      formatSize(Number(item.size) || 0),
      item.created_at ? copyText('app.interface.d_39e49c7757', { v0: (formatDateTime(item.created_at)) }) : '',
    ].filter(Boolean).join(' · ');
    return `<label class="playback-cache-row ${checked ? 'is-selected' : ''}">
      <span class="settings-toggle-switch playback-cache-check">
        <input type="checkbox" name="playback-cache-item" value="${escapeHTML(uuid)}" ${checked ? 'checked' : ''}>
        <span class="settings-toggle-slider" aria-hidden="true"></span>
      </span>
      <span class="playback-cache-main">
        <strong>${escapeHTML(name)}</strong>
        <span>${escapeHTML(detail)}</span>
        <small title="${escapeHTML(item.cache_path || '')}">${escapeHTML(item.cache_path || '')}</small>
      </span>
    </label>`;
  }).join('');
}

function renderPlaybackCacheModalContent() {
  const status = state.playbackCacheBuildStatus || {};
  const active = isPlaybackCacheBuildActive(status);
  const percent = playbackCacheBuildProgressPercent(status);
  const selectedCount = state.playbackCacheSelected.size;
  const items = Array.isArray(state.playbackCacheItems) ? state.playbackCacheItems : [];
  const statusParts = [
    status.message || copyText('app.interface.s_737edd10ff'),
    Number(status.total) > 0 ? `${Number(status.done) || 0} / ${Number(status.total) || 0}` : '',
    Number(status.generated) > 0 ? copyText('app.interface.d_9d4452bbbb', { v0: (Number(status.generated)) }) : '',
    Number(status.skipped) > 0 ? copyText('app.interface.d_6ffb776acc', { v0: (Number(status.skipped)) }) : '',
    Number(status.failed) > 0 ? copyText('app.interface.d_43be80f0a3', { v0: (Number(status.failed)) }) : '',
  ].filter(Boolean);
  return `<div class="playback-cache-toolbar">
    <label class="playback-cache-select-all">
      <span class="settings-toggle-switch playback-cache-check">
        <input id="playback-cache-select-all" type="checkbox" ${playbackCacheSelectionAllChecked() ? 'checked' : ''} ${items.length ? '' : 'disabled'}>
        <span class="settings-toggle-slider" aria-hidden="true"></span>
      </span>
      <span>${escapeHTML(copyText('app.interface.runtime.selectAllCache'))}</span>
    </label>
    <span>${escapeHTML(items.length ? copyText('app.interface.d_6422788be3', { v0: (items.length), v1: (selectedCount) }) : copyText('app.interface.s_9bb6a734f8'))}</span>
  </div>
  <div class="playback-cache-progress">
    <span style="width:${percent}%"></span>
  </div>
  <div class="playback-cache-status">${statusParts.map(part => `<span>${escapeHTML(part)}</span>`).join('')}</div>
  ${status.error ? `<div class="playback-cache-error">${escapeHTML(status.error)}</div>` : ''}
  <div class="playback-cache-list">${renderPlaybackCacheRows()}</div>
  <div class="modal-footer playback-cache-actions">
    <button class="btn" id="playback-cache-close" type="button">${copyText('app.interface.commonClose')}</button>
    <button class="btn" id="playback-cache-delete" type="button" ${selectedCount ? '' : 'disabled'}>${escapeHTML(copyText('app.interface.runtime.deleteSelected'))}</button>
    <button class="btn" id="playback-cache-cancel-build" type="button" ${active ? '' : 'disabled'}>${state.playbackCacheBuildCancelPending ? copyText('app.interface.s_6bc0d687ae') : copyText('app.interface.s_7cf36613f2')}</button>
    <button class="btn btn-primary" id="playback-cache-start-build" type="button" ${active ? 'disabled' : ''}>${escapeHTML(copyText('app.interface.runtime.startConversion'))}</button>
  </div>`;
}

function bindPlaybackCacheModalHandlers() {
  const modal = $('#playback-cache-modal');
  if (!modal) return;
  $('#playback-cache-close')?.addEventListener('click', closePlaybackCacheModal);
  $('#playback-cache-select-all')?.addEventListener('change', e => {
    const checked = !!e.target.checked;
    state.playbackCacheSelected = new Set(checked ? state.playbackCacheItems.map(item => String(item.uuid || '')).filter(Boolean) : []);
    syncPlaybackCacheModal();
  });
  $$('input[name="playback-cache-item"]', modal).forEach(input => {
    input.addEventListener('change', e => {
      const uuid = String(e.target.value || '');
      if (!uuid) return;
      if (e.target.checked) state.playbackCacheSelected.add(uuid);
      else state.playbackCacheSelected.delete(uuid);
      syncPlaybackCacheModal();
    });
  });
  $('#playback-cache-delete')?.addEventListener('click', async () => {
    const uuids = [...state.playbackCacheSelected];
    if (!uuids.length) return;
    try {
      await deletePlaybackCaches(uuids);
      state.playbackCacheSelected.clear();
      await refreshPlaybackCacheItems();
      syncPlaybackCacheModal();
      showToast(copyText('app.interface.s_7e6eb07bc5'));
    } catch (e) {
      alert(copyText('app.interface.s_ed1c2231b7') + ((e && e.error) || e));
    }
  });
  $('#playback-cache-start-build')?.addEventListener('click', async () => {
    try {
      state.playbackCacheBuildStatus = await startPlaybackCacheBuild();
      state.playbackCacheBuildCancelPending = false;
      syncPlaybackCacheModal();
      void pollPlaybackCacheBuildStatus();
    } catch (e) {
      alert(copyText('app.interface.s_d51e6d7f65') + ((e && e.error) || e));
    }
  });
  $('#playback-cache-cancel-build')?.addEventListener('click', async () => {
    if (state.playbackCacheBuildCancelPending || !isPlaybackCacheBuildActive()) return;
    state.playbackCacheBuildCancelPending = true;
    syncPlaybackCacheModal();
    try {
      state.playbackCacheBuildStatus = await cancelPlaybackCacheBuild();
      void pollPlaybackCacheBuildStatus();
    } catch (e) {
      state.playbackCacheBuildCancelPending = false;
      syncPlaybackCacheModal();
      alert(copyText('app.interface.s_9c26a5f6c4') + ((e && e.error) || e));
    }
  });
}

function syncPlaybackCacheModal() {
  const content = $('#playback-cache-content');
  if (!content) return;
  content.innerHTML = renderPlaybackCacheModalContent();
  bindPlaybackCacheModalHandlers();
}

async function openPlaybackCacheModal() {
  const modal = $('#playback-cache-modal');
  if (!modal) return;
  modal.classList.add('open');
  syncPlaybackCacheModal();
  try {
    await Promise.all([refreshPlaybackCacheItems(), fetchPlaybackCacheBuildStatus().then(status => {
      state.playbackCacheBuildStatus = status || state.playbackCacheBuildStatus;
    })]);
  } catch (e) {
    console.error("播放兼容缓存加载失败", e);
    showToast(copyText('app.interface.s_1c9c8c1025'));
  }
  syncPlaybackCacheModal();
  if (isPlaybackCacheBuildActive()) void pollPlaybackCacheBuildStatus();
}

function closePlaybackCacheModal() {
  $('#playback-cache-modal')?.classList.remove('open');
}

async function pollThumbnailBuildStatus() {
  stopThumbnailBuildPolling();
  try {
    const status = await fetchThumbnailBuildStatus();
    const previous = state.thumbnailBuildStatus || {};
    state.thumbnailBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_e45f077ef4') };
    if (!isThumbnailBuildActive(state.thumbnailBuildStatus)) {
      state.thumbnailBuildOverlaySuppressed = false;
    }
    if (state.thumbnailBuildStatus.status !== 'cancelling') {
      state.thumbnailBuildCancelPending = false;
    }
    syncThumbnailBuildOverlay(state.thumbnailBuildStatus);
    if (isThumbnailBuildActive(state.thumbnailBuildStatus)) {
      refreshVisiblePendingThumbnails();
      state.thumbnailBuildPollTimer = setTimeout(pollThumbnailBuildStatus, 450);
      return;
    }
    if (previous.status === 'running' || previous.status === 'cancelling') {
      refreshVisiblePendingThumbnails(true);
      if (state.thumbnailBuildStatus.status === 'completed') {
        showToast(copyText('app.interface.d_05c466385d', { v0: (state.thumbnailBuildStatus.generated || 0) }), 3200);
      } else if (state.thumbnailBuildStatus.status === 'cancelled') {
        showToast(copyText('app.interface.s_9540398f58'));
      } else if (state.thumbnailBuildStatus.status === 'failed') {
        showToast(copyText('app.interface.s_4e1c3e7fcd'));
      }
    }
  } catch (e) {
    console.error("获取缩略图任务状态失败", e);
    hideBlockingProgress();
    state.thumbnailBuildCancelPending = false;
  }
}

async function pollVideoThumbnailRefreshStatus() {
  stopVideoThumbnailRefreshPolling();
  try {
    const status = await fetchVideoThumbnailRefreshStatus();
    const previous = state.videoThumbnailRefreshStatus || {};
    state.videoThumbnailRefreshStatus = status || { status: 'idle', message: copyText('app.interface.s_e68748b3f3') };
    if (state.videoThumbnailRefreshStatus.status !== 'cancelling') {
      state.videoThumbnailRefreshCancelPending = false;
    }
    syncVideoThumbnailRefreshOverlay(state.videoThumbnailRefreshStatus);
    if (isVideoThumbnailRefreshActive(state.videoThumbnailRefreshStatus)) {
      state.videoThumbnailRefreshPollTimer = setTimeout(pollVideoThumbnailRefreshStatus, 450);
      return;
    }
    if (previous.status === 'running' || previous.status === 'cancelling') {
      if (state.videoThumbnailRefreshStatus.status === 'completed') {
        showToast(copyText('app.interface.d_cad7a7e647', { v0: (state.videoThumbnailRefreshStatus.refreshed || 0) }), 3200);
      } else if (state.videoThumbnailRefreshStatus.status === 'cancelled') {
        showToast(copyText('app.interface.s_c495d95d5d'));
      } else if (state.videoThumbnailRefreshStatus.status === 'failed') {
        showToast(copyText('app.interface.s_d903a50cb6'));
      }
    }
  } catch (e) {
    console.error("获取视频缩略图任务状态失败", e);
    hideBlockingProgress();
    state.videoThumbnailRefreshCancelPending = false;
  }
}

async function pollEXIFBackfillStatus() {
  stopEXIFBackfillPolling();
  try {
    const status = await fetchEXIFBackfillStatus();
    const previous = state.exifBackfillStatus || {};
    state.exifBackfillStatus = status || { status: 'idle', message: copyText('app.interface.s_f53844d2ff') };
    if (state.exifBackfillStatus.status !== 'cancelling') {
      state.exifBackfillCancelPending = false;
    }
    syncEXIFBackfillOverlay(state.exifBackfillStatus);
    if (isEXIFBackfillActive(state.exifBackfillStatus)) {
      state.exifBackfillPollTimer = setTimeout(pollEXIFBackfillStatus, 450);
      return;
    }
    if (previous.status === 'running' || previous.status === 'cancelling') {
      if (state.exifBackfillStatus.status === 'completed') {
        showToast(copyText('app.interface.d_3b41b22cb3', { v0: (state.exifBackfillStatus.updated || 0), v1: (state.exifBackfillStatus.skipped || 0), v2: (state.exifBackfillStatus.failed || 0) }), 4200);
        if (state.exifBackfillStatus.errors && state.exifBackfillStatus.errors.length) {
          alert(`${copyText('app.interface.s_c9d96a9a1a')}${state.exifBackfillStatus.errors.join('\n')}`);
        }
      } else if (state.exifBackfillStatus.status === 'cancelled') {
        showToast(copyText('app.interface.s_bae1dc07b9'));
      } else if (state.exifBackfillStatus.status === 'failed') {
        showToast(copyText('app.interface.s_4b40af4bb5'));
      }
      if (state.view === 'settings') renderSettingsContent();
    }
  } catch (e) {
    console.error("获取 EXIF 修正任务状态失败", e);
    hideBlockingProgress();
    state.exifBackfillCancelPending = false;
  }
}

async function pollLibraryBatchBuildStatus() {
  stopLibraryBatchBuildPolling();
  try {
    const status = await fetchLibraryBatchBuildStatus();
    const previous = state.libraryBatchBuildStatus || {};
    state.libraryBatchBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_5ae3799d16') };
    if (state.libraryBatchBuildStatus.status !== 'cancelling') {
      state.libraryBatchBuildCancelPending = false;
    }
    if (isLibraryBatchBuildActive(state.libraryBatchBuildStatus)) {
      if (state.view === 'settings') {
        syncLibraryBatchWorkflowPanel();
        syncLibraryBatchBuildPanel();
        syncLibraryBatchThumbnailBuildPanel();
      }
      state.libraryBatchBuildPollTimer = setTimeout(pollLibraryBatchBuildStatus, 600);
      return;
    }
    let workflowTransitionedToThumbnails = false;
    if (state.libraryBatchWorkflowPhase === 'scan' && state.libraryBatchWorkflowEnabled) {
      if (state.libraryBatchBuildStatus.status === 'completed') {
        setLibraryBatchWorkflowPhase('thumbnails');
        workflowTransitionedToThumbnails = true;
        try {
          const thumbnailStatus = await fetchLibraryBatchThumbnailBuildStatus();
          state.libraryBatchThumbnailBuildStatus = thumbnailStatus || { status: 'idle', message: copyText('app.interface.s_96a3670f18') };
          if (isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus)) {
            stopLibraryBatchThumbnailBuildPolling();
            state.libraryBatchThumbnailBuildPollTimer = setTimeout(pollLibraryBatchThumbnailBuildStatus, 300);
          } else {
            await openLibraryBatchThumbnailBuildWorkflow({ aggressive: state.libraryBatchWorkflowAggressive, startedByWorkflow: true });
          }
        } catch (workflowError) {
          clearLibraryBatchWorkflowPhase();
          alert(copyText('app.interface.s_c1039031e2') + ((workflowError && workflowError.error) || workflowError.message || workflowError));
        }
      } else if (state.libraryBatchBuildStatus.status === 'cancelled' || state.libraryBatchBuildStatus.status === 'failed') {
        clearLibraryBatchWorkflowPhase();
      }
    } else if (state.libraryBatchWorkflowPhase === 'scan') {
      clearLibraryBatchWorkflowPhase();
    }
    if (state.view === 'settings') {
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    }
    if (previous.status === 'running' || previous.status === 'cancelling') {
      if (state.libraryBatchBuildStatus.status === 'completed' && !workflowTransitionedToThumbnails) {
        showToast(state.libraryBatchBuildStatus.message || copyText('app.libraryBatchScan.completed'), 3600);
      } else if (state.libraryBatchBuildStatus.status === 'cancelled') {
        showToast(copyText('app.libraryBatchScan.cancelled'), 3200);
      } else if (state.libraryBatchBuildStatus.status === 'failed') {
        showToast(copyText('app.libraryBatchScan.failed'), 3200);
      }
      if (state.view === 'settings') renderSettingsContent();
    }
  } catch (e) {
    if (isForbiddenError(e)) {
      state.libraryBatchBuildStatus = { status: 'idle', message: copyText('app.interface.s_5ae3799d16') };
      state.libraryBatchBuildCancelPending = false;
      if (state.view === 'settings') syncLibraryBatchWorkflowPanel();
      return;
    }
    console.error("获取批量扫描任务状态失败", e);
    state.libraryBatchBuildCancelPending = false;
  }
}

async function pollLibraryBatchThumbnailBuildStatus() {
  stopLibraryBatchThumbnailBuildPolling();
  try {
    const status = await fetchLibraryBatchThumbnailBuildStatus();
    const previous = state.libraryBatchThumbnailBuildStatus || {};
    state.libraryBatchThumbnailBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_96a3670f18') };
    if (state.libraryBatchThumbnailBuildStatus.status !== 'cancelling') {
      state.libraryBatchThumbnailBuildCancelPending = false;
    }
    if (state.view === 'settings') {
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    }
    if (isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus)) {
      state.libraryBatchThumbnailBuildPollTimer = setTimeout(pollLibraryBatchThumbnailBuildStatus, 600);
      return;
    }
    if (state.libraryBatchWorkflowPhase === 'thumbnails') {
      clearLibraryBatchWorkflowPhase();
    }
    if (previous.status === 'running' || previous.status === 'cancelling') {
      if (state.libraryBatchThumbnailBuildStatus.status === 'completed') {
        showToast(state.libraryBatchThumbnailBuildStatus.message || copyText('app.libraryBatchThumbnail.completed'), 3600);
      } else if (state.libraryBatchThumbnailBuildStatus.status === 'cancelled') {
        showToast(copyText('app.libraryBatchThumbnail.cancelled'), 3200);
      } else if (state.libraryBatchThumbnailBuildStatus.status === 'failed') {
        showToast(copyText('app.libraryBatchThumbnail.failed'), 3200);
      }
      if (state.view === 'settings') renderSettingsContent();
    }
  } catch (e) {
    if (isForbiddenError(e)) {
      state.libraryBatchThumbnailBuildStatus = { status: 'idle', message: copyText('app.interface.s_96a3670f18') };
      state.libraryBatchThumbnailBuildCancelPending = false;
      if (state.view === 'settings') syncLibraryBatchWorkflowPanel();
      return;
    }
    console.error("获取批量缩略图任务状态失败", e);
    state.libraryBatchThumbnailBuildCancelPending = false;
  }
}

async function openVideoThumbnailRefreshWorkflow() {
  try {
    const status = await refreshVideoThumbnails();
    state.videoThumbnailRefreshStatus = status || { status: 'idle', message: copyText('app.interface.s_e68748b3f3') };
    state.videoThumbnailRefreshCancelPending = false;
    syncVideoThumbnailRefreshOverlay(state.videoThumbnailRefreshStatus);
    if (isVideoThumbnailRefreshActive(state.videoThumbnailRefreshStatus)) {
      stopVideoThumbnailRefreshPolling();
      state.videoThumbnailRefreshPollTimer = setTimeout(pollVideoThumbnailRefreshStatus, 300);
    } else if (state.videoThumbnailRefreshStatus.status === 'completed') {
      showToast(copyText('app.interface.s_b99555c691'));
    }
  } catch (e) {
    alert(copyText('app.interface.s_ee9cee3a3b') + ((e && e.error) || e));
  }
}

async function openThumbnailBuildWorkflow() {
  try {
    const status = await startThumbnailBuild();
    state.thumbnailBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_e45f077ef4') };
    state.thumbnailBuildCancelPending = false;
    state.thumbnailBuildOverlaySuppressed = false;
    syncThumbnailBuildOverlay(state.thumbnailBuildStatus);
    if (isThumbnailBuildActive(state.thumbnailBuildStatus)) {
      stopThumbnailBuildPolling();
      state.thumbnailBuildPollTimer = setTimeout(pollThumbnailBuildStatus, 300);
    } else if (state.thumbnailBuildStatus.status === 'completed') {
      showToast(copyText('app.interface.s_a8dde83764'));
    }
  } catch (e) {
    alert(copyText('app.interface.s_3ba2ab8314') + ((e && e.error) || e));
  }
}

async function openLibraryBatchBuildWorkflow(options = {}) {
  try {
    const status = await startLibraryBatchBuild(options);
    state.libraryBatchBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_5ae3799d16') };
    state.libraryBatchBuildCancelPending = false;
    if (state.view === 'settings') {
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    }
    if (isLibraryBatchBuildActive(state.libraryBatchBuildStatus)) {
      stopLibraryBatchBuildPolling();
      state.libraryBatchBuildPollTimer = setTimeout(pollLibraryBatchBuildStatus, 300);
    } else if (state.libraryBatchBuildStatus.status === 'completed') {
      showToast(state.libraryBatchBuildStatus.message || copyText('app.libraryBatchScan.completed'));
    }
  } catch (e) {
    if (options.startedByWorkflow) throw e;
    alert(copyText('app.interface.s_f06e7d0504') + ((e && e.error) || e));
  }
}

async function openLibraryBatchThumbnailBuildWorkflow(options = {}) {
  try {
    const status = await startLibraryBatchThumbnailBuild({ aggressive: !!options.aggressive });
    state.libraryBatchThumbnailBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_96a3670f18') };
    state.libraryBatchThumbnailBuildCancelPending = false;
    if (state.view === 'settings') {
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    }
    if (isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus)) {
      stopLibraryBatchThumbnailBuildPolling();
      state.libraryBatchThumbnailBuildPollTimer = setTimeout(pollLibraryBatchThumbnailBuildStatus, 300);
    } else if (state.libraryBatchThumbnailBuildStatus.status === 'completed') {
      if (!options.startedByWorkflow) {
        showToast(state.libraryBatchThumbnailBuildStatus.message || copyText('app.libraryBatchThumbnail.completed'));
      }
    }
  } catch (e) {
    if (options.startedByWorkflow) throw e;
    alert(copyText('app.interface.s_f5047d684b') + ((e && e.error) || e));
  }
}

function cancelActivePreloads() {
  for (const job of state.activePreloadJobs) {
    if (!job || !job.img) continue;
    job.img.onload = null;
    job.img.onerror = null;
    job.img.src = '';
    if (typeof job.resolve === 'function') job.resolve(false);
  }
  state.activePreloadJobs.clear();
}

function cancelVisibleThumbnailWarmups() {
  for (const timer of visibleThumbnailWarmTimers.values()) {
    clearTimeout(timer);
  }
  visibleThumbnailWarmTimers.clear();
  state.visibleThumbnailWarmSignatures = {};
  state.warmedThumbnailKeys = [];
  state.warmedThumbnailSet.clear();
}

function interruptThumbnailBuildForForegroundTask() {
  cancelActivePreloads();
  cancelVisibleThumbnailWarmups();
  if (!isThumbnailBuildActive()) return;
  state.thumbnailBuildOverlaySuppressed = true;
  hideBlockingProgress();
  if (state.thumbnailBuildCancelPending) return;
  state.thumbnailBuildCancelPending = true;
  void cancelThumbnailBuild()
    .then(status => {
      state.thumbnailBuildStatus = status || { status: 'idle', message: copyText('app.interface.s_e45f077ef4') };
      if (!isThumbnailBuildActive(state.thumbnailBuildStatus)) {
        state.thumbnailBuildOverlaySuppressed = false;
        state.thumbnailBuildCancelPending = false;
      }
    })
    .catch(e => {
      state.thumbnailBuildCancelPending = false;
      state.thumbnailBuildOverlaySuppressed = false;
      console.error("前台任务取消缩略图构建失败", e);
    });
}

function pauseAutoLoadMore(kind = '') {
  if (kind === 'timeline') {
    state.timelineAutoLoadPaused = true;
  } else if (kind === 'random-album') {
    state.randomAlbumAutoLoadPaused = true;
  }
}

function resumeAutoLoadMore(kind = '') {
  if (kind === 'timeline') {
    state.timelineAutoLoadPaused = false;
    if (state.view === 'timeline') {
      observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
      maybeLoadMoreImmediately('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
    }
  } else if (kind === 'random-album') {
    state.randomAlbumAutoLoadPaused = false;
    if (state.view === 'random-album') {
      observeLoadMore('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
      maybeLoadMoreImmediately('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
    }
  }
}

function armAutoLoadMoreResume(kind = '') {
  const resume = () => resumeAutoLoadMore(kind);
  window.addEventListener('wheel', resume, { once: true, passive: true });
  window.addEventListener('touchmove', resume, { once: true, passive: true });
  window.addEventListener('keydown', resume, { once: true });
}

function setLightboxMediaLoading(loading) {
  state.lightboxMediaLoading = !!loading;
  const spinner = $('#lb-loading');
  if (spinner) spinner.classList.toggle('show', state.lightboxMediaLoading);
}

// ── 右键菜单 ──────────────────────────────────────────
let _ctxMenu = null;
let _ctxMenuCloseTimer = null;
function armContextMenuAutoClose() {
  clearTimeout(_ctxMenuCloseTimer);
  _ctxMenuCloseTimer = null;
}
function positionContextMenu(menu, x, y, options = {}) {
  const margin = 12;
  const gap = Number.isFinite(options.gap) ? options.gap : 8;
  const anchor = options.anchor;
  const anchorRect = options.anchorRect || (anchor && anchor.getBoundingClientRect ? anchor.getBoundingClientRect() : null);
  const menuRect = menu.getBoundingClientRect();
  let left = Number.isFinite(x) ? x : margin;
  let top = Number.isFinite(y) ? y : margin;

  if (anchorRect) {
    if (options.align === 'end') {
      left = anchorRect.right - menuRect.width;
    } else if (options.align === 'center') {
      left = anchorRect.left + (anchorRect.width - menuRect.width) / 2;
    } else {
      left = anchorRect.left;
    }

    if (options.vertical === 'top') {
      top = anchorRect.top;
    } else if (options.vertical === 'above') {
      top = anchorRect.top - menuRect.height - gap;
    } else if (options.vertical === 'auto') {
      const below = anchorRect.bottom + gap;
      const above = anchorRect.top - menuRect.height - gap;
      top = below + menuRect.height <= window.innerHeight - margin
        ? below
        : above;
    } else {
      top = anchorRect.bottom + gap;
    }
  }

  left = Math.min(Math.max(margin, left), Math.max(margin, window.innerWidth - menuRect.width - margin));
  top = Math.min(Math.max(margin, top), Math.max(margin, window.innerHeight - menuRect.height - margin));
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
  menu.style.visibility = '';
}
function showContextMenu(x, y, items, options = {}) {
  closeContextMenu();
  const menu = el('div', 'context-menu');
  menu.style.left = `${Number.isFinite(x) ? x : 12}px`;
  menu.style.top = `${Number.isFinite(y) ? y : 12}px`;
  menu.style.visibility = 'hidden';
  items.forEach(item => {
    if (item === '-') {
      const sep = el('div', 'context-menu-separator');
      menu.appendChild(sep); return;
    }
    const btn = el('button', `context-menu-item${item.danger ? ' danger' : ''}${item.disabled ? ' disabled' : ''}`);
    btn.disabled = !!item.disabled;
    btn.innerHTML = `<span class="context-menu-icon">${item.icon || ''}</span><span>${escapeHTML(item.label)}</span>`;
    btn.addEventListener('click', () => {
      if (item.disabled) return;
      closeContextMenu();
      item.action();
    });
    menu.appendChild(btn);
  });
  document.body.appendChild(menu);
  _ctxMenu = menu;
  positionContextMenu(menu, x, y, options);
}
function closeContextMenu() {
  clearTimeout(_ctxMenuCloseTimer);
  _ctxMenuCloseTimer = null;
  if (_ctxMenu) { _ctxMenu.remove(); _ctxMenu = null; }
}

// ── 渲染框架 ──────────────────────────────────────────
function renderApp() {
  const navItem = (view, icon, label, active = state.view === view) =>
    `<a class="nav-item${active ? ' active' : ''}" href="#" data-view="${view}" data-label="${escapeHTML(label)}" title="${escapeHTML(label)}" aria-label="${escapeHTML(label)}">${navIconMarkup(view, active) || icon}<span class="nav-label">${escapeHTML(label)}</span></a>`;
  const rootPrimaryView = isRootUser() ? 'root-debug' : 'timeline';
  const rootPrimaryLabel = isRootUser() ? copyText('app.interface.navigationDebug') : copyText('app.interface.s_4f8ef92599');
  document.body.innerHTML = `
<div class="drawer-overlay" id="drawer-overlay"></div>
<div id="app">
  <div class="sidebar-peek-zone" id="sidebar-peek-zone" aria-hidden="true"></div>
  <button class="nav-logo nav-logo-floating" id="nav-logo-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.navLibrarySettings'))}">${renderNavLogo()}</button>
  <nav class="nav" id="main-nav">
    ${navItem(rootPrimaryView, isRootUser() ? (icons.rootDebug || icons.timeline) : icons.timeline, rootPrimaryLabel)}
    ${isRootUser() ? '' : navItem('favorites', icons.favorite, copyText('app.interface.albumFavorites'))}
    ${isRootUser() ? '' : navItem('random-album', icons.shuffle, copyText('app.interface.s_3666819c06'))}
    ${isRootUser() ? '' : navItem('albums', icons.album, copyText('app.interface.s_c657c81e6b'), state.view === 'albums' || state.view === 'album-detail')}
    ${isRootUser() ? '' : navItem('memories', icons.memories, copyText('app.interface.s_aec50b8b6c'))}
    ${isVisitorUser() || isRootUser() ? '' : navItem('trash', icons.trash, copyText('app.interface.s_64ea8751fb'))}
    ${navItem('settings', icons.settings, copyText('app.interface.s_7debf9cb03'))}
    <div class="nav-spacer"></div>
  </nav>
  <div class="main">
    <div class="topbar">
      <button class="hamburger" id="hamburger-btn" aria-label="${escapeHTML(copyText('app.interface.runtime.menu'))}">
        <span></span><span></span><span></span>
      </button>
      <span class="topbar-title" id="topbar-title" data-note="${escapeHTML(topbarNoteText())}"></span>
      <div class="topbar-leading" id="topbar-leading"></div>
      <div class="topbar-meta" id="topbar-meta"></div>
      <div id="topbar-actions"></div>
    </div>
    <div class="content" id="content"></div>
    <div class="content" id="settings-view-host" hidden aria-hidden="true"></div>
  </div>
</div>
<button class="floating-search-btn" id="floating-search-btn" type="button" aria-label="${escapeHTML(copyText('app.interface.runtime.search'))}" title="${escapeHTML(copyText('app.interface.runtime.search'))}">${icons.floatingSearch || icons.search}</button>
${renderGridScaleButton()}
${renderLightbox()}
${renderSearchOverlay()}
${renderUploadModal()}
${renderCreateAlbumModal()}
${renderSmartAlbumModal()}
${renderEditAlbumModal()}
${renderRenameMediaModal()}
${renderShareModal()}
${renderAlbumPickerModal()}
${renderShareListModal()}
${renderPlaybackCacheModal()}
${renderTimelineLoadAllModal()}
${renderRandomAlbumLoadAllModal()}
${renderLibraryLogoGuideModal()}`;

  bindNav();
  bindGlobal();
  syncSidebarUI();
  syncRoleAwareNavigation();
  syncRangeProgress();
  $('#topbar-title').textContent = copyText('app.interface.s_ce56f617e4');
  $('#content').innerHTML = `<div class="load-more"><div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.loading'))}</div>`;
  void ensureSettingsDataLoaded().then(() => {
    if (state.view === 'settings') renderSettings();
    else renderView();
    startRoleAwareBackgroundPolling();
    void ensureSettingsViewMounted().catch(e => console.warn("预构建设置页失败", e));

    void api.post('/api/library-scan/refresh', {}).then(() => {
      resetMediaFilteredViewState();
      if (state.view !== 'settings' && !state.lightboxOpen) {
        renderView();
      }
    }).catch(e => {
      console.warn("资源库检查失败", e);
    });
  });
}

function bindNav() {
  $$('.nav-item[data-view]').forEach(a => {
    a.addEventListener('click', e => {
      e.preventDefault();
      closeDrawer();
      const nav = $('#main-nav');
      const isActive = a.classList.contains('active');
      if (isCompactNavLayout()) {
        if (isActive && nav && !nav.classList.contains('nav-picker-open')) {
          setNavPickerOpen(true);
          return;
        }
        if (isActive && nav && nav.classList.contains('nav-picker-open')) {
          closeNavPicker();
          return;
        }
      }
      closeNavPicker();
      if (a.dataset.view === 'albums' && state.lastAlbumDetailID) {
        openLastAlbumDetail();
        return;
      }
      if (a.dataset.view === 'timeline' && !isRootUser()) {
        resetTimelineToInitialPage();
      }
      switchView(a.dataset.view);
    });
  });
  const navLogoBtn = $('#nav-logo-btn');
  if (navLogoBtn) navLogoBtn.addEventListener('click', () => openLibrarySettings());

  // 汉堡按钮 / 抽屉 (b-6)
  const hamburger = $('#hamburger-btn');
  const overlay   = $('#drawer-overlay');
  if (hamburger) hamburger.addEventListener('click', toggleDrawer);
  if (overlay)   overlay.addEventListener('click', closeDrawer);
  window.addEventListener('resize', () => {
    syncSidebarUI();
    updateSelectionModeUI();
    if (!isCompactNavLayout()) closeNavPicker();
    scheduleLightboxViewportChangeCheck(false);
  });
  document.addEventListener('click', e => {
    const nav = $('#main-nav');
    if (!nav || !nav.classList.contains('nav-picker-open')) return;
    if (e.target.closest('#main-nav')) return;
    if (e.target.closest('#nav-logo-btn')) return;
    closeNavPicker();
  });
}

function toggleDrawer() {
  if (!isMobileLayout()) {
    return;
  }
  const nav     = $('#main-nav');
  const overlay = $('#drawer-overlay');
  const open    = nav && nav.classList.toggle('open');
  if (overlay) overlay.classList.toggle('open', open);
}
function closeDrawer() {
  if (!isMobileLayout()) {
    return;
  }
  const nav     = $('#main-nav');
  const overlay = $('#drawer-overlay');
  if (nav)     nav.classList.remove('open');
  if (overlay) overlay.classList.remove('open');
}

function settingsViewHost() {
  return $('#settings-view-host') || $('#content');
}

function setSettingsViewVisible(visible) {
  const main = $('#content');
  const host = $('#settings-view-host');
  if (!main || !host) return;
  host.hidden = !visible;
  host.setAttribute('aria-hidden', visible ? 'false' : 'true');
  main.hidden = !!visible;
  main.setAttribute('aria-hidden', visible ? 'true' : 'false');
}

async function ensureSettingsViewMounted(force = false) {
  if (!state.settingsReady) await ensureSettingsDataLoaded();
  const mode = isRootUser() ? 'root' : 'user';
  if (!force && state.settingsMounted && state.settingsRenderMode === mode) return false;
  if (mode === 'root') {
    await renderRootConsoleContent({ host: settingsViewHost(), force: true });
  } else {
    renderSettingsContent({ host: settingsViewHost(), activate: state.view === 'settings' });
  }
  state.settingsMounted = true;
  state.settingsRenderMode = mode;
  return true;
}

function openLibrarySettings() {
  state.settingsFocus = 'libraries';
  if (state.view === 'settings') {
    const target = $('#settings-library-list');
    if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    return;
  }
  closeLightbox();
  closeDrawer();
  switchView('settings');
}

function openLastAlbumDetail() {
  const albumID = Number(state.lastAlbumDetailID) || 0;
  if (!albumID) {
    switchView('albums');
    return;
  }
  if (state.currentAlbum && Number(state.currentAlbum.id) === albumID) {
    state.currentAlbumID = albumID;
    state.view = 'album-detail';
    setHashView('album-detail', albumID);
    syncNavActiveView('album-detail');
    applyLibraryBranding();
    renderView();
    return;
  }
  state.currentAlbum = null;
  state.currentAlbumID = albumID;
  state.view = 'album-detail';
  setHashView('album-detail', albumID);
  syncNavActiveView('album-detail');
  applyLibraryBranding();
  renderView();
}

function switchView(view) {
  if (!isAllowedRootView(view) && view !== 'album-detail') {
    view = isRootUser() ? 'root-debug' : 'timeline';
  }
  interruptThumbnailBuildForForegroundTask();
  if (state.view === 'settings' && view !== 'settings' && canManageLibraries() && !isRootUser()) {
    syncLibraryDraftsToRuntime();
  }
  saveViewScroll();
  disconnectLoadMoreObserver();
  closeNavPicker();
  if (view === 'settings' && state.view !== 'settings') {
    state.settingsScrollRestorePending = true;
  }
  if (view !== 'settings') stopSettingsAvailabilityPolling();
  setSettingsViewVisible(view === 'settings');
  state.view = view;
  state.selected.clear();
  state.selectionAnchorID = null;
  updateSelectionModeUI();
  if (view !== 'album-detail') state.pendingAlbumPhotoID = null;
  if (view !== 'album-detail') {
    if (view !== 'albums') state.currentAlbumID = null;
    setHashView(view); // c-2: 同步到 hash
  }
  syncNavActiveView(view);
  applyLibraryBranding();
  renderView();
}
function isLibraryBuildBlocking(status = state.libraryBuildStatus) {
  return ['discovering', 'running', 'cancelling', 'error'].includes(status && status.status);
}
function formatSecondsShort(seconds) {
  const value = Math.max(0, Number(seconds) || 0);
  const minutes = Math.floor(value / 60);
  const rest = Math.floor(value % 60);
  if (minutes >= 60) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    return copyText('app.interface.d_c5f671b80a', { v0: (hours), v1: (mins) });
  }
  if (minutes > 0) return copyText('app.interface.d_226018136b', { v0: (minutes), v1: (rest) });
  return copyText('app.interface.commonSeconds', { v0: (rest) });
}
async function refreshLibraryBuildStatus() {
  try {
    const previousBlocking = isLibraryBuildBlocking();
    const status = await fetchLibraryBuildStatus();
    state.libraryBuildStatus = status || { status: 'idle' };
    const nextBlocking = isLibraryBuildBlocking();
    if (nextBlocking || previousBlocking) renderView();
    if (['discovering', 'running', 'cancelling'].includes(state.libraryBuildStatus.status)) {
      clearTimeout(state.libraryBuildPollTimer);
      state.libraryBuildPollTimer = setTimeout(refreshLibraryBuildStatus, 1000);
    }
  } catch (e) {
    if (isForbiddenError(e)) {
      state.libraryBuildStatus = { status: 'idle', message: copyText('app.interface.s_2552c2ae42') };
      renderView();
      return;
    }
    console.error(e);
  }
}
function startLibraryBuildPolling() {
  clearTimeout(state.libraryBuildPollTimer);
  refreshLibraryBuildStatus();
}
function resetVisitorServerTaskStatuses() {
  state.libraryBuildStatus = { status: 'idle', message: copyText('app.interface.s_2552c2ae42') };
  state.libraryBatchBuildStatus = { status: 'idle', message: copyText('app.interface.s_5ae3799d16') };
  state.libraryBatchThumbnailBuildStatus = { status: 'idle', message: copyText('app.interface.s_96a3670f18') };
  stopLibraryBatchBuildPolling();
  stopLibraryBatchThumbnailBuildPolling();
}
function startRoleAwareBackgroundPolling() {
  if (!canRunServerTasks() && !canAccessBatchWorkflow()) {
    resetVisitorServerTaskStatuses();
    if (state.view === 'settings') {
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    }
    return;
  }
  if (canRunServerTasks()) startLibraryBuildPolling();
  if (canAccessBatchWorkflow()) {
    void pollLibraryBatchBuildStatus();
    void pollLibraryBatchThumbnailBuildStatus();
  }
}
function estimateLibraryBuildDiscoveryPercent(found) {
  const count = Math.max(0, Number(found) || 0);
  if (count <= 0) return 8;
  return Math.min(42, 8 + Math.log2(count + 1) * 4);
}
function renderLibraryBuild() {
  const status = state.libraryBuildStatus || {};
  const running = ['discovering', 'running', 'cancelling'].includes(status.status);
  const cancellable = ['discovering', 'running'].includes(status.status);
  const total = Number(status.total) || 0;
  const done = Number(status.done) || 0;
  const discovering = status.status === 'discovering';
  const percent = total > 0
    ? Math.min(100, Math.max(0, Number(status.percent) || done / total * 100))
    : (discovering ? estimateLibraryBuildDiscoveryPercent(done) : (running ? 8 : 100));
  const progressText = total > 0
    ? `${done} / ${total}`
    : (done > 0
      ? copyText('app.libraryBuild.discoveredCount', { count: done })
      : copyText('app.libraryBuild.discovering'));
  $('#topbar-title').textContent = copyText('app.libraryBuild.title');
  $('#topbar-meta').innerHTML = '';
  $('#topbar-actions').innerHTML = '';
  $('#content').innerHTML = `
<div class="library-build-page">
  <section class="library-build-panel">
    <div class="library-build-heading">
      <div>
        <h2>${escapeHTML(status.message || copyText('app.libraryBuild.preparing'))}</h2>
        <p>${escapeHTML(running ? copyText('app.libraryBuild.runningDescription') : copyText('app.libraryBuild.pausedDescription'))}</p>
      </div>
      <span class="library-build-badge">${escapeHTML(running ? copyText('app.libraryBuild.runningBadge') : copyText('app.libraryBuild.pausedBadge'))}</span>
    </div>
    <div class="library-build-progress">
      <div class="library-build-progress-fill" style="width:${percent}%"></div>
    </div>
    <div class="library-build-stats">
      <div><strong>${escapeHTML(progressText)}</strong><span>${escapeHTML(copyText('app.libraryBuild.progressLabel'))}</span></div>
      <div><strong>${Math.round(percent)}%</strong><span>${escapeHTML(copyText('app.libraryBuild.percentLabel'))}</span></div>
      <div><strong>${formatSecondsShort(status.elapsed_seconds)}</strong><span>${escapeHTML(copyText('app.libraryBuild.elapsedLabel'))}</span></div>
      <div><strong>${status.eta_seconds > 0 ? formatSecondsShort(status.eta_seconds) : escapeHTML(copyText('app.libraryBuild.etaPending'))}</strong><span>${escapeHTML(copyText('app.libraryBuild.etaLabel'))}</span></div>
      ${Number(status.pruned) > 0 ? `<div><strong>${Number(status.pruned)}</strong><span>${escapeHTML(copyText('app.libraryBuild.prunedLabel'))}</span></div>` : ''}
    </div>
    ${status.error ? `<div class="library-build-error">${escapeHTML(status.error)}</div>` : ''}
    <div class="library-build-exit">
      ${renderSettingsToggle('library-build-exit-after', copyText('app.libraryBuild.exitAfterComplete'), !!status.exit_after_complete)}
    </div>
    <div class="library-build-actions">
      <button class="btn btn-danger library-build-cancel" id="library-build-cancel" type="button" ${cancellable ? '' : 'disabled'}>${escapeHTML(status.status === 'cancelling' ? copyText('app.libraryBuild.cancelling') : copyText('app.libraryBuild.cancel'))}</button>
    </div>
  </section>
</div>`;
}
function renderView() {
  const leading = $('#topbar-leading');
  if (leading) leading.innerHTML = '';
  if (isLibraryBuildBlocking()) {
    setSettingsViewVisible(false);
    renderLibraryBuild();
    return;
  }
  setSettingsViewVisible(state.view === 'settings');
  switch (state.view) {
    case 'root-debug':   renderRootDebugTimeline(); break;
    case 'timeline':     isRootUser() ? renderRootDebugTimeline() : renderTimeline(); break;
    case 'favorites':    renderFavorites();   break;
    case 'random-album': renderRandomAlbum(); break;
    case 'albums':       renderAlbums();      break;
    case 'album-detail': renderAlbumDetail(); break;
    case 'memories':     renderMemories();    break;
    case 'trash':        renderTrash();       break;
    case 'settings':     renderSettings();    break;
  }
}

function randomAlbumScore(photo, seed = state.randomAlbumSeed) {
  return hashString(`${seed}:${photo.uuid || photo.id || photo.original_name || ''}`);
}

function shuffleRandomAlbumBatch(items, seed = state.randomAlbumSeed) {
  return [...items].sort((a, b) => randomAlbumScore(a, seed) - randomAlbumScore(b, seed));
}

function randomAlbumPageLimit() {
  return window.innerWidth <= 900 ? 28 : 40;
}

function randomAlbumRenderChunkSize() {
  return window.innerWidth <= 900 ? 8 : 12;
}

function randomAlbumWarmCandidates(photos) {
  return (photos || []).slice(0, 24);
}

async function appendRandomAlbumGrid(photos, { progressive = false } = {}) {
  const wrap = $('#random-album-wrap');
  if (!wrap) return;
  let grid = $('#random-album-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'random-album-grid';
    const loadMore = $('#load-more');
    if (loadMore) wrap.insertBefore(grid, loadMore);
    else wrap.appendChild(grid);
  }
  if (!progressive || photos.length <= randomAlbumRenderChunkSize()) {
    const fragment = document.createDocumentFragment();
    photos.forEach(photo => fragment.appendChild(makePhotoThumb(photo, state.randomAlbumPhotos)));
    grid.appendChild(fragment);
    return;
  }
  const chunkSize = randomAlbumRenderChunkSize();
  for (let index = 0; index < photos.length; index += chunkSize) {
    if (!document.body.contains(grid)) return;
    const fragment = document.createDocumentFragment();
    photos.slice(index, index + chunkSize).forEach(photo => {
      fragment.appendChild(makePhotoThumb(photo, state.randomAlbumPhotos));
    });
    grid.appendChild(fragment);
    if (index + chunkSize < photos.length) {
      await new Promise(resolve => requestAnimationFrame(resolve));
    }
  }
}

async function fetchAllRandomAlbumPhotos() {
  const allPhotos = [];
  let cursor = '';
  let pageCount = 0;
  let cancelled = false;
  let controller = null;
  showBlockingProgress(
    copyText('app.randomAlbum.blockingTitle'),
    copyText('app.randomAlbum.blockingBatch', { batch: 1 }),
    {
    cancelText: copyText('app.randomAlbum.cancelOrganizing'),
    onCancel: () => {
      cancelled = true;
      if (controller) controller.abort();
      updateBlockingProgress(copyText('app.randomAlbum.cancelling', { count: allPhotos.length }));
    },
  });
  for (;;) {
    if (cancelled) throw new DOMException('Aborted', 'AbortError');
    const params = new URLSearchParams({ limit: '240' });
    params.set('seed', String(state.randomAlbumSeed));
    appendMediaKindParam(params);
    if (cursor) params.set('cursor', cursor);
    controller = new AbortController();
    const response = await fetchWithPageSession(`/api/media/random?${params.toString()}`, { signal: controller.signal });
    if (!response.ok) throw await buildAPIError(response);
    const page = await response.json();
    const nextPhotos = page.photos || [];
    allPhotos.push(...nextPhotos);
    pageCount += 1;
    updateBlockingProgress(copyText('app.randomAlbum.organizing', { count: allPhotos.length }));
    const loadMore = $('#load-more');
    if (loadMore) {
      loadMore.innerHTML = `<div class="spinner"></div>${escapeHTML(copyText('app.randomAlbum.collecting', { count: allPhotos.length }))}`;
    }
    if (!page.has_more || !page.next_cursor) break;
    cursor = page.next_cursor;
    if (pageCount % 8 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
  }
  updateBlockingProgress(copyText('app.randomAlbum.finalizing', { count: allPhotos.length }));
  return allPhotos;
}

async function fetchRandomAlbumPage(cursor, limit, signal) {
  const params = new URLSearchParams({ limit: String(limit), seed: String(state.randomAlbumSeed) });
  appendMediaKindParam(params);
  if (cursor) params.set('cursor', cursor);
  const response = await fetchWithPageSession(`/api/media/random?${params.toString()}`, { signal });
  if (!response.ok) throw await buildAPIError(response);
  return response.json();
}

function renderRandomAlbumLoadAllModal() {
  return `<div class="modal-overlay timeline-load-all-modal" id="random-album-load-all-modal">
  <div class="modal" style="width:500px">
    <div class="modal-title">${escapeHTML(copyText('app.randomAlbum.modalTitle'))}</div>
    <p class="modal-copy">${escapeHTML(copyText('app.randomAlbum.modalCopy'))}</p>
    <div class="timeline-load-progress">
      <div class="timeline-load-progress-bar" id="random-album-load-all-bar"></div>
    </div>
    <div class="timeline-load-status" id="random-album-load-all-status">${escapeHTML(copyText('app.randomAlbum.modalReady'))}</div>
    <div class="modal-footer">
      <button class="btn" id="random-album-load-all-cancel">${escapeHTML(copyText('app.randomAlbum.modalCancel'))}</button>
      <button class="btn btn-primary" id="random-album-load-all-start">${escapeHTML(copyText('app.randomAlbum.modalStart'))}</button>
    </div>
  </div>
</div>`;
}

function openRandomAlbumLoadAllModal() {
  const modal = $('#random-album-load-all-modal');
  if (!modal || state.randomAlbumBulkLoading) return;
  if (state.randomAlbumLoading && !state.randomAlbumLoaded) {
    showToast(copyText('app.randomAlbum.collectingToast'));
    return;
  }
  modal.classList.add('open');
  const total = state.randomAlbumPhotos.length || state.timelineTotal || 0;
  updateRandomAlbumLoadAllProgress(state.randomAlbumLoaded ? state.randomAlbumPhotos.length : 0, total, copyText('app.randomAlbum.modalPrepare'));
  $('#random-album-load-all-start').disabled = false;
  $('#random-album-load-all-start').textContent = state.randomAlbumLoaded ? copyText('app.interface.s_c2ca5668a2') : copyText('app.interface.s_26f8b9e0eb');
  $('#random-album-load-all-cancel').disabled = false;
  $('#random-album-load-all-cancel').textContent = copyText('app.interface.commonCancel');
  $('#random-album-load-all-cancel').onclick = closeOrCancelRandomAlbumLoadAll;
  $('#random-album-load-all-start').onclick = loadAllRandomAlbumPhotos;
}

function closeOrCancelRandomAlbumLoadAll() {
  if (state.randomAlbumBulkLoading) {
    state.randomAlbumBulkCancelRequested = true;
    if (state.randomAlbumBulkAbortController) state.randomAlbumBulkAbortController.abort();
    if (state.randomAlbumAbortController) state.randomAlbumAbortController.abort();
    cancelActivePreloads();
    disconnectLoadMoreObserver();
    pauseAutoLoadMore('random-album');
    armAutoLoadMoreResume('random-album');
    updateRandomAlbumLoadAllProgress(state.randomAlbumPhotos.length, state.randomAlbumPhotos.length, copyText('app.interface.s_6bc0d687ae'));
    $('#random-album-load-all-cancel').disabled = true;
    return;
  }
  const modal = $('#random-album-load-all-modal');
  if (modal) modal.classList.remove('open');
}

function updateRandomAlbumLoadAllProgress(loaded, total, message = '') {
  const bar = $('#random-album-load-all-bar');
  const status = $('#random-album-load-all-status');
  const safeLoaded = Math.max(0, Number(loaded) || 0);
  const safeTotal = Math.max(0, Number(total) || 0);
  const percent = safeTotal > 0 ? Math.min(100, Math.round((safeLoaded / safeTotal) * 100)) : 0;
  if (bar) bar.style.width = `${percent}%`;
  if (status) {
    const countText = safeTotal > 0 ? `${safeLoaded} / ${safeTotal}` : `${safeLoaded}`;
    status.textContent = message || copyText('app.interface.d_14d838fa52', { v0: (countText) });
  }
}

function preloadThumbnail(photo, shouldCancel) {
  return new Promise(resolve => {
    if (shouldCancel && shouldCancel()) {
      resolve(false);
      return;
    }
    const img = new Image();
    const job = {
      img,
      resolve: value => resolve(value),
    };
    state.activePreloadJobs.add(job);
    const cleanup = value => {
      img.onload = null;
      img.onerror = null;
      state.activePreloadJobs.delete(job);
      resolve(value);
    };
    img.decoding = 'async';
    img.onload = () => cleanup(true);
    img.onerror = () => cleanup(false);
    img.src = mediaThumbURL(photo);
    if (shouldCancel && shouldCancel()) {
      img.src = '';
      cleanup(false);
    }
  });
}

async function preloadThumbnailsInBatches(photos, onProgress, shouldCancel) {
  const batchSize = 10;
  let loaded = 0;
  for (let i = 0; i < photos.length; i += batchSize) {
    if (shouldCancel && shouldCancel()) break;
    const batch = photos.slice(i, i + batchSize);
    await Promise.allSettled(batch.map(photo => preloadThumbnail(photo, shouldCancel)));
    if (shouldCancel && shouldCancel()) break;
    loaded += batch.length;
    if (onProgress) onProgress(Math.min(loaded, photos.length), photos.length);
    await new Promise(resolve => requestAnimationFrame(resolve));
  }
}

const visibleThumbnailWarmTimers = new Map();

function requestVisibleThumbnailWarmup(viewKey, photos) {
  if (!isWarmEnabled()) return;
  const candidates = (photos || [])
    .filter(photo => photo && photo.uuid)
    .slice(0, 48);
  const uuids = candidates.map(photo => photo.uuid);
  if (!uuids.length) return;
  const signature = uuids.join(',');
  if (state.visibleThumbnailWarmSignatures[viewKey] === signature) return;
  if (visibleThumbnailWarmTimers.has(viewKey)) {
    clearTimeout(visibleThumbnailWarmTimers.get(viewKey));
  }
  const timer = window.setTimeout(() => {
    visibleThumbnailWarmTimers.delete(viewKey);
    state.visibleThumbnailWarmSignatures[viewKey] = signature;
    void api.post('/api/media/thumbnails/warm', { uuids }).catch(() => {});
    candidates.slice(0, 24).forEach(photo => {
      const key = `thumb:${photo.uuid}`;
      if (!rememberWarmedThumbnail(key)) return;
      const img = new Image();
      img.decoding = 'async';
      img.loading = 'eager';
      img.src = mediaThumbURL(photo);
    });
  }, 120);
  visibleThumbnailWarmTimers.set(viewKey, timer);
}

async function loadAllRandomAlbumPhotos() {
  if (state.randomAlbumBulkLoading) return;
  state.randomAlbumBulkLoading = true;
  state.randomAlbumBulkCancelRequested = false;
  state.blockingInteraction = true;
  disconnectLoadMoreObserver();
  const startBtn = $('#random-album-load-all-start');
  const cancelBtn = $('#random-album-load-all-cancel');
  if (startBtn) {
    startBtn.disabled = true;
    startBtn.textContent = copyText('app.interface.commonLoading');
  }
  if (cancelBtn) {
    cancelBtn.disabled = false;
    cancelBtn.textContent = copyText('app.interface.s_43448f3e11');
  }
  try {
    let photos = state.randomAlbumLoaded ? [...state.randomAlbumPhotos] : [];
    if (!state.randomAlbumLoaded) {
      const allPhotos = [];
      let cursor = '';
      let total = 0;
      let pageCount = 0;
      state.randomAlbumLoading = true;
      updateRandomAlbumLoadAllProgress(0, 0, copyText('app.interface.s_685e4b1e3a'));
      while (!state.randomAlbumBulkCancelRequested) {
        state.randomAlbumBulkAbortController = new AbortController();
        const page = await fetchRandomAlbumPage(cursor, 300, state.randomAlbumBulkAbortController.signal);
        const nextPhotos = page.photos || [];
        if (Number.isFinite(Number(page.total))) total = Number(page.total);
        allPhotos.push(...nextPhotos);
        pageCount += 1;
        updateRandomAlbumLoadAllProgress(allPhotos.length, total, copyText('app.interface.d_804507bf64', { v0: (allPhotos.length), v1: (total ? ` / ${total}` : '') }));
        const loadMore = $('#load-more');
        if (loadMore) loadMore.innerHTML = `<div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.collectingMedia', { count: allPhotos.length, total: total ? ` / ${total}` : '' }))}`;
        if (!page.has_more || !page.next_cursor) break;
        cursor = page.next_cursor;
        if (pageCount % 4 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
      }
      if (state.randomAlbumBulkCancelRequested) throw new DOMException('Aborted', 'AbortError');
      updateRandomAlbumLoadAllProgress(allPhotos.length, total || allPhotos.length, copyText('app.interface.s_caac71f4a5'));
      photos = allPhotos;
      state.randomAlbumPhotos = photos;
      state.randomAlbumCursor = '';
      state.randomAlbumHasMore = false;
      state.randomAlbumLoaded = true;
      state.randomAlbumLoading = false;
      if (state.view === 'random-album') await renderRandomAlbumGrid();
    }

    updateRandomAlbumLoadAllProgress(0, photos.length, copyText('app.interface.s_8ba1e129d8'));
    await preloadThumbnailsInBatches(
      photos,
      (loaded, total) => updateRandomAlbumLoadAllProgress(loaded, total, copyText('app.interface.d_1394046b40', { v0: (loaded), v1: (total) })),
      () => state.randomAlbumBulkCancelRequested,
    );
    if (state.randomAlbumBulkCancelRequested) {
      updateRandomAlbumLoadAllProgress(0, photos.length, copyText('app.interface.s_6f2f78385a'));
      showToast(copyText('app.interface.s_f4de70d873'));
    } else {
      updateRandomAlbumLoadAllProgress(photos.length, photos.length, copyText('app.interface.d_e663acbda3', { v0: (photos.length) }));
      showToast(copyText('app.interface.s_3b82beb515'));
      setTimeout(() => $('#random-album-load-all-modal')?.classList.remove('open'), 700);
    }
  } catch (e) {
    if (e && e.name === 'AbortError') {
      updateRandomAlbumLoadAllProgress(0, state.randomAlbumPhotos.length, copyText('app.interface.s_a4d5fd7920'));
      showToast(copyText('app.interface.s_f4de70d873'));
    } else {
      console.error(e);
      updateRandomAlbumLoadAllProgress(0, state.randomAlbumPhotos.length, copyText('app.interface.s_37d3b73ac9'));
      showToast(copyText('app.interface.s_0b1dc9ea19'));
    }
  } finally {
    state.randomAlbumBulkLoading = false;
    state.randomAlbumLoading = false;
    state.randomAlbumBulkAbortController = null;
    state.blockingInteraction = false;
    if (startBtn) {
      startBtn.disabled = false;
      startBtn.textContent = copyText('app.interface.s_5982c44c18');
    }
    if (cancelBtn) {
      cancelBtn.disabled = false;
      cancelBtn.textContent = copyText('app.interface.commonClose');
    }
    if (state.view === 'random-album') {
      observeLoadMore('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
    }
  }
}

async function loadMoreRandomAlbum() {
  if (state.randomAlbumLoading || state.randomAlbumBulkLoading || state.randomAlbumAutoLoadPaused || !state.randomAlbumHasMore) return;
  state.randomAlbumLoading = true;
  try {
    state.randomAlbumAbortController = new AbortController();
    const page = await fetchRandomAlbumPage(state.randomAlbumCursor, randomAlbumPageLimit(), state.randomAlbumAbortController.signal);
    const photos = page.photos || [];
    state.randomAlbumTotal = Number.isFinite(Number(page.total)) ? Number(page.total) : state.randomAlbumTotal;
    state.randomAlbumPhotos.push(...photos);
    state.randomAlbumCursor = page.next_cursor || '';
    state.randomAlbumHasMore = page.has_more || false;
    state.randomAlbumLoaded = !state.randomAlbumHasMore;
    await appendRandomAlbumGrid(photos, { progressive: true });
    if (state.pendingRandomAlbumPhotoID) await focusPendingRandomAlbumPhoto({ quiet: true });
    requestVisibleThumbnailWarmup('random-album', randomAlbumWarmCandidates(photos));
  } catch (e) {
    if (!e || e.name !== 'AbortError') {
      console.error(e);
      throw e;
    }
  } finally {
    state.randomAlbumLoading = false;
    state.randomAlbumAbortController = null;
    updateRandomAlbumLoadMoreUI();
    maybeLoadMoreImmediately('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
  }
}

async function renderRandomAlbum() {
  state.randomAlbumAutoLoadPaused = false;
  const writable = canWriteMedia();
  $('#topbar-title').textContent = copyText('app.interface.s_3666819c06');
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({
      id: 'random-return-position-btn',
      icon: icons.topbarReturnRandomPosition || icons.topbarBackAlbums || icons.back,
      label: copyText('app.interface.s_9ad685a60c'),
      variant: 'accent',
      disabled: !state.randomAlbumLastViewedPhotoID,
    }),
  ]);
  $('#topbar-meta').innerHTML = renderSelectionBarMarkup({
    extraAction: writable ? `${renderAddToAlbumSelectionAction()}<button class="btn-icon" id="delete-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.deleteSelectedMedia'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.deleteSelectedMedia'))}">${icons.selectionDelete}</button>` : '',
  }) + renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'random-load-all-btn', icon: icons.topbarLoadAll, label: copyText('app.interface.s_118c2fd3f5'), variant: 'accent' }),
  ]);
  bindMediaKindFilterControl();
  bindSelectionBarHandlers({
    extraButtonID: writable ? 'delete-sel-btn' : '',
    extraAction: writable ? deleteSelected : null,
    actionBindings: writable ? [{ id: 'add-to-album-sel-btn', action: () => openAlbumPickerModal(null) }] : [],
  });
  $('#random-load-all-btn')?.addEventListener('click', openRandomAlbumLoadAllModal);
  $('#random-return-position-btn')?.addEventListener('click', async () => {
    const targetID = Number(state.randomAlbumLastViewedPhotoID || 0);
    if (!targetID) return;
    state.pendingRandomAlbumPhotoID = targetID;
    if (await focusPendingRandomAlbumPhoto({ quiet: true })) {
      showToast(copyText('app.interface.s_7a34c54c32'));
      return;
    }
    showToast(copyText('app.interface.s_5c36b49bed'));
  });

  if (state.randomAlbumViewLoaded) {
    $('#content').innerHTML = `<div id="random-album-wrap"></div>`;
    await renderRandomAlbumGrid();
    requestVisibleThumbnailWarmup('random-album', randomAlbumWarmCandidates(state.randomAlbumPhotos));
    observeLoadMore('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
    if (!(await focusPendingRandomAlbumPhoto())) restoreViewScroll('random-album');
    return;
  }

  $('#content').innerHTML = `<div id="random-album-wrap"></div>`;
  await renderRandomAlbumGrid();
  try {
    state.randomAlbumPhotos = [];
    state.randomAlbumCursor = '';
    state.randomAlbumHasMore = true;
    state.randomAlbumLoaded = false;
    state.randomAlbumTotal = 0;
    await loadMoreRandomAlbum();
    state.randomAlbumViewLoaded = true;
    observeLoadMore('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
    if (!(await focusPendingRandomAlbumPhoto())) restoreViewScroll('random-album');
  } catch (e) {
    $('#random-album-wrap').innerHTML = `<p style="color:var(--danger)">${copyText('app.interface.d_random_album_load_failed', { error: (e && e.error) || e })}</p>`;
    restoreViewScroll('random-album');
  }
}

async function renderRandomAlbumGrid() {
  const wrap = $('#random-album-wrap');
  if (!wrap) return;
  if (!state.randomAlbumPhotos.length && !state.randomAlbumHasMore && !state.randomAlbumLoading) {
    wrap.innerHTML = `<div class="empty">${icons.shuffle}<p>${escapeHTML(copyText('app.interface.runtime.randomEmpty'))}</p></div>`;
    return;
  }
  wrap.innerHTML = '';
  if (state.randomAlbumPhotos.length) {
    await appendRandomAlbumGrid(state.randomAlbumPhotos, { progressive: state.randomAlbumPhotos.length > randomAlbumRenderChunkSize() });
  }
  const loadMore = el('div', 'load-more');
  loadMore.id = 'load-more';
  wrap.appendChild(loadMore);
  updateRandomAlbumLoadMoreUI();
}

function updateRandomAlbumLoadMoreUI() {
  const loadMore = $('#load-more');
  if (!loadMore || state.view !== 'random-album') return;
  if (state.randomAlbumLoading) {
    loadMore.innerHTML = `<div class="spinner"></div>${copyText('app.interface.s_2189ef91ed', { v0: state.randomAlbumPhotos.length, v1: state.randomAlbumTotal ? ` / ${state.randomAlbumTotal}` : '' })} <button class="btn btn-sm" id="random-page-cancel" type="button">${copyText('app.interface.commonCancel')}</button>`;
    $('#random-page-cancel')?.addEventListener('click', () => {
      if (state.randomAlbumAbortController) state.randomAlbumAbortController.abort();
      pauseAutoLoadMore('random-album');
      armAutoLoadMoreResume('random-album');
    });
    return;
  }
  if (state.randomAlbumHasMore) {
    loadMore.innerHTML = copyText('app.interface.d_e9028766a8', { v0: (state.randomAlbumPhotos.length), v1: (state.randomAlbumTotal ? ` / ${state.randomAlbumTotal}` : '') });
    return;
  }
  loadMore.textContent = copyText('app.interface.d_73ac3c2cd8', { v0: (state.randomAlbumPhotos.length) });
}

function applyRootUsersPayload(data = {}) {
  state.rootConsoleUsers = Array.isArray(data.users) ? data.users : [];
  state.rootConsoleLoaded = true;
}

async function ensureRootConsoleUsersLoaded(force = false) {
  if (!force && state.rootConsoleLoaded) return state.rootConsoleUsers;
  const data = await fetchRootUsers();
  applyRootUsersPayload(data);
  return state.rootConsoleUsers;
}

function rootUserRoleLabel(role) {
  switch (String(role || '').trim()) {
    case 'root': return 'root';
    case 'admin': return copyText('app.interface.s_ef84e765e0');
    default: return copyText('app.interface.s_2f7ccc8479');
  }
}

function rootUserAccentColor(role) {
  switch (String(role || '').trim()) {
    case 'root': return '#245549';
    case 'admin': return '#2d6a5f';
    default: return '#8d6a42';
  }
}

function rootUserInitial(username = '') {
  const value = String(username || '').trim();
  if (!value) return 'U';
  return value.slice(0, 1).toUpperCase();
}

function renderRootUserRoleBadges(user = {}) {
  const badges = [
    `<span class="settings-library-state-badge root-user-role-badge root-user-role-badge-${escapeHTML(String(user.role || 'visitor').trim() || 'visitor')}">${escapeHTML(rootUserRoleLabel(user.role))}</span>`
  ];
  if (user.can_login === false) {
    badges.push(`<span class="settings-library-state-badge is-unavailable">${copyText('app.interface.s_login_unavailable')}</span>`);
  }
  if (user.protected) {
    badges.push(`<span class="settings-library-state-badge root-user-role-badge root-user-role-badge-protected">${copyText('app.interface.s_system_entry')}</span>`);
  }
  return badges.join('');
}

function rootUserLibrarySummary(user = {}) {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  if (user.role === 'root') return copyText('app.interface.s_0bcf827b0d');
  if (user.role === 'admin') {
    const defaultName = libraries.find(item => item.id === user.default_library_id)?.name || copyText('app.interface.s_9fdb61ca93');
    return copyText('app.interface.d_96701ba9b3', { v0: (libraries.length), v1: (defaultName) });
  }
  const allowed = Array.isArray(user.allowed_library_ids) ? user.allowed_library_ids : [];
  if (!allowed.length) return String(user.login_blocked_reason || copyText('app.interface.s_c1f2e31ff7'));
  const defaultName = libraries.find(item => item.id === user.default_library_id)?.name || copyText('app.interface.s_9fdb61ca93');
  return copyText('app.interface.d_a1357841b2', { v0: (allowed.length), v1: (defaultName) });
}

function renderRootConsoleUserRows() {
  if (!state.rootConsoleUsers.length) {
    return `<div class="settings-empty">${escapeHTML(copyText('app.interface.runtime.usersEmpty'))}</div>`;
  }
  return state.rootConsoleUsers.map(user => `
    <div class="settings-library-row root-user-card${user.can_login === false ? ' is-unavailable' : ''}" data-root-user="${escapeHTML(user.username)}" style="--library-accent:${rootUserAccentColor(user.role)};cursor:default">
      <div class="settings-library-main">
        <div class="settings-library-logo-block">
          <div class="settings-library-logo-preview root-user-logo-preview" aria-hidden="true">
            <span class="root-user-avatar-mark">${icons.albumCardUser || icons.photo || rootUserInitial(user.username)}</span>
          </div>
        </div>
        <div class="settings-library-fields">
          <div class="settings-library-name-display">${escapeHTML(user.username)}</div>
          <div class="settings-library-path-display">${escapeHTML(rootUserLibrarySummary(user))}</div>
          <div class="settings-library-meta-row">
            ${renderRootUserRoleBadges(user)}
          </div>
          ${user.can_login === false && user.login_blocked_reason ? `<div class="settings-library-unavailable-note">${escapeHTML(user.login_blocked_reason)}</div>` : ''}
        </div>
      </div>
      <div class="settings-library-row-actions root-user-card-actions">
        <button class="btn btn-sm" type="button" data-root-edit="${escapeHTML(user.username)}">${copyText('app.interface.s_a7f814c0a4')}</button>
        ${user.protected ? '' : `<button class="btn btn-danger btn-sm" type="button" data-root-delete="${escapeHTML(user.username)}">${copyText('app.interface.commonDelete')}</button>`}
      </div>
    </div>`).join('');
}

function renderRootDebugMediaGrid() {
  const grid = el('div', 'photo-grid');
  rootDebugMediaItems.forEach(photo => {
    const card = el('button', 'photo-thumb');
    card.type = 'button';
    card.dataset.id = photo.id;
    card.dataset.kind = photo.media_kind || 'image';
    card.style.setProperty('--thumb-placeholder', stableMediaPlaceholderColor(photo));
    card.innerHTML = `<img loading="lazy" draggable="false" src="${mediaThumbURL(photo)}" alt="${escapeHTML(photo.original_name || '')}"${isVideoMedia(photo) ? ` onerror=\"this.onerror=null;this.src='${videoPosterPlaceholder}'\"` : ''}>${isVideoMedia(photo) ? `<span class="media-badge">${escapeHTML(videoFormatLabel(photo))}</span>` : ''}`;
    const imageEl = card.querySelector('img');
    imageEl?.addEventListener('load', () => {
      requestAnimationFrame(() => requestAnimationFrame(() => card.classList.remove('thumb-loading', 'thumb-load-failed')));
    });
    if (imageEl && !isVideoMedia(photo)) {
      imageEl.addEventListener('error', () => retryThumbnailLoad(imageEl, photo));
    }
    card.addEventListener('click', () => {
      openLightbox(rootDebugMediaItems, rootDebugMediaItems.findIndex(item => item.id === photo.id), {
        returnView: 'settings',
        debugControls: true,
      });
    });
    grid.appendChild(card);
  });
  return grid;
}

const rootDebugPreviewImage = `data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 512 512'><defs><linearGradient id='g' x1='0' x2='1' y1='0' y2='1'><stop offset='0' stop-color='%23d8e4ea'/><stop offset='1' stop-color='%2390a8b8'/></linearGradient></defs><rect width='512' height='512' rx='48' fill='url(%23g)'/><circle cx='170' cy='178' r='68' fill='rgba(255,255,255,0.92)'/><path d='M88 376c44-52 88-78 132-78 40 0 75 15 106 46 20 20 39 32 58 32 18 0 34-7 48-21v69H88z' fill='rgba(255,255,255,0.9)'/></svg>`;

function renderRootDebugWorkbenchPanel({ title, copy = '', iconKey = 'settingsPanelHeadBlank1', className = 'settings-panel-update-card', body = '' } = {}) {
  return `<section class="card settings-panel ${className}">
    ${renderSettingsPanelHeader(title, copy, iconKey)}
    <div class="settings-group settings-group-update">${body}</div>
  </section>`;
}

function renderRootDebugPreviewBlock(title, copy, preview) {
  return `<div class="root-debug-preview-block">
    <div class="settings-static">
      <strong>${escapeHTML(title)}</strong>
      ${copy ? `<span>${escapeHTML(copy)}</span>` : ''}
    </div>
    <div class="root-debug-module-preview">${preview}</div>
  </div>`;
}

function renderRootDebugBlockingCard(title, detail, percent, cancelText = copyText('app.interface.commonCancel')) {
  return `<div class="blocking-progress-card">
    <div class="spinner"></div>
    <strong>${escapeHTML(title)}</strong>
    <span>${escapeHTML(detail)}</span>
    <div class="blocking-progress-track"><div style="width:${Math.max(0, Math.min(100, Number(percent) || 0))}%"></div></div>
    <button class="btn btn-sm blocking-progress-cancel" type="button">${escapeHTML(cancelText)}</button>
  </div>`;
}

function renderRootDebugLibraryBuildPreview() {
  const status = {
    message: copyText('app.interface.s_f840c2bbf5'),
    done: 1287,
    total: 0,
    percent: 0,
    elapsed_seconds: 73,
    eta_seconds: 0,
    pruned: 18,
    exit_after_complete: false,
  };
  const percent = estimateLibraryBuildDiscoveryPercent(status.done);
  return `<section class="library-build-panel">
    <div class="library-build-heading">
      <div>
        <h2>${escapeHTML(status.message)}</h2>
        <p>${escapeHTML(copyText('app.interface.s_f3b6e21b06'))}</p>
      </div>
      <span class="library-build-badge">${escapeHTML(copyText('app.interface.s_5942497005'))}</span>
    </div>
    <div class="library-build-progress">
      <div class="library-build-progress-fill" style="width:${percent}%"></div>
    </div>
    <div class="library-build-stats">
      <div><strong>${escapeHTML(copyText('app.interface.d_d749755d89', { v0: (status.done) }))}</strong><span>${escapeHTML(copyText('app.interface.s_b1395d9a33'))}</span></div>
      <div><strong>${Math.round(percent)}%</strong><span>${escapeHTML(copyText('app.interface.s_4fbedf689c'))}</span></div>
      <div><strong>${formatSecondsShort(status.elapsed_seconds)}</strong><span>${escapeHTML(copyText('app.interface.s_1bf38effca'))}</span></div>
      <div><strong>${escapeHTML(copyText('app.interface.s_40ab4a28b4'))}</strong><span>${escapeHTML(copyText('app.interface.s_5886b1dac6'))}</span></div>
      <div><strong>${Number(status.pruned)}</strong><span>${escapeHTML(copyText('app.interface.s_83e103fd97'))}</span></div>
    </div>
    <div class="library-build-exit">
      ${renderSettingsToggle('root-debug-build-exit-after', copyText('app.interface.s_ce45a083b2'), !!status.exit_after_complete)}
    </div>
    <div class="library-build-actions">
      <button class="btn btn-danger library-build-cancel" type="button">${escapeHTML(copyText('app.interface.runtime.stopBuild'))}</button>
    </div>
  </section>`;
}

function renderRootDebugPlaybackCachePreview() {
  return `<div class="modal playback-cache-modal root-debug-modal-card">
    <div class="modal-title">${copyText('app.interface.s_f07e25890b')}</div>
    <p class="modal-copy">${escapeHTML(copyText('app.interface.runtime.playbackCacheCopy'))}</p>
    <div class="playback-cache-toolbar">
      <label class="playback-cache-select-all">
        <span class="settings-toggle-switch playback-cache-check">
          <input type="checkbox" checked>
          <span class="settings-toggle-slider" aria-hidden="true"></span>
        </span>
        <span>${escapeHTML(copyText('app.interface.runtime.selectAllCache'))}</span>
      </label>
      <span>${escapeHTML(copyText('app.interface.runtime.playbackCacheCount', { count: 2, selected: 1 }))}</span>
    </div>
    <div class="playback-cache-progress"><span style="width:38%"></span></div>
    <div class="playback-cache-status"><span>${escapeHTML(copyText('app.interface.runtime.buildingPlaybackCache'))}</span><span>14 / 60</span><span>${escapeHTML(copyText('app.interface.runtime.generatedCount', { count: 12 }))}</span><span>${escapeHTML(copyText('app.interface.runtime.skippedCount', { count: 2 }))}</span></div>
    <div class="playback-cache-list">
      <label class="playback-cache-row is-selected">
        <span class="settings-toggle-switch playback-cache-check">
          <input type="checkbox" checked>
          <span class="settings-toggle-slider" aria-hidden="true"></span>
        </span>
        <span class="playback-cache-main">
          <strong>IMG_1413.MOV</strong>
          <span>video/quicktime · 174.3 MB · ${copyText('app.interface.d_generated_at', { date: formatDateTime(new Date().toISOString()) })}</span>
          <small title="playback-cache/debug-cache-1.mp4">playback-cache/debug-cache-1.mp4</small>
        </span>
      </label>
      <label class="playback-cache-row">
        <span class="settings-toggle-switch playback-cache-check">
          <input type="checkbox">
          <span class="settings-toggle-slider" aria-hidden="true"></span>
        </span>
        <span class="playback-cache-main">
          <strong>SAM_0209.MP4</strong>
          <span>video/mp4 · 90.4 MB · ${copyText('app.interface.d_generated_at', { date: formatDateTime(new Date().toISOString()) })}</span>
          <small title="playback-cache/debug-cache-2.mp4">playback-cache/debug-cache-2.mp4</small>
        </span>
      </label>
    </div>
    <div class="modal-footer playback-cache-actions">
      <button class="btn" type="button">${copyText('app.interface.commonClose')}</button>
      <button class="btn" type="button">${escapeHTML(copyText('app.interface.runtime.deleteSelected'))}</button>
      <button class="btn" type="button">${copyText('app.interface.s_7cf36613f2')}</button>
      <button class="btn btn-primary" type="button">${escapeHTML(copyText('app.interface.runtime.startConversion'))}</button>
    </div>
  </div>`;
}

function renderRootDebugTimelineLoadPreview(title, copy, loaded, total, buttonText = copyText('app.interface.s_26f8b9e0eb')) {
  const percent = total > 0 ? Math.round((loaded / total) * 100) : 0;
  return `<div class="modal root-debug-modal-card">
    <div class="modal-title">${escapeHTML(title)}</div>
    <p class="modal-copy">${escapeHTML(copy)}</p>
    <div class="timeline-load-progress"><div class="timeline-load-progress-bar" style="width:${percent}%"></div></div>
    <div class="timeline-load-status">${escapeHTML(copyText('app.interface.d_97cd0bf9d4', { v0: (loaded), v1: (total) }))}</div>
    <div class="modal-footer">
      <button class="btn" type="button">${copyText('app.interface.commonCancel')}</button>
      <button class="btn btn-primary" type="button">${escapeHTML(buttonText)}</button>
    </div>
  </div>`;
}

function renderRootDebugUploadPreview() {
  return `<div class="modal root-debug-modal-card" style="width:520px">
    <div class="modal-title">${icons.upload} ${escapeHTML(copyText('app.interface.runtime.uploadMedia'))}</div>
    <div class="upload-zone">
      ${icons.upload}
      <div style="margin-top:8px">${escapeHTML(copyText('app.interface.runtime.dropMedia'))}</div>
      <div style="font-size:.8rem;margin-top:4px">${escapeHTML(copyText('app.interface.runtime.supportedFormats'))}</div>
    </div>
    <div class="upload-queue">
      <div class="upload-item">
        <span class="up-name">IMG_7688.JPG</span>
        <div style="flex:1"><div class="progress-bar"><div class="progress-fill" style="width:72%"></div></div></div>
        <span class="up-status">${escapeHTML(copyText('app.interface.runtime.uploading'))}</span>
        <button class="btn btn-sm" style="display:none" type="button">${escapeHTML(copyText('app.interface.runtime.retryFailed'))}</button>
      </div>
      <div class="upload-item">
        <span class="up-name">MVI_8411.MOV</span>
        <div style="flex:1"><div class="progress-bar"><div class="progress-fill" style="width:100%"></div></div></div>
        <span class="up-status done">${escapeHTML(copyText('app.interface.runtime.complete'))}</span>
        <button class="btn btn-sm" style="display:none" type="button">${escapeHTML(copyText('app.interface.runtime.retryFailed'))}</button>
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn" type="button" style="display:none">${escapeHTML(copyText('app.interface.runtime.retryFailed'))}</button>
      <button class="btn" type="button">${copyText('app.interface.commonClose')}</button>
    </div>
  </div>`;
}

function renderRootDebugAlbumPickerPreview() {
  return `<div class="modal root-debug-modal-card" style="width:480px">
    <div class="modal-title">${icons.album} ${escapeHTML(copyText('app.interface.runtime.chooseAlbum'))}</div>
    <div class="album-picker-grid-viewport">
      <div class="album-picker-grid">
      <div class="album-card album-card-user album-card-picker is-selected" role="button" aria-pressed="true">
        <div class="album-cover"><div class="album-cover-empty album-cover-empty-user" aria-hidden="true">${icons.albumCardUser || icons.photo}</div></div>
        <div class="album-info">
          <div class="album-name">${escapeHTML(copyText('app.interface.runtime.demoTravelAlbum'))}</div>
          <div class="album-subtitle">${copyText('app.interface.albumUserCreated')}</div>
          <div class="album-count-row">${icons.albumCardCount || icons.photo}<span>${escapeHTML(copyText('app.interface.runtime.itemCount', { count: 128 }))}</span></div>
          <span class="album-kind">${icons.albumCardUser}</span>
        </div>
      </div>
      <div class="album-card album-card-user album-card-picker" role="button" aria-pressed="false">
        <div class="album-cover"><div class="album-cover-empty album-cover-empty-user" aria-hidden="true">${icons.albumCardUser || icons.photo}</div></div>
        <div class="album-info">
          <div class="album-name">${escapeHTML(copyText('app.interface.runtime.demoFavoritesAlbum'))}</div>
          <div class="album-subtitle">${copyText('app.interface.albumUserCreated')}</div>
          <div class="album-count-row">${icons.albumCardCount || icons.photo}<span>${escapeHTML(copyText('app.interface.runtime.itemCount', { count: 64 }))}</span></div>
          <span class="album-kind">${icons.albumCardUser}</span>
        </div>
      </div>
      <div class="album-card album-card-user album-card-picker" role="button" aria-pressed="false">
        <div class="album-cover"><div class="album-cover-empty album-cover-empty-user" aria-hidden="true">${icons.albumCardUser || icons.photo}</div></div>
        <div class="album-info">
          <div class="album-name">${escapeHTML(copyText('app.interface.runtime.demoUnsortedAlbum'))}</div>
          <div class="album-subtitle">${copyText('app.interface.albumUserCreated')}</div>
          <div class="album-count-row">${icons.albumCardCount || icons.photo}<span>${escapeHTML(copyText('app.interface.runtime.itemCount', { count: 23 }))}</span></div>
          <span class="album-kind">${icons.albumCardUser}</span>
        </div>
      </div>
      </div>
    </div>
    <div style="font-size:.8rem;color:var(--text2);margin-top:10px">${escapeHTML(copyText('app.interface.runtime.stableDebugHint'))}</div>
    <div class="modal-footer">
      <button class="btn" type="button">${copyText('app.interface.commonCancel')}</button>
      <button class="btn btn-primary" type="button">${copyText('app.interface.commonAdd')}</button>
    </div>
  </div>`;
}

function renderRootDebugShareModalPreview() {
  return `<div class="modal root-debug-modal-card root-debug-share-modal">
    <div class="settings-share-row">
      <div class="settings-share-preview has-image"><img loading="lazy" src="${rootDebugPreviewImage}" alt="debug share preview"></div>
      <div class="settings-share-main">
        <div class="settings-share-title">${escapeHTML(copyText('app.interface.runtime.sharePhoto'))}</div>
        <p class="settings-share-meta">${escapeHTML(copyText('app.interface.runtime.shareChoose'))}</p>
      </div>
    </div>
    <div class="settings-share-actions">
      <button class="btn" type="button" style="${escapeHTML(currentAccentButtonStyle())}">${icons.shareModalLink} ${escapeHTML(copyText('app.interface.runtime.link'))}</button>
      <button class="btn" type="button">${icons.close} ${escapeHTML(copyText('app.interface.runtime.cancel'))}</button>
    </div>
  </div>`;
}

function renderRootDebugShareListPreview() {
  return `<div class="modal root-debug-modal-card" style="width:480px">
    <div class="modal-title">${icons.share} ${escapeHTML(copyText('app.interface.runtime.manageShares'))}</div>
    <div id="share-list-content">
      <div style="display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--border);font-size:.85rem;">
        <div style="flex:1;overflow:hidden">
          <div style="font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${escapeHTML(`${location.origin}/s/debug-share-token-1`)}</div>
          <div style="color:var(--text2);font-size:.75rem;margin-top:2px">${copyText('app.interface.shareNeverExpires')}</div>
        </div>
        <button class="btn btn-sm" type="button">${escapeHTML(copyText('app.interface.runtime.copy'))}</button>
        <button class="btn btn-sm btn-danger" type="button">${copyText('app.interface.commonDelete')}</button>
      </div>
      <div style="display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--border);font-size:.85rem;">
        <div style="flex:1;overflow:hidden">
          <div style="font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${escapeHTML(`${location.origin}/s/debug-share-token-2`)}</div>
          <div style="color:var(--text2);font-size:.75rem;margin-top:2px">${copyText('app.interface.shareNeverExpires')}</div>
        </div>
        <button class="btn btn-sm" type="button">${escapeHTML(copyText('app.interface.runtime.copy'))}</button>
        <button class="btn btn-sm btn-danger" type="button">${copyText('app.interface.commonDelete')}</button>
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn" type="button">${copyText('app.interface.commonCancel')}</button>
      <button class="btn btn-primary" type="button">${escapeHTML(copyText('app.interface.runtime.newShare'))}</button>
    </div>
  </div>`;
}

async function renderRootDebugTimeline() {
  $('#topbar-title').textContent = copyText('app.interface.navigationDebug');
  $('#topbar-leading').innerHTML = '';
  $('#topbar-meta').innerHTML = '';
  $('#topbar-actions').innerHTML = '';
  $('#content').innerHTML = `
<div class="settings-layout root-debug-workbench">
  ${renderRootDebugWorkbenchPanel({
    title: copyText('app.interface.s_ff1af925dc'),
    copy: copyText('app.interface.s_806c0ce425'),
    iconKey: 'settingsPanelHeadBlank1',
    body: renderRootDebugPreviewBlock(copyText('app.interface.s_fc641f0b17'), copyText('app.interface.s_bbbbfb3675'), `<div class="root-debug-library-build-preview">${renderRootDebugLibraryBuildPreview()}</div>`),
  })}
  ${renderRootDebugWorkbenchPanel({
    title: copyText('app.interface.s_17105effed'),
    copy: copyText('app.interface.s_313cf91a8b'),
    iconKey: 'settingsPanelHeadBlank2',
    body: `<div class="root-debug-preview-grid root-debug-progress-grid">
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_a5d12ab217'), copyText('app.interface.s_3dd9331f3a'), renderRootDebugBlockingCard(copyText('app.interface.s_1e2142b559'), copyText('app.interface.s_4a25714117'), 36))}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_3b6d6b5f12'), copyText('app.interface.s_769549fd5d'), renderRootDebugBlockingCard(copyText('app.interface.s_7c23ed20a4'), copyText('app.interface.s_9739fdc528'), 29))}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_146cc0eb9b'), copyText('app.interface.s_5aeb4c32c3'), renderRootDebugBlockingCard(copyText('app.interface.s_3b79d3ec78'), copyText('app.interface.s_38c8ae2202'), 27))}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_cc240bfd48'), copyText('app.interface.s_0de1fa7fba'), renderRootDebugBlockingCard(copyText('app.interface.s_3faa2ed1ab'), copyText('app.interface.s_df7754aebf'), 25))}
    </div>`,
  })}
  ${renderRootDebugWorkbenchPanel({
    title: copyText('app.interface.s_d1dadca4ed'),
    copy: copyText('app.interface.s_2999740723'),
    iconKey: 'settingsPanelHeadBlank3',
    body: `<div class="root-debug-preview-grid">
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_f07e25890b'), copyText('app.interface.s_021699a1b1'), renderRootDebugPlaybackCachePreview())}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_08584f8db6'), copyText('app.interface.s_5108b43b5a'), renderRootDebugTimelineLoadPreview(copyText('app.interface.s_d95504a29c'), copyText('app.interface.s_1dba037b4d'), 240, 1200))}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_485d770a0d'), copyText('app.interface.s_969e056bcb'), renderRootDebugTimelineLoadPreview(copyText('app.interface.s_465cb7fa80'), copyText('app.interface.s_c139034ea0'), 180, 900, copyText('app.interface.s_c2ca5668a2')))}
    </div>`,
  })}
  ${renderRootDebugWorkbenchPanel({
    title: copyText('app.interface.s_0648f81b8c'),
    copy: copyText('app.interface.s_c4e50c4e2c'),
    iconKey: 'settingsPanelHeadBlank4',
    body: `<div class="root-debug-preview-grid">
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_c18deae5e2'), copyText('app.interface.s_8a7214ddfb'), renderRootDebugUploadPreview())}
      ${renderRootDebugPreviewBlock(copyText('app.interface.albumAddToAlbum'), copyText('app.interface.s_6b3b7b0ac1'), renderRootDebugAlbumPickerPreview())}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_d08a1b16f9'), copyText('app.interface.s_9371585f7e'), renderRootDebugShareModalPreview())}
      ${renderRootDebugPreviewBlock(copyText('app.interface.s_0db5a29771'), copyText('app.interface.s_34d8993cf4'), renderRootDebugShareListPreview())}
    </div>`,
  })}
</div>`;
}

function openRootUserEditor(user = null) {
  const editing = !!user;
  const isProtectedRoot = editing && user.role === 'root';
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  const selectedAllowed = new Set(Array.isArray(user && user.allowed_library_ids) ? user.allowed_library_ids : []);
  if (!editing && !selectedAllowed.size && libraries[0] && libraries[0].id) {
    selectedAllowed.add(libraries[0].id);
  }
  const initialRole = isProtectedRoot ? 'root' : String(user && user.role || 'visitor');
  const modal = el('div', 'modal-overlay open');
  modal.innerHTML = `
    <div class="modal library-editor-card">
      <div class="modal-title">${editing ? copyText('app.interface.s_2b8f00100b') : copyText('app.interface.s_30416407e5')}</div>
      <div class="settings-group">
        <div class="settings-control">
          <label for="root-user-username"><span>${escapeHTML(copyText('app.interface.runtime.username'))}</span></label>
          <input class="input" id="root-user-username" type="text" value="${escapeHTML(editing ? user.username : '')}" ${isProtectedRoot ? 'disabled' : ''} placeholder="${escapeHTML(copyText('app.interface.runtime.usernameExample'))}">
        </div>
        <div class="settings-control">
          <label for="root-user-password"><span>${editing ? copyText('app.interface.s_d22c9c0085') : copyText('app.interface.s_c839a8ff17')}</span><span>${editing ? copyText('app.interface.s_ab786199c3') : copyText('app.interface.s_fae79d5b88')}</span></label>
          <input class="input" id="root-user-password" type="password" value="" placeholder="${editing ? copyText('app.interface.s_8df42bda48') : copyText('app.interface.s_1969133807')}">
        </div>
        <div class="settings-control">
          <label for="root-user-role"><span>${escapeHTML(copyText('app.interface.runtime.role'))}</span></label>
          <select class="input" id="root-user-role" ${isProtectedRoot ? 'disabled' : ''}>
            ${isProtectedRoot ? '<option value="root" selected>root</option>' : `
              <option value="visitor" ${initialRole === 'visitor' ? 'selected' : ''}>${copyText('app.interface.s_2f7ccc8479')}</option>
              <option value="admin" ${initialRole === 'admin' ? 'selected' : ''}>${copyText('app.interface.s_ef84e765e0')}</option>`}
          </select>
        </div>
        <div class="settings-control" id="root-user-libraries-wrap">
          <label><span>${escapeHTML(copyText('app.interface.runtime.libraryAuthorization'))}</span><span id="root-user-libraries-hint">${initialRole === 'admin' ? copyText('app.interface.s_942ee4ff15') : copyText('app.interface.s_d2d1bcb9ba')}</span></label>
          <div class="root-console-library-list" id="root-user-library-list">
            ${libraries.map(library => `
              <label class="root-console-library-item">
                <input type="checkbox" value="${escapeHTML(library.id || '')}" ${initialRole === 'admin' ? 'checked disabled' : (selectedAllowed.has(library.id) ? 'checked' : '')}>
                <span>${escapeHTML(library.name)}</span>
              </label>`).join('')}
          </div>
        </div>
        <div class="settings-control">
          <label for="root-user-default-library"><span>${copyText('app.interface.s_0642c4b8bf')}</span></label>
          <select class="input" id="root-user-default-library"></select>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" id="root-user-cancel" type="button">${copyText('app.interface.commonCancel')}</button>
        <button class="btn btn-primary" id="root-user-save" type="button">${editing ? copyText('app.interface.commonSave') : copyText('app.interface.commonCreate')}</button>
      </div>
    </div>`;
  document.body.appendChild(modal);

  const usernameInput = $('#root-user-username', modal);
  const passwordInput = $('#root-user-password', modal);
  const roleSelect = $('#root-user-role', modal);
  const defaultSelect = $('#root-user-default-library', modal);
  const libraryWrap = $('#root-user-libraries-wrap', modal);
  const libraryInputs = () => $$('input[type="checkbox"]', $('#root-user-library-list', modal));
  const close = () => modal.remove();

  const syncDefaultOptions = () => {
    const role = roleSelect.value;
    const availableLibraries = libraries.filter(library => role === 'admin' || libraryInputs().some(input => input.checked && input.value === library.id));
    defaultSelect.innerHTML = availableLibraries.length
      ? availableLibraries.map(library => `<option value="${escapeHTML(library.id)}" ${library.id === (user && user.default_library_id) ? 'selected' : ''}>${escapeHTML(library.name)}</option>`).join('')
      : `<option value="">${copyText('app.interface.s_unspecified')}</option>`;
    if (!availableLibraries.some(library => library.id === defaultSelect.value)) {
      defaultSelect.value = availableLibraries[0]?.id || '';
    }
  };

  const syncRoleState = () => {
    const role = roleSelect.value;
    const isAdmin = role === 'admin';
    const isRoot = role === 'root';
    if (libraryWrap) libraryWrap.hidden = isRoot;
    libraryInputs().forEach(input => {
      if (isAdmin) {
        input.checked = true;
        input.disabled = true;
      } else {
        input.disabled = false;
      }
    });
    const hint = $('#root-user-libraries-hint', modal);
    if (hint) {
      hint.textContent = isRoot
        ? copyText('app.interface.s_0415771f48')
        : (isAdmin ? copyText('app.interface.s_942ee4ff15') : copyText('app.interface.s_d2d1bcb9ba'));
    }
    syncDefaultOptions();
  };

  roleSelect?.addEventListener('change', syncRoleState);
  libraryInputs().forEach(input => input.addEventListener('change', syncDefaultOptions));
  $('#root-user-cancel', modal)?.addEventListener('click', close);
  modal.addEventListener('click', e => {
    if (e.target === modal) close();
  });
  $('#root-user-save', modal)?.addEventListener('click', async () => {
    const payload = {
      username: usernameInput.value.trim(),
      password: passwordInput.value,
      role: roleSelect.value,
      allowed_library_ids: libraryInputs().filter(input => input.checked).map(input => input.value),
      default_library_id: defaultSelect.value || '',
    };
    if (!editing && (!payload.username || !payload.password)) {
      showToast(copyText('app.interface.s_85362f2da5'));
      return;
    }
    try {
      const resp = editing
        ? await updateRootUser(user.username, {
          new_username: isProtectedRoot ? '' : payload.username,
          new_password: payload.password,
          role: payload.role,
          allowed_library_ids: payload.allowed_library_ids,
          default_library_id: payload.default_library_id,
        })
        : await createRootUser(payload);
      applyRootUsersPayload(resp);
      await renderRootConsoleContent();
      showToast(editing ? copyText('app.interface.s_c78135f467') : copyText('app.interface.s_da1f8fdfe9'));
      close();
    } catch (e) {
      appAlert((e && e.error) || String(e), { title: editing ? copyText('app.interface.s_40525a7328') : copyText('app.interface.s_deb3990191') });
    }
  });
  syncRoleState();
}

async function renderRootConsoleContent({ host = settingsViewHost(), force = false } = {}) {
  if (!state.rootConsoleLoaded) {
    host.innerHTML = `<div class="load-more"><div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.loadingRootConsole'))}</div>`;
    await ensureRootConsoleUsersLoaded();
  }
  if (!force && (!isRootUser() || state.view !== 'settings')) return;
  host.innerHTML = `
<div class="settings-layout">
  <section class="card settings-panel settings-panel-application">
    ${renderSettingsPanelHeader(copyText('app.interface.s_13c4b99ee4'), copyText('app.interface.s_8f4034b363'), settingsPanelHeadIconMap.rootConsole)}
    <div class="settings-group">
      <div class="settings-actions settings-actions-fill">
        <button class="btn btn-primary" id="root-create-user-btn" type="button">${icons.add || icons.plus || '+'} ${escapeHTML(copyText('app.interface.runtime.newUser'))}</button>
        <button class="btn" id="root-refresh-users-btn" type="button">${escapeHTML(copyText('app.interface.runtime.refreshList'))}</button>
      </div>
    </div>
  </section>
  <section class="card settings-panel settings-panel-batch-workflow">
    <div id="settings-library-batch-workflow">${renderLibraryBatchWorkflowPanel()}</div>
  </section>
  <section class="card settings-panel settings-panel-share">
    ${renderSettingsPanelHeader(copyText('app.interface.s_6b2b5d5355'), copyText('app.interface.s_02d19effb0'), settingsPanelHeadIconMap.userAccess)}
    <div class="settings-group">
      <div class="settings-library-list root-user-list" id="root-user-list">${renderRootConsoleUserRows()}</div>
    </div>
  </section>
</div>`;

  bindSettingsPanelToggles(host);
  state.settingsMounted = true;
  state.settingsRenderMode = 'root';

  $('#root-create-user-btn')?.addEventListener('click', () => openRootUserEditor());
  $('#root-refresh-users-btn')?.addEventListener('click', async () => {
    await ensureRootConsoleUsersLoaded(true);
    renderRootConsoleContent();
    showToast(copyText('app.interface.s_62a322e9c5'));
  });
  if (canAccessBatchWorkflow()) syncLibraryBatchWorkflowPanel();
  $$('[data-root-edit]').forEach(button => {
    button.addEventListener('click', () => {
      const user = state.rootConsoleUsers.find(item => item.username === button.dataset.rootEdit);
      if (user) openRootUserEditor(user);
    });
  });
  $$('[data-root-delete]').forEach(button => {
    button.addEventListener('click', async () => {
      const username = String(button.dataset.rootDelete || '').trim();
      if (!username) return;
      if (!(await appConfirm(copyText('app.interface.d_f214fd2169', { v0: (username) }), { danger: true }))) return;
      try {
        const resp = await removeRootUser(username);
        applyRootUsersPayload(resp);
        renderRootConsoleContent();
        showToast(copyText('app.interface.s_7f894ecd04'));
      } catch (e) {
        appAlert((e && e.error) || String(e), { title: copyText('app.interface.s_72250c5922'), danger: true });
      }
    });
  });
}

async function renderSettings() {
  scheduleSettingsAvailabilityPolling();
  if (state.settingsReady && state.view === 'settings') {
    await refreshSettingsAvailability();
  }
  if (state.view !== 'settings') return;
  setSettingsViewVisible(true);
  const host = settingsViewHost();
  if (isRootUser()) {
    $('#topbar-title').textContent = copyText('app.interface.s_13c4b99ee4');
    $('#topbar-leading').innerHTML = '';
    $('#topbar-meta').innerHTML = '';
    $('#topbar-actions').innerHTML = '';
    if (!state.settingsReady) {
      host.innerHTML = `<div class="load-more"><div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.loadingRootConsole'))}</div>`;
      await ensureSettingsDataLoaded();
    }
    if (state.view !== 'settings') return;
    try {
      await ensureSettingsViewMounted();
      if (state.settingsScrollRestorePending) {
        restoreViewScroll('settings');
        state.settingsScrollRestorePending = false;
      }
    } catch (e) {
      console.error('render root console failed', e);
      host.innerHTML = `<div class="card settings-panel"><h3>${escapeHTML(copyText('app.interface.runtime.rootConsoleLoadFailed'))}</h3><p style="color:var(--danger)">${escapeHTML((e && e.message) || e)}</p></div>`;
    }
    return;
  }
  $('#topbar-title').textContent = settingsText('common.pageTitle');
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'settings-save-top-btn', icon: icons.topbarSave, label: settingsText('common.save'), variant: 'accent', disabled: !state.settingsDirty }),
  ]);
  $('#topbar-meta').innerHTML = '';
  $('#topbar-actions').innerHTML = '';
  const canRunServerTasksUI = canRunServerTasks();
  if (!state.settingsReady) {
    host.innerHTML = `<div class="load-more"><div class="spinner"></div>${escapeHTML(settingsText('common.loading'))}</div>`;
    await ensureSettingsDataLoaded();
  }

  if (state.view !== 'settings') return;

  try {
    const didMountSettingsView = await ensureSettingsViewMounted();
    bindSettingsTopSaveButton();
    if (!didMountSettingsView && canRunServerTasksUI) {
      void pollVideoThumbnailRefreshStatus();
      void pollEXIFBackfillStatus();
      void pollThumbnailBuildStatus();
    }
    if (state.settingsScrollRestorePending) {
      restoreViewScroll('settings');
      state.settingsScrollRestorePending = false;
    }
    refreshMissingLibraryLogosOnce();
    refreshShareManagementUI();
    observeDeferredSettingsShareLoad();
    if (state.settingsFocus === 'libraries') {
      const target = $('#settings-library-list');
      if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      state.settingsFocus = '';
    }
  } catch (e) {
    console.error('render settings failed', e);
    host.innerHTML = `<div class="card settings-panel"><h3>${escapeHTML(settingsText('common.loadFailedTitle'))}</h3><p>${escapeHTML(settingsText('common.loadFailedCopy'))}</p><p style="color:var(--danger)">${(e && e.message) || e}</p></div>`;
    if (state.settingsScrollRestorePending) {
      restoreViewScroll('settings');
      state.settingsScrollRestorePending = false;
    }
  }
}

function renderLibrarySettingsRows() {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  if (!libraries.length) {
    return `<div class="settings-empty">${escapeHTML(settingsText('libraries.empty'))}</div>`;
  }
  return libraries.map((library, index) => renderLibrarySettingsRow(library, index)).join('');
}

function renderLibraryAccentSwatches(accentColor) {
  const color = normalizeHexColor(accentColor) || defaultLibraryAccentPalette[0];
  const weights = [100, 82, 64, 46, 30, 18];
  return `<div class="settings-library-swatches" aria-hidden="true">
    ${weights.map((weight, index) => `<span style="--swatch-mix:${weight}%;--swatch-color:${color};--swatch-layer:${weights.length - index}"></span>`).join('')}
  </div>`;
}

function formatLibraryShortDate(value) {
  const raw = String(value || '').trim();
  if (!raw) return settingsText('common.emptyValue');
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return `${date.getFullYear()}.${date.getMonth() + 1}.${date.getDate()}`;
}

function formatLibraryRelativeTime(value) {
  const raw = String(value || '').trim();
  if (!raw) return settingsText('common.emptyValue');
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  const diff = Date.now() - date.getTime();
  if (diff < 0) return formatLibraryShortDate(raw);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff < minute) return settingsText('common.justNow');
  if (diff < hour) return settingsText('common.minutesAgo', { count: Math.floor(diff / minute) });
  if (diff < day) return settingsText('common.hoursAgo', { count: Math.floor(diff / hour) });
  if (diff < day * 7) return settingsText('common.daysAgo', { count: Math.floor(diff / day) });
  return formatLibraryShortDate(raw);
}

function formatLibraryStorageValue(value) {
  const raw = String(value || '').trim();
  if (!raw) return settingsText('common.emptyValue');
  return raw.replace(/(\d)\s+(B|KB|MB|GB|TB|PB)\b/i, '$1$2');
}

function libraryStatValue(library = {}, keys = [], fallback = settingsText('common.emptyValue')) {
  for (const key of keys) {
    const value = library[key];
    if (value === 0) return '0';
    if (value != null && String(value).trim() !== '') return String(value);
  }
  return fallback;
}

function renderLibraryInfoItem(icon, label, value, { compactLabel = true } = {}) {
  const safeValue = value === 0 ? '0' : String(value || settingsText('common.emptyValue'));
  return `<div class="settings-library-info-item">
    <span class="settings-library-info-icon" aria-hidden="true">${icon || ''}</span>
    <span class="settings-library-info-label${compactLabel ? ' pc-hidden' : ''}">${escapeHTML(label)}</span>
    <span class="settings-library-info-value">${escapeHTML(safeValue)}</span>
  </div>`;
}

function renderLibraryInfoGrid(library = {}, { availabilityMetaText = '', availabilityDetailText = '' } = {}) {
  const created = formatLibraryShortDate(library.created_at || library.createdAt);
  const scannedAt = libraryStatValue(library, ['last_scanned_at', 'lastScannedAt', 'last_scan_at', 'lastScanAt'], '');
  const scanned = scannedAt ? formatLibraryRelativeTime(scannedAt) : (availabilityMetaText || settingsText('common.emptyValue'));
  const unsupported = libraryStatValue(library, ['unsupported_media_count', 'unsupportedMediaCount'], availabilityDetailText || '0');
  const storage = formatLibraryStorageValue(libraryStatValue(library, ['total_size_text', 'totalSizeText', 'storage_text', 'storageText'], library.status || (library.available === false ? copyText('app.interface.s_beff4a1cd1') : copyText('app.interface.s_e91365cf9e'))));
  const photos = libraryStatValue(library, ['photo_count', 'photoCount', 'image_count', 'imageCount'], '0');
  const videos = libraryStatValue(library, ['video_count', 'videoCount'], '0');
  return `<div class="settings-library-info-grid">
    ${renderLibraryInfoItem(icons.libraryInfoCreated, settingsText('libraries.info.created'), created)}
    ${renderLibraryInfoItem(icons.libraryInfoScanned, settingsText('libraries.info.scanned'), scanned)}
    ${renderLibraryInfoItem(icons.libraryInfoUnsupported, settingsText('libraries.info.unsupported'), unsupported)}
    ${renderLibraryInfoItem(icons.libraryInfoStorage, settingsText('libraries.info.storage'), storage)}
    ${renderLibraryInfoItem(icons.libraryInfoPhotos, settingsText('libraries.info.photos'), photos)}
    ${renderLibraryInfoItem(icons.libraryInfoVideos, settingsText('libraries.info.videos'), videos)}
  </div>`;
}

function renderLibrarySettingsRow(library = {}, index = 0) {
  const libraryIndex = Number.isInteger(Number(library.index)) ? Number(library.index) : index;
  const id = library.id || '';
  const name = library.name || settingsText('libraries.nameFallback', { index: index + 1 });
  const path = library.path || '';
  const logoAsset = library.logo_asset || '';
  const logoURL = resolveLibraryLogoURL(library);
  const accentColor = resolveLibraryAccentColor(library);
  const available = library.available !== false;
  const unavailableReason = String(library.unavailable_reason || '').trim();
  const availabilityMetaText = libraryAvailabilityMetaText(library);
  const availabilityDetailText = libraryAvailabilityDetailText(library);
  const isActive = (id && id === (state.serverSettings.active_library_id || '')) || (!id && path && path === (state.serverSettings.storage_path || ''));
  const isPrimaryLibrary = libraryIndex === 0;
  return `
    <div class="settings-library-row${isActive ? ' active' : ''}${available ? '' : ' is-unavailable'}" draggable="${available ? 'true' : 'false'}" data-library-id="${escapeHTML(id)}" data-library-index="${libraryIndex}" data-logo-asset="${escapeHTML(logoAsset)}" data-logo-preview-url="${escapeHTML(logoURL)}" data-logo-action="" data-library-available="${available ? 'true' : 'false'}" data-library-unavailable-reason="${escapeHTML(unavailableReason)}" style="--library-accent:${accentColor}">
      <div class="settings-library-main">
        <div class="settings-library-logo-block">
          <div class="settings-library-logo-preview"></div>
        </div>
        <div class="settings-library-fields">
          <input class="settings-library-name" type="hidden" value="${escapeHTML(name)}">
          <input class="settings-library-path" type="hidden" value="${escapeHTML(path)}">
          <input class="settings-library-accent" type="hidden" value="${accentColor}">
          <div class="settings-library-title-row">
            <div class="settings-library-name-display">
              <span>${escapeHTML(name)}</span>
              ${isPrimaryLibrary ? `<span class="settings-library-primary-mark" title="${escapeHTML(settingsText('libraries.primary'))}" aria-label="${escapeHTML(settingsText('libraries.primary'))}">${icons.superLike || ''}</span>` : ''}
            </div>
            ${available ? '' : `<span class="settings-library-state-badge is-unavailable">${escapeHTML(settingsText('libraries.unavailable'))}</span>`}
            ${library.rebuild_required ? `<span class="settings-library-state-badge">${escapeHTML(copyText('app.interface.runtime.needsRebuild'))}</span>` : ''}
          </div>
          <div class="settings-library-path-display">${escapeHTML(path || settingsText('libraries.pathUnset'))}</div>
          <div class="settings-library-meta-row">
            ${renderLibraryAccentSwatches(accentColor)}
          </div>
        </div>
      </div>
      ${renderLibraryInfoGrid(library, { availabilityMetaText, availabilityDetailText })}
    </div>`;
}

function collectLibraryInputs() {
  return collectLibraryDrafts().map((library, index) => ({
    id: library.id || '',
    name: library.name || settingsText('libraries.nameFallback', { index: index + 1 }),
    path: library.path || '',
    logo_asset: library.logo_asset || '',
    accent_color: normalizeHexColor(library.accent_color) || '',
  })).filter(item => item.path);
}

function collectLibraryDrafts() {
  const rows = $$('.settings-library-row').filter(row =>
    !!$('.settings-library-name', row)
    && !!$('.settings-library-path', row)
    && !!$('.settings-library-accent', row)
  );
  if (!rows.length) return [];
  return rows.map((row, index) => {
    const nameInput = $('.settings-library-name', row);
    const pathInput = $('.settings-library-path', row);
    const accentInput = $('.settings-library-accent', row);
    const draft = {
      id: row.dataset.libraryId || '',
      name: nameInput.value.trim() || settingsText('libraries.nameFallback', { index: index + 1 }),
      path: pathInput.value.trim(),
      logo_asset: row.dataset.logoAction === 'remove' ? '' : row.dataset.logoAsset || '',
      logo_image_url: row.dataset.logoPreviewUrl || '',
      accent_color: normalizeHexColor(accentInput.value) || '',
    };
    return mergeLibraryDraftWithState(draft);
  }).filter(item => item.path);
}

function syncLibraryDraftsToRuntime() {
  const libraries = collectLibraryDrafts();
  if (!libraries.length) return [];
  state.serverSettings.libraries = normalizeLibraryList(libraries, state.serverSettings.storage_path || '');
  if (!state.serverSettings.libraries.some(library => (library.id && library.id === state.serverSettings.active_library_id) || library.path === state.serverSettings.storage_path)) {
    state.serverSettings.active_library_id = state.serverSettings.libraries[0]?.id || '';
    state.serverSettings.storage_path = state.serverSettings.libraries[0]?.path || '';
  }
  return state.serverSettings.libraries;
}

let draggedLibraryRow = null;
let libraryDragChanged = false;
function commitLibraryRowOrderChange() {
  reindexLibrarySettingsRows();
  if (canManageLibrariesUI) syncActiveLibrarySelect();
  applyLibraryBranding();
  setSettingsDirty();
}
function buildLibraryLogoPreview(row, library = {}) {
  const preview = $('.settings-library-logo-preview', row);
  const file = row._pendingLogoFile || null;
  const displayName = $('.settings-library-name', row) ? $('.settings-library-name', row).value.trim() : (library.name || '');
  const displayPath = $('.settings-library-path', row) ? $('.settings-library-path', row).value.trim() : (library.path || '');
  const displayLibrary = {
    index: Number.isInteger(Number(row.dataset.libraryIndex)) ? Number(row.dataset.libraryIndex) : (Number.isInteger(Number(library.index)) ? Number(library.index) : -1),
    id: row.dataset.libraryId || library.id || '',
    name: displayName || library.name || '',
    path: displayPath || library.path || '',
    logo_asset: row.dataset.logoAsset || library.logo_asset || '',
    logo_image_url: library.logo_image_url || '',
    accent_color: $('.settings-library-accent', row) ? $('.settings-library-accent', row).value : (library.accent_color || ''),
  };
  const logoURL = row.dataset.logoPreviewUrl || resolveLibraryLogoURL(displayLibrary);
  const accent = resolveLibraryAccentColor(displayLibrary);
  if (preview) {
    preview.classList.toggle('has-image', !!logoURL);
    preview.innerHTML = logoURL
      ? `<img src="${logoURL}" alt="${escapeHTML(displayName || settingsText('libraries.logoAlt'))}">`
      : `<span class="library-avatar-placeholder">${icons.photo}</span>`;
    preview.style.borderColor = accent;
    preview.style.boxShadow = 'none';
    applyLibraryPreviewFallback(preview, displayLibrary);
  }
}

function createLibrarySettingsRow(library = {}, index = 0) {
  return createLibraryRowElement(library, index);
}

function libraryDraftFromRow(row) {
  if (!row) return {};
  return {
    id: row.dataset.libraryId || '',
    name: $('.settings-library-name', row)?.value.trim() || '',
    path: $('.settings-library-path', row)?.value.trim() || '',
    logo_asset: row.dataset.logoAsset || '',
    logo_image_url: row.dataset.logoPreviewUrl || '',
    accent_color: normalizeHexColor($('.settings-library-accent', row)?.value) || defaultLibraryAccentPalette[0],
    available: row.dataset.libraryAvailable !== 'false',
    unavailable_reason: row.dataset.libraryUnavailableReason || '',
  };
}

function createLibraryRowElement(library = {}, index = 0) {
  const wrap = el('div');
  wrap.innerHTML = renderLibrarySettingsRow(library, index).trim();
  const row = wrap.firstElementChild;
  buildLibraryLogoPreview(row, library);
  return row;
}

async function switchLibraryRowImmediately(row) {
  if (!row) return;
  if (row.dataset.libraryAvailable === 'false') {
    return;
  }
  const nextID = String(row.dataset.libraryId || '').trim();
  const nextPath = $('.settings-library-path', row)?.value.trim() || '';
  if (!nextPath) {
    showToast(settingsText('libraries.pathRequired'));
    return;
  }
  const currentID = String(state.serverSettings.active_library_id || '').trim();
  const currentPath = String(state.serverSettings.storage_path || '').trim();
  if ((nextID && nextID === currentID) || (!nextID && nextPath === currentPath) || nextPath === currentPath) {
    showToast(settingsText('libraries.alreadyCurrent'));
    return;
  }
  const previousID = state.serverSettings.active_library_id || '';
  const previousPath = state.serverSettings.storage_path || '';
  if (typeof stopEchoFrame === 'function') stopEchoFrame();
  state.serverSettings.active_library_id = nextID;
  state.serverSettings.storage_path = nextPath;
  syncActiveLibrarySelect();
  applyLibraryBranding();
  setSettingsDirty();
  try {
    await completeSettingsSave(null, { successMessage: settingsText('libraries.switchSuccess') });
  } catch (e) {
    state.serverSettings.active_library_id = previousID;
    state.serverSettings.storage_path = previousPath;
    syncActiveLibrarySelect();
    applyLibraryBranding();
    setSettingsDirty(true);
    alert(copyText('app.interface.s_552ea7bc12') + ((e && e.error) || e.message || e));
  }
}

async function deleteLibraryRowFromContextMenu(row) {
  if (!row) return;
  const library = libraryDraftFromRow(row);
  const originalLibraryID = String(row.dataset.libraryId || '').trim();
  const originalLibraryPath = $('.settings-library-path', row)?.value.trim() || '';
  if (!(await appConfirm(settingsText('libraries.editor.deleteConfirm', { name: library.name || settingsText('libraries.editor.unnamed') }), { title: settingsText('libraries.editor.deleteTitle'),
    confirmText: settingsText('libraries.editor.delete'),
    cancelText: settingsText('libraries.editor.cancel'),
    danger: true,
  }))) return;
  row.remove();
  if (!$('#settings-library-list .settings-library-row')) {
    $('#settings-library-list')?.insertAdjacentHTML('beforeend', `<div class="settings-empty">${escapeHTML(settingsText('libraries.empty'))}</div>`);
  }
  reindexLibrarySettingsRows();
  const remainingLibraries = collectLibraryDrafts();
  const currentActiveID = String(state.serverSettings.active_library_id || '').trim();
  const currentActivePath = String(state.serverSettings.storage_path || '').trim();
  if ((originalLibraryID && originalLibraryID === currentActiveID) || (originalLibraryPath && originalLibraryPath === currentActivePath)) {
    state.serverSettings.active_library_id = remainingLibraries[0]?.id || '';
    state.serverSettings.storage_path = remainingLibraries[0]?.path || '';
  }
  syncActiveLibrarySelect();
  applyLibraryBranding();
  setSettingsDirty();
  try {
    await completeSettingsSave(null, { successMessage: settingsText('libraries.editor.deleteSuccess') });
  } catch (e) {
    alert(copyText('app.interface.s_7f94f1dce6') + ((e && e.error) || e.message || e));
  }
}

async function switchLibraryRowFromContextMenu(row) {
  if (!row) return;
  const libraryName = String(row.dataset.libraryName || $('.settings-library-name', row)?.value || settingsText('libraries.thisLibrary')).trim() || settingsText('libraries.thisLibrary');
  const confirmed = await appConfirm(settingsText('libraries.switchConfirm', { name: libraryName }), {
    title: settingsText('libraries.switchTitle'),
    confirmText: settingsText('libraries.editor.switch'),
    cancelText: settingsText('libraries.editor.cancel'),
  });
  if (!confirmed) return;
  await switchLibraryRowImmediately(row);
}

function normalizedLibraryPathForComparison(path) {
  return String(path || '').trim().replace(/[\\/]+$/, '').toLowerCase();
}

function openLibraryBatchEditorModal(bindRow = null) {
  const modal = el('div', 'modal-overlay library-editor-modal open');
  modal.innerHTML = `
    <div class="modal library-editor-card">
      <div class="modal-title">${escapeHTML(settingsText('libraries.editor.batchTitle'))}</div>
      <div class="settings-control">
        <label for="library-batch-editor-path"><span>${escapeHTML(settingsText('libraries.editor.batchPathLabel'))}</span></label>
        <input class="input" id="library-batch-editor-path" type="text" value="" placeholder="${escapeHTML(settingsText('libraries.editor.batchPathPlaceholder'))}">
      </div>
      <div class="settings-control">
        <label><span>${escapeHTML(settingsText('libraries.editor.batchPending'))}</span><span id="library-batch-editor-count">0</span></label>
        <div class="settings-batch-build-selection-list" id="library-batch-editor-list">
          <div class="settings-batch-build-empty">${escapeHTML(settingsText('libraries.editor.batchEmpty'))}</div>
        </div>
      </div>
      <div class="modal-footer library-editor-actions">
        ${libraryEditorActionButton({ id: 'library-batch-editor-select-all', kind: 'switch', icon: icons.libraryBatchSelectAll, label: copyText('app.interface.s_3e44b2a933'), disabled: true })}
        ${libraryEditorActionButton({ id: 'library-batch-editor-explore', kind: 'switch', icon: icons.libraryBatchExplore, label: copyText('app.interface.s_2918633ac8') })}
        ${libraryEditorActionButton({ id: 'library-batch-editor-create', kind: 'save', icon: icons.libraryBatchCreate, label: copyText('app.interface.commonCreate'), disabled: true })}
        ${libraryEditorActionButton({ id: 'library-batch-editor-cancel', kind: 'cancel', icon: icons.libraryBatchCancel, label: copyText('app.interface.commonCancel') })}
      </div>
    </div>`;
  document.body.appendChild(modal);

  const pathInput = $('#library-batch-editor-path', modal);
  const list = $('#library-batch-editor-list', modal);
  const count = $('#library-batch-editor-count', modal);
  const selectAllButton = $('#library-batch-editor-select-all', modal);
  const exploreButton = $('#library-batch-editor-explore', modal);
  const createButton = $('#library-batch-editor-create', modal);
  const pending = [];
  const selected = new Set();
  const existingPaths = new Set(collectLibraryDrafts().map(library => normalizedLibraryPathForComparison(library.path)));

  const renderPending = () => {
    count.textContent = pending.length ? `${selected.size}/${pending.length}` : '0';
    selectAllButton.disabled = pending.length === 0;
    createButton.disabled = selected.size === 0;
    list.innerHTML = pending.length
      ? pending.map((item, index) => {
        const key = normalizedLibraryPathForComparison(item.path);
        const checked = selected.has(key);
        return `<label class="settings-batch-build-select-row ${checked ? 'is-selected' : ''}">
          <span class="settings-toggle-switch settings-batch-build-checkbox">
            <input type="checkbox" data-library-batch-index="${index}" ${checked ? 'checked' : ''}>
            <span class="settings-toggle-slider" aria-hidden="true"></span>
          </span>
          <span class="settings-batch-build-select-copy"><strong>${escapeHTML(item.name)}</strong><span>${escapeHTML(item.path)}</span></span>
        </label>`;
      }).join('')
      : `<div class="settings-batch-build-empty">${escapeHTML(settingsText('libraries.editor.batchEmpty'))}</div>`;
  };
  const close = () => modal.remove();

  list.addEventListener('change', e => {
    const input = e.target.closest('input[data-library-batch-index]');
    if (!input) return;
    const item = pending[Number(input.dataset.libraryBatchIndex)];
    if (!item) return;
    const key = normalizedLibraryPathForComparison(item.path);
    if (input.checked) selected.add(key);
    else selected.delete(key);
    renderPending();
  });

  selectAllButton.addEventListener('click', () => {
    const allSelected = pending.length > 0 && selected.size === pending.length;
    selected.clear();
    if (!allSelected) pending.forEach(item => selected.add(normalizedLibraryPathForComparison(item.path)));
    renderPending();
  });

  exploreButton.addEventListener('click', async () => {
    const parent = pathInput.value.trim();
    if (!parent) {
      showToast(settingsText('libraries.editor.batchPathLabel'));
      return;
    }
    exploreButton.disabled = true;
    exploreButton.querySelector('.library-editor-action-label').textContent = settingsText('libraries.editor.batchExploring');
    try {
      const response = await discoverLibraryDirectories(parent);
      pending.length = 0;
      selected.clear();
      for (const directory of Array.isArray(response.directories) ? response.directories : []) {
        const path = String(directory.path || '').trim();
        const name = String(directory.name || '').trim();
        const key = normalizedLibraryPathForComparison(path);
        if (!path || !name || existingPaths.has(key) || pending.some(item => normalizedLibraryPathForComparison(item.path) === key)) continue;
        pending.push({ name, path });
        selected.add(key);
      }
      renderPending();
      if (!pending.length) showToast(settingsText('libraries.editor.batchNoNew'));
    } catch (e) {
      appAlert((e && e.error) || String(e), { title: settingsText('libraries.editor.batchDiscoverFailed') });
    } finally {
      exploreButton.disabled = false;
      exploreButton.querySelector('.library-editor-action-label').textContent = copyText('app.interface.s_2918633ac8');
    }
  });

  createButton.addEventListener('click', async () => {
    if (!pending.length) return;
    const currentLibraries = collectLibraryDrafts();
    const additions = pending.filter(item => selected.has(normalizedLibraryPathForComparison(item.path))).map((item, index) => ({
      id: '',
      name: item.name || settingsText('libraries.nameFallback', { index: currentLibraries.length + index + 1 }),
      path: item.path,
      logo_asset: '',
      logo_image_url: '',
      accent_color: defaultLibraryAccentPalette[(currentLibraries.length + index) % defaultLibraryAccentPalette.length],
    }));
    for (const library of additions) {
      const row = createLibraryRowElement(library, currentLibraries.length);
      $('#settings-library-list')?.appendChild(row);
      if (typeof bindRow === 'function') bindRow(row);
      currentLibraries.push(library);
    }
    $('#settings-library-list .settings-empty')?.remove();
    reindexLibrarySettingsRows();
    setSettingsDirty();
    try {
      await completeSettingsSave(createButton, { successMessage: settingsText('libraries.editor.batchCreateSuccess', { count: additions.length }) });
      close();
    } catch (e) {
      appAlert((e && e.error) || String(e), { title: settingsText('libraries.editor.createTitle') });
    }
  });
  $('#library-batch-editor-cancel', modal).addEventListener('click', close);
  modal.addEventListener('click', e => {
    if (e.target === modal) close();
  });
  renderPending();
}

function updateBatchLibraryAddButton() {
  const button = $('#settings-add-library-btn');
  if (!button) return;
  const batch = !!state.libraryBatchAddModifierHeld;
  const label = settingsText(batch ? 'sections.libraries.batchAdd' : 'sections.libraries.add');
  button.innerHTML = `${icons.libraryCreate || ''} ${escapeHTML(label)}`;
  button.title = label;
  button.setAttribute('aria-label', label);
}

function bindBatchLibraryAddShortcut() {
  if (state.libraryBatchAddShortcutBound) return;
  state.libraryBatchAddShortcutBound = true;
  const update = held => {
    state.libraryBatchAddModifierHeld = !!held;
    updateBatchLibraryAddButton();
  };
  document.addEventListener('keydown', e => {
    if (e.key === 'Alt' || e.code === 'AltLeft' || e.code === 'AltRight') update(true);
  });
  document.addEventListener('keyup', e => {
    if (e.key === 'Alt' || e.code === 'AltLeft' || e.code === 'AltRight') update(false);
  });
  window.addEventListener('blur', () => update(false));
}

function showLibraryRowContextMenu(x, y, row, options = {}) {
  if (!row) return;
  const unavailable = row.dataset.libraryAvailable === 'false';
  showContextMenu(x, y, compactContextMenuItems([
    {
      role: 'switch',
      icon: icons.libraryEditorSwitch,
      label: settingsText('libraries.editor.switch'),
      disabled: unavailable,
      action: () => { void switchLibraryRowFromContextMenu(row); },
    },
    {
      role: 'delete',
      icon: icons.libraryEditorDelete,
      label: settingsText('libraries.editor.delete'),
      danger: true,
      action: () => { void deleteLibraryRowFromContextMenu(row); },
    },
  ]), options);
}

function libraryEditorActionButton({ id, kind = 'default', icon = '', label = '', hidden = false, disabled = false } = {}) {
  return `<button class="btn library-editor-action-btn library-editor-action-btn-${escapeHTML(kind)}" id="${escapeHTML(id)}" type="button"${hidden ? ' hidden' : ''}${disabled ? ' disabled' : ''}>
    <span class="library-editor-action-icon">${icon || ''}</span>
    <span class="library-editor-action-label">${escapeHTML(label)}</span>
  </button>`;
}

function openLibraryEditorModal(row = null, bindRow = null) {
  const editing = !!row;
  const index = editing ? Number(row.dataset.libraryIndex) || 0 : $$('.settings-library-row').length;
  const originalLibraryID = editing ? String(row.dataset.libraryId || '').trim() : '';
  const originalLibraryPath = editing ? ($('.settings-library-path', row)?.value.trim() || '') : '';
  const draft = editing
    ? libraryDraftFromRow(row)
    : {
      id: '',
      name: settingsText('libraries.nameFallback', { index: index + 1 }),
      path: '',
      logo_asset: '',
      logo_image_url: '',
      accent_color: defaultLibraryAccentPalette[index % defaultLibraryAccentPalette.length],
    };
  const modal = el('div', 'modal-overlay library-editor-modal open');
  modal.innerHTML = `
    <div class="modal library-editor-card">
      <div class="modal-title">${editing ? escapeHTML(settingsText('libraries.editor.editTitle')) : escapeHTML(settingsText('libraries.editor.createTitle'))}</div>
      <div class="library-editor-preview">
        <div class="settings-library-logo-preview library-editor-logo-preview${draft.logo_image_url || draft.logo_asset ? ' has-image' : ''}" id="library-editor-logo-trigger" role="button" tabindex="0" aria-label="${escapeHTML(settingsText('libraries.editor.chooseLogo'))}"></div>
        <div class="library-editor-preview-copy">${escapeHTML(settingsText('libraries.editor.previewCopy'))}</div>
      </div>
      <div class="settings-control">
        <label for="library-editor-name"><span>${escapeHTML(settingsText('libraries.editor.nameLabel'))}</span></label>
        <input class="input" id="library-editor-name" type="text" maxlength="32" value="${escapeHTML(draft.name || '')}" placeholder="${escapeHTML(settingsText('libraries.editor.namePlaceholder'))}">
      </div>
      <div class="settings-control">
        <label for="library-editor-path"><span>${escapeHTML(settingsText('libraries.editor.pathLabel'))}</span></label>
        <input class="input" id="library-editor-path" type="text" value="${escapeHTML(draft.path || '')}" placeholder="${escapeHTML(settingsText('libraries.editor.pathPlaceholder'))}">
      </div>
      <div class="settings-control library-editor-inline">
        <label for="library-editor-accent-text"><span>${escapeHTML(settingsText('libraries.editor.accentLabel'))}</span></label>
        <div class="library-editor-accent-row">
          <input class="input library-editor-accent-text" id="library-editor-accent-text" type="text" maxlength="7" value="${escapeHTML(normalizeHexColor(draft.accent_color) || defaultLibraryAccentPalette[0])}" placeholder="${escapeHTML(settingsText('libraries.editor.accentPlaceholder'))}">
          <input class="library-editor-accent-picker" id="library-editor-accent" type="color" value="${normalizeHexColor(draft.accent_color) || defaultLibraryAccentPalette[0]}">
        </div>
      </div>
      <div class="modal-footer library-editor-actions">
        ${libraryEditorActionButton({ id: 'library-editor-save', kind: 'save', icon: icons.libraryEditorSave, label: editing ? settingsText('libraries.editor.save') : settingsText('libraries.editor.create') })}
        ${libraryEditorActionButton({ id: 'library-editor-switch', kind: 'switch', icon: icons.libraryEditorSwitch, label: settingsText('libraries.editor.switch'), hidden: !editing })}
        ${libraryEditorActionButton({ id: 'library-editor-delete', kind: 'delete', icon: icons.libraryEditorDelete, label: settingsText('libraries.editor.delete'), hidden: !editing })}
        ${libraryEditorActionButton({ id: 'library-editor-cancel', kind: 'cancel', icon: icons.libraryEditorCancel, label: settingsText('libraries.editor.cancel') })}
      </div>
    </div>`;
  document.body.appendChild(modal);

  const preview = $('.library-editor-logo-preview', modal);
  const nameInput = $('#library-editor-name', modal);
  const pathInput = $('#library-editor-path', modal);
  const accentTextInput = $('#library-editor-accent-text', modal);
  const accentInput = $('#library-editor-accent', modal);
  const switchButton = $('#library-editor-switch', modal);
  const logoTrigger = $('#library-editor-logo-trigger', modal);
  const logoInput = el('input');
  logoInput.type = 'file';
  logoInput.accept = '.png,.jpg,.jpeg,.gif,.webp,image/png,image/jpeg,image/gif,image/webp';
  logoInput.hidden = true;
  modal.appendChild(logoInput);
  let pendingFile = editing ? row._pendingLogoFile || null : null;
  let pendingLogoNeedsCrop = editing ? !!row._pendingLogoNeedsCrop : false;
  let logoAsset = draft.logo_asset || '';
  let logoPreviewUrl = draft.logo_image_url || '';
  let logoAction = editing ? row.dataset.logoAction || '' : '';
  let portableLocked = false;
  let inspectionGeneration = 0;
  let inspectionTimer;
  const inspectPortable = async () => {
    if (editing) return;
    const path = pathInput.value.trim();
    const generation = ++inspectionGeneration;
    const save = $('#library-editor-save', modal);
    save.disabled = true;
    try {
      const result = path ? await api.post('/api/settings/libraries/inspect', { path }) : { existing: false };
      if (generation !== inspectionGeneration || path !== pathInput.value.trim() || !modal.isConnected) return;
      const previouslyLocked = portableLocked;
      portableLocked = !!result.existing;
      if (portableLocked) {
        const library = result.library;
        draft.id = library.id;
        nameInput.value = library.name || nameInput.value;
        accentTextInput.value = library.accent_color || draft.accent_color;
        logoAsset = library.logo_asset || '';
        logoPreviewUrl = result.logo_image_url || '';
        pendingFile = null;
        logoAction = '';
      } else if (previouslyLocked) {
        draft.id = '';
        nameInput.value = draft.name;
        accentTextInput.value = draft.accent_color;
        logoAsset = '';
        logoPreviewUrl = '';
      }
      nameInput.disabled = portableLocked;
      accentInput.disabled = portableLocked;
      accentTextInput.disabled = portableLocked;
      logoTrigger.setAttribute('aria-disabled', String(portableLocked));
      $('.library-editor-preview-copy', modal).textContent = portableLocked
        ? copyText('app.interface.s_df4c1396aa')
        : settingsText('libraries.editor.previewCopy');
      updatePreview();
    } catch (err) {
      if (generation === inspectionGeneration) showToast(copyText('app.interface.s_25495081e0') + (err.error || err.message || err));
      throw err;
    } finally {
      if (generation === inspectionGeneration) save.disabled = false;
    }
  };

  const currentEditorAccent = () => normalizeHexColor(accentTextInput.value) || normalizeHexColor(accentInput.value) || defaultLibraryAccentPalette[0];
  const updateActionStyles = () => {
    if (!switchButton) return;
    const accentValue = currentEditorAccent();
    switchButton.style.background = accentValue;
    switchButton.style.borderColor = accentValue;
    switchButton.style.color = readableTextColorForBackground(accentValue);
  };
  const updatePreview = ({ syncAccentText = false } = {}) => {
    const accentValue = currentEditorAccent();
    const library = {
      index,
      id: draft.id || '',
      name: nameInput.value.trim(),
      path: pathInput.value.trim(),
      logo_asset: logoAsset,
      logo_image_url: logoPreviewUrl,
      accent_color: accentValue,
    };
    const logoURL = logoPreviewUrl || resolveLibraryLogoURL(library);
    preview.classList.toggle('has-image', !!logoURL);
    preview.innerHTML = logoURL ? `<img src="${logoURL}" alt="${escapeHTML(library.name || settingsText('libraries.avatarAlt'))}">` : `<span class="library-avatar-placeholder">${icons.photo}</span>`;
    preview.style.borderColor = accentValue;
    preview.style.boxShadow = 'none';
    accentInput.value = accentValue;
    if (syncAccentText) accentTextInput.value = accentValue;
    updateActionStyles();
    applyLibraryPreviewFallback(preview, library);
  };

  const close = () => {
    modal.remove();
  };

  const buildLibraryDraft = () => ({
    id: draft.id || '',
    name: nameInput.value.trim() || settingsText('libraries.nameFallback', { index: index + 1 }),
    path: pathInput.value.trim(),
    logo_asset: logoAction === 'remove' ? '' : logoAsset,
    logo_image_url: logoPreviewUrl,
    accent_color: normalizeHexColor(accentTextInput.value || accentInput.value) || defaultLibraryAccentPalette[index % defaultLibraryAccentPalette.length],
  });

  const applyLibraryDraftToRow = ({ activate = false } = {}) => {
    const library = buildLibraryDraft();
    if (!library.path) {
      showToast(settingsText('libraries.pathRequired'));
      return null;
    }
    const nextRow = createLibraryRowElement(library, index);
    nextRow.dataset.logoAction = logoAction;
    nextRow._pendingLogoFile = pendingFile;
    nextRow._pendingLogoNeedsCrop = pendingLogoNeedsCrop;
    if (editing) row.replaceWith(nextRow);
    else {
      $('#settings-library-list .settings-empty')?.remove();
      $('#settings-library-list').appendChild(nextRow);
    }
    if (typeof bindRow === 'function') bindRow(nextRow);
    reindexLibrarySettingsRows();
    if (activate) {
      state.serverSettings.active_library_id = library.id || '';
      state.serverSettings.storage_path = library.path;
    } else if (editing) {
      const currentActiveID = String(state.serverSettings.active_library_id || '').trim();
      const currentActivePath = String(state.serverSettings.storage_path || '').trim();
      if ((originalLibraryID && originalLibraryID === currentActiveID) || (originalLibraryPath && originalLibraryPath === currentActivePath)) {
        state.serverSettings.active_library_id = library.id || '';
        state.serverSettings.storage_path = library.path;
      }
    }
    syncActiveLibrarySelect();
    applyLibraryBranding();
    setSettingsDirty();
    return nextRow;
  };

  const persistLibraryDraft = async (button, { activate = false, successMessage = '' } = {}) => {
    if (!editing) {
      try { await inspectPortable(); } catch (_) { return; }
    }
    const nextRow = applyLibraryDraftToRow({ activate });
    if (!nextRow) return;
    try {
      await completeSettingsSave(button, { successMessage });
      close();
    } catch (e) {
      alert(copyText('app.interface.s_90f327cca4') + ((e && e.error) || e.message || e));
    }
  };

  nameInput.addEventListener('input', updatePreview);
  pathInput.addEventListener('input', updatePreview);
  pathInput.addEventListener('input', () => {
    if (editing) return;
    inspectionGeneration++;
    clearTimeout(inspectionTimer);
    inspectionTimer = setTimeout(() => inspectPortable().catch(() => {}), 450);
  });
  accentTextInput.addEventListener('input', () => {
    const color = normalizeHexColor(accentTextInput.value);
    if (color) accentInput.value = color;
    updatePreview();
  });
  accentInput.addEventListener('input', () => {
    accentTextInput.value = normalizeHexColor(accentInput.value) || accentInput.value;
    updatePreview();
  });
  const openLogoPicker = () => { if (!portableLocked) logoInput.click(); };
  logoTrigger?.addEventListener('click', openLogoPicker);
  logoTrigger?.addEventListener('keydown', e => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      openLogoPicker();
    }
  });
  logoInput.addEventListener('change', e => {
    const file = e.target.files && e.target.files[0];
    if (!file) return;
    openLibraryLogoGuideModal(file, croppedFile => {
      if (logoPreviewUrl && logoPreviewUrl.startsWith('blob:')) URL.revokeObjectURL(logoPreviewUrl);
      pendingFile = croppedFile;
      pendingLogoNeedsCrop = false;
      logoAction = 'upload';
      logoPreviewUrl = URL.createObjectURL(croppedFile);
      updatePreview();
    }, () => {
      logoInput.value = '';
    });
  });
  $('#library-editor-cancel', modal).addEventListener('click', close);
  modal.addEventListener('click', e => {
    if (e.target === modal) close();
  });
  $('#library-editor-save', modal).addEventListener('click', async () => {
    await persistLibraryDraft($('#library-editor-save', modal), {
      activate: false,
      successMessage: editing ? settingsText('libraries.editor.saveSuccess') : settingsText('libraries.editor.createSuccess'),
    });
  });
  $('#library-editor-switch', modal)?.addEventListener('click', async () => {
    await persistLibraryDraft($('#library-editor-switch', modal), {
      activate: true,
      successMessage: settingsText('libraries.switchSuccess'),
    });
  });
  $('#library-editor-delete', modal)?.addEventListener('click', async () => {
    if (!editing) return;
    const library = buildLibraryDraft();
    if (!(await appConfirm(settingsText('libraries.editor.deleteConfirm', { name: library.name || settingsText('libraries.editor.unnamed') }), { title: settingsText('libraries.editor.deleteTitle'),
      confirmText: settingsText('libraries.editor.delete'),
      cancelText: settingsText('libraries.editor.cancel'),
      danger: true,
    }))) return;
    row.remove();
    if (!$('#settings-library-list .settings-library-row')) {
      $('#settings-library-list')?.insertAdjacentHTML('beforeend', `<div class="settings-empty">${escapeHTML(settingsText('libraries.empty'))}</div>`);
    }
    reindexLibrarySettingsRows();
    const remainingLibraries = collectLibraryDrafts();
    const currentActiveID = String(state.serverSettings.active_library_id || '').trim();
    const currentActivePath = String(state.serverSettings.storage_path || '').trim();
    if ((originalLibraryID && originalLibraryID === currentActiveID) || (originalLibraryPath && originalLibraryPath === currentActivePath)) {
      state.serverSettings.active_library_id = remainingLibraries[0]?.id || '';
      state.serverSettings.storage_path = remainingLibraries[0]?.path || '';
    }
    syncActiveLibrarySelect();
    applyLibraryBranding();
    setSettingsDirty();
    try {
      await completeSettingsSave($('#library-editor-delete', modal), { successMessage: settingsText('libraries.editor.deleteSuccess') });
      close();
    } catch (e) {
      alert(copyText('app.interface.s_7f94f1dce6') + ((e && e.error) || e.message || e));
    }
  });
  updatePreview({ syncAccentText: true });
}

function syncActiveLibrarySelect() {
  const select = $('#settings-active-library');
  const libraries = collectLibraryDrafts();
  if (!select && !libraries.length) return;
  const previousIndex = select ? select.selectedIndex : -1;
  const current = ((select && select.value) || state.serverSettings.active_library_id || state.serverSettings.storage_path || '').trim();
  let activeLibrary = resolveActiveLibrary(libraries, current, state.serverSettings.storage_path || '');
  if (!activeLibrary && previousIndex >= 0 && libraries[previousIndex]) {
    activeLibrary = libraries[previousIndex];
  }
  if (!activeLibrary && libraries.length) activeLibrary = libraries[0];
  const activePath = activeLibrary && activeLibrary.path || '';
  state.serverSettings.active_library_id = activeLibrary && activeLibrary.id || '';
  state.serverSettings.storage_path = activePath || '';
  state.serverSettings.libraries = normalizeLibraryList(libraries, state.serverSettings.storage_path || '');
  if (select) {
    select.innerHTML = libraries.map((library, index) => {
      const selected = (activeLibrary && ((library.id && library.id === activeLibrary.id) || library.path === activeLibrary.path)) || (!activePath && index === 0);
      return `<option value="${escapeHTML(library.id || library.path)}" ${selected ? 'selected' : ''} ${library.available === false ? 'disabled' : ''}>${library.name}</option>`;
    }).join('') || `<option value="">${escapeHTML(settingsText('libraries.emptyOption'))}</option>`;
  }
  $$('.settings-library-row').forEach(row => {
    const id = row.dataset.libraryId || '';
    const path = $('.settings-library-path', row)?.value.trim() || '';
    row.classList.toggle('active', (!!id && id === (activeLibrary && activeLibrary.id || '')) || (!!path && path === activePath));
  });
  const floatingAdd = $('#settings-add-library-btn');
  if (floatingAdd && activeLibrary) {
    const accent = resolveLibraryAccentColor(activeLibrary);
    floatingAdd.style.setProperty('--highlight-bg', accent);
    floatingAdd.style.setProperty('--highlight-text', readableTextColorForBackground(accent));
  }
}

function reindexLibrarySettingsRows() {
  $$('.settings-library-row').forEach((row, index) => {
    row.dataset.libraryIndex = String(index);
  });
}

function setSettingsDirty(dirty = true) {
  state.settingsDirty = !!dirty;
  const saveBtn = $('#settings-save-top-btn');
  if (saveBtn) saveBtn.disabled = !state.settingsDirty;
}

async function refreshMissingLibraryLogosOnce() {
  if (state.libraryLogosRefreshStarted || state.settingsDirty) return;
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  if (!libraries.some(library => !library.logo_asset && !library.logo_image_url && !library.logoImageUrl)) return;
  state.libraryLogosRefreshStarted = true;
  try {
    const resp = await refreshLibraryLogos(false);
    const data = resp.result || {};
    if (data.updated > 0) showToast(settingsText('libraries.logoRefreshUpdated', { count: data.updated }));
    if (state.view === 'settings' && !state.settingsDirty) renderSettingsContent({ host: settingsViewHost() });
  } catch (e) {
    console.warn("批量更新资源库头像失败", e);
  }
}

async function persistSettingsWithLibraryAssets() {
  collectSettingsFormState();
  const rows = $$('.settings-library-row');
  const libraryOps = rows
    .map(row => ({
      path: $('.settings-library-path', row).value.trim(),
      action: row.dataset.logoAction || '',
      file: row._pendingLogoFile || null,
    }))
    .filter(operation => operation.path && (operation.action === 'remove' || operation.file));

  const resp = await persistSettings();
  let latestMessage = (resp && resp.message) || copyText('app.interface.settingsSaved');
  const requiresRestart = !!(resp && resp.requires_restart);

  for (const operation of libraryOps) {
    const targetIndex = state.serverSettings.libraries.findIndex(library => (library.path || '').trim() === operation.path);
    if (targetIndex < 0) continue;
    if (operation.action === 'remove') {
      const removeResp = await deleteLibraryLogo(targetIndex);
      latestMessage = (removeResp && removeResp.message) || latestMessage;
      continue;
    }
    if (operation.file) {
      const uploadResp = await uploadLibraryLogo(targetIndex, operation.file);
      latestMessage = (uploadResp && uploadResp.message) || latestMessage;
    }
  }

  return { message: latestMessage, data: state.serverSettings, requires_restart: requiresRestart };
}

async function completeSettingsSave(button, { savingLabel = copyText('app.interface.commonSaving'), restartingLabel = copyText('app.interface.s_ce2d2904e8'), successMessage = '' } = {}) {
  const previousLibraryIdentity = activeLibraryIdentity(state.savedServerSettings || state.serverSettings);
  const runSave = () => persistSettingsWithLibraryAssets();
  const resp = button ? await withButtonBusy(button, renderButtonBusySpinner(), runSave, { ariaLabel: savingLabel }) : await runSave();
  const activeLibraryChanged = previousLibraryIdentity !== activeLibraryIdentity(state.serverSettings);
  setSettingsDirty(false);
  if (resp && resp.requires_restart) {
    showToast(successMessage || (resp && resp.message) || copyText('app.interface.settingsSaved'), 2200);
    if (button) await withButtonBusy(button, renderButtonBusySpinner(), () => restartApp(), { ariaLabel: restartingLabel, keepBusyState: true });
    else await restartApp();
    setTimeout(() => location.reload(), 1800);
    return resp;
  }
  if (activeLibraryChanged) {
    location.reload();
    return resp;
  }
  showToast(successMessage || (resp && resp.message) || copyText('app.interface.settingsSaved'));
  renderSettings();
  return resp;
}

function bindSettingsTopSaveButton() {
  const saveBtn = $('#settings-save-top-btn');
    if (saveBtn) {
      saveBtn.disabled = !state.settingsDirty;
      saveBtn.onclick = async () => {
        try {
          await completeSettingsSave(saveBtn);
        } catch (e) {
          alert(copyText('app.interface.s_7b1be8c492') + ((e && e.error) || e.message || e));
        }
      };
    }
}

function collectSettingsFormState() {
  if (canManageLibraries()) {
    state.serverSettings.libraries = collectLibraryInputs();
    if (!state.serverSettings.libraries.length) throw new Error(settingsText('libraries.keepOne'));
    state.serverSettings.port = parseInt($('#settings-port')?.value, 10) || state.serverSettings.port || 8080;
    const activeValue = $('#settings-active-library')?.value.trim() || state.serverSettings.active_library_id || state.serverSettings.storage_path || '';
    const activeLibrary = resolveActiveLibrary(state.serverSettings.libraries, activeValue, state.serverSettings.storage_path || '');
    state.serverSettings.active_library_id = activeLibrary && activeLibrary.id || '';
    state.serverSettings.storage_path = activeLibrary && activeLibrary.path || state.serverSettings.storage_path || state.serverSettings.libraries[0].path;
  }
  state.serverSettings.thumbnail_dir = $('#settings-thumbnail-dir')?.value.trim() || state.serverSettings.thumbnail_dir || '';
  state.serverSettings.thumbnail_size = 512;
  state.serverSettings.trash_dir = $('#settings-trash-dir')?.value.trim() || state.serverSettings.trash_dir || '';
  state.serverSettings.use_system_player = !!state.serverSettings.use_system_player;
  state.videoSectionMinMinutes = Math.min(240, Math.max(1, Number($('#settings-video-section-min')?.value) || state.videoSectionMinMinutes || 10));
  state.videoAutoplayNext = !!$('#settings-video-autoplay-next')?.checked;
  updatePlayerKeymapSource($('#settings-player-keymap').value);
  return state.serverSettings;
}

function renderSettingsShareRows() {
  if (!state.shareLinks.length) {
    return '';
  }
  return state.shareLinks.map(link => {
    const typeLabel = link.type === 'album' ? settingsText('shares.typeAlbum') : settingsText('shares.typeMedia');
    const knownPhoto = link.type === 'photo' ? findKnownPhotoByID(link.target_id) : null;
    const title = String((knownPhoto && knownPhoto.original_name) || `${typeLabel} #${link.target_id}`).trim();
    const url = `${location.origin}/s/${link.token}`;
    const previewImage = knownPhoto ? mediaThumbURL(knownPhoto) : '';
    const detail = knownPhoto
      ? formatSize(Number(knownPhoto.size) || 0)
      : (link.type === 'album' ? settingsText('shares.typeAlbum') : settingsText('shares.typeMedia'));
    const previewMarkup = previewImage
      ? `<img loading="lazy" src="${previewImage}" alt="${escapeHTML(title)}">`
      : `<span class="settings-share-preview-icon" aria-hidden="true">${link.type === 'album' ? (icons.album || icons.photo || '') : (icons.share || icons.photo || '')}</span>`;
    return `
      <div class="settings-share-row" data-share-id="${link.id}">
        <div class="settings-share-preview${previewImage ? ' has-image' : ''}">
          ${previewMarkup}
        </div>
        <div class="settings-share-main">
          <div class="settings-share-title">${escapeHTML(title)}</div>
          <div class="settings-share-meta">${escapeHTML(detail)}</div>
        </div>
        <div class="settings-share-actions">
          <button class="btn settings-share-action" type="button" data-copy-share="${link.id}" aria-label="${escapeHTML(settingsText('shares.copy'))}" title="${escapeHTML(settingsText('shares.copy'))}">${icons.shareCardCopy || icons.contextShare || icons.share || ''}</button>
          <a class="btn settings-share-action" href="${url}" target="_blank" rel="noopener noreferrer" aria-label="${escapeHTML(settingsText('shares.open'))}" title="${escapeHTML(settingsText('shares.open'))}">${icons.shareCardOpen || icons.contextReveal || ''}</a>
          <button class="btn settings-share-action is-danger" type="button" data-delete-share="${link.id}" aria-label="${escapeHTML(settingsText('shares.delete'))}" title="${escapeHTML(settingsText('shares.delete'))}">${icons.shareCardDelete || icons.contextDelete || icons.trash || ''}</button>
        </div>
      </div>`;
  }).join('');
}

function renderSettingsShareEmptyState() {
  return `<div class="settings-share-empty-state">
    <span>${escapeHTML(settingsText('shares.emptyTitle'))}</span>
  </div>`;
}

function renderSettingsShareDeferredState() {
  return `<div class="settings-share-empty-state">
    <span>${escapeHTML(settingsText('shares.lazyHint'))}</span>
  </div>`;
}

function renderSettingsShareLoadingState() {
  return `<div class="settings-share-empty-state">
    <span>${escapeHTML(settingsText('shares.loading'))}</span>
  </div>`;
}

function refreshShareManagementUI() {
  const wrap = $('#settings-share-list');
  if (!wrap) return;
  if (!state.shareLinksLoaded) {
    wrap.innerHTML = state.shareLinksLoading ? renderSettingsShareLoadingState() : renderSettingsShareDeferredState();
    const actions = $('#settings-share-bulk-actions');
    if (actions) actions.hidden = true;
    return;
  }
  if (!state.shareLinks.length) {
    wrap.innerHTML = renderSettingsShareEmptyState();
    const actions = $('#settings-share-bulk-actions');
    if (actions) actions.hidden = true;
    return;
  }
  wrap.innerHTML = renderSettingsShareRows();
  const actions = $('#settings-share-bulk-actions');
  if (actions) actions.hidden = false;
  wrap.querySelectorAll('[data-copy-share]').forEach(button => {
    button.onclick = async () => {
      const share = state.shareLinks.find(item => item.id === Number(button.dataset.copyShare));
      if (!share) return;
      await navigator.clipboard.writeText(`${location.origin}/s/${share.token}`);
      showToast(settingsText('shares.copied'));
    };
  });
  wrap.querySelectorAll('[data-delete-share]').forEach(button => {
    button.onclick = async () => {
      const shareID = Number(button.dataset.deleteShare);
      if (!shareID) return;
      try {
        await withButtonBusy(button, settingsText('shares.deleting'), () => api.del(`/api/media/shares/${shareID}`));
        removeShareLinkFromState(shareID);
        showToast(settingsText('shares.deleted'));
      } catch (e) {
        alert(copyText('app.interface.s_5a5a8a09cc') + ((e && e.error) || e));
      }
    };
  });
  $('#settings-copy-all-shares-btn')?.addEventListener('click', async () => {
    if (!state.shareLinks.length) return;
    const text = state.shareLinks.map(item => `${location.origin}/s/${item.token}`).join('\n');
    await navigator.clipboard.writeText(text);
    showToast(settingsText('shares.copyAllDone'));
  });
  $('#settings-delete-all-shares-btn')?.addEventListener('click', async () => {
    if (!state.shareLinks.length) return;
    if (!(await appConfirm(settingsText('shares.deleteAllConfirm'), {
      danger: true,
      confirmText: settingsText('shares.deleteAll'),
    }))) return;
    const ids = state.shareLinks.map(item => item.id).filter(Boolean);
    for (const id of ids) {
      await api.del(`/api/media/shares/${id}`);
      removeShareLinkFromState(id);
    }
    showToast(settingsText('shares.deleteAllDone'));
  });
}

function observeDeferredSettingsShareLoad() {
  const panel = $('#settings-share-panel');
  if (!panel || state.shareLinksLoaded) return;
  const triggerLoad = () => {
    if (state.shareLinksLoaded || state.shareLinksLoading) return;
    refreshShareManagementUI();
    loadShareMap().then(() => {
      if (state.view === 'settings') refreshShareManagementUI();
    }).catch(() => {});
  };
  if (typeof IntersectionObserver !== 'function') {
    triggerLoad();
    return;
  }
  const observer = new IntersectionObserver(entries => {
    if (!entries[0] || !entries[0].isIntersecting) return;
    observer.disconnect();
    triggerLoad();
  }, { rootMargin: '120px 0px' });
  observer.observe(panel);
}

function refreshPhotoShareIndicators(type, targetId) {
  if (type !== 'photo') return;
  document.querySelectorAll(`.photo-thumb[data-id="${targetId}"]`).forEach(thumb => {
    const existing = thumb.querySelector('.share-badge');
    if (existing) existing.remove();
  });
}

function updateVideoBookmarkThumbIndicators(photoOrId = null) {
  const targetId = photoOrId && typeof photoOrId === 'object' ? photoOrId.id : photoOrId;
  const thumbSelector = targetId
    ? `.photo-thumb[data-id="${targetId}"]`
    : '.photo-thumb[data-kind="video"]';
  document.querySelectorAll(thumbSelector).forEach(thumb => {
    const photo = findKnownPhotoByID(Number(thumb.dataset.id));
    const count = getVideoBookmarkCountByKey(thumb.dataset.id) || Math.max(0, Number(photo && photo.video_bookmark_count) || 0);
    const existing = thumb.querySelector('.video-bookmark-badge');
    if (count > 0 && !existing) {
      thumb.insertAdjacentHTML('beforeend', `<span class="video-bookmark-badge" title="${escapeHTML(copyText('app.interface.d_video_bookmarks', { count }))}" aria-label="${escapeHTML(copyText('app.interface.d_video_bookmarks', { count }))}"><span class="video-bookmark-icon">${icons.bookmark}</span><span class="video-bookmark-count">${count}</span></span>`);
    } else if (count > 0 && existing) {
      existing.title = copyText('app.interface.d_c3dbd5be97', { v0: (count) });
      existing.setAttribute('aria-label', copyText('app.interface.d_c3dbd5be97', { v0: (count) }));
      const countEl = existing.querySelector('.video-bookmark-count');
      if (countEl) countEl.textContent = String(count);
    } else if (existing) {
      existing.remove();
    }
  });
}
function refreshVideoBookmarkThumbIndicators() {
  updateVideoBookmarkThumbIndicators();
}

function renderSettingsToggle(id, label, checked, description = '', disabled = false) {
  return `<label class="settings-toggle${disabled ? ' settings-toggle-disabled' : ''}" for="${id}"${disabled ? ' aria-disabled="true"' : ''}>
    <span class="settings-toggle-copy">
      <span class="settings-toggle-title">${escapeHTML(label)}</span>
      ${description ? `<span class="settings-toggle-description">${escapeHTML(description)}</span>` : ''}
    </span>
    <span class="settings-toggle-switch">
      <input id="${id}" type="checkbox" ${checked ? 'checked' : ''} ${disabled ? 'disabled' : ''}>
      <span class="settings-toggle-slider" aria-hidden="true"></span>
    </span>
  </label>`;
}

const settingsPanelHeadIconMap = {
  libraries: 'settingsPanelHeadBlank1',
  batchWorkflow: 'settingsPanelHeadBlank2',
  display: 'settingsPanelHeadBlank3',
  playback: 'settingsPanelHeadBlank4',
  echoFrame: 'settingsPanelHeadEchoFrame',
  experimental: 'settingsPanelHeadBlank5',
  pwa: 'settingsPanelHeadBlank6',
  app: 'settingsPanelHeadBlank7',
  shares: 'settingsPanelHeadBlank8',
  update: 'settingsPanelHeadUpdate',
};

function renderSettingsPanelHeader(title, copy = '', iconKey = 'settingsPanelHeadBlank1') {
  const safeTitle = escapeHTML(title || '');
  const safeCopy = String(copy || '').trim();
  const icon = icons[iconKey] || icons.settingsPanelHeadBlank1 || '';
  return `<button class="settings-panel-head settings-panel-toggle" type="button" data-settings-panel-toggle aria-expanded="false">
    <span class="settings-panel-head-icon" aria-hidden="true">${icon}</span>
    <span class="settings-panel-head-copy">
      <h3>${safeTitle}</h3>
      ${safeCopy ? `<p>${escapeHTML(safeCopy)}</p>` : ''}
    </span>
    <span class="settings-panel-toggle-icon" aria-hidden="true">${icons.back || ''}</span>
  </button>`;
}

function bindSettingsPanelToggles(host) {
  if (!host) return;
  host.querySelectorAll('[data-settings-panel-toggle]').forEach(header => {
    const panel = header.closest('.settings-panel');
    if (!panel) return;
    const title = header.querySelector('.settings-panel-head-copy h3')?.textContent?.trim() || '';
    const panelClass = Array.from(panel.classList)
      .filter(name => name !== 'is-expanded' && name !== 'is-collapsed')
      .join(' ');
    const stateKey = `${panelClass}|${title}`;
    const contentHost = header.parentElement || panel;
    const content = Array.from(contentHost.children).filter(child => child !== header);
    const setExpanded = expanded => {
      header.setAttribute('aria-expanded', expanded ? 'true' : 'false');
      panel.classList.toggle('is-expanded', expanded);
      panel.classList.toggle('is-collapsed', !expanded);
      content.forEach(child => {
        child.classList.toggle('settings-panel-collapsible-content', true);
        child.classList.toggle('is-collapsed', !expanded);
        child.setAttribute('aria-hidden', expanded ? 'false' : 'true');
      });
    };
    setExpanded(settingsPanelExpandedState.get(stateKey) === true);
    header.addEventListener('click', () => {
      const expanded = header.getAttribute('aria-expanded') !== 'true';
      settingsPanelExpandedState.set(stateKey, expanded);
      setExpanded(expanded);
    });
  });
}

function persistLibraryBatchWorkflowPrefs() {
  localStorage.setItem(libraryBatchWorkflowEnabledStorageKey, state.libraryBatchWorkflowEnabled ? '1' : '0');
  localStorage.setItem(libraryBatchWorkflowAggressiveStorageKey, state.libraryBatchWorkflowAggressive ? '1' : '0');
}

function setLibraryBatchWorkflowPhase(phase = '') {
  state.libraryBatchWorkflowPhase = phase || '';
  if (state.libraryBatchWorkflowPhase) localStorage.setItem(libraryBatchWorkflowPhaseStorageKey, state.libraryBatchWorkflowPhase);
  else localStorage.removeItem(libraryBatchWorkflowPhaseStorageKey);
}

function clearLibraryBatchWorkflowPhase() {
  state.libraryBatchWorkflowStartPending = false;
  setLibraryBatchWorkflowPhase('');
}

function normalizeLibraryBatchWorkflowRuntimeState() {
  const scanActive = isLibraryBatchBuildActive(state.libraryBatchBuildStatus);
  const thumbnailActive = isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus);
  const awaitingThumbnailStart = state.libraryBatchWorkflowPhase === 'scan'
    && state.libraryBatchWorkflowEnabled
    && String(state.libraryBatchBuildStatus && state.libraryBatchBuildStatus.status || '') === 'completed';
  if (!scanActive && !thumbnailActive) {
    state.libraryBatchBuildCancelPending = false;
    state.libraryBatchThumbnailBuildCancelPending = false;
    if (!awaitingThumbnailStart && !state.libraryBatchWorkflowStartPending && libraryBatchWorkflowActive()) {
      setLibraryBatchWorkflowPhase('');
    }
  }
}

function libraryBatchWorkflowActive() {
  return state.libraryBatchWorkflowPhase === 'scan' || state.libraryBatchWorkflowPhase === 'thumbnails';
}

async function syncLibraryBatchWorkflowSelections() {
  const selectedLibraryIDs = Array.isArray(state.libraryBatchBuildStatus?.selected_library_ids) && state.libraryBatchBuildStatus.selected_library_ids.length
    ? state.libraryBatchBuildStatus.selected_library_ids
    : $$('input[name="settings-library-batch-workflow-selection"]:checked').map(node => node.value);
  state.libraryBatchBuildStatus = await setLibraryBatchBuildSelection(selectedLibraryIDs);
  state.libraryBatchThumbnailBuildStatus = await setLibraryBatchThumbnailBuildSelection(selectedLibraryIDs);
}

async function setLibraryBatchWorkflowSelectionAll(enabled) {
  const selectedLibraryIDs = enabled ? allBatchSelectableLibraryValues(libraryBatchWorkflowSelectionStatus()) : [];
  applyLibraryBatchSelectionState(state.libraryBatchBuildStatus, selectedLibraryIDs);
  applyLibraryBatchSelectionState(state.libraryBatchThumbnailBuildStatus, selectedLibraryIDs);
  state.libraryBatchBuildStatus = await setLibraryBatchBuildSelection(selectedLibraryIDs);
  state.libraryBatchThumbnailBuildStatus = await setLibraryBatchThumbnailBuildSelection(selectedLibraryIDs);
}

function libraryBatchBuildStatusLabel(status) {
  switch (String(status || '')) {
    case 'pending': return settingsText('batchWorkflow.statusPending');
    case 'scanning': return settingsText('batchWorkflow.statusScanning');
    case 'building': return settingsText('batchWorkflow.statusBuilding');
    case 'completed': return settingsText('batchWorkflow.statusCompleted');
    case 'failed': return settingsText('batchWorkflow.statusFailed');
    case 'cancelled': return settingsText('batchWorkflow.statusCancelled');
    case 'running': return settingsText('batchWorkflow.statusRunning');
    case 'cancelling': return settingsText('batchWorkflow.statusCancelling');
    default: return settingsText('batchWorkflow.statusIdle');
  }
}

function libraryBatchWorkflowSelectionStatus() {
  const scan = state.libraryBatchBuildStatus || {};
  const thumbnails = state.libraryBatchThumbnailBuildStatus || {};
  if (state.libraryBatchWorkflowPhase === 'thumbnails') return thumbnails;
  if (state.libraryBatchWorkflowPhase === 'scan') return scan;
  if (isLibraryBatchBuildActive(scan)) return scan;
  if (isLibraryBatchThumbnailBuildActive(thumbnails)) return thumbnails;
  if (thumbnails.status === 'completed' && state.libraryBatchWorkflowEnabled && Array.isArray(thumbnails.libraries) && thumbnails.libraries.length) return thumbnails;
  if (Array.isArray(scan.libraries) && scan.libraries.length) return scan;
  if (Array.isArray(thumbnails.libraries) && thumbnails.libraries.length) return thumbnails;
  return scan || thumbnails || {};
}

function clampProgressPercent(value) {
  return Math.max(0, Math.min(100, Number(value) || 0));
}

function libraryBatchWorkflowStageProgressPercent() {
  const scan = state.libraryBatchBuildStatus || {};
  const thumbnails = state.libraryBatchThumbnailBuildStatus || {};
  if (isLibraryBatchThumbnailBuildActive(thumbnails) || state.libraryBatchWorkflowPhase === 'thumbnails') return libraryBatchTaskPercent(thumbnails);
  if (isLibraryBatchBuildActive(scan) || state.libraryBatchWorkflowPhase === 'scan') return libraryBatchTaskPercent(scan);
  return libraryBatchTaskPercent(state.libraryBatchWorkflowEnabled ? thumbnails : scan);
}

function libraryBatchTaskPercent(status) {
  if (!status) return 0;
  if (status.status === 'completed') return 100;
  const total = Math.max(0, Number(status.total_libraries) || 0);
  if (!total) return 0;
  const finished = Math.max(0, Number(status.completed_libraries) || 0) + Math.max(0, Number(status.failed_libraries) || 0);
  const current = Math.max(finished, Math.max(0, Number(status.current_library_index) || 0) - 1);
  return clampProgressPercent((current + (current < total ? clampProgressPercent(status.current_percent) / 100 : 0)) / total * 100);
}

function libraryBatchWorkflowTotalProgressPercent() {
  const scan = state.libraryBatchBuildStatus || {};
  const thumbnails = state.libraryBatchThumbnailBuildStatus || {};
  const scanPercent = libraryBatchTaskPercent(scan);
  const thumbnailPercent = libraryBatchTaskPercent(thumbnails);
  const twoStage = state.libraryBatchWorkflowEnabled;
  if (state.libraryBatchWorkflowPhase === 'thumbnails' || isLibraryBatchThumbnailBuildActive(thumbnails)) {
    return twoStage ? 50 + thumbnailPercent / 2 : thumbnailPercent;
  }
  if (state.libraryBatchWorkflowPhase === 'scan' || isLibraryBatchBuildActive(scan)) {
    return twoStage ? scanPercent / 2 : scanPercent;
  }
  if (twoStage && String(thumbnails.status || '') === 'completed') return 100;
  return twoStage ? scanPercent / 2 : libraryBatchWorkflowStageProgressPercent();
}

function estimateBatchETASeconds(status, percent) {
  const elapsed = Math.max(0, Number(status && status.elapsed_seconds) || 0);
  const value = clampProgressPercent(percent);
  if (elapsed <= 2 || value <= 0 || value >= 100) return 0;
  return Math.max(0, Math.round(elapsed * (100 - value) / value));
}

function libraryBatchWorkflowMetricItems(stagePercent, totalPercent) {
  const activeStatus = isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus) || state.libraryBatchWorkflowPhase === 'thumbnails'
    ? (state.libraryBatchThumbnailBuildStatus || {})
    : (state.libraryBatchBuildStatus || {});
  const status = libraryBatchWorkflowSelectionStatus();
  const current = Math.max(0, Number(activeStatus.current_library_index || status.current_library_index) || 0);
  const totalLibraries = Math.max(0, Number(activeStatus.total_libraries || status.total_libraries) || 0);
  const done = Math.max(0, Number(activeStatus.current_done || status.current_done) || 0);
  const total = Math.max(0, Number(activeStatus.current_total || status.current_total) || 0);
  const eta = estimateBatchETASeconds(activeStatus, stagePercent);
  const items = [];
  if (totalLibraries > 0 && current > 0) items.push(settingsText('batchWorkflow.metricLibrary', { current: Math.min(current, totalLibraries), total: totalLibraries }));
  if (total > 0) items.push(settingsText('batchWorkflow.metricStage', { current: done, total }));
  items.push(settingsText('batchWorkflow.metricCurrent', { percent: Math.round(stagePercent) }));
  if (Math.round(totalPercent) !== Math.round(stagePercent)) items.push(settingsText('batchWorkflow.metricTotal', { percent: Math.round(totalPercent) }));
  items.push(settingsText('batchWorkflow.metricEta', { value: eta > 0 ? formatSecondsShort(eta) : settingsText('batchWorkflow.etaPending') }));
  return items;
}

function libraryBatchWorkflowProgressCopy() {
  const status = libraryBatchWorkflowSelectionStatus();
  if (isLibraryBatchBuildActive(state.libraryBatchBuildStatus)) {
    return renderLibraryBatchTaskProgress(state.libraryBatchBuildStatus, settingsText('batchWorkflow.scanStart'));
  }
  if (isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus)) {
    return renderLibraryBatchTaskProgress(state.libraryBatchThumbnailBuildStatus, settingsText('batchWorkflow.thumbStart'));
  }
  return renderLibraryBatchTaskProgress(status, settingsText('batchWorkflow.idle'));
}

function renderLibraryBatchWorkflowSummary(status, totalPercent) {
  const scanRows = Array.isArray(state.libraryBatchBuildStatus?.libraries) ? state.libraryBatchBuildStatus.libraries : [];
  const thumbRows = Array.isArray(state.libraryBatchThumbnailBuildStatus?.libraries) ? state.libraryBatchThumbnailBuildStatus.libraries : [];
  const phase = String(status?.current_phase || '');
  const phaseName = {
    scan: copyText('app.interface.s_47b1409f6e'), thumbnails: copyText('app.interface.s_83f794b00a'),
  }[phase] || (status?.status === 'completed'
    ? settingsText('batchWorkflow.statusCompleted')
    : settingsText('batchWorkflow.statusIdle'));
  const count = (rows, key) => rows.reduce((sum, row) => sum + (Number(row[key]) || 0), 0);
  const activeCount = Math.max(0, Number(status?.current_done) || 0);
  const activeTotal = Math.max(0, Number(status?.current_total) || 0);
  const values = [
    [copyText('app.interface.s_61998c8c88'), phaseName],
    [copyText('app.interface.s_e1c52eb929'), status?.current_library_name || copyText('app.interface.d_7f6f57480c', { v0: (Math.max(0, Number(status?.completed_libraries) || 0)), v1: (Math.max(0, Number(status?.total_libraries) || 0)) })],
    [copyText('app.interface.s_073957fea3'), activeTotal ? `${Math.min(activeCount, activeTotal)} / ${activeTotal}` : copyText('app.interface.s_faf6292b7a')],
    [copyText('app.interface.s_562d1d2725'), `${Math.round(totalPercent)}%`],
    [copyText('app.interface.s_e7ad34645a'), count(scanRows, 'imported')],
    [copyText('app.interface.s_e952762557'), count(scanRows, 'skipped')],
    [copyText('app.interface.s_60248e225f'), count(thumbRows, 'skipped')],
    [copyText('app.interface.s_cfc54766b7'), count(thumbRows, 'generated')],
    [copyText('app.interface.s_763284f0fb'), count(scanRows, 'failed') + count(thumbRows, 'failed')],
  ];
  return `<dl class="settings-batch-workflow-summary">${values.map(([label, value]) => `<div><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(String(value))}</dd></div>`).join('')}</dl>`;
}

function renderLibraryBatchWorkflowPanel() {
  normalizeLibraryBatchWorkflowRuntimeState();
  const starting = !!state.libraryBatchWorkflowStartPending;
  const running = libraryBatchWorkflowActive();
  const controlsDisabled = starting || running || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive();
  const hasSelection = libraryBatchWorkflowHasSelection();
  const selectionStatus = libraryBatchWorkflowSelectionStatus();
  const totalProgressPercent = libraryBatchWorkflowTotalProgressPercent();
  const progressStatus = libraryBatchWorkflowSelectionStatus();
  return `<div class="settings-batch-build-panel settings-batch-build-panel-master">
    ${renderSettingsPanelHeader(settingsText('sections.batchWorkflow.title'), settingsText('sections.batchWorkflow.copy'), settingsPanelHeadIconMap.batchWorkflow)}
    <div class="settings-batch-workflow-progress" aria-label="${escapeHTML(settingsText('batchWorkflow.progressAria'))}">
      <span style="width:${totalProgressPercent}%"></span>
    </div>
    ${renderLibraryBatchWorkflowSummary(progressStatus, totalProgressPercent)}
    <div class="settings-batch-build-selection">
      <strong>${escapeHTML(settingsText('batchWorkflow.selectionTitle'))}</strong>
      <div class="settings-batch-build-selection-list settings-batch-workflow-selection-list ${controlsDisabled ? 'is-disabled' : ''}">${renderLibraryBatchSelectionRows(selectionStatus, 'settings-library-batch-workflow-selection', controlsDisabled)}</div>
    </div>
    <div class="settings-group settings-batch-build-master-toggles ${controlsDisabled ? 'is-disabled' : ''}">
      ${renderSettingsToggle('settings-library-batch-workflow-select-all', settingsText('batchWorkflow.selectAll'), libraryBatchWorkflowAllSelected(), settingsText('batchWorkflow.selectAllCopy'), controlsDisabled)}
      ${renderSettingsToggle('settings-library-batch-workflow-enabled', settingsText('batchWorkflow.autoThumbnails'), state.libraryBatchWorkflowEnabled, settingsText('batchWorkflow.autoThumbnailsCopy'), controlsDisabled)}
      ${renderSettingsToggle('settings-library-batch-workflow-aggressive', settingsText('batchWorkflow.aggressive'), state.libraryBatchWorkflowAggressive, settingsText('batchWorkflow.aggressiveCopy'), controlsDisabled)}
    </div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-start-library-batch-workflow-btn" type="button" ${controlsDisabled || !hasSelection ? 'disabled' : ''}>${escapeHTML(starting ? settingsText('batchWorkflow.startBusy') : (state.libraryBatchWorkflowEnabled ? settingsText('batchWorkflow.startFull') : settingsText('batchWorkflow.startScan')))}</button>
      <button class="btn" id="settings-cancel-library-batch-workflow-btn" type="button" ${running || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive() ? '' : 'disabled'}>${escapeHTML(running ? settingsText('batchWorkflow.cancelWorkflow') : settingsText('batchWorkflow.cancelStage'))}</button>
    </div>
  </div>`;
}

async function cancelLibraryBatchWorkflow() {
  clearLibraryBatchWorkflowPhase();
  if (isLibraryBatchBuildActive()) {
    if (state.libraryBatchBuildCancelPending) return;
    state.libraryBatchBuildCancelPending = true;
    syncLibraryBatchWorkflowPanel();
    try {
      state.libraryBatchBuildStatus = await cancelLibraryBatchBuild();
      stopLibraryBatchBuildPolling();
      state.libraryBatchBuildPollTimer = setTimeout(pollLibraryBatchBuildStatus, 300);
    } catch (e) {
      state.libraryBatchBuildCancelPending = false;
      alert(copyText('app.interface.s_6ded5e046d') + ((e && e.error) || e));
    }
    syncLibraryBatchWorkflowPanel();
    return;
  }
  if (isLibraryBatchThumbnailBuildActive()) {
    if (state.libraryBatchThumbnailBuildCancelPending) return;
    state.libraryBatchThumbnailBuildCancelPending = true;
    syncLibraryBatchWorkflowPanel();
    try {
      state.libraryBatchThumbnailBuildStatus = await cancelLibraryBatchThumbnailBuild();
      stopLibraryBatchThumbnailBuildPolling();
      state.libraryBatchThumbnailBuildPollTimer = setTimeout(pollLibraryBatchThumbnailBuildStatus, 300);
    } catch (e) {
      state.libraryBatchThumbnailBuildCancelPending = false;
      alert(copyText('app.interface.s_d5a6de8c62') + ((e && e.error) || e));
    }
    syncLibraryBatchWorkflowPanel();
  }
}

async function openLibraryBatchFullWorkflow() {
  if (state.libraryBatchWorkflowStartPending || libraryBatchWorkflowActive() || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive()) {
    syncLibraryBatchWorkflowPanel();
    return;
  }
  state.libraryBatchWorkflowStartPending = true;
  syncLibraryBatchWorkflowPanel();
  try {
    await syncLibraryBatchWorkflowSelections();
    setLibraryBatchWorkflowPhase(state.libraryBatchWorkflowEnabled ? 'scan' : '');
    await openLibraryBatchBuildWorkflow({
      aggressive: state.libraryBatchWorkflowAggressive,
      startedByWorkflow: true,
      buildThumbnailsAfterScan: state.libraryBatchWorkflowEnabled,
    });
  } catch (e) {
    clearLibraryBatchWorkflowPhase();
    alert(copyText('app.interface.s_17bc5b6063') + ((e && e.error) || e.message || e));
  } finally {
    state.libraryBatchWorkflowStartPending = false;
    syncLibraryBatchWorkflowPanel();
  }
}

function libraryBatchSelectedSet(status) {
  const selectedIDs = Array.isArray(status && status.selected_library_ids) ? status.selected_library_ids.map(id => String(id || '').trim()).filter(Boolean) : [];
  if (selectedIDs.length) return new Set(selectedIDs);
  return new Set(Array.isArray(status && status.selected_paths) ? status.selected_paths.map(path => String(path || '').trim()).filter(Boolean) : []);
}

function libraryBatchSelectionConfigured(status) {
  return !!(status && status.selection_configured) || libraryBatchSelectedSet(status).size > 0;
}

function enabledBatchSelectableLibraries(status) {
  return allBatchSelectableLibraries(status).filter(library => library && library.available !== false);
}

function allBatchSelectableLibraryValues(status) {
  return enabledBatchSelectableLibraries(status).map(library => String(library.id || library.path || '').trim()).filter(Boolean);
}

function libraryBatchStatusAllSelected(status) {
  const rows = enabledBatchSelectableLibraries(status);
  if (!rows.length) return false;
  const selected = libraryBatchSelectedSet(status);
  if (!libraryBatchSelectionConfigured(status)) return true;
  return rows.every(library => {
    const id = String(library.id || '').trim();
    const path = String(library.path || '').trim();
    return (id && selected.has(id)) || (path && selected.has(path));
  });
}

function libraryBatchWorkflowAllSelected() {
  return libraryBatchStatusAllSelected(libraryBatchWorkflowSelectionStatus());
}

function libraryBatchStatusHasSelection(status) {
  const rows = enabledBatchSelectableLibraries(status);
  if (!rows.length) return false;
  if (!libraryBatchSelectionConfigured(status)) return true;
  return libraryBatchSelectedSet(status).size > 0;
}

function libraryBatchWorkflowHasSelection() {
  return libraryBatchStatusHasSelection(libraryBatchWorkflowSelectionStatus());
}

function applyLibraryBatchSelectionState(status, selectedLibraryIDs) {
  if (!status) return;
  status.selected_library_ids = Array.isArray(selectedLibraryIDs) ? [...selectedLibraryIDs] : [];
  status.selected_paths = [];
  status.selection_configured = true;
}

function libraryBatchCanResume(status) {
  const rows = Array.isArray(status && status.libraries) ? status.libraries : [];
  if (!rows.length) return false;
  const hasCompleted = rows.some(row => String(row.status || '') === 'completed');
  const hasPendingWork = rows.some(row => String(row.status || '') !== 'completed');
  return hasCompleted && hasPendingWork;
}

function allBatchSelectableLibraries(status) {
  const configured = normalizeLibraries(state.serverSettings && state.serverSettings.libraries, state.serverSettings && state.serverSettings.storage_path || '');
  const rows = Array.isArray(status && status.libraries) ? status.libraries : [];
  if (!configured.length) return rows;
  const rowByPath = new Map(rows.map(row => [String(row.path || '').trim(), row]));
  return configured.map((library, index) => {
    const path = String(library.path || '').trim();
    const row = rowByPath.get(path) || {};
    return {
      id: library.id || row.id || '',
      name: library.name || row.name || copyText('app.interface.libraryNumberedName', { v0: (index + 1) }),
      path,
      status: row.status || 'pending',
      message: row.message || '',
      available: library.available !== false,
      unavailable_reason: library.unavailable_reason || '',
      imported: row.imported || 0,
      skipped: row.skipped || 0,
      pruned: row.pruned || 0,
      generated: row.generated || 0,
      failed: row.failed || 0,
    };
  });
}

function renderLibraryBatchSelectionRows(status, inputName, disabled = false) {
  const selected = libraryBatchSelectedSet(status);
  const usingIDs = Array.isArray(status && status.selected_library_ids) && status.selected_library_ids.length > 0;
  const selectionConfigured = libraryBatchSelectionConfigured(status);
  const rows = allBatchSelectableLibraries(status);
  if (!rows.length) return `<div class="settings-batch-build-empty">${escapeHTML(settingsText('batchWorkflow.noSelectableLibraries'))}</div>`;
  return rows.map((library, index) => {
    const value = String(library.id || library.path || '').trim();
    const compareValue = usingIDs ? value : String(library.path || '').trim();
    const libraryAvailable = library.available !== false;
    const interactive = libraryAvailable && !disabled;
    const checked = selectionConfigured ? selected.has(compareValue) : libraryAvailable;
    return `<label class="settings-batch-build-select-row ${checked ? 'is-selected' : ''}${libraryAvailable ? '' : ' is-unavailable'}${interactive ? '' : ' is-disabled'}">
      <span class="settings-toggle-switch settings-batch-build-checkbox">
        <input type="checkbox" name="${escapeHTML(inputName)}" value="${escapeHTML(value)}" ${checked ? 'checked' : ''} ${interactive ? '' : 'disabled'}>
        <span class="settings-toggle-slider" aria-hidden="true"></span>
      </span>
      <span class="settings-batch-build-select-copy">
        <strong>${escapeHTML(library.name || settingsText('libraries.nameFallback', { index: index + 1 }))}</strong>
        <span>${escapeHTML(library.path || '')}</span>
        ${libraryAvailable || !library.unavailable_reason ? '' : `<span class="settings-batch-build-select-note">${escapeHTML(library.unavailable_reason)}</span>`}
      </span>
    </label>`;
  }).join('');
}

function renderLibraryBatchTaskProgress(status, idleLabel) {
  const current = Math.max(0, Number(status && status.current_library_index) || 0);
  const totalLibraries = Math.max(0, Number(status && status.total_libraries) || 0);
  const currentDone = Math.max(0, Number(status && status.current_done) || 0);
  const currentTotal = Math.max(0, Number(status && status.current_total) || 0);
  const completed = Math.max(0, Number(status && status.completed_libraries) || 0);
  const failed = Math.max(0, Number(status && status.failed_libraries) || 0);
  const currentLibrary = String(status && status.current_library_name || '').trim();
  let phase = settingsText('batchWorkflow.phaseScan');
  switch (String(status && status.current_phase || '')) {
    case 'thumbnails':
      phase = settingsText('batchWorkflow.phaseThumbnails');
      break;
    case 'migrate':
      phase = settingsText('batchWorkflow.phaseMigrate');
      break;
    case 'cleanup':
      phase = settingsText('batchWorkflow.phaseCleanup');
      break;
    case 'maintenance':
      phase = settingsText('batchWorkflow.phaseMaintenance');
      break;
    case 'playback':
      phase = settingsText('batchWorkflow.phasePlayback');
      break;
    default:
      phase = settingsText('batchWorkflow.phaseScan');
      break;
  }
  const parts = [];
  if (currentLibrary) parts.push(settingsText('batchWorkflow.taskCurrent', { name: currentLibrary }));
  if (totalLibraries > 0) parts.push(settingsText('batchWorkflow.taskLibrary', { current, total: totalLibraries }));
  if (currentTotal > 0) {
    parts.push(settingsText('batchWorkflow.taskPhase', { phase, current: currentDone, total: currentTotal }));
  } else if (currentLibrary) {
    parts.push(phase);
  } else if (idleLabel) {
    parts.push(idleLabel);
  }
  parts.push(settingsText('batchWorkflow.taskDone', { count: completed }));
  if (failed > 0) parts.push(settingsText('batchWorkflow.taskFailed', { count: failed }));
  if (status && status.low_resource_mode) parts.push(settingsText('batchWorkflow.taskLowResource'));
  return parts.join(' · ');
}

function renderLibraryBatchBuildPanel() {
  const status = state.libraryBatchBuildStatus || {};
  const libraries = Array.isArray(status.libraries) ? status.libraries : [];
  const active = isLibraryBatchBuildActive(status);
  const otherActive = isLibraryBatchThumbnailBuildActive();
  const canResume = libraryBatchCanResume(status);
  const hasSelection = libraryBatchStatusHasSelection(status);
  const currentName = String(status.current_library_name || '').trim();
  const summary = [
    settingsText('batchWorkflow.summaryTotal', { count: Math.max(0, Number(status.total_libraries) || libraries.length || 0) }),
    settingsText('batchWorkflow.summaryCompleted', { count: Math.max(0, Number(status.completed_libraries) || 0) }),
    settingsText('batchWorkflow.summaryFailed', { count: Math.max(0, Number(status.failed_libraries) || 0) }),
  ].join(' · ');
  const list = libraries.length ? libraries.map((library, index) => {
    const rowStatus = String(library.status || 'pending');
    const current = currentName && currentName === String(library.name || '').trim();
    const detailParts = [];
    if (Number(library.imported) > 0) detailParts.push(settingsText('batchWorkflow.detailImported', { count: Number(library.imported) }));
    if (Number(library.skipped) > 0) detailParts.push(settingsText('batchWorkflow.detailSkipped', { count: Number(library.skipped) }));
    if (Number(library.pruned) > 0) detailParts.push(settingsText('batchWorkflow.detailPruned', { count: Number(library.pruned) }));
    if (Number(library.failed) > 0) detailParts.push(settingsText('batchWorkflow.detailFailed', { count: Number(library.failed) }));
    const meta = detailParts.length ? detailParts.join(' · ') : (library.message || library.path || '');
    return `<div class="settings-batch-build-row ${current ? 'active' : ''} is-${escapeHTML(rowStatus)}">
      <div class="settings-batch-build-row-main">
        <strong>${escapeHTML(library.name || settingsText('libraries.nameFallback', { index: index + 1 }))}</strong>
        <span>${escapeHTML(meta)}</span>
      </div>
      <em>${escapeHTML(libraryBatchBuildStatusLabel(rowStatus))}</em>
    </div>`;
  }).join('') : `<div class="settings-batch-build-empty">${escapeHTML(settingsText('batchWorkflow.scanEmpty'))}</div>`;
  const progressPercent = Math.max(0, Math.min(100, Number(status.current_percent) || 0));
  return `<div class="settings-batch-build-panel">
    <div class="settings-batch-build-head">
      <div>
        <strong>${escapeHTML(status.message || settingsText('batchWorkflow.scanTitle'))}</strong>
        <span>${escapeHTML(summary)}</span>
      </div>
      <span class="settings-batch-build-badge ${active ? 'active' : ''}">${escapeHTML(active ? settingsText('batchWorkflow.statusRunning') : libraryBatchBuildStatusLabel(status.status || 'idle'))}</span>
    </div>
    <div class="settings-batch-build-meta">
      <span>${escapeHTML(renderLibraryBatchTaskProgress(status, settingsText('batchWorkflow.scanStart')))}</span>
      <span>${escapeHTML(status.aggressive_mode ? settingsText('batchWorkflow.modeAggressive') : (status.low_resource_mode ? settingsText('batchWorkflow.modeLowResource') : settingsText('batchWorkflow.modeStandard')))}</span>
    </div>
    <div class="settings-batch-build-progress"><span style="width:${progressPercent}%"></span></div>
    <div class="settings-batch-build-selection">
      <strong>${escapeHTML(settingsText('batchWorkflow.scanScope'))}</strong>
      <div class="settings-batch-build-selection-list">${renderLibraryBatchSelectionRows(status, 'settings-library-batch-build-selection')}</div>
    </div>
    <label class="settings-batch-build-exit">
      <input id="settings-library-batch-build-exit" type="checkbox" ${status.exit_after_complete ? 'checked' : ''}>
      <span>${escapeHTML(settingsText('batchWorkflow.exitAfterComplete'))}</span>
    </label>
    <div class="settings-batch-build-list">${list}</div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-build-all-libraries-btn" type="button" ${(active || otherActive || !hasSelection) ? 'disabled' : ''}>${escapeHTML(active ? settingsText('batchWorkflow.scanActive') : (canResume ? settingsText('batchWorkflow.scanResume') : settingsText('batchWorkflow.scanStart')))}</button>
      <button class="btn" id="settings-cancel-all-libraries-btn" type="button" ${active ? '' : 'disabled'}>${state.libraryBatchBuildCancelPending ? copyText('app.libraryBatchScan.cancelling') : copyText('app.libraryBatchScan.cancel')}</button>
    </div>
  </div>`;
}

function syncLibraryBatchBuildPanel() {
  const container = $('#settings-library-batch-build');
  if (!container) return;
  container.innerHTML = renderLibraryBatchBuildPanel();
  $$('input[name="settings-library-batch-build-selection"]').forEach(input => {
    input.addEventListener('change', async () => {
      if (isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive()) {
        syncLibraryBatchBuildPanel();
        return;
      }
      const selectedLibraryIDs = $$('input[name="settings-library-batch-build-selection"]:checked').map(node => node.value);
      applyLibraryBatchSelectionState(state.libraryBatchBuildStatus, selectedLibraryIDs);
      try {
        state.libraryBatchBuildStatus = await setLibraryBatchBuildSelection(selectedLibraryIDs);
        setTimeout(syncLibraryBatchBuildPanel, 180);
        syncLibraryBatchWorkflowPanel();
      } catch (e) {
        alert(copyText('app.interface.s_0239c81603') + ((e && e.error) || e));
        syncLibraryBatchBuildPanel();
      }
    });
  });
  $('#settings-build-all-libraries-btn')?.addEventListener('click', openLibraryBatchBuildWorkflow);
  $('#settings-cancel-all-libraries-btn')?.addEventListener('click', async () => {
    if (state.libraryBatchBuildCancelPending || !isLibraryBatchBuildActive()) return;
    state.libraryBatchBuildCancelPending = true;
    syncLibraryBatchBuildPanel();
    try {
      state.libraryBatchBuildStatus = await cancelLibraryBatchBuild();
      stopLibraryBatchBuildPolling();
      state.libraryBatchBuildPollTimer = setTimeout(pollLibraryBatchBuildStatus, 300);
      syncLibraryBatchBuildPanel();
    } catch (e) {
      state.libraryBatchBuildCancelPending = false;
      syncLibraryBatchBuildPanel();
      alert(copyText('app.interface.s_6ded5e046d') + ((e && e.error) || e));
    }
  });
  $('#settings-library-batch-build-exit')?.addEventListener('change', async e => {
    try {
      state.libraryBatchBuildStatus = await setLibraryBatchBuildExitAfterComplete(e.target.checked);
      syncLibraryBatchBuildPanel();
    } catch (err) {
      e.target.checked = !e.target.checked;
      alert(copyText('app.interface.settingsSaveFailedPrefix') + ((err && err.error) || err));
    }
  });
}

function syncLibraryBatchWorkflowPanel() {
  const container = $('#settings-library-batch-workflow');
  if (!container) return;
  container.innerHTML = renderLibraryBatchWorkflowPanel();
  bindSettingsPanelToggles(container);
  $$('input[name="settings-library-batch-workflow-selection"]').forEach(input => {
    input.addEventListener('change', async () => {
      if (state.libraryBatchWorkflowStartPending || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive()) {
        syncLibraryBatchWorkflowPanel();
        return;
      }
      const selectedLibraryIDs = $$('input[name="settings-library-batch-workflow-selection"]:checked').map(node => node.value);
      applyLibraryBatchSelectionState(state.libraryBatchBuildStatus, selectedLibraryIDs);
      applyLibraryBatchSelectionState(state.libraryBatchThumbnailBuildStatus, selectedLibraryIDs);
      try {
        state.libraryBatchBuildStatus = await setLibraryBatchBuildSelection(selectedLibraryIDs);
        state.libraryBatchThumbnailBuildStatus = await setLibraryBatchThumbnailBuildSelection(selectedLibraryIDs);
        setTimeout(syncLibraryBatchWorkflowPanel, 180);
      } catch (e) {
        alert(copyText('app.interface.s_c0d87ab62c') + ((e && e.error) || e));
        syncLibraryBatchWorkflowPanel();
      }
    });
  });
  $('#settings-library-batch-workflow-select-all')?.addEventListener('change', async e => {
    if (state.libraryBatchWorkflowStartPending || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive()) {
      syncLibraryBatchWorkflowPanel();
      return;
    }
    try {
      await setLibraryBatchWorkflowSelectionAll(!!e.target.checked);
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    } catch (err) {
      alert(copyText('app.interface.s_c0d87ab62c') + ((err && err.error) || err));
      syncLibraryBatchWorkflowPanel();
    }
  });
  $('#settings-library-batch-workflow-enabled')?.addEventListener('change', e => {
    state.libraryBatchWorkflowEnabled = !!e.target.checked;
    persistLibraryBatchWorkflowPrefs();
    syncLibraryBatchWorkflowPanel();
  });
  $('#settings-library-batch-workflow-aggressive')?.addEventListener('change', e => {
    state.libraryBatchWorkflowAggressive = !!e.target.checked;
    persistLibraryBatchWorkflowPrefs();
    syncLibraryBatchWorkflowPanel();
  });
  $('#settings-start-library-batch-workflow-btn')?.addEventListener('click', openLibraryBatchFullWorkflow);
  $('#settings-cancel-library-batch-workflow-btn')?.addEventListener('click', cancelLibraryBatchWorkflow);
}

function renderLibraryBatchThumbnailBuildPanel() {
  const status = state.libraryBatchThumbnailBuildStatus || {};
  const libraries = Array.isArray(status.libraries) ? status.libraries : [];
  const active = isLibraryBatchThumbnailBuildActive(status);
  const otherActive = isLibraryBatchBuildActive();
  const canResume = libraryBatchCanResume(status);
  const hasSelection = libraryBatchStatusHasSelection(status);
  const currentName = String(status.current_library_name || '').trim();
  const summary = [
    settingsText('batchWorkflow.summaryTotal', { count: Math.max(0, Number(status.total_libraries) || libraries.length || 0) }),
    settingsText('batchWorkflow.summaryCompleted', { count: Math.max(0, Number(status.completed_libraries) || 0) }),
    settingsText('batchWorkflow.summaryFailed', { count: Math.max(0, Number(status.failed_libraries) || 0) }),
  ].join(' · ');
  const list = libraries.length ? libraries.map((library, index) => {
    const rowStatus = String(library.status || 'pending');
    const current = currentName && currentName === String(library.name || '').trim();
    const detailParts = [];
    if (Number(library.imported) > 0) detailParts.push(settingsText('batchWorkflow.detailMigrated', { count: Number(library.imported) }));
    if (Number(library.pruned) > 0) detailParts.push(settingsText('batchWorkflow.detailPruned', { count: Number(library.pruned) }));
    if (Number(library.generated) > 0) detailParts.push(settingsText('batchWorkflow.detailGenerated', { count: Number(library.generated) }));
    if (Number(library.skipped) > 0) detailParts.push(settingsText('batchWorkflow.detailExisting', { count: Number(library.skipped) }));
    if (Number(library.failed) > 0) detailParts.push(settingsText('batchWorkflow.detailFailed', { count: Number(library.failed) }));
    const meta = detailParts.length ? detailParts.join(' · ') : (library.message || library.path || '');
    return `<div class="settings-batch-build-row ${current ? 'active' : ''} is-${escapeHTML(rowStatus)}">
      <div class="settings-batch-build-row-main">
        <strong>${escapeHTML(library.name || settingsText('libraries.nameFallback', { index: index + 1 }))}</strong>
        <span>${escapeHTML(meta)}</span>
      </div>
      <em>${escapeHTML(libraryBatchBuildStatusLabel(rowStatus))}</em>
    </div>`;
  }).join('') : `<div class="settings-batch-build-empty">${escapeHTML(settingsText('batchWorkflow.thumbEmpty'))}</div>`;
  const progressPercent = Math.max(0, Math.min(100, Number(status.current_percent) || 0));
  return `<div class="settings-batch-build-panel">
    <div class="settings-batch-build-head">
      <div>
        <strong>${escapeHTML(status.message || settingsText('batchWorkflow.thumbTitle'))}</strong>
        <span>${escapeHTML(summary)}</span>
      </div>
      <span class="settings-batch-build-badge ${active ? 'active' : ''}">${escapeHTML(active ? settingsText('batchWorkflow.statusRunning') : libraryBatchBuildStatusLabel(status.status || 'idle'))}</span>
    </div>
    <div class="settings-batch-build-meta">
      <span>${escapeHTML(renderLibraryBatchTaskProgress(status, settingsText('batchWorkflow.thumbStart')))}</span>
      <span>${escapeHTML(status.aggressive_mode ? settingsText('batchWorkflow.modeAggressive') : (status.low_resource_mode ? settingsText('batchWorkflow.modeLowResource') : settingsText('batchWorkflow.modeStandard')))}</span>
    </div>
    <div class="settings-batch-build-progress"><span style="width:${progressPercent}%"></span></div>
    <div class="settings-batch-build-selection">
      <strong>${escapeHTML(settingsText('batchWorkflow.thumbScope'))}</strong>
      <div class="settings-batch-build-selection-list">${renderLibraryBatchSelectionRows(status, 'settings-library-batch-thumbnail-build-selection')}</div>
    </div>
    <label class="settings-batch-build-exit">
      <input id="settings-library-batch-thumbnail-build-exit" type="checkbox" ${status.exit_after_complete ? 'checked' : ''}>
      <span>${escapeHTML(settingsText('batchWorkflow.exitAfterComplete'))}</span>
    </label>
    <div class="settings-batch-build-list">${list}</div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-build-all-library-thumbnails-btn" type="button" ${(active || otherActive || !hasSelection) ? 'disabled' : ''}>${escapeHTML(active ? settingsText('batchWorkflow.thumbActive') : (canResume ? settingsText('batchWorkflow.thumbResume') : settingsText('batchWorkflow.thumbStart')))}</button>
      <button class="btn" id="settings-cancel-all-library-thumbnails-btn" type="button" ${active ? '' : 'disabled'}>${state.libraryBatchThumbnailBuildCancelPending ? copyText('app.libraryBatchThumbnail.cancelling') : copyText('app.libraryBatchThumbnail.cancel')}</button>
    </div>
  </div>`;
}

function syncLibraryBatchThumbnailBuildPanel() {
  const container = $('#settings-library-batch-thumbnail-build');
  if (!container) return;
  container.innerHTML = renderLibraryBatchThumbnailBuildPanel();
  bindSettingsPanelToggles(container);
  $$('input[name="settings-library-batch-thumbnail-build-selection"]').forEach(input => {
    input.addEventListener('change', async () => {
      if (isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive()) {
        syncLibraryBatchThumbnailBuildPanel();
        return;
      }
      const selectedLibraryIDs = $$('input[name="settings-library-batch-thumbnail-build-selection"]:checked').map(node => node.value);
      applyLibraryBatchSelectionState(state.libraryBatchThumbnailBuildStatus, selectedLibraryIDs);
      try {
        state.libraryBatchThumbnailBuildStatus = await setLibraryBatchThumbnailBuildSelection(selectedLibraryIDs);
        setTimeout(syncLibraryBatchThumbnailBuildPanel, 180);
        syncLibraryBatchWorkflowPanel();
      } catch (e) {
        alert(copyText('app.interface.s_2c5af95723') + ((e && e.error) || e));
        syncLibraryBatchThumbnailBuildPanel();
      }
    });
  });
  $('#settings-build-all-library-thumbnails-btn')?.addEventListener('click', openLibraryBatchThumbnailBuildWorkflow);
  $('#settings-cancel-all-library-thumbnails-btn')?.addEventListener('click', async () => {
    if (state.libraryBatchThumbnailBuildCancelPending || !isLibraryBatchThumbnailBuildActive()) return;
    state.libraryBatchThumbnailBuildCancelPending = true;
    syncLibraryBatchThumbnailBuildPanel();
    try {
      state.libraryBatchThumbnailBuildStatus = await cancelLibraryBatchThumbnailBuild();
      stopLibraryBatchThumbnailBuildPolling();
      state.libraryBatchThumbnailBuildPollTimer = setTimeout(pollLibraryBatchThumbnailBuildStatus, 300);
      syncLibraryBatchThumbnailBuildPanel();
    } catch (e) {
      state.libraryBatchThumbnailBuildCancelPending = false;
      syncLibraryBatchThumbnailBuildPanel();
      alert(copyText('app.interface.s_d5a6de8c62') + ((e && e.error) || e));
    }
  });
  $('#settings-library-batch-thumbnail-build-exit')?.addEventListener('change', async e => {
    try {
      state.libraryBatchThumbnailBuildStatus = await setLibraryBatchThumbnailBuildExitAfterComplete(e.target.checked);
      syncLibraryBatchThumbnailBuildPanel();
    } catch (err) {
      e.target.checked = !e.target.checked;
      alert(copyText('app.interface.settingsSaveFailedPrefix') + ((err && err.error) || err));
    }
  });
}

function renderSettingsContent({ host = settingsViewHost(), activate = state.view === 'settings' } = {}) {
  const activeAccent = resolveLibraryAccentColor(currentLibraryBrand() || {});
  const activeAccentText = readableTextColorForBackground(activeAccent);
  const thumbnailBuildSize = 512;
  const idleSeconds = lightboxUIIdleSeconds();
  const pwa = normalizePWASettings(state.pwaSettings);
  const hasShareLinks = Array.isArray(state.shareLinks) && state.shareLinks.length > 0;
  const canManageLibrariesUI = canManageLibraries();
  const canRunServerTasksUI = canRunServerTasks();
  const canAccessBatchWorkflowUI = canAccessBatchWorkflow();
  const canManageSharesUI = canManageShareLinks();
  const canAdminUI = !!state.serverSettings.can_admin;
  const showExperimentalSettings = !isVisitorUser();
  const availableLocales = window.EchoGalleryI18n && window.EchoGalleryI18n.getAvailableLocales
    ? window.EchoGalleryI18n.getAvailableLocales()
    : [{ code: 'zh-CN', label: 'zh-CN' }];
  const currentLocale = window.EchoGalleryI18n && window.EchoGalleryI18n.getLocale
    ? window.EchoGalleryI18n.getLocale()
    : 'zh-CN';
  host.innerHTML = `
<div class="settings-layout">
  ${canManageLibrariesUI ? `
  <section class="card settings-panel settings-panel-application">
    ${renderSettingsPanelHeader(settingsText('sections.libraries.title'), settingsText('sections.libraries.copy'), settingsPanelHeadIconMap.libraries)}
    <div class="settings-application-grid">
      <div class="settings-control settings-control-panel settings-control-wide">
        <div class="settings-library-list" id="settings-library-list">${renderLibrarySettingsRows()}</div>
      </div>
    </div>
    <button class="btn btn-primary settings-floating-add-library" id="settings-add-library-btn" type="button" style="--highlight-bg:${activeAccent};--highlight-text:${activeAccentText}">${icons.libraryCreate} ${escapeHTML(settingsText('sections.libraries.add'))}</button>
  </section>
  ` : ''}

  ${canAccessBatchWorkflowUI ? `
  <section class="card settings-panel settings-panel-batch-workflow">
    <div id="settings-library-batch-workflow">${renderLibraryBatchWorkflowPanel()}</div>
  </section>
  ` : ''}

  <section class="card settings-panel settings-panel-display">
    ${renderSettingsPanelHeader(settingsText('sections.display.title'), settingsText('sections.display.copy'), settingsPanelHeadIconMap.display)}
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-grid-gap"><span>${escapeHTML(settingsText('display.gridGap'))}</span><span id="settings-grid-gap-value">${state.gridGap}px</span></label>
        <input class="input" id="settings-grid-gap" type="range" min="0" max="24" step="1" value="${state.gridGap}">
      </div>
      <div class="settings-control">
        <label for="settings-thumb-radius"><span>${escapeHTML(settingsText('display.thumbRadius'))}</span><span id="settings-thumb-radius-value">${state.thumbRadius}px</span></label>
        <input class="input" id="settings-thumb-radius" type="range" min="0" max="24" step="1" value="${state.thumbRadius}">
      </div>
      <div class="settings-control settings-control-disabled" aria-disabled="true">
        <label for="settings-thumbnail-size"><span>${escapeHTML(settingsText('display.thumbnailSize'))}</span><span id="settings-thumbnail-size-value">${thumbnailBuildSize}px</span></label>
        <input class="input" id="settings-thumbnail-size" type="range" min="512" max="512" step="1" value="512" disabled>
      </div>
      ${renderSettingsToggle('settings-warm-enabled', settingsText('display.warmEnabled'), isWarmEnabled(), settingsText('display.warmEnabledCopy'))}
      <div class="settings-control">
        <label for="settings-lightbox-ui-idle-seconds"><span>${escapeHTML(settingsText('display.uiIdle'))}</span><span id="settings-lightbox-ui-idle-seconds-value">${escapeHTML(idleSeconds === 0 ? settingsText('display.uiIdleTap') : settingsText('display.seconds', { count: idleSeconds }))}</span></label>
        <input class="input" id="settings-lightbox-ui-idle-seconds" type="range" min="0" max="15" step="1" value="${idleSeconds}">
      </div>
      ${renderSettingsToggle('settings-throttled-video-seek', settingsText('display.throttledSeek'), state.throttledVideoSeek, settingsText('display.throttledSeekCopy'))}
      <div class="settings-control${state.throttledVideoSeek ? '' : ' settings-control-disabled'}" aria-disabled="${state.throttledVideoSeek ? 'false' : 'true'}">
        <label for="settings-video-seek-throttle-ms"><span>${escapeHTML(settingsText('display.seekThreshold'))}</span><span id="settings-video-seek-throttle-ms-value">${normalizeVideoSeekThrottleMS(state.videoSeekThrottleMS, 240)} ms</span></label>
        <input class="input" id="settings-video-seek-throttle-ms" type="range" min="120" max="1000" step="20" value="${normalizeVideoSeekThrottleMS(state.videoSeekThrottleMS, 240)}" ${state.throttledVideoSeek ? '' : 'disabled'}>
      </div>
      <div class="settings-control">
        <label for="settings-video-volume-swipe-sensitivity"><span>${escapeHTML(settingsText('display.volumeSwipeSensitivity'))}</span><span id="settings-video-volume-swipe-sensitivity-value">${normalizeVideoVolumeSwipeSensitivity(state.videoVolumeSwipeSensitivity, 100)}%</span></label>
        <input class="input" id="settings-video-volume-swipe-sensitivity" type="range" min="40" max="220" step="5" value="${normalizeVideoVolumeSwipeSensitivity(state.videoVolumeSwipeSensitivity, 100)}">
      </div>
      <div class="settings-control">
        <label for="settings-video-volume-min-percent"><span>${escapeHTML(settingsText('display.volumeMin'))}</span><span id="settings-video-volume-min-percent-value">${normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0)}%</span></label>
        <input class="input" id="settings-video-volume-min-percent" type="range" min="0" max="100" step="1" value="${normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0)}">
      </div>
      <div class="settings-control">
        <label for="settings-video-volume-max-percent"><span>${escapeHTML(settingsText('display.volumeMax'))}</span><span id="settings-video-volume-max-percent-value">${Math.max(normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0), normalizeVideoVolumePercent(state.videoVolumeMaxPercent, 100))}%</span></label>
        <input class="input" id="settings-video-volume-max-percent" type="range" min="0" max="100" step="1" value="${Math.max(normalizeVideoVolumePercent(state.videoVolumeMinPercent, 0), normalizeVideoVolumePercent(state.videoVolumeMaxPercent, 100))}">
      </div>
      ${canRunServerTasksUI ? renderSettingsToggle('settings-low-resource-mode', settingsText('display.lowResourceMode'), !!state.serverSettings.low_resource_mode, settingsText('display.lowResourceModeCopy')) : ''}
      ${canManageLibrariesUI ? `<div class="settings-control">
        <label for="settings-thumbnail-dir"><span>${escapeHTML(settingsText('display.thumbnailDir'))}</span><span>${escapeHTML(settingsText('display.thumbnailDirCopy'))}</span></label>
        <input class="input" id="settings-thumbnail-dir" type="text" value="${escapeHTML(state.serverSettings.thumbnail_dir || '')}">
      </div>` : ''}
      ${canManageLibrariesUI ? `<div class="settings-control">
        <label for="settings-trash-dir"><span>${escapeHTML(settingsText('display.trashDir'))}</span><span>${escapeHTML(settingsText('display.trashDirCopy'))}</span></label>
        <input class="input" id="settings-trash-dir" type="text" value="${escapeHTML(state.serverSettings.trash_dir || '')}">
      </div>` : ''}
    </div>
  </section>

  ${typeof renderEchoFrameSettings === 'function' ? renderEchoFrameSettings() : ''}
  <section class="card settings-panel settings-panel-playback">
    ${renderSettingsPanelHeader(settingsText('sections.playback.title'), settingsText('sections.playback.copy'), settingsPanelHeadIconMap.playback)}
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-slideshow-interval"><span>${escapeHTML(settingsText('playback.slideshowInterval'))}</span><span id="settings-slideshow-interval-value">${escapeHTML(settingsText('playback.seconds', { count: state.slideshowInterval / 1000 }))}</span></label>
        <input class="input" id="settings-slideshow-interval" type="range" min="1" max="30" step="1" value="${state.slideshowInterval / 1000}">
      </div>
      ${renderSettingsToggle('settings-slideshow-loop', settingsText('playback.slideshowLoop'), state.slideshowLoop)}
      <div class="settings-control">
        <label for="settings-slideshow-mode"><span>${escapeHTML(settingsText('playback.slideshowMode'))}</span><span>${escapeHTML(state.slideshowMode === 'random' ? settingsText('playback.random') : settingsText('playback.sequential'))}</span></label>
        <select class="input" id="settings-slideshow-mode">
          <option value="random" ${state.slideshowMode === 'random' ? 'selected' : ''}>${escapeHTML(settingsText('playback.randomOption'))}</option>
          <option value="sequential" ${state.slideshowMode === 'sequential' ? 'selected' : ''}>${escapeHTML(settingsText('playback.sequentialOption'))}</option>
        </select>
      </div>
      <div class="settings-control settings-control-wide">
        <label for="settings-player-keymap"><span>${escapeHTML(settingsText('playback.keymap'))}</span><span>${escapeHTML(settingsText('playback.keymapCopy'))}</span></label>
        <div class="settings-config-editor settings-config-editor-plain">
          <div class="settings-config-editor-head">
            <strong>ZXCWASD.conf</strong>
          </div>
          <div class="settings-config-editor-body">
            <textarea class="input settings-textarea settings-config-editor-textarea" id="settings-player-keymap" rows="12" spellcheck="false"></textarea>
          </div>
        </div>
      </div>
      <div class="settings-actions settings-actions-fill">
        <button class="btn" id="settings-reset-keymap-btn">${escapeHTML(settingsText('playback.resetKeymap'))}</button>
      </div>
      ${renderSettingsToggle('settings-autoplay-video', settingsText('playback.autoplayVideo'), state.experimentalAutoplayVideo)}
      ${renderSettingsToggle('settings-video-autoplay-next', settingsText('playback.autoplayNext'), state.videoAutoplayNext)}
      <div class="settings-control">
        <label for="settings-video-section-min"><span>${escapeHTML(settingsText('playback.videoSectionMin'))}</span><span id="settings-video-section-min-value">${escapeHTML(settingsText('playback.minutes', { count: state.videoSectionMinMinutes }))}</span></label>
        <input class="input" id="settings-video-section-min" type="range" min="1" max="240" step="1" value="${state.videoSectionMinMinutes}">
      </div>
      ${renderSettingsToggle('settings-continue-last-video-position', settingsText('playback.continueLastPosition'), state.continueLastVideoPosition, '', true)}
      ${renderSettingsToggle('settings-prefetch-neighbors', settingsText('playback.prefetchNeighbors'), state.experimentalPrefetchNeighbors)}
      <div class="settings-actions settings-actions-equal">
        ${canRunServerTasksUI ? `<button class="btn" id="settings-refresh-video-thumbs-btn">${escapeHTML(settingsText('playback.refreshVideoThumbs'))}</button>` : ''}
        ${canRunServerTasksUI ? `<button class="btn" id="settings-playback-cache-btn" type="button">${escapeHTML(settingsText('playback.playbackCache'))}</button>` : ''}
        ${canRunServerTasksUI ? `<button class="btn" id="settings-backfill-exif-btn" type="button">${escapeHTML(settingsText('playback.backfillExif'))}</button>` : ''}
      </div>
    </div>
  </section>

  ${showExperimentalSettings ? `<section class="card settings-panel settings-panel-experimental">
    ${renderSettingsPanelHeader(settingsText('sections.experimental.title'), settingsText('sections.experimental.copy'), settingsPanelHeadIconMap.experimental)}
    <div class="settings-group">
      ${renderSettingsToggle('settings-exp-restore-last-view', settingsText('experimental.restoreLastView'), state.experimentalRestoreLastView)}
      <div class="settings-static">
        <strong>${escapeHTML(settingsText('experimental.quickSwitch'))}</strong>
        <span>${escapeHTML(settingsText('experimental.quickSwitchCopy'))}</span>
      </div>
    </div>
  </section>` : ''}

  <section class="card settings-panel settings-panel-update${showExperimentalSettings ? '' : ' settings-panel-update-full'}">
    ${renderSettingsPanelHeader(settingsText('sections.pwa.title'), settingsText('pwa.headerCopy'), settingsPanelHeadIconMap.pwa)}
    <div class="settings-group settings-group-pwa">
      <div class="settings-control">
        <label for="settings-pwa-icon-source"><span>${escapeHTML(settingsText('pwa.iconSource'))}</span><span>${escapeHTML(settingsText('pwa.iconSourceCopy'))}</span></label>
        <select class="input" id="settings-pwa-icon-source">
          <option value="favicon" ${pwa.iconSource === 'favicon' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.favicon'))}</option>
          <option value="library" ${pwa.iconSource === 'library' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.libraryAvatar'))}</option>
          <option value="png" ${pwa.iconSource === 'png' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.png'))}</option>
          <option value="upload" ${pwa.iconSource === 'upload' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.upload'))}</option>
        </select>
      </div>
      <div class="settings-control">
        <label for="settings-pwa-icon-upload-btn"><span>${escapeHTML(settingsText('pwa.iconUpload'))}</span><span>${escapeHTML(pwa.uploadedIconURL ? settingsText('pwa.iconUploadReady') : settingsText('pwa.iconUploadHint'))}</span></label>
        <button class="btn settings-pwa-upload-btn" id="settings-pwa-icon-upload-btn" type="button">${icons.upload} ${escapeHTML(settingsText('pwa.chooseIcon'))}</button>
        <input id="settings-pwa-icon-file" type="file" accept=".png,.jpg,.jpeg,.gif,.webp,image/png,image/jpeg,image/gif,image/webp" hidden>
      </div>
      <div class="settings-control">
        <label for="settings-pwa-theme-mode"><span>${escapeHTML(settingsText('pwa.themeMode'))}</span><span>${escapeHTML(settingsText('pwa.themeModeCopy'))}</span></label>
        <select class="input" id="settings-pwa-theme-mode">
          <option value="accent" ${pwa.themeMode === 'accent' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.followAccent'))}</option>
          <option value="fixed" ${pwa.themeMode === 'fixed' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.fixedColor'))}</option>
        </select>
      </div>
      <div class="settings-control">
        <label for="settings-pwa-theme-color"><span>${escapeHTML(settingsText('pwa.fixedTheme'))}</span><span>${escapeHTML(settingsText('pwa.fixedThemeCopy'))}</span></label>
        <input class="input" id="settings-pwa-theme-color" type="color" value="${escapeHTML(pwa.themeColor)}" ${pwa.themeMode === 'fixed' ? '' : 'disabled'}>
      </div>
      <div class="settings-control">
        <label for="settings-pwa-cache-strategy"><span>${escapeHTML(settingsText('pwa.cacheStrategy'))}</span><span>${escapeHTML(settingsText('pwa.cacheStrategyCopy'))}</span></label>
        <select class="input" id="settings-pwa-cache-strategy">
          <option value="static-only" ${pwa.cacheStrategy === 'static-only' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.cacheStaticOnly'))}</option>
          <option value="app-shell" ${pwa.cacheStrategy === 'app-shell' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.cacheAppShell'))}</option>
          <option value="disabled" ${pwa.cacheStrategy === 'disabled' ? 'selected' : ''}>${escapeHTML(settingsText('pwa.cacheDisabled'))}</option>
        </select>
      </div>
    </div>
  </section>

  <section class="card settings-panel settings-panel-app">
    ${renderSettingsPanelHeader(settingsText('sections.app.title'), settingsText('sections.app.copy'), settingsPanelHeadIconMap.app)}
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-gallery-name"><span>${escapeHTML(copyText('app.interface.runtime.galleryName'))}</span><span>${escapeHTML(copyText('app.interface.runtime.galleryNameCopy'))}</span></label>
        <input class="input" id="settings-gallery-name" type="text" maxlength="80" value="${escapeHTML(currentAppDisplayName())}" placeholder="EchoGallery" ${state.serverSettings.can_admin ? '' : 'disabled'}>
      </div>
      <div class="settings-control">
        <label for="settings-locale"><span>${escapeHTML(settingsText('appConfig.language'))}</span><span>${escapeHTML(settingsText('appConfig.languageCopy'))}</span></label>
        <select class="input" id="settings-locale" ${availableLocales.length < 2 ? 'disabled' : ''}>
          ${availableLocales.map(locale => `<option value="${escapeHTML(locale.code)}" ${locale.code === currentLocale ? 'selected' : ''}>${escapeHTML(locale.label)}</option>`).join('')}
        </select>
      </div>
    </div>
    <div class="settings-actions settings-app-actions settings-app-actions-${canRunServerTasksUI ? '4' : '3'}">
      <a class="btn" id="settings-github-btn" href="https://github.com/in4pira10n/EchoGallery" target="_blank" rel="noopener noreferrer">${icons.github} ${escapeHTML(settingsText('appConfig.github'))}</a>
      <button class="btn" id="theme-btn" type="button"><span class="theme-icon">${document.documentElement.dataset.theme === 'dark' ? icons.sun : icons.moon}</span> ${escapeHTML(settingsText('appConfig.themeDark'))}</button>
      <button class="btn" id="logout-btn" type="button">${icons.logout} ${escapeHTML(settingsText('appConfig.logout'))}</button>
      ${canRunServerTasksUI ? `<button class="btn btn-danger" id="shutdown-btn" type="button">${icons.shutdown} ${escapeHTML(settingsText('appConfig.shutdown'))}</button>` : ''}
    </div>
    ${canManageLibrariesUI ? `<div class="settings-group settings-transfer-group">
      <div class="settings-static">
        <strong>${escapeHTML(copyText('app.interface.runtime.transferPackage'))}</strong>
        <span>${escapeHTML(copyText('app.interface.runtime.transferCopy'))}</span>
      </div>
      <div class="settings-actions settings-actions-equal">
        <button class="btn" id="settings-export-config-btn" type="button">${icons.download || ''} ${escapeHTML(copyText('app.interface.runtime.exportConfig'))}</button>
        <button class="btn" id="settings-import-config-btn" type="button">${icons.upload || ''} ${escapeHTML(copyText('app.interface.runtime.importConfig'))}</button>
        <input id="settings-import-config-file" type="file" accept=".zip,application/zip" hidden>
      </div>
    </div>` : ''}
  </section>

  <section class="card settings-panel settings-panel-update-card">
    ${renderSettingsPanelHeader(settingsText('sections.update.title'), settingsText('sections.update.copy'), settingsPanelHeadIconMap.update)}
    <div class="settings-group settings-group-update">
      ${renderSettingsToggle('settings-update-local-enabled', settingsText('update.local'), !!state.updateSettings.localEnabled, settingsText('update.localCopy'))}
      <div class="settings-static settings-update-config ${state.updateSettings.localEnabled ? '' : 'settings-update-config-disabled'}">
        <div class="settings-update-config-head">
          <strong>local-update.toml</strong>
        </div>
        <div class="settings-update-config-body">
          <textarea class="input settings-textarea settings-config-editor-textarea settings-update-config-textarea" id="settings-local-update-config" spellcheck="false" rows="8" ${state.updateSettings.localEnabled ? '' : 'disabled'}>${escapeHTML(state.serverSettings.local_update_config_text || renderLocalUpdateConfigPreview())}</textarea>
        </div>
      </div>
      ${renderSettingsToggle('settings-update-github-enabled', settingsText('update.github'), !!state.updateSettings.githubEnabled, settingsText('update.githubCopy'))}
      <div class="settings-actions settings-actions-fill settings-update-actions">
        <button class="btn" id="settings-check-update-btn" type="button">${escapeHTML(settingsText('update.check'))}</button>
        <button class="btn btn-primary" id="settings-run-update-btn" type="button" ${state.localUpdateCheckReady && state.localUpdateHasUpdate ? '' : 'disabled'}>${escapeHTML(settingsText('update.run'))}</button>
      </div>
    </div>
  </section>

  ${canManageSharesUI ? `
  <section class="card settings-panel settings-panel-share" id="settings-share-panel">
    ${renderSettingsPanelHeader(settingsText('sections.shares.title'), hasShareLinks ? settingsText('sections.shares.copy') : settingsText('sections.shares.emptyCopy'), settingsPanelHeadIconMap.shares)}
    <div class="settings-group settings-share-group">
      <div class="settings-control">
        <div class="settings-share-list" id="settings-share-list">${state.shareLinksLoaded ? (hasShareLinks ? renderSettingsShareRows() : renderSettingsShareEmptyState()) : renderSettingsShareDeferredState()}</div>
      </div>
      ${state.shareLinksLoaded && hasShareLinks ? `<div class="settings-actions settings-actions-fill settings-share-bulk-actions" id="settings-share-bulk-actions">
        <button class="btn" id="settings-copy-all-shares-btn" type="button">${escapeHTML(settingsText('shares.copyAll'))}</button>
        <button class="btn" id="settings-delete-all-shares-btn" type="button">${escapeHTML(settingsText('shares.deleteAll'))}</button>
      </div>` : ''}
    </div>
  </section>
  ` : ''}
</div>`;

  bindSettingsPanelToggles(host);
  state.settingsMounted = true;
  if (typeof bindEchoFrameSettings === 'function') bindEchoFrameSettings(host);
  state.settingsRenderMode = 'user';

  syncActiveLibrarySelect();
  if ($('#settings-thumbnail-dir')) $('#settings-thumbnail-dir').value = state.serverSettings.thumbnail_dir || '';
  if ($('#settings-trash-dir')) $('#settings-trash-dir').value = state.serverSettings.trash_dir || '';
  const playerKeymapTextarea = $('#settings-player-keymap');
  if (playerKeymapTextarea) {
    playerKeymapTextarea.value = state.playerKeymapSource || '';
    autoGrowTextarea(playerKeymapTextarea);
  }
  autoGrowTextarea($('#settings-local-update-config'));
  syncRangeProgress(host);

  bindBatchLibraryAddShortcut();
  updateBatchLibraryAddButton();
  $('#settings-add-library-btn')?.addEventListener('click', e => {
    if (e.altKey || state.libraryBatchAddModifierHeld) openLibraryBatchEditorModal(bindLibraryRow);
    else openLibraryEditorModal(null, bindLibraryRow);
  });
  if (canAccessBatchWorkflowUI) syncLibraryBatchWorkflowPanel();

  function bindLibraryRow(row) {
    addUniversalLongPress(row, async () => {
      if (row.dataset.libraryAvailable === 'false') return;
      const libraryName = String(row.dataset.libraryName || $('.settings-library-name', row)?.value || settingsText('libraries.thisLibrary')).trim() || settingsText('libraries.thisLibrary');
      const confirmed = await appConfirm(settingsText('libraries.switchConfirm', { name: libraryName }), {
        title: settingsText('libraries.switchTitle'),
        confirmText: copyText('app.interface.s_2f116b7d1c'),
      });
      if (!confirmed) return;
      row._suppressNextClick = true;
      setTimeout(() => { row._suppressNextClick = false; }, 280);
      void switchLibraryRowImmediately(row);
    });
    row.addEventListener('dragstart', e => {
      if (row.dataset.libraryAvailable === 'false') {
        e.preventDefault();
        return;
      }
      if (e.target.closest('button, input, textarea, select')) {
        e.preventDefault();
        return;
      }
      draggedLibraryRow = row;
      libraryDragChanged = false;
      row.classList.add('dragging');
      if (e.dataTransfer) {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', row.dataset.libraryIndex || '');
      }
    });
    row.addEventListener('dragend', () => {
      if (draggedLibraryRow === row) draggedLibraryRow = null;
      if (libraryDragChanged) {
        commitLibraryRowOrderChange();
        libraryDragChanged = false;
        row._suppressNextClick = true;
        setTimeout(() => { row._suppressNextClick = false; }, 120);
      }
      row.classList.remove('dragging');
      $$('.settings-library-row.drag-over').forEach(item => item.classList.remove('drag-over'));
    });
    row.addEventListener('dragover', e => {
      if (!draggedLibraryRow || draggedLibraryRow === row) return;
      e.preventDefault();
      const list = $('#settings-library-list');
      if (!list) return;
      $$('.settings-library-row.drag-over').forEach(item => item.classList.remove('drag-over'));
      const previousSibling = draggedLibraryRow.previousElementSibling;
      const previousParent = draggedLibraryRow.parentElement;
      const rect = row.getBoundingClientRect();
      const columns = getComputedStyle(list).gridTemplateColumns.split(' ').filter(Boolean).length;
      const insertAfter = columns > 1
        ? e.clientX > rect.left + rect.width / 2
        : e.clientY > rect.top + rect.height / 2;
      const reference = insertAfter ? row.nextElementSibling : row;
      if (reference !== draggedLibraryRow) list.insertBefore(draggedLibraryRow, reference);
      if (draggedLibraryRow.parentElement !== previousParent || draggedLibraryRow.previousElementSibling !== previousSibling) libraryDragChanged = true;
      row.classList.add('drag-over');
    });
    row.addEventListener('dragleave', () => row.classList.remove('drag-over'));
    row.addEventListener('drop', e => {
      if (!draggedLibraryRow) return;
      e.preventDefault();
      row.classList.remove('drag-over');
      const movedRow = draggedLibraryRow;
      movedRow.classList.remove('dragging');
      draggedLibraryRow = null;
      if (libraryDragChanged) {
        commitLibraryRowOrderChange();
        libraryDragChanged = false;
        movedRow._suppressNextClick = true;
        row._suppressNextClick = true;
        setTimeout(() => {
          movedRow._suppressNextClick = false;
          row._suppressNextClick = false;
        }, 120);
      }
    });
    row.addEventListener('click', async e => {
      if (row._suppressNextClick) return;
      if (e.target.closest('button, input, textarea, select')) return;
      openLibraryEditorModal(row, bindLibraryRow);
    });
    row.addEventListener('contextmenu', e => {
      if (e.target.closest('button, input, textarea, select')) return;
      e.preventDefault();
      showLibraryRowContextMenu(e.clientX, e.clientY, row);
    });
    $$('.settings-library-name, .settings-library-path, .settings-library-accent', row).forEach(input => {
      input.addEventListener('input', () => {
        buildLibraryLogoPreview(row);
        syncActiveLibrarySelect();
        applyLibraryBranding();
        setSettingsDirty();
      });
    });
    buildLibraryLogoPreview(row);
  }
  if (canManageLibrariesUI) {
    $$('.settings-library-row').forEach(bindLibraryRow);
    reindexLibrarySettingsRows();
  }
  $('#settings-active-library')?.addEventListener('change', e => {
    const activeLibrary = resolveActiveLibrary(collectLibraryDrafts(), e.target.value, state.serverSettings.storage_path || '');
    state.serverSettings.active_library_id = activeLibrary && activeLibrary.id || '';
    state.serverSettings.storage_path = activeLibrary && activeLibrary.path || '';
    applyLibraryBranding();
    setSettingsDirty();
  });
  $('#settings-port')?.addEventListener('input', () => setSettingsDirty());
  $('#settings-thumbnail-dir')?.addEventListener('input', () => setSettingsDirty());
  $('#settings-trash-dir')?.addEventListener('input', () => setSettingsDirty());
  $('#settings-grid-gap').addEventListener('input', e => {
    setGridGap(e.target.value);
    $('#settings-grid-gap-value').textContent = `${state.gridGap}px`;
    setSettingsDirty();
  });
  $('#settings-thumb-radius').addEventListener('input', e => {
    setThumbRadius(e.target.value);
    $('#settings-thumb-radius-value').textContent = `${state.thumbRadius}px`;
    setSettingsDirty();
  });
  $('#settings-low-resource-mode')?.addEventListener('change', e => {
    state.serverSettings.low_resource_mode = !!e.target.checked;
    setSettingsDirty();
  });
  $('#settings-warm-enabled')?.addEventListener('change', e => {
    state.serverSettings.warm_enabled = !!e.target.checked;
    setSettingsDirty();
  });
  $('#settings-lightbox-ui-idle-seconds')?.addEventListener('input', e => {
    state.serverSettings.lightbox_ui_idle_seconds = normalizeLightboxUIIdleSeconds(e.target.value, 0);
    $('#settings-lightbox-ui-idle-seconds-value').textContent = state.serverSettings.lightbox_ui_idle_seconds === 0
      ? settingsText('display.uiIdleTap')
      : settingsText('display.seconds', { count: state.serverSettings.lightbox_ui_idle_seconds });
    updateRangeProgress(e.target);
    refreshLightboxUiActivity();
    setSettingsDirty();
  });
  $('#settings-slideshow-interval').addEventListener('input', e => {
    updateSlideshowSetting('interval', parseInt(e.target.value, 10) * 1000);
    $('#settings-slideshow-interval-value').textContent = copyText('app.interface.commonSeconds', { v0: (state.slideshowInterval / 1000) });
    setSettingsDirty();
  });
  $('#settings-slideshow-loop').addEventListener('change', e => { updateSlideshowSetting('loop', e.target.checked); setSettingsDirty(); });
  $('#settings-slideshow-mode').addEventListener('change', e => { updateSlideshowSetting('mode', e.target.value); setSettingsDirty(); });
  $('#settings-player-keymap').addEventListener('input', () => setSettingsDirty());
  $('#settings-reset-keymap-btn').addEventListener('click', async () => {
    try {
      await resetPlayerKeymapToDefault();
      setSettingsDirty();
      showToast(copyText('app.interface.s_efb41280d1'));
    } catch (e) {
      alert(copyText('app.interface.s_0820e77972') + ((e && e.error) || e));
    }
  });
  $('#settings-autoplay-video').addEventListener('change', e => { setExperimentalSetting('autoplayVideo', e.target.checked); setSettingsDirty(); });
  $('#settings-video-autoplay-next').addEventListener('change', e => { state.videoAutoplayNext = !!e.target.checked; setSettingsDirty(); });
  $('#settings-video-section-min').addEventListener('input', e => {
    state.videoSectionMinMinutes = Math.min(240, Math.max(1, Number(e.target.value) || 10));
    $('#settings-video-section-min-value').textContent = copyText('app.interface.d_41d1498dab', { v0: (state.videoSectionMinMinutes) });
    setSettingsDirty();
    updateVideoBookmarkProgress();
  });
  $('#settings-throttled-video-seek').addEventListener('change', e => {
    state.throttledVideoSeek = !!e.target.checked;
    renderSettingsContent();
    setSettingsDirty();
  });
  $('#settings-video-seek-throttle-ms')?.addEventListener('input', e => {
    state.videoSeekThrottleMS = normalizeVideoSeekThrottleMS(e.target.value, 240);
    $('#settings-video-seek-throttle-ms-value').textContent = `${state.videoSeekThrottleMS} ms`;
    updateRangeProgress(e.target);
    setSettingsDirty();
  });
  $('#settings-video-volume-swipe-sensitivity')?.addEventListener('input', e => {
    state.videoVolumeSwipeSensitivity = normalizeVideoVolumeSwipeSensitivity(e.target.value, 100);
    $('#settings-video-volume-swipe-sensitivity-value').textContent = `${state.videoVolumeSwipeSensitivity}%`;
    updateRangeProgress(e.target);
    setSettingsDirty();
  });
  $('#settings-video-volume-min-percent')?.addEventListener('input', e => {
    state.videoVolumeMinPercent = normalizeVideoVolumePercent(e.target.value, 0);
    if (state.videoVolumeMaxPercent < state.videoVolumeMinPercent) state.videoVolumeMaxPercent = state.videoVolumeMinPercent;
    $('#settings-video-volume-min-percent-value').textContent = `${state.videoVolumeMinPercent}%`;
    const maxInput = $('#settings-video-volume-max-percent');
    if (maxInput) {
      maxInput.value = String(state.videoVolumeMaxPercent);
      updateRangeProgress(maxInput);
    }
    $('#settings-video-volume-max-percent-value').textContent = `${state.videoVolumeMaxPercent}%`;
    applyVideoVolumeOutput($('#lb-video'), effectiveVideoVolume($('#lb-video')));
    setSettingsDirty();
  });
  $('#settings-video-volume-max-percent')?.addEventListener('input', e => {
    state.videoVolumeMaxPercent = Math.max(state.videoVolumeMinPercent, normalizeVideoVolumePercent(e.target.value, 100));
    e.target.value = String(state.videoVolumeMaxPercent);
    $('#settings-video-volume-max-percent-value').textContent = `${state.videoVolumeMaxPercent}%`;
    updateRangeProgress(e.target);
    applyVideoVolumeOutput($('#lb-video'), effectiveVideoVolume($('#lb-video')));
    setSettingsDirty();
  });
  $('#settings-continue-last-video-position').addEventListener('change', e => { state.continueLastVideoPosition = !!e.target.checked; setSettingsDirty(); });
  $('#settings-prefetch-neighbors').addEventListener('change', e => { setExperimentalSetting('prefetchNeighbors', e.target.checked); setSettingsDirty(); });
  if (activate && canRunServerTasksUI) {
    void pollVideoThumbnailRefreshStatus();
    void pollEXIFBackfillStatus();
  }
  $('#settings-refresh-video-thumbs-btn')?.addEventListener('click', () => {
    openVideoThumbnailRefreshWorkflow();
  });
  $('#settings-playback-cache-btn')?.addEventListener('click', openPlaybackCacheModal);
  $('#settings-backfill-exif-btn')?.addEventListener('click', async () => {
    try {
      const status = await repairEXIFMetadata();
      state.exifBackfillStatus = status || { status: 'idle', message: copyText('app.interface.s_f53844d2ff') };
      state.exifBackfillCancelPending = false;
      syncEXIFBackfillOverlay(state.exifBackfillStatus);
      if (isEXIFBackfillActive(state.exifBackfillStatus)) {
        stopEXIFBackfillPolling();
        state.exifBackfillPollTimer = setTimeout(pollEXIFBackfillStatus, 300);
      } else if (state.exifBackfillStatus.status === 'completed') {
        showToast(copyText('app.interface.s_1d309c430f'));
      }
    } catch (e) {
      alert(copyText('app.interface.s_9b8b568f8c') + ((e && e.error) || e));
    }
  });
  const restoreLastViewToggle = $('#settings-exp-restore-last-view');
  if (restoreLastViewToggle) {
    restoreLastViewToggle.addEventListener('change', e => { setExperimentalSetting('restoreLastView', e.target.checked); setSettingsDirty(); });
  }
  const syncPWASettings = () => {
    state.pwaSettings = normalizePWASettings({
      name: currentAppDisplayName(),
      iconSource: $('#settings-pwa-icon-source')?.value,
      themeMode: $('#settings-pwa-theme-mode')?.value,
      themeColor: $('#settings-pwa-theme-color')?.value,
      startPage: state.pwaSettings && state.pwaSettings.startPage,
      cacheStrategy: $('#settings-pwa-cache-strategy')?.value,
      uploadedIconURL: state.pwaSettings && state.pwaSettings.uploadedIconURL || state.serverSettings.pwa_icon_url || '',
    });
    const colorInput = $('#settings-pwa-theme-color');
    if (colorInput) colorInput.disabled = state.pwaSettings.themeMode !== 'fixed';
    persistPWASettings();
    applyLibraryBranding();
  };
  $('#settings-gallery-name')?.addEventListener('input', () => {
    state.serverSettings.app_name = $('#settings-gallery-name').value.trim() || 'EchoGallery';
    window.__GALLERY_NAME__ = state.serverSettings.app_name;
    state.pwaSettings = normalizePWASettings({ ...state.pwaSettings, name: state.serverSettings.app_name });
    persistPWASettings();
    applyLibraryBranding();
    setSettingsDirty();
  });
  $('#settings-locale')?.addEventListener('change', event => {
    if (!window.EchoGalleryI18n || !window.EchoGalleryI18n.setLocale(event.target.value)) {
      showToast(settingsText('appConfig.languageChangeFailed'));
    }
  });
  $('#settings-pwa-icon-source')?.addEventListener('change', syncPWASettings);
  $('#settings-pwa-theme-mode')?.addEventListener('change', syncPWASettings);
  $('#settings-pwa-theme-color')?.addEventListener('input', syncPWASettings);
  $('#settings-pwa-cache-strategy')?.addEventListener('change', syncPWASettings);
  $('#settings-pwa-icon-upload-btn')?.addEventListener('click', () => {
    $('#settings-pwa-icon-file')?.click();
  });
  $('#settings-pwa-icon-file')?.addEventListener('change', async e => {
    const input = e.currentTarget;
    const file = input && input.files && input.files[0];
    if (!file) return;
    openLibraryLogoGuideModal(file, async croppedFile => {
      const btn = $('#settings-pwa-icon-upload-btn');
      try {
        const resp = await withButtonBusy(btn, renderButtonBusySpinner(), () => uploadPWAIcon(croppedFile), { ariaLabel: settingsText('pwa.uploadBusy') });
        const source = $('#settings-pwa-icon-source');
        if (source) source.value = 'upload';
        syncPWASettings();
        showToast((resp && resp.message) || settingsText('pwa.uploadSuccess'));
        renderSettingsContent();
      } catch (err) {
        alert(copyText('app.interface.s_412a25b4d6') + ((err && err.error) || err));
      } finally {
        input.value = '';
      }
    }, () => {
      input.value = '';
    });
  });
  $('#settings-update-local-enabled')?.addEventListener('change', e => {
    state.updateSettings.localEnabled = !!e.target.checked;
    state.localUpdateCheckReady = false;
    state.localUpdateHasUpdate = false;
    const runBtn = $('#settings-run-update-btn');
    if (runBtn) runBtn.disabled = true;
    persistUpdateSettings();
    renderSettingsContent();
  });
  $('#settings-update-github-enabled')?.addEventListener('change', e => {
    state.updateSettings.githubEnabled = !!e.target.checked;
    state.localUpdateCheckReady = false;
    state.localUpdateHasUpdate = false;
    const runBtn = $('#settings-run-update-btn');
    if (runBtn) runBtn.disabled = true;
    persistUpdateSettings();
    renderSettingsContent();
  });
  $('#settings-local-update-config')?.addEventListener('input', e => {
    state.serverSettings.local_update_config_text = e.target.value;
    state.localUpdateCheckReady = false;
    state.localUpdateHasUpdate = false;
    const runBtn = $('#settings-run-update-btn');
    if (runBtn) runBtn.disabled = true;
    autoGrowTextarea(e.target);
  });
  $('#settings-player-keymap')?.addEventListener('input', e => {
    autoGrowTextarea(e.target);
  });
  $('#settings-check-update-btn')?.addEventListener('click', async e => {
    const btn = e.currentTarget;
    try {
      const text = $('#settings-local-update-config')?.value || state.serverSettings.local_update_config_text || '';
      const saved = await saveLocalUpdateConfig(text);
      state.serverSettings.local_update_config_text = saved.text || text;
      const result = await withButtonBusy(btn, settingsText('update.checking'), () => checkLocalUpdate());
      state.localUpdateCheckReady = true;
      state.localUpdateHasUpdate = !!(result && result.has_update);
      const runBtn = $('#settings-run-update-btn');
      if (runBtn) runBtn.disabled = !state.localUpdateHasUpdate;
      showToast(result.has_update ? (result.message || settingsText('update.updateFound')) : (result.message || settingsText('update.upToDate')), 3200);
    } catch (err) {
      state.localUpdateCheckReady = false;
      state.localUpdateHasUpdate = false;
      const runBtn = $('#settings-run-update-btn');
      if (runBtn) runBtn.disabled = true;
      alert(copyText('app.interface.s_b441bb977f') + ((err && err.error) || err));
    }
  });
  $('#settings-run-update-btn')?.addEventListener('click', async e => {
    const btn = e.currentTarget;
    if (!state.localUpdateCheckReady || !state.localUpdateHasUpdate) return;
    try {
      const text = $('#settings-local-update-config')?.value || state.serverSettings.local_update_config_text || '';
      const saved = await saveLocalUpdateConfig(text);
      state.serverSettings.local_update_config_text = saved.text || text;
      const result = await withButtonBusy(btn, settingsText('update.running'), () => applyLocalUpdate());
      state.localUpdateCheckReady = !!(result && result.has_update);
      state.localUpdateHasUpdate = false;
      const runBtn = $('#settings-run-update-btn');
      if (runBtn) runBtn.disabled = true;
      showToast(result.message || (result.updated > 0 ? settingsText('update.updatedPrograms', { count: result.updated }) : settingsText('update.upToDate')), 3600);
    } catch (err) {
      alert(copyText('app.interface.s_83c4ee7eb6') + ((err && err.error) || err));
    }
  });
  $('#theme-btn')?.addEventListener('click', e => { e.preventDefault(); toggleTheme(); });
  $('#settings-export-config-btn')?.addEventListener('click', async e => {
    try {
      await withButtonBusy(e.currentTarget, renderButtonBusySpinner(), exportPortableConfig, { ariaLabel: copyText('app.interface.s_5065f5a636') });
    } catch (err) {
      alert(copyText('app.interface.s_8e63e8094b') + ((err && err.error) || err.message || err));
    }
  });
  $('#settings-import-config-btn')?.addEventListener('click', () => $('#settings-import-config-file')?.click());
  $('#settings-import-config-file')?.addEventListener('change', async e => {
    const input = e.currentTarget;
    const file = input?.files?.[0];
    if (!file) return;
    try {
      const confirmed = await appConfirm(copyText('app.interface.s_6b11a177ae'), {
        title: copyText('app.interface.s_52da5e65e3'), confirmText: copyText('app.interface.s_74682b66b0'), cancelText: copyText('app.interface.commonCancel'),
      });
      if (!confirmed) return;
      const button = $('#settings-import-config-btn');
      const result = await withButtonBusy(button, renderButtonBusySpinner(), () => importPortableConfig(file), { ariaLabel: copyText('app.interface.s_79159ea3c7') });
      applyPortableBrowserSettings(result.browser_settings);
      showToast(result.message || copyText('app.interface.s_51f8d427d8'));
      if (result.auto_restarting) {
        setTimeout(() => { location.href = '/login'; }, 1800);
      } else {
        alert(copyText('app.interface.s_476569b3a2'));
      }
    } catch (err) {
      alert(copyText('app.interface.s_11a636597f') + ((err && err.error) || err.message || err));
    } finally {
      input.value = '';
    }
  });
  if (activate && canRunServerTasksUI) void pollThumbnailBuildStatus();
  $('#shutdown-btn')?.addEventListener('click', e => { e.preventDefault(); shutdownFromUI(); });
  $('#logout-btn')?.addEventListener('click', e => { e.preventDefault(); logout(); });
  refreshShareManagementUI();
  applyLibraryBranding();
  if (state.settingsFocus === 'libraries') {
    const target = $('#settings-library-list');
    if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    state.settingsFocus = '';
  }
}

// ── 时间线视图 ─────────────────────────────────────────
function renderSelectionBarMarkup({ barID = 'sel-bar', countID = 'sel-count', clearButtonID = 'clear-sel-btn', extraAction = '', actions = '', includeDeselectFavorite = true } = {}) {
  const downloadLabel = copyText('app.interface.d_45360bd7f1', { v0: (mediaArchiveSaveLabel()) });
  const deselectFavoriteLabel = copyText('app.interface.s_3e6dbf75b7');
  const defaultActions = `
    <button class="btn-icon" id="download-sel-btn" type="button" title="${escapeHTML(downloadLabel)}" aria-label="${escapeHTML(downloadLabel)}">${icons.selectionDownload}</button>
    ${includeDeselectFavorite ? `<button class="btn-icon" id="deselect-favorite-sel-btn" type="button" title="${deselectFavoriteLabel}" aria-label="${deselectFavoriteLabel}">${icons.selectionDeselectFavorite || icons.deselectFav || icons.favorite}</button>` : ''}`;
  return `<span id="${barID}" class="selected-bar">
    <button class="btn-icon" id="${clearButtonID}" type="button" title="${escapeHTML(copyText('app.interface.selectionCancel'))}" aria-label="${escapeHTML(copyText('app.interface.selectionCancel'))}">${icons.selectionClear}</button>
    <span class="selected-count" id="${countID}" aria-label="${escapeHTML(copyText('app.interface.runtime.selectedCountAria'))}">0</span>
    ${actions || defaultActions}
    ${extraAction}
  </span>`;
}

function albumAddIconMarkup() {
  const dedicatedIcon = String(icons.albumContextAdd || '');
  if (/<(?:path|circle|rect|line|polyline|polygon)\b/i.test(dedicatedIcon)) return dedicatedIcon;
  return icons.contextAlbum || icons.album || '';
}

function renderAddToAlbumSelectionAction() {
  const label = copyText('app.interface.albumAddToAlbum');
  return `<button class="btn-icon" id="add-to-album-sel-btn" type="button" title="${label}" aria-label="${label}">${albumAddIconMarkup()}</button>`;
}

function bindSelectionBarHandlers({ extraButtonID = '', extraAction, actionBindings = [], barID = 'sel-bar', countID = 'sel-count', clearButtonID = 'clear-sel-btn' } = {}) {
  $(`#${clearButtonID}`)?.addEventListener('click', clearSelection);
  $('#download-sel-btn')?.addEventListener('click', downloadSelected);
  $('#deselect-favorite-sel-btn')?.addEventListener('click', deselectFavoriteSelected);
  if (extraButtonID && typeof extraAction === 'function') {
    $(`#${extraButtonID}`)?.addEventListener('click', extraAction);
  }
  actionBindings.forEach(({ id, action }) => {
    if (id && typeof action === 'function') $(`#${id}`)?.addEventListener('click', action);
  });
  updateSelectionBar({ barID, countID });
}

function renderTrashSelectionBarMarkup() {
  return renderSelectionBarMarkup({
    barID: 'trash-sel-bar',
    countID: 'trash-sel-count',
    clearButtonID: 'trash-clear-sel-btn',
    actions: `
      <button class="btn-icon" id="restore-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.batchRestore'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.batchRestore'))}">${icons.topbarRestoreSelected}</button>
      <button class="btn-icon" id="hard-delete-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.s_40e1ba2e1e'))}" aria-label="${escapeHTML(copyText('app.interface.s_40e1ba2e1e'))}">${icons.selectionDelete}</button>`,
    extraAction: '',
  });
}

function renderTrashFolderSelectionBarMarkup() {
  return renderSelectionBarMarkup({
    barID: 'trash-folder-sel-bar',
    countID: 'trash-folder-sel-count',
    clearButtonID: 'trash-folder-clear-sel-btn',
    actions: `
      <button class="btn-icon" id="restore-folder-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.restoreFolder'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.restoreFolder'))}">${icons.topbarRestoreSelected}</button>
      <button class="btn-icon" id="hard-delete-folder-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.deleteFolder'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.deleteFolder'))}">${icons.selectionDelete}</button>`,
    extraAction: '',
  });
}

function bindTrashSelectionBarHandlers() {
  bindSelectionBarHandlers({
    barID: 'trash-sel-bar',
    countID: 'trash-sel-count',
    clearButtonID: 'trash-clear-sel-btn',
    actionBindings: [
      { id: 'restore-sel-btn', action: restoreSelected },
      { id: 'hard-delete-sel-btn', action: hardDeleteSelected },
    ],
  });
  $('#trash-folder-clear-sel-btn')?.addEventListener('click', clearSelection);
  $('#restore-folder-sel-btn')?.addEventListener('click', restoreSelectedTrashFolders);
  $('#hard-delete-folder-sel-btn')?.addEventListener('click', hardDeleteSelectedTrashFolders);
}

async function renderTimeline() {
  state.timelineAutoLoadPaused = false;
  const writable = canWriteMedia();
  $('#topbar-title').textContent = copyText('app.interface.s_4f8ef92599');
  $('#topbar-leading').innerHTML = renderTimelineOrderControl();
  $('#topbar-meta').innerHTML = renderSelectionBarMarkup({
    extraAction: writable ? `${renderAddToAlbumSelectionAction()}<button class="btn-icon" id="delete-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.deleteSelectedMedia'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.deleteSelectedMedia'))}">${icons.selectionDelete}</button>` : '',
  }) + `<span class="timeline-jump-status" id="timeline-jump-status" hidden></span>` + renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = `<div class="topbar-action-group">${renderTopbarGlassButton({ id: 'timeline-load-all-btn', icon: icons.topbarLoadAll, label: copyText('app.interface.s_118c2fd3f5') })}${writable ? renderTopbarGlassButton({ id: 'upload-btn', icon: icons.topbarUpload, label: copyText('app.interface.s_a977650db1'), variant: 'accent' }) : ''}</div>`;
  bindMediaKindFilterControl();
  bindSelectionBarHandlers({
    extraButtonID: writable ? 'delete-sel-btn' : '',
    extraAction: writable ? deleteSelected : null,
    actionBindings: writable ? [{ id: 'add-to-album-sel-btn', action: () => openAlbumPickerModal(null) }] : [],
  });
  $('#timeline-order-btn').addEventListener('click', async () => {
    const nextOrder = state.timelineOrder === 'asc' ? 'desc' : 'asc';
    const nextLabel = nextOrder === 'asc' ? copyText('app.interface.s_b284a0bd6c') : copyText('app.interface.s_923b57ad06');
    if (!(await appConfirm(copyText('app.interface.d_8714d3085b', { v0: (nextLabel) }), {
      title: copyText('app.interface.s_4b553446cb'),
      confirmText: copyText('app.interface.s_2f116b7d1c'),
      cancelText: copyText('app.interface.commonCancel'),
    }))) return;
    state.timelineOrder = nextOrder;
    localStorage.setItem(timelineOrderStorageKey, state.timelineOrder);
    state.photos = [];
    state.timelineCursor = '';
    state.timelineHasMore = true;
    state.timelineTotal = 0;
    state.timelineLoaded = false;
    state.viewScrollPositions[viewScrollKeyFor('timeline')] = 0;
    switchView('timeline');
  });
  $('#timeline-load-all-btn').addEventListener('click', openTimelineLoadAllModal);
  $('#upload-btn')?.addEventListener('click', openUploadModal);

  $('#content').innerHTML = `
<div id="timeline-wrap"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>${copyText('app.interface.commonLoading')}</div>`;

  const hasPendingFocus = !!state.pendingTimelinePhotoID;
  const canReuseLoadedTimelineForPendingFocus = hasPendingFocus
    && state.timelineLoaded
    && state.photos.some(photo => Number(photo && photo.id) === Number(state.pendingTimelinePhotoID));

  if (canReuseLoadedTimelineForPendingFocus) {
    state.timelineLocatingActive = false;
    await loadShareMap();
    renderTimelineGrid();
    requestVisibleThumbnailWarmup('timeline', state.photos);
    observeLoadMore('load-more-top', loadMoreTimelineBefore, () => state.timelineHasBefore && !timelineUsesManualBeforeLoading() && !state.timelineLoadingBefore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused, { rootMargin: '50% 0px 0px 0px' });
    observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
    if (!(await focusPendingTimelinePhoto())) restoreViewScroll('timeline');
    return;
  }

  if (hasPendingFocus) {
    try {
      state.timelineLocatingActive = true;
      const located = await loadTimelineLocateWindow(state.pendingTimelinePhotoID);
      state.photos = Array.isArray(located && located.photos) ? located.photos : [];
      state.timelineLocateWindow = located || null;
      state.timelineLocateTargetIndex = Number.isFinite(Number(located && located.target_index)) ? Number(located.target_index) : -1;
      state.timelineLocateMissingBeforeCount = Math.max(0, Number(located && located.missing_before_count) || 0);
      state.timelineHasBefore = !!(located && located.has_before);
      state.timelineCursor = (located && located.next_cursor) || '';
      state.timelineHasMore = !!(located && located.has_after);
      state.timelineTotal = state.photos.length;
      state.timelineLoaded = true;
      renderTimelineGrid();
      const focusedPending = await focusPendingTimelinePhoto();
      observeLoadMore('load-more-top', loadMoreTimelineBefore, () => state.timelineHasBefore && !timelineUsesManualBeforeLoading() && !state.timelineLoadingBefore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused, { rootMargin: '50% 0px 0px 0px' });
      observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
      if (!focusedPending) restoreViewScroll('timeline');
      return;
    } catch (e) {
      console.warn('timeline locate failed, fallback to regular loading', e);
      state.timelineLocatingActive = false;
      state.timelineLocateWindow = null;
      state.timelineLocateTargetIndex = -1;
      state.timelineLocateMissingBeforeCount = 0;
      removeTimelineLocateBlocking();
    }
  }

  state.photos = [];
  state.timelineLocatingActive = false;
  state.timelineLocateWindow = null;
  state.timelineLocateTargetIndex = -1;
  state.timelineLocateMissingBeforeCount = 0;
  state.timelineHasBefore = false;
  state.timelineCursor = '';
  state.timelineHasMore = true;
  state.timelineTotal = 0;
  await loadShareMap();   // b-2: 加载分享状态
  await loadMoreTimeline();
  state.timelineLoaded = true;
  const focusedPending = await focusPendingTimelinePhoto();
  observeLoadMore('load-more-top', loadMoreTimelineBefore, () => state.timelineHasBefore && !timelineUsesManualBeforeLoading() && !state.timelineLoadingBefore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused, { rootMargin: '50% 0px 0px 0px' });
  observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
  if (hasPendingFocus && focusedPending) return;
  state.viewScrollPositions[viewScrollKeyFor('timeline')] = 0;
  window.scrollTo(0, 0);
}

function setTimelineJumpStatus(message = '') {
  const status = $('#timeline-jump-status');
  if (!status) return;
  status.textContent = message;
  status.hidden = !message;
}

function renderTimelineLoadAllModal() {
  return `<div class="modal-overlay timeline-load-all-modal" id="timeline-load-all-modal">
  <div class="modal" style="width:500px">
    <div class="modal-title">${copyText('app.interface.s_d95504a29c')}</div>
    <p class="modal-copy">${escapeHTML(copyText('app.interface.runtime.timelineLoadCopy'))}</p>
    <div class="timeline-load-progress">
      <div class="timeline-load-progress-bar" id="timeline-load-all-bar"></div>
    </div>
    <div class="timeline-load-status" id="timeline-load-all-status">${escapeHTML(copyText('app.interface.runtime.timelineLoadReady'))}</div>
    <div class="modal-footer">
      <button class="btn" id="timeline-load-all-cancel">${copyText('app.interface.commonCancel')}</button>
      <button class="btn btn-primary" id="timeline-load-all-start">${copyText('app.interface.s_26f8b9e0eb')}</button>
    </div>
  </div>
</div>`;
}

function openTimelineLoadAllModal() {
  const modal = $('#timeline-load-all-modal');
  if (!modal || state.timelineBulkLoading) return;
  if (!state.timelineHasMore) {
    showToast(copyText('app.interface.s_dcb62c8e73'));
    return;
  }
  modal.classList.add('open');
  updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal || state.photos.length || 0, copyText('app.interface.s_b978080d09'));
  $('#timeline-load-all-start').disabled = false;
  $('#timeline-load-all-start').textContent = copyText('app.interface.s_26f8b9e0eb');
  $('#timeline-load-all-cancel').disabled = false;
  $('#timeline-load-all-cancel').textContent = copyText('app.interface.commonCancel');
  $('#timeline-load-all-cancel').onclick = closeOrCancelTimelineLoadAll;
  $('#timeline-load-all-start').onclick = loadAllTimelinePhotos;
}

function closeOrCancelTimelineLoadAll() {
  if (state.timelineBulkLoading) {
    state.timelineBulkCancelRequested = true;
    if (state.timelineBulkAbortController) state.timelineBulkAbortController.abort();
    if (state.timelineLoadAbortController) state.timelineLoadAbortController.abort();
    cancelActivePreloads();
    disconnectLoadMoreObserver();
    pauseAutoLoadMore('timeline');
    armAutoLoadMoreResume('timeline');
    updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, copyText('app.interface.s_6bc0d687ae'));
    $('#timeline-load-all-cancel').disabled = true;
    return;
  }
  const modal = $('#timeline-load-all-modal');
  if (modal) modal.classList.remove('open');
}

function updateTimelineLoadAllProgress(loaded, total, message = '') {
  const bar = $('#timeline-load-all-bar');
  const status = $('#timeline-load-all-status');
  const safeLoaded = Math.max(0, Number(loaded) || 0);
  const safeTotal = Math.max(0, Number(total) || 0);
  const percent = safeTotal > 0 ? Math.min(100, Math.round((safeLoaded / safeTotal) * 100)) : 0;
  if (bar) bar.style.width = `${percent}%`;
  if (status) {
    const countText = safeTotal > 0 ? `${safeLoaded} / ${safeTotal}` : `${safeLoaded}`;
    status.textContent = message || copyText('app.interface.d_14d838fa52', { v0: (countText) });
  }
}

function preloadTimelineThumbnails(photos) {
  void preloadThumbnailsInBatches(photos || [], null, () => state.timelineBulkCancelRequested);
}

async function fetchTimelinePage(cursor, limit, signal) {
  const params = new URLSearchParams({
    limit: String(limit),
    order: state.timelineOrder === 'asc' ? 'asc' : 'desc',
  });
  appendMediaKindParam(params);
  if (cursor) params.set('cursor', cursor);
  const response = await fetchWithPageSession(`/api/media?${params.toString()}`, { signal });
  if (!response.ok) throw await buildAPIError(response);
  return response.json();
}

async function loadAllTimelinePhotos() {
  if (state.timelineBulkLoading) return;
  state.timelineBulkLoading = true;
  state.timelineBulkCancelRequested = false;
  state.blockingInteraction = true;
  disconnectLoadMoreObserver();
  $('#timeline-load-all-start').disabled = true;
  $('#timeline-load-all-start').textContent = copyText('app.interface.commonLoading');
  $('#timeline-load-all-cancel').disabled = false;
  $('#timeline-load-all-cancel').textContent = copyText('app.interface.s_43448f3e11');
  try {
    let cursor = state.timelineCursor;
    let hasMore = state.timelineHasMore;
    let loaded = state.photos.length;
    let total = state.timelineTotal || loaded;
    preloadTimelineThumbnails(state.photos);
    updateTimelineLoadAllProgress(loaded, total, copyText('app.interface.d_d91dd1833c', { v0: (loaded) }));

    while (hasMore && !state.timelineBulkCancelRequested) {
      state.timelineBulkAbortController = new AbortController();
      const page = await fetchTimelinePage(cursor, 300, state.timelineBulkAbortController.signal);
      const photos = page.photos || [];
      if (Number.isFinite(Number(page.total))) total = Number(page.total);
      state.timelineTotal = total;
      state.photos.push(...photos);
      state.timelineCursor = page.next_cursor || '';
      state.timelineHasMore = page.has_more || false;
      cursor = state.timelineCursor;
      hasMore = state.timelineHasMore;
      loaded = state.photos.length;
      appendTimelineGrid(photos);
      preloadTimelineThumbnails(photos);
      updateTimelineLoadAllProgress(loaded, total, copyText('app.interface.d_bec8ed4111', { v0: (loaded), v1: (total ? ` / ${total}` : '') }));
      const loadMore = $('#load-more');
      if (loadMore) loadMore.innerHTML = `<div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.loadingAllTimeline', { loaded, total: total ? ` / ${total}` : '' }))}`;
      await new Promise(resolve => requestAnimationFrame(resolve));
    }

    if (state.timelineBulkCancelRequested) {
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, copyText('app.interface.d_847a00d4e2', { v0: (state.photos.length) }));
      showToast(copyText('app.interface.s_41221e525c'));
    } else {
      state.timelineHasMore = false;
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal || state.photos.length, copyText('app.interface.d_e663acbda3', { v0: (state.photos.length) }));
      showToast(copyText('app.interface.s_fe80dcf306'));
      setTimeout(() => $('#timeline-load-all-modal')?.classList.remove('open'), 700);
    }
  } catch (e) {
    if (e && e.name === 'AbortError') {
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, copyText('app.interface.d_847a00d4e2', { v0: (state.photos.length) }));
      showToast(copyText('app.interface.s_41221e525c'));
    } else {
      console.error(e);
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, copyText('app.interface.s_37d3b73ac9'));
      showToast(copyText('app.interface.s_1ddabdf83e'));
    }
  } finally {
    state.timelineBulkLoading = false;
    state.timelineBulkAbortController = null;
    state.blockingInteraction = false;
    $('#timeline-load-all-start').disabled = false;
    $('#timeline-load-all-start').textContent = copyText('app.interface.s_5982c44c18');
    $('#timeline-load-all-cancel').disabled = false;
    $('#timeline-load-all-cancel').textContent = copyText('app.interface.commonClose');
    updateLoadMoreUI('load-more', state.timelineHasMore);
    if (state.view === 'timeline') {
      observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
    }
  }
}

async function renderFavorites() {
  const writable = canWriteMedia();
  $('#topbar-title').textContent = copyText('app.interface.albumFavorites');
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'download-all-favorites-btn', icon: icons.topbarDownloadFavorites, label: copyText('app.interface.d_5268dbed6c', { v0: (mediaArchiveSaveLabel()) }), variant: 'accent' }),
  ]);
  $('#topbar-meta').innerHTML = renderSelectionBarMarkup({
    includeDeselectFavorite: false,
    extraAction: writable
      ? `${renderAddToAlbumSelectionAction()}<button class="btn-icon" id="unfavorite-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.s_db0b6c0ec7'))}" aria-label="${escapeHTML(copyText('app.interface.s_db0b6c0ec7'))}">${icons.dislike || icons.favorite}</button>`
      : '',
  }) + renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = '';
  bindMediaKindFilterControl();
  bindSelectionBarHandlers({
    actionBindings: writable ? [
      { id: 'add-to-album-sel-btn', action: () => openAlbumPickerModal(null) },
      { id: 'unfavorite-sel-btn', action: unfavoriteSelected },
    ] : [],
  });

  $('#content').innerHTML = `
<div id="favorite-wrap"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>${copyText('app.interface.commonLoading')}</div>`;
  $('#download-all-favorites-btn').addEventListener('click', () => {
    void confirmDownloadAction(copyText('app.interface.d_517f6c6da1', { v0: (mediaArchiveSaveVerb()) }), () => {
      void downloadAllFavorites();
    });
  });

  await loadShareMap();
  if (state.favoriteLoaded) {
    appendFavoriteGrid(state.favoritePhotos);
    requestVisibleThumbnailWarmup('favorites', state.favoritePhotos);
    updateFavoriteTotalHint();
    updateLoadMoreUI('load-more', state.favoriteHasMore);
    observeLoadMore('load-more', loadMoreFavorites, () => state.favoriteHasMore && !state.favoriteLoading);
    restoreViewScroll('favorites');
    return;
  }

  state.favoritePhotos = [];
  state.favoriteCursor = '';
  state.favoriteHasMore = true;
  state.favoriteTotal = 0;
  updateFavoriteTotalHint();
  await loadMoreFavorites();
  observeLoadMore('load-more', loadMoreFavorites, () => state.favoriteHasMore && !state.favoriteLoading);
  restoreViewScroll('favorites');
}

async function loadMoreTimeline() {
  if (state.timelineLoading || state.timelineBulkLoading || state.timelineAutoLoadPaused || !state.timelineHasMore) return;
  state.timelineLoading = true;
  try {
    state.timelineLoadAbortController = new AbortController();
    const params = new URLSearchParams({
      limit: String(computeTimelinePageSize()),
      order: state.timelineOrder === 'asc' ? 'asc' : 'desc',
    });
    appendMediaKindParam(params);
    if (state.timelineCursor) params.set('cursor', state.timelineCursor);
    const url = `/api/media?${params.toString()}`;
    const page = await api.get(url, { signal: state.timelineLoadAbortController.signal });
    state.timelineTotal = Number.isFinite(Number(page.total)) ? Number(page.total) : state.timelineTotal;
    state.photos.push(...(page.photos || []));
    state.timelineCursor = page.next_cursor || '';
    state.timelineHasMore = page.has_more || false;
    state.timelineLoaded = true;
    appendTimelineGrid(page.photos || []);
    if (!state.timelineLocatingActive) {
      requestVisibleThumbnailWarmup('timeline', page.photos || []);
    }
  } catch (e) {
    if (!e || e.name !== 'AbortError') console.error(e);
    if (!e || e.name !== 'AbortError') {
      state.timelineHasMore = false;
      showToast(copyText('app.interface.s_6ba92a9d5d'), 5000);
    }
  }
  finally {
    state.timelineLoading = false;
    state.timelineLoadAbortController = null;
    updateLoadMoreUI('load-more', state.timelineHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
  }
}

async function loadMoreTimelineBefore() {
  if (state.timelineLoadingBefore || state.timelineLoading || state.timelineBulkLoading || state.timelineAutoLoadPaused || !state.timelineHasBefore) return;
  const firstPhoto = state.photos[0];
  if (!firstPhoto || !firstPhoto.id) {
    state.timelineHasBefore = false;
    clearTimelinePlaceholders();
    updateLoadMoreUI('load-more-top', false);
    return;
  }
  state.timelineLoadingBefore = true;
  const anchorThumb = firstVisiblePhotoThumb();
  const anchorID = anchorThumb ? Number(anchorThumb.dataset.id || 0) : 0;
  const anchorTop = anchorThumb ? anchorThumb.getBoundingClientRect().top : 0;
  try {
    const page = await loadTimelineBeforeWindow(firstPhoto.id);
    const prependPhotos = Array.isArray(page && page.photos) ? page.photos : [];
    if (!prependPhotos.length) {
      state.timelineHasBefore = false;
      clearTimelinePlaceholders();
      updateLoadMoreUI('load-more-top', false);
      return;
    }
    const existing = new Set(state.photos.map(photo => photo && photo.id));
    const uniquePrepend = prependPhotos.filter(photo => photo && !existing.has(photo.id));
    if (!uniquePrepend.length) {
      state.timelineHasBefore = !!(page && page.has_more);
      if (!state.timelineHasBefore) clearTimelinePlaceholders();
      updateLoadMoreUI('load-more-top', state.timelineHasBefore);
      return;
    }
    state.photos = [...uniquePrepend, ...state.photos];
    state.timelineHasBefore = !!(page && page.has_more);
    prependTimelineGrid(uniquePrepend);
    if (!state.timelineHasBefore) clearTimelinePlaceholders();
    void document.documentElement.offsetHeight;
    if (!state.timelineLocatingActive) {
      requestVisibleThumbnailWarmup('timeline', uniquePrepend);
    }
    const refreshedAnchor = anchorID
      ? document.querySelector(`.photo-thumb[data-id="${anchorID}"]`)
      : null;
    const nextTop = refreshedAnchor ? refreshedAnchor.getBoundingClientRect().top : 0;
    const delta = nextTop - anchorTop;
    if (delta) {
      state.ignoreScrollSaveUntil = Date.now() + 120;
      state.programmaticScrollTargetTop = (window.scrollY || window.pageYOffset || 0) + delta;
      window.scrollBy(0, delta);
    }
  } catch (e) {
    if (!e || e.name !== 'AbortError') console.error(e);
  } finally {
    state.timelineLoadingBefore = false;
    updateLoadMoreUI('load-more-top', state.timelineHasBefore);
    if (!timelineUsesManualBeforeLoading()) {
      requestAnimationFrame(() => {
        maybeTriggerTimelineBeforeLoadFromVisibleBoundary();
      });
    }
  }
}

function appendTimelineGrid(newPhotos) {
  const container = $('#timeline-wrap');
  if (!container) return;
  if (state.photos.length === 0 && newPhotos.length === 0 && state.timelineTotal === 0 && !state.timelineHasMore) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.interface.runtime.emptyMedia'))}</p></div>`;
    return;
  }
  if (newPhotos.length && container.querySelector('.empty')) container.innerHTML = '';
  let grid = $('#timeline-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'timeline-grid';
    container.appendChild(grid);
  }
  const fragment = document.createDocumentFragment();
  newPhotos.forEach(p => fragment.appendChild(makePhotoThumb(p, state.photos)));
  grid.appendChild(fragment);
}

function prependTimelineGrid(newPhotos) {
  const container = $('#timeline-wrap');
  if (!container || !newPhotos.length) return;
  let grid = $('#timeline-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'timeline-grid';
    container.appendChild(grid);
  }
  const placeholders = Array.from(grid.querySelectorAll('.timeline-placeholder-thumb'));
  const replacementCount = Math.min(placeholders.length, newPhotos.length);
  const replacementStart = Math.max(0, placeholders.length - replacementCount);
  for (let i = 0; i < replacementCount; i += 1) {
    const replacement = makePhotoThumb(newPhotos[i], state.photos);
    grid.replaceChild(replacement, placeholders[replacementStart + i]);
  }
  if (replacementCount < newPhotos.length) {
    const fragment = document.createDocumentFragment();
    newPhotos.slice(replacementCount).forEach(p => fragment.appendChild(makePhotoThumb(p, state.photos)));
    grid.insertBefore(fragment, grid.firstChild);
  }
  state.timelineLocateMissingBeforeCount = Math.max(0, timelinePlaceholderCount() - replacementCount);
}

function renderTimelineGrid() {
  const container = $('#timeline-wrap');
  if (!container) return;
  container.innerHTML = '';
  container.insertAdjacentHTML('beforeend', `<div class="load-more load-more-top" id="load-more-top"><div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.loadEarlier'))}</div>`);
  const placeholderCount = timelinePlaceholderCount();
  if (placeholderCount > 0) {
    let grid = $('#timeline-grid');
    if (!grid) {
      grid = el('div', 'photo-grid');
      grid.id = 'timeline-grid';
      container.appendChild(grid);
    }
    const placeholderFragment = document.createDocumentFragment();
    for (let i = 0; i < placeholderCount; i += 1) {
      placeholderFragment.appendChild(makeTimelinePlaceholder(i));
    }
    grid.appendChild(placeholderFragment);
  }
  if (state.photos.length) appendTimelineGrid(state.photos);
  if (state.photos.length === 0 && !state.timelineHasMore && !state.timelineLoading) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.interface.runtime.emptyMedia'))}</p></div>`;
  }
  updateLoadMoreUI('load-more-top', state.timelineHasBefore);
  updateLoadMoreUI('load-more', state.timelineHasMore);
}

function appendFavoriteGrid(newPhotos) {
  const container = $('#favorite-wrap');
  if (!container) return;
  if (state.favoritePhotos.length === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.favorite}<p>${escapeHTML(copyText('app.interface.runtime.emptyFavorites'))}</p></div>`;
    updateLoadMoreUI('load-more', false);
    return;
  }
  if (newPhotos.length && container.querySelector('.empty')) container.innerHTML = '';
  let grid = $('#favorite-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'favorite-grid';
    container.appendChild(grid);
  }
  const fragment = document.createDocumentFragment();
  newPhotos.forEach(p => fragment.appendChild(makePhotoThumb(p, state.favoritePhotos)));
  grid.appendChild(fragment);
}

function sortFavoritePhotosInPlace() {
  state.favoritePhotos.sort((a, b) => {
    const superDiff = Number(!!b.is_super_favorite) - Number(!!a.is_super_favorite);
    if (superDiff) return superDiff;
    const timeA = Date.parse(a.taken_at || a.uploaded_at || '') || 0;
    const timeB = Date.parse(b.taken_at || b.uploaded_at || '') || 0;
    if (timeA !== timeB) return timeB - timeA;
    return Number(b.id || 0) - Number(a.id || 0);
  });
}

function rerenderFavoriteGrid() {
  const container = $('#favorite-wrap');
  if (!container) return;
  sortFavoritePhotosInPlace();
  container.innerHTML = '';
  appendFavoriteGrid(state.favoritePhotos);
  renderFavoritesEmptyStateIfNeeded();
}

function updateFavoriteTotalHint() {
  const hint = $('#favorite-total-hint');
  if (!hint) return;
  hint.textContent = copyText('app.interface.d_7cb33fddb4', { v0: (state.favoriteTotal || 0) });
}

async function loadMoreFavorites() {
  if (state.favoriteLoading || !state.favoriteHasMore) return;
  state.favoriteLoading = true;
  try {
    const params = new URLSearchParams();
    appendMediaKindParam(params);
    if (state.favoriteCursor) params.set('cursor', state.favoriteCursor);
    const query = params.toString();
    const url = '/api/media/favorites' + (query ? `?${query}` : '');
    const page = await api.get(url);
    const photos = page.photos || [];
    state.favoriteTotal = Number.isFinite(Number(page.total)) ? Number(page.total) : state.favoritePhotos.length + photos.length;
    updateFavoriteTotalHint();
    state.favoritePhotos.push(...photos);
    sortFavoritePhotosInPlace();
    state.favoriteCursor = page.next_cursor || '';
    state.favoriteHasMore = page.has_more || false;
    state.favoriteLoaded = true;
    rerenderFavoriteGrid();
    requestVisibleThumbnailWarmup('favorites', photos);
  } catch (e) {
    console.error(e);
    state.favoriteHasMore = false;
    showToast(copyText('app.interface.s_5ec59cad90'), 5000);
  } finally {
    state.favoriteLoading = false;
    updateLoadMoreUI('load-more', state.favoriteHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreFavorites, () => state.favoriteHasMore && !state.favoriteLoading);
  }
}

function loadMemoryEntries() {
  try {
    const scopedKey = currentMemoriesStorageKey();
    let raw = localStorage.getItem(scopedKey);
    if (raw == null && localStorage.getItem(memoriesMigrationStorageKey) !== '1') {
      const legacy = localStorage.getItem(memoriesStorageKey);
      if (legacy != null) {
        raw = legacy;
        localStorage.setItem(scopedKey, legacy);
        localStorage.removeItem(memoriesStorageKey);
      }
      localStorage.setItem(memoriesMigrationStorageKey, '1');
    }
    const entries = JSON.parse(raw || '[]');
    return Array.isArray(entries) ? entries.filter(entry => entry && entry.id).slice(0, 120) : [];
  } catch (_) {
    return [];
  }
}

function currentMemoriesStorageKey() {
  const username = String(state.serverSettings.current_username || 'default').trim() || 'default';
  const library = String(state.serverSettings.active_library_id || '').trim()
    || normalizePathForCompare(state.serverSettings.storage_path || '')
    || 'default';
  return `${memoriesStorageKey}:${encodeURIComponent(username)}:${encodeURIComponent(library)}`;
}

function saveMemoryEntries(entries) {
  try {
    localStorage.setItem(currentMemoriesStorageKey(), JSON.stringify((entries || []).slice(0, 120)));
    return true;
  } catch (error) {
    console.warn("保存回忆记录失败", error);
    return false;
  }
}

function rememberPhoto(photo) {
  if (!photo || !photo.id) return;
  const id = Number(photo.id);
  if (!Number.isFinite(id) || id <= 0) return;
  if (state.lightboxLastRememberedPhotoID === id) return;
  state.lightboxLastRememberedPhotoID = id;
  const viewedAt = new Date().toISOString();
  const entries = loadMemoryEntries().filter(entry => Number(entry.id) !== id);
  entries.unshift({ id, viewed_at: viewedAt });
  saveMemoryEntries(entries);
  state.memoryEntries = entries.slice(0, 120);
  if (state.memoriesLoaded) {
    state.memoryPhotos = [
      { ...photo, memory_viewed_at: viewedAt },
      ...state.memoryPhotos.filter(item => Number(item && item.id) !== id),
    ].slice(0, 120);
    if (state.view === 'memories') renderMemoryGroups();
  }
}

async function renderMemories() {
  $('#topbar-title').textContent = copyText('app.interface.s_aec50b8b6c');
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'clear-memories-btn', icon: icons.topbarClearMemories, label: copyText('app.interface.s_ed7ecdfbd9') }),
  ]);
  $('#topbar-meta').innerHTML = renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = '';
  bindMediaKindFilterControl();
  $('#clear-memories-btn')?.addEventListener('click', async () => {
    if (!(await appConfirm(copyText('app.interface.s_97edb24737'), {
      title: copyText('app.interface.s_ed7ecdfbd9'),
      confirmText: copyText('app.interface.s_7b15e5e8e7'),
      cancelText: copyText('app.interface.commonCancel'),
      danger: true,
    }))) return;
    localStorage.removeItem(currentMemoriesStorageKey());
    state.memoryEntries = [];
    state.memoryPhotos = [];
    renderMemories();
  });
  $('#content').innerHTML = `<div id="memory-groups"></div>`;
  if (state.memoriesLoaded) {
    renderMemoryGroups();
    restoreViewScroll('memories');
    return;
  }
  state.memoriesLoading = true;
  state.memoryEntries = loadMemoryEntries();
  await loadShareMap();
  const photos = [];
  const liveEntries = [];
  for (const entry of state.memoryEntries) {
    try {
      const photo = await api.get(`/api/media/${entry.id}`);
      if (photo && photo.id) {
        photo.memory_viewed_at = entry.viewed_at;
        photos.push(photo);
        liveEntries.push(entry);
      }
    } catch (_) {
      // 媒体可能已被删除或移动到回收站，静默剔除历史项。
    }
  }
  state.memoryEntries = liveEntries;
  state.memoryPhotos = photos;
  state.memoriesLoaded = true;
  saveMemoryEntries(liveEntries);
  renderMemoryGroups();
  restoreViewScroll('memories');
  state.memoriesLoading = false;
}

function renderMemoryGroups() {
  const container = $('#memory-groups');
  if (!container) return;
  const photos = state.memoryPhotos.filter(mediaKindMatchesFilter);
  if (!photos.length) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.interface.runtime.emptyMemories'))}</p></div>`;
    return;
  }
  container.innerHTML = '';
  const grid = el('div', 'photo-grid');
  photos.forEach(photo => grid.appendChild(makePhotoThumb(photo, photos)));
  container.appendChild(grid);
}

function isVideoMedia(photo) {
  return photo && photo.media_kind === 'video';
}

function mediaExtension(photo) {
  const name = String((photo && photo.original_name) || '').toLowerCase();
  const dot = name.lastIndexOf('.');
  return dot >= 0 ? name.slice(dot) : '';
}

function requiresSystemPlayer(photo) {
  if (!isVideoMedia(photo)) return false;
  const ext = mediaExtension(photo);
  if (browserUnsupportedVideoExtensions.includes(ext)) return true;
  const mime = String((photo && photo.mime_type) || '').toLowerCase();
  return mime.includes('x-ms-wmv') || mime.includes('x-ms-wma');
}
function requiresBrowserPlaybackConversion(photo) {
  if (!isVideoMedia(photo)) return false;
  const ext = mediaExtension(photo);
  const mime = String((photo && photo.mime_type) || '').toLowerCase();
  if (['.mkv', '.avi', '.wmv', '.wma', '.mpg', '.mpeg', '.ts', '.mts', '.m2ts', '.ogv'].includes(ext)) return true;
  return mime.includes('matroska') || mime.includes('x-ms-wmv') || mime.includes('x-ms-wma') || mime.includes('x-msvideo');
}

function mediaThumbURL(photo) {
  if (!photo) return '';
  const preferred = isVideoMedia(photo)
    ? (photo.poster_url || photo.thumbnail_url)
    : photo.thumbnail_url;
  return withPageSessionURL(preferred || `/media/thumbnails/${photo.uuid}`);
}

function mediaFileURL(photo) {
  if (photo && photo.file_url) return withPageSessionURL(photo.file_url);
  return withPageSessionURL(isVideoMedia(photo) ? `/media/files/${photo.uuid}` : `/media/photos/${photo.uuid}`);
}

function mediaPlaybackURL(photo) {
  if (photo && photo.playback_url) return withPageSessionURL(photo.playback_url);
  return withPageSessionURL(isVideoMedia(photo) ? `/media/playback/${photo.uuid}` : mediaFileURL(photo));
}

function shouldAutoplayVideoInLightbox(photo) {
  if (!photo || !isVideoMedia(photo)) return false;
  return !!(state.slideshowPlaying || state.experimentalAutoplayVideo || isWindowsClient());
}

function mediaThumbURLFromUUID(uuid) {
  return withPageSessionURL(`/media/thumbnails/${uuid}`);
}

function computeTimelinePageSize() {
  const viewportWidth = Math.max(window.innerWidth || 0, 320);
  const viewportHeight = Math.max(window.innerHeight || 0, 480);
  const rootStyles = getComputedStyle(document.documentElement);
  const gridSize = Math.max(72, Number.parseFloat(rootStyles.getPropertyValue('--grid-size')) || state.gridSize || 180);
  const gap = Math.max(0, Number.parseFloat(rootStyles.getPropertyValue('--grid-gap')) || state.gridGap || 0);
  const contentWidth = Math.max(Math.min(viewportWidth - 32, viewportWidth), 240);
  const fixedColumns = fixedTouchGridColumns();
  const columns = fixedColumns > 0
    ? fixedColumns
    : Math.max(1, Math.floor((contentWidth + gap) / (gridSize + gap)));
  const tileSize = fixedColumns > 0
    ? Math.max(72, (contentWidth - gap * Math.max(0, columns - 1)) / columns)
    : gridSize;
  const rowHeight = Math.max(72, tileSize + gap);
  const visibleRows = Math.max(2, Math.ceil((viewportHeight + gap) / rowHeight));
  const bufferRows = 2;
  return Math.max(24, Math.min(72, columns * (visibleRows + bufferRows)));
}

function stableMediaPlaceholderColor(photo) {
  if (photo && photo.dominant_color) return String(photo.dominant_color);
  const seed = String((photo && (photo.uuid || photo.original_name || photo.id)) || 'echogallery');
  let hash = 0;
  for (let i = 0; i < seed.length; i += 1) {
    hash = ((hash << 5) - hash + seed.charCodeAt(i)) | 0;
  }
  const hue = Math.abs(hash) % 360;
  return `hsl(${hue} 20% 82%)`;
}

function videoFormatLabel(photo) {
  const ext = mediaExtension(photo).replace(/^\./, '').trim();
  if (ext) return ext.toUpperCase();
  const mime = String((photo && photo.mime_type) || '').toLowerCase();
  if (mime.includes('quicktime')) return 'MOV';
  const subtype = mime.split('/')[1]?.split(';')[0]?.trim();
  return subtype ? subtype.replace(/^x-/, '').toUpperCase() : 'VIDEO';
}

function formatDuration(durationMS) {
  const totalSeconds = Math.max(0, Math.floor((durationMS || 0) / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}
function aspectRatioLabel(width, height) {
  const w = Number(width) || 0;
  const h = Number(height) || 0;
  if (!w || !h) return '—';
  const ratio = w / h;
  const wallpaperRatios = [
    393 / 852,
    748 / 1619,
    440 / 956,
    402 / 874,
    420 / 912,
    390 / 844,
    430 / 932,
    428 / 926,
    414 / 896,
    375 / 812,
    744 / 1133,
    820 / 1180,
    834 / 1194,
  ];
  if (wallpaperRatios.some(candidate => Math.abs(Math.log(ratio / candidate)) <= 0.014 || Math.abs(Math.log(ratio / (1 / candidate))) <= 0.014)) {
    return copyText('app.interface.s_d27e504b51');
  }
  const commonRatios = [
    [1, 1],
    [4, 3],
    [3, 4],
    [3, 2],
    [2, 3],
    [16, 9],
    [9, 16],
    [16, 10],
    [10, 16],
    [5, 4],
    [4, 5],
    [18, 9],
    [21, 9],
  ];
  let best = null;
  let bestDiff = Infinity;
  commonRatios.forEach(([rw, rh]) => {
    const candidate = rw / rh;
    const diff = Math.abs(Math.log(ratio / candidate));
    if (diff < bestDiff) {
      bestDiff = diff;
      best = [rw, rh];
    }
  });
  const threshold = 0.06;
  if (best && bestDiff <= threshold) return `${best[0]}:${best[1]}`;
  const gcd = (a, b) => (b ? gcd(b, a % b) : a);
  const divisor = gcd(w, h) || 1;
  return `${Math.round(w / divisor)}:${Math.round(h / divisor)}`;
}
function compactExifValue(value) {
  const text = String(value == null ? '' : value).trim();
  return text.replace(/^"(.*)"$/, '$1').replace(/^'(.*)'$/, '$1').trim();
}
function parseExifFraction(value) {
  const text = compactExifValue(value);
  if (!text) return null;
  const parts = text.split('/');
  if (parts.length === 1) {
    const n = Number(parts[0]);
    return Number.isFinite(n) ? n : null;
  }
  const numerator = Number(parts[0]);
  const denominator = Number(parts[1]);
  if (!Number.isFinite(numerator) || !Number.isFinite(denominator) || denominator === 0) return null;
  return numerator / denominator;
}
function formatExifAperture(value) {
  const n = parseExifFraction(value);
  if (n == null) return compactExifValue(value);
  return n % 1 === 0 ? n.toFixed(0) : n.toFixed(1).replace(/0$/, '');
}
function formatExifFocalLength(value) {
  const n = parseExifFraction(value);
  if (n == null) return `${compactExifValue(value)}mm`;
  return `${n % 1 === 0 ? n.toFixed(0) : n.toFixed(1).replace(/0$/, '')}mm`;
}
function formatExifOrientation(value) {
  const orientation = Number(compactExifValue(value));
  switch (orientation) {
    case 1: return copyText('app.interface.s_5f14560d66');
    case 2: return copyText('app.interface.s_26e83874f5');
    case 3: return copyText('app.interface.s_627a0c3120');
    case 4: return copyText('app.interface.s_0c2a06739b');
    case 5: return copyText('app.interface.s_4459412cbe');
    case 6: return copyText('app.interface.s_271cc553e2');
    case 7: return copyText('app.interface.s_5175f7f85a');
    case 8: return copyText('app.interface.s_4459412cbe');
    default: return copyText('app.interface.d_9b8c903470', { v0: (compactExifValue(value) || '—') });
  }
}
function formatExifLocation(exif) {
  const address = compactExifValue(exif && (exif.location_address || exif.address));
  if (address) return address;
  if (exif && exif.has_gps && Number.isFinite(Number(exif.latitude)) && Number.isFinite(Number(exif.longitude))) {
    return `${Number(exif.latitude).toFixed(5)}, ${Number(exif.longitude).toFixed(5)}`;
  }
  return '';
}
function buildExifInfoItems(photo) {
  const exif = photo && photo.exif;
  if (!exif || typeof exif !== 'object') return [];
  const items = [];
  const camera = [compactExifValue(exif.make), compactExifValue(exif.model)].filter(Boolean).join(' ');
  if (camera) items.push([icons.exifCamera, copyText('app.interface.s_6042a0b155'), camera]);
  if (compactExifValue(exif.f_number)) items.push([icons.exifAperture, copyText('app.interface.s_dbce052d6a'), formatExifAperture(exif.f_number)]);
  if (compactExifValue(exif.exposure_time)) items.push([icons.exifShutter, copyText('app.interface.s_328df5535f'), compactExifValue(exif.exposure_time)]);
  if (Number(exif.iso_speed) > 0) items.push([icons.exifISO, 'ISO', String(Number(exif.iso_speed))]);
  if (compactExifValue(exif.focal_length)) items.push([icons.exifFocal, copyText('app.interface.s_7bf130cc12'), formatExifFocalLength(exif.focal_length)]);
  if (Number(exif.orientation) > 0) items.push([icons.exifOrientation, copyText('app.interface.s_a88d531f32'), formatExifOrientation(exif.orientation)]);
  const location = formatExifLocation(exif);
  if (location) items.push([icons.exifGPS, copyText('app.interface.s_88c34452cc'), location]);
  return items;
}

function hasInfoValue(value) {
  return value != null && String(value).trim() !== '' && String(value).trim() !== '—';
}

function compactInfoItems(items) {
  return items.filter(([, , value]) => hasInfoValue(value));
}

function formatVideoCodec(value) {
  const raw = compactExifValue(value).toLowerCase();
  if (!raw) return '';
  const names = {
    h264: 'H.264',
    avc1: 'H.264',
    x264: 'H.264',
    hevc: 'H.265 / HEVC',
    h265: 'H.265 / HEVC',
    x265: 'H.265 / HEVC',
    vp9: 'VP9',
    vp8: 'VP8',
    av1: 'AV1',
    mpeg4: 'MPEG-4 Part 2',
    mpeg2video: 'MPEG-2 Video',
    mpeg1video: 'MPEG-1 Video',
    mjpeg: 'Motion JPEG',
    prores: 'Apple ProRes',
    theora: 'Theora',
    wmv1: 'Windows Media Video 7',
    wmv2: 'Windows Media Video 8',
    wmv3: 'Windows Media Video 9',
    vc1: 'VC-1',
    dvvideo: 'DV Video',
  };
  return names[raw] || raw.toUpperCase();
}

function formatVideoFrameRate(value) {
  const rate = Number(value);
  if (!Number.isFinite(rate) || rate <= 0) return '';
  const rounded = Math.round(rate * 100) / 100;
  return `${Number.isInteger(rounded) ? rounded.toFixed(0) : rounded.toFixed(2)} fps`;
}

function normalizeVideoSeekThrottleMS(value, fallback = 240) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return Math.min(1000, Math.max(120, Number(fallback) || 240));
  return Math.min(1000, Math.max(120, Math.round(numeric)));
}

function shouldThrottleVideoSeek() {
  return !!state.throttledVideoSeek;
}

function clearPendingVideoSeekCommit() {
  if (state.pendingVideoSeekCommitTimer) {
    clearTimeout(state.pendingVideoSeekCommitTimer);
    state.pendingVideoSeekCommitTimer = null;
  }
}

function pendingVideoSeekTimeFor(video = $('#lb-video')) {
  const pending = state.pendingVideoSeek;
  if (!pending || pending.video !== video) return null;
  const time = Number(pending.time);
  return Number.isFinite(time) ? time : null;
}

function commitPendingVideoSeek(force = false) {
  const pending = state.pendingVideoSeek;
  clearPendingVideoSeekCommit();
  if (!pending || !pending.video) return false;
  const { video, time, photo } = pending;
  state.pendingVideoSeek = null;
  if (!force && (!document.contains(video) || video.classList.contains('hidden'))) return false;
  if (!Number.isFinite(Number(video.duration)) || video.duration <= 0) return false;
  const nextTime = Math.max(0, Math.min(video.duration, Number(time) || 0));
  video.currentTime = nextTime;
  updateVideoBookmarkProgress(video, photo);
  showVideoProgressActivity();
  return true;
}

function queueVideoSeekCommit(video, time, photo = state.lightboxPhotos[state.lightboxIndex]) {
  if (!video) return false;
  state.pendingVideoSeek = { video, time, photo };
  clearPendingVideoSeekCommit();
  state.pendingVideoSeekCommitTimer = setTimeout(() => {
    commitPendingVideoSeek(true);
  }, normalizeVideoSeekThrottleMS(state.videoSeekThrottleMS, 240));
  return true;
}

function seekVideoToTime(video, time, options = {}) {
  const photo = options.photo || state.lightboxPhotos[state.lightboxIndex];
  const throttled = options.throttled !== false && shouldThrottleVideoSeek();
  if (!video || video.classList.contains('hidden') || !Number.isFinite(video.duration) || video.duration <= 0) return false;
  const nextTime = Math.max(0, Math.min(video.duration, Number(time) || 0));
  if (throttled) {
    queueVideoSeekCommit(video, nextTime, photo);
  } else {
    state.pendingVideoSeek = null;
    clearPendingVideoSeekCommit();
    video.currentTime = nextTime;
  }
  updateVideoBookmarkProgress(video, photo);
  showVideoProgressActivity();
  return true;
}

function buildLightboxInfoItems(photo) {
  if (!photo) return [];
  const weekday = mediaWeekdayIndex(photo.taken_at);
  const dateIcon = weekday ? (icons[`infoDateDay${weekday}`] || icons.infoDate) : icons.infoDate;
  const base = [
    [isVideoMedia(photo) ? icons.mediaVideo : icons.mediaImage, copyText('app.interface.s_e4e46c7235'), isVideoMedia(photo) ? copyText('app.interface.mediaVideo') : copyText('app.interface.mediaImage')],
    [icons.infoMime, 'MIME', photo.mime_type],
    [dateIcon, copyText('app.interface.s_130ac85d9b'), formatMediaDateTime(photo.taken_at)],
    [icons.infoDimensions, copyText('app.interface.s_94a6711f24'), photo.width && photo.height ? `${photo.width} × ${photo.height}` : ''],
    [icons.infoFileSize, copyText('app.interface.s_fd20702c73'), formatSizeMB(photo.size)],
  ];
  if (!isVideoMedia(photo)) {
    return compactInfoItems([
      ...base.slice(0, 4),
      [icons.infoRatio, copyText('app.interface.s_f05f921f10'), aspectRatioLabel(photo.width, photo.height)],
      base[4],
      ...buildExifInfoItems(photo),
    ]);
  }
  const videoEXIF = photo.exif && typeof photo.exif === 'object' ? photo.exif : {};
  const videoCamera = [compactExifValue(videoEXIF.make), compactExifValue(videoEXIF.model)].filter(Boolean).join(' ');
  return compactInfoItems([
    ...base,
    [icons.exifCamera, copyText('app.interface.s_6042a0b155'), videoCamera],
    [icons.infoVideoCodec, copyText('app.interface.s_c94faa718a'), formatVideoCodec(videoEXIF.video_codec)],
    [icons.infoVideoFramerate, copyText('app.interface.s_7757a0dc83'), formatVideoFrameRate(videoEXIF.video_frame_rate)],
  ]);
}

// ── 缩略图 ────────────────────────────────────────────
function retryThumbnailLoad(imageEl, photo) {
  if (!imageEl || !photo) return;
  const attempt = Number(imageEl.dataset.thumbRetryAttempt || 0);
  const delays = [450, 1200, 2600];
  if (attempt >= delays.length) {
    imageEl.dataset.thumbFailed = '1';
    imageEl.closest('.photo-thumb')?.classList.add('thumb-load-failed');
    return;
  }
  imageEl.dataset.thumbRetryAttempt = String(attempt + 1);
  const nextURL = appendURLParam(mediaThumbURL(photo), 'r', String(Date.now()));
  imageEl.closest('.photo-thumb')?.classList.add('thumb-loading');
  window.setTimeout(() => {
    if (!document.body.contains(imageEl)) return;
    imageEl.src = nextURL;
  }, delays[attempt]);
}

function prefetchPhotosForLightboxIntent(listRef, index) {
  if (!isWarmEnabled()) return;
  const source = Array.isArray(listRef) ? listRef : [];
  if (!source.length || !Number.isFinite(index)) return;
  const candidates = [index, index - 1, index + 1, index - 2, index + 2]
    .filter(i => i >= 0 && i < source.length)
    .map(i => source[i])
    .filter(Boolean);
  candidates.forEach(photo => {
    const keyBase = photo.uuid || photo.id || photo.original_name || '';
    if (!keyBase) return;
    if (isVideoMedia(photo)) {
      const posterKey = `video-poster:${keyBase}`;
      if (rememberPrefetchedMedia(posterKey)) {
        const link = document.createElement('link');
        link.rel = 'prefetch';
        link.as = 'image';
        link.href = mediaThumbURL(photo);
        document.head.appendChild(link);
        setTimeout(() => link.remove(), 1500);
      }
      return;
    }
    const imageKey = `image-full:${keyBase}`;
    if (!rememberPrefetchedMedia(imageKey)) return;
    const img = new Image();
    img.decoding = 'async';
    img.src = mediaFileURL(photo);
  });
}

function makePhotoThumb(photo, listRef, opts = {}) {
  const div = el('div', 'photo-thumb');
  div.dataset.id = photo.id;
  div.dataset.kind = photo.media_kind || 'image';
  div.style.setProperty('--thumb-placeholder', stableMediaPlaceholderColor(photo));
  div.classList.add('thumb-loading');

  const isDebugMedia = isRootDebugMedia(photo);
  const thumbListRef = Array.isArray(listRef) ? listRef : [photo];
  const favoriteToggle = opts.trashMode || opts.disableFavorite || !canWriteMedia() || isDebugMedia
    ? ''
    : `<button class="favorite-toggle${photo.is_favorite ? ' active' : ''}${photo.is_super_favorite ? ' super-active' : ''}" type="button" aria-label="${escapeHTML(favoriteLabelForPhoto(photo))}" title="${escapeHTML(favoriteLabelForPhoto(photo))}">${favoriteIconForPhoto(photo)}</button>`;
  const mediaDurationBadge = isVideoMedia(photo) && photo.duration_ms
    ? `<span class="media-duration-badge" title="${escapeHTML(formatDuration(photo.duration_ms))}">${escapeHTML(formatDuration(photo.duration_ms))}</span>`
    : '';
  const bookmarkCount = getVideoBookmarkCount(photo);
  const videoBookmarkBadge = bookmarkCount > 0
    ? `<span class="video-bookmark-badge" title="${escapeHTML(copyText('app.interface.d_video_bookmarks', { count: bookmarkCount }))}" aria-label="${escapeHTML(copyText('app.interface.d_video_bookmarks', { count: bookmarkCount }))}"><span class="video-bookmark-icon">${icons.bookmark}</span><span class="video-bookmark-count">${bookmarkCount}</span></span>`
    : '';
  const thumbFallback = isVideoMedia(photo) ? ` onerror="this.onerror=null;this.src='${videoPosterPlaceholder}'"` : '';

  div.innerHTML = `<span class="check">${icons.check}</span>${favoriteToggle}<img loading="lazy" draggable="false" src="${mediaThumbURL(photo)}" alt="${photo.original_name}"${thumbFallback}><span class="thumb-bottom-gradient" aria-hidden="true"></span>${videoBookmarkBadge}${mediaDurationBadge}`;
  const imageEl = div.querySelector('img');
  if (imageEl) {
    imageEl.addEventListener('load', () => {
      requestAnimationFrame(() => requestAnimationFrame(() => div.classList.remove('thumb-loading', 'thumb-load-failed')));
      delete imageEl.dataset.thumbFailed;
    });
  }
  if (imageEl && !isVideoMedia(photo)) {
    imageEl.addEventListener('error', () => {
      retryThumbnailLoad(imageEl, photo);
    });
  }

  // b-1: 点击 .check 区域直接进入/切换选择模式
  const checkEl = div.querySelector('.check');
  if (opts.disableSelection || isDebugMedia) {
    checkEl.remove();
  } else {
    checkEl.addEventListener('click', e => {
      e.stopPropagation();
      toggleSelect(photo.id, div, { event: e, listRef: thumbListRef });
    });
  }
  const favoriteEl = div.querySelector('.favorite-toggle');
  if (favoriteEl) {
    favoriteEl.addEventListener('click', async e => {
      e.stopPropagation();
      await toggleFavorite(photo);
    });
    bindHoldAction(favoriteEl, e => {
      e.preventDefault();
      e.stopPropagation();
      void toggleFavorite(photo, { mode: 'hold', sourceButton: favoriteEl });
    });
    ['touchstart', 'touchmove', 'touchend', 'touchcancel'].forEach(type => {
      favoriteEl.addEventListener(type, e => {
        e.stopPropagation();
      }, { passive: true });
    });
  }
  if (isVideoMedia(photo)) requestVideoPlaybackIndicatorSync(photo);
  const photoIndex = Array.isArray(listRef) ? listRef.indexOf(photo) : -1;
  const warmIntent = () => {
    if (photoIndex >= 0) prefetchPhotosForLightboxIntent(thumbListRef, photoIndex);
  };
  div.addEventListener('pointerenter', warmIntent, { passive: true });
  div.addEventListener('focusin', warmIntent);

  // 图片主体点击
  div.addEventListener('click', e => {
    setFocusedPhoto(photo.id, { scroll: false });
    if (isDebugMedia) {
      openLightbox(thumbListRef, thumbListRef.indexOf(photo), { returnView: 'settings', debugControls: true });
      return;
    }
    if (state.selected.size > 0) {
      if (opts.disableSelection) return;
      toggleSelect(photo.id, div, { event: e, listRef: thumbListRef });
    } else {
      openLightbox(thumbListRef, thumbListRef.indexOf(photo));
    }
  });

  // PC 右键菜单
  div.addEventListener('contextmenu', e => {
    e.preventDefault();
    setFocusedPhoto(photo.id, { scroll: false });
    const menuOptions = { mobile: isTouchLikeDevice(), disableSelection: opts.disableSelection };
    if (opts.trashMode) showTrashContextMenu(e.clientX, e.clientY, photo, menuOptions);
    else if (!isDebugMedia) showPhotoContextMenu(e.clientX, e.clientY, photo, div, thumbListRef, menuOptions);
  });

  // c-3: 长按触发操作菜单（移动端）
  addLongPress(div, e => {
    const touch = e.changedTouches[0];
    if (opts.trashMode) showTrashContextMenu(touch.clientX, touch.clientY, photo, { mobile: true, disableSelection: opts.disableSelection });
    else if (!isDebugMedia) showPhotoContextMenu(touch.clientX, touch.clientY, photo, div, thumbListRef, { mobile: true, disableSelection: opts.disableSelection });
  });

  if (state.selected.has(photo.id)) div.classList.add('selected');
  if (state.focusedPhotoID === photo.id) div.classList.add('keyboard-focused');
  return div;
}

function bindHoldAction(el, callback, delay = 620) {
  if (!el) return;
  let timer = null;
  let fired = false;
  const clear = () => {
    clearTimeout(timer);
    timer = null;
  };
  el.addEventListener('pointerdown', e => {
    if (e.button > 0) return;
    fired = false;
    clear();
    timer = setTimeout(() => {
      fired = true;
      callback(e);
    }, delay);
  });
  ['pointerup', 'pointercancel', 'pointerleave'].forEach(type => {
    el.addEventListener(type, e => {
      clear();
      if (!fired) return;
      e.preventDefault();
      e.stopPropagation();
    }, true);
  });
  el.addEventListener('click', e => {
    if (!fired) return;
    fired = false;
    e.preventDefault();
    e.stopPropagation();
  }, true);
}

function visiblePhotoThumbs() {
  return $$('.photo-thumb').filter(thumb => thumb.offsetParent !== null && !thumb.dataset.placeholder);
}

function setFocusedPhoto(photoID, { scroll = true } = {}) {
  state.focusedPhotoID = Number(photoID) || null;
  $$('.photo-thumb.keyboard-focused').forEach(thumb => thumb.classList.remove('keyboard-focused'));
  const thumb = state.focusedPhotoID ? findPhotoThumb(state.focusedPhotoID) : null;
  if (thumb) {
    thumb.classList.add('keyboard-focused');
    if (scroll) thumb.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }
}

function currentViewPhotos() {
  switch (state.view) {
    case 'timeline': return state.photos;
    case 'favorites': return state.favoritePhotos;
    case 'memories': return state.memoryPhotos;
    case 'random-album': return state.randomAlbumPhotos;
    case 'album-detail': return state.albumPhotos;
    case 'trash': return state.trashPhotos;
    default: return [];
  }
}

function findKnownPhotoByID(id) {
  const targetID = Number(id);
  if (!targetID) return null;
  const pools = [
    currentViewPhotos(),
    state.photos,
    state.favoritePhotos,
    state.randomAlbumPhotos,
    state.albumPhotos,
    state.trashPhotos,
    state.memoryPhotos,
    state.searchResults,
  ];
  for (const pool of pools) {
    const found = (pool || []).find(photo => Number(photo && photo.id) === targetID);
    if (found) return found;
  }
  return null;
}

function refreshVisiblePendingThumbnails(force = false) {
  const now = Date.now();
  if (!force) {
    const last = Number(state.thumbnailVisibleRefreshAt || 0);
    if (now - last < 1200) return;
  }
  state.thumbnailVisibleRefreshAt = now;
  document.querySelectorAll('.photo-thumb.thumb-load-failed img, .photo-thumb.thumb-loading img').forEach(imageEl => {
    const thumb = imageEl.closest('.photo-thumb');
    if (!thumb || !document.body.contains(thumb)) return;
    if (!force && imageEl.complete && imageEl.naturalWidth > 0) {
      thumb.classList.remove('thumb-loading', 'thumb-load-failed');
      delete imageEl.dataset.thumbFailed;
      return;
    }
    const photo = findKnownPhotoByID(thumb.dataset.id);
    if (!photo) return;
    delete imageEl.dataset.thumbFailed;
    imageEl.dataset.thumbRetryAttempt = '0';
    thumb.classList.add('thumb-loading');
    thumb.classList.remove('thumb-load-failed');
    imageEl.src = appendURLParam(mediaThumbURL(photo), 'live', `${now}-${photo.id}`);
  });
}

function focusPhotoByGridStep(direction) {
  const thumbs = visiblePhotoThumbs();
  if (!thumbs.length) return false;
  let index = thumbs.findIndex(thumb => Number(thumb.dataset.id) === Number(state.focusedPhotoID));
  if (index < 0) index = 0;
  let columns = 1;
  const firstTop = thumbs[0].getBoundingClientRect().top;
  for (const thumb of thumbs) {
    if (Math.abs(thumb.getBoundingClientRect().top - firstTop) < 4) columns += thumb === thumbs[0] ? 0 : 1;
  }
  const step = direction === 'left' ? -1 : direction === 'right' ? 1 : direction === 'up' ? -columns : columns;
  const nextIndex = Math.max(0, Math.min(thumbs.length - 1, index + step));
  setFocusedPhoto(Number(thumbs[nextIndex].dataset.id));
  return true;
}

function openFocusedPhoto() {
  if (!state.focusedPhotoID) return false;
  const photos = currentViewPhotos();
  const index = photos.findIndex(photo => Number(photo.id) === Number(state.focusedPhotoID));
  if (index < 0) return false;
  openLightbox(photos, index);
  return true;
}

function findPhotoThumb(id) {
  const matches = Array.from(document.querySelectorAll(`.photo-thumb[data-id="${id}"]`));
  if (!matches.length) return null;
  return matches.find(node => node.offsetParent !== null) || matches[0];
}

function currentLoadedPreviewURLForPhoto(photo) {
  if (!photo || !photo.id) return '';
  const thumb = findPhotoThumb(photo.id);
  const image = thumb?.querySelector('img');
  if (image && image.src && image.complete && image.dataset.thumbFailed !== '1') {
    return image.currentSrc || image.src || '';
  }
  return '';
}

function lightboxDismissPhotoThumbRect(photo = state.lightboxPhotos[state.lightboxIndex]) {
  if (!photo || !photo.id) return null;
  const thumb = findPhotoThumb(photo.id);
  if (!thumb) return null;
  const rect = thumb.getBoundingClientRect();
  if (!rect || rect.width <= 0 || rect.height <= 0) return null;
  const style = getComputedStyle(thumb);
  return {
    left: rect.left,
    top: rect.top,
    width: rect.width,
    height: rect.height,
    borderRadius: style.borderRadius || '0px',
  };
}

function currentLightboxMediaRect() {
  const media = currentLightboxMediaElement();
  if (!media || media.classList.contains('hidden')) return null;
  const rect = media.getBoundingClientRect();
  if (!rect || rect.width <= 0 || rect.height <= 0) return null;
  return {
    left: rect.left,
    top: rect.top,
    width: rect.width,
    height: rect.height,
  };
}

function setLightboxDismissFade(progress) {
  const lightbox = $('#lightbox');
  if (!lightbox) return;
  const next = clampLightboxValue(Number(progress) || 0, 0, 1);
  lightbox.style.setProperty('--lightbox-dismiss-fade-progress', next.toFixed(4));
}

function clearLightboxDismissFade() {
  $('#lightbox')?.style.removeProperty('--lightbox-dismiss-fade-progress');
}

function activeLightboxDismissProxy() {
  return false;
}

function updateLightboxDismissProxy(translateY, scale = 1) {
  return;
}

function resetLightboxDismissProxy() {
  state.lightboxDismissProxyActive = false;
  state.lightboxDismissProxyRect = null;
  state.lightboxDismissProxySourceRect = null;
  state.lightboxDismissProxyTargetRect = null;
}

function compactContextMenuItems(items) {
  const result = [];
  (items || []).forEach(item => {
    if (item === '-') {
      if (result.length && result[result.length - 1] !== '-') result.push(item);
      return;
    }
    if (item) result.push(item);
  });
  while (result[0] === '-') result.shift();
  while (result[result.length - 1] === '-') result.pop();
  return result;
}

function filterPhotoContextMenuItemsForDevice(items, options = {}) {
  const mobile = !!options.mobile;
  const lightbox = !!options.lightbox;
  const filtered = (items || []).filter(item => {
    if (item === '-') return true;
    if (!item) return false;
    if (!canWriteMedia() && ['favorite', 'share', 'delete', 'rename', 'convert-playback', 'delete-playback-cache'].includes(item.role)) return false;
    if (!canEditBookmarks() && ['bookmark'].includes(item.role)) return false;
    if (!canRevealInFileManager() && item.role === 'reveal') return false;
    return true;
  });
  if (!mobile) return compactContextMenuItems(filtered);
  return compactContextMenuItems(filtered.filter(item => {
    if (item === '-') return true;
    if (!item) return false;
    if (lightbox && item.role === 'reveal') return false;
    return true;
  }));
}

function mobileSelectionMenuItem(photo, options = {}) {
  if (!options.mobile || options.lightbox || options.disableSelection) return null;
  const selected = state.selected.has(photo.id);
  return {
    role: 'select',
    icon: icons.check,
    label: selected ? copyText('app.interface.selectionCancel') : copyText('app.interface.s_4a8a30a99a'),
    action: () => toggleSelect(photo.id, options.thumbEl, { listRef: options.listRef }),
  };
}

function photoContextMenuItems(photo, thumbEl, listRef, containingAlbums = [], options = {}) {
  const isSelected = state.selected.has(photo.id);
  const isShared = !!state.shareMap[`photo:${photo.id}`];
  const favoriteLabel = favoriteLabelForPhoto(photo, { context: state.view });
  const favoriteAction = () => toggleFavorite(photo);
  const favoriteIcon = photo.is_super_favorite ? icons.superLike : photo.is_favorite ? icons.like : icons.nonLike;
  const items = [
    mobileSelectionMenuItem(photo, { ...options, thumbEl, listRef }),
    '-',
    { role: 'timeline', icon: icons.contextTimeline, label: copyText('app.interface.s_2cf712b27f'), action: () => openInTimeline(photo.id) },
    { role: 'favorite', icon: favoriteIcon || icons.contextFavorite, label: favoriteLabel, action: favoriteAction },
    canWriteMedia() ? { role: 'album-add', icon: icons.plus || icons.contextAlbum, label: copyText('app.interface.albumAddToAlbum'), action: () => addSinglePhotoToAlbum(photo.id) } : null,
    { role: 'reveal', icon: icons.contextReveal, label: copyText('app.interface.mediaOpenInFileManager'), action: () => revealInFinder(photo.id) },
    { role: 'download', icon: icons.contextDownload, label: singleMediaSaveLabel(), action: () => confirmDownloadAction(copyText('app.interface.d_66274d1532', { v0: (singleMediaSaveLabel()) }), () => saveOrDownloadMedia(photo)) },
    canWriteMedia() ? { role: 'rename', icon: icons.libraryCardEdit, label: copyText('app.interface.s_1cd80fd7a8'), action: () => openRenameMediaModal(photo) } : null,
  ];
  if (requiresBrowserPlaybackConversion(photo)) {
    const converted = playbackCacheExistsForPhoto(photo);
    items.push({
      role: 'convert-playback',
      icon: icons.contextConvertPlayback || icons.contextDownload,
      label: converted ? copyText('app.interface.s_479bf3a52b') : copyText('app.interface.s_678953bc56'),
      action: () => converted ? deleteBrowserPlaybackCacheForPhoto(photo) : buildBrowserPlaybackCacheForPhoto(photo),
    });
  }
  if (containingAlbums.length) {
    items.push('-');
    containingAlbums.forEach(album => {
      items.push({ role: 'album', icon: albumTypeContextIcon(album), label: copyText('app.interface.d_7b16bce3b1', { v0: (albumDisplayTitle(album)) }), action: () => openInAlbum(album, photo.id) });
    });
  }
  items.push(
    '-',
    {
      role: 'share',
      icon: isShared ? icons.shareModalLink : icons.contextShare,
      label: isShared ? copyText('app.interface.s_cfd638a5e3') : copyText('app.interface.s_abf17e6cfa'),
      action: () => isShared ? copyExistingShareLink('photo', photo.id) : openShareModal('photo', photo.id),
    },
    '-',
    { role: 'delete', icon: icons.contextDelete, label: copyText('app.interface.commonDelete'), danger: true, action: () => deleteSinglePhoto(photo.id) },
  );
  return filterPhotoContextMenuItemsForDevice(items, options);
}

function albumTypeContextIcon(album) {
  if (String(album && album.source_kind || '').trim() === 'smart') return icons.albumIntelligence || icons.contextAlbum;
  if (isFolderAlbum(album)) return icons.albumCardFolder || icons.contextAlbum;
  return icons.albumCardUser || icons.contextAlbum;
}

// 时间线图片右键菜单
async function showPhotoContextMenu(x, y, photo, thumbEl, listRef, options = {}) {
  let albums = [];
  try {
    albums = await api.get(`/api/media/${photo.id}/albums`);
  } catch (e) {
    console.error("加载媒体所在相册失败:", e);
  }
  if (requiresBrowserPlaybackConversion(photo)) {
    try {
      await ensurePlaybackCacheItemsLoaded();
    } catch (e) {
      console.error("加载播放兼容缓存列表失败:", e);
    }
  }
  if ((!albums || !albums.length) && state.view === 'album-detail' && state.currentAlbum && state.currentAlbum.id) {
    albums = [state.currentAlbum];
  }
  showContextMenu(x, y, photoContextMenuItems(photo, thumbEl, listRef, albums || [], options), options);
}

async function showCurrentLightboxContextMenu() {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  if (!photo) return;
  if (isRootDebugMedia(photo)) return;
  const button = $('#lb-more');
  const rect = button ? button.getBoundingClientRect() : { left: window.innerWidth - 64, right: window.innerWidth - 18, top: 32, bottom: 84, width: 46, height: 52 };
  await showPhotoContextMenu(rect.right, rect.bottom, photo, findPhotoThumb(photo.id), state.lightboxPhotos, {
    anchorRect: rect,
    align: 'end',
    vertical: 'below',
    lightbox: true,
    mobile: isTouchLikeDevice() || isMobileLayout(),
  });
}
function renderVideoBookmarkMenuContent(photo, video) {
  const currentTime = Math.max(0, Number(video.currentTime) || 0);
  return `
    <div class="lightbox-bookmark-menu-head">
      <div>
        <div class="lightbox-bookmark-menu-title">${escapeHTML(copyText('app.interface.runtime.videoBookmarks'))}</div>
        <div class="lightbox-bookmark-menu-subtitle">${escapeHTML(copyText('app.interface.runtime.videoBookmarkHelp'))}</div>
      </div>
      <span class="lightbox-bookmark-menu-current">${copyText('app.interface.d_bookmark_current_time', { time: formatDuration(currentTime * 1000) })}</span>
    </div>
    <div class="lightbox-bookmark-menu-list">
      ${Array.from({ length: 10 }, (_, i) => {
        const slot = i + 1;
        const bookmark = getVideoBookmarkSlot(photo, slot);
        const timeText = bookmark ? formatDuration(bookmark.time * 1000) : copyText('app.interface.s_a3eef8ee5f');
        const label = bookmark
          ? `${bookmark.name || copyText('app.interface.d_a90c323409', { v0: (slot) })} · ${timeText}`
          : copyText('app.interface.d_613794a2c6', { v0: (slot) });
        return `
          <div class="lightbox-bookmark-row${bookmark ? '' : ' is-empty'}" data-slot="${slot}">
            <button type="button" class="lightbox-bookmark-jump" data-video-bookmark-slot="${slot}" title="${escapeHTML(label)}">
              <span class="lightbox-bookmark-slot">${slot === 10 ? '0' : slot}</span>
              <span class="lightbox-bookmark-text">${escapeHTML(label)}</span>
            </button>
            <button type="button" class="btn-icon lightbox-bookmark-delete" data-video-bookmark-delete="${slot}"${copyText('app.interface.s_37f82782ff')} ${slot}"${copyText('app.interface.s_ffdd1b078a')} ${slot}" ${bookmark ? '' : 'disabled'}>${icons.contextDelete}</button>
          </div>`;
      }).join('')}
    </div>`;
}
function refreshVideoBookmarkMenu() {
  const menu = _ctxMenu;
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!menu || !photo || !video || !menu.classList.contains('lightbox-bookmark-menu')) return;
  menu.innerHTML = renderVideoBookmarkMenuContent(photo, video);
}
function showVideoBookmarkMenu() {
  if (!canEditBookmarks()) return;
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!isVideoMedia(photo) || !video || video.classList.contains('hidden')) return;
  closeContextMenu();
  const menu = el('div', 'context-menu lightbox-bookmark-menu');
  const button = $('#lb-video-bookmark');
  const rect = button ? button.getBoundingClientRect() : { left: window.innerWidth - 64, right: window.innerWidth - 18, top: 32, bottom: 84, width: 46, height: 52 };
  menu.style.left = `${rect.right}px`;
  menu.style.top = `${rect.top}px`;
  menu.style.visibility = 'hidden';
  menu.innerHTML = renderVideoBookmarkMenuContent(photo, video);
  document.body.appendChild(menu);
  _ctxMenu = menu;
  positionContextMenu(menu, rect.right, rect.bottom, {
    anchorRect: rect,
    align: 'end',
    vertical: 'below',
  });
}

// 回收站图片右键菜单 (b-4)
function showTrashContextMenu(x, y, photo, options = {}) {
  const writable = canWriteMedia();
  const items = [
    mobileSelectionMenuItem(photo, options),
    '-',
    (options.mobile || !canRevealInFileManager()) ? null : { role: 'reveal', icon: icons.contextReveal, label: copyText('app.interface.mediaOpenInFileManager'), action: () => revealInFinder(photo.id) },
    '-',
    writable ? { icon: icons.contextRestore, label: copyText('app.interface.s_24428e32fe'), action: () => restorePhoto(photo.id) } : null,
    writable ? { icon: icons.contextDelete, label: copyText('app.interface.commonDeletePermanently'), danger: true, action: () => hardDeleteSinglePhoto(photo.id) } : null,
  ];
  showContextMenu(x, y, compactContextMenuItems(items), options);
}

async function addSelectedMediaToAlbum(album) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (album && String(album.source_kind || '').trim() === 'smart') {
    showToast(copyText('app.interface.s_605b8679bc'));
    return;
  }
  if (!album || isFolderAlbum(album)) {
    showToast(copyText('app.interface.s_a5cc87a07d'));
    return;
  }
  const ids = [...state.selected];
  if (!ids.length) {
    showToast(copyText('app.interface.s_29aa1c728a'));
    return;
  }
  const result = await addMediaIDsToAlbum(album, ids);
  clearSelection();
  showToast(copyText('app.interface.runtime.addAlbumResult', { ok: result.ok, album: album.name, failure: result.fail ? copyText('app.interface.runtime.addAlbumFailure', { count: result.fail }) : '' }));
}

async function addMediaIDsToAlbum(album, ids = []) {
  const uniqueIDs = [...new Set((ids || []).map(id => Number(id)).filter(id => id > 0))];
  let ok = 0;
  let fail = 0;
  for (const id of uniqueIDs) {
    try {
      await api.post(`/api/media/albums/${album.id}`, { media_id: id });
      ok++;
    } catch (e) {
      fail++;
      console.warn("添加媒体到相册失败", { albumID: album.id, mediaID: id, error: e });
    }
  }
  if (ok) invalidateAlbumListState();
  return { ok, fail };
}

function invalidateAlbumListState() {
  state.albums = [];
  state.albumsLoaded = false;
  clearAlbumChildrenIndex();
}

function albumContextMenuItems(album, options = {}) {
  const { includeView = true, mobile = false } = options;
  const folderAlbum = String(album && album.source_kind || '').trim() === 'folder'
    || String(album && album.description || '').trim() === copyText('app.interface.s_326155a897');
  const smartAlbum = String(album && album.source_kind || '').trim() === 'smart';
  const items = [];
  if (includeView) {
    items.push({ role: 'view', icon: icons.contextView, label: copyText('app.interface.s_f7acefd2d4'), action: () => openAlbumDetail(album) });
  }
  if (!folderAlbum && !smartAlbum && canWriteMedia() && state.selected.size) {
    items.push({ role: 'album-add', icon: albumAddIconMarkup(), label: copyText('app.interface.s_2d639385ba'), action: () => addSelectedMediaToAlbum(album) });
  }
  items.push(
    canRevealInFileManager() ? { role: 'reveal', icon: icons.contextReveal, label: copyText('app.interface.mediaOpenInFileManager'), action: () => revealAlbumInFinder(album.id) } : null,
    { role: 'download', icon: icons.albumContextDownload, label: mediaArchiveSaveLabel(), action: () => confirmDownloadAction(copyText('app.interface.d_41fbb11ac8', { v0: (mediaArchiveSaveVerb()) }), () => triggerDownload(`/api/media/albums/${album.id}/download`)) },
  );
  if (smartAlbum && canWriteMedia()) {
    items.push(
      '-',
      { role: 'edit', icon: icons.libraryCardEdit, label: copyText('app.interface.s_13794d2141'), action: () => openSmartAlbumModal(album) },
      { role: 'delete', icon: icons.libraryCardDelete, label: copyText('app.interface.commonDelete'), danger: true, action: () => deleteAlbum(album) },
    );
  } else if (!folderAlbum && canWriteMedia()) {
    items.push(
      '-',
      { role: 'edit', icon: icons.libraryCardEdit, label: copyText('app.interface.s_a7f814c0a4'), action: () => openEditAlbumModal(album) },
      { role: 'delete', icon: icons.libraryCardDelete, label: copyText('app.interface.commonDelete'), danger: true, action: () => deleteAlbum(album) },
    );
  } else if (folderAlbum && !isRootFolderAlbum(album) && canWriteMedia()) {
    items.push(
      '-',
      { role: 'delete', icon: icons.libraryCardDelete, label: copyText('app.interface.s_6f0bb18bcd'), danger: true, action: () => deleteAlbum(album) },
    );
  }
  if (!mobile) return compactContextMenuItems(items);
  return compactContextMenuItems(items.filter(item => item && item.role !== 'view'));
}

function showAlbumContextMenu(x, y, album, options = {}) {
  if (!album || !album.id) return;
  showContextMenu(x, y, albumContextMenuItems(album, options), options);
}

async function addSinglePhotoToAlbum(photoId) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!photoId) return;
  openAlbumPickerModal([Number(photoId)]);
}

function favoriteIconForPhoto(photo) {
  if (photo && photo.is_super_favorite) return icons.superLike || icons.favoriteFilled || icons.like || icons.favorite;
  if (photo && photo.is_favorite) return icons.like || icons.favoriteFilled || icons.favorite;
  return icons.nonLike || icons.favorite;
}
function favoriteLabelForPhoto(photo, { context = state.view } = {}) {
  if (photo && (photo.is_super_favorite || photo.is_favorite)) return copyText('app.interface.s_db0b6c0ec7');
  return copyText('app.interface.s_42916f6b81');
}
function nextFavoriteStateForPhoto(photo, { context = state.view } = {}) {
  if (photo && (photo.is_super_favorite || photo.is_favorite)) return { favorite: false, superFavorite: false, kind: 'remove' };
  return { favorite: true, superFavorite: false, kind: 'add' };
}
function alternateFavoriteStateForPhoto(photo) {
  if (!photo || (!photo.is_favorite && !photo.is_super_favorite)) return null;
  if (photo.is_super_favorite) return { favorite: true, superFavorite: false, kind: 'downgrade' };
  return { favorite: true, superFavorite: true, kind: 'upgrade' };
}
function applyFavoriteStateToPhoto(photo, favorite, superFavorite = false) {
  if (!photo) return;
  photo.is_favorite = !!favorite || !!superFavorite;
  photo.is_super_favorite = !!superFavorite;
}
function updatePhotoFavoriteInCollections(photoId, favorite, superFavorite = false) {
  const collections = [
    state.photos,
    state.favoritePhotos,
    state.randomAlbumPhotos,
    state.albumPhotos,
    state.trashPhotos,
    state.lightboxPhotos,
    state.memoryPhotos,
    state.searchResults,
  ];
  collections.forEach(list => {
    if (!Array.isArray(list)) return;
    list.forEach(photo => {
      if (photo && photo.id === photoId) applyFavoriteStateToPhoto(photo, favorite, superFavorite);
    });
  });
}

function sortFavoritePhotosInPlace() {
  state.favoritePhotos.sort((a, b) => {
    const superDiff = Number(!!b.is_super_favorite) - Number(!!a.is_super_favorite);
    if (superDiff !== 0) return superDiff;
    const takenA = a && a.taken_at ? new Date(a.taken_at).getTime() : 0;
    const takenB = b && b.taken_at ? new Date(b.taken_at).getTime() : 0;
    if (takenA !== takenB) return takenB - takenA;
    return Number(b && b.id) - Number(a && a.id);
  });
}

function syncFavoritePhotoList(photoId, favorite, superFavorite = false, sourcePhoto = null) {
  const existingIndex = state.favoritePhotos.findIndex(photo => Number(photo && photo.id) === Number(photoId));
  if (!favorite) {
    if (existingIndex >= 0) {
      state.favoritePhotos.splice(existingIndex, 1);
      if (state.favoriteLoaded) {
        state.favoriteTotal = Math.max(0, (state.favoriteTotal || 0) - 1);
        updateFavoriteTotalHint();
      }
    }
    return;
  }
  if (existingIndex >= 0) {
    applyFavoriteStateToPhoto(state.favoritePhotos[existingIndex], favorite, superFavorite);
    sortFavoritePhotosInPlace();
    if (state.favoriteLoaded) updateFavoriteTotalHint();
    return;
  }
  if (!state.favoriteLoaded || !sourcePhoto) return;
  const favoritePhoto = { ...sourcePhoto };
  applyFavoriteStateToPhoto(favoritePhoto, favorite, superFavorite);
  state.favoritePhotos.push(favoritePhoto);
  sortFavoritePhotosInPlace();
  state.favoriteTotal = Math.max(0, Number(state.favoriteTotal) || 0) + 1;
  updateFavoriteTotalHint();
}

function updateFavoriteButtonsInDOM(photoId, favorite, superFavorite = false, { animateButton = null } = {}) {
  document.querySelectorAll(`.photo-thumb[data-id="${photoId}"] .favorite-toggle`).forEach(button => {
    const photo = findKnownPhotoByID(photoId) || { is_favorite: favorite, is_super_favorite: superFavorite };
    applyFavoriteStateToPhoto(photo, favorite, superFavorite);
    syncFavoriteButtonState(button, photo, { animateIcon: animateButton === button });
  });
}
function animateFavoriteIconSwap(button, nextIcon) {
  if (!button) return;
  const applyNext = () => {
    button.innerHTML = nextIcon;
    const nextSVG = button.querySelector('svg');
    if (!nextSVG || typeof nextSVG.animate !== 'function') return;
    nextSVG.animate([
      { transform: 'scale(0)' },
      { transform: 'scale(1.12)' },
      { transform: 'scale(1)' },
    ], {
      duration: 320,
      easing: 'cubic-bezier(.16, 1, .3, 1)',
      fill: 'both',
    });
  };
  const currentSVG = button.querySelector('svg');
  if (!currentSVG || typeof currentSVG.animate !== 'function') {
    applyNext();
    return;
  }
  currentSVG.animate([
    { transform: 'scale(1)' },
    { transform: 'scale(0)' },
  ], {
    duration: 210,
    easing: 'cubic-bezier(.55, 0, .85, .2)',
    fill: 'both',
  });
  clearTimeout(button._favoriteMorphTimer);
  button._favoriteMorphTimer = setTimeout(applyNext, 180);
}
function syncFavoriteButtonState(button, photo, { animateIcon = false } = {}) {
  if (!button || !photo) return;
  button.classList.toggle('active', !!photo.is_favorite);
  button.classList.toggle('super-active', !!photo.is_super_favorite);
  button.setAttribute('aria-label', favoriteLabelForPhoto(photo));
  button.title = favoriteLabelForPhoto(photo);
  const nextIcon = favoriteIconForPhoto(photo);
  if (animateIcon) animateFavoriteIconSwap(button, nextIcon);
  else button.innerHTML = nextIcon;
}

function removePhotoFromList(list, photoId) {
  if (!Array.isArray(list)) return false;
  const index = list.findIndex(photo => photo && photo.id === photoId);
  if (index < 0) return false;
  list.splice(index, 1);
  return true;
}

function renderFavoritesEmptyStateIfNeeded() {
  const container = $('#favorite-wrap');
  if (!container) return;
  if (container.querySelector('.photo-thumb')) return;
  container.innerHTML = `<div class="empty">${icons.favorite}<p>${escapeHTML(copyText('app.empty.favorites'))}</p></div>`;
}

function removeFavoritePhotoFromUI(photoId) {
  const removedFavorite = removePhotoFromList(state.favoritePhotos, photoId);
  removePhotoFromList(state.lightboxPhotos, photoId);
  if (removedFavorite) {
    state.favoriteTotal = Math.max(0, (state.favoriteTotal || 0) - 1);
    updateFavoriteTotalHint();
  }
  state.selected.delete(photoId);
  const thumb = findPhotoThumb(photoId);
  if (!thumb) {
    renderFavoritesEmptyStateIfNeeded();
    updateSelectionBar();
    return;
  }
  thumb.remove();
  renderFavoritesEmptyStateIfNeeded();
  updateSelectionModeUI();
  updateSelectionBar();
}

async function setPhotoFavorite(photoId, favorite, superFavorite = false) {
  await api.put(`/api/media/${photoId}/favorite`, { favorite: !!favorite || !!superFavorite, super_favorite: !!superFavorite });
  updatePhotoFavoriteInCollections(photoId, !!favorite || !!superFavorite, !!superFavorite);
}

function updateLightboxFavoriteButton(photo) {
  const favoriteBtn = $('#lb-favorite');
  if (!favoriteBtn) return;
  favoriteBtn.classList.toggle('hidden', !canWriteMedia() || isRootDebugMedia(photo));
  if (!photo || !canWriteMedia() || isRootDebugMedia(photo)) return;
  syncFavoriteButtonState(favoriteBtn, photo);
}
function syncLightboxShareButton() {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const shareBtn = $('#lb-share');
  shareBtn?.classList.toggle('hidden', !canManageShareLinks() || isRootDebugMedia(photo));
  $('#lb-more')?.classList.toggle('hidden', isRootDebugMedia(photo));
  if (!shareBtn || !photo || !canManageShareLinks() || isRootDebugMedia(photo)) return;
  const isShared = !!state.shareMap[`photo:${photo.id}`];
  shareBtn.innerHTML = isShared ? icons.shareModalLink : icons.share;
  shareBtn.title = isShared ? copyText('app.interface.s_cfd638a5e3') : copyText('app.interface.s_7a92434114');
  shareBtn.setAttribute('aria-label', isShared ? copyText('app.interface.s_cfd638a5e3') : copyText('app.interface.s_7a92434114'));
}

function favoriteToastForTransition(next, photo) {
  if (next.kind === 'upgrade') return copyText('app.interface.s_877b5c0f9b');
  if (next.kind === 'downgrade') return copyText('app.interface.s_2c521248ab');
  if (next.kind === 'remove') return copyText('app.interface.s_d72714eb4b');
  if (next.kind === 'add') return copyText('app.interface.s_89ddfdfcc4');
  return photo && photo.is_super_favorite ? copyText('app.interface.s_877b5c0f9b') : copyText('app.interface.s_89ddfdfcc4');
}

async function toggleFavorite(photo, { mode = 'click', sourceButton = null } = {}) {
  if (!canWriteMedia()) return forbidVisitorAction();
  const next = mode === 'hold'
    ? alternateFavoriteStateForPhoto(photo)
    : nextFavoriteStateForPhoto(photo, { context: state.view });
  if (!next) return;
  const nextFavorite = next.favorite;
  const nextSuperFavorite = next.superFavorite;
  try {
    await setPhotoFavorite(photo.id, nextFavorite, nextSuperFavorite);
    const knownPhoto = findKnownPhotoByID(photo.id) || photo;
    applyFavoriteStateToPhoto(knownPhoto, nextFavorite, nextSuperFavorite);
    syncFavoritePhotoList(photo.id, nextFavorite, nextSuperFavorite, knownPhoto);
    updateFavoriteButtonsInDOM(photo.id, nextFavorite, nextSuperFavorite, {
      animateButton: mode === 'hold' ? sourceButton : null,
    });
    if (state.view === 'favorites' && !nextFavorite) {
      closeLightbox();
      removeFavoritePhotoFromUI(photo.id);
      showToast(copyText('app.interface.s_d72714eb4b'));
      return;
    }
    if ($('#lightbox').classList.contains('open')) {
      syncFavoriteButtonState($('#lb-favorite'), knownPhoto, {
        animateIcon: mode === 'hold' && sourceButton && sourceButton.id === 'lb-favorite',
      });
    }
    if (state.view === 'favorites' && (next.kind === 'upgrade' || next.kind === 'downgrade')) rerenderFavoriteGrid();
    showToast(favoriteToastForTransition(next, knownPhoto));
  } catch (e) {
    showToast(copyText('app.interface.s_e80d7fcdfc') + ((e && e.error) || e), 3200);
  }
}

async function unfavoriteSelected() {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!state.selected.size) return;
  const ids = [...state.selected];
  for (const id of ids) {
    try {
      await setPhotoFavorite(id, false);
      syncFavoritePhotoList(id, false, false);
    } catch (e) {
      console.error("取消收藏失败:", id, e);
    }
  }
  clearSelection();
  switchView('favorites');
}

function deselectFavoriteSelected() {
  if (!state.selected.size) return;
  const favoriteIDs = [...state.selected].filter(id => {
    const photo = findKnownPhotoByID(id);
    return !!(photo && (photo.is_favorite || photo.is_super_favorite));
  });
  if (!favoriteIDs.length) {
    showToast(copyText('app.interface.s_cf04bfa78b'));
    return;
  }
  favoriteIDs.forEach(id => {
    state.selected.delete(id);
    setThumbSelectedState(id, false);
  });
  if (state.selectionAnchorID && !state.selected.has(Number(state.selectionAnchorID))) {
    const remainingIDs = [...state.selected];
    state.selectionAnchorID = remainingIDs.length ? remainingIDs[remainingIDs.length - 1] : null;
  }
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
  showToast(copyText('app.interface.d_d223fbc99b', { v0: (favoriteIDs.length) }));
}

async function deleteSinglePhoto(photoId) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!(await appConfirm(copyText('app.interface.s_a47c3bbc8a'), { danger: true }))) return;
  try {
    await api.del(`/api/media/${photoId}`);
    removeDeletedPhotosFromUI([photoId]);
    invalidateTrashViewState();
    showToast(copyText('app.interface.s_54e2ac947c'));
  }
  catch(e) { showToast(copyText('app.interface.s_5a5a8a09cc') + (e.error || e), 3200); }
}

async function hardDeleteSinglePhoto(photoId) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!(await appConfirm(copyText('app.interface.s_83ad15289c'), {
    title: copyText('app.interface.s_8fa58e3a10'),
    confirmText: copyText('app.interface.commonDeletePermanently'),
    danger: true,
  }))) return;
  try {
    await api.del(`/api/media/trash/${photoId}`);
    removeTrashPhotosFromUI([photoId]);
    showToast(copyText('app.interface.s_9b09be02fc'));
  } catch (e) {
    showToast(copyText('app.interface.s_3935403997') + ((e && e.error) || e), 3200);
  }
}

// ── 选择 ─────────────────────────────────────────────
function selectionOrderedIDs(listRef) {
  const source = Array.isArray(listRef) && listRef.length ? listRef : currentViewPhotos();
  const ids = [];
  const seen = new Set();
  source.forEach(photo => {
    const id = Number(photo && photo.id);
    if (!id || seen.has(id)) return;
    seen.add(id);
    ids.push(id);
  });
  return ids;
}
function setThumbSelectedState(id, selected) {
  document.querySelectorAll(`.photo-thumb[data-id="${id}"]`).forEach(thumb => {
    thumb.classList.toggle('selected', selected);
  });
}
function selectRangeBetween(anchorID, targetID, listRef) {
  const ordered = selectionOrderedIDs(listRef);
  const anchorIndex = ordered.indexOf(Number(anchorID));
  const targetIndex = ordered.indexOf(Number(targetID));
  if (anchorIndex < 0 || targetIndex < 0) return false;
  const [from, to] = anchorIndex <= targetIndex ? [anchorIndex, targetIndex] : [targetIndex, anchorIndex];
  ordered.slice(from, to + 1).forEach(id => {
    state.selected.add(id);
    setThumbSelectedState(id, true);
  });
  return true;
}
function toggleSelect(id, thumbEl, options = {}) {
  const numericID = Number(id);
  if (!numericID) return;
  const { event = null, listRef = null } = options;
  if (state.view === 'trash' && state.trashFolderSelection.size) {
    clearTrashFolderSelectionOnly();
  }
  if (event?.shiftKey && state.selectionAnchorID && state.selectionAnchorID !== numericID) {
    if (selectRangeBetween(state.selectionAnchorID, numericID, listRef)) {
      updateSelectionModeUI();
      updateSelectionBar();
      updateTrashSelBar();
      return;
    }
  }
  if (state.selected.has(numericID)) {
    state.selected.delete(numericID);
    setThumbSelectedState(numericID, false);
  }
  else {
    state.selected.add(numericID);
    setThumbSelectedState(numericID, true);
  }
  state.selectionAnchorID = numericID;
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
}
function clearSelection() {
  state.selected.clear();
  state.trashFolderSelection.clear();
  state.selectionAnchorID = null;
  $$('.photo-thumb.selected').forEach(t => t.classList.remove('selected'));
  $$('.trash-folder-card.is-selected').forEach(card => card.classList.remove('is-selected'));
  $$('.trash-folder-card[aria-pressed="true"]').forEach(card => card.setAttribute('aria-pressed', 'false'));
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
  updateTrashFolderSelectionBar();
}
function clearTrashPhotoSelectionOnly() {
  if (!state.selected.size) return;
  state.selected.clear();
  state.selectionAnchorID = null;
  $$('.photo-thumb.selected').forEach(t => t.classList.remove('selected'));
  updateSelectionBar();
  updateTrashSelBar();
}
function clearTrashFolderSelectionOnly() {
  if (!state.trashFolderSelection.size) return;
  state.trashFolderSelection.clear();
  $$('.trash-folder-card.is-selected').forEach(card => card.classList.remove('is-selected'));
  $$('.trash-folder-card[aria-pressed="true"]').forEach(card => card.setAttribute('aria-pressed', 'false'));
  updateTrashFolderSelectionBar();
}
function updateSelectionModeUI() {
  const selecting = state.selected.size > 0 || state.trashFolderSelection.size > 0;
  document.body.classList.toggle('selection-mode', selecting);
  const leading = $('#topbar-leading');
  if (leading) leading.inert = selecting && window.matchMedia('(max-width: 640px) and (hover: none) and (pointer: coarse)').matches;
}
function updateSelectionBar({ barID = 'sel-bar', countID = 'sel-count' } = {}) {
  const bar = $(`#${barID}`);
  if (!bar) return;
  bar.classList.toggle('visible', state.selected.size > 0);
  const cnt = $(`#${countID}`);
  if (cnt) cnt.textContent = state.selected.size;
}
function selectAllInGroup(groupEl) {
  if (state.view === 'trash' && state.trashFolderSelection.size) {
    clearTrashFolderSelectionOnly();
  }
  const thumbs = $$('.photo-thumb', groupEl);
  thumbs.forEach(thumb => {
    const id = Number(thumb.dataset.id);
    if (!state.selected.has(id)) {
      state.selected.add(id);
      thumb.classList.add('selected');
    }
  });
  if (thumbs.length) state.selectionAnchorID = Number(thumbs[0].dataset.id) || state.selectionAnchorID;
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
}
async function deleteSelected() {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!state.selected.size) return;
  if (!(await appConfirm(copyText('app.interface.d_bec7e0bdc2', { v0: (state.selected.size) }), { danger: true }))) return;
  const ids = [...state.selected];
  const deleted = [];
  const failed = [];
  for (const id of ids) {
    try {
      await api.del(`/api/media/${id}`);
      deleted.push(id);
    } catch (e) {
      failed.push(id);
      console.error(e);
    }
  }
  removeDeletedPhotosFromUI(deleted);
  if (deleted.length) invalidateTrashViewState();
  if (failed.length) {
    showToast(copyText('app.interface.d_c0a199de2e', { v0: (deleted.length), v1: (failed.length) }));
  } else if (deleted.length) {
    showToast(copyText('app.interface.d_f8534f24f5', { v0: (deleted.length) }));
  }
}

function removePhotoIDsFromList(list, ids) {
  if (!Array.isArray(list) || !list.length || !ids.size) return 0;
  let writeIndex = 0;
  let removed = 0;
  for (let readIndex = 0; readIndex < list.length; readIndex += 1) {
    const item = list[readIndex];
    const itemID = Number(item && item.id);
    if (itemID && ids.has(itemID)) {
      removed += 1;
      continue;
    }
    list[writeIndex] = item;
    writeIndex += 1;
  }
  list.length = writeIndex;
  return removed;
}
function ensureCurrentViewEmptyState() {
  switch (state.view) {
    case 'timeline': {
      const container = $('#timeline-wrap');
      if (container && !container.querySelector('.photo-thumb') && state.photos.length === 0 && !state.timelineHasMore && !state.timelineLoading) {
        container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.interface.runtime.emptyMedia'))}</p></div>`;
      }
      updateLoadMoreUI('load-more', state.timelineHasMore);
      break;
    }
    case 'favorites': {
      renderFavoritesEmptyStateIfNeeded();
      updateFavoriteTotalHint();
      updateLoadMoreUI('load-more', state.favoriteHasMore);
      break;
    }
    case 'memories': {
      const container = $('#memory-groups');
      if (container && !container.querySelector('.photo-thumb')) {
        container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.interface.runtime.emptyMemories'))}</p></div>`;
      }
      break;
    }
    case 'random-album': {
      const wrap = $('#random-album-wrap');
      if (wrap && !wrap.querySelector('.photo-thumb') && !state.randomAlbumHasMore && !state.randomAlbumLoading) {
        wrap.innerHTML = `<div class="empty">${icons.shuffle}<p>${escapeHTML(copyText('app.interface.runtime.randomEmpty'))}</p></div>`;
      }
      updateRandomAlbumLoadMoreUI();
      break;
    }
    case 'album-detail': {
      const container = $('#album-groups');
      if (container && !container.querySelector('.photo-thumb') && state.albumPhotos.length === 0 && !albumDirectChildCount(state.currentAlbum)) {
        container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.empty.albumMedia'))}</p></div>`;
      }
      updateLoadMoreUI('load-more', state.albumHasMore);
      break;
    }
    default:
      break;
  }
}
function removeDeletedPhotosFromUI(ids) {
  if (!Array.isArray(ids) || !ids.length) return;
  const idSet = new Set(ids.map(id => Number(id)).filter(Boolean));
  if (!idSet.size) return;
  removePhotoIDsFromList(state.photos, idSet);
  const removedFavorites = removePhotoIDsFromList(state.favoritePhotos, idSet);
  removePhotoIDsFromList(state.randomAlbumPhotos, idSet);
  const removedAlbumPhotos = removePhotoIDsFromList(state.albumPhotos, idSet);
  removePhotoIDsFromList(state.memoryPhotos, idSet);
  removePhotoIDsFromList(state.lightboxPhotos, idSet);
  state.memoryEntries = (state.memoryEntries || []).filter(entry => !idSet.has(Number(entry && entry.id)));
  saveMemoryEntries(state.memoryEntries);
  if (removedFavorites) {
    state.favoriteTotal = Math.max(0, (state.favoriteTotal || 0) - removedFavorites);
  }
  if (removedAlbumPhotos && state.currentAlbum) {
    state.currentAlbum.photo_count = Math.max(0, Number(state.currentAlbum.photo_count || 0) - removedAlbumPhotos);
    const cachedAlbum = (state.albums || []).find(album => Number(album && album.id) === Number(state.currentAlbum.id));
    if (cachedAlbum) cachedAlbum.photo_count = Math.max(0, Number(cachedAlbum.photo_count || 0) - removedAlbumPhotos);
  }
  if (state.timelineTotal) state.timelineTotal = Math.max(0, state.timelineTotal - idSet.size);
  if (state.randomAlbumTotal) state.randomAlbumTotal = Math.max(0, state.randomAlbumTotal - idSet.size);
  idSet.forEach(id => {
    state.selected.delete(id);
    delete state.shareMap[`photo:${id}`];
    if (state.focusedPhotoID === id) state.focusedPhotoID = null;
    if (state.selectionAnchorID === id) state.selectionAnchorID = null;
    document.querySelectorAll(`.photo-thumb[data-id="${id}"]`).forEach(node => node.remove());
  });
  if (!state.focusedPhotoID) {
    const nextThumb = visiblePhotoThumbs()[0];
    if (nextThumb) setFocusedPhoto(Number(nextThumb.dataset.id), { scroll: false });
  }
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
  ensureCurrentViewEmptyState();
}

function removeTrashPhotosFromUI(ids) {
  if (!Array.isArray(ids) || !ids.length) return;
  const idSet = new Set(ids.map(id => Number(id)).filter(Boolean));
  if (!idSet.size) return;
  removePhotoIDsFromList(state.trashPhotos, idSet);
  idSet.forEach(id => {
    state.selected.delete(id);
    if (state.selectionAnchorID === id) state.selectionAnchorID = null;
    document.querySelectorAll(`.photo-thumb[data-id="${id}"]`).forEach(node => node.remove());
  });
  updateSelectionModeUI();
  updateTrashSelBar();
  const container = $('#trash-wrap');
  if (container && state.trashPhotos.length === 0 && !state.trashHasMore && !state.trashLoading) {
    renderTrashGroups([]);
  }
}

function openInTimeline(photoId, photo = null) {
  if (photo && !mediaKindMatchesFilter(photo)) {
    state.mediaKindFilter = 'all';
    localStorage.setItem(mediaKindFilterStorageKey, 'all');
  }
  resetTimelineToInitialPage();
  state.pendingTimelinePhotoID = photoId;
  state.timelineJumpCancelRequested = false;
  showTimelineLocateBlocking();
  closeLightbox();
  switchView('timeline');
}

async function loadTimelineLocateWindow(photoId) {
  const params = new URLSearchParams();
  params.set('id', String(photoId));
  params.set('limit', String(computeTimelinePageSize()));
  params.set('order', state.timelineOrder === 'asc' ? 'asc' : 'desc');
  appendMediaKindParam(params);
  return api.get(`/api/media/timeline/locate?${params.toString()}`);
}

async function loadTimelineBeforeWindow(photoId) {
  const params = new URLSearchParams();
  params.set('id', String(photoId));
  params.set('limit', String(computeTimelinePageSize()));
  params.set('order', state.timelineOrder === 'asc' ? 'asc' : 'desc');
  appendMediaKindParam(params);
  return api.get(`/api/media/timeline/before?${params.toString()}`);
}

function computeAlbumPageSize() {
  return computeTimelinePageSize();
}

async function loadAlbumLocateWindow(albumId, photoId) {
  const params = new URLSearchParams();
  params.set('id', String(photoId));
  params.set('limit', String(computeAlbumPageSize()));
  params.set('sort', normalizeAlbumDetailSort(state.albumDetailSort));
  appendMediaKindParam(params);
  return api.get(`/api/media/albums/${albumId}/locate?${params.toString()}`);
}

async function loadAlbumBeforeWindow(albumId, photoId) {
  const params = new URLSearchParams();
  params.set('id', String(photoId));
  params.set('limit', String(computeAlbumPageSize()));
  params.set('sort', normalizeAlbumDetailSort(state.albumDetailSort));
  appendMediaKindParam(params);
  return api.get(`/api/media/albums/${albumId}/before?${params.toString()}`);
}

function openInAlbum(album, photoId) {
  if (!album || !album.id) return;
  saveViewScroll();
  state.albumEntrySourceView = state.view === 'random-album' ? 'random-album' : (state.view === 'timeline' ? 'timeline' : '');
  state.albumEntrySourcePhotoID = Number(photoId) || null;
  state.pendingAlbumPhotoID = photoId;
  showTimelineLocateBlocking({
    title: copyText('app.interface.s_28e2338a24'),
    message: copyText('app.interface.s_8c65b73dc8'),
    buttonText: copyText('app.interface.s_e94b248b5f'),
  });
  closeLightbox();
  openAlbumDetail(album);
}

async function focusPendingTimelinePhoto(options = {}) {
  if (!state.pendingTimelinePhotoID) return false;
  const skipScroll = !!options.skipScroll;
  const photoId = state.pendingTimelinePhotoID;
  let thumb = findPhotoThumb(photoId);
  state.pendingTimelinePhotoID = null;
  state.timelineJumpCancelRequested = false;
  if (!thumb) {
    setTimelineJumpStatus('');
    showToast(copyText('app.interface.s_aa18e0c728'));
    state.timelineLocatingActive = false;
    removeTimelineLocateBlocking();
    return false;
  }
  if (!skipScroll) thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  setFocusedPhoto(photoId, { scroll: false });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
  setTimelineJumpStatus('');
  state.timelineLocatingActive = false;
  removeTimelineLocateBlocking();
  return true;
}

async function focusPendingAlbumPhoto(options = {}) {
  if (!state.pendingAlbumPhotoID) return false;
  const skipScroll = !!options.skipScroll;
  const photoId = state.pendingAlbumPhotoID;
  let thumb = findPhotoThumb(photoId);
  state.pendingAlbumPhotoID = null;
  if (!thumb) {
    showToast(copyText('app.interface.s_d69dbc783f'));
    state.albumLocatingActive = false;
    removeTimelineLocateBlocking();
    return false;
  }
  if (!skipScroll) thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  setFocusedPhoto(photoId, { scroll: false });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
  state.albumLocatingActive = false;
  removeTimelineLocateBlocking();
  return true;
}

// ── 相册列表 ──────────────────────────────────────────
async function renderAlbums() {
  $('#topbar-title').textContent = copyText('app.interface.s_c657c81e6b');
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'new-album-btn', icon: icons.topbarNewAlbum, label: copyText('app.interface.s_0d7af6be71'), variant: 'accent' }),
  ]);
  $('#topbar-meta').innerHTML = renderAlbumViewModeControl() + renderAlbumKindFilterControl();
  $('#topbar-actions').innerHTML = '';
  const newAlbumButton = $('#new-album-btn');
  const smartFilterActive = state.albumKindFilter === 'smart';
  newAlbumButton.title = smartFilterActive
    ? copyText('app.interface.s_e110c63ea2')
    : copyText('app.interface.s_eb7dc6da9a');
  newAlbumButton.setAttribute('aria-label', smartFilterActive ? copyText('app.interface.s_e110c63ea2') : copyText('app.interface.s_0d7af6be71'));
  newAlbumButton.addEventListener('click', event => {
    if (event.altKey || state.albumKindFilter === 'smart') {
      event.preventDefault();
      openSmartAlbumModal();
      return;
    }
    openCreateAlbumModal();
  });
  syncAlbumToolbarLayout();
  if (state.albumToolbarResizeHandler) window.removeEventListener('resize', state.albumToolbarResizeHandler);
  state.albumToolbarResizeHandler = () => requestAnimationFrame(syncAlbumToolbarLayout);
  window.addEventListener('resize', state.albumToolbarResizeHandler, { passive: true });
  bindAlbumKindFilterControl();
  $('#album-view-grid-btn').addEventListener('click', () => {
    state.albumViewMode = 'grid';
    renderAlbums();
  });
  $('#album-view-list-btn').addEventListener('click', () => {
    state.albumViewMode = 'list';
    renderAlbums();
  });

  $('#content').innerHTML = `<div id="album-grid-wrap"></div>`;
  if (state.albumsLoaded) {
    renderAlbumGrid();
    restoreViewScroll('albums');
    return;
  }
  try {
    await ensureAlbumsLoaded();
    renderAlbumGrid();
    restoreViewScroll('albums');
  } catch(e) { $('#content').innerHTML = `<p style="color:var(--danger)">${escapeHTML(copyText('app.interface.runtime.loadFailed'))}</p>`; }
}

async function ensureAlbumsLoaded() {
  if (state.albumsLoaded && Array.isArray(state.albums)) return state.albums;
  state.albums = await api.get('/api/media/albums');
  state.albumsLoaded = true;
  clearAlbumChildrenIndex();
  return state.albums;
}

function renderAlbumGrid() {
  const wrap = $('#album-grid-wrap');
  if (!wrap) return;
  const albums = albumChildrenForParent('', state.albumKindFilter);
  if (!albums.length) {
    wrap.innerHTML = `<div class="empty">${icons.album}<p>${escapeHTML(copyText('app.empty.albums'))}</p></div>`;
    return;
  }
  const grid = el('div', `album-grid${state.albumViewMode === 'list' ? ' list' : ''}`);
  albums.forEach(a => grid.appendChild(makeAlbumCard(a)));
  wrap.innerHTML = '';
  wrap.appendChild(grid);
}

function isFolderAlbum(album) {
  return String(album && album.source_kind || '').trim() === 'folder'
    || String(album && album.description || '').trim() === copyText('app.interface.s_326155a897');
}

function normalizeAlbumPath(value = '') {
  return String(value || '')
    .trim()
    .replace(/\\/g, '/')
    .replace(/\/+/g, '/')
    .replace(/^\/|\/$/g, '');
}

function folderAlbumPath(album) {
  if (!isFolderAlbum(album)) return '';
  return normalizeAlbumPath(album.source_rel_path || album.name || '');
}

function albumDisplayTitle(album) {
  if (!album) return '';
  const path = folderAlbumPath(album);
  if (!path) return String(album.name || '').trim() || copyText('app.interface.s_c657c81e6b');
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || path;
}

function isRootFolderAlbum(album) {
  return isFolderAlbum(album)
    && !normalizeAlbumPath(album && (album.source_rel_path || ''))
    && String(album && album.name || '').trim() === copyText('app.interface.s_18755acbbb');
}

function albumRelativeAddress(album) {
  const path = folderAlbumPath(album);
  return path || String(album && album.description || '').trim();
}

function albumParentPath(album) {
  const path = folderAlbumPath(album);
  if (!path || !path.includes('/')) return '';
  return path.split('/').slice(0, -1).join('/');
}

function compareAlbumsByPath(a, b) {
  if (isRootFolderAlbum(a)) return isRootFolderAlbum(b) ? 0 : -1;
  if (isRootFolderAlbum(b)) return 1;
  return albumRelativeAddress(a).localeCompare(albumRelativeAddress(b), 'zh-Hans-CN', { numeric: true, sensitivity: 'base' });
}

function albumMatchesKindFilter(album, filter = state.albumKindFilter) {
  const normalized = normalizeAlbumKindFilter(filter);
  const smartAlbum = String(album && album.source_kind || '').trim() === 'smart';
  if (normalized === 'smart') return smartAlbum;
  if (normalized === 'user') return !isFolderAlbum(album) && !smartAlbum;
  if (normalized === 'folder') return isFolderAlbum(album);
  return true;
}

function albumChildrenForParent(parentPath = '', kindFilter = 'all') {
  const target = normalizeAlbumPath(parentPath);
  const index = buildAlbumChildrenIndex();
  const children = (index.get(target) || []).filter(album => target ? true : albumMatchesKindFilter(album, kindFilter));
  return children.slice().sort(compareAlbumsByPath);
}

function albumChildAlbumsForDetail(album) {
  if (!isFolderAlbum(album)) return [];
  return albumChildrenForParent(folderAlbumPath(album));
}

function albumSiblingList(album = state.currentAlbum) {
  if (!album) return [];
  return albumChildrenForParent(isFolderAlbum(album) ? albumParentPath(album) : '');
}

function albumDirectChildCount(album) {
  if (!album || !isFolderAlbum(album)) return 0;
  const index = buildAlbumChildrenIndex();
  return (index.get(folderAlbumPath(album)) || []).length;
}

function albumItemCount(album) {
  return Math.max(0, Number(album && album.photo_count) || 0) + albumDirectChildCount(album);
}

function buildAlbumChildrenIndex() {
  if (state.albumChildrenIndex) return state.albumChildrenIndex;
  const index = new Map();
  let rootSeen = false;
  for (const album of state.albums || []) {
    const folderAlbum = isFolderAlbum(album);
    if (folderAlbum && isRootFolderAlbum(album)) {
      if (rootSeen) continue;
      rootSeen = true;
    }
    const parent = folderAlbum ? albumParentPath(album) : '';
    if (!index.has(parent)) index.set(parent, []);
    index.get(parent).push(album);
  }
  state.albumChildrenIndex = index;
  return index;
}

function clearAlbumChildrenIndex() {
  state.albumChildrenIndex = null;
}

function findAlbumByPath(path = '') {
  const target = normalizeAlbumPath(path);
  if (!target) return null;
  return (state.albums || []).find(album => folderAlbumPath(album) === target) || null;
}

function parentAlbumFor(album) {
  return findAlbumByPath(albumParentPath(album));
}

function returnToAlbumParent(album = state.currentAlbum) {
  if (!parentAlbumFor(album) && state.albumEntrySourceView) {
    const sourceView = state.albumEntrySourceView;
    const sourcePhotoID = Number(state.albumEntrySourcePhotoID) || null;
    state.albumEntrySourceView = '';
    state.albumEntrySourcePhotoID = null;
    if (sourceView === 'timeline' && sourcePhotoID) {
      state.pendingTimelinePhotoID = sourcePhotoID;
      switchView('timeline');
      return;
    }
    if (sourceView === 'random-album' && sourcePhotoID) {
      state.pendingRandomAlbumPhotoID = sourcePhotoID;
      switchView('random-album');
      return;
    }
  }
  const parent = parentAlbumFor(album);
  if (parent) {
    openAlbumDetail(parent);
  } else {
    switchView('albums');
  }
}

function makeAlbumCard(album, options = {}) {
  const picker = options.picker === true;
  const smartAlbum = String(album && album.source_kind || '').trim() === 'smart';
  const albumKind = smartAlbum ? 'smart' : isFolderAlbum(album) ? 'folder' : 'user';
  const rootAlbum = isRootFolderAlbum(album);
  const albumKindLabel = albumKind === 'folder' ? copyText('app.interface.s_46ecac2910') : albumKind === 'smart' ? copyText('app.interface.albumSmart') : copyText('app.interface.s_05b503551a');
  const albumKindIcon = albumKind === 'folder' ? icons.albumCardFolder : albumKind === 'smart' ? icons.albumIntelligence : icons.albumCardUser;
  const title = albumDisplayTitle(album);
  const description = albumKind === 'smart'
    ? String(album.description || '').trim() || smartAlbumDescribeRules(album.smart_rules || []) || copyText('app.interface.s_d3282923b5')
    : albumRelativeAddress(album) || (albumKind === 'folder' ? copyText('app.interface.s_7c1e875802') : copyText('app.interface.albumUserCreated'));
  const itemCount = albumItemCount(album);
  const card = el('div', `album-card album-card-${albumKind}${rootAlbum ? ' album-card-root' : ''}${!picker && state.albumViewMode === 'list' ? ' list' : ''}${picker ? ' album-card-picker' : ''}`);
  // c-1: 用 cover_uuid 显示封面缩略图
  const emptyCoverIcon = albumKind === 'folder' ? icons.albumCardFolder : albumKind === 'smart' ? icons.albumIntelligence : icons.albumCardUser;
  const coverHtml = album.cover_uuid
    ? `<img loading="lazy" src="${mediaThumbURLFromUUID(album.cover_uuid)}" alt="${escapeHTML(title)}" onerror="this.onerror=null;this.src='${videoPosterPlaceholder}'">`
    : `<div class="album-cover-empty album-cover-empty-${albumKind}" aria-hidden="true">${emptyCoverIcon || icons.photo}</div>`;
  card.innerHTML = `
	<div class="album-cover">${coverHtml}</div>
	<div class="album-info">
	  <div class="album-name" title="${escapeHTML(title)}">${escapeHTML(title)}</div>
    <div class="album-subtitle">${escapeHTML(description)}</div>
    <div class="album-count-row">${icons.albumCardCount || icons.photo}<span>${escapeHTML(copyText('app.interface.runtime.albumItemCount', { count: itemCount }))}</span></div>
    ${rootAlbum ? `<span class="album-root-label">${copyText('app.echoFrame.libraryRoot')}</span>` : ''}
  </div>`;
  const kind = el('span', 'album-kind', albumKindIcon);
  kind.title = albumKindLabel;
  kind.setAttribute('aria-label', albumKindLabel);
  $('.album-info', card)?.appendChild(kind);
  if (picker) {
    card.setAttribute('role', 'button');
    card.setAttribute('tabindex', '0');
    card.setAttribute('aria-pressed', 'false');
    card.setAttribute('aria-label', copyText('app.interface.d_8f61647a1d', { v0: (title) }));
    return card;
  }
  card.addEventListener('click', () => openAlbumDetail(album));
  card.addEventListener('contextmenu', e => {
    e.preventDefault();
    showAlbumContextMenu(e.clientX, e.clientY, album, { mobile: isTouchLikeDevice() });
  });
  addLongPress(card, e => {
    const touch = e.changedTouches[0];
    showAlbumContextMenu(touch.clientX, touch.clientY, album, { mobile: true });
  });
  return card;
}

// ── 相册详情 ──────────────────────────────────────────
async function openAlbumDetail(album) {
  await ensureAlbumsLoaded().catch(() => {});
  saveViewScroll();
  const nextID = Number(album && album.id);
  const prevID = Number(state.currentAlbumID || 0);
  const cached = (state.albums || []).find(item => Number(item && item.id) === nextID);
  if (cached) album = { ...album, ...cached };
  state.currentAlbum = album;
  state.currentAlbumID = album.id;
  state.lastAlbumDetailID = album.id;
  if (nextID !== prevID) {
    state.albumPhotos = [];
    state.albumCursor = '';
    state.albumHasMore = true;
    state.albumHasBefore = false;
    state.albumLoadingBefore = false;
    state.albumLocateMissingBeforeCount = 0;
    state.albumLocatingActive = false;
    state.albumDetailLoadedKey = '';
  }
  state.view = 'album-detail';
  setHashView('album-detail', album.id);
  syncNavActiveView('album-detail');
  renderAlbumDetail();
}
function adjacentAlbum(offset) {
  const albums = albumSiblingList(state.currentAlbum);
  const index = albums.findIndex(item => Number(item.id) === Number(state.currentAlbumID));
  if (index < 0) return null;
  return albums[index + offset] || null;
}

function openAdjacentAlbum(offset) {
  const album = adjacentAlbum(offset);
  if (!album) return;
  openAlbumDetail(album);
}

async function renderAlbumDetail() {
  let album = state.currentAlbum;
  if (state.currentAlbumID && (!state.albumsLoaded || !(state.albums || []).some(item => Number(item && item.id) === Number(state.currentAlbumID)))) {
    try {
      await ensureAlbumsLoaded();
    } catch (e) {
      console.warn("补载相册列表失败，Q/E 相册切换可能暂不可用", e);
    }
  }
  if (!album && state.currentAlbumID) {
    try {
		album = await api.get(`/api/media/albums/${state.currentAlbumID}/detail`);
      state.currentAlbum = album;
    } catch (e) {
      // 相册不存在或加载失败时回退到相册列表
      state.currentAlbum = null;
      state.currentAlbumID = null;
      switchView('albums');
      return;
    }
  }
  if (!album) {
    switchView('albums');
    return;
  }
  const albumTitle = albumDisplayTitle(album);
  const albumAddress = albumRelativeAddress(album);
  const childAlbums = albumChildAlbumsForDetail(album);
  const writable = canWriteMedia();
  $('#topbar-title').textContent = albumTitle;
  $('#topbar-meta').innerHTML = childAlbums.length
    ? renderAlbumViewModeControl()
    : renderSelectionBarMarkup({
      extraAction: writable
        ? `${renderAddToAlbumSelectionAction()}<button class="btn-icon" id="delete-sel-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.deleteSelectedMedia'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.deleteSelectedMedia'))}">${icons.selectionDelete}</button>`
        : '',
    });
  $('#topbar-actions').innerHTML = '';
  if (!childAlbums.length) {
    bindSelectionBarHandlers({
      extraButtonID: writable ? 'delete-sel-btn' : '',
      extraAction: writable ? deleteSelected : null,
      actionBindings: writable ? [{ id: 'add-to-album-sel-btn', action: () => openAlbumPickerModal(null) }] : [],
    });
  }

  $('#content').innerHTML = `
<div class="album-detail-pill" id="album-detail-pill">
  <button class="album-detail-back-btn" id="album-detail-back-btn" type="button" title="${escapeHTML(copyText('app.interface.runtime.albumDetailBack'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.albumDetailBack'))}">${icons.topbarBackAlbums}</button>
  <span class="album-detail-title" title="${escapeHTML(albumAddress || albumTitle)}">${escapeHTML(albumTitle)}</span>
</div>
${childAlbums.length ? '' : `<div class="album-detail-bottom-controls" id="album-detail-bottom-controls">${renderAlbumDetailBottomActionControls()}</div>`}
<div id="album-child-wrap"></div><div class="load-more load-more-top" id="load-more-top" ${childAlbums.length ? 'style="display:none"' : ''}><div class="spinner"></div>${escapeHTML(copyText('app.interface.runtime.loadEarlier'))}</div><div id="album-groups"></div><div class="load-more" id="load-more" ${childAlbums.length ? 'style="display:none"' : ''}><div class="spinner"></div>${copyText('app.interface.commonLoading')}</div>`;
  renderAlbumChildGrid(childAlbums);
  if (childAlbums.length) {
    $('#album-view-grid-btn')?.addEventListener('click', () => {
      state.albumViewMode = 'grid';
      renderAlbumDetail();
    });
    $('#album-view-list-btn')?.addEventListener('click', () => {
      state.albumViewMode = 'list';
      renderAlbumDetail();
    });
  } else {
    bindMediaKindFilterControl();
    $$('[data-album-detail-sort]').forEach(btn => {
      btn.addEventListener('click', () => {
        const next = normalizeAlbumDetailSort(btn.dataset.albumDetailSort);
        if (next === state.albumDetailSort) return;
        state.albumDetailSort = next;
        localStorage.setItem(albumDetailSortStorageKey, next);
        state.albumPhotos = [];
        state.albumCursor = '';
        state.albumHasMore = true;
        state.albumHasBefore = false;
        state.albumLoadingBefore = false;
        state.albumLocateMissingBeforeCount = 0;
        state.albumLocatingActive = false;
        state.albumDetailLoadedKey = '';
        state.viewScrollPositions[viewScrollKeyFor('album-detail', album.id)] = 0;
        renderAlbumDetail();
      });
    });
  }
  $('#album-detail-back-btn')?.addEventListener('click', () => {
    returnToAlbumParent(album);
  });
  $('#album-detail-pill')?.addEventListener('contextmenu', e => {
    e.preventDefault();
    showAlbumContextMenu(e.clientX, e.clientY, album, { includeView: false, mobile: isTouchLikeDevice() });
  });
  addLongPress($('#album-detail-pill'), e => {
    const touch = e.changedTouches[0];
    showAlbumContextMenu(touch.clientX, touch.clientY, album, { includeView: false, mobile: true });
  });
  const hasPendingAlbumFocus = !!state.pendingAlbumPhotoID;
  const detailStateKey = `${album.id}:${state.albumDetailSort}:${state.mediaKindFilter}:${childAlbums.map(item => item.id).join(',')}`;
  if (childAlbums.length) {
    state.albumPhotos = [];
    state.albumCursor = '';
    state.albumHasMore = false;
    state.albumHasBefore = false;
    state.albumLoading = false;
    state.albumLoadingBefore = false;
    state.albumLocateMissingBeforeCount = 0;
    state.albumLocatingActive = false;
    state.albumDetailLoadedKey = detailStateKey;
    renderAlbumGroups([]);
    updateLoadMoreUI('load-more', false);
    updateLoadMoreUI('load-more-top', false);
    restoreViewScroll('album-detail', album.id);
    return;
  }
  const canReuseLoadedAlbumForPendingFocus = hasPendingAlbumFocus
    && state.albumDetailLoadedKey === detailStateKey
    && state.albumPhotos.some(photo => Number(photo && photo.id) === Number(state.pendingAlbumPhotoID));
  if (state.albumDetailLoadedKey === detailStateKey && !hasPendingAlbumFocus) {
    renderAlbumGroups(state.albumPhotos);
    requestVisibleThumbnailWarmup(`album:${album.id}`, state.albumPhotos);
    observeLoadMore('load-more-top', loadMoreAlbumBefore, () => state.albumHasBefore && !albumUsesManualBeforeLoading() && !state.albumLoadingBefore && !state.albumLoading, { rootMargin: '50% 0px 0px 0px' });
    updateLoadMoreUI('load-more', state.albumHasMore);
    updateLoadMoreUI('load-more-top', state.albumHasBefore);
    observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    restoreViewScroll('album-detail', album.id);
    return;
  }
  if (canReuseLoadedAlbumForPendingFocus) {
    state.albumLocatingActive = false;
    renderAlbumGroups(state.albumPhotos);
    requestVisibleThumbnailWarmup(`album:${album.id}`, state.albumPhotos);
    observeLoadMore('load-more-top', loadMoreAlbumBefore, () => state.albumHasBefore && !albumUsesManualBeforeLoading() && !state.albumLoadingBefore && !state.albumLoading, { rootMargin: '50% 0px 0px 0px' });
    updateLoadMoreUI('load-more', state.albumHasMore);
    updateLoadMoreUI('load-more-top', state.albumHasBefore);
    observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    if (!(await focusPendingAlbumPhoto())) restoreViewScroll('album-detail', album.id);
    return;
  }
  state.albumPhotos = [];
  state.albumCursor = '';
  state.albumHasMore = true;
  state.albumHasBefore = false;
  state.albumLoadingBefore = false;
  state.albumLocateMissingBeforeCount = 0;
  if (hasPendingAlbumFocus) {
    try {
      state.albumLocatingActive = true;
      const located = await loadAlbumLocateWindow(album.id, state.pendingAlbumPhotoID);
      state.albumPhotos = Array.isArray(located && located.photos) ? located.photos : [];
      state.albumLocateMissingBeforeCount = Math.max(0, Number(located && located.missing_before_count) || 0);
      state.albumHasBefore = !!(located && located.has_before);
      state.albumCursor = (located && located.next_cursor) || '';
      state.albumHasMore = !!(located && located.has_after);
      state.albumDetailLoadedKey = detailStateKey;
      renderAlbumGroups(state.albumPhotos);
      const focusedPendingAlbumPhoto = await focusPendingAlbumPhoto();
      observeLoadMore('load-more-top', loadMoreAlbumBefore, () => state.albumHasBefore && !albumUsesManualBeforeLoading() && !state.albumLoadingBefore && !state.albumLoading, { rootMargin: '50% 0px 0px 0px' });
      observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
      maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
      if (!focusedPendingAlbumPhoto) restoreViewScroll('album-detail', album.id);
      return;
    } catch (e) {
      console.warn('album locate failed, fallback to regular loading', e);
      state.albumLocatingActive = false;
      state.albumLocateMissingBeforeCount = 0;
      removeTimelineLocateBlocking();
    }
  }
  state.albumLocatingActive = false;
  await loadMoreAlbumPhotos();
  state.albumDetailLoadedKey = detailStateKey;
  const focusedPendingAlbumPhoto = await focusPendingAlbumPhoto();
  observeLoadMore('load-more-top', loadMoreAlbumBefore, () => state.albumHasBefore && !albumUsesManualBeforeLoading() && !state.albumLoadingBefore && !state.albumLoading, { rootMargin: '50% 0px 0px 0px' });
  observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
  maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
  if (!hasPendingAlbumFocus || !focusedPendingAlbumPhoto) restoreViewScroll('album-detail', album.id);
}

function renderAlbumChildGrid(albums = []) {
  const wrap = $('#album-child-wrap');
  if (!wrap) return;
  if (!albums.length) {
    wrap.innerHTML = '';
    return;
  }
  const grid = el('div', `album-grid${state.albumViewMode === 'list' ? ' list' : ''}`);
  albums.forEach(album => grid.appendChild(makeAlbumCard(album)));
  wrap.innerHTML = '';
  wrap.appendChild(grid);
}
function syncAlbumState(updatedAlbum) {
  if (!updatedAlbum || !updatedAlbum.id) return;
  const targetID = Number(updatedAlbum.id);
  state.albums = (state.albums || []).map(album => (
    Number(album.id) === targetID ? { ...album, ...updatedAlbum } : album
  ));
  clearAlbumChildrenIndex();
  if (state.currentAlbum && Number(state.currentAlbum.id) === targetID) {
    state.currentAlbum = { ...state.currentAlbum, ...updatedAlbum };
  }
}
async function deleteAlbum(album) {
  if (!album) return;
  const folderAlbum = isFolderAlbum(album);
  if (folderAlbum && isRootFolderAlbum(album)) {
    showToast(copyText('app.interface.s_a00e26eddd'));
    return;
  }
  const message = folderAlbum
    ? copyText('app.interface.d_de1d9b1d4f', { v0: (albumDisplayTitle(album)) })
    : copyText('app.interface.d_15e4e681ed', { v0: (album.name) });
  if (!(await appConfirm(message, {
    danger: true,
    title: folderAlbum ? copyText('app.interface.s_6f0bb18bcd') : copyText('app.interface.s_0666a3610c'),
    confirmText: folderAlbum ? copyText('app.interface.s_298cbe31d3') : copyText('app.interface.commonDelete'),
  }))) return;
  try {
	await api.del(`/api/media/albums/${album.id}`);
    invalidateAlbumListState();
    invalidateTrashViewState();
    state.currentAlbum = null;
    state.currentAlbumID = null;
    state.lastAlbumDetailID = null;
    showToast(folderAlbum ? copyText('app.interface.s_39574f4d07') : copyText('app.interface.s_0d262240b4'));
    switchView('albums');
  } catch (e) {
    showToast(copyText('app.interface.s_3220a60584') + (e.error || e), 3200);
  }
}
async function loadMoreAlbumPhotos() {
  if (state.albumLoading || !state.albumHasMore || !state.currentAlbum) return;
  state.albumLoading = true;
	try {
		const id = state.currentAlbum.id;
		const params = new URLSearchParams();
		appendMediaKindParam(params);
		params.set('sort', normalizeAlbumDetailSort(state.albumDetailSort));
		if (state.albumCursor) params.set('cursor', state.albumCursor);
		const query = params.toString();
		const url = `/api/media/albums/${id}` + (query ? `?${query}` : '');
		const page = await api.get(url);
    const photos = page.photos || [];
    state.albumPhotos.push(...photos);
    state.albumCursor = page.next_cursor || '';
    state.albumHasMore = page.has_more || false;
    renderAlbumGroups(photos);
    requestVisibleThumbnailWarmup(`album:${id}`, photos);
  } catch(e) { console.error(e); }
  finally {
    state.albumLoading = false;
    updateLoadMoreUI('load-more-top', state.albumHasBefore);
    updateLoadMoreUI('load-more', state.albumHasMore);
    observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
  }
}

function prependAlbumGrid(newPhotos) {
  const container = $('#album-groups');
  if (!container || !newPhotos.length) return;
  let grid = $('#album-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'album-grid';
    container.appendChild(grid);
  }
  const placeholders = Array.from(grid.querySelectorAll('.timeline-placeholder-thumb'));
  const replacementCount = Math.min(placeholders.length, newPhotos.length);
  const replacementStart = Math.max(0, placeholders.length - replacementCount);
  for (let i = 0; i < replacementCount; i += 1) {
    const replacement = makePhotoThumb(newPhotos[i], state.albumPhotos);
    grid.replaceChild(replacement, placeholders[replacementStart + i]);
  }
  if (replacementCount < newPhotos.length) {
    const fragment = document.createDocumentFragment();
    newPhotos.slice(replacementCount).forEach(p => fragment.appendChild(makePhotoThumb(p, state.albumPhotos)));
    grid.insertBefore(fragment, grid.firstChild);
  }
  state.albumLocateMissingBeforeCount = Math.max(0, albumPlaceholderCount() - replacementCount);
}

async function loadMoreAlbumBefore() {
  if (state.albumLoadingBefore || state.albumLoading || !state.albumHasBefore || !state.currentAlbum) return;
  const firstPhoto = state.albumPhotos[0];
  if (!firstPhoto || !firstPhoto.id) {
    state.albumHasBefore = false;
    clearAlbumPlaceholders();
    updateLoadMoreUI('load-more-top', false);
    return;
  }
  state.albumLoadingBefore = true;
  const anchorThumb = firstVisiblePhotoThumb();
  const anchorID = anchorThumb ? Number(anchorThumb.dataset.id || 0) : 0;
  const anchorTop = anchorThumb ? anchorThumb.getBoundingClientRect().top : 0;
  try {
    const page = await loadAlbumBeforeWindow(state.currentAlbum.id, firstPhoto.id);
    const prependPhotos = Array.isArray(page && page.photos) ? page.photos : [];
    if (!prependPhotos.length) {
      state.albumHasBefore = false;
      clearAlbumPlaceholders();
      updateLoadMoreUI('load-more-top', false);
      return;
    }
    const existing = new Set(state.albumPhotos.map(photo => photo && photo.id));
    const uniquePrepend = prependPhotos.filter(photo => photo && !existing.has(photo.id));
    if (!uniquePrepend.length) {
      state.albumHasBefore = !!(page && page.has_more);
      if (!state.albumHasBefore) clearAlbumPlaceholders();
      updateLoadMoreUI('load-more-top', state.albumHasBefore);
      return;
    }
    state.albumPhotos = [...uniquePrepend, ...state.albumPhotos];
    state.albumHasBefore = !!(page && page.has_more);
    prependAlbumGrid(uniquePrepend);
    if (!state.albumHasBefore) clearAlbumPlaceholders();
    void document.documentElement.offsetHeight;
    if (!state.albumLocatingActive) {
      requestVisibleThumbnailWarmup(`album:${state.currentAlbum && state.currentAlbum.id}`, uniquePrepend);
    }
    const refreshedAnchor = anchorID
      ? document.querySelector(`.photo-thumb[data-id="${anchorID}"]`)
      : null;
    const nextTop = refreshedAnchor ? refreshedAnchor.getBoundingClientRect().top : 0;
    const delta = nextTop - anchorTop;
    if (delta) {
      state.ignoreScrollSaveUntil = Date.now() + 120;
      state.programmaticScrollTargetTop = (window.scrollY || window.pageYOffset || 0) + delta;
      window.scrollBy(0, delta);
    }
  } catch (e) {
    if (!e || e.name !== 'AbortError') console.error(e);
  } finally {
    state.albumLoadingBefore = false;
    updateLoadMoreUI('load-more-top', state.albumHasBefore);
    if (!albumUsesManualBeforeLoading()) {
      requestAnimationFrame(() => {
        maybeTriggerAlbumBeforeLoadFromVisibleBoundary();
      });
    }
  }
}

async function focusPendingRandomAlbumPhoto({ quiet = false, skipScroll = false } = {}) {
  if (!state.pendingRandomAlbumPhotoID || state.view !== 'random-album') return false;
  const photoId = state.pendingRandomAlbumPhotoID;
  let thumb = findPhotoThumb(photoId);
  while (!thumb && state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused) {
    await loadMoreRandomAlbum();
    thumb = findPhotoThumb(photoId);
  }
  if (!thumb) {
    if (!quiet && !state.randomAlbumHasMore) {
      state.pendingRandomAlbumPhotoID = null;
      showToast(copyText('app.interface.s_19dc0a3725'));
    }
    return false;
  }
  state.pendingRandomAlbumPhotoID = null;
  state.randomAlbumLastViewedPhotoID = photoId;
  if (!skipScroll) thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  setFocusedPhoto(photoId, { scroll: false });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
  return true;
}

function focusPhotoThumbInCurrentView(photoId) {
  const thumb = findPhotoThumb(photoId);
  if (!thumb) return false;
  thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  setFocusedPhoto(photoId, { scroll: false });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
  return true;
}

function prepositionLightboxReturnTarget(photoId, returnView) {
  if (!photoId || !['timeline', 'album-detail', 'random-album', 'favorites'].includes(returnView)) return false;
  const thumb = findPhotoThumb(photoId);
  if (!thumb) return false;
  thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  return true;
}
function renderAlbumGroups(newPhotos) {
  const container = $('#album-groups');
  if (!container) return;
  if (state.albumPhotos.length === 0 && newPhotos.length === 0) {
    if (albumDirectChildCount(state.currentAlbum)) {
      container.innerHTML = '';
      return;
    }
    container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.empty.albumMedia'))}</p></div>`;
    return;
  }
  if (newPhotos.length && container.querySelector('.empty')) container.innerHTML = '';
  let grid = $('#album-grid');
  if (!grid) {
    container.innerHTML = '';
    grid = el('div', 'photo-grid');
    grid.id = 'album-grid';
    if (albumPlaceholderCount() > 0) {
      const placeholderFragment = document.createDocumentFragment();
      for (let i = 0; i < albumPlaceholderCount(); i += 1) {
        placeholderFragment.appendChild(makeAlbumPlaceholder(i));
      }
      grid.appendChild(placeholderFragment);
    }
    container.appendChild(grid);
  }
  const fragment = document.createDocumentFragment();
  newPhotos.forEach(p => fragment.appendChild(makePhotoThumb(p, state.albumPhotos)));
  grid.appendChild(fragment);
  updateLoadMoreUI('load-more-top', state.albumHasBefore);
  updateLoadMoreUI('load-more', state.albumHasMore);
}

// ── 回收站 (b-4 修复) ─────────────────────────────────
async function renderTrash() {
  state.trashContentFilter = normalizeTrashContentFilter(state.trashContentFilter);
  const writable = canWriteMedia();
  $('#topbar-title').textContent = copyText('app.interface.s_64ea8751fb');
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    writable ? renderTopbarGlassButton({ id: 'restore-all-trash-btn', icon: icons.topbarRestoreAll || icons.restore, label: copyText('app.interface.s_3d96157b6f') }) : '',
    writable ? renderTopbarGlassButton({ id: 'empty-trash-btn', icon: icons.topbarEmptyTrash, label: copyText('app.interface.s_4833ca01fc'), variant: 'danger' }) : '',
  ]);
  $('#topbar-meta').innerHTML = (writable ? renderTrashSelectionBarMarkup() + renderTrashFolderSelectionBarMarkup() : '')
    + renderTrashContentFilterControl();
  $('#topbar-actions').innerHTML = '';
  bindTrashContentFilterControl();
  if (writable) bindTrashSelectionBarHandlers();
  $('#restore-all-trash-btn')?.addEventListener('click', restoreAllTrash);
  $('#empty-trash-btn')?.addEventListener('click', emptyTrash);

  $('#content').innerHTML = `
<div id="trash-wrap"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>${copyText('app.interface.commonLoading')}</div>`;

  if (!state.trashLoaded) {
    state.trashPhotos = [];
    state.trashCursor = '';
    state.trashHasMore = true;
    state.trashLoading = false;
    await Promise.all([loadMoreTrash(), loadTrashAlbums()]);
  } else if (!state.trashAlbumsLoaded) {
    await loadTrashAlbums();
  } else {
    renderTrashGroups([], { reset: true });
  }
  syncTrashLoadMoreObserver();
  restoreViewScroll('trash');
}

async function loadTrashAlbums() {
  try {
    const response = await api.get('/api/media/trash/albums');
    const entries = Array.isArray(response) ? response : (response?.albums || []);
    const seen = new Set();
    state.trashAlbums = entries.filter(entry => {
      const groupID = String(entry?.group_id || '').trim();
      if (!groupID || seen.has(groupID)) return false;
      seen.add(groupID);
      return true;
    });
    state.trashAlbumsLoaded = true;
    renderTrashGroups([], { reset: true });
  } catch (e) {
    state.trashAlbumsLoaded = false;
    console.error("加载文件夹回收站失败:", e);
  }
}

async function loadMoreTrash() {
  if (state.trashLoading || !state.trashHasMore || state.trashContentFilter === 'folders') return;
  state.trashLoading = true;
  try {
    const params = new URLSearchParams();
    if (state.trashCursor) params.set('cursor', state.trashCursor);
    const query = params.toString();
    const url = '/api/media/trash' + (query ? `?${query}` : '');
    const page = await api.get(url);
    const photos = page.photos || [];
    state.trashPhotos.push(...photos);
    state.trashCursor = page.next_cursor || '';
    state.trashHasMore = page.has_more || false;
    state.trashLoaded = true;
    renderTrashGroups(photos);
    requestVisibleThumbnailWarmup('trash', photos);
  } catch (e) {
    console.error(e);
  } finally {
    state.trashLoading = false;
    syncTrashLoadMoreObserver();
    maybeLoadMoreImmediately('load-more', loadMoreTrash, () => state.trashContentFilter !== 'folders' && state.trashHasMore && !state.trashLoading);
  }
}

function renderTrashFolderCard(entry) {
  const groupID = String(entry?.group_id || '').trim();
  const name = String(entry?.name || '').trim() || copyText('app.interface.s_3143c44957');
  const path = String(entry?.source_rel_path || '').trim() || name;
  const mediaCount = Math.max(0, Number(entry?.photo_count) || 0);
  const childFolderCount = Math.max(0, (Number(entry?.folder_count) || 0) - 1);
  const coverUUID = String(entry?.cover_uuid || '').trim();
  const fallbackCover = `<span class="trash-folder-card-glyph-fallback"${coverUUID ? ' hidden' : ''}>${icons.albumCardFolder || icons.album || icons.trash}</span>`;
  const coverMarkup = coverUUID
    ? `<img loading="lazy" src="${mediaThumbURLFromUUID(coverUUID)}" alt="${escapeHTML(name)}" onerror="this.onerror=null;this.style.display='none';this.parentElement.classList.remove('has-cover');this.parentElement.querySelector('.trash-folder-card-glyph-fallback').hidden=false;">${fallbackCover}`
    : fallbackCover;
  const selected = state.trashFolderSelection.has(groupID);
  const card = el('article', `trash-folder-card${selected ? ' is-selected' : ''}`);
  card.dataset.groupID = groupID;
  card.tabIndex = 0;
  card.setAttribute('role', 'button');
  card.setAttribute('aria-pressed', selected ? 'true' : 'false');
  card.innerHTML = `
    <div class="trash-folder-card-glyph${coverUUID ? ' has-cover' : ''}">${coverMarkup}</div>
    <div class="trash-folder-card-body">
      <strong class="trash-folder-card-name">${escapeHTML(name)}</strong>
      <span class="trash-folder-card-path">${escapeHTML(path)}</span>
      <span class="trash-folder-card-meta">${escapeHTML(copyText('app.interface.runtime.trashFolderMeta', { mediaCount, folderCount: childFolderCount, date: formatDateTime(entry?.deleted_at) }))}</span>
    </div>
    <div class="trash-folder-card-actions">
      <button class="btn-icon" type="button" data-trash-folder-action="restore" title="${escapeHTML(copyText('app.interface.runtime.restoreFolder'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.restoreFolder'))}">${icons.topbarRestoreSelected}</button>
      <button class="btn-icon" type="button" data-trash-folder-action="delete" title="${escapeHTML(copyText('app.interface.runtime.deleteFolder'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.deleteFolder'))}">${icons.selectionDelete}</button>
    </div>`;
  card.addEventListener('click', event => {
    if (event.target.closest('button')) return;
    toggleTrashFolderSelection(groupID, card);
  });
  card.addEventListener('keydown', event => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    toggleTrashFolderSelection(groupID, card);
  });
  card.querySelector('[data-trash-folder-action="restore"]')?.addEventListener('click', event => {
    event.stopPropagation();
    restoreTrashFolder(groupID);
  });
  card.querySelector('[data-trash-folder-action="delete"]')?.addEventListener('click', event => {
    event.stopPropagation();
    hardDeleteTrashFolder(groupID);
  });
  return card;
}

function renderTrashGroups(newPhotos = [], { reset = false } = {}) {
  const container = $('#trash-wrap');
  if (!container) return;
  const filter = normalizeTrashContentFilter(state.trashContentFilter);
  const showFolders = filter !== 'media';
  const showMedia = filter !== 'folders';
  const photosForView = state.trashPhotos;
  const albumsForView = state.trashAlbums;
  // Do not show the empty state while the data source used by the current
  // filter is still loading. The media and folder requests run in parallel,
  // so either one may render first during the initial visit or a filter
  // switch.
  const visibleDataReady = (!showMedia || state.trashLoaded)
    && (!showFolders || state.trashAlbumsLoaded);

  if (reset) container.innerHTML = '';
  if (showFolders && albumsForView.length > 0) {
    let section = $('#trash-folder-section');
    if (!section) {
      section = el('section', 'trash-section trash-folder-section');
      section.id = 'trash-folder-section';
      section.innerHTML = `<div class="album-grid trash-folder-grid" id="trash-folder-grid"></div>`;
      container.prepend(section);
    }
    const grid = $('#trash-folder-grid');
    if (grid && (reset || grid.children.length !== albumsForView.length)) {
      grid.innerHTML = '';
      albumsForView.forEach(entry => grid.appendChild(renderTrashFolderCard(entry)));
    }
  } else {
    $('#trash-folder-section')?.remove();
  }

  if (showMedia) {
    let section = $('#trash-media-section');
    if (!section) {
      section = el('section', 'trash-section trash-media-section');
      section.id = 'trash-media-section';
      section.innerHTML = `<div class="photo-grid" id="trash-grid"></div>`;
      container.appendChild(section);
    }
    const grid = $('#trash-grid');
    if (grid) {
      const shouldBuildAll = reset;
      const items = shouldBuildAll ? photosForView : newPhotos;
      if (shouldBuildAll) grid.innerHTML = '';
      const fragment = document.createDocumentFragment();
      items.forEach(photo => fragment.appendChild(makePhotoThumb(photo, state.trashPhotos, { trashMode: true })));
      grid.appendChild(fragment);
    }
  } else {
    $('#trash-media-section')?.remove();
  }

  const hasVisibleContent = (showFolders && albumsForView.length > 0) || (showMedia && photosForView.length > 0);
  if (hasVisibleContent) {
    // A previous request may have rendered a temporary empty state before
    // the other parallel request returned. Remove it as soon as real content
    // becomes available instead of leaving it above the grid.
    container.querySelector('.empty')?.remove();
  } else if (visibleDataReady && !state.trashLoading) {
    container.innerHTML = `<div class="empty">${icons.trash}<p>${escapeHTML(copyText('app.empty.trash'))}</p></div>`;
    updateLoadMoreUI('load-more', false);
  }
  updateTrashFolderSelectionBar();
}

async function emptyTrash() {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!(await appConfirm(copyText('app.interface.s_6175f8b51e'), {
    title: copyText('app.interface.s_4833ca01fc'),
    confirmText: copyText('app.interface.s_a5c2dc91c6'),
    danger: true,
  }))) return;
  const button = $('#empty-trash-btn');
  if (button) button.disabled = true;
  try {
    await api.del('/api/media/trash');
    invalidateTrashViewState();
    state.trashLoaded = true;
    state.trashHasMore = false;
    clearSelection();
    await renderTrash();
    showToast(copyText('app.interface.s_0bb942936e'));
  } catch (e) {
    if (button) button.disabled = false;
    showToast(copyText('app.interface.s_e80d7fcdfc') + ((e && e.error) || e), 3200);
  }
}

function invalidateTrashViewState() {
  state.trashPhotos = [];
  state.trashCursor = '';
  state.trashHasMore = true;
  state.trashLoading = false;
  state.trashLoaded = false;
  state.trashAlbums = [];
  state.trashAlbumsLoaded = false;
  state.trashFolderSelection.clear();
}
function invalidateRestoredMediaViewState() {
  resetMediaFilteredViewState();
  state.timelineHasBefore = false;
  state.timelineLocateWindow = null;
  state.timelineLocateTargetIndex = -1;
  state.timelineLocateMissingBeforeCount = 0;
  state.albumHasBefore = false;
  state.albumLocateMissingBeforeCount = 0;
}
async function restorePhoto(id) {
  if (!canWriteMedia()) return forbidVisitorAction();
  try {
    await api.post(`/api/media/${id}/restore`, {});
    invalidateRestoredMediaViewState();
    invalidateTrashViewState();
    switchView('trash');
  }
  catch(e) { showToast(copyText('app.interface.s_0820e77972') + (e.error || e), 3200); }
}

// c-5: 更新回收站批量操作栏
function updateTrashSelBar() {
  updateSelectionBar({ barID: 'trash-sel-bar', countID: 'trash-sel-count' });
  updateSelectionModeUI();
}

function updateTrashFolderSelectionBar() {
  const bar = $('#trash-folder-sel-bar');
  if (!bar) return;
  const count = state.trashFolderSelection.size;
  bar.classList.toggle('visible', count > 0);
  const countEl = $('#trash-folder-sel-count');
  if (countEl) countEl.textContent = count;
  updateSelectionModeUI();
}

function toggleTrashFolderSelection(groupID, card) {
  const key = String(groupID || '').trim();
  if (!key) return;
  if (state.trashFolderSelection.has(key)) {
    state.trashFolderSelection.delete(key);
    card?.classList.remove('is-selected');
    card?.setAttribute('aria-pressed', 'false');
  } else {
    // Folder batches and individual media have different bulk operations.
    // Keep their selection sets mutually exclusive to avoid overlapping bars.
    clearTrashPhotoSelectionOnly();
    state.trashFolderSelection.add(key);
    card?.classList.add('is-selected');
    card?.setAttribute('aria-pressed', 'true');
  }
  updateTrashFolderSelectionBar();
}

// c-5: 批量恢复选中图片
async function restoreSelected() {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!state.selected.size) return;
  if (!(await appConfirm(copyText('app.interface.d_2a6decd7aa', { v0: (state.selected.size) })))) return;
  const ids = [...state.selected];
  clearSelection();
  for (const id of ids) {
		try { await api.post(`/api/media/${id}/restore`, {}); }
    catch(e) { console.error("恢复失败:", id, e); }
  }
  invalidateRestoredMediaViewState();
  invalidateTrashViewState();
  switchView('trash');
}

async function restoreTrashFolder(groupID) {
  if (!canWriteMedia()) return forbidVisitorAction();
  const key = String(groupID || '').trim();
  if (!key) return;
  if (!(await appConfirm(copyText('app.interface.s_5282dc370f')))) return;
  try {
    await api.post(`/api/media/trash/albums/${encodeURIComponent(key)}/restore`, {});
    invalidateRestoredMediaViewState();
    invalidateTrashViewState();
    showToast(copyText('app.interface.s_c33538af61'));
    switchView('trash');
  } catch (e) {
    showToast(copyText('app.interface.s_3d3385bd47') + (e.error || e), 3200);
  }
}

async function hardDeleteTrashFolder(groupID) {
  if (!canWriteMedia()) return forbidVisitorAction();
  const key = String(groupID || '').trim();
  if (!key) return;
  if (!(await appConfirm(copyText('app.interface.s_16a6901b3e'), {
    title: copyText('app.interface.s_f15a7b7211'),
    confirmText: copyText('app.interface.commonDeletePermanently'),
    danger: true,
  }))) return;
  try {
    await api.del(`/api/media/trash/albums/${encodeURIComponent(key)}`);
    invalidateTrashViewState();
    showToast(copyText('app.interface.s_361d029f77'));
    switchView('trash');
  } catch (e) {
    showToast(copyText('app.interface.s_5eda634952') + (e.error || e), 3200);
  }
}

async function restoreSelectedTrashFolders() {
  if (!canWriteMedia()) return forbidVisitorAction();
  const groups = [...state.trashFolderSelection];
  if (!groups.length) return;
  if (!(await appConfirm(copyText('app.interface.d_7f6baf36d2', { v0: (groups.length) })))) return;
  clearSelection();
  const failed = [];
  for (const groupID of groups) {
    try {
      await api.post(`/api/media/trash/albums/${encodeURIComponent(groupID)}/restore`, {});
    } catch (e) {
      failed.push(groupID);
      console.error("恢复文件夹失败:", groupID, e);
    }
  }
  invalidateRestoredMediaViewState();
  invalidateTrashViewState();
  showToast(failed.length ? copyText('app.interface.d_d282f7ab82', { v0: (groups.length - failed.length), v1: (failed.length) }) : copyText('app.interface.d_b9a3908fe3', { v0: (groups.length) }));
  switchView('trash');
}

async function hardDeleteSelectedTrashFolders() {
  if (!canWriteMedia()) return forbidVisitorAction();
  const groups = [...state.trashFolderSelection];
  if (!groups.length) return;
  if (!(await appConfirm(copyText('app.interface.d_1015d1691d', { v0: (groups.length) }), {
    title: copyText('app.interface.s_47fe869bb9'),
    confirmText: copyText('app.interface.commonDeletePermanently'),
    danger: true,
  }))) return;
  clearSelection();
  const failed = [];
  for (const groupID of groups) {
    try {
      await api.del(`/api/media/trash/albums/${encodeURIComponent(groupID)}`);
    } catch (e) {
      failed.push(groupID);
      console.error("永久删除文件夹失败:", groupID, e);
    }
  }
  invalidateTrashViewState();
  showToast(failed.length ? copyText('app.interface.d_91f07a5621', { v0: (groups.length - failed.length), v1: (failed.length) }) : copyText('app.interface.d_05944eb430', { v0: (groups.length) }));
  switchView('trash');
}

async function collectTrashPhotoIDs() {
  const ids = [];
  const seen = new Set();
  const appendPageIDs = photos => {
    (photos || []).forEach(photo => {
      const id = Number(photo && photo.id);
      if (!id || seen.has(id)) return;
      seen.add(id);
      ids.push(id);
    });
  };
  appendPageIDs(state.trashPhotos);
  let cursor = state.trashCursor || '';
  let hasMore = !!state.trashHasMore;
  while (hasMore) {
    const params = new URLSearchParams();
    if (cursor) params.set('cursor', cursor);
    const query = params.toString();
    const page = await api.get('/api/media/trash' + (query ? `?${query}` : ''));
    appendPageIDs(page.photos);
    cursor = page.next_cursor || '';
    hasMore = !!page.has_more;
  }
  return ids;
}

async function restoreAllTrash() {
  if (!canWriteMedia()) return forbidVisitorAction();
  try {
    const ids = await collectTrashPhotoIDs();
    const groups = [...new Set((state.trashAlbums || []).map(entry => String(entry?.group_id || '').trim()).filter(Boolean))];
    if (!ids.length && !groups.length) {
      showToast(copyText('app.interface.s_61e2f22b71'));
      return;
    }
    if (!(await appConfirm(copyText('app.interface.d_d3479c16f0', { v0: (groups.length), v1: (ids.length) })))) return;
    for (const groupID of groups) {
      try {
        await api.post(`/api/media/trash/albums/${encodeURIComponent(groupID)}/restore`, {});
      } catch (e) {
        console.error("恢复文件夹失败:", groupID, e);
      }
    }
    for (const id of ids) {
      try {
        await api.post(`/api/media/${id}/restore`, {});
      } catch (e) {
        console.error("恢复失败:", id, e);
      }
    }
    invalidateRestoredMediaViewState();
    showToast(copyText('app.interface.d_579c0d07b5', { v0: (groups.length), v1: (ids.length) }));
    switchView('trash');
  } catch (e) {
    showToast(copyText('app.interface.s_0ee497ecfb') + ((e && e.error) || e.message || e), 3200);
  }
}

async function hardDeleteSelected() {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!state.selected.size) return;
  const ids = [...state.selected];
  if (!(await appConfirm(copyText('app.interface.d_bbde533e4a', { v0: (ids.length) }), {
    title: copyText('app.interface.s_40e1ba2e1e'),
    confirmText: copyText('app.interface.commonDeletePermanently'),
    danger: true,
  }))) return;

  const button = $('#hard-delete-sel-btn');
  if (button) button.disabled = true;
  const deleted = [];
  const failed = [];
  for (const id of ids) {
    try {
      await api.del(`/api/media/trash/${id}`);
      deleted.push(id);
    } catch (e) {
      failed.push(id);
      console.error("永久删除失败:", id, e);
    }
  }
  removeTrashPhotosFromUI(deleted);
  if (button && button.isConnected) button.disabled = false;
  if (failed.length) {
    showToast(copyText('app.interface.d_097965878b', { v0: (deleted.length), v1: (failed.length) }), 3200);
  } else if (deleted.length) {
    showToast(copyText('app.interface.d_27ddbb2168', { v0: (deleted.length) }));
  }
}

// ── 无限滚动 ──────────────────────────────────────────
function observeLoadMore(id, loadFn, canLoad, options = {}) {
  disconnectSingleLoadMoreObserver(id);
  const sentinel = document.getElementById(id);
  if (!sentinel) return;
  const observer = new IntersectionObserver(entries => {
    if (entries[0].isIntersecting && canLoad()) loadFn();
  }, { rootMargin: options.rootMargin || '200px' });
  observer.observe(sentinel);
  state.loadMoreObservers[id] = observer;
}
function updateLoadMoreUI(id, hasMore) {
  const e = document.getElementById(id);
  if (!e) return;
  e.style.display = hasMore ? '' : 'none';
}

// 当首屏内容不足、底部哨兵一直在可视区时，IntersectionObserver 可能只触发一次，
// 这里在每次加载结束后主动检查一次，确保能继续把后续页面补出来。
function maybeLoadMoreImmediately(id, loadFn, canLoad) {
  if (state.blockingInteraction) return;
  const sentinel = document.getElementById(id);
  if (!sentinel || !canLoad()) return;
  const rect = sentinel.getBoundingClientRect();
  if (rect.top <= window.innerHeight + 200) {
    requestAnimationFrame(() => {
      if (canLoad()) loadFn();
    });
  }
}

function maybeLoadMoreImmediatelyFromTop(id, loadFn, canLoad) {
  if (state.blockingInteraction) return;
  const sentinel = document.getElementById(id);
  if (!sentinel || !canLoad()) return;
  const rect = sentinel.getBoundingClientRect();
  if (rect.bottom >= -200 && rect.top <= 200) {
    requestAnimationFrame(() => {
      if (canLoad()) loadFn();
    });
  }
}

// ── 灯箱 ──────────────────────────────────────────────
function renderLightbox() {
  if (!window.EchoGalleryLightbox || typeof window.EchoGalleryLightbox.renderShell !== 'function') {
    return `<div class="lightbox" id="lightbox" tabindex="-1"><div class="lightbox-loading show" id="lb-loading"><div class="spinner"></div><span>${copyText('app.interface.s_lightbox_failed')}</span></div></div>`;
  }
  return window.EchoGalleryLightbox.renderShell({
    mode: 'app',
    icons,
    canWriteMedia: canWriteMedia(),
    canManageShareLinks: canManageShareLinks(),
  });
}
function updateLightboxHeaderLayout() {
  const header = $('#lightbox-header');
  const main = header ? $('.lightbox-header-main', header) : null;
  const controls = header ? $('.lightbox-header-controls', header) : null;
  const slideshow = header ? $('.lightbox-slideshow-controls', header) : null;
  const actions = header ? $('.lightbox-action-group', header) : null;
  const zoom = header ? $('.lightbox-zoom-panel', header) : null;
  if (!header || !main || !controls || !slideshow || !actions || !zoom) return;
  const applyMode = mode => {
    header.classList.toggle('lightbox-hide-slideshow-controls', mode >= 1);
    header.classList.toggle('lightbox-hide-zoom-controls', mode >= 2);
    const forceCompact = isMobileLayout() || (isTouchLikeDevice() && currentLightboxViewportOrientation() === 'portrait');
    header.classList.toggle('lightbox-compact-controls', mode >= 3 || forceCompact);
    main.style.maxWidth = '';
  };
  const constrainTitle = () => {
    const headerRect = header.getBoundingClientRect();
    const actionRect = actions.getBoundingClientRect();
    const controlsVisible = getComputedStyle(controls).display !== 'none' && controls.offsetWidth > 0;
    const controlsRect = controlsVisible ? controls.getBoundingClientRect() : null;
    const rightBoundary = controlsRect ? controlsRect.left : actionRect.left;
    const maxWidth = Math.max(0, Math.floor(rightBoundary - headerRect.left - 16));
    main.style.maxWidth = `${maxWidth}px`;
  };
  const hasOverlap = () => {
    const headerRect = header.getBoundingClientRect();
    const visible = [main, controls, actions].filter(node => {
      const style = getComputedStyle(node);
      return style.display !== 'none' && node.offsetWidth > 0;
    });
    const rects = visible.map(node => node.getBoundingClientRect());
    if (rects.some(rect => rect.left < headerRect.left - 1 || rect.right > headerRect.right + 1)) return true;
    for (let i = 0; i < rects.length - 1; i += 1) {
      if (rects[i].right + 16 > rects[i + 1].left) return true;
    }
    return false;
  };
  if (isMobileLayout() || (isTouchLikeDevice() && currentLightboxViewportOrientation() === 'portrait')) {
    applyMode(3);
    return;
  }
  for (let mode = 0; mode <= 3; mode += 1) {
    applyMode(mode);
    constrainTitle();
    if (!hasOverlap()) return;
  }
}
function refreshLightboxUiActivity() {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open')) return;
  lightbox.classList.remove('ui-idle');
  clearTimeout(state.lightboxUiIdleTimer);
  const idleSeconds = lightboxUIIdleSeconds();
  if (idleSeconds > 0) {
    state.lightboxUiIdleTimer = setTimeout(() => {
      const current = $('#lightbox');
      if (!current || !current.classList.contains('open')) return;
      current.classList.add('ui-idle');
    }, idleSeconds * 1000);
  }
}
function clearLightboxUiIdleTimer() {
  clearTimeout(state.lightboxUiIdleTimer);
  state.lightboxUiIdleTimer = null;
  clearTimeout(state.lightboxInfoInteractionTimer);
  state.lightboxInfoInteractionTimer = null;
  $('#lightbox')?.classList.remove('ui-idle', 'lightbox-info-interacting');
}
function markLightboxInfoInteraction(settleMs = 1100) {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open')) return;
  lightbox.classList.add('lightbox-info-interacting');
  clearTimeout(state.lightboxInfoInteractionTimer);
  state.lightboxInfoInteractionTimer = setTimeout(() => {
    $('#lightbox')?.classList.remove('lightbox-info-interacting');
  }, settleMs);
}

function lightboxIsInteractive() {
  const lightbox = $('#lightbox');
  return !!lightbox && lightbox.classList.contains('open') && !document.hidden;
}

function shouldPauseLightboxBackgroundWork() {
  return !!$('#lightbox')?.classList.contains('open');
}

function lockPageScrollForLightbox() {
  if (document.body.classList.contains('lightbox-page-lock')) return;
  state.lightboxPageScrollTop = window.scrollY || window.pageYOffset || 0;
  // 暂时禁用滚动锁定；保留滚动位置记录以便后续恢复。
}
function unlockPageScrollForLightbox() {
  if (state.lightboxSkipScrollRestore) {
    state.lightboxSkipScrollRestore = false;
    state.lightboxPageScrollTop = 0;
    return;
  }
  const top = Number(state.lightboxPageScrollTop) || 0;
  window.scrollTo(0, top);
  state.lightboxPageScrollTop = 0;
}
function handleLightboxVideoAppHidden() {
  const lightbox = $('#lightbox');
  const video = $('#lb-video');
  if (!lightbox || !lightbox.classList.contains('open') || !video || video.classList.contains('hidden')) return;
  clearPendingVideoSeekCommit();
  saveCurrentVideoResumePosition({ quiet: true });
  flushPendingVideoPlaybackPreference(undefined, { keepalive: true });
  state.lightboxVisibilityResumeTime = Math.max(0, Number(video.currentTime) || 0);
  state.lightboxVisibilityResumePending = !video.paused;
  if (!video.paused) video.pause();
}
function handleLightboxVideoAppVisible() {
  if (typeof echoFrame !== 'undefined' && echoFrame) return;
  const lightbox = $('#lightbox');
  const video = $('#lb-video');
  if (!lightbox || !lightbox.classList.contains('open') || !video || video.classList.contains('hidden')) {
    state.lightboxVisibilityResumePending = false;
    return;
  }
  void restoreVideoAudioOutput(video);
  if (!state.lightboxVisibilityResumePending) return;
  const resumeTime = Math.max(0, Number(state.lightboxVisibilityResumeTime) || 0);
  state.lightboxVisibilityResumePending = false;
  requestAnimationFrame(() => {
    if (resumeTime > 0 && Number.isFinite(video.duration) && video.duration > 0) {
      video.currentTime = Math.min(Math.max(0, resumeTime), Math.max(0, video.duration - 0.2));
    }
    void restoreVideoAudioOutput(video).finally(() => {
      video.play().catch(() => {});
    });
  });
}
function bindGlobal() {
  $('#rename-media-cancel')?.addEventListener('click', closeRenameMediaModal);
  $('#rename-media-save')?.addEventListener('click', () => void submitRenameMedia());
  $('#rename-media-name')?.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); void submitRenameMedia(); }
    if (e.key === 'Escape') { e.preventDefault(); closeRenameMediaModal(); }
  });
  $('#rename-media-modal')?.addEventListener('click', e => {
    if (e.target === e.currentTarget) closeRenameMediaModal();
  });
  bindHoldAction($('#lb-favorite'), e => {
    e.preventDefault();
    e.stopPropagation();
    void toggleFavorite(state.lightboxPhotos[state.lightboxIndex], {
      mode: 'hold',
      sourceButton: $('#lb-favorite'),
    });
  });
  bindHoldAction($('#lb-video-bookmark'), e => {
    e.preventDefault();
    e.stopPropagation();
    const photo = state.lightboxPhotos[state.lightboxIndex];
    const result = addVideoBookmarkAtCurrentTime(photo, $('#lb-video'));
    if (result.ok) {
      showToast(copyText('app.interface.d_9dd0a13c2e', { v0: (result.slot) }));
      refreshVideoBookmarkMenu();
      showVideoProgressActivity();
      return;
    }
    if (result.reason === 'full') showToast(copyText('app.interface.s_088a54f63c'));
    else if (result.reason === 'nearby') showToast(copyText('app.interface.s_d92ab6cef4'));
    else showToast(copyText('app.interface.s_e20389c83b'));
  });
  document.addEventListener('click', e => {
    if (!e.target.closest('.context-menu')) closeContextMenu();
  });
  window.addEventListener('scroll', noteManualViewScroll, { passive: true });
  window.addEventListener('keydown', e => {
    if (typeof handleEchoFrameKeydown === 'function' && handleEchoFrameKeydown(e)) return;
    if (handleSearchShortcut(e)) return;
    if (handleViewNumberShortcut(e)) return;
    if (handleAlbumSwitchShortcut(e)) return;
    if (handleDeleteShortcut(e)) return;
    if (handlePhotoGridKeyboard(e)) return;
    if (e.key === 'Escape') {
      if (state.searchOpen) {
        e.preventDefault();
        closeGlobalSearch();
        return;
      }
      if (_ctxMenu) {
        closeContextMenu();
        return;
      }
      if (!$('#lightbox').classList.contains('open') && state.view === 'album-detail' && !document.querySelector('.modal-overlay.open')) {
        e.preventDefault();
        returnToAlbumParent();
        return;
      }
      closeContextMenu();
    }
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.altKey || e.key === 'Alt') syncLightboxTemporaryZoom(true);
    if (handleLightboxKeydown(e)) return;
  }, true);
  window.addEventListener('keyup', e => {
    if (typeof echoFrame !== 'undefined' && echoFrame) return;
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.key === 'f' || e.key === 'F') {
      e.preventDefault();
      e.stopImmediatePropagation();
      finishLightboxFavoriteKeyHold();
      return;
    }
    if (!e.altKey && e.key === 'Alt') syncLightboxTemporaryZoom(false);
  }, true);
  window.addEventListener('blur', () => {
    clearLightboxFavoriteKeyHoldState();
    resetLightboxTemporaryZoom();
    handleLightboxVideoAppHidden();
  });
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      clearLightboxFavoriteKeyHoldState();
      handleLightboxVideoAppHidden();
    }
    else {
      refreshPrimaryScreenOrientationSoon();
      handleLightboxVideoAppVisible();
    }
  });
  window.addEventListener('pagehide', () => {
    clearLightboxFavoriteKeyHoldState();
    handleLightboxVideoAppHidden();
  });
  window.addEventListener('pageshow', () => {
    refreshPrimaryScreenOrientationSoon();
    handleLightboxVideoAppVisible();
  });
  window.addEventListener('orientationchange', () => {
    refreshPrimaryScreenOrientationSoon();
    refreshDeviceClassesSoon();
    scheduleLightboxViewportChangeCheck(true);
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', () => {
      refreshDeviceClassesSoon();
      scheduleLightboxViewportChangeCheck(false);
    });
  }
  document.addEventListener('fullscreenchange', () => {
    if (typeof echoFrame !== 'undefined' && echoFrame) return;
    if (typeof echoFrameFullscreenExitUntil !== 'undefined' && Date.now() < echoFrameFullscreenExitUntil) return;
    const lightbox = $('#lightbox');
    if (!lightbox || !lightbox.classList.contains('open')) return;
    if (!document.fullscreenElement) exitLightboxToContext();
  });
  document.addEventListener('gesturestart', e => e.preventDefault(), { passive: false });
  document.addEventListener('gesturechange', e => e.preventDefault(), { passive: false });
  document.addEventListener('gestureend', e => e.preventDefault(), { passive: false });
  document.addEventListener('touchstart', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    if (e.touches && e.touches.length >= 2) beginTouchPinch(e);
  }, { passive: false });
  document.addEventListener('touchmove', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    if (e.touches && e.touches.length >= 2) {
      updateTouchPinch(e);
    }
  }, { passive: false });
  document.addEventListener('touchend', endTouchPinch, { passive: false });
  document.addEventListener('touchcancel', endTouchPinch, { passive: false });
  document.addEventListener('mousemove', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    if (!$('#lightbox').classList.contains('open')) return;
    updateLightboxFocusPointFromPointer(e.clientX, e.clientY);
    if (lightboxUIIdleSeconds() > 0) refreshLightboxUiActivity();
    if (state.lightboxBoostActive && state.lightboxPointerInside) applyLightboxZoom();
  }, { passive: true });
  document.addEventListener('scroll', e => {
    if (e.target?.closest?.('#lb-info')) markLightboxInfoInteraction(1300);
  }, true);
  document.addEventListener('wheel', e => {
    if (e.target?.closest?.('#lb-info')) markLightboxInfoInteraction(1300);
  }, { passive: true, capture: true });
  document.addEventListener('click', e => {
    if (Date.now() < state.lightboxSwipeClickSuppressUntil) {
      if (e.target?.closest?.('.echo-frame')) return;
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    if (e.target?.closest?.('.echo-frame')) return;
    if (e.target.closest('#lb-close')) exitLightboxToContext();
    if (e.target.closest('#lb-prev')) void lbNav(-1);
    if (e.target.closest('#lb-next')) void lbNav(1);
    const touchPlaybackToggle = e.target.closest('#lb-touch-playback-toggle');
    if (touchPlaybackToggle) {
      e.preventDefault();
      e.stopPropagation();
      touchPlaybackToggle.classList.add('is-pressed');
      setTimeout(() => touchPlaybackToggle.classList.remove('is-pressed'), 170);
      toggleVideoPlayback();
      return;
    }
    if (e.target.closest('#lb-download')) downloadCurrentPhoto();
    if (e.target.closest('#lb-video-bookmark')) showVideoBookmarkMenu();
    if (e.target.closest('#lb-favorite')) toggleCurrentLightboxFavorite();
    if (e.target.closest('#lb-share')) lbShare();
    if (e.target.closest('#lb-more')) showCurrentLightboxContextMenu();
    if (e.target.closest('#lb-fit-height')) setLightboxFit();
    if (e.target.closest('#lb-slideshow-toggle')) toggleSlideshow();
    const progressMarker = e.target.closest('#lb-video-progress [data-video-bookmark-slot], #lb-video-progress [data-video-section]');
    if (progressMarker) {
      e.preventDefault();
      e.stopPropagation();
      jumpVideoToProgressMarker(progressMarker);
      return;
    }
    const bookmarkDelete = e.target.closest('[data-video-bookmark-delete]');
    if (bookmarkDelete) {
      e.preventDefault();
      e.stopPropagation();
      const slot = Number(bookmarkDelete.dataset.videoBookmarkDelete);
      const photo = state.lightboxPhotos[state.lightboxIndex];
      if (slot && deleteVideoBookmarkSlot(photo, slot)) {
        showToast(copyText('app.interface.d_72a35bb91f', { v0: (slot) }));
        refreshVideoBookmarkMenu();
      }
      return;
    }
    const bookmarkAction = e.target.closest('[data-video-bookmark-slot]');
    if (bookmarkAction) {
      const slot = Number(bookmarkAction.dataset.videoBookmarkSlot);
      const photo = state.lightboxPhotos[state.lightboxIndex];
      const video = $('#lb-video');
      const bookmark = getVideoBookmarkSlot(photo, slot);
      if (slot && video && !video.classList.contains('hidden')) {
        if (bookmark) {
          video.currentTime = bookmark.time;
          updateVideoBookmarkProgress(video, photo);
          showToast(copyText('app.interface.d_87eed94c06', { v0: (slot), v1: (bookmark.name ? `：${bookmark.name}` : '') }));
        } else if (canEditBookmarks() && setVideoBookmarkSlot(photo, slot, video.currentTime || 0)) {
          showToast(copyText('app.interface.d_9dd0a13c2e', { v0: (slot) }));
          refreshVideoBookmarkMenu();
        } else if (canEditBookmarks()) {
          showToast(copyText('app.interface.s_d92ab6cef4'));
        }
      }
    }
    const progressTrack = e.target.closest('.lightbox-video-progress-track');
    if (progressTrack) {
      seekVideoFromProgressClientX(progressTrack, e.clientX);
      return;
    }
    if (shouldStopSlideshowFromClick(e.target)) stopSlideshow();
    if (shouldToggleLightboxUiFromClick(e.target)) {
      toggleLightboxUiIdle();
      return;
    }
    if (e.target.closest('#global-search-btn, #floating-search-btn')) openGlobalSearch();
    if (e.target.closest('#search-close-btn')) closeGlobalSearch();
    if (e.target.closest('#search-overlay') && !e.target.closest('.search-panel')) closeGlobalSearch();
    if (e.target.closest('#search-more-btn') && state.searchHasMore) runGlobalSearch(state.searchQuery, { reset: false });
    const filterBtn = e.target.closest('[data-search-filter]');
    if (filterBtn) {
      state.searchFilter = filterBtn.dataset.searchFilter || 'all';
      $$('.search-filter').forEach(btn => btn.classList.toggle('active', btn === filterBtn));
      const query = $('#global-search-input')?.value.trim() || '';
      resetSearchResults(query);
      renderSearchResults();
      if (query) runGlobalSearch(query, { reset: true });
    }
  });
  document.addEventListener('pointerdown', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    const rangeInput = e.target?.closest?.('input[type="range"]');
    if (rangeInput && setRangeValueFromPointer(rangeInput, e.clientX)) {
      state.activeRangePointer = {
        id: e.pointerId,
        input: rangeInput,
      };
      if (rangeInput.setPointerCapture) {
        try { rangeInput.setPointerCapture(e.pointerId); } catch (_) {}
      }
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.target?.closest?.('#lb-info')) markLightboxInfoInteraction(1400);
    state.lightboxLastPointerType = e.pointerType || '';
    const progressTrack = e.target.closest('.lightbox-video-progress-track');
    if (progressTrack) beginVideoProgressScrub(e, progressTrack);
    else {
      if (beginVideoSurfaceSwipe(e)) return;
      else beginLightboxSwipe(e);
    }
  }, true);
  document.addEventListener('pointermove', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    if (state.activeRangePointer && state.activeRangePointer.id === e.pointerId) {
      setRangeValueFromPointer(state.activeRangePointer.input, e.clientX);
      e.preventDefault();
      return;
    }
    if ($('#lightbox').classList.contains('open') && e.target?.closest?.('#lb-info')) markLightboxInfoInteraction(1200);
    if (state.videoSurfaceSwipe && updateVideoSurfaceSwipe(e)) return;
    if (state.lightboxSwipe && updateLightboxSwipe(e)) return;
    if (state.videoProgressScrubPointerId != null) updateVideoProgressScrub(e);
  }, true);
  document.addEventListener('pointerup', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    if (state.activeRangePointer && state.activeRangePointer.id === e.pointerId) {
      const input = state.activeRangePointer.input;
      setRangeValueFromPointer(input, e.clientX, { commit: true });
      if (input && input.releasePointerCapture) {
        try { input.releasePointerCapture(e.pointerId); } catch (_) {}
      }
      state.activeRangePointer = null;
      e.preventDefault();
      return;
    }
    if (state.videoSurfaceSwipe) {
      if (endVideoSurfaceSwipe(e)) return;
    }
    if (state.lightboxSwipe) {
      void endLightboxSwipe(e);
      return;
    }
    if (state.videoProgressScrubPointerId != null) endVideoProgressScrub(e);
  }, true);
  document.addEventListener('pointercancel', e => {
    if (e.target?.closest?.('.echo-frame')) return;
    if (state.activeRangePointer && state.activeRangePointer.id === e.pointerId) {
      const input = state.activeRangePointer.input;
      if (input && input.releasePointerCapture) {
        try { input.releasePointerCapture(e.pointerId); } catch (_) {}
      }
      state.activeRangePointer = null;
      return;
    }
    if (state.videoSurfaceSwipe) {
      if (endVideoSurfaceSwipe(e)) return;
    }
    if (state.lightboxSwipe) {
      void endLightboxSwipe(e);
      return;
    }
    if (state.videoProgressScrubPointerId != null) endVideoProgressScrub(e);
  }, true);
  document.addEventListener('pointerup', e => {
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.target.closest('#lb-video')) refocusLightboxAfterVideoControl();
  }, true);
  $('#lb-video')?.addEventListener('volumechange', e => {
    const video = e.currentTarget;
    if (!video || video.classList.contains('hidden')) return;
    showVolumeOverlay(video);
    persistVideoPlaybackPreferenceSoon(video);
  });
  document.addEventListener('input', e => {
    if (e.target.matches('input[type="range"]')) updateRangeProgress(e.target);
    if (e.target.matches('#floating-grid-scale-input')) {
      setGridScale(e.target.value);
      persistGridScaleSoon();
      return;
    }
    if (e.target.matches('#lb-slideshow-interval')) updateSlideshowSetting('interval', parseInt(e.target.value, 10) * 1000);
    if (e.target.matches('#lb-zoom')) setLightboxZoom(parseInt(e.target.value, 10));
    if (e.target.matches('#global-search-input')) {
      syncSearchInputs(e.target.value);
      scheduleGlobalSearch();
    }
  });
  document.addEventListener('change', async e => {
    if (!e.target.matches('#library-build-exit-after')) return;
    try {
      state.libraryBuildStatus = await setLibraryBuildExitAfterComplete(e.target.checked);
      renderView();
    } catch (err) {
      e.target.checked = !e.target.checked;
      alert(copyText('app.interface.settingsSaveFailedPrefix') + ((err && err.error) || err));
    }
  });
  document.addEventListener('click', async e => {
    const btn = e.target.closest('#library-build-cancel');
    if (!btn) return;
    btn.disabled = true;
    btn.textContent = copyText('app.libraryBuild.cancelling');
    try {
      state.libraryBuildStatus = await cancelLibraryBuild();
      renderView();
      startLibraryBuildPolling();
    } catch (err) {
      btn.disabled = false;
      btn.textContent = copyText('app.libraryBuild.cancel');
      alert(copyText('app.interface.s_b769765b66') + ((err && err.error) || err));
    }
  });
  document.addEventListener('contextmenu', e => {
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.target.closest('.lb-nav')) return;
    if (!e.target.closest('#lb-img, #lb-video, .lightbox-body')) return;
    e.preventDefault();
    const photo = state.lightboxPhotos[state.lightboxIndex];
    if (!photo) return;
    showPhotoContextMenu(e.clientX, e.clientY, photo, findPhotoThumb(photo.id), state.lightboxPhotos, { lightbox: true, mobile: isTouchLikeDevice() });
  });
  document.addEventListener('change', e => {
    if (e.target.matches('#lb-slideshow-interval')) updateSlideshowSetting('interval', parseInt(e.target.value, 10) * 1000);
    if (e.target.matches('#lb-zoom')) setLightboxZoom(parseInt(e.target.value, 10));
  });
  document.addEventListener('click', e => {
    const zoomStep = e.target.closest('[data-lightbox-zoom-step]');
    if (zoomStep) {
      e.preventDefault();
      e.stopPropagation();
      stepLightboxZoom(Number(zoomStep.dataset.lightboxZoomStep));
      return;
    }
    if (e.target.closest('#lb-slideshow-loop')) {
      updateSlideshowSetting('loop', !state.slideshowLoop);
    }
  });
  document.addEventListener('keydown', e => {
    const zoomStep = e.target.closest?.('[data-lightbox-zoom-step]');
    if (!zoomStep || (e.key !== 'Enter' && e.key !== ' ')) return;
    e.preventDefault();
    stepLightboxZoom(Number(zoomStep.dataset.lightboxZoomStep));
  });
}

function handlePhotoGridKeyboard(e) {
  if (state.searchOpen || state.blockingInteraction || document.querySelector('.modal-overlay.open')) return false;
  if ($('#lightbox')?.classList.contains('open')) return false;
  if (shouldIgnoreGlobalShortcut(e.target)) return false;
  const key = e.key.toLowerCase();
  const map = { w: 'up', a: 'left', s: 'down', d: 'right' };
  if (map[key]) {
    e.preventDefault();
    return focusPhotoByGridStep(map[key]);
  }
  if ((e.key === 'Enter' || e.key === ' ') && state.focusedPhotoID) {
    e.preventDefault();
    return openFocusedPhoto();
  }
  return false;
}
function ensureAppleStatusBarStyle() {
  let meta = document.querySelector('meta[name="apple-mobile-web-app-status-bar-style"]');
  if (!meta) {
    meta = document.createElement('meta');
    meta.name = 'apple-mobile-web-app-status-bar-style';
    document.head.appendChild(meta);
  }
  meta.content = 'black-translucent';
}
function shouldAttemptLightboxFullscreen() {
  return isTouchLikeDevice() || isMobileLayout();
}
function lightboxFullscreenElement() {
  return document.fullscreenElement || document.webkitFullscreenElement || document.msFullscreenElement || null;
}
function lightboxIsFullscreenTarget() {
  const lightbox = $('#lightbox');
  return !!lightbox && lightboxFullscreenElement() === lightbox;
}
async function enterLightboxFullscreen() {
  const lightbox = $('#lightbox');
  if (!lightbox || !lightbox.classList.contains('open') || !shouldAttemptLightboxFullscreen()) return false;
  if (lightboxIsFullscreenTarget()) return true;
  const request = lightbox.requestFullscreen || lightbox.webkitRequestFullscreen || lightbox.msRequestFullscreen;
  if (typeof request !== 'function') return false;
  try {
    const result = request.call(lightbox, { navigationUI: 'hide' });
    if (result && typeof result.then === 'function') await result;
    return true;
  } catch (_) {
    return false;
  }
}
async function exitLightboxFullscreen() {
  if (!lightboxFullscreenElement()) return;
  try {
    if (typeof document.exitFullscreen === 'function') {
      await document.exitFullscreen();
      return;
    }
    if (typeof document.webkitExitFullscreen === 'function') {
      document.webkitExitFullscreen();
      return;
    }
    if (typeof document.msExitFullscreen === 'function') {
      document.msExitFullscreen();
    }
  } catch (_) {
  }
}
function applyLightboxStatusBarSurface() {
  ensureAppleStatusBarStyle();
  syncLightboxViewportGeometry();
  let themeMeta = document.querySelector('meta[name="theme-color"]');
  state.lightboxHadThemeColorMeta = !!themeMeta;
  state.lightboxPreviousThemeColor = themeMeta ? themeMeta.content : '';
  if (!themeMeta) {
    themeMeta = document.createElement('meta');
    themeMeta.name = 'theme-color';
    document.head.appendChild(themeMeta);
  }
  themeMeta.content = '#000000';
  document.documentElement.classList.add('lightbox-open');
}
function restoreLightboxStatusBarSurface() {
  document.documentElement.classList.remove('lightbox-open');
  document.documentElement.style.removeProperty('--eg-lightbox-viewport-height');
  const themeMeta = document.querySelector('meta[name="theme-color"]');
  if (!themeMeta) return;
  if (!state.lightboxHadThemeColorMeta) {
    themeMeta.remove();
    return;
  }
  if (state.lightboxPreviousThemeColor) {
    themeMeta.content = state.lightboxPreviousThemeColor;
  }
}
async function openLightbox(photos, index, options = {}) {
  state.lightboxLastRememberedPhotoID = null;
  interruptThumbnailBuildForForegroundTask();
  const target = photos[Math.max(0, index)];
  if (target && isVideoMedia(target)) {
    await ensureSettingsDataLoaded();
    if (state.serverSettings.use_system_player) {
      openWithSystemPlayer(target.id)
        .then(() => {
          rememberPhoto(target);
          showToast(copyText('app.interface.s_e2f6b6217c'));
        })
        .catch(e => alert(copyText('app.interface.s_62bf2d195f') + ((e && e.error) || e)));
      return;
    }
  }
  applyLightboxStatusBarSurface();
  clearLightboxUnderzoomState();
  resetLightboxDismissProxy();
  clearLightboxDismissFade();
  clearLightboxDismissGesture();
  resetLightboxZoomToDefault();
  state.lightboxPhotos = photos;
  state.lightboxIndex  = Math.max(0, index);
  state.lightboxSeedPhotoID = target && target.id ? Number(target.id) : null;
  state.lightboxSeedPreviewURL = currentLoadedPreviewURLForPhoto(target);
  state.lightboxOriginThumbRect = lightboxDismissPhotoThumbRect(target);
  state.lightboxReturnView = options.returnView || state.view;
  state.lightboxReturnAlbumID = state.currentAlbumID;
  state.lightboxDebugControls = !!options.debugControls;
  if (state.lightboxReturnView === 'random-album' && target && target.id) {
    state.randomAlbumLastViewedPhotoID = Number(target.id);
  }
  state.slideshowRandomQueue = [];
  state.videoSurfaceSwipe = null;
  state.lightboxSwipe = null;
  state.lightboxSwipeClickSuppressUntil = 0;
  state.lightboxPinchZoomActive = false;
  state.lightboxViewportOrientation = currentLightboxViewportOrientation();
  resetLightboxSwipeVisual();
  resetLightboxFocusPoint();
  resetLightboxTemporaryZoom();
  setLightboxMediaLoading(true);
  applyLightboxZoom();
  lockPageScrollForLightbox();
  $('#lightbox').classList.add('open');
  requestAnimationFrame(() => {
    refreshPrimaryScreenOrientationSoon();
  });
  updateLightboxHeaderLayout();
  refreshLightboxUiActivity();
  focusLightboxKeyboardSurface();
  lbRender();
}
function closeLightbox() {
  if (typeof stopEchoFrame === 'function') stopEchoFrame();
  stopSlideshow();
  const lightbox = $('#lightbox');
  clearLightboxUiIdleTimer();
  clearTimeout(state.lightboxVideoProgressTimer);
  clearTimeout(state.videoSurfaceTapTimer);
  state.videoProgressScrubPointerId = null;
  state.videoSurfaceLastTap = null;
  state.videoSurfaceSwipe = null;
  state.lightboxSwipe = null;
  state.lightboxSwipeClickSuppressUntil = 0;
  state.lightboxPinchZoomActive = false;
  resetLightboxSwipeVisual();
  state.lightboxLastPointerType = '';
  $('#lb-video-progress')?.classList.remove('active', 'scrubbing', 'touching');
  $('#lb-touch-playback-toggle')?.classList.remove('active');
  state.lightboxPlaybackToken += 1;
  resetLightboxPrefetchCache();
  setLightboxMediaLoading(false);
  resetLightboxFocusPoint();
  resetLightboxTemporaryZoom();
  saveCurrentVideoResumePosition({ quiet: true });
  flushPendingVideoPlaybackPreference();
  syncTouchPlaybackButton(null, null);
  clearLightboxMediaSession();
  const video = $('#lb-video');
  if (video) {
    video.pause();
    video.removeAttribute('src');
    video.removeAttribute('controls');
    video.removeAttribute('controlslist');
    video.removeAttribute('disablepictureinpicture');
    video.removeAttribute('disableremoteplayback');
    video.removeAttribute('x-webkit-airplay');
    video.load();
  }
  $('#lightbox').classList.remove('open');
  void exitLightboxFullscreen();
  unlockPageScrollForLightbox();
  restoreLightboxStatusBarSurface();
  document.body.classList.remove('lightbox-exiting-to-grid');
  document.documentElement.classList.remove('lightbox-exiting-to-grid');
  clearLightboxUnderzoomState();
  resetLightboxDismissProxy();
  clearLightboxDismissFade();
  clearLightboxDismissGesture();
  state.lightboxSeedPhotoID = null;
  state.lightboxSeedPreviewURL = '';
  state.lightboxOriginThumbRect = null;
  state.lightboxReturnView = '';
  state.lightboxReturnAlbumID = null;
  state.lightboxDebugControls = false;
  state.lightboxPageLoading = false;
  clearLightboxUnderzoomState();
  resetLightboxDismissProxy();
  clearLightboxDismissFade();
}
function exitLightboxToContext() {
  const returnView = state.lightboxReturnView;
  const returnAlbumID = state.lightboxReturnAlbumID;
  const currentPhoto = state.lightboxPhotos[state.lightboxIndex];
  const currentPhotoID = currentPhoto?.id || null;
  // EchoFrame may return a media item outside the timeline's loaded window.
  if (currentPhotoID && returnView === 'timeline'
      && (state.view !== 'timeline' || !findPhotoThumb(currentPhotoID) || !mediaKindMatchesFilter(currentPhoto))) {
    openInTimeline(currentPhotoID, currentPhoto);
    return;
  }
  const shouldUseOriginalScroll = currentPhotoID && Number(currentPhotoID) === Number(state.lightboxSeedPhotoID || 0);
  if (currentPhotoID && returnView === 'album-detail') state.pendingAlbumPhotoID = currentPhotoID;
  if (currentPhotoID && returnView === 'timeline') state.pendingTimelinePhotoID = currentPhotoID;
  if (currentPhotoID && returnView === 'random-album') state.pendingRandomAlbumPhotoID = currentPhotoID;
  const prepositioned = shouldUseOriginalScroll ? false : prepositionLightboxReturnTarget(currentPhotoID, returnView);
  if (prepositioned || shouldUseOriginalScroll) {
    state.lightboxSkipScrollRestore = prepositioned;
    document.body.classList.add('lightbox-exiting-to-grid');
    document.documentElement.classList.add('lightbox-exiting-to-grid');
  }
  closeLightbox();
  if (returnView === 'album-detail' && returnAlbumID && state.view !== 'album-detail') {
    if (state.currentAlbum && state.currentAlbum.id !== returnAlbumID) state.currentAlbum = null;
    state.currentAlbumID = returnAlbumID;
    state.view = 'album-detail';
    setHashView('album-detail', returnAlbumID);
    syncNavActiveView('album-detail');
    renderAlbumDetail();
    return;
  }
  if (currentPhotoID && returnView === 'album-detail') {
    focusPendingAlbumPhoto({ skipScroll: shouldUseOriginalScroll });
    return;
  }
  if (currentPhotoID && returnView === 'timeline') {
    focusPendingTimelinePhoto({ skipScroll: true });
    return;
  }
  if (currentPhotoID && returnView === 'random-album') {
    focusPendingRandomAlbumPhoto({ skipScroll: shouldUseOriginalScroll });
    return;
  }
  if (returnView === 'search') {
    openGlobalSearch(state.searchQuery || $('#global-search-input')?.value || '', { focus: !isTouchLikeDevice() });
    return;
  }
  if (currentPhotoID && returnView === 'favorites') {
    focusPhotoThumbInCurrentView(currentPhotoID);
  }
}
function lightboxCanLoadMoreForward() {
  switch (state.lightboxReturnView) {
    case 'timeline':
      return state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused;
    case 'random-album':
      return state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused;
    case 'favorites':
      return state.favoriteHasMore && !state.favoriteLoading;
    case 'album-detail':
      return state.albumHasMore && !state.albumLoading && !!state.currentAlbum;
    case 'trash':
      return state.trashHasMore && !state.trashLoading;
    case 'search':
      return state.searchHasMore && !state.searchLoading && !!state.searchQuery;
    default:
      return false;
  }
}

async function loadMoreForLightbox() {
  if (state.lightboxPageLoading || !lightboxCanLoadMoreForward()) return false;
  const beforeLength = state.lightboxPhotos.length;
  state.lightboxPageLoading = true;
  try {
    switch (state.lightboxReturnView) {
      case 'timeline':
        await loadMoreTimeline();
        break;
      case 'random-album':
        await loadMoreRandomAlbum();
        break;
      case 'favorites':
        await loadMoreFavorites();
        break;
      case 'album-detail':
        await loadMoreAlbumPhotos();
        break;
      case 'trash':
        await loadMoreTrash();
        break;
      case 'search':
        await runGlobalSearch(state.searchQuery, { reset: false });
        state.lightboxPhotos = state.searchResults;
        break;
      default:
        return false;
    }
  } finally {
    state.lightboxPageLoading = false;
  }
  if (state.lightboxPhotos.length > beforeLength) {
    appendNewRandomQueueIndexes(beforeLength, state.lightboxPhotos.length);
  }
  return state.lightboxPhotos.length > beforeLength;
}

async function ensureLightboxIndexAvailable(index) {
  if (index < state.lightboxPhotos.length) return true;
  if (index < 0) return false;
  const allowForegroundLoad = state.slideshowPlaying || index === state.lightboxPhotos.length;
  if (!allowForegroundLoad) return false;
  if (state.slideshowPreloadPromise) {
    await state.slideshowPreloadPromise;
    if (index < state.lightboxPhotos.length) return true;
  }
  for (let attempt = 0; attempt < 3 && index >= state.lightboxPhotos.length && lightboxCanLoadMoreForward(); attempt += 1) {
    const loaded = await loadMoreForLightbox();
    if (!loaded) break;
  }
  return index < state.lightboxPhotos.length;
}

async function lbNav(dir) {
  const n = state.lightboxIndex + dir;
  if (n < 0) return;
  if (!(await ensureLightboxIndexAvailable(n))) return;
  await lbGoTo(n);
}
function playNextVideoFromLightbox() {
  if (!state.videoAutoplayNext) return false;
  for (let index = state.lightboxIndex + 1; index < state.lightboxPhotos.length; index += 1) {
    if (isVideoMedia(state.lightboxPhotos[index])) {
      void lbGoTo(index);
      return true;
    }
  }
  return false;
}
async function lbGoTo(index, options = {}) {
  if (index < 0) return;
  if (!(await ensureLightboxIndexAvailable(index))) return;
  clearLightboxUnderzoomState();
  clearLightboxDismissGesture();
  if (state.lightboxPinchZoomActive) {
    resetLightboxZoomToDefault();
  }
  state.lightboxPlaybackToken += 1;
  const video = $('#lb-video');
  if (video) {
    saveCurrentVideoResumePosition({ quiet: true });
    flushPendingVideoPlaybackPreference();
    video.pause();
    syncTouchPlaybackButton(null, null);
    clearLightboxMediaSession();
    video.removeAttribute('src');
    video.removeAttribute('controls');
    video.removeAttribute('controlslist');
    video.removeAttribute('disablepictureinpicture');
    video.removeAttribute('disableremoteplayback');
    video.removeAttribute('x-webkit-airplay');
    video.load();
  }
  state.lightboxIndex = index;
  if (state.lightboxReturnView === 'random-album') {
    const photo = state.lightboxPhotos[state.lightboxIndex];
    if (photo && photo.id) state.randomAlbumLastViewedPhotoID = Number(photo.id);
  }
  if (state.slideshowMode === 'random' && !options.fromSlideshow) resetRandomQueue();
  if (options.fromSlideshow) ensureSlideshowHasMoreSoon({ pages: 1 });
  lbRender();
}
function lbRender() {
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  state.videoSurfaceSwipe = null;
  state.lightboxSwipe = null;
  resetLightboxSwipeVisual();
  rememberPhoto(p);
  resetLightboxFocusPoint();
  const img = $('#lb-img');
  const video = $('#lb-video');
  setLightboxMediaLoading(true);
  if (isVideoMedia(p)) {
    let playbackErrorReported = false;
    img.classList.add('hidden');
    img.onload = null;
    img.onerror = null;
    img.removeAttribute('src');
    video.classList.remove('hidden');
    video.removeAttribute('controls');
    video.setAttribute('controlslist', 'nodownload noplaybackrate noremoteplayback');
    video.setAttribute('disablepictureinpicture', '');
    video.setAttribute('disableremoteplayback', 'true');
    video.setAttribute('x-webkit-airplay', 'deny');
    video.playsInline = true;
    video.preload = 'auto';
    video.onloadeddata = () => {
      setLightboxMediaLoading(false);
      applyLightboxZoom();
    };
    video.onloadedmetadata = () => {
      applyLightboxZoom();
      restoreVideoResumePosition(video, p);
      restoreVideoPlaybackPreference(video, p);
      updateVideoBookmarkProgress(video, p);
    };
    video.onerror = () => {
      setLightboxMediaLoading(false);
      const mediaError = video.error;
      const errorCode = mediaError ? Number(mediaError.code) || 0 : 0;
      if (!playbackErrorReported && Number(p.id) > 0 && errorCode !== 1) {
        playbackErrorReported = true;
        const errorNames = {
          1: 'MEDIA_ERR_ABORTED',
          2: 'MEDIA_ERR_NETWORK',
          3: 'MEDIA_ERR_DECODE',
          4: 'MEDIA_ERR_SRC_NOT_SUPPORTED',
        };
        let sourceURLPath = '';
        try {
          sourceURLPath = new URL(video.currentSrc || video.src, location.href).pathname;
        } catch (_) {}
        const report = {
          code: errorCode,
          code_name: errorNames[errorCode] || 'MEDIA_ERR_UNKNOWN',
          message: mediaError && mediaError.message ? String(mediaError.message) : '',
          ready_state: Number(video.readyState) || 0,
          network_state: Number(video.networkState) || 0,
          current_time: Number.isFinite(video.currentTime) ? video.currentTime : 0,
          duration: Number.isFinite(video.duration) ? video.duration : 0,
          video_width: Number(video.videoWidth) || 0,
          video_height: Number(video.videoHeight) || 0,
          can_play_mime_type: video.canPlayType(p.mime_type || ''),
          source_url_path: sourceURLPath,
        };
        void fetchWithPageSession(`/api/media/${encodeURIComponent(p.id)}/playback-error`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(report),
          keepalive: true,
        }).catch(() => {});
      }
      showToast(copyText('app.interface.s_55e621a960'));
    };
    video.onfocus = refocusLightboxAfterVideoControl;
    video.ontimeupdate = () => {
      handleVideoResumeTimeUpdate(video);
      updateVideoBookmarkProgress(video, p);
    };
    video.onplay = () => {
      syncTouchPlaybackButton(video, p);
      updateMediaSessionPlaybackState(video);
      showVideoProgressActivity(3000);
    };
    video.onpause = () => {
      syncTouchPlaybackButton(video, p);
      updateMediaSessionPlaybackState(video);
      showVideoProgressActivity(3000);
    };
    video.onended = () => {
      syncTouchPlaybackButton(video, p);
      updateMediaSessionPlaybackState(video);
      clearPendingVideoSeekCommit();
      if (!playNextVideoFromLightbox()) updateVideoBookmarkProgress(video, p);
    };
    restoreVideoPlaybackPreference(video, p);
    video.src = mediaPlaybackURL(p);
    video.poster = mediaThumbURL(p);
    video.load();
    syncTouchPlaybackButton(video, p);
    syncLightboxMediaSession(p, video);
    if (video.readyState >= 2) {
      setLightboxMediaLoading(false);
      applyLightboxZoom();
    }
    if (shouldAutoplayVideoInLightbox(p)) autoplayLightboxVideo(video);
  } else {
    state.lightboxPlaybackToken += 1;
    const renderToken = state.lightboxPlaybackToken;
    video.pause();
    video.classList.add('hidden');
    video.preload = 'metadata';
    video.onloadeddata = null;
    video.onloadedmetadata = null;
    video.onerror = null;
    video.onfocus = null;
    video.ontimeupdate = null;
    video.onplay = null;
    video.onpause = null;
    video.onended = null;
    video.removeAttribute('src');
    video.removeAttribute('controls');
    video.removeAttribute('controlslist');
    video.removeAttribute('disablepictureinpicture');
    video.removeAttribute('disableremoteplayback');
    video.removeAttribute('x-webkit-airplay');
    video.load();
    syncTouchPlaybackButton(null, p);
    clearLightboxMediaSession();
    updateVideoBookmarkProgress(video, p);
    img.classList.remove('hidden');
    const thumbURL = mediaThumbURL(p);
    const seededPreviewURL = Number(state.lightboxSeedPhotoID || 0) === Number(p.id || 0)
      ? String(state.lightboxSeedPreviewURL || '')
      : '';
    const previewURL = seededPreviewURL || thumbURL;
    const fullURL = mediaFileURL(p);
    let fullLoaded = false;
    const finishPreview = () => {
      if (state.lightboxPlaybackToken !== renderToken) return;
      setLightboxMediaLoading(false);
      applyLightboxZoom();
    };
    img.onload = () => {
      if (state.lightboxPlaybackToken !== renderToken) return;
      if (img.dataset.previewStage === 'thumb' && !fullLoaded) {
        finishPreview();
      } else {
        fullLoaded = true;
        img.dataset.previewStage = 'full';
        finishPreview();
      }
    };
    img.onerror = () => {
      if (state.lightboxPlaybackToken !== renderToken) return;
      if (img.dataset.previewStage === 'thumb') {
        img.dataset.previewStage = 'full';
        img.src = fullURL;
        return;
      }
      setLightboxMediaLoading(false);
    };
    img.dataset.previewStage = 'thumb';
    img.src = previewURL;
    if (img.complete) finishPreview();
    if (thumbURL !== fullURL) {
      const fullImage = new Image();
      fullImage.decoding = 'async';
      fullImage.onload = () => {
        if (state.lightboxPlaybackToken !== renderToken) return;
        fullLoaded = true;
        img.dataset.previewStage = 'full';
        img.src = fullURL;
      };
      fullImage.onerror = () => {
        if (state.lightboxPlaybackToken !== renderToken || !img.complete) return;
        setLightboxMediaLoading(false);
      };
      fullImage.src = fullURL;
    }
  }
  $('#lb-title').textContent = p.original_name;
  updateLightboxFavoriteButton(p);
  syncLightboxShareButton();
  updateVideoBookmarkButton(p);
  updateVideoBookmarkProgress(video, p);
  $('#lb-prev').classList.toggle('hidden', state.lightboxIndex === 0);
  $('#lb-next').classList.toggle('hidden', state.lightboxIndex === state.lightboxPhotos.length - 1 && !lightboxCanLoadMoreForward());
  applyLightboxZoom();
  updateLightboxHeaderLayout();
  updateSlideshowControls();
  if (state.slideshowPlaying) scheduleSlideshowStep();
  if (!state.slideshowPlaying && (state.experimentalPrefetchNeighbors || isTouchLikeDevice())) prefetchAdjacentMedia();
  const items = buildLightboxInfoItems(p);
  $('#lb-info').innerHTML = items.map(([icon, k, v]) => {
    const value = addPanguSpacing(v);
    return `<div class="lb-info-item" title="${escapeHTML(`${k}: ${value}`)}" aria-label="${escapeHTML(`${k}: ${value}`)}"><span class="lb-info-key">${icon}</span><span class="lb-info-value">${escapeHTML(value)}</span></div>`;
  }).join('');
}

async function toggleCurrentLightboxFavorite() {
  if (!canWriteMedia()) return forbidVisitorAction();
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  await toggleFavorite(p);
}
function clearLightboxFavoriteKeyHoldState() {
  clearTimeout(state.lightboxFavoriteKeyHoldTimer);
  state.lightboxFavoriteKeyHoldTimer = null;
  state.lightboxFavoriteKeyHoldActive = false;
  state.lightboxFavoriteKeyHoldTriggered = false;
}
function startLightboxFavoriteKeyHold() {
  if (state.lightboxFavoriteKeyHoldActive) return;
  state.lightboxFavoriteKeyHoldActive = true;
  state.lightboxFavoriteKeyHoldTriggered = false;
  clearTimeout(state.lightboxFavoriteKeyHoldTimer);
  state.lightboxFavoriteKeyHoldTimer = setTimeout(() => {
    state.lightboxFavoriteKeyHoldTimer = null;
    state.lightboxFavoriteKeyHoldTriggered = true;
    const photo = state.lightboxPhotos[state.lightboxIndex];
    if (!photo) return;
    void toggleFavorite(photo, {
      mode: 'hold',
      sourceButton: $('#lb-favorite'),
    });
  }, 620);
}
function finishLightboxFavoriteKeyHold() {
  if (!state.lightboxFavoriteKeyHoldActive) return false;
  const triggered = state.lightboxFavoriteKeyHoldTriggered;
  clearTimeout(state.lightboxFavoriteKeyHoldTimer);
  state.lightboxFavoriteKeyHoldTimer = null;
  state.lightboxFavoriteKeyHoldActive = false;
  state.lightboxFavoriteKeyHoldTriggered = false;
  if (!triggered) {
    void toggleCurrentLightboxFavorite();
  }
  return true;
}
function prefetchAdjacentMedia() {
  if (!isWarmEnabled()) return;
  if (!lightboxIsInteractive() || shouldPauseLightboxBackgroundWork()) return;
  const neighbors = [state.lightboxIndex - 2, state.lightboxIndex - 1, state.lightboxIndex + 1, state.lightboxIndex + 2]
    .filter(i => i >= 0 && i < state.lightboxPhotos.length)
    .map(i => state.lightboxPhotos[i]);
  neighbors.forEach(photo => {
    if (!photo) return;
    const kind = isVideoMedia(photo) ? 'video-poster' : 'image';
    const key = `${kind}:${photo.uuid || photo.id || photo.original_name || ''}`;
    if (!rememberPrefetchedMedia(key)) return;
    if (isVideoMedia(photo)) {
      const link = document.createElement('link');
      link.rel = 'prefetch';
      link.as = 'image';
      link.href = mediaThumbURL(photo);
      document.head.appendChild(link);
      setTimeout(() => link.remove(), 1000);
      return;
    }
    const img = new Image();
    img.decoding = 'async';
    img.src = mediaFileURL(photo);
  });
}
function warmLightboxIndex(index) {
  if (!isWarmEnabled()) return;
  if (!lightboxIsInteractive() || shouldPauseLightboxBackgroundWork()) return;
  if (!Number.isFinite(index) || index < 0) return;
  if (index >= state.lightboxPhotos.length) {
    if (!lightboxCanLoadMoreForward()) return;
    const pageKey = `lightbox-page:${state.lightboxReturnView || state.view}:${index}`;
    if (!rememberPrefetchedMedia(pageKey)) return;
    void ensureLightboxIndexAvailable(index).then(() => prefetchAdjacentMedia()).catch(() => {});
    return;
  }
  prefetchPhotosForLightboxIntent(state.lightboxPhotos, index);
}
async function lbShare() {
  if (!canManageShareLinks()) return forbidVisitorAction();
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  const isShared = !!state.shareMap[`photo:${p.id}`];
  if (isShared) {
    await copyExistingShareLink('photo', p.id);
    syncLightboxShareButton();
    return;
  }
  openShareModal('photo', p.id);
  syncLightboxShareButton();
}

function triggerDownload(url) {
	const a = document.createElement('a');
	a.href = withPageSessionURL(url);
	a.style.display = 'none';
	document.body.appendChild(a);
	a.click();
	a.remove();
}

function shouldUseSystemPhotoSave() {
  return isTouchLikeDevice() && typeof navigator.share === 'function';
}

function shouldShowSystemPhotoSaveLabel() {
  return isTouchLikeDevice() || isMobileLayout();
}

function singleMediaSaveLabel() {
  return shouldShowSystemPhotoSaveLabel() ? copyText('app.interface.s_f32a9c43f6') : copyText('app.interface.s_2b9d013177');
}

function mediaArchiveSaveLabel() {
  return shouldShowSystemPhotoSaveLabel() ? copyText('app.interface.s_f32a9c43f6') : copyText('app.interface.s_2b9d013177');
}

function mediaArchiveSaveVerb() {
  return shouldShowSystemPhotoSaveLabel() ? copyText('app.interface.s_13527a30f2') : copyText('app.interface.s_2b9d013177');
}

function parseDownloadFilename(disposition, fallbackName = 'EchoGallery') {
  const text = String(disposition || '');
  const encoded = text.match(/filename\*=UTF-8''([^;]+)/i);
  if (encoded) {
    try {
      return decodeURIComponent(encoded[1].trim().replace(/^"|"$/g, ''));
    } catch (_) {}
  }
  const plain = text.match(/filename="?([^";]+)"?/i);
  return plain ? plain[1].trim() : fallbackName;
}

async function fetchMediaDownloadPayload(photo) {
  if (!photo || !photo.id) return null;
  const response = await fetchWithPageSession(`/api/media/${photo.id}/download`);
  if (!response.ok) throw await buildAPIError(response);
  const blob = await response.blob();
  const filename = parseDownloadFilename(response.headers.get('Content-Disposition'), photo.original_name || 'EchoGallery');
  return {
    blob,
    filename,
    mimeType: blob.type || photo.mime_type || 'application/octet-stream',
  };
}

function triggerBlobDownload(blob, filename) {
  if (!blob) return;
  const objectURL = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = objectURL;
  a.download = filename || 'EchoGallery';
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
}

async function saveMediaToSystemPhotos(photo, downloadPayload) {
  if (!photo || !photo.id || !shouldUseSystemPhotoSave()) return false;
  const payload = downloadPayload || (await fetchMediaDownloadPayload(photo));
  if (!payload || !payload.blob) return false;
  const file = new File([payload.blob], payload.filename, { type: payload.mimeType });
  const shareData = { files: [file] };
  if (typeof navigator.canShare === 'function' && !navigator.canShare(shareData)) return false;
  await navigator.share(shareData);
  return true;
}

async function saveOrDownloadMedia(photo) {
  if (!photo || !photo.id) return;
  let downloadPayload = null;
  if (shouldShowSystemPhotoSaveLabel()) {
    try {
      downloadPayload = await fetchMediaDownloadPayload(photo);
      const shared = await saveMediaToSystemPhotos(photo, downloadPayload);
      if (shared) {
        showToast(copyText('app.interface.s_5b42274b26'));
        return;
      }
      if (downloadPayload && downloadPayload.blob) {
        triggerBlobDownload(downloadPayload.blob, downloadPayload.filename);
        showToast(copyText('app.interface.s_61da85d60c'));
        return;
      }
      showToast(copyText('app.interface.s_1d40142312'));
    } catch (e) {
      if (e && e.name === 'AbortError') return;
      if (downloadPayload && downloadPayload.blob) {
        triggerBlobDownload(downloadPayload.blob, downloadPayload.filename);
        showToast(copyText('app.interface.s_2a50ecebb5'), 2600);
        return;
      }
      showToast(copyText('app.interface.s_4cdc47f024'), 2600);
    }
  }
  triggerDownload(`/api/media/${photo.id}/download`);
}

async function confirmDownloadAction(message, action) {
  if (!(await appConfirm(message || copyText('app.interface.d_7705b4a756', { v0: (mediaArchiveSaveVerb()) })))) return false;
  if (typeof action === 'function') action();
  return true;
}

function renderButtonBusySpinner() {
	return '<span class="button-busy-inline" aria-hidden="true"><span class="button-busy-spinner"></span></span>';
}

async function withButtonBusy(button, busyText, fn, options = {}) {
	if (!button) return fn();
	const prevHTML = button.innerHTML;
	const prevDisabled = button.disabled;
	const prevAriaLabel = button.getAttribute('aria-label');
	button.disabled = true;
	button.innerHTML = busyText;
	if (options.ariaLabel) button.setAttribute('aria-label', options.ariaLabel);
	try {
		return await fn();
	} finally {
		if (options.keepBusyState) return;
		button.disabled = prevDisabled;
		button.innerHTML = prevHTML;
		if (prevAriaLabel == null) button.removeAttribute('aria-label');
		else button.setAttribute('aria-label', prevAriaLabel);
	}
}

async function triggerPostDownload(url, payload, fallbackName, options = {}) {
	const res = await fetchWithPageSession(url, {
		method: 'POST',
		headers: buildPageSessionHeaders({ 'Content-Type': 'application/json' }),
		body: JSON.stringify(payload),
		signal: options.signal,
	});
	if (!res.ok) {
		throw await buildAPIError(res);
	}
	const blob = await res.blob();
	const objectURL = URL.createObjectURL(blob);
	const a = document.createElement('a');
	const disposition = res.headers.get('Content-Disposition') || '';
	const match = disposition.match(/filename\*=UTF-8''([^;]+)/);
	a.href = objectURL;
	a.download = match ? decodeURIComponent(match[1]) : fallbackName;
	a.style.display = 'none';
	document.body.appendChild(a);
	a.click();
	a.remove();
	setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
}

async function downloadCurrentPhoto() {
	const p = state.lightboxPhotos[state.lightboxIndex];
	if (!p) return;
  const label = singleMediaSaveLabel();
  if (!(await appConfirm(copyText('app.interface.d_66274d1532', { v0: (label) })))) return;
  if (isRootDebugMedia(p)) {
    triggerDownload(mediaPlaybackURL(p));
    return;
  }
	const btn = $('#lb-download');
	if (!btn) {
		await saveOrDownloadMedia(p);
		return;
	}
	await withButtonBusy(btn, shouldUseSystemPhotoSave() ? copyText('app.interface.commonSaving') : copyText('app.interface.s_65188d08a2'), async () => {
		await saveOrDownloadMedia(p);
		await new Promise(resolve => setTimeout(resolve, 600));
	});
}

async function downloadSelected() {
	if (!state.selected.size) return;
  if (!(await appConfirm(copyText('app.interface.d_031fca5881', { v0: (mediaArchiveSaveVerb()), v1: (state.selected.size) })))) return;
	const btn = $('#download-sel-btn');
	try {
		await withButtonBusy(btn, renderButtonBusySpinner(), async () => {
			await triggerPostDownload('/api/media/download', {
				media_ids: [...state.selected],
			}, `echogallery-selection-${Date.now()}.zip`);
		}, { ariaLabel: copyText('app.interface.s_711d04e000') });
	} catch (e) {
		showToast(copyText('app.interface.runtime.archiveError', { action: mediaArchiveSaveVerb(), error: (e.error || e) }), 3200);
	}
}

async function collectAllFavoriteIDs() {
  const ids = [];
  let cursor = '';
  for (;;) {
    const params = new URLSearchParams({ limit: '240' });
    if (cursor) params.set('cursor', cursor);
    const page = await api.get(`/api/media/favorites?${params.toString()}`);
    (page.photos || []).forEach(photo => {
      if (photo && photo.id) ids.push(photo.id);
    });
    if (!page.has_more || !page.next_cursor) break;
    cursor = page.next_cursor;
  }
  return ids;
}

async function downloadAllFavorites() {
  const btn = $('#download-all-favorites-btn');
  try {
    await withButtonBusy(btn, copyText('app.interface.s_5df7401ada'), async () => {
      const ids = await collectAllFavoriteIDs();
      if (!ids.length) {
        showToast(copyText('app.interface.d_87ff89620c', { v0: (mediaArchiveSaveVerb()) }));
        return;
      }
      await triggerPostDownload('/api/media/download', {
        media_ids: ids,
      }, `echogallery-favorites-${Date.now()}.zip`);
    });
  } catch (e) {
    showToast(copyText('app.interface.runtime.archiveError', { action: mediaArchiveSaveVerb(), error: (e.error || e) }), 3200);
  }
}

async function buildBrowserPlaybackCacheForPhoto(photo) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!photo || !photo.id) return;
  if (!(await appConfirm(copyText('app.interface.runtime.conversionConfirm', { name: photo.original_name || copyText('app.interface.s_fed98ce4a2') })))) return;
  try {
    showToast(copyText('app.interface.s_bc4ce77037'), 2200);
    const resp = await api.post(`/api/media/${photo.id}/playback-cache`, {});
    state.playbackCacheItemsLoaded = true;
    if (!playbackCacheExistsForPhoto(photo)) {
      state.playbackCacheItems = [...(state.playbackCacheItems || []), (resp && resp.data) || { uuid: photo.uuid, original_name: photo.original_name }];
    }
    showToast((resp && resp.message) || copyText('app.interface.s_6ca0fffa9e'));
    if ($('#lightbox').classList.contains('open')) {
      const current = state.lightboxPhotos[state.lightboxIndex];
      if (current && Number(current.id) === Number(photo.id)) {
        const video = $('#lb-video');
        if (video && !video.classList.contains('hidden')) {
          const wasPaused = video.paused;
          const currentTime = Number(video.currentTime) || 0;
          video.src = appendURLParam(mediaPlaybackURL(photo), 'converted', String(Date.now()));
          video.load();
          video.addEventListener('loadedmetadata', function onLoadedMetadata() {
            video.removeEventListener('loadedmetadata', onLoadedMetadata);
            if (currentTime > 0 && Number.isFinite(video.duration)) {
              video.currentTime = Math.max(0, Math.min(video.duration - 0.2, currentTime));
            }
            if (!wasPaused) video.play().catch(() => {});
          }, { once: true });
        }
      }
    }
  } catch (e) {
    showToast(copyText('app.interface.s_b310d97eca') + (((e && e.error) || e)), 3200);
  }
}

async function deleteBrowserPlaybackCacheForPhoto(photo) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!photo || !photo.id) return;
  if (!(await appConfirm(copyText('app.interface.runtime.deleteConversionConfirm', { name: photo.original_name || copyText('app.interface.s_fed98ce4a2') }), { danger: true }))) return;
  try {
    const current = state.lightboxPhotos[state.lightboxIndex];
    const isCurrent = current && Number(current.id) === Number(photo.id);
    if (isCurrent) {
      const video = $('#lb-video');
      if (video && !video.classList.contains('hidden')) {
        video.pause();
        video.removeAttribute('src');
        video.load();
      }
    }
    await api.del(`/api/media/${photo.id}/playback-cache`);
    state.playbackCacheItems = (state.playbackCacheItems || []).filter(item => String(item.uuid || '') !== String(photo.uuid || ''));
    showToast(copyText('app.interface.s_5db8fad7ad'));
    if (isCurrent) lbRender();
  } catch (e) {
    showToast(copyText('app.interface.d_4d26cdf9fe', { v0: ((e && e.error) || e) }), 3200);
  }
}

let renameMediaTarget = null;

function splitMediaFileName(name) {
  const value = String(name || '');
  const dot = value.lastIndexOf('.');
  if (dot <= 0) return { base: value, extension: '' };
  return { base: value.slice(0, dot), extension: value.slice(dot) };
}

function updateMediaNameInCollections(photo) {
  if (!photo) return;
  const collections = [state.photos, state.favoritePhotos, state.randomAlbumPhotos, state.albumPhotos, state.trashPhotos, state.lightboxPhotos, state.memoryPhotos, state.searchResults];
  collections.forEach(list => {
    if (!Array.isArray(list)) return;
    list.forEach(item => {
      if (item && Number(item.id) === Number(photo.id)) Object.assign(item, photo);
    });
  });
}

function openRenameMediaModal(photo) {
  if (!canWriteMedia() || !photo) return forbidVisitorAction();
  renameMediaTarget = photo;
  const parts = splitMediaFileName(photo.original_name);
  const modal = $('#rename-media-modal');
  const input = $('#rename-media-name');
  const extension = $('#rename-media-extension');
  if (!modal || !input || !extension) return;
  input.value = parts.base;
  extension.textContent = parts.extension;
  modal.classList.add('open');
  requestAnimationFrame(() => { input.focus(); input.select(); });
}

function closeRenameMediaModal() {
  $('#rename-media-modal')?.classList.remove('open');
  renameMediaTarget = null;
}

async function submitRenameMedia() {
  const photo = renameMediaTarget;
  const input = $('#rename-media-name');
  if (!photo || !input) return;
  const name = String(input.value || '').trim();
  if (!name) return showToast(copyText('app.interface.s_549f3e605d'));
  const save = $('#rename-media-save');
  if (save) save.disabled = true;
  try {
    const resp = await api.patch(`/api/media/${photo.id}/rename`, { name });
    const updated = (resp && resp.data) || { ...photo, original_name: `${name}${splitMediaFileName(photo.original_name).extension}` };
    updateMediaNameInCollections(updated);
    closeRenameMediaModal();
    renderView();
    if ($('#lightbox')?.classList.contains('open')) lbRender();
    showToast((resp && resp.message) || copyText('app.interface.s_7f001bd423'));
  } catch (e) {
    showToast(copyText('app.interface.d_7eb3bc044c', { v0: ((e && e.error) || e) }), 3200);
  } finally {
    if (save) save.disabled = false;
  }
}

// ── 上传模态框 ────────────────────────────────────────
function renderRenameMediaModal() {
  return `<div class="modal-overlay library-editor-modal" id="rename-media-modal" role="presentation">
  <div class="modal library-editor-card rename-media-card" role="dialog" aria-modal="true" aria-labelledby="rename-media-title">
    <div class="modal-title" id="rename-media-title">${escapeHTML(copyText('app.interface.runtime.renameMedia'))}</div>
    <div class="rename-media-field">
      <label class="settings-control-label" for="rename-media-name">${escapeHTML(copyText('app.interface.runtime.fileName'))}</label>
      <div class="rename-media-input-row"><input class="input" id="rename-media-name" type="text" maxlength="240" autocomplete="off"><span id="rename-media-extension"></span></div>
      <small>${escapeHTML(copyText('app.interface.runtime.renameNote'))}</small>
    </div>
    <div class="modal-footer library-editor-actions rename-media-actions">
      ${libraryEditorActionButton({ id: 'rename-media-cancel', kind: 'cancel', icon: icons.libraryEditorCancel, label: copyText('app.interface.commonCancel') })}
      ${libraryEditorActionButton({ id: 'rename-media-save', kind: 'save', icon: icons.libraryEditorSave, label: copyText('app.interface.commonSave') })}
    </div>
  </div>
</div>`;
}

function renderUploadModal() {
  return `<div class="modal-overlay" id="upload-modal">
  <div class="modal" style="width:520px">
    <div class="modal-title">${icons.upload} ${escapeHTML(copyText('app.upload.title'))}</div>
    <div class="upload-zone" id="drop-zone">
      ${icons.upload}
      <div style="margin-top:8px">${escapeHTML(copyText('app.upload.hint'))}</div>
      <div style="font-size:.8rem;margin-top:4px">${escapeHTML(copyText('app.upload.support'))}</div>
      <input type="file" id="file-input" accept="image/*,.bmp,.tif,.tiff,.mp4,.mov,.m4v,.webm,.mkv,.avi,.wmv,.wma,.ts,.mts,.m2ts,.mpg,.mpeg,.3gp,.3g2,.ogv,video/*,audio/x-ms-wma" multiple aria-hidden="true">
    </div>
    <div class="upload-queue" id="upload-queue"></div>
    <div class="modal-footer">
      <button class="btn" id="retry-failed-btn" style="display:none">${escapeHTML(copyText('app.upload.retryFailed'))}</button>
      <button class="btn" id="upload-close-btn">${escapeHTML(copyText('app.upload.close'))}</button>
    </div>
  </div>
</div>`;
}
let _uploadZoneBound = false;
function openUploadModal() {
  if (!canWriteMedia()) return forbidVisitorAction();
  $('#upload-modal').classList.add('open');
  $('#upload-queue').innerHTML = '';
  $('#retry-failed-btn').style.display = 'none';
  state.uploadJobs = [];
  state.uploadRunning = false;
  const input = $('#file-input');
  if (input) input.value = '';
  if (!_uploadZoneBound) { _uploadZoneBound = true; bindUploadZone(); }
}
function closeUploadModal() { $('#upload-modal').classList.remove('open'); }
function bindUploadZone() {
  const zone = $('#drop-zone');
  const input = $('#file-input');
  $('#upload-close-btn').addEventListener('click', () => { closeUploadModal(); switchView('timeline'); });
  $('#retry-failed-btn').addEventListener('click', retryFailedUploads);
  zone.addEventListener('click', e => {
    if (e.target === input) return;
    // 某些桌面浏览器虽然实现了 showPicker，但对当前隐藏 input 调用会失败。
    // 这里优先尝试 showPicker，失败时立即回退到 input.click()。
    try {
      if (typeof input.showPicker === 'function') {
        input.showPicker();
        return;
      }
    } catch (_) {
      // ignore and fallback
    }
    input.click();
  });
  zone.addEventListener('dragover', e => { e.preventDefault(); zone.classList.add('drag-over'); });
  zone.addEventListener('dragleave', () => zone.classList.remove('drag-over'));
  zone.addEventListener('drop', e => { e.preventDefault(); zone.classList.remove('drag-over'); handleFiles(e.dataTransfer.files); });
  input.addEventListener('change', () => { if (input.files.length) handleFiles(input.files); });
}
async function handleFiles(fileList) {
  const files = [...fileList].filter(f => {
    const name = (f.name || '').toLowerCase();
    if (f.type.startsWith('image/') || supportedImageExtensions.some(ext => name.endsWith(ext))) return true;
    return f.type.startsWith('video/') || supportedVideoExtensions.some(ext => name.endsWith(ext));
  });
  if (!files.length) return;
  const queue = $('#upload-queue');
  for (const file of files) {
    const id = uid();
    const job = { id, file, status: 'queued' };
    state.uploadJobs.push(job);
    const row = el('div', 'upload-item');
    row.id = `upload-row-${id}`;
    row.innerHTML = `<span class="up-name">${file.name}</span><div style="flex:1"><div class="progress-bar"><div class="progress-fill" style="width:0%" id="prog-${id}"></div></div></div><span class="up-status" id="stat-${id}">${escapeHTML(copyText('app.upload.waiting'))}</span><button class="btn btn-sm" id="retry-${id}" style="display:none">${escapeHTML(copyText('app.upload.retryFailed'))}</button>`;
    queue.appendChild(row);
    const retryBtn = row.querySelector(`#retry-${id}`);
    retryBtn.addEventListener('click', () => retrySingleUpload(id));
  }

  runUploadQueue();
}

async function runUploadQueue() {
  if (state.uploadRunning) return;
  state.uploadRunning = true;
  try {
    for (;;) {
      const job = state.uploadJobs.find(j => j.status === 'queued');
      if (!job) break;
      await uploadFile(job.file, job.id, job);
    }
  } finally {
    state.uploadRunning = false;
    updateRetryFailedButton();
  }
}

async function uploadFile(file, id, job) {
  const prog = $(`#prog-${id}`);
  const stat = $(`#stat-${id}`);
  const retryBtn = $(`#retry-${id}`);
  if (job) job.status = 'uploading';
  if (retryBtn) retryBtn.style.display = 'none';
  if (stat) { stat.textContent = copyText('app.upload.uploading'); stat.className = 'up-status'; }
  const fd = new FormData();
  const lowerName = (file.name || '').toLowerCase();
  const isVideo = file.type.startsWith('video/') || supportedVideoExtensions.some(ext => lowerName.endsWith(ext));
  fd.append(isVideo ? 'media' : 'photo', file);
  // 传递浏览器 File 对象的本地最后修改时间，供后端在无 EXIF 时作为回退时间。
  if (file.lastModified) {
    fd.append('client_last_modified_ms', String(file.lastModified));
  }
	try {
		await new Promise((resolve, reject) => {
			const xhr = new XMLHttpRequest();
			xhr.open('POST', '/api/media/upload');
      xhr.upload.onprogress = e => { if (prog && e.lengthComputable) prog.style.width = (e.loaded / e.total * 100) + '%'; };
      xhr.onload = () => { if (xhr.status === 201) resolve(); else { try { reject(JSON.parse(xhr.responseText)); } catch { reject({ error: xhr.statusText }); } } };
      xhr.onerror = () => reject({ error: copyText('app.upload.networkError') });
      xhr.send(fd);
    });
    if (prog) prog.style.width = '100%';
    if (job) job.status = 'done';
    if (stat) { stat.textContent = copyText('app.upload.done'); stat.className = 'up-status done'; }
  } catch(e) {
    if (job) job.status = 'failed';
    if (stat) { stat.textContent = localizedAPIText(e, 'error', copyText('app.upload.failed')); stat.className = 'up-status error'; }
    if (retryBtn) retryBtn.style.display = '';
  }
  updateRetryFailedButton();
}

function updateRetryFailedButton() {
  const btn = $('#retry-failed-btn');
  if (!btn) return;
  const failedCount = state.uploadJobs.filter(j => j.status === 'failed').length;
  btn.style.display = failedCount > 0 ? '' : 'none';
  btn.textContent = failedCount > 0 ? `${copyText('app.upload.retryFailed')}（${failedCount}）` : copyText('app.upload.retryFailed');
}

function retrySingleUpload(id) {
  const job = state.uploadJobs.find(j => j.id === id);
  if (!job) return;
  job.status = 'queued';
  const prog = $(`#prog-${id}`);
  const stat = $(`#stat-${id}`);
  const retryBtn = $(`#retry-${id}`);
  if (prog) prog.style.width = '0%';
  if (stat) { stat.textContent = copyText('app.upload.retryWaiting'); stat.className = 'up-status'; }
  if (retryBtn) retryBtn.style.display = 'none';
  runUploadQueue();
}

function retryFailedUploads() {
  let found = false;
  state.uploadJobs.forEach(job => {
    if (job.status === 'failed') {
      found = true;
      job.status = 'queued';
      const prog = $(`#prog-${job.id}`);
      const stat = $(`#stat-${job.id}`);
      const retryBtn = $(`#retry-${job.id}`);
      if (prog) prog.style.width = '0%';
      if (stat) { stat.textContent = copyText('app.upload.retryWaiting'); stat.className = 'up-status'; }
      if (retryBtn) retryBtn.style.display = 'none';
    }
  });
  if (found) runUploadQueue();
}

// ── 新建相册模态框 ────────────────────────────────────
function renderCreateAlbumModal() {
  return `<div class="modal-overlay library-editor-modal" id="create-album-modal" role="presentation">
  <div class="modal library-editor-card create-album-card" role="dialog" aria-modal="true" aria-labelledby="create-album-title">
    <div class="modal-title" id="create-album-title">${escapeHTML(copyText('app.createAlbum.title'))}</div>
    <div class="settings-control">
      <label for="album-name-input"><span>${escapeHTML(copyText('app.createAlbum.nameLabel'))}</span></label>
      <input class="input" id="album-name-input" placeholder="${escapeHTML(copyText('app.createAlbum.namePlaceholder'))}" maxlength="50">
    </div>
    <div class="settings-control">
      <label for="album-desc-input"><span>${escapeHTML(copyText('app.createAlbum.descLabel'))}</span></label>
      <input class="input" id="album-desc-input" placeholder="${escapeHTML(copyText('app.createAlbum.descPlaceholder'))}">
    </div>
    <div class="modal-footer library-editor-actions create-album-actions">
      ${libraryEditorActionButton({ id: 'cancel-album-btn', kind: 'cancel', icon: icons.libraryEditorCancel, label: copyText('app.createAlbum.cancel') })}
      ${libraryEditorActionButton({ id: 'confirm-album-btn', kind: 'save', icon: icons.libraryEditorSave, label: copyText('app.createAlbum.confirm') })}
    </div>
  </div>
</div>`;
}

function renderSmartAlbumModal() {
  return `<div class="modal-overlay library-editor-modal smart-album-overlay" id="smart-album-modal" role="presentation">
  <div class="modal library-editor-card smart-album-card" role="dialog" aria-modal="true" aria-labelledby="smart-album-title">
    <header class="smart-album-heading">
      <div class="smart-album-icon"><img src="/static/svg/album-intelligence-create.svg" alt=""></div>
      <h2 class="modal-title" id="smart-album-title">${copyText('app.interface.s_f16a5da6b5')}</h2>
      <p class="modal-copy">${escapeHTML(copyText('app.interface.runtime.smartAlbumCopy'))}</p>
    </header>
    <div class="smart-album-main">
      <section class="smart-album-name-section">
        <div class="smart-album-section-label">${escapeHTML(copyText('app.interface.runtime.smartAlbumNameSection'))}</div>
        <label class="smart-album-control"><input class="echo-input" id="smart-album-name" type="text" maxlength="50" placeholder="${escapeHTML(copyText('app.interface.runtime.smartAlbumNamePlaceholder'))}" aria-label="${escapeHTML(copyText('app.interface.runtime.smartAlbumNameAria'))}"></label>
        <div class="smart-album-description-heading smart-album-section-label">${escapeHTML(copyText('app.interface.runtime.smartAlbumDescriptionSection'))}</div>
        <label class="smart-album-control"><textarea class="echo-input smart-album-description" id="smart-album-description" rows="2" maxlength="2000" aria-label="${escapeHTML(copyText('app.interface.runtime.smartAlbumDescriptionAria'))}" placeholder="${escapeHTML(copyText('app.interface.runtime.smartAlbumDescriptionPlaceholder'))}"></textarea></label>
      </section>
      <section class="smart-album-match-mode" aria-label="${escapeHTML(copyText('app.interface.runtime.smartAlbumMatchModeAria'))}">
        <div class="smart-album-section-label">${escapeHTML(copyText('app.interface.runtime.smartAlbumRulesSection'))}</div>
        <div class="echo-segmented smart-album-segmented" role="group" aria-label="${escapeHTML(copyText('app.interface.runtime.smartAlbumMatchModeAria'))}">
          <button class="echo-segmented__option" id="smart-album-match-all" type="button" aria-pressed="true">${copyText('app.interface.s_778fc8f994')}</button>
          <button class="echo-segmented__option" id="smart-album-match-any" type="button" aria-pressed="false">${escapeHTML(copyText('app.interface.s_623a8d3806'))}</button>
        </div>
      </section>
      <section class="smart-album-rules-section" aria-labelledby="smart-album-rules-title">
        <div class="smart-album-section-label" id="smart-album-rules-title">${escapeHTML(copyText('app.interface.runtime.smartAlbumRulesSection'))}</div>
        <div class="smart-album-rule-list" id="smart-album-rule-list"></div>
        <div class="smart-album-rule-toolbar">
          <span class="smart-album-rule-count" id="smart-album-rule-count" aria-live="polite"></span>
          <button class="btn btn-icon btn-iconish smart-album-rule-action smart-album-rule-add smart-album-add-rule" id="smart-album-add-rule" type="button" aria-label="${escapeHTML(copyText('app.interface.runtime.addRuleAria'))}" title="${escapeHTML(copyText('app.interface.s_af1c0446d5'))}">+</button>
        </div>
      </section>
      <section class="smart-album-preview-section" aria-labelledby="smart-album-preview-title">
        <div class="smart-album-preview-heading">
          <div class="smart-album-section-label" id="smart-album-preview-title">${escapeHTML(copyText('app.interface.runtime.smartAlbumPreviewSection'))}</div>
          <span class="smart-album-preview-count" id="smart-album-preview-count" aria-live="polite"></span>
        </div>
        <div class="smart-album-preview-viewport">
          <div class="smart-album-preview-list" id="smart-album-preview-list"></div>
        </div>
      </section>
    </div>
    <div class="modal-footer library-editor-actions smart-album-actions">
      ${libraryEditorActionButton({ id: 'smart-album-cancel', kind: 'cancel', icon: icons.libraryEditorCancel, label: copyText('app.interface.commonCancel') })}
      ${libraryEditorActionButton({ id: 'smart-album-create', kind: 'save', icon: icons.libraryEditorSave, label: copyText('app.interface.commonCreate') })}
    </div>
  </div>
</div>`;
}

const smartAlbumMaxRules = 10;

const smartAlbumDefaultRules = () => [
  { field: 'media_type', operator: 'is', value: 'video' },
];

function smartAlbumFormatDuration(value) {
  const duration = parseSmartAlbumDuration(value);
  if (duration == null) return String(value || '').trim();
  const totalSeconds = Math.floor(duration / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours && !minutes && !seconds) return copyText('app.interface.d_7ca6a3cfb0', { v0: (hours) });
  if (hours) return copyText('app.interface.d_0535c2f100', { v0: hours, v1: minutes ? ` ${copyText('app.interface.runtime.durationMinutes', { count: minutes })}` : '', v2: seconds ? ` ${copyText('app.interface.runtime.durationSeconds', { count: seconds })}` : '' });
  if (minutes && !seconds) return copyText('app.interface.d_41d1498dab', { v0: (minutes) });
  if (minutes) return copyText('app.interface.d_86aa4cdd56', { v0: (minutes), v1: (seconds) });
  return copyText('app.interface.commonSeconds', { v0: (seconds) });
}

function smartAlbumDescribeRules(rules = []) {
  const descriptions = rules.map(rule => {
    if (rule.field === 'media_type') {
      const type = rule.value === 'video' ? copyText('app.interface.mediaVideo') : copyText('app.interface.mediaImage');
      return rule.operator === 'is_not' ? copyText('app.interface.d_f30286cc0f', { v0: (type) }) : copyText('app.interface.d_af4178d9f7', { v0: (type) });
    }
    if (rule.field === 'duration') {
      if (rule.operator === 'between') return copyText('app.interface.d_48a3bb55e4', { v0: (smartAlbumFormatDuration(rule.min)), v1: (smartAlbumFormatDuration(rule.max)) });
      if (rule.operator === 'at_most') return copyText('app.interface.d_07d233bf2c', { v0: (smartAlbumFormatDuration(rule.max)) });
      return copyText('app.interface.d_aa6fd93c5e', { v0: (smartAlbumFormatDuration(rule.min)) });
    }
    const operators = {
      contains: copyText('app.interface.s_f896fb3776'),
      not_contains: copyText('app.interface.s_82c5d50f38'),
      starts_with: copyText('app.interface.s_ab6d48d60e'),
      ends_with: copyText('app.interface.s_da2e2a52a7'),
    };
    const operation = operators[rule.operator] || copyText('app.interface.s_f896fb3776');
    const value = String(rule.value || '').trim();
    return copyText('app.interface.d_d66a8b431a', { v0: (operation), v1: (value ? ` ${value}` : '') });
  }).filter(Boolean);
  return descriptions.join(' · ');
}

function syncSmartAlbumDescription(modal) {
  const input = $('#smart-album-description');
  if (!input) return;
  const previousAuto = modal._smartAlbumAutoDescription || '';
  const keepAutomatic = !modal._smartAlbumDescriptionIsCustom || input.value.trim() === previousAuto;
  const nextAuto = smartAlbumDescribeRules(modal._smartAlbumRules || smartAlbumDefaultRules());
  if (keepAutomatic) {
    input.value = nextAuto;
    modal._smartAlbumDescriptionIsCustom = false;
  }
  modal._smartAlbumAutoDescription = nextAuto;
}

function smartAlbumOperators(field) {
  if (field === 'duration') return [
    { value: 'between', label: copyText('app.interface.s_0749e26154') },
    { value: 'at_least', label: copyText('app.interface.s_8476871a9b') },
    { value: 'at_most', label: copyText('app.interface.s_f652eab91a') },
  ];
  if (field === 'filename') return [
    { value: 'contains', label: copyText('app.interface.s_f896fb3776') },
    { value: 'not_contains', label: copyText('app.interface.s_82c5d50f38') },
    { value: 'starts_with', label: copyText('app.interface.s_ab6d48d60e') },
    { value: 'ends_with', label: copyText('app.interface.s_da2e2a52a7') },
  ];
  return [
    { value: 'is', label: copyText('app.interface.s_30160a21b9') },
    { value: 'is_not', label: copyText('app.interface.s_e5d9c6c8a8') },
  ];
}

function smartAlbumSelectControl({ label, value, options, className = '' }) {
  const optionMarkup = options.map(option => `<option value="${escapeHTML(option.value)}"${String(option.value) === String(value) ? ' selected' : ''}>${escapeHTML(option.label)}</option>`).join('');
  return `<label class="echo-field em-select-field smart-album-control ${className}">
    <span class="smart-album-visually-hidden">${escapeHTML(label)}</span>
    <select class="echo-input" aria-label="${escapeHTML(label)}">${optionMarkup}</select>
  </label>`;
}

function smartAlbumRuleValueMarkup(rule, index) {
  if (rule.field === 'media_type') {
    return smartAlbumSelectControl({
      label: copyText('app.interface.d_d80b7abcb8', { v0: (index + 1) }),
      value: rule.value || 'video',
      className: 'smart-album-rule-value',
      options: [
        { value: 'image', label: copyText('app.interface.mediaImage') },
        { value: 'video', label: copyText('app.interface.mediaVideo') },
      ],
    });
  }
  if (rule.field === 'duration') {
    const first = `<input class="echo-input" data-duration-bound="min" type="text" inputmode="numeric" maxlength="8" value="${escapeHTML(rule.min || '')}" aria-label="${escapeHTML(copyText('app.interface.d_smart_rule_min', { index: index + 1 }))}" placeholder="0:00">`;
    const second = `<input class="echo-input" data-duration-bound="max" type="text" inputmode="numeric" maxlength="8" value="${escapeHTML(rule.max || '')}" aria-label="${escapeHTML(copyText('app.interface.d_smart_rule_max', { index: index + 1 }))}" placeholder="10:00">`;
    if (rule.operator === 'between') {
      return `<div class="smart-album-range"><label class="smart-album-control">${first}</label><span class="smart-album-range-separator">${escapeHTML(copyText('app.interface.runtime.rangeTo'))}</span><label class="smart-album-control">${second}</label></div>`;
    }
    const bound = rule.operator === 'at_most' ? 'max' : 'min';
    const value = bound === 'max' ? rule.max : rule.min;
    return `<label class="smart-album-control smart-album-rule-value"><input class="echo-input" data-duration-bound="${bound}" type="text" inputmode="numeric" maxlength="8" value="${escapeHTML(value || '')}" aria-label="${escapeHTML(copyText('app.interface.d_smart_rule_duration', { index: index + 1 }))}" placeholder="10:00"></label>`;
  }
  return `<label class="smart-album-control smart-album-rule-value"><input class="echo-input" data-rule-value type="text" maxlength="80" value="${escapeHTML(rule.value || '')}" aria-label="${escapeHTML(copyText('app.interface.d_smart_rule_filename', { index: index + 1 }))}" placeholder="${escapeHTML(copyText('app.interface.s_ae47ab4ccc'))}"></label>`;
}

function renderSmartAlbumRules(modal) {
  const list = $('#smart-album-rule-list');
  if (!list) return;
  const rules = modal._smartAlbumRules || smartAlbumDefaultRules();
  const fields = [
    { value: 'media_type', label: copyText('app.interface.s_f81f24e056') },
    { value: 'duration', label: copyText('app.interface.s_44b32eb7de') },
    { value: 'filename', label: copyText('app.interface.s_572c5cd955') },
  ];
  list.innerHTML = rules.map((rule, index) => `
    <div class="smart-album-rule-row${rule.field === 'duration' ? ' smart-album-rule-row-duration' : ''}" data-rule-index="${index}">
      ${smartAlbumSelectControl({ label: copyText('app.interface.d_989028dea8', { v0: (index + 1) }), value: rule.field, options: fields, className: 'smart-album-rule-field' })}
      ${smartAlbumSelectControl({ label: copyText('app.interface.d_b5a44da941', { v0: (index + 1) }), value: rule.operator, options: smartAlbumOperators(rule.field), className: 'smart-album-rule-operator' })}
      ${smartAlbumRuleValueMarkup(rule, index)}
      <button class="btn btn-icon btn-iconish smart-album-rule-action smart-album-rule-remove" data-remove-smart-album-rule type="button" aria-label="${escapeHTML(copyText('app.interface.d_smart_rule_remove', { index: index + 1 }))}" title="${escapeHTML(copyText('app.interface.s_4eb2a6f040'))}"${rules.length <= 1 ? ' disabled' : ''}>−</button>
    </div>`).join('');
  const count = $('#smart-album-rule-count');
  if (count) count.textContent = copyText('app.interface.d_5f1ee6e9c8', { v0: (rules.length), v1: (smartAlbumMaxRules) });
  const addButton = $('#smart-album-add-rule');
  if (addButton) {
    addButton.disabled = rules.length >= smartAlbumMaxRules;
    addButton.title = rules.length >= smartAlbumMaxRules ? copyText('app.interface.d_bd044c3057', { v0: (smartAlbumMaxRules) }) : copyText('app.interface.s_af1c0446d5');
  }
}

function readSmartAlbumRules() {
  return $$('.smart-album-rule-row', $('#smart-album-rule-list')).map(row => {
    const field = $('.smart-album-rule-field select', row)?.value || 'media_type';
    const operator = $('.smart-album-rule-operator select', row)?.value || 'is';
    if (field === 'duration') {
      return {
        field,
        operator,
        min: $('[data-duration-bound="min"]', row)?.value.trim() || '',
        max: $('[data-duration-bound="max"]', row)?.value.trim() || '',
      };
    }
    return { field, operator, value: $('[data-rule-value]', row)?.value.trim() || $('.smart-album-rule-value select', row)?.value || '' };
  });
}

function parseSmartAlbumDuration(value) {
  const parts = String(value || '').trim().split(':');
  if (!parts.length || parts.length > 3 || parts.some(part => !/^\d+$/.test(part))) return null;
  const numbers = parts.map(Number);
  if (numbers.slice(1).some(part => part > 59)) return null;
  if (numbers.length === 1) return numbers[0] * 60 * 1000;
  if (numbers.length === 2) return (numbers[0] * 60 + numbers[1]) * 1000;
  return (numbers[0] * 3600 + numbers[1] * 60 + numbers[2]) * 1000;
}

function smartAlbumRuleMatches(photo, rule) {
  if (rule.field === 'media_type') {
    const matches = photo.media_kind === rule.value;
    return rule.operator === 'is_not' ? !matches : matches;
  }
  if (rule.field === 'duration') {
    const duration = Number(photo.duration_ms);
    if (!isVideoMedia(photo) || !Number.isFinite(duration) || duration <= 0) return false;
    const min = parseSmartAlbumDuration(rule.min);
    const max = parseSmartAlbumDuration(rule.max);
    if (rule.operator === 'at_least') return min != null && duration >= min;
    if (rule.operator === 'at_most') return max != null && duration <= max;
    return min != null && max != null && duration >= min && duration <= max;
  }
  const name = String(photo.original_name || '').toLocaleLowerCase();
  const value = String(rule.value || '').trim().toLocaleLowerCase();
  if (!value) return false;
  if (rule.operator === 'not_contains') return !name.includes(value);
  if (rule.operator === 'starts_with') return name.startsWith(value);
  if (rule.operator === 'ends_with') return name.endsWith(value);
  return name.includes(value);
}

function smartAlbumPreviewSource() {
  const pools = state.searchResults.length ? [state.searchResults] : [
    state.photos,
    state.albumPhotos,
    state.favoritePhotos,
    state.randomAlbumPhotos,
    state.memoryPhotos,
  ];
  const seen = new Set();
  const photos = [];
  for (const pool of pools) {
    for (const photo of pool || []) {
      if (!photo) continue;
      const key = String(photo.uuid || photo.id || `${photo.original_name || ''}:${photo.taken_at || ''}`);
      if (seen.has(key)) continue;
      seen.add(key);
      photos.push(photo);
      if (photos.length >= 240) return photos;
    }
  }
  return photos;
}

function renderSmartAlbumPreview(modal) {
  const list = $('#smart-album-preview-list');
  const count = $('#smart-album-preview-count');
  if (!list || !count) return;
  const source = smartAlbumPreviewSource();
  const rules = modal._smartAlbumRules || smartAlbumDefaultRules();
  const mode = modal.dataset.matchMode === 'any' ? 'any' : 'all';
  const matches = source.filter(photo => mode === 'all'
    ? rules.every(rule => smartAlbumRuleMatches(photo, rule))
    : rules.some(rule => smartAlbumRuleMatches(photo, rule)));
  count.textContent = source.length ? copyText('app.interface.d_fd0bbf385e', { v0: (matches.length) }) : '';
  if (!source.length) {
    list.innerHTML = `<div class="smart-album-preview-empty">${copyText('app.interface.s_smart_preview_prompt')}</div>`;
    return;
  }
  if (!matches.length) {
    list.innerHTML = `<div class="smart-album-preview-empty">${copyText('app.interface.s_smart_preview_empty')}</div>`;
    return;
  }

  list.replaceChildren();
  const fragment = document.createDocumentFragment();
  matches.slice(0, 10).forEach(photo => {
    const card = el('article', 'smart-album-preview-item');
    const thumb = el('div', 'smart-album-preview-thumb');
    const img = el('img');
    img.loading = 'lazy';
    img.src = mediaThumbURL(photo);
    img.alt = photo.original_name || '';
    img.onerror = () => {
      if (isVideoMedia(photo)) {
        img.onerror = null;
        img.src = videoPosterPlaceholder;
      } else if (img.dataset.fallbackApplied !== '1') {
        img.dataset.fallbackApplied = '1';
        img.src = mediaFileURL(photo);
      }
    };
    thumb.appendChild(img);
    if (isVideoMedia(photo)) {
      const badge = el('span', 'smart-album-preview-kind');
      badge.textContent = copyText('app.interface.mediaVideo');
      thumb.appendChild(badge);
    }

    const details = searchResultDetails(photo);
    const copy = el('div', 'smart-album-preview-copy');
    const name = el('strong');
    name.textContent = photo.original_name || copyText('app.interface.s_90a5096a0c');
    const meta = el('span');
    meta.textContent = [details.kind, details.size].filter(value => value && value !== '—').join(' · ');
    const date = el('span');
    date.textContent = details.date;
    copy.append(name, meta, date);
    card.append(thumb, copy);
    fragment.appendChild(card);
  });
  list.appendChild(fragment);
}

function bindSmartAlbumViewport(modal) {
  if (modal._smartAlbumViewportHandler) {
    window.removeEventListener('resize', modal._smartAlbumViewportHandler);
    window.visualViewport?.removeEventListener('resize', modal._smartAlbumViewportHandler);
    window.visualViewport?.removeEventListener('scroll', modal._smartAlbumViewportHandler);
  }
  const syncViewport = () => {
    const viewport = window.visualViewport;
    modal.style.setProperty('--smart-album-viewport-height', `${Math.round(viewport?.height || window.innerHeight)}px`);
    modal.style.setProperty('--smart-album-viewport-top', `${Math.round(viewport?.offsetTop || 0)}px`);
  };
  modal._smartAlbumViewportHandler = syncViewport;
  window.addEventListener('resize', syncViewport, { passive: true });
  window.visualViewport?.addEventListener('resize', syncViewport, { passive: true });
  window.visualViewport?.addEventListener('scroll', syncViewport, { passive: true });
  syncViewport();
}

function unbindSmartAlbumViewport(modal) {
  if (modal._smartAlbumViewportHandler) {
    window.removeEventListener('resize', modal._smartAlbumViewportHandler);
    window.visualViewport?.removeEventListener('resize', modal._smartAlbumViewportHandler);
    window.visualViewport?.removeEventListener('scroll', modal._smartAlbumViewportHandler);
    modal._smartAlbumViewportHandler = null;
  }
  modal.style.removeProperty('--smart-album-viewport-height');
  modal.style.removeProperty('--smart-album-viewport-top');
}

function openSmartAlbumModal(album = null) {
  if (!canWriteMedia()) return forbidVisitorAction();
  const modal = $('#smart-album-modal');
  if (!modal) return;
  if (modal._smartAlbumKeydown) document.removeEventListener('keydown', modal._smartAlbumKeydown);
  modal._smartAlbumReturnFocus = document.activeElement;
  modal._smartAlbumEditingAlbumID = album ? Number(album.id) : 0;
  modal._smartAlbumRules = Array.isArray(album && album.smart_rules) && album.smart_rules.length
    ? album.smart_rules.map(rule => ({ ...rule }))
    : smartAlbumDefaultRules();
  modal.dataset.matchMode = album && album.smart_match_mode === 'any' ? 'any' : 'all';
  $('#smart-album-name').value = album ? String(album.name || '') : '';
  const generatedDescription = smartAlbumDescribeRules(modal._smartAlbumRules);
  const storedDescription = String(album && album.description || '').trim();
  modal._smartAlbumAutoDescription = generatedDescription;
  modal._smartAlbumDescriptionIsCustom = Boolean(storedDescription && storedDescription !== generatedDescription);
  $('#smart-album-description').value = storedDescription || generatedDescription;
  $('#smart-album-description').oninput = () => {
    modal._smartAlbumDescriptionIsCustom = $('#smart-album-description').value.trim() !== modal._smartAlbumAutoDescription;
  };
  $('#smart-album-title').textContent = album ? copyText('app.interface.s_a684e2391a') : copyText('app.interface.s_f16a5da6b5');
  const saveLabel = $('.library-editor-action-label', $('#smart-album-create'));
  if (saveLabel) saveLabel.textContent = album ? copyText('app.interface.commonSave') : copyText('app.interface.commonCreate');
  renderSmartAlbumRules(modal);
  $('#smart-album-match-all').setAttribute('aria-pressed', String(modal.dataset.matchMode === 'all'));
  $('#smart-album-match-any').setAttribute('aria-pressed', String(modal.dataset.matchMode === 'any'));
  bindSmartAlbumViewport(modal);
  modal.classList.add('open');
  renderSmartAlbumPreview(modal);

  const setMode = mode => {
    modal.dataset.matchMode = mode;
    $('#smart-album-match-all').setAttribute('aria-pressed', String(mode === 'all'));
    $('#smart-album-match-any').setAttribute('aria-pressed', String(mode === 'any'));
    renderSmartAlbumPreview(modal);
  };
  $('#smart-album-match-all').onclick = () => setMode('all');
  $('#smart-album-match-any').onclick = () => setMode('any');
  $('#smart-album-cancel').onclick = closeSmartAlbumModal;
  $('#smart-album-create').onclick = saveSmartAlbum;
  $('#smart-album-add-rule').onclick = () => {
    const rules = readSmartAlbumRules();
    if (rules.length >= smartAlbumMaxRules) return;
    rules.push({ field: 'media_type', operator: 'is', value: 'image' });
    modal._smartAlbumRules = rules;
    renderSmartAlbumRules(modal);
    syncSmartAlbumDescription(modal);
    renderSmartAlbumPreview(modal);
    requestAnimationFrame(() => $('.smart-album-rule-row:last-child .smart-album-rule-field select')?.focus());
  };
  modal.onclick = event => {
    if (event.target === modal) closeSmartAlbumModal();
  };
  const ruleList = $('#smart-album-rule-list');
  ruleList.onclick = event => {
    const removeButton = event.target.closest('[data-remove-smart-album-rule]');
    if (!removeButton) return;
    const rules = readSmartAlbumRules();
    const index = Number(removeButton.closest('.smart-album-rule-row')?.dataset.ruleIndex);
    if (rules.length <= 1 || !Number.isInteger(index) || index < 0 || index >= rules.length) return;
    rules.splice(index, 1);
    modal._smartAlbumRules = rules;
    renderSmartAlbumRules(modal);
    syncSmartAlbumDescription(modal);
    renderSmartAlbumPreview(modal);
    const focusIndex = Math.min(index, rules.length - 1);
    requestAnimationFrame(() => {
      const row = $$('.smart-album-rule-row', ruleList)[focusIndex];
      (row && $('.smart-album-rule-remove:not(:disabled)', row) || $('.smart-album-rule-field select', row))?.focus({ preventScroll: true });
    });
  };
  ruleList.oninput = () => {
    modal._smartAlbumRules = readSmartAlbumRules();
    syncSmartAlbumDescription(modal);
    renderSmartAlbumPreview(modal);
  };
  ruleList.onchange = event => {
    const row = event.target.closest('.smart-album-rule-row');
    if (!row) return;
    const index = Number(row.dataset.ruleIndex);
    const rules = readSmartAlbumRules();
    if (event.target.matches('.smart-album-rule-field select')) {
      const field = event.target.value;
      rules[index] = field === 'duration'
        ? { field, operator: 'between', min: '8:00', max: '10:00' }
        : field === 'filename'
          ? { field, operator: 'contains', value: '' }
          : { field, operator: 'is', value: 'video' };
    } else if (event.target.matches('.smart-album-rule-operator select')) {
      const rule = rules[index];
      const operator = event.target.value;
      if (rule.field === 'duration') {
        if (operator === 'between') {
          rules[index] = {
            field: 'duration',
            operator,
            min: rule.operator === 'at_most' ? '8:00' : rule.min || '8:00',
            max: rule.operator === 'at_least' ? '10:00' : rule.max || '10:00',
          };
        } else if (operator === 'at_least') {
          rules[index] = { field: 'duration', operator, min: rule.operator === 'at_most' ? rule.max : rule.min || '8:00', max: '' };
        } else {
          rules[index] = { field: 'duration', operator, min: '', max: rule.operator === 'at_least' ? rule.min : rule.max || '10:00' };
        }
      } else {
        rule.operator = operator;
      }
    }
    modal._smartAlbumRules = rules;
    renderSmartAlbumRules(modal);
    syncSmartAlbumDescription(modal);
    renderSmartAlbumPreview(modal);
    const focusIndex = Math.min(index, rules.length - 1);
    const focusSelector = event.target.matches('.smart-album-rule-operator select')
      ? '.smart-album-rule-operator select'
      : '.smart-album-rule-field select';
    requestAnimationFrame(() => $(focusSelector, $$('.smart-album-rule-row', ruleList)[focusIndex])?.focus({ preventScroll: true }));
  };
  modal._smartAlbumKeydown = event => {
    if (event.key === 'Escape' && modal.classList.contains('open')) {
      event.preventDefault();
      closeSmartAlbumModal();
    }
  };
  document.addEventListener('keydown', modal._smartAlbumKeydown);
  if (!window.matchMedia('(max-width: 600px)').matches) {
    requestAnimationFrame(() => $('#smart-album-name')?.focus({ preventScroll: true }));
  }
}

async function saveSmartAlbum() {
  if (!canWriteMedia()) return forbidVisitorAction();
  const modal = $('#smart-album-modal');
  const name = $('#smart-album-name').value.trim();
  if (!name) {
    showToast(copyText('app.interface.s_de8c9c8a84'));
    $('#smart-album-name').focus({ preventScroll: true });
    return;
  }
  const payload = {
    name,
    description: $('#smart-album-description').value.trim(),
    source_kind: 'smart',
    smart_match_mode: modal.dataset.matchMode === 'any' ? 'any' : 'all',
    smart_rules: modal._smartAlbumRules || readSmartAlbumRules(),
  };
  const editingAlbumID = Number(modal._smartAlbumEditingAlbumID) || 0;
  try {
    const saved = await withButtonBusy($('#smart-album-create'), copyText('app.interface.commonSaving'), () => (
      editingAlbumID
        ? api.put(`/api/media/albums/${editingAlbumID}`, payload)
        : api.post('/api/media/albums', payload)
    ));
    closeSmartAlbumModal();
    invalidateAlbumListState();
    if (editingAlbumID) {
      if (state.currentAlbum && Number(state.currentAlbum.id) === editingAlbumID) {
        state.currentAlbum = saved;
        state.currentAlbumID = editingAlbumID;
        state.albumDetailLoadedKey = '';
        state.albumPhotos = [];
        state.albumCursor = '';
        state.albumHasMore = true;
        await renderAlbumDetail();
      } else if (state.view === 'albums') {
        await renderAlbums();
      }
      showToast(copyText('app.interface.s_808784b81b'));
      return;
    }
    state.albumKindFilter = 'smart';
    localStorage.setItem(albumKindFilterStorageKey, state.albumKindFilter);
    switchView('albums');
    showToast(copyText('app.interface.s_407ee438ca'));
  } catch (error) {
    showToast(copyText('app.interface.d_4aa76eef8d', { v0: (error.error || error.message || error) }), 3600);
  }
}

function closeSmartAlbumModal() {
  const modal = $('#smart-album-modal');
  if (!modal) return;
  unbindSmartAlbumViewport(modal);
  modal.classList.remove('open');
  if (modal._smartAlbumKeydown) {
    document.removeEventListener('keydown', modal._smartAlbumKeydown);
    modal._smartAlbumKeydown = null;
  }
  const returnFocus = modal._smartAlbumReturnFocus;
  modal._smartAlbumReturnFocus = null;
  if (returnFocus && typeof returnFocus.focus === 'function' && document.contains(returnFocus)) {
    returnFocus.focus({ preventScroll: true });
  }
}

function renderEditAlbumModal() {
  return `<div class="modal-overlay library-editor-modal" id="edit-album-modal" role="presentation">
  <div class="modal library-editor-card edit-album-card" role="dialog" aria-modal="true" aria-labelledby="edit-album-title">
    <div class="modal-title" id="edit-album-title">${escapeHTML(copyText('app.interface.runtime.editAlbum'))}</div>
    <div class="settings-control">
      <label for="edit-album-name-input"><span>${escapeHTML(copyText('app.interface.runtime.smartAlbumNameSection'))}</span></label>
      <input class="input" id="edit-album-name-input" placeholder="${escapeHTML(copyText('app.interface.runtime.editAlbumNamePlaceholder'))}" maxlength="50">
    </div>
    <div class="settings-control">
      <label for="edit-album-desc-input"><span>${escapeHTML(copyText('app.interface.runtime.editAlbumDescriptionLabel'))}</span></label>
      <input class="input" id="edit-album-desc-input" placeholder="${escapeHTML(copyText('app.interface.runtime.editAlbumDescriptionPlaceholder'))}">
    </div>
    <div class="modal-footer library-editor-actions edit-album-actions">
      ${libraryEditorActionButton({ id: 'cancel-edit-album-btn', kind: 'cancel', icon: icons.libraryEditorCancel, label: copyText('app.interface.commonCancel') })}
      ${libraryEditorActionButton({ id: 'confirm-edit-album-btn', kind: 'save', icon: icons.libraryEditorSave, label: copyText('app.interface.commonSave') })}
    </div>
  </div>
</div>`;
}
function openCreateAlbumModal(afterCreate = null) {
  if (!canWriteMedia()) return forbidVisitorAction();
  const modal = $('#create-album-modal');
  modal._afterCreate = typeof afterCreate === 'function' ? afterCreate : null;
  modal.classList.add('open');
  $('#album-name-input').value = '';
  $('#album-desc-input').value = '';
  $('#cancel-album-btn').onclick = () => {
    modal._afterCreate = null;
    modal.classList.remove('open');
  };
  $('#confirm-album-btn').onclick = createAlbum;
}
function openEditAlbumModal(album = state.currentAlbum) {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!album) return;
  const modal = $('#edit-album-modal');
  if (!modal) return;
  modal.classList.add('open');
  modal.dataset.albumId = String(album.id);
  $('#edit-album-name-input').value = album.name || '';
  $('#edit-album-desc-input').value = album.description || '';
  $('#cancel-edit-album-btn').onclick = () => modal.classList.remove('open');
  $('#confirm-edit-album-btn').onclick = saveEditedAlbum;
}
async function createAlbum() {
  if (!canWriteMedia()) return forbidVisitorAction();
  const modal = $('#create-album-modal');
  const name = $('#album-name-input').value.trim();
  if (!name) { alert(copyText('app.createAlbum.emptyNameAlert')); return; }
  try {
		const createdAlbum = await api.post('/api/media/albums', { name, description: $('#album-desc-input').value.trim() });
	    const afterCreate = modal._afterCreate;
	    modal._afterCreate = null;
	    modal.classList.remove('open');
    invalidateAlbumListState();
    if (typeof afterCreate === 'function') {
      await afterCreate(createdAlbum);
      return;
    }
    // 新建的是用户相册；若当前停留在“文件夹”筛选，不应让刚创建的相册看起来像没有成功。
    if (state.view === 'albums' && state.albumKindFilter !== 'all') {
      state.albumKindFilter = 'all';
      localStorage.setItem(albumKindFilterStorageKey, state.albumKindFilter);
    }
    // 通过 switchView 而不是直接 render，确保菜单高亮和 hash 保持一致。
    switchView('albums');
  } catch(e) { alert(copyText('app.interface.s_3acc59fba4') + (e.error || e)); }
}
async function saveEditedAlbum() {
  if (!canWriteMedia()) return forbidVisitorAction();
  const modal = $('#edit-album-modal');
  const albumID = Number(modal?.dataset.albumId || 0);
  const name = $('#edit-album-name-input').value.trim();
  if (!albumID) return;
  if (!name) {
    alert(copyText('app.interface.s_fa8e656804'));
    return;
  }
  try {
    const updatedAlbum = await withButtonBusy($('#confirm-edit-album-btn'), copyText('app.interface.commonSaving'), () => api.put(`/api/media/albums/${albumID}`, {
      name,
      description: $('#edit-album-desc-input').value.trim(),
    }));
    syncAlbumState(updatedAlbum);
    modal.classList.remove('open');
    if (state.view === 'albums') renderAlbums();
    if (state.currentAlbum && Number(state.currentAlbum.id) === albumID) {
      $('#topbar-title').textContent = state.currentAlbum.name || '';
      const title = $('.album-detail-title');
      if (title) {
        title.textContent = state.currentAlbum.name || '';
        title.title = state.currentAlbum.name || '';
      }
    }
    showToast(copyText('app.interface.s_ef3683c227'));
  } catch (e) {
    alert(copyText('app.interface.s_9bf62efc2e') + (e.error || e));
  }
}

// ── 相册选择弹窗 (b-3) ───────────────────────────────
function renderAlbumPickerModal() {
  return `<div class="modal-overlay library-editor-modal" id="album-picker-modal" role="presentation">
  <div class="modal library-editor-card album-picker-modal" role="dialog" aria-modal="true" aria-labelledby="album-picker-title">
    <div class="modal-title" id="album-picker-title">${icons.album} ${escapeHTML(copyText('app.interface.runtime.chooseAlbum'))}</div>
    <p class="modal-copy album-picker-description">${escapeHTML(copyText('app.interface.runtime.albumPickerCopy'))}</p>
    <div class="album-picker-grid-viewport">
      <div class="album-picker-grid" id="album-picker-grid"></div>
    </div>
    <div class="modal-copy album-picker-hint" id="album-picker-hint" aria-live="polite"></div>
    <div class="modal-footer library-editor-actions album-picker-actions">
      ${libraryEditorActionButton({ id: 'album-picker-cancel', kind: 'cancel', icon: icons.libraryEditorCancel, label: copyText('app.interface.commonCancel') })}
      ${libraryEditorActionButton({ id: 'album-picker-confirm', kind: 'save', icon: icons.libraryEditorSave, label: copyText('app.interface.commonAdd') })}
    </div>
  </div>
</div>`;
}

let _pickerPhotoIds = null;
let _pickerSelected = null;

async function openAlbumPickerModal(photoIds) {
  if (!canWriteMedia()) return forbidVisitorAction();
	// photoIds: null=用已选集合, 数组=指定媒体
  _pickerPhotoIds = photoIds;
  _pickerSelected = null;
  const modal = $('#album-picker-modal');
  const cancelBtn = $('#album-picker-cancel');
  const confirmBtn = $('#album-picker-confirm');
  const setConfirmLabel = label => {
    const labelNode = $('.library-editor-action-label', confirmBtn);
    if (labelNode) labelNode.textContent = label;
    else confirmBtn.textContent = label;
  };
  modal.classList.add('open');
  $('#album-picker-hint').textContent = '';

  // 无论后续加载结果如何，都先绑定好按钮，避免“无相册时只能刷新”的死状态。
  cancelBtn.disabled = false;
  cancelBtn.onclick = () => modal.classList.remove('open');
  confirmBtn.disabled = false;
  setConfirmLabel(copyText('app.interface.commonAdd'));
  confirmBtn.onclick = confirmAlbumPicker;

  const grid = $('#album-picker-grid');
  grid.innerHTML = `<div class="album-picker-state">${copyText('app.interface.commonLoading')}</div>`;

  try {
		const albums = await api.get('/api/media/albums');
    const userAlbums = (albums || []).filter(album => !isFolderAlbum(album) && String(album.source_kind || '').trim() !== 'smart');
    if (!userAlbums.length) {
      grid.innerHTML = `<div class="album-picker-state">${escapeHTML(copyText('app.interface.runtime.noUserAlbums'))}</div>`;
      $('#album-picker-hint').textContent = copyText('app.interface.s_6e4ceff142');
      setConfirmLabel(copyText('app.interface.s_6cd8e9d15d'));
      confirmBtn.onclick = () => {
        modal.classList.remove('open');
        openCreateAlbumModal(() => openAlbumPickerModal(_pickerPhotoIds));
      };
      return;
    }
    grid.innerHTML = '';
    userAlbums.forEach(a => {
      const item = makeAlbumCard(a, { picker: true });
      const selectAlbum = () => {
        $$('.album-card-picker.is-selected', grid).forEach(selected => {
          selected.classList.remove('is-selected');
          selected.setAttribute('aria-pressed', 'false');
        });
        item.classList.add('is-selected');
        item.setAttribute('aria-pressed', 'true');
        _pickerSelected = a;
      };
      item.addEventListener('click', selectAlbum);
      item.addEventListener('keydown', event => {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        event.preventDefault();
        selectAlbum();
      });
      grid.appendChild(item);
    });
  } catch(e) { grid.innerHTML = `<div class="album-picker-state error">${escapeHTML(copyText('app.interface.runtime.loadFailed'))}</div>`; }

}

async function confirmAlbumPicker() {
  if (!canWriteMedia()) return forbidVisitorAction();
  if (!_pickerSelected) { $('#album-picker-hint').textContent = copyText('app.interface.s_56995c3fd8'); return; }
	const album = _pickerSelected;
	const ids = _pickerPhotoIds || [...state.selected];
	if (!ids.length) { $('#album-picker-hint').textContent = copyText('app.interface.s_82e3493ff6'); return; }

	$('#album-picker-confirm').disabled = true;
	const { ok, fail } = await addMediaIDsToAlbum(album, ids);
  $('#album-picker-confirm').disabled = false;
  $('#album-picker-modal').classList.remove('open');
	showToast(copyText('app.interface.runtime.addAlbumResult', { ok, album: album.name, failure: fail ? copyText('app.interface.runtime.addAlbumFailure', { count: fail }) : '' }));
  if (_pickerPhotoIds === null) clearSelection();
}

// ── 分享模态框 ────────────────────────────────────────
function renderShareModal() {
  return `<div class="modal-overlay" id="share-modal">
  <div class="modal">
    <div class="settings-share-row">
      <div class="settings-share-preview" id="share-preview"></div>
      <div class="settings-share-main">
        <div class="settings-share-title">${escapeHTML(copyText('app.share.title'))}</div>
        <p class="settings-share-meta">${escapeHTML(copyText('app.share.subtitle'))}</p>
      </div>
    </div>
    <div class="settings-share-actions">
      <button class="btn" id="share-link-btn" type="button">${icons.shareModalLink} ${escapeHTML(copyText('app.share.link'))}</button>
      <button class="btn" id="share-cancel-btn" type="button">${icons.close} ${escapeHTML(copyText('app.share.cancel'))}</button>
    </div>
  </div>
</div>`;
}
let _shareTarget = null;
function openShareModal(type, targetId) {
  if (!canManageShareLinks()) return forbidVisitorAction();
  _shareTarget = { type, targetId };
  $('#share-modal').classList.add('open');
  renderShareModalPreview();
  const accentStyle = currentAccentButtonStyle();
  $('#share-link-btn')?.setAttribute('style', accentStyle);
  $('#share-cancel-btn').onclick = () => { $('#share-modal').classList.remove('open'); };
  $('#share-link-btn').onclick = generateShareLink;
}
async function generateShareLink() {
  if (!canManageShareLinks()) return forbidVisitorAction();
  if (!_shareTarget) return;
  const body = { type: _shareTarget.type, target_id: _shareTarget.targetId };
  try {
		const link = await api.post('/api/media/shares', body);
    upsertShareLink(link);
    const url = `${location.origin}/s/${link.token}`;
    await navigator.clipboard.writeText(url);
    showToast(copyText('app.share.copiedToast'));
  } catch(e) { alert(copyText('app.interface.s_e7cffa10b2') + (e.error || e)); }
}

function shareModalTargetPhoto() {
  if (!_shareTarget || _shareTarget.type !== 'photo') return null;
  const targetID = Number(_shareTarget.targetId || 0);
  if (!targetID) return null;
  const pools = [
    state.lightboxPhoto,
    ...(Array.isArray(state.photos) ? state.photos : []),
    ...(Array.isArray(state.albumPhotos) ? state.albumPhotos : []),
    ...(Array.isArray(state.randomAlbumPhotos) ? state.randomAlbumPhotos : []),
    ...(Array.isArray(state.favoritesPhotos) ? state.favoritesPhotos : []),
    ...(Array.isArray(state.searchPhotos) ? state.searchPhotos : []),
    ...(Array.isArray(state.trashPhotos) ? state.trashPhotos : []),
  ];
  return pools.find(photo => photo && Number(photo.id) === targetID) || null;
}

function renderShareModalPreview() {
  const wrap = $('#share-preview');
  if (!wrap) return;
  const photo = shareModalTargetPhoto();
  if (photo && photo.uuid) {
    wrap.classList.add('has-image');
    wrap.innerHTML = `<img loading="lazy" src="${escapeHTML(mediaThumbURLFromUUID(photo.uuid))}" alt="${escapeHTML(photo.original_name || '')}">`;
    return;
  }
  wrap.classList.remove('has-image');
  wrap.innerHTML = `<span class="settings-share-preview-icon">${icons.share || ''}</span>`;
}

async function convertImageBlobToClipboardPNG(blob) {
  if (!blob) throw new Error('empty image blob');
  if (blob.type === 'image/png') return blob;

  const sourceURL = URL.createObjectURL(blob);
  try {
    const img = new Image();
    img.decoding = 'async';
    await new Promise((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = () => reject(new Error('image decode failed'));
      img.src = sourceURL;
    });
    const width = Math.max(1, img.naturalWidth || img.width || 1);
    const height = Math.max(1, img.naturalHeight || img.height || 1);
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('canvas context unavailable');
    ctx.drawImage(img, 0, 0, width, height);
    return await new Promise((resolve, reject) => {
      canvas.toBlob(result => {
        if (!result) {
          reject(new Error('png encode failed'));
          return;
        }
        resolve(result);
      }, 'image/png');
    });
  } finally {
    URL.revokeObjectURL(sourceURL);
  }
}

async function copyShareTargetMedia() {
  if (!canManageShareLinks()) return forbidVisitorAction();
  const photo = shareModalTargetPhoto();
  if (!photo || !photo.uuid) {
    showToast(copyText('app.share.copiedMediaFailed'));
    return;
  }
  if (!navigator.clipboard || typeof navigator.clipboard.write !== 'function' || typeof ClipboardItem === 'undefined') {
    showToast(copyText('app.share.copiedMediaUnsupported'));
    return;
  }
  try {
    const response = await fetch(mediaFileURL(photo), { credentials: 'same-origin' });
    if (!response.ok) throw new Error(`media ${response.status}`);
    const blob = await response.blob();
    const mimeType = blob.type || photo.mime_type || '';
    if (!mimeType.startsWith('image/')) throw new Error('unsupported clipboard media type');
    const clipboardBlob = await convertImageBlobToClipboardPNG(blob);
    await navigator.clipboard.write([new ClipboardItem({ [clipboardBlob.type || 'image/png']: clipboardBlob })]);
    showToast(copyText('app.share.copiedMediaToast'));
  } catch (error) {
    console.warn('copy share media failed', error);
    showToast(copyText('app.share.copiedMediaFailed'));
  }
}

// ── 分享列表弹窗 (b-2) ───────────────────────────────
function renderShareListModal() {
  return `<div class="modal-overlay" id="share-list-modal">
  <div class="modal" style="width:480px">
    <div class="modal-title">${icons.share} ${escapeHTML(copyText('app.share.manageTitle'))}</div>
    <div id="share-list-content"></div>
    <div class="modal-footer">
      <button class="btn" id="share-list-close">${escapeHTML(copyText('app.share.cancel'))}</button>
      <button class="btn btn-primary" id="share-list-add">${escapeHTML(copyText('app.share.addNew'))}</button>
    </div>
  </div>
</div>`;
}

function renderPlaybackCacheModal() {
  return `<div class="modal-overlay" id="playback-cache-modal">
  <div class="modal playback-cache-modal">
    <div class="modal-title">${copyText('app.interface.s_f07e25890b')}</div>
    <p class="modal-copy">${escapeHTML(copyText('app.interface.runtime.playbackCacheCopy'))}</p>
    <div id="playback-cache-content">${renderPlaybackCacheModalContent()}</div>
  </div>
</div>`;
}

function renderLibraryLogoGuideModal() {
  return `<div class="modal-overlay" id="library-logo-guide-modal">
  <div class="modal library-logo-crop-modal" role="dialog" aria-modal="true" aria-labelledby="library-logo-guide-title" aria-describedby="library-logo-guide-copy">
    <div class="modal-title" id="library-logo-guide-title">${escapeHTML(copyText('app.logoCrop.title'))}</div>
    <p class="modal-copy" id="library-logo-guide-copy">${escapeHTML(copyText('app.logoCrop.copy'))}</p>
    <div class="library-logo-crop-stage" id="library-logo-crop-stage">
      <img id="library-logo-crop-img" alt="${escapeHTML(copyText('app.logoCrop.alt'))}">
      <div class="library-logo-crop-mask" aria-hidden="true"></div>
    </div>
    <div class="library-logo-crop-toolbar" aria-label="${escapeHTML(copyText('app.logoCrop.zoom'))}">
      <button class="btn library-logo-crop-tool" id="library-logo-crop-zoom-out" type="button" aria-label="${escapeHTML(copyText('app.logoCrop.zoomOut'))}" title="${escapeHTML(copyText('app.logoCrop.zoomOut'))}">−</button>
      <label class="library-logo-crop-control" for="library-logo-crop-zoom">
        <span>${escapeHTML(copyText('app.logoCrop.zoom'))}</span>
        <input id="library-logo-crop-zoom" type="range" min="1" max="3" step="0.01" value="1">
        <output id="library-logo-crop-zoom-value" for="library-logo-crop-zoom">100%</output>
      </label>
      <button class="btn library-logo-crop-tool" id="library-logo-crop-zoom-in" type="button" aria-label="${escapeHTML(copyText('app.logoCrop.zoomIn'))}" title="${escapeHTML(copyText('app.logoCrop.zoomIn'))}">+</button>
      <button class="btn library-logo-crop-tool library-logo-crop-reset" id="library-logo-crop-reset" type="button" aria-label="${escapeHTML(copyText('app.logoCrop.reset'))}" title="${escapeHTML(copyText('app.logoCrop.reset'))}">${icons.refresh || ''}</button>
    </div>
    <p class="library-logo-crop-hint">${escapeHTML(copyText('app.logoCrop.hint'))}</p>
    <div class="modal-footer library-logo-crop-actions">
      <button class="btn library-editor-action-btn library-editor-action-btn-delete" id="library-logo-guide-cancel" type="button" aria-label="${escapeHTML(copyText('app.logoCrop.cancel'))}" title="${escapeHTML(copyText('app.logoCrop.cancel'))}">
        <span class="library-editor-action-icon">${icons.libraryEditorCancel || ''}</span>
        <span class="library-editor-action-label">${escapeHTML(copyText('app.logoCrop.cancel'))}</span>
      </button>
      <button class="btn library-editor-action-btn library-editor-action-btn-save" id="library-logo-guide-confirm" type="button" aria-label="${escapeHTML(copyText('app.logoCrop.confirm'))}" title="${escapeHTML(copyText('app.logoCrop.confirm'))}">
        <span class="library-editor-action-icon">${icons.libraryEditorSave || ''}</span>
        <span class="library-editor-action-label">${escapeHTML(copyText('app.logoCrop.confirm'))}</span>
      </button>
    </div>
  </div>
</div>`;
}

function openLibraryLogoGuideModal(file, onConfirm, onCancel) {
  const objectURL = URL.createObjectURL(file);
  state.pendingLibraryLogoGuide = { file, onConfirm, onCancel, objectURL };
  const modal = $('#library-logo-guide-modal');
  const stage = $('#library-logo-crop-stage');
  const img = $('#library-logo-crop-img');
  const zoom = $('#library-logo-crop-zoom');
  const zoomValue = $('#library-logo-crop-zoom-value');
  const zoomOut = $('#library-logo-crop-zoom-out');
  const zoomIn = $('#library-logo-crop-zoom-in');
  const reset = $('#library-logo-crop-reset');
  const crop = {
    x: 0,
    y: 0,
    zoom: 1,
    dragging: false,
    sx: 0,
    sy: 0,
    ox: 0,
    oy: 0,
    naturalWidth: 0,
    naturalHeight: 0,
    baseScale: 1,
    stageSize: 0,
    pointers: new Map(),
    pinchDistance: 0,
    pinchZoom: 1,
  };
  state.pendingLibraryLogoCrop = crop;
  zoom.value = '1';
  const clamp = (value, min, max) => Math.min(max, Math.max(min, value));
  const pointerDistance = () => {
    const points = [...crop.pointers.values()];
    if (points.length < 2) return 0;
    return Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y);
  };
  const setZoom = value => {
    crop.zoom = clamp(Number(value) || 1, Number(zoom.min) || 1, Number(zoom.max) || 3);
    apply();
  };
  const clampOffset = () => {
    if (!crop.naturalWidth || !crop.naturalHeight || !crop.stageSize) return;
    const scaledWidth = crop.naturalWidth * crop.baseScale * crop.zoom;
    const scaledHeight = crop.naturalHeight * crop.baseScale * crop.zoom;
    const centeredX = (crop.stageSize - scaledWidth) / 2;
    const centeredY = (crop.stageSize - scaledHeight) / 2;
    crop.x = clamp(crop.x, crop.stageSize - scaledWidth - centeredX, -centeredX);
    crop.y = clamp(crop.y, crop.stageSize - scaledHeight - centeredY, -centeredY);
  };
  const apply = () => {
    if (!crop.naturalWidth || !crop.naturalHeight) return;
    crop.stageSize = stage.getBoundingClientRect().width || 280;
    crop.baseScale = Math.max(crop.stageSize / crop.naturalWidth, crop.stageSize / crop.naturalHeight);
    clampOffset();
    const width = crop.naturalWidth * crop.baseScale * crop.zoom;
    const height = crop.naturalHeight * crop.baseScale * crop.zoom;
    img.style.width = `${width}px`;
    img.style.height = `${height}px`;
    img.style.left = `${(crop.stageSize - width) / 2}px`;
    img.style.top = `${(crop.stageSize - height) / 2}px`;
    img.style.transform = `translate(${crop.x}px, ${crop.y}px)`;
    zoom.value = String(crop.zoom);
    updateRangeProgress(zoom);
    if (zoomValue) zoomValue.textContent = `${Math.round(crop.zoom * 100)}%`;
  };
  const resetCrop = () => {
    crop.x = 0;
    crop.y = 0;
    crop.zoom = 1;
    apply();
  };
  zoom.oninput = () => setZoom(zoom.value);
  zoomOut?.addEventListener('click', () => setZoom(crop.zoom - .1));
  zoomIn?.addEventListener('click', () => setZoom(crop.zoom + .1));
  reset?.addEventListener('click', resetCrop);
  stage.onwheel = e => {
    e.preventDefault();
    setZoom(crop.zoom + (e.deltaY < 0 ? .08 : -.08));
  };
  stage.onpointerdown = e => {
    e.preventDefault();
    crop.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
    stage.setPointerCapture(e.pointerId);
    if (crop.pointers.size >= 2) {
      crop.dragging = false;
      crop.pinchDistance = pointerDistance();
      crop.pinchZoom = crop.zoom;
      return;
    }
    crop.dragging = true;
    crop.sx = e.clientX;
    crop.sy = e.clientY;
    crop.ox = crop.x;
    crop.oy = crop.y;
  };
  stage.onpointermove = e => {
    if (!crop.pointers.has(e.pointerId)) return;
    crop.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (crop.pointers.size >= 2) {
      const distance = pointerDistance();
      if (crop.pinchDistance > 0 && distance > 0) setZoom(crop.pinchZoom * distance / crop.pinchDistance);
      return;
    }
    if (crop.dragging) {
      crop.x = crop.ox + e.clientX - crop.sx;
      crop.y = crop.oy + e.clientY - crop.sy;
      apply();
    }
  };
  const releasePointer = e => {
    crop.pointers.delete(e.pointerId);
    crop.dragging = false;
    try { stage.releasePointerCapture(e.pointerId); } catch (_) {}
    if (crop.pointers.size === 1) {
      const remaining = [...crop.pointers.values()][0];
      crop.dragging = true;
      crop.sx = remaining.x;
      crop.sy = remaining.y;
      crop.ox = crop.x;
      crop.oy = crop.y;
    }
  };
  stage.onpointerup = releasePointer;
  stage.onpointercancel = releasePointer;
  img.onload = () => {
    crop.naturalWidth = img.naturalWidth;
    crop.naturalHeight = img.naturalHeight;
    apply();
  };
  img.src = objectURL;
  const handleResize = () => apply();
  const close = (cancelled = false) => {
    modal.classList.remove('open');
    const pending = state.pendingLibraryLogoGuide;
    state.pendingLibraryLogoGuide = null;
    state.pendingLibraryLogoCrop = null;
    window.removeEventListener('resize', handleResize);
    document.removeEventListener('keydown', handleKeydown);
    if (pending && pending.objectURL) URL.revokeObjectURL(pending.objectURL);
    if (cancelled && pending && pending.onCancel) pending.onCancel();
  };
  const handleKeydown = e => {
    if (e.key === 'Escape' && modal.classList.contains('open')) {
      e.preventDefault();
      close(true);
    }
  };
  $('#library-logo-guide-cancel').onclick = () => close(true);
  modal.onclick = e => {
    if (e.target === modal) close(true);
  };
  document.addEventListener('keydown', handleKeydown);
  window.addEventListener('resize', handleResize);
  modal.classList.add('open');
  if (img.complete) {
    crop.naturalWidth = img.naturalWidth;
    crop.naturalHeight = img.naturalHeight;
    apply();
  }
  $('#library-logo-guide-confirm').onclick = async () => {
    const pending = state.pendingLibraryLogoGuide;
    const activeCrop = state.pendingLibraryLogoCrop;
    if (!pending || !activeCrop) return;
    const confirmBtn = $('#library-logo-guide-confirm');
    if (confirmBtn) {
      confirmBtn.disabled = true;
      confirmBtn.textContent = copyText('app.logoCrop.processing');
    }
    try {
      const cropped = await cropLibraryLogoFile(pending.file, activeCrop);
      const callback = pending.onConfirm;
      close(false);
      if (callback) callback(cropped);
    } catch (err) {
      showToast((err && err.message) || copyText('app.interface.s_8d8dcd2c5d'));
      if (confirmBtn) {
        confirmBtn.disabled = false;
        confirmBtn.textContent = copyText('app.logoCrop.confirm');
      }
    }
  };
}

function cropLibraryLogoFile(file, crop, options = {}) {
  return new Promise((resolve, reject) => {
    const img = new Image();
    const url = URL.createObjectURL(file);
    img.onload = () => {
      try {
        const edge = 512;
        const canvas = document.createElement('canvas');
        canvas.width = edge;
        canvas.height = edge;
        const ctx = canvas.getContext('2d');
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, edge, edge);
        let sourceX = 0;
        let sourceY = 0;
        let sourceW = img.naturalWidth;
        let sourceH = img.naturalHeight;
        if (options.autoSquare) {
          const square = Math.min(img.naturalWidth, img.naturalHeight);
          sourceX = Math.max(0, Math.round((img.naturalWidth - square) / 2));
          sourceY = Math.max(0, Math.round((img.naturalHeight - square) / 2));
          sourceW = square;
          sourceH = square;
        } else {
          const stage = $('#library-logo-crop-stage');
          const stageSize = Number(crop && crop.stageSize) || (stage ? stage.getBoundingClientRect().width : 280);
          const baseScale = Number(crop && crop.baseScale) || Math.max(stageSize / img.naturalWidth, stageSize / img.naturalHeight);
          const zoom = Number(crop && crop.zoom) || 1;
          const scale = baseScale * zoom;
          const cropSize = Math.min(img.naturalWidth, img.naturalHeight, stageSize / scale);
          const drawnW = img.naturalWidth * scale;
          const drawnH = img.naturalHeight * scale;
          const offsetX = (stageSize - drawnW) / 2 + ((crop && crop.x) || 0);
          const offsetY = (stageSize - drawnH) / 2 + ((crop && crop.y) || 0);
          sourceX = clampCropSource(-offsetX / scale, img.naturalWidth - cropSize);
          sourceY = clampCropSource(-offsetY / scale, img.naturalHeight - cropSize);
          sourceW = cropSize;
          sourceH = cropSize;
        }
        ctx.drawImage(img, sourceX, sourceY, sourceW, sourceH, 0, 0, edge, edge);
        canvas.toBlob(blob => {
          URL.revokeObjectURL(url);
          if (!blob) {
            reject(new Error(copyText('app.interface.s_dc74d760da')));
            return;
          }
          resolve(new File([blob], `${file.name.replace(/\.[^.]+$/, '') || 'library-logo'}-cropped.png`, { type: 'image/png' }));
        }, 'image/png');
      } catch (err) {
        URL.revokeObjectURL(url);
        reject(err);
      }
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(copyText('app.interface.s_1121bab930')));
    };
    img.src = url;
  });
}

function clampCropSource(value, max) {
  return Math.min(Math.max(0, value), Math.max(0, max));
}

let _shareListTarget = null;
function openShareListModal(type, targetId) {
  if (!canManageShareLinks()) return forbidVisitorAction();
  _shareListTarget = { type, targetId };
  const modal = $('#share-list-modal');
  modal.classList.add('open');
  renderShareList();
  $('#share-list-close').onclick = () => modal.classList.remove('open');
  $('#share-list-add').onclick = () => {
    modal.classList.remove('open');
    openShareModal(type, targetId);
  };
}

function renderShareList() {
  const key   = `${_shareListTarget.type}:${_shareListTarget.targetId}`;
  const links = state.shareMap[key] || [];
  const wrap  = $('#share-list-content');
  if (!links.length) {
    wrap.innerHTML = `<p style="color:var(--text2);padding:12px 0">${escapeHTML(copyText('app.interface.runtime.shareListEmpty'))}</p>`;
    return;
  }
  wrap.innerHTML = '';
  links.forEach(l => {
    const url = `${location.origin}/s/${l.token}`;
    const exp = l.expires_at ? copyText('app.interface.d_fc6bb9d898', { v0: (formatDate(l.expires_at)) }) : copyText('app.interface.shareNeverExpires');
    const row = el('div');
    row.style.cssText = 'display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--border);font-size:.85rem;';
    row.innerHTML = `
      <div style="flex:1;overflow:hidden">
        <div style="font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${url}</div>
        <div style="color:var(--text2);font-size:.75rem;margin-top:2px">${exp}</div>
      </div>
      <button class="btn btn-sm" data-copy="${url}">${escapeHTML(copyText('app.interface.runtime.copy'))}</button>
      <button class="btn btn-sm btn-danger" data-del="${l.id}">${copyText('app.interface.commonDelete')}</button>`;
    row.querySelector('[data-copy]').addEventListener('click', e => {
      navigator.clipboard.writeText(e.target.dataset.copy).then(() => showToast(copyText('app.interface.s_ef309df0e7')));
    });
    row.querySelector('[data-del]').addEventListener('click', async e => {
      const id = parseInt(e.target.dataset.del);
      try {
		await api.del(`/api/media/shares/${id}`);
        removeShareLinkFromState(id);
        renderShareList();
        showToast(copyText('app.interface.s_60d52ad095'));
      } catch(ex) { alert(copyText('app.interface.s_72250c5922')); }
    });
    wrap.appendChild(row);
  });
}

// ── 登出 ──────────────────────────────────────────────
async function shutdownFromUI() {
  try {
    await restartApp();
    showToast(copyText('app.interface.s_55f51dbd2f'));
    setTimeout(() => {
      document.body.innerHTML = `<div class="empty"><p style="display:inline-flex;align-items:center;gap:.42em;">${escapeHTML(copyText('app.restart.restarting'))}<button type="button" aria-label="${escapeHTML(copyText('app.restart.refresh'))}" title="${escapeHTML(copyText('app.restart.refresh'))}" style="display:inline-flex;align-items:center;justify-content:center;width:1em;height:1em;padding:0;border:0;background:none;color:currentColor;cursor:pointer;opacity:.88;"><span style="display:inline-flex;width:1em;height:1em;">${icons.refresh || ''}</span></button></p></div>`;
      document.body.querySelector('.empty button')?.addEventListener('click', () => location.reload());
    }, 500);
  } catch (e) {
    alert(copyText('app.interface.s_80345d3ee1') + ((e && e.error) || e));
  }
}

async function logout() {
  stopPageSessionHeartbeat();
  try {
    await api.post('/api/auth/logout', {}, { suppressForbiddenToast: true });
  } finally {
    await releaseCurrentPageSession({ keepalive: true });
    location.href = '/login';
  }
}

// ── 启动 ──────────────────────────────────────────────
initDeviceClasses();
scheduleInitialDeviceClassRefreshes();
refreshPrimaryScreenOrientationSoon();
initTheme();
initGridScale();
initDisplaySettings();
initSidebarPreference();
initSlideshowSettings();
initLightboxZoom();
initExperimentalSettings();

// c-2: 从 hash 恢复视图，支持刷新后保持页面
function getHashView() {
  const h = location.hash.replace('#', '');
  if (isAllowedRootView(h)) {
    return { view: h, albumID: null };
  }
  if (h.startsWith('album/')) {
    const id = Number(h.split('/')[1]);
    if (Number.isFinite(id) && id > 0) {
      return { view: 'album-detail', albumID: id };
    }
  }
  if (state.experimentalRestoreLastView) {
    const last = localStorage.getItem('last_view_hash') || '';
    if (isAllowedRootView(last)) {
      return { view: last, albumID: null };
    }
  }
  return { view: 'timeline', albumID: null };
}
function setHashView(view, albumID = null) {
  if (view === 'album-detail' && albumID) {
    history.replaceState(null, '', `#album/${albumID}`);
    if (state.experimentalRestoreLastView) localStorage.setItem('last_view_hash', `album/${albumID}`);
    return;
  }
  if (isAllowedRootView(view)) {
    history.replaceState(null, '', '#' + view);
    if (state.experimentalRestoreLastView) localStorage.setItem('last_view_hash', view);
  }
}
const initialRoute = getHashView();
state.view = initialRoute.view;
state.currentAlbumID = initialRoute.albumID;
async function bootstrapApp() {
  const transferred = consumeLoginTransferPageSessionID();
  if (!transferred && navigationType() !== 'reload') {
    resetPageSessionID();
  } else {
    ensurePageSessionID();
  }
  startPageSessionHeartbeat();
  await Promise.all([
    loadCopyCatalog(),
    loadInlineSVGIcons(),
  ]);
  renderApp();
}
window.addEventListener('pagehide', () => {
  void releaseCurrentPageSession({ keepalive: true });
});
window.addEventListener('beforeunload', () => {
  void releaseCurrentPageSession({ keepalive: true });
});
bootstrapApp();
