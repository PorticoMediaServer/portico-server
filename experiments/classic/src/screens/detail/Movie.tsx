import React, {useState} from 'react';
import {ScrollView, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useQuery} from '../../data/DataProvider';
import type {Credit, MovieDetail} from '../../data/types';
import {Screen, ErrorView} from '../../ui/Page';
import {Shelf} from '../../ui/Shelf';
import {Button} from '../../ui/Button';
import {Badge, Chip, Loading, Pill, Surface} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Icon} from '../../ui/Icon';
import {Focusable} from '../../ui/Focusable';
import {Artwork} from '../../ui/Artwork';
import {ChoiceSheet} from '../../ui/Sheet';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useCardHandlers} from '../../app/useCards';
import {useToast} from '../../app/toast';
import {formatDuration} from '../../player/PlayerProvider';
import {push} from '../../nav/nav';
import {DetailHero, Facts, ActionRow} from './DetailHero';
import {useGlow} from '../../app/glow';

/** Movie (and home video) detail: hero, facts, play and resume, versions, audio and subtitles, cast, extras and related. */
export function MovieScreen() {
  const route = useRoute() as {name: string; params: {id: string}};
  const glow = useGlow();
  const q = useQuery(c => (route.name === 'HomeVideo' ? c.catalog.homeVideo(route.params.id) : c.catalog.movie(route.params.id)), [route.params.id], ['personal']);
  React.useEffect(() => { glow(route.params.id); return () => glow(undefined); }, [route.params.id, glow]);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading label="Loading" /></Screen>;
  return <MovieBody d={q.data} home={route.name === 'HomeVideo'} />;
}

