import test from 'node:test';import assert from 'node:assert/strict';
import {ApiError,CredentialSessionService,type CredentialRecord,type HostedSession} from '../src/index.ts';
const uuid=async()=>crypto.randomUUID();const delay=()=>new Promise(r=>setTimeout(r,10));
const session=(token='one'):HostedSession=>({accessToken:'access-'+token,refreshToken:'refresh-'+token,familyId:'family',expiresAt:new Date(Date.now()+3600000).toISOString(),refreshExpiresAt:new Date(Date.now()+86400000).toISOString(),account:{id:'a',username:'a',displayName:'A'},profiles:[{id:'p',name:'P'}]});
function store(){let value:CredentialRecord|undefined;return{load:async()=>structuredClone(value),save:async(v:CredentialRecord)=>{value=structuredClone(v)},value:()=>value}}
const api=(refresh:any,revoke=async(_token:string)=>{})=>({authorityFromSession:(s:HostedSession)=>s.refreshToken!,refresh,revoke});
test('refresh is single flight, stores receipt before request, hides refresh secret',async()=>{const storage=store();let calls=0;const service=new CredentialSessionService(api(async()=>{calls++;assert.ok(storage.value()?.active?.pendingRequestId);await delay();return session('two')}),storage,uuid);await service.adopt(session(),{profileId:'p'});assert.equal('refreshToken' in service.getSnapshot().session!,false);const [a,b]=await Promise.all([service.accessSession(true),service.accessSession(true)]);assert.equal(calls,1);assert.equal(a.accessToken,b.accessToken);assert.equal(storage.value()?.active?.pendingRequestId,undefined);assert.equal(service.getSnapshot().context?.profileId,'p')});
test('ambiguous rotation survives restart with exact receipt',async()=>{const storage=store();const ids:string[]=[];const first=new CredentialSessionService(api(async(_token:string,id:string)=>{ids.push(id);throw new Error('network')}),storage,uuid);await first.adopt(session());await assert.rejects(first.accessSession(true));const second=new CredentialSessionService(api(async(_token:string,id:string)=>{ids.push(id);return session('two')}),storage,uuid);await second.accessSession();assert.equal(ids[0],ids[1]);assert.equal(ids.length,2)});
test('logout fences a late refresh and revokes without restoring session',async()=>{const storage=store();let resolve:any;const revoked:string[]=[];const service=new CredentialSessionService(api(()=>new Promise(r=>resolve=r),async token=>{revoked.push(token)}),storage,uuid);await service.adopt(session());const pending=service.accessSession(true);await delay();const logout=service.logout();assert.equal(service.getSnapshot().phase,'signedOut');resolve(session('two'));await assert.rejects(pending);await logout;await delay();assert.equal(service.getSnapshot().phase,'signedOut');assert.equal(storage.value()?.active,undefined);assert.ok(revoked.includes('refresh-two'))});
test('storage failure before refresh blocks the network',async()=>{const storage=store();let calls=0,fail=false;const service=new CredentialSessionService(api(async()=>{calls++;return session('two')}),{load:storage.load,save:async v=>{if(fail)throw new Error('disk');await storage.save(v)}},uuid);await service.adopt(session());fail=true;await assert.rejects(service.accessSession(true));assert.equal(calls,0)});
test('cookie authority supports login callback inside complete coordinator lock',async()=>{let locked=false;let value:any;const coordinator={runExclusive:async<T>(fn:()=>Promise<T>)=>{assert.equal(locked,false);locked=true;try{return await fn()}finally{locked=false}}};const service=new CredentialSessionService({authorityFromSession:s=>({familyId:s.familyId}),refresh:async()=>{assert.ok(locked);return {...session('two'),refreshToken:undefined}},revoke:async()=>{assert.ok(locked)}},{load:async()=>value,save:async record=>{assert.ok(locked);value=structuredClone(record);if(value.active){delete value.active.session.refreshToken;value.active.session.accessToken='';}}},uuid,100,coordinator);await service.authenticate(async()=>{assert.ok(locked);return {...session(),refreshToken:undefined}});await service.accessSession(true);assert.equal(value.active.session.accessToken,'');assert.deepEqual(value.active.authority,{familyId:'family'})});
test('failed pair commit retains original receipt for retry',async()=>{const storage=store();let fail=false;const ids:string[]=[];const service=new CredentialSessionService(api(async(_token:string,id:string)=>{ids.push(id);fail=true;return session('two')}),{load:storage.load,save:async v=>{if(fail&&!v.active?.pendingRequestId){fail=false;throw new Error('atomic write failed')}await storage.save(v)}},uuid);await service.adopt(session());await assert.rejects(service.accessSession(true));await assert.rejects(service.accessSession(true));assert.equal(ids[0],ids[1]);assert.ok(storage.value()?.active?.pendingRequestId)});
test('late authentication after logout is revoked and cannot publish',async()=>{const storage=store();let resolve:any;const revoked:string[]=[];const service=new CredentialSessionService(api(async()=>session(),async token=>{revoked.push(token)}),storage,uuid,20);const pending=service.authenticate(()=>new Promise(r=>resolve=r));await delay();const logout=service.logout();resolve(session());await assert.rejects(pending);await logout;await delay();assert.equal(service.getSnapshot().phase,'signedOut');assert.ok(revoked.includes('refresh-one'))});

