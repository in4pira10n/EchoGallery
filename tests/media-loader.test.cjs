const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const vm = require('node:vm');
const { create } = require('../web/static/media-loader.js');
const tick = () => new Promise(resolve => setImmediate(resolve));
function harness(options = {}) {
  const calls = [], revoked = [];
  let serial = 0;
  const loader = create({ limit: 3, maxBytes: 10, maxEntries: 3,
    fetch: (url, { signal }) => new Promise((resolve, reject) => {
      const call = { url, signal, finish: (size = 4) => resolve({ ok: true, blob: async () => ({ size }) }),
        fail: () => reject(new Error('offline')) };
      signal.addEventListener('abort', () => reject(new DOMException('cancelled', 'AbortError')), { once: true });
      calls.push(call);
    }),
    decode: async () => {}, createURL: () => `blob:${++serial}`, revokeURL: url => revoked.push(url),
    ...options,
  });
  return { loader, calls, revoked };
}

test('shares downloads and decode, promotes prefetch, cancels only after last owner leaves', async () => {
  const h = harness();
  const prefetch = h.loader.acquire('library:a:full:7', '/7', 3);
  const foreground = h.loader.acquire('library:a:full:7', '/7', 0);
  prefetch.release();
  assert.equal(h.calls.length, 1);
  assert.equal(h.calls[0].signal.aborted, false);
  h.calls[0].finish();
  assert.equal((await foreground.promise).url, 'blob:1');
  const next = h.loader.acquire('library:a:full:7', '/7', 0);
  assert.equal((await next.promise).url, 'blob:1');
  assert.equal(h.calls.length, 1);
  foreground.release(); next.release(); h.loader.dispose();
});

test('reserves foreground slot and starts visible cards before buffered cards and prefetch', async () => {
  const h = harness();
  h.loader.setPaused(true);
  const handles = [h.loader.acquire('prefetch', '/prefetch', 3), h.loader.acquire('buffer', '/buffer', 2),
    h.loader.acquire('visible', '/visible', 1), h.loader.acquire('current', '/current', 0)];
  h.loader.setPaused(false);
  assert.deepEqual(h.calls.map(c => c.url), ['/current', '/visible']);
  h.calls[0].finish(); await tick();
  assert.deepEqual(h.calls.map(c => c.url), ['/current', '/visible', '/buffer']);
  h.calls[1].finish(); await tick();
  assert.equal(h.calls[3].url, '/prefetch');
  assert.ok(h.loader.stats().peakActive <= 3);
  h.calls.slice(2).forEach(c => c.finish()); await Promise.all(handles.map(h => h.promise));
  handles.forEach(h => h.release()); h.loader.dispose();
});

test('only one speculative request runs, foreground uses reserved slot immediately', async () => {
  const h = harness();
  const a = h.loader.acquire('a', '/a', 3), b = h.loader.acquire('b', '/b', 3);
  assert.equal(h.calls.length, 1);
  const current = h.loader.acquire('c', '/c', 0);
  assert.equal(h.calls.length, 2);
  assert.equal(h.calls[1].url, '/c');
  a.release(); b.release(); current.release();
  await assert.rejects(a.promise, { name: 'AbortError' });
  await assert.rejects(b.promise, { name: 'AbortError' });
  await assert.rejects(current.promise, { name: 'AbortError' });
  h.loader.dispose();
});

test('cancelled and failed requests can be retried without poisoning completion cache', async () => {
  const h = harness();
  let item = h.loader.acquire('a', '/a', 0);
  item.release(); await assert.rejects(item.promise, { name: 'AbortError' });
  item = h.loader.acquire('a', '/a', 0);
  h.calls.at(-1).fail(); await assert.rejects(item.promise, /offline/); item.release();
  item = h.loader.acquire('a', '/a', 0); h.calls.at(-1).finish();
  await item.promise; item.release();
  assert.equal(h.loader.stats().completed, 1);
  h.loader.dispose();
});

