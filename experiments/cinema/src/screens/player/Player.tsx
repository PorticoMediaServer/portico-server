import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {ActivityIndicator, Animated, Easing, Pressable, StyleSheet, TVFocusGuideView, View, useTVEventHandler, useWindowDimensions} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {resolveArt} from '../../data/art';
import {useQuery, useServices} from '../../data/hooks';
import type {ArtRef, Episode, Item, Movie, Season} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {ListRow, Segmented, Stepper} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {glow} from '../../design/Glow';
import {Icon, type IconName} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {back as backNav} from '../../nav/back';
import {go, back as navBack, push} from '../../nav/routes';
import {player, usePlayer} from '../../state/player';
import {openRateSheet} from '../shared/actions';
import {code, fmtClock, toCard} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {playbackChoice} from '../title/parts';
import {GroupPill} from './GroupPill';
import {openPlayOn} from './PlayOn';
import {Timeline, type Mark} from './Timeline';

/** The simulated picture: the title's own frames, crossfading slowly with a gentle push-in. */
export function VideoSurface({frames, playing}: {frames: ArtRef[]; playing: boolean}) {
  const [i, setI] = useState(0);
  useEffect(() => {
    if (!playing || frames.length < 2) return;
    const t = setInterval(() => setI(v => (v + 1) % frames.length), 6500);
    return () => clearInterval(t);
  }, [playing, frames.length]);
  return (
    <View style={[StyleSheet.absoluteFill, {backgroundColor: '#000'}]}>
      {frames.map((f, k) => <Frame key={f + k} art={f} on={k === i} playing={playing} />)}
    </View>
  );
}

function Frame({art, on, playing}: {art: ArtRef; on: boolean; playing: boolean}) {
  const o = useRef(new Animated.Value(on ? 1 : 0)).current;
  const sc = useRef(new Animated.Value(1)).current;
  useEffect(() => {
    Animated.timing(o, {toValue: on ? 1 : 0, duration: 900, useNativeDriver: true}).start();
    if (on && playing) {
      sc.setValue(1);
      Animated.timing(sc, {toValue: 1.06, duration: 7000, easing: Easing.linear, useNativeDriver: true}).start();
    } else sc.stopAnimation();
  }, [on, playing, o, sc]);
  const src = resolveArt(art);
  if (!src) return null;
  return <Animated.Image source={src} resizeMode="cover" style={[StyleSheet.absoluteFill, {width: '100%', height: '100%', opacity: o, transform: [{scale: sc}]}]} />;
}

type Tab = 'info' | 'chapters' | 'audio' | 'subtitles' | 'quality' | 'speed' | 'queue';

