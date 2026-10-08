import React, {useState} from 'react';
import {Dialog as RDialog, DropdownMenu, Popover as RPopover, Tooltip as RTooltip} from 'radix-ui';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';

export function TooltipProvider({children}: {children: React.ReactNode}) {
  return <RTooltip.Provider delayDuration={350} skipDelayDuration={150}>{children}</RTooltip.Provider>;
}

export function Tooltip({label, children, side = 'top'}: {label: React.ReactNode; children: React.ReactElement; side?: 'top' | 'right' | 'bottom' | 'left'}) {
  return (
    <RTooltip.Root>
      <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
      <RTooltip.Portal>
        <RTooltip.Content side={side} sideOffset={8} className="tooltip">{label}</RTooltip.Content>
      </RTooltip.Portal>
    </RTooltip.Root>
  );
}

/**
 * Modal dialog. `size` picks the width; `kind="sheet"` slides in from the right edge (a side panel
 * for longer tasks such as the metadata editor or the player's options).
 */
export function Dialog({open, onOpenChange, title, description, children, footer, size = 'md', kind = 'dialog', hideTitle, className, trigger}: {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  children?: React.ReactNode;
  footer?: React.ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  kind?: 'dialog' | 'sheet';
  hideTitle?: boolean;
  className?: string;
  trigger?: React.ReactElement;
}) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      {trigger ? <RDialog.Trigger asChild>{trigger}</RDialog.Trigger> : null}
      <RDialog.Portal>
        <RDialog.Overlay className={cx('dialog-overlay', kind === 'sheet' && 'is-sheet')} />
        <RDialog.Content className={cx('dialog', `dialog-${size}`, kind === 'sheet' && 'dialog-sheet', className)} aria-describedby={description ? undefined : undefined}>
          <div className="dialog-head">
            <div className="dialog-titles">
              <RDialog.Title className={cx('dialog-title', hideTitle && 'sr-only')}>{title}</RDialog.Title>
              {description ? <RDialog.Description className="dialog-desc">{description}</RDialog.Description> : <RDialog.Description className="sr-only">{typeof title === 'string' ? title : ''}</RDialog.Description>}
            </div>
            <RDialog.Close className="dialog-close" aria-label="Close"><Icon name="close" size={18} /></RDialog.Close>
          </div>
          <div className="dialog-body">{children}</div>
          {footer ? <div className="dialog-foot">{footer}</div> : null}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

export const DialogClose = RDialog.Close;

export type MenuItem =
  | {id: string; label: React.ReactNode; icon?: IconName; danger?: boolean; disabled?: boolean; checked?: boolean; hint?: string; separatorBefore?: boolean; onSelect?: () => void}
  | {id: string; heading: string; separatorBefore?: boolean};

