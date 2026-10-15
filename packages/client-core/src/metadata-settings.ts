import {routePath} from "../../contracts/src/routes.ts";
import {unreadableServerResponse} from './server-messages.ts';
/** The OpenSubtitles fields say whether the owner saved their own key and password (never sent back) and the account name. */
export type MetadataSettings=Readonly<{revision:number;localArtworkLimitBytes:number;localArtworkUsedBytes:number;includeAdult:boolean;tmdbCredentialConfigured:boolean;tvdbCredentialConfigured:boolean;openSubtitlesKeyConfigured:boolean;openSubtitlesUsername:string;openSubtitlesPasswordConfigured:boolean}>;
/** A credential left out is unchanged; an empty one goes back to the server's own (for the username: no account). */
export type MetadataSettingsChange=Readonly<{expectedRevision:number;localArtworkLimitBytes:number;includeAdult:boolean;tmdbReadToken?:string;tvdbProjectKey?:string;openSubtitlesApiKey?:string;openSubtitlesUsername?:string;openSubtitlesPassword?:string}>;
export function parseMetadataSettings(raw:unknown):MetadataSettings{
 if(typeof raw!=='object'||raw===null||Array.isArray(raw))throw new Error(unreadableServerResponse);
 const v=raw as Record<string,unknown>;
 if(typeof v.revision!=='number'||!Number.isSafeInteger(v.revision)||v.revision<1||typeof v.localArtworkLimitBytes!=='number'||!Number.isSafeInteger(v.localArtworkLimitBytes)||v.localArtworkLimitBytes<1||typeof v.includeAdult!=='boolean'||typeof v.tmdbCredentialConfigured!=='boolean'||typeof v.tvdbCredentialConfigured!=='boolean')throw new Error(unreadableServerResponse);
 if(typeof v.localArtworkUsedBytes!=="number"||!Number.isSafeInteger(v.localArtworkUsedBytes)||v.localArtworkUsedBytes<0)throw new Error(unreadableServerResponse);
 return Object.freeze({localArtworkUsedBytes:v.localArtworkUsedBytes,revision:v.revision,localArtworkLimitBytes:v.localArtworkLimitBytes,includeAdult:v.includeAdult,tmdbCredentialConfigured:v.tmdbCredentialConfigured,tvdbCredentialConfigured:v.tvdbCredentialConfigured,
  openSubtitlesKeyConfigured:v.openSubtitlesKeyConfigured===true,openSubtitlesUsername:typeof v.openSubtitlesUsername==='string'&&v.openSubtitlesUsername.length<=512?v.openSubtitlesUsername:'',openSubtitlesPasswordConfigured:v.openSubtitlesPasswordConfigured===true});
}
type Request=<T>(path:string,method?:string,body?:unknown)=>Promise<T>;
export function metadataSettingsService(request:Request){const path=routePath("get_metadata_settings");return Object.freeze({async read(){return parseMetadataSettings(await request<unknown>(path));},async save(change:MetadataSettingsChange){return parseMetadataSettings(await request<unknown>(path,'PUT',change));}});}
const GIBIBYTE=2**30;
/** The stored artwork limit as its field shows it, in gigabytes: a whole number where it is one, otherwise up to two places. */
export function artworkLimitText(bytes:number):string{const gb=bytes/GIBIBYTE;return Number.isInteger(gb)?String(gb):gb.toFixed(2).replace(/0+$/,'').replace(/\.$/,'');}
/** What a typed limit stores, or nothing when it is not a size the server takes (0.1 GB to a million). A comma is read as the decimal mark. */
export function artworkLimitBytes(text:string):number|undefined{const typed=text.trim().replace(',','.');if(!/^\d+(\.\d+)?$/.test(typed))return undefined;const gb=Number(typed);return gb>=0.1&&gb<=1_000_000?Math.round(gb*GIBIBYTE):undefined;}
