const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

test('portable browser settings use an explicit whitelist', () => {
  const values = new Map([
    ['pwa', '{"name":"Gallery"}'],
    ['filter', 'video'],
    ['session', 'secret-session'],
  ]);
  const context = vm.createContext({
    pwaSettingsStorageKey: 'pwa', mediaKindFilterStorageKey: 'filter', timelineOrderStorageKey: 'timeline',
    albumDetailSortStorageKey: 'album', globalVideoVolumeStorageKey: 'volume',
    libraryBatchWorkflowEnabledStorageKey: 'batch-enabled', libraryBatchWorkflowAggressiveStorageKey: 'batch-fast',
    localStorage: {
      getItem: key => values.has(key) ? values.get(key) : null,
      setItem: (key, value) => values.set(key, value),
    },
  });
  const declaration = source.match(/^const portableBrowserSettingKeys = \[[^]*?^];/m);
  assert.ok(declaration);
  vm.runInContext(declaration[0], context);
  for (const name of ['portableBrowserSettings', 'applyPortableBrowserSettings']) {
    const match = source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match);
    vm.runInContext(match[0], context);
  }
  const exported = context.portableBrowserSettings();
  assert.equal(exported.pwa, '{"name":"Gallery"}');
  assert.equal(exported.filter, 'video');
  assert.equal(exported.session, undefined);
  context.applyPortableBrowserSettings({ pwa: '{"name":"New"}', session: 'hijack', unknown: 'x' });
  assert.equal(values.get('pwa'), '{"name":"New"}');
  assert.equal(values.get('session'), 'secret-session');
  assert.equal(values.has('unknown'), false);
});

test('settings expose transfer controls and no legacy cache claims', () => {
  assert.match(source, /id="settings-export-config-btn"/);
  assert.match(source, /id="settings-import-config-btn"/);
  assert.match(source, /资源库路径会按 ID 自动映射/);
  assert.doesNotMatch(source, /运行时锁.*配置包/);
});
