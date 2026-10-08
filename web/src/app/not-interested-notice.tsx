import {useEffect, useMemo, useSyncExternalStore} from 'react';
import {Inset, Notice} from '../ui';
import {currentI18n} from './i18n';
import {useViewerScope} from './viewer-scope';
import {currentNotInterestedNotice, dismissNotInterested, hiddenNotInterested, notInterestedVersion, recommendationViewer, subscribeNotInterested, undoNotInterested} from './not-interested';

/** All mounted recommendation screens read the same viewer-scoped decisions. */
export function useNotInterested(): {hidden: ReadonlySet<string>} {
  const viewer = recommendationViewer(useViewerScope());
  const version = useSyncExternalStore(subscribeNotInterested, notInterestedVersion);
  const hidden = useMemo(() => hiddenNotInterested(viewer), [viewer, version]);
  return {hidden};
}

/** Rendered once in Shell, so two mounted screens cannot offer duplicate Undo actions. */
export function NotInterestedNotice() {
  const viewer = recommendationViewer(useViewerScope());
  useSyncExternalStore(subscribeNotInterested, notInterestedVersion);
  const notice = currentNotInterestedNotice(viewer);
  useEffect(() => {
    if (!notice || notice.phase === 'pending') return;
    const id = setTimeout(() => dismissNotInterested(viewer, notice.operation), 10000);
    return () => clearTimeout(id);
  }, [viewer, notice]);
  if (!notice) return null;
  const t = currentI18n().t;
  return <Inset>{notice.error
    ? <Notice tone="error" compact>{t(notice.error === 'undo' ? 'entry.notInterestedUndoFailed' : 'entry.notInterestedFailed', {title: notice.title})}</Notice>
    : <Notice tone="info" compact action={{label: t('action.undo'), onClick: () => void undoNotInterested(viewer, notice)}}>{t('entry.notInterestedDone', {title: notice.title})}</Notice>
  }</Inset>;
}
