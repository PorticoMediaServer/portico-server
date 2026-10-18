import {wireFixture} from './wire-fixture.ts';
import test from 'node:test';
import assert from 'node:assert/strict';
import {PersonalSavedService,makeSavedDefinition,savedTitlePrefix,savedCategoryId,savedViewBrowse,savedChange,retryUnconfirmed,UNCONFIRMED} from '../src/personal-saved.ts';
const scope={serverId:'server',viewerId:'["local","account","profile"]'};
const media=(id='item')=>({id,libraryId:'library',kind:'movie',title:'Movie',available:true,playback:{itemId:id,startSeconds:0}});
const resource=(patch:any={})=>({serverId:'server',viewerFence:'fence',id:'collection',kind:'collection',name:'Collection',summary:'',visibility:'private',revision:1,role:'owner',entryCount:0,actions:['pin','update','delete','entries','share'],status:'ready',invalidComponents:[],pinned:false,pinRevision:0,shares:[],...patch});
const directory=(resources:any[]=[])=>({serverId:'server',viewerFence:'fence',revision:1,resources,nextCursor:''});
const content=(r=resource(),entries:any[]=[])=>({resource:r,entries,nextCursor:''});
const deferred=()=>{let resolve!:(v:any)=>void;const promise=new Promise<any>(r=>resolve=r);return{promise,resolve};};
const tick=()=>new Promise(r=>setTimeout(r,0));
function setup(request:(path:string,method?:string,body?:any,signal?:AbortSignal)=>Promise<any>){let n=0;return new PersonalSavedService({request:async (...args:any[])=>wireFixture(await (request as any)(...args),{'duration': 'durationMs', 'progressSeconds': 'progressMs', 'startSeconds': 'startMs', 'positionSeconds': 'positionMs'})},scope,async()=>`00000000-0000-4000-8000-${String(++n).padStart(12,'0')}`);}

