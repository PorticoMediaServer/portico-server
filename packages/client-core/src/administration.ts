import {unreadableServerResponse} from './server-messages.ts';
import type {Alert} from './console.ts';
/** Owner administration wire shapes: media deletion and trash, library configuration, live source configuration, DVR defaults, library channel presets and maintenance. Parsers only; no transport policy and no stored credentials. */
export type AdminEnvelope<T>=Readonly<{protocolVersion:string;serverId:string;result:T}>;
export type AdminFailure=Readonly<{code:string;message:string;retryable:boolean;fields:readonly string[]}>;
export type CostClassId='metadata-only'|'single-pass-read'|'decode'|'network';
export type CostClass=Readonly<{id:CostClassId;name:string;description:string;order:number}>;
export type LibraryAnalysisOperation=Readonly<{id:string;name:string;costClass:CostClassId;description:string;mediaKinds:readonly string[];defaultEnabled:boolean;requires:readonly string[];producesArtifacts:boolean;network:boolean}>;
export type AnalysisMatrix=Readonly<{costClasses:readonly CostClass[];operations:readonly LibraryAnalysisOperation[]}>;
export type MetadataProviderId='tmdb'|'tvdb'|'musicbrainz'|'anilist'|'local';
export type MetadataProvider=Readonly<{id:MetadataProviderId;name:string;mediaKinds:readonly string[];supportsApiKey:boolean;supportsLocale:boolean;attributionNote:string}>;
export type ProviderSelection=Readonly<{mediaKind:string;provider:MetadataProviderId;apiKeySet:boolean;language:string;region:string}>;
export type ChapterThumbnailMode='none'|'embedded'|'generated';
export type GeneratedNavigation=Readonly<{trickplayIntervalSeconds:number;trickplayTileWidth:number;trickplayMaxTiles:number;chapterThumbnailMode:ChapterThumbnailMode;videoPreviewEnabled:boolean;videoPreviewSeconds:number}>;
export type LibrarySettings=Readonly<{allowMediaDeletion:boolean;trashRetentionDays:number;providers:readonly ProviderSelection[];analysis:readonly string[];navigation:GeneratedNavigation}>;
export type LibrarySource=Readonly<{id:string;name:string;root:string;classification:'local'|'network';enabled:boolean}>;
export type LibraryAdminDocument=Readonly<{libraryId:string;libraryName:string;libraryKind:string;revision:number;digest:string;settings:LibrarySettings;analysisMatrix:AnalysisMatrix;availableProviders:readonly MetadataProvider[];enumerations:Readonly<Record<string,readonly string[]>>;sources:readonly LibrarySource[]}>;
export type DeleteFile=Readonly<{assetId:string;path:string;bytes:number;present:boolean;shared:boolean}>;
export type DeleteDependent=Readonly<{kind:string;count:number;description:string}>;
export type DeleteTarget=Readonly<{itemId:string;title:string;kind:string;libraryId:string;libraryName:string;allowMediaDeletion:boolean;trashRetentionDays:number;bytes:number;files:readonly DeleteFile[];dependents:readonly DeleteDependent[]}>;
export type DeletePreview=Readonly<{revision:number;confirmation:string;confirmationKind:'title'|'count';totalBytes:number;totalFiles:number;totalBytesText:string;blocked:readonly string[];trashRetentionDays:number;targets:readonly DeleteTarget[]}>;
export type DeleteReceipt=Readonly<{itemId:string;title:string;removed:boolean;trashEntryId:string;filesMoved:number;filesKept:number;bytes:number;code:string}>;
export type DeleteResult=Readonly<{revision:number;removed:number;failed:number;bytesTrashed:number;filesRetained:boolean;receipts:readonly DeleteReceipt[]}>;
export type TrashEntry=Readonly<{id:string;libraryId:string;itemId:string;title:string;kind:string;fileCount:number;bytes:number;bytesText:string;trashedAt:string;expiresAt:string;state:'held'|'restored'|'purged';expired:boolean;files:readonly Readonly<{originalPath:string;bytes:number}>[]}>;
export type TrashPage=Readonly<{items:readonly TrashEntry[];nextCursor:string;heldBytes:number;heldCount:number}>;
export type FilesystemRoot=Readonly<{path:string;name:string;kind:string;description:string}>;
export type FilesystemEntry=Readonly<{name:string;path:string;kind:'directory'|'file'|'unknown';readable:boolean;symlink:boolean;bytes:number}>;
export type FilesystemPage=Readonly<{path:string;parent:string;separator:string;platform:string;writable:boolean;truncated:boolean;roots:readonly FilesystemRoot[];entries:readonly FilesystemEntry[];nextCursor:string}>;
export type LiveDefaults=Readonly<{streamBufferSeconds:number;retryWindowSeconds:number;userAgent:string;guideDays:number;logoImport:boolean;discoveryEnabled:boolean}>;
export type FilterRule=Readonly<{mode:'include'|'exclude';values:readonly string[]}>;
export type LiveSourceKind='playlist'|'xmltv-guide'|'hdhomerun';
export type LiveSourceSettings=Readonly<{kind:LiveSourceKind;streamBufferSeconds:number|null;retryWindowSeconds:number|null;userAgent:string;guideSourceId:string;filters:Readonly<{categories:FilterRule;countries:FilterRule;keywords:FilterRule}>;logoImport:Readonly<{enabled:boolean;overwriteExisting:boolean}>;channelNumbering:Readonly<{mode:'source'|'sequential';startAt:number;step:number}>}>;
export type LiveSourceDocument=Readonly<{sourceId:string;sourceName:string;revision:number;digest:string;tunerCount:number;availableKinds:readonly LiveSourceKind[];settings:LiveSourceSettings;effective:LiveDefaults}>;
export type ChannelMapEntry=Readonly<{channelId:string;name:string;sourceNumber:string;number:string;group:string;guideChannelId:string;hidden:boolean;position:number;overridden:boolean}>;
export type ChannelMapPage=Readonly<{sourceId:string;generation:string;items:readonly ChannelMapEntry[];nextCursor:string}>;
export type FolderToken=Readonly<{token:string;description:string;example:string;supportsPadding:boolean}>;
export type KeepPolicy=Readonly<{mode:'keep-all'|'keep-count'|'keep-days';keepCount:number;keepDays:number;keepUntilWatched:boolean}>;
export type DVRDefaults=Readonly<{prePaddingSeconds:number;postPaddingSeconds:number;keepPolicy:KeepPolicy;conversion:Readonly<{mode:'none'|'remux'|'ladder';ladderId:string;deleteOriginal:boolean}>;folderTemplate:string;movieFolderTemplate:string;recordingProfile:string;sidecars:Readonly<{nfo:boolean;poster:boolean;fanart:boolean;thumbnail:boolean}>;tunerPreference:string}>;
export type DVRDocument=Readonly<{revision:number;digest:string;settings:DVRDefaults;folderTokens:readonly FolderToken[];enumerations:Readonly<Record<string,readonly string[]>>}>;
export type TunerAssignment=Readonly<{recordingId:string;channelId:string;title:string;state:string;startsAt:string;endsAt:string;allocationId:string}>;
export type TunerAllocation=Readonly<{sourceId:string;sourceName:string;tunerCount:number;inUse:number;conflicts:number;active:readonly TunerAssignment[];upcoming:readonly TunerAssignment[]}>;
export type TunerView=Readonly<{observedAt:string;upcomingWindowHours:number;sources:readonly TunerAllocation[]}>;
export type StorageCategory=Readonly<{id:string;name:string;description:string;directories:readonly string[];cleanable:boolean;regenerable:boolean;retentionKey:string}>;
export type StorageUsage=StorageCategory&Readonly<{bytes:number;bytesText:string;fileCount:number;paths:readonly string[];present:boolean;oldestDays:number;entries:number}>;
export type StorageReport=Readonly<{stateDirectory:string;categories:readonly StorageUsage[];totalBytes:number;totalBytesText:string;observedAt:string;truncated:boolean}>;
export type UpdateBuild=Readonly<{version:string;buildId:string;sourceDigest:string;builtAt:string;channel:string;notes:string;url:string}>;
export type UpdateStatus='current'|'update-available'|'no-feed'|'no-release'|'unavailable';
export type UpdateReport=Readonly<{current:UpdateBuild;channel:string;feedConfigured:boolean;latest:UpdateBuild|null;updateAvailable:boolean;checkedAt:string;status:UpdateStatus;message:string;autoInstall:false}>;
export type BackupKind='scheduled'|'manual'|'pre-restore';
// TODO(be backups): switch to generated types once apigen publishes the spec §4 routes.
// The shapes below are hand-written from `Spec — Backups (Plex model).md` §4 and §6.2.
export type BackupSummary=Readonly<{id:string;createdAt:string;kind:BackupKind;schemaVersion:number;serverVersion:string;bytes:number;path:string}>;
export type BackupRunning=Readonly<{jobId:string;phase:string;bytesDone:number;bytesTotal:number}>;
export type RestoreOutcome='restored'|'rolled_back';
export type LastRestore=Readonly<{at:string;outcome:RestoreOutcome;reason:string}>;
export type BackupsDocument=Readonly<{backups:readonly BackupSummary[];running?:BackupRunning;lastRestore?:LastRestore}>;
export type RestoreSource=Readonly<{backupId:string}|{path:string}>;
export type RestoreStaged=Readonly<{staged:true;restartRequired:true;validation:Readonly<{schemaVersion:number;integrity:'ok'}>}>;
/** The 4xx codes POST /v1/admin/backups/restore answers with (spec §4); `insufficient_disk` is shared with start. */
export const RESTORE_REFUSAL_CODES=Object.freeze(['restore_source_invalid','restore_schema_newer','restore_pre_release','restore_integrity_failed','restore_not_portico','insufficient_disk'] as const);
export type RestoreRefusalCode=typeof RESTORE_REFUSAL_CODES[number];
/** isRestoreRefusalCode answers whether a failure code is one of the catalogue-mapped restore refusals. */
export function isRestoreRefusalCode(code:string):code is RestoreRefusalCode{
 return (RESTORE_REFUSAL_CODES as readonly string[]).includes(code);
}
/** backupProgressFraction reads bytesDone/bytesTotal clamped to [0,1]; undefined while the total is unknown. */
export function backupProgressFraction(running:BackupRunning):number|undefined{
 if(!(running.bytesTotal>0))return undefined;
 return Math.max(0,Math.min(1,running.bytesDone/running.bytesTotal));
}
/** The console alert code for the §6.2 state-permissions warning, as be/backups raises it. */
export const STATE_PERMISSIONS_CODE='state-permissions';
export type StatePermissionsItem=Readonly<{id:string;revision:number;severity:string;status:string;firstAt:number;lastAt:number;occurrences:number}>;

