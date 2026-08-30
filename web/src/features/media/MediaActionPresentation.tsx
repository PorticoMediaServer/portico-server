import { ActionWatchlistIcon, ActionAddToListIcon, ActionConfirmIcon, ActionDownloadIcon, ActionMoreIcon, LibrarySavedIcon, PlaybackQualityIcon, ActionFavoriteIcon, NavigationLibraryIcon, MediaPlaylistIcon, ActionEditIcon, PlaybackPlayIcon, ActionRefreshIcon, PlaybackSeekForwardIcon, ActionResetIcon, NavigationSearchIcon, ActionRateIcon, ActionLikeIcon, ActionDeleteIcon, AccountWatchTogetherIcon, ActionCustomizeIcon } from '#portico-icons';
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
  if (glyph === 'Play') return <PlaybackPlayIcon />;
  if (glyph === 'RotateCcw') return <ActionResetIcon />;
  if (glyph === 'Bookmark') return <ActionWatchlistIcon />;
  if (glyph === 'BookmarkPlus') return <ActionAddToListIcon />;
  if (glyph === 'Heart') return <ActionFavoriteIcon />;
  if (glyph === 'CircleCheck') return <ActionConfirmIcon />;
  if (glyph === 'Download') return <ActionDownloadIcon />;
  if (glyph === 'ListPlus') return <ActionAddToListIcon />;
  if (glyph === 'FolderHeart') return <LibrarySavedIcon />;
  if (glyph === 'Library') return <NavigationLibraryIcon />;
  if (glyph === 'ListMusic') return <MediaPlaylistIcon />;
  if (glyph === 'Pencil') return <ActionEditIcon />;
  if (glyph === 'RefreshCw') return <ActionRefreshIcon />;
  if (glyph === 'RotateCw') return <PlaybackSeekForwardIcon />;
  if (glyph === 'ScanSearch') return <NavigationSearchIcon />;
  if (glyph === 'WandSparkles') return <ActionCustomizeIcon />;
  if (glyph === 'Gauge') return <PlaybackQualityIcon />;
  if (glyph === 'Trash2') return <ActionDeleteIcon />;
  if (glyph === 'UsersRound') return <AccountWatchTogetherIcon />;
  if (glyph === 'Star') return <ActionRateIcon />;
  if (glyph === 'ThumbsUp') return <ActionLikeIcon />;
  return <ActionMoreIcon />;
}
