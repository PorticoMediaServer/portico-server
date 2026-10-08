import React, {useEffect, useState} from 'react';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {AdminState, Health} from '../../data/admin';
import {Button} from '../../ui/Button';
import {Badge, Skeleton, toast} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {cx} from '../../ui/cx';

/** Reads one console section's state. */
export function useAdmin<K extends keyof AdminState>(key: K) {
  const services = useServices();
  const version = useDataVersion();
  return useQuery(`admin.${key}.${version}`, () => services.admin.get(key));
}

/**
 * A settings form over one console section: edits stay in a draft until Save, and a bar at the
 * bottom says there are unsaved changes (Discard or Save).
 */
export function useAdminForm<K extends keyof AdminState>(key: K) {
  const services = useServices();
  const q = useAdmin(key);
  const [draft, setDraft] = useState<AdminState[K] | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  useEffect(() => { if (q.data && draft === undefined) setDraft(structuredClone(q.data)); }, [q.data, draft]);
  const dirty = !!q.data && draft !== undefined && JSON.stringify(q.data) !== JSON.stringify(draft);
  const save = async () => {
    if (draft === undefined) return;
    setBusy(true);
    await services.admin.set(key, draft);
    setBusy(false);
    setDraft(undefined);
    toast('Changes saved', {icon: 'check'});
  };
  return {data: draft, loading: q.loading && !q.data, set: (patch: Partial<AdminState[K]>) => setDraft(d => (d ? {...d, ...patch} : d)), replace: setDraft, dirty, busy, save, discard: () => setDraft(q.data ? structuredClone(q.data) : undefined)};
}

export function UnsavedBar({form}: {form: {dirty: boolean; busy: boolean; save: () => Promise<void>; discard: () => void}}) {
  if (!form.dirty) return null;
  return (
    <div className="unsaved" role="region" aria-label="Unsaved changes">
      <Icon name="edit" size={16} />
      <span>You have unsaved changes</span>
      <span className="spacer" />
      <Button size="sm" variant="ghost" label="Discard" onClick={form.discard} />
      <Button size="sm" variant="primary" label="Save changes" busy={form.busy} onClick={() => void form.save()} />
    </div>
  );
}

export function SectionHead({title, lede, actions, eyebrow}: {title: string; lede?: React.ReactNode; actions?: React.ReactNode; eyebrow?: string}) {
  return (
    <header className="adm-head">
      <div className="adm-head-text">
        {eyebrow ? <div className="eyebrow">{eyebrow}</div> : null}
        <h1 className="display adm-title">{title}</h1>
        {lede ? <p className="adm-lede">{lede}</p> : null}
      </div>
      {actions ? <div className="adm-actions">{actions}</div> : null}
    </header>
  );
}

export const healthTone = (h: Health) => (h === 'ok' ? 'good' : h === 'warn' ? 'warn' : 'bad') as 'good' | 'warn' | 'bad';
export function HealthDot({state}: {state: Health}) {
  return <span className={cx('health-dot', `is-${state}`)} aria-label={state === 'ok' ? 'Healthy' : state === 'warn' ? 'Needs a look' : 'Problem'} />;
}

export function Stat({label, value, sub, icon, tone}: {label: string; value: React.ReactNode; sub?: React.ReactNode; icon?: IconName; tone?: 'good' | 'warn' | 'bad'}) {
  return (
    <div className={cx('stat', tone && `is-${tone}`)}>
      <span className="stat-label">{icon ? <Icon name={icon} size={15} /> : null}{label}</span>
      <strong className="stat-value tabular">{value}</strong>
      {sub ? <span className="stat-sub">{sub}</span> : null}
    </div>
  );
}

/** A small area chart for telemetry; values 0–100 unless `max` says otherwise. */
export function Spark({values, max = 100, label, unit = '%', color = 'var(--accent-strong)', height = 64}: {values: number[]; max?: number; label: string; unit?: string; color?: string; height?: number}) {
  const w = 240;
  const pts = values.map((v, i) => [(i / Math.max(1, values.length - 1)) * w, height - (Math.min(max, v) / max) * (height - 6) - 3] as const);
  const line = pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ');
  const last = values[values.length - 1] ?? 0;
  const id = `g-${label.replace(/\W/g, '')}`;
  return (
    <figure className="spark">
      <figcaption><span>{label}</span><strong className="tabular">{Math.round(last)}{unit}</strong></figcaption>
      <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" role="img" aria-label={`${label}: now ${Math.round(last)}${unit}, peak ${Math.round(Math.max(...values))}${unit}`}>
        <defs><linearGradient id={id} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor={color} stopOpacity="0.45" /><stop offset="1" stopColor={color} stopOpacity="0" /></linearGradient></defs>
        <path d={`${line} L${w},${height} L0,${height} Z`} fill={`url(#${id})`} />
        <path d={line} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
      </svg>
    </figure>
  );
}

export function Bars({values, labels, label}: {values: number[]; labels?: string[]; label: string}) {
  const max = Math.max(1, ...values);
  return (
    <div className="bars" role="img" aria-label={`${label}: ${values.join(', ')}`}>
      {values.map((v, i) => <span key={i} className="bar" style={{height: `${(v / max) * 100}%`}} title={labels?.[i] ? `${labels[i]}: ${v}` : String(v)} />)}
    </div>
  );
}

export function AdminSkeleton() {
  return <div className="stack"><Skeleton h={120} r={16} /><Skeleton h={220} r={16} /><Skeleton h={160} r={16} /></div>;
}

export function StatusBadge({state}: {state: string}) {
  const map: Record<string, ['good' | 'warn' | 'bad' | 'accent' | 'neutral' | 'live', string]> = {
    running: ['accent', 'Running'], queued: ['neutral', 'Queued'], done: ['good', 'Done'], failed: ['bad', 'Failed'], cancelled: ['neutral', 'Canceled'],
    mounted: ['good', 'Mounted'], unmounted: ['neutral', 'Not mounted'], error: ['bad', 'Error'], ok: ['good', 'OK'], skipped: ['neutral', 'Skipped'],
    new: ['accent', 'New'], looking: ['warn', 'Looking into it'], fixed: ['good', 'Fixed'], wontFix: ['neutral', 'Won’t fix'],
    playing: ['live', 'Playing'], paused: ['warn', 'Paused'], starting: ['accent', 'Starting'],
    approved: ['good', 'Approved'], blocked: ['bad', 'Blocked'], waiting: ['warn', 'Waiting'],
    upToDate: ['good', 'Up to date'], beingMade: ['accent', 'Being made'], notGenerated: ['warn', 'Not made yet'],
  };
  const [tone, text] = map[state] ?? ['neutral', state];
  return <Badge tone={tone}>{text}</Badge>;
}
