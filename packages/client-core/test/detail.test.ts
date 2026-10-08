import test from 'node:test';
import assert from 'node:assert/strict';
import { DetailService, type DetailApi, type PersonalState } from '../src/detail.ts';
const scope = { serverId: 'server', viewerId: JSON.stringify(['local','account','profile']) };
const target = { libraryId: 'library', itemId: 'item' };
const personal = (revision = 0): PersonalState => ({ watchlisted: false, favorite: false, rating: null, revision,watched:false,progressSeconds:0,lastPlayedAt:'',status:'current',conflicts:[] });
function data(itemId = 'item', state = personal()): any {
  return { scope: { serverId: 'server', libraryId: 'library', itemId, viewerFence: 'fence' }, revision: { catalog: 1, viewer: state.revision }, item: { id: itemId, libraryId: 'library', title: 'Server title', kind: 'movie', duration: 100, progressSeconds: 30, available: true, overview: 'Full server description', sources: [{ id: 'source', container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1920, height: 1080 }] }, personal: state,
    actions: [{ id: 'watchlist', labelKey: 'action.watchlist', enabled: true }, { id: 'favorite', labelKey: 'action.favorite', enabled: true }, { id: 'rating', labelKey: 'action.rating', enabled: true, min: .5, max: 5, step: .5 }, { id: 'resume', labelKey: 'action.resume', enabled: true, playback: { itemId, startSeconds: 30 } }, { id: 'start_over', labelKey: 'action.start_over', enabled: true, playback: { itemId, startSeconds: 0 } }],
    metadata: { status: 'available', ratings: [{ provider: 'tmdb', value: 7.8, max: 10, votes: 20, sourceUrl: 'https://www.themoviedb.org/movie/123', observedAt: '2026-09-04T00:00:00Z' }], genres: [{ id: '18', name: 'Drama', provider: 'tmdb' }], credits: [{ id: '1', name: 'Actor', role: 'Role', department: 'Acting', provider: 'tmdb' }] } };
}
function receipt(body: any, state: PersonalState, itemId = 'item'): any { return { operationId: body.operationId, serverId: 'server', libraryId: 'library', itemId, viewerFence: 'fence', personal: state }; }
function deferred<T>() { let resolve!: (v: T) => void; const promise = new Promise<T>(r => { resolve = r; }); return { promise, resolve }; }
const tick = () => new Promise(resolve => setTimeout(resolve, 0));
function setup(handler: (path: string, method?: string, body?: any, signal?: AbortSignal) => Promise<any>, timeoutMs = 1000) {
  let ids = 0;
  const api: DetailApi = { request: <T>(p: string, m?: string, b?: unknown, s?: AbortSignal) => handler(p,m,b,s) as Promise<T> };
  return new DetailService({ api, scope, timeoutMs, requestId: async () => `12345678-1234-4123-8123-${String(++ids).padStart(12,'0')}` });
}

test('detail preserves authoritative content, attribution, personal values and exact playback actions', async () => {
  const service = setup(async () => data()); await service.select(target);
  const state = service.getSnapshot(); assert.equal(state.phase, 'ready');
  assert.deepEqual(state.data!.personal, personal());
  assert.deepEqual(state.data!.metadata, data().metadata);
  assert.deepEqual(state.data!.actions.find(a => a.id === 'resume')!.playback, { itemId: 'item', startSeconds: 30 });
  assert.deepEqual(state.data!.actions.find(a => a.id === 'start_over')!.playback, { itemId: 'item', startSeconds: 0 });
  assert.throws(() => { (state.data!.metadata.credits as any).push({}); }, TypeError);
});

test('item change cancels and fences late detail even if request ignores abort', async () => {
  const old = deferred<any>(); let signal: AbortSignal | undefined;
  const service = setup(async (p,_m,_b,s) => { if (p.includes('/item/')) { signal = s; return old.promise; } return data('new-item'); });
  const pending = service.select(target); await service.select({ ...target, itemId: 'new-item' }); await pending;
  assert.equal(signal!.aborted, true); old.resolve(data()); await tick();
  assert.equal(service.getSnapshot().data!.scope.itemId, 'new-item');
});

