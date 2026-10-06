const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

test('batch workflow ignores old migration preferences and starts scanning', async () => {
  const match = source.match(/^async function openLibraryBatchFullWorkflow\([^]*?^}/m);
  assert.ok(match, 'batch workflow handler is missing');
  const calls = [];
  const state = {
    libraryBatchWorkflowStartPending: false,
    libraryBatchWorkflowAggressive: false,
    libraryBatchWorkflowEnabled: false,
    libraryBatchWorkflowMoveLegacyThumbnails: true,
    libraryBatchWorkflowCleanThumbnailFiles: false,
    libraryBatchWorkflowBuildPlaybackCaches: false,
    libraryBatchThumbnailBuildStatus: { status: 'idle' },
  };
  const context = vm.createContext({
    state,
    libraryBatchWorkflowActive: () => false,
    isLibraryBatchBuildActive: () => false,
    isLibraryBatchThumbnailBuildActive: status => status?.status === 'running',
    syncLibraryBatchWorkflowPanel: () => {},
    syncLibraryBatchWorkflowSelections: async () => { calls.push('selection'); },
    setLibraryBatchWorkflowPhase: phase => { calls.push(phase); },
    clearLibraryBatchWorkflowPhase: () => { calls.push('clear'); },
    openLibraryBatchThumbnailBuildWorkflow: async () => { calls.push('thumbnails'); },
    openLibraryBatchBuildWorkflow: async options => { calls.push(options); },
    alert: message => { throw new Error(message); },
  });
  vm.runInContext(match[0], context);
  await context.openLibraryBatchFullWorkflow();
  assert.equal(calls[0], 'selection');
  assert.equal(calls[1], '');
  assert.equal(calls[2].buildThumbnailsAfterScan, false);
  assert.equal(calls[2].moveLegacyThumbnails, undefined);
  assert.equal(calls[2].cleanThumbnailFiles, undefined);
  assert.equal(calls.includes('thumbnails'), false);
  assert.equal(state.libraryBatchWorkflowStartPending, false);
  assert.equal(source.includes('settings-migrate-library-thumbnails-btn'), false);
});

test('scan option still starts the scan stage first', async () => {
  const match = source.match(/^async function openLibraryBatchFullWorkflow\([^]*?^}/m);
  const calls = [];
  const state = {
    libraryBatchWorkflowStartPending: false,
    libraryBatchWorkflowAggressive: false,
    libraryBatchWorkflowEnabled: true,
    libraryBatchWorkflowMoveLegacyThumbnails: true,
    libraryBatchWorkflowCleanThumbnailFiles: false,
    libraryBatchWorkflowBuildPlaybackCaches: false,
  };
  const context = vm.createContext({
    state,
    libraryBatchWorkflowActive: () => false,
    isLibraryBatchBuildActive: () => false,
    isLibraryBatchThumbnailBuildActive: () => false,
    syncLibraryBatchWorkflowPanel: () => {},
    syncLibraryBatchWorkflowSelections: async () => {},
    setLibraryBatchWorkflowPhase: phase => { calls.push(phase); },
    clearLibraryBatchWorkflowPhase: () => {},
    openLibraryBatchBuildWorkflow: async options => { calls.push(options); },
    openLibraryBatchThumbnailBuildWorkflow: async () => { calls.push('thumbnails'); },
    alert: message => { throw new Error(message); },
  });
  vm.runInContext(match[0], context);
  await context.openLibraryBatchFullWorkflow();
  assert.equal(calls[0], 'scan');
  assert.equal(calls[1].buildThumbnailsAfterScan, true);
  assert.equal(calls[1].moveLegacyThumbnails, undefined);
  assert.equal(calls.includes('thumbnails'), false);
});

