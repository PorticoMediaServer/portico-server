import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';
import type {Library} from '@core/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {browseNameId} from '@core/presentation/browse-toolbar.ts';
import {appliedFilterParts, browseToolbarModel, type FactPart, type TextEnvironment} from '@portico/client-core/kit-model';
import {LibraryHeader, LibrarySwitcher, StickyBrowseHeader, BrowseToolbar, type LibraryChoice, type BrowseLabels} from '../../kit/browse';
import {Tabs} from '../../kit/controls';
import {AmbientArt} from '../../kit/artwork';
import {StateView} from '../../kit/states';
import {Menu} from '../../kit/overlays';
import type {CardSelectionIntent} from '../../kit/cards';
import {LoadingGrid} from '../../kit/states/LoadingGrid';
import type {ShellArtwork} from '../../shell/artwork';
import {useShellWidth} from '../../shell/viewport';
import {BrowseResults} from './BrowseResults';
import {BrowseFilters} from './Filters';
import {BrowseFilteredEmpty} from './FilteredEmpty';
import {BrowseScopeChips} from './ScopeChips';
import type {BrowseController, BrowseMember} from './controller';
import type {BrowseItemActions} from './EntryCard';
import styles from './browse.module.css';
import {browsePresentation, type BrowsePresentation, type ScreenAppearance} from './appearance';
import type {LibraryPlaySelection} from './play-selection';

