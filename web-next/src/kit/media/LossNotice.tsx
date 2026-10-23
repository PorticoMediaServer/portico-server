import {useEffect, useRef, useState} from 'react';
import {Icon} from '../Icon';
import type {MediaLayout} from './types';
import styles from './media.module.css';

export type PlaybackLossNotice = Readonly<{
  /** Stable playback-run identity, not generation: track/quality changes share this fence. */
  playbackId: string;
  cause: 'initial' | 'automatic' | 'user-choice';
  kind: 'picture' | 'sound';
  /** Canonical localized loss/replacement sentences, built from structured losses only. */
  loss: string; replacement: string;
}>;

/** Once per playback, six seconds, never focusable. A player choice cannot trigger a notice. */
export function LossNotice({notice, enabled, layout = 'desktop'}: {notice?: PlaybackLossNotice; enabled: boolean; layout?: MediaLayout}) {
  const shown = useRef(new Set<string>()), [visible, setVisible] = useState<PlaybackLossNotice | undefined>();
  useEffect(() => {
    if (!notice || !enabled || notice.cause === 'user-choice' || shown.current.has(notice.playbackId) || !notice.loss) return;
    shown.current.add(notice.playbackId); setVisible(notice);
  }, [notice, enabled]);
  useEffect(() => { if (!visible) return; const timer = setTimeout(() => setVisible(undefined), 6000); return () => clearTimeout(timer); }, [visible]);
  if (!enabled || !visible || notice?.playbackId !== visible.playbackId) return null;
  return <div key={visible.playbackId} className={styles.lossNotice} data-layout={layout} role="status" aria-live="polite" aria-atomic="true">
    <span className={styles.lossIcon}><Icon name={visible.kind === 'picture' ? 'i-image' : 'i-vol'} size={layout === 'tv' ? 28 : layout === 'phone' ? 16 : 20} /></span>
    <span><b>{visible.loss}</b>{visible.replacement && <> {visible.replacement}</>}</span><i className={styles.lossCountdown} />
  </div>;
}
