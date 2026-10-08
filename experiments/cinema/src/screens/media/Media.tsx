import React, {useEffect, useRef, useState} from 'react';
import {Animated, FlatList, Image, Pressable, StyleSheet, TVFocusGuideView, View, useTVEventHandler, useWindowDimensions} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {resolveArt} from '../../data/art';
import {useQuery, useServices} from '../../data/hooks';
import type {Album, Artist, Audiobook, Photo, PhotoAlbum, TrackItem} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {ListRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {glow} from '../../design/Glow';
import {Icon} from '../../design/Icon';
import {Row} from '../../design/Row';
import {EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {back as backNav} from '../../nav/back';
import {Screen} from '../../nav/Screen';
import {back, go, push, type Routes} from '../../nav/routes';
import {player, usePlayer} from '../../state/player';
import {isManager, useSession} from '../../state/session';
import {openAddToPlaylist, openItemMenu, useItemState} from '../shared/actions';
import {fmtClock, openItem, toCard} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {ActionBar} from '../title/parts';

function useTint(t?: string) {
  useEffect(() => {
    if (t) glow.setTint(t);
  }, [t]);
}

// ---------------------------------------------------------------- Artist

export function ArtistScreen() {
  const route = useRoute<RouteProp<Routes, 'Artist'>>();
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const a = q.data as Artist | undefined;
  const albums = useItems(a?.albumIds);
  const allTracks = (a?.albumIds ?? []).flatMap(id => (albums.lookup(id) as Album | undefined)?.trackIds ?? []);
  const topIds = allTracks.filter((_, i) => i % 3 === 0).slice(0, 8);
  const tracks = useItems(topIds);
  const [more, setMore] = useState(false);
  useTint(a?.tint);
  const size = isTV ? 360 : layout.compact ? 180 : 240;
  return (
    <Screen title={a?.title}>
      {a ? (
        <View style={[styles.artistHead, layout.compact && {flexDirection: 'column', alignItems: 'center'}]}>
          <Artwork art={a.art.square} title={a.title} tint={a.tint} style={{width: size, height: size, borderRadius: size / 2}} />
          <View style={[{flex: 1}, layout.compact ? {alignItems: 'center', marginTop: 14} : {marginLeft: u(30)}]}>
            <Text variant="eyebrow" tone="accent">{a.genres.join(' · ')}</Text>
            <Text variant="hero" center={layout.compact} style={isTV ? {marginRight: 420} : undefined}>{a.title}</Text>
            <Text variant="caption" tone="tertiary" center={layout.compact}>{a.band ? `Formed ${a.formed}` : `Active since ${a.formed}`} · {a.origin}</Text>
            <Text variant="body" tone="secondary" numberOfLines={more ? undefined : 3} center={layout.compact} style={{marginTop: u(8), maxWidth: 900}}>{a.bio}</Text>
            <Button label={more ? 'Show less' : 'Read more'} size="sm" variant="ghost" onPress={() => setMore(!more)} style={{alignSelf: layout.compact ? 'center' : 'flex-start'}} />
            <View style={{flexDirection: 'row', gap: 10, marginTop: u(8)}}>
              <Button label="Play" icon="play" variant="primary" hasTVPreferredFocus onPress={() => { player.play(allTracks, {kind: 'audio'}); go('NowPlaying'); }} />
              <Button label="Shuffle" icon="shuffle" onPress={() => { player.play(allTracks, {kind: 'audio', shuffle: true}); go('NowPlaying'); }} />
              <IconButton icon="more" label="More actions" onPress={() => openItemMenu(s, a, {manager: isManager(session.role)})} />
            </View>
          </View>
        </View>
      ) : null}
      {topIds.length ? (
        <View style={{marginTop: u(24)}}>
          <Text variant="title" style={{marginBottom: u(8)}}>Top songs</Text>
          {topIds.map((id, i) => {
            const t = tracks.lookup(id) as TrackItem | undefined;
            return t ? <TrackRow key={id} t={t} index={i + 1} onPress={() => { player.play(topIds, {kind: 'audio', index: i}); go('NowPlaying'); }} /> : null;
          })}
        </View>
      ) : null}
      <View style={{marginHorizontal: -layout.gutter, marginTop: u(16)}}>
        <Row title="Albums" items={albums.cards(a?.albumIds ?? [])} onPressItem={c => push('Album', {id: c.id})} onLongPressItem={c => { const it = albums.lookup(c.id); if (it) openItemMenu(s, it, {manager: isManager(session.role)}); }} />
      </View>
    </Screen>
  );
}

function TrackRow({t, index, onPress, showArtist}: {t: TrackItem; index: number; onPress: () => void; showArtist?: boolean}) {
  const s = useServices();
  const current = usePlayer(p => p.itemId === t.id);
  const session = useSession();
  return (
    <Focusable onPress={onPress} onLongPress={() => openItemMenu(s, t, {manager: isManager(session.role)})} scale={isTV ? 1.02 : 1} accessibilityLabel={`${t.title}, ${fmtClock(t.duration)}`}>
      {({focused}) => {
        const lit = focused && isTV;
        return (
          <View style={[styles.track, lit && {backgroundColor: colors.white}]}>
            <View style={{width: u(28), alignItems: 'center'}}>
              {current ? <Icon name="audio" size={16} color={lit ? colors.actionInk : colors.accentStrong} /> : <Text variant="caption" color={lit ? colors.actionInk : colors.textTertiary}>{index}</Text>}
            </View>
            <View style={{flex: 1, marginLeft: u(10)}}>
              <Text variant="bodyStrong" numberOfLines={1} color={lit ? colors.actionInk : current ? colors.accentStrong : colors.text}>{t.title}</Text>
              {showArtist ? <Text variant="caption" color={lit ? 'rgba(3,18,28,0.7)' : colors.textTertiary} numberOfLines={1}>{t.artistName}</Text> : null}
            </View>
            {t.hasLyrics ? <View style={{marginRight: u(10)}}><Icon name="lyrics" size={14} color={lit ? colors.actionInk : colors.textTertiary} /></View> : null}
            <Text variant="caption" color={lit ? colors.actionInk : colors.textTertiary}>{fmtClock(t.duration)}</Text>
          </View>
        );
      }}
    </Focusable>
  );
}

// ---------------------------------------------------------------- Album

export function AlbumScreen() {
  const route = useRoute<RouteProp<Routes, 'Album'>>();
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const al = q.data as Album | undefined;
  const tracks = useItems(al?.trackIds);
  const st = useItemState(route.params.id);
  const more = useQuery(al ? 'item:' + al.artistId : null, () => s.catalog.item(al!.artistId));
  const others = (more.data as Artist | undefined)?.albumIds.filter(x => x !== al?.id) ?? [];
  const otherItems = useItems(others);
  useTint(al?.tint);
  const size = isTV ? 480 : layout.compact ? layout.width - 90 : 300;
  const wide = !layout.compact;
  const discs = al ? [...new Set(al.trackIds.map(id => (tracks.lookup(id) as TrackItem | undefined)?.disc ?? 1))] : [];
  return (
    <Screen title={al?.title}>
      {al ? (
        <View style={{flexDirection: wide ? 'row' : 'column', gap: u(28)}}>
          <View style={{alignItems: 'center'}}>
            <Artwork art={al.art.square} title={al.title} tint={al.tint} style={[{width: size, height: size, borderRadius: radius.lg}, styles.shadow]} />
          </View>
          <View style={{flex: 1}}>
            <Text variant="eyebrow" tone="accent">{al.albumType} · {al.year} · {al.label}</Text>
            <Text variant="hero" style={isTV ? {marginRight: 420} : undefined}>{al.title}</Text>
            <Focusable onPress={() => push('Artist', {id: al.artistId})} accessibilityRole="link" style={{alignSelf: 'flex-start'}}>
              {({focused}) => <Text variant="subheading" color={focused && isTV ? colors.actionInk : colors.accentStrong} style={focused && isTV ? {backgroundColor: colors.white, borderRadius: 8, paddingHorizontal: 8, marginHorizontal: -8} : undefined}>{al.artistName}</Text>}
            </Focusable>
            <Text variant="caption" tone="tertiary" style={{marginTop: 4}}>{al.trackIds.length} songs · {Math.round(al.duration / 60)} min{al.discs > 1 ? ` · ${al.discs} discs` : ''}</Text>
            <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 10, marginTop: u(14)}}>
              <Button label="Play" icon="play" variant="primary" hasTVPreferredFocus onPress={() => { player.play(al.trackIds, {kind: 'audio'}); go('NowPlaying'); }} />
              <Button label="Shuffle" icon="shuffle" onPress={() => { player.play(al.trackIds, {kind: 'audio', shuffle: true}); go('NowPlaying'); }} />
              <IconButton icon="heart" label={st.favorite ? 'Remove from Favorites' : 'Add to Favorites'} active={st.favorite} fill={st.favorite} onPress={() => s.user.set([al.id], {favorite: !st.favorite})} />
              <IconButton icon="listMusic" label="Add to playlist" onPress={() => openAddToPlaylist(s, [al.id])} />
              <IconButton icon="more" label="More actions" onPress={() => openItemMenu(s, al, {manager: isManager(session.role)})} />
            </View>
            <View style={{marginTop: u(18)}}>
              {discs.map(d => (
                <View key={d}>
                  {discs.length > 1 ? <Text variant="eyebrow" tone="tertiary" style={{marginTop: u(10), marginBottom: u(4)}}>Disc {d}</Text> : null}
                  {al.trackIds.map((id, i) => {
                    const t = tracks.lookup(id) as TrackItem | undefined;
                    return t && t.disc === d ? <TrackRow key={id} t={t} index={t.number} onPress={() => { player.play(al.trackIds, {kind: 'audio', index: i}); go('NowPlaying'); }} /> : null;
                  })}
                </View>
              ))}
            </View>
          </View>
        </View>
      ) : null}
      {others.length ? <View style={{marginHorizontal: -layout.gutter, marginTop: u(20)}}><Row title="More from" subject={al?.artistName} items={otherItems.cards(others)} onPressItem={c => push('Album', {id: c.id})} /></View> : null}
    </Screen>
  );
}

