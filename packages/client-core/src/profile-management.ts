import {routePath as developmentRoutePath} from "../../contracts/src/development/routes.ts";
import {routeBody} from "../../contracts/src/routes.ts";
import type {RouteResponse} from "../../contracts/src/routes.ts";
import {routePath, encodedRoutePath} from "../../contracts/src/routes.ts";
import {CodedError,unreadableServerResponse} from './server-messages.ts';
import {parseLocalSession,currentUTCTimestamp} from './local-session.ts';
import type {LocalSession} from './index.ts';
import {keepReadable,oneOfOr} from './tolerant.ts';

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
 Readonly<{kind:'signed-in';snapshot:DirectSnapshot;session:LocalSession;accountScoped:boolean;passwordChangeRequired:boolean;resetFileOwnerSession?:true}>
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
/**
 * A picture colour this app does not know reads as `blue`, the colour a new profile starts with; the name sent is never kept.
 * Everything else about a profile is exact, its PIN requirement and allowed libraries above all. Where the whole list is relied
 * on (sign-in, the account snapshot) one unreadable profile refuses the list. With `readable`, a list read for display leaves
 * that profile out instead: it is never shown with part of it missing. Either way there is exactly one primary profile, no id
 * appears twice and the list is no longer than an account may hold.
 */
export function parseManagedProfiles(raw:unknown,readable=false):readonly ManagedProfile[]{
 if(!Array.isArray(raw)||raw.length<1||raw.length>8)invalid();
 const row=(v:unknown)=>{if(!obj(v)||!text(v.id,128)||!text(v.name,120)||typeof v.primary!=='boolean'||!natural(v.position)||!text(v.art,64)||!revision(v.revision)||!revision(v.pinRevision)||typeof v.pinRequired!=='boolean'||(v.trustRevision!==undefined&&!revision(v.trustRevision)))invalid();
  return Object.freeze({id:v.id,name:v.name,primary:v.primary,position:v.position,art:oneOfOr('profiles','profile.art',v.art,PROFILE_ART,'blue'),revision:v.revision,pinRevision:v.pinRevision,pinRequired:v.pinRequired,...(v.trustRevision===undefined?{}:{trustRevision:v.trustRevision as number}),...(v.allowedLibraries===undefined?{}:{allowedLibraries:v.allowedLibraries===null?null:ids(v.allowedLibraries)})});};
 const result:ManagedProfile[]=readable?keepReadable('profiles','rows',raw,row):raw.map(row);
 if(new Set(result.map(v=>v.id)).size!==result.length||result.filter(v=>v.primary).length!==1)invalid();
 return Object.freeze(result.sort((a,b)=>a.position-b.position||a.id.localeCompare(b.id)));
}
export function parseDirectSnapshot(raw:unknown,expected:Partial<ProfileScope>={},readableProfiles=false):DirectSnapshot{
 if(!obj(raw)||raw.authority!=='local'||!text(raw.serverId,128)||!obj(raw.account)||typeof raw.canManage!=='boolean'||(raw.custodyPending!==undefined&&typeof raw.custodyPending!=='boolean'))invalid();const a=raw.account;
 if(!text(a.id,128)||!text(a.username,64)||!text(a.primaryProfileId,128)||!['owner','admin','member'].includes(a.role as string)||(a.hostedAccountId!==undefined&&!text(a.hostedAccountId,128))||!revision(a.revision)||typeof a.disabled!=='boolean'||(expected.accountId&&a.id!==expected.accountId)||(expected.serverId&&raw.serverId!==expected.serverId)||(expected.authority&&expected.authority!=='local'))invalid();
 const profiles=parseManagedProfiles(raw.profiles,readableProfiles);if(!profiles.some(p=>p.primary&&p.id===a.primaryProfileId))invalid();
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
 if(raw.resetFileOwnerSession!==undefined&&raw.resetFileOwnerSession!==true)invalid();
 const resetFileOwnerSession=raw.resetFileOwnerSession===true;
 if(resetFileOwnerSession&&(accountScoped||passwordChangeRequired||snapshot.account.role!=='owner'||session.viewer.role!=='owner'||session.refreshToken))invalid();

 // A viewer session straight from sign-in only for one unprotected profile and no forced password change.
 if(!accountScoped&&!resetFileOwnerSession&&(passwordChangeRequired||snapshot.profiles.length!==1||snapshot.profiles[0]!.pinRequired))invalid();
 return Object.freeze({kind:'signed-in',snapshot,session,accountScoped,passwordChangeRequired,...(resetFileOwnerSession?{resetFileOwnerSession:true as const}:{})});
}
/** `POST /v1/direct/sign-in`. */
export async function directSignIn(api:ProfileAPI,username:string,password:string,signal?:AbortSignal):Promise<DirectSignIn>{
 return parseDirectSignIn(await api.request<RouteResponse<"post_direct_sign_in">>(routePath("post_direct_sign_in"),'POST',routeBody("post_direct_sign_in", {username,password}),signal));
}
/** Development builds only: the server's seeded e2e owner, if it has one (`GET /v1/dev/e2e`; 404 otherwise). */
export async function devE2EUsername(api:ProfileAPI,signal?:AbortSignal):Promise<string|undefined>{
 try{const raw=await api.request<unknown>(developmentRoutePath("get_dev_e2e"),'GET',undefined,signal);return obj(raw)&&text(raw.username,64)?raw.username:undefined;}catch{return undefined;}
}
/** Development builds only: signs in as the seeded e2e owner without a password (`POST /v1/dev/e2e/sign-in`, loopback servers built with devtrust). */
export async function devE2ESignIn(api:ProfileAPI,username:string,signal?:AbortSignal):Promise<DirectSignIn>{
 return parseDirectSignIn(await api.request<unknown>(developmentRoutePath("post_dev_e2e_sign_in"),'POST',{username},signal));
}
/** Finishes a two-step sign-in: the challenge token and an authenticator or recovery code (`POST /v1/auth/two-factor/challenge`). */
export async function completeDirectSignInChallenge(api:ProfileAPI,token:string,code:string,signal?:AbortSignal):Promise<DirectSignIn>{
 if(!text(token,256)||!text(code.trim(),64))throw new CodedError('code_required','Enter the code from your authenticator app or a recovery code.');
 const result=parseDirectSignIn(await api.request<RouteResponse<"post_auth_two_factor_challenge">>(routePath("post_auth_two_factor_challenge"),'POST',routeBody("post_auth_two_factor_challenge", {token,code:code.trim()}),signal));
 if(result.kind==='challenge')invalid();
 return result;
}
/**
 * A required password change (sign-in said `passwordChangeRequired`): with that account session's
 * bearer, `POST /v1/direct/password {currentPassword, newPassword}`; then sign in again.
 */
