import {useLayoutEffect,useRef} from 'react';
import type {ChromeUpdate,WindowAction,WindowChromeHostState,WindowSnapshot} from './window';
import styles from './window.module.css';

/** Only Windows draws its caption controls. AppKit and the Linux desktop own theirs.
 * The four SVG paths and dimensions are desktop-app.html at design 812dacac. */
export function WindowChrome({native,state,onAction,onGeometry}:{native:WindowSnapshot;state:WindowChromeHostState;onAction:(action:WindowAction)=>void;onGeometry:(rects:ChromeUpdate['captions'])=>void}) {
  const minimize=useRef<HTMLButtonElement>(null),maximize=useRef<HTMLButtonElement>(null),close=useRef<HTMLButtonElement>(null);
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
    {state.surface==='pages'&&state.scrolled&&<div className={styles.shade} aria-hidden="true"/>}
    <div className={styles.caption} data-window-no-drag data-focused={native.focused}>
      <button ref={minimize} type="button" aria-label={native.captions.minimize} data-hover={native.captionHover===1} onClick={()=>onAction('minimize')}>
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M0 5.5h10" stroke="currentColor" strokeWidth="1" fill="none"/></svg>
      </button>
      <button ref={maximize} type="button" aria-label={native.maximized?native.captions.restore:native.captions.maximize} data-hover={native.captionHover===2} onClick={()=>onAction('toggleMaximize')}>
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">{native.maximized?<g stroke="currentColor" strokeWidth="1" fill="none"><rect x=".5" y="2.5" width="7" height="7" rx="1"/><path d="M2.5 2.5V1.5a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1h-1"/></g>:<rect x=".5" y=".5" width="9" height="9" rx="1" stroke="currentColor" strokeWidth="1" fill="none"/>}</svg>
      </button>
      <button ref={close} type="button" className={styles.close} aria-label={native.captions.close} data-hover={native.captionHover===3} onClick={()=>onAction('close')}>
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M.5.5l9 9M9.5.5l-9 9" stroke="currentColor" strokeWidth="1" fill="none"/></svg>
      </button>
    </div>
  </>;
}
