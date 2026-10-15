'use strict';

let echoFrame = null;
let echoFrameFullscreenExitUntil = 0;

function echoFrameText(key, params) {
  return typeof copyText === 'function' ? copyText(`app.echoFrame.${key}`, params) : '';
}

function echoFrameLocale() {
  return typeof copyLocale === 'function'
    ? copyLocale()
    : (typeof navigator !== 'undefined' && navigator.language) || 'en';
}

function normalizeEchoFrameSettings(raw = {}) {
  raw = raw || {};
  const pick = (key, values, fallback) => values.includes(raw[key]) ? raw[key] : fallback;
  const interval = Number.isInteger(raw.interval) && raw.interval >= 3000 && raw.interval <= 600000 && raw.interval % 1000 === 0 ? raw.interval : 30000;
  return {
    source: pick('source', ['library', 'current', 'favorites', 'albums', 'random'], 'library'),
    album_ids: [...new Set((Array.isArray(raw.album_ids) ? raw.album_ids : []).map(Number).filter(id => Number.isSafeInteger(id) && id > 0))].slice(0, 32),
    album_library_id: String(raw.album_library_id || ''),
    order: pick('order', ['random', 'asc', 'desc'], 'random'),
    interval,
    interval_custom: raw.interval_custom === true || ![15000, 30000, 60000, 300000].includes(interval),
    end: pick('end', ['loop', 'reshuffle', 'stop'], 'reshuffle'),
    transition: pick('transition', ['fade', 'crossfade', 'zoom'], 'crossfade'),
    text_opacity: Number.isInteger(raw.text_opacity) && raw.text_opacity >= 10 && raw.text_opacity <= 100 ? raw.text_opacity : 60,
    videos: raw.videos !== false, muted: raw.muted !== false,
    clock: raw.clock !== false, date: raw.date !== false, wake_lock: raw.wake_lock !== false,
  };
}

function echoFrameIntervalLabel(interval) {
  const seconds = interval / 1000;
  if (seconds < 60) return echoFrameText('intervalSeconds', { count: seconds });
  const minutes = Math.floor(seconds / 60);
  return seconds % 60
    ? echoFrameText('intervalMixed', { minutes, seconds: seconds % 60 })
    : echoFrameText('intervalMinutes', { count: minutes });
}

function renderEchoFrameSettings() {
  const p = normalizeEchoFrameSettings(state.echoFrameSettings);
  const select = (key, label, options) => `<div class="settings-control"><label for="echo-frame-${key}">${label}</label><select class="input" id="echo-frame-${key}" data-echo-frame="${key}">${options.map(([value, text]) => `<option value="${value}"${String(key === 'interval' && p.interval_custom ? 'custom' : p[key]) === String(value) ? ' selected' : ''}>${text}</option>`).join('')}</select></div>`;
  return `<section class="card settings-panel settings-panel-echo-frame">
    ${renderSettingsPanelHeader(echoFrameText('title'), echoFrameText('description'), settingsPanelHeadIconMap.echoFrame)}
    <div class="settings-group">
      ${select('source', echoFrameText('source'), [['library', echoFrameText('sourceLibrary')], ['current', echoFrameText('sourceCurrentAlbum')], ['favorites', echoFrameText('sourceFavorites')], ['random', echoFrameText('sourceRandom')], ['albums', echoFrameText('sourceAlbums')]])}
      <div class="settings-control settings-control-wide" id="echo-frame-albums"${p.source !== 'albums' ? ' hidden' : ''}><label>${echoFrameText('albumsLabel')}</label><div class="echo-frame-album-options">${echoFrameText('albumsLoading')}</div></div>
      ${select('order', echoFrameText('order'), [['random', echoFrameText('orderRandom')], ['asc', echoFrameText('orderOldToNew')], ['desc', echoFrameText('orderNewToOld')]])}
      ${select('interval', echoFrameText('interval'), [[15000, echoFrameText('interval15Seconds')], [30000, echoFrameText('interval30Seconds')], [60000, echoFrameText('interval1Minute')], [300000, echoFrameText('interval5Minutes')], ['custom', echoFrameText('intervalCustom')]])}
      <div class="settings-control" id="echo-frame-custom-interval"${p.interval_custom ? '' : ' hidden'}>
        <label for="echo-frame-interval-slider"><span>${echoFrameText('customInterval')}</span><span id="echo-frame-interval-value">${echoFrameIntervalLabel(p.interval)}</span></label>
        <input class="input" id="echo-frame-interval-slider" type="range" min="3" max="600" step="1" value="${p.interval / 1000}">
      </div>
      <div class="settings-control settings-control-wide">
        <label for="echo-frame-text-opacity-slider"><span>${echoFrameText('textOpacity')}</span><span id="echo-frame-text-opacity-value">${p.text_opacity}%</span></label>
        <input class="input" id="echo-frame-text-opacity-slider" type="range" min="10" max="100" step="1" value="${p.text_opacity}">
      </div>
      ${select('end', echoFrameText('playbackEnd'), [['reshuffle', echoFrameText('endReshuffle')], ['loop', echoFrameText('endLoop')], ['stop', echoFrameText('endStop')]])}
      ${select('transition', echoFrameText('transition'), [['fade', echoFrameText('transitionFade')], ['crossfade', echoFrameText('transitionCrossfade')], ['zoom', echoFrameText('transitionZoom')]])}
      ${[['videos', echoFrameText('playVideos')], ['muted', echoFrameText('muteVideos')], ['clock', echoFrameText('showClock')], ['date', echoFrameText('showDate')], ['wake_lock', echoFrameText('wakeLock')]].map(([key, label]) => renderSettingsToggle(`echo-frame-${key}`, label, p[key])).join('')}
    </div>
  </section>`;
}

