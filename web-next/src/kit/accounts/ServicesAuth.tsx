import {Fragment, type ReactNode} from 'react';
import {createI18n, parseMessage} from '@i18n';
import type {IdentityProvider} from '@core/hosted-security.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {Icon, type IconName} from '../Icon';
import {AuthStage} from './AuthStage';
import styles from './services-auth.module.css';

const t = createI18n().t;
export function ServicesAuthStage({children, variant = 'auth'}: {children: ReactNode; variant?: 'auth' | 'signup' | 'reset' | 'state'}) {
  return <AuthStage service legal shape={variant === 'state' ? 'round' : 'side'} layout={variant === 'state' ? 'center' : 'column'}
    className={styles.stage} contentClassName={[styles.content, styles[variant]].join(' ')}>{children}</AuthStage>;
}
export function ServicesHeading({children, singleLine = false}: {children: ReactNode; singleLine?: boolean}) {
  return <h1 className={styles.heading} data-single-line={singleLine || undefined}>{children}</h1>;
}
export function ServicesLead({children}: {children: ReactNode}) { return <p className={styles.lead}>{children}</p>; }
export function ServicesQuiet({children}: {children: ReactNode}) { return <p className={styles.quiet}>{children}</p>; }
export function ServicesNotice({children, tone = 'warning', icon, busy = false}: {children: ReactNode; tone?: 'warning' | 'error' | 'info' | 'success'; icon?: IconName; busy?: boolean}) {
  return <div className={styles.notice} data-tone={tone} role={tone === 'error' ? 'alert' : 'status'} aria-busy={busy || undefined}>
    {busy ? <span className={styles.spinner} aria-hidden="true" /> : <Icon name={icon ?? (tone === 'success' ? 'i-check' : tone === 'info' ? 'i-info' : 'i-warn')} size={20} />}
    <span>{children}</span>
  </div>;
}
export function ServicesStrength({password, id}: {password: string; id: string}) {
  if (!password) return null;
  const strength = passwordStrength(password);
  // Only the two textual strength states drawn in account 9/14/15 are introduced here.
  const label = strength.level === 0 ? t('services.password.tooShort') : strength.level === 4 ? t('services.password.strong') : '';
  return <div id={id} className={styles.strength} data-level={strength.level} aria-live="polite">
    {[1, 2, 3, 4].map(level => <i key={level} data-on={level <= Math.max(1, strength.level) || undefined} aria-hidden="true" />)}
    {label && <span>{label}</span>}
  </div>;
}
export function ServicesActions({children}: {children: ReactNode}) { return <div className={styles.actions}>{children}</div>; }
export function ServicesLabel({children, detail}: {children: ReactNode; detail?: ReactNode}) { return <span className={styles.label}>{children}{detail && <small>{detail}</small>}</span>; }
export function ServicesStateIcon({tone = 'success', icon = 'i-check'}: {tone?: 'success' | 'warning' | 'error' | 'info'; icon?: IconName}) {
  return <span className={styles.stateIcon} data-tone={tone}><Icon name={icon} size={34} /></span>;
}
export function ServicesProviderButton({provider, children, disabled, onClick}: {provider: IdentityProvider; children: ReactNode; disabled?: boolean; onClick: () => void}) {
  return <button type="button" className={styles.provider} data-provider={provider} disabled={disabled} onClick={onClick}><Icon name={provider === 'apple' ? 'i-apple' : 'i-google'} size={20} />{children}</button>;
}
export function ServicesDivider({children}: {children: ReactNode}) { return <div className={styles.divider}>{children}</div>; }
export function ServicesPair({children}: {children: ReactNode}) { return <div className={styles.pair}>{children}</div>; }
export function ServicesHint({children}: {children: ReactNode}) { return <p className={styles.hint}>{children}</p>; }
export function ServicesTerms() {
  const message = createI18n().t('services.signup.agreement', {terms: '{terms}', privacy: '{privacy}'});
  return <p className={styles.terms}>{parseMessage(message).map((part, index) => part.kind === 'text' ? <Fragment key={index}>{part.value}</Fragment> : part.kind === 'arg' && part.name === 'terms' ? <a key={index} href="https://getportico.tv/terms">{t('services.signup.terms')}</a> : part.kind === 'arg' && part.name === 'privacy' ? <a key={index} href="https://getportico.tv/privacy">{t('services.signup.privacy')}</a> : null)}</p>;
}
