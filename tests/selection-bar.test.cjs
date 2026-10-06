const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

function loadFunction(context, name) {
  const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
  assert.ok(match, `missing function: ${name}`);
  vm.runInContext(match[0], context);
}

test('selection bar exposes the deselect-favorite action', () => {
  const context = vm.createContext({
    icons: {
      selectionDownload: '<svg data-icon="download"></svg>',
      selectionDeselectFavorite: '<svg data-icon="deselect-favorite"></svg>',
      selectionClear: '<svg data-icon="clear"></svg>',
    },
    mediaArchiveSaveLabel: () => '下载',
    escapeHTML: value => String(value),
  });
  loadFunction(context, 'renderSelectionBarMarkup');

  const markup = context.renderSelectionBarMarkup();
  assert.match(markup, /id="deselect-favorite-sel-btn"/);
  assert.match(markup, /title="不选个人收藏"/);
  assert.match(markup, /data-icon="deselect-favorite"/);
  assert.doesNotMatch(context.renderSelectionBarMarkup({ includeDeselectFavorite: false }), /deselect-favorite-sel-btn/);
});

test('deselect-favorite action only removes favorite media from the current selection', () => {
  const toast = [];
  const deselectedThumbs = [];
  const updates = [];
  const photos = new Map([
    [1, { id: 1, is_favorite: true, is_super_favorite: false }],
    [2, { id: 2, is_favorite: false, is_super_favorite: false }],
    [3, { id: 3, is_favorite: false, is_super_favorite: true }],
  ]);
  const context = vm.createContext({
    state: { selected: new Set([1, 2, 3]), selectionAnchorID: 3 },
    findKnownPhotoByID: id => photos.get(Number(id)) || null,
    setThumbSelectedState: (id, selected) => deselectedThumbs.push([id, selected]),
    updateSelectionModeUI: () => updates.push('mode'),
    updateSelectionBar: () => updates.push('bar'),
    updateTrashSelBar: () => updates.push('trash'),
    showToast: message => toast.push(message),
  });
  loadFunction(context, 'deselectFavoriteSelected');

  context.deselectFavoriteSelected();

  assert.deepEqual([...context.state.selected], [2]);
  assert.equal(context.state.selectionAnchorID, 2);
  assert.deepEqual(deselectedThumbs, [[1, false], [3, false]]);
  assert.deepEqual(updates, ['mode', 'bar', 'trash']);
  assert.deepEqual(toast, ['已取消选择 2 个个人收藏']);
});
