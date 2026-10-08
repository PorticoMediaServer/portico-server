import React, {createContext, useCallback, useContext, useMemo, useState} from 'react';
import {ScrollView, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useClient} from '../data/DataProvider';
import type {Entry} from '../data/types';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Text} from '../ui/Text';
import {Button} from '../ui/Button';
import {useToast} from './toast';
import {useSession} from './session';

type Sel = {
  active: boolean;
  selected: Map<string, Entry>;
  start: (e?: Entry) => void;
  toggle: (e: Entry) => void;
  selectAll: (list: Entry[]) => void;
  clear: () => void;
  isSelected: (id: string) => boolean;
};
const Ctx = createContext<Sel | null>(null);
const BarCtx = createContext<{setBulk: (b: Bulk | undefined) => void} | null>(null);
type Bulk = {addTo: (k: 'playlist' | 'collection', e: Entry[]) => void; edit: (e: Entry[]) => void; del: (e: Entry[]) => void};

/** Multi-select: long-press Select…, tap cards to add, then act on all of them. */
export function SelectionProvider({children}: {children: React.ReactNode}) {
  const [active, setActive] = useState(false);
  const [selected, setSelected] = useState<Map<string, Entry>>(new Map());
  const start = useCallback((e?: Entry) => { setActive(true); setSelected(e ? new Map([[e.id, e]]) : new Map()); }, []);
  const toggle = useCallback((e: Entry) => setSelected(prev => { const n = new Map(prev); if (n.has(e.id)) n.delete(e.id); else n.set(e.id, e); return n; }), []);
  const selectAll = useCallback((list: Entry[]) => setSelected(new Map(list.map(e => [e.id, e]))), []);
  const clear = useCallback(() => { setActive(false); setSelected(new Map()); }, []);
  const value = useMemo(() => ({active, selected, start, toggle, selectAll, clear, isSelected: (id: string) => selected.has(id)}), [active, selected, start, toggle, selectAll, clear]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
export const useSelection = () => useContext(Ctx)!;

/** The bulk-action bar docked at the bottom while selecting. */
export function SelectionBar({onAddTo, onEdit, onDelete, visibleEntries}: {onAddTo: (k: 'playlist' | 'collection', e: Entry[]) => void; onEdit: (e: Entry[]) => void; onDelete: (e: Entry[]) => void; visibleEntries?: Entry[]}) {
  const sel = useSelection();
  const client = useClient();
  const toast = useToast();
  const {isAdmin} = useSession();
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const [busy, setBusy] = useState<string>();
  if (!sel.active) return null;
  const list = [...sel.selected.values()];
  const ids = list.map(e => e.id);
  const run = async (key: string, fn: () => Promise<void>, msg: string) => {
    setBusy(key);
    await fn();
    setBusy(undefined);
    toast({message: msg, icon: 'check'});
  };
  const n = list.length;
  const anyWatchlisted = list.some(e => client.personal.get(e.id).watchlisted);
  const anyFav = list.some(e => client.personal.get(e.id).favorite);
  const actionButtons = (
    <>
      {visibleEntries?.length ? <Button size="sm" label={`Select all ${visibleEntries.length}`} variant="ghost" onPress={() => sel.selectAll(visibleEntries)} /> : null}
      <Button size="sm" icon="watched" label="Watched" disabled={!n} loading={busy === 'w'} onPress={() => void run('w', () => client.personal.setWatched(ids, true), `${n} updated.`)} />
      <Button size="sm" icon="eyeOff" label="Unwatched" disabled={!n} loading={busy === 'u'} onPress={() => void run('u', () => client.personal.setWatched(ids, false), `${n} updated.`)} />
      <Button size="sm" icon={anyWatchlisted ? 'savedFilled' : 'saved'} label={anyWatchlisted ? 'Remove from Watchlist' : 'Add to Watchlist'} disabled={!n} onPress={() => void run('wl', () => client.personal.setWatchlist(ids, !anyWatchlisted), `${n} updated.`)} />
      <Button size="sm" icon={anyFav ? 'heartFilled' : 'heart'} label={anyFav ? 'Remove from Favorites' : 'Add to Favorites'} disabled={!n} onPress={() => void run('f', () => client.personal.setFavorite(ids, !anyFav), `${n} updated.`)} />
      <Button size="sm" icon="playlist" label="Playlist" disabled={!n} onPress={() => onAddTo('playlist', list)} />
      <Button size="sm" icon="collection" label="Collection" disabled={!n} onPress={() => onAddTo('collection', list)} />
      {isAdmin ? <Button size="sm" icon="edit" label="Edit" disabled={!n} onPress={() => onEdit(list)} /> : null}
      {isAdmin ? <Button size="sm" icon="refresh" label="Refresh" disabled={!n} onPress={() => void run('r', () => client.catalog.refreshMetadata(ids), `${n} refreshes queued.`)} /> : null}
      {isAdmin ? <Button size="sm" icon="trash" label="Delete" variant="danger" disabled={!n} onPress={() => onDelete(list)} /> : null}
    </>
  );
  const frame = {position: 'absolute' as const, left: m.isTV ? 140 : 12, right: m.isTV ? 40 : 12, bottom: m.isTV ? 36 : insets.bottom + (m.isPhone ? 66 : 16), backgroundColor: colors.overlay, borderRadius: m.radius.sheet, borderWidth: 1, borderColor: colors.lineStrong, padding: m.space.sm, shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 24};
  const count = <Text variant="subheading">{n} selected</Text>;
  const done = <Button size="sm" label="Done" variant="primary" onPress={sel.clear} />;
  if (m.isPhone) {
    // One line of actions that scrolls sideways, so the bar never covers the page.
    return (
      <View style={[frame, {gap: m.space.sm}]}>
        <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingLeft: m.space.sm}}>{count}{done}</View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: m.space.sm}}>{actionButtons}</ScrollView>
      </View>
    );
  }
  return (
    <View style={[frame, {flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: m.space.sm}]}>
      <View style={{paddingHorizontal: m.space.sm, minWidth: m.isTV ? 160 : 90}}>{count}</View>
      {actionButtons}
      <View style={{flex: 1}} />
      {done}
    </View>
  );
}

export {BarCtx};
