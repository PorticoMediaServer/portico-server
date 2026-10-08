import React, {Suspense, lazy} from 'react';
import {Link, useNavigate, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Collection, Item, Row} from '../../data/types';
import {useSession} from '../../app/session';
import {Page} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {ErrorState, Skeleton, StateView} from '../../ui/Feedback';
import {Art} from '../../ui/Media';
import {Icon} from '../../ui/Icon';
import {Hero} from './Hero';
import {Shelf} from '../shared/Shelf';
import {CardSkeleton, MediaCard} from '../shared/Cards';
import {backdropFor} from '../shared/items';
import {setTint} from '../../app/tint';

const Customize = lazy(() => import('./Customize').then(m => ({default: m.CustomizeHome})));

export function HomeScreen() {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/'});
  const rows = useQuery('home', () => services.catalog.home());
  const ids = [...new Set((rows.data ?? []).flatMap(r => r.itemIds.slice(0, 20)))];
  const items = useQuery(rows.data ? `home.items.${rows.data.map(r => r.id + r.itemIds.length).join()}` : null, () => services.catalog.items(ids));
  const collections = useQuery('collections.all', () => services.catalog.collections());
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  const closeCustomize = () => void navigate({to: '/', search: {}, replace: true});
  const loading = !rows.data || (!items.data && rows.loading !== false) || (!items.data && !!rows.data?.length);
  if (rows.error) return <Page title="Home"><ErrorState error={rows.error} onRetry={rows.reload} what="Home" /></Page>;
  if (!loading && !rows.data?.length) {
    return (
      <Page title="Home">
        {session.manager ? (
          <StateView icon="library" title="Your Home fills up as you add media" body="Add a library and Portico finds the films, shows and music in it, then shows what’s new and what to watch next right here." action={{label: 'Add a library', icon: 'plus', onClick: () => void navigate({to: '/server/$section', params: {section: 'libraries'}})}} secondary={{label: 'Customize Home', onClick: () => void navigate({to: '/', search: {customize: true}})}} />
        ) : (
          <StateView icon="library" title="Nothing’s been shared with you yet" body="When the owner of this server shares a library with you, what’s new and what to watch next appears here." action={{label: 'Switch server', icon: 'server', onClick: () => void navigate({to: '/welcome/$screen', params: {screen: 'servers'}})}} />
        )}
        {search.customize ? <Suspense fallback={null}><Customize onClose={closeCustomize} /></Suspense> : null}
      </Page>
    );
  }
  const featured = rows.data?.find(r => r.kind === 'featured');
  const heroItems = (featured?.itemIds ?? []).map(id => byId.get(id)).filter((x): x is Item => !!x);
  const rest = (rows.data ?? []).filter(r => r.kind !== 'featured');
  const showTitles = new Map<string, string>();
  (items.data ?? []).forEach(i => i.kind === 'show' && showTitles.set(i.id, i.title));
  return (
    <Page title="Home" flush className="home">
      {loading ? <HeroSkeleton /> : heroItems.length ? <Hero items={heroItems} /> : <div style={{height: 40}} />}
      <div className="home-rows">
        {loading ? [0, 1, 2].map(i => <RowSkeleton key={i} />) : rest.map((row, i) => (
          <React.Fragment key={row.id}>
            <HomeRow row={row} byId={byId} />
            {i === 3 && collections.data ? <CollectionFeature collection={collections.data.find(c => c.id === 'col-midnight-movies') ?? collections.data[0]} byId={byId} /> : null}
          </React.Fragment>
        ))}
        {!loading ? (
          <div className="home-foot">
            <Button variant="secondary" icon="sliders" label="Customize Home" to="/" search={{customize: true}} />
            <p className="faint">Reorder, hide or add rows, including your saved views.</p>
          </div>
        ) : null}
      </div>
      {search.customize ? <Suspense fallback={null}><Customize onClose={closeCustomize} /></Suspense> : null}
    </Page>
  );
}

