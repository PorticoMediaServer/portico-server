import React from 'react';
import {DropdownMenu, Popover as RPopover, Tooltip as RTooltip} from 'radix-ui';
import {cx} from './cx';
import {Icon, type IconName} from './Icon';
import s from './Overlay.module.css';

/** A viewport rectangle a menu opens from: a control's box, or a point for right-click and long press. */
export type MenuAnchor = {x: number; y: number; width: number; height: number};

/** The anchor for a control that was just activated. */
export function anchorOf(element: Element): MenuAnchor {
  const r = element.getBoundingClientRect();
  return {x: r.left, y: r.top, width: r.width, height: r.height};
}

/** The anchor for a pointer position (context menu, long press). */
export function anchorAt(x: number, y: number): MenuAnchor {
  return {x, y, width: 0, height: 0};
}

export type MenuItem = {id: string; label: string; icon?: IconName; meta?: string; disabled?: boolean; destructive?: boolean; selected?: boolean; separatorBefore?: boolean; group?: string};

/**
 * Context menu on quiet material. Options are rows, not buttons; blue marks
 * selection and keyboard highlight only.
 */
/** `header` is a non-interactive block above the items (who and where you are, for a profile menu). */
export function Menu({trigger, items, onSelect, align = 'end', label, header}: {trigger: React.ReactNode; items: readonly MenuItem[]; onSelect: (id: string) => void; align?: 'start' | 'end' | 'center'; label?: string; header?: React.ReactNode}) {
  let lastGroup: string | undefined;
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className={s.menu} align={align} sideOffset={6} collisionPadding={12} aria-label={label}>
          {header ? <><DropdownMenu.Label className={s.menuHeader}>{header}</DropdownMenu.Label><DropdownMenu.Separator className={s.separator} /></> : null}
          {items.map(item => {
            const groupChanged = item.group !== lastGroup;
            lastGroup = item.group;
            return (
              <React.Fragment key={item.id}>
                {(item.separatorBefore || (groupChanged && item.group)) ? <DropdownMenu.Separator className={s.separator} /> : null}
                {groupChanged && item.group ? <DropdownMenu.Label className={s.menuLabel}>{item.group}</DropdownMenu.Label> : null}
                <DropdownMenu.Item className={cx(s.menuItem, item.destructive && s.destructive)} disabled={item.disabled} onSelect={() => onSelect(item.id)}>
                  {item.icon ? <Icon name={item.icon} size={17} className={s.menuIcon} /> : null}
                  <span>{item.label}</span>
                  {item.meta ? <span className={s.menuMeta}>{item.meta}</span> : null}
                  {item.selected ? <Icon name="check" size={16} className={s.check} /> : null}
                </DropdownMenu.Item>
              </React.Fragment>
            );
          })}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function Popover({trigger, children, open, onOpenChange, align = 'end', side}: {trigger: React.ReactNode; children: React.ReactNode; open?: boolean; onOpenChange?: (o: boolean) => void; align?: 'start' | 'end' | 'center'; side?: 'top' | 'bottom' | 'left' | 'right'}) {
  return (
    <RPopover.Root open={open} onOpenChange={onOpenChange}>
      <RPopover.Trigger asChild>{trigger}</RPopover.Trigger>
      <RPopover.Portal>
        <RPopover.Content className={s.popover} align={align} side={side} sideOffset={8} collisionPadding={12}>
          {children}
        </RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}

export function Tooltip({label, children}: {label: string; children: React.ReactElement}) {
  return (
    <RTooltip.Provider delayDuration={500}>
      <RTooltip.Root>
        <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
        <RTooltip.Portal>
          <RTooltip.Content className={s.tooltip} sideOffset={6}>{label}</RTooltip.Content>
        </RTooltip.Portal>
      </RTooltip.Root>
    </RTooltip.Provider>
  );
}

/**
 * A menu that opens from an anchor rectangle rather than a trigger element,
 * for cards whose More control, right-click or long press opens shared
 * actions: same material and rows as Menu, positioned by the anchor.
 */
// WEB-SYS-04: remember whether the last input was a key, so a menu opened from the keyboard
// takes focus (and one opened by pointer doesn't steal it).
let keyboardInput = false;
if (typeof window !== 'undefined') {
  window.addEventListener('keydown', () => { keyboardInput = true; }, true);
  window.addEventListener('pointerdown', () => { keyboardInput = false; }, true);
}
const menuItems = (root: HTMLElement) => [...root.querySelectorAll<HTMLElement>('[role=menuitem],[role=menuitemradio],[role=menuitemcheckbox]')].filter(el => !(el as HTMLButtonElement).disabled);

export function AnchoredMenu({anchor, open, onOpenChange, children, label, width = 280}: {anchor?: MenuAnchor; open: boolean; onOpenChange: (o: boolean) => void; children: React.ReactNode; label?: string; width?: number}) {
  const point = !!anchor && anchor.width === 0 && anchor.height === 0;
  const opener = React.useRef<HTMLElement | null>(null);
  React.useLayoutEffect(() => { if (open && document.activeElement instanceof HTMLElement && document.activeElement !== document.body) opener.current = document.activeElement; }, [open]);
  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    const items = menuItems(e.currentTarget);
    if (!items.length) return;
    const at = items.indexOf(document.activeElement as HTMLElement);
    const go = (i: number) => { e.preventDefault(); items[(i + items.length) % items.length]!.focus(); };
    if (e.key === 'ArrowDown') go(at + 1);
    else if (e.key === 'ArrowUp') go(at < 0 ? items.length - 1 : at - 1);
    else if (e.key === 'Home') go(0);
    else if (e.key === 'End') go(items.length - 1);
  };
  return (
    <RPopover.Root open={open && !!anchor} onOpenChange={onOpenChange}>
      {anchor ? <RPopover.Anchor asChild><span aria-hidden style={{position: 'fixed', left: anchor.x, top: anchor.y, width: anchor.width, height: anchor.height, pointerEvents: 'none'}} /></RPopover.Anchor> : null}
      <RPopover.Portal>
        <RPopover.Content className={s.menu} role="menu" aria-label={label} align={point ? 'start' : 'end'} side="bottom" sideOffset={point ? 2 : 6} collisionPadding={12} style={{width, maxWidth: 'min(320px, calc(100vw - 24px))', maxHeight: 'var(--radix-popover-content-available-height)', overflowY: 'auto'}}
          onKeyDown={onKeyDown}
          onOpenAutoFocus={e => { e.preventDefault(); if (keyboardInput) { const root = e.currentTarget as HTMLElement; requestAnimationFrame(() => menuItems(root)[0]?.focus()); } }}
          onCloseAutoFocus={e => { if (opener.current?.isConnected) { e.preventDefault(); opener.current.focus(); } }}>
          {children}
        </RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}

/** Title block at the top of an anchored menu: what the actions apply to. */
export function MenuHeading({title, caption}: {title: string; caption?: string}) {
  return (
    <div className={s.menuHeading}>
      <span className={s.menuTitle}>{title}</span>
      {caption ? <span className={s.menuCaption}>{caption}</span> : null}
    </div>
  );
}

/** One row of an anchored menu. */
export function MenuRow({icon, label, meta, selected, disabled, destructive, trailingIcon, onSelect}: {icon?: IconName; label: string; meta?: string; selected?: boolean; disabled?: boolean; destructive?: boolean; trailingIcon?: IconName; onSelect: () => void}) {
  return (
    <button type="button" role="menuitem" className={cx(s.menuItem, s.menuButton, destructive && s.destructive)} disabled={disabled} onClick={onSelect}>
      {icon ? <Icon name={icon} size={17} className={s.menuIcon} /> : null}
      <span>{label}</span>
      {meta ? <span className={s.menuMeta}>{meta}</span> : null}
      {selected ? <Icon name="check" size={16} className={s.check} /> : trailingIcon ? <Icon name={trailingIcon} size={16} className={s.menuTrailing} /> : null}
    </button>
  );
}

export function MenuSeparator() {
  return <div className={s.separator} role="separator" />;
}
