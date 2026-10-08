import React from 'react';
import {Link, type LinkProps} from '@tanstack/react-router';
import {cx} from './cx';
import {Icon, type IconName} from './Icon';
import s from './Button.module.css';

export type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger' | 'link' | 'glass';
type Common = {
  variant?: ButtonVariant;
  size?: 'sm' | 'md' | 'lg';
  icon?: IconName;
  iconAfter?: IconName;
  iconSize?: number;
  label?: string;
  selected?: boolean;
  round?: boolean;
  block?: boolean;
  loading?: boolean;
  className?: string;
  children?: React.ReactNode;
};
type ButtonProps = Common & Omit<React.ComponentPropsWithoutRef<'button'>, 'children' | 'className'> & {to?: never; href?: never};
type RouterLinkProps = Common & {to: LinkProps['to']; params?: LinkProps['params']; search?: LinkProps['search']; href?: never; disabled?: boolean; onClick?: React.MouseEventHandler};
type AnchorProps = Common & Omit<React.ComponentPropsWithoutRef<'a'>, 'children' | 'className'> & {href: string; to?: never};

function content({icon, iconAfter, iconSize, label, children, loading, size}: Common) {
  const px = iconSize ?? (size === 'sm' ? 16 : size === 'lg' ? 20 : 18);
  // Component spec: loading keeps the geometry and the label; a spinner the size of the icon takes the leading icon's place.
  return (
    <>
      {loading ? <span className={s.spinner} style={{width: px, height: px}} aria-hidden /> : icon ? <Icon name={icon} size={px} /> : null}
      {label ?? children}
      {iconAfter ? <Icon name={iconAfter} size={px} /> : null}
    </>
  );
}

function classes(p: Common, iconOnly: boolean) {
  return cx(s.button, s[p.variant ?? 'secondary'], p.size && p.size !== 'md' && s[p.size], p.round && s.round, p.block && s.block, p.selected && s.selected, p.loading && s.loading, iconOnly && s.iconOnly, p.className);
}

/**
 * The one action primitive. Primary is the only filled blue action; everything
 * else is quiet material. Renders a router link, a plain anchor, or a button.
 */
export function Button(props: ButtonProps | RouterLinkProps | AnchorProps) {
  const iconOnly = !!props.icon && props.label === undefined && props.children === undefined;
  if ('to' in props && props.to) {
    const {variant, size, icon, iconAfter, iconSize, label, selected, round, block, loading, className, children, to, params, search, disabled, onClick, ...rest} = props as RouterLinkProps;
    return (
      <Link to={to} params={params} search={search} className={classes(props, iconOnly)} aria-disabled={disabled || undefined} onClick={onClick} {...(rest as object)}>
        {content(props)}
      </Link>
    );
  }
  if ('href' in props && props.href) {
    const {variant, size, icon, iconAfter, iconSize, label, selected, round, block, loading, className, children, ...rest} = props as AnchorProps;
    return (
      <a className={classes(props, iconOnly)} {...rest}>
        {content(props)}
      </a>
    );
  }
  // `disabled` is destructured out of the pass-through so the computed value is
  // the one that lands on the element. Spreading `rest` after it used to let an
  // explicit `disabled={false}` — which is what a validity check writes while a
  // save is in flight — overwrite the loading lock and leave a busy action
  // clickable. The lock is one-way: loading always disables.
  const {variant, size, icon, iconAfter, iconSize, label, selected, round, block, loading, className, children, type = 'button', disabled, ...rest} = props as ButtonProps;
  return (
    <button type={type} className={classes(props, iconOnly)} aria-pressed={selected} aria-busy={loading || undefined} {...rest} disabled={disabled || loading}>
      {content(props)}
    </button>
  );
}

/** Icon-only button with a required accessible name. */
export function IconButton({name, label, size = 'md', ...rest}: {name: IconName; label: string} & Omit<ButtonProps, 'icon' | 'label' | 'children'>) {
  return <Button icon={name} aria-label={label} title={label} size={size} {...rest} />;
}
