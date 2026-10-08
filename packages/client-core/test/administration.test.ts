import test from 'node:test';
import assert from 'node:assert/strict';
import {AdministrationError,analysisSelectionComplete,backupProgressFraction,deletionAllowed,folderTemplateProblems,folderTemplateTokens,isRestoreRefusalCode,isStatePermissionsAlert,parseAnalysisMatrix,parseBackupsDocument,parseChannelMapPage,parseDVRDocument,parseDeletePreview,parseDeleteResult,parseEnvelope,parseFailure,parseFilesystemPage,parseLibraryDocument,parseLiveSourceDocument,parseRestoreStaged,parseStatePermissionsItem,parseStorageReport,parseTrashPage,parseTunerView,parseUpdateReport} from '../src/administration.ts';

const matrix={costClasses:[{id:'metadata-only',name:'Metadata only',description:'One read.',order:1},{id:'decode',name:'Decodes the video',description:'Expensive.',order:3}],operations:[{id:'probe',name:'Media probe',costClass:'metadata-only',description:'Reads facts.',mediaKinds:['movie'],defaultEnabled:true,requires:null,producesArtifacts:false,network:false},{id:'trickplay',name:'Trickplay tiles',costClass:'decode',description:'Decodes frames.',mediaKinds:['movie'],defaultEnabled:false,requires:['probe'],producesArtifacts:true,network:false}]};
const navigation={trickplayIntervalSeconds:10,trickplayTileWidth:320,trickplayMaxTiles:400,chapterThumbnailMode:'generated',videoPreviewEnabled:false,videoPreviewSeconds:30};
function libraryDocument(overrides:Record<string,unknown>={}){return {libraryId:'lib',libraryName:'Films',libraryKind:'movie',revision:2,digest:'d',settings:{allowMediaDeletion:true,trashRetentionDays:30,providers:[{mediaKind:'movie',provider:'tmdb',apiKey:'',apiKeySet:true,language:'en',region:'GB'}],analysis:['probe','trickplay'],navigation},analysisMatrix:matrix,availableProviders:[{id:'tmdb',name:'TMDB',mediaKinds:['movie'],supportsApiKey:true,supportsLocale:true,attributionNote:'Metadata provided by TMDB.'}],enumerations:{chapterThumbnailMode:['none','embedded','generated']},sources:[{id:'src',name:'Films',root:'/mnt/media/Films',classification:'local',enabled:true}],...overrides};}

test('the envelope is checked and a response from another server is refused',()=>{
 const value=parseEnvelope({protocolVersion:'1.0',serverId:'srv-1',result:libraryDocument()},'srv-1',parseLibraryDocument);
 assert.equal(value.result.libraryName,'Films');
 assert(Object.isFrozen(value.result.sources));
 assert.throws(()=>parseEnvelope({protocolVersion:'1.0',serverId:'srv-2',result:libraryDocument()},'srv-1',parseLibraryDocument),(e:unknown)=>e instanceof AdministrationError&&e.code==='wrong_server');
});

test('a validation refusal keeps the fields the server named',()=>{
 const failure=parseFailure({error:{code:'invalid_administration_input',message:'invalid values: settings.folderTemplate',retryable:false,fields:['settings.folderTemplate']}});
 assert.deepEqual([...failure.fields],['settings.folderTemplate']);
 assert.equal(parseFailure('nonsense').code,'request_failed');
});

test('a returned provider API key is refused rather than held',()=>{
 const leaking=libraryDocument({settings:{allowMediaDeletion:true,trashRetentionDays:30,providers:[{mediaKind:'movie',provider:'tmdb',apiKey:'secret',apiKeySet:true,language:'en',region:'GB'}],analysis:['probe'],navigation}});
 assert.throws(()=>parseLibraryDocument(leaking),(e:unknown)=>e instanceof AdministrationError&&e.code==='provider_key_disclosed');
});

test('the analysis matrix must publish a cost class for every operation, and dependencies are reported',()=>{
 const parsed=parseAnalysisMatrix(matrix);
 assert.equal(parsed.operations[0].requires.length,0);
 assert.deepEqual([...analysisSelectionComplete(parsed,['trickplay'])],['probe']);
 assert.deepEqual([...analysisSelectionComplete(parsed,['probe','trickplay'])],[]);
 assert.throws(()=>parseAnalysisMatrix({...matrix,operations:[{...matrix.operations[1],costClass:'network'}]}));
});

