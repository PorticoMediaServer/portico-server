import test from 'node:test';
import assert from 'node:assert/strict';
import {parseLocalBook,parseMusicDetails,parseMusicObservation,parseMusicPolicy} from '../src/music-metadata.ts';
import {MetadataReviewService} from '../src/metadata-review.ts';
const id='11111111-1111-1111-1111-111111111111';
const policy={revision:2,localMode:'prefer',musicBrainzEnabled:true,acoustidEnabled:false,acoustidConfigured:false,fingerprintAllowed:false};
const evidence={fingerprintStatus:'not_available',providerError:'',confidence:.9,margin:.2,strongSignals:3,algorithm:'music-evidence-v1'};
function state():any{return {serverId:'server',viewerFence:'fence',libraryId:'library',kind:'song',entityId:'song',revision:3,status:'needs_selection',selectedId:'',manual:false,attempts:0,nextAttempt:'',error:'',attribution:'MusicBrainz',policy:{...policy},observation:{...evidence},candidates:[{id,title:'Song',artist:'Artist',edition:'',confidence:.9,reasons:['exact_title','exact_artist','duration_match','owner_review_required'],decision:'candidate',observedAt:'2026-09-06'}],actions:['select','retry','search','policy']};}
test('local book roles, decimal series positions and identifiers stay typed and immutable',()=>{
 const raw={authors:['Author One','Author Two'],narrators:['Reader'],series:[{name:'Series',position:'2.5'}],identifiers:[{scheme:'isbn',value:'9780000000002'}],edition:'Unabridged',language:'fr-CA',sources:{authors:'opf',narrators:'embedded'}};
 const out=parseLocalBook(raw);raw.authors.push('Later');assert.equal(out.authors.length,2);assert.equal(out.series[0].position,'2.5');assert.equal(out.identifiers[0].scheme,'isbn');assert.ok(Object.isFrozen(out.series[0]));
 for(const patch of [{sources:{author:'/private/file'}},{series:Array(65).fill({name:'x'})},{identifiers:[{scheme:'isbn',value:'\0'}]},{authors:[null]}])assert.throws(()=>parseLocalBook({...raw,...patch}));
});
test('provider credits retain joinphrases and distinguish recording ISRCs from work IDs',()=>{
 const out=parseMusicDetails({status:'Official',script:'Latn',credits:[{name:'Artist',joinphrase:' feat. ',artist:{id,name:'Artist'}}],labels:[{'catalog-number':'CAT-01',label:{id,name:'Label'}}],isrcs:['CAAAA2300001'],relations:[{type:'performance','target-type':'work',work:{id,title:'Work'},attributes:['live']}]});
 assert.equal(out.credits[0].joinPhrase,' feat. ');assert.equal(out.relations[0].targetType,'work');assert.equal(out.labels[0].catalogNumber,'CAT-01');assert.equal(out.script,'Latn');assert.ok(Object.isFrozen(out.credits));
 assert.throws(()=>parseMusicDetails({credits:[],labels:[],relations:[],isrcs:['not-an-isrc']}));
});
test('policy and observation projections reject malformed values and drop private extras',()=>{
 assert.equal(parseMusicPolicy(policy).revision,2);assert.throws(()=>parseMusicPolicy({...policy,revision:0}));assert.throws(()=>parseMusicPolicy({...policy,acoustidEnabled:'yes'}));
 for(const patch of [{confidence:NaN},{confidence:1.01},{margin:-1},{strongSignals:1.5}])assert.throws(()=>parseMusicObservation({...evidence,...patch}));
 const out=parseMusicObservation({...evidence,fingerprint:'private'});assert.equal((out as any).fingerprint,undefined);
});
test('music policy is an owner action using both current job and library policy revisions',async()=>{
 let raw=state();const writes:any[]=[];
 const service=new MetadataReviewService({scope:{serverId:'server',viewerId:'owner'},api:{request:async<T>(path,method,body)=>{if(method==='GET')return raw as T;writes.push({path,method,body});raw={...raw,revision:4,status:'pending',policy:{...raw.policy,revision:3,acoustidEnabled:true}};return {entityId:'song',status:'pending'} as T;}}});
 await service.select({kind:'song',entityId:'song',libraryId:'library'});
 await service.configureMusic({localMode:'supplement',musicBrainzEnabled:true,acoustidEnabled:true});
 assert.deepEqual(writes,[{path:'/v1/items/song/metadata/musicbrainz/policy',method:'POST',body:{expectedRevision:3,expectedPolicyRevision:2,localMode:'supplement',musicBrainzEnabled:true,acoustidEnabled:true}}]);
 assert.equal(service.getSnapshot().data!.policy!.revision,3);service.dispose();
});
test('policy cannot write from absent capability or stale scope; conflicts refresh without replay',async()=>{
 let raw=state(),writes=0;raw.actions=['select','retry','search'];
 const service=new MetadataReviewService({scope:{serverId:'server',viewerId:'owner'},api:{request:async<T>(_path,method)=>{if(method==='GET')return raw as T;writes++;raw={...raw,revision:5,policy:{...raw.policy,revision:4}};throw Object.assign(new Error('Changed'),{code:'metadata_conflict'});}}});
 await service.select({kind:'song',entityId:'song',libraryId:'library'});assert.throws(()=>service.configureMusic({localMode:'off',musicBrainzEnabled:false,acoustidEnabled:false}));
 raw.actions.push('policy');await service.refresh();await service.configureMusic({localMode:'off',musicBrainzEnabled:false,acoustidEnabled:false});assert.equal(writes,1);assert.equal(service.getSnapshot().data!.revision,5);
 raw.viewerFence='different';await service.refresh();assert.equal(service.getSnapshot().data,null);service.dispose();
});

test('local audiobook policy uses library identity and revision; ignores late cancelled responses',async()=>{
 const {saveLocalBookPolicy,parseLocalAudioPolicy}=await import('../src/music-metadata.ts');
 const policy=parseLocalAudioPolicy({revision:4,localMode:'prefer'}),calls:any[]=[];const c=new AbortController();
 await saveLocalBookPolicy({request:async<T>(p,m,b)=>{calls.push([p,m,b]);return {entityId:'book/file',status:'saved'} as T;}},'book/file','library',policy,'off',c.signal);
 assert.deepEqual(calls,[['/v1/items/book%2Ffile/metadata/local-audio/policy','POST',{libraryId:'library',expectedRevision:4,localMode:'off'}]]);
 const late=new AbortController();const pending=saveLocalBookPolicy({request:async<T>()=>new Promise<T>(()=>{})},'book','library',policy,'off',late.signal);late.abort();await assert.rejects(pending,/cancelled/);
});
