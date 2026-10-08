import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readEventStream,type ServerEvent} from '../src/sse.ts';

const stream=(chunks:string[])=>new Response(new ReadableStream({start(c){const e=new TextEncoder();for(const x of chunks)c.enqueue(e.encode(x));c.close();}}));

test('frames are read across arbitrary chunk boundaries, with multi-line data, ids and comments',async()=>{
 const seen:ServerEvent[]=[];
 await readEventStream(stream([': hello\n\neve','nt: log\nda','ta: {"a":1}\nid: 7\n\n','data: one\ndata: two\n\r\n','event: heartbeat\n\n']),e=>seen.push(e));
 assert.deepEqual(seen.map(e=>[e.event,e.data,e.id]),[['log','{"a":1}','7'],['message','one\ntwo','7'],['heartbeat','','7']]);
});

test('a refused stream is an error with its status, and an abort ends the read quietly',async()=>{
 await assert.rejects(readEventStream(new Response('no',{status:403}),()=>{}),(e:any)=>e.status===403);
 const controller=new AbortController();let push:(s:string)=>void=()=>{};
 const open=new Response(new ReadableStream({start(c){const e=new TextEncoder();push=s=>c.enqueue(e.encode(s));}}));
 const seen:string[]=[];const done=readEventStream(open,e=>{seen.push(e.data);controller.abort();},controller.signal);
 push('data: first\n\n');await done;assert.deepEqual(seen,['first']);
});

test('an oversized frame is refused rather than buffered without bound',async()=>{
 await assert.rejects(readEventStream(stream(['data: '+'x'.repeat(300000)]),()=>{}),/oversized/);
});
