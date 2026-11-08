const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const root = path.join(__dirname, '..');
const catalog = JSON.parse(fs.readFileSync(path.join(root, 'web/static/strings/zh-CN.json'), 'utf8'));
const englishCatalog = JSON.parse(fs.readFileSync(path.join(root, 'web/static/strings/en-US.json'), 'utf8'));
const appSource = fs.readFileSync(path.join(root, 'web/static/app.js'), 'utf8');
const appCss = fs.readFileSync(path.join(root, 'web/pages/app.css'), 'utf8');
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

test('danmaku manager exposes a visible localized title', () => {
  const render = appSource.match(/^function renderDanmakuManager\([^]*?^}/m)?.[0] || '';
  assert.match(render, /copyText\('app\.danmaku\.manageTitle'\) \|\| copyText\('app\.danmaku\.manage'\)/);
  assert.match(render, /<h2 class="danmaku-manage-heading-title" id="danmaku-manage-title">\$\{escapeHTML\(title\)\}<\/h2>/);
  assert.match(appCss, /\.danmaku-modal-banner \.danmaku-manage-heading-title \{[^}]*display:\s*block;[^}]*visibility:\s*visible;[^}]*opacity:\s*1;/);
  assert.equal(catalog.app.danmaku.manageTitle, '管理弹幕记录');
  assert.equal(englishCatalog.app.danmaku.manageTitle, 'Manage danmaku records');
});

test('batch library editor scrolls its results and uses English labels', () => {
  assert.match(appCss, /\.modal \{[^}]*max-height:\s*calc\(100vh - 28px\);[^}]*overflow-y:\s*auto;/);
  assert.match(appCss, /\.settings-batch-build-selection-list \{[^}]*overflow-y:\s*auto;/);
  assert.equal(englishCatalog.app.settings.libraries.editor.create, 'Create');
  assert.equal(englishCatalog.app.settings.libraries.editor.batchTitle, 'Add libraries in batch');
  assert.equal(englishCatalog.app.interface.commonCreate, 'Create');
  assert.equal(englishCatalog.app.interface.s_3e44b2a933, 'Select all');
  assert.equal(englishCatalog.app.interface.s_2918633ac8, 'Explore');
});

