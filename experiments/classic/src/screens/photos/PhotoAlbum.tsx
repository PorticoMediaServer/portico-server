import React, {useEffect, useRef, useState} from 'react';
import {Animated, Easing, FlatList, Modal, PanResponder, StyleSheet, TVFocusGuideView, View, useTVEventHandler} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useQuery} from '../../data/DataProvider';
import type {PhotoItem} from '../../data/types';
import {PageHeader, useChromeInsets, useRegisterNav} from '../../ui/Page';
import {Button, IconButton} from '../../ui/Button';
import {Loading} from '../../ui/Controls';
import {Artwork} from '../../ui/Artwork';
import {Focusable} from '../../ui/Focusable';
import {Text} from '../../ui/Text';
import {colors} from '../../theme/tokens';
import {gridFor, useMetrics} from '../../theme/layout';
import {useContentWidth} from '../../ui/useWidth';
import {back} from '../../nav/nav';
import {usePrefs} from '../../app/prefs';

/** A photo album: the gallery grid, a full-screen viewer and a slideshow. */
export function PhotoAlbumScreen() {
  useRegisterNav();
  const {params} = useRoute() as {params: {id: string}};
  const m = useMetrics();
  const chrome = useChromeInsets();
  const q = useQuery(c => c.catalog.photoAlbum(params.id), [params.id]);
  const [width, onLayout] = useContentWidth();
  const [viewer, setViewer] = useState<{index: number; slideshow: boolean}>();
  const {columns, itemWidth} = gridFor(width - m.gutter * 2, m.isTV ? 280 : m.isPhone ? 110 : 170, m.isTV ? 16 : 6);
  const photos = q.data?.photos ?? [];
  const rows: PhotoItem[][] = [];
  for (let i = 0; i < photos.length; i += columns) rows.push(photos.slice(i, i + columns));
  const gap = m.isTV ? 16 : 6;
  const content = (
    <FlatList data={rows} keyExtractor={(r, i) => r[0]?.id ?? String(i)} contentContainerStyle={{paddingTop: chrome.top, paddingBottom: chrome.bottom, gap}}
      ListHeaderComponent={
        <View style={{marginBottom: m.space.md}}>
          <PageHeader eyebrow="Photo album" title={q.data?.entry.title ?? ''} subtitle={q.data ? `${photos.length} photos` : undefined} onBack={back}
            actions={photos.length ? <Button label="Slideshow" icon="play" variant="primary" onPress={() => setViewer({index: 0, slideshow: true})} hasTVPreferredFocus /> : undefined} />
          {q.loading && !q.data ? <Loading /> : null}
        </View>
      }
      renderItem={({item: row, index: r}) => (
        <View style={{flexDirection: 'row', gap, paddingHorizontal: m.gutter}}>
          {row.map((p, j) => (
            <Focusable key={p.id} onPress={() => setViewer({index: r * columns + j, slideshow: false})} focusScale={m.isTV ? 1.06 : 1} accessibilityLabel={`${p.title}, ${new Date(p.taken).toLocaleDateString('en-US')}`}>
              {({focused}) => <Artwork source={p.thumb} title={p.title} width={itemWidth} height={itemWidth} radius={m.isTV ? 10 : 4} style={{borderWidth: focused ? 3 : 0, borderColor: colors.focus}} />}
            </Focusable>
          ))}
        </View>
      )} />
  );
  return (
    <View style={{flex: 1}} onLayout={onLayout}>
      {m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{content}</TVFocusGuideView> : content}
      {viewer ? <PhotoViewer photos={photos} start={viewer.index} slideshow={viewer.slideshow} onClose={() => setViewer(undefined)} /> : null}
    </View>
  );
}

