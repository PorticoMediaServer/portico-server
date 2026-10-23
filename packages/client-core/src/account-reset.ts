import {routePath,routeBody} from '../../contracts/src/routes.ts';
import {parseDirectSignIn,type DirectSignIn,type ProfileAPI} from './profile-management.ts';

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
export async function makeMemberResetCode(api:ProfileAPI,id:string,expectedRevision:number,turnOffTwoStep=false,signal?:AbortSignal):Promise<Readonly<{code:string;expiresAt:string}>>{
 const raw=await api.request<{code?:unknown;expiresAt?:unknown}>(routePath('post_direct_members_id_reset_code',{id}),'POST',routeBody('post_direct_members_id_reset_code',{expectedRevision,turnOffTwoStep}),signal);
 if(typeof raw?.code!=='string'||!/^[-_A-Za-z0-9]{43}$/.test(raw.code)||typeof raw.expiresAt!=='string'||!Number.isFinite(Date.parse(raw.expiresAt)))throw new Error('The member reset response was invalid.');
 return Object.freeze({code:raw.code,expiresAt:raw.expiresAt});
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
