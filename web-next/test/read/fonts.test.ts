import {test, type TestContext} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {loadReadFonts} from '../../src/screens/read/fonts.ts';

async function environment(t: TestContext) {
  const literata = await readFile(new URL('../../../packages/reader-core/assets/Literata[opsz,wght].ttf', import.meta.url));
  const manrope = await readFile(new URL('../../src/kit/fonts/manrope-var-latin.woff2', import.meta.url));
  const original = {fetch: globalThis.fetch, FontFace: globalThis.FontFace};
  const requests: {url: string; credentials: RequestCredentials | undefined}[] = [], faces = new Set<FontFace>();
  let finish: (() => void) | undefined, hold = false;
  class Face {
    constructor(readonly family: string, readonly bytes: ArrayBuffer) {}
    load() { return hold ? new Promise<Face>(resolve => { finish = () => resolve(this); }) : Promise.resolve(this); }
  }
  globalThis.FontFace = Face as unknown as typeof FontFace;
  globalThis.fetch = async (input, options) => {
    const url = String(input); requests.push({url, credentials: options?.credentials});
    if (url.endsWith('Literata[opsz,wght].ttf')) return new Response(literata);
    if (url.endsWith('manrope-var-latin.woff2')) return new Response(manrope);
    throw new Error('unexpected test asset request');
  };
  t.after(() => Object.assign(globalThis, original));
  const document = {fonts: {add(face: FontFace) { faces.add(face); }, delete(face: FontFace) { return faces.delete(face); }}} as unknown as Document;
  return {document, requests, faces, literata, hold() { hold = true; }, release() { finish?.(); }, get waiting() { return finish !== undefined; }};
}

test('actual main font bytes are verified once, shared with EPUB and owned by the viewer lifetime', async t => {
  const env = await environment(t), life = new AbortController();
  const resource = await loadReadFonts({document: env.document, signal: life.signal}); t.after(resource.dispose);
  assert.equal(env.requests.length, 2); assert.ok(env.requests.every(request => request.credentials === 'omit'));
  assert.equal(resource.fonts.literata.size, 955132); assert.equal(resource.fonts.manrope.size, 23600);
  assert.equal(createHash('sha256').update(new Uint8Array(await resource.fonts.literata.arrayBuffer())).digest('hex'), 'b41138c9373112f32abb589cc22e8674b06ed4048b0c513be922bdd26f274440');
  assert.equal(env.faces.size, 1);
  life.abort(); assert.equal(env.faces.size, 0); resource.dispose();
});

test('a font that finishes loading after retirement is never registered in the app document', async t => {
  const env = await environment(t), life = new AbortController(); env.hold();
  const pending = loadReadFonts({document: env.document, signal: life.signal});
  while (!env.waiting) await new Promise<void>(resolve => setImmediate(resolve));
  life.abort(); env.release(); await assert.rejects(pending);
  assert.equal(env.faces.size, 0);
});
