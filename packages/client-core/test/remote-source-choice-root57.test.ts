import test from 'node:test';
import assert from 'node:assert/strict';
import {RemoteSourceService} from '../src/remote-sources.ts';
import {sourceChoiceAdopt,sourceChoiceCommand,sourceChoiceComparison,sourceChoiceFields,sourceChoicePutBody,type SourceChoiceBaseline,type SourceChoiceDefinition,type SourceChoiceField} from '../src/remote-source-choice.ts';

const openChoice=(current:()=>boolean=()=>true)=>({signal:new AbortController().signal,isCurrent:current,consentDirty:false as const});
const scope={serverId:'server',viewerId:'owner'},wire={serverId:'server',viewerFence:'fence'};
const reading={scanDepth:'basic',wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0} as const;
const advanced={scanIntervalSeconds:21600,watchForChanges:false,scanOnStart:false,fullRecheckDays:0,cacheBytes:0,cacheFloorBytes:0,seekPreviews:true,chapterImages:true,waveforms:true,loudness:true,checksums:true,introsMode:'inherit',doubleCheckFolders:false,dirCacheSeconds:0,readBufferBytes:0,connectTimeoutSeconds:0,idleTimeoutSeconds:0,retries:0,lowLevelRetries:0} as const;
const names=['list','once','everything','ask'] as const;
const definitions:SourceChoiceDefinition[]=names.map(name=>Object.freeze({name,definitionRevision:1,reading:Object.freeze({...reading}),advanced:Object.freeze({...advanced})}));
const applicable=Object.freeze(sourceChoiceFields.filter(field=>!['dirCacheSeconds','readBufferBytes','connectTimeoutSeconds','idleTimeoutSeconds','retries','lowLevelRetries'].includes(field)) as SourceChoiceField[]);
const currentAdvanced=Object.freeze({...advanced,loudness:false});
const baseline:SourceChoiceBaseline=Object.freeze({sourceId:'dav',generation:'7',configurationGeneration:'11',name:'once',definitionRevision:1,
 readRevision:2,advancedRevision:3,reading:Object.freeze({...reading}),advanced:Object.freeze({...advanced}),currentAdvanced,
 applicableFields:applicable,changedFields:Object.freeze(['loudness'] as SourceChoiceField[])});
const settings=(over:Record<string,unknown>={})=>({sourceId:'dav',generation:'7',revision:2,configurationGeneration:'11',values:{...reading},choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:baseline,...over});
function api(fn:(path:string,method:string,body:unknown)=>Promise<unknown>){return {request:<T>(p:string,m='GET',b?:unknown)=>fn(p,m,b) as Promise<T>};}

test('a coherent baseline compares the actual read facts and same-snapshot Advanced over the applicable set',()=>{
 const result=sourceChoiceComparison(settings());
 assert.deepEqual(result,{name:'once',definitionRevision:1,applicableFields:applicable,changedFields:['loudness']});
 assert.equal(Object.isFrozen(result),true);
 assert.deepEqual(sourceChoiceComparison(settings({values:{...reading,connections:6}})),undefined,'a changed read value without an exact server diff is withheld');
 assert.deepEqual(sourceChoiceComparison(settings({choiceBaseline:{...baseline,changedFields:[]}})),undefined,'a missing changed row is never a synthetic zero');
 assert.deepEqual(sourceChoiceComparison(settings({choiceBaseline:{...baseline,changedFields:['loudness','connections']}})),undefined,'an extra changed row is refused');
});

test('every original witness must match the same snapshot',()=>{
 for(const over of [
  {sourceId:'other'},{generation:'8'},{configurationGeneration:'12'},{revision:3},
  {choiceBaseline:{...baseline,name:'everything'}},{choiceBaseline:{...baseline,definitionRevision:2}},
  {choiceBaseline:{...baseline,applicableFields:applicable.slice(0,-1)}},
  {choiceDefinitions:definitions.map(value=>value.name==='once'?{...value,advanced:{...value.advanced,loudness:false}}:value)},
 ])assert.equal(sourceChoiceComparison(settings(over as never)),undefined,JSON.stringify(over));
 assert.equal(sourceChoiceComparison({...settings(),configurationGeneration:undefined}),undefined);
 assert.equal(sourceChoiceComparison({...settings(),choiceBaseline:undefined}),undefined);
});