function bindEchoFrameSettings(host) {
  const panel = host.querySelector('.settings-panel-echo-frame');
  if (!panel) return;
  let albumsLoaded = false;
  const slider = panel.querySelector('#echo-frame-interval-slider');
  const opacitySlider = panel.querySelector('#echo-frame-text-opacity-slider');
  updateRangeProgress(slider);
  updateRangeProgress(opacitySlider);
  slider.addEventListener('input', () => {
    const p = normalizeEchoFrameSettings(state.echoFrameSettings);
    p.interval = Number(slider.value) * 1000;
    p.interval_custom = true;
    state.echoFrameSettings = normalizeEchoFrameSettings(p);
    panel.querySelector('#echo-frame-interval-value').textContent = echoFrameIntervalLabel(p.interval);
    updateRangeProgress(slider);
    setSettingsDirty();
  });
  opacitySlider.addEventListener('input', () => {
    const p = normalizeEchoFrameSettings(state.echoFrameSettings);
    p.text_opacity = Number(opacitySlider.value);
    state.echoFrameSettings = normalizeEchoFrameSettings(p);
    panel.querySelector('#echo-frame-text-opacity-value').textContent = `${p.text_opacity}%`;
    updateRangeProgress(opacitySlider);
    setSettingsDirty();
  });
  const loadAlbums = async () => {
    if (albumsLoaded) return;
    albumsLoaded = true;
    const container = panel.querySelector('.echo-frame-album-options');
    const library = state.serverSettings.active_library_id;
    try {
      const result = await api.get('/api/media/albums');
      if (!panel.isConnected || library !== state.serverSettings.active_library_id) return;
      const albums = Array.isArray(result) ? result : (result.albums || []);
      const p = normalizeEchoFrameSettings(state.echoFrameSettings);
      if (p.album_library_id !== library) p.album_ids = [];
      container.innerHTML = albums.map(a => `<label class="echo-frame-album-option"><input type="checkbox" value="${Number(a.id)}" data-echo-frame-album${p.album_ids.includes(Number(a.id)) ? ' checked' : ''}><span>${escapeHTML(a.name || echoFrameText('unnamedAlbum'))}</span></label>`).join('') || echoFrameText('noAlbums');
    } catch (_) {
      albumsLoaded = false;
      container.textContent = echoFrameText('albumsLoadFailed');
    }
  };
  panel.addEventListener('change', e => {
    const p = normalizeEchoFrameSettings(state.echoFrameSettings);
    const key = e.target.dataset.echoFrame || (e.target.id.startsWith('echo-frame-') ? e.target.id.slice(11) : '');
    if (e.target.matches('[data-echo-frame-album]')) {
      p.album_library_id = state.serverSettings.active_library_id || '';
      p.album_ids = [...panel.querySelectorAll('[data-echo-frame-album]:checked')].map(input => Number(input.value));
      if (p.album_ids.length > 32) { e.target.checked = false; showToast(echoFrameText('maxAlbums')); return; }
    } else if (key === 'interval') {
      p.interval_custom = e.target.value === 'custom';
      if (!p.interval_custom) p.interval = Number(e.target.value);
      slider.value = p.interval / 1000;
      panel.querySelector('#echo-frame-custom-interval').hidden = !p.interval_custom;
      panel.querySelector('#echo-frame-interval-value').textContent = echoFrameIntervalLabel(p.interval);
      updateRangeProgress(slider);
    } else if (key in p) {
      p[key] = e.target.type === 'checkbox' ? e.target.checked : key === 'interval' ? Number(e.target.value) : e.target.value;
    } else return;
    state.echoFrameSettings = normalizeEchoFrameSettings(p);
    panel.querySelector('#echo-frame-albums').hidden = p.source !== 'albums';
    if (p.source === 'albums') void loadAlbums();
    setSettingsDirty();
  });
  if (normalizeEchoFrameSettings(state.echoFrameSettings).source === 'albums') void loadAlbums();
}

function echoFrameAlive(frame) {
  return echoFrame === frame && !frame.abort.signal.aborted && frame.library === state.serverSettings.active_library_id;
}

function echoFrameMobileMode() {
  return (typeof isTouchLikeDevice === 'function' && isTouchLikeDevice())
    || (typeof isMobileLayout === 'function' && isMobileLayout());
}

