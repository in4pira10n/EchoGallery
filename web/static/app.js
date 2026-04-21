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
const api = {
  async get(url) {
    const r = await fetch(url);
    if (!r.ok) throw await r.json();
    return r.json();
  },
  async post(url, data) {
    const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    if (!r.ok) throw await r.json();
    return r.json();
  },
  async put(url, data) {
    const r = await fetch(url, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    if (!r.ok) throw await r.json();
    return r.json();
  },
  async del(url) {
    const r = await fetch(url, { method: 'DELETE' });
    if (!r.ok) throw await r.json();
    return r.json();
  },
  async upload(url, formData) {
    const r = await fetch(url, { method: 'POST', body: formData });
    if (!r.ok) throw await r.json();
    return r.json();
  },
};

const videoPosterPlaceholder = `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(`
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 240">
  <rect width="240" height="240" rx="24" fill="#1b2a2f"/>
  <circle cx="120" cy="120" r="54" fill="#2d6a5f"/>
  <polygon points="105,90 105,150 152,120" fill="#f4f1e8"/>
  <text x="120" y="198" font-size="18" text-anchor="middle" fill="#d9e4dd" font-family="sans-serif">VIDEO</text>
</svg>`)} `;
function formatDate(iso) {
  const d = new Date(iso);
  return d.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' });
}
function formatDateTime(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}
function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1048576).toFixed(1) + ' MB';
}
function formatSizeExact(bytes) {
  if (!Number.isFinite(Number(bytes)) || Number(bytes) < 0) return '—';
  return `${formatSize(Number(bytes))} (${Number(bytes).toLocaleString('zh-CN')} B)`;
}
function fileExtension(name) {
  const ext = String(name || '').split('.').pop();
  if (!ext || ext === String(name || '')) return '—';
  return ext.toUpperCase();
}
function escapeHTML(value) {
  return String(value == null ? '' : value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
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
    library.logo_asset ||
    library.logoAsset ||
    library.path ||
    library.storage_path ||
    library.storagePath ||
    'echogallery'
  ).trim().toLowerCase();
}
function defaultLibraryLogoURL(library = {}) {
  const seed = libraryVisualSeed(library);
  return `https://picsum.photos/seed/${encodeURIComponent(`echogallery-${seed || 'default'}`)}/320/320`;
}
function libraryLogoAssetURL(library = {}) {
  const asset = String(library.logo_asset || library.logoAsset || '').trim();
  const index = Number.isFinite(Number(library.index)) ? Number(library.index) : Number(library.logo_index);
  if (!asset || !Number.isInteger(index) || index < 0) return '';
  return `/api/settings/libraries/${index}/logo?v=${encodeURIComponent(asset)}`;
}
function resolveLibraryLogoURL(library = {}) {
  return library.logo_image_url || library.logoImageUrl || libraryLogoAssetURL(library) || defaultLibraryLogoURL(library);
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
function normalizeLibraries(libraries, fallbackPath = '') {
  const result = [];
  const seen = new Set();
  (libraries || []).forEach((library, index) => {
    const path = (library && library.path || '').trim();
    if (!path) return;
    const key = path.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    result.push({
      index,
      name: (library && library.name || '').trim() || `资源库 ${index + 1}`,
      path,
      logo_asset: library && (library.logo_asset || library.logoAsset) || '',
      logo_image_url: library && (library.logo_image_url || library.logoImageUrl) || '',
      accent_color: normalizeHexColor(library && (library.accent_color || library.accentColor) || ''),
    });
  });
  if (!result.length && (fallbackPath || '').trim()) {
    result.push({ index: 0, name: '默认资源库', path: fallbackPath.trim(), logo_asset: '', logo_image_url: '', accent_color: '' });
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
    moved = false;
    timer = setTimeout(() => {
      if (!moved) { e.preventDefault(); callback(e); }
    }, delay);
  }, { passive: false });
  el.addEventListener('touchmove',  () => { moved = true; clearTimeout(timer); });
  el.addEventListener('touchend',   () => clearTimeout(timer));
  el.addEventListener('touchcancel',() => clearTimeout(timer));
}

// ── SVG 图标 ──────────────────────────────────────────
const icons = {
  timeline: '',
  favorite: '',
  album: '',
  shuffle: '',
  trash: '',
  upload: '',
  sun: '',
  moon: '',
  close: '',
  prev: '',
  next: '',
  share: '',
  check: '',
  photo: '',
  logout: '',
  plus: '',
  settings: '',
  shareSmall: '',
  favoriteSmall: '',
  github: '',
  pin: '',
  autoplay: '',
  play: '',
  pause: '',
};
const svgIconFiles = {
  timeline: 'timeline.svg',
  favorite: 'favorite.svg',
  album: 'album.svg',
  shuffle: 'shuffle.svg',
  trash: 'trash.svg',
  upload: 'upload.svg',
  share: 'share.svg',
  settings: 'settings.svg',
  sun: 'sun.svg',
  moon: 'moon.svg',
  close: 'close.svg',
  prev: 'prev.svg',
  next: 'next.svg',
  check: 'check.svg',
  photo: 'photo.svg',
  logout: 'logout.svg',
  plus: 'plus.svg',
  shareSmall: 'share-small.svg',
  favoriteSmall: 'favorite-small.svg',
  github: 'github.svg',
  pin: 'pin.svg',
  autoplay: 'autoplay.svg',
  play: 'play.svg',
  pause: 'pause.svg',
};
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
function initTheme() {
  document.documentElement.dataset.theme = 'light';
}
function toggleTheme() {
  const t = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = t;
  state.theme = t;
  updateThemeBtn();
  applyLibraryBranding();
  persistSettings().catch(() => {});
}
function updateThemeBtn() {
  const btn = $('#theme-btn');
  if (!btn) return;
  const iconEl = btn.querySelector('.theme-icon');
  if (iconEl) iconEl.innerHTML = document.documentElement.dataset.theme === 'dark' ? icons.sun : icons.moon;
}

function initGridScale() {
  state.gridSize = 180;
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
}
function setGridScale(size) {
  state.gridSize = Math.min(260, Math.max(100, Number(size) || 180));
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
}
function initDisplaySettings() {
  state.gridGap = 8;
  state.thumbRadius = 8;
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
  state.sidebarAutoHide = false;
  state.sidebarVisible = !state.sidebarAutoHide;
}
function setSidebarAutoHide(enabled) {
  state.sidebarAutoHide = !!enabled;
  state.sidebarVisible = !enabled;
  clearSidebarHideTimer();
  syncSidebarUI();
}
function clearSidebarHideTimer() {
  if (state.sidebarHideTimer) {
    clearTimeout(state.sidebarHideTimer);
    state.sidebarHideTimer = null;
  }
}
function isMobileLayout() {
  return window.matchMedia('(max-width: 640px)').matches;
}
function syncSidebarUI() {
  const app = $('#app');
  const nav = $('#main-nav');
  const toggle = $('#sidebar-pin-btn');
  if (!app || !nav) return;

  const autoHideActive = state.sidebarAutoHide && !isMobileLayout();
  app.classList.toggle('sidebar-auto-hide', autoHideActive);
  nav.classList.toggle('sidebar-visible', !autoHideActive || state.sidebarVisible);

  if (toggle) {
    toggle.innerHTML = autoHideActive ? `${icons.pin} 固定侧栏` : `${icons.autoplay} 自动隐藏侧栏`;
    toggle.setAttribute('aria-pressed', autoHideActive ? 'false' : 'true');
    toggle.title = autoHideActive ? '切换为固定侧边栏' : '切换为自动隐藏侧边栏';
  }
}
function scheduleSidebarHide(delay = 180) {
  if (!state.sidebarAutoHide || isMobileLayout()) return;
  clearSidebarHideTimer();
  state.sidebarHideTimer = setTimeout(() => {
    state.sidebarVisible = false;
    syncSidebarUI();
  }, delay);
}

