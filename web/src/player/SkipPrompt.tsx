import React, {useEffect, useRef, useState} from 'react';
import type {SegmentMarker, SegmentMarkerSet} from '@core/segment-markers.ts';
import {activeSegment, skipsAutomatically, skipTarget, UNDO_SECONDS, type SkipMode} from '@core/segment-skip.ts';
import {Button} from '../ui';
import s from './Player.module.css';
import {useI18n} from '../app/i18n';

export type {SkipMode};

type Undo = Readonly<{marker: SegmentMarker; from: number}>;

/**
 * Skip prompts for detected segments. The server decides which markers exist
 * and which are safe to skip unattended; the viewer's preference decides
 * whether to ask, skip automatically, or stay quiet (the shared rules in
 * segment-skip.ts). An intro skip lands a moment before the intro ends;
 * credits land on the scene after them. An unattended skip leaves a short
 * "Skipped intro · Undo" note, and undoing it goes back to where the skip
 * happened without skipping again. Every skip is recorded against the
 * session (spec §4.2).
 */
export function SkipPrompt({markers, position, duration, sessionId, modes, onSeek, onSkipped}: {markers: SegmentMarkerSet | null; position: number; duration?: number; sessionId?: string; modes: Record<string, SkipMode>; onSeek: (seconds: number) => void; onSkipped: (markerId: string, mode: 'automatic' | 'manual', positionSeconds: number) => void}) {
  const {t} = useI18n();
  const [dismissed, setDismissed] = useState<Set<string>>(() => new Set());
  const [undo, setUndo] = useState<Undo | null>(null);
  const skipped = useRef<Set<string>>(new Set());
  useEffect(() => {
    setDismissed(new Set());
    setUndo(null);
    skipped.current = new Set();
  }, [sessionId, markers?.revision]);
  useEffect(() => {
    if (!undo) return;
    const id = setTimeout(() => setUndo(null), UNDO_SECONDS * 1000);
    return () => clearTimeout(id);
  }, [undo]);
  const active = activeSegment(markers?.markers, position, modes, dismissed);
  const skip = (marker: SegmentMarker, mode: 'automatic' | 'manual') => {
    skipped.current.add(marker.id);
    setDismissed(prev => new Set(prev).add(marker.id));
    onSkipped(marker.id, mode, position);
    if (mode === 'automatic') setUndo({marker, from: position});
    onSeek(skipTarget(marker, duration));
  };
  useEffect(() => {
    if (active && skipsAutomatically(active, modes) && !skipped.current.has(active.id)) skip(active, 'automatic');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active?.id]);
  if (undo) {
    return (
      <div className={s.skip} role="status">
        <span className={s.skipNote}>{t('player.skipped', {kind: undo.marker.kind})}</span>
        <Button variant="glass" icon="back" label={t('action.undo')} onClick={() => { const from = undo.from; setUndo(null); onSeek(from); }} />
      </div>
    );
  }
  if (!active) return null;
  return (
    <div className={s.skip}>
      <Button variant="glass" icon="forward" label={t('player.skip', {kind: active.kind})} onClick={() => skip(active, 'manual')} />
      <Button variant="ghost" size="sm" label={t('action.dismiss')} onClick={() => setDismissed(prev => new Set(prev).add(active.id))} />
    </div>
  );
}