test('adjacent duplicate mutation reuses one operation while alternating intents retain order', async () => {
  let state = personal(); const writes: any[] = []; const first = deferred<any>();
  const service = setup(async (_p,m,b) => { if (m === 'GET') return data('item',state); writes.push(b); if (writes.length === 1) return first.promise; assert.equal(b.expectedRevision,state.revision); state = { ...state, favorite: b.favorite, revision: state.revision+1 }; return receipt(b,state); });
  await service.select(target);
  const one = service.mutate({ action: 'favorite', value: true });
  const duplicate = service.mutate({ action: 'favorite', value: true }); assert.equal(one,duplicate);
  const two = service.mutate({ action: 'favorite', value: false });
  const three = service.mutate({ action: 'favorite', value: true }); await tick();
  assert.equal(writes.length,1); assert.equal(service.getSnapshot().data!.personal.favorite,false);
  state = { ...state, favorite: true, revision: 1 }; first.resolve(receipt(writes[0],state));
  await Promise.all([one,two,three]);
  assert.deepEqual(writes.map(w => w.favorite),[true,false,true]);
  assert.deepEqual(writes.map(w => w.expectedRevision),[0,1,2]);
  assert.equal(service.getSnapshot().data!.personal.revision,3);
  assert.equal(service.getSnapshot().pending.length,0);
});

test('timed-out mutation retries exact operation and expected revision, never a new ambiguous set', async () => {
  const writes: any[] = []; const service = setup(async (_p,m,b) => { if (m==='GET') return data(); writes.push(b); if (writes.length===1) return new Promise(()=>{}); return receipt(b,{...personal(1),watchlisted:true}); },5);
  await service.select(target); await service.mutate({action:'watchlist',value:true});
  assert.equal(service.getSnapshot().pending[0].phase,'retry-required');
  assert.equal(service.getSnapshot().mutationError!.code,'timeout');
  await service.retryMutation(); assert.deepEqual(writes[0],writes[1]);
  assert.equal(service.getSnapshot().data!.personal.watchlisted,true);
});

test('late receipt with older personal revision cannot roll back a newer authoritative read', async () => {
  const mutation = deferred<any>(); let body: any, current = personal();
  const service = setup(async (_p,m,b) => { if(m==='GET') return data('item',current); body=b; return mutation.promise; });
  await service.select(target); const pending=service.mutate({action:'favorite',value:true}); await tick();
  current={...personal(3),favorite:false,rating:4}; await service.refresh();
  mutation.resolve(receipt(body,{...personal(1),favorite:true})); await pending;
  assert.deepEqual(service.getSnapshot().data!.personal,current);
});

test('late pre-mutation read cannot roll back confirmed personal state', async () => {
  const stale = deferred<any>(); let reads=0;
  const service=setup(async (_p,m,b)=>{ if(m==='GET'){reads++;return reads===1?data():stale.promise;} return receipt(b,{...personal(1),favorite:true}); });
  await service.select(target); const read=service.refresh(); await service.mutate({action:'favorite',value:true});
  stale.resolve(data()); await read;
  assert.equal(service.getSnapshot().data!.personal.favorite,true); assert.equal(service.getSnapshot().data!.personal.revision,1);
});

test('CAS conflict refreshes state and needs explicit retry with new operation against current revision', async () => {
  let current=personal(),writes:any[]=[];
  const service=setup(async (_p,m,b)=>{ if(m==='GET')return data('item',current); writes.push(b); if(writes.length===1){current={...personal(5),watchlisted:true};throw Object.assign(new Error('Personal state changed'),{code:'personal_state_conflict',retryable:true});} current={...current,favorite:true,revision:6};return receipt(b,current); });
  await service.select(target); await service.mutate({action:'favorite',value:true}); await tick();
  assert.equal(writes.length,1);assert.equal(service.getSnapshot().pending[0].phase,'conflict');assert.equal(service.getSnapshot().data!.personal.revision,5);
  await service.retryMutation();assert.notEqual(writes[0].operationId,writes[1].operationId);assert.equal(writes[1].expectedRevision,5);
  assert.equal(service.getSnapshot().data!.personal.watchlisted,true);assert.equal(service.getSnapshot().data!.personal.favorite,true);
});

