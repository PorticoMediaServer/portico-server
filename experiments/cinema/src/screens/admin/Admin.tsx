import React, {useState} from 'react';
import {ScrollView, StyleSheet, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import type {FeedbackRow, LogLine} from '../../data/admin';
import {Button} from '../../design/Button';
import {Badge, Chip, ListRow, Meter, Panel, Segmented, Stepper, TextField, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon, type IconName} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {EmptyState, RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {go, goSection, type Routes} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {ago, Bars, KV, useAdmin} from './common';
import {ChannelBuilderSection, ChannelsSection, DVRSection, LibrariesSection, LibrarySettingsSection, LiveTVSection, MetadataSection, StorageSection} from './AdminMedia';
import {AccessSection, AlertsSection, BackupsSection, GeneralSection, MaintenanceSection, NetworkSection, PeopleSection, SchedulesSection, TranscodingSection} from './AdminServer';

type Sec = {id: string; label: string; icon: IconName; group: 'Console' | 'Media' | 'Server'; lede: string; C: React.ComponentType};
export const SECTIONS: Sec[] = [
  {id: 'overview', label: 'Overview', icon: 'pulse', group: 'Console', lede: 'Health, attention items and who is watching right now.', C: Overview},
  {id: 'now-playing', label: 'Now Playing', icon: 'play', group: 'Console', lede: 'Every playback session on this server, and how it’s delivered.', C: NowPlaying},
  {id: 'activity', label: 'Activity', icon: 'activity', group: 'Console', lede: 'Playback sessions, scans and background tasks.', C: Activity},
  {id: 'logs', label: 'Logs', icon: 'terminal', group: 'Console', lede: 'What the server has been saying. Nothing here includes file paths, passwords or tokens.', C: Logs},
  {id: 'diagnostics', label: 'Diagnostics', icon: 'wrench', group: 'Console', lede: 'What this server can do right now, and why not when it can’t.', C: Diagnostics},
  {id: 'feedback', label: 'Viewer feedback', icon: 'message', group: 'Console', lede: 'Problems viewers reported from their apps.', C: Feedback},
  {id: 'libraries', label: 'Libraries', icon: 'library', group: 'Media', lede: 'What the server knows about and where it comes from.', C: LibrariesSection},
  {id: 'library-settings', label: 'Library settings', icon: 'settings', group: 'Media', lede: 'Matching, pictures, deleting and scanning for each library.', C: LibrarySettingsSection},
  {id: 'metadata', label: 'Metadata', icon: 'globe', group: 'Media', lede: 'Where titles, artwork, cast and lyrics come from.', C: MetadataSection},
  {id: 'storage', label: 'Storage', icon: 'hardDrive', group: 'Media', lede: 'Remote storage attached to this server.', C: StorageSection},
  {id: 'live-tv', label: 'Live TV', icon: 'live', group: 'Media', lede: 'Tuners, playlists and recording space.', C: LiveTVSection},
  {id: 'dvr', label: 'DVR', icon: 'dvr', group: 'Media', lede: 'Recording defaults, tuners and who can record.', C: DVRSection},
  {id: 'channels', label: 'Library Channels', icon: 'tv', group: 'Media', lede: 'Always-on channels generated from your libraries.', C: ChannelsSection},
  {id: 'channel-builder', label: 'Channel Builder', icon: 'wand', group: 'Media', lede: 'Design a channel: what plays, in what order.', C: ChannelBuilderSection},
  {id: 'transcoding', label: 'Transcoding', icon: 'cpu', group: 'Server', lede: 'Whether the server may convert media, and how much at once.', C: TranscodingSection},
  {id: 'network', label: 'Network', icon: 'network', group: 'Server', lede: 'How devices reach this server at home and away.', C: NetworkSection},
  {id: 'people', label: 'People', icon: 'users', group: 'Server', lede: 'Who can use this server and what they may see.', C: PeopleSection},
  {id: 'access', label: 'Access & API keys', icon: 'shield', group: 'Server', lede: 'Roles, limits, invitations, device approval and API keys.', C: AccessSection},
  {id: 'schedules', label: 'Schedules', icon: 'calendarClock', group: 'Server', lede: 'When heavy maintenance is allowed to run.', C: SchedulesSection},
  {id: 'maintenance', label: 'Maintenance', icon: 'wrench', group: 'Server', lede: 'Quiet hours, storage, deleted titles and updates.', C: MaintenanceSection},
  {id: 'backups', label: 'Backups', icon: 'archive', group: 'Server', lede: 'The server’s database and settings, kept as plain files you can copy.', C: BackupsSection},
  {id: 'alerts', label: 'Alerts', icon: 'bell', group: 'Server', lede: 'When the server puts a warning in your notifications.', C: AlertsSection},
  {id: 'general', label: 'General', icon: 'server', group: 'Server', lede: 'Server name, Portico Account connection and version.', C: GeneralSection},
];

/** Server administration: a console with its own section list beside the content. */
export function AdminScreen() {
  const route = useRoute<RouteProp<Routes, 'Admin'>>();
  const session = useSession();
  const layout = useLayout();
  const [id, setId] = useState(route.params?.section ?? 'overview');
  React.useEffect(() => {
    if (route.params?.section) setId(route.params.section);
  }, [route.params?.section]);
  if (!isManager(session.role)) return <AdminDeniedScreen />;
  const sec = SECTIONS.find(x => x.id === id) ?? SECTIONS[0];
  const C = sec.C;
  const side = !layout.compact;
  const nav = (
    <View style={side ? styles.side : undefined}>
      {side ? (
        <ScrollView contentContainerStyle={{paddingBottom: 40}} showsVerticalScrollIndicator={false}>
          {(['Console', 'Media', 'Server'] as const).map(g => (
            <View key={g} style={{marginBottom: u(10)}}>
              <Text variant="eyebrow" tone="tertiary" style={{marginLeft: u(12), marginBottom: u(4)}}>{g}</Text>
              {SECTIONS.filter(x => x.group === g).map(x => (
                <Focusable key={x.id} onPress={() => setId(x.id)} scale={1.03} accessibilityLabel={x.label} accessibilityState={{selected: x.id === id}} hasTVPreferredFocus={x.id === id}>
                  {({focused}) => {
                    const lit = focused && isTV;
                    return (
                      <View style={[styles.navItem, x.id === id && !lit && {backgroundColor: 'rgba(92,179,226,0.14)'}, lit && {backgroundColor: colors.white}]}>
                        <Icon name={x.icon} size={16} color={lit ? colors.actionInk : x.id === id ? colors.accentStrong : colors.textSecondary} />
                        <Text variant="captionStrong" color={lit ? colors.actionInk : x.id === id ? colors.text : colors.textSecondary} style={{marginLeft: u(10)}} numberOfLines={1}>{x.label}</Text>
                      </View>
                    );
                  }}
                </Focusable>
              ))}
            </View>
          ))}
        </ScrollView>
      ) : (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: 8, paddingBottom: 10}}>
          {SECTIONS.map(x => <Chip key={x.id} label={x.label} icon={x.icon} active={x.id === id} onPress={() => setId(x.id)} />)}
        </ScrollView>
      )}
    </View>
  );
  return (
    <Screen title="Manage server" scroll={false}>
      <Text variant="eyebrow" tone="accent">Server administration · {session.server?.name}</Text>
      <View style={{flex: 1, flexDirection: side ? 'row' : 'column', marginTop: u(8)}}>
        {nav}
        <ScrollView style={{flex: 1}} contentContainerStyle={{paddingBottom: 140, paddingLeft: side ? u(20) : 0}} showsVerticalScrollIndicator={false} key={sec.id}>
          <Text variant="display">{sec.label}</Text>
          <Text variant="body" tone="secondary" style={{marginBottom: u(14)}}>{sec.lede}</Text>
          <C />
        </ScrollView>
      </View>
    </Screen>
  );
}

