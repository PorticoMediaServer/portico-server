import test from 'node:test';
import assert from 'node:assert/strict';
import {createHash,generateKeyPairSync,sign} from 'node:crypto';
import {privateRouteHost,routeOrigin,verifyRoutes} from '../src/route-identity.ts';

const keys=generateKeyPairSync('ed25519');
const rawKey=keys.publicKey.export({format:'der',type:'spki'}).subarray(-32);
const publicKey=rawKey.toString('base64url');
const pin={publicKey,serverId:'srv_'+createHash('sha256').update('portico.server.identity.v1\0').update(rawKey).digest('base64url'),fingerprint:createHash('sha256').update(rawKey).digest('base64url')};
const key={keyId:'routes-key',publicKey};
const expected={accountId:'account',serverId:pin.serverId};
const alias='l-0123456789abcdef0123456789abcdef.ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv';
function signedLAN(baseUrl:string){
 const now=Date.now();
 const raw=Buffer.from(JSON.stringify({kind:'portico.routes',version:'1',algorithm:'Ed25519',keyId:key.keyId,audience:expected.accountId,...pin,name:'Media server',policyRevision:1,claimGeneration:'1',credentialGeneration:'1',generation:'1',issuedAt:new Date(now).toISOString(),expiresAt:new Date(now+45*60000).toISOString(),candidates:[
  {baseUrl,class:'lan',quality:'probe_required',generation:'1'},
  {baseUrl:'https://media.example:32500',class:'public',quality:'reachable',generation:'1'},
 ]}));
 return {payload:raw.toString('base64url'),signature:sign(null,raw,keys.privateKey).toString('base64url')};
}
test('signed secure LAN aliases preserve the LAN and public candidates',async()=>{
 const origin='https://'+alias+':32500';
 const verified=await verifyRoutes(signedLAN(origin),key,expected);
 assert.equal(verified.document.candidates[0].baseUrl,origin);
 assert.equal(verified.document.candidates[0].class,'lan');
 assert.equal(verified.document.candidates[1].baseUrl,'https://media.example:32500');
 assert.equal(privateRouteHost(alias),false);
 assert.throws(()=>routeOrigin('http://'+alias+':32500'));
});
test('signed LAN aliases require the exact dedicated HTTPS name',async()=>{
 for(const origin of [
  'http://'+alias+':32500',
  'https://'+alias.replace('l-','c-')+':32500',
  'https://'+alias.replace('l-','l-short-')+':32500',
  'https://'+alias.replace('ptc-aaaaaaaaaaaaaaaaaaaa','ptc-short')+':32500',
  'https://'+alias.replace('ptc-aaaaaaaaaaaaaaaaaaaa','ptc-00000000000000000000')+':32500',
  'https://'+alias.replace('ptc-aaaaaaaaaaaaaaaaaaaa','ptc-aaaaaaaaaaaaaaaaaaab')+':32500',
  'https://'+alias+'.evil.example:32500',
  'https://evil.'+alias+':32500',
  'https://public.example:32500',
  'https://'+alias+':32500/path',
  'https://user:password@'+alias+':32500',
 ]){
  await assert.rejects(verifyRoutes(signedLAN(origin),key,expected),origin);
 }
 const evidence=signedLAN('https://'+alias+':32500');
 await assert.rejects(verifyRoutes(evidence,key,{...expected,accountId:'other'}));
 await assert.rejects(verifyRoutes({...evidence,signature:'A'.repeat(86)},key,expected));
});
