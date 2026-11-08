const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const http = require('node:http');
const { spawn } = require('node:child_process');
const { once } = require('node:events');
const chrome = process.env.EG_TEST_CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const app = fs.readFileSync(path.join(__dirname, '../web/static/app.js'), 'utf8');
function actual(name) {
  const match = app.match(new RegExp('^(?:async )?function ' + name + '\\([^]*?^}', 'm'));
  assert.ok(match, name); return match[0];
}
const fixture = `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/style.css"><style>body{margin:0}header{height:120px}.photo-grid{--grid-size:180px;--grid-gap:2px} .photo-thumb{background:#375c4d}</style>
<header></header><main id="timeline-wrap"></main><footer style="height:80px"></footer><div id="lightbox"></div>
<script src="/wall.js"></script><script>
const $ = s => document.querySelector(s), $$ = (s,scope=document) => Array.from(scope.querySelectorAll(s));
const el = (tag,cls) => {const n=document.createElement(tag);n.className=cls;return n};
const copyText = key => key, escapeHTML = value => value, icons = {photo:''};
let photoWallRuntime=null, released=0;
const mediaLoadRuntime={cards:new Map(),releaseCard(){released++}};
const state={view:'timeline',photos:Array.from({length:10000},(_,i)=>({id:i+1,uuid:'p'+(i+1)})),timelineTotal:10000,
timelineHasMore:false,timelineLoading:false,serverSettings:{library:'one'},selected:new Set([9000]),focusedPhotoID:null,
timelineLocateMissingBeforeCount:0};
const activeLibraryIdentity = settings => settings.library;
const mediaKindMatchesFilter=()=>true, updateLoadMoreUI=()=>{};
const updateSelectionModeUI=()=>{}, updateSelectionBar=()=>{}, updateTrashSelBar=()=>{};
const updateTrashFolderSelectionBar=()=>{};
const albumDirectChildCount=()=>0, normalizeTrashContentFilter=value=>value;
function makePhotoThumb(photo,list){const n=el('div','photo-thumb');n.dataset.id=photo.id;
n.onclick=()=>{window.clickedIndex=list.indexOf(photo)};
n.innerHTML='<img alt=""><span class="photo-thumb-name">'+photo.id+'</span>';
if(state.selected.has(photo.id))n.classList.add('selected');
if(state.focusedPhotoID===photo.id)n.classList.add('keyboard-focused');
mediaLoadRuntime.cards.set(n.querySelector('img'),{});return n}
${['setPhotoWallGrid','makeLocatePlaceholder','timelinePlaceholderCount','appendTimelineGrid',
  'prependTimelineGrid','renderTimelineGrid','findPhotoThumb','visiblePhotoThumbs','currentViewPhotos',
  'setFocusedPhoto','focusPhotoByGridStep','selectAllInGroup','appendFavoriteGrid','renderMemoryGroups',
  'albumPlaceholderCount','renderAlbumGroups','renderTrashGroups','appendRandomAlbumGrid',
  'lockPageScrollForLightbox','unlockPageScrollForLightbox'].map(actual).join('\n')}
function snapshot(){const grid=$('#timeline-grid');const nodes=$$('.photo-thumb[data-id]');
const visible=nodes.filter(n=>{const r=n.getBoundingClientRect();return r.bottom>0&&r.top<innerHeight});
return {stats:photoWallRuntime.stats(),height:grid.getBoundingClientRect().height,scrollY,
lead:visible[0]?{id:Number(visible[0].dataset.id),top:visible[0].getBoundingClientRect().top}:null,
mounted:nodes.length,resources:mediaLoadRuntime.cards.size,released};}
renderTimelineGrid();
</script>`;

