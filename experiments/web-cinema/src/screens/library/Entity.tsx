import {useState} from 'react';
import {useNavigate, useParams} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Item} from '../../data/types';
import {player} from '../../app/player';
import {useSession} from '../../app/session';
import {Page, useTitle} from '../../ui/Layout';
import {Button, IconButton} from '../../ui/Button';
import {ConfirmDialog, Menu, PromptDialog} from '../../ui/Overlay';
import {ErrorState, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Facts, Stage, TitleSkeleton} from '../detail/parts';
import {MediaCard} from '../shared/Cards';
import {plural} from '../shared/items';
import {startPlayback} from '../shared/playback';

const KIND: Record<string, string> = {genre: 'Genre', mood: 'Mood', decade: 'Decade', studio: 'Studio', network: 'Network', collection: 'Collection', author: 'Author', series: 'Series'};

/** A genre, studio, network, decade, mood, collection, author or series: its titles as a grid. */
export function EntityScreen() {
  const {libraryId, view, entityId} = useParams({from: '/app/library/$libraryId/$view/$entityId'});
  const services = useServices();
  const navigate = useNavigate();
  const session = useSession();
  const q = useQuery(`entity.${libraryId}.${view}.${entityId}`, () => services.catalog.entity(libraryId, view, entityId));
  const items = useQuery(q.data ? `entity.items.${entityId}.${q.data.itemIds.length}` : null, () => services.catalog.items(q.data!.itemIds));
  const collections = useQuery(view === 'collection' ? 'collections.all' : null, () => services.catalog.collections());
  const col = collections.data?.find(c => c.id === entityId);
  const [renaming, setRenaming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  useTitle(q.data?.title);
  if (q.error) return <Page><ErrorState error={q.error} onRetry={q.reload} /></Page>;
  if (!q.data) return <Page flush><TitleSkeleton /></Page>;
  const list = items.data ?? [];
  const playable = list.filter((i: Item) => i.kind === 'movie' || i.kind === 'audiobook' || i.kind === 'album');
  const mine = col?.owner === 'me' || (col && session.manager);
  return (
    <Page flush className="title-page entity-page">
      <Stage art={q.data.art ?? list[0]?.art.poster ?? list[0]?.art.square} blur={!q.data.art} tint={q.data.tint} className="stage-short">
        <div className="eyebrow stage-eyebrow">{KIND[view] ?? 'Category'}{col?.shared ? ' · Shared with everyone' : ''}</div>
        <h1 className="display stage-title">{q.data.title}</h1>
        <Facts items={[view === 'collection' ? plural(q.data.itemIds.length, 'title') : q.data.subtitle.split(' · ')[1] ?? q.data.subtitle]} />
        {view === 'collection' && q.data.subtitle ? <p className="stage-syn"><span className="muted">{q.data.subtitle}</span></p> : null}
        <div className="action-bar">
          <div className="action-primary">
            {playable.length ? <Button variant="primary" size="lg" icon="play" label="Play" onClick={() => (playable[0]!.kind === 'movie' ? player.play(playable.map(i => i.id), {kind: 'video'}) : void startPlayback(services, playable[0]!))} /> : null}
            {playable.length > 1 ? <Button variant="glass" size="lg" icon="shuffle" label="Shuffle" onClick={() => player.play([...playable.map(i => i.id)].sort(() => Math.random() - 0.5), {kind: playable[0]!.kind === 'movie' ? 'video' : 'audio', shuffle: true})} /> : null}
          </div>
          {col && mine ? (
            <div className="action-secondary">
              <Menu label="Collection options" trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="Collection options"><Icon name="more" size={19} /></button>} items={[
                {id: 'rename', label: 'Rename…', icon: 'edit', onSelect: () => setRenaming(true)},
                {id: 'delete', label: 'Delete collection…', icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDeleting(true)},
              ]} />
            </div>
          ) : <IconButton icon="share" label="Copy link" onClick={() => { void navigator.clipboard?.writeText(location.href).catch(() => undefined); toast('Link copied'); }} />}
        </div>
      </Stage>
      <div className="page-pad">
        {list.length ? <div className="card-grid">{list.map(i => <MediaCard key={i.id} item={i} fill />)}</div> : items.loading ? null : <StateView icon="layers" title="Nothing here yet" body="Titles added to this appear here." />}
      </div>
      {col ? (
        <>
          <PromptDialog open={renaming} onOpenChange={setRenaming} title="Rename collection" label="Name" initial={col.title} confirmLabel="Rename" onConfirm={v => services.lists.renameCollection(col.id, v).then(() => { q.reload(); toast('Collection renamed'); })} />
          <ConfirmDialog open={deleting} onOpenChange={setDeleting} danger title={`Delete “${col.title}”?`} body="The titles stay in your library; only the collection goes." confirmLabel="Delete collection" onConfirm={() => services.lists.deleteCollection(col.id).then(() => { toast('Collection deleted'); void navigate({to: '/library/$libraryId', params: {libraryId}, search: {view: 'collections'}}); })} />
        </>
      ) : null}
    </Page>
  );
}
