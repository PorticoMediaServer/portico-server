import React, {useLayoutEffect, useRef} from 'react';
import {uiI18n} from './i18n';
import {Dialog as RDialog} from 'radix-ui';
import {cx} from './cx';
import {IconButton} from './Button';
import {useCompact} from './viewport';
import s from './Overlay.module.css';

/**
 * Modal dialog. Centered panel (or a right-side panel) on wide viewports,
 * bottom sheet on phones.
 * Focus is trapped, Escape and the Close action dismiss, and focus returns
 * to the invoker. Pass `dismissable={false}` for required acknowledgements.
 */
export function Dialog({open, onOpenChange, title, description, children, actions, width, dismissable = true, spread, placement = 'center'}: {open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: React.ReactNode; children?: React.ReactNode; actions?: React.ReactNode; width?: number; dismissable?: boolean; spread?: boolean; /** `side`: a full-height panel on the right (e.g. the episode panel); phones still get a bottom sheet. */ placement?: 'center' | 'side'}) {
  const compact = useCompact();
  // WEB-SYS-03: focus goes back to whatever opened the dialog (Radix only
  // restores it for its own Trigger, and ours are controlled).
  const invoker = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    const active = document.activeElement;
    if (!open || !(active instanceof HTMLElement) || active === document.body) return;
    // Opened from a menu item, which unmounts with its menu: return to the menu's trigger instead.
    const menu = active.closest('[role=menu]');
    const trigger = menu?.id ? document.querySelector<HTMLElement>(`[aria-controls="${CSS.escape(menu.id)}"]`) : null;
    invoker.current = trigger ?? active;
  }, [open]);
  return (
    <RDialog.Root open={open} onOpenChange={dismissable ? onOpenChange : o => o && onOpenChange(o)}>
      <RDialog.Portal>
        <RDialog.Overlay className={s.overlay} />
        <RDialog.Content className={cx(s.dialog, compact ? s.sheet : placement === 'side' && s.side)} style={width ? ({'--dialog-width': `${width}px`} as React.CSSProperties) : undefined} onPointerDownOutside={dismissable ? undefined : e => e.preventDefault()} onEscapeKeyDown={dismissable ? undefined : e => e.preventDefault()} onCloseAutoFocus={e => { const target = invoker.current; if (target?.isConnected) { e.preventDefault(); target.focus(); } invoker.current = null; }}>
          {compact ? <div className={s.grabber} aria-hidden /> : null}
          <div className={s.head}>
            <div className={s.headCopy}>
              <RDialog.Title className={s.title}>{title}</RDialog.Title>
              {description ? <RDialog.Description className={s.description}>{description}</RDialog.Description> : <RDialog.Description className="visually-hidden">{title}</RDialog.Description>}
            </div>
            {dismissable ? <RDialog.Close asChild><IconButton name="close" label={uiI18n().t('action.close')} variant="ghost" className={s.close} /></RDialog.Close> : null}
          </div>
          {children ? <div className={s.body}>{children}</div> : null}
          {actions ? <div className={cx(s.actions, spread && s.spread)}>{actions}</div> : null}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
