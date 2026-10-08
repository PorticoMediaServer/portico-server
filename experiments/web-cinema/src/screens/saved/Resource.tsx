import {useState} from 'react';
import {Link, useNavigate, useParams} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Item, TrackItem} from '../../data/types';
import {player, usePlayer} from '../../app/player';
import {Page, useTitle} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {ConfirmDialog, Menu, PromptDialog} from '../../ui/Overlay';
import {ErrorState, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {Facts, Stage, TitleSkeleton} from '../detail/parts';
import {TrackRow} from '../detail/Media';
import {MediaCard} from '../shared/Cards';
import {formatRuntime, hrefFor, plural, subtitleFor} from '../shared/items';
import {startPlayback} from '../shared/playback';
import {describeView} from '../library/Filters';

/** A playlist, collection or saved view: play it, reorder it, rename or delete it. */
export function ResourceScreen() {
  const {kind, resourceId} = useParams({from: '/app/saved/$kind/$resourceId'});
  if (kind === 'view') return <SavedViewPage id={resourceId} />;
  return <ListPage kind={kind === 'collection' ? 'collection' : 'playlist'} id={resourceId} />;
}

function ListPage({kind, id}: {kind: 'playlist' | 'collection'; id: string}) {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const q = useQuery(`resource.${kind}.${id}.${version}`, async () => (kind === 'playlist' ? services.lists.playlist(id) : (await services.catalog.collections()).find(c => c.id === id)));
  const list = q.data;
  const items = useQuery(list ? `resource.items.${id}.${list.itemIds.join(',')}` : null, () => services.catalog.items(list!.itemIds));
  const [renaming, setRenaming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const now = usePlayer(s => s.itemId);
  useTitle(list?.title);
  if (q.error) return <Page><ErrorState error={q.error} onRetry={q.reload} /></Page>;
  if (!list) return q.loading ? <Page flush><TitleSkeleton /></Page> : <Page><StateView icon="playlist" title={`This ${kind} was deleted`} action={{label: 'Back to Saved', to: '/saved'}} /></Page>;
  const all = items.data ?? [];
  const music = all.length > 0 && all.every(i => i.kind === 'track');
  const duration = all.reduce((a, i) => a + ('duration' in i ? (i as TrackItem).duration : 'runtime' in i ? (i as {runtime: number}).runtime * 60 : 0), 0);
  const editable = kind === 'playlist' || ('owner' in list && list.owner === 'me');
  const lead = all.find(i => i.art.backdrop) ?? all[0];
  const remove = (index: number, itemId: string, title: string) => {
    const p = kind === 'playlist' ? services.lists.removeFromPlaylist(id, index) : services.lists.removeFromCollection(id, itemId);
    void p.then(() => toast(`Removed “${title}”`, {action: {label: 'Undo', onClick: () => void (kind === 'playlist' ? services.lists.addToPlaylist(id, [itemId]) : services.lists.addToCollection(id, [itemId]))}}));
  };
  return (
    <Page flush className="title-page resource-page">
      <Stage art={lead?.art.backdrop ?? lead?.art.square} tint={list.tint} blur={music} className="stage-short">
        <div className="eyebrow stage-eyebrow">{kind === 'playlist' ? (music ? 'Music playlist' : 'Playlist') : 'Collection'}{list.shared ? ' · Shared with everyone' : ' · Only you'}</div>
        <h1 className="display stage-title">{list.title}</h1>
        <Facts items={[plural(list.itemIds.length, music ? 'song' : 'item'), duration ? formatRuntime(duration / 60) : null, 'updatedAt' in list ? `Updated ${new Date(list.updatedAt).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}` : null]} />
        {list.description ? <p className="stage-syn"><span className="muted">{list.description}</span></p> : null}
        <div className="action-bar">
          <div className="action-primary">
            <Button variant="primary" size="lg" icon="play" label="Play" disabled={!all.length} onClick={() => (music ? player.play(list.itemIds, {kind: 'audio'}) : all[0] && (all[0].kind === 'movie' || all[0].kind === 'episode' ? player.play(list.itemIds, {kind: 'video'}) : void startPlayback(services, all[0])))} />
            <Button variant="glass" size="lg" icon="shuffle" label="Shuffle" disabled={all.length < 2} onClick={() => player.play([...list.itemIds].sort(() => Math.random() - 0.5), {kind: music ? 'audio' : 'video', shuffle: true})} />
          </div>
          {editable ? (
            <div className="action-secondary">
              <Menu label={`${kind} options`} trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label={`${kind === 'playlist' ? 'Playlist' : 'Collection'} options`}><Icon name="more" size={19} /></button>} items={[
                {id: 'rename', label: 'Rename…', icon: 'edit', onSelect: () => setRenaming(true)},
                {id: 'share', label: list.shared ? 'Stop sharing' : 'Share with everyone', icon: 'people', onSelect: () => toast(list.shared ? 'Only you can see it now' : 'Everyone on this server can see it now')},
                {id: 'delete', label: `Delete ${kind}…`, icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDeleting(true)},
              ]} />
            </div>
          ) : null}
        </div>
      </Stage>
      <div className="page-pad">
        {!all.length && !items.loading ? <StateView icon={kind === 'playlist' ? 'playlist' : 'layers'} title={`This ${kind} is empty`} body="Add to it from any title’s More menu, or with Add to playlist in Now Playing." /> : music ? (
          <div className="tracklist" role="table" aria-label={list.title}>
            {(all as TrackItem[]).map((t, i) => (
              <TrackRow key={t.id + i} track={t} index={i + 1} showArt showAlbum playing={now === t.id} onPlay={() => player.play(list.itemIds, {kind: 'audio', index: i})} trailing={editable ? <ReorderTools index={i} count={all.length} onMove={(from, to) => void services.lists.movePlaylistItem(id, from, to)} onRemove={() => remove(i, t.id, t.title)} title={t.title} /> : undefined} />
            ))}
          </div>
        ) : kind === 'playlist' ? (
          <ol className="res-list">
            {all.map((it: Item, i) => (
              <li key={it.id + i} className="res-row">
                <span className="res-n tabular">{i + 1}</span>
                <Link to={hrefFor(it as never) as never} className="res-main">
                  <Art src={it.art.still ?? it.art.backdrop ?? it.art.poster} tint={it.tint} ratio="16/9" className="res-art" />
                  <span className="row-text"><span className="row-label">{it.title}</span><span className="row-help">{subtitleFor(it)}</span></span>
                </Link>
                {editable ? <ReorderTools index={i} count={all.length} onMove={(from, to) => void services.lists.movePlaylistItem(id, from, to)} onRemove={() => remove(i, it.id, it.title)} title={it.title} /> : null}
              </li>
            ))}
          </ol>
        ) : (
          <div className="card-grid">{all.map(i => <MediaCard key={i.id} item={i} fill />)}</div>
        )}
      </div>
      <PromptDialog open={renaming} onOpenChange={setRenaming} title={`Rename ${kind}`} label="Name" initial={list.title} confirmLabel="Rename" onConfirm={v => (kind === 'playlist' ? services.lists.renamePlaylist(id, v) : services.lists.renameCollection(id, v)).then(() => toast('Renamed'))} />
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} danger title={`Delete “${list.title}”?`} body={`The ${kind} goes; the ${music ? 'songs' : 'titles'} in it stay in your library.`} confirmLabel={`Delete ${kind}`} onConfirm={() => (kind === 'playlist' ? services.lists.deletePlaylist(id) : services.lists.deleteCollection(id)).then(() => { toast(`Deleted “${list.title}”`); void navigate({to: '/saved', search: {view: kind === 'playlist' ? 'playlists' : 'collections'}}); })} />
    </Page>
  );
}

