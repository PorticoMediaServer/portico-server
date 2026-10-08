import {unreadableServerResponse} from './server-messages.ts';
import {serviceProblem, serviceText} from './presentation/service-text.ts';
import type {MessageId} from '../../i18n/src/index.ts';
/** DVR intent protocol. This module never constructs a stream URL or starts playback.
 * Each instance belongs to one authenticated server/viewer lifetime. */
import type {ChannelApi, GuideChannel, GuideProgramme} from './channel-guide';
export type DVRView = 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage';
export type RecordingState = 'scheduled' | 'conflicted' | 'waiting-source' | 'waiting-guide' | 'preparing' | 'recording' | 'finalizing' | 'completed' | 'incomplete-playable' | 'failed' | 'cancelled' | 'pending-delete' | 'deleted';
export type RecordingOptions = {beforeSeconds: number; afterSeconds: number; priority: number; retentionDays: number; episodeLimit: number};
export type Occurrence = {sourceId: string; channelId: string; generation: string; programmeId: string};
export type Recording = {id: string; revision: number; occurrence: Occurrence; programme: GuideProgramme; ruleId: string; options: RecordingOptions; start: string; end: string; state: RecordingState; reason: string; keep: boolean; itemId: string; libraryId: string; coverageStart: string; coverageEnd: string; bytes: number; channel?: {id: string; name: string; number: string}; watched?: boolean|null;
 /** FEAT-08: the show this recording belongs to (rows group by it); absent for one-offs. */
 seriesId?: string; seriesTitle?: string; conflicts: {start: string; end: string; demand: number; capacity: number}[]};
