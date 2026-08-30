import { StatusWarningIcon, NavigationBackIcon, NavigationForwardIcon, AccountProfileIcon, DeviceNetworkIcon, StatusLoadingIcon, StatusLockedIcon, AccountSignInIcon, ActionRefreshIcon, DeviceServerIcon, DeviceOfflineIcon, StatusSecureIcon, DeviceWifiIcon } from '#portico-icons';
import {
  productMessage,
  resolveProductProblem,
  validPorticoUsername,
  type ProductMessageId,
  type ProductMessagePresentation,
  type SemanticIconId,
} from '@porticomediaserver/client-core';
import { type FormEvent, type ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import { PrimaryButton, SecondaryButton } from '../components/controls/Buttons';
import { PasswordInput, PasswordRequirements, validPorticoPassword } from '../components/controls/PasswordInput';
import { productText } from '../components/ProductLanguage';
import type { HostedServerSummary } from './runtimeMachine';
import { useRuntime, type SSOOnboardingPreview } from './RuntimeContext';
import { useHostedAvailabilityRetry } from './hostedAvailability';

const profileSelectionRequired = productMessage('auth.profile-selection-required');

function ProductStatusIcon({ icon }: { icon?: SemanticIconId }) {
  switch (icon) {
    case 'status.locked':
      return <StatusLockedIcon data-semantic-icon={icon} />;
    case 'status.offline':
      return <DeviceOfflineIcon data-semantic-icon={icon} />;
    case 'status.profile':
      return <AccountProfileIcon data-semantic-icon={icon} />;
    case 'status.server':
      return <DeviceOfflineIcon data-semantic-icon={icon} />;
    case 'status.loading':
      return <StatusLoadingIcon data-semantic-icon={icon} />;
    default:
      return <StatusWarningIcon data-semantic-icon={icon ?? 'status.error'} />;
  }
}

function canonicalProblem(reason: unknown, fallback: ProductMessageId = 'problem.request-failed'): ProductMessagePresentation {
  if (!reason || typeof reason !== 'object') return productMessage(fallback);
  const candidate = reason as { code?: unknown; messageId?: unknown; status?: unknown; details?: unknown };
  const resolved = resolveProductProblem({
    ...(typeof candidate.code === 'string' ? { code: candidate.code } : {}),
    ...(typeof candidate.messageId === 'string' ? { messageId: candidate.messageId } : {}),
    ...(typeof candidate.status === 'number' ? { status: candidate.status } : {}),
    ...(candidate.details && typeof candidate.details === 'object' ? { details: candidate.details as Readonly<Record<string, unknown>> } : {}),
  });
  return resolved.id === 'problem.request-failed' && fallback !== 'problem.request-failed'
    ? productMessage(fallback)
    : resolved;
}

function ProductProblem({ presentation }: { presentation: ProductMessagePresentation }) {
  return <div className="auth-error" role="alert">
    <ProductStatusIcon icon={presentation.icon} />
    <span><strong>{presentation.title}</strong>{presentation.body && <small>{presentation.body}</small>}</span>
  </div>;
}

function AccountLegalLinks() {
  return <p className="runtime-account-legal">By continuing, you agree to Portico’s <a href="https://getportico.tv/terms/" target="_blank" rel="noreferrer">Terms of Use</a> and acknowledge the <a href="https://getportico.tv/privacy/" target="_blank" rel="noreferrer">Privacy Policy</a>.</p>;
}

const NATIVE_DEVICE_AUTHORIZATION_COMPLETION_URL = 'portico://device-authorization-complete';
const NATIVE_DEVICE_SSO_GUARD_PREFIX = 'portico.hosted.native-device-sso-start.v1:';

export function consumeNativeDeviceSSOAutoStart(provider: 'google' | 'apple', code: string): boolean {
  if (typeof window === 'undefined' || !normalizeDeviceCode(code)) return false;
  const key = `${NATIVE_DEVICE_SSO_GUARD_PREFIX}${provider}:${normalizeDeviceCode(code)}`;
  try {
    if (window.sessionStorage.getItem(key) === 'started') return false;
    window.sessionStorage.setItem(key, 'started');
    return true;
  } catch { return false; }
}

export function nativeDeviceAuthorizationCompletionURL(nativeReturn: boolean): string | undefined {
  return nativeReturn ? NATIVE_DEVICE_AUTHORIZATION_COMPLETION_URL : undefined;
}

function RuntimePanel({ title, children, icon, wide = false, embedded = false }: { title: string; children: ReactNode; icon?: ReactNode; wide?: boolean; embedded?: boolean }) {
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), [title]);
  const panel = <section className={`auth-panel runtime-panel ${embedded ? 'runtime-panel-embedded' : ''} ${wide ? 'wide' : ''}`}>
    {!embedded && <img className="auth-wordmark" src="/brand/portico-wordmark-white.svg" alt="Portico" />}
    {icon && <div className="runtime-state-icon" aria-hidden="true">{icon}</div>}
    <h1 ref={heading} className="runtime-route-heading" tabIndex={-1}>{title}</h1>
    {children}
  </section>;
  return embedded ? <div className="runtime-embedded-surface">{panel}</div> : <main className="auth-surface runtime-surface">{panel}</main>;
}

function RuntimeProgress({ title, body, kind, serverName, embedded = false }: { title: string; body: string; kind: 'configuration' | 'local' | 'account' | 'memberships' | 'route'; serverName?: string; embedded?: boolean }) {
  const runtime = useRuntime();
  const [delayed, setDelayed] = useState(false);
  useEffect(() => {
    const timer = window.setTimeout(() => setDelayed(true), kind === 'route' ? 12_000 : 8_000);
    return () => window.clearTimeout(timer);
  }, [kind]);
  const icon = kind === 'route' ? <DeviceNetworkIcon /> : kind === 'memberships' ? <DeviceServerIcon /> : <StatusLoadingIcon />;
  // Ordinary connection work is intentionally silent inside the product
  // frame. The reserved projection area prevents a flash or layout shift;
  // only an exhausted/terminal recovery state becomes user-facing UI.
  if (embedded) return <div className="runtime-content-reservation" aria-busy="true" aria-label={title} />;
  return <RuntimePanel title={title} icon={icon}>
    <p className="runtime-intro" aria-live="polite">{delayed ? (kind === 'route' ? `The direct route to ${serverName || 'this server'} is taking longer than expected.` : 'This step is taking longer than expected.') : body}</p>
    <div className="runtime-progress-line" aria-hidden="true"><span /></div>
    {kind === 'route' && <div className="runtime-connection-facts"><span><StatusSecureIcon />HTTPS and server identity must verify</span><span><DeviceNetworkIcon />Playback connects directly to your server</span></div>}
    {delayed && kind !== 'configuration' && <div className="runtime-actions">{kind === 'route' && <SecondaryButton onClick={runtime.reselectServer}><NavigationBackIcon /> Choose another server</SecondaryButton>}{(kind === 'account' || kind === 'memberships') && <button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button>}</div>}
    {!delayed && kind === 'route' && <SecondaryButton onClick={runtime.reselectServer}><NavigationBackIcon /> Choose another server</SecondaryButton>}
  </RuntimePanel>;
}

