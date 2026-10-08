import {useCallback, useEffect, useMemo, useState} from 'react';
import type {ContentSection} from '@core/library-content.ts';
import type {HomeRow} from '@core/home.ts';
import {formatDuration, homeHero, type HomeHero} from '@core/presentation/index.ts';
import {homeRowTitle, useHome} from '../../app/home';
import {useOpenEntry} from '../../app/open';
import {useSelectionRefresh} from '../../app/selection';
import {useSession} from '../../app/session';
import {Artwork, Backdrop, Button, Inset, Notice, Page, PageHeader, ProgressBar, StateView, cx} from '../../ui';
import {LoadingShelves, SectionView} from '../shared/Sections';
import {CustomiseHome} from './Customise';
import {NoLibraries} from '../shared/NoLibraries';
import {useLibrariesContext} from '../../app/libraries';
import s from './Home.module.css';
import {ErrorNotice, ErrorState} from '../../app/errors';
import {currentI18n} from '../../app/i18n';
import {useNavigate} from '@tanstack/react-router';
import {CONTINUE_REMOVED, type ContinueRemoval} from '../../app/continue-watching';
import {isRecommendationRow} from '@core/recommendation-feedback.ts';
import {homeViewResourceId} from '@core/home.ts';
import {useNotInterested} from '../../app/not-interested-notice';
import {withoutHidden} from '../../app/not-interested';

/**
 * Home: what the viewer was last in the middle of as a hero, then every row the
 * server composed for this viewer in the viewer's order (Continue Watching,
 * Recently added, Watchlist, Recommended…). The client never ranks; it only
 * pages rows and lets the viewer arrange them.
 */
export function HomeScreen() {
  const {state, refresh, more, saveLayout, resetLayout, loadLayout} = useHome();
  useSelectionRefresh(refresh);
  const {system, owner} = useSession();
  const libraries = useLibrariesContext();
  const libraryName = useCallback((id: string) => libraries.items.find(l => l.id === id)?.name, [libraries.items]);
  const open = useOpenEntry();
  const [customise, setCustomise] = useState(false);
  const document = state.document;
  const navigate = useNavigate();
  // WEB-HOME-03: a row from one library continues in that library (recent rows by date added).
  const seeAll = (row: HomeRow) => void navigate({to: '/library/$libraryId', params: {libraryId: row.libraryId!}, search: /recent|added/i.test(row.kind + row.id) ? {view: 'browse', sort: 'added', direction: 'desc'} : {}});
  // P7: a recommendation row with no library of its own (Recommended, Trending now, Picks for
  // you) opens as its own paged page.
  const seeAllRow = (row: HomeRow) => void navigate({to: '/home/rows/$rowId', params: {rowId: row.id}});
  // P6: a saved view's row opens the view's own page (the complete list, its filters and sort).
  const seeAllView = (resourceId: string, row: HomeRow) => void navigate({to: '/saved/$kind/$resourceId', params: {kind: 'view', resourceId}, search: {title: row.title}});
  // X-12: a title removed from Continue Watching leaves the row at once; Undo puts it back.
  const [removal, setRemoval] = useState<ContinueRemoval & {error?: string}>();
  const [hidden, setHidden] = useState<ReadonlySet<string>>(() => new Set());
  useEffect(() => {
    const on = (e: Event) => {
      const r = (e as CustomEvent<ContinueRemoval>).detail;
      setHidden(prev => { const next = new Set(prev); if (r.phase === 'failed') next.delete(r.itemId); else next.add(r.itemId); return next; });
      setRemoval(r.phase === 'failed' ? {...r, error: currentI18n().t('web.continue.failed', {title: r.title})} : r);
    };
    window.addEventListener(CONTINUE_REMOVED, on);
    return () => window.removeEventListener(CONTINUE_REMOVED, on);
  }, []);
  useEffect(() => {
    if (!removal || removal.phase === 'pending') return;
    const id = setTimeout(() => setRemoval(current => (current === removal ? undefined : current)), 10000);
    return () => clearTimeout(id);
  }, [removal]);
  // P5: a recommendation marked Not interested leaves its rows at once; Undo puts it back.
  const notInterested = useNotInterested();
  const undo = (r: ContinueRemoval) => {
    setRemoval(undefined);
    r.undo().then(() => { setHidden(prev => { const next = new Set(prev); next.delete(r.itemId); return next; }); refresh(); }, () => setRemoval({...r, phase: 'failed', error: currentI18n().t('web.continue.undoFailed', {title: r.title})}));
  };
  // The hero is the server's choice (the newer of the two Continue rows' first entries); a title
  // just removed from Continue Watching leaves the hero with the row.
  const hero = useMemo(() => homeHero(document, currentI18n().t, {duration: formatDuration, hidden}), [document, hidden]);
  // WEB-HOME-02: the eyebrow is the row's name, never the server name.
  const heroEyebrow = hero ? homeRowTitle(hero.row.id, hero.row.title, libraryName, hero.row.titleText) : undefined;
  const rows = useMemo(() => (document?.rows ?? []).map(r => (hidden.size && r.id === 'continue' ? {...r, entries: r.entries.filter(e => !hidden.has(e.id))} : isRecommendationRow(r) ? withoutHidden(r, notInterested.hidden) : r)).filter(r => r.entries.length), [document, hidden, notInterested.hidden]);
  return (
    <div style={{position: 'relative'}}>
      {hero ? <Backdrop path={hero.entry.backdropUrl ?? hero.entry.posterUrl} height="min(72vh, 760px)" opacity={hero.art === 'backdrop' ? 0.8 : 0.45} className={hero.art === 'cover' ? s.soft : undefined} /> : null}
      <Page>
        {hero ? (
          <Hero hero={hero} eyebrow={heroEyebrow} onPlay={() => open(hero.entry, 'play')} onDetails={() => open(hero.entry)} />
        ) : (
          <PageHeader eyebrow={system?.name ?? currentI18n().t('web.home.serverFallback')} title={currentI18n().t('web.shell.home')} />
        )}
        <div className={s.body}>
          {/* WEB-HOME-04: Customize sits where people look for it, above the rows. */}
          {document ? <Inset><div className={s.customize}><Button variant="ghost" size="sm" icon="sliders" label={currentI18n().t('home.customize.action')} onClick={() => setCustomise(true)} /></div></Inset> : null}
          {removal ? (
            <Inset>
              {removal.error ? <Notice tone="error" compact>{removal.error}</Notice> : (
                <Notice tone="info" compact action={{label: currentI18n().t('web.continue.undo'), onClick: () => undo(removal), loading: removal.phase === 'pending'}}>{currentI18n().t('web.continue.removed', {title: removal.title})}</Notice>
              )}
            </Inset>
          ) : null}
          {state.phase === 'loading' && !document ? <LoadingShelves /> : null}
          {state.phase === 'error' && !document ? <ErrorState error={state.error} context="home" retry={refresh} refresh={refresh} /> : null}
          {state.phase === 'error' && document ? <Inset><ErrorNotice error={state.error} context="home" retry={refresh} refresh={refresh} /></Inset> : null}
          {rows.map(row => <SectionView key={row.id} section={sectionOf(row, libraryName)} libraryId={row.libraryId} onMore={row.nextCursor && !state.paging.has(row.id) ? () => void more(row.id) : undefined} onSeeAll={homeViewResourceId(row.id) ? () => seeAllView(homeViewResourceId(row.id)!, row) : row.libraryId ? () => seeAll(row) : isRecommendationRow(row) ? () => seeAllRow(row) : undefined} alwaysSeeAll={!!homeViewResourceId(row.id)} noSelect origin={isRecommendationRow(row) ? 'recommendation' : undefined} />)}
          {document && !rows.length && !hero ? (!libraries.items.length && !libraries.loading ? <NoLibraries where="home" /> : <StateView icon="home" title={currentI18n().t('web.empty.homeTitle')} body={currentI18n().t(owner ? 'web.empty.homeOwnerBody' : 'web.empty.homeMemberBody')} />) : null}
        </div>
      </Page>
      {document ? <CustomiseHome open={customise} onClose={() => setCustomise(false)} onSave={saveLayout} onReset={resetLayout} loadLayout={loadLayout} /> : null}
    </div>
  );
}

