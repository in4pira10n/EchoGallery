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
  async get(url, options = {}) {
    const r = await fetch(url, options);
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
const supportedImageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp', '.tif', '.tiff'];
const supportedVideoExtensions = ['.mp4', '.mov', '.m4v', '.webm', '.mkv', '.avi', '.wmv', '.wma', '.ts', '.mts', '.m2ts', '.mpg', '.mpeg', '.3gp', '.3g2', '.ogv'];
const browserUnsupportedVideoExtensions = ['.wmv', '.wma'];
const memoriesStorageKey = 'echogallery_memories_v1';
const mediaKindFilterStorageKey = 'echogallery_media_kind_filter_v1';
const timelineOrderStorageKey = 'echogallery_timeline_order_v1';
const albumDetailSortStorageKey = 'echogallery_album_detail_sort_v1';
const videoBookmarksStorageKey = 'echogallery_video_bookmarks_v2';
const legacyVideoBookmarksStorageKey = 'echogallery_video_bookmarks_v1';
const globalVideoVolumeStorageKey = 'echogallery_global_video_volume_v1';
const libraryBatchWorkflowEnabledStorageKey = 'echogallery_library_batch_workflow_enabled_v1';
const libraryBatchWorkflowAggressiveStorageKey = 'echogallery_library_batch_workflow_aggressive_v1';
const libraryBatchWorkflowMoveLegacyStorageKey = 'echogallery_library_batch_workflow_move_legacy_v1';
const libraryBatchWorkflowCleanFilesStorageKey = 'echogallery_library_batch_workflow_clean_files_v1';
const libraryBatchWorkflowPhaseStorageKey = 'echogallery_library_batch_workflow_phase_v1';
const copyCatalogPath = '/static/strings/zh-CN.json';
const COPY_DEFAULTS = {
  app: {
    search: {
      ariaLabel: '全局搜索',
      placeholder: '搜索文件名、类型、UUID',
      close: '关闭',
      downloadAll: '打包下载全部',
      loadMore: '加载更多',
      emptyPrompt: '输入关键词开始搜索',
      emptyResults: '没有匹配结果',
    },
    libraryBuild: {
      title: '扫描与构建资源库',
      preparing: '正在准备资源库',
      runningDescription: 'EchoGallery 正在建立媒体索引，完成后会自动进入资源库。',
      pausedDescription: '资源库构建需要处理后才能继续。',
      runningBadge: '运行中',
      pausedBadge: '需要处理',
      discovering: '发现媒体中',
      discoveredCount: '已发现 {count} 条',
      progressLabel: '处理进度',
      percentLabel: '完成度',
      elapsedLabel: '已用时间',
      etaLabel: '预计剩余',
      etaPending: '计算中',
      prunedLabel: '已清理失效媒体',
      exitAfterComplete: '构建完成后自动退出应用',
    },
    libraryBatchScan: {
      title: '正在批量扫描资源库',
      cancel: '取消批量扫描',
      cancelling: '正在取消…',
      completed: '批量扫描已完成',
      cancelled: '已取消批量扫描资源库',
      failed: '批量扫描失败',
    },
    libraryBatchThumbnail: {
      title: '正在批量构建缩略图',
      cancel: '取消批量缩略图',
      cancelling: '正在取消…',
      completed: '批量缩略图构建已完成',
      cancelled: '已取消批量缩略图任务',
      failed: '批量缩略图构建失败',
    },
    randomAlbum: {
      blockingTitle: '正在加载全部乱序相册媒体',
      blockingBatch: '正在读取第 {batch} 批…',
      cancelOrganizing: '取消整理',
      cancelling: '已读取 {count} 条媒体，正在取消…',
      organizing: '已读取 {count} 条媒体，正在整理乱序相册…',
      collecting: '正在收集全部媒体… 已读取 {count} 条',
      finalizing: '已读取 {count} 条媒体，正在完成整理…',
      modalTitle: '立即加载全部乱序相册媒体',
      modalCopy: '会先确保乱序相册已收集完整媒体列表，再批量预加载缩略图。加载期间会暂时锁定 App，减少后续浏览时被缩略图加载打断。',
      modalReady: '准备加载…',
      modalCancel: '取消',
      modalStart: '开始加载',
      modalPrepare: '准备加载全部乱序相册媒体…',
      collectingToast: '乱序相册正在收集媒体，请稍候',
    },
    empty: {
      favorites: '还没有加入个人收藏的媒体',
      albums: '还没有相册，点击左上角新建',
      albumMedia: '相册里还没有媒体',
      trash: '回收站是空的',
    },
    upload: {
      title: '上传媒体',
      hint: '拖拽图片或视频到这里，或点击选择文件',
      support: '支持 JPG、PNG、GIF、WebP、BMP、TIFF、MP4、MOV、M4V、WebM、MKV、AVI、WMV、WMA、MPEG、TS、3GP、OGV',
      retryFailed: '重传失败项',
      close: '关闭',
      waiting: '等待中',
      uploading: '上传中',
      networkError: '网络错误',
      done: '完成',
      failed: '失败',
      retryWaiting: '等待重传',
    },
    createAlbum: {
      title: '新建相册',
      nameLabel: '相册名称',
      namePlaceholder: '输入相册名称',
      descLabel: '描述（可选）',
      descPlaceholder: '输入描述',
      cancel: '取消',
      confirm: '创建',
      emptyNameAlert: '请输入相册名称',
    },
    share: {
      title: '创建分享链接',
      expireLabel: '过期时间',
      never: '永不过期',
      day7: '7 天',
      day30: '30 天',
      day90: '90 天',
      linkLabel: '分享链接',
      copyHint: '点击链接复制',
      close: '关闭',
      confirm: '生成链接',
      manageTitle: '管理分享链接',
      addNew: '新建分享…',
      copiedToast: '已复制分享链接',
    },
    logoCrop: {
      title: '裁剪资源库头像',
      copy: '拖动画面并调整缩放，保存后将按这个 1:1 构图作为资源库头像。',
      alt: '资源库头像预览',
      zoom: '缩放',
      reselect: '重新选择',
      confirm: '使用裁剪结果',
    },
    logout: {
      closed: 'EchoGallery 已退出，可以关闭此页面。',
    },
  },
};
const copyCatalog = JSON.parse(JSON.stringify(COPY_DEFAULTS));

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

function copyText(path, fallback, params) {
  const segments = String(path || '').split('.');
  let current = copyCatalog;
  for (const segment of segments) {
    if (!current || typeof current !== 'object' || !(segment in current)) {
      return interpolateCopy(fallback, params);
    }
    current = current[segment];
  }
  if (typeof current !== 'string') {
    return interpolateCopy(fallback, params);
  }
  return interpolateCopy(current, params);
}

async function loadCopyCatalog() {
  try {
    const response = await fetch(copyCatalogPath, { cache: 'no-store' });
    if (!response.ok) throw new Error(`copy ${response.status}`);
    const remote = await response.json();
    mergeCopyCatalog(copyCatalog, remote);
  } catch (error) {
    console.warn('加载文案资源失败，使用内置默认文案', error);
  }
}

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
function formatSizeMB(bytes) {
  const value = Number(bytes);
  if (!Number.isFinite(value) || value < 0) return '—';
  return `${(value / 1048576).toFixed(1)} MB`;
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
  return `/api/settings/libraries/${index}/logo?v=${encodeURIComponent(asset)}`;
}
function resolveLibraryLogoURL(library = {}) {
  const explicit = library.logo_image_url || library.logoImageUrl || libraryLogoAssetURL(library);
  if (explicit) return explicit;
  const key = libraryVisualSeed(library);
  return state.libraryRandomLogos && state.libraryRandomLogos[key] || '';
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
    const id = String(library && library.id || '').trim();
    const path = (library && library.path || '').trim();
    if (!path) return;
    const key = id || path.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    result.push({
      index,
      id,
      name: (library && library.name || '').trim() || `资源库 ${index + 1}`,
      path,
      logo_asset: library && (library.logo_asset || library.logoAsset) || '',
      logo_image_url: library && (library.logo_image_url || library.logoImageUrl) || '',
      accent_color: normalizeHexColor(library && (library.accent_color || library.accentColor) || ''),
    });
  });
  if (!result.length && (fallbackPath || '').trim()) {
    result.push({ index: 0, id: '', name: '默认资源库', path: fallbackPath.trim(), logo_asset: '', logo_image_url: '', accent_color: '' });
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
  logout: '',
  shutdown: '',
  plus: '',
  libraryCreate: '',
  advancedSettings: '',
  topbarLoadAll: '',
  topbarUpload: '',
  topbarDownloadFavorites: '',
  topbarShuffle: '',
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
  topbarTimelineOrder: '',
  albumCardFolder: '',
  albumCardUser: '',
  albumCardCount: '',
  albumViewGrid: '',
  albumViewList: '',
  topbarRestoreSelected: '',
  settings: '',
  settingsBold: '',
  shareSmall: '',
  favoriteSmall: '',
  favoriteFilled: '',
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
  infoSize: '',
  infoRatio: '',
  infoDimensions: '',
  infoFileSize: '',
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
  logout: 'logout-1.svg',
  shutdown: 'shutdown.svg',
  plus: 'plus.svg',
  libraryCreate: 'library-create.svg',
  advancedSettings: 'advanced-settings.svg',
  topbarLoadAll: 'topbar-load-all.svg',
  topbarUpload: 'topbar-upload.svg',
  topbarDownloadFavorites: 'topbar-download-favorites.svg',
  topbarShuffle: 'topbar-shuffle.svg',
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
  topbarTimelineOrder: 'topbar-timeline-order.svg',
  albumCardFolder: 'album-card-folder.svg',
  albumCardUser: 'album-card-user.svg',
  albumCardCount: 'album-card-count.svg',
  albumNavPrev: 'album-nav-prev.svg',
  albumNavNext: 'album-nav-next.svg',
  albumViewGrid: 'album-view-grid.svg',
  albumViewList: 'album-view-list.svg',
  topbarRestoreSelected: 'topbar-restore-selected.svg',
  floatingSearch: 'floating-search.svg',
  gridScaleButton: 'grid-scale-button.svg',
  shareSmall: 'share-small.svg',
  favoriteSmall: 'favorite-small.svg',
  favoriteFilled: 'favorite-filled.svg',
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
  infoSize: 'info-size.svg',
  infoRatio: 'info-ratio.svg',
  infoDimensions: 'info-dimensions.svg',
  infoFileSize: 'info-file-size.svg',
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
};