test('the read parser keeps the actual facts and adds only supplied choice facts',async()=>{
 const full=new RemoteSourceService(api(async()=>({scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,
  values:{...reading,allowAdvancedMediaAnalysis:true},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:baseline}})),scope,async()=>'op');
 const parsed=(await full.readProfile('dav'));
 assert.equal(parsed.configurationGeneration,'11');
 assert.equal(parsed.values.allowAdvancedMediaAnalysis,true,'actual consent stays published');
 assert.equal(Object.isFrozen(parsed.choiceDefinitions),true);
 assert.deepEqual(parsed.choiceDefinitions?.map(value=>value.name),names);
 assert.deepEqual(parsed.choiceBaseline?.changedFields,['loudness']);
 assert.deepEqual(sourceChoiceComparison(parsed)?.changedFields,['loudness']);
 full.dispose();
 const older=new RemoteSourceService(api(async()=>({scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading,allowAdvancedMediaAnalysis:false}}})),scope,async()=>'op');
 const plain=(await older.readProfile('dav'));
 assert.equal(Object.hasOwn(plain,'configurationGeneration'),false);
 assert.equal(Object.hasOwn(plain,'choiceDefinitions'),false);
 assert.equal(plain.values.allowAdvancedMediaAnalysis,false);
 assert.equal(sourceChoiceComparison(plain),undefined);
 older.dispose();
});

test('malformed choice projections are refused instead of being dropped or defaulted',async()=>{
 const base={scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:reading}};
 for(const extra of [
  {configurationGeneration:'0'},{configurationGeneration:11},
  {choiceDefinitions:definitions.slice(0,3)},
  {choiceDefinitions:[...definitions,{...definitions[0]!}]},
  {choiceDefinitions:definitions.map(value=>value.name==='once'?{...value,reading:{...reading,wholeFileReads:true}}:value)},
  {choiceDefinitions:definitions.map(value=>value.name==='once'?{...value,reading:{...reading,allowAdvancedMediaAnalysis:true}}:value)},
  {choiceApplicableFields:['loudness','loudness']},
  {choiceApplicableFields:['consent']},
  {choiceBaseline:{...baseline,changedFields:['loudness','loudness']}},
  {choiceBaseline:{...baseline,changedFields:['not_a_field']}},
  {choiceBaseline:{...baseline,unknown:true}},
 ]){
  const service=new RemoteSourceService(api(async()=>({...base,settings:{...base.settings,...extra}})),scope,async()=>'op');
  await assert.rejects(()=>service.readProfile('dav'),/Invalid remote source response/);service.dispose();
 }
});

test('an unchanged applied baseline parses zero changes, and a flags-only drift parses its exact row',async()=>{
 const zero=Object.freeze([...[]] as SourceChoiceField[]);
 const unchanged=Object.freeze({...baseline,currentAdvanced:Object.freeze({...advanced}),changedFields:zero});
 const parsed=sourceChoiceComparison(settings({choiceBaseline:unchanged}));
 assert.deepEqual(parsed,{name:'once',definitionRevision:1,applicableFields:applicable,changedFields:[]},'a freshly applied choice is a real zero, not a withheld comparison');
 const flags=Object.freeze({...baseline,currentAdvanced:Object.freeze({...advanced,checksums:false}),changedFields:Object.freeze(['checksums'] as SourceChoiceField[])});
 assert.deepEqual(sourceChoiceComparison(settings({choiceBaseline:flags})),{name:'once',definitionRevision:1,applicableFields:applicable,changedFields:['checksums']});
 assert.equal(sourceChoiceComparison(settings({choiceBaseline:{...flags,changedFields:[]}})),undefined,'actual drift with an empty server set stays withheld');
 const service=new RemoteSourceService(api(async()=>({scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,
  values:reading,configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:unchanged}})),scope,async()=>'op');
 const read=await service.readProfile('dav');
 assert.deepEqual(read.choiceBaseline?.changedFields,[]);
 assert.deepEqual(sourceChoiceComparison(read)?.changedFields,[]);
 service.dispose();
});

