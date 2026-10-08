/** Selected-server operations wire contract. No Hosted transport is accepted. */
import {parsePreferenceSnapshot,type DeviceClass,type PreferencePatch,type PreferenceScope} from './preferences.ts';
import {parseTelemetryReading,parseTelemetrySample,parseTranscodeCapacity,type TelemetryWindow} from './telemetry.ts';
import {parseAttention} from './attention.ts';
import {parsePlaybackHistory,type PlaybackHistoryPeriod} from './playback-history.ts';
export type ConsoleAPI={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>;requestConsole?<T>(path:string,method:string,body:unknown,signal:AbortSignal):Promise<T>};
export type ConsoleScope={serverId:string;viewerId:string};
export type Page<T>={items:T[];nextCursor:string};
export type HardwareBackend='auto'|'software'|'videotoolbox'|'vaapi'|'qsv'|'nvenc'|'amf';
export type ToneMappingAlgorithm='clip'|'linear'|'gamma'|'reinhard'|'hable'|'mobius';
export type X264Preset='ultrafast'|'superfast'|'veryfast'|'faster'|'fast'|'medium'|'slow';
export type PlanningPolicy='maximum_fidelity'|'maximum_compatibility'|'minimize_server_work';
export const HARDWARE_BACKENDS:HardwareBackend[]=['auto','software','videotoolbox','vaapi','qsv','nvenc','amf'];
export const TONE_MAPPING_ALGORITHMS:ToneMappingAlgorithm[]=['clip','linear','gamma','reinhard','hable','mobius'];
export const X264_PRESETS:X264Preset[]=['ultrafast','superfast','veryfast','faster','fast','medium','slow'];
export const PLANNING_POLICIES:PlanningPolicy[]=['maximum_fidelity','maximum_compatibility','minimize_server_work'];
/** Session caps: 0 means unlimited, which the server states rather than infers. */
export type TranscodingSettings={
 hardwareBackend:HardwareBackend;hardwareDevice:string;hdrToneMapping:boolean;hdrToneMappingAlgorithm:ToneMappingAlgorithm;
 x264Preset:X264Preset;directStreamRemux:boolean;planningPolicy:PlanningPolicy;
 throttleBufferSeconds:number;playedRetentionSeconds:number;temporaryDirectory:string;
 maxConcurrentSessions:number;maxHardwareSessions:number;maxSoftwareSessions:number;maxBackgroundSessions:number};