function navIconMarkup(view, active = false) {
  const iconSets = {
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
const themeManualStorageKey = 'echogallery_theme_manual';
const systemThemeQuery = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
function systemTheme() {
  return systemThemeQuery && systemThemeQuery.matches ? 'dark' : 'light';
}
function hasManualThemePreference() {
  return localStorage.getItem(themeManualStorageKey) === '1';
}
function setThemeValue(theme, { manual = false, persist = false } = {}) {
  const next = theme === 'dark' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  state.theme = next;
  if (manual) localStorage.setItem(themeManualStorageKey, '1');
  updateThemeBtn();
  applyLibraryBranding();
  if (persist) persistSettings().catch(() => {});
}
function initTheme() {
  document.documentElement.dataset.theme = systemTheme();
  if (systemThemeQuery) {
    const sync = () => {
      if (!hasManualThemePreference()) setThemeValue(systemTheme());
    };
    if (systemThemeQuery.addEventListener) systemThemeQuery.addEventListener('change', sync);
    else if (systemThemeQuery.addListener) systemThemeQuery.addListener(sync);
  }
}
function toggleTheme() {
  const t = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
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
function normalizeTimelineOrder(value) {
  return value === 'asc' ? 'asc' : 'desc';
}
function normalizeAlbumDetailSort(value) {
  return ['name', 'size', 'timeline_asc', 'timeline_desc'].includes(value) ? value : 'timeline_desc';
}
function normalizePathForCompare(value) {
  return String(value || '').trim().replace(/\\/g, '/').replace(/\/+$/g, '');
}
function normalizeLibrariesForCompare(libraries = []) {
  return normalizeLibraries(libraries).map(library => ({
    id: String(library.id || '').trim(),
    name: String(library.name || '').trim(),
    path: normalizePathForCompare(library.path),
    logo_asset: String(library.logo_asset || library.logoAsset || '').trim(),
    accent_color: normalizeHexColor(library.accent_color || library.accentColor || ''),
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
    { key: 'all', icon: icons.mediaAll || icons.photo, label: '显示全部媒体' },
    { key: 'image', icon: icons.mediaImage || icons.photo, label: '只显示图片' },
    { key: 'video', icon: icons.mediaVideo || icons.play, label: '只显示视频' },
  ];
  const classes = ['topbar-media-filter'];
  if (className) classes.push(className);
  return `<div class="${classes.join(' ')}" role="group" aria-label="媒体类型筛选">
    ${items.map(item => `<button class="media-filter-btn ${active === item.key ? 'active' : ''}" type="button" data-media-kind-filter="${item.key}" title="${item.label}" aria-label="${item.label}" aria-pressed="${active === item.key ? 'true' : 'false'}">${item.icon}</button>`).join('')}
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
function resetMediaFilteredViewState() {
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
function loadVideoBookmarks() {
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
function persistVideoBookmarks() {
  try {
    localStorage.setItem(videoBookmarksStorageKey, JSON.stringify(state.videoBookmarks || {}));
  } catch (_) {
    // 书签是体验增强，不应因为本地存储不可用影响播放。
  }
}
function getVideoBookmarkState(photo, { create = false } = {}) {
  const key = videoBookmarkKey(photo);
  if (!key) return null;
  let entry = state.videoBookmarks[key];
  if (!entry && create) {
    entry = { bookmarks: [], resumeTime: 0 };
    state.videoBookmarks[key] = entry;
  }
  if (entry && !Array.isArray(entry.bookmarks)) {
    state.videoBookmarks[key] = normalizeVideoBookmarkState(entry);
    entry = state.videoBookmarks[key];
  }
  return entry || null;
}
function getVideoBookmarkList(photo) {
  const entry = getVideoBookmarkState(photo);
  return entry ? entry.bookmarks : [];
}
function getVideoBookmarkCount(photo) {
  return isVideoMedia(photo) ? getVideoBookmarkList(photo).length : 0;
}
function getVideoBookmarkCountByKey(key) {
  const entry = state.videoBookmarks[String(key || '')];
  return entry && Array.isArray(entry.bookmarks) ? entry.bookmarks.length : 0;
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
  persistVideoBookmarks();
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(photo);
  updateVideoBookmarkThumbIndicators(photo);
  return true;
}
function deleteVideoBookmarkSlot(photo, slot) {
  const entry = getVideoBookmarkState(photo);
  const targetSlot = Math.max(1, Math.min(10, Number(slot) || 0));
  if (!entry || !targetSlot) return false;
  const before = entry.bookmarks.length;
  entry.bookmarks = entry.bookmarks.filter(item => item.slot !== targetSlot);
  if (entry.bookmarks.length === before) return false;
  persistVideoBookmarks();
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(photo);
  updateVideoBookmarkThumbIndicators(photo);
  return true;
}
function getVideoResumeTime(photo) {
  const entry = getVideoBookmarkState(photo);
  const value = entry ? Number(entry.resumeTime) : 0;
  return Number.isFinite(value) && value >= 0 ? Math.floor(value) : 0;
}
function setVideoResumeTime(photo, seconds) {
  const entry = getVideoBookmarkState(photo, { create: true });
  const value = Math.max(0, Number(seconds) || 0);
  if (!entry) return false;
  entry.resumeTime = Math.floor(value);
  persistVideoBookmarks();
  return true;
}
function updateVideoBookmarkButton(photo = state.lightboxPhotos[state.lightboxIndex]) {
  const btn = $('#lb-video-bookmark');
  if (!btn) return;
  const isVideo = isVideoMedia(photo);
  btn.classList.toggle('hidden', !isVideo);
  if (!isVideo) return;
  const count = getVideoBookmarkList(photo).length;
  btn.classList.toggle('active', count > 0);
  btn.title = count > 0 ? `视频书签 ${count}/10，点击管理` : '视频书签，点击管理';
  btn.setAttribute('aria-label', btn.title);
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
  const current = Math.max(0, Number(video.currentTime) || 0);
  fill.style.width = `${Math.max(0, Math.min(100, current / video.duration * 100))}%`;
  if (currentLabel) currentLabel.textContent = formatDuration(current * 1000);
  if (durationLabel) durationLabel.textContent = formatDuration(video.duration * 1000);
  const bookmarks = getVideoBookmarkList(photo);
  const sectionMinSeconds = Math.max(60, (Number(state.videoSectionMinMinutes) || 10) * 60);
  const sections = video.duration >= sectionMinSeconds
    ? Array.from({ length: 9 }, (_, index) => {
      const section = index + 1;
      const time = video.duration / 10 * section;
      const left = Math.max(0, Math.min(100, time / video.duration * 100));
      return `<button type="button" class="lightbox-video-progress-section-marker" data-video-section="${section}" style="left:${left}%" title="小节 ${section + 1} / 10" aria-label="跳转到小节 ${section + 1}"></button>`;
    })
    : [];
  const bookmarkMarkers = bookmarks.map(bookmark => {
    const left = Math.max(0, Math.min(100, bookmark.time / video.duration * 100));
    const label = bookmark.name ? `${bookmark.name} · ${formatDuration(bookmark.time * 1000)}` : `书签 ${bookmark.slot} · ${formatDuration(bookmark.time * 1000)}`;
    return `<button type="button" class="lightbox-video-progress-marker" data-video-bookmark-slot="${bookmark.slot}" style="left:${left}%" title="${escapeHTML(label)}" aria-label="${escapeHTML(label)}">${bookmark.slot === 10 ? '0' : bookmark.slot}</button>`;
  });
  markers.innerHTML = sections.concat(bookmarkMarkers).join('');
}
function videoPlaybackPreferenceKey(photo) {
  return photo && photo.id ? String(photo.id) : '';
}
function normalizeVideoVolumeValue(value, fallback = 1) {
  const volume = Number(value);
  if (!Number.isFinite(volume)) return Math.max(0, Math.min(1, Number(fallback) || 1));
  return Math.max(0, Math.min(1, volume));
}
function loadGlobalVideoVolume() {
  try {
    return normalizeVideoVolumeValue(localStorage.getItem(globalVideoVolumeStorageKey), 1);
  } catch (_) {
    return 1;
  }
}
function persistGlobalVideoVolume(volume) {
  state.globalVideoVolume = normalizeVideoVolumeValue(volume, state.globalVideoVolume);
  try {
    localStorage.setItem(globalVideoVolumeStorageKey, String(state.globalVideoVolume));
  } catch (_) {}
  return state.globalVideoVolume;
}
function normalizeVideoPlaybackPreference(raw) {
  return {
    volume: normalizeVideoVolumeValue(state.globalVideoVolume, 1),
    muted: !!(raw && raw.muted),
  };
}
async function loadVideoPlaybackPreference(photo) {
  const key = videoPlaybackPreferenceKey(photo);
  if (!key || !isVideoMedia(photo)) return normalizeVideoPlaybackPreference(null);
  if (state.videoPlaybackPreferences[key]) return state.videoPlaybackPreferences[key];
  try {
    const pref = await api.get(`/api/media/${photo.id}/playback`);
    state.videoPlaybackPreferences[key] = normalizeVideoPlaybackPreference(pref);
  } catch (_) {
    state.videoPlaybackPreferences[key] = normalizeVideoPlaybackPreference(null);
  }
  return state.videoPlaybackPreferences[key];
}
async function restoreVideoPlaybackPreference(video, photo) {
  if (!video || !isVideoMedia(photo)) return;
  const pref = await loadVideoPlaybackPreference(photo);
  if (state.lightboxPhotos[state.lightboxIndex] !== photo) return;
  video.dataset.restoringPlaybackPreference = '1';
  video.volume = normalizeVideoVolumeValue(state.globalVideoVolume, 1);
  video.muted = pref.muted;
  requestAnimationFrame(() => {
    if (video) delete video.dataset.restoringPlaybackPreference;
  });
}
function persistVideoPlaybackPreferenceSoon(video = $('#lb-video'), photo = state.lightboxPhotos[state.lightboxIndex]) {
  if (!video || !isVideoMedia(photo) || !photo.id || video.dataset.restoringPlaybackPreference === '1') return;
  persistGlobalVideoVolume(video.volume);
  const key = videoPlaybackPreferenceKey(photo);
  const muted = !!video.muted;
  const previous = state.videoPlaybackPreferences[key] || normalizeVideoPlaybackPreference(null);
  if (previous.muted === muted) return;
  const pref = normalizeVideoPlaybackPreference({ muted });
  state.videoPlaybackPreferences[key] = pref;
  clearTimeout(state.videoPlaybackPreferenceSaveTimer);
  state.videoPlaybackPreferenceSaveTimer = setTimeout(() => {
    api.put(`/api/media/${photo.id}/playback`, { muted: pref.muted }).catch(() => {});
  }, 400);
}
function showVideoProgressActivity(duration = 3000) {
  const progress = $('#lb-video-progress');
  if (!progress || progress.classList.contains('hidden')) return;
  progress.classList.add('active');
  clearTimeout(state.lightboxVideoProgressTimer);
  state.lightboxVideoProgressTimer = setTimeout(() => {
    if (!progress.classList.contains('scrubbing')) progress.classList.remove('active');
  }, duration);
}
function seekVideoFromProgressClientX(track, clientX) {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!track || !video || video.classList.contains('hidden') || !Number.isFinite(video.duration) || video.duration <= 0) return false;
  const rect = track.getBoundingClientRect();
  const ratio = rect.width > 0 ? Math.max(0, Math.min(1, (clientX - rect.left) / rect.width)) : 0;
  video.currentTime = video.duration * ratio;
  updateVideoBookmarkProgress(video, photo);
  showVideoProgressActivity();
  return true;
}
function beginVideoProgressScrub(e, track) {
  if (!track || e.button > 0) return false;
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
function handleLightboxVideoSurfaceClick(e) {
  if (!e || Date.now() < state.videoProgressClickSuppressUntil) return false;
  if (state.slideshowPlaying) return false;
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!isVideoMedia(photo) || !video || video.classList.contains('hidden')) return false;
  if (!e.target.closest('.lightbox-body')) return false;
  if (e.target.closest('.lightbox-header, .lightbox-info, .lightbox-video-progress, .lightbox-volume-feedback, .lb-nav, .context-menu, button, input, select, label, a')) return false;
  if (!e.target.closest('.lightbox-media-frame, #lb-video')) return false;
  const body = $('.lightbox-body');
  const rect = body ? body.getBoundingClientRect() : video.getBoundingClientRect();
  const ratio = rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5;
  const side = ratio <= 0.25 ? 'left' : ratio >= 0.75 ? 'right' : '';
  const isTouch = state.lightboxLastPointerType === 'touch' || state.lightboxLastPointerType === 'pen';
  if (isTouch && side) {
    const now = Date.now();
    const last = state.videoSurfaceLastTap || {};
    clearTimeout(state.videoSurfaceTapTimer);
    if (last.side === side && now - last.time <= 340) {
      state.videoSurfaceLastTap = null;
      adjustVideoTime(side === 'left' ? -5 : 5);
    } else {
      state.videoSurfaceLastTap = { side, time: now };
      state.videoSurfaceTapTimer = setTimeout(() => {
        state.videoSurfaceLastTap = null;
        toggleVideoPlayback();
      }, 260);
    }
  } else {
    clearTimeout(state.videoSurfaceTapTimer);
    state.videoSurfaceLastTap = null;
    toggleVideoPlayback();
  }
  e.preventDefault();
  e.stopPropagation();
  return true;
}
function saveCurrentVideoResumePosition({ quiet = false } = {}) {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!state.continueLastVideoPosition || !isVideoMedia(photo) || !video || video.classList.contains('hidden')) return false;
  if (!setVideoResumeTime(photo, video.currentTime || 0)) return false;
  if (!quiet) showToast(`已记住上次播放位置 ${formatDuration(video.currentTime * 1000)}`);
  return true;
}
function handleVideoResumeTimeUpdate(video) {
  if (!state.continueLastVideoPosition || !video || video.classList.contains('hidden')) return;
  const photo = state.lightboxPhotos[state.lightboxIndex];
  if (!isVideoMedia(photo) || !Number.isFinite(video.duration) || video.duration <= 0) return;
  const current = Number(video.currentTime) || 0;
  if (current < 0 || current > video.duration - 1) return;
  const now = Date.now();
  if (state.videoBookmarkSaveTimer && now - state.videoBookmarkSaveTimer < 2500) return;
  state.videoBookmarkSaveTimer = now;
  setVideoResumeTime(photo, current);
}
function restoreVideoResumePosition(video, photo) {
  if (!video || !isVideoMedia(photo)) return;
  if (!state.continueLastVideoPosition) return;
  const saved = getVideoResumeTime(photo);
  if (saved > 0 && Number.isFinite(video.duration) && saved < video.duration - 2) {
    video.currentTime = saved;
    showToast(`已从上次播放位置继续：${formatDuration(saved * 1000)}`);
  }
  updateVideoBookmarkButton(photo);
  updateVideoBookmarkProgress(video, photo);
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
    showToast(`已跳转到书签 ${slot}${bookmark.name ? `：${bookmark.name}` : ''}`);
    updateVideoBookmarkProgress(video, photo);
  } else if (setVideoBookmarkSlot(photo, slot, video.currentTime || 0)) {
    showToast(`已添加书签 ${slot}`);
  } else {
    showToast('附近 1 秒内已有书签，未新增');
  }
  return true;
}

function initGridScale() {
  state.gridSize = 180;
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  updateGridScaleProgress();
}
function setGridScale(size) {
  state.gridSize = Math.min(260, Math.max(72, Number(size) || 180));
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  updateGridScaleProgress();
  updateGridScaleButtonTitle();
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
function updateGridScaleButtonTitle() {
  const btn = $('#grid-scale-btn');
  const label = `缩放，当前 ${state.gridSize}px`;
  if (btn) {
    btn.title = label;
    btn.setAttribute('aria-label', label);
  }
  const input = $('#floating-grid-scale-input');
  if (input && input.value !== String(state.gridSize)) input.value = String(state.gridSize);
  updateRangeProgress(input);
}
function cycleGridScale(direction = 1) {
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
      .catch(e => console.error('保存缩放级别失败:', e));
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
    btn.title = state.slideshowPlaying ? '暂停幻灯片' : '播放幻灯片';
    btn.setAttribute('aria-label', btn.title);
    btn.classList.toggle('active', state.slideshowPlaying);
  }
  if (interval) {
    interval.value = String(state.slideshowInterval / 1000);
    updateRangeProgress(interval);
  }
  if (intervalValue) intervalValue.textContent = `${state.slideshowInterval / 1000} 秒`;
  if (loop) {
    loop.innerHTML = icons.slideshowLoop;
    loop.title = state.slideshowLoop ? '关闭循环' : '开启循环';
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
  scheduleSlideshowStep();
}
function toggleSlideshow() {
  if (state.slideshowPlaying) stopSlideshow();
  else startSlideshow();
}
function shouldStopSlideshowFromClick(target) {
  if (!state.slideshowPlaying || !target) return false;
  if (!target.closest('#lightbox.open')) return false;
  if (target.closest('.lightbox-header, .lightbox-info, .lb-nav, .context-menu, button, input, select, label')) return false;
  return !!target.closest('.lightbox-body');
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
    return { zoom: 300, mode: 'scale' };
  }
  return { zoom: state.lightboxZoom, mode: state.lightboxZoomMode };
}
function setLightboxZoom(value) {
  state.lightboxZoom = Math.min(300, Math.max(50, Number(value) || 100));
  state.lightboxZoomMode = 'scale';
  applyLightboxZoom();
}
function setLightboxFit() {
  state.lightboxZoom = 100;
  state.lightboxZoomMode = 'fit';
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
function lightboxViewportMetrics(body) {
  if (!body) return { width: 0, height: 0 };
  const style = window.getComputedStyle(body);
  const paddingLeft = parseFloat(style.paddingLeft || '0');
  const paddingRight = parseFloat(style.paddingRight || '0');
  const paddingTop = parseFloat(style.paddingTop || '0');
  const paddingBottom = parseFloat(style.paddingBottom || '0');
  return {
    width: Math.max(1, body.clientWidth - paddingLeft - paddingRight),
    height: Math.max(1, body.clientHeight - paddingTop - paddingBottom),
    paddingLeft,
    paddingTop,
    paddingRight,
    paddingBottom,
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
  if (label) label.textContent = `${effective.zoom}%`;
  if (input) {
    input.value = String(state.lightboxZoom);
    updateRangeProgress(input);
  }
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
    id: String(data.id || '').trim(),
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
  const normalizedID = String(currentID || '').trim();
  if (normalizedID) {
    const byID = libraries.find(item => String(item.id || '').trim() === normalizedID);
    if (byID) return byID;
  }
  const normalizedPath = String(currentPath || '').trim();
  return libraries.find(item => item.path === normalizedPath) || libraries[0] || null;
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
      if (state.view === 'settings') renderSettings().catch(e => console.warn('刷新资源库头像失败', e));
    }
  } catch (e) {
    console.warn('随机资源库头像加载失败', e);
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
  document.title = current && current.name ? `${current.name} - EchoGallery` : 'EchoGallery';
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
async function backfillPhotoEXIF() {
  return api.post('/api/settings/exif/backfill', {});
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
  <div class="search-panel" role="dialog" aria-modal="true" aria-label="${escapeHTML(copyText('app.search.ariaLabel', '全局搜索'))}">
    <div class="search-box">
      <span class="search-box-icon">${icons.search}</span>
      <input id="global-search-input" type="text" autocomplete="off" spellcheck="false" placeholder="${escapeHTML(copyText('app.search.placeholder', '搜索文件名、类型、UUID'))}">
      <button class="btn-icon search-close-btn" id="search-close-btn" type="button" aria-label="${escapeHTML(copyText('app.search.close', '关闭'))}">${icons.close}</button>
    </div>
    <div class="search-filters" id="search-filters">
      ${renderSearchFilterButton('all', '全部')}
      ${renderSearchFilterButton('photo', '照片')}
      ${renderSearchFilterButton('video', '视频')}
      ${renderSearchFilterButton('album', '相册')}
      ${renderSearchFilterButton('favorite', '个人收藏')}
    </div>
    <div class="search-status" id="search-status"></div>
    <div class="search-results" id="search-results"></div>
    <div class="search-actions">
      <button class="btn search-download-btn" id="search-download-all-btn" type="button">${escapeHTML(copyText('app.search.downloadAll', '打包下载全部'))}</button>
      <button class="btn search-more-btn" id="search-more-btn" type="button">${escapeHTML(copyText('app.search.loadMore', '加载更多'))}</button>
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

function openGlobalSearch(prefill) {
  const overlay = $('#search-overlay');
  const input = $('#global-search-input');
  if (!overlay || !input) return;
  state.searchOpen = true;
  overlay.classList.add('open');
  overlay.setAttribute('aria-hidden', 'false');
  if (prefill === undefined) syncSearchInputs(input.value || state.searchQuery || '');
  else syncSearchInputs(prefill);
  setTimeout(() => {
    input.focus();
    input.select();
  }, 0);
  renderSearchResults();
  if (input.value.trim() && input.value.trim() !== state.searchQuery) scheduleGlobalSearch();
}

function closeGlobalSearch() {
  state.searchOpen = false;
  if (state.searchAbortController) state.searchAbortController.abort();
  if (state.searchDownloadAbortController) state.searchDownloadAbortController.abort();
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
      const response = await fetch(`/api/media/search?${params.toString()}`, { signal: controller.signal });
      if (!response.ok) throw await response.json();
      const page = await response.json();
      const photos = page.photos || [];
      state.searchResults = reset ? photos : state.searchResults.concat(photos);
      state.searchCursor = page.next_cursor || '';
      state.searchHasMore = !!page.has_more;
      mediaTotal = Number.isFinite(Number(page.total)) ? Number(page.total) : state.searchResults.length;
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
      showToast('搜索失败，请稍后重试');
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
    const response = await fetch('/api/media/albums', { signal });
    if (!response.ok) throw await response.json();
    state.searchAlbumCache = await response.json();
  }
  const needle = query.trim().toLowerCase();
  return (state.searchAlbumCache || []).filter(album => {
    const haystack = `${album.name || ''} ${album.description || ''}`.toLowerCase();
    return haystack.includes(needle);
  }).slice(0, 24);
}

function searchResultMeta(photo) {
  const kind = isVideoMedia(photo) ? '视频' : '图片';
  const size = formatSizeMB(photo.size);
  const date = formatDateTime(photo.taken_at);
  return [kind, date, size].filter(value => value && value !== '—').join(' · ');
}

function renderSearchResults() {
  const resultsEl = $('#search-results');
  const statusEl = $('#search-status');
  const moreBtn = $('#search-more-btn');
  const downloadBtn = $('#search-download-all-btn');
  if (!resultsEl || !statusEl || !moreBtn || !downloadBtn) return;
  const query = $('#global-search-input')?.value.trim() || state.searchQuery;
  if (!query) {
    statusEl.textContent = '';
    resultsEl.innerHTML = `<div class="search-empty">输入关键词开始搜索</div>`;
    moreBtn.classList.remove('visible');
    downloadBtn.classList.remove('visible');
    return;
  }
  if (state.searchLoading && !state.searchResults.length) {
    statusEl.textContent = '搜索中…';
    resultsEl.innerHTML = `<div class="search-empty"><div class="spinner"></div></div>`;
    moreBtn.classList.remove('visible');
    downloadBtn.classList.remove('visible');
    return;
  }
  statusEl.textContent = state.searchTotal ? `找到 ${state.searchTotal} 条结果` : (state.searchResults.length ? `找到 ${state.searchResults.length} 条结果` : '');
  if (!state.searchResults.length && !state.searchAlbumResults.length) {
    resultsEl.innerHTML = `<div class="search-empty">没有匹配结果</div>`;
  } else {
    resultsEl.innerHTML = '';
    const fragment = document.createDocumentFragment();
    state.searchAlbumResults.forEach(album => {
      const card = el('button', 'search-result-card search-result-album');
      card.type = 'button';
      const cover = album.cover_uuid
        ? `<img loading="lazy" src="/media/thumbnails/${album.cover_uuid}" alt="${escapeHTML(album.name || '')}">`
        : `<span class="search-result-album-icon">${icons.album}</span>`;
      card.innerHTML = `
        <span class="search-result-thumb">
          ${cover}
          <span class="search-result-kind">相册</span>
        </span>
        <span class="search-result-main">
          <strong>${escapeHTML(album.name || '未命名相册')}</strong>
          <span>${album.photo_count || 0} 条媒体${album.description ? ` · ${escapeHTML(album.description)}` : ''}</span>
        </span>`;
      card.addEventListener('click', () => {
        closeGlobalSearch();
        openAlbumDetail(album);
      });
      fragment.appendChild(card);
    });
    state.searchResults.forEach((photo, index) => {
      const card = el('button', 'search-result-card');
      card.type = 'button';
      card.innerHTML = `
        <span class="search-result-thumb">
          <img loading="lazy" src="${mediaThumbURL(photo)}" alt="${escapeHTML(photo.original_name || '')}">
          ${isVideoMedia(photo) ? '<span class="search-result-kind">视频</span>' : ''}
        </span>
        <span class="search-result-main">
          <strong>${escapeHTML(photo.original_name || '未命名媒体')}</strong>
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
      fragment.appendChild(card);
    });
    resultsEl.appendChild(fragment);
  }
  moreBtn.classList.toggle('visible', state.searchHasMore);
  moreBtn.disabled = state.searchLoading;
  moreBtn.textContent = state.searchLoading ? '加载中…' : '加载更多';
  const hasDownloadableMedia = state.searchFilter !== 'album' && (state.searchTotal > state.searchAlbumResults.length || state.searchResults.length);
  downloadBtn.classList.toggle('visible', !!query && hasDownloadableMedia);
  downloadBtn.disabled = state.searchLoading && !state.searchDownloadLoading;
  downloadBtn.textContent = state.searchDownloadLoading ? '取消下载' : '打包下载全部';
  downloadBtn.classList.toggle('btn-danger', state.searchDownloadLoading);
}

function viewShortcutMap() {
  return ['timeline', 'favorites', 'random-album', 'albums', 'memories', 'trash', 'settings'];
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
  } catch (_) {
    state.serverSettings = {
      port: 8080,
      active_library_id: '',
      storage_path: '',
      libraries: [],
      thumbnail_dir: '',
      thumbnail_size: 512,
      trash_dir: '',
      use_system_player: false,
      jwt_secret_masked: '未加载',
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
      experimental_autoplay_video: false,
      video_autoplay_next: false,
      video_section_min_minutes: 10,
      experimental_prefetch_neighbors: true,
      experimental_restore_last_view: false,
      continue_last_video_position: true,
      low_resource_mode: false,
      player_keymap: '',
    };
    applyServerSettings(state.serverSettings);
  }
  state.savedServerSettings = cloneSettingsSnapshot(state.serverSettings);
  state.settingsDirty = false;
  return state.serverSettings;
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
async function fetchLibraryBuildStatus() {
  return api.get('/api/library-build/status');
}
async function setLibraryBuildExitAfterComplete(enabled) {
  return api.put('/api/library-build/exit-after-complete', { enabled });
}
async function cancelLibraryBuild() {
  return api.del('/api/library-build');
}
async function fetchLibraryBatchBuildStatus() {
  return api.get('/api/settings/libraries/build-all');
}
async function startLibraryBatchBuild(options = {}) {
  return api.post('/api/settings/libraries/build-all', { aggressive: !!options.aggressive });
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
  return api.get('/api/settings/libraries/thumbnails/build-all');
}
async function startLibraryBatchThumbnailBuild(options = {}) {
  return api.post('/api/settings/libraries/thumbnails/build-all', {
    aggressive: !!options.aggressive,
    move_legacy_thumbnails: !!options.moveLegacyThumbnails,
    clean_thumbnail_files: !!options.cleanThumbnailFiles,
  });
}
async function cancelLibraryBatchThumbnailBuild() {
  return api.del('/api/settings/libraries/thumbnails/build-all');
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
  state.serverSettings.port = Number(data.port) || 8080;
  state.serverSettings.active_library_id = data.active_library_id || '';
  state.serverSettings.storage_path = data.storage_path || '';
  state.serverSettings.libraries = normalizeLibraryList(data.libraries, data.storage_path || '');
  state.serverSettings.thumbnail_dir = data.thumbnail_dir || '';
  state.serverSettings.thumbnail_size = 512;
  state.serverSettings.trash_dir = data.trash_dir || '';
  state.serverSettings.use_system_player = !!data.use_system_player;
  state.serverSettings.jwt_secret_masked = data.jwt_secret_masked || '未设置';
  state.serverSettings.users = Array.isArray(data.users) ? data.users : [];
  setThemeValue(hasManualThemePreference() ? data.theme : systemTheme());
  state.gridSize = Math.min(260, Math.max(72, Number(data.grid_size) || 180));
  state.gridGap = Math.min(24, Math.max(0, Number(data.grid_gap) || 2));
  state.thumbRadius = Math.min(24, Math.max(0, Number(data.thumb_radius) || 2));
  state.slideshowMode = data.slideshow_mode === 'sequential' ? 'sequential' : 'random';
  state.slideshowLoop = data.slideshow_loop !== false;
  state.slideshowInterval = Math.min(30000, Math.max(1000, Number(data.slideshow_interval) || 5000));
  state.lightboxZoom = Math.min(300, Math.max(50, Number(data.lightbox_zoom) || 100));
  state.lightboxZoomMode = state.lightboxZoom === 100 ? 'fit' : 'scale';
  state.experimentalAutoplayVideo = !!data.experimental_autoplay_video;
  state.videoAutoplayNext = !!data.video_autoplay_next;
  state.videoSectionMinMinutes = Math.min(240, Math.max(1, Number(data.video_section_min_minutes) || 10));
  state.experimentalPrefetchNeighbors = data.experimental_prefetch_neighbors !== false;
  state.experimentalRestoreLastView = !!data.experimental_restore_last_view;
  state.continueLastVideoPosition = data.continue_last_video_position !== false;
  state.serverSettings.low_resource_mode = !!data.low_resource_mode;
  updatePlayerKeymapSource(data.player_keymap || '');
  document.documentElement.style.setProperty('--grid-size', state.gridSize + 'px');
  updateGridScaleProgress();
  applyDisplaySettings();
  applyLightboxZoom();
  updateSlideshowControls();
  syncSidebarUI();
  updateThemeBtn();
  applyLibraryBranding();
}
function buildSettingsPayload() {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  const activeLibrary = resolveActiveLibrary(libraries, state.serverSettings.active_library_id || '', state.serverSettings.storage_path || '');
  let activePath = (activeLibrary && activeLibrary.path || state.serverSettings.storage_path || '').trim();
  if (!activePath && libraries.length) activePath = libraries[0].path;
  return {
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
    lightbox_zoom: state.lightboxZoom,
    experimental_autoplay_video: !!state.experimentalAutoplayVideo,
    video_autoplay_next: !!state.videoAutoplayNext,
    video_section_min_minutes: state.videoSectionMinMinutes || 10,
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
  showToast(`已跳转到小节 ${targetIndex + 1} / 10`);
  return true;
}
function adjustVideoVolume(delta) {
  const video = $('#lb-video');
  if (!video || video.classList.contains('hidden')) return false;
  video.volume = Math.max(0, Math.min(1, video.volume + delta / 100));
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
  const value = video.muted ? 0 : Math.round((video.volume || 0) * 100);
  fill.style.width = `${value}%`;
  label.textContent = video.muted ? '静音' : `${value}%`;
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
    toggleCurrentLightboxFavorite();
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
  const label = `缩放，当前 ${state.gridSize}px`;
  return `<div class="floating-grid-scale-control" id="grid-scale-control">
    <button class="floating-search-btn floating-grid-scale-btn" id="grid-scale-btn" type="button" aria-label="${escapeHTML(label)}" title="${escapeHTML(label)}">${icons.gridScaleButton || icons.search}</button>
    <label class="floating-grid-scale-slider" for="floating-grid-scale-input" aria-label="照片墙缩放">
      <input id="floating-grid-scale-input" type="range" min="72" max="260" step="8" value="${state.gridSize}">
    </label>
  </div>`;
}
function renderTimelineOrderControl() {
  const ascending = state.timelineOrder === 'asc';
  const label = ascending ? '时间线正序' : '时间线倒序';
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
  return `<div class="topbar-media-filter album-view-toggle" role="group" aria-label="相册显示方式">
    <button class="media-filter-btn album-view-btn ${state.albumViewMode === 'grid' ? 'active' : ''}" id="album-view-grid-btn" type="button" title="大图" aria-label="大图" aria-pressed="${state.albumViewMode === 'grid' ? 'true' : 'false'}">${icons.albumViewGrid}</button>
    <button class="media-filter-btn album-view-btn ${state.albumViewMode === 'list' ? 'active' : ''}" id="album-view-list-btn" type="button" title="列表" aria-label="列表" aria-pressed="${state.albumViewMode === 'list' ? 'true' : 'false'}">${icons.albumViewList}</button>
  </div>`;
}
function renderAlbumDetailSortControl() {
  const active = normalizeAlbumDetailSort(state.albumDetailSort);
  const items = [
    { key: 'timeline_desc', label: '时间线倒序', icon: icons.albumSortTimelineDesc },
    { key: 'timeline_asc', label: '时间线正序', icon: icons.albumSortTimelineAsc },
    { key: 'name', label: '媒体名称', icon: icons.albumSortName },
    { key: 'size', label: '文件大小', icon: icons.albumSortSize },
  ];
  return `<div class="topbar-media-filter album-detail-sort-filter" role="group" aria-label="相册排序">
    ${items.map(item => `<button class="media-filter-btn album-detail-sort-btn ${active === item.key ? 'active' : ''}" type="button" data-album-detail-sort="${item.key}" title="${item.label}" aria-label="${item.label}" aria-pressed="${active === item.key ? 'true' : 'false'}">${item.icon || ''}</button>`).join('')}
  </div>`;
}
function renderAlbumDetailBottomActionControls() {
  return `<div class="album-detail-bottom-center">
    ${renderAlbumDetailSortControl()}
    ${renderMediaKindFilterControl('album-detail-filter')}
  </div>`;
}
async function revealInFinder(photoId) {
  try {
    await api.post(`/api/media/${photoId}/reveal`, {});
    showToast('已在文件管理器中定位');
  } catch (e) {
    alert('打开失败: ' + (e.error || e));
  }
}
async function revealAlbumInFinder(albumId) {
  try {
    await api.post(`/api/media/albums/${albumId}/reveal`, {});
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
  randomAlbumViewLoaded: false,
  randomAlbumTotal: 0,
  randomAlbumAbortController: null,
  timelineOrder: normalizeTimelineOrder(localStorage.getItem(timelineOrderStorageKey)),
  timelineCursor: '',
  timelineHasMore: true,
  timelineLoading: false,
  timelineTotal: 0,
  timelineLoaded: false,
  trashPhotos: [],
  trashCursor: '',
  trashHasMore: true,
  trashLoading: false,
  trashLoaded: false,
  albums: [],
  albumsLoaded: false,
  albumViewMode: 'grid',
  albumDetailSort: normalizeAlbumDetailSort(localStorage.getItem(albumDetailSortStorageKey)),
  currentAlbum: null,
  albumPhotos: [],
  albumCursor: '',
  albumHasMore: true,
  albumLoading: false,
  albumDetailLoadedKey: '',
  pendingAlbumPhotoID: null,
  lastAlbumDetailID: null,
  selected: new Set(),
  selectionAnchorID: null,
  lightboxPhotos: [],
  lightboxIndex: 0,
  lightboxSeedPhotoID: null,
  lightboxSeedPreviewURL: '',
  lightboxReturnView: '',
  lightboxReturnAlbumID: null,
  lightboxPageLoading: false,
  // b-2: 当前用户的分享链接，key=`${type}:${targetId}`
  shareMap: {},
  shareLinks: [],
  shareLinksLoaded: false,
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
  gridGap: 2,
  thumbRadius: 2,
  serverSettings: {
    port: 8080,
    active_library_id: '',
    storage_path: '',
    libraries: [],
    thumbnail_dir: '',
    thumbnail_size: 512,
    trash_dir: '',
    use_system_player: false,
    jwt_secret_masked: '未加载',
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
    experimental_autoplay_video: false,
    video_autoplay_next: false,
    video_section_min_minutes: 10,
    experimental_prefetch_neighbors: true,
    experimental_restore_last_view: false,
    continue_last_video_position: true,
    low_resource_mode: false,
    player_keymap: '',
  },
  savedServerSettings: null,
  playerKeymap: {},
  playerKeymapSource: '',
  experimentalAutoplayVideo: false,
  videoAutoplayNext: false,
  videoSectionMinMinutes: 10,
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
  settingsReady: false,
  settingsFocus: '',
  settingsScrollRestorePending: true,
  lightboxPlaybackToken: 0,
  lightboxMediaLoading: false,
  lightboxUiIdleTimer: null,
  lightboxVolumeTimer: null,
  lightboxVideoProgressTimer: null,
  videoProgressScrubPointerId: null,
  videoProgressClickSuppressUntil: 0,
  videoSurfaceLastTap: null,
  videoSurfaceTapTimer: null,
  lightboxLastPointerType: '',
  autoplayMutedHintShown: false,
  viewScrollPositions: {},
  loadMoreObserver: null,
  gridScalePersistTimer: null,
  prefetchedMediaKeys: [],
  prefetchedMediaSet: new Set(),
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
  searchDownloadAbortController: null,
  searchDownloadLoading: false,
  mediaKindFilter: normalizeMediaKindFilter(localStorage.getItem(mediaKindFilterStorageKey)),
  videoBookmarks: loadVideoBookmarks(),
  videoBookmarkSaveTimer: null,
  globalVideoVolume: loadGlobalVideoVolume(),
  videoPlaybackPreferences: {},
  videoPlaybackPreferenceSaveTimer: null,
  activePreloadJobs: new Set(),
  visibleThumbnailWarmSignatures: {},
  autoUpdateEnabled: localStorage.getItem('echogallery_auto_update') === '1',
  libraryBuildStatus: { status: 'discovering', message: '正在检查资源库状态' },
  libraryBuildPollTimer: null,
  libraryBatchBuildStatus: { status: 'idle', message: '当前没有批量扫描任务' },
  libraryBatchBuildPollTimer: null,
  libraryBatchBuildCancelPending: false,
  libraryBatchWorkflowEnabled: localStorage.getItem(libraryBatchWorkflowEnabledStorageKey) === '1',
  libraryBatchWorkflowAggressive: localStorage.getItem(libraryBatchWorkflowAggressiveStorageKey) === '1',
  libraryBatchWorkflowMoveLegacyThumbnails: localStorage.getItem(libraryBatchWorkflowMoveLegacyStorageKey) === '1',
  libraryBatchWorkflowCleanThumbnailFiles: localStorage.getItem(libraryBatchWorkflowCleanFilesStorageKey) === '1',
  libraryBatchWorkflowPhase: localStorage.getItem(libraryBatchWorkflowPhaseStorageKey) || '',
  libraryBatchThumbnailBuildStatus: { status: 'idle', message: '当前没有批量缩略图任务' },
  libraryBatchThumbnailBuildPollTimer: null,
  libraryBatchThumbnailBuildCancelPending: false,
  thumbnailBuildStatus: { status: 'idle', message: '当前没有缩略图任务' },
  thumbnailBuildPollTimer: null,
  thumbnailBuildCancelPending: false,
  thumbnailBuildOverlaySuppressed: false,
  thumbnailVisibleRefreshAt: 0,
  videoThumbnailRefreshStatus: { status: 'idle', message: '当前没有视频缩略图任务' },
  videoThumbnailRefreshPollTimer: null,
  videoThumbnailRefreshCancelPending: false,
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
    requestAnimationFrame(() => {
      if (view !== state.view) return;
      if (view === 'album-detail' && albumID !== state.currentAlbumID) return;
      window.scrollTo(0, top);
    });
  });
}

// ── 分享状态加载 ─────────────────────────────────────
async function loadShareMap() {
  try {
		const links = await api.get('/api/media/shares');
    setShareLinks(links || []);
    state.shareLinksLoaded = true;
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
  while (state.prefetchedMediaKeys.length > 72) {
    const expired = state.prefetchedMediaKeys.shift();
    if (expired) state.prefetchedMediaSet.delete(expired);
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
    overlay.innerHTML = `<div class="blocking-progress-card"><div class="spinner"></div><strong></strong><span></span><button class="btn btn-sm blocking-progress-cancel" type="button" hidden>取消</button></div>`;
    document.body.appendChild(overlay);
  }
  $('strong', overlay).textContent = title;
  $('span', overlay).textContent = detail;
  overlay.classList.toggle('non-blocking', nonBlocking);
  const cancelBtn = $('.blocking-progress-cancel', overlay);
  if (cancelBtn) {
    if (typeof options.onCancel === 'function') {
      cancelBtn.hidden = false;
      cancelBtn.textContent = options.cancelText || '取消';
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
    status && status.message ? status.message : '正在生成缩略图…',
    `已处理 ${countText}`,
    `新生成 ${generated}`,
  ];
  if (skipped > 0) parts.push(`已存在 ${skipped}`);
  if (failed > 0) parts.push(`失败 ${failed}`);
  if (Number(status && status.eta_seconds) > 0) parts.push(`ETA ${formatDuration((Number(status.eta_seconds) || 0) * 1000)}`);
  return parts.join(' · ');
}

function syncThumbnailBuildOverlay(status = state.thumbnailBuildStatus) {
  if (!isThumbnailBuildActive(status) || state.thumbnailBuildOverlaySuppressed) {
    hideBlockingProgress();
    return;
  }
  showBlockingProgress('正在加载全部缩略图', formatThumbnailBuildDetail(status), {
    cancelText: state.thumbnailBuildCancelPending ? '正在取消…' : '取消',
    onCancel: () => {
      if (state.thumbnailBuildCancelPending) return;
      state.thumbnailBuildCancelPending = true;
      updateBlockingProgress('正在发送取消请求…');
      void cancelThumbnailBuild().catch(e => {
        state.thumbnailBuildCancelPending = false;
        updateBlockingProgress('取消失败，请稍后重试');
        console.error('取消缩略图任务失败', e);
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
    status && status.message ? status.message : '正在刷新视频缩略图…',
    `已处理 ${countText}`,
    `已刷新 ${refreshed}`,
  ];
  if (failed > 0) parts.push(`失败 ${failed}`);
  if (Number(status && status.eta_seconds) > 0) parts.push(`ETA ${formatDuration((Number(status.eta_seconds) || 0) * 1000)}`);
  return parts.join(' · ');
}

function syncVideoThumbnailRefreshOverlay(status = state.videoThumbnailRefreshStatus) {
  if (!isVideoThumbnailRefreshActive(status)) {
    hideBlockingProgress();
    return;
  }
  showBlockingProgress('正在刷新视频缩略图', formatVideoThumbnailRefreshDetail(status), {
    cancelText: state.videoThumbnailRefreshCancelPending ? '正在取消…' : '取消',
    onCancel: () => {
      if (state.videoThumbnailRefreshCancelPending) return;
      state.videoThumbnailRefreshCancelPending = true;
      updateBlockingProgress('正在发送取消请求…');
      void cancelVideoThumbnailRefresh().catch(e => {
        state.videoThumbnailRefreshCancelPending = false;
        updateBlockingProgress('取消失败，请稍后重试');
        console.error('取消视频缩略图任务失败', e);
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

async function pollThumbnailBuildStatus() {
  stopThumbnailBuildPolling();
  try {
    const status = await fetchThumbnailBuildStatus();
    const previous = state.thumbnailBuildStatus || {};
    state.thumbnailBuildStatus = status || { status: 'idle', message: '当前没有缩略图任务' };
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
        showToast(`全部缩略图已就绪：新生成 ${state.thumbnailBuildStatus.generated || 0} 项`, 3200);
      } else if (state.thumbnailBuildStatus.status === 'cancelled') {
        showToast('已取消加载全部缩略图');
      } else if (state.thumbnailBuildStatus.status === 'failed') {
        showToast('加载全部缩略图失败');
      }
    }
  } catch (e) {
    console.error('获取缩略图任务状态失败', e);
    hideBlockingProgress();
    state.thumbnailBuildCancelPending = false;
  }
}

async function pollVideoThumbnailRefreshStatus() {
  stopVideoThumbnailRefreshPolling();
  try {
    const status = await fetchVideoThumbnailRefreshStatus();
    const previous = state.videoThumbnailRefreshStatus || {};
    state.videoThumbnailRefreshStatus = status || { status: 'idle', message: '当前没有视频缩略图任务' };
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
        showToast(`视频缩略图已刷新：完成 ${state.videoThumbnailRefreshStatus.refreshed || 0} 项`, 3200);
      } else if (state.videoThumbnailRefreshStatus.status === 'cancelled') {
        showToast('已取消刷新视频缩略图');
      } else if (state.videoThumbnailRefreshStatus.status === 'failed') {
        showToast('刷新视频缩略图失败');
      }
    }
  } catch (e) {
    console.error('获取视频缩略图任务状态失败', e);
    hideBlockingProgress();
    state.videoThumbnailRefreshCancelPending = false;
  }
}

async function pollLibraryBatchBuildStatus() {
  stopLibraryBatchBuildPolling();
  try {
    const status = await fetchLibraryBatchBuildStatus();
    const previous = state.libraryBatchBuildStatus || {};
    state.libraryBatchBuildStatus = status || { status: 'idle', message: '当前没有批量扫描任务' };
    if (state.libraryBatchBuildStatus.status !== 'cancelling') {
      state.libraryBatchBuildCancelPending = false;
    }
    if (state.view === 'settings') {
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    }
    if (isLibraryBatchBuildActive(state.libraryBatchBuildStatus)) {
      state.libraryBatchBuildPollTimer = setTimeout(pollLibraryBatchBuildStatus, 600);
      return;
    }
    if (state.libraryBatchWorkflowPhase === 'scan') {
      if (state.libraryBatchBuildStatus.status === 'completed') {
        setLibraryBatchWorkflowPhase('thumbnails');
        try {
          await syncLibraryBatchWorkflowSelections();
          await openLibraryBatchThumbnailBuildWorkflow({ aggressive: state.libraryBatchWorkflowAggressive, startedByWorkflow: true });
        } catch (workflowError) {
          clearLibraryBatchWorkflowPhase();
          alert('启动批量缩略图失败: ' + ((workflowError && workflowError.error) || workflowError.message || workflowError));
        }
      } else if (state.libraryBatchBuildStatus.status === 'cancelled' || state.libraryBatchBuildStatus.status === 'failed') {
        clearLibraryBatchWorkflowPhase();
      }
    }
    if (previous.status === 'running' || previous.status === 'cancelling') {
      if (state.libraryBatchBuildStatus.status === 'completed') {
        showToast(state.libraryBatchBuildStatus.message || copyText('app.libraryBatchScan.completed', '批量扫描已完成'), 3600);
      } else if (state.libraryBatchBuildStatus.status === 'cancelled') {
        showToast(copyText('app.libraryBatchScan.cancelled', '已取消批量扫描资源库'), 3200);
      } else if (state.libraryBatchBuildStatus.status === 'failed') {
        showToast(copyText('app.libraryBatchScan.failed', '批量扫描失败'), 3200);
      }
      if (state.view === 'settings') renderSettingsContent();
    }
  } catch (e) {
    console.error('获取批量扫描任务状态失败', e);
    state.libraryBatchBuildCancelPending = false;
  }
}

async function pollLibraryBatchThumbnailBuildStatus() {
  stopLibraryBatchThumbnailBuildPolling();
  try {
    const status = await fetchLibraryBatchThumbnailBuildStatus();
    const previous = state.libraryBatchThumbnailBuildStatus || {};
    state.libraryBatchThumbnailBuildStatus = status || { status: 'idle', message: '当前没有批量缩略图任务' };
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
        showToast(state.libraryBatchThumbnailBuildStatus.message || copyText('app.libraryBatchThumbnail.completed', '批量缩略图构建已完成'), 3600);
      } else if (state.libraryBatchThumbnailBuildStatus.status === 'cancelled') {
        showToast(copyText('app.libraryBatchThumbnail.cancelled', '已取消批量缩略图任务'), 3200);
      } else if (state.libraryBatchThumbnailBuildStatus.status === 'failed') {
        showToast(copyText('app.libraryBatchThumbnail.failed', '批量缩略图构建失败'), 3200);
      }
      if (state.view === 'settings') renderSettingsContent();
    }
  } catch (e) {
    console.error('获取批量缩略图任务状态失败', e);
    state.libraryBatchThumbnailBuildCancelPending = false;
  }
}

async function openVideoThumbnailRefreshWorkflow() {
  try {
    const status = await refreshVideoThumbnails();
    state.videoThumbnailRefreshStatus = status || { status: 'idle', message: '当前没有视频缩略图任务' };
    state.videoThumbnailRefreshCancelPending = false;
    syncVideoThumbnailRefreshOverlay(state.videoThumbnailRefreshStatus);
    if (isVideoThumbnailRefreshActive(state.videoThumbnailRefreshStatus)) {
      stopVideoThumbnailRefreshPolling();
      state.videoThumbnailRefreshPollTimer = setTimeout(pollVideoThumbnailRefreshStatus, 300);
    } else if (state.videoThumbnailRefreshStatus.status === 'completed') {
      showToast('视频缩略图已经刷新完成');
    }
  } catch (e) {
    alert('刷新视频缩略图失败: ' + ((e && e.error) || e));
  }
}

async function openThumbnailBuildWorkflow() {
  try {
    const status = await startThumbnailBuild();
    state.thumbnailBuildStatus = status || { status: 'idle', message: '当前没有缩略图任务' };
    state.thumbnailBuildCancelPending = false;
    state.thumbnailBuildOverlaySuppressed = false;
    syncThumbnailBuildOverlay(state.thumbnailBuildStatus);
    if (isThumbnailBuildActive(state.thumbnailBuildStatus)) {
      stopThumbnailBuildPolling();
      state.thumbnailBuildPollTimer = setTimeout(pollThumbnailBuildStatus, 300);
    } else if (state.thumbnailBuildStatus.status === 'completed') {
      showToast('全部缩略图已经加载完成');
    }
  } catch (e) {
    alert('加载全部缩略图失败: ' + ((e && e.error) || e));
  }
}

async function openLibraryBatchBuildWorkflow(options = {}) {
  try {
    const status = await startLibraryBatchBuild(options);
    state.libraryBatchBuildStatus = status || { status: 'idle', message: '当前没有批量扫描任务' };
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
      showToast(state.libraryBatchBuildStatus.message || copyText('app.libraryBatchScan.completed', '批量扫描已完成'));
    }
  } catch (e) {
    alert('批量扫描资源库失败: ' + ((e && e.error) || e));
  }
}

async function openLibraryBatchThumbnailBuildWorkflow(options = {}) {
  try {
    const requestOptions = {
      ...options,
      moveLegacyThumbnails: Object.prototype.hasOwnProperty.call(options, 'moveLegacyThumbnails') ? !!options.moveLegacyThumbnails : !!state.libraryBatchWorkflowMoveLegacyThumbnails,
      cleanThumbnailFiles: Object.prototype.hasOwnProperty.call(options, 'cleanThumbnailFiles') ? !!options.cleanThumbnailFiles : !!state.libraryBatchWorkflowCleanThumbnailFiles,
    };
    const status = await startLibraryBatchThumbnailBuild(requestOptions);
    state.libraryBatchThumbnailBuildStatus = status || { status: 'idle', message: '当前没有批量缩略图任务' };
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
      showToast(state.libraryBatchThumbnailBuildStatus.message || copyText('app.libraryBatchThumbnail.completed', '批量缩略图构建已完成'));
    }
  } catch (e) {
    if (options.startedByWorkflow) throw e;
    alert('批量构建全部资源库缩略图失败: ' + ((e && e.error) || e));
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
      state.thumbnailBuildStatus = status || { status: 'idle', message: '当前没有缩略图任务' };
      if (!isThumbnailBuildActive(state.thumbnailBuildStatus)) {
        state.thumbnailBuildOverlaySuppressed = false;
        state.thumbnailBuildCancelPending = false;
      }
    })
    .catch(e => {
      state.thumbnailBuildCancelPending = false;
      state.thumbnailBuildOverlaySuppressed = false;
      console.error('前台任务取消缩略图构建失败', e);
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
function showContextMenu(x, y, items) {
  closeContextMenu();
  const menu = el('div', 'context-menu');
  menu.style.left = `${x}px`;
  menu.style.top = `${y}px`;
  items.forEach(item => {
    if (item === '-') {
      const sep = el('div', 'context-menu-separator');
      menu.appendChild(sep); return;
    }
    const btn = el('button', `context-menu-item${item.danger ? ' danger' : ''}`);
    btn.innerHTML = `<span class="context-menu-icon">${item.icon || ''}</span><span>${escapeHTML(item.label)}</span>`;
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
  const navItem = (view, icon, label, active = state.view === view) =>
    `<a class="nav-item${active ? ' active' : ''}" href="#" data-view="${view}" data-label="${escapeHTML(label)}" title="${escapeHTML(label)}" aria-label="${escapeHTML(label)}">${navIconMarkup(view, active) || icon}<span class="nav-label">${escapeHTML(label)}</span></a>`;
  document.body.innerHTML = `
<div class="drawer-overlay" id="drawer-overlay"></div>
<div id="app">
  <div class="sidebar-peek-zone" id="sidebar-peek-zone" aria-hidden="true"></div>
  <button class="nav-logo nav-logo-floating" id="nav-logo-btn" type="button" title="打开资源库设置">${renderNavLogo()}</button>
  <nav class="nav" id="main-nav">
    ${navItem('timeline', icons.timeline, '时间线')}
    ${navItem('favorites', icons.favorite, '个人收藏')}
    ${navItem('random-album', icons.shuffle, '乱序相册')}
    ${navItem('albums', icons.album, '相册', state.view === 'albums' || state.view === 'album-detail')}
    ${navItem('memories', icons.memories, '回忆')}
    ${navItem('trash', icons.trash, '回收站')}
    ${navItem('settings', icons.settings, '设置')}
    <div class="nav-spacer"></div>
  </nav>
  <div class="main">
    <div class="topbar">
      <button class="hamburger" id="hamburger-btn" aria-label="菜单">
        <span></span><span></span><span></span>
      </button>
      <span class="topbar-title" id="topbar-title" data-note="${escapeHTML(topbarNoteText())}"></span>
      <div class="topbar-leading" id="topbar-leading"></div>
      <div class="topbar-meta" id="topbar-meta"></div>
      <div id="topbar-actions"></div>
    </div>
    <div class="content" id="content"></div>
  </div>
</div>
<button class="floating-search-btn" id="floating-search-btn" type="button" aria-label="搜索" title="搜索">${icons.floatingSearch || icons.search}</button>
${renderGridScaleButton()}
${renderLightbox()}
${renderSearchOverlay()}
${renderUploadModal()}
${renderCreateAlbumModal()}
${renderEditAlbumModal()}
${renderShareModal()}
${renderAlbumPickerModal()}
${renderShareListModal()}
${renderTimelineLoadAllModal()}
${renderRandomAlbumLoadAllModal()}
${renderLibraryLogoGuideModal()}`;

  bindNav();
  bindGlobal();
  syncSidebarUI();
  syncRangeProgress();
  ensureSettingsDataLoaded();
  startLibraryBuildPolling();
  void pollLibraryBatchBuildStatus();
  void pollLibraryBatchThumbnailBuildStatus();
  renderView();
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
    if (!isCompactNavLayout()) closeNavPicker();
    if ($('#lightbox')?.classList.contains('open')) {
      applyLightboxZoom();
      updateLightboxHeaderLayout();
    }
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
  interruptThumbnailBuildForForegroundTask();
  saveViewScroll();
  disconnectLoadMoreObserver();
  closeNavPicker();
  if (view === 'settings' && state.view !== 'settings') {
    state.settingsScrollRestorePending = true;
  }
  state.view = view;
  state.selected.clear();
  state.selectionAnchorID = null;
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
    return `${hours} 小时 ${mins} 分`;
  }
  if (minutes > 0) return `${minutes} 分 ${rest} 秒`;
  return `${rest} 秒`;
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
    console.error(e);
  }
}
function startLibraryBuildPolling() {
  clearTimeout(state.libraryBuildPollTimer);
  refreshLibraryBuildStatus();
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
      ? copyText('app.libraryBuild.discoveredCount', '已发现 {count} 条', { count: done })
      : copyText('app.libraryBuild.discovering', '发现媒体中'));
  $('#topbar-title').textContent = copyText('app.libraryBuild.title', '扫描与构建资源库');
  $('#topbar-meta').innerHTML = '';
  $('#topbar-actions').innerHTML = '';
  $('#content').innerHTML = `
<div class="library-build-page">
  <section class="library-build-panel">
    <div class="library-build-heading">
      <div>
        <h2>${escapeHTML(status.message || copyText('app.libraryBuild.preparing', '正在准备资源库'))}</h2>
        <p>${escapeHTML(running ? copyText('app.libraryBuild.runningDescription', 'EchoGallery 正在建立媒体索引，完成后会自动进入资源库。') : copyText('app.libraryBuild.pausedDescription', '资源库构建需要处理后才能继续。'))}</p>
      </div>
      <span class="library-build-badge">${escapeHTML(running ? copyText('app.libraryBuild.runningBadge', '运行中') : copyText('app.libraryBuild.pausedBadge', '需要处理'))}</span>
    </div>
    <div class="library-build-progress">
      <div class="library-build-progress-fill" style="width:${percent}%"></div>
    </div>
    <div class="library-build-stats">
      <div><strong>${escapeHTML(progressText)}</strong><span>${escapeHTML(copyText('app.libraryBuild.progressLabel', '处理进度'))}</span></div>
      <div><strong>${Math.round(percent)}%</strong><span>${escapeHTML(copyText('app.libraryBuild.percentLabel', '完成度'))}</span></div>
      <div><strong>${formatSecondsShort(status.elapsed_seconds)}</strong><span>${escapeHTML(copyText('app.libraryBuild.elapsedLabel', '已用时间'))}</span></div>
      <div><strong>${status.eta_seconds > 0 ? formatSecondsShort(status.eta_seconds) : escapeHTML(copyText('app.libraryBuild.etaPending', '计算中'))}</strong><span>${escapeHTML(copyText('app.libraryBuild.etaLabel', '预计剩余'))}</span></div>
      ${Number(status.pruned) > 0 ? `<div><strong>${Number(status.pruned)}</strong><span>${escapeHTML(copyText('app.libraryBuild.prunedLabel', '已清理失效媒体'))}</span></div>` : ''}
    </div>
    ${status.error ? `<div class="library-build-error">${escapeHTML(status.error)}</div>` : ''}
    <label class="library-build-exit">
      <input type="checkbox" id="library-build-exit-after" ${status.exit_after_complete ? 'checked' : ''}>
      <span>${escapeHTML(copyText('app.libraryBuild.exitAfterComplete', '构建完成后自动退出应用'))}</span>
    </label>
    <button class="btn library-build-cancel" id="library-build-cancel" type="button" ${cancellable ? '' : 'disabled'}>${escapeHTML(status.status === 'cancelling' ? copyText('app.libraryBuild.cancelling', '正在停止…') : copyText('app.libraryBuild.cancel', '停止构建'))}</button>
  </section>
</div>`;
}
function renderView() {
  const leading = $('#topbar-leading');
  if (leading) leading.innerHTML = '';
  if (isLibraryBuildBlocking()) {
    renderLibraryBuild();
    return;
  }
  switch (state.view) {
    case 'timeline':     renderTimeline();    break;
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
    copyText('app.randomAlbum.blockingTitle', '正在加载全部乱序相册媒体'),
    copyText('app.randomAlbum.blockingBatch', '正在读取第 1 批…', { batch: 1 }),
    {
    cancelText: copyText('app.randomAlbum.cancelOrganizing', '取消整理'),
    onCancel: () => {
      cancelled = true;
      if (controller) controller.abort();
      updateBlockingProgress(copyText('app.randomAlbum.cancelling', '已读取 {count} 条媒体，正在取消…', { count: allPhotos.length }));
    },
  });
  for (;;) {
    if (cancelled) throw new DOMException('Aborted', 'AbortError');
    const params = new URLSearchParams({ limit: '240' });
    params.set('seed', String(state.randomAlbumSeed));
    appendMediaKindParam(params);
    if (cursor) params.set('cursor', cursor);
    controller = new AbortController();
    const response = await fetch(`/api/media/random?${params.toString()}`, { signal: controller.signal });
    if (!response.ok) throw await response.json();
    const page = await response.json();
    const nextPhotos = page.photos || [];
    allPhotos.push(...nextPhotos);
    pageCount += 1;
    updateBlockingProgress(copyText('app.randomAlbum.organizing', '已读取 {count} 条媒体，正在整理乱序相册…', { count: allPhotos.length }));
    const loadMore = $('#load-more');
    if (loadMore) {
      loadMore.innerHTML = `<div class="spinner"></div>${escapeHTML(copyText('app.randomAlbum.collecting', '正在收集全部媒体… 已读取 {count} 条', { count: allPhotos.length }))}`;
    }
    if (!page.has_more || !page.next_cursor) break;
    cursor = page.next_cursor;
    if (pageCount % 8 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
  }
  updateBlockingProgress(copyText('app.randomAlbum.finalizing', '已读取 {count} 条媒体，正在完成整理…', { count: allPhotos.length }));
  return allPhotos;
}

async function fetchRandomAlbumPage(cursor, limit, signal) {
  const params = new URLSearchParams({ limit: String(limit), seed: String(state.randomAlbumSeed) });
  appendMediaKindParam(params);
  if (cursor) params.set('cursor', cursor);
  const response = await fetch(`/api/media/random?${params.toString()}`, { signal });
  if (!response.ok) throw await response.json();
  return response.json();
}

function renderRandomAlbumLoadAllModal() {
  return `<div class="modal-overlay timeline-load-all-modal" id="random-album-load-all-modal">
  <div class="modal" style="width:500px">
    <div class="modal-title">${escapeHTML(copyText('app.randomAlbum.modalTitle', '立即加载全部乱序相册媒体'))}</div>
    <p class="modal-copy">${escapeHTML(copyText('app.randomAlbum.modalCopy', '会先确保乱序相册已收集完整媒体列表，再批量预加载缩略图。加载期间会暂时锁定 App，减少后续浏览时被缩略图加载打断。'))}</p>
    <div class="timeline-load-progress">
      <div class="timeline-load-progress-bar" id="random-album-load-all-bar"></div>
    </div>
    <div class="timeline-load-status" id="random-album-load-all-status">${escapeHTML(copyText('app.randomAlbum.modalReady', '准备加载…'))}</div>
    <div class="modal-footer">
      <button class="btn" id="random-album-load-all-cancel">${escapeHTML(copyText('app.randomAlbum.modalCancel', '取消'))}</button>
      <button class="btn btn-primary" id="random-album-load-all-start">${escapeHTML(copyText('app.randomAlbum.modalStart', '开始加载'))}</button>
    </div>
  </div>
</div>`;
}

function openRandomAlbumLoadAllModal() {
  const modal = $('#random-album-load-all-modal');
  if (!modal || state.randomAlbumBulkLoading) return;
  if (state.randomAlbumLoading && !state.randomAlbumLoaded) {
    showToast(copyText('app.randomAlbum.collectingToast', '乱序相册正在收集媒体，请稍候'));
    return;
  }
  modal.classList.add('open');
  const total = state.randomAlbumPhotos.length || state.timelineTotal || 0;
  updateRandomAlbumLoadAllProgress(state.randomAlbumLoaded ? state.randomAlbumPhotos.length : 0, total, copyText('app.randomAlbum.modalPrepare', '准备加载全部乱序相册媒体…'));
  $('#random-album-load-all-start').disabled = false;
  $('#random-album-load-all-start').textContent = state.randomAlbumLoaded ? '预加载缩略图' : '开始加载';
  $('#random-album-load-all-cancel').disabled = false;
  $('#random-album-load-all-cancel').textContent = '取消';
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
    updateRandomAlbumLoadAllProgress(state.randomAlbumPhotos.length, state.randomAlbumPhotos.length, '正在取消…');
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
    status.textContent = message || `已加载 ${countText} 条媒体`;
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
  const uuids = (photos || [])
    .map(photo => photo && photo.uuid || '')
    .filter(Boolean)
    .slice(0, 48);
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
    startBtn.textContent = '加载中…';
  }
  if (cancelBtn) {
    cancelBtn.disabled = false;
    cancelBtn.textContent = '取消加载';
  }
  try {
    let photos = state.randomAlbumLoaded ? [...state.randomAlbumPhotos] : [];
    if (!state.randomAlbumLoaded) {
      const allPhotos = [];
      let cursor = '';
      let total = 0;
      let pageCount = 0;
      state.randomAlbumLoading = true;
      updateRandomAlbumLoadAllProgress(0, 0, '正在读取第 1 批媒体…');
      while (!state.randomAlbumBulkCancelRequested) {
        state.randomAlbumBulkAbortController = new AbortController();
        const page = await fetchRandomAlbumPage(cursor, 300, state.randomAlbumBulkAbortController.signal);
        const nextPhotos = page.photos || [];
        if (Number.isFinite(Number(page.total))) total = Number(page.total);
        allPhotos.push(...nextPhotos);
        pageCount += 1;
        updateRandomAlbumLoadAllProgress(allPhotos.length, total, `正在收集媒体… 已读取 ${allPhotos.length}${total ? ` / ${total}` : ''} 条`);
        const loadMore = $('#load-more');
        if (loadMore) loadMore.innerHTML = `<div class="spinner"></div>正在收集全部媒体… ${allPhotos.length}${total ? ` / ${total}` : ''}`;
        if (!page.has_more || !page.next_cursor) break;
        cursor = page.next_cursor;
        if (pageCount % 4 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
      }
      if (state.randomAlbumBulkCancelRequested) throw new DOMException('Aborted', 'AbortError');
      updateRandomAlbumLoadAllProgress(allPhotos.length, total || allPhotos.length, '正在整理完成…');
      photos = allPhotos;
      state.randomAlbumPhotos = photos;
      state.randomAlbumCursor = '';
      state.randomAlbumHasMore = false;
      state.randomAlbumLoaded = true;
      state.randomAlbumLoading = false;
      if (state.view === 'random-album') await renderRandomAlbumGrid();
    }

    updateRandomAlbumLoadAllProgress(0, photos.length, '正在预加载缩略图…');
    await preloadThumbnailsInBatches(
      photos,
      (loaded, total) => updateRandomAlbumLoadAllProgress(loaded, total, `正在预加载缩略图… ${loaded} / ${total}`),
      () => state.randomAlbumBulkCancelRequested,
    );
    if (state.randomAlbumBulkCancelRequested) {
      updateRandomAlbumLoadAllProgress(0, photos.length, '已取消预加载');
      showToast('已取消加载全部乱序相册');
    } else {
      updateRandomAlbumLoadAllProgress(photos.length, photos.length, `加载完成，共 ${photos.length} 条媒体`);
      showToast('全部乱序相册媒体已加载');
      setTimeout(() => $('#random-album-load-all-modal')?.classList.remove('open'), 700);
    }
  } catch (e) {
    if (e && e.name === 'AbortError') {
      updateRandomAlbumLoadAllProgress(0, state.randomAlbumPhotos.length, '已取消加载');
      showToast('已取消加载全部乱序相册');
    } else {
      console.error(e);
      updateRandomAlbumLoadAllProgress(0, state.randomAlbumPhotos.length, '加载失败，请稍后重试');
      showToast('加载全部乱序相册失败');
    }
  } finally {
    state.randomAlbumBulkLoading = false;
    state.randomAlbumLoading = false;
    state.randomAlbumBulkAbortController = null;
    state.blockingInteraction = false;
    if (startBtn) {
      startBtn.disabled = false;
      startBtn.textContent = '重新加载';
    }
    if (cancelBtn) {
      cancelBtn.disabled = false;
      cancelBtn.textContent = '关闭';
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
  $('#topbar-title').textContent = '乱序相册';
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'reshuffle-btn', icon: icons.topbarShuffle, label: '打乱相册顺序', variant: 'accent' }),
  ]);
  $('#topbar-meta').innerHTML = renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'random-load-all-btn', icon: icons.topbarLoadAll, label: '加载全部', variant: 'accent' }),
  ]);
  bindMediaKindFilterControl();
  $('#random-load-all-btn').addEventListener('click', openRandomAlbumLoadAllModal);
  $('#reshuffle-btn').addEventListener('click', async () => {
    state.randomAlbumSeed = Date.now();
    state.randomAlbumPhotos = [];
    state.randomAlbumCursor = '';
    state.randomAlbumHasMore = true;
    state.randomAlbumLoaded = false;
    state.randomAlbumViewLoaded = false;
    state.randomAlbumTotal = 0;
    $('#content').innerHTML = `<div id="random-album-wrap"></div>`;
    await renderRandomAlbumGrid();
    await loadMoreRandomAlbum();
    restoreViewScroll('random-album');
  });

  if (state.randomAlbumViewLoaded) {
    $('#content').innerHTML = `<div id="random-album-wrap"></div>`;
    await renderRandomAlbumGrid();
    requestVisibleThumbnailWarmup('random-album', randomAlbumWarmCandidates(state.randomAlbumPhotos));
    observeLoadMore('load-more', loadMoreRandomAlbum, () => state.randomAlbumHasMore && !state.randomAlbumLoading && !state.randomAlbumBulkLoading && !state.randomAlbumAutoLoadPaused);
    restoreViewScroll('random-album');
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
    restoreViewScroll('random-album');
  } catch (e) {
    $('#random-album-wrap').innerHTML = `<p style="color:var(--danger)">加载失败: ${(e && e.error) || e}</p>`;
    restoreViewScroll('random-album');
  }
}

async function renderRandomAlbumGrid() {
  const wrap = $('#random-album-wrap');
  if (!wrap) return;
  if (!state.randomAlbumPhotos.length && !state.randomAlbumHasMore && !state.randomAlbumLoading) {
    wrap.innerHTML = `<div class="empty">${icons.shuffle}<p>还没有可浏览的媒体</p></div>`;
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
    loadMore.innerHTML = `<div class="spinner"></div>正在载入乱序媒体… 已读取 ${state.randomAlbumPhotos.length}${state.randomAlbumTotal ? ` / ${state.randomAlbumTotal}` : ''} 条 <button class="btn btn-sm" id="random-page-cancel" type="button">取消</button>`;
    $('#random-page-cancel')?.addEventListener('click', () => {
      if (state.randomAlbumAbortController) state.randomAlbumAbortController.abort();
      pauseAutoLoadMore('random-album');
      armAutoLoadMoreResume('random-album');
    });
    return;
  }
  if (state.randomAlbumHasMore) {
    loadMore.innerHTML = `已随机载入 ${state.randomAlbumPhotos.length}${state.randomAlbumTotal ? ` / ${state.randomAlbumTotal}` : ''} 条媒体`;
    return;
  }
  loadMore.textContent = `已随机载入 ${state.randomAlbumPhotos.length} 条媒体`;
}

async function renderSettings() {
  $('#topbar-title').textContent = '设置';
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'settings-save-top-btn', icon: icons.topbarSave, label: '保存', variant: 'accent', disabled: !state.settingsDirty }),
  ]);
  $('#topbar-meta').innerHTML = '';
  $('#topbar-actions').innerHTML = '';
  if (!state.settingsReady) {
    $('#content').innerHTML = `<div class="load-more"><div class="spinner"></div>加载设置中…</div>`;
    await Promise.allSettled([ensureSettingsDataLoaded(), loadShareMap()]);
  }
  if (state.settingsReady && !state.shareLinksLoaded) await loadShareMap();

  if (state.view !== 'settings') return;

  try {
    renderSettingsContent();
    bindSettingsTopSaveButton();
    if (state.settingsScrollRestorePending) {
      restoreViewScroll('settings');
      state.settingsScrollRestorePending = false;
    }
    refreshMissingLibraryLogosOnce();
  } catch (e) {
    console.error('render settings failed', e);
    $('#content').innerHTML = `<div class="card settings-panel"><h3>设置加载失败</h3><p>设置项已经回退到当前可用值，你可以刷新后重试。</p><p style="color:var(--danger)">${(e && e.message) || e}</p></div>`;
    if (state.settingsScrollRestorePending) {
      restoreViewScroll('settings');
      state.settingsScrollRestorePending = false;
    }
  }
}

