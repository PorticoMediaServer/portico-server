import type { MediaItem, MediaKind } from '../../data/models';
import { normalizedContextKind } from './actionLabels';

export type ContextTarget = {
  id: string;
  title: string;
  subtitle: string;
  kind: MediaKind;
  type: MediaItem['type'];
  poster: string;
  backdrop: string;
  actions?: string[];
  watchlisted?: boolean;
  favorite?: boolean;
  watched?: boolean;
  progress?: number;
  progressSeconds?: number;
  fileCount?: number;
  reaction?: '' | 'like' | 'dislike';
  userRating?: number;
};

export function targetFromMedia(item: MediaItem): ContextTarget {
  return {
    id: item.id,
    title: item.title,
    subtitle: item.subtitle || [item.year || undefined, item.length || undefined].filter(Boolean).join(' · '),
    kind: normalizedContextKind(item),
    type: item.type,
    poster: item.poster,
    backdrop: item.backdrop,
    actions: item.actions,
    watchlisted: item.watchlisted,
    favorite: item.favorite,
    watched: item.watched,
    progress: item.progress,
    progressSeconds: item.progressSeconds,
    fileCount: item.fileCount,
    reaction: item.reaction,
    userRating: item.userRating,
  };
}
