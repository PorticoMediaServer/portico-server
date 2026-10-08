import React, {useEffect, useState} from 'react';
import {FlatList, ScrollView, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import type {Channel, Program} from '../../data/types';
import {Screen, useChromeInsets} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Badge, Chip, EmptyState, ListRow, Loading, ProgressBar, Segmented, Tabs} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {Focusable} from '../../ui/Focusable';
import {ActionMenu, Sheet} from '../../ui/Sheet';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useSession} from '../../app/session';
import {useToast} from '../../app/toast';
import {root} from '../../nav/nav';
import {GuideGrid} from './Guide';
import {Recordings} from './Recordings';

const HALF = 30 * 60000;
const time = (t: number) => new Date(t).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});

/** Live TV: the guide, the channel list and recordings (DVR). */
export function LiveScreen() {
  const route = useRoute() as {params?: {view?: 'guide' | 'channels' | 'recordings'}};
  const m = useMetrics();
  const client = useClient();
  const {profile, isAdmin} = useSession();
  const [view, setView] = useState(route.params?.view ?? 'guide');
  useEffect(() => { if (route.params?.view) setView(route.params.view); }, [route.params?.view]);
  const [source, setSource] = useState('all');
  const [group, setGroup] = useState('all');
  const [favorites, setFavorites] = useState(false);
  const [hidden, setHidden] = useState(false);
  const [offset, setOffset] = useState(0);
  const [program, setProgram] = useState<{p: Program; c: Channel}>();
  const [channelMenu, setChannelMenu] = useState<Channel>();
  const base = Math.floor(Date.now() / HALF) * HALF - HALF + offset;
  const span = (m.isTV ? 3.5 : m.isPhone ? 3 : 4) * 3600000;
  const sources = useQuery(c => c.live.sources(), []);
  const channels = useQuery(c => c.live.channels({source, group, favorites, hidden}), [source, group, favorites, hidden, profile?.id], ['live']);
  const guide = useQuery(c => c.live.guide(base, base + span + 6 * 3600000, channels.data?.map(x => x.id)), [base, channels.data], ['live']);
  const chrome = useChromeInsets();
  const actions = useActions();
  const groups = ['all', ...new Set((channels.data ?? []).map(c => c.group))];
  const watch = (c: Channel, p?: Program) => void actions.play({id: c.id, kind: 'channel', title: p?.title ?? c.name, subtitle: `${c.number} · ${c.name}`, image: p?.image, shape: 'wide', caption: p?.episodeTitle});
  const day = new Date(base + HALF).toDateString() === new Date().toDateString() ? 'Today' : 'Tomorrow';
  return (
    <Screen scroll={view !== 'guide'} contentStyle={{gap: m.space.lg}}>
      <View style={{paddingHorizontal: m.gutter, gap: m.space.md}}>
        <View style={{flexDirection: 'row', alignItems: 'flex-end', gap: m.space.md}}>
          <View style={{flex: 1}}><Text variant="label" tone="accent">Live TV · {channels.data?.length ?? '…'} channels</Text><Text variant="display">{view === 'guide' ? 'Guide' : view === 'channels' ? 'Channels' : 'Recordings'}</Text></View>
          {isAdmin && !m.isPhone ? <Button size="sm" icon="settings" label="Live TV settings" variant="ghost" onPress={() => root('Admin', {section: 'live-tv'})} /> : null}
        </View>
        <Tabs tabs={[{value: 'guide', label: 'Guide'}, {value: 'channels', label: 'Channels'}, ...(profile?.limits?.dvr === false ? [] : [{value: 'recordings' as const, label: 'Recordings'}])]} value={view} onChange={v => setView(v)} />
        {view !== 'recordings' ? (
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: m.space.xs, alignItems: 'center', paddingVertical: m.isTV ? 8 : 0}}>
            <Chip icon="starFilled" label="Favorites" selected={favorites} onPress={() => setFavorites(f => !f)} />
            {(sources.data ?? []).length > 1 ? [{id: 'all', name: 'All sources'}, ...(sources.data ?? [])].map(s => <Chip key={s.id} label={s.name} selected={source === s.id} onPress={() => setSource(s.id)} />) : null}
            <View style={{width: 1, height: 24, backgroundColor: colors.line, marginHorizontal: 4}} />
            {groups.map(g => <Chip key={g} label={g === 'all' ? 'All groups' : g} selected={group === g} onPress={() => setGroup(g)} />)}
            <Chip icon="eyeOff" label="Hidden" selected={hidden} onPress={() => setHidden(h => !h)} />
          </ScrollView>
        ) : null}
        {view === 'guide' ? (
          <View style={{flexDirection: 'row', gap: m.space.sm, alignItems: 'center', flexWrap: 'wrap'}}>
            <Segmented value={day} onChange={d => setOffset(d === 'Today' ? 0 : Math.round((new Date(new Date().setHours(24, 0, 0, 0)).getTime() - Date.now()) / HALF) * HALF + 18 * 3600000)} options={[{value: 'Today', label: 'Today'}, {value: 'Tomorrow', label: 'Tomorrow'}]} />
            <Button size="sm" icon="back" label={m.isPhone ? undefined : 'Earlier'} accessibilityLabel="Two hours earlier" variant="secondary" onPress={() => setOffset(o => o - 2 * 3600000)} disabled={offset <= -6 * 3600000} />
            <Button size="sm" label="Now" variant={offset === 0 ? 'primary' : 'secondary'} onPress={() => setOffset(0)} />
            <Button size="sm" iconRight="forward" label={m.isPhone ? undefined : 'Later'} accessibilityLabel="Two hours later" variant="secondary" onPress={() => setOffset(o => o + 2 * 3600000)} />
          </View>
        ) : null}
      </View>
      {view === 'guide' ? (
        channels.data && guide.data ? (channels.data.length ? (
          <View style={{flex: 1, paddingLeft: m.gutter, paddingBottom: chrome.bottom}}>
            <GuideGrid channels={channels.data} guide={guide.data} from={base} to={base + span} onProgram={(p, c) => setProgram({p, c})} onChannel={c => setChannelMenu(c)} height={m.height - (m.isTV ? 420 : m.isPhone ? 380 : 330) - chrome.top} />
          </View>
        ) : <EmptyState icon="live" title="No channels match" body="Try a different search, or turn off a filter." action={{label: 'Clear filters', onPress: () => { setFavorites(false); setGroup('all'); setSource('all'); }}} />) : <Loading label="Loading the guide" />
      ) : null}
      {view === 'channels' ? (
        <View style={{paddingHorizontal: m.gutter, gap: m.isTV ? 8 : 4}}>
          {channels.data?.map(c => <ChannelRow key={c.id} c={c} progs={guide.data?.[c.id] ?? []} onWatch={p => watch(c, p)} onMore={() => setChannelMenu(c)} onProgram={p => setProgram({p, c})} />)}
          {channels.data && !channels.data.length ? <EmptyState icon="channels" title="No channels in this filter" action={{label: 'Show all channels', onPress: () => { setFavorites(false); setGroup('all'); }}} /> : null}
        </View>
      ) : null}
      {view === 'recordings' ? <Recordings /> : null}
      {program ? <ProgramSheet p={program.p} c={program.c} onClose={() => setProgram(undefined)} onWatch={() => { setProgram(undefined); watch(program.c, program.p); }} /> : null}
      <ActionMenu visible={!!channelMenu} onClose={() => setChannelMenu(undefined)} title={channelMenu?.name} subtitle={channelMenu ? `Channel ${channelMenu.number}${channelMenu.library ? ' · Library Channel' : ''}` : undefined} image={channelMenu?.logo} items={[
        {id: 'watch', label: 'Watch now', icon: 'play'},
        {id: 'fav', label: channelMenu?.favorite ? 'Remove from Favorites' : 'Add to Favorites', icon: channelMenu?.favorite ? 'starFilled' : 'star'},
        {id: 'hide', label: channelMenu?.hidden ? 'Show channel' : 'Hide channel', icon: channelMenu?.hidden ? 'eye' : 'eyeOff', subtitle: channelMenu?.hidden ? 'Bring this channel back into the guide.' : 'Hidden channels stay out of the guide until you show them again.'},
        {id: 'edit', label: 'Edit channel', icon: 'edit', hidden: !isAdmin},
      ]} onSelect={async id => {
        const c = channelMenu!;
        setChannelMenu(undefined);
        if (id === 'watch') watch(c, guide.data?.[c.id]?.[0]);
        if (id === 'fav') await client.live.setFavorite(c.id, !c.favorite);
        if (id === 'hide') await client.live.setHidden(c.id, !c.hidden);
        if (id === 'edit') root('Admin', {section: c.library ? 'channel-builder' : 'live-tv', param: c.library ? c.id : undefined});
      }} />
    </Screen>
  );
}

