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
const app = fs.readFileSync(path.join(__dirname,'../web/static/app.js'),'utf8');
function actual(name) {
  const match = app.match(new RegExp('^(?:async )?function '+name+'\\([^]*?^}', 'm'));
  assert.ok(match,name); return match[0];
}
const helpers = ['isFolderAlbum','normalizeAlbumPath','folderAlbumPath','albumRelativeAddress','albumParentPath',
  'isRootFolderAlbum','compareAlbumsByPath','albumMatchesKindFilter','albumChildrenForParent',
  'albumChildAlbumsForDetail','albumDirectChildCount','albumDirectMediaCount','albumItemCount',
  'buildAlbumChildrenIndex','clearAlbumChildrenIndex','findAlbumByPath','parentAlbumFor','returnToAlbumParent',
  'makeAlbumCard','openAlbumDetail','albumDetailPillContent','renderAlbumDetail','renderAlbumChildGrid',
  'renderAlbums','renderAlbumGrid','ensureAlbumsLoaded','renderAlbumViewModeControl','renderTopbarLeadingGroup','renderTopbarGlassButton',
  'sanitizeInlineSVGMarkup','sanitizeSVGNode','applyDisplaySettings','normalizeHexColor'];
const effects = app.slice(app.indexOf('// Page effects are independent'),app.indexOf('// ── 渲染框架'));
const fixture = `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/style.css"><div id="app"><nav class="nav" id="main-nav"><a class="nav-item active" data-view="albums" data-label="Albums">Albums</a></nav>
<button class="nav-logo nav-logo-floating" id="nav-logo-btn">EG</button><div class="main"><div class="topbar"><span class="topbar-title" id="topbar-title"></span><div class="topbar-leading" id="topbar-leading"></div><div class="topbar-meta" id="topbar-meta"></div><div id="topbar-actions"></div></div><div class="content" id="content"></div></div></div><button class="floating-search-btn" id="floating-search-btn">S</button><div class="lightbox" id="lightbox"></div>
<script src="/effects.js"></script><script>
const $=(s,c=document)=>c.querySelector(s), $$=(s,c=document)=>[...c.querySelectorAll(s)], el=(tag,cls,html='')=>{const n=document.createElement(tag);n.className=cls;n.innerHTML=html;return n};
const icon=sanitizeInlineSVGMarkup(${JSON.stringify(fs.readFileSync(path.join(__dirname,'../web/static/svg/album-bold.svg'),'utf8'))});
const icons=new Proxy({}, {get:()=>icon});
const copyText=k=>k, escapeHTML=v=>String(v??''), normalizeAlbumKindFilter=v=>v, albumDisplayTitle=a=>a.name;
const videoPosterPlaceholder='data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4" fill="teal"/></svg>';
const mediaThumbURLFromUUID=()=>videoPosterPlaceholder;
const state={view:'albums',albumsLoaded:true,albumChildrenIndex:null,albums:[],albumViewMode:'grid',albumKindFilter:'all',serverSettings:{library:'one'},selected:new Set(),libraryBuildStatus:{status:'idle'},mediaKindFilter:'all',albumDetailSort:'name',albumPhotos:[],albumDetailLoadedKey:'',loadMoreObservers:{}};
for(let i=1;i<=120;i++)state.albums.push({id:i,name:'Folder '+i,source_kind:'folder',source_rel_path:'Folder '+i,photo_count:2});
for(let i=1;i<=24;i++)state.albums.push({id:200+i,name:'Child '+i,source_kind:'folder',source_rel_path:'Folder 1/Child '+i,photo_count:2});
state.albums.push({id:500,name:'🐱 русский العربية',source_kind:'folder',source_rel_path:'Folder 1/Child 1/🐱 русский العربية',photo_count:1});
const activeLibraryIdentity=s=>s.library,isTouchLikeDevice=()=>matchMedia('(pointer:coarse)').matches,isPhoneTouchLayout=()=>isTouchLikeDevice()&&!document.documentElement.classList.contains('is-ipad-device');
const canWriteMedia=()=>false,isRootUser=()=>false,isLibraryBuildBlocking=()=>false,smartAlbumDescribeRules=()=>'',navIconMarkup=()=>icon;
const noop=()=>{}, showAlbumContextMenu=noop,addLongPress=noop,setHashView=noop,syncNavActiveView=noop,syncAlbumToolbarLayout=noop,bindAlbumKindFilterControl=noop,bindMediaKindFilterControl=noop,bindSelectionBarHandlers=noop,openCreateAlbumModal=noop,openSmartAlbumModal=noop,renderAlbumDetailBottomActionControls=()=>'',renderSelectionBarMarkup=()=>'',renderAlbumKindFilterControl=()=>'',updateLoadMoreUI=noop,observeLoadMore=noop,maybeLoadMoreImmediately=noop,requestVisibleThumbnailWarmup=noop,focusPendingAlbumPhoto=async()=>{},renderAlbumGroups=noop,loadMoreAlbumBefore=noop,albumUsesManualBeforeLoading=()=>false;
const disconnectLoadMoreObserver=noop;
let backCalls=0;
function switchView(view){state.view=view;return renderAlbums()}
async function loadMoreAlbumPhotos(){document.querySelector('#album-groups').innerHTML='<div class="photo-grid"><div class="photo-thumb" style="height:220px"><img src="'+videoPosterPlaceholder+'"></div></div>';state.albumHasMore=false}
${helpers.map(actual).join('\n')}
${effects}
renderAlbums();initPageEffects();
window.errors=[];window.addEventListener('error',e=>window.errors.push(e.message));
function pointer(type,x,y=350,id=1){document.querySelector('#content').dispatchEvent(new PointerEvent(type,{bubbles:true,cancelable:true,pointerType:'touch',isPrimary:true,pointerId:id,clientX:x,clientY:y}));}
function previewSVGReferencesValid(){
  const layer=document.querySelector('.page-swipe-preview');
  const ids=[...layer.querySelectorAll('[id]')].map(node=>node.id);
  if(!ids.length||new Set(ids).size!==ids.length)return false;
  return [...layer.querySelectorAll('svg')].every(svg=>{
    const localIDs=new Set([...svg.querySelectorAll('[id]')].map(node=>node.id));
    return [...svg.querySelectorAll('*')].every(node=>[...node.attributes].every(attr=>{
      const refs=[...attr.value.matchAll(/url\\(["']?(?:[^#]*#)([^)"']+)["']?\\)/g)];
      return refs.every(match=>localIDs.has(match[1]));
    }));
  });
}
</script>`;

