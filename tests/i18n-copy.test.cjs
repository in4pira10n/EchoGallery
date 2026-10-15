const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const root = path.join(__dirname, '..');
const catalog = JSON.parse(fs.readFileSync(path.join(root, 'web/static/strings/zh-CN.json'), 'utf8'));
const englishCatalog = JSON.parse(fs.readFileSync(path.join(root, 'web/static/strings/en-US.json'), 'utf8'));
const appSource = fs.readFileSync(path.join(root, 'web/static/app.js'), 'utf8');
const echoFrameSource = fs.readFileSync(path.join(root, 'web/static/echo-frame.js'), 'utf8');
const lightboxSource = fs.readFileSync(path.join(root, 'web/static/lightbox.js'), 'utf8');
const setupSource = fs.readFileSync(path.join(root, 'web/pages/setup.html'), 'utf8');
const loginSource = fs.readFileSync(path.join(root, 'web/pages/login.html'), 'utf8');
const registerSource = fs.readFileSync(path.join(root, 'web/pages/register.html'), 'utf8');
const setupFallbackSource = fs.readFileSync(path.join(root, 'internal/api/setup_page.go'), 'utf8');
const apiSource = fs.readFileSync(path.join(root, 'internal/api/router.go'), 'utf8');
const i18nSource = fs.readFileSync(path.join(root, 'web/static/i18n.js'), 'utf8');
const localeManifest = JSON.parse(fs.readFileSync(path.join(root, 'web/static/strings/locales.json'), 'utf8'));

function valueAt(object, dottedPath) {
  if (!dottedPath) return object;
  return dottedPath.split('.').reduce((value, key) => value && value[key], object);
}

function assertCatalogKeys(source, expression, basePath) {
  const keys = new Set([...source.matchAll(expression)].map(match => match[1]).filter(key => !key.includes('${')));
  const missing = [...keys].filter(key => typeof valueAt(catalog, `${basePath}.${key}`) !== 'string');
  assert.deepEqual(missing, [], `Missing ${basePath} copy keys: ${missing.join(', ')}`);
}

function flattenCatalog(value, prefix = '') {
  return Object.entries(value || {}).flatMap(([key, child]) => (
    child && typeof child === 'object' && !Array.isArray(child)
      ? flattenCatalog(child, prefix ? `${prefix}.${key}` : key)
      : [prefix ? `${prefix}.${key}` : key]
  ));
}

