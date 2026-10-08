import React, {useEffect, useRef, useState} from 'react';
import {Image, ScrollView, StyleSheet, TVFocusGuideView, View, useWindowDimensions} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {resolveArt} from '../../data/art';
import {useQuery, useServices} from '../../data/hooks';
import type {Audiobook, TrackItem} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {ListRow, Segmented} from '../../design/Controls';
import {glow} from '../../design/Glow';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {back as backNav} from '../../nav/back';
import {back, push} from '../../nav/routes';
import {player, usePlayer} from '../../state/player';
import {openAddToPlaylist} from '../shared/actions';
import {fmtClock} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {openPlayOn} from './PlayOn';
import {Timeline} from './Timeline';
import {plural} from '../../design/format';

/** Now playing for music and audiobooks: artwork, transport, lyrics, queue, chapters, speed and sleep. */
export function NowPlayingScreen() {
  const p = usePlayer();
  const s = useServices();
  const insets = useSafeAreaInsets();
  const {width, height} = useWindowDimensions();
  const q = useQuery(p.itemId ? 'item:' + p.itemId : null, () => s.catalog.item(p.itemId!));
  const it = q.data as TrackItem | Audiobook | undefined;
  const book = it?.kind === 'audiobook' ? it : undefined;
  const [tab, setTab] = useState<'lyrics' | 'queue' | 'chapters'>(book ? 'chapters' : 'lyrics');
  useEffect(() => {
    if (it) glow.setTint(it.tint);
    // Open on the most useful panel: chapters for books, lyrics when the song has them, else up next.
    if (book) setTab('chapters');
    else if (it?.kind === 'track') setTab(it.hasLyrics ? 'lyrics' : 'queue');
  }, [it, book]);
  useEffect(() => backNav.register({priority: 40, active: () => true, handle: () => back()}), []);
  if (!p.active || !it) {
    return (
      <View style={[styles.fill, {alignItems: 'center', justifyContent: 'center'}]}>
        <Text variant="title">Nothing is playing</Text>
        <Button label="Close" onPress={back} style={{marginTop: 16}} hasTVPreferredFocus />
      </View>
    );
  }
  const art = it.kind === 'track' ? it.art.square : it.art.poster;
  const src = resolveArt(art);
  const wide = isTV || width > 700;
  const artSize = isTV ? 440 : wide ? Math.min(420, height * 0.42) : width - 64;
  const chapterIdx = book ? Math.max(0, book.chapters.findIndex((c, i) => p.position >= c.start && (i === book.chapters.length - 1 || p.position < book.chapters[i + 1].start))) : -1;
  const chapter = book ? book.chapters[chapterIdx] : undefined;
  const subtitle = it.kind === 'track' ? `${it.artistName} — ${it.albumTitle}` : `${it.author} · read by ${it.narrator}`;
  const sleep = () => ui.menu({title: 'Sleep timer', actions: [
    {label: 'Off', checked: p.sleep.mode === 'off', onPress: () => player.setSleep('off')},
    ...[15, 30, 45, 60, 90].map(m => ({label: m === 60 ? '1 hour' : `${plural(m, 'minute', 'minutes')}`, checked: p.sleep.label === `${m} min`, onPress: () => player.setSleep('minutes', m)})),
    {label: book ? 'End of this chapter' : 'End of this item', checked: p.sleep.mode === 'endOfItem' || p.sleep.mode === 'endOfChapter', onPress: () => player.setSleep(book ? 'endOfChapter' : 'endOfItem')},
  ]});
  const speed = () => ui.menu({title: 'Speed', actions: [0.75, 1, 1.25, 1.5, 1.75, 2].map(r => ({label: r === 1 ? 'Normal' : `${r}×`, checked: p.rate === r, onPress: () => player.set({rate: r})}))});
  const controls = (
    <View style={styles.transport}>
      {!book ? <IconButton icon="shuffle" label={p.shuffle ? 'Shuffle on' : 'Shuffle off'} active={p.shuffle} variant="ghost" onPress={() => player.set({shuffle: !p.shuffle})} /> : <IconButton icon="skipBack" label="Back 15 seconds" variant="ghost" onPress={() => player.skip(-15)} />}
      <IconButton icon="prev" label="Previous" size={50} variant="ghost" fill onPress={() => (book && chapter ? player.seek(book.chapters[Math.max(0, chapterIdx - 1)].start) : player.previous())} />
      <IconButton icon={p.playing ? 'pause' : 'play'} label={p.playing ? 'Pause' : 'Play'} size={74} variant="primary" fill hasTVPreferredFocus onPress={player.toggle} />
      <IconButton icon="next" label="Next" size={50} variant="ghost" fill onPress={() => (book ? player.seek(book.chapters[Math.min(book.chapters.length - 1, chapterIdx + 1)].start) : player.next())} />
      {!book ? <IconButton icon={p.repeat === 'one' ? 'repeatOne' : 'repeat'} label={`Repeat ${p.repeat}`} active={p.repeat !== 'off'} variant="ghost" onPress={() => player.set({repeat: p.repeat === 'off' ? 'all' : p.repeat === 'all' ? 'one' : 'off'})} /> : <IconButton icon="skipForward" label="Forward 30 seconds" variant="ghost" onPress={() => player.skip(30)} />}
    </View>
  );
  return (
    <View style={styles.fill}>
      {src ? <Image source={src} blurRadius={isTV ? 60 : 40} style={[StyleSheet.absoluteFill, {width: '100%', height: '100%', opacity: 0.55}]} resizeMode="cover" /> : null}
      <View style={[StyleSheet.absoluteFill, {backgroundColor: 'rgba(4,10,16,0.55)'}]} />
      <TVFocusGuideView autoFocus style={[styles.fill, {paddingTop: insets.top + (isTV ? 70 : 8), paddingHorizontal: isTV ? 110 : 22, paddingBottom: insets.bottom + 12, flexDirection: wide ? 'row' : 'column', gap: isTV ? 90 : 20}]}>
        <View style={{alignItems: wide ? 'flex-start' : 'stretch', width: wide ? artSize : undefined}}>
          {!isTV ? (
            <View style={{flexDirection: 'row', justifyContent: 'space-between', marginBottom: 8}}>
              <IconButton icon="chevronDown" label="Close now playing" variant="ghost" onPress={back} />
              <Text variant="eyebrow" tone="secondary" style={{alignSelf: 'center'}}>{book ? 'Listening' : 'Now playing'}</Text>
              <IconButton icon="airplay" label="Play on" variant="ghost" onPress={() => openPlayOn(it.title)} />
            </View>
          ) : null}
          <Artwork art={art} title={it.title} tint={it.tint} style={[{width: book ? artSize * (wide ? 0.66 : 0.6) : wide ? artSize : Math.min(artSize, height * 0.42), height: book ? artSize * (wide ? 0.66 : 0.6) * 1.5 : wide ? artSize : Math.min(artSize, height * 0.42), borderRadius: radius.lg, alignSelf: 'center'}, styles.shadow]} />
          <View style={{marginTop: u(16), width: '100%'}}>
            <Text variant="display" numberOfLines={2}>{book ? chapter?.title ?? it.title : it.title}</Text>
            <Text variant="body" tone="secondary" numberOfLines={1}>{book ? it.title : subtitle}</Text>
            {book ? <Text variant="caption" tone="tertiary" numberOfLines={1}>{subtitle}</Text> : null}
            <Timeline position={p.position} duration={p.duration} onSeek={t => player.seek(t)} chapters={book ? book.chapters.map(c => ({start: c.start, title: c.title})) : []} />
            <View style={{flexDirection: 'row', justifyContent: 'space-between'}}>
              <Text variant="caption" tone="secondary">{fmtClock(p.position)}</Text>
              <Text variant="caption" tone="secondary">-{fmtClock(p.duration - p.position)}</Text>
            </View>
            {controls}
            <View style={styles.extras}>
              <Button icon="speed" label={p.rate === 1 ? 'Speed' : `${p.rate}×`} size="sm" onPress={speed} />
              <Button icon="moon" label={p.sleep.mode === 'off' ? 'Sleep' : p.sleep.label} size="sm" active={p.sleep.mode !== 'off'} onPress={sleep} />
              {!book ? <Button icon="plus" label="Playlist" size="sm" onPress={() => openAddToPlaylist(s, [it.id])} /> : null}
              {isTV ? <Button icon="airplay" label="Play on" size="sm" onPress={() => openPlayOn(it.title)} /> : null}
              {it.kind === 'track' ? <Button icon="album" label="Album" size="sm" onPress={() => push('Album', {id: it.albumId})} /> : <Button icon="book" label="Details" size="sm" onPress={() => push('Audiobook', {id: it.id})} />}
            </View>
          </View>
        </View>
        <View style={{flex: 1, minHeight: wide ? undefined : 320}}>
          <Segmented value={tab} onChange={setTab} options={book ? [{value: 'chapters', label: 'Chapters'}, {value: 'queue', label: 'Queue'}] : [{value: 'lyrics', label: 'Lyrics', icon: 'lyrics'}, {value: 'queue', label: 'Up next', icon: 'queue'}]} />
          <View style={{flex: 1, marginTop: u(14)}}>
            {tab === 'lyrics' && it.kind === 'track' ? <Lyrics trackId={it.id} position={p.position} /> : null}
            {tab === 'queue' ? <Queue /> : null}
            {tab === 'chapters' && book ? (
              <ScrollView>
                {book.chapters.map((c, i) => <ListRow key={i} label={c.title} help={`${fmtClock(c.start)} · ${Math.round(c.duration / 60)} min`} onPress={() => player.seek(c.start)} chevron={false} compact right={i === chapterIdx ? <Icon name="audio" size={16} color={colors.accentStrong} /> : undefined} />)}
              </ScrollView>
            ) : null}
          </View>
        </View>
      </TVFocusGuideView>
    </View>
  );
}