test('a delete preview confirms by title for one target and by count for several',()=>{
 const target={itemId:'item-1',title:'The Lighthouse',kind:'movie',libraryId:'lib',libraryName:'Films',allowMediaDeletion:true,trashRetentionDays:30,bytes:10,files:[{assetId:'a',path:'/media/one.mkv',bytes:10,present:true,shared:false}],dependents:[{kind:'collection',count:1,description:'collections that list this item'}]};
 const single=parseDeletePreview({revision:7,confirmation:'The Lighthouse',confirmationKind:'title',totalBytes:10,totalFiles:1,totalBytesText:'10 B',blocked:[],trashRetentionDays:30,targets:[target]});
 assert.equal(single.confirmation,'The Lighthouse');
 assert(deletionAllowed(single));
 const bulk=parseDeletePreview({revision:7,confirmation:'2',confirmationKind:'count',totalBytes:20,totalFiles:2,totalBytesText:'20 B',blocked:['other'],trashRetentionDays:14,targets:[target,{...target,itemId:'item-2'}]});
 assert.equal(deletionAllowed(bulk),false);
 // A count that does not match the targets would prompt for the wrong text.
 assert.throws(()=>parseDeletePreview({revision:7,confirmation:'3',confirmationKind:'count',totalBytes:20,totalFiles:2,totalBytesText:'20 B',blocked:[],trashRetentionDays:14,targets:[target,{...target,itemId:'item-2'}]}));
 assert.throws(()=>parseDeletePreview({revision:7,confirmation:'The Lighthouse',confirmationKind:'title',totalBytes:20,totalFiles:2,totalBytesText:'20 B',blocked:[],trashRetentionDays:14,targets:[target,{...target,itemId:'item-2'}]}));
});

test('a delete result whose receipts disagree with its counts is refused',()=>{
 const receipt={itemId:'item-1',title:'The Lighthouse',removed:true,trashEntryId:'entry',filesMoved:2,filesKept:0,bytes:10,code:''};
 const result=parseDeleteResult({revision:8,removed:1,failed:0,bytesTrashed:10,filesRetained:false,receipts:[receipt]});
 assert.equal(result.receipts[0].trashEntryId,'entry');
 assert.throws(()=>parseDeleteResult({revision:8,removed:2,failed:0,bytesTrashed:10,filesRetained:false,receipts:[receipt]}));
});

test('the trash page and the folder picker parse into frozen projections',()=>{
 const trash=parseTrashPage({items:[{id:'entry',libraryId:'lib',itemId:'item-1',title:'The Lighthouse',kind:'movie',fileCount:1,bytes:10,bytesText:'10 B',trashedAt:'2026-09-16T20:11:04Z',expiresAt:'2026-10-16T20:11:04Z',state:'held',expired:false,files:[{originalPath:'/media/one.mkv',bytes:10}]}],nextCursor:'',heldBytes:10,heldCount:1});
 assert.equal(trash.items[0].state,'held');
 assert(Object.isFrozen(trash.items));
 const page=parseFilesystemPage({path:'/mnt/media',parent:'/mnt',separator:'/',platform:'linux',writable:true,truncated:false,roots:[{path:'/',name:'/',kind:'root',description:'The root filesystem'}],entries:[{name:'Films',path:'/mnt/media/Films',kind:'directory',readable:true,symlink:false}],nextCursor:'next'});
 assert.equal(page.entries[0].bytes,0);
 assert.equal(page.nextCursor,'next');
  assert.throws(()=>parseFilesystemPage({...page,entries:[{name:'x',path:'/x',kind:'socket',readable:true,symlink:false}]}));
});

test('CD-43: a capped 64-root page parses as truncated; a 65th root still refuses',()=>{
  const root=(n:number)=>({path:`/mnt/d${n}`,name:`d${n}`,kind:'media',description:`Dataset ${n}`});
  const base={path:'/',parent:'',separator:'/',platform:'linux',writable:true,entries:[],nextCursor:''};
  const page=parseFilesystemPage({...base,truncated:true,roots:Array.from({length:64},(_,n)=>root(n))});
  assert.equal(page.roots.length,64);
  assert.equal(page.truncated,true);
  assert.throws(()=>parseFilesystemPage({...base,truncated:true,roots:Array.from({length:65},(_,n)=>root(n))}));
});

