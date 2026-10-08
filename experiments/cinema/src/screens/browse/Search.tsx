import React, {useEffect, useMemo, useState} from 'react';
import {ScrollView, StyleSheet, TVFocusGuideView, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {Channel, Item, Person, SearchGroup} from '../../data/types';
import {Button} from '../../design/Button';
import {Chip, ListRow, TextField} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {Row} from '../../design/Row';
import {EmptyState, RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {go, push, type Routes} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {openItemMenu} from '../shared/actions';
import {openItem, personCard} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {plural} from '../../design/format';

const FILTERS = [{id: 'all', label: 'All'}, {id: 'movies', label: 'Movies'}, {id: 'shows', label: 'Shows'}, {id: 'episodes', label: 'Episodes'}, {id: 'people', label: 'People'}, {id: 'artists', label: 'Artists'}, {id: 'albums', label: 'Albums'}, {id: 'songs', label: 'Songs'}, {id: 'books', label: 'Books'}, {id: 'channels', label: 'Channels'}];

function validate(q: string): string | null {
  const t = q.trim();
  if (!t) return null;
  if (t.replace(/[^a-z0-9]/gi, '').length < 2) return 'Type at least two letters or numbers to search.';
  if (t.split(/\s+/).length > 8) return 'Search with eight words or fewer.';
  if (t.length > 128) return 'Searches can be up to 128 characters.';
  return null;
}

export function SearchScreen() {
  const route = useRoute<RouteProp<Routes, 'Search'>>();
  const s = useServices();
  const [query, setQuery] = useState(route.params?.query ?? '');
  const [debounced, setDebounced] = useState(query);
  const [kind, setKind] = useState('all');
  useEffect(() => {
    if (route.params?.query) setQuery(route.params.query);
  }, [route.params?.query]);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), 300);
    return () => clearTimeout(t);
  }, [query]);
  const error = validate(debounced);
  const run = debounced.trim().length >= 2 && !error;
  const res = useQuery(run ? `search:${kind}:${debounced}` : null, () => s.catalog.search(debounced, kind));
  const recent = useQuery('recent-searches', () => s.catalog.recentSearches());
  useEffect(() => {
    if (run && res.data?.length) {
      const t = setTimeout(() => s.catalog.addRecentSearch(debounced.trim()), 1500);
      return () => clearTimeout(t);
    }
  }, [run, res.data, debounced, s]);
  const body = (
    <>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8), paddingVertical: u(10)}}>
        {FILTERS.map(f => <Chip key={f.id} label={f.label} active={kind === f.id} onPress={() => setKind(f.id)} />)}
      </ScrollView>
      {error ? <Text variant="caption" tone="amber" style={{marginVertical: 8}}>{error}</Text> : null}
      {!run ? (
        <View>
          {recent.data?.length ? (
            <View style={{marginTop: u(10)}}>
              <View style={{flexDirection: 'row', alignItems: 'center'}}>
                <Text variant="title" style={{flex: 1}}>Recent searches</Text>
                <Button label="Clear" size="sm" variant="ghost" onPress={() => s.catalog.clearRecentSearches()} />
              </View>
              {recent.data.map(r => <ListRow key={r} icon="history" label={r} onPress={() => setQuery(r)} compact />)}
            </View>
          ) : null}
          <EmptyState icon="search" title="Search your libraries" body="Movies, shows, episodes, music and books across every library on this server." compact />
        </View>
      ) : res.loading && !res.data ? (
        <RowsSkeleton rows={2} />
      ) : res.data && !res.data.length ? (
        <EmptyState icon="search" title={`No matches for “${debounced}”`} body={kind === 'all' ? 'Check the spelling or try a shorter search.' : `Nothing in ${FILTERS.find(f => f.id === kind)?.label}. Try searching everything.`} action={kind !== 'all' ? {label: 'Search everything', onPress: () => setKind('all')} : undefined} />
      ) : (
        <TVFocusGuideView autoFocus><Results groups={res.data ?? []} onSeeAll={setKind} kind={kind} /></TVFocusGuideView>
      )}
    </>
  );
  if (isTV) {
    return (
      <Screen scroll={false}>
        <View style={{flex: 1, flexDirection: 'row', gap: 16}}>
          <TVKeyboard value={query} onChange={setQuery} />
          {/* Inner left padding leaves room for a focused card's lift so it isn't clipped. */}
          <ScrollView style={{flex: 1}} contentContainerStyle={{paddingBottom: 80, paddingLeft: 44}}>
            <Text variant="eyebrow" tone="accent">Search</Text>
            <Text variant="hero" numberOfLines={1}>{query || 'Movies, shows, music…'}</Text>
            {body}
          </ScrollView>
        </View>
      </Screen>
    );
  }
  return (
    <Screen heading="Search">
      <TextField value={query} onChangeText={setQuery} placeholder="Movies, shows, music and more…" icon="search" autoCapitalize="none" />
      {body}
    </Screen>
  );
}

