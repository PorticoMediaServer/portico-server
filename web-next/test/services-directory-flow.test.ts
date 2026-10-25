import test from 'node:test';
import assert from 'node:assert/strict';
import {ServicesDirectoryFlow, type DirectoryItem} from '../src/services/directory-flow.ts';

const server: DirectoryItem={id:'srv_'+'a'.repeat(43),name:'Original server',baseUrl:'https://server.example',publicKey:'p'.repeat(43),policyRevision:1,endpointVerified:true,lastSeenAt:'2026-10-25T12:00:00Z',presence:{lastSeenAt:'2026-10-25T12:00:00Z'},ownedByMe:false,role:'member',status:'active'};
const directory=(items: DirectoryItem[])=>({items,version:'1',watch:'1',membershipEnding:undefined});
const deferred=<T,>()=>{let resolve!:(value:T)=>void;const promise=new Promise<T>(done=>{resolve=done;});return{promise,resolve};};
function fixture(){
 let current=true,reads=0,connections=0,disposed=0;
 let read=async()=>directory([server]),connect=async()=>({origin:'https://server.example',dispose:()=>{disposed++;}});
 const destinations:string[]=[];
 const flow=new ServicesDirectoryFlow(directory([server]),{current:()=>current,read:()=>{reads++;return read();},connect:()=>{connections++;return connect();},navigate:origin=>{destinations.push(origin);},empty:()=>{},now:()=>1000});
 return{flow,destinations,get reads(){return reads;},get connections(){return connections;},get disposed(){return disposed;},retire:()=>{current=false;},read:(value:typeof read)=>{read=value;},connect:(value:typeof connect)=>{connect=value;}};
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