function initSlideshowSettings() {
  state.slideshowMode = 'sequential';
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
  const mode = $('#lb-slideshow-mode');
  const interval = $('#lb-slideshow-interval');
  const intervalValue = $('#lb-slideshow-interval-value');
  const loop = $('#lb-slideshow-loop');
  if (btn) {
    btn.innerHTML = state.slideshowPlaying ? `${icons.pause} 暂停` : `${icons.play} 播放`;
    btn.classList.toggle('active', state.slideshowPlaying);
  }
  if (mode) mode.value = state.slideshowMode;
  if (interval) interval.value = String(state.slideshowInterval / 1000);
  if (intervalValue) intervalValue.textContent = `${state.slideshowInterval / 1000} 秒`;
  if (loop) loop.checked = state.slideshowLoop;
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
  return state.slideshowLoop ? 0 : null;
}
function scheduleSlideshowStep() {
  if (!state.slideshowPlaying) return;
  if (state.slideshowTimer) clearTimeout(state.slideshowTimer);
  state.slideshowTimer = setTimeout(() => {
    const nextIndex = getNextSlideshowIndex();
    if (nextIndex == null) {
      stopSlideshow();
      return;
    }
    lbGoTo(nextIndex, { fromSlideshow: true });
  }, state.slideshowInterval);
}
function startSlideshow() {
  if (!state.lightboxPhotos.length) return;
  state.slideshowPlaying = true;
  if (state.slideshowMode === 'random' && !state.slideshowRandomQueue.length) resetRandomQueue();
  updateSlideshowControls();
  scheduleSlideshowStep();
}
function toggleSlideshow() {
  if (state.slideshowPlaying) stopSlideshow();
  else startSlideshow();
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
  state.lightboxZoomMode = 'height';
}
function setLightboxZoom(value) {
  state.lightboxZoom = Math.min(300, Math.max(50, Number(value) || 100));
  state.lightboxZoomMode = 'scale';
  applyLightboxZoom();
}
function setLightboxFitHeight() {
  state.lightboxZoom = 100;
  state.lightboxZoomMode = 'height';
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
  return {
    width: Number(photo.width) || (video && video.videoWidth) || 0,
    height: Number(photo.height) || (video && video.videoHeight) || 0,
  };
}
function applyLightboxZoom() {
  const img = $('#lb-img');
  const video = $('#lb-video');
  const label = $('#lb-zoom-value');
  const input = $('#lb-zoom');
  const body = $('#lightbox .lightbox-body');
  const fitHeightBtn = $('#lb-fit-height');
  const { width, height } = lightboxMediaNaturalSize();
  let zoomed = state.lightboxZoom > 100;
  if (body && width > 0 && height > 0) {
    const viewportWidth = Math.max(1, body.clientWidth);
    const viewportHeight = Math.max(1, body.clientHeight);
    const fitScale = state.lightboxZoomMode === 'height'
      ? viewportHeight / height
      : Math.min(viewportWidth / width, viewportHeight / height, 1);
    const displayScale = fitScale * (state.lightboxZoom / 100);
    const targetWidth = Math.max(1, Math.round(width * displayScale));
    const targetHeight = Math.max(1, Math.round(height * displayScale));
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
  if (fitHeightBtn) fitHeightBtn.classList.toggle('active', state.lightboxZoomMode === 'height');
  if (label) label.textContent = state.lightboxZoomMode === 'height' ? '适应高度' : `${state.lightboxZoom}%`;
  if (input) input.value = String(state.lightboxZoom);
}
function initExperimentalSettings() {
  state.experimentalAutoplayVideo = false;
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
  return {
    index: Number.isInteger(Number(data.index)) ? Number(data.index) : (Number.isInteger(Number(data.logo_index)) ? Number(data.logo_index) : -1),
    name: data.name || '',
    path: data.path || '',
    logo_asset: data.logo_asset || data.logoAsset || '',
    logo_image_url: data.logo_image_url || data.logoImageUrl || '',
    accent_color: normalizeHexColor(data.accent_color || data.accentColor || ''),
  };
}
function normalizeLibraryList(libraries, fallbackPath = '') {
  const normalized = normalizeLibraries(libraries, fallbackPath);
  return normalized.map(item => normalizeLibrary(item));
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
function removeShareLinkFromState(id) {
  let removed = null;
  state.shareLinks = state.shareLinks.filter(link => {
    if (link.id !== id) return true;
    removed = link;
    return false;
  });
  rebuildShareMap();
  refreshShareManagementUI();
  if (removed) refreshPhotoShareIndicators(removed.type, removed.target_id);
}
function createTextFavicon(text) {
  const glyph = (text || 'PA').trim().slice(0, 2) || 'PA';
  return `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#2d6a5f"/><text x="32" y="41" font-size="28" text-anchor="middle" fill="#fff" font-family="Arial, sans-serif">${glyph}</text></svg>`)}`;
}
function resolveActiveLibrary(libraries, currentPath) {
  const normalizedPath = String(currentPath || '').trim();
  return libraries.find(item => item.path === normalizedPath) || libraries[0] || null;
}
function currentLibraryBrand() {
  const draftLibraries = collectLibraryDrafts();
  if (draftLibraries.length) {
    const activeSelect = $('#settings-active-library');
    const activePath = activeSelect ? activeSelect.value.trim() : (state.serverSettings.storage_path || '').trim();
    return resolveActiveLibrary(draftLibraries, activePath);
  }
  return resolveActiveLibrary(state.serverSettings.libraries || [], state.serverSettings.storage_path || '');
}
function libraryFallbackText(library = {}, limit = 3) {
  const label = String(library.name || library.path || 'EchoGallery').trim();
  return escapeHTML((label || 'EG').slice(0, limit));
}
function applyNavLogoFallback(navLogo, library = {}) {
  if (!navLogo) return;
  const img = $('.nav-logo-image', navLogo);
  const media = $('.nav-logo-media', navLogo);
  if (!img || !media) return;
  const fallback = () => {
    media.innerHTML = `<span class="nav-logo-mark">${libraryFallbackText(library)}</span>`;
  };
  img.addEventListener('error', fallback, { once: true });
}
function applyLibraryPreviewFallback(preview, library = {}) {
  if (!preview) return;
  const img = $('img', preview);
  if (!img) return;
  const fallback = () => {
    preview.classList.remove('has-image');
    preview.innerHTML = `<span>${libraryFallbackText(library)}</span>`;
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
  document.title = current && current.name ? `${current.name} - EchoGallery` : 'EchoGallery';
  let favicon = document.querySelector('link[rel="icon"]');
  if (!favicon) {
    favicon = document.createElement('link');
    favicon.rel = 'icon';
    document.head.appendChild(favicon);
  }
  favicon.href = current ? resolveLibraryLogoURL(current) : createTextFavicon(current && current.name ? current.name : 'PA');
  const navLogo = document.querySelector('.nav-logo');
  if (navLogo) {
    navLogo.innerHTML = renderNavLogo();
    applyNavLogoFallback(navLogo, current || {});
  }
}
async function refreshVideoThumbnails() {
  return api.post('/api/settings/video-thumbnails/refresh', {});
}
function renderNavLogo() {
  const current = currentLibraryBrand();
  if (current) {
    const label = current.name || 'EchoGallery';
    return `<span class="nav-logo-media"><img class="nav-logo-image" src="${resolveLibraryLogoURL(current)}" alt="${escapeHTML(label)}"></span><span class="nav-logo-text">${escapeHTML(label)}</span>`;
  }
  const logoText = current && current.name ? current.name.trim() : '';
  const logoMark = logoText ? `<span class="nav-logo-mark">${logoText.slice(0, 3)}</span>` : icons.photo;
  return `${logoMark} ${logoText || 'EchoGallery'}`;
}
function viewShortcutMap() {
  return ['timeline', 'favorites', 'random-album', 'albums', 'trash', 'settings'];
}
function shouldIgnoreGlobalShortcut(target) {
  if (!target) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
}
function handleViewNumberShortcut(e) {
  if (state.blockingInteraction) {
    e.preventDefault();
    e.stopImmediatePropagation();
    return true;
  }
  if (!e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false;
  if (shouldIgnoreGlobalShortcut(e.target)) return false;
  if (!/^[1-6]$/.test(e.key)) return false;
  const view = viewShortcutMap()[Number(e.key) - 1];
  if (!view) return false;
  e.preventDefault();
  e.stopImmediatePropagation();
  closeLightbox();
  closeDrawer();
  switchView(view);
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
  return upper;
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
    map[normalizeKeyToken(key)] = { command, args: parts };
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
  } catch (_) {
    state.serverSettings = {
      port: 8080,
      storage_path: '',
      libraries: [],
      thumbnail_dir: '',
      thumbnail_size: 256,
      trash_dir: '',
      use_system_player: false,
      jwt_secret_masked: '未加载',
      users: [],
      theme: 'light',
      grid_size: 180,
      grid_gap: 8,
      thumb_radius: 8,
      sidebar_auto_hide: false,
      slideshow_mode: 'sequential',
      slideshow_loop: true,
      slideshow_interval: 5000,
      lightbox_zoom: 100,
      experimental_autoplay_video: false,
      experimental_prefetch_neighbors: true,
      experimental_restore_last_view: false,
      player_keymap: '',
    };
    applyServerSettings(state.serverSettings);
  }
  state.settingsDirty = false;
  return state.serverSettings;
}
async function saveServerSettings(payload) {
  const resp = await api.put('/api/settings', payload);
  state.serverSettings = resp.data || state.serverSettings;
  applyServerSettings(state.serverSettings);
  state.settingsDirty = false;
  return resp;
}
async function restartApp() {
  return api.post('/api/settings/restart', {});
}
function applyServerSettings(data = {}) {
  state.serverSettings.port = Number(data.port) || 8080;
  state.serverSettings.storage_path = data.storage_path || '';
  state.serverSettings.libraries = normalizeLibraryList(data.libraries, data.storage_path || '');
  state.serverSettings.thumbnail_dir = data.thumbnail_dir || '';
  state.serverSettings.thumbnail_size = Math.min(512, Math.max(96, Number(data.thumbnail_size) || 256));
  state.serverSettings.trash_dir = data.trash_dir || '';
  state.serverSettings.use_system_player = !!data.use_system_player;
  state.serverSettings.jwt_secret_masked = data.jwt_secret_masked || '未设置';
  state.serverSettings.users = Array.isArray(data.users) ? data.users : [];
  state.theme = data.theme || 'light';
  document.documentElement.dataset.theme = state.theme;
  state.gridSize = Math.min(260, Math.max(100, Number(data.grid_size) || 180));
  state.gridGap = Math.min(24, Math.max(0, Number(data.grid_gap) || 8));
  state.thumbRadius = Math.min(24, Math.max(0, Number(data.thumb_radius) || 8));
  state.sidebarAutoHide = !!data.sidebar_auto_hide;
  state.sidebarVisible = !state.sidebarAutoHide;
  state.slideshowMode = data.slideshow_mode === 'random' ? 'random' : 'sequential';
  state.slideshowLoop = data.slideshow_loop !== false;
  state.slideshowInterval = Math.min(30000, Math.max(1000, Number(data.slideshow_interval) || 5000));
  state.lightboxZoom = Math.min(300, Math.max(50, Number(data.lightbox_zoom) || 100));
  state.lightboxZoomMode = state.lightboxZoom === 100 ? 'height' : 'scale';
  state.experimentalAutoplayVideo = !!data.experimental_autoplay_video;
  state.experimentalPrefetchNeighbors = data.experimental_prefetch_neighbors !== false;
  state.experimentalRestoreLastView = !!data.experimental_restore_last_view;
  updatePlayerKeymapSource(data.player_keymap || '');
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  applyDisplaySettings();
  applyLightboxZoom();
  updateSlideshowControls();
  syncSidebarUI();
  updateThemeBtn();
  applyLibraryBranding();
}
function buildSettingsPayload() {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  let activePath = (state.serverSettings.storage_path || '').trim();
  if (!activePath && libraries.length) activePath = libraries[0].path;
  return {
    port: state.serverSettings.port || 8080,
    storage_path: activePath,
    libraries,
    thumbnail_dir: state.serverSettings.thumbnail_dir || '',
    thumbnail_size: state.serverSettings.thumbnail_size || 256,
    trash_dir: state.serverSettings.trash_dir || '',
    use_system_player: !!state.serverSettings.use_system_player,
    theme: state.theme || 'light',
    grid_size: state.gridSize,
    grid_gap: state.gridGap,
    thumb_radius: state.thumbRadius,
    sidebar_auto_hide: !!state.sidebarAutoHide,
    slideshow_mode: state.slideshowMode,
    slideshow_loop: !!state.slideshowLoop,
    slideshow_interval: state.slideshowInterval,
    lightbox_zoom: state.lightboxZoom,
    experimental_autoplay_video: !!state.experimentalAutoplayVideo,
    experimental_prefetch_neighbors: !!state.experimentalPrefetchNeighbors,
    experimental_restore_last_view: !!state.experimentalRestoreLastView,
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
  return resp;
}
async function deleteLibraryLogo(index) {
  const resp = await api.del(`/api/settings/libraries/${index}/logo`);
  state.serverSettings = resp.data || state.serverSettings;
  applyServerSettings(state.serverSettings);
  return resp;
}
async function openWithSystemPlayer(photoId) {
  await api.post(`/api/media/${photoId}/play`, {});
}
let settingsBootstrapPromise = null;
async function ensureSettingsDataLoaded(force = false) {
  if (force) {
    state.settingsReady = false;
    settingsBootstrapPromise = null;
  }
  if (state.settingsReady) return;
  if (!settingsBootstrapPromise) {
    settingsBootstrapPromise = Promise.allSettled([
      loadServerSettings(),
      state.playerKeymapSource ? Promise.resolve() : initPlayerKeymapSettings(),
    ]).finally(() => {
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
  return true;
}
function adjustVideoVolume(delta) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  video.volume = Math.max(0, Math.min(1, video.volume + delta / 100));
  video.muted = false;
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
  return true;
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
  const preferMutedFirst = isWindowsClient();
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
    showToast('Windows 自动播放已改为静音启动，可手动取消静音');
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
  if (raw === ' ') return 'SPACE';
  return normalizeKeyToken(raw);
}
function handleLightboxKeydown(e) {
  if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.key === '0') {
    e.preventDefault();
    e.stopImmediatePropagation();
    setLightboxFitHeight();
    return true;
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
  if (e.key === 'a' || e.key === 'A') { lbNav(-1); return true; }
  if (e.key === 'd' || e.key === 'D') { lbNav(1); return true; }
  if (e.key === ' ') {
    e.preventDefault();
    toggleSlideshow();
    return true;
  }
  return false;
}
function renderGridScaleControl() {
  return `<label class="grid-scale" title="调整每页显示密度">
    <span>缩放</span>
    <input type="range" id="grid-scale" min="100" max="260" step="10" value="${state.gridSize}">
  </label>`;
}
function bindGridScaleControl() {
  const input = $('#grid-scale');
  if (!input) return;
  input.addEventListener('input', () => setGridScale(input.value));
}
async function revealInFinder(photoId) {
  try {
    await api.post(`/api/media/${photoId}/reveal`, {});
    showToast('已在文件管理器中定位');
  } catch (e) {
    alert('打开失败: ' + (e.error || e));
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
  timelineCursor: '',
  timelineHasMore: true,
  timelineLoading: false,
  trashPhotos: [],
  trashCursor: '',
  trashHasMore: true,
  trashLoading: false,
  albums: [],
  currentAlbum: null,
  albumPhotos: [],
  albumCursor: '',
  albumHasMore: true,
  albumLoading: false,
  selected: new Set(),
  lightboxPhotos: [],
  lightboxIndex: 0,
  lightboxReturnView: '',
  lightboxReturnAlbumID: null,
  // b-2: 当前用户的分享链接，key=`${type}:${targetId}`
  shareMap: {},
  shareLinks: [],
  // d-2: 上传队列状态
  uploadJobs: [],
  uploadRunning: false,
  // h-1: 用于刷新后恢复相册详情页
  currentAlbumID: null,
  sidebarAutoHide: false,
  sidebarVisible: true,
  sidebarHideTimer: null,
  slideshowPlaying: false,
  slideshowMode: 'sequential',
  slideshowLoop: true,
  slideshowInterval: 3000,
  slideshowTimer: null,
  slideshowRandomQueue: [],
  pendingTimelinePhotoID: null,
  lightboxZoom: 100,
  lightboxZoomMode: 'height',
  gridGap: 8,
  thumbRadius: 8,
  serverSettings: {
    port: 8080,
    storage_path: '',
    libraries: [],
    thumbnail_dir: '',
    thumbnail_size: 256,
    trash_dir: '',
    use_system_player: false,
    jwt_secret_masked: '未加载',
    users: [],
    theme: 'light',
    grid_size: 180,
    grid_gap: 8,
    thumb_radius: 8,
    sidebar_auto_hide: false,
    slideshow_mode: 'sequential',
    slideshow_loop: true,
    slideshow_interval: 5000,
    lightbox_zoom: 100,
    experimental_autoplay_video: false,
    experimental_prefetch_neighbors: true,
    experimental_restore_last_view: false,
    player_keymap: '',
  },
  playerKeymap: {},
  playerKeymapSource: '',
  experimentalAutoplayVideo: false,
  experimentalPrefetchNeighbors: true,
  experimentalRestoreLastView: false,
  favoritePhotos: [],
  favoriteCursor: '',
  favoriteHasMore: true,
  favoriteLoading: false,
  settingsDirty: false,
  settingsReady: false,
  settingsFocus: '',
  lightboxPlaybackToken: 0,
  autoplayMutedHintShown: false,
  viewScrollPositions: {},
  loadMoreObserver: null,
  prefetchedMediaKeys: [],
  prefetchedMediaSet: new Set(),
  blockingInteraction: false,
  lightboxPageScrollTop: 0,
};

function viewScrollKeyFor(view = state.view, albumID = state.currentAlbumID) {
  if (view === 'album-detail') return `album-detail:${albumID || 0}`;
  return view;
}
function saveViewScroll(view = state.view, albumID = state.currentAlbumID) {
  state.viewScrollPositions[viewScrollKeyFor(view, albumID)] = window.scrollY || window.pageYOffset || 0;
}
function restoreViewScroll(view = state.view, albumID = state.currentAlbumID) {
  const key = viewScrollKeyFor(view, albumID);
  const top = state.viewScrollPositions[key] || 0;
  requestAnimationFrame(() => {
    if (view !== state.view) return;
    if (view === 'album-detail' && albumID !== state.currentAlbumID) return;
    window.scrollTo(0, top);
  });
}

// ── 分享状态加载 ─────────────────────────────────────
async function loadShareMap() {
  try {
		const links = await api.get('/api/media/shares');
    setShareLinks(links || []);
  } catch(e) { /* 非关键，忽略 */ }
}

function disconnectLoadMoreObserver() {
  if (state.loadMoreObserver) {
    state.loadMoreObserver.disconnect();
    state.loadMoreObserver = null;
  }
}

function resetLightboxPrefetchCache() {
  state.prefetchedMediaKeys = [];
  state.prefetchedMediaSet.clear();
}

function rememberPrefetchedMedia(key) {
  if (!key || state.prefetchedMediaSet.has(key)) return false;
  state.prefetchedMediaSet.add(key);
  state.prefetchedMediaKeys.push(key);
  while (state.prefetchedMediaKeys.length > 24) {
    const expired = state.prefetchedMediaKeys.shift();
    if (expired) state.prefetchedMediaSet.delete(expired);
  }
  return true;
}

function showBlockingProgress(title, detail = '') {
  state.blockingInteraction = true;
  let overlay = $('#blocking-progress');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.id = 'blocking-progress';
    overlay.className = 'blocking-progress';
    overlay.innerHTML = `<div class="blocking-progress-card"><div class="spinner"></div><strong></strong><span></span></div>`;
    document.body.appendChild(overlay);
  }
  $('strong', overlay).textContent = title;
  $('span', overlay).textContent = detail;
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
  if (overlay) overlay.classList.remove('show');
}

// ── 右键菜单 ──────────────────────────────────────────
let _ctxMenu = null;
function showContextMenu(x, y, items) {
  closeContextMenu();
  const menu = el('div');
  menu.style.cssText = `position:fixed;left:${x}px;top:${y}px;z-index:2000;
    background:var(--card);border:1px solid var(--border);border-radius:8px;
    box-shadow:0 4px 16px rgba(0,0,0,.15);padding:4px 0;min-width:160px;`;
  items.forEach(item => {
    if (item === '-') {
      const sep = el('div');
      sep.style.cssText = 'height:1px;background:var(--border);margin:4px 0;';
      menu.appendChild(sep); return;
    }
    const btn = el('button');
    btn.textContent = item.label;
    if (item.danger) btn.style.color = 'var(--danger)';
    btn.style.cssText += `display:block;width:100%;padding:8px 14px;background:none;border:none;
      text-align:left;font-size:.88rem;cursor:pointer;color:${item.danger ? 'var(--danger)' : 'var(--text)'};`;
    btn.addEventListener('mouseenter', () => btn.style.background = 'var(--bg2)');
    btn.addEventListener('mouseleave', () => btn.style.background = 'none');
    btn.addEventListener('click', () => { closeContextMenu(); item.action(); });
    menu.appendChild(btn);
  });
  document.body.appendChild(menu);
  _ctxMenu = menu;
  const rect = menu.getBoundingClientRect();
  if (rect.right > window.innerWidth)   menu.style.left = (x - rect.width) + 'px';
  if (rect.bottom > window.innerHeight) menu.style.top  = (y - rect.height) + 'px';
}
function closeContextMenu() {
  if (_ctxMenu) { _ctxMenu.remove(); _ctxMenu = null; }
}

// ── 渲染框架 ──────────────────────────────────────────
function renderApp() {
  const isDark = document.documentElement.dataset.theme === 'dark';
  document.body.innerHTML = `
<div class="drawer-overlay" id="drawer-overlay"></div>
<div id="app">
  <div class="sidebar-peek-zone" id="sidebar-peek-zone" aria-hidden="true"></div>
  <nav class="nav" id="main-nav">
    <button class="nav-logo" id="nav-logo-btn" type="button" title="打开资源库设置">${renderNavLogo()}</button>
    <a class="nav-item${state.view === 'timeline' ? ' active' : ''}" href="#" data-view="timeline">${icons.timeline} 时间线</a>
    <a class="nav-item${state.view === 'favorites' ? ' active' : ''}" href="#" data-view="favorites">${icons.favorite} 个人收藏</a>
    <a class="nav-item${state.view === 'random-album' ? ' active' : ''}" href="#" data-view="random-album">${icons.shuffle} 乱序相册</a>
    <a class="nav-item${state.view === 'albums' || state.view === 'album-detail' ? ' active' : ''}" href="#" data-view="albums">${icons.album} 相册</a>
    <a class="nav-item${state.view === 'trash' ? ' active' : ''}" href="#" data-view="trash">${icons.trash} 回收站</a>
    <a class="nav-item${state.view === 'settings' ? ' active' : ''}" href="#" data-view="settings">${icons.settings} 设置</a>
    <div class="nav-spacer"></div>
    <div class="nav-bottom">
      <a class="nav-item" href="https://github.com/in4pira10n/EchoGallery" target="_blank" rel="noopener noreferrer">${icons.github} 源码仓库</a>
      <a class="nav-item" href="#" id="theme-btn"><span class="theme-icon">${isDark ? icons.sun : icons.moon}</span> 切换主题</a>
      <a class="nav-item" href="#" id="logout-btn">${icons.logout} 退出登录</a>
    </div>
  </nav>
  <div class="main">
    <div class="topbar">
      <button class="hamburger" id="hamburger-btn" aria-label="菜单">
        <span></span><span></span><span></span>
      </button>
      <button class="btn-icon sidebar-pin-btn" id="sidebar-pin-btn" aria-label="切换侧边栏模式"></button>
      <span class="topbar-title" id="topbar-title"></span>
      <div class="topbar-meta" id="topbar-meta"></div>
      <div id="topbar-actions"></div>
    </div>
    <div class="content" id="content"></div>
  </div>
</div>
${renderLightbox()}
${renderUploadModal()}
${renderCreateAlbumModal()}
${renderShareModal()}
${renderAlbumPickerModal()}
${renderShareListModal()}`;

  bindNav();
  bindGlobal();
  syncSidebarUI();
  ensureSettingsDataLoaded();
  renderView();
}

function bindNav() {
  $$('.nav-item[data-view]').forEach(a => {
    a.addEventListener('click', e => { e.preventDefault(); closeDrawer(); switchView(a.dataset.view); });
  });
  const navLogoBtn = $('#nav-logo-btn');
  if (navLogoBtn) navLogoBtn.addEventListener('click', () => openLibrarySettings());
  $('#theme-btn').addEventListener('click', e => { e.preventDefault(); toggleTheme(); });
  $('#logout-btn').addEventListener('click', e => { e.preventDefault(); logout(); });

  // 汉堡按钮 / 抽屉 (b-6)
  const hamburger = $('#hamburger-btn');
  const overlay   = $('#drawer-overlay');
  const pinBtn    = $('#sidebar-pin-btn');
  const nav       = $('#main-nav');
  if (hamburger) hamburger.addEventListener('click', toggleDrawer);
  if (overlay)   overlay.addEventListener('click', closeDrawer);
  if (pinBtn) pinBtn.addEventListener('click', () => setSidebarAutoHide(!state.sidebarAutoHide));
  if (nav) {
    nav.addEventListener('mouseenter', () => clearSidebarHideTimer());
    nav.addEventListener('mouseleave', () => scheduleSidebarHide());
  }
  window.addEventListener('resize', syncSidebarUI);
}

function toggleDrawer() {
  if (!isMobileLayout()) {
    if (state.sidebarAutoHide) {
      state.sidebarVisible = !state.sidebarVisible;
      syncSidebarUI();
    }
    return;
  }
  const nav     = $('#main-nav');
  const overlay = $('#drawer-overlay');
  const open    = nav && nav.classList.toggle('open');
  if (overlay) overlay.classList.toggle('open', open);
}
function closeDrawer() {
  if (!isMobileLayout()) {
    scheduleSidebarHide(0);
    return;
  }
  const nav     = $('#main-nav');
  const overlay = $('#drawer-overlay');
  if (nav)     nav.classList.remove('open');
  if (overlay) overlay.classList.remove('open');
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

function switchView(view) {
  saveViewScroll();
  disconnectLoadMoreObserver();
  state.view = view;
  state.selected.clear();
  if (view !== 'album-detail') {
    state.currentAlbumID = null;
    setHashView(view); // c-2: 同步到 hash
  }
  $$('.nav-item[data-view]').forEach(a => a.classList.toggle('active', a.dataset.view === view));
  applyLibraryBranding();
  renderView();
}
function renderView() {
  switch (state.view) {
    case 'timeline':     renderTimeline();    break;
    case 'favorites':    renderFavorites();   break;
    case 'random-album': renderRandomAlbum(); break;
    case 'albums':       renderAlbums();      break;
    case 'album-detail': renderAlbumDetail(); break;
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

function appendRandomAlbumGrid(photos) {
  const wrap = $('#random-album-wrap');
  if (!wrap) return;
  let grid = $('#random-album-grid');
  if (!grid) {
    wrap.innerHTML = '';
    grid = el('div', 'photo-grid');
    grid.id = 'random-album-grid';
    wrap.appendChild(grid);
  }
  const fragment = document.createDocumentFragment();
  photos.forEach(photo => fragment.appendChild(makePhotoThumb(photo, state.randomAlbumPhotos)));
  grid.appendChild(fragment);
}

async function fetchAllRandomAlbumPhotos() {
  const allPhotos = [];
  let cursor = '';
  let pageCount = 0;
  showBlockingProgress('正在加载全部时间线媒体', '正在读取第 1 批…');
  for (;;) {
    const params = new URLSearchParams({ limit: '240' });
    if (cursor) params.set('cursor', cursor);
    const url = `/api/media?${params.toString()}`;
    const page = await api.get(url);
    const nextPhotos = page.photos || [];
    allPhotos.push(...nextPhotos);
    pageCount += 1;
    updateBlockingProgress(`已读取 ${allPhotos.length} 条媒体，正在整理乱序相册…`);
    const loadMore = $('#load-more');
    if (loadMore) {
      loadMore.innerHTML = `<div class="spinner"></div>正在收集全部媒体… 已读取 ${allPhotos.length} 条`;
    }
    if (!page.has_more || !page.next_cursor) break;
    cursor = page.next_cursor;
    if (pageCount % 8 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
  }
  updateBlockingProgress(`已读取 ${allPhotos.length} 条媒体，正在打乱顺序…`);
  return shuffleRandomAlbumBatch(allPhotos);
}

async function loadMoreRandomAlbum() {
  if (state.randomAlbumLoading) return;
  state.randomAlbumLoading = true;
  try {
    state.randomAlbumPhotos = await fetchAllRandomAlbumPhotos();
    state.randomAlbumCursor = '';
    state.randomAlbumHasMore = false;
    state.randomAlbumLoaded = true;
  } catch (e) {
    console.error(e);
    throw e;
  } finally {
    state.randomAlbumLoading = false;
    hideBlockingProgress();
    renderRandomAlbumGrid();
  }
}

async function renderRandomAlbum() {
  $('#topbar-title').textContent = '乱序相册';
  $('#topbar-meta').innerHTML = `<div class="topbar-hint">会先收集当前资源库的全部媒体，再统一打乱成一个随机顺序。</div>` + renderGridScaleControl();
  $('#topbar-actions').innerHTML = `<button class="btn btn-sm" id="reshuffle-btn">${icons.shuffle} 重新打乱</button>`;
  bindGridScaleControl();
  $('#reshuffle-btn').addEventListener('click', async () => {
    state.randomAlbumSeed = Date.now();
    state.randomAlbumPhotos = [];
    state.randomAlbumCursor = '';
    state.randomAlbumHasMore = false;
    state.randomAlbumLoaded = false;
    $('#content').innerHTML = `<div id="random-album-wrap"><div class="load-more" id="load-more"><div class="spinner"></div>正在收集全部媒体并打乱…</div></div>`;
    await loadMoreRandomAlbum();
    restoreViewScroll('random-album');
  });

  if (state.randomAlbumLoaded) {
    $('#content').innerHTML = `<div id="random-album-wrap"></div>`;
    renderRandomAlbumGrid();
    restoreViewScroll('random-album');
    return;
  }

  $('#content').innerHTML = `<div id="random-album-wrap"><div class="load-more" id="load-more"><div class="spinner"></div>正在收集全部媒体并打乱…</div></div>`;
  try {
    state.randomAlbumPhotos = [];
    state.randomAlbumCursor = '';
    state.randomAlbumHasMore = false;
    await loadMoreRandomAlbum();
    restoreViewScroll('random-album');
  } catch (e) {
    $('#random-album-wrap').innerHTML = `<p style="color:var(--danger)">加载失败: ${(e && e.error) || e}</p>`;
    restoreViewScroll('random-album');
  }
}

function renderRandomAlbumGrid() {
  const wrap = $('#random-album-wrap');
  if (!wrap) return;
  if (!state.randomAlbumPhotos.length) {
    wrap.innerHTML = `<div class="empty">${icons.shuffle}<p>还没有可浏览的媒体</p></div>`;
    return;
  }
  wrap.innerHTML = '';
  appendRandomAlbumGrid(state.randomAlbumPhotos);
  const loadMore = el('div', 'load-more');
  loadMore.id = 'load-more';
  loadMore.textContent = `已随机载入 ${state.randomAlbumPhotos.length} 条媒体`;
  wrap.appendChild(loadMore);
}

async function renderSettings() {
  $('#topbar-title').textContent = '设置';
  $('#topbar-meta').innerHTML = '<div class="topbar-hint">集中管理服务端配置、本地显示偏好、播放器与实验性功能。</div>';
  $('#topbar-actions').innerHTML = `<div class="topbar-action-group"><button class="btn btn-sm" id="settings-save-restart-top-btn">保存并重启</button><button class="btn btn-primary btn-sm" id="settings-save-top-btn" ${state.settingsDirty ? '' : 'disabled'}>保存设置</button></div>`;
  if (!state.settingsReady) {
    $('#content').innerHTML = `<div class="load-more"><div class="spinner"></div>加载设置中…</div>`;
    await Promise.allSettled([ensureSettingsDataLoaded(), loadShareMap()]);
  }
  if (state.settingsReady && !state.shareLinks.length) await loadShareMap();

  if (state.view !== 'settings') return;

  try {
    renderSettingsContent();
    bindSettingsTopSaveButton();
    restoreViewScroll('settings');
  } catch (e) {
    console.error('render settings failed', e);
    $('#content').innerHTML = `<div class="card settings-panel"><h3>设置加载失败</h3><p>设置项已经回退到当前可用值，你可以刷新后重试。</p><p style="color:var(--danger)">${(e && e.message) || e}</p></div>`;
    restoreViewScroll('settings');
  }
}

function renderLibrarySettingsRows() {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  if (!libraries.length) {
    return `<div class="settings-empty">暂无资源库，请先添加一个路径。</div>`;
  }
  return libraries.map((library, index) => `
    <div class="settings-library-row" data-library-index="${index}" data-logo-asset="${escapeHTML(library.logo_asset || '')}" data-logo-preview-url="${escapeHTML(library.logo_image_url || '')}" data-logo-action="">
      <div class="settings-library-logo-block">
        <div class="settings-library-logo-preview has-image">
          <img src="${resolveLibraryLogoURL(library)}" alt="${escapeHTML(library.name || '资源库 Logo')}">
        </div>
        <label class="settings-library-color-control">
          <span>主色</span>
          <input class="settings-library-accent" type="color" value="${resolveLibraryAccentColor(library)}">
        </label>
        <div class="settings-library-logo-actions">
          <label class="btn btn-sm settings-library-upload-label">
            <input class="settings-library-logo-input" type="file" accept=".png,.jpg,.jpeg,.gif,.webp,image/png,image/jpeg,image/gif,image/webp">
            选择图像
          </label>
          <button class="btn btn-sm settings-library-clear-logo" type="button" ${library.logo_asset ? '' : 'disabled'}>移除图像</button>
        </div>
        <div class="settings-library-logo-hint">${library.logo_asset ? '已设置资源库图像，重新上传会按中心裁剪为正方形，并在侧栏以圆形显示' : '未设置资源库图像；上传后会按中心裁剪为正方形，并在侧栏以圆形显示'}</div>
      </div>
      <div class="settings-library-fields">
        <input class="input settings-library-name" type="text" maxlength="32" placeholder="资源库名称" value="${escapeHTML(library.name)}">
        <input class="input settings-library-path" type="text" placeholder="资源库路径" value="${escapeHTML(library.path)}">
      </div>
      <button class="btn btn-danger btn-sm settings-library-remove" type="button">删除资源库</button>
    </div>
  `).join('');
}

function collectLibraryInputs() {
  const rows = $$('.settings-library-row');
  return rows.map((row, index) => ({
    name: $('.settings-library-name', row).value.trim() || `资源库 ${index + 1}`,
    path: $('.settings-library-path', row).value.trim(),
    logo_asset: row.dataset.logoAsset || '',
    accent_color: normalizeHexColor($('.settings-library-accent', row).value) || '',
  })).filter(item => item.path);
}

function collectLibraryDrafts() {
  const rows = $$('.settings-library-row');
  if (!rows.length) return [];
  return rows.map((row, index) => ({
    name: $('.settings-library-name', row).value.trim() || `资源库 ${index + 1}`,
    path: $('.settings-library-path', row).value.trim(),
    logo_asset: row.dataset.logoAsset || '',
    logo_image_url: row.dataset.logoPreviewUrl || '',
    accent_color: normalizeHexColor($('.settings-library-accent', row).value) || '',
  })).filter(item => item.path);
}

function buildLibraryLogoPreview(row, library = {}) {
  const preview = $('.settings-library-logo-preview', row);
  const hint = $('.settings-library-logo-hint', row);
  const clearBtn = $('.settings-library-clear-logo', row);
  const file = row._pendingLogoFile || null;
  const displayName = $('.settings-library-name', row) ? $('.settings-library-name', row).value.trim() : (library.name || '');
  const displayPath = $('.settings-library-path', row) ? $('.settings-library-path', row).value.trim() : (library.path || '');
  const displayLibrary = {
    index: Number.isInteger(Number(row.dataset.libraryIndex)) ? Number(row.dataset.libraryIndex) : (Number.isInteger(Number(library.index)) ? Number(library.index) : -1),
    name: displayName || library.name || '',
    path: displayPath || library.path || '',
    logo_asset: row.dataset.logoAsset || library.logo_asset || '',
    logo_image_url: library.logo_image_url || '',
    accent_color: $('.settings-library-accent', row) ? $('.settings-library-accent', row).value : (library.accent_color || ''),
  };
  const logoURL = row.dataset.logoPreviewUrl || resolveLibraryLogoURL(displayLibrary);
  const accent = resolveLibraryAccentColor(displayLibrary);
  if (preview) {
    preview.classList.add('has-image');
    preview.innerHTML = `<img src="${logoURL}" alt="${escapeHTML(displayName || '资源库 Logo')}">`;
    preview.style.borderColor = rgbaColor(accent, .35);
    preview.style.boxShadow = `inset 0 0 0 1px ${rgbaColor(accent, .12)}`;
    applyLibraryPreviewFallback(preview, displayLibrary);
  }
  if (hint) {
    if (file) hint.textContent = `待上传：${file.name}，保存后会按中心裁剪为正方形，并在侧栏以圆形显示`;
    else if (row.dataset.logoAction === 'remove') hint.textContent = '保存后将移除资源库图像';
    else hint.textContent = row.dataset.logoAsset
      ? '已设置资源库图像，重新上传会按中心裁剪为正方形，并在侧栏以圆形显示'
      : '未设置资源库图像；上传后会按中心裁剪为正方形，并在侧栏以圆形显示';
  }
  if (clearBtn) clearBtn.disabled = !row.dataset.logoAsset && !file && row.dataset.logoAction !== 'remove';
}

function createLibrarySettingsRow(library = {}, index = 0) {
  const row = el('div', 'settings-library-row');
  const name = library.name || `资源库 ${index + 1}`;
  const path = library.path || '';
  const logoAsset = library.logo_asset || '';
  const logoURL = library.logo_image_url || '';
  const accentColor = resolveLibraryAccentColor(library);
  row.dataset.libraryIndex = index;
  row.dataset.logoAsset = logoAsset;
  row.dataset.logoPreviewUrl = logoURL;
  row.dataset.logoAction = '';
  row.innerHTML = `
    <div class="settings-library-logo-block">
      <div class="settings-library-logo-preview has-image">
        <img src="${resolveLibraryLogoURL(library)}" alt="${escapeHTML(name || '资源库 Logo')}">
      </div>
      <label class="settings-library-color-control">
        <span>主色</span>
        <input class="settings-library-accent" type="color" value="${accentColor}">
      </label>
      <div class="settings-library-logo-actions">
        <label class="btn btn-sm settings-library-upload-label">
          <input class="settings-library-logo-input" type="file" accept=".png,.jpg,.jpeg,.gif,.webp,image/png,image/jpeg,image/gif,image/webp">
          选择图像
        </label>
        <button class="btn btn-sm settings-library-clear-logo" type="button" ${logoAsset ? '' : 'disabled'}>移除图像</button>
      </div>
      <div class="settings-library-logo-hint">${logoAsset ? '已设置资源库图像，重新上传会按中心裁剪为正方形，并在侧栏以圆形显示' : '未设置资源库图像；上传后会按中心裁剪为正方形，并在侧栏以圆形显示'}</div>
    </div>
    <div class="settings-library-fields">
      <input class="input settings-library-name" type="text" maxlength="32" placeholder="资源库名称" value="${escapeHTML(name)}">
      <input class="input settings-library-path" type="text" placeholder="资源库路径" value="${escapeHTML(path)}">
    </div>
    <button class="btn btn-danger btn-sm settings-library-remove" type="button">删除资源库</button>`;
  return row;
}

function syncActiveLibrarySelect() {
  const select = $('#settings-active-library');
  if (!select) return;
  const libraries = collectLibraryDrafts();
  const previousIndex = select.selectedIndex;
  const current = (select.value || state.serverSettings.storage_path || '').trim();
  let activePath = current;
  if (!libraries.some(library => library.path === activePath) && previousIndex >= 0 && libraries[previousIndex]) {
    activePath = libraries[previousIndex].path;
  }
  if (!activePath && libraries.length) activePath = libraries[0].path;
  state.serverSettings.storage_path = activePath || '';
  select.innerHTML = libraries.map((library, index) => {
    const selected = library.path === activePath || (!activePath && index === 0);
    return `<option value="${library.path}" ${selected ? 'selected' : ''}>${library.name}</option>`;
  }).join('') || '<option value="">请先添加资源库</option>';
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

async function persistSettingsWithLibraryAssets() {
  const rows = $$('.settings-library-row');
  const libraryOps = rows
    .map(row => ({
      path: $('.settings-library-path', row).value.trim(),
      action: row.dataset.logoAction || '',
      file: row._pendingLogoFile || null,
    }))
    .filter(operation => operation.path && (operation.action === 'remove' || operation.file));

  const resp = await persistSettings();
  let latestMessage = (resp && resp.message) || '设置已保存';

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

  return { message: latestMessage, data: state.serverSettings };
}

function bindSettingsTopSaveButton() {
  const saveBtn = $('#settings-save-top-btn');
  const restartBtn = $('#settings-save-restart-top-btn');
  if (saveBtn) {
    saveBtn.disabled = !state.settingsDirty;
    saveBtn.onclick = async () => {
      try {
        collectSettingsFormState();
        const resp = await withButtonBusy(saveBtn, '保存中…', () => persistSettingsWithLibraryAssets());
        setSettingsDirty(false);
        showToast((resp && resp.message) || '设置已保存');
        renderSettings();
      } catch (e) {
        alert('保存失败: ' + ((e && e.error) || e.message || e));
      }
    };
  }
  if (restartBtn) {
    restartBtn.onclick = async () => {
      try {
        collectSettingsFormState();
        if (state.settingsDirty) {
          await withButtonBusy(restartBtn, '保存中…', () => persistSettingsWithLibraryAssets());
        }
        const resp = await withButtonBusy(restartBtn, '重启中…', () => restartApp());
        setSettingsDirty(false);
        showToast((resp && resp.message) || '服务正在重启', 2600);
        setTimeout(() => location.reload(), 1800);
      } catch (e) {
        alert('操作失败: ' + ((e && e.error) || e.message || e));
      }
    };
  }
}

function collectSettingsFormState() {
  state.serverSettings.libraries = collectLibraryInputs();
  if (!state.serverSettings.libraries.length) throw new Error('请至少保留一个资源库');
  state.serverSettings.port = parseInt($('#settings-port').value, 10) || 8080;
  state.serverSettings.storage_path = $('#settings-active-library').value.trim() || state.serverSettings.libraries[0].path;
  state.serverSettings.thumbnail_dir = $('#settings-thumbnail-dir').value.trim();
  state.serverSettings.thumbnail_size = parseInt($('#settings-thumbnail-size').value, 10) || 256;
  state.serverSettings.trash_dir = $('#settings-trash-dir').value.trim();
  state.serverSettings.use_system_player = $('#settings-use-system-player').checked;
  updatePlayerKeymapSource($('#settings-player-keymap').value);
  return state.serverSettings;
}

function renderSettingsShareRows() {
  if (!state.shareLinks.length) {
    return `<div class="settings-empty">还没有分享链接。你可以在照片、视频或相册的操作菜单里创建。</div>`;
  }
  return state.shareLinks.map(link => {
    const typeLabel = link.type === 'album' ? '相册' : '照片 / 视频';
    const title = `${typeLabel} #${link.target_id}`;
    const url = `${location.origin}/s/${link.token}`;
    const expires = link.expires_at ? formatDateTime(link.expires_at) : '永不过期';
    const created = link.created_at ? formatDateTime(link.created_at) : '—';
    return `
      <div class="settings-share-row" data-share-id="${link.id}">
        <div class="settings-share-main">
          <div class="settings-share-title">${escapeHTML(title)}</div>
          <div class="settings-share-url">${escapeHTML(url)}</div>
          <div class="settings-share-meta">创建于 ${escapeHTML(created)} · 过期时间 ${escapeHTML(expires)}</div>
        </div>
        <div class="settings-share-actions">
          <button class="btn btn-sm" type="button" data-copy-share="${link.id}">复制</button>
          <a class="btn btn-sm" href="${url}" target="_blank" rel="noopener noreferrer">打开</a>
          <button class="btn btn-danger btn-sm" type="button" data-delete-share="${link.id}">删除</button>
        </div>
      </div>`;
  }).join('');
}

