import React from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useClient, useQuery} from '../../data/DataProvider';
import {Screen, ErrorView} from '../../ui/Page';
import {Shelf} from '../../ui/Shelf';
import {Button} from '../../ui/Button';
import {Loading, ProgressBar} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Focusable} from '../../ui/Focusable';
import {Icon} from '../../ui/Icon';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useActions} from '../../app/actions';
import {useCardHandlers} from '../../app/useCards';
import {useToast} from '../../app/toast';
import {formatDuration, formatTime, usePlayer} from '../../player/PlayerProvider';
import {push} from '../../nav/nav';
import {DetailHero, Facts, ActionRow} from '../detail/DetailHero';

/** An audiobook: cover, author and narrator, resume, and the chapter list. */
export function AudiobookScreen() {
  const {params} = useRoute() as {params: {id: string}};
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const player = usePlayer();
  const toast = useToast();
  const cards = useCardHandlers();
  const q = useQuery(c => c.catalog.audiobook(params.id), [params.id], ['personal']);
  if (q.error) return <Screen><ErrorView message={q.error} onRetry={q.refresh} /></Screen>;
  if (!q.data) return <Screen><Loading /></Screen>;
  const d = q.data;
  const e = d.entry;
  const total = e.duration ?? 1;
  const current = d.chapters.findIndex((c, i) => d.progress >= c.start && d.progress < (d.chapters[i + 1]?.start ?? Infinity));
  return (
    <Screen bleedTop contentStyle={{gap: m.rowGap}}>
      <DetailHero poster={e.image} eyebrow={d.series ? `${d.series.name} · Book ${d.series.index}` : `Audiobook · ${d.genre}`} title={e.title} synopsis={e.overview}
        facts={
          <View style={{gap: 6}}>
            <Focusable onPress={() => push('Entity', {libraryId: 'lib-audiobooks', kind: 'author', id: d.author, title: d.author})} style={({focused}) => ({alignSelf: 'flex-start', borderRadius: 8, paddingHorizontal: m.isTV ? 8 : 0, backgroundColor: focused ? colors.raisedHigh : 'transparent'})}><Text variant="subheading" tone="accent">{d.author}</Text></Focusable>
            <Facts items={[`Read by ${d.narrator}`, formatDuration(total), `${d.chapters.length} chapters`, d.publisher]} />
          </View>
        }>
        {d.progress ? <View style={{gap: 4, maxWidth: 460}}><ProgressBar value={d.progress / total} /><Text variant="caption" tone="tertiary">{formatDuration(total - d.progress)} left · Chapter {current + 1}</Text></View> : null}
        <ActionRow>
          <Button label={d.progress ? `Resume chapter ${current + 1}` : 'Play'} icon="play" variant="primary" size="lg" onPress={() => void actions.play(e)} hasTVPreferredFocus />
          {d.progress ? <Button label={m.isPhone ? undefined : 'Start over'} accessibilityLabel="Start over" icon="refresh" size="lg" onPress={() => void actions.play(e, {fromStart: true})} /> : null}
          <Button icon={d.personal.favorite ? 'heartFilled' : 'heart'} size="lg" selected={d.personal.favorite} accessibilityLabel={d.personal.favorite ? 'Remove from Favorites' : 'Add to Favorites'} onPress={async () => { await client.personal.setFavorite([e.id], !d.personal.favorite); toast({message: d.personal.favorite ? 'Removed from Favorites.' : 'Added to Favorites.'}); }} />
          <Button icon="more" size="lg" accessibilityLabel="More actions" onPress={() => actions.more(e)} />
        </ActionRow>
      </DetailHero>
      <View style={{paddingHorizontal: m.gutter, gap: 2}}>
        <Text variant="heading" style={{marginBottom: 6}}>Chapters</Text>
        {d.chapters.map((c, i) => (
          <Focusable key={c.number} onPress={() => player.play([e], 0, {mode: 'book', position: c.start})} accessibilityLabel={`${c.title}, ${formatTime(c.duration)}`}
            style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.md, paddingVertical: m.isTV ? 14 : 10, paddingHorizontal: 10, borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedBright : i === current ? 'rgba(64,159,212,0.1)' : 'transparent'})}>
            <Text variant="captionStrong" tone="tertiary" style={{width: m.isTV ? 44 : 28}}>{c.number}</Text>
            <Text variant="body" style={{flex: 1}} numberOfLines={1} color={i === current ? colors.accentStrong : colors.text}>{c.title}</Text>
            {i < current ? <Icon name="check" size={m.iconSmall} color={colors.textTertiary} /> : null}
            <Text variant="caption" tone="tertiary">{formatTime(c.duration)}</Text>
          </Focusable>
        ))}
      </View>
      {d.more.length ? <Shelf title={`More by ${d.author}`} entries={d.more} onPress={cards.onPress} onLongPress={cards.onLongPress} /> : null}
    </Screen>
  );
}
