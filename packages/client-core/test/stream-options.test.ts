import test from 'node:test';
import assert from 'node:assert/strict';
import {parseStreamOptions} from '../src/console.ts';

test('history facets validate bounded rows',async()=>{
 const facets={libraries:[{id:'music',label:'Music'}],viewers:[{id:'local:owner:primary',label:'Owner'}]};
 assert.deepEqual(parseStreamOptions(facets),facets);
 for(const bad of [{libraries:[],viewers:null},{libraries:[{id:'',label:'bad'}],viewers:[]},{libraries:[],viewers:Array(501).fill({id:'a',label:'b'})}])assert.throws(()=>parseStreamOptions(bad));
});