test('collection directory and opaque membership use exact server IDs, roles and page positions',async()=>{
 const paths:string[]=[];const s=setup(async p=>{paths.push(p);return p.includes('/content')?{...content(resource(),[{id:'member',hidden:false,media:media()},{id:'hidden',hidden:true}]),nextCursor:p.includes('cursor')?'':'next'}:directory([resource()]);});
 await s.select({view:'collections'});assert.equal(s.getSnapshot().resources[0].id,'collection');
 await s.select({view:'resource',resourceId:'collection'});assert.deepEqual(s.getSnapshot().entries.map(e=>e.id),['member','hidden']);
 await s.next();assert.equal(s.getSnapshot().cursor,'next');assert.deepEqual(s.getSnapshot().history,[null]);await s.previous();assert.equal(s.getSnapshot().cursor,null);assert(paths.some(p=>p.includes('cursor=next')));assert(Object.isFrozen(s.getSnapshot().entries));s.dispose();
});
test('collection network retry preserves exact operation and original CAS revision',async()=>{
 const writes:any[]=[];let revision=1;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({revision}));writes.push(b);if(writes.length===1)throw new Error('lost response');revision=2;return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false}};});
 await s.select({view:'resource',resourceId:'collection'});assert.equal(await s.mutate({action:'entries',addItemIds:['item']}),false);assert(s.getSnapshot().retryPending);assert(await s.retry());assert.deepEqual(writes[0],writes[1]);assert.equal(writes[0].expectedRevision,1);assert.equal(s.getSnapshot().resource?.revision,2);s.dispose();
});
test('CAS conflict refresh requires an explicit rebase with a different operation ID',async()=>{
 const writes:any[]=[];let revision=1;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({revision}));writes.push(b);if(writes.length===1){revision=5;throw Object.assign(new Error('Changed'),{code:'revision_mismatch',status:412});}revision=6;return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false}};});
 await s.select({view:'resource',resourceId:'collection'});await s.mutate({action:'update',name:'Changed'});assert(s.getSnapshot().mutationError?.conflict);assert.equal(writes.length,1);assert.equal(s.getSnapshot().resource?.revision,5);assert(await s.rebase());assert.notEqual(writes[0].operationId,writes[1].operationId);assert.equal(writes[1].expectedRevision,5);s.dispose();
});
test('current deletion status on replay does not resurrect a previously created collection',async()=>{
 const s=setup(async(p,m,b)=>m==='GET'?directory():{serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'deleted-collection',revision:1,deleted:false},current:{resourceId:'deleted-collection',revision:2,deleted:true}});
 await s.select({view:'collections'});assert(await s.mutate({action:'create',kind:'collection',name:'Collection'}));assert.deepEqual(s.getSnapshot().result,{resourceId:'deleted-collection',deleted:true});s.dispose();
});
test('viewer permissions and opaque member validation fail closed',async()=>{
 let writes=0;const s=setup(async(p,m)=>{if(m!=='GET')writes++;return content(resource({role:'viewer',actions:['pin']}));});await s.select({view:'resource',resourceId:'collection'});await assert.rejects(()=>s.mutate({action:'entries',addItemIds:['item']}),/not currently/);assert.equal(writes,0);s.dispose();
 // A hidden member stays opaque whatever else its row carries, and a repeated member keeps its first row.
 for(const entries of [[{id:'hidden',hidden:true,media:media()}],[{id:'hidden',hidden:true},{id:'hidden',hidden:true}]]){const opaque=setup(async()=>content(resource(),entries));await opaque.select({view:'resource',resourceId:'collection'});assert.equal(opaque.getSnapshot().error,null);assert.deepEqual(opaque.getSnapshot().entries,[{id:'hidden',hidden:true}]);opaque.dispose();}
});
test('one member, history row or resource this app cannot read is left out and the rest is kept',async()=>{
 const members=setup(async()=>({...content(resource(),[{id:'one',hidden:false,media:media('one')},{id:'broken',hidden:false,media:{id:'x'}},{hidden:false},'nonsense',{id:'two',hidden:false,media:media('two'),futureField:[1]}]),futureField:true}));
 await members.select({view:'resource',resourceId:'collection'});assert.equal(members.getSnapshot().error,null);assert.deepEqual(members.getSnapshot().entries.map(e=>e.id),['one','two']);members.dispose();
 const row=(id:string,patch:any={})=>({id,media:media(id),updatedAt:'2026-09-06T12:00:00Z',positionSeconds:30,completed:false,...patch});
 const history=setup(async()=>({serverId:'server',viewerFence:'fence',revision:4,entries:[row('one'),row('bad',{positionSeconds:-1}),row('one',{completed:true}),row('two')],nextCursor:''}));
 await history.select({view:'history'});assert.equal(history.getSnapshot().error,null);assert.deepEqual(history.getSnapshot().entries.map(e=>[e.id,e.completed]),[['one',false],['two',false]]);assert.equal(history.getSnapshot().revision,4);history.dispose();
 const listed=setup(async()=>directory([resource(),resource({id:'smart',kind:'smart-shelf'}),resource({id:'review',status:'archived'}),resource({id:'other',actions:['pin','teleport'],shares:[{authority:'local',accountId:'a',profileId:'p',role:'viewer',displayName:'Second'},{authority:'local',accountId:'a',profileId:'q',role:'curator',displayName:'Third'}]}),resource()]));
 await listed.select({view:'collections'});assert.equal(listed.getSnapshot().error,null);assert.deepEqual(listed.getSnapshot().resources.map(r=>r.id),['collection','other']);
 assert.deepEqual(listed.getSnapshot().resources[1].actions,['pin']);assert.deepEqual(listed.getSnapshot().resources[1].shares.map(s=>[s.profileId,s.role]),[['p','viewer'],['q','viewer']]);listed.dispose();
});
test('sizes, scope, fences, revisions, roles and cursors still refuse the whole document',async()=>{
 for(const page of [directory(Array.from({length:101},(_,i)=>resource({id:'c'+i}))),directory([resource(),resource({id:'two',viewerFence:'other'})]),directory([resource({serverId:'other'})]),{...directory(),serverId:'other'},{...directory(),revision:-1},{...directory(),nextCursor:7},{...directory(),resources:{}}]){
  const s=setup(async()=>page);await s.select({view:'collections'});assert.equal(s.getSnapshot().error?.code,'invalid_saved');assert.equal(s.getSnapshot().resources.length,0);s.dispose();
 }
 for(const patch of [{role:'curator'},{role:'viewer'},{role:'editor',actions:['pin','delete']},{revision:'1'},{pinRevision:-1},{visibility:'everyone'},{viewerFence:''},{id:'another'},{actions:new Array(9).fill('pin')}]){
  const s=setup(async()=>content(resource(patch)));await s.select({view:'resource',resourceId:'collection'});assert.equal(s.getSnapshot().error?.code,'invalid_saved',JSON.stringify(patch));assert.equal(s.getSnapshot().resource,null);s.dispose();
 }
});
test('late reads and writes cannot publish into another resource or disposed scope',async()=>{
 const late=deferred();let body:any;const s=setup(async(p,m,b)=>{if(m==='GET')return p.includes('/content')?content():directory();body=b;return late.promise;});await s.select({view:'resource',resourceId:'collection'});const change=s.mutate({action:'delete'});await tick();await s.select({view:'views'});await change;late.resolve({serverId:'server',viewerFence:'fence',receipt:{operationId:body.operationId,resourceId:'collection',revision:2,deleted:true}});await tick();assert.equal(s.getSnapshot().route?.view,'views');assert.equal(s.getSnapshot().result,null);s.dispose();
 const read=deferred();const x=setup(async()=>read.promise);const work=x.select({view:'collections'});x.dispose();await work;read.resolve(directory([resource()]));await tick();assert.equal(x.getSnapshot().resources.length,0);
});
test('history-only clear and reset-viewing-activity remain distinct idempotent commands',async()=>{
 const bodies:any[]=[];let revision=0;const s=setup(async(p,m,b)=>{if(m==='GET')return {serverId:'server',viewerFence:'fence',revision,entries:[{id:'occurrence',media:media(),updatedAt:'2026-09-06T12:00:00Z',positionSeconds:30,completed:false}],nextCursor:''};bodies.push(b);return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,revision:++revision,action:b.action}};});await s.select({view:'history'});await s.mutate({action:'clear-history'});await s.mutate({action:'reset-viewing-activity'});assert.deepEqual(bodies.map(b=>b.action),['clear-history','reset-viewing-activity']);assert.deepEqual(bodies.map(b=>b.expectedRevision),[0,1]);assert.notEqual(bodies[0].operationId,bodies[1].operationId);s.dispose();
});
test('saved query needs-review does not execute or normalize an unsupported definition',async()=>{
 const s=setup(async()=>content(resource({id:'query',kind:'view',actions:['pin','update','delete'],status:'needs-review',invalidComponents:['pivot'],definition:{pivot:'obsolete'}})));await s.select({view:'resource',resourceId:'query'});assert.equal(s.getSnapshot().resource?.status,'needs-review');assert.equal(s.getSnapshot().browse,null);assert.equal(s.getSnapshot().resource?.definition,undefined);assert.equal(s.getSnapshot().error,null);s.dispose();
 assert.throws(()=>makeSavedDefinition('','prefix'));assert.equal(savedTitlePrefix(makeSavedDefinition('library','Prefix')),'Prefix');assert.equal(savedCategoryId(makeSavedDefinition('library','','grid','title','asc','decade:1990')),'decade:1990');
});
test('pin uses its separate revision and remains available to a resource viewer',async()=>{
 let body:any;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({role:'viewer',actions:['pin'],pinRevision:3}));assert.equal(p,'/v1/saved-pins/collection/collection');body=b;return {serverId:'server',viewerFence:'fence',operationId:b.operationId,resourceId:'collection',revision:1,pinRevision:4,pinned:true,deleted:false};});await s.select({view:'resource',resourceId:'collection'});assert(await s.mutate({action:'pin',pinned:true}));assert.equal(body.expectedRevision,3);assert.equal(body.pinned,true);s.dispose();
});
test('scope navigation cancels stalled operation-ID acquisition without issuing a mutation',async()=>{
 const id=deferred();let writes=0;const s=new PersonalSavedService({request:async<T>(_p,m)=>{if(m!=='GET')writes++;return directory() as T;}},scope,()=>id.promise);await s.select({view:'collections'});const pending=s.mutate({action:'create',kind:'collection',name:'Name'});await s.select({view:'views'});assert.equal(await pending,false);id.resolve('00000000-0000-4000-8000-000000000001');await tick();assert.equal(writes,0);assert.equal(s.getSnapshot().pending,false);s.dispose();
});