class AdministrationError extends Error{
 code:string;retryable:boolean;fields:readonly string[];
 constructor(code:string,message:string,retryable=false,fields:readonly string[]=[]){super(message);this.name='AdministrationError';this.code=code;this.retryable=retryable;this.fields=Object.freeze([...fields]);}
}
export {AdministrationError};
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const filled=(v:unknown,max=512):v is string=>text(v,max)&&v.length>0;
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const flag=(v:unknown):v is boolean=>typeof v==='boolean';
function invalid(what:string):never{throw Object.assign(new AdministrationError('invalid_administration_response',unreadableServerResponse),{detail:what});}
function list(v:unknown,max:number,what:string):unknown[]{if(!Array.isArray(v)||v.length>max)invalid(what);return v;}
function strings(v:unknown,max:number,what:string):readonly string[]{const values=list(v,max,what);if(!values.every(value=>text(value,512)))invalid(what);return Object.freeze(values as string[]);}
function nullableCount(v:unknown,what:string):number|null{if(v===null)return null;if(!count(v))invalid(what);return v;}
function choice<T extends string>(v:unknown,allowed:readonly T[],what:string):T{if(!allowed.includes(v as T))invalid(what);return v as T;}
function vocabulary(v:unknown,what:string):Readonly<Record<string,readonly string[]>>{if(!object(v))invalid(what);const out:Record<string,readonly string[]>={};for(const[key,value]of Object.entries(v)){if(!filled(key,64))invalid(what);out[key]=strings(value,256,what);}return Object.freeze(out);}

