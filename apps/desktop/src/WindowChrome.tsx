import {useId,useLayoutEffect,useRef,type ReactNode,type Ref} from 'react';
import type {ChromeUpdate,WindowAction,WindowChromeHostState,WindowSnapshot} from './window';
import styles from './window.module.css';

function CaptionControl({element,label,hover,className,activate,children}:{element:Ref<HTMLSpanElement>;label:string;hover:boolean;className?:string;activate:()=>void;children:ReactNode}){
  const pressed=useRef(false);
  return <span ref={element} role="button" tabIndex={0} aria-label={label} data-hover={hover} className={className}
    onClick={()=>{pressed.current=false;activate();}}
    onBlur={()=>{pressed.current=false;}}
    onKeyDown={event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();event.stopPropagation();if(event.key==='Enter'&&!event.repeat)activate();else if(event.key===' ')pressed.current=true;}}}
    onKeyUp={event=>{if(event.key===' '){event.preventDefault();event.stopPropagation();const active=pressed.current;pressed.current=false;if(active&&document.activeElement===event.currentTarget)activate();}}}>
    {children}
  </span>;
}

/** Only Windows draws its caption controls. AppKit and the Linux desktop own theirs.
 * The four SVG paths and dimensions are desktop-app.html at design 812dacac. */
export function WindowChrome({native,state,onAction,onGeometry}:{native:WindowSnapshot;state:WindowChromeHostState;onAction:(action:WindowAction)=>void;onGeometry:(rects:ChromeUpdate['captions'])=>void}) {
  const symbol=useId();
  const minimize=useRef<HTMLSpanElement>(null),maximize=useRef<HTMLSpanElement>(null),close=useRef<HTMLSpanElement>(null);
  const shown=native.os==='windows'&&!native.fullscreen&&!state.fullscreen&&native.captions!==null;
  useLayoutEffect(()=>{
    if (!shown) {onGeometry(null);return;}
    const project=()=>{
      const rect=(element:HTMLElement)=>{const box=element.getBoundingClientRect();return {x:box.x,y:box.y,width:box.width,height:box.height};};
      if (minimize.current&&maximize.current&&close.current) onGeometry({minimize:rect(minimize.current),maximize:rect(maximize.current),close:rect(close.current)});
    };
    project();window.addEventListener('resize',project,{passive:true});
    return ()=>window.removeEventListener('resize',project);
  },[shown,onGeometry]);
  if (!shown||!native.captions) return null;
  return <>
    <svg width="0" height="0" aria-hidden="true" style={{position:'absolute',overflow:'hidden'}}>
      <defs>
        <symbol id={`${symbol}-min`} viewBox="0 0 10 10"><path d="M0 5.5h10" stroke="currentColor" strokeWidth="1" fill="none"/></symbol>
        <symbol id={`${symbol}-max`} viewBox="0 0 10 10"><rect x=".5" y=".5" width="9" height="9" rx="1" stroke="currentColor" strokeWidth="1" fill="none"/></symbol>
        <symbol id={`${symbol}-rest`} viewBox="0 0 10 10"><g stroke="currentColor" strokeWidth="1" fill="none"><rect x=".5" y="2.5" width="7" height="7" rx="1"/><path d="M2.5 2.5V1.5a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1h-1"/></g></symbol>
        <symbol id={`${symbol}-x`} viewBox="0 0 10 10"><path d="M.5.5l9 9M9.5.5l-9 9" stroke="currentColor" strokeWidth="1" fill="none"/></symbol>
      </defs>
    </svg>
    {state.surface==='pages'&&state.scrolled&&<div className={styles.shade} aria-hidden="true"/>}
    <div className={styles.caption} data-window-no-drag data-focused={native.focused}>
      <CaptionControl element={minimize} label={native.captions.minimize} hover={native.captionHover===1} activate={()=>onAction('minimize')}>
        <svg width="10" height="10" aria-hidden="true"><use href={`#${symbol}-min`}/></svg>
      </CaptionControl>
      <CaptionControl element={maximize} label={native.maximized?native.captions.restore:native.captions.maximize} hover={native.captionHover===2} activate={()=>onAction('toggleMaximize')}>
        <svg width="10" height="10" aria-hidden="true"><use href={`#${symbol}-${native.maximized?'rest':'max'}`}/></svg>
      </CaptionControl>
      <CaptionControl element={close} className={styles.close} label={native.captions.close} hover={native.captionHover===3} activate={()=>onAction('close')}>
        <svg width="10" height="10" aria-hidden="true"><use href={`#${symbol}-x`}/></svg>
      </CaptionControl>
    </div>
  </>;
}
