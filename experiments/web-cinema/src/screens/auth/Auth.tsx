import React, {useEffect, useState} from 'react';
import {Link, useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Profile, Server} from '../../data/types';
import {useSession} from '../../app/session';
import {Button} from '../../ui/Button';
import {ChoiceCards, Checkbox, Field, Input, PasswordInput} from '../../ui/Controls';
import {Badge, Notice, Spinner, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Avatar, BrandSymbol} from '../../ui/Media';
import {cx} from '../../ui/cx';
import {AuthFrame} from './AuthFrame';
import {QRMock} from '../settings/Profiles';
import {relativeTime} from '../shared/items';

/** Every way into Portico, one URL each: /welcome/<screen>. */
export function AuthScreen() {
  const {screen} = useParams({from: '/welcome/$screen'});
  switch (screen) {
    case 'signin': return <SignIn />;
    case 'create': return <CreateAccount />;
    case 'forgot': return <Forgot />;
    case 'code': return <TwoStep />;
    case 'direct': return <DirectSignIn />;
    case 'new-password': return <NewPassword />;
    case 'servers': return <ServerChooser />;
    case 'profiles': return <Profiles />;
    case 'setup': return <Setup />;
    case 'restore': return <Restore />;
    case 'link-tv': return <LinkTV />;
    case 'waiting': return <WaitingApproval />;
    case 'unavailable': return <Unavailable />;
    case 'offline': return <Offline />;
    case 'ended': return <Ended />;
    default: return <Landing />;
  }
}

function Heading({title, lede, icon}: {title: string; lede?: React.ReactNode; icon?: React.ReactNode}) {
  return (
    <div className="auth-heading">
      {icon}
      <h1 className="display auth-title">{title}</h1>
      {lede ? <p className="auth-lede">{lede}</p> : null}
    </div>
  );
}

function useFinish() {
  const session = useSession();
  const navigate = useNavigate();
  return (via: 'account' | 'direct' = 'account') => {
    session.signIn(via);
    session.setProfile(null);
    void navigate({to: '/welcome/$screen', params: {screen: via === 'account' ? 'servers' : 'profiles'}});
  };
}

function Landing() {
  const navigate = useNavigate();
  const [phase, setPhase] = useState(0);
  useEffect(() => {
    const a = setTimeout(() => setPhase(1), 1200);
    const b = setTimeout(() => void navigate({to: '/welcome/$screen', params: {screen: 'signin'}}), 3200);
    return () => { clearTimeout(a); clearTimeout(b); };
  }, [navigate]);
  return (
    <AuthFrame bare>
      <div className="landing">
        <Spinner size={30} label="Starting" />
        <p className="auth-lede">{phase ? 'Opening Ehler Home Theater…' : 'Finding your servers…'}</p>
        <Button variant="ghost" label="Use another server or account" to="/welcome/$screen" params={{screen: 'signin'}} />
      </div>
    </AuthFrame>
  );
}

