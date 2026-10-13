import test from 'node:test';
import assert from 'node:assert/strict';
import {decode} from '../src/optional/runtime.ts';
import {schemas} from '../src/optional/schemas.ts';

function requestSchema(fragment:string): keyof typeof schemas {
 const name=Object.keys(schemas).find(name=>name.endsWith('Request')&&name.includes(fragment));
 assert.ok(name); return name as keyof typeof schemas;
}

test('nullable subtitle contract keeps string and null and refuses other values',()=>{
 const name=requestSchema('SendDeviceCommand');
 for(const subtitles of [null,'eng']) assert.equal((decode(name,{type:'setTracks',subtitles}) as {subtitles:unknown}).subtitles,subtitles);
 for(const subtitles of [1,{},[],true]) assert.throws(()=>decode(name,{type:'setTracks',subtitles}));
});

test('nullable library contract keeps null and string lists and refuses malformed lists',()=>{
 const name=requestSchema('EditAccountProfile');
 for(const allowedLibraries of [null,[],['library']]) assert.doesNotThrow(()=>decode(name,{expectedRevision:1,policy:{serverId:'server',allowedLibraries}}));
 for(const allowedLibraries of [1,{},[1],true]) assert.throws(()=>decode(name,{expectedRevision:1,policy:{serverId:'server',allowedLibraries}}));
});

test('analysis control decoder preserves acknowledgements and queued jobs',async()=>{
 const api=await import('../src/runtime.ts');
 const name='PostItemsIdAnalysisJobsJobControlResponse';
 for(const status of ['pause','resume','cancel']) assert.equal((api.decode(name,{status}) as {status:string}).status,status);
 const job={id:'job',status:'queued',processed:0,analyzed:0,warnings:0};
 const decoded=api.decode(name,job) as typeof job;
 assert.equal(decoded.id,job.id);assert.equal(decoded.warnings,0);
 for(const invalid of [null,[],1,{}, {status:1}]) assert.throws(()=>api.decode(name,invalid));
});

test('oneOf decoding refuses overlapping and absent response branches',async()=>{
 const {readFileSync}=await import('node:fs');
 const {stripTypeScriptTypes}=await import('node:module');
 const shapes={Unique:{oneOf:[
  {type:'object',properties:{name:{type:'string'}},required:['name']},
  {type:'object',properties:{count:{type:'integer'}},required:['count']},
 ]}};
 const template=readFileSync(new URL('../../../apikit/templates/runtime.ts.txt',import.meta.url),'utf8');
 const source=template.replace("import { schemas } from './schemas.ts';",`const schemas = ${JSON.stringify(shapes)};`);
 const api=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 assert.equal(api.decode('Unique',{name:'title'}).name,'title');
 assert.equal(api.decode('Unique',{count:1}).count,1);
 for(const value of [{},null,[],{name:'title',count:1}]) assert.throws(()=>api.decode('Unique',value));
});
