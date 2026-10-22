import {useId, type ComponentPropsWithoutRef, type ReactNode} from 'react';
import {Tabs as TabsPrimitive, RadioGroup} from 'radix-ui';
import {Icon, type IconName} from '../Icon';
import styles from './controls.module.css';

export type ChoiceItem = Readonly<{value: string; label: ReactNode; disabled?: boolean}>;
export type TabsProps = Omit<ComponentPropsWithoutRef<typeof TabsPrimitive.Root>, 'children' | 'defaultValue' | 'onChange'> & {
  items: readonly (ChoiceItem & {panel?: ReactNode})[];
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  phone?: boolean;
  listClassName?: string;
};

export function Tabs({items, value, defaultValue, onValueChange, phone, className, listClassName, 'aria-label': label, ...props}: TabsProps) {
  const initial = defaultValue ?? items.find(item => !item.disabled)?.value;
  return <TabsPrimitive.Root {...props} value={value} defaultValue={initial} onValueChange={onValueChange} className={className}>
    <TabsPrimitive.List className={[styles.tabs, listClassName].filter(Boolean).join(' ')} data-phone={phone || undefined} aria-label={label}>
      {items.map(item => <TabsPrimitive.Trigger key={item.value} value={item.value} disabled={item.disabled} className={styles.tab}>{item.label}</TabsPrimitive.Trigger>)}
    </TabsPrimitive.List>
    {items.map(item => item.panel !== undefined && <TabsPrimitive.Content key={item.value} value={item.value} className={styles.tabPanel}>{item.panel}</TabsPrimitive.Content>)}
  </TabsPrimitive.Root>;
}

export type SegmentedProps = Omit<ComponentPropsWithoutRef<typeof RadioGroup.Root>, 'children' | 'defaultValue' | 'onChange'> & {
  items: readonly (ChoiceItem & {icon?: IconName; accessibleLabel?: string})[];
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  variant?: 'glass' | 'browse' | 'phone';
  name?: string;
  disabled?: boolean;
};

export function Segmented({items, value, defaultValue, onValueChange, variant = 'glass', name, disabled, className, ...props}: SegmentedProps) {
  const id = useId();
  return <RadioGroup.Root {...props} id={props.id ?? id} orientation="horizontal" value={value} defaultValue={defaultValue ?? items.find(item => !item.disabled)?.value}
    onValueChange={onValueChange} disabled={disabled} name={name} className={[styles.segmented, className].filter(Boolean).join(' ')} data-variant={variant}>
    {items.map(item => <RadioGroup.Item key={item.value} value={item.value} disabled={item.disabled} aria-label={item.accessibleLabel} className={styles.segment}>
      {item.icon && <Icon name={item.icon} size={18} />}{item.label}
    </RadioGroup.Item>)}
  </RadioGroup.Root>;
}
