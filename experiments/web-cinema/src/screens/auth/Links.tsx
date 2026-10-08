import {useEffect, useState} from 'react';
import {Link, useLocation, useNavigate, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import {useSession} from '../../app/session';
import {together} from '../../app/together';
import {Button} from '../../ui/Button';
import {ChoiceCards, Field, Input, PasswordInput} from '../../ui/Controls';
import {Notice, Spinner} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Avatar, BrandSymbol} from '../../ui/Media';
import {AuthFrame} from './AuthFrame';

/** Pages reached from a link in an email or on another device. Secrets arrive in the URL fragment. */
export function LinkScreen() {
  const path = useLocation().pathname;
  switch (path) {
    case '/device': return <DeviceApproval />;
    case '/claim': return <Claim />;
    case '/reset': return <Reset />;
    case '/verify-email': return <VerifyEmail />;
    case '/join': return <Join />;
    case '/together/join': return <TogetherJoin />;
    default: return <AccountReturn />;
  }
}

const fragment = (key: string) => new URLSearchParams(location.hash.slice(1)).get(key) ?? undefined;
function Heading({title, lede, icon}: {title: string; lede?: React.ReactNode; icon?: React.ReactNode}) {
  return <div className="auth-heading">{icon}<h1 className="display auth-title">{title}</h1>{lede ? <p className="auth-lede">{lede}</p> : null}</div>;
}

function DeviceApproval() {
  const services = useServices();
  const search = useSearch({strict: false}) as {code?: string};
  const [code, setCode] = useState(search.code ?? '');
  const [device, setDevice] = useState<{name: string; platform: string; location: string} | null>(null);
  const [scope, setScope] = useState<'server' | 'account'>('server');
  const [state, setState] = useState<'enter' | 'review' | 'approved' | 'declined'>('enter');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  return (
    <AuthFrame title="Link a TV">
      {state === 'enter' ? (
        <>
          <Heading icon={<span className="auth-glyph"><Icon name="tv" size={28} /></span>} title="Link a TV" lede="Enter the code your TV shows. It signs the TV in as you." />
          <form className="stack" onSubmit={async e => { e.preventDefault(); setBusy(true); const d = await services.auth.lookupDevice(code); setBusy(false); if (!d) return setError('That code didn’t work. Codes change every 15 minutes; check the TV for the current one.'); setDevice(d); setState('review'); }}>
            <Field label="Code on your TV" error={error || undefined}><Input value={code} onChange={e => { setCode(e.target.value.toUpperCase()); setError(''); }} placeholder="KQ7-MXP" className="pin-input" autoFocus maxLength={8} autoComplete="off" /></Field>
            <Button full size="lg" variant="accent" label="Continue" busy={busy} disabled={code.replace(/[^A-Z0-9]/gi, '').length < 6} type="submit" />
          </form>
        </>
      ) : state === 'review' && device ? (
        <>
          <Heading icon={<span className="auth-glyph"><Icon name="tv" size={28} /></span>} title={`Sign in on ${device.name}?`} lede={<>{device.platform} · {device.location}. Only approve a TV you’re in front of.</>} />
          <ChoiceCards label="Sign the TV in to" columns={1} value={scope} onChange={setScope} options={[{value: 'server', title: 'Just this server', body: 'The TV can use Ehler Home Theater as you.'}, {value: 'account', title: 'Your Portico Account', body: 'The TV can reach all your servers.'}]} />
          <div className="auth-nav"><Button variant="ghost" label="Deny" onClick={() => setState('declined')} /><Button variant="accent" label="Approve" busy={busy} onClick={async () => { setBusy(true); await services.auth.approveDevice(code); setBusy(false); setState('approved'); }} /></div>
        </>
      ) : state === 'approved' ? (
        <Heading icon={<span className="auth-glyph is-good"><Icon name="check" size={28} /></span>} title="Your TV is signed in" lede="It continues on its own in a moment. You can sign it out in Settings › Account." />
      ) : (
        <Heading icon={<span className="auth-glyph is-warn"><Icon name="close" size={28} /></span>} title="Not approved" lede="The TV wasn’t signed in. If you didn’t start this, you can ignore it." />
      )}
      {state === 'approved' || state === 'declined' ? <Button full variant="secondary" label="Back to Portico" to="/" /> : null}
    </AuthFrame>
  );
}

