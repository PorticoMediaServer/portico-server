import React, {useMemo, useState} from 'react';
import {FlatList, ScrollView, StyleSheet, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import type {Services} from '../../data/services';
import {useQuery, useServices} from '../../data/hooks';
import type {Channel, Programme, Recording, RecordingRule} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {Badge, Chip, ListRow, Meter, Panel, Segmented, Stepper, TextField, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Banner, EmptyState, RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {go, type Routes} from '../../nav/routes';
import {useSession} from '../../state/session';
import {plural} from '../../design/format';

const MIN = 60_000;
export const fmtTime = (t: number) => new Date(t).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
const dayLabel = (t: number) => {
  const d = new Date(t).toDateString();
  if (d === new Date().toDateString()) return 'Today';
  if (d === new Date(Date.now() + 86400000).toDateString()) return 'Tomorrow';
  if (d === new Date(Date.now() - 86400000).toDateString()) return 'Yesterday';
  return new Date(t).toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'});
};

export function LiveScreen() {
  const route = useRoute<RouteProp<Routes, 'Live'>>();
  const [tab, setTab] = useState<'guide' | 'channels' | 'dvr'>(route.params?.tab ?? 'guide');
  React.useEffect(() => {
    if (route.params?.tab) setTab(route.params.tab);
  }, [route.params?.tab]);
  const session = useSession();
  return (
    <Screen heading="Live TV" eyebrow={session.server?.name} scroll={tab !== 'guide'}>
      <View style={{flexDirection: 'row', marginBottom: u(12)}}>
        <Segmented value={tab} onChange={setTab} options={[{value: 'guide', label: 'Guide', icon: 'calendar'}, {value: 'channels', label: 'Channels', icon: 'tv'}, {value: 'dvr', label: 'DVR', icon: 'dvr'}]} />
      </View>
      {tab === 'guide' ? <Guide /> : tab === 'channels' ? <Channels /> : <DVR />}
    </Screen>
  );
}

// ---------------------------------------------------------------- Programme sheet

export function openProgramme(s: Services, p: Programme, ch: Channel | undefined, recording?: Recording) {
  const now = Date.now();
  const onNow = p.start <= now && p.end > now;
  const future = p.start > now;
  ui.sheet({
    title: p.title,
    subtitle: `${ch ? `${ch.number} ${ch.name} · ` : ''}${dayLabel(p.start)} ${fmtTime(p.start)} – ${fmtTime(p.end)}`,
    render: close => <ProgrammeBody s={s} p={p} ch={ch} recording={recording} onNow={onNow} future={future} close={close} />,
  });
}

function ProgrammeBody({s, p, ch, recording, onNow, future, close}: {s: Services; p: Programme; ch?: Channel; recording?: Recording; onNow: boolean; future: boolean; close: () => void}) {
  const reminders = useQuery('reminders', () => s.live.reminders());
  const reminded = reminders.data?.includes(p.id);
  const [seriesOpts, setSeriesOpts] = useState(false);
  const [newOnly, setNewOnly] = useState(true);
  const [keep, setKeep] = useState(5);
  const [early, setEarly] = useState(1);
  const [late, setLate] = useState(2);
  const left = Math.round((p.end - Date.now()) / MIN);
  const started = Math.round((Date.now() - p.start) / MIN);
  if (seriesOpts) {
    return (
      <View>
        <Text variant="heading" style={{marginBottom: 8}}>Record series</Text>
        <Segmented value={newOnly ? 'new' : 'all'} onChange={v => setNewOnly(v === 'new')} options={[{value: 'new', label: 'New episodes'}, {value: 'all', label: 'All episodes'}]} />
        <Stepper label="Keep" help={keep === 0 ? 'Keep every episode' : `Keep the latest ${keep}`} value={keep} onChange={setKeep} min={0} max={20} />
        <Stepper label="Start early" value={early} onChange={setEarly} min={0} max={30} unit="min" />
        <Stepper label="End late" value={late} onChange={setLate} min={0} max={120} step={5} unit="min" />
        <Button label="Record series" icon="record" variant="primary" hasTVPreferredFocus style={{marginTop: 12}} onPress={() => s.live.record(p.id, {series: true, newOnly, keep: keep || 'all', startEarly: early, endLate: late}).then(() => { ui.toast('Series recording set up. New airings are recorded automatically.'); close(); })} />
      </View>
    );
  }
  return (
    <View>
      <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap', marginBottom: 10}}>
        {onNow ? <Badge label="On now" tone="accent" /> : null}
        {p.isNew ? <Badge label="New" tone="amber" /> : null}
        {recording ? <Badge label={recording.state === 'recording' ? 'Recording' : 'Recording scheduled'} tone="record" /> : null}
        <Badge label={p.genre} tone="neutral" />
        <Badge label={p.rating} tone="neutral" />
      </View>
      {p.episodeTitle ? <Text variant="subheading">{p.episodeTitle}</Text> : null}
      <Text variant="body" tone="secondary" style={{marginVertical: 8}}>{p.synopsis || 'No description for this program.'}</Text>
      {onNow ? <Text variant="caption" tone="tertiary">Started {started} min ago · {left} min left</Text> : null}
      <View style={{gap: 4, marginTop: 12}}>
        {onNow ? <ListRow icon="play" label="Watch live" onPress={() => { close(); go('LivePlayer', {channelId: p.channelId}); }} hasTVPreferredFocus /> : null}
        {!recording && (onNow || future) ? <ListRow icon="record" label="Record" help="Just this airing" onPress={() => s.live.record(p.id, {series: false}).then(() => { ui.toast('Recording scheduled. It’s listed under DVR › Upcoming.'); close(); })} hasTVPreferredFocus={!onNow} /> : null}
        {p.series && (onNow || future) ? <ListRow icon="layers" label="Record series…" help="New or all episodes, how many to keep" onPress={() => setSeriesOpts(true)} /> : null}
        {recording && recording.state === 'scheduled' ? <ListRow icon="x" label="Cancel recording" destructive onPress={() => s.live.cancelRecording(recording.id).then(() => { ui.toast('Recording canceled'); close(); })} /> : null}
        {recording ? <ListRow icon="dvr" label="View in DVR" onPress={() => { close(); go('Live', {tab: 'dvr'}); }} /> : null}
        {future ? <ListRow icon={reminded ? 'bellRing' : 'bell'} label={reminded ? 'Stop reminding me' : 'Remind me'} help="A notification when it starts" onPress={() => s.live.toggleReminder(p.id).then(on => ui.toast(on ? 'Reminder set.' : 'Reminder removed.'))} /> : null}
        {ch ? <ListRow icon="tv" label={`More for ${ch.name}`} onPress={() => { close(); setTimeout(() => openChannel(s, ch), 250); }} /> : null}
      </View>
    </View>
  );
}

export function openChannel(s: Services, ch: Channel) {
  ui.menu({title: `${ch.number} ${ch.name}`, subtitle: `${ch.group} · ${ch.source}`, actions: [
    {label: 'Watch channel now', icon: 'play', onPress: () => go('LivePlayer', {channelId: ch.id})},
    {label: ch.favorite ? 'Remove from favorite channels' : 'Add to favorite channels', icon: 'star', onPress: () => s.live.setChannel(ch.id, {favorite: !ch.favorite})},
    {label: ch.hidden ? 'Show channel' : 'Hide channel', icon: ch.hidden ? 'eye' : 'eyeOff', detail: ch.hidden ? 'Bring this channel back into the guide.' : 'Hidden channels stay out of the guide until you show them again.', onPress: () => s.live.setChannel(ch.id, {hidden: !ch.hidden})},
  ]});
}

// ---------------------------------------------------------------- Guide grid

function useChannelFilter() {
  const [group, setGroup] = useState('All');
  const [favorites, setFavorites] = useState(false);
  const [hidden, setHidden] = useState(false);
  const [query, setQuery] = useState('');
  return {group, setGroup, favorites, setFavorites, hidden, setHidden, query, setQuery};
}

function Filters({f, groups}: {f: ReturnType<typeof useChannelFilter>; groups: string[]}) {
  return (
    <View style={{marginBottom: u(10)}}>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8), alignItems: 'center'}}>
        {['All', ...groups].map(g => <Chip key={g} label={g === 'All' ? 'All groups' : g === 'Library' ? 'Library Channels' : g} active={f.group === g} onPress={() => f.setGroup(g)} />)}
        <Chip label="Favorites" icon="star" active={f.favorites} onPress={() => f.setFavorites(!f.favorites)} />
        <Chip label="Hidden" icon="eyeOff" active={f.hidden} onPress={() => f.setHidden(!f.hidden)} />
      </ScrollView>
      {!isTV ? <TextField value={f.query} onChangeText={f.setQuery} placeholder="Find a channel" icon="search" style={{marginTop: u(10), marginBottom: 0}} /> : null}
    </View>
  );
}

