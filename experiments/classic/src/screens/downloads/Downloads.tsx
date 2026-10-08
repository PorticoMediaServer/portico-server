import React, {useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import type {Download} from '../../data/types';
import {Screen} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Badge, EmptyState, Group, ListRow, ProgressBar, Surface, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {ActionMenu, ChoiceSheet, ConfirmDialog} from '../../ui/Sheet';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {openEntry} from '../../nav/nav';

const stateLabel: Record<Download['state'], {label: string; tone: 'neutral' | 'good' | 'warn' | 'bad' | 'accent'}> = {queued: {label: 'Waiting', tone: 'neutral'}, preparing: {label: 'Preparing', tone: 'accent'}, downloading: {label: 'Downloading', tone: 'accent'}, ready: {label: 'Ready', tone: 'good'}, paused: {label: 'Paused', tone: 'warn'}, failed: {label: 'Failed', tone: 'bad'}, expired: {label: 'Expired', tone: 'neutral'}};

/** Downloads (iPhone and iPad): what's on this device, what the server is preparing, storage and rules. */
export function DownloadsScreen() {
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const q = useQuery(c => c.downloads.list(), [], ['downloads']);
  const [menu, setMenu] = useState<Download>();
  const [confirm, setConfirm] = useState<Download>();
  const [pick, setPick] = useState<'limit' | 'quality'>();
  const {run} = useAction();
  const s = client.downloads.settings();
  if (m.isTV) return <Screen><EmptyState icon="download" title="Downloads aren’t available on this device" body="Download titles on your iPhone or iPad to watch without a connection." /></Screen>;
  const d = q.data;
  return (
    <Screen contentStyle={{gap: m.space.lg, paddingHorizontal: m.gutter}} refreshing={q.loading} onRefresh={q.refresh}>
      <View><Text variant="label" tone="accent">On this device</Text><Text variant="display">Downloads</Text></View>
      {d ? (
        <Surface style={{gap: 8}}>
          <View style={{flexDirection: 'row', justifyContent: 'space-between'}}><Text variant="bodyStrong">{d.usedGB} GB used</Text><Text variant="caption" tone="tertiary">{d.freeGB} GB free on this device</Text></View>
          <ProgressBar value={d.usedGB / (d.usedGB + d.freeGB)} height={8} />
          <Text variant="caption" tone="tertiary">{d.serverSpaceGB} GB of download space left on the server. Viewing done offline is sent to your server when it’s reachable.</Text>
        </Surface>
      ) : null}
      {d?.items.length ? (
        <Group title="Titles">
          {d.items.map(x => {
            const st = stateLabel[x.state];
            return (
              <ListRow key={x.id} left={<Artwork source={x.entry.image} title={x.entry.title} width={x.entry.shape === 'wide' ? 96 : 46} height={x.entry.shape === 'wide' ? 54 : 69} radius={6} />}
                title={x.entry.kind === 'episode' ? `${x.entry.subtitle} · ${x.entry.title}` : x.entry.title}
                subtitle={`${x.quality} · ${x.size}${x.expires ? ` · kept until ${x.expires}` : ''}${x.watched ? ' · Watched' : ''}`}
                trailing={<View style={{alignItems: 'flex-end', gap: 6, width: 110}}><Badge label={x.state === 'downloading' || x.state === 'preparing' ? `${st.label} ${Math.round(x.progress * 100)}%` : st.label} tone={st.tone} />{x.state === 'downloading' || x.state === 'preparing' || x.state === 'paused' ? <ProgressBar value={x.progress} style={{width: 100}} /> : null}</View>}
                onPress={() => (x.state === 'ready' ? void actions.play(x.entry) : setMenu(x))} onLongPress={() => setMenu(x)} chevron={false} />
            );
          })}
        </Group>
      ) : d ? <EmptyState icon="download" title="Nothing on this device yet" body="Press and hold a title and choose Download. It’s kept here to watch without a connection." /> : null}
      {d?.requests.length ? (
        <Group title="Whole shows and seasons">
          {d.requests.map(r => <ListRow key={r.id} left={<Artwork source={r.show.image} title={r.show.title} width={46} height={69} radius={6} />} title={r.show.title} subtitle={`${r.policy === 'all' ? 'All episodes' : r.policy === 'unwatched' ? 'Unwatched only' : 'Next unwatched'} · Keep the next ${r.keep} episodes\n${r.state === 'preparing' ? 'Finding everything to download…' : `${r.ready} of ${r.total} ready`}`} trailing={<Button size="sm" icon="close" accessibilityLabel="Remove" variant="ghost" onPress={() => void client.downloads.remove(r.id)} />} />)}
        </Group>
      ) : null}
      <Group title="Rules">
        <View style={{paddingHorizontal: 14}}><Toggle label="Wi-Fi only" help="Never use mobile data to download" value={s.wifiOnly} onChange={v => void client.downloads.updateSettings({wifiOnly: v})} /></View>
        <View style={{paddingHorizontal: 14}}><Toggle label="Remove after watching" help="Free the space once you finish a title" value={s.removeAfterWatching} onChange={v => void client.downloads.updateSettings({removeAfterWatching: v})} /></View>
        <View style={{paddingHorizontal: 14}}><Toggle label="Download the next episode" help="When you finish one, ask your server for the next" value={s.nextEpisode} onChange={v => void client.downloads.updateSettings({nextEpisode: v})} /></View>
        <ListRow title="Storage limit" subtitle="The most space downloads may use on this device" value={s.limitGB ? `${s.limitGB} GB` : 'No limit'} onPress={() => setPick('limit')} />
        <ListRow title="Default quality" value={s.quality} onPress={() => setPick('quality')} />
      </Group>
      <ActionMenu visible={!!menu} onClose={() => setMenu(undefined)} title={menu?.entry.title} image={menu?.entry.image} items={[
        {id: 'play', label: 'Play', icon: 'play', hidden: menu?.state !== 'ready'},
        {id: 'pause', label: menu?.state === 'paused' ? 'Resume preparing' : 'Pause preparing', icon: menu?.state === 'paused' ? 'play' : 'pause', hidden: !(menu && ['downloading', 'preparing', 'paused'].includes(menu.state))},
        {id: 'retry', label: 'Try again', icon: 'refresh', hidden: menu?.state !== 'failed'},
        {id: 'go', label: 'Go to title', icon: 'info'},
        {id: 'remove', label: 'Remove from this device', icon: 'trash', destructive: true},
      ]} onSelect={id => {
        const x = menu!;
        setMenu(undefined);
        if (id === 'play') void actions.play(x.entry);
        if (id === 'pause') void client.downloads.pause(x.id, x.state !== 'paused');
        if (id === 'retry') void client.downloads.retry(x.id);
        if (id === 'go') openEntry(x.entry);
        if (id === 'remove') setTimeout(() => setConfirm(x), 250);
      }} />
      <ConfirmDialog visible={!!confirm} onClose={() => setConfirm(undefined)} title={confirm?.state === 'ready' ? 'Remove from this device?' : 'Stop preparing this download?'} body={confirm?.state === 'ready' ? `${confirm?.entry.title} will need to download again to watch offline.` : 'Your server stops getting it ready. You can start again later.'} confirmLabel="Remove" destructive onConfirm={() => void run(async () => { await client.downloads.remove(confirm!.id); setConfirm(undefined); })} />
      <ChoiceSheet visible={pick === 'limit'} onClose={() => setPick(undefined)} title="Storage limit" options={[{value: 0, label: 'No limit'}, {value: 8, label: '8 GB'}, {value: 16, label: '16 GB'}, {value: 32, label: '32 GB'}, {value: 64, label: '64 GB'}]} value={s.limitGB ?? 0} onChange={v => void client.downloads.updateSettings({limitGB: v || null})} />
      <ChoiceSheet visible={pick === 'quality'} onClose={() => setPick(undefined)} title="Default quality" options={[{value: 'Original', label: 'Original', subtitle: 'The file as it is'}, {value: '1080p', label: '1080p'}, {value: '720p', label: '720p'}, {value: 'Smaller', label: 'Smaller'}, {value: 'Smallest', label: 'Smallest'}]} value={s.quality} onChange={v => void client.downloads.updateSettings({quality: v})} />
    </Screen>
  );
}
