import React, {useCallback} from 'react';
import type {ContentEntry, ContentSection} from '@core/library-content.ts';
import {captionFor, continueTitle, formatDuration, iconFor, isContinueRow, progressFor, shapeFor} from '@core/presentation/index.ts';
import {cardCaptionOf, continueCaptionOf} from '../../app/content';
import {useEntryHref, useOpenEntry} from '../../app/open';
import {currentI18n} from '../../app/i18n';
import {accumulatePage, mergeSectionPages, SECTION_WINDOW_AT, type AppendedSections, type PageStore} from './section-pages';
import {selectableKind, useIsSelected, useSelectionActions, useSelectionActive, useSelectionSection} from '../../app/selection';
import type {MenuAnchor} from '../../ui';
import type {EntryOrigin} from '../../player/engine';
import type {ContentHeading} from '@core/library-content.ts';
import {serverTextLabel} from '@core/presentation/index.ts';

/**
 * A section's title. A heading whose key is a row title code (`home.row.*`: a Discover personal
 * row, a title page's More like X) is the catalogue message with its params, else its fallback;
 * other headings keep the server's words as before.
 */
export function sectionHeading(heading: ContentHeading): string {
  return heading.key.startsWith('home.row.') ? serverTextLabel(currentI18n(), heading, heading.fallback) : heading.fallback;
}
import {Button, Card, Grid, Icon, ListRow, Section, Shelf, Skeleton, Surface, Inset, WindowedGrid, createStaticCollection, gridColumnMin, type ShelfDensity, type GridDensity} from '../../ui';
import l from './EntryListRow.module.css';

function densityFor(kind: ContentEntry['kind']): ShelfDensity & GridDensity {
  const shape = shapeFor(kind);
  return shape === 'square' ? 'square' : shape === 'circle' ? 'person' : 'poster';
}

/** Renders one server-authored section as a shelf, grid or list. Never reorders. */
/** Rows for individual playable units (songs, audiobook files, chapters) play on activation; their detail page is reachable from the trailing action. */
function playsOnActivate(entry: ContentEntry): boolean {
  return entry.playback !== undefined && (entry.kind === 'song' || entry.kind === 'audiobook_file' || entry.kind === 'chapter');
}

/**
 * PERF-S02/S08: accumulate cursor pages in order (the web port of Apple's
 * `useAppendedPages`). Pure core lives in `./section-pages` (unit-tested
 * without React); this is the React binding.
 */
export function useAppendedPages<T>(scope: string, cursor: string | null, page: readonly T[], keyOf: (item: T) => string): readonly T[] {
  const store = React.useRef<PageStore<T>>({scope: '', pages: new Map()});
  const keyRef = React.useRef(keyOf);
  keyRef.current = keyOf;
  const stableKeyOf = useCallback((item: T) => keyRef.current(item), []);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return React.useMemo(() => accumulatePage(store.current, scope, cursor, page, stableKeyOf), [scope, cursor, page]);
}

export type {AppendedSections};

/** PERF-S02: section "More" appends inside its own section; other sections stay. */
export function useAppendedSections(routeKey: string, pageCursor: string | null, sections: readonly ContentSection[]): readonly ContentSection[] {
  const previous = React.useRef<AppendedSections | null>(null);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return React.useMemo(() => {
    const merged = mergeSectionPages(previous.current, routeKey, pageCursor, sections);
    previous.current = merged;
    return merged.sections;
  }, [routeKey, pageCursor, sections]);
}

export {SECTION_WINDOW_AT};

