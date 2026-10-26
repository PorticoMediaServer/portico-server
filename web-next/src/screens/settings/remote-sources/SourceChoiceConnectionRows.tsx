import {defaultI18n} from '@i18n';
import type {SourceReadValues} from '@core/remote-sources.ts';
import type {SourceChoiceField} from '@core/remote-source-choice.ts';
import {SettingNumber} from '../../../kit/settings';
import styles from './source-choice-connection-rows.module.css';

const t = defaultI18n.t;

type ConnectionField = 'connections' | 'requestsPerSecond' | 'bytesPerSecond';
type ConnectionValues = Pick<SourceReadValues, ConnectionField>;

export type SourceChoiceConnectionMarkers = Readonly<{
  /** The rows this opening applies; the caller supplies the actual closed IDs. */
  applicable: readonly SourceChoiceField[];
  /** The rows whose current value differs from the chosen definition. */
  changed: readonly SourceChoiceField[];
  /** The caller's already-formatted baseline words for one row, or undefined. */
  value: (field: SourceChoiceField) => string | undefined;
  onReset?: (field: SourceChoiceField) => void;
}>;

export type SourceChoiceConnectionRowsProps = Readonly<{
  values: ConnectionValues;
  disabled: boolean;
  onChange: (patch: Partial<ConnectionValues>) => void;
  markers?: SourceChoiceConnectionMarkers;
}>;

/** The three connection limits as the Reading dialog already draws them, with the
 * authored changed-row marker from remote-sources.html (.r.chg and its bar at
 * 1016-1017, the ringed number at 751). Marker presentation only: the caller owns
 * applicability, the changed set, each formatted baseline string and the exact
 * callbacks, so this reads no comparison, command, read, state or clock. A row
 * outside the marker, or with no markers at all, draws the plain number control. */
export function SourceChoiceConnectionRows({values, disabled, onChange, markers}: SourceChoiceConnectionRowsProps) {
  const on = (field: SourceChoiceField) => !!markers && markers.applicable.includes(field) && markers.changed.includes(field);
  const word = (field: SourceChoiceField) => (on(field) ? markers!.value(field) : undefined);
  const reset = (field: SourceChoiceField) => (markers?.onReset ? () => markers.onReset!(field) : undefined);
  const conn = word('connections'), rps = word('requestsPerSecond'), speed = word('bytesPerSecond');
  return <div className={styles.rows}>
    <div className={styles.cell} data-changed={on('connections') || undefined}>
      <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32}
        unit={t('next.remoteSources.atOnce')} disabled={disabled}
        onCommit={value => {if (value !== null) onChange({connections: value});}}/>
      {conn !== undefined && <p className={styles.marker}>
        <span>{t('next.remoteSources.presetValue', {value: conn})}</span>
        {markers?.onReset && <button type="button" className={styles.reset} disabled={disabled} onClick={reset('connections')}>{t('action.reset')}</button>}
      </p>}
    </div>
    <div className={styles.cell} data-changed={on('requestsPerSecond') || undefined}>
      <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50}
        unit={t('next.remoteSources.aSecond')} disabled={disabled}
        onCommit={value => {if (value !== null) onChange({requestsPerSecond: value});}}/>
      {rps !== undefined && <p className={styles.marker}>
        <span>{t('next.remoteSources.presetValue', {value: rps})}</span>
        {markers?.onReset && <button type="button" className={styles.reset} disabled={disabled} onClick={reset('requestsPerSecond')}>{t('action.reset')}</button>}
      </p>}
    </div>
    <div className={styles.cell} data-changed={on('bytesPerSecond') || undefined}>
      <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null}
        min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={disabled}
        onCommit={value => onChange({bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      {speed !== undefined && <p className={styles.marker}>
        <span>{t('next.remoteSources.presetValue', {value: speed})}</span>
        {markers?.onReset && <button type="button" className={styles.reset} disabled={disabled} onClick={reset('bytesPerSecond')}>{t('action.reset')}</button>}
      </p>}
    </div>
  </div>;
}
