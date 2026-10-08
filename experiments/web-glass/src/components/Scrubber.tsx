import { useRef, useState, type ReactNode } from 'react';
import { fmtClock } from '../lib/hooks';
import { cls } from './ui';

/** Pointer-driven scrubber: grows on hover, shows a time bubble, supports chapter ticks and keyboard. */
export function Scrubber({
  value,
  max,
  onSeek,
  marks,
  preview,
  buffered,
  className,
  label = 'Seek',
  showTimes = true,
  remaining = true,
}: {
  value: number;
  max: number;
  onSeek: (t: number) => void;
  marks?: number[];
  preview?: (t: number) => ReactNode;
  buffered?: number;
  className?: string;
  label?: string;
  showTimes?: boolean;
  remaining?: boolean;
}) {
  const bar = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [drag, setDrag] = useState<number | null>(null);
  const tFrom = (clientX: number) => {
    const r = bar.current!.getBoundingClientRect();
    return Math.max(0, Math.min(1, (clientX - r.left) / r.width)) * max;
  };
  const shown = drag ?? value;
  const pct = max ? (shown / max) * 100 : 0;
  return (
    <div className={cls('scrubber', (hover !== null || drag !== null) && 'active', className)}>
      <div
        ref={bar}
        className="scrub-track"
        role="slider"
        tabIndex={0}
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={Math.round(max)}
        aria-valuenow={Math.round(shown)}
        aria-valuetext={`${fmtClock(shown)} of ${fmtClock(max)}`}
        onPointerMove={(e) => {
          setHover(tFrom(e.clientX));
          if (drag !== null) setDrag(tFrom(e.clientX));
        }}
        onPointerLeave={() => setHover(null)}
        onPointerDown={(e) => {
          (e.target as HTMLElement).setPointerCapture(e.pointerId);
          setDrag(tFrom(e.clientX));
        }}
        onPointerUp={(e) => {
          if (drag !== null) onSeek(tFrom(e.clientX));
          setDrag(null);
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowRight') onSeek(Math.min(max, value + 10));
          if (e.key === 'ArrowLeft') onSeek(Math.max(0, value - 10));
        }}
      >
        <div className="scrub-rail">
          {buffered !== undefined && <i className="scrub-buffer" style={{ width: `${(buffered / max) * 100}%` }} />}
          <i className="scrub-fill" style={{ width: `${pct}%` }} />
          {marks?.map((m, i) => (i === 0 ? null : <i key={i} className="scrub-mark" style={{ left: `${(m / max) * 100}%` }} />))}
        </div>
        <i className="scrub-knob" style={{ left: `${pct}%` }} />
        {hover !== null && (
          <div className="scrub-bubble" style={{ left: `${(hover / max) * 100}%` }}>
            {preview?.(hover)}
            <span>{fmtClock(hover)}</span>
          </div>
        )}
      </div>
      {showTimes && (
        <div className="scrub-times tnum">
          <span>{fmtClock(shown)}</span>
          <span>{remaining ? `−${fmtClock(max - shown)}` : fmtClock(max)}</span>
        </div>
      )}
    </div>
  );
}
