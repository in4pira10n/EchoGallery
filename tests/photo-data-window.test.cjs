const test=require('node:test');
const assert=require('node:assert/strict');
const {create}=require('../web/static/photo-data-window.js');
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
async function until(check){for(let i=0;i<100;i++){if(check())return;await sleep(10)}assert.ok(check(),'condition settled')}
function fixture(overrides={}){
 const disk=new Map();let serial=0;
 const store={key:()=>++serial,write:async rows=>{for(const row of rows)disk.set(row.key,structuredClone(row.value))},read:async keys=>keys.map(key=>structuredClone(disk.get(key))),};
 if(overrides.write){const write=store.write;store.write=async rows=>{await overrides.write(rows);await write(rows)}}
 if(overrides.read)store.read=overrides.read;
 const window={setTimeout,clearTimeout};
 const runtime=create({window,store,limit:32});
 const items=Array.from({length:100},(_,i)=>({id:i,uuid:'p'+i,original_name:'p'+i+'.jpg',path:'/photo/'+i,exif:{model:'Canon'},is_favorite:false}));
 runtime.track(items);runtime.pin([items[99]],32);
 return {runtime,items,disk};
}
test('full records commit before eviction and restore in place with live edits',async t=>{
 const {runtime,items,disk}=fixture();t.after(()=>runtime.dispose());
 await until(()=>runtime.stats().resident===32);
 assert.equal(runtime.stats().archived,68);assert.equal(items[99].exif.model,'Canon');
 const item=items[0];assert.equal(item.path,undefined);assert.ok(disk.size>=68);
 item.is_favorite=true;await Promise.all([runtime.ready([item]),runtime.ready([item])]);
 assert.equal(items[0],item);assert.equal(item.path,'/photo/0');assert.equal(item.exif.model,'Canon');assert.equal(item.is_favorite,true);assert.equal(runtime.stats().reads,1);
 assert.deepEqual(items.map(p=>p.id),Array.from({length:100},(_,i)=>i));
});
test('write failure retains complete records',async t=>{
 const {runtime,items}=fixture({write:()=>{throw new Error('quota')}});t.after(()=>runtime.dispose());
 await until(()=>runtime.stats().disabled);assert.equal(runtime.stats().archived,0);assert.ok(items.every(p=>p.exif.model==='Canon'));
});
test('pinning or editing during commit protects original data',async t=>{
 let release;const writing=new Promise(r=>release=r);
 const {runtime,items}=fixture({write:()=>writing});t.after(()=>runtime.dispose());
 await sleep(50);runtime.pin([items[0]],32);items[1].exif.model='Edited';release();
 await until(()=>runtime.stats().resident===32);
 assert.equal(items[0].path,'/photo/0');await runtime.ready([items[1]]);assert.equal(items[1].exif.model,'Edited');
});
test('identical IDs in different libraries have independent stored records',async t=>{
 const {runtime,items}=fixture();t.after(()=>runtime.dispose());
 const other={...items[0],path:'/other/0'};runtime.track([other,...Array.from({length:64},(_,id)=>({id:1000+id,path:"/filler"}))]);runtime.pin([],32);
 await until(()=>!runtime.full(items[0])&&!runtime.full(other));
 await runtime.ready([items[0],other]);assert.equal(items[0].path,'/photo/0');assert.equal(other.path,'/other/0');
});
