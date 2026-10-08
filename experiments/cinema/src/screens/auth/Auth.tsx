import React, {useEffect, useState} from 'react';
import {ago} from '../../design/time';
import {ActivityIndicator, StyleSheet, View} from 'react-native';
import QRCode from 'react-native-qrcode-svg';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useAction, useQuery, useServices} from '../../data/hooks';
import type {Server} from '../../data/types';
import {Button} from '../../design/Button';
import {Badge, ListRow, TextField, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {go, replaceAll, type Routes} from '../../nav/routes';
import {useSession} from '../../state/session';
import {AuthFrame, Divider} from './AuthFrame';


// ---------------------------------------------------------------- Sign in (all modes)

export function SignInScreen() {
  const route = useRoute<RouteProp<Routes, 'SignIn'>>();
  const mode = route.params?.mode ?? 'account';
  if (mode === 'direct' || mode === 'recovery') return <DirectSignIn recovery={mode === 'recovery'} />;
  if (mode === 'create') return <CreateAccount />;
  if (mode === 'forgot') return <ForgotPassword />;
  if (mode === 'code') return <CodeStep />;
  if (mode === 'newPassword') return <NewPassword />;
  return <AccountSignIn />;
}

function AccountSignIn() {
  const s = useServices();
  const session = useSession();
  const [user, setUser] = useState('');
  const [pass, setPass] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {busy, run} = useAction();
  const submit = () =>
    run(async () => {
      setError(null);
      try {
        const r = await s.auth.signIn(user, pass);
        if (r.needsCode) go('SignIn', {mode: 'code'});
        else {
          session.signIn('account');
          replaceAll('Chooser');
        }
      } catch (e) {
        setError((e as Error).message);
      }
    });
  if (isTV) {
    return (
      <AuthFrame title="Welcome home." lede="Sign in with your Portico Account to reach your servers." wide>
        <View style={{flexDirection: 'row', gap: 60, alignItems: 'center'}}>
          <View style={{flex: 1}}>
            <Text variant="heading">Use your phone</Text>
            <Text variant="body" tone="secondary" style={{marginTop: 8}}>Scan the code, or go to getportico.tv/link on any device and enter the code shown on the next screen.</Text>
            <Button label="Show a code" icon="qr" variant="primary" style={{marginTop: 24, alignSelf: 'flex-start'}} onPress={() => go('LinkTV')} hasTVPreferredFocus />
          </View>
          <View style={{width: 1, alignSelf: 'stretch', backgroundColor: colors.line}} />
          <View style={{flex: 1}}>
            <Text variant="heading">Use the remote</Text>
            <TextField label="Username or email" value={user} onChangeText={setUser} />
            <TextField label="Password" value={pass} onChangeText={setPass} secure error={error} />
            <Button label="Sign in" busy={busy} onPress={submit} />
          </View>
        </View>
        <View style={{flexDirection: 'row', gap: 20, marginTop: 30, justifyContent: 'center'}}>
          <Button label="Sign in directly to a server" variant="ghost" onPress={() => go('SignIn', {mode: 'direct'})} />
        </View>
      </AuthFrame>
    );
  }
  return (
    <AuthFrame title="Welcome home." lede="Sign in with your Portico Account to access servers shared with you.">
      <Button label="Sign in with Apple" icon="phone" variant="secondary" onPress={() => { session.signIn('account'); replaceAll('Chooser'); }} />
      <View style={{height: 10}} />
      <Button label="Sign in with Google" icon="globe" variant="secondary" onPress={() => ui.toast('Google sign-in opens in a browser window.')} />
      <Divider label="or" />
      <TextField label="Username or email" value={user} onChangeText={setUser} placeholder="you@example.com" />
      <TextField label="Password" value={pass} onChangeText={setPass} secure error={error} onSubmit={submit} />
      <Button label="Sign in" variant="primary" busy={busy} onPress={submit} />
      <View style={styles.links}>
        <Button label="Forgot password?" variant="ghost" size="sm" onPress={() => go('SignIn', {mode: 'forgot'})} />
        <Button label="Create account" variant="ghost" size="sm" onPress={() => go('SignIn', {mode: 'create'})} />
      </View>
      <Divider label="other ways in" />
      <ListRow icon="server" label="Sign in directly to a server" help="Use an account that lives on your server" onPress={() => go('SignIn', {mode: 'direct'})} />
      <ListRow icon="mail" label="Have an invitation? Join this server" onPress={() => go('Join')} />
      <ListRow icon="tv" label="Link a TV" help="Approve the code a TV is showing" onPress={() => go('DeviceApprove')} />
    </AuthFrame>
  );
}

function DirectSignIn({recovery}: {recovery: boolean}) {
  const s = useServices();
  const session = useSession();
  const [addr, setAddr] = useState(recovery ? 'portico.home.arpa:32500' : '');
  const [user, setUser] = useState('');
  const [pass, setPass] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {busy, run} = useAction();
  const submit = () =>
    run(async () => {
      setError(null);
      try {
        const r = await s.auth.signInDirect(addr, user, pass);
        if (r.mustChangePassword) return go('SignIn', {mode: 'newPassword'});
        session.signIn('direct');
        replaceAll('Profiles');
      } catch (e) {
        setError((e as Error).message);
      }
    });
  return (
    <AuthFrame title={recovery ? 'Owner recovery sign-in' : 'Sign in to a server'} lede={recovery ? 'Use the owner account stored on this server when Portico Accounts can’t be reached.' : 'Use an account that lives on your server. It stays between this device and that server.'}>
      <TextField label="Server address" value={addr} onChangeText={setAddr} placeholder="192.168.1.20:32500" keyboardType="url" />
      <TextField label="Username" value={user} onChangeText={setUser} />
      <TextField label="Password" value={pass} onChangeText={setPass} secure error={error} onSubmit={submit} help={recovery ? 'Forgot this password? On the server’s computer, run portico-server recover-owner.' : undefined} />
      <Button label="Sign in" variant="primary" busy={busy} onPress={submit} hasTVPreferredFocus={isTV} />
      <View style={styles.links}>
        <Button label="Use a Portico Account" variant="ghost" size="sm" onPress={() => go('SignIn', {mode: 'account'})} />
        {!recovery ? <Button label="Owner recovery" variant="ghost" size="sm" onPress={() => go('SignIn', {mode: 'recovery'})} /> : null}
        <Button label="Join with an invitation" variant="ghost" size="sm" onPress={() => go('Join', {direct: true})} />
      </View>
    </AuthFrame>
  );
}

function CreateAccount() {
  const s = useServices();
  const [f, setF] = useState({name: '', username: '', email: '', password: ''});
  const [done, setDone] = useState(false);
  const {busy, run} = useAction();
  if (done) return <AuthFrame title="Check your email" lede={`We sent a code to ${f.email || 'your email'}. Enter it to finish creating your account.`}><CodeInput onDone={() => replaceAll('Chooser')} /></AuthFrame>;
  return (
    <AuthFrame title="Create your Portico Account" lede="One sign-in for every server and device.">
      <TextField label="Your name" value={f.name} onChangeText={v => setF({...f, name: v})} autoCapitalize="words" />
      <TextField label="Username" value={f.username} onChangeText={v => setF({...f, username: v})} help="3 to 64 characters, no spaces or @." />
      <TextField label="Email" value={f.email} onChangeText={v => setF({...f, email: v})} keyboardType="email-address" />
      <TextField label="Password" value={f.password} onChangeText={v => setF({...f, password: v})} secure help="At least 8 characters. Longer is stronger." />
      <Button label="Create account" variant="primary" busy={busy} onPress={() => run(async () => { await s.auth.createAccount(f.name, f.username, f.email, f.password); setDone(true); })} />
    </AuthFrame>
  );
}

function ForgotPassword() {
  const s = useServices();
  const [v, setV] = useState('');
  const [sent, setSent] = useState(false);
  const {busy, run} = useAction();
  return (
    <AuthFrame title="Forgot your password?" lede={sent ? 'If that matches a Portico Account, a reset link is on its way. It works for 30 minutes.' : 'Enter the username and email on your Portico Account and we’ll send a reset link.'}>
      {sent ? (
        <Button label="Back to sign in" variant="primary" onPress={() => go('SignIn', {mode: 'account'})} />
      ) : (
        <>
          <TextField label="Username or email" value={v} onChangeText={setV} />
          <Button label="Send reset link" variant="primary" busy={busy} onPress={() => run(async () => { await s.auth.requestPasswordReset(v); setSent(true); })} />
        </>
      )}
    </AuthFrame>
  );
}

function CodeInput({onDone}: {onDone: () => void}) {
  const s = useServices();
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {busy, run} = useAction();
  return (
    <>
      <TextField label="Code" value={code} onChangeText={setCode} keyboardType="number-pad" maxLength={11} placeholder="123 456" error={error} />
      <Button label="Continue" variant="primary" busy={busy} hasTVPreferredFocus={isTV} onPress={() => run(async () => { try { await s.auth.verifyCode(code); onDone(); } catch (e) { setError((e as Error).message); } })} />
      <Button label="Send the code again" variant="ghost" size="sm" style={{marginTop: 8, alignSelf: 'center'}} onPress={() => ui.toast('A new code is on its way.')} />
    </>
  );
}

function CodeStep() {
  const session = useSession();
  return (
    <AuthFrame title="Enter your code" lede="Two-step verification is on for this account. Enter the code from your authenticator app, or a recovery code.">
      <CodeInput onDone={() => { session.signIn('account'); replaceAll('Chooser'); }} />
    </AuthFrame>
  );
}

function NewPassword() {
  const [a, setA] = useState('');
  const [b, setB] = useState('');
  const mismatch = b.length > 0 && a !== b;
  return (
    <AuthFrame title="Choose a new password" lede="The server’s owner asked you to choose a new password before you continue.">
      <TextField label="New password" value={a} onChangeText={setA} secure help="At least 8 characters. Longer is stronger." />
      <TextField label="Confirm password" value={b} onChangeText={setB} secure error={mismatch ? 'The passwords don’t match.' : null} />
      <Button label="Continue" variant="primary" disabled={!a || mismatch} onPress={() => replaceAll('Profiles')} />
    </AuthFrame>
  );
}

// ---------------------------------------------------------------- Server chooser

export function ChooserScreen() {
  const s = useServices();
  const session = useSession();
  const q = useQuery('servers', () => s.auth.servers());
  const open = (srv: Server) => {
    if (srv.status !== 'online') return go('ServerUnavailable');
    session.setServer(srv);
    replaceAll('Profiles');
  };
  return (
    <AuthFrame title="Choose a server" lede="Your Portico Account has more than one server. Choose one to open. You can switch later in Settings.">
      {!q.data ? <ActivityIndicator color={colors.accentStrong} /> : q.data.map((srv, i) => (
        <Focusable key={srv.id} onPress={() => open(srv)} hasTVPreferredFocus={i === 0} scale={isTV ? 1.03 : 1} accessibilityLabel={`${srv.name}, ${srv.status}`}>
          {({focused}) => {
            const lit = focused && isTV;
            return (
              <View style={[styles.server, lit && {backgroundColor: colors.white}]}>
                <View style={[styles.serverIcon, {backgroundColor: srv.status === 'online' ? 'rgba(134,207,179,0.16)' : 'rgba(243,139,136,0.14)'}]}>
                  <Icon name="server" size={20} color={srv.status === 'online' ? colors.healthy : colors.danger} />
                </View>
                <View style={{flex: 1, marginLeft: u(12)}}>
                  <Text variant="bodyStrong" color={lit ? colors.actionInk : colors.text}>{srv.name}</Text>
                  <Text variant="caption" color={lit ? 'rgba(3,18,28,0.7)' : colors.textTertiary}>{srv.status === 'online' ? `Online · ${srv.role === 'owner' ? 'You own this server' : srv.role === 'admin' ? 'Administrator' : 'Member'}` : `Can’t reach · last seen ${ago(srv.lastSeen)}`}</Text>
                </View>
                {srv.id === session.server?.id ? <Badge label="Current" /> : null}
              </View>
            );
          }}
        </Focusable>
      ))}
      <View style={styles.links}>
        <Button label="Sign in directly to another server" variant="ghost" size="sm" onPress={() => go('SignIn', {mode: 'direct'})} />
        <Button label="Refresh servers" variant="ghost" size="sm" icon="refresh" onPress={q.reload} />
      </View>
    </AuthFrame>
  );
}

export function ServerUnavailableScreen() {
  return (
    <AuthFrame title="Portico can’t reach Maya’s Archive" lede="It was last online 3 days ago. Portico will connect automatically when it’s back.">
      <View style={styles.pulse}><Icon name="cloudOff" size={40} color={colors.amber} /></View>
      <Button label="Check now" icon="refresh" variant="primary" hasTVPreferredFocus onPress={() => ui.toast('Still can’t reach Maya’s Archive.', {tone: 'error'})} />
      <View style={styles.links}>
        <Button label="Choose another server" variant="ghost" size="sm" onPress={() => go('Chooser')} />
        <Button label="Sign in directly to this server" variant="ghost" size="sm" onPress={() => go('SignIn', {mode: 'direct'})} />
      </View>
    </AuthFrame>
  );
}

export function OfflineScreen() {
  return (
    <AuthFrame title="You’re offline" lede="This device can’t reach the internet or your server. Downloads on this device still play.">
      <View style={styles.pulse}><Icon name="wifiOff" size={40} color={colors.amber} /></View>
      <Button label="Try again" icon="refresh" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('Restore')} />
      {!isTV ? <Button label="Open downloads" variant="ghost" style={{marginTop: 8}} onPress={() => go('Downloads')} /> : null}
    </AuthFrame>
  );
}

