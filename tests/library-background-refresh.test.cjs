const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
const code = source.slice(source.indexOf('async function refreshLibraryAfterInitialView('), source.indexOf('\nfunction bindNav()'));
function harness(result = {}) {
  const calls = [];
  const photos = [{ id: 1 }];
  const state = { view: 'timeline', photos, albumPhotos: [], albums: [], serverSettings: { id: 'one' }, timelineLoaded: true, timelineCursor: 'cursor', lightboxPhotos: photos, lightboxOpen: false };
  const context = vm.createContext({
    state, activeLibraryIdentity: settings => settings.id,
    requestAnimationFrame: callback => callback(), setTimeout: callback => callback(),
    api: { post: async url => { calls.push(url); return result; } },
    logLibrarySwitch: phase => calls.push(phase),
    finishLibrarySwitchDiagnostic: (phase, fields) => calls.push({ phase, fields }),
    renderView: async () => calls.push('render'), console,
  });
  vm.runInContext(code, context);
  return { context, calls, state, photos };
}
test('background disk check waits for first view and unchanged results do not touch the wall', async () => {
  const h = harness({ changed: false, imported: 0, pruned: 0, skipped: 10000 });
  let ready;
  const firstView = new Promise(resolve => { ready = resolve; });
  const task = h.context.refreshLibraryAfterInitialView(firstView);
  await Promise.resolve();
  assert.deepEqual(h.calls, []);
  ready();
  await task;
  assert.equal(h.calls.includes('/api/library-scan/refresh'), true);
  assert.equal(h.calls.includes('render'), false);
  assert.equal(h.state.timelineLoaded, true);
  assert.equal(h.state.photos, h.photos);
  assert.equal(h.state.timelineCursor, 'cursor');
});
test('changed scan invalidates future views without disturbing the current wall or lightbox', async () => {
  const h = harness({ changed: true, imported: 1, pruned: 0 });
  h.state.lightboxOpen = true;
  await h.context.refreshLibraryAfterInitialView(Promise.resolve());
  assert.equal(h.state.timelineLoaded, false);
  assert.equal(h.state.photos, h.photos);
  assert.equal(h.state.lightboxPhotos, h.photos);
  assert.equal(h.state.timelineCursor, 'cursor');
  assert.equal(h.calls.includes('render'), false);
});
test('new media refreshes an empty wall and switched libraries ignore obsolete checks', async () => {
  const h = harness({ changed: true, imported: 1 });
  h.state.photos = [];
  await h.context.refreshLibraryAfterInitialView(Promise.resolve());
  assert.equal(h.calls.includes('render'), true);
  const other = harness();
  let ready;
  const task = other.context.refreshLibraryAfterInitialView(new Promise(resolve => { ready = resolve; }));
  other.state.serverSettings.id = 'two';
  ready();
  await task;
  assert.deepEqual(other.calls, []);
});
