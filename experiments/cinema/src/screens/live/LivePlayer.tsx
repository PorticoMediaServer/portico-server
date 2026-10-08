import React, {useEffect, useMemo, useRef, useState} from 'react';
import {ActivityIndicator, Pressable, ScrollView, StyleSheet, TVFocusGuideView, View, useTVEventHandler} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useQuery, useServices} from '../../data/hooks';
import type {Channel, Programme} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {Badge} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {back as backNav} from '../../nav/back';
import {back, go, type Routes} from '../../nav/routes';
import {player, usePlayer} from '../../state/player';
import {VideoSurface} from '../player/Player';
import {Timeline} from '../player/Timeline';
import {fmtTime, openProgramme} from './Live';

const CAPTIONS = ['…and that brings us to the forecast for the weekend,', 'with clear skies expected across the valley.', 'Now, let’s head back to the studio.', 'Thanks. Our top story tonight comes from the harbor,', 'where crews worked through the night to reopen the bridge.'];

/** The live player: LIVE badge, behind-live scrubbing, channel up/down, mini guide and captions. */
export function LivePlayerScreen() {
  const route = useRoute<RouteProp<Routes, 'LivePlayer'>>();
  const s = useServices();
  const insets = useSafeAreaInsets();
  const p = usePlayer();
  const [channelId, setChannelId] = useState(route.params.channelId);
  const [chrome, setChrome] = useState(true);
  const [guide, setGuide] = useState(false);
  const [problem, setProblem] = useState(false);
  const hide = useRef<ReturnType<typeof setTimeout> | null>(null);
  const chans = useQuery('channels', () => s.live.channels());
  const now = Date.now();
  const progs = useQuery(`guide:live:${Math.floor(now / (5 * 60_000))}`, () => s.live.guide(now - 3 * 3600_000, now + 3 * 3600_000));
  const list = (chans.data ?? []).filter(c => !c.hidden);
  const ch = list.find(c => c.id === channelId);
  const cur = (progs.data ?? []).find(x => x.channelId === channelId && x.start <= now && x.end > now);
  const next = (progs.data ?? []).filter(x => x.channelId === channelId && x.start >= (cur?.end ?? now)).sort((a, b) => a.start - b.start)[0];
  const frames = useQuery('live-frames', async () => (await s.catalog.browse({libraryId: 'lib-movies', filters: {quick: [], fields: {}}, sort: {field: 'random', direction: 'asc'}, pageSize: 40})).items.map(i => i.art.backdrop).filter(Boolean) as string[]);
  const myFrames = useMemo(() => {
    const all = frames.data ?? [];
    const off = list.findIndex(c => c.id === channelId) * 3;
    return all.length ? [0, 1, 2].map(i => all[(off + i) % all.length]) : [];
  }, [frames.data, list, channelId]);

  useEffect(() => {
    player.playLive(channelId);
    setProblem(false);
  }, [channelId]);
  useEffect(() => () => player.stop(), []);
  const poke = () => {
    setChrome(true);
    if (hide.current) clearTimeout(hide.current);
    hide.current = setTimeout(() => !guide && setChrome(false), 5000);
  };
  useEffect(() => {
    poke();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId]);
  const zap = (d: number) => {
    const i = list.findIndex(c => c.id === channelId);
    const nextCh = list[(i + d + list.length) % list.length];
    if (nextCh) setChannelId(nextCh.id);
  };
  useEffect(() => backNav.register({priority: 50, active: () => true, handle: () => (guide ? setGuide(false) : back())}), [guide]);
  useTVEventHandler(e => {
    if (!e) return;
    if (e.eventType === 'playPause') player.toggle();
    if (!chrome && !guide && (e.eventType === 'up' || e.eventType === 'down')) zap(e.eventType === 'up' ? -1 : 1);
    poke();
  });
  const pad = isTV ? 80 : 18;
  return (
    <Pressable style={{flex: 1, backgroundColor: '#000'}} onPress={poke} accessible={false} focusable={isTV ? !chrome : undefined}>
      <VideoSurface frames={myFrames} playing={p.playing} />
      {p.buffering ? <View style={styles.center}><ActivityIndicator color={colors.white} size="large" /><Text variant="caption" style={{marginTop: 8}}>Tuning {ch?.name}…</Text></View> : null}
      {p.captions ? <View style={[styles.captions, {bottom: chrome ? (isTV ? 280 : 170) : 60}]} pointerEvents="none"><Text variant="bodyStrong" center style={styles.capText}>{CAPTIONS[Math.floor(p.position / 4) % CAPTIONS.length]}</Text></View> : null}
      {problem ? (
        <View style={[styles.center, {backgroundColor: 'rgba(0,0,0,0.8)', padding: 30}]}>
          <Icon name="alertCircle" size={36} color={colors.amber} />
          <Text variant="title" center style={{marginTop: 10}}>{ch?.name} can’t start right now</Text>
          <Text variant="body" tone="secondary" center style={{marginTop: 6}}>All tuners are busy recording or in use. Try again when one frees up.</Text>
          <View style={{flexDirection: 'row', gap: 10, marginTop: 16}}>
            <Button label="Try again" variant="primary" hasTVPreferredFocus onPress={() => setProblem(false)} />
            <Button label="Back to guide" onPress={() => go('Live', {tab: 'guide'})} />
          </View>
        </View>
      ) : null}
      {chrome || guide ? (
        <TVFocusGuideView autoFocus style={StyleSheet.absoluteFill} pointerEvents="box-none">
          <View style={styles.shade} pointerEvents="none" />
          <View style={[styles.top, {paddingTop: insets.top + (isTV ? 50 : 8), paddingHorizontal: pad}]}>
            {!isTV ? <IconButton icon="x" label="Close player" variant="glass" onPress={back} /> : null}
            <Artwork art={ch?.logo} tint={ch?.tint} style={{width: u(64), height: u(36), borderRadius: 6, marginHorizontal: u(12)}} />
            <View style={{flex: 1}}>
              <View style={{flexDirection: 'row', alignItems: 'center', gap: 8}}>
                <View style={styles.live}><Text variant="micro" tone="white">LIVE</Text></View>
                <Text variant="captionStrong" tone="secondary">{ch?.number} · {ch?.name}</Text>
              </View>
              <Text variant={isTV ? 'display' : 'title'} numberOfLines={1}>{cur?.title ?? 'No program information'}</Text>
            </View>
          </View>
          <View style={[styles.bottom, {paddingHorizontal: pad, paddingBottom: insets.bottom + (isTV ? 50 : 12)}]}>
            {cur ? (
              <>
                <Timeline position={now - cur.start - p.liveBehind * 1000} duration={cur.end - cur.start} live={{behind: p.liveBehind}} />
                <View style={{flexDirection: 'row', justifyContent: 'space-between'}}>
                  <Text variant="caption" tone="secondary">{fmtTime(cur.start)} – {fmtTime(cur.end)}</Text>
                  <Text variant="captionStrong" tone={p.liveBehind ? 'amber' : 'accent'}>{p.liveBehind ? `${Math.round(p.liveBehind / 60)} min behind live` : 'Live'}</Text>
                  <Text variant="caption" tone="secondary">{next ? `Next · ${fmtTime(next.start)} ${next.title}` : ''}</Text>
                </View>
              </>
            ) : null}
            <View style={styles.controls}>
              <IconButton icon="chevronUp" label="Previous channel" variant="glass" onPress={() => zap(-1)} />
              <IconButton icon="chevronDown" label="Next channel" variant="glass" onPress={() => zap(1)} />
              <IconButton icon="skipBack" label="Back 10 seconds" variant="glass" onPress={() => player.skip(-30)} />
              <IconButton icon={p.playing ? 'pause' : 'play'} label={p.playing ? 'Pause' : 'Play'} fill variant="glass" size={54} hasTVPreferredFocus onPress={player.toggle} />
              <IconButton icon="skipForward" label="Forward 10 seconds" variant="glass" onPress={() => player.skip(30)} disabled={!p.liveBehind} />
              <Button label="Live" icon="live" variant={p.liveBehind ? 'primary' : 'glass'} size="sm" onPress={player.jumpToLive} />
              <Button label="Guide" icon="list" variant={guide ? 'primary' : 'glass'} size="sm" onPress={() => setGuide(!guide)} />
              <IconButton icon="subtitles" label={p.captions ? 'Turn off captions' : 'Turn on captions'} active={p.captions} variant="glass" onPress={() => player.set({captions: !p.captions})} />
              {cur ? <IconButton icon="record" label="Record" variant="glass" color={colors.record} onPress={() => openProgramme(s, cur, ch)} /> : null}
              <IconButton icon="alert" label="Simulate a channel problem" variant="glass" onPress={() => setProblem(true)} />
            </View>
            {guide ? <MiniGuide list={list} progs={progs.data ?? []} current={channelId} onPick={id => { setChannelId(id); setGuide(false); }} /> : null}
          </View>
        </TVFocusGuideView>
      ) : null}
    </Pressable>
  );
}

