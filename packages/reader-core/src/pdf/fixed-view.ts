import type {ReaderDocument,ReaderLocation,ReaderView} from '../ports.ts';
import {planFixedLayout} from '../layout.ts';

/** Adapter-private projection of the canonical Reading facade, never a store. */
export type FixedPreferences={twoPages:boolean;firstPageAlone:boolean;comicFit:'auto'|'width'|'height';comicDirection:'file'|'ltr'|'rtl'|'vertical';pdfView:'automatic'|'spread'|'width'|'page'};
export type PageSurface=Readonly<{width:number;height:number;mount(target:HTMLElement,scale:number,signal:AbortSignal):Promise<()=>void>;dispose():void}>;
export type FixedPageLoader=(page:number,options:{width:number;signal:AbortSignal})=>Promise<PageSurface>;
type Zoom={kind:'scale';value:number}|{kind:'fit';value:'page'|'width'|'height'|'spread'};

/** Both fixed formats use the common spread planner and their own real page renderer. */
export async function mountFixed<P extends FixedPreferences>(format:'pdf'|'cbz',pageCount:number,fileDirection:'ltr'|'rtl'|'vertical',load:FixedPageLoader,options:Parameters<ReaderDocument<P>['mount']>[0]):Promise<ReaderView<P>>{
  const document=options.container.ownerDocument,host=document.createElement('div');
  host.dataset.readerFixed=format;Object.assign(host.style,{position:'relative',display:'flex',width:'100%',height:'100%',overflow:'auto',overscrollBehavior:'contain',touchAction:'none'});
  let preferences=options.preferences,viewport=options.viewport,page=0,closed=false,version=0;
  let zoom:Zoom|undefined,scale=1,location:ReaderLocation|null=null,cleanup:(()=>void)[]=[],pending:AbortController|undefined;
  const surfacesByPage=new Map<number,PageSurface>();
  const lifetime=new AbortController(),signal=AbortSignal.any([lifetime.signal,options.signal]);
  const direction=()=>format==='cbz'&&preferences.comicDirection!=='file'?preferences.comicDirection:fileDirection;
  const fit=()=>zoom?.kind==='fit'?zoom.value:format==='pdf'?(preferences.pdfView==='automatic'?'auto':preferences.pdfView):preferences.comicFit;
  const plan=(at=page)=>planFixedLayout({format,viewport,page:at,pageCount,twoPages:preferences.twoPages,firstPageAlone:preferences.firstPageAlone,direction:direction(),fit:fit()});
  const clean=()=>{for(const fn of cleanup.splice(0))fn();};
  async function render(at:number,taskSignal:AbortSignal):Promise<void>{
    if(closed)throw new Error('reader_closed');taskSignal.throwIfAborted();
    if(!Number.isInteger(at)||at<0||at>=pageCount)throw new Error('page_invalid');
    pending?.abort();const controller=new AbortController();pending=controller;const current=++version;
    const operation=AbortSignal.any([signal,controller.signal,taskSignal]),layout=plan(at),next=document.createElement('div'),releases:(()=>void)[]=[],created=new Map<number,PageSurface>();
    try{
      const surfaces:PageSurface[]=[];
      for(const index of layout.visualPages){
        let surface=surfacesByPage.get(index);
        if(!surface){surface=await load(index,{width:Math.max(32,Math.min(2048,Math.ceil(viewport.width/layout.visualPages.length))),signal:operation});if(operation.aborted){surface.dispose();throw operation.reason;}created.set(index,surface);}
        operation.throwIfAborted();surfaces.push(surface);
      }
      const width=surfaces.reduce((sum,s)=>sum+s.width,0),height=Math.max(...surfaces.map(s=>s.height));
      const fitted=layout.fit==='width'?viewport.width/width:layout.fit==='height'?viewport.height/height:Math.min(viewport.width/width,viewport.height/height);
      const requested=zoom?.kind==='scale'?zoom.value:fitted;
      if(!Number.isFinite(requested)||requested<=0||!Number.isFinite(width)||!Number.isFinite(height))throw new Error('page_invalid');
      Object.assign(next.style,{position:'relative',display:'flex',flex:'none',alignItems:'center',width:`${width*requested}px`,height:`${height*requested}px`,minWidth:'0',margin:'auto'});
      for(let i=0;i<surfaces.length;i++){
        const surface=surfaces[i],target=document.createElement('div');target.dataset.readerPage=String(layout.visualPages[i]);
        Object.assign(target.style,{position:'relative',flex:'none',width:`${surface.width*requested}px`,height:`${surface.height*requested}px`,overflow:'hidden'});next.append(target);
        releases.push(await surface.mount(target,requested,operation));operation.throwIfAborted();
      }
      if(closed||current!==version)throw new DOMException('','AbortError');
      clean();cleanup=releases;host.replaceChildren(next);page=at;scale=requested;
      for(const [index,surface]of created)surfacesByPage.set(index,surface);
      for(const [index,surface]of surfacesByPage)if(!layout.visualPages.includes(index)){surface.dispose();surfacesByPage.delete(index);}
      host.scrollLeft=0;host.scrollTop=0;
      location={format,page,progression:pageCount===1?0:page/(pageCount-1)};options.onLocation(location);
    }catch(error){for(const release of releases.reverse())release();for(const surface of created.values())surface.dispose();throw error;}
  }
  const pointers=new Map<number,{x:number;y:number}>();let pinch:number|undefined,drag:{x:number;y:number;left:number;top:number}|undefined;
  const distance=()=>{const a=[...pointers.values()];return a.length===2?Math.hypot(a[1].x-a[0].x,a[1].y-a[0].y):undefined;};
  const onDown=(event:PointerEvent)=>{
    // PDF text selection must retain browser handles and native selection semantics.
    if((event.target as Element)?.closest?.('[data-reader-text]'))return;
    pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});host.setPointerCapture?.(event.pointerId);
    if(pointers.size===1)drag={x:event.clientX,y:event.clientY,left:host.scrollLeft,top:host.scrollTop};else{pinch=distance();drag=undefined;}
  };
  const onMove=(event:PointerEvent)=>{
    if(!pointers.has(event.pointerId))return;pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});const nextDistance=distance();
    if(pinch&&nextDistance){const next=Math.max(.05,Math.min(8,scale*nextDistance/pinch));pinch=nextDistance;void view.setZoom!({kind:'scale',value:next},signal).catch(()=>{});}
    else if(drag){host.scrollLeft=drag.left+drag.x-event.clientX;host.scrollTop=drag.top+drag.y-event.clientY;}
  };
  const onUp=(event:PointerEvent)=>{pointers.delete(event.pointerId);pinch=undefined;drag=undefined;};
  const onWheel=(event:WheelEvent)=>{if(!event.ctrlKey)return;event.preventDefault();void view.setZoom!({kind:'scale',value:Math.max(.05,Math.min(8,scale*Math.exp(-event.deltaY*.002)))},signal).catch(()=>{});};
  const onDouble=()=>{void view.setZoom!(zoom?.kind==='scale'?{kind:'fit',value:'page'}:{kind:'scale',value:Math.min(8,scale*2)},signal).catch(()=>{});};
  const view:ReaderView<P>={
    async navigate(target,taskSignal){
      let at=page;
      if(target.kind==='next')at=plan().nextPage??page;
      else if(target.kind==='previous')at=plan().previousPage??page;
      else if(target.kind==='progression'){if(!Number.isFinite(target.value)||target.value<0||target.value>1)throw new Error('page_invalid');at=Math.round(target.value*(pageCount-1));}
      else{if(target.location.format==='epub'||target.location.format!==format)throw new Error('page_invalid');at=target.location.page;}
      await render(at,taskSignal);
    },
    async applyPreferences(next,taskSignal){const previous=preferences,previousZoom=zoom;preferences=next;zoom=undefined;try{await render(page,taskSignal);}catch(error){preferences=previous;zoom=previousZoom;throw error;}},
    async resize(next,taskSignal){const previous=viewport;viewport=next;try{await render(page,taskSignal);}catch(error){viewport=previous;throw error;}},
    getLocation(){return location;},
    async setZoom(next,taskSignal){if(next.kind==='scale'&&(!Number.isFinite(next.value)||next.value<.05||next.value>8))throw new Error('zoom_invalid');const previous=zoom;zoom=next;try{await render(page,taskSignal);}catch(error){zoom=previous;throw error;}},
    getZoom(){return {scale,...zoom?.kind==='scale'?{}:{fit:plan().fit}};},
    async pan(position,taskSignal){taskSignal.throwIfAborted();if(closed||!Number.isFinite(position.x)||!Number.isFinite(position.y))throw new Error('pan_invalid');host.scrollLeft=Math.max(0,position.x);host.scrollTop=Math.max(0,position.y);},
    dispose(){if(closed)return;closed=true;version++;pending?.abort();lifetime.abort();clean();for(const surface of surfacesByPage.values())surface.dispose();surfacesByPage.clear();host.remove();}
  };
  host.addEventListener('pointerdown',onDown,{signal});host.addEventListener('pointermove',onMove,{signal});host.addEventListener('pointerup',onUp,{signal});host.addEventListener('pointercancel',onUp,{signal});host.addEventListener('wheel',onWheel,{passive:false,signal});host.addEventListener('dblclick',onDouble,{signal});
  signal.addEventListener('abort',()=>{void view.dispose();},{once:true});
  try{await render(0,signal);signal.throwIfAborted();options.container.append(host);return view;}catch(error){await view.dispose();throw error;}
}
