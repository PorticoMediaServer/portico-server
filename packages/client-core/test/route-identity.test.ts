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
 assert.equal(routeOrigin('http://'+alias+':32500'),'http://'+alias+':32500');
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

function proofServer(serverNow:()=>number,signer=keys.privateKey){
 return (async(_url:RequestInfo|URL,init?:RequestInit)=>{
  const asked=JSON.parse(String(init?.body));const at=serverNow();
  const payload=Buffer.from(JSON.stringify({kind:'portico.route-proof',version:'1',...pin,baseUrl:asked.baseUrl,nonce:asked.nonce,issuedAt:new Date(at).toISOString(),expiresAt:new Date(at+30000).toISOString()}));
  return new Response(JSON.stringify({payload:payload.toString('base64url'),signature:sign(null,payload,signer).toString('base64url')}),{status:200,headers:{'Content-Type':'application/json'}});
 }) as typeof fetch;
}
test('a device clock up to two minutes off in either direction still accepts a fresh proof',async()=>{
 const {probeRoute}=await import('../src/route-identity.ts');
 const server=Date.parse('2026-10-13T12:00:00.000Z');
 for(const offset of [0,45_000,110_000,-110_000]){
  assert.deepEqual(await probeRoute('http://192.168.1.20:32500',undefined,{fetcher:proofServer(()=>server),now:()=>server+offset}),pin,String(offset));
 }
});
test('clocks further apart are named as a clock problem, not as a wrong server',async()=>{
 const {probeRoute}=await import('../src/route-identity.ts');
 const server=Date.parse('2026-10-13T12:00:00.000Z');
 for(const offset of [200_000,-200_000]){
  await assert.rejects(probeRoute('http://192.168.1.20:32500',undefined,{fetcher:proofServer(()=>server),now:()=>server+offset}),(e:{code?:string})=>e.code==='clocks_disagree',String(offset));
 }
});
test('a proof signed by another key is still a wrong server, whatever the clocks say',async()=>{
 const {probeRoute}=await import('../src/route-identity.ts');
 const other=generateKeyPairSync('ed25519').privateKey;
 const server=Date.parse('2026-10-13T12:00:00.000Z');
 await assert.rejects(probeRoute('http://192.168.1.20:32500',undefined,{fetcher:proofServer(()=>server,other),now:()=>server+200_000}),(e:{code?:string})=>e.code==='identity_mismatch');
});
test('explicit HTTP origins work on public, overlay and IPv6 addresses',()=>{
 for(const origin of ['http://media.example:32500','http://100.64.1.2:32500','http://203.0.113.9:32500','http://[2001:db8::9]:32500']) assert.equal(routeOrigin(origin),origin);
 for(const origin of ['http://user:secret@media.example','http://media.example/path','http://media.example?secret=x']) assert.throws(()=>routeOrigin(origin));
});

test('signed per-family public candidates keep distinct ports and race the reachable family',async()=>{
 const {RouteConnection}=await import('../src/route-connection.ts');
 const origins=['https://ipv4-route.example:42424','https://ipv6-route.example:32500'];
 const signed=signedLAN('https://'+alias+':32500');
 const document=JSON.parse(Buffer.from(signed.payload,'base64url').toString());
 document.candidates=origins.map(baseUrl=>({baseUrl,class:'public',quality:'reachable',generation:'1'}));
 const raw=Buffer.from(JSON.stringify(document));
 const verified=await verifyRoutes({payload:raw.toString('base64url'),signature:sign(null,raw,keys.privateKey).toString('base64url')},key,expected);
 assert.deepEqual(verified.document.candidates.map(c=>c.baseUrl),origins);
 for(const reachable of origins){
  const probes:string[]=[];
  const connection=new RouteConnection({pin,logicalOrigin:origins[0],memory:{current:verified,paired:[]},probe:async origin=>{probes.push(origin);if(origin!==reachable)throw new Error('family unavailable');return pin;}});
  try{assert.equal(await connection.resolve(),reachable);assert(probes.includes(reachable));assert.equal(connection.getSnapshot().origin,reachable);}finally{connection.dispose();}
 }
});