export function AdminDeniedScreen() {
  return (
    <Screen>
      <EmptyState icon="lock" title="Only the server owner can manage this server" body="Ask the owner to share what you need, or sign in with the owner account." action={{label: 'Go home', icon: 'home', onPress: () => goSection('Home')}} />
    </Screen>
  );
}

// ---------------------------------------------------------------- Console sections

function Overview() {
  const {data, run} = useAdmin('overview');
  const np = useAdmin('nowPlaying');
  if (!data) return <RowsSkeleton rows={2} />;
  const t = data.telemetry;
  const open = data.attention.filter(a => !a.acknowledged);
  return (
    <View>
      <View style={styles.health}>
        {data.health.map(h => (
          <View key={h.label} style={[styles.healthCard, {borderColor: h.state === 'ok' ? 'rgba(134,207,179,0.4)' : h.state === 'warn' ? 'rgba(232,188,106,0.5)' : colors.danger}]}>
            <View style={{flexDirection: 'row', alignItems: 'center', gap: 6}}>
              <View style={{width: 8, height: 8, borderRadius: 4, backgroundColor: h.state === 'ok' ? colors.healthy : h.state === 'warn' ? colors.amber : colors.danger}} />
              <Text variant="captionStrong">{h.label}</Text>
            </View>
            <Text variant="caption" tone="tertiary" numberOfLines={2}>{h.detail}</Text>
          </View>
        ))}
      </View>
      <Panel title="Needs your attention" lede={open.length ? undefined : 'Nothing needs your attention right now.'}>
        {open.map((a, i) => (
          <ListRow key={a.id} icon={a.severity === 'urgent' ? 'alert' : a.severity === 'warn' ? 'alertCircle' : 'info'} label={a.title} help={a.body} hasTVPreferredFocus={i === 0}
            onPress={() => ui.menu({title: a.title, actions: [{label: 'Open', icon: 'arrowRight', onPress: () => go('Admin', {section: a.section})}, {label: 'Acknowledge', icon: 'check', onPress: () => run('acknowledge', {id: a.id})}]})} right={<Badge label={a.severity} tone={a.severity === 'urgent' ? 'danger' : a.severity === 'warn' ? 'amber' : 'accent'} />} />
        ))}
      </Panel>
      <Panel title={`Watching now · ${np.data?.length ?? 0}`} right={<Button label="Now Playing" size="sm" iconRight="chevronRight" onPress={() => go('Admin', {section: 'now-playing'})} />}>
        {(np.data ?? []).map(p => <ListRow key={p.id} icon={p.state === 'paused' ? 'pause' : 'play'} label={`${p.title}`} help={`${p.profile} on ${p.device} · ${p.delivery} · ${p.location}`} compact />)}
      </Panel>
      <Panel title="Load" lede="Last 4 hours" right={<Segmented value="1h" onChange={() => {}} options={[{value: '10m', label: '10 min'}, {value: '1h', label: '1 hour'}, {value: '24h', label: '24 hours'}]} />}>
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: u(20)}}>
          <Bars label="Processor" values={t.cpu} />
          <Bars label="Memory" values={t.memory} color={colors.healthy} />
          <Bars label="Sent to viewers" values={t.netOut} color={colors.amber} unit=" Mb/s" />
          <Bars label="Video encoder" values={t.gpu} color="#8a5fd0" />
        </View>
      </Panel>
      <Panel title="Viewing" lede="Hours watched per day, last 30 days">
        <Bars label="Viewing" values={t.viewingHours} unit=" h" height={80} />
      </Panel>
      <Panel title="Storage">
        {data.storage.map(d => (
          <View key={d.label} style={{paddingHorizontal: u(14), marginBottom: u(10)}}>
            <View style={{flexDirection: 'row', justifyContent: 'space-between'}}><Text variant="captionStrong">{d.label}</Text><Text variant="caption" tone="tertiary">{d.usedTB} of {d.totalTB} TB</Text></View>
            <Meter value={d.usedTB / d.totalTB} color={d.usedTB / d.totalTB > 0.9 ? colors.amber : colors.accentStrong} style={{marginTop: 4}} />
          </View>
        ))}
      </Panel>
      <Panel title="Recent playback">
        {data.recent.map(r => <ListRow key={r.title + r.at} icon="history" label={r.title} help={`${r.profile} · ${r.device} · ${ago(r.at)}`} compact />)}
      </Panel>
    </View>
  );
}