export type RuleConfig = {name: string; sourceId: string; seriesId: string; enabled: boolean; episodes: 'all' | 'new'; allowedChannels: string[]; blockedChannels: string[]; keywords: string[]; blockedKeywords: string[]; options: RecordingOptions};
export type RecordingRule = {id: string; revision: number; config: RuleConfig; reconcileState: string; diagnostic: string; updatedAt: string};
export type DVRPage = {recordings: Recording[]; rules: RecordingRule[]; nextCursor: string; revision: number; captureAvailable: boolean; captureUnavailableReason: string; deletionAvailable: boolean; usage: {bytes: number; pendingDeleteBytes: number; recordings: number}};
export type Capacity = {known: boolean; effective: number; planningEstimate?: number; mode?: string};
export type RecordingDetail = {recording: Recording; capacity: Capacity; overlaps: {id: string; title: string; start: string; end: string; priority: number; state: string}[]; nextOverlap: string};
export type RulePreview = {matches: number; unknownNewEvidence: number; sample: GuideProgramme[]; generation: string; capacity: Capacity};
export type DeletePreview = {recordingId: string; revision: number; bytes: number; keep: boolean; activeReaders: boolean; result: string};
export type StoragePolicy = {revision: number; retentionDays: number; episodeLimit: number; floorBytes: number; capBytes: number};
export type RecordingStorage = {policy: StoragePolicy; measurement: {freeBytes: number; usedBytes: number; reservedBytes: number; writeHealthy: boolean; measuredAt: string}; pendingDeleteBytes: number; forecastBytes: number; forecastHours: number; forecastDescription: string; captureAvailable: boolean; warning: string};
export type ChannelChoice = {id: string; name: string; number: string};
export type ChannelChoices = {channels: ChannelChoice[]; generation: string; nextCursor: string};
export type RecordingDraft = {channel: GuideChannel; programme: GuideProgramme; series: boolean};
export const defaultRecordingOptions: RecordingOptions = {beforeSeconds: 0, afterSeconds: 0, priority: 0, retentionDays: 0, episodeLimit: 0};
export const dvrViews: readonly DVRView[] = ['upcoming', 'recorded', 'rules', 'history', 'storage'];
export const viewTitle: Record<DVRView, string> = {upcoming: 'Upcoming', recorded: 'Recorded', rules: 'Series rules', history: 'History', storage: 'Storage'};
const states: readonly string[] = ['scheduled','conflicted','waiting-source','waiting-guide','preparing','recording','finalizing','completed','incomplete-playable','failed','cancelled','pending-delete','deleted'];
const object = (v: unknown): v is Record<string, any> => !!v && typeof v === 'object' && !Array.isArray(v);
const text = (v: unknown, max = 4096): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const number = (v: unknown, min = 0, max = Number.MAX_SAFE_INTEGER): v is number => Number.isSafeInteger(v) && Number(v) >= min && Number(v) <= max;
const id = (v: unknown, length = 64): v is string => typeof v === 'string' && new RegExp(`^[a-f0-9]{${length}}$`).test(v);
const instant = (v: unknown): v is string => typeof v === 'string' && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(v) && Number.isFinite(Date.parse(v)) && new Date(v).toISOString().slice(0,19) === v.slice(0,19);
function invalid(): never { throw new Error(unreadableServerResponse); }
export function validRecordingOptions(v: unknown): v is RecordingOptions {return object(v) && number(v.beforeSeconds,0,21600) && number(v.afterSeconds,0,21600) && number(v.priority,-1000,1000) && number(v.retentionDays,0,3650) && number(v.episodeLimit,0,100000);}
function programme(v: unknown): GuideProgramme {
 if (!object(v) || !id(v.id) || !id(v.channelId) || !text(v.title,512) || !instant(v.start) || !instant(v.end) || Date.parse(v.start)>=Date.parse(v.end) || !['provider-id','interval-only','generated'].includes(v.lineage)) invalid();
 const seriesId=v.seriesId??'',episodeId=v.episodeId??'',description=v.description??'',newEvidence=v.newEvidence||'unknown';
 if(!text(seriesId,512)||!text(episodeId,512)||!text(description,8192)||!['new','repeat','unknown'].includes(newEvidence))invalid();
 return {id:v.id,channelId:v.channelId,title:v.title,start:v.start,end:v.end,lineage:v.lineage,seriesId,episodeId,description,newEvidence} as GuideProgramme;
}
export function parseRecording(v: unknown): Recording {
 if(!object(v)||!id(v.id)||!number(v.revision,1)||!object(v.occurrence)||!id(v.occurrence.sourceId,48)||!id(v.occurrence.channelId)||!id(v.occurrence.generation,48)||!id(v.occurrence.programmeId)||!validRecordingOptions(v.options)||!instant(v.start)||!instant(v.end)||!states.includes(v.state)||!text(v.reason,256)||typeof v.keep!=='boolean'||!text(v.ruleId,64)||v.ruleId!==''&&!id(v.ruleId)||!text(v.itemId,64)||!text(v.libraryId,64)||v.itemId!==''&&(!id(v.itemId)||!id(v.libraryId))||!number(v.bytes)||!Array.isArray(v.conflicts)||v.conflicts.length>100000)invalid();
 if(!(v.coverageStart===''||instant(v.coverageStart))||!(v.coverageEnd===''||instant(v.coverageEnd)))invalid();
 for(const c of v.conflicts)if(!object(c)||!instant(c.start)||!instant(c.end)||Date.parse(c.start)>=Date.parse(c.end)||!number(c.demand,1)||!number(c.capacity))invalid();
 const p=programme(v.programme);if(p.id!==v.occurrence.programmeId||p.channelId!==v.occurrence.channelId)invalid();
 if(Date.parse(v.start)>=Date.parse(v.end)||!!v.coverageStart!==!!v.coverageEnd||v.coverageStart&&Date.parse(v.coverageStart)>Date.parse(v.coverageEnd))invalid();
 // Part 2.2: channel and watched are optional (older servers omit them); render only when present.
 let channel: Recording['channel'];
 if(v.channel!==undefined){
  const c=v.channel;
  if(!object(c)||!id(c.id)||!text(c.name,256)||!text(c.number,32))invalid();
  channel={id:c.id,name:c.name,number:c.number};
 }
 let watched: Recording['watched'];
 if(v.watched!==undefined){
  if(v.watched!==null&&typeof v.watched!=='boolean')invalid();
  watched=v.watched;
 }
 // FEAT-08: optional grouping key and show name; an unreadable value is left out, never fatal.
 const series={...(text(v.seriesId,512)&&v.seriesId?{seriesId:v.seriesId}:{}),...(text(v.seriesTitle,512)&&v.seriesTitle?{seriesTitle:v.seriesTitle}:{})};
 const options:RecordingOptions={beforeSeconds:v.options.beforeSeconds,afterSeconds:v.options.afterSeconds,priority:v.options.priority,retentionDays:v.options.retentionDays,episodeLimit:v.options.episodeLimit};
 return {id:v.id,revision:v.revision,occurrence:{sourceId:v.occurrence.sourceId,channelId:v.occurrence.channelId,generation:v.occurrence.generation,programmeId:v.occurrence.programmeId},programme:p,ruleId:v.ruleId,options,start:v.start,end:v.end,state:v.state,reason:v.reason,keep:v.keep,itemId:v.itemId,libraryId:v.libraryId,coverageStart:v.coverageStart,coverageEnd:v.coverageEnd,bytes:v.bytes,...(channel===undefined?{}:{channel}),...(watched===undefined?{}:{watched}),conflicts:v.conflicts.map((c:any)=>({start:c.start,end:c.end,demand:c.demand,capacity:c.capacity})),...series} as Recording;
}
/** Part 2.2: unwatched counts only watched===false; null (unpublished) counts as neither. */
export function countUnwatched(recordings: readonly Pick<Recording,'watched'>[]): number {
  return recordings.filter(r=>r.watched===false).length;
}
function config(v:unknown):RuleConfig{
 if(!object(v)||!text(v.name,160)||!v.name||!id(v.sourceId,48)||!text(v.seriesId,512)||!v.seriesId||typeof v.enabled!=='boolean'||!['all','new'].includes(v.episodes)||!validRecordingOptions(v.options))invalid();
 for(const k of ['allowedChannels','blockedChannels'])if(!Array.isArray(v[k])||v[k].length>2000||v[k].some((x:unknown)=>!id(x)))invalid();
 for(const k of ['keywords','blockedKeywords'])if(!Array.isArray(v[k])||v[k].length>20||v[k].some((x:unknown)=>!text(x,128)||!x.trim()))invalid();
 return {name:v.name,sourceId:v.sourceId,seriesId:v.seriesId,enabled:v.enabled,episodes:v.episodes,allowedChannels:[...v.allowedChannels],blockedChannels:[...v.blockedChannels],keywords:[...v.keywords],blockedKeywords:[...v.blockedKeywords],options:{beforeSeconds:v.options.beforeSeconds,afterSeconds:v.options.afterSeconds,priority:v.options.priority,retentionDays:v.options.retentionDays,episodeLimit:v.options.episodeLimit}};
}
function rule(v:unknown):RecordingRule{if(!object(v)||!id(v.id)||!number(v.revision,1)||!text(v.reconcileState,64)||!text(v.diagnostic,256)||!instant(v.updatedAt))invalid();return {id:v.id,revision:v.revision,config:config(v.config),reconcileState:v.reconcileState,diagnostic:v.diagnostic,updatedAt:v.updatedAt} as RecordingRule;}
export function parseDVRPage(v: unknown): DVRPage {
 if(!object(v)||!Array.isArray(v.recordings)||v.recordings.length>50||!Array.isArray(v.rules)||v.rules.length>50||!text(v.nextCursor,4096)||!number(v.revision)||typeof v.captureAvailable!=='boolean'||typeof v.deletionAvailable!=='boolean'||!text(v.captureUnavailableReason,256)||!object(v.usage)||!number(v.usage.bytes)||!number(v.usage.pendingDeleteBytes)||!number(v.usage.recordings))invalid();
 const recordings=v.recordings.map(parseRecording),rules=v.rules.map(rule);
 if(new Set(recordings.map((r:Recording)=>r.id)).size!==recordings.length||new Set(rules.map((r:RecordingRule)=>r.id)).size!==rules.length)invalid();
 return {recordings,rules,nextCursor:v.nextCursor,revision:v.revision,captureAvailable:v.captureAvailable,captureUnavailableReason:v.captureUnavailableReason,deletionAvailable:v.deletionAvailable,usage:{bytes:v.usage.bytes,pendingDeleteBytes:v.usage.pendingDeleteBytes,recordings:v.usage.recordings}} as DVRPage;
}
export function dvrMessage(error: unknown): string {
 const status=object(error)?error.status:0;
 if(status===401||status===403)return serviceText('dvr.error.access');
 if(status===409)return serviceText('dvr.error.changed');
 if(status===422)return serviceText('dvr.error.unsupported');
 if(status===400)return serviceText('dvr.error.invalid');
 return serviceProblem(error,'live','dvr.error.unconfirmed');
}
export function isAccessError(error: unknown): boolean {return object(error)&&(error.status===401||error.status===403);}
const stateIds:Record<string,MessageId>={scheduled:'dvr.state.scheduled',conflicted:'dvr.state.conflicted','waiting-source':'dvr.state.waitingSource','waiting-guide':'dvr.state.waitingGuide',preparing:'dvr.state.preparing',recording:'dvr.state.recording',finalizing:'dvr.state.finalizing',completed:'dvr.state.completed','incomplete-playable':'dvr.state.incompletePlayable',incomplete:'dvr.state.incomplete',failed:'dvr.state.failed',cancelled:'dvr.state.cancelled','pending-delete':'dvr.state.pendingDelete',deleted:'dvr.state.deleted'};
const reasonIds:Record<string,MessageId>={'capture-window-missed':'dvr.reason.missed','tuner-conflict':'dvr.reason.conflict','storage-floor':'dvr.reason.storageFloor','storage-cap':'dvr.reason.storageCap','owner-cancelled':'dvr.reason.cancelled','source-disabled':'dvr.reason.sourceDisabled','source-changed':'dvr.reason.sourceChanged','guide-occurrence-missing':'dvr.reason.guideMissing','partial-capture':'dvr.reason.partial','physical-reader-active':'dvr.reason.inUse','active-reader-grace':'dvr.reason.graceWait','artifact-remove-unavailable':'dvr.reason.removeRetry','recovered-partial':'dvr.reason.recoveredPartial'};
/** A recording's state in the viewer's language (catalogue `dvr.state.*`). */
export function recordingStateLabel(state: string): string {const id=stateIds[state];return id?serviceText(id):state.replace(/-/g,' ').replace(/^./,c=>c.toUpperCase());}
/** Why a recording is in its state (catalogue `dvr.reason.*`); empty when there's no reason. */
export function recordingReason(reason:string):string {
 if(!reason)return '';const id=reasonIds[reason];return id?serviceText(id):reason.replaceAll('-',' ');
}
export const mutableRecording=(r:Recording)=>['scheduled','conflicted','waiting-source','waiting-guide'].includes(r.state);
export const activeRecording=(r:Recording)=>mutableRecording(r)||['preparing','recording','finalizing'].includes(r.state);
export const playableRecording=(r:Recording)=>!!r.itemId&&!!r.libraryId&&['completed','incomplete-playable'].includes(r.state);
export function occurrence(d:RecordingDraft):Occurrence{return {sourceId:d.channel.sourceId,channelId:d.channel.id,generation:d.channel.generation,programmeId:d.programme.id};}
export function draftRule(d:RecordingDraft):RuleConfig{return {name:d.programme.title,sourceId:d.channel.sourceId,seriesId:d.programme.seriesId,enabled:true,episodes:'all',allowedChannels:[],blockedChannels:[],keywords:[],blockedKeywords:[],options:{...defaultRecordingOptions}};}
export type SeriesRuleUi = {episodes: 'all'|'new'; keep: number; beforeSeconds: number; afterSeconds: number; anyChannel: boolean};
/**
 * FEAT-02: rule-editor state → RuleConfig (plus the anchor). Both clients' record-series dialogs
 * build through this so the guide sheet and the rule editor offer the same bounded choices.
 * Out-of-range values fail via config(), never reach the server.
 */
