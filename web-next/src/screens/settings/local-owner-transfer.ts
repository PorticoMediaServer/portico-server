import type {ProfileAPI} from '@core/profile-management.ts';
import {SettingsPeopleClient} from '@core/settings-people.ts';
import {transferLocalOwnership} from '@core/portico-servers.ts';

export type LocalOwnerCandidate = Readonly<{accountId: string; username: string}>;
export type LocalOwnerTransferCaller = Readonly<{isCurrent: () => boolean; signal: AbortSignal}>;
export type LocalOwnerTransferOpening = Readonly<{members: readonly LocalOwnerCandidate[]; ownerRevision: number}>;
export type LocalOwnerTransferPort = Readonly<{
  opening: LocalOwnerTransferOpening;
  accept: (input: Readonly<{accountId: string; currentPassword: string; expectedRevision: number}>, caller: LocalOwnerTransferCaller) => Promise<void>;
  close: () => void;
}>;
type Admission = {
  readonly caller: LocalOwnerTransferCaller; readonly abort: AbortController;
  live: boolean; busy: boolean; port?: LocalOwnerTransferPort;
};
export type LocalOwnerTransferOptions = Readonly<{
  accountId: string; profileId: string;
  /** The captured Settings build must still be the same Local owner/main profile,
   * server, family and Sign-in opening. Never supply a latest-context getter. */
  isCurrent: () => boolean;
  /** Synchronously retire owner Settings, then reconcile through the original
   * session refresher. A 204 is not a new role or sign-in receipt. */
  onTransferred: () => void | Promise<void>;
}>;
const ended = () => new DOMException('The original ownership opening ended.', 'AbortError');
const changed = () => Object.assign(new Error('The ownership selection changed.'), {code: 'profile_changed'});
const activeCaller = (caller: LocalOwnerTransferCaller) => {
  if (caller.signal.aborted) return false;
  try {return caller.isCurrent();} catch {return false;}
};

/** Root63: one bounded People page under an authoritative Local-kind context.
 * The database's Local-kind fences forbid every Portico link; there is no
 * identity inference from names, per-person fanout or whole-members scan. */
export class LocalOwnerTransferSource {
  private closed = false;
  private admission?: Admission;
  private readonly people: SettingsPeopleClient;
  private readonly accountId: string;
  private readonly profileId: string;
  private readonly parentCurrent: () => boolean;
  private readonly onTransferred: () => void | Promise<void>;
  /** Used only to keep the existing sign-in move command mutually exclusive. */
  get inFlight() {return this.admission?.busy === true;}
  constructor(private readonly api: ProfileAPI, options: LocalOwnerTransferOptions) {
    this.accountId = options.accountId; this.profileId = options.profileId;
    this.parentCurrent = options.isCurrent; this.onTransferred = options.onTransferred;
    this.people = new SettingsPeopleClient(api, this.current);
  }
  private current = () => {
    if (this.closed) return false;
    try {return this.parentCurrent();} catch {return false;}
  };
  private owns = (entry: Admission) => this.current() && this.admission === entry && entry.live &&
    !entry.abort.signal.aborted && activeCaller(entry.caller);
  private assert(entry: Admission) {if (!this.owns(entry)) throw ended();}
  private retire(entry: Admission) {
    entry.live = false; entry.abort.abort();
    if (this.admission === entry) this.admission = undefined;
  }
  /** The row mints its own opening before this read. Cancel/reopen retires only
   * that opening; a late read cannot supply a successor with its candidates. */
  open = async (caller: LocalOwnerTransferCaller): Promise<LocalOwnerTransferPort> => {
    if (!this.current() || !activeCaller(caller)) throw ended();
    if (this.admission) this.retire(this.admission);
    const entry: Admission = {caller: Object.freeze({...caller}), abort: new AbortController(), live: true, busy: false};
    this.admission = entry;
    const signal = AbortSignal.any([entry.abort.signal, entry.caller.signal]);
    try {
      this.assert(entry);
      const [owner, page] = await Promise.all([
        this.people.person(this.accountId, signal),
        this.people.read({search: '', role: 'member', status: 'active', sort: 'person', direction: 'asc'}, '', signal),
      ]);
      this.assert(entry);
      if (owner.accountId !== this.accountId || owner.profileId !== this.profileId || owner.role !== 'owner' || owner.disabled || owner.hostedAccountId) throw changed();
      const members = Object.freeze(page.items.filter(row => row.role === 'member' && !row.disabled && row.accountId !== this.accountId)
        .map(row => Object.freeze({accountId: row.accountId, username: row.username})));
      const revisions = new Map(page.items.map(row => [row.accountId, row.revision]));
      const opening = Object.freeze({members, ownerRevision: owner.revision});
      const port: LocalOwnerTransferPort = Object.freeze({opening, close: () => this.retire(entry),
        accept: async (input, commandCaller) => {
          this.assert(entry);
          if (entry.port !== port || entry.busy || !activeCaller(commandCaller) || input.expectedRevision !== opening.ownerRevision ||
            !members.some(row => row.accountId === input.accountId)) throw ended();
          // Capture request-local scalars and the exact caller before the first await.
          const target = input.accountId, password = input.currentPassword, revision = opening.ownerRevision;
          const command = Object.freeze({...commandCaller});
          const commandSignal = AbortSignal.any([signal, command.signal]);
          const assert = () => {this.assert(entry); if (entry.port !== port || !activeCaller(command) || commandSignal.aborted) throw ended();};
          entry.busy = true;
          try {
            assert();
            const person = await this.people.person(target, commandSignal);
            assert();
            if (person.accountId !== target || person.role !== 'member' || person.disabled || person.hostedAccountId ||
              person.revision !== revisions.get(target)) throw changed();
            await transferLocalOwnership(this.api, target, password, revision, commandSignal);
            assert();
            // Spend this source before any await in authority reconciliation.
            this.dispose();
            await this.onTransferred();
          } finally {entry.busy = false;}
        },
      });
      entry.port = port;
      return port;
    } catch (error) {this.retire(entry); throw error;}
  };
  dispose = () => {
    if (this.closed) return;
    this.closed = true;
    if (this.admission) this.retire(this.admission);
    this.people.dispose();
  };
}
