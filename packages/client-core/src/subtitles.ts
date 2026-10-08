import {unreadableServerResponse} from './server-messages.ts';
import type {LibraryContentApi} from './library-content.ts';

export type SubtitleFormat='srt'|'vtt'|'ass'|'ssa'|'pgs'|'sup'|'vobsub'|'idx'|'dvb';
export type SubtitlePresentation=Readonly<{sessionId:string;generation:number;streamUrl:string;mode:'direct'|'hls';positionUs:string}>;
export type SubtitleResource = Readonly<{
 manifestName?:string;id:string; sourceId:string; revision:number; scope:'personal'|'shared'; language:string; title:string;
 format:SubtitleFormat; origin:'upload'|'sidecar'|'embedded'|'provider'; provider?:string; attribution?:string;
 rights:string; offsetUs:string; canManage:boolean; enabled:boolean; reason?:string; pinned?:boolean;
 retired?:boolean; discoveryId?:string;renderer:'external_text'|'burn_in';default:boolean;forced:boolean;
}>;
export type SubtitleDiscovery = Readonly<{
 id:string;sourceId:string;revision:number;origin:'sidecar'|'embedded';format:string;language:string;title:string;
 default:boolean;forced:boolean;enabled:boolean;reason?:string;
}>;
export type SubtitlePlan = Readonly<{
 version:1;sessionId:string;generation:number;sourceId:string;revision:number;catalogRevision:number;
 renderer:'external_text'|'burn_in';presentation?:SubtitlePresentation;mode:'off'|'track';offAvailable:true;offsetUs:string;selected:SubtitleResource|null;
 documentUrl?:string;resources:readonly SubtitleResource[];discovered:readonly SubtitleDiscovery[];appliedRevision?:number;
}>;
export type SubtitleCatalog = Readonly<{
 version:1;itemId:string;revision:number;canShare:boolean;
 sources:readonly Readonly<{id:string;durationUs:string;inventoryRevision:number;available:boolean;timingKnown:boolean}>[];
 resources:readonly SubtitleResource[];discovered:readonly SubtitleDiscovery[];
 provider:Readonly<{id:string;enabled:boolean;reason?:string}>;
}>;
/** `italic` and `top` are the two presentation facts every player can honour:
 * a cue that is wholly italic, and a cue authored at the top of the picture. */
