import {parseMusicTechnicalFacts} from './music-facts.ts';
import type {MediaItem} from './index.ts';
import {wireDurations} from './wire-units.ts';
import {unreadableServerResponse} from './server-messages.ts';
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
export function parseMediaItem(raw:unknown):MediaItem {
 const item=wireDurations(raw,{duration:'durationMs',progressSeconds:'progressMs'});
 if(!object(item)||!['id','kind','title','libraryId'].every(k=>typeof item[k]==='string')||typeof item.available!=='boolean'||typeof item.duration!=='number'||typeof item.progressSeconds!=='number')throw new Error(unreadableServerResponse);
 const audioFacts=item.kind==='song'?parseMusicTechnicalFacts(item.audioFacts):undefined;
 if(audioFacts)item.audioFacts=audioFacts;else delete item.audioFacts;
 // P2-29: an item no longer carries its sources; they are a File info lane of their own.
 delete item.sources;
 return item as unknown as MediaItem;
}