function echoFrameResetSources(frame, reshuffle = false) {
  if (reshuffle) {
    frame.seed = Math.floor(Math.random() * 2147483646) + 1;
    frame.order = 'random';
    frame.paths = ['/api/media/random'];
  }
  frame.sources = frame.paths.map(path => ({ path, cursor: '', done: false, items: [] }));
  frame.seen.clear();
  frame.cycleCount = 0;
  frame.sourceStep = 0;
}

function echoFrameShuffle(items, seed) {
  // Stable within a cycle, so loop repeats and reshuffle changes the order.
  items.sort((a, b) => hashString(`${seed}:${a.uuid}`) - hashString(`${seed}:${b.uuid}`));
  return items;
}

async function echoFrameFill(frame, source) {
  if (source.items.length || source.done) return;
  const randomLibrary = source.path === '/api/media/random';
  const params = new URLSearchParams({ limit: '64', order: frame.order === 'asc' ? 'asc' : 'desc', sort: frame.order === 'asc' ? 'timeline_asc' : 'timeline_desc' });
  params.set('echo_frame', '1');
  if (!frame.p.videos) params.set('kind', 'image');
  if (randomLibrary) params.set('seed', String(frame.seed));
  if (randomLibrary && frame.albumIDs.length) params.set('album_ids', frame.albumIDs.join(','));
  if (frame.p.source === 'favorites') { params.set('favorite', '1'); params.set('echo_frame', '1'); }
  if (source.cursor) params.set('cursor', source.cursor);
  const page = await api.get(`${source.path}?${params}`, { signal: frame.abort.signal });
  if (!echoFrameAlive(frame)) return;
  source.items = (page.photos || []).filter(photo => frame.p.videos || !isVideoMedia(photo));
  if (frame.order === 'random' && !randomLibrary) echoFrameShuffle(source.items, frame.seed);
  source.done = !page.has_more || !page.next_cursor || page.next_cursor === source.cursor;
  source.cursor = page.next_cursor || '';
}

async function echoFrameNextPhoto(frame) {
  while (echoFrameAlive(frame)) {
    await Promise.all(frame.sources.map(source => echoFrameFill(frame, source)));
    if (!echoFrameAlive(frame)) return null;
    const available = frame.sources.filter(source => source.items.length);
    if (!available.length) {
      if (frame.sources.some(source => !source.done)) continue;
      if (!frame.cycleCount || frame.p.end === 'stop') return null;
      echoFrameResetSources(frame, frame.p.end === 'reshuffle');
      continue;
    }
    let source;
    if (frame.order === 'random') source = available[hashString(`${frame.seed}:${frame.sourceStep++}`) % available.length];
    else source = available.sort((a, b) => {
      const delta = Date.parse(a.items[0].taken_at) - Date.parse(b.items[0].taken_at);
      return (delta || a.items[0].id - b.items[0].id) * (frame.order === 'asc' ? 1 : -1);
    })[0];
    const photo = source.items.shift();
    if (frame.seen.has(photo.uuid)) continue;
    if (!frame.seen.size && photo.uuid === frame.current?.photo.uuid && source.items.length) {
      source.items.push(photo);
      continue;
    }
    frame.seen.add(photo.uuid);
    return photo;
  }
  return null;
}

function releaseEchoFrameMedia(media) {
  if (!media) return;
  if (media.tagName === 'VIDEO') { media.oncanplay = media.onerror = null; media.pause(); media.removeAttribute('src'); media.removeAttribute('poster'); media.load(); }
  else media.removeAttribute('src');
  media.remove();
}

function echoFrameImage(frame, url) {
  return new Promise((resolve, reject) => {
    const img = new Image();
    let finished = false;
    const cleanup = () => {
      clearTimeout(timeout);
      frame.abort.signal.removeEventListener('abort', cancel);
      img.onload = img.onerror = null;
    };
    const fail = error => {
      if (finished) return;
      finished = true; cleanup(); releaseEchoFrameMedia(img); reject(error);
    };
    const cancel = () => fail(new DOMException('Aborted', 'AbortError'));
    const timeout = setTimeout(() => fail(new Error(echoFrameText('mediaLoadTimeout'))), 45000);
    frame.abort.signal.addEventListener('abort', cancel, { once: true });
    img.onerror = () => fail(new Error(echoFrameText('mediaDecodeFailed')));
    img.onload = async () => {
      try {
        if (img.decode) await img.decode();
        if (!echoFrameAlive(frame)) return cancel();
        if (finished) return;
        finished = true; cleanup(); resolve(img);
      } catch (error) { fail(error); }
    };
    img.src = url;
    if (!echoFrameAlive(frame)) cancel();
  });
}

async function echoFramePrepare(frame, photo) {
  const media = await echoFrameImage(frame, isVideoMedia(photo) ? mediaThumbURL(photo) : mediaFileURL(photo));
  return { photo, media };
}

