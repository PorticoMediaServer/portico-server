import type {HttpLocalApi,LocalSession} from '@core/index.ts';
import {RouteConnection,sessionRoute} from '@core/route-connection';
import {restoreNativeSession,forgetNativeSession,type RememberedServer,type ServerConnectionStorage,type HostedConnectionSource,type ConnectionEnvironment} from '@core/server-connections';
import {browserAccount} from './account';

/** Version 1 is encrypted with a non-exportable key. Version 2 is plain JSON, used where the
 * page has no `crypto.subtle` (a server's web app over plain HTTP on the LAN): HTTP is
 * first-class, the browser's per-origin storage still isolates it, and on plain HTTP anyone who
 * can read the traffic already sees these tokens in transit. */
type Vault={version:1;iv:Uint8Array;cipher:ArrayBuffer}|{version:2;plain:string};
const canEncrypt=()=>!!globalThis.crypto?.subtle;
/** Native credentials are never ordinary drafts/localStorage. The non-exportable
 * AES key and authenticated encrypted record are separate IndexedDB values. */
export class BrowserProtectedStore<RecordType> {
 constructor(private readonly databaseName='portico.server-connections.v1'){}
 private async transaction<T>(mode:IDBTransactionMode,work:(store:IDBObjectStore,done:(v:T)=>void)=>void):Promise<T>{
  return new Promise((resolve,reject)=>{let db:IDBDatabase|undefined,tx:IDBTransaction|undefined,done=false,value:T;const finish=(error?:unknown)=>{if(done)return;done=true;clearTimeout(timer);db?.close();error?reject(new Error('The saved server connection could not be read or written.')):resolve(value);};const timer=setTimeout(()=>{try{tx?.abort();}catch{}finish(new Error());},5000);
   const request=indexedDB.open(this.databaseName,1);request.onupgradeneeded=()=>request.result.createObjectStore('vault');request.onerror=()=>finish(request.error);request.onblocked=()=>finish(new Error());request.onsuccess=()=>{db=request.result;if(done){db.close();return;}try{tx=db.transaction('vault',mode,mode==='readwrite'?{durability:'strict'}:undefined);if(mode==='readwrite'&&tx.durability!=='strict')throw new Error();tx.oncomplete=()=>finish();tx.onabort=()=>finish(tx?.error);tx.onerror=()=>finish(tx?.error);work(tx.objectStore('vault'),v=>{value=v;});}catch(e){try{tx?.abort();}catch{}finish(e);}};
  });
 }
 private exclusive<T>(work:()=>Promise<T>):Promise<T>{return navigator.locks.request(this.databaseName+':selected',{signal:AbortSignal.timeout(15000)},work);}
 private values(){return this.transaction<{key?:CryptoKey;record?:Vault}>('readonly',(store,done)=>{const key=store.get('key'),record=store.get('selected');record.onsuccess=()=>done({key:key.result,record:record.result});});}
 private async decode(values:{key?:CryptoKey;record?:Vault}):Promise<RecordType|undefined>{
  if(!values.record)return;
  if(values.record.version===2){if(typeof values.record.plain!=='string'||values.record.plain.length>196608)throw new Error('Saved server credentials are incomplete.');return JSON.parse(values.record.plain);}
  // An encrypted record where nothing can decrypt it is as good as absent; the next save replaces it.
  if(!canEncrypt())return;
  if(!values.key||values.record.version!==1||!(values.record.iv instanceof Uint8Array)||values.record.iv.length!==12||!(values.record.cipher instanceof ArrayBuffer)||values.record.cipher.byteLength>196608)throw new Error('Saved server credentials are incomplete.');
  const clear=await crypto.subtle.decrypt({name:'AES-GCM',iv:new Uint8Array(values.record.iv),additionalData:new TextEncoder().encode(this.databaseName)},values.key,values.record.cipher);try{return JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(clear));}finally{new Uint8Array(clear).fill(0);}
 }
 read(){return this.exclusive(async()=>this.decode(await this.values()));}
 change(update:(current:RecordType|undefined)=>RecordType|undefined):Promise<void>{return this.exclusive(async()=>{
  const values=await this.values(),current=await this.decode(values),next=update(current);
  if(next===current)return;
  if(!next){await this.transaction<void>('readwrite',(store,done)=>{store.delete('selected');done();});return;}
  const bytes=new TextEncoder().encode(JSON.stringify(next));if(bytes.length>192000)throw new Error('Saved server routes exceeded their limit.');
  if(!canEncrypt()){const plain=new TextDecoder().decode(bytes);await this.transaction<void>('readwrite',(store,done)=>{store.put({version:2,plain} satisfies Vault,'selected');done();});return;}
  let key=values.key;if(!key){if(values.record)throw new Error('The credential encryption key is missing.');key=await crypto.subtle.generateKey({name:'AES-GCM',length:256},false,['encrypt','decrypt']);}
  const iv=crypto.getRandomValues(new Uint8Array(12));let cipher:ArrayBuffer;try{cipher=await crypto.subtle.encrypt({name:'AES-GCM',iv,additionalData:new TextEncoder().encode(this.databaseName)},key,bytes);}finally{bytes.fill(0);}
  await this.transaction<void>('readwrite',(store,done)=>{if(!values.key)store.put(key,'key');store.put({version:1,iv,cipher} satisfies Vault,'selected');done();});
 });}
}
let environment:ConnectionEnvironment|undefined,active:RouteConnection|undefined,networkRevision=0;
export function browserConnectionEnvironment():ConnectionEnvironment {
 if(environment)return environment;environment={storage:new BrowserProtectedStore<RememberedServer>()};
 const changed=()=>active?.networkChanged('browser:'+ ++networkRevision);
 addEventListener('online',changed);addEventListener('offline',changed);
 const connection=(navigator as Navigator&{connection?:EventTarget}).connection;connection?.addEventListener('change',()=>{activeApi?.setTransportClass(linkType());changed();});
 document.addEventListener('visibilitychange',()=>active?.setForeground(!document.hidden));
 addEventListener('focus',()=>active?.foregroundActivity());
 document.addEventListener('pointerdown',()=>active?.foregroundActivity(),{passive:true});
 document.addEventListener('keydown',()=>active?.foregroundActivity());
 return environment;
}
/* A browser only knows its link on some platforms (Chromium on Android and ChromeOS); elsewhere it says nothing, which the server treats as a valid answer. */
const linkType=()=>(navigator as Navigator&{connection?:{type?:string}}).connection?.type;
let activeApi:HttpLocalApi|undefined;
export function trackBrowserConnection(api:HttpLocalApi){activeApi=api;api.setDeviceClass('web');api.setTransportClass(linkType());const next=api.getRouteConnection();if(active&&active!==next){const retired=active;setTimeout(()=>retired.dispose(),15000);}active=next;active?.setForeground(!document.hidden);}
export function browserHostedSource(accountId:string):HostedConnectionSource {
 const central=browserAccount();return {origin:central.api.origin,accountId,request:async<T>(path:string,signal:AbortSignal)=>{
  // Called only for explicit directory selection or failed/slow cached-route
  // recovery. Healthy local navigation/playback never renews Hosted authority.
  await central.service.synchronize();const session=await central.service.accessSession();if(session.account.id!==accountId)throw new Error('The saved server belongs to a different Portico Account.');return central.api.send<T>(path,'GET',undefined,signal,session.accessToken);
 }};
}
export async function restoreBrowserConnection(expected:LocalSession){const env=browserConnectionEnvironment(),central=browserAccount();return restoreNativeSession(env,record=>record.hosted?.origin===central.api.origin?browserHostedSource(record.hosted.accountId):undefined,expected);}
export async function forgetBrowserConnection(session:LocalSession){const env=browserConnectionEnvironment();const captured=sessionRoute(session.accessToken);await forgetNativeSession(env,session);const retained=await env.storage.read();if(retained&&sessionRoute(retained.session.accessToken)===captured)return;if(active===captured)active=undefined;setTimeout(()=>{if(active!==captured)captured?.dispose();},16000);}
