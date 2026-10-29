const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

function load(context, ...names) {
  for (const name of names) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, 'missing function: ' + name);
    vm.runInContext(match[0], context);
  }
}

test('native video output uses the same volume path on touch and desktop devices', async () => {
  for (const touch of [false, true]) {
    const video = { volume: 1, muted: false, classList: { contains: () => false }, removeAttribute() {} };
    const context = vm.createContext({
      state: { globalVideoVolume: 1, videoVolumeMinPercent: 0, videoVolumeMaxPercent: 100 },
      $: () => video,
      isTouchLikeDevice: () => touch,
    });
    load(context, 'normalizeVideoVolumeValue', 'normalizeVideoVolumePercent', 'effectiveVideoVolumeBounds',
      'clampVideoVolumeToBounds', 'effectiveVideoVolume', 'applyVideoVolumeOutput', 'restoreVideoAudioOutput');
    context.applyVideoVolumeOutput(video, 0.4);
    assert.equal(video.volume, 0.4);
    assert.equal(context.effectiveVideoVolume(video), 0.4);
    await context.restoreVideoAudioOutput(video);
    assert.equal(video.volume, 0.4);
  }
  assert.doesNotMatch(source, /AudioContext|createMediaElementSource|createGain|lightboxVideoGainNode/);
});

test('a browser that ignores programmatic volume does not report a simulated gain value', () => {
  const video = { get volume() { return 1; }, set volume(value) {} };
  const context = vm.createContext({
    state: { globalVideoVolume: 1, videoVolumeMinPercent: 0, videoVolumeMaxPercent: 100 },
    $: () => video,
  });
  load(context, 'normalizeVideoVolumeValue', 'normalizeVideoVolumePercent', 'effectiveVideoVolumeBounds',
    'clampVideoVolumeToBounds', 'effectiveVideoVolume', 'applyVideoVolumeOutput');
  context.applyVideoVolumeOutput(video, 0.2);
  assert.equal(context.effectiveVideoVolume(video), 1);
});

test('desktop volume shortcuts retain their native percentage steps', () => {
  const video = { volume: 0.5, classList: { contains: () => false } };
  let output;
  const context = vm.createContext({
    $: () => video,
    effectiveVideoVolume: () => video.volume,
    setVideoVolumeValue: value => { output = value; },
  });
  load(context, 'adjustVideoVolume');
  assert.equal(context.adjustVideoVolume(5), true);
  assert.equal(output, 0.55);
});

test('upward swipes do not change volume and downward swipes dismiss across the video', () => {
  let seek;
  let dismissed = false;
  const video = { duration: 100, classList: { contains: () => false } };
  const context = vm.createContext({
    state: {},
    $: () => video,
    seekVideoToTime: (element, value) => { seek = value; },
    clampLightboxValue: (value, min, max) => Math.max(min, Math.min(max, value)),
    setLightboxDismissGesture: () => { dismissed = true; },
    setLightboxUnderzoomProgress: () => {},
  });
  load(context, 'updateVideoSurfaceSwipe');
  const swipe = () => ({ pointerId: 1, startX: 0, startY: 0, startTime: 10, width: 300, height: 600 });
  const event = (x, y) => ({ pointerId: 1, clientX: x, clientY: y, preventDefault() {}, stopPropagation() {} });
  context.state.videoSurfaceSwipe = swipe();
  assert.equal(context.updateVideoSurfaceSwipe(event(0, -100)), false);
  assert.equal(context.state.videoSurfaceSwipe.dragging, undefined);
  context.state.videoSurfaceSwipe = swipe();
  assert.equal(context.updateVideoSurfaceSwipe(event(100, 0)), true);
  assert.equal(context.state.videoSurfaceSwipe.mode, 'seek');
  assert.ok(seek > 10);
  for (const [x, y] of [[5, 5], [150, 300], [295, 490]]) {
    dismissed = false;
    context.state.videoSurfaceSwipe = { ...swipe(), startX: x, startY: y };
    assert.equal(context.updateVideoSurfaceSwipe(event(x, y + 100)), true);
    assert.equal(context.state.videoSurfaceSwipe.mode, 'dismiss');
    assert.equal(dismissed, true);
  }
});
