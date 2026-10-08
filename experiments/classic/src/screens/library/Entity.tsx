import React from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useQuery} from '../../data/DataProvider';
import {EntryGrid} from '../../ui/Grid';
import {ErrorView} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {Scrim} from '../../ui/Canvas';
import {EmptyState} from '../../ui/Controls';
import {useMetrics} from '../../theme/layout';
import {useContentWidth} from '../../ui/useWidth';
import {useCardHandlers} from '../../app/useCards';
import {useActions} from '../../app/actions';
import {back} from '../../nav/nav';
import {IconButton} from '../../ui/Button';

/** Genre, decade, studio, network, collection, author and series pages. */
export function EntityScreen() {
  const {params} = useRoute() as {params: {libraryId: string; kind: string; id: string; title?: string}};
  const m = useMetrics();
  const actions = useActions();
  const data = useQuery(c => c.libraries.entity(params.libraryId, params.kind, params.id), [params.libraryId, params.kind, params.id], ['personal']);
  const cards = useCardHandlers();
  const [width, onLayout] = useContentWidth();
  const d = data.data;
  const bannerH = m.isTV ? 340 : m.isPhone ? 220 : 280;
  const header = (
    <View style={{marginTop: -(m.isTV ? 48 : m.isPhone ? 100 : 28), marginBottom: m.space.lg}}>
      <View style={{height: bannerH, justifyContent: 'flex-end'}}>
        {d?.backdrop ? <View style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0}}><Artwork source={d.backdrop} title={d.title} width={width} height={bannerH} radius={0} style={{opacity: 0.7}} /><Scrim width={width} height={bannerH} /></View> : null}
        <View style={{paddingHorizontal: m.gutter, flexDirection: 'row', alignItems: 'flex-end', gap: m.space.md}}>
          {!m.isTV ? <IconButton icon="back" label="Back" variant="secondary" onPress={back} size={m.isPhone ? 36 : 40} /> : null}
          <View style={{flex: 1}}>
            <Text variant="label" tone="accent">{d?.subtitle ?? ''}</Text>
            <Text variant="display" numberOfLines={2}>{d?.title ?? params.title ?? ''}</Text>
          </View>
        </View>
      </View>
      <View style={{paddingHorizontal: m.gutter, gap: m.space.md, marginTop: m.space.md}}>
        {d?.overview ? <Text variant="body" tone="secondary" style={{maxWidth: 820}}>{d.overview}</Text> : null}
        {d?.entries.length ? (
          <View style={{flexDirection: 'row', gap: m.space.sm}}>
            <Button label="Play all" icon="play" variant="primary" onPress={() => void actions.play({id: params.id, kind: params.kind === 'collection' ? 'collection' : 'movie', title: d.title, shape: 'poster', libraryId: params.libraryId, ...(params.kind === 'collection' ? {} : {id: d.entries[0]!.id, kind: d.entries[0]!.kind})})} hasTVPreferredFocus />
            <Button label="Shuffle" icon="shuffle" variant="secondary" onPress={() => void actions.play({...d.entries[Math.floor(Math.random() * d.entries.length)]!})} />
          </View>
        ) : null}
      </View>
    </View>
  );
  return (
    <View style={{flex: 1}} onLayout={onLayout}>
      <EntryGrid width={width} entries={d?.entries ?? []} shape={d?.shape ?? 'poster'} loading={data.loading && !d} header={header} {...cards}
        empty={data.error ? <ErrorView message={data.error} onRetry={data.refresh} /> : <EmptyState icon="folder" title="Nothing here" body="The server has no items for this page yet." />} />
    </View>
  );
}
