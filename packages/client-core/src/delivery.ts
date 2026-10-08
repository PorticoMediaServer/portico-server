import {unreadableServerResponse} from './server-messages.ts';
/** Server-authored delivery policy and per-session conversion diagnostics.
 *
 * The client renders these; it never derives them. Network class, quality rungs,
 * clamps and reason codes are all the server's conclusions, and a payload that
 * does not match the published shape is rejected rather than repaired. */

export type NetworkClass='local'|'wifi'|'cellular'|'remote'|'unknown';
export type TransportClass='wifi'|'cellular'|'ethernet'|'unknown';
export type DeliveryMode='allow'|'prefer'|'never'|'require';
import type {PlanningPolicy} from './console.ts';

export const networkClasses:readonly NetworkClass[]=Object.freeze(['local','wifi','cellular','remote','unknown'] as const);
export const transportClasses:readonly TransportClass[]=Object.freeze(['wifi','cellular','ethernet','unknown'] as const);
const deliveryModes:readonly DeliveryMode[]=Object.freeze(['allow','prefer','never','require'] as const);
const planningPolicies:readonly PlanningPolicy[]=Object.freeze(['maximum_fidelity','maximum_compatibility','minimize_server_work'] as const);

/** One narrowing the server applied over the viewer's own choice. Clamps only
 * ever narrow, so `applied` is never greater than `requested`. */
export type DeliveryClamp=Readonly<{field:string;source:string;requested:number;applied:number}>;

export type ResolvedDeliveryPolicy=Readonly<{
 networkClass:NetworkClass;serverLocality:'local'|'remote'|'unknown';transportClass:TransportClass;preferenceLane:'local'|'wifi'|'cellular'|'unknown';
 directPlay:DeliveryMode;directStream:DeliveryMode;transcode:DeliveryMode;qualityMode:string;
 maxVideoBitrateBps:number;maxAudioBitrateBps:number;maxVideoHeight:number;allowHDR:boolean;
 planningPolicy:PlanningPolicy;clamps:readonly DeliveryClamp[];
}>;

export type QualityRungKind='automatic'|'original'|'fixed';
export type QualityRung=Readonly<{
 id:string;kind:QualityRungKind;label:string;enabled:boolean;reason?:string;
 maxVideoBitrateBps?:number;maxAudioBitrateBps?:number;targetDisplayHeight?:number;
}>;

export type DeliveryStrategy='original'|'copy_remux'|'audio_conversion'|'video_conversion';
export type StreamAction='copy'|'convert'|'original'|'drop'|'burn_in'|'unknown';
/** Backends a running session can report; 'auto' is a configuration value, never an outcome. */
export type DeliveryBackend='software'|'videotoolbox'|'vaapi'|'qsv'|'nvenc'|'amf';
export type HardwareStage=Readonly<{operation:'decode'|'deinterlace'|'tone_map'|'subtitle_burn_in'|'scale'|'encode';execution:'hardware'|'software'}>;
export type SessionStreamDecision=Readonly<{kind:'video'|'audio'|'subtitle';inputCodec:string;outputCodec:string;action:StreamAction}>;

/** The per-session decision detail published on the occurrence read. */
export type DeliveryAudioRendition=Readonly<{failure?:string;ordinal:number;streamIndex:number;action:string;codec:string;channels:number;bitrateBps:number;language:string;label:string;default:boolean}>;
export type SessionDelivery=Readonly<{
 audioRenditions?:readonly DeliveryAudioRendition[];
 mode:'direct'|'hls'|'remote';strategy:DeliveryStrategy;reasonCodes:readonly string[];policy:ResolvedDeliveryPolicy|null;
 qualityId:string;hardware:Readonly<{backend:DeliveryBackend;stages:readonly HardwareStage[]}>;
 streams:readonly SessionStreamDecision[];throttled:boolean;convertedThroughSeconds:number;
 toneMap:boolean;toneMapAlgorithm?:string;targetDisplayHeight?:number;targetVideoBitrateBps?:number;targetAudioBitrateBps?:number;
 playedRetentionSeconds:number;throttleBufferSeconds:number;
 /** Why this route: the device the plan was made for, the source as the planner
  * saw it, and every route with the rules that rejected it. */
 decision?:DecisionTrace;
 audioChannels?:number;downmix?:string;deinterlace?:boolean;outputContainer?:string;
 /** The class of a conversion failure, or of a hardware failure recovered in software. */
 failure?:string;
}>;

