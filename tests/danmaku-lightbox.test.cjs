const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

function functionSource(name) {
  const match = source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
  assert.ok(match, 'missing function: ' + name);
  return match[0];
}

test('lightbox uploader avatars upgrade legacy HTTP URLs and load immediately', () => {
  const context = vm.createContext({
    URL,
    window: { location: { href: 'https://gallery.example/' } },
    icons: { albumCardUser: '<svg>user</svg>' },
  });
  for (const name of ['escapeHTML', 'safeDanmakuAvatarURL', 'lightboxInfoIconMarkup']) {
    vm.runInContext(functionSource(name), context);
  }

  assert.equal(context.safeDanmakuAvatarURL('http://i0.hdslb.com/bfs/face/avatar.jpg'), 'https://i0.hdslb.com/bfs/face/avatar.jpg');
  assert.equal(context.safeDanmakuAvatarURL('//i0.hdslb.com/bfs/face/avatar.jpg'), 'https://i0.hdslb.com/bfs/face/avatar.jpg');
  assert.equal(context.safeDanmakuAvatarURL('javascript:alert(1)'), '');

  const markup = context.lightboxInfoIconMarkup({ avatar: 'https://i0.hdslb.com/bfs/face/avatar.jpg' });
  assert.match(markup, /class="lb-info-avatar"[^>]*loading="eager"/);
  assert.match(markup, /src="https:\/\/i0\.hdslb\.com\/bfs\/face\/avatar\.jpg"/);
});

test('settings and keyboard share danmaku visibility across media changes', () => {
  let onChange;
  const input = { checked: true, addEventListener: (type, listener) => { if (type === 'change') onChange = listener; } };
  const context = vm.createContext({
    window: {},
    $: selector => selector === '#settings-danmaku-enabled' ? input : null,
    copyText: key => key,
    showToast: () => {},
    observeDanmakuSteppedRanges: () => {},
    updateRangeProgress: () => {},
    requestAnimationFrame: callback => callback(),
    $$: () => [],
    document: { querySelectorAll: () => [] },
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../web/static/danmaku.js'), 'utf8'), context);
  vm.runInContext(functionSource('toggleLightboxDanmaku'), context);
  vm.runInContext(functionSource('bindDanmakuSettings'), context);
  context.bindDanmakuSettings();
  const renderer = context.window.EchoGalleryDanmaku;
  assert.equal(renderer.isEnabled(), true);
  context.toggleLightboxDanmaku();
  assert.equal(renderer.isEnabled(), false);
  assert.equal(input.checked, false);
  input.checked = true;
  onChange({ target: input });
  assert.equal(renderer.isEnabled(), true);
  context.toggleLightboxDanmaku();
  assert.equal(renderer.isEnabled(), false);
  renderer.stop();
  assert.equal(renderer.isEnabled(), false);
  renderer.setEnabled(true);
  assert.equal(renderer.isEnabled(), true);
});

test('filename display decodes SMB escapes while preserving other Unicode and original names', () => {
  const context = vm.createContext({ window: {}, navigator: { language: 'zh-CN' } });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../web/static/lightbox.js'), 'utf8'), context);
  const display = context.window.EchoGalleryLightbox.displayFilename;
  const original = '轻语\uf027掏耳\uf027硬质偏多.mp4';
  assert.equal(display(original), '轻语|掏耳|硬质偏多.mp4');
  assert.equal(original, '轻语\uf027掏耳\uf027硬质偏多.mp4');
  assert.equal(display('\uf020\uf021\uf022\uf023\uf024\uf025\uf026\uf027\uf028\uf029'), '"*:<>?\\| .');
  assert.equal(display('中文 日本語 한글 🎧 e\u0301 | \ue000'), '中文 日本語 한글 🎧 e\u0301 | \ue000');
  assert.equal(display(null), '');
});