test('the retired whole-file and consent flags never add a changed row',async()=>{
 const unchanged=Object.freeze({...baseline,currentAdvanced:Object.freeze({...advanced}),changedFields:Object.freeze([] as SourceChoiceField[])});
 const actual={...reading,wholeFileReads:true,allowAdvancedMediaAnalysis:true};
 const comparison=sourceChoiceComparison({...settings({choiceBaseline:unchanged}),values:actual});
 assert.deepEqual(comparison,{name:'once',definitionRevision:1,applicableFields:applicable,changedFields:[]},'legacy whole-file and independent consent are not choice rows');
 const service=new RemoteSourceService(api(async()=>({scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,
  values:actual,configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:unchanged}})),scope,async()=>'op');
 const read=await service.readProfile('dav');
 assert.equal(read.values.wholeFileReads,true,'the actual compatibility fact stays published');
 assert.equal(read.values.allowAdvancedMediaAnalysis,true,'the actual consent stays published');
 assert.deepEqual(read.choiceBaseline?.changedFields,[]);
 assert.deepEqual(sourceChoiceComparison(read)?.changedFields,[]);
 service.dispose();
});


const advancedSettings=(over:Record<string,unknown>={})=>({sourceId:'dav',generation:'7',revision:0,saved:false,cacheSupported:false,cacheApplied:false,cacheUsedBytes:0,scanIntervalObserved:0,values:{...advanced},...over});
const originalRead=(over:Record<string,unknown>={})=>({sourceId:'dav',generation:'7',revision:0,saved:false,
 values:{...reading,wholeFileReads:true,allowAdvancedMediaAnalysis:true},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,...over});

test('a first explicit choice captures the original read and Advanced pair without a baseline',()=>{
 const offered=definitions[1]!;
 const command=sourceChoiceCommand(originalRead() as never,advancedSettings() as never,offered);
 assert.ok(command);
 assert.equal(command.expectedRevision,0);assert.equal(command.startChoice.expectedAdvancedRevision,0);
 assert.equal(command.startChoice.expectedConfigurationGeneration,'11');assert.equal(command.startChoice.name,'once');
 assert.deepEqual(command.values,{scanDepth:'basic',wholeFileReads:true,connections:4,requestsPerSecond:4,bytesPerSecond:0});
 assert.equal(Object.hasOwn(command.values,'allowAdvancedMediaAnalysis'),false,'consent never enters the wire values');
 assert.equal(command.originalConsent,true);
 assert.deepEqual(command.definition,offered);assert.notEqual(command.definition,offered,'definition is a frozen copy, not an alias');
 assert.equal(Object.isFrozen(command.definition),true);assert.equal(Object.isFrozen(command.definition.reading),true);
 assert.equal(Object.isFrozen(command.originalAdvanced),true);assert.notEqual(command.originalAdvanced,advanced);
 assert.deepEqual(command.applicableFields,applicable);assert.equal(Object.isFrozen(command.applicableFields),true);
 assert.equal(Object.isFrozen(command.startChoice),true);
});

