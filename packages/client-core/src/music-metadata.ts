import {unreadableServerResponse} from './server-messages.ts';
/** Bounded music/local-book projections. No provider requests or client ranking. */
export type MusicPolicy = Readonly<{revision:number;localMode:'off'|'supplement'|'prefer';musicBrainzEnabled:boolean;acoustidEnabled:boolean;acoustidConfigured:boolean;fingerprintAllowed:boolean}>;
export type MusicPolicyInput = Pick<MusicPolicy,'localMode'|'musicBrainzEnabled'|'acoustidEnabled'>;
export type MusicMatchObservation = Readonly<{fingerprintStatus:string;providerError:string;confidence:number;margin:number;strongSignals:number;algorithm:string}>;
export type MusicEvidenceDetails = Readonly<{status?:string;packaging?:string;script?:string;language?:string;barcode?:string;credits:readonly Readonly<{id:string;name:string;joinPhrase:string}>[];labels:readonly Readonly<{name:string;catalogNumber:string}>[];isrcs:readonly string[];relations:readonly Readonly<{type:string;name:string;id:string;targetType:string;attributes:readonly string[]}>[]}>;
export type LocalBookMetadata = Readonly<{authors:readonly string[];narrators:readonly string[];series:readonly Readonly<{name:string;position?:string}>[];identifiers:readonly Readonly<{scheme:string;value:string}>[];edition?:string;publisher?:string;language?:string;sources:Readonly<Record<string,string>>}>;
const obj=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const mbid=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(v);
function invalid():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_music_metadata'});}
function list(v:unknown,max=128):unknown[]{if(!Array.isArray(v)||v.length>max)invalid();return v;}
function strings(v:unknown,max=128):readonly string[]{return Object.freeze(list(v,max).map(s=>{if(!text(s))invalid();return s;}));}
export function parseMusicPolicy(v:unknown):MusicPolicy {
 if(!obj(v)||typeof v.revision!=='number'||!Number.isSafeInteger(v.revision)||v.revision<1||!['off','supplement','prefer'].includes(v.localMode as string))invalid();
 for(const k of ['musicBrainzEnabled','acoustidEnabled','acoustidConfigured','fingerprintAllowed'])if(typeof v[k]!=='boolean')invalid();
 return Object.freeze({revision:v.revision,localMode:v.localMode,musicBrainzEnabled:v.musicBrainzEnabled,acoustidEnabled:v.acoustidEnabled,acoustidConfigured:v.acoustidConfigured,fingerprintAllowed:v.fingerprintAllowed}) as MusicPolicy;
}
export function parseMusicObservation(v:unknown):MusicMatchObservation {
 if(!obj(v)||!text(v.fingerprintStatus,64)||!text(v.providerError,64)||!text(v.algorithm,64)||typeof v.strongSignals!=='number'||!Number.isSafeInteger(v.strongSignals)||v.strongSignals<0||v.strongSignals>16)invalid();
 for(const k of ['confidence','margin'])if(typeof v[k]!=='number'||!Number.isFinite(v[k])||(v[k] as number)<0||(v[k] as number)>1)invalid();
 return Object.freeze({fingerprintStatus:v.fingerprintStatus,providerError:v.providerError,confidence:v.confidence,margin:v.margin,strongSignals:v.strongSignals,algorithm:v.algorithm}) as MusicMatchObservation;
}
export function parseMusicDetails(v:unknown):MusicEvidenceDetails {
 if(!obj(v))invalid();const optional:Record<string,string>={};for(const k of ['status','packaging','language','script','barcode'])if(v[k]!==undefined){if(!text(v[k],256))invalid();optional[k]=v[k];}
 const credits=list(v.credits).map(c=>{if(!obj(c)||!obj(c.artist)||!mbid(c.artist.id)||!text(c.artist.name,2048)||!text(c.name,2048)||!text(c.joinphrase,256))invalid();return Object.freeze({id:c.artist.id,name:c.name||c.artist.name,joinPhrase:c.joinphrase});});
 const labels=list(v.labels).map(l=>{if(!obj(l)||!text(l['catalog-number'],512))invalid();if(l.label!==undefined&&l.label!==null&&(!obj(l.label)||!text(l.label.name,2048)||l.label.id!==''&&!mbid(l.label.id)))invalid();return Object.freeze({catalogNumber:l['catalog-number'],name:obj(l.label)?l.label.name as string:''});});
 const relations=list(v.relations).map(r=>{if(!obj(r)||!text(r.type,256)||!text(r['target-type'],256))invalid();let name='',id='';if(r.artist!==undefined){if(!obj(r.artist)||!mbid(r.artist.id)||!text(r.artist.name,2048))invalid();name=r.artist.name;id=r.artist.id;}if(r.work!==undefined){if(!obj(r.work)||!mbid(r.work.id)||!text(r.work.title))invalid();name=r.work.title;id=r.work.id;}return Object.freeze({type:r.type,targetType:r['target-type'],name,id,attributes:strings(r.attributes??[])});});
 const isrcs=strings(v.isrcs);if(isrcs.some(v=>! /^[A-Z]{2}[A-Z0-9]{3}[0-9]{7}$/.test(v)))invalid();
 return Object.freeze({...optional,credits:Object.freeze(credits),labels:Object.freeze(labels),isrcs,relations:Object.freeze(relations)});
}
export function parseLocalBook(v:unknown):LocalBookMetadata {
 if(!obj(v)||!obj(v.sources)||Object.keys(v.sources).length>32)invalid();const sources:Record<string,string>={};for(const [key,value] of Object.entries(v.sources)){if(!text(key,64)||!['embedded','opf','nfo','portico_json'].includes(value as string))invalid();sources[key]=value as string;}
 const fields:Record<string,string>={};for(const k of ['edition','publisher','language'])if(v[k]!==undefined){if(!text(v[k]))invalid();fields[k]=v[k];}
 const series=list(v.series,64).map(s=>{if(!obj(s)||!text(s.name)||s.position!==undefined&&!text(s.position,128))invalid();return Object.freeze({name:s.name,...(s.position!==undefined?{position:s.position as string}:{})});});
 const identifiers=list(v.identifiers,64).map(i=>{if(!obj(i)||!text(i.scheme,64)||!text(i.value,1024))invalid();return Object.freeze({scheme:i.scheme,value:i.value});});
 return Object.freeze({...fields,authors:strings(v.authors),narrators:strings(v.narrators),series:Object.freeze(series),identifiers:Object.freeze(identifiers),sources:Object.freeze(sources)});
}
export const musicEvidenceLabel=(reason:string):string=>({provider_name_search:'Title and artist search',owner_review_required:'Owner review required',automatic_evidence_match:'Automatic evidence match',exact_title:'Title agrees',exact_artist:'Artist agrees',duration_match:'Duration agrees',duration_conflict:'Duration differs',acoustid_recording_match:'Fingerprint supports this recording',isrc_match:'ISRC agrees',isrc_conflict:'ISRC differs',release_year_match:'Release year agrees',release_year_conflict:'Release year differs',barcode_match:'Barcode agrees',barcode_conflict:'Barcode differs',country_match:'Release country agrees',country_conflict:'Release country differs',track_count_match:'Track count agrees',track_count_conflict:'Track count differs',edition_conflict:'Edition differs',local_evidence_conflict:'Local evidence disagrees'}[reason]??reason.replaceAll('_',' '));
export const fingerprintStatusLabel=(status:string):string=>({unsupported_algorithm:'This fingerprint format is not compatible with AcoustID. Other matching evidence is still used.',not_applicable:'Fingerprint matching applies to recordings, not releases.',disabled:'Fingerprint matching is off.',scan_policy_disallows:'Fingerprint matching requires Complete or a Custom scan policy that permits fingerprints.',credential_required:'Configure the server’s AcoustID application key to use fingerprint lookup.',not_available:'No current fingerprint is available. Other matching evidence is still used.',stale_or_invalid:'The fingerprint is stale or invalid. Other matching evidence is still used.',provider_disabled:'AcoustID authentication or terms prevented lookup. Update the server application credential before retrying.',provider_backoff:'AcoustID requested a pause. Retry after the provider cooldown.',provider_unavailable:'AcoustID could not be reached. Other matching evidence is still used.',no_match:'AcoustID returned no supported recording match.',matched:'AcoustID supports one or more recording candidates.',ready:'A current fingerprint is available for lookup.'}[status]??status.replaceAll('_',' '));

