import {useCallback, useEffect, useRef, useState, type ReactNode} from 'react';
import {Icon, type IconName} from '../../kit/Icon';
import {observedProgress, readerInteractiveTarget, readKeyTurn, readTapTurn, type ReadDirection, type ReadTurn} from './interaction';
import styles from './reader.module.css';

/** All copy is supplied by the localized, frame-backed assembly. No engine copy leaks. */
export type ReaderChromeCopy = Readonly<{
  back: string; previous: string; next: string; progress: string;
}>;
export type ReaderChromeAction = Readonly<{
  id: string; label: string; icon?: IconName; text?: ReactNode;
  pressed?: boolean; expanded?: boolean; disabled?: boolean;
  onPress: (anchor: HTMLButtonElement) => void;
}>;
export type ReaderChapterMark = Readonly<{id: string; start: number; end?: number; current?: boolean}>;
export type ReaderChromeProps = Readonly<{
  title?: string; subtitle?: string; copy: ReaderChromeCopy;
  /** These strings and fractions come from actual renderer observations. Missing stays absent. */
  chapter?: string; pages?: string; remainingInSection?: string; percent?: string; remainingInBook?: string;
  progression?: number; chapters?: readonly ReaderChapterMark[]; direction: ReadDirection;
  onBack: () => void; onTurn: (turn: ReadTurn) => void; onSeek?: (progression: number) => void;
  actions: readonly ReaderChromeAction[]; children: ReactNode; panel?: ReactNode;
  /** A panel/menu or a live text selection pins chrome until it closes. */
  holdOpen?: boolean; initiallyVisible?: boolean; reducedMotion?: boolean; fixedPage?: boolean;
  /** Reader engine owns taps/selection in its iframe; forwarded fractions use the same handler. */
  onSurfaceTap?: (handler: (fraction: number) => void) => (() => void);
}>;

export function ReaderChrome({title, subtitle, copy, chapter, pages, remainingInSection, percent, remainingInBook, progression, chapters = [], direction,
  onBack, onTurn, onSeek, actions, children, panel, holdOpen = false, initiallyVisible = true, reducedMotion = false, fixedPage = false, onSurfaceTap}: ReaderChromeProps) {
  const root = useRef<HTMLDivElement>(null), timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [visible, setVisible] = useState(initiallyVisible), [focusWithin, setFocusWithin] = useState(false);
  const held = holdOpen || panel !== undefined && panel !== null || focusWithin;
  const cancel = useCallback(() => { if (timer.current !== undefined) clearTimeout(timer.current); timer.current = undefined; }, []);
  const show = useCallback(() => { cancel(); setVisible(true); }, [cancel]);
  const [activity, setActivity] = useState(0);
  const wake = useCallback(() => { show(); setActivity(value => value + 1); }, [show]);
  useEffect(() => {
    cancel();
    // Read frames: never hide while a panel/menu/selection/focused control is active.
    if (visible && !held) timer.current = setTimeout(() => { timer.current = undefined; setVisible(false); }, 2800);
    return cancel;
  }, [cancel, held, visible, activity]);
  const tap = useCallback((fraction: number) => {
    const turn = readTapTurn(fraction, direction);
    if (turn === 'chrome') { if (!held) { cancel(); setVisible(value => !value); } }
    else if (turn) onTurn(turn);
  }, [direction, held, cancel, onTurn]);
  useEffect(() => onSurfaceTap?.(tap), [onSurfaceTap, tap]);
  const shown = visible || held, progress = observedProgress(progression);
  return <div ref={root} className={styles.reader} data-reader-root data-fixed={fixedPage || undefined} data-reduced-motion={reducedMotion || undefined}
    data-chrome={shown ? 'shown' : 'hidden'} data-panel={panel !== undefined && panel !== null || undefined} dir={direction === 'rtl' ? 'rtl' : 'ltr'}
    onPointerMove={event => { if (event.pointerType === 'mouse') wake(); }}
    onKeyDown={event => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || readerInteractiveTarget(event.target)) return;
      wake();
      const turn = readKeyTurn(event.key, direction, event.shiftKey);
      if (turn) { event.preventDefault(); onTurn(turn); }
    }}>
    <div className={styles.surface} onClick={event => {
      if (event.defaultPrevented || readerInteractiveTarget(event.target) || window.getSelection()?.toString()) return;
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
        <div className={styles.tools}>{actions.map(action => <button key={action.id} type="button" className={styles.tool} aria-label={action.label}
          aria-pressed={action.pressed} aria-expanded={action.expanded} disabled={action.disabled} data-open={action.expanded || undefined}
          onClick={event => action.onPress(event.currentTarget)}>{action.icon ? <Icon name={action.icon} size={22} /> : action.text}</button>)}</div>
      </header>
      <button type="button" className={`${styles.turn} ${styles.left}`} aria-label={direction === 'rtl' ? copy.next : copy.previous} onClick={() => onTurn(direction === 'rtl' ? 'next' : 'previous')}><Icon name="i-left" size={26} /></button>
      <button type="button" className={`${styles.turn} ${styles.right}`} aria-label={direction === 'rtl' ? copy.previous : copy.next} onClick={() => onTurn(direction === 'rtl' ? 'previous' : 'next')}><Icon name="i-right" size={26} /></button>
      <footer className={styles.bottom} dir="ltr">
        <div className={styles.position}>{chapter && <b>{chapter}</b>}{pages && <small>{pages}</small>}</div>
        <div className={styles.progressGroup}>{remainingInSection && <span className={styles.sectionRemaining}>{remainingInSection}</span>}
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
        <div className={styles.remaining}>{percent && <b>{percent}</b>}{remainingInBook && <small>{remainingInBook}</small>}</div>
      </footer>
    </div>
    {panel}
  </div>;
}
