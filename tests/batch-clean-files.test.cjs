const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
test('batch cleanup is opt-in in the API payload', async () => {
  const calls = [];
  const context = vm.createContext({ api: { post: async (url, body) => calls.push({ url, body }) } });
  const start = source.indexOf('async function startLibraryBatchBuild(');
  vm.runInContext(source.slice(start, source.indexOf('\nasync function ', start + 1)), context);
  await context.startLibraryBatchBuild();
  await context.startLibraryBatchBuild({ cleanFiles: true });
  assert.equal(calls[0].body.clean_files, false);
  assert.equal(calls[1].body.clean_files, true);
  assert.equal(calls[1].url, '/api/settings/libraries/build-all');
  assert.equal(calls[1].body.clean_thumbnail_files, undefined);
});

test('cleanup UI starts disabled and its descriptions are translated', () => {
  assert.match(source, /libraryBatchWorkflowCleanFiles: false/);
  assert.match(source, /renderSettingsToggle\('settings-library-batch-workflow-clean-files', settingsText\('batchWorkflow.cleanFiles'\)/);
  for (const locale of ['en-US', 'zh-CN']) {
    const catalog = JSON.parse(fs.readFileSync(path.join(__dirname, `../web/static/strings/${locale}.json`), 'utf8'));
    for (const key of ['cleanFiles', 'cleanFilesCopy', 'taskCleanup']) {
      assert.equal(typeof catalog.app.settings.batchWorkflow[key], 'string');
    }
    assert.match(catalog.app.settings.batchWorkflow.taskCleanup, /\{name\}/);
    assert.equal(typeof catalog.app.libraryBuild.scanPhase.cleanup, 'string');
  }
});
