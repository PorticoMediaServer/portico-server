import {createRoot} from 'react-dom/client';
import {WindowChrome} from '../src/WindowChrome';
import type {WindowSnapshot,ChromeUpdate} from '../src/window';

/** Controlled visual fixture: only the production caption component is under test.
 * The surrounding immutable reference is a scaffold, never a product Home claim. */
export function mountCaption(parent:HTMLElement,native:WindowSnapshot){
 const mount=document.createElement('div');
 // Keep the reference window's real clipping and stacking context. Only the
 // fixture's coordinate system changes; production caption paint stays untouched.
 mount.style.cssText='position:absolute;inset:0;transform:translateZ(0)';parent.append(mount);
 const root=createRoot(mount);let geometry:ChromeUpdate['captions']=null;
 const actions:string[]=[];
 root.render(<WindowChrome native={native} state={{surface:'pages',controlsVisible:true,fullscreen:false}} onGeometry={value=>{geometry=value}} onAction={value=>{actions.push(value)}}/>);
 return {geometry:()=>geometry,actions,dispose(){root.unmount();mount.remove();}};
}
