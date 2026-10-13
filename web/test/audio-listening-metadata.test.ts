import test from 'node:test';
import assert from 'node:assert/strict';
import {listeningMetadata} from '../src/bridge/system-listening-metadata.ts';
test('book Media Session exposes author, narrator and chapter without losing the book title',()=>{
 const item={id:'part',title:'Part one',kind:'audiobook_file',bookFile:{bookTitle:'A book',localMetadata:{authors:['An author'],narrators:['A narrator']}}} as any;
 const chapters=[{title:'Opening',startSeconds:0,endSeconds:10},{title:'Second chapter',startSeconds:10,endSeconds:25}];
 assert.deepEqual(listeningMetadata(item,12,chapters),{title:'Second chapter',artist:'An author • A narrator',album:'A book'});
 assert.equal(listeningMetadata(item,30,chapters).title,'Part one');
 assert.equal(listeningMetadata({...item,bookFile:undefined},0).artist,'');
});
