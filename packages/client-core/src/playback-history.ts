/**
 * Owner playback history (Playback v1 sessions). endedAt and durationSeconds are null while a
 * session is still running: the server records no end time before one
 * exists, and a client must show that as "still playing" rather than as a
 * zero-length watch.
 */
export type PlaybackHistoryPeriod='24h'|'7d'|'30d';
export const PLAYBACK_HISTORY_PERIODS:PlaybackHistoryPeriod[]=['24h','7d','30d'];
export type PlaybackHistoryEntry={
 id:string;viewer:string;authority:string;accountId:string;profileId:string;
 title:string;mediaKind:string;libraryName:string;itemId:string;
 startedAt:number;endedAt:number|null;durationSeconds:number|null;
 deliveryMode:string;deliveryStrategy:string;qualityMode:string;state:string};
export type PlaybackHistoryPage={items:PlaybackHistoryEntry[];nextCursor:string};

const obj=(v:unknown):v is Record<string,any>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const integer=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const text=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length<=max;
function invalid():never{throw new Error('Invalid selected-server playback history response.');}

export function parsePlaybackHistoryEntry(value:unknown):PlaybackHistoryEntry{
 if(!obj(value))invalid();
 if(!text(value.id,200)||!value.id||!integer(value.startedAt))invalid();
 for(const key of ['viewer','authority','accountId','profileId','title','mediaKind','libraryName','itemId','deliveryMode','deliveryStrategy','qualityMode','state'])
  if(!text(value[key],512))invalid();
 for(const key of ['endedAt','durationSeconds'])if(value[key]!==null&&!integer(value[key]))invalid();
 // A recorded end must not precede its start, and a duration must match it.
 if(value.endedAt!==null&&(value.endedAt<value.startedAt||value.durationSeconds===null))invalid();
 if(value.endedAt===null&&value.durationSeconds!==null)invalid();
 return value as PlaybackHistoryEntry;
}

export function parsePlaybackHistory(value:unknown):PlaybackHistoryPage{
 if(!obj(value)||!Array.isArray(value.items)||value.items.length>200||typeof value.nextCursor!=='string'||
  value.nextCursor.length>200)invalid();
 return {items:value.items.map(parsePlaybackHistoryEntry),nextCursor:value.nextCursor};
}