export type SubtitleCue=Readonly<{startUs:string;endUs:string;text:string;italic?:boolean;top?:boolean}>;
export type SubtitleDocument=Readonly<{version:1;timeDomain:'source-relative';cues:readonly SubtitleCue[]}>;
export type SubtitleCandidate=Readonly<{id:string;language:string;title:string;attribution:string}>;
export type SubtitleReceipt=Readonly<{operationId:string;resourceId:string;revision:number;catalogRevision:number;deleted?:boolean}>;
const record=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>text(v,128)&&/^[A-Za-z0-9_-]+$/.test(v);
const integer=(v:unknown,min=1):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min;
const decimal=(v:unknown,max=86400000000,signed=false):v is string=>typeof v==='string'&&/^(?:0|[1-9]\d*|-[1-9]\d*)$/.test(v)&&Number.isSafeInteger(Number(v))&&Number(v)>=(signed?-max:0)&&Number(v)<=max;
function invalid():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_subtitles'});}
function array(v:unknown,max:number):unknown[]{if(!Array.isArray(v)||v.length>max)invalid();return v;}
function resource(v:unknown):SubtitleResource{
 if(!record(v)||!id(v.id)||!id(v.sourceId)||!integer(v.revision)||!['personal','shared'].includes(String(v.scope))||!text(v.language,64)||!text(v.title,160)||!['srt','vtt','ass','ssa','pgs','sup','vobsub','idx','dvb'].includes(String(v.format))||!['upload','sidecar','embedded','provider'].includes(String(v.origin))||!text(v.rights,500)||!decimal(v.offsetUs,600000000,true)||typeof v.canManage!=='boolean'||typeof v.enabled!=='boolean')invalid();
 for(const k of ['provider','attribution','reason','manifestName'])if(v[k]!==undefined&&!text(v[k],512))invalid();
 if(v.renderer!==undefined&&!['external_text','burn_in'].includes(String(v.renderer)))invalid();
 for(const k of ['default','forced','pinned','retired'])if(v[k]!==undefined&&typeof v[k]!=='boolean')invalid();
 if(v.discoveryId!==undefined&&!id(v.discoveryId))invalid();
  const renderer=v.renderer??'external_text';if(renderer==='external_text'&&!['srt','vtt','ass','ssa'].includes(String(v.format)))invalid();
 return Object.freeze({...v,renderer,default:v.default??false,forced:v.forced??false}) as SubtitleResource;
}
function discovery(v:unknown):SubtitleDiscovery{
 if(!record(v)||!id(v.id)||!id(v.sourceId)||!integer(v.revision)||!['sidecar','embedded'].includes(String(v.origin))||!text(v.format,64)||!text(v.language,64)||!text(v.title,160)||typeof v.default!=='boolean'||typeof v.forced!=='boolean'||typeof v.enabled!=='boolean'||v.reason!==undefined&&!text(v.reason,128))invalid();
 return Object.freeze({...v}) as SubtitleDiscovery;
}
export function validateSubtitlePlan(v:unknown):SubtitlePlan{
 if(!record(v)||v.version!==1||!id(v.sessionId)||!integer(v.generation)||!id(v.sourceId)||!integer(v.revision)||!integer(v.catalogRevision)||!['external_text','burn_in'].includes(String(v.renderer))||!['off','track'].includes(String(v.mode))||v.offAvailable!==true||!decimal(v.offsetUs,600000000,true))invalid();
 const resources=array(v.resources,128).map(resource),discovered=array(v.discovered,1024).map(discovery),selected=v.selected===null?null:resource(v.selected);
 if(resources.some(r=>r.sourceId!==v.sourceId)||discovered.some(r=>r.sourceId!==v.sourceId)||selected&&selected.sourceId!==v.sourceId)invalid();
 if(new Set(resources.map(r=>r.id)).size!==resources.length||new Set(discovered.map(r=>r.id)).size!==discovered.length)invalid();
 if((v.mode==='off')!==(selected===null)||v.mode==='off'&&(v.offsetUs!=='0'||v.documentUrl!==undefined))invalid();
 if(v.documentUrl!==undefined&&(!selected||!text(v.documentUrl,512)||!new RegExp('^/v1/media/[A-Za-z0-9_-]{1,128}/subtitles/'+selected.id+'/'+selected.revision+'$').test(v.documentUrl)))invalid();
 if(selected&&(selected.pinned!==true||selected.renderer!==v.renderer))invalid();if(v.mode==='off'&&v.renderer!=='external_text')invalid();if(selected?.enabled&&v.renderer==='external_text'&&!v.documentUrl)invalid();if(v.renderer==='burn_in'&&v.documentUrl!==undefined)invalid();
 if(v.presentation!==undefined){const p=v.presentation;if(!record(p)||p.sessionId!==v.sessionId||p.generation!==v.generation||!text(p.streamUrl,1024)||!/^\/v1\/media\/[A-Za-z0-9_-]+(?:\/master\.m3u8)?$/.test(p.streamUrl)||!['direct','hls'].includes(String(p.mode))||!decimal(p.positionUs))invalid();}
 if(v.appliedRevision!==undefined&&(!integer(v.appliedRevision)||v.appliedRevision>v.revision))invalid();
 return Object.freeze({...v,selected,resources:Object.freeze(resources),discovered:Object.freeze(discovered)}) as SubtitlePlan;
}
export function validateSubtitleCatalog(v:unknown):SubtitleCatalog{
 if(!record(v)||v.version!==1||!id(v.itemId)||!integer(v.revision)||typeof v.canShare!=='boolean'||!record(v.provider)||!text(v.provider.id,64)||typeof v.provider.enabled!=='boolean'||v.provider.reason!==undefined&&!text(v.provider.reason,128))invalid();
 const sources=array(v.sources,32).map(s=>{if(!record(s)||!id(s.id)||!decimal(s.durationUs)||!integer(s.inventoryRevision,0)||typeof s.available!=='boolean'||typeof s.timingKnown!=='boolean')invalid();return Object.freeze({...s});});
 const resources=array(v.resources,128).map(resource),discovered=array(v.discovered,1024).map(discovery);
 if(resources.some(r=>!sources.some(s=>s.id===r.sourceId))||discovered.some(r=>!sources.some(s=>s.id===r.sourceId)))invalid();
 return Object.freeze({...v,sources:Object.freeze(sources),resources:Object.freeze(resources),discovered:Object.freeze(discovered),provider:Object.freeze({...v.provider})}) as SubtitleCatalog;
}
/** Strict about the envelope, forgiving about the cues.
 *
 * A document that is not a version 1 source-relative cue list is refused: the
 * caller shows no subtitles and asks again. Inside a valid envelope one bad cue
 * costs only itself. A cue with unreadable times, an end before its start or no
 * words is dropped; control characters and any markup that slipped through are
 * removed (angle brackets are words: the server sends plain text and players escape it); an over-long cue is cut; cues out of order are put in order; and the
 * whole is bounded (20 000 cues, 4 MiB of text). A document whose every cue was
 * bad is refused like a bad envelope, so "this track is empty" is never shown for
 * what is really a broken response. */
