import {wireDurations} from './wire-units.ts';
/**
 * API keys, personal access tokens, outbound webhooks, and the two routes that
 * exist for tools outside Portico (the bounded title query and the "these paths
 * changed" report).
 *
 * This module is transport and parsing only; nothing here decides policy.
 * What grants access is read whole: a list of keys, tokens or webhooks with a
 * row this app cannot read is refused, so a credential or a destination is
 * never missing from what the owner sees and no scope or state is guessed.
 * Receipts are exact too. The delivery log, the title query and the scopes on
 * offer leave out only the row or the value this app cannot read. Secrets pass through exactly once, on the answer to a
 * create or a rotation, and are never kept by this module.
 */
import {firstOfEach,keepReadable} from './tolerant.ts';
export type ApiAccessTransport={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};

const unreadable='The server sent something this app could not read.';
export class ApiAccessError extends Error{
 readonly code:string;
 constructor(code:string,message=unreadable){super(message);this.name='ApiAccessError';this.code=code;}
}
const invalid=(what:string):never=>{throw new ApiAccessError('invalid_document',`${unreadable} (${what})`);};
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max:number,what:string):string=>typeof v==='string'&&v.length<=max?v:invalid(what);
const filled=(v:unknown,max:number,what:string):string=>{const s=text(v,max,what);return s.length?s:invalid(what);};
const optional=(v:unknown,max:number,what:string):string|undefined=>v===undefined||v===''?undefined:text(v,max,what);
const whole=(v:unknown,what:string):number=>typeof v==='number'&&Number.isInteger(v)&&v>=0?v:invalid(what);
const flag=(v:unknown,what:string):boolean=>typeof v==='boolean'?v:invalid(what);
const list=<T>(v:unknown,max:number,each:(raw:unknown)=>T,what:string):readonly T[]=>Array.isArray(v)&&v.length<=max?Object.freeze(v.map(each)):invalid(what);
const oneOf=<T extends string>(v:unknown,allowed:readonly T[],what:string):T=>typeof v==='string'&&(allowed as readonly string[]).includes(v)?v as T:invalid(what);
const instant=(v:unknown,what:string):string=>{const s=filled(v,40,what);return Number.isNaN(Date.parse(s))?invalid(what):s;};
const optionalInstant=(v:unknown,what:string):string|undefined=>v===undefined||v===''?undefined:instant(v,what);
const query=(values:Readonly<Record<string,string|number|undefined>>):string=>{
 const parts=Object.entries(values).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`);
 return parts.length?`?${parts.join('&')}`:'';
};

// ── Scopes ──────────────────────────────────────────────────────────────────

/** What a key or token may be given. One scope does not include another, except that
 * `admin` is everything. A scope never raises what the member behind the credential may do. */
export const apiScopes=['read','playback','library','admin'] as const;
export type ApiScope=typeof apiScopes[number];

/** Ready-made choices for a simple picker: each is the scopes a typical use needs. */
export const apiScopePresets=Object.freeze({
 read:Object.freeze(['read']) as readonly ApiScope[],
 playback:Object.freeze(['read','playback']) as readonly ApiScope[],
 library:Object.freeze(['read','playback','library']) as readonly ApiScope[],
 admin:Object.freeze(['admin']) as readonly ApiScope[],
});
export type ApiScopePreset=keyof typeof apiScopePresets;
export const apiScopePresetIds=['read','playback','library','admin'] as const satisfies readonly ApiScopePreset[];

/** presetOf names the preset whose scopes are exactly these, if one is. */
export function presetOf(scopes:readonly ApiScope[]):ApiScopePreset|undefined{
 const key=[...scopes].sort().join(',');
 return apiScopePresetIds.find(id=>[...apiScopePresets[id]].sort().join(',')===key);
}

// ── Keys and tokens ─────────────────────────────────────────────────────────

/** Every API key secret starts with `pk.` and every personal access token with `pat.`. */
export const apiKeyPrefix='pk.';
export const accessTokenPrefix='pat.';
export const isApiCredentialSecret=(secret:string):boolean=>secret.startsWith(apiKeyPrefix)||secret.startsWith(accessTokenPrefix);

export const apiCredentialStates=['active','awaiting_approval','expired','revoked','ended'] as const;
export type ApiCredentialState=typeof apiCredentialStates[number];
export type ApiCredentialKind='key'|'token';
export type ApiCredentialMember=Readonly<{accountId:string;username:string;profileId:string;profile:string}>;
export type ApiCredential=Readonly<{id:string;kind:ApiCredentialKind;name:string;scopes:readonly ApiScope[];hint:string;createdAt:string;
 expiresAt?:string;lastUsedAt?:string;state:ApiCredentialState;endedReason?:string;member:ApiCredentialMember}>;
/** The answer to a create. `secret` is present only the first time the request is answered. */
export type IssuedApiCredential=Readonly<{credential:ApiCredential;secret?:string}>;
export type ApiCredentialPage=Readonly<{items:readonly ApiCredential[];limit:number;nextCursor?:string}>;
export type ApiAccessOptions=Readonly<{scopes:readonly ApiScope[];maximumKeys:number;maximumTokens:number;tokensAllowed:boolean;requestsPerMinute:number}>;
export type NewApiCredential=Readonly<{operationId:string;name:string;scopes:readonly ApiScope[];expiresAt?:string}>;

export function parseApiCredential(raw:unknown):ApiCredential{
 if(!object(raw)||!object(raw.member))return invalid('credential');
 const scopes=list(raw.scopes,apiScopes.length,v=>oneOf(v,apiScopes,'credential scope'),'credential scopes');
 if(!scopes.length)invalid('credential scopes');
 // A listing must never carry a secret; a document that does is not one to trust.
 if('secret' in raw)invalid('credential carries a secret');
 return Object.freeze({id:filled(raw.id,128,'credential id'),kind:oneOf(raw.kind,['key','token'] as const,'credential kind'),name:filled(raw.name,160,'credential name'),
  scopes,hint:text(raw.hint,16,'credential hint'),createdAt:instant(raw.createdAt,'credential createdAt'),expiresAt:optionalInstant(raw.expiresAt,'credential expiresAt'),
  lastUsedAt:optionalInstant(raw.lastUsedAt,'credential lastUsedAt'),state:oneOf(raw.state,apiCredentialStates,'credential state'),endedReason:optional(raw.endedReason,64,'credential endedReason'),
  member:Object.freeze({accountId:filled(raw.member.accountId,128,'member account'),username:text(raw.member.username,128,'member username'),profileId:filled(raw.member.profileId,128,'member profile id'),profile:text(raw.member.profile,128,'member profile')})});
}
export function parseIssuedApiCredential(raw:unknown):IssuedApiCredential{
 if(!object(raw))return invalid('issued credential');
 const credential=parseApiCredential(raw.credential);
 const secret=optional(raw.secret,512,'credential secret');
 const prefix=credential.kind==='key'?apiKeyPrefix:accessTokenPrefix;
 if(secret!==undefined&&!secret.startsWith(prefix))invalid('credential secret');
 return Object.freeze({credential,secret});
}
export function parseApiCredentialPage(raw:unknown):ApiCredentialPage{
 if(!object(raw))return invalid('credential page');
 return Object.freeze({items:list(raw.items,100,parseApiCredential,'credential page items'),limit:whole(raw.limit,'credential page limit'),nextCursor:optional(raw.nextCursor,512,'credential page cursor')});
}
export function parseApiAccessOptions(raw:unknown):ApiAccessOptions{
 if(!object(raw))return invalid('api access options');
 // A scope this app does not know is not offered; the ones it knows still are.
 const scopes=Object.freeze(firstOfEach('api-access','options.scopes',keepReadable('api-access','options.scopes',list(raw.scopes,16,v=>v,'scopes'),v=>object(v)?oneOf(v.id,apiScopes,'scope id'):invalid('scope')),scope=>scope));
 return Object.freeze({scopes,maximumKeys:whole(raw.maximumKeys,'maximumKeys'),maximumTokens:whole(raw.maximumTokens,'maximumTokens'),tokensAllowed:flag(raw.tokensAllowed,'tokensAllowed'),requestsPerMinute:whole(raw.requestsPerMinute,'requestsPerMinute')});
}

const API_KEYS='/v1/admin/api-keys',MEMBER_TOKENS='/v1/admin/access-tokens',MY_TOKENS='/v1/me/access-tokens',OPTIONS='/v1/me/api-access';
const id=(value:string):string=>encodeURIComponent(value);
const body=(q:NewApiCredential)=>({operationId:q.operationId,name:q.name,scopes:[...q.scopes],...(q.expiresAt?{expiresAt:q.expiresAt}:{})});

export const getApiAccessOptions=async(t:ApiAccessTransport,signal?:AbortSignal):Promise<ApiAccessOptions>=>parseApiAccessOptions(await t.request<unknown>(OPTIONS,'GET',undefined,signal));
/** The owner's API keys. */
export const listApiKeys=async(t:ApiAccessTransport,cursor='',signal?:AbortSignal):Promise<ApiCredentialPage>=>parseApiCredentialPage(await t.request<unknown>(API_KEYS+query({limit:100,cursor}),'GET',undefined,signal));
/** `operationId` is one per thing the person asked for and is reused on a retry; a retry's answer has no secret. */
export const createApiKey=async(t:ApiAccessTransport,q:NewApiCredential,signal?:AbortSignal):Promise<IssuedApiCredential>=>parseIssuedApiCredential(await t.request<unknown>(API_KEYS,'POST',body(q),signal));
export const revokeApiKey=async(t:ApiAccessTransport,keyId:string,signal?:AbortSignal):Promise<ApiCredential>=>parseApiCredential(await t.request<unknown>(`${API_KEYS}/${id(keyId)}`,'DELETE',undefined,signal));
/** A member's own personal access tokens. */
export const listAccessTokens=async(t:ApiAccessTransport,cursor='',signal?:AbortSignal):Promise<ApiCredentialPage>=>parseApiCredentialPage(await t.request<unknown>(MY_TOKENS+query({limit:100,cursor}),'GET',undefined,signal));
export const createAccessToken=async(t:ApiAccessTransport,q:NewApiCredential,signal?:AbortSignal):Promise<IssuedApiCredential>=>parseIssuedApiCredential(await t.request<unknown>(MY_TOKENS,'POST',body(q),signal));
export const revokeAccessToken=async(t:ApiAccessTransport,tokenId:string,signal?:AbortSignal):Promise<ApiCredential>=>parseApiCredential(await t.request<unknown>(`${MY_TOKENS}/${id(tokenId)}`,'DELETE',undefined,signal));
/** Every member's tokens, for the owner. */
export const listMemberAccessTokens=async(t:ApiAccessTransport,options:Readonly<{cursor?:string;accountId?:string}>={},signal?:AbortSignal):Promise<ApiCredentialPage>=>parseApiCredentialPage(await t.request<unknown>(MEMBER_TOKENS+query({limit:100,cursor:options.cursor,accountId:options.accountId}),'GET',undefined,signal));
export const revokeMemberAccessToken=async(t:ApiAccessTransport,tokenId:string,signal?:AbortSignal):Promise<ApiCredential>=>parseApiCredential(await t.request<unknown>(`${MEMBER_TOKENS}/${id(tokenId)}`,'DELETE',undefined,signal));

// ── Webhooks ────────────────────────────────────────────────────────────────

export const webhookEvents=['title.added','title.updated','title.removed','scan.started','scan.finished','playback.started','playback.paused','playback.resumed','playback.stopped',
 'recording.finished','recording.failed','member.signed_in','backup.succeeded','backup.failed','storage.low','update.available','server.started'] as const;
export type WebhookEvent=typeof webhookEvents[number];
export type Webhook=Readonly<{id:string;name:string;url:string;events:readonly string[];enabled:boolean;secretHint:string;createdAt:string;updatedAt:string;waiting:number;failing:number;dropped:number}>;
/** The answer to adding a webhook or rotating its secret: the one time the secret is shown. */
export type IssuedWebhook=Readonly<{webhook:Webhook;secret?:string}>;
export type WebhookList=Readonly<{items:readonly Webhook[];events:readonly string[];maximum:number}>;
export type WebhookDeliveryState='pending'|'delivered'|'failed';
export type WebhookDelivery=Readonly<{id:string;event:string;state:WebhookDeliveryState;attempts:number;createdAt:string;finishedAt?:string;nextAttemptAt?:string;status:number;durationMs:number;error?:string}>;
export type WebhookDeliveryPage=Readonly<{items:readonly WebhookDelivery[];limit:number;nextCursor?:string}>;
export type NewWebhook=Readonly<{name:string;url:string;events:readonly string[];secret?:string;enabled?:boolean}>;
export type WebhookChange=Readonly<{name?:string;url?:string;events?:readonly string[];enabled?:boolean;rotateSecret?:boolean;secret?:string}>;

export function parseWebhook(raw:unknown):Webhook{
 if(!object(raw))return invalid('webhook');
 if('secret' in raw)invalid('webhook carries a secret');
 // Event names are kept as text: a newer server may send one this build does not list.
 return Object.freeze({id:filled(raw.id,128,'webhook id'),name:filled(raw.name,160,'webhook name'),url:filled(raw.url,2048,'webhook url'),events:list(raw.events,64,v=>filled(v,64,'webhook event'),'webhook events'),
  enabled:flag(raw.enabled,'webhook enabled'),secretHint:text(raw.secretHint,16,'webhook secret hint'),createdAt:instant(raw.createdAt,'webhook createdAt'),updatedAt:instant(raw.updatedAt,'webhook updatedAt'),
  waiting:whole(raw.waiting,'webhook waiting'),failing:whole(raw.failing,'webhook failing'),dropped:whole(raw.dropped,'webhook dropped')});
}
export function parseIssuedWebhook(raw:unknown):IssuedWebhook{
 if(!object(raw))return invalid('issued webhook');
 return Object.freeze({webhook:parseWebhook(raw.webhook),secret:optional(raw.secret,256,'webhook secret')});
}
export function parseWebhookList(raw:unknown):WebhookList{
 if(!object(raw))return invalid('webhook list');
 return Object.freeze({items:list(raw.items,100,parseWebhook,'webhook list items'),events:list(raw.events,64,v=>filled(v,64,'webhook event'),'webhook list events'),maximum:whole(raw.maximum,'webhook maximum')});
}
export function parseWebhookDelivery(raw:unknown):WebhookDelivery{
 if(!object(raw))return invalid('webhook delivery');
 return Object.freeze({id:filled(raw.id,128,'delivery id'),event:filled(raw.event,64,'delivery event'),state:oneOf(raw.state,['pending','delivered','failed'] as const,'delivery state'),attempts:whole(raw.attempts,'delivery attempts'),
  createdAt:instant(raw.createdAt,'delivery createdAt'),finishedAt:optionalInstant(raw.finishedAt,'delivery finishedAt'),nextAttemptAt:optionalInstant(raw.nextAttemptAt,'delivery nextAttemptAt'),
  status:whole(raw.status,'delivery status'),durationMs:whole(raw.durationMs,'delivery duration'),error:optional(raw.error,128,'delivery error')});
}
export function parseWebhookDeliveryPage(raw:unknown):WebhookDeliveryPage{
 if(!object(raw))return invalid('webhook delivery page');
 return Object.freeze({items:Object.freeze(firstOfEach('api-access','deliveries',keepReadable('api-access','deliveries',list(raw.items,100,v=>v,'delivery page items'),parseWebhookDelivery),row=>row.id)),limit:whole(raw.limit,'delivery page limit'),nextCursor:optional(raw.nextCursor,512,'delivery page cursor')});
}

const WEBHOOKS='/v1/admin/webhooks';
export const listWebhooks=async(t:ApiAccessTransport,signal?:AbortSignal):Promise<WebhookList>=>parseWebhookList(await t.request<unknown>(WEBHOOKS,'GET',undefined,signal));
export const createWebhook=async(t:ApiAccessTransport,q:NewWebhook,signal?:AbortSignal):Promise<IssuedWebhook>=>parseIssuedWebhook(await t.request<unknown>(WEBHOOKS,'POST',{...q,events:[...q.events]},signal));
/** A field left out of `change` is left as it is. */
export const updateWebhook=async(t:ApiAccessTransport,webhookId:string,change:WebhookChange,signal?:AbortSignal):Promise<IssuedWebhook>=>parseIssuedWebhook(await t.request<unknown>(`${WEBHOOKS}/${id(webhookId)}`,'PATCH',{...change,...(change.events?{events:[...change.events]}:{})},signal));
export const deleteWebhook=async(t:ApiAccessTransport,webhookId:string,signal?:AbortSignal):Promise<void>=>{await t.request<unknown>(`${WEBHOOKS}/${id(webhookId)}`,'DELETE',undefined,signal);};
/** Queues a test event; its result appears in the delivery log a moment later. */
export const sendWebhookTest=async(t:ApiAccessTransport,webhookId:string,signal?:AbortSignal):Promise<WebhookDelivery>=>parseWebhookDelivery(await t.request<unknown>(`${WEBHOOKS}/${id(webhookId)}/test`,'POST',undefined,signal));
export const listWebhookDeliveries=async(t:ApiAccessTransport,webhookId:string,options:Readonly<{cursor?:string;limit?:number}>={},signal?:AbortSignal):Promise<WebhookDeliveryPage>=>parseWebhookDeliveryPage(await t.request<unknown>(`${WEBHOOKS}/${id(webhookId)}/deliveries`+query({limit:options.limit??50,cursor:options.cursor}),'GET',undefined,signal));

// ── For tools outside Portico ───────────────────────────────────────────────

export const titleKinds=['movie','show','season','episode','artist','album','song','book','author','collection'] as const;
export type TitleKind=typeof titleKinds[number];
export type TitleRow=Readonly<{id:string;kind:string;title:string;libraryId:string;year?:number;addedAt?:string;available:boolean;durationSeconds?:number;rating?:number;posterUrl?:string;providerIds:Readonly<Record<string,string>>}>;
export type TitlePage=Readonly<{items:readonly TitleRow[];limit:number;nextCursor?:string}>;
/** Either `library` and `kind` (a list), or `provider` and `providerId` (a lookup). A range is
 * accepted only on the key being sorted by: years with `sort:'year'`, added times with `sort:'added'`. */
export type TitleQuery=Readonly<{library?:string;kind?:TitleKind;sort?:'title'|'added'|'year';order?:'asc'|'desc';yearFrom?:number;yearTo?:number;addedAfter?:string;addedBefore?:string;
 provider?:'tmdb'|'tvdb'|'imdb'|'anilist';providerId?:string;cursor?:string;limit?:number}>;

export function parseTitleRow(raw:unknown):TitleRow{
 raw=wireDurations(raw,{durationSeconds:"durationMs"});
 if(!object(raw)||!object(raw.providerIds))return invalid('title');
 const providerIds:Record<string,string>={};
 for(const [provider,value] of Object.entries(raw.providerIds))providerIds[text(provider,32,'provider')]=filled(value,64,'provider id');
 const number=(v:unknown,what:string):number|undefined=>v===undefined?undefined:typeof v==='number'&&Number.isFinite(v)&&v>=0?v:invalid(what);
 return Object.freeze({id:filled(raw.id,128,'title id'),kind:filled(raw.kind,32,'title kind'),title:text(raw.title,1024,'title'),libraryId:filled(raw.libraryId,128,'title library'),
  year:number(raw.year,'title year'),addedAt:optional(raw.addedAt,40,'title addedAt'),available:flag(raw.available,'title available'),durationSeconds:number(raw.durationSeconds,'title duration'),
  rating:number(raw.rating,'title rating'),posterUrl:optional(raw.posterUrl,2048,'title poster'),providerIds:Object.freeze(providerIds)});
}
export function parseTitlePage(raw:unknown):TitlePage{
 if(!object(raw))return invalid('title page');
 return Object.freeze({items:Object.freeze(firstOfEach('api-access','titles',keepReadable('api-access','titles',list(raw.items,100,v=>v,'title page items'),parseTitleRow),row=>row.id)),limit:whole(raw.limit,'title page limit'),nextCursor:optional(raw.nextCursor,4096,'title page cursor')});
}
export const queryTitles=async(t:ApiAccessTransport,q:TitleQuery,signal?:AbortSignal):Promise<TitlePage>=>parseTitlePage(await t.request<unknown>('/v1/titles'+query({...q}),'GET',undefined,signal));

export type PathChangeKind='added'|'changed'|'removed';
export type ReportedPath=Readonly<{path:string;status:'queued'|'outside_libraries'|'invalid';libraryId?:string;sourceId?:string;folder?:string}>;
export type PathReport=Readonly<{paths:readonly ReportedPath[];jobIds:readonly string[]}>;
export function parsePathReport(raw:unknown):PathReport{
 if(!object(raw))return invalid('path report');
 const paths=list(raw.paths,400,v=>object(v)?Object.freeze({path:text(v.path,8192,'reported path'),status:oneOf(v.status,['queued','outside_libraries','invalid'] as const,'reported path status'),
  libraryId:optional(v.libraryId,128,'reported library'),sourceId:optional(v.sourceId,128,'reported source'),folder:optional(v.folder,8192,'reported folder')}):invalid('reported path'),'reported paths');
 const jobIds=list(raw.jobs,400,v=>object(v)?filled(v.id,128,'job id'):invalid('job'),'jobs');
 return Object.freeze({paths,jobIds});
}
/** Tells the server which absolute paths were added, changed or removed, so it scans only those folders. */
export const reportChangedPaths=async(t:ApiAccessTransport,paths:readonly Readonly<{path:string;change?:PathChangeKind}>[],signal?:AbortSignal):Promise<PathReport>=>
 parsePathReport(await t.request<unknown>('/v1/libraries/paths-changed','POST',{paths:paths.map(p=>p.change?{path:p.path,change:p.change}:{path:p.path})},signal));
