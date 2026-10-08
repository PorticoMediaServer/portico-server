import React, {useEffect, useState} from 'react';
import {uiI18n} from './i18n';
import {cx} from './cx';
import {Button} from './Button';
import {Icon, type IconName} from './Icon';
import {Dialog} from './Dialog';
import {Input} from './Field';
import {Notice} from './Feedback';
import s from './Settings.module.css';

/**
 * Settings and administration primitives. Every admin page is built from
 * these so labels, controls, validation and save semantics align.
 */
export function SettingsPage({children, className}: {children: React.ReactNode; className?: string}) {
  return <div className={cx(s.page, className)}>{children}</div>;
}

export function SettingsGroup({title, description, action, children, id}: {title?: React.ReactNode; description?: React.ReactNode; action?: React.ReactNode; children: React.ReactNode; id?: string}) {
  return (
    <section className={s.group} id={id} aria-label={typeof title === 'string' ? title : undefined}>
      {title || action ? (
        <header className={s.groupHead}>
          <div>
            {title ? <h2 className={s.groupTitle}>{title}</h2> : null}
            {description ? <p className={s.groupDesc}>{description}</p> : null}
          </div>
          {action}
        </header>
      ) : null}
      <div className={s.rows}>{children}</div>
    </section>
  );
}

export function SettingsRow({label, help, control, meta, state, stack, icon, onClick, full, start, id}: {label: React.ReactNode; help?: React.ReactNode; control?: React.ReactNode; meta?: React.ReactNode; state?: React.ReactNode; stack?: boolean; icon?: IconName; onClick?: () => void; full?: boolean; start?: boolean; /** An anchor for a link that lands on this row (Settings search). */ id?: string}) {
  const body = (
    <>
      {/* CON-08: the glyph has its own leading slot, so the subtitle lines up under the title. */}
      <div className={s.rowLead}>
        {icon ? <span className={s.rowIcon} aria-hidden><Icon name={icon} size={16} /></span> : null}
        <div className={s.rowCopy}>
          <span className={s.rowLabel}>{label}</span>
          {help ? <span className={s.rowHelp}>{help}</span> : null}
          {state ? <span className={s.rowState}>{state}</span> : null}
        </div>
      </div>
      <div className={cx(s.rowControl, full && s.full, start && s.start)}>
        {meta ? <span className={s.rowMeta}>{meta}</span> : null}
        {control}
        {onClick ? <Icon name="forward" size={16} /> : null}
      </div>
    </>
  );
  if (onClick) return <button type="button" id={id} className={cx(s.row, s.rowClickable, stack && s.rowStack)} onClick={onClick}>{body}</button>;
  return <div id={id} className={cx(s.row, stack && s.rowStack)}>{body}</div>;
}

export function SettingsActions({children, note, sticky}: {children: React.ReactNode; note?: React.ReactNode; sticky?: boolean}) {
  return (
    <div className={cx(s.actions, sticky && s.actionsSticky)}>
      {note ? <span className={s.actionsNote}>{note}</span> : null}
      {children}
    </div>
  );
}

const freshnessIds = {Checked: 'freshness.checked', Measured: 'freshness.measured', Updated: 'freshness.updated', Observed: 'freshness.observed'} as const;
function freshnessText(verb: string, when: string): string {
  const id = freshnessIds[verb as keyof typeof freshnessIds];
  return id ? uiI18n().t(id, {when}) : `${verb} ${when}`;
}

type RefreshEntry = {at?: string | number | Date; onRefresh?: () => void; refreshing?: boolean};
type RefreshScope = {register: (id: string, entry: RefreshEntry) => () => void};
const RefreshScopeContext = React.createContext<RefreshScope | null>(null);
const RefreshStateContext = React.createContext<readonly RefreshEntry[]>([]);

/**
 * One refresh for a page (Justin, 2 Oct 2026): inside a `PageRefresh`, every panel's own
 * "Updated … · Refresh" is silent and reports to the page instead, and `PageRefreshControl`
 * shows the oldest reading with one Refresh that asks them all again.
 */
export function PageRefresh({children}: {children: React.ReactNode}) {
  const entries = React.useRef(new Map<string, RefreshEntry>());
  const [list, setList] = useState<readonly RefreshEntry[]>([]);
  const scope = React.useMemo<RefreshScope>(() => ({
    register: (id, entry) => {
      entries.current.set(id, entry);
      setList([...entries.current.values()]);
      return () => { entries.current.delete(id); setList([...entries.current.values()]); };
    },
  }), []);
  return <RefreshScopeContext.Provider value={scope}><RefreshStateContext.Provider value={list}>{children}</RefreshStateContext.Provider></RefreshScopeContext.Provider>;
}

export function PageRefreshControl() {
  const list = React.useContext(RefreshStateContext);
  const times = list.map(e => (e.at ? new Date(e.at).getTime() : NaN)).filter(Number.isFinite);
  return <FreshnessView at={times.length ? Math.min(...times) : undefined} verb="Updated" refreshing={list.some(e => e.refreshing)} onRefresh={list.some(e => e.onRefresh) ? () => { for (const e of list) e.onRefresh?.(); } : undefined} />;
}

