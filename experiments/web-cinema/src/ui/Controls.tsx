import React, {useId, useState} from 'react';
import {Checkbox as RCheckbox, Slider as RSlider, Switch as RSwitch} from 'radix-ui';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';

export function Field({label, help, error, children, optional, className}: {label: React.ReactNode; help?: React.ReactNode; error?: React.ReactNode; children: React.ReactElement<{id?: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean}>; optional?: boolean; className?: string}) {
  const id = useId();
  const helpId = help || error ? `${id}-help` : undefined;
  return (
    <div className={cx('field', className)}>
      <label className="field-label" htmlFor={id}>{label}{optional ? <span className="field-optional"> (optional)</span> : null}</label>
      {React.cloneElement(children, {id, 'aria-describedby': helpId, 'aria-invalid': error ? true : undefined})}
      {error ? <span id={helpId} className="field-error"><Icon name="alert" size={14} />{error}</span> : help ? <span id={helpId} className="field-help">{help}</span> : null}
    </div>
  );
}

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(function Input({className, ...rest}, ref) {
  return <input ref={ref} className={cx('input', className)} {...rest} />;
});

export function PasswordInput(props: React.InputHTMLAttributes<HTMLInputElement>) {
  const [shown, setShown] = useState(false);
  return (
    <div className="input-wrap">
      <input {...props} type={shown ? 'text' : 'password'} className={cx('input', props.className)} />
      <button type="button" className="input-affix" onClick={() => setShown(s => !s)} aria-label={shown ? 'Hide password' : 'Show password'}><Icon name={shown ? 'eyeOff' : 'eye'} size={17} /></button>
    </div>
  );
}

export function TextArea({className, ...rest}: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cx('input', 'textarea', className)} {...rest} />;
}

export function SearchInput({value, onChange, placeholder, label, autoFocus, className, onKeyDown, size = 'md'}: {value: string; onChange: (v: string) => void; placeholder?: string; label: string; autoFocus?: boolean; className?: string; onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>; size?: 'md' | 'lg'}) {
  return (
    <div className={cx('search-input', `search-input-${size}`, className)}>
      <Icon name="search" size={size === 'lg' ? 24 : 18} />
      <input type="search" value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} aria-label={label} autoFocus={autoFocus} onKeyDown={onKeyDown} spellCheck={false} autoComplete="off" />
      {value ? <button type="button" className="search-clear" onClick={() => onChange('')} aria-label="Clear search"><Icon name="close" size={16} /></button> : null}
    </div>
  );
}

export function Select<T extends string>({value, onChange, options, label, className, id, ...rest}: {value: T; onChange: (v: T) => void; options: readonly {value: T; label: string; disabled?: boolean}[]; label?: string; className?: string; id?: string; 'aria-describedby'?: string}) {
  return (
    <div className={cx('select', className)}>
      <select id={id} value={value} onChange={e => onChange(e.target.value as T)} aria-label={label} {...rest}>
        {options.map(o => <option key={o.value} value={o.value} disabled={o.disabled}>{o.label}</option>)}
      </select>
      <Icon name="chevronDown" size={16} />
    </div>
  );
}

export function Switch({checked, onChange, label, disabled, id}: {checked: boolean; onChange: (v: boolean) => void; label?: string; disabled?: boolean; id?: string}) {
  return (
    <RSwitch.Root id={id} className="switch" checked={checked} onCheckedChange={onChange} disabled={disabled} aria-label={label}>
      <RSwitch.Thumb className="switch-thumb" />
    </RSwitch.Root>
  );
}

export function Checkbox({checked, onChange, label, disabled}: {checked: boolean; onChange: (v: boolean) => void; label: React.ReactNode; disabled?: boolean}) {
  const id = useId();
  return (
    <div className="checkbox-row">
      <RCheckbox.Root id={id} className="checkbox" checked={checked} onCheckedChange={v => onChange(v === true)} disabled={disabled}>
        <RCheckbox.Indicator><Icon name="check" size={14} stroke={3} /></RCheckbox.Indicator>
      </RCheckbox.Root>
      <label htmlFor={id}>{label}</label>
    </div>
  );
}

