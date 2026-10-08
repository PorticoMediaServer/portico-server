import {useState} from 'react';
import {useServices} from '../../data/hooks';
import type {AdminAccount, AdminInvite, ApiKey, Backup, Schedule} from '../../data/admin';
import {useSession} from '../../app/session';
import {Panel, Row, KeyValue} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Checkbox, ChoiceCards, Field, Input, Segmented, Select, Stepper, Switch, TextArea} from '../../ui/Controls';
import {Badge, Notice, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Avatar} from '../../ui/Media';
import {ConfirmDialog, Dialog, DialogClose} from '../../ui/Overlay';
import {relativeTime, untilTime} from '../shared/items';
import {AdminSkeleton, SectionHead, StatusBadge, UnsavedBar, useAdmin, useAdminForm} from './common';
import type {Sec} from './Server';

export function SystemSection({id, sec}: {id: string; sec: Sec}) {
  switch (id) {
    case 'network': return <Network sec={sec} />;
    case 'people': return <People sec={sec} />;
    case 'access': return <Access sec={sec} />;
    case 'schedules': return <Schedules sec={sec} />;
    case 'maintenance': return <Maintenance sec={sec} />;
    case 'backups': return <Backups sec={sec} />;
    case 'general': return <General sec={sec} />;
    default: return <Transcoding sec={sec} />;
  }
}

