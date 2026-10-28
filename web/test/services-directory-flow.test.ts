import test from 'node:test';
import assert from 'node:assert/strict';
import {ServicesDirectoryFlow, type DirectoryItem} from '../src/services/directory-flow.ts';
import {DirectoryAccountUnavailable, accountDirectoryFailure, directoryFailurePhase} from '../src/services/directory-errors.ts';
import {RouteError} from '@core/route-identity.ts';
import {ApiError} from '@core/index.ts';
import type {contactKnownServerForOwnerRecovery} from '@core/owner-recovery-contact.ts';

const server: DirectoryItem={id:'srv_'+'a'.repeat(43),name:'Original server',baseUrl:'https://server.example',publicKey:'p'.repeat(43),policyRevision:1,endpointVerified:true,lastSeenAt:'2026-10-25T12:00:00Z',presence:{lastSeenAt:'2026-10-25T12:00:00Z'},ownedByMe:false,role:'member',status:'active'};
const directory=(items: DirectoryItem[])=>({items,version:'1',watch:'1',membershipEnding:undefined});
const deferred=<T,>()=>{let resolve!:(value:T)=>void;const promise=new Promise<T>(done=>{resolve=done;});return{promise,resolve};};
function fixture(){
 let current=true,reads=0,connections=0,disposed=0;
 let read=async()=>directory([server]),connect=async()=>({origin:'https://server.example',dispose:()=>{disposed++;}});
 const destinations:string[]=[], contacts: Parameters<typeof contactKnownServerForOwnerRecovery>[0][]=[], purposes: (string|undefined)[]=[];
 let contact: typeof contactKnownServerForOwnerRecovery = async captured => {contacts.push({...captured});return {purpose:'owner-recovery',origin:captured.baseUrl,serverId:captured.serverId,signInURL:captured.baseUrl+'/sign-in'};};
 const flow=new ServicesDirectoryFlow(directory([server]),{current:()=>current,read:()=>{reads++;return read();},connect:()=>{connections++;return connect();},contact:(captured,options)=>contact(captured,options),navigate:(origin,purpose)=>{destinations.push(origin);purposes.push(purpose);},empty:()=>{},now:()=>1000});
 return{flow,destinations,contacts,purposes,contact:(value:typeof contact)=>{contact=value;},get reads(){return reads;},get connections(){return connections;},get disposed(){return disposed;},retire:()=>{current=false;},read:(value:typeof read)=>{read=value;},connect:(value:typeof connect)=>{connect=value;}};
}
test('directory proves exact fresh server before origin-only handoff and closes owned routes',async()=>{
 const f=fixture();await f.flow.open(server,f.flow.getSnapshot());assert.deepEqual(f.destinations,['https://server.example']);assert.equal(f.connections,1);assert.equal(f.disposed,1);f.flow.dispose();
});
test('retired original account during directory read cannot connect or navigate',async()=>{
 const f=fixture(),gate=deferred<ReturnType<typeof directory>>();f.read(()=>gate.promise);
 const pending=f.flow.open(server,f.flow.getSnapshot());f.retire();gate.resolve(directory([server]));await pending;
 assert.equal(f.connections,0);assert.deepEqual(f.destinations,[]);f.flow.dispose();
});
test('changed pin or ambiguous identity refuses rather than adopting current directory successor',async()=>{
 for(const items of [[{...server,publicKey:'q'.repeat(43)}],[server,server]]){
  const f=fixture();f.read(async()=>directory(items));await f.flow.open(server,f.flow.getSnapshot());assert.equal(f.connections,0);assert.deepEqual(f.destinations,[]);assert.equal((f.flow.getSnapshot().error as {code:string}).code,'identity_mismatch');f.flow.dispose();
 }
});
test('a late connection after disposal closes only its own route and cannot navigate',async()=>{
 const f=fixture(),gate=deferred<{origin:string;dispose:()=>void}>();f.connect(()=>gate.promise);
 const pending=f.flow.open(server,f.flow.getSnapshot());await new Promise(resolve=>setImmediate(resolve));f.flow.dispose();let closed=0;gate.resolve({origin:'https://server.example',dispose:()=>{closed++;}});await pending;
 assert.equal(closed,1);assert.deepEqual(f.destinations,[]);
});
test('old rendered actions cannot start after a fresh directory publication',async()=>{
 const f=fixture(),rendered=f.flow.getSnapshot();await f.flow.refresh(rendered);const reads=f.reads;await f.flow.open(server,rendered);assert.equal(f.reads,reads);assert.equal(f.connections,0);f.flow.dispose();
});
test('background cancellation retires exact automatic flight before late proof publication',async()=>{
 const f=fixture(),gate=deferred<{origin:string;dispose:()=>void}>();f.connect(()=>gate.promise);
 const pending=f.flow.open(server,f.flow.getSnapshot(),true);await new Promise(resolve=>setImmediate(resolve));f.flow.cancelAutomatic();let closed=0;gate.resolve({origin:'https://server.example',dispose:()=>{closed++;}});await pending;
 assert.equal(closed,1);assert.deepEqual(f.destinations,[]);assert.equal(f.flow.getSnapshot().phase,'idle');f.flow.dispose();
});

