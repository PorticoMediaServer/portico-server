import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {browserAccount} from '../../bridge/account';
import {accountSecurityClient} from '../../bridge/account-security-client';
import {decodeVerificationToken, normalizeTogetherCode, setPendingTogetherCode, useFragmentSecrets} from '../../app/link-fragment';
import {confirmEmailLink, emailLinkOutcome} from '../../app/email-link';
import {errorText} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {Button, Input, Loading, Notice, PasswordInput, Text, PasswordStrength} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';
import {AccountForm} from './AccountForm';
import {DirectJoin} from './DirectJoin';
import {AuthFrame} from './AuthFrame';
import s from './Auth.module.css';

/** The Portico Account signed in in this browser, and a request helper carrying its bearer. */
function useAccount() {
  const central = useMemo(browserAccount, []);
  const client = useMemo(accountSecurityClient, []);
  const account = useSyncExternalStore(central.service.subscribe, central.service.getSnapshot);
  const authed = async () => {
    const session = await central.service.accessSession();
    return {session, request: <T,>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal) => client.request(path, method, body, signal, session.accessToken) as Promise<T>};
  };
  return {central, client, live: account.session, authed};
}

const status = (e: unknown) => (e as {status?: number} | undefined)?.status;

/** `/reset#token=…` — choose a new Portico Account password (Hosted `/v1/account/recovery/complete`). */
export function ResetPasswordScreen() {
  const i18n = useI18n();
  const navigate = useNavigate();
  const {token} = useFragmentSecrets(['token'] as const);
  const {client} = useAccount();
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [factor, setFactor] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  if (!token) return <AuthFrame title={i18n.t('web.reset.title')}><Notice tone="warning">{i18n.t('web.link.invalid')}</Notice><Button variant="link" label={i18n.t('web.link.backToPortico')} to="/" /></AuthFrame>;
  const submit = async () => {
    if (password !== confirm) { setError(i18n.t('web.reset.mismatch')); return; }
    setBusy(true); setError('');
    try {
      await client.request('/v1/account/recovery/complete', 'POST', {token, newPassword: password, factor: factor.trim()});
      setDone(true);
    } catch (e) {
      setError([404, 410].includes(status(e) ?? 0) ? i18n.t('web.reset.expired') : errorText(e, 'link', 'save'));
    } finally { setBusy(false); }
  };
  return (
    <AuthFrame title={done ? i18n.t('web.reset.doneTitle') : i18n.t('web.reset.title')} lede={done ? undefined : i18n.t('web.reset.lede')}>
      {done ? (
        <div className={s.stack}>
          <Notice tone="success">{i18n.t('web.reset.done')}</Notice>
          <Button variant="primary" block label={i18n.t('web.link.continue')} onClick={() => void navigate({to: '/'})} />
        </div>
      ) : (
        <form className={s.stack} onSubmit={e => { e.preventDefault(); void submit(); }}>
          <PasswordInput label={i18n.t('web.reset.password')} autoComplete="new-password" aria-describedby="reset-password-strength" value={password} onChange={e => setPassword(e.target.value)} autoFocus />
          <PasswordStrength id="reset-password-strength" password={password} />
          <PasswordInput label={i18n.t('web.reset.confirm')} autoComplete="new-password" value={confirm} onChange={e => setConfirm(e.target.value)} />
          <Input label={i18n.t('web.reset.factor')} help={i18n.t('web.reset.factorHelp')} inputMode="numeric" autoComplete="one-time-code" value={factor} onChange={e => setFactor(e.target.value)} optional />
          {error ? <Notice tone="error">{error}</Notice> : null}
          <Button type="submit" variant="primary" block label={i18n.t('web.reset.submit')} loading={busy} disabled={!password || !confirm} />
        </form>
      )}
    </AuthFrame>
  );
}

/**
 * `/verify-email#token=…`. When this browser still holds the binding (the registration it
 * started, the Google/Apple sign-in in progress, or the signed-in account for an email change),
 * the bound flow runs as before and signs in here. Otherwise (A33) the token alone confirms the
 * email from any browser: `POST /v1/email-links/confirm`, and the person goes back to the device
 * where they started, or signs in again after an email change.
 */
