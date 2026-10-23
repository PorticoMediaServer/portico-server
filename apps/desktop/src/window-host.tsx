import {createRoot} from 'react-dom/client';
import {WindowChrome} from './WindowChrome';
import {nativeWindowBridge,type ChromeUpdate,type WindowAction,type WindowBridge,type WindowChromeHostState,type WindowSnapshot} from './window';

/** App-owned host for the proposed canonical installWindowChromeHost seam. No canonical
 * source or another lane's WIP is imported. The entry installs it after the checked seam lands. */
export function createDesktopWindowChromeHost({onEnvironment,bridge=nativeWindowBridge,prepareToClose}:{onEnvironment:(value:{os:WindowSnapshot['os'];fullscreen:boolean})=>void;bridge?:WindowBridge;prepareToClose?:()=>Promise<void>}) {
  let state:WindowChromeHostState={surface:'pages',controlsVisible:true,fullscreen:false,scrolled:false};
  let native:WindowSnapshot|undefined,disposed=false,reading=false;
  let captions:ChromeUpdate['captions']=null,pending:ChromeUpdate|undefined,sending:Promise<void>|undefined;
  let environmentKey='';
  const mount=document.createElement('div');mount.dataset.desktopWindowChrome='';document.body.append(mount);
  const root=createRoot(mount);
  const report=()=>console.warn('window_unavailable');
  function send() {
    if (sending) return;
    sending=(async()=>{while(pending){const value=pending;pending=undefined;await bridge.chrome(value);}})()
      .catch(()=>{pending=undefined;report();}).finally(()=>{sending=undefined;});
  }
  const publish=()=>{
    if (!native) return;
    pending={enabled:!disposed,surface:state.surface,controlsVisible:state.controlsVisible,fullscreen:state.fullscreen||native.fullscreen,captions:disposed?null:captions};
    void send();
  };
  const geometry=(value:ChromeUpdate['captions'])=>{captions=value;publish();};
  const action=(value:WindowAction)=>{
    if (disposed&&!mount.isConnected) return;
    void (async()=>{if(value==='close')await prepareToClose?.();await bridge.action(value);await read();})().catch(report);
  };
  function render() {
    if (!native||disposed) return;
    const fullscreen=native.fullscreen||state.fullscreen;
    const key=native.os+':'+fullscreen;
    if(key!==environmentKey){environmentKey=key;onEnvironment({os:native.os,fullscreen});}
    root.render(<WindowChrome native={native} state={state} onGeometry={geometry} onAction={action}/>);
  }
  async function read() {
    if (reading||disposed) return;
    reading=true;
    try {const next=await bridge.read();if(!disposed){native=next;render();publish();}}
    catch {report();}
    finally {reading=false;}
  }
  const interactive='a,button,input,textarea,select,summary,[contenteditable]:not([contenteditable=false]),[role=button],[role=link],[data-window-no-drag]';
  const dragRegion=(event:MouseEvent)=>{
    if (!native||native.os==='linux'||native.fullscreen||state.fullscreen||event.button!==0) return false;
    for (const target of event.composedPath()) {
      if (!(target instanceof Element)) continue;
      if (target.matches(interactive)) return false;
      if (target.hasAttribute('data-window-drag-region')) return true;
    }
    return false;
  };
  let macDouble:{x:number;y:number}|undefined;
  const down=(event:MouseEvent)=>{
    if(!dragRegion(event)||event.detail<1||event.detail>2)return;
    event.preventDefault();event.stopImmediatePropagation();
    if(native?.os==='macos'&&event.detail===2){macDouble={x:event.clientX,y:event.clientY};return;}
    action(event.detail===2?'toggleMaximize':'drag');
  };
  const up=(event:MouseEvent)=>{
    const start=macDouble;macDouble=undefined;
    if(start&&event.detail===2&&event.clientX===start.x&&event.clientY===start.y&&dragRegion(event))action('toggleMaximize');
  };
  document.addEventListener('mousedown',down,true);document.addEventListener('mouseup',up,true);
  const timer=setInterval(()=>{void read();},200);
  void read();
  return {
    update(next:WindowChromeHostState){if(!disposed){state={...next};render();publish();}},
    dispose(){
      if(disposed)return;disposed=true;clearInterval(timer);document.removeEventListener('mousedown',down,true);document.removeEventListener('mouseup',up,true);
      pending=undefined;
      // Native restoration must precede removal of the drawn controls.
      void (async()=>{await sending;await bridge.chrome({enabled:false,surface:'pages',controlsVisible:true,fullscreen:false,captions:null});root.unmount();mount.remove();})().catch(report);
    },
  };
}
