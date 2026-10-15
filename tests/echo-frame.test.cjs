const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname, '../web/static/echo-frame.js'), 'utf8');

function harness(get = async () => ({ photos: [] })) {
  const context = vm.createContext({
    document: { hidden: false, addEventListener() {} }, window: { addEventListener() {} },
    state: { serverSettings: { active_library_id: 'a' } }, api: { get },
    AbortController, DOMException, URLSearchParams, setTimeout: () => 1, clearTimeout() {},
    hashString: str => [...str].reduce((n, c) => (n * 31 + c.charCodeAt(0)) >>> 0, 0),
    isVideoMedia: photo => photo.media_kind === 'video', showToast() {},
  });
  vm.runInContext(source, context);
  const frame = {
    library: 'a', abort: new AbortController(), p: { videos: true, end: 'stop', interval: 30000 },
    order: 'random', paths: ['/api/media/random'], albumIDs: [], seed: 10, seen: new Set(),
    history: [], historyIndex: -1, surface: {}, root: { querySelectorAll: () => [] },
  };
  context.frame = frame;
  vm.runInContext('echoFrame = frame', context);
  context.echoFrameResetSources(frame);
  return { context, frame };
}

test('empty sources stop without restarting or repeatedly querying', async () => {
  let requests = 0;
  const { context, frame } = harness(async () => { requests++; return { photos: [], has_more: false }; });
  frame.p.end = 'reshuffle';
  assert.equal(await context.echoFrameNextPhoto(frame), null);
  assert.equal(requests, 1);
});

test('pagination deduplicates album overlaps and advances across empty pages', async () => {
  const calls = [];
  const { context, frame } = harness(async url => {
    calls.push(url);
    const cursor = new URL(url, 'http://test').searchParams.get('cursor');
    if (!cursor) return { photos: [], has_more: true, next_cursor: 'next' };
    return { photos: [{ uuid: 'one', id: 1 }, { uuid: 'two', id: 2 }], has_more: false };
  });
  frame.paths = ['/api/media/albums/1', '/api/media/albums/2'];
  frame.order = 'asc';
  context.echoFrameResetSources(frame);
  assert.equal((await context.echoFrameNextPhoto(frame)).uuid, 'one');
  assert.equal((await context.echoFrameNextPhoto(frame)).uuid, 'two');
  assert.equal(await context.echoFrameNextPhoto(frame), null);
  assert.equal(calls.length, 4);
});

test('random favorites and multiple albums are filtered server-side', async () => {
  let url;
  const { context, frame } = harness(async value => { url = value; return { photos: [] }; });
  frame.albumIDs = [8, 12];
  frame.p = { source: 'favorites', videos: false };
  await context.echoFrameFill(frame, frame.sources[0]);
  const params = new URL(url, 'http://test').searchParams;
  assert.equal(params.get('album_ids'), '8,12');
  assert.equal(params.get('favorite'), '1');
  assert.equal(params.get('kind'), 'image');
  assert.equal(params.get('limit'), '64');
});

test('a late page response from the previous library is discarded', async () => {
  let resolve;
  const { context, frame } = harness(() => new Promise(r => { resolve = r; }));
  const pending = context.echoFrameFill(frame, frame.sources[0]);
  context.state.serverSettings.active_library_id = 'b';
  resolve({ photos: [{ uuid: 'stale' }], has_more: true, next_cursor: 'old' });
  await pending;
  assert.equal(frame.sources[0].items.length, 0);
});

test('hidden-page preload is retained and displayed once on return', async () => {
  const { context, frame } = harness();
  const prepared = { photo: { uuid: 'first' }, media: {} };
  frame.next = Promise.resolve(prepared);
  context.document.hidden = true;
  const shown = [];
  context.echoFrameShow = (f, item) => { shown.push(item.photo.uuid); f.current = item; };
  context.echoFramePrefetch = async () => null;
  await context.echoFrameAdvance(frame);
  assert.equal(frame.ready, prepared);
  assert.equal(frame.history.length, 0);
  context.document.hidden = false;
  await context.echoFrameAdvance(frame);
  assert.deepEqual(shown, ['first']);
  assert.equal(frame.history.length, 1);
});

