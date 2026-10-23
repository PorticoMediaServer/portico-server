import type {ReaderSource} from '../ports.ts';
import type {PDFDocumentProxy,PDFPageProxy,PDFWorker} from 'pdfjs-dist/types/src/display/api.d.ts';

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
  if(!source.id||!Number.isSafeInteger(source.size)||source.size<8||source.size>(options.maxBytes??512*1024*1024))throw new PdfInputError('pdf_limit');
  const life=new AbortController(),combined=AbortSignal.any([signal,life.signal]);
  const firstEnd=Math.min(source.size,rangeBytes),initial=await source.read({start:0,endExclusive:firstEnd},combined);combined.throwIfAborted();
  if(initial.byteLength!==firstEnd||!new TextDecoder('latin1').decode(initial.subarray(0,1024)).includes('%PDF-'))throw new PdfInputError('pdf_invalid');
  const library=await import('pdfjs-dist/legacy/build/pdf.mjs');combined.throwIfAborted();
  let nativeWorker:Worker|undefined,worker:PDFWorker|undefined;
  if(options.workerUrl){nativeWorker=(options.createWorker??((url)=>new Worker(url,{type:'module'})))(options.workerUrl);worker=library.PDFWorker.create({port:nativeWorker});}
  else if(typeof window!=='undefined')throw new PdfInputError('worker_required');
  const transport=new library.PDFDataRangeTransport(source.size,new Uint8Array(initial),true);
  let failed:unknown,closed=false,active=0,totalRequested=firstEnd,requests=0;
  const queue:{start:number;end:number}[]=[];
  let rejectFatal!:(error:unknown)=>void;const fatal=new Promise<never>((_resolve,reject)=>{rejectFatal=reject;});void fatal.catch(()=>{});
  let loading:ReturnType<typeof library.getDocument>|undefined;
  const fail=(error:unknown)=>{if(failed||closed)return;failed=error;rejectFatal(error);life.abort(error);queue.length=0;void loading?.destroy().catch(()=>{});};
  const pump=()=>{
    while(!closed&&!combined.aborted&&!failed&&active<2&&queue.length){
      const range=queue.shift()!;active++;
      void (async()=>{
        const result=new Uint8Array(range.end-range.start);
        for(let start=range.start;start<range.end;start+=rangeBytes){
          combined.throwIfAborted();const end=Math.min(start+rangeBytes,range.end);
          const bytes=await source.read({start,endExclusive:end},combined);combined.throwIfAborted();
          if(bytes.byteLength!==end-start)throw new PdfInputError('pdf_invalid');
          result.set(bytes,start-range.start);
        }
        // pdf.js matches a range reader by its original begin offset.
        transport.onDataRange(range.start,result);
      })().catch(fail).finally(()=>{active--;pump();});
    }
  };
  transport.requestDataRange=(start,end)=>{
    if(closed||combined.aborted)return;
    if(!Number.isSafeInteger(start)||!Number.isSafeInteger(end)||start<0||end<=start||end>source.size){fail(new PdfInputError('pdf_invalid'));return;}
    if(end-start>16*1024*1024){fail(new PdfInputError('pdf_limit'));return;}
    totalRequested+=end-start;requests++;
    if(queue.length>=128||requests>32768||totalRequested>Math.max(source.size*4,16*1024*1024)){fail(new PdfInputError('pdf_limit'));return;}
    queue.push({start,end});pump();
  };
  transport.abort=()=>{life.abort();queue.length=0;};
  const abort=()=>{rejectFatal(combined.reason);void loading?.destroy().catch(()=>{});};combined.addEventListener('abort',abort,{once:true});
  let disposed:Promise<void>|undefined;
  const dispose=():Promise<void>=>{
    if(disposed)return disposed;closed=true;queue.length=0;combined.removeEventListener('abort',abort);life.abort();
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
      run<T>(promise:Promise<T>,taskSignal:AbortSignal){if(closed)return Promise.reject(new PdfInputError('closed'));return abortRace(Promise.race([promise,fatal]),AbortSignal.any([taskSignal,life.signal]));},
      page(index:number,taskSignal:AbortSignal){if(closed)return Promise.reject(new PdfInputError('closed'));if(!Number.isInteger(index)||index<0||index>=document.numPages)return Promise.reject(new PdfInputError('page_invalid'));return abortRace(Promise.race([document.getPage(index+1),fatal]),AbortSignal.any([taskSignal,life.signal]));},dispose});
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
