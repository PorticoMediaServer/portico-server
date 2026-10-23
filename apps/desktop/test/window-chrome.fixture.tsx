import {createRoot} from 'react-dom/client';
import {WindowChrome} from '../src/WindowChrome';
import type {WindowSnapshot,ChromeUpdate} from '../src/window';

/** Controlled visual fixture: only the production caption component is under test.
 * The surrounding immutable reference is a scaffold, never a product Home claim. */
export function mountCaption(parent:HTMLElement,native:WindowSnapshot){
 const mount=document.createElement('div');
 const box=parent.getBoundingClientRect(),frame=parent.closest('.win');
 // The real host mounts beside the app on body, not under its page's .p reset.
 // Only this reference-coordinate containing block substitutes for the OS window.
 Object.assign(mount.style,{position:'absolute',left:`${box.left+scrollX}px`,top:`${box.top+scrollY}px`,width:`${box.width}px`,height:`${box.height}px`,contain:'layout',zIndex:'60',pointerEvents:'none',overflow:'hidden',borderRadius:frame?getComputedStyle(frame).borderRadius:'0'});
 document.body.append(mount);const root=createRoot(mount);
 let geometry:ChromeUpdate['captions']=null;
 const actions:string[]=[];
 root.render(<WindowChrome native={native} state={{surface:'pages',controlsVisible:true,fullscreen:false}} onGeometry={value=>{geometry=value}} onAction={value=>{actions.push(value)}}/>);
 return {geometry:()=>geometry,actions,dispose(){root.unmount();mount.remove();}};
}
