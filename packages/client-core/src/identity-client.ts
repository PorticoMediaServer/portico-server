import {unreadableServerResponse} from './server-messages.ts';
/** Typed calls for the identity workstream on a direct server: profile restrictions and
 * devices, two-factor. Parsing is strict; these functions add no state. */
import {parseDevice,parseDevices,parseRememberedAccounts,validInstallationId,type Device,type RememberedAccount} from './identity-devices.ts';
import {parsePINRecoveryMethods,parseProfileAvatar,parseProfileRestrictions,parseRatingSystems,parseTwoFactorState,restrictionEdit,type PINRecoveryMethods,type ProfileAvatar,type ProfileRestrictions,type RatingSystem,type TwoFactorState} from './identity-restrictions.ts';

export type IdentityApi={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};
export type DeviceDescription=Readonly<{installationId:string;name:string;platform:string;app:string;appVersion:string}>;
export type TwoFactorEnrolment=Readonly<{secret:string;uri:string;recoveryCodes:readonly string[]}>;
const id=(v:string)=>encodeURIComponent(v);

export function parseTwoFactorEnrolment(raw:unknown):TwoFactorEnrolment{
 const v=raw as Record<string,unknown>|null;
 if(!v||typeof v.secret!=='string'||!/^[A-Z2-7]{16,128}$/.test(v.secret)||typeof v.uri!=='string'||!v.uri.startsWith('otpauth://totp/')||v.uri.length>1024)throw new Error(unreadableServerResponse);
 if(!Array.isArray(v.recoveryCodes)||v.recoveryCodes.length>20||v.recoveryCodes.some(c=>typeof c!=='string'||c.length<6||c.length>64))throw new Error(unreadableServerResponse);
 return Object.freeze({secret:v.secret,uri:v.uri,recoveryCodes:Object.freeze([...v.recoveryCodes as string[]])});
}

/** Sends bytes rather than JSON; only a picture upload needs it. The app supplies it because
 * only the app holds the bearer and the platform's way of naming a file's bytes. */
export type IdentityUpload=(path:string,body:Blob|ArrayBuffer|Uint8Array,contentType:string)=>Promise<unknown>;
export const AVATAR_MAX_BYTES=4*1024*1024;

export class IdentityClient{
 private api:IdentityApi;private upload?:IdentityUpload;
 constructor(api:IdentityApi,upload?:IdentityUpload){this.api=api;this.upload=upload;}
 /** The server sends `{items}`; the parser takes the list. */
 async ratingSystems(signal?:AbortSignal):Promise<readonly RatingSystem[]>{const raw=await this.api.request<{items?:unknown}>('/v1/rating-systems','GET',undefined,signal);return parseRatingSystems(raw?.items);}
 async restrictions(profileId:string,signal?:AbortSignal):Promise<ProfileRestrictions>{return parseProfileRestrictions(await this.api.request<unknown>(`/v1/direct/profiles/${id(profileId)}/restrictions`,'GET',undefined,signal));}
 /** C45: the signed-in account session manages restrictions and PINs; there is no step-up proof. */
 async saveRestrictions(current:ProfileRestrictions,changes:Parameters<typeof restrictionEdit>[1]):Promise<ProfileRestrictions>{
  return parseProfileRestrictions(await this.api.request<unknown>(`/v1/direct/profiles/${id(current.profileId)}/restrictions`,'PUT',restrictionEdit(current,changes)));
 }
 async pinRecovery():Promise<PINRecoveryMethods>{return parsePINRecoveryMethods(await this.api.request<unknown>('/v1/direct/pin-recovery'));}
 async resetPIN(profileId:string,pin:string):Promise<void>{await this.api.request(`/v1/direct/profiles/${id(profileId)}/pin-reset`,'POST',{pin});}
 async removeAvatar(profileId:string):Promise<void>{await this.api.request(`/v1/direct/profiles/${id(profileId)}/avatar`,'DELETE');}
 avatar(raw:unknown):ProfileAvatar{return parseProfileAvatar(raw);}
 /** Every profile's current picture version in one read; a profile with none is absent. */
 async avatars(signal?:AbortSignal):Promise<readonly ProfileAvatar[]>{const raw=await this.api.request<{items?:unknown}>('/v1/direct/profiles/avatars','GET',undefined,signal);if(!Array.isArray(raw?.items)||raw.items.length>64)throw new Error(unreadableServerResponse);return Object.freeze(raw.items.map(parseProfileAvatar));}
 /** The server sniffs, crops and re-encodes the picture itself; the type sent here is only a courtesy. */
 async uploadAvatar(profileId:string,picture:Blob|ArrayBuffer|Uint8Array,contentType:string):Promise<ProfileAvatar>{
  if(!this.upload)throw new Error('This device can’t send pictures.');
  const size=picture instanceof Blob?picture.size:picture.byteLength;
  if(!size)throw new Error('That file is empty.');if(size>AVATAR_MAX_BYTES)throw new Error('Choose a picture smaller than 4 MB.');
  return parseProfileAvatar(await this.upload(`/v1/direct/profiles/${id(profileId)}/avatar`,picture,/^image\/(jpeg|png|webp)$/.test(contentType)?contentType:'application/octet-stream'));
 }