export function SectionView({section, onSeeAll, onMore, hideTitle, inShow, libraryId, noSelect, loadingMore, origin, alwaysSeeAll}: {/** See all even when the row shows everything it has (a saved view's page has more than its row). */ alwaysSeeAll?: boolean; section: ContentSection; onSeeAll?: () => void; onMore?: () => void; hideTitle?: boolean; inShow?: boolean; libraryId?: string; /** WEB-LIB-08: Home rows don't offer the selection checkbox. */ noSelect?: boolean; /** A "More" page is on its way; the button shows it instead of firing again. */ loadingMore?: boolean; /** Where the cards sit, for their actions menu (a recommendation row offers Not interested). */ origin?: EntryOrigin}) {
  const selecting = useSelectionActive();
  const actions = useSelectionActions();
  useSelectionSection(section.id, section.entries);
  const selectable = !!actions && selecting;
  // A stable getter, so appending a page doesn't re-render every card (they're memoised on their props).
  const latest = React.useRef(section.entries);
  latest.current = section.entries;
  const siblings = useCallback(() => latest.current, []);
  // PERF-S02: each cursor fires at most once — a second press (or a sentinel
  // still in view) with the same cursor is a no-op until the next page arrives.
  const firedCursor = React.useRef('');
  const fireMore = useCallback(() => {
    if (!onMore || !section.nextCursor) return;
    const key = `${section.id}:${section.nextCursor}`;
    if (firedCursor.current === key) return;
    firedCursor.current = key;
    onMore();
  }, [onMore, section.id, section.nextCursor]);
  if (!section.entries.length) return null;
  const first = section.entries[0]!;
  // A mixed row takes the poster density (its square art is fitted into poster frames).
  const density = section.entries.some(e => densityFor(e.kind) !== densityFor(first.kind)) && densityFor(first.kind) === 'square' ? 'poster' : densityFor(first.kind);
  // WEB-HOME-01: one geometry per row. Square art (albums) in a row of posters sits
  // inside a poster frame instead of a shorter square card.
  const rowShape = section.entries.every(e => shapeFor(e.kind, inShow ? 'showWorkspace' : 'shelf') === 'square') ? 'square' : undefined;
  const card = (entry: ContentEntry) => <EntryCard key={entry.id} entry={entry} siblings={siblings} inShow={inShow} libraryId={libraryId} selectable={selectable} noSelect={noSelect} rowShape={rowShape ?? (inShow ? undefined : 'poster')} origin={origin} resume={isContinueRow(section.id)} />;
  if (section.type === 'list') {
    return (
      <Section title={hideTitle ? undefined : sectionHeading(section.heading)}>
        <Inset>
          <Surface padless>
            {section.entries.length > SECTION_WINDOW_AT ? (
              <WindowedSectionList section={section} libraryId={libraryId} />
            ) : (
              section.entries.map(entry => (
                <EntryListRow key={entry.id} entry={entry} libraryId={libraryId} artShape={shapeFor(entry.kind, inShow ? 'showWorkspace' : 'shelf')} origin={origin} />
              ))
            )}
          </Surface>
          {/* PERF-S02: paging a list is the viewer's choice (one request per
              press), never an auto-firing sentinel. */}
          {onMore && section.nextCursor ? <ShowMoreButton loading={loadingMore} onMore={fireMore} /> : null}
        </Inset>
      </Section>
    );
  }
  if (section.type === 'grid') {
    return (
      <Section title={hideTitle ? undefined : sectionHeading(section.heading)} subtitle={!hideTitle && section.totalCount > section.entries.length ? `${section.entries.length} of ${section.totalCount}` : undefined}>
        {section.entries.length > SECTION_WINDOW_AT ? (
          <WindowedSectionGrid section={section} density={density} renderItem={card} />
        ) : (
          <Grid density={density}>{section.entries.map(card)}</Grid>
        )}
        {onMore && section.nextCursor ? <ShowMoreButton loading={loadingMore} onMore={fireMore} /> : null}
      </Section>
    );
  }
  return (
    <Shelf title={hideTitle ? undefined : sectionHeading(section.heading)} count={!onSeeAll && section.totalCount > section.entries.length ? section.totalCount : undefined} density={density} action={onSeeAll && (section.totalCount > section.entries.length || section.seeAll || alwaysSeeAll) ? {label: currentI18n().t('shelf.seeAll'), ariaLabel: currentI18n().t('shelf.seeAllCount', {count: section.totalCount}), onClick: onSeeAll} : undefined}>
      {section.entries.map(card)}
      {onMore && section.nextCursor ? <LoadMoreSentinel onMore={fireMore} /> : null}
    </Shelf>
  );
}

/** PERF-S02: an explicit "Show more" (one request per press, like Apple's grid button). */
function ShowMoreButton({loading, onMore}: {loading?: boolean; onMore: () => void}) {
  return (
    <div style={{display: 'flex', justifyContent: 'center', padding: '12px 0'}}>
      <Button variant="secondary" size="sm" label={currentI18n().t('action.showMore')} loading={loading} onClick={onMore} />
    </div>
  );
}

