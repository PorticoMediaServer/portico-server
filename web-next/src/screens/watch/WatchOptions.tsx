import {useEffect,useLayoutEffect,useRef} from 'react';
import type {I18n} from '@i18n';
import {Segmented} from '../../kit/controls';
import {TrackChoices} from '../../kit/media';
import {Icon} from '../../kit/Icon';
import {BottomSheet} from '../../kit/overlays';
import type {WatchPanelContext} from './types';
import type {WatchPanelModel,WatchRadioChoice} from './panel-model';
import styles from './panels.module.css';

function RadioColumn({title,rows,select,reportError}:{title:string;rows:readonly WatchRadioChoice[];select(id:string):void|Promise<void>;reportError(error:unknown):void}){
 const buttons=useRef(new Map<string,HTMLButtonElement>()),selected=rows.find(row=>row.selected),first=rows.find(row=>row.enabled);
 const act=(row:WatchRadioChoice)=>{if(!row.enabled)return;try{void Promise.resolve(select(row.id)).catch(reportError);}catch(error){reportError(error);}};
 return <section className={styles.column} aria-label={title}><h3>{title}</h3><div role="radiogroup" aria-label={title}>{rows.map(row=><button key={row.id} type="button" role="radio"
  ref={node=>{if(node)buttons.current.set(row.id,node);else buttons.current.delete(row.id);}} aria-checked={row.selected} aria-disabled={!row.enabled||undefined}
  tabIndex={row.selected||!selected&&row.id===first?.id?0:-1} className={styles.row} data-selected={row.selected||undefined}
  onClick={()=>act(row)} onKeyDown={event=>{
   const direction=['ArrowDown','ArrowRight'].includes(event.key)?1:['ArrowUp','ArrowLeft'].includes(event.key)?-1:0;
   if(!direction)return;event.preventDefault();event.stopPropagation();const enabled=rows.filter(r=>r.enabled),at=enabled.findIndex(r=>r.id===row.id);
   const next=enabled[(at+direction+enabled.length)%enabled.length];if(next){buttons.current.get(next.id)?.focus();act(next);}
  }}><span>{row.label}{row.detail&&<small>{row.detail}</small>}</span><Icon name="i-check" size={20}/></button>)}</div></section>;
}
/** The drawn single options panel. Every visible radio row has actual source membership and
 * every action calls that same runtime with its captured full fence. No optimistic selection. */
export function WatchOptions({model,context,i18n,reportError,openSettings}:{openSettings():void|Promise<void>;model:WatchPanelModel;context:WatchPanelContext;i18n:I18n;reportError(error:unknown):void}){
 const root=useRef<HTMLDivElement>(null),t=i18n.t;
 useLayoutEffect(()=>{
  if(!context.returnFocusRef.current?.matches(':focus-visible'))return;
  const section=context.panel==='quality'?'quality':context.panel==='speed'?'speed':context.panel==='audio'?'audio':'subtitles';
  const scope=root.current?.querySelector(`[data-section="${section}"]`)??root.current;
  scope?.querySelector<HTMLButtonElement>('[aria-checked="true"], [role="radio"][tabindex="0"]')?.focus({preventScroll:true});
 },[context.panel,context.returnFocusRef]);
 useEffect(()=>()=>{const button=context.returnFocusRef.current;if(button?.isConnected)button.focus({preventScroll:true});},[context.returnFocusRef]);
 if(!model.isCurrent())return null;
 const content=<><div ref={root} className={styles.columns} onKeyDown={event=>{
  if(event.key==='Escape'){event.preventDefault();event.stopPropagation();context.close();}
  else if(event.defaultPrevented&&['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key))event.stopPropagation();
 }}>
  {model.subtitles.length>0&&<div data-section="subtitles"><TrackChoices title={t('player.opt.subtitles')} choices={model.subtitles} layout={context.layout} onSelect={model.selectSubtitle} reportError={reportError} t={t}/></div>}
  {(model.audio.length>0||model.quality.length>0)&&<div>
   {model.audio.length>0&&<div data-section="audio"><TrackChoices title={t('player.opt.audio')} choices={model.audio} layout={context.layout} onSelect={model.selectAudio} reportError={reportError} t={t}/></div>}
   {model.quality.length>0&&<div data-section="quality"><RadioColumn title={t('player.opt.quality')} rows={model.quality} select={model.selectQuality} reportError={reportError}/></div>}
  </div>}
  {model.speed.length>0&&<div data-section="speed" className={styles.column}>
   <h3>{t('player.opt.speed')}<span>{model.speed.find(row=>row.selected)?.label}</span></h3>
   <Segmented className={styles.speeds} aria-label={t('player.opt.speed')} value={model.speed.find(row=>row.selected)?.id??''}
    items={model.speed.map(row=>({value:row.id,label:row.label,disabled:!row.enabled}))}
    onValueChange={id=>{try{model.selectSpeed(id);}catch(error){reportError(error);}}}/>
  </div>}
 </div><div className={styles.note}><Icon name="i-info" size={20}/><span><b>{t('player.opt.temporaryChanges')}</b>{' '}{t('player.opt.defaultsUnchanged')}</span><a href="/settings" onClick={event=>{if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;event.preventDefault();try{context.close();void Promise.resolve(openSettings()).catch(reportError);}catch(error){reportError(error);}}}>{t('next.reader.openSettings')}<Icon name="i-right" size={16}/></a></div></>;
 return context.layout==='phone'?<BottomSheet open title={t('next.dock.options')} onOpenChange={open=>{if(!open)context.close();}} returnFocusRef={context.returnFocusRef}
  detents={[Math.max(1,(typeof window==='undefined'?context.phoneTop:window.visualViewport?.height??window.innerHeight)-context.phoneTop)]}>{content}</BottomSheet>
  :<section className={styles.options} role="dialog" aria-label={t('next.dock.options')}>{content}</section>;
}
