import {unreadableServerResponse} from './server-messages.ts';
export type SessionControls=Readonly<{sessionId:string;revision:string;state:string;positionMs:number;actions:readonly string[]}>;
export type RemotePlaybackControls=SessionControls&Readonly<{device:Readonly<{id:string;name:string}>;item?:Readonly<{id:string;title:string}>;kind:string}>;
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const text=(v:unknown,max=256):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f]/.test(v);
function invalid():never{throw new Error(unreadableServerResponse);}
export function parseSessionControls(v:unknown):SessionControls{
 if(!obj(v)||!text(v.sessionId)||!v.sessionId||!text(v.revision,32)||!/^\d+$/.test(v.revision)||!text(v.state,32)||!Number.isSafeInteger(v.positionMs)||Number(v.positionMs)<0||!Array.isArray(v.actions)||v.actions.length>16)invalid();
 return Object.freeze({sessionId:v.sessionId,revision:v.revision,state:v.state,positionMs:v.positionMs as number,actions:Object.freeze(v.actions.filter((a):a is string=>['pause','resume','seek','stop'].includes(String(a))))});
}
export function parseRemotePlaybackControls(v:unknown):Readonly<{items:readonly RemotePlaybackControls[];nextCursor:string}>{
 if(!obj(v)||!Array.isArray(v.items)||v.items.length>100||!text(v.nextCursor))invalid();
 const items=v.items.map(e=>{const c=parseSessionControls(e);if(!obj(e)||!obj(e.device)||!text(e.device.id)||!text(e.device.name,512)||!text(e.kind,32))invalid();let item:Readonly<{id:string;title:string}>|undefined;if(e.item!==undefined){if(!obj(e.item)||!text(e.item.id)||!text(e.item.title,2048))invalid();item=Object.freeze({id:e.item.id,title:e.item.title});}return Object.freeze({...c,device:Object.freeze({id:e.device.id,name:e.device.name}),kind:e.kind,...(item?{item}:{})});});
 return Object.freeze({items:Object.freeze(items),nextCursor:v.nextCursor});
}
export function playbackControlsService(request:<T>(path:string,method?:string,body?:unknown)=>Promise<T>){
 const path=(id:string)=>'/v1/admin/sessions/'+encodeURIComponent(id)+'/controls';
 return Object.freeze({
 async remote(cursor='',limit=50){return parseRemotePlaybackControls(await request<unknown>(`/v1/playback/controls?cursor=${encodeURIComponent(cursor)}&limit=${limit}`));},
 async readAdmin(id:string){return parseSessionControls(await request<unknown>(path(id)));},
 async controlAdmin(current:SessionControls,action:'pause'|'resume'|'seek',positionMs?:number){return parseSessionControls(await request<unknown>(path(current.sessionId),'POST',{action,expectedRevision:current.revision,...(positionMs===undefined?{}:{positionMs})}));},
 });
}
