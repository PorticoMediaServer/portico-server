/// <reference path="./foliate.d.ts" />
import {EPUB} from 'foliate-js/epub.js';
import './vendor/paginator.js';
import {fromRange, joinIndir} from 'foliate-js/epubcfi.js';
import {ReaderError, throwIfAborted} from '../errors.ts';
import {abortScope} from '../abort.ts';
import type {ReaderAdapter, ReaderDocument, ReaderLocation, ReaderMetadata, ReaderMount,
  ReaderNavigation, ReaderSearchOptions, ReaderSearchResult, ReaderTocItem, ReaderView, ReaderViewport} from '../ports.ts';
import {openEpubArchive, safeArchivePath} from './archive.ts';
import {resourceAssets} from './resources.ts';
import {rejectXmlEntities, secureMarkup, secureCss} from './security.ts';
import {epubDisplay, type EpubDisplayPreferences} from './display.ts';
import {validateEpubRaster} from './media.ts';
import {epubFontKey} from './font-obfuscation.ts';
import {metadataLanguage, metadataText} from './metadata.ts';

type Section = {id: string; size: number; cfi: string; linear?: string; createDocument(): Promise<Document>; resolveHref(href:string):string};
type Navigation = {index: number; anchor?: number | ((doc: Document) => Node | Range | number | null)};
type Toc = {href: string; label: string; subitems?: Toc[]};
type Book = {
  init(): Promise<Book>; landmarks?: {type: string[]; href: string}[]; sections: Section[]; toc?: Toc[]; metadata?: Record<string, unknown>; dir?: string;
  rendition?: {layout?: string}; transformTarget: EventTarget;
  resolveHref(href: string): Navigation | null; resolveCFI(cfi: string): Navigation; destroy(): void;
};
type Relocate = {index: number; range: Range; fraction: number; size?: number};
interface Paginator extends HTMLElement {
  open(book: Book): void; goTo(target: Navigation): Promise<void>; next(): Promise<void>; prev(): Promise<void>;
  setStyles(styles: string): void; render(): void; destroy(): void;
  getContents(): {doc: Document; index: number}[];
}
export type EpubAdapterOptions = Readonly<{fonts: Readonly<{literata: Blob; manrope: Blob}>}>;
function cfiAt(book: Book, index: number, range?: Range): string {
  return range ? joinIndir(book.sections[index].cfi, fromRange(range)) : book.sections[index].cfi;
}
function progression(book: Book, index: number, fraction: number): number {
  const sizes = book.sections.map(s=>s.linear==='no'?0:s.size), total = sizes.reduce((a,b)=>a+b,0);
  return total ? Math.max(0,Math.min(1,(sizes.slice(0,index).reduce((a,b)=>a+b,0)+(sizes[index]??0)*fraction)/total)) : 0;
}
function chapterAt(book:Book,index:number,visible:Range,tocBySection:Map<number,{label:string;cfi:string}[]>):string|undefined {
  let nearest:Range|undefined,label:string|undefined;
  for(const item of tocBySection.get(index)??[]) {
    const target=book.resolveCFI(item.cfi);const anchor=typeof target?.anchor==='function'?target.anchor(visible.startContainer.ownerDocument!):null;
    if(!anchor)continue;
    const range=visible.startContainer.ownerDocument!.createRange();
    if((anchor as Range).startContainer)range.setStart((anchor as Range).startContainer,(anchor as Range).startOffset);
    else if((anchor as Node).nodeType)range.selectNodeContents(anchor as Node);else continue;
    range.collapse(true);
    if(range.compareBoundaryPoints(0,visible)<=0&&(!nearest||range.compareBoundaryPoints(0,nearest)>0)){nearest=range;label=item.label;}
  }
  return label;
}
async function cancellable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  throwIfAborted(signal);
  let abort!: () => void, timer: ReturnType<typeof setTimeout>;
  const stopped = new Promise<never>((_,reject)=>{
    abort=()=>reject(new ReaderError('aborted')); signal.addEventListener('abort',abort,{once:true});
    timer=setTimeout(()=>reject(new ReaderError('render_failed')),15000);
  });
  try {return await Promise.race([promise,stopped]);}
  finally {clearTimeout(timer!);signal.removeEventListener('abort',abort);}
}
/** Direct engine modules only: no stock reader, URL opener, media overlay or
 * ambient network client is imported. Online assets and local ZIPs share this. */
