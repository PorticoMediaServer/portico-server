import {useEffect, useMemo, useRef, useState} from 'react';
import {passwordStrength} from '@core/password-strength.ts';
import {AuthPasswordField} from '../kit/accounts';
import {HostedError, ServicesBackLink, ServicesAuthStage, ServicesHeading, ServicesLead, ServicesNotice, ServicesStrength, ServicesActions, ServicesLabel, ServicesStateIcon} from '../kit/accounts/ServicesAuth';
import {Button, TextField} from '../kit/controls';
import {resetPassword, verifyEmail, type VerificationOutcome} from './link-effects';
import type {ServicesAccount} from './session';
import {servicesI18n} from './i18n';
import styles from './services.module.css';

export function PasswordChanged() {
  const {t} = useMemo(servicesI18n, []);
  return <ServicesAuthStage variant="state"><ServicesStateIcon /><ServicesHeading>{t('web.reset.doneTitle')}</ServicesHeading><ServicesLead>{t('web.reset.done')}</ServicesLead><ServicesActions><Button variant="primary" onClick={() => location.assign('/')}>{t('web.link.continue')}</Button></ServicesActions></ServicesAuthStage>;
}

export function EmailVerified({outcome}: {outcome: VerificationOutcome}) {
  const {t} = useMemo(servicesI18n, []);
  return <ServicesAuthStage variant="state"><ServicesStateIcon />
    <ServicesHeading>{t(outcome === 'changed' ? 'web.verify.changedTitle' : 'web.verify.doneTitle')}</ServicesHeading>
    <ServicesLead>{t(outcome === 'changed' ? 'web.verify.changed' : outcome === 'elsewhere' ? 'web.verify.confirmedElsewhere' : 'web.verify.done')}</ServicesLead>
    {outcome !== 'elsewhere' && <ServicesActions><Button variant="primary" onClick={() => location.assign('/')}>{t(outcome === 'changed' ? 'web.verify.signIn' : 'web.link.continue')}</Button></ServicesActions>}
  </ServicesAuthStage>;
}

