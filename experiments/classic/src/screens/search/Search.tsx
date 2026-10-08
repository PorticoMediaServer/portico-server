import React, {useEffect, useRef, useState} from 'react';
import {ScrollView, TextInput, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useQuery} from '../../data/DataProvider';
import type {Entry, SearchGroupId} from '../../data/types';
import {Screen} from '../../ui/Page';
import {Shelf, ShelfSkeleton} from '../../ui/Shelf';
import {MediaCard} from '../../ui/Card';
import {Chip, EmptyState, TextField} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Button} from '../../ui/Button';
import {Focusable} from '../../ui/Focusable';
import {Icon} from '../../ui/Icon';
import {colors} from '../../theme/tokens';
import {gridFor, useMetrics} from '../../theme/layout';
import {useContentWidth} from '../../ui/useWidth';
import {useCardHandlers} from '../../app/useCards';
import {useSession} from '../../app/session';

const SCOPES: {id: 'all' | SearchGroupId; label: string}[] = [{id: 'all', label: 'All'}, {id: 'movies', label: 'Movies'}, {id: 'shows', label: 'Shows'}, {id: 'episodes', label: 'Episodes'}, {id: 'people', label: 'People'}, {id: 'artists', label: 'Artists'}, {id: 'albums', label: 'Albums'}, {id: 'songs', label: 'Songs'}, {id: 'books', label: 'Books'}, {id: 'live', label: 'Live TV'}];

/** Search every library: grouped results, scopes, recent searches, and an on-screen keyboard on TV. */
export function SearchScreen() {
  const route = useRoute() as {params?: {query?: string}};
  const m = useMetrics();
  const client = useClient();
  const {isAdmin} = useSession();
  const [text, setText] = useState(route.params?.query ?? '');
  const [q, setQ] = useState(text);
  const [scope, setScope] = useState<'all' | SearchGroupId>('all');
  const [recent, setRecent] = useState(client.search.recent());
  const cards = useCardHandlers();
  const [width, onLayout] = useContentWidth();
  useEffect(() => { const t = setTimeout(() => setQ(text), 260); return () => clearTimeout(t); }, [text]);
  const valid = q.trim().replace(/[^\p{L}\p{N}]/gu, '').length >= 2;
  const res = useQuery(c => (valid ? c.search.query(q, scope === 'all' ? undefined : scope) : Promise.resolve(undefined)), [q, scope, valid]);
  useEffect(() => { if (res.data?.groups.length) { client.search.remember(q); setRecent(client.search.recent()); } }, [res.data]); // eslint-disable-line react-hooks/exhaustive-deps
  const groups = res.data?.groups ?? [];
  const scoped = scope !== 'all' ? groups[0] : undefined;
  const {columns, itemWidth} = gridFor(width - m.gutter * 2, scoped?.entries[0]?.shape === 'wide' ? m.wide.w : scoped?.entries[0]?.shape === 'square' ? m.square : m.poster.w, m.gap);
  return (
    <View style={{flex: 1}} onLayout={onLayout}>
      <Screen contentStyle={{gap: m.space.lg}} keyboardShouldPersistTaps="handled">
        <View style={{paddingHorizontal: m.gutter, gap: m.space.md}}>
          <Text variant="display">Search</Text>
          {m.isTV ? <TvKeyboard value={text} onChange={setText} /> : <TextField icon="search" placeholder="Movies, shows, music and more…" value={text} onChangeText={setText} autoFocus={!text} returnKeyType="search" clearButtonMode="while-editing" autoCorrect={false} />}
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: m.space.xs, paddingVertical: m.isTV ? 8 : 0}}>
            {SCOPES.map(s => <Chip key={s.id} label={s.label} selected={scope === s.id} onPress={() => setScope(s.id)} />)}
          </ScrollView>
        </View>
        {!text.trim() ? (
          recent.length ? (
            <View style={{paddingHorizontal: m.gutter, gap: m.space.sm}}>
              <View style={{flexDirection: 'row', alignItems: 'center'}}><Text variant="heading" style={{flex: 1}}>Recent searches</Text><Button label="Clear" size="sm" variant="ghost" onPress={() => { client.search.clearRecent(); setRecent([]); }} /></View>
              <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.xs}}>{recent.map(r => <Chip key={r} icon="history" label={r} onPress={() => setText(r)} />)}</View>
            </View>
          ) : <EmptyState icon="search" title="Search your libraries" body="Movies, shows, episodes, music and books across every library on this server." />
        ) : !valid ? <Text variant="body" tone="tertiary" style={{paddingHorizontal: m.gutter}}>Type at least two letters or numbers to search.</Text>
          : res.loading && !res.data ? <><ShelfSkeleton /><ShelfSkeleton shape="circle" /></>
          : !groups.length ? (
            <EmptyState icon="search" title={`No matches for “${q}”`} body={scope === 'all' ? 'Check the spelling or try a shorter search.' : `Nothing in ${SCOPES.find(s => s.id === scope)?.label}. Try searching everything.`} action={scope !== 'all' ? {label: 'Search everything', onPress: () => setScope('all')} : isAdmin ? undefined : undefined} />
          ) : scoped ? (
            <View style={{paddingHorizontal: m.gutter, gap: m.space.md}}>
              <Text variant="caption" tone="tertiary">{scoped.total} result{scoped.total === 1 ? '' : 's'}{scoped.total > scoped.entries.length ? ` · showing ${scoped.entries.length}` : ''}</Text>
              <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.gap}}>
                {scoped.entries.map(e => <MediaCard key={e.id} entry={e} width={itemWidth} onPress={cards.onPress} onLongPress={cards.onLongPress} />)}
              </View>
              {columns ? null : null}
            </View>
          ) : (
            <View style={{gap: m.rowGap}}>
              {groups.map(g => <Shelf key={g.id} title={g.title} subtitle={g.total > g.entries.length ? `${g.entries.length} of ${g.total}` : undefined} entries={g.entries} onPress={cards.onPress} onLongPress={cards.onLongPress} onSeeAll={g.total > 4 ? () => setScope(g.id) : undefined} total={g.total} />)}
            </View>
          )}
      </Screen>
    </View>
  );
}

