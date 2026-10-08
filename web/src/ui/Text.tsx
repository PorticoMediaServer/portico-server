import React from 'react';
import {cx} from './cx';
import s from './Text.module.css';

export type TextVariant = 'display' | 'title' | 'heading' | 'subheading' | 'body' | 'bodyStrong' | 'caption' | 'captionStrong' | 'micro' | 'label' | 'mono';
export type TextTone = 'primary' | 'secondary' | 'tertiary' | 'muted' | 'accent' | 'danger' | 'warning' | 'healthy';

type Props<T extends keyof React.JSX.IntrinsicElements = 'span'> = {
  as?: T;
  variant?: TextVariant;
  tone?: TextTone;
  clamp?: 1 | 2 | 3;
  center?: boolean;
  balance?: boolean;
  className?: string;
  children?: React.ReactNode;
} & Omit<React.ComponentPropsWithoutRef<T>, 'as' | 'children' | 'className'>;

/** Typographic roles. The semantic element is chosen by the caller; the role decides the look. */
export function Text<T extends keyof React.JSX.IntrinsicElements = 'span'>({as, variant = 'body', tone = 'primary', clamp, center, balance, className, children, ...rest}: Props<T>) {
  const Tag = (as ?? 'span') as React.ElementType;
  return (
    <Tag className={cx(s[variant], s[tone], clamp && s[`clamp${clamp}`], center && s.center, balance && s.balance, className)} {...rest}>
      {children}
    </Tag>
  );
}

/** Convenience for the classes alone, for elements that need to stay semantic. */
export function textClass(variant: TextVariant, tone: TextTone = 'primary', extra?: string): string {
  return cx(s[variant], s[tone], extra);
}
