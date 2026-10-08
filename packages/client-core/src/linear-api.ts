import {unreadableServerResponse} from './server-messages.ts';
/** Versioned, renderer-independent linear feature DTOs. Credentials are write-only. */
import type {ChannelApi} from './channel-guide';
export const LINEAR_PROTOCOL='1.0' as const;
export type SourceKind='m3u'|'xtream'|'hdhomerun';
export type SourceAccess='owner-only'|'server-members';
export type ChannelMapping=Readonly<{channelKey:string;guideKey:string}>;
export type RemoteSourceDraft={id:string;expectedRevision:number;name:string;kind:SourceKind;locator:string;guideUrl:string;username:string;password:string;confirmedLanRoots:string[];mappings:ChannelMapping[];refreshSeconds:number;ownerLimit:number;useDiscoveredCapacity:boolean;viewerAccess:SourceAccess};
export type RemoteSourcePreview=Readonly<{previewId:string;preview:{channels:number;programmes:number;availableStart:string;availableEnd:string;names:string[]};discoveredCapacity:number;capacityMode:'defaulted'|'discovered'|'configured';mappingKeys:readonly {channelKey:string;name:string;guideKey:string}[];expiresAt:string}>;
export type RefreshStatus=Readonly<{sourceId:string;state:'healthy'|'degraded'|'credentials-required'|'refreshing';errorCode:string;nextRefresh:string;failures:number}>;
export function remoteSourceDraft(id:string):RemoteSourceDraft{return{id,expectedRevision:0,name:'',kind:'m3u',locator:'',guideUrl:'',username:'',password:'',confirmedLanRoots:[],mappings:[],refreshSeconds:21600,ownerLimit:0,useDiscoveredCapacity:false,viewerAccess:'owner-only'}}
export const isRecord=(v:unknown):v is Record<string,unknown>=>v!==null&&typeof v==='object'&&!Array.isArray(v);
export const boundedText=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
export const safeCounter=(v:unknown,max=Number.MAX_SAFE_INTEGER):v is number=>Number.isSafeInteger(v)&&Number(v)>=0&&Number(v)<=max;
export const utcInstant=(v:unknown):v is string=>typeof v==='string'&&/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$/.test(v)&&Number.isFinite(Date.parse(v));
export function envelope(v:unknown,serverId:string):Record<string,unknown>{if(!isRecord(v)||v.serverId!==serverId)throw new Error(unreadableServerResponse);return v}
function bad():never{throw new Error(unreadableServerResponse)}
export function parseRemoteSourcePreview(raw:unknown,serverId:string):RemoteSourcePreview{
 const v=envelope(raw,serverId).remotePreview;if(!isRecord(v)||!boundedText(v.previewId,48)||v.previewId.length!==48||!isRecord(v.preview)||!safeCounter(v.preview.channels,2000)||v.preview.channels<1||!safeCounter(v.preview.programmes,100000)||!boundedText(v.preview.availableStart,64)||!boundedText(v.preview.availableEnd,64)||!Array.isArray(v.preview.names)||v.preview.names.length!==v.preview.channels||v.preview.names.some(x=>!boundedText(x,256))||!safeCounter(v.discoveredCapacity,256)||!['defaulted','discovered','configured'].includes(String(v.capacityMode))||!utcInstant(v.expiresAt)||!Array.isArray(v.mappingKeys)||v.mappingKeys.length!==v.preview.channels)bad();
 const keys=new Set<string>();for(const k of v.mappingKeys){if(!isRecord(k)||!boundedText(k.channelKey,256)||!k.channelKey||keys.has(k.channelKey)||!boundedText(k.name,256)||!boundedText(k.guideKey,256))bad();keys.add(k.channelKey)}
 return v as unknown as RemoteSourcePreview;
}
export async function previewRemoteSource(api:ChannelApi,serverId:string,draft:RemoteSourceDraft,signal?:AbortSignal){return parseRemoteSourcePreview(await api.request('/v1/admin/live-sources/remote/preview','POST',draft,signal),serverId)}
export function linearError(error:unknown,fallback='The request did not finish. Try again.'):string{
 if(!isRecord(error))return fallback;const code=typeof error.code==='string'?error.code:isRecord(error.error)?error.error.code:undefined;
 if(error.status===401||error.status===403)return 'Your access changed. Reopen this server to continue.';
 switch(code){case 'invalid_channel_input':return 'The playlist or guide could not be used. Check the addresses and try again.';case 'source_lan_confirmation_required':return 'This source is on your local network. Confirm the addresses below to let Portico reach them, then it checks again.';case 'source_has_dependencies':return 'This source is still referenced by recordings, rules or active playback. Disable it until those dependencies are resolved.';case 'source_network_policy':return 'This address was blocked by the source network policy. Confirm the exact LAN device root only when you trust it.';case 'source_credentials_required':return 'The provider rejected these credentials. Re-enter them and preview again.';case 'source_unavailable':return 'The source could not be reached or returned an unsupported response. Your last saved guide is preserved.';case 'channel_revision_conflict':return 'This configuration changed or the preview expired. Reload and preview again.';case 'source_capacity_unavailable':return 'The confirmed tuners are occupied or reserved for recordings.';default:return fallback}
}

/** CD-06: the local-network roots a preview asks the owner to confirm, when that is why it failed. */
export function lanConfirmationRoots(error:unknown):readonly string[]|undefined{
 if(!isRecord(error))return undefined;const code=typeof error.code==='string'?error.code:undefined;
 if(code!=='source_lan_confirmation_required')return undefined;
 const roots=(error as {confirmRoots?:unknown}).confirmRoots;return Array.isArray(roots)&&roots.length&&roots.every(r=>typeof r==='string')?roots as string[]:undefined;
}
/** The draft to preview again once the owner confirmed `roots`: every root confirmed so far, once each. */
export function withConfirmedLanRoots(draft:RemoteSourceDraft,roots:readonly string[]):RemoteSourceDraft{
 return {...draft,confirmedLanRoots:[...new Set([...draft.confirmedLanRoots,...roots])]};
}