test('collection receipts preserve added, unchanged and refused item outcomes for UI confirmation',async()=>{
 for(const entries of [
  {added:['item'],removed:[],unchanged:[],failed:[]},
  {added:[],removed:[],unchanged:['item'],failed:[]},
  {added:[],removed:[],unchanged:[],failed:[{itemId:'item',code:'item_unavailable'}]},
 ]){
  const s=setup(async(_p,m,b)=>m==='GET'?content():{serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision:2,deleted:false,entries}});
  await s.select({view:'resource',resourceId:'collection'});assert.equal(await s.mutate({action:'entries',addItemIds:['item']}),true);
  assert.deepEqual(s.getSnapshot().result?.entries,entries);assert.ok(Object.isFrozen(s.getSnapshot().result?.entries));s.dispose();
 }
});
test('malformed collection outcome cannot become a successful receipt',async()=>{
 const s=setup(async(_p,m,b)=>m==='GET'?content():{serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision:2,deleted:false,entries:{added:['item'],removed:[],unchanged:[],failed:[{itemId:'item',code:'denied'}]}}});
 await s.select({view:'resource',resourceId:'collection'});assert.equal(await s.mutate({action:'entries',addItemIds:['item']}),false);assert.equal(s.getSnapshot().result,null);s.dispose();
});
test('a collection removal carries its undo token, and Undo sends the token alone',async()=>{
 const writes:any[]=[];let revision=1;const s=setup(async(_p,m,b)=>{if(m==='GET')return content(resource({revision}),[{id:'member',hidden:false,media:media()}]);writes.push(b);revision++;return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false,entries:b.undoToken?{added:['member'],removed:[],unchanged:[],failed:[]}:{added:[],removed:['member'],unchanged:[],failed:[]},...(b.undoToken?{}:{undoToken:'undo-1',undoUntil:'2026-10-15T12:00:30.000Z'})}};});
 await s.select({view:'resource',resourceId:'collection'});
 assert.equal(await s.mutate({action:'entries',removeEntryIds:['member']}),true);
 assert.deepEqual(s.getSnapshot().result?.undo,{token:'undo-1',until:'2026-10-15T12:00:30.000Z'});
 assert.equal(await s.mutate({action:'entries',undoToken:'undo-1'}),true);
 assert.equal(writes[1].undoToken,'undo-1');assert.equal(writes[1].addItemIds,undefined);assert.equal(writes[1].removeEntryIds,undefined);assert.equal(s.getSnapshot().result?.undo,undefined);
 await assert.rejects(()=>s.mutate({action:'entries',undoToken:'undo-1',removeEntryIds:['member']}),/Invalid/);s.dispose();
});
test('a collection move sends the entry and its new neighbour; a move refused because the collection changed is made again once, silently',async()=>{
 const writes:any[]=[];const paths:string[]=[];let revision=3;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({revision,actions:['pin','update','delete','entries','reorder','share']}),[{id:'e1',hidden:false,media:media('a')},{id:'e2',hidden:false,media:media('b')}]);paths.push(p);writes.push(b);if(writes.length===1){revision=4;throw Object.assign(new Error('Changed'),{code:'revision_mismatch'});}revision=5;return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false}};});
 await s.select({view:'resource',resourceId:'collection'});
 assert.equal(await s.mutate({action:'reorder',entryIds:['e2'],afterEntryId:''}),true);
 assert.deepEqual(paths,['/v1/saved-resources/collection/order','/v1/saved-resources/collection/order']);
 assert.deepEqual([writes[0].expectedRevision,writes[1].expectedRevision],[3,4]);
 assert.deepEqual(writes[1].entryIds,['e2']);assert.equal(writes[1].afterEntryId,'');assert.notEqual(writes[0].operationId,writes[1].operationId);
 assert.equal(s.getSnapshot().mutationError,null);
 await assert.rejects(()=>s.mutate({action:'reorder',entryIds:['e1','e1']}),/Invalid collection order/);
 s.dispose();
 // An editor may move entries; a viewer is not offered it.
 const viewer=setup(async()=>content(resource({role:'viewer',actions:['pin']})));await viewer.select({view:'resource',resourceId:'collection'});
 await assert.rejects(()=>viewer.mutate({action:'reorder',entryIds:['e1']}),/not currently/);viewer.dispose();
 const editor=setup(async()=>content(resource({role:'editor',actions:['pin','entries','reorder']})));await editor.select({view:'resource',resourceId:'collection'});
 assert.deepEqual(editor.getSnapshot().resource?.actions,['pin','entries','reorder']);editor.dispose();
});
test('a collection paged past its first page is read again through every page on screen, and a move keeps the viewer where they are',async()=>{
 // 100 members, 40 a page: the cursor names where the page starts.
 let order=Array.from({length:100},(_,i)=>'m'+i);let revision=1;const reads:string[]=[];
 const s=setup(async(p,m,b)=>{
  if(m==='GET'){const at=Number(new URL('http://x'+p).searchParams.get('cursor')??0);reads.push(String(at));const ids=order.slice(at,at+40);return {resource:resource({revision,actions:['pin','update','delete','entries','reorder','share']}),entries:ids.map(id=>({id,hidden:false,media:media(id)})),nextCursor:at+40<order.length?String(at+40):''};}
  const from=order.indexOf(b.entryIds[0]);order.splice(from,1);order.splice(b.afterEntryId?order.indexOf(b.afterEntryId)+1:0,0,b.entryIds[0]);revision++;
  return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false}};
 });
 await s.select({view:'resource',resourceId:'collection'});await s.next();await s.next();
 assert.equal(s.getSnapshot().cursor,'80');reads.length=0;
 // A change elsewhere: the first page is read again, through the page in view, as one first page.
 order=['new',...order];await s.revalidate();
 assert.deepEqual(reads,['0','40','80']);assert.equal(s.getSnapshot().cursor,null);assert.deepEqual(s.getSnapshot().history,[]);
 assert.equal(s.getSnapshot().entries.length,101);assert.equal(s.getSnapshot().entries[0].id,'new');assert.equal(s.getSnapshot().nextCursor,'');
 // A move deep in the list: no loading state, every entry still on screen, in the server's new order.
 reads.length=0;const loading:boolean[]=[];const off=s.subscribe(()=>loading.push(s.getSnapshot().loading));
 assert(await s.mutate({action:'reorder',entryIds:['m90'],afterEntryId:''}));off();
 assert(!loading.includes(true));assert.equal(s.getSnapshot().entries.length,101);assert.equal(s.getSnapshot().entries[0].id,'m90');assert.equal(s.getSnapshot().resource?.revision,2);
 s.dispose();
});