test('a guide-only live source may not carry stream tuning or a guide attachment',()=>{
 const base={sourceId:'src',sourceName:'Rooftop aerial',revision:3,digest:'d',tunerCount:2,availableKinds:['playlist','xmltv-guide','hdhomerun'],settings:{kind:'playlist',streamBufferSeconds:12,retryWindowSeconds:null,userAgent:'',guideSourceId:'src-guide',filters:{categories:{mode:'include',values:['News']},countries:{mode:'include',values:[]},keywords:{mode:'exclude',values:['shopping']}},logoImport:{enabled:true,overwriteExisting:false},channelNumbering:{mode:'sequential',startAt:100,step:1}},effective:{streamBufferSeconds:12,retryWindowSeconds:60,userAgent:'Portico/1.0',guideDays:7,logoImport:true,discoveryEnabled:true}};
 const parsed=parseLiveSourceDocument(base);
 assert.equal(parsed.settings.streamBufferSeconds,12);
 assert.equal(parsed.settings.retryWindowSeconds,null);
 assert.throws(()=>parseLiveSourceDocument({...base,settings:{...base.settings,kind:'xmltv-guide'}}));
 const guide=parseLiveSourceDocument({...base,settings:{...base.settings,kind:'xmltv-guide',streamBufferSeconds:null,guideSourceId:''}});
 assert.equal(guide.settings.kind,'xmltv-guide');
});

test('a channel map that serves two channels on one number is refused',()=>{
 const entry={channelId:'bbc-two',name:'BBC Two',sourceNumber:'2',number:'102',group:'Terrestrial',guideChannelId:'bbctwo.uk',hidden:false,position:1,overridden:true};
 const page=parseChannelMapPage({sourceId:'src',generation:'gen-1',items:[entry,{...entry,channelId:'bbc-four',number:'104',position:2}],nextCursor:''});
 assert.equal(page.items.length,2);
 assert.throws(()=>parseChannelMapPage({sourceId:'src',generation:'gen-1',items:[entry,{...entry,channelId:'bbc-four',position:2}],nextCursor:''}));
 // A hidden channel does not claim its number.
 assert.equal(parseChannelMapPage({sourceId:'src',generation:'gen-1',items:[entry,{...entry,channelId:'bbc-four',position:2,hidden:true}],nextCursor:''}).items.length,2);
});

test('the DVR document refuses deleting the original with no conversion, and templates are checked before a write',()=>{
 const settings={prePaddingSeconds:60,postPaddingSeconds:180,keepPolicy:{mode:'keep-count',keepCount:5,keepDays:0,keepUntilWatched:true},conversion:{mode:'remux',ladderId:'',deleteOriginal:false},folderTemplate:'{series}/Season {season:02}/{series} - S{season:02}E{episode:02} - {title}',movieFolderTemplate:'{title} ({year})/{title} ({year})',recordingProfile:'original',sidecars:{nfo:true,poster:true,fanart:false,thumbnail:true},tunerPreference:'least-busy'};
 const tokens=[{token:'{series}',description:'Series title.',example:'Nova',supportsPadding:false},{token:'{season}',description:'Season number.',example:'3',supportsPadding:true},{token:'{episode}',description:'Episode number.',example:'7',supportsPadding:true},{token:'{title}',description:'Title.',example:'The Planets',supportsPadding:false},{token:'{year}',description:'Year.',example:'2024',supportsPadding:false}];
 const document=parseDVRDocument({revision:1,digest:'d',settings,folderTokens:tokens,enumerations:{keepMode:['keep-all','keep-count','keep-days']}});
 assert.equal(document.settings.keepPolicy.keepCount,5);
 assert.throws(()=>parseDVRDocument({revision:1,digest:'d',settings:{...settings,conversion:{mode:'none',ladderId:'',deleteOriginal:true}},folderTokens:tokens,enumerations:{}}));
 assert.deepEqual([...folderTemplateTokens('{series}/{title} {season:02}')],['{season}','{series}','{title}']);
 assert.deepEqual([...folderTemplateProblems(settings.folderTemplate,document.folderTokens)],[]);
 assert.deepEqual([...folderTemplateProblems('{series}/{unknown}',document.folderTokens)],['Unknown token {unknown}.','A template must include {title}, {episode} or {date} so recordings do not collide.']);
 assert(folderTemplateProblems('../{title}',document.folderTokens).some(p=>/escape/.test(p)));
 assert(folderTemplateProblems('{title:02}',document.folderTokens).some(p=>/cannot be padded/.test(p)));
});