function renderLibrarySettingsRows() {
  const libraries = normalizeLibraries(state.serverSettings.libraries, state.serverSettings.storage_path || '');
  if (!libraries.length) {
    return `<div class="settings-empty">暂无资源库，请先添加一个路径。</div>`;
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

function renderLibrarySettingsRow(library = {}, index = 0) {
  const id = library.id || '';
  const name = library.name || `资源库 ${index + 1}`;
  const path = library.path || '';
  const logoAsset = library.logo_asset || '';
  const logoURL = library.logo_image_url || '';
  const accentColor = resolveLibraryAccentColor(library);
  const isActive = (id && id === (state.serverSettings.active_library_id || '')) || (!id && path && path === (state.serverSettings.storage_path || ''));
  return `
    <div class="settings-library-row${isActive ? ' active' : ''}" draggable="true" data-library-id="${escapeHTML(id)}" data-library-index="${index}" data-logo-asset="${escapeHTML(logoAsset)}" data-logo-preview-url="${escapeHTML(logoURL)}" data-logo-action="" style="--library-accent:${accentColor}">
      <div class="settings-library-main">
        <div class="settings-library-logo-block">
          <div class="settings-library-logo-preview"></div>
        </div>
        <div class="settings-library-fields">
          <input class="settings-library-name" type="hidden" value="${escapeHTML(name)}">
          <input class="settings-library-path" type="hidden" value="${escapeHTML(path)}">
          <input class="settings-library-accent" type="hidden" value="${accentColor}">
          <div class="settings-library-name-display">${escapeHTML(name)}</div>
          <div class="settings-library-path-display">${escapeHTML(path || '尚未设置路径')}</div>
          <div class="settings-library-meta-row">
            ${renderLibraryAccentSwatches(accentColor)}
          </div>
        </div>
      </div>
      <div class="settings-library-row-actions">
        <button class="settings-library-icon-action settings-library-edit" type="button" title="编辑资源库" aria-label="编辑资源库">${icons.libraryCardEdit}</button>
        <button class="settings-library-icon-action settings-library-remove" type="button" title="删除资源库" aria-label="删除资源库">${icons.libraryCardDelete}</button>
      </div>
    </div>`;
}

function collectLibraryInputs() {
  const rows = $$('.settings-library-row');
  return rows.map((row, index) => ({
    id: row.dataset.libraryId || '',
    name: $('.settings-library-name', row).value.trim() || `资源库 ${index + 1}`,
    path: $('.settings-library-path', row).value.trim(),
    logo_asset: row.dataset.logoAction === 'remove' ? '' : row.dataset.logoAsset || '',
    accent_color: normalizeHexColor($('.settings-library-accent', row).value) || '',
  })).filter(item => item.path);
}

function collectLibraryDrafts() {
  const rows = $$('.settings-library-row');
  if (!rows.length) return [];
  return rows.map((row, index) => ({
    id: row.dataset.libraryId || '',
    name: $('.settings-library-name', row).value.trim() || `资源库 ${index + 1}`,
    path: $('.settings-library-path', row).value.trim(),
    logo_asset: row.dataset.logoAction === 'remove' ? '' : row.dataset.logoAsset || '',
    logo_image_url: row.dataset.logoPreviewUrl || '',
    accent_color: normalizeHexColor($('.settings-library-accent', row).value) || '',
  })).filter(item => item.path);
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
  syncActiveLibrarySelect();
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
      ? `<img src="${logoURL}" alt="${escapeHTML(displayName || '资源库 Logo')}">`
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
  };
}

function createLibraryRowElement(library = {}, index = 0) {
  const wrap = el('div');
  wrap.innerHTML = renderLibrarySettingsRow(library, index).trim();
  const row = wrap.firstElementChild;
  buildLibraryLogoPreview(row, library);
  return row;
}

function openLibraryEditorModal(row = null, bindRow = null) {
  const editing = !!row;
  const index = editing ? Number(row.dataset.libraryIndex) || 0 : $$('.settings-library-row').length;
  const draft = editing
    ? libraryDraftFromRow(row)
    : {
      id: '',
      name: `资源库 ${index + 1}`,
      path: '',
      logo_asset: '',
      logo_image_url: '',
      accent_color: defaultLibraryAccentPalette[index % defaultLibraryAccentPalette.length],
    };
  const modal = el('div', 'modal-overlay library-editor-modal open');
  modal.innerHTML = `
    <div class="modal library-editor-card">
      <div class="modal-title">${editing ? '编辑资源库' : '新建资源库'}</div>
      <div class="library-editor-preview">
        <div class="settings-library-logo-preview library-editor-logo-preview${draft.logo_image_url || draft.logo_asset ? ' has-image' : ''}" id="library-editor-logo-trigger" role="button" tabindex="0" aria-label="选择资源库头像"></div>
        <div class="library-editor-preview-copy">点击头像更换图片；头像和主色会用于侧栏、资源库卡片与高亮状态。</div>
      </div>
      <div class="settings-control">
        <label for="library-editor-name"><span>资源库名称</span></label>
        <input class="input" id="library-editor-name" type="text" maxlength="32" value="${escapeHTML(draft.name || '')}" placeholder="例如：家庭照片">
      </div>
      <div class="settings-control">
        <label for="library-editor-path"><span>资源库路径</span></label>
        <input class="input" id="library-editor-path" type="text" value="${escapeHTML(draft.path || '')}" placeholder="/Users/you/Pictures/Library">
      </div>
      <div class="settings-control library-editor-inline">
        <label for="library-editor-accent-text"><span>主色</span></label>
        <div class="library-editor-accent-row">
          <input class="input library-editor-accent-text" id="library-editor-accent-text" type="text" maxlength="7" value="${escapeHTML(normalizeHexColor(draft.accent_color) || defaultLibraryAccentPalette[0])}" placeholder="#3366ff">
          <input class="library-editor-accent-picker" id="library-editor-accent" type="color" value="${normalizeHexColor(draft.accent_color) || defaultLibraryAccentPalette[0]}">
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" id="library-editor-cancel" type="button">取消</button>
        <button class="btn btn-primary" id="library-editor-save" type="button">${editing ? '保存修改' : '新建资源库'}</button>
      </div>
    </div>`;
  document.body.appendChild(modal);

  const preview = $('.library-editor-logo-preview', modal);
  const nameInput = $('#library-editor-name', modal);
  const pathInput = $('#library-editor-path', modal);
  const accentTextInput = $('#library-editor-accent-text', modal);
  const accentInput = $('#library-editor-accent', modal);
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

  const currentEditorAccent = () => normalizeHexColor(accentTextInput.value) || normalizeHexColor(accentInput.value) || defaultLibraryAccentPalette[0];
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
    preview.innerHTML = logoURL ? `<img src="${logoURL}" alt="${escapeHTML(library.name || '资源库头像')}">` : `<span class="library-avatar-placeholder">${icons.photo}</span>`;
    preview.style.borderColor = accentValue;
    preview.style.boxShadow = 'none';
    accentInput.value = accentValue;
    if (syncAccentText) accentTextInput.value = accentValue;
    applyLibraryPreviewFallback(preview, library);
  };

  const close = () => {
    modal.remove();
  };

  nameInput.addEventListener('input', updatePreview);
  pathInput.addEventListener('input', updatePreview);
  accentTextInput.addEventListener('input', () => {
    const color = normalizeHexColor(accentTextInput.value);
    if (color) accentInput.value = color;
    updatePreview();
  });
  accentInput.addEventListener('input', () => {
    accentTextInput.value = normalizeHexColor(accentInput.value) || accentInput.value;
    updatePreview();
  });
  const openLogoPicker = () => logoInput.click();
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
    const library = {
      id: draft.id || '',
      name: nameInput.value.trim() || `资源库 ${index + 1}`,
      path: pathInput.value.trim(),
      logo_asset: logoAction === 'remove' ? '' : logoAsset,
      logo_image_url: logoPreviewUrl,
      accent_color: normalizeHexColor(accentTextInput.value || accentInput.value) || defaultLibraryAccentPalette[index % defaultLibraryAccentPalette.length],
    };
    if (!library.path) {
      showToast('请填写资源库路径');
      return;
    }
    const nextRow = createLibraryRowElement(library, index);
    nextRow.dataset.logoAction = logoAction;
    nextRow._pendingLogoFile = pendingFile;
    nextRow._pendingLogoNeedsCrop = pendingLogoNeedsCrop;
    if (editing && row.classList.contains('active')) {
      state.serverSettings.active_library_id = library.id || '';
      state.serverSettings.storage_path = library.path;
    }
    if (editing) row.replaceWith(nextRow);
    else {
      $('#settings-library-list .settings-empty')?.remove();
      $('#settings-library-list').appendChild(nextRow);
    }
    if (typeof bindRow === 'function') bindRow(nextRow);
    reindexLibrarySettingsRows();
    syncActiveLibrarySelect();
    applyLibraryBranding();
    setSettingsDirty();
    try {
      await withButtonBusy($('#library-editor-save', modal), '保存中…', async () => {
        await persistSettingsWithLibraryAssets();
      });
      setSettingsDirty(false);
      close();
      showToast('资源库已保存到 config.json');
      renderSettings();
    } catch (e) {
      alert('保存资源库失败: ' + ((e && e.error) || e.message || e));
    }
  });
  updatePreview({ syncAccentText: true });
  nameInput.focus();
}

