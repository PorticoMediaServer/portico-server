import test from 'node:test';
import assert from 'node:assert/strict';
import {validDirectPassword} from '../src/direct-account.ts';

test('accepts a policy password: 8+ chars, upper, lower and a non-letter', () => {
  assert.equal(validDirectPassword('Abcdef1!'), true);
  assert.equal(validDirectPassword('Correct Horse 9'), true);
  assert.equal(validDirectPassword('Pässwörd1'), true);
});

test('rejects short, single-case, letter-only and oversize values', () => {
  assert.equal(validDirectPassword(''), false);
  assert.equal(validDirectPassword('Ab1!'), false);
  assert.equal(validDirectPassword('abcdefgh1!'), false, 'no uppercase');
  assert.equal(validDirectPassword('ABCDEFGH1!'), false, 'no lowercase');
  assert.equal(validDirectPassword('Abcdefgh'), false, 'letters only');
  // 72-byte UTF-8 cap: 71 ASCII chars fit, 73 do not.
  assert.equal(validDirectPassword('A' + 'a'.repeat(69) + '1'), true);
  assert.equal(validDirectPassword('A' + 'a'.repeat(71) + '1'), false);
  // Counting is by code point, not UTF-16 units: 7 emojis + 'Aa1' is 10 chars.
  assert.equal(validDirectPassword('😀'.repeat(7) + 'Aa1'), true);
  assert.equal(validDirectPassword('😀'.repeat(4) + 'Aa1'), false, '7 code points is too short');
});

test('edge: non-string-shaped and boundary input never throws', () => {
  assert.equal(validDirectPassword('Aa1!Aa1!'), true);
  assert.equal(validDirectPassword('        Aa1'), true, 'spaces count as non-letters');
  assert.equal(validDirectPassword('12345678'), false);
});


test('invitation preview has explicit public fields and rejects private recipient details',async()=>{
 const {parsePublicInvitation}=await import('../src/direct-account.ts');
 const v={serverName:'Fixture server',expiresAt:'2026-11-01T12:00:00Z',role:'member',libraries:[{name:'Films'}],allLibraries:false,signInMethod:'portico',inviterUsername:'fixture_owner'};
 assert.equal(parsePublicInvitation(v).inviterUsername,'fixture_owner');
 for(const field of ['notes','email','porticoUsername'])assert.throws(()=>parsePublicInvitation({...v,[field]:'private'}));
 for(const patch of [{signInMethod:'other'},{role:'owner'},{libraries:[{}]},{expiresAt:'bad'}])assert.throws(()=>parsePublicInvitation({...v,...patch}));
});
test('private notes use Unicode character limits and never accept control or formatting characters',async()=>{
 const {privatePersonNotes}=await import('../src/direct-account.ts');
 assert.equal(privatePersonNotes('  fixture note  '),'fixture note');assert.equal(privatePersonNotes('  '),null);assert.equal(privatePersonNotes(null),null);
 assert.equal(Array.from(privatePersonNotes('😀'.repeat(500))!).length,500);
 for(const s of ['x'.repeat(501),'private\ntext','hidden\u202e'])assert.throws(()=>privatePersonNotes(s));
});
test('invitation client recovers only code and expiry against the current revision',async()=>{
 const {PeopleInvitationsClient}=await import('../src/direct-account.ts');const calls:any[]=[];
 const api={request:async<T>(path:string,method?:string,body?:unknown)=>{calls.push({path,method,body});return {code:'x'.repeat(43),expiresAt:'2026-11-01T12:00:00Z',notes:'must never be shared'} as T;}};
 const c=new PeopleInvitationsClient(api);const link=await c.link({id:'fixture',revision:2,state:'pending'});
 assert.deepEqual(link,{code:'x'.repeat(43),expiresAt:'2026-11-01T12:00:00Z'});assert.deepEqual(calls[0],{path:'/v1/admin/access/invitations/fixture/link',method:'POST',body:{expectedRevision:2}});
 await assert.rejects(c.link({id:'fixture',revision:2,state:'accepted'}));assert.equal(calls.length,1);
});
test('person identity is username first, profile only when different and Private viewer stays private',async()=>{
 const {personIdentity}=await import('../src/direct-account.ts');
 assert.deepEqual(personIdentity('fixture_user','Kids'),{username:'fixture_user',profileName:'Kids'});
 assert.deepEqual(personIdentity('fixture_user','fixture_user'),{username:'fixture_user'});
 assert.deepEqual(personIdentity('fixture_user','Kids',true),{privateViewerKey:'together.privateViewer'});
 assert.throws(()=>personIdentity('','Kids'));
});