test('batch API payloads never include paused stages', async () => {
  const calls = [];
  const context = vm.createContext({ api: { post: async (_url, payload) => { calls.push(payload); } } });
  for (const name of ['startLibraryBatchBuild', 'startLibraryBatchThumbnailBuild']) {
    const match = source.match(new RegExp('^async function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, name + ' is missing');
    vm.runInContext(match[0], context);
  }
  const oldOptions = { aggressive: false, buildThumbnailsAfterScan: true, moveLegacyThumbnails: true, cleanThumbnailFiles: true, buildPlaybackCaches: true };
  await context.startLibraryBatchBuild(oldOptions);
  await context.startLibraryBatchThumbnailBuild(oldOptions);
  for (const payload of calls) {
    assert.equal(payload.move_legacy_thumbnails, undefined);
    assert.equal(payload.clean_thumbnail_files, undefined);
    assert.equal(payload.build_playback_caches, undefined);
  }
  assert.equal(calls[0].build_thumbnails_after_scan, true);
});

test('progress remains continuous across scan and thumbnail build', () => {
  const context = vm.createContext({
    state: { libraryBatchWorkflowEnabled: true, libraryBatchWorkflowPhase: 'scan' },
    isLibraryBatchBuildActive: status => status?.status === 'running',
    isLibraryBatchThumbnailBuildActive: status => status?.status === 'running',
  });
  for (const name of ['clampProgressPercent', 'libraryBatchTaskPercent', 'libraryBatchWorkflowStageProgressPercent', 'libraryBatchWorkflowTotalProgressPercent']) {
    const match = source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
    assert.ok(match, name + ' is missing');
    vm.runInContext(match[0], context);
  }
  const scan = { status: 'running', total_libraries: 2, completed_libraries: 0, current_library_index: 1, current_percent: 50 };
  const thumbnails = { status: 'idle', total_libraries: 2, completed_libraries: 0 };
  context.state.libraryBatchBuildStatus = scan;
  context.state.libraryBatchThumbnailBuildStatus = thumbnails;
  const values = [context.libraryBatchWorkflowTotalProgressPercent()];
  scan.status = 'completed';
  scan.completed_libraries = 2;
  context.state.libraryBatchWorkflowPhase = 'thumbnails';
  thumbnails.status = 'running';
  values.push(context.libraryBatchWorkflowTotalProgressPercent());
  for (const current_percent of [50, 85, 95, 100]) {
    thumbnails.current_library_index = 1;
    thumbnails.current_percent = current_percent;
    values.push(context.libraryBatchWorkflowTotalProgressPercent());
  }
  assert.deepEqual(values.map(Math.round), [13, 50, 63, 71, 74, 75]);
});

test('summary shows only scan and thumbnail build counters', () => {
  const match = source.match(/^function renderLibraryBatchWorkflowSummary\([^]*?^}/m);
  assert.ok(match);
  const context = vm.createContext({
    state: {
      libraryBatchBuildStatus: { libraries: [{ imported: 12, skipped: 640, failed: 0 }] },
      libraryBatchThumbnailBuildStatus: { libraries: [{ skipped: 600, generated: 40, failed: 0 }] },
    },
    escapeHTML: value => String(value).replaceAll('<', '&lt;'),
  });
  vm.runInContext(match[0], context);
  const markup = context.renderLibraryBatchWorkflowSummary({
    status: 'running', current_phase: 'thumbnails', total_libraries: 1,
    current_done: 640, current_total: 17009,
    libraries: [{ skipped: 600, generated: 40 }],
  }, 51);
  assert.match(markup, /检查并构建/);
  assert.match(markup, /本阶段<\/dt><dd>640 \/ 17009/);
  assert.match(markup, /总进度<\/dt><dd>51%/);
  assert.match(markup, /扫描新增<\/dt><dd>12/);
  assert.match(markup, /缩略图已存在<\/dt><dd>600/);
  assert.match(markup, /新生成缩略图<\/dt><dd>40/);
  assert.doesNotMatch(markup, /旧图|迁移|清理/);
  assert.doesNotMatch(markup, /<table|border=/);
  assert.equal(source.includes('settings-migrate-library-thumbnails-btn'), false);
});