/** parseFailure reads the server's error body, keeping the named fields so a form can mark them. */
export function parseFailure(v:unknown):AdminFailure{
 const body=object(v)&&object(v.error)?v.error:{};
 return Object.freeze({code:filled(body.code,128)?body.code:'request_failed',message:text(body.message)?body.message:'The administration request failed.',retryable:body.retryable===true,fields:Array.isArray(body.fields)?strings(body.fields,64,'error.fields'):Object.freeze([])});
}

/** parseEnvelope checks the shared envelope and confirms the response came from the expected server.
 * Routes published through the API registry (apikit: maintenance settings, backups, restore…)
 * answer the bare document with no envelope; that body is read as the result (§4a: a shape the
 * server really sends is never "couldn't read"). The server check applies whenever an envelope is present. */
export function parseEnvelope<T>(v:unknown,serverId:string,read:(result:unknown)=>T):AdminEnvelope<T>{
 if(object(v)&&v.protocolVersion===undefined&&v.serverId===undefined&&v.result===undefined)return Object.freeze({protocolVersion:'',serverId,result:read(v)});
 if(!object(v)||!filled(v.protocolVersion,32)||!filled(v.serverId,256)||v.result===undefined)invalid('envelope');
 if(v.serverId!==serverId)throw new AdministrationError('wrong_server','This response came from a different server.');
 return Object.freeze({protocolVersion:v.protocolVersion,serverId:v.serverId,result:read(v.result)});
}

const costClasses:readonly CostClassId[]=['metadata-only','single-pass-read','decode','network'];
function parseOperation(v:unknown):LibraryAnalysisOperation{
 if(!object(v)||!filled(v.id,64)||!filled(v.name,128)||!text(v.description,1024)||!flag(v.defaultEnabled)||!flag(v.producesArtifacts)||!flag(v.network))invalid('analysis operation');
 return Object.freeze({id:v.id,name:v.name,costClass:choice(v.costClass,costClasses,'analysis operation cost class'),description:v.description,mediaKinds:strings(v.mediaKinds,32,'analysis operation media kinds'),defaultEnabled:v.defaultEnabled,requires:v.requires===null||v.requires===undefined?Object.freeze([]):strings(v.requires,16,'analysis operation requires'),producesArtifacts:v.producesArtifacts,network:v.network});
}
/** parseAnalysisMatrix reads the operation matrix. Every operation's cost class must be one the document also publishes. */
export function parseAnalysisMatrix(v:unknown):AnalysisMatrix{
 if(!object(v))invalid('analysis matrix');
 const classes=list(v.costClasses,16,'cost classes').map(entry=>{if(!object(entry)||!filled(entry.name,128)||!text(entry.description,1024)||!count(entry.order))invalid('cost class');return Object.freeze({id:choice(entry.id,costClasses,'cost class id'),name:entry.name,description:entry.description,order:entry.order});});
 const operations=list(v.operations,128,'analysis operations').map(parseOperation);
 const known=new Set(classes.map(entry=>entry.id));
 if(!operations.every(operation=>known.has(operation.costClass)))invalid('analysis operation cost class');
 return Object.freeze({costClasses:Object.freeze(classes),operations:Object.freeze(operations)});
}

const providerIds:readonly MetadataProviderId[]=['tmdb','tvdb','musicbrainz','anilist','local'];
function parseProvider(v:unknown):MetadataProvider{
 if(!object(v)||!filled(v.name,128)||!flag(v.supportsApiKey)||!flag(v.supportsLocale)||!text(v.attributionNote,512))invalid('metadata provider');
 return Object.freeze({id:choice(v.id,providerIds,'metadata provider id'),name:v.name,mediaKinds:strings(v.mediaKinds,32,'metadata provider media kinds'),supportsApiKey:v.supportsApiKey,supportsLocale:v.supportsLocale,attributionNote:v.attributionNote});
}
/** parseMetadataProviders reads the published provider list. */
export function parseMetadataProviders(v:unknown):readonly MetadataProvider[]{
 if(!object(v))invalid('metadata providers');
 return Object.freeze(list(v.providers,32,'metadata providers').map(parseProvider));
}

