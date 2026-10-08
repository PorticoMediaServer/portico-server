import React, {useCallback, useEffect, useRef, useState} from 'react';
import {ActivityIndicator, Animated, Easing, Pressable, StyleSheet, TVFocusGuideView, View, useTVEventHandler} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {useClient, useQuery} from '../data/DataProvider';
import type {Chapter, Entry, Marker} from '../data/types';
import {Artwork} from '../ui/Artwork';
import {Scrim} from '../ui/Canvas';
import {Button, IconButton} from '../ui/Button';
import {Text} from '../ui/Text';
import {Icon} from '../ui/Icon';
import {Focusable} from '../ui/Focusable';
import {Badge, ProgressBar} from '../ui/Controls';
import {ActionMenu} from '../ui/Sheet';
import {formatTime, usePlayer, usePlayerState} from './PlayerProvider';
import {Timeline} from './Timeline';
import {PlayerOptions, type OptionsTab} from './PlayerOptions';
import {QueuePanel} from './QueuePanel';
import {PlayOnSheet} from './PlayOn';
import {MiniGuide} from './MiniGuide';
import {GroupPill} from './GroupPill';
import {usePrefs} from '../app/prefs';
import {useActions} from '../app/actions';
import {stillFor} from '../data/fixtures/catalog';
import {openEntry, root} from '../nav/nav';

