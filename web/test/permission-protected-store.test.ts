import test from 'node:test';
import assert from 'node:assert/strict';
import {LocalSessionRefresher} from '@core/session-refresh.ts';
import {parseLocalSession} from '@core/local-session.ts';
import type {RememberedServer} from '@core/server-connections.ts';
import {BrowserProtectedStore} from '../src/platform/server-connections.ts';
import {fakeIndexedDB} from './protected-store-fixture.ts';

async function setup(t:import('node:test').TestContext){
 const db=fakeIndexedDB(),had=['indexedDB','navigator'].map(name=>[name,Object.getOwnPropertyDescriptor(globalThis,name)] as const);
 Object.defineProperty(globalThis,'indexedDB',{configurable:true,value:db.indexedDB});
 Object.defineProperty(globalThis,'navigator',{configurable:true,value:{locks:{request:(_name:string,a:unknown,b?:unknown)=>(typeof a==='function'?a:b as ()=>unknown)()}}});
 t.after(()=>{for(const [name,descriptor]of had){if(descriptor)Object.defineProperty(globalThis,name,descriptor);else delete (globalThis as Record<string,unknown>)[name];}});
 const store=new BrowserProtectedStore<RememberedServer>('portico.test.permission.vault'),session=parseLocalSession({accessToken:'permission_boundary_test_access',
  sessionFamilyId:'family',deviceId:'device',installationId:'install',tokenGeneration:'1',expiresAt:'2099-01-01T00:00:00Z',authorizationHorizon:'2100-01-01T00:00:00Z',
  viewer:{authority:'local',accountId:'account',profileId:'profile',serverId:'server',role:'owner'}},{},{stored:true});
 await store.change(()=>({version:'1',selectionId:'selection',logicalOrigin:'https://portico.test',session,pin:{serverId:'server',publicKey:'k'.repeat(43),fingerprint:'f'.repeat(43)},routes:{paired:[]}}));
 let rotations=0,signedOut=0;const changes:Promise<unknown>[]=[];
 const ref=new LocalSessionRefresher({session,env:{storage:{read:()=>store.read(),change:(update,beforeCommit)=>{const p=store.change(update,beforeCommit);changes.push(p.catch(()=>{}));return p;}}},api:{request:async<T>()=>({viewer:{...session.viewer,role:'admin'}} as T),setAccessToken:()=>{},setAuthRecovery:()=>{}},
  setTimer:()=>1,clearTimer:()=>{},onRotated:()=>{rotations++;},onSignedOut:()=>{signedOut++;}});t.after(()=>ref.stop());
 return {db,store,ref,changes,counts:()=>({rotations,signedOut})};
}
function gate(){let release!:()=>void;return {promise:new Promise<void>(yes=>{release=yes;}),release:()=>release()};}
for(const retire of ['stop','cancel'] as const)test('real encrypted vault: '+retire+' during permission encryption vetoes the wrapped late commit',async t=>{
 const h=await setup(t),original=h.db.data.get('selected'),entered=gate(),waiting=gate(),encrypt=crypto.subtle.encrypt.bind(crypto.subtle);
 t.mock.method(crypto.subtle,'encrypt',async(...args:Parameters<typeof encrypt>)=>{const cipher=await encrypt(...args);entered.release();await waiting.promise;return cipher;});
 const c=new AbortController(),pending=h.ref.reconcilePermissions({signal:c.signal});await entered.promise;
 if(retire==='stop')h.ref.stop();else c.abort();
 assert.deepEqual(await pending,{kind:'superseded',reason:retire==='stop'?'owner-stopped':'interest-cancelled'});waiting.release();
 // Join the actual guarded transaction, including its adapter-wrapped rejection.
 await Promise.all(h.changes);
 assert.equal(h.db.data.get('selected'),original);assert.equal((await h.store.read())!.session.viewer.role,'owner');assert.deepEqual(h.counts(),{rotations:0,signedOut:0});
});
test('real encrypted vault: genuine encryption and wrapped database failures remain failed and preserve the selected role',async t=>{
 const h=await setup(t),original=h.db.data.get('selected'),error=new Error('real encryption failure');
 const encrypt=t.mock.method(crypto.subtle,'encrypt',async()=>{throw error;});
 const answer=await h.ref.reconcilePermissions();assert.equal(answer.kind,'failed');if(answer.kind==='failed'){assert.equal(answer.error,error);assert.equal(answer.retryable,true);}
 encrypt.mock.restore();assert.equal(h.db.data.get('selected'),original);assert.equal((await h.store.read())!.session.viewer.role,'owner');
 h.db.failOpen(1);const blocked=await h.ref.reconcilePermissions();assert.equal(blocked.kind,'failed');if(blocked.kind==='failed'){assert.ok(blocked.error instanceof Error);assert.equal(blocked.retryable,true);}
 assert.equal(h.db.data.get('selected'),original);assert.deepEqual(h.counts(),{rotations:0,signedOut:0});
});
test('real encrypted vault: committed supported role is readable before handover',async t=>{
 const h=await setup(t);assert.deepEqual(await h.ref.reconcilePermissions(),{kind:'adopted',role:'admin'});
 assert.equal((await h.store.read())!.session.viewer.role,'admin');assert.equal(h.ref.current().viewer.role,'admin');assert.deepEqual(h.counts(),{rotations:1,signedOut:0});
});
