import React from 'react';
import {View} from 'react-native';
import type {AdminAccount, ApiKey} from '../../data/admin';
import {Button} from '../../design/Button';
import {Badge, Chip, ListRow, Meter, Panel, Segmented, Stepper, TextField, ToggleRow} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {Banner, RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, u} from '../../design/tokens';
import {go} from '../../nav/routes';
import {ago, KV, useAdmin} from './common';
import {plural} from '../../design/format';

export function TranscodingSection() {
  const {data, save} = useAdmin('transcoding');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      <Panel title="Conversion" lede="When a device cannot play a file as it is, the server can convert it on the fly.">
        <ToggleRow label="Allow transcoding" help="When a device can’t play a file, the server converts it. If you turn this off, those devices show an error instead." value={data.allow} onChange={v => save({allow: v})} />
        <ListRow label="Hardware encoder" value={{auto: 'Automatic (Intel Quick Sync)', videotoolbox: 'VideoToolbox', vaapi: 'VAAPI', nvenc: 'NVENC', off: 'Processor only'}[data.hardware]} onPress={() => ui.menu({title: 'Hardware encoder', actions: (['auto', 'vaapi', 'nvenc', 'off'] as const).map(h => ({label: {auto: 'Automatic', videotoolbox: 'VideoToolbox', vaapi: 'VAAPI', nvenc: 'NVENC', off: 'Processor only'}[h], checked: data.hardware === h, onPress: () => save({hardware: h})}))})} />
        <ToggleRow label="Convert HDR for screens that can’t show it" value={data.tonemap} onChange={v => save({tonemap: v})} />
        <ToggleRow label="Isolate the video tools" help="This host can’t isolate the video tools, so conversions run with baseline protections." value={data.sandbox} onChange={v => save({sandbox: v})} disabled />
      </Panel>
      <Panel title="Capacity" lede="Leave a field empty for no limit.">
        <Stepper label="Simultaneous streams" help="Across everyone using this server." value={data.serverCap} onChange={v => save({serverCap: v})} min={0} max={20} />
        <Stepper label="Per account" help="Applies to each account. Set a member’s own stream limit in Server › Access." value={data.accountCap} onChange={v => save({accountCap: v})} min={0} max={10} />
        {data.overrides.map(o => <ListRow key={o.account} label={o.account} help="Override for this account." value={`Cap ${o.cap}`} onPress={() => ui.confirm({title: 'Remove override?', confirmLabel: 'Remove override', onConfirm: () => save({overrides: data.overrides.filter(x => x.account !== o.account)})})} />)}
        <Button label="Add an account override" icon="plus" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.menu({title: 'Choose an account', actions: ['Maya Ehler', 'Grandma Ruth'].map(a => ({label: a, onPress: () => save({overrides: [...data.overrides, {account: a, cap: 1}]})}))})} />
      </Panel>
    </View>
  );
}