export type RuntimeSettings=TranscodingSettings&{name:string;transcodingEnabled:boolean;perAccountCap:number|null;serverCap:number|null;diagnosticDays:number;notificationDays:number;jobDays:number;/** How long the play history is kept; 0 keeps everything. Absent on a server without the setting. */playHistoryDays?:number;'home.communityActivityEnabled'?:boolean};
export type SettingsDocument={revision:number;digest:string;requested:RuntimeSettings;effective:RuntimeSettings;activeRevision:number;restartFields:string[];registryRevision:string};
export type Diagnostic={platform:'web'|'ios'|'tvos';state:'idle'|'starting'|'playing'|'paused'|'buffering'|'failed'|'unknown';online:boolean};
export type FeedbackStatus='open'|'in-progress'|'resolved'|'closed';
export type FeedbackReport={id:string;createdAt:number;updatedAt:number;revision:number;status:FeedbackStatus;category:string;message:string;itemId?:string;diagnostic?:Diagnostic;contextAvailable:boolean;events?:{sequence:number;at:number;revision:number;status:FeedbackStatus;reply:string;actorClass:'owner'|'admin'|'viewer'}[]};
export type SubmitFeedback={category:'playback'|'metadata'|'subtitles'|'other';message:string;itemId:string;diagnostic?:Diagnostic};
export type Notice={id:string;createdAt:number;readAt:number|null;code:string;targetId?:string;message:string};
export type Inbox=Page<Notice>&{unread:number};
export type Job={id:string;kind:string;resource:string;trigger:string;state:'queued'|'running'|'paused'|'reconciling'|'cancellation-requested'|'cancelled'|'failed'|'succeeded';phase:string;revision:number;attempt:number;createdAt:number;updatedAt:number;nextAt:number;domainId?:string;errorCode?:string;predecessor?:string;settingsRevision:number;lane:string;actions:('cancel'|'retry'|'pause'|'resume')[];processed?:number};
export type Schedule={id:string;revision:number;kind:string;resource:string;enabled:boolean;timezone:string;startMinute:number;windowMinutes:number;catchUp:boolean;lastSlot:string;createdAt:number};
export type StreamOptions={libraries:{id:string;label:string}[];viewers:{id:string;label:string}[]};
export type JobKind={kind:string;lane:string;resourceRequired:boolean};
export type Alert={id:string;code:string;severity:string;status:string;firstAt:number;lastAt:number;occurrences:number;revision:number};
export type Measurement={name:string;observedAt:number;freshUntil:number;facts:Record<string,{state:string;value:unknown;unit?:string;reason?:string}>};
export type EvidenceRecord={sequence:number;at:number;lane?:string;severity?:string;component?:string;code?:string;action?:string;target?:string;hash?:string;previousHash?:string;fields?:Record<string,number>};
export type SupportExport={id:string;expiresAt:number;manifest:{version:string;from:number;to:number;included:string[];excluded:string[];redaction:string;maxBytes:number};snapshot?:unknown;runtime?:Page<EvidenceRecord>;client?:Page<EvidenceRecord>;audit?:Page<EvidenceRecord>};
const obj=(v:unknown):v is Record<string,any>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const integer=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const text=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max;
const id=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9_.-]{1,160}$/.test(v);
function invalid():never{throw new Error('Invalid selected-server console response.');}
function base(v:unknown):Record<string,any>{if(!obj(v))invalid();return v;}
function revision(v:Record<string,any>){if(!integer(v.revision)||v.revision<1)invalid();}
export function parseSettings(v:unknown):SettingsDocument{
 const d=base(v);revision(d);if(!/^[a-f0-9]{64}$/.test(d.digest)||!integer(d.activeRevision)||!Array.isArray(d.restartFields))invalid();
 for(const key of ['requested','effective']){const s=base(d[key]);if(!text(s.name,100)||typeof s.transcodingEnabled!=='boolean')invalid();for(const key of ['serverCap','perAccountCap'])if(s[key]!==null&&(!integer(s[key])||s[key]<1||s[key]>1000000))invalid(); for(const key of ['diagnosticDays','notificationDays','jobDays'])if(!integer(s[key])||s[key]<1||s[key]>(key==='notificationDays'?180:30))invalid();if(s.playHistoryDays!==undefined&&(!integer(s.playHistoryDays)||s.playHistoryDays<0||s.playHistoryDays>36500))invalid();checkTranscoding(s);if(s['home.communityActivityEnabled']!==undefined&&typeof s['home.communityActivityEnabled']!=='boolean')invalid();}
 return d as SettingsDocument;
}
/** Transcoding values are rejected outside the bounds the registry publishes,
 * so a client cannot render a choice this server would refuse to save. */
