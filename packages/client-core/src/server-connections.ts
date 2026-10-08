import {hostedGate} from './hosted-gate.ts';
import {ApiError,HttpLocalApi,type HostedServer,type LocalSession} from './index.ts';
import {credentialLock,installationIdentity} from './installation.ts';
import {parseLocalSession} from './local-session.ts';
import {RouteConnection,bindSessionRoute,forgetSessionRoute,sessionRoute,sessionRoute as sessionRouteForRotation,type DiscoveryRoute,type RouteMemory} from './route-connection.ts';
import {RouteError,routeBase64,probeRoute,routeOrigin,routeJSON,serverPin,sameServerPin,verifyRoutes,webRouteCrypto,type RouteCrypto,type RouteKey,type ServerPin,type SignedRoutes,type VerifiedRoutes} from './route-identity.ts';
export type RememberedServer = {
 version:'1';selectionId?:string;logicalOrigin:string;session:LocalSession;pin:ServerPin;
 hosted?:{origin:string;accountId:string;key:RouteKey};
 routes:{current?:SignedRoutes;previous?:SignedRoutes;paired:string[];lastGood?:string};
 /** A refresh sent but not yet stored: retried with this same request id, so the server replays its answer. */
 pendingRefresh?:string;
};
/** Implementations protect and atomically replace credentials + raw key + route
 * evidence together. A cache update cannot overwrite another selected family. */