function refreshShareManagementUI() {
  const wrap = $('#settings-share-list');
  if (!wrap) return;
  wrap.innerHTML = renderSettingsShareRows();
  wrap.querySelectorAll('[data-copy-share]').forEach(button => {
    button.onclick = async () => {
      const share = state.shareLinks.find(item => item.id === Number(button.dataset.copyShare));
      if (!share) return;
      await navigator.clipboard.writeText(`${location.origin}/s/${share.token}`);
      showToast('分享链接已复制');
    };
  });
  wrap.querySelectorAll('[data-delete-share]').forEach(button => {
    button.onclick = async () => {
      const shareID = Number(button.dataset.deleteShare);
      if (!shareID) return;
      try {
        await withButtonBusy(button, '删除中…', () => api.del(`/api/media/shares/${shareID}`));
        removeShareLinkFromState(shareID);
        showToast('分享链接已删除');
      } catch (e) {
        alert('删除失败: ' + ((e && e.error) || e));
      }
    };
  });
}

function refreshPhotoShareIndicators(type, targetId) {
  if (type !== 'photo') return;
  const shared = !!state.shareMap[`photo:${targetId}`];
  document.querySelectorAll(`.photo-thumb[data-id="${targetId}"]`).forEach(thumb => {
    const existing = thumb.querySelector('.share-badge');
    if (shared && !existing) thumb.insertAdjacentHTML('beforeend', `<span class="share-badge">${icons.shareSmall}</span>`);
    if (!shared && existing) existing.remove();
  });
}

