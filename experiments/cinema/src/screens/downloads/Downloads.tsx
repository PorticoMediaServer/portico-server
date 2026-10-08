import React, {useState} from 'react';
import {View} from 'react-native';
import {useQuery, useServices} from '../../data/hooks';
import type {Download} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {Badge, ListRow, Meter, Panel, Segmented, ToggleRow} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {Screen} from '../../nav/Screen';
import {go, push} from '../../nav/routes';
import {openItem, playItem, subtitleFor} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {plural} from '../../design/format';

const stateLabel: Record<Download['state'], string> = {queued: 'Waiting', downloading: 'Downloading', paused: 'Paused', done: 'On this device', failed: 'Failed', preparing: 'Preparing', ready: 'Ready to save', expired: 'Expired'};

/** Downloads (phone and tablet): on this device, on your server, whole-show requests and settings. */
export function DownloadsScreen() {
  const s = useServices();
  const [tab, setTab] = useState<'device' | 'server' | 'settings'>('device');
  const list = useQuery('downloads', () => s.downloads.list());
  const reqs = useQuery('download-requests', () => s.downloads.requests());
  const store = useQuery('download-storage', () => s.downloads.storage());
  const settings = useQuery('download-settings', () => s.downloads.settings());
  const items = useItems([...(list.data ?? []).map(d => d.itemId), ...(reqs.data ?? []).map(r => r.showId)]);
  if (isTV) return <Screen heading="Downloads"><EmptyState icon="download" title="Downloads aren’t available on this device" body="Save titles to your iPhone or iPad to watch offline." /></Screen>;
  const device = (list.data ?? []).filter(d => d.where === 'device');
  const server = (list.data ?? []).filter(d => d.where === 'server');
  const row = (d: Download) => {
    const it = items.lookup(d.itemId);
    const menu = () => ui.menu({title: it?.title ?? 'Download', actions: [
      ...(d.state === 'done' && it ? [{label: 'Play', icon: 'play' as const, onPress: () => playItem(it)}] : []),
      ...(d.state === 'downloading' || d.state === 'preparing' ? [{label: d.where === 'server' ? 'Pause preparing' : 'Pause', icon: 'pause' as const, onPress: () => s.downloads.pause(d.id)}] : []),
      ...(d.state === 'paused' ? [{label: 'Resume', icon: 'play' as const, onPress: () => s.downloads.resume(d.id)}] : []),
      ...(d.state === 'ready' ? [{label: 'Save file', icon: 'download' as const, onPress: () => ui.toast('Saving to this device…')}] : []),
      ...(it ? [{label: 'Go to title', icon: 'arrowRight' as const, onPress: () => openItem(it)}] : []),
      {label: d.where === 'server' ? (d.state === 'preparing' ? 'Stop preparing' : 'Remove from server') : 'Remove from this device', icon: 'trash', destructive: true, onPress: () => ui.confirm({title: d.where === 'server' ? 'Remove the prepared file?' : 'Remove from this device?', body: d.where === 'server' ? 'The server deletes the file it prepared. You can download the title again later.' : 'The file is deleted from this device. Your place in it is kept.', confirmLabel: 'Remove', destructive: true, onConfirm: () => s.downloads.remove(d.id)})},
    ]});
    return (
      <ListRow key={d.id} label={it?.title ?? 'Untitled download'} onPress={menu}
        help={`${it ? subtitleFor(it, items.lookup) + ' · ' : ''}${d.quality} · ${d.sizeMB > 1024 ? (d.sizeMB / 1024).toFixed(1) + ' GB' : d.sizeMB + ' MB'}${d.expires ? ` · kept until ${new Date(d.expires).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}` : ''}`}
        right={
          <View style={{alignItems: 'flex-end', width: 110}}>
            <Badge label={stateLabel[d.state]} tone={d.state === 'done' || d.state === 'ready' ? 'healthy' : d.state === 'expired' || d.state === 'failed' ? 'danger' : d.state === 'paused' ? 'neutral' : 'accent'} />
            {d.progress < 1 && d.state !== 'expired' ? <Meter value={d.progress} height={4} style={{width: 100, marginTop: 6}} /> : null}
            {d.state === 'downloading' ? <Text variant="micro" tone="tertiary" style={{marginTop: 3}}>{Math.round(d.progress * 100)}% · about 4 min left</Text> : null}
          </View>
        }
      />
    );
  };
  return (
    <Screen heading="Downloads" eyebrow="On this device">
      <Segmented value={tab} onChange={setTab} options={[{value: 'device', label: 'This device'}, {value: 'server', label: 'On your server'}, {value: 'settings', label: 'Settings'}]} style={{marginBottom: u(14)}} />
      {tab === 'device' ? (
        <>
          {store.data ? (
            <Panel>
              <Text variant="bodyStrong">{store.data.usedGB} GB used · {store.data.freeGB} GB free on this device</Text>
              <Meter value={store.data.usedGB / (settings.data?.limitGB ?? 64)} height={8} style={{marginTop: 8}} />
              <Text variant="caption" tone="tertiary" style={{marginTop: 6}}>Viewing done offline is sent to your server when it’s reachable.</Text>
            </Panel>
          ) : null}
          {!device.length && list.data ? <EmptyState icon="download" title="Nothing on this device yet" body="Press and hold a title and choose Download. It’s kept here to watch without a connection." action={{label: 'Browse movies', onPress: () => go('Library', {libraryId: 'lib-movies'})}} /> : null}
          {device.map(row)}
        </>
      ) : null}
      {tab === 'server' ? (
        <>
          <Text variant="caption" tone="tertiary" style={{marginBottom: 8}}>{store.data?.serverSpaceGB ?? 0} GB of download space left on the server.</Text>
          {(reqs.data ?? []).length ? (
            <Panel title="Whole shows and seasons">
              {(reqs.data ?? []).map(r => {
                const show = items.lookup(r.showId);
                return (
                  <View key={r.id} style={{flexDirection: 'row', alignItems: 'center', paddingVertical: 8}}>
                    <Artwork art={show?.art.poster} tint={show?.tint} style={{width: 40, height: 60, borderRadius: 6}} />
                    <View style={{flex: 1, marginLeft: 12}}>
                      <Text variant="bodyStrong">{show?.title}</Text>
                      <Text variant="caption" tone="tertiary">{{all: 'All episodes', unwatched: 'Unwatched only', next: `Keep the next ${plural(r.keepNext, 'episode', 'episodes')}`}[r.policy]} · {r.ready} of {r.total} ready{r.failed ? ` · ${r.failed} couldn’t be prepared` : ''}</Text>
                      <Meter value={r.ready / Math.max(1, r.total)} height={4} style={{marginTop: 6}} />
                    </View>
                    <IconButton icon="x" label="Cancel request" variant="ghost" onPress={() => s.downloads.remove(r.id)} />
                  </View>
                );
              })}
            </Panel>
          ) : null}
          {server.map(row)}
          {!server.length ? <EmptyState icon="server" title="Nothing prepared on your server" body="Open a title’s menu and choose Download. The server prepares the file, then you can save it." /> : null}
        </>
      ) : null}
      {tab === 'settings' && settings.data ? (
        <Panel title="Rules">
          <ToggleRow label="Wi-Fi only" help="Never use mobile data to download" value={settings.data.wifiOnly} onChange={v => s.downloads.updateSettings({wifiOnly: v})} />
          <ToggleRow label="Remove after watching" help="Free the space once you finish a title" value={settings.data.removeAfterWatching} onChange={v => s.downloads.updateSettings({removeAfterWatching: v})} />
          <ToggleRow label="Download the next episode" help="When you finish one, ask your server for the next" value={settings.data.nextEpisode} onChange={v => s.downloads.updateSettings({nextEpisode: v})} />
          <ListRow label="Storage limit" help="The most space downloads may use on this device" value={settings.data.limitGB ? `${settings.data.limitGB} GB` : 'No limit'} onPress={() => ui.menu({title: 'Storage limit', actions: [8, 16, 32, 64, 0].map(g => ({label: g ? `${g} GB` : 'No limit', checked: (settings.data!.limitGB ?? 0) === g, onPress: () => s.downloads.updateSettings({limitGB: g || null})}))})} />
          <ListRow label="Quality" help="Default for new downloads" value={settings.data.quality} onPress={() => ui.menu({title: 'Download quality', actions: ['Original', '1080p', '720p', '480p'].map(q => ({label: q, checked: settings.data!.quality === q, onPress: () => s.downloads.updateSettings({quality: q})}))})} />
        </Panel>
      ) : null}
      <Text variant="caption" tone="tertiary" style={{marginTop: 10}}>This server offers downloads. Its owner can turn them off in the server’s settings.</Text>
    </Screen>
  );
}

export {push, colors};