const applyFilter = (list: Channel[], f: ReturnType<typeof useChannelFilter>) =>
  list.filter(c => (f.group === 'All' || c.group === f.group) && (!f.favorites || c.favorite) && (f.hidden || !c.hidden) && (!f.query || c.name.toLowerCase().includes(f.query.toLowerCase()) || c.number.includes(f.query)));

function Guide() {
  const s = useServices();
  const layout = useLayout();
  const f = useChannelFilter();
  const half = 30 * MIN;
  const [start, setStart] = useState(() => Math.floor(Date.now() / half) * half - half);
  const span = isTV ? 3 * 60 * MIN : layout.compact ? 2 * 60 * MIN : 3 * 60 * MIN;
  const end = start + span;
  const chans = useQuery('channels', () => s.live.channels());
  const guide = useQuery(`guide:${start}`, () => s.live.guide(start, end));
  const recs = useQuery('recordings', () => s.live.recordings());
  const groups = useMemo(() => [...new Set((chans.data ?? []).map(c => c.group))], [chans.data]);
  const list = applyFilter(chans.data ?? [], f);
  const colW = isTV ? 300 : layout.compact ? 92 : 190;
  const gridW = layout.width - layout.railWidth - layout.gutter * 2 - colW;
  const pxPerMin = gridW / (span / MIN);
  const rowH = isTV ? 110 : 64;
  const now = Date.now();
  const byChannel = useMemo(() => {
    const m = new Map<string, Programme[]>();
    (guide.data ?? []).forEach(p => m.set(p.channelId, [...(m.get(p.channelId) ?? []), p]));
    return m;
  }, [guide.data]);
  const recByProg = new Map((recs.data ?? []).filter(r => r.programmeId).map(r => [r.programmeId!, r]));
  const slots = Array.from({length: span / half}, (_, i) => start + i * half);
  return (
    <View style={{flex: 1}}>
      <Filters f={f} groups={groups} />
      <View style={{flexDirection: 'row', alignItems: 'center', gap: u(8), marginBottom: u(8), flexWrap: 'wrap'}}>
        <Button icon="chevronLeft" label="Earlier" size="sm" onPress={() => setStart(v => v - 2 * 60 * MIN)} />
        <Button icon="clock" label="Jump to now" size="sm" onPress={() => setStart(Math.floor(Date.now() / half) * half - half)} />
        <Button iconRight="chevronRight" label="Later" size="sm" onPress={() => setStart(v => v + 2 * 60 * MIN)} />
        <Button icon="calendar" label={dayLabel(start + half)} size="sm" onPress={() => ui.menu({title: 'Day', actions: [-1, 0, 1].map(d => ({label: dayLabel(Date.now() + d * 86400000), onPress: () => setStart(Math.floor((Date.now() + d * 86400000) / half) * half - half)}))})} />
      </View>
      <View style={{flexDirection: 'row', marginLeft: colW}}>
        {slots.map(t => <Text key={t} variant="captionStrong" tone="tertiary" style={{width: pxPerMin * 30}}>{fmtTime(t)}</Text>)}
      </View>
      {chans.loading && !chans.data ? <RowsSkeleton rows={2} /> : null}
      {chans.data && !list.length ? <EmptyState icon="tv" title="No channels match" body="Try a different search, or turn off a filter." action={{label: 'Clear filters', onPress: () => { f.setGroup('All'); f.setFavorites(false); f.setQuery(''); }}} /> : null}
      <FlatList
        data={list}
        keyExtractor={c => c.id}
        initialNumToRender={isTV ? 7 : 10}
        contentContainerStyle={{paddingBottom: 160}}
        getItemLayout={(_, i) => ({length: rowH + 4, offset: (rowH + 4) * i, index: i})}
        renderItem={({item: ch, index}) => (
          <View style={{flexDirection: 'row', height: rowH, marginBottom: 4}}>
            <Focusable onPress={() => openChannel(s, ch)} scale={1.03} accessibilityLabel={`${ch.number} ${ch.name}`} style={{width: colW - 6, marginRight: 6}}>
              {({focused}) => (
                <View style={[styles.chCell, {height: rowH}, focused && isTV && {backgroundColor: colors.white}]}>
                  <Artwork art={ch.logo} tint={ch.tint} style={{width: isTV ? 110 : layout.compact ? 60 : 80, height: isTV ? 62 : layout.compact ? 34 : 45, borderRadius: 6}} />
                  {!layout.compact ? (
                    <View style={{marginLeft: 10, flex: 1}}>
                      <Text variant="captionStrong" numberOfLines={1} color={focused && isTV ? colors.actionInk : colors.text}>{ch.number}</Text>
                      <Text variant="caption" numberOfLines={1} color={focused && isTV ? colors.actionInk : colors.textTertiary}>{ch.name}</Text>
                    </View>
                  ) : null}
                  {ch.favorite ? <View style={{position: 'absolute', top: 4, right: 4}}><Icon name="star" size={10} color={colors.amber} fill /></View> : null}
                </View>
              )}
            </Focusable>
            <View style={{width: gridW, height: rowH, overflow: 'hidden'}}>
              {(byChannel.get(ch.id) ?? []).map((p, pi) => {
                const x = Math.max(0, (p.start - start) / MIN) * pxPerMin;
                const x2 = Math.min(span / MIN, (p.end - start) / MIN) * pxPerMin;
                if (x2 <= 0 || x >= gridW) return null;
                const live = p.start <= now && p.end > now;
                const rec = recByProg.get(p.id);
                return (
                  <Focusable key={p.id} onPress={() => openProgramme(s, p, ch, rec)} scale={1} hasTVPreferredFocus={index === 0 && live} accessibilityLabel={`${p.title}, ${fmtTime(p.start)} to ${fmtTime(p.end)}`} style={{position: 'absolute', left: x, width: Math.max(8, x2 - x - 4), height: rowH}}>
                    {({focused}) => (
                      <View style={[styles.cell, {height: rowH}, live && styles.cellLive, focused && isTV && styles.cellFocus]}>
                        <View style={{flexDirection: 'row', alignItems: 'center', gap: 4}}>
                          {rec ? <View style={styles.recDot} /> : null}
                          <Text variant="captionStrong" numberOfLines={1} color={focused && isTV ? colors.actionInk : colors.text} style={{flexShrink: 1}}>{p.title}</Text>
                        </View>
                        <Text variant="micro" numberOfLines={1} color={focused && isTV ? 'rgba(3,18,28,0.7)' : colors.textTertiary}>{p.isNew ? 'NEW · ' : ''}{fmtTime(p.start)}{p.episodeTitle ? ` · ${p.episodeTitle}` : ''}</Text>
                      </View>
                    )}
                  </Focusable>
                );
              })}
              {now > start && now < end ? <View pointerEvents="none" style={[styles.nowLine, {left: ((now - start) / MIN) * pxPerMin}]} /> : null}
            </View>
          </View>
        )}
      />
    </View>
  );
}

