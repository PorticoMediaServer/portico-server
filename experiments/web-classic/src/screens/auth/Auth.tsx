import React, {useEffect, useState} from 'react';
import {Link, useLocation, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import {useSession} from '@/app/session';
import {Button} from '@/ui/Button';
import {Artwork, Spinner} from '@/ui/Bits';
import {Checkbox, PinInput, RadioCards, TextField} from '@/ui/Form';
import {Icon, type IconName} from '@/ui/Icon';
import {ProfileGrid, ServerList} from '@/shell/Switchers';
import {useTitle} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {PlayerProvider, usePlayer} from '@/player/PlayerProvider';
import PlayerHost from '@/player/PlayerHost';
import {SelectionProvider} from '@/app/selection';
import {ActionsProvider} from '@/app/actions';
import {useRouteSearch} from '../common';
import {AppleMark, AuthFrame, GoogleMark, Qr, authStyles as s} from './AuthFrame';

/** Signed-out and link pages, chosen by path. */
export default function AuthScreen() {
  const path = useLocation().pathname;
  switch (path) {
    case '/welcome': return <Landing />;
    case '/sign-in': return <AccountSignIn />;
    case '/sign-in/two-step': return <TwoStep />;
    case '/sign-in/direct': return <DirectSignIn />;
    case '/sign-in/quick-connect': return <QuickConnect />;
    case '/sign-in/servers': return <ChooseServer />;
    case '/sign-in/profiles': return <ChooseProfile />;
    case '/sign-in/waiting': return <WaitingForApproval />;
    case '/join': return <Join />;
    case '/device': return <LinkDevice />;
    case '/activate': return <Activate />;
    case '/claim': return <Claim />;
    case '/setup': return <Setup />;
    case '/restore': return <Restore />;
    case '/verify-email': return <VerifyEmail />;
    case '/reset': return <ResetPassword />;
    case '/together/join': return <TogetherJoin />;
    case '/app/account-return': return <AccountReturn />;
    case '/unavailable': return <Unavailable />;
    case '/offline': return <Offline />;
    default: return <Landing />;
  }
}

function Choice({icon, title, help, to, onClick}: {icon: IconName; title: string; help: string; to?: string; onClick?: () => void}) {
  const body = <><span className={s.choiceIcon}><Icon name={icon} size={20} /></span><span className={s.choiceText}><span className={s.choiceTitle} style={{display: 'block'}}>{title}</span><span className={s.choiceHelp} style={{display: 'block'}}>{help}</span></span><Icon name="forward" size={16} className="c-3" /></>;
  return to ? <Link to={to as never} className={s.choice}>{body}</Link> : <button type="button" className={s.choice} onClick={onClick}>{body}</button>;
}

function Landing() {
  useTitle('Welcome');
  return (
    <AuthFrame title="Your movies, shows and music, everywhere" lede="Sign in to watch what’s on your Portico server, at home or away." foot={<>New to Portico? <Link to="/setup" className="c-accent" style={{fontWeight: 650}}>Set up a server</Link> on a computer or NAS.</>}>
      <Button variant="primary" size="lg" block label="Sign in with your Portico Account" to="/sign-in" />
      <div className={s.divider}>or</div>
      <div className={s.choices}>
        <Choice icon="phone" title="Use a code from another device" help="Approve this browser from your phone" to="/activate" />
        <Choice icon="server" title="Sign in directly to a server" help="With the server’s address and your account on it" to="/sign-in/direct" />
        <Choice icon="mail" title="Join with an invitation" help="Someone shared their server with you" to="/join" />
        <Choice icon="download" title="Watch downloads" help="Titles saved in this browser, without a connection" to="/offline" />
      </div>
    </AuthFrame>
  );
}

function strength(pw: string) { let n = 0; if (pw.length >= 8) n++; if (pw.length >= 12) n++; if (/\d/.test(pw) && /[a-z]/i.test(pw)) n++; if (/[^\w\s]/.test(pw) || pw.split(' ').length >= 3) n++; return n; }

function AccountSignIn() {
  const search = useRouteSearch<{mode?: string; next?: string}>();
  const client = useClient();
  const navigate = useNavigate();
  const mode = search.mode === 'create' ? 'create' : search.mode === 'forgot' ? 'forgot' : 'signin';
  const [identity, setIdentity] = useState('');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState('');
  const [agree, setAgree] = useState(false);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  useTitle(mode === 'create' ? 'Create a Portico Account' : mode === 'forgot' ? 'Reset your password' : 'Sign in');
  const go = (m?: string) => { setError(undefined); setSent(false); void navigate({to: '/sign-in' as never, search: (m ? {mode: m} : {}) as never}); };
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setError(undefined);
    try {
      if (mode === 'forgot') { await client.session.requestPasswordReset(identity); setSent(true); }
      else if (mode === 'create') { await client.session.createAccount({username: identity, email, password}); void navigate({to: '/sign-in/servers' as never}); }
      else { const r = await client.session.signInAccount(identity, password); void navigate({to: (r.twoStep ? '/sign-in/two-step' : '/sign-in/servers') as never}); }
    } catch (err) { setError(err instanceof Error ? err.message : 'That didn’t work. Try again.'); } finally { setBusy(false); }
  };
  if (mode === 'forgot') return (
    <AuthFrame title={sent ? 'Check your email' : 'Reset your password'} lede={sent ? `If ${identity} has a Portico Account, a link to choose a new password is on its way. It works for an hour.` : 'Enter your username or email. We’ll send a link to choose a new password.'}>
      {sent ? <Button variant="secondary" block label="Back to sign in" onClick={() => go()} /> : (
        <form onSubmit={submit} style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}}>
          <TextField label="Username or email" value={identity} onChange={setIdentity} autoComplete="username" autoFocus error={error} />
          <Button type="submit" variant="primary" size="lg" block label="Send reset link" busy={busy} disabled={!identity.trim()} />
          <div className={s.links}><button type="button" onClick={() => go()}>Back to sign in</button></div>
        </form>
      )}
    </AuthFrame>
  );
  const level = strength(password);
  return (
    <AuthFrame title={mode === 'create' ? 'Create your Portico Account' : 'Sign in to Portico'} lede={mode === 'create' ? 'One account for every server you use and every device you watch on.' : undefined}>
      <div className={s.providers}>
        <Button className={s.apple} onClick={() => void navigate({to: '/app/account-return' as never})}><AppleMark />Sign in with Apple</Button>
        <Button className={s.google} onClick={() => void navigate({to: '/app/account-return' as never})}><GoogleMark />Sign in with Google</Button>
      </div>
      <div className={s.divider}>or with your {mode === 'create' ? 'email' : 'password'}</div>
      <form onSubmit={submit} style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}}>
        <TextField label={mode === 'create' ? 'Username' : 'Username or email'} value={identity} onChange={v => { setIdentity(v); setError(undefined); }} autoComplete="username" autoFocus />
        {mode === 'create' ? <TextField type="email" label="Email" value={email} onChange={setEmail} autoComplete="email" /> : null}
        <TextField type="password" label="Password" value={password} onChange={v => { setPassword(v); setError(undefined); }} autoComplete={mode === 'create' ? 'new-password' : 'current-password'} error={error} />
        {mode === 'create' && password ? <div><div className={s.meter} data-level={level} aria-hidden>{[1, 2, 3, 4].map(n => <span key={n} data-on={n <= level} />)}</div><p className="t-caption c-3" style={{marginTop: 6}}>{['Too short. Use at least 8 characters.', 'Weak. Longer is stronger.', 'Fair. A few more characters would help.', 'Good.', 'Strong.'][level]}</p></div> : null}
        {mode === 'create' ? <Checkbox checked={agree} onChange={setAgree} label="I agree to the Terms and Privacy Policy" /> : null}
        <Button type="submit" variant="primary" size="lg" block label={mode === 'create' ? 'Create account' : 'Sign in'} busy={busy} disabled={!identity.trim() || !password || (mode === 'create' && (!agree || level < 1 || !email.includes('@')))} />
      </form>
      <div className={s.links}>
        {mode === 'create' ? <button type="button" onClick={() => go()}>I already have an account</button> : <><button type="button" onClick={() => go('forgot')}>Forgot password?</button><button type="button" onClick={() => go('create')}>Create an account</button></>}
      </div>
    </AuthFrame>
  );
}