export function NetworkSection() {
  const {data, save} = useAdmin('network');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      <Panel title="Remote access" lede="Lets your devices reach this server from outside your home through your Portico Account." right={<Badge label={data.remote ? 'On' : 'Off'} tone={data.remote ? 'healthy' : 'neutral'} />}>
        <ToggleRow label="Remote access" help="When off, verified local connections keep working." value={data.remote} onChange={v => save({remote: v})} />
        <KV k="Public address" v={`${data.publicAddress}:${data.publicPort}`} />
        <KV k="Home addresses" v={data.localAddresses.join(', ')} />
        <KV k="Router mapping" v={`${data.mapping.protocol} · ${data.mapping.state === 'open' ? 'Port open' : 'the router refused it'}`} tone={data.mapping.state === 'open' ? 'healthy' : 'amber'} />
        <ToggleRow label="Automatic port mapping" help="Ask the router to open a port. Works on most home routers." value={data.mapping.enabled} onChange={v => save({mapping: {...data.mapping, enabled: v}})} />
      </Panel>
      <Panel title="HTTPS certificate" lede="A publicly trusted certificate lets browsers and apps connect securely from anywhere." right={<Badge label={data.certificate.state === 'trusted' ? 'Trusted' : data.certificate.state} tone="healthy" />}>
        <KV k="Domain" v={data.certificate.domain} />
        <KV k="Valid until" v={new Date(data.certificate.expires).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'})} />
        <Button label="Request a certificate" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.toast('Portico obtains and renews it automatically through your Portico Account.')} />
      </Panel>
      <Panel title="Access from outside" lede="Rules for devices that are not on your home network.">
        <ListRow label="Who may sign in from outside" value={{everyone: 'Everyone', accountsOnly: 'Portico Accounts only', nobody: 'Nobody'}[data.outside]} onPress={() => ui.menu({title: 'Who may sign in from outside', actions: (['everyone', 'accountsOnly', 'nobody'] as const).map(o => ({label: {everyone: 'Everyone', accountsOnly: 'Portico Accounts only', nobody: 'Nobody'}[o], checked: data.outside === o, onPress: () => save({outside: o})}))})} />
        <ListRow label="Encrypted connections" help="Whether a device may connect without HTTPS." value={data.encrypted} onPress={() => ui.menu({title: 'Encrypted connections', actions: (['required', 'preferred', 'optional'] as const).map(o => ({label: o, checked: data.encrypted === o, onPress: () => save({encrypted: o})}))})} />
        <Stepper label="Quality limit away from home" help="Highest bitrate for anyone outside your home network. Zero means no limit." value={data.qualityLimit} onChange={v => save({qualityLimit: v})} min={0} max={80} unit="Mb/s" />
        <Stepper label="Upload speed" help="Streams away from home share about 80% of it. Zero means unknown." value={data.uploadSpeed} onChange={v => save({uploadSpeed: v})} min={0} max={1000} step={5} unit="Mb/s" />
        <ListRow label="End paused videos after" help="Frees the stream for someone else. Music, audiobooks and Live TV are never ended." value={data.pausedLimit ? `${plural(data.pausedLimit, 'minute', 'minutes')}` : 'Never'} onPress={() => ui.menu({title: 'End paused videos after', actions: [0, 30, 60, 120, 240].map(m => ({label: m ? `${plural(m, 'minute', 'minutes')}` : 'Never', checked: data.pausedLimit === m, onPress: () => save({pausedLimit: m})}))})} />
        <ToggleRow label="Find this server on the home network" value={data.discovery} onChange={v => save({discovery: v})} />
        <ToggleRow label="Allow incoming IPv6 connections" help="Your router already allows incoming IPv6." value={data.ipv6} onChange={v => save({ipv6: v})} />
        <ToggleRow label="Count your public address as home" help="For routers that send devices at home through your public address." value={data.wanAsLan} onChange={v => save({wanAsLan: v})} />
      </Panel>
      <Panel title="Advanced">
        <TextField label="Home networks" value={data.homeNetworks} onChangeText={v => save({homeNetworks: v})} placeholder="192.168.2.0/24" help="One per line. Leave empty to use the usual private ranges." multiline style={{marginHorizontal: u(14)}} />
        <TextField label="Extra addresses" value={data.extraAddresses} onChangeText={v => save({extraAddresses: v})} help="HTTPS addresses members’ apps can also use, such as your own domain behind a reverse proxy." multiline style={{marginHorizontal: u(14)}} />
        <TextField label="Trusted reverse proxies" value={data.trustedProxies} onChangeText={v => save({trustedProxies: v})} placeholder="127.0.0.1" multiline style={{marginHorizontal: u(14)}} />
        <ListRow label="Address for home devices" value="Automatic" help="Automatic skips container and virtual networks." />
      </Panel>
    </View>
  );
}

