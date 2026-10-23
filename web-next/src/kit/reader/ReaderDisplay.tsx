import {useId, type RefObject} from 'react';
import {RadioGroup} from 'radix-ui';
import {Icon, type IconName} from '../Icon';
import {Segmented} from '../controls';
import {Popover} from '../overlays/Popover';
import {BottomSheet} from '../overlays/BottomSheet';
import type {OverlayChange} from '../overlays/types';
import styles from './display.module.css';

type DisplayOption = Readonly<{value: string; label: string; disabled?: boolean}>;
type DisplayGroup<Option extends DisplayOption> = Readonly<{
  label: string; value: string; options: readonly Option[]; onChange: (value: string) => void;
}>;
export type ReaderThemeOption = DisplayOption & Readonly<{tone: 'dark' | 'black' | 'sepia' | 'light'}>;
export type ReaderFontOption = DisplayOption & Readonly<{
  face: 'publisher' | 'literata' | 'manrope'; compactLabel: string; description?: string;
}>;
export type ReaderDisplayRow = DisplayGroup<DisplayOption & Readonly<{icon?: IconName}>> & Readonly<{
  id: string; hideOnPhone?: boolean;
}>;
export type ReaderDisplaySize = Readonly<{
  label: string; sliderLabel: string; smallerLabel: string; largerLabel: string;
  value: number; min: number; max: number; step: number; valueText: string; summary?: string;
  /** Tick values and bounds come from the canonical Reading facade. */
  ticks?: readonly number[]; disabled?: boolean; onChange: (value: number) => void;
}>;
export type ReaderDisplayControlsProps = Readonly<{
  themes: DisplayGroup<ReaderThemeOption>; fonts: DisplayGroup<ReaderFontOption>;
  size: ReaderDisplaySize; rows: readonly ReaderDisplayRow[];
  copy: Readonly<{sample: string; sizeSample: string; sessionNote: string; defaultsNote: string; openSettings?: string}>;
  onOpenSettings?: () => void;
}>;

/** Frame-backed controls only: the core supplies values and applies book-local overrides. */
export function ReaderDisplayControls({themes, fonts, size, rows, copy, onOpenSettings}: ReaderDisplayControlsProps) {
  const id = useId();
  const fraction = size.max > size.min ? Math.max(0, Math.min(1, (size.value - size.min) / (size.max - size.min))) : 0;
  const changeSize = (value: number) => {
    if (!size.disabled && Number.isFinite(value)) size.onChange(Math.max(size.min, Math.min(size.max, value)));
  };
  return <div className={styles.controls}>
    <section className={styles.group} aria-labelledby={`${id}-page`}><h3 id={`${id}-page`}>{themes.label}</h3>
      <RadioGroup.Root className={styles.themes} value={themes.value} onValueChange={themes.onChange} aria-labelledby={`${id}-page`} orientation="horizontal">
        {themes.options.map(option => <RadioGroup.Item key={option.value} className={styles.theme} value={option.value} disabled={option.disabled} data-tone={option.tone}>
          <span aria-hidden="true">{copy.sample}</span>{option.label}
        </RadioGroup.Item>)}
      </RadioGroup.Root>
    </section>
    <section className={styles.group} aria-labelledby={`${id}-font`}><h3 id={`${id}-font`}>{fonts.label}</h3>
      <RadioGroup.Root className={styles.fonts} value={fonts.value} onValueChange={fonts.onChange} aria-labelledby={`${id}-font`}>
        {fonts.options.map(option => <RadioGroup.Item key={option.value} className={styles.font} value={option.value} disabled={option.disabled} data-face={option.face}>
          <span className={styles.fontDesktop}>{option.label}{option.description && <small>{option.description}</small>}</span>
          <span className={styles.fontCompact}><span aria-hidden="true">{copy.sample}</span><small>{option.compactLabel}</small></span>
          <Icon className={styles.check} name="i-check" size={18} />
        </RadioGroup.Item>)}
      </RadioGroup.Root>
    </section>
    <section className={styles.group} aria-labelledby={`${id}-size`}><h3 id={`${id}-size`}>{size.label}{size.summary && <span>{size.summary}</span>}</h3>
      <div className={styles.sizer}>
        <button type="button" className={styles.smaller} aria-label={size.smallerLabel} disabled={size.disabled || size.value <= size.min} onClick={() => changeSize(size.value - size.step)}>{copy.sizeSample}</button>
        <div className={styles.range}><span className={styles.track} aria-hidden="true"><i style={{width: `${fraction * 100}%`}} />
          {size.ticks?.filter(value => Number.isFinite(value) && value >= size.min && value <= size.max).map(value => <u key={value} style={{left: `${size.max > size.min ? (value - size.min) / (size.max - size.min) * 100 : 0}%`}} />)}
          <b style={{left: `${fraction * 100}%`}} /></span>
          <input type="range" min={size.min} max={size.max} step={size.step} value={size.value} disabled={size.disabled || size.max <= size.min} aria-label={size.sliderLabel} aria-valuetext={size.valueText} onChange={event => changeSize(event.currentTarget.valueAsNumber)} />
        </div>
        <button type="button" className={styles.larger} aria-label={size.largerLabel} disabled={size.disabled || size.value >= size.max} onClick={() => changeSize(size.value + size.step)}>{copy.sizeSample}</button>
        <span className={styles.value}>{size.valueText}</span>
      </div>
    </section>
    <div className={`${styles.group} ${styles.rows}`}>{rows.map(row => <div key={row.id} className={styles.row} data-phone-hidden={row.hideOnPhone || undefined}>
      <span>{row.label}</span><Segmented className={styles.segments} aria-label={row.label} value={row.value} onValueChange={row.onChange}
        items={row.options} />
    </div>)}</div>
    <p className={styles.note}><Icon name="i-info" size={18} /><span><b>{copy.sessionNote}</b>{' '}{copy.defaultsNote}</span>
      {onOpenSettings && copy.openSettings && <button type="button" onClick={onOpenSettings}>{copy.openSettings}<Icon name="i-right" size={16} /></button>}
    </p>
  </div>;
}

export type ReaderDisplayProps = ReaderDisplayControlsProps & Readonly<{
  label: string; open: boolean; onOpenChange: OverlayChange;
  anchorRef: RefObject<HTMLElement | null>;
}>;

/** Desktop popover and compact sheet share the same controlled values and callbacks. */
export function ReaderDisplayPopover({label, open, onOpenChange, anchorRef, ...controls}: ReaderDisplayProps) {
  return <Popover open={open} onOpenChange={onOpenChange} title={label} accessibilityLabel={label} anchorRef={anchorRef} width={404} sideOffset={8} className={styles.popover}>
    <ReaderDisplayControls {...controls} />
  </Popover>;
}

/** Sheet assembly is explicit so an unmounted desktop overlay never owns phone focus. */
export function ReaderDisplaySheet({label, open, onOpenChange, anchorRef, ...controls}: ReaderDisplayProps) {
  return <BottomSheet open={open} onOpenChange={onOpenChange} title={label} accessibilityLabel={label} returnFocusRef={anchorRef}
    showCloseButton={false} fitContent className={styles.sheet} headerClassName={styles.sheetHeader} bodyClassName={styles.sheetBody}>
    <ReaderDisplayControls {...controls} />
  </BottomSheet>;
}