function Lyrics({trackId, position}: {trackId: string; position: number}) {
  const s = useServices();
  const q = useQuery('lyrics:' + trackId, () => s.catalog.lyrics(trackId));
  const ref = useRef<ScrollView>(null);
  const lines = q.data;
  const idx = lines ? Math.max(0, lines.findIndex((l, i) => position >= l.t && (i === lines.length - 1 || position < lines[i + 1].t))) : -1;
  useEffect(() => {
    if (idx >= 0) ref.current?.scrollTo({y: Math.max(0, idx * u(40) - u(80)), animated: true});
  }, [idx]);
  if (q.loading) return null;
  if (!lines) return <Text variant="body" tone="tertiary">This song doesn’t have lyrics yet.</Text>;
  return (
    <ScrollView ref={ref} showsVerticalScrollIndicator={false}>
      {lines.map((l, i) => (
        <Text key={i} variant={isTV ? 'title' : 'heading'} style={{minHeight: u(40), opacity: i === idx ? 1 : i < idx ? 0.35 : 0.55}} color={i === idx ? colors.white : colors.textSecondary}>{l.line}</Text>
      ))}
    </ScrollView>
  );
}

function Queue() {
  const p = usePlayer();
  const items = useItems(p.queue);
  const s = useServices();
  return (
    <ScrollView>
      {p.queue.map((id, i) => {
        const it = items.lookup(id);
        const now = i === p.index;
        return (
          <ListRow key={id + i} label={it?.title ?? 'Still loading'} help={now ? 'Now playing' : it?.kind === 'track' ? (it as TrackItem).artistName : undefined} onPress={() => player.jumpTo(i)} chevron={false} compact
            right={!now ? <View style={{flexDirection: 'row'}}><IconButton icon="arrowUp" label="Move up" size={30} variant="ghost" onPress={() => player.move(i, i - 1)} /><IconButton icon="arrowDown" label="Move down" size={30} variant="ghost" onPress={() => player.move(i, i + 1)} /><IconButton icon="x" label="Remove from queue" size={30} variant="ghost" onPress={() => player.removeAt(i)} /></View> : <Icon name="audio" size={16} color={colors.accentStrong} />} />
        );
      })}
      <Button label="Save as playlist" icon="save" size="sm" style={{marginTop: 10, alignSelf: 'flex-start'}} onPress={() => ui.prompt({title: 'Save as playlist', fields: [{key: 'name', label: 'Playlist name', initial: 'Saved queue'}], submitLabel: 'Save', onSubmit: v => { s.lists.createPlaylist(v.name || 'Saved queue', '', false, p.queue).then(() => ui.toast('Saved as a playlist.')); return null; }})} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  fill: {flex: 1, backgroundColor: colors.inkDeep},
  transport: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: u(10)},
  extras: {flexDirection: 'row', flexWrap: 'wrap', gap: u(8), marginTop: u(14)},
  shadow: {shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 30, shadowOffset: {width: 0, height: 16}},
});