export function PeopleSection() {
  const {data, save} = useAdmin('people');
  if (!data) return <RowsSkeleton rows={1} />;
  const edit = (a: AdminAccount) => ui.sheet({title: a.name, subtitle: `${a.kind} · ${a.role} · last seen ${ago(a.lastSeen)}`, render: close => (
    <View>
      {a.kind === 'Portico Account' ? <Text variant="caption" tone="tertiary" style={{marginBottom: 8}}>Signs in with their Portico Account. Their password and two-step verification are managed there.</Text> : <TextField label="New password" value="" onChangeText={() => {}} secure help="Leave empty to keep the current password." />}
      <Text variant="captionStrong" tone="secondary">Profiles</Text>
      {a.profiles.map(p => <ListRow key={p.id} label={p.name} help={p.kids ? 'Kids' : 'Profile'} compact onPress={() => ui.prompt({title: 'Edit profile', fields: [{key: 'n', label: 'Profile name', initial: p.name, help: 'Shown when they pick a profile.'}, {key: 'p', label: 'New PIN', secure: true, help: 'Four digits. Asked for when switching to this profile.'}], submitLabel: 'Save', onSubmit: () => null})} />)}
      <Text variant="captionStrong" tone="secondary" style={{marginTop: 10}}>Libraries they can see</Text>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginVertical: 8}}>
        <Chip label="All libraries, including ones added later" active={a.libraries === 'all'} onPress={() => save(data.map(x => (x.id === a.id ? {...x, libraries: 'all'} : x)))} />
        {['Movies', 'TV Shows', 'Anime', 'Music', 'Audiobooks', 'Photos', 'Home Videos'].map(l => <Chip key={l} label={l} active={a.libraries !== 'all' && a.libraries.includes(l)} onPress={() => save(data.map(x => (x.id === a.id ? {...x, libraries: x.libraries === 'all' ? [l] : x.libraries.includes(l) ? x.libraries.filter(y => y !== l) : [...x.libraries, l]} : x)))} />)}
      </View>
      {a.role !== 'owner' ? <ToggleRow label="Disable this account" help="They cannot sign in until it is enabled again." value={a.disabled} onChange={v => save(data.map(x => (x.id === a.id ? {...x, disabled: v} : x)))} /> : null}
      <View style={{flexDirection: 'row', gap: 10, marginTop: 10}}>
        <Button label="Done" variant="primary" hasTVPreferredFocus onPress={close} />
        {a.role !== 'owner' ? <Button label="Delete" variant="danger" onPress={() => ui.confirm({title: `Delete “${a.name}”?`, body: 'Its watch history, Watchlist and playlists are deleted. This cannot be undone.', confirmLabel: 'Delete', destructive: true, onConfirm: () => { save(data.filter(x => x.id !== a.id)); close(); }})} /> : null}
      </View>
    </View>
  )});
  return (
    <View>
      <View style={{flexDirection: 'row', gap: 10, marginBottom: 12}}>
        <Button label="Invite" icon="mail" variant="primary" hasTVPreferredFocus onPress={() => go('Admin', {section: 'access'})} />
        <Button label="Add account" icon="userPlus" onPress={() => ui.prompt({title: 'Add an account', body: 'An account made on this server, for someone without a Portico Account.', fields: [{key: 'n', label: 'Name'}, {key: 'u', label: 'Username'}, {key: 'p', label: 'Password', secure: true}], submitLabel: 'Add account', onSubmit: v => { if (!v.n || !v.u) return 'Enter a name and username.'; save([...data, {id: 'ac' + Date.now(), name: v.n, username: v.u, kind: 'Server account', role: 'member', disabled: false, profiles: [{id: 'p', name: v.n.split(' ')[0], color: '#409fd4', kids: false}], libraries: 'all', limits: {streams: 2, devices: 3, remoteMbps: 0, rating: null}, lastSeen: Date.now()}]); return null; }})} />
      </View>
      <Panel title="Accounts on this server" lede="Everyone who can use this server: Portico Account members and accounts made here. Each has its own profiles.">
        {data.map(a => <ListRow key={a.id} icon="user" label={a.name} help={`${a.kind} · @${a.username} · ${plural(a.profiles.length, 'profile', 'profiles')} · ${a.libraries === 'all' ? 'All libraries' : a.libraries.join(', ')}`} right={<View style={{flexDirection: 'row', gap: 6}}>{a.role === 'owner' ? <Badge label="Owner" /> : a.role === 'admin' ? <Badge label="Admin" tone="amber" /> : null}{a.disabled ? <Badge label="Disabled" tone="danger" /> : null}</View>} onPress={() => edit(a)} />)}
      </Panel>
    </View>
  );
}

