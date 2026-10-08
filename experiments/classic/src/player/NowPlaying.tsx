import React, {useEffect, useState} from 'react';
import {Image, ScrollView, StyleSheet, TVFocusGuideView, View, useTVEventHandler} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {useQuery} from '../data/DataProvider';
import {Artwork} from '../ui/Artwork';
import {IconButton, Button} from '../ui/Button';
import {Text} from '../ui/Text';
import {Segmented} from '../ui/Controls';
import {Focusable} from '../ui/Focusable';
import {formatTime, usePlayer, usePlayerState} from './PlayerProvider';
import {Timeline} from './Timeline';
import {PlayerOptions, type OptionsTab} from './PlayerOptions';
import {QueuePanel} from './QueuePanel';
import {PlayOnSheet} from './PlayOn';
import {openEntry} from '../nav/nav';
import {useActions} from '../app/actions';

/** Now playing for music (lyrics, queue) and audiobooks (chapters, speed, sleep timer). */
export function NowPlaying() {
  const s = usePlayerState();
  const player = usePlayer();
  const actions = useActions();
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const e = s.queue[s.index];
  const book = s.mode === 'book';
  const [side, setSide] = useState<'lyrics' | 'queue' | 'chapters'>(book ? 'chapters' : 'lyrics');
  // Switching between a book and music keeps this screen mounted; start on the panel that fits.
  useEffect(() => setSide(book ? 'chapters' : 'lyrics'), [book]);
  const [panel, setPanel] = useState<'options' | 'queue' | 'playon'>();
  const [optionsTab, setOptionsTab] = useState<OptionsTab>('speed');
  const track = useQuery(async c => (e && e.kind === 'track' ? c.catalog.tracks([e.id])[0] : undefined), [e?.id]);
  const bookDetail = useQuery(c => (e && book ? c.catalog.audiobook(e.id) : Promise.resolve(undefined)), [e?.id, book]);
  useTVEventHandler(evt => { if (evt.eventType === 'playPause') player.toggle(); });
  if (!e) return null;
  const chapters = bookDetail.data?.chapters ?? [];
  const chapterIndex = Math.max(0, chapters.findIndex((c, i) => s.position >= c.start && s.position < (chapters[i + 1]?.start ?? Infinity)));
  const chapter = chapters[chapterIndex];
  const wide = !m.isPhone;
  const art = m.isTV ? (book ? 380 : 520) : wide ? Math.min(460, m.height * 0.5) : Math.min(m.width - 64, m.height * 0.42);
  const artH = book ? art * 1.5 : art;
  const artW = book ? art : art;
  const lyrics = track.data?.lyrics;
  const lyricIndex = lyrics ? lyrics.reduce((idx, l, i) => (s.position >= l.at ? i : idx), -1) : -1;

  const transport = (
    <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: m.isTV ? 28 : 18}}>
      {book ? <IconButton icon="skipBack" label="Back 15 seconds" onPress={() => player.skip(-15)} /> : <IconButton icon="shuffle" label={s.shuffle ? 'Shuffle on' : 'Shuffle off'} selected={s.shuffle} onPress={() => player.set({shuffle: !s.shuffle})} />}
      <IconButton icon="previous" label={book ? 'Previous chapter' : 'Previous'} size={m.isTV ? 72 : 52} onPress={() => (book && chapter ? player.seek(chapters[Math.max(0, chapterIndex - (s.position - chapter.start < 3 ? 1 : 0))]!.start) : player.previous())} />
      <IconButton icon={s.playing ? 'pause' : 'play'} label={s.playing ? 'Pause' : 'Play'} size={m.isTV ? 100 : 76} filled onPress={player.toggle} hasTVPreferredFocus />
      <IconButton icon="next" label={book ? 'Next chapter' : 'Next'} size={m.isTV ? 72 : 52} onPress={() => (book && chapters[chapterIndex + 1] ? player.seek(chapters[chapterIndex + 1]!.start) : player.next())} />
      {book ? <IconButton icon="skipForward" label="Forward 30 seconds" onPress={() => player.skip(30)} /> : <IconButton icon={s.repeat === 'one' ? 'repeatOne' : 'repeat'} label={s.repeat === 'off' ? 'Repeat off' : s.repeat === 'one' ? 'Repeat this title' : 'Repeat all'} selected={s.repeat !== 'off'} onPress={() => player.set({repeat: s.repeat === 'off' ? 'all' : s.repeat === 'all' ? 'one' : 'off'})} />}
    </View>
  );
  const info = (
    <View style={{gap: 4, alignItems: wide ? 'flex-start' : 'center'}}>
      <Text variant={m.isTV ? 'title' : 'heading'} numberOfLines={2} center={!wide}>{e.title}</Text>
      <Focusable accessibilityLabel={book ? 'Go to audiobook' : 'Go to artist'} style={({focused}) => ({borderRadius: 6, paddingHorizontal: focused ? 6 : 0, marginHorizontal: focused ? -6 : 0, backgroundColor: focused ? colors.raisedBright : 'transparent'})} onPress={() => { if (track.data) { player.minimize(); openEntry({id: track.data.artistId, kind: 'artist', title: track.data.artist}); } else if (book) { player.minimize(); openEntry(e); } }}>
        <Text variant="body" tone="accent" center={!wide}>{book ? `${bookDetail.data?.author ?? e.subtitle} · read by ${bookDetail.data?.narrator ?? ''}` : `${track.data?.artist ?? e.subtitle} — ${track.data?.album ?? ''}`}</Text>
      </Focusable>
      {book && chapter ? <Text variant="caption" tone="tertiary">{chapter.title} · {chapterIndex + 1} of {chapters.length}</Text> : null}
    </View>
  );
  const utility = (
    <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm, justifyContent: wide ? 'flex-start' : 'center'}}>
      {book ? <Button size="sm" icon="speed" label={s.speed === 1 ? 'Speed' : `${s.speed}×`} variant="secondary" onPress={() => { setOptionsTab('speed'); setPanel('options'); }} /> : null}
      <Button size="sm" icon="sleep" label={s.sleepLabel ? `Sleep · ${s.sleepLabel}` : 'Sleep timer'} variant="secondary" onPress={() => { setOptionsTab('sleep'); setPanel('options'); }} />
      {!wide && !book ? <Button size="sm" icon="lyrics" label="Lyrics" variant={side === 'lyrics' ? 'primary' : 'secondary'} onPress={() => setSide('lyrics')} /> : null}
      {!wide ? <Button size="sm" icon={book ? 'chapters' : 'queue'} label={book ? 'Chapters' : 'Up next'} variant="secondary" onPress={() => (book ? setSide('chapters') : setPanel('queue'))} /> : null}
      <Button size="sm" icon="cast" label={s.cast ? s.cast.name : 'Play on'} variant="secondary" onPress={() => setPanel('playon')} />
      <Button size="sm" icon="more" label="More" variant="ghost" onPress={() => { player.minimize(); setTimeout(() => actions.more(e), 300); }} />
    </View>
  );
  const sidePanel = (
    <View style={{flex: 1, gap: m.space.md}}>
      {wide ? <Segmented value={side} onChange={setSide} options={book ? [{value: 'chapters', label: 'Chapters', icon: 'chapters'}, {value: 'queue', label: 'Up next', icon: 'queue'}] : [{value: 'lyrics', label: 'Lyrics', icon: 'lyrics'}, {value: 'queue', label: 'Up next', icon: 'queue'}]} /> : null}
      <ScrollView style={{flex: 1}} contentContainerStyle={{gap: side === 'lyrics' ? m.space.sm : 2, paddingBottom: 40}}>
        {side === 'lyrics' ? (lyrics ? lyrics.map((l, i) => (
          <Focusable key={i} onPress={() => player.seek(l.at)} focusScale={1}>
            {({focused}) => <Text variant={m.isTV ? 'title' : 'heading'} color={focused ? colors.accentStrong : i === lyricIndex ? colors.text : i < lyricIndex ? colors.textTertiary : colors.textSecondary} style={{opacity: i === lyricIndex ? 1 : 0.7}}>{l.line}</Text>}
          </Focusable>
        )) : <Text variant="body" tone="tertiary">No lyrics for this song.</Text>) : null}
        {side === 'queue' ? s.queue.slice(s.index, s.index + 40).map((q, i) => (
          <Focusable key={`${q.id}-${i}`} onPress={() => player.jump(s.index + i)} style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: 12, padding: 8, borderRadius: 10, backgroundColor: focused ? colors.raisedBright : i === 0 ? 'rgba(64,159,212,0.14)' : 'transparent'})}>
            <Artwork source={q.image} title={q.title} width={m.isTV ? 64 : 44} height={m.isTV ? 64 : 44} radius={6} />
            <View style={{flex: 1}}><Text variant="bodyStrong" numberOfLines={1} color={i === 0 ? colors.accentStrong : colors.text}>{q.title}</Text><Text variant="caption" tone="tertiary" numberOfLines={1}>{i === 0 ? 'Now playing' : q.subtitle}</Text></View>
            <Text variant="caption" tone="tertiary">{formatTime(q.duration ?? 0)}</Text>
          </Focusable>
        )) : null}
        {side === 'chapters' ? chapters.map((c, i) => (
          <Focusable key={c.number} onPress={() => player.seek(c.start)} hasTVPreferredFocus={false} style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: m.isTV ? 14 : 10, paddingHorizontal: 10, borderRadius: 10, backgroundColor: focused ? colors.raisedBright : i === chapterIndex ? 'rgba(64,159,212,0.14)' : 'transparent'})}>
            <Text variant="captionStrong" tone="tertiary" style={{width: m.isTV ? 48 : 32}}>{c.number}</Text>
            <Text variant="body" style={{flex: 1}} color={i === chapterIndex ? colors.accentStrong : colors.text} numberOfLines={1}>{c.title}</Text>
            <Text variant="caption" tone="tertiary">{formatTime(c.duration)}</Text>
          </Focusable>
        )) : null}
      </ScrollView>
    </View>
  );
  return (
    <View style={{flex: 1, backgroundColor: colors.ink}}>
      {e.image ? <Image source={e.image} blurRadius={m.isTV ? 60 : 40} style={[StyleSheet.absoluteFill, {opacity: 0.55}]} resizeMode="cover" /> : null}
      <View style={[StyleSheet.absoluteFill, {backgroundColor: 'rgba(7,16,24,0.62)'}]} />
      <TVFocusGuideView style={{flex: 1, paddingTop: m.isTV ? 64 : insets.top + 8, paddingBottom: m.isTV ? 56 : insets.bottom + 12, paddingHorizontal: m.isTV ? 96 : m.isPhone ? 20 : 40}}>
        <View style={{flexDirection: 'row', alignItems: 'center', gap: 12, marginBottom: m.space.md}}>
          {m.isTV ? null : <IconButton icon="chevronDown" label="Minimize player" variant="secondary" onPress={player.minimize} />}
          <View style={{flex: 1, alignItems: 'center'}}><Text variant="label" tone="tertiary">{book ? 'Audiobook' : 'Now playing'}</Text></View>
          {m.isTV ? null : <IconButton icon="close" label="Close player" variant="secondary" onPress={player.close} />}
        </View>
        {wide ? (
          <View style={{flex: 1, flexDirection: 'row', gap: m.isTV ? 80 : 40}}>
            <View style={{width: Math.max(artW, m.isTV ? 560 : 360), gap: m.space.lg, justifyContent: 'center'}}>
              <Artwork source={e.image} title={e.title} width={artW} height={artH > m.height * 0.5 ? m.height * 0.5 : artH} radius={m.radius.panel} style={{alignSelf: 'center', shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 30}} resizeMode="cover" />
              {info}
              <Timeline position={s.position} duration={s.duration} onSeek={player.seek} chapters={chapters.map(c => ({title: c.title, start: c.start}))} />
              {transport}
              {utility}
            </View>
            {sidePanel}
          </View>
        ) : (
          <View style={{flex: 1, gap: m.space.lg}}>
            {side === 'lyrics' && lyrics && !book ? <View style={{flex: 1}}>{sidePanel}</View> : side === 'chapters' && book ? (
              <View style={{flex: 1, flexDirection: 'row', gap: 16}}>
                <Artwork source={e.image} title={e.title} width={110} height={165} radius={10} />
                {sidePanel}
              </View>
            ) : <Artwork source={e.image} title={e.title} width={art} height={art} radius={m.radius.panel} style={{alignSelf: 'center'}} />}
            {info}
            <Timeline position={s.position} duration={s.duration} onSeek={player.seek} />
            {transport}
            {utility}
          </View>
        )}
      </TVFocusGuideView>
      <PlayerOptions visible={panel === 'options'} onClose={() => setPanel(undefined)} entry={e} initialTab={optionsTab} mode={book ? 'book' : 'music'} />
      <QueuePanel visible={panel === 'queue'} onClose={() => setPanel(undefined)} />
      <PlayOnSheet visible={panel === 'playon'} onClose={() => setPanel(undefined)} />
    </View>
  );
}
