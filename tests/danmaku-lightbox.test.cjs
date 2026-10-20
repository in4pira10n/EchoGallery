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