test('cache evicts idle blobs by byte/entry budget without revoking pinned resources', async () => {
  const h = harness({ maxEntries: 1 });
  const pinned = h.loader.acquire('a', '/a', 0); h.calls[0].finish(20); await pinned.promise;
  assert.deepEqual(h.revoked, []);
  pinned.release(); assert.deepEqual(h.revoked, ['blob:1']);
  const b = h.loader.acquire('b', '/b', 0); h.calls.at(-1).finish(); await b.promise; b.release();
  const c = h.loader.acquire('c', '/c', 0); h.calls.at(-1).finish(); await c.promise;
  assert.ok(h.revoked.includes('blob:2'));
  c.release(); h.loader.dispose(); assert.ok(h.revoked.includes('blob:3'));
});

test('cancel during decode revokes URL and never publishes stale completion', async () => {
  let finishDecode;
  const h = harness({ decode: (url, signal) => new Promise((resolve, reject) => {
    finishDecode = resolve;
    signal.addEventListener('abort', () => reject(new DOMException('', 'AbortError')), { once: true });
  }) });
  const a = h.loader.acquire('a', '/a', 0); h.calls[0].finish(); await tick();
  a.release(); await assert.rejects(a.promise, { name: 'AbortError' }); finishDecode();
  assert.deepEqual(h.revoked, ['blob:1']); assert.equal(h.loader.stats().completed, 0);
  h.loader.dispose();
});

test('clear aborts pending work, revokes cached objects, pause prevents restarting old queue', async () => {
  const h = harness();
  const a = h.loader.acquire('a', '/a', 3), b = h.loader.acquire('b', '/b', 3);
  h.loader.setPaused(true); h.loader.clear();
  await assert.rejects(a.promise, { name: 'AbortError' });
  await assert.rejects(b.promise, { name: 'AbortError' });
  h.loader.setPaused(false); await tick();
  assert.equal(h.calls.length, 1); assert.equal(h.loader.stats().entries, 0);
  h.loader.dispose();
});

const source = fs.readFileSync(require.resolve('../web/static/app.js'), 'utf8');
function appHarness(mobile = true) {
  const h = harness({ maxDecodedBytes: mobile ? 32 * 1024 * 1024 : Infinity });
  const timers = new Map(), observers = [];
  let timer = 0;
  const lightbox = { open: false, classList: { contains: () => lightbox.open } };
  const context = vm.createContext({
    window: { EchoGalleryMediaLoader: { create: () => h.loader }, removeEventListener() {}, addEventListener() {} },
    state: { serverSettings: { storage_path: 'a', warm_enabled: true }, experimentalPrefetchNeighbors: true,
      activePreloadJobs: new Set() },
    document: { hidden: false, body: {} },
    activeLibraryIdentity: settings => settings.storage_path,
    mediaFileURL: photo => '/full/' + photo.id, mediaThumbURL: photo => '/thumb/' + photo.id,
    isTouchLikeDevice: () => mobile, isVideoMedia: photo => photo?.media_kind === 'video',
    isWarmEnabled: () => context.state.serverSettings.warm_enabled,
    $: selector => selector === '#lightbox' ? lightbox : null,
    setTimeout: fn => { timers.set(++timer, fn); return timer; }, clearTimeout: id => timers.delete(id),
    requestAnimationFrame: fn => { timers.set(++timer, fn); return timer; }, cancelAnimationFrame: id => timers.delete(id),
    MutationObserver: class { constructor(fn) { this.fn = fn; } observe() {} disconnect() {} },
    IntersectionObserver: class { constructor(fn, options) { this.fn = fn; this.options = options; observers.push(this); }
      observe() {} unobserve() {} disconnect() {} },
    appendURLParam: url => url + '?r=1', fetchWithPageSession() {},
    cancelActivePreloads() {}, cancelVisibleThumbnailWarmups() {},
    videoPosterPlaceholder: 'data:poster', console,
  });
  vm.runInContext('let mediaLoadRuntime = null;', context);
  for (const name of ['mediaResourceSpec', 'mediaLoadingRuntime', 'resetMediaLoadingRuntime', 'registerManagedThumbnail',
    'refreshManagedThumbnail', 'cancelMediaPrefetch', 'syncMediaLoadingActivity', 'queueMediaPrefetch',
    'loadManagedLightboxImage', 'prefetchAdjacentMedia', 'prefetchPhotosForLightboxIntent']) {
    const match = source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, name); vm.runInContext(match[0], context);
  }
  context.lightboxIsInteractive = () => lightbox.open && !context.document.hidden;
  const flush = () => { const pending = [...timers.values()]; timers.clear(); pending.forEach(fn => fn()); };
  return { ...h, context, timers, observers, lightbox, flush };
}

