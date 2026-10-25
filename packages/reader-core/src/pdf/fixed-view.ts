import {throwSignalAborted as throwIfAborted} from '../abort.ts';
import {abortScope,abortReason} from '../abort.ts';
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
  host.tabIndex=0;host.setAttribute('role','document');
  host.dataset.readerFixed=format;Object.assign(host.style,{position:'relative',display:'flex',width:'100%',height:'100%',overflow:'auto',overscrollBehavior:'contain',touchAction:'none'});
  let preferences=options.preferences,viewport=options.viewport,page=0,closed=false,version=0,mutation=0,rendering=0,scrolling=false;
  let zoom:Zoom|undefined,scale=1,fittedScale=1,verticalWidth:number|undefined,location:ReaderLocation|null=null,cleanup:(()=>void)[]=[],pending:AbortController|undefined;
  const surfacesByPage=new Map<number,PageSurface>();
  const lifetime=new AbortController(),scope=abortScope([lifetime.signal,options.signal]),signal=scope.signal;
  const direction=():'ltr'|'rtl'|'vertical'=>{const value:FixedPreferences['comicDirection']=preferences.comicDirection;return format==='cbz'&&value!=='file'?value:fileDirection;};
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
      clean();cleanup=releases;host.replaceChildren(next);page=at;scale=requested;fittedScale=fitted;if(vertical)verticalWidth=width;
      for(const [index,surface]of created)surfacesByPage.set(index,surface);
      for(const [index,surface]of surfacesByPage)if(!visualPages.includes(index)){surface.dispose();surfacesByPage.delete(index);}
      host.scrollLeft=direction()==='rtl'?host.scrollWidth-host.clientWidth:0;host.scrollTop=vertical?(next.querySelector<HTMLElement>(`[data-reader-page="${at}"]`)!.offsetTop+(anchorOffset??0)):0;
      location={format,page,progression:pageCount===1?0:page/(pageCount-1)};options.onLocation(location);
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
    pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});host.setPointerCapture?.(event.pointerId);
    if(pointers.size===1)drag={x:event.clientX,y:event.clientY,left:host.scrollLeft,top:host.scrollTop};else{pinch=distance();drag=undefined;}
  };
  const onMove=(event:PointerEvent)=>{
    if(!pointers.has(event.pointerId))return;
    if(tap?.id===event.pointerId&&Math.hypot(event.clientX-tap.x,event.clientY-tap.y)>=8)tap.moved=true;
    if(pointers.size>1||tap?.moved)options.onSurfaceInput?.({kind:'activity'});
    pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});const nextDistance=distance();
    if(pinch&&nextDistance){const next=Math.max(.05,Math.min(8,scale*nextDistance/pinch));pinch=nextDistance;gestureZoom({kind:'scale',value:next});}
    else if(drag&&tap?.moved){host.scrollLeft=drag.left+drag.x-event.clientX;host.scrollTop=drag.top+drag.y-event.clientY;}
  };
  const onUp=(event:PointerEvent)=>{
    pointers.delete(event.pointerId);pinch=undefined;drag=undefined;
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
      throwIfAborted(taskSignal);const own=++mutation,previous=preferences,before=plan(),left=host.scrollLeft,top=host.scrollTop;preferences=next;
      const after=plan(),samePages=before.visualPages.join(',')===after.visualPages.join(',');
      if(samePages&&before.fit===after.fit&&before.direction===after.direction)return;
      try{await render(page,taskSignal);if(samePages&&zoom?.kind==='scale'){host.scrollLeft=left;host.scrollTop=top;}}
      catch(error){if(own===mutation)preferences=previous;throw error;}
    },
    async resize(next,taskSignal){const own=++mutation,previous=viewport;viewport=next;try{await render(page,taskSignal);}catch(error){if(own===mutation)viewport=previous;throw error;}},
    getLocation(){return location;},
    async setZoom(next,taskSignal){
      if(next.kind==='scale'&&(!Number.isFinite(next.value)||next.value<.05||next.value>8)||next.kind==='fit'&&!['page','width','height','spread'].includes(next.value))throw new Error('zoom_invalid');
      const own=++mutation,previous=zoom,x=(host.scrollLeft+viewport.width/2)/scale,y=(host.scrollTop+viewport.height/2)/scale;zoom=next;
      try{await render(page,taskSignal);host.scrollLeft=Math.max(0,x*scale-viewport.width/2);host.scrollTop=Math.max(0,y*scale-viewport.height/2);}catch(error){if(own===mutation)zoom=previous;throw error;}
    },
    getZoom(){return {scale,...zoom?.kind==='scale'?{}:{fit:plan().fit}};},
    canTurnAtFit(){return zoom?.kind!=='scale'||Math.abs(scale-fittedScale)<=1e-6;},
    async pan(position,taskSignal){throwIfAborted(taskSignal);if(closed||!Number.isFinite(position.x)||!Number.isFinite(position.y))throw new Error('pan_invalid');host.scrollLeft=Math.max(0,position.x);host.scrollTop=Math.max(0,position.y);},
    dispose(){if(closed)return;closed=true;version++;clearTap();pending?.abort();lifetime.abort();scope.dispose();clean();for(const surface of surfacesByPage.values())surface.dispose();surfacesByPage.clear();host.remove();}
  };
  const onScroll=()=>{
    clearTap();if(closed||rendering||scrolling)return;options.onSurfaceInput?.({kind:'activity'});
    if(direction()!=='vertical')return;
    const bounds=host.getBoundingClientRect();let greatest=0,visible:HTMLElement|undefined;
    for(const element of host.querySelectorAll<HTMLElement>('[data-reader-page]')){
      const rect=element.getBoundingClientRect(),intersection=Math.max(0,Math.min(bounds.bottom,rect.bottom)-Math.max(bounds.top,rect.top));
      if(intersection>greatest){greatest=intersection;visible=element;}
    }
    if(!visible)return;const at=Number(visible.dataset.readerPage);if(at===page)return;
    const offset=host.scrollTop-visible.offsetTop;page=at;location={format,page,progression:pageCount===1?0:page/(pageCount-1)};options.onLocation(location);
    mutation++;scrolling=true;
    void render(at,signal,offset).then(()=>{if(!closed)options.onSurfaceInput?.({kind:'activity'});}).catch(()=>{}).finally(()=>{scrolling=false;});
  };
  host.addEventListener('pointerdown',onDown,{signal});host.addEventListener('pointermove',onMove,{signal});host.addEventListener('pointerup',onUp,{signal});host.addEventListener('pointercancel',onUp,{signal});host.addEventListener('wheel',onWheel,{passive:false,signal});
  host.addEventListener('keydown',onKey,{signal});host.addEventListener('focusin',()=>options.onSurfaceInput?.({kind:'activity'}),{signal});
  host.addEventListener('scroll',onScroll,{passive:true,signal});
  signal.addEventListener('abort',()=>{void view.dispose();},{once:true});
  try{await render(0,signal);throwIfAborted(signal);options.container.append(host);return view;}catch(error){await view.dispose();throw error;}
}
