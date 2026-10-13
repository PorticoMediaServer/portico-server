import test from 'node:test';
import assert from 'node:assert/strict';
import {ManagedProfilesClient,parseManagedProfiles,parseDirectSnapshot,parseTrustedProfile,trustScopeKey} from '../src/profile-management.ts';
import {resetSkippedParts,skippedParts} from '../src/tolerant.ts';
const p=(i=0)=>({id:'profile-'+i,name:'Profile '+i,primary:i===0,position:i,art:'blue',revision:1,pinRevision:1,pinRequired:false});
const scope={authority:'local' as const,accountId:'account',profileId:'profile',serverId:'server',installationId:'installation'};
const apiFor=(response:unknown,calls:Array<{path:string;method?:string;body?:unknown}>)=>({request:async<T>(path:string,method?:string,body?:unknown)=>{calls.push({path,method,body});return response as T;}});
test('profile DTOs require one primary and at most eight unique active profiles',()=>{
 assert.equal(parseManagedProfiles(Array.from({length:8},(_,i)=>p(i))).length,8);
 for(const rows of [[],Array.from({length:9},(_,i)=>p(i)),[p(),p()],[{...p(),primary:false}],[{...p(),pinRevision:0}],[{...p(),art:7}],[{...p(),art:''}],[{...p(),art:'x'.repeat(65)}],[p(),{...p(1),pinRequired:'no'}],[p(),{...p(1),allowedLibraries:[7]}],[p(),{...p(1),revision:0}],[p(),null]])assert.throws(()=>parseManagedProfiles(rows));
 const sorted=parseManagedProfiles([p(2),p(),p(1)]);assert.equal(sorted[0].id,'profile-0');assert.ok(Object.isFrozen(sorted));
});
test('Direct account DTO does not derive administration from a selected child profile',()=>{
 const raw={authority:'local',serverId:'server',account:{id:'account',username:'member',primaryProfileId:'profile-0',role:'member',revision:1,disabled:false,allowedLibraries:[]},profiles:[p(),p(1)],canManage:false};
 assert.equal(parseDirectSnapshot(raw,{accountId:'account',serverId:'server'}).canManage,false);
 assert.throws(()=>parseDirectSnapshot(raw,{accountId:'another'}));assert.throws(()=>parseDirectSnapshot(raw,{authority:'hosted'}));
 assert.throws(()=>parseDirectSnapshot({...raw,account:{...raw.account,primaryProfileId:'profile-1'}}));
});
test('trusted selection requires full installation scope, revisions and bounded expiry',()=>{
 const now=Date.now(),proof={...scope,token:'opaque',pinRevision:1,profileRevision:1,membershipRevision:1,expiresAt:new Date(now+86400000).toISOString()};
 assert.equal(parseTrustedProfile(proof,scope,now).token,'opaque');
 for(const patch of [{installationId:'other'},{accountId:'other'},{profileId:'other'},{serverId:'other'},{authority:'hosted'},{pinRevision:0},{membershipRevision:0},{expiresAt:new Date(now-1).toISOString()},{expiresAt:new Date(now+31*86400000).toISOString()}])assert.throws(()=>parseTrustedProfile({...proof,...patch},scope,now));
 assert.notEqual(trustScopeKey(scope),trustScopeKey({...scope,authority:'hosted'}));assert.notEqual(trustScopeKey(scope),trustScopeKey({...scope,installationId:'other'}));
 const hosted={...scope,authority:'hosted' as const};assert.throws(()=>parseTrustedProfile({...proof,...hosted},hosted,now));
 assert.equal(parseTrustedProfile({...proof,...hosted,policyRevision:4,trustRevision:1},hosted,now).policyRevision,4);
});
test('profile edits carry optimistic revision and refuse malformed PINs before sending',async()=>{
 const calls:any[]=[];const c=new ManagedProfilesClient(apiFor({},calls),scope);
 await c.edit(p(),{pin:'0123',policy:{serverId:'server',allowedLibraries:null}});
 assert.deepEqual(calls[0].body,{pin:'0123',policy:{allowedLibraries:null},expectedRevision:1});
 for(const pin of ['123','12345','12a4',' 123'])await assert.rejects(c.edit(p(),{pin}));
 await assert.rejects(c.edit(p(),{policy:{serverId:'another',allowedLibraries:[]}}));assert.equal(calls.length,1);
 await assert.rejects(c.previewDelete(p()));assert.equal(calls.length,1);
});
test('a picture colour this app does not know reads as blue and the profile is otherwise unchanged',()=>{
 resetSkippedParts();
 // The colour is one of this app's own names or blue: a value from the server is never passed on to be drawn.
 const [first,second]=parseManagedProfiles([{...p(),art:'remote-url',pinRequired:true,allowedLibraries:['films'],futureField:{nested:true}},{...p(1),art:'coral'}]);
 assert.deepEqual(first,{id:'profile-0',name:'Profile 0',primary:true,position:0,art:'blue',revision:1,pinRevision:1,pinRequired:true,allowedLibraries:['films']});
 assert.equal(second.art,'coral');
 assert.deepEqual(skippedParts(),[{document:'profiles',part:'profile.art',count:1}]);
});
test('a profile list read for display leaves out one unreadable profile and never shows it with a restriction missing',async()=>{
 const hosted={authority:'hosted' as const,accountId:'account'};
 const listed=(profiles:unknown[],extra:Record<string,unknown>={})=>new ManagedProfilesClient(apiFor({accountId:'account',maxProfiles:8,profiles,...extra},[]),hosted).list();
 resetSkippedParts();
 const kept=await listed([p(),{...p(1),pinRequired:'no'},{...p(2),allowedLibraries:[7]},{...p(3),allowedLibraries:'all'},{...p(4),pinRevision:0},{...p(5),name:''},null,{...p(7),pinRequired:true,allowedLibraries:[]}],{futureField:1});
 assert.deepEqual(kept.map(v=>v.id),['profile-0','profile-7']);assert.equal(kept[1].pinRequired,true);assert.deepEqual(kept[1].allowedLibraries,[]);
 assert.deepEqual(skippedParts(),[{document:'profiles',part:'rows',count:6}]);
 // The primary profile, the size of the list, a repeated id, the account and its profile limit are not negotiable.
 await assert.rejects(listed([{...p(),pinRequired:'no'},p(1)]));
 await assert.rejects(listed([p(),{...p(1),primary:true}]));
 await assert.rejects(listed([p(),p(1),{...p(1),pinRequired:true}]));
 await assert.rejects(listed(Array.from({length:9},(_,i)=>p(i))));
 await assert.rejects(listed([]));
 await assert.rejects(listed([p()],{accountId:'another'}));
 await assert.rejects(listed([p()],{maxProfiles:9}));
 await assert.rejects(listed({} as unknown as unknown[]));
});
test('sign-in and the account snapshot still refuse a profile list with an unreadable profile in it',async()=>{
 const account={id:'account',username:'member',primaryProfileId:'profile-0',role:'member',revision:1,disabled:false,allowedLibraries:[]};
 const raw={authority:'local',serverId:'server',account,profiles:[p(),{...p(1),pinRequired:'no'}],canManage:false};
 // Left out, the second profile would make this look like an account with one unprotected profile.
 assert.throws(()=>parseDirectSnapshot(raw));assert.throws(()=>parseDirectSnapshot(raw,{accountId:'account',serverId:'server'}));
 const local=new ManagedProfilesClient(apiFor(raw,[]),scope);
 assert.deepEqual((await local.list()).map(v=>v.id),['profile-0']);
 // A profile whose row could not be read has no policy this app can state, so none is stated.
 await assert.rejects(local.policies('profile-1'));
 assert.deepEqual(await local.policies('profile-0'),[{serverId:'server',allowedLibraries:null}]);
 // The account's own role, restrictions and identity are exact in every reading.
 for(const patch of [{role:'superuser'},{allowedLibraries:[7]},{allowedLibraries:undefined},{disabled:'no'},{revision:0},{id:''}]){
  const changed={...raw,profiles:[p()],account:{...account,...patch}};
  assert.throws(()=>parseDirectSnapshot(changed));await assert.rejects(new ManagedProfilesClient(apiFor(changed,[]),scope).list());
 }
 await assert.rejects(new ManagedProfilesClient(apiFor({...raw,profiles:[p()],serverId:'another'},[]),scope).list());
});
test('a hosted profile policy list is a restriction document and is read whole or not at all',async()=>{
 const hosted={authority:'hosted' as const,accountId:'account'};
 const policies=(items:unknown)=>new ManagedProfilesClient(apiFor({items,futureField:1},[]),hosted).policies('profile-1');
 assert.deepEqual(await policies([{serverId:'a',allowedLibraries:null,futureField:1},{serverId:'b',allowedLibraries:['films']}]),[{serverId:'a',allowedLibraries:null},{serverId:'b',allowedLibraries:['films']}]);
 await assert.rejects(policies([{serverId:'a',allowedLibraries:null},{serverId:'b',allowedLibraries:[7]}]));
 await assert.rejects(policies([{serverId:'a',allowedLibraries:null},{serverId:'b'}]));
 await assert.rejects(policies([{serverId:'a',allowedLibraries:null},{serverId:'',allowedLibraries:[]}]));
 await assert.rejects(policies([{serverId:'a',allowedLibraries:null},null]));
 await assert.rejects(policies(Array.from({length:1001},()=>({serverId:'a',allowedLibraries:null}))));
});