function HostedSignIn() {
  const runtime = useRuntime();
  const [mode, setMode] = useState<'sign-in' | 'register' | 'request-reset' | 'complete-reset'>(runtime.hasPasswordResetIntent ? 'complete-reset' : 'sign-in');
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [mfaCode, setMfaCode] = useState('');
  const [recoveryCode, setRecoveryCode] = useState('');
  const [useRecoveryCode, setUseRecoveryCode] = useState(false);
  const [error, setError] = useState<ProductMessagePresentation>();
  const [resetSent, setResetSent] = useState(false);
  const canonicalNotice = runtime.state.id === 'hosted-sign-in' && runtime.state.messageId
    ? productMessage(runtime.state.messageId)
    : undefined;
  const changeMode = (next: typeof mode) => {
    setMode(next); setError(undefined); setResetSent(false); setUsername(''); setPassword(''); setMfaCode(''); setRecoveryCode(''); setUseRecoveryCode(false);
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError(undefined);
    try {
      if (mode === 'request-reset') {
        await runtime.requestPasswordReset(email);
        setResetSent(true);
      } else if (mode === 'complete-reset') {
        if (!validPorticoPassword(password)) throw new TypeError('Choose a password that meets every requirement.');
        await runtime.completePasswordReset(password);
        setPassword('');
        changeMode('sign-in');
      } else if (mode === 'register') {
        if (!validPorticoUsername(username)) throw new TypeError('Choose a valid username.');
        if (!validPorticoPassword(password)) throw new TypeError('Choose a password that meets every requirement.');
        await runtime.hostedRegister({ email: email.trim(), username: username.trim(), password });
        changeMode('sign-in');
      } else {
        await runtime.hostedLogin({ login: email.trim(), password, mfaCode: useRecoveryCode ? undefined : mfaCode.trim() || undefined, recoveryCode: useRecoveryCode ? recoveryCode.trim() || undefined : undefined });
      }
    } catch (reason) {
      setError(canonicalProblem(reason));
    }
  };
  const claimSetup = runtime.hasServerClaimIntent;
  const deviceSetup = runtime.hasDeviceAuthorizationIntent;
  const claimServerName = runtime.serverClaimName;
  const title = mode === 'register' ? (deviceSetup ? 'Create an account to connect your device' : claimSetup ? (claimServerName ? `Create an account to continue setting up “${claimServerName}”` : 'Create an account to continue') : productText('account.create-title')) : mode === 'request-reset' ? 'Recover your Portico Account' : mode === 'complete-reset' ? 'Choose a new password' : runtime.mfaRequired ? (claimServerName ? `Verify your sign-in for “${claimServerName}”` : 'Verify your sign-in') : deviceSetup ? 'Sign in to connect your device' : claimSetup ? (claimServerName ? `Continue setting up your server “${claimServerName}”` : 'Continue setting up your server') : 'Sign in to Portico';
  const intro = mode === 'register' ? (deviceSetup ? 'Create a Portico Account, then review the device before granting access.' : claimSetup ? 'Create a Portico Account to continue with server setup.' : productText('account.create-intro')) : mode === 'request-reset' ? 'Enter your account email. If it matches an account, Portico will send a secure recovery link.' : mode === 'complete-reset' ? 'Choose a new password for your Portico Account.' : runtime.mfaRequired ? 'Enter the code from your authenticator, or use one of your recovery codes.' : deviceSetup ? 'Sign in or create an account, then confirm the device requesting access.' : claimSetup ? 'Sign in or create a Portico Account to continue with server setup.' : 'Sign in using your Portico Account credentials.';
  const showIdentityProviders = (mode === 'sign-in' || mode === 'register') && !runtime.mfaRequired;
	const submitDisabled = runtime.busy
		|| (mode === 'register' && (!validPorticoUsername(username) || !email.trim() || !validPorticoPassword(password)))
		|| (mode === 'request-reset' && !email.trim())
		|| (mode === 'complete-reset' && !validPorticoPassword(password))
		|| (mode === 'sign-in' && (!email.trim() || !password || (runtime.mfaRequired && !(useRecoveryCode ? recoveryCode.trim() : mfaCode.trim()))));
  const identityProviderURL = (provider: 'google' | 'apple') => {
    const target = new URL(`/auth/sso/${provider}/start`, 'https://web.getportico.tv');
    if (typeof window !== 'undefined') {
      const returnTo = new URL(`${window.location.origin}${window.location.pathname}${window.location.search}`);
      if (runtime.hasLocalLoginIntent) returnTo.searchParams.set('localLoginResume', '1');
      target.searchParams.set('returnTo', returnTo.toString());
    }
    return target.toString();
  };
  useEffect(() => {
    const provider = runtime.deviceAuthorizationProvider;
    const code = runtime.deviceAuthorizationCode;
    if (mode !== 'sign-in' || runtime.mfaRequired || !runtime.nativeDeviceAuthorizationReturn || !provider || !code) return;
    if (!consumeNativeDeviceSSOAutoStart(provider, code)) return;
    window.location.assign(identityProviderURL(provider));
  }, [mode, runtime.deviceAuthorizationCode, runtime.deviceAuthorizationProvider, runtime.mfaRequired, runtime.nativeDeviceAuthorizationReturn]);
  if (resetSent) return <RuntimePanel title="Save your email"><p className="runtime-intro">If an account matches <strong>{email}</strong>, recovery instructions are on the way. The link expires and can be used only once.</p><div className="runtime-actions"><PrimaryButton onClick={() => changeMode('sign-in')}><NavigationBackIcon /> Back to sign in</PrimaryButton><SecondaryButton disabled={runtime.busy} onClick={() => setResetSent(false)}>Send again</SecondaryButton></div></RuntimePanel>;
  return <RuntimePanel title={title}>
    <p className="runtime-intro">{intro}</p>
    {showIdentityProviders && <>
      <div className="runtime-identity-providers" aria-label="Continue with a sign-in provider">
        <a className="runtime-identity-provider google" href={identityProviderURL('google')}>
          <svg aria-hidden="true" viewBox="0 0 24 24"><path fill="#4285F4" d="M21.6 12.23c0-.71-.06-1.4-.18-2.07H12v3.92h5.38a4.6 4.6 0 0 1-2 3.02v2.54h3.24c1.9-1.75 2.98-4.33 2.98-7.41Z"/><path fill="#34A853" d="M12 22c2.7 0 4.98-.9 6.63-2.36l-3.24-2.54c-.9.6-2.05.96-3.39.96-2.61 0-4.82-1.76-5.61-4.13H3.04v2.62A10 10 0 0 0 12 22Z"/><path fill="#FBBC05" d="M6.39 13.93A6 6 0 0 1 6.08 12c0-.67.11-1.33.31-1.93V7.45H3.04A10 10 0 0 0 2 12c0 1.64.39 3.19 1.04 4.55l3.35-2.62Z"/><path fill="#EA4335" d="M12 5.94c1.47 0 2.79.5 3.83 1.5l2.87-2.87A9.63 9.63 0 0 0 12 2a10 10 0 0 0-8.96 5.45l3.35 2.62C7.18 7.7 9.39 5.94 12 5.94Z"/></svg>
          <span>Sign in with Google</span>
        </a>
        <a className="runtime-identity-provider apple" href={identityProviderURL('apple')}>
          <svg aria-hidden="true" viewBox="0 0 24 24"><path fill="currentColor" d="M16.77 12.53c-.02-2.24 1.83-3.33 1.91-3.38a4.08 4.08 0 0 0-3.21-1.74c-1.35-.14-2.66.81-3.35.81-.7 0-1.76-.8-2.9-.78a4.26 4.26 0 0 0-3.58 2.18c-1.55 2.68-.4 6.62 1.09 8.79.75 1.06 1.62 2.25 2.76 2.21 1.12-.05 1.54-.71 2.89-.71 1.34 0 1.73.71 2.9.68 1.2-.02 1.96-1.06 2.68-2.13a8.75 8.75 0 0 0 1.23-2.49 3.86 3.86 0 0 1-2.42-3.44Zm-2.18-6.55a3.9 3.9 0 0 0 .9-2.8 4 4 0 0 0-2.6 1.33 3.73 3.73 0 0 0-.92 2.7 3.3 3.3 0 0 0 2.62-1.23Z"/></svg>
          <span>Sign in with Apple</span>
        </a>
      </div>
      <div className="runtime-identity-divider"><span>{mode === 'register' ? 'or create an account with email' : 'or sign in with email'}</span></div>
    </>}
    <form onSubmit={submit}>
      {mode === 'register' && <label><span>Username</span><input autoFocus autoCapitalize="none" autoComplete="username" placeholder="Choose a username" minLength={3} maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,30}[A-Za-z0-9]" value={username} onChange={(event) => setUsername(event.target.value)} required /><small>3–32 letters, numbers, periods, underscores, or hyphens.</small></label>}
      {mode !== 'complete-reset' && <label><span>{mode === 'sign-in' ? 'Username or email' : 'Email'}</span><input autoFocus={mode !== 'register'} type={mode === 'sign-in' ? 'text' : 'email'} autoCapitalize="none" autoComplete="username" placeholder={mode === 'sign-in' ? 'Username or email' : 'you@example.com'} value={email} onChange={(event) => setEmail(event.target.value)} required /></label>}
      {(mode === 'sign-in' || mode === 'register' || mode === 'complete-reset') && <label><span>{mode === 'complete-reset' ? 'New password' : 'Password'}</span><PasswordInput aria-label={mode === 'complete-reset' ? 'New password' : 'Password'} autoComplete={mode === 'sign-in' ? 'current-password' : 'new-password'} placeholder={mode === 'sign-in' ? 'Password' : 'Create a secure password'} minLength={mode === 'sign-in' ? undefined : 8} maxLength={72} value={password} onChange={(event) => setPassword(event.target.value)} required aria-describedby={mode !== 'sign-in' ? 'runtime-password-requirements' : undefined} />{mode !== 'sign-in' && <PasswordRequirements id="runtime-password-requirements" value={password} />}</label>}
      {mode === 'sign-in' && runtime.mfaRequired && <div className="runtime-mfa"><label><span>{useRecoveryCode ? 'Recovery code' : 'Verification code'}</span><input autoFocus inputMode={useRecoveryCode ? 'text' : 'numeric'} autoComplete="one-time-code" value={useRecoveryCode ? recoveryCode : mfaCode} onChange={(event) => useRecoveryCode ? setRecoveryCode(event.target.value) : setMfaCode(event.target.value.replace(/\s/g, ''))} required /></label><button type="button" className="runtime-text-action" onClick={() => { setUseRecoveryCode((value) => !value); setMfaCode(''); setRecoveryCode(''); }}>{useRecoveryCode ? 'Use an authenticator code' : 'Use a recovery code'}</button></div>}
      {canonicalNotice && <p className="runtime-message warning" role="alert"><ProductStatusIcon icon={canonicalNotice.icon} /><span><strong>{canonicalNotice.title}</strong>{canonicalNotice.body}</span></p>}
      {error && <ProductProblem presentation={error} />}
      <PrimaryButton type="submit" disabled={submitDisabled}>{runtime.busy ? <><StatusLoadingIcon className="runtime-spinner" /> Please wait…</> : mode === 'register' ? 'Create account' : mode === 'request-reset' ? 'Send recovery email' : mode === 'complete-reset' ? 'Update password' : runtime.mfaRequired ? 'Verify and sign in' : 'Sign in'}</PrimaryButton>
      {mode === 'sign-in' ? <div className="runtime-form-links"><button type="button" className="runtime-text-action" onClick={() => changeMode('request-reset')}>Forgot password?</button><button type="button" className="runtime-text-action" onClick={() => changeMode('register')}>Create an account</button></div> : mode !== 'complete-reset' && <button type="button" className="runtime-text-action runtime-back-link" onClick={() => changeMode('sign-in')}><NavigationBackIcon /> Back to sign in</button>}
    </form>
    {(mode === 'sign-in' || mode === 'register') && <AccountLegalLinks />}
  </RuntimePanel>;
}