function TwoStep() {
  const client = useClient();
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [recovery, setRecovery] = useState(false);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useTitle('Two-step sign-in');
  const submit = async (value: string) => { setBusy(true); setError(undefined); try { await client.session.verifyCode(value); void navigate({to: '/sign-in/servers' as never}); } catch (e) { setError(e instanceof Error ? e.message : 'That code didn’t work.'); setCode(''); } finally { setBusy(false); } };
  return (
    <AuthFrame title="Enter your code" lede={recovery ? 'Enter one of the recovery codes you saved when you turned on two-step sign-in. Each works once.' : 'Open your authenticator app and enter the six-digit code for Portico.'}>
      {recovery ? (
        <form onSubmit={e => { e.preventDefault(); void submit(code); }} style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}}>
          <TextField label="Recovery code" placeholder="xxxx-xxxx" value={code} onChange={setCode} autoFocus error={error} autoComplete="one-time-code" />
          <Button type="submit" variant="primary" size="lg" block label="Continue" busy={busy} disabled={code.length < 9} />
        </form>
      ) : (
        <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 12, justifyItems: 'center'}}>
          <PinInput length={6} secret={false} label="Authenticator code" value={code} onChange={v => { setCode(v); setError(undefined); }} autoFocus onComplete={v => void submit(v)} />
          {error ? <p className="c-danger t-caption" role="alert">{error}</p> : busy ? <Spinner size={22} /> : null}
        </div>
      )}
      <div className={s.links}><button type="button" onClick={() => { setRecovery(!recovery); setCode(''); setError(undefined); }}>{recovery ? 'Use my authenticator app' : 'Use a recovery code'}</button><Link to="/sign-in">Back</Link></div>
    </AuthFrame>
  );
}

