import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import {useSession} from '@/app/session';
import {Button, IconButton} from '@/ui/Button';
import {Avatar, Badge, Skeleton} from '@/ui/Bits';
import {TextField} from '@/ui/Form';
import {ConfirmDialog, Dialog, Menu, useToast} from '@/ui/Overlay';
import {Group, PageHeader, Row} from '@/ui/Page';
import {LinkTabs} from '@/ui/Tabs';
import {useTitle} from '@/ui/hooks';
import {useRouteSearch} from '../common';
import {DevicesSection, SecuritySection} from '../settings/Account';
import {NotificationsSection} from '../settings/Other';
import {serverLine} from '@/shell/Switchers';
import {plural} from '@/app/format';

const TABS = [{id: 'sessions', label: 'Where you’re signed in'}, {id: 'security', label: 'Security'}, {id: 'servers', label: 'Servers'}, {id: 'notifications', label: 'Notifications'}, {id: 'data', label: 'Your data'}];

/** The Portico Account: things that belong to the account rather than to one server. */
export default function AccountScreen() {
  const session = useSession();
  const search = useRouteSearch<{tab?: string}>();
  const tab = TABS.find(t => t.id === search.tab)?.id ?? 'sessions';
  const acct = session.session!.account;
  useTitle('Portico Account');
  return (
    <div style={{padding: '28px 0 72px'}}>
      <PageHeader eyebrow="Portico Account" title={acct.displayName} subtitle={`${acct.email} · username ${acct.username}. You use this account to sign in on every device and server.`} actions={<Avatar name={acct.displayName} art="blue" size={56} />} />
      <div style={{padding: '0 var(--gutter)', marginBottom: 24}}><LinkTabs label="Account" current={tab} tabs={TABS.map(t => ({...t, href: `/account?tab=${t.id}`}))} /></div>
      <div style={{padding: '0 var(--gutter)', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 28, maxWidth: 900}}>
        {tab === 'sessions' ? <DevicesSection /> : tab === 'security' ? <><EmailGroup /><SecuritySection /><ProvidersGroup /></> : tab === 'servers' ? <ServersAndInvites /> : tab === 'notifications' ? <NotificationsSection /> : <DataTab />}
      </div>
    </div>
  );
}

function EmailGroup() {
  const session = useSession();
  const client = useClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  return (
    <>
      <Group title="Email">
        <Row icon="mail" label={session.session!.account.email} help="Used for signing in, password resets and account notices." trailing={<Button size="sm" variant="secondary" label="Change…" onClick={() => setOpen(true)} />} />
      </Group>
      <Dialog open={open} onOpenChange={setOpen} title="Change email" description="We’ll send a link to the new address. The change happens when you open it." width={440} footer={<><Button variant="ghost" label="Cancel" onClick={() => setOpen(false)} /><Button variant="primary" label="Send link" disabled={!/.+@.+\..+/.test(email)} onClick={async () => { await client.account.changeEmail(email); setOpen(false); toast.show(`Check ${email} for a confirmation link`, {icon: 'mail'}); }} /></>}>
        <TextField type="email" label="New email" value={email} onChange={setEmail} autoFocus autoComplete="email" />
      </Dialog>
    </>
  );
}

function ProvidersGroup() {
  const session = useSession();
  const client = useClient();
  const toast = useToast();
  const acct = session.session!.account;
  const [providers, setProviders] = useState(acct.providers);
  const toggle = async (p: 'apple' | 'google') => {
    const on = !providers.includes(p);
    await client.account.connectProvider(p, on);
    setProviders(on ? [...providers, p] : providers.filter(x => x !== p));
    toast.show(on ? `You can sign in with ${p === 'apple' ? 'Apple' : 'Google'} now` : `Disconnected ${p === 'apple' ? 'Apple' : 'Google'}`, {icon: 'link'});
  };
  return (
    <Group title="Sign in with Apple or Google" lede="Use your Apple or Google account instead of a password. Your password keeps working too.">
      <Row icon="link" label="Apple" help={providers.includes('apple') ? 'Connected' : 'Not connected'} trailing={<Button size="sm" variant={providers.includes('apple') ? 'ghost' : 'secondary'} label={providers.includes('apple') ? 'Disconnect' : 'Connect'} onClick={() => void toggle('apple')} />} />
      <Row icon="link" label="Google" help={providers.includes('google') ? 'Connected' : 'Not connected'} trailing={<Button size="sm" variant={providers.includes('google') ? 'ghost' : 'secondary'} label={providers.includes('google') ? 'Disconnect' : 'Connect'} onClick={() => void toggle('google')} />} />
    </Group>
  );
}

