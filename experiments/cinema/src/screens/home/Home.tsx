import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Animated, FlatList, NativeScrollEvent, NativeSyntheticEvent, ScrollView, StyleSheet, TVFocusGuideView, View, useWindowDimensions} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useQuery, useServices} from '../../data/hooks';
import type {Item, Row as RowModel} from '../../data/types';
import {Backdrop} from '../../design/Backdrop';
import {Button} from '../../design/Button';
import type {CardModel} from '../../design/Card';
import {glow} from '../../design/Glow';
import {Row} from '../../design/Row';
import {Banner, EmptyState, ErrorState, RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Header, useChromeInsets} from '../../nav/Screen';
import {go, push} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {openItemMenu, useItemState, type Origin} from '../shared/actions';
import {openItem, playItem} from '../shared/cards';
import {HeroInfo} from '../shared/HeroInfo';
import {useItems} from '../shared/useItems';

const originFor = (row: RowModel): Origin => (row.kind === 'continue' ? 'continue' : row.recommendation ? 'recommendation' : 'default');

export function HomeScreen() {
  const s = useServices();
  const session = useSession();
  const rows = useQuery('home', () => s.catalog.home());
  const allIds = useMemo(() => [...new Set((rows.data ?? []).flatMap(r => r.itemIds))], [rows.data]);
  const items = useItems(rows.data ? allIds : undefined, 'home:' + allIds.length + ':' + allIds.slice(0, 40).join(','));
  const manager = isManager(session.role);
  if (rows.error) return <View style={{flex: 1, justifyContent: 'center'}}><ErrorState onRetry={rows.reload} /></View>;
  if (!rows.data || items.loading && !items.map.size) return <HomeLoading />;
  if (!rows.data.length) return <HomeEmptyScreen />;
  const featured = rows.data.find(r => r.kind === 'featured');
  const shelves = rows.data.filter(r => r.kind !== 'featured');
  return isTV ? <TVHome featured={featured} shelves={shelves} items={items} manager={manager} /> : <TouchHome featured={featured} shelves={shelves} items={items} manager={manager} />;
}

function HomeLoading() {
  const chrome = useChromeInsets();
  return (
    <View style={{flex: 1, paddingLeft: chrome.left}}>
      <RowsSkeleton rows={isTV ? 2 : 3} hero />
    </View>
  );
}

type Items = ReturnType<typeof useItems>;

function useRowHandlers(rowsById: Map<string, RowModel>, items: Items, manager: boolean) {
  const s = useServices();
  const press = useCallback((card: CardModel) => {
    const it = items.lookup(card.id);
    if (it) openItem(it);
  }, [items]);
  const longPress = useCallback((rowId: string) => (card: CardModel) => {
    const it = items.lookup(card.id);
    const row = rowsById.get(rowId);
    if (it) openItemMenu(s, it, {origin: row ? originFor(row) : 'default', manager});
  }, [items, rowsById, s, manager]);
  return {press, longPress};
}

// ---------------------------------------------------------------- TV: the Marquee

/**
 * TV Home is a marquee: the top of the screen is always the focused title, full-bleed, in the
 * display serif; the shelves live in the lower half and slide up one at a time as you move down.
 */
