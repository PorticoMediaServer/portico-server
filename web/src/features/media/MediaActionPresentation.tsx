import {
  Bookmark,
  BookmarkPlus,
  Check,
  Download,
  Ellipsis,
  FolderHeart,
  Gauge,
  Heart,
  Library,
  ListMusic,
  ListPlus,
  Pencil,
  Play,
  RefreshCw,
  RotateCw,
  RotateCcw,
  ScanSearch,
  Star,
  ThumbsUp,
  Trash2,
  UsersRound,
  WandSparkles,
} from '#portico-icons';
import { mediaActionsForSurface, type MediaActionSurface, type PresentedMediaAction } from '@porticomediaserver/client-core';
import { useMemo } from 'react';
import { useAuthSession, useProductContract } from '../../data/DataProvider';

export function useMediaActionPresentations(actionIds: readonly string[], surface: MediaActionSurface = 'web') {
  const contract = useProductContract();
  const auth = useAuthSession();
  const key = actionIds.join('\u001f');
  const permissions = auth.viewer?.user?.permissions;
  const includeWebAdministration = surface === 'web' && Boolean(
    permissions?.editMetadata || permissions?.manageLibraries || permissions?.deleteMedia,
  );
  return useMemo(() => {
    if (contract.status !== 'success') return [];
    const ids = actionIds as Parameters<typeof mediaActionsForSurface>[1];
    const actions = mediaActionsForSurface(contract.data, ids, surface);
    if (!includeWebAdministration) return actions;
    const seen = new Set(actions.map((action) => action.id));
    return [...actions, ...mediaActionsForSurface(contract.data, ids, 'web-admin').filter((action) => !seen.has(action.id))]
      .sort((left, right) => right.priority - left.priority);
  }, [contract, includeWebAdministration, key, surface]);
}

export function actionPresentation(actions: readonly PresentedMediaAction[], ...ids: string[]) {
  return actions.find((action) => ids.includes(action.id));
}

export function MediaActionIcon({ action }: { action: PresentedMediaAction }) {
  const glyph: string = action.icon.glyph;
  if (glyph === 'Play') return <Play />;
  if (glyph === 'RotateCcw') return <RotateCcw />;
  if (glyph === 'Bookmark') return <Bookmark />;
  if (glyph === 'BookmarkPlus') return <BookmarkPlus />;
  if (glyph === 'Heart') return <Heart />;
  if (glyph === 'CircleCheck') return <Check />;
  if (glyph === 'Download') return <Download />;
  if (glyph === 'ListPlus') return <ListPlus />;
  if (glyph === 'FolderHeart') return <FolderHeart />;
  if (glyph === 'Library') return <Library />;
  if (glyph === 'ListMusic') return <ListMusic />;
  if (glyph === 'Pencil') return <Pencil />;
  if (glyph === 'RefreshCw') return <RefreshCw />;
  if (glyph === 'RotateCw') return <RotateCw />;
  if (glyph === 'ScanSearch') return <ScanSearch />;
  if (glyph === 'WandSparkles') return <WandSparkles />;
  if (glyph === 'Gauge') return <Gauge />;
  if (glyph === 'Trash2') return <Trash2 />;
  if (glyph === 'UsersRound') return <UsersRound />;
  if (glyph === 'Star') return <Star />;
  if (glyph === 'ThumbsUp') return <ThumbsUp />;
  return <Ellipsis />;
}
