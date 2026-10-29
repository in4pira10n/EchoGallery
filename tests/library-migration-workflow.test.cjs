const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');

test('batch stages show localized library names and discovery counts instead of Idle', () => {
  for (const locale of ['zh-CN', 'en-US']) {
    const catalog = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/static/strings', locale + '.json'), 'utf8'));
    const context = vm.createContext({
      copyCatalog: catalog, window: {},
      state: { libraryBatchBuildStatus: { libraries: [] }, libraryBatchThumbnailBuildStatus: { libraries: [] } },
    });
    for (const name of ['interpolateCopy', 'copyText', 'settingsText', 'escapeHTML', 'libraryBatchBuildStatusLabel', 'libraryBatchPhaseLabel', 'libraryBatchActivityLabel', 'libraryBatchMessage', 'renderLibraryBatchTaskProgress', 'renderLibraryBatchWorkflowSummary']) {
      const match = source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'));
      assert.ok(match, name + ' is missing');
      vm.runInContext(match[0], context);
    }
    for (const phase of ['checking', 'discovering', 'scan', 'exif', 'thumbnails', 'audit', 'repair']) {
      const status = {
        status: 'running', current_phase: phase, current_library_name: 'Library <A>',
        current_library_index: 1, total_libraries: 2, current_done: 32, current_total: phase === 'discovering' ? 0 : 100,
        message: '后台中文状态',
      };
      const label = context.libraryBatchActivityLabel(status);
      assert.ok(label.includes('Library <A>'), label);
      assert.notEqual(label, catalog.app.settings.batchWorkflow.statusIdle);
      assert.equal(context.libraryBatchMessage(status), label);
      const summary = context.renderLibraryBatchWorkflowSummary(status, 10);
      const detail = context.renderLibraryBatchTaskProgress(status, 'Idle');
      assert.ok(summary.includes('Library &lt;A&gt;'), summary);
      assert.ok(detail.includes('Library <A>'), detail);
      assert.doesNotMatch(summary, /后台中文状态|<A>|Idle/);
      assert.doesNotMatch(detail, /Idle|\{\w+\}/);
      if (phase === 'discovering') {
        assert.ok(summary.includes(context.settingsText('batchWorkflow.taskDiscovered', { count: 32 })));
        assert.ok(detail.includes(context.settingsText('batchWorkflow.taskDiscovered', { count: 32 })));
      }
      if (locale === 'en-US') {
        assert.doesNotMatch(summary, /[\u4e00-\u9fff]/);
        assert.doesNotMatch(detail, /[\u4e00-\u9fff]/);
      }
    }
    for (const phase of ['', 'preparing', 'unrecognized']) {
      const label = context.libraryBatchPhaseLabel({ status: 'running', current_phase: phase });
      assert.equal(label, catalog.app.settings.batchWorkflow.phasePreparing);
    }
    for (const status of ['discovering', 'scanning', 'building']) {
      const label = context.libraryBatchActivityLabel({ status, current_library_name: 'Library A' });
      assert.ok(label.includes('Library A'), label);
      assert.notEqual(label, catalog.app.settings.batchWorkflow.statusIdle);
    }
    assert.equal(context.libraryBatchPhaseLabel({ status: 'idle' }), catalog.app.settings.batchWorkflow.statusIdle);
    assert.equal(context.libraryBatchPhaseLabel({ status: 'cancelled', current_phase: 'scan' }), catalog.app.settings.batchWorkflow.statusCancelled);
    assert.equal(context.libraryBatchPhaseLabel({ status: 'cancelling' }), catalog.app.settings.batchWorkflow.statusCancelling);
    assert.equal(context.libraryBatchMessage({ message_key: 'batchWorkflow.taskScanning', current_library_name: 'Library A' }), context.settingsText('batchWorkflow.taskScanning', { name: 'Library A' }));
  }
});

