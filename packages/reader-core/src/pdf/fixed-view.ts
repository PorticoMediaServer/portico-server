import {throwSignalAborted as throwIfAborted} from '../abort.ts';
import {abortScope,abortReason} from '../abort.ts';
import type {ReaderDocument,ReaderFixedViewport,ReaderLocation,ReaderView} from '../ports.ts';
import {planFixedLayout, fixedVisualPanes, validReaderCrease} from '../layout.ts';

/** Adapter-private projection of the canonical Reading facade, never a store. */
export type FixedPreferences={twoPages:boolean;firstPageAlone:boolean;comicFit:'auto'|'width'|'height';comicDirection:'file'|'ltr'|'rtl'|'vertical';pdfView:'automatic'|'spread'|'width'|'page'};
export type PageSurface=Readonly<{width:number;height:number;mount(target:HTMLElement,scale:number,signal:AbortSignal):Promise<()=>void>;dispose():void}>;
export type FixedPageLoader=(page:number,options:{width:number;signal:AbortSignal})=>Promise<PageSurface>;
type Zoom={kind:'scale';value:number}|{kind:'fit';value:'page'|'width'|'height'|'spread'};

export type FixedPageViewportInput=Readonly<{
  page:number;pageWidth:number;pageHeight:number;scale:number;fittedScale:number;
  /** Measured scroll offset and visible box of the page's own scroll host. */
  scroll:Readonly<{left:number;top:number;width:number;height:number}>;
  /** Reported window the host fills, used for the box when it is not measured. */
  viewport:Readonly<{width:number;height:number}>;
}>;
const fraction=(value:number)=>Math.min(1,Math.max(0,value));
/** What a zoomed fixed page actually shows, in 0..1 page fractions. A page at or
 * below its fitted scale has nothing to report, so the result is absent at fit,
 * before layout, and for any input that cannot describe a real page. */
export function fixedPageViewport(input:FixedPageViewportInput):ReaderFixedViewport|undefined{
  const {page,pageWidth,pageHeight,scale,fittedScale,scroll,viewport}=input;
  const boxWidth=scroll.width>0?scroll.width:viewport.width,boxHeight=scroll.height>0?scroll.height:viewport.height;
  if(!Number.isSafeInteger(page)||page<0||!Number.isFinite(pageWidth)||pageWidth<=0||!Number.isFinite(pageHeight)||pageHeight<=0
    ||!Number.isFinite(scale)||scale<=0||!Number.isFinite(fittedScale)||fittedScale<=0
    ||!Number.isFinite(scroll.left)||!Number.isFinite(scroll.top)||!Number.isFinite(boxWidth)||boxWidth<=0||!Number.isFinite(boxHeight)||boxHeight<=0)return;
  if(scale-fittedScale<=1e-6)return;
  const width=pageWidth*scale,height=pageHeight*scale;
  // A page smaller than its box is centred by the flex host; a larger one starts at zero.
  const offsetX=Math.max(0,(boxWidth-width)/2),offsetY=Math.max(0,(boxHeight-height)/2);
  const x=fraction((scroll.left-offsetX)/width),y=fraction((scroll.top-offsetY)/height);
  const right=fraction((scroll.left+boxWidth-offsetX)/width),bottom=fraction((scroll.top+boxHeight-offsetY)/height);
  return Object.freeze({page,pageWidth,pageHeight,x,y,width:Math.max(0,right-x),height:Math.max(0,bottom-y),scale});
}