function ChannelRow({c, progs, onWatch, onMore, onProgram}: {c: Channel; progs: Program[]; onWatch: (p?: Program) => void; onMore: () => void; onProgram: (p: Program) => void}) {
  const m = useMetrics();
  const now = Date.now();
  const cur = progs.find(p => p.start <= now && p.end > now);
  const next = progs.find(p => p.start >= (cur?.end ?? now));
  return (
    <Focusable onPress={() => onWatch(cur)} onLongPress={onMore} accessibilityLabel={`${c.name}, now ${cur?.title ?? 'no information'}`}
      style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.md, padding: m.isTV ? 16 : 10, borderRadius: m.radius.panel, backgroundColor: focused ? colors.raisedBright : colors.raised, borderWidth: 1, borderColor: focused ? colors.focus : colors.lineSoft})}>
      <View style={{width: m.isTV ? 130 : 80, alignItems: 'center', gap: 2}}>
        <Artwork source={c.logo} title={c.name} width={m.isTV ? 120 : 72} height={m.isTV ? 66 : 40} radius={4} resizeMode="contain" style={{backgroundColor: 'transparent'}} />
        <Text variant="micro" tone="tertiary">{c.number}{c.favorite ? ' ★' : ''}</Text>
      </View>
      {!m.isPhone && cur?.image ? <Artwork source={cur.image} title={cur.title} width={m.isTV ? 200 : 128} height={m.isTV ? 112 : 72} radius={m.radius.small} /> : null}
      <View style={{flex: 1, gap: 4}}>
        <View style={{flexDirection: 'row', gap: 8, alignItems: 'center'}}>
          <Text variant="bodyStrong" numberOfLines={1} style={{flexShrink: 1}}>{cur?.title ?? 'No information'}</Text>
          {cur?.isLive ? <Badge label="Live" tone="record" /> : cur?.isNew ? <Badge label="New" tone="accent" /> : null}
          {c.library ? <Badge label="Library Channel" icon="collection" /> : null}
        </View>
        {cur ? <ProgressBar value={(now - cur.start) / (cur.end - cur.start)} height={m.isTV ? 5 : 3} /> : null}
        <Text variant="caption" tone="tertiary" numberOfLines={1}>{cur ? `Until ${time(cur.end)}` : ''}{next ? `  ·  Next · ${time(next.start)}  ${next.title}` : ''}</Text>
      </View>
      {!m.isTV ? <Button size="sm" icon="info" accessibilityLabel={`More for ${c.name}`} variant="ghost" onPress={() => (cur ? onProgram(cur) : onMore())} /> : null}
    </Focusable>
  );
}

