import React, {createContext, useCallback, useContext, useRef, useState} from 'react';
import {ContextMenu as RContext, Dialog as RDialog, DropdownMenu as RMenu, Popover as RPopover, Tooltip as RTooltip} from 'radix-ui';
import {Button, IconButton, type ButtonVariant} from './Button';
import {Icon, type IconName} from './Icon';
import {TextField} from './Form';
import {cx} from './cx';
import s from './Overlay.module.css';

// ── Dialog ──────────────────────────────────────────────────────────────
export type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  children?: React.ReactNode;
  footer?: React.ReactNode;
  footerSplit?: boolean;
  width?: number;
  flush?: boolean;
  /** A side panel instead of a centred dialog (queues, filters). */
  side?: 'right' | 'left';
  initialFocus?: React.RefObject<HTMLElement | null>;
  className?: string;
};

/**
 * Modal dialog. Centred on wide screens, a bottom sheet on phones. Radix
 * traps focus, restores it to the trigger on close and closes on Escape.
 */
export function Dialog({open, onOpenChange, title, description, children, footer, footerSplit, width, flush, side, initialFocus, className}: DialogProps) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className={s.backdrop} />
        <RDialog.Content
          className={cx(side ? cx(s.panel, side === 'left' && s.panelLeft) : s.dialog, className)}
          style={{'--dialog-w': width ? `${width}px` : undefined} as React.CSSProperties}
          aria-describedby={description ? undefined : undefined}
          onOpenAutoFocus={e => {
            // Focus the field meant to be first (autoFocus), else the dialog itself, not the close button.
            e.preventDefault();
            const root = e.currentTarget as HTMLElement;
            const target = initialFocus?.current ?? root.querySelector<HTMLElement>('[autofocus], [data-autofocus]');
            (target ?? root).focus({preventScroll: true});
          }}
          tabIndex={-1}
        >
          <div className={s.head}>
            <div className={s.headText}>
              <RDialog.Title className={s.title}>{title}</RDialog.Title>
              {description ? <RDialog.Description className={s.description}>{description}</RDialog.Description> : <RDialog.Description className="sr-only">{typeof title === 'string' ? title : ''}</RDialog.Description>}
            </div>
            <RDialog.Close asChild><IconButton icon="close" label="Close" variant="ghost" className={s.close} tooltip={false} /></RDialog.Close>
          </div>
          <div className={cx(s.body, flush && s.bodyFlush)}>{children}</div>
          {footer ? <div className={cx(s.foot, footerSplit && s.footSplit)}>{footer}</div> : null}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

/** A yes/no question. `typeToConfirm` makes the viewer type a word first, for irreversible actions. */
export function ConfirmDialog({open, onOpenChange, title, body, action, cancel = 'Cancel', tone = 'primary', typeToConfirm, busy, onConfirm, children}: {open: boolean; onOpenChange: (o: boolean) => void; title: string; body?: React.ReactNode; action: string; cancel?: string; tone?: 'primary' | 'danger'; typeToConfirm?: string; busy?: boolean; onConfirm: () => void | Promise<unknown>; children?: React.ReactNode}) {
  const [typed, setTyped] = useState('');
  const [working, setWorking] = useState(false);
  const ready = !typeToConfirm || typed.trim().toLowerCase() === typeToConfirm.toLowerCase();
  const variant: ButtonVariant = tone === 'danger' ? 'dangerSolid' : 'primary';
  return (
    <Dialog open={open} onOpenChange={o => { if (!o) setTyped(''); onOpenChange(o); }} title={title} width={460} footer={<>
      <Button variant="ghost" label={cancel} onClick={() => onOpenChange(false)} />
      <Button variant={variant} label={action} busy={busy || working} disabled={!ready} onClick={async () => { setWorking(true); try { await onConfirm(); } finally { setWorking(false); setTyped(''); } }} />
    </>}>
      {body ? <div className="t-body c-2">{body}</div> : null}
      {children}
      {typeToConfirm ? <div style={{marginTop: 16}}><TextField label={`Type ${typeToConfirm} to confirm`} value={typed} onChange={setTyped} autoFocus autoComplete="off" /></div> : null}
    </Dialog>
  );
}

// ── Menus ───────────────────────────────────────────────────────────────
export type MenuItem =
  | {type?: 'item'; id: string; label: string; icon?: IconName; danger?: boolean; disabled?: boolean; hint?: string; checked?: boolean; separatorBefore?: boolean; onSelect?: () => void}
  | {type: 'label'; id: string; label: string; separatorBefore?: boolean}
  | {type: 'sub'; id: string; label: string; icon?: IconName; items: MenuItem[]; separatorBefore?: boolean};

type Parts = typeof RMenu | typeof RContext;
function renderItems(P: Parts, items: MenuItem[], onSelect?: (id: string) => void, checkable?: boolean): React.ReactNode {
  return items.map(item => (
    <React.Fragment key={item.id}>
      {item.separatorBefore ? <P.Separator className={s.separator} /> : null}
      {item.type === 'label' ? <P.Label className={s.menuLabel}>{item.label}</P.Label>
        : item.type === 'sub' ? (
          <P.Sub>
            <P.SubTrigger className={s.item}>{item.icon ? <Icon name={item.icon} size={18} /> : null}<span className={s.itemLabel}>{item.label}</span><Icon name="forward" size={14} /></P.SubTrigger>
            <P.Portal><P.SubContent className={s.menu} sideOffset={4} collisionPadding={8}>{renderItems(P, item.items, onSelect, item.items.some(i => i.type !== 'label' && i.type !== 'sub' && i.checked !== undefined))}</P.SubContent></P.Portal>
          </P.Sub>
        ) : (
          <P.Item className={cx(s.item, item.danger && s.itemDanger)} disabled={item.disabled} onSelect={() => { item.onSelect?.(); onSelect?.(item.id); }}>
            {checkable ? <span className={s.check}>{item.checked ? <Icon name="check" size={16} /> : null}</span> : item.icon ? <Icon name={item.icon} size={18} /> : null}
            <span className={s.itemLabel}>{item.label}</span>
            {item.hint ? <span className={s.itemHint}>{item.hint}</span> : null}
          </P.Item>
        )}
    </React.Fragment>
  ));
}