// ---------------------------------------------------------------- Audiobook

export function AudiobookScreen() {
  const route = useRoute<RouteProp<Routes, 'Audiobook'>>();
  const s = useServices();
  const layout = useLayout();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const b = q.data as Audiobook | undefined;
  const st = useItemState(route.params.id);
  useTint(b?.tint);
  const chapterIdx = b ? Math.max(0, b.chapters.findIndex((c, i) => st.position >= c.start && (i === b.chapters.length - 1 || st.position < b.chapters[i + 1].start))) : 0;
  const w = isTV ? 340 : layout.compact ? 180 : 240;
  return (
    <Screen title={b?.title}>
      {b ? (
        <View style={{flexDirection: layout.compact ? 'column' : 'row', gap: u(28), alignItems: layout.compact ? 'center' : 'flex-start'}}>
          <Artwork art={b.art.poster} title={b.title} tint={b.tint} style={[{width: w, height: w * 1.5, borderRadius: radius.md}, styles.shadow]} />
          <View style={{flex: 1, alignSelf: 'stretch'}}>
            <Text variant="eyebrow" tone="accent">{b.series ? `${b.series.name} · Book ${b.series.number}` : b.genres.join(' · ')}</Text>
            <Text variant="hero" style={isTV ? {marginRight: 420} : undefined}>{b.title}</Text>
            <Focusable onPress={() => push('Entity', {libraryId: b.libraryId, view: 'authors', entityId: `author:${b.author}`})} accessibilityRole="link" style={{alignSelf: 'flex-start'}}>
              {({focused}) => <Text variant="subheading" color={focused && isTV ? colors.actionInk : colors.accentStrong} style={focused && isTV ? {backgroundColor: colors.white, borderRadius: 8, paddingHorizontal: 8, marginHorizontal: -8} : undefined}>{b.author}</Text>}
            </Focusable>
            <Text variant="caption" tone="tertiary">Read by {b.narrator} · {Math.floor(b.duration / 3600)} hr {Math.round((b.duration % 3600) / 60)} min · {b.chapters.length} chapters{b.publicDomain ? ' · Public domain' : ''}</Text>
            <Text variant="body" tone="secondary" style={{marginTop: u(10), maxWidth: 900}}>{b.synopsis}</Text>
            {st.progress > 0 ? <Text variant="captionStrong" tone="accent" style={{marginTop: u(10)}}>Resume {b.chapters[chapterIdx]?.title} · {Math.round(st.progress * 100)}% listened</Text> : null}
            <ActionBar item={b} />
          </View>
        </View>
      ) : null}
      {b ? (
        <View style={{marginTop: u(24)}}>
          <Text variant="title" style={{marginBottom: u(6)}}>Chapters</Text>
          {b.chapters.map((c, i) => (
            <ListRow key={i} label={c.title} help={`${fmtClock(c.start)} · ${Math.round(c.duration / 60)} min`} chevron={false} compact onPress={() => { player.play([b.id], {kind: 'book', from: c.start}); go('NowPlaying'); }} right={i === chapterIdx && st.progress > 0 ? <Icon name="bookmark" size={16} color={colors.accentStrong} /> : undefined} />
          ))}
          <Text variant="title" style={{marginTop: u(20), marginBottom: u(6)}}>About {b.author}</Text>
          <Text variant="body" tone="secondary">{b.authorBio}</Text>
        </View>
      ) : null}
    </Screen>
  );
}