test('consent stays only as optional metadata for true, false and older absence',()=>{
 for(const [consent,want] of [[true,true],[false,false],[undefined,undefined]] as const){
  const values={...reading,wholeFileReads:false,...(consent===undefined?{}:{allowAdvancedMediaAnalysis:consent})};
  const command=sourceChoiceCommand(originalRead({values}) as never,advancedSettings() as never,definitions[1]!,openChoice());
  assert.ok(command);
  assert.equal(Object.hasOwn(command.values,'allowAdvancedMediaAnalysis'),false);
  assert.equal(Object.hasOwn(command.startChoice,'allowAdvancedMediaAnalysis'),false);
  assert.equal(Object.hasOwn(command.definition,'allowAdvancedMediaAnalysis'),false);
  assert.equal(Object.hasOwn(command,'originalConsent'),consent!==undefined);
  if(consent!==undefined)assert.equal(command.originalConsent,want);
 }
});

test('a substituted, mismatched, stale or unsupplied original pair yields no intent',()=>{
 const offered=definitions[1]!;
 for(const over of [
  {sourceId:'other'},{generation:'8'},{configurationGeneration:undefined},{revision:-1},{revision:1.5},
  {choiceApplicableFields:undefined},{choiceApplicableFields:[]},{choiceDefinitions:[]},
 ])assert.equal(sourceChoiceCommand(originalRead(over) as never,advancedSettings() as never,offered),undefined,JSON.stringify(over));
 assert.equal(sourceChoiceCommand(originalRead() as never,advancedSettings({sourceId:'other'}) as never,offered),undefined);
 assert.equal(sourceChoiceCommand(originalRead() as never,advancedSettings({generation:'8'}) as never,offered),undefined);
 assert.equal(sourceChoiceCommand(originalRead() as never,advancedSettings({revision:-1}) as never,offered),undefined);
 assert.equal(sourceChoiceCommand(originalRead() as never,advancedSettings() as never,{...offered} as never),undefined,'a clone is not the offered object');
 assert.equal(sourceChoiceCommand(originalRead() as never,advancedSettings() as never,{...offered,definitionRevision:0} as never),undefined);
 const invalid={...offered,definitionRevision:0} as SourceChoiceDefinition;
 assert.equal(sourceChoiceCommand(originalRead({choiceDefinitions:[definitions[0],invalid,definitions[2],definitions[3]]}) as never,advancedSettings() as never,invalid),undefined,'an invalid published definition cannot be offered');
});

test('input mutation cannot alter the frozen capture',()=>{
 const mk=(name:'list'|'once'|'everything'|'ask'):SourceChoiceDefinition=>({name,definitionRevision:1,reading:{...reading},advanced:{...advanced}} as SourceChoiceDefinition);
 const offered=mk('once');
 const read={sourceId:'dav',generation:'7',revision:0,saved:false,values:{...reading,wholeFileReads:true,allowAdvancedMediaAnalysis:false},
  configurationGeneration:'11',choiceDefinitions:[mk('list'),offered,mk('everything'),mk('ask')],choiceApplicableFields:[...applicable]};
 const advancedIn={sourceId:'dav',generation:'7',revision:0,saved:false,cacheSupported:false,cacheApplied:false,cacheUsedBytes:0,scanIntervalObserved:0,values:{...advanced}};
 const command=sourceChoiceCommand(read as never,advancedIn as never,offered);
 assert.ok(command);
 (offered.reading as {connections:number}).connections=99;(offered.advanced as {loudness:boolean}).loudness=false;
 (read.values as {wholeFileReads:boolean}).wholeFileReads=false;advancedIn.values.loudness=false;read.choiceApplicableFields.push('retries');
 assert.equal(command.definition.reading.connections,4);assert.equal(command.definition.advanced.loudness,true);
 assert.equal(command.values.wholeFileReads,true);assert.equal(command.originalAdvanced.loudness,true);
 assert.equal(command.applicableFields.includes('retries'),false,'a later applicable entry cannot enter the capture');
});