export function ResetPage({account, token}: {account: ServicesAccount; token?: string}) {
  const {t} = useMemo(servicesI18n, []);
  const [password, setPassword] = useState(''), [confirm, setConfirm] = useState(''), [factor, setFactor] = useState('');
  const [busy, setBusy] = useState(false), [done, setDone] = useState(false), [error, setError] = useState<unknown>();
  const [mismatch, setMismatch] = useState(false);
  const operation = useRef<AbortController | null>(null);
  useEffect(() => () => operation.current?.abort(), []);
  const status = (error as {status?: number} | undefined)?.status;
  const code = (error as {code?: string} | undefined)?.code;
  // The reset route names its own closed states: a dead, used, superseded or
  // exhausted link answers 410 reset_link_unavailable, a refused password
  // answers password_invalid and a rejected second factor answers mfa_invalid.
  const expired = status === 410 || code === 'reset_link_unavailable';
  const factorWrong = code === 'mfa_invalid';
  const passwordWrong = code === 'password_invalid';
  const submit = async () => {
    if (!token || operation.current) return;
    if (password !== confirm) { setMismatch(true); return; }
    const controller = new AbortController(); operation.current = controller;
    setBusy(true); setError(undefined); setMismatch(false);
    try {
      await resetPassword(account, token, password, factor, controller.signal);
      if (!controller.signal.aborted) { setPassword(''); setConfirm(''); setFactor(''); setDone(true); }
    } catch (value) { if (!controller.signal.aborted) setError(value); }
    finally { if (operation.current === controller) operation.current = null; if (!controller.signal.aborted) setBusy(false); }
  };
  if (done) return <PasswordChanged />;
  return <ServicesAuthStage variant="reset"><ServicesHeading singleLine>{t('web.reset.title')}</ServicesHeading>
    {!token || expired ? <><ServicesNotice icon={expired ? 'i-clock' : 'i-warn'}>{t(expired ? 'web.reset.expired' : 'web.link.invalid')}</ServicesNotice><ServicesActions><Button variant={expired ? 'primary' : 'glass'} onClick={() => location.assign(expired ? '/?mode=forgot' : '/')}>{t(expired ? 'services.reset.askNew' : 'web.link.backToPortico')}</Button></ServicesActions></> : <>
      <ServicesLead>{t('web.reset.lede')}</ServicesLead>
      <form className={styles.form} onSubmit={event => { event.preventDefault(); void submit(); }}>
        <div><AuthPasswordField name="new-password" label={<ServicesLabel detail={t('auth.passwordRule')}>{t('web.reset.password')}</ServicesLabel>}
          autoComplete="new-password" value={password} onChange={event => { setPassword(event.currentTarget.value); setMismatch(false); }} autoFocus required disabled={busy}
          aria-describedby={password ? 'reset-strength' : undefined} error={passwordWrong ? t('services.reset.longer') : undefined} />
          <ServicesStrength password={password} id="reset-strength" /></div>
        <AuthPasswordField name="confirm-password" label={t('web.reset.confirm')} autoComplete="new-password" value={confirm}
          onChange={event => { setConfirm(event.currentTarget.value); setMismatch(false); }} required disabled={busy} error={mismatch ? t('web.reset.mismatch') : undefined} />
        <TextField variant="auth" name="one-time-code" label={<ServicesLabel detail={t('field.optional')}>{t('web.reset.factor')}</ServicesLabel>}
          autoComplete="one-time-code" value={factor} onChange={event => setFactor(event.currentTarget.value)} placeholder={t('services.factor.placeholder')} disabled={busy}
          description={<><span className={styles.desktopOnly}>{t('web.reset.factorHelp')}</span><span className={styles.phoneOnly}>{t('services.reset.factorHelpPhone')}</span></>}
          error={factorWrong ? t('signIn.error.codeInvalid') : undefined} />
        {error && !factorWrong && !passwordWrong ? <HostedError error={error} /> : null}
        <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={!passwordStrength(password).acceptable || !confirm}>{t('web.reset.submit')}</Button></ServicesActions>
      </form>
      <ServicesBackLink href="/">{t('web.link.backToPortico')}</ServicesBackLink>
    </>}
  </ServicesAuthStage>;
}

export function VerifyPage({account, token}: {account: ServicesAccount; token?: string}) {
  const {t} = useMemo(servicesI18n, []);
  const [outcome, setOutcome] = useState<VerificationOutcome>();
  const [error, setError] = useState<unknown>();
  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void verifyEmail(account, token, controller.signal).then(result => { if (!controller.signal.aborted) setOutcome(result); }, value => { if (!controller.signal.aborted) setError(value); });
    return () => controller.abort();
  }, [account, token]);
  const code = (error as {code?: string} | undefined)?.code;
  // invalid_link means the fragment token did not parse: the drawn cut-off link.
  // challenge_invalid is the route's only dead-or-used answer on this page.
  const cutOff = !token || code === 'invalid_link';
  const expired = code === 'challenge_invalid';
  if (outcome) return <EmailVerified outcome={outcome} />;
  return <ServicesAuthStage variant="reset">
    <ServicesHeading>{t('web.verify.title')}</ServicesHeading>
    {cutOff ? <ServicesNotice>{t('web.link.invalid')}</ServicesNotice> : expired ? <ServicesNotice icon="i-clock">{t('web.verify.expired')}</ServicesNotice> : error ? <HostedError error={error} /> : <ServicesNotice tone="info" busy>{t('web.verify.working')}</ServicesNotice>}
    {!token || error ? <ServicesActions><Button onClick={() => location.assign('/')}>{t('web.link.backToPortico')}</Button></ServicesActions> : null}
  </ServicesAuthStage>;
}
