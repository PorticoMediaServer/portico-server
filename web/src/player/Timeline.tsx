import React, {useCallback, useRef, useState, useSyncExternalStore} from 'react';
import type {PlaybackService} from '@core/index.ts';
import {trickplayFrameAt, type TrickplaySet} from '@core/trickplay.ts';
import {formatClock} from '@core/presentation/index.ts';
import {cx, useArtworkUrl} from '../ui';
import s from './Player.module.css';

/**
 * Seek bar. Position comes from the playback service; a drag shows the
 * target and commits once on release. Buffered ranges are advisory.
 */
export type TimelineMark = {start: number; end?: number; title?: string};

export function Timeline({position, duration, buffered = 0, disabled, onSeek, live, onLive, programme, programmeLabel, behindLabel, trickplay, step = 10, chapters, segments}: {position: number; duration: number; buffered?: number; disabled?: boolean; onSeek: (seconds: number) => void; live?: boolean; onLive?: () => void; programme?: {start: number; end: number; title?: string}; /** MU4 FEAT-07: the programme's wall-clock times ("7:30 – 8:30 PM"). */ programmeLabel?: string; /** MU4 FEAT-07: how far behind live ("2:15 behind live"). */ behindLabel?: string; trickplay?: TrickplaySet | null; step?: number; /** FEAT-11: chapter starts, drawn as gaps in the rail. */ chapters?: readonly TimelineMark[]; /** FEAT-11: intro and credits, a subtle tint on the rail. */ segments?: readonly Required<Pick<TimelineMark, 'start' | 'end'>>[]}) {
  const [drag, setDrag] = useState<number | null>(null);
  const [hover, setHover] = useState<number | null>(null);
  const track = useRef<HTMLDivElement>(null);
  const max = duration > 0 ? duration : 1;
  const shown = drag ?? position;
  const pct = Math.min(100, Math.max(0, (shown / max) * 100));
  // The time under the pointer, and the thumb's time while dragging (FEAT-11).
  const target = drag ?? hover;
  const chapterAt = (seconds: number) => { let title: string | undefined; for (const c of chapters ?? []) { if (c.start <= seconds) title = c.title; else break; } return title; };
  const at = (clientX: number) => {
    const el = track.current!;
    const r = el.getBoundingClientRect();
    return Math.max(0, Math.min(max, ((clientX - r.left) / r.width) * max));
  };
  return (
    <div className={s.timeline}>
      <span aria-hidden>{programmeLabel ?? formatClock(shown)}</span>
      <div
        ref={track}
        className={cx(s.track, drag !== null && s.dragging)}
        onPointerMove={e => setHover(at(e.clientX))}
        onPointerLeave={() => setHover(null)}
        onPointerDown={e => {
          if (disabled) return;
          (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
          setDrag(at(e.clientX));
        }}
        onPointerUp={e => {
          if (drag === null) return;
          const target = at(e.clientX);
          setDrag(null);
          onSeek(target);
        }}
        onPointerCancel={() => setDrag(null)}
        onPointerMoveCapture={e => drag !== null && setDrag(at(e.clientX))}
      >
        <div className={s.rail}>
          {buffered > 0 ? <div className={s.buffered} style={{width: `${Math.min(100, (buffered / max) * 100)}%`}} /> : null}
          {segments?.map(m => <div key={m.start} className={s.segment} style={{left: `${(m.start / max) * 100}%`, width: `${((m.end - m.start) / max) * 100}%`}} />)}
          <div className={s.played} style={{width: `${pct}%`}} />
          {chapters?.map(c => c.start > 0 && c.start < max ? <i key={c.start} className={s.chapterTick} style={{left: `${(c.start / max) * 100}%`}} /> : null)}
        </div>
        <div className={s.knob} style={{left: `${pct}%`}} />
        {target !== null ? (
          trickplay
            ? <TrickplayPreview set={trickplay} seconds={target} left={(target / max) * 100} label={formatClock(target)} chapter={chapterAt(target)} />
            : <span className={s.hoverTime} style={{left: clampLeft((target / max) * 100, 40)}}>{chapterAt(target) ? <b>{chapterAt(target)}</b> : null}{formatClock(target)}</span>
        ) : null}
        <input
          className={s.range}
          type="range"
          min={0}
          max={max}
          step={0.5}
          value={shown}
          disabled={disabled}
          aria-label="Playback position"
          aria-valuetext={`${formatClock(shown)} of ${formatClock(duration)}`}
          onChange={e => onSeek(Number(e.target.value))}
          onKeyDown={e => {
            if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
              e.preventDefault();
              onSeek(Math.max(0, Math.min(max, position + (e.key === 'ArrowLeft' ? -step : step))));
            }
          }}
        />
      </div>
      {live ? (
        <>{behindLabel ? <span aria-hidden>{behindLabel}</span> : null}<button type="button" className={s.live} onClick={onLive} aria-label="Go to live"><i />LIVE</button></>
      ) : (
        <span aria-hidden>{duration > 0 ? `-${formatClock(Math.max(0, duration - shown))}` : formatClock(duration)}</span>
      )}
      {programme?.title ? <span className="visually-hidden">{programme.title}</span> : null}
    </div>
  );
}

/** One sprite-sheet frame at the hovered position. Tiles load through the
 * authorized artwork cache, so scrubbing back over a sheet costs nothing. */
function TrickplayPreview({set, seconds, left, label, chapter}: {set: TrickplaySet; seconds: number; left: number; label: string; chapter?: string}) {
  const frame = trickplayFrameAt(set, seconds);
  const {url} = useArtworkUrl(frame?.tileUrl);
  if (!frame) return <span className={s.hoverTime} style={{left: clampLeft(left, 40)}}>{chapter ? <b>{chapter}</b> : null}{label}</span>;
  const scale = 160 / frame.width;
  // FEAT-11: the time sits in the preview's bottom band (sprites carry no burned-in time), the
  // chapter above it, and the preview stays inside the bar at either end.
  return (
    <div className={s.previewWrap} aria-hidden style={{left: clampLeft(left, 80)}}>
      {chapter ? <span className={s.previewChapter}>{chapter}</span> : null}
      <div className={s.preview} style={{backgroundImage: url ? `url(${url})` : undefined, backgroundPosition: `-${frame.x * scale}px -${frame.y * scale}px`, backgroundSize: `${set.width * scale}px ${set.height * scale}px`}}>
        <span className={s.previewTime}>{label}</span>
      </div>
    </div>
  );
}

/** Keep a centred overlay of half-width `half` inside the bar. */
const clampLeft = (percent: number, half: number) => `clamp(${half}px, ${percent}%, calc(100% - ${half}px))`;

/** The seek bar, following the playback service itself so it moves smoothly without the player re-rendering around it. */
export function LiveTimeline({service, pending = true, ...rest}: Omit<React.ComponentProps<typeof Timeline>, 'position'> & {service: PlaybackService; /** Show where a seek is going while it lands. */ pending?: boolean}) {
  const read = useCallback(() => {
    const now = service.getSnapshot();
    return (pending ? now.pendingSeek?.positionSeconds : undefined) ?? now.positionSeconds;
  }, [service, pending]);
  const position = useSyncExternalStore(service.subscribe, read);
  return <Timeline position={position} {...rest} />;
}
