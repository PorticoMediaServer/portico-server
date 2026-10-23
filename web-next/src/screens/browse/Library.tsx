import {useCallback, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';
import {browseNameId} from '@core/presentation/browse-toolbar.ts';
import {appliedFilterParts, browseToolbarModel, type TextEnvironment} from '@portico/client-core/kit-model';
import {LibraryHeader, LibrarySwitcher, StickyBrowseHeader, BrowseToolbar, FilteredEmpty, type LibraryChoice, type BrowseLabels} from '../../kit/browse';
import {Tabs} from '../../kit/controls';
import {AmbientArt} from '../../kit/artwork';
import {GridSkeleton, StateView} from '../../kit/states';
import type {ShellArtwork} from '../../shell/artwork';
import {useShellWidth} from '../../shell/viewport';
import {BrowseResults} from './BrowseResults';
import {BrowseFilters} from './Filters';
import {BrowseScopeChips} from './ScopeChips';
import type {BrowseController, BrowseMember} from './controller';
import type {BrowseItemActions} from './EntryCard';
import styles from './browse.module.css';

/** Live values are projected by the parent from the checked appearance facade. */
export type BrowsePresentation = Readonly<{view: 'grid' | 'list'; cardWidth: number; backdrops: boolean; cardSize: 'small' | 'medium' | 'large'}>;
export type LibraryBindings = Readonly<{
  controller: BrowseController;
  libraries: readonly LibraryChoice[];
  library: Readonly<{id: string; name: string; kind: string; latestArtwork?: string}>;
  pivot: string;
  tabs: readonly Readonly<{value: string; label: string}>[];
  appearance: BrowsePresentation;
  onAppearanceChange: (change: Partial<Pick<BrowsePresentation, 'view' | 'cardSize'>>) => void;
  onLibrary: (id: string) => void;
  onLibraries: () => void;
  onPivot: (pivot: string) => void;
  libraryMenu: ReactNode;
  /** Required real selection bar and per-entry capabilities from the selected viewer. */
  selection: Readonly<{toolbar?: ReactNode; actions: (member: BrowseMember, index: number) => BrowseItemActions; registerWindow: (first: number, last: number) => void}>;
  familyForEntry?: Parameters<typeof BrowseResults>[0]['familyForEntry'];
  onSaveView: () => void;
  canSaveView: boolean;
  showSaveView?: boolean;
  artwork: ShellArtwork;
  env: TextEnvironment;
  reducedMotion: boolean;
  owner: boolean;
  reportError: (error: unknown) => void;
  empty: ReactNode;
}>;
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
    {state.phase === 'ready' && state.capabilities && state.sort ? <LibraryBrowse {...bindings}/> : <GridSkeleton elapsedMs={300} window={{width, height: window.innerHeight, scrollTop: 0}} itemCount={21}/>}
  </div>;
}
function LibraryIdentity({library, libraries, pivot, tabs, onPivot, onLibrary, onLibraries, libraryMenu, appearance, artwork}: LibraryBindings) {
  const compact = useShellWidth() < 600;
  const switcher = <LibrarySwitcher libraries={libraries} value={library.id} onValueChange={onLibrary} compact={compact}/>;
  const navigation = <Tabs className={compact ? styles.phoneTabs : styles.headerTabs} value={pivot} onValueChange={onPivot} items={tabs} phone={compact}/>;
  const ambient = appearance.backdrops && library.latestArtwork ? <AmbientArt source={artwork.ambient(library.latestArtwork, {group: library.id, order: 0})} treatment="library" width="100%" height={560}/> : undefined;
  return compact ? <StickyBrowseHeader name={library.name} switcher={switcher} menu={libraryMenu} onLibraries={onLibraries} topInset={0}>{navigation}</StickyBrowseHeader> :
    <LibraryHeader name={library.name} switcher={switcher} menu={libraryMenu} ambient={ambient}>{navigation}</LibraryHeader>;
}
export function LibraryBrowse(bindings: LibraryBindings) {
  const {controller, appearance, library, env, selection, reportError} = bindings;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const snapshot = useSyncExternalStore(controller.collection.subscribe, controller.collection.getSnapshot, controller.collection.getSnapshot);
  const [filtersOpen, setFiltersOpen] = useState(false), width = useShellWidth(), compact = width < 600;
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
  const shape = bindings.pivot === 'collections' ? 'collection' : ['music', 'audiobook'].includes(library.kind) ? 'square' : 'poster';
  return <>
    <BrowseToolbar model={model} labels={browseLabels} platform={compact ? 'phone' : 'desktop'} resultCount={resultCount} topInset={compact ? 94 : 76} sticky
      onQuickChange={(id, checked) => {const quick = [...model.quick, ...model.filterQuick].find(quick => quick.id === id);if (quick) setPredicates(checked ? [...state.predicates, quick.query] : state.predicates.filter(node => JSON.stringify(node) !== JSON.stringify(quick.query)));}}
      onFilters={() => setFiltersOpen(true)} filtersOpen={filtersOpen} onSortChange={sort}
      onReshuffle={() => sort({...model.sort, seed: crypto.getRandomValues(new Uint32Array(1))[0]})}
      view={views.includes('grid') && views.includes('list') ? {value: view, onValueChange: view => bindings.onAppearanceChange({view})} : undefined}
      cardSize={!compact && view === 'grid' ? {value: appearance.cardSize, options: (['small', 'medium', 'large'] as const).map(value => ({value, label: t(`next.browse.${value}`)})), onValueChange: value => {if (value === 'small' || value === 'medium' || value === 'large') bindings.onAppearanceChange({cardSize: value});}} : undefined}
      actions={<BrowseScopeChips controller={controller} reportError={reportError}/>} save={bindings.showSaveView === false ? undefined : {canSave: bindings.canSaveView, onSave: bindings.onSaveView}} applied={chips}
      onRemoveFilter={id => setPredicates(state.predicates.filter(node => node !== model.applied[Number(id)]))} onClearFilters={() => setPredicates([])} selection={selection.toolbar}/>
    <BrowseResults controller={controller} label={library.name} view={view} cardWidth={appearance.cardWidth} shape={shape} artwork={bindings.artwork} env={env} backdrops={appearance.backdrops}
      reducedMotion={bindings.reducedMotion} actions={selection.actions} familyForEntry={bindings.familyForEntry} onVisibleRange={selection.registerWindow} onSort={sort}
      empty={state.predicates.length ? <FilteredEmpty onClear={() => setPredicates([])}/> : bindings.empty}/>
    <BrowseFilters controller={controller} model={model} labels={browseLabels} compact={compact} open={filtersOpen} resultCount={resultCount} canSaveView={bindings.canSaveView} onSaveView={bindings.onSaveView} onClose={() => setFiltersOpen(false)} reportError={bindings.reportError}/>
  </>;
}
