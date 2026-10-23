import assert from 'node:assert/strict';
import {test} from 'node:test';
import {ZipWriter, Uint8ArrayWriter, TextReader} from '@zip.js/zip.js';
import {openEpubArchive,safeArchivePath} from '../src/epub/archive.ts';
import {sourceFromBlob} from '../src/source.ts';
const container='<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="book.opf" media-type="application/oebps-package+xml"/></rootfiles></container>';
async function zip(entries:Record<string,string>,password?:string) {
  const writer=new ZipWriter(new Uint8ArrayWriter(),{useWebWorkers:false});
  await writer.add('mimetype',new TextReader('application/epub+zip'),{level:0});
  await writer.add('META-INF/container.xml',new TextReader(container));
  for(const [path,text]of Object.entries(entries))await writer.add(path,new TextReader(text),{password});
  return await writer.close();
}
test('EPUB ZIP range loader checks actual CRC/content and closes/cancels deterministically',async()=>{
  const bytes=await zip({'chapter.xhtml':'<html xmlns="http://www.w3.org/1999/xhtml"><body>Actual test content</body></html>'});
  const source=sourceFromBlob(new Blob([bytes]),'local-test','revision-1');
  const abort=new AbortController();const archive=await openEpubArchive(source,abort.signal);
  assert.match(new TextDecoder().decode(await archive.read('chapter.xhtml',abort.signal)),/Actual test content/);
  assert.deepEqual([...await archive.read('missing.xhtml',abort.signal)],[]);
  await archive.dispose();await assert.rejects(archive.read('chapter.xhtml',abort.signal));
});
test('EPUB rejects traversal/aliases, encrypted entries, truncated ZIP and declared bombs',async()=>{
  for(const path of ['../outside','/absolute','a\\b','a//b','a/./b','a/%2e%2e/b','a?token=x'])assert.throws(()=>safeArchivePath(path));
  const hostile=await zip({'../outside':'unsafe'});
  await assert.rejects(openEpubArchive(sourceFromBlob(new Blob([hostile]),'test','1'),new AbortController().signal));
  const encrypted=await zip({'secret.xhtml':'secret'},'test-only-password');
  await assert.rejects(openEpubArchive(sourceFromBlob(new Blob([encrypted]),'test','1'),new AbortController().signal),{code:'protected'});
  const bytes=await zip({'chapter.xhtml':'text'});
  await assert.rejects(openEpubArchive(sourceFromBlob(new Blob([bytes.subarray(0,bytes.length-2)]),'test','1'),new AbortController().signal));
  const bomb=await zip({'bomb.xhtml':'a'.repeat(1024*1024)});
  await assert.rejects(openEpubArchive(sourceFromBlob(new Blob([bomb]),'test','1'),new AbortController().signal),{code:'limit_exceeded'});
});