function NowPlaying() {
  const {data, run} = useAdmin('nowPlaying');
  if (!data) return <RowsSkeleton rows={1} />;
  if (!data.length) return <EmptyState icon="play" title="Nobody is watching right now" />;
  return (
    <View>
      {data.map((p, i) => (
        <Panel key={p.id} title={p.title} lede={p.subtitle} right={<Badge label={p.state} tone={p.state === 'playing' ? 'healthy' : 'neutral'} />}>
          <Meter value={p.progress} style={{marginHorizontal: u(14), marginBottom: u(8)}} />
          <KV k="Viewer" v={`${p.profile} (${p.account})`} />
          <KV k="Device" v={`${p.device} · ${p.platform}`} />
          <KV k="Delivery" v={`${p.delivery} · ${p.quality}${p.hardware ? ` · ${p.hardware}` : ''}`} tone={p.delivery.startsWith('Converting') ? 'amber' : 'healthy'} />
          <KV k="Network" v={`${p.location} · ${p.rateMbps} Mb/s`} />
          <KV k="Started" v={ago(p.startedAt)} />
          <View style={{flexDirection: 'row', gap: 10, margin: u(10)}}>
            <Button label="Open" size="sm" icon="arrowRight" onPress={() => go('Movie', {id: p.itemId})} hasTVPreferredFocus={i === 0} />
            <Button label="Stop" size="sm" icon="stop" variant="danger" onPress={() => ui.prompt({title: `Stop playback of “${p.title}”?`, body: `Playback stops on ${p.profile}’s device, and they see that the server owner stopped it.`, fields: [{key: 'm', label: 'Message to the viewer', placeholder: 'For example: The server is restarting in five minutes.', help: 'Shown with the notice. Up to 500 characters.', multiline: true}], submitLabel: 'Stop playback', onSubmit: () => { run('stopSession', {id: p.id}).then(() => ui.toast('Playback stopped.')); return null; }})} />
          </View>
        </Panel>
      ))}
    </View>
  );
}