function ServersAndInvites() {
  const client = useClient();
  const toast = useToast();
  const navigate = useNavigate();
  const servers = useQuery(c => c.session.servers(), []);
  const invites = useQuery(c => c.account.invitations(), [], ['account']);
  const [code, setCode] = useState('');
  const [leave, setLeave] = useState<string>();
  return (
    <>
      <Group title="Servers" lede="Servers you own and servers shared with you.">
        {!servers.data ? <div style={{padding: 16}}><Skeleton height={120} /></div> : servers.data.map(sv => (
          <Row key={sv.id} icon="server" label={<>{sv.name} {sv.status === 'unreachable' ? <Badge tone="warn" dot>Not reachable</Badge> : null}</>} help={serverLine(sv)} trailing={<Menu label={`${sv.name} options`} items={[
            {id: 'open', label: 'Switch to this server', icon: 'switch', disabled: sv.status !== 'online', onSelect: () => void navigate({to: '/settings/servers' as never})},
            ...(sv.yours ? [] : [{id: 'leave', label: 'Leave…', icon: 'signOut' as const, danger: true, onSelect: () => setLeave(sv.name)}]),
          ]} trigger={<IconButton variant="ghost" icon="more" label={`${sv.name} options`} />} />} />
        ))}
      </Group>
      <Group title="Invitations">
        {invites.data?.map(inv => (
          <Row key={inv.id} icon="mail" label={`${inv.server}`} help={`From ${inv.from} · ${plural(inv.libraries, 'library', 'libraries')} · ${inv.state === 'waiting' ? `Expires ${inv.expires}` : inv.state === 'accepted' ? 'Accepted' : inv.state === 'declined' ? 'Declined' : 'Expired'}`}
            trailing={inv.state === 'waiting' ? <><Button size="sm" variant="primary" label="Accept" onClick={async () => { await client.account.answerInvitation(inv.id, true); toast.show(`You joined ${inv.server}`, {icon: 'check'}); }} /><Button size="sm" variant="ghost" label="Decline" onClick={async () => { await client.account.answerInvitation(inv.id, false); toast.show('Invitation declined', {icon: 'check'}); }} /></> : <Badge tone={inv.state === 'accepted' ? 'good' : 'neutral'}>{inv.state === 'accepted' ? 'Accepted' : inv.state === 'declined' ? 'Declined' : 'Expired'}</Badge>} />
        ))}
        <form style={{display: 'flex', gap: 8, padding: 16, alignItems: 'flex-end', borderTop: '1px solid var(--line-soft)'}} onSubmit={e => { e.preventDefault(); void navigate({to: '/join' as never, search: {code} as never}); }}>
          <TextField label="Have an invitation code or link?" placeholder="Paste it here" value={code} onChange={setCode} className="grow" />
          <Button type="submit" variant="secondary" label="Continue" disabled={!code.trim()} />
        </form>
      </Group>
      {leave ? <ConfirmDialog open onOpenChange={o => !o && setLeave(undefined)} title={`Leave ${leave}?`} body="You lose access to its libraries. The owner can invite you again." action="Leave" tone="danger" onConfirm={async () => { await client.account.leaveServer(leave); setLeave(undefined); toast.show(`You left ${leave}`, {icon: 'check'}); }} /> : null}
    </>
  );
}

function DataTab() {
  const client = useClient();
  const toast = useToast();
  const session = useSession();
  const navigate = useNavigate();
  const [exporting, setExporting] = useState(false);
  const [del, setDel] = useState(false);
  const [typed, setTyped] = useState('');
  const [error, setError] = useState<string>();
  return (
    <>
      <Group title="Your data">
        <Row icon="download" label="Export your account data" help="A copy of your account, profiles, watch history, ratings and lists, as plain files. It’s ready within a few minutes." trailing={<Button size="sm" variant="secondary" label="Prepare export" busy={exporting} onClick={async () => { setExporting(true); await client.account.exportData(); setExporting(false); toast.show('Your export is ready. We’ve sent a link to your email.', {icon: 'download'}); }} />} />
      </Group>
      <Group title="Delete account">
        <Row icon="trash" label="Delete your Portico Account" help="Your profiles, history and lists are deleted from Portico. Servers you own keep running, and their media isn’t touched." trailing={<Button size="sm" variant="danger" label="Delete account…" onClick={() => setDel(true)} />} />
      </Group>
      <Dialog open={del} onOpenChange={setDel} title="Delete your Portico Account?" description="This can’t be undone. Type your account email to confirm." width={480} footer={<><Button variant="ghost" label="Cancel" onClick={() => setDel(false)} /><Button variant="dangerSolid" label="Delete account" disabled={!typed} onClick={async () => { try { await client.account.deleteAccount(typed); setDel(false); await session.signOut(); void navigate({to: '/welcome' as never}); } catch (e) { setError(e instanceof Error ? e.message : 'That didn’t work.'); } }} /></>}>
        <TextField label="Account email" value={typed} onChange={v => { setTyped(v); setError(undefined); }} error={error} placeholder={session.session!.account.email} autoFocus />
      </Dialog>
    </>
  );
}
