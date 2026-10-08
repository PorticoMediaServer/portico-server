import React from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useQuery} from '../../data/DataProvider';
import {EntryGrid} from '../../ui/Grid';
import {PageHeader, ErrorView} from '../../ui/Page';
import {EmptyState} from '../../ui/Controls';
import {useContentWidth} from '../../ui/useWidth';
import {useCardHandlers} from '../../app/useCards';
import {back} from '../../nav/nav';

/** A Home row's See all, as a paged grid. */
export function HomeRowScreen() {
  const {params} = useRoute() as {params: {rowId: string; title?: string}};
  const row = useQuery(c => c.home.row(params.rowId), [params.rowId], ['home', 'personal']);
  const cards = useCardHandlers(params.rowId === 'continue' ? 'continue' : undefined);
  const [width, onLayout] = useContentWidth();
  const shape = row.data?.shape === 'circle' ? 'circle' : row.data?.shape ?? 'poster';
  return (
    <View style={{flex: 1}} onLayout={onLayout}>
      <EntryGrid width={width} entries={row.data?.entries ?? []} shape={shape} loading={row.loading && !row.data} {...cards}
        header={<PageHeader title={row.data?.title ?? params.title ?? ''} subtitle={row.data?.reason ?? (row.data ? `${row.data.total} titles` : undefined)} onBack={back} />}
        empty={row.error ? <ErrorView message={row.error} onRetry={row.refresh} /> : <EmptyState icon="sparkle" title="Nothing here right now" body="This row fills in as you watch and as the library grows." />} />
    </View>
  );
}