function checkTranscoding(s:Record<string,any>){
 if(!HARDWARE_BACKENDS.includes(s.hardwareBackend)||!TONE_MAPPING_ALGORITHMS.includes(s.hdrToneMappingAlgorithm)||
  !X264_PRESETS.includes(s.x264Preset)||!PLANNING_POLICIES.includes(s.planningPolicy))invalid();
 if(typeof s.hdrToneMapping!=='boolean'||typeof s.directStreamRemux!=='boolean')invalid();
 if(!text(s.hardwareDevice,200)||!text(s.temporaryDirectory,1024))invalid();
 if(!integer(s.throttleBufferSeconds)||s.throttleBufferSeconds<10||s.throttleBufferSeconds>600)invalid();
 if(!integer(s.playedRetentionSeconds)||s.playedRetentionSeconds>3600)invalid();
 for(const key of ['maxConcurrentSessions','maxHardwareSessions','maxSoftwareSessions','maxBackgroundSessions'])
  if(!integer(s[key])||s[key]>1000)invalid();
}
export function parseReport(v:unknown):FeedbackReport{const r=base(v);revision(r);if(!id(r.id)||!integer(r.createdAt)||!integer(r.updatedAt)||!['open','in-progress','resolved','closed'].includes(r.status)||!id(r.category)||!text(r.message,4000)||typeof r.contextAvailable!=='boolean'||r.itemId!==undefined&&!id(r.itemId))invalid();if(r.events!==undefined){if(!Array.isArray(r.events)||r.events.length>200)invalid();for(const e of r.events)if(!obj(e)||!integer(e.at)||!text(e.reply,4000)||!['owner','admin','viewer'].includes(e.actorClass))invalid();}return r as FeedbackReport;}
export function parseEvidenceRecord(value:unknown):EvidenceRecord {
 const record=base(value);
 if(!integer(record.sequence)||!integer(record.at)||record.at>8640000000000000)invalid();
 for(const key of ['lane','severity','component','code','action','target','hash','previousHash'])if(record[key]!==undefined&&!text(record[key],4096))invalid();
 if(record.fields!==undefined){if(!obj(record.fields)||Object.keys(record.fields).length>64)invalid();for(const [key,value] of Object.entries(record.fields))if(!text(key,100)||typeof value!=='number'||!Number.isFinite(value))invalid();}
 return record as EvidenceRecord;
}
export function parsePage<T>(v:unknown,parse:(v:unknown)=>T):Page<T>{const p=base(v);if(!Array.isArray(p.items)||p.items.length>40||typeof p.nextCursor!=='string'||p.nextCursor!==''&&!/^\d{1,19}$/.test(p.nextCursor))invalid();return {items:p.items.map(parse),nextCursor:p.nextCursor};}
function parseInbox(v:unknown):Inbox{const p=base(v);if(!integer(p.unread))invalid();return {...parsePage(v,n=>{const v=base(n);if(!id(v.id)||!integer(v.createdAt)||v.readAt!==null&&!integer(v.readAt)||!text(v.message,300)||v.targetId!==undefined&&!id(v.targetId))invalid();return v as Notice;}),unread:p.unread};}
function parseJob(v:unknown):Job{const j=base(v);revision(j);if(!id(j.id)||!id(j.kind)||!Array.isArray(j.actions)||!j.actions.every((v:unknown)=>v==='retry'||v==='cancel'||v==='pause'||v==='resume')||!['queued','running','paused','reconciling','cancellation-requested','cancelled','failed','succeeded'].includes(j.state))invalid();return j as Job;}
function list<T>(v:unknown,parse:(v:unknown)=>T,max=100):T[]{if(!Array.isArray(v)||v.length>max)invalid();return v.map(parse);}
function parseSchedule(v:unknown):Schedule{const s=base(v);revision(s);if(!id(s.id)||!id(s.kind)||!text(s.timezone,80)||typeof s.enabled!=='boolean'||!integer(s.startMinute)||s.startMinute>=1440||!integer(s.windowMinutes)||s.windowMinutes<1||s.windowMinutes>1440||typeof s.catchUp!=='boolean')invalid();return s as Schedule;}
let sequence=0;
/** Operation keys are uniqueness tokens, not credentials or authorization. */
export function consoleOperationId():string{return globalThis.crypto?.randomUUID?.()??`op-${Date.now().toString(36)}-${(++sequence).toString(36)}-${Math.random().toString(36).slice(2,16)}`;}
export class ConsoleClient {
 private disposed=false;private fence?:string;private requests=new Set<AbortController>();private intents=new Map<string,{body:string;key:string;method:string;parse:(v:unknown)=>unknown}>();private reconciled=new Set<()=>void>();private listeners=new Set<()=>void>();
 private drafts=new Map<string,unknown>();private denied=false;private recoveredExport?:SupportExport;
 private api:ConsoleAPI;readonly scope:ConsoleScope;
 constructor(api:ConsoleAPI,scope:ConsoleScope){if(!scope.serverId||!scope.viewerId)throw new Error('A selected viewer is required.');this.api=api;this.scope={...scope};}
 onDenied=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private revoke(){this.denied=true;this.recoveredExport=undefined;this.drafts.clear();this.intents.clear();for(const c of this.requests)c.abort();for(const fn of this.listeners)fn();}
 dispose(){this.disposed=true;this.recoveredExport=undefined;for(const c of this.requests)c.abort();this.requests.clear();this.drafts.clear();this.intents.clear();this.listeners.clear();this.reconciled.clear();}
 setDraft(key:string,value:unknown){if(this.disposed||this.denied)return;if(value===undefined)this.drafts.delete(key);else this.drafts.set(key,value);}
 getDraft<T>(key:string):T|undefined{return this.drafts.get(key) as T|undefined;}
 hasDrafts(){return this.drafts.size>0||this.intents.size>0;}
 hasUnconfirmed(){return this.intents.size>0;}
 takeRecoveredExport(){const value=this.recoveredExport;this.recoveredExport=undefined;return value&&value.expiresAt>Date.now()?value:undefined;}
 onReconciled(fn:()=>void){this.reconciled.add(fn);return()=>{this.reconciled.delete(fn);};}
 async reconcilePending(){
  const entry=this.intents.entries().next().value;if(!entry)return;
  const [path,intent]=entry;const body=JSON.parse(intent.body) as Record<string,unknown>;
  try{const value=await this.call(path,intent.method,{...body,idempotencyKey:intent.key},intent.parse);this.intents.delete(path);
   if(path==='/v1/admin/console/exports')this.recoveredExport=value as SupportExport;
   if(path==='/v1/feedback')this.drafts.delete('feedback');
   else if(path.startsWith('/v1/admin/feedback/'))this.drafts.delete('reply:'+decodeURIComponent(path.split('/').pop()!));
   else if(path==='/v1/admin/console/settings')this.drafts.delete('runtime-settings');
   else if(path==='/v1/preferences')this.drafts.delete('preferences:'+body.scope);
   else if(path==='/v1/admin/console/schedules')this.drafts.delete('schedule');
   for(const fn of this.reconciled)fn();
  }catch(e){if(obj(e)&&[400,404,409,410,429].includes(e.status)){this.intents.delete(path);for(const fn of this.reconciled)fn();}throw e;}
 }
 private async call<T>(path:string,method:string,body:unknown,parse:(v:unknown)=>T):Promise<T>{
  if(this.disposed||this.denied)throw new Error('This server scope is closed.');const c=new AbortController();this.requests.add(c);const timer=setTimeout(()=>c.abort(),10000);
  try{const raw=await (this.api.requestConsole?this.api.requestConsole<unknown>(path,method,body,c.signal):this.api.request<unknown>(path,method,body,c.signal));if(this.disposed||this.denied||c.signal.aborted)throw new Error('Console request ended.');
   if(!obj(raw)||!obj(raw.scope)||raw.scope.serverId!==this.scope.serverId||typeof raw.scope.viewerFence!=='string'||!/^[a-f0-9]{64}$/.test(raw.scope.viewerFence)||this.fence!==undefined&&this.fence!==raw.scope.viewerFence){this.revoke();throw new Error('The selected viewer changed. Reopen this server.');}
   const out=parse(raw.data);this.fence=raw.scope.viewerFence;return out;
  }catch(e){if(obj(e)&&(e.status===401||e.status===403))this.revoke();throw e;}finally{clearTimeout(timer);this.requests.delete(c);}
 }
 private async mutate<T>(path:string,method:string,body:Record<string,unknown>,parse:(v:unknown)=>T):Promise<T>{
  const serialized=JSON.stringify(body);let intent=this.intents.get(path);
  if(intent&&intent.body!==serialized)throw new Error('A previous action has an unconfirmed result. Confirm the pending action before submitting different values.');
  if(!intent){intent={body:serialized,key:consoleOperationId(),method,parse};this.intents.set(path,intent);}
  try{const result=await this.call(path,method,{...body,idempotencyKey:intent.key},parse);this.intents.delete(path);return result;}
  catch(e){if(obj(e)&&[400,404,409,410,429].includes(e.status))this.intents.delete(path);throw e;}

 }
 registry(){return this.call('/v1/console/registry','GET',undefined,base);}
 settings(){return this.call('/v1/admin/console/settings','GET',undefined,parseSettings);}
 applySettings(expectedRevision:number,values:RuntimeSettings){return this.mutate('/v1/admin/console/settings','PATCH',{expectedRevision,values},parseSettings);}
 preferences(deviceClass:DeviceClass){return this.call('/v1/preferences?deviceClass='+deviceClass,'GET',undefined,parsePreferenceSnapshot);}
 /** `values` carries only the keys that change; `null` clears that override. */
 applyPreferences(scope:PreferenceScope,deviceClass:DeviceClass,expectedRevision:number,values:PreferencePatch){return this.mutate('/v1/preferences','PATCH',{scope,deviceClass,expectedRevision,values},parsePreferenceSnapshot);}
 submit(value:SubmitFeedback){return this.mutate('/v1/feedback','POST',value,parseReport);}
 reports(owner=false,cursor=''){return this.call((owner?'/v1/admin/feedback':'/v1/feedback')+'?cursor='+encodeURIComponent(cursor),'GET',undefined,v=>parsePage(v,parseReport));}
 report(id:string,owner=false){return this.call((owner?'/v1/admin/feedback/':'/v1/feedback/')+encodeURIComponent(id),'GET',undefined,parseReport);}
 triage(id:string,expectedRevision:number,status:FeedbackStatus,reply:string){return this.mutate('/v1/admin/feedback/'+encodeURIComponent(id),'PATCH',{expectedRevision,status,reply},parseReport);}
 inbox(cursor=''){return this.call('/v1/notifications?cursor='+encodeURIComponent(cursor),'GET',undefined,parseInbox);}
 readNotice(id:string){return this.call('/v1/notifications/'+encodeURIComponent(id)+'/read','POST',{},v=>{if(!obj(v)||v.read!==true)invalid();});}
 accountOptions(){return this.call('/v1/admin/console/accounts','GET',undefined,v=>list(v,row=>{if(!obj(row)||!id(row.id)||!text(row.label,128))invalid();return {id:row.id as string,label:row.label as string};},1000));}
 streamOptions(){return this.call('/v1/admin/console/stream-options','GET',undefined,parseStreamOptions);}
 jobs(cursor=''){return this.call('/v1/admin/console/jobs?cursor='+encodeURIComponent(cursor),'GET',undefined,v=>parsePage(v,parseJob));}
 jobKinds(){return this.call('/v1/admin/console/job-kinds','GET',undefined,v=>list(v,k=>{const d=base(k);if(!id(d.kind)||typeof d.resourceRequired!=='boolean')invalid();return d as JobKind;}));}
 runJob(kind:string,resource:string){return this.mutate('/v1/admin/console/jobs','POST',{kind,resource},parseJob);}
 jobCommand(job:Job,action:'cancel'|'retry'|'pause'|'resume'){return this.mutate('/v1/admin/console/jobs/'+encodeURIComponent(job.id)+'/'+action,'POST',{expectedRevision:job.revision},parseJob);}
 schedules(){return this.call('/v1/admin/console/schedules','GET',undefined,v=>list(v,parseSchedule));}
 saveSchedule(value:Schedule,expectedRevision:number){const {revision:_revision,lastSlot:_lastSlot,createdAt:_createdAt,...editable}=value;return this.mutate('/v1/admin/console/schedules','PUT',{expectedRevision,value:editable},parseSchedule);}
 measurement(name:string){return this.call('/v1/admin/console/panels/'+encodeURIComponent(name),'GET',undefined,v=>{const p=base(v);if(p.name!==name||!integer(p.observedAt)||p.freshUntil!==p.observedAt+30000||!obj(p.facts)||Object.keys(p.facts).length>30)invalid();for(const f of Object.values(p.facts))if(!obj(f)||!text(f.state,40))invalid();return p as Measurement;});}
 alerts(){return this.call('/v1/admin/console/alerts','GET',undefined,v=>list(v,a=>{const p=base(a);revision(p);if(!id(p.id)||!id(p.code))invalid();return p as Alert;}));}
 acknowledge(alert:Alert){return this.mutate('/v1/admin/console/alerts/'+encodeURIComponent(alert.id)+'/acknowledge','POST',{expectedRevision:alert.revision},base);}
 records(lane:'runtime'|'client'|'audit',cursor=''){return this.call('/v1/admin/console/diagnostics/'+lane+'?cursor='+encodeURIComponent(cursor),'GET',undefined,v=>parsePage(v,parseEvidenceRecord));}
 capture(component:string,expiresAt:number){return this.mutate('/v1/admin/console/diagnostics/capture','POST',{component,expiresAt},base);}
 createExport(from:number,to:number,components:string[]){return this.mutate('/v1/admin/console/exports','POST',{from,to,components},parseExport);}
 export(id:string){return this.call('/v1/admin/console/exports/'+encodeURIComponent(id),'GET',undefined,parseExport);}
 /** Transcoding capacity, probes and dependencies as this server reports them. */
 transcodeCapacity(){return this.call('/v1/admin/transcode/capacity','GET',undefined,parseTranscodeCapacity);}
 /** Charted host and GPU history. The server chooses the resolution per window. */
 telemetry(window:TelemetryWindow='10m'){return this.call('/v1/admin/telemetry?window='+window,'GET',undefined,parseTelemetryReading);}
 telemetryNow(){return this.call('/v1/admin/telemetry/now','GET',undefined,parseTelemetrySample);}
 attention(){return this.call('/v1/admin/attention','GET',undefined,parseAttention);}
 playbackHistory(period:PlaybackHistoryPeriod='24h',cursor='',limit=50){
  return this.call('/v1/admin/playback/history?'+new URLSearchParams({period,cursor,limit:String(limit)}),'GET',undefined,parsePlaybackHistory);
 }
 /** The export is a download, so its path is handed to the browser, not fetched. */
 playbackHistoryExportPath(period:PlaybackHistoryPeriod='24h'){return '/v1/admin/playback/history.csv?period='+period;}
}
function parseExport(v:unknown):SupportExport{const p=base(v);if(!id(p.id)||!integer(p.expiresAt)||!obj(p.manifest)||!Array.isArray(p.manifest.included)||!Array.isArray(p.manifest.excluded)||!text(p.manifest.redaction,500))invalid();return p as SupportExport;}


export function parseStreamOptions(value:unknown):StreamOptions{const v=base(value);for(const field of ['libraries','viewers'])if(!Array.isArray(v[field])||v[field].length>500||v[field].some((row:unknown)=>!obj(row)||!text(row.id,300)||!row.id||!text(row.label,4096)))invalid();return v as StreamOptions;}
