import type {ServerInvitation} from '@core/direct-account.ts';

/** The exact page a row's controls were painted from. */
export type InvitationPage = Readonly<{items: readonly ServerInvitation[]; nextCursor?: string; matchedCount: number; totalCount: number}>;
/** The committed request admission: the component plus the exact query, refresh,
 * version, service and authority currentness that asked for the painted page. */
export type InvitationRequest = Readonly<{component: object; request: object}>;
export type InvitationLease = Readonly<{request: InvitationRequest; page: InvitationPage | undefined}>;
/** One layout activation of the mounted screen. StrictMode's replay creates a
 * new one, so no token from the previous activation can be reused. */
export type InvitationAdmission = Readonly<{component: object; token: object}>;
export type InvitationOpening = Readonly<{admission: InvitationAdmission; lease: InvitationLease; invitation: ServerInvitation; operationId: string}>;
export type InvitationRead = Readonly<{admission: InvitationAdmission; request: InvitationRequest; cursor: string; signal: AbortSignal; abort: () => void}>;
export type InvitationCopy = Readonly<{admission: InvitationAdmission; lease: InvitationLease; invitation: ServerInvitation}>;

export function invitationRequest(component: object, request: object): InvitationRequest {
  return Object.freeze({component, request});
}
export function invitationLease(request: InvitationRequest, page: InvitationPage | undefined): InvitationLease {
  return Object.freeze({request, page});
}
export function invitationOpening(admission: InvitationAdmission, lease: InvitationLease, invitation: ServerInvitation, operationId: string): InvitationOpening {
  return Object.freeze({admission, lease, invitation, operationId});
}
export function sameOpening(current: InvitationOpening | null | undefined, opening: InvitationOpening | null | undefined): boolean {
  return !!current && current === opening;
}

/**
 * The screen's token layer. React state paints; this class decides what may
 * speak or act, synchronously and without reacquiring a later value.
 *
 * - The request admission is published separately from the resident page, so a
 *   new query, refresh, version or service retires the old controls at that
 *   commit, before the passive read clears the page they painted.
 * - `activate` is renewable per layout setup, so StrictMode's replay cannot
 *   leave the screen dead or revive a token from the previous activation.
 * - An opening exists only while it is the committed-active one. Closing it is
 *   synchronous, before React commits the next paint.
 */
export class InvitationAdmissions {
  private activation: InvitationAdmission | null = null;
  private committedRequest: InvitationRequest | null = null;
  private committedLease: InvitationLease | null = null;
  private opening: InvitationOpening | null = null;
  private running: InvitationOpening | null = null;
  private reading: InvitationRead | null = null;
  private copying: InvitationCopy | null = null;

  constructor(private readonly authorityCurrent: () => boolean) {}

  activate(component: object): InvitationAdmission {
    const admission: InvitationAdmission = Object.freeze({component, token: Object.freeze({})});
    this.activation = admission;
    return admission;
  }
  retire(admission: InvitationAdmission): void {
    if (this.activation === admission) this.activation = null;
  }
  activated(): boolean { return this.activation !== null; }
  live(): boolean { return this.activation !== null && this.authorityCurrent(); }

  publishRequest(request: InvitationRequest): void {
    if (this.activation && request.component === this.activation.component && this.live()) this.committedRequest = request;
  }
  retireRequest(request: InvitationRequest): void {
    if (this.committedRequest === request) this.committedRequest = null;
  }
  publishLease(lease: InvitationLease): void {
    if (this.activation && lease.request.component === this.activation.component && this.committedRequest === lease.request && this.live()) this.committedLease = lease;
  }
  retireLease(lease: InvitationLease): void {
    if (this.committedLease === lease) this.committedLease = null;
  }
  private leaseCurrent(lease: InvitationLease): boolean {
    return this.activation !== null && this.live() && this.committedLease === lease && this.committedRequest === lease.request;
  }
  resident(lease: InvitationLease, invitation: ServerInvitation): boolean {
    return !!lease.page && lease.page.items.some(item => item === invitation);
  }

  /** One read at a time for the exact committed request and cursor. A newer
   * request supersedes (and aborts) an older read; the old read can never paint. */
  requestRead(request: InvitationRequest, cursor: string): InvitationRead | undefined {
    if (!this.activation || !this.live() || this.committedRequest !== request) return undefined;
    if (this.reading && this.reading.request === request && this.reading.cursor === cursor) return undefined;
    this.reading?.abort();
    const controller = new AbortController();
    const read: InvitationRead = Object.freeze({admission: this.activation, request, cursor, signal: controller.signal, abort: () => controller.abort()});
    this.reading = read;
    return read;
  }
  readCurrent(read: InvitationRead): boolean {
    return this.reading === read && read.admission === this.activation && this.live() && this.committedRequest === read.request;
  }
  releaseRead(read: InvitationRead): void {
    if (this.reading !== read) return;
    this.reading = null;
    read.abort();
  }

  open(lease: InvitationLease, invitation: ServerInvitation, operationId: string): InvitationOpening | undefined {
    if (!this.activation || !this.leaseCurrent(lease) || !this.resident(lease, invitation) || this.running) return undefined;
    const opening = invitationOpening(this.activation, lease, invitation, operationId);
    this.opening = opening;
    return opening;
  }
  isOpen(opening: InvitationOpening | null): boolean {
    return !!opening && this.opening === opening && opening.admission === this.activation;
  }
  close(opening: InvitationOpening | null): void {
    if (opening && this.opening === opening) this.opening = null;
  }
  /** Only the exact committed-active opening may claim, once, synchronously. */
  claim(opening: InvitationOpening): InvitationOpening | undefined {
    if (!this.isOpen(opening) || this.running) return undefined;
    if (!this.leaseCurrent(opening.lease) || !this.resident(opening.lease, opening.invitation)) return undefined;
    this.running = opening;
    return opening;
  }
  settled(opening: InvitationOpening): boolean { return this.running === opening; }
  /** Exact opening, page and activation for anything effectful. */
  commandCurrent(opening: InvitationOpening): boolean {
    return this.running === opening && this.opening === opening && opening.admission === this.activation
      && this.leaseCurrent(opening.lease) && this.resident(opening.lease, opening.invitation);
  }
  release(opening: InvitationOpening): void {
    if (this.running === opening) this.running = null;
  }

  copy(lease: InvitationLease, invitation: ServerInvitation): InvitationCopy | undefined {
    if (!this.activation || !this.leaseCurrent(lease) || !this.resident(lease, invitation) || this.copying) return undefined;
    const claim: InvitationCopy = Object.freeze({admission: this.activation, lease, invitation});
    this.copying = claim;
    return claim;
  }
  copyCurrent(claim: InvitationCopy): boolean {
    return this.copying === claim && claim.admission === this.activation && this.leaseCurrent(claim.lease)
      && this.resident(claim.lease, claim.invitation);
  }
  releaseCopy(claim: InvitationCopy): void {
    if (this.copying === claim) this.copying = null;
  }
}
