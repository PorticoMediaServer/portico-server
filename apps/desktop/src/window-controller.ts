import type {ChromeUpdate,WindowAction,WindowBridge,WindowChromeHostState,WindowSnapshot} from './window.ts';

/** Owns the native IPC lifetime. The shell's DOM fullscreen intent must never be
 * fed back as an observed native environment fact. */
export function createDesktopWindowController({bridge,onSnapshot,onEnvironment,onError,prepareToClose}:{
  bridge:WindowBridge;
  onSnapshot:(native:WindowSnapshot,state:WindowChromeHostState)=>void;
  onEnvironment:(environment:{os:WindowSnapshot['os'];fullscreen:boolean})=>void;
  onError:()=>void;
  prepareToClose?:()=>Promise<void>;
}) {
  let state:WindowChromeHostState={surface:'pages',controlsVisible:true,fullscreen:false,scrolled:false};
  let native:WindowSnapshot|undefined,disposed=false,reading=false;
  let captions:ChromeUpdate['captions']=null,pending:ChromeUpdate|undefined,sending:Promise<void>|undefined;
  let environmentKey='',actions=Promise.resolve(),closing:Promise<void>|undefined;
  function send() {
    if (sending) return;
    sending=(async()=>{while(pending&&!disposed){const value=pending;pending=undefined;await bridge.chrome(value);}})()
      .catch(()=>{pending=undefined;onError();}).finally(()=>{sending=undefined;});
  }
  function publish() {
    if (!native||disposed) return;
    pending={enabled:true,surface:state.surface,controlsVisible:state.controlsVisible,fullscreen:state.fullscreen||native.fullscreen,captions};
    send();
  }
  function render() {
    if (!native||disposed) return;
    const key=native.os+':'+native.fullscreen;
    if(key!==environmentKey){environmentKey=key;onEnvironment({os:native.os,fullscreen:native.fullscreen});}
    if (!disposed) onSnapshot(native,state);
  }
  async function read() {
    if (reading||disposed) return;
    reading=true;
    try {const next=await bridge.read();if(!disposed){native=next;render();publish();}}
    catch {if(!disposed)onError();}
    finally {reading=false;}
  }
  return {
    read,
    native:()=>native,
    state:()=>state,
    update(next:WindowChromeHostState){if(!disposed){state={...next};render();publish();}},
    geometry(value:ChromeUpdate['captions']){if(!disposed){captions=value;publish();}},
    action(value:WindowAction){
      if(disposed)return Promise.resolve();
      actions=actions.then(async()=>{
        if(disposed)return;
        if(value==='close')await prepareToClose?.();
        if(disposed)return;
        await bridge.action(value);
        if(!disposed)await read();
      }).catch(()=>{if(!disposed)onError();});
      return actions;
    },
    dispose():Promise<void>{
      if(closing)return closing;
      disposed=true;pending=undefined;
      // Restore native chrome after all older writes and actions, then remove UI.
      closing=(async()=>{await actions;await sending;await bridge.chrome({enabled:false,surface:'pages',controlsVisible:true,fullscreen:false,captions:null});})();
      return closing;
    },
  };
}