function syncActiveLibrarySelect() {
  const select = $('#settings-active-library');
  const libraries = collectLibraryDrafts();
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
      return `<option value="${escapeHTML(library.id || library.path)}" ${selected ? 'selected' : ''}>${library.name}</option>`;
    }).join('') || '<option value="">请先添加资源库</option>';
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
    if (data.updated > 0) showToast(`已更新 ${data.updated} 个资源库头像`);
    if (state.view === 'settings' && !state.settingsDirty) renderSettings();
  } catch (e) {
    console.warn('批量更新资源库头像失败', e);
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
  if (saveBtn) {
    saveBtn.disabled = !state.settingsDirty;
    saveBtn.onclick = async () => {
      try {
        collectSettingsFormState();
        const payload = buildSettingsPayload();
        const needsRestart = settingsPayloadRequiresRestart(payload, state.savedServerSettings || state.serverSettings);
        const resp = await withButtonBusy(saveBtn, '保存中…', () => persistSettingsWithLibraryAssets());
        setSettingsDirty(false);
        if (needsRestart) {
          showToast('设置已保存，正在重启服务…', 2600);
          await withButtonBusy(saveBtn, '重启中…', () => restartApp());
          setTimeout(() => location.reload(), 1800);
          return;
        }
        showToast((resp && resp.message) || '设置已保存');
        renderSettings();
      } catch (e) {
        alert('保存失败: ' + ((e && e.error) || e.message || e));
      }
    };
  }
}

