import {test} from 'node:test';
import assert from 'node:assert/strict';
import {ReceiverHost,parseReceiverInboxEvent} from '../src/remote-playback.ts';
import {SOCIAL_PROTOCOL} from '../src/social-playback.ts';

const receiver={id:'rcv_1',kind:'portico',deviceId:'tv-1',displayName:'Living room',platform:'tvos',keyFingerprint:'A'.repeat(43),supportedCommands:['load','play','pause','seek','stop'],grantPolicy:'per-device',authorizationRevision:'1',state:'active',presence:'online',lastSeenAt:'x',createdAt:'x',updatedAt:'x'};
const grant=(state:string)=>({id:'grt_1',receiverId:'rcv_1',controllerDeviceId:'phone-1',controllerDisplayName:'Phone',receiverKeyFingerprint:'A'.repeat(43),allowedCommands:['load','play','pause','seek','stop'],authorizationRevision:'1',state,expiresAt:'x',createdAt:'x',decidedAt:state==='pending'?null:'x'});
const env=(body:Record<string,unknown>)=>({protocolVersion:SOCIAL_PROTOCOL,serverTime:'x',...body});
const device={deviceId:'tv-1',displayName:'Living room',platform:'tvos',keyFingerprint:'A'.repeat(43)};
const player={load:()=>{},state:()=>({itemId:'',playbackId:'',playing:false,positionSeconds:0})};
const settle=async()=>{for(let i=0;i<100;i++)await new Promise(setImmediate);};

/** A held-open notifications event stream the test drives frame by frame. */
function fakeStream(){
 const enc=new TextEncoder();
 let controller:{enqueue(chunk:Uint8Array):void;close():void}|undefined,opens=0;
 const open=async(signal:AbortSignal):Promise<Response>=>{
  opens++;
  const stream=new ReadableStream<Uint8Array>({start(c){controller=c;}});
  signal.addEventListener('abort',()=>{try{controller?.close();}catch{}},{once:true});
  return new Response(stream,{headers:{'Content-Type':'text/event-stream'}});
 };
 return {
  open,
  opens:()=>opens,
  send:(event:string,data:string)=>controller?.enqueue(enc.encode(`event: ${event}\ndata: ${data}\n\n`)),
  hint:(kind:string,receiverId?:string)=>controller?.enqueue(enc.encode(`event: receiver.inbox_changed\ndata: ${JSON.stringify(receiverId===undefined?{kind}:{kind,receiverId})}\n\n`)),
  drop:()=>{try{controller?.close();}catch{}controller=undefined;},
 };
}

function hostWithStream(box:{grants:unknown[];handoffs:unknown[]}){
 const calls:{path:string;method:string}[]=[];
 const api={request:async<T,>(path:string,method='GET'):Promise<T>=>{
  calls.push({path,method});
  if(path==='/v1/receivers')return env({receiver}) as T;
  if(path.endsWith('/inbox'))return env({receiverId:'rcv_1',grants:box.grants,handoffs:box.handoffs}) as T;
  return env({}) as T;
 }};
 const events=fakeStream();
 const host=new ReceiverHost({api,device,player,pollMs:30000,heartbeatMs:45000,rand:()=>0.5,events:events.open});
 const inbox=()=>calls.filter(c=>c.path.endsWith('/inbox')).length;
 return {host,calls,inbox,events};
}

test('receiver inbox hints accept the three kinds and skip anything newer or malformed',()=>{
 assert.deepEqual(parseReceiverInboxEvent({kind:'grant',receiverId:'rcv_1'}),{kind:'grant',receiverId:'rcv_1'});
 assert.deepEqual(parseReceiverInboxEvent({kind:'handoff',receiverId:'rcv_1'}),{kind:'handoff',receiverId:'rcv_1'});
 assert.deepEqual(parseReceiverInboxEvent({kind:'resync'}),{kind:'resync'});
 for(const bad of [null,undefined,'grant',[],{},{kind:'change',revision:3},'GRANT',{kind:'GRANT'},{kind:'grant',receiverId:''},{kind:'grant',receiverId:42},{kind:'grant',receiverId:'x'.repeat(201)}])
  assert.equal(parseReceiverInboxEvent(bad),undefined);
});

