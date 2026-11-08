/* Bounded square-card grids. Media order and pagination stay with the caller. */
(function (root) {
  'use strict';
  function layout({ count, width, columns, gap = 0, top = 0, viewport = 0, focus = null }) {
    columns = Math.max(1, columns || 1);
    const cell = Math.max(1, (width - gap * (columns - 1)) / columns);
    const pitch = cell + gap;
    const rows = Math.ceil(count / columns);
    const height = Math.max(0, rows * pitch - (rows ? gap : 0));
    const buffer = Math.min(8, Math.max(2, Math.ceil(viewport / pitch)));
    let first = Math.max(0, Math.floor(-top / pitch) - buffer);
    let last = Math.min(rows, Math.ceil((viewport - top) / pitch) + buffer);
    if (focus != null) {
      first = Math.max(0, Math.floor(focus / columns) - buffer);
      last = Math.min(rows, first + Math.ceil(viewport / pitch) + buffer * 2 + 1);
    } else if (top > viewport + buffer * pitch || top + height < -buffer * pitch) {
      first = last = 0;
    }
    first = Math.min(rows, first);
    last = Math.max(first, last);
    return { columns, cell, pitch, rows, height, start: first * columns,
      end: Math.min(count, last * columns), budget: columns * (Math.ceil(viewport / pitch) + buffer * 2 + 2) };
  }
  function create(options) {
    const win = options.window || root;
    const doc = win.document;
    const grids = new Map();
    let frame = 0, idleTimer = 0, touching = false, lastScroll = -Infinity, pendingAnchor = null;
    let mountedPeak = 0, created = 0, released = 0, previousScrollY = win.scrollY || 0;
    let userScrollActive = false, userScrollTimer = 0;
    const keyOf = item => String(item.uuid || item.id);
    const clock = () => win.performance.now();
    function drop(controller, key, node) {
      options.release?.(node);
      node.remove(); controller.nodes.delete(key); released++;
    }
    function dispose(controller) {
      for (const [key, node] of controller.nodes) drop(controller, key, node);
      resize?.unobserve(controller.grid);
      grids.delete(controller.grid);
    }
    function geometry(controller, focus = null) {
      const rect = controller.grid.getBoundingClientRect();
      if (controller.relayout || Math.abs(rect.width - (controller.geometry?.width || 0)) > .5) {
        // Old explicit positions can create implicit columns after a column
        // reduction. Clear them before reading the new resolved CSS tracks.
        for (const node of controller.nodes.values()) {
          node.style.gridColumnStart = 'auto';
          node.style.gridRowStart = 'auto';
        }
        controller.relayout = false;
      }
      const style = win.getComputedStyle(controller.grid);
      const columns = style.gridTemplateColumns.trim().split(/\s+/).filter(value => /^\d/.test(value)).length || 1;
      const gap = parseFloat(style.rowGap) || 0;
      const sizeKey = `${rect.width}:${columns}:${gap}`;
      if (controller.sizeKey !== sizeKey) {
        // Use the browser's actual subpixel layout precision. A fraction of a
        // pixel per row otherwise accumulates into visible drift in deep grids.
        const probe = doc.createElement('div');
        probe.style.cssText = `position:absolute;visibility:hidden;pointer-events:none;height:${Math.max(1, (rect.width - gap * (columns - 1)) / columns)}px;width:0`;
        controller.grid.appendChild(probe);
        controller.cell = probe.getBoundingClientRect().height;
        probe.remove(); controller.sizeKey = sizeKey;
      }
      return { ...layout({ count: controller.items.length + controller.placeholders,
        width: controller.cell * columns + gap * (columns - 1), columns, gap,
        top: rect.top, viewport: win.innerHeight, focus }), top: rect.top, width: rect.width };
    }
    function rememberAnchor(controller, measured) {
      if (!controller.items.length || measured.top >= win.innerHeight || measured.top + measured.height <= 0) {
        controller.anchor = null; return;
      }
      const cell = Math.max(controller.placeholders,
        Math.max(0, Math.floor(-measured.top / measured.pitch)) * measured.columns);
      const index = Math.min(controller.items.length - 1, Math.max(0, cell - controller.placeholders));
      controller.anchor = { controller, key: keyOf(controller.items[index]),
        offset: measured.top + Math.floor((index + controller.placeholders) / measured.columns) * measured.pitch };
    }
    function atScrollBoundary() {
      const top = win.scrollY || 0;
      const max = Math.max(0, Math.max(doc.documentElement.scrollHeight, doc.body.scrollHeight) - win.innerHeight);
      return top <= 1 || max - top <= 1;
    }
    function restoreAnchor() {
      if (!pendingAnchor) return;
      if (atScrollBoundary()) {
        pendingAnchor = null;
        win.clearTimeout(idleTimer);
        return;
      }
      if (touching || clock() - lastScroll < 160) {
        win.clearTimeout(idleTimer); idleTimer = win.setTimeout(restoreAnchor, 180); return;
      }
      const anchor = pendingAnchor; pendingAnchor = null;
      const c = anchor.controller;
      if (!c.grid.isConnected || !grids.has(c.grid)) return;
      const index = c.indices.get(anchor.key);
      if (index == null) return;
      const g = geometry(c);
      const delta = g.top + Math.floor((index + c.placeholders) / g.columns) * g.pitch - anchor.offset;
      if (Math.abs(delta) > .5) win.scrollBy({ top: delta, behavior: 'auto' });
      schedule();
    }
    function render(c, focus = null) {
      if (!c.grid.isConnected) return;
      const g = geometry(c, focus);
      if (!g.width) return;
      c.geometry = g;
      c.grid.style.height = `${g.height}px`;
      c.grid.style.gridAutoRows = `${g.cell}px`;
      c.grid.style.alignContent = 'start';
      c.grid.style.overflowAnchor = 'none';
      const wanted = new Set();
      let cursor = c.grid.firstElementChild;
      for (let i = g.start; i < g.end; i++) {
        const placeholder = i < c.placeholders;
        const item = placeholder ? null : c.items[i - c.placeholders];
        const key = placeholder ? `placeholder:${i}` : keyOf(item);
        wanted.add(key);
        let node = c.nodes.get(key);
        if (!node) {
          node = placeholder ? options.placeholder(c.kind, i) : options.card(item, c.liveList, c.cardOptions);
          c.nodes.set(key, node); created++;
        }
        node.style.gridRowStart = String(Math.floor(i / g.columns) + 1);
        node.style.gridColumnStart = String(i % g.columns + 1);
        node.style.height = '100%';
        if (node === cursor) cursor = cursor.nextElementSibling;
        else c.grid.insertBefore(node, cursor);
      }
      for (const [key, node] of c.nodes) {
        if (!wanted.has(key)) {
          // Preserve a native focused control until focus moves away.
          if (c.indices.has(key) && node.contains(doc.activeElement)) {
            const i = c.indices.get(key) + c.placeholders;
            node.style.gridRowStart = String(Math.floor(i / g.columns) + 1);
            node.style.gridColumnStart = String(i % g.columns + 1);
            continue;
          }
          drop(c, key, node);
        }
      }
      mountedPeak = Math.max(mountedPeak, [...grids.values()].reduce((n, value) => n + value.nodes.size, 0));
      rememberAnchor(c, g);
    }
    function refresh() {
      frame = 0;
      for (const c of grids.values()) {
        if (!c.grid.isConnected) dispose(c);
        else render(c);
      }
      restoreAnchor();
    }
    function schedule() { if (!frame) frame = win.requestAnimationFrame(refresh); }
    function onResize() {
      if (!pendingAnchor) pendingAnchor = [...grids.values()].map(c => c.anchor).find(Boolean) || null;
      grids.forEach(c => { c.relayout = true; });
      schedule();
    }
    const resize = typeof win.ResizeObserver === 'function' ? new win.ResizeObserver(entries => {
      if (entries.some(entry => {
        const c = grids.get(entry.target);
        return c && Math.abs(entry.contentRect.width - (c.geometry?.width || 0)) > .5;
      })) onResize();
    }) : null;
    const mutations = new win.MutationObserver(() => {
      for (const c of grids.values()) if (!c.grid.isConnected) dispose(c);
    });
    mutations.observe(doc.body, { childList: true, subtree: true });
    function trackUserScrollEnd() {
      win.clearTimeout(userScrollTimer);
      userScrollTimer = win.setTimeout(() => {
        if (!touching) userScrollActive = false;
      }, 180);
    }
    function onInput() { userScrollActive = true; trackUserScrollEnd(); }
    function onScroll() {
      const y = win.scrollY || 0;
      // Touch inertia can continue long after the last input event. Keep the
      // deferred layout anchor moving until scrolling actually becomes idle.
      if (pendingAnchor && atScrollBoundary()) pendingAnchor = null;
      else if (pendingAnchor && userScrollActive) pendingAnchor.offset -= y - previousScrollY;
      previousScrollY = y;
      if (userScrollActive) trackUserScrollEnd();
      lastScroll = clock(); schedule();
    }
    function onTouchStart() { touching = true; onInput(); }
    function onTouchEnd() { touching = false; onInput(); restoreAnchor(); }
    win.addEventListener('scroll', onScroll, { passive: true });
    win.addEventListener('resize', onResize, { passive: true });
    win.addEventListener('wheel', onInput, { passive: true });
    doc.addEventListener('touchstart', onTouchStart, { passive: true });
    doc.addEventListener('touchend', onTouchEnd, { passive: true });
    doc.addEventListener('touchcancel', onTouchEnd, { passive: true });
    return {
      set(grid, items, config = {}) {
        let c = grids.get(grid);
        if (c && c.scope !== config.scope) { dispose(c); c = null; }
        if (!c) {
          c = { grid, nodes: new Map(), items: [], list: config.list, scope: config.scope, indices: new Map() };
          // Event closures keep a stable array view while pagination replaces
          // the backing list; recycled cards never retain obsolete page arrays.
          c.liveList = new Proxy([], {
            get: (_, property) => Reflect.get(c.list || [], property),
            has: (_, property) => Reflect.has(c.list || [], property),
          });
          grids.set(grid, c); resize?.observe(grid);
        }
        if (!pendingAnchor && c.anchor) pendingAnchor = c.anchor;
        c.list = config.list;
        c.items = Array.from(items); c.placeholders = config.placeholders || 0;
        c.kind = config.kind; c.cardOptions = config.cardOptions || {};
        c.indices = new Map(c.items.map((item, index) => [keyOf(item), index]));
        render(c); restoreAnchor();
        // Callers can populate a grid just before inserting it in the page.
        if (!grid.isConnected) schedule();
      },
      ensure(id) {
        for (const c of grids.values()) {
          if (!c.grid.isConnected) continue;
          const index = c.items.findIndex(item => Number(item.id) === Number(id));
          if (index < 0) continue;
          pendingAnchor = null; win.clearTimeout(idleTimer);
          render(c, index + c.placeholders);
          return c.nodes.get(keyOf(c.items[index])) || null;
        }
        return null;
      },
      columns(grid) { return grids.get(grid)?.geometry?.columns || 1; },
      items(grid) { return grids.get(grid)?.items || []; },
      remove(ids, grid) {
        const removed = new Set([...ids].map(Number));
        for (const c of grids.values()) {
          if (grid !== undefined && c.grid !== grid) continue;
          if (!pendingAnchor && c.anchor && !removed.has(Number(c.items[c.indices.get(c.anchor.key)]?.id))) pendingAnchor = c.anchor;
          c.items = c.items.filter(item => !removed.has(Number(item.id)));
          c.indices = new Map(c.items.map((item, index) => [keyOf(item), index]));
          render(c);
        }
        restoreAnchor();
      },
      refresh: schedule,
      relayout: onResize,
      stats() {
        return { created, released, mountedPeak, grids: [...grids.values()].map(c => ({
          id: c.grid.id, total: c.items.length + c.placeholders, mounted: c.nodes.size,
          budget: c.geometry?.budget || 0, height: c.geometry?.height || 0,
        })) };
      },
      dispose() {
        for (const c of grids.values()) dispose(c);
        resize?.disconnect(); mutations.disconnect();
        win.cancelAnimationFrame(frame); win.clearTimeout(idleTimer); win.clearTimeout(userScrollTimer);
        win.removeEventListener('scroll', onScroll); win.removeEventListener('resize', onResize);
        win.removeEventListener('wheel', onInput);
        doc.removeEventListener('touchstart', onTouchStart); doc.removeEventListener('touchend', onTouchEnd);
        doc.removeEventListener('touchcancel', onTouchEnd);
      },
    };
  }
  const api = { create, layout };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.EchoGalleryPhotoWall = api;
})(typeof window !== 'undefined' ? window : globalThis);
