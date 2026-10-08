import {CodedError,unreadableServerResponse} from './server-messages.ts';
import {parseLocalSession,currentUTCTimestamp} from './local-session.ts';
import type {LocalSession} from './index.ts';

/** Account credentials administer profiles; a selected profile is only a viewer. */
export interface ProfileAPI {request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>}
export type ProfileAuthority='local'|'hosted';
export const PROFILE_ART=['blue','violet','mint','coral','gold','slate','rose','sky'] as const;
export type ProfileArt=typeof PROFILE_ART[number];
export type ManagedProfile=Readonly<{id:string;name:string;primary:boolean;position:number;art:ProfileArt;revision:number;pinRevision:number;pinRequired:boolean;trustRevision?:number;allowedLibraries?:readonly string[]|null}>;
/** `hostedAccountId`: a Portico Account member (Spec — Hosted at Scale). Its password, email and
 * two-step verification are managed at the Portico Account; profiles, PINs, sessions and devices
 * here, exactly as for a direct account. */
export type DirectAccount=Readonly<{id:string;username:string;primaryProfileId:string;role:'owner'|'admin'|'member';revision:number;disabled:boolean;allowedLibraries:readonly string[];hostedAccountId?:string}>;
/** `custodyPending`: the owner is a Portico Account, but Hosted's custody of the server's claim
 * (retiring it, its certificates) is not with it yet: after a transfer to it, or after a direct
 * owner. Ask the owner to accept it (`acceptPorticoCustody`). */
export type DirectSnapshot=Readonly<{authority:'local';serverId:string;account:DirectAccount;profiles:readonly ManagedProfile[];canManage:boolean;custodyPending?:boolean}>;
/**
 * C45 direct sign-in (Justin's account decisions). There is no separate account token any more:
 * - `signed-in`: one device-bound `session`. With several profiles or a PIN it has role `account`
 *   (`accountScoped`) and a profile must be chosen; with one unprotected profile it is the viewer.
 *   `passwordChangeRequired` means: `changeRequiredPassword` with this session, then sign in again.
 * - `challenge`: the account has two-step verification; finish with `completeDirectSignInChallenge`.
 * - `device-pending`: the owner must approve this device first; nothing was issued.
 */
export type DirectSignIn=
 Readonly<{kind:'signed-in';snapshot:DirectSnapshot;session:LocalSession;accountScoped:boolean;passwordChangeRequired:boolean}>
 |Readonly<{kind:'challenge';token:string;expiresAt:string}>
 |Readonly<{kind:'device-pending';deviceId:string;installationId:string}>;
