import {useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import {useSession} from '../../app/session';
import {Page, PageHeader, Panel, Row, Tabs} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Field, Input, Switch} from '../../ui/Controls';
import {Badge, Notice, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Avatar} from '../../ui/Media';
import {ConfirmDialog, Dialog, DialogClose} from '../../ui/Overlay';
import {DevicesSection, PasswordDialog, TwoStepDialog} from '../settings/Profiles';
import {ProfilesSection} from '../settings/Profiles';
import {relativeTime} from '../shared/items';

const TABS = [{value: 'overview', label: 'Overview'}, {value: 'security', label: 'Security'}, {value: 'sessions', label: 'Signed in'}, {value: 'servers', label: 'Servers'}, {value: 'invitations', label: 'Invitations'}, {value: 'notifications', label: 'Email and data'}] as const;

/** The Portico Account: who you are, security, where you're signed in, servers and invitations. */
export function AccountScreen() {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/account'});
  const tab = (TABS.some(t => t.value === search.tab) ? search.tab : 'overview') as (typeof TABS)[number]['value'];
  const account = useQuery(`account.${version}`, () => services.account.account());
  const invites = useQuery(`invitations.${version}`, () => services.account.invitations());
  const waiting = (invites.data ?? []).filter(i => i.state === 'waiting').length;
  const a = account.data;
  return (
    <Page title="Portico Account" className="settings-page">
      <PageHeader eyebrow="Portico Account" title={a?.name ?? 'Account'} lede={a ? `${a.username} · ${a.email}` : undefined} actions={a ? <Avatar name={a.name} color="#409fd4" size={56} ring /> : null} />
      <Tabs label="Account" value={tab} onChange={v => void navigate({to: '/account', search: {tab: v === 'overview' ? undefined : v}, replace: true})} tabs={TABS.map(t => ({...t, count: t.value === 'invitations' && waiting ? waiting : undefined}))} />
      <div className="settings-col">
        {!a ? <Skeleton h={200} /> : tab === 'overview' ? <Overview /> : tab === 'security' ? <Security /> : tab === 'sessions' ? <DevicesSection /> : tab === 'servers' ? <AccountServers /> : tab === 'invitations' ? <Invitations /> : <EmailAndData />}
      </div>
    </Page>
  );
}

function Overview() {
  const services = useServices();
  const navigate = useNavigate();
  const a = useQuery('account.overview', () => services.account.account()).data;
  const invites = useQuery('invitations', () => services.account.invitations()).data ?? [];
  const waiting = invites.filter(i => i.state === 'waiting');
  return (
    <div className="stack">
      {waiting.length ? <Notice tone="info" title={`${waiting[0]!.inviter} invited you to ${waiting[0]!.server}`} action={<Button size="sm" variant="primary" label="Review" onClick={() => void navigate({to: '/account', search: {tab: 'invitations'}})} />}>{waiting[0]!.libraries} libraries shared with you.</Notice> : null}
      <Panel title="Profiles on your servers" subtitle="Profiles belong to each server; these are on the one you’re using." padded>
        <ProfilesSection />
      </Panel>
      <Panel padded={false}>
        <Row icon="profile" label="Display name" help={a?.name} />
        <Row icon="mail" label="Email" help={`${a?.email} · Verified`} />
        <Row icon="key" label="Username" help={a?.username} />
      </Panel>
    </div>
  );
}