/** PERF-S02: a large appended grid section renders only its visible window. */
function WindowedSectionGrid({section, density, renderItem}: {section: ContentSection; density: GridDensity; renderItem: (entry: ContentEntry) => React.ReactNode}) {
  const store = React.useMemo(() => createStaticCollection<ContentEntry>({keyOf: e => e.id}), []);
  React.useEffect(() => { store.setItems(section.entries); }, [store, section.entries]);
  React.useEffect(() => () => store.collection.dispose(), [store]);
  const render = useCallback((entry: ContentEntry) => renderItem(entry), [renderItem]);
  return (
    <WindowedGrid
      collection={store.collection}
      columnMin={gridColumnMin(density)}
      estimateRowHeight={density === 'square' ? 240 : 300}
      label={sectionHeading(section.heading)}
      renderItem={render}
      renderPlaceholder={() => <div aria-hidden><Skeleton style={{aspectRatio: density === 'square' ? '1 / 1' : '2 / 3', width: '100%'}} /><Skeleton height={14} width="70%" style={{marginTop: 8}} /></div>}
    />
  );
}

/** PERF-S02: a large appended list section renders only its visible window. */
function WindowedSectionList({section, libraryId}: {section: ContentSection; libraryId?: string}) {
  const store = React.useMemo(() => createStaticCollection<ContentEntry>({keyOf: e => e.id}), []);
  React.useEffect(() => { store.setItems(section.entries); }, [store, section.entries]);
  React.useEffect(() => () => store.collection.dispose(), [store]);
  const render = useCallback((entry: ContentEntry) => <EntryListRow entry={entry} libraryId={libraryId} artShape={shapeFor(entry.kind, 'list')} />, [libraryId]);
  return (
    <WindowedGrid
      collection={store.collection}
      estimateRowHeight={64}
      label={sectionHeading(section.heading)}
      renderItem={render}
      renderPlaceholder={() => <div style={{height: 64, padding: '8px 16px'}}><Skeleton height={48} /></div>}
    />
  );
}

/**
 * Loads the next sideways page when the sentinel scrolls into view — once per
 * entry into view, so an idle page sends nothing and a scroll step sends one
 * request (the cursor guard in `SectionView` covers the rest).
 */
function LoadMoreSentinel({onMore}: {onMore: () => void}) {
  const ref = React.useRef<HTMLDivElement>(null);
  const latest = React.useRef(onMore);
  latest.current = onMore;
  const armed = React.useRef(true);
  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver(entries => {
      if (entries.some(e => e.isIntersecting)) {
        if (armed.current) { armed.current = false; latest.current(); }
      } else {
        armed.current = true;
      }
    }, {rootMargin: '400px'});
    io.observe(el);
    return () => io.disconnect();
  }, []);
  return <div ref={ref} style={{width: 1}} aria-hidden />;
}

/** WEB-LIB-04: one list row with the same identity and actions as a card (Play, More, watched state). */
export const EntryListRow = React.memo(function EntryListRow({entry, libraryId, artShape, origin}: {entry: ContentEntry; libraryId?: string; artShape?: ReturnType<typeof shapeFor>; origin?: EntryOrigin}) {
  const t = currentI18n().t;
  const open = useOpenEntry();
  const activatePlays = playsOnActivate(entry);
  const progress = progressFor(entry);
  const added = entry.addedAt && Number.isFinite(Date.parse(entry.addedAt)) ? new Date(entry.addedAt).toLocaleDateString(undefined, {year: 'numeric', month: 'short', day: 'numeric'}) : '';
  const watched = entry.watched === true && entry.kind !== 'category' && entry.kind !== 'collection';
  const anchor = (el: Element): MenuAnchor => { const r = el.getBoundingClientRect(); return {x: r.left, y: r.top, width: r.width, height: r.height}; };
  return (
    <ListRow
      className={l.row}
      title={entry.title}
      subtitle={cardCaptionOf(entry)}
      meta={
        <span className={l.meta}>
          <span className={l.duration}>{formatDuration(entry.duration)}</span>
          <span className={l.added} title={added ? t('card.addedOn', {date: added}) : undefined}>{added}</span>
          <span className={l.state}>{watched ? <Icon name="check" size={16} aria-label={t('card.watched')} /> : progress ? `${Math.round(progress * 100)}%` : null}</span>
        </span>
      }
      art={{path: entry.posterUrl ?? entry.backdropUrl, icon: iconFor(entry.kind), progress}}
      artShape={artShape ?? shapeFor(entry.kind, 'list')}
      onClick={() => open(entry, activatePlays ? 'play' : 'open', libraryId)}
      actions={
        <>
          {activatePlays && entry.navigation ? <Button variant="ghost" size="sm" icon="info" aria-label={t('card.details', {title: entry.title})} onClick={() => open(entry, 'open', libraryId)} />
            : entry.playback ? <Button variant="ghost" size="sm" icon="play" aria-label={t('card.play', {title: entry.title})} onClick={() => open(entry, 'play', libraryId)} /> : null}
          {entry.kind !== 'category' ? <Button variant="ghost" size="sm" icon="more" aria-label={t('card.more', {title: entry.title})} onClick={(e: React.MouseEvent<HTMLElement>) => open(entry, 'more', libraryId, anchor(e.currentTarget), origin)} /> : null}
        </>
      }
    />
  );
});

