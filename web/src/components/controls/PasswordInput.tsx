import { ActionConfirmIcon, AccountVisibilityShowIcon, AccountVisibilityHideIcon } from '#portico-icons';
import { forwardRef, useId, useState, type InputHTMLAttributes } from 'react';
import { productText } from '../ProductLanguage';

export const passwordRequirements = [
  { id: 'length', labelId: 'auth.password.requirement.minimum-length' as const, test: (value: string) => value.length >= 8 },
  { id: 'uppercase', labelId: 'auth.password.requirement.uppercase' as const, test: (value: string) => /[A-Z]/.test(value) },
  { id: 'lowercase', labelId: 'auth.password.requirement.lowercase' as const, test: (value: string) => /[a-z]/.test(value) },
  { id: 'number-special', labelId: 'auth.password.requirement.number-or-special' as const, test: (value: string) => /[^A-Za-z]/.test(value) },
] as const;

export function validPorticoPassword(value: string): boolean {
  return passwordRequirements.every((requirement) => requirement.test(value));
}

export type PasswordStrength = 'Weak' | 'Medium' | 'Strong';

export function porticoPasswordStrength(value: string): PasswordStrength {
  if (!validPorticoPassword(value)) return 'Weak';
  const hasNumber = /\d/.test(value);
  const hasSpecial = /[^A-Za-z0-9]/.test(value);
  return value.length >= 16 || (value.length >= 12 && hasNumber && hasSpecial)
    ? 'Strong'
    : 'Medium';
}

function passwordStrengthLabel(strength: PasswordStrength): string {
  if (strength === 'Strong') return productText('auth.password.strength.strong');
  if (strength === 'Medium') return productText('auth.password.strength.medium');
  return productText('auth.password.strength.weak');
}

export const PasswordInput = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function PasswordInput({ className, id, ...props }, ref) {
    const generatedId = useId();
    const inputId = id ?? generatedId;
    const [visible, setVisible] = useState(false);
    const label = typeof props['aria-label'] === 'string' && props['aria-label'].trim()
      ? props['aria-label']
      : 'Password';
    return <span className={`portico-password-input${className ? ` ${className}` : ''}`}>
      <input {...props} id={inputId} ref={ref} type={visible ? 'text' : 'password'} />
      <button
        type="button"
        className="portico-password-visibility"
        aria-controls={inputId}
        aria-label={`${productText(visible ? 'auth.password.hide' : 'auth.password.show')}: ${label}`}
        aria-pressed={visible}
        disabled={props.disabled}
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          setVisible((current) => !current);
        }}
      >
        {visible ? <AccountVisibilityHideIcon aria-hidden="true" /> : <AccountVisibilityShowIcon aria-hidden="true" />}
      </button>
    </span>;
  },
);

export function PasswordRequirements({ value, id }: { value: string; id?: string }) {
  const strength = porticoPasswordStrength(value);
  const strengthLabel = passwordStrengthLabel(strength);
  return <div id={id} className="portico-password-guidance" aria-live="polite">
    <ul className="portico-password-requirements" aria-label="Password requirements">
      {passwordRequirements.map((requirement) => {
        const met = requirement.test(value);
        return <li key={requirement.id} className={met ? 'met' : ''}>
          <ActionConfirmIcon aria-hidden="true" />
          <span>{productText(requirement.labelId)}</span>
          <span className="sr-only">: {met ? 'met' : 'not met'}</span>
        </li>;
      })}
    </ul>
    <p className={`portico-password-strength ${strength.toLowerCase()}`}>Strength: <strong>{strengthLabel}</strong></p>
  </div>;
}
