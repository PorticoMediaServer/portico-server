import {EPUB} from 'foliate-js/epub.js';
import 'foliate-js/paginator.js';
import {fromRange, joinIndir} from 'foliate-js/epubcfi.js';
import {ReaderError, throwIfAborted} from '../errors.ts';
import type {ReaderAdapter, ReaderDocument, ReaderLocation, ReaderMetadata, ReaderMount,
  ReaderNavigation, ReaderSearchOptions, ReaderSearchResult, ReaderTocItem, ReaderView, ReaderViewport} from '../ports.ts';
import {openEpubArchive, safeArchivePath, type EpubAssets} from './archive.ts';
import {rejectXmlEntities, secureMarkup} from './security.ts';
import {epubDisplay, type EpubDisplayPreferences} from './display.ts';

type Section = {id: string; size: number; cfi: string; linear?: string; createDocument(): Promise<Document>};
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
function metadataText(value: unknown): string | undefined {
  if (typeof value === 'string') return value || undefined;
  if (Array.isArray(value)) return value.map(metadataText).filter(Boolean).join(', ') || undefined;
  if (value && typeof value === 'object') {
    const record = value as Record<string,unknown>;
    return metadataText(record.name) ?? metadataText(Object.values(record).find(v=>typeof v==='string'));
  }
}
function cfiAt(book: Book, index: number, range?: Range): string {
  return range ? joinIndir(book.sections[index].cfi, fromRange(range)) : book.sections[index].cfi;
}
function progression(book: Book, index: number, fraction: number): number {
  const sizes = book.sections.map(s=>s.linear==='no'?0:s.size), total = sizes.reduce((a,b)=>a+b,0);
  return total ? Math.max(0,Math.min(1,(sizes.slice(0,index).reduce((a,b)=>a+b,0)+(sizes[index]??0)*fraction)/total)) : 0;
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
function resourceAssets(source: import('../ports.ts').ReaderSource): EpubAssets {
  const resources=source.resources!; let total=0;
  if(resources.entries.length>4096) throw new ReaderError('limit_exceeded');
  const entries=new Map<string, import('../ports.ts').ReaderResource>();
  for(const entry of resources.entries) {
    const path=safeArchivePath(entry.path);
    if(entries.has(path)||!Number.isSafeInteger(entry.size)||entry.size<0||entry.size>16*1024*1024||(total+=entry.size)>256*1024*1024)
      throw new ReaderError('limit_exceeded');
    entries.set(path,entry);
  }
  return {entries:resources.entries,
    async read(path,signal) {
      throwIfAborted(signal);const entry=entries.get(safeArchivePath(path));if(!entry)return new Uint8Array();
      const bytes=await resources.read(path,signal);throwIfAborted(signal);
      if(!(bytes instanceof Uint8Array)||bytes.length!==entry.size)throw new ReaderError('invalid_book');return bytes;
    },dispose(){entries.clear();}};
}
/** Direct engine modules only: no stock reader, URL opener, media overlay or
 * ambient network client is imported. Online assets and local ZIPs share this. */
export function createBrowserEpubAdapter<P extends EpubDisplayPreferences>(options: EpubAdapterOptions): ReaderAdapter<P> {
  return {format:'epub',async open({source,signal:parent}) {
    const lifetime=new AbortController(),signal=AbortSignal.any([parent,lifetime.signal]);throwIfAborted(signal);
    const assets=source.resources?resourceAssets(source):await openEpubArchive(source,signal);
    let book: Book | undefined, mounted=false, disposed=false, view: ReaderView<P>|undefined;
    const byPath=new Map(assets.entries.map(e=>[e.path,e]));
    const decoder=new TextDecoder('utf-8',{fatal:true});
    async function loadText(path:string):Promise<string|null> {
      throwIfAborted(signal);
      const entry=byPath.get(safeArchivePath(path));if(!entry)return null;
      let text=decoder.decode(await assets.read(path,signal));throwIfAborted(signal);
      const mime='mime' in entry ? entry.mime : '';
      if(mime==='text/html'||mime==='application/xhtml+xml'||/\.x?html?$/i.test(path)||/<(?:\w+:)?html[\s>]/i.test(text))
        text=secureMarkup(text,'application/xhtml+xml');
      else if(mime==='image/svg+xml'||/\.svg$/i.test(path)) text=secureMarkup(text,'image/svg+xml');
      else rejectXmlEntities(text);
      return text;
    }
    async function loadBlob(path:string):Promise<Blob|null> {
      throwIfAborted(signal);const entry=byPath.get(safeArchivePath(path));if(!entry)return null;
      let bytes=await assets.read(path,signal);throwIfAborted(signal);
      const mime='mime' in entry && typeof entry.mime==='string'?entry.mime:'';
      if(mime==='image/svg+xml'||/\.svg$/i.test(path))bytes=new TextEncoder().encode(secureMarkup(decoder.decode(bytes),'image/svg+xml'));
      return new Blob([bytes as Uint8Array<ArrayBuffer>],{type:mime});
    }
    const close=async()=>{if(disposed)return;disposed=true;lifetime.abort();try{await view?.dispose();}finally{book?.destroy();await assets.dispose();}};
    try {
      const encryption=await loadText('META-INF/encryption.xml');
      if(encryption) {
        const xml=new DOMParser().parseFromString(encryption,'application/xml');
        if(xml.querySelector('parsererror'))throw new ReaderError('invalid_book');
        for(const method of Array.from(xml.getElementsByTagNameNS('*','EncryptionMethod'))) {
          if(!['http://www.idpf.org/2008/embedding','http://ns.adobe.com/pdf/enc#RC'].includes(method.getAttribute('Algorithm')??''))throw new ReaderError('protected');
        }
      }
      book=await (new EPUB({loadText,loadBlob,getSize:(path:string)=>byPath.get(path)?.size??0}) as Book).init();
      throwIfAborted(signal);
      if(!book.sections.length||book.sections.length>2048)throw new ReaderError('invalid_book');
      // Both the pre-loader and post-rewrite transformation are necessary:
      // external resources never enter the loader; CSP precedes iframe parsing.
      book.transformTarget.addEventListener('load',event=>{
        const detail=(event as CustomEvent<{isScript:boolean;allow:boolean}>).detail;if(detail.isScript)detail.allow=false;
      });
      book.transformTarget.addEventListener('data',event=>{
        const detail=(event as CustomEvent<{data:string|Blob|Promise<string|Blob>;type:string}>).detail;
        if(['application/xhtml+xml','text/html','image/svg+xml'].includes(detail.type)) {
          detail.data=Promise.resolve(detail.data).then(async data=>{throwIfAborted(signal);return secureMarkup(typeof data==='string'?data:await data.text(),detail.type as 'application/xhtml+xml',true);});
        }
      });
      const b=book;
      const metadata:ReaderMetadata=Object.freeze({title:metadataText(b.metadata?.title),author:metadataText(b.metadata?.author),
        language:metadataText(b.metadata?.language),direction:b.dir==='rtl'?'rtl':'ltr'});
      const tocBySection=new Map<number,string>();
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
            progression:progression(b,target.index,0),chapter:item.label};
          if(!tocBySection.has(target.index))tocBySection.set(target.index,item.label);
          result.push(Object.freeze({id:item.href,label:item.label,location:Object.freeze(location),...(item.subitems?{children:await mapToc(item.subitems)}:{})}));
        }return Object.freeze(result);
      }
      const toc=await mapToc(b.toc??[]);
      const document:ReaderDocument<P>={id:source.id,revision:source.revision,format:'epub',metadata,toc,
        async mount(mount) {
          throwIfAborted(signal);throwIfAborted(mount.signal);if(mounted)throw new ReaderError('invalid_book');mounted=true;
          view=await mountEpub(b,mount,options,tocBySection,signal);return view;
        },
        async *search(query:string,search:ReaderSearchOptions):AsyncIterable<ReaderSearchResult> {
          if(!query.trim()||query.length>1024||search.limit<1||search.limit>500)throw new ReaderError('limit_exceeded');
          const searchSignal=AbortSignal.any([signal,search.signal]);let count=0;
          for(let index=0;index<b.sections.length;index++) {
            throwIfAborted(searchSignal);
            const doc=await b.sections[index].createDocument();throwIfAborted(searchSignal);
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
              throwIfAborted(searchSignal);
              let start=offsets.length-1,end=start;
              while(start>0&&offsets[start]>at)start--;end=start;
              while(end+1<offsets.length&&offsets[end+1]<at+query.length)end++;
              const range=doc.createRange();range.setStart(nodes[start],at-offsets[start]);range.setEnd(nodes[end],at+query.length-offsets[end]);
              const cfi=cfiAt(b,index,range),chapter=tocBySection.get(index);
              yield {id:cfi,excerpt:text.slice(Math.max(0,at-60),Math.min(text.length,at+query.length+60)),
                location:{format:'epub',cfi,section:b.sections[index].id,progression:progression(b,index,text.length?at/text.length:0),chapter},chapter};
              if(++count>=search.limit)return;
            }
          }
        },dispose:close};
      return document;
    } catch(error){await close();throw error;}
  }};
}
async function mountEpub<P extends EpubDisplayPreferences>(book:Book,mount:ReaderMount<P>,options:EpubAdapterOptions,tocBySection:Map<number,string>,parent:AbortSignal):Promise<ReaderView<P>> {
  const abort=new AbortController(),signal=AbortSignal.any([parent,mount.signal,abort.signal]);
  const renderer=document.createElement('foliate-paginator') as Paginator;
  renderer.style.cssText='display:block;position:relative;width:100%;height:100%;';
  const urls={literata:URL.createObjectURL(options.fonts.literata),manrope:URL.createObjectURL(options.fonts.manrope)};
  let location:ReaderLocation|null=null,preferences=mount.preferences,viewport=mount.viewport,dead=false,loaded=false;
  let cleanupDoc:()=>void=()=>{};
  function current(){throwIfAborted(signal);if(dead)throw new ReaderError('closed');}
  function apply() {
    const display=epubDisplay(preferences,viewport,urls);renderer.style.background=display.background;
    renderer.setAttribute('flow',display.flow);renderer.setAttribute('margin',`${display.top}px`);
    renderer.setAttribute('gap',`${display.gap}%`);renderer.setAttribute('max-column-count',String(display.columns));
    renderer.setAttribute('max-inline-size',`${Math.max(100,display.maxInlineSize)}px`);
    renderer.setStyles(display.css);renderer.render();
  }
  const relocate=(event:Event)=>{
    if(dead||signal.aborted)return;const d=(event as CustomEvent<Relocate>).detail;
    if(!book.sections[d.index]||!Number.isFinite(d.fraction))return;
    location=Object.freeze({format:'epub',cfi:cfiAt(book,d.index,d.range),section:book.sections[d.index].id,
      progression:progression(book,d.index,d.fraction),chapter:tocBySection.get(d.index),
      ...(d.size&&Number.isFinite(d.size)&&d.size>0?{pagesLeftInSection:Math.max(0,Math.ceil((1-d.fraction)/d.size)-1)}:{})});
    mount.onLocation(location);
  };
  const load=(event:Event)=>{
    if(dead||signal.aborted)return;cleanupDoc();const {doc,index}=(event as CustomEvent<{doc:Document;index:number}>).detail;
    doc.documentElement.lang ||= metadataText(book.metadata?.language)??'';
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
      const target=book.resolveHref(href.startsWith('#')?book.sections[index].id+href:href);
      if(target)void cancellable(renderer.goTo(target),signal).catch(()=>{});
    },opts);
  };
  renderer.addEventListener('relocate',relocate);renderer.addEventListener('load',load);
  renderer.open(book);mount.container.append(renderer);apply();
  const dispose=()=>{if(dead)return;dead=true;abort.abort();cleanupDoc();renderer.removeEventListener('relocate',relocate);renderer.removeEventListener('load',load);
    try{if(loaded)renderer.destroy();}finally{renderer.remove();URL.revokeObjectURL(urls.literata);URL.revokeObjectURL(urls.manrope);}};
  signal.addEventListener('abort',dispose,{once:true});
  const view:ReaderView<P>={
    async navigate(target:ReaderNavigation,callerSignal) {
      current();const active=AbortSignal.any([signal,callerSignal]);
      if(target.kind==='next')await cancellable(renderer.next(),active);
      else if(target.kind==='previous')await cancellable(renderer.prev(),active);
      else if(target.kind==='location') {
        if(target.location.format!=='epub')throw new ReaderError('invalid_location');
        const resolved=book.resolveCFI(target.location.cfi);if(!resolved||!book.sections[resolved.index])throw new ReaderError('invalid_location');
        await cancellable(renderer.goTo(resolved),active);
      } else {
        if(!Number.isFinite(target.value)||target.value<0||target.value>1)throw new ReaderError('invalid_location');
        const sizes=book.sections.map(s=>s.linear==='no'?0:s.size),total=sizes.reduce((a,b)=>a+b,0);let remaining=total*target.value,index=0;
        while(index<sizes.length-1&&remaining>=sizes[index])remaining-=sizes[index++];
        await cancellable(renderer.goTo({index,anchor:sizes[index]?remaining/sizes[index]:0}),active);
      }current();
    },
    async applyPreferences(next,callerSignal){current();throwIfAborted(callerSignal);preferences=next;apply();await new Promise<void>(r=>requestAnimationFrame(()=>r()));current();},
    async resize(next,callerSignal){current();throwIfAborted(callerSignal);viewport=next;apply();await new Promise<void>(r=>requestAnimationFrame(()=>r()));current();},
    getLocation(){return location;},dispose
  };
  try {
    const body=book.landmarks?.find(mark=>mark.type.includes('bodymatter')||mark.type.includes('text'));
    const target=body?book.resolveHref(body.href):null;
    const index=Math.max(0,book.sections.findIndex(s=>s.linear!=='no'));
    await cancellable(renderer.goTo(target??{index}),signal);loaded=true;current();return view;
  }
  catch(error){dispose();throw error;}
}