// ---------------------------------------------------------------- Channel list

function Channels() {
  const s = useServices();
  const f = useChannelFilter();
  const chans = useQuery('channels', () => s.live.channels());
  const now = Date.now();
  const guide = useQuery(`guide:now:${Math.floor(now / (5 * MIN))}`, () => s.live.guide(now, now + 3 * 60 * MIN));
  const groups = [...new Set((chans.data ?? []).map(c => c.group))];
  const list = applyFilter(chans.data ?? [], f);
  const lib = (chans.data ?? []).filter(c => c.library);
  return (
    <View>
      <Filters f={f} groups={groups} />
      {!lib.length ? <Banner icon="tv" title="No Library Channels yet" body="Library Channels turn the media you already own into channels that play around the clock." /> : null}
      {list.map((ch, i) => {
        const progs = (guide.data ?? []).filter(p => p.channelId === ch.id).sort((a, b) => a.start - b.start);
        const cur = progs.find(p => p.start <= now && p.end > now);
        const next = progs.find(p => p.start >= (cur?.end ?? now));
        return (
          <Focusable key={ch.id} onPress={() => go('LivePlayer', {channelId: ch.id})} onLongPress={() => openChannel(s, ch)} hasTVPreferredFocus={i === 0} scale={isTV ? 1.02 : 1} accessibilityLabel={`${ch.name}, now ${cur?.title ?? 'no information'}`}>
            {({focused}) => {
              const lit = focused && isTV;
              return (
                <View style={[styles.chRow, lit && {backgroundColor: colors.white}]}>
                  <Artwork art={ch.logo} tint={ch.tint} style={{width: u(84), height: u(47), borderRadius: 6}} />
                  <View style={{flex: 1, marginLeft: u(12)}}>
                    <Text variant="captionStrong" color={lit ? colors.actionInk : colors.textTertiary}>{ch.number} · {ch.name}{ch.favorite ? '  ★' : ''}</Text>
                    <Text variant="bodyStrong" numberOfLines={1} color={lit ? colors.actionInk : colors.text}>{cur ? cur.title : 'No program information'}</Text>
                    {cur ? <Meter value={(now - cur.start) / (cur.end - cur.start)} height={3} style={{marginVertical: 4, maxWidth: 260}} /> : null}
                    <Text variant="caption" numberOfLines={1} color={lit ? 'rgba(3,18,28,0.7)' : colors.textTertiary}>{next ? `Next · ${fmtTime(next.start)}  ${next.title}` : ''}</Text>
                  </View>
                  <IconButton icon="more" label={`More for ${ch.name}`} variant="ghost" onPress={() => openChannel(s, ch)} />
                </View>
              );
            }}
          </Focusable>
        );
      })}
    </View>
  );
}