async function echoFramePrefetch(frame) {
  // Only one decoded next image is retained; failed items are skipped with a bounded retry budget.
  for (let tries = 0; tries < 20 && echoFrameAlive(frame); tries++) {
    const photo = await echoFrameNextPhoto(frame);
    if (!photo) return null;
    try { return await echoFramePrepare(frame, photo); }
    catch (error) { if (error.name === 'AbortError') throw error; }
  }
  throw new Error(echoFrameText('consecutiveLoadFailed'));
}

function echoFrameSchedule(frame) {
  clearTimeout(frame.timer);
  frame.root.classList?.toggle('is-paused', frame.paused);
  if (!echoFrameAlive(frame) || frame.paused || document.hidden || frame.ended || frame.current?.transitioning) return;
  frame.timer = setTimeout(() => void echoFrameAdvance(frame), frame.p.interval);
}

async function echoFrameAdvance(frame, direction = 1) {
  if (!echoFrameAlive(frame) || frame.busy) return;
  frame.busy = true;
  const pausedAtStart = frame.paused;
  clearTimeout(frame.timer);
  try {
    let prepared;
    const index = frame.historyIndex + direction;
    let fromHistory = false;
    if (index >= 0 && index < frame.history.length) {
      prepared = await echoFramePrepare(frame, frame.history[index]);
      fromHistory = true;
    } else if (direction < 0) return;
    else {
      prepared = frame.ready || await frame.next;
      frame.ready = null;
      frame.next = null;
      if (!echoFrameAlive(frame)) { releaseEchoFrameMedia(prepared?.media); return; }
      if (!prepared) {
        frame.ended = true;
        if (!frame.current) { showToast(echoFrameText('noPlayableMedia')); stopEchoFrame(); }
        return;
      }
    }
    if (!echoFrameAlive(frame)) { releaseEchoFrameMedia(prepared?.media); return; }
    if (document.hidden || (frame.paused && !pausedAtStart)) {
      if (fromHistory) releaseEchoFrameMedia(prepared.media);
      else frame.ready = prepared;
      return;
    }
    if (fromHistory) frame.historyIndex = index;
    else {
      frame.history.push(prepared.photo);
      if (frame.history.length > 12) frame.history.shift();
      frame.historyIndex = frame.history.length - 1;
      frame.cycleCount++;
    }
    frame.ended = false;
    echoFrameShow(frame, prepared);
    if (!frame.next) {
      frame.next = echoFramePrefetch(frame).then(item => {
        if (!echoFrameAlive(frame)) { releaseEchoFrameMedia(item?.media); return null; }
        return item;
      });
      // Observe early rejections while the current photo is still on screen.
      frame.next.catch(() => {});
    }
  } catch (error) {
    if (echoFrameAlive(frame)) { showToast(error.message || echoFrameText('readFailed')); stopEchoFrame(); }
  } finally { frame.busy = false; echoFrameSchedule(frame); }
}

function echoFrameShow(frame, prepared) {
  const { photo, media } = prepared;
  const old = frame.current;
  frame.current = prepared;
  prepared.transitioning = true;
  echoFrameClock(frame, true);
  frame.root.querySelector('.echo-frame-loading')?.remove();
  media.className = 'echo-frame-media';
  media.alt = photo.original_name || '';
  frame.surface.appendChild(media);
  // Commit the initial opacity so the browser creates a real CSS transition.
  void getComputedStyle(media).opacity;
  requestAnimationFrame(() => {
    if (!echoFrameAlive(frame) || frame.current !== prepared) return;
    media.classList.add('is-visible');
    if (old && frame.p.transition === 'fade') old.media.classList.remove('is-visible');
    echoFrameDateTransition(frame, media);
    const animation = media.getAnimations?.().find(a => a.transitionProperty === 'opacity');
    const finish = () => {
      if (!echoFrameAlive(frame) || frame.current !== prepared) return;
      prepared.transitioning = false;
      echoFrameSchedule(frame);
    };
    // Start the full display interval only after the incoming photo has settled.
    if (animation) animation.finished.then(finish, finish);
    else {
      const style = getComputedStyle(media);
      const milliseconds = value => parseFloat(value) * (value.trim().endsWith('ms') ? 1 : 1000);
      setTimeout(finish, milliseconds(style.transitionDuration) + milliseconds(style.transitionDelay));
    }
  });
  if (old) {
    // Keep the previous image opaque beneath a crossfade to avoid a dark dip.
    if (old.video) releaseEchoFrameMedia(old.video);
    setTimeout(() => releaseEchoFrameMedia(old.media), 6300);
  }
  if (isVideoMedia(photo)) {
    const video = document.createElement('video');
    prepared.video = video;
    video.className = 'echo-frame-media echo-frame-video';
    video.muted = frame.p.muted;
    video.playsInline = true;
    video.loop = true;
    video.preload = 'metadata';
    video.poster = media.src;
    video.oncanplay = () => {
      if (echoFrameAlive(frame) && frame.current === prepared && !document.hidden && !frame.paused) {
        video.play().then(() => video.classList.add('is-visible')).catch(() => {});
      }
    };
    video.onerror = () => { releaseEchoFrameMedia(video); prepared.video = null; };
    video.src = mediaPlaybackURL(photo);
    frame.surface.appendChild(video);
  }
}