// ── A library's presets: its saved views, listed by library and pivot ──────────────────────────
const nested={all:[{any:[{field:'genre',operator:'in',value:['Drama','Crime']},{not:{field:'watched',operator:'equals',value:true}}]},{field:'year',operator:'between',value:[1990,1999]}]};
const presetDef=(patch:any={})=>({libraryId:'shows',pivot:'series',query:nested,sort:[{field:'random',direction:'asc',seed:42},{field:'title',direction:'asc'}],presentation:'grid',...patch});
const preset=(patch:any={})=>resource({id:'p1',kind:'view',name:'Nineties drama',actions:['pin','update','delete'],definition:presetDef(),...patch});

test('presets: a library and pivot narrow the server\'s list; the whole query and sort (a random seed too) come back as stored',async()=>{
 const paths:string[]=[];const s=setup(async p=>{paths.push(p);return directory([preset(),preset({id:'elsewhere',definition:presetDef({libraryId:'films'})}),preset({id:'review',status:'needs-review',definition:undefined,invalidComponents:['query.all[0]']})]);});
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 const query=new URL('http://x'+paths[0]).searchParams;
 assert.deepEqual([query.get('kind'),query.get('libraryId'),query.get('pivot')],['view','shows','series']);
 const [ready,review]=s.getSnapshot().resources;
 assert.deepEqual(s.getSnapshot().resources.map(r=>r.id),['p1','review'],'one saved for another library is not offered here');
 assert.deepEqual(savedViewBrowse(ready!),{libraryId:'shows',pivot:'series',query:nested,sort:[{field:'random',direction:'asc',seed:42},{field:'title',direction:'asc'}]},'nothing flattened, the seed kept');
 assert.equal(review!.status,'needs-review');assert.equal(savedViewBrowse(review!),undefined,'needs review: not applied, and no empty query stands in for it');
 assert.throws(()=>s.select({view:'views',pivot:'series'} as any),/scope/,'a pivot without its library');
 assert.throws(()=>s.select({view:'collections',libraryId:'shows'} as any),/scope/);
 s.dispose();
});