export function SignInEndedScreen() {
  return (
    <AuthFrame title="This sign-in has ended" lede="Your sign-in to Ehler Home Theater has ended. Sign in again to continue.">
      <Button label="Sign in again" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('SignIn')} />
    </AuthFrame>
  );
}

export function RestoreScreen() {
  const [phase, setPhase] = useState<'checking' | 'waiting'>('checking');
  useEffect(() => {
    const t = setTimeout(() => setPhase('waiting'), 1800);
    return () => clearTimeout(t);
  }, []);
  return (
    <AuthFrame title={phase === 'checking' ? 'Reconnecting' : 'Waiting for your server'} lede={phase === 'checking' ? 'Checking your saved connection.' : 'Your server is temporarily unavailable. We’ll bring you back here automatically when it reconnects.'}>
      <ActivityIndicator color={colors.accentStrong} size="large" style={{marginBottom: u(16)}} />
      <Button label="Check now" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('Profiles')} />
      <Button label="Use another account or server" variant="ghost" style={{marginTop: 8}} onPress={() => replaceAll('SignIn')} />
    </AuthFrame>
  );
}

export function DeviceWaitingScreen() {
  return (
    <AuthFrame title="Waiting for approval" lede="This device signed in but can’t browse or play until the server’s owner approves it under Server › Access.">
      <View style={styles.pulse}><Icon name="shieldCheck" size={40} color={colors.accentStrong} /></View>
      <Button label="Check again" icon="refresh" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('Profiles')} />
      <Button label="Use another account" variant="ghost" style={{marginTop: 8}} onPress={() => replaceAll('SignIn')} />
    </AuthFrame>
  );
}