function SignIn() {
  const services = useServices();
  const navigate = useNavigate();
  const search = useSearch({from: '/welcome/$screen'});
  const finish = useFinish();
  const [user, setUser] = useState('');
  const [pass, setPass] = useState('');
  const [error, setError] = useState(search.error ?? '');
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setError('');
    setBusy(true);
    try {
      const r = await services.auth.signIn(user, pass);
      if (r.needsCode) void navigate({to: '/welcome/$screen', params: {screen: 'code'}});
      else finish();
    } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };
  return (
    <AuthFrame title="Sign in" footer={<>New to Portico? <Link to="/welcome/$screen" params={{screen: 'create'}} className="link">Create a Portico Account</Link></>}>
      <Heading title="Welcome home" lede="Sign in with your Portico Account to reach your servers from anywhere." />
      <div className="auth-providers">
        <Button full variant="primary" icon="account" label="Sign in with Apple" onClick={() => { toast('Continuing with Apple…'); setTimeout(() => finish(), 700); }} />
        <Button full variant="secondary" icon="globe" label="Sign in with Google" onClick={() => { toast('Continuing with Google…'); setTimeout(() => finish(), 700); }} />
      </div>
      <div className="auth-or"><span>or with your email</span></div>
      <form className="stack" onSubmit={e => { e.preventDefault(); void submit(); }}>
        {error ? <Notice tone="bad">{error}</Notice> : null}
        <Field label="Username or email"><Input value={user} onChange={e => setUser(e.target.value)} autoComplete="username" autoFocus /></Field>
        <Field label="Password"><PasswordInput value={pass} onChange={e => setPass(e.target.value)} autoComplete="current-password" /></Field>
        <div className="auth-row"><Link to="/welcome/$screen" params={{screen: 'forgot'}} className="link small">Forgot password?</Link></div>
        <Button full size="lg" variant="accent" label="Sign in" busy={busy} type="submit" />
      </form>
      <div className="auth-alt">
        <Link to="/welcome/$screen" params={{screen: 'direct'}} className="auth-alt-link"><Icon name="server" size={18} /><span><strong>Sign in to a server directly</strong><span>With an account made on the server itself</span></span><Icon name="chevronRight" size={16} /></Link>
        <Link to="/welcome/$screen" params={{screen: 'link-tv'}} className="auth-alt-link"><Icon name="qr" size={18} /><span><strong>Use a code from another device</strong><span>Sign in here with your phone</span></span><Icon name="chevronRight" size={16} /></Link>
      </div>
    </AuthFrame>
  );
}

function CreateAccount() {
  const services = useServices();
  const finish = useFinish();
  const [f, setF] = useState({name: '', username: '', email: '', password: ''});
  const [agree, setAgree] = useState(false);
  const [busy, setBusy] = useState(false);
  const badUser = f.username && !/^[a-z0-9_.-]{3,64}$/i.test(f.username);
  return (
    <AuthFrame title="Create your account" footer={<>Already have one? <Link to="/welcome/$screen" params={{screen: 'signin'}} className="link">Sign in</Link></>}>
      <Heading title="Create your account" lede="One Portico Account for all your servers and devices." />
      <form className="stack" onSubmit={async e => { e.preventDefault(); setBusy(true); await services.auth.createAccount(f.name, f.username, f.email, f.password); setBusy(false); toast(`Check ${f.email} to verify your email`, {icon: 'mail'}); finish(); }}>
        <Field label="Your name"><Input value={f.name} onChange={e => setF({...f, name: e.target.value})} autoComplete="name" autoFocus /></Field>
        <Field label="Username" help="3 to 64 characters: letters, numbers, dots, dashes." error={badUser ? 'Use 3 to 64 characters, no spaces or @.' : undefined}><Input value={f.username} onChange={e => setF({...f, username: e.target.value})} autoComplete="username" /></Field>
        <Field label="Email"><Input type="email" value={f.email} onChange={e => setF({...f, email: e.target.value})} autoComplete="email" /></Field>
        <Field label="Password" help="At least 10 characters."><PasswordInput value={f.password} onChange={e => setF({...f, password: e.target.value})} autoComplete="new-password" /></Field>
        <Checkbox checked={agree} onChange={setAgree} label={<>I agree to the <a className="link" href="https://getportico.tv/terms" target="_blank" rel="noreferrer">Terms</a> and <a className="link" href="https://getportico.tv/privacy" target="_blank" rel="noreferrer">Privacy Policy</a></>} />
        <Button full size="lg" variant="accent" label="Create account" busy={busy} disabled={!agree || !f.name || !f.username || !!badUser || !/.+@.+/.test(f.email) || f.password.length < 10} type="submit" />
      </form>
    </AuthFrame>
  );
}