function ChooseServer() {
  const navigate = useNavigate();
  useTitle('Choose a server');
  return (
    <AuthFrame title="Choose a server" lede="Servers on your Portico Account, including ones shared with you." foot={<>Have an invitation? <Link to="/join" className="c-accent" style={{fontWeight: 650}}>Join a server</Link></>}>
      <ServerList onChosen={sv => void navigate({to: '/sign-in/profiles' as never, search: {server: sv.id} as never})} />
    </AuthFrame>
  );
}

function ChooseProfile() {
  const search = useRouteSearch<{server?: string}>();
  const navigate = useNavigate();
  useTitle('Who’s watching?');
  return (
    <AuthFrame title="Who’s watching?" wide lede="Each profile has its own history, Watchlist and recommendations.">
      <ProfileGrid serverId={search.server} onChosen={() => void navigate({to: '/' as never})} />
    </AuthFrame>
  );
}

function DirectSignIn() {
  const search = useRouteSearch<{host?: string}>();
  const client = useClient();
  const navigate = useNavigate();
  const [host, setHost] = useState(search.host ?? '');
  const [user, setUser] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useTitle('Sign in to a server');
  return (
    <AuthFrame title="Sign in to a server" lede="Use your server’s address and the account its owner made for you. No Portico Account needed." foot="On the same network as the server? The address is shown in Portico Server’s window, for example 192.168.1.20:32500.">
      <form style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}} onSubmit={async e => { e.preventDefault(); setBusy(true); setError(undefined); try { await client.session.signInDirect(host, user, password); void navigate({to: '/sign-in/profiles' as never}); } catch (err) { setError(err instanceof Error ? err.message : 'That didn’t work.'); } finally { setBusy(false); } }}>
        <TextField label="Server address" placeholder="192.168.1.20:32500" value={host} onChange={setHost} autoFocus icon="server" autoComplete="url" spellCheck={false} />
        <TextField label="Username" value={user} onChange={setUser} autoComplete="username" />
        <TextField type="password" label="Password" value={password} onChange={setPassword} autoComplete="current-password" error={error} />
        <Button type="submit" variant="primary" size="lg" block label="Sign in" busy={busy} disabled={!host || !user || !password} />
      </form>
      <div className={s.divider}>or</div>
      <Choice icon="qr" title="Use Quick Connect" help="Get a code to approve from a device that’s already signed in" to="/sign-in/quick-connect" />
      <div className={s.links}><Link to="/welcome">Other ways to sign in</Link><Link to="/reset?owner=1">Owner recovery</Link></div>
    </AuthFrame>
  );
}

function useApprovalTimer(ms: number, onDone: () => void) {
  useEffect(() => { const t = setTimeout(onDone, ms); return () => clearTimeout(t); }, []); // eslint-disable-line react-hooks/exhaustive-deps
}

function QuickConnect() {
  const navigate = useNavigate();
  const [approved, setApproved] = useState(false);
  useApprovalTimer(9000, () => setApproved(true));
  useTitle('Quick Connect');
  return (
    <AuthFrame title={approved ? 'Approved' : 'Quick Connect'} lede={approved ? 'This browser can sign in to Lighthouse now.' : 'On a device that’s already signed in to your server, open Settings › Devices › Quick Connect and enter this code.'}>
      {approved ? <Button variant="primary" size="lg" block label="Continue" onClick={() => void navigate({to: '/sign-in/profiles' as never})} /> : (
        <>
          <div className={s.code} aria-label="Quick Connect code">482 916</div>
          <p className="t-caption c-3" style={{display: 'flex', gap: 8, alignItems: 'center', justifyContent: 'center'}}><Spinner size={14} />Waiting for approval. The code works for 10 minutes.</p>
          <div className={s.links}><Link to="/sign-in/direct">Use a password instead</Link></div>
        </>
      )}
    </AuthFrame>
  );
}

