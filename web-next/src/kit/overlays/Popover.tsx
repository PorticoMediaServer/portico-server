import {Fragment, createContext, forwardRef, useContext, useId, useEffect, useLayoutEffect, useMemo, useRef, useState, type ButtonHTMLAttributes, type KeyboardEvent, type ReactNode} from 'react';
import {Popover as RadixPopover} from 'radix-ui';
import {Icon, type IconName} from '../Icon';
import {BottomSheet} from './BottomSheet';
import {classes, focusElement, focusable, useDismissReason, useExclusiveAnchor, useInDialog, useOverlayKeys, usePhone, usePresence, useSurfaceFocus, useSurfaceMotion} from './shared';
import type {AnchoredOverlayProps, OverlayChange, OverlayStyle} from './types';
import styles from './overlays.module.css';
import touchStyles from './touch-menu.module.css';

/** select17: caller-supplied facts from the captured current item, without invented metadata. */
export function TouchMenuTitle({title, artwork, kind, year}: {title: string; artwork?: ReactNode; kind?: string; year?: string}) {
  return <span className={touchStyles.title}>{artwork && <span className={touchStyles.art}>{artwork}</span>}<span className={touchStyles.text}>
    <b>{title}</b>{(kind || year) && <span className={touchStyles.facts}>{kind && <span>{kind}</span>}{year && <span>{year}</span>}</span>}
  </span></span>;
}

export type PopoverProps = AnchoredOverlayProps & {role?: 'dialog' | 'menu'; onContentKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void; touchContext?: boolean};