export function PlayerScreen() {
  const route = useRoute<RouteProp<Routes2, 'Player'>>();
  const s = useServices();
  const insets = useSafeAreaInsets();
  const {width} = useWindowDimensions();
  const p = usePlayer();
  const {id, from, queue, trailer} = route.params;
  const [chrome, setChrome] = useState(true);
  const [tab, setTab] = useState<Tab | null>(null);
  const [scrub, setScrub] = useState<number | null>(null);
  const [scrubFocus, setScrubFocus] = useState(false);
  const [dismissedPost, setDismissedPost] = useState(false);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const q = queue ?? [id];
    player.play(q, {kind: 'video', index: Math.max(0, q.indexOf(id)), from: from ?? 0});
    if (trailer) setTimeout(() => player.set({duration: 138}), 50);
    glow.setTint(null);
    return () => {
      if (!player.get().minimized) player.stop();
    };
  }, [id, from, queue, trailer]);

  // Until the store has switched to this video (a song may still be loaded), show the requested id.
  const currentId = p.kind === 'video' && p.itemId ? p.itemId : id;
  const itemQ = useQuery('item:' + currentId, () => s.catalog.item(currentId));
  // The store can still hold a song for a moment while a video loads; only render video items.
  const item = (itemQ.data && ['movie', 'homevideo', 'episode'].includes(itemQ.data.kind) ? itemQ.data : undefined) as Movie | Episode | undefined;
  const showQ = useQuery(item?.kind === 'episode' ? 'item:' + item.showId : null, () => s.catalog.item((item as Episode).showId));
  const seasonQ = useQuery(item?.kind === 'episode' ? 'item:' + item.seasonId : null, () => s.catalog.item((item as Episode).seasonId));
  const season = seasonQ.data as Season | undefined;
  const nextEpId = item?.kind === 'episode' && season ? season.episodeIds[season.episodeIds.indexOf(item.id) + 1] : undefined;
  const nextEp = useItems(nextEpId ? [nextEpId] : undefined);
  const related = useQuery(item && item.kind === 'movie' ? 'related:' + item.id : null, () => s.catalog.related(item!.id));
  const relatedItems = useItems(related.data?.slice(0, 6));

  const poke = useCallback(() => {
    setChrome(true);
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      if (player.get().playing && !tab) setChrome(false);
    }, 5000);
  }, [tab]);
  useEffect(() => {
    poke();
  }, [poke, p.playing]);

  // Back/Menu: close an open panel, then hide the chrome, then leave the player.
  useEffect(
    () =>
      backNav.register({
        priority: 50,
        active: () => true,
        handle: () => {
          if (tab) return setTab(null);
          if (chrome && player.get().playing && isTV) return setChrome(false);
          player.stop();
          navBack();
        },
      }),
    [tab, chrome],
  );

  useTVEventHandler(evt => {
    if (!evt) return;
    const t = evt.eventType;
    if (t === 'playPause') {
      player.toggle();
      poke();
      return;
    }
    if (scrubFocus && (t === 'left' || t === 'right')) {
      const base = scrub ?? player.get().position;
      setScrub(Math.max(0, Math.min(player.get().duration, base + (t === 'right' ? 10 : -10))));
      poke();
      return;
    }
    if (!chrome && (t === 'left' || t === 'right')) {
      player.skip(t === 'right' ? 10 : -10);
      ui.toast(t === 'right' ? 'Forward 10 seconds' : 'Back 10 seconds');
      return;
    }
    if (['up', 'down', 'left', 'right', 'select', 'swipeUp', 'swipeDown'].includes(t)) poke();
  });

  const frames = useMemo(() => {
    if (!item) return [];
    const f = (item.chapters ?? []).map(c => c.art).filter(Boolean) as ArtRef[];
    const extra = [item.art.still, item.art.backdrop, showQ.data?.art.backdrop].filter(Boolean) as ArtRef[];
    return [...new Set([...extra.slice(0, 1), ...f, ...extra])].slice(0, 10);
  }, [item, showQ.data]);

  if (!item) return <View style={styles.black}><ActivityIndicator color={colors.white} size="large" /></View>;
  const chapters: Mark[] = (item.chapters ?? []).map(c => ({start: c.start, title: c.title, art: c.art ?? item.art.backdrop}));
  const ep = item.kind === 'episode' ? item : undefined;
  const segments: Mark[] = ep ? [{start: 0, end: ep.introEnd ?? 0}, {start: ep.creditsStart ?? p.duration, end: p.duration}] : [];
  const inIntro = ep?.introEnd && p.position > 3 && p.position < ep.introEnd;
  const nearEnd = p.duration > 0 && p.position >= p.duration - (ep ? 45 : 25);
  const title = trailer ? `${item.title} · Trailer` : item.title;
  const subtitle = ep ? `${showQ.data?.title ?? ''} · ${code(ep.seasonNumber, ep.number)}` : `${item.year ?? ''}`;
  const choice = playbackChoice(item.id);
  const version = item.versions.find(v => v.id === choice.version) ?? item.versions[0];
  const audio = item.audioTracks.find(a => a.id === (p.audioTrack ?? choice.audio)) ?? item.audioTracks[0];
  const subtitleTrack = item.subtitleTracks.find(t => t.id === (p.subtitle ?? choice.subtitle));
  const shown = chrome || !p.playing || tab !== null;
  const pad = isTV ? 80 : 18;

  const tabs: {id: Tab; label: string; icon: IconName; detail: string}[] = [
    {id: 'info', label: 'Info', icon: 'info', detail: ''},
    ...(chapters.length ? [{id: 'chapters' as Tab, label: 'Chapters', icon: 'list' as IconName, detail: String(chapters.length)}] : []),
    {id: 'audio', label: 'Audio', icon: 'audio', detail: audio?.label ?? ''},
    {id: 'subtitles', label: 'Subtitles', icon: 'subtitles', detail: subtitleTrack?.label ?? 'Off'},
    {id: 'quality', label: 'Quality', icon: 'gauge', detail: p.quality === 'auto' ? 'Automatic' : p.quality === 'original' ? 'Original' : p.quality},
    {id: 'speed', label: 'Speed', icon: 'speed', detail: p.rate === 1 ? 'Normal' : `${p.rate}×`},
    {id: 'queue', label: 'Up next', icon: 'queue', detail: String(Math.max(0, p.queue.length - p.index - 1))},
  ];

  return (
    <Pressable style={styles.black} onPress={() => (shown && !isTV ? setChrome(false) : poke())} accessible={false} focusable={isTV ? !shown : undefined}>
      <VideoSurface frames={frames} playing={p.playing && !p.buffering} />
      {subtitleTrack ? (
        <View style={[styles.subs, {bottom: shown ? (isTV ? 260 : 150) : isTV ? 90 : 50}]} pointerEvents="none">
          <Text variant="bodyStrong" center style={styles.subText}>{p.position % 20 < 10 ? '— Where were you last night?' : '— Somewhere I shouldn’t have been.'}</Text>
        </View>
      ) : null}
      {p.buffering ? <View style={styles.center} pointerEvents="none"><ActivityIndicator size="large" color={colors.white} /><Text variant="caption" style={{marginTop: 10}}>Getting ready</Text></View> : null}
      {p.ownerStopped ? <OwnerStopped message={p.ownerStopped} /> : null}

      {shown ? (
        <TVFocusGuideView autoFocus style={StyleSheet.absoluteFill} pointerEvents="box-none">
          <View style={styles.shade} pointerEvents="none" />
          <View style={[styles.top, {paddingTop: insets.top + (isTV ? 50 : 8), paddingHorizontal: pad}]}>
            {!isTV ? <IconButton icon="chevronDown" label="Minimize player" variant="glass" onPress={() => { player.minimize(); navBack(); }} /> : null}
            <View style={{flex: 1, marginHorizontal: u(12)}}>
              <Text variant="eyebrow" tone="accent" numberOfLines={1}>{subtitle}</Text>
              <Text variant={isTV ? 'display' : 'title'} numberOfLines={1}>{title}</Text>
            </View>
            <GroupPill />
            {!isTV ? <IconButton icon="airplay" label="Play on" variant="glass" onPress={() => openPlayOn(item.title)} /> : null}
            {!isTV ? <IconButton icon="x" label="Close player" variant="glass" onPress={() => { player.stop(); navBack(); }} style={{marginLeft: 8}} /> : null}
          </View>
          {!isTV ? (
            <View style={styles.centerControls} pointerEvents="box-none">
              <IconButton icon="skipBack" label="Back 10 seconds" size={52} variant="glass" onPress={() => { player.skip(-10); poke(); }} />
              <IconButton icon={p.playing ? 'pause' : 'play'} label={p.playing ? 'Pause' : 'Play'} size={72} variant="glass" fill onPress={() => { player.toggle(); poke(); }} />
              <IconButton icon="skipForward" label="Forward 10 seconds" size={52} variant="glass" onPress={() => { player.skip(10); poke(); }} />
            </View>
          ) : null}
          <View style={[styles.bottom, {paddingHorizontal: pad, paddingBottom: insets.bottom + (isTV ? 50 : 12)}]}>
            {isTV ? (
              <Focusable
                hasTVPreferredFocus
                scale={1}
                onFocus={() => setScrubFocus(true)}
                onBlur={() => { setScrubFocus(false); setScrub(null); }}
                onPress={() => { if (scrub !== null) { player.seek(scrub); setScrub(null); } else player.toggle(); poke(); }}
                accessibilityLabel={`Playback position ${fmtClock(p.position)} of ${fmtClock(p.duration)}`}>
                {() => <Timeline position={p.position} duration={p.duration} chapters={chapters} segments={segments} focused={scrubFocus} preview={scrub} />}
              </Focusable>
            ) : (
              <Timeline position={p.position} duration={p.duration} chapters={chapters} segments={segments} onSeek={t => { player.seek(t); poke(); }} />
            )}
            <View style={styles.times}>
              <Text variant="captionStrong">{fmtClock(scrub ?? p.position)}</Text>
              <Text variant="caption" tone="secondary">{p.playing ? '' : 'Paused · '}{version?.label ?? ''}{p.rate !== 1 ? ` · ${p.rate}×` : ''}</Text>
              <Text variant="captionStrong">-{fmtClock(p.duration - (scrub ?? p.position))}</Text>
            </View>
            <View style={styles.tabs}>
              {tabs.map(t => (
                <Focusable key={t.id} onPress={() => setTab(tab === t.id ? null : t.id)} scale={1.06} accessibilityLabel={`${t.label}${t.detail ? ', ' + t.detail : ''}`}>
                  {({focused}) => (
                    <View style={[styles.tab, (focused && isTV) && {backgroundColor: colors.white}, tab === t.id && !(focused && isTV) && {backgroundColor: 'rgba(92,179,226,0.25)'}]}>
                      <Icon name={t.icon} size={15} color={focused && isTV ? colors.actionInk : colors.text} />
                      <Text variant="captionStrong" color={focused && isTV ? colors.actionInk : colors.text} style={{marginLeft: 6}}>{t.label}</Text>
                      {t.detail && (isTV || t.id !== 'info') ? <Text variant="caption" color={focused && isTV ? 'rgba(3,18,28,0.6)' : colors.textTertiary} style={{marginLeft: 6}} numberOfLines={1}>{t.detail}</Text> : null}
                    </View>
                  )}
                </Focusable>
              ))}
              <Focusable onPress={() => openMore(item, s)} scale={1.06} accessibilityLabel="More">
                {({focused}) => <View style={[styles.tab, focused && isTV && {backgroundColor: colors.white}]}><Icon name="more" size={15} color={focused && isTV ? colors.actionInk : colors.text} /></View>}
              </Focusable>
            </View>
            {tab ? <OptionsPanel tab={tab} item={item} s={s} onClose={() => setTab(null)} onJump={t => { player.seek(t); setTab(null); }} /> : null}
          </View>
        </TVFocusGuideView>
      ) : null}

      {inIntro && !nearEnd ? (
        <View style={[styles.skip, {bottom: shown ? (isTV ? 300 : 170) : isTV ? 90 : 60, right: pad}]}>
          <Button label="Skip intro" icon="ff" variant="glass" size="lg" hasTVPreferredFocus={!shown} onPress={() => player.seek(ep!.introEnd!)} />
        </View>
      ) : null}
      {nearEnd && !dismissedPost ? (
        <PostPlay item={item} nextEp={nextEpId ? (nextEp.lookup(nextEpId) as Episode | undefined) : undefined} related={(related.data ?? []).map(r => relatedItems.lookup(r)).filter(Boolean) as Item[]} onDismiss={() => setDismissedPost(true)} width={width} s={s} />
      ) : null}
    </Pressable>
  );
}

