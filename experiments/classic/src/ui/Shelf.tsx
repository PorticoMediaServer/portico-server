import React, {useCallback} from 'react';
import {FlatList, View, type ListRenderItem} from 'react-native';
import {useMetrics} from '../theme/layout';
import type {Entry, Shape} from '../data/types';
import {MediaCard, cardSize} from './Card';
import {Skeleton, SectionTitle} from './Controls';
import {usePrefs} from '../app/prefs';

type ShelfProps = {
  title: string;
  subtitle?: string;
  entries: Entry[];
  shape?: Shape;
  onPress: (e: Entry) => void;
  onLongPress?: (e: Entry) => void;
  onFocus?: (e: Entry) => void;
  onSeeAll?: () => void;
  total?: number;
  first?: boolean;
  gutter?: number;
};

/** A titled horizontal row of cards (Plex "hub"). */
export const Shelf = React.memo(function Shelf({title, subtitle, entries, shape, onPress, onLongPress, onFocus, onSeeAll, total, first, gutter}: ShelfProps) {
  const m = useMetrics();
  const g = gutter ?? m.gutter;
  const renderItem: ListRenderItem<Entry> = useCallback(({item, index}) => (
    <MediaCard entry={item} shape={shape ?? item.shape} onPress={onPress} onLongPress={onLongPress} onFocus={onFocus} hasTVPreferredFocus={first && index === 0} />
  ), [shape, onPress, onLongPress, onFocus, first]);
  return (
    <View style={{gap: m.isTV ? 18 : 10}}>
      <SectionTitle title={title} subtitle={subtitle} action={onSeeAll ? {label: total && total > entries.length ? `See all ${total}` : 'See all', onPress: onSeeAll} : undefined} style={{paddingHorizontal: g}} />
      <FlatList
        horizontal
        data={entries}
        keyExtractor={e => e.id}
        renderItem={renderItem}
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{paddingHorizontal: g, gap: m.gap, paddingVertical: m.isTV ? 14 : 2}}
        initialNumToRender={m.isTV ? 7 : 5}
        maxToRenderPerBatch={6}
        windowSize={5}
        removeClippedSubviews={!m.isTV}
        style={{marginVertical: m.isTV ? -14 : 0}}
      />
    </View>
  );
});

export function ShelfSkeleton({shape = 'poster', count = 8}: {shape?: Shape; count?: number}) {
  const m = useMetrics();
  const {cardScale} = usePrefs();
  const s = cardSize(m, shape, cardScale);
  return (
    <View style={{gap: m.isTV ? 18 : 10, paddingHorizontal: m.gutter}}>
      <Skeleton width={m.isTV ? 320 : 160} height={m.type.heading.size} />
      <View style={{flexDirection: 'row', gap: m.gap, overflow: 'hidden'}}>
        {Array.from({length: count}, (_, i) => (
          <View key={i} style={{gap: 8}}>
            <Skeleton width={s.w} height={s.h} radius={shape === 'circle' ? s.w / 2 : m.radius.card} />
            <Skeleton width={s.w * 0.7} height={m.type.caption.size} />
          </View>
        ))}
      </View>
    </View>
  );
}
