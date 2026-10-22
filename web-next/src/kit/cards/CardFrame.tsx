import {useEffect, useId, useRef, type ReactNode, type CSSProperties, type MouseEvent} from 'react';
import {defaultI18n} from '@i18n';
import type {FactPart} from '@portico/client-core/kit-model';
import {Icon} from '../Icon';
import {activateCard, boundedProgress} from './interaction';
import type {CardProps} from './types';
import styles from './cards.module.css';

export const cx = (...names: (string | false | undefined)[]) => names.filter(Boolean).join(' ');

export function Facts({parts = [], className}: {parts?: readonly FactPart[]; className?: string}) {
  return parts.length ? <span className={cx(styles.facts, className)}>{parts.map(part => <span key={part.id} data-fact={part.id} data-role={part.role}>{part.text}</span>)}</span> : null;
}

export function Progress({value, edge = false, className}: {value?: number; edge?: boolean; className?: string}) {
  const progress = boundedProgress(value);
  return progress === undefined ? null : <span className={cx(styles.progress, edge && styles.edge, className)} aria-hidden="true"><span style={{width: `${progress * 100}%`}} /></span>;
}

export function NotInterestedButton({onClick, className}: {onClick: () => void; className?: string}) {
  const id = useId(), label = defaultI18n.t('next.cards.notInterested');
  return <span className={cx(styles.notInterestedWrap, className)}>
    <button type="button" className={styles.notInterested} aria-label={label} aria-describedby={id} onClick={onClick}><Icon name="i-not" size={18} /></button>
    <span className={styles.tooltip} id={id} role="tooltip">{label}</span>
  </span>;
}

type FrameProps = CardProps & Readonly<{children: ReactNode; kind: string; primaryClassName?: string; playIcon?: 'i-play' | 'i-book' | 'i-pause'; overlay?: ReactNode; footer?: ReactNode}>;

/** Primary navigation and auxiliary actions are sibling buttons, never nested controls. */
export function CardFrame({children, kind, primaryClassName, playIcon = 'i-play', overlay, footer, ...props}: FrameProps) {
  const {model, onNavigate, onPlay, onMore, onNotInterested, selection, className, style, primaryRef} = props;
  const pending = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const origin = useRef<{x: number; y: number} | undefined>(undefined);
  const held = useRef(false);
  const cancelHold = () => { if (pending.current !== undefined) clearTimeout(pending.current); pending.current = undefined; origin.current = undefined; };
  useEffect(() => () => { if (pending.current !== undefined) clearTimeout(pending.current); }, []);
  const click = (event: MouseEvent<HTMLButtonElement>) => {
    if (held.current) { held.current = false; event.preventDefault(); return; }
    activateCard(onNavigate, selection, {range: event.shiftKey, source: event.detail === 0 ? 'keyboard' : 'pointer'});
  };
  return <article className={cx(styles.card, styles[kind], styles[`context-${props.context ?? 'shelf'}`], props.density === 'grid' && styles['context-grid'], selection?.active && styles.selecting, selection?.checked && styles.selected, props.next && styles.next, className)} style={style} data-card-kind={kind}>
    <button type="button" ref={primaryRef} className={cx(styles.primary, primaryClassName)} aria-label={model.title} aria-pressed={selection?.active ? selection.checked : undefined} onClick={click} onFocus={props.onFocus} onBlur={props.onBlur}
      onKeyDown={event => { if (event.key === 'Escape' && selection?.active) { event.preventDefault(); event.stopPropagation(); selection.onExit(); } }}
      onPointerDown={event => {
        if (!event.isPrimary) return;
        cancelHold();
        held.current = false;
        if (event.pointerType === 'mouse' || !selection) return;
        origin.current = {x: event.clientX, y: event.clientY};
        pending.current = setTimeout(() => { held.current = true; pending.current = undefined; selection.onToggle({range: false, source: 'long-press'}); }, 500);
      }}
      onPointerMove={event => { const start = origin.current; if (start && Math.hypot(event.clientX - start.x, event.clientY - start.y) > 10) cancelHold(); }}
      onPointerUp={cancelHold} onPointerCancel={cancelHold} onPointerLeave={cancelHold}
      onContextMenu={event => { if (selection && held.current) event.preventDefault(); }}>
      {children}
    </button>
    {onPlay && !selection?.active && <button type="button" className={styles.play} aria-label={props.playLabel ?? defaultI18n.t(playIcon === 'i-book' ? 'next.cards.read' : playIcon === 'i-pause' ? 'next.cards.pause' : 'next.cards.play')} onClick={onPlay}><Icon name={playIcon} size={22} /></button>}
    {onMore && <button type="button" className={styles.more} aria-label={defaultI18n.t('next.cards.more')} aria-haspopup="menu" aria-expanded={props.moreExpanded} onClick={event => onMore(event.currentTarget)}><Icon name="i-more" size={18} /></button>}
    {onNotInterested && <NotInterestedButton onClick={onNotInterested} className={styles.notInterestedPosition} />}
    {selection && <span className={styles.selection}>{selection.control}</span>}
    {overlay}{footer}
  </article>;
}

export function CardCaption({model, titleLines = 1}: {model: CardProps['model']; titleLines?: 1 | 2}) {
  return <span className={styles.caption}><span className={cx(styles.title, titleLines === 2 && styles.twoLines)}>{model.title}</span><Facts parts={model.caption} className={styles.subtitle} /></span>;
}

export function CardBadges({watched, completionLabel, isNew, progress}: {watched?: boolean; completionLabel?: string; isNew?: boolean; progress?: number}) {
  return <>{isNew && <span className={styles.newBadge}>{defaultI18n.t('next.cards.new')}</span>}{watched && <span className={cx(styles.watched, !!completionLabel && styles.finished)} aria-label={completionLabel ?? defaultI18n.t('next.cards.watched')}><Icon name="i-check" size={16} />{completionLabel}</span>}<Progress value={progress} edge /></>;
}

export const tintStyle = (tint?: string): CSSProperties => tint ? {'--cards-tint': tint} as CSSProperties : {};
