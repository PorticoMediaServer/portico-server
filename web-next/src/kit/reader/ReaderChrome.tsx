import {useCallback, useEffect, useRef, useState, type ReactNode} from 'react';
import type {ReaderSurfaceInput} from '@portico/reader-core';
import {Icon, type IconName} from '../Icon';
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
}>;

export function ReaderChrome({title, subtitle, copy, chapter, pages, remainingInSection, percent, remainingInBook, progression, chapters = [], direction, tapTurnEnabled, pageTurnsEnabled,
  onBack, onTurn, onSeek, actions, children, panel, panelReflows = false, onPanelDismiss, showPageArrows, holdOpen = false, textSelectionActive = false, initiallyVisible = true, reducedMotion = false, fixedPage = false, fixedPageFooter, fixedPageStrip, searchNavigation, hint, subscribeSurfaceInput}: ReaderChromeProps) {
  const root = useRef<HTMLDivElement>(null), timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [visible, setVisible] = useState(initiallyVisible), [focusWithin, setFocusWithin] = useState(false);
  const held = holdOpen || textSelectionActive || Boolean(panel) || focusWithin;
  const cancel = useCallback(() => { if (timer.current !== undefined) clearTimeout(timer.current); timer.current = undefined; }, []);
  const show = useCallback(() => { cancel(); setVisible(true); }, [cancel]);
  const arm = useCallback(() => {
    cancel();
    if (!held) timer.current = setTimeout(() => { timer.current = undefined; setVisible(false); }, 2800);
  }, [cancel, held]);
  // Surface activity is deliberately not a React store update per mousemove.
  const wake = useCallback(() => { setVisible(true); arm(); }, [arm]);
  useEffect(() => { if (!root.current?.contains(document.activeElement)) root.current?.focus({preventScroll: true}); }, []);
  useEffect(() => {
    // Read frames: never hide while a panel/menu/selection/focused control is active.
    if (visible) arm(); else cancel();
    return cancel;
  }, [arm, cancel, visible]);
  const tap = useCallback((fraction: number) => {
    if (panel) { onPanelDismiss?.('outside'); return; }
    const turn = readTapTurn(fraction, direction);
    if (turn === 'chrome') { if (!held) { cancel(); setVisible(value => !value); } }
    else if (turn && tapTurnEnabled && pageTurnsEnabled && !textSelectionActive) onTurn(turn);
  }, [panel, onPanelDismiss, direction, held, cancel, onTurn, tapTurnEnabled, pageTurnsEnabled, textSelectionActive]);
  const key = useCallback((input: Extract<ReaderSurfaceInput, {kind: 'key'}>) => {
    if (input.editable || textSelectionActive || input.alt || input.ctrl || input.meta) return;
    if (panel && input.key === 'Escape') { input.preventDefault(); onPanelDismiss?.('escape'); return; }
    wake();
    const turn = readKeyTurn(input.key, direction, input.shift);
    if (turn && pageTurnsEnabled) { input.preventDefault(); onTurn(turn); }
  }, [panel, onPanelDismiss, direction, onTurn, pageTurnsEnabled, textSelectionActive, wake]);
  useEffect(() => subscribeSurfaceInput?.(input => {
    if (input.kind === 'tap') tap(input.fraction);
    else if (input.kind === 'activity') wake();
    else key(input);
  }), [subscribeSurfaceInput, tap, wake, key]);
  const shown = visible || held, progress = observedProgress(progression);
  const customFixedFooter = fixedPage && fixedPageFooter != null;
  return <div ref={root} tabIndex={-1} className={styles.reader} data-reader-root data-fixed={fixedPage || undefined} data-reduced-motion={reducedMotion || undefined}
    data-chrome={shown ? 'shown' : 'hidden'} data-panel={panelReflows || undefined} dir={direction === 'rtl' ? 'rtl' : 'ltr'}
    onPointerMove={event => { if (event.pointerType === 'mouse') wake(); }}
    onKeyDown={event => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || readerInteractiveTarget(event.target)) return;
      key({kind: 'key', key: event.key, shift: event.shiftKey, alt: event.altKey, ctrl: event.ctrlKey, meta: event.metaKey, editable: false, preventDefault: () => event.preventDefault()});
    }}>
    <div className={styles.surface} data-reader-surface onClick={event => {
      if (event.defaultPrevented || readerInteractiveTarget(event.target) || window.getSelection()?.toString()) return;
      // The fixed renderer owns delayed single/double taps and selection. Its
      // DOM click must not toggle chrome or turn a page a second time.
      if (event.target instanceof Element && event.target.closest('[data-reader-fixed]')) return;
      const bounds = event.currentTarget.getBoundingClientRect();
      if (bounds.width > 0) tap((event.clientX - bounds.left) / bounds.width);
    }}>{children}</div>
    {title && <div className={styles.runningTitle} aria-hidden={shown}>{title}</div>}
    {(chapter || remainingInSection) && <div className={styles.runningFooter} aria-hidden={shown}><span>{chapter}</span><span>{remainingInSection}</span></div>}
    <div className={styles.chrome} inert={!shown} aria-hidden={!shown}
      onFocusCapture={() => { setFocusWithin(true); show(); }} onBlurCapture={event => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setFocusWithin(false); }}>
      <header className={styles.top} dir="ltr">
        <button type="button" className={styles.back} aria-label={copy.back} onClick={onBack}><Icon name="i-left" size={24} /></button>
        <div className={styles.title}>{title && <b>{title}</b>}{subtitle && <small>{subtitle}</small>}</div>
        <div className={styles.tools}>{actions.map(action => <span key={action.id} className={styles.action}>{action.separatorBefore && <span className={styles.separator} aria-hidden="true" />}<button type="button" className={styles.tool} aria-label={action.label}
          aria-pressed={action.pressed} aria-expanded={action.expanded} disabled={action.disabled} data-open={action.expanded || undefined}
          onClick={event => action.onPress(event.currentTarget)}>{action.icon ? <Icon name={action.icon} size={22} /> : action.text}</button></span>)}</div>
      </header>
      {showPageArrows && <><button type="button" disabled={!pageTurnsEnabled} className={`${styles.turn} ${styles.left}`} aria-label={direction === 'rtl' ? copy.next : copy.previous} onClick={() => onTurn(direction === 'rtl' ? 'next' : 'previous')}><Icon name="i-left" size={26} /></button>
      <button type="button" disabled={!pageTurnsEnabled} className={`${styles.turn} ${styles.right}`} aria-label={direction === 'rtl' ? copy.previous : copy.next} onClick={() => onTurn(direction === 'rtl' ? 'previous' : 'next')}><Icon name="i-right" size={26} /></button></>}
      <footer className={`${styles.bottom}${customFixedFooter ? ` ${styles.fixedPageFooter}` : ''}`} dir="ltr">
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
      {fixedPageStrip && <div className={styles.fixedStrip}>{fixedPageStrip}</div>}
    </div>
    {searchNavigation && !panel && <div className={styles.searchNavigation}>{searchNavigation}</div>}
    {panel}
    {hint}
  </div>;
}
