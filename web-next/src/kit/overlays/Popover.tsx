import {createContext, forwardRef, useContext, useId, useMemo, useRef, type ButtonHTMLAttributes, type KeyboardEvent, type ReactNode} from 'react';
import {Popover as RadixPopover} from 'radix-ui';
import {Icon, type IconName} from '../Icon';
import {BottomSheet} from './BottomSheet';
import {classes, focusElement, focusable, useDismissReason, useExclusiveAnchor, useOverlayKeys, usePhone, usePresence, useSurfaceFocus, useSurfaceMotion} from './shared';
import type {AnchoredOverlayProps, OverlayChange, OverlayStyle} from './types';
import styles from './overlays.module.css';

export type PopoverProps = AnchoredOverlayProps & {role?: 'dialog' | 'menu'; onContentKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void};

export function Popover({open, onOpenChange, title, children, description, trigger, anchorRef, modal = false, placement = 'bottom-end', width = 300, initialFocusRef, returnFocusRef, accessibilityLabel, className, testID, role = 'dialog', onContentKeyDown}: PopoverProps) {
  const id = useId(), reason = useDismissReason(), focus = useSurfaceFocus(open, initialFocusRef, returnFocusRef);
  const present = usePresence(open, 160), motionRef = useSurfaceMotion(open, 'menu');
  const anchor = useMemo(() => ({current: {getBoundingClientRect: () => anchorRef?.current?.getBoundingClientRect() ?? new DOMRect()}}), [anchorRef]);
  useOverlayKeys(open, onOpenChange); useExclusiveAnchor(open, onOpenChange);
  if (!trigger && !anchorRef) throw new TypeError('Popover requires a trigger or an anchor ref.');
  return <RadixPopover.Root open={open} onOpenChange={value => { onOpenChange(value, reason.current); reason.current = 'action'; }} modal={modal}>
    {trigger ? <RadixPopover.Trigger asChild>{trigger}</RadixPopover.Trigger> : <RadixPopover.Anchor virtualRef={anchor} />}
    {present && <RadixPopover.Portal forceMount><RadixPopover.Content forceMount ref={motionRef}
      side={placement.startsWith('top') ? 'top' : 'bottom'} align={placement.endsWith('start') ? 'start' : 'end'} sideOffset={8} collisionPadding={8}
      className={classes(styles.popover, className)} style={{'--overlay-width': `${width}px`} as OverlayStyle} role={role}
      aria-label={accessibilityLabel} aria-labelledby={accessibilityLabel ? undefined : `${id}-title`} aria-describedby={description === undefined ? undefined : `${id}-description`}
      data-state={open ? 'open' : 'closed'} data-testid={testID}
      onOpenAutoFocus={focus.onOpenAutoFocus} onCloseAutoFocus={focus.onCloseAutoFocus}
      onEscapeKeyDown={() => { reason.current = 'escape'; }}
      onInteractOutside={() => { reason.current = 'outside'; if (!modal) focus.suppressRestore.current = true; }}
      onKeyDown={event => { onContentKeyDown?.(event); if (event.key === 'Tab' && role === 'menu' && !event.defaultPrevented) {
        event.preventDefault(); focus.suppressRestore.current = true;
        const origin = returnFocusRef?.current ?? anchorRef?.current ?? document.querySelector<HTMLElement>(`[aria-controls="${event.currentTarget.id}"]`);
        const candidates = focusable(document.body).filter(item => !event.currentTarget.contains(item));
        const position = origin ? candidates.indexOf(origin) : -1;
        const next = position < 0 ? candidates[0] : candidates[(position + (event.shiftKey ? -1 : 1) + candidates.length) % candidates.length];
        onOpenChange(false, 'action'); focusElement(next);
      } }}>
      <span id={`${id}-title`} className={styles.visuallyHidden}>{title}</span>
      {description !== undefined && <div id={`${id}-description`} className={styles.popoverDescription}>{description}</div>}
      {children}
    </RadixPopover.Content></RadixPopover.Portal>}
  </RadixPopover.Root>;
}

type RowExtras = {icon?: IconName; description?: ReactNode; selected?: boolean; destructive?: boolean};
export type MenuRowProps = ButtonHTMLAttributes<HTMLButtonElement> & RowExtras;
export const MenuRow = forwardRef<HTMLButtonElement, MenuRowProps>(function MenuRow({icon, description, selected, destructive = false, children, className, type = 'button', ...props}, ref) {
  return <button {...props} ref={ref} type={type} className={classes(styles.menuRow, className)} data-selected={selected || undefined} data-destructive={destructive || undefined} aria-pressed={props.role?.startsWith('menuitem') ? undefined : selected}>
    {icon && <span className={styles.menuIcon}><Icon name={icon} size={18} /></span>}
    <span className={styles.menuLabel}>{children}</span>
    {description !== undefined && <span className={styles.menuDescription}>{description}</span>}
    {selected && <Icon name="i-check" size={18} className={styles.menuCheck} />}
  </button>;
});