export interface ServerConnectionStorage {
 read():Promise<RememberedServer|undefined>;
 change(update:(current:RememberedServer|undefined)=>RememberedServer|undefined):Promise<void>;
}
export type ConnectionEnvironment={storage:ServerConnectionStorage;fetcher?:typeof fetch;crypto?:RouteCrypto;discover?:(signal:AbortSignal)=>Promise<readonly DiscoveryRoute[]>};
export type HostedConnectionSource={origin:string;accountId:string;request:<T>(path:string,signal:AbortSignal)=>Promise<T>};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
export function parseRememberedServer(value:unknown):RememberedServer|undefined {
 if(value===null||value===undefined)return;
 if(!object(value)||value.version!=='1'||typeof value.logicalOrigin!=='string'||!object(value.pin)||!object(value.routes)||!Array.isArray(value.routes.paired)||value.routes.paired.length>8)throw new RouteError('invalid_saved_connection','Saved server connection is incomplete. Sign in again to establish its identity.');
 const session=parseLocalSession(value.session,{},{stored:true}),pin=value.pin;
 if(pin.serverId!==session.viewer.serverId||typeof pin.publicKey!=='string'||typeof pin.fingerprint!=='string'||value.logicalOrigin!==routeOrigin(value.logicalOrigin))throw new RouteError('invalid_saved_connection','Saved server identity is incomplete.');
 const result:RememberedServer={version:'1',logicalOrigin:value.logicalOrigin,session,pin:pin as ServerPin,routes:{paired:value.routes.paired.map(v=>routeOrigin(String(v))),...(typeof value.routes.lastGood==='string'?{lastGood:routeOrigin(value.routes.lastGood)}:{})}};
 if(value.pendingRefresh!==undefined){if(typeof value.pendingRefresh!=='string'||!/^[A-Za-z0-9_-]{1,128}$/.test(value.pendingRefresh))throw new RouteError('invalid_saved_connection','Saved renewal state is invalid.');result.pendingRefresh=value.pendingRefresh;}
 if(value.selectionId!==undefined){if(typeof value.selectionId!=='string'||!/^[-_A-Za-z0-9]{32}$/.test(value.selectionId))throw new RouteError('invalid_saved_connection','Saved selection identity is invalid.');result.selectionId=value.selectionId;}
 if(value.hosted!==undefined){if(!object(value.hosted)||typeof value.hosted.origin!=='string'||typeof value.hosted.accountId!=='string'||!object(value.hosted.key)||typeof value.hosted.key.keyId!=='string'||typeof value.hosted.key.publicKey!=='string'||(session.viewer.authority==='hosted'&&value.hosted.accountId!==session.viewer.accountId)||!['hosted','local'].includes(session.viewer.authority))throw new RouteError('invalid_saved_connection','Saved account route authority is incomplete.');result.hosted={origin:hostedOrigin(value.hosted.origin),accountId:value.hosted.accountId,key:{keyId:value.hosted.key.keyId,publicKey:value.hosted.key.publicKey}};}
 for(const name of ['current','previous'] as const){const v=value.routes[name];if(v!==undefined){if(!object(v)||typeof v.payload!=='string'||typeof v.signature!=='string'||v.payload.length>50000||v.signature.length!==86)throw new RouteError('invalid_saved_connection','Saved route evidence is incomplete.');result.routes[name]={payload:v.payload,signature:v.signature};}}
 return result;
}
function hostedOrigin(origin:string):string {const value=routeOrigin(origin),u=new URL(value);if(u.protocol!=='https:'&&!['localhost','127.0.0.1','[::1]'].includes(u.hostname))throw new RouteError('invalid_account_origin','Portico Account services require HTTPS.');return value;}
function memoryRecord(memory:RouteMemory):RememberedServer['routes']{return {current:memory.current?.envelope,previous:memory.previous?.envelope,paired:[...memory.paired],lastGood:memory.lastGood};}
async function routeKey(source:HostedConnectionSource,env:ConnectionEnvironment,signal:AbortSignal):Promise<RouteKey>{
 const origin=hostedOrigin(source.origin),response=await (env.fetcher??globalThis.fetch)(origin+'/v1/keys',{signal,redirect:'error',credentials:'omit',headers:{Accept:'application/json'}}),key=await routeJSON(response,signal,2048);
 if(!object(key)||typeof key.keyId!=='string'||typeof key.publicKey!=='string'||key.keyId.length>256||key.publicKey.length!==43)throw new RouteError('invalid_account_key','Account route verification is unavailable.');return {keyId:key.keyId,publicKey:key.publicKey};
}
function sameSelection(a:RememberedServer|undefined,b:RememberedServer):boolean {
 return !!a&&a.selectionId===b.selectionId&&a.pin.serverId===b.pin.serverId&&a.session.sessionFamilyId===b.session.sessionFamilyId&&
  (b.selectionId!==undefined||a.session.accessToken===b.session.accessToken);
}
function attachPersistence(connection:RouteConnection,env:ConnectionEnvironment,record:RememberedServer,hosted?:RememberedServer['hosted']){
 connection.setPersistence(memory=>env.storage.change(current=>{
  if(!sameSelection(current,record))return current;
  return {...current!,routes:memoryRecord(memory),...(hosted?{hosted}:{})};
 }));
}
async function currentDocument(source:HostedConnectionSource,env:ConnectionEnvironment,pin:ServerPin,key:RouteKey,signal:AbortSignal):Promise<{routes:VerifiedRoutes;key:RouteKey}>{
 // Asking for fresh routes is the app's own idea, never a tap: it waits for the shared circuit,
 // and connections to the same server share one request.
 const signed=await hostedGate(source.origin).run('automatic',()=>source.request<unknown>('/v1/servers/'+encodeURIComponent(pin.serverId)+'/routes',signal),'routes:'+pin.serverId);
 try{return {routes:await verifyRoutes(signed,key,{accountId:source.accountId,serverId:pin.serverId,pin},env.crypto),key};}catch(e){if(!(e instanceof RouteError)||e.code!=='invalid_signature')throw e;const freshKey=await routeKey(source,env,signal);return {routes:await verifyRoutes(signed,freshKey,{accountId:source.accountId,serverId:pin.serverId,pin},env.crypto),key:freshKey};}
}
export async function connectHostedServer(server:HostedServer,source:HostedConnectionSource,env:ConnectionEnvironment,signal:AbortSignal):Promise<{api:HttpLocalApi;connection:RouteConnection;hosted:NonNullable<RememberedServer['hosted']>}> {
 const key=await routeKey(source,env,signal),signed=server.routes??await source.request<unknown>('/v1/servers/'+encodeURIComponent(server.id)+'/routes',signal);
 const verified=await verifyRoutes(signed,key,{accountId:source.accountId,serverId:server.id},env.crypto);
 if(verified.document.publicKey!==server.publicKey||verified.document.policyRevision<server.policyRevision)throw new RouteError('identity_mismatch','The server directory and signed identity disagree.');
 const origin=verified.document.candidates.find(c=>c.baseUrl===server.baseUrl)?.baseUrl??verified.document.candidates[0]?.baseUrl;
 if(!origin)throw new RouteError('server_unavailable','This server has not published a usable address. Its owner can repair remote access from the local network.');
 const hosted={origin:hostedOrigin(source.origin),accountId:source.accountId,key};
 const connection=new RouteConnection({pin:verified.document,logicalOrigin:origin,memory:{current:verified,paired:[]},fetcher:env.fetcher,crypto:env.crypto,discover:env.discover,fresh:async signal=>{const next=await currentDocument(source,env,verified.document,hosted.key,signal);hosted.key=next.key;return next.routes;}});
 try{await connection.resolve(signal);const api=new HttpLocalApi(origin,'',env.fetcher);api.useRoutes(connection);return {api,connection,hosted};}catch(e){connection.dispose();throw e;}
}
/** Explicit local pairing precedes credential entry. A public self-signed key is
 * not silently trusted; UI must display and confirm this pin. */