function TVHome({featured, shelves, items, manager}: {featured?: RowModel; shelves: RowModel[]; items: Items; manager: boolean}) {
  const {height} = useWindowDimensions();
  const layout = useLayout();
  const first = featured?.itemIds[0] ?? shelves[0]?.itemIds[0];
  const [focusId, setFocusId] = useState<string | undefined>(first);
  const [eyebrow, setEyebrow] = useState<string>(featured ? 'Featured' : shelves[0]?.title ?? '');
  const focusItem = focusId ? items.lookup(focusId) : undefined;
  const show = focusItem?.kind === 'episode' ? items.lookup(focusItem.showId) : undefined;
  const rowsById = useMemo(() => new Map([...(featured ? [featured] : []), ...shelves].map(r => [r.id, r])), [featured, shelves]);
  const {press, longPress} = useRowHandlers(rowsById, items, manager);
  useEffect(() => {
    if (focusItem) glow.setTint(focusItem.tint);
  }, [focusItem]);
  const heroH = Math.round(height * 0.52);
  const all = featured ? [featured, ...shelves] : shelves;
  const listRef = useRef<FlatList<RowModel>>(null);
  const onFocus = useCallback((row: RowModel, index: number) => (card: CardModel) => {
    listRef.current?.scrollToIndex({index, animated: true, viewOffset: 0});
    setFocusId(card.id);
    setEyebrow(row.kind === 'featured' ? 'Featured' : row.subject ? `${row.title} ${row.subject}` : row.title);
  }, []);
  const backdropArt = focusItem ? focusItem.art.backdrop ?? show?.art.backdrop ?? focusItem.art.still ?? focusItem.art.square : undefined;
  return (
    <View style={{flex: 1}}>
      <Backdrop art={backdropArt} style={{left: layout.railWidth * 0.4}} bottom={1} top={0.35} />
      <View style={[styles.marqueeInfo, {height: heroH, paddingLeft: layout.railWidth + layout.gutter}]} pointerEvents="none">
        {focusItem ? <HeroInfo item={show && focusItem.kind === 'episode' ? focusItem : focusItem} eyebrow={eyebrow} lookup={items.lookup} /> : null}
        {focusItem?.kind === 'episode' && show ? <Text variant="captionStrong" tone="accent" style={{marginTop: 10}}>{show.title}</Text> : null}
      </View>
      <View style={{position: 'absolute', left: layout.railWidth, right: 0, top: heroH - 10, bottom: 0}}>
        <TVFocusGuideView autoFocus style={{flex: 1}}>
          <FlatList
            ref={listRef}
            onScrollToIndexFailed={() => {}}
            data={[...all, {id: '__customise'} as RowModel]}
            keyExtractor={r => r.id}
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{paddingBottom: height * 0.4}}
            windowSize={5}
            renderItem={({item: row, index}) =>
              row.id === '__customise' ? (
                <View style={{paddingHorizontal: layout.gutter, paddingTop: 20, flexDirection: 'row', gap: 20}}>
                  <Button label="Customize Home" icon="edit" onPress={() => push('HomeCustomise')} />
                  <Button label="Screen index" icon="list" variant="ghost" onPress={() => go('ScreenIndex')} />
                </View>
              ) : (
                <Row
                  title={row.title}
                  subject={row.subject}
                  items={items.cards(row.itemIds)}
                  onPressItem={press}
                  onLongPressItem={longPress(row.id)}
                  onFocusItem={onFocus(row, index)}
                  onSeeAll={row.kind !== 'featured' ? () => push('HomeRow', {rowId: row.id}) : undefined}
                  firstFocus={index === 0}
                />
              )
            }
          />
        </TVFocusGuideView>
      </View>
    </View>
  );
}

// ---------------------------------------------------------------- Touch: hero carousel + shelves

function TouchHome({featured, shelves, items, manager}: {featured?: RowModel; shelves: RowModel[]; items: Items; manager: boolean}) {
  const layout = useLayout();
  const chrome = useChromeInsets();
  const insets = useSafeAreaInsets();
  const rowsById = useMemo(() => new Map(shelves.map(r => [r.id, r])), [shelves]);
  const {press, longPress} = useRowHandlers(rowsById, items, manager);
  const session = useSession();
  const scrollY = useRef(new Animated.Value(0)).current;
  const headerOpacity = scrollY.interpolate({inputRange: [0, 240], outputRange: [0, 1], extrapolate: 'clamp'});
  return (
    <View style={{flex: 1, paddingLeft: chrome.left}}>
      <Animated.ScrollView
        onScroll={Animated.event([{nativeEvent: {contentOffset: {y: scrollY}}}], {useNativeDriver: true})}
        scrollEventThrottle={16}
        contentContainerStyle={{paddingBottom: chrome.bottom + 30}}
        showsVerticalScrollIndicator={false}>
        {featured ? <HeroCarousel row={featured} items={items} /> : <View style={{height: insets.top + 60}} />}
        {session.role === 'owner' && !layout.compact ? null : null}
        {shelves.map(row => (
          <Row
            key={row.id}
            title={row.title}
            subject={row.subject}
            items={items.cards(row.itemIds)}
            onPressItem={press}
            onLongPressItem={longPress(row.id)}
            onSeeAll={() => push('HomeRow', {rowId: row.id})}
          />
        ))}
        <View style={{paddingHorizontal: layout.gutter, paddingTop: 8, flexDirection: 'row', gap: 10, flexWrap: 'wrap'}}>
          <Button label="Customize Home" icon="edit" onPress={() => push('HomeCustomise')} />
        </View>
      </Animated.ScrollView>
      <Animated.View pointerEvents="none" style={[StyleSheet.absoluteFill, {bottom: undefined, height: insets.top + 56, backgroundColor: 'rgba(4,10,16,0.9)', opacity: headerOpacity, left: chrome.left}]} />
      <View style={[StyleSheet.absoluteFill, {bottom: undefined, left: chrome.left}]} pointerEvents="box-none">
        <Header transparent showBack={false} />
      </View>
    </View>
  );
}

