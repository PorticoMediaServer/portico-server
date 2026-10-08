import React, {useEffect, useMemo, useState} from 'react';
import {FlatList, StyleSheet, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {Episode, Item, Movie, Season, Show} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button} from '../../design/Button';
import {Card} from '../../design/Card';
import {Chip, Segmented} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Row} from '../../design/Row';
import {EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {push, type Routes} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {openItemMenu, useItemState} from '../shared/actions';
import {code, fmtRemaining, fmtRuntime, openItem, playItem, toCard} from '../shared/cards';
import {HeroInfo} from '../shared/HeroInfo';
import {useItems} from '../shared/useItems';
import {ActionBar, CastRow, DetailsPanel, ExtrasRow, PlaybackChips, RelatedRow, Section} from './parts';
import {Stage} from './Stage';
import {plural} from '../../design/format';

// ---------------------------------------------------------------- Movie (and home video)

export function MovieScreen() {
  const route = useRoute<RouteProp<Routes, 'Movie'>>();
  const s = useServices();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const m = q.data as Movie | undefined;
  const directors = m?.credits.filter(c => c.job === 'Director').map(c => c.name) ?? [];
  const stars = m?.credits.filter(c => c.department === 'Acting').slice(0, 3).map(c => c.name) ?? [];
  return (
    <Stage art={m?.art.backdrop ?? m?.art.poster} tint={m?.tint} loading={!m} error={q.error} onRetry={q.reload} title={m?.title}
      below={m ? (
        <>
          {m.kind === 'movie' ? <ExtrasRow item={m} /> : null}
          {m.credits.length ? <CastRow credits={m.credits} /> : null}
          <RelatedRow id={m.id} />
          <DetailsPanel item={m} />
        </>
      ) : null}>
      {m ? (
        <>
          <HeroInfo item={m} eyebrow={m.kind === 'homevideo' ? 'Home video' : m.franchise ? m.franchise : m.real ? 'Public domain classic' : 'Movie'} />
          {m.tagline ? <Text variant="subheading" tone="accent" style={{marginTop: u(8)}}>{m.tagline}</Text> : null}
          {directors.length || stars.length ? <Text variant="caption" tone="tertiary" style={{marginTop: u(8)}}>{[directors.length ? `Directed by ${directors.join(', ')}` : null, stars.length ? `Starring ${stars.join(', ')}` : null].filter(Boolean).join('  ·  ')}</Text> : null}
          <PlaybackChips item={m} />
          <ActionBar item={m} extra={m.extras[0] ? <Button label="Trailer" icon="film" size="lg" onPress={() => push('Player', {id: m.id, trailer: true})} /> : undefined} />
        </>
      ) : null}
    </Stage>
  );
}

// ---------------------------------------------------------------- Show

export function ShowScreen() {
  const route = useRoute<RouteProp<Routes, 'Show'>>();
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const show = q.data as Show | undefined;
  const seasons = useItems(show?.seasonIds);
  const next = useQuery(show ? 'nextup:' + show.id : null, () => s.catalog.nextUp(show!.id));
  const nextEp = useItems(next.data ? [next.data] : undefined);
  const [seasonId, setSeasonId] = useState<string | undefined>(route.params.season);
  useEffect(() => {
    if (!seasonId && show) {
      const fromNext = next.data?.replace(/e\d+$/, '');
      setSeasonId(fromNext && show.seasonIds.includes(fromNext) ? fromNext : show.seasonIds[0]);
    }
  }, [show, next.data, seasonId]);
  const season = seasonId ? (seasons.lookup(seasonId) as Season | undefined) : undefined;
  const eps = useItems(season?.episodeIds);
  const nextItem = next.data ? (nextEp.lookup(next.data) as Episode | undefined) : undefined;
  const nst = useItemState(next.data ?? '_');
  const watchedCount = season ? season.episodeIds.filter(e => s.user.state(e).watched).length : 0;
  const seasonMenu = () => season && ui.menu({title: season.title, subtitle: `${watchedCount} watched · ${season.episodeIds.length - watchedCount} unwatched`, actions: [
    {label: `Play ${season.title}`, icon: 'play', onPress: () => playItem(eps.lookup(season.episodeIds[0])!, {queue: season.episodeIds})},
    {label: 'Mark season as watched', icon: 'checkCircle', onPress: () => s.user.markContainerWatched(season.id, true).then(n => ui.toast(`${season.title} marked as watched (${plural(n, 'episode', 'episodes')}).`))},
    {label: 'Mark season as unwatched', icon: 'eyeOff', onPress: () => s.user.markContainerWatched(season.id, false).then(() => ui.toast(`${season.title} marked as unwatched.`))},
    {label: 'Jump to…', icon: 'hash', onPress: () => ui.prompt({title: 'Jump to', fields: [{key: 'season', label: 'Season number', initial: String(season.number)}, {key: 'episode', label: 'Episode number', initial: '1'}], submitLabel: 'Go', onSubmit: v => {
      const sid = `${show!.id}-s${parseInt(v.season, 10)}`;
      if (!show!.seasonIds.includes(sid)) return `Choose a season from 1 to ${show!.seasonIds.filter(x => !x.endsWith('-s0')).length}.`;
      push('Episode', {id: `${sid}e${parseInt(v.episode, 10) || 1}`});
      return null;
    }})},
    ...(!isTV ? [{label: 'Download season…', icon: 'download' as const, onPress: () => openItemMenu(s, season, {manager: isManager(session.role)})}] : []),
  ]});
  return (
    <Stage art={show?.art.backdrop ?? show?.art.poster} tint={show?.tint} loading={!show} error={q.error} onRetry={q.reload} title={show?.title}
      below={show ? (
        <>
          <Section title="Episodes" right={<Button icon="more" label={isTV ? 'Season options' : undefined} size="sm" onPress={seasonMenu} accessibilityLabel={`${season?.title ?? 'Season'} options`} />}>
            <View style={{paddingHorizontal: layout.gutter, marginBottom: u(10)}}>
              <FlatList horizontal data={show.seasonIds} keyExtractor={x => x} showsHorizontalScrollIndicator={false} ItemSeparatorComponent={() => <View style={{width: u(8)}} />}
                renderItem={({item: sid}) => {
                  const se = seasons.lookup(sid) as Season | undefined;
                  return <Chip label={se?.title ?? '…'} active={sid === seasonId} onPress={() => setSeasonId(sid)} count={se?.episodeIds.length} />;
                }} />
              {season ? <Text variant="caption" tone="tertiary" style={{marginTop: u(8)}}>{`${watchedCount} watched · ${season.episodeIds.length - watchedCount} unwatched`}</Text> : null}
            </View>
            {season && !season.episodeIds.length ? <EmptyState compact icon="shows" title="No episodes here" body="This season has no episodes on the server yet." /> : null}
            {isTV ? (
              <Row title="" items={eps.cards(season?.episodeIds ?? [], {context: 'showDetail'}).map(c => ({...c, title: c.title, subtitle: c.subtitle}))} onPressItem={c => push('Episode', {id: c.id})} onLongPressItem={c => { const it = eps.lookup(c.id); if (it) openItemMenu(s, it, {manager: isManager(session.role)}); }} />
            ) : (
              <View style={{paddingHorizontal: layout.gutter}}>
                {(season?.episodeIds ?? []).map(id => eps.lookup(id) as Episode | undefined).filter(Boolean).map(ep => <EpisodeRow key={ep!.id} ep={ep!} />)}
              </View>
            )}
          </Section>
          <CastRow credits={show.credits} />
          <RelatedRow id={show.id} />
          <Section title="About">
            <View style={{paddingHorizontal: layout.gutter}}>
              {[['Network', show.network], ['Status', show.status], ['Created by', show.credits.filter(c => c.job === 'Creator').map(c => c.name).join(', ') || '—'], ['Episodes', String(show.episodeCount)], ['Rating', show.contentRating]].map(([k, v]) => (
                <View key={k} style={{marginBottom: u(10)}}><Text variant="eyebrow" tone="tertiary">{k}</Text><Text variant="body" tone="secondary">{v}</Text></View>
              ))}
            </View>
          </Section>
        </>
      ) : null}>
      {show ? (
        <>
          <HeroInfo item={show} eyebrow={show.anime ? 'Anime' : show.network} />
          {nextItem ? (
            <View style={{marginTop: u(12)}}>
              <Text variant="captionStrong" tone="accent">{nst.progress > 0 ? 'Continue' : 'Up next'} · {code(nextItem.seasonNumber, nextItem.number)} · {nextItem.title}</Text>
              {nst.progress > 0 ? <View style={styles.progress}><View style={[styles.progressFill, {width: `${nst.progress * 100}%`}]} /></View> : null}
            </View>
          ) : null}
          {nextItem ? <ActionBarForShow show={show} ep={nextItem} /> : <ActionBar item={show} />}
        </>
      ) : null}
    </Stage>
  );
}

function ActionBarForShow({show, ep}: {show: Show; ep: Episode}) {
  const st = useItemState(ep.id);
  return (
    <ActionBar
      item={show}
      hidePlay
      extra={
        <Button
          label={st.progress > 0 ? `Resume ${code(ep.seasonNumber, ep.number).replace(' · ', ' ')} · ${fmtRemaining((1 - st.progress) * ep.runtime * 60)}` : `Play ${code(ep.seasonNumber, ep.number).replace(' · ', ' ')}`}
          icon="play"
          variant="primary"
          size="lg"
          hasTVPreferredFocus
          onPress={() => playItem(ep, {from: st.position})}
        />
      }
    />
  );
}

function EpisodeRow({ep}: {ep: Episode}) {
  const st = useItemState(ep.id);
  const s = useServices();
  const session = useSession();
  return (
    <Focusable onPress={() => push('Episode', {id: ep.id})} onLongPress={() => openItemMenu(s, ep, {manager: isManager(session.role)})} accessibilityLabel={`${code(ep.seasonNumber, ep.number)}, ${ep.title}`}>
      <View style={styles.epRow}>
        <View>
          <Artwork art={ep.art.still} title={ep.title} tint={ep.tint} fallbackVariant="caption" style={styles.epArt}>
            <View style={styles.playDot}><Icon name="play" size={12} color={colors.white} fill /></View>
            {st.progress > 0 && !st.watched ? <View style={styles.epTrack}><View style={[styles.progressFill, {width: `${st.progress * 100}%`}]} /></View> : null}
          </Artwork>
        </View>
        <View style={{flex: 1, marginLeft: 12}}>
          <Text variant="captionStrong" numberOfLines={1}>{ep.number}. {ep.title}</Text>
          <Text variant="caption" tone="tertiary">{fmtRuntime(ep.runtime)}{st.watched ? ' · Watched' : ''}</Text>
          <Text variant="caption" tone="secondary" numberOfLines={2} style={{marginTop: 2}}>{ep.synopsis}</Text>
        </View>
      </View>
    </Focusable>
  );
}

// ---------------------------------------------------------------- Episode page

export function EpisodeScreen() {
  const route = useRoute<RouteProp<Routes, 'Episode'>>();
  const s = useServices();
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const ep = q.data as Episode | undefined;
  const showQ = useQuery(ep ? 'item:' + ep.showId : null, () => s.catalog.item(ep!.showId));
  const seasonQ = useQuery(ep ? 'item:' + ep.seasonId : null, () => s.catalog.item(ep!.seasonId));
  const show = showQ.data as Show | undefined;
  const season = seasonQ.data as Season | undefined;
  const idx = season && ep ? season.episodeIds.indexOf(ep.id) : -1;
  const prev = idx > 0 ? season!.episodeIds[idx - 1] : undefined;
  const next = season && idx >= 0 && idx < season.episodeIds.length - 1 ? season.episodeIds[idx + 1] : undefined;
  const more = useItems(season?.episodeIds);
  return (
    <Stage art={ep?.art.still ?? show?.art.backdrop} tint={ep?.tint} loading={!ep} error={q.error} onRetry={q.reload} title={ep?.title}
      below={ep ? (
        <>
          <Row title={`More from ${season?.title ?? 'this season'}`} items={more.cards(season?.episodeIds ?? [], {context: 'showDetail'})} onPressItem={c => push('Episode', {id: c.id})} />
          <CastRow credits={ep.credits} />
          <DetailsPanel item={ep} />
        </>
      ) : null}>
      {ep ? (
        <>
          <Focusable onPress={() => show && push('Show', {id: show.id})} accessibilityRole="link" accessibilityLabel={`Go to ${show?.title}`} style={{alignSelf: 'flex-start'}}>
            {({focused}) => <Text variant="eyebrow" color={focused && isTV ? colors.actionInk : colors.accentStrong} style={focused && isTV ? {backgroundColor: colors.white, borderRadius: 8, paddingHorizontal: 8, marginHorizontal: -8} : undefined}>{show?.title ?? ''} ›</Text>}
          </Focusable>
          <HeroInfo item={ep} lookup={id => (id === show?.id ? show : undefined)} />
          <Text variant="caption" tone="tertiary" style={{marginTop: 6}}>Aired {ep.airDate}</Text>
          <PlaybackChips item={ep} />
          <ActionBar item={ep} extra={
            <>
              {prev ? <Button icon="chevronLeft" accessibilityLabel="Previous episode" size="lg" onPress={() => push('Episode', {id: prev})} /> : null}
              {next ? <Button icon="chevronRight" accessibilityLabel="Next episode" size="lg" onPress={() => push('Episode', {id: next})} /> : null}
            </>
          } />
        </>
      ) : null}
    </Stage>
  );
}

// ---------------------------------------------------------------- Person

export function PersonScreen() {
  const route = useRoute<RouteProp<Routes, 'Person'>>();
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const p = useQuery('person:' + route.params.id, () => s.catalog.person(route.params.id));
  const cr = useQuery('credits:' + route.params.id, () => s.catalog.personCredits(route.params.id));
  const [tab, setTab] = useState<'all' | 'cast' | 'crew'>('all');
  const [more, setMore] = useState(false);
  const ids = useMemo(() => {
    const cast = cr.data?.cast.map(c => c.itemId) ?? [];
    const crew = cr.data?.crew.map(c => c.itemId) ?? [];
    return [...new Set(tab === 'cast' ? cast : tab === 'crew' ? crew : [...cast, ...crew])];
  }, [cr.data, tab]);
  const items = useItems(ids);
  const role = (id: string) => cr.data?.cast.find(c => c.itemId === id)?.character ?? cr.data?.crew.find(c => c.itemId === id)?.job;
  const person = p.data;
  const known = [...ids].map(id => items.lookup(id)).filter(Boolean).sort((a, b) => ((b as Movie).audienceScore ?? 0) - ((a as Movie).audienceScore ?? 0)).slice(0, 8) as Item[];
  const cols = isTV ? 6 : layout.compact ? 3 : 5;
  const w = Math.floor((layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * (isTV ? 36 : 12)) / cols);
  return (
    <Screen title={person?.name}>
      {person ? (
        <View style={[styles.personHead, layout.compact && {flexDirection: 'column', alignItems: 'center'}]}>
          <Artwork art={person.art.headshot} title={person.name.split(' ').map(x => x[0]).join('')} tint={person.tint} style={{width: u(150), height: u(150), borderRadius: u(75)}} fallbackVariant="display" />
          <View style={[{flex: 1}, layout.compact ? {alignItems: 'center', marginTop: 14} : {marginLeft: u(28)}]}>
            <Text variant="eyebrow" tone="accent">{person.knownFor}</Text>
            <Text variant="hero" center={layout.compact}>{person.name}</Text>
            <Text variant="caption" tone="tertiary" center={layout.compact}>{[person.born ? (person.died ? `${person.born} – ${person.died}` : `Born ${person.born}`) : null, person.birthplace].filter(Boolean).join(' · ')}</Text>
            <Text variant="body" tone="secondary" numberOfLines={more ? undefined : 3} style={{marginTop: u(10), maxWidth: 900}}>{person.bio || 'No biography on this server yet.'}</Text>
            {person.bio && person.bio.length > 140 ? <Button label={more ? 'Show less' : 'Read more'} variant="ghost" size="sm" onPress={() => setMore(!more)} style={{alignSelf: layout.compact ? 'center' : 'flex-start'}} /> : null}
          </View>
        </View>
      ) : null}
      {known.length > 3 ? (
        <View style={{marginHorizontal: -layout.gutter, marginTop: u(20)}}>
          <Row title="Known for" items={known.map(i => toCard(i, s.user.state(i.id)))} onPressItem={c => { const it = items.lookup(c.id); if (it) openItem(it); }} />
        </View>
      ) : null}
      <View style={{flexDirection: 'row', alignItems: 'center', marginTop: u(16), marginBottom: u(12), gap: 12}}>
        <Text variant="title" style={{flex: 1}}>Credits</Text>
        <Segmented value={tab} onChange={setTab} options={[{value: 'all', label: 'All'}, {value: 'cast', label: 'Cast'}, {value: 'crew', label: 'Crew'}]} />
      </View>
      {!ids.length && cr.data ? <EmptyState compact icon="users" title="No credits here" body={tab === 'crew' ? 'No crew credits on this server.' : tab === 'cast' ? 'No acting credits on this server.' : 'Nothing on this server credits this person yet.'} /> : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: isTV ? 36 : 12}}>
        {ids.map(id => items.lookup(id)).filter(Boolean).map(it => (
          <Card key={it!.id} item={{...toCard(it!, s.user.state(it!.id)), subtitle: role(it!.id) ? `as ${role(it!.id)}` : String(it!.year ?? '')}} width={w} showTitle="always" onPress={() => openItem(it!)} onLongPress={() => openItemMenu(s, it!, {manager: isManager(session.role)})} />
        ))}
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  progress: {height: 4, borderRadius: 2, backgroundColor: 'rgba(255,255,255,0.2)', marginTop: 6, width: 220, overflow: 'hidden'},
  progressFill: {height: '100%', backgroundColor: colors.accentStrong},
  epRow: {flexDirection: 'row', paddingVertical: 10},
  epArt: {width: 150, height: 84, borderRadius: radius.sm},
  playDot: {position: 'absolute', left: 6, bottom: 6, width: 22, height: 22, borderRadius: 11, backgroundColor: 'rgba(7,16,24,0.7)', alignItems: 'center', justifyContent: 'center'},
  epTrack: {position: 'absolute', left: 0, right: 0, bottom: 0, height: 3, backgroundColor: 'rgba(255,255,255,0.3)'},
  personHead: {flexDirection: 'row', alignItems: 'center', marginTop: u(10)},
});

export {Segmented};
