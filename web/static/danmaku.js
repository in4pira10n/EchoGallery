(function () {
  let cleanup = () => {};
  let enabled = true;
  let refresh = () => {};
  let preferences = { font_step: 1, font_weight_step: 2, opacity_step: 2, speed_step: 2, block_list: '' };
  let blocked = [];

  function configure(value = {}) {
    preferences = { ...preferences, ...value };
    blocked = String(preferences.block_list || '').split('\n').map(s => s.trim()).filter(Boolean).slice(0, 100).map(rule => {
      if (rule.startsWith('/') && rule.lastIndexOf('/') > 0) {
        const end = rule.lastIndexOf('/');
        try { const re = new RegExp(rule.slice(1, end), rule.slice(end + 1).replace(/[gy]/g, '')); return text => re.test(text); } catch (_) { return () => false; }
      }
      return text => text.toLocaleLowerCase().includes(rule.toLocaleLowerCase());
    });
    refresh();
  }

  function toggle() {
    enabled = !enabled;
    refresh();
    return enabled;
  }

  function stop() {
    cleanup();
    cleanup = () => {};
    refresh = () => {};
  }

  async function start(video, mediaID, request) {
    stop();
    const host = video.closest('.lightbox');
    if (!host) return;
    const controller = new AbortController();
    const canvas = document.createElement('canvas');
    canvas.className = 'lightbox-danmaku';
    canvas.setAttribute('aria-hidden', 'true');
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    host.append(canvas);
    let frame = 0;
    let entries = [];
    let width = 0;
    let height = 0;
    let pixelRatio = 0;
    let textSize = 18.75;
    let lastTime = -1;
    let cursor = 0;
    let active = [];
    let lifetime = 8;
    const fixedLifetime = 4;
    const font = size => `${[400, 500, 600, 700, 800][preferences.font_weight_step] || 600} ${size}px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;

    function resize() {
      const w = host.clientWidth;
      const h = host.clientHeight;
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      if (w === width && h === height && dpr === pixelRatio) return;
      width = w;
      height = h;
      pixelRatio = dpr;
      textSize = (parseFloat(getComputedStyle(document.body).fontSize) || 15) * 1.25;
      canvas.width = Math.ceil(width * dpr);
      canvas.height = Math.ceil(height * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      lastTime = -1;
    }

    function position(item, time) {
      const progress = (time - item.time) / lifetime;
      if (item.mode === 6) return -item.width + (width + item.width) * progress;
      if (item.mode === 1) return width - (width + item.width) * progress;
      return (width - item.width) / 2;
    }

    function draw() {
      frame = 0;
      if (controller.signal.aborted) return;
      resize();
      const time = video.currentTime || 0;
      lifetime = [12, 10, 8, 6, 4][preferences.speed_step] || 8;
      ctx.globalAlpha = [0, .25, .5, .75, 1][preferences.opacity_step] ?? .5;
      ctx.clearRect(0, 0, width, height);
      if (!enabled) {
        lastTime = -1;
        return;
      }
      if (time < lastTime || lastTime < 0 || time - lastTime > 1) {
        active = [];
        let lo = 0;
        let hi = entries.length;
        while (lo < hi) {
          const mid = (lo + hi) >>> 1;
          if (entries[mid].time < time - lifetime) lo = mid + 1;
          else hi = mid;
        }
        cursor = lo;
      }
      // Match the app's body text scale regardless of video or lightbox dimensions.
      const fontSize = textSize / 1.25 * ([1, 1.25, 1.5, 1.75, 2, 2.5][preferences.font_step] || 1.25);
      const sizeScale = fontSize / 25;
      const lineHeight = Math.ceil(38 * sizeScale);
      const lanes = Math.max(1, Math.floor(height / lineHeight));
      while (cursor < entries.length && entries[cursor].time <= time) {
        const entry = entries[cursor++];
        if (blocked.some(matches => matches(entry.text))) continue;
        active = active.filter(item => entry.time - item.time < item.life);
        const item = { ...entry, size: fontSize, life: [4, 5].includes(entry.mode) ? fixedLifetime : lifetime };
        if (time - item.time >= item.life) continue;
        ctx.font = font(item.size);
        item.width = ctx.measureText(item.text).width;
        let lane = -1;
        for (let n = 0; n < lanes; n++) {
          const candidate = item.mode === 4 ? lanes - 1 - n : n;
          const free = active.every(other => {
            if (other.lane !== candidate) return true;
            if (other.mode !== item.mode || ![1, 6].includes(item.mode)) return false;
            // A following comment must not catch the previous one before it exits.
            const elapsed = item.time - other.time;
            const speed = (width + item.width) / lifetime;
            const otherSpeed = (width + other.width) / lifetime;
            const gap = otherSpeed * elapsed - other.width;
            return gap > 12 && gap - Math.max(0, speed - otherSpeed) * (other.life - elapsed) > 12;
          });
          if (free) { lane = candidate; break; }
        }
        if (lane >= 0) active.push({ ...item, lane });
      }
      active = active.filter(item => time - item.time < item.life);
      ctx.textBaseline = 'top';
      ctx.lineJoin = 'round';
      ctx.lineWidth = 3 * sizeScale;
      ctx.strokeStyle = 'rgba(0,0,0,.8)';
      for (const item of active) {
        ctx.font = font(item.size);
        ctx.fillStyle = item.color;
        const x = position(item, time);
        const y = item.lane * lineHeight + 4;
        ctx.strokeText(item.text, x, y);
        ctx.fillText(item.text, x, y);
      }
      lastTime = time;
      if (!video.paused && !video.ended && !document.hidden) frame = requestAnimationFrame(draw);
    }

    function schedule() {
      if (!frame) frame = requestAnimationFrame(draw);
    }
    refresh = () => { lastTime = -1; schedule(); };
    const events = ['play', 'pause', 'seeked', 'loadedmetadata', 'timeupdate', 'ended'];
    events.forEach(event => video.addEventListener(event, schedule));
    document.addEventListener('visibilitychange', schedule);
    window.addEventListener('resize', schedule);
    window.addEventListener('orientationchange', schedule);
    const viewport = window.visualViewport;
    viewport?.addEventListener('resize', schedule);
    const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(schedule) : null;
    observer?.observe(host);
    observer?.observe(video);
    cleanup = () => {
      controller.abort();
      cancelAnimationFrame(frame);
      observer?.disconnect();
      events.forEach(event => video.removeEventListener(event, schedule));
      document.removeEventListener('visibilitychange', schedule);
      window.removeEventListener('resize', schedule);
      window.removeEventListener('orientationchange', schedule);
      viewport?.removeEventListener('resize', schedule);
      canvas.remove();
    };
    try {
      const response = await request(`/api/media/${encodeURIComponent(mediaID)}/danmaku`, { signal: controller.signal });
      if (!response.ok || response.status === 204) return;
      const xml = new DOMParser().parseFromString(await response.text(), 'application/xml');
      if (controller.signal.aborted || xml.querySelector('parsererror')) return;
      entries = Array.from(xml.querySelectorAll('d')).slice(0, 100000).flatMap(node => {
        const values = (node.getAttribute('p') || '').split(',').map(Number);
        const [time, mode, size, color] = values;
        if (!Number.isFinite(time) || time < 0 || ![1, 4, 5, 6].includes(mode) || !Number.isFinite(color)) return [];
        return [{ time, mode, size: Math.max(16, size || 25), color: `#${(color & 0xffffff).toString(16).padStart(6, '0')}`, text: (node.textContent || '').slice(0, 300) }];
      }).sort((a, b) => a.time - b.time);
      lastTime = -1;
      schedule();
    } catch (error) {
      if (error.name !== 'AbortError') console.warn('Danmaku load failed', error);
    }
  }

  window.EchoGalleryDanmaku = { start, stop, toggle, configure };
}());