function parseSelection(v:unknown):ProviderSelection{
 if(!object(v)||!filled(v.mediaKind,64)||!flag(v.apiKeySet)||!text(v.language,16)||!text(v.region,8))invalid('provider selection');
 // A key is never part of a read. A response that carries one is refused rather
 // than quietly held in client memory.
 if(v.apiKey!==undefined&&v.apiKey!=='')throw new AdministrationError('provider_key_disclosed',unreadableServerResponse);
 return Object.freeze({mediaKind:v.mediaKind,provider:choice(v.provider,providerIds,'provider selection provider'),apiKeySet:v.apiKeySet,language:v.language,region:v.region});
}
function parseNavigation(v:unknown):GeneratedNavigation{
 if(!object(v)||!count(v.trickplayIntervalSeconds)||!count(v.trickplayTileWidth)||!count(v.trickplayMaxTiles)||!flag(v.videoPreviewEnabled)||!count(v.videoPreviewSeconds))invalid('generated navigation');
 return Object.freeze({trickplayIntervalSeconds:v.trickplayIntervalSeconds,trickplayTileWidth:v.trickplayTileWidth,trickplayMaxTiles:v.trickplayMaxTiles,chapterThumbnailMode:choice(v.chapterThumbnailMode,['none','embedded','generated'] as const,'chapter thumbnail mode'),videoPreviewEnabled:v.videoPreviewEnabled,videoPreviewSeconds:v.videoPreviewSeconds});
}
function parseLibrarySettings(v:unknown):LibrarySettings{
 if(!object(v)||!flag(v.allowMediaDeletion)||!count(v.trashRetentionDays))invalid('library settings');
 return Object.freeze({allowMediaDeletion:v.allowMediaDeletion,trashRetentionDays:v.trashRetentionDays,providers:Object.freeze(list(v.providers,16,'library providers').map(parseSelection)),analysis:strings(v.analysis,128,'library analysis'),navigation:parseNavigation(v.navigation)});
}
/** parseLibraryDocument reads one library's administration document. */
export function parseLibraryDocument(v:unknown):LibraryAdminDocument{
 if(!object(v)||!filled(v.libraryId,256)||!text(v.libraryName,512)||!filled(v.libraryKind,64)||!count(v.revision)||v.revision<1||!filled(v.digest,128))invalid('library document');
 const sources=list(v.sources,256,'library sources').map(entry=>{
  if(!object(entry)||!filled(entry.id,256)||!text(entry.name,512)||!text(entry.root,8192)||!flag(entry.enabled))invalid('library source');
  return Object.freeze({id:entry.id,name:entry.name,root:entry.root,classification:choice(entry.classification,['local','network'] as const,'library source classification'),enabled:entry.enabled});
 });
 return Object.freeze({libraryId:v.libraryId,libraryName:v.libraryName,libraryKind:v.libraryKind,revision:v.revision,digest:v.digest,settings:parseLibrarySettings(v.settings),analysisMatrix:parseAnalysisMatrix(v.analysisMatrix),availableProviders:Object.freeze(list(v.availableProviders,32,'available providers').map(parseProvider)),enumerations:vocabulary(v.enumerations,'library enumerations'),sources:Object.freeze(sources)});
}

/** analysisSelectionComplete answers whether every enabled operation's dependencies are also enabled, which is what the server requires of a write. */
export function analysisSelectionComplete(matrix:AnalysisMatrix,selected:readonly string[]):readonly string[]{
 const enabled=new Set(selected);
 const missing=new Set<string>();
 for(const operation of matrix.operations)if(enabled.has(operation.id))for(const need of operation.requires)if(!enabled.has(need))missing.add(need);
 return Object.freeze([...missing].sort());
}