function LocalLoginRecovery() {
  const runtime = useRuntime();
  if (runtime.state.id !== 'local-login-recovery') return null;
  const body = runtime.state.reason === 'callback-policy'
    ? 'This server address is not currently approved for Portico Account sign-in. Return to the server setup page, confirm its public address, and start sign-in again.'
    : runtime.state.reason === 'expired'
    ? 'This server sign-in request expired. Return to your server and choose Use a Portico Account again.'
    : runtime.state.reason === 'unavailable'
      ? 'This browser could not restore the protected server sign-in request. Return to your server and start sign-in again.'
      : 'The protected server sign-in request is no longer available. Return to your server and choose Use a Portico Account again.';
  return <RuntimePanel title={runtime.state.reason === 'callback-policy' ? 'Check this server address' : 'Restart server sign-in'}>
    <p className="runtime-intro">{body}</p>
    <div className="runtime-actions"><SecondaryButton onClick={() => void runtime.hostedLogout()}><NavigationBackIcon /> Go to Portico sign in</SecondaryButton></div>
  </RuntimePanel>;
}

function rotatedOnboardingToken(reason: unknown): string | undefined {
  if (!reason || typeof reason !== 'object') return undefined;
  const candidate = reason as { onboardingToken?: unknown; details?: unknown };
  const details = candidate.details && typeof candidate.details === 'object'
    ? candidate.details as Record<string, unknown>
    : undefined;
  const token = candidate.onboardingToken ?? details?.onboardingToken;
  return typeof token === 'string' && token.trim() ? token.trim() : undefined;
}

