import React, {useEffect, useState} from 'react';
import {ScrollView, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useQuery} from '../../data/DataProvider';
import type {EpisodeSummary, ShowDetail} from '../../data/types';
import {Screen, ErrorView} from '../../ui/Page';
import {Shelf} from '../../ui/Shelf';
import {Button} from '../../ui/Button';
import {Loading, Pill, ProgressBar, Tabs} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Icon} from '../../ui/Icon';
import {Focusable} from '../../ui/Focusable';
import {Artwork} from '../../ui/Artwork';
import {ActionMenu} from '../../ui/Sheet';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useCardHandlers} from '../../app/useCards';
import {useToast} from '../../app/toast';
import {formatDuration} from '../../player/PlayerProvider';
import {push} from '../../nav/nav';
import {DetailHero, Facts, ActionRow} from './DetailHero';
import {PersonChip} from './Movie';
import {useGlow} from '../../app/glow';

/** Show detail: next up, seasons, episodes, cast and related shows. */
export function ShowScreen() {
  const {params} = useRoute() as {params: {id: string; season?: number}};
  const glow = useGlow();
  const q = useQuery(c => c.catalog.show(params.id), [params.id], ['personal']);
  useEffect(() => { glow(params.id); return () => glow(undefined); }, [params.id, glow]);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  return <ShowBody d={q.data} initialSeason={params.season} />;
}

function ShowBody({d, initialSeason}: {d: ShowDetail; initialSeason?: number}) {
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const cards = useCardHandlers();
  const e = d.entry;
  const seasons = d.seasons;
  const defaultSeason = initialSeason ?? d.nextUp?.season ?? seasons.find(s => s.number > 0)?.number ?? seasons[0]!.number;
  const [season, setSeason] = useState(defaultSeason);
  const [seasonMenu, setSeasonMenu] = useState(false);
  const cur = seasons.find(s => s.number === season) ?? seasons[0]!;
  const next = d.nextUp;
  const code = (x: EpisodeSummary) => (x.season === 0 ? `Special ${x.episode}` : `S${x.season} E${x.episode}`);
  return (
    <Screen bleedTop contentStyle={{gap: m.rowGap}}>
      <DetailHero backdrop={e.backdrop} poster={e.image} eyebrow={d.network} title={e.title} synopsis={e.overview}
        facts={<Facts items={[e.subtitle, `${seasons.filter(s => s.number > 0).length} season${seasons.filter(s => s.number > 0).length === 1 ? '' : 's'}`, ...(e.genres ?? []).slice(0, 2), `${d.watchedCount} watched · ${d.episodeCount - d.watchedCount} unwatched`]}>{e.rating ? <Pill label={e.rating} /> : null}{d.audienceRating ? <View style={{flexDirection: 'row', gap: 4, alignItems: 'center'}}><Icon name="starFilled" size={m.iconSmall} color={colors.amber} /><Text variant="captionStrong">{d.audienceRating.toFixed(1)}</Text></View> : null}</Facts>}>
        <ActionRow>
          <Button label={next ? `${next.progress ? 'Resume' : 'Play'} ${code(next)}` : 'Play'} icon="play" variant="primary" size="lg" onPress={() => void actions.play(next ?? e)} hasTVPreferredFocus />
          <Button label={m.isPhone ? undefined : 'Shuffle'} accessibilityLabel="Shuffle" icon="shuffle" size="lg" onPress={() => void actions.play(e, {shuffle: true})} />
          <Button icon={d.personal.watchlisted ? 'savedFilled' : 'saved'} size="lg" selected={d.personal.watchlisted} accessibilityLabel={d.personal.watchlisted ? 'Remove from Watchlist' : 'Add to Watchlist'} onPress={async () => { await client.personal.setWatchlist([e.id], !d.personal.watchlisted); toast({message: d.personal.watchlisted ? 'Removed from Watchlist.' : 'Added to Watchlist.'}); }} />
          <Button icon="watched" size="lg" selected={e.watched} accessibilityLabel={e.watched ? 'Mark show as unwatched' : 'Mark show as watched'} onPress={async () => { await client.personal.setWatched([e.id], !e.watched); toast({message: e.watched ? 'Show marked as unwatched.' : 'Show marked as watched.'}); }} />
          <Button icon="more" size="lg" accessibilityLabel="More actions" onPress={() => actions.more(e)} />
        </ActionRow>
        {next ? <Text variant="caption" tone="secondary">Next up: <Text variant="captionStrong">{code(next)} · {next.title}</Text></Text> : null}
        {d.creators.length ? <Text variant="caption" tone="secondary">Created by <Text variant="captionStrong">{d.creators.map(c => c.name).join(', ')}</Text></Text> : null}
      </DetailHero>

      <View style={{gap: m.space.md}}>
        <View style={{paddingHorizontal: m.gutter, flexDirection: 'row', alignItems: 'center', gap: m.space.md}}>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{flex: 1}} contentContainerStyle={{paddingVertical: m.isTV ? 6 : 0}}>
            <Tabs tabs={seasons.map(s => ({value: String(s.number), label: s.number === 0 ? 'Specials' : `Season ${s.number}`, count: s.episodes.length}))} value={String(season)} onChange={v => setSeason(Number(v))} style={{flexWrap: 'nowrap'}} />
          </ScrollView>
          <Button icon="more" label={m.isPhone ? undefined : 'Season options'} accessibilityLabel={`${cur.title} options`} size="sm" variant="secondary" onPress={() => setSeasonMenu(true)} />
        </View>
        <View style={{paddingHorizontal: m.gutter, flexDirection: 'row', gap: m.space.md, alignItems: 'center'}}>
          <Text variant="caption" tone="tertiary">{cur.year ? `${cur.year} · ` : ''}{cur.episodes.length} episodes · {cur.watchedCount} watched</Text>
        </View>
        <View style={{paddingHorizontal: m.gutter, gap: m.isTV ? 12 : 6}}>
          {cur.episodes.length ? cur.episodes.map(ep => <EpisodeRow key={ep.id} ep={ep} isNext={next?.id === ep.id} />) : <Text variant="body" tone="tertiary">This season has no episodes on the server yet.</Text>}
        </View>
      </View>

      {d.cast.length ? (
        <View style={{gap: m.isTV ? 18 : 10}}>
          <Text variant="heading" style={{paddingHorizontal: m.gutter}}>Cast</Text>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{paddingHorizontal: m.gutter, gap: m.gap, paddingVertical: m.isTV ? 14 : 2}} style={{marginVertical: m.isTV ? -14 : 0}}>
            {[...d.creators, ...d.cast].map((c, i) => <PersonChip key={`${c.personId}-${i}`} credit={c} />)}
          </ScrollView>
        </View>
      ) : null}
      {d.related.length ? <Shelf title="More like this" entries={d.related} onPress={cards.onPress} onLongPress={cards.onLongPress} onFocus={cards.onFocus} /> : null}
      <ActionMenu visible={seasonMenu} onClose={() => setSeasonMenu(false)} title={`${cur.title} options`} subtitle={e.title} items={[
        {id: 'play', label: `Play ${cur.title}`, icon: 'play'},
        {id: 'watched', label: cur.watchedCount === cur.episodes.length ? 'Mark season as unwatched' : 'Mark season as watched', icon: 'watched'},
        {id: 'download', label: 'Download season…', icon: 'download'},
        {id: 'playlist', label: 'Add season to playlist…', icon: 'playlist'},
      ]} onSelect={async id => {
        setSeasonMenu(false);
        if (id === 'play') { const first = cur.episodes.find(x => !x.watched) ?? cur.episodes[0]; if (first) void actions.play(first); }
        if (id === 'watched') { const on = cur.watchedCount !== cur.episodes.length; await client.personal.setWatched(cur.episodes.map(x => x.id), on); toast({message: `${cur.title} marked as ${on ? 'watched' : 'unwatched'}`}); }
        if (id === 'download') actions.more(e);
        if (id === 'playlist') actions.addTo('playlist', cur.episodes);
      }} />
    </Screen>
  );
}