test('ending stops on last media; loop resets cursor and avoids an immediate repeat', async () => {
  const { context, frame } = harness(async () => ({ photos: [{ uuid: 'one' }, { uuid: 'two' }] }));
  frame.order = 'asc';
  await context.echoFrameNextPhoto(frame);
  await context.echoFrameNextPhoto(frame);
  frame.cycleCount = 2;
  frame.current = { photo: { uuid: 'one' } };
  assert.equal(await context.echoFrameNextPhoto(frame), null);
  frame.p.end = 'loop';
  assert.equal((await context.echoFrameNextPhoto(frame)).uuid, 'two');
});

test('returning to an unloaded or filtered timeline item uses server-side location', () => {
  const appSource = fs.readFileSync(require('node:path').join(__dirname, '../web/static/app.js'), 'utf8');
  for (const [loaded, filter] of [[false, 'all'], [true, 'image']]) {
    const calls = [];
    const context = vm.createContext({
      state: { view: 'timeline', lightboxReturnView: 'timeline', lightboxPhotos: [{ id: 42, media_kind: 'video' }], lightboxIndex: 0, mediaKindFilter: filter },
      findPhotoThumb: () => loaded ? {} : null,
      mediaKindFilterStorageKey: 'kind', localStorage: { setItem() {} },
      resetTimelineToInitialPage: () => calls.push('reset'),
      showTimelineLocateBlocking: () => calls.push('blocking'),
      closeLightbox: () => calls.push('close'),
      switchView: view => calls.push(view),
    });
    for (const name of ['currentMediaKindParam', 'mediaKindMatchesFilter', 'openInTimeline', 'exitLightboxToContext']) {
      const match = appSource.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
      assert.ok(match);
      vm.runInContext(match[0], context);
    }
    context.exitLightboxToContext();
    assert.deepEqual(calls, ['reset', 'blocking', 'close', 'timeline']);
    assert.equal(context.state.pendingTimelinePhotoID, 42);
    assert.equal(context.state.mediaKindFilter, 'all');
  }
});

test('user exit returns to the current media and stops slideshow playback', async () => {
  for (const alreadyListed of [true, false]) {
    const { context, frame } = harness();
    const photo = { id: 42, uuid: 'playing' };
    frame.current = { photo };
    context.state.lightboxPhotos = alreadyListed ? [{ id: 42 }] : [{ id: 1 }];
    const calls = [];
    context.stopSlideshow = () => calls.push('stop');
    context.stopEchoFrame = () => vm.runInContext('echoFrame = null', context);
    context.$ = () => ({ classList: { contains: () => true } });
    context.lbGoTo = async index => {
      assert.equal(context.state.lightboxPhotos[index], photo);
      calls.push('render');
    };
    context.refreshLightboxUiActivity = () => calls.push('controls');
    context.focusLightboxKeyboardSurface = () => calls.push('focus');
    await context.exitEchoFrameToLightbox();
    assert.deepEqual(calls, ['stop', 'render', 'controls', 'focus']);
    assert.equal(context.state.lightboxPhotos.filter(item => item.id === 42).length, 1);
  }
});

test('display interval starts after the actual media transition finishes', async () => {
  const { context, frame } = harness();
  let finish;
  const animation = { transitionProperty: 'opacity', finished: new Promise(resolve => { finish = resolve; }) };
  const prepared = { photo: {}, media: { classList: { add() {} }, getAnimations: () => [animation] } };
  frame.surface.appendChild = () => {};
  frame.root.querySelector = () => null;
  frame.p.interval = 3000;
  context.echoFrameClock = () => {};
  context.echoFrameDateTransition = () => {};
  context.getComputedStyle = () => ({ opacity: '0' });
  const callbacks = [];
  const delays = [];
  context.requestAnimationFrame = callback => callbacks.push(callback);
  context.setTimeout = (callback, delay) => { delays.push(delay); return 1; };
  context.echoFrameShow(frame, prepared);
  context.echoFrameSchedule(frame);
  assert.deepEqual(delays, []);
  callbacks.shift()();
  context.echoFrameSchedule(frame);
  assert.deepEqual(delays, []);
  finish();
  await Promise.resolve();
  assert.deepEqual(delays, [3000]);
  assert.equal(context.normalizeEchoFrameSettings({ interval: 3000 }).interval, 3000);
});

