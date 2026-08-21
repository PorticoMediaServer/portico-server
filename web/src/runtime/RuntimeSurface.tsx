import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  CircleUserRound,
  Globe2,
  LoaderCircle,
  LockKeyhole,
  LogIn,
  RefreshCw,
  Server,
  ServerOff,
  ShieldCheck,
  Wifi,
  WifiOff,
} from '#portico-icons';
import {
  productMessage,
  resolveProductProblem,
  validPorticoUsername,
  type ProductMessageId,
  type ProductMessagePresentation,
  type SemanticIconId,
} from '@portico/client-core';
import { type FormEvent, type ReactNode, useEffect, useRef, useState } from 'react';
import { PrimaryButton, SecondaryButton } from '../components/controls/Buttons';
import { PasswordInput, PasswordRequirements, validPorticoPassword } from '../components/controls/PasswordInput';
import { productText } from '../components/ProductLanguage';
import type { HostedServerSummary } from './runtimeMachine';
import { useRuntime } from './RuntimeContext';

const profileSelectionRequired = productMessage('auth.profile-selection-required');

function ProductStatusIcon({ icon }: { icon?: SemanticIconId }) {
  switch (icon) {
    case 'status.locked':
      return <LockKeyhole data-semantic-icon={icon} />;
    case 'status.offline':
      return <WifiOff data-semantic-icon={icon} />;
    case 'status.profile':
      return <CircleUserRound data-semantic-icon={icon} />;
    case 'status.server':
      return <ServerOff data-semantic-icon={icon} />;
    case 'status.loading':
      return <LoaderCircle data-semantic-icon={icon} />;
    default:
      return <AlertTriangle data-semantic-icon={icon ?? 'status.error'} />;
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
  const icon = kind === 'route' ? <Globe2 /> : kind === 'memberships' ? <Server /> : <LoaderCircle />;
  // Ordinary connection work is intentionally silent inside the product
  // frame. The reserved projection area prevents a flash or layout shift;
  // only an exhausted/terminal recovery state becomes user-facing UI.
  if (embedded) return <div className="runtime-content-reservation" aria-busy="true" aria-label={title} />;
  return <RuntimePanel title={title} icon={icon}>
    <p className="runtime-intro" aria-live="polite">{delayed ? (kind === 'route' ? `The direct route to ${serverName || 'this server'} is taking longer than expected.` : 'This step is taking longer than expected.') : body}</p>
    <div className="runtime-progress-line" aria-hidden="true"><span /></div>
    {kind === 'route' && <div className="runtime-connection-facts"><span><ShieldCheck />HTTPS and server identity must verify</span><span><Globe2 />Playback connects directly to your server</span></div>}
    {delayed && kind !== 'configuration' && <div className="runtime-actions"><PrimaryButton onClick={runtime.retry}><RefreshCw /> Try again</PrimaryButton>{kind === 'route' && <SecondaryButton onClick={runtime.reselectServer}><ArrowLeft /> Choose another server</SecondaryButton>}{(kind === 'account' || kind === 'memberships') && <button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button>}</div>}
    {!delayed && kind === 'route' && <SecondaryButton onClick={runtime.reselectServer}><ArrowLeft /> Choose another server</SecondaryButton>}
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
    setMode(next); setError(undefined); setResetSent(false); setMfaCode(''); setRecoveryCode(''); setUseRecoveryCode(false);
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
  const claimServerName = runtime.serverClaimName;
  const title = mode === 'register' ? (claimSetup ? (claimServerName ? `Create an account to continue setting up “${claimServerName}”` : 'Create an account to continue') : productText('account.create-title')) : mode === 'request-reset' ? 'Recover your Portico Account' : mode === 'complete-reset' ? 'Choose a new password' : runtime.mfaRequired ? (claimServerName ? `Verify your sign-in for “${claimServerName}”` : 'Verify your sign-in') : claimSetup ? (claimServerName ? `Continue setting up your server “${claimServerName}”` : 'Continue setting up your server') : 'Sign in to Portico';
  const intro = mode === 'register' ? (claimSetup ? 'Create a Portico Account to continue with server setup.' : productText('account.create-intro')) : mode === 'request-reset' ? 'Enter your account email. If it matches an account, Portico will send a secure recovery link.' : mode === 'complete-reset' ? 'Choose a new password for your Portico Account. The recovery token has already been removed from the address bar.' : runtime.mfaRequired ? 'Enter the code from your authenticator, or use one of your recovery codes.' : claimSetup ? 'Sign in or create a Portico Account to continue with server setup.' : 'Sign in using your Portico Account credentials.';
  if (resetSent) return <RuntimePanel title="Check your email"><p className="runtime-intro">If an account matches <strong>{email}</strong>, recovery instructions are on the way. The link expires and can be used only once.</p><div className="runtime-actions"><PrimaryButton onClick={() => changeMode('sign-in')}><ArrowLeft /> Back to sign in</PrimaryButton><SecondaryButton disabled={runtime.busy} onClick={() => setResetSent(false)}>Send again</SecondaryButton></div></RuntimePanel>;
  return <RuntimePanel title={title}>
    <p className="runtime-intro">{intro}</p>
    <form onSubmit={submit}>
      {mode === 'register' && <label><span>Username</span><input autoFocus autoCapitalize="none" autoComplete="username" minLength={3} maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,30}[A-Za-z0-9]" value={username} onChange={(event) => setUsername(event.target.value)} required /><small>3–32 letters, numbers, periods, underscores, or hyphens.</small></label>}
      {mode !== 'complete-reset' && <label><span>{mode === 'sign-in' ? 'Username or email' : 'Email'}</span><input autoFocus={mode !== 'register'} type={mode === 'sign-in' ? 'text' : 'email'} autoCapitalize="none" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>}
      {(mode === 'sign-in' || mode === 'register' || mode === 'complete-reset') && <label><span>{mode === 'complete-reset' ? 'New password' : 'Password'}</span><PasswordInput aria-label={mode === 'complete-reset' ? 'New password' : 'Password'} autoComplete={mode === 'sign-in' ? 'current-password' : 'new-password'} minLength={mode === 'sign-in' ? undefined : 8} maxLength={72} value={password} onChange={(event) => setPassword(event.target.value)} required aria-describedby={mode !== 'sign-in' ? 'runtime-password-requirements' : undefined} />{mode !== 'sign-in' && <PasswordRequirements id="runtime-password-requirements" value={password} />}</label>}
      {mode === 'sign-in' && runtime.mfaRequired && <div className="runtime-mfa"><label><span>{useRecoveryCode ? 'Recovery code' : 'Verification code'}</span><input autoFocus inputMode={useRecoveryCode ? 'text' : 'numeric'} autoComplete="one-time-code" value={useRecoveryCode ? recoveryCode : mfaCode} onChange={(event) => useRecoveryCode ? setRecoveryCode(event.target.value) : setMfaCode(event.target.value.replace(/\s/g, ''))} required /></label><button type="button" className="runtime-text-action" onClick={() => { setUseRecoveryCode((value) => !value); setMfaCode(''); setRecoveryCode(''); }}>{useRecoveryCode ? 'Use an authenticator code' : 'Use a recovery code'}</button></div>}
      {canonicalNotice && <p className="runtime-message warning" role="alert"><ProductStatusIcon icon={canonicalNotice.icon} /><span><strong>{canonicalNotice.title}</strong>{canonicalNotice.body}</span></p>}
      {error && <ProductProblem presentation={error} />}
      <PrimaryButton type="submit" disabled={runtime.busy || (mode === 'sign-in' && runtime.mfaRequired && !(useRecoveryCode ? recoveryCode.trim() : mfaCode.trim()))}>{runtime.busy ? <><LoaderCircle className="runtime-spinner" /> Please wait…</> : mode === 'register' ? 'Create account' : mode === 'request-reset' ? 'Send recovery email' : mode === 'complete-reset' ? 'Update password' : runtime.mfaRequired ? 'Verify and sign in' : 'Sign in'}</PrimaryButton>
      {mode === 'sign-in' ? <div className="runtime-form-links"><button type="button" className="runtime-text-action" onClick={() => changeMode('request-reset')}>Forgot password?</button><button type="button" className="runtime-text-action" onClick={() => changeMode('register')}>Create an account</button></div> : mode !== 'complete-reset' && <button type="button" className="runtime-text-action runtime-back-link" onClick={() => changeMode('sign-in')}><ArrowLeft /> Back to sign in</button>}
    </form>
  </RuntimePanel>;
}

function NoMemberships() {
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
  return <RuntimePanel title="No servers yet" icon={<ServerOff />}>
    <p className="runtime-intro">Claim a server you own, or accept an invitation from another server owner.</p>
    <div className="runtime-segmented" role="tablist" aria-label="Add a server"><button type="button" role="tab" aria-selected={mode === 'claim'} className={mode === 'claim' ? 'active' : ''} onClick={() => { setMode('claim'); setValue(''); setError(undefined); }}>Claim a server</button><button type="button" role="tab" aria-selected={mode === 'invite'} className={mode === 'invite' ? 'active' : ''} onClick={() => { setMode('invite'); setValue(''); setError(undefined); }}>Accept an invite</button></div>
    <form className="runtime-membership-form" onSubmit={(event) => { event.preventDefault(); void run(); }}><label><span>{mode === 'claim' ? 'Server claim code' : 'Invitation code'}</span><input autoFocus value={value} onChange={(event) => setValue(event.target.value)} autoComplete="off" required /><small>{mode === 'claim' ? 'Find this one-time code on the server’s Portico claim screen.' : 'Invitation links fill this automatically. Paste the invitation token only when adding it manually.'}</small></label>{error && <ProductProblem presentation={error} />}<PrimaryButton type="submit" disabled={runtime.busy || !value.trim()}>{runtime.busy ? <><LoaderCircle className="runtime-spinner" /> Please wait…</> : mode === 'claim' ? 'Claim server' : 'Accept invitation'}</PrimaryButton></form>
    <div className="runtime-footer-actions"><SecondaryButton disabled={runtime.busy} onClick={() => void runtime.refreshMemberships().catch(() => undefined)}><RefreshCw /> Refresh servers</SecondaryButton><button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button></div>
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
  if (runtime.state.id !== 'server-selection') return null;
  return <RuntimePanel title="Choose a server" icon={<Server />} wide={runtime.state.servers.length > 3}>
    <p className="runtime-intro">Only servers that allow Portico Account access can open from web.getportico.tv. Every connection is verified before server credentials are issued.</p>
    <div className="runtime-server-list">{runtime.state.servers.map((server) => {
      const eligible = server.remoteAccessEnabled && server.preferredAuthMode === 'portico';
      const status = !server.remoteAccessEnabled ? 'Remote Access is off' : server.preferredAuthMode !== 'portico' ? 'This Server sign-in only' : relativeHeartbeat(server);
      return <button key={server.id} onClick={() => void runtime.selectServer(server)} disabled={runtime.busy || !eligible}><span className={`runtime-server-icon ${eligible ? '' : 'unavailable'}`}><Server /></span><span><strong>{server.name}</strong><small>{status}</small></span>{eligible ? <ArrowRight /> : <LockKeyhole />}</button>;
    })}</div>
    <div className="runtime-footer-actions"><SecondaryButton disabled={runtime.busy} onClick={() => void runtime.refreshMemberships().catch(() => undefined)}><RefreshCw /> Refresh servers</SecondaryButton><button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button></div>
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
    ><span className="runtime-profile-avatar">{profile.name.trim().slice(0, 1).toLocaleUpperCase() || 'P'}</span><span><strong>{profile.name}</strong><small>{profile.hasPIN ? 'PIN required' : 'Open profile'}</small></span><ArrowRight /></button>)}</div>
    {selected?.hasPIN && <form className="runtime-profile-pin" onSubmit={(event) => { event.preventDefault(); void open(selected.id, pin); }}>
      <label><span>Enter {selected.name}’s PIN</span><input autoFocus inputMode="numeric" autoComplete="one-time-code" minLength={4} maxLength={4} pattern="[0-9]{4}" value={pin} onChange={(event) => setPin(event.target.value.replace(/\D/g, '').slice(0, 4))} required /></label>
      <PrimaryButton type="submit" disabled={runtime.busy || pin.length !== 4}>{runtime.busy ? <><LoaderCircle className="runtime-spinner" /> Opening…</> : 'Open profile'}</PrimaryButton>
    </form>}
    {runtime.state.messageId && <ProductProblem presentation={productMessage(runtime.state.messageId)} />}
    <div className="runtime-footer-actions">
      {localLogin
        ? <><SecondaryButton disabled={runtime.busy} onClick={runtime.retry}><RefreshCw /> Refresh profiles</SecondaryButton><button type="button" className="runtime-text-action" onClick={() => void runtime.continueWithHostedAccount()}>Continue to Portico</button></>
        : <SecondaryButton onClick={runtime.reselectServer}><ArrowLeft /> Choose another server</SecondaryButton>}
      <button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button>
    </div>
  </RuntimePanel>;
}