function Security() {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const session = useSession();
  const a = useQuery(`account.sec.${version}`, () => services.account.account()).data;
  const [pw, setPw] = useState(false);
  const [email, setEmail] = useState(false);
  const [two, setTwo] = useState(false);
  const [del, setDel] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  if (!a) return <Skeleton h={300} />;
  return (
    <div className="stack">
      <Panel title="Password and email" padded={false}>
        <Row icon="key" label="Password" help={a.hasPassword ? 'Last changed 4 months ago' : 'You sign in with Apple. Add a password to sign in without it.'}><Button size="sm" variant="secondary" label={a.hasPassword ? 'Change…' : 'Add password…'} onClick={() => setPw(true)} /></Row>
        <Row icon="mail" label="Email" help={a.email}><Button size="sm" variant="secondary" label="Change…" onClick={() => setEmail(true)} /></Row>
      </Panel>
      <Panel title="Sign-in methods" subtitle="Sign in to Portico with another account you already have." padded={false}>
        <Row icon="account" label="Sign in with Apple" help={a.providers.apple ? 'Connected' : 'Not connected'}><Button size="sm" variant={a.providers.apple ? 'ghost' : 'secondary'} label={a.providers.apple ? 'Disconnect' : 'Connect'} onClick={() => void services.account.updateAccount({providers: {...a.providers, apple: !a.providers.apple}}).then(() => toast(a.providers.apple ? 'Apple disconnected' : 'Apple connected'))} /></Row>
        <Row icon="globe" label="Sign in with Google" help={a.providers.google ? 'Connected' : 'Not connected'}><Button size="sm" variant={a.providers.google ? 'ghost' : 'secondary'} label={a.providers.google ? 'Disconnect' : 'Connect'} onClick={() => void services.account.updateAccount({providers: {...a.providers, google: !a.providers.google}}).then(() => toast(a.providers.google ? 'Google disconnected' : 'Google connected'))} /></Row>
      </Panel>
      <Panel title="Two-step verification" padded={false}>
        <Row icon="shieldCheck" label={a.twoStep ? 'On' : 'Off'} help={a.twoStep ? `Codes from your authenticator app · ${a.recoveryCodesLeft} recovery codes left` : 'Ask for a code from an authenticator app when you sign in.'}>
          {a.twoStep ? <><Button size="sm" variant="ghost" label="New recovery codes" onClick={() => void services.account.updateAccount({recoveryCodesLeft: 10}).then(() => toast('10 new recovery codes made; the old ones stopped working'))} /><Button size="sm" variant="secondary" label="Turn off" onClick={() => void services.account.updateAccount({twoStep: false}).then(() => toast('Two-step verification is off'))} /></> : <Button size="sm" variant="primary" label="Turn on…" onClick={() => setTwo(true)} />}
        </Row>
      </Panel>
      <Panel title="Delete your Portico Account" subtitle="Your servers keep running and their owners keep their media. You lose access to servers you joined, and sign-in through Portico stops.">
        <Button variant="danger" icon="trash" label="Delete account…" onClick={() => setDel(true)} />
      </Panel>
      {pw ? <PasswordDialog onClose={() => setPw(false)} /> : null}
      {two ? <TwoStepDialog onClose={() => setTwo(false)} /> : null}
      <Dialog open={email} onOpenChange={setEmail} title="Change email" description="We’ll send a link to the new address; the change happens when you open it." size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Send link" disabled={!/.+@.+\..+/.test(newEmail)} onClick={() => { toast(`Check ${newEmail} for a link to confirm the change`, {icon: 'mail'}); setEmail(false); }} /></>}>
        <Field label="New email"><Input type="email" value={newEmail} onChange={e => setNewEmail(e.target.value)} autoFocus /></Field>
      </Dialog>
      <ConfirmDialog open={del} onOpenChange={setDel} danger title="Delete your Portico Account?" body={<><p>This can’t be undone. You’ll be signed out everywhere, and servers you own will need to be connected to another account to use Portico sign-in.</p><p>Servers you own: Ehler Home Theater.</p></>} typeToConfirm={a.username} confirmLabel="Delete account" onConfirm={() => { toast('Account deleted'); session.signOut(); void navigate({to: '/welcome/$screen', params: {screen: 'signin'}}); }} />
    </div>
  );
}

function AccountServers() {
  const services = useServices();
  const version = useDataVersion();
  const session = useSession();
  const servers = useQuery(`account.servers.${version}`, () => services.account.servers());
  const [leave, setLeave] = useState<string | null>(null);
  return (
    <Panel title="Servers on your account" padded={false}>
      {!servers.data ? <Row label={<Skeleton w={200} h={16} />} /> : servers.data.map(s => (
        <Row key={s.id} icon="server" label={<>{s.name}{s.id === session.serverId ? <Badge tone="accent" className="ml">Current</Badge> : null}</>} help={`${s.role === 'owner' ? 'You own it' : s.role === 'admin' ? 'Admin' : 'Member'} · ${s.status === 'online' ? 'Online' : `Can’t reach it · last seen ${relativeTime(s.lastSeen).toLowerCase()}`} · ${s.libraries} libraries`}>
          {s.role === 'owner' ? <Button size="sm" variant="ghost" label="Server console" to="/server/$section" params={{section: 'general'}} /> : <Button size="sm" variant="ghost" label="Leave…" onClick={() => setLeave(s.id)} />}
        </Row>
      ))}
      <ConfirmDialog open={!!leave} onOpenChange={o => !o && setLeave(null)} danger title={`Leave ${servers.data?.find(s => s.id === leave)?.name}?`} body="Your profiles and history there are deleted. The owner can invite you again." confirmLabel="Leave server" onConfirm={async () => { if (leave) await services.account.leaveServer(leave); toast('You left the server'); }} />
    </Panel>
  );
}