function onboardingUsernameUnavailable(reason: unknown): boolean {
  if (!reason || typeof reason !== 'object') return false;
  const candidate = reason as { code?: unknown; usernameUnavailable?: unknown; details?: unknown };
  const details = candidate.details && typeof candidate.details === 'object'
    ? candidate.details as Record<string, unknown>
    : undefined;
  return candidate.code === 'username_unavailable'
    || candidate.usernameUnavailable === true
    || details?.usernameUnavailable === true;
}

function SSOOnboarding() {
  const runtime = useRuntime();
  const initialToken = runtime.ssoOnboardingToken ?? '';
  const [onboardingToken, setOnboardingToken] = useState(initialToken);
  const [preview, setPreview] = useState<SSOOnboardingPreview>();
  const [username, setUsername] = useState('');
  const [contactEmail, setContactEmail] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ProductMessagePresentation>();
  const [usernameError, setUsernameError] = useState('');

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(undefined);
    void runtime.previewSSOOnboarding(onboardingToken, controller.signal).then((next) => {
      if (controller.signal.aborted) return;
      setPreview(next);
      setUsername((current) => current || next.suggestedUsername || '');
    }).catch((reason) => {
      if (!controller.signal.aborted) setError(canonicalProblem(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [onboardingToken]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const normalizedUsername = username.trim();
    setError(undefined);
    setUsernameError('');
    if (!validPorticoUsername(normalizedUsername)) {
      setUsernameError('Use 3–32 letters, numbers, periods, underscores, or hyphens. Start and end with a letter or number.');
      return;
    }
    if (preview?.verifiedContactEmailRequired) return;
    try {
      await runtime.completeSSOOnboarding({
        onboardingToken,
        username: normalizedUsername,
        ...(!preview?.providerEmail && contactEmail.trim() ? { contactEmail: contactEmail.trim() } : {}),
      });
    } catch (reason) {
      if (onboardingUsernameUnavailable(reason)) {
        const rotated = rotatedOnboardingToken(reason);
        if (rotated) setOnboardingToken(rotated);
        setUsernameError('That username is already in use. Choose another.');
        return;
      }
      setError(canonicalProblem(reason));
    }
  };

  if (loading) return <RuntimeProgress title="Finishing account setup" body="Checking your verified sign-in details…" kind="account" />;
  if (!preview) return <RuntimePanel title="Account setup could not be opened"><p className="runtime-intro">This setup link is invalid, expired, or could not be verified. Start again with Google or Apple.</p>{error && <ProductProblem presentation={error} />}<a className="button primary" href="/">Back to sign in</a></RuntimePanel>;

  const providerName = preview.provider === 'apple' ? 'Apple' : 'Google';
  return <RuntimePanel title="Choose your Portico username">
    <p className="runtime-intro">Your username is required and must be unique. You can use it to sign in and identify your Portico Account.</p>
    <form onSubmit={submit}>
      {preview.providerEmail && <label htmlFor="sso-onboarding-email"><span>{providerName}-verified email</span><input id="sso-onboarding-email" type="email" value={preview.providerEmail} readOnly aria-readonly="true" /><small>{preview.privateEmail ? 'Apple private relay address. Apple forwards messages according to your Sign in with Apple settings.' : `Verified by ${providerName}.`}</small></label>}
      {!preview.providerEmail && !preview.verifiedContactEmailRequired && <label htmlFor="sso-onboarding-contact-email"><span>Contact email (optional)</span><input id="sso-onboarding-contact-email" type="email" autoCapitalize="none" autoComplete="email" value={contactEmail} onChange={(event) => setContactEmail(event.target.value)} placeholder="you@example.com" /><small>This address is optional and will not be marked verified.</small></label>}
      {preview.verifiedContactEmailRequired && <div className="runtime-message warning" role="alert"><StatusWarningIcon /><span><strong>A verified contact email is still required</strong>Portico cannot finish this account safely yet. Entering an address here would not verify that you own it. Start the provider flow again after verified-email onboarding is available.</span></div>}
      <label htmlFor="sso-onboarding-username"><span>Username</span><input id="sso-onboarding-username" aria-label="Username" autoFocus autoCapitalize="none" autoComplete="username" minLength={3} maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,30}[A-Za-z0-9]" value={username} onChange={(event) => { setUsername(event.target.value); setUsernameError(''); }} aria-invalid={Boolean(usernameError)} aria-describedby={usernameError ? 'sso-onboarding-username-error' : 'sso-onboarding-username-help'} required /><small id={usernameError ? undefined : 'sso-onboarding-username-help'}>3–32 letters, numbers, periods, underscores, or hyphens.</small></label>
      {usernameError && <p id="sso-onboarding-username-error" className="runtime-message warning" role="alert"><StatusWarningIcon /><span>{usernameError}</span></p>}
      {error && <ProductProblem presentation={error} />}
      <PrimaryButton type="submit" disabled={runtime.busy || preview.verifiedContactEmailRequired}>{runtime.busy ? <><StatusLoadingIcon className="runtime-spinner" /> Creating account…</> : 'Create Portico Account'}</PrimaryButton>
    </form>
    <AccountLegalLinks />
  </RuntimePanel>;
}

function normalizeDeviceCode(value: string): string {
  const raw = value.toUpperCase().replace(/[^A-HJ-KM-NP-Z2-9]/g, '').slice(0, 8);
  return raw.length > 4 ? `${raw.slice(0, 4)}-${raw.slice(4)}` : raw;
}

type DeviceRequestTicket = { version: number; code: string; controller: AbortController };

function useDeviceRequestFence(initialCode: string) {
  const codeRef = useRef(normalizeDeviceCode(initialCode));
  const versionRef = useRef(0);
  const controllerRef = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => controllerRef.current?.abort(), []);
  return {
    begin(code: string): DeviceRequestTicket {
      controllerRef.current?.abort();
      const controller = new AbortController();
      controllerRef.current = controller;
      versionRef.current += 1;
      return { version: versionRef.current, code, controller };
    },
    invalidate(nextCode: string) {
      codeRef.current = nextCode;
      versionRef.current += 1;
      controllerRef.current?.abort();
      controllerRef.current = undefined;
    },
    current(ticket: DeviceRequestTicket) {
      return !ticket.controller.signal.aborted && ticket.version === versionRef.current && ticket.code === codeRef.current;
    },
  };
}

function cleanRequestAbort(reason: unknown): boolean {
  return reason instanceof DOMException && reason.name === 'AbortError';
}

function DeviceAuthorization() {
  const runtime = useRuntime();
  const deviceState = runtime.state.id === 'device-authorization' ? runtime.state : undefined;
  if (deviceState?.mode === 'generic') return <GenericDeviceAuthorization initialCode={deviceState.initialCode} nativeReturn={deviceState.nativeReturn === true} />;
  return <TVDeviceAuthorization initialCode={deviceState?.initialCode} servers={deviceState?.servers ?? []} />;
}

function TVDeviceAuthorization({ initialCode = '', servers }: { initialCode?: string; servers: HostedServerSummary[] }) {
  const runtime = useRuntime();
  const [code, setCode] = useState(normalizeDeviceCode(initialCode));
  const [preview, setPreview] = useState<{ deviceName: string; platform: string; appVersion?: string }>();
  const [previewContract, setPreviewContract] = useState<Awaited<ReturnType<typeof runtime.previewTVSetup>>>();
  const [selectedServerId, setSelectedServerId] = useState('');
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<ProductMessagePresentation>();
  const [busy, setBusy] = useState(false);
  const requestFence = useDeviceRequestFence(initialCode);
  const valid = code.replace('-', '').length === 8;
  const eligibleServers = servers.filter((server) => server.remoteAccessEnabled && server.preferredAuthMode === 'portico');
  const requiresServerChoice = eligibleServers.length > 1;
  const previewMatchesCode = Boolean(preview && previewContract && normalizeDeviceCode(previewContract.code) === normalizeDeviceCode(code));
  const review = async () => {
    const requestCode = normalizeDeviceCode(code);
    if (requestCode.replace('-', '').length !== 8) return;
    const ticket = requestFence.begin(requestCode);
    setBusy(true); setError(undefined);
    try {
      const result = await runtime.previewTVSetup(requestCode, ticket.controller.signal);
      if (!requestFence.current(ticket)) return;
      if (normalizeDeviceCode(result.code) !== requestCode) throw new TypeError('Portico returned a preview for a different TV setup code.');
      setPreview(result);
      setPreviewContract(result);
    }
    catch (reason) { if (requestFence.current(ticket) && !cleanRequestAbort(reason)) { setPreview(undefined); setPreviewContract(undefined); setError(canonicalProblem(reason)); } }
    finally { if (requestFence.current(ticket)) setBusy(false); }
  };
  const authorize = async () => {
    const requestCode = normalizeDeviceCode(code);
    if (!previewContract || (requiresServerChoice && !selectedServerId) || normalizeDeviceCode(previewContract.code) !== requestCode) return;
    const ticket = requestFence.begin(requestCode);
    setBusy(true); setError(undefined);
    try {
      await runtime.authorizeTVSetup(previewContract, selectedServerId, ticket.controller.signal);
      if (!requestFence.current(ticket)) return;
      setConnected(true);
    } catch (reason) { if (requestFence.current(ticket) && !cleanRequestAbort(reason)) setError(canonicalProblem(reason)); }
    finally { if (requestFence.current(ticket)) setBusy(false); }
  };
  useEffect(() => { if (valid && initialCode) void review(); }, []);
  useEffect(() => {
    if (eligibleServers.length === 1) setSelectedServerId(eligibleServers[0].id);
    else if (!eligibleServers.some((server) => server.id === selectedServerId)) setSelectedServerId('');
  }, [eligibleServers.map((server) => server.id).join('\u0000')]);
  if (connected) return <RuntimePanel title="TV connected" icon={<StatusSecureIcon />}>
    <p className="runtime-intro">Return to your TV. Portico will finish signing in automatically.</p>
  </RuntimePanel>;
  return <RuntimePanel title="Connect a device">
    <p className="runtime-intro">Enter the eight-character code shown on your TV, or review the code filled in from its QR code.</p>
    <form onSubmit={(event) => { event.preventDefault(); void (previewMatchesCode ? authorize() : review()); }}>
      <label><span>Device code</span><input autoFocus={!initialCode} inputMode="text" autoComplete="one-time-code" autoCapitalize="characters" spellCheck={false} placeholder="XXXX-XXXX" minLength={9} maxLength={9} pattern="[A-HJ-KM-NP-Z2-9]{4}-[A-HJ-KM-NP-Z2-9]{4}" value={code} onChange={(event) => { const nextCode = normalizeDeviceCode(event.target.value); requestFence.invalidate(nextCode); setCode(nextCode); setBusy(false); setPreview(undefined); setPreviewContract(undefined); setSelectedServerId(''); setError(undefined); }} required /></label>
      {previewMatchesCode && <div className="runtime-device-review"><strong>{preview!.deviceName}</strong><span>{[preview!.platform, preview!.appVersion].filter(Boolean).join(' · ')}</span><p>{eligibleServers.length > 1 ? 'Confirm this is your TV, then choose the Portico server it should open.' : 'Confirm this is your TV, then connect it to your Portico Account.'}</p></div>}
      {previewMatchesCode && eligibleServers.length > 1 && <div className="runtime-device-servers" role="radiogroup" aria-label="Portico server">{servers.map((server) => {
        const eligible = server.remoteAccessEnabled && server.preferredAuthMode === 'portico';
        return <button key={server.id} type="button" role="radio" aria-checked={selectedServerId === server.id} disabled={!eligible || busy} onClick={() => setSelectedServerId(server.id)}><DeviceServerIcon /><span><strong>{server.name}</strong><small>{eligible ? 'Available' : 'Portico Account remote access required'}</small></span></button>;
      })}</div>}
      {previewMatchesCode && eligibleServers.length === 0 && <p className="runtime-message warning">No servers are available yet. You can still sign in to this TV and add or accept access to a server later.</p>}
      {error && <ProductProblem presentation={error} />}
      <div className="runtime-actions"><PrimaryButton type="submit" disabled={busy || !valid || (previewMatchesCode && requiresServerChoice && !selectedServerId)}>{busy ? <><StatusLoadingIcon className="runtime-spinner" /> Please wait…</> : previewMatchesCode ? 'Connect TV' : 'Review request'}</PrimaryButton></div>
    </form>
  </RuntimePanel>;
}

function GenericDeviceAuthorization({ initialCode = '', nativeReturn = false }: { initialCode?: string; nativeReturn?: boolean }) {
  const runtime = useRuntime();
  const [code, setCode] = useState(normalizeDeviceCode(initialCode));
  const [preview, setPreview] = useState<{ deviceName: string; platform: string; appVersion?: string }>();
  const [previewedCode, setPreviewedCode] = useState('');
  const [finished, setFinished] = useState<'approved' | 'denied'>();
  const [error, setError] = useState<ProductMessagePresentation>();
  const [busy, setBusy] = useState(false);
  const requestFence = useDeviceRequestFence(initialCode);
  const valid = code.replace('-', '').length === 8;
  const previewMatchesCode = Boolean(preview && previewedCode === normalizeDeviceCode(code));
  const review = async () => {
    const requestCode = normalizeDeviceCode(code);
    if (requestCode.replace('-', '').length !== 8) return;
    const ticket = requestFence.begin(requestCode);
    setBusy(true); setError(undefined);
    try {
      const result = await runtime.previewGenericDeviceAuthorization(requestCode, ticket.controller.signal);
      if (!requestFence.current(ticket)) return;
      setPreview(result); setPreviewedCode(requestCode);
    }
    catch (reason) { if (requestFence.current(ticket) && !cleanRequestAbort(reason)) { setPreview(undefined); setPreviewedCode(''); setError(canonicalProblem(reason)); } }
    finally { if (requestFence.current(ticket)) setBusy(false); }
  };
  const decide = async (decision: 'approve' | 'deny') => {
    const requestCode = normalizeDeviceCode(code);
    if (!preview || previewedCode !== requestCode) return;
    const ticket = requestFence.begin(requestCode);
    setBusy(true); setError(undefined);
    try {
      const result = await runtime.decideGenericDeviceAuthorization(requestCode, decision, ticket.controller.signal);
      if (requestFence.current(ticket)) {
        setFinished(result.status);
        const completionURL = nativeDeviceAuthorizationCompletionURL(nativeReturn && result.status === 'approved');
        if (completionURL) window.location.assign(completionURL);
      }
    }
    catch (reason) { if (requestFence.current(ticket) && !cleanRequestAbort(reason)) setError(canonicalProblem(reason)); }
    finally { if (requestFence.current(ticket)) setBusy(false); }
  };
  useEffect(() => { if (valid && initialCode) void review(); }, []);
  if (finished) return <RuntimePanel title={finished === 'approved' ? 'Device connected' : 'Request denied'} icon={<StatusSecureIcon />}><p className="runtime-intro">{finished === 'approved' ? 'Return to your device. Portico will finish signing in automatically.' : 'The device was not granted access.'}</p></RuntimePanel>;
  return <RuntimePanel title="Authorize a device">
    <p className="runtime-intro">Enter the eight-character code shown on your device, then confirm the request before granting access.</p>
    <form onSubmit={(event) => { event.preventDefault(); void (previewMatchesCode ? decide('approve') : review()); }}>
      <label><span>Device code</span><input autoFocus={!initialCode} inputMode="text" autoComplete="one-time-code" autoCapitalize="characters" spellCheck={false} placeholder="XXXX-XXXX" minLength={9} maxLength={9} pattern="[A-HJ-KM-NP-Z2-9]{4}-[A-HJ-KM-NP-Z2-9]{4}" value={code} onChange={(event) => { const nextCode = normalizeDeviceCode(event.target.value); requestFence.invalidate(nextCode); setCode(nextCode); setBusy(false); setPreview(undefined); setPreviewedCode(''); setError(undefined); }} required /></label>
      {previewMatchesCode && <div className="runtime-device-review"><strong>{preview!.deviceName}</strong><span>{[preview!.platform, preview!.appVersion].filter(Boolean).join(' · ')}</span><p>Confirm this is the device requesting access before you continue.</p></div>}
      {error && <ProductProblem presentation={error} />}
      <div className="runtime-actions"><PrimaryButton type="submit" disabled={busy || !valid}>{busy ? <><StatusLoadingIcon className="runtime-spinner" /> Please wait…</> : previewMatchesCode ? 'Approve device' : 'Review request'}</PrimaryButton>{previewMatchesCode && <SecondaryButton disabled={busy} onClick={() => void decide('deny')}>Deny</SecondaryButton>}</div>
    </form>
  </RuntimePanel>;
}

function NoMemberships({ embedded = false }: { embedded?: boolean }) {
  const runtime = useRuntime();
  const [mode, setMode] = useState<'claim' | 'invite'>('claim');
  const [value, setValue] = useState('');
  const [error, setError] = useState<ProductMessagePresentation>();
  const run = async () => {
    setError(undefined);
    try {
      if (mode === 'claim') await runtime.claimServer(value.trim());
      else await runtime.acceptInvite(value.trim());
    } catch (reason) { setError(canonicalProblem(reason)); }
  };
  return <RuntimePanel title="No servers yet" icon={<DeviceOfflineIcon />} embedded={embedded}>
    <p className="runtime-intro">Claim a server you own, or accept an invitation from another server owner.</p>
    <div className="runtime-segmented" role="tablist" aria-label="Add a server"><button type="button" role="tab" aria-selected={mode === 'claim'} className={mode === 'claim' ? 'active' : ''} onClick={() => { setMode('claim'); setValue(''); setError(undefined); }}>Claim a server</button><button type="button" role="tab" aria-selected={mode === 'invite'} className={mode === 'invite' ? 'active' : ''} onClick={() => { setMode('invite'); setValue(''); setError(undefined); }}>Accept an invite</button></div>
    <form className="runtime-membership-form" onSubmit={(event) => { event.preventDefault(); void run(); }}><label><span>{mode === 'claim' ? 'Server claim code' : 'Invitation code'}</span><input autoFocus value={value} onChange={(event) => setValue(event.target.value)} autoComplete="off" required /><small>{mode === 'claim' ? 'Find this one-time code on the server’s Portico claim screen.' : 'Invitation links fill this automatically. Paste the invitation token only when adding it manually.'}</small></label>{error && <ProductProblem presentation={error} />}<PrimaryButton type="submit" disabled={runtime.busy || !value.trim()}>{runtime.busy ? <><StatusLoadingIcon className="runtime-spinner" /> Please wait…</> : mode === 'claim' ? 'Claim server' : 'Accept invitation'}</PrimaryButton></form>
    <div className="runtime-footer-actions"><SecondaryButton disabled={runtime.busy} onClick={() => void runtime.refreshMemberships().catch(() => undefined)}><ActionRefreshIcon /> Refresh servers</SecondaryButton></div>
  </RuntimePanel>;
}

export function relativeHeartbeat(server: HostedServerSummary): string {
  if (!server.lastHeartbeatAt) return 'Never online';
  const heartbeat = Date.parse(server.lastHeartbeatAt);
  if (!Number.isFinite(heartbeat) || heartbeat < Date.UTC(2020, 0, 1)) return 'Never online';
  const elapsed = Date.now() - heartbeat;
  if (elapsed < -5 * 60_000) return 'Status unavailable';
  if (elapsed < 60_000) return 'Online now';
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 60) return `Last online ${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `Last online ${hours}h ago`;
  return `Last online ${Math.floor(hours / 24)}d ago`;
}

function ServerSelection() {
  const runtime = useRuntime();
  const [refreshError, setRefreshError] = useState<ProductMessagePresentation>();
  const [selectionError, setSelectionError] = useState<ProductMessagePresentation>();
  if (runtime.state.id !== 'server-selection') return null;
  const refresh = async () => {
    setRefreshError(undefined);
    try {
      await runtime.refreshMemberships();
    } catch (reason) {
      setRefreshError(canonicalProblem(reason));
    }
  };
  const select = async (server: HostedServerSummary) => {
    setSelectionError(undefined);
    try {
      await runtime.selectServer(server);
    } catch (reason) {
      setSelectionError(canonicalProblem(reason));
    }
  };
  return <RuntimePanel title="Choose a server" icon={<DeviceServerIcon />} wide={runtime.state.servers.length > 3}>
    <p className="runtime-intro">Only servers that allow Portico Account access can open from web.getportico.tv. Every connection is verified before server credentials are issued.</p>
    <div className="runtime-server-list">{runtime.state.servers.map((server) => {
      const eligible = server.remoteAccessEnabled && server.preferredAuthMode === 'portico';
      const status = !server.remoteAccessEnabled ? 'Remote Access is off' : server.preferredAuthMode !== 'portico' ? 'This Server sign-in only' : relativeHeartbeat(server);
      return <button key={server.id} onClick={() => void select(server)} disabled={runtime.busy || !eligible || runtime.canSelectHostedServer === false}><span className={`runtime-server-icon ${eligible ? '' : 'unavailable'}`}><DeviceServerIcon /></span><span><strong>{server.name}</strong><small>{status}</small></span>{eligible ? <NavigationForwardIcon /> : <StatusLockedIcon />}</button>;
    })}</div>
    {(refreshError || selectionError) && <ProductProblem presentation={(refreshError || selectionError)!} />}
    <div className="runtime-footer-actions"><SecondaryButton disabled={runtime.busy} onClick={() => void refresh()}><ActionRefreshIcon /> Refresh servers</SecondaryButton><button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button></div>
  </RuntimePanel>;
}

function ProfileSelection() {
  const runtime = useRuntime();
  const [selectedProfileId, setSelectedProfileId] = useState<string>();
  const [pin, setPin] = useState('');
  if (runtime.state.id !== 'profile-selection') return null;
  const selected = runtime.state.profiles.find((profile) => profile.id === selectedProfileId);
  const localLogin = Boolean(runtime.localLoginServerName);
  const open = async (profileId: string, profilePin?: string) => {
    await runtime.selectProfile(profileId, profilePin);
  };
  return <RuntimePanel title={profileSelectionRequired.title!} icon={<ProductStatusIcon icon={profileSelectionRequired.icon} />} wide={runtime.state.profiles.length > 4}>
    <p className="runtime-intro">{profileSelectionRequired.body}</p>
    <div className="runtime-profile-list" aria-label={productText('profiles.choose-title')}>{[...runtime.state.profiles].sort((left, right) => left.sortOrder - right.sortOrder).map((profile) => <button
      key={profile.id}
      type="button"
      disabled={runtime.busy}
      aria-pressed={selectedProfileId === profile.id}
      onClick={() => {
        setPin('');
        if (profile.hasPIN) setSelectedProfileId(profile.id);
        else void open(profile.id);
      }}
    ><span className="runtime-profile-avatar">{profile.name.trim().slice(0, 1).toLocaleUpperCase() || 'P'}</span><span><strong>{profile.name}</strong><small>{profile.hasPIN ? 'PIN required' : 'Open profile'}</small></span><NavigationForwardIcon /></button>)}</div>
    {selected?.hasPIN && <form className="runtime-profile-pin" onSubmit={(event) => { event.preventDefault(); void open(selected.id, pin); }}>
      <label><span>Enter {selected.name}’s PIN</span><input autoFocus inputMode="numeric" autoComplete="one-time-code" minLength={4} maxLength={4} pattern="[0-9]{4}" value={pin} onChange={(event) => setPin(event.target.value.replace(/\D/g, '').slice(0, 4))} required /></label>
      <PrimaryButton type="submit" disabled={runtime.busy || pin.length !== 4}>{runtime.busy ? <><StatusLoadingIcon className="runtime-spinner" /> Opening…</> : 'Open profile'}</PrimaryButton>
    </form>}
    {runtime.state.messageId && <ProductProblem presentation={productMessage(runtime.state.messageId)} />}
    <div className="runtime-footer-actions">
      {localLogin
        ? <><SecondaryButton disabled={runtime.busy} onClick={runtime.retry}><ActionRefreshIcon /> Refresh profiles</SecondaryButton><button type="button" className="runtime-text-action" onClick={() => void runtime.continueWithHostedAccount()}>Continue to Portico</button></>
        : <SecondaryButton onClick={runtime.reselectServer}><NavigationBackIcon /> Choose another server</SecondaryButton>}
      <button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button>
    </div>
  </RuntimePanel>;
}

function RuntimeRecovery({ embedded = false }: { embedded?: boolean }) {
  const runtime = useRuntime();
  const recovery = runtime.state.id === 'runtime-recovery' ? runtime.state : undefined;
  const availabilityReason = useMemo(() => recovery?.automaticAvailabilityRetry ? {
    retryAfterMs: recovery.availabilityRetryAfterMs,
    retryAt: recovery.availabilityRetryAt,
  } : undefined, [recovery?.automaticAvailabilityRetry, recovery?.availabilityRetryAfterMs, recovery?.availabilityRetryAt]);
  const availability = useHostedAvailabilityRetry({
    enabled: recovery?.automaticAvailabilityRetry === true,
    reason: availabilityReason,
    retry: runtime.retry,
    cohort: runtime.hostedRetryCohort,
    failureStartedAt: runtime.hostedAvailabilityFailureStartedAt,
  });
  if (!recovery) return null;
  if (recovery.automaticRoutePublicationRetry) {
    return <RuntimeProgress
      title="Finishing server setup"
      body={`Portico is preparing a secure connection to ${recovery.serverName || 'this server'}. This page will continue automatically.`}
      kind="route"
      serverName={recovery.serverName}
      embedded={embedded}
    />;
  }
  if (availability.automatic && !availability.showWarning) {
    return <RuntimeProgress
      title="Opening Portico"
      body="Still checking your Portico Account…"
      kind="account"
      embedded={embedded}
    />;
  }
  const serverName = recovery.serverName || 'this server';
  const copy = productMessage(recovery.messageId, { serverName });
  const needsSignIn = recovery.recoveryActions.includes('sign-in');
  const title = availability.automatic ? availability.copy.title : copy.title ?? 'Portico';
  const body = availability.automatic ? availability.copy.body : copy.body ?? copy.text;
  return <RuntimePanel title={title} icon={<ProductStatusIcon icon={copy.icon} />} embedded={embedded}>
    <p className="runtime-intro">{body}</p>
    <div className="runtime-actions">
      {needsSignIn && <PrimaryButton onClick={() => void runtime.hostedLogout()}><AccountSignInIcon /> Sign in again</PrimaryButton>}
      {recovery.recoveryActions.includes('try-nearby') && <PrimaryButton onClick={() => void runtime.tryNearbyConnection()}><DeviceWifiIcon /> Try nearby connection</PrimaryButton>}
      {!needsSignIn && !availability.automatic && recovery.recoveryActions.includes('retry') && <PrimaryButton onClick={runtime.retry}><ActionRefreshIcon /> Try again</PrimaryButton>}
      {recovery.recoveryActions.includes('reselect-server') && <SecondaryButton onClick={runtime.reselectServer}><NavigationBackIcon /> Choose another server</SecondaryButton>}
      {!availability.automatic && recovery.recoveryActions.includes('refresh-memberships') && <SecondaryButton onClick={() => void runtime.refreshMemberships().catch(() => undefined)}><ActionRefreshIcon /> Refresh servers</SecondaryButton>}
      {recovery.recoveryActions.includes('continue-account') && <SecondaryButton onClick={() => void runtime.continueWithHostedAccount()}><NavigationForwardIcon /> Continue to Portico</SecondaryButton>}
      {!needsSignIn && recovery.recoveryActions.includes('sign-out') && <button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button>}
    </div>
  </RuntimePanel>;
}

export function RuntimeSurface({ embedded = false }: { embedded?: boolean }) {
  const runtime = useRuntime();
  switch (runtime.state.id) {
    case 'runtime-config':
      return <RuntimeProgress title="Starting Portico" body="Loading the web application configuration…" kind="configuration" />;
    case 'checking-local-server':
      return <RuntimeProgress title={`Connecting to ${runtime.state.serverName}`} body="Checking the local server and your browser session…" kind="local" serverName={runtime.state.serverName} embedded={embedded} />;
    case 'hosted-account-session':
      return <RuntimeProgress title="Opening Portico" body="Checking your Portico Account session…" kind="account" embedded={embedded} />;
    case 'hosted-sign-in':
      return <HostedSignIn />;
    case 'local-login-recovery':
      return <LocalLoginRecovery />;
    case 'sso-onboarding':
      return <SSOOnboarding />;
    case 'device-authorization':
      return <DeviceAuthorization />;
    case 'server-memberships':
      return <RuntimeProgress title="Finding your servers" body="Loading servers shared with your Portico Account…" kind="memberships" embedded={embedded} />;
    case 'no-memberships':
      return <NoMemberships embedded={embedded} />;
    case 'server-selection':
      return <ServerSelection />;
    case 'profile-selection':
      return <ProfileSelection />;
    case 'route-discovery':
      return <RuntimeProgress title={`Connecting to ${runtime.state.selectedServer.name}`} body="Discovering and verifying a secure direct route…" kind="route" serverName={runtime.state.selectedServer.name} embedded={embedded} />;
    case 'runtime-recovery':
      return <RuntimeRecovery embedded={embedded} />;
    case 'server-ready':
      return null;
  }
}
