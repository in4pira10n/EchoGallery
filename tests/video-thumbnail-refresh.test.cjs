const assert=require('node:assert/strict');
const fs=require('node:fs');
const test=require('node:test');
const vm=require('node:vm');
const source=fs.readFileSync(require('node:path').join(__dirname,'../web/static/app.js'),'utf8');
test('video thumbnail refresh action is admin-only, localized, and cache-busted',()=>{
 assert.match(source,/isVideoMedia\(photo\) && canWriteMedia\(\) \? \{ role: 'refresh-video-thumbnail'/);
 assert.match(source,/api\.post\(`\/api\/media\/\$\{photo\.id\}\/thumbnail\/refresh`/);
 assert.match(source,/item\.poster_url = url/);
 for(const file of ['zh-CN','en-US']) {
  const catalog=JSON.parse(fs.readFileSync(require('node:path').join(__dirname,`../web/static/strings/${file}.json`),'utf8'));
  assert.ok(catalog.app.interface.refreshVideoThumbnail);
  assert.ok(catalog.app.interface.refreshVideoThumbnailDone);
  assert.ok(catalog.app.interface.refreshVideoThumbnailFailed);
 }
});
test('forced refresh changes every cached photo copy and reloads rendered cards',async()=>{
 const photo={id:7,uuid:'v7',media_kind:'video',poster_url:'/old'};
 const image={}; const record={photo,image}; let reloaded=null,toast='';
 const state={serverSettings:{id:'library'},photos:[photo],favoritePhotos:[],randomAlbumPhotos:[],albumPhotos:[],trashPhotos:[],lightboxPhotos:[],memoryPhotos:[],searchResults:[]};
 const context=vm.createContext({state,mediaLoadRuntime:{library:'library',cards:new Map([[image,record]])},
  activeLibraryIdentity:settings=>settings.id,isVideoMedia:item=>item.media_kind==='video',canWriteMedia:()=>true,
  api:{post:async()=>({uuid:'v7',thumbnail_url:'/new?v=10'})},
  refreshManagedThumbnail:target=>{reloaded=target},showToast:value=>{toast=value},copyText:key=>key,console,
 });
 vm.runInContext(source.match(/^async function refreshVideoThumbnail\([^]*?^}/m)[0],context);
 await context.refreshVideoThumbnail(photo);
 assert.equal(photo.poster_url,'/new?v=10');
 assert.equal(photo.thumbnail_url,'/new?v=10');
 assert.equal(reloaded,image);
 assert.equal(toast,'app.interface.refreshVideoThumbnailDone');
});