type Routes2 = {Player: {id: string; from?: number; queue?: string[]; trailer?: boolean}};

function OptionsPanel({tab, item, s, onClose, onJump}: {tab: Tab; item: Movie | Episode; s: ReturnType<typeof useServices>; onClose: () => void; onJump: (t: number) => void}) {
  const p = usePlayer();
  const queueItems = useItems(p.queue);
  const choice = playbackChoice(item.id);
  const row = (label: string, checked: boolean, onPress: () => void, help?: string, first?: boolean) => <ListRow key={label} label={label} help={help} onPress={onPress} chevron={false} compact hasTVPreferredFocus={first} right={checked ? <Icon name="check" size={18} color={colors.accentStrong} /> : undefined} />;
  let body: React.ReactNode = null;
  if (tab === 'info') {
    const v = item.versions[0];
    body = (
      <View style={{flexDirection: isTV ? 'row' : 'column', gap: u(16)}}>
        <Artwork art={item.art.poster ?? item.art.still} tint={item.tint} style={{width: u(90), height: u(135), borderRadius: 8}} />
        <View style={{flex: 1}}>
          <Text variant="bodyStrong">{item.title}</Text>
          <Text variant="caption" tone="secondary" numberOfLines={3}>{item.synopsis}</Text>
          <Text variant="caption" tone="tertiary" style={{marginTop: 8}}>Playing the original file · {v?.video} · {v?.audio}</Text>
          <Text variant="caption" tone="tertiary">Connection: home network · Direct play · no conversion needed</Text>
          <View style={{flexDirection: 'row', gap: 8, marginTop: 10}}>
            <Button label="Rate this" icon="star" size="sm" onPress={() => openRateSheet(s, item)} hasTVPreferredFocus />
            <Button label="Report a problem" icon="flag" size="sm" onPress={() => go('Feedback', {itemId: item.id, playback: true})} />
          </View>
        </View>
      </View>
    );
  } else if (tab === 'chapters') {
    body = (
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: u(10)}}>
        {item.chapters.map((c, i) => (
          <Focusable key={i} onPress={() => onJump(c.start)} hasTVPreferredFocus={i === 0} scale={1.06} accessibilityLabel={`Chapter ${i + 1}, ${c.title}`}>
            {({focused}) => (
              <View style={{width: u(150)}}>
                <Artwork art={c.art ?? item.art.backdrop} tint={item.tint} style={[{width: u(150), height: u(84), borderRadius: 8}, focused && isTV && {borderWidth: 3, borderColor: colors.white}]} />
                <Text variant="caption" numberOfLines={1} style={{marginTop: 4}}>{i + 1}. {c.title}</Text>
                <Text variant="micro" tone="tertiary">{fmtClock(c.start)}</Text>
              </View>
            )}
          </Focusable>
        ))}
      </View>
    );
  } else if (tab === 'audio') {
    body = item.audioTracks.map((a, i) => row(a.label, a.id === (p.audioTrack ?? choice.audio ?? item.audioTracks[0].id), () => player.set({audioTrack: a.id}), [a.codec, a.channels].filter(Boolean).join(' · '), i === 0));
  } else if (tab === 'subtitles') {
    const cur = p.subtitle ?? choice.subtitle ?? 'off';
    body = (
      <>
        {row('Off', cur === 'off', () => player.set({subtitle: 'off'}), undefined, true)}
        {item.subtitleTracks.map(t => row(t.label + (t.forced ? ' · Forced' : ''), t.id === cur, () => player.set({subtitle: t.id}), t.origin === 'embedded' ? 'In the file' : t.origin === 'sidecar' ? 'Beside the file' : 'Downloaded'))}
        <Stepper label="Timing" help="Move subtitles earlier or later" value={p.subtitleOffset} onChange={v => player.set({subtitleOffset: v})} min={-10} max={10} step={0.5} unit="s" />
        <ListRow icon="search" label="Find subtitles…" help="Search providers for this title" onPress={() => ui.menu({title: 'Find subtitles', subtitle: 'Results from OpenSubtitles', actions: [{label: 'English · Blu-ray release', detail: 'Rated 9.1 · matches this file', onPress: () => ui.toast('Subtitles added.')}, {label: 'English (SDH) · Web release', detail: 'Rated 8.4', onPress: () => ui.toast('Subtitles added.')}, {label: 'German · Blu-ray release', detail: 'Rated 8.8', onPress: () => ui.toast('Subtitles added.')}]})} compact />
      </>
    );
  } else if (tab === 'quality') {
    body = (
      <>
        {row('Automatic', p.quality === 'auto', () => player.set({quality: 'auto'}), 'Best quality your connection allows', true)}
        {row('Original', p.quality === 'original', () => player.set({quality: 'original'}), 'Plays the file as is')}
        {(['1080p', '720p', '480p'] as const).map(q => row(`Convert to ${q}`, p.quality === q, () => player.set({quality: q}), q === '1080p' ? 'About 8 Mb/s' : q === '720p' ? 'About 4 Mb/s' : 'About 1.5 Mb/s'))}
        <Text variant="caption" tone="tertiary" style={{marginTop: 8}}>Connection: home network · up to 4K</Text>
      </>
    );
  } else if (tab === 'speed') {
    body = <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>{[0.5, 0.75, 1, 1.25, 1.5, 2].map((r, i) => <Button key={r} label={r === 1 ? 'Normal' : `${r}×`} active={p.rate === r} hasTVPreferredFocus={i === 2} onPress={() => player.set({rate: r})} />)}</View>;
  } else if (tab === 'queue') {
    body = (
      <>
        {p.queue.length <= 1 ? <Text variant="caption" tone="tertiary">Nothing is queued after this.</Text> : null}
        {p.queue.map((qid, i) => {
          const it = queueItems.lookup(qid);
          return <ListRow key={qid + i} label={`${i === p.index ? 'Now playing · ' : ''}${it?.title ?? 'Still loading'}`} help={it?.kind === 'episode' ? code((it as Episode).seasonNumber, (it as Episode).number) : undefined} onPress={() => i !== p.index && player.jumpTo(i)} chevron={false} compact hasTVPreferredFocus={i === 0} right={i !== p.index ? <View style={{flexDirection: 'row'}}><IconButton icon="arrowUp" label="Move up" size={30} variant="ghost" onPress={() => player.move(i, i - 1)} /><IconButton icon="arrowDown" label="Move down" size={30} variant="ghost" onPress={() => player.move(i, i + 1)} /><IconButton icon="x" label="Remove from queue" size={30} variant="ghost" onPress={() => player.removeAt(i)} /></View> : undefined} />;
        })}
        <Button label="Save as playlist" icon="save" size="sm" style={{marginTop: 8, alignSelf: 'flex-start'}} onPress={() => ui.prompt({title: 'Save as playlist', fields: [{key: 'name', label: 'Playlist name', initial: 'Saved queue'}], submitLabel: 'Save', onSubmit: v => { s.lists.createPlaylist(v.name || 'Saved queue', '', false, p.queue).then(() => ui.toast('Saved as a playlist.')); return null; }})} />
      </>
    );
  }
  return (
    <View style={styles.panel}>
      {body}
      {!isTV ? <Button label="Done" size="sm" variant="ghost" onPress={onClose} style={{alignSelf: 'flex-end', marginTop: 6}} /> : null}
    </View>
  );
}

