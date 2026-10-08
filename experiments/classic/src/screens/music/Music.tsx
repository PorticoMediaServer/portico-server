import React from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useQuery} from '../../data/DataProvider';
import type {Entry, TrackItem} from '../../data/types';
import {Screen, ErrorView} from '../../ui/Page';
import {Shelf} from '../../ui/Shelf';
import {Button} from '../../ui/Button';
import {Loading} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Icon} from '../../ui/Icon';
import {Focusable} from '../../ui/Focusable';
import {Artwork} from '../../ui/Artwork';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useCardHandlers} from '../../app/useCards';
import {useToast} from '../../app/toast';
import {formatTime, usePlayer, usePlayerState} from '../../player/PlayerProvider';
import {push} from '../../nav/nav';
import {DetailHero, Facts, ActionRow} from '../detail/DetailHero';

const trackEntry = (t: TrackItem): Entry => ({id: t.id, kind: 'track', title: t.title, subtitle: t.artist, image: t.image, shape: 'square', duration: t.duration, parentId: t.albumId});

export function TrackRow({t, index, list, showArt}: {t: TrackItem; index: number; list: TrackItem[]; showArt?: boolean}) {
  const m = useMetrics();
  const player = usePlayer();
  const s = usePlayerState();
  const actions = useActions();
  const playing = s.queue[s.index]?.id === t.id && s.mode === 'music';
  return (
    <Focusable onPress={() => player.play(list.map(trackEntry), index, {mode: 'music'})} onLongPress={() => actions.more(trackEntry(t))} accessibilityLabel={`${t.title}, ${formatTime(t.duration)}`}
      style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.md, paddingVertical: m.isTV ? 14 : 10, paddingHorizontal: m.isTV ? 18 : 10, borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedBright : 'transparent'})}>
      {({focused}) => (
        <>
          {showArt ? <Artwork source={t.image} title={t.album} width={m.isTV ? 56 : 40} height={m.isTV ? 56 : 40} radius={4} /> : (
            <View style={{width: m.isTV ? 40 : 26, alignItems: 'center'}}>
              {playing ? <Icon name="audio" size={m.iconSmall} color={colors.accentStrong} /> : <Text variant="captionStrong" tone="tertiary">{t.number}</Text>}
            </View>
          )}
          <View style={{flex: 1}}>
            <Text variant="body" numberOfLines={1} color={playing ? colors.accentStrong : colors.text}>{t.title}</Text>
            {showArt ? <Text variant="caption" tone="tertiary" numberOfLines={1}>{t.album}</Text> : null}
          </View>
          {t.hasLyrics ? <Icon name="lyrics" size={m.iconSmall} color={colors.textTertiary} /> : null}
          <Text variant="caption" tone="tertiary">{formatTime(t.duration)}</Text>
          {!m.isTV ? <Focusable onPress={() => actions.more(trackEntry(t))} accessibilityLabel={`More for ${t.title}`} style={{padding: 4}}><Icon name="more" size={18} color={focused ? colors.text : colors.textTertiary} /></Focusable> : null}
        </>
      )}
    </Focusable>
  );
}

