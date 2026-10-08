import test from 'node:test';
import assert from 'node:assert/strict';
import {renditionPlan,validateAudioPlan} from '../src/audio-selection.ts';
import {parseSessionDelivery,type DeliveryAudioRendition} from '../src/delivery.ts';
test('delivery diagnostics carry all three audio renditions into the selection handshake',()=>{
 const renditions:DeliveryAudioRendition[]=Array.from({length:3},(_,ordinal)=>({ordinal,streamIndex:ordinal+1,action:'copy',codec:ordinal===1?'ac3':'aac',channels:ordinal===1?6:2,bitrateBps:192000,language:'en',label:'Track '+ordinal,default:ordinal===2}));
 const delivery=parseSessionDelivery({mode:'hls',strategy:'copy_remux',qualityId:'auto',reasonCodes:[],throttled:false,convertedThroughSeconds:0,toneMap:false,playedRetentionSeconds:0,throttleBufferSeconds:60,hardware:{backend:'software',stages:[]},streams:[],audioRenditions:renditions});
 assert.equal(delivery.audioRenditions?.length,3);
 const plan=renditionPlan({id:'session',generation:2},{id:'source',factsRevision:1},delivery.audioRenditions!);
 assert.equal(plan.renditions.length,3);assert.equal(plan.defaultRenditionId,'audio-2');assert.equal(plan.renditions[1].codec,'ac3');
 assert.throws(()=>validateAudioPlan({...plan,renditions:[plan.renditions[0],plan.renditions[0]]}));
});