function WaitingForApproval() {
  const navigate = useNavigate();
  const [approved, setApproved] = useState(false);
  useApprovalTimer(8000, () => setApproved(true));
  useTitle('Waiting for approval');
  return (
    <AuthFrame title={approved ? 'You’re approved' : 'Waiting for the owner'} lede={approved ? 'The server’s owner approved this browser.' : 'Lighthouse asks its owner to approve new devices. They’ve been notified; this page continues on its own once they do.'}>
      <div className={s.center}>
        <div className={cx(s.centerIcon, approved && s.centerIconGood)}><Icon name={approved ? 'check' : 'shield'} size={28} /></div>
        {approved ? <Button variant="primary" size="lg" block label="Continue" onClick={() => void navigate({to: '/sign-in/profiles' as never})} /> : <><dl className={s.facts} style={{width: '100%'}}><div><dt>Device</dt><dd>Chrome on this computer</dd></div><div><dt>Server</dt><dd>Lighthouse</dd></div><div><dt>Asked</dt><dd>Just now</dd></div></dl><p className="t-caption c-3" style={{display: 'flex', gap: 8, alignItems: 'center'}}><Spinner size={14} />Waiting…</p></>}
      </div>
      {!approved ? <Button variant="ghost" block label="Cancel" to="/welcome" /> : null}
    </AuthFrame>
  );
}

function Activate() {
  const navigate = useNavigate();
  const code = useQuery(c => c.session.activationCode(), []);
  const [approved, setApproved] = useState(false);
  useApprovalTimer(10000, () => setApproved(true));
  useTitle('Sign in with a code');
  return (
    <AuthFrame title={approved ? 'You’re signed in' : 'Sign in with your phone'} lede={approved ? 'Your phone approved this browser.' : <>On your phone, go to <b>{code.data?.url ?? 'getportico.tv/link'}</b> and enter this code, or scan the QR code.</>}>
      {approved ? <Button variant="primary" size="lg" block label="Continue" onClick={() => void navigate({to: '/sign-in/servers' as never})} /> : !code.data ? <div style={{display: 'flex', justifyContent: 'center', padding: 24}}><Spinner /></div> : (
        <>
          <div className={s.code} aria-label={`Code ${code.data.code}`}>{code.data.code}</div>
          <Qr value={`https://${code.data.url}?code=${code.data.code}`} label="QR code to approve this browser" />
          <p className="t-caption c-3" style={{display: 'flex', gap: 8, alignItems: 'center', justifyContent: 'center'}}><Spinner size={14} />Waiting for your phone. The code works for 10 minutes.</p>
        </>
      )}
      <div className={s.links}><Link to="/welcome">Other ways to sign in</Link></div>
    </AuthFrame>
  );
}

