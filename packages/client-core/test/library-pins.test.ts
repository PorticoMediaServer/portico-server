import test from 'node:test';
import assert from 'node:assert/strict';
import {parseLibraryNavigationPins,parsePinOrder,parsePinnedLibraries} from '../src/library-pins.ts';

const library=(id:string,pinned:boolean,patch:Record<string,unknown>={})=>({id,name:id.toUpperCase(),kind:'movie',defaultView:'browse',pinned,...patch});

test('a library this app cannot read is left out of the listing and a repeated one keeps its first row',()=>{
 const libraries=parsePinnedLibraries({items:[library('a',true),library('broken',true,{pinned:'yes'}),library('a',true,{name:'Again'}),library('b',false,{kind:'hologram',futureField:1}),null],futureField:true});
 assert.deepEqual(libraries,[library('a',true),library('b',false,{kind:'hologram'})]);
});

test('the size of the listing and its outer shape still refuse it',()=>{
 assert.throws(()=>parsePinnedLibraries({items:Array.from({length:2049},(_,i)=>library('l'+i,false))}),{code:'invalid_pins'});
 assert.throws(()=>parsePinnedLibraries({items:{}}),{code:'invalid_pins'});
 assert.throws(()=>parsePinnedLibraries([]),{code:'invalid_pins'});
});

test('an arrangement that is sent back whole stays exact',()=>{
 assert.deepEqual(parsePinOrder({serverId:'server',viewerFence:'fence',revision:2,order:[{kind:'view',id:'a'}],futureField:1},'server').order,[{kind:'view',id:'a'}]);
 for(const patch of [{order:[{kind:'shelf',id:'a'}]},{order:[{kind:'view',id:'a'},{kind:'view',id:'a'}]},{order:[{kind:'view'}]},{revision:-1},{viewerFence:''},{serverId:'other'},{order:Array.from({length:201},(_,i)=>({kind:'view',id:'v'+i}))}])
  assert.throws(()=>parsePinOrder({serverId:'server',viewerFence:'fence',revision:2,order:[],...patch},'server'),{code:'invalid_pins'},JSON.stringify(patch));
 for(const patch of [{pinnedLibraryIds:['a','a']},{pinnedLibraryIds:['a',7]},{revision:'3'},{pinnedLibraryIds:Array.from({length:101},(_,i)=>'l'+i)}])
  assert.throws(()=>parseLibraryNavigationPins({pinnedLibraryIds:['a'],revision:3,...patch}),{code:'invalid_pins'},JSON.stringify(patch));
});