function parseDeleteFile(v:unknown):DeleteFile{
 if(!object(v)||!filled(v.assetId,256)||!text(v.path,8192)||!count(v.bytes)||!flag(v.present)||!flag(v.shared))invalid('delete file');
 return Object.freeze({assetId:v.assetId,path:v.path,bytes:v.bytes,present:v.present,shared:v.shared});
}
function parseTarget(v:unknown):DeleteTarget{
 if(!object(v)||!filled(v.itemId,256)||!text(v.title,1024)||!filled(v.kind,64)||!filled(v.libraryId,256)||!text(v.libraryName,512)||!flag(v.allowMediaDeletion)||!count(v.trashRetentionDays)||!count(v.bytes))invalid('delete target');
 const dependents=list(v.dependents,32,'delete dependents').map(entry=>{if(!object(entry)||!filled(entry.kind,64)||!count(entry.count)||!text(entry.description,512))invalid('delete dependent');return Object.freeze({kind:entry.kind,count:entry.count,description:entry.description});});
 return Object.freeze({itemId:v.itemId,title:v.title,kind:v.kind,libraryId:v.libraryId,libraryName:v.libraryName,allowMediaDeletion:v.allowMediaDeletion,trashRetentionDays:v.trashRetentionDays,bytes:v.bytes,files:Object.freeze(list(v.files,512,'delete files').map(parseDeleteFile)),dependents:Object.freeze(dependents)});
}
/** parseDeletePreview reads a delete preview. The confirmation it carries is the only text the delete will accept. */
export function parseDeletePreview(v:unknown):DeletePreview{
 if(!object(v)||!count(v.revision)||!text(v.confirmation,1024)||!count(v.totalBytes)||!count(v.totalFiles)||!text(v.totalBytesText,64)||!count(v.trashRetentionDays))invalid('delete preview');
 const targets=list(v.targets,200,'delete targets').map(parseTarget);
 const kind=choice(v.confirmationKind,['title','count'] as const,'confirmation kind');
 // A single-target preview confirms by title and a multi-target one by count;
 // anything else would let a client prompt for the wrong text.
 if(kind==='title'&&targets.length!==1||kind==='count'&&v.confirmation!==String(targets.length))invalid('confirmation');
 return Object.freeze({revision:v.revision,confirmation:v.confirmation,confirmationKind:kind,totalBytes:v.totalBytes,totalFiles:v.totalFiles,totalBytesText:v.totalBytesText,blocked:strings(v.blocked,200,'blocked libraries'),trashRetentionDays:v.trashRetentionDays,targets:Object.freeze(targets)});
}
/** deletionAllowed answers whether every target in a preview sits in a library that has enabled deletion. */
export function deletionAllowed(preview:DeletePreview):boolean{
 return preview.blocked.length===0&&preview.targets.every(target=>target.allowMediaDeletion);
}
/** parseDeleteResult reads a delete outcome, one receipt per target. */
export function parseDeleteResult(v:unknown):DeleteResult{
 if(!object(v)||!count(v.revision)||!count(v.removed)||!count(v.failed)||!count(v.bytesTrashed)||!flag(v.filesRetained))invalid('delete result');
 const receipts=list(v.receipts,200,'delete receipts').map(entry=>{
  if(!object(entry)||!filled(entry.itemId,256)||!text(entry.title,1024)||!flag(entry.removed)||!count(entry.filesMoved)||!count(entry.filesKept)||!count(entry.bytes))invalid('delete receipt');
  return Object.freeze({itemId:entry.itemId,title:entry.title,removed:entry.removed,trashEntryId:text(entry.trashEntryId,256)?entry.trashEntryId:'',filesMoved:entry.filesMoved,filesKept:entry.filesKept,bytes:entry.bytes,code:text(entry.code,64)?entry.code:''});
 });
 if(receipts.filter(receipt=>receipt.removed).length!==v.removed)invalid('delete result counts');
 return Object.freeze({revision:v.revision,removed:v.removed,failed:v.failed,bytesTrashed:v.bytesTrashed,filesRetained:v.filesRetained,receipts:Object.freeze(receipts)});
}
/** parseTrashPage reads one page of held deletions. */
export function parseTrashPage(v:unknown):TrashPage{
 if(!object(v)||!text(v.nextCursor,1024)||!count(v.heldBytes)||!count(v.heldCount))invalid('trash page');
 const items=list(v.items,200,'trash entries').map(entry=>{
  if(!object(entry)||!filled(entry.id,256)||!filled(entry.libraryId,256)||!filled(entry.itemId,256)||!text(entry.title,1024)||!filled(entry.kind,64)||!count(entry.fileCount)||!count(entry.bytes)||!text(entry.bytesText,64)||!text(entry.trashedAt,64)||!text(entry.expiresAt,64)||!flag(entry.expired))invalid('trash entry');
  const files=list(entry.files,512,'trash files').map(file=>{if(!object(file)||!text(file.originalPath,8192)||!count(file.bytes))invalid('trash file');return Object.freeze({originalPath:file.originalPath,bytes:file.bytes});});
  return Object.freeze({id:entry.id,libraryId:entry.libraryId,itemId:entry.itemId,title:entry.title,kind:entry.kind,fileCount:entry.fileCount,bytes:entry.bytes,bytesText:entry.bytesText,trashedAt:entry.trashedAt,expiresAt:entry.expiresAt,state:choice(entry.state,['held','restored','purged'] as const,'trash entry state'),expired:entry.expired,files:Object.freeze(files)});
 });
 return Object.freeze({items:Object.freeze(items),nextCursor:v.nextCursor,heldBytes:v.heldBytes,heldCount:v.heldCount});
}

/** parseFilesystemPage reads the folder picker's roots or one directory page. */
export function parseFilesystemPage(v:unknown):FilesystemPage{
 if(!object(v)||!text(v.path,8192)||!text(v.parent,8192)||!filled(v.separator,4)||!filled(v.platform,32)||!flag(v.writable)||!flag(v.truncated)||!text(v.nextCursor,1024))invalid('filesystem page');
 const roots=list(v.roots,64,'filesystem roots').map(entry=>{if(!object(entry)||!filled(entry.path,8192)||!filled(entry.name,256)||!filled(entry.kind,32)||!text(entry.description,512))invalid('filesystem root');return Object.freeze({path:entry.path,name:entry.name,kind:entry.kind,description:entry.description});});
 const entries=list(v.entries,200,'filesystem entries').map(entry=>{
  if(!object(entry)||!filled(entry.name,512)||!filled(entry.path,8192)||!flag(entry.readable)||!flag(entry.symlink))invalid('filesystem entry');
  return Object.freeze({name:entry.name,path:entry.path,kind:choice(entry.kind,['directory','file','unknown'] as const,'filesystem entry kind'),readable:entry.readable,symlink:entry.symlink,bytes:count(entry.bytes)?entry.bytes:0});
 });
 return Object.freeze({path:v.path,parent:v.parent,separator:v.separator,platform:v.platform,writable:v.writable,truncated:v.truncated,roots:Object.freeze(roots),entries:Object.freeze(entries),nextCursor:v.nextCursor});
}

