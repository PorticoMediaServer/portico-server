import React, {useEffect, useRef, useState} from 'react';
import type {SegmentMarker, SegmentMarkerSet} from '@core/segment-markers.ts';
import {Button} from '../ui';
import s from './Player.module.css';

export type SkipMode = 'ask' | 'auto' | 'off';

const verbs: Record<SegmentMarker['kind'], string> = {intro: 'Skip intro', recap: 'Skip recap', credits: 'Skip credits', commercial: 'Skip ad', outro: 'Skip outro'};

/**
 * Skip prompts for detected segments. The server decides which markers exist
 * and which are safe to skip unattended; the viewer's preference decides
 * whether to ask, skip automatically, or stay quiet. A skip seeks to the end
 * of the segment and records the action against the session (spec §4.2).
 */
export function SkipPrompt({markers, position, sessionId, modes, onSeek, onSkipped}: {markers: SegmentMarkerSet | null; position: number; sessionId?: string; modes: Record<string, SkipMode>; onSeek: (seconds: number) => void; onSkipped: (markerId: string, mode: 'automatic' | 'manual', positionSeconds: number) => void}) {
  const [dismissed, setDismissed] = useState<Set<string>>(() => new Set());
  const skipped = useRef<Set<string>>(new Set());
  useEffect(() => {
    setDismissed(new Set());
    skipped.current = new Set();
  }, [sessionId, markers?.revision]);
  const active = markers?.markers.find(m => position >= m.startSeconds && position < m.endSeconds - 0.5 && !dismissed.has(m.id) && (modes[m.kind] ?? 'ask') !== 'off');
  const skip = (marker: SegmentMarker, mode: 'automatic' | 'manual') => {
    skipped.current.add(marker.id);
    setDismissed(prev => new Set(prev).add(marker.id));
    onSkipped(marker.id, mode, position);
    onSeek(marker.endSeconds);
  };
  useEffect(() => {
    if (!active) return;
    // Unattended skip only when the viewer asked for it and the server says the marker is safe.
    if ((modes[active.kind] ?? 'ask') === 'auto' && active.automaticSafe && !skipped.current.has(active.id)) skip(active, 'automatic');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active?.id]);
  if (!active) return null;
  return (
    <div className={s.skip}>
      <Button variant="glass" icon="forward" label={verbs[active.kind]} onClick={() => skip(active, 'manual')} />
      <Button variant="ghost" size="sm" label="Dismiss" onClick={() => setDismissed(prev => new Set(prev).add(active.id))} />
    </div>
  );
}
