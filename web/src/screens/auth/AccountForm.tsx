import React, {useEffect, useMemo, useRef, useState} from 'react';
import {AccountAttemptIds, MFARequiredError, type IdentityOptions, type IdentityProvider, type MFAChallenge} from '@core/hosted-security';
import type {HostedSession} from '@core/index.ts';
import {browserAccount} from '../../bridge/account';
import {accountSecurityClient} from '../../bridge/account-security-client';
import {Button, Input, Notice, PasswordInput, Text, PasswordStrength} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';
import s from './Auth.module.css';

type Mode = 'signin' | 'create' | 'verify' | 'forgot' | 'sent';
/** MAIL-04: the recovery request carries the email only; the username stays empty. */
export const recoveryRequestBody = (contactAddress: string, requestId: string) => ({username: '', contactAddress: contactAddress.trim(), requestId});
/** X-04 / INT gate 3: words come from the sign-in presenter by code and status, never from `error.message`. */
const say = (e: unknown, step: SignInStep) => signInFailure(e, {step, online: navigator.onLine}, currentI18n()).text;
const email = (v: string) => /^\S+@\S+\.\S+$/.test(v.trim());

import {GoogleMark, AppleMark, providerStyle} from '../../ui/ProviderIdentity';
import {signInFailure, type SignInStep} from '../../app/sign-in-errors';
import {currentI18n} from '../../app/i18n';
export {GoogleMark, AppleMark, providerStyle} from '../../ui/ProviderIdentity';

/**
 * Signing in to a Portico Account in the browser, and the two things next to it: making an
 * account and getting back into one. Everything happens here; nothing is handed to another
 * device or another page.
 */
