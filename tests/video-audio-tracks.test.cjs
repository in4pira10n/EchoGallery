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
function harness(tracks = [{ index: 0, id: '1', default: true, language: 'zh' }, { index: 1, id: '2', label: 'English' }]) {
  const photo = { id: 7, uuid: 'video-7', media_kind: 'video' };
  const video = new EventTarget();
  Object.assign(video, { readyState: 2, currentTime: 42, playbackRate: 1.5, paused: false, muted: false, volume: 0.6,
    duration: 100, dataset: {}, src: '/media/playback/original', classList: { contains: () => false },
    load() {
      this.currentTime = 0; this.playbackRate = 1; this.paused = true;
      queueMicrotask(() => {
        if (this.failNext && this.src.includes('soundtrack=')) {
          this.failNext = false; this.dispatchEvent(new Event('error')); return;
        }
        this.dispatchEvent(new Event('loadedmetadata'));
        this.dispatchEvent(new Event('loadeddata'));
      });
    },
    play() { this.paused = false; return Promise.resolve(); },
  });
  const saved = [], messages = [], posts = [];
  const state = { lightboxPhotos: [photo], lightboxIndex: 0, videoPlaybackPreferences: { '7': {} } };
  const context = vm.createContext({ state, AbortController, DOMException, setTimeout, clearTimeout,
    $: selector => selector === '#lb-video' ? video : { classList: { contains: () => true } },
    activeLibraryIdentity: () => 'library-a',
    loadVideoPlaybackPreference: async () => state.videoPlaybackPreferences['7'],
    videoPlaybackPreferenceKey: item => String(item.id),
    queueVideoPlaybackPreferenceSave: (item, patch) => saved.push({ photo: item, patch }),
    copyText: (key, params) => ({ key, params }),
    showToast: message => messages.push(message),
    withPageSessionURL: url => url,
    mediaPlaybackURL: () => '/media/playback/original',
    shouldAutoplayVideoInLightbox: () => false,
    api: {
      get: async () => ({ tracks }),
      post: async (url, body, options) => {
        posts.push({ url, body, signal: options.signal });
        return { track: tracks[body.index], url: '/media/playback/video-7?soundtrack=' + body.index };
      },
    },
  });
  load(context, 'normalizeVideoAudioTrack', 'cancelVideoSoundtrackRequest', 'isCurrentSoundtrackRequest',
    'videoSoundtrackErrorText', 'loadLightboxVideoSource', 'loadVideoSoundtrackSource', 'switchLightboxAudioTrack');
  return { context, photo, video, state, saved, messages, posts };
}

test('switches cached audio without native track API and preserves playback state', async () => {
  const h = harness();
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.saved[0].patch.audio_track.index, 1);
  assert.equal(h.state.videoPlaybackPreferences['7'].audioTrack.index, 1);
  assert.equal(h.messages.at(-1).params.track, 'English');
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.saved[1].patch.audio_track.index, 0);
  assert.equal(h.video.currentTime, 42);
  assert.equal(h.video.playbackRate, 1.5);
  assert.equal(h.video.paused, false);
  assert.equal(h.video.muted, false);
  assert.equal(h.video.volume, 0.6);
  assert.equal(h.video.src, '/media/playback/video-7?soundtrack=0');
  assert.equal(h.video.dataset.soundtrackReplacing, undefined);
  assert.doesNotMatch(source, /video\.audioTracks|selectNativeVideoAudioTrack|restoreVideoAudioTrack/);
});

test('paused and muted video remains paused and muted after a switch', async () => {
  const h = harness(); h.video.paused = true; h.video.muted = true;
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.video.paused, true); assert.equal(h.video.muted, true);
});

test('zero or single tracks and missing FFmpeg have localized feedback', async () => {
  for (const [tracks, key] of [[[], 'audioTrackNone'], [[{ index: 0 }], 'audioTrackSingle']]) {
    const h = harness(tracks);
    await h.context.switchLightboxAudioTrack(h.photo);
    assert.equal(h.messages[0].key, 'app.lightbox.' + key);
    assert.equal(h.posts.length, 0); assert.equal(h.saved.length, 0);
  }
  const h = harness();
  h.context.api.get = async () => { throw { error_key: 'app.lightbox.audioTrackToolsRequired' }; };
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.messages.at(-1).key, 'app.lightbox.audioTrackToolsRequired');
});

test('conversion failure keeps original playback and does not save a false choice', async () => {
  const h = harness();
  h.context.api.post = async () => { throw new Error('ffmpeg failed'); };
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.video.src, '/media/playback/original');
  assert.equal(h.video.currentTime, 42); assert.equal(h.saved.length, 0);
  assert.equal(h.messages.at(-1).key, 'app.lightbox.audioTrackFailed');
});

