import test from 'node:test';
import assert from 'node:assert/strict';
import {RemoteSourceService} from '../src/remote-sources.ts';
import {sourceChoiceComparison,sourceChoiceFields,type SourceChoiceBaseline,type SourceChoiceDefinition,type SourceChoiceField} from '../src/remote-source-choice.ts';

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
