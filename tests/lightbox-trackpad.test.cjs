const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(require.resolve('../web/static/app.js'), 'utf8');
function load(context, ...names) {
  for (const name of names) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, name); vm.runInContext(match[0], context);
  }
}
function harness(videoMode = false) {
  const timers = new Map(), calls = [];
  let serial = 0, open = true, zoomed = false, modal = false;
  const photo = { id: 1, media_kind: videoMode ? 'video' : 'image' };
  const body = { clientWidth: 600, clientHeight: 600, classList: { contains: () => zoomed } };
  const media = { duration: 120, classList: { contains: () => false, add() {}, remove() {} }, style: { setProperty() {} } };
  const context = vm.createContext({
    state: { lightboxPhotos: [photo, { id: 2 }], lightboxIndex: 0, lightboxZoom: 100 },
    document: { querySelector: () => modal ? {} : null },
    window: { innerWidth: 600, innerHeight: 600 },
    getComputedStyle: () => ({ transform: 'matrix(1, 0, 0, 1, -80, 0)' }),
    DOMMatrixReadOnly: class { constructor() { this.m41 = -80; } },
    $: selector => selector === '.lightbox-body' ? body : media,
    lightboxIsInteractive: () => open,
    isVideoMedia: photo => photo?.media_kind === 'video',
    isLightboxSwipeExcludedTarget: target => target.excluded,
    lightboxCanLoadMoreForward: () => false,
    currentLightboxMediaElement: () => media,
    beginLightboxSwipe(e) {
      calls.push(['begin-photo']);
      context.state.lightboxSwipe = { pointerId: -1, startX: 0, startY: 0, width: 600, height: 600 };
      return true;
    },
    beginVideoSurfaceSwipe(e) {
      calls.push(['begin-video']);
      context.state.videoSurfaceSwipe = { pointerId: -1, startX: 0, startY: 0, startTime: 10, width: 600, height: 600 };
      return true;
    },
    endLightboxSwipe() { calls.push(['end-photo', context.state.lightboxSwipe.mode]); context.state.lightboxSwipe = null; },
    endVideoSurfaceSwipe() { calls.push(['end-video', context.state.videoSurfaceSwipe.mode]); context.state.videoSurfaceSwipe = null; },
    clampLightboxValue: (value, min, max) => Math.max(min, Math.min(max, value)),
    setLightboxDismissGesture: (...args) => calls.push(['dismiss', ...args]),
    setLightboxUnderzoomProgress: value => calls.push(['underzoom', value]),
    setLightboxDismissFade() {},
    setLightboxSwipePreviewTransform: x => calls.push(['preview', x]),
    seekVideoToTime: (video, time) => calls.push(['seek', time]),
    resetLightboxSwipeVisual: options => calls.push(['reset-preview', options]),
    clearLightboxDismissGesture() {}, clearLightboxUnderzoomState() {},
    activeLibraryIdentity: () => 'library-1',
    lightboxSwipeAnimationMs: 380,
    lbNav: async direction => { calls.push(['navigate', direction]); },
    updateLightboxFocusPointFromPointer() {},
    setLightboxZoom: value => { context.state.lightboxZoom = value; },
    setLightboxFit: () => { context.state.lightboxZoom = 100; },
    startLightboxUnderzoomExit: () => calls.push(['exit']),
    setTimeout: callback => { timers.set(++serial, callback); return serial; },
    clearTimeout: id => timers.delete(id),
  });
  vm.runInContext('let lightboxTrackpad = null;', context);
  load(context, 'resetLightboxTrackpad', 'lightboxTrackpadTarget', 'finishLightboxTrackpad',
    'handleLightboxTrackpadWheel', 'handleLightboxTrackpadGesture', 'lightboxUnderzoomProgressForZoom',
    'updateLightboxSwipe', 'updateVideoSurfaceSwipe');
  const event = (dx, dy, options = {}) => ({ deltaX: dx, deltaY: dy, deltaMode: 0, clientX: 300, clientY: 300,
    target: { closest: () => true }, preventDefault() { this.prevented = true; }, stopPropagation() {}, ...options });
  const flush = () => { const callbacks = [...timers.values()]; timers.clear(); callbacks.forEach(fn => fn()); };
  return { context, calls, timers, event, flush, setOpen: value => { open = value; },
    setZoomed: value => { zoomed = value; }, setModal: value => { modal = value; } };
}

test('navigation loading remains tracked without displaying the overlay; initial opening may display it', () => {
  let shown;
  const context = vm.createContext({ state: { lightboxSuppressLoading: true },
    $: () => ({ classList: { toggle: (name, value) => { shown = value; } } }) });
  load(context, 'setLightboxMediaLoading');
  context.setLightboxMediaLoading(true);
  assert.equal(context.state.lightboxMediaLoading, true); assert.equal(shown, false);
  context.state.lightboxSuppressLoading = false; context.setLightboxMediaLoading(true);
  assert.equal(shown, true);
  const go = source.match(/^async function lbGoTo\([^]*?^}/m)[0];
  assert.match(go, /state\.lightboxSuppressLoading = true/);
  const open = source.match(/^async function openLightbox\([^]*?^}/m)[0];
  assert.match(open, /state\.lightboxSuppressLoading = false/);
});

