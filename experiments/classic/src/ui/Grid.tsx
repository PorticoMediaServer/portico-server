import React, {forwardRef, useCallback, useImperativeHandle, useMemo, useRef, useState} from 'react';
import {FlatList, TVFocusGuideView, View, type ListRenderItem} from 'react-native';
import {gridFor, useMetrics} from '../theme/layout';
import type {Entry, Shape} from '../data/types';
import {MediaCard, cardSize} from './Card';
import {Skeleton} from './Controls';
import {usePrefs} from '../app/prefs';
import {useRegisterNav, useChromeInsets} from './Page';

export type GridHandle = {scrollToIndex: (i: number) => void};
type Props = {
  entries: Entry[];
  shape: Shape;
  onPress: (e: Entry) => void;
  onLongPress?: (e: Entry) => void;
  onFocus?: (e: Entry) => void;
  header?: React.ReactElement | null;
  footer?: React.ReactElement | null;
  empty?: React.ReactElement | null;
  onEndReached?: () => void;
  loading?: boolean;
  width: number;
  rightInset?: number;
};

/**
 * A virtualized grid of cards that fills the width exactly. It is also the
 * screen's scroll view, so large libraries only render what's visible.
 */
export const EntryGrid = forwardRef<GridHandle, Props>(function EntryGrid({entries, shape, onPress, onLongPress, onFocus, header, footer, empty, onEndReached, loading, width, rightInset = 0}, ref) {
  useRegisterNav();
  const m = useMetrics();
  const chrome = useChromeInsets();
  const {cardScale} = usePrefs();
  const base = cardSize(m, shape, cardScale);
  const inner = width - m.gutter * 2 - rightInset;
  const {columns, itemWidth} = gridFor(inner, base.w, m.gap);
  const itemH = Math.round((itemWidth * base.h) / base.w) + (m.isTV ? 78 : 50) + m.gap;
  const rows = useMemo(() => {
    const out: Entry[][] = [];
    for (let i = 0; i < entries.length; i += columns) out.push(entries.slice(i, i + columns));
    return out;
  }, [entries, columns]);
  const list = useRef<FlatList<Entry[]>>(null);
  const [headerH, setHeaderH] = useState(0);
  useImperativeHandle(ref, () => ({scrollToIndex: (i: number) => list.current?.scrollToOffset({offset: headerH + Math.floor(i / columns) * itemH, animated: true})}), [columns, itemH, headerH]);
  const renderRow: ListRenderItem<Entry[]> = useCallback(({item, index}) => (
    <View style={{flexDirection: 'row', gap: m.gap, paddingHorizontal: m.gutter, height: itemH}}>
      {item.map((e, j) => <MediaCard key={e.id} entry={e} shape={shape} width={itemWidth} onPress={onPress} onLongPress={onLongPress} onFocus={onFocus} hasTVPreferredFocus={index === 0 && j === 0 && m.isTV} />)}
    </View>
  ), [m.gap, m.gutter, m.isTV, itemH, shape, itemWidth, onPress, onLongPress, onFocus]);
  const content = (
    <FlatList
      ref={list}
      data={rows}
      keyExtractor={(r, i) => r[0]?.id ?? String(i)}
      renderItem={renderRow}
      getItemLayout={(_, i) => ({length: itemH, offset: headerH + itemH * i, index: i})}
      ListHeaderComponent={header ? <View onLayout={e => setHeaderH(e.nativeEvent.layout.height)}>{header}</View> : null}
      ListFooterComponent={loading ? <GridSkeleton columns={columns} itemWidth={itemWidth} shape={shape} /> : footer ?? null}
      ListEmptyComponent={!loading ? empty ?? null : null}
      onEndReached={onEndReached}
      onEndReachedThreshold={1.5}
      contentContainerStyle={{paddingTop: chrome.top, paddingBottom: chrome.bottom}}
      initialNumToRender={m.isTV ? 3 : 5}
      maxToRenderPerBatch={4}
      windowSize={m.isTV ? 5 : 7}
      removeClippedSubviews={!m.isTV}
      showsVerticalScrollIndicator={!m.isTV}
    />
  );
  return m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{content}</TVFocusGuideView> : content;
});

export function GridSkeleton({columns, itemWidth, shape, rows = 2}: {columns: number; itemWidth: number; shape: Shape; rows?: number}) {
  const m = useMetrics();
  const base = cardSize(m, shape);
  const h = Math.round((itemWidth * base.h) / base.w);
  return (
    <View style={{gap: m.gap, paddingHorizontal: m.gutter}}>
      {Array.from({length: rows}, (_, r) => (
        <View key={r} style={{flexDirection: 'row', gap: m.gap}}>
          {Array.from({length: columns}, (_, c) => <View key={c} style={{gap: 8}}><Skeleton width={itemWidth} height={h} radius={shape === 'circle' ? itemWidth / 2 : m.radius.card} /><Skeleton width={itemWidth * 0.7} height={12} /></View>)}
        </View>
      ))}
    </View>
  );
}
