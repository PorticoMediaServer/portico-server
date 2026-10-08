import {unreadableServerResponse} from './server-messages.ts';
/** Device records, remembered browser accounts and the tvOS Top Shelf feed.
 *
 * An installation id is generated once per install and kept. It is not a
 * fingerprint and it grants nothing, but it is the only thing standing between a
 * stranger and this installation's remembered-account list, so it must be a real
 * secret: `newInstallationId` produces 256 bits of randomness, and every parser
 * here refuses a shorter one rather than accepting a guessable value.
 *
 * The tokens themselves stay in the client. The server keeps only the account
 * descriptors and which one was used last. */
export type ApprovalState='approved'|'pending'|'denied';
export type Device=Readonly<{id:string;installationId:string;name:string;platform:string;app:string;appVersion:string;ip:string;firstSeen:string;lastSeen:string;trusted:boolean;approvalState:ApprovalState;lastProfileId:string;rememberAccount:boolean;current:boolean;sessions:number}>;
export type RememberedAccount=Readonly<{accountId:string;username:string;displayName:string;avatarVersion:number;automaticSignIn:boolean;lastUsed:string}>;
export type TopShelfEntry=Readonly<{id:string;title:string;subtitle:string;imageUrl:string;displayUrl:string;playUrl:string;progressPercent:number}>;
export type TopShelfSection=Readonly<{id:string;title:string;shape:'poster'|'landscape'|'square';entries:readonly TopShelfEntry[]}>;
export type TopShelfFeed=Readonly<{serverId:string;profileId:string;expiresAt:string;sections:readonly TopShelfSection[]}>;

const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=256):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>typeof v==='string'&&v.length>0&&v.length<=128&&/^[A-Za-z0-9_-]+$/.test(v);
const bool=(v:unknown):v is boolean=>typeof v==='boolean';
const count=(v:unknown,min=0,max=1e6):v is number=>typeof v==='number'&&Number.isInteger(v)&&v>=min&&v<=max;
const instant=(v:unknown):v is string=>typeof v==='string'&&v.length<=40&&!Number.isNaN(Date.parse(v));
const only=(v:Record<string,unknown>,keys:readonly string[]):void=>{if(Object.keys(v).some(k=>!keys.includes(k)))fail('unexpected field');};
function fail(reason:string):never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_device_document',detail:reason,retryable:false});}

/** An installation id must be long enough to be unguessable, because the
 * remembered-account list is readable by anyone who can present one. */
export const validInstallationId=(v:unknown):v is string=>typeof v==='string'&&v.length>=32&&v.length<=128&&/^[A-Za-z0-9_-]+$/.test(v);

/** newInstallationId generates one. Callers persist it for the life of the
 * install; regenerating it makes the device look new to the owner's approval
 * policy and orphans the remembered-account list. */
export function newInstallationId():string{
 const raw=new Uint8Array(32);
 crypto.getRandomValues(raw);
 let text='';
 for(const byte of raw)text+='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'[byte&63];
 return text;
}

const approvalStates=['approved','pending','denied'] as const;
const deviceKeys=['id','authority','installationId','name','platform','app','appVersion','ip','firstSeen','lastSeen','trusted','approvalState','lastProfileId','rememberAccount','current','sessions'] as const;

export function parseDevice(raw:unknown):Device{
 if(!obj(raw))fail('device');only(raw,deviceKeys);
 // Every device record now names the authority that signed it in; servers from before that omit it.
 if(raw.authority!==undefined&&raw.authority!=='local'&&raw.authority!=='hosted')fail('authority');
 if(!id(raw.id)||!validInstallationId(raw.installationId))fail('device identity');
 if(!text(raw.name,120)||raw.name==='')fail('name');
 for(const field of ['platform','app','appVersion'])if(!text(raw[field],64))fail(field);
 // The server records the peer address or nothing; it never echoes a header the
 // caller supplied, so anything that is not an address is a defect.
 if(typeof raw.ip!=='string'||raw.ip.length>45||/[^0-9a-fA-F.:]/.test(raw.ip))fail('ip');
 if(!instant(raw.firstSeen)||!instant(raw.lastSeen))fail('timestamps');
 if(!bool(raw.trusted)||!bool(raw.rememberAccount)||!bool(raw.current))fail('device flags');
 if(typeof raw.approvalState!=='string'||!approvalStates.includes(raw.approvalState as ApprovalState))fail('approvalState');
 if(raw.lastProfileId!==undefined&&raw.lastProfileId!==''&&!id(raw.lastProfileId))fail('lastProfileId');
 if(!count(raw.sessions,0,1000))fail('sessions');
 return Object.freeze({id:raw.id as string,installationId:raw.installationId as string,name:raw.name as string,platform:raw.platform as string,app:raw.app as string,appVersion:raw.appVersion as string,ip:raw.ip,firstSeen:raw.firstSeen as string,lastSeen:raw.lastSeen as string,trusted:raw.trusted as boolean,approvalState:raw.approvalState as ApprovalState,lastProfileId:(raw.lastProfileId as string|undefined)??'',rememberAccount:raw.rememberAccount as boolean,current:raw.current as boolean,sessions:raw.sessions as number});
}

/** Devices arrive most recently active first. At most one may be `current`: two
 * would mean the list cannot tell the person which device they are holding. */
