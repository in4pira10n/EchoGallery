/* Shared image download/decode scheduler. Video streaming stays native. */
(function (root) {
  'use strict';
  const abortError = () => new DOMException('Media request cancelled', 'AbortError');
  function create(options = {}) {
    const limit = options.limit || 4;
    const maxBytes = options.maxBytes ?? 32 * 1024 * 1024;
    const maxEntries = options.maxEntries ?? 72;
    const request = options.fetch || root.fetch.bind(root);
    const createURL = options.createURL || (blob => URL.createObjectURL(blob));
    const revokeURL = options.revokeURL || (url => URL.revokeObjectURL(url));
    const now = options.now || (() => performance.now());
    const tasks = new Map();
    let active = 0, paused = false, disposed = false, clock = 0;
    const metrics = { requests: 0, bodyBytes: 0, completed: 0, cancelled: 0, failed: 0, shared: 0, peakActive: 0, decodeMS: 0 };
    function drop(task) {
      if (tasks.get(task.key) === task) tasks.delete(task.key);
      if (task.url) revokeURL(task.url);
      task.url = '';
    }
    function trim() {
      let bytes = 0, count = 0;
      const idle = [];
      for (const task of tasks.values()) {
        if (task.status !== 'complete') continue;
        bytes += task.bytes; count++;
        if (!task.owners.size) idle.push(task);
      }
      idle.sort((a, b) => a.used - b.used);
      for (const task of idle) {
        if (bytes <= maxBytes && count <= maxEntries) break;
        bytes -= task.bytes; count--; drop(task);
      }
    }
    function priority(task) { return Math.min(...task.owners.values()); }
    function pump() {
      if (paused || disposed) return;
      const queued = [...tasks.values()].filter(t => t.status === 'queued' && t.owners.size)
        .sort((a, b) => priority(a) - priority(b) || a.order - b.order);
      for (const task of queued) {
        const rank = priority(task);
        // Keep a slot free for the current lightbox image; speculative work
        // may occupy at most one slot and never competes with foreground work.
        const backgroundActive = [...tasks.values()].filter(t => t.status === 'loading' && priority(t) >= 3).length;
        if (active >= limit || (rank > 0 && active >= limit - 1) || (rank >= 3 && backgroundActive >= 1)) continue;
        run(task);
      }
    }
    async function run(task) {
      active++;
      task.status = 'loading';
      metrics.requests++;
      metrics.peakActive = Math.max(metrics.peakActive, active);
      const signal = task.controller.signal;
      let objectURL = '';
      try {
        const response = await request(task.source, { signal });
        if (!response.ok) throw new Error(`Media HTTP ${response.status}`);
        const blob = await response.blob();
        metrics.bodyBytes += blob.size;
        if (signal.aborted) throw abortError();
        objectURL = createURL(blob);
        const start = now();
        if (options.decode) await options.decode(objectURL, signal);
        else await decode(objectURL, signal);
        metrics.decodeMS += now() - start;
        if (signal.aborted) throw abortError();
        task.url = objectURL; objectURL = '';
        task.bytes = blob.size;
        task.status = 'complete'; task.used = ++clock;
        metrics.completed++;
        task.resolve({ url: task.url, bytes: task.bytes });
      } catch (error) {
        if (objectURL) revokeURL(objectURL);
        task.status = signal.aborted ? 'cancelled' : 'failed';
        if (!signal.aborted) metrics.failed++;
        if (tasks.get(task.key) === task) tasks.delete(task.key);
        task.reject(signal.aborted ? abortError() : error);
      } finally {
        active--; trim(); pump();
      }
    }
    function acquire(key, source, rank = 3) {
      if (disposed) throw new Error('Media loader disposed');
      let task = tasks.get(key);
      if (task?.controller.signal.aborted) { drop(task); task = null; }
      if (!task) {
        let resolve, reject;
        const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
        // Consumers still receive rejection, even if released before attaching a handler.
        promise.catch(() => {});
        task = { key, source, owners: new Map(), controller: new AbortController(), status: 'queued',
          promise, resolve, reject, order: ++clock, used: clock, url: '', bytes: 0 };
        tasks.set(key, task);
      } else metrics.shared++;
      const owner = Symbol();
      task.owners.set(owner, rank); task.used = ++clock;
      let released = false;
      const handle = {
        promise: task.promise,
        promote(nextRank) { if (!released) { task.owners.set(owner, nextRank); pump(); } },
        release() {
          if (released) return;
          released = true; task.owners.delete(owner);
          if (!task.owners.size && (task.status === 'queued' || task.status === 'loading') && !task.controller.signal.aborted) {
            metrics.cancelled++;
            task.controller.abort();
            if (task.status === 'queued') { task.status = 'cancelled'; task.reject(abortError()); drop(task); }
          }
          trim(); pump();
        },
      };
      pump();
      return handle;
    }
    function clear() {
      for (const task of [...tasks.values()]) {
        if (task.status !== 'complete' && !task.controller.signal.aborted) metrics.cancelled++;
        task.controller.abort(); task.reject(abortError()); drop(task);
      }
    }
    return {
      acquire,
      setPaused(value) { paused = !!value; if (!paused) pump(); },
      clear,
      dispose() { disposed = true; clear(); },
      stats() {
        const items = [...tasks.values()];
        return { ...metrics, active, queued: items.filter(t => t.status === 'queued').length,
          retainedBytes: items.reduce((n, t) => n + t.bytes, 0), entries: items.length,
          pinned: items.filter(t => t.owners.size).length, limit, maxBytes, maxEntries };
      },
    };
  }
  function decode(url, signal) {
    return new Promise((resolve, reject) => {
      const image = new Image();
      let finished = false;
      image.decoding = 'async';
      function finish(error) {
        if (finished) return;
        finished = true;
        image.onload = image.onerror = null;
        signal.removeEventListener('abort', abort);
        image.removeAttribute('src');
        if (error) reject(error); else resolve();
      }
      const abort = () => finish(abortError());
      image.onload = () => {
        if (typeof image.decode === 'function') image.decode().then(() => finish(), error => finish(error));
        else finish();
      };
      image.onerror = () => finish(new Error('Image decode failed'));
      signal.addEventListener('abort', abort, { once: true });
      if (signal.aborted) abort(); else image.src = url;
    });
  }
  const api = { create };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.EchoGalleryMediaLoader = api;
})(typeof window === 'undefined' ? globalThis : window);
