import {
  contextualMediaPlayAction,
  productMessage,
  reserveOrderedSurfaceSlots,
  resolveReservedSurfaceSlot,
  type ProductMessageId,
  type ProductMessagePresentation,
  type ProductMessageVariables,
  type ReservedSurfaceSlot,
} from '@porticomediaserver/client-core';
import { LibraryBig, Plus, RefreshCw, SlidersHorizontal } from '#portico-icons';
import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ProductLanguageIcon, productLanguageProblem } from '../../components/states/ProductLanguageState';
import { useAuthSession, useHome, useHomeRow, useLibraries, useMediaMutations, usePorticoDataSource } from '../../data/DataProvider';
import { canManageLibraries as viewerCanManageLibraries } from '../../data/authority';
import type { HomeRow, MediaItem } from '../../data/models';
import { useWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import { orderHomeRows } from '../../preferences/webDisplayPreferences';
import { markWebTiming, measureWebTiming } from '../../runtime/performance';
import { MediaActionMenu, MediaRail } from '../catalog/CatalogSurface';
import { mediaPresentation } from '../catalog/mediaPresentation';
import { targetFromMedia } from '../media/contextTarget';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { playbackOptionsForItems, watchNavigationState } from '../player/watchNavigation';
import { HomeCustomizationDialog } from './HomeCustomizationDialog';
import './home.css';

type HomeRowDescriptor = HomeRow;
type HomeArtworkShape = NonNullable<HomeRowDescriptor['artworkShape']>;

function homeRowArtworkShape(row: HomeRowDescriptor): HomeArtworkShape {
  if (row.artworkShape) return row.artworkShape;
  if (row.items[0]) return mediaPresentation(row.items[0]).artworkShape;
  if (row.type.toLocaleLowerCase().includes('square') || row.kind === 'music' || row.id === 'continue_listening') return 'square';
  if (row.type.toLocaleLowerCase().includes('landscape')) return 'landscape';
  return 'poster';
}

function homeRowFingerprint(row: HomeRowDescriptor | undefined): string {
  if (!row) return '';
  return JSON.stringify({
    id: row.id,
    title: row.title,
    type: row.type,
    endpoint: row.endpoint,
    hasMore: row.hasMore,
    nextCursor: row.nextCursor,
    items: row.items.map((item) => [item.id, item.title, item.poster, item.backdrop, item.progressSeconds]),
  });
}

function homeProblem(reason: unknown, fallback: ProductMessageId, variables: ProductMessageVariables = {}) {
  const presentation = productLanguageProblem(reason, fallback);
  return presentation.id === fallback ? productMessage(fallback, variables) : presentation;
}

function useNearViewport(eager: boolean) {
  const target = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(eager);
  useEffect(() => {
    if (active || eager) return;
    if (!('IntersectionObserver' in globalThis)) {
      setActive(true);
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setActive(true);
        observer.disconnect();
      }
    }, { rootMargin: '700px 0px' });
    if (target.current) observer.observe(target.current);
    return () => observer.disconnect();
  }, [active, eager]);
  return { target, active };
}

function HomeRowSkeleton({ title, busy = true, shape }: { title: string; busy?: boolean; shape?: HomeArtworkShape }) {
  return <section className={`portico-home-row portico-home-row-skeleton is-${shape ?? 'neutral'} ${busy ? '' : 'is-resolved-neutral'}`} aria-label={busy ? productMessage('home.row-loading', { rowTitle: title }).text : title} aria-busy={busy || undefined} data-artwork-shape={shape} data-reserved={busy ? 'unresolved' : 'resolved-empty'}>
    <span className="sr-only">{busy ? productMessage('home.row-loading', { rowTitle: title }).text : title}</span>
  </section>;
}