export type RouteRejection=Readonly<{code:string;detail?:string}>;
export type RouteEvaluation=Readonly<{route:DeliveryStrategy;transport?:string;admissible:boolean;chosen:boolean;rejections:readonly RouteRejection[]}>;
export type DecisionTrace=Readonly<{
 client:Readonly<{family:string;platform?:string;model?:string;engine?:string;evidence:string;revision:string;published:boolean}>;
 container:string;bitRate?:number;
 video?:Readonly<{codec:string;profile?:string;level?:number;bitDepth?:number;width?:number;height?:number;frameRate?:number;interlaced?:boolean;dynamicRange?:string;dolbyVisionProfile?:number;bitRate?:number}>;
 audio?:Readonly<{streamIndex:number;codec:string;channels?:number;language?:string;objectAudio?:string;chosenBy:string;tracks:number}>;
 routes:readonly RouteEvaluation[];notes:readonly string[];
}>;

class DeliveryFailure extends Error{code:string;constructor(message:string){super(message);this.code='invalid_delivery_payload';}}
function invalid():never{throw new DeliveryFailure(unreadableServerResponse);}
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=256):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const count=(v:unknown,max=Number.MAX_SAFE_INTEGER):v is number=>Number.isSafeInteger(v)&&Number(v)>=0&&Number(v)<=max;
function list(v:unknown,max:number):unknown[]{if(!Array.isArray(v)||v.length>max)invalid();return v;}
function oneOf<T extends string>(v:unknown,allowed:readonly T[]):T{if(typeof v!=='string'||!(allowed as readonly string[]).includes(v))invalid();return v as T;}

function clamp(v:unknown):DeliveryClamp{
 if(!obj(v)||!text(v.field,64)||!text(v.source,64)||!count(v.requested)||!count(v.applied))invalid();
 // A clamp that widened is not a clamp; refuse it rather than display it.
 if(v.applied>v.requested&&v.requested>0)invalid();
 return Object.freeze({field:v.field,source:v.source,requested:v.requested,applied:v.applied});
}

export function parseResolvedDeliveryPolicy(raw:unknown):ResolvedDeliveryPolicy{
 if(!obj(raw))invalid();
 const networkClass=oneOf(raw.networkClass,networkClasses);
 const preferenceLane=oneOf(raw.preferenceLane,['local','wifi','cellular','unknown'] as const);
 // The lane a remote client lands in is the unknown lane: no viewer configures
 // every network they might visit.
 const expected=networkClass==='remote'?'unknown':networkClass;
 if(preferenceLane!==expected)invalid();
 if(typeof raw.allowHDR!=='boolean')invalid();
 if(!count(raw.maxVideoBitrateBps)||!count(raw.maxAudioBitrateBps)||!count(raw.maxVideoHeight,4320))invalid();
 if(!text(raw.qualityMode,64))invalid();
 return Object.freeze({
  networkClass,
  serverLocality:oneOf(raw.serverLocality,['local','remote','unknown'] as const),
  transportClass:oneOf(raw.transportClass,transportClasses),
  preferenceLane,
  directPlay:oneOf(raw.directPlay,deliveryModes),
  directStream:oneOf(raw.directStream,deliveryModes),
  transcode:oneOf(raw.transcode,deliveryModes),
  qualityMode:raw.qualityMode,
  maxVideoBitrateBps:raw.maxVideoBitrateBps,
  maxAudioBitrateBps:raw.maxAudioBitrateBps,
  maxVideoHeight:raw.maxVideoHeight,
  allowHDR:raw.allowHDR,
  planningPolicy:oneOf(raw.planningPolicy,planningPolicies),
  clamps:Object.freeze(list(raw.clamps??[],16).map(clamp)),
 });
}