export async function inspectDirectServer(origin:string,env:ConnectionEnvironment,signal:AbortSignal):Promise<ServerPin>{return probeRoute(routeOrigin(origin),undefined,{fetcher:env.fetcher,crypto:env.crypto,signal});}
export async function connectPairedServer(origin:string,pin:ServerPin,env:ConnectionEnvironment,signal:AbortSignal):Promise<{api:HttpLocalApi;connection:RouteConnection}>{
 const checked=await serverPin(pin.serverId,pin.publicKey,pin.fingerprint,env.crypto);
 const connection=new RouteConnection({pin:checked,logicalOrigin:routeOrigin(origin),memory:{paired:[routeOrigin(origin)]},fetcher:env.fetcher,crypto:env.crypto,discover:env.discover});
 try{await connection.resolve(signal);const api=new HttpLocalApi(connection.logicalOrigin,'',env.fetcher);api.useRoutes(connection);return{api,connection};}catch(e){connection.dispose();throw e;}
}
export async function rememberNativeSession(api:HttpLocalApi,session:LocalSession,env:ConnectionEnvironment,hosted?:RememberedServer['hosted'],isCurrent:()=>boolean=()=>true):Promise<void>{
 const parsed=parseLocalSession(session),connection=api.getRouteConnection();if(!connection||connection.getSnapshot().phase!=='ready'||connection.pin.serverId!==parsed.viewer.serverId)throw new RouteError('identity_mismatch','Verify this server before saving its sign-in.');
 // Local pairing alone cannot supply the offline raw-key credential pin. The
 // issuing server must return it with the native session, in its transaction.
 if(!hosted&&!parsed.serverIdentity)throw new RouteError('missing_server_key','This server must include its identity key in the sign-in response.');
 if(parsed.serverIdentity){const actual=await serverPin(parsed.viewer.serverId,parsed.serverIdentity.publicKey,parsed.serverIdentity.fingerprint,env.crypto);if(!sameServerPin(actual,connection.pin))throw new RouteError('identity_mismatch','The native session returned a different server identity.');}
 // A Portico Account member (Spec — Hosted at Scale) holds a server-local session whose account is the
 // server's own; the Portico Account only discovers routes for it. A legacy hosted viewer must match.
 if(hosted&&parsed.viewer.authority==='hosted'&&parsed.viewer.accountId!==hosted.accountId)throw new RouteError('identity_mismatch','The native session belongs to another account.');
 const record:RememberedServer={version:'1',selectionId:routeBase64((env.crypto??webRouteCrypto).random(24)),logicalOrigin:connection.logicalOrigin,session:parsed,pin:{serverId:connection.pin.serverId,publicKey:connection.pin.publicKey,fingerprint:connection.pin.fingerprint},routes:memoryRecord(connection.getMemory()),...(hosted?{hosted}: {})};
 let previous:RememberedServer|undefined;
 await env.storage.change(current=>{if(!isCurrent())throw new RouteError('cancelled','The selected account or server changed.');previous=current;return record;});
 if(!isCurrent()){await env.storage.change(current=>sameSelection(current,record)?previous:current);throw new RouteError('cancelled','The selected account or server changed.');}
 // The storage acknowledgement itself may be delayed behind a newer selection.
 const saved=await env.storage.read();
 if(!isCurrent()||!sameSelection(saved,record)){if(!isCurrent())await env.storage.change(current=>sameSelection(current,record)?previous:current);throw new RouteError('cancelled','The selected account or server changed.');}
 attachPersistence(connection,env,record,hosted);bindSessionRoute(parsed.accessToken,connection);api.setAccessToken(parsed.accessToken);
}
/** Restore outcome: the server knows this sign-in but refuses it access (403, or 401 right after a
 * renewal). Not offline and not a reset: say so once, keep the credential, don't retry. */