function renderSettingsContent() {
  const users = (state.serverSettings.users || []).map(name => `<span class="settings-chip">${name}</span>`).join('');
  $('#content').innerHTML = `
<div class="settings-layout">
  <section class="card settings-panel">
    <h3>应用配置</h3>
    <p>保存到服务端配置文件 config.json。端口、资源库路径等变更在重启后完全生效。</p>
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-port"><span>服务端口</span></label>
        <input class="input" id="settings-port" type="number" min="1" max="65535" value="${state.serverSettings.port || 8080}">
      </div>
      <div class="settings-control">
        <label for="settings-active-library"><span>当前启用资源库</span><span>切换后重启服务可完全生效</span></label>
        <select class="input" id="settings-active-library"></select>
      </div>
      <div class="settings-control">
        <label><span>资源库管理</span><span>为每个资源库配一个名字，可在这里增删改</span></label>
        <div class="settings-library-list" id="settings-library-list">${renderLibrarySettingsRows()}</div>
        <div class="settings-actions">
          <button class="btn" id="settings-add-library-btn" type="button">${icons.plus} 添加资源库</button>
        </div>
      </div>
      <div class="settings-control">
        <label for="settings-thumbnail-dir"><span>缩略图目录</span><span>建议放到空间更充足的磁盘，重启后生效</span></label>
        <input class="input" id="settings-thumbnail-dir" type="text" value="${escapeHTML(state.serverSettings.thumbnail_dir || '')}">
      </div>
      <div class="settings-control">
        <label for="settings-thumbnail-size"><span>缩略图生成尺寸</span><span id="settings-thumbnail-size-value">${state.serverSettings.thumbnail_size || 256}px</span></label>
        <input class="input" id="settings-thumbnail-size" type="range" min="96" max="512" step="32" value="${state.serverSettings.thumbnail_size || 256}">
      </div>
      <div class="settings-control">
        <label for="settings-trash-dir"><span>回收站目录</span><span>永久删除时移动到这里</span></label>
        <input class="input" id="settings-trash-dir" type="text" value="${escapeHTML(state.serverSettings.trash_dir || '')}">
      </div>
      <label class="settings-checkbox">
        <input id="settings-use-system-player" type="checkbox" ${state.serverSettings.use_system_player ? 'checked' : ''}>
        <span>视频优先使用系统播放器</span>
      </label>
      <div class="settings-static">
        <strong>JWT Secret</strong>
        <span>${state.serverSettings.jwt_secret_masked || '未设置'}</span>
      </div>
      <div class="settings-static">
        <strong>用户列表</strong>
        <div class="settings-chip-row">${users || '<span class="settings-empty">暂无用户</span>'}</div>
      </div>
    </div>
  </section>

  <section class="card settings-panel">
    <h3>浏览显示</h3>
    <p>这些设置现在统一保存到 config.json，调整后会立即生效。</p>
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-grid-size"><span>缩略图大小</span><span id="settings-grid-size-value">${state.gridSize}px</span></label>
        <input class="input" id="settings-grid-size" type="range" min="100" max="260" step="10" value="${state.gridSize}">
      </div>
      <div class="settings-control">
        <label for="settings-grid-gap"><span>图像间距</span><span id="settings-grid-gap-value">${state.gridGap}px</span></label>
        <input class="input" id="settings-grid-gap" type="range" min="0" max="24" step="1" value="${state.gridGap}">
      </div>
      <div class="settings-control">
        <label for="settings-thumb-radius"><span>图像圆角</span><span id="settings-thumb-radius-value">${state.thumbRadius}px</span></label>
        <input class="input" id="settings-thumb-radius" type="range" min="0" max="24" step="1" value="${state.thumbRadius}">
      </div>
      <label class="settings-checkbox">
        <input id="settings-sidebar-auto-hide" type="checkbox" ${state.sidebarAutoHide ? 'checked' : ''}>
        <span>自动隐藏侧边栏（仅手动按钮展开）</span>
      </label>
    </div>
  </section>

  <section class="card settings-panel">
    <h3>灯箱、播放器与性能</h3>
    <p>当关闭“系统播放器”时，将使用内置播放器并应用 IINA 风格快捷键。</p>
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-lightbox-zoom"><span>默认灯箱缩放</span><span id="settings-lightbox-zoom-value">${state.lightboxZoom}%</span></label>
        <input class="input" id="settings-lightbox-zoom" type="range" min="50" max="300" step="10" value="${state.lightboxZoom}">
      </div>
      <div class="settings-control">
        <label for="settings-slideshow-interval"><span>默认幻灯片间隔</span><span id="settings-slideshow-interval-value">${state.slideshowInterval / 1000} 秒</span></label>
        <input class="input" id="settings-slideshow-interval" type="range" min="1" max="30" step="1" value="${state.slideshowInterval / 1000}">
      </div>
      <label class="settings-checkbox">
        <input id="settings-slideshow-loop" type="checkbox" ${state.slideshowLoop ? 'checked' : ''}>
        <span>幻灯片循环播放</span>
      </label>
      <label class="settings-checkbox">
        <input id="settings-slideshow-random" type="checkbox" ${state.slideshowMode === 'random' ? 'checked' : ''}>
        <span>幻灯片默认随机模式</span>
      </label>
      <div class="settings-control">
        <label for="settings-player-keymap"><span>IINA 快捷键映射</span><span>支持 .conf 风格</span></label>
        <textarea class="input settings-textarea" id="settings-player-keymap" rows="12" ${state.serverSettings.use_system_player ? 'disabled' : ''}></textarea>
      </div>
      <div class="settings-actions">
        <button class="btn" id="settings-reset-keymap-btn" ${state.serverSettings.use_system_player ? 'disabled' : ''}>恢复默认快捷键</button>
      </div>
      <label class="settings-checkbox">
        <input id="settings-autoplay-video" type="checkbox" ${state.experimentalAutoplayVideo ? 'checked' : ''}>
        <span>视频打开后自动播放</span>
      </label>
      <label class="settings-checkbox">
        <input id="settings-prefetch-neighbors" type="checkbox" ${state.experimentalPrefetchNeighbors ? 'checked' : ''}>
        <span>预加载前后相邻媒体</span>
      </label>
      <div class="settings-actions">
        <button class="btn" id="settings-refresh-video-thumbs-btn">刷新视频缩略图</button>
      </div>
    </div>
  </section>

  <section class="card settings-panel">
    <h3>实验性功能</h3>
    <p>这些功能还在打磨中，可能会继续调整行为，开启状态会写入 config.json。</p>
    <div class="settings-group">
      <label class="settings-checkbox">
        <input id="settings-exp-restore-last-view" type="checkbox" ${state.experimentalRestoreLastView ? 'checked' : ''}>
        <span>启动时恢复上次浏览页面</span>
      </label>
      <div class="settings-static">
        <strong>快捷切页</strong>
        <span>支持 macOS Option + 1-6、Windows Alt + 1-6 切换到时间线、个人收藏、乱序相册、相册、回收站、设置。</span>
      </div>
    </div>
  </section>

  <section class="card settings-panel">
    <h3>分享链接</h3>
    <p>这里集中查看、复制和删除当前账号创建的全部分享链接。</p>
    <div class="settings-group">
      <div class="settings-control">
        <label><span>链接管理</span><span>删除后原链接会立刻失效</span></label>
        <div class="settings-share-list" id="settings-share-list">${renderSettingsShareRows()}</div>
      </div>
    </div>
  </section>
</div>`;

  syncActiveLibrarySelect();
  $('#settings-thumbnail-dir').value = state.serverSettings.thumbnail_dir || '';
  $('#settings-thumbnail-size').value = String(state.serverSettings.thumbnail_size || 256);
  $('#settings-trash-dir').value = state.serverSettings.trash_dir || '';
  $('#settings-player-keymap').value = state.playerKeymapSource || '';

  $('#settings-add-library-btn').addEventListener('click', () => {
    const list = $('#settings-library-list');
    const index = $$('.settings-library-row', list).length;
    const row = createLibrarySettingsRow({
      name: `资源库 ${index + 1}`,
      path: '',
      logo_asset: '',
      logo_image_url: '',
      accent_color: defaultLibraryAccentPalette[index % defaultLibraryAccentPalette.length],
    }, index);
    list.appendChild(row);
    bindLibraryRow(row);
    syncActiveLibrarySelect();
    applyLibraryBranding();
    setSettingsDirty();
  });

  function bindLibraryRow(row) {
    $('.settings-library-remove', row).addEventListener('click', () => {
      row.remove();
      reindexLibrarySettingsRows();
      syncActiveLibrarySelect();
      applyLibraryBranding();
      setSettingsDirty();
    });
    const logoInput = $('.settings-library-logo-input', row);
    const clearLogoBtn = $('.settings-library-clear-logo', row);
    if (logoInput) {
      logoInput.addEventListener('change', e => {
        const file = e.target.files && e.target.files[0];
        if (!file) return;
        if (row.dataset.logoPreviewUrl && row.dataset.logoPreviewUrl.startsWith('blob:')) {
          URL.revokeObjectURL(row.dataset.logoPreviewUrl);
        }
        row._pendingLogoFile = file;
        row.dataset.logoAction = 'upload';
        row.dataset.logoPreviewUrl = URL.createObjectURL(file);
        buildLibraryLogoPreview(row);
        applyLibraryBranding();
        setSettingsDirty();
      });
    }
    if (clearLogoBtn) {
      clearLogoBtn.addEventListener('click', () => {
        if (row.dataset.logoPreviewUrl && row.dataset.logoPreviewUrl.startsWith('blob:')) {
          URL.revokeObjectURL(row.dataset.logoPreviewUrl);
        }
        row._pendingLogoFile = null;
        const hadAsset = !!row.dataset.logoAsset;
        row.dataset.logoPreviewUrl = '';
        row.dataset.logoAction = hadAsset ? 'remove' : '';
        if (logoInput) logoInput.value = '';
        buildLibraryLogoPreview(row);
        applyLibraryBranding();
        setSettingsDirty();
      });
    }
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
  $$('.settings-library-row').forEach(bindLibraryRow);
  reindexLibrarySettingsRows();
  $('#settings-active-library').addEventListener('change', e => {
    state.serverSettings.storage_path = e.target.value;
    applyLibraryBranding();
    setSettingsDirty();
  });
  $('#settings-port').addEventListener('input', () => setSettingsDirty());
  $('#settings-thumbnail-dir').addEventListener('input', () => setSettingsDirty());
  $('#settings-thumbnail-size').addEventListener('input', e => {
    $('#settings-thumbnail-size-value').textContent = `${e.target.value}px`;
    setSettingsDirty();
  });
  $('#settings-trash-dir').addEventListener('input', () => setSettingsDirty());
  $('#settings-use-system-player').addEventListener('change', e => {
    $('#settings-player-keymap').disabled = e.target.checked;
    $('#settings-reset-keymap-btn').disabled = e.target.checked;
    setSettingsDirty();
  });

  $('#settings-grid-size').addEventListener('input', e => {
    setGridScale(e.target.value);
    $('#settings-grid-size-value').textContent = `${state.gridSize}px`;
    setSettingsDirty();
  });
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
  $('#settings-sidebar-auto-hide').addEventListener('change', e => { setSidebarAutoHide(e.target.checked); setSettingsDirty(); });
  $('#settings-lightbox-zoom').addEventListener('input', e => {
    setLightboxZoom(e.target.value);
    $('#settings-lightbox-zoom-value').textContent = `${state.lightboxZoom}%`;
    setSettingsDirty();
  });
  $('#settings-slideshow-interval').addEventListener('input', e => {
    updateSlideshowSetting('interval', parseInt(e.target.value, 10) * 1000);
    $('#settings-slideshow-interval-value').textContent = `${state.slideshowInterval / 1000} 秒`;
    setSettingsDirty();
  });
  $('#settings-slideshow-loop').addEventListener('change', e => { updateSlideshowSetting('loop', e.target.checked); setSettingsDirty(); });
  $('#settings-slideshow-random').addEventListener('change', e => { updateSlideshowSetting('mode', e.target.checked ? 'random' : 'sequential'); setSettingsDirty(); });
  $('#settings-player-keymap').addEventListener('input', () => setSettingsDirty());
  $('#settings-reset-keymap-btn').addEventListener('click', async () => {
    try {
      await resetPlayerKeymapToDefault();
      setSettingsDirty();
      showToast('已恢复默认快捷键，记得保存设置');
    } catch (e) {
      alert('恢复失败: ' + ((e && e.error) || e));
    }
  });
  $('#settings-autoplay-video').addEventListener('change', e => { setExperimentalSetting('autoplayVideo', e.target.checked); setSettingsDirty(); });
  $('#settings-prefetch-neighbors').addEventListener('change', e => { setExperimentalSetting('prefetchNeighbors', e.target.checked); setSettingsDirty(); });
  $('#settings-refresh-video-thumbs-btn').addEventListener('click', async () => {
    const btn = $('#settings-refresh-video-thumbs-btn');
    try {
      const resp = await withButtonBusy(btn, '刷新中…', () => refreshVideoThumbnails());
      const data = resp.data || {};
      showToast(`视频缩略图刷新完成：共 ${data.total || 0} 个，成功 ${data.refreshed || 0} 个，失败 ${data.failed || 0} 个`, 3200);
      if (data.errors && data.errors.length) {
        alert(`以下文件刷新失败：\n${data.errors.join('\n')}`);
      }
    } catch (e) {
      alert('刷新失败: ' + ((e && e.error) || e));
    }
  });
  $('#settings-exp-restore-last-view').addEventListener('change', e => { setExperimentalSetting('restoreLastView', e.target.checked); setSettingsDirty(); });
  refreshShareManagementUI();
  applyLibraryBranding();
  if (state.settingsFocus === 'libraries') {
    const target = $('#settings-library-list');
    if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    state.settingsFocus = '';
  }
}

// ── 时间线视图 ─────────────────────────────────────────
async function renderTimeline() {
  $('#topbar-title').textContent = '时间线';
  $('#topbar-meta').innerHTML = `<span class="topbar-hint" id="timeline-jump-status" hidden></span>` + renderGridScaleControl();
  $('#topbar-actions').innerHTML = `<button class="btn btn-primary btn-sm" id="upload-btn">${icons.upload} 上传</button>`;
  bindGridScaleControl();
  $('#upload-btn').addEventListener('click', openUploadModal);

  $('#content').innerHTML = `
<div class="toolbar">
  <span id="sel-bar" class="selected-bar">
    <span class="selected-count" id="sel-count">0</span> 张已选
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="download-sel-btn">下载选中</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="add-to-album-btn">${icons.album} 添加到相册</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="delete-sel-btn">${icons.trash} 删除</button>
    <button class="btn-icon" style="color:#fff" id="clear-sel-btn">${icons.close}</button>
  </span>
</div>
<div id="timeline-groups"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;

  $('#clear-sel-btn').addEventListener('click', clearSelection);
  $('#download-sel-btn').addEventListener('click', downloadSelected);
  $('#delete-sel-btn').addEventListener('click', deleteSelected);
  $('#add-to-album-btn').addEventListener('click', () => openAlbumPickerModal(null));

  state.photos = [];
  state.timelineCursor = '';
  state.timelineHasMore = true;
  const hasPendingFocus = !!state.pendingTimelinePhotoID;
  await loadShareMap();   // b-2: 加载分享状态
  await loadMoreTimeline();
  const focusedPending = await focusPendingTimelinePhoto();
  observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading);
  if (!hasPendingFocus || !focusedPending) restoreViewScroll('timeline');
}

function setTimelineJumpStatus(message = '') {
  const status = $('#timeline-jump-status');
  if (!status) return;
  status.textContent = message;
  status.hidden = !message;
}

async function renderFavorites() {
  $('#topbar-title').textContent = '个人收藏';
  $('#topbar-meta').innerHTML = renderGridScaleControl();
  $('#topbar-actions').innerHTML = '';
  bindGridScaleControl();

  $('#content').innerHTML = `
<div class="toolbar">
  <span id="sel-bar" class="selected-bar">
    <span class="selected-count" id="sel-count">0</span> 条已选
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="download-sel-btn">下载选中</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="add-to-album-btn">${icons.album} 添加到相册</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="unfavorite-sel-btn">${icons.favorite} 取消收藏</button>
    <button class="btn-icon" style="color:#fff" id="clear-sel-btn">${icons.close}</button>
  </span>
</div>
<div id="favorite-groups"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;

  $('#clear-sel-btn').addEventListener('click', clearSelection);
  $('#download-sel-btn').addEventListener('click', downloadSelected);
  $('#add-to-album-btn').addEventListener('click', () => openAlbumPickerModal(null));
  $('#unfavorite-sel-btn').addEventListener('click', unfavoriteSelected);

  state.favoritePhotos = [];
  state.favoriteCursor = '';
  state.favoriteHasMore = true;
  await loadShareMap();
  await loadMoreFavorites();
  observeLoadMore('load-more', loadMoreFavorites, () => state.favoriteHasMore && !state.favoriteLoading);
  restoreViewScroll('favorites');
}

async function loadMoreTimeline() {
  if (state.timelineLoading || !state.timelineHasMore) return;
  state.timelineLoading = true;
  try {
    const params = new URLSearchParams({ limit: state.pendingTimelinePhotoID ? '180' : '30' });
    if (state.timelineCursor) params.set('cursor', state.timelineCursor);
    const url = `/api/media?${params.toString()}`;
    const page = await api.get(url);
    state.photos.push(...(page.photos || []));
    state.timelineCursor = page.next_cursor || '';
    state.timelineHasMore = page.has_more || false;
    renderTimelineGroups(page.photos || [], state.photos.length - (page.photos || []).length);
  } catch (e) { console.error(e); }
  finally {
    state.timelineLoading = false;
    updateLoadMoreUI('load-more', state.timelineHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading);
  }
}

function renderTimelineGroups(newPhotos, offset) {
  const container = $('#timeline-groups');
  if (!container) return;
  if (offset === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>还没有媒体，点击右上角上传吧</p></div>`;
    return;
  }
  const groups = groupByDate(newPhotos);
  for (const [date, photos] of Object.entries(groups)) {
    let group = container.querySelector(`[data-date="${CSS.escape(date)}"]`);
    if (!group) {
      group = el('div', 'date-group');
      group.dataset.date = date;
      group.innerHTML = `<div class="date-label"><span class="date-text">${date}</span><button class="date-select-all" type="button">全选</button></div><div class="photo-grid"></div>`;
      group.querySelector('.date-select-all').addEventListener('click', () => selectAllInGroup(group));
      container.appendChild(group);
    }
    const grid = group.querySelector('.photo-grid');
    photos.forEach(p => grid.appendChild(makePhotoThumb(p, state.photos)));
  }
}

function renderFavoriteGroups(newPhotos) {
  const container = $('#favorite-groups');
  if (!container) return;
  if (state.favoritePhotos.length === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.favorite}<p>还没有加入个人收藏的媒体</p></div>`;
    return;
  }
  const groups = groupByDate(newPhotos);
  for (const [date, photos] of Object.entries(groups)) {
    let group = container.querySelector(`[data-date="${CSS.escape(date)}"]`);
    if (!group) {
      group = el('div', 'date-group');
      group.dataset.date = date;
      group.innerHTML = `<div class="date-label"><span class="date-text">${date}</span><button class="date-select-all" type="button">全选</button></div><div class="photo-grid"></div>`;
      group.querySelector('.date-select-all').addEventListener('click', () => selectAllInGroup(group));
      container.appendChild(group);
    }
    const grid = group.querySelector('.photo-grid');
    photos.forEach(p => grid.appendChild(makePhotoThumb(p, state.favoritePhotos)));
  }
}

async function loadMoreFavorites() {
  if (state.favoriteLoading || !state.favoriteHasMore) return;
  state.favoriteLoading = true;
  try {
    const url = '/api/media/favorites' + (state.favoriteCursor ? `?cursor=${encodeURIComponent(state.favoriteCursor)}` : '');
    const page = await api.get(url);
    state.favoritePhotos.push(...(page.photos || []));
    state.favoriteCursor = page.next_cursor || '';
    state.favoriteHasMore = page.has_more || false;
    renderFavoriteGroups(page.photos || []);
  } catch (e) {
    console.error(e);
  } finally {
    state.favoriteLoading = false;
    updateLoadMoreUI('load-more', state.favoriteHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreFavorites, () => state.favoriteHasMore && !state.favoriteLoading);
  }
}

function isVideoMedia(photo) {
  return photo && photo.media_kind === 'video';
}

function mediaThumbURL(photo) {
  return `/media/thumbnails/${photo.uuid}`;
}

function mediaFileURL(photo) {
  return isVideoMedia(photo) ? `/media/files/${photo.uuid}` : `/media/photos/${photo.uuid}`;
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
  const gcd = (a, b) => (b ? gcd(b, a % b) : a);
  const divisor = gcd(w, h) || 1;
  return `${Math.round(w / divisor)}:${Math.round(h / divisor)}`;
}

// ── 缩略图 ────────────────────────────────────────────
function makePhotoThumb(photo, listRef, opts = {}) {
  const div = el('div', 'photo-thumb');
  div.dataset.id = photo.id;
  div.dataset.kind = photo.media_kind || 'image';

  const isShared = !!state.shareMap[`photo:${photo.id}`];
  const favoriteToggle = opts.trashMode
    ? ''
    : `<button class="favorite-toggle${photo.is_favorite ? ' active' : ''}" type="button" aria-label="${photo.is_favorite ? '取消收藏' : '加入收藏'}">${icons.favoriteSmall}</button>`;
  const shareBadge = isShared
    ? `<span class="share-badge">${icons.shareSmall}</span>` : '';
  const mediaBadge = isVideoMedia(photo)
    ? `<span class="media-badge">视频${photo.duration_ms ? ` · ${formatDuration(photo.duration_ms)}` : ''}</span>`
    : '';
  const thumbFallback = isVideoMedia(photo) ? ` onerror="this.onerror=null;this.src='${videoPosterPlaceholder}'"` : '';

  div.innerHTML = `<span class="check">${icons.check}</span>${favoriteToggle}<img loading="lazy" src="${mediaThumbURL(photo)}" alt="${photo.original_name}"${thumbFallback}>${shareBadge}${mediaBadge}`;
  const imageEl = div.querySelector('img');
  if (imageEl && !isVideoMedia(photo)) {
    imageEl.addEventListener('error', () => {
      if (imageEl.dataset.fallbackApplied === '1') return;
      imageEl.dataset.fallbackApplied = '1';
      imageEl.src = mediaFileURL(photo);
    });
  }

  // b-1: 点击 .check 区域直接进入/切换选择模式
  const checkEl = div.querySelector('.check');
  checkEl.addEventListener('click', e => {
    e.stopPropagation();
    toggleSelect(photo.id, div);
  });
  const favoriteEl = div.querySelector('.favorite-toggle');
  if (favoriteEl) {
    favoriteEl.addEventListener('click', async e => {
      e.stopPropagation();
      await toggleFavorite(photo);
    });
  }

  // 图片主体点击
  div.addEventListener('click', () => {
    if (opts.trashMode) {
      openLightbox(listRef, listRef.indexOf(photo));
      return;
    }
    if (state.selected.size > 0) {
      toggleSelect(photo.id, div);
    } else {
      openLightbox(listRef, listRef.indexOf(photo));
    }
  });

  // PC 右键菜单
  div.addEventListener('contextmenu', e => {
    e.preventDefault();
    if (opts.trashMode) showTrashContextMenu(e.clientX, e.clientY, photo);
    else showPhotoContextMenu(e.clientX, e.clientY, photo, div, listRef);
  });

  // c-3: 长按触发操作菜单（移动端）
  addLongPress(div, e => {
    const touch = e.changedTouches[0];
    if (opts.trashMode) showTrashContextMenu(touch.clientX, touch.clientY, photo);
    else showPhotoContextMenu(touch.clientX, touch.clientY, photo, div, listRef);
  });

  if (state.selected.has(photo.id)) div.classList.add('selected');
  return div;
}

function findPhotoThumb(id) {
  return document.querySelector(`.photo-thumb[data-id="${id}"]`);
}

function photoContextMenuItems(photo, thumbEl, listRef) {
  const isSelected = state.selected.has(photo.id);
  const isShared = !!state.shareMap[`photo:${photo.id}`];
  const favoriteLabel = photo.is_favorite ? '取消收藏' : '加入个人收藏';
  const items = [
    { label: isSelected ? '取消选择' : '选择（点击勾选图标可快速选择）', action: () => toggleSelect(photo.id, thumbEl) },
    { label: '查看', action: () => openLightbox(listRef, listRef.indexOf(photo)) },
    { label: favoriteLabel, action: () => toggleFavorite(photo) },
    { label: '在文件管理器中打开', action: () => revealInFinder(photo.id) },
    { label: '下载', action: () => triggerDownload(`/api/media/${photo.id}/download`) },
  ];
  if (state.view === 'random-album') {
    items.push({ label: '在时间线中查看', action: () => openInTimeline(photo.id) });
  }
  items.push(
    '-',
    { label: '添加到相册…', action: () => openAlbumPickerModal([photo.id]) },
    { label: isShared ? '管理分享…' : '分享…', action: () => isShared ? openShareListModal('photo', photo.id) : openShareModal('photo', photo.id) },
    '-',
    { label: '删除', danger: true, action: () => deleteSinglePhoto(photo.id) },
  );
  return items;
}

// 时间线图片右键菜单
function showPhotoContextMenu(x, y, photo, thumbEl, listRef) {
  showContextMenu(x, y, photoContextMenuItems(photo, thumbEl, listRef));
}

// 回收站图片右键菜单 (b-4)
function showTrashContextMenu(x, y, photo) {
  showContextMenu(x, y, [
    { label: '在文件管理器中打开', action: () => revealInFinder(photo.id) },
    '-',
    { label: '恢复到时间线', action: () => restorePhoto(photo.id) },
    { label: '永久删除', danger: true, action: () => hardDeleteSinglePhoto(photo.id) },
  ]);
}

async function addSinglePhotoToAlbum(photoId) {
  openAlbumPickerModal([photoId]);
}

function updatePhotoFavoriteInCollections(photoId, favorite) {
  const collections = [
    state.photos,
    state.favoritePhotos,
    state.randomAlbumPhotos,
    state.albumPhotos,
    state.trashPhotos,
    state.lightboxPhotos,
  ];
  collections.forEach(list => {
    if (!Array.isArray(list)) return;
    list.forEach(photo => {
      if (photo && photo.id === photoId) photo.is_favorite = favorite;
    });
  });
}

function updateFavoriteButtonsInDOM(photoId, favorite) {
  document.querySelectorAll(`.photo-thumb[data-id="${photoId}"] .favorite-toggle`).forEach(button => {
    button.classList.toggle('active', favorite);
    button.setAttribute('aria-label', favorite ? '取消收藏' : '加入收藏');
  });
}

function removePhotoFromList(list, photoId) {
  if (!Array.isArray(list)) return;
  const index = list.findIndex(photo => photo && photo.id === photoId);
  if (index >= 0) list.splice(index, 1);
}

function cleanupEmptyDateGroup(group) {
  if (!group) return;
  if (group.querySelector('.photo-thumb')) return;
  group.remove();
}

function renderFavoritesEmptyStateIfNeeded() {
  const container = $('#favorite-groups');
  if (!container) return;
  if (container.querySelector('.photo-thumb')) return;
  container.innerHTML = `<div class="empty">${icons.favorite}<p>还没有加入个人收藏的媒体</p></div>`;
}

function removeFavoritePhotoFromUI(photoId) {
  removePhotoFromList(state.favoritePhotos, photoId);
  removePhotoFromList(state.lightboxPhotos, photoId);
  state.selected.delete(photoId);
  const thumb = findPhotoThumb(photoId);
  if (!thumb) {
    renderFavoritesEmptyStateIfNeeded();
    updateSelectionBar();
    return;
  }
  const group = thumb.closest('.date-group');
  thumb.remove();
  cleanupEmptyDateGroup(group);
  renderFavoritesEmptyStateIfNeeded();
  updateSelectionModeUI();
  updateSelectionBar();
}

async function setPhotoFavorite(photoId, favorite) {
  await api.put(`/api/media/${photoId}/favorite`, { favorite });
  updatePhotoFavoriteInCollections(photoId, favorite);
}

async function toggleFavorite(photo) {
  const nextFavorite = !photo.is_favorite;
  try {
    await setPhotoFavorite(photo.id, nextFavorite);
    updateFavoriteButtonsInDOM(photo.id, nextFavorite);
    if (state.view === 'favorites' && !nextFavorite) {
      closeLightbox();
      removeFavoritePhotoFromUI(photo.id);
      showToast('已取消收藏');
      return;
    }
    if ($('#lightbox').classList.contains('open')) lbRender();
    showToast(nextFavorite ? '已加入个人收藏' : '已取消收藏');
  } catch (e) {
    alert('操作失败: ' + ((e && e.error) || e));
  }
}

async function unfavoriteSelected() {
  if (!state.selected.size) return;
  const ids = [...state.selected];
  for (const id of ids) {
    try {
      await setPhotoFavorite(id, false);
    } catch (e) {
      console.error('取消收藏失败:', id, e);
    }
  }
  clearSelection();
  switchView('favorites');
}

async function deleteSinglePhoto(photoId) {
  if (!confirm('确定要将这条照片/视频移入回收站吗？')) return;
  try { await api.del(`/api/media/${photoId}`); switchView('timeline'); }
  catch(e) { alert('删除失败: ' + (e.error || e)); }
}

async function hardDeleteSinglePhoto(photoId) {
  if (!confirm('确定要永久删除这条照片/视频吗？此操作不可恢复。')) return;
  try { await api.del(`/api/trash/${photoId}`); switchView('trash'); }
  catch(e) { alert('删除失败: ' + (e.error || e)); }
}

// ── 选择 ─────────────────────────────────────────────
function toggleSelect(id, thumbEl) {
  const target = thumbEl || findPhotoThumb(id);
  if (state.selected.has(id)) {
    state.selected.delete(id);
    if (target) target.classList.remove('selected');
  }
  else {
    state.selected.add(id);
    if (target) target.classList.add('selected');
  }
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
}
function clearSelection() {
  state.selected.clear();
  $$('.photo-thumb.selected').forEach(t => t.classList.remove('selected'));
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
}
function updateSelectionModeUI() {
  document.body.classList.toggle('selection-mode', state.selected.size > 0);
}
function updateSelectionBar() {
  const bar = $('#sel-bar');
  if (!bar) return;
  bar.classList.toggle('visible', state.selected.size > 0);
  const cnt = $('#sel-count');
  if (cnt) cnt.textContent = state.selected.size;
}
function selectAllInGroup(groupEl) {
  const thumbs = $$('.photo-thumb', groupEl);
  thumbs.forEach(thumb => {
    const id = Number(thumb.dataset.id);
    if (!state.selected.has(id)) {
      state.selected.add(id);
      thumb.classList.add('selected');
    }
  });
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
}
async function deleteSelected() {
  if (!state.selected.size) return;
  if (!confirm(`确定要删除选中的 ${state.selected.size} 条照片/视频吗？`)) return;
  for (const id of state.selected) {
    try { await api.del(`/api/media/${id}`); } catch (e) { console.error(e); }
  }
  clearSelection();
  switchView('timeline');
}

function openInTimeline(photoId) {
  state.pendingTimelinePhotoID = photoId;
  closeLightbox();
  switchView('timeline');
}

async function focusPendingTimelinePhoto() {
  if (!state.pendingTimelinePhotoID) return false;
  const photoId = state.pendingTimelinePhotoID;
  setTimelineJumpStatus('正在时间线中定位媒体，可能需要继续加载…');
  showBlockingProgress('正在时间线中定位媒体', '正在检查已加载内容…');
  let thumb = findPhotoThumb(photoId);
  while (!thumb && state.timelineHasMore && !state.timelineLoading) {
    setTimelineJumpStatus(`正在时间线中定位媒体… 已加载 ${state.photos.length} 条`);
    updateBlockingProgress(`已加载 ${state.photos.length} 条媒体，继续向下查找…`);
    await loadMoreTimeline();
    thumb = findPhotoThumb(photoId);
  }
  state.pendingTimelinePhotoID = null;
  if (!thumb) {
    setTimelineJumpStatus('');
    hideBlockingProgress();
    showToast('目标媒体暂未在当前时间线中找到');
    return false;
  }
  thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
  setTimelineJumpStatus('');
  hideBlockingProgress();
  return true;
}

// ── 相册列表 ──────────────────────────────────────────
async function renderAlbums() {
  $('#topbar-title').textContent = '相册';
  $('#topbar-meta').innerHTML = '';
  $('#topbar-actions').innerHTML = `<button class="btn btn-primary btn-sm" id="new-album-btn">${icons.plus} 新建相册</button>`;
  $('#new-album-btn').addEventListener('click', openCreateAlbumModal);

  $('#content').innerHTML = `<div id="album-grid-wrap"></div>`;
  try {
	state.albums = await api.get('/api/media/albums');
    renderAlbumGrid();
    restoreViewScroll('albums');
  } catch(e) { $('#content').innerHTML = `<p style="color:var(--danger)">加载失败</p>`; }
}

function renderAlbumGrid() {
  const wrap = $('#album-grid-wrap');
  if (!wrap) return;
  if (!state.albums || !state.albums.length) {
    wrap.innerHTML = `<div class="empty">${icons.album}<p>还没有相册，点击右上角新建</p></div>`;
    return;
  }
  const grid = el('div', 'album-grid');
  state.albums.forEach(a => grid.appendChild(makeAlbumCard(a)));
  wrap.innerHTML = '';
  wrap.appendChild(grid);
}

function makeAlbumCard(album) {
  const card = el('div', 'album-card');
  // c-1: 用 cover_uuid 显示封面缩略图
  const coverHtml = album.cover_uuid
    ? `<img loading="lazy" src="/media/thumbnails/${album.cover_uuid}" alt="${album.name}" onerror="this.onerror=null;this.src='${videoPosterPlaceholder}'">`
    : `<div class="album-cover-empty">${icons.photo}</div>`;
  card.innerHTML = `
<div class="album-cover">${coverHtml}</div>
<div class="album-info">
  <div class="album-name">${album.name}</div>
  <div class="album-count">${album.photo_count || 0} 条照片/视频</div>
</div>`;
  card.addEventListener('click', () => openAlbumDetail(album));
  return card;
}

// ── 相册详情 ──────────────────────────────────────────
async function openAlbumDetail(album) {
  saveViewScroll();
  state.currentAlbum = album;
  state.currentAlbumID = album.id;
  state.albumPhotos = [];
  state.albumCursor = '';
  state.albumHasMore = true;
  state.view = 'album-detail';
  setHashView('album-detail', album.id);
  $$('.nav-item[data-view]').forEach(a => a.classList.toggle('active', a.dataset.view === 'albums'));
  renderAlbumDetail();
}
async function renderAlbumDetail() {
  let album = state.currentAlbum;
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
  $('#topbar-title').textContent = album.name;
  $('#topbar-meta').innerHTML = renderGridScaleControl();
  $('#topbar-actions').innerHTML = `<button class="btn btn-sm" id="download-album-btn">下载相册</button><button class="btn btn-danger btn-sm" id="delete-album-btn">删除相册</button><button class="btn btn-sm" id="back-albums-btn">← 返回相册</button>`;
  bindGridScaleControl();
  $('#download-album-btn').addEventListener('click', () => {
    withButtonBusy($('#download-album-btn'), '打包中…', async () => {
		triggerDownload(`/api/media/albums/${album.id}/download`);
      await new Promise(resolve => setTimeout(resolve, 600));
    });
  });
  $('#delete-album-btn').addEventListener('click', async () => {
    if (!confirm(`确定要删除相册「${album.name}」吗？照片/视频本身不会被删除。`)) return;
    try {
		await api.del(`/api/media/albums/${album.id}`);
      state.currentAlbum = null;
      switchView('albums');
    } catch (e) {
      alert('删除相册失败: ' + (e.error || e));
    }
  });
  $('#back-albums-btn').addEventListener('click', () => switchView('albums'));

  $('#content').innerHTML = `
<div class="toolbar">
  <span id="sel-bar" class="selected-bar">
    <span class="selected-count" id="sel-count">0</span> 条已选
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="download-sel-btn">下载选中</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="add-to-album-btn">${icons.album} 添加到相册</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="delete-sel-btn">${icons.trash} 删除</button>
    <button class="btn-icon" style="color:#fff" id="clear-sel-btn">${icons.close}</button>
  </span>
</div>
<div id="album-groups"></div><div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;
  $('#clear-sel-btn').addEventListener('click', clearSelection);
  $('#download-sel-btn').addEventListener('click', downloadSelected);
  $('#delete-sel-btn').addEventListener('click', deleteSelected);
  $('#add-to-album-btn').addEventListener('click', () => openAlbumPickerModal(null));
  state.albumPhotos = []; state.albumCursor = ''; state.albumHasMore = true;
  await loadMoreAlbumPhotos();
  observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
  restoreViewScroll('album-detail', album.id);
}
async function loadMoreAlbumPhotos() {
  if (state.albumLoading || !state.albumHasMore || !state.currentAlbum) return;
  state.albumLoading = true;
	try {
		const id = state.currentAlbum.id;
		const url = `/api/media/albums/${id}` + (state.albumCursor ? `?cursor=${encodeURIComponent(state.albumCursor)}` : '');
		const page = await api.get(url);
    state.albumPhotos.push(...(page.photos || []));
    state.albumCursor = page.next_cursor || '';
    state.albumHasMore = page.has_more || false;
    renderAlbumGroups(page.photos || []);
  } catch(e) { console.error(e); }
  finally {
    state.albumLoading = false;
    updateLoadMoreUI('load-more', state.albumHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
  }
}
function renderAlbumGroups(newPhotos) {
  const container = $('#album-groups');
  if (!container) return;
  if (state.albumPhotos.length === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>相册里还没有照片/视频</p></div>`;
    return;
  }
  const groups = groupByDate(newPhotos);
  for (const [date, photos] of Object.entries(groups)) {
    let group = container.querySelector(`[data-date="${CSS.escape(date)}"]`);
    if (!group) {
      group = el('div', 'date-group');
      group.dataset.date = date;
      group.innerHTML = `<div class="date-label"><span class="date-text">${date}</span><button class="date-select-all" type="button">全选</button></div><div class="photo-grid"></div>`;
      group.querySelector('.date-select-all').addEventListener('click', () => selectAllInGroup(group));
      container.appendChild(group);
    }
    const grid = group.querySelector('.photo-grid');
    photos.forEach(p => grid.appendChild(makePhotoThumb(p, state.albumPhotos)));
  }
}

