const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(require.resolve('../web/static/app.js'), 'utf8');
function harness(video = false, reduced = false) {
  let time = 0, serial = 0, exits = 0;
  const frames = new Map(), values = new Map(), classes = new Set(['open']);
  const classList = { contains: name => classes.has(name), add: (...names) => names.forEach(name => classes.add(name)),
    remove: (...names) => names.forEach(name => classes.delete(name)), toggle(name, value) { value ? classes.add(name) : classes.delete(name); } };
  const style = { setProperty: (name, value) => values.set(name, value), removeProperty: name => values.delete(name) };
  const lightbox = { classList, style };
  const media = { classList: { ...classList, contains: () => false }, style };
  const body = { clientWidth: 400, clientHeight: 800, classList: { contains: () => false }, setPointerCapture() {}, releasePointerCapture() {} };
  const state = { lightboxPhotos: [{ id: 1, media_kind: video ? 'video' : 'image' }], lightboxIndex: 0 };
  const context = vm.createContext({ state, performance: { now: () => time }, Date,
    window: { innerHeight: 800, innerWidth: 400, matchMedia: () => ({ matches: reduced }) },
    requestAnimationFrame: callback => { frames.set(++serial, callback); return serial; }, cancelAnimationFrame: id => frames.delete(id),
    syncMediaLoadingActivity() {}, clearTimeout() {}, $: selector => selector === '#lightbox' ? lightbox : selector === '.lightbox-body' ? body : media,
    clampLightboxValue: (value, min, max) => Math.max(min, Math.min(max, value)),
    isVideoMedia: photo => photo?.media_kind === 'video', currentLightboxMediaElement: () => media,
    isLightboxSwipeExcludedTarget: () => false, setupLightboxSwipePreview() {}, lightboxCanLoadMoreForward: () => false,
    setLightboxDismissFade: progress => values.set('fade', progress), clearLightboxDismissFade: () => values.delete('fade'),
    exitLightboxToContext: () => { exits++; }, lightboxSwipeAnimationMs: 380,
  });
  vm.runInContext('let lightboxTouchDismissMotion = null; let mediaLoadRuntime = null;', context);
  for (const name of ['setLightboxReturnThumbnailWarmup', 'lightboxReturnThumbnailsReady', 'stopLightboxTouchDismissMotion', 'beginLightboxTouchDismiss', 'updateLightboxTouchDismiss',
    'renderLightboxTouchDismiss', 'endLightboxTouchDismiss', 'animateLightboxTouchDismiss',
    'setLightboxDismissGesture', 'clearLightboxDismissGesture', 'clearLightboxUnderzoomState',
    'lightboxUnderzoomVisualProgress', 'setLightboxUnderzoomProgress',
    'beginVideoSurfaceSwipe', 'updateVideoSurfaceSwipe', 'endVideoSurfaceSwipe',
    'beginLightboxSwipe', 'updateLightboxSwipe', 'endLightboxSwipe']) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, name); vm.runInContext(match[0], context);
  }
  const event = (y, type = 'pointermove') => ({ pointerId: 1, pointerType: 'touch', clientX: 200, clientY: y, type,
    target: { closest: selector => selector.includes('lightbox-media-frame') }, preventDefault() {}, stopPropagation() {} });
  const begin = y => context[video ? 'beginVideoSurfaceSwipe' : 'beginLightboxSwipe'](event(y, 'pointerdown'));
  const move = (y, dt = 20) => { time += dt; return context[video ? 'updateVideoSurfaceSwipe' : 'updateLightboxSwipe'](event(y)); };
  const end = (type = 'pointerup') => context[video ? 'endVideoSurfaceSwipe' : 'endLightboxSwipe'](event(0, type));
  const tick = (dt = 16) => { time += dt; const batch = [...frames.values()]; frames.clear(); batch.forEach(callback => callback(time)); };
  const settle = () => { for (let i = 0; i < 120 && frames.size; i++) tick(); assert.equal(frames.size, 0); };
  const y = () => parseFloat(values.get('--lightbox-dismiss-translate-y')) || 0;
  return { context, state, values, frames, begin, move, end, tick, settle, y, wait: dt => { time += dt; }, exits: () => exits };
}
for (const video of [false, true]) {
  test(`${video ? 'video' : 'photo'} dismissal follows the finger and carries release velocity`, async () => {
    const h = harness(video);h.begin(100);h.move(140);h.move(180);
    assert.equal(h.y(), 80, 'no displacement amplification');
    await h.end();assert.equal(h.y(), 80, 'release does not reset the displayed transform');
    h.tick();assert.ok(h.y() > 80);h.settle();assert.equal(h.exits(), 1, 'short fast flick dismisses');
  });
}
test('slow short drag returns without overshoot; reversing upward cancels a long drag', async () => {
  for (const reverse of [false, true]) {
    const h = harness();h.begin(100);
    if (reverse) { h.move(400, 100);h.move(350, 20); }
    else { h.move(180, 200);h.wait(150); }
    await h.end();h.settle();assert.equal(h.exits(), 0);assert.equal(h.y(), 0);assert.equal(h.values.has('fade'), false);
  }
});
test('new touch grabs an exit in progress from its current position and can cancel it', async () => {
  const h = harness();h.begin(100);h.move(160);h.move(210);await h.end();h.tick();
  const current = h.y();assert.ok(current > 110);
  h.begin(400);assert.equal(h.y(), current, 'grabbing must not jump');
  h.tick();assert.equal(h.y(), current, 'old animation no longer runs');
  h.move(380);assert.ok(Math.abs(h.y() - (current - 20)) < .02);
  await h.end('pointercancel');h.settle();assert.equal(h.exits(), 0);assert.equal(h.y(), 0);
});
test('closing or changing media cancels outstanding dismissal frames', async () => {
  const h = harness();h.begin(100);h.move(200);await h.end();
  h.context.clearLightboxDismissGesture();h.settle();assert.equal(h.exits(), 0);
});
test('reduced motion uses opacity without displacement or shrink', async () => {
  const h = harness(false, true);h.begin(100);h.move(180);h.move(260);
  assert.equal(h.y(), 0);assert.equal(h.values.get('--lightbox-dismiss-progress'), '0.0000');
  await h.end();h.tick();assert.equal(h.y(), 0);h.settle();assert.equal(h.exits(), 1);
});

