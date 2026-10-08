import test from 'node:test';
import assert from 'node:assert/strict';
import {webMediaCompleted} from '../src/bridge/web-playback-binding.ts';
test('the final browser pause is terminal before ended is dispatched, so reapplying play cannot loop',()=>{
 assert.equal(webMediaCompleted({ended:false,paused:true,currentTime:120,duration:120}),true);
 assert.equal(webMediaCompleted({ended:true,paused:false,currentTime:120,duration:120}),true);
 assert.equal(webMediaCompleted({ended:false,paused:true,currentTime:119.9,duration:120}),false);
 assert.equal(webMediaCompleted({ended:false,paused:true,currentTime:0,duration:NaN}),false);
 assert.equal(webMediaCompleted({ended:false,paused:true,currentTime:0,duration:0}),false);
 assert.equal(webMediaCompleted({ended:false,paused:true,currentTime:120,duration:Infinity}),false);
});