// ---------------------------------------------------------------- Invitations

export function JoinScreen() {
  const route = useRoute<RouteProp<Routes, 'Join'>>();
  const s = useServices();
  const inv = useQuery('invitation', () => s.auth.invitation('token'));
  const profiles = useQuery('profiles', () => s.account.profiles());
  const [profile, setProfile] = useState('pr-justin');
  const [answer, setAnswer] = useState<'accepted' | 'declined' | null>(null);
  const [f, setF] = useState({address: 'cabin.ehler.net:32500', code: '', name: '', username: '', password: '', confirm: ''});
  const [error, setError] = useState<string | null>(null);
  const {busy, run} = useAction();
  if (route.params?.direct) {
    return (
      <AuthFrame title="Join this server" lede="Enter the server’s address and your invitation code, then choose a username and password.">
        <TextField label="Server address" value={f.address} onChangeText={v => setF({...f, address: v})} />
        <TextField label="Invitation code" value={f.code} onChangeText={v => setF({...f, code: v})} placeholder="H7K2-P9QX" />
        <TextField label="Your name" value={f.name} onChangeText={v => setF({...f, name: v})} help="Shown on your profile. You can change it later." autoCapitalize="words" />
        <TextField label="Username" value={f.username} onChangeText={v => setF({...f, username: v})} help="You sign in with it. 3 to 64 characters, no spaces or @." />
        <TextField label="Password" value={f.password} onChangeText={v => setF({...f, password: v})} secure />
        <TextField label="Confirm password" value={f.confirm} onChangeText={v => setF({...f, confirm: v})} secure error={f.confirm && f.confirm !== f.password ? 'The passwords don’t match.' : error} />
        <Button label="Join and sign in" variant="primary" busy={busy} onPress={() => run(async () => { try { await s.auth.joinDirect(f.code, f.name, f.username, f.password); replaceAll('Profiles'); } catch (e) { setError((e as Error).message); } })} />
      </AuthFrame>
    );
  }
  if (answer) {
    return (
      <AuthFrame title={answer === 'accepted' ? 'You’re in' : 'Invitation declined'} lede={answer === 'accepted' ? `You’ve joined ${inv.data?.server}. It appears in your servers.` : `You declined the invitation to ${inv.data?.server}.`}>
        <Button label="Continue" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('Chooser')} />
      </AuthFrame>
    );
  }
  return (
    <AuthFrame title="You’re invited" lede={inv.data ? `${inv.data.inviter} invited you to ${inv.data.server}.` : 'Looking up the invitation'}>
      {inv.data ? (
        <>
          <View style={styles.inviteCard}>
            <Icon name="server" size={22} color={colors.accentStrong} />
            <View style={{marginLeft: 12, flex: 1}}>
              <Text variant="bodyStrong">{inv.data.server}</Text>
              <Text variant="caption" tone="tertiary">{inv.data.libraries} libraries shared with you · expires in 5 days</Text>
            </View>
          </View>
          <Text variant="captionStrong" tone="secondary" style={{marginVertical: 8}}>Which profile should use this server?</Text>
          {(profiles.data ?? []).map(p => <ListRow key={p.id} label={p.name} onPress={() => setProfile(p.id)} chevron={false} right={profile === p.id ? <Icon name="check" size={18} color={colors.accentStrong} /> : undefined} />)}
          <View style={[styles.links, {marginTop: 16}]}>
            <Button label="Decline" onPress={() => setAnswer('declined')} />
            <Button label="Accept invitation" variant="primary" busy={busy} onPress={() => run(async () => { await s.auth.acceptInvitation('token', profile); setAnswer('accepted'); })} hasTVPreferredFocus />
          </View>
        </>
      ) : <ActivityIndicator color={colors.accentStrong} />}
    </AuthFrame>
  );
}

