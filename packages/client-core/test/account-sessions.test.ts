import test from 'node:test';import assert from 'node:assert/strict';
import {AccountSessionReviewService,type AccountSessionReviewApi} from '../src/account-sessions.ts';
const scope={accountId:'account',familyId:'current'};
const row={id:'other',mode:'native',createdAt:'2026-09-05T10:00:00Z',expiresAt:'2026-09-12T10:00:00Z',status:'active',current:false};
const current={...row,id:'current',mode:'browser',current:true};
const page=(items:unknown[]=[current,row],nextCursor='')=>({accountId:'account',currentFamilyId:'current',observedAt:'2026-09-05T12:00:00Z',items,nextCursor});
const api=(fn:(p:string,m?:string)=>Promise<unknown>):AccountSessionReviewApi=>({request:fn as AccountSessionReviewApi['request']});
const deferred=()=>{let resolve!:(v:unknown)=>void;return {promise:new Promise(r=>resolve=r),resolve};};
test('records only whitelisted facts and separates current from other session revocation',async()=>{
 let revoked=false;const service=new AccountSessionReviewService({scope,api:api(async(p,m)=>{if(m==='DELETE'){revoked=true;return {id:'other',revoked:true,current:false};}return page([current,{...row,status:revoked?'revoked':'active',refreshToken:'never retain'}]);})});await service.refresh();assert.equal('refreshToken' in service.getSnapshot().items[1],false);await service.revoke('other');assert.equal(service.getSnapshot().phase,'ready');assert.equal(service.getSnapshot().items[1].status,'revoked');assert.equal(service.getSnapshot().items[0].current,true);assert.throws(()=>service.revoke('other'),/Refresh/);service.dispose();
});
test('current-family receipt ends local review without issuing an unauthorized follow-up read',async()=>{
 let reads=0;const service=new AccountSessionReviewService({scope,api:api(async(p,m)=>m==='DELETE'?{id:'current',revoked:true,current:true}:(reads++,page()))});await service.refresh();await service.revoke('current');assert.equal(service.getSnapshot().phase,'ended');assert.equal(service.getSnapshot().items.length,0);assert.equal(reads,1);service.dispose();
});
test('expired authority ends view without claiming the requested remote operation succeeded',async()=>{
 const service=new AccountSessionReviewService({scope,api:api(async(p,m)=>{if(m==='DELETE')throw Object.assign(new Error('expired'),{status:401});return page();})});await service.refresh();await service.revoke('other');assert.equal(service.getSnapshot().phase,'ended');assert.match(service.getSnapshot().notice,/Sign in again/);assert.doesNotMatch(service.getSnapshot().notice,/revoked|removed/i);service.dispose();
});
test('read failure removes stale controls and preserves true error instead of empty success',async()=>{
 let fail=false;const service=new AccountSessionReviewService({scope,api:api(async()=>{if(fail)throw new Error('offline');return page();})});await service.refresh();fail=true;await service.refresh();assert.equal(service.getSnapshot().phase,'error');assert.equal(service.getSnapshot().items.length,0);assert.throws(()=>service.revoke('other'),/Refresh/);service.dispose();
});
test('cross-account, wrong current-family and invented current markers fail closed',async()=>{
 for(const raw of [{...page(),accountId:'elsewhere'},{...page(),currentFamilyId:'different'},page([{...row,current:true}])]){const service=new AccountSessionReviewService({scope,api:api(async()=>raw)});await service.refresh();assert.equal(service.getSnapshot().error?.code,'invalid_session_review');assert.equal(service.getSnapshot().items.length,0);service.dispose();}
});
test('late account-view completion cannot publish a current-session end',async()=>{
 const late=deferred();const service=new AccountSessionReviewService({scope,api:api(async(p,m)=>m==='DELETE'?late.promise:page())});await service.refresh();const pending=service.revoke('current');service.dispose();late.resolve({id:'current',revoked:true,current:true});await pending;assert.notEqual(service.getSnapshot().phase,'ended');
});
test('deadline retains exact target for safe retry and rejects another overlapping intent',async()=>{
 const commands:string[]=[];let first=true;const never=deferred();const service=new AccountSessionReviewService({scope,timeoutMs:10,api:api(async(p,m)=>{if(m==='DELETE'){commands.push(p);if(first){first=false;return never.promise;}return {id:'other',revoked:true,current:false};}return page();})});await service.refresh();const pending=service.revoke('other');assert.equal(service.revoke('other'),pending);assert.throws(()=>service.revoke('current'),/Wait/);await pending;assert.equal(service.getSnapshot().retryId,'other');assert.throws(()=>service.revoke('current'),/Resolve/);await service.retryRevoke();assert.equal(commands.length,2);assert.equal(commands[0],commands[1]);assert.equal(service.getSnapshot().retryId,null);service.dispose();never.resolve({id:'other',revoked:true,current:false});
});
test('pagination replaces one40row page and refresh resets continuation',async()=>{
 const forty=Array.from({length:40},(_,n)=>({...row,id:'page-'+n}));const paths:string[]=[];const service=new AccountSessionReviewService({scope,api:api(async p=>{paths.push(p);return p.includes('?')?page([row]):page(forty,'next-page');})});await service.refresh();await service.next();assert.equal(service.getSnapshot().items.length,1);assert.equal(service.getSnapshot().hasPrevious,true);await service.previous();assert.equal(service.getSnapshot().items.length,40);await service.refresh();assert.equal(service.getSnapshot().hasPrevious,false);assert.equal(paths.at(-1),'/v1/account/sessions');service.dispose();
});
test('expired/revoked rows cannot be acted on and malformed receipts do not claim success',async()=>{
 const service=new AccountSessionReviewService({scope,api:api(async(p,m)=>m==='DELETE'?{id:'someone-else',revoked:true,current:false}:page([current,{...row,status:'expired'}]))});await service.refresh();assert.throws(()=>service.revoke('other'),/Refresh/);await service.revoke('current');assert.equal(service.getSnapshot().mutationError?.code,'invalid_session_review');assert.equal(service.getSnapshot().retryId,'current');assert.equal(service.getSnapshot().notice,'');service.dispose();
});

test('obsolete review field is refused even when false',async()=>{
 const service=new AccountSessionReviewService({scope,api:api(async()=>({...page(),legacySessionsPresent:false}))});
 await service.refresh();assert.equal(service.getSnapshot().error?.code,'invalid_session_review');service.dispose();
});