export function PhotoViewer({photos, start, slideshow: initialSlideshow, onClose}: {photos: PhotoItem[]; start: number; slideshow: boolean; onClose: () => void}) {
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const prefs = usePrefs();
  const [index, setIndex] = useState(start);
  const [slideshow, setSlideshow] = useState(initialSlideshow);
  const [chrome, setChrome] = useState(!initialSlideshow);
  const [info, setInfo] = useState(false);
  const fade = useRef(new Animated.Value(1)).current;
  const zoom = useRef(new Animated.Value(1)).current;
  const go = (d: number) => {
    Animated.timing(fade, {toValue: 0, duration: 200, useNativeDriver: true}).start(() => {
      setIndex(i => (i + d + photos.length) % photos.length);
      zoom.setValue(1);
      Animated.timing(fade, {toValue: 1, duration: 360, useNativeDriver: true}).start();
    });
  };
  useEffect(() => {
    if (!slideshow) return;
    if (!prefs.reduceMotion) Animated.timing(zoom, {toValue: 1.1, duration: 5000, easing: Easing.linear, useNativeDriver: true}).start();
    const t = setTimeout(() => go(1), 5000);
    return () => clearTimeout(t);
  }, [slideshow, index]); // eslint-disable-line react-hooks/exhaustive-deps
  useTVEventHandler(evt => {
    if (evt.eventType === 'right' && !chrome) go(1);
    if (evt.eventType === 'left' && !chrome) go(-1);
    if (evt.eventType === 'playPause') setSlideshow(s => !s);
    if ((evt.eventType === 'up' || evt.eventType === 'down' || evt.eventType === 'select') && !chrome) setChrome(true);
  });
  const pan = useRef(PanResponder.create({onMoveShouldSetPanResponder: (_, g) => Math.abs(g.dx) > 20, onPanResponderRelease: (_, g) => { if (Math.abs(g.dx) > 50) go(g.dx < 0 ? 1 : -1); else if (Math.abs(g.dy) > 80) onClose(); }})).current;
  const p = photos[index]!;
  return (
    <Modal visible transparent={false} animationType="fade" onRequestClose={() => (chrome && m.isTV ? setChrome(false) : onClose())} supportedOrientations={['portrait', 'landscape']}>
      <View style={{flex: 1, backgroundColor: '#000'}} {...(m.isTV ? {} : pan.panHandlers)}>
        <Focusable style={StyleSheet.absoluteFill} onPress={() => setChrome(c => !c)} focusable={false} accessible={false}>
          <Animated.View style={[StyleSheet.absoluteFill, {opacity: fade, transform: [{scale: zoom}]}]}>
            <Artwork source={p.image} title={p.title} width="100%" height="100%" radius={0} resizeMode="contain" fade={false} style={{backgroundColor: '#000'}} />
          </Animated.View>
        </Focusable>
        {chrome ? (
          <TVFocusGuideView autoFocus style={[StyleSheet.absoluteFill, {justifyContent: 'space-between', paddingTop: m.isTV ? 48 : insets.top + 8, paddingBottom: m.isTV ? 48 : insets.bottom + 12, paddingHorizontal: m.isTV ? 64 : 16}]} pointerEvents="box-none">
            <View style={{flexDirection: 'row', alignItems: 'center', gap: 12}}>
              {m.isTV ? null : <IconButton icon="close" label="Close" variant="secondary" onPress={onClose} />}
              <View style={{flex: 1}}>
                <Text variant="subheading">{p.title}</Text>
                <Text variant="caption" tone="secondary">{index + 1} of {photos.length} · {new Date(p.taken).toLocaleString('en-US', {dateStyle: 'medium', timeStyle: 'short'})}</Text>
              </View>
              <IconButton icon="info" label="Photo info" variant="secondary" selected={info} onPress={() => setInfo(i => !i)} />
            </View>
            {info ? <View style={{alignSelf: 'flex-end', backgroundColor: colors.overlay, borderRadius: 14, padding: 16, gap: 4}}><Text variant="captionStrong">{p.camera}</Text><Text variant="caption" tone="secondary">{p.portrait ? '3024 × 4032' : '4032 × 3024'} · f/1.8 · 1/250 s · ISO 64</Text><Text variant="caption" tone="secondary">{new Date(p.taken).toDateString()}</Text></View> : <View />}
            <View style={{flexDirection: 'row', justifyContent: 'center', gap: m.space.md}}>
              <IconButton icon="back" label="Previous photo" variant="secondary" onPress={() => go(-1)} />
              <Button label={slideshow ? 'Pause slideshow' : 'Slideshow'} icon={slideshow ? 'pause' : 'play'} variant="primary" onPress={() => { setSlideshow(s => !s); if (!slideshow) setChrome(false); }} hasTVPreferredFocus />
              <IconButton icon="forward" label="Next photo" variant="secondary" onPress={() => go(1)} />
            </View>
          </TVFocusGuideView>
        ) : null}
      </View>
    </Modal>
  );
}