test('account outage recovery uses only the original directory address and pin without another Hosted read',async()=>{
 const f=fixture();f.read(async()=>directory([{...server,baseUrl:'https://new-directory.example'}]));
 f.connect(async()=>{throw new DirectoryAccountUnavailable(new TypeError('network'));});
 await f.flow.open(server,f.flow.getSnapshot());const reads=f.reads;
 assert.equal(f.flow.getSnapshot().status,'account-waiting');
 await f.flow.recoverOwner(f.flow.getSnapshot());
 assert.equal(f.reads,reads);assert.deepEqual(f.contacts,[{baseUrl:server.baseUrl,serverId:server.id,publicKey:server.publicKey}]);
 assert.deepEqual(f.destinations,[server.baseUrl]);assert.deepEqual(f.purposes,['owner-recovery']);f.flow.dispose();
});
test('server route failure does not masquerade as account outage or expose emergency contact',async()=>{
 const f=fixture();f.connect(async()=>{throw new RouteError('route_unavailable','unreachable');});
 await f.flow.open(server,f.flow.getSnapshot());assert.equal(f.flow.getSnapshot().status,'unreachable');
 await f.flow.recoverOwner(f.flow.getSnapshot());assert.equal(f.contacts.length,0);assert.deepEqual(f.destinations,[]);f.flow.dispose();
});
test('a late emergency proof after choosing another server cannot navigate',async()=>{
 const f=fixture(),gate=deferred<Awaited<ReturnType<typeof contactKnownServerForOwnerRecovery>>>();
 f.read(async()=>{throw new ApiError(503,'unavailable','unavailable',true);});await f.flow.open(server,f.flow.getSnapshot());
 f.contact(()=>gate.promise);const work=f.flow.recoverOwner(f.flow.getSnapshot());
 f.flow.cancelOpening(f.flow.getSnapshot());gate.resolve({purpose:'owner-recovery',origin:server.baseUrl,serverId:server.id,signInURL:server.baseUrl+'/sign-in'});await work;
 assert.deepEqual(f.destinations,[]);assert.equal(f.flow.getSnapshot().target,undefined);f.flow.dispose();
});
test('emergency contact rejects changed destinations and preserves failed-pin refusal',async()=>{
 const f=fixture();f.read(async()=>{throw new ApiError(503,'unavailable','unavailable',true);});await f.flow.open(server,f.flow.getSnapshot());
 f.contact(async()=>({purpose:'owner-recovery',origin:'https://other.example',serverId:server.id,signInURL:'https://other.example/sign-in'}));
 await f.flow.recoverOwner(f.flow.getSnapshot());assert.equal(f.flow.getSnapshot().status,'identity');assert.deepEqual(f.destinations,[]);
 await f.flow.recoverOwner(f.flow.getSnapshot());assert.deepEqual(f.destinations,[]);f.flow.dispose();
});
test('old rendered emergency action cannot adopt a refreshed directory or account',async()=>{
 const f=fixture();f.read(async()=>{throw new ApiError(503,'unavailable','unavailable',true);});await f.flow.open(server,f.flow.getSnapshot());const rendered=f.flow.getSnapshot();
 f.read(async()=>directory([{...server,baseUrl:'https://successor.example'}]));await f.flow.refresh(rendered);
 await f.flow.recoverOwner(rendered);assert.equal(f.contacts.length,0);assert.deepEqual(f.destinations,[]);f.flow.dispose();
});
test('automatic account retry respects Retry-After floor and only source-known failures classify as account outage',async()=>{
 const f=fixture();f.read(async()=>{throw new ApiError(429,'rate_limited','rate_limited',true,700);});await f.flow.open(server,f.flow.getSnapshot(),true);
 assert.equal(f.flow.retryDelay(),700);assert.equal(f.flow.getSnapshot().seconds,700);
 assert.equal(directoryFailurePhase(new RouteError('route_unavailable','server'),false),'unreachable');
 assert.equal(directoryFailurePhase(accountDirectoryFailure(new TypeError('network')),false),'account-waiting');
 assert.equal(directoryFailurePhase(accountDirectoryFailure(new ApiError(403,'forbidden','forbidden',false)),false),undefined);f.flow.dispose();
});
