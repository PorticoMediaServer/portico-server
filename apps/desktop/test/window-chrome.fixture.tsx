import {createRoot} from 'react-dom/client';
import {WindowChrome} from '../src/WindowChrome';
import type {WindowSnapshot,ChromeUpdate} from '../src/window';

/** Controlled visual fixture: only the production caption component is under test.
 * The surrounding immutable reference is a scaffold, never a product Home claim. */
export function mountCaption(parent:HTMLElement,native:WindowSnapshot){
 const mount=document.createElement('div');
 Object.assign(mount.style,{position:'absolute',inset:'0',transform:'translateZ(0)',zIndex:'60',pointerEvents:'none'});
 parent.append(mount);const root=createRoot(mount);
 let geometry:ChromeUpdate['captions']=null;
 const actions:string[]=[];
 root.render(<WindowChrome native={native} state={{surface:'pages',controlsVisible:true,fullscreen:false}} onGeometry={value=>{geometry=value}} onAction={value=>{actions.push(value)}}/>);
 return {geometry:()=>geometry,actions,dispose(){root.unmount();mount.remove();}};
}
