import {ReaderError,throwIfAborted} from './errors.ts';
/** Bundlers rewrite this trusted app asset to a hashed same-origin file. */
export const literataFontUrl = new URL('../assets/Literata[opsz,wght].ttf', import.meta.url).href;
export const LITERATA_FONT_SHA256 = 'b41138c9373112f32abb589cc22e8674b06ed4048b0c513be922bdd26f274440';
/** One Blob can supply the app FontFace and every open-book engine. This
 * loader never uses the selected-server client, cookies or book-supplied URLs. */
export async function loadLiterataFont(options: Readonly<{signal?: AbortSignal}> = {}): Promise<Blob> {
  const signal=options.signal??new AbortController().signal;throwIfAborted(signal);
  const response=await fetch(literataFontUrl,{credentials:'omit',signal});
  if(!response.ok||!response.body)throw new ReaderError('source_unavailable');
  const reader=response.body.getReader(),chunks:Uint8Array<ArrayBuffer>[]=[],expected=955132;let size=0;
  try {
    for(;;) {throwIfAborted(signal);const next=await reader.read();if(next.done)break;
      size+=next.value.length;if(size>expected)throw new ReaderError('invalid_source');chunks.push(next.value.slice());}
    throwIfAborted(signal);
    const prefix=new Uint8Array(4);let at=0;
    for(const chunk of chunks){const length=Math.min(4-at,chunk.length);prefix.set(chunk.subarray(0,length),at);at+=length;if(at===4)break;}
    if(size!==expected||prefix[0]!==0||prefix[1]!==1||prefix[2]!==0||prefix[3]!==0)throw new ReaderError('invalid_source');
    const blob=new Blob(chunks,{type:'font/ttf'});
    const hash=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',await blob.arrayBuffer())),x=>x.toString(16).padStart(2,'0')).join('');
    throwIfAborted(signal);if(hash!==LITERATA_FONT_SHA256)throw new ReaderError('invalid_source');
    return blob;
  } finally {await reader.cancel().catch(()=>{});reader.releaseLock();}
}
