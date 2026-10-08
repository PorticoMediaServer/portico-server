import React from 'react';
import {Link} from '@tanstack/react-router';
import {Icon, type IconName} from './Icon';
import {Tooltip} from './Overlay';
import {cx} from './cx';

export type ButtonVariant = 'primary' | 'accent' | 'secondary' | 'ghost' | 'danger' | 'glass';
type Common = {
  variant?: ButtonVariant;
  size?: 'sm' | 'md' | 'lg';
  icon?: IconName;
  trailingIcon?: IconName;
  label?: React.ReactNode;
  busy?: boolean;
  className?: string;
  full?: boolean;
};
type ButtonProps = Common & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & {to?: never; href?: never};
type LinkProps = Common & {to: string; params?: Record<string, string>; search?: Record<string, unknown>; href?: never; onClick?: React.MouseEventHandler; 'aria-label'?: string; disabled?: never; title?: string};
type HrefProps = Common & {href: string; to?: never; target?: string; 'aria-label'?: string; onClick?: React.MouseEventHandler; disabled?: never; title?: string};

function body({icon, trailingIcon, label, busy, size}: Common) {
  const s = size === 'lg' ? 22 : size === 'sm' ? 16 : 18;
  return (
    <>
      {busy ? <span className="btn-spinner" aria-hidden /> : icon ? <Icon name={icon} size={s} fill={icon === 'play' ? 'currentColor' : undefined} /> : null}
      {label !== undefined ? <span className="btn-label">{label}</span> : null}
      {trailingIcon ? <Icon name={trailingIcon} size={s - 2} /> : null}
    </>
  );
}

export function Button(props: ButtonProps | LinkProps | HrefProps) {
  const {variant = 'secondary', size = 'md', icon, trailingIcon, label, busy, className, full, ...rest} = props as Common & Record<string, unknown>;
  const cls = cx('btn', `btn-${variant}`, `btn-${size}`, label === undefined && 'btn-icon', full && 'btn-full', busy && 'is-busy', className);
  const inner = body({icon, trailingIcon, label, busy, size});
  if ('to' in props && props.to) {
    const {to, params, search, ...r} = rest as {to: string; params?: Record<string, string>; search?: Record<string, unknown>};
    return <Link to={to as never} params={params as never} search={search as never} className={cls} {...(r as object)}>{inner}</Link>;
  }
  if ('href' in props && props.href) return <a className={cls} {...(rest as object)} rel="noopener noreferrer">{inner}</a>;
  const r = rest as React.ButtonHTMLAttributes<HTMLButtonElement>;
  return <button type="button" className={cls} {...r} disabled={r.disabled || !!busy} aria-busy={busy || undefined}>{inner}</button>;
}

/** A round icon-only button with a tooltip; the label is also its accessible name. */
export function IconButton({icon, label, variant = 'glass', size = 'md', active, className, tooltip = true, ...rest}: {icon: IconName; label: string; variant?: ButtonVariant; size?: 'sm' | 'md' | 'lg'; active?: boolean; tooltip?: boolean} & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'>) {
  const btn = (
    <button type="button" aria-label={label} aria-pressed={active === undefined ? undefined : active} className={cx('btn', `btn-${variant}`, `btn-${size}`, 'btn-icon', 'btn-round', active && 'is-active', className)} {...rest}>
      <Icon name={icon} size={size === 'lg' ? 22 : size === 'sm' ? 16 : 19} fill={active && (icon === 'heart' || icon === 'star' || icon === 'like' || icon === 'dislike' || icon === 'bookmark') ? 'currentColor' : icon === 'play' ? 'currentColor' : undefined} />
    </button>
  );
  return tooltip ? <Tooltip label={label}>{btn}</Tooltip> : btn;
}