export type {BrowsePresentation} from './appearance';
export type LibraryBindings = Readonly<{
  controller: BrowseController;
  libraries: readonly LibraryChoice[];
  library: Readonly<{id: string; name: string; kind: string; latestArtwork?: string}>;
  pivot: string;
  tabs: readonly Readonly<{value: string; label: string}>[];
  appearance: BrowsePresentation;
  preferences?: ScreenAppearance;
  /** Only an actual per-library choice overrides the confirmed device default. */
  libraryViewOverride?: 'grid' | 'list';
  onAppearanceChange: (change: Partial<Pick<BrowsePresentation, 'view' | 'cardSize'>>) => void;
  onLibrary: (id: string) => void;
  onLibraries: () => void;
  onPivot?: (pivot: string) => void;
  libraryMenu: ReactNode;
  /** Required real selection bar and per-entry capabilities from the selected viewer. */
  selection: Readonly<{toolbar?: ReactNode; actions: (member: BrowseMember, index: number) => BrowseItemActions; registerWindow?: (first: number, last: number) => void; playSelection?: LibraryPlaySelection}>;
  familyForEntry?: Parameters<typeof BrowseResults>[0]['familyForEntry'];
  scopeLibraries?: readonly Library[];
  shape?: 'poster' | 'square' | 'collection';
  onSaveView?: () => void;
  /** A surface that names its own sort on the bar (a row's order reads "Best match"). */
  sortLabel?: string;
  /** The quick filters this surface draws; absent draws every one the server offers. */
  quickIds?: readonly string[];
  /** Names the scope chips from the viewer's own library list rather than the kind's generic name. */
  scopeLabel?: (kind: string) => string;
  canSaveView: boolean;
  showSaveView?: boolean;
  artwork: ShellArtwork;
  env: TextEnvironment;
  reducedMotion: boolean;
  owner: boolean;
  reportError: (error: unknown) => void;
  empty: ReactNode;
  /** Quiet owner guidance for the ordinary unmatched filter, never a badge or warning. */
  unmatchedHelp?: string;
}>;
export type LibraryBrowseBindings = Omit<LibraryBindings, 'libraries' | 'tabs' | 'onLibrary' | 'onLibraries' | 'onPivot' | 'libraryMenu'>;
const t = defaultI18n.t;
export const browseLabels: BrowseLabels = {
  field: value => {const id = browseNameId(value.id, value.labelKey, defaultI18n.has);return id && defaultI18n.has(id) ? t(id) : value.labelKey;},
  sort: value => {const id = browseNameId(value.id, value.labelKey, defaultI18n.has);return id && defaultI18n.has(id) ? t(id) : value.labelKey;},
  quick: value => defaultI18n.has(value.labelKey) ? t(value.labelKey) : value.labelKey,
};
export function LibraryScreen(bindings: LibraryBindings) {
  const {controller} = bindings;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const width = useShellWidth();
  useEffect(() => {void controller.load();}, [controller]);
  if (state.phase === 'error') return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: controller.retry}}/>;
  return <div className={styles.library}>
    <LibraryIdentity {...bindings}/>
    {state.phase === 'ready' && state.capabilities && state.sort ? <LibraryBrowse {...bindings}/> : <LoadingGrid key={bindings.library.id} width={width}/>}
    {state.phase === 'ready' && bindings.unmatchedHelp && state.predicates.some(node => 'field' in node && node.field === 'match' && node.operator === 'equals' && node.value === 'unmatched') ? <p className={styles.unmatchedHelp}>{bindings.unmatchedHelp}</p> : null}
  </div>;
}
export function LibraryIdentity({library, libraries, pivot, tabs, onPivot, onLibrary, onLibraries, libraryMenu, appearance: base, preferences, libraryViewOverride, artwork, wash, counts}: Pick<LibraryBindings,
  'library' | 'libraries' | 'pivot' | 'tabs' | 'onPivot' | 'onLibrary' | 'onLibraries' | 'libraryMenu' | 'appearance' | 'preferences' | 'libraryViewOverride' | 'artwork'> & Readonly<{
  /** A surface's own drawn wash, in place of the single latest-artwork ambient. */
  wash?: ReactNode;
  /** The drawn count line under the library name; absent draws none. */
  counts?: readonly FactPart[];
}>) {
  const appearance = browsePresentation(base, preferences, libraryViewOverride);
  const compact = useShellWidth() < 600;
  const switcher = <LibrarySwitcher libraries={libraries} value={library.id} onValueChange={onLibrary} compact={compact}/>;
  const navigation = tabs.length && onPivot ? <Tabs className={compact ? styles.phoneTabs : styles.headerTabs} value={pivot} onValueChange={onPivot} items={tabs} phone={compact}/> : undefined;
  const ambient = appearance.backdrops ? wash ?? (library.latestArtwork ? <AmbientArt source={artwork.ambient(library.latestArtwork, {group: library.id, order: 0})} treatment="library" width="100%" height={560}/> : undefined) : undefined;
  return compact ? <StickyBrowseHeader name={library.name} switcher={switcher} menu={libraryMenu} onLibraries={onLibraries} topInset={0}>{navigation}</StickyBrowseHeader> :
    <LibraryHeader name={library.name} counts={counts} switcher={switcher} menu={libraryMenu} ambient={ambient}>{navigation}</LibraryHeader>;
}
/** One card's menu, bound to the exact opening: the card, its entry, the
 * controller and the applied query that drew it. */
type LibraryCardMenu = Readonly<{anchor: HTMLElement; title: string; entry: ContentEntry; controller: BrowseController;
  queryRevision: number; play?: () => void; select?: () => void}>;
