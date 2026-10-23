import {readFileInfo, type DetailApi, type DetailScope, type DetailTarget, type FileInfoProjection} from '../detail.ts';
import type {PlaybackOptionsClient} from '../playback-v1/options.ts';
import type {MediaVersion, Session} from '../playback-v1/types.ts';
import type {SelectionContext} from '../selection-model/index.ts';
import {selectionExecutionError} from './execution.ts';

export type EntryMenuDetailsTarget = Readonly<{
  context: SelectionContext;
  scope: DetailScope;
  title: DetailTarget;
  /** The genuine server-selected playable item, distinct from a show/collection identity. */
  playbackItemId?: string;
}>;

/** Lazy Q6 facts. File/part counts never stand in for grouped available versions. */
export class EntryMenuDetails {
  private epoch = 0;
  private life: AbortController | undefined;
  private target: EntryMenuDetailsTarget | undefined;
  private versions: readonly MediaVersion[] = [];
  constructor(private readonly ports: {
    api: DetailApi;
    options: Pick<PlaybackOptionsClient, 'options'>;
    isCurrent: (context: SelectionContext) => boolean;
  }) {}
  select(target: EntryMenuDetailsTarget) {
    this.clear(); this.target = target; this.life = new AbortController();
  }
  clear() {this.epoch++; this.life?.abort(); this.life = undefined; this.target = undefined; this.versions = [];}
  private ticket() {
    const target = this.target, life = this.life, epoch = this.epoch;
    if (!target || !life || life.signal.aborted || !this.ports.isCurrent(target.context)) throw selectionExecutionError('stale-home-context');
    return {target, life, epoch};
  }
  private check(ticket: ReturnType<EntryMenuDetails['ticket']>) {
    if (ticket.epoch !== this.epoch || ticket.life.signal.aborted || !this.ports.isCurrent(ticket.target.context)) throw selectionExecutionError('stale-home-context');
  }
  async fileInfo(cursor?: string): Promise<FileInfoProjection> {
    const ticket = this.ticket();
    const result = await readFileInfo(this.ports.api, ticket.target.scope, ticket.target.title, {cursor, limit: 100}, ticket.life.signal);
    this.check(ticket);
    const context = ticket.target.context;
    if (result.scope.viewerFence !== context.scope.privacyFence || String(result.revision.catalog) !== context.revision.catalogRevision || String(result.revision.viewer) !== context.revision.viewerRevision) throw selectionExecutionError('home-offers-changed');
    return result;
  }
  async availableVersions(): Promise<readonly MediaVersion[]> {
    const ticket = this.ticket(), id = ticket.target.playbackItemId;
    if (!id) return [];
    const result = await this.ports.options.options(id, {}, ticket.life.signal);
    this.check(ticket);
    if (result.itemId !== id || new Set(result.versions.map(version => version.id)).size !== result.versions.length) throw selectionExecutionError('invalid-playback-options');
    this.versions = result.versions;
    return this.versions;
  }
  hasDifferentVersion() {return !!this.target && !!this.life && !this.life.signal.aborted && this.ports.isCurrent(this.target.context) && this.versions.length >= 2;}
  /** The host's real fenced playback runtime owns session/engine attachment and stale-start cleanup. */
  async playVersion(versionId: string, start: (request: Readonly<{context: SelectionContext; itemId: string; versionId: string; signal: AbortSignal}>) => Promise<Session>): Promise<Session> {
    const ticket = this.ticket(), itemId = ticket.target.playbackItemId;
    if (!itemId || !this.hasDifferentVersion() || !this.versions.some(version => version.id === versionId)) throw selectionExecutionError('version-not-offered');
    const session = await start({context: ticket.target.context, itemId, versionId, signal: ticket.life.signal});
    this.check(ticket);
    if (session.itemId !== itemId || session.versionId !== versionId) throw selectionExecutionError('playback-result-mismatch');
    return session;
  }
  dispose() {this.clear();}
}