test('a slow long drag dismisses only after the motion finishes', async () => {
  const h = harness(true);h.begin(100);h.move(310, 300);h.wait(150);await h.end();
  assert.equal(h.exits(), 0);assert.equal(h.y(), 210);
  let previous = h.y();
  for (let i = 0; i < 120 && h.frames.size; i++) {
    h.tick();assert.ok(h.y() >= previous);assert.ok(h.y() <= 800);previous = h.y();
  }
  assert.equal(h.exits(), 1);assert.equal(h.frames.size, 0);
});

function timelineReturn(h) {
  const c = h.context;
  let resolve, reject, requests = 0, mounts = 0, closes = 0;
  const response = new Promise((yes, no) => { resolve = yes; reject = no; });
  c.state.lightboxReturnView = 'timeline';
  c.state.mediaKindFilter = 'all';
  c.activeLibraryIdentity = () => 'library';c.mediaKindMatchesFilter = () => true;
  c.loadTimelineLocateWindow = () => { requests++;return response; };
  c.resetTimelineToInitialPage = () => {};
  c.switchView = () => { mounts++;c.finishLightboxReturnTransition(); };
  c.findPhotoThumb = () => mounts ? {} : null;
  c.closeLightbox = () => { closes++;c.clearLightboxDismissGesture(); };
  c.console = { warn() {} };c.showToast = () => {};c.copyText = key => key;
  for (const name of ['exitLightboxToContext', 'prepareTimelineLightboxReturn', 'openInTimeline', 'finishLightboxReturnTransition']) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, name);vm.runInContext(match[0], c);
  }
  return { resolve: () => resolve({ photos: [{ id: 1, original_name: 'fresh.jpg' }] }), reject: () => reject(new Error('offline')),
    requests: () => requests, mounts: () => mounts, closes: () => closes };
}
test('timeline preparation starts at release and closes only when both grid and animation are ready', async () => {
  const h = harness(), timeline = timelineReturn(h);
  h.begin(100);h.move(200);await h.end();assert.equal(timeline.requests(), 1, 'request starts before the next animation frame');
  assert.equal(h.y(), 100);timeline.resolve();await new Promise(setImmediate);
  assert.equal(timeline.mounts(), 1);assert.equal(timeline.closes(), 0, 'fast IO cannot cut off the moving media');
  h.settle();assert.equal(timeline.closes(), 1);assert.equal(timeline.requests(), 1);
});
test('a slow timeline response completes without a second locate request', async () => {
  const h = harness(), timeline = timelineReturn(h);
  h.begin(100);h.move(200);await h.end();h.settle();assert.equal(timeline.closes(), 0);
  timeline.resolve();await new Promise(setImmediate);
  assert.equal(timeline.closes(), 1);assert.equal(timeline.requests(), 1);
});
test('interrupting dismissal invalidates a pending timeline response', async () => {
  const h = harness(), timeline = timelineReturn(h);
  h.begin(100);h.move(200);await h.end();h.tick();h.begin(400);
  timeline.resolve();await new Promise(setImmediate);
  assert.equal(timeline.mounts(), 0);assert.equal(timeline.closes(), 0);
  await h.end('pointercancel');h.settle();assert.equal(h.y(), 0);
});
test('a failed slow timeline return restores visible media and permits retry', async () => {
  const h = harness(), timeline = timelineReturn(h);
  h.begin(100);h.move(200);await h.end();h.settle();timeline.reject();await new Promise(setImmediate);
  h.settle();assert.equal(h.y(), 0);assert.equal(timeline.closes(), 0);assert.equal(h.state.lightboxReturnInFlight, false);
});

test('a mounted return card keeps its media object when locate returns fresh instances', async () => {
  const h = harness(), timeline = timelineReturn(h);
  const photo = h.state.lightboxPhotos[0];
  const image = { isConnected: true, closest: () => ({}), getBoundingClientRect: () => ({ top: 1000, bottom: 1100, width: 100 }) };
  h.context.retained = { photo, image };
  vm.runInContext('mediaLoadRuntime = { cards: new Map([[retained.image, retained]]) }', h.context);
  h.begin(100);h.move(200);await h.end();timeline.resolve();await new Promise(setImmediate);
  assert.equal(h.state.timelineReturnWindow.photos[0], photo);
  assert.equal(photo.original_name, 'fresh.jpg');h.settle();assert.equal(timeline.closes(), 1);
});
test('timeline DOM readiness cannot close the lightbox before visible thumbnail pixels are ready', async () => {
  const h = harness(), timeline = timelineReturn(h);
  let pixelsReady = false;h.context.lightboxReturnThumbnailsReady = () => pixelsReady;
  h.begin(100);h.move(200);await h.end();timeline.resolve();await new Promise(setImmediate);h.settle();
  assert.equal(timeline.mounts(), 1);assert.equal(timeline.closes(), 0);
  pixelsReady = true;h.context.finishLightboxReturnTransition();
  assert.equal(timeline.closes(), 1);
});