function parseLiveDefaults(v:unknown):LiveDefaults{
 if(!object(v)||!count(v.streamBufferSeconds)||!count(v.retryWindowSeconds)||!text(v.userAgent,200)||!count(v.guideDays)||!flag(v.logoImport)||!flag(v.discoveryEnabled))invalid('live defaults');
 return Object.freeze({streamBufferSeconds:v.streamBufferSeconds,retryWindowSeconds:v.retryWindowSeconds,userAgent:v.userAgent,guideDays:v.guideDays,logoImport:v.logoImport,discoveryEnabled:v.discoveryEnabled});
}
/** parseLiveDefaultsDocument reads the server-wide live source settings. */
export function parseLiveDefaultsDocument(v:unknown):Readonly<{scope:string;revision:number;digest:string;settings:LiveDefaults}>{
 if(!object(v)||!filled(v.scope,64)||!count(v.revision)||v.revision<1||!filled(v.digest,128))invalid('live defaults document');
 return Object.freeze({scope:v.scope,revision:v.revision,digest:v.digest,settings:parseLiveDefaults(v.settings)});
}
function parseFilter(v:unknown,what:string):FilterRule{
 if(!object(v))invalid(what);
 return Object.freeze({mode:choice(v.mode,['include','exclude'] as const,what),values:strings(v.values,200,what)});
}
const liveKinds:readonly LiveSourceKind[]=['playlist','xmltv-guide','hdhomerun'];
/** parseLiveSourceDocument reads one source's configuration and the effective values behind it. */
export function parseLiveSourceDocument(v:unknown):LiveSourceDocument{
 if(!object(v)||!filled(v.sourceId,256)||!text(v.sourceName,512)||!count(v.revision)||v.revision<1||!filled(v.digest,128)||!count(v.tunerCount)||!object(v.settings))invalid('live source document');
 const s=v.settings;
 if(!object(s.filters)||!object(s.logoImport)||!object(s.channelNumbering)||!text(s.userAgent,200)||!text(s.guideSourceId,256))invalid('live source settings');
 const numbering=s.channelNumbering;
 if(!count(numbering.startAt)||!count(numbering.step))invalid('live source numbering');
 const logos=s.logoImport;
 if(!flag(logos.enabled)||!flag(logos.overwriteExisting))invalid('live source logo import');
 const kind=choice(s.kind,liveKinds,'live source kind');
 const buffer=nullableCount(s.streamBufferSeconds,'live source stream buffer');
 // A guide-only source has nothing to stream, so buffered stream tuning on one
 // would mean the client and the server disagree about what the source is.
 if(kind==='xmltv-guide'&&(buffer!==null||s.guideSourceId!==''))invalid('guide-only source');
 const settings=Object.freeze({kind,streamBufferSeconds:buffer,retryWindowSeconds:nullableCount(s.retryWindowSeconds,'live source retry window'),userAgent:s.userAgent,guideSourceId:s.guideSourceId,filters:Object.freeze({categories:parseFilter(s.filters.categories,'live source category filter'),countries:parseFilter(s.filters.countries,'live source country filter'),keywords:parseFilter(s.filters.keywords,'live source keyword filter')}),logoImport:Object.freeze({enabled:logos.enabled,overwriteExisting:logos.overwriteExisting}),channelNumbering:Object.freeze({mode:choice(numbering.mode,['source','sequential'] as const,'live source numbering mode'),startAt:numbering.startAt,step:numbering.step})});
 const kinds=strings(v.availableKinds,8,'live source kinds');
 if(!kinds.every(value=>liveKinds.includes(value as LiveSourceKind)))invalid('live source kinds');
 return Object.freeze({sourceId:v.sourceId,sourceName:v.sourceName,revision:v.revision,digest:v.digest,tunerCount:v.tunerCount,availableKinds:Object.freeze(kinds as LiveSourceKind[]),settings,effective:parseLiveDefaults(v.effective)});
}
/** parseChannelMapPage reads one page of a source's channel map. */
export function parseChannelMapPage(v:unknown):ChannelMapPage{
 if(!object(v)||!filled(v.sourceId,256)||!text(v.generation,256)||!text(v.nextCursor,1024))invalid('channel map page');
 const items=list(v.items,200,'channel map entries').map(entry=>{
  if(!object(entry)||!filled(entry.channelId,256)||!text(entry.name,512)||!text(entry.sourceNumber,16)||!text(entry.number,16)||!text(entry.group,256)||!text(entry.guideChannelId,256)||!flag(entry.hidden)||!count(entry.position)||!flag(entry.overridden))invalid('channel map entry');
  return Object.freeze({channelId:entry.channelId,name:entry.name,sourceNumber:entry.sourceNumber,number:entry.number,group:entry.group,guideChannelId:entry.guideChannelId,hidden:entry.hidden,position:entry.position,overridden:entry.overridden});
 });
 // Two channels served on the same number would make the lineup ambiguous.
 const served=items.filter(entry=>!entry.hidden&&entry.number!=='').map(entry=>entry.number);
 if(new Set(served).size!==served.length)invalid('duplicate channel number');
 return Object.freeze({sourceId:v.sourceId,generation:v.generation,items:Object.freeze(items),nextCursor:v.nextCursor});
}

