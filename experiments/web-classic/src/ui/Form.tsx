import React, {useId, useRef} from 'react';
import {Checkbox as RCheckbox, RadioGroup as RRadio, Slider as RSlider, Switch as RSwitch, ToggleGroup} from 'radix-ui';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';
import s from './Form.module.css';

export function Field({label, help, error, children, id, className}: {label?: React.ReactNode; help?: React.ReactNode; error?: React.ReactNode; children: React.ReactNode; id?: string; className?: string}) {
  return (
    <div className={cx(s.field, className)}>
      {label ? <label className={s.label} htmlFor={id}>{label}</label> : null}
      {children}
      {error ? <div className={s.error} id={id ? `${id}-error` : undefined} role="alert"><Icon name="alert" size={15} style={{marginTop: 1}} />{error}</div> : help ? <div className={s.help} id={id ? `${id}-help` : undefined}>{help}</div> : null}
    </div>
  );
}

type TextFieldProps = {label?: React.ReactNode; help?: React.ReactNode; error?: React.ReactNode; value: string; onChange: (v: string) => void; icon?: IconName; trailing?: React.ReactNode; multiline?: boolean; compact?: boolean; className?: string} & Omit<React.InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>;

export const TextField = React.forwardRef<HTMLInputElement, TextFieldProps>(function TextField({label, help, error, value, onChange, icon, trailing, multiline, compact, className, id: idProp, ...rest}, ref) {
  const auto = useId();
  const id = idProp ?? auto;
  const described = error ? `${id}-error` : help ? `${id}-help` : undefined;
  const control = multiline
    ? <textarea id={id} className={s.control} value={value} onChange={e => onChange(e.target.value)} aria-invalid={error ? true : undefined} aria-describedby={described} {...(rest as React.TextareaHTMLAttributes<HTMLTextAreaElement>)} />
    : <input ref={ref} id={id} className={cx(s.control, compact && s.compact)} value={value} onChange={e => onChange(e.target.value)} aria-invalid={error ? true : undefined} aria-describedby={described} {...rest} />;
  return (
    <Field label={label} help={help} error={error} id={id} className={className}>
      {icon || trailing ? <div className={s.inputWrap}>{icon ? <Icon name={icon} size={18} className={s.inputIcon} /> : null}{control}{trailing ? <span className={s.inputTrail}>{trailing}</span> : null}</div> : control}
    </Field>
  );
});

export type Option<V extends string = string> = {value: V; label: string; help?: string; disabled?: boolean};

export function Select<V extends string>({label, help, value, onChange, options, compact, className, id: idProp, ariaLabel}: {label?: React.ReactNode; help?: React.ReactNode; value: V; onChange: (v: V) => void; options: readonly Option<V>[]; compact?: boolean; className?: string; id?: string; ariaLabel?: string}) {
  const auto = useId();
  const id = idProp ?? auto;
  const control = (
    <select id={id} className={cx(s.control, s.select, compact && s.compact)} value={value} onChange={e => onChange(e.target.value as V)} aria-label={label ? undefined : ariaLabel}>
      {options.map(o => <option key={o.value} value={o.value} disabled={o.disabled}>{o.label}</option>)}
    </select>
  );
  if (!label && !help) return <span className={className} style={{display: 'block'}}>{control}</span>;
  return <Field label={label} help={help} id={id} className={className}>{control}</Field>;
}

export function Switch({checked, onChange, label, disabled, id}: {checked: boolean; onChange: (v: boolean) => void; label?: string; disabled?: boolean; id?: string}) {
  return (
    <RSwitch.Root id={id} className={s.switch} checked={checked} onCheckedChange={onChange} disabled={disabled} aria-label={label}>
      <RSwitch.Thumb className={s.thumb} />
    </RSwitch.Root>
  );
}

export function Checkbox({checked, onChange, label, help, disabled}: {checked: boolean; onChange: (v: boolean) => void; label: React.ReactNode; help?: React.ReactNode; disabled?: boolean}) {
  const id = useId();
  return (
    <label className={s.checkRow} htmlFor={id}>
      <RCheckbox.Root id={id} className={s.checkbox} checked={checked} onCheckedChange={v => onChange(v === true)} disabled={disabled}>
        <RCheckbox.Indicator><Icon name="check" size={14} strokeWidth={2.6} /></RCheckbox.Indicator>
      </RCheckbox.Root>
      <span><span className="t-body">{label}</span>{help ? <span className="t-caption c-3" style={{display: 'block', marginTop: 2}}>{help}</span> : null}</span>
    </label>
  );
}