export function LibraryBrowse(bindings: LibraryBrowseBindings) {
  const {controller, library, env, selection, reportError} = bindings;
  const appearance = browsePresentation(bindings.appearance, bindings.preferences, bindings.libraryViewOverride);
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const snapshot = useSyncExternalStore(controller.collection.subscribe, controller.collection.getSnapshot, controller.collection.getSnapshot);
  const [filtersOpen, setFiltersOpen] = useState(false), width = useShellWidth(), compact = width < 600;
  // A card's own menu: its ⋯ button or a hold on an unselected card opens it at
  // that exact card, for the query it was opened under.
  const [cardMenu, setCardMenu] = useState<LibraryCardMenu>();
  const cardAnchor = useRef<HTMLElement | null>(null), cardMenuRef = useRef<LibraryCardMenu | undefined>(undefined);
  // The ref follows committed openings only, and its cleanup retires the exact
  // opening it held; a later menu is never cleared by an earlier one.
  useLayoutEffect(() => {
    cardMenuRef.current = cardMenu;
    return () => {if (cardMenuRef.current === cardMenu) cardMenuRef.current = undefined;};
  }, [cardMenu]);
  const queryRevision = state.queryRevision;
  const openCardMenu = useCallback((anchor: HTMLElement, base: BrowseItemActions, entry: ContentEntry, title: string) => {
    // A retained opener cannot draw a menu for a page the controller no longer serves.
    if (controller.getSnapshot().queryRevision !== queryRevision || !controller.contains(entry)) return;
    cardAnchor.current = anchor;
    setCardMenu({anchor, title, entry, controller, queryRevision,
      ...(base.onPlay ? {play: base.onPlay} : {}), ...(base.selection ? {select: () => base.selection!.onToggle({range: false, source: 'pointer'})} : {})});
  }, [controller, queryRevision]);
  /** The exact opening still owns the menu: same object, same controller, same applied query, same registered page. */
  const cardMenuCurrent = useCallback((opened: LibraryCardMenu) => cardMenuRef.current === opened && opened.controller === controller
    && controller.getSnapshot().queryRevision === opened.queryRevision && controller.contains(opened.entry), [controller]);
  const cardActions = useCallback((member: BrowseMember, index: number): BrowseItemActions => {
    const base = bindings.selection.actions(member, index), title = member.entry.displayTitle ?? member.entry.title, descriptor = base.selection;
    // No ⋯ without something it can really do, and an owner that already supplies
    // its own menu keeps it.
    const own = (base.onPlay || descriptor) ? (anchor: HTMLButtonElement) => openCardMenu(anchor, base, member.entry, title) : undefined;
    const more = base.onMore ?? own;
    return {...base, ...(more ? {onMore: more} : {}),
      ...(descriptor ? {selection: {...descriptor, onToggle: (intent: CardSelectionIntent) => {
        // A hold opens the menu only while this card is not selecting; once the
        // surface is selecting, the card keeps its drawn toggle gesture.
        if (intent.source === 'long-press' && intent.anchor && descriptor.active === false && (base.onMore || own)) {
          if (base.onMore) base.onMore(intent.anchor as HTMLButtonElement);
          else openCardMenu(intent.anchor, base, member.entry, title);
          return;
        }
        descriptor.onToggle(intent);
      }}} : {})};
  }, [bindings.selection, openCardMenu]);
  const model = useMemo(() => state.capabilities && state.sort ? browseToolbarModel(state.capabilities, library.kind, state.predicates, state.sort, {...env, owner: bindings.owner, scope: controller.source.base?.length ? 'group' : 'library'}) : undefined,
    [state.capabilities, state.sort, state.predicates, library.kind, env, bindings.owner, controller]);
  const sort = useCallback((sort: BrowseSortSelection) => {try {controller.setQuery(controller.getSnapshot().predicates, sort);} catch (error) {reportError(error);}}, [controller, reportError]);
  if (!model) return null;
  const pivot = state.capabilities?.resolvedPivot ?? state.capabilities?.pivots.find(pivot => pivot.id === controller.source.pivot);
  const views = (pivot?.supportedViews ?? []).filter((view): view is BrowsePresentation['view'] => view === 'grid' || view === 'list');
  const view = views.find(view => view === appearance.view) ?? views[0];
  if (!view) return null;
  const chips = model.applied.flatMap((node, index) => 'field' in node ? [{id: String(index), parts: appliedFilterParts(node, id => {const field = state.capabilities!.fields.find(field => field.id === id);return field ? browseLabels.field(field) : id;}, env)}] : []);
  const setPredicates = (nodes: readonly BrowseNode[]) => {try {controller.setQuery(nodes);} catch (error) {bindings.reportError(error);}};
  const resultCount = snapshot.total === undefined || snapshot.refreshing ? undefined : t(snapshot.totalIsLowerBound ? 'lib.countAtLeast' : 'lib.count', {count: snapshot.total, pivot: controller.source.pivot});
  const shape = bindings.shape ?? (bindings.pivot === 'collections' ? 'collection' : ['music', 'audiobook'].includes(library.kind) ? 'square' : 'poster');
  // A new applied query retires the menu at once; the stale state never draws again.
  const openMenu = cardMenu && cardMenu.controller === controller && cardMenu.queryRevision === state.queryRevision ? cardMenu : undefined;
  return <>
    <BrowseToolbar model={model} labels={browseLabels} platform={compact ? 'phone' : 'desktop'} resultCount={resultCount} topInset={compact ? 94 : 76} sticky
      onQuickChange={(id, checked) => {const quick = [...model.quick, ...model.filterQuick].find(quick => quick.id === id);if (quick) setPredicates(checked ? [...state.predicates, quick.query] : state.predicates.filter(node => JSON.stringify(node) !== JSON.stringify(quick.query)));}}
      onFilters={() => setFiltersOpen(true)} filtersOpen={filtersOpen} onSortChange={sort}
      onReshuffle={() => sort({...model.sort, seed: crypto.getRandomValues(new Uint32Array(1))[0]})}
      sortLabel={bindings.sortLabel} quickIds={bindings.quickIds}
      view={views.includes('grid') && views.includes('list') ? {value: view, onValueChange: view => bindings.onAppearanceChange({view})} : undefined}
      cardSize={!compact && view === 'grid' ? {value: appearance.cardSize, options: (['small', 'medium', 'large'] as const).map(value => ({value, label: t(`next.browse.${value}`)})), onValueChange: value => {if (value === 'small' || value === 'medium' || value === 'large') bindings.onAppearanceChange({cardSize: value});}} : undefined}
      actions={<BrowseScopeChips controller={controller} libraries={bindings.scopeLibraries} reportError={reportError} labelFor={bindings.scopeLabel}/>} save={bindings.showSaveView === false || !bindings.onSaveView ? undefined : {canSave: bindings.canSaveView, onSave: bindings.onSaveView}} applied={chips}
      onRemoveFilter={id => setPredicates(state.predicates.filter(node => node !== model.applied[Number(id)]))} onClearFilters={() => setPredicates([])} selection={selection.toolbar}/>
    <BrowseResults controller={controller} label={library.name} view={view} cardWidth={appearance.cardWidth} shape={shape} artwork={bindings.artwork} env={env} backdrops={appearance.backdrops}
      reducedMotion={bindings.preferences?.reducedMotion ?? bindings.reducedMotion} actions={cardActions} familyForEntry={bindings.familyForEntry} onVisibleRange={selection.registerWindow} onSort={sort}
      empty={state.predicates.length ? <BrowseFilteredEmpty controller={controller} model={model} labels={browseLabels} predicates={state.predicates}
        library={library.name} onDrop={setPredicates} onClear={() => setPredicates([])}/> : bindings.empty}/>
    <BrowseFilters controller={controller} model={model} labels={browseLabels} compact={compact} open={filtersOpen} resultCount={resultCount} canSaveView={bindings.canSaveView} onSaveView={bindings.onSaveView} onClose={() => setFiltersOpen(false)} reportError={bindings.reportError}/>
    <Menu open={!!openMenu} onOpenChange={open => {if (!open && openMenu && cardMenuRef.current === openMenu) setCardMenu(undefined);}} title={openMenu?.title ?? ''} anchorRef={cardAnchor} width={280}
      items={openMenu ? [
        ...(openMenu.play ? [{id: 'play', label: t('next.selection.play'), icon: 'i-play' as const, onSelect: () => {if (!cardMenuCurrent(openMenu)) return; setCardMenu(undefined); openMenu.play!();}}] : []),
        ...(openMenu.select ? [{id: 'select', label: t('next.selection.select'), icon: 'i-select' as const, onSelect: () => {if (!cardMenuCurrent(openMenu)) return; setCardMenu(undefined); openMenu.select!();}}] : []),
      ] : []}/>
  </>;
}
