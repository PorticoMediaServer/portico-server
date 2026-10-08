import {unreadableServerResponse} from './server-messages.ts';
/** Validated server-owned relationship rows. Clients never infer or rank them. */
import {validateContentEntry,type ContentEntry} from './library-content.ts';
export type RelatedMovieRow=Readonly<{id:string;relation:'genre'|'person'|'album'|'artist'|'book'|'author'|'show';provider:string;evidenceId:string;heading:string;entries:readonly ContentEntry[]}>;
export type RelatedMovieProjection=Readonly<{version:1;libraryId:string;libraryName:string;rows:readonly RelatedMovieRow[]}>;
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length>0&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
function invalid():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_related_movies',retryable:false});}
export function validateRelatedMovies(value:unknown,source:{libraryId:string;itemId:string;kind:string}):RelatedMovieProjection{
 if(!object(value)||!['movie','song','audiobook_file','episode'].includes(source.kind)||value.version!==1||value.libraryId!==source.libraryId||!text(value.libraryName)||!Array.isArray(value.rows)||value.rows.length>3)invalid();
 const rows=value.rows.map(raw=>{
  if(!object(raw)||!(source.kind==='movie'?['genre','person']:source.kind==='episode'?['genre','person','show']:source.kind==='song'?['album','artist','genre']:['book','author','genre']).includes(raw.relation as string)||!text(raw.provider,256)||!text(raw.evidenceId,256)||raw.id!==`${raw.relation}:${raw.provider}:${raw.evidenceId}`||!text(raw.heading)||!Array.isArray(raw.entries)||raw.entries.length<1||raw.entries.length>12)invalid();
  const expectedKind=source.kind==='song'?'album':source.kind==='audiobook_file'?'book':source.kind==='episode'?'show':'movie';
  const entries=raw.entries.map(value=>{const entry=validateContentEntry(value);if(entry.kind!==expectedKind||entry.libraryId!==source.libraryId||entry.id===source.itemId||entry.available!==true||entry.navigation?.view!==(['album','book','show'].includes(expectedKind)?expectedKind:'item')||entry.navigation.entityId!==entry.id||entry.playback!==undefined)invalid();return entry;});
  if(new Set(entries.map(e=>e.id)).size!==entries.length)invalid();
  return Object.freeze({id:raw.id as string,relation:raw.relation as RelatedMovieRow['relation'],provider:raw.provider,evidenceId:raw.evidenceId,heading:raw.heading,entries:Object.freeze(entries)});
 });
 if(new Set(rows.map(r=>r.id)).size!==rows.length||rows.filter(r=>r.relation==='genre').length>2||rows.filter(r=>r.relation==='person').length>1)invalid();
 return Object.freeze({version:1,libraryId:source.libraryId,libraryName:value.libraryName,rows:Object.freeze(rows)});
}
