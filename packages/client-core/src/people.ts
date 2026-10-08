import {unreadableServerResponse} from './server-messages.ts';
/** Canonical person pages. The server owns person identity, credit membership, ordering and
 * paging; this module only rejects a payload that does not match what it asked for. */
import {validateContentEntry,type ContentEntry,type ContentScope,type LibraryContentApi} from './library-content.ts';
export type PersonRole='cast'|'crew'|'all';
export type Person=Readonly<{id:string;name:string;sortName:string;biography:string;birthDate:string;deathDate:string;portraitUrl:string;roles:readonly string[];knownFor:readonly ContentEntry[];providerIds:Readonly<Record<string,string>>;revision:number}>;
export type PersonCredit=Readonly<{media:ContentEntry;role:string;character:string;department:string;creditKind:'cast'|'crew'}>;
export type PersonPage=Readonly<{serverId:string;viewerFence:string;revision:Readonly<{catalog:number;viewer:number}>;person:Person;credits:readonly PersonCredit[];pageInfo:Readonly<{total:number;nextCursor:string}>}>;
export type PersonSummary=Readonly<{id:string;name:string;sortName:string;portraitUrl:string;roles:readonly string[];creditCount:number}>;
export type PeopleDirectory=Readonly<{serverId:string;viewerFence:string;revision:Readonly<{catalog:number;viewer:number}>;q:string;people:readonly PersonSummary[]}>;
export const MAX_PERSON_CREDITS=100;
export const MAX_KNOWN_FOR=8;
const roles:readonly PersonRole[]=['cast','crew','all'];
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const line=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>line(v,256)&&v.length>0;
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
class Failure extends Error{code:string;constructor(code:string,message:string){super(message);this.code=code;}}
function invalid():never{throw new Failure('invalid_person',unreadableServerResponse);}
function array(v:unknown,max:number):unknown[]{if(!Array.isArray(v)||v.length>max)invalid();return v;}
function labels(v:unknown,max:number):readonly string[]{return Object.freeze(array(v,max).map(x=>{if(!line(x,256)||!x)invalid();return x;}));}
function providers(v:unknown):Readonly<Record<string,string>>{
 if(!object(v))invalid();
 const out:Record<string,string>={};
 const keys=Object.keys(v);
 if(keys.length>16)invalid();
 for(const key of keys){if(!id(key)||!id(v[key]))invalid();out[key]=v[key] as string;}
 return Object.freeze(out);
}
function person(v:unknown):Person{
 if(!object(v)||!id(v.id)||!line(v.name,1024)||!v.name||!line(v.sortName,1024)||!text(v.biography,20000)||!line(v.birthDate,64)||!line(v.deathDate,64)||!line(v.portraitUrl,4096)||!count(v.revision))invalid();
 const knownFor=array(v.knownFor,MAX_KNOWN_FOR).map(validateContentEntry);
 if(new Set(knownFor.map(e=>e.id)).size!==knownFor.length)invalid();
 return Object.freeze({id:v.id,name:v.name,sortName:v.sortName,biography:v.biography,birthDate:v.birthDate,deathDate:v.deathDate,portraitUrl:v.portraitUrl,roles:labels(v.roles,16),knownFor:Object.freeze(knownFor),providerIds:providers(v.providerIds),revision:v.revision});
}
export function parsePersonPage(raw:unknown,scope:ContentScope,request:Readonly<{role?:PersonRole;limit:number;personId:string}>):PersonPage{
 if(!object(raw)||raw.serverId!==scope.serverId||!id(raw.viewerFence)||!object(raw.revision)||!object(raw.pageInfo))invalid();
 if(!count(raw.revision.catalog)||!count(raw.revision.viewer))invalid();
 const subject=person(raw.person);
 if(subject.id!==request.personId)invalid();
 const credits=array(raw.credits,request.limit).map(v=>{
  if(!object(v)||!line(v.role,512)||!line(v.character,512)||!line(v.department,256)||(v.creditKind!=='cast'&&v.creditKind!=='crew'))invalid();
  // The server classifies the credit; a client that guessed from department
  // would disagree with the server's own role filter.
  if(request.role==='cast'&&v.creditKind!=='cast'||request.role==='crew'&&v.creditKind!=='crew')invalid();
  return Object.freeze({media:validateContentEntry(v.media),role:v.role,character:v.character,department:v.department,creditKind:v.creditKind});
 });
 const info=raw.pageInfo;
 if(!count(info.total)||!line(info.nextCursor,4096)||info.total<credits.length)invalid();
 if(info.nextCursor&&credits.length<request.limit)invalid();
 return Object.freeze({serverId:raw.serverId as string,viewerFence:raw.viewerFence,revision:Object.freeze({catalog:raw.revision.catalog,viewer:raw.revision.viewer}),person:subject,credits:Object.freeze(credits),pageInfo:Object.freeze({total:info.total,nextCursor:info.nextCursor})});
}
export function parsePeopleDirectory(raw:unknown,scope:ContentScope,limit:number):PeopleDirectory{
 if(!object(raw)||raw.serverId!==scope.serverId||!id(raw.viewerFence)||!object(raw.revision)||!line(raw.q,512))invalid();
 if(!count(raw.revision.catalog)||!count(raw.revision.viewer))invalid();
 const people=array(raw.people,limit).map(v=>{
  if(!object(v)||!id(v.id)||!line(v.name,1024)||!v.name||!line(v.sortName,1024)||!count(v.creditCount))invalid();
  if(v.portraitUrl!==undefined&&!line(v.portraitUrl,4096))invalid();
  return Object.freeze({id:v.id,name:v.name,sortName:v.sortName,portraitUrl:(v.portraitUrl as string)??'',roles:labels(v.roles,16),creditCount:v.creditCount});
 });
 if(new Set(people.map(p=>p.id)).size!==people.length)invalid();
 return Object.freeze({serverId:raw.serverId as string,viewerFence:raw.viewerFence,revision:Object.freeze({catalog:raw.revision.catalog,viewer:raw.revision.viewer}),q:raw.q,people:Object.freeze(people)});
}
export function personPath(personId:string,options:Readonly<{role?:PersonRole;limit?:number;cursor?:string|null}>={}):string{
 if(!id(personId))throw new Error('A person id is required.');
 const limit=options.limit??40;
 if(!Number.isInteger(limit)||limit<1||limit>MAX_PERSON_CREDITS)throw new Error('Person credits page 1 to 100 rows.');
 if(options.role!==undefined&&!roles.includes(options.role))throw new Error('Unknown credit role.');
 const params=new URLSearchParams({limit:String(limit)});
 if(options.role)params.set('role',options.role);
 if(options.cursor)params.set('cursor',options.cursor);
 return '/v1/people/'+encodeURIComponent(personId)+'?'+params;
}
export function peopleSearchPath(q:string,limit=25):string{
 if(typeof q!=='string'||!q.trim())throw new Error('A person name query is required.');
 if(!Number.isInteger(limit)||limit<1||limit>MAX_PERSON_CREDITS)throw new Error('People search returns 1 to 100 rows.');
 return '/v1/people?'+new URLSearchParams({q:q.split(/\p{White_Space}+/u).filter(Boolean).join(' '),limit:String(limit)});
}
/** The portrait address is server-authored; a client never composes a provider URL. */
export function personPortraitPath(personId:string,size:'thumbnail'|'full'='thumbnail'):string{
 if(!id(personId))throw new Error('A person id is required.');
 return '/v1/people/'+encodeURIComponent(personId)+'/portrait'+(size==='thumbnail'?'?size=thumbnail':'');
}
export async function readPerson(api:LibraryContentApi,scope:ContentScope,personId:string,options:Readonly<{role?:PersonRole;limit?:number;cursor?:string|null;signal?:AbortSignal}>={}):Promise<PersonPage>{
 const limit=options.limit??40;
 const raw=await api.request<unknown>(personPath(personId,{role:options.role,limit,cursor:options.cursor}),'GET',undefined,options.signal);
 return parsePersonPage(raw,scope,{role:options.role,limit,personId});
}
export async function readPeople(api:LibraryContentApi,scope:ContentScope,q:string,limit=25,signal?:AbortSignal):Promise<PeopleDirectory>{
 return parsePeopleDirectory(await api.request<unknown>(peopleSearchPath(q,limit),'GET',undefined,signal),scope,limit);
}
