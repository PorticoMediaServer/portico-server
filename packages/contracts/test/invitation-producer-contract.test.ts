import test from 'node:test';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {mkdtempSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {decode} from '../src/runtime.ts';
import {schemas} from '../src/schemas.ts';
import {parseDirectSignIn} from '../../client-core/src/profile-management.ts';

// Capture fresh, marshalled acceptance/sign-in producer results, including a
// confirmed server factor and a real pending-device record. No hand-built
// successful snapshot or credential-bearing fixture can mask a producer drift.
test('published acceptance decoder and client consume actual producer states',()=>{
 const directory=mkdtempSync(join(tmpdir(),'portico-invitation-contract-'));
 try {
  const path=join(directory,'responses.json');
  execFileSync('go',['test','./internal/httpapi','-run','^TestInvitationAcceptanceProducerContract$','-count=1'],{
   cwd:fileURLToPath(new URL('../../../server/',import.meta.url)),
   env:{...process.env,PORTICO_TEST_INVITATION_RESPONSES:path},
   timeout:120_000,stdio:'pipe',
  });
  const responses=JSON.parse(readFileSync(path,'utf8'));
  assert.deepEqual(Object.keys(responses).sort(),['challenge','device-pending','local','signed-in']);
  for(const [state,raw] of Object.entries(responses) as [string,any][]){
   const result=decode('PostAccessInvitationsAcceptResponse',raw) as any;
   assert.equal('notes' in result,false);
   assert.equal('email' in result,false);
   assert.doesNotThrow(()=>decode('PostAccessInvitationsAcceptResponse',{...raw,futureField:true}));
   if(state==='local'){
    assert.equal(result.username,'contract_local');
    assert.ok(Array.isArray(result.allowedLibraries));
   } else {
    assert.equal(parseDirectSignIn(raw).kind,state);
    assert.equal(parseDirectSignIn(result).kind,state);
    if(state==='challenge'||state==='device-pending'){
     assert.equal(result.profiles,null);
     assert.equal(result.account.allowedLibraries,null);
    }
   }
  }
  // Additive presentation fields may overlap intermediate states deliberately.
  // anyOf must retain a usable challenge rather than rejecting two matches.
  const overlap={...responses['device-pending'],challenge:responses.challenge.challenge};
  assert.equal(parseDirectSignIn(decode('PostAccessInvitationsAcceptResponse',overlap)).kind,'challenge');
  for(const bad of [
   {}, null, [],
   {...responses.challenge,challenge:undefined},
   {...responses.challenge,challenge:{token:42,expiresAt:'invalid'}},
   {...responses['device-pending'],deviceId:undefined},
   {...responses['device-pending'],installationId:42},
   {...responses['device-pending'],deviceApprovalPending:'true'},
   {...responses['signed-in'],session:undefined},
   {...responses['signed-in'],profiles:null},
   {...responses['signed-in'],account:{...responses['signed-in'].account,allowedLibraries:null}},
  ]) assert.throws(()=>decode('PostAccessInvitationsAcceptResponse',bad));
  const variants=(schemas.PostAccessInvitationsAcceptResponse as any).anyOf;
  assert.equal(variants.length,4);
  for(const branch of variants){
   assert.equal('notes' in branch.properties,false);
   assert.equal('email' in branch.properties,false);
  }
 } finally {
  rmSync(directory,{recursive:true,force:true});
 }
});