test('presets: saving keeps the seed and the nested query; a refused save says so and is not success',async()=>{
 const bodies:any[]=[];let refuse=true;const s=setup(async(p,m,b)=>{if(m==='GET')return directory([]);bodies.push(b);if(refuse)throw Object.assign(new Error('That name is taken.'),{code:'invalid_request'});return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'new',revision:1,deleted:false}};});
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 assert.equal(await s.mutate({action:'create',kind:'view',name:'Mine',definition:presetDef() as any}),false);
 assert.equal(s.getSnapshot().mutationError?.message,'That name is taken.');
 assert.deepEqual(bodies[0].definition.sort[0],{field:'random',direction:'asc',seed:42});assert.deepEqual(bodies[0].definition.query,nested);
 s.dismiss();refuse=false;assert.equal(await s.mutate({action:'create',kind:'view',name:'Mine',definition:presetDef() as any}),true);s.dispose();
});

test('presets: rename and delete from the list, at the row\'s revision, without reading any preset\'s content',async()=>{
 const paths:string[]=[];const writes:any[]=[];let rows=[preset({revision:3})];
 const s=setup(async(p,m,b)=>{paths.push(m+' '+p);if(m==='GET')return directory(rows);writes.push(b);const deleted=m==='DELETE';if(deleted)rows=[];else rows=[preset({revision:4,name:b.name})];return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'p1',revision:4,deleted}};});
 await s.select({view:'views',libraryId:'shows'});
 assert(await s.mutate({action:'update',name:'Renamed'},false,s.getSnapshot().resources[0]));
 assert.equal(writes[0].expectedRevision,3);assert.equal(writes[0].definition,undefined,'a rename leaves the definition as it is');
 assert.equal(s.getSnapshot().resources[0]!.name,'Renamed');
 assert(await s.mutate({action:'delete'},false,s.getSnapshot().resources[0]));
 assert.equal(s.getSnapshot().resources.length,0,'read again without it');
 assert(!paths.some(p=>p.includes('/content')),'no preset was opened');
 await assert.rejects(()=>s.mutate({action:'update',name:'x'},false,preset() as any),/listed/,'only a row of this list');
 s.dispose();
});

