const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
const css = fs.readFileSync(path.join(__dirname, '../web/pages/app.css'), 'utf8');

function loadFunctions(context, names) {
  for (const name of names) {
    const match = source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, `missing function: ${name}`);
    vm.runInContext(match[0], context);
  }
}

test('root album listing keeps user albums and folders side by side and filters each kind', () => {
  const context = vm.createContext({
    state: {
      albumKindFilter: 'all',
      albumChildrenIndex: null,
      albums: [
        { id: 1, name: '根目录', source_kind: 'folder', source_rel_path: '', description: '自动从文件夹导入' },
        { id: 2, name: '2024', source_kind: 'folder', source_rel_path: 'Trips/2024' },
        { id: 3, name: 'Trips', source_kind: 'folder', source_rel_path: 'Trips' },
        { id: 4, name: '用户相册', source_kind: '', source_rel_path: '', description: '' },
      ],
    },
  });
  loadFunctions(context, [
    'normalizeAlbumKindFilter', 'isFolderAlbum', 'normalizeAlbumPath', 'folderAlbumPath',
    'albumRelativeAddress', 'albumParentPath', 'isRootFolderAlbum', 'compareAlbumsByPath',
    'albumMatchesKindFilter', 'buildAlbumChildrenIndex', 'albumChildrenForParent',
  ]);

  const all = context.albumChildrenForParent('').map(album => album.id);
  const folders = context.albumChildrenForParent('', 'folder').map(album => album.id);
  const users = context.albumChildrenForParent('', 'user').map(album => album.id);
  const nested = context.albumChildrenForParent('Trips').map(album => album.id);
  assert.equal(all[0], 1, 'root folder stays first');
  assert.deepEqual(Array.from(all).sort(), [1, 3, 4]);
  assert.deepEqual(Array.from(folders).sort(), [1, 3]);
  assert.deepEqual(Array.from(users), [4]);
  assert.deepEqual(Array.from(nested), [2]);
});

test('user album detail does not render the root album grid as child folders', () => {
  const context = vm.createContext({
    state: {
      albumKindFilter: 'all',
      albumChildrenIndex: null,
      albums: [
        { id: 1, name: '根目录', source_kind: 'folder', source_rel_path: '', description: '自动从文件夹导入' },
        { id: 2, name: '用户相册', source_kind: '', source_rel_path: '', description: '' },
      ],
    },
  });
  loadFunctions(context, [
    'normalizeAlbumKindFilter', 'isFolderAlbum', 'normalizeAlbumPath', 'folderAlbumPath',
    'albumRelativeAddress', 'albumParentPath', 'isRootFolderAlbum', 'compareAlbumsByPath',
    'albumMatchesKindFilter', 'buildAlbumChildrenIndex', 'albumChildrenForParent',
    'albumChildAlbumsForDetail',
  ]);

  assert.equal(context.albumChildAlbumsForDetail(context.state.albums[1]).length, 0);
});

