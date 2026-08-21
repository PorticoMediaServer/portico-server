import { forwardRef, type MouseEvent, type ReactNode } from 'react';

export const IconButton = forwardRef<HTMLButtonElement, { label: string; children: ReactNode; onClick?: (event: MouseEvent<HTMLButtonElement>) => void; className?: string; disabled?: boolean }>(
  function IconButton({ label, children, onClick, className = '', disabled = false }, ref) {
    return <button ref={ref} type="button" className={`icon-button ${className}`} onClick={onClick} aria-label={label} title={label} disabled={disabled}>{children}</button>;
  },
);

export function PrimaryButton({ children, onClick, type = 'button', disabled = false }: { children: ReactNode; onClick?: () => void; type?: 'button' | 'submit'; disabled?: boolean }) {
  return <button type={type} className="button primary" onClick={onClick} disabled={disabled}>{children}</button>;
}

export function SecondaryButton({ children, onClick, selected = false, disabled = false }: { children: ReactNode; onClick?: () => void; selected?: boolean; disabled?: boolean }) {
  return <button type="button" className={`button secondary ${selected ? 'selected' : ''}`} onClick={onClick} disabled={disabled}>{children}</button>;
}
