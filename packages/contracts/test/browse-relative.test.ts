import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {stripTypeScriptTypes} from 'node:module';
import {schemas} from '../src/schemas.ts';

// Use the actual generated browse node schema in the production runtime.
const nodeSchema = (schemas as any).BrowseLibraryResponse.properties.applied.properties.query;
async function decoder() {
 const template=readFileSync(new URL('../src/runtime.ts',import.meta.url),'utf8');
 const source=template.replace("import { schemas } from './schemas.ts';",`const schemas = ${JSON.stringify({BrowseNode:nodeSchema})};`);
 return await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
}

test('published relative browse predicate advertises date fields and whole day limits', async () => {
 const relative=nodeSchema.oneOf.find((s:any)=>s.properties?.operator?.enum?.includes('within-last-days'));
 assert.deepEqual([...relative.properties.field.enum].sort(),['dateAdded','lastPlayedAt','releaseDate']);
 assert.deepEqual(relative.properties.value,{type:'integer',minimum:1,maximum:36500});
 const api=await decoder();
 for(const field of relative.properties.field.enum)for(const value of [1,30,36500]) {
  const node={field,operator:'within-last-days',value};assert.deepEqual({...api.decode('BrowseNode',node)},node);
 }
 for(const value of [null,'30',true,[],{},0,-1,36501,1.5])assert.throws(()=>api.decode('BrowseNode',{field:'dateAdded',operator:'within-last-days',value}));
 assert.throws(()=>api.decode('BrowseNode',{field:'dateAdded',operator:'within-last-days'}));
 assert.throws(()=>api.decode('BrowseNode',{field:'year',operator:'within-last-days',value:30}));
 assert.doesNotThrow(()=>api.decode('BrowseNode',{field:'dateAdded',operator:'at-least',value:'2026-09-23'}));
});
