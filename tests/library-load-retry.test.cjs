const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname,'../web/static/app.js'),'utf8');
const extract = name => source.match(new RegExp('^(?:async )?function '+name+'\\([^]*?^}', 'm'))[0];

test('cold library settings and foreground pages have bounded time to finish on a slow disk',async()=>{
  const delays=[];
  const context=vm.createContext({AbortController,copyText:key=>key,
    setTimeout:(fn,delay)=>{delays.push(delay);return 1},clearTimeout(){},
    api:{get:async()=>({active_library_id:'one'})},state:{},
    logLibrarySwitch(){},applyServerSettings(){},cloneSettingsSnapshot:value=>({...value}),
  });
  vm.runInContext(extract('getWithLoadTimeout')+'\n'+extract('loadServerSettings'),context);
  await context.getWithLoadTimeout('/api/media');
  await context.loadServerSettings();
  assert.deepEqual(delays,[60000,120000]);
});

test('timeline page failure preserves the cursor and existing cards for retry',async()=>{
  const state={view:'timeline',photos:[{id:1}],timelineHasMore:true,timelineCursor:'page2',timelineTotal:4759};
  const sentinel={innerHTML:''};let retry,fail=true;
  const context=vm.createContext({state,AbortController,URLSearchParams,
    $:()=>sentinel,console:{error(){}},copyText:key=>key,escapeHTML:value=>value,
    computeTimelinePageSize:()=>30,appendMediaKindParam(){},
    getWithLoadTimeout:async()=>{if(fail)throw new Error('temporary failure');return {photos:[{id:2}],next_cursor:'page3',has_more:true,total:0}},
    appendTimelineGrid(){},requestVisibleThumbnailWarmup(){},
    renderLibraryLoadError:(target,callback)=>{retry=callback},showToast(){},updateLoadMoreUI(){},maybeLoadMoreImmediately(){},
  });
  vm.runInContext(extract('loadMoreTimeline'),context);
  await context.loadMoreTimeline();
  assert.equal(state.timelineCursor,'page2');
  assert.equal(state.timelineHasMore,true);
  assert.equal(state.timelineAutoLoadPaused,true);
  assert.equal(state.photos.length,1);
  fail=false;retry();
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(state.timelineAutoLoadPaused,false);
  assert.equal(state.timelineCursor,'page3');
  assert.equal(state.timelineTotal,4759);
  assert.equal(state.photos.length,2);
});
