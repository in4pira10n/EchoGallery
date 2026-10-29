const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const appSource = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
const appCss = fs.readFileSync(path.join(__dirname, '../web/pages/app.css'), 'utf8');

function functionSource(name) {
  const match = appSource.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
  assert.ok(match, `missing function: ${name}`);
  return match[0];
}

function makeContext(view, { modal = false, ignoreTarget = false, echoFrameActive = false } = {}) {
  const classes = new Set();
  const state = { view, photoWallShowNames: false };
  const context = vm.createContext({
    state,
    _ctxMenu: null,
    echoFrame: echoFrameActive ? {} : null,
    document: {
      documentElement: {
        classList: {
          toggle(name, enabled) {
            if (enabled) classes.add(name);
            else classes.delete(name);
          },
        },
      },
      querySelector: () => modal ? {} : null,
    },
    $: () => ({ classList: { contains: () => false } }),
    shouldIgnoreGlobalShortcut: () => ignoreTarget,
  });
  for (const name of ['isPhotoWallPinchView', 'isPhotoWallView', 'syncPhotoWallNameMode', 'handlePhotoGridKeyboard']) {
    vm.runInContext(functionSource(name), context);
  }
  return { context, state, classes };
}

function keyEvent({ key = 'n', repeat = false, target = {} } = {}) {
  return {
    key,
    repeat,
    target,
    isComposing: false,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    preventDefault() { this.defaultPrevented = true; },
    stopImmediatePropagation() { this.stopped = true; },
  };
}

test('N toggles photo names across photo wall views and leaves non-wall views alone', () => {
  for (const view of ['timeline', 'favorites', 'random-album', 'album-detail', 'memories', 'trash', 'root-debug']) {
    const { context, state, classes } = makeContext(view);
    const event = keyEvent();

    assert.equal(context.handlePhotoGridKeyboard(event), true, view);
    assert.equal(state.photoWallShowNames, true, view);
    assert.equal(classes.has('photo-wall-name-mode'), true, view);
    assert.equal(event.defaultPrevented, true, view);
    assert.equal(event.stopped, true, view);

    context.handlePhotoGridKeyboard(keyEvent());
    assert.equal(state.photoWallShowNames, false, view);
    assert.equal(classes.has('photo-wall-name-mode'), false, view);
  }

  const { context, state } = makeContext('albums');
  assert.equal(context.handlePhotoGridKeyboard(keyEvent()), false);
  assert.equal(state.photoWallShowNames, false);
});

test('N ignores repeated presses, form targets, and open dialogs', () => {
  const repeated = makeContext('timeline');
  repeated.context.handlePhotoGridKeyboard(keyEvent({ repeat: true }));
  assert.equal(repeated.state.photoWallShowNames, false);

  const form = makeContext('timeline', { ignoreTarget: true });
  assert.equal(form.context.handlePhotoGridKeyboard(keyEvent({ target: { tagName: 'INPUT' } })), false);

  const modal = makeContext('timeline', { modal: true });
  assert.equal(modal.context.handlePhotoGridKeyboard(keyEvent()), false);

  const echoFrame = makeContext('timeline', { echoFrameActive: true });
  assert.equal(echoFrame.context.handlePhotoGridKeyboard(keyEvent()), false);
});

test('name overlay follows the three-line design and swaps with video bookmarks', () => {
  assert.match(appSource, /class=\"photo-thumb-name\" title=\"\$\{escapeHTML\(photoName\)\}\"/);
  assert.match(appCss, /-webkit-line-clamp:\s*3;/);
  assert.match(appCss, /--thumb-overlay-name-size:\s*13px;/);
  assert.match(appCss, /\.photo-thumb \.photo-thumb-name \{[^}]*font-size:\s*var\(--thumb-overlay-name-size\);/);
  assert.match(appCss, /\.photo-thumb \.photo-thumb-name \{[^}]*bottom:\s*9px;[^}]*line-height:\s*18px;/);
  assert.doesNotMatch(appCss, /\.photo-thumb \.photo-thumb-name \{[^}]*font-size:\s*clamp\(/);
  assert.match(appCss, /linear-gradient\(180deg, rgba\(0, 0, 0, 0\) 50%, rgba\(0, 0, 0, \.8\) 100%\)/);
  assert.match(appCss, /html\.photo-wall-name-mode \.photo-thumb \.photo-thumb-name \{[^}]*opacity:\s*1;/);
  assert.match(appCss, /html\.photo-wall-name-mode \.photo-thumb \.video-bookmark-badge \{[^}]*opacity:\s*0;/);
  assert.match(appCss, /@media \(prefers-reduced-motion: reduce\)/);
});