function Forgot() {
  const services = useServices();
  const [who, setWho] = useState('');
  const [sent, setSent] = useState(false);
  return (
    <AuthFrame title="Forgot password" footer={<Link to="/welcome/$screen" params={{screen: 'signin'}} className="link">Back to sign in</Link>}>
      {sent ? (
        <Heading icon={<span className="auth-glyph"><Icon name="mail" size={28} /></span>} title="Check your email" lede={<>If <strong>{who}</strong> has a Portico Account, a reset link is on its way. It works for an hour.</>} />
      ) : (
        <>
          <Heading title="Reset your password" lede="Enter your username or email and we’ll send you a link." />
          <form className="stack" onSubmit={async e => { e.preventDefault(); await services.auth.requestPasswordReset(who); setSent(true); }}>
            <Field label="Username or email"><Input value={who} onChange={e => setWho(e.target.value)} autoFocus /></Field>
            <Button full size="lg" variant="accent" label="Send reset link" disabled={!who.trim()} type="submit" />
          </form>
        </>
      )}
    </AuthFrame>
  );
}

function TwoStep() {
  const services = useServices();
  const finish = useFinish();
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [trust, setTrust] = useState(true);
  return (
    <AuthFrame title="Two-step verification" footer={<Link to="/welcome/$screen" params={{screen: 'signin'}} className="link">Use a different account</Link>}>
      <Heading icon={<span className="auth-glyph"><Icon name="shieldCheck" size={28} /></span>} title="Enter your code" lede="Open your authenticator app and enter the 6-digit code for Portico. A recovery code works too." />
      <form className="stack" onSubmit={async e => { e.preventDefault(); setBusy(true); try { await services.auth.verifyCode(code); finish(); } catch (err) { setError((err as Error).message); } finally { setBusy(false); } }}>
        <Field label="Code" error={error || undefined}><Input value={code} onChange={e => { setCode(e.target.value); setError(''); }} inputMode="numeric" autoComplete="one-time-code" className="pin-input" autoFocus /></Field>
        <Checkbox checked={trust} onChange={setTrust} label="Don’t ask again on this browser" />
        <Button full size="lg" variant="accent" label="Verify" busy={busy} disabled={code.replace(/\s/g, '').length < 6} type="submit" />
      </form>
    </AuthFrame>
  );
}

function DirectSignIn() {
  const services = useServices();
  const navigate = useNavigate();
  const finish = useFinish();
  const [f, setF] = useState({address: '', username: '', password: ''});
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [owner, setOwner] = useState(false);
  return (
    <AuthFrame title="Sign in to a server" footer={<>Have an invitation code? <Link to="/join" className="link">Join a server</Link></>}>
      <Heading icon={<span className="auth-glyph"><Icon name="server" size={28} /></span>} title="Sign in to a server" lede="Use an account made on the server itself. Ask its owner for the address." />
      <form className="stack" onSubmit={async e => {
        e.preventDefault();
        setBusy(true); setError('');
        try { const r = await services.auth.signInDirect(f.address, f.username, f.password); if (r.mustChangePassword) void navigate({to: '/welcome/$screen', params: {screen: 'new-password'}}); else finish('direct'); } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
      }}>
        {error ? <Notice tone="bad">{error}</Notice> : null}
        <Field label="Server address" help="Like 192.168.1.20:32500 or media.example.com"><Input value={f.address} onChange={e => setF({...f, address: e.target.value})} placeholder="192.168.1.20:32500" autoFocus autoComplete="url" /></Field>
        <Field label="Username"><Input value={f.username} onChange={e => setF({...f, username: e.target.value})} autoComplete="username" /></Field>
        <Field label={owner ? 'Recovery key' : 'Password'} help={owner ? 'The owner’s recovery key from the server’s setup.' : undefined}><PasswordInput value={f.password} onChange={e => setF({...f, password: e.target.value})} autoComplete="current-password" /></Field>
        <Checkbox checked={owner} onChange={setOwner} label="I’m the owner and can’t sign in (use the recovery key)" />
        <Button full size="lg" variant="accent" label="Sign in" busy={busy} type="submit" />
      </form>
      <div className="auth-alt"><Link to="/welcome/$screen" params={{screen: 'signin'}} className="auth-alt-link"><Icon name="account" size={18} /><span><strong>Use your Portico Account instead</strong><span>Reach every server from one sign-in</span></span><Icon name="chevronRight" size={16} /></Link></div>
    </AuthFrame>
  );
}