export function seriesRuleConfig(d:RecordingDraft,ui:SeriesRuleUi):{config:RuleConfig;anchor:Occurrence}{
 if(ui.episodes!=='all'&&ui.episodes!=='new')invalid();
 const draft=draftRule({...d,series:true});
 draft.episodes=ui.episodes;
 draft.allowedChannels=ui.anyChannel?[]:[d.channel.id];
 draft.options={...defaultRecordingOptions,beforeSeconds:ui.beforeSeconds,afterSeconds:ui.afterSeconds,episodeLimit:ui.keep};
 return {config:config(draft),anchor:occurrence(d)};
}
export function bytesLabel(bytes:number):string{if(!Number.isFinite(bytes)||bytes<0)return 'Unknown';const units=['B','KiB','MiB','GiB','TiB'];let n=0;while(bytes>=1024&&n<4){bytes/=1024;n++}return `${bytes.toLocaleString(undefined,{maximumFractionDigits:n?1:0})} ${units[n]}`;}
export function recordingTime(value:string):string{return value?new Date(value).toLocaleString(undefined,{month:'short',day:'numeric',hour:'numeric',minute:'2-digit',timeZoneName:'short'}):serviceText('dvr.notCaptured');}

export class DVRClient {
 private disposed=false;private accessEpoch=0;private accessChanged:(()=>void)|undefined;private requests=new Set<AbortController>();private receipts=new Map<string,Promise<string>>();
 private api:ChannelApi;private serverId:string;private newRequestId:()=>Promise<string>;private timeoutMs:number;
 constructor(api:ChannelApi,serverId:string,newRequestId:()=>Promise<string>,timeoutMs=18000){this.api=api;this.serverId=serverId;this.newRequestId=newRequestId;this.timeoutMs=timeoutMs;}
 // React effect replay may reuse the memoized instance, but not its requests.
 activate(){this.dispose();this.disposed=false;this.accessEpoch++;}
 onAccessChanged(listener:()=>void){this.accessChanged=listener;}
 dispose(){this.accessChanged=undefined;this.disposed=true;for(const request of this.requests)request.abort();this.requests.clear();this.receipts.clear();}
 private async read<T>(path:string,method='GET',body?:unknown):Promise<T>{
  if(this.disposed)throw new Error('Recording scope closed.');const epoch=this.accessEpoch;const abort=new AbortController();this.requests.add(abort);let timer:ReturnType<typeof setTimeout>|undefined;
  try {const raw=await Promise.race([this.api.request<unknown>(path,method,body,abort.signal),new Promise<never>((_,reject)=>{timer=setTimeout(()=>{abort.abort();reject(new Error('Recording request timed out.'));},this.timeoutMs)})]);if(this.disposed||abort.signal.aborted||epoch!==this.accessEpoch)throw new Error('Recording scope closed.');if(!object(raw)||raw.protocolVersion!=='1.0'||raw.serverId!==this.serverId||!('result' in raw))invalid();return raw.result as T;}
  catch(error){if(!this.disposed&&epoch===this.accessEpoch&&isAccessError(error)){this.accessEpoch++;for(const request of this.requests)request.abort();this.accessChanged?.();}throw error;}
  finally{clearTimeout(timer);this.requests.delete(abort);}
 }
 private async mutate<T>(path:string,method:string,body:Record<string,unknown>):Promise<T>{
  const epoch=this.accessEpoch,key=JSON.stringify([path,method,body]);let request=this.receipts.get(key);
  if(!request){request=this.newRequestId().then(value=>{if(!id(value,48))invalid();return value});this.receipts.set(key,request);request.catch(()=>{if(this.receipts.get(key)===request)this.receipts.delete(key)});}
  // Keep unknown-outcome receipts for this viewer lifetime. Never auto-rebase a
  // stale revision or replace a request id after a timeout.
  const requestId=await request;
  if(epoch!==this.accessEpoch)throw new Error('Recording scope closed.');
  return this.read<T>(path,method,{...body,requestId});
 }
 async list(view:Exclude<DVRView,'storage'>,cursor=''){return parseDVRPage(await this.read('/v1/dvr?'+new URLSearchParams({state:view,limit:'50',...(cursor?{cursor}:{})})));}
 async detail(recordingId:string,after=''):Promise<RecordingDetail>{if(!id(recordingId)||after&&!id(after))invalid();const v=await this.read<RecordingDetail>('/v1/dvr/recordings/'+recordingId+(after?'?after='+after:''));if(!object(v)||!validCapacity(v.capacity)||!Array.isArray(v.overlaps)||v.overlaps.length>20||!text(v.nextOverlap,64)||v.nextOverlap&&!id(v.nextOverlap))invalid();const r=parseRecording(v.recording);if(r.id!==recordingId)invalid();for(const o of v.overlaps)if(!object(o)||!id(o.id)||!text(o.title,512)||!instant(o.start)||!instant(o.end)||Date.parse(o.start)>=Date.parse(o.end)||!number(o.priority,-1000,1000)||!states.includes(o.state))invalid();if(new Set(v.overlaps.map(o=>o.id)).size!==v.overlaps.length)invalid();return {recording:r,capacity:capacity(v.capacity),overlaps:v.overlaps.map(o=>({id:o.id,title:o.title,start:o.start,end:o.end,priority:o.priority,state:o.state})),nextOverlap:v.nextOverlap};}
 async schedule(d:RecordingDraft,options:RecordingOptions){if(!validRecordingOptions(options))invalid();return parseRecording(await this.mutate('/v1/dvr/recordings','POST',{occurrence:occurrence(d),options}));}
 async update(r:Recording,options:RecordingOptions){if(!validRecordingOptions(options))invalid();return parseRecording(await this.mutate('/v1/dvr/recordings/'+r.id,'PATCH',{expectedRevision:r.revision,options}));}
 async cancel(r:Recording){return parseRecording(await this.mutate('/v1/dvr/recordings/'+r.id+'/cancel','POST',{expectedRevision:r.revision}));}
 async keep(r:Recording){return parseRecording(await this.mutate('/v1/dvr/recordings/'+r.id+'/keep','PUT',{expectedRevision:r.revision,keep:!r.keep}));}
 async deletePreview(r:Recording):Promise<DeletePreview>{const v=await this.read<DeletePreview>('/v1/dvr/recordings/'+r.id+'/delete-preview');if(!object(v)||v.recordingId!==r.id||!number(v.revision,1)||!number(v.bytes)||typeof v.keep!=='boolean'||typeof v.activeReaders!=='boolean'||!text(v.result,1024))invalid();return {recordingId:v.recordingId,revision:v.revision,bytes:v.bytes,keep:v.keep,activeReaders:v.activeReaders,result:v.result};}
 async delete(r:Recording,preview:DeletePreview){if(preview.recordingId!==r.id)invalid();return parseRecording(await this.mutate('/v1/dvr/recordings/'+r.id,'DELETE',{expectedRevision:preview.revision}));}
 async newRuleID(){const [a,b]=await Promise.all([this.newRequestId(),this.newRequestId()]);if(!id(a,48)||!id(b,48))invalid();return (a+b).slice(0,64);}
 async saveRule(ruleId:string,revision:number,c:RuleConfig,anchor?:Occurrence){if(!id(ruleId)||!number(revision))invalid();config(c);return rule(await this.mutate('/v1/dvr/rules/'+ruleId,'PUT',{id:ruleId,expectedRevision:revision,config:c,...(anchor?{anchor}:{})}));}
 async deleteRule(r:RecordingRule,future:'keep'|'cancel'){const v=await this.mutate<{deleted:boolean}>('/v1/dvr/rules/'+r.id,'DELETE',{expectedRevision:r.revision,future});if(v.deleted!==true)invalid();}
 async preview(c:RuleConfig):Promise<RulePreview>{config(c);const v=await this.read<RulePreview>('/v1/dvr/rules/preview','POST',c);if(!object(v)||!number(v.matches)||!number(v.unknownNewEvidence)||!id(v.generation,48)||!Array.isArray(v.sample)||v.sample.length>12||!validCapacity(v.capacity))invalid();return {matches:v.matches,unknownNewEvidence:v.unknownNewEvidence,sample:v.sample.map(programme),generation:v.generation,capacity:capacity(v.capacity)};}
 async channels(sourceId:string,search='',cursor=''):Promise<ChannelChoices>{if(!id(sourceId,48))invalid();const v=await this.read<ChannelChoices>('/v1/dvr/channels?'+new URLSearchParams({sourceId,search,cursor}));if(!object(v)||!id(v.generation,48)||!text(v.nextCursor,128)||!Array.isArray(v.channels)||v.channels.length>30)invalid();for(const c of v.channels)if(!object(c)||!id(c.id)||!text(c.name,256)||!text(c.number,32))invalid();return {channels:v.channels.map(c=>({id:c.id,name:c.name,number:c.number})),generation:v.generation,nextCursor:v.nextCursor};}
 async storage():Promise<RecordingStorage>{const v=await this.read<RecordingStorage>('/v1/dvr/storage');if(!object(v)||!validStoragePolicy(v.policy)||!object(v.measurement)||!number(v.measurement.usedBytes)||!number(v.measurement.freeBytes)||!number(v.measurement.reservedBytes)||typeof v.measurement.writeHealthy!=='boolean'||!(v.measurement.measuredAt===''||instant(v.measurement.measuredAt))||!number(v.pendingDeleteBytes)||!number(v.forecastBytes)||!number(v.forecastHours,1)||!text(v.forecastDescription,2048)||typeof v.captureAvailable!=='boolean'||!text(v.warning,256))invalid();return {policy:storagePolicy(v.policy),measurement:{freeBytes:v.measurement.freeBytes,usedBytes:v.measurement.usedBytes,reservedBytes:v.measurement.reservedBytes,writeHealthy:v.measurement.writeHealthy,measuredAt:v.measurement.measuredAt},pendingDeleteBytes:v.pendingDeleteBytes,forecastBytes:v.forecastBytes,forecastHours:v.forecastHours,forecastDescription:v.forecastDescription,captureAvailable:v.captureAvailable,warning:v.warning};}
 async saveStorage(p:StoragePolicy){if(!validStoragePolicy(p))invalid();const {revision,...policy}=p;const v=await this.mutate<StoragePolicy>('/v1/dvr/storage','PUT',{expectedRevision:revision,policy});if(!validStoragePolicy(v))invalid();return storagePolicy(v);}
}
function validCapacity(v:unknown):v is Capacity{return object(v)&&typeof v.known==='boolean'&&number(v.effective)&&(!v.known||v.effective>0)&&(v.planningEstimate===undefined||number(v.planningEstimate))&&(v.mode===undefined||text(v.mode,64));}
function capacity(v:Capacity):Capacity{return {known:v.known,effective:v.effective,...(v.planningEstimate===undefined?{}:{planningEstimate:v.planningEstimate}),...(v.mode===undefined?{}:{mode:v.mode})};}
function storagePolicy(v:StoragePolicy):StoragePolicy{return {revision:v.revision,retentionDays:v.retentionDays,episodeLimit:v.episodeLimit,floorBytes:v.floorBytes,capBytes:v.capBytes};}
export function validStoragePolicy(v:unknown):v is StoragePolicy{return object(v)&&number(v.revision,1)&&number(v.retentionDays,0,3650)&&number(v.episodeLimit,0,100000)&&number(v.floorBytes)&&number(v.capBytes);}

