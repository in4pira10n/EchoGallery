const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

function menuContext(hostname, admin = true) {
  const requests = [];
  const selections = [];
  let trashItems;
  const context = vm.createContext({
    copyText: key => ({
      'app.interface.s_4a8a30a99a': '选择照片',
      'app.interface.s_f02e943954': '取消选择',
      'app.interface.s_f7acefd2d4': '在相册中查看',
      'app.interface.s_cf4d216eca': '在文件管理器中打开',
      'app.interface.s_3755f56f2f': '删除',
      'app.interface.s_326155a897': '自动从文件夹导入',
    }[key] || ''),
    window: { location: { hostname } },
    state: { selected: new Set() },
    canManageLibraries: () => admin,
    canWriteMedia: () => true,
    canEditBookmarks: () => true,
    icons: { check: '<svg></svg>' },
    toggleSelect: (id) => {
      selections.push(id);
      if (context.state.selected.has(id)) context.state.selected.delete(id);
      else context.state.selected.add(id);
    },
    mediaArchiveSaveLabel: () => 'Download',
    showContextMenu: (_x, _y, items) => { trashItems = items; },
    api: { post: async (url) => { requests.push(url); } },
    showToast() {},
    alert(message) { throw new Error(message); },
  });
  for (const name of [
    'canRevealInFileManager', 'revealInFinder', 'revealAlbumInFinder',
    'compactContextMenuItems', 'filterPhotoContextMenuItemsForDevice', 'mobileSelectionMenuItem',
    'showTrashContextMenu', 'isRootFolderAlbum', 'albumContextMenuItems',
  ]) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, 'missing function: ' + name);
    vm.runInContext(match[0], context);
  }
  return { context, requests, selections, trashItems: () => trashItems };
}

function hasReveal(items) {
  return items.some(item => item.role === 'reveal');
}

for (const hostname of ['192.168.1.2', '10.0.0.2', '172.16.0.2', '[2001:db8::1]', 'gallery.example', 'localhost.example', 'notlocalhost', '127.0.0.1.example', '127.0.0.256']) {
  test('hide reveal across all menus on ' + hostname, async () => {
    const { context: c, requests, trashItems } = menuContext(hostname);
    assert.equal(c.canRevealInFileManager(), false);
    for (const mobile of [false, true]) {
      assert.equal(hasReveal(c.filterPhotoContextMenuItemsForDevice([{ role: 'reveal' }, '-', { role: 'download' }], { mobile })), false);
      assert.equal(hasReveal(c.albumContextMenuItems({ id: 1 }, { mobile })), false);
      c.showTrashContextMenu(0, 0, { id: 1 }, { mobile });
      assert.equal(hasReveal(trashItems()), false);
      assert.notEqual(trashItems()[0], '-');
    }
    await c.revealInFinder(1);
    await c.revealAlbumInFinder(1);
    assert.deepEqual(requests, []);
  });
}

for (const hostname of ['127.0.0.1', '127.0.0.2', '127.255.255.254', 'localhost', 'LOCALHOST', 'localhost.', 'gallery.localhost', '[::1]', '::1']) {
  test('allow admin reveal on ' + hostname, async () => {
    const { context: c, requests, trashItems } = menuContext(hostname);
    assert.equal(c.canRevealInFileManager(), true);
    assert.equal(hasReveal(c.filterPhotoContextMenuItemsForDevice([{ role: 'reveal' }])), true);
    assert.equal(hasReveal(c.albumContextMenuItems({ id: 1 })), true);
    c.showTrashContextMenu(0, 0, { id: 1 });
    assert.equal(hasReveal(trashItems()), true);
    c.showTrashContextMenu(0, 0, { id: 1 }, { mobile: true });
    assert.equal(hasReveal(trashItems()), false);
    assert.equal(hasReveal(c.filterPhotoContextMenuItemsForDevice([{ role: 'reveal' }], { mobile: true, lightbox: true })), false);
    await c.revealInFinder(1);
    await c.revealAlbumInFinder(1);
    assert.deepEqual(requests, ['/api/media/1/reveal', '/api/media/albums/1/reveal']);
  });
}