// ---------------------------------------------------------------- Photos

export function PhotoAlbumScreen() {
  const route = useRoute<RouteProp<Routes, 'PhotoAlbum'>>();
  const s = useServices();
  const layout = useLayout();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const pa = q.data as PhotoAlbum | undefined;
  const photos = useItems(pa?.photoIds);
  useTint(pa?.tint);
  const cols = isTV ? 6 : layout.compact ? 3 : 5;
  const gap = isTV ? 16 : 4;
  const w = Math.floor((layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * gap) / cols);
  return (
    <Screen title={pa?.title}>
      {pa ? (
        <View style={{marginBottom: u(16)}}>
          <Text variant="eyebrow" tone="accent">{pa.place} · {pa.dateStart}{pa.dateEnd !== pa.dateStart ? ` – ${pa.dateEnd}` : ''}</Text>
          <Text variant="hero">{pa.title}</Text>
          <View style={{flexDirection: 'row', gap: 10, marginTop: u(10)}}>
            <Button label="Slideshow" icon="play" variant="primary" hasTVPreferredFocus onPress={() => push('PhotoViewer', {albumId: pa.id, index: 0, slideshow: true})} />
            <Text variant="caption" tone="tertiary" style={{alignSelf: 'center'}}>{pa.photoIds.length} photos</Text>
          </View>
        </View>
      ) : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap}}>
        {(pa?.photoIds ?? []).map((id, i) => {
          const p = photos.lookup(id) as Photo | undefined;
          return (
            <Focusable key={id} onPress={() => push('PhotoViewer', {albumId: pa!.id, index: i})} scale={1.08} accessibilityLabel={p?.caption || `Photo ${i + 1}`}>
              {({focused}) => <Artwork art={p?.art.thumb} tint={p?.tint} style={[{width: w, height: w, borderRadius: isTV ? radius.sm : 2}, focused && isTV && {borderWidth: 4, borderColor: colors.white}]} />}
            </Focusable>
          );
        })}
      </View>
    </Screen>
  );
}