function NewPassword() {
  const finish = useFinish();
  const [p, setP] = useState('');
  const [r, setR] = useState('');
  return (
    <AuthFrame title="Choose a new password">
      <Heading icon={<span className="auth-glyph"><Icon name="key" size={28} /></span>} title="Choose a new password" lede="The server owner asked you to set your own password before you continue." />
      <form className="stack" onSubmit={e => { e.preventDefault(); toast('Password changed'); finish('direct'); }}>
        <Field label="New password" help="At least 10 characters."><PasswordInput value={p} onChange={e => setP(e.target.value)} autoComplete="new-password" autoFocus /></Field>
        <Field label="Repeat password" error={r && r !== p ? 'The passwords don’t match.' : undefined}><PasswordInput value={r} onChange={e => setR(e.target.value)} autoComplete="new-password" /></Field>
        <Button full size="lg" variant="accent" label="Save and continue" disabled={p.length < 10 || p !== r} type="submit" />
      </form>
    </AuthFrame>
  );
}

function ServerChooser() {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const servers = useQuery('auth.servers', () => services.auth.servers());
  const [checking, setChecking] = useState(false);
  const pick = (s: Server) => {
    if (s.status !== 'online') return;
    session.setServer(s.id);
    void navigate({to: '/welcome/$screen', params: {screen: 'profiles'}});
  };
  return (
    <AuthFrame title="Choose a server" footer={<><Link to="/welcome/$screen" params={{screen: 'direct'}} className="link">Sign in to a server directly</Link> · <button type="button" className="link" onClick={() => { session.signOut(); void navigate({to: '/welcome/$screen', params: {screen: 'signin'}}); }}>Sign out of justin</button></>}>
      <Heading title="Choose a server" lede="Servers you own or were invited to with your Portico Account." />
      <div className="server-list">
        {(servers.data ?? []).map(s => (
          <button key={s.id} type="button" className={cx('server-pick', s.status !== 'online' && 'is-off', s.id === session.serverId && 'is-current')} onClick={() => pick(s)} aria-disabled={s.status !== 'online'}>
            <span className="server-glyph"><BrandSymbol size={24} /></span>
            <span className="set-text"><strong>{s.name}</strong><span>{s.status === 'online' ? `${s.role === 'owner' ? 'Yours' : s.role === 'admin' ? 'Admin' : 'Shared with you'} · ${s.libraries} libraries` : `Can’t reach it · last seen ${relativeTime(s.lastSeen).toLowerCase()}`}</span></span>
            {s.status === 'online' ? (s.id === session.serverId ? <Badge tone="accent">Last used</Badge> : <Icon name="chevronRight" size={18} className="row-chevron" />) : <Badge tone="warn">Unreachable</Badge>}
          </button>
        ))}
        {!servers.data ? <div className="landing"><Spinner /></div> : null}
      </div>
      <Button variant="ghost" icon="refresh" label="Check now" busy={checking} onClick={() => { setChecking(true); setTimeout(() => { setChecking(false); servers.reload(); }, 900); }} />
    </AuthFrame>
  );
}