export type ProfileScope=Readonly<{authority:ProfileAuthority;accountId:string;serverId?:string}>;
export type ProfileSelectionInput=Readonly<{pin?:string;installationId?:string;trust?:boolean;trustToken?:string}>;
export type TrustedProfile=Readonly<{token:string;authority:ProfileAuthority;accountId:string;profileId:string;serverId:string;installationId:string;pinRevision:number;profileRevision:number;policyRevision?:number;membershipRevision?:number;trustRevision?:number;expiresAt:string}>;
export type SignedProfileProof=Readonly<{payload:string;signature:string;keyId:string}>;
export type DirectSelection=Readonly<{session:LocalSession;trustedSelection?:TrustedProfile}>;
export type ProfileDeletion=Readonly<{id?:string;profileId:string;revision:number;name?:string;primary:boolean;status?:string;playlists?:number;savedResources?:number;servers?:readonly {id:string;name:string;acknowledged:boolean}[]}>;
export type ProfilePolicy=Readonly<{serverId:string;allowedLibraries:readonly string[]|null}>;
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=256):v is string=>typeof v==='string'&&v.length>0&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const natural=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const revision=(v:unknown):v is number=>natural(v)&&v>0;
function invalid():never{throw new CodedError('invalid_response',unreadableServerResponse);}
function ids(v:unknown):readonly string[]{if(!Array.isArray(v)||v.length>1000||v.some(x=>!text(x,128))||new Set(v).size!==v.length)invalid();return Object.freeze([...v] as string[]);}
export function parseManagedProfiles(raw:unknown):readonly ManagedProfile[]{
 if(!Array.isArray(raw)||raw.length<1||raw.length>8)invalid();
 const result=raw.map(v=>{if(!obj(v)||!text(v.id,128)||!text(v.name,120)||typeof v.primary!=='boolean'||!natural(v.position)||!PROFILE_ART.includes(v.art as ProfileArt)||!revision(v.revision)||!revision(v.pinRevision)||typeof v.pinRequired!=='boolean'||(v.trustRevision!==undefined&&!revision(v.trustRevision)))invalid();
  return Object.freeze({id:v.id,name:v.name,primary:v.primary,position:v.position,art:v.art as ProfileArt,revision:v.revision,pinRevision:v.pinRevision,pinRequired:v.pinRequired,...(v.trustRevision===undefined?{}:{trustRevision:v.trustRevision as number}),...(v.allowedLibraries===undefined?{}:{allowedLibraries:v.allowedLibraries===null?null:ids(v.allowedLibraries)})});});
 if(new Set(result.map(v=>v.id)).size!==result.length||result.filter(v=>v.primary).length!==1)invalid();
 return Object.freeze(result.sort((a,b)=>a.position-b.position||a.id.localeCompare(b.id)));
}
export function parseDirectSnapshot(raw:unknown,expected:Partial<ProfileScope>={}):DirectSnapshot{
 if(!obj(raw)||raw.authority!=='local'||!text(raw.serverId,128)||!obj(raw.account)||typeof raw.canManage!=='boolean'||(raw.custodyPending!==undefined&&typeof raw.custodyPending!=='boolean'))invalid();const a=raw.account;
 if(!text(a.id,128)||!text(a.username,64)||!text(a.primaryProfileId,128)||!['owner','admin','member'].includes(a.role as string)||(a.hostedAccountId!==undefined&&!text(a.hostedAccountId,128))||!revision(a.revision)||typeof a.disabled!=='boolean'||(expected.accountId&&a.id!==expected.accountId)||(expected.serverId&&raw.serverId!==expected.serverId)||(expected.authority&&expected.authority!=='local'))invalid();
 const profiles=parseManagedProfiles(raw.profiles);if(!profiles.some(p=>p.primary&&p.id===a.primaryProfileId))invalid();
 return Object.freeze({authority:'local',serverId:raw.serverId,account:Object.freeze({id:a.id,username:a.username,primaryProfileId:a.primaryProfileId,role:a.role as DirectAccount['role'],revision:a.revision,disabled:a.disabled,allowedLibraries:ids(a.allowedLibraries),...(a.hostedAccountId===undefined?{}:{hostedAccountId:a.hostedAccountId as string})}),profiles,canManage:raw.canManage,...(raw.custodyPending===true?{custodyPending:true}:{})});
}
export function parseDirectSignIn(raw:unknown):DirectSignIn{
 if(!obj(raw))invalid();
 if(raw.challenge!==undefined){
  const c=raw.challenge;if(!obj(c)||!text(c.token,256)||!currentUTCTimestamp(c.expiresAt))invalid();
  return Object.freeze({kind:'challenge',token:c.token,expiresAt:c.expiresAt});
 }
 if(raw.deviceApprovalPending!==undefined){
  if(raw.deviceApprovalPending!==true||!text(raw.deviceId,128)||!text(raw.installationId,128)||raw.session!==undefined)invalid();
  return Object.freeze({kind:'device-pending',deviceId:raw.deviceId,installationId:raw.installationId});
 }
 const snapshot=parseDirectSnapshot(raw);
 if(raw.passwordChangeRequired!==undefined&&typeof raw.passwordChangeRequired!=='boolean')invalid();
 const passwordChangeRequired=raw.passwordChangeRequired===true;
 const session=parseLocalSession(raw.session,{authority:'local',accountId:snapshot.account.id,serverId:snapshot.serverId,profileId:snapshot.account.primaryProfileId});
 const accountScoped=session.viewer.role==='account';
 // A viewer session straight from sign-in only for one unprotected profile and no forced password change.
 if(!accountScoped&&(passwordChangeRequired||snapshot.profiles.length!==1||snapshot.profiles[0]!.pinRequired))invalid();
 return Object.freeze({kind:'signed-in',snapshot,session,accountScoped,passwordChangeRequired});
}
/** `POST /v1/direct/sign-in`. */
export async function directSignIn(api:ProfileAPI,username:string,password:string,signal?:AbortSignal):Promise<DirectSignIn>{
 return parseDirectSignIn(await api.request<unknown>('/v1/direct/sign-in','POST',{username,password},signal));
}
/** Development builds only: the server's seeded e2e owner, if it has one (`GET /v1/dev/e2e`; 404 otherwise). */
export async function devE2EUsername(api:ProfileAPI,signal?:AbortSignal):Promise<string|undefined>{
 try{const raw=await api.request<unknown>('/v1/dev/e2e','GET',undefined,signal);return obj(raw)&&text(raw.username,64)?raw.username:undefined;}catch{return undefined;}
}
/** Development builds only: signs in as the seeded e2e owner without a password (`POST /v1/dev/e2e/sign-in`, loopback servers built with devtrust). */
export async function devE2ESignIn(api:ProfileAPI,username:string,signal?:AbortSignal):Promise<DirectSignIn>{
 return parseDirectSignIn(await api.request<unknown>('/v1/dev/e2e/sign-in','POST',{username},signal));
}
/** Finishes a two-step sign-in: the challenge token and an authenticator or recovery code (`POST /v1/auth/two-factor/challenge`). */
export async function completeDirectSignInChallenge(api:ProfileAPI,token:string,code:string,signal?:AbortSignal):Promise<DirectSignIn>{
 if(!text(token,256)||!text(code.trim(),64))throw new CodedError('code_required','Enter the code from your authenticator app or a recovery code.');
 const result=parseDirectSignIn(await api.request<unknown>('/v1/auth/two-factor/challenge','POST',{token,code:code.trim()},signal));
 if(result.kind==='challenge')invalid();
 return result;
}
/**
 * A required password change (sign-in said `passwordChangeRequired`): with that account session's
 * bearer, `POST /v1/direct/password {currentPassword, newPassword}`; then sign in again.
 */