test('album kind filter exposes user/folder choices and keeps smart albums disabled', () => {
  const context = vm.createContext({
    state: { albumKindFilter: 'all' },
    icons: { album: '<svg></svg>', albumCardUser: '<svg><path /></svg>', albumCardFolder: '<svg><path /></svg>', albumIntelligence: '<svg><path /></svg>' },
  });
  loadFunctions(context, ['normalizeAlbumKindFilter', 'renderAlbumKindFilterControl']);
  const markup = context.renderAlbumKindFilterControl();
  assert.match(markup, /data-album-kind-filter="user"/);
  assert.match(markup, /data-album-kind-filter="folder"/);
  assert.match(markup, /data-album-kind-filter="smart"[^>]*disabled aria-disabled="true"/);
  assert.match(source, /albumIntelligence:\s*'album-intelligence\.svg'/);
  assert.match(source, /\{ key: 'smart', icon: icons\.albumIntelligence \|\| icons\.album/);
  assert.doesNotMatch(markup, /<span>/);
});

test('album picker uses the shared modal and modal button structure', () => {
  const context = vm.createContext({
    icons: { album: '<svg></svg>' },
    escapeHTML: value => String(value),
  });
  loadFunctions(context, ['libraryEditorActionButton', 'renderAlbumPickerModal']);
  const markup = context.renderAlbumPickerModal();
  assert.match(markup, /class="modal-overlay library-editor-modal" id="album-picker-modal"/);
  assert.match(markup, /class="modal library-editor-card album-picker-modal"/);
  assert.match(markup, /class="album-picker-grid-viewport"\s*>\s*<div class="album-picker-grid" id="album-picker-grid"/);
  assert.match(markup, /class="modal-footer library-editor-actions album-picker-actions"/);
  assert.match(markup, /class="btn library-editor-action-btn library-editor-action-btn-cancel" id="album-picker-cancel"/);
  assert.match(markup, /class="btn library-editor-action-btn library-editor-action-btn-save" id="album-picker-confirm"/);
});

test('create album modal reuses the library editor modal and its action buttons', () => {
  const context = vm.createContext({
    icons: { libraryEditorCancel: '<svg></svg>', libraryEditorSave: '<svg></svg>' },
    copyText: (_key, fallback) => fallback,
    escapeHTML: value => String(value),
  });
  loadFunctions(context, ['libraryEditorActionButton', 'renderCreateAlbumModal']);
  const markup = context.renderCreateAlbumModal();
  assert.match(markup, /class="modal-overlay library-editor-modal" id="create-album-modal"/);
  assert.match(markup, /class="modal library-editor-card create-album-card"/);
  assert.match(markup, /class="modal-footer library-editor-actions create-album-actions"/);
  assert.match(markup, /library-editor-action-btn-cancel" id="cancel-album-btn"/);
  assert.match(markup, /library-editor-action-btn-save" id="confirm-album-btn"/);
});

test('edit album modal reuses the library editor modal and its action buttons', () => {
  const context = vm.createContext({
    icons: { libraryEditorCancel: '<svg></svg>', libraryEditorSave: '<svg></svg>' },
    escapeHTML: value => String(value),
  });
  loadFunctions(context, ['libraryEditorActionButton', 'renderEditAlbumModal']);
  const markup = context.renderEditAlbumModal();
  assert.match(markup, /class="modal-overlay library-editor-modal" id="edit-album-modal"/);
  assert.match(markup, /class="modal library-editor-card edit-album-card"/);
  assert.match(markup, /class="settings-control"/);
  assert.match(markup, /class="modal-footer library-editor-actions edit-album-actions"/);
  assert.match(markup, /library-editor-action-btn-cancel" id="cancel-edit-album-btn"/);
  assert.match(markup, /library-editor-action-btn-save" id="confirm-edit-album-btn"/);
  assert.doesNotMatch(markup, /form-group|form-label|btn-primary/);
});

test('album picker builds cards from the shared album card structure', () => {
  assert.match(source, /const item = makeAlbumCard\(a, \{ picker: true \}\)/);
  assert.match(source, /class="album-picker-grid-viewport"[\s\S]*class="album-picker-grid"/);
  assert.match(source, /album-card-picker\.is-selected/);
  assert.match(css, /\.album-card-picker\.is-selected\s*\{/);
  assert.match(css, /\.album-picker-grid-viewport\s*\{[\s\S]*overflow-y:\s*auto/);
  assert.match(css, /@media \(max-width: 640px\)[\s\S]*#album-picker-modal \.album-picker-grid\s*\{[\s\S]*grid-template-columns:\s*repeat\(2/);
  assert.doesNotMatch(source, /album-picker-item|album-picker-cover|album-picker-name/);
  assert.doesNotMatch(css, /\.album-picker-item|\.album-picker-cover|\.album-picker-name/);
});

test('empty dedicated add-to-album SVG falls back to a visible icon', () => {
  const context = vm.createContext({
    icons: {
      albumContextAdd: '<svg xmlns="http://www.w3.org/2000/svg"></svg>',
      contextAlbum: '<svg><path d="M1 1" /></svg>',
      album: '<svg><circle /></svg>',
    },
  });
  loadFunctions(context, ['albumAddIconMarkup']);
  assert.equal(context.albumAddIconMarkup(), context.icons.contextAlbum);
  context.icons.albumContextAdd = '<svg><path d="M2 2" /></svg>';
  assert.equal(context.albumAddIconMarkup(), context.icons.albumContextAdd);
});

test('album context menu only offers adding selected media to user albums', () => {
  const added = [];
  const context = vm.createContext({
    state: { selected: new Set([10]) },
    icons: {
      albumContextAdd: '<svg></svg>',
      contextAlbum: '<svg><path /></svg>',
      album: '<svg></svg>',
      libraryCardEdit: '<svg data-icon="edit"></svg>',
      libraryCardDelete: '<svg data-icon="delete"></svg>',
    },
    canWriteMedia: () => true,
    canRevealInFileManager: () => false,
    mediaArchiveSaveLabel: () => '下载',
    mediaArchiveSaveVerb: () => '下载',
    addSelectedMediaToAlbum: album => added.push(album.id),
    openAlbumDetail() {},
    confirmDownloadAction() {},
    triggerDownload() {},
    openEditAlbumModal() {},
    deleteAlbum() {},
  });
  loadFunctions(context, ['compactContextMenuItems', 'albumAddIconMarkup', 'albumContextMenuItems']);

  const userAlbum = context.albumContextMenuItems({ id: 2, source_kind: '' });
  const addAction = userAlbum.find(item => item.role === 'album-add');
  assert.ok(addAction);
  assert.equal(addAction.icon, context.icons.contextAlbum);
  addAction.action();
  assert.deepEqual(added, [2]);
  const editAction = userAlbum.find(item => item.role === 'edit');
  const deleteAction = userAlbum.find(item => item.role === 'delete');
  assert.equal(editAction.label, '编辑');
  assert.equal(editAction.icon, context.icons.libraryCardEdit);
  assert.equal(deleteAction.label, '删除');
  assert.equal(deleteAction.icon, context.icons.libraryCardDelete);

  const folderAlbum = context.albumContextMenuItems({ id: 3, source_kind: 'folder' });
  assert.equal(folderAlbum.some(item => item.role === 'album-add'), false);
  context.state.selected.clear();
  assert.equal(context.albumContextMenuItems({ id: 4, source_kind: '' }).some(item => item.role === 'album-add'), false);
});

test('empty album covers use different icons for folders and user albums', () => {
  assert.match(source, /const emptyCoverIcon = albumKind === 'folder' \? icons\.albumCardFolder : icons\.albumCardUser;/);
  assert.match(source, /album-cover-empty album-cover-empty-\$\{albumKind\}/);
  assert.match(source, /emptyCoverIcon \|\| icons\.photo/);
});

test('logo crop reset stays in the zoom row and mobile action buttons split evenly', () => {
  const modal = source.match(/^function renderLibraryLogoGuideModal\([^]*?^}/m)?.[0] || '';
  assert.ok(modal);
  assert.ok(modal.indexOf('id="library-logo-crop-zoom-in"') < modal.indexOf('id="library-logo-crop-reset"'));
  assert.match(css, /\.library-logo-crop-toolbar\s*\{[^}]*grid-template-columns:\s*42px minmax\(0, 1fr\) 42px 42px/s);
  assert.match(css, /\.library-logo-crop-actions\s*\{[^}]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\)/s);
  assert.doesNotMatch(css, /\.library-logo-crop-reset svg\s*\{[^}]*filter:\s*invert\(1\)/s);
});