function Activity() {
  const {data, run} = useAdmin('activity');
  const np = useAdmin('nowPlaying');
  if (!data) return <RowsSkeleton rows={1} />;
  const color = {running: 'accent', queued: 'neutral', done: 'healthy', failed: 'danger', cancelled: 'neutral'} as const;
  return (
    <View>
      <Panel title="Playback · now">
        {(np.data ?? []).map(p => <ListRow key={p.id} icon="play" label={p.title} help={`${p.profile} · ${p.device} · ${p.delivery}`} compact />)}
      </Panel>
      <Panel title="Background tasks">
        {data.tasks.map((t, i) => (
          <View key={t.id} style={{paddingHorizontal: u(14), paddingVertical: u(8)}}>
            <View style={{flexDirection: 'row', alignItems: 'center', gap: 8}}>
              <Text variant="bodyStrong" style={{flex: 1}}>{t.name}</Text>
              <Badge label={t.state} tone={color[t.state]} />
              {t.state === 'running' ? <Button label="Cancel" size="sm" variant="ghost" hasTVPreferredFocus={i === 0} onPress={() => run('cancelTask', {id: t.id}).then(() => ui.toast('Task canceled.'))} /> : null}
            </View>
            <Text variant="caption" tone="tertiary">{t.detail} · started {ago(t.startedAt)}</Text>
            {t.state === 'running' ? <Meter value={t.progress} height={4} style={{marginTop: 6}} /> : null}
          </View>
        ))}
      </Panel>
    </View>
  );
}

