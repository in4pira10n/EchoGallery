const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');
const root = path.join(__dirname, '..');
const source = fs.readFileSync(path.join(root, 'web/static/app.js'), 'utf8');
const en = JSON.parse(fs.readFileSync(path.join(root,'web/static/strings/en-US.json'),'utf8')).app.settings.update;
const zh = JSON.parse(fs.readFileSync(path.join(root,'web/static/strings/zh-CN.json'),'utf8')).app.settings.update;
function fixture(extra = {}) {
  const context = vm.createContext({
    settingsText: key => en[key.replace(/^update\./,'')] || key,
    copyText: key => key,
    escapeHTML: text => String(text || '').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;'),
    canRunServerTasks: () => true, icons: {photo:'photo',refresh:'refresh',close:'close'},
    withPageSessionURL: url => url, $: () => null, document: {querySelectorAll: () => []},
    state:{serverSettings:{}}, showToast: () => {}, console:{warn:()=>{}}, ...extra,
  });
  vm.runInContext(source.slice(source.indexOf('const localUpdateUI ='), source.indexOf('function normalizePathForCompare(')),context);
  return { context, run: code => vm.runInContext(code, context) };
}

test('update panel uses localized states, real library avatars and escapes identity data', () => {
  const {run} = fixture();
  run('localUpdateUI.config = {distribute:["D:/Apps"],include:[],exclude:[],update_package_dir:"D:/Updates"}; localUpdateUI.targets = [{path:"D:/Apps/EchoGallery.exe",name:"<img src=x>",avatar_url:"/api/settings/local-update/avatar?id=one"}]');
  const html = run('renderLocalUpdatePanel()');
  assert.ok(html.includes('&lt;img src=x>'));
  assert.ok(html.includes('/api/settings/local-update/avatar?id=one'));
  assert.ok(html.includes(en.unknownVersion));
  assert.ok(html.includes(en.updateAll));
  assert.ok(!html.includes('undefined'));
  run('localUpdateUI.source = "github"');
  const github = run('renderLocalUpdatePanel()');
  assert.ok(github.includes(en.githubUnavailable));
  assert.ok(!github.includes('id="settings-run-update-btn"'));
});

test('all update panel keys exist in both catalogs', () => {
  const section = source.slice(source.indexOf('const localUpdateUI ='), source.indexOf('function normalizePathForCompare('));
  const keys = new Set([...section.matchAll(/\bt\('([^']+)'\)|settingsText\('update\.([^']+)'/g)].map(m=>m[1]||m[2]));
  for (const key of keys) {
    assert.equal(typeof en[key], 'string', `English ${key}`);
    assert.equal(typeof zh[key], 'string', `Chinese ${key}`);
  }
});

test('editing config invalidates the checked snapshot', () => {
  const {run} = fixture();
  run('localUpdateUI.check = {check_id:"old",has_update:true}; invalidateLocalUpdate()');
  assert.equal(run('localUpdateUI.check'), null);
  assert.equal(run('localUpdateUI.generation'), 1);
});

test('confirmation cannot apply a snapshot that changed while dialog was open', async () => {
  let accept;
  let applied = 0;
  const {run} = fixture({appConfirm: () => new Promise(resolve=>{accept=resolve}),api:{post:async()=>{applied++}}});
  run('localUpdateUI.check = {check_id:"old",has_update:true}');
  const pending = run('runCheckedLocalUpdate()');
  run('invalidateLocalUpdate()');
  accept(true);
  await pending;
  assert.equal(applied,0);
});

test('a single-row update sends only that checked target', async () => {
  let request;
  const {run} = fixture({appConfirm:async()=>true,api:{post:async(url,body)=>{request={url,body};return {restarting:true}}}});
  run('localUpdateUI.check = {check_id:"checked",has_update:true}');
  await run('runCheckedLocalUpdate(["D:/Apps/EchoGallery.exe"])');
  assert.equal(request.body.check_id,'checked');
  assert.deepEqual(Array.from(request.body.targets),['D:/Apps/EchoGallery.exe']);
  assert.equal(run('localUpdateUI.check'),null);
});


test('settings mounts the update panel while the debug workbench keeps its own body', () => {
  const start = source.indexOf('function renderRootDebugWorkbenchPanel(');
  const end = source.indexOf('function renderRootDebugPreviewBlock(', start);
  const debug = source.slice(start, end);
  const context = vm.createContext({renderSettingsPanelHeader: () => '<header></header>'});
  vm.runInContext(debug, context);
  assert.ok(vm.runInContext('renderRootDebugWorkbenchPanel({body:"debug-body"})', context).includes('debug-body'));
  assert.ok(!debug.includes('renderLocalUpdatePanel'));
  const header = source.indexOf("${renderSettingsPanelHeader(settingsText('sections.update.title')");
  const card = source.slice(header, source.indexOf('</section>', header));
  assert.ok(card.includes('id="settings-local-update-panel">${renderLocalUpdatePanel()}'));
  assert.ok(!card.includes('settings-update-local-enabled'));
  assert.equal((source.match(/id="settings-local-update-panel"/g) || []).length, 1);
});

 test('version name and build code come from the program and render separately', () => {
  const {run} = fixture();
  run('localUpdateUI.releaseName = "EchoGallery v3.0.0 : Nice to Meet You"; localUpdateUI.versionCode = "v3.0.0build202610211158"');
  const html = run('renderLocalUpdatePanel()');
  assert.ok(html.includes(en.versionName));
  assert.ok(html.includes(en.versionCode));
  assert.ok(html.includes('EchoGallery v3.0.0 : Nice to Meet You'));
  assert.ok(html.includes('v3.0.0build202610211158'));
  assert.ok(!source.includes('/api/settings/local-update/release'));
 });