/** Full-screen photo viewer: swipe or arrow between photos, captions, and a slideshow. */
export function PhotoViewerScreen() {
  const route = useRoute<RouteProp<Routes, 'PhotoViewer'>>();
  const s = useServices();
  const insets = useSafeAreaInsets();
  const {width} = useWindowDimensions();
  const q = useQuery('item:' + route.params.albumId, () => s.catalog.item(route.params.albumId));
  const pa = q.data as PhotoAlbum | undefined;
  const photos = useItems(pa?.photoIds);
  const [index, setIndex] = useState(route.params.index);
  const [show, setShow] = useState(true);
  const [playing, setPlaying] = useState(!!route.params.slideshow);
  const fade = useRef(new Animated.Value(1)).current;
  const count = pa?.photoIds.length ?? 0;
  const go2 = (d: number) => {
    if (!count) return;
    Animated.sequence([Animated.timing(fade, {toValue: 0.2, duration: 160, useNativeDriver: true}), Animated.timing(fade, {toValue: 1, duration: 260, useNativeDriver: true})]).start();
    setTimeout(() => setIndex(i => (i + d + count) % count), 150);
  };
  useEffect(() => {
    if (!playing) return;
    const t = setInterval(() => go2(1), 4000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playing, count]);
  useEffect(() => backNav.register({priority: 45, active: () => true, handle: () => back()}), []);
  useTVEventHandler(e => {
    if (e?.eventType === 'right') go2(1);
    if (e?.eventType === 'left') go2(-1);
    if (e?.eventType === 'playPause') setPlaying(p => !p);
  });
  const photo = pa ? (photos.lookup(pa.photoIds[index]) as Photo | undefined) : undefined;
  const src = resolveArt(photo?.art.backdrop);
  return (
    <Pressable style={{flex: 1, backgroundColor: '#000'}} onPress={() => setShow(v => !v)} accessible={false} focusable={isTV ? !show : undefined}>
      {src ? <Animated.Image source={src} resizeMode="contain" style={[StyleSheet.absoluteFill, {width: '100%', height: '100%', opacity: fade}]} /> : <EmptyState icon="images" title="Loading photo" />}
      {show ? (
        <TVFocusGuideView autoFocus style={StyleSheet.absoluteFill} pointerEvents="box-none">
          <View style={[styles.viewerTop, {paddingTop: insets.top + (isTV ? 40 : 8)}]}>
            {!isTV ? <IconButton icon="x" label="Close" variant="glass" onPress={back} /> : null}
            <View style={{flex: 1, marginHorizontal: 12}}>
              <Text variant="bodyStrong" numberOfLines={1}>{photo?.caption}</Text>
              <Text variant="caption" tone="secondary">{pa?.title} · {index + 1} of {count} · {photo?.place}</Text>
            </View>
          </View>
          <View style={[styles.viewerBottom, {paddingBottom: insets.bottom + (isTV ? 40 : 12)}]}>
            <Button icon="chevronLeft" accessibilityLabel="Previous photo" variant="glass" onPress={() => go2(-1)} />
            <Button icon={playing ? 'pause' : 'play'} label={playing ? 'Pause slideshow' : 'Slideshow'} variant="glass" hasTVPreferredFocus onPress={() => setPlaying(p => !p)} />
            <Button icon="chevronRight" accessibilityLabel="Next photo" variant="glass" onPress={() => go2(1)} />
          </View>
          {!isTV ? (
            <>
              <Pressable style={[styles.zone, {left: 0}]} onPress={() => go2(-1)} accessibilityLabel="Previous photo" />
              <Pressable style={[styles.zone, {right: 0}]} onPress={() => go2(1)} accessibilityLabel="Next photo" />
            </>
          ) : null}
        </TVFocusGuideView>
      ) : null}
      <View style={{position: 'absolute', width, height: 0}} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  artistHead: {flexDirection: 'row', alignItems: 'center', marginTop: u(8)},
  track: {flexDirection: 'row', alignItems: 'center', paddingVertical: u(10), paddingHorizontal: u(8), borderRadius: radius.md},
  shadow: {shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 24, shadowOffset: {width: 0, height: 12}},
  viewerTop: {flexDirection: 'row', alignItems: 'center', paddingHorizontal: isTV ? 80 : 16, backgroundColor: 'rgba(0,0,0,0.35)', paddingBottom: 10},
  viewerBottom: {position: 'absolute', left: 0, right: 0, bottom: 0, flexDirection: 'row', justifyContent: 'center', gap: 14},
  zone: {position: 'absolute', top: 120, bottom: 120, width: '22%'},
});

export {Image, FlatList, toCard, openItem};
