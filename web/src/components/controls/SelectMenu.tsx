import { ActionConfirmIcon, NavigationExpandIcon } from '#portico-icons';
import { useId, useRef, useState } from 'react';
import { AnchoredOverlay } from '../overlay/OverlayPortal';

export type SelectMenuOption = { id: string; label: string; disabled?: boolean };

export function SelectMenu({ label, value, options, onChange, labelledBy, describedBy }: { label: string; value: string; options: ReadonlyArray<SelectMenuOption>; onChange: (id: string) => void; labelledBy?: string; describedBy?: string }) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const identifier = useId();
  const triggerId = `${identifier}-trigger`;
  const listboxId = `${identifier}-listbox`;
  const selected = options.find((option) => option.id === value);
  return (
    <div className="select-menu">
      <button id={triggerId} ref={triggerRef} type="button" className="select-trigger" onClick={() => setOpen(!open)} aria-label={labelledBy ? undefined : label} aria-labelledby={labelledBy} aria-describedby={describedBy} aria-expanded={open} aria-haspopup="listbox" aria-controls={open ? listboxId : undefined}>
        <span>{label && <small>{label}</small>}{selected?.label ?? value}</span><NavigationExpandIcon />
      </button>
      {open && <AnchoredOverlay id={listboxId} labelledBy={triggerId} returnFocusRef={triggerRef} anchorRef={triggerRef} minAnchorWidth className="select-popover" role="listbox" onDismiss={() => setOpen(false)}>
        {options.map((option) => <button type="button" role="option" aria-selected={value === option.id} disabled={option.disabled} key={option.id} onClick={() => { onChange(option.id); setOpen(false); triggerRef.current?.focus(); }} className={value === option.id ? 'chosen' : ''}>{option.label}{value === option.id && <ActionConfirmIcon />}</button>)}
      </AnchoredOverlay>}
    </div>
  );
}