/** On-screen keyboard for the remote, with the system keyboard as an option. */
function TvKeyboard({value, onChange}: {value: string; onChange: (v: string) => void}) {
  const input = useRef<TextInput>(null);
  const keys = 'abcdefghijklmnopqrstuvwxyz1234567890'.split('');
  const key = (label: string, onPress: () => void, w = 58, icon?: 'back' | 'close') => (
    <Focusable key={label} onPress={onPress} focusScale={1.12} accessibilityLabel={label === ' ' ? 'Space' : label}
      style={({focused}) => ({width: w, height: 58, borderRadius: 12, alignItems: 'center', justifyContent: 'center', backgroundColor: focused ? colors.focus : colors.raisedHigh})}>
      {({focused}) => (icon ? <Icon name={icon} size={24} color={focused ? colors.accentInk : colors.text} /> : <Text variant="subheading" color={focused ? colors.accentInk : colors.text}>{label === ' ' ? 'space' : label.toUpperCase()}</Text>)}
    </Focusable>
  );
  return (
    <View style={{flexDirection: 'row', gap: 40, alignItems: 'flex-start'}}>
      <View style={{width: 58 * 9 + 8 * 8, flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>
        {keys.map(k => key(k, () => onChange(value + k)))}
        {key(' ', () => onChange(value + ' '), 58 * 3 + 16)}
        {key('Delete', () => onChange(value.slice(0, -1)), 58 * 2 + 8, 'back')}
        {key('Clear', () => onChange(''), 58 * 2 + 8, 'close')}
      </View>
      <View style={{flex: 1, gap: 16}}>
        <View style={{height: 72, borderRadius: 16, backgroundColor: colors.raisedSoft, borderWidth: 2, borderColor: colors.lineControl, justifyContent: 'center', paddingHorizontal: 24, flexDirection: 'row', alignItems: 'center', gap: 14}}>
          <Icon name="search" size={30} color={colors.textTertiary} />
          <Text variant="heading" tone={value ? 'primary' : 'tertiary'} style={{flex: 1}} numberOfLines={1}>{value || 'Movies, shows, music and more…'}</Text>
        </View>
        <Button label="Use the system keyboard" icon="keyboard" variant="ghost" onPress={() => input.current?.focus()} />
        <TextInput ref={input} value={value} onChangeText={onChange} style={{width: 1, height: 1, opacity: 0}} />
      </View>
    </View>
  );
}

export type {Entry};