function Invitations() {
  const services = useServices();
  const version = useDataVersion();
  const invites = useQuery(`invitations.${version}`, () => services.account.invitations());
  const [link, setLink] = useState('');
  const waiting = (invites.data ?? []).filter(i => i.state === 'waiting');
  const earlier = (invites.data ?? []).filter(i => i.state !== 'waiting');
  return (
    <div className="stack">
      {!invites.data ? <Skeleton h={120} /> : waiting.length ? waiting.map(i => (
        <Panel key={i.id} className="invite-card">
          <div className="invite-row">
            <Avatar name={i.inviter} color="#e8bc6a" size={52} />
            <div className="set-text"><strong>{i.inviter} invited you to {i.server}</strong><span>{i.libraries} libraries · expires {new Date(i.expires).toLocaleDateString('en-US', {month: 'long', day: 'numeric'})}</span></div>
            <div className="hstack">
              <Button variant="ghost" label="Decline" onClick={() => void services.account.answerInvitation(i.id, false).then(() => toast('Invitation declined'))} />
              <Button variant="primary" label="Accept" onClick={() => void services.account.answerInvitation(i.id, true).then(() => toast(`You joined ${i.server}. Switch servers from the profile menu.`))} />
            </div>
          </div>
        </Panel>
      )) : <StateView compact icon="mail" title="No invitations waiting" body="When someone shares their server with you, it shows up here." />}
      <Panel title="Have an invitation link?" subtitle="Paste it here if the email didn’t open Portico.">
        <form className="hstack" onSubmit={e => { e.preventDefault(); toast(link.includes('join') ? 'Invitation found: Sam’s Film Club' : 'That isn’t an invitation link. It starts with getportico.tv/join.', {tone: link.includes('join') ? 'good' : 'bad'}); }}>
          <Input value={link} onChange={e => setLink(e.target.value)} placeholder="https://getportico.tv/join#…" aria-label="Invitation link" style={{flex: 1, minWidth: 220}} />
          <Button variant="secondary" label="Open" type="submit" disabled={!link.trim()} />
        </form>
      </Panel>
      {earlier.length ? (
        <Panel title="Earlier" padded={false}>
          {earlier.map(i => <Row key={i.id} icon="mail" label={`${i.server} · from ${i.inviter}`} help={i.state === 'accepted' ? 'Accepted' : i.state === 'declined' ? 'Declined' : i.state === 'expired' ? 'Expired' : 'Withdrawn'} />)}
        </Panel>
      ) : null}
    </div>
  );
}

function EmailAndData() {
  const [prefs, setPrefs] = useState({security: true, invites: true, product: false, digest: true});
  const [preparing, setPreparing] = useState(false);
  return (
    <div className="stack">
      <Panel title="Email me about" padded={false}>
        <Row label="Security alerts" help="New sign-ins and changes to your account. Always on."><Switch checked disabled onChange={() => undefined} label="Security alerts" /></Row>
        <Row label="Invitations" help="When someone shares a server with you."><Switch checked={prefs.invites} onChange={v => setPrefs({...prefs, invites: v})} label="Invitations" /></Row>
        <Row label="Weekly digest" help="What’s new on your servers."><Switch checked={prefs.digest} onChange={v => setPrefs({...prefs, digest: v})} label="Weekly digest" /></Row>
        <Row label="Portico news" help="New features, rarely."><Switch checked={prefs.product} onChange={v => setPrefs({...prefs, product: v})} label="Portico news" /></Row>
      </Panel>
      <Panel title="Download your data" subtitle="Everything your Portico Account holds, as plain files you can open: account details, servers, sign-ins and settings.">
        <Button variant="secondary" icon="download" label={preparing ? 'Preparing…' : 'Download your data'} busy={preparing} onClick={() => { setPreparing(true); setTimeout(() => { setPreparing(false); toast('portico-account-export.zip is ready', {icon: 'download'}); }, 1400); }} />
      </Panel>
    </div>
  );
}
