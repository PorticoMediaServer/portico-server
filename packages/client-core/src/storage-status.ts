import {routePath, encodedRoutePath} from "../../contracts/src/routes.ts";
import {unreadableServerResponse} from './server-messages.ts';
export type UpgradeCopy=Readonly<{id:string;createdAt:string;bytes:number;path:string}>;
export type StorageStatus=Readonly<{databasePath:string;cacheDirectory:string;backupDirectory:string;reclaimableBytes:number;lastReclaimedAt?:string;lastReclaimedBytes:number;upgradeCopies:readonly UpgradeCopy[]}>;
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=8192):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f]/.test(v);
const count=(v:unknown):v is number=>Number.isSafeInteger(v)&&Number(v)>=0;
function invalid():never{throw new Error(unreadableServerResponse);}
export function parseStorageStatus(v:unknown):StorageStatus{
 if(!obj(v)||!text(v.databasePath)||!text(v.cacheDirectory)||!text(v.backupDirectory)||!count(v.reclaimableBytes)||!count(v.lastReclaimedBytes)||!Array.isArray(v.upgradeCopies)||v.upgradeCopies.length>256)invalid();
 const copies=v.upgradeCopies.map(e=>{if(!obj(e)||!text(e.id,128)||!/^before-\d{8}T\d{6}\.\d{9}Z\.sqlite$/.test(e.id)||!text(e.createdAt,64)||!Number.isFinite(Date.parse(e.createdAt))||!text(e.path)||!count(e.bytes))invalid();return Object.freeze({id:e.id,createdAt:e.createdAt,path:e.path,bytes:e.bytes});});
 return Object.freeze({databasePath:v.databasePath,cacheDirectory:v.cacheDirectory,backupDirectory:v.backupDirectory,reclaimableBytes:v.reclaimableBytes,lastReclaimedBytes:v.lastReclaimedBytes,...(text(v.lastReclaimedAt,64)&&v.lastReclaimedAt?{lastReclaimedAt:v.lastReclaimedAt}:{}),upgradeCopies:Object.freeze(copies)});
}
export type FFmpegValidation=Readonly<{path:string;state:'accepted'|'refused'|'unchecked';inUse:boolean;reasonCode?:string;missingFeatures:readonly string[];checkedAt:string}>;
export function parseFFmpegValidation(v:unknown):FFmpegValidation{
 if(!obj(v)||!text(v.path)||!['accepted','refused','unchecked'].includes(String(v.state))||typeof v.inUse!=='boolean'||!text(v.checkedAt,64)||!Array.isArray(v.missingFeatures)||v.missingFeatures.length>128||!v.missingFeatures.every(x=>text(x,128))||v.reasonCode!==undefined&&!text(v.reasonCode,128))invalid();
 return Object.freeze({path:v.path,state:v.state as FFmpegValidation['state'],inUse:v.inUse,checkedAt:v.checkedAt,missingFeatures:Object.freeze(v.missingFeatures as string[]),...(typeof v.reasonCode==='string'?{reasonCode:v.reasonCode}:{})});
}
export function storageStatusService(request:<T>(path:string,method?:string,body?:unknown)=>Promise<T>){return Object.freeze({
 async read(){return parseStorageStatus(await request<unknown>(routePath("get_storage_status")));},
 async removeUpgradeCopy(copy:UpgradeCopy){await request<unknown>(encodedRoutePath("delete_upgrade_copy", {"id": encodeURIComponent(copy.id)}),'DELETE');},
});}
