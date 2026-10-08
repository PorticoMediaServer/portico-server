import React, {useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import type {Recording, SeriesRule} from '../../data/types';
import {Button} from '../../ui/Button';
import {Badge, EmptyState, ListRow, Loading, Notice, ProgressBar, Segmented, Stepper, Surface, Tabs, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {ActionMenu, ConfirmDialog, Sheet} from '../../ui/Sheet';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useToast} from '../../app/toast';
import {root} from '../../nav/nav';
import {colors} from '../../theme/tokens';

const when = (r: Recording) => `${new Date(r.start).toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric'})} · ${new Date(r.start).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})} – ${new Date(r.end).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}`;
const stateBadge = (r: Recording) => ({scheduled: {label: 'Scheduled', tone: 'accent' as const}, recording: {label: 'Recording', tone: 'record' as const}, conflicted: {label: 'Tuner conflict', tone: 'warn' as const}, completed: {label: 'Recorded', tone: 'good' as const}, incomplete: {label: 'Incomplete · playable', tone: 'warn' as const}, failed: {label: 'Failed', tone: 'bad' as const}, cancelled: {label: 'Canceled', tone: 'neutral' as const}})[r.state];

/** DVR: upcoming, recorded, series rules, history and storage, with conflict resolution. */
export function Recordings() {
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const [tab, setTab] = useState<'upcoming' | 'recorded' | 'rules' | 'history' | 'storage'>('upcoming');
  const [menu, setMenu] = useState<Recording>();
  const [confirm, setConfirm] = useState<{kind: 'cancel' | 'delete'; r: Recording}>();
  const [rule, setRule] = useState<SeriesRule>();
  const [deleteRule, setDeleteRule] = useState<SeriesRule>();
  const [conflict, setConflict] = useState(false);
  const {busy, run} = useAction();
  const q = useQuery(c => c.live.recordings(), [], ['dvr', 'live']);
  if (!q.data) return <Loading label="Loading recordings" />;
  const d = q.data;
  const play = (r: Recording) => void actions.play({id: r.id, kind: 'recording', title: r.title, subtitle: r.channelName, image: r.image, shape: 'wide', duration: (r.end - r.start) / 1000, caption: r.episodeTitle});
  const row = (r: Recording) => {
    const b = stateBadge(r);
    return (
      <ListRow key={r.id} left={<Artwork source={r.image} title={r.title} width={m.isTV ? 160 : 96} height={m.isTV ? 90 : 54} radius={6} />} title={r.episodeTitle ? `${r.title} · ${r.episodeTitle}` : r.title} subtitle={`${r.channelName} · ${when(r)}${r.size ? ` · ${r.size}` : ''}${r.reason ? `\n${r.reason}` : ''}`}
        trailing={<View style={{alignItems: 'flex-end', gap: 4}}><Badge label={b.label} tone={b.tone} />{r.kept ? <Badge label="Kept" /> : null}{r.watched ? <Text variant="micro" tone="tertiary">Watched</Text> : null}</View>}
        onPress={() => (r.state === 'completed' || r.state === 'incomplete' ? play(r) : setMenu(r))} onLongPress={() => setMenu(r)} chevron={false} />
    );
  };
  return (
    <View style={{paddingHorizontal: m.gutter, gap: m.space.md}}>
      <Tabs tabs={[{value: 'upcoming', label: 'Upcoming', count: d.upcoming.length}, {value: 'recorded', label: 'Recorded', count: d.recorded.length}, {value: 'rules', label: 'Series rules', count: d.rules.length}, {value: 'history', label: 'History'}, {value: 'storage', label: 'Storage'}]} value={tab} onChange={setTab} />
      {d.conflicts.length && tab === 'upcoming' ? <Notice tone="warn" title="Conflicts" body="These overlap with other recordings and may not all be captured. Cancel one to make room." action={{label: 'Resolve conflict…', onPress: () => setConflict(true)}} /> : null}
      {tab === 'upcoming' ? (d.upcoming.length ? <Surface padded={false} style={{padding: 4}}>{d.upcoming.map(row)}</Surface> : <EmptyState icon="recordings" title="Nothing scheduled" body="Open a program in the guide and choose Record." action={{label: 'Open the guide', onPress: () => root('Live', {view: 'guide'})}} />) : null}
      {tab === 'recorded' ? (d.recorded.length ? <Surface padded={false} style={{padding: 4}}>{d.recorded.map(row)}</Surface> : <EmptyState icon="recordings" title="No recordings yet" body="Recordings appear here once they finish." />) : null}
      {tab === 'history' ? <Surface padded={false} style={{padding: 4}}>{d.history.map(row)}</Surface> : null}
      {tab === 'rules' ? (d.rules.length ? <Surface padded={false} style={{padding: 4}}>
        {d.rules.map(r => <ListRow key={r.id} left={<Artwork source={r.image} title={r.title} width={m.isTV ? 160 : 96} height={m.isTV ? 90 : 54} radius={6} />} title={r.title} subtitle={`${r.channel === 'this' ? r.channelName : 'Any channel'} · ${r.newOnly ? 'New episodes only' : 'All episodes'} · ${r.keep === 'all' ? 'Keep every episode' : `Keep the latest ${r.keep}`}\n${r.upcoming} upcoming · ${r.recorded} recorded`} trailing={<Badge label={r.paused ? 'Paused' : 'On'} tone={r.paused ? 'neutral' : 'good'} />} onPress={() => setRule(r)} />)}
      </Surface> : <EmptyState icon="recordings" title="No series rules" body="Choose Record series on a program in the guide to keep recording new episodes." />) : null}
      {tab === 'storage' ? (
        <Surface style={{gap: m.space.md}}>
          <Text variant="subheading">Recording space</Text>
          <ProgressBar value={d.storage.usedGB / d.storage.totalGB} height={m.isTV ? 14 : 10} />
          <Text variant="body">{d.storage.usedGB} GB used · {d.storage.freeGB} GB free</Text>
          <Text variant="caption" tone="secondary">About {d.storage.hoursLeft} hours of recording space left · Recordings are kept for {d.storage.retentionDays} days · {d.storage.pendingDeleteGB} GB waiting to be deleted</Text>
          {d.storage.writable ? <Badge label="The recording space is healthy" tone="good" /> : <Notice tone="error" body="The recording space can’t be written to" />}
        </Surface>
      ) : null}
      <ActionMenu visible={!!menu} onClose={() => setMenu(undefined)} title={menu?.title} subtitle={menu ? `${menu.channelName} · ${when(menu)}` : undefined} image={menu?.image} items={[
        {id: 'play', label: menu?.state === 'recording' ? 'Watch from the start' : 'Play', icon: 'play', hidden: !(menu && ['completed', 'incomplete', 'recording'].includes(menu.state))},
        {id: 'cancel', label: 'Cancel recording', icon: 'close', hidden: !(menu && ['scheduled', 'recording', 'conflicted'].includes(menu.state)), destructive: true},
        {id: 'conflict', label: 'Resolve conflict…', icon: 'alert', hidden: menu?.state !== 'conflicted'},
        {id: 'keep', label: menu?.kept ? 'Stop keeping' : 'Keep until I delete it', icon: 'lock', hidden: !(menu && ['completed', 'incomplete'].includes(menu.state))},
        {id: 'delete', label: 'Delete recording', icon: 'trash', destructive: true, hidden: !(menu && ['completed', 'incomplete', 'failed'].includes(menu.state))},
      ]} onSelect={async id => {
        const r = menu!;
        setMenu(undefined);
        if (id === 'play') play(r);
        if (id === 'cancel') setConfirm({kind: 'cancel', r});
        if (id === 'delete') setConfirm({kind: 'delete', r});
        if (id === 'conflict') setConflict(true);
        if (id === 'keep') { await client.live.keepRecording(r.id, !r.kept); toast({message: r.kept ? 'No longer kept.' : 'Kept until you delete it.'}); }
      }} />
      <ConfirmDialog visible={!!confirm} onClose={() => setConfirm(undefined)} title={confirm?.kind === 'cancel' ? `Cancel recording ${confirm?.r.title}?` : `Delete ${confirm?.r.title}?`}
        body={confirm?.kind === 'cancel' ? (confirm.r.state === 'recording' ? 'What has been recorded so far is kept. A series rule, if there is one, stays in place.' : 'It won’t be recorded. A series rule, if there is one, stays in place.') : confirm?.r.kept ? `You chose to keep this recording. Deleting it frees ${confirm?.r.size} and can’t be undone.` : `The recording (${confirm?.r.size ?? 'its file'}) is deleted from the server. This can’t be undone.`}
        confirmLabel={confirm?.kind === 'cancel' ? 'Cancel recording' : 'Delete recording'} destructive busy={busy}
        onConfirm={() => void run(async () => { if (confirm!.kind === 'cancel') await client.live.cancelRecording(confirm!.r.id); else await client.live.deleteRecording(confirm!.r.id); setConfirm(undefined); })} />
      <Sheet visible={conflict} onClose={() => setConflict(false)} title={`${d.conflicts[0]?.title ?? 'This recording'} can’t record`} subtitle="You have 4 tuners, and these recordings need them at the same time." size="md">
        {d.conflicts.map(r => (
          <View key={r.id} style={{flexDirection: 'row', alignItems: 'center', gap: m.space.md, padding: 8, borderRadius: 12, backgroundColor: colors.raised}}>
            <Artwork source={r.image} title={r.title} width={m.isTV ? 140 : 90} height={m.isTV ? 80 : 50} radius={6} />
            <View style={{flex: 1}}><Text variant="bodyStrong">{r.title}</Text><Text variant="caption" tone="tertiary">{r.channelName} · {when(r)}</Text></View>
            <Button size="sm" label="Record this instead" variant="primary" onPress={() => void run(async () => { await client.live.resolveConflict(r.id, 'record'); setConflict(false); toast({message: `${r.title} will record.`}); })} />
            <Button size="sm" label="Skip this one" variant="ghost" onPress={() => void run(async () => { await client.live.resolveConflict(r.id, 'skip'); })} />
          </View>
        ))}
        {!d.conflicts.length ? <Text variant="body" tone="secondary">No conflicts right now.</Text> : null}
      </Sheet>
      {rule ? <RuleSheet rule={rule} onClose={() => setRule(undefined)} onDelete={() => { setDeleteRule(rule); setRule(undefined); }} /> : null}
      <Sheet visible={!!deleteRule} onClose={() => setDeleteRule(undefined)} title={`Delete the rule for ${deleteRule?.title}?`} subtitle="Recordings it already made are kept. Choose what happens to the ones it has scheduled." size="sm"
        footer={<><Button label="Keep them" variant="secondary" onPress={() => void run(async () => { await client.live.deleteRule(deleteRule!.id, false); setDeleteRule(undefined); })} /><Button label="Cancel them" variant="danger" onPress={() => void run(async () => { await client.live.deleteRule(deleteRule!.id, true); setDeleteRule(undefined); })} /></>} />
    </View>
  );
}

function RuleSheet({rule, onClose, onDelete}: {rule: SeriesRule; onClose: () => void; onDelete: () => void}) {
  const client = useClient();
  const m = useMetrics();
  const [r, setR] = useState(rule);
  const {busy, run} = useAction();
  return (
    <Sheet visible onClose={onClose} title={`Edit rule · ${rule.title}`} subtitle={`${rule.upcoming} upcoming airings match`} size="md"
      footer={<><Button label="Delete rule…" icon="trash" variant="danger" onPress={onDelete} /><View style={{flex: 1}} /><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Save" variant="primary" loading={busy} onPress={() => void run(async () => { await client.live.updateRule(r.id, r); onClose(); })} /></>}>
      <Toggle label={r.paused ? 'Rule is paused' : 'Rule is on'} help="Paused rules keep their recordings but don’t schedule new ones." value={!r.paused} onChange={v => setR({...r, paused: !v})} />
      <Segmented value={r.newOnly ? 'new' : 'all'} onChange={v => setR({...r, newOnly: v === 'new'})} options={[{value: 'new', label: 'New episodes only'}, {value: 'all', label: 'All episodes'}]} />
      <Segmented value={r.channel} onChange={v => setR({...r, channel: v})} options={[{value: 'this', label: `This channel${r.channelName ? ` (${r.channelName})` : ''}`}, {value: 'any', label: 'Any channel'}]} />
      <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'}}><Text variant="body">Keep</Text><Segmented value={r.keep === 'all' ? 'all' : 'latest'} onChange={v => setR({...r, keep: v === 'all' ? 'all' : 5})} options={[{value: 'all', label: 'Every episode'}, {value: 'latest', label: 'The latest few'}]} /></View>
      {r.keep !== 'all' ? <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'}}><Text variant="body">Keep the latest</Text><Stepper label="Episodes" value={r.keep as number} onChange={v => setR({...r, keep: v})} min={1} max={30} /></View> : null}
      <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'}}><Text variant="body">Start early</Text><Stepper label="Start early" value={r.startEarly} onChange={v => setR({...r, startEarly: v})} min={0} max={30} unit="min" /></View>
      <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'}}><Text variant="body">End late</Text><Stepper label="End late" value={r.endLate} onChange={v => setR({...r, endLate: v})} min={0} max={120} step={5} unit="min" /></View>
      <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'}}><View><Text variant="body">Priority</Text><Text variant="caption" tone="tertiary">Higher wins a tuner clash</Text></View><Stepper label="Priority" value={r.priority} onChange={v => setR({...r, priority: v})} min={1} max={10} /></View>
      <Text variant="caption" tone="tertiary">{m.isTV ? '' : ''}</Text>
    </Sheet>
  );
}
