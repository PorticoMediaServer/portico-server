import test from 'node:test';
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {privacy,safePath} from './review-smoke-privacy.mjs';

test('diagnostic text removes credentials, URLs and opaque identifiers',()=>{
 const value=randomBytes(24).toString('hex');
 const result=privacy({kind:'text',privateValues:[value],value:'Username '+value+' https://example.test/media?cap='+value+' Bearer '+value});
 assert.ok(!result.includes(value));assert.ok(!result.includes('https://'));assert.ok(result.startsWith('Username '));
});
test('API paths retain the operation while removing captured identifiers',()=>{
 const value=randomBytes(24).toString('base64url');
 assert.equal(safePath('/v1/direct/profiles/'+value+'/select'),'/v1/direct/profiles/:id/select');
 assert.equal(safePath('/v1/me'),'/v1/me');
 assert.equal(safePath('/assets/index-BoPfPJiR.js'),'/assets/index-BoPfPJiR.js');
});
