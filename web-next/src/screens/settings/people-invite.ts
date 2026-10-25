import type {ServerInvitation} from '@core/direct-account.ts';

/** The exact create this People workspace will send for one invitation. One
 * attempt owns one body and one operation id, so a retry after an uncertain
 * answer repeats the same command instead of minting a second capability. */
export type InviteAttempt = Readonly<{
  operationId: string;
  body: Readonly<{operationId: string; role: string; allowedLibraries: readonly string[]; expiresInHours: number; notes?: string}>;
}>;

/** Definite refusals did not create anything, so the attempt may be replaced. */
const definite = (status: number | undefined) => status !== undefined && status >= 400 && status < 500 && status !== 408 && status !== 429;

/**
 * Single-flight create for the People Invite flow. `begin` is synchronous and
 * returns undefined while an attempt is in flight; an uncertain outcome keeps
 * the attempt (same body, same operation id) for the next explicit retry.
 */
export class InviteAttempts {
  private attempt: InviteAttempt | undefined;
  private running = false;
  private retired = false;

  constructor(private readonly createOperationId: () => string) {}

  /** The current attempt, or a new one. Undefined means "already running". */
  begin(input: Readonly<{role: string; allowedLibraries: readonly string[]; expiresInHours: number; notes?: string}>): InviteAttempt | undefined {
    if (this.running || this.retired) return undefined;
    this.running = true;
    if (!this.attempt) {
      const operationId = this.createOperationId();
      this.attempt = Object.freeze({operationId, body: Object.freeze({...input, allowedLibraries: Object.freeze([...input.allowedLibraries]), operationId})});
    }
    return this.attempt;
  }

  /** A created invitation ends the attempt; the next Invite is a new command. */
  created(): void { this.attempt = undefined; this.running = false; }

  /** An uncertain or retryable failure keeps the attempt for the retry. */
  failed(error: unknown): void {
    this.running = false;
    const status = (error as {status?: unknown})?.status;
    if (typeof status === 'number' && definite(status)) this.attempt = undefined;
  }

  /** Retired workspaces never send, reuse or publish anything again. */
  retire(): void { this.retired = true; this.running = false; this.attempt = undefined; }

  get pending(): InviteAttempt | undefined { return this.retired ? undefined : this.attempt; }
}

export type InviteCreated = ServerInvitation;