/** "Checked 2 minutes ago" with a stale treatment after the given age. Silent inside a `PageRefresh`. */
export function Freshness(props: {at?: string | number | Date; staleAfterMs?: number; onRefresh?: () => void; refreshing?: boolean; verb?: string}) {
  const scope = React.useContext(RefreshScopeContext);
  const id = React.useId();
  const at = props.at ? new Date(props.at).getTime() : undefined;
  const latest = React.useRef(props);
  latest.current = props;
  useEffect(() => scope?.register(id, {at, refreshing: props.refreshing, onRefresh: props.onRefresh ? () => latest.current.onRefresh?.() : undefined}), [scope, id, at, props.refreshing, !!props.onRefresh]); // eslint-disable-line react-hooks/exhaustive-deps
  return scope ? null : <FreshnessView {...props} />;
}

function FreshnessView({at, staleAfterMs = 5 * 60_000, onRefresh, refreshing, verb = 'Checked'}: {at?: string | number | Date; staleAfterMs?: number; onRefresh?: () => void; refreshing?: boolean; verb?: string}) {
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick(v => v + 1), 30_000);
    return () => clearInterval(t);
  }, []);
  if (!at) return onRefresh ? <Button variant="ghost" size="sm" icon="refresh" label={uiI18n().t('action.refresh')} onClick={onRefresh} loading={refreshing} /> : null;
  const ms = Date.now() - new Date(at).getTime();
  const stale = ms > staleAfterMs;
  return (
    <span className={cx(s.freshness, stale && s.stale)}>
      <Icon name={stale ? 'warning' : 'clock'} size={13} />
      {freshnessText(verb, relative(ms))}
      {onRefresh ? <Button variant="link" size="sm" label={uiI18n().t('action.refresh')} onClick={onRefresh} loading={refreshing} /> : null}
    </span>
  );
}
/** "just now", "5 minutes ago", "yesterday", then a date, in the viewer's language (I18N-03). For use inside a sentence. */
export function relative(ms: number): string {
  const i18n = uiI18n();
  const text = i18n.relativeTime(Date.now() - Math.max(0, ms));
  return text === i18n.t('relative.justNow') || text === i18n.t('relative.yesterday') ? text.charAt(0).toLocaleLowerCase() + text.slice(1) : text;
}

export function Table({columns, children, className}: {columns: readonly {label: string; num?: boolean; actions?: boolean; width?: string}[]; children: React.ReactNode; className?: string}) {
  return (
    <div className={cx(s.tableWrap, className)}>
      <table className={s.table}>
        <thead><tr>{columns.map((c, i) => <th key={i} className={cx(c.num && s.num, c.actions && s.actionsCell)} style={c.width ? {width: c.width} : undefined}>{c.label}</th>)}</tr></thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}
export const tableCell = {num: s.num, actions: s.actionsCell};

export type StatusTone = 'healthy' | 'warning' | 'danger' | 'neutral' | 'accent' | 'record';
export function Status({tone, children}: {tone: StatusTone; children: React.ReactNode}) {
  const color = tone === 'healthy' ? 'var(--status-healthy)' : tone === 'warning' ? 'var(--status-warning)' : tone === 'danger' ? 'var(--status-danger)' : tone === 'accent' ? 'var(--accent)' : tone === 'record' ? 'var(--status-record)' : 'var(--text-tertiary)';
  return <span className={s.status}><i className={s.statusDot} style={{background: color}} />{children}</span>;
}

export function Metric({label, value, sub}: {label: string; value: React.ReactNode; sub?: React.ReactNode}) {
  return (
    <div className={s.metric}>
      <span className={s.metricLabel}>{label}</span>
      <span className={s.metricValue}>{value}</span>
      {sub ? <span className={s.metricSub}>{sub}</span> : null}
    </div>
  );
}
export function MetricGrid({children}: {children: React.ReactNode}) {
  return <div className={s.metricGrid}>{children}</div>;
}

/**
 * Destructive confirmation. Names the target and impact; optionally requires
 * typing the exact target name. The confirm action is the only danger-toned
 * control on the page.
 */
export function ConfirmDialog({open, onOpenChange, title, body, confirmLabel = 'Confirm', cancelLabel = 'Cancel', onConfirm, destructive = true, typedConfirmation, busy, error}: {open: boolean; onOpenChange: (o: boolean) => void; title: string; body?: React.ReactNode; confirmLabel?: string; cancelLabel?: string; onConfirm: () => void | Promise<void>; destructive?: boolean; typedConfirmation?: string; busy?: boolean; error?: string}) {
  const [typed, setTyped] = useState('');
  useEffect(() => {
    if (!open) setTyped('');
  }, [open]);
  const ready = !typedConfirmation || typed.trim() === typedConfirmation;
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={title} dismissable={!busy} actions={<><Button variant="ghost" label={cancelLabel} onClick={() => onOpenChange(false)} disabled={busy} /><Button variant={destructive ? 'danger' : 'primary'} label={confirmLabel} onClick={() => void onConfirm()} disabled={!ready} loading={busy} /></>}>
      {body ? <div style={{fontSize: 14, color: 'var(--text-secondary)', lineHeight: 1.5}}>{body}</div> : null}
      {typedConfirmation ? <Input label={uiI18n().t('confirm.typeToConfirm', {token: `“${typedConfirmation}”`})} value={typed} onChange={e => setTyped(e.target.value)} autoComplete="off" spellCheck={false} /> : null}
      {error ? <Notice tone="error">{error}</Notice> : null}
    </Dialog>
  );
}