export type DVRQuery = {view:DVRView;cursor?:string;recordingId?:string};
export type DVRSnapshot = Readonly<{query:DVRQuery;page:DVRPage|null;storage:RecordingStorage|null;detail:RecordingDetail|null;loading:boolean;mutating:boolean;accessDenied:boolean;error:string;revision:number}>;
/** Observable query state, not a navigator. All destinations are supplied by the
 * application's existing LibraryNavigationService. Late reads cannot revive a
 * previous route or viewer; authorization errors clear private presentation. */
export class DVRModel {
 private listeners=new Set<()=>void>();private epoch=0;private overlapRequest=0;private mutation=0;private closed=false;private activation=0;
 private snapshot:DVRSnapshot={query:{view:'upcoming'},page:null,storage:null,detail:null,loading:false,mutating:false,accessDenied:false,error:'',revision:0};
 readonly client:DVRClient;constructor(client:DVRClient){this.client=client;this.observeAccess();}
 private observeAccess(){this.client.onAccessChanged(()=>{this.epoch++;this.publish({page:null,detail:null,storage:null,loading:false,mutating:false,accessDenied:true,error:dvrMessage({status:403})});});}
 activate(){const lease=++this.activation;this.epoch++;this.closed=false;this.client.activate();this.observeAccess();this.publish({page:null,detail:null,storage:null,loading:false,mutating:false,accessDenied:false,error:''});return lease;}
 deactivate(lease:number){if(lease===this.activation)this.dispose();}
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>this.listeners.delete(fn)};
 getSnapshot=()=>this.snapshot;
 private publish(p:Partial<DVRSnapshot>){if(this.closed)return;this.snapshot=Object.freeze({...this.snapshot,...p,revision:this.snapshot.revision+1});for(const fn of this.listeners)fn();}
 dispose(){this.closed=true;this.epoch++;this.client.dispose();this.snapshot=Object.freeze({...this.snapshot,page:null,storage:null,detail:null,loading:false,mutating:false,error:'',revision:this.snapshot.revision+1});this.listeners.clear();}
 async load(query:DVRQuery=this.snapshot.query){
  const epoch=++this.epoch,same=JSON.stringify(query)===JSON.stringify(this.snapshot.query);
  this.publish({query,loading:true,error:'',...(!same?{page:null,storage:null,detail:null}:{})});
  try{if(query.recordingId){const detail=await this.client.detail(query.recordingId);if(epoch===this.epoch)this.publish({detail,loading:false,accessDenied:false});}
   else if(query.view==='storage'){const storage=await this.client.storage();if(epoch===this.epoch)this.publish({storage,loading:false,accessDenied:false});}
   else{const page=await this.client.list(query.view,query.cursor);if(epoch===this.epoch)this.publish({page,loading:false,accessDenied:false});}}
  catch(e){if(epoch===this.epoch)this.publish({loading:false,error:dvrMessage(e),...(isAccessError(e)?{page:null,storage:null,detail:null,accessDenied:true}:{})});}
 }
 async act(work:()=>Promise<unknown>):Promise<boolean>{if(this.snapshot.mutating||this.closed)return false;const epoch=this.epoch,mutation=++this.mutation;this.publish({mutating:true,error:''});try{await work();if(this.closed||epoch!==this.epoch)return false;await this.load();return true;}catch(e){if(!this.closed&&epoch===this.epoch)this.publish({error:dvrMessage(e),...(isAccessError(e)?{page:null,storage:null,detail:null,accessDenied:true}:{})});return false;}finally{if(mutation===this.mutation)this.publish({mutating:false});}}
 async moreOverlaps(){
  const d=this.snapshot.detail,epoch=this.epoch,request=++this.overlapRequest;if(!d?.nextOverlap)return;
  try{const next=await this.client.detail(d.recording.id,d.nextOverlap);
   if(epoch!==this.epoch||request!==this.overlapRequest)return;
   if(next.recording.revision!==d.recording.revision){await this.load();return;}
   const overlaps=[...new Map([...d.overlaps,...next.overlaps].map(o=>[o.id,o])).values()];
   this.publish({detail:{...next,overlaps}});
  }catch(e){if(epoch===this.epoch&&request===this.overlapRequest)this.publish({error:dvrMessage(e),...(isAccessError(e)?{page:null,storage:null,detail:null,accessDenied:true}:{})});}
 }
}