/** Both fixed formats use the common spread planner and their own real page renderer. */
export async function mountFixed<P extends FixedPreferences>(format:'pdf'|'cbz'|'cbr',pageCount:number,fileDirection:'ltr'|'rtl'|'vertical',load:FixedPageLoader,options:Parameters<ReaderDocument<P>['mount']>[0]):Promise<ReaderView<P>>{
  const document=options.container.ownerDocument,host=document.createElement('div');
  const paneStyle=document.createElement('style');paneStyle.textContent='[data-reader-fixed] [data-reader-page]{scrollbar-width:none}[data-reader-fixed] [data-reader-page]::-webkit-scrollbar{display:none}';document.head.append(paneStyle);
  host.tabIndex=0;host.setAttribute('role','document');
  host.dataset.readerFixed=format;Object.assign(host.style,{position:'relative',display:'flex',width:'100%',height:'100%',overflow:'auto',overscrollBehavior:'contain',touchAction:'none'});
  let preferences=options.preferences,viewport=options.viewport,page=0,closed=false,version=0,mutation=0,rendering=0,scrolling=false;
  let zoom:Zoom|undefined,scale=1,fittedScale=1,verticalWidth:number|undefined,location:ReaderLocation|null=null,cleanup:(()=>void)[]=[],pending:AbortController|undefined;
  let foldPanes:Map<number,{clip:HTMLDivElement;scale:number}>|undefined,dragPane:HTMLDivElement|undefined,gesturePage:number|undefined;
  let pageGeometry:{page:number;width:number;height:number;scale:number;fittedScale:number}|undefined,reportedViewport:ReaderFixedViewport|undefined,viewportFrame:number|undefined;
  const scrollState=(at=gesturePage??page)=>{const pane=foldPanes?.get(at);return pane?{left:pane.clip.scrollLeft,top:pane.clip.scrollTop,width:pane.clip.clientWidth,height:pane.clip.clientHeight}:{left:host.scrollLeft,top:host.scrollTop,width:viewport.width,height:viewport.height};};
  const scrollTo=(left:number,top:number,at=gesturePage??page)=>{const pane=foldPanes?.get(at);if(pane){pane.clip.scrollLeft=Math.max(0,left);pane.clip.scrollTop=Math.max(0,top);}else{host.scrollLeft=Math.max(0,left);host.scrollTop=Math.max(0,top);}};
  // The single-page host is the scroll boundary here; it uses the same positive
  // scroll offsets the pan/scroll code above already clamps to.
  const currentViewport=(at=page)=>{const geometry=pageGeometry;
    return geometry&&geometry.page===at?fixedPageViewport({page:at,pageWidth:geometry.width,pageHeight:geometry.height,scale:geometry.scale,
      fittedScale:geometry.fittedScale,scroll:scrollState(at),viewport}):undefined;};
  const sameViewport=(left:ReaderFixedViewport|undefined,right:ReaderFixedViewport|undefined)=>left===right||!!left&&!!right
    &&left.page===right.page&&left.pageWidth===right.pageWidth&&left.pageHeight===right.pageHeight&&left.scale===right.scale
    &&left.x===right.x&&left.y===right.y&&left.width===right.width&&left.height===right.height;
  const locate=(next=currentViewport())=>{reportedViewport=next;
    return location={format,page,progression:pageCount===1?0:page/(pageCount-1),...(next?{viewport:next}:{})};};
  // Pan geometry changes at pointer/scroll rate; one publish per frame at most.
  const scheduleViewport=()=>{if(viewportFrame!==undefined||closed)return;
    viewportFrame=requestAnimationFrame(()=>{viewportFrame=undefined;publishViewport();});};
  const publishViewport=()=>{if(closed)return;const next=currentViewport();
    if(!sameViewport(reportedViewport,next))options.onLocation(locate(next));};
  const surfacesByPage=new Map<number,PageSurface>();
  const lifetime=new AbortController(),scope=abortScope([lifetime.signal,options.signal]),signal=scope.signal;
  const direction=():'ltr'|'rtl'|'vertical'=>{const value:FixedPreferences['comicDirection']=preferences.comicDirection;return format!=='pdf'&&value!=='file'?value:fileDirection;};
  const fit=():'auto'|'page'|'width'|'height'|'spread'=>{
    if(zoom?.kind==='fit')return zoom.value;
    if(format==='pdf'){const value:FixedPreferences['pdfView']=preferences.pdfView;return value==='automatic'?'auto':value;}
    return preferences.comicFit;
  };
  const plan=(at=page)=>{
    const layout=planFixedLayout({format,viewport,page:at,pageCount,twoPages:preferences.twoPages,firstPageAlone:preferences.firstPageAlone,direction:direction(),fit:fit()});
    if(layout.direction==='vertical'&&fit()==='auto')return {...layout,fit:'width' as const};
    return format==='pdf'&&preferences.pdfView==='automatic'&&zoom?.kind!=='fit'?{...layout,fit:layout.spread?'spread' as const:'page' as const}:layout;
  };
  const clean=()=>{for(const fn of cleanup.splice(0))fn();};
  // motion.html 5.1: a tap or key turn slides the outgoing and incoming spreads together (slow 320,
  // out); reduced motion cross-fades them in 160 ms. The outgoing surfaces are released only when the
  // slide ends, and a new turn first finishes the running one. Zoom, resize and continuous scrolling
  // keep the immediate swap.
  let turning:{finish:()=>void;defer:(fn:()=>void)=>void}|undefined;
  const reducedMotion=()=>{try{return matchMedia('(prefers-reduced-motion: reduce)').matches||document.documentElement.dataset.reduceMotion==='true';}catch{return true;}};
  const swap=(next:HTMLDivElement,releases:(()=>void)[],turn:number)=>{
    turning?.finish();
    const previous=turn?host.firstElementChild as HTMLElement|null:null;
    if(!previous||typeof next.animate!=='function'||typeof previous.animate!=='function'){clean();cleanup=releases;host.replaceChildren(next);return;}
    const outgoing=cleanup.splice(0);cleanup=releases;
    const from=previous.getBoundingClientRect(),box=host.getBoundingClientRect();
    Object.assign(previous.style,{position:'absolute',left:`${from.left-box.left+host.scrollLeft}px`,top:`${from.top-box.top+host.scrollTop}px`,margin:'0',pointerEvents:'none'});
    host.append(next);
    const reduce=reducedMotion(),width=host.clientWidth||box.width;
    const timing={duration:reduce?160:320,easing:reduce?'linear':'cubic-bezier(.2,.8,.2,1)'};
    const leaving=previous.animate(reduce?[{opacity:1},{opacity:0}]:[{transform:'none'},{transform:`translateX(${-turn*width}px)`}],timing);
    const arriving=next.animate(reduce?[{opacity:0},{opacity:1}]:[{transform:`translateX(${turn*width}px)`},{transform:'none'}],timing);
    let done=false;
    const handle={finish:()=>{if(done)return;done=true;leaving.cancel();arriving.cancel();previous.remove();for(const fn of outgoing)fn();if(turning===handle)turning=undefined;},defer:(fn:()=>void)=>{if(done)fn();else outgoing.push(fn);}};
    turning=handle;arriving.onfinish=handle.finish;
  };
  const turnFrom=(at:number,vertical:boolean)=>vertical||at===page||!host.firstElementChild?0:Math.sign(at-page)*(direction()==='rtl'?-1:1);
  async function render(at:number,taskSignal:AbortSignal,anchorOffset?:number):Promise<void>{
    if(closed)throw new Error('reader_closed');throwIfAborted(taskSignal);
    if(!Number.isInteger(at)||at<0||at>=pageCount)throw new Error('page_invalid');
    pending?.abort();const controller=new AbortController();pending=controller;const current=++version;
    const task=abortScope([signal,controller.signal,taskSignal]),operation=task.signal,layout=plan(at),next=document.createElement('div'),releases:(()=>void)[]=[],created=new Map<number,PageSurface>();
    // Continuous comics retain only actual adjacent pages. There are no blank
    // estimated-height pages, fabricated content, or whole-archive downloads.
    const vertical=layout.direction==='vertical',visualPages=vertical?Array.from({length:Math.min(pageCount-1,at+2)-Math.max(0,at-1)+1},(_,i)=>Math.max(0,at-1)+i):layout.visualPages;
    rendering++;
    try{
      const surfaces:PageSurface[]=[];
      for(const index of visualPages){
        let surface=surfacesByPage.get(index);
        if(!surface){surface=await load(index,{width:Math.max(32,Math.min(2048,Math.ceil(viewport.width/layout.visualPages.length))),signal:operation});if(operation.aborted){surface.dispose();throw abortReason(operation);}created.set(index,surface);}
        throwIfAborted(operation);surfaces.push(surface);
      }
      // A continuous column has one stable page width despite different scan
      // resolutions. Each page retains its actual aspect ratio.
      const fold=!vertical?validReaderCrease(viewport):undefined;
      if(fold){
        // foldable6: physical panes around the reported crease. A cover or a last
        // page sits on its own recto/verso side; each page fits its own pane so
        // unequal aspects never cross the hinge.
        const lone=visualPages.length===1?(preferences.firstPageAlone&&visualPages[0]===0?'cover' as const:visualPages[0]===pageCount-1?'last' as const:'only' as const):undefined;
        const panes=fixedVisualPanes(viewport,visualPages,direction()==='rtl'?'rtl':'ltr',lone);
        Object.assign(next.style,{position:'relative',display:'block',flex:'none',width:`${viewport.width}px`,height:`${viewport.height}px`,minWidth:'0',margin:'0'});
        const panesById=new Map<number,{clip:HTMLDivElement;scale:number}>();
        let applied=0,fittedCurrent=0;
        for(const {page:pageNumber,pane} of panes){
          const index=visualPages.indexOf(pageNumber),surface=surfaces[index];
          const fitAll=Math.min(pane.width/surface.width,pane.height/surface.height);
          const fitted=Math.min(fitAll,layout.fit==='width'?pane.width/surface.width:layout.fit==='height'?pane.height/surface.height:fitAll);
          const renderScale=zoom?.kind==='scale'?zoom.value:fitted;
          if(!Number.isFinite(renderScale)||renderScale<=0)throw new Error('page_invalid');
          const w=surface.width*renderScale,h=surface.height*renderScale,clip=document.createElement('div'),content=document.createElement('div'),target=document.createElement('div');
          clip.dataset.readerPage=String(pageNumber);
          // The pane clips at the hinge, and scrolls the page inside itself so a
          // zoomed page can be panned to every edge without crossing the crease.
          Object.assign(clip.style,{position:'absolute',left:`${pane.x}px`,top:'0px',width:`${pane.width}px`,height:`${pane.height}px`,overflow:'auto',scrollbarWidth:'none'});
          const cw=Math.max(pane.width,w),ch=Math.max(pane.height,h);
          Object.assign(content.style,{position:'relative',width:`${cw}px`,height:`${ch}px`});
          Object.assign(target.style,{position:'absolute',left:`${(cw-w)/2}px`,top:`${(ch-h)/2}px`,width:`${w}px`,height:`${h}px`});
          content.append(target);clip.append(content);next.append(clip);panesById.set(pageNumber,{clip,scale:renderScale});
          releases.push(await surface.mount(target,renderScale,operation));throwIfAborted(operation);
          if(pageNumber===at){applied=renderScale;fittedCurrent=fitted;}
        }
        if(!(applied>0))throw new Error('page_invalid');
        if(closed||current!==version)throw new DOMException('','AbortError');
        swap(next,releases,turnFrom(at,false));foldPanes=panesById;page=at;scale=applied;fittedScale=fittedCurrent;verticalWidth=undefined;pageGeometry=undefined;
      }else{
      const width=vertical?(verticalWidth??surfaces[visualPages.indexOf(at)].width):surfaces.reduce((sum,s)=>sum+s.width,0),height=vertical?surfaces.reduce((sum,s)=>sum+s.height*width/s.width,0):Math.max(...surfaces.map(s=>s.height));
      const currentSurface=surfaces[visualPages.indexOf(at)],fitHeight=vertical?currentSurface.height*width/currentSurface.width:height;
      const fitted=layout.fit==='width'?viewport.width/width:layout.fit==='height'?viewport.height/fitHeight:Math.min(viewport.width/width,viewport.height/fitHeight);
      const requested=zoom?.kind==='scale'?zoom.value:fitted;
      if(!Number.isFinite(requested)||requested<=0||!Number.isFinite(width)||!Number.isFinite(height))throw new Error('page_invalid');
      Object.assign(next.style,{position:'relative',display:'flex',flexDirection:vertical?'column':'row',flex:'none',alignItems:'center',width:`${width*requested}px`,height:`${height*requested}px`,minWidth:'0',margin:'auto'});
      for(let i=0;i<surfaces.length;i++){
        const surface=surfaces[i],renderScale=vertical?requested*width/surface.width:requested,target=document.createElement('div');target.dataset.readerPage=String(visualPages[i]);
        Object.assign(target.style,{position:'relative',flex:'none',width:`${surface.width*renderScale}px`,height:`${surface.height*renderScale}px`,overflow:'hidden'});next.append(target);
        releases.push(await surface.mount(target,renderScale,operation));throwIfAborted(operation);
      }
      if(closed||current!==version)throw new DOMException('','AbortError');
      swap(next,releases,turnFrom(at,vertical));foldPanes=undefined;page=at;scale=requested;fittedScale=fitted;if(vertical)verticalWidth=width;
      pageGeometry=!vertical&&surfaces.length===1?{page:at,width:surfaces[0].width,height:surfaces[0].height,scale:requested,fittedScale:fitted}:undefined;
      }
      for(const [index,surface]of created)surfacesByPage.set(index,surface);
      for(const [index,surface]of surfacesByPage)if(!visualPages.includes(index)){const owner=turning;surfacesByPage.delete(index);if(owner)owner.defer(()=>surface.dispose());else surface.dispose();}
      if(foldPanes){for(const {clip} of foldPanes.values()){clip.scrollLeft=0;clip.scrollTop=0;}}
      else{host.scrollLeft=direction()==='rtl'?host.scrollWidth-host.clientWidth:0;host.scrollTop=vertical?(next.querySelector<HTMLElement>(`[data-reader-page="${at}"]`)!.offsetTop+(anchorOffset??0)):0;}
      location=locate();options.onLocation(location);
    }catch(error){for(const release of releases.reverse())release();for(const surface of created.values())surface.dispose();throw error;}finally{rendering--;task.dispose();}
  }
  const pointers=new Map<number,{x:number;y:number}>();let pinch:number|undefined,drag:{x:number;y:number;left:number;top:number}|undefined,tap:{id:number;x:number;y:number;multiple:boolean;moved:boolean}|undefined;
  let pendingTap:{timer:ReturnType<typeof setTimeout>;x:number;y:number;time:number}|undefined;
  const clearTap=()=>{if(pendingTap){clearTimeout(pendingTap.timer);pendingTap=undefined;}};
  const gestureZoom=(next:Zoom)=>{void view.setZoom!(next,signal).then(()=>{if(!closed)options.onSurfaceInput?.({kind:'activity'});}).catch(()=>{});};
  const distance=()=>{const a=[...pointers.values()];return a.length===2?Math.hypot(a[1].x-a[0].x,a[1].y-a[0].y):undefined;};
  const onDown=(event:PointerEvent)=>{
    // A completed centre tap owns the chrome toggle. Do not wake it on
    // contact first, which would invert the user's hidden/shown intention.
    if(!tap)tap={id:event.pointerId,x:event.clientX,y:event.clientY,multiple:false,moved:false};else{tap.multiple=true;clearTap();options.onSurfaceInput?.({kind:'activity'});}
    // PDF text selection must retain browser handles and native selection semantics.
    if(event.pointerType!=='touch'&&(event.target as Element)?.closest?.('[data-reader-text] span'))return;
    pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});try{host.setPointerCapture?.(event.pointerId);}catch{/* synthetic pointers */}
    if(pointers.size===1){const hit=(event.target as Element|null)?.closest?.('[data-reader-page]');const clip=hit instanceof HTMLElement?foldPanes?.get(Number(hit.dataset.readerPage))?.clip:undefined;const pane=clip&&clip===hit?clip:undefined;dragPane=pane;gesturePage=pane?Number(pane.dataset.readerPage):undefined;const at=pane?{left:pane.scrollLeft,top:pane.scrollTop}:scrollState();drag={x:event.clientX,y:event.clientY,left:at.left,top:at.top};}else{pinch=distance();drag=undefined;dragPane=undefined;}
  };
  const onMove=(event:PointerEvent)=>{
    if(!pointers.has(event.pointerId))return;
    if(tap?.id===event.pointerId&&Math.hypot(event.clientX-tap.x,event.clientY-tap.y)>=8)tap.moved=true;
    if(pointers.size>1||tap?.moved)options.onSurfaceInput?.({kind:'activity'});
    pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});const nextDistance=distance();
    if(pinch&&nextDistance){const next=Math.max(.05,Math.min(8,scale*nextDistance/pinch));pinch=nextDistance;gestureZoom({kind:'scale',value:next});}
    else if(drag&&tap?.moved){if(dragPane){dragPane.scrollLeft=Math.max(0,drag.left+drag.x-event.clientX);dragPane.scrollTop=Math.max(0,drag.top+drag.y-event.clientY);}else scrollTo(drag.left+drag.x-event.clientX,drag.top+drag.y-event.clientY);scheduleViewport();}
  };
  const onUp=(event:PointerEvent)=>{dragPane=undefined;
    pointers.delete(event.pointerId);pinch=undefined;drag=undefined;if(!pointers.size)gesturePage=undefined;
    if(tap?.id===event.pointerId){
      const candidate=tap;tap=undefined;
      if(!candidate.multiple&&!candidate.moved&&event.type==='pointerup'&&Math.hypot(event.clientX-candidate.x,event.clientY-candidate.y)<8&&document.getSelection()?.isCollapsed!==false){
        const rect=host.getBoundingClientRect();if(rect.width<=0)return;
        if(pendingTap&&event.timeStamp-pendingTap.time<=300&&Math.hypot(event.clientX-pendingTap.x,event.clientY-pendingTap.y)<24){clearTap();onDouble();}
        else{
          clearTap();const fraction=Math.max(0,Math.min(1,(event.clientX-rect.left)/rect.width)),atVersion=version;
          const timer=setTimeout(()=>{pendingTap=undefined;if(!closed&&version===atVersion)options.onSurfaceInput?.({kind:'tap',fraction});},300);
          pendingTap={timer,x:event.clientX,y:event.clientY,time:event.timeStamp};
        }
      }
    }
  };
  const onKey=(event:KeyboardEvent)=>{
    const target=event.target as HTMLElement;
    options.onSurfaceInput?.({kind:'key',key:event.key,shift:event.shiftKey,alt:event.altKey,ctrl:event.ctrlKey,meta:event.metaKey,editable:!!target.closest?.('input,textarea,select,[contenteditable=true]')||document.getSelection()?.isCollapsed===false,preventDefault:()=>event.preventDefault()});
  };
  const onWheel=(event:WheelEvent)=>{if(!event.ctrlKey)return;event.preventDefault();gestureZoom({kind:'scale',value:Math.max(.05,Math.min(8,scale*Math.exp(-event.deltaY*.002)))});};
  const onDouble=()=>{gestureZoom(zoom?.kind==='scale'?{kind:'fit',value:'page'}:{kind:'scale',value:Math.min(8,scale*2)});};
  const view:ReaderView<P>={
    async navigate(target,taskSignal){
      mutation++;clearTap();
      let at=page;
      if(target.kind==='next')at=plan().nextPage??page;
      else if(target.kind==='previous')at=plan().previousPage??page;
      else if(target.kind==='progression'){if(!Number.isFinite(target.value)||target.value<0||target.value>1)throw new Error('page_invalid');at=Math.round(target.value*(pageCount-1));}
      else if(target.kind==='location'){if(target.location.format==='epub'||target.location.format!==format)throw new Error('page_invalid');at=target.location.page;}
      else throw new Error('page_invalid');
      await render(at,taskSignal);
    },
    async applyPreferences(next,taskSignal){
      throwIfAborted(taskSignal);const own=++mutation,previous=preferences,before=plan(),held=scrollState();preferences=next;
      const after=plan(),samePages=before.visualPages.join(',')===after.visualPages.join(',');
      if(samePages&&before.fit===after.fit&&before.direction===after.direction)return;
      try{await render(page,taskSignal);if(samePages&&zoom?.kind==='scale'){scrollTo(held.left,held.top,page);publishViewport();}}
      catch(error){if(own===mutation)preferences=previous;throw error;}
    },
    async resize(next,taskSignal){const own=++mutation,previous=viewport;viewport=next;try{await render(page,taskSignal);}catch(error){if(own===mutation)viewport=previous;throw error;}},
    getLocation(){return location;},
    async setZoom(next,taskSignal){
      if(next.kind==='scale'&&(!Number.isFinite(next.value)||next.value<.05||next.value>8)||next.kind==='fit'&&!['page','width','height','spread'].includes(next.value))throw new Error('zoom_invalid');
      const own=++mutation,previous=zoom,anchor=gesturePage??page,held=scrollState(anchor),x=(held.left+held.width/2)/scale,y=(held.top+held.height/2)/scale;zoom=next;
      try{await render(page,taskSignal);scrollTo(x*scale-held.width/2,y*scale-held.height/2,anchor);publishViewport();}catch(error){if(own===mutation)zoom=previous;throw error;}
    },
    getZoom(){return {scale,...zoom?.kind==='scale'?{}:{fit:plan().fit}};},
    canTurnAtFit(){return zoom?.kind!=='scale'||Math.abs(scale-fittedScale)<=1e-6;},
    async pan(position,taskSignal){throwIfAborted(taskSignal);if(closed||!Number.isFinite(position.x)||!Number.isFinite(position.y))throw new Error('pan_invalid');scrollTo(position.x,position.y,page);publishViewport();},
    dispose(){if(closed)return;closed=true;version++;turning?.finish();clearTap();if(viewportFrame!==undefined){cancelAnimationFrame(viewportFrame);viewportFrame=undefined;}pending?.abort();lifetime.abort();scope.dispose();clean();for(const surface of surfacesByPage.values())surface.dispose();surfacesByPage.clear();paneStyle.remove();host.remove();}
  };
  const onScroll=()=>{
    clearTap();if(closed||rendering||scrolling)return;options.onSurfaceInput?.({kind:'activity'});scheduleViewport();
    if(direction()!=='vertical')return;
    const bounds=host.getBoundingClientRect();let greatest=0,visible:HTMLElement|undefined;
    for(const element of host.querySelectorAll<HTMLElement>('[data-reader-page]')){
      const rect=element.getBoundingClientRect(),intersection=Math.max(0,Math.min(bounds.bottom,rect.bottom)-Math.max(bounds.top,rect.top));
      if(intersection>greatest){greatest=intersection;visible=element;}
    }
    if(!visible)return;const at=Number(visible.dataset.readerPage);if(at===page)return;
    const offset=host.scrollTop-visible.offsetTop;page=at;location=locate();options.onLocation(location);
    mutation++;scrolling=true;
    void render(at,signal,offset).then(()=>{if(!closed)options.onSurfaceInput?.({kind:'activity'});}).catch(()=>{}).finally(()=>{scrolling=false;});
  };
  host.addEventListener('pointerdown',onDown,{signal});host.addEventListener('pointermove',onMove,{signal});host.addEventListener('pointerup',onUp,{signal});host.addEventListener('pointercancel',onUp,{signal});host.addEventListener('wheel',onWheel,{passive:false,signal});
  host.addEventListener('keydown',onKey,{signal});host.addEventListener('focusin',()=>options.onSurfaceInput?.({kind:'activity'}),{signal});
  host.addEventListener('scroll',onScroll,{passive:true,signal});
  signal.addEventListener('abort',()=>{void view.dispose();},{once:true});
  try{await render(0,signal);throwIfAborted(signal);options.container.append(host);return view;}catch(error){await view.dispose();throw error;}
}