export type LocalAudioPolicy=Readonly<{revision:number;localMode:'prefer'|'supplement'|'off'}>;
export function parseLocalAudioPolicy(v:unknown):LocalAudioPolicy {
 if(!obj(v)||!Number.isSafeInteger(v.revision)||Number(v.revision)<1||!['prefer','supplement','off'].includes(v.localMode as string))invalid();
 return Object.freeze({revision:v.revision,localMode:v.localMode}) as LocalAudioPolicy;
}
export async function saveLocalBookPolicy(api:{request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>},itemId:string,libraryId:string,policy:LocalAudioPolicy,localMode:LocalAudioPolicy['localMode'],signal:AbortSignal):Promise<void> {
 parseLocalAudioPolicy({...policy,localMode});
 if(!itemId||!libraryId||itemId.length>256||libraryId.length>256||signal.aborted)throw new Error('Local metadata scope changed.');
 let abort:(()=>void)|undefined;
 try {
 const out=await Promise.race([api.request<unknown>('/v1/items/'+encodeURIComponent(itemId)+'/metadata/local-audio/policy','POST',{libraryId,expectedRevision:policy.revision,localMode},signal),new Promise<never>((_,reject)=>{abort=()=>reject(new Error('Local metadata request cancelled. Refresh to confirm the current policy.'));signal.addEventListener('abort',abort,{once:true});if(signal.aborted)abort();})]);
 if(signal.aborted)throw new Error('Local metadata request cancelled. Refresh to confirm the current policy.');
 if(!obj(out)||out.entityId!==itemId||out.status!=='saved')invalid();
 } finally {if(abort)signal.removeEventListener('abort',abort);}
}