/** Full-screen video and live player (simulated picture, real chrome). */
export function VideoPlayer() {
  const s = usePlayerState();
  const player = usePlayer();
  const client = useClient();
  const actions = useActions();
  const prefs = usePrefs();
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const entry = s.queue[s.index];
  const live = s.mode === 'live';
  const [chrome, setChrome] = useState(true);
  const [panel, setPanel] = useState<'options' | 'queue' | 'playon' | 'more' | 'guide' | 'chapters'>();
  const [optionsTab, setOptionsTab] = useState<OptionsTab>('audio');
  const hideTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const opacity = useRef(new Animated.Value(1)).current;
  const zoom = useRef(new Animated.Value(1)).current;
  const [episodesInRow, setEpisodesInRow] = useState(0);

  const detail = useQuery(async c => {
    if (!entry || live) return undefined;
    if (entry.kind === 'episode') { const d = await c.catalog.episode(entry.id); return {chapters: [] as Chapter[], markers: d.markers, next: d.next, show: d.show}; }
    if (entry.kind === 'movie' && c.catalog.entry(entry.id)) { const d = await c.catalog.movie(entry.id); return {chapters: d.chapters, markers: [{kind: 'credits' as const, start: (d.entry.duration ?? 0) - 300, end: d.entry.duration ?? 0}], related: d.related.slice(0, 6)}; }
    return undefined;
  }, [entry?.id, live]);
  const chapters = detail.data?.chapters ?? [];
  const markers: Marker[] = detail.data?.markers ?? [];

  const poke = useCallback(() => {
    setChrome(true);
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setChrome(false), 5000);
  }, []);
  useEffect(() => { poke(); return () => { if (hideTimer.current) clearTimeout(hideTimer.current); }; }, [poke, entry?.id]);
  useEffect(() => { if (!s.playing) setChrome(true); else poke(); }, [s.playing, poke]);
  useEffect(() => { Animated.timing(opacity, {toValue: chrome || panel ? 1 : 0, duration: 220, useNativeDriver: true}).start(); }, [chrome, panel, opacity]);
  useEffect(() => {
    if (prefs.reduceMotion) return;
    const loop = Animated.loop(Animated.sequence([Animated.timing(zoom, {toValue: 1.08, duration: 22000, easing: Easing.inOut(Easing.quad), useNativeDriver: true}), Animated.timing(zoom, {toValue: 1, duration: 22000, easing: Easing.inOut(Easing.quad), useNativeDriver: true})]));
    if (s.playing) loop.start();
    return () => loop.stop();
  }, [s.playing, zoom, prefs.reduceMotion, entry?.id]);

  useTVEventHandler(evt => {
    if (panel) return;
    if (evt.eventType === 'playPause') { player.toggle(); poke(); return; }
    if (!chrome && ['up', 'down', 'left', 'right', 'select', 'swipeUp', 'swipeDown'].includes(evt.eventType)) poke();
    else if (chrome && evt.eventType !== 'blur' && evt.eventType !== 'focus') poke();
    if (live && evt.eventType === 'longUp') setPanel('guide');
  });

  // Skip intro / credits prompts, auto-skip per preferences.
  const activeMarker = markers.find(mk => s.position >= mk.start && s.position < mk.end - 2);
  useEffect(() => {
    if (!activeMarker) return;
    const pref = activeMarker.kind === 'intro' ? prefs.introSkip : activeMarker.kind === 'credits' ? prefs.creditsSkip : 'auto';
    if (pref === 'auto' && activeMarker.kind !== 'credits') player.seek(activeMarker.end);
  }, [activeMarker?.kind, activeMarker?.start]); // eslint-disable-line react-hooks/exhaustive-deps

  // Live: ask the server to start the channel, and say plainly why when it can't.
  const [problem, setProblem] = useState<string>();
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (!live || !s.liveChannelId) { setProblem(undefined); return; }
    let alive = true;
    setProblem(undefined);
    void client.live.start(s.liveChannelId).then(r => {
      if (!alive) return;
      if (r.ok) { if (attempt > 0) player.set({buffering: false, playing: true}); return; }
      setProblem(r.reason);
      player.set({playing: false, buffering: false});
    });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, s.liveChannelId, attempt]);
  if (!entry) return null;
  const next = s.queue[s.index + 1];
  const title = entry.kind === 'episode' ? entry.subtitle ?? entry.title : entry.title;
  const subtitle = entry.kind === 'episode' ? `${entry.caption?.split(' · ').filter(p => /^(S\d|E\d)/.test(p)).join(' · ')} · ${entry.title}` : live ? entry.subtitle : [entry.year, entry.rating].filter(Boolean).join(' · ');
  const picture = live ? entry.image : entry.kind === 'episode' ? entry.image : entry.backdrop ?? entry.image;
  const creditsReached = s.mode === 'video' && (s.ended || (activeMarker?.kind === 'credits' && next && prefs.autoplayNext));
  const pad = m.isTV ? 72 : m.isPhone ? 16 : 32;
  if (live && problem) {
    const channel = entry.subtitle?.split(' · ').pop() ?? 'This channel';
    const why: Record<string, string> = {
      tunersBusy: 'All tuners are busy recording or in use. Try again when one frees up.',
      sourceUnavailable: 'The channel’s source isn’t responding. Try again in a minute.',
      storage: 'The server has no room to buffer live TV right now. The owner can free up space in Server › Storage.',
      stalled: 'The channel took too long to get ready. Try again.',
      startFailed: 'The server couldn’t start this channel. Try again, or pick another channel.',
    };
    return (
      <View style={{flex: 1, backgroundColor: colors.ink, alignItems: 'center', justifyContent: 'center', padding: pad, gap: m.space.md}}>
        <Icon name="live" size={m.isTV ? 72 : 48} color={colors.amber} />
        <Text variant={m.isTV ? 'title' : 'heading'} center>{channel} can’t start right now</Text>
        <Text variant="body" tone="secondary" center style={{maxWidth: 640}}>{why[problem] ?? why.startFailed}</Text>
        <View style={{flexDirection: 'row', gap: m.space.sm, marginTop: m.space.sm}}>
          <Button label="Try again" icon="refresh" variant="primary" hasTVPreferredFocus onPress={() => { player.set({buffering: true, playing: true}); setAttempt(a => a + 1); }} />
          <Button label="Back to guide" icon="guide" onPress={() => { player.close(); root('Live', {view: 'guide'}); }} />
        </View>
      </View>
    );
  }
  const stillWatching = episodesInRow >= 3 && s.ended;

  return (
    <View style={{flex: 1, backgroundColor: '#000'}}>
      {m.isTV ? (
        <View style={StyleSheet.absoluteFill} pointerEvents="none">
        <Animated.View style={[StyleSheet.absoluteFill, {transform: [{scale: zoom}], opacity: s.playing ? 1 : 0.8}]}>
          <Artwork source={picture} title={entry.title} width="100%" height="100%" radius={0} fade={false} />
        </Animated.View>
        </View>
      ) : (
      <Pressable style={StyleSheet.absoluteFill} onPress={() => (chrome ? setChrome(false) : poke())} accessible={false} focusable={false}>
        <Animated.View style={[StyleSheet.absoluteFill, {transform: [{scale: zoom}], opacity: s.playing ? 1 : 0.8}]}>
          <Artwork source={picture} title={entry.title} width="100%" height="100%" radius={0} fade={false} />
        </Animated.View>
      </Pressable>
      )}
      {s.subtitles !== 'off' && !creditsReached ? (
        <View pointerEvents="none" style={{position: 'absolute', left: 0, right: 0, bottom: chrome ? (m.isTV ? 260 : 150) : (m.isTV ? 90 : 60), alignItems: 'center'}}>
          <Text variant={m.isTV ? 'heading' : 'bodyStrong'} style={{backgroundColor: 'rgba(0,0,0,0.55)', paddingHorizontal: 12, paddingVertical: 4, borderRadius: 6, textAlign: 'center', maxWidth: '80%'}}>{live ? '[Live captions] …and the forecast for the coast tonight is clear.' : ['I told you the tide would turn.', 'Then why are we still here?', 'Because someone has to keep the light on.', '[distant thunder]'][Math.floor(s.position / 7) % 4]}</Text>
        </View>
      ) : null}
      {s.buffering ? (
        <View pointerEvents="none" style={[StyleSheet.absoluteFill, {alignItems: 'center', justifyContent: 'center', gap: 12}]}>
          <ActivityIndicator size="large" color={colors.text} />
          <Text variant="bodyStrong">{live ? 'Tuning…' : 'Getting ready'}</Text>
        </View>
      ) : null}
      {s.cast ? (
        <View pointerEvents="none" style={[StyleSheet.absoluteFill, {alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(7,16,24,0.78)', gap: 12}]}>
          <Icon name={s.cast.kind === 'airplay' ? 'airplay' : 'cast'} size={m.isTV ? 80 : 56} color={colors.accentStrong} />
          <Text variant="heading">Playing on {s.cast.name}</Text>
          <Text variant="body" tone="secondary">Use the other device’s remote or app to control it.</Text>
        </View>
      ) : null}

      <Animated.View pointerEvents={chrome || panel ? 'box-none' : 'none'} style={[StyleSheet.absoluteFill, {opacity}]}>
        <Scrim width={m.width} height={m.height} left={false} top strength={1.1} />
        <TVFocusGuideView style={{flex: 1, justifyContent: 'space-between', paddingTop: m.isTV ? 56 : insets.top + 8, paddingBottom: m.isTV ? 56 : insets.bottom + 12, paddingHorizontal: pad}}>
          <View style={{flexDirection: 'row', alignItems: 'flex-start', gap: m.space.md}}>
            {m.isTV ? null : <IconButton icon="chevronDown" label="Close player" variant="secondary" onPress={player.close} />}
            <View style={{flex: 1}}>
              <View style={{flexDirection: 'row', alignItems: 'center', gap: 10}}>
                {live ? <Badge label="LIVE" tone="record" /> : null}
                <Text variant={m.isTV ? 'title' : 'heading'} numberOfLines={1}>{title}</Text>
              </View>
              {subtitle ? <Text variant="body" tone="secondary" numberOfLines={1}>{subtitle}</Text> : null}
              {live && s.position > 30 ? <Text variant="caption" tone="amber">{formatTime(s.position)} behind live</Text> : null}
            </View>
            <GroupPill inline />
            <IconButton icon="cast" label="Play on" variant="secondary" onPress={() => setPanel('playon')} selected={!!s.cast} />
            <IconButton icon="subtitles" label={s.subtitles === 'off' ? 'Turn on captions' : 'Turn off captions'} variant="secondary" selected={s.subtitles !== 'off'} onPress={() => player.set({subtitles: s.subtitles === 'off' ? 's1' : 'off'})} />
            <IconButton icon="settings" label="Playback options" variant="secondary" onPress={() => { setOptionsTab('audio'); setPanel('options'); }} />
          </View>

          <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: m.isTV ? 36 : 28}}>
            {live ? <IconButton icon="chevronDown" label="Previous channel" variant="secondary" onPress={() => setPanel('guide')} /> : entry.kind === 'episode' ? <IconButton icon="previous" label="Previous" onPress={player.previous} disabled={s.index === 0 && s.position < 5} /> : null}
            {!live ? <IconButton icon="skipBack" label={`Back ${prefs.skipBack} seconds`} size={m.isTV ? 80 : 56} onPress={() => { player.skip(-prefs.skipBack); poke(); }} /> : null}
            <IconButton icon={s.playing ? 'pause' : 'play'} label={s.playing ? 'Pause' : 'Play'} size={m.isTV ? 112 : 76} filled onPress={() => { player.toggle(); poke(); }} hasTVPreferredFocus />
            {!live ? <IconButton icon="skipForward" label={`Forward ${prefs.skipForward} seconds`} size={m.isTV ? 80 : 56} onPress={() => { player.skip(prefs.skipForward); poke(); }} /> : null}
            {live ? <IconButton icon="chevronUp" label="Next channel" variant="secondary" onPress={() => setPanel('guide')} /> : entry.kind === 'episode' ? <IconButton icon="next" label="Next" onPress={player.next} disabled={!next} /> : null}
          </View>

          <View style={{gap: m.space.md}}>
            {!live ? <Timeline position={s.position} duration={s.duration} onSeek={sec => { player.seek(sec); poke(); }} chapters={chapters} thumbFor={sec => stillFor(`${entry.id}-thumb-${Math.floor(sec / 60)}`)} onActivity={poke} /> : (
              <View style={{gap: 6}}>
                <ProgressBar value={0.6} height={m.isTV ? 8 : 5} color={colors.record} />
                <View style={{flexDirection: 'row', justifyContent: 'space-between'}}><Text variant="captionStrong" tone="secondary">{entry.caption ?? 'On now'}</Text><Text variant="captionStrong" tone="secondary">Until {new Date(Date.now() + 22 * 60000).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}</Text></View>
              </View>
            )}
            <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm, alignItems: 'center'}}>
              <Button size="sm" icon="audio" label="Audio & subtitles" variant="secondary" onPress={() => { setOptionsTab('subtitles'); setPanel('options'); }} />
              <Button size="sm" icon="quality" label={s.quality === 'auto' ? 'Automatic' : s.quality === 'original' ? 'Original' : `${s.quality}p`} variant="secondary" onPress={() => { setOptionsTab('quality'); setPanel('options'); }} />
              {!live ? <Button size="sm" icon="speed" label={s.speed === 1 ? 'Speed' : `${s.speed}×`} variant="secondary" onPress={() => { setOptionsTab('speed'); setPanel('options'); }} /> : null}
              {chapters.length ? <Button size="sm" icon="chapters" label="Chapters" variant="secondary" onPress={() => setPanel('chapters')} /> : null}
              {live ? <Button size="sm" icon="guide" label="Guide" variant="secondary" onPress={() => setPanel('guide')} /> : null}
              {live ? <Button size="sm" icon="record" label="Record" variant="secondary" onPress={() => { void client.live.record(`${s.liveChannelId}@${Math.floor(Date.now() / 1800000) * 1800000}`); }} /> : null}
              {!live && s.queue.length > 1 ? <Button size="sm" icon="queue" label="Up next" variant="secondary" onPress={() => setPanel('queue')} /> : null}
              {s.sleepEndsAt ? <Badge label={`Sleep · ${s.sleepLabel}`} icon="sleep" tone="accent" /> : null}
              <View style={{flex: 1}} />
              <Button size="sm" icon="more" label="More" variant="ghost" onPress={() => setPanel('more')} />
            </View>
          </View>
        </TVFocusGuideView>
      </Animated.View>

      {activeMarker && !creditsReached && !panel ? (
        <View style={{position: 'absolute', right: pad, bottom: chrome ? (m.isTV ? 250 : 170) : (m.isTV ? 72 : 40)}}>
          <Button label={activeMarker.kind === 'intro' ? 'Skip intro' : activeMarker.kind === 'recap' ? 'Skip recap' : 'Skip credits'} icon="next" variant="primary" size="lg" hasTVPreferredFocus onPress={() => player.seek(activeMarker.end)} />
        </View>
      ) : null}

      {creditsReached && !panel ? (
        <PostPlay next={next} related={(detail.data as {related?: Entry[]} | undefined)?.related} stillWatching={stillWatching}
          onPlayNext={() => { setEpisodesInRow(n => n + 1); player.next(); }}
          onReplay={() => player.seek(0)} onClose={player.close} onKeepWatching={() => { setEpisodesInRow(0); player.next(); }}
          onOpen={e => { player.close(); openEntry(e); }} />
      ) : null}

      <PlayerOptions visible={panel === 'options'} onClose={() => setPanel(undefined)} entry={entry} initialTab={optionsTab} mode={live ? 'live' : 'video'} />
      <QueuePanel visible={panel === 'queue'} onClose={() => setPanel(undefined)} />
      <PlayOnSheet visible={panel === 'playon'} onClose={() => setPanel(undefined)} />
      <MiniGuide visible={panel === 'guide'} onClose={() => setPanel(undefined)} />
      <ActionMenu visible={panel === 'chapters'} onClose={() => setPanel(undefined)} title="Chapters" items={chapters.map((c, i) => ({id: String(i), label: c.title, subtitle: formatTime(c.start), icon: 'chapters', selected: s.position >= c.start && (chapters[i + 1]?.start ?? Infinity) > s.position}))} onSelect={id => { player.seek(chapters[Number(id)]!.start); setPanel(undefined); }} />
      <ActionMenu visible={panel === 'more'} onClose={() => setPanel(undefined)} title={entry.title} items={[
        {id: 'info', label: 'Playback info', icon: 'info'},
        {id: 'queue', label: 'Up next', icon: 'queue', hidden: live},
        {id: 'details', label: 'Go to details', icon: 'forward', hidden: live},
        {id: 'rate', label: 'Rate this', icon: 'star', hidden: live},
        {id: 'pip', label: 'Picture in Picture', icon: 'pip', hidden: m.isTV},
        {id: 'report', label: 'Report a problem', icon: 'flag'},
      ]} onSelect={id => {
        setPanel(undefined);
        if (id === 'info') setTimeout(() => { setOptionsTab('info'); setPanel('options'); }, 250);
        if (id === 'queue') setTimeout(() => setPanel('queue'), 250);
        if (id === 'details') { player.close(); openEntry(entry); }
        if (id === 'rate') { player.close(); setTimeout(() => actions.more(entry), 300); }
        if (id === 'pip') player.minimize();
        if (id === 'report') { player.set({playing: false}); setTimeout(() => actions.feedback(entry), 250); }
      }} />
    </View>
  );
}