// ---------------------------------------------------------------- Linking a TV

export function LinkTVScreen() {
  const s = useServices();
  const q = useQuery('tvcode', () => s.auth.tvLinkCode());
  const session = useSession();
  return (
    <AuthFrame title="Sign in with your phone" lede="Scan the code with your phone’s camera, or go to getportico.tv/link on any device and enter this code." wide>
      <View style={{flexDirection: isTV ? 'row' : 'column', alignItems: 'center', justifyContent: 'center', gap: u(30)}}>
        <View style={styles.qr}>{q.data ? <QRCode value={`https://${q.data.url}?code=${q.data.code}`} size={isTV ? 320 : 180} backgroundColor="#ffffff" color={colors.ink} /> : null}</View>
        <View style={{alignItems: 'center'}}>
          <Text variant="caption" tone="tertiary">Your code</Text>
          <Text style={{fontFamily: 'Menlo', fontSize: isTV ? 96 : 44, color: colors.text, letterSpacing: 6}}>{q.data?.code ?? '···-···'}</Text>
          <View style={{flexDirection: 'row', alignItems: 'center', gap: 8, marginTop: 10}}>
            <ActivityIndicator color={colors.accentStrong} />
            <Text variant="caption" tone="secondary">Waiting for you to approve it · expires in 15 minutes</Text>
          </View>
          <View style={{flexDirection: 'row', gap: 12, marginTop: 24}}>
            <Button label="Simulate approval" variant="primary" hasTVPreferredFocus onPress={() => { session.signIn('account'); replaceAll('Chooser'); }} />
            <Button label="Get a new code" icon="refresh" onPress={q.reload} />
          </View>
        </View>
      </View>
    </AuthFrame>
  );
}