test('decode failure restores original source and position without saving', async () => {
  const h = harness(); h.video.failNext = true;
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.video.src, '/media/playback/original');
  assert.equal(h.video.currentTime, 42); assert.equal(h.video.paused, false);
  assert.equal(h.saved.length, 0); assert.equal(h.video.dataset.soundtrackIndex, undefined);
});

test('playback continues during conversion and switches at the latest position', async () => {
  const h = harness(); const post = h.context.api.post;
  h.context.api.post = async (...args) => { h.video.currentTime = 57; return post(...args); };
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.video.currentTime, 57);
});

test('saved track zero is prepared before initial playback', async () => {
  const h = harness();
  h.state.videoPlaybackPreferences['7'].audioTrack = { index: 0, id: '1' };
  await h.context.loadLightboxVideoSource(h.video, h.photo);
  assert.equal(h.posts[0].body.index, 0);
  assert.equal(h.video.src, '/media/playback/video-7?soundtrack=0');
  assert.equal(h.video.dataset.soundtrackIndex, '0'); assert.equal(h.saved.length, 0);
});

test('missing saved cache can be regenerated; unavailable conversion falls back with feedback', async () => {
  const h = harness();
  h.state.videoPlaybackPreferences['7'].audioTrack = { index: 1 };
  h.context.api.post = async () => { throw new Error('ffmpeg unavailable'); };
  await h.context.loadLightboxVideoSource(h.video, h.photo);
  assert.equal(h.video.src, '/media/playback/original');
  assert.equal(h.messages[0].key, 'app.lightbox.audioTrackRestoreFailed');
  assert.equal(h.state.videoPlaybackPreferences['7'].audioTrack.index, 1);
});

test('library changes during conversion cannot change another library playback', async () => {
  const h = harness(); let library = 'first';
  h.context.activeLibraryIdentity = () => library;
  const post = h.context.api.post;
  h.context.api.post = async (...args) => { library = 'second'; return post(...args); };
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.video.src, '/media/playback/original'); assert.equal(h.saved.length, 0);
});

test('navigation aborts cache requests and rapid clicks do not start duplicate builds', async () => {
  const h = harness(); let finish;
  h.context.api.get = () => new Promise(resolve => { finish = resolve; });
  const pending = h.context.switchLightboxAudioTrack(h.photo);
  await Promise.resolve();
  await h.context.switchLightboxAudioTrack(h.photo);
  assert.equal(h.messages.at(-1).key, 'app.lightbox.audioTrackLoading');
  const signal = h.video._soundtrackRequest.controller.signal;
  h.context.cancelVideoSoundtrackRequest(h.video);
  finish({ tracks: [] }); await pending;
  assert.equal(signal.aborted, true); assert.equal(h.posts.length, 0); assert.equal(h.saved.length, 0);
});

test('cloning and normalizing playback preferences retains audio-track index zero', () => {
  const h = harness([]);
  h.context.normalizeVideoBookmarkState = () => ({ bookmarks: [], resumeTime: 0 });
  h.context.clampVideoVolumeToBounds = value => value;
  load(h.context, 'cloneVideoPlaybackBookmarks', 'normalizeVideoPlaybackPreference', 'cloneVideoPlaybackPreference');
  const clone = h.context.cloneVideoPlaybackPreference({ audio_track: { index: 0, id: 'track-0' }, volume: 0.5 });
  assert.equal(clone.audioTrack.index, 0);
  assert.equal(clone.audioTrack.id, 'track-0');
  assert.equal(h.context.cloneVideoPlaybackPreference(clone).audioTrack.id, 'track-0');
});

test('audio selection is included in the existing playback save queue and flush payload', () => {
  const h = harness([]);
  h.state.videoPlaybackPreferenceSaveQueue = {};
  h.state.videoPlaybackPreferenceSaveTimers = {};
  let flushPayload;
  h.context.setTimeout = () => 1;
  h.context.clearTimeout = () => {};
  h.context.buildPageSessionHeaders = value => value;
  h.context.fetchWithPageSession = (url, options) => { flushPayload = JSON.parse(options.body); return Promise.resolve({ ok: false }); };
  load(h.context, 'queueVideoPlaybackPreferenceSave', 'flushPendingVideoPlaybackPreference');
  h.context.queueVideoPlaybackPreferenceSave(h.photo, { audio_track: { index: 0, id: 'a' } });
  h.context.flushPendingVideoPlaybackPreference(h.photo, { keepalive: true });
  assert.equal(flushPayload.audio_track.index, 0);
  assert.equal(flushPayload.audio_track.id, 'a');
});
