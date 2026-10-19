const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const html = fs.readFileSync(path.join(__dirname, '../web/pages/login.html'), 'utf8');
const i18n = fs.readFileSync(path.join(__dirname, '../web/static/i18n.js'), 'utf8');
const catalog = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/static/strings/zh-CN.json'), 'utf8'));
const localeManifest = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/static/strings/locales.json'), 'utf8'));
const css = fs.readFileSync(path.join(__dirname, '../web/pages/login.css'), 'utf8');

function openPage(offline = false) {
  const nodes = new Map();
  const events = new Map();
  const requests = [];
  function node(id) {
    if (!nodes.has(id)) {
      nodes.set(id, {
        textContent: '',
        value: '',
        src: '/static/pwa-icon.png',
        style: {},
        attributes: {},
        classList: { add() {}, remove() {}, toggle() {} },
        addEventListener() {},
        setAttribute(name, value) { this.attributes[name] = String(value); },
        removeAttribute(name) { delete this.attributes[name]; },
        getAttribute(name) { return name === 'src' ? this.src : this.attributes[name] || ''; },
      });
    }
    return nodes.get(id);
  }
  const document = {
    title: '',
    documentElement: { dataset: {} },
    getElementById: node,
    querySelector(selector) {
      if (selector === '.auth-pane') return node('login-auth-pane');
      if (selector === '.auth-brand') return node('login-auth-brand');
      if (selector === '.auth-recovery') return node('login-register-link');
      return node(selector);
    },
    addEventListener() {},
  };
  const localStorage = {
    getItem() { return null; },
    setItem() {},
  };
  const fetch = async (url) => {
    requests.push(url);
    if (offline) throw new Error('offline');
    if (url.endsWith('/locales.json')) return { ok: true, json: async () => localeManifest };
    return { ok: true, json: async () => catalog };
  };
  const window = {
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    addEventListener: (event, listener) => events.set(event, listener),
    clearTimeout() {},
    setTimeout() { return 0; },
    fetch,
    localStorage,
    navigator: { languages: ['zh-CN'], language: 'zh-CN' },
    location: { reload() {} },
    document,
  };
  const context = vm.createContext({
    document,
    window,
    fetch,
    localStorage,
    sessionStorage: {
      getItem() { return null; },
      setItem() {},
    },
    URLSearchParams,
    location: { search: '' },
  });
  vm.runInContext(i18n, context);
  for (const match of html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)) {
    vm.runInContext(match[1], context);
  }
  return {
    node,
    document,
    pageshow: (persisted) => events.get('pageshow')({ persisted }),
    requests,
  };
}

test('login copy is loaded from the local zh-CN catalog', async () => {
  const page = openPage();
  await new Promise(setImmediate);
  assert.deepEqual(page.requests, ['/static/strings/locales.json', '/static/strings/zh-CN.json']);
  assert.equal(page.node('login-username-label').textContent, catalog.pages.login.usernameLabel);
  assert.equal(page.node('login-password-label').textContent, catalog.pages.login.passwordLabel);
  assert.equal(page.node('username').placeholder, catalog.pages.login.usernamePlaceholder);
  assert.equal(page.node('password').placeholder, catalog.pages.login.passwordPlaceholder);
  assert.equal(page.node('register-btn').attributes.title, catalog.pages.login.registerAria);
});

test('offline catalog loading leaves the login shell usable', async () => {
  const page = openPage(true);
  await new Promise(setImmediate);
  assert.deepEqual(page.requests, ['/static/strings/locales.json', '/static/strings/zh-CN.json']);
  assert.equal(page.node('login-username-label').textContent, '');
  page.pageshow(true);
});

test('login layout keeps the local auth shell and avoids the old photo carousel', () => {
  assert.match(html, /class="auth-screen"/);
  assert.match(html, /class="auth-pane"/);
  assert.match(html, /class="auth-stage" aria-hidden="true"/);
  assert.doesNotMatch(html, /login-hero-media|login-hero-summary|<image\b/);
});

test('login styles keep separate light and dark surfaces', () => {
  assert.match(css, /\.login-page \{/);
  assert.match(css, /\[data-theme="dark"\] \.login-page \{/);
  assert.match(css, /\[data-theme="dark"\] \.auth-screen[\s\S]*background:/);
});

test('login styles include reduced-motion and narrow-screen fallbacks', () => {
  assert.match(css, /@media \(prefers-reduced-motion: reduce\)[\s\S]*\.auth-stage \{\s*animation: none;/);
  assert.match(css, /@media \(max-width: 780px\)/);
  assert.match(css, /\.auth-pane[\s\S]*padding:/);
  assert.match(css, /env\(safe-area-inset-bottom\)/);
});