export async function changeRequiredPassword(api:ProfileAPI,currentPassword:string,newPassword:string,signal?:AbortSignal):Promise<void>{
 await api.request(routePath("post_direct_password"),'POST',routeBody("post_direct_password", {currentPassword,newPassword}),signal);
}
export function parseTrustedProfile(raw:unknown,scope:ProfileScope&{profileId:string;installationId:string},now=Date.now()):TrustedProfile{
 if(!obj(raw)||raw.authority!==scope.authority||raw.accountId!==scope.accountId||raw.profileId!==scope.profileId||raw.serverId!==scope.serverId||raw.installationId!==scope.installationId||!text(raw.token,4096)||!revision(raw.pinRevision)||!revision(raw.profileRevision)||!currentUTCTimestamp(raw.expiresAt)||Date.parse(raw.expiresAt)<=now||Date.parse(raw.expiresAt)>now+30*86400000+60000)invalid();
 for(const key of ['policyRevision','membershipRevision','trustRevision'])if(raw[key]!==undefined&&!revision(raw[key]))invalid();
 if(scope.authority==='hosted'&&(!revision(raw.policyRevision)||!revision(raw.trustRevision)))invalid();if(scope.authority==='local'&&!revision(raw.membershipRevision))invalid();
 return Object.freeze({token:raw.token,authority:scope.authority,accountId:scope.accountId,profileId:scope.profileId,serverId:scope.serverId!,installationId:scope.installationId,pinRevision:raw.pinRevision,profileRevision:raw.profileRevision,expiresAt:raw.expiresAt,...(raw.policyRevision===undefined?{}:{policyRevision:raw.policyRevision as number}),...(raw.membershipRevision===undefined?{}:{membershipRevision:raw.membershipRevision as number}),...(raw.trustRevision===undefined?{}:{trustRevision:raw.trustRevision as number})});
}
export async function selectDirectProfile(api:ProfileAPI,scope:ProfileScope&{serverId:string},profileId:string,input:ProfileSelectionInput,signal?:AbortSignal):Promise<DirectSelection>{
 if(scope.authority!=='local'||!text(profileId,128)||(input.pin!==undefined&&input.pin!==''&&!/^\d{4}$/.test(input.pin)))throw new CodedError('pin_format','Enter the four-digit profile PIN.');
 const raw=await api.request<RouteResponse<"post_direct_profiles_id_select">>(encodedRoutePath("post_direct_profiles_id_select", {"id": encodeURIComponent(profileId)}),'POST',input,signal);if(!obj(raw))invalid();
 const session=parseLocalSession(raw.session,{authority:'local',accountId:scope.accountId,serverId:scope.serverId,profileId});
 const trustedSelection=raw.trustedSelection===undefined?undefined:parseTrustedProfile(raw.trustedSelection,{...scope,profileId,installationId:input.installationId??''});
 return Object.freeze({session,...(trustedSelection?{trustedSelection}:{})});
}
export function trustScopeKey(scope:ProfileScope&{profileId:string;installationId:string}):string{
 if(!['hosted','local'].includes(scope.authority)||![scope.accountId,scope.serverId,scope.profileId,scope.installationId].every(v=>text(v,128)))throw new CodedError('invalid_scope','A complete installation and viewer scope is required.');
 return JSON.stringify(['portico-profile-trust-v1',scope.authority,scope.accountId,scope.serverId,scope.profileId,scope.installationId]);
}
/** Validation and revision-bound mutations are shared by web, iOS and tvOS. */
/**
 * The profiles of a local account, managed on its own server. (Hosted Portico Account profiles
 * have no API of their own: an account's profiles are each server's, so they are managed here.)
 */
