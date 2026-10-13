import {unreadableServerResponse} from './server-messages.ts';
/** Validated server-owned relationship rows. Clients never infer or rank them. */
import {validateContentEntry,type ContentEntry} from './library-content.ts';
import {firstOfEach,keepReadable} from './tolerant.ts';
export type RelatedMovieRow=Readonly<{id:string;relation:'genre'|'person'|'album'|'artist'|'book'|'author'|'show';provider:string;evidenceId:string;heading:string;entries:readonly ContentEntry[]}>;
export type RelatedMovieProjection=Readonly<{version:1;libraryId:string;libraryName:string;rows:readonly RelatedMovieRow[]}>;
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=512):v is string=>typeof v==='string'&&v.length>0&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
function invalid():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_related_movies',retryable:false});}
/** Words a row is drawn with: absent reads as empty, and words over their bound refuse what carries them. */
function words(v:unknown):string{if(v===undefined||v===null||v==='')return '';if(!text(v))invalid();return v;}
/**
 * A title's related rows. The version, the library and the bound of 3 rows are exact (a caller that
 * cannot read them shows the title without related rows). A row of a relation this app does not
 * know, or one that cannot be read, is left out alone; so is an entry that is not an available
 * title of the expected kind in this library, and a row with no entry left. A repeated row or
 * entry keeps its first appearance, and at most two genre rows and one person row are shown.
 */
export function validateRelatedMovies(value:unknown,source:{libraryId:string;itemId:string;kind:string}):RelatedMovieProjection{
 if(!object(value)||!['movie','song','audiobook_file','episode'].includes(source.kind)||value.version!==1||value.libraryId!==source.libraryId||!Array.isArray(value.rows)||value.rows.length>3)invalid();
 const relations=source.kind==='movie'?['genre','person']:source.kind==='episode'?['genre','person','show']:source.kind==='song'?['album','artist','genre']:['book','author','genre'];
 const expectedKind=source.kind==='song'?'album':source.kind==='audiobook_file'?'book':source.kind==='episode'?'show':'movie';
 const rows=firstOfEach('related','rows',keepReadable('related','rows',value.rows,raw=>{
  if(!object(raw)||!relations.includes(raw.relation as string)||!text(raw.provider,256)||!text(raw.evidenceId,256)||raw.id!==`${raw.relation}:${raw.provider}:${raw.evidenceId}`||!Array.isArray(raw.entries)||raw.entries.length>12)invalid();
  const entries=firstOfEach('related','entries',keepReadable('related','entries',raw.entries,value=>{const entry=validateContentEntry(value);if(entry.kind!==expectedKind||entry.libraryId!==source.libraryId||entry.id===source.itemId||entry.available!==true||entry.navigation?.view!==(['album','book','show'].includes(expectedKind)?expectedKind:'item')||entry.navigation.entityId!==entry.id||entry.playback!==undefined)invalid();return entry;}),e=>e.id);
  if(!entries.length)return undefined;
  return Object.freeze({id:raw.id as string,relation:raw.relation as RelatedMovieRow['relation'],provider:raw.provider,evidenceId:raw.evidenceId,heading:words(raw.heading),entries:Object.freeze(entries)});
 }),r=>r.id);
 let genres=0,people=0;
 const shown=rows.filter(r=>r.relation==='genre'?++genres<=2:r.relation==='person'?++people<=1:true);
 return Object.freeze({version:1,libraryId:source.libraryId,libraryName:words(value.libraryName),rows:Object.freeze(shown)});
}
