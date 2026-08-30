import { ActionConfirmIcon, NavigationExpandIcon } from '#portico-icons';
import { useRef, useState } from 'react';
import { AnchoredOverlay } from '../../components/overlay/OverlayPortal';

export type LiveChoice = {
  id: string;
  label: string;
  detail?: string;
};

export function LiveChoiceMenu({ label, value, choices, onChange, className = '' }: { label: string; value: string; choices: LiveChoice[]; onChange: (value: string) => void; className?: string }) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const selected = choices.find((choice) => choice.id === value) ?? choices[0];
  return <div className={`live-choice ${className}`}>
    <button ref={triggerRef} type="button" className="live-choice-trigger" aria-expanded={open} aria-haspopup="listbox" onClick={() => setOpen((current) => !current)}>
      <span><span>{label}</span><strong>{selected?.label ?? value}</strong></span><NavigationExpandIcon />
    </button>
    {open && <AnchoredOverlay anchorRef={triggerRef} minAnchorWidth className="live-choice-popover" role="listbox" onDismiss={() => setOpen(false)}>
      {choices.map((choice) => <button key={choice.id} type="button" role="option" aria-selected={choice.id === value} className={choice.id === value ? 'chosen' : ''} onClick={() => { onChange(choice.id); setOpen(false); triggerRef.current?.focus(); }}><span><strong>{choice.label}</strong>{choice.detail && <span>{choice.detail}</span>}</span>{choice.id === value && <ActionConfirmIcon />}</button>)}
    </AnchoredOverlay>}
  </div>;
}
