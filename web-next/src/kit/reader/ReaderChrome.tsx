import {useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject} from 'react';
import {easings, motion, reducedMotion as reducedMotionPolicy} from '@design';
import type {ReaderSurfaceInput} from '@portico/reader-core';
import {Icon, type IconName} from '../Icon';
import {useReducedMotion} from '../overlays/shared';
import {observedProgress, readerInteractiveTarget, readKeyTurn, readTapTurn, type ReadDirection, type ReadTurn} from './interaction';
import styles from './reader.module.css';

/** All copy is supplied by the localized, frame-backed assembly. No engine copy leaks. */
export type ReaderChromeCopy = Readonly<{
  back: string; previous: string; next: string; progress: string;
}>;
export type ReaderChromeAction = Readonly<{
  id: string; label: string; icon?: IconName; text?: ReactNode;
  pressed?: boolean; expanded?: boolean; disabled?: boolean; separatorBefore?: boolean;
  onPress: (anchor: HTMLButtonElement) => void;
}>;
export type ReaderChapterMark = Readonly<{id: string; start: number; end?: number; current?: boolean}>;
/** Original publication observations, not pointer/zoom/focus activity or a timer. */
export type ReaderReadActivity = Readonly<{subscribe: (listener: () => void) => (() => void); scroll: () => void}>;
export type ReaderChromeProps = Readonly<{
  title?: string; subtitle?: string; copy: ReaderChromeCopy;
  /** These strings and fractions come from actual renderer observations. Missing stays absent. */
  chapter?: string; pages?: string; remainingInSection?: string; percent?: string; remainingInBook?: string;
  progression?: number; chapters?: readonly ReaderChapterMark[]; direction: ReadDirection;
  /** From the live Reading facade and renderer fit/zoom observation, not a second default map. */
  tapTurnEnabled: boolean; pageTurnsEnabled: boolean;
  onBack: () => void; onTurn: (turn: ReadTurn) => void; onSeek?: (progression: number) => void;
  actions: readonly ReaderChromeAction[]; children: ReactNode; panel?: ReactNode;
  panelReflows?: boolean; showPageArrows: boolean;
  /** Real retained search results supplied by the reader assembly. */
  searchNavigation?: ReactNode;
  /** Optional localized assembly content for fixed pages; prose always keeps the generic footer. */
  fixedPageFooter?: ReactNode;
  /** Optional drawn strip (page thumbnails) that sits above the footer on fixed pages. */
  fixedPageStrip?: ReactNode;
  /** Optional one-time coach mark (phone-read 14) drawn above the page and chrome. */
  hint?: ReactNode;
  /** The same page tap closes a docked panel without also navigating the book. */
  onPanelDismiss?: (reason: 'outside' | 'escape') => void;
  /** A panel/menu or a live text selection pins chrome until it closes. */
  holdOpen?: boolean; textSelectionActive?: boolean; initiallyVisible?: boolean; reducedMotion?: boolean; fixedPage?: boolean;
  /** Reader engine owns taps/selection in its iframe; forwarded fractions use the same handler. */
  subscribeSurfaceInput?: (listener: (input: ReaderSurfaceInput) => void) => (() => void);
  readActivity?: ReaderReadActivity;
}>;

/** Exit freezes the painted pose, even when entry is interrupted. Only a fully
 * invisible bar starts a fresh 8px entry; reversals use its actual painted pose. */
function useReaderChromeMotion(root: RefObject<HTMLDivElement | null>, shown: boolean, reduce: boolean) {
  const systemOrAppReduced = useReducedMotion(root);
  const layers = useRef(new Map<HTMLElement, {shown: boolean; animation?: Animation}>());
  useLayoutEffect(() => {
    const reduced = reduce || systemOrAppReduced || window.matchMedia('(prefers-reduced-motion: reduce)').matches
      || !!root.current?.closest('[data-reduce-motion="true"]');
    const elements = new Set(root.current?.querySelectorAll<HTMLElement>('[data-reader-chrome-layer]') ?? []);
    for (const [element, owner] of layers.current) if (!elements.has(element)) {owner.animation?.cancel(); layers.current.delete(element);}
    for (const element of elements) {
      const previous = layers.current.get(element), painted = getComputedStyle(element);
      const offset = element.dataset.readerChromeLayer === 'top' ? -8 : element.dataset.readerChromeLayer === 'turn' ? 0 : 8;
      const freshEntry = shown && (!previous || !previous.shown && Number(painted.opacity) === 0);
      const from = {opacity: !previous && shown ? 0 : Number(painted.opacity),
        transform: reduced ? 'none' : freshEntry ? `translateY(${offset}px)` : painted.transform};
      previous?.animation?.cancel();
      const target = {opacity: shown ? 1 : 0, transform: reduced || shown ? 'none' : from.transform};
      const owner = {shown, animation: element.animate([from, target], {
        duration: reduced ? reducedMotionPolicy.durationMs : shown ? motion.standard : motion.quick,
        easing: reduced ? reducedMotionPolicy.easing : shown ? easings.out : easings.in, fill: 'both',
      })};
      layers.current.set(element, owner);
      owner.animation.onfinish = () => {
        if (layers.current.get(element) !== owner) return;
        element.style.opacity = String(target.opacity); element.style.transform = target.transform;
        owner.animation.cancel();
      };
    }
  }, [root, shown, reduce, systemOrAppReduced]);
  useLayoutEffect(() => () => {
    for (const {animation} of layers.current.values()) animation?.cancel();
    layers.current.clear();
  }, []);
}