export function DeviceApproveScreen() {
  const s = useServices();
  const [code, setCode] = useState('');
  const [device, setDevice] = useState<{name: string; platform: string; location: string} | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const {busy, run} = useAction();
  if (done) return <AuthFrame title="TV signed in" lede={`${device?.name} can now use your Portico Account.`}><Button label="Done" variant="primary" onPress={() => go('Home')} /></AuthFrame>;
  return (
    <AuthFrame title="Link a TV" lede="Type the code shown on the TV to sign it in. Approving as Justin Ehler.">
      {!device ? (
        <>
          <TextField label="Code" value={code} onChangeText={v => setCode(v.toUpperCase())} placeholder="KQ7-MXP" autoCapitalize="none" error={error} />
          <Button label="Review device" variant="primary" busy={busy} onPress={() => run(async () => { const d = await s.auth.lookupDevice(code); if (d) setDevice(d); else setError('That code wasn’t found. Check it on the TV and try again.'); })} />
        </>
      ) : (
        <>
          <View style={styles.inviteCard}>
            <Icon name="tv" size={24} color={colors.accentStrong} />
            <View style={{marginLeft: 12}}>
              <Text variant="bodyStrong">{device.name}</Text>
              <Text variant="caption" tone="tertiary">{device.platform} · {device.location}</Text>
            </View>
          </View>
          <Text variant="caption" tone="amber" style={{marginVertical: 10}}>Approve only if these details match the device in front of you.</Text>
          <View style={styles.links}>
            <Button label="Not mine" onPress={() => { setDevice(null); setCode(''); }} />
            <Button label="Sign it in" variant="primary" busy={busy} onPress={() => run(async () => { await s.auth.approveDevice(code); setDone(true); })} />
          </View>
        </>
      )}
    </AuthFrame>
  );
}