test('precision horizontal scroll follows every delta beyond the threshold and settles only when idle', () => {
  const h = harness();
  assert.equal(h.context.handleLightboxTrackpadWheel(h.event(25, 1)), true);
  assert.equal(h.context.state.lightboxSwipe.mode, 'horizontal');
  h.context.handleLightboxTrackpadWheel(h.event(45, 0));
  assert.equal(h.calls.filter(c => c[0] === 'end-photo').length, 0);
  for (let n = 0; n < 10; n++) h.context.handleLightboxTrackpadWheel(h.event(15, 0));
  assert.equal(h.calls.filter(c => c[0] === 'preview').at(-1)[1], -220);
  assert.equal(h.calls.filter(c => c[0] === 'begin-photo').length, 1);
  h.flush();
  assert.equal(h.calls.filter(c => c[0] === 'end-photo').length, 0);
  h.context.handleLightboxTrackpadWheel(h.event(2, 0));
  assert.equal(h.calls.filter(c => c[0] === 'begin-photo').length, 1);
  assert.equal(h.calls.filter(c => c[0] === 'preview').at(-1)[1], -82);
});

test('downward content dragging reuses dismiss feedback for both photos and videos', () => {
  for (const video of [false, true]) {
    const h = harness(video);
    h.context.handleLightboxTrackpadWheel(h.event(0, -140));
    assert.ok(h.calls.some(c => c[0] === 'dismiss'));
    assert.ok(!h.calls.some(c => c[0] === (video ? 'end-video' : 'end-photo')));
    h.flush();
    assert.ok(h.calls.some(c => c[0] === (video ? 'end-video' : 'end-photo') && c[1] === 'dismiss'));
  }
});

test('photo drag can reverse after crossing the threshold and snap back without navigating', () => {
  const h = harness();
  load(h.context, 'endLightboxSwipe');
  h.context.handleLightboxTrackpadWheel(h.event(180, 0));
  h.context.handleLightboxTrackpadWheel(h.event(-165, 0));
  assert.equal(h.calls.filter(c => c[0] === 'preview').at(-1)[1], -15);
  h.flush();
  assert.equal(h.calls.filter(c => c[0] === 'navigate').length, 0);
  assert.equal(h.calls.filter(c => c[0] === 'preview').at(-1)[1], 0);
});

test('new trackpad input interrupts a pending navigation from its current visual position', async () => {
  const h = harness();
  h.context.handleLightboxTrackpadWheel(h.event(180, 0));
  h.flush();
  h.context.handleLightboxTrackpadWheel(h.event(-75, 0));
  assert.equal(h.calls.filter(c => c[0] === 'preview').at(-1)[1], -5);
  h.flush();
  h.flush();
  await new Promise(setImmediate);
  assert.equal(h.calls.filter(c => c[0] === 'navigate').length, 0);
});

test('photo navigation is decided from the final displacement after input ends', async () => {
  const h = harness();
  load(h.context, 'endLightboxSwipe');
  h.context.handleLightboxTrackpadWheel(h.event(100, 0));
  h.context.handleLightboxTrackpadWheel(h.event(80, 0));
  assert.equal(h.calls.filter(c => c[0] === 'preview').at(-1)[1], -180);
  assert.equal(h.calls.filter(c => c[0] === 'navigate').length, 0);
  h.flush();
  h.flush();
  await new Promise(setImmediate);
  assert.deepEqual(h.calls.filter(c => c[0] === 'navigate'), [['navigate', 1]]);
});

test('video horizontal gestures seek, upward movement never changes audio volume', () => {
  const h = harness(true);
  h.context.handleLightboxTrackpadWheel(h.event(-90, 0));
  assert.ok(h.calls.find(c => c[0] === 'seek')[1] > 10);
  h.flush();
  h.flush(); // Allow the completed gesture's inertia latch to expire.
  h.context.handleLightboxTrackpadWheel(h.event(0, 80));
  assert.equal(h.context.state.videoSurfaceSwipe.mode, undefined);
  assert.equal(h.calls.filter(c => c[0] === 'seek').length, 1);
  h.flush();
});

test('controls, dialogs, discrete wheel scrolling, zoomed pictures and real touch retain ownership', () => {
  const h = harness();
  for (const options of [{ deltaMode: 1 }, { deltaMode: 2 }, { altKey: true },
    { target: { excluded: true, closest: () => true } }]) {
    assert.equal(h.context.handleLightboxTrackpadWheel(h.event(50, 0, options)), false);
  }
  h.setModal(true); assert.equal(h.context.handleLightboxTrackpadWheel(h.event(50, 0)), false);
  h.setModal(false); h.setZoomed(true);
  assert.equal(h.context.handleLightboxTrackpadWheel(h.event(50, 0)), false);
  h.setZoomed(false); h.context.state.lightboxSwipe = { pointerId: 4 };
  assert.equal(h.context.handleLightboxTrackpadWheel(h.event(50, 0)), false);
  h.context.state.lightboxSwipe = null; h.setOpen(false);
  assert.equal(h.context.handleLightboxTrackpadWheel(h.event(50, 0)), false);
});

test('Chromium control-wheel pinch and Safari gesture scale use lightbox zoom and underzoom exit', () => {
  const h = harness();
  const e = h.event(0, -20, { ctrlKey: true });
  assert.equal(h.context.handleLightboxTrackpadWheel(e), true);
  assert.ok(h.context.state.lightboxZoom > 100); assert.equal(e.prevented, true);
  h.flush();
  h.context.state.lightboxZoom = 100;
  h.context.handleLightboxTrackpadGesture(h.event(0, 0, { type: 'gesturestart' }));
  h.context.handleLightboxTrackpadGesture(h.event(0, 0, { type: 'gesturechange', scale: .85 }));
  assert.equal(h.context.state.lightboxZoom, 85);
  h.context.handleLightboxTrackpadGesture(h.event(0, 0, { type: 'gestureend' }));
  assert.equal(h.context.state.lightboxZoom, 100);
  h.context.handleLightboxTrackpadWheel(h.event(0, 40, { ctrlKey: true })); h.flush();
  assert.ok(h.calls.some(c => c[0] === 'exit'));
});