// ── 回收站 (b-4 修复) ─────────────────────────────────
async function renderTrash() {
  $('#topbar-title').textContent = '回收站';
  $('#topbar-meta').innerHTML = renderGridScaleControl();
  $('#topbar-actions').innerHTML = `<button class="btn btn-danger btn-sm" id="empty-trash-btn">${icons.trash} 清空回收站</button>`;
  bindGridScaleControl();
  $('#empty-trash-btn').addEventListener('click', emptyTrash);

  // c-5: 加入批量恢复工具栏
  $('#content').innerHTML = `
<div class="toolbar">
  <span id="trash-sel-bar" class="selected-bar">
    <span class="selected-count" id="trash-sel-count">0</span> 条已选
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="restore-sel-btn">${icons.prev} 批量恢复</button>
    <button class="btn btn-sm" style="background:rgba(255,255,255,.2);border-color:transparent;color:#fff" id="hard-delete-sel-btn">${icons.trash} 批量删除</button>
    <button class="btn-icon" style="color:#fff" id="trash-clear-sel-btn">${icons.close}</button>
  </span>
</div>
<p style="font-size:.82rem;color:var(--text2);margin-bottom:8px">左键预览，右键或长按恢复/删除，点击勾选图标批量操作</p>
<div id="trash-groups"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;

  $('#trash-clear-sel-btn').addEventListener('click', () => { clearSelection(); updateTrashSelBar(); });
  $('#restore-sel-btn').addEventListener('click', restoreSelected);
  $('#hard-delete-sel-btn').addEventListener('click', hardDeleteSelected);

  state.trashPhotos = []; state.trashCursor = ''; state.trashHasMore = true;
  await loadMoreTrash();
  observeLoadMore('load-more', loadMoreTrash, () => state.trashHasMore && !state.trashLoading);
  restoreViewScroll('trash');
}
async function loadMoreTrash() {
  if (state.trashLoading || !state.trashHasMore) return;
  state.trashLoading = true;
  try {
		const url = '/api/media/trash' + (state.trashCursor ? `?cursor=${encodeURIComponent(state.trashCursor)}` : '');
    const page = await api.get(url);
    state.trashPhotos.push(...(page.photos || []));
    state.trashCursor = page.next_cursor || '';
    state.trashHasMore = page.has_more || false;
    renderTrashGroups(page.photos || []);
  } catch(e) { console.error(e); }
  finally {
    state.trashLoading = false;
    updateLoadMoreUI('load-more', state.trashHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreTrash, () => state.trashHasMore && !state.trashLoading);
  }
}
function renderTrashGroups(newPhotos) {
  const container = $('#trash-groups');
  if (!container) return;
  if (state.trashPhotos.length === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.trash}<p>回收站是空的</p></div>`;
    return;
  }
  const groups = groupByDate(newPhotos);
  for (const [date, photos] of Object.entries(groups)) {
    let group = container.querySelector(`[data-date="${CSS.escape(date)}"]`);
    if (!group) {
      group = el('div', 'date-group');
      group.dataset.date = date;
      group.innerHTML = `<div class="date-label"><span class="date-text">${date}</span><button class="date-select-all" type="button">全选</button></div><div class="photo-grid"></div>`;
      group.querySelector('.date-select-all').addEventListener('click', () => selectAllInGroup(group));
      container.appendChild(group);
    }
    const grid = group.querySelector('.photo-grid');
    // b-4 + c-5: trashMode 支持勾选批量恢复
    photos.forEach(p => {
      const thumb = makePhotoThumb(p, state.trashPhotos, { trashMode: true });
      // 覆盖 check 的点击，同时更新回收站选择栏
      const ck = thumb.querySelector('.check');
      ck.addEventListener('click', e => {
        e.stopPropagation();
        toggleSelect(p.id, thumb);
        updateTrashSelBar();
      }, { capture: true });
      grid.appendChild(thumb);
    });
  }
}
async function emptyTrash() {
  if (!confirm('确定要永久删除回收站中所有照片/视频吗？此操作不可恢复。')) return;
  try { await api.del('/api/media/trash'); switchView('trash'); }
  catch(e) { alert('操作失败: ' + (e.error || e)); }
}
async function restorePhoto(id) {
  try { await api.post(`/api/media/${id}/restore`, {}); switchView('trash'); }
  catch(e) { alert('恢复失败: ' + (e.error || e)); }
}