// ---------------------------------------------------------------- Claim, setup and email links

export function ClaimScreen() {
  const s = useServices();
  const q = useQuery('claim', () => s.auth.claim());
  const [state, setState] = useState<'ask' | 'approved' | 'denied'>('ask');
  if (state !== 'ask') return <AuthFrame title={state === 'approved' ? 'Connected' : 'Not connected'} lede={state === 'approved' ? `${q.data?.server} is connected to your Portico Account. It finishes setting up on its own.` : `You declined. ${q.data?.server} isn’t connected. You can close this window.`}><Button label={`Return to ${q.data?.server ?? 'Portico'}`} variant="primary" onPress={() => go('Home')} /></AuthFrame>;
  return (
    <AuthFrame title={`Connect ${q.data?.server ?? 'a server'}`} lede="Connecting lets people sign in with their Portico Account and reach this server away from home.">
      {[['Server', q.data?.server], ['Address', q.data?.address], ['Portico Account', 'justin@example.com'], ['Requested', '2 minutes ago'], ['Owner profile', 'Justin']].map(([k, v]) => (
        <View key={k} style={styles.kv}><Text variant="caption" tone="tertiary">{k}</Text><Text variant="bodyStrong">{v ?? '…'}</Text></View>
      ))}
      <Text variant="caption" tone="amber" style={{marginVertical: 12}}>Approve only if you started this from your own server and the address above is yours.</Text>
      <View style={styles.links}>
        <Button label="Deny" onPress={() => setState('denied')} />
        <Button label="Approve" variant="primary" hasTVPreferredFocus onPress={() => setState('approved')} />
      </View>
    </AuthFrame>
  );
}

