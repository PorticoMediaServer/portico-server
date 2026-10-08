import React, {useRef, useState} from 'react';
import {PanResponder, StyleSheet, View} from 'react-native';
import type {ArtRef} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {fmtClock} from '../shared/cards';

export type Mark = {start: number; end?: number; title?: string; art?: ArtRef};

/**
 * Scrub bar: chapter starts are drawn as gaps, intro and credits as a soft tint, and while
 * scrubbing a preview picture (the nearest chapter frame) rides above the thumb.
 */
export function Timeline({position, duration, onSeek, chapters = [], segments = [], focused, preview, live}: {position: number; duration: number; onSeek?: (s: number) => void; chapters?: Mark[]; segments?: Mark[]; focused?: boolean; preview?: number | null; live?: {behind: number}}) {
  const [width, setWidth] = useState(1);
  const [drag, setDrag] = useState<number | null>(null);
  const pct = (v: number) => (duration ? Math.max(0, Math.min(1, v / duration)) : 0);
  const shown = drag ?? preview ?? null;
  const pan = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => !isTV,
      onMoveShouldSetPanResponder: () => !isTV,
      onPanResponderGrant: e => setDrag((e.nativeEvent.locationX / widthRef.current) * durRef.current),
      onPanResponderMove: e => setDrag(Math.max(0, Math.min(durRef.current, (e.nativeEvent.locationX / widthRef.current) * durRef.current))),
      onPanResponderRelease: e => {
        const t = Math.max(0, Math.min(durRef.current, (e.nativeEvent.locationX / widthRef.current) * durRef.current));
        seekRef.current?.(t);
        setDrag(null);
      },
    }),
  ).current;
  const widthRef = useRef(width);
  widthRef.current = width;
  const durRef = useRef(duration);
  durRef.current = duration;
  const seekRef = useRef(onSeek);
  seekRef.current = onSeek;
  const at = shown ?? position;
  const chapter = [...chapters].reverse().find(c => c.start <= at);
  const h = focused ? u(8) : u(5);
  return (
    <View>
      {shown !== null ? (
        <View pointerEvents="none" style={[styles.preview, {left: Math.max(0, Math.min(width - u(200), pct(shown) * width - u(100)))}]}>
          <Artwork art={chapter?.art} tint={colors.raised} style={styles.previewArt} />
          <Text variant="captionStrong" center style={{marginTop: 6}}>{fmtClock(shown)}</Text>
          {chapter?.title ? <Text variant="caption" tone="secondary" center numberOfLines={1}>{chapter.title}</Text> : null}
        </View>
      ) : null}
      <View {...pan.panHandlers} onLayout={e => setWidth(e.nativeEvent.layout.width)} style={{paddingVertical: u(10)}} hitSlop={{top: 16, bottom: 16}}>
        <View style={[styles.track, {height: h, borderRadius: h / 2}]}>
          {segments.map((sg, i) => <View key={i} style={[styles.segment, {left: `${pct(sg.start) * 100}%`, width: `${(pct(sg.end ?? sg.start) - pct(sg.start)) * 100}%`}]} />)}
          {live ? null : <View style={[styles.buffer, {width: `${Math.min(1, pct(position) + 0.08) * 100}%`}]} />}
          <View style={[styles.fill, {width: `${(live ? 1 - Math.min(1, live.behind / 3600) : pct(at)) * 100}%`}]} />
          {chapters.slice(1).map((c, i) => <View key={i} style={[styles.gap, {left: `${pct(c.start) * 100}%`}]} />)}
        </View>
        <View style={[styles.thumb, {left: `${(live ? 1 - Math.min(1, live.behind / 3600) : pct(at)) * 100}%`, width: focused ? u(20) : u(14), height: focused ? u(20) : u(14), marginLeft: focused ? -u(10) : -u(7), marginTop: focused ? -u(14) : -u(10)}, focused && styles.thumbFocus]} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  track: {backgroundColor: 'rgba(255,255,255,0.22)', overflow: 'hidden'},
  buffer: {position: 'absolute', left: 0, top: 0, bottom: 0, backgroundColor: 'rgba(255,255,255,0.22)'},
  fill: {position: 'absolute', left: 0, top: 0, bottom: 0, backgroundColor: colors.accentStrong},
  segment: {position: 'absolute', top: 0, bottom: 0, backgroundColor: 'rgba(232,188,106,0.45)'},
  gap: {position: 'absolute', top: 0, bottom: 0, width: 3, marginLeft: -1.5, backgroundColor: 'rgba(7,16,24,0.9)'},
  thumb: {position: 'absolute', top: '50%', borderRadius: 99, backgroundColor: colors.white},
  thumbFocus: {shadowColor: colors.accentStrong, shadowOpacity: 0.9, shadowRadius: 14},
  preview: {position: 'absolute', bottom: u(34), width: u(200), alignItems: 'center'},
  previewArt: {width: u(200), height: u(112), borderRadius: u(8), borderWidth: 2, borderColor: colors.white},
});