/** A dropdown menu of actions (Radix: roving focus, typeahead, Esc to close). */
export function Menu({trigger, items, onSelect, align = 'end', side = 'bottom', header, label, onOpenChange}: {trigger: React.ReactElement; items: MenuItem[]; onSelect?: (id: string) => void; align?: 'start' | 'center' | 'end'; side?: 'top' | 'bottom' | 'left' | 'right'; header?: React.ReactNode; label?: string; onOpenChange?: (open: boolean) => void}) {
  return (
    <DropdownMenu.Root modal={false} onOpenChange={onOpenChange}>
      <DropdownMenu.Trigger asChild aria-label={label}>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="menu" align={align} side={side} sideOffset={8} collisionPadding={12} loop>
          {header ? <div className="menu-header">{header}</div> : null}
          {items.map(item => (
            <React.Fragment key={item.id}>
              {item.separatorBefore ? <DropdownMenu.Separator className="menu-sep" /> : null}
              {'heading' in item ? (
                <DropdownMenu.Label className="menu-label">{item.heading}</DropdownMenu.Label>
              ) : (
                <DropdownMenu.Item className={cx('menu-item', item.danger && 'is-danger')} disabled={item.disabled} onSelect={() => { item.onSelect?.(); onSelect?.(item.id); }}>
                  {item.icon ? <Icon name={item.icon} size={17} /> : item.checked !== undefined ? <span className="menu-icon-space" /> : null}
                  <span className="menu-item-label">{item.label}</span>
                  {item.hint ? <span className="menu-hint">{item.hint}</span> : null}
                  {item.checked ? <Icon name="check" size={16} className="menu-check" /> : null}
                </DropdownMenu.Item>
              )}
            </React.Fragment>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function Popover({trigger, children, align = 'center', side = 'bottom', open, onOpenChange, className}: {trigger: React.ReactElement; children: React.ReactNode; align?: 'start' | 'center' | 'end'; side?: 'top' | 'bottom' | 'left' | 'right'; open?: boolean; onOpenChange?: (o: boolean) => void; className?: string}) {
  return (
    <RPopover.Root open={open} onOpenChange={onOpenChange}>
      <RPopover.Trigger asChild>{trigger}</RPopover.Trigger>
      <RPopover.Portal>
        <RPopover.Content className={cx('popover', className)} align={align} side={side} sideOffset={8} collisionPadding={12}>{children}</RPopover.Content>
      </RPopover.Portal>
    </RPopover.Root>
  );
}

/** Confirmation for destructive or consequential actions. */
export function ConfirmDialog({open, onOpenChange, title, body, confirmLabel, danger, onConfirm, typeToConfirm, children}: {open: boolean; onOpenChange: (o: boolean) => void; title: string; body?: React.ReactNode; confirmLabel: string; danger?: boolean; onConfirm: () => void | Promise<unknown>; typeToConfirm?: string; children?: React.ReactNode}) {
  const [busy, setBusy] = useState(false);
  const [typed, setTyped] = useState('');
  const ok = !typeToConfirm || typed.trim() === typeToConfirm;
  return (
    <Dialog open={open} onOpenChange={o => { if (!o) setTyped(''); onOpenChange(o); }} title={title} size="sm" footer={
      <>
        <DialogClose asChild><button type="button" className="btn btn-ghost btn-md"><span className="btn-label">Cancel</span></button></DialogClose>
        <button type="button" className={cx('btn btn-md', danger ? 'btn-danger' : 'btn-primary', busy && 'is-busy')} disabled={!ok || busy} onClick={async () => { setBusy(true); try { await onConfirm(); onOpenChange(false); setTyped(''); } finally { setBusy(false); } }}>
          {busy ? <span className="btn-spinner" aria-hidden /> : null}<span className="btn-label">{confirmLabel}</span>
        </button>
      </>
    }>
      {body ? <div className="confirm-body">{body}</div> : null}
      {children}
      {typeToConfirm ? (
        <label className="field" style={{marginTop: 16}}>
          <span className="field-label">Type <strong>{typeToConfirm}</strong> to confirm</span>
          <input className="input" value={typed} onChange={e => setTyped(e.target.value)} autoComplete="off" />
        </label>
      ) : null}
    </Dialog>
  );
}

/** Asks for one line of text (rename, name a new playlist…). */
export function PromptDialog({open, onOpenChange, title, label, initial = '', confirmLabel, onConfirm, placeholder, help}: {open: boolean; onOpenChange: (o: boolean) => void; title: string; label: string; initial?: string; confirmLabel: string; onConfirm: (value: string) => void | Promise<unknown>; placeholder?: string; help?: string}) {
  const [value, setValue] = useState(initial);
  const [busy, setBusy] = useState(false);
  React.useEffect(() => { if (open) setValue(initial); }, [open, initial]);
  const submit = async () => {
    if (!value.trim()) return;
    setBusy(true);
    try { await onConfirm(value.trim()); onOpenChange(false); } finally { setBusy(false); }
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={title} size="sm" footer={
      <>
        <DialogClose asChild><button type="button" className="btn btn-ghost btn-md"><span className="btn-label">Cancel</span></button></DialogClose>
        <button type="button" className="btn btn-primary btn-md" disabled={!value.trim() || busy} onClick={submit}><span className="btn-label">{confirmLabel}</span></button>
      </>
    }>
      <form onSubmit={e => { e.preventDefault(); void submit(); }}>
        <label className="field">
          <span className="field-label">{label}</span>
          <input className="input" value={value} onChange={e => setValue(e.target.value)} placeholder={placeholder} autoFocus maxLength={120} />
          {help ? <span className="field-help">{help}</span> : null}
        </label>
      </form>
    </Dialog>
  );
}
