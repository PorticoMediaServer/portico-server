import { useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { cls } from '../../components/ui';
import { useIsMobile } from '../../lib/hooks';

export function Panel({ title, action, children, className, pad = true, sub }: { title?: ReactNode; action?: ReactNode; children: ReactNode; className?: string; pad?: boolean; sub?: ReactNode }) {
  return (
    <section className={cls('cpanel', className)}>
      {(title || action) && (
        <header className="cpanel-head">
          <div>
            {title && <h2>{title}</h2>}
            {sub && <div className="cpanel-sub">{sub}</div>}
          </div>
          {action && <div className="cpanel-action">{action}</div>}
        </header>
      )}
      <div className={cls(pad && 'cpanel-body')}>{children}</div>
    </section>
  );
}

export function Dot({ tone }: { tone: 'ok' | 'warn' | 'err' | 'idle' | 'info' | 'live' }) {
  return <i className={cls('cdot', tone)} aria-hidden="true" />;
}

export function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [w, setW] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setW(el.clientWidth));
    ro.observe(el);
    setW(el.clientWidth);
    return () => ro.disconnect();
  }, []);
  return [ref, w] as const;
}

export function Spark({ values, color = 'var(--blue-400)', height = 34, max }: { values: number[]; color?: string; height?: number; max?: number }) {
  const [ref, w] = useWidth<HTMLDivElement>();
  const path = useMemo(() => {
    if (!w || values.length < 2) return { line: '', area: '' };
    const m = max ?? Math.max(...values) * 1.15;
    const step = w / (values.length - 1);
    const pts = values.map((v, i) => [i * step, height - (v / m) * (height - 2) - 1]);
    const line = pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`).join('');
    return { line, area: `${line}L${w},${height}L0,${height}Z` };
  }, [values, w, height, max]);
  return (
    <div ref={ref} className="spark" style={{ height }}>
      <svg width={w} height={height} aria-hidden="true">
        <path d={path.area} fill={color} opacity={0.14} />
        <path d={path.line} fill="none" stroke={color} strokeWidth={1.6} strokeLinejoin="round" />
      </svg>
    </div>
  );
}

export function Stat({ label, value, unit, sub, spark, color, tone }: { label: string; value: ReactNode; unit?: string; sub?: ReactNode; spark?: number[]; color?: string; tone?: 'warn' | 'err' | 'ok' }) {
  return (
    <div className={cls('cstat', tone)}>
      <div className="cstat-label">{label}</div>
      <div className="cstat-value tnum">
        {value}
        {unit && <span className="cstat-unit">{unit}</span>}
      </div>
      {sub && <div className="cstat-sub">{sub}</div>}
      {spark && <Spark values={spark} color={color} />}
    </div>
  );
}

/** Multi-series time chart with a hover crosshair. */
export function TimeChart({ series, height = 200, unit = '', fmt = (v: number) => v.toFixed(0), times }: { series: { name: string; color: string; values: number[] }[]; height?: number; unit?: string; fmt?: (v: number) => string; times: number[] }) {
  const [ref, w] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const L = 40;
  const B = 22;
  const n = times.length;
  const max = Math.max(1, ...series.flatMap((s) => s.values)) * 1.12;
  const niceMax = (() => {
    const p = Math.pow(10, Math.floor(Math.log10(max)));
    return Math.ceil(max / p) * p;
  })();
  const iw = Math.max(0, w - L - 6);
  const ih = height - B - 6;
  const x = (i: number) => L + (i / Math.max(1, n - 1)) * iw;
  const y = (v: number) => 6 + ih - (v / niceMax) * ih;
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => f * niceMax);
  const xt = [0, Math.floor(n / 4), Math.floor(n / 2), Math.floor((3 * n) / 4), n - 1];
  return (
    <div
      ref={ref}
      className="tchart"
      style={{ height }}
      onPointerMove={(e) => {
        const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
        const i = Math.round(((e.clientX - r.left - L) / iw) * (n - 1));
        setHover(i >= 0 && i < n ? i : null);
      }}
      onPointerLeave={() => setHover(null)}
    >
      {w > 0 && (
        <svg width={w} height={height} role="img" aria-label={series.map((s) => s.name).join(', ')}>
          {ticks.map((t) => (
            <g key={t}>
              <line x1={L} x2={w - 6} y1={y(t)} y2={y(t)} className="tc-grid" />
              <text x={L - 8} y={y(t) + 4} className="tc-label" textAnchor="end">
                {fmt(t)}
              </text>
            </g>
          ))}
          {xt.map((i) => (
            <text key={i} x={x(i)} y={height - 5} className="tc-label" textAnchor={i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle'}>
              {new Date(times[i]).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}
            </text>
          ))}
          {series.map((s) => {
            const d = s.values.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('');
            return (
              <g key={s.name}>
                <path d={`${d}L${x(n - 1)},${y(0)}L${x(0)},${y(0)}Z`} fill={s.color} opacity={0.1} />
                <path d={d} fill="none" stroke={s.color} strokeWidth={1.8} strokeLinejoin="round" />
              </g>
            );
          })}
          {hover !== null && (
            <g>
              <line x1={x(hover)} x2={x(hover)} y1={6} y2={6 + ih} className="tc-cross" />
              {series.map((s) => (
                <circle key={s.name} cx={x(hover)} cy={y(s.values[hover])} r={3.5} fill={s.color} stroke="#0b1622" strokeWidth={2} />
              ))}
            </g>
          )}
        </svg>
      )}
      {hover !== null && w > 0 && (
        <div className="tc-tip" style={{ left: Math.min(w - 160, Math.max(0, x(hover) + 10)) }}>
          <div className="tc-tip-time">{new Date(times[hover]).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', second: '2-digit' })}</div>
          {series.map((s) => (
            <div key={s.name} className="tc-tip-row">
              <i style={{ background: s.color }} />
              {s.name}
              <b className="tnum">
                {fmt(s.values[hover])}
                {unit}
              </b>
            </div>
          ))}
        </div>
      )}
      <div className="tc-legend">
        {series.map((s) => (
          <span key={s.name}>
            <i style={{ background: s.color }} />
            {s.name}
          </span>
        ))}
      </div>
    </div>
  );
}

export interface Col<T> {
  key: string;
  label: string;
  render: (row: T) => ReactNode;
  sort?: (row: T) => number | string;
  width?: string;
  align?: 'right' | 'left';
  className?: string;
}

/** Dense, sortable, keyboard-navigable table (j/k or ↑/↓ to move, Enter to open). */
export function Table<T>({ rows, cols, getKey, onOpen, empty, initialSort, dense }: { rows: T[]; cols: Col<T>[]; getKey: (r: T) => string; onOpen?: (r: T) => void; empty?: ReactNode; initialSort?: { key: string; dir: 1 | -1 }; dense?: boolean }) {
  const [sort, setSort] = useState(initialSort ?? null);
  const [sel, setSel] = useState(-1);
  const mobile = useIsMobile();
  const sorted = useMemo(() => {
    if (!sort) return rows;
    const c = cols.find((x) => x.key === sort.key);
    if (!c?.sort) return rows;
    return [...rows].sort((a, b) => {
      const va = c.sort!(a);
      const vb = c.sort!(b);
      return (va < vb ? -1 : va > vb ? 1 : 0) * sort.dir;
    });
  }, [rows, cols, sort]);
  const template = cols.map((c) => c.width ?? 'minmax(0,1fr)').join(' ');
  if (mobile) {
    // Phones get a stacked list: the first column leads, the rest become labelled facts.
    const [lead, ...rest] = cols;
    return (
      <div className="ccards">
        {sorted.length === 0 && <div className="ctable-empty">{empty ?? 'Nothing to show'}</div>}
        {sorted.map((r) => (
          <div key={getKey(r)} className={cls('ccard', onOpen && 'clickable')} onClick={() => onOpen?.(r)} role={onOpen ? 'button' : undefined}>
            <div className="ccard-lead">{lead.render(r)}</div>
            <dl className="ccard-facts">
              {rest
                .filter((c) => c.label)
                .map((c) => (
                  <div key={c.key}>
                    <dt>{c.label}</dt>
                    <dd>{c.render(r)}</dd>
                  </div>
                ))}
            </dl>
            {rest.filter((c) => !c.label).map((c) => (
              <div key={c.key} className="ccard-extra">
                {c.render(r)}
              </div>
            ))}
          </div>
        ))}
      </div>
    );
  }
  return (
    <div
      className={cls('ctable', dense && 'dense')}
      role="grid"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'j' || e.key === 'ArrowDown') {
          e.preventDefault();
          setSel((s) => Math.min(sorted.length - 1, s + 1));
        } else if (e.key === 'k' || e.key === 'ArrowUp') {
          e.preventDefault();
          setSel((s) => Math.max(0, s - 1));
        } else if (e.key === 'Enter' && sel >= 0 && onOpen) onOpen(sorted[sel]);
      }}
    >
      <div className="ctr head" role="row" style={{ gridTemplateColumns: template }}>
        {cols.map((c) => (
          <button
            key={c.key}
            role="columnheader"
            className={cls('cth', c.align === 'right' && 'right', !c.sort && 'nosort')}
            onClick={() => c.sort && setSort((s) => (s?.key === c.key ? { key: c.key, dir: s.dir === 1 ? -1 : 1 } : { key: c.key, dir: 1 }))}
            tabIndex={-1}
          >
            {c.label}
            {sort?.key === c.key && (sort.dir === 1 ? <ArrowUp /> : <ArrowDown />)}
          </button>
        ))}
      </div>
      {sorted.length === 0 && <div className="ctable-empty">{empty ?? 'Nothing to show'}</div>}
      {sorted.map((r, i) => (
        <div
          key={getKey(r)}
          role="row"
          className={cls('ctr', i === sel && 'sel', onOpen && 'clickable')}
          style={{ gridTemplateColumns: template }}
          onClick={() => {
            setSel(i);
            onOpen?.(r);
          }}
        >
          {cols.map((c) => (
            <div key={c.key} role="gridcell" className={cls('ctd', c.align === 'right' && 'right', c.className)}>
              {c.render(r)}
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

export function KV({ rows, mono }: { rows: [string, ReactNode][]; mono?: boolean }) {
  return (
    <dl className={cls('ckv', mono && 'mono')}>
      {rows.map(([k, v]) => (
        <div key={k}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Bar({ value, tone }: { value: number; tone?: 'warn' | 'err' }) {
  return (
    <span className={cls('cmeter', tone)}>
      <i style={{ width: `${Math.min(100, value * 100)}%` }} />
    </span>
  );
}

export function SettingRow({ title, sub, children }: { title: string; sub?: ReactNode; children: ReactNode }) {
  return (
    <div className="csetting">
      <div className="csetting-text">
        <div className="csetting-title">{title}</div>
        {sub && <div className="csetting-sub">{sub}</div>}
      </div>
      <div className="csetting-control">{children}</div>
    </div>
  );
}