export function ArtistScreen() {
  const {params} = useRoute() as {params: {id: string}};
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const cards = useCardHandlers();
  const q = useQuery(c => c.catalog.artist(params.id), [params.id], ['personal']);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  const d = q.data;
  const e = d.entry;
  return (
    <Screen bleedTop contentStyle={{gap: m.rowGap}}>
      <DetailHero backdrop={e.backdrop} poster={e.image} posterShape="square" round eyebrow={`Artist · ${d.genre}`} title={e.title} synopsis={d.bio} facts={<Facts items={[`${d.albums.length} album${d.albums.length === 1 ? '' : 's'}`, `${d.popular.length}+ songs`]} />}>
        <ActionRow>
          <Button label="Play" icon="play" variant="primary" size="lg" onPress={() => void actions.play(e)} hasTVPreferredFocus />
          <Button label="Shuffle" icon="shuffle" size="lg" onPress={() => void actions.play(e, {shuffle: true})} />
          <Button icon={d.personal.favorite ? 'heartFilled' : 'heart'} size="lg" selected={d.personal.favorite} accessibilityLabel={d.personal.favorite ? 'Remove from Favorites' : 'Add to Favorites'} onPress={async () => { await client.personal.setFavorite([e.id], !d.personal.favorite); toast({message: d.personal.favorite ? 'Removed from Favorites.' : 'Added to Favorites.'}); }} />
          <Button icon="more" size="lg" accessibilityLabel="More actions" onPress={() => actions.more(e)} />
        </ActionRow>
      </DetailHero>
      <View style={{paddingHorizontal: m.gutter, gap: 4}}>
        <Text variant="heading" style={{marginBottom: 6}}>Popular</Text>
        {d.popular.map((t, i) => <TrackRow key={t.id} t={t} index={i} list={d.popular} showArt />)}
      </View>
      <Shelf title="Albums" entries={d.albums} shape="square" onPress={cards.onPress} onLongPress={cards.onLongPress} />
      {d.similar.length ? <Shelf title="Similar artists" entries={d.similar} shape="circle" onPress={cards.onPress} onLongPress={cards.onLongPress} /> : null}
    </Screen>
  );
}

export function AlbumScreen() {
  const {params} = useRoute() as {params: {id: string}};
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const cards = useCardHandlers();
  const q = useQuery(c => c.catalog.album(params.id), [params.id], ['personal']);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  const d = q.data;
  const e = d.entry;
  const discs = [...new Set(d.tracks.map(t => t.disc))];
  return (
    <Screen bleedTop contentStyle={{gap: m.rowGap}}>
      <DetailHero backdrop={d.artist.backdrop} poster={e.image} posterShape="square" eyebrow={`${d.type} · ${d.genre}`} title={e.title}
        facts={
          <View style={{gap: 6}}>
            <Focusable onPress={() => push('Artist', {id: d.artist.id})} style={({focused}) => ({alignSelf: 'flex-start', borderRadius: 8, paddingHorizontal: m.isTV ? 8 : 0, backgroundColor: focused ? colors.raisedHigh : 'transparent'})}><Text variant="subheading" tone="accent">{d.artist.title}</Text></Focusable>
            <Facts items={[String(e.year), `${d.tracks.length} songs`, formatTime(d.totalDuration), d.label]} />
          </View>
        }>
        <ActionRow>
          <Button label="Play" icon="play" variant="primary" size="lg" onPress={() => void actions.play(e)} hasTVPreferredFocus />
          <Button label="Shuffle" icon="shuffle" size="lg" onPress={() => void actions.play(e, {shuffle: true})} />
          <Button icon={d.personal.favorite ? 'heartFilled' : 'heart'} size="lg" selected={d.personal.favorite} accessibilityLabel={d.personal.favorite ? 'Remove from Favorites' : 'Add to Favorites'} onPress={async () => { await client.personal.setFavorite([e.id], !d.personal.favorite); toast({message: d.personal.favorite ? 'Removed from Favorites.' : 'Added to Favorites.'}); }} />
          <Button icon="playlist" size="lg" accessibilityLabel="Add to playlist" onPress={() => actions.addTo('playlist', d.tracks.map(trackEntry))} />
          <Button icon="more" size="lg" accessibilityLabel="More actions" onPress={() => actions.more(e)} />
        </ActionRow>
      </DetailHero>
      <View style={{paddingHorizontal: m.gutter, gap: 2}}>
        {discs.map(disc => (
          <View key={disc}>
            {discs.length > 1 ? <Text variant="label" tone="tertiary" style={{marginVertical: 8}}>Disc {disc}</Text> : null}
            {d.tracks.filter(t => t.disc === disc).map(t => <TrackRow key={t.id} t={t} index={d.tracks.indexOf(t)} list={d.tracks} />)}
          </View>
        ))}
        <Text variant="caption" tone="tertiary" style={{marginTop: 8}}>{e.year} · {d.label}</Text>
      </View>
      {d.more.length ? <Shelf title={`More from ${d.artist.title}`} entries={d.more} shape="square" onPress={cards.onPress} onLongPress={cards.onLongPress} /> : null}
    </Screen>
  );
}