export function HomeRowSurface({ descriptor, eager, onResolved }: { descriptor: HomeRowDescriptor; eager: boolean; onResolved: (row: HomeRowDescriptor, resolution: 'ready' | 'empty' | 'failed') => void }) {
  const source = usePorticoDataSource();
  const { target, active } = useNearViewport(eager);
  const [row, setRow] = useState<HomeRowDescriptor>(descriptor);
  const [loadingMore, setLoadingMore] = useState(false);
  const [rowReloadKey, setRowReloadKey] = useState(0);
  const [error, setError] = useState<ProductMessagePresentation>();
  const generation = useRef(0);
  const continuation = useRef<AbortController | undefined>(undefined);
  const rowQuery = useHomeRow(descriptor.id, undefined, rowReloadKey, {
    // The Home manifest is the authoritative first page. A second request is
    // needed only for a descriptor whose preview was intentionally deferred.
    enabled: active && descriptor.items.length === 0,
    initialData: descriptor.items.length > 0 ? descriptor : undefined,
  });
  const rowQueryData = rowQuery.status === 'success' ? rowQuery.data : undefined;
  const descriptorFingerprint = homeRowFingerprint(descriptor);
  const rowQueryFingerprint = homeRowFingerprint(rowQueryData);
  const supersededQueryFingerprint = useRef('');

  useEffect(() => {
    generation.current += 1;
    continuation.current?.abort();
    supersededQueryFingerprint.current = rowQueryFingerprint && rowQueryFingerprint !== descriptorFingerprint
      ? rowQueryFingerprint
      : '';
    setRow((current) => current.id === descriptor.id && current.items.length > 0 && descriptor.items.length === 0
      ? { ...descriptor, ...current }
      : descriptor);
    setLoadingMore(false);
    setError(undefined);
    return () => continuation.current?.abort();
  }, [descriptorFingerprint]);

  useEffect(() => {
    if (!active || rowQuery.status === 'loading') return;
    if (rowQuery.status === 'success' && rowQueryData) {
			// A fresh Home manifest preview is newer than the first-page value
			// retained in the row cache. Ignore that one superseded snapshot; the
			// next network result (or the newly seeded preview) is authoritative.
      if (supersededQueryFingerprint.current === rowQueryFingerprint) {
        onResolved(descriptor, descriptor.items.length ? 'ready' : 'empty');
        return;
      }
      supersededQueryFingerprint.current = '';
      const next = { ...descriptor, ...rowQueryData };
      setRow(next);
      onResolved(next, next.items.length ? 'ready' : 'empty');
    } else {
      onResolved(descriptor, descriptor.items.length ? 'ready' : 'failed');
    }
  }, [active, descriptor, descriptorFingerprint, onResolved, rowQuery.status, rowQueryData, rowQueryFingerprint]);

  const loadMore = async () => {
    if (!row.nextCursor) return;
    continuation.current?.abort();
    const controller = new AbortController();
    continuation.current = controller;
    const activeGeneration = generation.current;
    const activeRowID = row.id;
    const activeCursor = row.nextCursor;
    setLoadingMore(true);
    setError(undefined);
    try {
      const nextPage = await source.homeRow(row.id, row.nextCursor, controller.signal);
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setRow((current) => {
        if (current.id !== activeRowID || current.nextCursor !== activeCursor) return current;
        const known = new Set(current.items.map((item) => item.id));
        const next = { ...current, ...nextPage, items: [...current.items, ...nextPage.items.filter((item) => !known.has(item.id))] };
        onResolved(next, next.items.length ? 'ready' : 'empty');
        return next;
      });
    } catch (reason) {
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setError(homeProblem(reason, 'home.row-partial', { rowTitle: row.title }));
    } finally {
      if (continuation.current === controller) continuation.current = undefined;
      if (!controller.signal.aborted && activeGeneration === generation.current) setLoadingMore(false);
    }
  };

  const artworkShape = homeRowArtworkShape(row);
  if (!active || (rowQuery.status === 'loading' && row.items.length === 0)) return <div ref={target}><HomeRowSkeleton title={descriptor.title} shape={homeRowArtworkShape(descriptor)} /></div>;
  if (rowQuery.status === 'error' && row.items.length === 0) {
    const failure = homeProblem(rowQuery.error, 'home.row-unavailable', { rowTitle: descriptor.title });
    return <div ref={target} data-home-slot-resolution="failed" className="portico-home-row"><div className="portico-home-row-error" role="status"><ProductLanguageIcon presentation={failure} /><span><strong>{failure.title}</strong><small>{failure.body}</small></span><SecondaryButton onClick={() => setRowReloadKey((value) => value + 1)}><RefreshCw /> {failure.actions[0]?.label}</SecondaryButton></div></div>;
  }
  // Empty server rows are policy inputs, not useful product surfaces. Keep
  // them available to customization and later invalidations without exposing
  // placeholder copy that makes a healthy Home page look broken.
  if (row.items.length === 0) return <div ref={target} data-home-slot-resolution="empty"><HomeRowSkeleton title={descriptor.title} shape={artworkShape} busy={false} /></div>;
  return <div ref={target} className="portico-home-row">
    {error && <div className="portico-home-row-warning" role="status"><ProductLanguageIcon presentation={error} /> {error.body}<button type="button" onClick={() => void loadMore()}>{error.actions[0]?.label}</button></div>}
    <MediaRail title={row.title} detail={row.explanation || row.detail} items={row.items} shape={artworkShape} playbackContext={{ type: 'queue', id: `home:${row.id}`, title: row.title }} hasMore={Boolean(row.hasMore && row.nextCursor)} continuationKey={row.nextCursor ?? undefined} loadingMore={loadingMore} onEndReached={() => void loadMore()} />
  </div>;
}