export const ACCESS_REFUSED='access_refused';
export async function restoreNativeSession(env:ConnectionEnvironment,source?:(record:RememberedServer)=>HostedConnectionSource|undefined,expected?:LocalSession):Promise<{api:HttpLocalApi;session:LocalSession}|undefined>{
 const record=parseRememberedServer(await env.storage.read());if(!record)return;
 // The protected store may hold a newer rotation of the same family than the caller's copy.
 if(expected&&(record.session.sessionFamilyId!==expected.sessionFamilyId||BigInt(record.session.tokenGeneration)<BigInt(expected.tokenGeneration)||(record.session.tokenGeneration===expected.tokenGeneration&&record.session.accessToken!==expected.accessToken)||record.session.viewer.serverId!==expected.viewer.serverId||record.session.viewer.accountId!==expected.viewer.accountId||record.session.viewer.profileId!==expected.viewer.profileId))return;
 const pin=await serverPin(record.pin.serverId,record.pin.publicKey,record.pin.fingerprint,env.crypto??webRouteCrypto);
 if(record.session.serverIdentity&&!sameServerPin(pin,await serverPin(pin.serverId,record.session.serverIdentity.publicKey,record.session.serverIdentity.fingerprint,env.crypto)))throw new RouteError('identity_mismatch','Saved credentials and server identity disagree.');
 const memory:RouteMemory={paired:record.routes.paired,lastGood:record.routes.lastGood};let current:VerifiedRoutes|undefined,previous:VerifiedRoutes|undefined;
 if(record.hosted){for(const field of ['current','previous'] as const){const signed=record.routes[field];if(signed){const verified=await verifyRoutes(signed,record.hosted.key,{accountId:record.hosted.accountId,serverId:pin.serverId,pin,allowExpiredHint:true},env.crypto);if(field==='current')current=verified;else previous=verified;}}}
 const freshSource=source?.(record),hosted=record.hosted;
 const connection=new RouteConnection({pin,logicalOrigin:record.logicalOrigin,memory:{...memory,current,previous},fetcher:env.fetcher,crypto:env.crypto,discover:env.discover,fresh:freshSource&&hosted?async signal=>{const next=await currentDocument(freshSource,env,pin,hosted.key,signal);hosted.key=next.key;return next.routes;}:undefined});
 try{
  await connection.resolve();
  const selected=await env.storage.read();
  if(!sameSelection(selected,record)||selected!.session.accessToken!==record.session.accessToken)throw new RouteError('cancelled','The saved selection changed during reconnect.');
  const api=new HttpLocalApi(record.logicalOrigin,'',env.fetcher);api.useRoutes(connection);api.setAccessToken(record.session.accessToken);let renewedNow=false;
  // A sign-in saved more than a few minutes ago has an expired access token: renew it first.
  if(record.session.refreshToken&&(record.pendingRefresh||Date.parse(record.session.expiresAt)-Date.now()<REFRESH_MIN_VALID_MS)){
   bindSessionRoute(record.session.accessToken,connection);
   const renewed=await refreshStoredSession({env,api,family:{sessionFamilyId:record.session.sessionFamilyId,serverId:record.pin.serverId}});
   record.session=renewed;record.pendingRefresh=undefined;api.setAccessToken(renewed.accessToken);renewedNow=true;
  }
  // Fresh private-key possession is not membership. The server must accept the
  // same native family under its current local or signed cached policy.
  // An access token the server no longer accepts (expired early, or the server restarted) is not
  // a lost sign-in: renew it with the refresh token once, then ask again.
  // Only a 401 is a credential problem. A 403, or a 401 for a token that was just renewed, means
  // the server knows this sign-in and refuses it access: `access_refused`, reported once and never
  // retried (each retry would rotate a refresh token for nothing). The credential stays.
  const refusedAccess=()=>new RouteError(ACCESS_REFUSED,'This profile no longer has access to this server.');
  const statusOf=(e:unknown)=>(e as {status?:unknown}|null)?.status;
  let me:Awaited<ReturnType<HttpLocalApi['me']>>;
  try{me=await api.me();}
  catch(e){
   const status=statusOf(e);
   if(status===403||(status===401&&renewedNow))throw refusedAccess();
   if(!record.session.refreshToken||status!==401)throw e;
   bindSessionRoute(record.session.accessToken,connection);
   const renewed=await refreshStoredSession({env,api,family:{sessionFamilyId:record.session.sessionFamilyId,serverId:record.pin.serverId},rejected:record.session.accessToken});
   record.session=renewed;record.pendingRefresh=undefined;api.setAccessToken(renewed.accessToken);
   try{me=await api.me();}
   catch(again){const s=statusOf(again);if(s===401||s===403)throw refusedAccess();throw again;}
  }
  if(JSON.stringify(Object.entries(me.viewer).sort())!==JSON.stringify(Object.entries(record.session.viewer).sort()))throw new RouteError('identity_mismatch','The saved viewer no longer matches this server session.');
  const selectedAfter=await env.storage.read();
  if(!sameSelection(selectedAfter,record)||selectedAfter!.session.accessToken!==record.session.accessToken)throw new RouteError('cancelled','The saved selection changed during reconnect.');
  attachPersistence(connection,env,record,hosted);bindSessionRoute(record.session.accessToken,connection);return {api,session:record.session};
 }catch(e){connection.dispose();throw e;}
}
export async function forgetNativeSession(env:ConnectionEnvironment,session:LocalSession):Promise<void>{
 // The family is the sign-in: a rotated access token in the store is still this sign-in.
 let stored:string|undefined;
 await env.storage.change(current=>{if(current?.session.sessionFamilyId===session.sessionFamilyId&&current.session.viewer.serverId===session.viewer.serverId){stored=current.session.accessToken;return undefined;}return current;});
 forgetSessionRoute(session.accessToken);if(stored)forgetSessionRoute(stored);
}
/** Existing authorization renewal owns token rotation. This only updates the
 * atomic saved native envelope without replacing route or playback authority. */
