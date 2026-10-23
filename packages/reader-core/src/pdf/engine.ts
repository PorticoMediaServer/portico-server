import type {ReaderSource} from '../ports.ts';
import type {PDFDocumentProxy,PDFPageProxy,PDFWorker} from 'pdfjs-dist/types/src/display/api.d.ts';
import {pdfRanges} from './ranges.ts';

export class PdfInputError extends Error {
  readonly code:'pdf_invalid'|'pdf_limit'|'page_invalid'|'worker_required'|'closed';
  constructor(code:PdfInputError['code']){super(code);this.name='PdfInputError';this.code=code;}
}
export type PdfOptions=Readonly<{
  /** Bundle the matching pdfjs-dist worker; no CDN or PDF-provided URLs. */
  workerUrl?:string|URL;
  createWorker?:(url:string|URL)=>Worker;
  cMapUrl?:string;standardFontDataUrl?:string;wasmUrl?:string;
  maxBytes?:number;maxPages?:number;
}>;
export type PdfEngine=Readonly<{
  document:PDFDocumentProxy;
  library:typeof import('pdfjs-dist/legacy/build/pdf.mjs');
  signal:AbortSignal;
  page(index:number,signal:AbortSignal):Promise<PDFPageProxy>;
  run<T>(promise:Promise<T>,signal:AbortSignal):Promise<T>;
  dispose():Promise<void>;
}>;
const rangeBytes=64*1024;
function abortRace<T>(promise:Promise<T>,signal:AbortSignal):Promise<T>{
  signal.throwIfAborted();return new Promise((resolve,reject)=>{
    const abort=()=>reject(signal.reason);signal.addEventListener('abort',abort,{once:true});
    promise.then(resolve,reject).finally(()=>signal.removeEventListener('abort',abort));
  });
}
export async function openPdf(source:ReaderSource,signal:AbortSignal,options:PdfOptions={}):Promise<PdfEngine>{
  signal.throwIfAborted();
  if(typeof source.read!=='function'||!source.id||!source.revision||!Number.isSafeInteger(source.size)||source.size<8||source.size>(options.maxBytes??512*1024*1024))throw new PdfInputError('pdf_limit');
  const id=source.id,revision=source.revision,size=source.size;
  const unchanged=()=>{if(source.id!==id||source.revision!==revision||source.size!==size)throw new PdfInputError('pdf_invalid');};
  const life=new AbortController(),combined=AbortSignal.any([signal,life.signal]);
  const firstEnd=Math.min(source.size,rangeBytes),initial=await source.read({start:0,endExclusive:firstEnd},combined);combined.throwIfAborted();unchanged();
  if(!(initial instanceof Uint8Array)||initial.byteLength!==firstEnd||!new TextDecoder('latin1').decode(initial.subarray(0,1024)).includes('%PDF-'))throw new PdfInputError('pdf_invalid');
  const library=await import('pdfjs-dist/legacy/build/pdf.mjs');combined.throwIfAborted();
  let nativeWorker:Worker|undefined,worker:PDFWorker|undefined;
  if(options.workerUrl){nativeWorker=(options.createWorker??((url)=>new Worker(url,{type:'module'})))(options.workerUrl);worker=library.PDFWorker.create({port:nativeWorker});}
  else if(typeof window!=='undefined')throw new PdfInputError('worker_required');
  const transport=new library.PDFDataRangeTransport(source.size,new Uint8Array(initial),true);
  let failed:unknown,closed=false;
  let rejectFatal!:(error:unknown)=>void;const fatal=new Promise<never>((_resolve,reject)=>{rejectFatal=reject;});void fatal.catch(()=>{});
  let loading:ReturnType<typeof library.getDocument>|undefined;
  const fail=(error:unknown)=>{if(failed||closed)return;failed=error;rejectFatal(error);life.abort(error);void loading?.destroy().catch(()=>{});};
  const ranges=pdfRanges(source,{signal:combined,onRange:(start,bytes)=>transport.onDataRange(start,bytes),onError:fail});
  transport.requestDataRange=(start,end)=>ranges.request(start,end);
  transport.abort=()=>ranges.dispose();
  const abort=()=>{rejectFatal(combined.reason);void dispose().catch(()=>{});};combined.addEventListener('abort',abort,{once:true});
  let disposed:Promise<void>|undefined;
  const dispose=():Promise<void>=>{
    if(disposed)return disposed;closed=true;ranges.dispose();combined.removeEventListener('abort',abort);life.abort();
    disposed=(async()=>{try{await loading?.destroy();}finally{worker?.destroy();nativeWorker?.terminate();}})();return disposed;
  };
  try{
    loading=library.getDocument({range:transport,length:source.size,rangeChunkSize:rangeBytes,disableStream:true,disableAutoFetch:true,worker,
      stopAtErrors:true,enableXfa:false,maxImageSize:40_000_000,canvasMaxAreaInBytes:64*1024*1024,
      useWorkerFetch:false,...options.cMapUrl?{cMapUrl:options.cMapUrl,cMapPacked:true}:{},...options.standardFontDataUrl?{standardFontDataUrl:options.standardFontDataUrl}:{},...options.wasmUrl?{wasmUrl:options.wasmUrl}:{}});
    // Password challenges fail closed rather than leaving an unresolved loading task.
    loading.onPassword=()=>fail(new PdfInputError('pdf_invalid'));
    const document=await abortRace(Promise.race([loading.promise,fatal]),combined);
    if(!Number.isSafeInteger(document.numPages)||document.numPages<1||document.numPages>(options.maxPages??10000))throw new PdfInputError('pdf_limit');
    return Object.freeze({document,library,signal:life.signal,
      async run<T>(promise:Promise<T>,taskSignal:AbortSignal){if(closed)throw new PdfInputError('closed');unchanged();const result=await abortRace(Promise.race([promise,fatal]),AbortSignal.any([taskSignal,life.signal]));unchanged();return result;},
      async page(index:number,taskSignal:AbortSignal){if(closed)throw new PdfInputError('closed');unchanged();if(!Number.isInteger(index)||index<0||index>=document.numPages)throw new PdfInputError('page_invalid');const result=await abortRace(Promise.race([document.getPage(index+1),fatal]),AbortSignal.any([taskSignal,life.signal]));unchanged();return result;},dispose});
  }catch(error){await dispose();throw error;}
}