export function Slider({value, onChange, min = 0, max = 100, step = 1, label, className, onCommit}: {value: number; onChange: (v: number) => void; min?: number; max?: number; step?: number; label: string; className?: string; onCommit?: (v: number) => void}) {
  return (
    <RSlider.Root className={cx('slider', className)} value={[value]} min={min} max={max} step={step} onValueChange={v => onChange(v[0]!)} onValueCommit={v => onCommit?.(v[0]!)} aria-label={label}>
      <RSlider.Track className="slider-track"><RSlider.Range className="slider-range" /></RSlider.Track>
      <RSlider.Thumb className="slider-thumb" aria-label={label} />
    </RSlider.Root>
  );
}

/** A compact set of mutually exclusive choices (views, periods, filters). */
export function Segmented<T extends string>({value, onChange, options, label, size = 'md', className}: {value: T; onChange: (v: T) => void; options: readonly {value: T; label: React.ReactNode; icon?: IconName; count?: number}[]; label: string; size?: 'sm' | 'md'; className?: string}) {
  return (
    <div className={cx('segmented', `segmented-${size}`, className)} role="radiogroup" aria-label={label} onKeyDown={e => {
      const i = options.findIndex(o => o.value === value);
      if (e.key === 'ArrowRight' || e.key === 'ArrowDown') { e.preventDefault(); const n = options[(i + 1) % options.length]!; onChange(n.value); (e.currentTarget.children[(i + 1) % options.length] as HTMLElement)?.focus(); }
      if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') { e.preventDefault(); const j = (i - 1 + options.length) % options.length; onChange(options[j]!.value); (e.currentTarget.children[j] as HTMLElement)?.focus(); }
    }}>
      {options.map(o => (
        <button key={o.value} type="button" role="radio" aria-checked={o.value === value} tabIndex={o.value === value ? 0 : -1} className={cx('segment', o.value === value && 'is-on')} onClick={() => onChange(o.value)}>
          {o.icon ? <Icon name={o.icon} size={16} /> : null}
          {o.label}
          {o.count !== undefined ? <span className="segment-count">{o.count}</span> : null}
        </button>
      ))}
    </div>
  );
}

/** Choice chips that can each be on or off (quick filters). */
export function Chip({on, onClick, children, icon, removable, className}: {on?: boolean; onClick?: () => void; children: React.ReactNode; icon?: IconName; removable?: boolean; className?: string}) {
  return (
    <button type="button" className={cx('chip', on && 'is-on', className)} aria-pressed={on === undefined ? undefined : on} onClick={onClick}>
      {icon ? <Icon name={icon} size={15} /> : null}
      <span>{children}</span>
      {removable ? <Icon name="close" size={14} /> : null}
    </button>
  );
}

/** Large selectable cards for choices that need a sentence each (sign-in mode, library kind). */
export function ChoiceCards<T extends string>({value, onChange, options, label, columns = 2}: {value: T; onChange: (v: T) => void; options: readonly {value: T; title: string; body?: string; icon?: IconName}[]; label: string; columns?: number}) {
  return (
    <div className="choice-cards" role="radiogroup" aria-label={label} style={{'--cols': columns} as React.CSSProperties}>
      {options.map(o => (
        <button key={o.value} type="button" role="radio" aria-checked={o.value === value} className={cx('choice-card', o.value === value && 'is-on')} onClick={() => onChange(o.value)}>
          {o.icon ? <span className="choice-icon"><Icon name={o.icon} size={20} /></span> : null}
          <span className="choice-text"><strong>{o.title}</strong>{o.body ? <span>{o.body}</span> : null}</span>
          <span className="choice-dot" aria-hidden />
        </button>
      ))}
    </div>
  );
}

export function Stepper({value, onChange, min = 0, max = 99, step = 1, label, suffix}: {value: number; onChange: (v: number) => void; min?: number; max?: number; step?: number; label: string; suffix?: string}) {
  return (
    <div className="stepper" role="group" aria-label={label}>
      <button type="button" onClick={() => onChange(Math.max(min, value - step))} disabled={value <= min} aria-label={`Decrease ${label}`}><Icon name="minus" size={16} /></button>
      <span className="stepper-value tabular" aria-live="polite">{value}{suffix ? ` ${suffix}` : ''}</span>
      <button type="button" onClick={() => onChange(Math.min(max, value + step))} disabled={value >= max} aria-label={`Increase ${label}`}><Icon name="plus" size={16} /></button>
    </div>
  );
}