export function VerifyEmailScreen() {
  const i18n = useI18n();
  const navigate = useNavigate();
  const {token} = useFragmentSecrets(['token'] as const);
  const link = useMemo(() => decodeVerificationToken(token), [token]);
  const {central, client, live, authed} = useAccount();
  const [phase, setPhase] = useState<'working' | 'done' | 'elsewhere' | 'changed' | 'failed'>('working');
  const [error, setError] = useState('');
  useEffect(() => {
    if (!token) return;
    let alive = true;
    const bound = async (): Promise<boolean> => {
      if (!link) return false;
      if (link.kind === 'registration') {
        const current = await client.registration().catch(() => null);
        if (current?.registrationId !== link.resource) return false;
        await central.service.authenticate(signal => client.verifyRegistration(link.resource, {code: link.code}, signal));
        return true;
      }
      if (link.kind === 'oidc_contact') {
        const tx = await client.transaction().catch(() => null);
        if (!tx) return false;
        await client.verifyContact(tx.transactionId, link.resource, link.code);
        await central.service.authenticate(signal => client.complete(tx.transactionId, {}, signal)).catch(() => {});
        return true;
      }
      if (!live) return false;
      const {request} = await authed();
      await request('/v1/account/security/email/confirm', 'POST', {challengeId: link.resource, code: link.code, operationId: crypto.randomUUID()});
      return true;
    };
    const run = async () => {
      if (await bound()) { if (alive) setPhase('done'); return; }
      const result = await confirmEmailLink((path, method, body, signal) => client.request(path, method, body, signal), token);
      if (alive) setPhase(emailLinkOutcome(result) === 'changed' ? 'changed' : 'elsewhere');
    };
    run().catch(e => {
      if (!alive) return;
      setPhase('failed');
      setError([404, 409, 410].includes(status(e) ?? 0) ? i18n.t('web.verify.expired') : errorText(e, 'link', 'save'));
    });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);
  if (!token) return <AuthFrame title={i18n.t('web.verify.title')}><Notice tone="warning">{i18n.t('web.link.invalid')}</Notice><Button variant="link" label={i18n.t('web.link.backToPortico')} to="/" /></AuthFrame>;
  const title = phase === 'done' || phase === 'elsewhere' ? i18n.t('web.verify.doneTitle') : phase === 'changed' ? i18n.t('web.verify.changedTitle') : i18n.t('web.verify.title');
  return (
    <AuthFrame title={title}>
      <div className={s.stack}>
        {phase === 'working' ? <Loading label={i18n.t('web.verify.working')} /> : null}
        {phase === 'done' ? <><Notice tone="success">{i18n.t('web.verify.done')}</Notice><Button variant="primary" block label={i18n.t('web.link.continue')} onClick={() => void navigate({to: '/'})} /></> : null}
        {phase === 'elsewhere' ? <Notice tone="success">{i18n.t('web.verify.confirmedElsewhere')}</Notice> : null}
        {phase === 'changed' ? <><Notice tone="success">{i18n.t('web.verify.changed')}</Notice><Button variant="primary" block label={i18n.t('web.verify.signIn')} onClick={() => void navigate({to: '/'})} /></> : null}
        {phase === 'failed' ? <><Notice tone="warning">{error}</Notice><Button variant="link" label={i18n.t('web.link.backToPortico')} to="/" /></> : null}
      </div>
    </AuthFrame>
  );
}

/**
 * `/join#code=<code>&server=<origin>`: a server's invitation (DirectJoin). A Portico Account accepts
 * it with its identity; anyone else chooses a username and password on that server. The old
 * Portico Account invitation links (`#invite=`) belonged to Hosted memberships, which servers now
 * own (Spec — Hosted at Scale): they ask for a new invitation.
 */
export function JoinScreen() {
  const i18n = useI18n();
  const {invite, code, server} = useFragmentSecrets(['invite', 'code', 'server'] as const);
  if (invite && !code) return <AuthFrame title={i18n.t('web.join.title')}><Notice tone="warning">{i18n.t('web.join.unavailable')}</Notice><Button variant="link" label={i18n.t('web.link.backToPortico')} to="/" /></AuthFrame>;
  return <DirectJoin code={code} server={server} />;
}

/** `/together/join#code=…` — hands the code to Watch Together (prefilled there), through sign-in if needed. */
export function TogetherJoinScreen() {
  const navigate = useNavigate();
  const {code} = useFragmentSecrets(['code'] as const);
  useEffect(() => {
    const value = normalizeTogetherCode(code);
    if (value) setPendingTogetherCode(value);
    void navigate({to: '/together', replace: true});
  }, [code, navigate]);
  return null;
}
