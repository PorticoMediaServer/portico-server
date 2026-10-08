import React, {useState} from 'react';
import type {Shape} from '@/data/types';
import {Icon, type IconName} from './Icon';
import {Button} from './Button';
import {cx} from './cx';
import s from './Bits.module.css';

const hashHue = (t: string) => { let h = 0; for (let i = 0; i < t.length; i++) h = (h * 31 + t.charCodeAt(i)) | 0; return Math.abs(h); };
const PLACEHOLDER_TINTS = ['#2b4f66', '#4a3f6b', '#2d5a52', '#6a4436', '#5a4a1f', '#3d4a5a', '#5d3d57', '#2f5d75'];
export const tintOf = (title: string) => PLACEHOLDER_TINTS[hashHue(title) % PLACEHOLDER_TINTS.length]!;

/**
 * Artwork with a tinted placeholder while it loads and a typographic stand-in
 * when there is none, so no card ever shows an empty box.
 */
export const Artwork = React.memo(function Artwork({src, title, shape = 'poster', tint, className, style, icon, eager, alt = '', radius, sizes}: {src?: string; title: string; shape?: Shape; tint?: string; className?: string; style?: React.CSSProperties; icon?: IconName; eager?: boolean; alt?: string; radius?: number; sizes?: string}) {
  const [state, setState] = useState<'loading' | 'loaded' | 'failed'>('loading');
  const missing = !src || state === 'failed';
  const round = shape === 'circle';
  return (
    <div className={cx(s.art, s[shape], className)} style={{'--tint': tint ?? tintOf(title), '--radius': radius !== undefined ? `${radius}px` : undefined, ...style} as React.CSSProperties}>
      {missing ? (
        <div className={s.placeholder} aria-hidden>
          {icon ? <Icon name={icon} size={30} className={s.placeholderIcon} /> : null}
          {round ? <span className={s.initials}>{initials(title)}</span> : icon ? null : <span className={s.placeholderTitle}>{title}</span>}
        </div>
      ) : (
        <img src={src} alt={alt} loading={eager ? 'eager' : 'lazy'} decoding="async" sizes={sizes} data-loaded={state === 'loaded'} data-logo={src.includes('/logos/') || undefined} onLoad={() => setState('loaded')} onError={() => setState('failed')} draggable={false} />
      )}
    </div>
  );
});

export const initials = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map(w => w[0]!.toUpperCase()).join('');

export function Avatar({name, art = 'blue', size = 32, kids, className}: {name: string; art?: string; size?: number; kids?: boolean; className?: string}) {
  return <span className={cx(s.avatar, kids && size >= 40 && s.avatarKids, className)} style={{width: size, height: size, fontSize: Math.round(size * 0.42), '--art': `var(--art-${art})`} as React.CSSProperties} aria-hidden>{initials(name).slice(0, 1)}</span>;
}

export type Tone = 'neutral' | 'good' | 'warn' | 'bad' | 'accent' | 'live';
export function Badge({children, tone = 'neutral', dot, className}: {children: React.ReactNode; tone?: Tone; dot?: boolean; className?: string}) {
  return <span className={cx(s.badge, tone !== 'neutral' && s[tone], dot && s.dotBadge, className)}>{children}</span>;
}

export function Chip({children, pressed, onClick, icon, onRemove, title}: {children: React.ReactNode; pressed?: boolean; onClick?: () => void; icon?: IconName; onRemove?: () => void; title?: string}) {
  return (
    <button type="button" className={s.chip} aria-pressed={pressed} onClick={onRemove ?? onClick} title={title}>
      {icon ? <Icon name={icon} size={15} /> : null}{children}{onRemove ? <Icon name="close" size={14} className={s.chipRemove} /> : null}
    </button>
  );
}
export function Chips({children, scroll, label}: {children: React.ReactNode; scroll?: boolean; label?: string}) {
  return <div className={cx(s.chips, scroll && s.chipsScroll)} role={label ? 'group' : undefined} aria-label={label}>{children}</div>;
}

