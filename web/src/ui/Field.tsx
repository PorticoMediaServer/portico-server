import React, {useId, useState} from 'react';
import {uiI18n} from './i18n';
import {Checkbox as RCheckbox, Switch as RSwitch} from 'radix-ui';
import {cx} from './cx';
import {Icon} from './Icon';
import {IconButton} from './Button';
import s from './Field.module.css';

type FieldShell = {label: string; help?: React.ReactNode; error?: React.ReactNode; optional?: boolean; className?: string; id?: string; hideLabel?: boolean};

function Shell({label, help, error, optional, className, id, hideLabel, children}: FieldShell & {children: React.ReactNode}) {
  return (
    <div className={cx(s.field, className)}>
      <label className={cx(s.label, hideLabel && 'visually-hidden')} htmlFor={id}>
        <span>{label}</span>
        {optional ? <span className={s.optional}>{uiI18n().t('field.optional')}</span> : null}
      </label>
      {children}
      {error ? <span className={s.error} id={id ? id + '-help' : undefined} role="alert"><Icon name="warning" size={14} /> <span>{error}</span></span> : help ? <span className={s.help} id={id ? id + '-help' : undefined}>{help}</span> : null}
    </div>
  );
}

/** Text input with a persistent label, help and inline validation. */
export function Input({label, help, error, optional, className, hideLabel, trailing, mono, code, ...rest}: FieldShell & Omit<React.ComponentPropsWithoutRef<'input'>, 'className'> & {trailing?: React.ReactNode; mono?: boolean; code?: boolean}) {
  const auto = useId();
  const id = rest.id ?? auto;
  return (
    <Shell label={label} help={help} error={error} optional={optional} className={className} id={id} hideLabel={hideLabel}>
      <div className={s.control}>
        <input {...rest} id={id} className={cx(s.input, trailing && s.withTrailing, mono && s.mono, code && s.code)} aria-invalid={error ? true : undefined} aria-describedby={[help || error ? id + '-help' : '', rest['aria-describedby'] ?? ''].filter(Boolean).join(' ') || undefined} />
        {trailing ? <span className={s.trailing}>{trailing}</span> : null}
      </div>
    </Shell>
  );
}

export function PasswordInput(props: FieldShell & Omit<React.ComponentPropsWithoutRef<'input'>, 'className' | 'type'>) {
  const [shown, setShown] = useState(false);
  return <Input {...props} type={shown ? 'text' : 'password'} trailing={<IconButton name={shown ? 'eyeOff' : 'eye'} label={shown ? uiI18n().t('field.hidePassword') : uiI18n().t('field.showPassword')} variant="ghost" size="sm" aria-pressed={shown} onClick={() => setShown(v => !v)} />} />;
}

export function TextArea({label, help, error, optional, className, hideLabel, ...rest}: FieldShell & Omit<React.ComponentPropsWithoutRef<'textarea'>, 'className'>) {
  const auto = useId();
  const id = rest.id ?? auto;
  return (
    <Shell label={label} help={help} error={error} optional={optional} className={className} id={id} hideLabel={hideLabel}>
      <textarea {...rest} id={id} className={cx(s.input, s.textarea)} aria-invalid={error ? true : undefined} />
    </Shell>
  );
}

export function Select({label, help, error, optional, className, hideLabel, options, placeholder, ...rest}: FieldShell & Omit<React.ComponentPropsWithoutRef<'select'>, 'className'> & {options: readonly {value: string; label: string; disabled?: boolean}[]; placeholder?: string}) {
  const auto = useId();
  const id = rest.id ?? auto;
  return (
    <Shell label={label} help={help} error={error} optional={optional} className={className} id={id} hideLabel={hideLabel}>
      <div className={s.control}>
        <select {...rest} id={id} className={cx(s.input, s.select)} aria-invalid={error ? true : undefined}>
          {placeholder ? <option value="" disabled>{placeholder}</option> : null}
          {options.map(o => <option key={o.value} value={o.value} disabled={o.disabled}>{o.label}</option>)}
        </select>
        <Icon name="chevronDown" size={16} className={s.selectIcon} />
      </div>
    </Shell>
  );
}

export function Switch({checked, onCheckedChange, disabled, label}: {checked: boolean; onCheckedChange: (v: boolean) => void; disabled?: boolean; label: string}) {
  return (
    <RSwitch.Root className={s.switchRoot} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} aria-label={label}>
      <RSwitch.Thumb className={s.switchThumb} />
    </RSwitch.Root>
  );
}

export function Checkbox({checked, onCheckedChange, label, help, disabled, radio}: {checked: boolean; onCheckedChange: (v: boolean) => void; label: React.ReactNode; help?: React.ReactNode; disabled?: boolean; radio?: boolean}) {
  const id = useId();
  return (
    <label className={s.checkRow} htmlFor={id}>
      <RCheckbox.Root id={id} className={cx(s.checkBox, radio && s.radio)} checked={checked} onCheckedChange={v => onCheckedChange(v === true)} disabled={disabled}>
        <RCheckbox.Indicator>{radio ? null : <Icon name="check" size={14} strokeWidth={2.5} />}</RCheckbox.Indicator>
      </RCheckbox.Root>
      <span className={s.checkCopy}>
        <span>{label}</span>
        {help ? <span className={s.checkHelp}>{help}</span> : null}
      </span>
    </label>
  );
}