function RuntimeRecovery({ embedded = false }: { embedded?: boolean }) {
  const runtime = useRuntime();
  if (runtime.state.id !== 'runtime-recovery') return null;
  const serverName = runtime.state.serverName || 'this server';
  const copy = productMessage(runtime.state.messageId, { serverName });
  const needsSignIn = runtime.state.recoveryActions.includes('sign-in');
  return <RuntimePanel title={copy.title ?? 'Portico'} icon={<ProductStatusIcon icon={copy.icon} />} embedded={embedded}>
    <p className="runtime-intro">{copy.body ?? copy.text}</p>
    <div className="runtime-actions">
      {needsSignIn && <PrimaryButton onClick={() => void runtime.hostedLogout()}><LogIn /> Sign in again</PrimaryButton>}
      {runtime.state.recoveryActions.includes('try-nearby') && <PrimaryButton onClick={() => void runtime.tryNearbyConnection()}><Wifi /> Try nearby connection</PrimaryButton>}
      {!needsSignIn && runtime.state.recoveryActions.includes('retry') && <PrimaryButton onClick={runtime.retry}><RefreshCw /> Try again</PrimaryButton>}
      {runtime.state.recoveryActions.includes('reselect-server') && <SecondaryButton onClick={runtime.reselectServer}><ArrowLeft /> Choose another server</SecondaryButton>}
      {runtime.state.recoveryActions.includes('refresh-memberships') && <SecondaryButton onClick={() => void runtime.refreshMemberships().catch(() => undefined)}><RefreshCw /> Refresh servers</SecondaryButton>}
      {runtime.state.recoveryActions.includes('continue-account') && <SecondaryButton onClick={() => void runtime.continueWithHostedAccount()}><ArrowRight /> Continue to Portico</SecondaryButton>}
      {!needsSignIn && runtime.state.recoveryActions.includes('sign-out') && <button type="button" className="runtime-text-action" onClick={() => void runtime.hostedLogout()}>Sign out</button>}
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
    case 'server-memberships':
      return <RuntimeProgress title="Finding your servers" body="Loading servers shared with your Portico Account…" kind="memberships" embedded={embedded} />;
    case 'no-memberships':
      return <NoMemberships />;
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
