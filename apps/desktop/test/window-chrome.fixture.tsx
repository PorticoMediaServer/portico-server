import {createRoot} from 'react-dom/client';
import {useLayoutEffect} from 'react';
import {WindowChrome} from '../src/WindowChrome';
import type {WindowSnapshot,ChromeUpdate} from '../src/window';

/** Controlled visual fixture: only the production caption component is under test.
 * The surrounding immutable reference is a scaffold, never a product Home claim. */
export function mountCaption(parent:HTMLElement,native:WindowSnapshot){
 const mount=document.createElement('div');
 // Match the production body mount and its single fixed stacking context. A
 // contained wrapper has an extra stacking context absent from the actual host.
 document.body.append(mount);const root=createRoot(mount);
 let geometry:ChromeUpdate['captions']=null;
 const actions:string[]=[];
 function FramedCaption(){
  useLayoutEffect(()=>{
   const caption=mount.querySelector<HTMLElement>('[data-window-no-drag]');
   if(caption){
    const box=parent.getBoundingClientRect();
    // Translate the window viewport's origin only; no glyph offsets or paint overrides.
    caption.style.top=`${box.top}px`;caption.style.right=`${innerWidth-box.right}px`;
    window.dispatchEvent(new Event('resize'));
   }
  },[]);
  return <WindowChrome native={native} state={{surface:'pages',controlsVisible:true,fullscreen:false}} onGeometry={value=>{geometry=value}} onAction={value=>{actions.push(value)}}/>;
 }
 root.render(<FramedCaption/>);
 return {geometry:()=>geometry,actions,dispose(){root.unmount();mount.remove();}};
}