// ---------------------------------------------------------------- DVR

function DVR() {
  const s = useServices();
  const [view, setView] = useState<'upcoming' | 'recorded' | 'rules' | 'history' | 'storage'>('upcoming');
  const recs = useQuery('recordings', () => s.live.recordings());
  const chans = useQuery('channels', () => s.live.channels());
  const chName = (id: string) => chans.data?.find(c => c.id === id);
  const all = recs.data ?? [];
  const upcoming = all.filter(r => r.state === 'scheduled' || r.state === 'conflicted' || r.state === 'recording').sort((a, b) => a.start - b.start);
  const recorded = all.filter(r => r.state === 'completed' || r.state === 'incomplete');
  const history = all.filter(r => r.state === 'failed' || r.state === 'cancelled' || r.state === 'incomplete');
  const stateBadge = (r: Recording) => ({scheduled: <Badge label="Scheduled" />, conflicted: <Badge label="Tuner conflict" tone="danger" />, recording: <Badge label="Recording" tone="record" />, completed: <Badge label="Recorded" tone="healthy" />, incomplete: <Badge label="Incomplete" tone="amber" />, failed: <Badge label="Failed" tone="danger" />, cancelled: <Badge label="Canceled" tone="neutral" />}[r.state]);
  const recMenu = (r: Recording) => ui.menu({title: r.title, subtitle: `${chName(r.channelId)?.name ?? ''} · ${dayLabel(r.start)} ${fmtTime(r.start)}`, actions: [
    ...(r.state === 'completed' || r.state === 'incomplete' || r.state === 'recording' ? [{label: 'Play', icon: 'play' as const, onPress: () => go('LivePlayer', {channelId: r.channelId})}] : []),
    ...(r.state === 'conflicted' ? [{label: 'Resolve conflict…', icon: 'alert' as const, onPress: () => resolve(r)}] : []),
    ...(r.state === 'scheduled' || r.state === 'conflicted' ? [{label: 'Cancel recording', icon: 'x' as const, destructive: true, onPress: () => ui.confirm({title: `Cancel recording ${r.title}?`, body: 'It won’t be recorded. A series rule, if there is one, stays in place.', confirmLabel: 'Cancel recording', cancelLabel: 'Keep it', destructive: true, onConfirm: () => s.live.cancelRecording(r.id)})}] : []),
    ...(r.state === 'completed' || r.state === 'incomplete' ? [
      {label: r.kept ? 'Stop keeping' : 'Keep until I delete it', icon: 'bookmark' as const, onPress: () => s.live.keepRecording(r.id, !r.kept)},
      {label: 'Delete recording', icon: 'trash' as const, destructive: true, onPress: () => ui.confirm({title: `Delete ${r.title}?`, body: r.kept ? `You chose to keep this recording. Deleting it frees ${r.sizeGB} GB and can’t be undone.` : `The recording (${r.sizeGB} GB) is deleted from the server. This can’t be undone.`, confirmLabel: 'Delete recording', destructive: true, onConfirm: () => s.live.deleteRecording(r.id)})},
    ] : []),
  ]});
  const resolve = (r: Recording) => ui.sheet({title: `${r.title} can’t record`, subtitle: 'You have 4 tuners, and these recordings need them at the same time.', render: close => (
    <View>
      {[r, ...upcoming.filter(x => x.id !== r.id).slice(0, 3)].map(x => <ListRow key={x.id} icon="record" label={x.title} help={`${chName(x.channelId)?.name ?? ''} · ${fmtTime(x.start)} – ${fmtTime(x.end)}`} compact />)}
      <View style={{flexDirection: 'row', gap: 10, marginTop: 12}}>
        <Button label="Skip this one" onPress={() => { s.live.resolveConflict(r.id, 'skip'); close(); }} />
        <Button label="Record this instead" variant="primary" hasTVPreferredFocus onPress={() => { s.live.resolveConflict(r.id, 'record'); close(); }} />
      </View>
    </View>
  )});
  const row = (r: Recording, i: number) => (
    <ListRow key={r.id} icon={r.state === 'recording' ? 'record' : r.state === 'completed' ? 'play' : 'calendarClock'} label={r.title + (r.episodeTitle ? ` · ${r.episodeTitle}` : '')} help={`${chName(r.channelId)?.number ?? ''} ${chName(r.channelId)?.name ?? ''} · ${dayLabel(r.start)} ${fmtTime(r.start)} – ${fmtTime(r.end)}${r.sizeGB ? ` · ${r.sizeGB} GB` : ''}${r.kept ? ' · Kept' : ''}${r.reason ? ` · ${r.reason}` : ''}`} right={stateBadge(r)} onPress={() => recMenu(r)} hasTVPreferredFocus={i === 0} />
  );
  return (
    <View>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8), marginBottom: u(12)}}>
        {([['upcoming', 'Upcoming'], ['recorded', 'Recorded'], ['rules', 'Series rules'], ['history', 'History'], ['storage', 'Storage']] as const).map(([v, l]) => <Chip key={v} label={l} active={view === v} onPress={() => setView(v)} />)}
      </ScrollView>
      {view === 'upcoming' ? (upcoming.length ? upcoming.map(row) : <EmptyState icon="calendarClock" title="Nothing scheduled" body="Open a program in the guide and choose Record." action={{label: 'Open the guide', onPress: () => go('Live', {tab: 'guide'})}} />) : null}
      {view === 'recorded' ? (recorded.length ? <RecordedGroups list={recorded} row={row} /> : <EmptyState icon="dvr" title="No recordings yet" body="Recordings appear here once they finish." />) : null}
      {view === 'rules' ? <Rules chName={chName} /> : null}
      {view === 'history' ? (history.length ? history.map(row) : <EmptyState icon="history" title="No recording history" />) : null}
      {view === 'storage' ? <DVRStorage /> : null}
    </View>
  );
}