export function LoadingShelves({rows = 2, density = 'poster'}: {rows?: number; density?: ShelfDensity}) {
  return (
    <>
      {Array.from({length: rows}).map((_, r) => (
        <Shelf key={r} title=" " density={density}>
          {Array.from({length: 9}).map((_, i) => <Skeleton key={i} style={{aspectRatio: density === 'square' ? '1 / 1' : '2 / 3', width: '100%'}} />)}
        </Shelf>
      ))}
    </>
  );
}
export function LoadingGrid({density = 'poster'}: {density?: GridDensity}) {
  return <Grid density={density}>{Array.from({length: 18}).map((_, i) => <Skeleton key={i} style={{aspectRatio: density === 'square' ? '1 / 1' : '2 / 3', width: '100%'}} />)}</Grid>;
}

/**
 * One card with handlers that only change when the entry does, so the
 * memoised Card skips re-rendering when a neighbour is ticked. Subscribes to
 * the chosen set itself, in the smallest component that needs it.
 */
export const EntryCard = React.memo(function EntryCard({entry, siblings, inShow, libraryId, selectable, rowShape, noSelect, origin, resume}: {entry: ContentEntry; /** A Continue Watching card: the show as the title, what is left as the caption. */ resume?: boolean; noSelect?: boolean; origin?: EntryOrigin; /** The entries around this one (for shift-click range selection), read on demand. */ siblings: () => readonly ContentEntry[]; inShow?: boolean; libraryId?: string; selectable: boolean; /** The row's shared geometry, when the row mixes kinds. */ rowShape?: 'poster' | 'square'}) {
  const natural = shapeFor(entry.kind, inShow ? 'showWorkspace' : 'shelf');
  const shape = rowShape && natural !== 'landscape' ? rowShape : natural;
  const open = useOpenEntry();
  const hrefOf = useEntryHref();
  const actions = useSelectionActions();
  const selected = useIsSelected(entry.id);
  const onOpen = useCallback(() => open(entry, 'open', libraryId), [open, entry, libraryId]);
  const onPlay = useCallback(() => open(entry, 'play', libraryId), [open, entry, libraryId]);
  const onMore = useCallback((anchor: MenuAnchor) => open(entry, 'more', libraryId, anchor, origin), [open, entry, libraryId, origin]);
  const onSelect = useCallback(({range}: {range: boolean}) => {
    if (!actions) return;
    if (range) {
      const last = actions.entriesNow().at(-1);
      const entries = siblings();
      const a = last ? entries.findIndex(e => e.id === last.id) : -1;
      const b = entries.findIndex(e => e.id === entry.id);
      if (a >= 0 && b >= 0) { actions.selectMany(entries.slice(Math.min(a, b), Math.max(a, b) + 1)); return; }
    }
    actions.toggle(entry);
  }, [actions, siblings, entry]);
  return (
    <Card
      title={resume ? continueTitle(entry) : entry.title}
      caption={resume ? continueCaptionOf(entry) : cardCaptionOf(entry)}
      path={entry.posterUrl ?? entry.backdropUrl}
      shape={shape}
      fit={shape !== natural ? 'contain' : undefined}
      icon={iconFor(entry.kind)}
      progress={progressFor(entry)}
      watched={entry.watched === true && entry.kind !== 'category' && entry.kind !== 'collection'}
      unavailable={entry.available === false}
      onOpen={onOpen}
      href={hrefOf(entry, libraryId)}
      selectable={selectable}
      selected={selected}
      onSelect={actions && !noSelect && selectableKind(entry.kind) ? onSelect : undefined}
      onPlay={entry.playback ? onPlay : undefined}
      onMore={entry.kind !== 'category' ? onMore : undefined}
    />
  );
});
