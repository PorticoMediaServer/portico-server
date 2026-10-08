import React, {useState} from 'react';
import {ScrollView, StyleSheet, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useServices} from '../../data/hooks';
import {Button} from '../../design/Button';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {push} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {openAddToCollection, openAddToPlaylist} from './actions';
import {plural} from '../../design/format';

/** Bulk actions over a multi-selection (web: shared/SelectionBar). */
export function SelectionBar({ids, total, onSelectAll, onDone}: {ids: string[]; total: number; onSelectAll: () => void; onDone: () => void}) {
  const s = useServices();
  const session = useSession();
  const insets = useSafeAreaInsets();
  const [working, setWorking] = useState<string | null>(null);
  const none = ids.length === 0;
  const bulk = async (label: string, fn: () => Promise<unknown>) => {
    setWorking(`Preparing selection…`);
    await new Promise<void>(r => setTimeout(() => r(), 400));
    setWorking(`${ids.length} of ${ids.length} done…`);
    await fn();
    setWorking(null);
    ui.toast(`${ids.length} updated · ${label}.`);
  };
  const manager = isManager(session.role);
  return (
    <View style={[styles.bar, {bottom: isTV ? 30 : insets.bottom + 70}]}>
      <View style={{flexDirection: 'row', alignItems: 'center', marginBottom: u(8)}}>
        <Text variant="bodyStrong" style={{flex: 1}}>{working ?? `${ids.length} selected`}</Text>
        <Button label={`Select all ${total}`} size="sm" variant="ghost" onPress={onSelectAll} />
        <Button label="Done" size="sm" variant="primary" onPress={onDone} />
      </View>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8)}}>
        <Button label="Watched" icon="checkCircle" size="sm" disabled={none} onPress={() => bulk('marked watched', () => s.user.set(ids, {watched: true}))} />
        <Button label="Unwatched" icon="eyeOff" size="sm" disabled={none} onPress={() => bulk('marked unwatched', () => s.user.set(ids, {watched: false}))} />
        <Button label="Add to Watchlist" icon="bookmark" size="sm" disabled={none} onPress={() => bulk('added to Watchlist', () => s.user.set(ids, {watchlist: true}))} />
        <Button label="Remove from Watchlist" icon="bookmarkCheck" size="sm" disabled={none} onPress={() => bulk('removed from Watchlist', () => s.user.set(ids, {watchlist: false}))} />
        <Button label="Favorite" icon="heart" size="sm" disabled={none} onPress={() => bulk('added to Favorites', () => s.user.set(ids, {favorite: true}))} />
        <Button label="Unfavorite" icon="heart" size="sm" disabled={none} onPress={() => bulk('removed from Favorites', () => s.user.set(ids, {favorite: false}))} />
        <Button label="Playlist" icon="listMusic" size="sm" disabled={none} onPress={() => openAddToPlaylist(s, ids)} />
        <Button label="Collection" icon="layers" size="sm" disabled={none} onPress={() => openAddToCollection(s, ids)} />
        {manager ? <Button label="Edit" icon="edit" size="sm" disabled={none} onPress={() => push('MetadataEditor', {id: ids[0], ids})} /> : null}
        {manager ? <Button label="Refresh" icon="refresh" size="sm" disabled={none} onPress={() => ui.toast(`${ids.length} refreshes queued.`)} /> : null}
        {manager ? <Button label="Delete" icon="trash" size="sm" variant="danger" disabled={none} onPress={() => ui.confirm({title: `Delete ${plural(ids.length, 'title', 'titles')}?`, body: 'The files move to Deleted titles on the server and are removed for good after 30 days.', confirmLabel: 'Delete', destructive: true, typeToConfirm: String(ids.length), onConfirm: () => ui.toast(`${ids.length} moved to trash.`)})} /> : null}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {position: 'absolute', left: 12, right: 12, padding: u(12), borderRadius: radius.lg, backgroundColor: colors.overlay, borderWidth: 1, borderColor: colors.lineStrong, shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 20},
});
