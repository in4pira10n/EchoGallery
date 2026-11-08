const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname, '../web/static/app.js'), 'utf8');
function harness() {
  let rect = { top: 900, bottom: 920 };
  const frames = [];
  const sentinel = { isConnected: true, getClientRects: () => [rect], getBoundingClientRect: () => rect };
  const context = vm.createContext({
    state: { blockingInteraction: false }, window: { innerHeight: 800 },
    document: { getElementById: () => sentinel }, requestAnimationFrame: fn => frames.push(fn),
  });
  for (const name of ['loadMoreSentinelIsNear', 'maybeLoadMoreImmediately']) {
    vm.runInContext(source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'))[0], context);
  }
  return { context, sentinel, frames, move: value => { rect = value; } };
}
test('pagination fills a short wall but ignores a sentinel far above the viewport', () => {
  const h = harness();
  let loads = 0;
  h.move({ top: -3000, bottom: -2980 });
  h.context.maybeLoadMoreImmediately('load-more', () => loads++, () => true);
  assert.equal(h.frames.length, 0);
  h.move({ top: 900, bottom: 920 });
  h.context.maybeLoadMoreImmediately('load-more', () => loads++, () => true);
  h.frames.shift()();
  assert.equal(loads, 1);
});
test('pagination rechecks layout, connection, and modal state before loading', () => {
  const h = harness();
  let loads = 0;
  const check = () => h.context.maybeLoadMoreImmediately('load-more', () => loads++, () => true);
  check(); h.move({ top: 1500, bottom: 1520 }); h.frames.shift()();
  assert.equal(loads, 0);
  h.move({ top: 900, bottom: 920 });
  check(); h.sentinel.isConnected = false; h.frames.shift()();
  assert.equal(loads, 0);
  h.sentinel.isConnected = true;
  check(); h.context.state.blockingInteraction = true; h.frames.shift()();
  assert.equal(loads, 0);
});

test('scroll rearms a continuously visible sentinel after a blocked intersection callback', () => {
  const h = harness();
  const listeners = new Map();
  let callback, disconnected = false, loads = 0;
  Object.assign(h.context, {
    disconnectSingleLoadMoreObserver() {},
    IntersectionObserver: class {
      constructor(fn) { callback = fn; }
      observe() {}
      disconnect() { disconnected = true; }
    },
  });
  h.context.window.addEventListener = (event, fn) => listeners.set(event, fn);
  h.context.window.removeEventListener = (event, fn) => { if (listeners.get(event) === fn) listeners.delete(event); };
  h.context.state.loadMoreObservers = {};
  h.context.state.blockingInteraction = true;
  vm.runInContext(source.match(/^function observeLoadMore\([^]*?^}/m)[0], h.context);
  h.context.observeLoadMore('load-more', () => loads++, () => true);
  callback([{target: h.sentinel, isIntersecting: true}]);
  assert.equal(loads, 0);
  h.context.state.blockingInteraction = false;
  listeners.get('scroll')();
  h.frames.shift()();
  assert.equal(loads, 1);
  h.context.state.loadMoreObservers['load-more'].disconnect();
  assert.equal(disconnected, true);
  assert.equal(listeners.size, 0);
});
