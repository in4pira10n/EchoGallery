const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
test('discovery renders measured counts, operation, wait and escaped path in both locales', () => {
  const code = source.slice(source.indexOf('function renderLibraryScanDiscovery('), source.indexOf('\nfunction libraryBuildHeading('));
  for (const locale of ['en-US', 'zh-CN']) {
    const strings = JSON.parse(fs.readFileSync(path.join(__dirname, `../web/static/strings/${locale}.json`), 'utf8'));
    const keys = [];
    const ctx = vm.createContext({Date, copyText: (key, values = {}) => {
      keys.push(key);
      let value = key.split('.').reduce((value, part) => value?.[part], strings);
      assert.equal(typeof value, 'string', `missing ${locale} ${key}`);
      return value.replace(/\{(\w+)\}/g, (_, name) => values[name] ?? '');
    }, escapeHTML: value => String(value).replaceAll('<', '&lt;').replaceAll('>', '&gt;')});
    vm.runInContext(code, ctx);
    const html = ctx.renderLibraryScanDiscovery({phase: 'discovering', operation: 'read_directory', directories: 12, pending_directories: 7, media: 884, operation_started_at: new Date(Date.now()-10000).toISOString(), path: '//NAS/<Pictures>'});
    assert.match(html, /12/); assert.match(html, /884/); assert.match(html, /&lt;Pictures&gt;/);
    assert.ok(keys.includes('app.libraryBuild.operationWait'));
    assert.equal(ctx.renderLibraryScanDiscovery(null), '');
  }
});
test('failed status polling keeps retrying while the scan view blocks', async () => {
  const code = source.slice(source.indexOf('async function refreshLibraryBuildStatus('), source.indexOf('\nfunction startLibraryBuildPolling('));
  const timers = [];
  const ctx = vm.createContext({state: {libraryBuildStatus:{status:'discovering'}}, isLibraryBuildBlocking: () => true,
    fetchLibraryBuildStatus: async () => {throw new Error('network');}, isForbiddenError: () => false,
    clearTimeout: () => {}, setTimeout: (_,ms) => timers.push(ms), console:{error:()=>{}}});
  vm.runInContext(code,ctx); await ctx.refreshLibraryBuildStatus(); assert.deepEqual(timers,[2000]);
});