 async devices(installationId:string,signal?:AbortSignal):Promise<readonly Device[]>{
  return parseDevices(await this.api.request<unknown>('/v1/devices?installationId='+id(installationId),'GET',undefined,signal));
 }
 /** Tells the server which device this is, at sign-in. It refreshes the record when the
  * installation is already known and never overwrites a name a person gave the device. Under an
  * owner's approve-new-devices policy a new arrival is refused with `device_approval_pending`. */
 async registerDevice(device:DeviceDescription):Promise<Device>{return parseDevice(await this.api.request<unknown>('/v1/devices','POST',device));}
 /** Ties this viewing session to the device, so "sign this device out" knows what to end. */
 async bindDevice(deviceId:string,installationId:string,sessionFamilyId:string):Promise<void>{await this.api.request(`/v1/devices/${id(deviceId)}/sessions/bind`,'POST',{installationId,sessionFamilyId});}
 /** Who has used this installation. Asked before anyone is signed in; nothing in it is a credential. */
 async rememberedAccounts(installationId:string,signal?:AbortSignal):Promise<readonly RememberedAccount[]>{return parseRememberedAccounts(await this.api.request<unknown>('/v1/auth/remembered-accounts?installationId='+id(installationId),'GET',undefined,signal));}
 async rememberAccount(installationId:string,automaticSignIn:boolean):Promise<readonly RememberedAccount[]>{return parseRememberedAccounts(await this.api.request<unknown>('/v1/direct/remembered-accounts','POST',{installationId,automaticSignIn}));}
 async forgetAccount(accountId:string,installationId:string):Promise<void>{await this.api.request(`/v1/direct/remembered-accounts/${id(accountId)}?installationId=${id(installationId)}`,'DELETE');}
 async editDevice(deviceId:string,edit:{name?:string;trusted?:boolean;rememberAccount?:boolean}):Promise<Device>{return parseDevice(await this.api.request<unknown>(`/v1/devices/${id(deviceId)}`,'PATCH',edit));}
 async approveDevice(deviceId:string,approved:boolean):Promise<Device>{return parseDevice(await this.api.request<unknown>(`/v1/devices/${id(deviceId)}/approval`,'POST',{approved}));}
 async signOutDevice(deviceId:string):Promise<void>{await this.api.request(`/v1/devices/${id(deviceId)}/sessions`,'DELETE');}
 async removeDevice(deviceId:string):Promise<void>{await this.api.request(`/v1/devices/${id(deviceId)}`,'DELETE');}
 /** Ends every session of the account (C45: no password; the signed-in session is the authority). */
 async signOutEverywhere():Promise<void>{await this.api.request('/v1/direct/sessions/sign-out-everywhere','POST');}
 /** A direct account's password. C45: the current password always, plus `code` (authenticator or
  * recovery code) when two-step verification is on, in this one request. */
 async changePassword(currentPassword:string,newPassword:string,code?:string):Promise<void>{await this.api.request('/v1/direct/password','POST',{currentPassword,newPassword,...(code?{code}:{})});}

 async twoFactor(signal?:AbortSignal):Promise<TwoFactorState>{return parseTwoFactorState(await this.api.request<unknown>('/v1/direct/two-factor','GET',undefined,signal));}
 async enrolTwoFactor(password:string):Promise<TwoFactorEnrolment>{return parseTwoFactorEnrolment(await this.api.request<unknown>('/v1/direct/two-factor/enrol','POST',{password}));}
 /** C45: confirming enrolment sends the current password with the first code. */
 async verifyTwoFactor(password:string,code:string):Promise<TwoFactorState>{return parseTwoFactorState(await this.api.request<unknown>('/v1/direct/two-factor/verify','POST',{password,code}));}
 /** C45: turning it off sends the current password and a current code. */
 async disableTwoFactor(password:string,code:string):Promise<void>{await this.api.request('/v1/direct/two-factor','DELETE',{password,code});}
}

/** Registers this device and binds the session in one step, after a direct sign-in. Returns
 * `pending` when the owner must approve the device first, `denied` when the owner turned it away
 * (`device_denied`); any other failure is `skipped`, because a device list that is briefly
 * incomplete must never cost someone their sign-in. */
export async function announceDevice(identity:IdentityClient,device:DeviceDescription,sessionFamilyId:string):Promise<'bound'|'pending'|'denied'|'skipped'>{
 if(!validInstallationId(device.installationId))return 'skipped';
 try{const record=await identity.registerDevice(device);await identity.bindDevice(record.id,device.installationId,sessionFamilyId);return 'bound';}
 catch(e){const code=(e as {code?:string}|null)?.code;return code==='device_approval_pending'?'pending':code==='device_denied'?'denied':'skipped';}
}