test('mobile prefetch debounces quick navigation and only loads in browsing direction', async () => {
  const h = appHarness();
  const photos = [1, 2, 3, 4, 5].map(id => ({ id, media_kind: 'image' }));
  h.context.queueMediaPrefetch(photos, 0); h.context.queueMediaPrefetch(photos, 1);
  assert.equal(h.timers.size, 1); assert.equal(h.calls.length, 0);
  h.flush(); assert.deepEqual(h.calls.map(c => c.url), ['/full/3']);
  vm.runInContext('mediaLoadRuntime.direction = -1', h.context);
  h.context.queueMediaPrefetch(photos, 3); await tick();
  assert.equal(h.calls[0].signal.aborted, true);
  h.flush(); assert.equal(h.calls.at(-1).url, '/full/3');
  h.context.resetMediaLoadingRuntime(); await tick();
});

test('both switches disable speculative loading on touch, including a pending timer', async () => {
  const h = appHarness();
  const photos = [1, 2].map(id => ({ id }));
  h.context.queueMediaPrefetch(photos, 0);
  h.context.state.experimentalPrefetchNeighbors = false;
  h.context.syncMediaLoadingActivity(); h.flush(); assert.equal(h.calls.length, 0);
  h.context.state.experimentalPrefetchNeighbors = true;
  h.context.state.serverSettings.warm_enabled = false;
  h.context.queueMediaPrefetch(photos, 0); h.flush(); assert.equal(h.calls.length, 0);
  h.context.resetMediaLoadingRuntime();
});

test('lightbox promotes in-flight prefetched photo and ignores a stale foreground result', async () => {
  const h = appHarness();
  const photos = [1, 2].map(id => ({ id }));
  h.context.queueMediaPrefetch(photos, 0); h.flush();
  let valid = true, ready = 0;
  const image = { dataset: {} };
  h.context.loadManagedLightboxImage(photos[1], image, () => valid, () => ready++, () => {});
  h.context.cancelMediaPrefetch();
  assert.equal(h.calls.length, 1); assert.equal(h.calls[0].signal.aborted, false);
  valid = false; h.calls[0].finish(); await tick();
  assert.equal(ready, 0); assert.equal(image.src, undefined);
  h.context.resetMediaLoadingRuntime();
});

test('thumbnail only loads in buffer; leaving, removal and background release ownership', async () => {
  const h = appHarness();
  const classes = new Set(['thumb-loading']);
  const card = { classList: { add: c => classes.add(c), remove: (...names) => names.forEach(n => classes.delete(n)) } };
  const image = { isConnected: true, removeAttribute(name) { delete this[name]; }, closest: () => card };
  h.context.registerManagedThumbnail(image, { id: 9 });
  assert.equal(h.calls.length, 0);
  h.observers[0].fn([{ target: image, isIntersecting: true }]);
  assert.equal(h.calls[0].url, '/thumb/9');
  h.observers[1].fn([{ target: image, isIntersecting: true }]);
  h.observers[0].fn([{ target: image, isIntersecting: false }]);
  await tick(); assert.equal(h.calls[0].signal.aborted, true);
  h.observers[0].fn([{ target: image, isIntersecting: true }]);
  h.calls.at(-1).finish(); await tick(); assert.match(image.src, /^blob:/);
  image.onload(); assert.equal(classes.has('thumb-loading'), false);
  h.context.document.hidden = true; h.context.syncMediaLoadingActivity();
  assert.equal(image.src, undefined); assert.equal(h.loader.stats().entries, 0);
  assert.equal(classes.has('thumb-loading'), true);
  h.context.document.hidden = false; h.context.syncMediaLoadingActivity();
  assert.equal(h.calls.length, 3);
  vm.runInContext('mediaLoadRuntime.sweep()', h.context);
  image.isConnected = false;
  vm.runInContext('mediaLoadRuntime.sweep()', h.context); await tick();
  assert.equal(h.calls.at(-1).signal.aborted, true);
  h.context.resetMediaLoadingRuntime();
});