/** Link a TV: enter the code a TV shows, check it's the right device, approve. */
function LinkDevice() {
  const client = useClient();
  const session = useSession();
  const [code, setCode] = useState(useRouteSearch<{code?: string}>().code ?? '');
  const [device, setDevice] = useState<{device: string; platform: string; location: string}>();
  const [done, setDone] = useState<'approved' | 'denied'>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useTitle('Link a TV');
  if (done) return (
    <AuthFrame title={done === 'approved' ? `${device?.device} is signed in` : 'Not approved'} lede={done === 'approved' ? 'It’s signed in with your Portico Account. You can sign it out any time from Settings › Devices.' : 'The TV wasn’t signed in. If you didn’t start this, you can ignore it.'}>
      <div className={s.center}><div className={cx(s.centerIcon, done === 'approved' ? s.centerIconGood : s.centerIconBad)}><Icon name={done === 'approved' ? 'check' : 'close'} size={28} /></div></div>
      <Button variant="secondary" block label={session.phase === 'signedIn' ? 'Back to Portico' : 'Done'} to={session.phase === 'signedIn' ? '/' : '/welcome'} />
    </AuthFrame>
  );
  if (device) return (
    <AuthFrame title="Sign in this TV?" lede="Only approve a device you’re setting up right now.">
      <dl className={s.facts}><div><dt>Device</dt><dd>{device.device}</dd></div><div><dt>Kind</dt><dd>{device.platform}</dd></div><div><dt>Where</dt><dd>{device.location}</dd></div><div><dt>Account</dt><dd>{session.session?.account.username ?? 'justin'}</dd></div></dl>
      <Button variant="primary" size="lg" block label="Approve" busy={busy} onClick={async () => { setBusy(true); await client.session.confirmDevice(code, true); setDone('approved'); }} />
      <Button variant="ghost" block label="Don’t approve" onClick={async () => { await client.session.confirmDevice(code, false); setDone('denied'); }} />
    </AuthFrame>
  );
  return (
    <AuthFrame title="Link a TV" lede="Enter the code your TV shows. It signs the TV in with your Portico Account.">
      <form style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}} onSubmit={async e => { e.preventDefault(); setBusy(true); setError(undefined); try { setDevice(await client.session.approveDevice(code)); } catch (err) { setError(err instanceof Error ? err.message : 'That code wasn’t found.'); } finally { setBusy(false); } }}>
        <TextField label="Code from the TV" placeholder="K7QD-2MXF" value={code} onChange={v => { setCode(v.toUpperCase()); setError(undefined); }} autoFocus error={error} autoComplete="off" spellCheck={false} style={{fontSize: 22, letterSpacing: '0.16em', fontWeight: 700, textAlign: 'center', height: 56}} />
        <Button type="submit" variant="primary" size="lg" block label="Continue" busy={busy} disabled={code.replace(/[^a-z0-9]/gi, '').length < 8} />
      </form>
      {session.phase !== 'signedIn' ? <div className={s.links}><Link to="/sign-in">Sign in first</Link></div> : <div className={s.links}><Link to="/settings">Back to Settings</Link></div>}
    </AuthFrame>
  );
}

function Join() {
  const search = useRouteSearch<{code?: string}>();
  const client = useClient();
  const navigate = useNavigate();
  const code = search.code ?? (location.hash.match(/code=([\w-]+)/)?.[1] ?? 'LIGHTHOUSE-INVITE');
  const inv = useQuery(c => c.session.invitation(code), [code]);
  const [how, setHow] = useState<'account' | 'local'>('account');
  const [user, setUser] = useState('');
  const [pw, setPw] = useState('');
  const [done, setDone] = useState<'accepted' | 'declined'>();
  const [busy, setBusy] = useState(false);
  useTitle('Invitation');
  if (!inv.data) return <AuthFrame title="Opening your invitation…"><div style={{display: 'flex', justifyContent: 'center'}}><Spinner /></div></AuthFrame>;
  if (inv.data.state !== 'valid') return <AuthFrame title="This invitation has expired" lede={`Ask ${inv.data.inviter} to send a new one. Invitations work for 7 days.`}><Button variant="secondary" block label="Back" to="/welcome" /></AuthFrame>;
  if (done) return <AuthFrame title={done === 'accepted' ? `Welcome to ${inv.data.server}` : 'Invitation declined'} lede={done === 'accepted' ? `${inv.data.inviter} shared ${inv.data.libraries} libraries with you.` : `We’ve let ${inv.data.inviter} know.`}>{done === 'accepted' ? <Button variant="primary" size="lg" block label="Start watching" onClick={() => void navigate({to: '/sign-in/profiles' as never})} /> : <Button variant="secondary" block label="Done" to="/welcome" />}</AuthFrame>;
  return (
    <AuthFrame title={`${inv.data.inviter} invited you to ${inv.data.server}`} lede={`You’ll be able to watch ${inv.data.libraries} of their libraries.`}>
      <RadioCards label="How to join" value={how} onChange={setHow} options={[{value: 'account', label: 'With my Portico Account', help: 'Recommended. Use the server from any device you’re signed in on.'}, {value: 'local', label: 'With a username and password', help: 'An account on this server only.'}]} />
      {how === 'local' ? <><TextField label="Choose a username" value={user} onChange={setUser} autoComplete="username" /><TextField type="password" label="Choose a password" value={pw} onChange={setPw} autoComplete="new-password" help="At least 8 characters." /></> : null}
      <Button variant="primary" size="lg" block label="Accept invitation" busy={busy} disabled={how === 'local' && (!user || pw.length < 8)} onClick={async () => { setBusy(true); await client.session.acceptInvitation(code, how === 'account' ? {account: true} : {username: user, password: pw}); setDone('accepted'); }} />
      <Button variant="ghost" block label="Decline" onClick={async () => { await client.session.declineInvitation(code); setDone('declined'); }} />
    </AuthFrame>
  );
}

