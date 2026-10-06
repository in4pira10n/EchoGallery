const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const html = fs.readFileSync(path.join(__dirname, '../web/pages/login.html'), 'utf8');
const catalog = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/static/strings/zh-CN.json'), 'utf8'));
const css = fs.readFileSync(path.join(__dirname, '../web/pages/login.css'), 'utf8');

function openPage(storage = new Map(), offline = false, random = () => 0) {
  const nodes = new Map();
  const events = new Map();
  const requests = [];
  function node(id) {
    if (!nodes.has(id)) nodes.set(id, { textContent: '', value: '', style: {}, addEventListener() {}, setAttribute() {} });
    return nodes.get(id);
  }
  const context = vm.createContext({
    document: {
      documentElement: { dataset: {} },
      getElementById: node,
      querySelector: node,
      addEventListener() {},
    },
    window: {
      matchMedia: () => ({ matches: false, addEventListener() {} }),
      addEventListener: (event, listener) => events.set(event, listener),
    },
    localStorage: {
      getItem: (key) => storage.get(key),
      setItem: (key, value) => storage.set(key, value),
    },
    Math: Object.assign(Object.create(Math), { random }),
    location: { search: '' },
    URLSearchParams,
    fetch: async (url) => {
      requests.push(url);
      if (offline) throw new Error('offline');
      return { ok: true, json: async () => catalog };
    },
  });
  for (const match of html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)) {
    vm.runInContext(match[1], context);
  }
  return {
    motto: () => node('login-hero-summary-text').textContent,
    author: () => node('login-hero-summary-author').textContent,
    pageshow: (persisted) => events.get('pageshow')({ persisted }),
    requests,
  };
}

test('each opening excludes the previous motto, even with identical random values', () => {
  const storage = new Map();
  let previous = '';
  for (let i = 0; i < 20; i++) {
    const page = openPage(storage);
    assert.ok(page.motto());
    assert.notEqual(page.motto(), previous);
    previous = page.motto();
  }
});

test('author appears separately with a double em dash instead of parentheses', () => {
  let random = 0.65;
  const page = openPage(new Map(), false, () => random);
  assert.equal(page.motto(), 'Well done is better than well said.');
  assert.equal(page.author(), '\u2014\u2014 Benjamin Franklin');
  random = 0.7;
  page.pageshow(true);
  assert.equal(page.motto(), 'Nothing will come of nothing.');
  assert.equal(page.author(), '\u2014\u2014 William Shakespeare');
});

test('previously stored quotes still prevent consecutive repeats', () => {
  const storage = new Map([['echogallery_login_motto_v1', 'Well done is better than well said. (Benjamin Franklin)']]);
  const page = openPage(storage, false, () => 0.65);
  assert.notEqual(page.motto(), 'Well done is better than well said.');
  assert.notEqual(page.author(), '\u2014\u2014 Benjamin Franklin');
});

test('loading localized copy does not overwrite the motto or request an external API', async () => {
  const page = openPage();
  const initial = page.motto();
  await new Promise(setImmediate);
  assert.equal(page.motto(), initial);
  assert.deepEqual(page.requests, ['/static/strings/zh-CN.json']);
});

test('restoring from browser back-forward cache changes the motto', () => {
  const page = openPage();
  const initial = page.motto();
  page.pageshow(false);
  assert.equal(page.motto(), initial);
  page.pageshow(true);
  assert.notEqual(page.motto(), initial);
});

test('offline mode and blocked browser storage do not break the welcome page', async () => {
  const blockedStorage = { get() { throw new Error('blocked'); }, set() { throw new Error('blocked'); } };
  const page = openPage(blockedStorage, true);
  const initial = page.motto();
  assert.ok(initial);
  await new Promise(setImmediate);
  assert.equal(page.motto(), initial);
  page.pageshow(true);
  assert.notEqual(page.motto(), initial);
});

test('gallery art is decorative, local, and does not restore the photo carousel', () => {
  assert.match(html, /class="login-hero-art" aria-hidden="true"/);
  assert.match(html, /<svg[^>]*focusable="false"/);
  assert.match(html, /class="login-art-moon-cutout"/);
  assert.doesNotMatch(html, /\/api\/login\/hero|login-hero-media|<image\b/);
});

test('hero text colors meet AA contrast against both theme backgrounds', () => {
  function luminance(hex) {
    const channels = hex.match(/[a-f\d]{2}/gi).map((value) => {
      const c = parseInt(value, 16) / 255;
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  }
  const themes = [...css.matchAll(/(?:\[data-theme="dark"\] )?\.login-page \{([^}]+)\}/g)];
  assert.equal(themes.length, 2);
  for (const [, declarations] of themes) {
    const color = (name) => declarations.match(new RegExp('--login-' + name + ': (#[a-f\\d]{6})'))[1];
    const background = luminance(color('hero-bg'));
    for (const name of ['ink', 'muted']) {
      const foreground = luminance(color(name));
      const contrast = (Math.max(background, foreground) + 0.05) / (Math.min(background, foreground) + 0.05);
      assert.ok(contrast >= 4.5, name + ' contrast: ' + contrast);
    }
  }
});

test('gallery styles include reduced-motion and narrow-screen fallbacks', () => {
  assert.match(css, /@media \(prefers-reduced-motion: reduce\)\s*\{\s*\.login-page \.login-hero-art \{ animation: none;/);
  assert.match(css, /@media \(max-width: 780px\)/);
  assert.match(css, /\.login-page \.login-card \{ width: min\(100%, 460px\);/);
  assert.match(css, /\.login-page \.login-hero-art \{\s*position: absolute;[\s\S]*?animation: none;/);
  assert.match(css, /env\(safe-area-inset-bottom\)/);
});