test('the capture is pure, requires real IDs and the complete published definition set',()=>{
 const offered=definitions[1]!;
 const originalFetch=globalThis.fetch,spy=()=>{throw new Error('unexpected I/O')};
 (globalThis as {fetch:unknown}).fetch=spy;
 try{
  const command=sourceChoiceCommand(originalRead() as never,advancedSettings() as never,offered);
  assert.ok(command,'a pure successful capture is not undefined');
 }finally{(globalThis as {fetch:unknown}).fetch=originalFetch;}
 for(const [read,advanced] of [[originalRead({sourceId:''}),advancedSettings()],[originalRead({sourceId:'a'.repeat(161)}),advancedSettings()],
  [originalRead(),advancedSettings({sourceId:''})],[originalRead(),advancedSettings({sourceId:'a'.repeat(161)})]])
  assert.equal(sourceChoiceCommand(read as never,advanced as never,offered),undefined);
 assert.equal(sourceChoiceCommand(originalRead({choiceDefinitions:definitions.slice(0,3).includes(offered)?definitions.slice(0,3):[offered,...definitions.slice(0,2)]}) as never,advancedSettings() as never,offered),undefined,'an incomplete set is not an actual offer');
 assert.equal(sourceChoiceCommand(originalRead({choiceDefinitions:[offered,offered,...definitions.slice(1,3)]}) as never,advancedSettings() as never,offered),undefined,'a duplicate set is not an actual offer');
});

test('getter-backed originals are read once and cannot change the capture',()=>{
 const offered=definitions[1]!;
 const once=<T,>(value:T)=>{let reads=0;return ()=>{if(reads++)throw new Error('re-read');return value;};};
 const getSource=once('dav'),getGeneration=once('7'),getRevision=once(0),getConfiguration=once('11');
 const getAdvancedSource=once('dav'),getAdvancedGeneration=once('7'),getAdvancedRevision=once(0);
 const read={get sourceId(){return getSource();},get generation(){return getGeneration();},get revision(){return getRevision();},
  get configurationGeneration(){return getConfiguration();},values:{...reading,wholeFileReads:true},choiceDefinitions:[offered],choiceApplicableFields:[...applicable]};
 const advancedIn={get sourceId(){return getAdvancedSource();},get generation(){return getAdvancedGeneration();},get revision(){return getAdvancedRevision();},
  saved:false,cacheSupported:false,cacheApplied:false,cacheUsedBytes:0,scanIntervalObserved:0,values:{...advanced}};
 const command=sourceChoiceCommand(read as never,advancedIn as never,offered);
 assert.ok(command,'a value read once is enough');
 assert.equal(command.sourceId,'dav');assert.equal(command.expectedGeneration,'7');assert.equal(command.expectedRevision,0);
 assert.equal(command.startChoice.expectedConfigurationGeneration,'11');assert.equal(command.startChoice.expectedAdvancedRevision,0);
});


test('the atomic body carries only the selected four controls and the original witnesses',()=>{
 const command=sourceChoiceCommand(originalRead() as never,advancedSettings({revision:5}) as never,definitions[1]!,openChoice())!;
 const operationId='1788580000000-12345678-1234-1234-1234-123456789012';
 const body=sourceChoicePutBody(command,operationId);
 assert.deepEqual(Object.keys(body).sort(),['expectedGeneration','expectedRevision','operationId','startChoice','values']);
 assert.deepEqual(body.values,{scanDepth:'basic',wholeFileReads:true,connections:4,requestsPerSecond:4,bytesPerSecond:0});
 assert.equal(Object.hasOwn(body.values,'allowAdvancedMediaAnalysis'),false);
 assert.deepEqual(body.startChoice,{name:'once',definitionRevision:1,expectedAdvancedRevision:5,expectedConfigurationGeneration:'11'});
 assert.equal(body.expectedGeneration,'7');assert.equal(body.expectedRevision,0);
 assert.equal(Object.isFrozen(body),true);
 assert.throws(()=>sourceChoicePutBody(command,'not-an-operation'));
});

