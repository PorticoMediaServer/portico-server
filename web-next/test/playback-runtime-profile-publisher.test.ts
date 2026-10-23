import test from 'node:test';
import assert from 'node:assert/strict';
import {keepBrowserProfilePublished} from '../src/platform/playback/client-profile.ts';
import {installPlaybackEnvironment} from '../src/platform/playback/environment.ts';
import {webDeclaredProfile} from '../../packages/client-core/src/client-profile.ts';

test('background publication and output changes use the installed native capability provider',async t=>{
  const originalWindow=Object.getOwnPropertyDescriptor(globalThis,'window');
  const originalDevices=Object.getOwnPropertyDescriptor(navigator,'mediaDevices');
  const page=new EventTarget(),devices=new EventTarget();
  Object.defineProperty(globalThis,'window',{configurable:true,value:page});
  Object.defineProperty(navigator,'mediaDevices',{configurable:true,value:devices});
  let stop=()=>{},undo=()=>{};
  t.after(()=>{stop();undo();if(originalWindow)Object.defineProperty(globalThis,'window',originalWindow);else delete (globalThis as any).window;if(originalDevices)Object.defineProperty(navigator,'mediaDevices',originalDevices);else delete (navigator as any).mediaDevices;});
  let calls=0;
  undo=installPlaybackEnvironment({capabilities:async()=>webDeclaredProfile({app:'native-'+(++calls)})});
  const sent:unknown[]=[];
  stop=keepBrowserProfilePublished({requestRaw:async(_path,_method,options)=>{sent.push(options?.body);return {status:204,headers:{}};}},'scope');
  const settle=async()=>{for(let i=0;i<5;i++)await new Promise(setImmediate);};
  await settle();assert.equal((sent[0] as any).client.app,'native-1');
  devices.dispatchEvent(new Event('devicechange'));await settle();
  assert.equal((sent[1] as any).client.app,'native-2','browser probing never overwrites actual installed native facts');
  stop();devices.dispatchEvent(new Event('devicechange'));await settle();assert.equal(sent.length,2);
});