test('scope change prevents queued writes and late mutation publication in new account/profile/item', async () => {
  const mutation=deferred<any>();let body:any,writes=0;
  const service=setup(async (_p,m,b)=>{if(m==='GET')return data();writes++;body=b;return mutation.promise;});
  await service.select(target);const a=service.mutate({action:'favorite',value:true});const b=service.mutate({action:'rating',value:4});await tick();
  service.setScope({...scope,viewerId:'new-profile'},{request:async<T>()=>data() as T});await Promise.all([a,b]);
  assert.equal(service.getSnapshot().data,null);assert.equal(service.getSnapshot().pending.length,0);
  mutation.resolve(receipt(body,{...personal(1),favorite:true}));await tick();assert.equal(writes,1);assert.equal(service.getSnapshot().data,null);
});

test('rating clear is explicit null and unsupported or malformed intents do not reach transport', async()=>{
 let last:any;const service=setup(async(_p,m,b)=>{if(m==='GET')return data();last=b;return receipt(b,personal(1));});await service.select(target);
 await service.mutate({action:'rating',value:null});assert.equal(last.rating,null);assert.deepEqual(Object.keys(last).sort(),['expectedRevision','operationId','rating']);
 assert.throws(()=>service.mutate({action:'rating',value:3.7}),/Invalid/);assert.throws(()=>service.mutate({action:'rating',value:0}),/Invalid/);
 assert.throws(()=>service.mutate({action:'playlist',value:true} as any),/Invalid/);
 const sparse=setup(async()=>{const d=data();d.actions=[];d.metadata={status:'unavailable',ratings:[],genres:[],credits:[]};return d;});await sparse.select(target);
 assert.throws(()=>sparse.mutate({action:'favorite',value:true}),/not currently available/);assert.deepEqual(sparse.getSnapshot().data!.metadata.credits,[]);
});

test('wrong-scope receipts, malformed detail and overlarge metadata fail closed',async()=>{
 for(const alter of [(d:any)=>{d.scope.itemId='other';},(d:any)=>{d.metadata.credits=Array(201).fill({});},(d:any)=>{d.personal.rating=8;},(d:any)=>{d.actions[3].playback.itemId='other';}]){
  const service=setup(async()=>{const d=data();alter(d);return d;});await service.select(target);assert.equal(service.getSnapshot().phase,'error');assert.equal(service.getSnapshot().data,null);
 }
 const service=setup(async(_p,m,b)=>m==='GET'?data():{...receipt(b,{...personal(1),favorite:true}),viewerFence:'other'});await service.select(target);await service.mutate({action:'favorite',value:true});
 assert.equal(service.getSnapshot().mutationError!.code,'invalid_detail');assert.equal(service.getSnapshot().data!.personal.favorite,false);
});

test('bounded queue, cancellation and disposal settle pending commands without late publication',async()=>{
 const service=setup(async(_p,m)=>m==='GET'?data():new Promise(()=>{}),10000);await service.select(target);
 const pending=[];for(let i=0;i<8;i++)pending.push(service.mutate({action:'favorite',value:i%2===0}));
 assert.throws(()=>service.mutate({action:'rating',value:5}),/Wait/);service.cancel();await Promise.all(pending);assert.equal(service.getSnapshot().phase,'idle');
 service.dispose();assert.throws(()=>service.refresh(),/disposed/);
});