export async function changeRequiredPassword(api:ProfileAPI,currentPassword:string,newPassword:string,signal?:AbortSignal):Promise<void>{
 await api.request('/v1/direct/password','POST',{currentPassword,newPassword},signal);
}
export function parseTrustedProfile(raw:unknown,scope:ProfileScope&{profileId:string;installationId:string},now=Date.now()):TrustedProfile{
 if(!obj(raw)||raw.authority!==scope.authority||raw.accountId!==scope.accountId||raw.profileId!==scope.profileId||raw.serverId!==scope.serverId||raw.installationId!==scope.installationId||!text(raw.token,4096)||!revision(raw.pinRevision)||!revision(raw.profileRevision)||!currentUTCTimestamp(raw.expiresAt)||Date.parse(raw.expiresAt)<=now||Date.parse(raw.expiresAt)>now+30*86400000+60000)invalid();
 for(const key of ['policyRevision','membershipRevision','trustRevision'])if(raw[key]!==undefined&&!revision(raw[key]))invalid();
 if(scope.authority==='hosted'&&(!revision(raw.policyRevision)||!revision(raw.trustRevision)))invalid();if(scope.authority==='local'&&!revision(raw.membershipRevision))invalid();
 return Object.freeze({token:raw.token,authority:scope.authority,accountId:scope.accountId,profileId:scope.profileId,serverId:scope.serverId!,installationId:scope.installationId,pinRevision:raw.pinRevision,profileRevision:raw.profileRevision,expiresAt:raw.expiresAt,...(raw.policyRevision===undefined?{}:{policyRevision:raw.policyRevision as number}),...(raw.membershipRevision===undefined?{}:{membershipRevision:raw.membershipRevision as number}),...(raw.trustRevision===undefined?{}:{trustRevision:raw.trustRevision as number})});
}
export async function selectDirectProfile(api:ProfileAPI,scope:ProfileScope&{serverId:string},profileId:string,input:ProfileSelectionInput,signal?:AbortSignal):Promise<DirectSelection>{
 if(scope.authority!=='local'||!text(profileId,128)||(input.pin!==undefined&&input.pin!==''&&!/^\d{4}$/.test(input.pin)))throw new CodedError('pin_format','Enter the four-digit profile PIN.');
 const raw=await api.request<unknown>('/v1/direct/profiles/'+encodeURIComponent(profileId)+'/select','POST',input,signal);if(!obj(raw))invalid();
 const session=parseLocalSession(raw.session,{authority:'local',accountId:scope.accountId,serverId:scope.serverId,profileId});
 const trustedSelection=raw.trustedSelection===undefined?undefined:parseTrustedProfile(raw.trustedSelection,{...scope,profileId,installationId:input.installationId??''});
 return Object.freeze({session,...(trustedSelection?{trustedSelection}:{})});
}
export function trustScopeKey(scope:ProfileScope&{profileId:string;installationId:string}):string{
 if(!['hosted','local'].includes(scope.authority)||![scope.accountId,scope.serverId,scope.profileId,scope.installationId].every(v=>text(v,128)))throw new CodedError('invalid_scope','A complete installation and viewer scope is required.');
 return JSON.stringify(['portico-profile-trust-v1',scope.authority,scope.accountId,scope.serverId,scope.profileId,scope.installationId]);
}
/** Validation and revision-bound mutations are shared by web, iOS and tvOS. */
export class ManagedProfilesClient {
 readonly api:ProfileAPI;readonly scope:ProfileScope;
 constructor(api:ProfileAPI,scope:ProfileScope){if(!text(scope.accountId,128)||!['local','hosted'].includes(scope.authority)||(scope.authority==='local'&&!text(scope.serverId,128)))throw new Error('An account profile scope is required.');this.api=api;this.scope=Object.freeze({...scope});}
 private path(profileId?:string):string{return(this.scope.authority==='local'?'/v1/direct':'/v1/account')+'/profiles'+(profileId?'/'+encodeURIComponent(profileId):'');}
 async list(signal?:AbortSignal):Promise<readonly ManagedProfile[]>{const raw=await this.api.request<unknown>(this.scope.authority==='local'?'/v1/direct':this.path(),'GET',undefined,signal);if(this.scope.authority==='local')return parseDirectSnapshot(raw,this.scope).profiles;if(!obj(raw)||raw.accountId!==this.scope.accountId||raw.maxProfiles!==8)invalid();return parseManagedProfiles(raw.profiles);}
 async create(name:string,art:ProfileArt='blue',signal?:AbortSignal):Promise<void>{if(!text(name.trim(),120)||!PROFILE_ART.includes(art))throw new Error('Enter a profile name.');await this.api.request(this.path(),'POST',{name:name.trim(),art},signal);}
 async edit(profile:ManagedProfile,change:{name?:string;art?:ProfileArt;pin?:string;policy?:ProfilePolicy},signal?:AbortSignal):Promise<void>{
  if(change.pin!==undefined&&change.pin!==''&&!/^\d{4}$/.test(change.pin))throw new Error('Use exactly four numeric digits for the profile PIN.');
  if(change.name!==undefined&&!text(change.name.trim(),120))throw new Error('Enter a profile name.');
  if(change.policy){if(this.scope.authority==='local'&&change.policy.serverId!==this.scope.serverId)throw new Error('This profile belongs to a different server.');if(change.policy.allowedLibraries!==null)ids(change.policy.allowedLibraries);}
  const policy=change.policy?(this.scope.authority==='local'?{allowedLibraries:change.policy.allowedLibraries}:change.policy):undefined;
  await this.api.request(this.path(profile.id),'PATCH',{...change,...(policy?{policy}:{}),expectedRevision:profile.revision},signal);
 }
 async order(profiles:readonly ManagedProfile[],signal?:AbortSignal):Promise<void>{await this.api.request(this.path()+'/order','PUT',{profiles:profiles.map(p=>({id:p.id,revision:p.revision}))},signal);}
 async forgetTrust(profileId:string,signal?:AbortSignal):Promise<void>{await this.api.request(this.scope.authority==='local'?'/v1/direct/trust':this.path(profileId)+'/trust','DELETE',this.scope.authority==='local'?{profileId}:undefined,signal);}
 async policies(profileId:string,signal?:AbortSignal):Promise<readonly ProfilePolicy[]>{
  if(this.scope.authority==='local'){const rows=await this.list(signal);const p=rows.find(v=>v.id===profileId);if(!p)invalid();return[{serverId:this.scope.serverId!,allowedLibraries:p.allowedLibraries??null}];}
  const raw=await this.api.request<unknown>(this.path(profileId)+'/policies','GET',undefined,signal);if(!obj(raw)||!Array.isArray(raw.items)||raw.items.length>1000)invalid();return raw.items.map(v=>{if(!obj(v)||!text(v.serverId,128))invalid();return Object.freeze({serverId:v.serverId,allowedLibraries:v.allowedLibraries===null?null:ids(v.allowedLibraries)});});
 }
 async previewDelete(profile:ManagedProfile,signal?:AbortSignal):Promise<ProfileDeletion>{
  if(profile.primary)throw new Error('The primary profile is managed through account settings.');const raw=await this.api.request<unknown>(this.path(profile.id)+'/deletion-preview','GET',undefined,signal);
  if(!obj(raw)||raw.profileId!==profile.id||!revision(raw.revision)||raw.primary!==false)invalid();return raw as ProfileDeletion;
 }
 async delete(preview:ProfileDeletion,signal?:AbortSignal):Promise<ProfileDeletion|undefined>{
  if(preview.primary||!revision(preview.revision))throw new Error('Preview this profile deletion first.');return this.api.request<ProfileDeletion|undefined>(this.path(preview.profileId),'DELETE',{expectedRevision:preview.revision,deleteSavedResources:true},signal);
 }
 async deletionStatus(id:string,signal?:AbortSignal):Promise<ProfileDeletion>{if(this.scope.authority!=='hosted'||!text(id,128))invalid();const raw=await this.api.request<ProfileDeletion>('/v1/account/profile-deletions/'+encodeURIComponent(id),'GET',undefined,signal);if(raw.id!==id)invalid();return raw;}
}
