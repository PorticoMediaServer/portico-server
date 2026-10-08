import React from 'react';
import {Tabs as RTabs} from 'radix-ui';
import {cx} from './cx';
import {Icon, type IconName} from './Icon';
import s from './Tabs.module.css';

export type TabItem = {id: string; label: string; disabled?: boolean; count?: number};

/**
 * Peer views. Arrow keys move between tabs, activation selects. The
 * controlled `value` is durable selection; focus is never selection.
 */
export function Tabs({items, value, onChange, size, inline, children, label}: {items: readonly TabItem[]; value: string; onChange: (id: string) => void; size?: 'large'; inline?: boolean; children?: React.ReactNode; label?: string}) {
  return (
    <RTabs.Root value={value} onValueChange={onChange} className={size === 'large' ? s.large : undefined}>
      <RTabs.List className={cx(s.list, inline && s.inline)} aria-label={label}>
        {items.map(t => (
          <RTabs.Trigger key={t.id} value={t.id} className={s.tab} disabled={t.disabled}>
            {t.label}
            {t.count != null ? <span style={{marginLeft: 6, opacity: 0.6, fontWeight: 500}}>{t.count}</span> : null}
          </RTabs.Trigger>
        ))}
      </RTabs.List>
      {children}
    </RTabs.Root>
  );
}
export function TabPanel({id, children, className}: {id: string; children: React.ReactNode; className?: string}) {
  return <RTabs.Content value={id} className={cx(s.content, className)}>{children}</RTabs.Content>;
}

export function Segmented<T extends string>({options, value, onChange, label, size}: {options: readonly {id: T; label?: string; icon?: IconName}[]; value: T; onChange: (id: T) => void; label: string; size?: 'sm'}) {
  return (
    <div className={s.segmented} role="group" aria-label={label}>
      {options.map(o => (
        <button key={o.id} type="button" className={s.segment} aria-pressed={o.id === value} onClick={() => onChange(o.id)} aria-label={o.label ?? o.id} title={o.label}>
          {o.icon ? <Icon name={o.icon} size={16} /> : null}
          {o.label && !o.icon ? o.label : null}
          {o.label && o.icon ? <span className="visually-hidden">{o.label}</span> : null}
        </button>
      ))}
    </div>
  );
}

/**
 * A pill toggle. It forwards its ref and spreads unknown props so it can be a Radix
 * `asChild` trigger (Popover, DropdownMenu): without that the trigger has no anchor and no
 * pointer handlers, and the menu either opens off screen or not at all.
 */
export const Chip = React.forwardRef<HTMLButtonElement, {label: string; pressed?: boolean; onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void; icon?: IconName; count?: number} & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'onClick' | 'children'>>(
  function Chip({label, pressed, onClick, icon, count, className, ...rest}, ref) {
    return (
      <button ref={ref} type="button" aria-pressed={!!pressed} data-state={pressed ? 'on' : 'off'} {...rest} className={cx(s.chip, className)} onClick={onClick}>
        {icon ? <Icon name={icon} size={14} /> : null}
        {label}
        {count != null ? <span style={{opacity: 0.7}}>{count}</span> : null}
      </button>
    );
  },
);
export function Chips({children}: {children: React.ReactNode}) {
  return <div className={s.chips}>{children}</div>;
}