/** parseDVRDocument reads the recording defaults and the folder token vocabulary. */
export function parseDVRDocument(v:unknown):DVRDocument{
 if(!object(v)||!count(v.revision)||v.revision<1||!filled(v.digest,128)||!object(v.settings))invalid('dvr document');
 const s=v.settings;
 if(!count(s.prePaddingSeconds)||!count(s.postPaddingSeconds)||!object(s.keepPolicy)||!object(s.conversion)||!object(s.sidecars)||!text(s.folderTemplate,400)||!text(s.movieFolderTemplate,400)||!filled(s.recordingProfile,64)||!filled(s.tunerPreference,64))invalid('dvr settings');
 const keep=s.keepPolicy,conversion=s.conversion,sidecars=s.sidecars;
 if(!count(keep.keepCount)||!count(keep.keepDays)||!flag(keep.keepUntilWatched)||!text(conversion.ladderId,128)||!flag(conversion.deleteOriginal)||![sidecars.nfo,sidecars.poster,sidecars.fanart,sidecars.thumbnail].every(flag))invalid('dvr policy');
 const mode=choice(conversion.mode,['none','remux','ladder'] as const,'dvr conversion mode');
 // Removing the original with no converted copy would delete the recording.
 if(mode==='none'&&conversion.deleteOriginal)invalid('dvr conversion');
 const tokens=list(v.folderTokens,64,'folder tokens').map(entry=>{if(!object(entry)||!filled(entry.token,64)||!text(entry.description,512)||!text(entry.example,128)||!flag(entry.supportsPadding))invalid('folder token');return Object.freeze({token:entry.token,description:entry.description,example:entry.example,supportsPadding:entry.supportsPadding});});
 const settings=Object.freeze({prePaddingSeconds:s.prePaddingSeconds,postPaddingSeconds:s.postPaddingSeconds,keepPolicy:Object.freeze({mode:choice(keep.mode,['keep-all','keep-count','keep-days'] as const,'dvr keep mode'),keepCount:keep.keepCount,keepDays:keep.keepDays,keepUntilWatched:keep.keepUntilWatched}),conversion:Object.freeze({mode,ladderId:conversion.ladderId,deleteOriginal:conversion.deleteOriginal}),folderTemplate:s.folderTemplate,movieFolderTemplate:s.movieFolderTemplate,recordingProfile:s.recordingProfile,sidecars:Object.freeze({nfo:sidecars.nfo as boolean,poster:sidecars.poster as boolean,fanart:sidecars.fanart as boolean,thumbnail:sidecars.thumbnail as boolean}),tunerPreference:s.tunerPreference});
 return Object.freeze({revision:v.revision,digest:v.digest,settings,folderTokens:Object.freeze(tokens),enumerations:vocabulary(v.enumerations,'dvr enumerations')});
}
/** folderTemplateTokens lists the tokens a template uses, so a client can check one against the published vocabulary before it saves. */
export function folderTemplateTokens(template:string):readonly string[]{
 const found=new Set<string>();
 for(const match of template.matchAll(/\{([A-Za-z]+)(?::0?2)?\}/g))found.add('{'+match[1]+'}');
 return Object.freeze([...found].sort());
}
/** folderTemplateProblems names what the server would refuse about a template, so a form can say so before the write. */
export function folderTemplateProblems(template:string,tokens:readonly FolderToken[]):readonly string[]{
 const problems:string[]=[];
 const known=new Map(tokens.map(token=>[token.token,token.supportsPadding]));
 if(template==='')problems.push('A template is required.');
 if(/^[\\/]/.test(template)||template.includes('..')||template.includes('//'))problems.push('A template may not escape the recordings folder.');
 for(const name of folderTemplateTokens(template))if(!known.has(name))problems.push('Unknown token '+name+'.');
 for(const match of template.matchAll(/\{([A-Za-z]+):0?2\}/g))if(known.get('{'+match[1]+'}')===false)problems.push('Token {'+match[1]+'} cannot be padded.');
 if(!/\{(title|episode|date)\}/.test(template))problems.push('A template must include {title}, {episode} or {date} so recordings do not collide.');
 return Object.freeze(problems);
}

function parseAssignment(v:unknown):TunerAssignment{
 if(!object(v)||!filled(v.recordingId,256)||!text(v.channelId,256)||!text(v.title,1024)||!filled(v.state,64)||!text(v.startsAt,64)||!text(v.endsAt,64))invalid('tuner assignment');
 return Object.freeze({recordingId:v.recordingId,channelId:v.channelId,title:v.title,state:v.state,startsAt:v.startsAt,endsAt:v.endsAt,allocationId:text(v.allocationId,256)?v.allocationId:''});
}
/** parseTunerView reads the allocation page. */
export function parseTunerView(v:unknown):TunerView{
 if(!object(v)||!text(v.observedAt,64)||!count(v.upcomingWindowHours))invalid('tuner view');
 const sources=list(v.sources,64,'tuner sources').map(entry=>{
  if(!object(entry)||!filled(entry.sourceId,256)||!text(entry.sourceName,512)||!count(entry.tunerCount)||!count(entry.inUse)||!count(entry.conflicts))invalid('tuner allocation');
  const active=list(entry.active,200,'active recordings').map(parseAssignment);
  if(active.length!==entry.inUse)invalid('tuner allocation counts');
  return Object.freeze({sourceId:entry.sourceId,sourceName:entry.sourceName,tunerCount:entry.tunerCount,inUse:entry.inUse,conflicts:entry.conflicts,active:Object.freeze(active),upcoming:Object.freeze(list(entry.upcoming,200,'upcoming recordings').map(parseAssignment))});
 });
 return Object.freeze({observedAt:v.observedAt,upcomingWindowHours:v.upcomingWindowHours,sources:Object.freeze(sources)});
}

/** parseStorageReport reads the storage care measurement. */
export function parseStorageReport(v:unknown):StorageReport{
 if(!object(v)||!text(v.stateDirectory,8192)||!count(v.totalBytes)||!text(v.totalBytesText,64)||!text(v.observedAt,64)||!flag(v.truncated))invalid('storage report');
 const categories=list(v.categories,64,'storage categories').map(entry=>{
  if(!object(entry)||!filled(entry.id,64)||!filled(entry.name,128)||!text(entry.description,1024)||!flag(entry.cleanable)||!flag(entry.regenerable)||!text(entry.retentionKey,64)||!count(entry.bytes)||!text(entry.bytesText,64)||!count(entry.fileCount)||!flag(entry.present)||!count(entry.oldestDays))invalid('storage category');
  return Object.freeze({id:entry.id,name:entry.name,description:entry.description,directories:strings(entry.directories,16,'storage directories'),cleanable:entry.cleanable,regenerable:entry.regenerable,retentionKey:entry.retentionKey,bytes:entry.bytes,bytesText:entry.bytesText,fileCount:entry.fileCount,paths:strings(entry.paths,16,'storage paths'),present:entry.present,oldestDays:entry.oldestDays,entries:count(entry.entries)?entry.entries:0});
 });
 return Object.freeze({stateDirectory:v.stateDirectory,categories:Object.freeze(categories),totalBytes:v.totalBytes,totalBytesText:v.totalBytesText,observedAt:v.observedAt,truncated:v.truncated});
}