function Claim() {
  const client = useClient();
  const req = useQuery(c => c.session.claimRequest(), []);
  const [done, setDone] = useState<'approved' | 'denied'>();
  const [busy, setBusy] = useState(false);
  useTitle('Connect a server');
  if (!req.data) return <AuthFrame title="Loading the request…"><div style={{display: 'flex', justifyContent: 'center'}}><Spinner /></div></AuthFrame>;
  if (done) return <AuthFrame title={done === 'approved' ? `${req.data.server} is connected` : 'Request denied'} lede={done === 'approved' ? 'It’s on your Portico Account now. You can reach it from any device you’re signed in on, and share it with others.' : 'The server wasn’t connected. If you didn’t start this, you can ignore it.'}><Button variant="secondary" block label="Done" to="/" /></AuthFrame>;
  return (
    <AuthFrame title={`Connect ${req.data.server} to your Portico Account?`} lede="A Portico Server asked to join your account. Approve it only if you’re setting it up right now.">
      <dl className={s.facts}><div><dt>Server</dt><dd>{req.data.server}</dd></div><div><dt>Address</dt><dd>{req.data.address}</dd></div><div><dt>Asked</dt><dd>{req.data.requested}</dd></div></dl>
      <Button variant="primary" size="lg" block label="Connect server" busy={busy} onClick={async () => { setBusy(true); await client.session.answerClaim(true); setDone('approved'); }} />
      <Button variant="ghost" block label="Deny" onClick={async () => { await client.session.answerClaim(false); setDone('denied'); }} />
    </AuthFrame>
  );
}

/** First run of a new server: name, owner, libraries, remote access. */
function Setup() {
  const client = useClient();
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [name, setName] = useState('Lighthouse');
  const [mode, setMode] = useState<'hosted' | 'local'>('hosted');
  const [owner, setOwner] = useState('');
  const [pw, setPw] = useState('');
  const [folders, setFolders] = useState<{kind: string; path: string}[]>([{kind: 'Movies', path: '/media/Movies'}, {kind: 'TV Shows', path: '/media/TV'}]);
  const [remote, setRemote] = useState(true);
  const [busy, setBusy] = useState(false);
  useTitle('Set up Portico Server');
  const steps = ['Name', 'Owner', 'Libraries', 'Away from home'];
  const titles = ['Name your server', 'Who owns this server?', 'Add your media', 'Watch away from home?'];
  return (
    <AuthFrame title={step < 4 ? titles[step] : `${name} is ready`} lede={step === 0 ? 'Welcome to Portico Server. A few questions and your media is ready to watch.' : step === 4 ? 'Portico is scanning your folders now. Titles appear as they’re found.' : undefined}>
      {step < 4 ? <div className={s.steps} aria-label={`Step ${step + 1} of 4: ${steps[step]}`}>{steps.map((x, i) => <span key={x} data-on={i <= step} />)}</div> : null}
      {step === 0 ? <TextField label="Server name" help="Shown in the server switcher on every device." value={name} onChange={setName} autoFocus /> : null}
      {step === 1 ? <><RadioCards label="Owner" value={mode} onChange={setMode} options={[{value: 'hosted', label: 'My Portico Account', help: 'Recommended. Reach and share the server from anywhere.'}, {value: 'local', label: 'An account on this server only', help: 'Stays on your network. You can connect a Portico Account later.'}]} />{mode === 'local' ? <><TextField label="Username" value={owner} onChange={setOwner} /><TextField type="password" label="Password" value={pw} onChange={setPw} help="At least 8 characters." /></> : <p className="t-caption c-3">You’ll approve the connection on getportico.tv in the next step.</p>}</> : null}
      {step === 2 ? (
        <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 8}}>
          {folders.map((f, i) => <div key={i} className={s.choice} style={{cursor: 'default'}}><span className={s.choiceIcon}><Icon name="folder" size={20} /></span><span className={s.choiceText}><span className={s.choiceTitle} style={{display: 'block'}}>{f.kind}</span><span className={s.choiceHelp}>{f.path}</span></span><Button size="sm" variant="ghost" icon="close" label="" aria-label={`Remove ${f.kind}`} onClick={() => setFolders(folders.filter((_, j) => j !== i))} /></div>)}
          <Button variant="secondary" icon="plus" label="Add a library" onClick={() => setFolders([...folders, {kind: ['Music', 'Anime', 'Audiobooks', 'Photos', 'Home Videos'][folders.length % 5]!, path: `/media/${['Music', 'Anime', 'Audiobooks', 'Photos', 'Home Videos'][folders.length % 5]}`}])} />
          <p className="t-caption c-3">You can add more libraries and folders any time under Manage server › Libraries.</p>
        </div>
      ) : null}
      {step === 3 ? <><RadioCards label="Remote access" value={remote ? 'on' : 'off'} onChange={v => setRemote(v === 'on')} options={[{value: 'on', label: 'Yes, let me watch away from home', help: 'Portico sets up the connection. Your media stays on your server.'}, {value: 'off', label: 'Only on my home network', help: 'You can turn this on later in Manage server › Network.'}]} /></> : null}
      {step === 4 ? <div className={s.center}><div className={cx(s.centerIcon, s.centerIconGood)}><Icon name="check" size={28} /></div></div> : null}
      <div style={{display: 'flex', gap: 8}}>
        {step > 0 && step < 4 ? <Button variant="ghost" label="Back" onClick={() => setStep(step - 1)} /> : null}
        <span className="grow" />
        {step < 3 ? <Button variant="primary" label="Continue" disabled={(step === 0 && !name.trim()) || (step === 1 && mode === 'local' && (!owner || pw.length < 8))} onClick={() => setStep(step + 1)} /> : step === 3 ? <Button variant="primary" label="Finish setup" busy={busy} onClick={async () => { setBusy(true); await client.session.setupServer({name, mode, owner, password: pw}); setBusy(false); setStep(4); }} /> : <Button variant="primary" block label="Start watching" onClick={() => void navigate({to: '/' as never})} />}
      </div>
    </AuthFrame>
  );
}