function Claim() {
  const services = useServices();
  const claim = useQuery('claim', () => services.auth.claim());
  const [state, setState] = useState<'ask' | 'done' | 'denied'>('ask');
  const [busy, setBusy] = useState(false);
  return (
    <AuthFrame title="Connect a server">
      {state === 'ask' ? (
        <>
          <Heading icon={<span className="auth-glyph"><BrandSymbol size={28} /></span>} title="Connect this server to your account?" lede="A Portico server asked to be connected to your Portico Account. You’ll own it, and can reach it from anywhere." />
          {claim.data ? <div className="claim-card"><strong>{claim.data.server}</strong><span className="faint small">{claim.data.address} · asked {Math.round((Date.now() - claim.data.requested) / 60000)} minutes ago</span></div> : <Spinner />}
          <Notice tone="info">Only approve a server you just set up yourself.</Notice>
          <div className="auth-nav"><Button variant="ghost" label="Deny" onClick={() => setState('denied')} /><Button variant="accent" label="Connect server" busy={busy} onClick={() => { setBusy(true); setTimeout(() => { setBusy(false); setState('done'); }, 800); }} /></div>
        </>
      ) : state === 'done' ? (
        <>
          <Heading icon={<span className="auth-glyph is-good"><Icon name="check" size={28} /></span>} title={`${claim.data?.server} is yours`} lede="It’s connected to your Portico Account. Your main profile on it is ready." />
          <Button full size="lg" variant="accent" label="Open the server" to="/" />
        </>
      ) : <Heading icon={<span className="auth-glyph is-warn"><Icon name="close" size={28} /></span>} title="Not connected" lede="The server stays unclaimed. You can connect it later from its console." />}
    </AuthFrame>
  );
}

function Reset() {
  const services = useServices();
  const token = fragment('token');
  const [p, setP] = useState('');
  const [r, setR] = useState('');
  const [code, setCode] = useState('');
  const [done, setDone] = useState(false);
  useEffect(() => { if (location.hash) history.replaceState(null, '', location.pathname); }, []);
  return (
    <AuthFrame title="Reset password" footer={<Link to="/welcome/$screen" params={{screen: 'signin'}} className="link">Back to sign in</Link>}>
      {done ? (
        <><Heading icon={<span className="auth-glyph is-good"><Icon name="check" size={28} /></span>} title="Password changed" lede="Sign in with your new password. Other devices were signed out." /><Button full size="lg" variant="accent" label="Sign in" to="/welcome/$screen" params={{screen: 'signin'}} /></>
      ) : (
        <>
          <Heading icon={<span className="auth-glyph"><Icon name="key" size={28} /></span>} title="Choose a new password" lede={token ? 'For justin@example.com' : 'For your Portico Account'} />
          <form className="stack" onSubmit={async e => { e.preventDefault(); await services.auth.resetPassword(p, code); setDone(true); }}>
            <Field label="New password" help="At least 10 characters."><PasswordInput value={p} onChange={e => setP(e.target.value)} autoComplete="new-password" autoFocus /></Field>
            <Field label="Repeat password" error={r && r !== p ? 'The passwords don’t match.' : undefined}><PasswordInput value={r} onChange={e => setR(e.target.value)} autoComplete="new-password" /></Field>
            <Field label="Two-step code" help="Your account uses two-step verification." optional><Input value={code} onChange={e => setCode(e.target.value)} inputMode="numeric" autoComplete="one-time-code" /></Field>
            <Button full size="lg" variant="accent" label="Change password" disabled={p.length < 10 || p !== r} type="submit" />
          </form>
        </>
      )}
    </AuthFrame>
  );
}

function VerifyEmail() {
  const services = useServices();
  const token = fragment('token') ?? 'ok';
  const result = useQuery(`verify.${token}`, () => services.auth.verifyEmail(token));
  return (
    <AuthFrame title="Verify email">
      {!result.data ? <Heading icon={<span className="auth-glyph"><Spinner size={28} /></span>} title="Verifying your email…" /> : result.data === 'done' ? (
        <><Heading icon={<span className="auth-glyph is-good"><Icon name="check" size={28} /></span>} title="Email verified" lede="Thanks. Your Portico Account is all set." /><Button full size="lg" variant="accent" label="Continue to Portico" to="/" /></>
      ) : (
        <><Heading icon={<span className="auth-glyph is-warn"><Icon name="clock" size={28} /></span>} title="This link has expired" lede="Verification links work for 24 hours. Sign in and we’ll send a new one." /><Button full size="lg" variant="accent" label="Sign in" to="/welcome/$screen" params={{screen: 'signin'}} /></>
      )}
    </AuthFrame>
  );
}