export function validateSubtitleDocument(v:unknown):SubtitleDocument{
 if(!record(v)||v.version!==1||v.timeDomain!=='source-relative'||!Array.isArray(v.cues)||v.cues.length>20000)invalid();
 const day=86_400_000_000;let total=0,ordered=true,previous=-1;
 const cues:{startUs:string;endUs:string;text:string;italic?:boolean;top?:boolean;at:number}[]=[];
 for(const c of v.cues as unknown[]){
  if(!record(c)||!decimal(c.startUs)||!decimal(c.endUs)||typeof c.text!=='string')continue;
  const start=Number(c.startUs),end=Number(c.endUs);
  if(!(end>start)||end>day)continue;
  let words=c.text.replace(/\{\\[^{}]*\}/g,'').replace(/[\x00-\x08\x0b-\x1f\x7f]/g,'').replace(/[ \t]+\n/g,'\n').trim();
  if(!words)continue;
  if(words.length>8192)words=words.slice(0,8192);
  total+=words.length;if(total>4194304)break;
  if(start<previous)ordered=false;previous=start;
  cues.push({startUs:c.startUs,endUs:c.endUs,text:words,...(c.italic===true?{italic:true}:{}),...(c.top===true?{top:true}:{}),at:start});
 }
 if(!cues.length&&v.cues.length)invalid();
 if(!ordered)cues.sort((a,b)=>a.at-b.at);
 return Object.freeze({version:1,timeDomain:'source-relative',cues:Object.freeze(cues.map(({at:_,...cue})=>Object.freeze(cue)))});
}
/** One authorised read of the active subtitle document.
 *
 * Players reload the document every few seconds purely to stay authorised. The
 * server runs its full check every time and answers 304 with no body when the
 * validator sent here still names the document, so the reload costs a header
 * instead of the whole track. The browser cache is never involved: the request
 * is `no-store` and the validator is set by hand, so a revoked viewer can only
 * ever be answered by the server. */
export type SubtitleDocumentRead=Readonly<{unchanged:true}>|Readonly<{unchanged:false;document:SubtitleDocument;etag:string}>;
export async function fetchSubtitleDocument(fetcher:typeof fetch,url:string,etag:string,signal:AbortSignal):Promise<SubtitleDocumentRead>{
 const validator=/^"[A-Za-z0-9_.-]{1,200}"$/.test(etag)?etag:'';
 const response=await fetcher(url,{signal,credentials:'omit',cache:'no-store',redirect:'error',...(validator?{headers:{'If-None-Match':validator}}:{})});
 if(response.status===304&&validator){void response.body?.cancel().catch(()=>{});return Object.freeze({unchanged:true as const});}
 const document=await readSubtitleDocument(response,signal);
 const next=response.headers.get('ETag')??'';
 return Object.freeze({unchanged:false as const,document,etag:/^"[A-Za-z0-9_.-]{1,200}"$/.test(next)?next:''});
}
export async function readSubtitleDocument(response:Response,signal:AbortSignal):Promise<SubtitleDocument>{
 if(!response.ok){void response.body?.cancel().catch(()=>{});throw Object.assign(new Error('Subtitle access is no longer available.'),{status:response.status});}
 if(!response.headers.get('Content-Type')?.toLowerCase().startsWith('application/json'))invalid();
 const max=8388608,declared=response.headers.get('Content-Length');if(declared!==null&&(!/^\d+$/.test(declared)||Number(declared)>max))invalid();
 if(!response.body)invalid();const reader=response.body.getReader();const chunks:Uint8Array[]=[];let total=0;
 const abort=()=>{void reader.cancel().catch(()=>{});};signal.addEventListener('abort',abort,{once:true});
 try{while(!signal.aborted){const {done,value}=await reader.read();if(done)break;total+=value.length;if(total>max)invalid();chunks.push(value);}if(signal.aborted)throw new Error('Subtitle request cancelled.');const bytes=new Uint8Array(total);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.length;}return validateSubtitleDocument(JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes)));}
 finally{signal.removeEventListener('abort',abort);void reader.cancel().catch(()=>{});reader.releaseLock();}
}
export function subtitleReason(reason?:string):string{
 const reasons:Record<string,string>={provider_not_configured:'Configure the OpenSubtitles API key on the server.',unsupported_format:'This format is not supported by the configured decoder.',unsupported_text_features:'This subtitle uses an unsupported or invalid text feature.',extraction_unavailable:'The server needs FFmpeg and verified source timing to extract this track.',source_changed:'The media source changed. Rescan and import again.',source_unavailable:'The media source is unavailable.',ambiguous_sidecar:'This sidecar matches more than one media file.',unsupported_timing:'Subtitle timing could not be validated.',capacity:'The subtitle exceeds a supported size or cue limit.'};return reasons[reason??'']??'This subtitle is unavailable. Refresh sources or upload a supported subtitle.';
}
export type SubtitleTarget=Readonly<{itemId:string;sessionId:string;generation:number;sourceId?:never}>|Readonly<{itemId:string;sourceId:string;sessionId?:never;generation?:never}>;
export type SubtitleSnapshot=Readonly<{catalog:SubtitleCatalog|null;plan:SubtitlePlan|null;busy:boolean;loading:boolean;suppressed:boolean;error:string|null;canRetry:boolean;candidates:readonly SubtitleCandidate[]}>;
/** Bound to exactly one viewer/API and playback generation. No disk cache or
 * separate media clock. Management operations do not replace the active pin. */