test('library editor Enter activates Switch and timeline order confirmation is translated', () => {
  const editor = appSource.slice(appSource.indexOf('function openLibraryEditorModal('), appSource.indexOf('\nfunction syncActiveLibrarySelect', appSource.indexOf('function openLibraryEditorModal(')));
  assert.match(editor, /event\.key !== 'Enter'[\s\S]*?const action = editing && switchButton[\s\S]*?action\.click\(\)/);
  for (const [copy, title, confirm] of [
    [catalog, '反转时间线顺序', '切换'],
    [englishCatalog, 'Reverse timeline order', 'Switch'],
  ]) {
    assert.equal(copy.app.interface.s_4b553446cb, title);
    assert.equal(copy.app.interface.s_2f116b7d1c, confirm);
    assert.equal(typeof copy.app.interface.d_8714d3085b, 'string');
  }
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

test('library build page localizes backend status and all duration units', () => {
  for (const [copy, language] of [[catalog, 'zh-CN'], [englishCatalog, 'en-US']]) {
    const nodes = new Map();
    const context = vm.createContext({
      copyCatalog: copy, window: {},
      state: { libraryBuildStatus: { status: 'running', message: '正在扫描与构建资源库', done: 395, total: 979, elapsed_seconds: 3, eta_seconds: 5 } },
      $: selector => {
        if (!nodes.has(selector)) nodes.set(selector, {});
        return nodes.get(selector);
      },
      escapeHTML: value => String(value || ''),
      renderSettingsToggle: (_id, label) => label,
    });
    for (const name of ['interpolateCopy', 'copyText', 'formatSecondsShort', 'renderLibraryScanDiscovery', 'libraryBuildHeading', 'renderLibraryBuild']) {
      const fn = appSource.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
      assert.ok(fn, `Missing ${name}`);
      vm.runInContext(fn[0], context);
    }
    context.renderLibraryBuild();
    const html = nodes.get('#content').innerHTML;
    assert.ok(html.includes(copy.app.libraryBuild.runningTitle));
    assert.ok(html.includes('395 / 979'));
    for (const status of ['discovering', 'running', 'cancelling', 'error']) {
      const heading = context.libraryBuildHeading({ status, done: 12, message: '后台中文状态' });
      assert.ok(heading.trim());
      if (language === 'en-US') assert.doesNotMatch(heading, /[\u4e00-\u9fff]/);
    }
    if (language === 'en-US') {
      assert.doesNotMatch(html, /[\u4e00-\u9fff]/);
      assert.equal(context.formatSecondsShort(3), '3s');
      assert.equal(context.formatSecondsShort(65), '1m 5s');
      assert.equal(context.formatSecondsShort(3660), '1h 1m');
    } else {
      assert.equal(context.formatSecondsShort(3), '3 秒');
    }
  }
});

test('versioned catalogs keep audio track menu labels present with an older PWA cache', async () => {
  for (const copy of [catalog, englishCatalog]) {
    const requests = [];
    const window = {
      document: { currentScript: { src: 'http://localhost/static/i18n.js?v=eg-current' }, documentElement: {}, addEventListener() {} },
      navigator: { languages: [copy.locale] },
      localStorage: { getItem: () => null },
      fetch: async url => {
        requests.push(url);
        const parsed = new URL(url, 'http://localhost');
        // The previous cache contains catalogs from before audio track switching.
        const data = parsed.pathname.endsWith('/locales.json') ? localeManifest
          : parsed.searchParams.get('v') === 'eg-current' ? copy
          : { locale: copy.locale, app: { lightbox: {} } };
        return { ok: true, json: async () => data };
      },
    };
    vm.runInNewContext(i18nSource, { window, Promise, encodeURIComponent });
    const loaded = await window.EchoGalleryI18n.load();
    const context = vm.createContext({ window, copyCatalog: loaded });
    for (const name of ['interpolateCopy', 'copyText']) {
      vm.runInContext(appSource.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'))[0], context);
    }
    assert.equal(context.copyText('app.lightbox.switchAudioTrack'), copy.app.lightbox.switchAudioTrack);
    assert.match(appSource, /role: 'switch-audio-track'[^\n]+label: copyText\('app\.lightbox\.switchAudioTrack'\)/);
    assert.ok(requests.every(url => new URL(url, 'http://localhost').searchParams.get('v') === 'eg-current'));
  }
});

test('PWA catalogs refresh online and use only JSON cache entries offline', async () => {
  const listeners = {};
  const fresh = { ok: true, clone() { return this; } };
  const cached = { cachedCatalog: true };
  let online = true;
  let cachedCatalog = cached;
  const windowOrigin = 'http://localhost';
  vm.runInNewContext(fs.readFileSync(path.join(root, 'web/static/sw.js'), 'utf8'), {
    URL, location: { origin: windowOrigin },
    self: { addEventListener: (name, listener) => { listeners[name] = listener; } },
    fetch: async () => { if (!online) throw new Error('offline'); return fresh; },
    caches: {
      match: async request => request === '/' ? { htmlShell: true } : cachedCatalog,
      open: async () => ({ put: async () => {} }),
    },
  });
  const request = { method: 'GET', mode: 'cors', url: windowOrigin + '/static/strings/en-US.json?v=eg-current' };
  function fetchCatalog() {
    let response;
    listeners.fetch({ request, respondWith: promise => { response = promise; } });
    return response;
  }
  assert.equal(await fetchCatalog(), fresh);
  online = false;
  assert.equal(await fetchCatalog(), cached);
  cachedCatalog = undefined;
  await assert.rejects(fetchCatalog(), /offline/);
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

