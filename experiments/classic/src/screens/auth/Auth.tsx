import React, {useEffect, useState} from 'react';
import {ActivityIndicator, View} from 'react-native';
import Svg, {Path} from 'react-native-svg';
import {useNavigation, useRoute} from '@react-navigation/native';
import QRCode from 'react-native-qrcode-svg';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import {Button} from '../../ui/Button';
import {Badge, ListRow, Loading, Notice, TextField, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {Focusable} from '../../ui/Focusable';
import {colors} from '../../theme/tokens';
import {isTV, useMetrics} from '../../theme/layout';
import {useSession} from '../../app/session';
import {AuthFrame} from './AuthFrame';
import {usePlayer} from '../../player/PlayerProvider';
import {PlayerHost} from '../../player/PlayerHost';
import {Icon} from '../../ui/Icon';
import {useToast} from '../../app/toast';
import {serverLine} from '../../app/format';
import {ProfileGrid} from '../../shell/ProfileMenu';
import {PasswordStrength} from '../settings/Sections';
import type {AuthRoutes} from '../../nav/routes';

type Nav = {navigate: (name: keyof AuthRoutes, params?: object) => void; goBack: () => void; canGoBack: () => boolean; replace: (name: keyof AuthRoutes, params?: object) => void};
const useNav = () => useNavigation() as unknown as Nav;
const useBack = () => { const n = useNav(); return n.canGoBack() ? () => n.goBack() : undefined; };

const APPLE = 'M12.152 6.896c-.948 0-2.415-1.078-3.96-1.04-2.04.027-3.91 1.183-4.961 3.014-2.117 3.675-.546 9.103 1.519 12.09 1.013 1.454 2.208 3.09 3.792 3.039 1.52-.065 2.09-.987 3.935-.987 1.831 0 2.35.987 3.96.948 1.637-.026 2.676-1.48 3.676-2.948 1.156-1.688 1.636-3.325 1.662-3.415-.039-.013-3.182-1.221-3.22-4.857-.026-3.04 2.48-4.494 2.597-4.559-1.429-2.09-3.623-2.324-4.39-2.376-2-.156-3.675 1.09-4.61 1.09zM15.53 3.83c.843-1.012 1.4-2.427 1.245-3.83-1.207.052-2.662.805-3.532 1.818-.78.896-1.454 2.338-1.273 3.714 1.338.104 2.715-.688 3.559-1.701';
const GOOGLE: [string, string][] = [
  ['#EA4335', 'M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z'],
  ['#4285F4', 'M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z'],
  ['#FBBC05', 'M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z'],
  ['#34A853', 'M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z'],
];

/** Sign in with Apple (white style) and Sign in with Google (dark style), per each provider's button guidelines. */
function ProviderButton({provider, onPress, disabled}: {provider: 'apple' | 'google'; onPress: () => void; disabled?: boolean}) {
  const m = useMetrics();
  const apple = provider === 'apple';
  return (
    <Focusable onPress={onPress} disabled={disabled} accessibilityRole="button" accessibilityLabel={apple ? 'Sign in with Apple' : 'Sign in with Google'}
      style={{height: m.control + 4, borderRadius: m.radius.control, backgroundColor: apple ? '#ffffff' : '#131314', borderWidth: apple ? 0 : 1, borderColor: '#8e918f', flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 8}}>
      <Svg width={apple ? 17 : 18} height={apple ? 20 : 18} viewBox={apple ? '0 0 24 24' : '0 0 48 48'} style={{marginTop: apple ? -2 : 0}}>
        {apple ? <Path d={APPLE} fill="#000000" /> : GOOGLE.map(([c, d]) => <Path key={c} d={d} fill={c} />)}
      </Svg>
      <Text style={{fontFamily: 'Manrope-SemiBold', fontSize: 16, color: apple ? '#000000' : '#e3e3e3'}}>{apple ? 'Sign in with Apple' : 'Sign in with Google'}</Text>
    </Focusable>
  );
}

function LinkText({label, onPress}: {label: string; onPress: () => void}) {
  return (
    <Focusable onPress={onPress} accessibilityRole="link" style={({focused}) => ({alignSelf: 'flex-start', paddingVertical: 4, paddingHorizontal: 6, marginHorizontal: -6, borderRadius: 8, backgroundColor: focused ? colors.focus : 'transparent'})}>
      {({focused}) => <Text variant="bodyStrong" color={focused ? colors.accentInk : colors.accentStrong}>{label}</Text>}
    </Focusable>
  );
}

export function LandingScreen() {
  const nav = useNav();
  const m = useMetrics();
  return (
    <AuthFrame title="Welcome home." lede="Your movies, shows, music and more, from your own Portico server.">
      <View style={{gap: 10}}>
        <Button label="Sign in with Portico Account" icon="user" variant="primary" size="lg" grow onPress={() => nav.navigate('AccountSignIn')} hasTVPreferredFocus />
        {m.isTV ? <Button label="Use a code from another device" icon="qr" size="lg" grow onPress={() => nav.navigate('Activate')} /> : null}
        <Button label="Sign in directly to a server" icon="server" size="lg" grow onPress={() => nav.navigate('DirectSignIn')} />
        <Button label="Have an invitation? Join this server" icon="mail" variant="ghost" grow onPress={() => nav.navigate('Join')} />
        {!m.isTV ? <Button label="Watch downloads" icon="download" variant="ghost" grow onPress={() => nav.navigate('Offline')} /> : null}
      </View>
      <Text variant="caption" tone="tertiary">One Portico Account reaches every server you own or that someone shares with you. By continuing, you agree to the Terms and Privacy Policy.</Text>
    </AuthFrame>
  );
}

export function AccountSignInScreen() {
  const route = useRoute() as {params?: {mode?: 'signIn' | 'create' | 'forgot'}};
  const nav = useNav();
  const back = useBack();
  const m = useMetrics();
  const client = useClient();
  const toast = useToast();
  const [mode, setMode] = useState(route.params?.mode ?? 'signIn');
  const [identity, setIdentity] = useState('justin');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [sent, setSent] = useState(false);
  const {busy, run, error, clearError} = useAction();
  const title = mode === 'create' ? 'Create your account' : mode === 'forgot' ? 'Forgot your password?' : 'Sign in to Portico';
  const lede = mode === 'create' ? 'One Portico Account reaches every server you own or that someone shares with you.' : mode === 'forgot' ? 'Enter the email on your account. We’ll email you a link to reset your password.' : 'Use your Portico Account to reach every server you own or that someone shares with you.';
  return (
    <AuthFrame title={title} lede={lede} onBack={back}>
      {error ? <Notice tone="error" body={error} /> : null}
      {mode === 'signIn' ? (
        <>
          {!m.isTV ? (
            <View style={{gap: 8}}>
              <ProviderButton provider="apple" disabled={busy} onPress={() => void run(async () => { const r = await client.session.signInAccount('apple', 'provider'); nav.navigate(r.twoStep ? 'TwoStep' : 'ServerChooser', {next: 'servers'}); })} />
              <ProviderButton provider="google" disabled={busy} onPress={() => void run(async () => { const r = await client.session.signInAccount('google', 'provider'); nav.navigate(r.twoStep ? 'TwoStep' : 'ServerChooser', {next: 'servers'}); })} />
              <Text variant="caption" tone="tertiary" center>or sign in with email</Text>
            </View>
          ) : <Text variant="caption" tone="tertiary">To sign in with Google or Apple, go back and use a code from another device.</Text>}
          <TextField label="Username or email" value={identity} onChangeText={setIdentity} autoCapitalize="none" autoCorrect={false} textContentType="username" />
          <TextField label="Password" value={password} onChangeText={setPassword} secureTextEntry textContentType="password" />
          <Button label="Sign in" variant="primary" size="lg" grow loading={busy} onPress={() => void run(async () => { const r = await client.session.signInAccount(identity, password); nav.navigate(r.twoStep ? 'TwoStep' : 'ServerChooser', {next: 'servers'}); })} hasTVPreferredFocus />
          <View style={{flexDirection: 'row', gap: 16, flexWrap: 'wrap'}}>
            <LinkText label="Forgot password?" onPress={() => { clearError(); setMode('forgot'); }} />
            <LinkText label="Create account" onPress={() => { clearError(); setMode('create'); }} />
            {m.isTV ? <LinkText label="Use a code from another device" onPress={() => nav.navigate('Activate')} /> : null}
          </View>
        </>
      ) : null}
      {mode === 'create' ? (
        sent ? (
          <>
            <Text variant="subheading">Check your email</Text>
            <Text variant="body" tone="secondary">We sent a code to {email || 'your address'}. Enter it to finish creating your account.</Text>
            <TextField label="Code" keyboardType="number-pad" />
            <Button label="Continue" variant="primary" grow onPress={() => nav.navigate('ServerChooser')} />
            <LinkText label="Send the code again" onPress={() => toast({message: `We sent a new code to ${email || 'your address'}.`, icon: 'mail'})} />
          </>
        ) : (
          <>
            <TextField label="Choose a username" help="Letters, numbers, dots and dashes. People use it to share a server with you." autoCapitalize="none" value={identity} onChangeText={setIdentity} />
            <TextField label="Email" keyboardType="email-address" autoCapitalize="none" value={email} onChangeText={setEmail} />
            <TextField label="Password" secureTextEntry value={password} onChangeText={setPassword} />
            <PasswordStrength value={password} />
            <Button label="Create account" variant="primary" size="lg" grow loading={busy} disabled={!email.includes('@') || password.length < 8} onPress={() => void run(async () => { await client.session.createAccount({username: identity, email, password}); setSent(true); })} />
            <LinkText label="Back to sign in" onPress={() => setMode('signIn')} />
          </>
        )
      ) : null}
      {mode === 'forgot' ? (
        sent ? <Notice tone="success" body="We’ve sent a link to reset your password. Open it on any device, then sign in here with your new password." action={{label: 'Back to sign in', onPress: () => { setSent(false); setMode('signIn'); }}} /> : (
          <>
            <TextField label="Email" keyboardType="email-address" autoCapitalize="none" value={email} onChangeText={setEmail} />
            <Text variant="caption" tone="tertiary">For your privacy the answer is the same whether or not an account matches. If you signed up with Google or Apple, there is no Portico password to reset.</Text>
            <Button label="Email me a link" variant="primary" grow loading={busy} onPress={() => void run(async () => { await client.session.requestPasswordReset(email); setSent(true); })} />
            <LinkText label="Back to sign in" onPress={() => setMode('signIn')} />
          </>
        )
      ) : null}
    </AuthFrame>
  );
}

export function TwoStepScreen() {
  const nav = useNav();
  const client = useClient();
  const [code, setCode] = useState('');
  const {busy, run, error} = useAction();
  return (
    <AuthFrame title="Enter your verification code" lede="Two-step verification is on for this account. Enter the code from your authenticator app, or a recovery code." onBack={useBack()}>
      {error ? <Notice tone="error" body={error} /> : null}
      <TextField label="Verification code" value={code} onChangeText={setCode} keyboardType="number-pad" placeholder="123456" autoFocus maxLength={9} />
      <Button label="Verify" variant="primary" size="lg" grow loading={busy} disabled={code.length < 6} onPress={() => void run(async () => { await client.session.verifyCode(code); nav.navigate('ServerChooser'); })} hasTVPreferredFocus />
      <Text variant="caption" tone="tertiary">Lost your authenticator? Enter one of your recovery codes, like k2m9-x7qa.</Text>
    </AuthFrame>
  );
}

export function DirectSignInScreen() {
  const nav = useNav();
  const client = useClient();
  const m = useMetrics();
  const [host, setHost] = useState('192.168.1.20:32500');
  const [user, setUser] = useState('justin');
  const [pw, setPw] = useState('');
  const [recovery, setRecovery] = useState(false);
  const {busy, run, error} = useAction();
  return (
    <AuthFrame title={recovery ? 'Owner recovery sign-in' : 'Sign in to a server'} lede={recovery ? 'Use the owner account stored on this server when Portico Accounts can’t be reached.' : 'Use an account that lives on your server. It stays between this device and that server.'} onBack={useBack()}>
      {error ? <Notice tone="error" body={error} /> : null}
      <TextField label="Server address" value={host} onChangeText={setHost} autoCapitalize="none" autoCorrect={false} keyboardType="url" help="It usually looks like 192.168.1.20:32500." />
      <TextField label="Username" value={user} onChangeText={setUser} autoCapitalize="none" />
      <TextField label="Password" value={pw} onChangeText={setPw} secureTextEntry />
      <Button label="Sign in" variant="primary" size="lg" grow loading={busy} onPress={() => void run(async () => { await client.session.signInDirect(host, user, pw); nav.navigate('Profiles'); })} hasTVPreferredFocus />
      <View style={{flexDirection: 'row', gap: 16, flexWrap: 'wrap'}}>
        {m.isTV ? <LinkText label="Use Quick Connect instead" onPress={() => nav.navigate('QuickConnect')} /> : null}
        <LinkText label={recovery ? 'Back to server sign-in' : 'Owner recovery sign-in'} onPress={() => setRecovery(r => !r)} />
        <LinkText label="Use a Portico Account" onPress={() => nav.navigate('AccountSignIn')} />
      </View>
      {recovery ? <Text variant="caption" tone="tertiary">Forgot this password? On the server’s computer, run portico-server recover-owner.</Text> : null}
    </AuthFrame>
  );
}

export function ServerChooserScreen() {
  const nav = useNav();
  const client = useClient();
  const servers = useQuery(c => c.session.servers(), []);
  const {run, busy} = useAction();
  return (
    <AuthFrame title="Choose a server" lede="Your Portico Account has more than one server. Choose one to open. You can switch later in Settings." onBack={useBack()}>
      {servers.loading ? <Loading label="Finding your servers" /> : null}
      {servers.data?.map((s, i) => (
        <ListRow key={s.id} icon="server" title={s.name} subtitle={serverLine(s)} hasTVPreferredFocus={i === 0} disabled={busy}
          trailing={<Badge label={s.status === 'online' ? 'Online' : 'Can’t reach'} tone={s.status === 'online' ? 'good' : 'bad'} />}
          onPress={() => void run(async () => { const r = await client.session.chooseServer(s.id); nav.navigate(r.status === 'online' ? 'Profiles' : 'ServerUnavailable', {serverId: s.id}); })} />
      ))}
    </AuthFrame>
  );
}

export function ProfilesScreen() {
  const m = useMetrics();
  return (
    <AuthFrame title="Who’s watching?" wide onBack={useBack()}>
      <ProfileGrid />
      <Text variant="caption" tone="tertiary" center>{m.isTV ? 'Profiles, PINs and limits are managed on your phone or computer.' : 'Manage profiles in Settings › Profiles.'}</Text>
    </AuthFrame>
  );
}

export function JoinScreen() {
  const route = useRoute() as {params?: {code?: string}};
  const client = useClient();
  const nav = useNav();
  const [code, setCode] = useState(route.params?.code ?? '');
  const [withPassword, setWithPassword] = useState(false);
  const [user, setUser] = useState('');
  const [pw, setPw] = useState('');
  const [done, setDone] = useState<'joined' | 'declined'>();
  const inv = useQuery(c => (code.length >= 8 ? c.session.invitation(code) : Promise.resolve(undefined)), [code]);
  const {busy, run, error} = useAction();
  const back = useBack();
  if (done) return <AuthFrame title={done === 'joined' ? 'You’ve joined Lighthouse.' : 'You declined the invitation to Lighthouse.'} lede={done === 'joined' ? 'It appears in your servers.' : 'The person who invited you will see that you declined.'}><Button label="Continue to Portico" variant="primary" grow onPress={() => nav.navigate(done === 'joined' ? 'Profiles' : 'Landing')} hasTVPreferredFocus /></AuthFrame>;
  return (
    <AuthFrame title="You’re invited" lede={inv.data ? `${inv.data.inviter} invited you to ${inv.data.server}.` : 'Enter the code from your invitation to see who invited you.'} onBack={back}>
      {error ? <Notice tone="error" body={error} /> : null}
      <TextField label="Invitation code" value={code} onChangeText={t => setCode(t.toUpperCase())} autoCapitalize="characters" placeholder="K7QD2MXF" />
      {inv.loading && code.length >= 8 ? <Loading label="Looking up the invitation" /> : null}
      {inv.data?.state === 'valid' ? (
        <>
          <Notice tone="info" title={inv.data.server} body={`${inv.data.libraries} libraries shared with you`} icon="server" />
          {withPassword ? (
            <>
              <TextField label="Username" help="You sign in with it. 3 to 64 characters, no spaces or @." value={user} onChangeText={setUser} autoCapitalize="none" />
              <TextField label="Password" secureTextEntry value={pw} onChangeText={setPw} />
              <Button label="Join and sign in" variant="primary" grow loading={busy} disabled={user.length < 3 || pw.length < 8} onPress={() => void run(async () => { await client.session.acceptInvitation(code, {username: user, password: pw}); setDone('joined'); })} />
            </>
          ) : (
            <>
              <Button label="Join with my Portico Account" icon="user" variant="primary" size="lg" grow loading={busy} onPress={() => void run(async () => { await client.session.acceptInvitation(code, {account: true}); setDone('joined'); })} hasTVPreferredFocus />
              <Button label="Use a username and password" variant="ghost" grow onPress={() => setWithPassword(true)} />
            </>
          )}
          <Button label="Decline" variant="ghost" grow onPress={() => void run(async () => { await client.session.declineInvitation(code); setDone('declined'); })} />
        </>
      ) : inv.data?.state === 'expired' ? <Notice tone="error" body="This invitation has expired or was already used. Ask the owner for a new one." /> : null}
    </AuthFrame>
  );
}

/** TV activation: a code and a QR code; the phone approves it under Settings › Link a TV. */
export function ActivateScreen() {
  const client = useClient();
  const {enter} = useSession();
  const m = useMetrics();
  const act = useQuery(c => c.session.activationCode(), []);
  const [state, setState] = useState<'waiting' | 'approved'>('waiting');
  const [left, setLeft] = useState(600);
  useEffect(() => { const t = setInterval(() => setLeft(l => (l <= 1 ? 600 : l - 1)), 1000); return () => clearInterval(t); }, []);
  const approve = async () => { setState('approved'); await client.session.chooseServer('srv-lighthouse'); setTimeout(() => void enter('pr-justin'), 1400); };
  // The sample household approves the code from Justin's phone after a short wait.
  useEffect(() => {
    if (!act.data || state !== 'waiting') return;
    const t = setTimeout(() => void approve(), 15000);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [act.data]);
  return (
    <AuthFrame title="Activate Portico on this TV" lede="Scan the QR code, or visit getportico.tv/link and enter the code. Sign in with Google, Apple, or your Portico Account in the browser." wide onBack={useBack()}>
      {act.data ? (
        <View style={{flexDirection: m.isPhone ? 'column' : 'row', gap: m.space.xl, alignItems: 'center'}}>
          <View style={{padding: 16, backgroundColor: '#fff', borderRadius: 18}}><QRCode value={`https://getportico.tv/link?code=${act.data.code}`} size={m.isTV ? 280 : 180} /></View>
          <View style={{flex: m.isPhone ? undefined : 1, gap: m.space.md}}>
            <Text variant="label" tone="tertiary">Your code</Text>
            <Text variant="display" style={{letterSpacing: m.isTV ? 12 : 6}}>{act.data.code}</Text>
            <Text variant="body" tone="secondary">getportico.tv/link</Text>
            <View style={{flexDirection: 'row', alignItems: 'center', gap: isTV ? 20 : 10}}>
              {state === 'waiting' ? <ActivityIndicator color={colors.accentStrong} /> : null}
              {state === 'approved' ? <Icon name="check" size={m.isTV ? 28 : 20} color={colors.healthy} /> : null}
              <Text variant="bodyStrong">{state === 'waiting' ? 'Waiting for approval' : 'Approved on Justin’s iPhone. Signing in…'}</Text>
            </View>
            <Text variant="caption" tone="tertiary">This code refreshes by itself in {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}. Leave this screen open while you sign in.</Text>
            <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap'}}>
              <Button label="Get a new code" icon="refresh" variant="secondary" onPress={() => { setLeft(600); act.refresh(); }} hasTVPreferredFocus disabled={state === 'approved'} />
            </View>
          </View>
        </View>
      ) : <Loading label="Preparing your code" />}
    </AuthFrame>
  );
}

export function QuickConnectScreen() {
  const client = useClient();
  const nav = useNav();
  const [approved, setApproved] = useState(false);
  // The sample server's owner approves the code after a short wait.
  useEffect(() => {
    const t = setTimeout(() => { setApproved(true); void client.session.chooseServer('srv-lighthouse').then(() => setTimeout(() => nav.navigate('Profiles'), 1200)); }, 15000);
    return () => clearTimeout(t);
  }, [client, nav]);
  return (
    <AuthFrame title="Sign in to Lighthouse" lede="On a phone signed in to Lighthouse, open Portico › Settings › Link a TV, or on a computer open lighthouse.local:32500/device." onBack={useBack()}>
      <View style={{alignItems: 'center', gap: 12, paddingVertical: 12}}>
        <Text variant="display" style={{letterSpacing: 10}}>482 913</Text>
        <View style={{flexDirection: 'row', gap: isTV ? 20 : 10, alignItems: 'center'}}>{approved ? <Icon name="check" size={20} color={colors.healthy} /> : <ActivityIndicator color={colors.accentStrong} />}<Text variant="body" tone="secondary">{approved ? 'Approved. Signing in…' : 'Waiting for approval'}</Text></View>
      </View>
      <Button label="Use a password instead" variant="secondary" grow onPress={() => nav.navigate('DirectSignIn')} hasTVPreferredFocus />
    </AuthFrame>
  );
}

export function ClaimScreen() {
  const m = useMetrics();
  const client = useClient();
  const nav = useNav();
  const q = useQuery(c => c.session.claimRequest(), []);
  const [done, setDone] = useState<'approved' | 'denied'>();
  const {busy, run} = useAction();
  return (
    <AuthFrame title="Connect Studio Annex" lede="Connecting lets people sign in with their Portico Account and reach this server away from home." onBack={useBack()}>
      {done ? (
        <>
          <Notice tone={done === 'approved' ? 'success' : 'info'} title={done === 'approved' ? 'Connected' : 'Declined'} body={done === 'approved' ? 'Studio Annex is connected to your Portico Account. It finishes setting up on its own.' : 'You declined. Studio Annex isn’t connected. You can close this window.'} />
          <Button label="Back to Portico" variant="primary" grow onPress={() => nav.navigate('Landing')} hasTVPreferredFocus />
        </>
      ) : q.data ? (
        <>
          {[['Server', q.data.server], ['Address', q.data.address], ['Portico Account', 'justin (Justin Ehler)'], ['Requested', q.data.requested]].map(([k, v]) => <View key={k} style={{flexDirection: 'row', gap: 12}}><Text variant="caption" tone="tertiary" style={{width: m.isTV ? 240 : 132}}>{k}</Text><Text variant="captionStrong" style={{flex: 1}}>{v}</Text></View>)}
          <ListRow icon="user" title="Owner profile" subtitle="The profile that owns the server and can manage it." value="Justin" />
          <Notice tone="warn" body="Approve only if you started this from your own server and the address above is yours." />
          <View style={{flexDirection: 'row', gap: 8}}>
            <Button label="Approve" variant="primary" loading={busy} onPress={() => void run(async () => { await client.session.answerClaim(true); setDone('approved'); })} hasTVPreferredFocus />
            <Button label="Deny" variant="ghost" onPress={() => void run(async () => { await client.session.answerClaim(false); setDone('denied'); })} />
          </View>
        </>
      ) : <Loading label="Looking up this server" />}
    </AuthFrame>
  );
}

export function SetupScreen() {
  const client = useClient();
  const nav = useNav();
  const m = useMetrics();
  const [step, setStep] = useState(0);
  const [name, setName] = useState('Lighthouse');
  const [mode, setMode] = useState<'hosted' | 'local'>('hosted');
  const [owner, setOwner] = useState('justin');
  const [pw, setPw] = useState('');
  const [saved, setSaved] = useState(false);
  const {busy, run} = useAction();
  const goBack = useBack();
  return (
    <AuthFrame title={step === 0 ? 'Welcome to Portico' : step === 1 ? 'How will people sign in?' : mode === 'hosted' ? 'Create a recovery owner' : 'Create the owner account'} lede={step === 0 ? 'Let’s set up your server. It takes about a minute.' : step === 2 ? (mode === 'hosted' ? 'This account lives on the server and gets you back in if your Portico Account is ever unavailable. Keep its password somewhere safe.' : 'You will use this to sign in and to manage the server.') : undefined} onBack={step ? () => setStep(s => s - 1) : goBack}>
      <View style={{flexDirection: 'row', gap: 6}}>{[0, 1, 2].map(i => <View key={i} style={{flex: 1, height: 4, borderRadius: 2, backgroundColor: i <= step ? colors.accent : colors.track}} />)}</View>
      {step === 0 ? (
        <>
          <TextField label="Server name" help="Shown on every device that connects." value={name} onChangeText={setName} />
          <Button label="Continue" variant="primary" grow onPress={() => setStep(1)} hasTVPreferredFocus />
        </>
      ) : step === 1 ? (
        <>
          <ListRow icon="user" title="Portico Accounts" subtitle="One sign-in for all your apps, easy invitations and access away from home." selected={mode === 'hosted'} onPress={() => setMode('hosted')} chevron={false} trailing={<Badge label="Recommended" tone="accent" />} />
          <ListRow icon="server" title="Keep accounts on this server" subtitle="Works without the internet. You’ll manage every sign-in yourself." selected={mode === 'local'} onPress={() => setMode('local')} chevron={false} />
          <Button label="Continue" variant="primary" grow onPress={() => setStep(2)} />
        </>
      ) : (
        <>
          <TextField label="Username" value={owner} onChangeText={setOwner} autoCapitalize="none" />
          <TextField label="Password" secureTextEntry value={pw} onChangeText={setPw} />
          <PasswordStrength value={pw} />
          <Toggle label="I saved this password somewhere safe" value={saved} onChange={setSaved} />
          {mode === 'hosted' ? <Text variant="caption" tone="tertiary">Next, you’ll connect the server to your Portico Account.</Text> : null}
          <Button label={mode === 'hosted' ? 'Create recovery owner' : 'Create owner'} variant="primary" grow disabled={pw.length < 8 || !saved} loading={busy} onPress={() => void run(async () => { await client.session.setupServer({name, mode, owner, password: pw}); nav.navigate(mode === 'hosted' ? 'Claim' : 'Profiles'); })} />
        </>
      )}
      {m.isTV ? null : null}
    </AuthFrame>
  );
}

export function RestoreScreen() {
  const client = useClient();
  const {enter} = useSession();
  const [state, setState] = useState<'checking' | 'slow'>('checking');
  useEffect(() => {
    const t = setTimeout(() => setState('slow'), 1800);
    void client.session.restore().then(async () => { await client.session.chooseServer('srv-lighthouse'); setTimeout(() => void enter('pr-justin'), 2600); });
    return () => clearTimeout(t);
  }, [client, enter]);
  return (
    <AuthFrame title="Opening Portico" lede="Checking your saved connection.">
      <View style={{flexDirection: 'row', gap: isTV ? 20 : 12, alignItems: 'center'}}><ActivityIndicator color={colors.accentStrong} /><Text variant="body">Connecting to lighthouse.local:32500…</Text></View>
      {state === 'slow' ? <Text variant="caption" tone="tertiary">This is taking longer than usual. Portico will keep trying.</Text> : null}
    </AuthFrame>
  );
}

export function VerifyEmailScreen() {
  const route = useRoute() as {params?: {token?: string}};
  const nav = useNav();
  const q = useQuery(c => c.session.verifyEmail(route.params?.token ?? 'ok'), []);
  return (
    <AuthFrame title={q.data === 'confirmed' ? 'Email confirmed' : q.data === 'expired' ? 'Confirm your email' : 'Confirming your email'}>
      {!q.data ? <Loading /> : q.data === 'confirmed' ? <Notice tone="success" body="You can go back to the device where you started, or sign in here." /> : <Notice tone="error" body="This link has expired or was already used. Ask for a new code from Portico." />}
      {q.data ? <Button label="Sign in" variant="primary" grow onPress={() => nav.navigate('AccountSignIn')} hasTVPreferredFocus /> : null}
    </AuthFrame>
  );
}

export function ResetPasswordScreen() {
  const client = useClient();
  const nav = useNav();
  const [pw, setPw] = useState('');
  const [pw2, setPw2] = useState('');
  const [done, setDone] = useState(false);
  const {busy, run, error} = useAction();
  return (
    <AuthFrame title={done ? 'Password changed' : 'Choose a new password'} lede={done ? 'Your password is changed. Sign in with the new one.' : 'For your Portico Account.'}>
      {error ? <Notice tone="error" body={error} /> : null}
      {done ? <Button label="Sign in" variant="primary" grow onPress={() => nav.navigate('AccountSignIn')} hasTVPreferredFocus /> : (
        <>
          <TextField label="New password" secureTextEntry value={pw} onChangeText={setPw} />
          <PasswordStrength value={pw} />
          <TextField label="Confirm new password" secureTextEntry value={pw2} onChangeText={setPw2} error={pw2 && pw2 !== pw ? 'The passwords don’t match.' : undefined} />
          <TextField label="Authenticator or recovery code" help="Only if two-step verification is on for this account." keyboardType="number-pad" />
          <Button label="Change password" variant="primary" grow loading={busy} disabled={pw.length < 8 || pw !== pw2} onPress={() => void run(async () => { await client.session.resetPassword(pw); setDone(true); })} />
        </>
      )}
    </AuthFrame>
  );
}

export function ServerUnavailableScreen() {
  const nav = useNav();
  const [seconds, setSeconds] = useState(8);
  useEffect(() => { const t = setInterval(() => setSeconds(s => (s <= 1 ? 8 : s - 1)), 1000); return () => clearInterval(t); }, []);
  return (
    <AuthFrame title="Can’t reach Studio Annex" lede="You’re still signed in. Portico is trying again automatically and will open as soon as the server answers.">
      <View style={{flexDirection: 'row', gap: isTV ? 20 : 12, alignItems: 'center'}}><ActivityIndicator color={colors.amber} /><Text variant="body">Trying again in {seconds} seconds</Text></View>
      <Text variant="caption" tone="tertiary">If the server is asleep or switched off, it may need to be woken up. Check that this device is on the same network, or that the server can be reached from outside. It was last online 3 days ago.</Text>
      <Button label="Check now" icon="refresh" variant="primary" grow onPress={() => setSeconds(8)} hasTVPreferredFocus />
      {isTV ? null : <Button label="Watch downloads" icon="download" grow onPress={() => nav.navigate('Offline')} />}
      <Button label="Choose another server" variant="ghost" grow onPress={() => nav.navigate('ServerChooser')} />
    </AuthFrame>
  );
}

export function OfflineScreen() {
  const nav = useNav();
  const m = useMetrics();
  const dl = useQuery(c => c.downloads.list(), []);
  const player = usePlayer();
  return (
    <AuthFrame title="This device is offline" lede={isTV ? 'Portico needs a network connection on Apple TV. Check the network in Settings, then try again.' : 'Watch your downloads without a connection. Sign in to download more.'} onBack={useBack()}>
      {isTV ? <Button label="Try again" icon="refresh" variant="primary" grow onPress={() => nav.navigate('Restore')} hasTVPreferredFocus /> : null}
      {isTV ? null : <Text variant="label" tone="tertiary">On this device</Text>}
      {isTV ? null : dl.data?.items.filter(d => d.state === 'ready').map(d => <ListRow key={d.id} left={<Artwork source={d.entry.image} title={d.entry.title} width={m.isTV ? 64 : 44} height={m.isTV ? 96 : 66} radius={6} />} title={d.entry.title} subtitle={`${d.quality} · ${d.size} · kept until ${d.expires}`} onPress={() => player.play([d.entry])} trailing={<Icon name="play" size={m.isTV ? 26 : 18} color={colors.accentStrong} />} chevron={false} />)}
      {isTV ? null : <Button label="Back to saved server" variant="ghost" grow onPress={() => nav.navigate('Restore')} />}
      <PlayerHost />
    </AuthFrame>
  );
}

export function DeviceApprovalScreen() {
  const nav = useNav();
  const [checking, setChecking] = useState(false);
  return (
    <AuthFrame title="Waiting for the owner to approve this device" lede="This server asks its owner to approve each new device. Once they approve it under Server › Access, you can carry on here.">
      <View style={{flexDirection: 'row', gap: isTV ? 20 : 12, alignItems: 'center'}}><ActivityIndicator color={colors.accentStrong} /><Text variant="body">Waiting</Text></View>
      <Button label="Check again" icon="refresh" variant="primary" grow loading={checking} onPress={() => { setChecking(true); setTimeout(() => { setChecking(false); nav.navigate('Profiles'); }, 1200); }} hasTVPreferredFocus />
      <Button label="Use another account or server" variant="ghost" grow onPress={() => nav.navigate('Landing')} />
    </AuthFrame>
  );
}
