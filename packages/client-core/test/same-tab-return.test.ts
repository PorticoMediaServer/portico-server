import test from 'node:test';
import assert from 'node:assert/strict';
import {consumeSameTabReturn,validateRegisteredReturnLocation,type ReturnPending,type ReturnPendingStore} from '../src/same-tab-return.ts';
const pending:ReturnPending={serverId:'server',purpose:'setup',claimOperationId:'op',registrationId:'ret_x',state:'s'.repeat(43),verifier:'v'.repeat(43),serverChallenge:'c'.repeat(32),installationId:'i'.repeat(43),createdAt:Date.now()};
for(const kind of ['network','bad-state','duplicate','expired','wrong-server','bad-path','missing'] as const)test(`return fragment is scrubbed before ${kind} failure`,async()=>{
 let saved:ReturnPending|undefined=kind==='missing'?undefined:{...pending,...(kind==='expired'?{createdAt:Date.now()-400_000}:{})},scrubbed=false,calls=0;
 const store:ReturnPendingStore={read:async()=>{assert.ok(scrubbed);return saved},change:async f=>{saved=f(saved)}};
 const api={getRouteConnection:()=>({pin:{serverId:kind==='wrong-server'?'wrong':'server'}}),request:async()=>{assert.ok(scrubbed);assert.equal(saved,undefined);calls++;throw new Error('offline')}};
 const fragment=`code=src_${'a'.repeat(43)}&state=${kind==='bad-state'?'wrong':pending.state}&transaction=srt_${'b'.repeat(43)}${kind==='duplicate'?'&code=extra':''}`;
 await assert.rejects(consumeSameTabReturn(api as never,store,{href:`https://server.example/${kind==='bad-path'?'other':'auth/portico-return'}#${fragment}`},{replaceState:(_a,_b,url)=>{assert.equal(url,kind==='bad-path'?'/other':'/auth/portico-return');scrubbed=true;}}));assert.equal(saved,undefined);assert.equal(calls,kind==='network'?1:0);
});

test('registered return consumer refuses arbitrary URLs and altered immutable bindings',()=>{
 const origin='https://server.example:32500',fragment=`code=src_${'a'.repeat(43)}&state=saved&transaction=srt_fixture`,valid=origin+'/auth/portico-return#'+fragment;
 assert.equal(validateRegisteredReturnLocation(valid,{id:'ret_fixture',origin},'saved','srt_fixture'),valid);
 for(const bad of [valid.replace('server.example:32500','attacker.example'),valid.replace(':32500',':443'),valid.replace('server.example','server.examp1e'),valid.replace('server.example','user@server.example'),valid.replace('https:','http:'),'javascript:alert(1)',valid.replace('/auth/portico-return','/other'),valid.replace('#','?x=1#'),valid+'&state=extra',valid.replace('state=saved','state=wrong'),valid.replace('transaction=srt_fixture','transaction=srt_other')])assert.throws(()=>validateRegisteredReturnLocation(bad,{id:'ret_fixture',origin},'saved','srt_fixture'));
});