function Results({groups, onSeeAll, kind}: {groups: SearchGroup[]; onSeeAll: (k: string) => void; kind: string}) {
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const itemIds = groups.filter(g => g.kind !== 'people' && g.kind !== 'channels').flatMap(g => g.ids);
  const items = useItems(itemIds);
  const peopleIds = groups.find(g => g.kind === 'people')?.ids ?? [];
  const people = useQuery(peopleIds.length ? 'people:' + peopleIds.join(',') : null, () => Promise.all(peopleIds.map(id => s.catalog.person(id))));
  const chans = useQuery(groups.some(g => g.kind === 'channels') ? 'channels' : null, () => s.live.channels());
  return (
    <View style={{marginHorizontal: -layout.gutter}}>
      {groups.map(g => {
        const title = `${g.label}`;
        const seeAll = kind === 'all' && g.total > g.ids.length ? () => onSeeAll(g.kind) : undefined;
        if (g.kind === 'people') {
          const cards = (people.data ?? []).filter(Boolean).map(p => personCard(p as Person, (p as Person).knownFor));
          return <Row key={g.kind} title={title} eyebrow={`${g.total} ${g.total === 1 ? 'result' : 'results'}`} items={cards} onPressItem={c => push('Person', {id: c.id})} onSeeAll={seeAll} />;
        }
        if (g.kind === 'channels') {
          const list = (chans.data ?? []).filter(c => g.ids.includes(c.id)) as Channel[];
          return <Row key={g.kind} title={title} eyebrow={`${plural(g.total, 'result', 'results')}`} items={list.map(c => ({id: c.id, title: c.name, subtitle: `Channel ${c.number}`, art: c.logo, tint: c.tint, shape: 'landscape' as const}))} onPressItem={c => go('LivePlayer', {channelId: c.id})} onSeeAll={seeAll} />;
        }
        return (
          <Row key={g.kind} title={title} eyebrow={`${g.total} ${g.total === 1 ? 'result' : 'results'}`} items={items.cards(g.ids)} onSeeAll={seeAll} seeAllLabel="See all"
            onPressItem={c => { const it = items.lookup(c.id); if (it) openItem(it as Item); }}
            onLongPressItem={c => { const it = items.lookup(c.id); if (it) openItemMenu(s, it, {manager: isManager(session.role)}); }} />
        );
      })}
    </View>
  );
}

/** On-screen keyboard for the remote: letters, numbers, space, delete and clear. */
function TVKeyboard({value, onChange}: {value: string; onChange: (v: string) => void}) {
  const keys = useMemo(() => [...'abcdefghijklmnopqrstuvwxyz1234567890'.split('')], []);
  const key = (label: string, onPress: () => void, wide = false, first = false, icon?: 'delete' | 'space' | 'x') => (
    <Focusable key={label} onPress={onPress} hasTVPreferredFocus={first} scale={1.15} accessibilityLabel={label}>
      {({focused}) => (
        <View style={[styles.key, wide && {width: 136}, focused && {backgroundColor: colors.white}]}>
          {icon ? <Icon name={icon} size={16} color={focused ? colors.actionInk : colors.text} /> : <Text variant="heading" color={focused ? colors.actionInk : colors.text}>{label}</Text>}
        </View>
      )}
    </Focusable>
  );
  return (
    <TVFocusGuideView autoFocus style={styles.keyboard}>
      <View style={styles.queryBox}>
        <Icon name="search" size={16} color={colors.textTertiary} />
        <Text variant="bodyStrong" style={{marginLeft: 12, flex: 1}} numberOfLines={1}>{value || ' '}</Text>
      </View>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 10, width: 6 * 64 + 5 * 10}}>
        {keys.map((k, i) => key(k, () => onChange(value + k), false, i === 0))}
        {key('space', () => onChange(value + ' '), true, false, 'space')}
        {key('delete', () => onChange(value.slice(0, -1)), true, false, 'delete')}
        {key('clear', () => onChange(''), true, false, 'x')}
      </View>
    </TVFocusGuideView>
  );
}

const styles = StyleSheet.create({
  keyboard: {width: 6 * 64 + 5 * 10, paddingTop: 10},
  queryBox: {flexDirection: 'row', alignItems: 'center', height: 70, borderRadius: radius.md, backgroundColor: 'rgba(237,245,250,0.08)', paddingHorizontal: 18, marginBottom: 20, borderWidth: 1, borderColor: colors.line},
  key: {width: 64, height: 64, borderRadius: 14, backgroundColor: 'rgba(237,245,250,0.08)', alignItems: 'center', justifyContent: 'center'},
});