test('real browser: long scrolling, pagination, recycling, resize anchors and offscreen focus',
  { skip: !fs.existsSync(chrome), timeout: 30000 }, async t => {
    const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'eg-wall-browser-'));
    const server = http.createServer((request, response) => {
      if (request.url === '/wall.js') { response.setHeader('Content-Type','text/javascript'); response.end(fs.readFileSync(path.join(__dirname,'../web/static/photo-wall.js'))); }
      else if (request.url === '/style.css') { response.setHeader('Content-Type','text/css'); response.end(fs.readFileSync(path.join(__dirname,'../web/pages/app.css'))); }
      else { response.setHeader('Content-Type','text/html'); response.end(fixture); }
    });
    await new Promise(resolve => server.listen(0,'127.0.0.1',resolve));
    const child = spawn(chrome, ['--headless=new','--no-first-run','--disable-gpu',
      '--disable-background-networking','--remote-debugging-port=0',`--user-data-dir=${profile}`,'about:blank'], {stdio:'ignore'});
    let socket;
    t.after(async () => {
      socket?.close(); server.closeAllConnections(); server.close();
      const exited = once(child, 'exit'); child.kill();
      await Promise.race([exited, sleep(2000)]);
      if (child.exitCode == null && child.signalCode == null) child.kill('SIGKILL');
      fs.rmSync(profile,{recursive:true,force:true,maxRetries:3,retryDelay:100});
    });
    const portFile = path.join(profile,'DevToolsActivePort');
    for(let i=0;i<100&&!fs.existsSync(portFile);i++) await sleep(50);
    assert.ok(fs.existsSync(portFile),'headless Chrome started');
    const port = fs.readFileSync(portFile,'utf8').split('\n')[0];
    const target = await (await fetch(`http://127.0.0.1:${port}/json/new`,{method:'PUT'})).json();
    socket = new WebSocket(target.webSocketDebuggerUrl); await once(socket,'open');
    let serial=0; const pending=new Map();
    socket.addEventListener('message',event=>{const result=JSON.parse(event.data);if(result.id){const p=pending.get(result.id);pending.delete(result.id);result.error?p.reject(result.error):p.resolve(result.result)}});
    const send = (method,params={})=>new Promise((resolve,reject)=>{const id=++serial;pending.set(id,{resolve,reject});socket.send(JSON.stringify({id,method,params}))});
    const evaluate = async expression => {
      const result=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});
      assert.equal(result.exceptionDetails,undefined,JSON.stringify(result.exceptionDetails));return result.result.value;
    };
    const settle = async () => { await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))'); await sleep(220); };
    await send('Emulation.setDeviceMetricsOverride',{width:1200,height:800,deviceScaleFactor:1,mobile:false});
    await send('Page.navigate',{url:`http://127.0.0.1:${server.address().port}`});
    for(let i=0;i<100;i++){if(await evaluate('typeof photoWallRuntime!=="undefined"&&!!photoWallRuntime'))break;await sleep(25)}
    await settle();
    const initial=await evaluate('snapshot()');
    assert.ok(initial.mounted<200); assert.equal(initial.resources,initial.mounted);
    for(const fraction of [.2,.8,.1,.99,.4,0]) {
      await evaluate(`scrollTo(0,${initial.height*fraction})`);await settle();
      const current=await evaluate('snapshot()');
      assert.ok(current.lead,'viewport is covered after long-distance scrolling');
      assert.ok(Math.abs(current.height-initial.height)<.5);assert.ok(current.mounted<=current.stats.grids[0].budget);
      assert.equal(current.resources,current.mounted,'recycled cards release media scheduler records');
    }
    await evaluate('scrollTo(0,6000);photoWallRuntime.relayout();scrollTo(0,0)');await settle();
    assert.equal(await evaluate('scrollY'),0,'a pending layout anchor must not pull the wall away from the top');
    await evaluate('scrollTo(0,6000);photoWallRuntime.relayout();scrollTo(0,99999999)');await settle();
    const bottom=await evaluate('({top:scrollY,max:Math.max(document.documentElement.scrollHeight,document.body.scrollHeight)-innerHeight})');
    assert.ok(bottom.max-bottom.top<=1,'a pending layout anchor must not pull the wall away from the bottom');
    await evaluate('scrollTo(0,6000)');await settle();
    const inertia=await evaluate(`(async()=>{
      document.dispatchEvent(new Event('touchstart'));
      document.dispatchEvent(new Event('touchend'));
      scrollBy(0,80);
      await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
      state.photos.push({id:30001,uuid:'inertia'});appendTimelineGrid([]);
      const start=scrollY;
      for(let i=1;i<=20;i++){
        scrollTo(0,start+i*80);
        await new Promise(r=>setTimeout(r,80));
      }
      return {expected:start+1600};
    })()`);await settle();
    assert.ok(Math.abs((await evaluate('scrollY'))-inertia.expected)<1,'long touch inertia must not replay an old pagination anchor');
    await evaluate('scrollTo(0,80000)');await settle();
    const before=await evaluate('snapshot()');
    await evaluate('window.retainedCard=findPhotoThumb(snapshot().lead.id)');
    await evaluate('state.photos.push(...Array.from({length:90},(_,i)=>({id:10001+i,uuid:"a"+i})));appendTimelineGrid(state.photos.slice(-90))');await settle();
    assert.deepEqual((await evaluate('snapshot()')).lead,before.lead,'append preserves visible anchor');
    assert.equal(await evaluate(`retainedCard===findPhotoThumb(${before.lead.id})`),true,'unchanged media retain their DOM identity');
    await evaluate('state.photos=[...state.photos];appendTimelineGrid([])');await settle();
    assert.equal(await evaluate(`retainedCard===findPhotoThumb(${before.lead.id})`),true,'replacing a page array preserves stable media keys');
    await evaluate('retainedCard.click()');
    assert.equal(await evaluate('clickedIndex'),await evaluate(`state.photos.findIndex(p=>p.id===${before.lead.id})`),'retained card opens the correct media index');
    await evaluate('state.photos.splice(0,30);photoWallRuntime.remove(Array.from({length:30},(_,i)=>i+1))');await settle();
    const deletedAnchor=(await evaluate('snapshot()')).lead;
    assert.equal(deletedAnchor.id,before.lead.id);
    assert.ok(Math.abs(deletedAnchor.top-before.lead.top)<1,'deleting earlier rows preserves visible anchor');
    await evaluate('state.timelineLocateMissingBeforeCount=50;appendTimelineGrid([])');await settle();
    const placeholderAnchor=(await evaluate('snapshot()')).lead;
    await evaluate('const older=Array.from({length:5},(_,i)=>({id:20000+i,uuid:"b"+i}));state.photos.unshift(...older);prependTimelineGrid(older)');await settle();
    const replacedAnchor=(await evaluate('snapshot()')).lead;
    assert.equal(replacedAnchor.id,placeholderAnchor.id);
    assert.ok(Math.abs(replacedAnchor.top-placeholderAnchor.top)<1,'replacing earlier placeholders preserves visible anchor');
    const anchor=await evaluate('snapshot().lead');
    await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:2,mobile:true});await settle();
    const resized=await evaluate(`({top:findPhotoThumb(${anchor.id}).getBoundingClientRect().top,stats:photoWallRuntime.stats()})`);
    assert.ok(Math.abs(resized.top-anchor.top)<1,`resize retains the media anchor offset: ${JSON.stringify({anchor,resized,snapshot:await evaluate('snapshot()')})}`);
    const beforeSize=await evaluate('snapshot().lead');
    await evaluate('document.querySelector(".photo-grid").style.setProperty("--grid-size","96px");photoWallRuntime.relayout()');await settle();
    assert.ok(Math.abs((await evaluate(`findPhotoThumb(${beforeSize.id}).getBoundingClientRect().top`))-beforeSize.top)<1);
    await evaluate('const n=findPhotoThumb(9000);n.scrollIntoView({block:"center"});setFocusedPhoto(9000,{scroll:false})');await settle();
    assert.equal(await evaluate('findPhotoThumb(9000).classList.contains("selected")'),true);
    await evaluate('focusPhotoByGridStep("right")');await settle();
    assert.equal(await evaluate('state.focusedPhotoID'),9001);
    assert.ok(await evaluate('!!findPhotoThumb(9001)'));
    await evaluate('state.trashFolderSelection=new Set();selectAllInGroup(document.querySelector("#timeline-grid"))');
    assert.equal(await evaluate('state.selected.size'),await evaluate('state.photos.length'),'selection includes unmounted media');
    await evaluate('document.documentElement.classList.add("fixed-touch-grid");document.documentElement.style.setProperty("--photo-grid-columns","3");photoWallRuntime.relayout()');await settle();
    await evaluate('scrollTo(0,20000)');await settle();
    const portrait=await evaluate('snapshot()');
    await evaluate('document.documentElement.style.setProperty("--photo-grid-columns","7");photoWallRuntime.relayout()');await settle();
    const afterColumns=await evaluate(`({top:findPhotoThumb(${portrait.lead.id}).getBoundingClientRect().top,snapshot:snapshot()})`);
    assert.ok(Math.abs(afterColumns.top-portrait.lead.top)<1,`column changes preserve media position: ${JSON.stringify({portrait,afterColumns})}`);
    const touchAnchor=await evaluate('snapshot().lead');
    const touchScroll=await evaluate('scrollY');
    await evaluate('document.dispatchEvent(new Event("touchstart"));document.documentElement.style.setProperty("--photo-grid-columns","3");photoWallRuntime.relayout()');await settle();
    assert.equal(await evaluate('scrollY'),touchScroll,'layout compensation waits while a finger is down');
    await evaluate('document.dispatchEvent(new Event("touchend"))');await settle();
    assert.ok(Math.abs((await evaluate(`findPhotoThumb(${touchAnchor.id}).getBoundingClientRect().top`))-touchAnchor.top)<1,'touch release restores the pending anchor');
    t.diagnostic(JSON.stringify({initialMounted:initial.mounted,total:10000,final:await evaluate('snapshot()')}));
    await evaluate('document.querySelector("#timeline-wrap").innerHTML=""');await settle();
    assert.equal(await evaluate('photoWallRuntime.stats().grids.length'),0);
    assert.equal(await evaluate('mediaLoadRuntime.cards.size'),0);
    for (const [view,host,field,render] of [
      ['favorites','favorite-wrap','favoritePhotos','appendFavoriteGrid(state.favoritePhotos)'],
      ['memories','memory-groups','memoryPhotos','renderMemoryGroups()'],
      ['album-detail','album-groups','albumPhotos','renderAlbumGroups(state.albumPhotos)'],
      ['trash','trash-wrap','trashPhotos','renderTrashGroups(state.trashPhotos)'],
      ['random-album','random-album-wrap','randomAlbumPhotos','appendRandomAlbumGrid(state.randomAlbumPhotos)'],
    ]) {
      await evaluate(`scrollTo(0,0);state.view=${JSON.stringify(view)};state[${JSON.stringify(field)}]=state.photos;
        state.trashContentFilter='media';state.trashLoaded=true;state.trashAlbums=[];
        document.querySelector('main').id=${JSON.stringify(host)};${render}`);await settle();
      const stats=await evaluate('photoWallRuntime.stats()');
      assert.equal(stats.grids.length,1,view);
      assert.ok(stats.grids[0].mounted>0&&stats.grids[0].mounted<200,view);
      if(view==='random-album') {
        await evaluate('window.oldLibraryCard=document.querySelector(".photo-thumb");state.serverSettings.library="two";appendRandomAlbumGrid([])');await settle();
        assert.equal(await evaluate('oldLibraryCard.isConnected'),false,'library changes discard old cards even when media IDs match');
        assert.equal(await evaluate('mediaLoadRuntime.cards.size'),(await evaluate('photoWallRuntime.stats()')).grids[0].mounted);
      }
      await evaluate('document.querySelector("main").innerHTML=""');await settle();
      assert.equal(await evaluate('mediaLoadRuntime.cards.size'),0,view);
    }
    // Start a fresh runtime with coarse-pointer detection enabled. Desktop
    // wheel tests above deliberately exercise the separate desktop path.
    await send('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:5});
    await send('Page.navigate',{url:`http://127.0.0.1:${server.address().port}`});
    for(let i=0;i<100;i++){if(await evaluate('typeof photoWallRuntime!=="undefined"&&!!photoWallRuntime'))break;await sleep(25)}
    await settle();
    assert.equal(await evaluate('matchMedia("(pointer: coarse)").matches && navigator.maxTouchPoints > 0'),true);
    await evaluate('scrollTo(0,20000)');await settle();
    const interrupted = await evaluate(`(async()=>{
      window.corrections=[];
      const nativeScrollBy=window.scrollBy.bind(window);
      window.scrollBy=(...args)=>{corrections.push(args);nativeScrollBy(...args)};
      document.dispatchEvent(new Event('touchstart'));
      document.querySelector('.photo-grid').style.setProperty('--grid-size','96px');
      photoWallRuntime.relayout();
      await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
      document.dispatchEvent(new Event('touchend'));
      scrollTo(0,scrollY+160);
      await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
      // A second touch interrupts inertia before layout compensation can run.
      document.dispatchEvent(new Event('touchstart'));
      const stopped=scrollY;
      state.photos.push({id:40001,uuid:'interrupted'});appendTimelineGrid([]);
      document.dispatchEvent(new Event('touchend'));
      // Height-only browser chrome events must not revive the old anchor.
      window.dispatchEvent(new Event('resize'));
      visualViewport.dispatchEvent(new Event('resize'));
      await new Promise(r=>setTimeout(r,700));
      return {stopped,actual:scrollY,corrections,stats:photoWallRuntime.stats()};
    })()`);
    assert.equal(interrupted.actual,interrupted.stopped,'interrupting inertia keeps the position chosen by the user');
    assert.deepEqual(interrupted.corrections,[],'second touch invalidates deferred anchor corrections');
    assert.ok(interrupted.stats.grids[0].mounted<=interrupted.stats.grids[0].budget,'DOM pruning resumes after the interrupted gesture settles');
    const mobileAnchor=await evaluate('snapshot().lead');
    await evaluate('document.dispatchEvent(new Event("touchstart"));document.querySelector(".photo-grid").style.setProperty("--grid-size","140px");photoWallRuntime.relayout();document.dispatchEvent(new Event("touchend"))');
    await sleep(700);
    assert.ok(Math.abs((await evaluate(`findPhotoThumb(${mobileAnchor.id}).getBoundingClientRect().top`))-mobileAnchor.top)<1,'a new mobile layout change still preserves its media anchor after settling');
  });