/** Program details: watch, record, record series, remind. */
export function ProgramSheet({p, c, onClose, onWatch}: {p: Program; c: Channel; onClose: () => void; onWatch: () => void}) {
  const m = useMetrics();
  const client = useClient();
  const toast = useToast();
  const {profile} = useSession();
  const [series, setSeries] = useState(false);
  const [channel, setChannel] = useState<'this' | 'any'>('this');
  const [newOnly, setNewOnly] = useState(true);
  const {busy, run} = useAction();
  const q = useQuery(cl => cl.live.program(p.id), [p.id], ['live']);
  const prog = q.data?.program ?? p;
  const now = Date.now();
  const onNow = prog.start <= now && prog.end > now;
  const future = prog.start > now;
  const canRecord = !c.library && profile?.limits?.dvr !== false;
  return (
    <Sheet visible onClose={onClose} size="md" header={prog.image ? <View><Artwork source={prog.image} title={prog.title} width="100%" height={m.isTV ? 300 : 180} radius={0} /></View> : undefined} title={prog.title}
      subtitle={[prog.episodeTitle, prog.season ? `S${prog.season} · E${prog.episode}` : undefined].filter(Boolean).join(' · ') || undefined}>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, alignItems: 'center'}}>
        {onNow ? <Badge label="On now" tone="record" /> : null}
        {prog.isNew ? <Badge label="New" tone="accent" /> : null}
        {prog.isLive ? <Badge label="Live" tone="record" /> : null}
        {prog.premiere ? <Badge label="Premiere" tone="good" /> : null}
        {prog.rating ? <Badge label={prog.rating} /> : null}
        <Text variant="caption" tone="secondary">{c.number} {c.name} · {new Date(prog.start).toLocaleDateString('en-US', {weekday: 'short'})} {time(prog.start)} – {time(prog.end)}{onNow ? ` · Started ${Math.round((now - prog.start) / 60000)} min ago · ${Math.round((prog.end - now) / 60000)} min left` : ''}</Text>
      </View>
      <Text variant="body" tone="secondary">{prog.overview ?? 'No description for this program.'}</Text>
      {q.data?.next ? <Text variant="caption" tone="tertiary">Next: {q.data.next.title} · {time(q.data.next.start)}</Text> : null}
      {series ? (
        <View style={{gap: m.space.sm}}>
          <Text variant="subheading">Record series</Text>
          <Segmented value={newOnly ? 'new' : 'all'} onChange={v => setNewOnly(v === 'new')} options={[{value: 'new', label: 'New episodes only'}, {value: 'all', label: 'All episodes'}]} />
          <Segmented value={channel} onChange={setChannel} options={[{value: 'this', label: 'This channel'}, {value: 'any', label: 'Any channel'}]} />
          <View style={{flexDirection: 'row', gap: 8}}>
            <Button label="Record series" icon="recordings" variant="primary" loading={busy} onPress={() => void run(async () => { await client.live.recordSeries(prog.id, {channel, newOnly}); toast({message: 'Series recording set up. New airings are recorded automatically.', icon: 'recordings'}); onClose(); })} />
            <Button label="Cancel" variant="ghost" onPress={() => setSeries(false)} />
          </View>
        </View>
      ) : (
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm}}>
          {onNow ? <Button label="Watch live" icon="play" variant="primary" onPress={onWatch} hasTVPreferredFocus /> : <Button label="Watch channel now" icon="play" variant={future ? 'secondary' : 'primary'} onPress={onWatch} hasTVPreferredFocus={!future} />}
          {canRecord ? (prog.recording ? <Button label="Cancel recording" icon="close" variant="danger" onPress={() => void run(async () => { await client.live.cancelRecording(prog.id); toast({message: 'Recording canceled'}); onClose(); })} /> : <Button label="Record" icon="record" variant={future ? 'primary' : 'secondary'} loading={busy} hasTVPreferredFocus={future} onPress={() => void run(async () => { await client.live.record(prog.id); toast({message: 'Recording scheduled. It’s listed under Recordings › Upcoming.', icon: 'record'}); onClose(); })} />) : null}
          {canRecord && prog.season ? <Button label="Record series…" icon="recordings" onPress={() => setSeries(true)} /> : null}
          {future ? <Button label={prog.reminder ? 'Stop reminding' : 'Remind me'} icon="bell" variant="secondary" onPress={() => void run(async () => { await client.live.remind(prog.id, !prog.reminder); toast({message: prog.reminder ? 'Reminder removed.' : 'You’ll get a reminder 2 minutes before it starts, while Portico is open.', icon: 'bell'}); onClose(); })} /> : null}
          {!canRecord && c.library ? <Text variant="caption" tone="tertiary">Channels made from your library play what you already have, so there’s nothing to record.</Text> : null}
        </View>
      )}
    </Sheet>
  );
}

export {FlatList, ListRow};