/** A dropdown menu from a list of items. */
export function Menu({trigger, items, onSelect, header, align = 'end', side = 'bottom', label, modal, open, onOpenChange}: {trigger: React.ReactElement; items: MenuItem[]; onSelect?: (id: string) => void; header?: React.ReactNode; align?: 'start' | 'center' | 'end'; side?: 'top' | 'bottom' | 'left' | 'right'; label?: string; modal?: boolean; open?: boolean; onOpenChange?: (o: boolean) => void}) {
  const checkable = items.some(i => (i.type ?? 'item') === 'item' && (i as {checked?: boolean}).checked !== undefined);
  return (
    <RMenu.Root modal={modal} open={open} onOpenChange={onOpenChange}>
      <RMenu.Trigger asChild>{trigger}</RMenu.Trigger>
      <RMenu.Portal>
        <RMenu.Content className={s.menu} align={align} side={side} sideOffset={6} collisionPadding={12} aria-label={label}>
          {header ? <div className={s.menuHeader}>{header}</div> : null}
          {renderItems(RMenu, items, onSelect, checkable)}
        </RMenu.Content>
      </RMenu.Portal>
    </RMenu.Root>
  );
}

/** Right-click (and long-press) menu around any content. */
export function ContextMenu({children, items, onSelect, disabled}: {children: React.ReactElement; items: () => MenuItem[]; onSelect?: (id: string) => void; disabled?: boolean}) {
  const [list, setList] = useState<MenuItem[]>([]);
  if (disabled) return children;
  return (
    <RContext.Root onOpenChange={o => { if (o) setList(items()); }}>
      <RContext.Trigger asChild>{children}</RContext.Trigger>
      <RContext.Portal>
        <RContext.Content className={s.menu} collisionPadding={12}>{renderItems(RContext, list, onSelect)}</RContext.Content>
      </RContext.Portal>
    </RContext.Root>
  );
}

// ── Tooltip and popover ─────────────────────────────────────────────────
export function Tooltip({label, children, side = 'top', disabled}: {label: React.ReactNode; children: React.ReactElement; side?: 'top' | 'right' | 'bottom' | 'left'; disabled?: boolean}) {
  if (disabled) return children;
  return (
    <RTooltip.Root>
      <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
      <RTooltip.Portal><RTooltip.Content className={s.tooltip} side={side} sideOffset={8} collisionPadding={8}>{label}</RTooltip.Content></RTooltip.Portal>
    </RTooltip.Root>
  );
}
export const TooltipProvider = RTooltip.Provider;

export function Popover({trigger, children, open, onOpenChange, align = 'center', side = 'bottom', width, label}: {trigger: React.ReactElement; children: React.ReactNode; open?: boolean; onOpenChange?: (o: boolean) => void; align?: 'start' | 'center' | 'end'; side?: 'top' | 'bottom' | 'left' | 'right'; width?: number; label?: string}) {
  return (
    <RPopover.Root open={open} onOpenChange={onOpenChange}>
      <RPopover.Trigger asChild>{trigger}</RPopover.Trigger>
      <RPopover.Portal>
        <RPopover.Content className={s.popover} align={align} side={side} sideOffset={8} collisionPadding={12} style={{width}} aria-label={label}>{children}</RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}

// ── Toasts ──────────────────────────────────────────────────────────────
export type Toast = {id: number; text: string; icon?: IconName; action?: {label: string; run: () => void}; tone?: 'error'};
type ToastApi = {show: (text: string, opts?: {icon?: IconName; action?: {label: string; run: () => void}; tone?: 'error'; duration?: number}) => void};
const ToastContext = createContext<ToastApi>({show: () => {}});
export const useToast = () => useContext(ToastContext);

/** Short confirmations with an optional Undo. Announced politely to screen readers. */
export function ToastProvider({children}: {children: React.ReactNode}) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const next = useRef(1);
  const show = useCallback<ToastApi['show']>((text, opts = {}) => {
    const id = next.current++;
    setToasts(list => [...list.slice(-2), {id, text, icon: opts.icon, action: opts.action, tone: opts.tone}]);
    setTimeout(() => setToasts(list => list.filter(t => t.id !== id)), opts.duration ?? (opts.action ? 6000 : 3600));
  }, []);
  const api = React.useMemo(() => ({show}), [show]);
  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className={s.toasts} role="status" aria-live="polite">
        {toasts.map(t => (
          <div key={t.id} className={cx(s.toast, t.tone === 'error' && s.toastError)}>
            {t.icon ? <Icon name={t.icon} size={18} /> : null}
            <span className={s.toastText}>{t.text}</span>
            {t.action ? <button type="button" className={s.toastAction} onClick={() => { t.action!.run(); setToasts(list => list.filter(x => x.id !== t.id)); }}>{t.action.label}</button> : null}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