function parseBuild(v:unknown):UpdateBuild{
 if(!object(v)||!text(v.version,64)||!text(v.buildId,128)||!text(v.sourceDigest,128)||!text(v.builtAt,64))invalid('update build');
 return Object.freeze({version:v.version,buildId:v.buildId,sourceDigest:v.sourceDigest,builtAt:v.builtAt,channel:text(v.channel,32)?v.channel:'',notes:text(v.notes,4096)?v.notes:'',url:text(v.url,2048)?v.url:''});
}
/** parseUpdateReport reads the updates page. A report claiming an automatic install is refused: this server never installs anything. */
export function parseUpdateReport(v:unknown):UpdateReport{
 if(!object(v)||!filled(v.channel,32)||!flag(v.feedConfigured)||!flag(v.updateAvailable)||!text(v.checkedAt,64)||!text(v.message,1024))invalid('update report');
 if(v.autoInstall!==false)throw new AdministrationError('unexpected_auto_install','This server reported that it installs updates on its own, which it must never do.');
 const latest=v.latest===null||v.latest===undefined?null:parseBuild(v.latest);
 const status=choice(v.status,['current','update-available','no-feed','no-release','unavailable'] as const,'update status');
 if(v.updateAvailable&&(latest===null||status!=='update-available'))invalid('update report');
 return Object.freeze({current:parseBuild(v.current),channel:v.channel,feedConfigured:v.feedConfigured,latest,updateAvailable:v.updateAvailable,checkedAt:v.checkedAt,status,message:v.message,autoInstall:false});
}

const backupKinds:readonly BackupKind[]=['scheduled','manual','pre-restore'];
const restoreOutcomes:readonly RestoreOutcome[]=['restored','rolled_back'];
/** parseBackupsDocument reads GET /v1/admin/backups (spec §4). A backup with an unknown kind is
 * skipped, and a malformed optional `running`/`lastRestore` block is dropped — either degrades
 * to a rendered list instead of failing the whole screen (§4a). */
export function parseBackupsDocument(v:unknown):BackupsDocument{
 if(!object(v))invalid('backups document');
 const backups:BackupSummary[]=[];
 for(const entry of list(v.backups,1000,'backups')){
  if(!object(entry)||!filled(entry.id,128)||!text(entry.createdAt,64))invalid('backup');
  if(!backupKinds.includes(entry.kind as BackupKind))continue;
  if(!count(entry.schemaVersion)||!text(entry.serverVersion,64)||!count(entry.bytes)||!text(entry.path,8192))invalid('backup');
  backups.push(Object.freeze({id:entry.id,createdAt:entry.createdAt,kind:entry.kind as BackupKind,schemaVersion:entry.schemaVersion,serverVersion:entry.serverVersion,bytes:entry.bytes,path:entry.path}));
 }
 const doc:{backups:readonly BackupSummary[];running?:BackupRunning;lastRestore?:LastRestore}={backups:Object.freeze(backups)};
 if(v.running!==undefined&&v.running!==null){
  const r=v.running;
  if(object(r)&&filled(r.jobId,128)&&text(r.phase,128)&&count(r.bytesDone)&&count(r.bytesTotal))doc.running=Object.freeze({jobId:r.jobId,phase:r.phase,bytesDone:r.bytesDone,bytesTotal:r.bytesTotal});
 }
 if(v.lastRestore!==undefined&&v.lastRestore!==null){
  const r=v.lastRestore;
  if(object(r)&&text(r.at,64)&&restoreOutcomes.includes(r.outcome as RestoreOutcome))doc.lastRestore=Object.freeze({at:r.at,outcome:r.outcome as RestoreOutcome,reason:text(r.reason,1024)?r.reason as string:''});
 }
 return Object.freeze(doc);
}

/** parseRestoreStaged reads the POST /v1/admin/backups/restore receipt. A mutation receipt is
 * strict: anything but `{staged: true, restartRequired: true, validation: {schemaVersion, integrity: "ok"}}`
 * is refused rather than treated as a restore. */
export function parseRestoreStaged(v:unknown):RestoreStaged{
 if(!object(v)||v.staged!==true||v.restartRequired!==true||!object(v.validation)||!count(v.validation.schemaVersion)||v.validation.integrity!=='ok')invalid('restore receipt');
 return Object.freeze({staged:true,restartRequired:true,validation:Object.freeze({schemaVersion:v.validation.schemaVersion as number,integrity:'ok' as const})});
}

/** parseStatePermissionsItem reads the §6.2 warning out of the health/alerts contract. */
export function parseStatePermissionsItem(v:unknown):StatePermissionsItem{
 if(!object(v)||v.code!==STATE_PERMISSIONS_CODE||!filled(v.id,160)||!count(v.revision)||(v.revision as number)<1||!text(v.severity,32)||!filled(v.status,32)||!count(v.firstAt)||!count(v.lastAt)||!count(v.occurrences))invalid('state permissions item');
 return Object.freeze({id:v.id as string,revision:v.revision as number,severity:v.severity as string,status:v.status as string,firstAt:v.firstAt as number,lastAt:v.lastAt as number,occurrences:v.occurrences as number});
}

/** isStatePermissionsAlert answers whether an alert is a live (unresolved) permissions warning. */
export function isStatePermissionsAlert(alert:Pick<Alert,'code'|'status'>):boolean{
 return alert.code===STATE_PERMISSIONS_CODE&&alert.status!=='resolved';
}

/**
 * The providers one selection can choose from: the ones that serve its media kind. "Local files
 * only" is the library's Metadata source choice, so it is listed here only while it is the stored value.
 */
export function providerChoices(available:readonly MetadataProvider[],selection:Readonly<{mediaKind:string;provider:string}>):readonly MetadataProvider[]{
 return available.filter(a=>a.mediaKinds.includes(selection.mediaKind)&&(a.id!=='local'||selection.provider==='local'));
}
