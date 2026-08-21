import type { PlaybackStartOptions } from '../../data/models';

const sourceTypes = new Set(['album', 'artist', 'playlist', 'collection', 'instant_mix', 'search', 'library', 'queue']);

export type PlaybackCollectionContext = Omit<NonNullable<PlaybackStartOptions['sourceContext']>, 'mediaIds'>;

type WatchNavigationState = {
  porticoPlayback?: PlaybackStartOptions;
};

function stringList(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const items = value.slice(0, 500).map((entry) => String(entry).trim()).filter(Boolean);
  return items.length ? items : undefined;
}

export function playbackOptionsForItems(
  items: Array<{ id: string; actions?: string[] }>,
  context: PlaybackCollectionContext,
): PlaybackStartOptions {
  const queueMediaIds = items.filter((item) => item.actions?.includes('play')).map((item) => item.id);
  return {
    queueMediaIds,
    sourceContext: { ...context, mediaIds: queueMediaIds },
  };
}

export function watchNavigationState(options?: PlaybackStartOptions): WatchNavigationState | undefined {
  if (!options) return undefined;
  return { porticoPlayback: options };
}

export function playbackOptionsFromNavigationState(value: unknown): PlaybackStartOptions {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const raw = (value as WatchNavigationState).porticoPlayback;
  if (!raw || typeof raw !== 'object') return {};
  const queueMediaIds = stringList(raw.queueMediaIds);
  const source = raw.sourceContext;
  const sourceMediaIds = source ? stringList(source.mediaIds) : undefined;
  const sourceContext = source && (!source.type || sourceTypes.has(source.type)) ? {
    type: source.type,
    id: typeof source.id === 'string' ? source.id.slice(0, 256) : undefined,
    title: typeof source.title === 'string' ? source.title.slice(0, 256) : undefined,
    mediaIds: sourceMediaIds,
  } : undefined;
  return {
    versionId: typeof raw.versionId === 'string' && raw.versionId.trim()
      ? raw.versionId.trim().slice(0, 256)
      : undefined,
    queueMediaIds,
    sourceContext,
    startSeconds: typeof raw.startSeconds === 'number' && Number.isFinite(raw.startSeconds) && raw.startSeconds >= 0 ? raw.startSeconds : undefined,
  };
}