export class SubtitleService {
 private state:SubtitleSnapshot={catalog:null,plan:null,busy:false,loading:true,suppressed:false,error:null,canRetry:false,candidates:[]};
 private listeners=new Set<()=>void>();private active=false;private life=0;private interval?:ReturnType<typeof setInterval>;
 private controllers=new Set<AbortController>();private queue:Promise<void>=Promise.resolve();private queued=0;private choice=0;private retryOperation?:()=>Promise<void>;
 private api:LibraryContentApi; target:SubtitleTarget; private operationId:()=>string; private transport?:{positionUs:()=>string;reserve?:(payload:Record<string,unknown>)=>Promise<Record<string,unknown>>;select?:(path:string,payload:Record<string,unknown>)=>Promise<unknown>;install:(presentation:SubtitlePresentation)=>boolean};
 constructor(api:LibraryContentApi,target:SubtitleTarget,operationId:()=>string,transport?:{positionUs:()=>string;reserve?:(payload:Record<string,unknown>)=>Promise<Record<string,unknown>>;select?:(path:string,payload:Record<string,unknown>)=>Promise<unknown>;install:(presentation:SubtitlePresentation)=>boolean}){this.api=api;this.target=target;this.operationId=operationId;this.transport=transport;if(!id(target.itemId)||(target.sourceId!==undefined?!id(target.sourceId):!id(target.sessionId)||!integer(target.generation)))throw new Error('Invalid subtitle target.');}
 getSnapshot=()=>this.state;subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(p:Partial<SubtitleSnapshot>){this.state=Object.freeze({...this.state,...p});this.listeners.forEach(f=>f());}
 private path(suffix=''){return '/v1/items/'+this.target.itemId+'/subtitles'+suffix;}
 get sourceId(){return this.target.sourceId??this.state.plan?.sourceId;}
 private selectionPath(){if(this.target.sourceId!==undefined)throw new Error('Start playback to select subtitles.');return '/v1/items/'+this.target.itemId+'/playback/'+this.target.sessionId+'/subtitles';}
 private polling=true;
 start(){if(this.active)return;this.active=true;this.life++;void this.refresh();this.arm();}
 private arm(){if(this.interval){clearInterval(this.interval);this.interval=undefined;}if(this.active&&this.polling&&this.target.sourceId===undefined)this.interval=setInterval(()=>void this.refresh(),25000);}
 /** PERF-24: re-reading the catalogue every 25 s is only worth it while subtitles are on or their
  * menu is open; a client turns it off otherwise (a later `setPolling(true)` reads at once). */
 setPolling(on:boolean){if(this.polling===on)return;this.polling=on;this.arm();if(on&&this.active)void this.refresh();}
 stop(){this.active=false;this.life++;this.choice++;if(this.interval)clearInterval(this.interval);for(const c of this.controllers)c.abort();this.controllers.clear();this.retryOperation=undefined;this.publish({catalog:null,plan:null,candidates:[],suppressed:true,loading:false});}
 private async request(path:string,method='GET',body?:unknown):Promise<unknown>{
  const c=new AbortController(),life=this.life;this.controllers.add(c);const timer=setTimeout(()=>c.abort(),method==='GET'?15000:110000);
  try{const result=await this.api.request<unknown>(path,method,body,c.signal);if(!this.active||life!==this.life)throw new Error('Subtitle scope retired.');return result;}finally{clearTimeout(timer);this.controllers.delete(c);}
 }
 private acceptPlan(raw:unknown){const p=validateSubtitlePlan(raw);if(this.target.generation===undefined||p.sessionId!==this.target.sessionId||p.generation<this.target.generation)invalid();
 if(p.presentation){if(!this.transport||!this.transport.install(p.presentation))throw new Error('Subtitle playback scope retired.');}
 if(p.generation!==this.target.generation){if(!p.presentation)invalid();this.target={itemId:this.target.itemId,sessionId:p.sessionId,generation:p.generation};}if(this.state.plan&&p.revision<this.state.plan.revision)return this.state.plan;this.publish({plan:p});return p;}
 private async plan(){return this.acceptPlan(await this.request(this.selectionPath()));}
 private fail(e:unknown){
  if(!this.active)return;const problem=e as {status?:number;code?:string};
  const denied=problem.status===401||problem.status===403,conflict=problem.status===409;
  const invalidInput=problem.status===400||problem.status===422,missing=problem.status===404;
  const invalidData=problem.code==='invalid_subtitles';
  const canRetry=!denied&&!conflict&&!invalidInput&&!missing&&!invalidData;
  this.publish({error:denied?'You no longer have access to these subtitles.':conflict?'Subtitles changed. Refresh and review the current revision before retrying.':invalidInput?'Check the language, file and timing. Use plain SRT/WebVTT or supported rich/bitmap subtitles within the size and cue limits.':missing?'This subtitle is no longer available. Refresh the current choices.':invalidData?unreadableServerResponse:'Subtitles could not be updated. Refresh, or retry the same operation.',canRetry,suppressed:denied||invalidData||!this.state.plan,...(denied?{catalog:null,plan:null,candidates:[]}: {})});
  if(!canRetry)this.retryOperation=undefined;
 }
 async refresh(){if(!this.active||this.queued)return;const life=this.life;this.publish({loading:true});try{const [raw]=await Promise.all([this.request(this.path()),this.target.sourceId===undefined?this.plan():Promise.resolve(null)]);if(!this.active||life!==this.life||this.queued)return;const catalog=validateSubtitleCatalog(raw);if(catalog.itemId!==this.target.itemId||this.target.sourceId!==undefined&&!catalog.sources.some(source=>source.id===this.target.sourceId))invalid();if(!this.state.catalog||catalog.revision>=this.state.catalog.revision)this.publish({catalog});this.publish({loading:false,error:null,suppressed:false});}catch(e){if(life===this.life){this.publish({loading:false});this.fail(e);}}}
 private enqueue(task:()=>Promise<void>){if(!this.active)return Promise.resolve();const life=this.life;this.queued++;this.publish({busy:true,error:null,canRetry:false});
  const run=async()=>{if(!this.active||life!==this.life)return;try{await task();this.retryOperation=undefined;this.publish({error:null,canRetry:false});}catch(e){this.retryOperation=task;this.fail(e);}};
  const result=this.queue.then(run,run);this.queue=result.finally(()=>{this.queued--;if(this.active&&life===this.life){this.publish({busy:this.queued>0});if(!this.queued&&!this.state.error)void this.refresh();}});return result;
 }
 refreshSources(){return this.enqueue(async()=>{const sourceId=this.sourceId;if(!sourceId)throw new Error('No active source.');const catalog=validateSubtitleCatalog(await this.request(this.path('/refresh'),'POST',{sourceId}));if(catalog.itemId!==this.target.itemId||!catalog.sources.some(source=>source.id===sourceId))invalid();this.publish({catalog});if(this.target.sourceId===undefined)await this.plan();});}
 remote(name:string,format:SubtitleFormat,language:string,title:string,rights:string,scope:'personal'|'shared') {const payload={operationId:this.operationId(),sourceId:this.sourceId,name,format,language,title,rights,scope};return this.enqueue(async()=>{this.receipt(await this.request(this.path('/remote'),'POST',payload));});}
 retry(){if(this.retryOperation)return this.enqueue(this.retryOperation);return this.refresh();}

