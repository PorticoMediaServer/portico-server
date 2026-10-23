import type {ReaderAdapter,ReaderDocument} from '../ports.ts';
import {mountFixed,type FixedPreferences} from '../pdf/fixed-view.ts';
import {openComicArchive,type ComicPageProvider,type ComicPage} from './archive.ts';
import {checkAbort,ComicInputError} from './zip.ts';
import {rasterInfo} from './image.ts';

type Source=Parameters<ReaderAdapter<object>['open']>[0]['source'];
export type ComicAdapterOptions=Readonly<{
  pages?:(source:Source,signal:AbortSignal)=>Promise<ComicPageProvider>;
  /** Explicitly opt in for local/offline files; never a remote-browser fallback. */
  localArchive?:boolean;
}>;
function validatePage(page:ComicPage,index:number):void{
  if(page.index!==index||page.bytes.byteLength>32*1024*1024)throw new ComicInputError('image_invalid');
  const observed=rasterInfo(page.bytes);
  if(observed.mime!==page.image.mime||observed.width!==page.image.width||observed.height!==page.image.height)throw new ComicInputError('image_invalid');
}
async function bitmapFor(page:ComicPage,signal:AbortSignal):Promise<ImageBitmap>{
  checkAbort(signal);validatePage(page,page.index);
  const bitmap=await createImageBitmap(new Blob([page.bytes as Uint8Array<ArrayBuffer>],{type:page.image.mime}));
  if(signal.aborted){bitmap.close();throw signal.reason;}
  if(bitmap.width!==page.image.width||bitmap.height!==page.image.height){bitmap.close();throw new ComicInputError('image_invalid');}return bitmap;
}
export function createComicAdapter<P extends FixedPreferences>(options:ComicAdapterOptions={}):ReaderAdapter<P>{
  return {format:'cbz',async open({source,signal}):Promise<ReaderDocument<P>>{
    checkAbort(signal);
    if(!options.pages&&!options.localArchive)throw new Error('comic_provider_required');
    const id=source.id,revision=source.revision,size=source.size;
    const provider=options.pages?await options.pages(source,signal):await openComicArchive(source,signal);
    if(signal.aborted){await provider.dispose();throw signal.reason;}
    if(provider.id!==id||provider.revision!==revision||!Number.isSafeInteger(provider.pageCount)||provider.pageCount<1||provider.pageCount>10000){await provider.dispose();throw new ComicInputError('archive_limit');}
    const life=new AbortController(),views=new Set<Awaited<ReturnType<ReaderDocument<P>['mount']>>>();let closed=false;
    const active=(taskSignal:AbortSignal)=>{if(closed)throw new ComicInputError('closed');if(source.id!==id||source.revision!==revision||source.size!==size||provider.id!==id||provider.revision!==revision)throw new ComicInputError('archive_invalid');const combined=AbortSignal.any([life.signal,taskSignal]);checkAbort(combined);return combined;};
    const document:ReaderDocument<P>={id,revision,format:'cbz',metadata:{...provider.metadata,pageCount:provider.pageCount},toc:[],
      async mount(mountOptions){
        const fixed=await mountFixed('cbz',provider.pageCount,provider.metadata.direction??'ltr',async(index,{signal:taskSignal})=>{
          // Retain only the current spread at the bounded source resolution.
          // A phone-size thumbnail must not become the cached zoom/resize image.
          const combined=active(taskSignal),page=await provider.page(index,{width:2048,signal:combined});active(combined);validatePage(page,index);
          const bitmap=await bitmapFor(page,combined);let released=false;
          return {width:bitmap.width,height:bitmap.height,
            async mount(target,scale,renderSignal){checkAbort(renderSignal);const canvas=target.ownerDocument.createElement('canvas');
              const ratio=Math.min(target.ownerDocument.defaultView?.devicePixelRatio??1,2,8192/(bitmap.width*scale),8192/(bitmap.height*scale),Math.sqrt(16_000_000/(bitmap.width*bitmap.height*scale*scale)));
              canvas.width=Math.max(1,Math.ceil(bitmap.width*scale*ratio));canvas.height=Math.max(1,Math.ceil(bitmap.height*scale*ratio));Object.assign(canvas.style,{display:'block',width:'100%',height:'100%'});
              canvas.setAttribute('role','img');canvas.setAttribute('aria-label',page.name);
              const context=canvas.getContext('2d');if(!context)throw new ComicInputError('image_invalid');context.drawImage(bitmap,0,0,canvas.width,canvas.height);target.append(canvas);
              return ()=>{canvas.width=0;canvas.height=0;canvas.remove();};
            },dispose(){if(!released){released=true;bitmap.close();}}
          };
        },{...mountOptions,signal:active(mountOptions.signal)});const view={...fixed,dispose(){views.delete(view);return fixed.dispose();}};views.add(view);return view;
      },
      async *search(_query,searchOptions){active(searchOptions.signal);},
      async page(index,pageOptions){
        const combined=active(pageOptions.signal),width=pageOptions.width;
        if(!Number.isInteger(index)||index<0||index>=provider.pageCount||!Number.isInteger(width)||width<32||width>2048)throw new ComicInputError('page_invalid');
        const page=await provider.page(index,{width,signal:combined});active(combined);validatePage(page,index);
        if(page.image.width<=width&&page.bytes.length<=16*1024*1024&&page.image.mime!=='image/gif')return {mime:page.image.mime,bytes:new Uint8Array(page.bytes),width:page.image.width,height:page.image.height};
        const bitmap=await bitmapFor(page,combined);
        try{
          const scaledWidth=Math.min(width,bitmap.width),scaledHeight=Math.max(1,Math.round(bitmap.height*scaledWidth/bitmap.width));
          if(scaledWidth*scaledHeight>16_000_000)throw new ComicInputError('image_limit');
          const canvas=new OffscreenCanvas(scaledWidth,scaledHeight),ctx=canvas.getContext('2d');if(!ctx)throw new ComicInputError('image_invalid');ctx.drawImage(bitmap,0,0,scaledWidth,scaledHeight);
          const blob=await canvas.convertToBlob({type:'image/png'});checkAbort(combined);if(blob.size>16*1024*1024)throw new ComicInputError('image_limit');return {mime:'image/png',bytes:new Uint8Array(await blob.arrayBuffer()),width:scaledWidth,height:scaledHeight};
        }finally{bitmap.close();}
      },
      async dispose(){if(closed)return;closed=true;life.abort();for(const view of views)await view.dispose();views.clear();await provider.dispose();}
    };return document;
  }};
}