/** Stream text with a byte budget; scans never gain scripts, links, forms or HTML. */
export async function pdfText(engine:PdfEngine,page:PDFPageProxy,signal:AbortSignal):Promise<string>{
  const combined=AbortSignal.any([engine.signal,signal]);combined.throwIfAborted();
  const reader=page.streamTextContent().getReader();let text='',bytes=0;
  const abort=()=>{void reader.cancel(combined.reason).catch(()=>{});};combined.addEventListener('abort',abort,{once:true});
  try{
    for(;;){const next=await engine.run(reader.read(),combined);combined.throwIfAborted();if(next.done)break;
      for(const item of next.value.items){if('str'in item){bytes+=item.str.length*2;if(bytes>2*1024*1024)throw new PdfInputError('pdf_limit');text+=item.str+(item.hasEOL?'\n':' ');}}
    }return text.trim();
  }finally{combined.removeEventListener('abort',abort);await reader.cancel().catch(()=>{});reader.releaseLock();}
}

export function pdfTextStream(page:PDFPageProxy,signal:AbortSignal):ReadableStream {
  const reader=page.streamTextContent().getReader();let bytes=0,items=0,ended=false;
  const stop=async(reason?:unknown)=>{if(ended)return;ended=true;signal.removeEventListener('abort',abort);await reader.cancel(reason).catch(()=>{});reader.releaseLock();};
  const abort=()=>{void stop(signal.reason);};signal.addEventListener('abort',abort,{once:true});
  return new ReadableStream({
    async pull(controller){
      try{signal.throwIfAborted();const next=await reader.read();signal.throwIfAborted();if(next.done){await stop();controller.close();return;}
        items+=next.value.items.length;for(const item of next.value.items)if('str'in item)bytes+=item.str.length*2;
        if(bytes>2*1024*1024||items>50000)throw new PdfInputError('pdf_limit');controller.enqueue(next.value);
      }catch(error){await stop(error);controller.error(error);}
    },cancel(reason){return stop(reason);}
  },{highWaterMark:1});
}
