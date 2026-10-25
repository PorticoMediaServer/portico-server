import type {ServerInvitation} from '@core/direct-account.ts';

/** The exact create this People workspace will send for one invitation. One
 * attempt owns one body and one operation id, so a retry after an uncertain
 * answer repeats the same command instead of minting a second capability. */
export type InviteAttempt = Readonly<{
  operationId: string;
  body: Readonly<{operationId: string; role: string; allowedLibraries: readonly string[]; expiresInHours: number; notes?: string; porticoUsername?: string}>;
}>;

/** The drawn Invite form's choices, before they become one immutable command. */
export type InviteChoices = Readonly<{role: 'member' | 'admin'; allLibraries: boolean; libraryIds: readonly string[]; expiresInHours: number; notes?: string; porticoUsername?: string}>;
export const expiryChoices = Object.freeze([24, 72, 168, 720] as const);
export const defaultInviteChoices: InviteChoices = Object.freeze({role: 'member', allLibraries: true, libraryIds: [], expiresInHours: 168});

/** What the server gets: admin is the existing wire word for Administrator, and
 * private notes stay a create-only field (never a card or a link). */
export function inviteBody(choices: InviteChoices) {
  if (!choices.allLibraries && !choices.libraryIds.length) throw new Error('invite_library_required');
  return Object.freeze({role: choices.role === 'admin' ? 'admin' : 'member',
    allowedLibraries: Object.freeze([...(choices.allLibraries ? [] : choices.libraryIds)]),
    expiresInHours: choices.expiresInHours,
    ...(choices.notes?.trim() ? {notes: choices.notes.trim()} : {}),
    ...(choices.porticoUsername?.trim() ? {porticoUsername: choices.porticoUsername.trim()} : {})});
}

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
  begin(input: Readonly<{role: string; allowedLibraries: readonly string[]; expiresInHours: number; notes?: string; porticoUsername?: string}>): InviteAttempt | undefined {
    if (this.running || this.retired) return undefined;
    // Capture the body and the cryptographic id before committing the gate: a
    // refusing id generator must leave no stuck attempt and no fallback id.
    if (!this.attempt) {
      const operationId = this.createOperationId();
      this.attempt = Object.freeze({operationId, body: Object.freeze({...input, allowedLibraries: Object.freeze([...input.allowedLibraries]), operationId})});
    }
    this.running = true;
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

  get busy(): boolean { return this.running; }

  get pending(): InviteAttempt | undefined { return this.retired ? undefined : this.attempt; }
}

export type InviteCreated = ServerInvitation;