test('the tuner view must agree with its own in-use count',()=>{
 const assignment={recordingId:'rec-1',channelId:'bbc-two',title:'Nova',state:'recording',startsAt:'2026-09-16T20:00:00Z',endsAt:'2026-09-16T21:00:00Z',allocationId:'alloc-1'};
 const view=parseTunerView({observedAt:'2026-09-16T20:11:04Z',upcomingWindowHours:48,sources:[{sourceId:'src',sourceName:'Rooftop aerial',tunerCount:2,inUse:1,conflicts:1,active:[assignment],upcoming:[{...assignment,recordingId:'rec-2',state:'scheduled',allocationId:''}]}]});
 assert.equal(view.sources[0].conflicts,1);
 assert.throws(()=>parseTunerView({observedAt:'',upcomingWindowHours:48,sources:[{sourceId:'src',sourceName:'x',tunerCount:2,inUse:2,conflicts:0,active:[assignment],upcoming:[]}]}));
});

test('storage and updates parse, and a server claiming auto-install is refused',()=>{
 const report=parseStorageReport({stateDirectory:'/var/lib/portico',totalBytes:100,totalBytesText:'100 B',observedAt:'2026-09-16T20:11:04Z',truncated:false,categories:[{id:'trash',name:'Trash',description:'Held files.',directories:['trash'],cleanable:true,regenerable:false,retentionKey:'trash',bytes:100,bytesText:'100 B',fileCount:2,paths:['/var/lib/portico/trash'],present:true,oldestDays:0,entries:1}]});
 assert.equal(report.categories[0].entries,1);
 const current={version:'1.2.0',buildId:'b1',sourceDigest:'d',builtAt:''};
 const update=parseUpdateReport({current,channel:'stable',feedConfigured:true,latest:{...current,version:'1.3.0',notes:'Faster scans'},updateAvailable:true,checkedAt:'2026-09-16T20:11:04Z',status:'update-available',message:'',autoInstall:false});
 assert.equal(update.latest?.version,'1.3.0');
 assert.throws(()=>parseUpdateReport({current,channel:'stable',feedConfigured:true,latest:null,updateAvailable:true,checkedAt:'',status:'update-available',message:'',autoInstall:false}));
  assert.throws(()=>parseUpdateReport({current,channel:'stable',feedConfigured:false,latest:null,updateAvailable:false,checkedAt:'',status:'no-feed',message:'',autoInstall:true}),(e:unknown)=>e instanceof AdministrationError&&e.code==='unexpected_auto_install');
});

test('the Plex-model backups document parses; unknown kinds skip the item, malformed optionals drop',()=>{
  const backup={id:'2026-09-24T013000Z',createdAt:'2026-09-24T01:30:00Z',kind:'manual',schemaVersion:41,serverVersion:'1.2.0',bytes:120,path:'/var/lib/portico/backups/2026-09-24T013000Z'};
  const doc=parseBackupsDocument({backups:[backup,{...backup,id:'other',kind:'encrypted-chunks'},],running:{jobId:'job-1',phase:'copying',bytesDone:60,bytesTotal:120},lastRestore:{at:'2026-09-24T02:00:00Z',outcome:'rolled_back',reason:'integrity check failed'}});
  assert.equal(doc.backups.length,1);
  assert.equal(doc.backups[0].kind,'manual');
  assert(Object.isFrozen(doc.backups));
  assert.equal(doc.running?.phase,'copying');
  assert.equal(doc.lastRestore?.outcome,'rolled_back');
  assert.equal(backupProgressFraction(doc.running!),(60/120));
  assert.equal(backupProgressFraction({jobId:'j',phase:'p',bytesDone:5,bytesTotal:0}),undefined);
  // A malformed running block or an unknown restore outcome degrades to absent, never fatal.
  const degraded=parseBackupsDocument({backups:[backup],running:{jobId:'j'},lastRestore:{at:'t',outcome:'migrated'}});
  assert.equal(degraded.running,undefined);
  assert.equal(degraded.lastRestore,undefined);
  assert.equal(parseBackupsDocument({backups:[]}).backups.length,0);
  assert.throws(()=>parseBackupsDocument({}),{code:'invalid_administration_response'});
  assert.throws(()=>parseBackupsDocument({backups:[{...backup,bytes:-1}]}),{code:'invalid_administration_response'});
});

