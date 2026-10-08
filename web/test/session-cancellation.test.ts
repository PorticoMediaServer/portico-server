import test from 'node:test';
import assert from 'node:assert/strict';
import {signInHintFor} from '../src/app/sign-in-hint.ts';
import {foregroundRetry, jittered} from '../src/app/foreground-retry.ts';
import {ViewerSelectionCommit, selectedViewerRecord} from '../../packages/client-core/src/viewer-selection.ts';
import * as signInErrors from '../src/app/sign-in-errors.ts';
import * as accountSwitch from '../src/app/account-switch.ts';
import {CodedError} from '../../packages/client-core/src/server-messages.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';
const deferred = () => { let resolve!: () => void; const promise = new Promise<void>(r => resolve = r); return {promise, resolve}; };
const tick = () => new Promise(r => setTimeout(r, 0));
const session = (profile: string) => ({accessToken: profile, expiresAt: new Date(Date.now()+3600000).toISOString(), authorizationHorizon: new Date(Date.now()+7200000).toISOString(), sessionFamilyId: 'family-'+profile, tokenGeneration: '1', viewer: {authority: 'local' as const, accountId: 'account', profileId: profile, serverId: 'server', role: 'member' as const}});
async function fixture(boundary: string) {
  const h=hooks(), wait=deferred(), entered=deferred();
  const previous=session('previous'), candidate=session('candidate');
  let native:any={session:previous}, saved:any=selectedViewerRecord('https://server.example',previous);
  const revoked:string[]=[], trusts:unknown[]=[];
  class Viewer {
    state:any={phase:'ready',serverUrl:'https://server.example',session:previous};
    subscribe=()=>()=>{};getSnapshot=()=>this.state;
    clear=()=>{this.state={phase:'signedOut'};};select=(serverUrl:string,session:any)=>{this.state={phase:'ready',serverUrl,session};};
  }
  class Api {
    baseUrl:string;token:string;
    constructor(url:string,token=''){this.baseUrl=url;this.token=token;}
    async request(path:string){
      if(path==='/v1/me'){if(boundary==='verify'){entered.resolve();await wait.promise;}return{viewer:session(this.token).viewer};}
      if(path==='/v1/direct')return{serverId:'server',account:{id:'account',username:'User'},profiles:[]};
    }
    async system(){return{};}
    async logout(){revoked.push(this.token);}
  }
  const env={storage:{read:async()=>native,change:async(fn:any)=>{native=fn(native);}}};
  const storage={read:async()=>saved,write:async(v:any)=>{if(v?.session.accessToken==='candidate'&&boundary==='selected'){entered.resolve();await wait.promise;}saved=v;}};
  const account={api:{origin:'https://hosted.example'},service:{subscribe:()=>()=>{},getSnapshot:()=>({phase:'signedOut'}),updateContext:async()=>{}}};
  const modules:any={
    react:h.react,'@core/index.ts':{HttpLocalApi:Api,ViewerService:Viewer,parseServerPresence:()=>undefined},'./known-servers':{knownServers:{remember:async()=>{},setProfile:async()=>{}}},'./sign-in-hint':{signInHintFor},'./foreground-retry':{foregroundRetry,jittered},'@core/viewer-selection.ts':{ViewerSelectionCommit,selectedViewerRecord},
    '@core/server-connections.ts':{rememberNativeSession:async(_:any,s:any,e:any,__?:any,current=()=>true)=>{
      const before=await e.storage.read();if(boundary==='native'){entered.resolve();await wait.promise;}
      if(!current())throw new Error('cancelled');await e.storage.change(()=>({session:s}));
      if(!current()){await e.storage.change(()=>before);throw new Error('cancelled');}
    }},
    '@core/profile-management.ts':{parseDirectSnapshot:(v:any)=>v,selectDirectProfile:async(_:unknown,__:unknown,profile:string)=>{if(boundary.startsWith('issuance')&&profile==='candidate'){entered.resolve();await wait.promise;if(boundary==='issuance-error')throw new Error('late issuance failure');}return{session:session(profile),trustedSelection:{token:'trust'}};}},
    '@core/session-selection.ts':{},'@core/portico-servers.ts':{ServerListWatcher:class{adopt(){}getRecord(){return null}foreground(){return Promise.resolve('not-due')}refresh(){return Promise.resolve('unchanged')}},isAccessRefused:()=>false,isSessionMigrated:()=>false,isClockSkew:()=>false,challengeNotProven:'NOT PROVEN',acceptPorticoCustody:async()=>{}},'@core/hosted-gate.ts':{hostedGate:()=>({run:(_k:string,f:()=>unknown)=>f(),blockedFor:()=>0})},'./server-list-cache':{lastServer:()=>undefined,rememberLastServer:()=>{},cachedServerList:()=>undefined,serverListStorage:()=>({load:()=>null,save:()=>{}}),storeServerList:()=>{}},'./i18n':{currentI18n:()=>({t:(k:string)=>k})},'../bridge/account':{browserAccount:()=>account},'../bridge/api':{},
    '../bridge/server-connections':{browserConnectionEnvironment:()=>env,trackBrowserConnection:()=>{},forgetBrowserConnection:async(s:any)=>env.storage.change((v:any)=>v?.session.accessToken===s.accessToken?undefined:v)},
    '../bridge/viewer-selection-storage':{browserViewerStorage:storage},'../bridge/restore-policy':{},
    '../bridge/profile-trust':{browserInstallation:()=> 'installation',browserTrustLoaded:async()=>{},readBrowserTrust:()=>undefined,saveBrowserTrust:(t:unknown)=>trusts.push(t)},
    '@core/installation.ts':{setCredentialLock:()=>{},setInstallationIdentity:()=>{},validInstallationId:()=>false},'@core/session-refresh.ts':{LocalSessionRefresher:class{stop(){}wake(){}}},'./sign-in-errors':signInErrors,'../bridge/audio/bytes':{audioByteCache:{clear(){}}},'./account-switch':accountSwitch,'@core/server-messages.ts':{CodedError},'@core/presentation/index.ts':{friendlyHost:()=>undefined},'./device':{describeBrowser:()=>'Browser'},'./errors':{errorText:(e:any)=>e?.message==='cancelled'||/cancel/i.test(String(e?.message))?'':'presented'},
  };
  const app=await componentModule(new URL('../src/app/session.tsx',import.meta.url),modules);
  const render=()=>h.render(()=>app.SessionProvider({children:null})).props.value;
  render().direct.openProfileChooser();await tick();render();
  return{render,entered,wait,revoked,trusts,viewer:app.viewer,native:()=>native,saved:()=>saved};
}
for(const boundary of ['issuance','native','selected','verify']){
  test(`direct Cancel during ${boundary} keeps old viewer and credentials and retires late candidate`,async()=>{
    const f=await fixture(boundary);const attempt=f.render().direct.chooseProfile('candidate');await f.entered.promise;
    f.render().direct.cancelPending();f.wait.resolve();await attempt;await tick();const view=f.render();
    assert.equal(f.viewer.getSnapshot().session?.accessToken,'previous');assert.equal(f.saved()?.session.accessToken,'previous');assert.equal(f.native()?.session.accessToken,'previous');
    assert.equal(view.busy,false);assert.equal(view.error,'');assert.equal(view.direct.pending,undefined);
    assert.deepEqual(f.revoked,['candidate']);assert.deepEqual(f.trusts,[]);
  });
}
test('disconnect invalidates issuance before it can start a new commit',async()=>{
  const f=await fixture('issuance');const attempt=f.render().direct.chooseProfile('candidate');await f.entered.promise;
  f.render().disconnectServer();f.wait.resolve();await attempt;await tick();
  assert.equal(f.viewer.getSnapshot().session,undefined);assert.equal(f.saved(),null);assert.equal(f.native(),undefined);
  assert.deepEqual(f.revoked.sort(),['candidate','previous']);assert.deepEqual(f.trusts,[]);
});
test('normal direct selection commits, retires prior session, and only then saves trust',async()=>{
  const f=await fixture('verify');const attempt=f.render().direct.chooseProfile('candidate');await f.entered.promise;
  assert.deepEqual(f.trusts,[]);f.wait.resolve();await attempt;await tick();
  assert.equal(f.viewer.getSnapshot().session?.accessToken,'candidate');assert.equal(f.saved()?.session.accessToken,'candidate');
  assert.equal(f.native()?.session.accessToken,'candidate');assert.deepEqual(f.revoked,['previous']);assert.equal(f.trusts.length,1);
});

test('late issuance cannot replace a newer successful selection',async()=>{
 const f=await fixture('issuance');const first=f.render().direct.chooseProfile('candidate');await f.entered.promise;
 await f.render().direct.chooseProfile('replacement');f.wait.resolve();await first;await tick();
 assert.equal(f.viewer.getSnapshot().session?.accessToken,'replacement');assert.equal(f.saved()?.session.accessToken,'replacement');assert.equal(f.native()?.session.accessToken,'replacement');
 assert.deepEqual(f.revoked.sort(),['candidate','previous']);assert.equal(f.render().busy,false);assert.equal(f.render().error,'');assert.equal(f.trusts.length,1);
});
test('late issuance error stays silent after Cancel',async()=>{
 const f=await fixture('issuance-error');const first=f.render().direct.chooseProfile('candidate');await f.entered.promise;
 f.render().direct.cancelPending();f.wait.resolve();await first;
 assert.equal(f.render().error,'');assert.equal(f.render().busy,false);assert.deepEqual(f.revoked,[]);
});