export function ReaderChrome({title, subtitle, copy, chapter, pages, remainingInSection, percent, remainingInBook, progression, chapters = [], direction, tapTurnEnabled, pageTurnsEnabled,
  onBack, onTurn, onSeek, actions, children, panel, panelReflows = false, onPanelDismiss, showPageArrows, holdOpen = false, textSelectionActive = false, initiallyVisible = true, reducedMotion = false, fixedPage = false, fixedPageFooter, fixedPageStrip, searchNavigation, hint, subscribeSurfaceInput, readActivity}: ReaderChromeProps) {
  const root = useRef<HTMLDivElement>(null), chrome = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(initiallyVisible), [focusWithin, setFocusWithin] = useState(false);
  const held = holdOpen || textSelectionActive || Boolean(panel) || focusWithin;
  const show = useCallback(() => setVisible(true), []);
  const readOn = useCallback(() => {
    if (!held && !chrome.current?.contains(document.activeElement) && !window.getSelection()?.toString()) setVisible(false);
  }, [held]);
  useEffect(() => { if (!root.current?.contains(document.activeElement)) root.current?.focus({preventScroll: true}); }, []);
  useLayoutEffect(() => readActivity?.subscribe(readOn), [readActivity, readOn]);
  const tap = useCallback((fraction: number) => {
    if (panel) { onPanelDismiss?.('outside'); return; }
    const turn = readTapTurn(fraction, direction);
    if (turn === 'chrome') { if (!held) setVisible(value => !value); }
    else if (turn && tapTurnEnabled && pageTurnsEnabled && !textSelectionActive) onTurn(turn);
  }, [panel, onPanelDismiss, direction, held, onTurn, tapTurnEnabled, pageTurnsEnabled, textSelectionActive]);
  const key = useCallback((input: Extract<ReaderSurfaceInput, {kind: 'key'}>) => {
    if (input.editable || textSelectionActive || input.alt || input.ctrl || input.meta) return;
    if (panel && input.key === 'Escape') { input.preventDefault(); onPanelDismiss?.('escape'); return; }
    const turn = readKeyTurn(input.key, direction, input.shift);
    if (turn && pageTurnsEnabled) { input.preventDefault(); onTurn(turn); }
  }, [panel, onPanelDismiss, direction, onTurn, pageTurnsEnabled, textSelectionActive]);
  useEffect(() => subscribeSurfaceInput?.(input => {
    if (input.kind === 'tap') tap(input.fraction);
    else if (input.kind === 'swipe') {
      if (panel) {onPanelDismiss?.('outside'); return;}
      const turn = readTapTurn(input.direction === 'left' ? 1 : 0, direction);
      if ((turn === 'next' || turn === 'previous') && pageTurnsEnabled && !textSelectionActive) onTurn(turn);
    }
    else if (input.kind === 'key') key(input);
  }), [subscribeSurfaceInput, tap, key, panel, onPanelDismiss, direction, pageTurnsEnabled, textSelectionActive, onTurn]);
  const shown = visible || held, progress = observedProgress(progression);
  useReaderChromeMotion(root, shown, reducedMotion);
  const customFixedFooter = fixedPage && fixedPageFooter != null;
  return <div ref={root} tabIndex={-1} className={styles.reader} data-reader-root data-fixed={fixedPage || undefined} data-reduced-motion={reducedMotion || undefined}
    data-chrome={shown ? 'shown' : 'hidden'} data-panel={panelReflows || undefined} dir={direction === 'rtl' ? 'rtl' : 'ltr'}
    onKeyDown={event => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || readerInteractiveTarget(event.target)) return;
      key({kind: 'key', key: event.key, shift: event.shiftKey, alt: event.altKey, ctrl: event.ctrlKey, meta: event.metaKey, editable: false, preventDefault: () => event.preventDefault()});
    }}>
    <div className={styles.surface} data-reader-surface onScrollCapture={event => {
      const target = event.target;
      // Foliate forwards its real scroll from the closed shadow root; fixed
      // pages scroll their own host/panes. Panel and control scrolls stay out.
      if (target instanceof Element && !readerInteractiveTarget(target) && (target.matches('foliate-paginator') || target.closest('[data-reader-fixed]'))) readActivity?.scroll();
    }} onClick={event => {
      if (event.defaultPrevented || readerInteractiveTarget(event.target) || window.getSelection()?.toString()) return;
      // The fixed renderer owns delayed single/double taps and selection. Its
      // DOM click must not toggle chrome or turn a page a second time.
      if (event.target instanceof Element && event.target.closest('[data-reader-fixed]')) return;
      const bounds = event.currentTarget.getBoundingClientRect();
      if (bounds.width > 0) tap((event.clientX - bounds.left) / bounds.width);
    }}>{children}</div>
    {title && <div className={styles.runningTitle} aria-hidden={shown}>{title}</div>}
    {(chapter || remainingInSection) && <div className={styles.runningFooter} aria-hidden={shown}><span>{chapter}</span><span>{remainingInSection}</span></div>}
    <div ref={chrome} className={styles.chrome} inert={!shown} aria-hidden={!shown}
      onFocusCapture={() => { setFocusWithin(true); show(); }} onBlurCapture={event => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setFocusWithin(false); }}>
      <header className={styles.top} dir="ltr" data-reader-chrome-layer="top">
        <button type="button" className={styles.back} aria-label={copy.back} onClick={onBack}><Icon name="i-left" size={24} /></button>
        <div className={styles.title}>{title && <b>{title}</b>}{subtitle && <small>{subtitle}</small>}</div>
        <div className={styles.tools}>{actions.map(action => <span key={action.id} className={styles.action}>{action.separatorBefore && <span className={styles.separator} aria-hidden="true" />}<button type="button" className={styles.tool} aria-label={action.label}
          aria-pressed={action.pressed} aria-expanded={action.expanded} disabled={action.disabled} data-open={action.expanded || undefined}
          onClick={event => action.onPress(event.currentTarget)}>{action.icon ? <Icon name={action.icon} size={22} /> : action.text}</button></span>)}</div>
      </header>
      {showPageArrows && <><button type="button" disabled={!pageTurnsEnabled} className={`${styles.turn} ${styles.left}`} data-reader-chrome-layer="turn" aria-label={direction === 'rtl' ? copy.next : copy.previous} onClick={() => onTurn(direction === 'rtl' ? 'next' : 'previous')}><Icon name="i-left" size={26} /></button>
      <button type="button" disabled={!pageTurnsEnabled} className={`${styles.turn} ${styles.right}`} data-reader-chrome-layer="turn" aria-label={direction === 'rtl' ? copy.previous : copy.next} onClick={() => onTurn(direction === 'rtl' ? 'previous' : 'next')}><Icon name="i-right" size={26} /></button></>}
      <footer className={`${styles.bottom}${customFixedFooter ? ` ${styles.fixedPageFooter}` : ''}`} dir="ltr" data-reader-chrome-layer="bottom">
        {customFixedFooter ? fixedPageFooter : <>
        <div className={styles.position}>{chapter && <b>{chapter}</b>}{pages && <small>{pages}</small>}</div>
        <div className={styles.progressGroup}>
          {progress !== undefined && <div className={styles.progress} dir={direction === 'rtl' ? 'rtl' : 'ltr'}>
            <span className={styles.track}><i className={styles.fill} style={{width: `${progress * 100}%`}} />
              {chapters.map(mark => { const start = observedProgress(mark.start), end = observedProgress(mark.end); return start === undefined ? null : <span key={mark.id}>
                <i className={styles.chapterMark} style={{insetInlineStart: `${start * 100}%`}} />
                {mark.current && end !== undefined && end > start && <i className={styles.currentChapter} style={{insetInlineStart: `${start * 100}%`, width: `${(end - start) * 100}%`}} />}
              </span>; })}<i className={styles.knob} style={{insetInlineStart: `${progress * 100}%`}} /></span>
            {onSeek && <input type="range" aria-label={copy.progress} aria-valuetext={pages ?? percent} min={0} max={1} step={0.0001} value={progress}
              onChange={event => { const value = observedProgress(event.currentTarget.valueAsNumber); if (value !== undefined) onSeek(value); }} />}
          </div>}
        </div>
        <div className={styles.remaining}>{remainingInSection && <b>{remainingInSection}</b>}{(percent || remainingInBook) && <small>{percent && <span>{percent}</span>}{remainingInBook && <span>{remainingInBook}</span>}</small>}</div>
        </>}
      </footer>
      {fixedPageStrip && <div className={styles.fixedStrip} data-reader-chrome-layer="strip">{fixedPageStrip}</div>}
    </div>
    {searchNavigation && !panel && <div className={styles.searchNavigation}>{searchNavigation}</div>}
    {panel}
    {hint}
  </div>;
}