function collectSettingsFormState() {
  state.serverSettings.libraries = collectLibraryInputs();
  if (!state.serverSettings.libraries.length) throw new Error('请至少保留一个资源库');
  state.serverSettings.port = parseInt($('#settings-port')?.value, 10) || state.serverSettings.port || 8080;
  const activeValue = $('#settings-active-library')?.value.trim() || state.serverSettings.active_library_id || state.serverSettings.storage_path || '';
  const activeLibrary = resolveActiveLibrary(state.serverSettings.libraries, activeValue, state.serverSettings.storage_path || '');
  state.serverSettings.active_library_id = activeLibrary && activeLibrary.id || '';
  state.serverSettings.storage_path = activeLibrary && activeLibrary.path || state.serverSettings.storage_path || state.serverSettings.libraries[0].path;
  state.serverSettings.thumbnail_dir = $('#settings-thumbnail-dir').value.trim();
  state.serverSettings.thumbnail_size = 512;
  state.serverSettings.trash_dir = $('#settings-trash-dir').value.trim();
  state.serverSettings.use_system_player = !!state.serverSettings.use_system_player;
  state.videoSectionMinMinutes = Math.min(240, Math.max(1, Number($('#settings-video-section-min')?.value) || state.videoSectionMinMinutes || 10));
  state.videoAutoplayNext = !!$('#settings-video-autoplay-next')?.checked;
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

function updateVideoBookmarkThumbIndicators(photoOrId = null) {
  const targetId = photoOrId && typeof photoOrId === 'object' ? photoOrId.id : photoOrId;
  const thumbSelector = targetId
    ? `.photo-thumb[data-id="${targetId}"]`
    : '.photo-thumb[data-kind="video"]';
  document.querySelectorAll(thumbSelector).forEach(thumb => {
    const count = getVideoBookmarkCountByKey(thumb.dataset.id);
    const existing = thumb.querySelector('.video-bookmark-badge');
    if (count > 0 && !existing) {
      thumb.insertAdjacentHTML('beforeend', `<span class="video-bookmark-badge" title="视频书签 ${count}/10" aria-label="视频书签 ${count}/10">${icons.bookmark}<span class="video-bookmark-count">${count}</span></span>`);
    } else if (count > 0 && existing) {
      existing.title = `视频书签 ${count}/10`;
      existing.setAttribute('aria-label', `视频书签 ${count}/10`);
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
  return `<label class="settings-toggle" for="${id}">
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

function persistLibraryBatchWorkflowPrefs() {
  localStorage.setItem(libraryBatchWorkflowEnabledStorageKey, state.libraryBatchWorkflowEnabled ? '1' : '0');
  localStorage.setItem(libraryBatchWorkflowAggressiveStorageKey, state.libraryBatchWorkflowAggressive ? '1' : '0');
  localStorage.setItem(libraryBatchWorkflowMoveLegacyStorageKey, state.libraryBatchWorkflowMoveLegacyThumbnails ? '1' : '0');
  localStorage.setItem(libraryBatchWorkflowCleanFilesStorageKey, state.libraryBatchWorkflowCleanThumbnailFiles ? '1' : '0');
}

function setLibraryBatchWorkflowPhase(phase = '') {
  state.libraryBatchWorkflowPhase = phase || '';
  if (state.libraryBatchWorkflowPhase) localStorage.setItem(libraryBatchWorkflowPhaseStorageKey, state.libraryBatchWorkflowPhase);
  else localStorage.removeItem(libraryBatchWorkflowPhaseStorageKey);
}

function clearLibraryBatchWorkflowPhase() {
  setLibraryBatchWorkflowPhase('');
}

function libraryBatchWorkflowActive() {
  return state.libraryBatchWorkflowPhase === 'scan' || state.libraryBatchWorkflowPhase === 'thumbnails';
}

function libraryBatchWorkflowOptionLabels() {
  const labels = [];
  if (state.libraryBatchWorkflowMoveLegacyThumbnails) labels.push('整理旧版资源库');
  if (state.libraryBatchWorkflowCleanThumbnailFiles) labels.push('清理文件');
  return labels;
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
    case 'pending': return '等待中';
    case 'scanning': return '扫描中';
    case 'building': return '缩略图';
    case 'completed': return '已完成';
    case 'failed': return '失败';
    case 'cancelled': return '已取消';
    case 'running': return '运行中';
    case 'cancelling': return '取消中';
    default: return '空闲';
  }
}

function libraryBatchWorkflowSelectionStatus() {
  const scan = state.libraryBatchBuildStatus || {};
  const thumbnails = state.libraryBatchThumbnailBuildStatus || {};
  if (state.libraryBatchWorkflowPhase === 'thumbnails') return thumbnails;
  if (state.libraryBatchWorkflowPhase === 'scan') return scan;
  if (isLibraryBatchBuildActive(scan)) return scan;
  if (isLibraryBatchThumbnailBuildActive(thumbnails)) return thumbnails;
  if (Array.isArray(scan.libraries) && scan.libraries.length) return scan;
  if (Array.isArray(thumbnails.libraries) && thumbnails.libraries.length) return thumbnails;
  return scan || thumbnails || {};
}

function libraryBatchWorkflowProgressPercent() {
  const scan = state.libraryBatchBuildStatus || {};
  const thumbnails = state.libraryBatchThumbnailBuildStatus || {};
  const scanPercent = Math.max(0, Math.min(100, Number(scan.current_percent) || 0));
  const thumbnailPercent = Math.max(0, Math.min(100, Number(thumbnails.current_percent) || 0));
  if (state.libraryBatchWorkflowEnabled || libraryBatchWorkflowActive()) {
    if (state.libraryBatchWorkflowPhase === 'thumbnails') return Math.min(100, 50 + thumbnailPercent / 2);
    if (state.libraryBatchWorkflowPhase === 'scan' || isLibraryBatchBuildActive(scan)) return scanPercent / 2;
    if (String(scan.status || '') === 'completed' && isLibraryBatchThumbnailBuildActive(thumbnails)) return Math.min(100, 50 + thumbnailPercent / 2);
    if (String(scan.status || '') === 'completed' && String(thumbnails.status || '') === 'completed') return 100;
  }
  if (isLibraryBatchThumbnailBuildActive(thumbnails)) return thumbnailPercent;
  if (isLibraryBatchBuildActive(scan)) return scanPercent;
  if (String(thumbnails.status || '') === 'completed') return 100;
  if (String(scan.status || '') === 'completed') return state.libraryBatchWorkflowEnabled ? 50 : 100;
  return Math.max(scanPercent, thumbnailPercent);
}

function libraryBatchWorkflowProgressCopy() {
  const status = libraryBatchWorkflowSelectionStatus();
  if (isLibraryBatchBuildActive(state.libraryBatchBuildStatus)) {
    return renderLibraryBatchTaskProgress(state.libraryBatchBuildStatus, '顺序扫描全部资源库');
  }
  if (isLibraryBatchThumbnailBuildActive(state.libraryBatchThumbnailBuildStatus)) {
    return renderLibraryBatchTaskProgress(state.libraryBatchThumbnailBuildStatus, '顺序为全部资源库补全缩略图');
  }
  return renderLibraryBatchTaskProgress(status, '等待开始批量任务');
}

function renderLibraryBatchWorkflowPanel() {
  const running = libraryBatchWorkflowActive();
  const controlsDisabled = running || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive();
  const hasSelection = libraryBatchWorkflowHasSelection();
  const selectionStatus = libraryBatchWorkflowSelectionStatus();
  const progressPercent = libraryBatchWorkflowProgressPercent();
  const currentPhase = state.libraryBatchWorkflowPhase === 'thumbnails' ? '批量缩略图阶段' : '批量扫描阶段';
  const optionLabels = libraryBatchWorkflowOptionLabels();
  const optionCopy = optionLabels.length ? `；附加处理：${optionLabels.join('、')}` : '';
  const statusCopy = running
    ? `当前处于${currentPhase}${state.libraryBatchWorkflowAggressive ? '，并已启用高资源模式' : ''}${optionCopy}`
    : (state.libraryBatchWorkflowEnabled
      ? `点击后会先扫描，再自动接着构建缩略图${state.libraryBatchWorkflowAggressive ? '，并临时启用高资源模式' : ''}${optionCopy}`
      : '关闭后保持现有两段式手动流程');
  return `<div class="settings-batch-build-panel settings-batch-build-panel-master">
    <div class="settings-batch-build-head">
      <div>
        <h3>批量工作流</h3>
        <span>${escapeHTML(statusCopy)}</span>
      </div>
      <span class="settings-batch-build-badge ${running ? 'active' : ''}">${escapeHTML(running ? '串行运行中' : '可选')}</span>
    </div>
    <div class="settings-batch-workflow-progress" aria-label="批量工作流进度">
      <span style="width:${progressPercent}%"></span>
    </div>
    <div class="settings-batch-build-meta">
      <span>${escapeHTML(libraryBatchWorkflowProgressCopy())}</span>
      <span>${Math.round(progressPercent)}%</span>
    </div>
    <div class="settings-batch-build-selection">
      <strong>资源库范围</strong>
      <div class="settings-batch-build-selection-list settings-batch-workflow-selection-list ${controlsDisabled ? 'is-disabled' : ''}">${renderLibraryBatchSelectionRows(selectionStatus, 'settings-library-batch-workflow-selection', controlsDisabled)}</div>
    </div>
    <div class="settings-group settings-batch-build-master-toggles ${controlsDisabled ? 'is-disabled' : ''}">
      ${renderSettingsToggle('settings-library-batch-workflow-select-all', '批量范围全选', libraryBatchWorkflowAllSelected(), '', controlsDisabled)}
      ${renderSettingsToggle('settings-library-batch-workflow-enabled', '扫描后自动构建缩略图', state.libraryBatchWorkflowEnabled, '适合一次性把多个资源库扫描并补全缩略图。', controlsDisabled)}
      ${renderSettingsToggle('settings-library-batch-workflow-aggressive', '高资源无人值守模式', state.libraryBatchWorkflowAggressive, '临时提高扫描与缩略图并发，优先缩短总耗时。', controlsDisabled)}
      ${renderSettingsToggle('settings-library-batch-workflow-move-legacy', '整理旧版资源库', state.libraryBatchWorkflowMoveLegacyThumbnails, '为旧资源库补齐稳定 ID，并把旧缩略图迁移到按资源库 ID 隔离的新目录。', controlsDisabled)}
      ${renderSettingsToggle('settings-library-batch-workflow-clean-files', '清理文件', state.libraryBatchWorkflowCleanThumbnailFiles, '清理旧 JPG、preview、build-preview 等遗留缩略图文件；不会删除其它资源库的缩略图目录。', controlsDisabled)}
    </div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-start-library-batch-workflow-btn" type="button" ${controlsDisabled || !hasSelection ? 'disabled' : ''}>${state.libraryBatchWorkflowEnabled ? '一键开始扫描并构建缩略图' : '开始批量扫描'}</button>
      <button class="btn" id="settings-cancel-library-batch-workflow-btn" type="button" ${running || isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive() ? '' : 'disabled'}>${running ? '取消当前工作流' : '取消当前阶段'}</button>
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
      alert('取消批量扫描失败: ' + ((e && e.error) || e));
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
      alert('取消批量缩略图失败: ' + ((e && e.error) || e));
    }
    syncLibraryBatchWorkflowPanel();
  }
}

async function openLibraryBatchFullWorkflow() {
  try {
    await syncLibraryBatchWorkflowSelections();
    setLibraryBatchWorkflowPhase(state.libraryBatchWorkflowEnabled ? 'scan' : '');
    await openLibraryBatchBuildWorkflow({ aggressive: state.libraryBatchWorkflowAggressive });
  } catch (e) {
    clearLibraryBatchWorkflowPhase();
    alert('启动批量工作流失败: ' + ((e && e.error) || e.message || e));
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

function allBatchSelectableLibraryValues(status) {
  return allBatchSelectableLibraries(status).map(library => String(library.id || library.path || '').trim()).filter(Boolean);
}

function libraryBatchStatusAllSelected(status) {
  const rows = allBatchSelectableLibraries(status);
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
  const rows = allBatchSelectableLibraries(status);
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
      name: library.name || row.name || `资源库 ${index + 1}`,
      path,
      status: row.status || 'pending',
      message: row.message || '',
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
  if (!rows.length) return '<div class="settings-batch-build-empty">当前没有可选资源库。</div>';
  return rows.map((library, index) => {
    const value = String(library.id || library.path || '').trim();
    const compareValue = usingIDs ? value : String(library.path || '').trim();
    const checked = selectionConfigured ? selected.has(compareValue) : true;
    return `<label class="settings-batch-build-select-row ${checked ? 'is-selected' : ''}">
      <span class="settings-toggle-switch settings-batch-build-checkbox">
        <input type="checkbox" name="${escapeHTML(inputName)}" value="${escapeHTML(value)}" ${checked ? 'checked' : ''} ${disabled ? 'disabled' : ''}>
        <span class="settings-toggle-slider" aria-hidden="true"></span>
      </span>
      <span class="settings-batch-build-select-copy">
        <strong>${escapeHTML(library.name || `资源库 ${index + 1}`)}</strong>
        <span>${escapeHTML(library.path || '')}</span>
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
  let phase = '扫描';
  switch (String(status && status.current_phase || '')) {
    case 'thumbnails':
      phase = '缩略图';
      break;
    case 'migrate':
      phase = '整理旧版资源库';
      break;
    case 'cleanup':
      phase = '清理文件';
      break;
    case 'maintenance':
      phase = '整理缩略图目录';
      break;
    default:
      phase = '扫描';
      break;
  }
  const parts = [];
  if (currentLibrary) parts.push(`当前：${currentLibrary}`);
  if (totalLibraries > 0) parts.push(`资源库 ${current} / ${totalLibraries}`);
  if (currentTotal > 0) {
    parts.push(`${phase} ${currentDone} / ${currentTotal}`);
  } else if (currentLibrary) {
    parts.push(phase);
  } else if (idleLabel) {
    parts.push(idleLabel);
  }
  parts.push(`完成 ${completed}`);
  if (failed > 0) parts.push(`失败 ${failed}`);
  if (status && status.low_resource_mode) parts.push('低资源模式');
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
    `总计 ${Math.max(0, Number(status.total_libraries) || libraries.length || 0)}`,
    `完成 ${Math.max(0, Number(status.completed_libraries) || 0)}`,
    `失败 ${Math.max(0, Number(status.failed_libraries) || 0)}`,
  ].join(' · ');
  const list = libraries.length ? libraries.map((library, index) => {
    const rowStatus = String(library.status || 'pending');
    const current = currentName && currentName === String(library.name || '').trim();
    const detailParts = [];
    if (Number(library.imported) > 0) detailParts.push(`导入 ${Number(library.imported)}`);
    if (Number(library.skipped) > 0) detailParts.push(`跳过 ${Number(library.skipped)}`);
    if (Number(library.pruned) > 0) detailParts.push(`清理 ${Number(library.pruned)}`);
    if (Number(library.failed) > 0) detailParts.push(`失败 ${Number(library.failed)}`);
    const meta = detailParts.length ? detailParts.join(' · ') : (library.message || library.path || '');
    return `<div class="settings-batch-build-row ${current ? 'active' : ''} is-${escapeHTML(rowStatus)}">
      <div class="settings-batch-build-row-main">
        <strong>${escapeHTML(library.name || `资源库 ${index + 1}`)}</strong>
        <span>${escapeHTML(meta)}</span>
      </div>
      <em>${escapeHTML(libraryBatchBuildStatusLabel(rowStatus))}</em>
    </div>`;
  }).join('') : '<div class="settings-batch-build-empty">当前没有批量扫描记录。</div>';
  const progressPercent = Math.max(0, Math.min(100, Number(status.current_percent) || 0));
  return `<div class="settings-batch-build-panel">
    <div class="settings-batch-build-head">
      <div>
        <strong>${escapeHTML(status.message || '批量扫描全部资源库')}</strong>
        <span>${escapeHTML(summary)}</span>
      </div>
      <span class="settings-batch-build-badge ${active ? 'active' : ''}">${escapeHTML(active ? '运行中' : libraryBatchBuildStatusLabel(status.status || 'idle'))}</span>
    </div>
    <div class="settings-batch-build-meta">
      <span>${escapeHTML(renderLibraryBatchTaskProgress(status, '顺序扫描全部资源库'))}</span>
      <span>${escapeHTML(status.aggressive_mode ? '高资源模式' : (status.low_resource_mode ? '低资源模式已开启' : '标准资源模式'))}</span>
    </div>
    <div class="settings-batch-build-progress"><span style="width:${progressPercent}%"></span></div>
    <div class="settings-batch-build-selection">
      <strong>批量扫描范围</strong>
      <div class="settings-batch-build-selection-list">${renderLibraryBatchSelectionRows(status, 'settings-library-batch-build-selection')}</div>
    </div>
    <label class="settings-batch-build-exit">
      <input id="settings-library-batch-build-exit" type="checkbox" ${status.exit_after_complete ? 'checked' : ''}>
      <span>全部完成后退出应用</span>
    </label>
    <div class="settings-batch-build-list">${list}</div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-build-all-libraries-btn" type="button" ${(active || otherActive || !hasSelection) ? 'disabled' : ''}>${active ? '批量扫描进行中' : (canResume ? '继续批量扫描' : '批量扫描全部资源库')}</button>
      <button class="btn" id="settings-cancel-all-libraries-btn" type="button" ${active ? '' : 'disabled'}>${state.libraryBatchBuildCancelPending ? copyText('app.libraryBatchScan.cancelling', '正在取消…') : copyText('app.libraryBatchScan.cancel', '取消批量扫描')}</button>
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
        alert('保存批量扫描勾选失败: ' + ((e && e.error) || e));
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
      alert('取消批量扫描失败: ' + ((e && e.error) || e));
    }
  });
  $('#settings-library-batch-build-exit')?.addEventListener('change', async e => {
    try {
      state.libraryBatchBuildStatus = await setLibraryBatchBuildExitAfterComplete(e.target.checked);
      syncLibraryBatchBuildPanel();
    } catch (err) {
      e.target.checked = !e.target.checked;
      alert('设置失败: ' + ((err && err.error) || err));
    }
  });
}

function syncLibraryBatchWorkflowPanel() {
  const container = $('#settings-library-batch-workflow');
  if (!container) return;
  container.innerHTML = renderLibraryBatchWorkflowPanel();
  $$('input[name="settings-library-batch-workflow-selection"]').forEach(input => {
    input.addEventListener('change', async () => {
      if (isLibraryBatchBuildActive() || isLibraryBatchThumbnailBuildActive()) {
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
        alert('保存批量范围失败: ' + ((e && e.error) || e));
        syncLibraryBatchWorkflowPanel();
      }
    });
  });
  $('#settings-library-batch-workflow-select-all')?.addEventListener('change', async e => {
    try {
      await setLibraryBatchWorkflowSelectionAll(!!e.target.checked);
      syncLibraryBatchWorkflowPanel();
      syncLibraryBatchBuildPanel();
      syncLibraryBatchThumbnailBuildPanel();
    } catch (err) {
      alert('保存批量范围失败: ' + ((err && err.error) || err));
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
  $('#settings-library-batch-workflow-move-legacy')?.addEventListener('change', e => {
    state.libraryBatchWorkflowMoveLegacyThumbnails = !!e.target.checked;
    persistLibraryBatchWorkflowPrefs();
    syncLibraryBatchWorkflowPanel();
  });
  $('#settings-library-batch-workflow-clean-files')?.addEventListener('change', e => {
    state.libraryBatchWorkflowCleanThumbnailFiles = !!e.target.checked;
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
    `总计 ${Math.max(0, Number(status.total_libraries) || libraries.length || 0)}`,
    `完成 ${Math.max(0, Number(status.completed_libraries) || 0)}`,
    `失败 ${Math.max(0, Number(status.failed_libraries) || 0)}`,
  ].join(' · ');
  const list = libraries.length ? libraries.map((library, index) => {
    const rowStatus = String(library.status || 'pending');
    const current = currentName && currentName === String(library.name || '').trim();
    const detailParts = [];
    if (Number(library.imported) > 0) detailParts.push(`已迁移 ${Number(library.imported)}`);
    if (Number(library.pruned) > 0) detailParts.push(`已清理 ${Number(library.pruned)}`);
    if (Number(library.generated) > 0) detailParts.push(`新生成 ${Number(library.generated)}`);
    if (Number(library.skipped) > 0) detailParts.push(`已存在 ${Number(library.skipped)}`);
    if (Number(library.failed) > 0) detailParts.push(`失败 ${Number(library.failed)}`);
    const meta = detailParts.length ? detailParts.join(' · ') : (library.message || library.path || '');
    return `<div class="settings-batch-build-row ${current ? 'active' : ''} is-${escapeHTML(rowStatus)}">
      <div class="settings-batch-build-row-main">
        <strong>${escapeHTML(library.name || `资源库 ${index + 1}`)}</strong>
        <span>${escapeHTML(meta)}</span>
      </div>
      <em>${escapeHTML(libraryBatchBuildStatusLabel(rowStatus))}</em>
    </div>`;
  }).join('') : '<div class="settings-batch-build-empty">当前没有批量缩略图记录。</div>';
  const progressPercent = Math.max(0, Math.min(100, Number(status.current_percent) || 0));
  const optionParts = [];
  if (status.move_legacy_thumbnails) optionParts.push('整理旧版资源库');
  if (status.clean_thumbnail_files) optionParts.push('清理文件');
  return `<div class="settings-batch-build-panel">
    <div class="settings-batch-build-head">
      <div>
        <strong>${escapeHTML(status.message || '批量构建全部资源库缩略图')}</strong>
        <span>${escapeHTML(summary)}</span>
      </div>
      <span class="settings-batch-build-badge ${active ? 'active' : ''}">${escapeHTML(active ? '运行中' : libraryBatchBuildStatusLabel(status.status || 'idle'))}</span>
    </div>
    <div class="settings-batch-build-meta">
      <span>${escapeHTML(renderLibraryBatchTaskProgress(status, '顺序为全部资源库补全缩略图'))}</span>
      <span>${escapeHTML([status.aggressive_mode ? '高资源模式' : (status.low_resource_mode ? '低资源模式已开启' : '标准资源模式'), ...optionParts].filter(Boolean).join(' · '))}</span>
    </div>
    <div class="settings-batch-build-progress"><span style="width:${progressPercent}%"></span></div>
    <div class="settings-batch-build-selection">
      <strong>批量缩略图范围</strong>
      <div class="settings-batch-build-selection-list">${renderLibraryBatchSelectionRows(status, 'settings-library-batch-thumbnail-build-selection')}</div>
    </div>
    <label class="settings-batch-build-exit">
      <input id="settings-library-batch-thumbnail-build-exit" type="checkbox" ${status.exit_after_complete ? 'checked' : ''}>
      <span>全部完成后退出应用</span>
    </label>
    <div class="settings-batch-build-list">${list}</div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-build-all-library-thumbnails-btn" type="button" ${(active || otherActive || !hasSelection) ? 'disabled' : ''}>${active ? '批量缩略图进行中' : (canResume ? '继续批量缩略图' : '批量构建全部资源库缩略图')}</button>
      <button class="btn" id="settings-cancel-all-library-thumbnails-btn" type="button" ${active ? '' : 'disabled'}>${state.libraryBatchThumbnailBuildCancelPending ? copyText('app.libraryBatchThumbnail.cancelling', '正在取消…') : copyText('app.libraryBatchThumbnail.cancel', '取消批量缩略图')}</button>
    </div>
  </div>`;
}

function syncLibraryBatchThumbnailBuildPanel() {
  const container = $('#settings-library-batch-thumbnail-build');
  if (!container) return;
  container.innerHTML = renderLibraryBatchThumbnailBuildPanel();
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
        alert('保存批量缩略图勾选失败: ' + ((e && e.error) || e));
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
      alert('取消批量缩略图失败: ' + ((e && e.error) || e));
    }
  });
  $('#settings-library-batch-thumbnail-build-exit')?.addEventListener('change', async e => {
    try {
      state.libraryBatchThumbnailBuildStatus = await setLibraryBatchThumbnailBuildExitAfterComplete(e.target.checked);
      syncLibraryBatchThumbnailBuildPanel();
    } catch (err) {
      e.target.checked = !e.target.checked;
      alert('设置失败: ' + ((err && err.error) || err));
    }
  });
}

function renderSettingsContent() {
  const users = (state.serverSettings.users || []).map(name => `<span class="settings-chip">${name}</span>`).join('');
  const activeAccent = resolveLibraryAccentColor(currentLibraryBrand() || {});
  const activeAccentText = readableTextColorForBackground(activeAccent);
  const thumbnailBuildSize = 512;
  $('#content').innerHTML = `
<div class="settings-layout">
  <section class="card settings-panel settings-panel-application">
    <h3>资源库</h3>
    <div class="settings-application-grid">
      <div class="settings-control settings-control-panel settings-control-wide">
        <label><span>资源库管理</span><span>为每个资源库配一个名字，可在这里增删改</span></label>
        <div class="settings-library-list" id="settings-library-list">${renderLibrarySettingsRows()}</div>
      </div>
    </div>
    <div class="settings-actions settings-actions-fill">
      <button class="btn" id="settings-build-all-thumbs-btn" type="button">加载全部缩略图</button>
    </div>
    <button class="btn btn-primary settings-floating-add-library" id="settings-add-library-btn" type="button" style="--highlight-bg:${activeAccent};--highlight-text:${activeAccentText}">${icons.libraryCreate} 添加资源库</button>
  </section>

  <section class="card settings-panel settings-panel-batch-workflow">
    <div id="settings-library-batch-workflow">${renderLibraryBatchWorkflowPanel()}</div>
  </section>

  <section class="card settings-panel settings-panel-display">
    <h3>浏览显示</h3>
    <p>这些设置现在统一保存到 config.json，调整后会立即生效。</p>
    <div class="settings-group">
      <div class="settings-control">
        <label for="settings-grid-gap"><span>图像间距</span><span id="settings-grid-gap-value">${state.gridGap}px</span></label>
        <input class="input" id="settings-grid-gap" type="range" min="0" max="24" step="1" value="${state.gridGap}">
      </div>
      <div class="settings-control">
        <label for="settings-thumb-radius"><span>图像圆角</span><span id="settings-thumb-radius-value">${state.thumbRadius}px</span></label>
        <input class="input" id="settings-thumb-radius" type="range" min="0" max="24" step="1" value="${state.thumbRadius}">
      </div>
      <div class="settings-control settings-control-disabled" aria-disabled="true">
        <label for="settings-thumbnail-size"><span>缩略图生成大小</span><span id="settings-thumbnail-size-value">${thumbnailBuildSize}px</span></label>
        <input class="input" id="settings-thumbnail-size" type="range" min="512" max="512" step="1" value="512" disabled>
      </div>
      ${renderSettingsToggle('settings-low-resource-mode', '低资源占用模式', !!state.serverSettings.low_resource_mode, '降低扫描与缩略图并发，更适合挂机和机械硬盘场景。')}
      <div class="settings-control">
        <label for="settings-thumbnail-dir"><span>缩略图目录</span><span>建议放到空间更充足的磁盘，重启后生效</span></label>
        <input class="input" id="settings-thumbnail-dir" type="text" value="${escapeHTML(state.serverSettings.thumbnail_dir || '')}">
      </div>
      <div class="settings-control">
        <label for="settings-trash-dir"><span>回收站目录</span><span>永久删除时移动到这里</span></label>
        <input class="input" id="settings-trash-dir" type="text" value="${escapeHTML(state.serverSettings.trash_dir || '')}">
      </div>
    </div>
  </section>

  <section class="card settings-panel settings-panel-playback">
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
      ${renderSettingsToggle('settings-slideshow-loop', '幻灯片循环播放', state.slideshowLoop)}
      <div class="settings-control">
        <label for="settings-slideshow-mode"><span>默认幻灯片顺序</span><span>${state.slideshowMode === 'random' ? '随机' : '顺序'}</span></label>
        <select class="input" id="settings-slideshow-mode">
          <option value="random" ${state.slideshowMode === 'random' ? 'selected' : ''}>随机播放</option>
          <option value="sequential" ${state.slideshowMode === 'sequential' ? 'selected' : ''}>顺序播放</option>
        </select>
      </div>
      <div class="settings-control settings-control-wide">
        <label for="settings-player-keymap"><span>IINA 快捷键映射</span><span>支持 .conf 风格</span></label>
        <textarea class="input settings-textarea" id="settings-player-keymap" rows="12"></textarea>
      </div>
      <div class="settings-actions">
        <button class="btn" id="settings-reset-keymap-btn">恢复默认快捷键</button>
      </div>
      ${renderSettingsToggle('settings-autoplay-video', '视频打开后自动播放', state.experimentalAutoplayVideo)}
      ${renderSettingsToggle('settings-video-autoplay-next', '视频播放结束后自动播放下一个视频', state.videoAutoplayNext)}
      <div class="settings-control">
        <label for="settings-video-section-min"><span>视频小节阈值</span><span id="settings-video-section-min-value">${state.videoSectionMinMinutes} 分钟</span></label>
        <input class="input" id="settings-video-section-min" type="range" min="1" max="240" step="1" value="${state.videoSectionMinMinutes}">
      </div>
      ${renderSettingsToggle('settings-continue-last-video-position', '继续上次播放位置', state.continueLastVideoPosition)}
      ${renderSettingsToggle('settings-prefetch-neighbors', '预加载前后相邻媒体', state.experimentalPrefetchNeighbors)}
      <div class="settings-actions">
        <button class="btn" id="settings-refresh-video-thumbs-btn">刷新视频缩略图</button>
        <button class="btn" id="settings-backfill-exif-btn" type="button">补录旧照片 EXIF</button>
      </div>
    </div>
  </section>

  <section class="card settings-panel settings-panel-experimental">
    <h3>实验性功能</h3>
    <p>这些功能还在打磨中，可能会继续调整行为，开启状态会写入 config.json。</p>
    <div class="settings-group">
      ${renderSettingsToggle('settings-exp-restore-last-view', '启动时恢复上次浏览页面', state.experimentalRestoreLastView)}
      <div class="settings-static">
        <strong>快捷切页</strong>
        <span>支持 macOS Option + 1-6、Windows Alt + 1-6 切换到时间线、个人收藏、乱序相册、相册、回收站、设置。</span>
      </div>
    </div>
  </section>

  <section class="card settings-panel settings-panel-update">
    <h3>更新</h3>
    <p>软件更新会用于获取新功能、性能优化与问题修复。EchoGallery 会尽量在更新前保留控制权，不在未确认的情况下修改本地程序。</p>
    <div class="settings-group">
      <div class="settings-static settings-update-note">
        <strong>软件更新</strong>
        <span>检查更新会对比 GitHub Release 中的最新版本；开启自动更新后，EchoGallery 可在发现新版本时提示安装。</span>
      </div>
      <div class="settings-actions">
        <button class="btn" id="settings-check-update-btn" type="button">检查更新</button>
      </div>
      ${renderSettingsToggle('settings-auto-update', '自动更新', state.autoUpdateEnabled)}
    </div>
  </section>

  <section class="card settings-panel settings-panel-app">
    <h3>应用配置</h3>
    <p>这些入口从侧边栏移动到这里，保持浏览界面更轻盈。</p>
    <div class="settings-actions settings-app-actions">
      <a class="btn" id="settings-github-btn" href="https://github.com/in4pira10n/EchoGallery" target="_blank" rel="noopener noreferrer">${icons.github} 项目仓库</a>
      <button class="btn" id="theme-btn" type="button"><span class="theme-icon">${document.documentElement.dataset.theme === 'dark' ? icons.sun : icons.moon}</span> 模式切换</button>
      <button class="btn" id="logout-btn" type="button">${icons.logout} 退出登录</button>
      <button class="btn btn-danger" id="shutdown-btn" type="button">${icons.shutdown} 退出程序</button>
    </div>
  </section>

  <section class="card settings-panel settings-panel-share">
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
  $('#settings-trash-dir').value = state.serverSettings.trash_dir || '';
  $('#settings-player-keymap').value = state.playerKeymapSource || '';
  syncRangeProgress($('#content'));

  $('#settings-add-library-btn').addEventListener('click', () => openLibraryEditorModal(null, bindLibraryRow));
  $('#settings-build-all-thumbs-btn')?.addEventListener('click', openThumbnailBuildWorkflow);
  syncLibraryBatchWorkflowPanel();

  function bindLibraryRow(row) {
    row.addEventListener('dragstart', e => {
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
      if (e.target.closest('button, input, textarea, select, .settings-library-row-actions')) return;
      const path = $('.settings-library-path', row)?.value.trim() || '';
      if (!path) {
        showToast('请先填写资源库路径');
        return;
      }
      if ((row.dataset.libraryId || '') === (state.serverSettings.active_library_id || '') || path === (state.serverSettings.storage_path || '').trim()) return;
      state.serverSettings.active_library_id = row.dataset.libraryId || '';
      state.serverSettings.storage_path = path;
      syncActiveLibrarySelect();
      applyLibraryBranding();
      setSettingsDirty();
      showToast('资源库切换已暂存，保存后会自动重启生效');
    });
    $('.settings-library-edit', row)?.addEventListener('click', () => openLibraryEditorModal(row, bindLibraryRow));
    $('.settings-library-remove', row).addEventListener('click', () => {
      row.remove();
      reindexLibrarySettingsRows();
      syncActiveLibrarySelect();
      applyLibraryBranding();
      setSettingsDirty();
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
  $$('.settings-library-row').forEach(bindLibraryRow);
  reindexLibrarySettingsRows();
  $('#settings-active-library')?.addEventListener('change', e => {
    const activeLibrary = resolveActiveLibrary(collectLibraryDrafts(), e.target.value, state.serverSettings.storage_path || '');
    state.serverSettings.active_library_id = activeLibrary && activeLibrary.id || '';
    state.serverSettings.storage_path = activeLibrary && activeLibrary.path || '';
    applyLibraryBranding();
    setSettingsDirty();
  });
  $('#settings-port')?.addEventListener('input', () => setSettingsDirty());
  $('#settings-thumbnail-dir').addEventListener('input', () => setSettingsDirty());
  $('#settings-trash-dir').addEventListener('input', () => setSettingsDirty());
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
  $('#settings-low-resource-mode').addEventListener('change', e => {
    state.serverSettings.low_resource_mode = !!e.target.checked;
    setSettingsDirty();
  });
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
  $('#settings-slideshow-mode').addEventListener('change', e => { updateSlideshowSetting('mode', e.target.value); setSettingsDirty(); });
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
  $('#settings-video-autoplay-next').addEventListener('change', e => { state.videoAutoplayNext = !!e.target.checked; setSettingsDirty(); });
  $('#settings-video-section-min').addEventListener('input', e => {
    state.videoSectionMinMinutes = Math.min(240, Math.max(1, Number(e.target.value) || 10));
    $('#settings-video-section-min-value').textContent = `${state.videoSectionMinMinutes} 分钟`;
    setSettingsDirty();
    updateVideoBookmarkProgress();
  });
  $('#settings-continue-last-video-position').addEventListener('change', e => { state.continueLastVideoPosition = !!e.target.checked; setSettingsDirty(); });
  $('#settings-prefetch-neighbors').addEventListener('change', e => { setExperimentalSetting('prefetchNeighbors', e.target.checked); setSettingsDirty(); });
  void pollVideoThumbnailRefreshStatus();
  $('#settings-refresh-video-thumbs-btn').addEventListener('click', () => {
    openVideoThumbnailRefreshWorkflow();
  });
  $('#settings-backfill-exif-btn').addEventListener('click', async () => {
    const btn = $('#settings-backfill-exif-btn');
    try {
      const resp = await withButtonBusy(btn, '补录中…', () => backfillPhotoEXIF());
      const data = resp.result || {};
      showToast(`EXIF 补录完成：扫描 ${data.scanned || 0} 张，写入 ${data.updated || 0} 张，跳过 ${data.skipped || 0} 张，失败 ${data.failed || 0} 张`, 4200);
      if (data.errors && data.errors.length) {
        alert(`以下文件补录失败：\n${data.errors.join('\n')}`);
      }
      if (state.view === 'settings') renderSettingsContent();
    } catch (e) {
      alert('EXIF 补录失败: ' + ((e && e.error) || e));
    }
  });
  $('#settings-exp-restore-last-view').addEventListener('change', e => { setExperimentalSetting('restoreLastView', e.target.checked); setSettingsDirty(); });
  $('#settings-check-update-btn')?.addEventListener('click', async () => {
    const btn = $('#settings-check-update-btn');
    await withButtonBusy(btn, '检查中…', async () => {
      await new Promise(resolve => setTimeout(resolve, 450));
      showToast('当前构建未提供版本元数据，暂无法判断是否有新版本');
    });
  });
  $('#settings-auto-update')?.addEventListener('change', e => {
    state.autoUpdateEnabled = !!e.target.checked;
    localStorage.setItem('echogallery_auto_update', state.autoUpdateEnabled ? '1' : '0');
    showToast(state.autoUpdateEnabled ? '已开启自动更新提示' : '已关闭自动更新');
  });
  $('#theme-btn')?.addEventListener('click', e => { e.preventDefault(); toggleTheme(); });
  void pollThumbnailBuildStatus();
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
function renderSelectionBarMarkup({ countLabel = '条已选', extraAction = '' } = {}) {
  return `<span id="sel-bar" class="selected-bar">
    <span class="selected-count" id="sel-count">0</span> ${countLabel}
    <button class="btn btn-sm" id="download-sel-btn">下载选中</button>
    ${extraAction}
    <button class="btn-icon" id="clear-sel-btn">${icons.close}</button>
  </span>`;
}

function bindSelectionBarHandlers({ extraButtonID = '', extraAction } = {}) {
  $('#clear-sel-btn')?.addEventListener('click', clearSelection);
  $('#download-sel-btn')?.addEventListener('click', downloadSelected);
  if (extraButtonID && typeof extraAction === 'function') {
    $(`#${extraButtonID}`)?.addEventListener('click', extraAction);
  }
  updateSelectionBar();
}

function renderTrashSelectionBarMarkup() {
  return `<span id="trash-sel-bar" class="selected-bar">
    <span class="selected-count" id="trash-sel-count">0</span> 条已选
    <button class="btn btn-sm" id="restore-sel-btn">${icons.topbarRestoreSelected} 批量恢复</button>
    <button class="btn btn-sm" id="hard-delete-sel-btn">${icons.trash} 批量删除</button>
    <button class="btn-icon" id="trash-clear-sel-btn">${icons.close}</button>
  </span>`;
}

function bindTrashSelectionBarHandlers() {
  $('#trash-clear-sel-btn')?.addEventListener('click', () => { clearSelection(); updateTrashSelBar(); });
  $('#restore-sel-btn')?.addEventListener('click', restoreSelected);
  $('#hard-delete-sel-btn')?.addEventListener('click', hardDeleteSelected);
  updateTrashSelBar();
}

async function renderTimeline() {
  state.timelineAutoLoadPaused = false;
  $('#topbar-title').textContent = '时间线';
  $('#topbar-leading').innerHTML = renderTimelineOrderControl();
  $('#topbar-meta').innerHTML = renderSelectionBarMarkup({
    countLabel: '张已选',
    extraAction: `<button class="btn btn-sm" id="delete-sel-btn">${icons.trash} 删除</button>`,
  }) + `<span class="timeline-jump-status" id="timeline-jump-status" hidden></span>` + renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = `<div class="topbar-action-group">${renderTopbarGlassButton({ id: 'timeline-load-all-btn', icon: icons.topbarLoadAll, label: '加载全部' })}${renderTopbarGlassButton({ id: 'upload-btn', icon: icons.topbarUpload, label: '上传', variant: 'accent' })}</div>`;
  bindMediaKindFilterControl();
  bindSelectionBarHandlers({ extraButtonID: 'delete-sel-btn', extraAction: deleteSelected });
  $('#timeline-order-btn').addEventListener('click', () => {
    state.timelineOrder = state.timelineOrder === 'asc' ? 'desc' : 'asc';
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
  $('#upload-btn').addEventListener('click', openUploadModal);

  $('#content').innerHTML = `
<div id="timeline-wrap"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;

  const hasPendingFocus = !!state.pendingTimelinePhotoID;
  if (state.timelineLoaded && !hasPendingFocus) {
    await loadShareMap();
    renderTimelineGrid();
    requestVisibleThumbnailWarmup('timeline', state.photos);
    observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
    restoreViewScroll('timeline');
    return;
  }

  state.photos = [];
  state.timelineCursor = '';
  state.timelineHasMore = true;
  state.timelineTotal = 0;
  await loadShareMap();   // b-2: 加载分享状态
  await loadMoreTimeline();
  state.timelineLoaded = true;
  const focusedPending = await focusPendingTimelinePhoto();
  observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
  if (!hasPendingFocus || !focusedPending) restoreViewScroll('timeline');
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
    <div class="modal-title">立即加载全部时间线媒体</div>
    <p class="modal-copy">加载期间会暂时锁定 App，只优先读取时间线和缩略图，减少后续浏览时被分页加载打断。</p>
    <div class="timeline-load-progress">
      <div class="timeline-load-progress-bar" id="timeline-load-all-bar"></div>
    </div>
    <div class="timeline-load-status" id="timeline-load-all-status">准备加载…</div>
    <div class="modal-footer">
      <button class="btn" id="timeline-load-all-cancel">取消</button>
      <button class="btn btn-primary" id="timeline-load-all-start">开始加载</button>
    </div>
  </div>
</div>`;
}

function openTimelineLoadAllModal() {
  const modal = $('#timeline-load-all-modal');
  if (!modal || state.timelineBulkLoading) return;
  if (!state.timelineHasMore) {
    showToast('时间线已全部加载');
    return;
  }
  modal.classList.add('open');
  updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal || state.photos.length || 0, '准备加载全部时间线媒体…');
  $('#timeline-load-all-start').disabled = false;
  $('#timeline-load-all-start').textContent = '开始加载';
  $('#timeline-load-all-cancel').disabled = false;
  $('#timeline-load-all-cancel').textContent = '取消';
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
    updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, '正在取消…');
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
    status.textContent = message || `已加载 ${countText} 条媒体`;
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
  const response = await fetch(`/api/media?${params.toString()}`, { signal });
  if (!response.ok) throw await response.json();
  return response.json();
}

async function loadAllTimelinePhotos() {
  if (state.timelineBulkLoading) return;
  state.timelineBulkLoading = true;
  state.timelineBulkCancelRequested = false;
  state.blockingInteraction = true;
  disconnectLoadMoreObserver();
  $('#timeline-load-all-start').disabled = true;
  $('#timeline-load-all-start').textContent = '加载中…';
  $('#timeline-load-all-cancel').disabled = false;
  $('#timeline-load-all-cancel').textContent = '取消加载';
  try {
    let cursor = state.timelineCursor;
    let hasMore = state.timelineHasMore;
    let loaded = state.photos.length;
    let total = state.timelineTotal || loaded;
    preloadTimelineThumbnails(state.photos);
    updateTimelineLoadAllProgress(loaded, total, `正在加载… 已读取 ${loaded} 条`);

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
      updateTimelineLoadAllProgress(loaded, total, `正在加载… 已读取 ${loaded}${total ? ` / ${total}` : ''} 条`);
      const loadMore = $('#load-more');
      if (loadMore) loadMore.innerHTML = `<div class="spinner"></div>正在加载全部时间线… ${loaded}${total ? ` / ${total}` : ''}`;
      await new Promise(resolve => requestAnimationFrame(resolve));
    }

    if (state.timelineBulkCancelRequested) {
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, `已取消，当前已加载 ${state.photos.length} 条`);
      showToast('已取消加载全部时间线');
    } else {
      state.timelineHasMore = false;
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal || state.photos.length, `加载完成，共 ${state.photos.length} 条媒体`);
      showToast('全部时间线媒体已加载');
      setTimeout(() => $('#timeline-load-all-modal')?.classList.remove('open'), 700);
    }
  } catch (e) {
    if (e && e.name === 'AbortError') {
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, `已取消，当前已加载 ${state.photos.length} 条`);
      showToast('已取消加载全部时间线');
    } else {
      console.error(e);
      updateTimelineLoadAllProgress(state.photos.length, state.timelineTotal, '加载失败，请稍后重试');
      showToast('加载全部时间线失败');
    }
  } finally {
    state.timelineBulkLoading = false;
    state.timelineBulkAbortController = null;
    state.blockingInteraction = false;
    $('#timeline-load-all-start').disabled = false;
    $('#timeline-load-all-start').textContent = '重新加载';
    $('#timeline-load-all-cancel').disabled = false;
    $('#timeline-load-all-cancel').textContent = '关闭';
    updateLoadMoreUI('load-more', state.timelineHasMore);
    if (state.view === 'timeline') {
      observeLoadMore('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
    }
  }
}