function Profiles() {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const profiles = useQuery(`profiles.${session.serverId}`, () => services.auth.profiles(session.serverId));
  const [pinFor, setPinFor] = useState<Profile | null>(null);
  const [pin, setPin] = useState('');
  const [wrong, setWrong] = useState(false);
  const [remember, setRemember] = useState(true);
  const enter = (p: Profile) => { session.setProfile(p.id); void navigate({to: '/'}); };
  const press = async (d: string) => {
    const next = (pin + d).slice(0, 4);
    setPin(next);
    setWrong(false);
    if (next.length === 4 && pinFor) {
      const ok = await services.auth.unlockProfile(pinFor.id, next);
      if (ok) enter(pinFor);
      else { setWrong(true); setPin(''); }
    }
  };
  useEffect(() => {
    if (!pinFor) return;
    const onKey = (e: KeyboardEvent) => { if (/^\d$/.test(e.key)) void press(e.key); if (e.key === 'Backspace') setPin(p => p.slice(0, -1)); if (e.key === 'Escape') { setPinFor(null); setPin(''); } };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  if (pinFor) {
    return (
      <AuthFrame title="Enter PIN" bare>
        <div className="pin-screen">
          <Avatar name={pinFor.name} color={pinFor.color} size={96} />
          <h1 className="display auth-title">{pinFor.name}</h1>
          <p className="auth-lede" aria-live="assertive">{wrong ? 'That PIN isn’t right. Try again.' : 'Enter your 4-digit PIN'}</p>
          <div className={cx('pin-dots', wrong && 'is-wrong')} aria-label={`${pin.length} of 4 digits entered`}>{[0, 1, 2, 3].map(i => <span key={i} className={cx(i < pin.length && 'is-on')} />)}</div>
          <div className="pin-pad">
            {['1', '2', '3', '4', '5', '6', '7', '8', '9', '', '0', '⌫'].map(k => k ? <button key={k} type="button" className="pin-key" onClick={() => (k === '⌫' ? setPin(p => p.slice(0, -1)) : void press(k))} aria-label={k === '⌫' ? 'Delete digit' : k}>{k}</button> : <span key="gap" />)}
          </div>
          <div className="hstack" style={{justifyContent: 'center'}}>
            <Button variant="ghost" label="Back" onClick={() => { setPinFor(null); setPin(''); }} />
            <Button variant="ghost" label="Forgot PIN?" onClick={() => toast('The main profile can remove the PIN in Settings › Profiles', {icon: 'info'})} />
          </div>
        </div>
      </AuthFrame>
    );
  }
  return (
    <AuthFrame title="Who’s watching?" bare full footer={<><Link to="/welcome/$screen" params={{screen: 'servers'}} className="link">Switch server</Link> · <span className="faint">{session.server?.name}</span></>}>
      <div className="whos">
        <h1 className="display auth-title is-big">Who’s watching?</h1>
        <div className="whos-grid">
          {(profiles.data ?? []).map(p => (
            <button key={p.id} type="button" className="whos-card" onClick={() => (p.pin ? setPinFor(p) : enter(p))}>
              <Avatar name={p.name} color={p.color} size={124} />
              <strong>{p.name}</strong>
              <span className="faint small">{p.kids ? 'Kids' : p.pin ? <><Icon name="lock" size={12} /> PIN</> : ' '}</span>
            </button>
          ))}
          {!profiles.data ? [0, 1, 2, 3].map(i => <span key={i} className="whos-card"><span className="skeleton" style={{width: 124, height: 124, borderRadius: '50%'}} /></span>) : null}
        </div>
        <Checkbox checked={remember} onChange={setRemember} label="Remember my profile on this browser" />
      </div>
    </AuthFrame>
  );
}

function Setup() {
  const services = useServices();
  const finish = useFinish();
  const [step, setStep] = useState(0);
  const [name, setName] = useState('Ehler Home Theater');
  const [mode, setMode] = useState<'account' | 'local'>('account');
  const [user, setUser] = useState('');
  const [pass, setPass] = useState('');
  const [busy, setBusy] = useState(false);
  const steps = ['Welcome', 'Name', 'Signing in', 'Owner'];
  return (
    <AuthFrame title="Set up your server" wide>
      <ol className="steps" aria-label="Setup steps">{steps.map((s, i) => <li key={s} className={cx(i === step && 'is-on', i < step && 'is-done')}><span>{i < step ? <Icon name="check" size={13} stroke={3} /> : i + 1}</span>{s}</li>)}</ol>
      {step === 0 ? (
        <>
          <Heading icon={<span className="auth-glyph"><BrandSymbol size={30} /></span>} title="Your server is ready" lede="Portico is installed. A few choices and it’s yours: a name, how people sign in, and who owns it." />
          <Button full size="lg" variant="accent" label="Get started" onClick={() => setStep(1)} />
        </>
      ) : step === 1 ? (
        <>
          <Heading title="Name your server" lede="People see this name when they choose a server." />
          <Field label="Server name"><Input value={name} onChange={e => setName(e.target.value)} autoFocus maxLength={50} /></Field>
          <div className="auth-nav"><Button variant="ghost" label="Back" onClick={() => setStep(0)} /><Button variant="accent" label="Continue" disabled={!name.trim()} onClick={() => setStep(2)} /></div>
        </>
      ) : step === 2 ? (
        <>
          <Heading title="How will people sign in?" lede="You can change this later in the server console." />
          <ChoiceCards label="How people sign in" columns={1} value={mode} onChange={setMode} options={[{value: 'account', title: 'With Portico Accounts', body: 'Recommended. Reach the server from anywhere, invite people by email, and sign in on TVs with a code.', icon: 'account'}, {value: 'local', title: 'With accounts on this server only', body: 'Everything stays on your network. You make each account yourself.', icon: 'server'}]} />
          <div className="auth-nav"><Button variant="ghost" label="Back" onClick={() => setStep(1)} /><Button variant="accent" label="Continue" onClick={() => setStep(3)} /></div>
        </>
      ) : (
        <>
          <Heading title={mode === 'account' ? 'Connect your Portico Account' : 'Create the owner account'} lede={mode === 'account' ? 'This account owns the server. Also set a recovery owner in case you can’t sign in.' : 'This account owns the server and can manage everything.'} />
          <div className="stack">
            <Field label={mode === 'account' ? 'Portico Account username or email' : 'Username'}><Input value={user} onChange={e => setUser(e.target.value)} autoFocus /></Field>
            <Field label="Password"><PasswordInput value={pass} onChange={e => setPass(e.target.value)} /></Field>
            <Notice tone="info" title="Recovery key">After setup you’ll get a recovery key. Keep it safe: it lets the owner back in if their password is lost.</Notice>
          </div>
          <div className="auth-nav"><Button variant="ghost" label="Back" onClick={() => setStep(2)} /><Button variant="accent" label="Finish setup" busy={busy} disabled={!user || !pass} onClick={async () => { setBusy(true); await services.auth.setupServer({name, mode, username: user, password: pass}); setBusy(false); toast(`${name} is set up`); finish(mode === 'account' ? 'account' : 'direct'); }} /></div>
        </>
      )}
    </AuthFrame>
  );
}

function Restore() {
  const navigate = useNavigate();
  const [phase, setPhase] = useState(0);
  useEffect(() => { const t = setTimeout(() => setPhase(1), 1500); return () => clearTimeout(t); }, []);
  return (
    <AuthFrame title="Reconnecting">
      <Heading icon={<span className="auth-glyph"><Spinner size={28} /></span>} title={phase ? 'Reconnecting to Ehler Home Theater' : 'Checking your saved connection'} lede={phase ? 'The server is taking longer than usual to answer. It may be restarting.' : 'One moment…'} />
      {phase ? <div className="stack"><Button full variant="accent" label="Try again" onClick={() => setPhase(0)} /><Button full variant="ghost" label="Use another server or account" onClick={() => void navigate({to: '/welcome/$screen', params: {screen: 'signin'}})} /></div> : null}
    </AuthFrame>
  );
}

function LinkTV() {
  const services = useServices();
  const code = useQuery('tv.code', () => services.auth.tvLinkCode());
  const [left, setLeft] = useState(15 * 60);
  useEffect(() => { const t = setInterval(() => setLeft(l => Math.max(0, l - 1)), 1000); return () => clearInterval(t); }, []);
  return (
    <AuthFrame title="Sign in with a code" wide>
      <Heading title="Sign in with your phone" lede={<>On your phone or computer, open <strong>getportico.tv/link</strong> and enter this code, or scan it.</>} />
      <div className="link-tv">
        <div className="qr is-lg"><QRMock seed={code.data?.code ?? 'code'} /></div>
        <div className="link-code">
          <span className="code-big tabular" aria-label={`Code ${code.data?.code.split('').join(' ') ?? ''}`}>{code.data?.code ?? '··· ···'}</span>
          <span className="faint small"><Spinner size={12} /> Waiting for you to approve · new code in {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}</span>
          <Button variant="ghost" icon="refresh" label="Get a new code" onClick={() => { code.reload(); setLeft(900); }} />
        </div>
      </div>
      <div className="auth-alt"><Link to="/welcome/$screen" params={{screen: 'signin'}} className="auth-alt-link"><Icon name="keyboard" size={18} /><span><strong>Sign in with a password instead</strong><span>Type your username and password here</span></span><Icon name="chevronRight" size={16} /></Link></div>
    </AuthFrame>
  );
}

function WaitingApproval() {
  return (
    <AuthFrame title="Waiting for approval">
      <Heading icon={<span className="auth-glyph"><Spinner size={28} /></span>} title="Waiting for the owner to approve" lede="Ehler Home Theater approves new devices before they can play. The owner has been asked; this page continues on its own once they do." />
      <Button full variant="ghost" label="Use another server" to="/welcome/$screen" params={{screen: 'servers'}} />
    </AuthFrame>
  );
}

function Unavailable() {
  const session = useSession();
  const navigate = useNavigate();
  return (
    <AuthFrame title="Server unavailable">
      <Heading icon={<span className="auth-glyph is-warn"><Icon name="wifiOff" size={28} /></span>} title={`Can’t reach ${session.server?.name ?? 'this server'}`} lede="It may be off, asleep or offline. Portico keeps trying; downloads on this device still play." />
      <div className="stack">
        <Button full variant="accent" icon="refresh" label="Try again" onClick={() => { session.setServer('srv-home'); void navigate({to: '/welcome/$screen', params: {screen: 'profiles'}}); }} />
        <Button full variant="secondary" icon="server" label="Choose another server" to="/welcome/$screen" params={{screen: 'servers'}} />
        <Button full variant="ghost" icon="download" label="Watch downloads" to="/welcome/$screen" params={{screen: 'offline'}} />
      </div>
    </AuthFrame>
  );
}

function Offline() {
  return (
    <AuthFrame title="You’re offline">
      <Heading icon={<span className="auth-glyph is-warn"><Icon name="wifiOff" size={28} /></span>} title="You’re offline" lede="Your downloads are still here. Everything else comes back when you’re connected." />
      <div className="stack"><Button full variant="accent" icon="download" label="Watch downloads" to="/downloads" /><Button full variant="ghost" icon="refresh" label="Try again" to="/" /></div>
    </AuthFrame>
  );
}

function Ended() {
  return (
    <AuthFrame title="Signed out">
      <Heading icon={<span className="auth-glyph is-warn"><Icon name="signOut" size={28} /></span>} title="You were signed out" lede="The server was reset or your sign-in was ended from another device. Sign in again to continue." />
      <Button full size="lg" variant="accent" label="Sign in" to="/welcome/$screen" params={{screen: 'signin'}} />
    </AuthFrame>
  );
}