test('loopback access does not bypass admin permission', async () => {
  const { context: c, requests, trashItems } = menuContext('127.0.0.1', false);
  assert.equal(c.canRevealInFileManager(), false);
  assert.equal(hasReveal(c.filterPhotoContextMenuItemsForDevice([{ role: 'reveal' }])), false);
  assert.equal(hasReveal(c.albumContextMenuItems({ id: 1 }, { mobile: true })), false);
  c.showTrashContextMenu(0, 0, { id: 1 });
  assert.equal(hasReveal(trashItems()), false);
  await c.revealInFinder(1);
  await c.revealAlbumInFinder(1);
  assert.deepEqual(requests, []);
});

test('touch menu selects and deselects a photo without changing desktop menus', () => {
  const { context: c, selections, trashItems } = menuContext('localhost');
  const photo = { id: 42 };
  assert.equal(c.mobileSelectionMenuItem(photo, { mobile: false }), null);
  assert.equal(c.mobileSelectionMenuItem(photo, { mobile: true, lightbox: true }), null);
  assert.equal(c.mobileSelectionMenuItem(photo, { mobile: true, disableSelection: true }), null);

  c.showTrashContextMenu(0, 0, photo, { mobile: true });
  assert.equal(trashItems()[0].label, '选择照片');
  trashItems()[0].action();
  assert.deepEqual(selections, [42]);

  c.showTrashContextMenu(0, 0, photo, { mobile: true });
  assert.equal(trashItems()[0].label, '取消选择');
  trashItems()[0].action();
  assert.deepEqual(selections, [42, 42]);
  assert.equal(c.state.selected.size, 0);
});

test('photo context menu uses plus for add and album-type icons for view entries', () => {
  const context = vm.createContext({
    copyText: key => ({
      'app.interface.s_2cf712b27f': '在时间线中显示',
      'app.interface.s_045f5177b2': '添加到相册',
      'app.interface.s_cf4d216eca': '在文件管理器中打开',
      'app.interface.s_3755f56f2f': '删除',
      'app.interface.s_326155a897': '自动从文件夹导入',
      'app.interface.d_7b16bce3b1': '在「{v0}」中查看',
    }[key] || ''),
    state: { selected: new Set(), shareMap: {} },
    icons: {
      albumCardUser: '<svg>user</svg>', albumCardFolder: '<svg>folder</svg>', plus: '<svg>plus</svg>',
      albumIntelligence: '<svg>smart</svg>', contextAlbum: '<svg>generic</svg>',
    },
    canWriteMedia: () => true,
    favoriteLabelForPhoto: () => '收藏',
    singleMediaSaveLabel: () => '下载',
    albumDisplayTitle: album => album.name,
    toggleFavorite() {},
    mobileSelectionMenuItem: () => null,
    filterPhotoContextMenuItemsForDevice: items => items,
    requiresBrowserPlaybackConversion: () => false,
  });
  for (const name of ['isFolderAlbum', 'albumTypeContextIcon', 'photoContextMenuItems']) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, 'missing function: ' + name);
    vm.runInContext(match[0], context);
  }
  const albums = [
    { id: 1, source_kind: 'folder', name: '文件夹' },
    { id: 2, source_kind: 'smart', name: '智能相册' },
    { id: 3, source_kind: 'user', name: '个人相册' },
  ];
  const items = context.photoContextMenuItems({ id: 42 }, null, [], albums);
  assert.equal(items.find(item => item && item.role === 'album-add').icon, '<svg>plus</svg>');
  assert.deepEqual(Array.from(items.filter(item => item && item.role === 'album'), item => item.icon), [
    '<svg>folder</svg>', '<svg>smart</svg>', '<svg>user</svg>',
  ]);
});