test('library changes revoke old ownership and separate otherwise identical media keys', async () => {
  const h = appHarness();
  const photo = { id: 1, uuid: 'same' };
  const oldKey = h.context.mediaResourceSpec(photo, 'full').key;
  let ready = false;
  h.context.loadManagedLightboxImage(photo, { dataset: {} }, () => true, () => { ready = true; }, () => {});
  h.context.state.serverSettings.storage_path = 'b';
  h.context.mediaLoadingRuntime(); await tick();
  assert.equal(h.calls[0].signal.aborted, true); assert.equal(ready, false);
  assert.notEqual(h.context.mediaResourceSpec(photo, 'full').key, oldKey);
  h.context.resetMediaLoadingRuntime();
});

test('actual lightbox render reuses the neighbor download and releases it on rapid navigation', async () => {
  const h = appHarness();
  const photos = [1, 2, 3].map(id => ({ id, original_name: String(id), media_kind: 'image' }));
  h.context.queueMediaPrefetch(photos, 0); h.flush();
  const image = { dataset: {}, classList: { remove() {} }, removeAttribute(name) { delete this[name]; } };
  const video = { classList: { add() {} }, pause() {}, removeAttribute() {}, load() {} };
  h.lightbox.open = true;
  h.context.$ = selector => ({ '#lb-img': image, '#lb-video': video, '#lightbox': h.lightbox }[selector]
    || { classList: { toggle() {} } });
  Object.assign(h.context.state, { lightboxPhotos: photos, lightboxIndex: 1, lightboxPlaybackToken: 0 });
  for (const name of ['cancelVideoSoundtrackRequest', 'resetLightboxSwipeVisual', 'rememberPhoto',
    'resetLightboxFocusPoint', 'setLightboxMediaLoading', 'syncTouchPlaybackButton', 'clearLightboxMediaSession',
    'updateVideoBookmarkProgress', 'applyLightboxZoom', 'updateLightboxFavoriteButton', 'syncLightboxShareButton',
    'updateVideoBookmarkButton', 'updateLightboxHeaderLayout', 'updateSlideshowControls', 'renderLightboxInfo']) {
    h.context[name] = () => {};
  }
  h.context.lightboxCanLoadMoreForward = () => false;
  h.context.window.EchoGalleryLightbox = { displayFilename: value => value };
  const render = source.match(/^function lbRender\([^]*?^}/m);
  vm.runInContext(render[0], h.context);
  h.context.lbRender();
  assert.equal(h.calls.filter(c => c.url === '/full/2').length, 1);
  assert.equal(h.calls[0].signal.aborted, false);
  h.context.state.lightboxIndex = 2; h.context.lbRender(); await tick();
  assert.equal(h.calls[0].signal.aborted, true);
  h.calls.filter(c => !c.signal.aborted).forEach(c => c.finish()); await tick();
  assert.match(image.src, /^blob:/);
  assert.equal(image.dataset.previewStage, 'full');
  assert.equal(h.loader.stats().pinned, 1);
  h.context.resetMediaLoadingRuntime();
});

