import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createReadingPublication,readerLocationFromReading,readingLocationFromReader} from '../src/index.ts';
import type {ReaderSessionInput} from '../src/index.ts';
import type {ReadingManifest,ReadingProgress} from '@portico/client-core';
const png=new Uint8Array(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mNgAAIAAAUAAarVyFEAAAAASUVORK5CYII=','base64'));
const active=()=>new AbortController().signal;
function fixture(format:'epub'|'pdf'|'cbz') {
  const lifetime=new AbortController(),calls:string[]=[],revision='a'.repeat(64);
  const manifest:ReadingManifest=Object.freeze({sessionId:'session-one',itemId:'item',sourceId:'asset',revision,format,size:100000,metadata:Object.freeze({title:'Catalogue title'}),
    ...format==='epub'?{resources:Object.freeze([{path:'chapter.xhtml',mime:'application/xhtml+xml',size:1}]),spine:Object.freeze(['chapter.xhtml'])}:{},
    ...format==='cbz'?{pageNames:Object.freeze(['page.png'])}:{}});
  const session:ReaderSessionInput={manifest,signal:lifetime.signal,async headPDF(){calls.push('head');},async readRange(range){calls.push('range');return new Uint8Array(range.endExclusive-range.start);},
    async readAsset(){calls.push('asset');return new Uint8Array([1]);},async page(index){calls.push('page');return {index,name:'page.png',bytes:png,mime:'image/png'};}};
  return {session,lifetime,calls};
}

test('PDF publication HEADs once and exposes only captured bounded ranges with owned bytes',async()=>{
  const f=fixture('pdf'),publication=await createReadingPublication(f.session);
  assert.deepEqual(f.calls,['head']);assert.equal(publication.source.id,'asset');assert.equal(publication.source.revision,f.session.manifest.revision);
  assert.equal(publication.source.resources,undefined);assert.equal(publication.source.sequential,undefined);assert.equal(publication.comicPages,undefined);
  assert.deepEqual(publication.metadata,{title:'Catalogue title',direction:undefined});assert.ok(Object.isFrozen(publication.source));
  assert.equal((await publication.source.read!({start:0,endExclusive:65536},active())).length,65536);
  await assert.rejects(publication.source.read!({start:0,endExclusive:65537},active()),{code:'limit_exceeded'});assert.deepEqual(f.calls,['head','range']);
  publication.dispose();await assert.rejects(publication.source.read!({start:0,endExclusive:1},active()),{code:'aborted'});assert.equal(f.lifetime.signal.aborted,false);
});

test('EPUB publication supplies original enumerated assets only and preserves omitted author',async()=>{
  const f=fixture('epub'),publication=await createReadingPublication(f.session);
  assert.equal(f.calls.length,0);assert.equal(publication.source.read,undefined);assert.equal(publication.metadata.author,undefined);
  assert.ok(Object.isFrozen(publication.source.resources!.entries[0]));
  await assert.rejects(publication.source.resources!.read('../missing',active()),{code:'invalid_source'});assert.equal(f.calls.length,0);
  assert.deepEqual(await publication.source.resources!.read('chapter.xhtml',active()),new Uint8Array([1]));assert.deepEqual(f.calls,['asset']);publication.dispose();
});

test('comic publication inspects actual returned variants, never supplies an archive reader, and bounds before I/O',async()=>{
  const f=fixture('cbz'),publication=await createReadingPublication(f.session);
  assert.equal(publication.source.read,undefined);assert.equal(publication.source.resources,undefined);assert.equal(publication.metadata.pageCount,1);
  const provider=await publication.comicPages!(publication.source,active());
  await assert.rejects(provider.page(-1,{width:300,signal:active()}),{code:'invalid_location'});await assert.rejects(provider.page(0,{width:31,signal:active()}),{code:'invalid_location'});assert.equal(f.calls.length,0);
  const page=await provider.page(0,{width:300,signal:active()});assert.deepEqual(page.image,{mime:'image/png',width:1,height:1});assert.notEqual(page.bytes,png);
  await provider.dispose();await assert.rejects(provider.page(0,{width:300,signal:active()}),{code:'aborted'});
  await assert.rejects(publication.comicPages!({...publication.source},active()),{code:'invalid_source'});publication.dispose();
});

test('comic MIME drift and late responses after authority retirement cannot become engine bytes',async()=>{
  const f=fixture('cbz');f.session.page=async index=>({index,name:'page.png',bytes:png,mime:'image/jpeg'});
  const publication=await createReadingPublication(f.session),provider=await publication.comicPages!(publication.source,active());
  await assert.rejects(provider.page(0,{width:300,signal:active()}),{code:'invalid_book'});await provider.dispose();publication.dispose();
  const g=fixture('epub');let finish!:(bytes:Uint8Array)=>void;g.session.readAsset=async()=>new Promise(done=>finish=done);
  const p=await createReadingPublication(g.session),reading=p.source.resources!.read('chapter.xhtml',active());g.lifetime.abort();finish(new Uint8Array([1]));await assert.rejects(reading,{code:'aborted'});p.dispose();
});

test('wire/core locations preserve zero-based facts and omit renderer-only labels',()=>{
  for(const location of [{format:'pdf' as const,page:0,progression:0},{format:'cbz' as const,page:2,progression:0.5},
    {format:'epub' as const,cfi:'epubcfi(/6/2!/4/2)',section:'chapter.xhtml',progression:0.2}]) {
    const wire=readingLocationFromReader({...location,chapter:'Observed chapter',pageLabel:'iii',pagesLeftInSection:2});
    assert.equal('chapter'in wire,false);assert.deepEqual(readerLocationFromReading(wire),location);
  }
  assert.throws(()=>readerLocationFromReading({format:'pdf',pageIndex:0,cfi:'epubcfi(/6)',progression:0}),{code:'invalid_location'});
  assert.throws(()=>readerLocationFromReading({format:'epub',cfi:'not-a-cfi',progression:0}),{code:'invalid_location'});
});

test('resume mapping preserves null and rejects stale/wrong-source progress without creating a saved location',async()=>{
  const f=fixture('pdf'),publication=await createReadingPublication(f.session);
  const progress:ReadingProgress={progress:{sourceId:'asset',sourceRevision:f.session.manifest.revision,revision:1,observedAt:1,stale:false,location:{format:'pdf',pageIndex:0,progression:0}}};
  assert.equal(publication.resumeLocation({progress:null}),null);assert.deepEqual(publication.resumeLocation(progress),{format:'pdf',page:0,progression:0});
  assert.equal(publication.resumeLocation({progress:{...progress.progress!,sourceRevision:'b'.repeat(64),stale:true}}),null);
  assert.throws(()=>publication.resumeLocation({progress:{...progress.progress!,sourceId:'other'}}),{code:'invalid_source'});assert.deepEqual(f.calls,['head']);publication.dispose();
});
