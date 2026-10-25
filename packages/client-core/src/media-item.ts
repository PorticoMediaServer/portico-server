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
 if(item.sources!==undefined){
  if(!Array.isArray(item.sources)||item.sources.length>64)throw new Error(unreadableServerResponse);
  item.sources=item.sources.map(source=>wireDurations(source,{duration:'durationMs'}));
 }
 return item as unknown as MediaItem;
}