async function renderFavorites() {
  $('#topbar-title').textContent = '个人收藏';
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'download-all-favorites-btn', icon: icons.topbarDownloadFavorites, label: '下载全部收藏', variant: 'accent' }),
  ]);
  $('#topbar-meta').innerHTML = renderSelectionBarMarkup({
    countLabel: '条已选',
    extraAction: `<button class="btn btn-sm" id="unfavorite-sel-btn">${icons.favorite} 取消收藏</button>`,
  }) + renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = '';
  bindMediaKindFilterControl();
  bindSelectionBarHandlers({ extraButtonID: 'unfavorite-sel-btn', extraAction: unfavoriteSelected });

  $('#content').innerHTML = `
<div id="favorite-wrap"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;
  $('#download-all-favorites-btn').addEventListener('click', () => {
    if (confirm('确定要打包下载全部个人收藏吗？媒体较多时可能需要一些时间。')) downloadAllFavorites();
  });

  await loadShareMap();
  if (state.favoriteLoaded) {
    appendFavoriteGrid(state.favoritePhotos);
    updateFavoriteTotalHint();
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
      limit: state.pendingTimelinePhotoID ? '180' : '30',
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
    requestVisibleThumbnailWarmup('timeline', page.photos || []);
  } catch (e) {
    if (!e || e.name !== 'AbortError') console.error(e);
  }
  finally {
    state.timelineLoading = false;
    state.timelineLoadAbortController = null;
    updateLoadMoreUI('load-more', state.timelineHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreTimeline, () => state.timelineHasMore && !state.timelineLoading && !state.timelineBulkLoading && !state.timelineAutoLoadPaused);
  }
}

function appendTimelineGrid(newPhotos) {
  const container = $('#timeline-wrap');
  if (!container) return;
  if (state.photos.length === 0 && newPhotos.length === 0 && state.timelineTotal === 0 && !state.timelineHasMore) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>还没有媒体，点击右上角上传吧</p></div>`;
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

function renderTimelineGrid() {
  const container = $('#timeline-wrap');
  if (!container) return;
  container.innerHTML = '';
  if (state.photos.length) appendTimelineGrid(state.photos);
  if (state.photos.length === 0 && !state.timelineHasMore && !state.timelineLoading) {
    container.innerHTML = `<div class="empty">${icons.photo}<p>还没有媒体，点击右上角上传吧</p></div>`;
  }
  updateLoadMoreUI('load-more', state.timelineHasMore);
}

function appendFavoriteGrid(newPhotos) {
  const container = $('#favorite-wrap');
  if (!container) return;
  if (state.favoritePhotos.length === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.favorite}<p>还没有加入个人收藏的媒体</p></div>`;
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

function updateFavoriteTotalHint() {
  const hint = $('#favorite-total-hint');
  if (!hint) return;
  hint.textContent = `共 ${state.favoriteTotal || 0} 条`;
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
    state.favoriteTotal = Number.isFinite(Number(page.total)) ? Number(page.total) : state.favoritePhotos.length + (page.photos || []).length;
    updateFavoriteTotalHint();
    state.favoritePhotos.push(...(page.photos || []));
    state.favoriteCursor = page.next_cursor || '';
    state.favoriteHasMore = page.has_more || false;
    state.favoriteLoaded = true;
    appendFavoriteGrid(page.photos || []);
  } catch (e) {
    console.error(e);
  } finally {
    state.favoriteLoading = false;
    updateLoadMoreUI('load-more', state.favoriteHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreFavorites, () => state.favoriteHasMore && !state.favoriteLoading);
  }
}

function loadMemoryEntries() {
  try {
    const entries = JSON.parse(localStorage.getItem(memoriesStorageKey) || '[]');
    return Array.isArray(entries) ? entries.filter(entry => entry && entry.id).slice(0, 120) : [];
  } catch (_) {
    return [];
  }
}

function saveMemoryEntries(entries) {
  localStorage.setItem(memoriesStorageKey, JSON.stringify((entries || []).slice(0, 120)));
}

function rememberPhoto(photo) {
  if (!photo || !photo.id) return;
  const id = Number(photo.id);
  const entries = loadMemoryEntries().filter(entry => Number(entry.id) !== id);
  entries.unshift({ id, viewed_at: new Date().toISOString() });
  saveMemoryEntries(entries);
}

async function renderMemories() {
  $('#topbar-title').textContent = '回忆';
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'clear-memories-btn', icon: icons.topbarClearMemories, label: '清除回忆' }),
  ]);
  $('#topbar-meta').innerHTML = renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = '';
  bindMediaKindFilterControl();
  $('#clear-memories-btn')?.addEventListener('click', () => {
    if (!confirm('确定要清空全部回忆记录吗？此操作只会移除浏览历史，不会删除媒体文件。')) return;
    localStorage.removeItem(memoriesStorageKey);
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
    container.innerHTML = `<div class="empty">${icons.photo}<p>还没有回忆，浏览一些照片或视频后会出现在这里</p></div>`;
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

function mediaThumbURL(photo) {
  return `/media/thumbnails/${photo.uuid}`;
}

function mediaFileURL(photo) {
  return isVideoMedia(photo) ? `/media/files/${photo.uuid}` : `/media/photos/${photo.uuid}`;
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
    case 1: return '横向';
    case 2: return '横向（镜像）';
    case 3: return '倒置';
    case 4: return '竖向（镜像）';
    case 5: return '横向（旋转）';
    case 6: return '竖向';
    case 7: return '横向（反向旋转）';
    case 8: return '横向（旋转）';
    default: return `方向 ${compactExifValue(value) || '—'}`;
  }
}
function buildExifInfoItems(photo) {
  const exif = photo && photo.exif;
  if (!exif || typeof exif !== 'object') return [];
  const items = [];
  const camera = [compactExifValue(exif.make), compactExifValue(exif.model)].filter(Boolean).join(' ');
  if (camera) items.push([icons.exifCamera, '相机', camera]);
  if (compactExifValue(exif.f_number)) items.push([icons.exifAperture, '光圈', formatExifAperture(exif.f_number)]);
  if (compactExifValue(exif.exposure_time)) items.push([icons.exifShutter, '快门', compactExifValue(exif.exposure_time)]);
  if (Number(exif.iso_speed) > 0) items.push([icons.exifISO, 'ISO', String(Number(exif.iso_speed))]);
  if (compactExifValue(exif.focal_length)) items.push([icons.exifFocal, '焦距', formatExifFocalLength(exif.focal_length)]);
  if (Number(exif.orientation) > 0) items.push([icons.exifOrientation, '方向', formatExifOrientation(exif.orientation)]);
  if (exif.has_gps && Number.isFinite(Number(exif.latitude)) && Number.isFinite(Number(exif.longitude))) {
    items.push([icons.exifGPS, 'GPS', `${Number(exif.latitude).toFixed(5)}, ${Number(exif.longitude).toFixed(5)}`]);
  }
  return items;
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
  const nextURL = `${mediaThumbURL(photo)}?r=${Date.now()}`;
  imageEl.closest('.photo-thumb')?.classList.add('thumb-loading');
  window.setTimeout(() => {
    if (!document.body.contains(imageEl)) return;
    imageEl.src = nextURL;
  }, delays[attempt]);
}

function prefetchPhotosForLightboxIntent(listRef, index) {
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
  div.classList.add('thumb-loading');

  const isShared = !!state.shareMap[`photo:${photo.id}`];
  const favoriteToggle = opts.trashMode
    ? ''
    : `<button class="favorite-toggle${photo.is_favorite ? ' active' : ''}" type="button" aria-label="${photo.is_favorite ? '取消收藏' : '加入收藏'}" title="${photo.is_favorite ? '已收藏，点击取消' : '加入个人收藏'}">${photo.is_favorite ? icons.favoriteFilled : icons.favorite}</button>`;
  const shareBadge = isShared
    ? `<span class="share-badge">${icons.shareSmall}</span>` : '';
  const mediaBadge = isVideoMedia(photo)
    ? `<span class="media-badge">${escapeHTML(videoFormatLabel(photo))}${photo.duration_ms ? ` · ${formatDuration(photo.duration_ms)}` : ''}</span>`
    : '';
  const bookmarkCount = getVideoBookmarkCount(photo);
  const videoBookmarkBadge = bookmarkCount > 0
    ? `<span class="video-bookmark-badge" title="视频书签 ${bookmarkCount}/10" aria-label="视频书签 ${bookmarkCount}/10">${icons.bookmark}<span class="video-bookmark-count">${bookmarkCount}</span></span>`
    : '';
  const thumbFallback = isVideoMedia(photo) ? ` onerror="this.onerror=null;this.src='${videoPosterPlaceholder}'"` : '';

  div.innerHTML = `<span class="check">${icons.check}</span>${favoriteToggle}<img loading="lazy" src="${mediaThumbURL(photo)}" alt="${photo.original_name}"${thumbFallback}>${videoBookmarkBadge}${shareBadge}${mediaBadge}`;
  const imageEl = div.querySelector('img');
  if (imageEl) {
    imageEl.addEventListener('load', () => {
      requestAnimationFrame(() => div.classList.remove('thumb-loading', 'thumb-load-failed'));
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
  checkEl.addEventListener('click', e => {
    e.stopPropagation();
    toggleSelect(photo.id, div, { event: e, listRef });
  });
  const favoriteEl = div.querySelector('.favorite-toggle');
  if (favoriteEl) {
    favoriteEl.addEventListener('click', async e => {
      e.stopPropagation();
      await toggleFavorite(photo);
    });
  }
  const photoIndex = Array.isArray(listRef) ? listRef.indexOf(photo) : -1;
  const warmIntent = () => {
    if (photoIndex >= 0) prefetchPhotosForLightboxIntent(listRef, photoIndex);
  };
  div.addEventListener('pointerenter', warmIntent, { passive: true });
  div.addEventListener('focusin', warmIntent);

  // 图片主体点击
  div.addEventListener('click', e => {
    setFocusedPhoto(photo.id, { scroll: false });
    if (opts.trashMode) {
      openLightbox(listRef, listRef.indexOf(photo));
      return;
    }
    if (state.selected.size > 0) {
      toggleSelect(photo.id, div, { event: e, listRef });
    } else {
      openLightbox(listRef, listRef.indexOf(photo));
    }
  });

  // PC 右键菜单
  div.addEventListener('contextmenu', e => {
    e.preventDefault();
    setFocusedPhoto(photo.id, { scroll: false });
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
  if (state.focusedPhotoID === photo.id) div.classList.add('keyboard-focused');
  return div;
}

function visiblePhotoThumbs() {
  return $$('.photo-thumb').filter(thumb => thumb.offsetParent !== null);
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
    imageEl.src = `${mediaThumbURL(photo)}?live=${now}-${photo.id}`;
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
  return document.querySelector(`.photo-thumb[data-id="${id}"]`);
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

function photoContextMenuItems(photo, thumbEl, listRef, containingAlbums = []) {
  const isSelected = state.selected.has(photo.id);
  const isShared = !!state.shareMap[`photo:${photo.id}`];
  const favoriteLabel = photo.is_favorite ? '取消收藏' : '加入个人收藏';
  const items = [
    { icon: icons.contextSelect, label: isSelected ? '取消选择' : '选择（点击勾选图标可快速选择）', action: () => toggleSelect(photo.id, thumbEl, { listRef }) },
    { icon: icons.contextView, label: '查看', action: () => openLightbox(listRef, listRef.indexOf(photo)) },
    { icon: icons.contextTimeline, label: '在时间线中查看', action: () => openInTimeline(photo.id) },
    { icon: icons.contextFavorite, label: favoriteLabel, action: () => toggleFavorite(photo) },
    { icon: icons.contextReveal, label: '在文件管理器中打开', action: () => revealInFinder(photo.id) },
    { icon: icons.contextDownload, label: '下载', action: () => triggerDownload(`/api/media/${photo.id}/download`) },
  ];
  if (containingAlbums.length) {
    items.push('-');
    containingAlbums.forEach(album => {
      items.push({ icon: icons.contextAlbum, label: `在相册中查看：${albumDisplayTitle(album)}`, action: () => openInAlbum(album, photo.id) });
    });
  }
  items.push(
    '-',
    { icon: icons.contextShare, label: isShared ? '管理分享…' : '分享…', action: () => isShared ? openShareListModal('photo', photo.id) : openShareModal('photo', photo.id) },
    '-',
    { icon: icons.contextDelete, label: '删除', danger: true, action: () => deleteSinglePhoto(photo.id) },
  );
  return items;
}

// 时间线图片右键菜单
async function showPhotoContextMenu(x, y, photo, thumbEl, listRef) {
  let albums = [];
  try {
    albums = await api.get(`/api/media/${photo.id}/albums`);
  } catch (e) {
    console.error('加载媒体所在相册失败:', e);
  }
  if ((!albums || !albums.length) && state.view === 'album-detail' && state.currentAlbum && state.currentAlbum.id) {
    albums = [state.currentAlbum];
  }
  showContextMenu(x, y, photoContextMenuItems(photo, thumbEl, listRef, albums || []));
}

async function showCurrentLightboxContextMenu() {
  const photo = state.lightboxPhotos[state.lightboxIndex];
  if (!photo) return;
  const button = $('#lb-more');
  const rect = button ? button.getBoundingClientRect() : { right: window.innerWidth - 18, bottom: 58 };
  await showPhotoContextMenu(rect.right, rect.bottom + 8, photo, findPhotoThumb(photo.id), state.lightboxPhotos);
}
function renderVideoBookmarkMenuContent(photo, video) {
  const currentTime = Math.max(0, Number(video.currentTime) || 0);
  return `
    <div class="lightbox-bookmark-menu-head">
      <div>
        <div class="lightbox-bookmark-menu-title">视频书签</div>
        <div class="lightbox-bookmark-menu-subtitle">按键盘上方 1-0 可添加或跳转</div>
      </div>
      <span class="lightbox-bookmark-menu-current">当前 ${escapeHTML(formatDuration(currentTime * 1000))}</span>
    </div>
    <div class="lightbox-bookmark-menu-list">
      ${Array.from({ length: 10 }, (_, i) => {
        const slot = i + 1;
        const bookmark = getVideoBookmarkSlot(photo, slot);
        const timeText = bookmark ? formatDuration(bookmark.time * 1000) : '空位';
        const label = bookmark
          ? `${bookmark.name || `书签 ${slot}`} · ${timeText}`
          : `书签 ${slot} · 点击保存当前时间`;
        if (!bookmark) {
          return `
            <button type="button" class="lightbox-bookmark-jump lightbox-bookmark-jump-standalone" data-video-bookmark-slot="${slot}" title="${escapeHTML(label)}">
              <span class="lightbox-bookmark-slot">${slot === 10 ? '0' : slot}</span>
              <span class="lightbox-bookmark-text">${escapeHTML(label)}</span>
            </button>`;
        }
        return `
          <div class="lightbox-bookmark-row" data-slot="${slot}">
            <button type="button" class="lightbox-bookmark-jump" data-video-bookmark-slot="${slot}" title="${escapeHTML(label)}">
              <span class="lightbox-bookmark-slot">${slot === 10 ? '0' : slot}</span>
              <span class="lightbox-bookmark-text">${escapeHTML(label)}</span>
            </button>
            <button type="button" class="btn-icon lightbox-bookmark-delete" data-video-bookmark-delete="${slot}" aria-label="删除书签 ${slot}" title="删除书签 ${slot}">${icons.contextDelete}</button>
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
  const photo = state.lightboxPhotos[state.lightboxIndex];
  const video = $('#lb-video');
  if (!isVideoMedia(photo) || !video || video.classList.contains('hidden')) return;
  closeContextMenu();
  const menu = el('div', 'context-menu lightbox-bookmark-menu');
  const button = $('#lb-video-bookmark');
  const rect = button ? button.getBoundingClientRect() : { left: window.innerWidth - 18, right: window.innerWidth - 18, top: 58, bottom: 58 };
  menu.style.left = `${rect.right}px`;
  menu.style.top = `${rect.bottom + 8}px`;
  menu.innerHTML = renderVideoBookmarkMenuContent(photo, video);
  document.body.appendChild(menu);
  _ctxMenu = menu;
  const menuRect = menu.getBoundingClientRect();
  if (menuRect.right > window.innerWidth) menu.style.left = `${Math.max(12, rect.right - menuRect.width)}px`;
  if (menuRect.bottom > window.innerHeight) menu.style.top = `${Math.max(12, rect.top - menuRect.height - 8)}px`;
}

// 回收站图片右键菜单 (b-4)
function showTrashContextMenu(x, y, photo) {
  showContextMenu(x, y, [
    { icon: icons.contextReveal, label: '在文件管理器中打开', action: () => revealInFinder(photo.id) },
    '-',
    { icon: icons.contextRestore, label: '恢复到时间线', action: () => restorePhoto(photo.id) },
    { icon: icons.contextDelete, label: '永久删除', danger: true, action: () => hardDeleteSinglePhoto(photo.id) },
  ]);
}

async function addSelectedMediaToAlbum(album) {
  showToast('文件夹相册暂不支持手动添加');
}

function albumContextMenuItems(album, { includeView = true } = {}) {
  const items = [];
  if (includeView) {
    items.push({ icon: icons.contextView, label: '查看', action: () => openAlbumDetail(album) });
  }
  items.push(
    { icon: icons.contextReveal, label: '在文件管理器中打开', action: () => revealAlbumInFinder(album.id) },
    { icon: icons.albumContextDownload, label: '下载', action: () => triggerDownload(`/api/media/albums/${album.id}/download`) },
  );
  return items;
}

function showAlbumContextMenu(x, y, album, options = {}) {
  if (!album || !album.id) return;
  showContextMenu(x, y, albumContextMenuItems(album, options));
}

async function addSinglePhotoToAlbum(photoId) {
  showToast('文件夹相册暂不支持手动添加');
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
    button.title = favorite ? '已收藏，点击取消' : '加入个人收藏';
    button.innerHTML = favorite ? icons.favoriteFilled : icons.favorite;
  });
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
  container.innerHTML = `<div class="empty">${icons.favorite}<p>${escapeHTML(copyText('app.empty.favorites', '还没有加入个人收藏的媒体'))}</p></div>`;
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

async function setPhotoFavorite(photoId, favorite) {
  await api.put(`/api/media/${photoId}/favorite`, { favorite });
  updatePhotoFavoriteInCollections(photoId, favorite);
}

function updateLightboxFavoriteButton(photo) {
  const favoriteBtn = $('#lb-favorite');
  if (!favoriteBtn || !photo) return;
  favoriteBtn.innerHTML = photo.is_favorite ? icons.favoriteFilled : icons.favorite;
  favoriteBtn.title = photo.is_favorite ? '取消收藏' : '收藏';
  favoriteBtn.setAttribute('aria-label', favoriteBtn.title);
  favoriteBtn.classList.toggle('active', !!photo.is_favorite);
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
    if ($('#lightbox').classList.contains('open')) updateLightboxFavoriteButton(photo);
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
  try {
    await api.del(`/api/media/${photoId}`);
    removeDeletedPhotosFromUI([photoId]);
    showToast('已移入回收站');
  }
  catch(e) { alert('删除失败: ' + (e.error || e)); }
}

async function hardDeleteSinglePhoto(photoId) {
  if (!confirm('确定要永久删除这条照片/视频吗？此操作不可恢复。')) return;
  try { await api.del(`/api/trash/${photoId}`); switchView('trash'); }
  catch(e) { alert('删除失败: ' + (e.error || e)); }
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
  state.selectionAnchorID = null;
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
  if (thumbs.length) state.selectionAnchorID = Number(thumbs[0].dataset.id) || state.selectionAnchorID;
  updateSelectionModeUI();
  updateSelectionBar();
  updateTrashSelBar();
}
async function deleteSelected() {
  if (!state.selected.size) return;
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
  if (failed.length) {
    showToast(`已删除 ${deleted.length} 条，${failed.length} 条失败`);
  } else if (deleted.length) {
    showToast(`已删除 ${deleted.length} 条媒体`);
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
        container.innerHTML = `<div class="empty">${icons.photo}<p>还没有媒体，点击右上角上传吧</p></div>`;
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
        container.innerHTML = `<div class="empty">${icons.photo}<p>还没有回忆，浏览一些照片或视频后会出现在这里</p></div>`;
      }
      break;
    }
    case 'random-album': {
      const wrap = $('#random-album-wrap');
      if (wrap && !wrap.querySelector('.photo-thumb') && !state.randomAlbumHasMore && !state.randomAlbumLoading) {
        wrap.innerHTML = `<div class="empty">${icons.shuffle}<p>还没有可浏览的媒体</p></div>`;
      }
      updateRandomAlbumLoadMoreUI();
      break;
    }
    case 'album-detail': {
      const container = $('#album-groups');
      if (container && !container.querySelector('.photo-thumb') && state.albumPhotos.length === 0 && !albumDirectChildCount(state.currentAlbum)) {
        container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.empty.albumMedia', '相册里还没有媒体'))}</p></div>`;
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

function openInTimeline(photoId) {
  state.pendingTimelinePhotoID = photoId;
  state.timelineJumpCancelRequested = false;
  closeLightbox();
  switchView('timeline');
}

function openInAlbum(album, photoId) {
  if (!album || !album.id) return;
  state.pendingAlbumPhotoID = photoId;
  closeLightbox();
  openAlbumDetail(album);
}

async function focusPendingTimelinePhoto() {
  if (!state.pendingTimelinePhotoID) return false;
  const photoId = state.pendingTimelinePhotoID;
  const token = ++state.timelineJumpToken;
  setTimelineJumpStatus('正在时间线中定位媒体，可能需要继续加载…');
  showBlockingProgress('正在时间线中定位媒体', '正在检查已加载内容…', {
    cancelText: '取消查找',
    onCancel: () => {
      state.timelineJumpCancelRequested = true;
      state.pendingTimelinePhotoID = null;
      state.timelineJumpToken += 1;
      setTimelineJumpStatus('');
      hideBlockingProgress();
      showToast('已取消在时间线中查找');
    },
  });
  let thumb = findPhotoThumb(photoId);
  while (!thumb && state.timelineHasMore && !state.timelineLoading && !state.timelineJumpCancelRequested && token === state.timelineJumpToken) {
    setTimelineJumpStatus(`正在时间线中定位媒体… 已加载 ${state.photos.length} 条`);
    updateBlockingProgress(`已加载 ${state.photos.length} 条媒体，继续向下查找…`);
    await loadMoreTimeline();
    if (state.timelineJumpCancelRequested || token !== state.timelineJumpToken) return false;
    if (state.timelineBulkLoading) break;
    thumb = findPhotoThumb(photoId);
  }
  if (state.timelineJumpCancelRequested || token !== state.timelineJumpToken) return false;
  state.pendingTimelinePhotoID = null;
  state.timelineJumpCancelRequested = false;
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
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'new-album-btn', icon: icons.topbarNewAlbum, label: '新建相册暂不可用', variant: 'accent', disabled: true }),
  ]);
  $('#topbar-meta').innerHTML = renderAlbumViewModeControl();
  $('#topbar-actions').innerHTML = '';
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
  } catch(e) { $('#content').innerHTML = `<p style="color:var(--danger)">加载失败</p>`; }
}

