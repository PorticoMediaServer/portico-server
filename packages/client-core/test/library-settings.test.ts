import test from 'node:test';
import assert from 'node:assert/strict';
import type {LibrarySettings} from '../src/administration.ts';
import {libraryScanDepth,setLibraryScanDepth,setLibrarySeekPreviews,setLibraryPreviewHours} from '../src/server-admin/library-policy.ts';
import {reorderLibraries,saveLibraryFolder} from '../src/server-admin/libraries.ts';

const settings:LibrarySettings={scanDepth:'basic',groupCollections:true,localTags:'prefer',lyricsSource:'files',titleSource:'tags',episodeNumbering:'season',findExtras:true,sortIgnoreArticles:true,refreshMode:'replace_unlocked',acoustidEnabled:false,continueWatching:{weeks:16,maximumItems:40,includeSeasonPremieres:true,videoPlayedThreshold:95,videoCompletion:'earliest'},allowMediaDeletion:false,monitorChanges:true,providers:[],analysis:['probe','local_metadata','subtitle_discovery'],navigation:{trickplayIntervalMs:10000,trickplayTileWidth:320,trickplayMaxTiles:1080,chapterThumbnailMode:'embedded',videoPreviewEnabled:false,videoPreviewMs:30000},segmentDetection:'server'};

test('scan presets are exact and preserve independent picture choices',()=>{
 const pictures={...settings,analysis:[...settings.analysis,'trickplay','chapter_images'],navigation:{...settings.navigation,chapterThumbnailMode:'generated' as const}};
 const complete=setLibraryScanDepth(pictures,'complete');
 assert.equal(libraryScanDepth(complete),'complete');
 assert.deepEqual(new Set(complete.analysis),new Set(['probe','local_metadata','subtitle_discovery','trickplay','chapter_images','waveform','loudness']));
 const files=setLibraryScanDepth(complete,'file_list_only');
 assert.equal(libraryScanDepth(files),'file_list_only');
 assert.deepEqual(new Set(files.analysis),new Set(['probe','trickplay','chapter_images']));
 assert.equal(libraryScanDepth({...settings,analysis:[...settings.analysis,'checksum']}),'custom');
 assert.equal(setLibrarySeekPreviews(settings,'smooth').navigation.trickplayIntervalMs,5000);
 assert.equal(setLibraryPreviewHours(setLibrarySeekPreviews(settings,'smooth'),6).navigation.trickplayMaxTiles,4096);
 assert.equal(setLibraryPreviewHours(settings,3).navigation.trickplayMaxTiles,1080);
});
test('stable library order uses the real route and rejects duplicate identities before transport',async()=>{
 let calls=0;
 const api={async request<T>(path:string,method?:string,body?:unknown):Promise<T>{calls++;assert.equal(path,'/v1/admin/libraries/order');assert.equal(method,'PUT');assert.deepEqual(body,{libraryIds:['second','first']});return body as T;}};
 await reorderLibraries(api,['second','first']);assert.equal(calls,1);
 await assert.rejects(reorderLibraries(api,['first','first']));assert.equal(calls,1);
});
test('folder writes preserve optional label and canonical interval milliseconds',async()=>{
 const api={async request<T>(path:string,method?:string,body?:unknown):Promise<T>{
  assert.equal(path,'/v1/admin/libraries/library/sources');assert.equal(method,'POST');
  assert.deepEqual(body,{name:'',path:'/media',classification:'local',followSymlinks:false,intervalMs:900000,expectedRevision:7,acceptReplacement:false});
  return {id:'source',libraryId:'library',kind:'local',name:'media',path:'/media',resolvedPath:'/media',classification:'local',enabled:true,followSymlinks:false,identityConfirmed:true,intervalMs:900000,generation:1,health:'ready',nextScanAt:'',lastCompleteAt:'',lastProgressAt:''} as T;
 }};
 const source=await saveLibraryFolder(api,'library',undefined,{name:'',path:'/media',classification:'local',followSymlinks:false,intervalSeconds:900,expectedRevision:7,acceptReplacement:false});
 assert.equal(source.intervalSeconds,900);assert.equal(source.name,'media');
});