function Join() {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const invite = useQuery('invite', () => services.auth.invitation(fragment('invite') ?? 'sample'));
  const profiles = useQuery('profiles.join', () => services.account.profiles());
  const [mode, setMode] = useState<'account' | 'direct'>('account');
  const [profile, setProfile] = useState('pr-justin');
  const [f, setF] = useState({code: '', name: '', username: '', password: ''});
  const [state, setState] = useState<'ask' | 'joined' | 'declined'>('ask');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  if (state !== 'ask') return (
    <AuthFrame title="Invitation">
      {state === 'joined' ? <><Heading icon={<span className="auth-glyph is-good"><Icon name="check" size={28} /></span>} title={`Welcome to ${invite.data?.server}`} lede="You can switch to it from the profile menu any time." /><Button full size="lg" variant="accent" label="Open it now" onClick={() => void navigate({to: '/'})} /></> : <Heading icon={<span className="auth-glyph is-warn"><Icon name="close" size={28} /></span>} title="Invitation declined" lede={`${invite.data?.inviter} won’t be told why.`} />}
    </AuthFrame>
  );
  return (
    <AuthFrame title="You’re invited">
      <Heading icon={invite.data ? <Avatar name={invite.data.inviter} color="#e8bc6a" size={64} /> : <Spinner />} title={invite.data ? `${invite.data.inviter} invited you to ${invite.data.server}` : 'Checking your invitation…'} lede={invite.data ? `${invite.data.libraries} libraries shared with you.` : undefined} />
      <div className="auth-tabs">
        <button type="button" className={mode === 'account' ? 'is-on' : ''} onClick={() => setMode('account')}>With my Portico Account</button>
        <button type="button" className={mode === 'direct' ? 'is-on' : ''} onClick={() => setMode('direct')}>With a code</button>
      </div>
      {mode === 'account' ? (
        session.signedIn ? (
          <div className="stack">
            <div className="field"><span className="field-label">Join as</span>
              <div className="join-profiles">{(profiles.data ?? []).filter(p => !p.kids).map(p => <button key={p.id} type="button" className={profile === p.id ? 'is-on' : ''} onClick={() => setProfile(p.id)}><Avatar name={p.name} color={p.color} size={40} />{p.name}</button>)}</div>
            </div>
            <div className="auth-nav"><Button variant="ghost" label="Decline" onClick={() => setState('declined')} /><Button variant="accent" label="Accept invitation" busy={busy} onClick={async () => { setBusy(true); await services.auth.acceptInvitation('sample', profile); setBusy(false); setState('joined'); }} /></div>
          </div>
        ) : <Button full size="lg" variant="accent" label="Sign in to accept" to="/welcome/$screen" params={{screen: 'signin'}} />
      ) : (
        <form className="stack" onSubmit={async e => { e.preventDefault(); setBusy(true); setError(''); try { await services.auth.joinDirect(f.code, f.name, f.username, f.password); setState('joined'); } catch (err) { setError((err as Error).message); } finally { setBusy(false); } }}>
          {error ? <Notice tone="bad">{error}</Notice> : null}
          <Field label="Invitation code"><Input value={f.code} onChange={e => setF({...f, code: e.target.value})} autoFocus /></Field>
          <Field label="Your name"><Input value={f.name} onChange={e => setF({...f, name: e.target.value})} autoComplete="name" /></Field>
          <Field label="Choose a username"><Input value={f.username} onChange={e => setF({...f, username: e.target.value})} autoComplete="username" /></Field>
          <Field label="Password" help="At least 10 characters."><PasswordInput value={f.password} onChange={e => setF({...f, password: e.target.value})} autoComplete="new-password" /></Field>
          <Button full size="lg" variant="accent" label="Join server" busy={busy} disabled={!f.code || !f.username || f.password.length < 10} type="submit" />
        </form>
      )}
    </AuthFrame>
  );
}

function TogetherJoin() {
  const services = useServices();
  const navigate = useNavigate();
  const code = fragment('code') ?? 'KXWP7M2Q';
  const group = useQuery(`together.join.${code}`, () => services.together.join(code));
  return (
    <AuthFrame title="Watch Together">
      {group.loading ? <Heading icon={<span className="auth-glyph"><Spinner size={28} /></span>} title="Finding the group…" /> : group.data ? (
        <>
          <Heading icon={<span className="auth-glyph"><Icon name="people" size={28} /></span>} title={`Join ${group.data.name}?`} lede={`${group.data.members.map(m => m.name).join(', ')} ${group.data.members.length === 1 ? 'is' : 'are'} here. Your player will stay in step with the group.`} />
          <Button full size="lg" variant="accent" icon="people" label="Join the group" onClick={() => { together.join(group.data!.id); void navigate({to: '/together', search: {group: group.data!.id}}); }} />
        </>
      ) : <Heading icon={<span className="auth-glyph is-warn"><Icon name="people" size={28} /></span>} title="This group has ended" lede="Ask the host for a new code." />}
    </AuthFrame>
  );
}

function AccountReturn() {
  const navigate = useNavigate();
  const session = useSession();
  useEffect(() => {
    const t = setTimeout(() => { session.signIn('account'); void navigate({to: '/welcome/$screen', params: {screen: 'servers'}}); }, 1800);
    return () => clearTimeout(t);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <AuthFrame title="Signing in">
      <Heading icon={<span className="auth-glyph"><Spinner size={28} /></span>} title="Finishing sign-in…" lede="You’re signed in with Apple. Taking you to your servers." />
    </AuthFrame>
  );
}