test('real browser: measured fades, phone parent swipe, cancellation, handoff and bounded previews',
 {skip:!fs.existsSync(chrome),timeout:60000}, async t=>{
  const profile=fs.mkdtempSync(path.join(os.tmpdir(),'eg-effects-'));
  const server=http.createServer((req,res)=>{
    if(req.url==='/effects.js'){res.setHeader('Content-Type','text/javascript');res.end(fs.readFileSync(path.join(__dirname,'../web/static/page-effects.js')))}
    else if(req.url==='/style.css'){res.setHeader('Content-Type','text/css');res.end(fs.readFileSync(path.join(__dirname,'../web/pages/app.css')))}
    else {res.setHeader('Content-Type','text/html');res.end(fixture)}
  });
  await new Promise(r=>server.listen(0,'127.0.0.1',r));
  const child=spawn(chrome,['--headless=new','--no-first-run','--disable-gpu','--disable-background-networking','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{stdio:'ignore'});
  let socket;
  t.after(async()=>{socket?.close();server.closeAllConnections();server.close();const exited=once(child,'exit');child.kill();await Promise.race([exited,sleep(2000)]);if(child.exitCode==null&&child.signalCode==null)child.kill('SIGKILL');fs.rmSync(profile,{recursive:true,force:true,maxRetries:3,retryDelay:100})});
  const file=path.join(profile,'DevToolsActivePort');
  for(let i=0;i<100&&!fs.existsSync(file);i++)await sleep(50);
  assert.ok(fs.existsSync(file));
  const port=fs.readFileSync(file,'utf8').split('\n')[0];
  const target=await(await fetch('http://127.0.0.1:'+port+'/json/new',{method:'PUT'})).json();
  socket=new WebSocket(target.webSocketDebuggerUrl);await once(socket,'open');
  let serial=0;const pending=new Map();
  socket.addEventListener('message',event=>{const r=JSON.parse(event.data);if(r.id){const p=pending.get(r.id);pending.delete(r.id);r.error?p.reject(r.error):p.resolve(r.result)}});
  const send=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial;pending.set(id,{resolve,reject});socket.send(JSON.stringify({id,method,params}))});
  const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});assert.equal(r.exceptionDetails,undefined,JSON.stringify(r.exceptionDetails));return r.result.value};
  const settle=async()=>{await sleep(650);await evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))')};
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true,screenWidth:390,screenHeight:844});
  await send('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:5});
  await send('Page.navigate',{url:'http://127.0.0.1:'+server.address().port});
  for(let i=0;i<100;i++){if(await evaluate('typeof albumSwipeBack!=="undefined"&&!!albumSwipeBack'))break;await sleep(30)}
  assert.deepEqual(await evaluate('window.errors'),[]);
  assert.equal(await evaluate('phonePageEffectsEnabled()'),true);
  await sleep(250);
  await evaluate('window.fadeMeasurements=0;const bounds=pageEdgeFades.element.getBoundingClientRect.bind(pageEdgeFades.element);pageEdgeFades.element.getBoundingClientRect=()=>{fadeMeasurements++;return bounds()}');
  await sleep(180);
  assert.ok(await evaluate('fadeMeasurements<=1'),'unchanged page fades do not continuously measure or schedule frames');
  const fade=await evaluate(`(()=>{pageEdgeFades.measure();const layer=pageEdgeFades.element;return {top:parseFloat(layer.style.getPropertyValue('--page-fade-top')),bottom:parseFloat(layer.style.getPropertyValue('--page-fade-bottom')),logo:document.querySelector('#nav-logo-btn').getBoundingClientRect().bottom,nav:innerHeight-document.querySelector('#main-nav').getBoundingClientRect().top,z:Number(getComputedStyle(layer).zIndex),buttonZ:Number(getComputedStyle(document.querySelector('#nav-logo-btn')).zIndex),color:getComputedStyle(layer).getPropertyValue('--page-fade-color').trim()}})()`);
  assert.ok(fade.top>=fade.logo+50);assert.ok(fade.bottom>=fade.nav+50);assert.ok(fade.z<fade.buttonZ);assert.equal(fade.color,'#f7f4ef');
  await evaluate("document.documentElement.dataset.theme='dark';pageEdgeFades.measure()");
  assert.equal(await evaluate("getComputedStyle(pageEdgeFades.element).getPropertyValue('--page-fade-color').trim()"),'#1a1a1a');
  await evaluate('state.serverSettings.progressive_linear_layer_enabled=false;applyDisplaySettings()');
  assert.equal(await evaluate('getComputedStyle(pageEdgeFades.element).display'),'none');
  await evaluate('state.serverSettings.progressive_linear_layer_enabled=true;state.serverSettings.progressive_linear_layer_color="#336699";state.serverSettings.progressive_linear_layer_dark_color="#112233";applyDisplaySettings();pageEdgeFades.measure()');
  assert.equal(await evaluate('getComputedStyle(pageEdgeFades.element).display'),'block');
  for (const edge of ['::before','::after']) {
    assert.match(await evaluate(`getComputedStyle(pageEdgeFades.element,${JSON.stringify(edge)}).backgroundImage`),/rgb\(17, 34, 51\)/);
  }
  await evaluate('document.documentElement.dataset.theme="light"');
  assert.equal(await evaluate("getComputedStyle(pageEdgeFades.element).getPropertyValue('--page-fade-color').trim()"),'#336699');
  await evaluate('document.documentElement.dataset.theme="dark"');
  assert.equal(await evaluate("getComputedStyle(pageEdgeFades.element).getPropertyValue('--page-fade-color').trim()"),'#112233');
  await evaluate('state.serverSettings.progressive_linear_layer_color="";state.serverSettings.progressive_linear_layer_dark_color="";applyDisplaySettings()');
  assert.equal(await evaluate("getComputedStyle(pageEdgeFades.element).getPropertyValue('--page-fade-color').trim()"),'#1a1a1a');
  const colorSettings=app.slice(app.indexOf("      ${renderSettingsToggle('settings-progressive-linear-layer'"),app.indexOf("      ${renderSettingsToggle('settings-warm-enabled'"));
  assert.equal(await evaluate(`(()=>{
    const settingsText=key=>key;
    ${actual('renderSettingsToggle')}
    const panel=document.createElement('section');panel.className='card settings-panel';
    panel.innerHTML=\`<div class="settings-group">${colorSettings}</div>\`;document.body.append(panel);
    const button=panel.querySelector('#settings-progressive-linear-layer-theme');
    const fullWidth=Math.abs(button.getBoundingClientRect().width-button.parentElement.getBoundingClientRect().width)<1;
    panel.remove();return fullWidth;
  })()`),true,'Follow theme fills its settings row');
  await evaluate("document.documentElement.dataset.theme='';scrollTo(0,500);openAlbumDetail(state.albums[0])");
  await evaluate('scrollTo(0,300);openAlbumDetail(state.albums.find(a=>a.id===201))');
  await evaluate('pointer("pointerdown",30);pointer("pointermove",120)');
  assert.equal(await evaluate('document.documentElement.classList.contains("page-back-swiping")'),true);
  assert.equal(await evaluate('document.querySelector(".page-swipe-foreground").style.transform'),'translate3d(90px, 0px, 0px)');
  const preview=await evaluate('({cards:document.querySelectorAll(".page-swipe-preview .album-card").length,nav:!!document.querySelector(".page-swipe-preview .nav"),logo:!!document.querySelector(".page-swipe-preview .nav-logo-floating"),fades:!!document.querySelector(".page-swipe-preview .page-edge-fades"),pill:!!document.querySelector(".page-swipe-preview .album-detail-pill"),ids:[...document.querySelectorAll(".page-swipe-preview [id]")].filter(node=>!node.closest("svg")).length})');
  if (process.env.EG_EFFECTS_SCREENSHOT) { const shot=await send('Page.captureScreenshot',{format:'png'});fs.writeFileSync(process.env.EG_EFFECTS_SCREENSHOT,Buffer.from(shot.data,'base64')); }
  assert.ok(preview.cards>0&&preview.cards<24);assert.equal(preview.nav,true);assert.equal(preview.logo,true);assert.equal(preview.fades,true);assert.equal(preview.pill,true);assert.equal(preview.ids,0);
  assert.equal(await evaluate('previewSVGReferencesValid()'),true,'cached preview retains independent SVG clips');
  await sleep(130);await evaluate('pointer("pointerup",120)');await settle();
  assert.equal(await evaluate('state.currentAlbumID'),201);assert.equal(await evaluate('document.querySelectorAll(".page-swipe-preview,.page-swipe-foreground").length'),0);
  // Grab a cancellation spring at its current presentation value, then commit.
  await evaluate('pointer("pointerdown",30);pointer("pointermove",90);pointer("pointerup",90)');
  await sleep(32);const beforeGrab=await evaluate('document.querySelector(".page-swipe-foreground")?.style.transform');
  assert.ok(beforeGrab);
  await evaluate('pointer("pointerdown",35);pointer("pointermove",250);pointer("pointerup",250)');await settle();
  assert.equal(await evaluate('state.currentAlbumID'),1);assert.equal(await evaluate('Math.round(scrollY)'),300);
  assert.equal(await evaluate('document.querySelectorAll(".page-swipe-preview,.page-swipe-foreground").length'),0);
  await evaluate('pointer("pointerdown",30);pointer("pointermove",250);pointer("pointerup",250)');await settle();
  assert.equal(await evaluate('state.view'),'albums');assert.equal(await evaluate('Math.round(scrollY)'),500);
  // Direct-link destination is synthesized without navigating during drag.
  await evaluate('openAlbumDetail(state.albums.find(a=>a.id===500));albumBackPreviews.clear();pointer("pointerdown",30);pointer("pointermove",150)');
  assert.equal(await evaluate('state.currentAlbumID'),500);
  assert.ok(await evaluate('document.querySelectorAll(".page-swipe-preview .album-card").length>0'));
  assert.equal(await evaluate('previewSVGReferencesValid()'),true,'fallback preview retains independent SVG clips');
  await evaluate('pointer("pointercancel",150)');await settle();
  assert.equal(await evaluate('state.currentAlbumID'),500);
  // Lightbox, iPad and desktop never acquire this gesture.
  for(const setup of ['document.querySelector("#lightbox").classList.add("open")','document.documentElement.classList.add("is-ipad-device")']){
    await evaluate(setup+';pointer("pointerdown",30);pointer("pointermove",250)');
    assert.equal(await evaluate('document.querySelector(".page-swipe-foreground")===null'),true);
    await evaluate('pointer("pointerup",250);document.querySelector("#lightbox").classList.remove("open");document.documentElement.classList.remove("is-ipad-device")');
  }
  // Real browser touch arbitration (not only synthetic PointerEvents).
  await send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:30,y:350}]});
  await send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:130,y:351}]});
  assert.equal(await evaluate('document.documentElement.classList.contains("page-back-swiping")'),true);
  await send('Input.dispatchTouchEvent',{type:'touchCancel',touchPoints:[]});await settle();
  assert.equal(await evaluate('state.currentAlbumID'),500);
  // Reduced-motion handoff still navigates and leaves no retained overlay.
  await send('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});
  await evaluate('pointer("pointerdown",30);pointer("pointermove",250);pointer("pointerup",250)');await settle();
  assert.equal(await evaluate('state.currentAlbumID'),201);
  assert.equal(await evaluate('document.querySelectorAll(".page-swipe-preview,.page-swipe-foreground").length'),0);
  await send('Emulation.setTouchEmulationEnabled',{enabled:false});
  await evaluate('pointer("pointerdown",30);pointer("pointermove",250)');
  assert.equal(await evaluate('document.querySelector(".page-swipe-foreground")===null'),true);
  assert.deepEqual(await evaluate('window.errors'),[]);
 });
