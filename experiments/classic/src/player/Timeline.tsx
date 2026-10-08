import React, {useRef, useState} from 'react';
import {PanResponder, TVFocusGuideView, View, useTVEventHandler} from 'react-native';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Focusable} from '../ui/Focusable';
import {Text} from '../ui/Text';
import {Artwork} from '../ui/Artwork';
import {formatTime} from './PlayerProvider';
import type {Chapter, Img} from '../data/types';

/**
 * The seek bar: chapter ticks, buffered range, scrub preview thumbnails.
 * Touch: drag. TV: focus the bar, then left/right scrub and Select commits.
 */
export function Timeline({position, duration, onSeek, chapters, thumbFor, live, onActivity}: {position: number; duration: number; onSeek: (s: number) => void; chapters?: Chapter[]; thumbFor?: (s: number) => Img; live?: boolean; onActivity?: () => void}) {
  const m = useMetrics();
  const [width, setWidth] = useState(1);
  const [scrub, setScrub] = useState<number>();
  const [focused, setFocused] = useState(false);
  const widthRef = useRef(1);
  widthRef.current = width;
  const durRef = useRef(duration);
  durRef.current = duration;
  const pan = useRef(PanResponder.create({
    onStartShouldSetPanResponder: () => true,
    onMoveShouldSetPanResponder: () => true,
    onPanResponderGrant: e => { setScrub(Math.max(0, Math.min(1, e.nativeEvent.locationX / widthRef.current)) * durRef.current); onActivity?.(); },
    onPanResponderMove: e => { setScrub(Math.max(0, Math.min(1, e.nativeEvent.locationX / widthRef.current)) * durRef.current); onActivity?.(); },
    onPanResponderRelease: () => setScrub(s => { if (s !== undefined) onSeek(s); return undefined; }),
    onPanResponderTerminate: () => setScrub(undefined),
  })).current;
  useTVEventHandler(evt => {
    if (!focused || !duration) return;
    if (evt.eventType === 'right' || evt.eventType === 'left') {
      const step = Math.max(10, duration / 120);
      setScrub(s => Math.max(0, Math.min(duration, (s ?? position) + (evt.eventType === 'right' ? step : -step))));
      onActivity?.();
    }
    if (evt.eventType === 'longRight' || evt.eventType === 'longLeft') {
      setScrub(s => Math.max(0, Math.min(duration, (s ?? position) + (evt.eventType === 'longRight' ? 60 : -60))));
    }
  });
  const shown = scrub ?? position;
  const frac = duration ? shown / duration : 1;
  const h = m.isTV ? (focused ? 12 : 8) : scrub !== undefined ? 8 : 5;
  const currentChapter = chapters?.slice().reverse().find(c => c.start <= shown);
  const thumbW = m.isTV ? 320 : 176;
  const bar = (
    <View onLayout={e => setWidth(e.nativeEvent.layout.width)} style={{height: m.isTV ? 44 : 30, justifyContent: 'center'}} {...(m.isTV ? {} : pan.panHandlers)}>
      <View style={{height: h, borderRadius: h, backgroundColor: 'rgba(237,245,250,0.22)', overflow: 'hidden'}}>
        {!live ? <View style={{position: 'absolute', left: 0, top: 0, bottom: 0, width: `${Math.min(100, (position / (duration || 1)) * 100 + 12)}%`, backgroundColor: 'rgba(237,245,250,0.18)'}} /> : null}
        <View style={{width: `${frac * 100}%`, height: h, backgroundColor: live ? colors.record : colors.accentStrong}} />
      </View>
      {chapters?.slice(1).map(c => <View key={c.start} style={{position: 'absolute', left: (c.start / (duration || 1)) * width - 1, width: 2, height: h, backgroundColor: 'rgba(7,16,24,0.8)'}} />)}
      <View style={{position: 'absolute', left: frac * width - (m.isTV ? 12 : 8), width: m.isTV ? 24 : 16, height: m.isTV ? 24 : 16, borderRadius: 12, backgroundColor: colors.text, borderWidth: focused ? 3 : 0, borderColor: colors.accent}} />
      {scrub !== undefined && thumbFor ? (
        <View pointerEvents="none" style={{position: 'absolute', bottom: m.isTV ? 60 : 40, left: Math.max(0, Math.min(width - thumbW, frac * width - thumbW / 2)), alignItems: 'center', gap: 6}}>
          <Artwork source={thumbFor(scrub)} width={thumbW} height={thumbW * 0.5625} radius={m.radius.card} fade={false} style={{borderWidth: 2, borderColor: colors.text}} />
          <View style={{backgroundColor: colors.overlay, borderRadius: 8, paddingHorizontal: 10, paddingVertical: 3}}>
            <Text variant="captionStrong">{formatTime(scrub)}{currentChapter ? ` · ${currentChapter.title}` : ''}</Text>
          </View>
        </View>
      ) : null}
    </View>
  );
  return (
    <View style={{gap: 4}}>
      {m.isTV ? (
        <TVFocusGuideView trapFocusLeft trapFocusRight>
          <Focusable onFocus={() => setFocused(true)} onBlur={() => { setFocused(false); setScrub(undefined); }} onPress={() => { if (scrub !== undefined) { onSeek(scrub); setScrub(undefined); } }} accessibilityLabel="Playback position" accessibilityRole="adjustable">
            {bar}
          </Focusable>
        </TVFocusGuideView>
      ) : bar}
      <View style={{flexDirection: 'row', justifyContent: 'space-between'}}>
        <Text variant="captionStrong" tone="secondary">{live ? 'LIVE' : formatTime(shown)}</Text>
        {currentChapter && !m.isPhone ? <Text variant="caption" tone="tertiary">{currentChapter.title}</Text> : null}
        <Text variant="captionStrong" tone="secondary">{live ? '' : `-${formatTime(Math.max(0, duration - shown))}`}</Text>
      </View>
    </View>
  );
}
