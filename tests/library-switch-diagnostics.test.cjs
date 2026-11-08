const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname, '../web/static/app.js'), 'utf8');
const start = source.indexOf('// Library switch diagnostics are disabled');
const end = source.indexOf('\nfunction authRedirectReason', start);
function harness(fetch = async () => ({ status: 200 })) {
  const logs = [], removed = [];
  const context = vm.createContext({
    sessionStorage: { removeItem: key => removed.push(key) },
    console: { info: message => logs.push(message) }, fetch, withPageSessionRequest: options => options,
  });
  vm.runInContext(source.slice(start, end), context);
  return { context, logs, removed };
}
test('library switching emits no diagnostics and clears an old persisted trace', async () => {
  const h = harness();
  h.context.beginLibrarySwitchDiagnostic('old', 'new');
  h.context.logLibrarySwitch('settings-loaded');
  assert.equal((await h.context.fetchWithPageSession('/api/settings')).status, 200);
  h.context.finishLibrarySwitchDiagnostic('initial-refresh-complete');
  assert.deepEqual(h.logs, []);
  assert.deepEqual(h.removed, ['eg-library-switch-diagnostic']);
});
test('quiet request wrapper preserves network errors', async () => {
  const error = new Error('offline');
  const h = harness(async () => { throw error; });
  await assert.rejects(h.context.fetchWithPageSession('/api/settings'), error);
  assert.deepEqual(h.logs, []);
});
