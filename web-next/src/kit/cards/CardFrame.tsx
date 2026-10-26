import {useSelectionRangePreview} from './selection-range';
import {useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode, type CSSProperties, type MouseEvent, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import type {FactPart} from '@portico/client-core/kit-model';
import {Icon} from '../Icon';
import {SelectionCircle, SelectionHoldFeedback} from '../selection';
import {activateCard, boundedProgress} from './interaction';
import {useCardHold} from './useCardHold';
import type {CardFactPresentation, CardProps, CardTextRun} from './types';
import styles from './cards.module.css';

export const cx = (...names: (string | false | undefined)[]) => names.filter(Boolean).join(' ');

/** Presentation can decorate canonical text, never replace it or introduce arbitrary markup. */
function CardText({text, runs}: {text: string; runs?: readonly CardTextRun[]}) {
  if (!Array.isArray(runs) || !runs.every(run => run && typeof run.text === 'string' && (run.emphasis === undefined || run.emphasis === 'match' || run.emphasis === 'secondary')) || runs.map(run => run.text).join('') !== text) return text;
  return runs.map((run, index) => run.emphasis === 'match' ? <mark key={index} className={styles.textMatch}>{run.text}</mark> : run.emphasis === 'secondary' ? <b key={index} className={styles.textSecondary}>{run.text}</b> : run.text);
}

export function Facts({parts = [], className, presentation}: {parts?: readonly FactPart[]; className?: string; presentation?: CardFactPresentation}) {
  return parts.length ? <span className={cx(styles.facts, className)}>{parts.map(part => <span key={part.id} data-fact={part.id} data-role={part.role}><CardText text={part.text} runs={presentation?.[part.id]} /></span>)}</span> : null;
}

export function Progress({value, edge = false, className}: {value?: number; edge?: boolean; className?: string}) {
  const progress = boundedProgress(value);
  return progress === undefined ? null : <span className={cx(styles.progress, edge && styles.edge, className)} aria-hidden="true"><span style={{width: `${progress * 100}%`}} /></span>;
}

export function NotInterestedButton({onClick, className, disabled}: {onClick: () => void; className?: string; disabled?: boolean}) {
  const id = useId(), label = defaultI18n.t('next.cards.notInterested');
  return <span className={cx(styles.notInterestedWrap, className)}>
    <button type="button" disabled={disabled} className={styles.notInterested} aria-label={label} aria-describedby={id} onClick={onClick}><Icon name="i-not" size={18} /></button>
    <span className={styles.tooltip} id={id} role="tooltip">{label}</span>
  </span>;
}

type FrameProps = CardProps & Readonly<{children: ReactNode; kind: string; primaryClassName?: string; playIcon?: 'i-play' | 'i-book' | 'i-pause'; playIconSize?: number; artworkRef?: RefObject<HTMLSpanElement | null>; overlay?: ReactNode; footer?: ReactNode}>;

/** Primary navigation and auxiliary actions are sibling buttons, never nested controls. */
export function CardFrame({children, kind, primaryClassName, playIcon = 'i-play', playIconSize = 22, artworkRef, overlay, footer, ...props}: FrameProps) {
  const {model, onNavigate, onPlay, onMore, onNotInterested, selection, disabled, className, style, primaryRef} = props;
  const hasAuxiliary = !!(onPlay || onMore || onNotInterested || selection);
  const auxiliaryRef = useRef<HTMLSpanElement>(null);
  const shadowRef = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const art = artworkRef?.current, frame = auxiliaryRef.current;
    if (!hasAuxiliary || !art || !frame) return;
    const primary = art.parentElement!;
    const align = () => {
      // offsetTop includes the browser button's genuine content-centering inset.
      // Read layout geometry: hover transforms must not feed back into this frame.
      const bounds = getComputedStyle(art);
      Object.assign(frame.style, {left: `${art.offsetLeft}px`, top: `${art.offsetTop}px`, width: bounds.width, height: bounds.height});
    };
    align();
    const observer = new ResizeObserver(align);
    // Caption/font changes can move art within a fixed-height primary target.
    observer.observe(primary);
    for (const child of primary.children) observer.observe(child);
    return () => observer.disconnect();
  }, [artworkRef, hasAuxiliary]);
  // The fading shadow layer matches the artwork's own box, read from layout only.
  useLayoutEffect(() => {
    const art = artworkRef?.current, shadow = shadowRef.current;
    if (!art || !shadow) return;
    const primary = art.parentElement!;
    const align = () => {
      const bounds = getComputedStyle(art);
      Object.assign(shadow.style, {left: `${art.offsetLeft}px`, top: `${art.offsetTop}px`, width: bounds.width, height: bounds.height});
    };
    align();
    const observer = new ResizeObserver(align);
    observer.observe(primary);
    for (const child of primary.children) observer.observe(child);
    return () => observer.disconnect();
  }, [artworkRef]);
  const rangeInteraction = useSelectionRangePreview(selection, disabled);
  const selectionHintId = useId();
  const selectionHint = selection?.hint ?? (selection && !selection.active ? {text: defaultI18n.t('next.selection.select')} : undefined);
  const holdGesture = useCardHold({selection, disabled, row: model});
  const click = (event: MouseEvent<HTMLButtonElement>) => {
    if (holdGesture.consumeClick()) { event.preventDefault(); return; }
    activateCard(onNavigate, selection, {range: event.shiftKey, source: event.detail === 0 ? 'keyboard' : 'pointer'});
  };
  return <article className={cx(styles.card, styles[kind], styles[`context-${props.context ?? 'shelf'}`], props.density === 'grid' && styles['context-grid'], selection?.active && styles.selecting, selection?.checked && styles.selected, selection?.preview && styles.range, !!holdGesture.hold && styles.holding, props.next && styles.next, className)} style={style} data-card-kind={kind} data-disabled={disabled || undefined}
    {...rangeInteraction}>
    <button type="button" disabled={disabled} ref={primaryRef} className={cx(styles.primary, primaryClassName)} aria-label={model.title} aria-pressed={selection?.active ? selection.checked : undefined} onClick={click} onFocus={props.onFocus} onBlur={event => {holdGesture.cancel(); props.onBlur?.(event);}}
      onKeyDown={event => { if (event.key === 'Escape') { holdGesture.cancel(); if (selection?.active) { event.preventDefault(); event.stopPropagation(); selection.onExit(); } } }}
      {...holdGesture.handlers}>
      {artworkRef && <span ref={shadowRef} className={styles.artShadow} aria-hidden="true" />}
      {children}
    </button>
    {hasAuxiliary && <span className={cx(styles.auxiliary, !!artworkRef && styles.artAligned)}><span ref={auxiliaryRef} className={artworkRef ? styles.artAuxiliary : styles.cardAuxiliary}>
    {onPlay && !selection?.active && <button type="button" disabled={disabled} className={styles.play} aria-label={props.playLabel ?? defaultI18n.t(playIcon === 'i-book' ? 'next.cards.read' : playIcon === 'i-pause' ? 'next.cards.pause' : 'next.cards.play')} onClick={onPlay}><Icon name={playIcon} size={playIconSize} /></button>}
    {onMore && <button type="button" disabled={disabled} className={styles.more} aria-label={defaultI18n.t('next.cards.more')} aria-haspopup="menu" aria-expanded={props.moreExpanded} onClick={event => onMore(event.currentTarget)}><Icon name="i-more" size={18} /></button>}
    {onNotInterested && <NotInterestedButton disabled={disabled} onClick={onNotInterested} className={styles.notInterestedPosition} />}
    {selection && <button type="button" disabled={disabled} className={styles.selection} aria-label={defaultI18n.t('card.select', {title: model.title})} aria-describedby={selectionHint ? selectionHintId : undefined} aria-pressed={selection.checked} onClick={event => {event.stopPropagation(); selection.onToggle({range: event.shiftKey, source: event.detail === 0 ? 'keyboard' : 'pointer'});}}>{selection.control ?? <SelectionCircle interactive={false} selected={selection.checked} selecting={selection.active} accessibilityLabel={model.title} />}</button>}
    {selectionHint && <span className={styles.selectionHint} id={selectionHintId} role="tooltip">{'shortcut' in selectionHint && selectionHint.shortcut && <kbd>{selectionHint.shortcut}</kbd>}{selectionHint.text}</span>}
    </span></span>}
    {holdGesture.hold && <SelectionHoldFeedback active progress={holdGesture.hold.progress} className={styles.holdFeedback} style={{left: holdGesture.hold.x - 31, top: holdGesture.hold.y - 31}} />}
    {overlay}{footer}
  </article>;
}