function sectionOf(row: HomeRow, libraryName: (id: string) => string | undefined): ContentSection {
  return {id: row.id, type: 'rail', heading: {key: row.id, fallback: homeRowTitle(row.id, row.title, libraryName, row.titleText)}, entries: row.entries, totalCount: row.total, nextCursor: row.nextCursor};
}

/** Home's hero. Every word on it comes from `homeHero` (client-core), so each client says the same. */
export function Hero({hero, eyebrow, onPlay, onDetails}: {hero: HomeHero; eyebrow?: string; onPlay: () => void; onDetails: () => void}) {
  const {entry} = hero;
  return (
    <section className={cx(s.hero, hero.art === 'cover' && s.withCover)} aria-label={hero.title}>
      {hero.art === 'cover' && entry.posterUrl ? <Artwork path={entry.posterUrl} shape={hero.coverShape} className={cx(s.cover, hero.coverShape === 'square' && s.coverSquare)} /> : null}
      <div className={s.words}>
        {eyebrow ? <span className={s.eyebrow}>{eyebrow}</span> : null}
        <h1 className={s.title}>{hero.title}</h1>
        {hero.line ? <span className={s.line}>{hero.line}</span> : null}
        {hero.meta ? <span className={s.meta}>{hero.meta}</span> : null}
        {entry.overview ? <p className={s.overview}>{entry.overview}</p> : null}
        {hero.progress > 0 ? <div className={s.progress}><ProgressBar value={hero.progress} thin label={hero.title} /></div> : null}
        <div className={s.actions}>
          {entry.playback ? <Button variant="primary" size="lg" icon="play" label={hero.playLabel} onClick={onPlay} /> : null}
          <Button variant="secondary" size="lg" icon="info" label={currentI18n().t('web.home.details')} onClick={onDetails} />
        </div>
      </div>
    </section>
  );
}