export function parseQualityRung(raw:unknown):QualityRung{
 if(!obj(raw)||!text(raw.id,64)||!/^[A-Za-z0-9_-]+$/.test(raw.id)||!text(raw.label,128)||typeof raw.enabled!=='boolean')invalid();
 const kind=oneOf(raw.kind,['automatic','original','fixed'] as const);
 for(const key of ['maxVideoBitrateBps','maxAudioBitrateBps','targetDisplayHeight'])if(raw[key]!==undefined&&!count(raw[key],key==='targetDisplayHeight'?4320:400_000_000))invalid();
 // Automatic and original are the server deciding and the source respectively;
 // neither carries a ceiling of its own, and a fixed rung always does.
 const ceilings=raw.maxVideoBitrateBps!==undefined||raw.maxAudioBitrateBps!==undefined||raw.targetDisplayHeight!==undefined;
 if(kind==='fixed'?!ceilings:ceilings)invalid();
 if(raw.reason!==undefined&&!text(raw.reason,64))invalid();
 if(raw.enabled&&raw.reason!==undefined&&raw.reason!=='')invalid();
 return Object.freeze({
  id:raw.id,kind,label:raw.label,enabled:raw.enabled,
  ...(raw.reason===undefined||raw.reason===''?{}:{reason:raw.reason as string}),
  ...(raw.maxVideoBitrateBps===undefined?{}:{maxVideoBitrateBps:raw.maxVideoBitrateBps as number}),
  ...(raw.maxAudioBitrateBps===undefined?{}:{maxAudioBitrateBps:raw.maxAudioBitrateBps as number}),
  ...(raw.targetDisplayHeight===undefined?{}:{targetDisplayHeight:raw.targetDisplayHeight as number}),
 });
}

/** A published rung set has exactly one automatic rung, and it is first. */
export function parseQualityRungs(raw:unknown):readonly QualityRung[]{
 const rungs=list(raw,16).map(parseQualityRung);
 if(rungs.length===0)return Object.freeze(rungs);
 if(rungs.filter(r=>r.kind==='automatic').length!==1||rungs[0].kind!=='automatic')invalid();
 if(new Set(rungs.map(r=>r.id)).size!==rungs.length)invalid();
 return Object.freeze(rungs);
}

const code=(v:unknown):v is string=>text(v,64)&&/^[a-z0-9_:]+$/.test(v);
const optionalText=(v:unknown,max=64)=>v===undefined||text(v,max);
const optionalCount=(v:unknown,max=Number.MAX_SAFE_INTEGER)=>v===undefined||count(v,max);
const optionalNumber=(v:unknown)=>v===undefined||typeof v==='number'&&Number.isFinite(v)&&v>=0;

/** The trace is an explanation. It is parsed strictly enough to be safe to
 * render, and a trace that does not fit is dropped on its own rather than taking
 * the rest of the technical details with it. */
export function parseDecisionTrace(raw:unknown):DecisionTrace|undefined{
 try{
  if(!obj(raw)||!obj(raw.client)||!text(raw.client.family,32)||!text(raw.client.evidence,16)||!text(raw.client.revision,64)||typeof raw.client.published!=='boolean'||!text(raw.container,32))return undefined;
  const c=raw.client;
  if(!optionalText(c.platform)||!optionalText(c.model)||!optionalText(c.engine))return undefined;
  let video:DecisionTrace['video'];
  if(raw.video!==undefined){
   const v=raw.video;
   if(!obj(v)||!text(v.codec,32)||!optionalText(v.profile)||!optionalCount(v.level,1000)||!optionalCount(v.bitDepth,16)||!optionalCount(v.width,65536)||!optionalCount(v.height,65536)||!optionalNumber(v.frameRate)||!optionalText(v.dynamicRange,16)||!optionalCount(v.dolbyVisionProfile,31)||!optionalCount(v.bitRate))return undefined;
   video=Object.freeze({codec:v.codec,profile:v.profile as string|undefined,level:v.level as number|undefined,bitDepth:v.bitDepth as number|undefined,width:v.width as number|undefined,height:v.height as number|undefined,frameRate:v.frameRate as number|undefined,interlaced:v.interlaced===true,dynamicRange:v.dynamicRange as string|undefined,dolbyVisionProfile:v.dolbyVisionProfile as number|undefined,bitRate:v.bitRate as number|undefined});
  }
  let audio:DecisionTrace['audio'];
  if(raw.audio!==undefined){
   const a=raw.audio;
   if(!obj(a)||!Number.isSafeInteger(a.streamIndex)||!text(a.codec,32)||!optionalCount(a.channels,64)||!optionalText(a.language,35)||!optionalText(a.objectAudio,16)||!text(a.chosenBy,32)||!count(a.tracks,128))return undefined;
   audio=Object.freeze({streamIndex:a.streamIndex as number,codec:a.codec,channels:a.channels as number|undefined,language:a.language as string|undefined,objectAudio:a.objectAudio as string|undefined,chosenBy:a.chosenBy,tracks:a.tracks});
  }
  const routes=list(raw.routes??[],8).map(r=>{
   if(!obj(r)||typeof r.admissible!=='boolean'||typeof r.chosen!=='boolean'||!optionalText(r.transport,16))invalid();
   const rejections=list(r.rejections??[],32).map(x=>{if(!obj(x)||!code(x.code)||!optionalText(x.detail,128))invalid();return Object.freeze({code:x.code,...(x.detail?{detail:x.detail as string}:{})});});
   return Object.freeze({route:oneOf(r.route,['original','copy_remux','audio_conversion','video_conversion'] as const),...(r.transport?{transport:r.transport as string}:{}),admissible:r.admissible,chosen:r.chosen,rejections:Object.freeze(rejections)});
  });
  const notes=list(raw.notes??[],16).filter(code);
  return Object.freeze({client:Object.freeze({family:c.family as string,platform:c.platform as string|undefined,model:c.model as string|undefined,engine:c.engine as string|undefined,evidence:c.evidence as string,revision:c.revision as string,published:c.published as boolean}),container:raw.container,bitRate:optionalCount(raw.bitRate)?raw.bitRate as number|undefined:undefined,video,audio,routes:Object.freeze(routes),notes:Object.freeze(notes)});
 }catch{return undefined;}
}