// c-5: 更新回收站批量操作栏
function updateTrashSelBar() {
  const bar = $('#trash-sel-bar');
  if (!bar) return;
  bar.classList.toggle('visible', state.selected.size > 0);
  const cnt = $('#trash-sel-count');
  if (cnt) cnt.textContent = state.selected.size;
  updateSelectionModeUI();
}

// c-5: 批量恢复选中图片
async function restoreSelected() {
  if (!state.selected.size) return;
  if (!confirm(`确定要恢复选中的 ${state.selected.size} 条照片/视频吗？`)) return;
  const ids = [...state.selected];
  clearSelection();
  for (const id of ids) {
		try { await api.post(`/api/media/${id}/restore`, {}); }
    catch(e) { console.error('恢复失败:', id, e); }
  }
  switchView('trash');
}

async function hardDeleteSelected() {
  if (!state.selected.size) return;
  if (!confirm(`确定要永久删除选中的 ${state.selected.size} 条照片/视频吗？此操作不可恢复。`)) return;
  const ids = [...state.selected];
  clearSelection();
  for (const id of ids) {
		try { await api.del(`/api/media/trash/${id}`); }
    catch(e) { console.error('永久删除失败:', id, e); }
  }
  switchView('trash');
}

// ── 无限滚动 ──────────────────────────────────────────
function observeLoadMore(id, loadFn, canLoad) {
  disconnectLoadMoreObserver();
  const sentinel = document.getElementById(id);
  if (!sentinel) return;
  state.loadMoreObserver = new IntersectionObserver(entries => {
    if (entries[0].isIntersecting && canLoad()) loadFn();
  }, { rootMargin: '200px' });
  state.loadMoreObserver.observe(sentinel);
}
function updateLoadMoreUI(id, hasMore) {
  const e = document.getElementById(id);
  if (!e) return;
  e.style.display = hasMore ? '' : 'none';
}