export function parseDevices(raw:unknown):readonly Device[]{
 if(!obj(raw)||!Array.isArray(raw.items)||raw.items.length>200)fail('device list');
 only(raw,['items']);
 const devices=raw.items.map(parseDevice);
 if(new Set(devices.map(d=>d.id)).size!==devices.length)fail('duplicate device ids');
 if(devices.filter(d=>d.current).length>1)fail('more than one current device');
 return Object.freeze(devices);
}

/** A device waiting on the owner cannot obtain a session, so a client must show
 * the wait rather than retrying sign-in. */
export const deviceAwaitingApproval=(device:Device):boolean=>device.approvalState==='pending';

export function parseRememberedAccounts(raw:unknown):readonly RememberedAccount[]{
 if(!obj(raw)||!Array.isArray(raw.items)||raw.items.length>10)fail('remembered accounts');
 only(raw,['items']);
 const accounts=raw.items.map(entry=>{
  if(!obj(entry))fail('remembered account');only(entry,['accountId','username','displayName','avatarVersion','avatarUrl','automaticSignIn','lastUsed']);
  if(!id(entry.accountId)||!text(entry.username,64)||entry.username===''||!text(entry.displayName,120))fail('remembered account fields');
  if(!count(entry.avatarVersion,0)||!bool(entry.automaticSignIn)||!instant(entry.lastUsed))fail('remembered account state');
  return Object.freeze({accountId:entry.accountId as string,username:entry.username as string,displayName:entry.displayName as string,avatarVersion:entry.avatarVersion as number,automaticSignIn:entry.automaticSignIn as boolean,lastUsed:entry.lastUsed as string});
 });
 if(new Set(accounts.map(a=>a.accountId)).size!==accounts.length)fail('duplicate remembered accounts');
 // Exactly one account per installation may sign in automatically; two would
 // make which one signs in a race.
 if(accounts.filter(a=>a.automaticSignIn).length>1)fail('more than one automatic sign-in');
 return Object.freeze(accounts);
}

const shapes=['poster','landscape','square'] as const;

/** The Top Shelf feed is read by the tvOS extension with a token valid for 24
 * hours. Artwork URLs carry that token, so they expire with the feed; a client
 * must refetch rather than caching an image URL past `expiresAt`. */
export function parseTopShelfFeed(raw:unknown):TopShelfFeed{
 if(!obj(raw))fail('feed');only(raw,['serverId','profileId','expiresAt','sections']);
 if(!id(raw.serverId)||!id(raw.profileId)||!instant(raw.expiresAt))fail('feed identity');
 if(!Array.isArray(raw.sections)||raw.sections.length>4)fail('sections');
 const sections=raw.sections.map(section=>{
  if(!obj(section))fail('section');only(section,['id','title','shape','entries']);
  if(!id(section.id)||!text(section.title,120))fail('section fields');
  if(typeof section.shape!=='string'||!shapes.includes(section.shape as TopShelfSection['shape']))fail('shape');
  if(!Array.isArray(section.entries)||section.entries.length>10)fail('entries');
  const entries=section.entries.map(entry=>{
   if(!obj(entry))fail('entry');only(entry,['id','title','subtitle','imageUrl','displayUrl','playUrl','progressPercent']);
   if(!id(entry.id)||!text(entry.title,256))fail('entry fields');
   if(entry.subtitle!==undefined&&!text(entry.subtitle,256))fail('subtitle');
   // Image URLs are server-relative; deep links use the app scheme. Neither may
   // be an arbitrary URL, or a feed row could send a viewer anywhere.
   if(entry.imageUrl!==undefined&&entry.imageUrl!==''&&(typeof entry.imageUrl!=='string'||!entry.imageUrl.startsWith('/v1/topshelf/art/')||entry.imageUrl.length>1024))fail('imageUrl');
   for(const field of ['displayUrl','playUrl']){
    const value=entry[field];
    if(field==='displayUrl'?typeof value!=='string':value!==undefined&&value!==''){
     if(typeof value!=='string')fail(field);
    }
    if(typeof value==='string'&&value!==''&&(!value.startsWith('portico://')||value.length>512))fail(field);
   }
   if(entry.progressPercent!==undefined&&!count(entry.progressPercent,1,99))fail('progressPercent');
   return Object.freeze({id:entry.id as string,title:entry.title as string,subtitle:(entry.subtitle as string|undefined)??'',imageUrl:(entry.imageUrl as string|undefined)??'',displayUrl:entry.displayUrl as string,playUrl:(entry.playUrl as string|undefined)??'',progressPercent:(entry.progressPercent as number|undefined)??0});
  });
  if(new Set(entries.map(e=>e.id)).size!==entries.length)fail('duplicate entries in a section');
  return Object.freeze({id:section.id as string,title:section.title as string,shape:section.shape as TopShelfSection['shape'],entries:Object.freeze(entries)});
 });
 if(new Set(sections.map(s=>s.id)).size!==sections.length)fail('duplicate sections');
 return Object.freeze({serverId:raw.serverId as string,profileId:raw.profileId as string,expiresAt:raw.expiresAt as string,sections:Object.freeze(sections)});
}

export const topShelfExpired=(feed:TopShelfFeed,now=Date.now()):boolean=>Date.parse(feed.expiresAt)<=now;