export function Popover({open, onOpenChange, title, children, description, trigger, anchorRef, modal = false, placement = 'bottom-end', sideOffset = 8, alignOffset = 0, width = 300, initialFocusRef, returnFocusRef, accessibilityLabel, className, testID, role = 'dialog', onContentKeyDown, touchContext = false}: PopoverProps) {
  const [spotlight, setSpotlight] = useState<DOMRect | null>(null);
  useLayoutEffect(() => {
    if (!open || !touchContext) {setSpotlight(null); return;}
    let frame: number;
    const measure = () => {
      const element = anchorRef?.current;
      const item = element?.closest<HTMLElement>('[data-card-kind],[data-track-item]') ?? element;
      const next = item?.getBoundingClientRect();
      if (next) setSpotlight(previous => previous && previous.x === next.x && previous.y === next.y && previous.width === next.width && previous.height === next.height ? previous : next);
      frame = requestAnimationFrame(measure);
    };
    measure(); return () => cancelAnimationFrame(frame);
  }, [open, touchContext, anchorRef]);
  const triggerElement = useRef<HTMLButtonElement | null>(null);
  const id = useId(), reason = useDismissReason(), focus = useSurfaceFocus(open, initialFocusRef, returnFocusRef ?? anchorRef ?? triggerElement);
  const withinDialog = useInDialog(open, anchorRef ?? triggerElement);
  const present = usePresence(open, touchContext ? 120 : 160), motionRef = useSurfaceMotion(open, 'menu'), scrimMotion = useSurfaceMotion(open, 'fade');
  const anchor = useMemo(() => ({current: {get contextElement() {return anchorRef?.current ?? undefined;}, getBoundingClientRect: () => anchorRef?.current?.getBoundingClientRect() ?? new DOMRect()}}), [anchorRef]);
  useOverlayKeys(open, onOpenChange); const superseded = useExclusiveAnchor(open, onOpenChange);
  if (superseded) focus.suppressRestore.current = true;
  useLayoutEffect(() => {
    const element = anchorRef?.current;
    if (trigger || !element) return;
    const names = ['aria-haspopup', 'aria-expanded', 'aria-controls'] as const, original = names.map(name => element.getAttribute(name));
    element.setAttribute('aria-haspopup', role); element.setAttribute('aria-expanded', String(open));
    if (open) element.setAttribute('aria-controls', `${id}-content`); else element.removeAttribute('aria-controls');
    return () => names.forEach((name, index) => { const previous = original[index]; if (previous === null || previous === undefined) element.removeAttribute(name); else element.setAttribute(name, previous); });
  }, [trigger, anchorRef, open, role, id]);
  if (!trigger && !anchorRef) throw new TypeError('Popover requires a trigger or an anchor ref.');
  return <RadixPopover.Root open={open} onOpenChange={value => { if (value === open) return; onOpenChange(value, reason.current); reason.current = 'action'; }} modal={modal || touchContext}>
    {trigger ? <RadixPopover.Trigger asChild ref={triggerElement} aria-haspopup={role} aria-controls={open ? `${id}-content` : undefined}>{trigger}</RadixPopover.Trigger> : <RadixPopover.Anchor virtualRef={anchor} />}
    {/* Exit presence must not leave the positioning wrapper as a pointer target. */}
    {present && !superseded && <RadixPopover.Portal forceMount><div style={{display: 'contents'}}>{touchContext && spotlight && <div ref={scrimMotion} aria-hidden="true" style={{position: 'fixed', inset: 0, zIndex: 'var(--layer-menu)', pointerEvents: open ? 'auto' : 'none'}}>{[
      {left: 0, top: 0, right: 0, height: Math.max(0, spotlight.top)},
      {left: 0, top: spotlight.bottom, right: 0, bottom: 0},
      {left: 0, top: spotlight.top, width: Math.max(0, spotlight.left), height: spotlight.height},
      {left: spotlight.right, top: spotlight.top, right: 0, height: spotlight.height},
    ].map((rect, index) => <div key={index} style={{position: 'absolute', background: 'rgba(3,8,12,.58)', ...rect}} />)}</div>}<div style={{pointerEvents: 'none'}}><RadixPopover.Content forceMount ref={motionRef}
      side={touchContext ? spotlight && spotlight.right + 312 > window.innerWidth - 8 ? 'left' : 'right' : placement.startsWith('top') ? 'top' : 'bottom'} align={touchContext ? 'start' : placement.endsWith('start') ? 'start' : 'end'} sideOffset={touchContext ? 12 : sideOffset} alignOffset={alignOffset} updatePositionStrategy={touchContext ? 'always' : 'optimized'} collisionPadding={8}
      id={`${id}-content`} className={classes(styles.popover, className)} style={{'--overlay-width': `${touchContext ? 300 : width}px`, pointerEvents: open ? 'auto' : 'none', ...(touchContext ? {borderRadius: 16, padding: 8, overflow: 'visible', background: 'rgba(22,44,58,.97)', backdropFilter: 'blur(26px) saturate(1.3)'} : {})} as OverlayStyle} role={role} aria-modal={modal && role === 'dialog' && open || undefined}
      aria-label={accessibilityLabel} aria-labelledby={accessibilityLabel ? undefined : `${id}-title`} aria-describedby={description === undefined ? undefined : `${id}-description`}
      data-state={open ? 'open' : 'closed'} data-within-dialog={withinDialog || undefined} data-testid={testID}
      onOpenAutoFocus={focus.onOpenAutoFocus} onCloseAutoFocus={focus.onCloseAutoFocus}
      onEscapeKeyDown={() => { reason.current = 'escape'; }}
      onInteractOutside={event => { if (!open || focus.suppressRestore.current || event.target instanceof Node && anchorRef?.current?.contains(event.target)) { event.preventDefault(); return; } reason.current = 'outside'; if (!modal) focus.suppressRestore.current = true; }}
      onKeyDown={event => { onContentKeyDown?.(event); if (event.key === 'Tab' && role === 'menu' && !event.defaultPrevented) {
        event.preventDefault(); focus.suppressRestore.current = true;
        const origin = returnFocusRef?.current ?? anchorRef?.current ?? triggerElement.current;
        const candidates = focusable(document.body).filter(item => !event.currentTarget.contains(item));
        const position = origin ? candidates.indexOf(origin) : -1;
        const next = position < 0 ? candidates[0] : candidates[(position + (event.shiftKey ? -1 : 1) + candidates.length) % candidates.length];
        onOpenChange(false, 'action'); focusElement(next);
      } }}>
      {touchContext ? <div id={`${id}-title`} style={{padding: '6px 20px 10px', margin: '0 -8px 6px', borderBottom: '1px solid rgba(237,245,250,.08)', fontSize: 15, fontWeight: 800}}>{title}</div> : <span id={`${id}-title`} className={styles.visuallyHidden}>{title}</span>}
      {description !== undefined && <div id={`${id}-description`} className={styles.popoverDescription}>{description}</div>}
      {children}
      {touchContext && <RadixPopover.Arrow width={14} height={7} style={{fill: 'rgba(22,44,58,.97)'}} />}
    </RadixPopover.Content></div></div></RadixPopover.Portal>}
  </RadixPopover.Root>;
}