function RecordedGroups({list, row}: {list: Recording[]; row: (r: Recording, i: number) => React.ReactNode}) {
  const groups = new Map<string, Recording[]>();
  list.forEach(r => groups.set(r.title, [...(groups.get(r.title) ?? []), r]));
  return (
    <View>
      {[...groups.entries()].map(([title, rs]) => (
        <Panel key={title} title={title} lede={`${rs.length} ${rs.length === 1 ? 'recording' : 'recordings'} · ${rs.filter(r => !r.watched).length} unwatched`}>
          {rs.map(row)}
        </Panel>
      ))}
    </View>
  );
}

function Rules({chName}: {chName: (id: string) => Channel | undefined}) {
  const s = useServices();
  const q = useQuery('rules', () => s.live.rules());
  const edit = (r: RecordingRule) => ui.sheet({title: `Edit rule · ${r.title}`, render: close => <RuleEditor r={r} close={close} />});
  if (q.data && !q.data.length) return <EmptyState icon="layers" title="No series rules" body="Choose Record series on a program in the guide to keep recording new episodes." />;
  return (
    <View>
      {(q.data ?? []).map((r, i) => (
        <ListRow key={r.id} icon="layers" label={r.title} hasTVPreferredFocus={i === 0} help={`${r.channelId ? chName(r.channelId)?.name ?? 'One channel' : 'Any channel'} · ${r.newOnly ? 'New episodes' : 'All episodes'} · ${r.keep === 'all' ? 'Keep every episode' : `Keep the latest ${r.keep}`} · ${r.scheduledCount} scheduled`} right={<Badge label={r.paused ? 'Paused' : 'On'} tone={r.paused ? 'neutral' : 'healthy'} />}
          onPress={() => ui.menu({title: r.title, actions: [
            {label: 'Edit rule…', icon: 'edit', onPress: () => edit(r)},
            {label: r.paused ? 'Resume rule' : 'Pause rule', icon: r.paused ? 'play' : 'pause', onPress: () => s.live.updateRule(r.id, {paused: !r.paused})},
            {label: 'Delete rule…', icon: 'trash', destructive: true, onPress: () => ui.sheet({title: `Delete the rule for ${r.title}?`, subtitle: 'Recordings it already made are kept. Choose what happens to the ones it has scheduled.', render: close => (
              <View style={{flexDirection: 'row', gap: 10}}>
                <Button label="Keep them" onPress={() => { s.live.deleteRule(r.id, false); close(); }} hasTVPreferredFocus />
                <Button label="Cancel them" variant="danger" onPress={() => { s.live.deleteRule(r.id, true); close(); }} />
              </View>
            )})},
          ]})} />
      ))}
    </View>
  );
}