async function ensureAlbumsLoaded() {
  if (state.albumsLoaded && Array.isArray(state.albums)) return state.albums;
  state.albums = await api.get('/api/media/albums');
  state.albumsLoaded = true;
  return state.albums;
}

function renderAlbumGrid() {
  const wrap = $('#album-grid-wrap');
  if (!wrap) return;
  const albums = albumChildrenForParent('');
  if (!albums.length) {
    wrap.innerHTML = `<div class="empty">${icons.album}<p>${escapeHTML(copyText('app.empty.albums', '还没有文件夹相册'))}</p></div>`;
    return;
  }
  const grid = el('div', `album-grid${state.albumViewMode === 'list' ? ' list' : ''}`);
  albums.forEach(a => grid.appendChild(makeAlbumCard(a)));
  wrap.innerHTML = '';
  wrap.appendChild(grid);
}

function isFolderAlbum(album) {
  return String(album && album.source_kind || '').trim() === 'folder'
    || String(album && album.description || '').trim() === '自动从文件夹导入';
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
  if (!path) return String(album.name || '').trim() || '相册';
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || path;
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
  return albumRelativeAddress(a).localeCompare(albumRelativeAddress(b), 'zh-Hans-CN', { numeric: true, sensitivity: 'base' });
}

function albumChildrenForParent(parentPath = '') {
  const target = normalizeAlbumPath(parentPath);
  return (state.albums || [])
    .filter(album => {
      if (!isFolderAlbum(album)) return false;
      return albumParentPath(album) === target;
    })
    .sort(compareAlbumsByPath);
}

function albumSiblingList(album = state.currentAlbum) {
  if (!album) return [];
  return albumChildrenForParent(isFolderAlbum(album) ? albumParentPath(album) : '');
}

function albumDirectChildCount(album) {
  if (!album || !isFolderAlbum(album)) return 0;
  return albumChildrenForParent(folderAlbumPath(album)).length;
}

function albumItemCount(album) {
  return Math.max(0, Number(album && album.photo_count) || 0) + albumDirectChildCount(album);
}

function findAlbumByPath(path = '') {
  const target = normalizeAlbumPath(path);
  if (!target) return null;
  return (state.albums || []).find(album => folderAlbumPath(album) === target) || null;
}

function parentAlbumFor(album) {
  return findAlbumByPath(albumParentPath(album));
}

function makeAlbumCard(album) {
  const albumKind = isFolderAlbum(album) ? 'folder' : 'user';
  const albumKindLabel = albumKind === 'folder' ? '文件夹' : '自建';
  const albumKindIcon = albumKind === 'folder' ? icons.albumCardFolder : icons.albumCardUser;
  const title = albumDisplayTitle(album);
  const description = albumRelativeAddress(album) || '相对地址不可用';
  const itemCount = albumItemCount(album);
  const card = el('div', `album-card album-card-${albumKind}${state.albumViewMode === 'list' ? ' list' : ''}`);
  // c-1: 用 cover_uuid 显示封面缩略图
  const coverHtml = album.cover_uuid
    ? `<img loading="lazy" src="/media/thumbnails/${album.cover_uuid}" alt="${escapeHTML(title)}" onerror="this.onerror=null;this.src='${videoPosterPlaceholder}'">`
    : `<div class="album-cover-empty">${icons.photo}</div>`;
  card.innerHTML = `
	<div class="album-cover">${coverHtml}</div>
	<div class="album-info">
	  <div class="album-name" title="${escapeHTML(title)}">${escapeHTML(title)}</div>
    <div class="album-subtitle">${escapeHTML(description)}</div>
    <div class="album-count-row">${icons.albumCardCount || icons.photo}<span>${itemCount} 个项目</span></div>
  </div>`;
  const kind = el('span', 'album-kind', albumKindIcon);
  kind.title = albumKindLabel;
  kind.setAttribute('aria-label', albumKindLabel);
  $('.album-info', card)?.appendChild(kind);
  card.addEventListener('click', () => openAlbumDetail(album));
  card.addEventListener('contextmenu', e => {
    e.preventDefault();
    showAlbumContextMenu(e.clientX, e.clientY, album);
  });
  addLongPress(card, e => {
    const touch = e.changedTouches[0];
    showAlbumContextMenu(touch.clientX, touch.clientY, album);
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
      console.warn('补载相册列表失败，Q/E 相册切换可能暂不可用', e);
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
  const childAlbums = albumChildrenForParent(folderAlbumPath(album));
  $('#topbar-title').textContent = albumTitle;
  $('#topbar-meta').innerHTML = childAlbums.length
    ? renderAlbumViewModeControl()
    : renderSelectionBarMarkup({
      countLabel: '条已选',
      extraAction: `<button class="btn btn-sm" id="delete-sel-btn">${icons.trash} 删除</button>`,
    });
  $('#topbar-actions').innerHTML = '';
  if (!childAlbums.length) {
    bindSelectionBarHandlers({ extraButtonID: 'delete-sel-btn', extraAction: deleteSelected });
  }

  $('#content').innerHTML = `
<div class="album-detail-pill" id="album-detail-pill">
  <button class="album-detail-back-btn" id="album-detail-back-btn" type="button" title="返回相册" aria-label="返回相册">${icons.topbarBackAlbums}</button>
  <span class="album-detail-title" title="${escapeHTML(albumAddress || albumTitle)}">${escapeHTML(albumTitle)}</span>
</div>
${childAlbums.length ? '' : `<div class="album-detail-bottom-controls" id="album-detail-bottom-controls">${renderAlbumDetailBottomActionControls()}</div>`}
<div id="album-child-wrap"></div><div id="album-groups"></div><div class="load-more" id="load-more" ${childAlbums.length ? 'style="display:none"' : ''}><div class="spinner"></div>加载中…</div>`;
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
        state.albumDetailLoadedKey = '';
        state.viewScrollPositions[viewScrollKeyFor('album-detail', album.id)] = 0;
        renderAlbumDetail();
      });
    });
  }
  $('#album-detail-back-btn')?.addEventListener('click', () => {
    const parent = parentAlbumFor(album);
    if (parent) openAlbumDetail(parent);
    else switchView('albums');
  });
  $('#album-detail-pill')?.addEventListener('contextmenu', e => {
    e.preventDefault();
    showAlbumContextMenu(e.clientX, e.clientY, album, { includeView: false });
  });
  addLongPress($('#album-detail-pill'), e => {
    const touch = e.changedTouches[0];
    showAlbumContextMenu(touch.clientX, touch.clientY, album, { includeView: false });
  });
  const hasPendingAlbumFocus = !!state.pendingAlbumPhotoID;
  const detailStateKey = `${album.id}:${state.albumDetailSort}:${state.mediaKindFilter}:${childAlbums.map(item => item.id).join(',')}`;
  if (childAlbums.length) {
    state.albumPhotos = [];
    state.albumCursor = '';
    state.albumHasMore = false;
    state.albumLoading = false;
    state.albumDetailLoadedKey = detailStateKey;
    renderAlbumGroups([]);
    updateLoadMoreUI('load-more', false);
    restoreViewScroll('album-detail', album.id);
    return;
  }
  if (state.albumDetailLoadedKey === detailStateKey && !hasPendingAlbumFocus) {
    renderAlbumGroups(state.albumPhotos);
    updateLoadMoreUI('load-more', state.albumHasMore);
    observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    restoreViewScroll('album-detail', album.id);
    return;
  }
  state.albumPhotos = [];
  state.albumCursor = '';
  state.albumHasMore = true;
  await loadMoreAlbumPhotos();
  state.albumDetailLoadedKey = detailStateKey;
  const focusedPendingAlbumPhoto = await focusPendingAlbumPhoto();
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
  if (state.currentAlbum && Number(state.currentAlbum.id) === targetID) {
    state.currentAlbum = { ...state.currentAlbum, ...updatedAlbum };
  }
}
async function deleteAlbum(album) {
  if (!album) return;
  if (!confirm(`确定要删除相册「${album.name}」吗？照片/视频本身不会被删除。`)) return;
  try {
	await api.del(`/api/media/albums/${album.id}`);
    state.albums = (state.albums || []).filter(item => Number(item.id) !== Number(album.id));
    state.currentAlbum = null;
    state.currentAlbumID = null;
    state.lastAlbumDetailID = null;
    showToast('已删除相册');
    switchView('albums');
  } catch (e) {
    alert('删除相册失败: ' + (e.error || e));
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
    state.albumPhotos.push(...(page.photos || []));
    state.albumCursor = page.next_cursor || '';
    state.albumHasMore = page.has_more || false;
    renderAlbumGroups(page.photos || []);
  } catch(e) { console.error(e); }
  finally {
    state.albumLoading = false;
    updateLoadMoreUI('load-more', state.albumHasMore);
    observeLoadMore('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
    maybeLoadMoreImmediately('load-more', loadMoreAlbumPhotos, () => state.albumHasMore && !state.albumLoading);
  }
}

async function focusPendingAlbumPhoto() {
  if (!state.pendingAlbumPhotoID) return false;
  const photoId = state.pendingAlbumPhotoID;
  let thumb = findPhotoThumb(photoId);
  while (!thumb && state.albumHasMore && !state.albumLoading) {
    await loadMoreAlbumPhotos();
    thumb = findPhotoThumb(photoId);
  }
  state.pendingAlbumPhotoID = null;
  if (!thumb) {
    showToast('目标媒体暂未在当前相册中找到');
    return false;
  }
  thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
  return true;
}

function focusPhotoThumbInCurrentView(photoId) {
  const thumb = findPhotoThumb(photoId);
  if (!thumb) return false;
  thumb.scrollIntoView({ behavior: 'auto', block: 'center' });
  thumb.classList.add('photo-thumb-focus');
  setTimeout(() => thumb.classList.remove('photo-thumb-focus'), 1000);
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
    container.innerHTML = `<div class="empty">${icons.photo}<p>${escapeHTML(copyText('app.empty.albumMedia', '相册里还没有媒体'))}</p></div>`;
    return;
  }
  if (newPhotos.length && container.querySelector('.empty')) container.innerHTML = '';
  let grid = $('#album-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'album-grid';
    container.appendChild(grid);
  }
  const fragment = document.createDocumentFragment();
  newPhotos.forEach(p => fragment.appendChild(makePhotoThumb(p, state.albumPhotos)));
  grid.appendChild(fragment);
}

