const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname, '../web/static/app.js'), 'utf8');
function extract(name) {
  return source.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'))[0];
}
function harness(fetchPage) {
  const state = {view:'random-album',randomAlbumPhotos:[],randomAlbumHasMore:true,randomAlbumCursor:'',randomAlbumTotal:0};
  const scheduled = [];
  const context = vm.createContext({state, AbortController, console:{error(){}},
    fetchRandomAlbumPage:fetchPage, randomAlbumPageLimit:()=>28,
    updateRandomAlbumLoadMoreUI(){}, appendRandomAlbumGrid:async()=>{},
    randomAlbumWarmCandidates:items=>items, requestVisibleThumbnailWarmup(){},
    maybeLoadMoreImmediately:(id,load,can)=>scheduled.push({load,can}),
    copyText:key=>key,
  });
  vm.runInContext(extract('canLoadMoreRandomAlbum')+'\n'+extract('loadMoreRandomAlbum'),context);
  return {state,context,scheduled};
}
test('sparse pages continue and later pages with omitted totals retain the first total',async()=>{
  const replies=[
    {photos:Array.from({length:13},(_,id)=>({id})),total:4759,has_more:true,next_cursor:'page2'},
    {photos:[],total:0,has_more:true,next_cursor:'page3'},
    {photos:[{id:99}],total:0,has_more:false},
  ];
  const cursors=[];
  const h=harness(async cursor=>{cursors.push(cursor);return replies.shift()});
  for(let i=0;i<3;i++) {
    await h.context.loadMoreRandomAlbum();
    assert.equal(h.state.randomAlbumTotal,4759);
    if(i<2) assert.equal(h.scheduled.at(-1).can(),true);
  }
  assert.deepEqual(cursors,['','page2','page3']);
  assert.equal(h.state.randomAlbumPhotos.length,14);
  assert.equal(h.state.randomAlbumLoaded,true);
});
test('errors preserve cursor and loaded photos and allow an explicit retry',async()=>{
  let fail=true;
  const h=harness(async()=>{if(fail)throw new Error('temporary failure');return {photos:[{id:2}],has_more:false}});
  h.state.randomAlbumPhotos.push({id:1}); h.state.randomAlbumCursor='page2';
  await h.context.loadMoreRandomAlbum();
  assert.equal(h.state.randomAlbumLoadError,true);
  assert.equal(h.state.randomAlbumCursor,'page2');
  assert.equal(h.state.randomAlbumHasMore,true);
  assert.equal(h.context.canLoadMoreRandomAlbum(),false);
  h.state.randomAlbumLoadError=false;fail=false;
  await h.context.loadMoreRandomAlbum();
  assert.equal(h.state.randomAlbumPhotos.length,2);
  assert.equal(h.state.randomAlbumLoaded,true);
});
test('an obsolete response cannot overwrite a new library or clear its active request',async()=>{
  const pending=[];
  const h=harness(()=>new Promise(resolve=>pending.push(resolve)));
  const old=h.context.loadMoreRandomAlbum();
  h.state.randomAlbumAbortController.abort();
  h.state.randomAlbumAbortController=null;
  h.state.randomAlbumLoading=false;
  h.state.randomAlbumPhotos=[];
  const next=h.context.loadMoreRandomAlbum();
  pending[0]({photos:[{id:100}],has_more:false,total:100});await old;
  assert.equal(h.state.randomAlbumLoading,true);
  assert.equal(h.state.randomAlbumPhotos.length,0);
  pending[1]({photos:[{id:200}],has_more:false,total:1});await next;
  assert.equal(h.state.randomAlbumPhotos[0].id,200);
  assert.equal(h.state.randomAlbumTotal,1);
});
test('invalid cursor progression is retryable rather than looping or finishing early',async()=>{
  const h=harness(async()=>({photos:[{id:1}],has_more:true,next_cursor:'same'}));
  h.state.randomAlbumCursor='same';
  await h.context.loadMoreRandomAlbum();
  assert.equal(h.state.randomAlbumLoadError,true);
  assert.equal(h.state.randomAlbumCursor,'same');
  assert.equal(h.state.randomAlbumPhotos.length,0);
});