test('adoption needs the exact association, both accepted revisions and an empty changed set',()=>{
 const command=sourceChoiceCommand(originalRead() as never,advancedSettings({revision:5}) as never,definitions[1]!,openChoice())!;
 const adoptedBaseline=Object.freeze({...baseline,advancedRevision:5,currentAdvanced:Object.freeze({...advanced}),changedFields:Object.freeze([] as SourceChoiceField[])});
 const adopted=settings({choiceBaseline:adoptedBaseline});
 assert.deepEqual(sourceChoiceAdopt(adopted,command)?.changedFields,[]);
 assert.equal(sourceChoiceAdopt(settings({choiceBaseline:{...adoptedBaseline,advancedRevision:4}}),command),undefined,'a differing accepted Advanced revision is not adopted');
 assert.equal(sourceChoiceAdopt(settings({choiceBaseline:{...adoptedBaseline,name:'everything'}}),command),undefined);
 assert.equal(sourceChoiceAdopt(settings({choiceBaseline:{...adoptedBaseline,changedFields:['loudness']}}),command),undefined);
 assert.equal(sourceChoiceAdopt(settings({generation:'8'}),command),undefined);
});

test('the service sends exactly one atomic PUT and adopts the exact accepted child',async()=>{
 const calls:{path:string;method:string;body:unknown}[]=[];
 const accepted=Object.freeze({...baseline,advancedRevision:5,currentAdvanced:Object.freeze({...advanced}),changedFields:Object.freeze([] as SourceChoiceField[])});
 const api={request:async(path:string,method:string,body:unknown)=>{
  calls.push({path,method,body});
  if(method==='PUT')return {scope:wire,receipt:{operationId:(body as {operationId:string}).operationId,accepted:true,removed:false,source:{id:'dav',kind:'webdav',name:'DAV',rootPath:'/server/remote/dav',generation:7,state:'ready',rangeSupport:'conditional',insecureLocal:false,credentialPresent:true},
   readSettings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading},configurationGeneration:'11'}}};
  return {scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:accepted}};
 }};
 const service=new RemoteSourceService(api as never,scope,async()=>'1788580000000-12345678-1234-1234-1234-123456789012');
 const profile={sourceId:'dav',generation:'7',revision:0,saved:false,values:{...reading,wholeFileReads:true},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable};
 const result=await service.saveProfileStartChoice(profile,advancedSettings({revision:5}) as never,definitions[1]!,openChoice());
 assert.equal(result.kind,'applied');
 assert.equal(calls.filter(call=>call.method==='PUT').length,1);
 assert.deepEqual((calls[0]!.body as {values:object}).values,{scanDepth:'basic',wholeFileReads:true,connections:4,requestsPerSecond:4,bytesPerSecond:0});
 assert.equal(Object.hasOwn((calls[0]!.body as {values:object}).values,'allowAdvancedMediaAnalysis'),false);
 service.dispose();
});

test('a no-op read revision is adopted, while another operation later Advanced revision is not',()=>{
 const command=sourceChoiceCommand(originalRead() as never,advancedSettings({revision:5}) as never,definitions[1]!,openChoice())!;
 const current=Object.freeze({...baseline,advancedRevision:5,currentAdvanced:Object.freeze({...advanced}),changedFields:Object.freeze([] as SourceChoiceField[])});
 assert.deepEqual(sourceChoiceAdopt(settings({choiceBaseline:current}),command)?.changedFields,[]);
 assert.equal(sourceChoiceAdopt(settings({choiceBaseline:{...current,advancedRevision:6}}),command),undefined,'equal values from a later operation are not adopted');
});

