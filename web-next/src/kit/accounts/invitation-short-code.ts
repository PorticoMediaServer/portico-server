import type {ServerShortInvitation} from '@core/direct-account.ts';
import {formatShortInvitationCode} from '@core/short-invitation-code.ts';

export type ShortCodeState =
  | Readonly<{phase: 'idle'}>
  | Readonly<{phase: 'issuing'; operationId: string}>
  | Readonly<{phase: 'ready'; operationId: string; code: string; expiresAt: string}>
  | Readonly<{phase: 'unavailable'; operationId: string; expiresAt: string}>
  | Readonly<{phase: 'failed'; operationId: string; error: unknown}>;

export type ShortCodeRun = (operationId: string, signal: AbortSignal) => Promise<ServerShortInvitation>;

export const freshShortCodeOperationId = (): string => {
  const id = globalThis.crypto?.randomUUID?.();
  if (id) return id;
  const bytes = new Uint8Array(16);
  globalThis.crypto?.getRandomValues?.(bytes);
  return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
};

/**
 * One explicit issuance lifetime. A new code gets a fresh operation ID; a
 * retry of the same request reuses it. A receipt without a code is a replay:
 * it becomes an absence and never causes an automatic second request.
 */
export class ShortCodeIssuer {
  private state: ShortCodeState = {phase: 'idle'};
  private operationId?: string;
  private active?: AbortController;
  private generation = 0;
  private disposed = false;
  private readonly listeners = new Set<() => void>();
  constructor(private readonly createOperationId: () => string = freshShortCodeOperationId) {}
  getSnapshot = (): ShortCodeState => this.state;
  subscribe = (listener: () => void) => {if (this.disposed) return () => {};this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(state: ShortCodeState) {if (this.disposed) return;this.state = state;for (const listener of this.listeners) listener();}
  reset() {
    ++this.generation;this.active?.abort();this.active = undefined;this.operationId = undefined;
    this.publish({phase: 'idle'});
  }
  dispose() {this.disposed = true;++this.generation;this.active?.abort();this.active = undefined;this.operationId = undefined;this.listeners.clear();}
  retry(run: ShortCodeRun) {return this.issue(false, run);}
  async issue(fresh: boolean, run: ShortCodeRun): Promise<void> {
    if (this.disposed || this.state.phase === 'issuing') return;
    if (fresh || !this.operationId) this.operationId = this.createOperationId();
    if (!this.operationId) {this.publish({phase: 'failed', operationId: '', error: new Error('invitation_operation_required')});return;}
    const operationId = this.operationId, generation = ++this.generation;
    this.active?.abort();
    const active = new AbortController();this.active = active;
    this.publish({phase: 'issuing', operationId});
    try {
      const result = await run(operationId, active.signal);
      if (this.disposed || active.signal.aborted || generation !== this.generation) return;
      if (result.shortCode === undefined) {
        this.publish({phase: 'unavailable', operationId, expiresAt: result.expiresAt});
        return;
      }
      const code = formatShortInvitationCode(result.shortCode);
      if (!code) throw new Error('invalid_short_code');
      this.publish({phase: 'ready', operationId, code, expiresAt: result.expiresAt});
    } catch (error) {
      if (!this.disposed && !active.signal.aborted && generation === this.generation) this.publish({phase: 'failed', operationId, error});
    } finally {
      if (this.active === active) this.active = undefined;
    }
  }
}