const MenuContext = createContext<{change: OverlayChange} | null>(null);
export type MenuItemProps = Omit<MenuRowProps, 'onSelect'> & {onSelect?: (event: Event) => void};
export const MenuItem = forwardRef<HTMLButtonElement, MenuItemProps>(function MenuItem({onSelect, onClick, selected, ...props}, ref) {
  const context = useContext(MenuContext);
  return <MenuRow {...props} ref={ref} selected={selected} role={selected === undefined ? 'menuitem' : 'menuitemradio'} aria-checked={selected} data-menu-item="" tabIndex={-1} onClick={event => {
    onClick?.(event); if (event.defaultPrevented) return;
    const selection = new Event('select', {cancelable: true}); onSelect?.(selection);
    if (!selection.defaultPrevented) context?.change(false, 'action');
  }} onPointerMove={event => { props.onPointerMove?.(event); if (event.pointerType === 'mouse' && !event.currentTarget.disabled) event.currentTarget.focus({preventScroll: true}); }} />;
});

export type MenuGroupProps = {label?: ReactNode; ownerOnly?: boolean; children: ReactNode; className?: string};
export function MenuGroup({label, ownerOnly = false, children, className}: MenuGroupProps) {
  const id = useId();
  return <div className={classes(styles.menuGroup, ownerOnly && styles.ownerGroup, className)} role="group" aria-labelledby={label === undefined ? undefined : id} data-owner-only={ownerOnly || undefined}>
    {label !== undefined && <div id={id} className={styles.menuGroupLabel}>{label}</div>}<div className={styles.menuGroupRows}>{children}</div>
  </div>;
}
export function MenuSeparator() { return <div className={styles.menuSeparator} role="separator" />; }

export type MenuEntry = Readonly<{id: string; label: ReactNode; icon?: IconName; description?: ReactNode; disabled?: boolean; selected?: boolean; destructive?: boolean; onSelect: () => void} | {id: string; separator: true} | {id: string; label: ReactNode; section: true; ownerOnly?: boolean}>;
export type MenuProps = Omit<AnchoredOverlayProps, 'children'> & {items?: readonly MenuEntry[]; children?: ReactNode};

export function Menu({items, children, open, onOpenChange, title, ...props}: MenuProps) {
  const phone = usePhone(), typeahead = useRef({text: '', time: 0}), initial = useRef<HTMLElement | null>(null);
  useExclusiveAnchor(phone && open, onOpenChange);
  const keydown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const rows = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[data-menu-item]:not(:disabled)'));
    if (!rows.length) return;
    const current = rows.indexOf(document.activeElement as HTMLButtonElement);
    let next: HTMLButtonElement | undefined;
    if (event.key === 'ArrowDown') next = rows[(current + 1) % rows.length];
    if (event.key === 'ArrowUp') next = rows[(current - 1 + rows.length) % rows.length];
    if (event.key === 'Home') next = rows[0]; if (event.key === 'End') next = rows.at(-1);
    if (event.key.length === 1 && event.key !== ' ') {
      const now = performance.now(), previous = now - typeahead.current.time < 700 ? typeahead.current.text : '';
      typeahead.current = {text: previous + event.key.toLocaleLowerCase(), time: now};
      const query = typeahead.current.text, repeated = [...query].every(char => char === query[0]);
      const sequence = [...rows.slice(current + 1), ...rows.slice(0, current + 1)];
      next = sequence.find(row => row.textContent?.trim().toLocaleLowerCase().startsWith(repeated ? query[0]! : query));
    }
    if (next) { event.preventDefault(); focusElement(next); }
  };
  const content = <MenuContext.Provider value={{change: onOpenChange}}><div ref={element => { initial.current = element?.querySelector<HTMLButtonElement>('[data-menu-item]:not(:disabled)') ?? null; }} onKeyDown={keydown} className={styles.menuItems}>
    {items?.map(item => 'separator' in item ? <MenuSeparator key={item.id} /> : 'section' in item ? <div key={item.id} className={classes(styles.menuGroupLabel, item.ownerOnly && styles.ownerLabel)}>{item.label}</div> : <MenuItem key={item.id} icon={item.icon} description={item.description} disabled={item.disabled} selected={item.selected} destructive={item.destructive} onSelect={item.onSelect}>{item.label}</MenuItem>)}
    {children}
  </div></MenuContext.Provider>;
  if (phone) return <BottomSheet open={open} onOpenChange={onOpenChange} trigger={props.trigger} title={title} description={props.description} className={classes(styles.phoneMenu, props.className)} fitContent initialFocusRef={props.initialFocusRef ?? initial} returnFocusRef={props.returnFocusRef ?? props.anchorRef} accessibilityLabel={props.accessibilityLabel} testID={props.testID}>{content}</BottomSheet>;
  return <Popover {...props} open={open} onOpenChange={onOpenChange} title={title} role="menu" className={classes(styles.menu, props.className)} initialFocusRef={props.initialFocusRef ?? initial}>{content}</Popover>;
}