// 当首屏内容不足、底部哨兵一直在可视区时，IntersectionObserver 可能只触发一次，
// 这里在每次加载结束后主动检查一次，确保能继续把后续页面补出来。
function maybeLoadMoreImmediately(id, loadFn, canLoad) {
  const sentinel = document.getElementById(id);
  if (!sentinel || !canLoad()) return;
  const rect = sentinel.getBoundingClientRect();
  if (rect.top <= window.innerHeight + 200) {
    requestAnimationFrame(() => {
      if (canLoad()) loadFn();
    });
  }
}

// ── 灯箱 ──────────────────────────────────────────────
function renderLightbox() {
  return `<div class="lightbox" id="lightbox">
  <div class="lightbox-header">
    <span class="lb-title" id="lb-title"></span>
    <label class="lightbox-zoom-wrap" for="lb-zoom">
      <span id="lb-zoom-value">100%</span>
      <input class="lightbox-slider" id="lb-zoom" type="range" min="50" max="300" step="10" aria-label="媒体缩放">
    </label>
    <button class="btn-icon lightbox-fit-height" style="color:#ccc" id="lb-fit-height" title="适应高度（Alt + 0）">适应高度</button>
    <div class="lightbox-slideshow-controls">
      <button class="btn-icon lightbox-play-btn" style="color:#ccc" id="lb-slideshow-toggle"></button>
      <select class="lightbox-select" id="lb-slideshow-mode" aria-label="播放模式">
        <option value="sequential">顺序</option>
        <option value="random">随机</option>
      </select>
      <label class="lightbox-slider-wrap" for="lb-slideshow-interval">
        <span id="lb-slideshow-interval-value">5 秒</span>
        <input class="lightbox-slider" id="lb-slideshow-interval" type="range" min="1" max="30" step="1" aria-label="播放间隔">
      </label>
      <label class="lightbox-loop-toggle" for="lb-slideshow-loop">
        <input type="checkbox" id="lb-slideshow-loop">
        <span>循环</span>
      </label>
    </div>
    <button class="btn-icon" style="color:#ccc" id="lb-download">下载</button>
    <button class="btn-icon" style="color:#ccc" id="lb-favorite">${icons.favorite}</button>
    <button class="btn-icon" style="color:#ccc" id="lb-share">${icons.share}</button>
    <button class="btn-icon" style="color:#ccc" id="lb-close">${icons.prev} 返回</button>
  </div>
  <div class="lightbox-body">
    <div class="lightbox-media-frame" id="lb-frame">
      <img class="lightbox-img" id="lb-img" src="" alt="">
      <video class="lightbox-video hidden" id="lb-video" controls playsinline preload="metadata"></video>
    </div>
    <button class="lb-nav lb-prev" id="lb-prev">${icons.prev}</button>
    <button class="lb-nav lb-next" id="lb-next">${icons.next}</button>
  </div>
  <div class="lightbox-info" id="lb-info"></div>
</div>`;
}
function lockPageScrollForLightbox() {
  if (document.body.classList.contains('lightbox-page-lock')) return;
  state.lightboxPageScrollTop = window.scrollY || window.pageYOffset || 0;
  document.body.classList.add('lightbox-page-lock');
  document.body.style.top = `-${state.lightboxPageScrollTop}px`;
}
function unlockPageScrollForLightbox() {
  if (!document.body.classList.contains('lightbox-page-lock')) return;
  const top = Number(state.lightboxPageScrollTop) || 0;
  document.body.classList.remove('lightbox-page-lock');
  document.body.style.top = '';
  window.scrollTo(0, top);
  state.lightboxPageScrollTop = 0;
}
function bindGlobal() {
  document.addEventListener('click', () => closeContextMenu());
  window.addEventListener('scroll', () => saveViewScroll(), { passive: true });
  document.addEventListener('keydown', e => {
    if (handleViewNumberShortcut(e)) return;
    if (e.key === 'Escape') closeContextMenu();
    if (!$('#lightbox').classList.contains('open')) return;
    handleLightboxKeydown(e);
  }, true);
  document.addEventListener('fullscreenchange', () => {
    const lightbox = $('#lightbox');
    if (!lightbox || !lightbox.classList.contains('open')) return;
    if (!document.fullscreenElement) closeLightbox();
  });
  document.addEventListener('click', e => {
    if (e.target.closest('#lb-close')) exitLightboxToContext();
    if (e.target.closest('#lb-prev'))  lbNav(-1);
    if (e.target.closest('#lb-next'))  lbNav(1);
    if (e.target.closest('#lb-download')) downloadCurrentPhoto();
    if (e.target.closest('#lb-favorite')) toggleCurrentLightboxFavorite();
    if (e.target.closest('#lb-share')) lbShare();
    if (e.target.closest('#lb-fit-height')) setLightboxFitHeight();
    if (e.target.closest('#lb-slideshow-toggle')) toggleSlideshow();
  });
  document.addEventListener('contextmenu', e => {
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.target.closest('.lb-nav')) return;
    if (!e.target.closest('#lb-img, #lb-video, .lightbox-body')) return;
    e.preventDefault();
    const photo = state.lightboxPhotos[state.lightboxIndex];
    if (!photo) return;
    showPhotoContextMenu(e.clientX, e.clientY, photo, findPhotoThumb(photo.id), state.lightboxPhotos);
  });
  document.addEventListener('change', e => {
    if (e.target.matches('#lb-slideshow-mode')) updateSlideshowSetting('mode', e.target.value);
    if (e.target.matches('#lb-slideshow-interval')) updateSlideshowSetting('interval', parseInt(e.target.value, 10) * 1000);
    if (e.target.matches('#lb-slideshow-loop')) updateSlideshowSetting('loop', e.target.checked);
    if (e.target.matches('#lb-zoom')) setLightboxZoom(parseInt(e.target.value, 10));
  });
}
async function openLightbox(photos, index) {
  const target = photos[Math.max(0, index)];
  if (target && isVideoMedia(target)) {
    await ensureSettingsDataLoaded();
    if (state.serverSettings.use_system_player) {
      openWithSystemPlayer(target.id)
        .then(() => showToast('已使用系统播放器打开'))
        .catch(e => alert('打开失败: ' + ((e && e.error) || e)));
      return;
    }
  }
  state.lightboxPhotos = photos;
  state.lightboxIndex  = Math.max(0, index);
  state.lightboxReturnView = state.view;
  state.lightboxReturnAlbumID = state.currentAlbumID;
  state.slideshowRandomQueue = [];
  applyLightboxZoom();
  lockPageScrollForLightbox();
  $('#lightbox').classList.add('open');
  lbRender();
}
function closeLightbox() {
  stopSlideshow();
  state.lightboxPlaybackToken += 1;
  resetLightboxPrefetchCache();
  const video = $('#lb-video');
  if (video) {
    video.pause();
    video.removeAttribute('src');
    video.load();
  }
  $('#lightbox').classList.remove('open');
  unlockPageScrollForLightbox();
  state.lightboxReturnView = '';
  state.lightboxReturnAlbumID = null;
}
function exitLightboxToContext() {
  closeLightbox();
}
function lbNav(dir) {
  const n = state.lightboxIndex + dir;
  if (n < 0 || n >= state.lightboxPhotos.length) return;
  lbGoTo(n);
}
function lbGoTo(index, options = {}) {
  if (index < 0 || index >= state.lightboxPhotos.length) return;
  state.lightboxPlaybackToken += 1;
  const video = $('#lb-video');
  if (video) {
    video.pause();
    video.removeAttribute('src');
    video.load();
  }
  state.lightboxIndex = index;
  if (state.slideshowMode === 'random' && !options.fromSlideshow) resetRandomQueue();
  lbRender();
}
function lbRender() {
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  const img = $('#lb-img');
  const video = $('#lb-video');
  if (isVideoMedia(p)) {
    img.classList.add('hidden');
    img.removeAttribute('src');
    video.classList.remove('hidden');
    video.preload = 'auto';
    video.src = mediaFileURL(p);
    video.poster = mediaThumbURL(p);
    video.load();
    if (state.slideshowPlaying || state.experimentalAutoplayVideo) autoplayLightboxVideo(video);
  } else {
    state.lightboxPlaybackToken += 1;
    video.pause();
    video.classList.add('hidden');
    video.preload = 'metadata';
    video.removeAttribute('src');
    video.load();
    img.classList.remove('hidden');
    img.src = mediaFileURL(p);
  }
  $('#lb-title').textContent = p.original_name;
  $('#lb-favorite').innerHTML = `${icons.favorite} ${p.is_favorite ? '取消收藏' : '收藏'}`;
  $('#lb-prev').classList.toggle('hidden', state.lightboxIndex === 0);
  $('#lb-next').classList.toggle('hidden', state.lightboxIndex === state.lightboxPhotos.length - 1);
  applyLightboxZoom();
  updateSlideshowControls();
  if (state.slideshowPlaying) scheduleSlideshowStep();
  if (state.experimentalPrefetchNeighbors) prefetchAdjacentMedia();
  const items = [
    ['类型', isVideoMedia(p) ? '视频' : '图片'],
    ['MIME', p.mime_type || '—'],
    ['扩展名', fileExtension(p.original_name)],
    ['拍摄时间', formatDateTime(p.taken_at)],
    ['上传时间', formatDateTime(p.uploaded_at)],
    ['尺寸', p.width && p.height ? `${p.width} × ${p.height}` : '—'],
    ['宽高比', aspectRatioLabel(p.width, p.height)],
    ['时长', isVideoMedia(p) && p.duration_ms ? formatDuration(p.duration_ms) : '—'],
    ['文件大小', formatSizeExact(p.size)],
    ['文件名', p.original_name],
    ['媒体 ID', String(p.id || '—')],
    ['UUID', p.uuid || '—'],
  ];
  $('#lb-info').innerHTML = items.map(([k, v]) => `<div class="lb-info-item" title="${escapeHTML(`${k}: ${v}`)}"><span class="lb-info-key">${escapeHTML(k)}</span><span class="lb-info-value">${escapeHTML(v)}</span></div>`).join('');
}

