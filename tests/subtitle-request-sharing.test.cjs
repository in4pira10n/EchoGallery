const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname, '../web/static/app.js'), 'utf8');
function harness() {
  const requests = [], badges = [];
  const context = vm.createContext({
    state: { serverSettings: { id: 'a' } }, activeLibraryIdentity: settings => settings.id,
    isVideoMedia: photo => photo.media_kind === 'video',
    api: { get: url => new Promise((resolve, reject) => requests.push({ url, resolve, reject })) },
  });
  vm.runInContext(source.slice(source.indexOf('let subtitleMatchCache ='), source.indexOf('\nlet danmakuMatchCache')), context);
  context.syncSubtitleThumbBadges = id => badges.push(id);
  return { context, requests, badges };
}
test('card and lightbox share pending subtitle discovery and its successful result', async () => {
  const h = harness();
  const photo = { id: 1, media_kind: 'video' };
  const card = h.context.probeSubtitleMatch(photo);
  const lightbox = h.context.loadVideoSubtitleTracks(photo);
  assert.equal(h.requests.length, 1);
  h.requests[0].resolve({ tracks: [{ index: 0 }] });
  await card;
  assert.equal((await lightbox)[0].index, 0);
  assert.equal((await h.context.loadVideoSubtitleTracks(photo)).length, 1);
  assert.equal(h.requests.length, 1);
  assert.deepEqual(h.badges, [1]);
});
test('failed subtitle discovery retries and old library results do not update badges', async () => {
  const h = harness();
  const photo = { id: 1, media_kind: 'video' };
  const first = h.context.probeSubtitleMatch(photo);
  h.requests[0].reject(new Error('offline'));
  await assert.rejects(first, /offline/);
  const old = h.context.loadVideoSubtitleTracks(photo);
  h.context.state.serverSettings.id = 'b';
  const current = h.context.loadVideoSubtitleTracks(photo);
  h.requests[1].resolve({ tracks: [{ index: 0 }] });
  await old;
  assert.deepEqual(h.badges, []);
  h.requests[2].resolve({ tracks: [] });
  await current;
  assert.equal(h.context.hasSubtitleMatch(photo), false);
  assert.deepEqual(h.badges, [1]);
});
