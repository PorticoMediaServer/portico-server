import {useCallback, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentEntry} from '@core/library-content.ts';
import type {PersonalSavedIntent, PersonalResourceEntry} from '@core/personal-saved.ts';
import type {SavedIntent} from '@core/saved.ts';
import {historyPeriods} from '@core/saved.ts';
import {Artwork, CoverMosaic, type ArtworkSource} from '../../kit/artwork';
import {Button, Select, Switch, TextField} from '../../kit/controls';
import {Dialog, Menu, MenuRow, Popover} from '../../kit/overlays';
import {StateView} from '../../kit/states';
import {PosterCard, SquareCard} from '../../kit/cards';
import {homeCardModel} from '../home/model';
import type {SavedViewBindings} from './Saved';
import {SavedSection} from './Saved';
import {sameSavedRoute} from './route.ts';
import {playlistRemovalUndo, type PlaylistRemovalUndo} from './removal.ts';
import {RemovalUndo} from './RemovalUndo';
import styles from './saved.module.css';

const t = defaultI18n.t;
export function SavedResourceScreen({bindings}: {bindings: SavedViewBindings}) {
  const {runtime, route, effects} = bindings;
  const playlist = useSyncExternalStore(runtime.saved.subscribe, runtime.saved.getSnapshot, runtime.saved.getSnapshot), personal = useSyncExternalStore(runtime.personal.subscribe, runtime.personal.getSnapshot, runtime.personal.getSnapshot);
  const [menu, setMenu] = useState(false), [dialog, setDialog] = useState<'rename' | 'delete'>(), [name, setName] = useState(''), [summary, setSummary] = useState(''), [busy, setBusy] = useState(false);
  const [rowMenu, setRowMenu] = useState<Readonly<{entryId: string; media: ContentEntry}>>(), [undo, setUndo] = useState<PlaylistRemovalUndo>();
  const rowAnchor = useRef<HTMLElement>(null), pendingAction = useRef(false);
  const clearUndo = useCallback(() => setUndo(undefined), []);
  const cancel = useRef<HTMLButtonElement>(null), nameInput = useRef<HTMLInputElement>(null);
  const isPlaylist = route.view === 'playlist', resource = isPlaylist ? playlist.playlist : personal.resource;
  const routeMatches = sameSavedRoute(isPlaylist ? playlist.route : personal.route, route);
  const nameShown = isPlaylist ? playlist.playlist?.name : personal.resource?.name;
  const kind = isPlaylist ? 'playlist' : personal.resource?.kind === 'view' ? 'view' : 'collection';
  const actions = resource?.actions ?? [], pinned = resource?.pinned ?? false;
  const perform = (playlistIntent: SavedIntent, personalIntent: PersonalSavedIntent, done?: () => void) => {
    if (pendingAction.current || !bindings.operations.current) return;
    pendingAction.current = true; setBusy(true);
    void bindings.operations.run(async () => {
      if (isPlaylist) await runtime.saved.mutate(playlistIntent);
      else if (!await runtime.personal.mutate(personalIntent)) throw runtime.personal.getSnapshot().mutationError;
    }, () => { pendingAction.current = false; setBusy(false); done?.(); }).catch(error => { pendingAction.current = false; setBusy(false); effects.reportError(error); });
  };
  const failure = isPlaylist ? playlist.error : personal.error;
  if (!routeMatches) return null;
  if (!resource) return failure ? <StateView title={t('saved.listLoadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void runtime.select(route); }}} /> : null;
  const paths = isPlaylist ? playlist.projection?.sections.flatMap(section => section.entries).flatMap(entry => entry.kind === 'playlist_entry' && !entry.hidden ? entry.media.posterUrl ? [entry.media.posterUrl] : [] : []).slice(0, 4) ?? [] : personal.resource?.artworkPaths ?? [];
  const covers = paths.map(path => bindings.artwork.foreground(path, 120, 1)).filter((source): source is ArtworkSource => !!source);
  const renameTitle = t(kind === 'playlist' ? 'web.saved.renamePlaylist' : kind === 'view' ? 'web.saved.renameView' : 'web.saved.renameCollection');
  return <main className={styles.page}>
    <div className={styles.resourceBack}><Button variant="text" icon="i-left" size="small" onClick={() => effects.selectRoute({view: kind === 'playlist' ? 'playlists' : kind === 'view' ? 'views' : 'collections'})}>{t('saved.title')}</Button></div>
    <header className={styles.resourceHeader}><div className={styles.resourceCover}><CoverMosaic covers={covers} shape="square" alt="" /></div><div>
      <span className={styles.resourceKind}>{t(kind === 'playlist' ? 'web.saved.playlist' : kind === 'view' ? 'web.saved.savedView' : 'web.saved.collection')}</span>
      <h1>{nameShown}</h1>{resource.summary && <p>{resource.summary}</p>}<p>{t('web.saved.items', {count: resource.entryCount})}</p>
      <div className={styles.resourceActions}>
        {effects.playResource && <><Button variant="primary" icon="i-play" size="compact" onClick={() => { void bindings.operations.run(() => effects.playResource!({kind, id: resource.id, shuffle: false})).catch(effects.reportError); }}>{t('action.play')}</Button>
          <Button icon="i-shuffle" size="compact" onClick={() => { void bindings.operations.run(() => effects.playResource!({kind, id: resource.id, shuffle: true})).catch(effects.reportError); }}>{t('title.shuffle')}</Button></>}
        {actions.includes('pin') && <Button icon="i-bookmark" size="compact" disabled={busy} onClick={() => perform({action: 'pin', pinned: !pinned}, {action: 'pin', pinned: !pinned})}>{t(pinned ? 'pins.unpin' : 'pins.pin')}</Button>}
        <Popover open={menu} onOpenChange={setMenu} title={t('player.more')} role="menu" width={260} trigger={<Button variant="icon" icon="i-more" iconOnly aria-label={t('player.more')} />}>
          {(actions.includes('rename') || actions.includes('update')) && <MenuRow onClick={() => { setName(nameShown ?? ''); setSummary(resource.summary); setDialog('rename'); setMenu(false); }}>{t('web.saved.rename')}</MenuRow>}
          {actions.includes('delete') && <MenuRow destructive onClick={() => { setDialog('delete'); setMenu(false); }}>{t(kind === 'playlist' ? 'web.saved.deletePlaylist' : kind === 'view' ? 'web.saved.deleteView' : 'web.saved.deleteCollection')}</MenuRow>}
        </Popover>
      </div>
      {!isPlaylist && personal.resource?.kind === 'collection' && personal.resource.actions.includes('update') && <label className={styles.visibility}><span>{t('web.saved.visibleToAll')}</span><Switch checked={personal.resource.visibility === 'server'} disabled={busy} onCheckedChange={checked => perform({action: 'pin', pinned}, {action: 'update', visibility: checked ? 'server' : 'private'})} /></label>}
    </div></header>
    {isPlaylist ? playlist.projection?.sections.map(section => <SavedSection key={section.id} {...bindings} service={runtime.saved} section={section} heading={false} onMore={(entry, anchor) => {
      if (entry.kind !== 'playlist_entry' || entry.hidden || !bindings.operations.current) return;
      rowAnchor.current = anchor; setRowMenu({entryId: entry.id, media: entry.media});
    }} />)
      : <PersonalRows bindings={bindings} entries={personal.entries} onMore={(entry, anchor) => {
        if (!entry.media || entry.hidden || !bindings.operations.current) return;
        rowAnchor.current = anchor; setRowMenu({entryId: entry.id, media: entry.media});
      }} />}
    <Menu open={!!rowMenu} onOpenChange={open => { if (!open) setRowMenu(undefined); }} title={rowMenu?.media.title ?? t('player.more')} anchorRef={rowAnchor} width={290}
      items={rowMenu ? [{id: 'go', label: t('web.saved.goToItem'), icon: 'i-info', onSelect: () => { if (bindings.operations.current) effects.openEntry(rowMenu.media); }},
        ...((isPlaylist ? actions.includes('remove') : kind === 'collection' && actions.includes('entries')) ? [
          {id: 'remove-separator', separator: true} as const,
          {id: 'remove', label: <span className={styles.removeMenuLabel}><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" d="M6 12h12" /></svg>{t(isPlaylist ? 'web.saved.removeFromPlaylist' : 'web.saved.removeFromCollection')}</span>, destructive: true, disabled: busy, onSelect: () => {
            const removed = rowMenu;
            perform({action: 'remove', entryId: removed.entryId}, {action: 'entries', removeEntryIds: [removed.entryId]}, () => {
              if (isPlaylist) setUndo(playlistRemovalUndo(runtime.saved.getSnapshot().lastReceipt, {serverId: runtime.scope.serverId, viewerFence: resource.viewerFence, playlistId: resource.id, entryId: removed.entryId}, removed.media.title));
            });
          }},
        ] : [])] : []} />
    <RemovalUndo value={undo} clear={clearUndo} undo={value => {
      if (Date.now() >= value.until) return;
      perform({action: 'restore', undoToken: value.token}, {action: 'entries', undoToken: value.token}, clearUndo);
    }} />
    {!isPlaylist && personal.nextCursor && <Button size="small" className={styles.showMore} loading={personal.loading} onClick={() => { void runtime.personal.next(); }}>{t('action.showMore')}</Button>}
    <Dialog open={!!dialog} onOpenChange={open => { if (!open && !busy) setDialog(undefined); }} title={dialog === 'rename' ? renameTitle : t('web.saved.deleteTitle', {name: nameShown ?? ''})} initialFocusRef={dialog === 'rename' ? nameInput : cancel}
      description={dialog === 'delete' ? t(kind === 'playlist' ? 'web.saved.deletePlaylistBody' : kind === 'view' ? 'web.saved.deleteViewBody' : 'web.saved.deleteCollectionBody') : undefined}
      footer={<><Button ref={cancel} size="small" disabled={busy} onClick={() => setDialog(undefined)}>{t('action.cancel')}</Button><Button variant="primary" className={dialog === 'delete' ? styles.destructive : undefined} size="small" loading={busy} disabled={dialog === 'rename' && !name.trim()} onClick={() => {
        if (dialog === 'rename') perform({action: 'rename', name: name.trim(), ...(actions.includes('summary') ? {summary} : {})}, {action: 'update', name: name.trim(), summary}, () => setDialog(undefined));
        else perform({action: 'delete'}, {action: 'delete'}, () => effects.selectRoute({view: kind === 'playlist' ? 'playlists' : kind === 'view' ? 'views' : 'collections'}));
      }}>{t(dialog === 'rename' ? 'action.save' : kind === 'playlist' ? 'web.saved.deletePlaylistAction' : kind === 'view' ? 'web.saved.deleteViewAction' : 'web.saved.deleteCollectionAction')}</Button></>}>
      {dialog === 'rename' && <><TextField ref={nameInput} label={t('lib.saveViewName')} value={name} disabled={busy} onChange={event => setName(event.currentTarget.value)} /><TextField label={t('web.saved.summary')} value={summary} disabled={busy} onChange={event => setSummary(event.currentTarget.value)} /></>}
    </Dialog>
  </main>;
}

function PersonalRows({bindings, entries, onMore}: {bindings: SavedViewBindings; entries: readonly PersonalResourceEntry[]; onMore: (entry: PersonalResourceEntry, anchor: HTMLButtonElement) => void}) {
  return <div className={styles.personalRows}>{entries.map(entry => {
    const media = entry.media;
    if (!media || entry.hidden) return <div key={entry.id} className={styles.hiddenRow}>{t('card.unavailable')}</div>;
    const model = homeCardModel(media, false, {t, locale: defaultI18n.region.formatLocale}), square = ['album', 'song', 'book', 'book_part', 'artist'].includes(media.kind);
    const Card = square ? SquareCard : PosterCard;
    return <div key={entry.id} className={styles.mediaListRow}><Card model={model} artwork={bindings.artwork.foreground(media.posterUrl, 128, square ? 1 : 2 / 3)} context="detail" selection={bindings.selection.personalCard(bindings.runtime.personal, entry)}
      onNavigate={() => bindings.effects.openEntry(media)} onMore={anchor => onMore(entry, anchor)} onPlay={media.playback && media.available !== false && !media.missing ? () => { void bindings.operations.run(() => bindings.effects.playEntry(media)).catch(bindings.effects.reportError); } : undefined} /></div>;
  })}</div>;
}

export function SavedHistory({bindings}: {bindings: SavedViewBindings}) {
  const service = bindings.runtime.personal, snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot), [confirm, setConfirm] = useState(false), [busy, setBusy] = useState(false), cancel = useRef<HTMLButtonElement>(null);
  if (!sameSavedRoute(snapshot.route, bindings.route)) return null;
  const groups = new Map<string, PersonalResourceEntry[]>();
  for (const entry of snapshot.entries) {
    if (!entry.updatedAt) continue;
    const date = new Date(entry.updatedAt); if (!Number.isFinite(date.getTime())) continue;
    const key = date.toDateString(); let rows = groups.get(key); if (!rows) { rows = []; groups.set(key, rows); } rows.push(entry);
  }
  return <>
    <div className={styles.toolbar}><Select aria-label={t('web.saved.period')} value={bindings.route.view === 'history' ? bindings.route.period ?? '30d' : '30d'} options={historyPeriods.map(period => ({value: period, label: period === 'all' ? t('web.saved.allTime') : period === '24h' ? t('server.plays.period.24h') : t('web.saved.days', {count: Number.parseInt(period)})}))}
      onValueChange={period => bindings.effects.selectRoute({view: 'history', period: period as typeof historyPeriods[number]})} /><div className={styles.toolbarEnd}><Button variant="text" size="small" onClick={() => setConfirm(true)}>{t('web.saved.clearHistory')}</Button></div></div>
    {snapshot.error && !snapshot.entries.length && <StateView title={t('saved.historyLoadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void service.retryRead(); }}} />}
    {!snapshot.loading && !snapshot.error && !snapshot.entries.length && <StateView title={t('web.saved.empty.history')} body={t('web.saved.empty.historyBody')} />}
    {[...groups].map(([key, entries]) => <section className={styles.historyDay} key={key}><h2>{key === new Date().toDateString() ? t('saved.today') : defaultI18n.date(entries[0].updatedAt!, 'dayMonth')}</h2>
      {entries.map(entry => entry.media && <HistoryRow key={entry.id} bindings={bindings} entry={entry} media={entry.media} />)}
    </section>)}
    {snapshot.nextCursor && <Button className={styles.showMore} size="small" loading={snapshot.loading} onClick={() => { void service.next(); }}>{t('action.showMore')}</Button>}
    <Dialog open={confirm} onOpenChange={open => { if (!busy) setConfirm(open); }} title={t('web.saved.clearHistoryTitle')} description={t('web.saved.clearHistoryBody')} initialFocusRef={cancel}
      footer={<><Button ref={cancel} size="small" disabled={busy} onClick={() => setConfirm(false)}>{t('action.cancel')}</Button><Button size="small" className={styles.destructive} loading={busy} onClick={() => {
        if (busy || !bindings.operations.current) return;
        setBusy(true); void bindings.operations.run(async () => { if (!await service.mutate({action: 'clear-history'})) throw service.getSnapshot().mutationError; }, () => { setBusy(false); setConfirm(false); }).catch(error => { setBusy(false); bindings.effects.reportError(error); });
      }}>{t('web.saved.clearHistoryAction')}</Button></>}><span /></Dialog>
  </>;
}
function HistoryRow({bindings, entry, media}: {bindings: SavedViewBindings; entry: PersonalResourceEntry; media: ContentEntry}) {
  const position = entry.positionSeconds ?? 0, duration = media.duration, progress = duration && duration > 0 ? Math.min(1, position / duration) : undefined;
  return <div className={styles.historyRow}>
    <button className={styles.historyArt} onClick={() => bindings.effects.openEntry(media)} aria-label={media.title}><Artwork source={bindings.artwork.foreground(media.backdropUrl ?? media.posterUrl, 320, 16 / 9)} alt="" /></button>
    <div><b>{media.title}</b><small>{media.show?.name ?? media.artist?.name ?? media.author}</small></div>
    <div>{progress !== undefined && <span className={styles.progress} aria-hidden="true"><span style={{width: `${progress * 100}%`}} /></span>}<small>{entry.completed ? t('web.saved.finished') : defaultI18n.clock(position)}</small></div>
    <time dateTime={entry.updatedAt}>{entry.updatedAt && defaultI18n.time(entry.updatedAt)}</time>
  </div>;
}