export class ManagedProfilesClient {
 readonly api:ProfileAPI;readonly scope:ProfileScope;
 constructor(api:ProfileAPI,scope:ProfileScope){if(scope.authority!=='local'||!text(scope.accountId,128)||!text(scope.serverId,128))throw new Error('An account profile scope is required.');this.api=api;this.scope=Object.freeze({...scope});}
 private path(profileId:string|undefined,action:'create'|'edit'|'delete'|'order'|'preview'):string{
  if(action==='create')return routePath("post_direct_profiles");
  if(action==='order')return routePath("put_direct_profiles_order");
  if(!profileId)throw new Error('A profile is required.');
  if(action==='edit')return routePath("patch_direct_profiles_id",{id:profileId});
  if(action==='delete')return routePath("delete_direct_profiles_id",{id:profileId});
  return routePath("get_direct_profiles_id_deletion_preview",{id:profileId});
 }
 async list(signal?:AbortSignal):Promise<readonly ManagedProfile[]>{const raw=await this.api.request<RouteResponse<"get_direct_account">>(routePath("get_direct_account"),'GET',undefined,signal);return parseDirectSnapshot(raw,this.scope,true).profiles;}
 async create(name:string,art:ProfileArt='blue',signal?:AbortSignal):Promise<void>{if(!text(name.trim(),120)||!PROFILE_ART.includes(art))throw new Error('Enter a profile name.');await this.api.request(this.path(undefined,'create'),'POST',{name:name.trim(),art},signal);}
 async edit(profile:ManagedProfile,change:{name?:string;art?:ProfileArt;pin?:string;policy?:ProfilePolicy},signal?:AbortSignal):Promise<void>{
  if(change.pin!==undefined&&change.pin!==''&&!/^\d{4}$/.test(change.pin))throw new Error('Use exactly four numeric digits for the profile PIN.');
  if(change.name!==undefined&&!text(change.name.trim(),120))throw new Error('Enter a profile name.');
  if(change.policy){if(change.policy.serverId!==this.scope.serverId)throw new Error('This profile belongs to a different server.');if(change.policy.allowedLibraries!==null)ids(change.policy.allowedLibraries);}
  const policy=change.policy?{allowedLibraries:change.policy.allowedLibraries}:undefined;
  await this.api.request(this.path(profile.id,'edit'),'PATCH',{...change,...(policy?{policy}:{}),expectedRevision:profile.revision},signal);
 }
 async order(profiles:readonly ManagedProfile[],signal?:AbortSignal):Promise<void>{await this.api.request(this.path(undefined,'order'),'PUT',{profiles:profiles.map(p=>({id:p.id,revision:p.revision}))},signal);}
 async forgetTrust(profileId:string,signal?:AbortSignal):Promise<void>{await this.api.request(routePath("delete_direct_trust"),'DELETE',{profileId},signal);}
 async previewDelete(profile:ManagedProfile,signal?:AbortSignal):Promise<ProfileDeletion>{
  if(profile.primary)throw new Error('The primary profile is managed through account settings.');const raw=await this.api.request<unknown>(this.path(profile.id,'preview'),'GET',undefined,signal);
  if(!obj(raw)||raw.profileId!==profile.id||!revision(raw.revision)||raw.primary!==false)invalid();return raw as ProfileDeletion;
 }
 async delete(preview:ProfileDeletion,signal?:AbortSignal):Promise<ProfileDeletion|undefined>{
  if(preview.primary||!revision(preview.revision))throw new Error('Preview this profile deletion first.');return this.api.request<ProfileDeletion|undefined>(this.path(preview.profileId,'delete'),'DELETE',{expectedRevision:preview.revision,deleteSavedResources:true},signal);
 }
}