type RowExtras = {icon?: IconName; description?: ReactNode; selected?: boolean; destructive?: boolean};
export type MenuRowProps = ButtonHTMLAttributes<HTMLButtonElement> & RowExtras;
export const MenuRow = forwardRef<HTMLButtonElement, MenuRowProps>(function MenuRow({icon, description, selected, destructive = false, children, className, type = 'button', ...props}, ref) {
  return <button {...props} ref={ref} type={type} className={classes(styles.menuRow, className)} data-selected={selected || undefined} data-destructive={destructive || undefined} aria-pressed={props.role?.startsWith('menuitem') ? undefined : selected}>
    {icon && <span data-menu-icon className={styles.menuIcon}><Icon name={icon} size={18} /></span>}
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
export type MenuProps = Omit<AnchoredOverlayProps, 'children'> & {items?: readonly MenuEntry[]; children?: ReactNode; presentation?: 'anchored' | 'responsive'; touchContext?: boolean};

function EntryGroups({items, phone, touchContext = false}: {items: readonly MenuEntry[]; phone: boolean; touchContext?: boolean}) {
  type Action = Exclude<MenuEntry, {separator: true} | {section: true}>;
  const groups: {id: string; label?: ReactNode; ownerOnly?: boolean; separator?: boolean; items: Action[]}[] = [{id: 'start', items: []}];
  for (const item of items) {
    if ('separator' in item) groups.push({id: item.id, separator: true, items: []});
    else if ('section' in item) groups.push({id: item.id, label: item.label, ownerOnly: item.ownerOnly, items: []});
    else groups.at(-1)!.items.push(item);
  }
  return <>{groups.filter(group => group.items.length || group.label !== undefined).map(group => <Fragment key={group.id}>
    {!phone && group.separator && <MenuSeparator />}<MenuGroup label={group.label} ownerOnly={group.ownerOnly}>{group.items.map(item => <MenuItem className={touchContext ? touchStyles.row : undefined} style={touchContext ? {height: 44, minHeight: 44} : undefined} key={item.id} icon={item.icon} description={item.description} disabled={item.disabled} selected={item.selected} destructive={item.destructive} onSelect={item.onSelect}>{item.label}</MenuItem>)}</MenuGroup>
  </Fragment>)}</>;
}

export function Menu({items, children, open, onOpenChange, title, presentation = 'responsive', touchContext = false, ...props}: MenuProps) {
  const phone = usePhone(), typeahead = useRef({text: '', time: 0}), initial = useRef<HTMLElement | null>(null), input = useRef<'pointer' | 'keyboard'>('keyboard');
  useEffect(() => {
    const pointer = () => { input.current = 'pointer'; }, keyboard = () => { input.current = 'keyboard'; };
    document.addEventListener('pointerdown', pointer, true); document.addEventListener('keydown', keyboard, true);
    return () => { document.removeEventListener('pointerdown', pointer, true); document.removeEventListener('keydown', keyboard, true); };
  }, []);
  const sheet = phone && presentation === 'responsive';
  useLayoutEffect(() => {
    if (!open || !touchContext) return;
    // A held touch opens before its pointer releases. Focus after the portal
    // mounts and after release's synthetic mouse focus, including reflows.
    let frame: number;
    const focusMenu = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (document.activeElement === document.body || document.activeElement === props.anchorRef?.current) focusElement(props.initialFocusRef?.current ?? initial.current);
      });
    };
    const release = (event: PointerEvent) => {if (event.pointerType !== 'mouse') focusMenu();};
    const blurred = (event: FocusEvent) => {if (event.target instanceof Node && initial.current?.contains(event.target) && event.relatedTarget == null) focusMenu();};
    focusMenu(); document.addEventListener('pointerup', release, true); document.addEventListener('focusout', blurred, true);
    return () => {cancelAnimationFrame(frame); document.removeEventListener('pointerup', release, true); document.removeEventListener('focusout', blurred, true);};
  }, [open, sheet, touchContext, props.initialFocusRef, props.anchorRef]);
  useExclusiveAnchor(sheet && open, onOpenChange);
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
  const content = <MenuContext.Provider value={{change: onOpenChange}}><div tabIndex={-1} ref={element => { initial.current = input.current === 'pointer' ? element : element?.querySelector<HTMLButtonElement>('[data-menu-item]:not(:disabled)') ?? null; }} onKeyDown={keydown} className={styles.menuItems} style={touchContext && !sheet ? {maxHeight: 'calc(100dvh - 32px)', overflowY: 'auto'} : undefined}>
    {items && <EntryGroups items={items} phone={sheet} touchContext={touchContext} />}
    {children}
  </div></MenuContext.Provider>;
  if (sheet) return <BottomSheet open={open} onOpenChange={onOpenChange} trigger={props.trigger} title={title} description={props.description} className={classes(styles.phoneMenu, touchContext && touchStyles.sheet, props.className)} fitContent showCloseButton={touchContext ? false : undefined} backdropOpacity={touchContext ? .58 : undefined} initialFocusRef={props.initialFocusRef ?? initial} returnFocusRef={props.returnFocusRef ?? props.anchorRef} accessibilityLabel={props.accessibilityLabel} testID={props.testID}>{content}</BottomSheet>;
  return <Popover {...props} touchContext={touchContext} open={open} onOpenChange={onOpenChange} title={title} role="menu" className={classes(styles.menu, props.className)} initialFocusRef={props.initialFocusRef ?? initial}>{content}</Popover>;
}
