import React, {useCallback, useEffect, useState} from 'react';
import {IdentityClient, type RememberedAccount} from '@core/index.ts';
import {devE2EUsername} from '@core/profile-management.ts';
import {browserInstallation} from '../../bridge/profile-trust';
import {serverAddress, useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {friendlyHost} from '@core/presentation/index.ts';
import {HttpLocalApi, type SystemInfo} from '@core/index.ts';
import {Button, Input, Loading, Notice, PasswordInput, Text, PasswordStrength} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';
import {AccountForm, AppleMark, GoogleMark, providerStyle} from './AccountForm';
import {AuthFrame} from './AuthFrame';
import {DirectProfiles} from './DirectProfiles';
import {ServerSetup} from './ServerSetup';
import s from './Auth.module.css';

const bundled = !import.meta.env.VITE_HOSTED_WEB && location.hostname !== 'web.getportico.tv';

/**
 * Signed-out entry. A Portico Account is the recommended path; a direct
 * server sign-in remains one step away. When served by a server (bundled
 * web) the server address is already known and verified.
 */
export function SignInScreen() {
  const session = useSession();
  const {t} = useI18n();
  // A direct sign-in that ended reopens on the direct form for the same server.
  const [mode, setMode] = useState<'account' | 'direct' | 'recovery'>(() => (session.signInHint?.authority === 'local' ? 'direct' : 'account'));
  const [probed, setProbed] = useState(!bundled);
  const [setup, setSetup] = useState<SystemInfo>();
  useEffect(() => {
    if (session.signInHint?.authority === 'local') setMode('direct');
  }, [session.signInHint]);
  // A bundled server tells us whether a Portico Account is even configured; otherwise direct sign-in is the only path.
  useEffect(() => {
    if (!bundled) return;
    let active = true;
    new HttpLocalApi(location.origin).system()
      .then((info: SystemInfo) => {
        if (!active) return;
        // ONB-01: a fresh server is set up here, never shown a sign-in form for an account that doesn't exist.
        if (info.setupRequired) setSetup(info);
        // A server that signs people in directly never shows the Portico Account hand-off.
        // (hostedConfigured only says the build could reach the account service; an older
        // server without hostedAttached falls back to it.)
        else if (!(info.hostedAttached ?? info.hostedConfigured)) setMode('direct');
      })
      // Unreachable: the local form is the one that can still explain what is wrong.
      .catch(() => { if (active) setMode('direct'); })
      .finally(() => active && setProbed(true));
    return () => { active = false; };
  }, []);
  if (!probed) return <AuthFrame legal={false}><Loading label={t('web.signin.opening')} /></AuthFrame>;
  if (setup) return <ServerSetup system={setup} />;
  if (session.direct.pending) return <AuthFrame title={t('web.servers.chooseProfile')} wide><DirectProfiles /></AuthFrame>;
  // INT gate 2: a Portico Account that is still signed in never gets a page of its own here. The
  // Gate sends it to the shell; it reaches this screen only for a direct sign-in that ended, and
  // "use a Portico Account" from there returns it to the shell.
  const toAccount = () => { if (session.hosted) session.dismissSignInHint(); else setMode('account'); };
  if (mode === 'direct' || mode === 'recovery' || session.hosted) return <DirectSignIn recovery={mode === 'recovery'} onAccount={toAccount} />;
  // Served by a server, the server has already decided: it either uses Portico Accounts or it
  // keeps its own, and the other way in is not offered.
  if (bundled) return <HostedHandOff onRecovery={() => setMode('recovery')} />;
  return <AccountSignIn onDirect={() => setMode('direct')} />;
}

function AccountSignIn({onDirect}: {onDirect: () => void}) {
  const session = useSession();
  const {t} = useI18n();
  const [heading, setHeading] = useState<{title: string; lede?: string}>({title: t('web.signin.welcomeHome'), lede: t('web.signin.accountLede')});
  const onTitle = useCallback((title: string, lede?: string) => setHeading(h => (h.title === title && h.lede === lede ? h : {title, lede})), []);
  return (
    <AuthFrame title={heading.title} lede={heading.lede}>
      <div className={s.stack}>
        <SignInEnded />
        {session.error ? <Notice tone="error">{session.error}</Notice> : null}
        <AccountForm onSignedIn={() => session.accountSignedIn()} onTitle={onTitle} />
        <div className={s.links}><Button variant="link" label={t('auth.signInDirectly')} onClick={onDirect} /></div>
      </div>
    </AuthFrame>
  );
}

/** A server that uses Portico Accounts, opened at its own address. The account service only signs
 * people in from its own site, so every option here continues there; the site then connects
 * back to this server. */
function HostedHandOff({onRecovery}: {onRecovery: () => void}) {
  const {t} = useI18n();
  const go = () => location.assign('https://web.getportico.tv/');
  return (
    <AuthFrame title={t('web.signin.welcomeHome')} lede={t('web.signin.hostedLede')}>
      <div className={s.stack}>
        <SignInEnded />
        <button type="button" style={providerStyle(true)} onClick={go}><GoogleMark />{t('auth.signInWithGoogle')}</button>
        <button type="button" style={providerStyle(false)} onClick={go}><AppleMark />{t('auth.signInWithApple')}</button>
        <div className={s.divider}>{t('web.signin.or')}</div>
        <Button variant="primary" size="lg" label={t('web.signin.emailButton')} iconAfter="external" onClick={go} block />
        <div className={s.links}><Button variant="link" label={t('auth.createAccount')} onClick={go} /></div>
        {/* The owner's recovery account lives on the server, for the day the account service cannot be reached. */}
        <div className={s.links}><Button variant="link" size="sm" label={t('web.direct.recoveryTitle')} onClick={onRecovery} /></div>
      </div>
    </AuthFrame>
  );
}

/** Direct sign-in to one server, on one page. The server's identity is pinned on first contact
 * and checked silently afterwards; a fingerprint is not something to ask a person to compare. */
function DirectSignIn({onAccount, recovery}: {onAccount: () => void; /** WEB-AUTH-04: reached from a Portico Account server's hand-off, as the owner's way in. */ recovery?: boolean}) {
  const {t} = useI18n();
  const session = useSession();
  const {direct} = session;
  const hint = session.signInHint?.authority === 'local' ? session.signInHint : undefined;
  const [username, setUsername] = useState(hint?.username ?? '');
  useEffect(() => { if (hint?.username) setUsername(current => current || hint.username!); }, [hint?.username]);
  const [password, setPassword] = useState('');
  const host = friendlyHost(direct.origin) ?? direct.origin.replace(/^https?:\/\//, '');
  // Who has signed in from this browser before. It is only a list of names to save typing:
  // choosing one fills in the username and the password is still asked for.
  const [remembered, setRemembered] = useState<readonly RememberedAccount[]>([]);
  // After a sign-in ended, the same server's list names who it was, so the username is filled in.
  const rememberedFrom = bundled ? location.origin : hint?.serverUrl;
  const endedAccount = hint?.accountId;
  useEffect(() => {
    if (!rememberedFrom) return;
    const controller = new AbortController();
    new IdentityClient(new HttpLocalApi(rememberedFrom)).rememberedAccounts(browserInstallation(), controller.signal).then(list => {
      setRemembered(list);
      const mine = list.find(a => a.accountId === endedAccount) ?? (list.length === 1 ? list[0] : undefined);
      if (mine) setUsername(current => current || mine.username);
    }, () => {});
    return () => controller.abort();
  }, [rememberedFrom, endedAccount]);
  // Development builds only: a server started for e2e tests offers its seeded owner, signed in with one click.
  const [devOwner, setDevOwner] = useState<string>();
  useEffect(() => {
    if (!import.meta.env.DEV || !direct.origin.trim()) return;
    const controller = new AbortController();
    let origin: string;
    try { origin = serverAddress(direct.origin); } catch { return; }
    void devE2EUsername(new HttpLocalApi(origin), controller.signal).then(setDevOwner);
    return () => controller.abort();
  }, [direct.origin]);
  const ready = !!direct.origin.trim() && !!username && !!password;
  if (direct.step) return <DirectStep />;
  return (
    <AuthFrame title={recovery ? t('web.direct.recoveryTitle') : t('web.direct.title')} lede={recovery ? t('web.direct.recoveryLede') : bundled ? t('web.direct.ledeBundled', {host}) : t('web.direct.lede')}>
      <form className={s.stack} onSubmit={e => { e.preventDefault(); if (ready) void direct.signIn(username, password); }}>
        <SignInEnded />
        {!bundled ? <Input label={t('web.direct.serverAddress')} value={direct.origin} onChange={e => { if (direct.pin) direct.resetPin(); direct.setOrigin(e.target.value); }} placeholder="192.168.1.20:32500" autoCapitalize="off" autoCorrect="off" spellCheck={false} required /> : null}
        {remembered.length > 1 ? (
          <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>{remembered.map(a => <Button key={a.accountId} size="sm" variant={username === a.username ? 'secondary' : 'outline'} icon="profile" label={a.displayName || a.username} onClick={() => setUsername(a.username)} />)}</div>
        ) : null}
        <Input label={t('web.direct.username')} value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" autoCapitalize="off" spellCheck={false} required minLength={3} maxLength={64} />
        <PasswordInput label={t('web.direct.password')} help={recovery ? t('web.direct.recoveryHelp') : undefined} value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" required />
        {session.error ? <Notice tone="error">{session.error}</Notice> : null}
        <Button variant="primary" type="submit" label={t('web.direct.signIn')} size="lg" loading={session.busy} disabled={!ready} block />
        {import.meta.env.DEV && devOwner ? <Button variant="outline" label={t('web.direct.devSignIn', {username: devOwner})} onClick={() => void direct.signIn(devOwner, '', undefined, true)} disabled={session.busy} block /> : null}
      </form>
      {!bundled ? <div className={s.links}><Button variant="link" label={t('web.direct.useAccount')} onClick={() => { direct.resetPin(); session.clearError(); onAccount(); }} /></div> : null}
      {!recovery ? <div className={s.links}><Button variant="link" label={t('web.directJoin.have')} to="/join" /></div> : null}
      {recovery ? <div className={s.links}><Button variant="link" icon="back" label={t('web.direct.back')} onClick={() => { session.clearError(); onAccount(); }} /></div> : null}
    </AuthFrame>
  );
}

/** In the shell's content area (a Portico Account's landing) a step is a panel, not a page of its own. */
function StepFrame({inShell, title, lede, children}: {inShell?: boolean; title: string; lede?: string; children: React.ReactNode}) {
  if (!inShell) return <AuthFrame title={title} lede={lede}>{children}</AuthFrame>;
  return (
    <section className={s.stack} aria-labelledby="step-title">
      <div className={s.heading}>
        <h2 id="step-title" className={s.stepTitle}>{title}</h2>
        {lede ? <p className={s.lede}>{lede}</p> : null}
      </div>
      {children}
    </section>
  );
}

/** C45: the extra step a direct sign-in may need: a two-step code, a required new password, or the owner's approval. */
export function DirectStep({inShell}: {inShell?: boolean}) {
  const {t} = useI18n();
  const session = useSession();
  const {direct} = session;
  const step = direct.step!;
  const [value, setValue] = useState('');
  const [again, setAgain] = useState('');
  const strong = (v: string) => passwordStrength(v).acceptable;
  const back = <div className={s.links}><Button variant="link" icon="back" label={t('action.back')} onClick={() => { session.clearError(); direct.cancelStep(); }} /></div>;
  if (step.kind === 'device-pending') {
    return (
      <StepFrame inShell={inShell} title={t('device.approval.title')} lede={t('device.approval.body')}>
        <div className={s.stack}>
          {session.error ? <Notice tone="error">{session.error}</Notice> : null}
          <Button variant="primary" size="lg" label={t('device.approval.checkAgain')} loading={session.busy} block onClick={() => void direct.continueSignIn('')} />
          {back}
        </div>
      </StepFrame>
    );
  }
  if (step.kind === 'new-password') {
    const ok = strong(value) && value === again;
    return (
      <StepFrame inShell={inShell} title={t('web.direct.newPasswordTitle')} lede={t('web.direct.newPasswordLede')}>
        <form className={s.stack} onSubmit={e => { e.preventDefault(); if (ok) void direct.continueSignIn(value); }}>
          <PasswordInput label={t('auth.newPassword')} autoComplete="new-password" aria-describedby="required-password-strength" value={value} autoFocus onChange={e => setValue(e.target.value)} />
          <PasswordStrength id="required-password-strength" password={value} />
          <PasswordInput label={t('account.confirmPassword')} autoComplete="new-password" value={again} onChange={e => setAgain(e.target.value)} />
          {session.error ? <Notice tone="error">{session.error}</Notice> : null}
          <Button variant="primary" type="submit" size="lg" label={t('web.direct.continue')} loading={session.busy} disabled={!ok} block />
        </form>
        {back}
      </StepFrame>
    );
  }
  return (
    <StepFrame inShell={inShell} title={t('web.direct.codeTitle')} lede={t('web.direct.codeLede')}>
      <form className={s.stack} onSubmit={e => { e.preventDefault(); if (value.trim().length >= 6) void direct.continueSignIn(value); }}>
        <Input label={t('security.authenticatorCode')} inputMode="numeric" autoComplete="one-time-code" value={value} autoFocus maxLength={40} onChange={e => setValue(e.target.value)} />
        {session.error ? <Notice tone="error">{session.error}</Notice> : null}
        <Button variant="primary" type="submit" size="lg" label={t('web.direct.continue')} loading={session.busy} disabled={value.trim().length < 6} block />
      </form>
      {back}
    </StepFrame>
  );
}

/** Why the sign-in screen opened: the previous sign-in to this server has ended. */
export function SignInEnded() {
  const {t} = useI18n();
  const {signInHint, servers} = useSession();
  if (!signInHint) return null;
  const server = servers.find(x => x.id === signInHint.serverId)?.name ?? friendlyHost(signInHint.serverUrl) ?? signInHint.serverUrl.replace(/^https?:\/\//, '');
  return <Notice tone="info">{t('web.signIn.ended', {server})}</Notice>;
}
