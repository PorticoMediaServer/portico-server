/** Same creation/change policy as Hosted; old password hashes remain valid. */
export function validDirectPassword(value:string):boolean {
 return Array.from(value).length>=8&&new TextEncoder().encode(value).length<=72&&/\p{Lu}/u.test(value)&&/\p{Ll}/u.test(value)&&/[^\p{L}]/u.test(value);
}

import {routePath,routeBody,type RouteRequest,type RouteResponse} from '../../contracts/src/routes.ts';
import type {ProfileAPI} from './profile-management.ts';
import {CodedError} from './server-messages.ts';
import {identityCode} from './route-identity.ts';

export type ServerInvitation=RouteResponse<'get_admin_access_invitation'>;
export type ServerInvitationLink=Readonly<{code:string;expiresAt:string;serverId?:string}>;
export type PeopleMember=RouteResponse<'get_admin_access_member'>;
export type PublicInvitation=Readonly<{serverName:string;expiresAt:string;role:'admin'|'member';libraries:readonly {name:string}[];allLibraries:boolean;signInMethod:'local'|'portico';inviterUsername?:string}>;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const bounded=(v:unknown,max:number):v is string=>typeof v==='string'&&v.length>0&&Array.from(v).length<=max&&!/[\p{Cc}\p{Cf}]/u.test(v);
const timestamp=(v:unknown):v is string=>typeof v==='string'&&/^\d{4}-\d\d-\d\dT/.test(v)&&Number.isFinite(Date.parse(v));
const rev=(v:unknown):v is number=>Number.isSafeInteger(v)&&Number(v)>0;
function invalidPeople():never{throw new CodedError('invalid_response','The server returned an unreadable people response.');}
/** Optional administrator text is never a recipient identity or part of a share link. */
export function privatePersonNotes(raw:string|null|undefined):string|null{
 if(raw==null)return null;
 const value=raw.trim();
 if(Array.from(value).length>500||/[\p{Cc}\p{Cf}]/u.test(value))throw new CodedError('invalid_notes','Use up to 500 characters for notes.');
 return value||null;
}
export function parsePublicInvitation(raw:unknown):PublicInvitation{
 if(!object(raw)||!bounded(raw.serverName,120)||!timestamp(raw.expiresAt)||!['member','admin'].includes(String(raw.role))||!['local','portico'].includes(String(raw.signInMethod))||typeof raw.allLibraries!=='boolean'||!Array.isArray(raw.libraries)||raw.libraries.length>256)invalidPeople();
 for(const field of ['notes','email','porticoUsername'])if(raw[field]!==undefined)invalidPeople();
 const libraries=raw.libraries.map(v=>{if(!object(v)||!bounded(v.name,256))invalidPeople();return Object.freeze({name:v.name});});
 if(raw.inviterUsername!==undefined&&!bounded(raw.inviterUsername,64))invalidPeople();
 return Object.freeze({serverName:raw.serverName,expiresAt:raw.expiresAt,role:raw.role as 'member'|'admin',libraries:Object.freeze(libraries),allLibraries:raw.allLibraries,signInMethod:raw.signInMethod as 'local'|'portico',...(raw.inviterUsername?{inviterUsername:raw.inviterUsername as string}:{})});
}
function parseServerInvitation(raw:unknown):ServerInvitation{
 if(!object(raw)||!bounded(raw.id,128)||!rev(raw.revision)||!['member','admin'].includes(String(raw.role))||!['pending','accepted','revoked'].includes(String(raw.state))||!timestamp(raw.createdAt)||!timestamp(raw.expiresAt)||!Array.isArray(raw.allowedLibraries)||raw.allowedLibraries.some(v=>!bounded(v,128)))invalidPeople();
 if(raw.email!==undefined)invalidPeople();if(raw.notes!==undefined&&raw.notes!==null&&(typeof raw.notes!=='string'||privatePersonNotes(raw.notes)!==raw.notes))invalidPeople();
 if(raw.serverId!==undefined){if(typeof raw.serverId!=='string')invalidPeople();try{identityCode(raw.serverId);}catch{invalidPeople();}}
 return Object.freeze({...raw,allowedLibraries:Object.freeze([...raw.allowedLibraries])}) as ServerInvitation;
}
/** Server-admin and recipient operations use distinct, generated contracts. */
export class PeopleInvitationsClient{
 readonly api:ProfileAPI;
 constructor(api:ProfileAPI){this.api=api;}
 async create(input:RouteRequest<'post_admin_access_invitations'>,signal?:AbortSignal):Promise<ServerInvitation>{
  const {notes,...rest}=input;return parseServerInvitation(await this.api.request(routePath('post_admin_access_invitations'),'POST',routeBody('post_admin_access_invitations',{...rest,notes:privatePersonNotes(notes)??undefined}),signal));
 }
 async read(id:string,signal?:AbortSignal):Promise<ServerInvitation>{return parseServerInvitation(await this.api.request(routePath('get_admin_access_invitation',{id}),'GET',undefined,signal));}
 async edit(id:string,input:RouteRequest<'put_admin_access_invitation'>,signal?:AbortSignal):Promise<ServerInvitation>{
  if(!rev(input.expectedRevision))invalidPeople();const {notes,...rest}=input;
  return parseServerInvitation(await this.api.request(routePath('put_admin_access_invitation',{id}),'PUT',routeBody('put_admin_access_invitation',{...rest,notes:privatePersonNotes(notes)??undefined}),signal));
 }
 async link(invitation:Pick<ServerInvitation,'id'|'revision'|'state'>,signal?:AbortSignal):Promise<ServerInvitationLink>{
  if(invitation.state!=='pending'||!rev(invitation.revision))throw new CodedError('invitation_expired','This invitation is no longer usable.');
  const raw=await this.api.request<unknown>(routePath('post_admin_access_invitation_link',{id:invitation.id}),'POST',routeBody('post_admin_access_invitation_link',{expectedRevision:invitation.revision}),signal);
  if(!object(raw)||!bounded(raw.code,128)||raw.code.length<32||!timestamp(raw.expiresAt))invalidPeople();
  if(raw.serverId!==undefined){if(typeof raw.serverId!=='string')invalidPeople();try{identityCode(raw.serverId);}catch{invalidPeople();}}
  return Object.freeze({code:raw.code,expiresAt:raw.expiresAt,...(raw.serverId===undefined?{}:{serverId:raw.serverId as string})});
 }
 async revoke(invitation:Pick<ServerInvitation,'id'|'revision'>,operationId:string,signal?:AbortSignal):Promise<ServerInvitation>{return parseServerInvitation(await this.api.request(routePath('post_admin_access_invitations_id_revoke',{id:invitation.id}),'POST',routeBody('post_admin_access_invitations_id_revoke',{expectedRevision:invitation.revision,operationId}),signal));}
 async preview(code:string,signal?:AbortSignal):Promise<PublicInvitation>{return parsePublicInvitation(await this.api.request(routePath('preview_invitation'),'POST',routeBody('preview_invitation',{code}),signal));}
 async acceptLocal(input:Extract<RouteRequest<'post_access_invitations_accept'>,{username:string}>,signal?:AbortSignal):Promise<Readonly<{accountId:string;username:string;profileId:string;role:string;allowedLibraries:string[]}>>{
  const raw=await this.api.request<unknown>(routePath('post_access_invitations_accept'),'POST',routeBody('post_access_invitations_accept',input),signal);
  if(!object(raw)||!bounded(raw.accountId,128)||!bounded(raw.username,64)||!bounded(raw.profileId,128)||!['admin','member'].includes(String(raw.role))||!Array.isArray(raw.allowedLibraries)||raw.allowedLibraries.some(v=>!bounded(v,128))||Object.hasOwn(raw,'notes'))invalidPeople();
  return Object.freeze({accountId:raw.accountId,username:raw.username,profileId:raw.profileId,role:raw.role as string,allowedLibraries:[...raw.allowedLibraries]});
 }
 async notes(member:Pick<PeopleMember,'accountId'|'revision'>,notes:string|null,operationId:string,signal?:AbortSignal):Promise<PeopleMember>{
  return this.api.request(routePath('put_admin_access_member_notes',{id:member.accountId}),'PUT',routeBody('put_admin_access_member_notes',{expectedRevision:member.revision,operationId,notes:privatePersonNotes(notes)??undefined}),signal);
 }
}
/** A profile is secondary to the genuine account username. Private viewers stay private. */
export function personIdentity(username:string,profileName:string,privateViewer=false):Readonly<{username?:string;profileName?:string;privateViewerKey?:'together.privateViewer'}>{
 if(privateViewer)return Object.freeze({privateViewerKey:'together.privateViewer'});
 if(!bounded(username,64))invalidPeople();
 return Object.freeze({username,...(profileName&&profileName!==username?{profileName}:{})});
}