function HomeRow({row, byId}: {row: Row; byId: Map<string, Item>}) {
  const services = useServices();
  const showIds = row.itemIds.map(id => byId.get(id)).filter(i => i?.kind === 'episode').map(i => (i as {showId: string}).showId);
  const shows = useQuery(showIds.length ? `shows.${[...new Set(showIds)].join()}` : null, () => services.catalog.items([...new Set(showIds)]));
  const showTitle = (item: Item) => (item.kind === 'episode' ? shows.data?.find(s => s.id === item.showId)?.title : undefined);
  const list = row.itemIds.map(id => byId.get(id)).filter((x): x is Item => !!x);
  if (!list.length) return null;
  const ranked = row.kind === 'popular';
  const context = row.kind === 'continue' ? 'continue' : row.recommendation ? 'recommendation' : 'card';
  const eyebrow = row.recommendation ? 'For you' : row.kind === 'continue' || row.kind === 'upNext' ? undefined : row.kind === 'recentIn' ? 'New' : undefined;
  return (
    <Shelf id={`row-${row.id}`} title={ranked ? 'Top 10 on this server' : row.title} subject={row.subject} eyebrow={eyebrow} variant={ranked ? 'ranked' : undefined} seeAll={list.length > 6 ? {to: '/home/rows/$rowId', params: {rowId: row.id}} : undefined} count={row.total && row.total > list.length ? row.total : undefined}>
      {(ranked ? list.slice(0, 10) : list).map((item, i) => (
        <MediaCard key={item.id} item={item} context={context} rank={ranked ? i + 1 : undefined} showTitle={showTitle(item)} shape={row.shape === 'square' ? undefined : undefined} />
      ))}
    </Shelf>
  );
}

/** An editorial break between rows: one collection, told big. */
function CollectionFeature({collection, byId}: {collection?: Collection; byId: Map<string, Item>}) {
  const services = useServices();
  const ids = collection?.itemIds.slice(0, 8) ?? [];
  const items = useQuery(collection ? `collection.feature.${collection.id}` : null, () => services.catalog.items(ids));
  if (!collection) return null;
  const list = items.data ?? ids.map(id => byId.get(id)).filter((x): x is Item => !!x);
  const lead = list.find(i => i.art.backdrop);
  return (
    <section className="feature" aria-labelledby="feature-title" onMouseEnter={() => setTint(collection.tint)}>
      <div className="feature-media" aria-hidden>
        {lead ? <Art src={backdropFor(lead)} tint={collection.tint} /> : null}
        <div className="feature-scrim" style={{'--c': collection.tint} as React.CSSProperties} />
      </div>
      <div className="feature-text">
        <div className="eyebrow feature-eyebrow">Collection</div>
        <h2 className="display feature-title" id="feature-title">{collection.title}</h2>
        <p className="feature-desc">{collection.description} {collection.itemIds.length} films.</p>
        <Link className="btn btn-glass btn-md" to="/library/$libraryId/$view/$entityId" params={{libraryId: collection.libraryId ?? 'lib-movies', view: 'collection', entityId: collection.id}}><span className="btn-label">Explore the collection</span><Icon name="chevronRight" size={16} /></Link>
      </div>
      <div className="feature-posters">
        {list.slice(0, 6).map(item => <MediaCard key={item.id} item={item} caption={false} />)}
      </div>
    </section>
  );
}

function HeroSkeleton() {
  return (
    <div className="hero hero-skeleton" aria-busy="true" aria-label="Loading Home">
      <div className="hero-content">
        <Skeleton w={120} h={12} />
        <Skeleton w="min(520px, 80%)" h={64} style={{marginTop: 18}} />
        <Skeleton w="min(380px, 60%)" h={14} style={{marginTop: 18}} />
        <Skeleton w="min(460px, 70%)" h={14} style={{marginTop: 10}} />
        <div className="hero-actions"><Skeleton w={140} h={52} r={26} /><Skeleton w={160} h={52} r={26} /></div>
      </div>
    </div>
  );
}

export function RowSkeleton({shape = 'poster'}: {shape?: 'poster' | 'square' | 'landscape'}) {
  return (
    <div className="shelf" aria-hidden>
      <div className="shelf-head"><Skeleton w={220} h={20} /></div>
      <div className="shelf-scroller" style={{overflow: 'hidden'}}>{Array.from({length: 9}, (_, i) => <CardSkeleton key={i} shape={shape} />)}</div>
    </div>
  );
}