/** End of an episode: Up Next with a countdown. End of a film: replay or more like it. */
function PostPlay({next, related, stillWatching, onPlayNext, onReplay, onClose, onKeepWatching, onOpen}: {next?: Entry; related?: Entry[]; stillWatching: boolean; onPlayNext: () => void; onReplay: () => void; onClose: () => void; onKeepWatching: () => void; onOpen: (e: Entry) => void}) {
  const m = useMetrics();
  const prefs = usePrefs();
  const s = usePlayerState();
  const [left, setLeft] = useState(prefs.countdown);
  const [cancelled, setCancelled] = useState(false);
  useEffect(() => {
    if (!next || cancelled || stillWatching || !prefs.autoplayNext) return;
    if (left <= 0) { onPlayNext(); return; }
    const t = setTimeout(() => setLeft(l => l - 1), 1000);
    return () => clearTimeout(t);
  }, [left, next, cancelled, stillWatching, prefs.autoplayNext, onPlayNext]);
  const w = m.isTV ? 420 : m.isPhone ? 200 : 300;
  return (
    <View style={{position: 'absolute', right: m.isTV ? 72 : 16, bottom: m.isTV ? 72 : 24, left: m.isPhone ? 16 : undefined, backgroundColor: colors.overlay, borderRadius: m.radius.sheet, padding: m.space.lg, gap: m.space.md, borderWidth: 1, borderColor: colors.line, maxWidth: m.isTV ? 900 : undefined}}>
      {stillWatching ? (
        <>
          <Text variant="heading">Still watching?</Text>
          <Text variant="body" tone="secondary">Playback paused after several episodes. Confirm to keep going.</Text>
          <View style={{flexDirection: 'row', gap: m.space.sm}}><Button label="I’m still watching" variant="primary" onPress={onKeepWatching} hasTVPreferredFocus /><Button label="Close" variant="ghost" onPress={onClose} /></View>
        </>
      ) : next ? (
        <View style={{flexDirection: m.isPhone ? 'column' : 'row', gap: m.space.md}}>
          <Artwork source={next.image} title={next.title} width={w} height={w * 0.5625} radius={m.radius.card} />
          <View style={{gap: 6, flex: m.isPhone ? undefined : 1, minWidth: m.isTV ? 360 : 200}}>
            <Text variant="label" tone="accent">Up next</Text>
            <Text variant="subheading" numberOfLines={2}>{next.title}</Text>
            <Text variant="caption" tone="tertiary" numberOfLines={1}>{next.caption ?? next.subtitle}</Text>
            <Text variant="caption" tone="secondary">{cancelled || !prefs.autoplayNext ? 'Ready when you are' : `Playing in ${left}s`}</Text>
            <View style={{flexDirection: 'row', gap: m.space.sm, marginTop: 6}}>
              <Button label="Play now" icon="play" variant="primary" onPress={onPlayNext} hasTVPreferredFocus />
              {!cancelled ? <Button label="Cancel" variant="ghost" onPress={() => setCancelled(true)} /> : <Button label="Close" variant="ghost" onPress={onClose} />}
            </View>
          </View>
        </View>
      ) : (
        <>
          <Text variant="heading">{s.ended ? 'Finished' : 'Credits'}</Text>
          {related?.length ? (
            <>
              <Text variant="label" tone="tertiary">More like this</Text>
              <View style={{flexDirection: 'row', gap: m.space.sm}}>
                {related.slice(0, m.isPhone ? 3 : 5).map(r => (
                  <Focusable key={r.id} onPress={() => onOpen(r)} focusScale={1.06} style={({focused}) => ({borderRadius: 8, borderWidth: 3, borderColor: focused ? colors.focus : 'transparent'})}>
                    <Artwork source={r.image} title={r.title} width={m.isTV ? 120 : 70} height={m.isTV ? 180 : 105} radius={6} />
                  </Focusable>
                ))}
              </View>
            </>
          ) : null}
          <View style={{flexDirection: 'row', gap: m.space.sm}}><Button label="Replay" icon="refresh" variant="secondary" onPress={onReplay} hasTVPreferredFocus /><Button label="Close" variant="ghost" onPress={onClose} /></View>
        </>
      )}
    </View>
  );
}
