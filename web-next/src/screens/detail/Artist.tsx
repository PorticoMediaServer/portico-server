import {useEffect, useLayoutEffect, useState, useSyncExternalStore} from 'react';
import {t} from './text';
import type {FactPart} from '@core/presentation/kit-model';
import {LibraryContentService, type ContentEntry, type ContentProjection, type ContentSection} from '@core/library-content.ts';
import {dependsOn} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../platform/freshness';
import {AmbientArt, CircleArt} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {Lockup} from '../../kit/layout';
import {TitleFactsLine} from '../../kit/title/TitleParts';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {runScreenCommand, screenLifetime, type ScreenContext} from './context';
import {TitleRow} from './Detail';
import styles from './artist.module.css';

export type ArtistScreenProps = Readonly<{context: ScreenContext; libraryId: string; artistId: string; onBack?: () => void}>;

/** One page's own content lease: a fresh guarded service, admission signal and lifetime, owned by a
 * layout effect so a replayed setup never reuses a disposed one. */
type Lease = Readonly<{service: LibraryContentService; signal: AbortSignal; isCurrent: () => boolean; dispose: () => void}>;

/** desktop-listen's artist page on its canonical library route (`?view=artist&entityId=`): the
 * artist's circle and colours, the facts the artist view publishes, Play of the whole server-side
 * artist container, and the artist's albums with their admitted total and real paging. */
export function ArtistScreen({context, libraryId, artistId}: ArtistScreenProps) {
  const [lease, setLease] = useState<Lease>();
  useLayoutEffect(() => {
    const scope = screenLifetime(context), life = new AbortController();
    const service = new LibraryContentService({api: scope.api, scope: {serverId: context.scope.serverId, viewerId: context.scope.viewerId}});
    const next: Lease = Object.freeze({service, signal: life.signal,
      isCurrent: () => !life.signal.aborted && scope.current() && context.isCurrent(),
      dispose: () => { life.abort(); service.cancel(); service.dispose(); scope.dispose(); }});
    setLease(next);
    // Synchronous retirement: this lease ends here; a replayed setup builds a fresh one.
    return () => { next.dispose(); setLease(current => current === next ? undefined : current); };
  }, [context, libraryId, artistId]);
  if (!lease) return <p className={styles.loading} role="status">{t('music.loading')}</p>;
  return <ArtistBody context={context} libraryId={libraryId} artistId={artistId} lease={lease} />;
}

function ArtistBody({context, libraryId, artistId, lease}: {context: ScreenContext; libraryId: string; artistId: string; lease: Lease}) {
  const phone = useShellWidth() < 1024, {service} = lease;
  const content = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    setFailed(false);
    void service.select({libraryId, view: 'artist', entityId: artistId}).then(() => {
      if (!active || !lease.isCurrent()) return;
      const value = service.getSnapshot(), projection = value.projection;
      if (value.phase !== 'ready' || !projection || projection.entity?.id !== artistId || projection.scope.libraryId !== libraryId) setFailed(true);
    }, error => { if (active && lease.isCurrent()) { setFailed(true); context.effects.reportError(error); } });
    return () => { active = false; };
  }, [service, lease, libraryId, artistId, context]);
  // The admitted page re-reads when the library or its titles change; never a client-side ranking.
  useFreshDocument(() => [dependsOn.libraries], async refresh => { if (lease.isCurrent()) await service.revalidate(refresh); }, () => lease.isCurrent() && service.getSnapshot().phase === 'ready');
  const projection: ContentProjection | undefined = content.phase === 'ready' ? content.projection ?? undefined : undefined;
  const artist = projection?.entity;
  // Gate the drawn page on this exact admitted artist and view before anything is announced.
  const admitted = !!projection && !!artist && artist.kind === 'artist' && artist.id === artistId
    && projection.scope.libraryId === libraryId && projection.scope.view === 'artist' && projection.scope.entityId === artistId
    ? {catalog: projection.revision.catalog, viewer: projection.revision.viewer} : undefined;
  const section: ContentSection | undefined = projection && admitted ? (content.appended.find(value => value.id === 'items') ?? projection.sections.find(value => value.id === 'items')) : undefined;
  const albums: readonly ContentEntry[] = section?.entries ?? [];
  if (!projection || !artist || !admitted) return failed || content.phase === 'error' || content.phase === 'refresh-required'
    ? <StateView title={t('title.nothingHere')} />
    : <p className={styles.loading} role="status">{t('music.loading')}</p>;
  const playArtist = context.effects.playArtist;
  // An absent section count stays absent; a published zero is the only zero drawn.
  const total = section && typeof section.totalCount === 'number' && Number.isFinite(section.totalCount) ? section.totalCount : undefined;
  // The admitted page's own revision pair is the witness; a different one is another admission.
  const witness = () => {
    if (!lease.isCurrent() || !context.isCurrent()) return false;
    const live = service.getSnapshot();
    if (live.phase !== 'ready') return false;
    const page = live.projection;
    return !!page && page.entity?.kind === 'artist' && page.entity.id === artistId && page.scope.libraryId === libraryId
      && page.scope.view === 'artist' && page.scope.entityId === artistId
      && page.revision.catalog === admitted.catalog && page.revision.viewer === admitted.viewer;
  };
  const cover = artist.posterUrl, discArt = cover ? context.artwork.foreground(cover, 480, 1) : undefined;
  const facts: FactPart[] = [
    ...(artist.genres && artist.genres.length > 0 ? [{id: 'genres', role: 'context' as const, text: artist.genres.slice(0, 3).join(', ')}] : []),
    ...(total === undefined ? [] : [{id: 'albums', role: 'supplement' as const, text: t('lib.count', {count: total, pivot: 'albums'})}]),
  ];
  return <div className={styles.page} data-layout={phone ? 'phone' : 'desktop'}>
    <header className={styles.hero}>
      <span className={styles.disc}><AmbientArt className={styles.ambient} treatment="phone" source={context.artwork.ambient(cover)} foreground={discArt} />
        <CircleArt covers={discArt ? [discArt] : []} initials={artist.title.split(/\s+/).filter(Boolean).slice(0, 2).map(word => Array.from(word)[0]).join('')} alt="" /></span>
      <div className={styles.copy}>
        <p className={styles.kind}>{t('filter.field.artist')}</p>
        <Lockup title={artist.title} platform={phone ? 'phone-web' : 'desktop'} />
        <TitleFactsLine parts={facts} />
        {playArtist && <div className={styles.actions}>
          <Button variant="primary" icon="i-play" onClick={() => runScreenCommand(context, () => {
            if (!witness()) throw new DOMException('Artist playback changed', 'AbortError');
            return playArtist(artistId, lease.signal, witness);
          })}>{t('next.controls.play')}</Button>
          <Button variant="glass" icon="i-shuffle" onClick={() => runScreenCommand(context, () => {
            if (!witness()) throw new DOMException('Artist playback changed', 'AbortError');
            return playArtist(artistId, lease.signal, witness, true);
          })}>{t('title.shuffle')}</Button>
        </div>}
      </div>
    </header>
    {albums.length > 0 && <TitleRow heading={t('music.albums')} entries={albums} binding={{context}} />}
    {section?.nextCursor && <div className={styles.more}><Button size="small" onClick={() => runScreenCommand(context, () => {
      if (!witness()) throw new DOMException('Artist page changed', 'AbortError');
      return service.next('items');
    })}>{t('title.nextPage')}</Button></div>}
  </div>;
}
