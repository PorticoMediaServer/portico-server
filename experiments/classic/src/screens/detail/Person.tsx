import React, {useState} from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useQuery} from '../../data/DataProvider';
import {Screen, ErrorView} from '../../ui/Page';
import {Shelf} from '../../ui/Shelf';
import {EmptyState, ListRow, Loading, Tabs} from '../../ui/Controls';
import {Artwork} from '../../ui/Artwork';
import {Text} from '../../ui/Text';
import {IconButton} from '../../ui/Button';
import {Focusable} from '../../ui/Focusable';
import {useMetrics} from '../../theme/layout';
import {colors} from '../../theme/tokens';
import {useCardHandlers} from '../../app/useCards';
import {back} from '../../nav/nav';

const fmt = (d?: string) => (d ? new Date(d).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}) : undefined);

/** A person: photo, biography, Known for, and credits by cast and crew. */
export function PersonScreen() {
  const {params} = useRoute() as {params: {id: string}};
  const m = useMetrics();
  const q = useQuery(c => c.catalog.person(params.id), [params.id]);
  const cards = useCardHandlers();
  const [tab, setTab] = useState<'all' | 'cast' | 'crew'>('all');
  const [more, setMore] = useState(false);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  const p = q.data;
  const credits = p.credits.filter(c => tab === 'all' || c.role === tab);
  const size = m.isTV ? 260 : m.isPhone ? 140 : 200;
  return (
    <Screen contentStyle={{gap: m.rowGap}}>
      <View style={{paddingHorizontal: m.gutter, flexDirection: m.isPhone ? 'column' : 'row', gap: m.space.xl, alignItems: m.isPhone ? 'center' : 'flex-start'}}>
        {!m.isTV ? <View style={{position: 'absolute', left: m.gutter, top: 0, zIndex: 2}}><IconButton icon="back" label="Back" variant="secondary" onPress={back} size={36} /></View> : null}
        <Artwork source={p.image} title={p.name} width={size} height={size} round style={{borderWidth: 3, borderColor: colors.line}} />
        <View style={{flex: m.isPhone ? undefined : 1, gap: m.space.sm, alignItems: m.isPhone ? 'center' : 'flex-start'}}>
          <Text variant="label" tone="accent">{p.credits[0]?.role === 'crew' ? p.credits[0]?.job ?? 'Crew' : 'Actor'} · {p.credits.length} titles on this server</Text>
          <Text variant="display" center={m.isPhone}>{p.name}</Text>
          <Text variant="body" tone="secondary">{p.born ? (p.died ? `${fmt(p.born)} – ${fmt(p.died)}` : `Born ${fmt(p.born)}`) : ''}</Text>
          <Focusable onPress={() => setMore(v => !v)} style={({focused}) => ({borderRadius: 10, padding: m.isTV ? 8 : 0, backgroundColor: focused ? colors.raisedHigh : 'transparent', maxWidth: 900})}>
            <Text variant="body" tone="secondary" numberOfLines={more ? undefined : 4} center={m.isPhone}>{p.bio ?? 'No biography on this server yet.'}</Text>
          </Focusable>
        </View>
      </View>
      {p.knownFor.length ? <Shelf title="Known for" entries={p.knownFor} onPress={cards.onPress} onLongPress={cards.onLongPress} onFocus={cards.onFocus} first /> : null}
      <View style={{paddingHorizontal: m.gutter, gap: m.space.md}}>
        <Text variant="heading">Credits</Text>
        <Tabs tabs={[{value: 'all', label: 'All', count: p.credits.length}, {value: 'cast', label: 'Cast', count: p.credits.filter(c => c.role === 'cast').length}, {value: 'crew', label: 'Crew', count: p.credits.filter(c => c.role === 'crew').length}]} value={tab} onChange={setTab} />
        {credits.length ? credits.map((c, i) => (
          <ListRow key={`${c.entry.id}-${i}`} left={<Artwork source={c.entry.image} title={c.entry.title} width={m.isTV ? 64 : 44} height={m.isTV ? 96 : 66} radius={6} />} title={c.entry.title} subtitle={[c.entry.year, c.character ? `as ${c.character}` : c.job].filter(Boolean).join(' · ')} value={c.entry.watched ? 'Watched' : undefined} onPress={() => cards.onPress(c.entry)} onLongPress={() => cards.onLongPress(c.entry)} />
        )) : <EmptyState icon="user" title="No credits here" body={tab === 'cast' ? 'No acting credits on this server.' : 'No crew credits on this server.'} compact />}
      </View>
    </Screen>
  );
}
