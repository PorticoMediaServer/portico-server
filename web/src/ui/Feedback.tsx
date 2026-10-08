import React from 'react';
import {cx} from './cx';
import {uiI18n} from './i18n';
import {Icon, type IconName} from './Icon';
import {Button} from './Button';
import s from './Feedback.module.css';

export type Tone = 'info' | 'success' | 'warning' | 'error' | 'neutral';
const icons: Record<Tone, IconName> = {info: 'info', success: 'success', warning: 'warning', error: 'error', neutral: 'info'};

/** One notice for every message. Errors use role=alert; the rest role=status. */
export function Notice({tone = 'info', title, children, action, secondaryAction, compact, className, icon}: {tone?: Tone; title?: string; children?: React.ReactNode; action?: {label: string; onClick: () => void; loading?: boolean}; secondaryAction?: {label: string; onClick: () => void}; compact?: boolean; className?: string; icon?: IconName | null}) {
  return (
    <div className={cx(s.notice, s[tone], compact && s.compact, className)} role={tone === 'error' ? 'alert' : 'status'}>
      {icon !== null ? <Icon name={icon ?? icons[tone]} size={18} className={s.icon} /> : null}
      <div className={s.noticeBody}>
        {title ? <span className={s.noticeTitle}>{title}</span> : null}
        {children ? <span>{children}</span> : null}
        {action || secondaryAction ? (
          <div className={s.noticeActions}>
            {action ? <Button size="sm" variant="secondary" label={action.label} onClick={action.onClick} loading={action.loading} /> : null}
            {secondaryAction ? <Button size="sm" variant="ghost" label={secondaryAction.label} onClick={secondaryAction.onClick} /> : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function Skeleton({width, height, radius, className, style}: {width?: number | string; height?: number | string; radius?: number | string; className?: string; style?: React.CSSProperties}) {
  return <div className={cx(s.skeleton, className)} aria-hidden style={{width, height, borderRadius: radius, ...style}} />;
}

/** Empty, error and offline states. Atmosphere stays; copy is specific; actions only when they help. */
export function StateView({icon = 'info', title, body, action, secondaryAction, inline, children}: {icon?: IconName; title: string; body?: React.ReactNode; action?: {label: string; onClick: () => void; loading?: boolean}; secondaryAction?: {label: string; onClick: () => void}; inline?: boolean; children?: React.ReactNode}) {
  return (
    <div className={cx(s.state, inline && s.stateInline)}>
      <span className={s.stateIcon}><Icon name={icon} size={24} strokeWidth={1.5} /></span>
      <span className={s.stateTitle}>{title}</span>
      {body ? <span className={s.stateBody}>{body}</span> : null}
      {children}
      {action || secondaryAction ? (
        <div className={s.stateActions}>
          {action ? <Button variant="primary" label={action.label} onClick={action.onClick} loading={action.loading} /> : null}
          {secondaryAction ? <Button variant="ghost" label={secondaryAction.label} onClick={secondaryAction.onClick} /> : null}
        </div>
      ) : null}
    </div>
  );
}

export function Badge({tone = 'neutral', children, dot, live, icon, outline}: {tone?: 'neutral' | 'accent' | 'healthy' | 'warning' | 'danger' | 'record'; children: React.ReactNode; dot?: boolean; live?: boolean; icon?: IconName; outline?: boolean}) {
  return (
    <span className={cx(s.badge, tone !== 'neutral' && s[tone], live && s.live, outline && s.outline)}>
      {dot || live ? <i className={s.dot} aria-hidden /> : null}
      {icon ? <Icon name={icon} size={12} /> : null}
      {children}
    </span>
  );
}

export function ProgressBar({value, thin, indeterminate, label}: {value?: number; thin?: boolean; indeterminate?: boolean; label?: string}) {
  const pct = value == null ? 0 : Math.max(0, Math.min(100, value * 100));
  return (
    <div className={cx(s.bar, thin && s.thin, indeterminate && s.indeterminate)} role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={indeterminate ? undefined : Math.round(pct)}>
      <i style={indeterminate ? undefined : {width: `${pct}%`}} />
    </div>
  );
}

export function Spinner({className}: {className?: string}) {
  return <span className={cx(s.spinner, className)} aria-hidden />;
}
/** Spinner with "Loading {thing}" (component spec: no ellipsis, never a bare "Loading" if the caller can say what). */
export function Loading({label = uiI18n().t('status.loading')}: {label?: string}) {
  return <div className={s.loading} role="status"><Spinner />{label}</div>;
}

export function KeyValue({rows}: {rows: readonly [React.ReactNode, React.ReactNode][]}) {
  return (
    <dl className={s.kv}>
      {rows.map(([k, v], i) => (
        <React.Fragment key={i}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </React.Fragment>
      ))}
    </dl>
  );
}

/**
 * A calm, centred status: one thin ring (or icon), a short title, one sentence,
 * and at most two actions. Used for opening, reconnecting and hard failures on
 * every platform so the product has a single way of waiting and of failing.
 */
export function StatusScreen({title, body, spinner = true, icon, action, secondaryAction, className}: {title: string; body?: React.ReactNode; spinner?: boolean; icon?: IconName; action?: {label: string; onClick: () => void}; secondaryAction?: {label: string; onClick: () => void}; className?: string}) {
  return (
    <div className={cx(s.status, className)} role="status" aria-live="polite">
      {spinner ? <span className={s.statusRing} aria-hidden /> : icon ? <span className={s.statusIcon}><Icon name={icon} size={28} /></span> : null}
      <h1 className={s.statusTitle}>{title}</h1>
      {body ? <p className={s.statusBody}>{body}</p> : null}
      {action || secondaryAction ? (
        <div className={s.statusActions}>
          {action ? <Button variant="primary" label={action.label} onClick={action.onClick} /> : null}
          {secondaryAction ? <Button variant="ghost" label={secondaryAction.label} onClick={secondaryAction.onClick} /> : null}
        </div>
      ) : null}
    </div>
  );
}
