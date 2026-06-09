(function () {
  'use strict';

  var root = document.getElementById('gallery-chooser-root');
  if (!root) return;

  var copyDefaults = {
    pageTitle: 'Gallery 选择',
    brand: 'EchoGallery',
    status: '找不到服务器',
    title: '选择其他 Gallery',
    message: '当前 Web App 对应的地址暂时不可用。你可以从最近打开的 Gallery 继续，或手动输入新的地址。',
    currentAddress: '当前地址',
    recentTitle: '最近打开',
    recentCount: '{count} 个记录',
    recentEmpty: '这里还没有最近打开的 Gallery 记录。你可以在下方输入新的地址。',
    unavailable: '当前不可达',
    open: '打开',
    remove: '移除',
    manualTitle: '选择其他 Gallery',
    manualHint: '输入 IP、域名或完整 URL',
    manualNameLabel: '显示名称（可选）',
    manualNamePlaceholder: '例如：客厅 NAS',
    manualUrlLabel: 'Gallery 地址',
    manualUrlPlaceholder: '例如：192.168.1.8:8080',
    submit: '打开 Gallery',
    invalidURL: '请输入有效的 Gallery 地址',
    lastSeen: '上次打开 {time}',
    justNow: '刚刚',
    minutesAgo: '{count} 分钟前',
    hoursAgo: '{count} 小时前',
    daysAgo: '{count} 天前'
  };
  var copy = JSON.parse(JSON.stringify(copyDefaults));

  function esc(text) {
    return String(text || '').replace(/[&<>"]/g, function (m) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[m];
    });
  }

  function mergeCopy(target, source) {
    if (!source || typeof source !== 'object') return target;
    Object.keys(source).forEach(function (key) {
      var value = source[key];
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        if (!target[key] || typeof target[key] !== 'object' || Array.isArray(target[key])) {
          target[key] = {};
        }
        mergeCopy(target[key], value);
        return;
      }
      target[key] = value;
    });
    return target;
  }

  function text(path, fallback, params) {
    var current = copy;
    String(path || '').split('.').forEach(function (segment) {
      if (current && typeof current === 'object' && segment in current) current = current[segment];
      else current = undefined;
    });
    var value = typeof current === 'string' ? current : fallback;
    return String(value || '').replace(/\{(\w+)\}/g, function (_, key) {
      return params && key in params ? String(params[key]) : '{' + key + '}';
    });
  }

  function formatLastSeen(timestamp) {
    var value = Math.max(0, Number(timestamp) || 0);
    if (!value) return '';
    var delta = Math.max(0, Math.round((Date.now() - value) / 60000));
    if (delta < 1) return text('justNow', '刚刚');
    if (delta < 60) return text('minutesAgo', '{count} 分钟前', { count: delta });
    if (delta < 1440) return text('hoursAgo', '{count} 小时前', { count: Math.round(delta / 60) });
    return text('daysAgo', '{count} 天前', { count: Math.round(delta / 1440) });
  }

  function galleryInitial(gallery) {
    var label = String((gallery && (gallery.displayName || gallery.title)) || 'E').trim();
    return esc((label.charAt(0) || 'E').toUpperCase());
  }

  function renderRecentCards(recents) {
    if (!recents.length) {
      return '<div class="setup-recovery-empty">' + esc(text('recentEmpty', '这里还没有最近打开的 Gallery 记录。你可以在下方输入新的地址。')) + '</div>';
    }
    return recents.map(function (gallery, index) {
      var unavailable = gallery.origin === location.origin;
      var icon = gallery.icon
        ? '<img class="gallery-chooser-option-image" src="' + esc(gallery.icon) + '" alt="' + esc(gallery.displayName || gallery.title || 'EchoGallery') + '">'
        : '<span class="gallery-chooser-option-fallback">' + galleryInitial(gallery) + '</span>';
      var lastSeen = formatLastSeen(gallery.lastSeen);
      return '' +
        '<div class="setup-library-option gallery-chooser-option' + (unavailable ? ' current' : '') + '" data-gallery-url="' + esc(gallery.url) + '" data-gallery-name="' + esc(gallery.displayName || gallery.title || '') + '" data-gallery-icon="' + esc(gallery.icon || '') + '" data-gallery-theme-color="' + esc(gallery.themeColor || '#2d6a5f') + '">' +
          '<span class="setup-library-option-dot gallery-chooser-option-dot" style="--gallery-chooser-accent:' + esc(gallery.themeColor || '#2d6a5f') + '">' + icon + '</span>' +
          '<span class="setup-library-option-main gallery-chooser-option-main">' +
            '<strong>' + esc(gallery.displayName || gallery.title || gallery.origin || gallery.url) + '</strong>' +
            '<small>' + esc(gallery.url) + '</small>' +
            (lastSeen ? '<small class="gallery-chooser-option-meta">' + esc(text('lastSeen', '上次打开 {time}', { time: lastSeen })) + '</small>' : '') +
          '</span>' +
          '<span class="gallery-chooser-option-actions">' +
            '<button class="btn btn-sm gallery-chooser-open" type="button" data-gallery-open="' + esc(gallery.url) + '" data-gallery-name="' + esc(gallery.displayName || gallery.title || '') + '" data-gallery-icon="' + esc(gallery.icon || '') + '" data-gallery-theme-color="' + esc(gallery.themeColor || '#2d6a5f') + '">' + esc(text('open', '打开')) + '</button>' +
            '<button class="btn btn-sm gallery-chooser-remove" type="button" data-gallery-remove="' + esc(gallery.url) + '">' + esc(text('remove', '移除')) + '</button>' +
          '</span>' +
          (unavailable ? '<em>' + esc(text('unavailable', '当前不可达')) + '</em>' : '') +
        '</div>';
    }).join('');
  }

  function render() {
    var pwa = window.EchoGalleryPWA || {};
    var recents = typeof pwa.loadRecentGalleries === 'function' ? pwa.loadRecentGalleries() : [];
    document.title = text('pageTitle', 'Gallery 选择');
    root.innerHTML =
      '<div class="setup-recovery-hero gallery-chooser-hero">' +
        '<div class="setup-recovery-brand"><span class="setup-recovery-mark">E</span><span>' + esc(text('brand', 'EchoGallery')) + '</span></div>' +
        '<span class="setup-recovery-status">' + esc(text('status', '找不到服务器')) + '</span>' +
        '<h1>' + esc(text('title', '选择其他 Gallery')) + '</h1>' +
        '<p>' + esc(text('message', '当前 Web App 对应的地址暂时不可用。你可以从最近打开的 Gallery 继续，或手动输入新的地址。')) + '</p>' +
        '<div class="setup-recovery-current"><span>' + esc(text('currentAddress', '当前地址')) + '</span><strong>' + esc(location.origin || location.href) + '</strong></div>' +
      '</div>' +
      '<div class="setup-recovery-grid gallery-chooser-grid">' +
        '<section class="setup-recovery-section">' +
          '<div class="setup-recovery-section-head"><strong>' + esc(text('recentTitle', '最近打开')) + '</strong><span>' + esc(text('recentCount', '{count} 个记录', { count: recents.length })) + '</span></div>' +
          '<div class="setup-library-chooser gallery-chooser-list" id="gallery-chooser-list">' + renderRecentCards(recents) + '</div>' +
        '</section>' +
        '<section class="setup-recovery-section setup-recovery-create gallery-chooser-create">' +
          '<div class="setup-recovery-section-head"><strong>' + esc(text('manualTitle', '选择其他 Gallery')) + '</strong><span>' + esc(text('manualHint', '输入 IP、域名或完整 URL')) + '</span></div>' +
          '<div class="form-group"><label class="form-label" for="gallery-chooser-name">' + esc(text('manualNameLabel', '显示名称（可选）')) + '</label><input class="input" id="gallery-chooser-name" type="text" placeholder="' + esc(text('manualNamePlaceholder', '例如：客厅 NAS')) + '"></div>' +
          '<div class="form-group"><label class="form-label" for="gallery-chooser-url">' + esc(text('manualUrlLabel', 'Gallery 地址')) + '</label><input class="input" id="gallery-chooser-url" type="text" placeholder="' + esc(text('manualUrlPlaceholder', '例如：192.168.1.8:8080')) + '" autocapitalize="off" spellcheck="false"></div>' +
          '<div class="setup-recovery-footer gallery-chooser-footer">' +
            '<button class="btn btn-primary" id="gallery-chooser-submit">' + esc(text('submit', '打开 Gallery')) + '</button>' +
            '<div class="login-error" id="gallery-chooser-error"></div>' +
          '</div>' +
        '</section>' +
      '</div>';

    bindEvents();
  }

  function bindEvents() {
    var pwa = window.EchoGalleryPWA || {};
    var list = document.getElementById('gallery-chooser-list');
    var submit = document.getElementById('gallery-chooser-submit');
    var input = document.getElementById('gallery-chooser-url');
    var nameInput = document.getElementById('gallery-chooser-name');
    var error = document.getElementById('gallery-chooser-error');

    function openGallery(rawURL, meta) {
      var normalize = typeof pwa.normalizeGalleryURL === 'function' ? pwa.normalizeGalleryURL : function (value) { return String(value || '').trim(); };
      var url = normalize(rawURL);
      if (!url) {
        if (error) error.textContent = text('invalidURL', '请输入有效的 Gallery 地址');
        return;
      }
      if (error) error.textContent = '';
      if (typeof pwa.recordCurrentGallery === 'function') {
        var displayName = meta && meta.displayName ? meta.displayName : (nameInput && nameInput.value.trim()) || url;
        pwa.recordCurrentGallery({
          url: url,
          displayName: displayName,
          title: displayName ? (displayName + ' - EchoGallery') : 'EchoGallery',
          icon: meta && meta.icon || '',
          themeColor: meta && meta.themeColor || '',
        });
      }
      location.href = url;
    }

    if (list) {
      list.addEventListener('click', function (event) {
        var removeButton = event.target.closest('[data-gallery-remove]');
        if (removeButton) {
          event.preventDefault();
          if (typeof pwa.removeRecentGallery === 'function') pwa.removeRecentGallery(removeButton.getAttribute('data-gallery-remove'));
          render();
          return;
        }
        var openButton = event.target.closest('[data-gallery-open]');
        if (openButton) {
          event.preventDefault();
          openGallery(openButton.getAttribute('data-gallery-open'), {
            displayName: openButton.getAttribute('data-gallery-name') || '',
            icon: openButton.getAttribute('data-gallery-icon') || '',
            themeColor: openButton.getAttribute('data-gallery-theme-color') || '',
          });
          return;
        }
        var card = event.target.closest('[data-gallery-url]');
        if (card) openGallery(card.getAttribute('data-gallery-url'), {
          displayName: card.getAttribute('data-gallery-name') || '',
          icon: card.getAttribute('data-gallery-icon') || '',
          themeColor: card.getAttribute('data-gallery-theme-color') || '',
        });
      });
    }
    if (submit) {
      submit.addEventListener('click', function () {
        openGallery(input && input.value);
      });
    }
    if (input) {
      input.addEventListener('keydown', function (event) {
        if (event.key === 'Enter') {
          event.preventDefault();
          openGallery(input.value);
        }
      });
    }
  }

  fetch('/static/strings/zh-CN.json', { cache: 'no-store' })
    .then(function (response) { return response.ok ? response.json() : null; })
    .then(function (catalog) {
      if (catalog && catalog.pages && catalog.pages.galleryChooser) {
        mergeCopy(copy, catalog.pages.galleryChooser);
      }
    })
    .catch(function () {})
    .finally(render);
})();
