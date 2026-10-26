import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import {Icon} from '../../../kit/Icon';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;

/** The two Google sign-in choices exactly as remote-sources.html 3g/3h draw
 * them: plain radio cards with the existing canonical descriptions. The
 * caller keeps the mode-switch callback and all OAuth transport. */
export function DriveModePane({mode, disabled, onChange}: Readonly<{
  mode: 'service' | 'oauth'; disabled: boolean; onChange: (mode: 'service' | 'oauth') => void;
}>) {
  const choose = (next: 'service' | 'oauth') => {if (!disabled) onChange(next);};
  return <div className={styles.driveModes} role="radiogroup" aria-label={t('next.remoteSources.driveSignIn')}>
    <button type="button" role="radio" aria-checked={mode === 'service'} data-picked={mode === 'service' || undefined}
      className={styles.driveMode} disabled={disabled} onClick={() => choose('service')}>
      <span className={styles.driveModeRadio} aria-hidden="true"/>
      <span className={styles.driveModeBody}>
        <span className={styles.driveModeTitle}>{t('next.remoteSources.driveServiceAccount')}
          <span className={styles.driveModeTag}>{t('next.remoteSources.driveRecommended')}</span></span>
        <span className={styles.driveModeHint}>{t('next.remoteSources.driveServiceAccountHint')}</span>
      </span>
    </button>
    <button type="button" role="radio" aria-checked={mode === 'oauth'} data-picked={mode === 'oauth' || undefined}
      className={styles.driveMode} disabled={disabled} onClick={() => choose('oauth')}>
      <span className={styles.driveModeRadio} aria-hidden="true"/>
      <span className={styles.driveModeBody}>
        <span className={styles.driveModeTitle}>{t('next.remoteSources.driveOAuth')}</span>
        <span className={styles.driveModeHint}>{t('next.remoteSources.driveOAuthHint')}</span>
      </span>
    </button>
  </div>;
}

/** The service-account key pane. Empty, invalid, OAuth-client and valid states
 * are the parser's existing facts; Upload/Clear/Copy and the OAuth switch are
 * the caller's own handlers. No parser, upload transport or secret handling. */
export function DriveKeyPane({value, disabled, error, email, oauth, onChange, onUpload, onClear, onCopy, onUseOAuth}: Readonly<{
  value: string; disabled: boolean; error?: string; email?: string; oauth: boolean;
  onChange: (value: string) => void; onUpload: () => void; onClear: () => void; onCopy: () => void; onUseOAuth: () => void;
}>) {
  const empty = !value.trim();
  return <>
    <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.driveKey')}</span>
      <textarea className={`${styles.config} ${styles.driveKeyArea}`} aria-label={t('next.remoteSources.driveKey')} value={value} placeholder={t('next.remoteSources.driveKeyPlaceholder')}
        maxLength={65536} disabled={disabled} spellCheck={false} rows={5} onChange={event => onChange(event.target.value)}/>
      <small className={styles.note}>{t('next.remoteSources.driveKeyHelp')}</small>
    </label>
    <div className={styles.testRow}>
      <Button size="compact" variant="glass" disabled={disabled} onClick={onUpload}>{t('next.remoteSources.driveUpload')}</Button>
      <Button size="compact" variant="text" disabled={disabled || empty} onClick={onClear}>{t('next.remoteSources.driveClear')}</Button>
    </div>
    {empty ? <p className={styles.note}>{t('next.remoteSources.driveKeyEmptyHint')}</p>
      : error ? <div className={styles.driveKeyError}><Icon name="i-warn" size={18}/><span>{error}</span>
        {oauth && <Button size="xs" variant="glass" disabled={disabled} onClick={onUseOAuth}>{t('next.remoteSources.driveOAuth')}</Button>}</div>
      : email ? <div className={styles.driveKeyValid}>
        <div className={styles.driveKeyHead}><Icon name="i-check" size={20}/><div>
          <b>{t('next.remoteSources.driveEmailHeading')}</b><small>{t('next.remoteSources.driveEmailHelp')}</small>
        </div></div>
        <span className={styles.driveKeyEmail}>{email}</span>
        <Button size="xs" variant="text" onClick={onCopy}>{t('next.remoteSources.driveCopy')}</Button>
      </div> : null}
  </>;
}
