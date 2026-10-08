import {useQuery} from '@/data/DataProvider';
import {Page, PageHeader} from '@/ui/Page';
import {CardGrid} from '@/ui/Shelf';
import {EmptyState, ErrorState} from '@/ui/Bits';
import {useTitle} from '@/ui/hooks';
import {plural} from '@/app/format';
import {useRouteParams} from '../common';

/** A Home row's See all: the whole row as a grid. */
export default function HomeRowPage() {
  const {rowId} = useRouteParams<{rowId: string}>();
  const row = useQuery(c => c.home.row(rowId), [rowId], ['home', 'personal']);
  useTitle(row.data?.title);
  if (row.error) return <ErrorState onRetry={row.refresh} />;
  return (
    <Page>
      <PageHeader back="/" title={row.data?.title ?? ' '} subtitle={row.data ? [row.data.reason, plural(row.data.total, 'title')].filter(Boolean).join(' · ') : undefined} />
      {row.data && !row.data.entries.length ? <EmptyState icon="sparkle" title="Nothing here right now" body="This row fills in as you watch and as new titles arrive." /> : <CardGrid entries={row.data?.entries} loading={row.loading} shape={row.data?.shape ?? 'poster'} origin={row.data?.kind === 'continue' ? 'continue' : row.data?.kind === 'picks' ? 'recommendation' : undefined} />}
    </Page>
  );
}