test('batch terminal messages and notifications use the selected locale', () => {
  for (const locale of ['zh-CN', 'en-US']) {
    const catalog = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/static/strings', locale + '.json'), 'utf8'));
    const context = vm.createContext({ copyCatalog: catalog, window: {} });
    for (const name of ['interpolateCopy', 'copyText', 'settingsText', 'libraryBatchBuildStatusLabel', 'libraryBatchPhaseLabel', 'libraryBatchActivityLabel', 'libraryBatchMessage']) {
      vm.runInContext(source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'))[0], context);
    }
    for (const kind of ['scan', 'thumbnails']) {
      const prefix = kind === 'scan' ? 'scan' : 'thumbnail';
      const status = { status: 'completed', total_libraries: 3, completed_libraries: 3, message: '批量任务完成' };
      assert.equal(context.libraryBatchMessage(status, '', kind), context.settingsText(`batchWorkflow.${prefix}Completed`, { count: 3 }));
      // A stale skip key must not hide failed libraries in a mixed result.
      status.message_key = `batchWorkflow.${prefix === 'scan' ? 'scan' : 'thumbnail'}Skipped`;
      status.failed_libraries = 1;
      status.completed_libraries = 2;
      const partial = context.libraryBatchMessage(status, '', kind);
      assert.equal(partial, context.settingsText(`batchWorkflow.${prefix}CompletedWithFailures`, { completed: 2, failed: 1 }));
      status.failed_libraries = 0;
      assert.equal(context.libraryBatchMessage(status, '', kind), context.settingsText(status.message_key));
      delete status.message_key;
      status.message_key = 'batchWorkflow.unknownFutureKey';
      assert.ok(context.libraryBatchMessage(status, '', kind));
      for (const state of ['pending', 'cancelled', 'failed', 'completed']) {
        const message = context.libraryBatchMessage({ status: state, message: '后台中文状态' }, '', kind);
        assert.equal(message, context.libraryBatchBuildStatusLabel(state));
        if (locale === 'en-US') assert.doesNotMatch(message, /[\u4e00-\u9fff]/);
      }
      if (locale === 'en-US') assert.doesNotMatch(partial, /[\u4e00-\u9fff]/);
    }
    const diagnostic = 'open /media/library: permission denied';
    assert.ok(context.libraryBatchMessage({ status: 'failed', path: '/media/library', message: diagnostic }).includes(diagnostic));
    assert.ok(context.libraryBatchMessage({ status: 'failed', error: diagnostic }).includes(diagnostic));
    assert.equal(context.libraryBatchMessage({ status: 'idle', message: '后台中文状态' }, 'Localized title'), 'Localized title');
  }
  assert.doesNotMatch(source, /showToast\(state\.libraryBatch(?:Build|ThumbnailBuild)Status\.message/);
  assert.match(source, /libraryBatchMessage\(state\.libraryBatchThumbnailBuildStatus, copyText\('app.libraryBatchThumbnail.completed'\), 'thumbnails'\)/);
});

test('batch workflow English copy includes controls, errors, and thumbnail API status', () => {
  const catalog = JSON.parse(fs.readFileSync(path.join(__dirname, '../web/static/strings/en-US.json'), 'utf8'));
  const batchSource = source.slice(source.indexOf('function libraryBatchBuildStatusLabel'), source.indexOf('function syncLibraryBatchThumbnailBuildPanel'));
  const keys = [...batchSource.matchAll(/copyText\('([^']+)'/g)].map(match => match[1]);
  for (const key of keys) {
    const text = key.split('.').reduce((value, part) => value?.[part], catalog);
    assert.equal(typeof text, 'string', key);
    assert.doesNotMatch(text, /[\u4e00-\u9fff]/, key);
  }
  for (const group of [catalog.app.settings.batchWorkflow, catalog.app.libraryBatchScan, catalog.app.libraryBatchThumbnail, catalog.app.settings.sections.batchWorkflow]) {
    for (const [key, text] of Object.entries(group)) assert.doesNotMatch(text, /[\u4e00-\u9fff]/, key);
  }
  for (const [group, pattern] of [['errors', /^(batch|libraryBuild|videoThumbnailRefresh|thumbnailBuild)/], ['status', /^(noBatch|noThumbnail|noVideoThumbnail|thumbnail|videoThumbnail)/]]) {
    for (const [key, text] of Object.entries(catalog.api[group])) {
      if (pattern.test(key)) assert.doesNotMatch(text, /[\u4e00-\u9fff]/, key);
    }
  }
});

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
  assert.equal(calls[0].complete_exif, false);
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
    copyText: key => ({
      'app.interface.s_83f794b00a': '检查并构建',
      'app.interface.s_073957fea3': '本阶段',
      'app.interface.s_562d1d2725': '总进度',
      'app.interface.s_e7ad34645a': '扫描新增',
      'app.interface.s_60248e225f': '缩略图已存在',
      'app.interface.s_cfc54766b7': '新生成缩略图',
    }[key] || ''),
    state: {
      libraryBatchBuildStatus: { libraries: [{ imported: 12, skipped: 640, failed: 0 }] },
      libraryBatchThumbnailBuildStatus: { libraries: [{ skipped: 600, generated: 40, failed: 0 }] },
    },
    escapeHTML: value => String(value).replaceAll('<', '&lt;'),
  });
  for (const name of ['libraryBatchPhaseLabel', 'libraryBatchActivityLabel']) {
    vm.runInContext(source.match(new RegExp('^function ' + name + '\\([^]*?^}', 'm'))[0], context);
  }
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