export function AccountForm({onSignedIn, onTitle}: {onSignedIn: (session: HostedSession) => void; onTitle?: (title: string, lede?: string) => void}) {
  const t = currentI18n().t;
  const central = useMemo(browserAccount, []);
  const client = useMemo(accountSecurityClient, []);
  const ids = useRef(new AccountAttemptIds(async () => crypto.randomUUID()));
  const [mode, setMode] = useState<Mode>('signin');
  const [options, setOptions] = useState<IdentityOptions>();
  const [username, setUsername] = useState('');
  const [name, setName] = useState('');
  const [address, setAddress] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [mfa, setMFA] = useState<MFAChallenge>();
  const [registration, setRegistration] = useState<{registrationId: string; maskedAddress: string}>();
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const accept = async (work: (signal: AbortSignal) => Promise<HostedSession>) => { onSignedIn(await central.service.authenticate(work)); };
  useEffect(() => {
    let live = true;
    client.options().then(o => live && setOptions(o), () => {});
    // Coming back from Google or Apple: the sign-in that was started is finished here.
    client.transaction().then(async tx => {
      if (!live || !tx || tx.status === 'failed' || tx.needsUsername || tx.mfaRequired) return;
      if (tx.status === 'verified' || tx.status === 'completed') await accept(signal => client.complete(tx.transactionId, {}, signal)).catch(() => {});
    }, () => {});
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client]);
  useEffect(() => {
    // MAIL-04: one link, /reset#token=, nothing to paste. The new password is chosen on that
    // page, so this form asks for the email only and then says to check the inbox.
    const titles: Record<Mode, [string, string?]> = {
      signin: mfa ? ['Enter your verification code'] : ['Welcome home.', 'Sign in with your Portico Account to access servers shared with you.'],
      create: ['Create your account', 'One Portico Account reaches every server you own or that someone shares with you.'],
      verify: ['Check your email', registration ? `We sent a code to ${registration.maskedAddress}.` : undefined],
      forgot: [currentI18n().t('auth.forgotPasswordTitle'), currentI18n().t('auth.resetLinkLedeEmail')],
      sent: [currentI18n().t('auth.checkEmail'), note],
    };
    onTitle?.(...titles[mode]);
  }, [mode, mfa, registration, note, onTitle]);
  const run = async (work: () => Promise<void>) => {
    if (busy) return;
    setBusy(true); setError('');
    try { await work(); } catch (e) {
      if (e instanceof MFARequiredError) { setMFA(e.challenge); setCode(''); } else setError(say(e, mfa || mode === 'verify' ? 'account-code' : mode === 'create' ? 'account-create' : mode === 'forgot' || mode === 'sent' ? 'account-recovery' : 'account'));
    } finally { setBusy(false); }
  };
  const go = (next: Mode) => { setError(''); setCode(''); setPassword(''); setMode(next); };
  const signIn = () => run(async () => {
    if (mfa) { await accept(signal => client.mfa(mfa, code.trim(), signal)); return; }
    const id = await ids.current.for('login', {username, password});
    await accept(signal => client.login(username.trim(), password, id, signal));
  });
  const provider = (p: IdentityProvider) => run(async () => {
    const started = await client.start(p, {mode: 'login', requestId: await ids.current.for('provider', {p})});
    if (!started.authorizationUrl) throw Object.assign(new Error('That sign-in isn’t available right now.'), {code: 'provider_unavailable', service: 'portico-account'});
    location.assign(started.authorizationUrl);
  });
  const create = () => run(async () => {
    setRegistration(await client.register({username: username.trim().toLowerCase(), displayName: name.trim() || username.trim(), password, contactAddress: address.trim(), requestId: await ids.current.for('register', {username, address})}));
    setMode('verify');
  });
  const verify = () => run(async () => { if (registration) await accept(signal => client.verifyRegistration(registration.registrationId, {code: code.trim()}, signal)); });
  const resend = () => run(async () => { if (registration) setRegistration(await client.resendRegistration(registration.registrationId, await ids.current.for('resend', {at: Date.now()}))); });
  const request = () => run(async () => {
    // MAIL-04: email only — the server accepts an empty username, and answers the same either way.
    const out = (await client.request('/v1/account/recovery/request', 'POST', recoveryRequestBody(address, await ids.current.for('recover', {address})))) as {message?: string} | undefined;
    setNote(out?.message ?? currentI18n().t('auth.resetLinkSent'));
    go('sent');
  });
  const strong = passwordStrength(password).acceptable;
  const providers = options?.providers.filter(p => p.enabled) ?? [];
  const back = <Button variant="link" label={t('auth.backToSignIn')} onClick={() => { setMFA(undefined); go('signin'); }} />;

  if (mode === 'create') {
    return (
      <form className={s.stack} onSubmit={e => { e.preventDefault(); void create(); }}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Input label={t('auth.email')} type="email" autoComplete="email" value={address} onChange={e => setAddress(e.target.value)} required />
        <Input label={t('auth.username')} help={t('auth.usernameHelp')} autoComplete="username" autoCapitalize="off" spellCheck={false} value={username} onChange={e => setUsername(e.target.value)} required minLength={3} maxLength={32} />
        <Input label={t('auth.yourName')} autoComplete="name" value={name} onChange={e => setName(e.target.value)} maxLength={80} />
        <PasswordInput label={t('auth.password')} autoComplete="new-password" aria-describedby="signup-password-strength" value={password} onChange={e => setPassword(e.target.value)} required />
        <PasswordStrength id="signup-password-strength" password={password} />
        <Button variant="primary" type="submit" size="lg" label={t('action.continue')} loading={busy} disabled={!email(address) || username.trim().length < 3 || !strong} block />
        <div className={s.links}>{back}</div>
      </form>
    );
  }
  if (mode === 'verify') {
    return (
      <form className={s.stack} onSubmit={e => { e.preventDefault(); void verify(); }}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Input label={t('auth.code')} inputMode="numeric" autoComplete="one-time-code" value={code} onChange={e => setCode(e.target.value)} autoFocus />
        <Button variant="primary" type="submit" size="lg" label={t('auth.createAccount')} loading={busy} disabled={code.trim().length < 4} block />
        <div className={s.links}><Button variant="link" label={t('auth.sendCodeAgain')} onClick={() => void resend()} />{back}</div>
      </form>
    );
  }
  if (mode === 'forgot') {
    return (
      <form className={s.stack} onSubmit={e => { e.preventDefault(); void request(); }}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Input label={t('auth.email')} type="email" autoComplete="email" value={address} onChange={e => setAddress(e.target.value)} required autoFocus />
        <Button variant="primary" type="submit" size="lg" label={currentI18n().t('auth.resetLinkAction')} loading={busy} disabled={!email(address)} block />
        <Text variant="caption" tone="tertiary">{t('auth.privacySameAnswer')}</Text>
        <div className={s.links}>{back}</div>
      </form>
    );
  }
  if (mode === 'sent') {
    return (
      <div className={s.stack}>
        <Text variant="body">{note}</Text>
        <div className={s.links}>{back}</div>
      </div>
    );
  }
  if (mfa) {
    return (
      <form className={s.stack} onSubmit={e => { e.preventDefault(); void signIn(); }}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Input label={t('profile.pinRecoveryCode')} inputMode="numeric" autoComplete="one-time-code" value={code} onChange={e => setCode(e.target.value)} autoFocus />
        <Button variant="primary" type="submit" size="lg" label={t('action.continue')} loading={busy} disabled={code.trim().length < 6} block />
        <div className={s.links}>{back}</div>
      </form>
    );
  }
  return (
    <form className={s.stack} onSubmit={e => { e.preventDefault(); void signIn(); }}>
      {providers.map(p => <button key={p.id} type="button" style={providerStyle(p.id === 'google')} disabled={busy} onClick={() => void provider(p.id)}>{p.id === 'google' ? <GoogleMark /> : <AppleMark />}{p.id === 'google' ? 'Sign in with Google' : 'Sign in with Apple'}</button>)}
      {providers.length && options?.password !== false ? <div className={s.divider}>{t('auth.orWithEmail')}</div> : null}
      {options?.password !== false ? (
        <>
          <Input label={t('auth.usernameOrEmail')} placeholder={t('auth.usernameOrEmail')} autoComplete="username" autoCapitalize="off" spellCheck={false} value={username} onChange={e => setUsername(e.target.value)} required />
          <PasswordInput label={t('auth.password')} placeholder={t('auth.password')} autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} required />
          {error ? <Notice tone="error">{error}</Notice> : null}
          <Button variant="primary" type="submit" size="lg" label={t('action.continue')} loading={busy} disabled={!username || !password} block />
        </>
      ) : error ? <Notice tone="error">{error}</Notice> : null}
      <div className={s.links}>
        {options?.recovery !== false ? <Button variant="link" label={t('auth.forgotPassword')} onClick={() => go('forgot')} /> : null}
        {options?.registration !== false ? <Button variant="link" label={t('auth.createAccount')} onClick={() => go('create')} /> : null}
      </div>
    </form>
  );
}
