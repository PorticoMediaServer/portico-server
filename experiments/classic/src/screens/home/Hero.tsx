import React, {useEffect, useRef, useState} from 'react';
import {Animated, PanResponder, View} from 'react-native';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import type {Entry} from '../../data/types';
import {Artwork} from '../../ui/Artwork';
import {Scrim} from '../../ui/Canvas';
import {Text} from '../../ui/Text';
import {Button} from '../../ui/Button';
import {Focusable} from '../../ui/Focusable';
import {Pill} from '../../ui/Controls';
import {formatDuration} from '../../player/PlayerProvider';
import {usePrefs} from '../../app/prefs';
import {useGlow} from '../../app/glow';

/**
 * The featured hero: a full-bleed backdrop with the title, facts, synopsis
 * and fast paths to play. Cycles through the featured titles.
 */
export function Hero({items, eyebrow, onPlay, onDetails, onWatchlist, watchlisted}: {items: Entry[]; eyebrow?: string; onPlay: (e: Entry) => void; onDetails: (e: Entry) => void; onWatchlist: (e: Entry) => void; watchlisted: (e: Entry) => boolean}) {
  const m = useMetrics();
  const prefs = usePrefs();
  const glow = useGlow();
  const [index, setIndex] = useState(0);
  const [measured, setMeasured] = useState(0);
  const [focusedInside, setFocusedInside] = useState(false);
  const fade = useRef(new Animated.Value(1)).current;
  const e = items[index % Math.max(1, items.length)];
  useEffect(() => { if (e) glow(e.id); }, [e?.id, glow]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (items.length < 2 || focusedInside) return;
    const t = setTimeout(() => go(1), 9000);
    return () => clearTimeout(t);
  }, [index, items.length, focusedInside]); // eslint-disable-line react-hooks/exhaustive-deps
  const go = (d: number) => {
    Animated.timing(fade, {toValue: 0, duration: prefs.reduceMotion ? 0 : 180, useNativeDriver: true}).start(() => {
      setIndex(i => (i + d + items.length) % items.length);
      Animated.timing(fade, {toValue: 1, duration: prefs.reduceMotion ? 0 : 320, useNativeDriver: true}).start();
    });
  };
  const pan = useRef(PanResponder.create({
    onMoveShouldSetPanResponder: (_, g) => Math.abs(g.dx) > 20 && Math.abs(g.dx) > Math.abs(g.dy) * 2,
    onPanResponderRelease: (_, g) => { if (Math.abs(g.dx) > 50) go(g.dx < 0 ? 1 : -1); },
  })).current;
  if (!e) return null;
  const heroH = m.hero;
  const width = measured || (m.isTV ? m.width - m.rail.collapsed : m.width);
  const meta = [e.kind === 'show' ? e.subtitle : e.year, e.duration ? formatDuration(e.duration) : e.caption, ...(e.genres ?? []).slice(0, 2)].filter(Boolean).join('  ·  ');
  const progress = e.progress ?? 0;
  const on = watchlisted(e);
  const phone = m.isPhone;
  return (
    <View onLayout={ev => setMeasured(ev.nativeEvent.layout.width)} style={{height: heroH + (phone ? 150 : 0), marginBottom: m.isTV ? 8 : 4}} {...pan.panHandlers}>
      <Animated.View style={{position: 'absolute', left: 0, right: 0, top: 0, height: heroH, opacity: fade}}>
        {prefs.showBackdrops ? <Artwork source={e.backdrop ?? e.image} title={e.title} width={width} height={heroH} radius={0} /> : null}
        <Scrim width={width} height={heroH} left={!phone} strength={phone ? 1 : 1.05} />
      </Animated.View>
      <Animated.View style={{position: 'absolute', left: m.gutter, right: m.gutter, bottom: phone ? 0 : m.isTV ? 70 : 40, maxWidth: m.isTV ? 900 : phone ? undefined : 560, gap: m.isTV ? 14 : 8, opacity: fade}}>
        {eyebrow ? <Text variant="label" tone="accent">{eyebrow}</Text> : null}
        <Text variant="display" numberOfLines={2} style={m.isTablet ? {fontSize: 46, lineHeight: 52} : undefined}>{e.title}</Text>
        <View style={{flexDirection: 'row', alignItems: 'center', gap: 10, flexWrap: 'wrap'}}>
          {e.rating ? <Pill label={e.rating} /> : null}
          <Text variant="caption" tone="secondary">{meta}</Text>
        </View>
        {e.overview ? <Text variant="body" tone="secondary" numberOfLines={phone ? 2 : 3}>{e.overview}</Text> : null}
        <View style={{flexDirection: 'row', gap: m.space.sm, marginTop: m.isTV ? 10 : 6, alignItems: 'center'}}>
          <Button label={progress > 0 ? (phone ? 'Resume' : `Resume · ${formatDuration((e.duration ?? 0) * (1 - progress))} left`) : 'Play'} icon="play" variant="primary" size="lg" onPress={() => onPlay(e)} hasTVPreferredFocus onFocus={() => setFocusedInside(true)} grow={phone} style={phone ? {flex: 1} : undefined} />
          <Button label={phone ? undefined : 'Details'} icon="info" variant="secondary" size="lg" onPress={() => onDetails(e)} accessibilityLabel="Details" onFocus={() => setFocusedInside(true)} />
          <Button icon={on ? 'savedFilled' : 'saved'} variant="secondary" size="lg" selected={on} onPress={() => onWatchlist(e)} accessibilityLabel={on ? 'Remove from Watchlist' : 'Add to Watchlist'} onFocus={() => setFocusedInside(true)} />
          {items.length > 1 ? (
            <View style={{flexDirection: 'row', gap: 6, marginLeft: m.space.sm, alignItems: 'center'}}>
              {items.map((it, i) => (
                <Focusable key={it.id} onPress={() => go(i - index)} onFocus={() => setFocusedInside(true)} accessibilityLabel={`Featured ${i + 1} of ${items.length}: ${it.title}`}
                  style={({focused}) => ({width: i === index ? (m.isTV ? 28 : 18) : (m.isTV ? 12 : 7), height: m.isTV ? 12 : 7, borderRadius: 6, backgroundColor: focused ? colors.focus : i === index ? colors.text : 'rgba(237,245,250,0.35)'})} />
              ))}
            </View>
          ) : null}
        </View>
      </Animated.View>
    </View>
  );
}