test('text opacity preference has a safe default and clamps invalid values', () => {
  const { context } = harness();
  context.settingsPanelHeadIconMap = { echoFrame: '<svg></svg>' };
  context.renderSettingsPanelHeader = () => '';
  context.renderSettingsToggle = () => '';
  assert.equal(context.normalizeEchoFrameSettings({}).text_opacity, 60);
  assert.equal(context.normalizeEchoFrameSettings({ text_opacity: 83 }).text_opacity, 83);
  assert.equal(context.normalizeEchoFrameSettings({ text_opacity: 0 }).text_opacity, 60);
  assert.match(context.renderEchoFrameSettings(), /id="echo-frame-text-opacity-slider"[^>]*min="10" max="100"/);
});

test('EchoFrame treats touch tablets and narrow layouts as mobile for centered date layout', () => {
  const { context } = harness();
  context.isTouchLikeDevice = () => true;
  context.isMobileLayout = () => false;
  assert.equal(context.echoFrameMobileMode(), true);
  context.isTouchLikeDevice = () => false;
  context.isMobileLayout = () => true;
  assert.equal(context.echoFrameMobileMode(), true);
  context.isMobileLayout = () => false;
  assert.equal(context.echoFrameMobileMode(), false);
});

test('date follows the media timeline and changes only at its midpoint', () => {
  const { context, frame } = harness();
  const classes = new Set(['is-visible']);
  const events = [];
  const item = {
    parentNode: {},
    style: {},
    classList: { remove: name => { classes.delete(name); events.push('hide'); }, add: name => classes.add(name) },
    get offsetWidth() { assert.equal(classes.has('is-visible'), false); events.push('commit'); return 100; },
    set textContent(value) {
      assert.equal(classes.has('is-visible'), false);
      assert.equal(item.style.opacity, '0');
      events.push('write');
    },
  };
  const date = { dataset: {}, querySelector: () => item, removeAttribute() {} };
  const clock = { querySelector: () => ({}), setAttribute() {} };
  frame.root.querySelector = selector => selector === '.echo-frame-time' ? clock : date;
  frame.current = { photo: { taken_at: '2026-05-03T12:00:00Z' } };
  frame.p.date = true;
  const callbacks = [];
  context.cancelAnimationFrame = () => {};
  context.performance = { now: () => 999999 };
  context.getComputedStyle = () => ({ opacity: '1' });
  context.requestAnimationFrame = callback => callbacks.push(callback);
  const animation = { currentTime: 0, transitionProperty: 'opacity', effect: { getTiming: () => ({ duration: 8000, delay: 0 }) } };
  const media = { getAnimations: () => [animation] };
  context.echoFrameClock(frame, true);
  context.echoFrameDateTransition(frame, media);
  assert.deepEqual(events, []);
  context.echoFrameClock(frame);
  assert.deepEqual(events, []);
  animation.currentTime = 3500;
  callbacks.shift()();
  assert.ok(Math.abs(Number(item.style.opacity) - 0.5) < 0.001);
  assert.deepEqual(events, []);
  animation.currentTime = 4000;
  callbacks.shift()();
  assert.equal(item.style.opacity, '0');
  assert.deepEqual(events, ['hide', 'commit', 'write']);
  context.echoFrameClock(frame);
  assert.deepEqual(events, ['hide', 'commit', 'write']);
  animation.currentTime = 4500;
  callbacks.shift()();
  assert.ok(Math.abs(Number(item.style.opacity) - 0.5) < 0.001);
  animation.currentTime = 5000;
  callbacks.shift()();
  context.echoFrameClock(frame);
  assert.equal(classes.has('is-visible'), true);
});
