import test from 'node:test';
import assert from 'node:assert/strict';
import {ManagedProfilesClient,parseManagedProfiles,parseDirectSnapshot,parseTrustedProfile,trustScopeKey} from '../src/profile-management.ts';
const p=(i=0)=>({id:'profile-'+i,name:'Profile '+i,primary:i===0,position:i,art:'blue',revision:1,pinRevision:1,pinRequired:false});
const scope={authority:'local' as const,accountId:'account',profileId:'profile',serverId:'server',installationId:'installation'};
const apiFor=(response:unknown,calls:Array<{path:string;method?:string;body?:unknown}>)=>({request:async<T>(path:string,method?:string,body?:unknown)=>{calls.push({path,method,body});return response as T;}});
test('profile DTOs require one primary and at most eight unique active profiles',()=>{
 assert.equal(parseManagedProfiles(Array.from({length:8},(_,i)=>p(i))).length,8);
 for(const rows of [[],Array.from({length:9},(_,i)=>p(i)),[p(),p()],[{...p(),primary:false}],[{...p(),pinRevision:0}],[{...p(),art:'remote-url'}]])assert.throws(()=>parseManagedProfiles(rows));
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