/** A small set of mutually exclusive choices shown together (view modes, periods). */
export function Segmented<V extends string>({value, onChange, options, label, className}: {value: V; onChange: (v: V) => void; options: readonly {value: V; label: string; icon?: IconName}[]; label: string; className?: string}) {
  return (
    <ToggleGroup.Root type="single" className={cx(s.segmented, className)} value={value} onValueChange={v => v && onChange(v as V)} aria-label={label}>
      {options.map(o => (
        <ToggleGroup.Item key={o.value} value={o.value} className={s.segment} aria-label={o.icon && !o.label ? o.value : undefined}>
          {o.icon ? <Icon name={o.icon} size={16} /> : null}{o.label}
        </ToggleGroup.Item>
      ))}
    </ToggleGroup.Root>
  );
}

export function Slider({value, onChange, min = 0, max = 100, step = 1, label, onCommit}: {value: number; onChange: (v: number) => void; min?: number; max?: number; step?: number; label: string; onCommit?: (v: number) => void}) {
  return (
    <RSlider.Root className={s.slider} value={[value]} min={min} max={max} step={step} onValueChange={v => onChange(v[0]!)} onValueCommit={v => onCommit?.(v[0]!)} aria-label={label}>
      <RSlider.Track className={s.sliderTrack}><RSlider.Range className={s.sliderRange} /></RSlider.Track>
      <RSlider.Thumb className={s.sliderThumb} aria-label={label} />
    </RSlider.Root>
  );
}

export function RadioCards<V extends string>({value, onChange, options, label}: {value: V; onChange: (v: V) => void; options: readonly Option<V>[]; label: string}) {
  return (
    <RRadio.Root className={s.radios} value={value} onValueChange={v => onChange(v as V)} aria-label={label}>
      {options.map(o => (
        <RRadio.Item key={o.value} value={o.value} className={s.radio} disabled={o.disabled}>
          <span className={s.radioDot} aria-hidden />
          <span className={s.radioText}><span className={s.radioTitle}>{o.label}</span>{o.help ? <span className={s.radioHelp}>{o.help}</span> : null}</span>
        </RRadio.Item>
      ))}
    </RRadio.Root>
  );
}

/** Four (or six) single-digit boxes that behave as one field: typing advances, Backspace goes back, paste fills. */
export function PinInput({length = 4, value, onChange, label, secret = true, autoFocus, onComplete}: {length?: number; value: string; onChange: (v: string) => void; label: string; secret?: boolean; autoFocus?: boolean; onComplete?: (v: string) => void}) {
  const refs = useRef<(HTMLInputElement | null)[]>([]);
  const set = (v: string) => {
    const clean = v.replace(/\D/g, '').slice(0, length);
    onChange(clean);
    if (clean.length === length) onComplete?.(clean);
    refs.current[Math.min(clean.length, length - 1)]?.focus();
  };
  return (
    <div className={s.pin} role="group" aria-label={label}>
      {Array.from({length}, (_, i) => (
        <input
          key={i}
          ref={el => { refs.current[i] = el; }}
          className={s.pinBox}
          inputMode="numeric"
          type={secret ? 'password' : 'text'}
          autoComplete="one-time-code"
          aria-label={`${label}, digit ${i + 1}`}
          autoFocus={autoFocus && i === 0}
          value={value[i] ?? ''}
          onChange={e => { const d = e.target.value.replace(/\D/g, '').slice(-1); if (!d) return; set((value.slice(0, i) + d).slice(0, length)); }}
          onKeyDown={e => { if (e.key === 'Backspace') { e.preventDefault(); set(value.slice(0, Math.max(0, value[i] ? i : i - 1))); } }}
          onPaste={e => { e.preventDefault(); set(e.clipboardData.getData('text')); }}
          onFocus={e => e.target.select()}
        />
      ))}
    </div>
  );
}
