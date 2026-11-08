const assert = require('node:assert/strict');
const test = require('node:test');
const { layout } = require('../web/static/photo-wall.js');

test('row window preserves exact total height and a bounded mount budget across a long wall', () => {
  for (const columns of [3, 5, 7, 16]) {
    const base = { count: 100000, columns, width: 1200, gap: 2, viewport: 844 };
    const initial = layout({ ...base, top: 100 });
    for (const top of [0, -1000, -50000, -initial.height + 900]) {
      const result = layout({ ...base, top });
      assert.equal(result.height, initial.height);
      assert.ok(result.end - result.start <= result.budget);
      assert.equal(result.start % columns, 0);
      assert.ok(result.start >= 0 && result.end <= base.count);
    }
  }
});

test('offscreen grids mount nothing and explicit location mounts the distant target row', () => {
  const base = { count: 10000, columns: 5, width: 1000, gap: 2, viewport: 700, top: 5000 };
  assert.equal(layout(base).end, 0);
  const located = layout({ ...base, focus: 9000 });
  assert.ok(located.start <= 9000 && located.end > 9000);
  assert.ok(located.end - located.start <= located.budget);
});

test('partial rows and empty grids do not add phantom trailing gaps', () => {
  assert.equal(layout({ count: 0, width: 302, columns: 3, gap: 1 }).height, 0);
  assert.equal(layout({ count: 4, width: 302, columns: 3, gap: 1 }).height, 201);
});