export function Progress({value, className, label}: {value: number; className?: string; label?: string}) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return <div className={cx(s.progress, className)} role="progressbar" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} aria-label={label ?? 'Progress'}><span style={{width: `${pct}%`}} /></div>;
}
export function Meter({value, tone, label}: {value: number; tone?: 'good' | 'warn' | 'bad'; label?: string}) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return <div className={cx(s.meter, tone === 'warn' && s.meterWarn, tone === 'bad' && s.meterBad, tone === 'good' && s.meterGood)} role="meter" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} aria-label={label}><span style={{width: `${pct}%`}} /></div>;
}

export function Spinner({label = 'Loading', size}: {label?: string; size?: number}) {
  return <span className={s.spinner} role="status" aria-label={label} style={size ? {width: size, height: size} : undefined} />;
}

export function Skeleton({width, height, radius, className, style}: {width?: number | string; height?: number | string; radius?: number; className?: string; style?: React.CSSProperties}) {
  return <span className={cx('skeleton', className)} style={{display: 'block', width, height, borderRadius: radius, ...style}} aria-hidden />;
}

export type StateAction = {label: string; onClick?: () => void; to?: string; icon?: IconName; variant?: 'primary' | 'secondary' | 'ghost'};
export function EmptyState({icon = 'info', title, body, actions, compact, tone}: {icon?: IconName; title: string; body?: React.ReactNode; actions?: StateAction[]; compact?: boolean; tone?: 'bad'}) {
  return (
    <div className={cx(s.state, compact && s.compactState)}>
      <div className={cx(s.stateIcon, tone === 'bad' && s.stateIconBad)}><Icon name={icon} size={28} /></div>
      <h2 className={s.stateTitle}>{title}</h2>
      {body ? <p className={s.stateBody}>{body}</p> : null}
      {actions?.length ? <div className={s.stateActions}>{actions.map((a, i) => <Button key={a.label} variant={a.variant ?? (i === 0 ? 'primary' : 'secondary')} icon={a.icon} label={a.label} onClick={a.onClick} {...(a.to ? {to: a.to} : {})} />)}</div> : null}
    </div>
  );
}

export function ErrorState({title = 'This page didn’t load', body = 'Portico couldn’t reach the server. Check your connection and try again.', onRetry, compact}: {title?: string; body?: string; onRetry?: () => void; compact?: boolean}) {
  return <EmptyState icon="alert" tone="bad" title={title} body={body} compact={compact} actions={onRetry ? [{label: 'Try again', icon: 'refresh', onClick: onRetry}] : undefined} />;
}

export function Surface({children, pad = true, className, style, as: As = 'div'}: {children: React.ReactNode; pad?: boolean; className?: string; style?: React.CSSProperties; as?: 'div' | 'section'}) {
  return <As className={cx(s.surface, pad && s.surfacePad, className)} style={style}>{children}</As>;
}

export function Kbd({children}: {children: React.ReactNode}) {
  return <kbd className={s.kbd}>{children}</kbd>;
}

export function Stars({value, onChange, size = 22}: {value?: number; onChange?: (v: number | undefined) => void; size?: number}) {
  if (!onChange) return <span className={s.stars} aria-label={`${value ?? 0} out of 5 stars`}>{[1, 2, 3, 4, 5].map(n => <Icon key={n} name={n <= (value ?? 0) ? 'starFilled' : 'star'} size={size} />)}</span>;
  return (
    <span role="radiogroup" aria-label="Your rating" style={{display: 'inline-flex'}}>
      {[1, 2, 3, 4, 5].map(n => (
        <button key={n} type="button" role="radio" aria-checked={value === n} aria-label={`${n} star${n === 1 ? '' : 's'}`} className={s.starBtn} data-on={n <= (value ?? 0)} onClick={() => onChange(value === n ? undefined : n)}>
          <Icon name={n <= (value ?? 0) ? 'starFilled' : 'star'} size={size} />
        </button>
      ))}
    </span>
  );
}