export async function rememberRotatedSession(env:ConnectionEnvironment,session:LocalSession):Promise<void>{
 const parsed=parseLocalSession(session);let saved:RememberedServer|undefined,connection:RouteConnection|undefined;
 await env.storage.change(current=>{
  if(!current||current.session.sessionFamilyId!==parsed.sessionFamilyId||current.session.viewer.serverId!==parsed.viewer.serverId)return current;
  for(const field of ['authority','accountId','profileId'] as const)if(current.session.viewer[field]!==parsed.viewer[field])throw new RouteError('identity_mismatch','Renewal changed the selected viewer.');
  if(parsed.serverIdentity&&(parsed.serverIdentity.publicKey!==current.pin.publicKey||parsed.serverIdentity.fingerprint!==current.pin.fingerprint))throw new RouteError('identity_mismatch','Renewal changed the pinned server.');
  const generation=BigInt(parsed.tokenGeneration),old=BigInt(current.session.tokenGeneration);
  if(generation<old)return current;
  if(generation===old&&parsed.accessToken!==current.session.accessToken)throw new RouteError('identity_mismatch','Conflicting credentials for the same generation.');
  connection=current.session.accessToken?sessionRouteForRotation(current.session.accessToken):undefined;
  saved={...current,session:parsed};return saved;
 });
 if(saved&&connection){const current=await env.storage.read();if(sameSelection(current,saved)&&current!.session.accessToken===parsed.accessToken)bindSessionRoute(parsed.accessToken,connection);}
}