function openMore(item: Movie | Episode, s: ReturnType<typeof useServices>) {
  ui.menu({title: 'More', actions: [
    {label: 'Play on…', icon: 'cast', detail: 'AirPlay, Google Cast or another Portico device', onPress: () => openPlayOn(item.title)},
    {label: 'Picture in Picture', icon: 'pip', onPress: () => { player.minimize(); navBack(); }},
    {label: 'Report a problem', icon: 'flag', onPress: () => go('Feedback', {itemId: item.id, playback: true})},
    {label: 'Rate this', icon: 'star', onPress: () => openRateSheet(s, item)},
    {label: 'Playback info', icon: 'info', detail: 'Direct play · 1080p H.264 · 11.2 Mb/s · home network', onPress: () => ui.toast('Direct play: the file plays as it is.')},
    ...(isTV ? [] : [{label: 'Keyboard shortcuts', icon: 'keyboard' as IconName, detail: 'Space play/pause · ←/→ 10 s · F full screen', onPress: () => {}}]),
    {label: 'Close player', icon: 'x', onPress: () => { player.stop(); navBack(); }},
  ]});
}

function PostPlay({item, nextEp, related, onDismiss, width, s}: {item: Movie | Episode; nextEp?: Episode; related: Item[]; onDismiss: () => void; width: number; s: ReturnType<typeof useServices>}) {
  const [count, setCount] = useState(10);
  useEffect(() => {
    if (!nextEp) return;
    const t = setInterval(() => setCount(c => c - 1), 1000);
    return () => clearInterval(t);
  }, [nextEp]);
  useEffect(() => {
    if (nextEp && count <= 0) go('Player', {id: nextEp.id});
  }, [count, nextEp]);
  if (nextEp) {
    return (
      <View style={[styles.post, {width: Math.min(width - 24, isTV ? 760 : 380)}]}>
        <Text variant="eyebrow" tone="accent">Up next · Playing in {Math.max(0, count)}s</Text>
        <View style={{flexDirection: 'row', marginTop: 10}}>
          <Artwork art={nextEp.art.still} tint={nextEp.tint} style={{width: u(160), height: u(90), borderRadius: 8}} />
          <View style={{flex: 1, marginLeft: 12}}>
            <Text variant="bodyStrong" numberOfLines={1}>{code(nextEp.seasonNumber, nextEp.number)} · {nextEp.title}</Text>
            <Text variant="caption" tone="secondary" numberOfLines={3}>{nextEp.synopsis}</Text>
          </View>
        </View>
        <View style={{flexDirection: 'row', gap: 10, marginTop: 12}}>
          <Button label="Play now" icon="play" variant="primary" hasTVPreferredFocus onPress={() => go('Player', {id: nextEp.id})} />
          <Button label="Cancel" onPress={onDismiss} />
        </View>
      </View>
    );
  }
  return (
    <View style={[styles.postFull]}>
      <Text variant="eyebrow" tone="accent">Finished</Text>
      <Text variant="display">{item.title}</Text>
      <View style={{flexDirection: 'row', gap: 10, marginVertical: 14}}>
        <Button label="Replay" icon="skipBack" hasTVPreferredFocus onPress={() => { player.seek(0); player.resume(); onDismiss(); }} />
        <Button label="Rate this" icon="star" onPress={() => openRateSheet(s, item)} />
        <Button label="Close" icon="x" onPress={() => { player.stop(); navBack(); }} />
      </View>
      <Text variant="title" style={{marginBottom: 10}}>More like {item.title}</Text>
      <View style={{flexDirection: 'row', gap: 12}}>
        {related.slice(0, isTV ? 6 : 3).map(r => {
          const c = toCard(r, s.user.state(r.id));
          return (
            <Focusable key={r.id} onPress={() => push('Movie', {id: r.id})} scale={1.08} accessibilityLabel={r.title}>
              {({focused}) => <Artwork art={c.art} title={r.title} tint={r.tint} style={[{width: u(92), height: u(138), borderRadius: 8}, focused && isTV && {borderWidth: 3, borderColor: colors.white}]} />}
            </Focusable>
          );
        })}
      </View>
    </View>
  );
}