function MiniGuide({list, progs, current, onPick}: {list: Channel[]; progs: Programme[]; current: string; onPick: (id: string) => void}) {
  const now = Date.now();
  return (
    <ScrollView horizontal style={{marginTop: u(12)}} contentContainerStyle={{gap: u(10)}} showsHorizontalScrollIndicator={false}>
      {list.map(c => {
        const cur = progs.find(p => p.channelId === c.id && p.start <= now && p.end > now);
        return (
          <Focusable key={c.id} onPress={() => onPick(c.id)} hasTVPreferredFocus={c.id === current} scale={1.06} accessibilityLabel={`${c.name}, now ${cur?.title ?? ''}`}>
            {({focused}) => (
              <View style={[styles.mini, c.id === current && {borderColor: colors.accentStrong}, focused && isTV && {backgroundColor: colors.white}]}>
                <Artwork art={c.logo} tint={c.tint} style={{width: u(90), height: u(50), borderRadius: 6}} />
                <Text variant="captionStrong" numberOfLines={1} color={focused && isTV ? colors.actionInk : colors.text} style={{marginTop: 6}}>{c.number} {c.name}</Text>
                <Text variant="caption" numberOfLines={1} color={focused && isTV ? 'rgba(3,18,28,0.7)' : colors.textTertiary}>{cur?.title ?? 'No information'}</Text>
              </View>
            )}
          </Focusable>
        );
      })}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  center: {...StyleSheet.absoluteFillObject, alignItems: 'center', justifyContent: 'center'},
  shade: {...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(0,0,0,0.35)'},
  top: {flexDirection: 'row', alignItems: 'center'},
  bottom: {position: 'absolute', left: 0, right: 0, bottom: 0},
  controls: {flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: u(10), marginTop: u(12)},
  live: {backgroundColor: colors.record, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 1},
  captions: {position: 'absolute', left: 40, right: 40, alignItems: 'center'},
  capText: {backgroundColor: 'rgba(0,0,0,0.7)', paddingHorizontal: 12, paddingVertical: 4, borderRadius: 4, overflow: 'hidden'},
  mini: {width: u(170), padding: u(8), borderRadius: radius.md, backgroundColor: 'rgba(19,38,52,0.9)', borderWidth: 2, borderColor: 'transparent'},
});

export {Badge};