const pendingConnections=new Map<string,{api:HttpLocalApi;hosted?:RememberedServer['hosted']}>();
export function prepareRoutedSession(api:HttpLocalApi,session:LocalSession,hosted?:RememberedServer['hosted']) {
 const connection=api.getRouteConnection();if(!connection||connection.pin.serverId!==session.viewer.serverId)throw new RouteError('identity_mismatch','This session does not match the verified server.');
 api.setAccessToken(session.accessToken);bindSessionRoute(session.accessToken,connection);pendingConnections.set(session.accessToken,{api,hosted});
 while(pendingConnections.size>8){const key=pendingConnections.keys().next().value!;pendingConnections.get(key)?.api.getRouteConnection()?.dispose();pendingConnections.delete(key);forgetSessionRoute(key);}
}
export async function commitRoutedSession(session:LocalSession,env:ConnectionEnvironment,isCurrent:()=>boolean=()=>true):Promise<HttpLocalApi>{
 const prepared=pendingConnections.get(session.accessToken);if(!prepared)throw new RouteError('missing_identity','The verified connection is no longer available. Select the server again.');
 try{await rememberNativeSession(prepared.api,session,env,prepared.hosted,isCurrent);pendingConnections.delete(session.accessToken);return prepared.api;}
 catch(error){if(!isCurrent()){pendingConnections.delete(session.accessToken);forgetSessionRoute(session.accessToken);}throw error;}
}

/** Trusted profile exchange is an explicit selection. Prove the cached server
 * before sending the offline proof; an old UI URL is not an identity check. */
export async function connectTrustedProfileServer(env:ConnectionEnvironment,scope:{accountId:string;serverId:string},signal:AbortSignal):Promise<HttpLocalApi>{
 const record=parseRememberedServer(await env.storage.read());
 if(!record||record.session.viewer.authority!=='hosted'||record.session.viewer.accountId!==scope.accountId||record.pin.serverId!==scope.serverId)throw new RouteError('identity_mismatch','Open the saved connection for this account and server first.');
 const pin=await serverPin(record.pin.serverId,record.pin.publicKey,record.pin.fingerprint,env.crypto);
 const memory:RouteMemory={paired:record.routes.paired,lastGood:record.routes.lastGood};
 let current:VerifiedRoutes|undefined,previous:VerifiedRoutes|undefined;
 if(record.hosted)for(const field of ['current','previous']as const){const signed=record.routes[field];if(signed){const v=await verifyRoutes(signed,record.hosted.key,{accountId:scope.accountId,serverId:scope.serverId,pin,allowExpiredHint:true},env.crypto);if(field==='current')current=v;else previous=v;}}
 const connection=new RouteConnection({pin,logicalOrigin:record.logicalOrigin,memory:{...memory,current,previous},fetcher:env.fetcher,crypto:env.crypto,discover:env.discover});
 try{await connection.resolve(signal);const selected=await env.storage.read();if(!sameSelection(selected,record)||signal.aborted)throw new RouteError('cancelled','The saved server selection changed.');const api=new HttpLocalApi(record.logicalOrigin,'',env.fetcher);api.useRoutes(connection);return api;}catch(e){connection.dispose();throw e;}
}

/** Renew when less than this much access time remains. */
export const REFRESH_MIN_VALID_MS = 60_000;

/**
 * A refresh that could not complete. `signOut` means the server refused the credential (reused,
 * revoked, expired, device no longer approved): the sign-in is over and the stored record has
 * been removed. Otherwise the credential is kept and the refresh can be retried (same request id).
 */
export class SessionRefreshError extends ApiError {
 readonly signOut:boolean;
 constructor(code:string,message:string,signOut:boolean,status=signOut?401:0){super(status,code,message,!signOut);this.signOut=signOut;}
}

export type RefreshStoredOptions=Readonly<{
 env:ConnectionEnvironment;
 /** The selected server's transport (routed); only its `request` is used. */
 api:Pick<HttpLocalApi,'request'>;
 family:Readonly<{sessionFamilyId:string;serverId:string}>;
 /** The access token a server just refused: refresh even if the stored one still looks fresh. */
 rejected?:string;
 now?:()=>number;
 minValidMs?:number;
 timeoutMs?:number;
 signal?:AbortSignal;
}>;

/**
 * One refresh against the protected connection store (lane C `POST /v1/auth/refresh`):
 * under the credential lock, re-read the store (another tab or a restore may already have
 * rotated), persist the request id before sending, send, then store the next access and refresh
 * credential in one atomic change. A crash after sending replays the same request id and the
 * server returns its stored answer (five minutes). Refusals end the sign-in.
 */
