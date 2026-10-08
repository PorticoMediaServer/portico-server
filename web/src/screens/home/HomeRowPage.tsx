import {useEffect, useMemo, useSyncExternalStore} from 'react';
import {useNavigate, useParams} from '@tanstack/react-router';
import {HomeRowPager} from '@core/home-row-page.ts';
import {homeRowTitle} from '../../app/home';
import {useSession} from '../../app/session';
import {useLibrariesContext} from '../../app/libraries';
import {currentI18n} from '../../app/i18n';
import {ErrorNotice, ErrorState} from '../../app/errors';
import {useNotInterested} from '../../app/not-interested-notice';
import {RECOMMENDATIONS_RESET, recommendationViewer, withoutHidden} from '../../app/not-interested';
import {useViewerScope} from '../../app/viewer-scope';
import {Inset, Page, PageHeader} from '../../ui';
import {LoadingGrid, SectionView} from '../shared/Sections';

/**
 * A Home row's See all (Recommendations P7): Recommended, Trending now or one of the Picks for
 * you rows as a paged grid, read from the row's own endpoint (`HomeRowPager`: the whole row, a
 * stale cursor starts again from the first page). Its cards are recommendations, so they offer
 * Not interested and leave the grid when turned down.
 */
export function HomeRowPageScreen() {
  const {rowId} = useParams({from: '/app/home/rows/$rowId'});
  const navigate = useNavigate();
  const {api} = useSession();
  const viewer = recommendationViewer(useViewerScope());
  const libraries = useLibrariesContext();
  const libraryName = (id: string) => libraries.items.find(l => l.id === id)?.name;
  const pager = useMemo(() => new HomeRowPager(api, rowId), [api, rowId]);
  useEffect(() => { void pager.load(); return () => pager.dispose(); }, [pager]);
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
  return (
    <Page>
      <PageHeader title={title || ' '} onBack={back} backLabel={currentI18n().t('action.back')} />
      {state.phase === 'loading' && !state.entries.length ? <LoadingGrid density={row?.artworkShape === 'square' ? 'square' : 'poster'} /> : null}
      {state.phase === 'error' && !row ? <ErrorState error={state.error} context="home" retry={() => void pager.load()} refresh={() => void pager.load()} /> : null}
      {state.error && row ? <Inset><ErrorNotice error={state.error} context="home" retry={() => void pager.more()} refresh={() => void pager.load()} /></Inset> : null}
      {row ? (
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
