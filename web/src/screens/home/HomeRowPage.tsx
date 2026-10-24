import {useCallback, useEffect, useMemo, useSyncExternalStore} from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {HomeRowPager} from '@core/home-row-page.ts';
import {homeRowTitle, retryHomeRow} from '../../app/home';
import {useSession} from '../../app/session';
import {useLibrariesContext} from '../../app/libraries';
import {ErrorNotice, ErrorState} from '../../app/errors';
import {useNotInterested} from '../../app/not-interested-notice';
import {RECOMMENDATIONS_RESET, recommendationViewer, withoutHidden} from '../../app/not-interested';
import {useViewerScope} from '../../app/viewer-scope';
import {useFreshDocument} from '../../app/freshness';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';

import {Button, Inset, Page, PageHeader} from '../../ui';
import {usePlayerActions} from '../../player/PlayerContext';
import {groupSequence, homeRowSequence} from '../../app/container-play';
import type {ContentEntry} from '@core/library-content.ts';
import {LoadingGrid, SectionView} from '../shared/Sections';
import {GroupBar, GroupGrid} from '../shared/GroupBrowse';
import {acrossGroup, useGroupBrowse, type GroupChoice} from '../../app/group-browse';
import {useI18n} from '../../app/i18n';

/**
 * A Home row's See all (Recommendations P7): Recommended, Trending now or one of the Picks for
 * you rows as a paged grid, read from the row's own endpoint (`HomeRowPager`: the whole row, a
 * stale cursor starts again from the first page). Its cards are recommendations, so they offer
 * Not interested and leave the grid when turned down.
 *
 * The library's sort and filters apply to the row's titles across libraries (`homeRow`): the row's
 * own order stands until a sort or filter is chosen, and stays the order under a filter.
 */
const rowDependencies = Object.freeze([dependsOn.libraries, dependsOn.list('recommendations'), ...titlesWithPersonalState()]);

export function HomeRowPageScreen() {
  const {t} = useI18n();
  const {rowId} = useParams({from: '/app/home/rows/$rowId'});
  const address = useSearch({from: '/app/home/rows/$rowId'});
  const navigate = useNavigate();
  const spec = useMemo(() => acrossGroup('homeRow', rowId, 'row'), [rowId]);
  const group = useGroupBrowse(spec, {sort: address.sort, direction: address.direction, filters: address.filters}, useCallback((next: GroupChoice) => void navigate({to: '/home/rows/$rowId', params: {rowId}, search: next, replace: true, resetScroll: false}), [navigate, rowId]));
  const {api} = useSession();
  const viewer = recommendationViewer(useViewerScope());
  const libraries = useLibrariesContext();
  const libraryName = (id: string) => libraries.items.find(l => l.id === id)?.name;
  const pager = useMemo(() => new HomeRowPager(api, rowId), [api, rowId]);
  useEffect(() => { void pager.load(); return () => pager.dispose(); }, [pager]);
  // The row's full list follows the libraries and this profile's state while it is open; pages already scrolled into stay.
  useFreshDocument(rowDependencies, context => pager.revalidate(context), () => pager.getSnapshot().phase === 'ready');
  useEffect(() => {
    const onReset = (event: Event) => { if ((event as CustomEvent<{viewer: string}>).detail.viewer === viewer) void pager.load(); };
    window.addEventListener(RECOMMENDATIONS_RESET, onReset);
    return () => window.removeEventListener(RECOMMENDATIONS_RESET, onReset);
  }, [viewer, pager]);
  const state = useSyncExternalStore(pager.subscribe, pager.getSnapshot);
  const notInterested = useNotInterested();
  const back = () => void navigate({to: '/'});
  const row = state.row;
  const title = row ? homeRowTitle(row.id, row.title, libraryName, row.titleText) : '';
  const entries = withoutHidden({entries: state.entries}, notInterested.hidden).entries;
  // Play and Shuffle play the whole row from the server: as it stands now in its own order, or as
  // sorted and filtered here. The first title shown is where Play starts.
  const player = usePlayerActions();
  const first = group.active ? firstPlayable(group.collection) : entries.find(e => e.playback);
  const play = (shuffle: boolean) => {
    if (!first) return;
    player.playSequence([first], 0, 0, group.active ? groupSequence(group, {shuffle}) : homeRowSequence(rowId, entries.map(e => e.kind), {shuffle}));
  };
  const actions = first ? <>
    <Button variant="primary" icon="play" label={t('action.play')} onClick={() => play(false)} />
    <Button variant="secondary" icon="shuffle" label={t('title.shuffle')} onClick={() => play(true)} />
  </> : undefined;
  return (
    <Page>
      <PageHeader title={title || ' '} onBack={back} backLabel={t('action.back')} actions={actions} />
      {state.phase === 'loading' && !state.entries.length ? <LoadingGrid density={row?.artworkShape === 'square' ? 'square' : 'poster'} /> : null}
      {state.phase === 'error' && !row ? <ErrorState error={state.error} context="home" retry={() => void pager.load()} refresh={() => void pager.load()} /> : null}
      {state.error && row ? <Inset><ErrorNotice error={state.error} context="home" retry={() => void retryHomeRow(pager, state.nextCursor)} refresh={() => void pager.load()} /></Inset> : null}
      {row && state.entries.length ? <Inset><GroupBar group={group} total={state.total} /></Inset> : null}
      {group.pending ? <LoadingGrid /> : group.active ? <GroupGrid group={group} square={row?.artworkShape === 'square'} /> : row ? (
        <SectionView
          section={{id: row.id, type: 'grid', heading: {key: row.id, fallback: title}, entries, totalCount: state.total, nextCursor: state.nextCursor}}
          libraryId={row.libraryId}
          hideTitle
          noSelect
          origin="recommendation"
          loadingMore={state.paging}
          onMore={state.nextCursor ? () => void pager.more() : undefined}
        />
      ) : null}
    </Page>
  );
}

/** The first playable title of a sorted or filtered row that has been read. */
function firstPlayable(list: {itemAt(index: number): ContentEntry | undefined} | undefined): ContentEntry | undefined {
  for (let i = 0; i < 50; i++) {
    const entry = list?.itemAt(i);
    if (!entry) return undefined;
    if (entry.playback) return entry;
  }
  return undefined;
}
