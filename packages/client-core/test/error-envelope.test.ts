import test from 'node:test';
import assert from 'node:assert/strict';
import {ApiError,HttpLocalApi} from '../src/index.ts';
import {presentError} from '../src/presentation/errors.ts';
import {enUS} from '../../i18n/src/index.ts';

// BE-API-08: Hosted's claim endpoints answer {error:{code,message,retryable}} instead of a flat
// {code}. Every client reader takes the code from the thrown ApiError, which the transports derive
// from error.code with the flat code as fallback (client-core index.ts, web bridge/account.ts).
test('claim failures read error.code, with the flat code as fallback', async()=>{
 const failure=(body:unknown)=>async()=>new Response(JSON.stringify(body),{status:409,headers:{'Content-Type':'application/json'}});
 const api=(body:unknown)=>new HttpLocalApi('http://127.0.0.1:19447','token',failure(body));
 const enveloped=await api({error:{code:'claim_stale',message:'That connection request was already answered.',retryable:false}}).request('/v1/server-claims/approve-code','POST',{}).catch(x=>x);
 assert.ok(enveloped instanceof ApiError);
 assert.equal(enveloped.code,'claim_stale');
 assert.equal(enveloped.message,'That connection request was already answered.');
 assert.equal(enveloped.retryable,false);
 const flat=await api({code:'claim_stale'}).request('/v1/server-claims/approve-code','POST',{}).catch(x=>x);
 assert.ok(flat instanceof ApiError);
 assert.equal(flat.code,'claim_stale','endpoints that still send the flat shape keep working');
});

// BE-API-08: the five registered server errors have their own sentences, keeping the server's
// retry meaning (unavailable/pending: Try again; gone artwork: Refresh; unsupported action: none).
test('the five registered errors map to catalogue messages with truthful actions',()=>{
 const cases=[
  ['metadata_unavailable',503,'error.metadataUnavailable','Metadata is temporarily unavailable. Try again in a moment.','try-again'],
  ['artwork_pending',404,'error.artworkPending','This artwork isn’t ready yet. Try again in a moment.','try-again'],
  ['artwork_gone',410,'error.artworkGone','This artwork is no longer available. Refresh to see the latest.','refresh'],
  ['web_unavailable',503,'error.webUnavailable','The web app is temporarily unavailable. Try again in a moment.','try-again'],
  ['method_not_allowed',405,'error.methodNotAllowed','This action isn’t supported here.','none'],
 ] as const;
 for(const [code,status,messageId,body,action] of cases){
  const p=presentError({status,code,message:'ignored server text'},'library');
  assert.equal(p.messageId,messageId,code);
  assert.ok(p.messageId in enUS,`${messageId} is in the catalogue`);
  assert.equal(p.body,body,code);
  assert.equal(p.action,action,code);
  assert.equal(p.code,code);
 }
});

// o3/api-hygiene: a write that left out the revision it read answers 428 revision_required; it
// reads as "this changed, refresh", like any 409 conflict (never a raw message or "try again").
test('428 revision_required reads as a change to refresh',()=>{
 const withCode=presentError({status:428,code:'revision_required',message:'ignored server text'},'library',{operation:'save'});
 const conflict=presentError({status:409,code:'conflict',message:'ignored server text'},'library',{operation:'save'});
 assert.equal(withCode.category,'changed');
 assert.equal(withCode.messageId,conflict.messageId);
 assert.equal(presentError({status:428,message:'ignored'},'library',{operation:'save'}).category,'changed');
});