test('an uncertain PUT retains the exact intent and recovers only through the source-operation GET',async()=>{
 let op='',putAttempts=0;
 const accepted=Object.freeze({...baseline,advancedRevision:5,currentAdvanced:Object.freeze({...advanced}),changedFields:Object.freeze([] as SourceChoiceField[])});
 const calls:string[]=[];
 const api={request:async(path:string,method:string,body:unknown)=>{
  calls.push(`${method} ${path}`);
  if(method==='PUT'){putAttempts++;op=(body as {operationId:string}).operationId;throw new TypeError('network');}
  if(path.includes('/storage/source-operations/'))return {scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:{id:'dav',kind:'webdav',name:'DAV',rootPath:'/server/remote/dav',generation:7,state:'ready',rangeSupport:'conditional',insecureLocal:false,credentialPresent:true},
   readSettings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading},configurationGeneration:'11'}}};
  return {scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:accepted}};
 }};
 const service=new RemoteSourceService(api as never,scope,async()=>'1788580000000-12345678-1234-1234-1234-123456789012');
 const profile={sourceId:'dav',generation:'7',revision:0,saved:false,values:{...reading,wholeFileReads:true},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable};
 const first=await service.saveProfileStartChoice(profile,advancedSettings({revision:5}) as never,definitions[1]!,openChoice());
 assert.equal(first.kind,'unknown');if(first.kind!=='unknown')return;
 assert.equal(putAttempts,1);
 const recovered=await service.recoverStartChoice(first.intent);
 assert.equal(recovered.kind,'applied');
 assert.deepEqual(calls.filter(call=>call.startsWith('PUT ')).length,1,'recovery never sends another PUT');
 assert.ok(calls.some(call=>call.includes('/storage/source-operations/')));
 service.dispose();
});

test('a retired callback cannot settle or publish the choice',async()=>{
 let current=true;
 const api={request:async()=>{current=false;return {scope:wire,receipt:{operationId:'1788580000000-12345678-1234-1234-1234-123456789012',accepted:true,removed:false,source:{id:'dav',kind:'webdav',name:'DAV',rootPath:'/server/remote/dav',generation:7,state:'ready',rangeSupport:'conditional',insecureLocal:false,credentialPresent:true},
  readSettings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading},configurationGeneration:'11'}}}};}};
 const service=new RemoteSourceService(api as never,scope,async()=>'1788580000000-12345678-1234-1234-1234-123456789012');
 const profile={sourceId:'dav',generation:'7',revision:0,saved:false,values:{...reading,wholeFileReads:true},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable};
 await assert.rejects(()=>service.saveProfileStartChoice(profile,advancedSettings({revision:5}) as never,definitions[1]!,openChoice(()=>current)),error=>Boolean(error&&(error as {code?:string}).code==='retired'));
 service.dispose();
});

test('the fresh read must still be the parent acceptance revision before adopting',async()=>{
 const accepted=Object.freeze({...baseline,advancedRevision:5,currentAdvanced:Object.freeze({...advanced}),changedFields:Object.freeze([] as SourceChoiceField[])});
 const api={request:async(path:string,method:string,body:unknown)=>{
  if(method==='PUT')return {scope:wire,receipt:{operationId:(body as {operationId:string}).operationId,accepted:true,removed:false,source:{id:'dav',kind:'webdav',name:'DAV',rootPath:'/server/remote/dav',generation:7,state:'ready',rangeSupport:'conditional',insecureLocal:false,credentialPresent:true},
   readSettings:{sourceId:'dav',generation:'7',revision:3,saved:true,values:{...reading},configurationGeneration:'11'}}};
  return {scope:wire,settings:{sourceId:'dav',generation:'7',revision:2,saved:true,values:{...reading},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable,choiceBaseline:accepted}};
 }};
 const service=new RemoteSourceService(api as never,scope,async()=>'1788580000000-12345678-1234-1234-1234-123456789012');
 const profile={sourceId:'dav',generation:'7',revision:0,saved:false,values:{...reading,wholeFileReads:true},configurationGeneration:'11',choiceDefinitions:definitions,choiceApplicableFields:applicable};
 const result=await service.saveProfileStartChoice(profile,advancedSettings({revision:5}) as never,definitions[1]!,openChoice());
 assert.equal(result.kind,'accepted-unavailable','a newer read revision is not the accepted parent');
 service.dispose();
});