function HeroCarousel({row, items}: {row: RowModel; items: Items}) {
  const layout = useLayout();
  const insets = useSafeAreaInsets();
  const w = layout.width - layout.railWidth;
  const h = layout.compact ? Math.round(w * 1.28) : Math.round(Math.min(620, w * 0.56));
  const [page, setPage] = useState(0);
  const list = row.itemIds.map(id => items.lookup(id)).filter((i): i is Item => !!i);
  const ref = useRef<ScrollView>(null);
  useEffect(() => {
    if (list[page]) glow.setTint(list[page].tint);
  }, [page, list]);
  useEffect(() => {
    const t = setInterval(() => {
      setPage(p => {
        const n = (p + 1) % Math.max(1, list.length);
        ref.current?.scrollTo({x: n * w, animated: true});
        return n;
      });
    }, 7000);
    return () => clearInterval(t);
  }, [list.length, w]);
  const onEnd = (e: NativeSyntheticEvent<NativeScrollEvent>) => setPage(Math.round(e.nativeEvent.contentOffset.x / w));
  return (
    <View style={{height: h}}>
      <ScrollView ref={ref} horizontal pagingEnabled showsHorizontalScrollIndicator={false} onMomentumScrollEnd={onEnd}>
        {list.map(it => (
          <HeroPage key={it.id} item={it} width={w} height={h} topInset={insets.top} />
        ))}
      </ScrollView>
      <View style={styles.dots}>
        {list.map((it, i) => <View key={it.id} style={[styles.dot, i === page && styles.dotOn]} />)}
      </View>
    </View>
  );
}

function HeroPage({item, width, height, topInset}: {item: Item; width: number; height: number; topInset: number}) {
  const layout = useLayout();
  const st = useItemState(item.id);
  const s = useServices();
  return (
    <View style={{width, height}}>
      <Backdrop art={item.art.backdrop ?? item.art.poster} left={!layout.compact} bottom={1} top={0.5} />
      <View style={[styles.heroText, {paddingHorizontal: layout.gutter, paddingTop: topInset + 60}, layout.compact ? {justifyContent: 'flex-end'} : {justifyContent: 'center', maxWidth: 560}]}>
        <HeroInfo item={item} eyebrow={item.kind === 'show' ? 'Featured series' : 'Featured'} compact={false}>
          <View style={styles.heroButtons}>
            <Button label={st.progress > 0 ? 'Resume' : 'Play'} icon="play" variant="primary" onPress={() => playItem(item, {from: st.position})} />
            <Button icon={st.watchlist ? 'bookmarkCheck' : 'plus'} accessibilityLabel={st.watchlist ? 'Remove from Watchlist' : 'Add to Watchlist'} onPress={() => s.user.set([item.id], {watchlist: !st.watchlist})} />
            <Button icon="info" accessibilityLabel="Details" onPress={() => openItem(item)} />
          </View>
        </HeroInfo>
      </View>
    </View>
  );
}

export function HomeEmptyScreen() {
  const session = useSession();
  const chrome = useChromeInsets();
  const owner = session.role === 'owner';
  return (
    <View style={{flex: 1, paddingLeft: chrome.left, justifyContent: 'center'}}>
      <EmptyState
        icon="film"
        title="Nothing here yet"
        body={owner ? 'Titles appear here once your libraries have been scanned.' : 'Titles appear here once the owner adds media to the libraries you can see.'}
        action={owner ? {label: 'Open library settings', icon: 'settings', onPress: () => go('Admin', {section: 'libraries'})} : undefined}
      />
    </View>
  );
}

export function CustodyBanner() {
  return <Banner icon="shieldCheck" tone="info" title="Accept ownership of this server’s Portico registration" body="You’re the owner of Ehler Home Theater now. Accept so your Portico Account holds its registration." />;
}

const styles = StyleSheet.create({
  marqueeInfo: {position: 'absolute', top: 0, left: 0, right: 0, justifyContent: 'flex-end', paddingBottom: 50, paddingRight: 400},
  heroText: {...StyleSheet.absoluteFillObject, paddingBottom: 36},
  heroButtons: {flexDirection: 'row', gap: 10, marginTop: 16},
  dots: {position: 'absolute', bottom: 12, alignSelf: 'center', flexDirection: 'row', gap: 6},
  dot: {width: 6, height: 6, borderRadius: 3, backgroundColor: 'rgba(237,245,250,0.3)'},
  dotOn: {width: 18, backgroundColor: colors.accentStrong},
});

export {u};
