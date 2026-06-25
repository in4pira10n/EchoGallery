(function () {
  const DEFAULT_ICON_FILES = {
    download: 'download.svg',
    video: 'media-video.svg',
    image: 'media-image.svg',
    date: 'info-date.svg',
    dimensions: 'info-dimensions.svg',
    fileSize: 'info-file-size.svg',
    mime: 'info-mime.svg',
    fit: 'fit.svg',
    prev: 'prev.svg',
    next: 'next.svg',
  };

  function normalizeIcon(svg) {
    return String(svg || '')
      .replace(/\sfill="black"/g, ' fill="currentColor"')
      .replace(/\sstroke="black"/g, ' stroke="currentColor"')
      .replace(/\sfill="#000(?:000)?"/gi, ' fill="currentColor"')
      .replace(/\sstroke="#000(?:000)?"/gi, ' stroke="currentColor"');
  }

  function loadIcons(iconFiles = DEFAULT_ICON_FILES) {
    const icons = {};
    return Promise.all(Object.keys(iconFiles).map(key => (
      fetch(`/static/svg/${iconFiles[key]}`, { cache: 'force-cache' })
        .then(r => (r.ok ? r.text() : ''))
        .then(svg => { icons[key] = normalizeIcon(svg); })
        .catch(() => { icons[key] = ''; })
    ))).then(() => icons);
  }

  function escapeHTML(value) {
    return String(value == null ? '' : value)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function addPanguSpacing(text) {
    return String(text || '')
      .replace(/([\u4e00-\u9fff])([A-Za-z0-9@#&%+=/.-])/g, '$1 $2')
      .replace(/([A-Za-z0-9@#&%+=/.-])([\u4e00-\u9fff])/g, '$1 $2');
  }

  function formatSizeMB(bytes) {
    const value = Number(bytes);
    if (!Number.isFinite(value) || value < 0) return '';
    return `${(value / 1048576).toFixed(1)} MB`;
  }

  function formatMediaDateTime(value) {
    if (!value) return '';
    const date = new Date(value);
    if (!Number.isFinite(date.getTime())) return '';
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    });
  }

  function aspectRatioLabel(width, height) {
    const w = Number(width) || 0;
    const h = Number(height) || 0;
    if (!w || !h) return '';
    const ratio = w / h;
    const commonRatios = [[1, 1], [4, 3], [3, 4], [3, 2], [2, 3], [16, 9], [9, 16], [16, 10], [10, 16], [5, 4], [4, 5], [18, 9], [21, 9]];
    let best = null;
    let bestDiff = Infinity;
    commonRatios.forEach(pair => {
      const diff = Math.abs(Math.log(ratio / (pair[0] / pair[1])));
      if (diff < bestDiff) {
        bestDiff = diff;
        best = pair;
      }
    });
    if (best && bestDiff <= 0.06) return `${best[0]}:${best[1]}`;
    const gcd = (a, b) => (b ? gcd(b, a % b) : a);
    const divisor = gcd(w, h) || 1;
    return `${Math.round(w / divisor)}:${Math.round(h / divisor)}`;
  }

  function renderInfoItems(items) {
    return items
      .filter(item => item && String(item[2] || '').trim())
      .map(item => {
        const value = addPanguSpacing(item[2]);
        return `<div class="lb-info-item" title="${escapeHTML(`${item[1]}: ${value}`)}" aria-label="${escapeHTML(`${item[1]}: ${value}`)}"><span class="lb-info-key">${item[0] || ''}</span><span class="lb-info-value">${escapeHTML(value)}</span></div>`;
      })
      .join('');
  }

  function renderShell(options = {}) {
    const icons = options.icons || {};
    const mode = options.mode || 'app';
    const isShare = mode === 'share';
    const canWriteMedia = options.canWriteMedia !== false;
    const canManageShareLinks = options.canManageShareLinks !== false;
    const rootClass = `lightbox${isShare ? ' open is-share-lightbox' : ''}`;
    const rootStyle = isShare ? ' style="display:flex"' : '';
    const headerClass = `lightbox-header${isShare ? ' lightbox-hide-slideshow-controls' : ''}`;
    const closeClass = `btn-icon lightbox-back-btn${isShare ? ' hidden' : ''}`;
    const videoBookmark = isShare ? '' : `<button class="btn-icon hidden" id="lb-video-bookmark" title="视频书签" aria-label="视频书签">${icons.bookmarkBlank || icons.bookmark || ''}</button>`;
    const favorite = isShare ? '' : `<button class="btn-icon${canWriteMedia ? '' : ' hidden'}" id="lb-favorite" title="收藏" aria-label="收藏">${icons.favorite || ''}</button>`;
    const share = isShare ? '' : `<button class="btn-icon${canManageShareLinks ? '' : ' hidden'}" id="lb-share" title="分享" aria-label="分享">${icons.share || ''}</button>`;
    const more = isShare ? '' : `<button class="btn-icon" id="lb-more" title="更多操作" aria-label="更多操作">${icons.more || ''}</button>`;
    const slideshowToggle = isShare ? '' : `<button class="btn-icon lightbox-play-btn" id="lb-slideshow-toggle"><span class="lightbox-play-icon"></span></button>`;
    const download = isShare ? `<a class="btn-icon hidden" id="lb-download" href="#" aria-label="下载" title="下载" download></a>` : '';
    const slideshowClass = `lightbox-control-pill lightbox-slideshow-controls${isShare ? ' hidden' : ''}`;
    const imgStyle = isShare ? ' style="display:block"' : '';
    const bodyID = isShare ? ' id="share-body"' : '';
    return `<div class="${rootClass}" id="lightbox" tabindex="-1"${rootStyle}>
  <div class="${headerClass}" id="lightbox-header">
    <div class="lightbox-header-main">
      <button class="${closeClass}" id="lb-close" title="返回" aria-label="返回">${icons.back || ''}</button>
      <span class="lb-title" id="lb-title"></span>
    </div>
    <div class="lightbox-header-controls">
      <div class="lightbox-control-pill lightbox-zoom-panel">
        <label class="lightbox-zoom-wrap" for="lb-zoom">
          <span class="lightbox-zoom-step" role="button" tabindex="0" data-lightbox-zoom-step="-10" aria-label="缩小 10%">-</span>
          <input class="lightbox-slider" id="lb-zoom" type="range" min="50" max="500" step="10" value="100" aria-label="媒体缩放">
          <span class="lightbox-zoom-step" role="button" tabindex="0" data-lightbox-zoom-step="10" aria-label="放大 10%">+</span>
          <span id="lb-zoom-value">100%</span>
        </label>
        <button class="btn-icon lightbox-fit-height" id="lb-fit-height" title="适应（Alt + 0）" aria-label="适应">${icons.fit || ''}</button>
      </div>
      <div class="${slideshowClass}">
        <label class="lightbox-slider-wrap" for="lb-slideshow-interval">
          <span id="lb-slideshow-interval-value">5 秒</span>
          <input class="lightbox-slider" id="lb-slideshow-interval" type="range" min="1" max="30" step="1" aria-label="播放间隔">
        </label>
        <button class="btn-icon lightbox-fit-height lightbox-loop-toggle" id="lb-slideshow-loop" type="button" title="开启循环" aria-label="开启循环" aria-pressed="false"></button>
      </div>
    </div>
    <div class="lightbox-action-group">
      ${videoBookmark}
      ${favorite}
      ${share}
      ${more}
      ${slideshowToggle}
      ${download}
    </div>
  </div>
  <div class="lightbox-body"${bodyID}>
    <div class="lightbox-loading${isShare ? ' show' : ''}" id="lb-loading"><div class="spinner"></div><span>${isShare ? '加载中…' : '媒体加载中…'}</span></div>
    <div class="lightbox-media-frame" id="lb-frame">
      <img class="lightbox-img${isShare ? ' hidden' : ''}" id="lb-img" src="" alt="" draggable="false"${imgStyle}>
      <video class="lightbox-video hidden" id="lb-video" ${isShare ? 'controls ' : ''}playsinline preload="metadata"></video>
      <div class="lightbox-swipe-strip" id="lb-swipe-strip" aria-hidden="true">
        <div class="lightbox-swipe-divider" aria-hidden="true"></div>
        <img class="lightbox-swipe-preview" id="lb-swipe-prev" alt="" draggable="false">
        <img class="lightbox-swipe-preview" id="lb-swipe-current" alt="" draggable="false">
        <img class="lightbox-swipe-preview" id="lb-swipe-next" alt="" draggable="false">
      </div>
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
    <button class="lb-touch-playback-toggle hidden" id="lb-touch-playback-toggle" type="button" title="播放视频" aria-label="播放视频" aria-pressed="false">
      <span id="lb-touch-playback-icon">${icons.play || ''}</span>
    </button>
    <button class="lb-nav lb-prev${isShare ? ' hidden' : ''}" id="lb-prev">${icons.prev || ''}</button>
    <button class="lb-nav lb-next${isShare ? ' hidden' : ''}" id="lb-next">${icons.next || ''}</button>
  </div>
  <div class="lightbox-info" id="lb-info"></div>
</div>`;
  }

  function createShareController(options = {}) {
    const token = options.token || '';
    const iconFiles = options.iconFiles || DEFAULT_ICON_FILES;
    let icons = {};
    let currentItems = [];
    let currentIndex = 0;
    let albumMode = false;
    let shareZoom = 100;
    let pinchGesture = null;

    const byID = id => document.getElementById(id);
    const mediaUrl = item => `/media/s/${token}/${encodeURIComponent(item.uuid || item.target_uuid || item.id || item.target_id || '')}`;

    function setLoading(text) {
      byID('lb-download')?.classList.add('hidden');
      byID('lb-img')?.classList.add('hidden');
      byID('lb-video')?.classList.add('hidden');
      if (byID('lb-info')) byID('lb-info').innerHTML = '';
      byID('lb-loading')?.classList.add('show');
      const label = document.querySelector('#lb-loading span');
      if (label) label.textContent = text || '加载中…';
    }

    function cssPx(value) {
      const parsed = parseFloat(value);
      return Number.isFinite(parsed) ? parsed : 0;
    }

    function viewportMetrics() {
      const body = byID('share-body') || document.querySelector('#lightbox .lightbox-body');
      if (!body) return { width: 1, height: 1, paddingLeft: 0, paddingTop: 0 };
      const style = window.getComputedStyle(body);
      const paddingLeft = cssPx(style.paddingLeft);
      const paddingRight = cssPx(style.paddingRight);
      const paddingTop = cssPx(style.paddingTop);
      const paddingBottom = cssPx(style.paddingBottom);
      return {
        width: Math.max(1, body.clientWidth - paddingLeft - paddingRight),
        height: Math.max(1, body.clientHeight - paddingTop - paddingBottom),
        paddingLeft,
        paddingTop,
      };
    }

    function clamp(value, min, max) {
      return Math.min(max, Math.max(min, Number(value) || 0));
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

    function focusPointFromClient(clientX, clientY, media) {
      const body = byID('share-body') || document.querySelector('#lightbox .lightbox-body');
      if (!body || !media) return null;
      const bodyRect = body.getBoundingClientRect();
      const mediaRect = media.getBoundingClientRect();
      const viewport = viewportMetrics();
      return {
        mediaX: clamp((clientX - mediaRect.left) / Math.max(1, mediaRect.width), 0, 1),
        mediaY: clamp((clientY - mediaRect.top) / Math.max(1, mediaRect.height), 0, 1),
        viewportX: clamp((clientX - bodyRect.left - viewport.paddingLeft) / Math.max(1, viewport.width), 0, 1),
        viewportY: clamp((clientY - bodyRect.top - viewport.paddingTop) / Math.max(1, viewport.height), 0, 1),
      };
    }

    function syncZoomUI() {
      const input = byID('lb-zoom');
      const label = byID('lb-zoom-value');
      if (input) input.value = String(shareZoom);
      if (label) label.textContent = `${shareZoom}%`;
    }

    function applyFit(scalePercent, focusPoint) {
      const item = currentItems[currentIndex] || {};
      const isVideo = item.media_kind === 'video' || item.target_media_kind === 'video';
      const media = byID(isVideo ? 'lb-video' : 'lb-img');
      if (!media || media.classList.contains('hidden')) return;
      const naturalWidth = Number(item.width) || media.naturalWidth || media.videoWidth || 0;
      const naturalHeight = Number(item.height) || media.naturalHeight || media.videoHeight || 0;
      if (!naturalWidth || !naturalHeight) return;
      const viewport = viewportMetrics();
      const zoom = Math.max(0.5, Math.min(5, Number(scalePercent || 100) / 100));
      const fitScale = Math.min(viewport.width / naturalWidth, viewport.height / naturalHeight) * zoom;
      const targetWidth = Math.max(1, Math.floor(naturalWidth * fitScale));
      const targetHeight = Math.max(1, Math.floor(naturalHeight * fitScale));
      const body = byID('share-body') || document.querySelector('#lightbox .lightbox-body');
      media.style.transform = '';
      media.style.maxWidth = 'none';
      media.style.maxHeight = 'none';
      media.style.width = `${targetWidth}px`;
      media.style.height = `${targetHeight}px`;
      if (body) {
        body.classList.toggle('zoomed', targetWidth > viewport.width || targetHeight > viewport.height);
        window.requestAnimationFrame(() => {
          const point = focusPoint || { mediaX: 0.5, mediaY: 0.5, viewportX: 0.5, viewportY: 0.5 };
          const bodyRect = body.getBoundingClientRect();
          const mediaRect = media.getBoundingClientRect();
          const mediaLeft = mediaRect.left - bodyRect.left - viewport.paddingLeft + body.scrollLeft;
          const mediaTop = mediaRect.top - bodyRect.top - viewport.paddingTop + body.scrollTop;
          const maxScrollLeft = Math.max(0, body.scrollWidth - body.clientWidth);
          const maxScrollTop = Math.max(0, body.scrollHeight - body.clientHeight);
          body.scrollLeft = clamp(mediaLeft + targetWidth * point.mediaX - viewport.width * point.viewportX, 0, maxScrollLeft);
          body.scrollTop = clamp(mediaTop + targetHeight * point.mediaY - viewport.height * point.viewportY, 0, maxScrollTop);
        });
      }
    }

    function setShareZoom(value, options = {}) {
      shareZoom = clamp(value, 50, 500);
      syncZoomUI();
      applyFit(shareZoom, options.focusPoint);
    }

    function updateDownload(item) {
      const link = byID('lb-download');
      if (!link) return;
      link.href = item.download_url || mediaUrl(item);
      link.innerHTML = icons.download || '';
      link.classList.remove('hidden');
    }

    function clearAlbumGrid() {
      byID('share-grid')?.remove();
    }

    function showItem(index) {
      if (!currentItems.length) return;
      clearAlbumGrid();
      currentIndex = Math.max(0, Math.min(index, currentItems.length - 1));
      const item = currentItems[currentIndex];
      const url = mediaUrl(item);
      const name = item.original_name || item.target_original_name || 'shared-media';
      const isVideo = item.media_kind === 'video' || item.target_media_kind === 'video';
      const img = byID('lb-img');
      const video = byID('lb-video');
      const frame = byID('lb-frame');
      byID('lb-loading')?.classList.remove('show');
      ['width', 'padding', 'box-sizing', 'overflow'].forEach(prop => frame?.style.removeProperty(prop));
      shareZoom = 100;
      syncZoomUI();
      [img, video].forEach(el => {
        if (!el) return;
        el.style.transform = '';
        el.style.width = '';
        el.style.height = '';
      });
      img?.classList.toggle('hidden', isVideo);
      video?.classList.toggle('hidden', !isVideo);
      if (isVideo) {
        img?.removeAttribute('src');
        video.onloadedmetadata = () => applyFit();
        video.onloadeddata = () => {
          byID('lb-loading')?.classList.remove('show');
          applyFit();
        };
        video.src = url;
      } else {
        video?.pause();
        video?.removeAttribute('src');
        video?.load();
        img.onload = () => {
          byID('lb-loading')?.classList.remove('show');
          applyFit();
        };
        img.src = url;
        img.alt = name;
        if (img.complete) {
          window.requestAnimationFrame(() => {
            byID('lb-loading')?.classList.remove('show');
            applyFit();
          });
        }
      }
      byID('lb-title').textContent = name;
      byID('lb-info').innerHTML = renderInfoItems([
        [isVideo ? icons.video : icons.image, '类型', isVideo ? '视频' : '图片'],
        [icons.mime, 'MIME', item.mime_type || item.target_mime_type || ''],
        [icons.date, '拍摄时间', formatMediaDateTime(item.taken_at || item.created_at)],
        [icons.dimensions, '尺寸', item.width && item.height ? `${item.width} × ${item.height}` : ''],
        [icons.dimensions, '宽高比（近似）', isVideo ? '' : aspectRatioLabel(item.width, item.height)],
        [icons.fileSize, '大小（MB）', formatSizeMB(item.size)],
      ]);
      applyFit();
      updateDownload(item);
      byID('lb-prev')?.classList.toggle('hidden', !albumMode || currentIndex === 0);
      byID('lb-next')?.classList.toggle('hidden', !albumMode || currentIndex === currentItems.length - 1);
    }

    function renderAlbum(items) {
      albumMode = true;
      currentItems = items || [];
      ['lb-download', 'lb-prev', 'lb-next', 'lb-img', 'lb-video'].forEach(id => byID(id)?.classList.add('hidden'));
      byID('lb-title').textContent = '分享的相册';
      byID('lb-info').innerHTML = '';
      byID('lb-loading')?.classList.remove('show');
      clearAlbumGrid();
      const grid = document.createElement('div');
      grid.className = 'photo-grid';
      grid.id = 'share-grid';
      const frame = byID('lb-frame');
      frame.style.width = '100%';
      frame.style.padding = 'calc(84px + env(safe-area-inset-top, 0px)) calc(18px + env(safe-area-inset-right, 0px)) calc(18px + env(safe-area-inset-bottom, 0px)) calc(18px + env(safe-area-inset-left, 0px))';
      frame.style.boxSizing = 'border-box';
      frame.style.overflow = 'auto';
      grid.style.width = 'min(1180px, 100%)';
      currentItems.forEach((item, index) => {
        const button = document.createElement('button');
        button.className = 'photo-thumb';
        button.type = 'button';
        button.setAttribute('aria-label', item.original_name || 'shared-media');
        const url = mediaUrl(item);
        button.innerHTML = item.media_kind === 'video'
          ? `<video muted playsinline preload="metadata" src="${url}"></video><span class="media-duration-badge">${icons.video || ''}</span>`
          : `<img loading="lazy" src="${url}" alt="${escapeHTML(item.original_name || 'shared-media')}">`;
        button.onclick = () => showItem(index);
        grid.appendChild(button);
      });
      frame.appendChild(grid);
    }

    async function load() {
      setLoading('加载中…');
      try {
        const response = await fetch(`/api/s/${encodeURIComponent(token)}`);
        if (!response.ok) {
          setLoading('链接无效或已过期');
          return;
        }
        const link = await response.json();
        if (link.type === 'photo') {
          currentItems = [{
            id: link.target_id,
            uuid: link.target_uuid,
            original_name: link.target_original_name,
            media_kind: link.target_media_kind,
            mime_type: link.target_mime_type,
            size: link.target_size,
            width: link.target_width,
            height: link.target_height,
            duration_ms: link.target_duration_ms,
            taken_at: link.target_taken_at,
            download_url: `/s/${token}/download`,
          }];
          showItem(0);
          return;
        }
        if (link.type === 'album') {
          const pageResponse = await fetch(`/api/s/${encodeURIComponent(token)}/photos`);
          if (!pageResponse.ok) {
            setLoading('相册加载失败');
            return;
          }
          const page = await pageResponse.json();
          renderAlbum(page.photos || []);
          return;
        }
        setLoading('当前分享类型暂不支持');
      } catch (_) {
        setLoading('加载失败');
      }
    }

    function bind() {
      byID('share-body')?.addEventListener('click', event => {
        if (albumMode && !byID('share-grid') && event.target === event.currentTarget) renderAlbum(currentItems);
      });
      document.addEventListener('keydown', event => {
        if (event.key === 'Escape' && albumMode && !byID('share-grid')) {
          renderAlbum(currentItems);
          return;
        }
        if (!currentItems.length || byID('share-grid')) return;
        if (event.key === 'ArrowLeft') showItem(currentIndex - 1);
        if (event.key === 'ArrowRight') showItem(currentIndex + 1);
      });
      byID('lb-prev').onclick = () => showItem(currentIndex - 1);
      byID('lb-next').onclick = () => showItem(currentIndex + 1);
      byID('lb-zoom').oninput = event => {
        setShareZoom(Number(event.target.value) || 100);
      };
      document.querySelectorAll('[data-lightbox-zoom-step]').forEach(step => {
        const applyStep = () => {
          setShareZoom(shareZoom + (Number(step.dataset.lightboxZoomStep) || 0));
        };
        step.addEventListener('click', event => {
          event.preventDefault();
          applyStep();
        });
        step.addEventListener('keydown', event => {
          if (event.key !== 'Enter' && event.key !== ' ') return;
          event.preventDefault();
          applyStep();
        });
      });
      byID('lb-fit-height').onclick = () => {
        setShareZoom(100);
      };
      byID('lb-download')?.addEventListener('click', () => {
        window.alert('即将开始下载。');
      });
      const body = byID('share-body');
      body?.addEventListener('touchstart', event => {
        if (!event.touches || event.touches.length < 2 || byID('share-grid')) return;
        const item = currentItems[currentIndex] || {};
        const isVideo = item.media_kind === 'video' || item.target_media_kind === 'video';
        if (isVideo) return;
        const img = byID('lb-img');
        const distance = touchDistance(event.touches);
        if (!img || distance <= 0) return;
        const point = touchMidpoint(event.touches);
        pinchGesture = {
          startDistance: distance,
          startZoom: shareZoom,
          focusPoint: focusPointFromClient(point.x, point.y, img),
        };
        event.preventDefault();
      }, { passive: false });
      body?.addEventListener('touchmove', event => {
        if (!pinchGesture || !event.touches || event.touches.length < 2) return;
        const distance = touchDistance(event.touches);
        if (distance <= 0) return;
        const point = touchMidpoint(event.touches);
        const img = byID('lb-img');
        const focusPoint = focusPointFromClient(point.x, point.y, img) || pinchGesture.focusPoint;
        setShareZoom(Math.round(pinchGesture.startZoom * (distance / pinchGesture.startDistance)), { focusPoint });
        event.preventDefault();
      }, { passive: false });
      ['touchend', 'touchcancel'].forEach(type => {
        body?.addEventListener(type, event => {
          if (event.touches && event.touches.length >= 2) return;
          pinchGesture = null;
        }, { passive: false });
      });
      window.addEventListener('resize', () => {
        window.clearTimeout(window.__egShareFitTimer);
        window.__egShareFitTimer = window.setTimeout(() => applyFit(shareZoom), 100);
      });
    }

    async function init() {
      icons = await loadIcons(iconFiles);
      byID('lb-fit-height').innerHTML = icons.fit || '';
      byID('lb-prev').innerHTML = icons.prev || '';
      byID('lb-next').innerHTML = icons.next || '';
      bind();
      await load();
    }

    return { init, showItem, renderAlbum };
  }

  window.EchoGalleryLightbox = {
    renderShell,
    createShareController,
    loadIcons,
    renderInfoItems,
    normalizeIcon,
  };
}());
