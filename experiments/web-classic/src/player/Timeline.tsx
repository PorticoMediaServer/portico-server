import {useRef, useState} from 'react';
import type {Chapter, Marker} from '@/data/types';
import {formatTime} from '@/app/format';
import {cx} from '@/ui/cx';
import s from './Player.module.css';

/**
 * Seek bar with chapter ticks, intro and credits segments and a preview
 * frame on hover (the server's scrubbing pictures). Keyboard: arrows step
 * 10 s, Page Up/Down a minute, Home and End.
 */
export function Timeline({position, duration, onSeek, chapters = [], markers = [], thumb, label = 'Playback position', buffered = 0}: {position: number; duration: number; onSeek: (t: number) => void; chapters?: Chapter[]; markers?: Marker[]; thumb?: (t: number) => string | undefined; label?: string; buffered?: number}) {
  const ref = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<{x: number; t: number}>();
  const [drag, setDrag] = useState<number>();
  const at = (clientX: number) => {
    const r = ref.current!.getBoundingClientRect();
    const f = Math.max(0, Math.min(1, (clientX - r.left) / r.width));
    return {x: f * r.width, t: f * duration};
  };
  const shown = drag ?? position;
  const pct = duration ? (shown / duration) * 100 : 0;
  const chapterAt = (t: number) => [...chapters].reverse().find(c => c.start <= t);
  return (
    <div
      ref={ref}
      className={cx(s.timeline, drag !== undefined && s.dragging)}
      role="slider"
      tabIndex={0}
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={Math.round(duration)}
      aria-valuenow={Math.round(shown)}
      aria-valuetext={`${formatTime(shown)} of ${formatTime(duration)}`}
      onPointerMove={e => { const h = at(e.clientX); setHover(h); if (drag !== undefined) setDrag(h.t); }}
      onPointerLeave={() => { if (drag === undefined) setHover(undefined); }}
      onPointerDown={e => { (e.target as HTMLElement).setPointerCapture?.(e.pointerId); setDrag(at(e.clientX).t); }}
      onPointerUp={e => { if (drag !== undefined) { onSeek(at(e.clientX).t); setDrag(undefined); } }}
      onKeyDown={e => {
        const step = e.key === 'ArrowLeft' ? -10 : e.key === 'ArrowRight' ? 10 : e.key === 'PageDown' ? -60 : e.key === 'PageUp' ? 60 : 0;
        if (step) { e.preventDefault(); e.stopPropagation(); onSeek(Math.max(0, Math.min(duration, position + step))); }
        else if (e.key === 'Home') { e.preventDefault(); onSeek(0); }
        else if (e.key === 'End') { e.preventDefault(); onSeek(Math.max(0, duration - 5)); }
      }}
    >
      <div className={s.rail}>
        <div className={s.buffered} style={{width: `${Math.min(100, pct + buffered)}%`}} />
        {markers.filter(m => m.kind !== 'recap').map(m => <div key={m.kind + m.start} className={s.segment} style={{left: `${(m.start / duration) * 100}%`, width: `${((m.end - m.start) / duration) * 100}%`}} title={m.kind === 'intro' ? 'Intro' : 'Credits'} />)}
        <div className={s.played} style={{width: `${pct}%`}} />
        {chapters.length <= 24 ? chapters.slice(1).map(c => <div key={c.start} className={s.tick} style={{left: `${(c.start / duration) * 100}%`}} />) : null}
        <div className={s.knob} style={{left: `${pct}%`}} />
      </div>
      {hover && duration ? (
        <div className={s.preview} style={{left: Math.max(100, Math.min(hover.x, (ref.current?.clientWidth ?? 0) - 100))}}>
          {thumb ? <div className={s.previewFrame} style={{backgroundImage: thumb(hover.t) ? `url(${thumb(hover.t)})` : undefined}} /> : null}
          {chapterAt(hover.t) && chapters.length > 1 ? <span className={s.previewChapter}>{chapterAt(hover.t)!.title}</span> : null}
          <span className={s.previewTime}>{formatTime(drag ?? hover.t)}</span>
        </div>
      ) : null}
    </div>
  );
}

/** Sample caption lines, so the subtitle and caption settings can be seen working. */
const LINES = ['Where were you last night?', 'I told you, I was at the harbor.', '[door creaks]', 'Then who was on the ferry?', 'Nobody saw anything. That’s the point.', '[distant thunder]', 'We leave at first light.', 'And if they follow us?', 'Then we make sure they don’t.', '[music swells]'];
export const captionAt = (t: number) => LINES[Math.floor(t / 6) % LINES.length]!;
export type CaptionStyle = {size: string; background: string};
export function Captions({text, bottom, style}: {text: string; bottom: number; style: CaptionStyle}) {
  const size = style.size === 'small' ? 18 : style.size === 'large' ? 30 : style.size === 'huge' ? 38 : 24;
  return <div className={cx(s.caption, style.background === 'none' ? s.captionOutline : s.captionBg)} style={{bottom, fontSize: size}} aria-live="off">{text}</div>;
}
export function useStableRef<T>(v: T) { const r = useRef(v); r.current = v; return r; }
