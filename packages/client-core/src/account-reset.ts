import {routePath,routeBody} from '../../contracts/src/routes.ts';
import {parseDirectSignIn,type DirectSignIn,type ProfileAPI} from './profile-management.ts';
import {identityCode} from './route-identity.ts';
import {parseTwoFactorState} from './identity-restrictions.ts';

export type ResetFileLocation=Readonly<{systemKind:'linux'|'windows'|'macos'|'docker';dataDirectory:string;fileDirectory:string;fileName:'reset-code.txt';retryAfterSeconds:number}>;
export function parseResetFileLocation(raw:unknown):ResetFileLocation {
 const v=raw as Record<string,unknown>;
 if(!v||typeof v!=='object'||!['linux','windows','macos','docker'].includes(String(v.systemKind))||typeof v.dataDirectory!=='string'||!v.dataDirectory||typeof v.fileDirectory!=='string'||!v.fileDirectory||v.fileName!=='reset-code.txt'||!Number.isInteger(v.retryAfterSeconds)||Number(v.retryAfterSeconds)<0||Number(v.retryAfterSeconds)>60||'code' in v||'username' in v||'account' in v||'session' in v)throw new Error('The reset file response was invalid.');
 return Object.freeze({systemKind:v.systemKind as ResetFileLocation['systemKind'],dataDirectory:v.dataDirectory,fileDirectory:v.fileDirectory,fileName:'reset-code.txt',retryAfterSeconds:Number(v.retryAfterSeconds)});
}
export async function requestOwnerResetFile(api:ProfileAPI,username?:string,signal?:AbortSignal):Promise<ResetFileLocation>{
 return parseResetFileLocation(await api.request(routePath('post_auth_owner_reset_request'),'POST',routeBody('post_auth_owner_reset_request',username===undefined?{}:{username}),signal));
}
export async function verifyOwnerResetFile(api:ProfileAPI,code:string,turnOffTwoStep=false,signal?:AbortSignal):Promise<DirectSignIn>{
 const out=parseDirectSignIn(await api.request(routePath('post_auth_owner_reset_verify'),'POST',routeBody('post_auth_owner_reset_verify',{code:code.trim(),turnOffTwoStep}),signal));
 if(out.kind!=='signed-in'||(!out.passwordChangeRequired&&!out.resetFileOwnerSession)||out.session.refreshToken)throw new Error('The reset file did not return the expected session.');
 return out;
}
export type MemberResetCode=Readonly<{code:string;expiresAt:string;serverId?:string}>;
export async function makeMemberResetCode(api:ProfileAPI,id:string,expectedRevision:number,turnOffTwoStep=false,signal?:AbortSignal):Promise<MemberResetCode>{
 const raw=await api.request<{code?:unknown;expiresAt?:unknown;serverId?:unknown}>(routePath('post_direct_members_id_reset_code',{id}),'POST',routeBody('post_direct_members_id_reset_code',{expectedRevision,turnOffTwoStep}),signal);
 if(typeof raw?.code!=='string'||!/^[-_A-Za-z0-9]{43}$/.test(raw.code)||typeof raw.expiresAt!=='string'||!Number.isFinite(Date.parse(raw.expiresAt)))throw new Error('The member reset response was invalid.');
 if(raw.serverId!==undefined){if(typeof raw.serverId!=='string')throw new Error('The member reset response was invalid.');identityCode(raw.serverId);}
 return Object.freeze({code:raw.code,expiresAt:raw.expiresAt,...(raw.serverId===undefined?{}:{serverId:raw.serverId as string})});
}
export async function verifyMemberResetCode(api:ProfileAPI,username:string,code:string,signal?:AbortSignal):Promise<DirectSignIn>{
 const out=parseDirectSignIn(await api.request(routePath('post_direct_password_reset'),'POST',routeBody('post_direct_password_reset',{username,code:code.trim()}),signal));
 if(out.kind!=='signed-in'||!out.passwordChangeRequired||!out.accountScoped||out.session.refreshToken||out.resetFileOwnerSession)throw new Error('The member reset did not return a password-change session.');
 return out;
}
/** The one-hour account session from a reset code may only choose its password.
 * Keeping two-step verification requires its ordinary authenticator/recovery proof. */
export async function changeResetPassword(api:ProfileAPI,newPassword:string,secondFactorCode='',signal?:AbortSignal):Promise<void>{
 await api.request(routePath('post_direct_password'),'POST',routeBody('post_direct_password',{currentPassword:'',newPassword,code:secondFactorCode.trim()}),signal);
}

function ownerPasswordGrant(grant:DirectSignIn){
 if(grant.kind!=='signed-in'||!grant.passwordChangeRequired||!grant.accountScoped||grant.session.refreshToken||grant.snapshot.account.role!=='owner'||grant.resetFileOwnerSession)throw new Error('The owner reset proof is not current.');
 return grant;
}
/** Existing security read, after the one-use file was proved. The API must be
 * bound to that restricted grant; this helper never publishes or replaces it. */
export async function ownerResetSecurity(api:ProfileAPI,grant:DirectSignIn,signal?:AbortSignal):Promise<Readonly<{username:string;twoStepEnabled:boolean}>>{
 const current=ownerPasswordGrant(grant);
 const state=parseTwoFactorState(await api.request(routePath('get_direct_two_factor'),'GET',undefined,signal));
 return Object.freeze({username:current.snapshot.account.username,twoStepEnabled:state.enabled});
}
/** The existing password command commits this choice and the new password in
 * one writer. Fresh sign-in still needs the preserved factor when left on. */
export async function finishOwnerResetPassword(api:ProfileAPI,grant:DirectSignIn,newPassword:string,turnOffTwoStep:boolean,signal?:AbortSignal):Promise<void>{
 ownerPasswordGrant(grant);
 if(typeof turnOffTwoStep!=='boolean')throw new Error('The two-step choice is invalid.');
 const body={currentPassword:'',newPassword,code:'',turnOffTwoStep};
 await api.request(routePath('post_direct_password'),'POST',routeBody('post_direct_password',body),signal);
}
