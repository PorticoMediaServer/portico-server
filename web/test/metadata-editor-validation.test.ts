import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule, hooks} from './helpers/component-harness.mjs';
const ui = Object.fromEntries(['Artwork','Button','Dialog','Icon','Input','Loading','Notice','Select','Spinner','Text','TextArea'].map(x=>[x,x]));
async function editorApp(){
 const h=hooks();
 const app=await componentModule(new URL('../src/screens/shared/MetadataEditor.tsx',import.meta.url),{
  react:h.react,'@core/metadata-repair.ts':{MetadataRepairService:class{}},'@core/metadata-bulk.ts':{bulkTargetLimit:200,validateBulkReceipt:()=>{throw new Error('unused');}},'./bulk-job':{runBulkRequests:async()=>({ok:0,failed:[],jobs:0})},
  '../../app/content':{useService:()=>({})},'../../app/metadata-editor':{useMetadataEditor:()=>undefined},'../../app/session':{useSession:()=>({})},
  '../../ui':{...ui,cx:()=>'',useCompact:()=>false},'./MetadataEditor.module.css':{default:{}},
  '../../app/i18n':{currentI18n:()=>defaultI18n},'../../app/viewer-scope':{useViewerScope:()=>({})},
  '../../app/errors':{errorText:()=>'error'},
 });
 return app;
}
const year = {field:'year',label:'Year',type:'integer',bulk:true,min:0,max:9999} as const;
const text5 = {field:'tagline',label:'Tagline',type:'text',bulk:true,maxLength:5} as const;
// CD-46: canonical non-negative decimals for integer and number fields.
test('integer and number fields accept canonical decimals within min/max only',async()=>{
 const app=await editorApp();
 assert.equal(app.validateRepairValue(year,'2024','item'),undefined);
 assert.equal(app.validateRepairValue({...year,type:'number'},'3','item'),undefined);
 for (const bad of ['007','+1',' 2024','2024 ','20.5','2e3','0x10','-1','abc']) {
  assert.deepEqual(app.validateRepairValue(year,bad,'item'),{key:'web.metadata.wholeNumber',params:{label:'Year'}},bad);
 }
 assert.deepEqual(app.validateRepairValue(year,'10000','item'),{key:'web.metadata.numberRange',params:{label:'Year',min:0,max:9999}});
});
// CD-46: an empty string clears (except the required title); real calendar dates only.
test('clear semantics and real YYYY-MM-DD validity',async()=>{
 const app=await editorApp();
 assert.equal(app.validateRepairValue(year,'','item'),undefined);
 const date = {field:'releaseDate',label:'Release date',type:'date',bulk:true} as const;
 assert.equal(app.validateRepairValue(date,'2024-05-17','item'),undefined);
 assert.equal(app.validateRepairValue(date,'2024-02-29','item'),undefined);
 assert.equal(app.validateRepairValue(date,'','item'),undefined);
 for (const bad of ['2024-02-30','2023-02-29','2024-13-01','2024-00-10','2024-01-00','24-05-17','2024-5-17','2024/05/17']) {
  assert.deepEqual(app.validateRepairValue(date,bad,'item'),{key:'web.metadata.dateError',params:{label:'Release date'}},bad);
 }
 const title = {field:'title',label:'Title',type:'text',bulk:false} as const;
 assert.deepEqual(app.validateRepairValue(title,'','item'),{key:'web.metadata.titleRequired'});
 assert.deepEqual(app.validateRepairValue(title,'   ','item'),{key:'web.metadata.titleRequired'});
 assert.equal(app.validateRepairValue(title,'','season'),undefined,'a season title clears, like the server');
});
// CD-46: text counts runes; enums meet the allow-list; lists bound entries and count.
test('text, enum and list bounds',async()=>{
 const app=await editorApp();
 assert.equal(app.validateRepairValue(text5,'12345','item'),undefined);
 assert.deepEqual(app.validateRepairValue(text5,'123456','item'),{key:'web.metadata.textTooLong',params:{label:'Tagline',max:5}});
 assert.equal(app.validateRepairValue(text5,'😀'.repeat(5),'item'),undefined,'five astral characters are five runes');
 assert.deepEqual(app.validateRepairValue(text5,'😀'.repeat(6),'item'),{key:'web.metadata.textTooLong',params:{label:'Tagline',max:5}});
 const e = {field:'status',label:'Status',type:'enum',bulk:false,allowed:['a','b']} as const;
 assert.equal(app.validateRepairValue(e,'a','item'),undefined);
 assert.deepEqual(app.validateRepairValue(e,'c','item'),{key:'web.metadata.enumError',params:{label:'Status'}});
 const list = {field:'tags',label:'Tags',type:'list',bulk:true,maxLength:128,max:64} as const;
 assert.equal(app.validateRepairList(list,[' a ','','b']),undefined,'trims and drops empties like the server');
 assert.deepEqual(app.validateRepairList(list,['x'.repeat(129)]),{key:'web.metadata.listEntryTooLong',params:{label:'Tags',max:128}});
 assert.deepEqual(app.validateRepairList(list,Array(65).fill('ok')),{key:'web.metadata.tooManyEntries',params:{label:'Tags',max:64}});
});
// WEB-MENU-03: candidate thumbnails go through the artwork route's candidate preview.
test('candidateArtworkPath builds the owner candidate preview with a thumbnail size',async()=>{
 const app=await editorApp();
 assert.equal(app.candidateArtworkPath({kind:'item',id:'abc'},{role:'poster',subject:'',id:'digest1'}),'/v1/metadata/item/abc/art/poster?candidate=digest1&size=thumbnail');
 assert.equal(app.candidateArtworkPath({kind:'show',id:'s 1'},{role:'backdrop',subject:'tmdb:7',id:'digest2'}),'/v1/metadata/show/s%201/art/backdrop?candidate=digest2&size=thumbnail&subject=tmdb%3A7');
});
// CD-48: only the screen engine sends query; MusicBrainz omits it and uses scanned metadata.
test('matchingSearchCommand keeps query for screen and omits stale query otherwise',async()=>{
 const app=await editorApp();
 const screen={provider:'tmdb',id:'12',engine:'screen'} as any;
 assert.deepEqual(app.matchingSearchCommand(screen,'  Dune  '),{action:'search',query:'Dune'});
 assert.deepEqual(app.matchingSearchCommand(screen,'   '),{action:'search'});
 const music={provider:'musicbrainz',id:'abc'} as any;
 assert.deepEqual(app.matchingSearchCommand(music,'Dune'),{action:'search'},'stale query omitted after switching to MusicBrainz');
 assert.deepEqual(app.matchingSearchCommand(undefined,'Dune'),{action:'search'},'no matcher offers no query');
});
// M25-1b: every identity status maps to catalogue copy, never raw.
test('matching status maps every known value to catalogue copy with a generic fallback', async () => {
  const app = await editorApp();
  assert.equal(app.matchingStatusKey('searching'), 'web.metadata.matchStatus.searching');
  assert.equal(app.matchingStatusKey('needs_selection'), 'web.metadata.matchStatus.needsSelection');
  assert.equal(app.matchingStatusKey('unmatched'), 'web.metadata.matchStatus.unmatched');
  assert.equal(app.matchingStatusKey('matched'), 'web.metadata.matchStatus.matched');
  assert.equal(app.matchingStatusKey('pending_children'), 'web.metadata.matchStatus.pendingChildren');
  assert.equal(app.matchingStatusKey('delegated_tvdb'), 'web.metadata.matchStatus.delegatedTvdb');
  assert.equal(app.matchingStatusKey('identity_conflict'), 'web.metadata.matchStatus.identityConflict');
  assert.equal(app.matchingStatusKey('bogus_future_status'), 'web.metadata.matchStatus.unknown');
  assert.equal(app.matchingStatusKey(undefined), 'web.metadata.matchStatus.unknown');
  assert.equal(app.matchingStatusKey(''), 'web.metadata.matchStatus.unknown');
  // The catalogue actually has copy for every key (t() blanks unknown ids).
  for (const status of ['searching', 'pending', 'pending_search', 'pending_episodes', 'pending_children', 'pending_apply', 'matched', 'matched_work', 'accepted', 'published', 'complete', 'needs_selection', 'needs_order', 'needs_parent_match', 'needs_consent', 'needs_season_mapping', 'unmatched', 'unresolved', 'unavailable', 'source_unavailable', 'provider_disabled', 'provider_unavailable', 'manual_preserved', 'delegated_tvdb', 'identity_conflict']) {
    const key = app.matchingStatusKey(status);
    assert.notEqual(defaultI18n.t(key), '', `${status} -> ${key} has copy`);
    assert.ok(!defaultI18n.t(key).includes('_'), `${key} is words, not snake_case`);
  }
  assert.notEqual(defaultI18n.t('web.metadata.matchStatus.unknown'), '');
  assert.notEqual(defaultI18n.t('web.metadata.notMatched'), '');
  assert.notEqual(defaultI18n.t('web.metadata.identityLocked'), '');
  assert.notEqual(defaultI18n.t('web.metadata.identityUnlocked'), '');
  assert.ok(defaultI18n.t('web.metadata.orders', {names: 'A, B'}).includes('A, B'));
  assert.notEqual(defaultI18n.t('web.metadata.searchProviderHelpAuto'), '');
  assert.notEqual(defaultI18n.t('web.metadata.noCandidatesMatchAuto'), '');
});
// M25-1a: only transient provider-work states trigger a reload after search/retry.
test('transient matching statuses are exactly the queued-work states', async () => {
  const app = await editorApp();
  for (const s of ['searching', 'pending', 'pending_search', 'pending_episodes', 'pending_children', 'pending_apply']) assert.equal(app.isTransientMatchingStatus(s), true, s);
  for (const s of ['matched', 'needs_selection', 'unmatched', 'unresolved', 'unavailable', 'manual_preserved', 'delegated_tvdb', 'identity_conflict', undefined, '', 'bogus']) assert.equal(app.isTransientMatchingStatus(s), false, String(s));
});
test('chunkBulkTargets splits at the shared limit with exact identities',async()=>{
 const app=await editorApp();
 const targets=Array.from({length:201},(_,i)=>({kind:'item',id:`id-${i}`,libraryId:'lib'}));
 const chunks=app.chunkBulkTargets(targets);
 assert.equal(chunks.length,2);
 assert.equal(chunks[0].length,200);
 assert.equal(chunks[1].length,1);
 assert.equal(chunks[0][0].id,'id-0');
 assert.equal(chunks[1][0].id,'id-200');
});
test('deleteOverLimit blocks 201 distinct IDs before preview and allows 200',async()=>{
 const harness=await componentModule(new URL('../src/app/delete-media-dialog.tsx',import.meta.url),{
  react:{createElement:()=>null,useEffect:()=>{},useState:(v:unknown)=>[v,()=>{}]},
  '@core/administration.ts':{}, './session':{useSession:()=>({})}, './libraries':{useLibrariesContext:()=>({items:[]})},
  './i18n':{useI18n:()=>({t:(k:string)=>k})}, './errors':{errorText:()=>'err'}, '../ui':{}, './delete-media':{},
 });
 assert.equal(harness.deleteOverLimit(Array.from({length:200},(_,i)=>`id-${i}`)),false);
 assert.equal(harness.deleteOverLimit(Array.from({length:201},(_,i)=>`id-${i}`)),true);
 // Duplicates do not inflate: 201 entries with 200 distinct is allowed.
 assert.equal(harness.deleteOverLimit([...Array.from({length:200},(_,i)=>`id-${i}`),'id-0']),false);
});
