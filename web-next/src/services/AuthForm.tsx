import {useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode, type FormEvent} from 'react';
import {passwordStrength} from '@core/password-strength.ts';
import {AuthCodeBoxes, AuthLinks, AuthPasswordField} from '../kit/accounts';
import {HostedError, ServicesActions, ServicesAuthStage, ServicesBackAction, ServicesDivider, ServicesHeading, ServicesHint, ServicesInvitationCredentials, ServicesLabel, ServicesLead, ServicesNotice, ServicesPair, ServicesProviderAccount, ServicesProviderButton, ServicesProviderContext, ServicesProviderReturnHint, ServicesQuiet, ServicesRegistrationLead, ServicesStateIcon, ServicesStrength, ServicesTerms, ServicesVerificationCode} from '../kit/accounts/ServicesAuth';
import {Button, TextField} from '../kit/controls';
import {ServicesAuthFlow} from './auth-flow';
import type {ServicesAccount} from './session';
import {servicesI18n} from './i18n';
import styles from './services.module.css';
import signInStyles from './services-signin-r46.module.css';

type Props = {account: ServicesAccount; onSignedIn: (gesture?: boolean) => void; initialMode?: 'signin' | 'create' | 'forgot'; initialUsername?: string; embedded?: boolean; invitation?: boolean; heading?: ReactNode; lead?: ReactNode; before?: ReactNode; after?: ReactNode};
export function ServicesAuthForm({account, onSignedIn, initialMode = 'signin', initialUsername = '', embedded = false, invitation = false, heading, lead, before, after}: Props) {
  const {t} = useMemo(servicesI18n, []), complete = useRef(onSignedIn); complete.current = onSignedIn;
  const flow = useMemo(() => new ServicesAuthFlow(account, gesture => complete.current(gesture), initialMode), [account, initialMode]);
  const state = useSyncExternalStore(flow.subscribe, flow.getSnapshot);
  // Follow family replacement without copying a second session into the form.
  useSyncExternalStore(account.service.subscribe, account.service.getSnapshot);
  const readyAccount = flow.readyAccount();
  const [username, setUsername] = useState(initialUsername), [password, setPassword] = useState(''), [email, setEmail] = useState(''), [name, setName] = useState(''), [code, setCode] = useState('');
  const [clock, setClock] = useState(Date.now);
  useEffect(() => {
    if (!state.retryAt) return;
    setClock(Date.now());
    const timer = setInterval(() => setClock(Date.now()), 250);
    return () => clearInterval(timer);
  }, [state.retryAt]);
  useEffect(() => { void flow.initialize(); return () => flow.dispose(); }, [flow]);
  useEffect(() => { setPassword(''); setCode(''); if (state.mode === 'provider') setName(state.suggestedName ?? ''); }, [state.mode, state.suggestedName]);
  const submit = (work: () => Promise<void>) => (event: FormEvent) => { event.preventDefault(); void work(); };
  const retrySeconds = Math.max(0, Math.ceil((state.retryAt - clock) / 1000));
  const retryLabel = t('services.signin.tryIn', {time: `${Math.floor(retrySeconds / 60)}:${String(retrySeconds % 60).padStart(2, '0')}`});
  const busy = state.busy, blocked = busy || retrySeconds > 0, mode = state.mode;
  const back = <button type="button" disabled={busy} onClick={() => flow.go('signin')}>{t('auth.backToSignIn')}</button>;
  const title = mode === 'create' ? t('auth.createYourAccount') : mode === 'verify' || mode === 'sent' ? t('auth.checkEmail') : mode === 'forgot' ? t('auth.forgotPasswordTitle') : mode === 'factor' ? t('auth.enterVerificationCode') : mode === 'recovery' ? t('services.signin.recovery') : mode === 'provider' ? t('auth.chooseUsername') : mode === 'ready' ? t('services.provider.ready') : heading ?? t('auth.welcomeHome');
  const body = mode === 'create' ? t('auth.oneAccount') : mode === 'verify' && state.registration ? t('auth.codeSentTo', {address: state.registration.maskedAddress}) : mode === 'sent' ? t('auth.resetLinkSent') : mode === 'forgot' ? t('auth.resetLinkLedeEmail') : mode === 'factor' ? t('services.signin.twoStep') : mode === 'recovery' ? t('services.signin.recoveryBody') : mode === 'provider' ? t('services.provider.lastThing') : mode === 'ready' ? t('services.provider.readyBody') : lead ?? t('auth.accountLead');
  const providers = state.options?.providers.filter(item => item.enabled).sort((a, b) => a.id.localeCompare(b.id)) ?? [];
  const providerButtons = <>{providers.map(item => <ServicesProviderButton key={item.id} provider={item.id} disabled={busy} onClick={() => void flow.provider(item.id)}>{t(item.id === 'apple' ? 'services.provider.apple' : 'services.provider.google')}</ServicesProviderButton>)}{providers.length > 0 && state.options?.password && <div className={mode === 'signin' ? signInStyles.divider : undefined}><ServicesDivider>{t(mode === 'create' ? 'services.provider.createDivider' : 'auth.orWithEmail')}</ServicesDivider></div>}</>;
  const content = <>
    {mode === 'verify' && <ServicesStateIcon tone="info" icon="i-mail" size={32} />}
    {mode === 'ready' && <ServicesStateIcon />}
    {mode === 'provider' && state.provider && <ServicesProviderContext provider={state.provider} email={state.verifiedEmail} />}
    {!(invitation && mode === 'signin') && <><ServicesHeading singleLine>{title}</ServicesHeading>{mode === 'verify' && state.registration ? <ServicesRegistrationLead address={state.registration.maskedAddress} /> : <ServicesLead>{body}</ServicesLead>}</>}
    {mode === 'signin' && before}
    {(mode === 'signin' || mode === 'create') && providerButtons}
    {state.error ? state.retryAt > 0 ? <ServicesNotice tone="error">{t('services.signin.tooMany')}</ServicesNotice> : (state.error as {code?: string})?.code === 'invalid_credentials' ? <ServicesNotice tone="error">{t('services.signin.credentials')}</ServicesNotice> : <HostedError error={state.error} /> : null}
    {mode === 'signin' && state.options?.password && <>
      <form className={styles.form} onSubmit={submit(() => flow.signIn(username, password, code))}>
        <TextField variant="auth" label={t('auth.usernameOrEmail')} placeholder={t('auth.usernameOrEmail')} name="username" autoComplete="username" autoCapitalize="off" spellCheck={false} value={username} onChange={event => setUsername(event.currentTarget.value)} disabled={busy} required />
        <AuthPasswordField containerClassName={signInStyles.password} label={t('auth.password')} placeholder={t('auth.password')} name="password" autoComplete="current-password" value={password} onChange={event => setPassword(event.currentTarget.value)} disabled={busy} required />
        <div className={signInStyles.submit}><ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={blocked || !username.trim() || !password}>{retrySeconds ? retryLabel : t('action.continue')}</Button></ServicesActions></div>
      </form>
      <AuthLinks>{state.options.recovery && <button type="button" disabled={busy} onClick={() => flow.go('forgot')}>{t('auth.forgotPassword')}</button>}{state.options.registration && <button type="button" disabled={busy} onClick={() => flow.go('create')}>{t('auth.createAccount')}</button>}</AuthLinks>
    </>}
    {mode === 'create' && state.options?.registration && <>
      <form className={styles.form} onSubmit={submit(() => flow.create({username, password, displayName: name, contactAddress: email}))}>
        <TextField variant="auth" label={t('auth.email')} name="email" type="email" autoComplete="email" value={email} onChange={event => setEmail(event.currentTarget.value)} description={t('services.signup.emailHelp')} required disabled={busy} />
        <ServicesPair>
          <TextField variant="auth" label={t('auth.username')} name="username" autoComplete="username" autoCapitalize="off" spellCheck={false} value={username} onChange={event => setUsername(event.currentTarget.value)} minLength={3} maxLength={32} required disabled={busy} />
          <TextField variant="auth" label={<ServicesLabel detail={t('field.optional')}>{t('auth.yourName')}</ServicesLabel>} name="name" autoComplete="name" placeholder={t('services.signup.sameUsername')} value={name} onChange={event => setName(event.currentTarget.value)} maxLength={80} disabled={busy} />
        </ServicesPair>
        <ServicesHint>{t('auth.usernameHelp')}</ServicesHint>
        <div><AuthPasswordField label={<ServicesLabel detail={t('auth.passwordRule')}>{t('auth.password')}</ServicesLabel>} name="new-password" autoComplete="new-password" value={password} onChange={event => setPassword(event.currentTarget.value)} disabled={busy} required aria-describedby={password ? 'signup-strength' : undefined} /><ServicesStrength password={password} id="signup-strength" /></div>
        <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={blocked || !/^\S+@\S+\.\S+$/.test(email.trim()) || username.trim().length < 3 || !passwordStrength(password).acceptable}>{retrySeconds ? retryLabel : t('action.continue')}</Button></ServicesActions>
      </form>
      <AuthLinks>{back}</AuthLinks><ServicesTerms />
    </>}
    {mode === 'verify' && state.registration && <>
      <form className={styles.form} onSubmit={submit(() => flow.verify(code))}>
        <ServicesVerificationCode><AuthCodeBoxes length={6} value={code} onChange={setCode} onComplete={() => {}} label={t('auth.code')} disabled={busy} /></ServicesVerificationCode>
        <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={blocked || code.length !== 6}>{retrySeconds ? retryLabel : t('auth.createAccount')}</Button></ServicesActions>
      </form>
      <AuthLinks><button type="button" disabled={blocked || state.resends >= 5} onClick={() => void flow.resend()}>{t('auth.sendCodeAgain')}</button><ServicesQuiet>{t('services.signup.resendLimit')}</ServicesQuiet></AuthLinks>
      <AuthLinks><button type="button" disabled={busy} onClick={() => flow.startAgain()}>{t('services.signup.startAgain')}</button>{back}</AuthLinks><ServicesQuiet>{t('services.signup.codeHelp')}</ServicesQuiet>
    </>}
    {mode === 'forgot' && state.options?.recovery && <>
      <form className={styles.form} onSubmit={submit(() => flow.recover(email))}>
        <TextField variant="auth" type="email" name="email" autoComplete="email" label={t('auth.email')} value={email} onChange={event => setEmail(event.currentTarget.value)} required disabled={busy} />
        <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={blocked || !/^\S+@\S+\.\S+$/.test(email.trim())}>{retrySeconds ? retryLabel : t('auth.resetLinkAction')}</Button></ServicesActions>
      </form><ServicesQuiet>{t('auth.privacySameAnswer')}</ServicesQuiet><ServicesQuiet>{t('services.recovery.provider')}</ServicesQuiet><AuthLinks>{back}</AuthLinks>
    </>}
    {mode === 'sent' && <><ServicesQuiet>{t('services.recovery.expiry')}</ServicesQuiet><AuthLinks>{back}</AuthLinks></>}
    {(mode === 'factor' || mode === 'recovery') && <>
      <form className={styles.form} onSubmit={submit(() => flow.signIn(username, password, code))}>
        {mode === 'factor' ? <ServicesVerificationCode label={false}><AuthCodeBoxes length={6} value={code} onChange={setCode} onComplete={() => {}} label={t('auth.code')} disabled={busy} /></ServicesVerificationCode> : <TextField variant="auth" name="recovery-code" autoComplete="one-time-code" label={t('services.signin.recovery')} placeholder={t('services.recovery.codePlaceholder')} value={code} onChange={event => setCode(event.currentTarget.value)} disabled={busy} />}
        {mode === 'factor' && <AuthLinks><span>{t('services.signin.lostPhone')}</span><button type="button" disabled={busy} onClick={() => flow.recovery()}>{t('services.signin.recovery')}</button></AuthLinks>}
        <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={blocked || (mode === 'factor' ? code.length !== 6 : !code.trim())}>{retrySeconds ? retryLabel : t('action.continue')}</Button></ServicesActions>
      </form><AuthLinks>{mode === 'recovery' ? <button type="button" disabled={busy} onClick={() => flow.recovery()}>{t('services.signin.authenticator')}</button> : back}</AuthLinks>
    </>}
    {mode === 'provider' && <>
      <form className={styles.form} onSubmit={submit(() => flow.finishProvider(username, name))}>
        <TextField variant="auth" label={t('auth.username')} name="username" autoComplete="username" autoCapitalize="off" spellCheck={false} description={t('services.provider.usernameHelp')} value={username} onChange={event => setUsername(event.currentTarget.value)} minLength={3} maxLength={32} required disabled={busy} />
        <TextField variant="auth" label={<ServicesLabel detail={t('field.optional')}>{t('auth.yourName')}</ServicesLabel>} name="name" autoComplete="name" value={name} onChange={event => setName(event.currentTarget.value)} disabled={busy} />
        <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={blocked || username.trim().length < 3}>{retrySeconds ? retryLabel : t('action.continue')}</Button></ServicesActions>
      </form><AuthLinks><button type="button" disabled={busy} onClick={() => void flow.cancelProvider()}><ServicesBackAction>{t('action.cancel')}</ServicesBackAction></button></AuthLinks><ServicesTerms provider={state.provider} />
    </>}
    {mode === 'ready' && <>{readyAccount && state.provider && <ServicesProviderAccount {...readyAccount} provider={state.provider} />}
      <ServicesActions><Button variant="primary" loading={busy} disabled={!readyAccount} onClick={() => void flow.continue()}>{t('action.continue')}</Button></ServicesActions><ServicesProviderReturnHint /></>}
    {mode === 'signin' && after}
  </>;
  return invitation && mode === 'signin' ? <ServicesInvitationCredentials>{content}</ServicesInvitationCredentials> : embedded ? content : <ServicesAuthStage legal={mode !== 'create' && mode !== 'provider' && mode !== 'verify'} variant={mode === 'provider' ? 'providerOnboarding' : mode === 'create' ? 'signup' : mode === 'verify' ? 'reset' : mode === 'ready' ? 'state' : 'auth'}>{content}</ServicesAuthStage>;
}
