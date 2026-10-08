import React from 'react';
import {ScrollView, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useQuery} from '../../data/DataProvider';
import {Screen, ErrorView} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Loading, Pill, Surface} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useToast} from '../../app/toast';
import {formatDuration} from '../../player/PlayerProvider';
import {push} from '../../nav/nav';
import {DetailHero, Facts, ActionRow} from './DetailHero';
import {PersonChip} from './Movie';
import {Shelf} from '../../ui/Shelf';

/** The episode page: its own facts and actions, previous/next, credits and the rest of the season. */
export function EpisodeScreen() {
  const {params} = useRoute() as {params: {id: string}};
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const q = useQuery(c => c.catalog.episode(params.id), [params.id], ['personal']);
  const season = useQuery(async c => { if (!q.data) return []; const s = await c.catalog.show(q.data.show.id); return s.seasons.find(x => x.number === q.data!.entry.season)?.episodes ?? []; }, [q.data?.show.id, q.data?.entry.season]);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  const d = q.data;
  const e = d.entry;
  const code = e.season === 0 ? `Special ${e.episode}` : `Season ${e.season} · Episode ${e.episode}`;
  const remaining = e.progress ? (e.duration ?? 0) * (1 - e.progress) : 0;
  return (
    <Screen bleedTop contentStyle={{gap: m.rowGap}}>
      <DetailHero backdrop={e.image ?? d.show.backdrop} poster={d.show.image} eyebrow={`${d.show.title} · ${code}`} title={e.title} synopsis={e.overview}
        facts={<Facts items={[e.airDate ? new Date(e.airDate).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}) : undefined, formatDuration(e.duration)]}>{e.rating ? <Pill label={e.rating} /> : null}<Pill label={d.versions[0]!.resolution.startsWith('4K') ? '4K' : 'HD'} /></Facts>}>
        <ActionRow>
          <Button label={e.progress ? `Resume · ${formatDuration(remaining)} left` : 'Play'} icon="play" variant="primary" size="lg" onPress={() => void actions.play(e)} hasTVPreferredFocus />
          {e.progress ? <Button label={m.isPhone ? undefined : 'Play from start'} accessibilityLabel="Play from start" icon="refresh" size="lg" onPress={() => void actions.play(e, {fromStart: true})} /> : null}
          <Button icon="watched" size="lg" selected={e.watched} accessibilityLabel={e.watched ? 'Mark as unwatched' : 'Mark as watched'} onPress={async () => { await client.personal.setWatched([e.id], !e.watched); toast({message: e.watched ? 'Marked as unwatched.' : 'Marked as watched.'}); }} />
          <Button icon="more" size="lg" accessibilityLabel="More actions" onPress={() => actions.more(e)} />
        </ActionRow>
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm}}>
          <Button size="sm" icon="back" label="Previous episode" variant="ghost" disabled={!d.previous} onPress={() => d.previous && push('Episode', {id: d.previous.id})} />
          <Button size="sm" iconRight="forward" label="Next episode" variant="ghost" disabled={!d.next} onPress={() => d.next && push('Episode', {id: d.next.id})} />
          <Button size="sm" icon="tv" label="Go to show" variant="ghost" onPress={() => push('Show', {id: d.show.id, season: e.season})} />
        </View>
        <Text variant="caption" tone="secondary">Directed by <Text variant="captionStrong">{d.directors.map(x => x.name).join(', ')}</Text>  ·  Written by <Text variant="captionStrong">{d.writers.map(x => x.name).join(', ')}</Text></Text>
      </DetailHero>
      <View style={{gap: m.isTV ? 18 : 10}}>
        <Text variant="heading" style={{paddingHorizontal: m.gutter}}>Cast</Text>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{paddingHorizontal: m.gutter, gap: m.gap, paddingVertical: m.isTV ? 14 : 2}} style={{marginVertical: m.isTV ? -14 : 0}}>
          {d.cast.map(c => <PersonChip key={c.personId} credit={c} />)}
        </ScrollView>
      </View>
      {season.data?.length ? <Shelf title={d.seasonTitle} entries={season.data.map(x => ({...x, subtitle: d.show.title}))} shape="wide" onPress={x => push('Episode', {id: x.id})} onLongPress={x => actions.more(x)} /> : null}
      <View style={{paddingHorizontal: m.gutter}}>
        <Surface>
          <Text variant="subheading" style={{marginBottom: 8}}>Media information</Text>
          {[['Resolution', d.versions[0]!.resolution], ['Video', d.versions[0]!.video], ['Audio', d.versions[0]!.audio], ['Container', d.versions[0]!.container], ['Size', `${d.versions[0]!.size} · ${d.versions[0]!.bitrate}`]].map(([k, v]) => (
            <View key={k} style={{flexDirection: 'row', gap: m.space.md}}><Text variant="caption" tone="tertiary" style={{width: m.isTV ? 180 : 96}}>{k}</Text><Text variant="caption" style={{flex: 1}}>{v}</Text></View>
          ))}
        </Surface>
      </View>
    </Screen>
  );
}