export function refreshStoredSession(o:RefreshStoredOptions):Promise<LocalSession>{
 const now=o.now??Date.now,minValid=o.minValidMs??REFRESH_MIN_VALID_MS;
 const same=(r:RememberedServer|undefined)=>!!r&&r.session.sessionFamilyId===o.family.sessionFamilyId&&r.pin.serverId===o.family.serverId;
 return credentialLock()('portico.session-refresh:'+o.family.serverId,async()=>{
  let record:RememberedServer|undefined;
  try{record=parseRememberedServer(await o.env.storage.read());}catch{record=undefined;}
  if(!record||!same(record))throw new SessionRefreshError('session_replaced','This sign-in was replaced or ended on this device.',true);
  const session=record.session;
  const fresh=Date.parse(session.expiresAt)-now()>minValid;
  if(fresh&&!record.pendingRefresh&&(o.rejected===undefined||session.accessToken!==o.rejected))return session;
  const identity=installationIdentity();
  if(!session.refreshToken||!identity)throw new SessionRefreshError('refresh_unavailable','This sign-in can’t be renewed. Sign in again when it expires.',false);
  let requestId=record.pendingRefresh;
  if(!requestId){
   const id=routeBase64((o.env.crypto??webRouteCrypto).random(24));requestId=id;
   await o.env.storage.change(current=>same(current)&&current!.session.refreshToken===session.refreshToken?{...current!,pendingRefresh:id}:current);
  }
  // React Native lacks AbortSignal.any/timeout: a plain controller bounds the request.
  const bound=new AbortController(),abort=()=>bound.abort(),timer=setTimeout(abort,o.timeoutMs??15_000);
  o.signal?.addEventListener('abort',abort,{once:true});if(o.signal?.aborted)abort();
  let raw:unknown;
  try{raw=await o.api.request<unknown>('/v1/auth/refresh','POST',{refreshToken:session.refreshToken,installationId:identity.installationId,requestId},bound.signal);}
  catch(e){
   const status=(e as {status?:unknown}|null)?.status;
   // Only a refusal (401/403: revoked, unknown family, or a reused refresh token) ends the sign-in.
   // A malformed request (400) is this client's problem and keeps the credential for a retry.
   if(status===401||status===403){
    await o.env.storage.change(current=>same(current)&&current!.session.refreshToken===session.refreshToken?undefined:current).catch(()=>{});
    forgetSessionRoute(session.accessToken);
    // The server's move to its own membership (migration 0120) ended this family: not a sign-out.
    // The code survives so the app can re-admit silently (`isSessionMigrated`).
    if(status===401&&(e as {code?:unknown}|null)?.code==='session_migrated')throw new SessionRefreshError('session_migrated','This server moved your sign-in to its own membership.',true);
    throw new SessionRefreshError('refresh_refused','Your sign-in has ended. Sign in again.',true);
   }
   if(status===404||status===405)throw new SessionRefreshError('refresh_unsupported','This server doesn’t renew sign-ins.',false,status as number);
   // Transport failures keep their own shape so restore policies still see "offline, retry".
   if(typeof status!=='number')throw e;
   throw new SessionRefreshError('refresh_unavailable_now','The server couldn’t renew your sign-in right now.',false,status);
  }finally{clearTimeout(timer);o.signal?.removeEventListener('abort',abort);}
  let next:LocalSession;
  try{next=parseLocalSession(raw,{authority:session.viewer.authority,accountId:session.viewer.accountId,profileId:session.viewer.profileId,serverId:session.viewer.serverId,role:session.viewer.role});}
  catch{throw new SessionRefreshError('refresh_invalid','The renewed sign-in didn’t match this one.',false);}
  if(next.sessionFamilyId!==session.sessionFamilyId||BigInt(next.tokenGeneration)<=BigInt(session.tokenGeneration)||(next.serverIdentity&&(next.serverIdentity.publicKey!==record.pin.publicKey||next.serverIdentity.fingerprint!==record.pin.fingerprint)))
   throw new SessionRefreshError('refresh_invalid','The renewed sign-in didn’t match this one.',false);
  const connection=sessionRoute(session.accessToken);
  try{
   await o.env.storage.change(current=>{if(!same(current)||current!.session.refreshToken!==session.refreshToken)return current;const {pendingRefresh:_done,...rest}=current!;return {...rest,session:next};});
  }catch{/* The request id stays stored: the next attempt replays and stores this same answer. */}
  if(connection)bindSessionRoute(next.accessToken,connection);
  return next;
 });
}
