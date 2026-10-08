import React, {useCallback} from 'react';
import {FlatList, StyleSheet, TVFocusGuideView, View} from 'react-native';
import {Card, aspectFor, type CardModel} from './Card';
import {Focusable} from './Focusable';
import {Icon} from './Icon';
import {EditorialTitle, Text} from './Text';
import {colors, isTV, radius, u} from './tokens';
import {useLayout} from './useLayout';
import {findNodeHandle} from 'react-native';
import {useRailTarget} from '../nav/Rail';

type Props = {
  title: string;
  subject?: string;
  eyebrow?: string;
  items: CardModel[];
  width?: number;
  onPressItem: (item: CardModel, index: number) => void;
  onLongPressItem?: (item: CardModel) => void;
  onFocusItem?: (item: CardModel) => void;
  onSeeAll?: () => void;
  seeAllLabel?: string;
  firstFocus?: boolean;
};

/**
 * A shelf. Row geometry comes from the first item's shape and never changes with contents.
 * TV rows remember their last focused card when focus comes back to them.
 */
export const Row = React.memo(function Row({title, subject, eyebrow, items, width, onPressItem, onLongPressItem, onFocusItem, onSeeAll, seeAllLabel = 'See all', firstFocus}: Props) {
  const layout = useLayout();
  const rail = useRailTarget();
  const railTag = isTV && rail ? findNodeHandle(rail) ?? undefined : undefined;
  const shape = items[0]?.shape ?? 'poster';
  const w = width ?? (shape === 'landscape' ? layout.landscapeWidth : shape === 'square' ? layout.squareWidth : shape === 'circle' ? Math.round(layout.posterWidth * 0.8) : layout.posterWidth);
  const gap = isTV ? 36 : 12;
  const renderItem = useCallback(
    ({item, index}: {item: CardModel; index: number}) => (
      <Card item={item} width={w} nextFocusLeft={index === 0 ? railTag : undefined} hasTVPreferredFocus={firstFocus && index === 0} onPress={() => onPressItem(item, index)} onLongPress={onLongPressItem ? () => onLongPressItem(item) : undefined} onFocus={onFocusItem ? () => onFocusItem(item) : undefined} />
    ),
    [w, onPressItem, onLongPressItem, onFocusItem, firstFocus, railTag],
  );
  if (!items.length) return null;
  const h = Math.round(w * aspectFor(shape));
  return (
    <View style={styles.row}>
      <View style={[styles.head, {paddingHorizontal: layout.gutter}]}>
        <View style={{flex: 1}}>
          {eyebrow ? <Text variant="eyebrow" tone="accent">{eyebrow}</Text> : null}
          <EditorialTitle lead={title} subject={subject} variant={isTV ? 'title' : 'title'} />
        </View>
        {onSeeAll && !isTV ? (
          <Focusable onPress={onSeeAll} accessibilityRole="button" accessibilityLabel={`${seeAllLabel}, ${title}`} style={styles.seeAll}>
            <Text variant="captionStrong" tone="accent">{seeAllLabel}</Text>
            <Icon name="chevronRight" size={14} color={colors.accentStrong} />
          </Focusable>
        ) : null}
      </View>
      <View>
        <FlatList
          horizontal
          data={items}
          keyExtractor={(it, i) => it.id + ':' + i}
          renderItem={renderItem}
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{paddingHorizontal: layout.gutter, paddingVertical: isTV ? 28 : 4}}
          ItemSeparatorComponent={() => <View style={{width: gap}} />}
          initialNumToRender={isTV ? 7 : 5}
          windowSize={5}
          removeClippedSubviews={!isTV}
          getItemLayout={(_, index) => ({length: w + gap, offset: layout.gutter + (w + gap) * index, index})}
          ListFooterComponent={
            onSeeAll && isTV ? (
              <View style={{marginLeft: gap}}>
                <Focusable onPress={onSeeAll} accessibilityRole="button" accessibilityLabel={`${seeAllLabel}, ${title}`} scale={1.08}>
                  {({focused}) => (
                    <View style={[styles.seeAllCard, {width: Math.round(w * 0.7), height: h}, focused && styles.seeAllFocused]}>
                      <Icon name="arrowRight" size={26} color={focused ? colors.actionInk : colors.text} />
                      <Text variant="captionStrong" color={focused ? colors.actionInk : colors.text} style={{marginTop: 12}}>{seeAllLabel}</Text>
                    </View>
                  )}
                </Focusable>
              </View>
            ) : null
          }
        />
      </View>
    </View>
  );
});

const styles = StyleSheet.create({
  row: {marginBottom: isTV ? 12 : 22},
  head: {flexDirection: 'row', alignItems: 'flex-end', marginBottom: isTV ? 0 : 8},
  seeAll: {flexDirection: 'row', alignItems: 'center', paddingVertical: 6, paddingLeft: 12, gap: 2},
  seeAllCard: {borderRadius: radius.md, backgroundColor: 'rgba(237,245,250,0.08)', alignItems: 'center', justifyContent: 'center', borderWidth: 1, borderColor: 'rgba(237,245,250,0.14)'},
  seeAllFocused: {backgroundColor: colors.white, borderColor: colors.white},
});