export function parseDeliveryAudioRenditions(raw:unknown):readonly DeliveryAudioRendition[]|undefined {
 if(!Array.isArray(raw)||raw.length<1||raw.length>16)return undefined;
 const seen=new Set<number>();
 for(let i=0;i<raw.length;i++){const r=raw[i];if(!obj(r)||r.ordinal!==i||!count(r.streamIndex,65535)||seen.has(r.streamIndex)||!['copy','convert'].includes(String(r.action))||!text(r.codec,32)||!count(r.channels,64)||!count(r.bitrateBps,4000000)||!text(r.language,63)||!text(r.label,256)||typeof r.default!=='boolean'||r.failure!==undefined&&!text(r.failure,64))return undefined;seen.add(r.streamIndex);}
 if(raw.filter(r=>r.default).length!==1)return undefined;
 return Object.freeze(raw.map(r=>Object.freeze({...r}))) as readonly DeliveryAudioRendition[];
}
export function parseSessionDelivery(raw:unknown):SessionDelivery{
 if(!obj(raw))invalid();
 if(!text(raw.qualityId,64)||typeof raw.throttled!=='boolean'||typeof raw.toneMap!=='boolean')invalid();
 if(typeof raw.convertedThroughSeconds!=='number'||!Number.isFinite(raw.convertedThroughSeconds)||raw.convertedThroughSeconds<0)invalid();
 if(!count(raw.playedRetentionSeconds,86_400)||!count(raw.throttleBufferSeconds,3_600))invalid();
 if(raw.toneMapAlgorithm!==undefined&&!text(raw.toneMapAlgorithm,32))invalid();
 for(const key of ['targetDisplayHeight','targetVideoBitrateBps','targetAudioBitrateBps'])if(raw[key]!==undefined&&!count(raw[key]))invalid();
 const hardware=obj(raw.hardware)?raw.hardware:invalid();
 const stages=list(hardware.stages??[],16).map(v=>{
  if(!obj(v))invalid();
  return Object.freeze({
 operation:oneOf(v.operation,['decode','deinterlace','tone_map','subtitle_burn_in','scale','encode'] as const),execution:oneOf(v.execution,['hardware','software'] as const)});
 });
 const streams=list(raw.streams??[],8).map(v=>{
  if(!obj(v)||!text(v.inputCodec,64)||!text(v.outputCodec,64))invalid();
  return Object.freeze({kind:oneOf(v.kind,['video','audio','subtitle'] as const),inputCodec:v.inputCodec,outputCodec:v.outputCodec,action:oneOf(v.action,['copy','convert','original','drop','burn_in','unknown'] as const)});
 });
 const reasonCodes=list(raw.reasonCodes??[],16).map(v=>{if(!text(v,64)||!/^[a-z0-9_]+$/.test(v))invalid();return v as string;});
 return Object.freeze({
  audioRenditions:parseDeliveryAudioRenditions(raw.audioRenditions),
  mode:oneOf(raw.mode,['direct','hls','remote'] as const),
  strategy:oneOf(raw.strategy,['original','copy_remux','audio_conversion','video_conversion'] as const),
  reasonCodes:Object.freeze(reasonCodes),
  policy:raw.policy==null?null:parseResolvedDeliveryPolicy(raw.policy),
  qualityId:raw.qualityId,
  hardware:Object.freeze({backend:oneOf(hardware.backend,['software','videotoolbox','vaapi','qsv','nvenc','amf'] as const),stages:Object.freeze(stages)}),
  streams:Object.freeze(streams),
  throttled:raw.throttled,
  convertedThroughSeconds:raw.convertedThroughSeconds,
  toneMap:raw.toneMap,
  ...(raw.toneMapAlgorithm===undefined?{}:{toneMapAlgorithm:raw.toneMapAlgorithm as string}),
  ...(raw.targetDisplayHeight===undefined?{}:{targetDisplayHeight:raw.targetDisplayHeight as number}),
  ...(raw.targetVideoBitrateBps===undefined?{}:{targetVideoBitrateBps:raw.targetVideoBitrateBps as number}),
  ...(raw.targetAudioBitrateBps===undefined?{}:{targetAudioBitrateBps:raw.targetAudioBitrateBps as number}),
  playedRetentionSeconds:raw.playedRetentionSeconds,
  throttleBufferSeconds:raw.throttleBufferSeconds,
  // Additions are read leniently: an explanation field that does not fit is
  // left out, never a reason to lose the rest.
  ...(()=>{const decision=parseDecisionTrace(raw.decision);return decision?{decision}:{};})(),
  ...(count(raw.audioChannels,64)&&raw.audioChannels>0?{audioChannels:raw.audioChannels}:{}),
  ...(code(raw.downmix)?{downmix:raw.downmix}:{}),
  ...(raw.deinterlace===true?{deinterlace:true}:{}),
  ...(code(raw.outputContainer)?{outputContainer:raw.outputContainer}:{}),
  ...(code(raw.failure)?{failure:raw.failure}:{}),
 });
}

