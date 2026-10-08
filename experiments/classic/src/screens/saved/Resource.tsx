import React, {useState} from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import {Screen, ErrorView} from '../../ui/Page';
import {Button, IconButton} from '../../ui/Button';
import {EmptyState, ListRow, Loading, TextField, Toggle, Badge} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {ActionMenu, ConfirmDialog, Sheet} from '../../ui/Sheet';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useCardHandlers} from '../../app/useCards';
import {useToast} from '../../app/toast';
import {formatDuration} from '../../player/PlayerProvider';
import {back, root} from '../../nav/nav';
import {ActionRow} from '../detail/DetailHero';

/** A playlist, personal collection or saved view: play, reorder, remove, rename, share, delete. */
export function ResourceScreen() {
  const {params} = useRoute() as {params: {kind: 'playlist' | 'collection' | 'view'; id: string}};
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const cards = useCardHandlers();
  const q = useQuery(c => c.saved.resource(params.kind, params.id), [params.kind, params.id], ['saved', 'personal']);
  const [sheet, setSheet] = useState<'menu' | 'rename' | 'delete'>();
  const [name, setName] = useState('');
  const {busy, run} = useAction();
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  const d = q.data;
  const label = d.kind === 'playlist' ? 'Playlist' : d.kind === 'view' ? 'Saved view' : d.shared && !d.editable ? 'Shared collection' : 'Collection';
  const art = m.isTV ? 280 : m.isPhone ? 140 : 200;
  const imgs = d.items.slice(0, 4).map(i => i.image);
  return (
    <Screen contentStyle={{gap: m.space.lg}}>
      <View style={{paddingHorizontal: m.gutter, flexDirection: m.isPhone ? 'column' : 'row', gap: m.space.xl, alignItems: m.isPhone ? 'flex-start' : 'flex-end'}}>
        {!m.isTV ? <IconButton icon="back" label="Back" variant="secondary" onPress={back} size={36} /> : null}
        <View style={{width: art, height: art, borderRadius: m.radius.panel, overflow: 'hidden', flexDirection: 'row', flexWrap: 'wrap'}}>
          {imgs.length >= 4 ? imgs.map((img, i) => <Artwork key={i} source={img} title={d.title} width={art / 2} height={art / 2} radius={0} />) : <Artwork source={imgs[0]} title={d.title} width={art} height={art} radius={0} />}
        </View>
        <View style={{flex: m.isPhone ? undefined : 1, gap: 8}}>
          <View style={{flexDirection: 'row', gap: 8, alignItems: 'center'}}><Text variant="label" tone="accent">{label}</Text>{d.smart ? <Badge label="Updates automatically" tone="accent" /> : null}{d.shared ? <Badge label="Shared with everyone on this server" /> : null}</View>
          <Text variant="display">{d.title}</Text>
          <Text variant="body" tone="secondary">{d.items.length} items{d.totalDuration ? ` · ${formatDuration(d.totalDuration)}` : ''} · by {d.owner}</Text>
          {d.description ? <Text variant="body" tone="secondary">{d.description}</Text> : null}
          <ActionRow>
            <Button label="Play" icon="play" variant="primary" disabled={!d.items.length} onPress={() => void actions.play({id: d.id, kind: d.kind, title: d.title, shape: 'square'})} hasTVPreferredFocus />
            <Button label="Shuffle" icon="shuffle" disabled={!d.items.length} onPress={() => void actions.play({id: d.id, kind: d.kind, title: d.title, shape: 'square'}, {shuffle: true})} />
            {d.kind === 'view' ? <Button label="Open in library" icon="filter" variant="ghost" onPress={() => root('Library', {libraryId: 'lib-movies', view: 'browse'})} /> : null}
            {d.editable ? <Button icon="more" accessibilityLabel={`${label} options`} onPress={() => setSheet('menu')} /> : null}
          </ActionRow>
          {d.editable && d.kind !== 'view' ? <View style={{maxWidth: 520}}><Toggle label="Visible to everyone on this server" help={d.shared ? 'Every member can browse it; only you can change it.' : 'Only this profile can see it.'} value={d.shared} onChange={v => void client.saved.setShared(d.id, v)} /></View> : null}
        </View>
      </View>
      <View style={{paddingHorizontal: m.gutter}}>
        {d.items.length ? d.items.map((e, i) => (
          <ListRow key={`${e.id}-${i}`} left={<View style={{flexDirection: 'row', alignItems: 'center', gap: 10}}><Text variant="captionStrong" tone="tertiary" style={{width: m.isTV ? 40 : 24}}>{i + 1}</Text><Artwork source={e.image} title={e.title} width={e.shape === 'wide' ? (m.isTV ? 120 : 72) : m.isTV ? 64 : 44} height={e.shape === 'poster' ? (m.isTV ? 96 : 66) : e.shape === 'wide' ? (m.isTV ? 68 : 40) : m.isTV ? 64 : 44} radius={6} /></View>}
            title={e.title} subtitle={[e.subtitle, e.duration ? formatDuration(e.duration) : undefined].filter(Boolean).join(' · ')} onPress={() => cards.onPress(e)} onLongPress={() => cards.onLongPress(e)} chevron={false}
            trailing={d.editable && d.kind !== 'view' && !m.isTV ? (
              <View style={{flexDirection: 'row'}}>
                {d.kind === 'playlist' ? <IconButton icon="chevronUp" label="Move up" size={34} disabled={i === 0} onPress={() => void client.saved.moveInResource(d.id, e.id, -1)} /> : null}
                {d.kind === 'playlist' ? <IconButton icon="chevronDown" label="Move down" size={34} disabled={i === d.items.length - 1} onPress={() => void client.saved.moveInResource(d.id, e.id, 1)} /> : null}
                <IconButton icon="close" label={d.kind === 'playlist' ? 'Remove from playlist' : 'Remove from collection'} size={34} onPress={async () => { await client.saved.removeFromResource(d.id, e.id); toast({message: `Removed “${e.title}”.`}); }} />
              </View>
            ) : undefined} />
        )) : <EmptyState icon={d.kind === 'playlist' ? 'playlist' : 'collection'} title={d.kind === 'playlist' ? 'This playlist is empty' : d.kind === 'view' ? 'Nothing matches this view right now' : 'Nothing in this collection yet'} body={d.kind === 'playlist' ? (m.isTV ? 'Press and hold Select on a title and choose Add to playlist.' : 'Press and hold a title and choose Add to playlist.') : undefined} />}
      </View>
      <ActionMenu visible={sheet === 'menu'} onClose={() => setSheet(undefined)} title={d.title} items={[
        {id: 'rename', label: 'Rename…', icon: 'edit'},
        {id: 'edit', label: m.isTV ? 'Remove items…' : 'Edit items', icon: 'list', hidden: !m.isTV || d.kind === 'view'},
        {id: 'delete', label: d.kind === 'playlist' ? 'Delete playlist…' : d.kind === 'view' ? 'Delete saved view…' : 'Delete collection…', icon: 'trash', destructive: true},
      ]} onSelect={id => { setSheet(undefined); if (id === 'rename') { setName(d.title); setTimeout(() => setSheet('rename'), 250); } if (id === 'delete') setTimeout(() => setSheet('delete'), 250); if (id === 'edit') toast({message: 'Press and hold an item to remove it.'}); }} />
      <Sheet visible={sheet === 'rename'} onClose={() => setSheet(undefined)} title={`Rename ${label.toLowerCase()}`} size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={() => setSheet(undefined)} /><Button label="Save" variant="primary" loading={busy} disabled={!name.trim()} onPress={() => void run(async () => { await client.saved.renameResource(d.id, name.trim()); setSheet(undefined); })} /></>}>
        <TextField label="Name" value={name} onChangeText={setName} />
      </Sheet>
      <ConfirmDialog visible={sheet === 'delete'} onClose={() => setSheet(undefined)} title={`Delete ${d.title}?`} body={d.kind === 'playlist' ? 'The playlist is deleted for everyone it’s shared with. The titles in it stay in your libraries.' : d.kind === 'view' ? 'The saved view is deleted. The titles it found stay in your libraries.' : 'The collection is deleted for everyone who can see it. The titles in it stay in your libraries.'} confirmLabel="Delete" destructive busy={busy} onConfirm={() => void run(async () => { await client.saved.deleteResource(d.id); setSheet(undefined); back(); })} />
    </Screen>
  );
}