export function AccessSection() {
  const people = useAdmin('people');
  const {data, save, run} = useAdmin('access');
  if (!data || !people.data) return <RowsSkeleton rows={1} />;
  const limits = (a: AdminAccount) => ui.sheet({title: `Limits for ${a.name}`, subtitle: 'Zero means no limit.', render: close => (
    <View>
      <Stepper label="Streams at once" value={a.limits.streams} onChange={v => people.save(people.data!.map(x => (x.id === a.id ? {...x, limits: {...x.limits, streams: v}} : x)))} min={0} max={10} />
      <Stepper label="Signed-in devices" value={a.limits.devices} onChange={v => people.save(people.data!.map(x => (x.id === a.id ? {...x, limits: {...x.limits, devices: v}} : x)))} min={0} max={20} />
      <Stepper label="Quality away from home" help="Highest bitrate when not on your home network." value={a.limits.remoteMbps} onChange={v => people.save(people.data!.map(x => (x.id === a.id ? {...x, limits: {...x.limits, remoteMbps: v}} : x)))} min={0} max={80} unit="Mb/s" />
      <ListRow label="Highest rating" value={a.limits.rating ?? 'No limit'} onPress={() => ui.menu({title: 'Highest rating', actions: ['No limit', 'G', 'PG', 'PG-13', 'R'].map(r => ({label: r, onPress: () => people.save(people.data!.map(x => (x.id === a.id ? {...x, limits: {...x.limits, rating: r === 'No limit' ? null : r}} : x)))}))})} />
      <Button label="Done" variant="primary" onPress={close} hasTVPreferredFocus />
    </View>
  )});
  const createKey = () => ui.prompt({title: 'Create an API key', fields: [{key: 'n', label: 'Name', help: 'What will use this key.'}], submitLabel: 'Create key', onSubmit: v => {
    run('createKey', {name: v.n || 'New key', access: 'Read only'}).then(key => ui.sheet({title: 'Copy your key now', subtitle: 'This is the only time the key is shown. The server keeps a fingerprint, not the key.', render: close => <View><Text variant="mono" selectable>{key}</Text><Button label="I’ve copied it" variant="primary" style={{marginTop: 12}} onPress={close} hasTVPreferredFocus /></View>}));
    return null;
  }});
  return (
    <View>
      <Panel title="People" lede="Administrators can manage libraries and people. Only the owner can change who is an administrator.">
        {people.data.filter(a => a.role !== 'owner').map((a, i) => (
          <ListRow key={a.id} icon="user" label={a.name} help={`${a.role === 'admin' ? 'Administrator' : 'Member'} · ${a.limits.streams ? plural(a.limits.streams, 'stream', 'streams') : 'Unlimited streams'} · ${a.limits.rating ?? 'any rating'}`} hasTVPreferredFocus={i === 0}
            onPress={() => ui.menu({title: a.name, actions: [
              {label: `Role: ${a.role === 'admin' ? 'Administrator' : 'Member'} · change`, icon: 'shield', onPress: () => people.save(people.data!.map(x => (x.id === a.id ? {...x, role: x.role === 'admin' ? 'member' : 'admin'} : x)))},
              {label: 'Limits…', icon: 'gauge', onPress: () => limits(a)},
            ]})} />
        ))}
      </Panel>
      <Panel title="Invitations" lede="Make a link and a code to share. They join with their Portico Account, or make an account on this server. Portico sends no email.">
        {data.invites.map(iv => <ListRow key={iv.id} icon="mail" label={iv.label} help={`${iv.role === 'admin' ? 'Administrator' : 'Member'} · expires ${new Date(iv.expires).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})} · ${iv.code}`} onPress={() => ui.menu({title: iv.label, actions: [{label: 'Copy link', icon: 'copy', onPress: () => ui.toast('Copied')}, {label: 'Revoke', icon: 'x', destructive: true, onPress: () => save({invites: data.invites.filter(x => x.id !== iv.id)})}]})} />)}
        {!data.invites.length ? <Text variant="caption" tone="tertiary" style={{margin: u(14)}}>No invitations waiting.</Text> : null}
        <Button label="Invite someone" icon="plus" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.prompt({title: 'Invite someone', fields: [{key: 'e', label: 'Email (optional)', help: 'Optional — only used to label the invitation.'}], submitLabel: 'Create invitation', onSubmit: v => { run('createInvite', {label: v.e, role: 'member'}).then(code => ui.sheet({title: 'Invitation ready', subtitle: `Send this link to ${v.e || 'the person you’re inviting'}. It works once and is not shown again.`, render: () => <View><Text variant="mono">https://getportico.tv/join#{code}</Text><Text variant="title" style={{marginTop: 8}}>{code}</Text><Button label="Copy link" icon="copy" style={{marginTop: 10}} onPress={() => ui.toast('Copied')} hasTVPreferredFocus /></View>})); return null; }})} />
      </Panel>
      <Panel title="Devices" lede={`${data.devices.filter(d => d.trust === 'waiting').length} device is waiting for your approval.`}>
        {data.devices.map(d => <ListRow key={d.id} icon="monitor" label={d.name} help={`${d.platform} · ${d.account} · last seen ${ago(d.lastSeen)}`} right={<Badge label={d.trust === 'approved' ? 'Approved' : d.trust === 'blocked' ? 'Blocked' : 'Waiting'} tone={d.trust === 'approved' ? 'healthy' : d.trust === 'blocked' ? 'danger' : 'amber'} />} onPress={() => ui.menu({title: d.name, actions: [{label: 'Approve', icon: 'check', onPress: () => save({devices: data.devices.map(x => (x.id === d.id ? {...x, trust: 'approved'} : x))})}, {label: 'Block', icon: 'ban', destructive: true, onPress: () => save({devices: data.devices.map(x => (x.id === d.id ? {...x, trust: 'blocked'} : x))})}]})} />)}
      </Panel>
      <Panel title="API keys">
        {data.keys.map((k: ApiKey) => <ListRow key={k.id} icon="key" label={k.name} help={`${k.access} · ends in ${k.hint} · last used ${k.lastUsed ? ago(k.lastUsed) : 'never'}`} onPress={() => ui.confirm({title: `Revoke “${k.name}”?`, body: 'Anything using this key stops working at once. This cannot be undone.', confirmLabel: 'Revoke', destructive: true, onConfirm: () => save({keys: data.keys.filter(x => x.id !== k.id)})})} />)}
        <Button label="Create a key" icon="plus" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={createKey} />
      </Panel>
    </View>
  );
}

export function SchedulesSection() {
  const {data, save} = useAdmin('schedules');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      {data.map((sc, i) => (
        <Panel key={sc.id} title={sc.name} lede={`${sc.description} · last run ${ago(sc.lastRun)} (${sc.lastResult})${sc.nextRun ? ` · next ${sc.nextRun > Date.now() ? ago(sc.nextRun) : 'when the window opens'}` : ''}`} right={<Badge label={sc.enabled ? 'On' : 'Off'} tone={sc.enabled ? 'healthy' : 'neutral'} />}>
          <ToggleRow label="Enabled" help="Disabling stops new runs; a run already in progress finishes." value={sc.enabled} onChange={v => save(data.map(x => (x.id === sc.id ? {...x, enabled: v} : x)))} />
          <ListRow label="Window starts" help="Local time in America/Los_Angeles." value={sc.start} onPress={() => ui.menu({title: 'Window starts', actions: ['00:00', '01:00', '02:00', '03:00', '04:30', '05:30'].map(t => ({label: t, checked: sc.start === t, onPress: () => save(data.map(x => (x.id === sc.id ? {...x, start: t} : x)))}))})} hasTVPreferredFocus={i === 0} />
          <Stepper label="Window length" help="Minutes the task may run for." value={sc.lengthMin} onChange={v => save(data.map(x => (x.id === sc.id ? {...x, lengthMin: v} : x)))} min={15} max={480} step={15} unit="min" />
          <ToggleRow label="Catch up after a missed window" help="Run at the next opportunity if the server was off during the window." value={sc.catchUp} onChange={v => save(data.map(x => (x.id === sc.id ? {...x, catchUp: v} : x)))} />
        </Panel>
      ))}
    </View>
  );
}

export function MaintenanceSection() {
  const {data, save, run} = useAdmin('maintenance');
  if (!data) return <RowsSkeleton rows={1} />;
  const total = data.buckets.reduce((a, b) => a + b.sizeGB, 0);
  return (
    <View>
      {!data.permissionsOk ? (
        <Panel title="Other accounts on this computer can read Portico data" lede="Fixing permissions keeps Portico’s files readable only by your account.">
          <Button label="Fix permissions" icon="shield" variant="primary" size="sm" hasTVPreferredFocus style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => run('fixPermissions').then(() => ui.toast('Permissions fixed.'))} />
        </Panel>
      ) : null}
      <Panel title="Quiet hours for heavy work" lede="Scanning, analysis and picture generation are the jobs that load the server. Give them a window when nobody is watching.">
        <ToggleRow label="Only do heavy work inside these hours" help="Selected jobs wait for an enabled window. Work already running can finish." value={data.onlyInWindows} onChange={v => save({onlyInWindows: v})} />
        {data.quietHours.map(q => <ToggleRow key={q.id} label={`${q.name} · ${q.start} for ${plural(q.hours, 'hour', 'hours')}`} help={q.days.join(', ')} value={q.enabled} onChange={v => save({quietHours: data.quietHours.map(x => (x.id === q.id ? {...x, enabled: v} : x))})} />)}
        <Button label="Add window" icon="plus" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => save({quietHours: [...data.quietHours, {id: 'q' + Date.now(), name: 'Weekend mornings', days: ['Sat', 'Sun'], start: '06:00', hours: 3, enabled: true}]})} />
      </Panel>
      <Panel title="Background work" lede="Scanning, analysis and picture generation keep running. Lower priority leaves more capacity for browsing and playback.">
        <View style={{padding: u(10)}}><Segmented value={data.priority} onChange={v => save({priority: v})} options={[{value: 'lower', label: 'Lower (recommended)'}, {value: 'normal', label: 'Normal'}]} /></View>
      </Panel>
      <Panel title="Storage" lede={`What the server itself keeps on disk, apart from your media · ${total.toFixed(1)} GB`}>
        {data.buckets.map(b => <ListRow key={b.id} icon="hardDrive" label={b.name} help={`${b.description}${b.canRecreate ? ' Rebuilt automatically when needed.' : ''}`} value={`${b.sizeGB} GB`} onPress={() => ui.confirm({title: `Clean up ${b.name}?`, body: `${b.canRecreate ? 'These files are rebuilt when next needed, which costs the server some work.' : 'These files cannot be recreated.'} Type CLEAN to confirm.`, typeToConfirm: 'CLEAN', confirmLabel: 'Clean up', destructive: !b.canRecreate, onConfirm: () => save({buckets: data.buckets.map(x => (x.id === b.id ? {...x, sizeGB: 0} : x))})})} />)}
      </Panel>
      <Panel title="Deleted titles" lede="Files removed through Portico wait here before they are gone for good.">
        {data.trash.map(t => <ListRow key={t.id} icon="trash" label={t.title} help={`${plural(t.files, 'file', 'files')} · ${t.sizeGB} GB · deleted ${ago(t.deletedAt)} · kept until ${new Date(t.keptUntil).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}`} right={t.due ? <Badge label="Due for removal" tone="amber" /> : undefined} onPress={() => ui.menu({title: t.title, actions: [{label: 'Put back', icon: 'refresh', onPress: () => run('putBack', {id: t.id})}]})} />)}
        {!data.trash.length ? <Text variant="caption" tone="tertiary" style={{margin: u(14)}}>Nothing is waiting to be removed.</Text> : null}
        <View style={{flexDirection: 'row', gap: 8, margin: u(10)}}>
          <Button label="Remove what’s due" size="sm" onPress={() => run('removeDue')} />
          <Button label="Remove everything" size="sm" variant="danger" onPress={() => ui.confirm({title: 'Remove these files for good?', body: `This permanently deletes ${plural(data.trash.length, 'title', 'titles')} from disk. It cannot be undone. Type the number ${data.trash.length} to confirm.`, typeToConfirm: String(data.trash.length), confirmLabel: 'Remove for good', destructive: true, onConfirm: () => run('removeAll')})} />
        </View>
      </Panel>
      <Panel title="Updates" lede="Portico never installs an update by itself." right={<Badge label={data.update.available ? 'An update is available' : 'Up to date'} tone={data.update.available ? 'amber' : 'healthy'} />}>
        <KV k="This server" v={`${data.update.current} · ${data.update.channel} channel`} />
        <KV k="New version" v={data.update.latest} />
        <Text variant="captionStrong" tone="secondary" style={{margin: u(14), marginBottom: 4}}>Release notes</Text>
        {data.update.notes.map(n => <Text key={n} variant="caption" tone="tertiary" style={{marginHorizontal: u(14)}}>• {n}</Text>)}
      </Panel>
      <Panel title="Message everyone" lede="Appears in each person’s notifications. It is not an email.">
        <TextField label="Title" value={data.broadcast.title} onChangeText={v => save({broadcast: {...data.broadcast, title: v}})} placeholder="The server will be down on Saturday morning" style={{marginHorizontal: u(14)}} autoCapitalize="sentences" />
        <TextField label="Message" value={data.broadcast.message} onChangeText={v => save({broadcast: {...data.broadcast, message: v}})} multiline style={{marginHorizontal: u(14)}} autoCapitalize="sentences" />
        <View style={{padding: u(10), gap: 8}}>
          <Segmented value={data.broadcast.audience} onChange={v => save({broadcast: {...data.broadcast, audience: v}})} options={[{value: 'everyone', label: 'Everyone'}, {value: 'admins', label: 'Administrators only'}]} />
          <Segmented value={data.broadcast.importance} onChange={v => save({broadcast: {...data.broadcast, importance: v}})} options={[{value: 'info', label: 'For information'}, {value: 'notice', label: 'Worth noticing'}, {value: 'urgent', label: 'Urgent'}]} />
        </View>
        <Button label="Send" icon="message" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.toast('Message sent to everyone.')} />
      </Panel>
    </View>
  );
}

export function BackupsSection() {
  const {data, save, run} = useAdmin('backups');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      <Panel title="Back up now" lede="Large libraries can take a few minutes. The server keeps working.">
        <View style={{flexDirection: 'row', gap: 10, margin: u(10), flexWrap: 'wrap'}}>
          <Button label="Back up" icon="save" variant="primary" hasTVPreferredFocus onPress={() => { ui.toast('Backing up…'); run('backup').then(m => ui.toast(m)); }} />
          <Button label="Restore from a file or folder…" icon="folderOpen" onPress={() => ui.sheet({title: 'Choose a backup', subtitle: 'Pick a backup folder or a database file on this server.', render: close => <View><ListRow icon="folder" label="/var/lib/portico/backups/2026-08-01" onPress={() => { close(); ui.confirm({title: 'Restore from this file or folder?', body: 'The server restarts and playback stops. A copy of the current data is kept as a Before restore backup. People may need to sign in again.', confirmLabel: 'Restore backup', destructive: true, onConfirm: () => ui.toast('Restarting… The server is starting again with the restored data.')}); }} hasTVPreferredFocus /><ListRow icon="database" label="/mnt/usb/portico.db" onPress={close} /></View>})} />
        </View>
        <KV k="Schedule" v={data.schedule} />
        <KV k="Last result" v={data.lastResult} tone="healthy" />
        <Stepper label="Backups to keep" value={data.keep} onChange={v => save({keep: v})} min={1} max={30} />
        <Text variant="caption" tone="tertiary" style={{margin: u(14)}}>Backups are plain SQLite files you can copy or open with any SQLite tool. Media files are never included.</Text>
      </Panel>
      <Panel title="Backups">
        {data.list.map(b => (
          <ListRow key={b.id} icon="archive" label={new Date(b.at).toLocaleString('en-US', {month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'})} help={`${b.kind} · ${b.sizeMB} MB · Schema ${b.schema} · Server ${b.version}`}
            onPress={() => ui.menu({title: b.id, actions: [
              {label: 'Restore this backup', icon: 'refresh', onPress: () => ui.confirm({title: 'Restore this backup?', body: 'The server restarts and playback stops. A copy of the current data is kept as a Before restore backup. People may need to sign in again.', confirmLabel: 'Restore backup', destructive: true, onConfirm: () => ui.toast('Restored')})},
              {label: 'Delete', icon: 'trash', destructive: true, onPress: () => ui.confirm({title: 'Delete this backup?', body: `The backup file is removed from this server. This cannot be undone. Type ${b.id} to confirm.`, typeToConfirm: b.id, confirmLabel: 'Delete backup', destructive: true, onConfirm: () => run('deleteBackup', {id: b.id}).then(() => ui.toast('Backup deleted.'))})},
            ]})} />
        ))}
      </Panel>
    </View>
  );
}

export function AlertsSection() {
  const {data, save} = useAdmin('alerts');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <Panel title="Alerts" lede="When the server puts a warning in your notifications.">
      <Stepper label="Warn when free space falls below" help="Percent of a drive that is still free" value={data.storageWarning} onChange={v => save({storageWarning: v})} min={1} max={50} unit="%" />
      <Stepper label="Warn urgently below" help="Recordings and conversions may start failing at this point" value={data.storageCritical} onChange={v => save({storageCritical: v})} min={1} max={25} unit="%" />
      <Stepper label="Warn before a certificate ends" help="Days ahead. Certificates renew by themselves; this only fires if renewal is stuck." value={data.certificateDays} onChange={v => save({certificateDays: v})} min={1} max={60} unit="days" />
      <Stepper label="Warn when the server has been unreachable for" value={data.offlineMinutes} onChange={v => save({offlineMinutes: v})} min={5} max={240} step={5} unit="min" />
      <ToggleRow label="Warn when a scan fails" value={data.failedScans} onChange={v => save({failedScans: v})} />
    </Panel>
  );
}

export function GeneralSection() {
  const {data, save} = useAdmin('general');
  const [custody, setCustody] = React.useState(false);
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      {data.connection !== 'notConnected' && !custody ? <Banner icon="shieldCheck" tone="info" title="Accept ownership of this server’s Portico registration" body={`You’re the owner of ${data.name} now. Accept so your Portico Account holds its registration, which covers remote access and invitations.`} action={{label: 'Accept', onPress: () => { setCustody(true); ui.toast(`You now hold ${data.name}’s Portico registration.`); }}} /> : null}
      <Panel title="Identity">
        <TextField label="Server name" value={data.name} onChangeText={v => save({name: v})} help="Shown on every device that connects and in your Portico Account." style={{marginHorizontal: u(14)}} autoCapitalize="words" />
        <KV k="Version" v={`Portico Server ${data.version}`} />
        <KV k="Server ID" v={data.serverId} />
        <KV k="Platform" v={data.platform} />
        <KV k="Data folder" v={data.dataFolder} />
      </Panel>
      <Panel title="Portico Account connection" lede="Connecting the server to a Portico Account enables remote access, invitations and switching between servers." right={<Badge label={data.connection === 'connected' ? 'Connected' : data.connection === 'pending' ? 'Approval pending' : 'Not connected'} tone={data.connection === 'connected' ? 'healthy' : 'amber'} />}>
        <KV k="Status" v={data.connection === 'connected' ? 'Signed by your Portico Account.' : 'Approval is waiting on your Portico Account.'} />
        <KV k="Owner account" v={data.ownerAccount} />
        <View style={{flexDirection: 'row', gap: 8, margin: u(10)}}>
          <Button label="Check status" size="sm" hasTVPreferredFocus onPress={() => ui.toast('Connected. Registration is current.')} />
          <Button label="Connect again" size="sm" onPress={() => go('Claim')} />
        </View>
      </Panel>
      <Panel title="Recovery owner" lede="A local owner account remains available for recovery even when the server is connected.">
        <KV k="Account" v={data.recoveryOwner} />
      </Panel>
      <Meter value={0} style={{height: 0}} color={colors.accent} />
    </View>
  );
}