test('cookie family change clears stale local session and revokes only bound old authority',async()=>{
 let record:any;let currentCookieFamily='old';const revoked:string[]=[];
 const service=new CredentialSessionService({authorityFromSession:s=>({familyId:s.familyId!}),refresh:async()=>{throw new ApiError(409,'family_changed','Session changed.')},revoke:async authority=>{revoked.push(authority.familyId);if(currentCookieFamily===authority.familyId)currentCookieFamily=''}},{load:async()=>structuredClone(record),save:async v=>{record=structuredClone(v)}},uuid);
 await service.adopt({...session(),familyId:'old',refreshToken:undefined});currentCookieFamily='new';
 await assert.rejects(service.accessSession(true));assert.equal(service.getSnapshot().phase,'signedOut');assert.equal(record.active,undefined);assert.deepEqual(revoked,['old']);assert.equal(currentCookieFamily,'new');
});

test('context storage reload cannot publish a signed-in snapshot after immediate logout',async()=>{
 const storage=store();let unblock:(()=>void)|undefined;let gate=false;
 const service=new CredentialSessionService(api(async()=>session()),{load:async()=>{const captured=await storage.load();if(gate){gate=false;await new Promise<void>(resolve=>unblock=resolve)}return captured},save:storage.save},uuid);
 await service.adopt(session());gate=true;const update=service.updateContext({profileId:'other'});await delay();const logout=service.logout();const seen:string[]=[];service.subscribe(()=>seen.push(service.getSnapshot().phase));assert.equal(service.getSnapshot().phase,'signedOut');unblock!();await update;await logout;assert.ok(seen.every(phase=>phase==='signedOut'));assert.equal(storage.value()?.active,undefined);
});

test('directory receipt fields do not corrupt persisted credential selection',async()=>{
 const storage=store();const service=new CredentialSessionService(api(async()=>session()),storage,uuid);
 await service.adopt(session());
 const server={id:'server',name:'Demo',baseUrl:'https://demo.example.com',publicKey:'key',policyRevision:1,endpointVerified:true,challengeId:'receipt'};
 await service.updateContext({profileId:'p',server});
 const {parseNativeCredentialRecord}=await import('../src/credential-format.ts');
 const restored=parseNativeCredentialRecord(storage.value());
 assert.equal(restored.active?.context.server?.id,'server');
 assert.equal('endpointVerified' in restored.active!.context.server!,false);
});

test('CD-37: 100-profile wire cap retained; eight active cap retained',async()=>{
 const {parseNativeCredentialRecord}=await import('../src/credential-format.ts');
 const {parseManagedProfiles}=await import('../src/profile-management.ts');
 const profiles100=Array.from({length:100},(_,i)=>({id:`p${i}`,name:`P${i}`}));
 const sess=(profiles:any)=>({accessToken:'a'.repeat(43),refreshToken:'r'.repeat(43),familyId:'family',expiresAt:new Date(Date.now()+3600000).toISOString(),refreshExpiresAt:new Date(Date.now()+86400000).toISOString(),account:{id:'a',username:'a',displayName:'A'},profiles});
 const record=(profiles:any)=>({version:1,revocations:[],active:{session:sess(profiles),authority:'r'.repeat(43),context:{}}});
 // 100 decodes (wire safety bound); 101 fails closed.
 assert.equal(parseNativeCredentialRecord(record(profiles100)).active?.session.profiles.length,100);
 assert.throws(()=>parseNativeCredentialRecord(record([...profiles100,{id:'extra',name:'Extra'}])));
 // Creation stays capped at eight active.
 const eight=Array.from({length:8},(_,i)=>({id:`profile-${i}`,name:`Profile ${i}`,primary:i===0,position:i,art:'blue',revision:1,pinRevision:1,pinRequired:false}));
 assert.equal(parseManagedProfiles(eight).length,8);
 assert.throws(()=>parseManagedProfiles([...eight,{id:'profile-8',name:'P8',primary:false,position:8,art:'blue',revision:1,pinRevision:1,pinRequired:false}]));
});

test('a browser credential Hosted refuses (CSRF, origin, mode) ends the sign-in; other refusals do not',async()=>{
 for(const [status,code,signedOut] of [[403,'csrf_required',true],[403,'origin_denied',true],[403,'session_mode_denied',true],[403,'forbidden',false],[503,'unavailable',false]] as const){
  const storage=store();
  const service=new CredentialSessionService(api(async()=>{throw new ApiError(status,code,'refused')}),storage,uuid);
  await service.adopt(session());
  await assert.rejects(service.accessSession(true));
  assert.equal(service.getSnapshot().phase==='signedOut',signedOut,code);
  assert.equal(storage.value()?.active===undefined,signedOut,code);
 }
});
