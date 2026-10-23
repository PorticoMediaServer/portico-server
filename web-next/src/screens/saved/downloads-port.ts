import type {DownloadTitle} from '@core/downloads.ts';
import type {SavedActor} from '@core/saved.ts';

/** Structural subset of the Settings parent scope. Family fences this binding only;
 * the native store persists server, authority, account and profile independently. */
export type SavedDownloadsScope = SavedActor & Readonly<{serverId: string; sessionFamilyId: string}>;
export type SavedDownloadRules = Readonly<{
  removeWatched: boolean;
  nextEpisode: boolean;
  unmeteredOnly: boolean;
  limitBytes: number;
}>;
export type SavedLocalDownload = Readonly<{
  id: string;
  revision: number;
  preparationId: string;
  itemId: string;
  quality: string;
  qualityLabel?: string;
  title: DownloadTitle;
  state: 'queued' | 'downloading' | 'paused' | 'failed' | 'ready';
  held: boolean;
  bytes: number;
  bytesDone: number;
  /** Native measured progress. Unknown length is null, never an invented zero. */
  percent: number | null;
  /** Present only when the native transfer has a measured rate. */
  etaSeconds?: number;
  offlineAccess: 'valid' | 'needsCheckIn';
  onServer: boolean;
  waitsForId?: string;
}>;
export type SavedDownloadsError = 'unavailable' | 'read' | 'write' | 'conflict' | 'folder' | 'play';
export type SavedDownloadsSnapshot = Readonly<{
  available: boolean;
  phase: 'loading' | 'ready' | 'error';
  items: readonly SavedLocalDownload[];
  /** Native aggregates across the entire captured viewer, before pagination. */
  groups?: readonly Readonly<{kind: 'episodes' | 'songs'; parentId: string; groupId: string; count: number}>[];
  nextCursor?: string;
  storage?: Readonly<{
    totalBytes: number;
    freeBytes: number;
    /** All native Portico files, including other accounts and servers. */
    usedBytes: number;
    /** Only the other-profile set eligible for the bounded removal below. */
    otherDownloadsBytes: number;
  }>;
  rules?: Readonly<{revision: number; value: SavedDownloadRules}>;
  /** The only deliberate native path exposed to this screen. */
  folder?: Readonly<{displayPath: string}>;
  error?: SavedDownloadsError;
}>;
/** Native-owned opaque CAS snapshot of exact entry ids and revisions. Only OTHER
 * profiles of this authority/server/account are eligible; never the active profile.
 * Changed targets reject with conflict. Confirmation never widens the deletion. */
export type SavedOtherDownloadsConfirmation = Readonly<{
  scope: SavedDownloadsScope;
  snapshotToken: string;
  bytes: number;
}>;
export type SavedDownloadsPort = Readonly<{
  scope: SavedDownloadsScope;
  subscribe(listener: () => void): () => void;
  getSnapshot(): SavedDownloadsSnapshot;
  refresh(signal: AbortSignal): Promise<void>;
  loadMore(signal: AbortSignal): Promise<void>;
  action(id: string, expectedRevision: number, action: 'pause' | 'resume' | 'retry' | 'remove', signal: AbortSignal): Promise<void>;
  play(id: string, signal: AbortSignal): Promise<void>;
  /** Actual persisted bounded bundle artwork. No path, token or network fallback. */
  getArtwork(id: string, signal: AbortSignal): Promise<Blob | null>;
  /** Publishes only after committed native CAS acknowledgement; no optimistic row. */
  setRules(expectedRevision: number, value: SavedDownloadRules, signal: AbortSignal): Promise<Readonly<{revision: number}>>;
  chooseFolder(signal: AbortSignal): Promise<void>;
  openFolder(signal: AbortSignal): Promise<void>;
  previewOtherProfiles(signal: AbortSignal): Promise<SavedOtherDownloadsConfirmation>;
  removeOtherProfiles(confirmation: SavedOtherDownloadsConfirmation, signal: AbortSignal): Promise<void>;
  dispose(): void;
}>;

export function sameSavedDownloadsScope(a: SavedDownloadsScope, b: SavedDownloadsScope): boolean {
  return a.serverId === b.serverId && a.authority === b.authority && a.accountId === b.accountId && a.profileId === b.profileId && a.sessionFamilyId === b.sessionFamilyId;
}