function EpisodeRow({ep, isNext}: {ep: EpisodeSummary; isNext: boolean}) {
  const m = useMetrics();
  const actions = useActions();
  const w = m.isTV ? 300 : m.isPhone ? 136 : 220;
  return (
    <Focusable onPress={() => push('Episode', {id: ep.id})} onLongPress={() => actions.more(ep)} focusScale={m.isTV ? 1.02 : 1} accessibilityLabel={`Episode ${ep.episode}, ${ep.title}`}
      style={({focused}) => ({flexDirection: 'row', gap: m.space.md, padding: m.isTV ? 14 : 8, borderRadius: m.radius.panel, backgroundColor: focused ? colors.raisedBright : isNext ? 'rgba(64,159,212,0.1)' : 'transparent', borderWidth: 1, borderColor: focused ? colors.focus : 'transparent'})}>
      <View>
        <Artwork source={ep.image} title={ep.title} width={w} height={w * 0.5625} radius={m.radius.card} />
        <View style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, alignItems: 'center', justifyContent: 'center'}}>
          {m.isTV ? <View style={{width: 64, height: 64, borderRadius: 40, backgroundColor: 'rgba(7,16,24,0.6)', alignItems: 'center', justifyContent: 'center'}}><Icon name="play" size={28} /></View> : (
            <Focusable onPress={() => void actions.play(ep)} accessibilityLabel={`Play episode ${ep.episode}`} style={{width: 40, height: 40, borderRadius: 40, backgroundColor: 'rgba(7,16,24,0.6)', alignItems: 'center', justifyContent: 'center'}}>
              <Icon name="play" size={18} />
            </Focusable>
          )}
        </View>
        {ep.progress ? <View style={{position: 'absolute', left: 8, right: 8, bottom: 8}}><ProgressBar value={ep.progress} /></View> : null}
      </View>
      <View style={{flex: 1, gap: 3}}>
        <View style={{flexDirection: 'row', alignItems: 'center', gap: 8}}>
          <Text variant="captionStrong" tone="accent">{ep.season === 0 ? 'Special' : `Episode ${ep.episode}`}</Text>
          {ep.watched ? <Icon name="watched" size={m.iconSmall} color={colors.accentStrong} /> : null}
          {isNext ? <Text variant="micro" tone="accent">UP NEXT</Text> : null}
        </View>
        <Text variant="bodyStrong" numberOfLines={1}>{ep.title}</Text>
        <Text variant="caption" tone="tertiary">{[formatDuration(ep.duration), ep.airDate ? new Date(ep.airDate).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: 'numeric'}) : undefined].filter(Boolean).join(' · ')}</Text>
        {!m.isPhone ? <Text variant="caption" tone="secondary" numberOfLines={2}>{ep.overview}</Text> : null}
      </View>
    </Focusable>
  );
}