async function echoFrameWake(frame) {
  if (!echoFrameAlive(frame) || !frame.p.wake_lock || document.hidden || frame.wake || frame.wakePending || !navigator.wakeLock) return;
  frame.wakePending = true;
  try {
    const lock = await navigator.wakeLock.request('screen');
    if (!echoFrameAlive(frame) || document.hidden) { await lock.release(); return; }
    frame.wake = lock;
    lock.addEventListener('release', () => { if (frame.wake === lock) frame.wake = null; });
  } catch (_) {}
  finally { frame.wakePending = false; }
}

function echoFrameClock(frame, deferDate = false) {
  const now = new Date();
  const clock = frame.root.querySelector('.echo-frame-time');
  clock.hidden = !frame.p.clock;
  const hour = String(now.getHours() % 12 || 12);
  const minute = String(now.getMinutes()).padStart(2, '0');
  clock.querySelector('.echo-frame-hour').textContent = hour;
  clock.querySelector('.echo-frame-minute').textContent = minute;
  clock.setAttribute('aria-label', now.toLocaleTimeString(echoFrameLocale(), { hour: 'numeric', minute: '2-digit', hour12: true }));
  clock.dateTime = now.toISOString();
  echoFrameUpdateGlass(frame);
  const date = frame.root.querySelector('.echo-frame-date');
  const takenAt = frame.current?.photo.taken_at;
  const taken = takenAt ? new Date(takenAt) : null;
  const validDate = taken && !Number.isNaN(taken.getTime()) && taken.getFullYear() > 1;
  const text = frame.p.date && validDate ? taken.toLocaleDateString(echoFrameLocale(), { weekday: 'long', month: 'long', day: 'numeric' }) : '';
  const item = date.querySelector('.echo-frame-date-item') || document.createElement('span');
  if (!item.parentNode) item.className = 'echo-frame-date-item';
  if (!item.parentNode) date.appendChild(item);
  if (deferDate) {
    cancelAnimationFrame(frame.dateAnimationFrame);
    frame.datePending = { text, takenAt, validDate, item, date };
    return;
  }
  if (frame.datePending) return;
  if (date.dataset.value !== text) {
    item.textContent = text;
    date.dataset.value = text;
    echoFrameGlassText(item);
  }
  if (frame.p.date && validDate) date.dateTime = takenAt;
  else date.removeAttribute('datetime');
}

