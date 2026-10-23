import {useRef, useState, type RefObject} from 'react';
import {Dialog, Popover} from 'radix-ui';
import {Icon} from '../Icon';
import {SignalLine} from './SignalLine';
import type {MediaLayout, SignalPathModel, SignalTranslate} from './types';
import styles from './media.module.css';

export type SignalPathProps = Readonly<{
  model: SignalPathModel; translate: SignalTranslate; layout?: MediaLayout;
  open: boolean; onOpenChange(open: boolean): void;
  anchorRef: RefObject<HTMLButtonElement | null>;
}>;

function SignalSteps({model, translate}: Pick<SignalPathProps, 'model' | 'translate'>) {
  return <ol className={styles.signalSteps}>{model.steps.map(step => <li key={step.id} data-changed={step.changed || undefined}>
    <span className={styles.stepLabel}>{'deviceName' in step.label ? step.label.deviceName : translate(step.label)}</span>
    <span className={styles.stepValue}>{translate(step.value)}{step.note && <small>{translate(step.note)}</small>}</span>
  </li>)}</ol>;
}

/** Facts and localized decisions come from the canonical Music formatter, never persisted intent. */
export function SignalPath({model, translate, layout = 'desktop', open, onOpenChange, anchorRef}: SignalPathProps) {
  const done = useRef<HTMLButtonElement>(null);
  const title = translate({key: 'player.signal.title'});
  const pill = <span className={styles.signalPill} data-healthy={model.pillHealthy || undefined}>{translate(model.pill)}</span>;
  const close = () => onOpenChange(false);
  const restore = (event: Event) => { event.preventDefault(); anchorRef.current?.focus({preventScroll: true}); };
  const content = <>
    {layout === 'tv' ? <><p className={styles.signalContext}>{translate({key: 'player.queueNowPlaying'})}</p><h2>{title}</h2>{pill}</> : <header className={styles.signalHeader}><b>{title}</b>{pill}{layout === 'desktop' && <button type="button" className={styles.close} onClick={close} aria-label={translate({key: 'next.states.close'})}><Icon name="i-x" size={18} /></button>}</header>}
    <SignalSteps model={model} translate={translate} />
    {layout === 'tv' && <footer className={styles.signalFooter}><button type="button" ref={done} onClick={close}>{translate({key: 'action.done'})}</button></footer>}
  </>;
  if (layout === 'desktop') return <Popover.Root open={open} onOpenChange={onOpenChange}>
    <Popover.Anchor virtualRef={{current: {getBoundingClientRect: () => anchorRef.current?.getBoundingClientRect() ?? new DOMRect()}}} />
    <Popover.Portal><Popover.Content className={styles.signalPopover} side="top" align="start" sideOffset={4} alignOffset={-12} collisionPadding={8} role="dialog" aria-label={title} onCloseAutoFocus={restore}>{content}</Popover.Content></Popover.Portal>
  </Popover.Root>;
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal>
    <Dialog.Overlay className={styles.signalScrim} />
    <Dialog.Content className={styles.signalDialog} data-layout={layout} aria-describedby={undefined} onCloseAutoFocus={restore} onOpenAutoFocus={event => { if (layout === 'tv') { event.preventDefault(); done.current?.focus(); } }}>
      <Dialog.Title className={styles.srOnly}>{title}</Dialog.Title>
      {layout === 'phone' && <button type="button" className={styles.grabber} onClick={close} aria-label={translate({key: 'next.states.close'})} onPointerDown={event => { event.currentTarget.dataset.startY = String(event.clientY); event.currentTarget.setPointerCapture(event.pointerId); }} onPointerUp={event => { if (event.clientY - Number(event.currentTarget.dataset.startY) > 40) close(); }}><i /></button>}
      {content}
    </Dialog.Content>
  </Dialog.Portal></Dialog.Root>;
}

/** Optional independent composition for an existing Now playing title column. */
export function MusicSignal({model, translate, layout = 'desktop'}: Pick<SignalPathProps, 'model' | 'translate' | 'layout'>) {
  const [open, setOpen] = useState(false), anchor = useRef<HTMLButtonElement>(null);
  return <><SignalLine model={model} translate={translate} layout={layout} ref={anchor} aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(true)} /><SignalPath model={model} translate={translate} layout={layout} open={open} onOpenChange={setOpen} anchorRef={anchor} /></>;
}
