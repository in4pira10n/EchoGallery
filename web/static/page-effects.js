/* Viewport-only page effects. No library data, navigation or image gestures live here. */
(function (global) {
  'use strict';
  function edgeFadeHeights(canvas, upper = [], lower = [], extension = 50) {
    return {
      top: Math.max(0, ...upper.map(rect => rect.bottom - canvas.top)) + extension,
      bottom: Math.max(0, ...lower.map(rect => canvas.bottom - rect.top)) + extension,
    };
  }
  function visible(element) {
    if (!element || element.closest('[hidden], [inert], [aria-hidden="true"]') || !element.getClientRects().length) return false;
    const style = getComputedStyle(element);
    return style.visibility !== 'hidden' && style.display !== 'none' && Number(style.opacity) !== 0;
  }
  function createEdgeFades({ container, controls, extension = 50 }) {
    const doc = container.ownerDocument, view = doc.defaultView;
    const layer = doc.createElement('div');
    layer.className = 'page-edge-fades';
    layer.setAttribute('aria-hidden', 'true');
    container.append(layer);
    let frame = 0, destroyed = false;
    const tracked = new Set();
    const schedule = () => {
      if (!destroyed && !frame) frame = view.requestAnimationFrame(() => { frame = 0; update(); });
    };
    const resize = new view.ResizeObserver(schedule);
    // Observe attributes on the shell, not photo-wall DOM mutations while scrolling.
    const mutations = new view.MutationObserver(schedule);
    function update() {
      if (destroyed) return;
      const elements = [...controls()];
      const next = new Set(elements);
      const changed = next.size !== tracked.size || elements.some(element => !tracked.has(element));
      for (const element of tracked) {
        if (!next.has(element)) { resize.unobserve(element); tracked.delete(element); }
      }
      for (const element of elements) {
        if (!tracked.has(element)) { tracked.add(element); resize.observe(element); }
      }
      if (changed) {
        mutations.disconnect();
        const ancestors = new Set();
        for (const element of elements) {
          mutations.observe(element, { attributes: true, childList: true, subtree: true,
            attributeFilter: ['hidden', 'inert', 'aria-hidden', 'class', 'style'] });
          for (let parent = element.parentElement; parent; parent = parent.parentElement) {
            if (ancestors.has(parent)) continue;
            ancestors.add(parent);
            mutations.observe(parent, { attributes: true, attributeFilter: ['hidden', 'inert', 'aria-hidden', 'class'] });
          }
        }
      }
      const upper = [], lower = [], canvas = layer.getBoundingClientRect();
      for (const element of elements) {
        if (!visible(element)) continue;
        const rect = element.getBoundingClientRect();
        if (!rect.width || !rect.height || rect.bottom <= canvas.top || rect.top >= canvas.bottom) continue;
        ((rect.top + rect.bottom) / 2 < (canvas.top + canvas.bottom) / 2 ? upper : lower).push(rect);
      }
      const heights = edgeFadeHeights(canvas, upper, lower, extension);
      for (const edge of ['top', 'bottom']) {
        const value = `${heights[edge]}px`;
        if (layer.style.getPropertyValue(`--page-fade-${edge}`) !== value) layer.style.setProperty(`--page-fade-${edge}`, value);
      }
    }
    view.addEventListener('resize', schedule);
    view.visualViewport?.addEventListener('resize', schedule);
    view.visualViewport?.addEventListener('scroll', schedule);
    // Register once: reconnecting ResizeObserver on every update would cause
    // another initial observation and keep requestAnimationFrame running at idle.
    resize.observe(layer);
    update();
    return { element: layer, update: schedule, measure: update, destroy() {
      destroyed = true; view.cancelAnimationFrame(frame); resize.disconnect(); mutations.disconnect();
      view.removeEventListener('resize', schedule);
      view.visualViewport?.removeEventListener('resize', schedule);
      view.visualViewport?.removeEventListener('scroll', schedule);
      layer.remove();
    } };
  }

  let previewSVGSerial = 0;
  function preparePreviewTree(tree) {
    const svgs = [...(tree.matches('svg') ? [tree] : []), ...tree.querySelectorAll('svg')];
    for (const svg of svgs) {
      const nodes = [svg, ...svg.querySelectorAll('*')];
      const ids = new Map();
      for (const node of nodes) {
        if (node.id) ids.set(node.id, `eg-preview-svg-${++previewSVGSerial}`);
      }
      // SVG definitions are part of the drawing. Strip page IDs, but namespace
      // local paint/clip/use references so the live page cannot change a preview.
      const reference = value => {
        const hash = value.lastIndexOf('#');
        if (hash < 0 || !ids.has(value.slice(hash + 1))) return value;
        const base = value.slice(0, hash);
        if (base && new URL(base, svg.baseURI).href !== svg.ownerDocument.URL.split('#')[0]) return value;
        return `#${ids.get(value.slice(hash + 1))}`;
      };
      for (const node of nodes) {
        for (const attribute of [...node.attributes]) {
          if (attribute.name === 'id') {
            node.id = ids.get(attribute.value);
          } else {
            let value = attribute.value.replace(/url\(\s*(['"]?)([^'"\s)]+)\1\s*\)/g,
              (_, quote, url) => `url("${reference(url)}")`);
            if (attribute.localName === 'href') value = reference(value);
            if (value !== attribute.value) node.setAttributeNS(attribute.namespaceURI, attribute.name, value);
          }
        }
      }
    }
    for (const node of [tree, ...tree.querySelectorAll('[id]')]) {
      if (!node.closest('svg')) node.removeAttribute('id');
    }
    return tree;
  }

  // Freeze just the currently painted cards and small chrome trees, never a full
  // library grid. Removing IDs also prevents a preview from changing live selectors.
  function capturePage({ content = [], controls = [], fades, height = innerHeight, width = innerWidth } = {}) {
    const page = document.createElement('div');
    page.className = 'page-swipe-snapshot';
    const pieces = [...content.map(element => [element, 0]), ...(fades ? [[fades, 30]] : []), ...controls.map(element => [element, 40])];
    for (const [source, z] of pieces) {
      if (source !== fades && !visible(source)) continue;
      const rect = source.getBoundingClientRect();
      if (!rect.width || !rect.height || rect.bottom <= 0 || rect.top >= height || rect.right <= 0 || rect.left >= width) continue;
      const copy = source.cloneNode(true);
      const originals = [source, ...source.querySelectorAll('*')];
      const copies = [copy, ...copy.querySelectorAll('*')];
      originals.forEach((node, i) => {
        // Content already has reusable EG classes. Only chrome needs frozen
        // geometry/paint after its ID and page context disappear. Inherited theme
        // variables stay live rather than being duplicated on every SVG path.
        if (z >= 40) {
          const style = getComputedStyle(node);
          for (const property of style) {
            if (!property.startsWith('--')) copies[i].style.setProperty(property, style.getPropertyValue(property));
          }
        }
        copies[i].removeAttribute('autofocus');
        copies[i].style.setProperty('animation', 'none');
        copies[i].style.setProperty('transition', 'none');
        if (node.tagName === 'IMG' && node.currentSrc) {
          copies[i].src = node.currentSrc;
          copies[i].removeAttribute('srcset');
          copies[i].loading = 'eager';
          copies[i].decoding = 'sync';
        }
      });
      Object.assign(copy.style, { position: 'absolute', inset: 'auto', left: `${rect.left}px`, top: `${rect.top}px`,
        width: `${rect.width}px`, height: `${rect.height}px`, boxSizing: 'border-box', margin: '0', transform: 'none', translate: 'none', zIndex: String(z) });
      page.append(preparePreviewTree(copy));
    }
    return page;
  }
  function swipeDecision(x, speed, width) {
    return speed > -250 && (x >= width * .36 || (x >= 40 && speed >= 500));
  }
  function springStep(x, velocity, target, dt) {
    const omega = 24, displacement = x - target, c = velocity + omega * displacement, decay = Math.exp(-omega * dt);
    return { x: target + (displacement + c * dt) * decay, velocity: (velocity - omega * c * dt) * decay };
  }
  function installSwipeBack({ isEnabled, createPreview, createCurrent, onBack, onError = console.error, surface = document }) {
    const view = surface.defaultView || window, doc = view.document, root = doc.documentElement;
    let pending, drag, frame = 0, settling = false, committing = false, x = 0, velocity = 0;
    let foreground, preview, destination, suppressClickUntil = 0, generation = 0;
    const reduced = () => view.matchMedia('(prefers-reduced-motion: reduce)').matches;
    function paint() {
      const progress = Math.max(0, Math.min(1, x / view.innerWidth));
      if (reduced()) {
        foreground.style.opacity = String(1 - progress);
      } else {
        foreground.style.transform = `translate3d(${Math.max(0, x)}px,0,0)`;
        preview.style.transform = `translate3d(${-view.innerWidth * .24 * (1 - progress)}px,0,0)`;
        preview.style.filter = `brightness(${.88 + .12 * progress})`;
      }
    }
    function reset() {
      generation++; view.cancelAnimationFrame(frame); frame = 0;
      const id = (drag || pending)?.id;
      if (id !== undefined && root.hasPointerCapture?.(id)) root.releasePointerCapture(id);
      pending = drag = null; settling = committing = false; x = velocity = 0;
      foreground?.remove(); preview?.remove(); foreground = preview = destination = null;
      root.classList.remove('page-back-swiping', 'page-back-committing');
    }
    async function finish(commit) {
      if (!commit || !isEnabled()) { reset(); return; }
      committing = true; root.classList.add('page-back-committing');
      const token = generation;
      try {
        if (await onBack(destination) === false) { committing = false; root.classList.remove('page-back-committing'); settle(false); return; }
        if (token !== generation) return;
        // Cover the actual parent until its chrome and restored scroll have painted.
        foreground.remove(); foreground = null;
        preview.style.transform = 'none'; preview.style.filter = 'none';
        root.classList.remove('page-back-swiping');
        await new Promise(resolve => view.requestAnimationFrame(() => view.requestAnimationFrame(resolve)));
        if (token === generation) reset();
      } catch (error) {
        if (token === generation) reset();
        onError(error);
      }
    }
    function settle(commit, speed = 0) {
      view.cancelAnimationFrame(frame); pending = drag = null; settling = true; velocity = speed;
      const target = commit ? view.innerWidth : 0;
      if (reduced()) { x = target; paint(); void finish(commit); return; }
      let previous = view.performance.now();
      function step(now) {
        if (!isEnabled()) { reset(); return; }
        const result = springStep(x, velocity, target, Math.min((now - previous) / 1000, .04)); previous = now;
        x = result.x; velocity = result.velocity; paint();
        if (Math.abs(x - target) < .5 && Math.abs(velocity) < 8) { settling = false; void finish(commit); }
        else frame = view.requestAnimationFrame(step);
      }
      frame = view.requestAnimationFrame(step);
    }
    function start(event) {
      if (event.pointerType !== 'touch') return;
      if (event.isPrimary === false) { if (drag || settling) settle(false); pending = null; return; }
      if (committing || !isEnabled() || event.clientX < 0 || event.clientX > view.innerWidth / 4
        || event.target.closest('button, a, input, textarea, select, [contenteditable="true"], [role="slider"], [role="dialog"], .context-menu')
        || view.getSelection()?.isCollapsed === false) return;
      if (settling) { view.cancelAnimationFrame(frame); settling = false; }
      pending = { id: event.pointerId, x: event.clientX, y: event.clientY, offset: x, time: view.performance.now() };
    }
    function move(event) {
      const current = drag || pending;
      if (!current || event.pointerId !== current.id) return;
      if (!isEnabled()) { reset(); return; }
      const dx = event.clientX - current.x, dy = event.clientY - current.y, now = view.performance.now();
      if (!drag) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) < 10) return;
        if (dx <= 0 || Math.abs(dy) >= dx / 1.4 || view.getSelection()?.isCollapsed === false) {
          pending = null; if (x > 0) settle(false); return;
        }
        if (!preview) {
          destination = createPreview();
          if (!destination?.page) { reset(); return; }
          foreground = doc.createElement('div'); foreground.className = 'page-swipe-foreground';
          foreground.inert = true; foreground.setAttribute('aria-hidden', 'true');
          foreground.append(createCurrent());
          preview = doc.createElement('div'); preview.className = 'page-swipe-preview';
          preview.inert = true; preview.setAttribute('aria-hidden', 'true');
          preview.append(preparePreviewTree(destination.page.cloneNode(true)));
          doc.body.append(preview, foreground);
          root.classList.add('page-back-swiping');
        }
        drag = { ...current, samples: [{ x: current.x, time: current.time }] }; pending = null;
        try { root.setPointerCapture?.(event.pointerId); } catch (_) {}
      }
      if (event.cancelable) event.preventDefault();
      drag.samples.push({ x: event.clientX, time: now });
      drag.samples = drag.samples.filter(sample => now - sample.time <= 100);
      x = Math.min(view.innerWidth, Math.max(0, drag.offset + dx)); paint();
    }
    function end(event) {
      const current = drag || pending;
      if (!current || event.pointerId !== current.id) return;
      const active = drag; pending = drag = null;
      if (root.hasPointerCapture?.(event.pointerId)) root.releasePointerCapture(event.pointerId);
      if (!active) { if (x > 0) settle(false); return; }
      suppressClickUntil = view.performance.now() + 450;
      active.samples.push({ x: event.clientX, time: view.performance.now() });
      const last = active.samples.at(-1), first = active.samples.find(sample => last.time - sample.time <= 100) || last;
      const speed = last.time > first.time ? (last.x - first.x) * 1000 / (last.time - first.time) : 0;
      settle(event.type !== 'pointercancel' && swipeDecision(x, speed, view.innerWidth), speed);
    }
    const touchMove = event => { if (drag && event.cancelable) event.preventDefault(); };
    const click = event => { if (view.performance.now() < suppressClickUntil) { event.preventDefault(); event.stopImmediatePropagation(); } };
    const context = () => { if (drag) settle(false); pending = null; };
    const resize = () => { if (!committing) reset(); };
    const handlers = [['pointerdown', start], ['pointermove', move], ['pointerup', end], ['pointercancel', end], ['touchmove', touchMove], ['click', click], ['contextmenu', context]];
    handlers.forEach(([name, handler]) => surface.addEventListener(name, handler, { capture: true, passive: false }));
    view.addEventListener('resize', resize); view.addEventListener('blur', reset); view.addEventListener('pagehide', reset);
    return { cancel: reset, destroy() {
      reset(); handlers.forEach(([name, handler]) => surface.removeEventListener(name, handler, true));
      view.removeEventListener('resize', resize); view.removeEventListener('blur', reset); view.removeEventListener('pagehide', reset);
    } };
  }
  const api = { edgeFadeHeights, createEdgeFades, preparePreviewTree, capturePage, swipeDecision, springStep, installSwipeBack };
  if (typeof module === 'object' && module.exports) module.exports = api;
  else global.EchoPageEffects = api;
})(typeof window === 'object' ? window : globalThis);