function ReorderTools({index, count, onMove, onRemove, title}: {index: number; count: number; onMove: (from: number, to: number) => void; onRemove: () => void; title: string}) {
  return (
    <span className="reorder">
      <button type="button" className="track-more is-shown" onClick={() => onMove(index, index - 1)} disabled={index === 0} aria-label={`Move ${title} up`}><Icon name="chevronUp" size={16} /></button>
      <button type="button" className="track-more is-shown" onClick={() => onMove(index, index + 1)} disabled={index === count - 1} aria-label={`Move ${title} down`}><Icon name="chevronDown" size={16} /></button>
      <button type="button" className="track-more is-shown" onClick={onRemove} aria-label={`Remove ${title}`}><Icon name="close" size={16} /></button>
    </span>
  );
}

function SavedViewPage({id}: {id: string}) {
  const services = useServices();
  const navigate = useNavigate();
  const views = useQuery('lists.views', () => services.lists.savedViews());
  const view = views.data?.find(v => v.id === id);
  const results = useQuery(view ? `view.results.${id}` : null, () => services.catalog.browse({libraryId: view!.libraryId, filters: view!.filters, sort: view!.sort, pageSize: 120}));
  const [renaming, setRenaming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  useTitle(view?.title);
  if (!view) return views.loading ? <Page flush><TitleSkeleton /></Page> : <Page><StateView icon="save" title="This saved view was deleted" action={{label: 'Back to Saved', to: '/saved'}} /></Page>;
  const lead = results.data?.items.find(i => i.art.backdrop);
  return (
    <Page flush className="title-page">
      <Stage art={lead?.art.backdrop} tint={lead?.tint ?? '#409fd4'} className="stage-short">
        <div className="eyebrow stage-eyebrow">Saved view{view.shared ? ' · Shared' : ''}{view.onHome ? ' · On Home' : ''}</div>
        <h1 className="display stage-title">{view.title}</h1>
        <Facts items={[results.data ? plural(results.data.total, 'title') : 'Counting…', describeView(view).filters, describeView(view).sort]} />
        <div className="action-bar">
          <div className="action-primary">
            <Button variant="primary" size="lg" icon="sliders" label="Open in library" onClick={() => void navigate({to: '/library/$libraryId', params: {libraryId: view.libraryId}, search: {view: 'browse', filters: JSON.stringify(view.filters), sort: view.sort.field, direction: view.sort.direction}})} />
          </div>
          <div className="action-secondary">
            <Menu label="Saved view options" trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="Saved view options"><Icon name="more" size={19} /></button>} items={[
              {id: 'rename', label: 'Rename…', icon: 'edit', onSelect: () => setRenaming(true)},
              {id: 'home', label: view.onHome ? 'Remove from Home' : 'Show on Home', icon: 'home', onSelect: () => toast(view.onHome ? 'Removed from Home' : 'Added to Home')},
              {id: 'delete', label: 'Delete view…', icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDeleting(true)},
            ]} />
          </div>
        </div>
      </Stage>
      <div className="page-pad">
        {results.data && !results.data.items.length ? <StateView icon="filter" title="Nothing matches this view right now" body="As titles are added or watched, they’ll show up here." /> : <div className="card-grid">{(results.data?.items ?? []).map(i => <MediaCard key={i.id} item={i} fill />)}</div>}
      </div>
      <PromptDialog open={renaming} onOpenChange={setRenaming} title="Rename saved view" label="Name" initial={view.title} confirmLabel="Rename" onConfirm={v => services.lists.renameView(id, v).then(() => { views.reload(); toast('Renamed'); })} />
      <ConfirmDialog open={deleting} onOpenChange={setDeleting} danger title={`Delete “${view.title}”?`} body="The view goes; nothing in your library changes." confirmLabel="Delete view" onConfirm={() => services.lists.deleteView(id).then(() => { toast('Saved view deleted'); void navigate({to: '/saved', search: {view: 'views'}}); })} />
    </Page>
  );
}

