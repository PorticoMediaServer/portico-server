import {createRoot} from 'react-dom/client';
import {WindowChrome} from './WindowChrome';
import {createDesktopWindowController} from './window-controller';
import {nativeWindowBridge,type ChromeUpdate,type WindowAction,type WindowBridge,type WindowChromeHostState,type WindowSnapshot} from './window';

/** Native host for the canonical MAIN window-chrome installer. */
export function createDesktopWindowChromeHost({onEnvironment,bridge=nativeWindowBridge,prepareToClose}:{onEnvironment:(value:{os:WindowSnapshot['os'];fullscreen:boolean})=>void;bridge?:WindowBridge;prepareToClose?:()=>Promise<void>}) {
  let disposed=false;
  const mount=document.createElement('div');mount.dataset.desktopWindowChrome='';document.body.append(mount);
  const root=createRoot(mount);
  const report=()=>console.warn('window_unavailable');
  const geometry=(value:ChromeUpdate['captions'])=>controller.geometry(value);
  const action=(value:WindowAction)=>{void controller.action(value);};
  const controller=createDesktopWindowController({bridge,onEnvironment,onError:report,prepareToClose,
    onSnapshot:(native,state)=>root.render(<WindowChrome native={native} state={state} onGeometry={geometry} onAction={action}/>),
  });
  const interactive='a,button,input,textarea,select,summary,[contenteditable]:not([contenteditable=false]),[role=button],[role=link],[data-window-no-drag]';
  const dragRegion=(event:MouseEvent)=>{
    const native=controller.native(),state=controller.state();
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
    if(controller.native()?.os==='macos'&&event.detail===2){macDouble={x:event.clientX,y:event.clientY};return;}
    action(event.detail===2?'toggleMaximize':'drag');
  };
  const up=(event:MouseEvent)=>{
    const start=macDouble;macDouble=undefined;
    if(start&&event.detail===2&&event.clientX===start.x&&event.clientY===start.y&&dragRegion(event))action('toggleMaximize');
  };
  document.addEventListener('mousedown',down,true);document.addEventListener('mouseup',up,true);
  const timer=setInterval(()=>{void controller.read();},200);
  void controller.read();
  return {
    update(next:WindowChromeHostState){controller.update(next);},
    dispose(){
      if(disposed)return;disposed=true;clearInterval(timer);document.removeEventListener('mousedown',down,true);document.removeEventListener('mouseup',up,true);
      // Native restoration must precede removal of the drawn controls.
      void controller.dispose().then(()=>{root.unmount();mount.remove();}).catch(report);
    },
  };
}