export function CardCaption({model, titleLines = 1, extraCaption, textPresentation}: {model: CardProps['model']; titleLines?: 1 | 2; extraCaption?: CardProps['extraCaption']; textPresentation?: CardProps['textPresentation']}) {
  return <span className={styles.caption}><span className={cx(styles.title, titleLines === 2 && styles.twoLines)}><CardText text={model.title} runs={textPresentation?.title} /></span><Facts parts={model.caption} className={styles.subtitle} presentation={textPresentation?.caption} />{extraCaption?.map((parts, index) => <Facts key={index} parts={parts} className={parts.some(part => part.role === 'reason') ? styles.reason : styles.subtitle} presentation={textPresentation?.extraCaption?.[index]} />)}</span>;
}

export function CardBadges({watched, completionLabel, watchedLabel, isNew, progress}: {watched?: boolean; completionLabel?: string; /** The accessible name for a bare check; video keeps "Watched". */ watchedLabel?: string; isNew?: boolean; progress?: number}) {
  return <>{isNew && <span className={styles.newBadge}>{defaultI18n.t('next.cards.new')}</span>}{watched && <span className={cx(styles.watched, !!completionLabel && styles.finished)} aria-label={completionLabel ?? watchedLabel ?? defaultI18n.t('next.cards.watched')}><Icon name="i-check" size={16} />{completionLabel}</span>}<Progress value={progress} edge /></>;
}

export const tintStyle = (tint?: string): CSSProperties => tint ? {'--cards-tint': tint} as CSSProperties : {};
