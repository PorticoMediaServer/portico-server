// Runs the actual shared client gate against a stopped loopback Hosted service.
// Credentials arrive through stdin, never argv or repository files.
import assert from 'node:assert/strict';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
let input='';for await(const part of process.stdin) input+=part;
const config=JSON.parse(input);input='';
for(const origin of [config.hosted,config.server]) assert.equal(new URL(origin).hostname,'127.0.0.1');
const {hostedGate,ApiError,HttpDeviceAuthorizationApi}=await import(pathToFileURL(resolve(config.serverTree,'packages/client-core/src/index.ts')));
const gate=hostedGate(config.hosted);let calls=0;
const request=async()=>{calls++;return fetch(config.hosted+'/health',{signal:AbortSignal.timeout(3000)});};
await assert.rejects(gate.run('interactive',request));
assert.ok(gate.blockedFor()>=10000);
await assert.rejects(gate.run('automatic',request),e=>e instanceof ApiError&&e.code==='account_service_recovering');
assert.equal(calls,1,'automatic work must never touch the network while the gate is open');
const response=await fetch(config.server+'/v1/me',{headers:{Authorization:'Bearer '+config.token},signal:AbortSignal.timeout(3000)});
assert.equal(response.status,200,'local admitted session survives the Hosted outage');
// Also exercise the real activation adapter while that origin's gate is open.
// Record an owning-client gap without weakening the backend acceptance checks.
const fetchRequest=globalThis.fetch;let devicePollRequests=0;
globalThis.fetch=(...args)=>{devicePollRequests++;return fetchRequest(...args);};
try{await assert.rejects(new HttpDeviceAuthorizationApi(config.hosted).poll('device_unknown'));}
finally{globalThis.fetch=fetchRequest;}
console.log(JSON.stringify({sharedGateBlocksAutomatic:true,devicePollBypassesGate:devicePollRequests>0,devicePollRequests}));