function RuleEditor({r, close}: {r: RecordingRule; close: () => void}) {
  const s = useServices();
  const [x, setX] = useState(r);
  return (
    <View>
      <ToggleRow label="New episodes only" value={x.newOnly} onChange={v => setX({...x, newOnly: v})} />
      <Stepper label="Keep" help={x.keep === 'all' ? 'Keep every episode' : `Keep the latest ${x.keep}`} value={x.keep === 'all' ? 0 : x.keep} onChange={v => setX({...x, keep: v || 'all'})} min={0} max={20} />
      <Stepper label="Start early" value={x.startEarly} onChange={v => setX({...x, startEarly: v})} min={0} max={30} unit="min" />
      <Stepper label="End late" value={x.endLate} onChange={v => setX({...x, endLate: v})} min={0} max={120} step={5} unit="min" />
      <Stepper label="Priority" help="Higher wins a tuner clash" value={x.priority} onChange={v => setX({...x, priority: v})} min={1} max={5} />
      <Button label="Save" variant="primary" style={{marginTop: 12}} onPress={() => { s.live.updateRule(r.id, x); ui.toast('Rule saved.'); close(); }} />
    </View>
  );
}

function DVRStorage() {
  const s = useServices();
  const q = useQuery('dvr-storage', () => s.live.storage());
  const d = q.data;
  if (!d) return null;
  return (
    <Panel title="Recording space" lede={`Recordings are kept for ${plural(d.retentionDays, 'day', 'days')}`}>
      <Text variant="display">{d.usedGB} GB <Text variant="body" tone="tertiary">of {d.totalGB} GB</Text></Text>
      <Meter value={d.usedGB / d.totalGB} height={10} style={{marginVertical: u(10)}} />
      <Text variant="body" tone="secondary">About {d.hoursLeft} hours of recording space left</Text>
      <Text variant="caption" tone="tertiary">{d.pendingDeleteGB} GB waiting to be deleted</Text>
    </Panel>
  );
}

const styles = StyleSheet.create({
  chCell: {flexDirection: 'row', alignItems: 'center', paddingHorizontal: 6, borderRadius: radius.sm, backgroundColor: 'rgba(16,33,45,0.8)'},
  cell: {borderRadius: radius.sm, backgroundColor: 'rgba(29,56,72,0.55)', paddingHorizontal: u(8), justifyContent: 'center', borderWidth: StyleSheet.hairlineWidth, borderColor: colors.lineSoft},
  cellLive: {backgroundColor: 'rgba(64,159,212,0.28)', borderColor: 'rgba(92,179,226,0.5)'},
  cellFocus: {backgroundColor: colors.white, borderColor: colors.white},
  recDot: {width: u(7), height: u(7), borderRadius: 4, backgroundColor: colors.record},
  nowLine: {position: 'absolute', top: 0, bottom: 0, width: 2, backgroundColor: colors.accentStrong},
  chRow: {flexDirection: 'row', alignItems: 'center', padding: u(8), borderRadius: radius.md, marginBottom: u(4)},
});
