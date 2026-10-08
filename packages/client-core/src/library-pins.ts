import {unreadableServerResponse} from './server-messages.ts';
/** Pinned libraries and pinned-resource order. The arrangement is server state:
 * clients read it, send a replacement with the revision they saw, and re-read.
 * Nothing here re-orders locally or remembers a pin the server did not confirm. */
export type PinnedResourceKind='collection'|'view'|'playlist';
export type PinOrderEntry=Readonly<{kind:PinnedResourceKind;id:string}>;
export type PinOrder=Readonly<{serverId:string;viewerFence:string;revision:number;order:readonly PinOrderEntry[]}>;
export type LibraryNavigationPins=Readonly<{pinnedLibraryIds:readonly string[];revision:number}>;
export type PinnedLibrary=Readonly<{id:string;name:string;kind:string;defaultView:string;pinned:boolean}>;

const obj=(v:unknown):v is Record<string,unknown>=>v!==null&&typeof v==='object'&&!Array.isArray(v);
const str=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>str(v,256)&&v.length>0&&!/[\r\n]/.test(v);
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const kinds:readonly string[]=['collection','view','playlist'];
function bad(message=unreadableServerResponse):never{throw Object.assign(new Error(message),{code:'invalid_pins'});}
function rows(v:unknown,max:number):unknown[]{if(!Array.isArray(v)||v.length>max)bad();return v;}
function freeze<T>(v:T):T{if(v&&typeof v==='object'){for(const child of Object.values(v))freeze(child);Object.freeze(v);}return v;}

export function parsePinOrder(raw:unknown,serverId:string):PinOrder{
 if(!obj(raw)||raw.serverId!==serverId||!id(raw.viewerFence)||!count(raw.revision))bad();
 const order=rows(raw.order,200).map(v=>{
  if(!obj(v)||!id(v.id)||!str(v.kind,32)||!kinds.includes(v.kind))bad();
  return {kind:v.kind as PinnedResourceKind,id:v.id};
 });
 if(new Set(order.map(entry=>entry.kind+':'+entry.id)).size!==order.length)bad();
 return freeze({serverId,viewerFence:raw.viewerFence,revision:raw.revision,order});
}

export function parseLibraryNavigationPins(raw:unknown):LibraryNavigationPins{
 if(!obj(raw)||!count(raw.revision))bad();
 const pinnedLibraryIds=rows(raw.pinnedLibraryIds,100).map(v=>{if(!id(v))bad();return v;});
 if(new Set(pinnedLibraryIds).size!==pinnedLibraryIds.length)bad();
 return freeze({pinnedLibraryIds,revision:raw.revision});
}

/** The listing is already in the viewer's order; a client that re-sorts it will
 * disagree with every other surface, so this only checks that it is. */
export function parsePinnedLibraries(raw:unknown):readonly PinnedLibrary[]{
 if(!obj(raw))bad();
 const items=rows(raw.items,2048).map(v=>{
  if(!obj(v)||!id(v.id)||!str(v.name,512)||!id(v.kind)||!str(v.defaultView,64)||typeof v.pinned!=='boolean')bad();
  return {id:v.id,name:v.name,kind:v.kind,defaultView:v.defaultView,pinned:v.pinned};
 });
 if(new Set(items.map(item=>item.id)).size!==items.length)bad();
 if(items.some((item,index)=>item.pinned&&index>0&&!items[index-1].pinned))bad();
 return freeze(items);
}

/** Builds the PUT body for a replacement arrangement. */
export function pinOrderRequest(expectedRevision:number,order:readonly PinOrderEntry[]):Readonly<{expectedRevision:number;order:readonly PinOrderEntry[]}>{
 if(!count(expectedRevision)||order.length>200)bad('That pin arrangement cannot be saved.');
 const seen=new Set<string>();
 for(const entry of order){
  if(!kinds.includes(entry.kind)||!id(entry.id)||seen.has(entry.kind+':'+entry.id))bad('That pin arrangement cannot be saved.');
  seen.add(entry.kind+':'+entry.id);
 }
 return freeze({expectedRevision,order:[...order]});
}

export function libraryNavigationRequest(expectedRevision:number,pinnedLibraryIds:readonly string[]):Readonly<{expectedRevision:number;pinnedLibraryIds:readonly string[]}>{
 if(!count(expectedRevision)||pinnedLibraryIds.length>100||new Set(pinnedLibraryIds).size!==pinnedLibraryIds.length||pinnedLibraryIds.some(entry=>!id(entry)))bad('Those library pins cannot be saved.');
 return freeze({expectedRevision,pinnedLibraryIds:[...pinnedLibraryIds]});
}