/** The GET /v1/playback/delivery-policy body. */
export type DeliveryPolicyDocument=Readonly<{serverId:string;policy:ResolvedDeliveryPolicy;ladder:readonly Readonly<{id:string;label:string;targetDisplayHeight:number;maxVideoBitrateBps:number;maxAudioBitrateBps:number}>[]}>;

export function parseDeliveryPolicyDocument(raw:unknown):DeliveryPolicyDocument{
 if(!obj(raw)||!text(raw.serverId,256))invalid();
 const ladder=list(raw.ladder??[],16).map(v=>{
  if(!obj(v)||!text(v.id,64)||!text(v.label,128)||!count(v.targetDisplayHeight,4320)||!count(v.maxVideoBitrateBps)||!count(v.maxAudioBitrateBps))invalid();
  return Object.freeze({id:v.id,label:v.label,targetDisplayHeight:v.targetDisplayHeight,maxVideoBitrateBps:v.maxVideoBitrateBps,maxAudioBitrateBps:v.maxAudioBitrateBps});
 });
 return Object.freeze({serverId:raw.serverId,policy:parseResolvedDeliveryPolicy(raw.policy),ladder:Object.freeze(ladder)});
}

/** The header a client uses to declare what it is connected over. The server
 * decides what that means; a declaration can only narrow the resolved lane. */
export const transportClassHeader='X-Portico-Transport-Class';
export const deviceClassHeader='X-Portico-Device-Class';

export function normalizeTransportClass(value:string|null|undefined):TransportClass{
 switch((value??'').trim().toLowerCase()){
  case 'wifi':case 'wi-fi':return 'wifi';
  case 'cellular':case 'mobile':case 'wwan':return 'cellular';
  case 'ethernet':case 'wired':return 'ethernet';
 }
 return 'unknown';
}

/** What the app knows about its surface and its connection, held once for every
 * transport that talks to the selected server. The playback control and queue
 * transports have their own fetch and never saw the API client's copy, so the
 * request that actually plans a session arrived without either header and every
 * per-network and per-device-class preference resolved to "unknown". */
let sharedTransportClass:TransportClass='unknown';
let sharedDeviceClass:''|'web'|'mobile'|'television'='';
export function setDeliveryContext(context:{transportClass?:string|null;deviceClass?:''|'web'|'mobile'|'television'}){
 if(context.transportClass!==undefined)sharedTransportClass=normalizeTransportClass(context.transportClass);
 if(context.deviceClass!==undefined)sharedDeviceClass=context.deviceClass;
}
export function deliveryContextHeaders():Record<string,string>{
 return {...(sharedTransportClass==='unknown'?{}:{[transportClassHeader]:sharedTransportClass}),...(sharedDeviceClass?{[deviceClassHeader]:sharedDeviceClass}:{})};
}
