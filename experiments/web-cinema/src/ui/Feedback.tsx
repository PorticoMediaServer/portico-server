import React, {useSyncExternalStore} from 'react';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';

export function Spinner({size = 22, label = 'Loading'}: {size?: number; label?: string}) {
  return <span className="spinner" style={{width: size, height: size}} role="status" aria-label={label} />;
}

export function Skeleton({w, h, r, className, style}: {w?: number | string; h?: number | string; r?: number | string; className?: string; style?: React.CSSProperties}) {
  return <span className={cx('skeleton', className)} style={{width: w, height: h, borderRadius: r, ...style}} aria-hidden />;
}

/** Empty, error and permission states: an icon, one sentence of title, a line of help, an action. */
export function StateView({icon = 'info', title, body, action, secondary, tone = 'neutral', compact, children}: {icon?: IconName; title: React.ReactNode; body?: React.ReactNode; action?: {label: string; onClick?: () => void; to?: string; icon?: IconName}; secondary?: {label: string; onClick?: () => void}; tone?: 'neutral' | 'error' | 'warn'; compact?: boolean; children?: React.ReactNode}) {
  return (
    <div className={cx('state-view', compact && 'is-compact', `tone-${tone}`)} role={tone === 'error' ? 'alert' : undefined}>
      <span className="state-icon"><Icon name={icon} size={compact ? 22 : 28} /></span>
      <h2 className="state-title">{title}</h2>
      {body ? <p className="state-body">{body}</p> : null}
      {children}
      {action || secondary ? (
        <div className="state-actions">
          {action ? (action.to ? <a className="btn btn-primary btn-md" href={action.to}>{action.icon ? <Icon name={action.icon} size={18} /> : null}<span className="btn-label">{action.label}</span></a> : <button type="button" className="btn btn-primary btn-md" onClick={action.onClick}>{action.icon ? <Icon name={action.icon} size={18} /> : null}<span className="btn-label">{action.label}</span></button>) : null}
          {secondary ? <button type="button" className="btn btn-ghost btn-md" onClick={secondary.onClick}><span className="btn-label">{secondary.label}</span></button> : null}
        </div>
      ) : null}
    </div>
  );
}

export function ErrorState({error, onRetry, what = 'this page'}: {error: Error | null; onRetry?: () => void; what?: string}) {
  return <StateView icon="wifiOff" tone="error" title={`Portico couldn’t load ${what}`} body={error?.message && !/fetch|network/i.test(error.message) ? error.message : 'Check that the server is on and reachable, then try again.'} action={onRetry ? {label: 'Try again', onClick: onRetry, icon: 'refresh'} : undefined} />;
}

export function Notice({tone = 'info', title, children, action, icon, className}: {tone?: 'info' | 'good' | 'warn' | 'bad'; title?: React.ReactNode; children?: React.ReactNode; action?: React.ReactNode; icon?: IconName; className?: string}) {
  const ic: IconName = icon ?? (tone === 'good' ? 'success' : tone === 'info' ? 'info' : 'alert');
  return (
    <div className={cx('notice', `notice-${tone}`, className)} role={tone === 'bad' ? 'alert' : 'status'}>
      <Icon name={ic} size={18} />
      <div className="notice-text">
        {title ? <strong>{title}</strong> : null}
        {children ? <div>{children}</div> : null}
      </div>
      {action ? <div className="notice-action">{action}</div> : null}
    </div>
  );
}

export function Badge({tone = 'neutral', children, icon, className}: {tone?: 'neutral' | 'accent' | 'good' | 'warn' | 'bad' | 'live' | 'solid'; children: React.ReactNode; icon?: IconName; className?: string}) {
  return <span className={cx('badge', `badge-${tone}`, className)}>{icon ? <Icon name={icon} size={12} stroke={2.4} /> : null}{children}</span>;
}

export function Progress({value, label, tone = 'accent', className, thin}: {value: number; label?: string; tone?: 'accent' | 'good' | 'warn' | 'bad' | 'white'; className?: string; thin?: boolean}) {
  const v = Math.max(0, Math.min(1, value));
  return (
    <div className={cx('progress', `progress-${tone}`, thin && 'is-thin', className)} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(v * 100)} aria-label={label}>
      <span style={{width: `${v * 100}%`}} />
    </div>
  );
}

// ---- Toasts: short confirmations with an optional Undo, announced politely.

export type Toast = {id: number; message: React.ReactNode; tone?: 'neutral' | 'good' | 'bad'; action?: {label: string; onClick: () => void}; icon?: IconName};
let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach(l => l());

export function toast(message: React.ReactNode, opts: Omit<Toast, 'id' | 'message'> & {duration?: number} = {}) {
  const id = nextId++;
  toasts = [...toasts.slice(-2), {id, message, ...opts}];
  emit();
  setTimeout(() => dismissToast(id), opts.duration ?? (opts.action ? 6500 : 4000));
  return id;
}
export function dismissToast(id: number) {
  toasts = toasts.filter(t => t.id !== id);
  emit();
}

export function Toaster() {
  const list = useSyncExternalStore(l => { listeners.add(l); return () => listeners.delete(l); }, () => toasts);
  return (
    <div className="toaster" aria-live="polite" aria-atomic="false">
      {list.map(t => (
        <div key={t.id} className={cx('toast', t.tone && `toast-${t.tone}`)} role="status">
          <Icon name={t.icon ?? (t.tone === 'bad' ? 'alert' : 'success')} size={18} />
          <span className="toast-msg">{t.message}</span>
          {t.action ? <button type="button" className="toast-action" onClick={() => { t.action!.onClick(); dismissToast(t.id); }}>{t.action.label}</button> : null}
          <button type="button" className="toast-close" aria-label="Dismiss" onClick={() => dismissToast(t.id)}><Icon name="close" size={15} /></button>
        </div>
      ))}
    </div>
  );
}
