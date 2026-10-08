import test from 'node:test';
import assert from 'node:assert/strict';
import {CredentialSessionService,type CredentialRecord,type HostedSession} from '../src/index.ts';

const session = ():HostedSession => ({accessToken:'access',refreshToken:'refresh',familyId:'family',expiresAt:new Date(Date.now()+3600000).toISOString(),refreshExpiresAt:new Date(Date.now()+86400000).toISOString(),account:{id:'account',username:'owner',displayName:'Owner'},profiles:[{id:'profile',name:'Before'}]});
const scope={accountId:'account',familyId:'family'};
function fixture(timeout=1000){
 let saved:CredentialRecord|undefined,fail=false,rotations=0;
 const service=new CredentialSessionService({authorityFromSession:s=>s.refreshToken!,refresh:async()=>{rotations++;return session();},revoke:async()=>{}},{load:async()=>structuredClone(saved),save:async value=>{if(fail)throw new Error('storage unavailable');saved=structuredClone(value);}},async()=>crypto.randomUUID(),timeout);
 return {service,saved:()=>saved,rotations:()=>rotations,setFailure:()=>{fail=true;}};
}
test('profile metadata refresh preserves credentials and context without rotation or extra fields',async()=>{
 const f=fixture();await f.service.adopt(session(),{profileId:'profile'});
 await f.service.refreshProfiles(scope,async current=>{assert.equal(current.accessToken,'access');assert.ok(!('refreshToken' in current));return {accountId:'account',profiles:[{id:'profile',name:'After',extra:'discard'}]};});
 assert.deepEqual(f.service.getSnapshot().session?.profiles,[{id:'profile',name:'After'}]);
 assert.equal(f.saved()?.active?.session.refreshToken,'refresh');assert.equal(f.saved()?.active?.authority,'refresh');assert.equal(f.saved()?.active?.context.profileId,'profile');assert.equal(f.rotations(),0);
});
test('logout fences an in-flight metadata response without republishing the old account',async()=>{
 const f=fixture();await f.service.adopt(session());let release!:(value:{accountId:string;profiles:{id:string;name:string}[]})=>void;let entered!:()=>void;
 const started=new Promise<void>(resolve=>entered=resolve);
 const pending=f.service.refreshProfiles(scope,()=>{entered();return new Promise(resolve=>release=resolve);});
 await started;const logout=f.service.logout();release({accountId:'account',profiles:[{id:'profile',name:'After'}]});
 await assert.rejects(pending,/Account changed/);await logout;assert.equal(f.service.getSnapshot().phase,'signedOut');assert.equal(f.saved()?.active,undefined);
});
test('cross-family and invalid metadata cannot replace stored profiles',async()=>{
 const f=fixture();await f.service.adopt(session());let calls=0;
 await assert.rejects(f.service.refreshProfiles({...scope,familyId:'other'},async()=>{calls++;return {accountId:'account',profiles:[]};}));assert.equal(calls,0);
 for(const payload of [{accountId:'foreign',profiles:[]},{accountId:'account',profiles:[{id:'duplicate',name:'A'},{id:'duplicate',name:'B'}]},{accountId:'account',profiles:[{id:'profile',name:'a'.repeat(121)}]}])await assert.rejects(f.service.refreshProfiles(scope,async()=>payload),/invalid/);
 assert.deepEqual(f.service.getSnapshot().session?.profiles,[{id:'profile',name:'Before'}]);
});
test('metadata timeout leaves the accepted snapshot unchanged and aborts the read',async()=>{
 const f=fixture(20);await f.service.adopt(session());let signal:AbortSignal|undefined;
 await assert.rejects(f.service.refreshProfiles(scope,async(_session,current)=>{signal=current;return new Promise(()=>{});}),/timed out/);
 assert.equal(signal?.aborted,true);assert.equal(f.service.getSnapshot().session?.profiles[0].name,'Before');
});
test('metadata storage failure does not publish or retain an uncommitted name',async()=>{
 const f=fixture();await f.service.adopt(session());f.setFailure();await assert.rejects(f.service.refreshProfiles(scope,async()=>({accountId:'account',profiles:[{id:'profile',name:'After'}]})),/storage unavailable/);
 assert.equal(f.service.getSnapshot().session?.profiles[0].name,'Before');assert.equal((await f.service.accessSession()).profiles[0].name,'Before');
});