function Logs() {
  const {data, save} = useAdmin('logs');
  const [level, setLevel] = useState<'debug' | 'info' | 'warn' | 'error'>('info');
  if (!data) return <RowsSkeleton rows={1} />;
  const order = ['debug', 'info', 'warn', 'error'];
  const lines = data.lines.filter(l => order.indexOf(l.level) >= order.indexOf(level));
  const tone = (l: LogLine) => (l.level === 'error' ? colors.danger : l.level === 'warn' ? colors.amber : l.level === 'debug' ? colors.textTertiary : colors.textSecondary);
  return (
    <View>
      <Panel title="Messages" right={<Badge label={data.following ? 'Following' : 'Paused'} tone={data.following ? 'healthy' : 'neutral'} />}>
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, paddingHorizontal: u(10), marginBottom: 8}}>
          {(['debug', 'info', 'warn', 'error'] as const).map((l, i) => <Chip key={l} label={l === 'debug' ? 'Everything' : `${l[0].toUpperCase() + l.slice(1)} and above`} active={level === l} onPress={() => setLevel(l)} />)}
          <Button label={data.following ? 'Pause' : 'Follow'} size="sm" onPress={() => save({following: !data.following})} />
        </View>
        <View style={{backgroundColor: 'rgba(4,10,16,0.7)', borderRadius: radius.md, padding: u(10), marginHorizontal: u(6)}}>
          {lines.slice(0, 40).map(l => (
            <Text key={l.id} variant="mono" color={tone(l)} numberOfLines={1}>{new Date(l.at).toLocaleTimeString('en-US', {hour12: false})}  {l.level.toUpperCase().padEnd(5)} {l.component.padEnd(10)} {l.message}</Text>
          ))}
          {!lines.length ? <Text variant="caption" tone="tertiary">No messages match.</Text> : null}
        </View>
      </Panel>
      <Panel title="Temporary capture" lede="Collect verbose records for one component for a short time. It switches itself off.">
        <ListRow label="Component" value={data.capture.component} onPress={() => ui.menu({title: 'Component', actions: ['playback', 'transcoder', 'scanner', 'livetv', 'auth', 'network'].map(c => ({label: c, checked: data.capture.component === c, onPress: () => save({capture: {...data.capture, component: c}})}))})} />
        <Stepper label="Duration" value={data.capture.minutes} onChange={v => save({capture: {...data.capture, minutes: v}})} min={5} max={60} step={5} unit="min" />
        <Button label={data.capture.until ? 'Stop now' : 'Start capture'} size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => save({capture: {...data.capture, until: data.capture.until ? null : Date.now() + data.capture.minutes * 60000}})} />
      </Panel>
      <Panel title="Support export" lede="A redacted bundle you can inspect before sending. Media, credentials and raw databases are never included.">
        <ListRow label="Time window" value={data.export.window} onPress={() => ui.menu({title: 'Time window', actions: ['1 hour', '24 hours', '7 days'].map(w => ({label: w, onPress: () => save({export: {...data.export, window: w}})}))})} />
        <ToggleRow label="Server records" value={data.export.server} onChange={v => save({export: {...data.export, server: v}})} />
        <ToggleRow label="Client uploads" value={data.export.client} onChange={v => save({export: {...data.export, client: v}})} />
        <ToggleRow label="Audit" value={data.export.audit} onChange={v => save({export: {...data.export, audit: v}})} />
        <ToggleRow label="Health snapshot" value={data.export.snapshot} onChange={v => save({export: {...data.export, snapshot: v}})} />
        <View style={{flexDirection: 'row', gap: 10, margin: u(10)}}>
          <Button label="Prepare export" size="sm" onPress={() => save({export: {...data.export, ready: true}}).then(() => ui.toast('Ready to download.'))} />
          {data.export.ready ? <Button label="Download JSON" icon="download" size="sm" variant="primary" onPress={() => ui.toast('Saved support-export.json')} /> : null}
        </View>
      </Panel>
      <Panel title="Retention" lede="How long records are kept. Shortening a period prunes soon after saving.">
        <Stepper label="Records" help="Diagnostic and log history." value={data.retention.records} onChange={v => save({retention: {...data.retention, records: v}})} min={1} max={365} unit="days" />
        <Stepper label="Notifications" help="Owner alerts and viewer notices." value={data.retention.notifications} onChange={v => save({retention: {...data.retention, notifications: v}})} min={1} max={365} unit="days" />
        <Stepper label="Task history" help="Finished scans, refreshes and maintenance runs." value={data.retention.tasks} onChange={v => save({retention: {...data.retention, tasks: v}})} min={1} max={365} unit="days" />
      </Panel>
    </View>
  );
}