 choose(resource:SubtitleResource|null,offsetUs=resource?.offsetUs??'0'){
  if(this.target.sourceId!==undefined)return Promise.reject(new Error('Start playback to select subtitles.'));
  if(!decimal(offsetUs,600000000,true))return Promise.reject(new Error('Subtitle offset must be within ten minutes.'));
  const choice=++this.choice,op=this.operationId();let payload:Record<string,unknown>|undefined;
  // Text overlays can turn off immediately. Burned cues remain until the committed video replacement.
  if(!resource&&this.state.plan?.renderer!=='burn_in')this.publish({suppressed:true});
  return this.enqueue(async()=>{if(choice!==this.choice)return;if(!payload){const current=await this.plan();payload={operationId:op,generation:this.target.generation,expectedRevision:current.revision,mode:resource?'track':'off',...(resource?{resourceId:resource.id,resourceRevision:resource.revision}:{}),offsetUs:resource?offsetUs:'0',positionUs:this.transport?.positionUs()??'0'};if(this.transport?.reserve)payload=await this.transport.reserve(payload);}const raw=payload.control&&this.transport?.select?await this.transport.select(this.selectionPath(),payload):await this.request(this.selectionPath(),'PUT',payload);if(choice===this.choice){this.acceptPlan(raw);this.publish({suppressed:false});}});
 }
 import(track:SubtitleDiscovery){if(this.target.sourceId!==undefined)return Promise.reject(new Error('Start playback to import and select a track.'));const choice=++this.choice,op=this.operationId(),selectOp=this.operationId();let receipt:SubtitleReceipt|undefined;let selection:Record<string,unknown>|undefined;
  return this.enqueue(async()=>{if(!receipt)receipt=this.receipt(await this.request(this.path('/import'),'POST',{operationId:op,sourceId:track.sourceId,discoveryId:track.id,expectedRevision:track.revision,scope:'personal'}));if(choice!==this.choice)return;if(!selection){const plan=await this.plan();selection={operationId:selectOp,generation:this.target.generation,expectedRevision:plan.revision,mode:'track',resourceId:receipt.resourceId,resourceRevision:receipt.revision,offsetUs:'0',positionUs:this.transport?.positionUs()??'0'};if(this.transport?.reserve)selection=await this.transport.reserve(selection);}const raw=selection.control&&this.transport?.select?await this.transport.select(this.selectionPath(),selection):await this.request(this.selectionPath(),'PUT',selection);if(choice===this.choice){this.acceptPlan(raw);this.publish({suppressed:false});}});
 }
 private receipt(v:unknown):SubtitleReceipt{if(!record(v)||!id(v.operationId)||!id(v.resourceId)||!integer(v.revision)||!integer(v.catalogRevision)||v.deleted!==undefined&&typeof v.deleted!=='boolean')invalid();return Object.freeze({...v}) as SubtitleReceipt;}
 save(input:{content?:string;data?:string;companion?:string;default?:boolean;forced?:boolean;format:SubtitleFormat;language:string;title:string;rights:string;scope:'personal'|'shared';offsetUs:string},resource?:SubtitleResource){
  const payload={...input,operationId:this.operationId(),sourceId:resource?.sourceId??this.sourceId,expectedRevision:resource?.revision??0,...(resource?{resourceId:resource.id}: {})};
  return this.enqueue(async()=>{this.receipt(await this.request(this.path(resource?'/'+resource.id:''),resource?'PUT':'POST',payload));});
 }
 remove(resource:SubtitleResource){const body={operationId:this.operationId(),expectedRevision:resource.revision};return this.enqueue(async()=>{this.receipt(await this.request(this.path('/'+resource.id),'DELETE',body));});}
 search(language:string,query='',page=1){return this.enqueue(async()=>{const raw=await this.request(this.path('/search'),'POST',{sourceId:this.sourceId,language,query,page});if(!record(raw)||raw.version!==1||!integer(raw.page))invalid();const candidates=array(raw.candidates,40).map(c=>{if(!record(c)||!id(c.id)||!text(c.language,64)||!text(c.title,160)||!text(c.attribution,512))invalid();return Object.freeze({...c}) as SubtitleCandidate;});this.publish({candidates:Object.freeze(candidates)});});}
 apply(candidate:SubtitleCandidate,scope:'personal'|'shared',rights:string){const payload={operationId:this.operationId(),candidateId:candidate.id,scope,rights};return this.enqueue(async()=>{this.receipt(await this.request(this.path('/apply'),'POST',payload));this.publish({candidates:[]});});}
}