// ── 回收站 (b-4 修复) ─────────────────────────────────
async function renderTrash() {
  $('#topbar-title').textContent = '回收站';
  $('#topbar-leading').innerHTML = renderTopbarLeadingGroup([
    renderTopbarGlassButton({ id: 'restore-all-trash-btn', icon: icons.topbarRestoreAll || icons.restore, label: '恢复全部' }),
    renderTopbarGlassButton({ id: 'empty-trash-btn', icon: icons.topbarEmptyTrash, label: '清空回收站', variant: 'danger' }),
  ]);
  $('#topbar-meta').innerHTML = renderTrashSelectionBarMarkup() + renderMediaKindFilterControl();
  $('#topbar-actions').innerHTML = '';
  bindMediaKindFilterControl();
  bindTrashSelectionBarHandlers();
  $('#empty-trash-btn').addEventListener('click', emptyTrash);
  $('#restore-all-trash-btn')?.addEventListener('click', restoreAllTrash);

  // c-5: 加入批量恢复工具栏
  $('#content').innerHTML = `
<div id="trash-wrap"></div>
<div class="load-more" id="load-more"><div class="spinner"></div>加载中…</div>`;

  if (state.trashLoaded) {
    renderTrashGroups(state.trashPhotos);
    observeLoadMore('load-more', loadMoreTrash, () => state.trashHasMore && !state.trashLoading);
    restoreViewScroll('trash');
    return;
  }

  state.trashPhotos = []; state.trashCursor = ''; state.trashHasMore = true;
  await loadMoreTrash();
  observeLoadMore('load-more', loadMoreTrash, () => state.trashHasMore && !state.trashLoading);
  restoreViewScroll('trash');
}
async function loadMoreTrash() {
  if (state.trashLoading || !state.trashHasMore) return;
  state.trashLoading = true;
  try {
		const params = new URLSearchParams();
		appendMediaKindParam(params);
		if (state.trashCursor) params.set('cursor', state.trashCursor);
		const query = params.toString();
		const url = '/api/media/trash' + (query ? `?${query}` : '');
    const page = await api.get(url);
    state.trashPhotos.push(...(page.photos || []));
    state.trashCursor = page.next_cursor || '';
    state.trashHasMore = page.has_more || false;
    state.trashLoaded = true;
    renderTrashGroups(page.photos || []);
  } catch(e) { console.error(e); }
  finally {
    state.trashLoading = false;
    updateLoadMoreUI('load-more', state.trashHasMore);
    maybeLoadMoreImmediately('load-more', loadMoreTrash, () => state.trashHasMore && !state.trashLoading);
  }
}
function renderTrashGroups(newPhotos) {
  const container = $('#trash-wrap');
  if (!container) return;
  if (state.trashPhotos.length === 0 && newPhotos.length === 0) {
    container.innerHTML = `<div class="empty">${icons.trash}<p>${escapeHTML(copyText('app.empty.trash', '回收站是空的'))}</p></div>`;
    return;
  }
  if (newPhotos.length && container.querySelector('.empty')) container.innerHTML = '';
  let grid = $('#trash-grid');
  if (!grid) {
    grid = el('div', 'photo-grid');
    grid.id = 'trash-grid';
    container.appendChild(grid);
  }
  const fragment = document.createDocumentFragment();
  newPhotos.forEach(p => {
    const thumb = makePhotoThumb(p, state.trashPhotos, { trashMode: true });
    fragment.appendChild(thumb);
  });
  grid.appendChild(fragment);
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
    appendMediaKindParam(params);
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
  try {
    const ids = await collectTrashPhotoIDs();
    if (!ids.length) {
      showToast('当前没有可恢复的媒体');
      return;
    }
    if (!confirm(`确定要恢复当前列表中的 ${ids.length} 条媒体吗？`)) return;
    for (const id of ids) {
      try {
        await api.post(`/api/media/${id}/restore`, {});
      } catch (e) {
        console.error('恢复失败:', id, e);
      }
    }
    showToast(`已恢复 ${ids.length} 条媒体`);
    switchView('trash');
  } catch (e) {
    alert('恢复全部失败: ' + ((e && e.error) || e.message || e));
  }
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

// ── 灯箱 ──────────────────────────────────────────────
function renderLightbox() {
  return `<div class="lightbox" id="lightbox" tabindex="-1">
  <div class="lightbox-header" id="lightbox-header">
    <div class="lightbox-header-main">
      <button class="btn-icon lightbox-back-btn" id="lb-close" title="返回" aria-label="返回">${icons.back}</button>
      <span class="lb-title" id="lb-title"></span>
    </div>
    <div class="lightbox-header-controls">
      <div class="lightbox-control-pill lightbox-zoom-panel">
        <label class="lightbox-zoom-wrap" for="lb-zoom">
          <span class="lightbox-zoom-step" aria-hidden="true">-</span>
          <input class="lightbox-slider" id="lb-zoom" type="range" min="50" max="300" step="10" aria-label="媒体缩放">
          <span class="lightbox-zoom-step" aria-hidden="true">+</span>
          <span id="lb-zoom-value">100%</span>
        </label>
        <button class="btn-icon lightbox-fit-height" id="lb-fit-height" title="适应（Alt + 0）" aria-label="适应">${icons.fit}</button>
      </div>
      <div class="lightbox-control-pill lightbox-slideshow-controls">
        <label class="lightbox-slider-wrap" for="lb-slideshow-interval">
          <span id="lb-slideshow-interval-value">5 秒</span>
          <input class="lightbox-slider" id="lb-slideshow-interval" type="range" min="1" max="30" step="1" aria-label="播放间隔">
        </label>
        <button class="btn-icon lightbox-fit-height lightbox-loop-toggle" id="lb-slideshow-loop" type="button" title="开启循环" aria-label="开启循环" aria-pressed="false"></button>
      </div>
    </div>
    <div class="lightbox-action-group">
      <button class="btn-icon" id="lb-download" title="下载" aria-label="下载">${icons.download}</button>
      <button class="btn-icon hidden" id="lb-video-bookmark" title="视频书签" aria-label="视频书签">${icons.bookmark}</button>
      <button class="btn-icon" id="lb-favorite" title="收藏" aria-label="收藏">${icons.favorite}</button>
      <button class="btn-icon" id="lb-share" title="分享" aria-label="分享">${icons.share}</button>
      <button class="btn-icon" id="lb-more" title="更多操作" aria-label="更多操作">${icons.more}</button>
      <button class="btn-icon lightbox-play-btn" id="lb-slideshow-toggle"><span class="lightbox-play-icon"></span><span class="lightbox-play-label">幻灯片</span></button>
    </div>
  </div>
  <div class="lightbox-body">
    <div class="lightbox-loading" id="lb-loading"><div class="spinner"></div><span>媒体加载中…</span></div>
    <div class="lightbox-media-frame" id="lb-frame">
      <img class="lightbox-img" id="lb-img" src="" alt="">
      <video class="lightbox-video hidden" id="lb-video" playsinline preload="metadata"></video>
    </div>
    <div class="lightbox-video-progress hidden" id="lb-video-progress" aria-hidden="true">
      <span class="lightbox-video-progress-time" id="lb-video-progress-current">0:00</span>
      <div class="lightbox-video-progress-track">
        <div class="lightbox-video-progress-fill" id="lb-video-progress-fill"></div>
        <div class="lightbox-video-progress-markers" id="lb-video-progress-markers"></div>
      </div>
      <span class="lightbox-video-progress-time" id="lb-video-progress-duration">0:00</span>
    </div>
    <div class="lightbox-volume-feedback" id="lb-volume-feedback"><div class="lightbox-volume-fill" id="lb-volume-fill"></div><span id="lb-volume-label">100%</span></div>
    <button class="lb-nav lb-prev" id="lb-prev">${icons.prev}</button>
    <button class="lb-nav lb-next" id="lb-next">${icons.next}</button>
  </div>
  <div class="lightbox-info" id="lb-info"></div>
</div>`;
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
    header.classList.toggle('lightbox-compact-controls', mode >= 3 || isMobileLayout());
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
  if (isMobileLayout()) {
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
  state.lightboxUiIdleTimer = setTimeout(() => {
    if ($('#lightbox')?.classList.contains('open')) lightbox.classList.add('ui-idle');
  }, 3000);
}
function clearLightboxUiIdleTimer() {
  clearTimeout(state.lightboxUiIdleTimer);
  state.lightboxUiIdleTimer = null;
  $('#lightbox')?.classList.remove('ui-idle');
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
  document.addEventListener('click', e => {
    if (!e.target.closest('.context-menu')) closeContextMenu();
  });
  window.addEventListener('scroll', () => saveViewScroll(), { passive: true });
  window.addEventListener('keydown', e => {
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
        switchView('albums');
        return;
      }
      closeContextMenu();
    }
    if (!$('#lightbox').classList.contains('open')) return;
    if (e.altKey || e.key === 'Alt') syncLightboxTemporaryZoom(true);
    if (handleLightboxKeydown(e)) return;
  }, true);
  window.addEventListener('keyup', e => {
    if (!$('#lightbox').classList.contains('open')) return;
    if (!e.altKey && e.key === 'Alt') syncLightboxTemporaryZoom(false);
  }, true);
  window.addEventListener('blur', () => resetLightboxTemporaryZoom());
  document.addEventListener('fullscreenchange', () => {
    const lightbox = $('#lightbox');
    if (!lightbox || !lightbox.classList.contains('open')) return;
    if (!document.fullscreenElement) exitLightboxToContext();
  });
  document.addEventListener('mousemove', e => {
    if (!$('#lightbox').classList.contains('open')) return;
    refreshLightboxUiActivity();
    updateLightboxFocusPointFromPointer(e.clientX, e.clientY);
    if (state.lightboxBoostActive && state.lightboxPointerInside) applyLightboxZoom();
  }, { passive: true });
  document.addEventListener('click', e => {
    if (e.target.closest('#lb-close')) exitLightboxToContext();
    if (e.target.closest('#lb-prev')) void lbNav(-1);
    if (e.target.closest('#lb-next')) void lbNav(1);
    if (e.target.closest('#lb-download')) downloadCurrentPhoto();
    if (e.target.closest('#lb-video-bookmark')) showVideoBookmarkMenu();
    if (e.target.closest('#lb-favorite')) toggleCurrentLightboxFavorite();
    if (e.target.closest('#lb-share')) lbShare();
    if (e.target.closest('#lb-more')) showCurrentLightboxContextMenu();
    if (e.target.closest('#lb-fit-height')) setLightboxFit();
    if (e.target.closest('#lb-slideshow-toggle')) toggleSlideshow();
    const bookmarkDelete = e.target.closest('[data-video-bookmark-delete]');
    if (bookmarkDelete) {
      e.preventDefault();
      e.stopPropagation();
      const slot = Number(bookmarkDelete.dataset.videoBookmarkDelete);
      const photo = state.lightboxPhotos[state.lightboxIndex];
      if (slot && deleteVideoBookmarkSlot(photo, slot)) {
        showToast(`已删除书签 ${slot}`);
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
          showToast(`已跳转到书签 ${slot}${bookmark.name ? `：${bookmark.name}` : ''}`);
        } else if (setVideoBookmarkSlot(photo, slot, video.currentTime || 0)) {
          showToast(`已添加书签 ${slot}`);
          refreshVideoBookmarkMenu();
        } else {
          showToast('附近 1 秒内已有书签，未新增');
        }
      }
    }
    const progressTrack = e.target.closest('.lightbox-video-progress-track');
    if (progressTrack) {
      seekVideoFromProgressClientX(progressTrack, e.clientX);
      return;
    }
    const sectionMarker = e.target.closest('[data-video-section]');
    if (sectionMarker) {
      const video = $('#lb-video');
      const section = Number(sectionMarker.dataset.videoSection);
      if (video && Number.isFinite(video.duration) && section > 0) {
        video.currentTime = video.duration / 10 * section;
        updateVideoBookmarkProgress(video);
        showVideoProgressActivity();
      }
      return;
    }
    if (handleLightboxVideoSurfaceClick(e)) return;
    if (shouldStopSlideshowFromClick(e.target)) stopSlideshow();
    if (e.target.closest('#global-search-btn, #floating-search-btn')) openGlobalSearch();
    if (e.target.closest('#search-close-btn')) closeGlobalSearch();
    if (e.target.closest('#search-overlay') && !e.target.closest('.search-panel')) closeGlobalSearch();
    if (e.target.closest('#search-more-btn') && state.searchHasMore) runGlobalSearch(state.searchQuery, { reset: false });
    if (e.target.closest('#search-download-all-btn')) downloadAllSearchResults();
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
    if (!$('#lightbox').classList.contains('open')) return;
    state.lightboxLastPointerType = e.pointerType || '';
    const progressTrack = e.target.closest('.lightbox-video-progress-track');
    if (progressTrack) beginVideoProgressScrub(e, progressTrack);
  }, true);
  document.addEventListener('pointermove', e => {
    if (state.videoProgressScrubPointerId != null) updateVideoProgressScrub(e);
  }, true);
  document.addEventListener('pointerup', e => {
    if (state.videoProgressScrubPointerId != null) endVideoProgressScrub(e);
  }, true);
  document.addEventListener('pointercancel', e => {
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
  window.addEventListener('storage', e => {
    if (e.key !== videoBookmarksStorageKey && e.key !== legacyVideoBookmarksStorageKey) return;
    state.videoBookmarks = loadVideoBookmarks();
    refreshVideoBookmarkThumbIndicators();
    updateVideoBookmarkButton();
    updateVideoBookmarkProgress();
  });
  document.addEventListener('change', async e => {
    if (!e.target.matches('#library-build-exit-after')) return;
    try {
      state.libraryBuildStatus = await setLibraryBuildExitAfterComplete(e.target.checked);
      renderView();
    } catch (err) {
      e.target.checked = !e.target.checked;
      alert('设置失败: ' + ((err && err.error) || err));
    }
  });
  document.addEventListener('click', async e => {
    const btn = e.target.closest('#library-build-cancel');
    if (!btn) return;
    btn.disabled = true;
    btn.textContent = copyText('app.libraryBuild.cancelling', '正在停止…');
    try {
      state.libraryBuildStatus = await cancelLibraryBuild();
      renderView();
      startLibraryBuildPolling();
    } catch (err) {
      btn.disabled = false;
      btn.textContent = copyText('app.libraryBuild.cancel', '停止构建');
      alert('停止失败: ' + ((err && err.error) || err));
    }
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
    if (e.target.matches('#lb-slideshow-interval')) updateSlideshowSetting('interval', parseInt(e.target.value, 10) * 1000);
    if (e.target.matches('#lb-zoom')) setLightboxZoom(parseInt(e.target.value, 10));
  });
  document.addEventListener('click', e => {
    if (e.target.closest('#lb-slideshow-loop')) {
      updateSlideshowSetting('loop', !state.slideshowLoop);
    }
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
async function openLightbox(photos, index, options = {}) {
  interruptThumbnailBuildForForegroundTask();
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
  state.lightboxSeedPhotoID = target && target.id ? Number(target.id) : null;
  state.lightboxSeedPreviewURL = currentLoadedPreviewURLForPhoto(target);
  state.lightboxReturnView = options.returnView || state.view;
  state.lightboxReturnAlbumID = state.currentAlbumID;
  state.slideshowRandomQueue = [];
  resetLightboxFocusPoint();
  resetLightboxTemporaryZoom();
  setLightboxMediaLoading(true);
  applyLightboxZoom();
  lockPageScrollForLightbox();
  $('#lightbox').classList.add('open');
  updateLightboxHeaderLayout();
  refreshLightboxUiActivity();
  focusLightboxKeyboardSurface();
  lbRender();
}
function closeLightbox() {
  stopSlideshow();
  clearLightboxUiIdleTimer();
  clearTimeout(state.lightboxVideoProgressTimer);
  clearTimeout(state.videoSurfaceTapTimer);
  state.videoProgressScrubPointerId = null;
  state.videoSurfaceLastTap = null;
  state.lightboxLastPointerType = '';
  $('#lb-video-progress')?.classList.remove('active', 'scrubbing', 'touching');
  state.lightboxPlaybackToken += 1;
  resetLightboxPrefetchCache();
  setLightboxMediaLoading(false);
  resetLightboxFocusPoint();
  resetLightboxTemporaryZoom();
  saveCurrentVideoResumePosition({ quiet: true });
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
  unlockPageScrollForLightbox();
  state.lightboxSeedPhotoID = null;
  state.lightboxSeedPreviewURL = '';
  state.lightboxReturnView = '';
  state.lightboxReturnAlbumID = null;
  state.lightboxPageLoading = false;
}
function exitLightboxToContext() {
  const returnView = state.lightboxReturnView;
  const returnAlbumID = state.lightboxReturnAlbumID;
  const currentPhotoID = state.lightboxPhotos[state.lightboxIndex]?.id || null;
  closeLightbox();
  if (currentPhotoID && returnView === 'album-detail') state.pendingAlbumPhotoID = currentPhotoID;
  if (currentPhotoID && returnView === 'timeline') state.pendingTimelinePhotoID = currentPhotoID;
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
    focusPendingAlbumPhoto();
    return;
  }
  if (currentPhotoID && returnView === 'timeline') {
    focusPendingTimelinePhoto();
    return;
  }
  if (currentPhotoID && ['favorites', 'random-album'].includes(returnView)) {
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
  return state.lightboxPhotos.length > beforeLength;
}

async function ensureLightboxIndexAvailable(index) {
  if (index < state.lightboxPhotos.length) return true;
  if (index < 0) return false;
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
  state.lightboxPlaybackToken += 1;
  const video = $('#lb-video');
  if (video) {
    saveCurrentVideoResumePosition({ quiet: true });
    video.pause();
    video.removeAttribute('src');
    video.removeAttribute('controls');
    video.removeAttribute('controlslist');
    video.removeAttribute('disablepictureinpicture');
    video.removeAttribute('disableremoteplayback');
    video.removeAttribute('x-webkit-airplay');
    video.load();
  }
  state.lightboxIndex = index;
  if (state.slideshowMode === 'random' && !options.fromSlideshow) resetRandomQueue();
  lbRender();
}
function lbRender() {
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  rememberPhoto(p);
  resetLightboxFocusPoint();
  const img = $('#lb-img');
  const video = $('#lb-video');
  setLightboxMediaLoading(true);
  if (isVideoMedia(p)) {
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
      showToast('当前浏览器无法播放这个视频文件，可在设置中手动启用系统播放器');
    };
    video.onfocus = refocusLightboxAfterVideoControl;
    video.ontimeupdate = () => {
      handleVideoResumeTimeUpdate(video);
      updateVideoBookmarkProgress(video, p);
    };
    video.onended = () => {
      if (!playNextVideoFromLightbox()) updateVideoBookmarkProgress(video, p);
    };
    restoreVideoPlaybackPreference(video, p);
    video.src = mediaFileURL(p);
    video.poster = mediaThumbURL(p);
    video.load();
    if (video.readyState >= 2) {
      setLightboxMediaLoading(false);
      applyLightboxZoom();
    }
    if (state.slideshowPlaying || state.experimentalAutoplayVideo || isWindowsClient()) autoplayLightboxVideo(video);
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
    video.onended = null;
    video.removeAttribute('src');
    video.removeAttribute('controls');
    video.removeAttribute('controlslist');
    video.removeAttribute('disablepictureinpicture');
    video.removeAttribute('disableremoteplayback');
    video.removeAttribute('x-webkit-airplay');
    video.load();
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
  updateVideoBookmarkButton(p);
  updateVideoBookmarkProgress(video, p);
  $('#lb-prev').classList.toggle('hidden', state.lightboxIndex === 0);
  $('#lb-next').classList.toggle('hidden', state.lightboxIndex === state.lightboxPhotos.length - 1 && !lightboxCanLoadMoreForward());
  applyLightboxZoom();
  updateLightboxHeaderLayout();
  updateSlideshowControls();
  if (state.slideshowPlaying) scheduleSlideshowStep();
  if (state.experimentalPrefetchNeighbors) prefetchAdjacentMedia();
  const items = [
    [isVideoMedia(p) ? icons.mediaVideo : icons.mediaImage, '类型', isVideoMedia(p) ? '视频' : '图片'],
    [icons.infoMime, 'MIME', p.mime_type || '—'],
    [icons.infoDate, '拍摄时间', formatDateTime(p.taken_at)],
    [icons.infoDimensions, '尺寸', p.width && p.height ? `${p.width} × ${p.height}` : '—'],
    [icons.infoRatio, '宽高比（近似）', aspectRatioLabel(p.width, p.height)],
    [icons.infoFileSize, '大小（MB）', formatSizeMB(p.size)],
  ].concat(buildExifInfoItems(p));
  $('#lb-info').innerHTML = items.map(([icon, k, v]) => `<div class="lb-info-item" title="${escapeHTML(`${k}: ${v}`)}" aria-label="${escapeHTML(`${k}: ${v}`)}"><span class="lb-info-key">${icon}</span><span class="lb-info-value">${escapeHTML(v)}</span></div>`).join('');
}

async function toggleCurrentLightboxFavorite() {
  const p = state.lightboxPhotos[state.lightboxIndex];
  if (!p) return;
  await toggleFavorite(p);
}
function prefetchAdjacentMedia() {
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

async function triggerPostDownload(url, payload, fallbackName, options = {}) {
	const res = await fetch(url, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
		signal: options.signal,
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
    await withButtonBusy(btn, '整理中…', async () => {
      const ids = await collectAllFavoriteIDs();
      if (!ids.length) {
        showToast('当前没有可下载的收藏媒体');
        return;
      }
      await triggerPostDownload('/api/media/download', {
        media_ids: ids,
      }, `echogallery-favorites-${Date.now()}.zip`);
    });
  } catch (e) {
    alert('下载失败: ' + (e.error || e));
  }
}

function buildSearchMediaParams(query, cursor = '') {
  const params = new URLSearchParams({ q: query, limit: '240' });
  if (state.searchFilter === 'photo') params.set('kind', 'image');
  if (state.searchFilter === 'video') params.set('kind', 'video');
  if (state.searchFilter === 'favorite') params.set('favorite', '1');
  if (cursor) params.set('cursor', cursor);
  return params;
}

async function collectAllSearchMediaIDs(query, signal) {
  const ids = [];
  let cursor = '';
  for (;;) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    const response = await fetch(`/api/media/search?${buildSearchMediaParams(query, cursor).toString()}`, { signal });
    if (!response.ok) throw await response.json();
    const page = await response.json();
    (page.photos || []).forEach(photo => {
      if (photo && photo.id) ids.push(photo.id);
    });
    if (!page.has_more || !page.next_cursor) break;
    cursor = page.next_cursor;
  }
  return ids;
}

async function downloadAllSearchResults() {
  if (state.searchDownloadAbortController) {
    state.searchDownloadAbortController.abort();
    return;
  }
  const query = $('#global-search-input')?.value.trim() || state.searchQuery;
  if (!query || state.searchFilter === 'album') {
    showToast('当前筛选没有可打包下载的媒体结果');
    return;
  }
  const btn = $('#search-download-all-btn');
  const controller = new AbortController();
  state.searchDownloadAbortController = controller;
  state.searchDownloadLoading = true;
  renderSearchResults();
  try {
    if (btn) btn.textContent = '取消下载';
    const ids = await collectAllSearchMediaIDs(query, controller.signal);
    if (!ids.length) {
      showToast('没有可下载的媒体结果');
      return;
    }
    if (controller.signal.aborted) throw new DOMException('Aborted', 'AbortError');
    await triggerPostDownload('/api/media/download', {
      media_ids: ids,
    }, `echogallery-search-${Date.now()}.zip`, { signal: controller.signal });
  } catch (e) {
    if (e && e.name === 'AbortError') {
      showToast('已取消打包下载');
      return;
    }
    alert('下载失败: ' + ((e && e.error) || e));
  } finally {
    if (state.searchDownloadAbortController === controller) {
      state.searchDownloadAbortController = null;
      state.searchDownloadLoading = false;
      renderSearchResults();
    }
  }
}

// ── 上传模态框 ────────────────────────────────────────
function renderUploadModal() {
  return `<div class="modal-overlay" id="upload-modal">
  <div class="modal" style="width:520px">
    <div class="modal-title">${icons.upload} ${escapeHTML(copyText('app.upload.title', '上传媒体'))}</div>
    <div class="upload-zone" id="drop-zone">
      ${icons.upload}
      <div style="margin-top:8px">${escapeHTML(copyText('app.upload.hint', '拖拽图片或视频到这里，或点击选择文件'))}</div>
      <div style="font-size:.8rem;margin-top:4px">${escapeHTML(copyText('app.upload.support', '支持 JPG、PNG、GIF、WebP、BMP、TIFF、MP4、MOV、M4V、WebM、MKV、AVI、WMV、WMA、MPEG、TS、3GP、OGV'))}</div>
      <input type="file" id="file-input" accept="image/*,.bmp,.tif,.tiff,.mp4,.mov,.m4v,.webm,.mkv,.avi,.wmv,.wma,.ts,.mts,.m2ts,.mpg,.mpeg,.3gp,.3g2,.ogv,video/*,audio/x-ms-wma" multiple aria-hidden="true">
    </div>
    <div class="upload-queue" id="upload-queue"></div>
    <div class="modal-footer">
      <button class="btn" id="retry-failed-btn" style="display:none">${escapeHTML(copyText('app.upload.retryFailed', '重传失败项'))}</button>
      <button class="btn" id="upload-close-btn">${escapeHTML(copyText('app.upload.close', '关闭'))}</button>
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
    row.innerHTML = `<span class="up-name">${file.name}</span><div style="flex:1"><div class="progress-bar"><div class="progress-fill" style="width:0%" id="prog-${id}"></div></div></div><span class="up-status" id="stat-${id}">${escapeHTML(copyText('app.upload.waiting', '等待中'))}</span><button class="btn btn-sm" id="retry-${id}" style="display:none">${escapeHTML(copyText('app.upload.retryFailed', '重传失败项'))}</button>`;
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
  if (stat) { stat.textContent = copyText('app.upload.uploading', '上传中'); stat.className = 'up-status'; }
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
      xhr.onerror = () => reject({ error: copyText('app.upload.networkError', '网络错误') });
      xhr.send(fd);
    });
    if (prog) prog.style.width = '100%';
    if (job) job.status = 'done';
    if (stat) { stat.textContent = copyText('app.upload.done', '完成'); stat.className = 'up-status done'; }
  } catch(e) {
    if (job) job.status = 'failed';
    if (stat) { stat.textContent = e.error || copyText('app.upload.failed', '失败'); stat.className = 'up-status error'; }
    if (retryBtn) retryBtn.style.display = '';
  }
  updateRetryFailedButton();
}

function updateRetryFailedButton() {
  const btn = $('#retry-failed-btn');
  if (!btn) return;
  const failedCount = state.uploadJobs.filter(j => j.status === 'failed').length;
  btn.style.display = failedCount > 0 ? '' : 'none';
  btn.textContent = failedCount > 0 ? `${copyText('app.upload.retryFailed', '重传失败项')}（${failedCount}）` : copyText('app.upload.retryFailed', '重传失败项');
}

function retrySingleUpload(id) {
  const job = state.uploadJobs.find(j => j.id === id);
  if (!job) return;
  job.status = 'queued';
  const prog = $(`#prog-${id}`);
  const stat = $(`#stat-${id}`);
  const retryBtn = $(`#retry-${id}`);
  if (prog) prog.style.width = '0%';
  if (stat) { stat.textContent = copyText('app.upload.retryWaiting', '等待重传'); stat.className = 'up-status'; }
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
      if (stat) { stat.textContent = copyText('app.upload.retryWaiting', '等待重传'); stat.className = 'up-status'; }
      if (retryBtn) retryBtn.style.display = 'none';
    }
  });
  if (found) runUploadQueue();
}

// ── 新建相册模态框 ────────────────────────────────────
function renderCreateAlbumModal() {
  return `<div class="modal-overlay" id="create-album-modal">
  <div class="modal">
    <div class="modal-title">${escapeHTML(copyText('app.createAlbum.title', '新建相册'))}</div>
    <div class="form-group"><label class="form-label">${escapeHTML(copyText('app.createAlbum.nameLabel', '相册名称'))}</label><input class="input" id="album-name-input" placeholder="${escapeHTML(copyText('app.createAlbum.namePlaceholder', '输入相册名称'))}" maxlength="50"></div>
    <div class="form-group"><label class="form-label">${escapeHTML(copyText('app.createAlbum.descLabel', '描述（可选）'))}</label><input class="input" id="album-desc-input" placeholder="${escapeHTML(copyText('app.createAlbum.descPlaceholder', '输入描述'))}"></div>
    <div class="modal-footer">
      <button class="btn" id="cancel-album-btn">${escapeHTML(copyText('app.createAlbum.cancel', '取消'))}</button>
      <button class="btn btn-primary" id="confirm-album-btn">${escapeHTML(copyText('app.createAlbum.confirm', '创建'))}</button>
    </div>
  </div>
</div>`;
}
function renderEditAlbumModal() {
  return `<div class="modal-overlay" id="edit-album-modal">
  <div class="modal">
    <div class="modal-title">编辑相册</div>
    <div class="form-group"><label class="form-label">相册名称</label><input class="input" id="edit-album-name-input" placeholder="输入相册名称" maxlength="50"></div>
    <div class="form-group"><label class="form-label">描述（可选）</label><input class="input" id="edit-album-desc-input" placeholder="输入描述"></div>
    <div class="modal-footer">
      <button class="btn" id="cancel-edit-album-btn">取消</button>
      <button class="btn btn-primary" id="confirm-edit-album-btn">保存</button>
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
function openEditAlbumModal(album = state.currentAlbum) {
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
  const name = $('#album-name-input').value.trim();
  if (!name) { alert(copyText('app.createAlbum.emptyNameAlert', '请输入相册名称')); return; }
  try {
		await api.post('/api/media/albums', { name, description: $('#album-desc-input').value.trim() });
    $('#create-album-modal').classList.remove('open');
    // 通过 switchView 而不是直接 render，确保菜单高亮和 hash 保持一致。
    switchView('albums');
  } catch(e) { alert('创建失败: ' + (e.error || e)); }
}
async function saveEditedAlbum() {
  const modal = $('#edit-album-modal');
  const albumID = Number(modal?.dataset.albumId || 0);
  const name = $('#edit-album-name-input').value.trim();
  if (!albumID) return;
  if (!name) {
    alert('请输入相册名称');
    return;
  }
  try {
    const updatedAlbum = await withButtonBusy($('#confirm-edit-album-btn'), '保存中…', () => api.put(`/api/media/albums/${albumID}`, {
      name,
      description: $('#edit-album-desc-input').value.trim(),
    }));
    syncAlbumState(updatedAlbum);
    modal.classList.remove('open');
    if (state.currentAlbum && Number(state.currentAlbum.id) === albumID) {
      $('#topbar-title').textContent = state.currentAlbum.name || '';
      const title = $('.album-detail-title');
      if (title) {
        title.textContent = state.currentAlbum.name || '';
        title.title = state.currentAlbum.name || '';
      }
    }
    showToast('相册已更新');
  } catch (e) {
    alert('更新相册失败: ' + (e.error || e));
  }
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
    <div class="modal-title">${icons.share} ${escapeHTML(copyText('app.share.title', '创建分享链接'))}</div>
    <div class="form-group">
      <label class="form-label">${escapeHTML(copyText('app.share.expireLabel', '过期时间'))}</label>
      <select class="input" id="share-expires">
        <option value="0">${escapeHTML(copyText('app.share.never', '永不过期'))}</option>
        <option value="7">${escapeHTML(copyText('app.share.day7', '7 天'))}</option>
        <option value="30">${escapeHTML(copyText('app.share.day30', '30 天'))}</option>
        <option value="90">${escapeHTML(copyText('app.share.day90', '90 天'))}</option>
      </select>
    </div>
    <div id="share-result" style="margin-top:12px;display:none">
      <label class="form-label">${escapeHTML(copyText('app.share.linkLabel', '分享链接'))}</label>
      <input class="input" id="share-link-input" readonly style="cursor:pointer">
      <p style="font-size:.8rem;color:var(--text2);margin-top:4px">${escapeHTML(copyText('app.share.copyHint', '点击链接复制'))}</p>
    </div>
    <div class="modal-footer">
      <button class="btn" id="share-cancel-btn">${escapeHTML(copyText('app.share.close', '关闭'))}</button>
      <button class="btn btn-primary" id="share-confirm-btn">${escapeHTML(copyText('app.share.confirm', '生成链接'))}</button>
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
    newInput.addEventListener('click', () => navigator.clipboard.writeText(url).then(() => showToast(copyText('app.share.copiedToast', '已复制分享链接'))));
  } catch(e) { alert('生成失败: ' + (e.error || e)); }
}

// ── 分享列表弹窗 (b-2) ───────────────────────────────
function renderShareListModal() {
  return `<div class="modal-overlay" id="share-list-modal">
  <div class="modal" style="width:480px">
    <div class="modal-title">${icons.share} ${escapeHTML(copyText('app.share.manageTitle', '管理分享链接'))}</div>
    <div id="share-list-content"></div>
    <div class="modal-footer">
      <button class="btn" id="share-list-close">${escapeHTML(copyText('app.share.close', '关闭'))}</button>
      <button class="btn btn-primary" id="share-list-add">${escapeHTML(copyText('app.share.addNew', '新建分享…'))}</button>
    </div>
  </div>
</div>`;
}

function renderLibraryLogoGuideModal() {
  return `<div class="modal-overlay" id="library-logo-guide-modal">
  <div class="modal library-logo-crop-modal">
    <div class="modal-title">${escapeHTML(copyText('app.logoCrop.title', '裁剪资源库头像'))}</div>
    <p class="modal-copy">${escapeHTML(copyText('app.logoCrop.copy', '拖动画面并调整缩放，保存后将按这个 1:1 构图作为资源库头像。'))}</p>
    <div class="library-logo-crop-stage" id="library-logo-crop-stage">
      <img id="library-logo-crop-img" alt="${escapeHTML(copyText('app.logoCrop.alt', '资源库头像预览'))}">
      <div class="library-logo-crop-mask" aria-hidden="true"></div>
    </div>
    <label class="library-logo-crop-control">
      <span>${escapeHTML(copyText('app.logoCrop.zoom', '缩放'))}</span>
      <input id="library-logo-crop-zoom" type="range" min="1" max="3" step="0.01" value="1">
    </label>
    <div class="modal-footer">
      <button class="btn" id="library-logo-guide-cancel">${escapeHTML(copyText('app.logoCrop.reselect', '重新选择'))}</button>
      <button class="btn btn-primary" id="library-logo-guide-confirm">${escapeHTML(copyText('app.logoCrop.confirm', '使用裁剪结果'))}</button>
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
  const crop = { x: 0, y: 0, zoom: 1, dragging: false, sx: 0, sy: 0, ox: 0, oy: 0 };
  state.pendingLibraryLogoCrop = crop;
  img.src = objectURL;
  zoom.value = '1';
  updateRangeProgress(zoom);
  const apply = () => {
    img.style.transform = `translate(${crop.x}px, ${crop.y}px) scale(${crop.zoom})`;
  };
  zoom.oninput = () => {
    updateRangeProgress(zoom);
    crop.zoom = Number(zoom.value) || 1;
    apply();
  };
  stage.onpointerdown = e => {
    crop.dragging = true;
    crop.sx = e.clientX;
    crop.sy = e.clientY;
    crop.ox = crop.x;
    crop.oy = crop.y;
    stage.setPointerCapture(e.pointerId);
  };
  stage.onpointermove = e => {
    if (!crop.dragging) return;
    crop.x = crop.ox + e.clientX - crop.sx;
    crop.y = crop.oy + e.clientY - crop.sy;
    apply();
  };
  stage.onpointerup = e => {
    crop.dragging = false;
    try { stage.releasePointerCapture(e.pointerId); } catch (_) {}
  };
  apply();
  modal.classList.add('open');
  $('#library-logo-guide-cancel').onclick = () => {
    modal.classList.remove('open');
    const pending = state.pendingLibraryLogoGuide;
    state.pendingLibraryLogoGuide = null;
    state.pendingLibraryLogoCrop = null;
    if (pending && pending.objectURL) URL.revokeObjectURL(pending.objectURL);
    if (pending && pending.onCancel) pending.onCancel();
  };
  $('#library-logo-guide-confirm').onclick = async () => {
    const pending = state.pendingLibraryLogoGuide;
    const activeCrop = state.pendingLibraryLogoCrop;
    const confirmBtn = $('#library-logo-guide-confirm');
    if (confirmBtn) {
      confirmBtn.disabled = true;
      confirmBtn.textContent = '正在裁剪…';
    }
    try {
      const cropped = await cropLibraryLogoFile(pending.file, activeCrop);
      modal.classList.remove('open');
      state.pendingLibraryLogoGuide = null;
      state.pendingLibraryLogoCrop = null;
      if (pending && pending.onConfirm) pending.onConfirm(cropped);
    } catch (err) {
      showToast((err && err.message) || '头像裁剪失败，请重新选择');
    } finally {
      if (confirmBtn) {
        confirmBtn.disabled = false;
        confirmBtn.textContent = '使用裁剪结果';
      }
      if (pending && pending.objectURL) URL.revokeObjectURL(pending.objectURL);
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
          const stageSize = stage ? stage.getBoundingClientRect().width : 280;
          const baseScale = Math.max(stageSize / img.naturalWidth, stageSize / img.naturalHeight);
          const scale = baseScale * ((crop && crop.zoom) || 1);
          const drawnW = img.naturalWidth * scale;
          const drawnH = img.naturalHeight * scale;
          const offsetX = (stageSize - drawnW) / 2 + ((crop && crop.x) || 0);
          const offsetY = (stageSize - drawnH) / 2 + ((crop && crop.y) || 0);
          sourceX = Math.max(0, -offsetX / scale);
          sourceY = Math.max(0, -offsetY / scale);
          sourceW = Math.min(img.naturalWidth - sourceX, stageSize / scale);
          sourceH = Math.min(img.naturalHeight - sourceY, stageSize / scale);
        }
        ctx.drawImage(img, sourceX, sourceY, sourceW, sourceH, 0, 0, edge, edge);
        canvas.toBlob(blob => {
          URL.revokeObjectURL(url);
          if (!blob) {
            reject(new Error('头像裁剪失败'));
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
      reject(new Error('无法读取头像图像'));
    };
    img.src = url;
  });
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
async function shutdownFromUI() {
  try {
    await shutdownApp();
    showToast('EchoGallery 正在退出');
    setTimeout(() => {
      document.body.innerHTML = `<div class="empty"><p>${escapeHTML(copyText('app.logout.closed', 'EchoGallery 已退出，可以关闭此页面。'))}</p></div>`;
    }, 500);
  } catch (e) {
    alert('退出失败: ' + ((e && e.error) || e));
  }
}

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
  if (['timeline','favorites','random-album','albums','memories','trash','settings'].includes(h)) {
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
    if (['timeline','favorites','random-album','albums','memories','trash','settings'].includes(last)) {
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
  if (['timeline','favorites','random-album','albums','memories','trash','settings'].includes(view)) {
    history.replaceState(null, '', '#' + view);
    if (state.experimentalRestoreLastView) localStorage.setItem('last_view_hash', view);
  }
}
const initialRoute = getHashView();
state.view = initialRoute.view;
state.currentAlbumID = initialRoute.albumID;
async function bootstrapApp() {
  await Promise.all([
    loadCopyCatalog(),
    loadInlineSVGIcons(),
  ]);
  renderApp();
}
bootstrapApp();