test('a grant event triggers exactly one inbox fetch; other frames are ignored',async(t)=>{
 t.mock.timers.enable({apis:['setTimeout','Date']});
 try{
  const box:{grants:unknown[];handoffs:unknown[]}={grants:[],handoffs:[]};
  const {host,inbox,events}=hostWithStream(box);
  host.start();await settle();
  assert.equal(events.opens(),1);
  const before=inbox();
  events.send('heartbeat','{"revision":7}');
  events.send('change','{"revision":8,"resync":false}');
  events.hint('grant','rcv_1');
  await settle();
  assert.equal(inbox()-before,1,'one fetch for the grant, none for notification frames');
  host.dispose();
 }finally{t.mock.timers.reset();}
});

test('resync refreshes the inbox (a waiting grant is asked about)',async(t)=>{
 t.mock.timers.enable({apis:['setTimeout','Date']});
 try{
  const box:{grants:unknown[];handoffs:unknown[]}={grants:[],handoffs:[]};
  const {host,events}=hostWithStream(box);
  host.start();await settle();
  assert.equal(host.getSnapshot().asking,undefined);
  box.grants=[grant('pending')];
  events.hint('resync');
  await settle();
  assert.equal(host.getSnapshot().asking?.controllerDisplayName,'Phone');
  host.dispose();
 }finally{t.mock.timers.reset();}
});

test('the connected stream suppresses the 30 s poll, keeping the 60 s safety poll',async(t)=>{
 t.mock.timers.enable({apis:['setTimeout','Date']});
 try{
  const {host,inbox,events}=hostWithStream({grants:[],handoffs:[]});
  host.start();await settle();
  assert.equal(inbox(),1,'polls once immediately on start');
  t.mock.timers.tick(30000);await settle();
  assert.equal(inbox(),1,'no 30 s poll while the stream is connected');
  assert.equal(events.opens(),1,'the stream was not dropped');
  t.mock.timers.tick(30000);await settle();
  assert.equal(inbox(),2,'the 60 s safety poll still runs');
  host.dispose();
 }finally{t.mock.timers.reset();}
});

test('a dropped stream resumes the 30 s poll, then backs off again on reconnect',async(t)=>{
 t.mock.timers.enable({apis:['setTimeout','Date']});
 try{
  const {host,inbox,events}=hostWithStream({grants:[],handoffs:[]});
  host.start();await settle();
  assert.equal(inbox(),1);
  events.drop();await settle();
  assert.equal(events.opens(),1,'reconnect waits out its backoff');
  // The drop wakes the poll loop: one immediate refresh, then the 30 s cadence (not the 60 s wait).
  assert.equal(inbox(),2,'a dropped stream refreshes at once');
  t.mock.timers.tick(5000);await settle();
  assert.equal(events.opens(),2,'the stream reconnects');
  t.mock.timers.tick(25000);await settle();
  assert.equal(inbox(),3,'30 s after the drop, not 60 s after start');
  t.mock.timers.tick(30000);await settle();
  assert.equal(inbox(),3,'reconnected: the safety poll backs off again');
  t.mock.timers.tick(30000);await settle();
  assert.equal(inbox(),4);
  host.dispose();
 }finally{t.mock.timers.reset();}
});

test('playing pauses everything: hints are dropped and no poll runs',async(t)=>{
 t.mock.timers.enable({apis:['setTimeout','Date']});
 try{
  const {host,inbox,events}=hostWithStream({grants:[],handoffs:[]});
  host.start();await settle();
  host.setPlayerActive(true);
  const before=inbox();
  events.hint('grant','rcv_1');
  events.hint('resync');
  t.mock.timers.tick(90000);await settle();
  assert.equal(inbox(),before,'no event fetch and no poll while the player is in the foreground');
  assert.equal(events.opens(),1,'the stream itself stays up while paused');
  host.setPlayerActive(false);
  t.mock.timers.tick(60000);await settle();
  assert.ok(inbox()>before,'polling resumes when idle again');
  host.dispose();
 }finally{t.mock.timers.reset();}
});
