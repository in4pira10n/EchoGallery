const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
function section(start, end) {
  return source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)));
}
function harness() {
  const calls = [];
  const state = { settingsReady: true, shareLinksLoaded: false, shareLinksLoading: false };
  const context = vm.createContext({
    state, settingsPanelExpandedState: new Map(),
    api: { get: async url => { calls.push(url); return [{ id: 1 }]; } },
    canManageShareLinks: () => true, canRunServerTasks: () => true,
    canAccessBatchWorkflow: () => true,
    setShareLinks: links => { state.shareLinks = links; },
    refreshShareManagementUI: () => {},
    pollEXIFBackfillStatus: async () => { calls.push('exif'); },
    pollVideoThumbnailRefreshStatus: async () => { calls.push('video-thumbnails'); },
    pollThumbnailBuildStatus: async () => { calls.push('thumbnails'); },
    pollLibraryBatchBuildStatus: async () => { calls.push('batch-scan'); },
    pollLibraryBatchThumbnailBuildStatus: async () => { calls.push('batch-thumbnails'); },
    startLibraryBuildPolling: () => { calls.push('library-build'); },
  });
  vm.runInContext(section('async function loadShareMap()', '\nfunction disconnectLoadMoreObserver'), context);
  vm.runInContext(section('let settingsWorkflowStatusLoad =', '\nfunction refreshPhotoShareIndicators'), context);
  vm.runInContext(section('function bindSettingsPanelToggles(', '\nfunction persistLibraryBatchWorkflowPrefs'), context);
  vm.runInContext(section('function startRoleAwareBackgroundPolling()', '\nfunction renderLibraryScanDiscovery'), context);
  function panel(name) {
    const classes = new Set(['settings-panel', name]);
    classes.contains = classes.has.bind(classes);
    classes.toggle = (key, enabled) => enabled ? classes.add(key) : classes.delete(key);
    const attrs = new Map();
    const element = { classList: classes };
    const header = {
      closest: () => element, querySelector: () => ({ textContent: name }),
      setAttribute: (key, value) => attrs.set(key, value), getAttribute: key => attrs.get(key),
      addEventListener: (event, fn) => { header[event] = fn; },
      parentElement: { children: [] },
    };
    header.parentElement.children = [header];
    context.bindSettingsPanelToggles({ querySelectorAll: () => [header] });
    return header;
  }
  return { calls, state, context, panel };
}
const settle = () => new Promise(resolve => setImmediate(resolve));
test('share data loads on expansion only, once, and closing makes no requests', async () => {
  const h = harness();
  const header = h.panel('settings-panel-share');
  assert.deepEqual(h.calls, []);
  header.click();
  await settle();
  assert.deepEqual(h.calls, ['/api/media/shares']);
  assert.equal(h.state.shareLinks[0].id, 1);
  header.click();
  header.click();
  await settle();
  assert.deepEqual(h.calls, ['/api/media/shares']);
});
test('background startup does not restore workflow tasks; expansion restores all five statuses', async () => {
  const h = harness();
  h.context.startRoleAwareBackgroundPolling();
  assert.deepEqual(h.calls, ['library-build']);
  const header = h.panel('settings-panel-batch-workflow');
  assert.equal(h.calls.length, 1);
  header.click();
  header.click();
  header.click(); // reopening while the restoration is in flight must not duplicate requests
  await settle();
  assert.deepEqual(h.calls, ['library-build', 'exif', 'video-thumbnails', 'thumbnails', 'batch-scan', 'batch-thumbnails']);
});
test('photo walls and hidden settings mounting cannot fetch the full share list or restore statuses', () => {
  assert.equal((source.match(/loadShareMap\(\)/g) || []).length, 2); // definition and explicit expansion
  const renderSettings = section('async function renderSettings()', '\nfunction renderLibrarySettingsRows');
  const renderContent = section('function renderSettingsContent(', '// ── 时间线视图');
  for (const code of [renderSettings, renderContent]) {
    assert.doesNotMatch(code, /void poll(?:EXIFBackfill|VideoThumbnailRefresh|ThumbnailBuild)Status\(\)/);
    assert.doesNotMatch(code, /observeDeferredSettingsShareLoad/);
  }
});
test('library availability and metrics are fetched from the settings-only endpoint in the background', () => {
  const renderSettings = section('async function renderSettings()', '\nfunction renderLibrarySettingsRows');
  assert.match(renderSettings, /void refreshSettingsAvailability\(\)/);
  assert.doesNotMatch(renderSettings, /await refreshSettingsAvailability\(\)/);
  assert.match(source, /api\.get\('\/api\/settings\/libraries'/);
  assert.match(source, /state\.view = 'timeline';\s*setHashView\('timeline'\);\s*location\.reload\(\)/);
});
