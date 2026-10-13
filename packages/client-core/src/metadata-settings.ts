import {unreadableServerResponse} from './server-messages.ts';
export type MetadataSettings=Readonly<{revision:number;localArtworkLimitBytes:number;includeAdult:boolean;tmdbCredentialConfigured:boolean;tvdbCredentialConfigured:boolean}>;
export type MetadataSettingsChange=Readonly<{expectedRevision:number;localArtworkLimitBytes:number;includeAdult:boolean;tmdbReadToken?:string;tvdbProjectKey?:string}>;
export function parseMetadataSettings(raw:unknown):MetadataSettings{
 if(typeof raw!=='object'||raw===null||Array.isArray(raw))throw new Error(unreadableServerResponse);
 const v=raw as Record<string,unknown>;
 if(typeof v.revision!=='number'||!Number.isSafeInteger(v.revision)||v.revision<1||typeof v.localArtworkLimitBytes!=='number'||!Number.isSafeInteger(v.localArtworkLimitBytes)||v.localArtworkLimitBytes<1||typeof v.includeAdult!=='boolean'||typeof v.tmdbCredentialConfigured!=='boolean'||typeof v.tvdbCredentialConfigured!=='boolean')throw new Error(unreadableServerResponse);
 return Object.freeze({revision:v.revision,localArtworkLimitBytes:v.localArtworkLimitBytes,includeAdult:v.includeAdult,tmdbCredentialConfigured:v.tmdbCredentialConfigured,tvdbCredentialConfigured:v.tvdbCredentialConfigured});
}
type Request=<T>(path:string,method?:string,body?:unknown)=>Promise<T>;
export function metadataSettingsService(request:Request){const path='/v1/admin/metadata/settings';return Object.freeze({async read(){return parseMetadataSettings(await request<unknown>(path));},async save(change:MetadataSettingsChange){return parseMetadataSettings(await request<unknown>(path,'PUT',change));}});}