async function toggleCurrentLightboxFavorite() {
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  await toggleFavorite(p);
}
function prefetchAdjacentMedia() {
  const neighbors = [state.lightboxIndex - 1, state.lightboxIndex + 1]
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
async function lbShare() {
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  closeLightbox();
  const isShared = !!state.shareMap[`photo:${p.id}`];
  if (isShared) openShareListModal('photo', p.id);
  else openShareModal('photo', p.id);
}

function triggerDownload(url) {
	const a = document.createElement('a');
	a.href = url;
	a.style.display = 'none';
	document.body.appendChild(a);
	a.click();
	a.remove();
}

async function withButtonBusy(button, busyText, fn) {
	if (!button) return fn();
	const prevHTML = button.innerHTML;
	const prevDisabled = button.disabled;
	button.disabled = true;
	button.innerHTML = busyText;
	try {
		return await fn();
	} finally {
		button.disabled = prevDisabled;
		button.innerHTML = prevHTML;
	}
}

async function triggerPostDownload(url, payload, fallbackName) {
	const res = await fetch(url, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	});
	if (!res.ok) {
		throw await res.json();
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

function downloadCurrentPhoto() {
	const p = state.lightboxPhotos[state.lightboxIndex];
	if (!p) return;
	withButtonBusy($('#lb-download'), '下载中…', async () => {
		triggerDownload(`/api/media/${p.id}/download`);
		await new Promise(resolve => setTimeout(resolve, 600));
	});
}

async function downloadSelected() {
	if (!state.selected.size) return;
	const btn = $('#download-sel-btn');
	try {
		await withButtonBusy(btn, '打包中…', async () => {
			await triggerPostDownload('/api/media/download', {
				media_ids: [...state.selected],
			}, `echogallery-selection-${Date.now()}.zip`);
		});
	} catch (e) {
		alert('下载失败: ' + (e.error || e));
	}
}

// ── 上传模态框 ────────────────────────────────────────
function renderUploadModal() {
  return `<div class="modal-overlay" id="upload-modal">
  <div class="modal" style="width:520px">
    <div class="modal-title">${icons.upload} 上传媒体</div>
    <div class="upload-zone" id="drop-zone">
      ${icons.upload}
      <div style="margin-top:8px">拖拽图片或视频到这里，或点击选择文件</div>
      <div style="font-size:.8rem;margin-top:4px">支持 JPG、PNG、GIF、WebP、MP4、MOV、M4V、WebM</div>
      <input type="file" id="file-input" accept="image/*,.mp4,.mov,.m4v,.webm,video/*" multiple aria-hidden="true">
    </div>
    <div class="upload-queue" id="upload-queue"></div>
    <div class="modal-footer">
      <button class="btn" id="retry-failed-btn" style="display:none">重传失败项</button>
      <button class="btn" id="upload-close-btn">关闭</button>
    </div>
  </div>
</div>`;
}
let _uploadZoneBound = false;
function openUploadModal() {
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
    if (f.type.startsWith('image/')) return true;
    const name = (f.name || '').toLowerCase();
    return f.type.startsWith('video/') || ['.mp4', '.mov', '.m4v', '.webm'].some(ext => name.endsWith(ext));
  });
  if (!files.length) return;
  const queue = $('#upload-queue');
  for (const file of files) {
    const id = uid();
    const job = { id, file, status: 'queued' };
    state.uploadJobs.push(job);
    const row = el('div', 'upload-item');
    row.id = `upload-row-${id}`;
    row.innerHTML = `<span class="up-name">${file.name}</span><div style="flex:1"><div class="progress-bar"><div class="progress-fill" style="width:0%" id="prog-${id}"></div></div></div><span class="up-status" id="stat-${id}">等待中</span><button class="btn btn-sm" id="retry-${id}" style="display:none">重传</button>`;
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
  if (stat) { stat.textContent = '上传中'; stat.className = 'up-status'; }
  const fd = new FormData();
  const lowerName = (file.name || '').toLowerCase();
  const isVideo = file.type.startsWith('video/') || ['.mp4', '.mov', '.m4v', '.webm'].some(ext => lowerName.endsWith(ext));
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
      xhr.onerror = () => reject({ error: '网络错误' });
      xhr.send(fd);
    });
    if (prog) prog.style.width = '100%';
    if (job) job.status = 'done';
    if (stat) { stat.textContent = '完成'; stat.className = 'up-status done'; }
  } catch(e) {
    if (job) job.status = 'failed';
    if (stat) { stat.textContent = e.error || '失败'; stat.className = 'up-status error'; }
    if (retryBtn) retryBtn.style.display = '';
  }
  updateRetryFailedButton();
}

function updateRetryFailedButton() {
  const btn = $('#retry-failed-btn');
  if (!btn) return;
  const failedCount = state.uploadJobs.filter(j => j.status === 'failed').length;
  btn.style.display = failedCount > 0 ? '' : 'none';
  btn.textContent = failedCount > 0 ? `重传失败项（${failedCount}）` : '重传失败项';
}

function retrySingleUpload(id) {
  const job = state.uploadJobs.find(j => j.id === id);
  if (!job) return;
  job.status = 'queued';
  const prog = $(`#prog-${id}`);
  const stat = $(`#stat-${id}`);
  const retryBtn = $(`#retry-${id}`);
  if (prog) prog.style.width = '0%';
  if (stat) { stat.textContent = '等待重传'; stat.className = 'up-status'; }
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
      if (stat) { stat.textContent = '等待重传'; stat.className = 'up-status'; }
      if (retryBtn) retryBtn.style.display = 'none';
    }
  });
  if (found) runUploadQueue();
}

// ── 新建相册模态框 ────────────────────────────────────
function renderCreateAlbumModal() {
  return `<div class="modal-overlay" id="create-album-modal">
  <div class="modal">
    <div class="modal-title">新建相册</div>
    <div class="form-group"><label class="form-label">相册名称</label><input class="input" id="album-name-input" placeholder="输入相册名称" maxlength="50"></div>
    <div class="form-group"><label class="form-label">描述（可选）</label><input class="input" id="album-desc-input" placeholder="输入描述"></div>
    <div class="modal-footer">
      <button class="btn" id="cancel-album-btn">取消</button>
      <button class="btn btn-primary" id="confirm-album-btn">创建</button>
    </div>
  </div>
</div>`;
}
function openCreateAlbumModal() {
  $('#create-album-modal').classList.add('open');
  $('#album-name-input').value = '';
  $('#album-desc-input').value = '';
  $('#cancel-album-btn').onclick = () => $('#create-album-modal').classList.remove('open');
  $('#confirm-album-btn').onclick = createAlbum;
}
async function createAlbum() {
  const name = $('#album-name-input').value.trim();
  if (!name) { alert('请输入相册名称'); return; }
  try {
		await api.post('/api/media/albums', { name, description: $('#album-desc-input').value.trim() });
    $('#create-album-modal').classList.remove('open');
    // 通过 switchView 而不是直接 render，确保菜单高亮和 hash 保持一致。
    switchView('albums');
  } catch(e) { alert('创建失败: ' + (e.error || e)); }
}

// ── 相册选择弹窗 (b-3) ───────────────────────────────
function renderAlbumPickerModal() {
  return `<div class="modal-overlay" id="album-picker-modal">
  <div class="modal" style="width:480px">
    <div class="modal-title">${icons.album} 选择相册</div>
    <div class="album-picker-grid" id="album-picker-grid"></div>
    <div style="font-size:.8rem;color:var(--text2);margin-top:10px" id="album-picker-hint"></div>
    <div class="modal-footer">
      <button class="btn" id="album-picker-cancel">取消</button>
      <button class="btn btn-primary" id="album-picker-confirm">添加</button>
    </div>
  </div>
</div>`;
}

let _pickerPhotoIds = null;
let _pickerSelected = null;

async function openAlbumPickerModal(photoIds) {
	// photoIds: null=用已选集合, 数组=指定媒体
  _pickerPhotoIds = photoIds;
  _pickerSelected = null;
  const modal = $('#album-picker-modal');
  const cancelBtn = $('#album-picker-cancel');
  const confirmBtn = $('#album-picker-confirm');
  modal.classList.add('open');
  $('#album-picker-hint').textContent = '';

  // 无论后续加载结果如何，都先绑定好按钮，避免“无相册时只能刷新”的死状态。
  cancelBtn.disabled = false;
  cancelBtn.onclick = () => modal.classList.remove('open');
  confirmBtn.disabled = false;
  confirmBtn.textContent = '添加';
  confirmBtn.onclick = confirmAlbumPicker;

  const grid = $('#album-picker-grid');
  grid.innerHTML = '<div style="padding:16px;color:var(--text2)">加载中…</div>';

  try {
		const albums = await api.get('/api/media/albums');
    if (!albums || !albums.length) {
      grid.innerHTML = `<div style="padding:16px;color:var(--text2)">还没有相册，请先新建相册</div>`;
      $('#album-picker-hint').textContent = '你可以直接在当前弹窗里去创建相册。';
      confirmBtn.textContent = '去创建相册';
      confirmBtn.onclick = () => {
        modal.classList.remove('open');
        openCreateAlbumModal();
      };
      return;
    }
    grid.innerHTML = '';
    albums.forEach(a => {
      const item = el('div', 'album-picker-item');
      item.innerHTML = `<div class="album-picker-cover">${icons.photo}</div><div class="album-picker-name">${a.name} (${a.photo_count||0})</div>`;
      item.addEventListener('click', () => {
        $$('.album-picker-item.picked').forEach(i => i.classList.remove('picked'));
        item.classList.add('picked');
        _pickerSelected = a;
      });
      grid.appendChild(item);
    });
  } catch(e) { grid.innerHTML = `<div style="color:var(--danger)">加载失败</div>`; }

}

async function confirmAlbumPicker() {
  if (!_pickerSelected) { $('#album-picker-hint').textContent = '请先选择一个相册'; return; }
	const album = _pickerSelected;
	const ids = _pickerPhotoIds || [...state.selected];
	if (!ids.length) { $('#album-picker-hint').textContent = '没有选中的照片/视频'; return; }

	$('#album-picker-confirm').disabled = true;
	let ok = 0, fail = 0;
	for (const id of ids) {
		try { await api.post(`/api/media/albums/${album.id}`, { media_id: id }); ok++; }
		catch(e) { fail++; }
	}
  $('#album-picker-confirm').disabled = false;
  $('#album-picker-modal').classList.remove('open');
  showToast(`已添加 ${ok} 条到「${album.name}」${fail ? `，${fail} 条失败` : ''}`);
  if (_pickerPhotoIds === null) clearSelection();
}

// ── 分享模态框 ────────────────────────────────────────
function renderShareModal() {
  return `<div class="modal-overlay" id="share-modal">
  <div class="modal">
    <div class="modal-title">${icons.share} 创建分享链接</div>
    <div class="form-group">
      <label class="form-label">过期时间</label>
      <select class="input" id="share-expires">
        <option value="0">永不过期</option>
        <option value="7">7 天</option>
        <option value="30">30 天</option>
        <option value="90">90 天</option>
      </select>
    </div>
    <div id="share-result" style="margin-top:12px;display:none">
      <label class="form-label">分享链接</label>
      <input class="input" id="share-link-input" readonly style="cursor:pointer">
      <p style="font-size:.8rem;color:var(--text2);margin-top:4px">点击链接复制</p>
    </div>
    <div class="modal-footer">
      <button class="btn" id="share-cancel-btn">关闭</button>
      <button class="btn btn-primary" id="share-confirm-btn">生成链接</button>
    </div>
  </div>
</div>`;
}
let _shareTarget = null;
function openShareModal(type, targetId) {
  _shareTarget = { type, targetId };
  $('#share-modal').classList.add('open');
  $('#share-result').style.display = 'none';
  $('#share-cancel-btn').onclick = () => { $('#share-modal').classList.remove('open'); };
  $('#share-confirm-btn').onclick = generateShareLink;
}
async function generateShareLink() {
  if (!_shareTarget) return;
  const days = parseInt($('#share-expires').value);
  const body = { type: _shareTarget.type, target_id: _shareTarget.targetId };
  if (days > 0) body.expires_in_days = days;
  try {
		const link = await api.post('/api/media/shares', body);
    upsertShareLink(link);

    const url = `${location.origin}/s/${link.token}`;
    const input = $('#share-link-input');
    input.value = url;
    $('#share-result').style.display = '';
    const newInput = input.cloneNode(true);
    input.parentNode.replaceChild(newInput, input);
    newInput.addEventListener('click', () => navigator.clipboard.writeText(url).then(() => showToast('已复制分享链接')));
  } catch(e) { alert('生成失败: ' + (e.error || e)); }
}

// ── 分享列表弹窗 (b-2) ───────────────────────────────
function renderShareListModal() {
  return `<div class="modal-overlay" id="share-list-modal">
  <div class="modal" style="width:480px">
    <div class="modal-title">${icons.share} 管理分享链接</div>
    <div id="share-list-content"></div>
    <div class="modal-footer">
      <button class="btn" id="share-list-close">关闭</button>
      <button class="btn btn-primary" id="share-list-add">新建分享…</button>
    </div>
  </div>
</div>`;
}

let _shareListTarget = null;
function openShareListModal(type, targetId) {
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
    wrap.innerHTML = `<p style="color:var(--text2);padding:12px 0">暂无分享链接</p>`;
    return;
  }
  wrap.innerHTML = '';
  links.forEach(l => {
    const url = `${location.origin}/s/${l.token}`;
    const exp = l.expires_at ? `过期：${formatDate(l.expires_at)}` : '永不过期';
    const row = el('div');
    row.style.cssText = 'display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--border);font-size:.85rem;';
    row.innerHTML = `
      <div style="flex:1;overflow:hidden">
        <div style="font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${url}</div>
        <div style="color:var(--text2);font-size:.75rem;margin-top:2px">${exp}</div>
      </div>
      <button class="btn btn-sm" data-copy="${url}">复制</button>
      <button class="btn btn-sm btn-danger" data-del="${l.id}">删除</button>`;
    row.querySelector('[data-copy]').addEventListener('click', e => {
      navigator.clipboard.writeText(e.target.dataset.copy).then(() => showToast('已复制分享链接'));
    });
    row.querySelector('[data-del]').addEventListener('click', async e => {
      const id = parseInt(e.target.dataset.del);
      try {
		await api.del(`/api/media/shares/${id}`);
        removeShareLinkFromState(id);
        renderShareList();
        showToast('分享链接已删除');
      } catch(ex) { alert('删除失败'); }
    });
    wrap.appendChild(row);
  });
}

// ── 登出 ──────────────────────────────────────────────
async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
  location.reload();
}

// ── 启动 ──────────────────────────────────────────────
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
  if (['timeline','favorites','random-album','albums','trash','settings'].includes(h)) {
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
    if (['timeline','favorites','random-album','albums','trash','settings'].includes(last)) {
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
  if (['timeline','favorites','random-album','albums','trash','settings'].includes(view)) {
    history.replaceState(null, '', '#' + view);
    if (state.experimentalRestoreLastView) localStorage.setItem('last_view_hash', view);
  }
}
const initialRoute = getHashView();
state.view = initialRoute.view;
state.currentAlbumID = initialRoute.albumID;
async function bootstrapApp() {
  await loadInlineSVGIcons();
  renderApp();
}
bootstrapApp();
