import {useRef, useState} from 'react';
import type {Episode, Movie} from '../data/types';
import {Art} from '../ui/Media';
import {formatClock} from '../screens/shared/items';

/**
 * The scrubber: intro and credits tinted, chapter ticks, a buffered run ahead of the playhead and,
 * on hover or while dragging, a preview frame with the chapter name and time.
 */
export function Timeline({item, position, duration, onSeek}: {item: Movie | Episode | null; position: number; duration: number; onSeek: (t: number) => void}) {
  const track = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [drag, setDrag] = useState<number | null>(null);
  const d = Math.max(1, duration);
  const at = (clientX: number) => {
    const r = track.current!.getBoundingClientRect();
    return Math.max(0, Math.min(1, (clientX - r.left) / r.width)) * d;
  };
  const shown = drag ?? position;
  const probe = drag ?? hover;
  const chapters = item?.chapters ?? [];
  const chapter = probe !== null ? [...chapters].reverse().find(c => c.start <= probe) : undefined;
  const intro = item?.kind === 'episode' && item.introEnd ? item.introEnd : 0;
  const credits = item?.kind === 'episode' && item.creditsStart ? item.creditsStart : 0;
  const pct = (t: number) => `${(t / d) * 100}%`;
  return (
    <div className="timeline">
      <div
        ref={track}
        className="timeline-track"
        role="slider"
        tabIndex={0}
        aria-label="Seek"
        aria-valuemin={0}
        aria-valuemax={Math.round(d)}
        aria-valuenow={Math.round(shown)}
        aria-valuetext={`${formatClock(shown)} of ${formatClock(d)}`}
        onPointerMove={e => { const t = at(e.clientX); setHover(t); if (drag !== null) setDrag(t); }}
        onPointerLeave={() => setHover(null)}
        onPointerDown={e => { (e.target as HTMLElement).setPointerCapture(e.pointerId); setDrag(at(e.clientX)); }}
        onPointerUp={e => { if (drag !== null) onSeek(at(e.clientX)); setDrag(null); }}
        onPointerCancel={() => setDrag(null)}
        onLostPointerCapture={() => setDrag(null)}
        onKeyDown={e => {
          const step = e.shiftKey ? 60 : 10;
          if (e.key === 'ArrowRight') { e.preventDefault(); onSeek(Math.min(d - 1, position + step)); }
          if (e.key === 'ArrowLeft') { e.preventDefault(); onSeek(Math.max(0, position - step)); }
          if (e.key === 'Home') { e.preventDefault(); onSeek(0); }
          if (e.key === 'End') { e.preventDefault(); onSeek(d - 5); }
        }}
        style={{touchAction: 'none'}}
      >
        <div className="timeline-rail">
          <span className="timeline-buffer" style={{width: pct(Math.min(d, shown + d * 0.08))}} />
          {intro ? <span className="timeline-seg" style={{left: 0, width: pct(intro)}} title="Intro" /> : null}
          {credits ? <span className="timeline-seg" style={{left: pct(credits), width: pct(d - credits)}} title="Credits" /> : null}
          <span className="timeline-fill" style={{width: pct(shown)}} />
          {chapters.slice(1).map((c, i) => <span key={i} className="timeline-tick" style={{left: pct(c.start)}} />)}
        </div>
        <span className="timeline-head" style={{left: pct(shown)}} />
        {probe !== null ? (
          <div className="timeline-preview" style={{left: `clamp(90px, ${pct(probe)}, calc(100% - 90px))`}} aria-hidden>
            <Art src={chapter?.art ?? item?.art.backdrop ?? item?.art.still} tint={item?.tint} ratio="16/9" eager />
            <span className="tp-text"><strong>{formatClock(probe)}</strong>{chapter ? <span>{chapter.title}</span> : null}</span>
          </div>
        ) : null}
      </div>
    </div>
  );
}