function HomeHero({ item, context, playbackOptions, showBackdrop }: { item?: MediaItem; context?: string; playbackOptions?: ReturnType<typeof playbackOptionsForItems>; showBackdrop: boolean }) {
  const navigate = useNavigate();
  const mediaActions = useMediaMutations();
  const auth = useAuthSession();
  const [saved, setSaved] = useState(item?.watchlisted ?? false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ProductMessagePresentation>();
  const [projectedActionIds, setProjectedActionIds] = useState<string[]>(item?.actions ?? []);
  const currentItemId = useRef(item?.id);
  const itemActionKey = (item?.actions ?? []).join('\u001f');
  const presentedActions = useMediaActionPresentations(projectedActionIds);
  const playAction = actionPresentation(presentedActions, 'play');
  const contextualPlayAction = item ? contextualMediaPlayAction(playAction, item) : undefined;
  const watchlistAction = saved
    ? actionPresentation(presentedActions, 'watchlist.remove', 'watchlist.update')
    : actionPresentation(presentedActions, 'watchlist.add', 'watchlist.update');
  useEffect(() => {
    currentItemId.current = item?.id;
    setSaved(item?.watchlisted ?? false);
    setSaving(false);
    setProjectedActionIds(item?.actions ?? []);
    setError(undefined);
  }, [item?.id, item?.watchlisted, itemActionKey]);
  if (!item) return <section className="portico-home-hero portico-home-hero-empty"><div><span>{productMessage('home.label').text}</span><h1>{auth.viewer?.serverName ?? 'Portico'}</h1><p>{productMessage('home.hero-placeholder').text}</p></div></section>;
  const toggleSaved = async () => {
    const itemId = item.id;
    const previous = saved;
    const next = !previous;
    setSaving(true);
    setError(undefined);
    try {
      const updated = await mediaActions.setWatchlist(itemId, next);
      if (currentItemId.current !== itemId) return;
      setSaved(updated.watchlisted ?? next);
      setProjectedActionIds(updated.actions ?? []);
    } catch (reason) {
      if (currentItemId.current !== itemId) return;
      setSaved(previous);
      setError(homeProblem(reason, 'home.saved-update-failed'));
    } finally {
      if (currentItemId.current === itemId) setSaving(false);
    }
  };
  return <section className="portico-home-hero" style={{ '--backdrop': showBackdrop && item.backdrop ? `url(${item.backdrop})` : 'none' } as CSSProperties}>
    <div className="portico-home-hero-copy">
      <span>{context || productMessage('home.continue-watching').text}</span>
      <h1>{item.title}</h1>
      <p className="portico-home-hero-meta">{[item.subtitle, item.year || undefined, item.length || undefined].filter(Boolean).join(' · ')}</p>
      {item.summary && <p className="portico-home-hero-summary">{item.summary}</p>}
      <div className="action-row">
        {contextualPlayAction && <PrimaryButton onClick={() => navigate(`/watch/${item.id}`, { state: watchNavigationState(playbackOptions) })}><MediaActionIcon action={contextualPlayAction} /> {contextualPlayAction.label}</PrimaryButton>}
        {watchlistAction && <SecondaryButton onClick={() => void toggleSaved()} disabled={saving}><MediaActionIcon action={watchlistAction} /> {watchlistAction.label}</SecondaryButton>}
        <MediaActionMenu target={targetFromMedia({ ...item, watchlisted: saved, actions: projectedActionIds })} playbackOptions={playbackOptions} />
      </div>
      {error && <p className="hero-action-error" role="alert">{error.body ?? error.title}</p>}
    </div>
  </section>;
}

function EmptyHomePage() {
  const auth = useAuthSession();
  const [reloadKey, setReloadKey] = useState(0);
  const libraries = useLibraries(reloadKey);
  const viewer = auth.viewer;
  const canManageLibraries = viewerCanManageLibraries(viewer?.user);

  if (libraries.status === 'loading') {
    const loading = productMessage('home.libraries-loading');
    return <div className="portico-home-page" aria-busy="true"><section className="portico-home-hero portico-home-hero-loading" aria-label={loading.title} /><div className="portico-home-content"><HomeRowSkeleton title={productMessage('home.label').text ?? ''} /></div></div>;
  }
  if (libraries.status === 'error') {
    const failure = homeProblem(libraries.error, 'home.libraries-unavailable');
    return <div className="standard-page"><div className="library-state error" role="alert"><ProductLanguageIcon presentation={failure} /><strong>{failure.title}</strong><p>{failure.body}</p><SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><RefreshCw /> {failure.actions[0]?.label}</SecondaryButton></div></div>;
  }

  const hasLibraries = libraries.data.length > 0;
  if (!hasLibraries && canManageLibraries) return <div className="portico-home-page"><section className="portico-home-hero portico-home-hero-empty portico-home-first-library"><div>
    <span>{productMessage('home.first-library-label').text}</span>
    <h1>{productMessage('home.first-library-title').text}</h1>
    <p>{productMessage('home.first-library-body').text}</p>
    <div className="action-row"><Link className="button primary" to="/settings/media?newLibrary=1"><Plus /> {productMessage('action.add-first-library').text}</Link></div>
  </div></section></div>;

  if (!hasLibraries) return <div className="portico-home-page"><section className="portico-home-hero portico-home-hero-empty portico-home-first-library"><div>
    <span>{productMessage('home.label').text}</span>
    <h1>{viewer?.serverName ?? 'Portico'}</h1>
    <p>{productMessage('home.no-shared-libraries').text}</p>
  </div></section></div>;

  const libraryCountMessage = productMessage(libraries.data.length === 1 ? 'home.library-ready-singular' : 'home.library-ready-plural', { count: libraries.data.length });
  return <div className="portico-home-page"><section className="portico-home-hero portico-home-hero-empty portico-home-first-library"><div>
    <span>{productMessage('home.label').text}</span>
    <h1>{productMessage('home.building-title').text}</h1>
    <p>{libraryCountMessage.text}</p>
    <div className="action-row"><Link className="button primary" to="/libraries"><LibraryBig /> {productMessage('action.open-libraries').text}</Link></div>
  </div></section></div>;
}

export function HomePage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [customizing, setCustomizing] = useState(false);
  const home = useHome(reloadKey);
  const display = useWebDisplayPreferences();
  const [rowSlots, setRowSlots] = useState<ReservedSurfaceSlot<HomeRowDescriptor, HomeRowDescriptor>[]>([]);
  const allRows = useMemo(() => home.status === 'success' ? home.data.rows as HomeRowDescriptor[] : [], [home]);
  const defaultHidden = display.preferences.homeRowOrder.length === 0 && display.preferences.hiddenHomeRows.length === 0
    ? allRows.filter((row) => row.defaultVisible === false && !row.required && (row.hideable === true || row.controls?.includes('hide') === true)).map((row) => row.id)
    : [];
  const hideableRows = new Set(allRows.filter((row) => !row.required && (row.hideable === true || row.controls?.includes('hide') === true)).map((row) => row.id));
  const hidden = new Set([...defaultHidden, ...display.preferences.hiddenHomeRows].filter((id) => hideableRows.has(id)));
  const rows = orderHomeRows(allRows, display.preferences.homeRowOrder).filter((row) => !hidden.has(row.id));
  const advertisedRows = useRef(rows);
  advertisedRows.current = rows;
  const orderedSlots = useMemo(() => reserveOrderedSurfaceSlots(rows, (row) => row.id, rowSlots), [rowSlots, rows]);
  const resolveRow = useCallback((row: HomeRowDescriptor, resolution: 'ready' | 'empty' | 'failed') => setRowSlots((current) => {
    const advertised = reserveOrderedSurfaceSlots(advertisedRows.current, (descriptor) => descriptor.id, current);
    return resolveReservedSurfaceSlot(advertised, row.id, resolution, resolution === 'ready' ? row : undefined);
  }), []);
  const resolved = orderedSlots.flatMap((slot) => slot.resolution === 'ready' && slot.data ? [slot.data] : slot.descriptor.items.length ? [slot.descriptor] : []);
  const heroRow = resolved.find((row) => row.id.includes('continue') && row.items.length) ?? resolved.find((row) => row.items.length);
  const hero = heroRow?.items[0];
  const heroPlaybackOptions = heroRow ? playbackOptionsForItems(heroRow.items, { type: 'queue', id: `home:${heroRow.id}`, title: heroRow.title }) : undefined;
  const failedRows = orderedSlots.filter((slot) => slot.resolution === 'failed');
  const degradedHome = productMessage('home.row-unavailable', { rowTitle: failedRows.length === 1 ? failedRows[0].descriptor.title : 'Some Home content' });

  useEffect(() => {
    if (home.status !== 'success') return;
    markWebTiming('home-useful-content');
    measureWebTiming('runtime-to-home-useful-content', 'first-frame', 'home-useful-content');
  }, [home.status]);

  if (home.status === 'loading') {
    const loading = productMessage('home.loading');
    const homeLabel = productMessage('home.label').text ?? '';
    return <div className="portico-home-page"><section className="portico-home-hero portico-home-hero-loading" aria-label={loading.title} aria-busy="true" /><div className="portico-home-content"><HomeRowSkeleton title={homeLabel} /></div></div>;
  }
  if (home.status === 'error') {
    const failure = homeProblem(home.error, 'home.unavailable');
    return <div className="standard-page"><div className="library-state error" role="alert"><ProductLanguageIcon presentation={failure} /><strong>{failure.title}</strong><p>{failure.body}</p><SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><RefreshCw /> {failure.actions[0]?.label}</SecondaryButton></div></div>;
  }
  if (allRows.length === 0) return <EmptyHomePage />;
  return <div className="portico-home-page">
    <HomeHero item={hero} context={heroRow?.title} playbackOptions={heroPlaybackOptions} showBackdrop={display.preferences.showBackdrops} />
    {display.status === 'error' && <div className="home-preference-warning" role="status"><ProductLanguageIcon presentation={productMessage('home.preferences-unavailable')} /><span>{productMessage('home.preferences-unavailable').body}</span><button type="button" onClick={display.retry}>{productMessage('home.preferences-unavailable').actions[0]?.label}</button></div>}
    <div className="portico-home-content">
      <div className="portico-home-toolbar"><SecondaryButton onClick={() => setCustomizing(true)}><SlidersHorizontal /> {productMessage('action.customize-home').text}</SecondaryButton></div>
      {failedRows.length > 0 && <div className="portico-home-degraded" role="status"><RefreshCw /><span>{degradedHome.title}</span><button type="button" onClick={() => { setRowSlots([]); setReloadKey((value) => value + 1); }}>{degradedHome.actions[0]?.label}</button></div>}
      {orderedSlots.map((slot, index) => <HomeRowSurface key={slot.id} descriptor={slot.descriptor} eager={index < 3} onResolved={resolveRow} />)}
    </div>
    {customizing && <HomeCustomizationDialog rows={allRows} preferences={display.preferences} busy={display.busy} onDismiss={() => setCustomizing(false)} onSave={display.update} />}
  </div>;
}