test('bounded episode/song/bookFile source metadata and unknown boundaries survive rich detail',async()=>{
 const fields={episode:{showId:'show',showTitle:'Show',seasonId:'season',seasonNumber:0,numbering:'seasonal',number:2,localIdentityStatus:'parsed',providerMatchStatus:'unmatched',orderingBasis:'unspecified',sourceBoundary:'unknown_shared_file'},song:{albumId:'album',albumTitle:'Album',albumArtist:'Various Artists',artist:'Performer',discNumber:2,trackNumber:null,providerMatchStatus:'unmatched',localMetadataIssue:'unassigned'},bookFile:{bookId:'book',bookTitle:'Book',discNumber:null,partNumber:2,sourceBoundary:'whole_file',localMetadataIssue:'unassigned'}};
 for(const [field,value]of Object.entries(fields)){
  const service=setup(async()=>{const d=data();d.item[field]=value;d.item.overview='Paragraph one.\n\nParagraph two.';d.item.sources[0].duration=100;return d;});await service.select(target);
  assert.equal(service.getSnapshot().phase,'ready');assert.deepEqual((service.getSnapshot().data!.item as any)[field],value);
  assert.equal(service.getSnapshot().data!.item.sources![0].duration,100);assert.match(service.getSnapshot().data!.item.overview!,/\n\n/);
 }
 const bad=setup(async()=>{const d=data();d.item.episode={...fields.episode,showTitle:'x'.repeat(2049)};return d;});await bad.select(target);assert.equal(bad.getSnapshot().error!.code,'invalid_detail');
});

test('watched and favorite actions use current reconciliation state without changing independent sets',async()=>{
 let current:PersonalState={...personal(),watchlisted:true,favorite:true,rating:4};const writes:any[]=[];
 const service=setup(async(_p,m,b)=>{if(m==='GET'){const d=data('item',current);d.actions.push({id:'watched',labelKey:'action.watched',enabled:true});return d;}writes.push(b);current={...current,revision:current.revision+1,...('watched'in b?{watched:b.watched,progressSeconds:0}:{favorite:b.favorite})};return {...receipt(b,personal()),current};});
 await service.select(target);await service.mutate({action:'watched',value:true});await tick();await service.mutate({action:'favorite',value:false});await tick();assert.equal(writes[0].watched,true);assert.equal(writes[1].favorite,false);assert.deepEqual(service.getSnapshot().data!.personal,{...personal(2),watchlisted:true,favorite:false,rating:4,watched:true});service.dispose();
});
test('offline reconciliation retains supplied device evidence and never silently rebases conflict',async()=>{
 let body:any;const service=setup(async(_p,m,b)=>{if(m==='GET')return data('item',personal(3));body=b;throw Object.assign(new Error('Review the current ancestor'),{code:'personal_needs_review'});});await service.select(target);
 const offline={deviceId:'device',deviceMutationId:'mutation',sequence:1,baseRevision:0,authoredAt:'2026-09-06T12:00:00Z'};
 await service.reconcile({operationId:'00000000-0000-4000-8000-000000000001',offline,intent:{action:'watched',value:true}});await tick();assert.deepEqual(body.offline,offline);assert.equal(body.expectedRevision,0);assert.equal(service.getSnapshot().pending[0].phase,'conflict');assert.throws(()=>service.retryMutation(),/offline intent needs review/);service.dismissMutation();assert.equal(service.getSnapshot().pending.length,0);service.dispose();
});
test('CD-24: audiobook part with progress past duration opens with a bounded resume',async()=>{
 // Persisted progress past the current duration still opens; the server clamps
 // the resume action to the known duration (detail.ts:140 bound kept).
 const over=data();
 over.item.duration=100;over.item.progressSeconds=150;
 over.actions=over.actions.map((a:any)=>a.id==='resume'?{...a,playback:{itemId:'item',startSeconds:100}}:a);
 const s=setup(async()=>over);await s.select(target);
 assert.equal(s.getSnapshot().phase,'ready');
 assert.equal(s.getSnapshot().data!.item.progressSeconds,150);
 assert.equal(s.getSnapshot().data!.actions.find(a=>a.id==='resume')!.playback!.startSeconds,100);
 s.dispose();
 // An unbounded resume past duration fails closed.
 const bad=data();
 bad.actions=bad.actions.map((a:any)=>a.id==='resume'?{...a,playback:{itemId:'item',startSeconds:150}}:a);
 const t=setup(async()=>bad);await t.select(target);
 assert.equal(t.getSnapshot().phase,'error');
 t.dispose();
});