function Restore() {
  const client = useClient();
  const navigate = useNavigate();
  const [slow, setSlow] = useState(false);
  useTitle('Connecting');
  useEffect(() => {
    const t = setTimeout(() => setSlow(true), 2500);
    const t2 = setTimeout(() => { void client.session.restore().then(r => navigate({to: (r === 'restored' ? '/' : '/unavailable') as never})); }, 5200);
    return () => { clearTimeout(t); clearTimeout(t2); };
  }, [client, navigate]);
  return (
    <AuthFrame title="Connecting to Lighthouse" lede={slow ? 'This is taking longer than usual. The server may be busy or starting up.' : 'Picking up where you left off.'}>
      <div className={s.center}><Spinner size={36} /></div>
      {slow ? <Button variant="ghost" block label="Choose another server" to="/sign-in/servers" /> : null}
    </AuthFrame>
  );
}

function VerifyEmail() {
  const search = useRouteSearch<{token?: string}>();
  const r = useQuery(c => c.session.verifyEmail(search.token ?? 'ok'), [search.token]);
  useTitle('Confirm your email');
  if (!r.data) return <AuthFrame title="Confirming your email…"><div className={s.center}><Spinner /></div></AuthFrame>;
  return r.data === 'confirmed'
    ? <AuthFrame title="Your email is confirmed" lede="Thanks. Account notices and password resets go to this address now."><div className={s.center}><div className={cx(s.centerIcon, s.centerIconGood)}><Icon name="check" size={28} /></div></div><Button variant="primary" block label="Open Portico" to="/" /></AuthFrame>
    : <AuthFrame title="This link has expired" lede="Links work for 24 hours. Sign in and we’ll send a new one."><div className={s.center}><div className={cx(s.centerIcon, s.centerIconBad)}><Icon name="clock" size={28} /></div></div><Button variant="secondary" block label="Sign in" to="/sign-in" /></AuthFrame>;
}

function ResetPassword() {
  const search = useRouteSearch<{owner?: string}>();
  const client = useClient();
  const [pw, setPw] = useState('');
  const [code, setCode] = useState('');
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const owner = search.owner === '1';
  useTitle('Choose a new password');
  if (done) return <AuthFrame title="Password changed" lede="Sign in with your new password. Other devices stay signed in."><Button variant="primary" block label="Sign in" to={owner ? '/sign-in/direct' : '/sign-in'} /></AuthFrame>;
  return (
    <AuthFrame title={owner ? 'Reset the owner password' : 'Choose a new password'} lede={owner ? 'On the server’s own computer, open Portico Server and choose Owner recovery to get a code. Enter it here.' : 'Your account has two-step sign-in, so enter a code from your authenticator app or a recovery code too.'}>
      <form style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}} onSubmit={async e => { e.preventDefault(); setBusy(true); setError(undefined); try { await client.session.resetPassword(pw, code); setDone(true); } catch (err) { setError(err instanceof Error ? err.message : 'That didn’t work.'); } finally { setBusy(false); } }}>
        <TextField label={owner ? 'Recovery code from the server' : 'Authenticator or recovery code'} value={code} onChange={setCode} autoFocus autoComplete="one-time-code" />
        <TextField type="password" label="New password" value={pw} onChange={v => { setPw(v); setError(undefined); }} autoComplete="new-password" error={error} help="At least 8 characters." />
        <Button type="submit" variant="primary" size="lg" block label="Change password" busy={busy} disabled={!pw || !code} />
      </form>
    </AuthFrame>
  );
}

