import {routePath as hostedRoutePath,encodedRoutePath as hostedEncodedRoutePath} from '../../contracts/src/hosted/routes.ts';
import {routePath} from '../../contracts/src/routes.ts';
import {requestPorticoChallenge,type HostedRequestApi} from './portico-servers.ts';
import {parseDirectSignIn} from './profile-management.ts';
import type {HttpLocalApi} from './index.ts';

export type ReturnPurpose='setup'|'switch'|'change-owner';
export type ReturnPending=Readonly<{serverId:string;purpose:ReturnPurpose;claimOperationId:string;registrationId:string;state:string;verifier:string;serverChallenge:string;installationId:string;createdAt:number}>;
/** Implementations must use protected credential storage. Plain persistent
 * drafts, localStorage and URLs are not storage for the verifier. */
export interface ReturnPendingStore {read():Promise<ReturnPending|undefined>;change(update:(old:ReturnPending|undefined)=>ReturnPending|undefined):Promise<void>}
const token=()=>{const b=new Uint8Array(32);globalThis.crypto.getRandomValues(b);return b64(b);};
const b64=(b:Uint8Array)=>btoa(String.fromCharCode(...b)).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
export async function prepareSameTabReturn(api:HttpLocalApi,store:ReturnPendingStore,input:Pick<ReturnPending,'purpose'|'claimOperationId'|'registrationId'|'installationId'>,signal?:AbortSignal){
 const serverId=api.getRouteConnection()?.pin.serverId;if(!serverId)throw new Error('Verify the server before continuing.');
 if(!/^[A-Za-z0-9_-]{32,128}$/.test(input.installationId)||!/^ret_[A-Za-z0-9_-]{43}$/.test(input.registrationId))throw new Error('The registered server return is invalid.');
 const serverChallenge=await requestPorticoChallenge(api,serverId,signal),verifier=token(),state=token();
 const pending:ReturnPending={...input,serverId,serverChallenge,verifier,state,createdAt:Date.now()};
 await store.change(()=>pending);
 const codeChallenge=b64(new Uint8Array(await globalThis.crypto.subtle.digest('SHA-256',new TextEncoder().encode(verifier))));
 const {verifier:_,createdAt:__,...body}=pending;return {...body,codeChallenge};
}
// The Hosted account client calls this only after its lookup exposes an actual
// current registered route. No return URL is accepted or inferred here.
export function validateRegisteredReturnLocation(location:string,registration:{id:string;origin:string},state:string,transaction:string):string {
 const origin=registration.origin;
 if(!/^https:\/\/(?:[a-z0-9.-]+|\[[0-9a-f:]+\])(?::[0-9]{1,5})?$/.test(origin))throw new Error('The registered server address is invalid.');
 const expected=new URL(origin),url=new URL(location),values=new URLSearchParams(url.hash.slice(1));
 if(expected.protocol!=='https:'||expected.username||expected.password||expected.search||expected.hash||!location.startsWith(origin+'/auth/portico-return#')||url.pathname!=='/auth/portico-return'||url.search||url.username||url.password||[...values.keys()].sort().join(',')!=='code,state,transaction'||values.get('state')!==state||values.get('transaction')!==transaction||!/^src_[A-Za-z0-9_-]{20,128}$/.test(values.get('code')??''))throw new Error('The return did not match the registered server.');
 return location;
}
export async function completeRegisteredReturn(hosted:HostedRequestApi,start:Omit<ReturnPending,'verifier'|'createdAt'>&{codeChallenge:string},registration:{id:string;origin:string},signal?:AbortSignal){
 if(registration.id!==start.registrationId)throw new Error('The return registration changed.');
 const begun=await hosted.request<{transactionId:string}>(hostedRoutePath('server_return_start'),'POST',start,signal);
 if(!/^srt_[A-Za-z0-9_-]{20,128}$/.test(begun.transactionId))throw new Error('The registered return did not start.');
 const out=await hosted.request<{location:string}>(hostedEncodedRoutePath('server_return_complete',{id:encodeURIComponent(begun.transactionId)}),'POST',{},signal);
 return validateRegisteredReturnLocation(out.location,registration,start.state,begun.transactionId);
}
/** Scrub synchronously, before reading protected state or awaiting any network
 * work. Both invalid and failed exchanges leave a clean history entry. */
export async function consumeSameTabReturn(api:Pick<HttpLocalApi,'request'|'getRouteConnection'>,store:ReturnPendingStore,location:Pick<Location,'href'>,history:Pick<History,'replaceState'>,continuation:{requestId?:string;setupToken?:string;expectedRevision?:string}={},signal?:AbortSignal){
 const url=new URL(location.href),fragment=url.hash;url.hash='';history.replaceState(null,'',url.pathname+url.search);
 const params=new URLSearchParams(fragment.slice(1));let pending:ReturnPending|undefined;
 try{
  pending=await store.read();await store.change(()=>undefined);
  if(url.pathname!=='/auth/portico-return'||url.search||!pending||Date.now()-pending.createdAt>5*60_000||pending.createdAt>Date.now()+60_000||api.getRouteConnection()?.pin.serverId!==pending.serverId||[...params.keys()].sort().join(',')!=='code,state,transaction'||params.getAll('state').length!==1||params.get('state')!==pending.state||!/^src_[A-Za-z0-9_-]{20,128}$/.test(params.get('code')??'')||!/^srt_[A-Za-z0-9_-]{20,128}$/.test(params.get('transaction')??''))throw new Error('This return is no longer valid. Start again.');
  const out=parseDirectSignIn(await api.request(routePath('post_auth_portico_return_exchange'),'POST',{code:params.get('code'),transactionId:params.get('transaction'),verifier:pending.verifier,registrationId:pending.registrationId,serverChallenge:pending.serverChallenge,installationId:pending.installationId,purpose:pending.purpose,expectedRevision:continuation.expectedRevision??'0',...(continuation.requestId?{requestId:continuation.requestId}:{}),...(continuation.setupToken?{setupToken:continuation.setupToken}:{})},signal));
  if(out.kind==='signed-in'&&(out.snapshot.serverId!==pending.serverId||out.resetFileOwnerSession))throw new Error('The server return did not match this server.');return out;
 }finally{pending=undefined;await store.change(()=>undefined);}
}