test('the restore receipt is strict and the refusal codes are recognised',()=>{
  const staged=parseRestoreStaged({staged:true,restartRequired:true,validation:{schemaVersion:41,integrity:'ok'}});
  assert.equal(staged.validation.schemaVersion,41);
  for(const raw of [null,{},{staged:true},{staged:true,restartRequired:true},{staged:true,restartRequired:true,validation:{schemaVersion:41,integrity:'failed'}},{staged:true,restartRequired:false,validation:{schemaVersion:41,integrity:'ok'}}])assert.throws(()=>parseRestoreStaged(raw),{code:'invalid_administration_response'});
  for(const code of ['restore_source_invalid','restore_schema_newer','restore_pre_release','restore_integrity_failed','restore_not_portico','insufficient_disk'])assert(isRestoreRefusalCode(code));
  assert.equal(isRestoreRefusalCode('request_failed'),false);
});

test('the state-permissions item parses and only live items match',()=>{
  const item=parseStatePermissionsItem({code:'state-permissions',id:'alert-1',revision:2,severity:'warning',status:'open',firstAt:10,lastAt:20,occurrences:3});
  assert.equal(item.id,'alert-1');
  assert(isStatePermissionsAlert({code:'state-permissions',status:'open'}));
  assert(isStatePermissionsAlert({code:'state-permissions',status:'acknowledged'}));
  assert.equal(isStatePermissionsAlert({code:'state-permissions',status:'resolved'}),false);
  assert.equal(isStatePermissionsAlert({code:'low_storage',status:'open'}),false);
  assert.throws(()=>parseStatePermissionsItem({...item,code:'low_storage'}),{code:'invalid_administration_response'});
});

test('current DVR defaults accept unavailable templates as empty and retain the supported settings',()=>{
 const settings={prePaddingSeconds:60,postPaddingSeconds:180,keepPolicy:{mode:'keep-all',keepCount:0,keepDays:0,keepUntilWatched:false},conversion:{mode:'none',ladderId:'',deleteOriginal:false},folderTemplate:'',movieFolderTemplate:'',recordingProfile:'original',sidecars:{nfo:false,poster:false,fanart:false,thumbnail:false},tunerPreference:'first-available'};
 const wire={revision:1,digest:'current',settings,folderTokens:[],enumerations:{keepMode:['keep-all','keep-count','keep-days'],conversionMode:['none'],recordingProfile:['original'],tunerPreference:['first-available']}};
 const document=parseDVRDocument(wire);
 assert.deepEqual(document.settings,settings);
 assert.deepEqual(document.folderTokens,[]);
 for(const bad of [undefined,null,42,'x'.repeat(401)])assert.throws(()=>parseDVRDocument({...wire,settings:{...settings,folderTemplate:bad}}),{code:'invalid_administration_response'});
});

// APL-SYS-12 (parser probe, 24 Sep): the runner e2e server's GET /v1/admin/updates answers this
// bare report (a registry route, no administration envelope); the web reads it directly.
test('the updates report as the server sends it parses', () => {
  const report = parseUpdateReport({current: {version: '0.1.0-dev', buildId: 'development', sourceDigest: 'unsealed', builtAt: '', channel: 'stable'}, channel: 'stable', feedConfigured: false, state: 'unconfigured', updateAvailable: false, checkedAt: '', status: 'no-feed', message: 'No release feed is configured, so this server only reports the build it is running.', autoInstall: false});
  assert.equal(report.status, 'no-feed');
  assert.equal(report.current.version, '0.1.0-dev');
});