test('presets: a late list from before a profile change never shows',async()=>{
 const late=deferred();const s=setup(async()=>late.promise);const work=s.select({view:'views',libraryId:'shows',pivot:'series'});s.dispose();await work;late.resolve(directory([preset()]));await tick();assert.equal(s.getSnapshot().resources.length,0);
});

test('presets: a save with no answer is unconfirmed and sent again as the same operation; a refused one is not, and a new save is a new operation',async()=>{
 const bodies:any[]=[];let mode='timeout';const s=setup(async(p,m,b)=>{if(m==='GET')return directory([]);bodies.push(b);if(mode==='timeout')throw Object.assign(new Error('The server took too long.'),{code:'timeout'});if(mode==='refused')throw Object.assign(new Error('That name is taken.'),{code:'name_conflict'});return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'new',revision:1,deleted:false}};});
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 const intent={action:'create',kind:'view',name:'Mine',definition:presetDef()} as any;
 assert.equal(await s.mutate(intent),false);
 assert.deepEqual(s.unconfirmed()?.intent,intent,'not known whether it was made');
 mode='ok';assert(await s.retry());assert.equal(bodies[1].operationId,bodies[0].operationId,'the same operation: made at most once');assert.equal(s.unconfirmed(),undefined);
 mode='refused';assert.equal(await s.mutate({...intent,name:'Taken'}),false);assert.equal(s.unconfirmed(),undefined,'refused: not made, nothing to send again');
 s.dismiss();mode='ok';assert(await s.mutate({...intent,name:'Other'}));assert.notEqual(bodies.at(-1).operationId,bodies[2].operationId,'a new save is a new operation');
 s.dispose();
});

test('presets: savedChange sends a change once, sends an unconfirmed one again as itself, and will not replace it with another',async()=>{
 const bodies:any[]=[];let mode='timeout';const s=setup(async(p,m,b)=>{if(m==='GET')return directory([preset({revision:2})]);bodies.push(b);if(mode==='timeout')throw Object.assign(new Error('The server took too long.'),{code:'timeout'});if(mode==='refused')throw Object.assign(new Error('That name is taken.'),{code:'name_conflict'});return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:b.kind?'new':'p1',revision:3,deleted:false}};});
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 const create={action:'create',kind:'view',name:'Mine',definition:presetDef()} as any;
 await assert.rejects(()=>savedChange(s,create),/took too long/);
 await assert.rejects(()=>savedChange(s,{...create,name:'Changed'}),(e:any)=>e.code===UNCONFIRMED,'a different change while the first is unconfirmed');
 assert.equal(bodies.length,1,'nothing new was sent');
 mode='ok';await savedChange(s,create);assert.equal(bodies[1].operationId,bodies[0].operationId,'the same operation again');
 mode='refused';const row=s.getSnapshot().resources[0]!;await assert.rejects(()=>savedChange(s,{action:'update',name:'Taken'},row),/taken/);
 mode='ok';await savedChange(s,{action:'update',name:'Free'},s.getSnapshot().resources[0]);assert.notEqual(bodies.at(-1).operationId,bodies.at(-2).operationId,'a refused change is set aside; the next is new');
 s.dispose();
});