export function SetupScreen() {
  const s = useServices();
  const [step, setStep] = useState(0);
  const [name, setName] = useState('Ehler Home Theater');
  const [mode, setMode] = useState<'account' | 'local'>('account');
  const [user, setUser] = useState('admin');
  const [pass, setPass] = useState('');
  const [confirm, setConfirm] = useState('');
  const [saved, setSaved] = useState(false);
  const {busy, run} = useAction();
  const steps = [
    <View key="0">
      <TextField label="Server name" value={name} onChangeText={setName} help="Shown on every device that connects." autoCapitalize="words" />
      <Button label="Continue" variant="primary" onPress={() => setStep(1)} hasTVPreferredFocus />
    </View>,
    <View key="1">
      <Text variant="heading" style={{marginBottom: 8}}>How will people sign in?</Text>
      <ListRow icon="user" label="Portico Accounts" help="Recommended. One sign-in for all your apps, easy invitations and access away from home." onPress={() => setMode('account')} chevron={false} right={mode === 'account' ? <Icon name="checkCircle" size={20} color={colors.accentStrong} /> : undefined} hasTVPreferredFocus />
      <ListRow icon="server" label="Keep accounts on this server" help="Works without the internet. You’ll manage every sign-in yourself." onPress={() => setMode('local')} chevron={false} right={mode === 'local' ? <Icon name="checkCircle" size={20} color={colors.accentStrong} /> : undefined} />
      <Button label="Continue" variant="primary" style={{marginTop: 12}} onPress={() => setStep(2)} />
    </View>,
    <View key="2">
      <Text variant="heading">{mode === 'account' ? 'Create a recovery owner' : 'Create the owner account'}</Text>
      <Text variant="caption" tone="tertiary" style={{marginBottom: 10}}>{mode === 'account' ? 'This account lives on the server and gets you back in if your Portico Account is ever unavailable.' : 'You will use this to sign in and to manage the server.'}</Text>
      <TextField label="Username" value={user} onChangeText={setUser} />
      <TextField label="Password" value={pass} onChangeText={setPass} secure help="At least 8 characters. Longer is stronger." />
      <TextField label="Confirm password" value={confirm} onChangeText={setConfirm} secure error={confirm && confirm !== pass ? 'The passwords don’t match.' : null} />
      <ToggleRow label="I saved this password somewhere safe" value={saved} onChange={setSaved} />
      <Button label={mode === 'account' ? 'Create recovery owner' : 'Create owner'} variant="primary" busy={busy} disabled={!saved || !pass || pass !== confirm} style={{marginTop: 10}} onPress={() => run(async () => { await s.auth.setupServer({name, mode, username: user, password: pass}); setStep(3); })} />
    </View>,
    <View key="3" style={{alignItems: 'center'}}>
      <Icon name="checkCircle" size={44} color={colors.healthy} />
      <Text variant="body" tone="secondary" center style={{marginVertical: 12}}>{mode === 'account' ? 'Next, you’ll connect the server to your Portico Account.' : 'Your server is ready. Add your first library to fill it.'}</Text>
      <Button label={mode === 'account' ? 'Connect to Portico Account' : 'Add a library'} variant="primary" hasTVPreferredFocus onPress={() => (mode === 'account' ? go('Claim') : go('Admin', {section: 'libraries'}))} />
    </View>,
  ];
  return (
    <AuthFrame title="Welcome to Portico" lede={step < 3 ? `Let’s set up your server. It takes about a minute. · Step ${step + 1} of 3` : 'All set'}>
      <View style={{flexDirection: 'row', gap: 6, marginBottom: 16}}>
        {[0, 1, 2].map(i => <View key={i} style={{flex: 1, height: 4, borderRadius: 2, backgroundColor: i <= step ? colors.accentStrong : colors.trackRest}} />)}
      </View>
      {steps[step]}
    </AuthFrame>
  );
}

