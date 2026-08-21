import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Globe2,
  KeyRound,
  LoaderCircle,
  RefreshCw,
  Server,
  ShieldCheck,
	UserPlus,
  WifiOff,
} from '#portico-icons';
import {
	productMessage,
	resolveProductProblem,
	type ProductMessageId,
} from '@porticomediaserver/client-core';
import { type FormEvent, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { PasswordInput, PasswordRequirements, validPorticoPassword } from '../../components/controls/PasswordInput';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { useAuthSession } from '../../data/DataProvider';
import { useOptionalRuntime } from '../../runtime/RuntimeContext';
import './browser-accounts.css';

function AuthBrand() {
  return <img className="auth-wordmark" src="/brand/portico-wordmark-white.svg" alt="Portico" />;
}

function ServerIdentity({ serverName, detail }: { serverName: string; detail: string }) {
  return <div className="auth-server"><Server /><span><strong>{serverName}</strong><small>{detail}</small></span></div>;
}

function productErrorText(reason: unknown, fallback: ProductMessageId): string {
	const candidate = reason && typeof reason === 'object'
		? reason as { code?: unknown; messageId?: unknown; status?: unknown; details?: unknown }
		: undefined;
	const resolved = candidate ? resolveProductProblem({
		...(typeof candidate.code === 'string' ? { code: candidate.code } : {}),
		...(typeof candidate.messageId === 'string' ? { messageId: candidate.messageId } : {}),
		...(typeof candidate.status === 'number' ? { status: candidate.status } : {}),
		...(candidate.details && typeof candidate.details === 'object' ? { details: candidate.details as Readonly<Record<string, unknown>> } : {}),
	}) : productMessage(fallback);
	const presentation = resolved.id === 'problem.request-failed' && fallback !== 'problem.request-failed'
		? productMessage(fallback)
		: resolved;
	return presentation.body ?? presentation.text ?? presentation.title ?? productMessage('problem.request-failed').body ?? 'Portico could not complete this request.';
}

function setupClaimErrorText(reason: unknown): string {
	const candidate = reason && typeof reason === 'object'
		? reason as { code?: unknown; detail?: unknown }
		: undefined;
	// Problem details returned by this endpoint are deliberately written for the
	// person performing setup (timeouts, Hosted rejection, invalid name, and so
	// on). Preserve that actionable explanation instead of collapsing distinct
	// failures into the shared request-failed fallback.
	if (typeof candidate?.detail === 'string'
		&& candidate.detail.trim()) {
		return candidate.detail.trim();
	}
	return productErrorText(reason, 'problem.request-failed');
}

const demoMode = import.meta.env.VITE_PORTICO_DEMO_MODE === 'true';
const demoLogin = String(import.meta.env.VITE_PORTICO_DEMO_PUBLIC_USERNAME ?? '').trim();
const demoPassword = String(import.meta.env.VITE_PORTICO_DEMO_PUBLIC_PASSWORD ?? '');

export function AuthLoadingSurface({ title = 'Opening your server', detail = 'Checking your session and server access…' }: { title?: string; detail?: string }) {
  const auth = useAuthSession();
  const runtime = useOptionalRuntime();
  const [delayed, setDelayed] = useState(false);
  useEffect(() => {
    const timer = window.setTimeout(() => setDelayed(true), 8_000);
    return () => window.clearTimeout(timer);
  }, []);
  return <main className="auth-surface"><section className="auth-panel auth-loading" aria-busy={!delayed} aria-live="polite">
    <AuthBrand />
    <div className="auth-loading-state"><LoaderCircle /><span><strong>{title}</strong><p>{delayed ? 'The server is taking longer than expected to answer.' : detail}</p></span></div>
    {delayed && <div className="auth-recovery-actions"><PrimaryButton onClick={auth.refresh}><RefreshCw /> Try again</PrimaryButton>{runtime?.config.mode === 'hosted' && <SecondaryButton onClick={runtime.disconnectServer}><ArrowLeft /> Choose another server</SecondaryButton>}</div>}
  </section></main>;
}

export function AuthFailureSurface({ message, onRetry }: { message: string; onRetry?: () => void }) {
  const auth = useAuthSession();
  const runtime = useOptionalRuntime();
  const fallback = productMessage('auth.server-session-load-failed');
  return <main className="auth-surface">
    <section className="auth-panel auth-problem" role="alert">
      <AuthBrand />
      <div className="auth-problem-icon"><WifiOff /></div>
      <h1>{fallback.title}</h1>
      <p>{message || fallback.body}</p>
      <div className="auth-recovery-actions"><PrimaryButton onClick={onRetry ?? auth.refresh}><RefreshCw /> Try again</PrimaryButton>{runtime?.config.mode === 'hosted' && <SecondaryButton onClick={runtime.disconnectServer}><ArrowLeft /> Choose another server</SecondaryButton>}</div>
    </section>
  </main>;
}

function accountLastUsed(value: string): string {
	const elapsed = Date.now() - Date.parse(value);
	if (!Number.isFinite(elapsed) || elapsed < 60_000) return 'Used just now';
	const minutes = Math.floor(elapsed / 60_000);
	if (minutes < 60) return `Used ${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `Used ${hours}h ago`;
	const days = Math.floor(hours / 24);
	return days === 1 ? 'Used yesterday' : `Used ${days}d ago`;
}

export function AccountChooserSurface({ serverName }: { serverName: string }) {
	const auth = useAuthSession();
	const navigate = useNavigate();
	const { data, error } = auth.browserAccounts;
	const [switchError, setSwitchError] = useState('');
	const openAccount = async (accountId: string) => {
		setSwitchError('');
		try {
			await auth.switchBrowserAccount(accountId);
			navigate('/home', { replace: true });
		}
		catch (reason) { setSwitchError(productErrorText(reason, 'problem.request-failed')); }
	};
	return <main className="auth-surface">
		<section className="auth-panel account-chooser-panel">
			<AuthBrand />
			<ServerIdentity serverName={serverName} detail="Choose an account" />
			<header className="auth-heading"><h1>Who’s watching?</h1><p>Choose an account remembered for this server.</p></header>
			{(switchError || error) && <p className="auth-error account-chooser-error" role="alert"><AlertTriangle />{switchError || reviewedProductErrorText(error, 'auth.browser-accounts-load-failed')}</p>}
			<div className="account-chooser-list">{data.accounts.map((account) => {
				const initial = account.displayName.trim().slice(0, 1).toLocaleUpperCase() || 'P';
				return <button key={account.id} type="button" disabled={auth.busy} onClick={() => void openAccount(account.id)}>
					<span className="account-chooser-avatar">{account.profileImageUrl ? <img src={account.profileImageUrl} alt="" /> : initial}</span>
					<span><strong>{account.displayName}</strong><small>{account.authProvider === 'portico' ? 'Portico Account' : 'This Server'} · {accountLastUsed(account.lastUsedAt)}</small></span>
					<ArrowRight />
				</button>;
			})}</div>
			<div className="account-chooser-actions">
				{data.canAddAccount && <SecondaryButton disabled={auth.busy} onClick={auth.beginAddAccount}><UserPlus /> Add account</SecondaryButton>}
				{error && <button className="auth-text-button" type="button" disabled={auth.busy} onClick={auth.retryBrowserAccounts}><RefreshCw /> Reload accounts</button>}
			</div>
		</section>
	</main>;
}

export function LocalProfileSelectionSurface({ serverName }: { serverName: string }) {
	const auth = useAuthSession();
	const challenge = auth.localProfileLogin;
	const selectionCopy = productMessage('auth.profile-selection-required');
	const [selectedProfileId, setSelectedProfileId] = useState<string>();
	const [pin, setPin] = useState('');
	const [error, setError] = useState('');
	if (!challenge) return null;
	const profiles = [...challenge.directory.profiles].sort((left, right) => left.sortOrder - right.sortOrder);
	const selected = profiles.find((profile) => profile.id === selectedProfileId);
	const open = async (profileId: string, profilePin?: string) => {
		setError('');
		try { await auth.selectLocalProfile(profileId, profilePin); }
		catch (reason) {
			if (reason instanceof DOMException && reason.name === 'AbortError') return;
			setError(productErrorText(reason, 'auth.profile-selection-failed'));
		}
	};
	return <main className="auth-surface">
		<section className="auth-panel account-chooser-panel">
			<AuthBrand />
			<ServerIdentity serverName={serverName} detail="This Server profile" />
			<header className="auth-heading"><h1>{selectionCopy.title}</h1><p>{selectionCopy.body}</p></header>
			{error && <p className="auth-error account-chooser-error" role="alert"><AlertTriangle />{error}</p>}
			<div className="account-chooser-list">{profiles.map((profile) => <button
				key={profile.id}
				type="button"
				disabled={auth.busy}
				onClick={() => {
					setError('');
					setPin('');
					if (profile.hasPIN) setSelectedProfileId(profile.id);
					else void open(profile.id);
				}}
			>
				<span className="account-chooser-avatar">{profile.name.trim().slice(0, 1).toLocaleUpperCase() || 'P'}</span>
				<span><strong>{profile.name}</strong><small>{profile.hasPIN ? 'PIN required' : 'Open profile'}</small></span>
				<ArrowRight />
			</button>)}</div>
			{selected?.hasPIN && <form className="runtime-profile-pin" onSubmit={(event) => { event.preventDefault(); void open(selected.id, pin); }}>
				<label><span>{productMessage('auth.profile-pin-required', { profileName: selected.name }).body}</span><input autoFocus aria-label={`${selected.name} profile PIN`} inputMode="numeric" autoComplete="one-time-code" minLength={4} maxLength={4} pattern="[0-9]{4}" value={pin} onChange={(event) => setPin(event.target.value.replace(/\D/g, '').slice(0, 4))} required /></label>
				<PrimaryButton type="submit" disabled={auth.busy || pin.length !== 4}>{auth.busy ? 'Opening…' : 'Open profile'}</PrimaryButton>
			</form>}
			<div className="account-chooser-actions"><button className="auth-text-button" type="button" disabled={auth.busy} onClick={auth.cancelLocalProfileLogin}><ArrowLeft /> Back to sign in</button></div>
		</section>
	</main>;
}

export function SignInSurface({ serverName, addingAccount = false }: { serverName: string; addingAccount?: boolean }) {
  const auth = useAuthSession();
  const runtime = useOptionalRuntime();
  const navigate = useNavigate();
  const capabilities = auth.viewer?.authCapabilities;
  const localEnabled = capabilities?.localCredentialsEnabled ?? true;
  const porticoEnabled = capabilities?.porticoAccountAuthEnabled ?? false;
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
	const [rememberOnBrowser, setRememberOnBrowser] = useState(true);
  const [error, setError] = useState('');
  const callbackWarning = runtime?.connectionWarning ? productMessage(runtime.connectionWarning) : undefined;
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError('');
    try {
      await auth.login({ login: login.trim(), password, rememberOnBrowser });
      runtime?.dismissConnectionWarning();
      if (addingAccount) navigate('/home', { replace: true });
    } catch (reason) {
      setError(productErrorText(reason, 'auth.invalid-credentials'));
    }
  };
  return <main className="auth-surface">
    <section className="auth-panel auth-sign-in-panel">
      <AuthBrand />
      <ServerIdentity serverName={serverName} detail={addingAccount ? 'Add account' : porticoEnabled && !localEnabled ? 'Portico Account sign-in' : 'This Server'} />
      <header className="auth-heading"><h1>{addingAccount ? 'Add an account' : `Sign in to ${serverName}`}</h1><p>{addingAccount ? 'Sign in once to make another account available from this browser.' : porticoEnabled && !localEnabled ? 'Use the Portico Account that has access to this server.' : 'Use an account managed by this server.'}</p></header>
      {callbackWarning && <p className="auth-error" role="alert" data-semantic-icon={callbackWarning.icon}><AlertTriangle />{callbackWarning.body ?? callbackWarning.text ?? callbackWarning.title}</p>}
      {demoMode && !addingAccount && localEnabled && demoLogin && demoPassword && <aside className="auth-demo-credentials" aria-label="Public demo credentials">
        <header><strong>Public demo access</strong><small>Direct Play only · transcoding is disabled</small></header>
        <dl>
          <div><dt>Username</dt><dd><code>{demoLogin}</code></dd></div>
          <div><dt>Password</dt><dd><code>{demoPassword}</code></dd></div>
        </dl>
      </aside>}
      <label className="auth-remember-account"><input type="checkbox" checked={rememberOnBrowser} onChange={(event) => setRememberOnBrowser(event.target.checked)} /><span>Remember this account on this browser</span></label>
      {porticoEnabled && <a className="button primary auth-portico-button" href={`/api/auth/portico/start?returnUrl=${encodeURIComponent('/?accountAdded=1')}&rememberOnBrowser=${rememberOnBrowser}`}><Globe2 /> Continue with Portico Account <ArrowRight /></a>}
      {localEnabled && porticoEnabled && <div className="auth-divider"><span>or use This Server</span></div>}
      {localEnabled && <form onSubmit={submit}>
        <label><span>Username or email</span><input autoFocus autoComplete="username" value={login} onChange={(event) => setLogin(event.target.value)} required /></label>
        <label><span>Password</span><PasswordInput aria-label="Password" autoComplete="current-password" maxLength={72} value={password} onChange={(event) => setPassword(event.target.value)} required /></label>
        {error && <p className="auth-error" role="alert"><AlertTriangle />{error}</p>}
        <PrimaryButton type="submit" disabled={auth.busy || !login.trim() || !password}><KeyRound /> {auth.busy ? 'Signing in…' : 'Sign in with This Server'}</PrimaryButton>
      </form>}
      {!localEnabled && !porticoEnabled && <div className="auth-method-error" role="alert"><AlertTriangle /><span><strong>No sign-in method is available</strong><p>Ask the server owner to finish setup or restore a supported sign-in method.</p></span></div>}
      <p className="auth-security-note"><ShieldCheck /> Credentials are sent only to {porticoEnabled && !localEnabled ? 'Portico over HTTPS' : 'this server'}.</p>
	  {addingAccount && <button className="auth-text-button auth-cancel-add" type="button" disabled={auth.busy} onClick={auth.cancelAddAccount}><ArrowLeft /> Back to Portico</button>}
    </section>
  </main>;
}

export function SetupSurface({ serverName }: { serverName: string }) {
  const auth = useAuthSession();
  const returningFromPortico = typeof window !== 'undefined' && new URL(window.location.href).searchParams.get('porticoSetup') === 'continue';
  const [mode, setMode] = useState<'choose' | 'local' | 'portico'>(returningFromPortico ? 'portico' : 'choose');
  const [setupServerName, setSetupServerName] = useState(serverName);
  const [details, setDetails] = useState({ username: '', email: '', password: '' });
  const [confirmPassword, setConfirmPassword] = useState('');
  const [localOnlyAcknowledged, setLocalOnlyAcknowledged] = useState(false);
  const [error, setError] = useState('');
  const [setupCheckRevision, setSetupCheckRevision] = useState(0);
  const setupStatusFailures = useRef(0);
  useEffect(() => {
    if (mode !== 'portico') return;
    let active = true;
    let retryTimer: number | undefined;
    const check = async () => {
      try {
        const status = await auth.porticoSetupStatus();
        if (!active) return;
        setupStatusFailures.current = 0;
        if (!status.setupRequired && status.porticoConnected) {
          window.location.replace('/api/auth/portico/start?returnUrl=/');
          return;
        }
        if (status.claimStatus === 'expired' || status.claimStatus === 'cancelled') {
          setError('This setup request expired before the server could finish activating. Return to account options and try again.');
          return;
        }
        retryTimer = window.setTimeout(check, 1_000);
      } catch (reason) {
        if (!active) return;
        setupStatusFailures.current += 1;
        if (setupStatusFailures.current < 3) {
          retryTimer = window.setTimeout(check, 1_500);
          return;
        }
        setError(setupClaimErrorText(reason));
      }
    };
    void check();
    return () => {
      active = false;
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
    };
  }, [auth, mode, setupCheckRevision]);
  const update = (field: keyof typeof details, value: string) => setDetails((current) => ({ ...current, [field]: value }));
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError('');
    try {
      if (!validPorticoPassword(details.password)) throw new TypeError('Choose a password that meets every requirement.');
      if (details.password !== confirmPassword) throw new TypeError('Passwords do not match.');
      const username = details.username.trim();
      await auth.setup({ ...details, serverName: setupServerName.trim(), username, email: details.email.trim(), displayName: username });
    } catch (reason) {
      setError(productErrorText(reason, 'problem.request-failed'));
    }
  };
  const startPorticoSetup = async () => {
    setError('');
	let claimUrl = '';
    try {
      if (!setupServerName.trim()) throw new TypeError('Enter a name for your Portico server.');
      const claim = await auth.startPorticoSetup(setupServerName.trim());
      if (!claim.claimUrl) throw new Error('Portico did not return a secure claim address. Try again or use This Server setup.');
	  claimUrl = claim.claimUrl;
    } catch (reason) {
	  setError(setupClaimErrorText(reason));
	  return;
    }
	window.location.href = claimUrl;
  };
  const chooseLocalMode = () => {
    setError('');
    if (!setupServerName.trim()) {
      setError('Enter a name for your Portico server.');
      return;
    }
    setMode('local');
  };
  return <main className="auth-surface">
    <section className="auth-panel setup-panel">
      <AuthBrand />
      <header className="auth-heading"><h1>Set Up Your Portico Server</h1><p>Set your server's name and choose a sign-in method.</p></header>
      <div className="setup-server-name">
        <label htmlFor="setup-server-name"><span>Server name</span><input id="setup-server-name" autoFocus autoComplete="off" maxLength={120} value={setupServerName} onChange={(event) => { setSetupServerName(event.target.value); setError(''); }} required /><small>Shown in Portico applications, invitations, and connection screens.</small></label>
      </div>
      {mode === 'portico' && <div className={`setup-portico-progress${error ? ' error' : ''}`} aria-live="polite">
        {!error && <><LoaderCircle className="runtime-spinner" /><p><strong>Finishing server setup…</strong><small>Portico is activating this server and creating your local owner profile. Remote Access will be checked separately.</small></p></>}
        {error && <><div className="auth-error" role="alert"><AlertTriangle />{error}</div><div className="runtime-footer-actions"><SecondaryButton onClick={() => { setupStatusFailures.current = 0; setError(''); setSetupCheckRevision((current) => current + 1); }}><RefreshCw /> Try again</SecondaryButton><button type="button" className="auth-text-button" onClick={() => { window.history.replaceState(null, '', '/'); setMode('choose'); setError(''); }}>Account options</button></div></>}
      </div>}
      {mode === 'choose' && <div className="setup-mode-grid">
        <button type="button" disabled={auth.busy} onClick={() => void startPorticoSetup()}><span className="setup-mode-icon"><Globe2 /></span><span><span className="setup-mode-title"><strong>Use A Portico Account</strong><em>Recommended</em></span><small>Using a Portico Account allows you to easily set up and access your server remotely for free, invite other users, and use multiple Portico servers - all in one place. This option provides automatic network setup, including secure remote access to your server.</small><small className="setup-mode-guidance">Use this option if you want a seamless experience across Portico, or if you're not sure which is right for you.</small></span><ArrowRight /></button>
        <button type="button" className="setup-mode-secondary" disabled={auth.busy} onClick={chooseLocalMode}><span className="setup-mode-icon"><KeyRound /></span><span><span className="setup-mode-title"><strong>Use Server Authentication Only</strong><em>Advanced</em></span><small>This option will set this server up without using Portico's Hosted Services - no central account management, automatic network setup or invites. Use this option if you want exclusive control over your Portico server experience. This option requires manual configuration of remote access, networking and security prior to use.</small><small className="setup-mode-guidance">Use this option if you understand the risks, and want complete control over your Portico server.</small></span><ArrowRight /></button>
      </div>}
      {mode === 'choose' && error && <div className="auth-error setup-choice-error" role="alert"><AlertTriangle /><span><strong>Portico couldn't complete this request</strong><small>{error}</small></span></div>}
      {mode === 'local' && <><button type="button" className="auth-back-button" onClick={() => { setMode('choose'); setError(''); }}><ArrowLeft /> Account options</button><form onSubmit={submit}>
        <div className="setup-fields">
          <label><span>Username</span><input autoFocus autoComplete="username" value={details.username} onChange={(event) => update('username', event.target.value)} required /></label>
        </div>
        <label><span>Email (optional)</span><input autoComplete="email" type="email" value={details.email} onChange={(event) => update('email', event.target.value)} /></label>
        <label><span>Password</span><PasswordInput aria-label="Password" autoComplete="new-password" minLength={8} maxLength={72} value={details.password} onChange={(event) => update('password', event.target.value)} required aria-describedby="setup-password-requirement" /><PasswordRequirements id="setup-password-requirement" value={details.password} /></label>
        <label><span>Confirm password</span><PasswordInput aria-label="Confirm password" autoComplete="new-password" minLength={8} maxLength={72} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required />{confirmPassword && details.password !== confirmPassword && <small className="setup-password-mismatch" role="status">Passwords do not match.</small>}</label>
        <label className="local-only-acknowledgement"><input type="checkbox" checked={localOnlyAcknowledged} onChange={(event) => setLocalOnlyAcknowledged(event.target.checked)} required /><span>I understand that This Server credentials are managed here and do not create a Portico Account.</span></label>
        {error && <p className="auth-error" role="alert"><AlertTriangle />{error}</p>}
        <PrimaryButton type="submit" disabled={auth.busy || !setupServerName.trim() || !localOnlyAcknowledged || !details.username.trim() || !validPorticoPassword(details.password) || details.password !== confirmPassword}>{auth.busy ? 'Creating owner…' : 'Create This Server owner'}</PrimaryButton>
      </form></>}
    </section>
  </main>;
}