function Diagnostics() {
  const {data} = useAdmin('diagnostics');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      <Panel title="Server capabilities" lede="What this server can do right now, and why not when it can’t.">
        {data.capabilities.map((c, i) => (
          <ListRow key={c.id} label={c.label} help={c.reason} hasTVPreferredFocus={i === 0} right={<Badge label={c.state === 'available' ? 'Available' : c.state === 'unavailable' ? 'Unavailable' : 'Not checked'} tone={c.state === 'available' ? 'healthy' : c.state === 'unavailable' ? 'danger' : 'neutral'} />}
            onPress={c.technical ? () => ui.sheet({title: 'Technical details', subtitle: c.label, render: () => <Text variant="mono">{c.technical}</Text>}) : undefined} />
        ))}
      </Panel>
      <Panel title="Checks">
        {data.checks.map(c => <ListRow key={c.label} icon={c.state === 'ok' ? 'checkCircle' : 'alertCircle'} label={c.label} help={c.detail} />)}
        <Button label="Run checks again" icon="refresh" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.toast('All checks finished.')} />
      </Panel>
    </View>
  );
}

function Feedback() {
  const {data, save} = useAdmin('feedback');
  if (!data) return <RowsSkeleton rows={1} />;
  if (!data.length) return <EmptyState icon="message" title="No reports" body="When a viewer reports a problem from their app it appears here." />;
  const statusLabel = {new: 'New', looking: 'Looking into it', fixed: 'Fixed', wontFix: 'Won’t fix'};
  const open = (r: FeedbackRow) => ui.sheet({title: r.category, subtitle: `Reported ${ago(r.at)} · ${statusLabel[r.status]}`, render: close => (
    <View>
      <Text variant="bodyStrong">{r.from}{r.title ? ` · about “${r.title}”` : ''}</Text>
      <Text variant="body" tone="secondary" style={{marginVertical: 10}}>{r.message}</Text>
      <Text variant="captionStrong" tone="secondary">Status</Text>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginVertical: 8}}>
        {(['new', 'looking', 'fixed', 'wontFix'] as const).map(st => <Chip key={st} label={statusLabel[st]} active={r.status === st} onPress={() => save(data.map(x => (x.id === r.id ? {...x, status: st} : x)))} />)}
      </View>
      <Button label="Reply to the viewer…" icon="message" onPress={() => ui.prompt({title: 'Reply to the viewer', body: 'The viewer sees this in their notifications.', fields: [{key: 'r', label: 'Reply', multiline: true, initial: r.reply}], submitLabel: 'Update report', onSubmit: v => { save(data.map(x => (x.id === r.id ? {...x, reply: v.r} : x))); ui.toast('Report updated.'); close(); return null; }})} />
    </View>
  )});
  return (
    <Panel>
      {data.map((r, i) => <ListRow key={r.id} icon="message" label={r.message} help={`${r.category} · ${r.from} · ${ago(r.at)}${r.title ? ' · about a title' : ''}`} right={<Badge label={statusLabel[r.status]} tone={r.status === 'new' ? 'accent' : r.status === 'fixed' ? 'healthy' : 'neutral'} />} onPress={() => open(r)} hasTVPreferredFocus={i === 0} />)}
    </Panel>
  );
}

const styles = StyleSheet.create({
  side: {width: isTV ? 340 : 230},
  navItem: {flexDirection: 'row', alignItems: 'center', paddingHorizontal: u(12), height: u(38), borderRadius: radius.md},
  health: {flexDirection: 'row', flexWrap: 'wrap', gap: u(10), marginBottom: u(16)},
  healthCard: {width: isTV ? 360 : 160, padding: u(12), borderRadius: radius.md, borderWidth: 1, backgroundColor: 'rgba(16,33,45,0.7)'},
});

export {TextField};
