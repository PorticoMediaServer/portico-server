import React from 'react';
import {Link} from '@tanstack/react-router';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';
import s from './Button.module.css';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'dangerSolid' | 'white' | 'default';
type Common = {
  variant?: ButtonVariant;
  size?: 'sm' | 'md' | 'lg';
  icon?: IconName;
  trailingIcon?: IconName;
  label?: React.ReactNode;
  busy?: boolean;
  block?: boolean;
  round?: boolean;
  pressed?: boolean;
  className?: string;
};
type AsButton = Common & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & {to?: undefined; href?: undefined; children?: React.ReactNode};
type AsLink = Common & {to: string; params?: Record<string, string>; search?: Record<string, unknown>; href?: undefined; children?: React.ReactNode; title?: string; onClick?: (e: React.MouseEvent) => void; 'aria-label'?: string; tabIndex?: number};

/**
 * The one button. Renders a router link when given `to`, so navigation keeps
 * real hrefs (middle-click, copy link) and still routes client-side.
 */
export const Button = React.forwardRef<HTMLButtonElement, AsButton | AsLink>(function Button(props, ref) {
  const {variant = 'default', size = 'md', icon, trailingIcon, label, busy, block, round, pressed, className, children, ...rest} = props as AsButton & {to?: string};
  const content = children ?? label;
  const iconOnly = !content;
  const iconSize = size === 'sm' ? 16 : size === 'lg' ? 20 : 18;
  const cls = cx(s.button, variant !== 'default' && s[variant], size !== 'md' && s[size], block && s.block, iconOnly && s.iconOnly, round && s.round, pressed && s.pressed, className);
  const inner = (
    <>
      {busy ? <span className={s.spinner} aria-hidden /> : icon ? <Icon name={icon} size={iconSize} /> : null}
      {content}
      {trailingIcon ? <Icon name={trailingIcon} size={iconSize - 2} /> : null}
    </>
  );
  if ('to' in props && props.to) {
    const {to, params, search, title, onClick, tabIndex} = props as AsLink;
    return <Link to={to as never} params={params as never} search={search as never} className={cls} title={title} onClick={onClick} tabIndex={tabIndex} aria-label={(props as AsLink)['aria-label']}>{inner}</Link>;
  }
  const {type = 'button', disabled, ...buttonRest} = rest as React.ButtonHTMLAttributes<HTMLButtonElement>;
  return <button ref={ref} type={type} className={cls} disabled={disabled || busy} aria-busy={busy || undefined} aria-pressed={pressed === undefined ? undefined : pressed} {...buttonRest}>{inner}</button>;
});

/** An icon-only button; `label` is required and becomes its accessible name and tooltip. */
export const IconButton = React.forwardRef<HTMLButtonElement, Omit<AsButton, 'label' | 'icon'> & {icon: IconName; label: string; tooltip?: boolean}>(function IconButton({icon, label, tooltip = true, ...rest}, ref) {
  return <Button ref={ref} icon={icon} aria-label={label} title={tooltip ? label : undefined} {...rest} />;
});
