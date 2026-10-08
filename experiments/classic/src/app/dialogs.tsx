import React, {useEffect, useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useQuery} from '../data/DataProvider';
import type {Entry} from '../data/types';
import {Sheet} from '../ui/Sheet';
import {Button} from '../ui/Button';
import {ListRow, Loading, Notice, Segmented, TextField, Toggle, Stepper} from '../ui/Controls';
import {Text} from '../ui/Text';
import {Focusable} from '../ui/Focusable';
import {Icon} from '../ui/Icon';
import {Artwork} from '../ui/Artwork';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {useToast} from './toast';
import {push} from '../nav/nav';

/** Add to playlist / Add to collection, with Create. */
export function PlaylistPicker({visible, kind, entries, onClose}: {visible: boolean; kind: 'playlist' | 'collection'; entries: Entry[]; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const m = useMetrics();
  const list = useQuery(c => (kind === 'playlist' ? c.saved.playlists() : c.saved.collections()), [kind, visible], ['saved']);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const {busy, run, error} = useAction();
  useEffect(() => { if (!visible) { setCreating(false); setName(''); } }, [visible]);
  const ids = entries.map(e => e.id);
  const label = kind === 'playlist' ? 'playlist' : 'collection';
  const add = (id: string, title: string) => void run(async () => {
    await (kind === 'playlist' ? client.saved.addToPlaylist(id, ids) : client.saved.addToCollection(id, ids));
    onClose();
    toast({message: ids.length > 1 ? `Added ${ids.length} items to ${title}.` : `Added to ${title}.`, icon: kind === 'playlist' ? 'playlist' : 'collection', action: {label: 'View', onPress: () => push('Resource', {kind, id})}});
  });
  const create = () => void run(async () => {
    const id = await (kind === 'playlist' ? client.saved.createPlaylist(name.trim(), ids) : client.saved.createCollection(name.trim(), ids));
    onClose();
    toast({message: `Created ${name.trim()}.`, action: {label: 'View', onPress: () => push('Resource', {kind, id})}});
  });
  return (
    <Sheet visible={visible} onClose={onClose} title={kind === 'playlist' ? 'Add to playlist' : 'Add to collection'} subtitle={entries.length === 1 ? entries[0]!.title : `${entries.length} items`} size="sm">
      {error ? <Notice tone="error" body={error} /> : null}
      {creating ? (
        <View style={{gap: m.space.md}}>
          <TextField label={`New ${label} name`} value={name} onChangeText={setName} autoFocus placeholder={kind === 'playlist' ? 'Late night drive' : 'Favourite heists'} onSubmitEditing={() => name.trim() && create()} />
          <View style={{flexDirection: 'row', gap: m.space.sm, justifyContent: 'flex-end'}}>
            <Button label="Cancel" variant="ghost" onPress={() => setCreating(false)} />
            <Button label={kind === 'playlist' ? 'Create playlist' : 'Create collection'} variant="primary" disabled={!name.trim()} loading={busy} onPress={create} />
          </View>
        </View>
      ) : (
        <>
          <ListRow icon="plus" title={kind === 'playlist' ? 'Create playlist' : 'Create collection'} onPress={() => setCreating(true)} chevron={false} hasTVPreferredFocus />
          {list.loading && !list.data ? <Loading label={`Loading ${label}s`} /> : null}
          {list.data?.map(r => (
            <ListRow key={r.id} left={<Artwork source={r.image} title={r.title} width={m.isTV ? 56 : 40} height={m.isTV ? 56 : 40} radius={6} />} title={r.title} subtitle={`${r.count} item${r.count === 1 ? '' : 's'}${r.shared ? ' · Shared' : ''}`} onPress={() => add(r.id, r.title)} chevron={false} />
          ))}
          {list.data && !list.data.length ? <Text variant="caption" tone="tertiary">{kind === 'playlist' ? 'No playlists yet.' : 'You don’t have any collections you can add to yet.'}</Text> : null}
        </>
      )}
    </Sheet>
  );
}

/** Five-star rating (Rate…). */
export function RatingSheet({visible, entry, onClose}: {visible: boolean; entry: Entry; onClose: () => void}) {
  const client = useClient();
  const m = useMetrics();
  const current = client.personal.get(entry.id).rating;
  const [value, setValue] = useState(current ?? 0);
  useEffect(() => setValue(current ?? 0), [current, visible]);
  const save = async (v: number | undefined) => { await client.personal.rate(entry.id, v); onClose(); };
  return (
    <Sheet visible={visible} onClose={onClose} title={`Rate ${entry.title}`} size="sm" footer={<><Button label="Clear rating" variant="ghost" onPress={() => void save(undefined)} disabled={!current} /><Button label="Save" variant="primary" onPress={() => void save(value || undefined)} /></>}>
      <View style={{flexDirection: 'row', justifyContent: 'center', gap: m.space.sm, paddingVertical: m.space.md}}>
        {[1, 2, 3, 4, 5].map(n => (
          <Focusable key={n} onPress={() => setValue(n)} onFocus={() => m.isTV && setValue(n)} focusScale={1.15} hasTVPreferredFocus={n === (current ?? 3)} accessibilityLabel={`${n} star${n === 1 ? '' : 's'}`}
            style={({focused}) => ({padding: 6, borderRadius: 40, backgroundColor: focused ? colors.raisedBright : 'transparent'})}>
            <Icon name={n <= value ? 'starFilled' : 'star'} size={m.isTV ? 56 : 38} color={n <= value ? colors.amber : colors.textTertiary} />
          </Focusable>
        ))}
      </View>
      <Text variant="caption" tone="tertiary" center>{value ? `Rated ${value} star${value === 1 ? '' : 's'}` : 'Not rated'}</Text>
    </Sheet>
  );
}

/** Download… with quality choice, and whole-show rules for shows. */
export function DownloadSheet({visible, entry, onClose}: {visible: boolean; entry: Entry; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [quality, setQuality] = useState('1080p');
  const [policy, setPolicy] = useState<'all' | 'unwatched' | 'next'>('next');
  const [keep, setKeep] = useState(3);
  const {busy, run, error} = useAction();
  const show = entry.kind === 'show';
  const options = [{value: 'Original', label: 'Original', sub: 'The file as it is · about 2.1 GB'}, {value: '1080p', label: '1080p', sub: 'About 1.4 GB · the server prepares it first'}, {value: '720p', label: '720p', sub: 'About 640 MB · the server prepares it first'}, {value: 'Smaller', label: 'Smaller', sub: 'About 320 MB'}];
  const go = () => void run(async () => {
    if (show) await client.downloads.addShow(entry.id, policy, keep);
    else await client.downloads.add(entry.id, quality);
    onClose();
    toast({message: show ? 'Whole-show download added. Episodes are prepared as they become due.' : 'Added to your downloads.', icon: 'download', action: {label: 'View downloads', onPress: () => push('Downloads')}});
  });
  return (
    <Sheet visible={visible} onClose={onClose} title={show ? 'Download all' : 'Download'} subtitle={entry.title} size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Download" icon="download" variant="primary" loading={busy} onPress={go} /></>}>
      {error ? <Notice tone="error" body={error} /> : null}
      {show ? (
        <>
          <Text variant="captionStrong" tone="secondary">Whole shows and seasons</Text>
          <Segmented value={policy} onChange={setPolicy} options={[{value: 'all', label: 'All episodes'}, {value: 'unwatched', label: 'Unwatched only'}, {value: 'next', label: 'Next unwatched'}]} />
          {policy !== 'all' ? <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'}}><Text variant="body">Keep the next episodes</Text><Stepper label="Episodes" value={keep} onChange={setKeep} min={1} max={10} /></View> : null}
        </>
      ) : options.map(o => <ListRow key={o.value} title={o.label} subtitle={o.sub} selected={quality === o.value} onPress={() => setQuality(o.value)} chevron={false} icon="quality" />)}
      <Text variant="caption" tone="tertiary">214 GB of download space left on the server.</Text>
    </Sheet>
  );
}

/** Delete… with the file-removal choice and typed confirmation. */
export function DeleteMediaSheet({visible, entries, onClose}: {visible: boolean; entries: Entry[]; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [removeFiles, setRemoveFiles] = useState(true);
  const [typed, setTyped] = useState('');
  const {busy, run} = useAction();
  useEffect(() => { if (!visible) setTyped(''); }, [visible]);
  const one = entries.length === 1;
  const token = one ? entries[0]!.title : String(entries.length);
  const go = () => void run(async () => {
    await client.catalog.deleteMedia(entries.map(e => e.id), removeFiles);
    onClose();
    toast({message: removeFiles ? 'Removed. The files are kept for 30 days under Server › Maintenance.' : 'Removed.', icon: 'trash'});
  });
  return (
    <Sheet visible={visible} onClose={onClose} title={one ? `Delete “${entries[0]?.title}”?` : `Delete ${entries.length} titles?`} size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Delete" variant="danger" disabled={typed.trim() !== token} loading={busy} onPress={go} /></>}>
      <Text variant="body" tone="secondary">{entries.length} file{entries.length === 1 ? '' : 's'}, {(entries.length * 2.3).toFixed(1)} GB.</Text>
      <Toggle label="Also remove the files from disk" help={removeFiles ? 'The files are kept for 30 days before they’re gone for good.' : 'Only Portico’s record is removed. The next scan finds the files again.'} value={removeFiles} onChange={setRemoveFiles} />
      <TextField label={one ? 'Type the title to confirm' : `Type ${entries.length} to confirm`} value={typed} onChangeText={setTyped} placeholder={token} autoCapitalize="none" />
    </Sheet>
  );
}