/** Retire a temporary account sign-in only after a distinct viewer family was accepted.
 * A direct sign-in may already be the viewer session, and deleting that bearer signs the viewer out. */
export function shouldRetireAccountSession(account:Pick<LocalSession,'accessToken'|'sessionFamilyId'>|undefined,viewer:Pick<LocalSession,'accessToken'|'sessionFamilyId'>):boolean {
 return !!account && !!account.accessToken && !!account.sessionFamilyId && !!viewer.sessionFamilyId && account.accessToken!==viewer.accessToken && account.sessionFamilyId!==viewer.sessionFamilyId;
}

/** A fresh sign-in proves Forgot PIN without replacing the installed viewer session.
 * `authenticate` completes local/Portico sign-in and any configured second factor.
 * The temporary transport is used only for this reset and then signed out.
 */
export async function resetProfilePINWithFreshSignIn(authenticate:()=>Promise<DirectSignIn>,temporaryAPI:(session:LocalSession)=>ProfileAPI,scope:ProfileScope&{serverId:string},profileId:string,pin:string,signal?:AbortSignal):Promise<void>{
 if(scope.authority!=='local'||!text(profileId,128)||pin!==''&&!/^\d{4}$/.test(pin))throw new CodedError('pin_format','Enter the four-digit profile PIN.');
 const signIn=await authenticate();if(signIn.kind!=='signed-in')throw new CodedError('authentication_required','Complete account sign-in first.');
 const api=temporaryAPI(signIn.session);
 try{
  if(signIn.snapshot.account.id!==scope.accountId||signIn.snapshot.serverId!==scope.serverId||!signIn.snapshot.profiles.some(p=>p.id===profileId))throw new CodedError('invalid_scope','Sign in to the account that owns this profile.');
  await api.request(routePath('post_direct_profile_pin_reset',{id:profileId}),'POST',routeBody('post_direct_profile_pin_reset',{pin}),signal);
 }finally{
  try{await api.request(routePath('delete_sessions_current'),'DELETE');}catch(e){if(!(e instanceof CodedError)||!['unauthorized','authentication_required','session_ended'].includes(e.code))throw e;}
 }
}
