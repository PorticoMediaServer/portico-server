import {useParams} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import {Page, PageHeader} from '../../ui/Layout';
import {ErrorState, StateView} from '../../ui/Feedback';
import {CardSkeleton, MediaCard} from '../shared/Cards';
import {plural} from '../shared/items';

/** A Home row's See all: the whole row as a grid. */
export function HomeRowScreen() {
  const {rowId} = useParams({from: '/app/home/rows/$rowId'});
  const services = useServices();
  const row = useQuery(`row.${rowId}`, () => services.catalog.row(rowId));
  const items = useQuery(row.data ? `row.items.${rowId}.${row.data.itemIds.length}` : null, () => services.catalog.items(row.data!.itemIds));
  const shows = useQuery(items.data ? `row.shows.${rowId}` : null, () => services.catalog.items([...new Set((items.data ?? []).map(i => (i.kind === 'episode' ? i.showId : '')).filter(Boolean))]));
  const title = row.data ? `${row.data.title}${row.data.subject ? ` ${row.data.subject}` : ''}` : 'Home';
  if (row.error) return <Page><ErrorState error={row.error} onRetry={row.reload} /></Page>;
  if (row.data === undefined && !row.loading) return <Page title="Row"><StateView icon="home" title="This row isn’t on Home any more" action={{label: 'Go home', to: '/', icon: 'home'}} /></Page>;
  return (
    <Page title={title}>
      <PageHeader back={{label: 'Home', to: '/'}} eyebrow={row.data?.recommendation ? 'For you' : 'Home'} title={row.data ? <>{row.data.title}{row.data.subject ? <> <span className="shelf-subject">{row.data.subject}</span></> : null}</> : '…'} lede={row.data ? plural(row.data.itemIds.length, 'title') : undefined} />
      <div className="card-grid">
        {items.data ? items.data.map(i => <MediaCard key={i.id} item={i} fill context={row.data?.kind === 'continue' ? 'continue' : row.data?.recommendation ? 'recommendation' : 'card'} showTitle={i.kind === 'episode' ? shows.data?.find(s => s.id === i.showId)?.title : undefined} />) : Array.from({length: 12}, (_, i) => <CardSkeleton key={i} fill />)}
      </div>
    </Page>
  );
}
