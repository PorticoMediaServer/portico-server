import {test} from 'node:test';
import assert from 'node:assert/strict';
import {metadataSettingsService,parseMetadataSettings,parseMetadataRefreshStatus} from '../src/metadata-settings.ts';
import {decodeGetMetadataSettingsResponse,decodeGetMetadataRefreshResponse} from '../../contracts/src/decoders.ts';
const settings={revision:7,localArtworkLimitBytes:2**31,localArtworkUsedBytes:0,includeAdult:false,onlineLookups:true,defaultLanguage:'en',defaultRegion:'US',artworkPreference:'folders',artworkLanguage:'same',refreshPolicy:'regular',fallbackLanguage:'en',animeTitles:'language',artworkMaxEdge:1920,castPhotos:true,acoustidKeyConfigured:false,tmdbCredentialConfigured:false,tvdbCredentialConfigured:false,openSubtitlesKeyConfigured:false,openSubtitlesUsername:'',openSubtitlesPasswordConfigured:false,openSubtitles:{keySource:'none',accountSource:'none',account:'none',downloadsRemaining:-1,downloadsAllowed:-1}};
const refresh={queueing:true,queued:32,requestedAt:'2026-10-23T00:00:00Z',enqueuedAt:''};
test('global metadata facade matches generated actual producer and strips secrets',()=>{
 const out=parseMetadataSettings({...settings,acoustidKey:'private',tmdbReadToken:'private'});
 assert.equal('acoustidKey' in out,false);assert.equal('tmdbReadToken' in out,false);assert.equal(out.artworkMaxEdge,1920);
 assert.equal(decodeGetMetadataSettingsResponse(out).defaultLanguage,'en');
 assert.equal(decodeGetMetadataRefreshResponse(parseMetadataRefreshStatus(refresh)).queueing,true);
 for(const bad of [{...settings,onlineLookups:'yes'},{...settings,animeTitles:'auto'},{...settings,artworkMaxEdge:8192},{...settings,fallbackLanguage:'online'},{...settings,defaultLanguage:''},{...settings,defaultRegion:'us'}])assert.throws(()=>parseMetadataSettings(bad));
 assert.throws(()=>parseMetadataRefreshStatus({...refresh,queued:-1}));
});
test('global metadata writes and bounded refresh use actual generated routes',async()=>{
 const calls:unknown[]=[];
 const service=metadataSettingsService(async<T>(path:string,method?:string,body?:unknown)=>{calls.push({path,method,body});return (path==='/v1/admin/metadata/refresh'?refresh:settings) as T;});
 await service.save({expectedRevision:7,localArtworkLimitBytes:2**31,includeAdult:false,defaultLanguage:'fr',acoustidKey:'new-private'});
 await service.refreshAll(7);await service.refreshStatus();
 assert.deepEqual(calls[0],{path:'/v1/admin/metadata/settings',method:'PUT',body:{expectedRevision:7,localArtworkLimitBytes:2**31,includeAdult:false,defaultLanguage:'fr',acoustidKey:'new-private'}});
 assert.deepEqual(calls[1],{path:'/v1/admin/metadata/refresh',method:'POST',body:{expectedRevision:7}});
 assert.deepEqual(calls[2],{path:'/v1/admin/metadata/refresh',method:undefined,body:undefined});
});