export function VerifyEmailScreen() {
  const s = useServices();
  const route = useRoute<RouteProp<Routes, 'VerifyEmail'>>();
  const q = useQuery('verify:' + (route.params?.token ?? 'ok'), () => s.auth.verifyEmail(route.params?.token ?? 'ok'));
  if (!q.data) return <AuthFrame title="Confirming your email"><ActivityIndicator color={colors.accentStrong} /></AuthFrame>;
  return q.data === 'done' ? (
    <AuthFrame title="Email confirmed" lede="Your email is confirmed. Go back to the device where you started, or continue here."><Button label="Continue to Portico" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('Profiles')} /></AuthFrame>
  ) : (
    <AuthFrame title="This link has expired" lede="This link has expired or was already used. Ask for a new code from Portico."><Button label="Sign in" variant="primary" onPress={() => replaceAll('SignIn')} /></AuthFrame>
  );
}

export function ResetPasswordScreen() {
  const s = useServices();
  const [a, setA] = useState('');
  const [b, setB] = useState('');
  const [code, setCode] = useState('');
  const [done, setDone] = useState(false);
  const {busy, run} = useAction();
  if (done) return <AuthFrame title="Password changed" lede="Your password is changed. Sign in with the new one."><Button label="Sign in" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('SignIn')} /></AuthFrame>;
  return (
    <AuthFrame title="Choose a new password" lede="For your Portico Account.">
      <TextField label="New password" value={a} onChangeText={setA} secure help="At least 8 characters. Longer is stronger." />
      <TextField label="Confirm new password" value={b} onChangeText={setB} secure error={b && a !== b ? 'The passwords don’t match.' : null} />
      <TextField label="Authenticator or recovery code" value={code} onChangeText={setCode} help="Only if two-step verification is on for this account." />
      <Button label="Change password" variant="primary" busy={busy} disabled={!a || a !== b} onPress={() => run(async () => { await s.auth.resetPassword(a, code); setDone(true); })} />
    </AuthFrame>
  );
}

export function AccountReturnScreen() {
  return (
    <AuthFrame title="Finishing sign-in" lede="Returning to the Portico app to finish signing in.">
      <ActivityIndicator color={colors.accentStrong} style={{marginBottom: 16}} />
      <Button label="Open Portico" variant="primary" hasTVPreferredFocus onPress={() => replaceAll('Profiles')} />
    </AuthFrame>
  );
}

export function NotFoundScreen() {
  return (
    <View style={{flex: 1, justifyContent: 'center'}}>
      <EmptyState icon="compass" title="This page isn’t here" body="The link may be out of date, or the title was removed from your library." action={{label: 'Go home', icon: 'home', onPress: () => go('Home')}} />
    </View>
  );
}

const styles = StyleSheet.create({
  links: {flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: 8, marginTop: 12},
  server: {flexDirection: 'row', alignItems: 'center', padding: u(12), borderRadius: radius.md, marginBottom: u(6), backgroundColor: 'rgba(237,245,250,0.05)'},
  serverIcon: {width: u(40), height: u(40), borderRadius: u(12), alignItems: 'center', justifyContent: 'center'},
  pulse: {alignSelf: 'center', width: u(88), height: u(88), borderRadius: u(44), backgroundColor: 'rgba(232,188,106,0.1)', alignItems: 'center', justifyContent: 'center', marginBottom: u(20)},
  inviteCard: {flexDirection: 'row', alignItems: 'center', padding: u(14), borderRadius: radius.md, backgroundColor: 'rgba(92,179,226,0.1)', borderWidth: 1, borderColor: 'rgba(92,179,226,0.3)', marginBottom: u(10)},
  qr: {padding: u(14), backgroundColor: '#fff', borderRadius: radius.lg},
  kv: {flexDirection: 'row', justifyContent: 'space-between', paddingVertical: u(8), borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.line},
});