function MovieBody({d, home}: {d: MovieDetail; home: boolean}) {
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const cards = useCardHandlers();
  const e = d.entry;
  const p = d.personal;
  const [version, setVersion] = useState(d.versions[0]!.id);
  const [audio, setAudio] = useState(d.audio.find(a => a.default)?.id ?? d.audio[0]?.id);
  const [sub, setSub] = useState<string>('off');
  const [pick, setPick] = useState<'audio' | 'sub' | 'version'>();
  const v = d.versions.find(x => x.id === version) ?? d.versions[0]!;
  const remaining = e.progress ? (e.duration ?? 0) * (1 - e.progress) : 0;
  const toggle = async (what: 'watchlist' | 'watched' | 'favorite') => {
    if (what === 'watchlist') await client.personal.setWatchlist([e.id], !p.watchlisted);
    if (what === 'favorite') await client.personal.setFavorite([e.id], !p.favorite);
    if (what === 'watched') await client.personal.setWatched([e.id], !e.watched);
    toast({message: what === 'watchlist' ? (p.watchlisted ? 'Removed from Watchlist.' : 'Added to Watchlist.') : what === 'favorite' ? (p.favorite ? 'Removed from Favorites.' : 'Added to Favorites.') : e.watched ? 'Marked as unwatched.' : 'Marked as watched.', icon: 'check'});
  };
  const people: Credit[] = [...d.directors, ...d.cast];
  const trailer = d.extras.find(x => x.id.endsWith('-trailer'));
  return (
    <Screen bleedTop contentStyle={{gap: m.rowGap}}>
      <DetailHero backdrop={e.backdrop} poster={home ? undefined : e.image} eyebrow={home ? 'Home video' : d.collections[0] ? `Part of ${d.collections[0].name}` : d.studio} title={e.title} synopsis={e.overview}
        facts={
          <>
            <Facts items={[String(e.year ?? ''), formatDuration(e.duration), ...(e.genres ?? []).slice(0, 3)]}>{e.rating ? <Pill label={e.rating} /> : null}{v.hdr ? <Pill label={v.hdr} /> : null}<Pill label={v.resolution.startsWith('4K') ? '4K' : 'HD'} /></Facts>
            {!home ? (
              <View style={{flexDirection: 'row', gap: m.space.md, alignItems: 'center', flexWrap: 'wrap'}}>
                {d.audienceRating ? <View style={{flexDirection: 'row', gap: 6, alignItems: 'center'}}><Icon name="starFilled" size={m.iconSmall} color={colors.amber} /><Text variant="captionStrong">{d.audienceRating.toFixed(1)}</Text><Text variant="caption" tone="tertiary">Audience</Text></View> : null}
                {d.criticRating ? <View style={{flexDirection: 'row', gap: 6, alignItems: 'center'}}><Icon name="sparkle" size={m.iconSmall} color={colors.healthy} /><Text variant="captionStrong">{d.criticRating}%</Text><Text variant="caption" tone="tertiary">Critics</Text></View> : null}
                {p.rating ? <View style={{flexDirection: 'row', gap: 6, alignItems: 'center'}}><Icon name="starFilled" size={m.iconSmall} color={colors.accentStrong} /><Text variant="captionStrong">You rated {p.rating}</Text></View> : null}
                {d.tagline ? <Text variant="caption" tone="tertiary" style={{fontStyle: 'italic'}}>“{d.tagline}”</Text> : null}
              </View>
            ) : null}
          </>
        }>
        <ActionRow>
          <Button label={e.progress ? `Resume · ${formatDuration(remaining)} left` : 'Play'} icon="play" variant="primary" size="lg" onPress={() => void actions.play(e)} hasTVPreferredFocus />
          {e.progress ? <Button label={m.isPhone ? undefined : 'Play from start'} accessibilityLabel="Play from start" icon="refresh" size="lg" onPress={() => void actions.play(e, {fromStart: true})} /> : null}
          {trailer ? <Button label={m.isPhone ? undefined : 'Trailer'} accessibilityLabel="Trailer" icon="movie" size="lg" onPress={() => void actions.play(trailer)} /> : null}
          {!home ? <Button icon={p.watchlisted ? 'savedFilled' : 'saved'} size="lg" selected={p.watchlisted} accessibilityLabel={p.watchlisted ? 'Remove from Watchlist' : 'Add to Watchlist'} onPress={() => void toggle('watchlist')} /> : null}
          <Button icon="watched" size="lg" selected={e.watched} accessibilityLabel={e.watched ? 'Mark as unwatched' : 'Mark as watched'} onPress={() => void toggle('watched')} />
          <Button icon={p.favorite ? 'heartFilled' : 'heart'} size="lg" selected={p.favorite} accessibilityLabel={p.favorite ? 'Remove from Favorites' : 'Add to Favorites'} onPress={() => void toggle('favorite')} />
          <Button icon="more" size="lg" accessibilityLabel="More actions" onPress={() => actions.more(e)} />
        </ActionRow>
        {e.progress && e.progress > 0.01 ? <View style={{flexDirection: 'row', alignItems: 'center', gap: 10, maxWidth: 420}}><View style={{flex: 1, height: 4, borderRadius: 2, backgroundColor: colors.track}}><View style={{width: `${e.progress * 100}%`, height: 4, borderRadius: 2, backgroundColor: colors.accent}} /></View><Text variant="caption" tone="tertiary">{Math.round(e.progress * 100)}%</Text></View> : null}
        {d.directors.length ? <Text variant="caption" tone="secondary">Directed by <Text variant="captionStrong">{d.directors.map(x => x.name).join(', ')}</Text>{d.writers.length ? <>  ·  Written by <Text variant="captionStrong">{d.writers.map(x => x.name).join(', ')}</Text></> : null}</Text> : null}
      </DetailHero>

      <View style={{paddingHorizontal: m.gutter, flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm}}>
        {d.versions.length > 1 ? <Chip icon="quality" label={`Version: ${v.label}`} onPress={() => setPick('version')} /> : null}
        <Chip icon="audio" label={`Audio: ${d.audio.find(a => a.id === audio)?.label ?? 'Default'}`} onPress={() => setPick('audio')} />
        <Chip icon="subtitles" label={`Subtitles: ${sub === 'off' ? 'Off' : d.subtitles.find(s => s.id === sub)?.label}`} onPress={() => setPick('sub')} />
      </View>

      {people.length ? (
        <View style={{gap: m.isTV ? 18 : 10}}>
          <Text variant="heading" style={{paddingHorizontal: m.gutter}}>Cast & crew</Text>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{paddingHorizontal: m.gutter, gap: m.gap, paddingVertical: m.isTV ? 14 : 2}} style={{marginVertical: m.isTV ? -14 : 0}}>
            {people.map((c, i) => <PersonChip key={`${c.personId}-${i}`} credit={c} />)}
          </ScrollView>
        </View>
      ) : null}
      {d.extras.length ? <Shelf title="Extras" entries={d.extras} shape="wide" onPress={x => void actions.play(x)} /> : null}
      {d.collections.length ? (
        <View style={{paddingHorizontal: m.gutter, flexDirection: 'row', gap: m.space.sm, flexWrap: 'wrap'}}>
          {d.collections.map(c => <Button key={c.id} icon="collection" label={`Part of ${c.name}`} variant="outline" onPress={() => push('Entity', {libraryId: e.libraryId ?? 'lib-movies', kind: 'collection', id: c.id, title: c.name})} />)}
        </View>
      ) : null}
      {d.related.length ? <Shelf title={home ? 'More home videos' : 'More like this'} entries={d.related} onPress={cards.onPress} onLongPress={cards.onLongPress} onFocus={cards.onFocus} shape={home ? 'wide' : 'poster'} /> : null}
      <View style={{paddingHorizontal: m.gutter}}>
        <Surface>
          <Text variant="subheading" style={{marginBottom: m.space.sm}}>Media information</Text>
          {d.versions.map((ver, i) => (
            <View key={ver.id} style={{gap: 4, marginTop: i ? m.space.md : 0}}>
              {d.versions.length > 1 ? <Text variant="captionStrong" tone="accent">Version {i + 1} · {ver.label}</Text> : null}
              {[['Resolution', ver.resolution], ['Video', ver.video], ['Audio', ver.audio], ['Container', ver.container], ['Size', `${ver.size} · ${ver.bitrate}`], ['Duration', formatDuration(e.duration)]].map(([k, val]) => (
                <View key={k} style={{flexDirection: 'row', gap: m.space.md}}><Text variant="caption" tone="tertiary" style={{width: m.isTV ? 180 : 96}}>{k}</Text><Text variant="caption" style={{flex: 1}}>{val}</Text></View>
              ))}
            </View>
          ))}
          {d.license ? <View style={{marginTop: m.space.md}}><Badge label={`${d.license} · Blender Foundation · blender.org`} tone="accent" /></View> : null}
        </Surface>
      </View>
      <ChoiceSheet visible={pick === 'version'} onClose={() => setPick(undefined)} title="Versions" options={d.versions.map(x => ({value: x.id, label: x.label, subtitle: `${x.video} · ${x.size}`}))} value={version} onChange={setVersion} />
      <ChoiceSheet visible={pick === 'audio'} onClose={() => setPick(undefined)} title="Audio" options={d.audio.map(x => ({value: x.id, label: x.label, subtitle: [x.codec, x.channels].filter(Boolean).join(' · ')}))} value={audio} onChange={setAudio} />
      <ChoiceSheet visible={pick === 'sub'} onClose={() => setPick(undefined)} title="Subtitles" options={[{value: 'off', label: 'Off'}, ...d.subtitles.map(x => ({value: x.id, label: x.label, subtitle: x.forced ? 'Forced' : x.origin === 'provider' ? 'Downloaded' : x.origin === 'sidecar' ? 'Beside the file' : 'In the file'}))]} value={sub} onChange={setSub} />
    </Screen>
  );
}

export function PersonChip({credit}: {credit: Credit}) {
  const m = useMetrics();
  const size = m.circle;
  return (
    <Focusable onPress={() => push('Person', {id: credit.personId})} focusScale={m.isTV ? 1.06 : 1} style={{width: size + 16, alignItems: 'center'}} accessibilityLabel={`${credit.name}${credit.character ? ` as ${credit.character}` : ''}`}>
      {({focused}) => (
        <View style={{alignItems: 'center', gap: 6}}>
          <View style={{borderRadius: size, borderWidth: 3, borderColor: focused ? colors.focus : 'transparent', padding: 2}}>
            <Artwork source={credit.image} title={credit.name} width={size} height={size} round />
          </View>
          <Text variant="captionStrong" center numberOfLines={1}>{credit.name}</Text>
          <Text variant="caption" tone="tertiary" center numberOfLines={1}>{credit.character ?? credit.job}</Text>
        </View>
      )}
    </Focusable>
  );
}
