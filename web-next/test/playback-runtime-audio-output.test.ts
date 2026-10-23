import test from 'node:test';
import assert from 'node:assert/strict';
import {configureDestination, destinationChannels, outputClockLag} from '../src/platform/playback/audio/output.ts';
import {BrowserEqualizer, flatEqualizer, parseEqualizer, parseAutoEqFixedBand} from '../src/platform/playback/audio/equalizer.ts';
import {probeAudioDecode} from '../src/platform/playback/audio/capabilities.ts';
import {MediaCarrier} from '../src/platform/playback/audio/carrier.ts';

test('actual destination setter/readback bounds declarations and surround bypasses the stereo carrier', async () => {
  const endpoint = {maxChannelCount:8,channelCount:2} as AudioDestinationNode;
  assert.equal(configureDestination(endpoint),8); assert.equal(endpoint.channelCount,8);
  const locked = {maxChannelCount:8,get channelCount(){return 2;},set channelCount(_value:number){throw new Error('fixed');}} as AudioDestinationNode;
  assert.equal(configureDestination(locked),2);
  assert.equal(destinationChannels({maxChannelCount:1,channelCount:1}),1);
  const links: unknown[] = [];
  const output = {connect:(to:unknown)=>links.push(to),disconnect(){}} as unknown as AudioNode;
  const carrier = new MediaCarrier({destination:endpoint,createMediaStreamDestination:()=>{throw new Error('surround must not use stereo stream');}},output,()=>{});
  assert.equal(carrier.carrying,false); assert.deepEqual(links,[endpoint]); carrier.dispose();
  for (const channels of [1,2,6,8]) {
    const caps = await probeAudioDecode(async()=>true,'blink',channels);
    assert.ok(caps.every(cap => cap.maxChannels! <= channels));
    assert.equal(caps.find(cap=>cap.codec==='pcm')!.maxChannels,channels);
  }
});

test('output clock uses real timestamps only on the direct path and qualifies stream-carrier estimates', () => {
  const clock = {state:'running',currentTime:10,baseLatency:0.01,outputLatency:0.03,getOutputTimestamp:()=>({contextTime:9.8,performanceTime:1000})} as AudioContext;
  const real = outputClockLag(clock,1050,false)!;
  assert.equal(real.basis,'timestamp'); assert.ok(Math.abs(real.seconds-0.15)<1e-9);
  assert.deepEqual(outputClockLag(clock,1050,true),{seconds:0.04,basis:'unmeasured-carrier'});
  assert.deepEqual(outputClockLag({...clock,getOutputTimestamp:()=>({contextTime:NaN,performanceTime:0})},1050,false),{seconds:0.04,basis:'latency'});
  assert.equal(outputClockLag({...clock,state:'suspended'},1050,false),undefined);
  assert.equal(outputClockLag({...clock,currentTime:100},1050,false)!.seconds,0.5);
});

test('EQ rejects malformed state and unsupported nodes never report processing enabled', () => {
  for (const patch of [{bandsDb:[1]},{bandsDb:Array(10).fill(NaN)},{preampDb:25},{enabled:'yes'}]) assert.throws(()=>parseEqualizer({...flatEqualizer,...patch}),/invalid_equalizer/);
  let disconnected=0;
  const parameter = () => ({value:1,cancelScheduledValues(){},setTargetAtTime(){}});
  const context = {currentTime:0,sampleRate:48000,createGain:()=>({gain:parameter(),connect(){},disconnect(){disconnected++;}}),createBiquadFilter:()=>{throw new Error('unsupported');}} as unknown as BaseAudioContext;
  const equalizer=new BrowserEqualizer(context);equalizer.apply(flatEqualizer);
  for (let i=0;i<2;i++) {
    assert.throws(()=>equalizer.apply({...flatEqualizer,enabled:true}),/unsupported/);
    assert.equal(equalizer.settings.enabled,false);
  }
  assert.equal(disconnected,2,'failed intermediate gain nodes are released');equalizer.dispose();
});

test('local fixed-band AutoEq imports stay off and reject incompatible parametric coefficients', () => {
  // Authored format fixture, not a headphone measurement or a bundled dataset.
  const text='Preamp: -4 dB\n'+[31,63,125,250,500,1000,2000,4000,8000,16000].map((frequency,i)=>`Filter ${i+1}: ON PK Fc ${frequency} Hz Gain ${i===5?3:0} dB Q 1.41`).join('\n');
  const settings=parseAutoEqFixedBand(text);assert.equal(settings.enabled,false);assert.equal(settings.preampDb,-4);assert.equal(settings.bandsDb[5],3);
  assert.throws(()=>parseAutoEqFixedBand(text.replace('Q 1.41','Q 7.0')),/invalid_equalizer/);
  assert.throws(()=>parseAutoEqFixedBand(text.replace('Fc 31 Hz','Fc 500 Hz')),/invalid_equalizer/);
});
