import {defaultI18n} from '@i18n';
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