function placeholders(value) {
  return [...String(value || '').matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort();
}

test('app copyText literal keys are present in zh-CN', () => {
  assert.equal(typeof catalog.locale, 'string');
  assert.doesNotMatch(appSource, /const COPY_DEFAULTS\s*=/);
  const keys = new Set([...appSource.matchAll(/copyText\(\s*['"`]([^'"`]+)['"`]/g)]
    .map(match => match[1]).filter(key => !key.includes('${')));
  const missing = [...keys].filter(key => typeof valueAt(catalog.app, key.replace(/^app\./, '')) !== 'string');
  assert.deepEqual(missing, [], `Missing app copy keys: ${missing.join(', ')}`);
});

test('every App interface copy entry is referenced by UI code', () => {
  const sources = [appSource, echoFrameSource, lightboxSource];
  const used = new Set();
  for (const source of sources) {
    for (const match of source.matchAll(/copyText\(\s*['` ]app\.interface\.([^'` ]+)['` ]/g)) used.add(match[1]);
  }
  function flatten(value, prefix = '') {
    return Object.entries(value || {}).flatMap(([key, child]) => (
      child && typeof child === 'object'
        ? flatten(child, prefix ? `${prefix}.${key}` : key)
        : [prefix ? `${prefix}.${key}` : key]
    ));
  }
  const catalogKeys = flatten(catalog.app.interface);
  const unused = catalogKeys.filter(key => !used.has(key));
  assert.deepEqual(unused, [], `Unused App copy keys: ${unused.join(', ')}`);
});

test('EchoFrame and lightbox UI copy is catalog-backed', () => {
  assertCatalogKeys(echoFrameSource, /echoFrameText\(\s*'([^']+)'/g, 'app.echoFrame');
  assertCatalogKeys(lightboxSource, /copyText\(\s*'([^']+)'/g, 'app.lightbox');
  assert.match(lightboxSource, /toLocaleString\(lightboxLocale/);
  assert.match(echoFrameSource, /toLocaleDateString\(echoFrameLocale\(\)/);
});

test('app date formatting follows catalog locale metadata', () => {
  assert.match(appSource, /function copyLocale\(\)/);
  assert.doesNotMatch(appSource, /toLocale(?:DateString|String)\('zh-CN'/);
});

test('setup UI text helper paths exist in the catalog', () => {
  assertCatalogKeys(setupSource, /text\(\s*'([^']+)'/g, 'pages.setup');
});

test('auth and setup templates do not hardcode Chinese UI copy', () => {
  for (const [name, source] of [['login', loginSource], ['register', registerSource], ['setup', setupSource]]) {
    assert.doesNotMatch(source, /[\u4e00-\u9fff]/, `${name} template should read UI copy from zh-CN.json`);
  }
});

test('setup fallback copy is catalog-backed', () => {
  const copy = catalog.pages.setup.assetFallback;
  assert.equal(typeof copy.pageTitle, 'string');
  assert.equal(typeof copy.title, 'string');
  assert.equal(typeof copy.message, 'string');
  assert.match(setupFallbackSource, /EchoGalleryI18n\.load\(\)/);
  assert.doesNotMatch(setupFallbackSource, /\/static\/strings\/zh-CN\.json/);
});

test('published catalogs are complete and preserve interpolation placeholders', () => {
  assert.deepEqual(localeManifest.locales.map(locale => locale.code), ['zh-CN', 'en-US']);
  assert.equal(englishCatalog.locale, 'en-US');
  const sourceKeys = flattenCatalog(catalog).filter(key => key !== 'locale');
  const englishKeys = flattenCatalog(englishCatalog).filter(key => key !== 'locale');
  assert.deepEqual(englishKeys.sort(), sourceKeys.sort());
  for (const key of sourceKeys) {
    assert.deepEqual(placeholders(valueAt(catalog, key)), placeholders(valueAt(englishCatalog, key)), `Placeholder mismatch: ${key}`);
  }
});

test('locale manifest only advertises catalogs that exist and keeps page titles', () => {
  for (const locale of localeManifest.locales) {
    assert.equal(fs.existsSync(path.join(root, 'web/static/strings', `${locale.code}.json`)), true, `Missing catalog: ${locale.code}`);
  }
  assert.equal(catalog.pages.login.pageTitle, '登录 - EchoGallery');
  assert.equal(catalog.pages.register.pageTitle, '注册 - EchoGallery');
  assert.equal(englishCatalog.pages.login.pageTitle, 'Sign in - EchoGallery');
  assert.equal(englishCatalog.pages.register.pageTitle, 'Register - EchoGallery');
  assert.equal(catalog.app.logout, undefined);
});

test('auth API errors expose catalog keys for browser localization', () => {
  for (const key of [
    'pages.login.errors.invalidRequest',
    'pages.login.errors.invalidCredentials',
    'pages.login.reasons.duplicate',
    'pages.register.errors.invalidRole',
    'pages.register.errors.passwordMismatch',
  ]) assert.equal(typeof valueAt(catalog, key), 'string', `Missing API copy ${key}`);
  assert.match(apiSource, /"error_key": key/);
  assert.match(appSource, /localizedAPIText\(payload, 'error'/);
  assert.match(loginSource, /EchoGalleryI18n\.text\(key\)/);
  assert.match(registerSource, /EchoGalleryI18n\.text\(payload\.error_key\)/);
});

test('language loader selects a supported browser language and falls back per key', async () => {
  const storage = new Map();
  const calls = [];
  const documentElement = {};
  const window = {
    localStorage: {
      getItem: key => storage.get(key) || null,
      setItem: (key, value) => storage.set(key, value),
    },
    navigator: { languages: ['fr-FR', 'en-GB'], language: 'fr-FR' },
    location: { reload() { this.reloaded = true; } },
    document: { documentElement, addEventListener() {} },
    fetch: async url => {
      calls.push(url);
      const pathname = new URL(url, 'http://localhost').pathname;
      const data = pathname.endsWith('/locales.json')
        ? { locales: [{ code: 'zh-CN', label: '简体中文' }, { code: 'en-US', label: 'English' }] }
        : pathname.endsWith('/en-US.json')
          ? { locale: 'en-US', app: { api: { title: 'Library' } } }
          : { locale: 'zh-CN', app: { api: { title: '资源库', fallback: '中文回退' } } };
      return { ok: true, json: async () => data };
    },
  };
  vm.runInNewContext(i18nSource, { window, Promise, encodeURIComponent });
  const catalog = await window.EchoGalleryI18n.load();
  assert.equal(catalog.locale, 'en-US');
  assert.equal(window.EchoGalleryI18n.text('app.api.title'), 'Library');
  assert.equal(window.EchoGalleryI18n.text('app.api.fallback'), '中文回退');
  assert.equal(documentElement.lang, 'en-US');
  assert.ok(calls.some(url => url.endsWith('/en-US.json')));
  assert.equal(window.EchoGalleryI18n.setLocale('zh-CN'), true);
  assert.equal(storage.get('echogallery_locale_v1'), 'zh-CN');
  assert.equal(window.location.reloaded, true);
});

