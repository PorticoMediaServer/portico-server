import {useEffect, useRef, useState} from 'react';
import {Icon} from '../Icon';
import {listeningNoticeKey} from '@core/audio-effects.ts';
import type {ListeningNoticeCode} from '../../platform/playback/listening-effects';
import type {MediaLayout} from './types';
import styles from './media.module.css';

export type ListeningNoticeProps = Readonly<{
  notice?: Readonly<{code: ListeningNoticeCode; sequence: number}>;
  /** Playback commands/error clear the canonical notice; this also fences late UI timers. */
  paused: boolean; stopped: boolean; errorShown: boolean; expanded: boolean;
  layout?: MediaLayout;
  translate(key: 'web.player.pressPlay' | 'listening.paused'): string;
}>;

/** The only two actionable quiet notices. Transient engine diagnostics have no input here. */
export function ListeningNotice({notice, paused, stopped, errorShown, expanded, layout = 'desktop', translate}: ListeningNoticeProps) {
  const [expired, setExpired] = useState<number | undefined>(), collapsedAt = useRef<{sequence: number; at: number} | undefined>(undefined);
  const sequence = notice?.sequence;
  useEffect(() => {
    if (sequence === undefined || expanded) { collapsedAt.current = undefined; setExpired(undefined); return; }
    if (collapsedAt.current?.sequence !== sequence) collapsedAt.current = {sequence, at: Date.now()};
    const timer = setTimeout(() => setExpired(sequence), Math.max(0, 8000 - (Date.now() - collapsedAt.current.at)));
    return () => clearTimeout(timer);
  }, [sequence, expanded]);
  if (!notice || !paused || stopped || errorShown || !expanded && expired === notice.sequence) return null;
  return <div key={notice.sequence} className={styles.listeningNotice} data-layout={layout} data-expanded={expanded} role="status" aria-live="polite" aria-atomic="true">
    <Icon name="i-pause" size={layout === 'tv' ? 28 : layout === 'phone' && expanded ? 14 : 18} />
    {translate(listeningNoticeKey(notice.code))}
  </div>;
}