function Transcoding({sec}: {sec: Sec}) {
  const form = useAdminForm('transcoding');
  const t = form.data;
  if (!t) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <Panel padded={false}>
        <Row label="Allow conversion" help="When off, devices that can’t play a file get an error instead."><Switch checked={t.allow} onChange={v => form.set({allow: v})} label="Allow conversion" /></Row>
        <Row label="At once, on this server" help="More at once needs a faster processor or graphics hardware."><Stepper label="Server limit" value={t.serverCap} onChange={v => form.set({serverCap: v})} min={1} max={16} /></Row>
        <Row label="At once, per account"><Stepper label="Per account" value={t.accountCap} onChange={v => form.set({accountCap: v})} min={1} max={8} /></Row>
        <Row label="Graphics hardware" help="Found: Intel Quick Sync (VAAPI)."><Select label="Graphics hardware" value={t.hardware} onChange={v => form.set({hardware: v})} options={[{value: 'auto', label: 'Automatic'}, {value: 'vaapi', label: 'VAAPI (Intel, AMD)'}, {value: 'nvenc', label: 'NVIDIA NVENC'}, {value: 'videotoolbox', label: 'VideoToolbox (Mac)'}, {value: 'off', label: 'Processor only'}]} /></Row>
        <Row label="Convert HDR for SDR screens" help="Keeps colors right on screens without HDR."><Switch checked={t.tonemap} onChange={v => form.set({tonemap: v})} label="Tone mapping" /></Row>
        <Row label="Isolate the video tools" help="Unavailable on this host; see Diagnostics."><Switch checked={t.sandbox} disabled onChange={() => undefined} label="Isolation" /></Row>
      </Panel>
      <Panel title="Per-account limits" padded={false} actions={<Button size="sm" variant="ghost" icon="plus" label="Add" onClick={() => form.set({overrides: [...t.overrides, {account: 'Grandma Ruth', cap: 1}]})} />}>
        {t.overrides.map((o, i) => <Row key={o.account + i} icon="profile" label={o.account}><Stepper label={`${o.account} limit`} value={o.cap} onChange={v => form.set({overrides: t.overrides.map((x, j) => (j === i ? {...x, cap: v} : x))})} min={0} max={8} /><Button size="sm" variant="ghost" icon="close" aria-label={`Remove ${o.account}`} onClick={() => form.set({overrides: t.overrides.filter((_, j) => j !== i)})} /></Row>)}
        {!t.overrides.length ? <Row label="Everyone uses the per-account limit" /> : null}
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

function Network({sec}: {sec: Sec}) {
  const form = useAdminForm('network');
  const [adv, setAdv] = useState(false);
  const n = form.data;
  if (!n) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Button variant="secondary" icon="refresh" label="Test from outside" onClick={() => toast('Reachable from outside at portico.ehler.net:32500', {icon: 'success'})} />} />
      <div className="stat-row">
        <div className="stat is-good"><span className="stat-label"><Icon name="globe" size={15} />From outside</span><strong className="stat-value">Reachable</strong><span className="stat-sub">{n.publicAddress}:{n.publicPort}</span></div>
        <div className="stat"><span className="stat-label"><Icon name="home" size={15} />At home</span><strong className="stat-value">Discoverable</strong><span className="stat-sub">{n.localAddresses[0]} · {n.localAddresses.length} addresses</span></div>
        <div className="stat is-good"><span className="stat-label"><Icon name="lock" size={15} />Certificate</span><strong className="stat-value">Trusted</strong><span className="stat-sub">{n.certificate.domain} · renews in {Math.round((n.certificate.expires - Date.now()) / 86400000)} days</span></div>
        <div className="stat"><span className="stat-label"><Icon name="network" size={15} />Router</span><strong className="stat-value">{n.mapping.state === 'open' ? 'Port open' : 'Refused'}</strong><span className="stat-sub">{n.mapping.protocol}</span></div>
      </div>
      <Panel title="Remote access" padded={false}>
        <Row label="Allow access from outside your home"><Switch checked={n.remote} onChange={v => form.set({remote: v})} label="Remote access" /></Row>
        <Row label="Who can connect from outside"><Select label="Who" value={n.outside} onChange={v => form.set({outside: v})} options={[{value: 'everyone', label: 'Everyone with access'}, {value: 'accountsOnly', label: 'Portico Accounts only'}, {value: 'nobody', label: 'Nobody (home only)'}]} /></Row>
        <Row label="Open a port on the router automatically" help={`${n.mapping.protocol} · ${n.mapping.state === 'open' ? 'the router agreed' : 'the router refused'}`}><Switch checked={n.mapping.enabled} onChange={v => form.set({mapping: {...n.mapping, enabled: v}})} label="Automatic port" /></Row>
        <Row label="Public port"><Input value={String(n.publicPort)} onChange={e => form.set({publicPort: Number(e.target.value.replace(/\D/g, '')) || 0})} inputMode="numeric" aria-label="Public port" style={{width: 110}} /></Row>
        <Row label="Encrypted connections"><Segmented size="sm" label="Encryption" value={n.encrypted} onChange={v => form.set({encrypted: v})} options={[{value: 'required', label: 'Required'}, {value: 'preferred', label: 'Preferred'}, {value: 'optional', label: 'Optional'}]} /></Row>
      </Panel>
      <Panel title="Quality from outside" padded={false}>
        <Row label="Limit each remote stream to" help="Keeps your upload from saturating."><Stepper label="Remote limit" value={n.qualityLimit} onChange={v => form.set({qualityLimit: v})} min={1} max={100} suffix="Mb/s" /></Row>
        <Row label="Your upload speed" help="Measured 3 days ago."><Stepper label="Upload speed" value={n.uploadSpeed} onChange={v => form.set({uploadSpeed: v})} min={1} max={1000} suffix="Mb/s" /></Row>
        <Row label="Pause remote streams idle for"><Stepper label="Idle pause" value={n.pausedLimit} onChange={v => form.set({pausedLimit: v})} min={5} max={240} step={5} suffix="min" /></Row>
      </Panel>
      <Panel title="Advanced" padded={false} actions={<Button size="sm" variant="ghost" label={adv ? 'Hide' : 'Show'} onClick={() => setAdv(!adv)} />}>
        {adv ? (
          <>
            <Row label="Home networks" help="Addresses treated as home, like 192.168.1.0/24. Blank finds them automatically." stack><Input value={n.homeNetworks} onChange={e => form.set({homeNetworks: e.target.value})} placeholder="Found automatically" className="mono" aria-label="Home networks" /></Row>
            <Row label="Extra addresses" help="Other ways to reach the server, like a reverse proxy." stack><Input value={n.extraAddresses} onChange={e => form.set({extraAddresses: e.target.value})} className="mono" aria-label="Extra addresses" /></Row>
            <Row label="Trusted proxies" stack><Input value={n.trustedProxies} onChange={e => form.set({trustedProxies: e.target.value})} placeholder="None" className="mono" aria-label="Trusted proxies" /></Row>
            <Row label="Treat outside as home" help="Only for testing."><Switch checked={n.wanAsLan} onChange={v => form.set({wanAsLan: v})} label="Treat outside as home" /></Row>
            <Row label="IPv6"><Switch checked={n.ipv6} onChange={v => form.set({ipv6: v})} label="IPv6" /></Row>
            <Row label="Announce on the home network"><Switch checked={n.discovery} onChange={v => form.set({discovery: v})} label="Discovery" /></Row>
            <Row label="Network interface"><Select label="Interface" value={n.interface} onChange={v => form.set({interface: v})} options={[{value: 'auto', label: 'Automatic'}, {value: 'eth0', label: 'eth0 · 192.168.1.20'}, {value: 'wlan0', label: 'wlan0'}]} /></Row>
          </>
        ) : <Row label="Home networks, proxies, IPv6 and the network interface" help="Only change these if you know you need to." />}
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

function People({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('people');
  const libs = useAdmin('libraries');
  const [edit, setEdit] = useState<AdminAccount | null>(null);
  const [adding, setAdding] = useState(false);
  const [del, setDel] = useState<AdminAccount | null>(null);
  const [f, setF] = useState({name: '', username: '', password: ''});
  if (!q.data) return <AdminSkeleton />;
  const set = (list: AdminAccount[]) => services.admin.set('people', list);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Button variant="primary" icon="addPerson" label="Add account" onClick={() => setAdding(true)} />} />
      <Panel padded={false}>
        {q.data.map(a => (
          <div key={a.id} className={`person-row${a.disabled ? ' is-off' : ''}`}>
            <Avatar name={a.name} color={a.profiles[0]?.color} size={44} />
            <div className="set-text"><strong>{a.name} <span className="faint">@{a.username}</span></strong><span>{a.kind} · {a.role === 'owner' ? 'Owner' : a.role === 'admin' ? 'Admin' : 'Member'} · {a.libraries === 'all' ? 'All libraries' : a.libraries.join(', ')} · seen {relativeTime(a.lastSeen).toLowerCase()}</span></div>
            <div className="person-profiles">{a.profiles.map(p => <Avatar key={p.id} name={p.name} color={p.color} size={26} />)}</div>
            {a.disabled ? <Badge>Disabled</Badge> : null}
            {a.role !== 'owner' ? <Button size="sm" variant="secondary" label="Edit" onClick={() => setEdit(structuredClone(a))} /> : <Badge tone="accent">You</Badge>}
          </div>
        ))}
      </Panel>
      {edit ? (
        <Dialog open onOpenChange={o => !o && setEdit(null)} title={edit.name} description={`${edit.kind} · @${edit.username}`} size="md" footer={<><Button variant="danger" label="Delete…" onClick={() => { setDel(edit); setEdit(null); }} /><span style={{flex: 1}} /><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Save" onClick={() => void set(q.data!.map(x => (x.id === edit.id ? edit : x))).then(() => { toast('Saved'); setEdit(null); })} /></>}>
          <div className="stack">
            <div className="field"><span className="field-label">Role</span><Segmented size="sm" label="Role" value={edit.role} onChange={v => setEdit({...edit, role: v})} options={[{value: 'member', label: 'Member'}, {value: 'admin', label: 'Admin'}]} /></div>
            <div className="field"><span className="field-label">Libraries they can see</span>
              <Checkbox checked={edit.libraries === 'all'} onChange={v => setEdit({...edit, libraries: v ? 'all' : []})} label="All libraries, including new ones" />
              {edit.libraries !== 'all' ? <div className="chip-row" style={{marginTop: 8}}>{(libs.data ?? []).map(l => { const on = (edit.libraries as string[]).includes(l.name); return <button key={l.id} type="button" className={`chip${on ? ' is-on' : ''}`} aria-pressed={on} onClick={() => setEdit({...edit, libraries: on ? (edit.libraries as string[]).filter(x => x !== l.name) : [...(edit.libraries as string[]), l.name]})}>{l.name}</button>; })}</div> : null}
            </div>
            <div className="grid-2">
              <div className="field"><span className="field-label">Streams at once</span><Stepper label="Streams" value={edit.limits.streams} onChange={v => setEdit({...edit, limits: {...edit.limits, streams: v}})} max={10} suffix={edit.limits.streams ? '' : '(no limit)'} /></div>
              <div className="field"><span className="field-label">Remote quality</span><Stepper label="Remote Mb/s" value={edit.limits.remoteMbps} onChange={v => setEdit({...edit, limits: {...edit.limits, remoteMbps: v}})} max={100} suffix="Mb/s" /></div>
            </div>
            <Field label="Highest rating"><Select value={edit.limits.rating ?? 'none'} onChange={v => setEdit({...edit, limits: {...edit.limits, rating: v === 'none' ? null : v}})} options={[{value: 'none', label: 'No limit'}, ...['G', 'PG', 'PG-13', 'R'].map(r => ({value: r, label: r}))]} /></Field>
            <Checkbox checked={edit.disabled} onChange={v => setEdit({...edit, disabled: v})} label="Disable this account (they can’t sign in; nothing is deleted)" />
          </div>
        </Dialog>
      ) : null}
      <Dialog open={adding} onOpenChange={setAdding} title="Add a server account" description="For someone without a Portico Account. To invite someone who has one, use Access › Invitations." size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Add account" disabled={!f.name || !f.username || f.password.length < 10} onClick={() => void set([...q.data!, {id: 'ac' + Date.now(), name: f.name, username: f.username, kind: 'Server account', role: 'member', disabled: false, profiles: [{id: 'np' + Date.now(), name: f.name.split(' ')[0]!, color: '#3fae78', kids: false}], libraries: 'all', limits: {streams: 2, devices: 3, remoteMbps: 8, rating: null}, lastSeen: Date.now()}]).then(() => { toast(`Added ${f.name}. They’ll choose a new password when they sign in.`); setAdding(false); setF({name: '', username: '', password: ''}); })} /></>}>
        <div className="stack">
          <Field label="Name"><Input value={f.name} onChange={e => setF({...f, name: e.target.value})} autoFocus /></Field>
          <Field label="Username"><Input value={f.username} onChange={e => setF({...f, username: e.target.value})} /></Field>
          <Field label="Temporary password" help="At least 10 characters. They change it when they first sign in."><Input value={f.password} onChange={e => setF({...f, password: e.target.value})} /></Field>
        </div>
      </Dialog>
      <ConfirmDialog open={!!del} onOpenChange={o => !o && setDel(null)} danger title={`Delete ${del?.name}’s account?`} body="Their profiles, history and playlists on this server are deleted." typeToConfirm={del?.username} confirmLabel="Delete account" onConfirm={() => set(q.data!.filter(x => x.id !== del?.id)).then(() => toast('Account deleted'))} />
    </div>
  );
}

function Access({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('access');
  const [invite, setInvite] = useState(false);
  const [label, setLabel] = useState('');
  const [role, setRole] = useState<'member' | 'admin'>('member');
  const [created, setCreated] = useState<string | null>(null);
  const [keyOpen, setKeyOpen] = useState(false);
  const [keyName, setKeyName] = useState('');
  const [keyAccess, setKeyAccess] = useState<ApiKey['access']>('Read only');
  const [secret, setSecret] = useState<string | null>(null);
  const [revoke, setRevoke] = useState<{kind: 'invite' | 'key'; id: string; name: string} | null>(null);
  if (!q.data) return <AdminSkeleton />;
  const a = q.data;
  const copy = (s: string, what: string) => { void navigator.clipboard?.writeText(s).catch(() => undefined); toast(`${what} copied`); };
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <Panel title="Invitations" subtitle="Anyone with the link or code can join until it expires." padded={false} actions={<Button size="sm" variant="primary" icon="plus" label="Invite someone" onClick={() => { setInvite(true); setCreated(null); }} />}>
        {a.invites.map((i: AdminInvite) => (
          <Row key={i.id} icon="mail" label={i.label} help={`${i.role === 'admin' ? 'Admin' : 'Member'} · code ${i.code} · expires ${untilTime(i.expires)}`}>
            <Button size="sm" variant="secondary" icon="copy" label="Copy link" onClick={() => copy(i.link, 'Invitation link')} />
            <Button size="sm" variant="ghost" label="Revoke" onClick={() => setRevoke({kind: 'invite', id: i.id, name: i.label})} />
          </Row>
        ))}
        {!a.invites.length ? <Row label="No open invitations" /> : null}
      </Panel>
      <Panel title="Devices" subtitle="New devices wait for approval before they can browse." padded={false}>
        {a.devices.map(d => (
          <Row key={d.id} icon={d.platform === 'web' ? 'monitor' : d.platform === 'ios' ? 'phone' : 'tv'} label={d.name} help={`${d.account} · ${d.platform} · seen ${relativeTime(d.lastSeen).toLowerCase()}`}>
            <StatusBadge state={d.trust} />
            {d.trust === 'waiting' ? <Button size="sm" variant="primary" label="Approve" onClick={() => void services.admin.set('access', {...a, devices: a.devices.map(x => (x.id === d.id ? {...x, trust: 'approved'} : x))}).then(() => toast(`${d.name} approved`))} /> : null}
            <Button size="sm" variant="ghost" label={d.trust === 'blocked' ? 'Unblock' : 'Block'} onClick={() => void services.admin.set('access', {...a, devices: a.devices.map(x => (x.id === d.id ? {...x, trust: d.trust === 'blocked' ? 'approved' : 'blocked'} : x))}).then(() => toast(d.trust === 'blocked' ? 'Unblocked' : `${d.name} blocked`))} />
          </Row>
        ))}
      </Panel>
      <Panel title="API keys" subtitle="For other apps, like home automation or scripts. Keys act for your account." padded={false} actions={<Button size="sm" variant="secondary" icon="plus" label="New key" onClick={() => { setKeyOpen(true); setSecret(null); }} />}>
        {a.keys.map(k => <Row key={k.id} icon="key" label={k.name} help={`${k.access} · ends in …${k.hint} · ${k.lastUsed ? `used ${relativeTime(k.lastUsed).toLowerCase()}` : 'never used'}`}><Button size="sm" variant="ghost" label="Revoke" onClick={() => setRevoke({kind: 'key', id: k.id, name: k.name})} /></Row>)}
      </Panel>
      <Dialog open={invite} onOpenChange={setInvite} title={created ? 'Invitation ready' : 'Invite someone'} size="sm" footer={created ? <Button variant="primary" label="Done" onClick={() => setInvite(false)} /> : <><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Create invitation" onClick={() => void services.admin.run('createInvite', {label: label || 'Invitation', role}).then(code => setCreated(code))} /></>}>
        {created ? (
          <div className="stack">
            <p className="muted">Send them this link, or the code to use in any Portico app. It works for 7 days.</p>
            <code className="setup-key">https://getportico.tv/join#{created}</code>
            <div className="hstack"><Button variant="secondary" icon="copy" label="Copy link" onClick={() => copy(`https://getportico.tv/join#${created}`, 'Link')} /><Button variant="ghost" icon="copy" label={`Copy code ${created}`} onClick={() => copy(created, 'Code')} /></div>
          </div>
        ) : (
          <div className="stack">
            <Field label="Who it’s for" optional help="Only you see this."><Input value={label} onChange={e => setLabel(e.target.value)} placeholder="uncle.dave@example.com" autoFocus /></Field>
            <ChoiceCards label="Role" columns={1} value={role} onChange={setRole} options={[{value: 'member', title: 'Member', body: 'Watches what you share.'}, {value: 'admin', title: 'Admin', body: 'Also manages libraries, people and settings.'}]} />
          </div>
        )}
      </Dialog>
      <Dialog open={keyOpen} onOpenChange={setKeyOpen} title={secret ? 'Copy your new key' : 'New API key'} size="sm" footer={secret ? <Button variant="primary" label="I’ve copied it" onClick={() => setKeyOpen(false)} /> : <><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Create key" disabled={!keyName.trim()} onClick={() => void services.admin.run('createKey', {name: keyName, access: keyAccess}).then(setSecret)} /></>}>
        {secret ? (
          <div className="stack"><Notice tone="warn" title="This is the only time it’s shown">Store it in the app that needs it. If you lose it, revoke it and make a new one.</Notice><code className="setup-key" style={{wordBreak: 'break-all'}}>{secret}</code><Button variant="secondary" icon="copy" label="Copy key" onClick={() => copy(secret, 'Key')} /></div>
        ) : (
          <div className="stack"><Field label="Name"><Input value={keyName} onChange={e => setKeyName(e.target.value)} placeholder="Home Assistant" autoFocus /></Field><Field label="Access"><Select value={keyAccess} onChange={setKeyAccess} options={[{value: 'Read only', label: 'Read only'}, {value: 'Playback', label: 'Playback control'}, {value: 'Full', label: 'Full access'}]} /></Field></div>
        )}
      </Dialog>
      <ConfirmDialog open={!!revoke} onOpenChange={o => !o && setRevoke(null)} danger title={`Revoke ${revoke?.name}?`} body={revoke?.kind === 'key' ? 'Apps using it stop working right away.' : 'The link and code stop working. People who already joined stay.'} confirmLabel="Revoke" onConfirm={() => services.admin.set('access', revoke?.kind === 'key' ? {...a, keys: a.keys.filter(k => k.id !== revoke.id)} : {...a, invites: a.invites.filter(i => i.id !== revoke?.id)}).then(() => toast('Revoked'))} />
    </div>
  );
}

function Schedules({sec}: {sec: Sec}) {
  const form = useAdminForm('schedules');
  const s = form.data;
  if (!s) return <AdminSkeleton />;
  const upd = (id: string, patch: Partial<Schedule>) => form.replace(s.map(x => (x.id === id ? {...x, ...patch} : x)));
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <Panel padded={false}>
        {s.map(x => (
          <div key={x.id} className="sched-row">
            <div className="set-text"><strong>{x.name}</strong><span>{x.description} Last ran {relativeTime(x.lastRun).toLowerCase()}{x.lastResult === 'failed' ? ' and failed' : ''}{x.enabled && x.nextRun ? ` · next ${new Date(x.nextRun).toLocaleString('en-US', {weekday: 'short', hour: 'numeric', minute: '2-digit'})}` : ''}.</span></div>
            <label className="sched-field"><span>Starts</span><Input type="time" value={x.start} onChange={e => upd(x.id, {start: e.target.value})} aria-label={`${x.name} start time`} /></label>
            <label className="sched-field"><span>For up to</span><Select label={`${x.name} length`} value={String(x.lengthMin)} onChange={v => upd(x.id, {lengthMin: Number(v)})} options={[30, 60, 90, 120, 240].map(m => ({value: String(m), label: m < 60 ? `${m} min` : `${m / 60} hr`}))} /></label>
            <Checkbox checked={x.catchUp} onChange={v => upd(x.id, {catchUp: v})} label="Catch up if missed" />
            <Switch checked={x.enabled} onChange={v => upd(x.id, {enabled: v})} label={`${x.name} on`} />
            <StatusBadge state={x.lastResult} />
          </div>
        ))}
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

function Maintenance({sec}: {sec: Sec}) {
  const services = useServices();
  const form = useAdminForm('maintenance');
  const [cleaning, setCleaning] = useState<string | null>(null);
  const [empty, setEmpty] = useState(false);
  const [sending, setSending] = useState(false);
  const m = form.data;
  if (!m) return <AdminSkeleton />;
  const due = m.trash.filter(t => t.due);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      {m.update.available ? (
        <Panel className="update-panel">
          <div className="set-console-row">
            <span className="set-icon is-accent"><Icon name="sparkles" size={22} /></span>
            <div className="set-text"><strong>Portico {m.update.latest} is available</strong><span>You have {m.update.current} ({m.update.channel}). {m.update.notes.join('. ')}.</span></div>
            <Button variant="primary" label="Update tonight" onClick={() => toast(`Updating to ${m.update.latest} at the start of quiet hours`)} />
          </div>
        </Panel>
      ) : null}
      {!m.permissionsOk ? <Notice tone="warn" title="Portico can’t write to 2 folders" action={<Button size="sm" variant="secondary" label="Fix permissions" onClick={() => void services.admin.run('fixPermissions').then(() => { form.discard(); toast('Permissions fixed'); })} />}>/Volumes/Media/Recordings and /var/lib/portico/cache. Recordings and artwork may fail until this is fixed.</Notice> : null}
      <Panel title="Quiet hours" subtitle="Heavy background work waits for these windows." padded={false}>
        {m.quietHours.map((q, i) => <Row key={q.id} icon="moon" label={q.name} help={`${q.days.length === 7 ? 'Every day' : q.days.join(', ')} · from ${q.start} for ${q.hours} hours`}><Switch checked={q.enabled} onChange={v => form.set({quietHours: m.quietHours.map((x, j) => (j === i ? {...x, enabled: v} : x))})} label={`${q.name} on`} /></Row>)}
        <Row label="Only run heavy work in quiet hours"><Switch checked={m.onlyInWindows} onChange={v => form.set({onlyInWindows: v})} label="Only in quiet hours" /></Row>
        <Row label="Background work priority"><Segmented size="sm" label="Priority" value={m.priority} onChange={v => form.set({priority: v})} options={[{value: 'lower', label: 'Lower than playback'}, {value: 'normal', label: 'Normal'}]} /></Row>
      </Panel>
      <Panel title="Storage cleanup" subtitle="Things Portico makes and can make again." padded={false}>
        {m.buckets.map(b => <Row key={b.id} icon="drive" label={`${b.name} · ${b.sizeGB} GB`} help={b.description}>{b.canRecreate ? <Button size="sm" variant="ghost" label="Clear" busy={cleaning === b.id} onClick={() => { setCleaning(b.id); setTimeout(() => { setCleaning(null); form.set({buckets: m.buckets.map(x => (x.id === b.id ? {...x, sizeGB: 0} : x))}); toast(`${b.sizeGB} GB freed; ${b.name.toLowerCase()} are made again as needed`); }, 900); }} /> : <Badge>Kept</Badge>}</Row>)}
      </Panel>
      <Panel title="Deleted titles" subtitle="Kept for their library’s keep period, then removed for good." padded={false} actions={due.length ? <Button size="sm" variant="danger" label={`Remove ${due.length} now`} onClick={() => setEmpty(true)} /> : null}>
        {m.trash.map(t => <Row key={t.id} icon="trash" label={t.title} help={`${t.files} ${t.files === 1 ? 'file' : 'files'} · ${t.sizeGB} GB · deleted ${relativeTime(t.deletedAt).toLowerCase()} · ${t.due ? 'due for removal' : `kept until ${new Date(t.keptUntil).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`}`}><Button size="sm" variant="secondary" label="Put back" onClick={() => void services.admin.run('putBack', {id: t.id}).then(() => { form.discard(); toast(`“${t.title}” is back in its library`); })} /></Row>)}
        {!m.trash.length ? <Row label="Nothing deleted" /> : null}
      </Panel>
      <Panel title="Message everyone" subtitle="Shows as a notification in every app on this server.">
        <div className="stack">
          <Field label="Title"><Input value={m.broadcast.title} onChange={e => form.set({broadcast: {...m.broadcast, title: e.target.value}})} placeholder="Server maintenance on Saturday" /></Field>
          <Field label="Message"><TextArea value={m.broadcast.message} onChange={e => form.set({broadcast: {...m.broadcast, message: e.target.value}})} rows={3} /></Field>
          <div className="hstack">
            <Segmented size="sm" label="Audience" value={m.broadcast.audience} onChange={v => form.set({broadcast: {...m.broadcast, audience: v}})} options={[{value: 'everyone', label: 'Everyone'}, {value: 'admins', label: 'Admins'}]} />
            <Segmented size="sm" label="Importance" value={m.broadcast.importance} onChange={v => form.set({broadcast: {...m.broadcast, importance: v}})} options={[{value: 'info', label: 'Info'}, {value: 'notice', label: 'Notice'}, {value: 'urgent', label: 'Urgent'}]} />
            <span className="spacer" style={{flex: 1}} />
            <Button variant="primary" icon="send" label="Send" busy={sending} disabled={!m.broadcast.title || !m.broadcast.message} onClick={() => { setSending(true); setTimeout(() => { setSending(false); toast('Message sent to everyone'); form.set({broadcast: {...m.broadcast, title: '', message: ''}}); }, 700); }} />
          </div>
        </div>
      </Panel>
      <ConfirmDialog open={empty} onOpenChange={setEmpty} danger title={`Remove ${due.length} titles for good?`} body={`${due.reduce((a, t) => a + t.sizeGB, 0).toFixed(1)} GB of files are deleted from disk. This can’t be undone.`} confirmLabel="Remove for good" onConfirm={() => services.admin.run('removeDue').then(() => { form.discard(); toast('Removed'); })} />
      <UnsavedBar form={form} />
    </div>
  );
}

function Backups({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('backups');
  const [busy, setBusy] = useState(false);
  const [restore, setRestore] = useState<Backup | null>(null);
  const [del, setDel] = useState<Backup | null>(null);
  if (!q.data) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<><label className="btn btn-secondary btn-md"><Icon name="upload" size={18} /><span className="btn-label">Restore from file…</span><input type="file" accept=".portico-backup,.zip" className="sr-only" onChange={e => e.target.files?.[0] && toast(`Checking ${e.target.files[0].name}…`)} /></label><Button variant="primary" icon="database" label="Back up now" busy={busy} onClick={() => { setBusy(true); void services.admin.run('backup').then(() => { setBusy(false); toast('Backup complete'); }); }} /></>} />
      <div className="stat-row">
        <div className="stat is-good"><span className="stat-label"><Icon name="success" size={15} />Last backup</span><strong className="stat-value">{relativeTime(q.data.list[0]?.at ?? Date.now())}</strong><span className="stat-sub">{q.data.lastResult}</span></div>
        <div className="stat"><span className="stat-label"><Icon name="calendar" size={15} />Schedule</span><strong className="stat-value">{q.data.schedule}</strong><span className="stat-sub">Keeps the last {q.data.keep}</span></div>
      </div>
      <Panel padded={false}>
        {q.data.list.map(b => (
          <Row key={b.id} icon="database" label={new Date(b.at).toLocaleString('en-US', {dateStyle: 'medium', timeStyle: 'short'})} help={`${b.kind} · ${b.sizeMB} MB · Portico ${b.version} · schema ${b.schema}`}>
            <Button size="sm" variant="ghost" icon="download" aria-label="Download backup" onClick={() => toast(`Downloading ${b.id}.portico-backup`)} />
            <Button size="sm" variant="secondary" label="Restore…" onClick={() => setRestore(b)} />
            <Button size="sm" variant="ghost" icon="trash" aria-label="Delete backup" onClick={() => setDel(b)} />
          </Row>
        ))}
      </Panel>
      <ConfirmDialog open={!!restore} onOpenChange={o => !o && setRestore(null)} danger title="Restore this backup?" body={<><p>The server restarts and goes back to how it was on {restore ? new Date(restore.at).toLocaleString('en-US', {dateStyle: 'medium', timeStyle: 'short'}) : ''}. Changes since then are lost; media files aren’t touched.</p><p>A backup of the current state is made first.</p></>} confirmLabel="Restore and restart" onConfirm={() => { toast('Restoring… the server will be back in a minute'); }} />
      <ConfirmDialog open={!!del} onOpenChange={o => !o && setDel(null)} danger title="Delete this backup?" confirmLabel="Delete" onConfirm={() => services.admin.run('deleteBackup', {id: del?.id}).then(() => toast('Backup deleted'))} />
    </div>
  );
}

function General({sec}: {sec: Sec}) {
  const form = useAdminForm('general');
  const session = useSession();
  const [custody, setCustody] = useState(true);
  const g = form.data;
  if (!g) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      {custody ? <Notice tone="info" title="Keep this server registered to you" action={<Button size="sm" variant="primary" label="Accept" onClick={() => { setCustody(false); toast('This server’s registration is now held by your Portico Account'); }} />}>Portico moved server registration to Portico Accounts. Accept to keep receiving updates and remote access for {g.name}.</Notice> : null}
      <Panel padded={false}>
        <Row label="Server name" help="People see this when they choose a server."><Input value={g.name} onChange={e => form.set({name: e.target.value})} aria-label="Server name" style={{width: 260}} /></Row>
        <Row label="Version" help={`${g.platform}`}>{g.version}</Row>
        <Row label="Server ID"><code>{g.serverId}</code></Row>
        <Row label="Data folder"><code>{g.dataFolder}</code></Row>
      </Panel>
      <Panel title="Portico Account" padded={false}>
        <Row icon="account" label="Connected to" help={g.connection === 'connected' ? 'Sign-in with Portico Accounts, invitations and remote access work.' : 'Not connected: only server accounts can sign in.'}>{g.connection === 'connected' ? <><Badge tone="good" icon="check">{g.ownerAccount}</Badge><Button size="sm" variant="ghost" label="Disconnect…" onClick={() => toast('Disconnecting needs the recovery owner to be set')} /></> : <Button size="sm" variant="primary" label="Connect" to="/claim" />}</Row>
        <Row icon="shield" label="Recovery owner" help="Can sign in with the recovery key if the Portico Account is lost.">{g.recoveryOwner}</Row>
      </Panel>
      <Panel title="About this server">
        <KeyValue items={[['Owner', session.server?.name ? 'Justin Ehler' : '—'], ['Address', session.server?.host ?? '—'], ['Up for', '23 days']]} />
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