export function createBrowserEpubAdapter<P extends EpubDisplayPreferences>(options: EpubAdapterOptions): ReaderAdapter<P> {
  return {format:'epub',async open({source,signal:parent}) {
    const id=source.id,revision=source.revision,size=source.size,resources=source.resources;
    const currentSource=()=>source.id===id&&source.revision===revision&&source.size===size&&source.resources===resources;
    const unchanged=()=>{if(!currentSource())throw new ReaderError('invalid_source');};
    const lifetime=new AbortController(),scope=abortScope([parent,lifetime.signal]),signal=scope.signal;
    let assets:Awaited<ReturnType<typeof openEpubArchive>>;
    try{throwIfAborted(signal);assets=source.resources?resourceAssets(source):await openEpubArchive(source,signal);}
    catch(error){scope.dispose();throw error;}
    let book: Book | undefined, mounted=false, disposed=false, view: ReaderView<P>|undefined;
    const byPath=new Map(assets.entries.map(e=>[e.path,e]));
    const decoder=new TextDecoder('utf-8',{fatal:true});
    async function loadText(path:string,callerSignal:AbortSignal=signal):Promise<string|null> {
      const task=abortScope([signal,callerSignal]),active=task.signal;
      try {throwIfAborted(active);unchanged();
      const entry=byPath.get(safeArchivePath(path));if(!entry)return null;
      let text=decoder.decode(await assets.read(path,active));throwIfAborted(active);unchanged();
      const mime='mime' in entry ? entry.mime : '';
      if(mime==='text/html'||mime==='application/xhtml+xml'||/\.x?html?$/i.test(path)||/<(?:\w+:)?html[\s>]/i.test(text))
        text=secureMarkup(text,'application/xhtml+xml');
      else if(mime==='image/svg+xml'||/\.svg$/i.test(path)) text=secureMarkup(text,'image/svg+xml');
      else if(mime==='text/css'||/\.css$/i.test(path))text=secureCss(text);
      else rejectXmlEntities(text);
      return text;
      }finally{task.dispose();}
    }
    async function loadBlob(path:string):Promise<Blob|null> {
      throwIfAborted(signal);unchanged();const entry=byPath.get(safeArchivePath(path));if(!entry)return null;
      let bytes=await assets.read(path,signal);throwIfAborted(signal);unchanged();
      const mime='mime' in entry && typeof entry.mime==='string'?entry.mime:'';
      if(mime==='image/svg+xml'||/\.svg$/i.test(path))bytes=new TextEncoder().encode(secureMarkup(decoder.decode(bytes),'image/svg+xml'));
      return new Blob([bytes as Uint8Array<ArrayBuffer>],{type:mime});
    }
    const close=async()=>{if(disposed)return;disposed=true;lifetime.abort();scope.dispose();try{await view?.dispose();}finally{book?.destroy();await assets.dispose();}};
    try {
      unchanged();
      const encryption=await loadText('META-INF/encryption.xml');
      if(encryption) {
        const xml=new DOMParser().parseFromString(encryption,'application/xml');
        if(xml.querySelector('parsererror'))throw new ReaderError('invalid_book');
        for(const method of Array.from(xml.getElementsByTagNameNS('*','EncryptionMethod'))) {
          if(!['http://www.idpf.org/2008/embedding','http://ns.adobe.com/pdf/enc#RC'].includes(method.getAttribute('Algorithm')??''))throw new ReaderError('protected');
        }
      }
      book=await (new EPUB({loadText,loadBlob,getSize:(path:string)=>byPath.get(path)?.size??0,sha1:epubFontKey}) as Book).init();
      throwIfAborted(signal);
      if(!book.sections.length||book.sections.length>2048)throw new ReaderError('invalid_book');
      // Both the pre-loader and post-rewrite transformation are necessary:
      // external resources never enter the loader; CSP precedes iframe parsing.
      book.transformTarget.addEventListener('load',event=>{
        const detail=(event as CustomEvent<{isScript:boolean;allow:boolean}>).detail;if(detail.isScript)detail.allow=false;
      });
      book.transformTarget.addEventListener('data',event=>{
        const detail=(event as CustomEvent<{name?:string;data:string|Blob|Promise<string|Blob>;type:string}>).detail;
        if(detail.type==='text/css')detail.data=Promise.resolve(detail.data).then(async data=>{throwIfAborted(signal);return secureCss(typeof data==='string'?data:await data.text(),true);});
        if(['application/xhtml+xml','text/html','image/svg+xml'].includes(detail.type)) {
          detail.data=Promise.resolve(detail.data).then(async data=>{throwIfAborted(signal);return secureMarkup(typeof data==='string'?data:await data.text(),detail.type as 'application/xhtml+xml',true);});
        } else if(detail.type!=='text/css') {
          detail.data=Promise.resolve(detail.data).then(async data=>{
            throwIfAborted(signal);
            // Inspect media after standard font deobfuscation, before the
            // engine creates a URL/browser decoder can consume it. Obfuscated
            // font bytes can otherwise have a coincidental image signature.
            if(data instanceof Blob) {
              const bytes=new Uint8Array(await data.arrayBuffer());throwIfAborted(signal);
              const observed=validateEpubRaster(bytes,detail.name??'',detail.type);
              if(observed)detail.type=observed.mime;
            }
            return data;
          });
        }
      });
      const b=book;
      const metadata:ReaderMetadata=Object.freeze({title:metadataText(b.metadata?.title),author:metadataText(b.metadata?.author),
        language:metadataLanguage(b.metadata?.language),...(b.dir==='rtl'||b.dir==='ltr'?{direction:b.dir}:{})});
      const tocBySection=new Map<number,{label:string;cfi:string}[]>();
      async function mapToc(items:readonly Toc[]):Promise<readonly ReaderTocItem[]> {
        const result:ReaderTocItem[]=[];
        for(const item of items) {
          throwIfAborted(signal);const target=b.resolveHref(item.href);if(!target||!b.sections[target.index])continue;
          const doc=await b.sections[target.index].createDocument();
          const node=typeof target.anchor==='function'?target.anchor(doc):null;
          const range=doc.createRange();if(node instanceof Range)range.setStart(node.startContainer,node.startOffset);
          else if(node instanceof Node)range.selectNodeContents(node);else range.selectNodeContents(doc.body??doc.documentElement);
          range.collapse(true);
          const location:ReaderLocation={format:'epub',cfi:cfiAt(b,target.index,range),section:b.sections[target.index].id,
            progression:progression(b,target.index,(()=>{
              const prefix=doc.createRange();prefix.selectNodeContents(doc.body??doc.documentElement);prefix.setEnd(range.startContainer,range.startOffset);
              return prefix.toString().length/Math.max(1,(doc.body??doc.documentElement).textContent?.length??1);
            })()),chapter:item.label};
          const list=tocBySection.get(target.index)??[];list.push({label:item.label,cfi:location.cfi});tocBySection.set(target.index,list);
          result.push(Object.freeze({id:item.href,label:item.label,location:Object.freeze(location),...(item.subitems?{children:await mapToc(item.subitems)}:{})}));
        }return Object.freeze(result);
      }
      const toc=await mapToc(b.toc??[]);
      const document:ReaderDocument<P>={id,revision,format:'epub',metadata,toc,
        async mount(mount) {
          throwIfAborted(signal);throwIfAborted(mount.signal);unchanged();if(mounted)throw new ReaderError('invalid_book');mounted=true;
          view=await mountEpub(b,{...mount,onLocation(value){if(currentSource())mount.onLocation(value);},
            onSelection(value){if(currentSource())mount.onSelection(value);},onSurfaceInput(value){if(currentSource())mount.onSurfaceInput?.(value);}},options,tocBySection,signal,unchanged);return view;
        },
        async *search(query:string,search:ReaderSearchOptions):AsyncIterable<ReaderSearchResult> {
          if(!query.trim()||query.length>1024||search.limit<1||search.limit>500)throw new ReaderError('limit_exceeded');
          const task=abortScope([signal,search.signal]),searchSignal=task.signal;let count=0;
          try {
          for(let index=0;index<b.sections.length;index++) {
            throwIfAborted(searchSignal);unchanged();
            // Search has its own read signal, so cancelling a query cancels its
            // asset request without closing the mounted publication.
            const textSource=await loadText(b.sections[index].id,searchSignal);throwIfAborted(searchSignal);
            if(!textSource)continue;
            const doc=new DOMParser().parseFromString(textSource,'application/xhtml+xml');
            if(doc.querySelector('parsererror'))throw new ReaderError('invalid_book');
            const walker=doc.createTreeWalker(doc.body??doc.documentElement,NodeFilter.SHOW_TEXT,{acceptNode(node){
              return node.parentElement?.closest('style,script,head')?NodeFilter.FILTER_REJECT:NodeFilter.FILTER_ACCEPT;
            }});
            const nodes:Text[]=[],offsets:number[]=[];let text='';
            for(let node=walker.nextNode();node;node=walker.nextNode()) {nodes.push(node as Text);offsets.push(text.length);text+=node.textContent??'';if(text.length>8*1024*1024)throw new ReaderError('limit_exceeded');}
            // Locale-neutral lowercase preserves UTF-16 offsets for common text;
            // search compares slices to avoid length-changing case mappings.
            const folded=text.toLowerCase(),useFolded=folded.length===text.length;
            const haystack=useFolded?folded:text,needle=useFolded?query.toLowerCase():query;
            for(let at=haystack.indexOf(needle);at>=0;at=haystack.indexOf(needle,at+Math.max(1,query.length))) {
              throwIfAborted(searchSignal);unchanged();
              let start=offsets.length-1,end=start;
              while(start>0&&offsets[start]>at)start--;end=start;
              while(end+1<offsets.length&&offsets[end+1]<at+query.length)end++;
              const range=doc.createRange();range.setStart(nodes[start],at-offsets[start]);range.setEnd(nodes[end],at+query.length-offsets[end]);
              const cfi=cfiAt(b,index,range),chapter=chapterAt(b,index,range,tocBySection);
              yield {id:cfi,excerpt:text.slice(Math.max(0,at-60),Math.min(text.length,at+query.length+60)),
                location:{format:'epub',cfi,section:b.sections[index].id,progression:progression(b,index,text.length?at/text.length:0),chapter},chapter};
              if(++count>=search.limit)return;
            }
          }
          }finally{task.dispose();}
        },dispose:close};
      unchanged();return document;
    } catch(error){await close();throw error;}
  }};
}
async function mountEpub<P extends EpubDisplayPreferences>(book:Book,mount:ReaderMount<P>,options:EpubAdapterOptions,tocBySection:Map<number,{label:string;cfi:string}[]>,parent:AbortSignal,unchanged:()=>void):Promise<ReaderView<P>> {
  const abort=new AbortController(),scope=abortScope([parent,mount.signal,abort.signal]),signal=scope.signal;
  const renderer=document.createElement('foliate-paginator') as Paginator;
  const surface=document.createElement('div');surface.style.cssText='position:relative;width:100%;height:100%;overflow:hidden;';
  renderer.style.cssText='display:block;position:absolute;left:0;right:0;width:100%;';
  const urls={literata:URL.createObjectURL(options.fonts.literata),manrope:URL.createObjectURL(options.fonts.manrope)};
  let location:ReaderLocation|null=null,preferences=mount.preferences,viewport=mount.viewport,dead=false;
  let cleanupDoc:()=>void=()=>{};
  function current(){throwIfAborted(signal);if(dead)throw new ReaderError('closed');unchanged();}
  function apply() {
    const display=epubDisplay(preferences,viewport,urls);surface.style.background=display.background;
    renderer.style.top=`${display.top}px`;renderer.style.height=`${Math.max(1,viewport.height-display.top-display.bottom)}px`;
    renderer.setAttribute('flow',display.flow);renderer.setAttribute('margin','0px');
    renderer.setAttribute('max-block-size',`${Math.max(1,viewport.height-display.top-display.bottom)}px`);
    renderer.setAttribute('gap',`${display.gap}%`);renderer.setAttribute('max-column-count',String(display.columns));
    renderer.setAttribute('max-inline-size',`${Math.max(100,display.maxInlineSize)}px`);
    renderer.setStyles(display.css);renderer.render();
  }
  const relocate=(event:Event)=>{
    if(dead||signal.aborted)return;const d=(event as CustomEvent<Relocate>).detail;
    if(!book.sections[d.index]||!Number.isFinite(d.fraction))return;
    location=Object.freeze({format:'epub',cfi:cfiAt(book,d.index,d.range),section:book.sections[d.index].id,
      progression:progression(book,d.index,d.fraction),chapter:chapterAt(book,d.index,d.range,tocBySection),
      ...((tocBySection.get(d.index)?.length??0)===1&&d.size&&Number.isFinite(d.size)&&d.size>0?{pagesLeftInSection:Math.max(0,Math.ceil((1-d.fraction)/d.size)-1)}:{})});
    mount.onLocation(location);
  };
  const load=(event:Event)=>{
    if(dead||signal.aborted)return;cleanupDoc();const {doc,index}=(event as CustomEvent<{doc:Document;index:number}>).detail;
    const iframe=doc.defaultView?.frameElement;
    if(mount.metadata?.title)iframe?.setAttribute('title',mount.metadata.title);
    doc.documentElement.lang ||= metadataLanguage(book.metadata?.language)??'';
    const listenerAbort=new AbortController(),opts={signal:listenerAbort.signal};cleanupDoc=()=>listenerAbort.abort();
    const selection=()=>{
      if(dead||signal.aborted)return;const selected=doc.defaultView?.getSelection();
      if(!selected||selected.isCollapsed||!selected.rangeCount){mount.onSelection(null);return;}
      const range=selected.getRangeAt(0);if(!range.toString().trim())return;
      const frame=doc.defaultView?.frameElement?.getBoundingClientRect(),host=mount.container.getBoundingClientRect();
      const rects=Array.from(range.getClientRects(),r=>({x:r.x+(frame?.x??0)-host.x,y:r.y+(frame?.y??0)-host.y,width:r.width,height:r.height}));
      mount.onSelection({text:range.toString().slice(0,65536),location:{format:'epub',cfi:cfiAt(book,index,range),section:book.sections[index].id,progression:location?.progression??0},rects});
    };
    doc.addEventListener('selectionchange',selection,opts);
    let down:{x:number;y:number;at:number}|null=null;
    doc.addEventListener('pointerdown',event=>{if(event.button===0)down={x:event.clientX,y:event.clientY,at:performance.now()};},opts);
    doc.addEventListener('pointerup',event=>{
      mount.onSurfaceInput?.({kind:'activity'});
      if(!down||Math.hypot(event.clientX-down.x,event.clientY-down.y)>10||performance.now()-down.at>400||doc.defaultView?.getSelection()?.toString())return;
      mount.onSurfaceInput?.({kind:'tap',fraction:Math.max(0,Math.min(1,event.clientX/(doc.defaultView?.innerWidth??viewport.width)))});down=null;
    },opts);
    doc.addEventListener('pointermove',()=>mount.onSurfaceInput?.({kind:'activity'}),opts);
    doc.addEventListener('keydown',event=>mount.onSurfaceInput?.({kind:'key',key:event.key,shift:event.shiftKey,alt:event.altKey,ctrl:event.ctrlKey,meta:event.metaKey,
      editable:(event.target as Element|null)?.nodeType===1&&!!(event.target as Element).closest('input,textarea,[contenteditable="true"]'),preventDefault:()=>event.preventDefault()}),opts);
    doc.addEventListener('click',event=>{
      const link=(event.target as Element|null)?.closest('a[href]');if(!link)return;event.preventDefault();
      const href=link.getAttribute('href');if(!href||/^[a-z][a-z\d+.-]*:|^\//i.test(href))return;
      const target=book.resolveHref(book.sections[index].resolveHref(href));
      if(target)void cancellable(renderer.goTo(target),signal).catch(()=>{});
    },opts);
  };
  renderer.addEventListener('relocate',relocate);renderer.addEventListener('load',load);
  const dispose=()=>{if(dead)return;dead=true;abort.abort();scope.dispose();cleanupDoc();renderer.removeEventListener('relocate',relocate);renderer.removeEventListener('load',load);
    try{renderer.destroy();}finally{surface.remove();URL.revokeObjectURL(urls.literata);URL.revokeObjectURL(urls.manrope);}};
  signal.addEventListener('abort',dispose,{once:true});
  const view:ReaderView<P>={
    async navigate(target:ReaderNavigation,callerSignal) {
      current();const task=abortScope([signal,callerSignal]),active=task.signal;
      try {
      if(target.kind==='next')await cancellable(renderer.next(),active);
      else if(target.kind==='previous')await cancellable(renderer.prev(),active);
      else if(target.kind==='location') {
        if(target.location.format!=='epub')throw new ReaderError('invalid_location');
        const resolved=book.resolveCFI(target.location.cfi);if(!resolved||!book.sections[resolved.index])throw new ReaderError('invalid_location');
        await cancellable(renderer.goTo(resolved),active);
      } else if(target.kind==='progression') {
        if(!Number.isFinite(target.value)||target.value<0||target.value>1)throw new ReaderError('invalid_location');
        const sizes=book.sections.map(s=>s.linear==='no'?0:s.size),total=sizes.reduce((a,b)=>a+b,0);let remaining=total*target.value,index=0;
        while(index<sizes.length-1&&remaining>=sizes[index])remaining-=sizes[index++];
        await cancellable(renderer.goTo({index,anchor:sizes[index]?remaining/sizes[index]:0}),active);
      }current();
      }finally{task.dispose();}
    },
    async applyPreferences(next,callerSignal){current();throwIfAborted(callerSignal);preferences=next;apply();await new Promise<void>(r=>requestAnimationFrame(()=>r()));current();},
    async resize(next,callerSignal){current();throwIfAborted(callerSignal);viewport=next;apply();await new Promise<void>(r=>requestAnimationFrame(()=>r()));current();},
    getLocation(){return location;},dispose
  };
  try {
    current();renderer.open(book);surface.append(renderer);mount.container.append(surface);apply();
    const body=book.landmarks?.find(mark=>mark.type.includes('bodymatter')||mark.type.includes('text'));
    const target=body?book.resolveHref(body.href):null;
    const index=Math.max(0,book.sections.findIndex(s=>s.linear!=='no'));
    await cancellable(renderer.goTo(target??{index}),signal);current();return view;
  }
  catch(error){dispose();throw error;}
}