function echoFrameGlassText(item) {
  if (!item || !item.textContent || !item.getBoundingClientRect) return;
  const style = getComputedStyle(item);
  const rect = item.getBoundingClientRect();
  if (!rect.width || !rect.height) return;
  const key = [item.textContent, rect.width, rect.height, style.fontSize, style.fontFamily, style.fontWeight, style.letterSpacing].join('|');
  if (item.dataset.glassKey === key) return;
  const canvas = document.createElement('canvas');
  const scale = Math.min(window.devicePixelRatio || 1, 3);
  const padding = 32;
  const horizontalPadding = 56;
  // Extra horizontal backing keeps the backdrop blur from clipping at glyph edges.
  canvas.width = Math.ceil((rect.width + horizontalPadding * 2) * scale);
  canvas.height = Math.ceil((rect.height + padding * 2) * scale);
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.scale(scale, scale);
  ctx.font = `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
  ctx.fillStyle = '#fff';
  const metrics = ctx.measureText(item.textContent);
  const ascent = metrics.fontBoundingBoxAscent ?? ctx.measureText('Mg').actualBoundingBoxAscent;
  const descent = metrics.fontBoundingBoxDescent ?? ctx.measureText('Mg').actualBoundingBoxDescent;
  const baseline = padding + (rect.height - ascent - descent) / 2 + ascent;
  const spacing = parseFloat(style.letterSpacing) || 0;
  if ('letterSpacing' in ctx) {
    ctx.letterSpacing = `${spacing}px`;
    ctx.fillText(item.textContent, horizontalPadding, baseline);
  } else {
    let x = horizontalPadding;
    for (const character of item.textContent) {
      ctx.fillText(character, x, baseline);
      x += ctx.measureText(character).width + spacing;
    }
  }
  item.style.setProperty('--echo-frame-glyph-mask', `url("${canvas.toDataURL()}")`);
  item.dataset.glassKey = key;
  item.classList.add('echo-frame-glass-text');
}

function echoFrameUpdateGlass(frame) {
  frame.root.querySelectorAll('.echo-frame-hour, .echo-frame-colon, .echo-frame-minute, .echo-frame-date-item').forEach(echoFrameGlassText);
}

function echoFrameDateTransition(frame, media) {
  const pending = frame.datePending;
  if (!pending) return;
  const style = getComputedStyle(media);
  const animation = media.getAnimations?.().find(a => a.transitionProperty === 'opacity');
  const timing = animation?.effect.getTiming();
  const milliseconds = value => parseFloat(value) * (value.trim().endsWith('ms') ? 1 : 1000);
  const duration = timing ? Number(timing.duration) : milliseconds(style.transitionDuration);
  const delay = timing ? Number(timing.delay) : milliseconds(style.transitionDelay);
  const total = Math.max(0, duration + delay);
  // Fade passes through black at the delay boundary; crossfade meets at half duration.
  const midpoint = delay > 0 ? delay : duration / 2;
  const fadeOut = Math.min(1000, midpoint);
  const fadeIn = Math.min(1000, total - midpoint);
  const started = performance.now();
  const { item, date } = pending;
  const initialOpacity = Number(getComputedStyle(item).opacity);
  item.style.transition = 'none';
  const smooth = value => (1 - Math.cos(Math.PI * Math.max(0, Math.min(1, value)))) / 2;
  let replaced = false;
  const tick = () => {
    if (!echoFrameAlive(frame) || frame.datePending !== pending) return;
    const elapsed = animation && typeof animation.currentTime === 'number'
      ? animation.currentTime : performance.now() - started;
    if (elapsed >= midpoint && !replaced) {
      item.style.opacity = '0';
      item.classList.remove('is-visible');
      void item.offsetWidth;
      item.textContent = pending.text;
      date.dataset.value = pending.text;
      echoFrameGlassText(item);
      if (frame.p.date && pending.validDate) date.dateTime = pending.takenAt;
      else date.removeAttribute('datetime');
      replaced = true;
    }
    const opacity = elapsed < midpoint
      ? initialOpacity * (1 - smooth(fadeOut ? (elapsed - midpoint + fadeOut) / fadeOut : 1))
      : smooth(fadeIn ? (elapsed - midpoint) / fadeIn : 1);
    item.style.opacity = String(opacity);
    if (elapsed >= midpoint + fadeIn) {
      item.classList.add('is-visible');
      item.style.opacity = '';
      item.style.transition = '';
      frame.datePending = null;
      return;
    }
    frame.dateAnimationFrame = requestAnimationFrame(tick);
  };
  tick();
}

function startEchoFrame() {
  if (echoFrame) return;
  const p = normalizeEchoFrameSettings(state.echoFrameSettings);
  const album = state.currentAlbum;
  if (p.source === 'current' && (!album || state.view !== 'album-detail')) { showToast(echoFrameText('openAlbumFirst')); return; }
  if (p.source === 'albums' && !p.album_ids.length) { showToast(echoFrameText('chooseAlbumsInSettings')); return; }
  if (p.source === 'albums' && p.album_library_id !== state.serverSettings.active_library_id) { showToast(echoFrameText('libraryChanged')); return; }
  stopSlideshow();
  $('#lb-video')?.pause();
  clearLightboxMediaSession();
  resetLightboxPrefetchCache();
  const root = el('section', `echo-frame echo-frame-${p.transition}${echoFrameMobileMode() ? ' echo-frame-mobile' : ''}`);
  root.style.setProperty('--echo-frame-text-opacity', String(p.text_opacity / 100));
  root.tabIndex = -1;
  root.setAttribute('role', 'dialog');
  root.setAttribute('aria-label', echoFrameText('exitHint'));
  root.setAttribute('aria-modal', 'true');
  root.innerHTML = `<div class="echo-frame-surface"></div><div class="echo-frame-loading" role="status">${echoFrameText('preparingOriginal')}</div><time class="echo-frame-time"><span class="echo-frame-hour" aria-hidden="true"></span><span class="echo-frame-colon" aria-hidden="true">:</span><span class="echo-frame-minute" aria-hidden="true"></span></time><time class="echo-frame-date" aria-live="polite"></time>`;
  const order = p.source === 'random' ? 'random' : p.order;
  const albumIDs = p.source === 'albums' ? p.album_ids : p.source === 'current' ? [Number(album.id)] : [];
  const paths = order === 'random' ? ['/api/media/random']
    : p.source === 'albums' ? p.album_ids.map(id => `/api/media/albums/${id}`)
    : p.source === 'current' ? [`/api/media/albums/${album.id}`]
    : p.source === 'favorites' ? ['/api/media/favorites']
    : [order === 'random' ? '/api/media/random' : '/api/media'];
  const frame = echoFrame = { root, surface: root.firstElementChild, p, order, paths, albumIDs, library: state.serverSettings.active_library_id,
    abort: new AbortController(), seed: Math.floor(Math.random() * 2147483646) + 1, seen: new Set(), history: [], historyIndex: -1,
    previousFocus: document.activeElement, paused: false, busy: false, ended: false, cycleCount: 0 };
  echoFrameResetSources(frame);
  (lightboxIsFullscreenTarget() ? $('#lightbox') : document.body).appendChild(root);
  if (typeof ResizeObserver !== 'undefined') {
    frame.glassObserver = new ResizeObserver(() => {
      if (echoFrameAlive(frame)) echoFrameUpdateGlass(frame);
    });
    frame.glassObserver.observe(root.querySelector('.echo-frame-time'));
    frame.glassObserver.observe(root.querySelector('.echo-frame-date'));
  }
  document.documentElement.classList.add('echo-frame-open');
  root.focus({ preventScroll: true });
  // Request before awaiting network IO, preserving the confirm button's user activation.
  const request = root.requestFullscreen || root.webkitRequestFullscreen;
  if (request) {
    try { Promise.resolve(request.call(root)).then(() => { if (!echoFrameAlive(frame) && document.fullscreenElement === root) void document.exitFullscreen(); }).catch(() => {}); } catch (_) {}
  }
  echoFrameClock(frame);
  frame.clockTimer = setInterval(() => echoFrameClock(frame), 1000);
  void echoFrameWake(frame);
  frame.next = echoFramePrefetch(frame);
  void echoFrameAdvance(frame);
  let lastTouchFavorite = 0;
  root.addEventListener('dblclick', e => {
    if (!frame.infoOpen && Date.now() - lastTouchFavorite > 700) { e.preventDefault(); void echoFrameFavorite(frame); }
  });
  addUniversalLongPress(root, async () => {
    const photo = frame.current?.photo;
    if (!photo || !echoFrameAlive(frame) || frame.infoOpen) return;
    frame.infoOpen = true;
    const paused = frame.paused;
    frame.paused = true;
    frame.root.classList.add('is-paused');
    clearTimeout(frame.timer);
    frame.current?.video?.pause();
    try {
      const albums = await api.get(`/api/media/${photo.id}/albums`, { signal: frame.abort.signal });
      if (!echoFrameAlive(frame)) return;
      const items = Array.isArray(albums) ? albums : albums.albums || [];
      // A modal must be inside the fullscreen element to remain visible.
      const promise = appAlert(`${photo.original_name}\n${new Date(photo.taken_at).toLocaleString(echoFrameLocale())}\n${items.map(a => a.name).join(' · ') || echoFrameText('libraryRoot')}`, { title: echoFrameText('mediaInfo') });
      const modal = [...document.querySelectorAll('.blocking-dialog-overlay.open')].pop();
      if (modal) { root.appendChild(modal); frame.infoModal = modal; }
      await promise;
    } catch (_) {}
    finally { frame.infoOpen = false; frame.paused = paused; echoFrameSchedule(frame); if (!paused && !document.hidden) frame.current?.video?.play().catch(() => {}); }
  });
  let down = null;
  let lastTap = null;
  root.addEventListener('pointerdown', e => {
    if (frame.infoOpen) return;
    down = { x: e.clientX, y: e.clientY, time: Date.now(), id: e.pointerId };
  });
  root.addEventListener('pointerup', e => {
    if (!down || down.id !== e.pointerId || frame.infoOpen) { down = null; return; }
    const now = Date.now();
    if (e.clientY - down.y > 100 && Math.abs(e.clientX - down.x) < 80) void exitEchoFrameToLightbox();
    else if (e.pointerType === 'touch' && now - down.time < 400 && Math.hypot(e.clientX - down.x, e.clientY - down.y) < 15) {
      if (lastTap && now - lastTap.time < 350 && Math.hypot(e.clientX - lastTap.x, e.clientY - lastTap.y) < 30) {
        lastTouchFavorite = now;
        lastTap = null;
        void echoFrameFavorite(frame);
      } else lastTap = { x: e.clientX, y: e.clientY, time: now };
    }
    down = null;
  });
  root.addEventListener('pointercancel', () => { down = null; lastTap = null; });
}

function stopEchoFrame() {
  const frame = echoFrame;
  if (!frame) return;
  echoFrame = null;
  frame.abort.abort();
  clearTimeout(frame.timer);
  clearInterval(frame.clockTimer);
  cancelAnimationFrame(frame.dateAnimationFrame);
  frame.glassObserver?.disconnect();
  frame.wake?.release().catch(() => {});
  frame.next?.then(item => releaseEchoFrameMedia(item?.media)).catch(() => {});
  releaseEchoFrameMedia(frame.ready?.media);
  frame.root.querySelectorAll('img, video').forEach(releaseEchoFrameMedia);
  frame.infoModal?.querySelector('[data-dialog-confirm]')?.click();
  echoFrameFullscreenExitUntil = Date.now() + 1500;
  if (document.fullscreenElement === frame.root || document.webkitFullscreenElement === frame.root) void exitLightboxFullscreen();
  frame.root.remove();
  document.documentElement.classList.remove('echo-frame-open');
  frame.previousFocus?.focus({ preventScroll: true });
}

async function exitEchoFrameToLightbox() {
  const frame = echoFrame;
  if (!frame) return;
  const photo = frame.current?.photo;
  const library = frame.library;
  stopSlideshow();
  stopEchoFrame();
  if (!photo || library !== state.serverSettings.active_library_id) return;
  const photos = [...state.lightboxPhotos];
  let index = photos.findIndex(item => Number(item.id) === Number(photo.id));
  if (index < 0) {
    index = photos.length;
    photos.push(photo);
  } else photos[index] = photo;
  if ($('#lightbox')?.classList.contains('open')) {
    state.lightboxPhotos = photos;
    await lbGoTo(index);
  } else await openLightbox(photos, index);
  if (echoFrame || library !== state.serverSettings.active_library_id) return;
  refreshLightboxUiActivity();
  focusLightboxKeyboardSurface();
}

async function echoFrameFavorite(frame) {
  const photo = frame.current?.photo;
  if (!photo || !echoFrameAlive(frame) || frame.favoriting || !canWriteMedia()) return;
  frame.favoriting = true;
  try {
    const favorite = !photo.is_favorite;
    await setPhotoFavorite(photo.id, favorite, false);
    if (!echoFrameAlive(frame)) return;
    applyFavoriteStateToPhoto(photo, favorite, false);
    const known = findKnownPhotoByID(photo.id);
    if (known) applyFavoriteStateToPhoto(known, favorite, false);
    syncFavoritePhotoList(photo.id, favorite, false, known || photo);
    updateFavoriteButtonsInDOM(photo.id, favorite, false);
    showToast(favorite ? echoFrameText('addedFavorite') : echoFrameText('removedFavorite'));
  } catch (_) { if (echoFrameAlive(frame)) showToast(echoFrameText('favoriteUpdateFailed')); }
  finally { frame.favoriting = false; }
}

function handleEchoFrameKeydown(e) {
  const frame = echoFrame;
  if (!frame) return false;
  if (frame.infoOpen) {
    if (e.key === 'Escape') { e.preventDefault(); e.stopImmediatePropagation(); frame.infoModal?.querySelector('[data-dialog-confirm]')?.click(); }
    return true;
  }
  e.preventDefault();
  e.stopImmediatePropagation();
  if (e.repeat) return true;
  const binding = state.playerKeymap[keyEventToken(e)];
  if (e.key === 'Escape') void exitEchoFrameToLightbox();
  else if (e.key.toLowerCase() === 'f') void echoFrameFavorite(frame);
  else if (binding?.command === 'seek') void echoFrameAdvance(frame, Number(binding.args[0]) < 0 ? -1 : 1);
  else if ((binding?.command === 'cycle' && binding.args[0] === 'pause') || e.key === ' ') {
    frame.paused = !frame.paused;
    frame.root.classList.toggle('is-paused', frame.paused);
    if (frame.paused) { clearTimeout(frame.timer); frame.current?.video?.pause(); }
    else { frame.current?.video?.play().catch(() => {}); echoFrameSchedule(frame); }
  }
  return true;
}

document.addEventListener('visibilitychange', () => {
  const frame = echoFrame;
  if (!frame) return;
  if (document.hidden) {
    clearTimeout(frame.timer);
    frame.current?.video?.pause();
    const lock = frame.wake;
    frame.wake = null;
    lock?.release().catch(() => {});
  }
  else {
    void echoFrameWake(frame);
    if (!frame.paused) {
      frame.current?.video?.play().catch(() => {});
      if (frame.ready || !frame.current) void echoFrameAdvance(frame);
      else echoFrameSchedule(frame);
    }
  }
});
document.addEventListener('fullscreenchange', () => { if (echoFrame && !document.fullscreenElement) void exitEchoFrameToLightbox(); });
window.addEventListener('pagehide', stopEchoFrame);

// Delegation survives app renders; a held press suppresses the ordinary slideshow click.
let echoFramePress = null;
let echoFrameSuppressClickUntil = 0;
document.addEventListener('pointerdown', e => {
  if (!e.target.closest('#lb-slideshow-toggle') || e.button > 0) return;
  clearTimeout(echoFramePress?.timer);
  const press = echoFramePress = { x: e.clientX, y: e.clientY, id: e.pointerId };
  press.timer = setTimeout(async () => {
    echoFramePress = null;
    echoFrameSuppressClickUntil = Infinity;
    const promise = appConfirm(echoFrameText('confirmEnter'), { title: 'EchoFrame', confirmText: echoFrameText('enter') });
    const modal = [...document.querySelectorAll('.blocking-dialog-overlay.open')].pop();
    if (modal && lightboxIsFullscreenTarget()) $('#lightbox').appendChild(modal);
    if (await promise && $('#lightbox')?.classList.contains('open')) startEchoFrame();
  }, 600);
});
for (const event of ['pointerup', 'pointercancel', 'pointermove']) document.addEventListener(event, e => {
  if (event !== 'pointermove' && echoFrameSuppressClickUntil === Infinity) echoFrameSuppressClickUntil = Date.now() + 700;
  if (!echoFramePress || e.pointerId !== echoFramePress.id) return;
  if (event === 'pointermove' && Math.hypot(e.clientX - echoFramePress.x, e.clientY - echoFramePress.y) <= 10) return;
  clearTimeout(echoFramePress.timer); echoFramePress = null;
});
document.addEventListener('click', e => {
  if (e.target.closest('#lb-slideshow-toggle') && Date.now() < echoFrameSuppressClickUntil) { e.preventDefault(); e.stopImmediatePropagation(); }
}, true);
document.addEventListener('contextmenu', e => {
  if (e.target.closest('#lb-slideshow-toggle, .echo-frame')) e.preventDefault();
}, true);