function TogetherJoin() {
  const session = useSession();
  const client = useClient();
  const navigate = useNavigate();
  const code = location.hash.match(/code=(\w+)/)?.[1] ?? 'HRBR7QXT';
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useTitle('Join a watch party');
  return (
    <AuthFrame title="You’re invited to watch together" lede="Friday Movie Night, hosted by Maya. You’ll watch in step with everyone in the group.">
      <div className={s.code}>{code.slice(0, 4)} {code.slice(4)}</div>
      {session.phase === 'signedIn' ? <Button variant="primary" size="lg" block label="Join the group" busy={busy} onClick={async () => { setBusy(true); try { await client.together.join(code); void navigate({to: '/together' as never}); } catch (e) { setError(e instanceof Error ? e.message : 'That didn’t work.'); } finally { setBusy(false); } }} /> : <Button variant="primary" size="lg" block label="Sign in to join" to="/sign-in" />}
      {error ? <p className="c-danger t-caption" role="alert">{error}</p> : null}
    </AuthFrame>
  );
}

function AccountReturn() {
  const navigate = useNavigate();
  useTitle('Finishing sign-in');
  useEffect(() => { const t = setTimeout(() => void navigate({to: '/sign-in/servers' as never, replace: true}), 1800); return () => clearTimeout(t); }, [navigate]);
  return <AuthFrame title="Finishing sign-in…" lede="You’re back from your Apple or Google account. One moment."><div className={s.center}><Spinner size={32} /></div></AuthFrame>;
}

function Unavailable() {
  const navigate = useNavigate();
  const [left, setLeft] = useState(15);
  const [trying, setTrying] = useState(false);
  useTitle('Server unavailable');
  useEffect(() => { const t = setInterval(() => setLeft(n => (n <= 1 ? 15 : n - 1)), 1000); return () => clearInterval(t); }, []);
  return (
    <AuthFrame title="Studio Annex isn’t reachable" lede="Portico last heard from it 3 days ago. It may be turned off, asleep or offline." foot="If you own it, check that the computer is on and connected to the internet.">
      <div className={s.center}><div className={cx(s.centerIcon, s.centerIconBad)}><Icon name="offline" size={28} /></div><p className="t-caption c-3">{trying ? 'Trying now…' : `Trying again in ${left} s`}</p></div>
      <Button variant="primary" block icon="refresh" label="Try now" busy={trying} onClick={() => { setTrying(true); setTimeout(() => { setTrying(false); setLeft(15); }, 1500); }} />
      <Button variant="secondary" block icon="server" label="Choose another server" onClick={() => void navigate({to: '/sign-in/servers' as never})} />
      <Button variant="ghost" block icon="download" label="Watch downloads" to="/offline" />
    </AuthFrame>
  );
}

function Offline() {
  return <PlayerProvider><SelectionProvider><ActionsProvider><OfflineBody /><PlayerHost /></ActionsProvider></SelectionProvider></PlayerProvider>;
}

function OfflineBody() {
  const player = usePlayer();
  const q = useQuery(c => c.downloads.list(), []);
  useTitle('Offline');
  const ready = q.data?.items.filter(d => d.state === 'ready') ?? [];
  return (
    <AuthFrame title="You’re offline" lede="These downloads play without a connection. Everything else comes back when you’re online." wide>
      <div className={s.downloads}>
        {ready.map(d => <div key={d.id} className={s.choice}><Artwork src={d.entry.image} title={d.entry.title} style={{width: 44}} /><span className={s.choiceText}><span className={s.choiceTitle} style={{display: 'block'}}>{d.entry.title}</span><span className={s.choiceHelp}>{d.quality} · {d.size}{d.expires ? ` · Expires ${d.expires}` : ''}</span></span><Button size="sm" variant="primary" icon="play" label="Play" onClick={() => player.play([d.entry], 0)} /></div>)}
        {q.data && !ready.length ? <p className="c-3">No downloads are ready in this browser.</p> : null}
      </div>
      <div className={s.links}><Link to="/welcome">Try connecting again</Link></div>
    </AuthFrame>
  );
}