/**
 * A library's presets as the scoped endpoint serves them: `count` presets by name, 40 a page, each
 * page's cursor bound to the list's revision (`rev:offset`); a cursor from an earlier revision is
 * `stale_continuation`, as the server answers. `fail` makes the next read of that cursor fail once.
 */
function presetServer(count:number){
 const s={rev:1,count,reads:[] as (string|null)[],fail:new Map<string,string>(),writes:[] as any[],deleted:new Set<string>(),answer:'ok' as 'ok'|'timeout'};
 const rows=()=>Array.from({length:s.count},(_,i)=>preset({id:`p${i}`,name:`View ${String(i).padStart(4,'0')}`})).filter(r=>!s.deleted.has(r.id));
 const request=async(path:string,method='GET',body?:any)=>{
  if(method!=='GET'){
   s.writes.push({method,path,body});
   if(method==='DELETE')s.deleted.add(decodeURIComponent(path.split('/').at(-1)!));
   s.rev++;
   if(s.answer==='timeout')throw Object.assign(new Error('The server took too long.'),{code:'timeout'});
   return {serverId:'server',viewerFence:'fence',receipt:{operationId:body.operationId,resourceId:method==='POST'?'new':decodeURIComponent(path.split('/').at(-1)!),revision:s.rev,deleted:method==='DELETE'}};
  }
  const cursor=new URL('http://x'+path).searchParams.get('cursor');s.reads.push(cursor);
  const failing=cursor!==null?s.fail.get(cursor):undefined;if(failing){s.fail.delete(cursor!);throw Object.assign(new Error('failed'),{code:failing});}
  let offset=0;if(cursor){const [rev,at]=cursor.split(':').map(Number);if(rev!==s.rev)throw Object.assign(new Error('The list changed.'),{code:'stale_continuation'});offset=at!;}
  const all=rows(),page=all.slice(offset,offset+40);
  return {serverId:'server',viewerFence:'fence',revision:s.rev,resources:page,nextCursor:offset+40<all.length?`${s.rev}:${offset+40}`:''};
 };
 return Object.assign(s,{request});
}
const names=(svc:PersonalSavedService)=>svc.getSnapshot().resources.map(r=>r.id);

test('presets: More after another device changed the list reads it again from the first page, never the expired cursor again; a network failure of a page still retries that page',async()=>{
 const server=presetServer(130);const s=setup(server.request);
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 await s.next();assert.equal(names(s)[0],'p40');
 // Another device saves a view: every cursor given before is stale.
 server.rev++;
 await s.next();
 assert.deepEqual(server.reads.slice(-2),['1:80',null],'the stale cursor once, then the first page');
 assert.equal(s.getSnapshot().error,null,'no stuck error');
 assert.deepEqual([names(s)[0],s.getSnapshot().cursor,s.getSnapshot().history.length,s.getSnapshot().restarted],['p0',null,0,true],'back at the first page, and the list says why');
 await s.next();assert.equal(s.getSnapshot().restarted,false,'until the next page');
 // A page that fails on the way (not a stale cursor): Try again asks for that same page.
 server.fail.set('2:80','network');
 await s.next();
 assert.equal(s.getSnapshot().error?.code,'network');
 await s.retryRead();
 assert.deepEqual(server.reads.slice(-2),['2:80','2:80'],'the same page again');
 assert.equal(names(s)[0],'p80');
 // An expired cursor that is somehow the error: Try again reads the first page, not the same cursor.
 server.fail.set('2:40','invalid_cursor');
 await s.previous();
 assert.equal(server.reads.at(-1),null,'recovered at once: invalid cursors are never retried');
 s.dispose();
});

test('presets: the picker\'s quiet re-read of a later page whose cursor went stale restarts from the first page; reopening after a failure is no dead end',async()=>{
 const server=presetServer(130);const s=setup(server.request);
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 await s.next();
 server.rev++;
 await s.revalidate();
 assert.deepEqual([server.reads.at(-1),names(s)[0],s.getSnapshot().restarted],[null,'p0',true]);
 s.dispose();
});

