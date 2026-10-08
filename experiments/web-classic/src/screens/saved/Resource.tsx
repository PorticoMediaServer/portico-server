import {useState} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import {useActions} from '@/app/actions';
import {usePlayer} from '@/player/PlayerProvider';
import {hrefFor} from '@/app/links';
import {formatDuration, formatTime, plural} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, EmptyState, ErrorState} from '@/ui/Bits';
import {ConfirmDialog, Menu, useToast} from '@/ui/Overlay';
import {Switch} from '@/ui/Form';
import {CardGrid} from '@/ui/Shelf';
import {BackLink} from '@/ui/Page';
import {useTitle} from '@/ui/hooks';
import {DetailLoading, useRouteParams} from '../common';
import {Mosaic, NameDialog} from './Saved';
import s from './Saved.module.css';

const NOUN = {playlist: 'playlist', collection: 'collection', view: 'saved view'} as const;

/** A playlist, personal collection or saved view: play, reorder, remove, rename, share and delete. */
export default function ResourceScreen() {
  const {kind, resourceId} = useRouteParams<{kind: 'playlist' | 'collection' | 'view'; resourceId: string}>();
  const q = useQuery(c => c.saved.resource(kind, resourceId), [kind, resourceId], ['saved', 'personal']);
  const client = useClient();
  const player = usePlayer();
  const actions = useActions();
  const toast = useToast();
  const navigate = useNavigate();
  const [rename, setRename] = useState(false);
  const [del, setDel] = useState(false);
  const [drag, setDrag] = useState<number>();
  useTitle(q.data?.title);
  if (q.error && !q.data) return <ErrorState title="This isn’t available" body="It may have been deleted, or it’s no longer shared with you." onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  const noun = NOUN[d.kind];
  const playable = d.items.filter(e => ['track', 'movie', 'episode', 'homevideo'].includes(e.kind));
  const list = d.kind === 'playlist';
  const play = (shuffle: boolean, from = 0) => { const q2 = shuffle ? [...playable].sort(() => Math.random() - 0.5) : playable; if (q2.length) player.play(q2, from, {mode: q2[0]!.kind === 'track' ? 'music' : 'video'}); };
  return (
    <div style={{paddingBottom: 72}}>
      <div style={{padding: '24px var(--gutter) 0'}}><BackLink fallback={`/saved?view=${d.kind === 'view' ? 'views' : `${d.kind}s`}`} label="Saved" /></div>
      <header className={s.head}>
        <Mosaic images={d.items.slice(0, 4).map(e => e.kind === 'episode' ? e.poster : e.image)} title={d.title} />
        <div className={s.headText}>
          <span className="t-label">{d.smart ? 'Smart view' : noun}{d.shared ? ' · Shared with this server' : ''}</span>
          <h1 className="t-display">{d.title}</h1>
          {d.description ? <p className="c-2">{d.description}</p> : null}
          <p className="t-caption c-3">{plural(d.items.length, d.items[0]?.kind === 'track' ? 'song' : 'item')}{d.totalDuration ? ` · ${formatDuration(d.totalDuration)}` : ''} · by {d.owner}</p>
          <div style={{display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 6}}>
            <Button variant="primary" icon="play" label="Play" disabled={!playable.length} onClick={() => play(false)} />
            <Button variant="secondary" icon="shuffle" label="Shuffle" disabled={!playable.length} onClick={() => play(true)} />
            {d.editable ? <Menu label="More" items={[
              {id: 'rename', label: d.kind === 'view' ? 'Rename…' : 'Rename and describe…', icon: 'edit', onSelect: () => setRename(true)},
              ...(d.kind === 'view' ? [{id: 'filters', label: 'Change filters', icon: 'filter' as const, onSelect: () => void navigate({to: '/library/lib-movies?view=browse' as never})}] : []),
              {id: 'share', label: d.shared ? 'Stop sharing' : 'Share with everyone on this server', icon: 'share', onSelect: async () => { await client.saved.setShared(d.id, !d.shared); toast.show(d.shared ? `${d.title} is private now` : `${d.title} is shared with everyone on this server`, {icon: 'share'}); }},
              {id: 'copy', label: 'Copy link', icon: 'link', onSelect: () => { void navigator.clipboard?.writeText(location.href).catch(() => {}); toast.show('Link copied', {icon: 'link'}); }},
              {id: 'delete', label: `Delete ${noun}…`, icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDel(true)},
            ]} trigger={<IconButton variant="secondary" icon="more" label="More" />} /> : <Badge>Shared by {d.owner}</Badge>}
          </div>
          {d.editable && d.kind !== 'view' ? <label style={{display: 'flex', alignItems: 'center', gap: 10, marginTop: 4}}><Switch checked={d.shared} onChange={async v => { await client.saved.setShared(d.id, v); }} label="Shared" /><span className="t-caption c-2">Everyone on this server can see it</span></label> : null}
        </div>
      </header>
      {!d.items.length ? (
        <EmptyState icon={d.kind === 'playlist' ? 'playlist' : 'collection'} title={`This ${noun} is empty`} body={d.kind === 'view' ? 'Nothing in the library matches these filters right now.' : 'Add titles with Add to playlist or Add to collection on any title’s menu.'} actions={[{label: 'Browse music', to: '/library/lib-music'}]} />
      ) : list ? (
        <div className={s.items} role="list" aria-label="Items">
          {d.items.map((e, i) => (
            <div key={e.id} role="listitem" className={`${s.item} ${drag === i ? s.dragging : ''}`} draggable={d.editable} onDragStart={() => setDrag(i)} onDragEnd={() => setDrag(undefined)} onDragOver={ev => { ev.preventDefault(); if (drag !== undefined && drag !== i) { void client.saved.moveInResource(d.id, d.items[drag]!.id, i - drag); setDrag(i); } }}>
              <span className={s.itemNum}>{i + 1}</span>
              <button type="button" onClick={() => play(false, playable.indexOf(e))} aria-label={`Play ${e.title}`}><Artwork src={e.kind === 'episode' ? e.poster : e.image} title={e.title} shape={e.shape === 'square' || e.kind === 'track' ? 'square' : 'poster'} tint={e.tint} /></button>
              <span style={{minWidth: 0}}>{hrefFor(e) && e.kind !== 'track' ? <Link to={hrefFor(e)! as never} className="t-strong clamp-1" style={{display: 'block'}}>{e.title}</Link> : <span className="t-strong clamp-1" style={{display: 'block'}}>{e.title}</span>}<span className="t-caption c-3 clamp-1" style={{display: 'block'}}>{e.subtitle}</span></span>
              <span className={`${s.itemTime} t-caption c-3 tnum`} style={{textAlign: 'right'}}>{e.duration ? formatTime(e.duration) : ''}</span>
              <span className={s.itemActions}>
                {d.editable ? <><IconButton size="sm" variant="ghost" icon="chevronUp" label={`Move ${e.title} up`} disabled={i === 0} onClick={() => void client.saved.moveInResource(d.id, e.id, -1)} /><IconButton size="sm" variant="ghost" icon="chevronDown" label={`Move ${e.title} down`} disabled={i === d.items.length - 1} onClick={() => void client.saved.moveInResource(d.id, e.id, 1)} /></> : null}
                <Menu label={`More for ${e.title}`} items={[...(d.editable ? [{id: 'remove', label: `Remove from ${d.title}`, icon: 'close' as const, onSelect: async () => { await client.saved.removeFromResource(d.id, e.id); toast.show(`Removed “${e.title}”`, {icon: 'check', action: {label: 'Undo', run: () => void client.saved.addToPlaylist(d.id, [e.id])}}); }}] : []), ...actions.items(e, 'playlist')]} trigger={<IconButton size="sm" variant="ghost" icon="more" label={`More for ${e.title}`} />} />
              </span>
            </div>
          ))}
        </div>
      ) : <CardGrid entries={d.items} />}
      <NameDialog open={rename} onOpenChange={setRename} title={`Rename ${noun}`} action="Save" initial={d.title} initialDescription={d.description ?? ''} onSave={async (name, description) => { await client.saved.renameResource(d.id, name, description); toast.show('Saved', {icon: 'check'}); }} />
      <ConfirmDialog open={del} onOpenChange={setDel} title={`Delete ${d.title}?`} body={`The ${noun} is deleted${d.shared ? ' for everyone on this server' : ''}. The titles in it stay in your libraries.`} action="Delete" tone="danger" onConfirm={async () => { await client.saved.deleteResource(d.id); toast.show(`Deleted ${d.title}`, {icon: 'trash'}); void navigate({to: `/saved?view=${d.kind === 'view' ? 'views' : `${d.kind}s`}` as never}); }} />
    </div>
  );
}