test('desktop prefetch keeps one neighbor on each side and PWA serves the shared loader', async () => {
  const h = appHarness(false);
  h.context.queueMediaPrefetch([1, 2, 3].map(id => ({ id })), 1); h.flush();
  assert.equal(h.calls[0].url, '/full/3');
  h.calls[0].finish(); await tick(); assert.equal(h.calls[1].url, '/full/1');
  h.context.resetMediaLoadingRuntime(); await tick();
  const html = fs.readFileSync(require.resolve('../web/pages/app.html'), 'utf8');
  const sw = fs.readFileSync(require.resolve('../web/static/sw.js'), 'utf8');
  assert.ok(html.indexOf('/static/media-loader.js') < html.indexOf('/static/app.js'));
  assert.match(sw, /\/static\/media-loader\.js\?v=/);
});

for (const mobile of [true, false]) {
  test(`visible video metadata is bounded and drops obsolete queued cards (${mobile ? 'touch' : 'desktop'})`, async () => {
    const h = appHarness(mobile);
    const metadata = [];
    h.context.isRootDebugMedia = () => false;
    const request = (kind, photo) => new Promise(resolve => metadata.push({ kind, id: photo.id, finish: resolve }));
    h.context.loadVideoPlaybackPreference = photo => request('playback', photo);
    h.context.probeSubtitleMatch = photo => request('subtitles', photo);
    const images = [1, 2, 3, 4, 5].map(id => ({
      id, dataset: {}, isConnected: true,
      closest: () => ({ classList: { add() {}, remove() {} } }),
      removeAttribute(name) { delete this[name]; },
    }));
    images.forEach(image => h.context.registerManagedThumbnail(image, { id: image.id, media_kind: 'video' }));
    h.observers[0].fn(images.map(target => ({ target, isIntersecting: true })));
    assert.equal(metadata.length, 0, 'buffered cards must not request metadata');
    h.observers[1].fn(images.map(target => ({ target, isIntersecting: true })));
    const active = mobile ? 1 : 2;
    assert.equal(metadata.length, active * 2);
    // Repeated visibility callbacks cannot duplicate active tasks.
    h.observers[1].fn(images.map(target => ({ target, isIntersecting: true })));
    assert.equal(metadata.length, active * 2);
    const obsolete = images[active];
    h.observers[1].fn([{ target: obsolete, isIntersecting: false }]);
    metadata.slice().forEach(call => call.finish());
    await tick();
    assert.equal(metadata.some(call => call.id === obsolete.id), false);
    assert.equal(metadata.length, active * 4);
    h.context.resetMediaLoadingRuntime();
    metadata.slice().forEach(call => call.finish());
    await tick();
    assert.equal(metadata.length, active * 4, 'library teardown must not start queued metadata');
  });
}

test('decoded pixel budget evicts a small compressed full image after its last owner leaves', async () => {
  const h = harness({ maxBytes: 100, maxDecodedBytes: 32 * 1024 * 1024,
    decode: async () => ({ width: 6000, height: 4000 }) });
  const current = h.loader.acquire('full', '/full', 0);
  const shared = h.loader.acquire('full', '/full', 0);
  h.calls[0].finish(1); await current.promise;
  assert.equal(h.loader.stats().estimatedDecodedBytes, 96000000);
  current.release(); assert.equal(h.revoked.length, 0, 'visible shared image stays valid');
  shared.release();
  assert.equal(h.loader.stats().estimatedDecodedBytes, 0);
  assert.deepEqual(h.revoked, ['blob:1']);
  h.loader.dispose();
});

test('mobile avoids speculative decoding of oversized photos; desktop keeps its existing behavior', async () => {
  for (const mobile of [true, false]) {
    const h = appHarness(mobile);
    const photos = [{ id: 1 }, { id: 2, width: 6000, height: 4000 }];
    h.context.queueMediaPrefetch(photos, 0); h.flush();
    assert.equal(h.calls.length, mobile ? 0 : 1);
    h.context.resetMediaLoadingRuntime(); await tick();
  }
});