function OwnerStopped({message}: {message: string}) {
  return (
    <View style={[StyleSheet.absoluteFill, styles.center, {backgroundColor: 'rgba(0,0,0,0.75)', padding: 30}]}>
      <Icon name="stop" size={36} color={colors.amber} />
      <Text variant="title" center style={{marginTop: 12}}>The server owner stopped this playback.</Text>
      {message ? <Text variant="body" tone="secondary" center style={{marginTop: 8}}>Message from the server owner: “{message}”</Text> : null}
      <Button label="Close" variant="primary" style={{marginTop: 20}} hasTVPreferredFocus onPress={() => { player.stop(); navBack(); }} />
    </View>
  );
}

const styles = StyleSheet.create({
  black: {flex: 1, backgroundColor: '#000'},
  center: {...StyleSheet.absoluteFillObject, alignItems: 'center', justifyContent: 'center'},
  shade: {...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(0,0,0,0.35)'},
  top: {flexDirection: 'row', alignItems: 'center'},
  centerControls: {...StyleSheet.absoluteFillObject, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: 36},
  bottom: {position: 'absolute', left: 0, right: 0, bottom: 0},
  times: {flexDirection: 'row', justifyContent: 'space-between', marginTop: 2},
  tabs: {flexDirection: 'row', flexWrap: 'wrap', gap: u(8), marginTop: u(12)},
  tab: {flexDirection: 'row', alignItems: 'center', height: u(32), paddingHorizontal: u(12), borderRadius: radius.pill, backgroundColor: 'rgba(237,245,250,0.12)', maxWidth: isTV ? 520 : 240},
  panel: {marginTop: u(12), backgroundColor: 'rgba(19,38,52,0.94)', borderRadius: radius.lg, padding: u(14), maxHeight: isTV ? 520 : 340, borderWidth: StyleSheet.hairlineWidth, borderColor: colors.line},
  skip: {position: 'absolute'},
  subs: {position: 'absolute', left: 40, right: 40, alignItems: 'center'},
  subText: {backgroundColor: 'rgba(0,0,0,0.55)', paddingHorizontal: 10, paddingVertical: 4, borderRadius: 4, overflow: 'hidden'},
  post: {position: 'absolute', right: 24, bottom: isTV ? 80 : 40, backgroundColor: 'rgba(19,38,52,0.96)', borderRadius: radius.lg, padding: u(16), borderWidth: 1, borderColor: colors.lineStrong},
  postFull: {...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(4,10,16,0.88)', justifyContent: 'center', paddingHorizontal: isTV ? 120 : 24},
});

export {Segmented};
