import {useQuery} from '@/data/DataProvider';
import {useActions} from '@/app/actions';
import {usePlayer} from '@/player/PlayerProvider';
import {plural} from '@/app/format';
import {Button} from '@/ui/Button';
import {Artwork, EmptyState, ErrorState} from '@/ui/Bits';
import {CardGrid} from '@/ui/Shelf';
import {BackLink} from '@/ui/Page';
import {useTitle} from '@/ui/hooks';
import {useRouteParams} from '../common';
import s from './Entity.module.css';

const LABEL: Record<string, string> = {genre: 'Genre', studio: 'Studio', decade: 'Decade', network: 'Network', collection: 'Collection', author: 'Author', series: 'Series', rating: 'Rating'};

/** Genre, studio, decade, network, collection, author and series pages: the matching titles with Play all and Shuffle. */
export default function EntityScreen() {
  const {libraryId, view, entityId} = useRouteParams<{libraryId: string; view: string; entityId: string}>();
  const id = decodeURIComponent(entityId);
  const data = useQuery(c => c.libraries.entity(libraryId, view, id), [libraryId, view, id], ['personal']);
  const actions = useActions();
  const player = usePlayer();
  useTitle(data.data?.title ?? id);
  if (data.error) return <ErrorState title="This page didn’t load" body={data.error} onRetry={data.refresh} />;
  const d = data.data;
  const playable = d?.entries.filter(e => e.kind === 'movie' || e.kind === 'homevideo' || e.kind === 'audiobook') ?? [];
  const playAll = (shuffle: boolean) => {
    if (!d) return;
    if (playable.length) { const q = shuffle ? [...playable].sort(() => Math.random() - 0.5) : playable; player.play(q, 0); }
    else if (d.entries[0]) void actions.play(d.entries[shuffle ? Math.floor(Math.random() * d.entries.length) : 0]!, {shuffle});
  };
  return (
    <div style={{paddingBottom: 72}}>
      <header className={s.head}>
        {d?.backdrop ? <div className={s.backdrop} style={{backgroundImage: `url(${d.backdrop})`}} /> : null}
        <div className={s.headInner}>
          {d?.image && view === 'collection' ? <Artwork src={d.image} title={d.title} shape="poster" className={s.poster} eager /> : null}
          <div className={s.text}>
            <BackLink fallback={`/library/${libraryId}`} />
            <span className={s.eyebrow}>{LABEL[view] ?? 'Category'}</span>
            <h1 className={s.title}>{d?.title ?? id}</h1>
            <span className="c-2 t-caption">{d ? plural(d.entries.length, d.shape === 'square' ? 'album' : 'title') : ' '}</span>
            {d?.overview ? <p className={s.overview}>{d.overview}</p> : null}
            {d?.entries.length ? (
              <div className={s.buttons}>
                <Button variant="primary" icon="play" label="Play all" onClick={() => playAll(false)} />
                <Button variant="secondary" icon="shuffle" label="Shuffle" onClick={() => playAll(true)} />
              </div>
            ) : null}
          </div>
        </div>
      </header>
      {d && !d.entries.length ? <EmptyState icon="folder" title="Nothing here yet" body="No titles in this library match right now." /> : <CardGrid entries={d?.entries} loading={data.loading} shape={d?.shape ?? 'poster'} />}
    </div>
  );
}