test('presets page a page at a time however far: More always reads the next page, the way back is bounded, the first page stays reachable, and a quiet re-read never grows the page',async()=>{
 const server=presetServer(70*40);const s=setup(server.request);
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 for(let page=2;page<=70;page++){
  const before=server.reads.length;await s.next();
  assert.equal(server.reads.length,before+1,`page ${page} was read (More is never a no-op while there is more)`);
  assert.equal(names(s).length,40,'one page shown');
  assert.equal(names(s)[0],`p${(page-1)*40}`);
 }
 assert.equal(s.getSnapshot().nextCursor,'','the last page');
 assert.equal(s.getSnapshot().history.length,64,'the way back is bounded');
 await s.previous();assert.equal(names(s)[0],`p${68*40}`,'Previous still steps back');
 const before=server.reads.length;await s.revalidate();
 assert.equal(server.reads.length,before+1,'a quiet re-read reads the page in view, once');
 assert.equal(names(s).length,40,'and shows it as one page, never an accumulated list');
 await s.first();assert.deepEqual([names(s)[0],s.getSnapshot().history.length],['p0',0],'the first page, from anywhere');
 s.dispose();
});

test('the Saved directory keeps its own paging: it still stops after 64 pages and re-reads every page on screen',async()=>{
 const server=presetServer(70*40);const s=setup(server.request);
 await s.select({view:'views'});
 for(let page=2;page<=66;page++)await s.next();
 assert.equal(s.getSnapshot().history.length,64);
 const before=server.reads.length;await s.next();
 assert.equal(server.reads.length,before,'unchanged generic bound');
 s.dispose();
});

test('presets: a delete with no answer is held as itself, named, and sent again exactly once the list no longer shows it; until then no other change is sent',async()=>{
 const server=presetServer(90);const s=setup(server.request);
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 await s.next();await s.next();
 const row=s.getSnapshot().resources.find(r=>r.id==='p85')!;
 server.answer='timeout';
 await assert.rejects(()=>savedChange(s,{action:'delete'},row),/took too long/);
 assert.deepEqual([s.unconfirmed()?.intent.action,s.unconfirmed()?.resourceId,s.unconfirmed()?.resourceName],['delete','p85','View 0085'],'held as itself, with the name it removes');
 // The delete was made: the list read again no longer has the row (its page is gone).
 await s.first();
 assert.equal(names(s).includes('p85'),false);
 await assert.rejects(()=>savedChange(s,{action:'create',kind:'view',name:'New',definition:presetDef() as any}),(e:any)=>e.code===UNCONFIRMED);
 assert.equal(server.writes.length,1,'nothing else sent while it is unconfirmed');
 server.answer='ok';
 await retryUnconfirmed(s);
 assert.equal(server.writes.length,2);
 assert.deepEqual([server.writes[1].method,server.writes[1].path,server.writes[1].body],[server.writes[0].method,server.writes[0].path,server.writes[0].body],'the very same request: one operation, at its revision');
 assert.equal(s.unconfirmed(),undefined);
 await assert.rejects(()=>retryUnconfirmed(s),/no unconfirmed change/);
 s.dispose();
});

test('presets: a held save is sent again as the definition it was sent with, whatever the toolbar shows now; a different save waits until it is resolved or set aside',async()=>{
 const server=presetServer(0);const s=setup(server.request);
 await s.select({view:'views',libraryId:'shows',pivot:'series'});
 server.answer='timeout';
 const sent={action:'create',kind:'view',name:'Nineties',definition:presetDef()} as any;
 await assert.rejects(()=>savedChange(s,sent),/took too long/);
 // The viewer changed the filters and the sort meanwhile: the same name over the new definition is another change.
 const now={...sent,definition:presetDef({query:null,sort:[{field:'year',direction:'desc'}]})};
 await assert.rejects(()=>savedChange(s,now),(e:any)=>e.code===UNCONFIRMED);
 server.answer='ok';
 await retryUnconfirmed(s);
 assert.equal(server.writes.length,2);
 assert.deepEqual(server.writes[1].body,server.writes[0].body,'the held intent itself: its definition, presentation and operation');
 assert.deepEqual(server.writes[1].body.definition.query,nested);
 s.dispose();
});
