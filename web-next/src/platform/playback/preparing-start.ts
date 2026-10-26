import type {PreparingSessionView, StartContext, StartOptions, StartTarget} from '@core/playback-v1/sessions.ts';

const retired = () => new DOMException('Original playback start retired', 'AbortError');
const valid = (predicate: () => boolean) => {try {return predicate() === true;} catch {return false;}};
const freeze = <T,>(value: T): T => {
  if (value && typeof value === 'object') {for (const child of Object.values(value)) freeze(child);Object.freeze(value);}
  return value;
};
type Flight = {seen: Set<string>; denied: boolean; session?: Readonly<{id: string; generation: number}>; cleanup?: AbortController; cancel(): void};
export type OriginalWebStart = Readonly<{
  /** The exact JSON body is passed to the canonical SDK, which owns its wire validation. */
  body: StartTarget & StartOptions; key: string; context: StartContext; finish(): void;
}>;
export type WebStartRegistration = Readonly<{
  remember(id: string): void; forget(id: string): void; dispose(): void;
  begin(target: StartTarget, options: StartOptions, key: string, signal: AbortSignal | undefined, isCurrent: () => boolean): OriginalWebStart;
}>;

/** Shell-private original-start custody, separate from queue-selection registrations.
 * A flight exists before request getters or I/O. Every actual registration seen
 * during that flight permanently denies cleanup for that id, even after forget.
 * Retiring a view/stack ends continuation, but does not erase its uninstalled 202
 * receipt while the original authenticated principal can still retire it. */
export class WebPreparingStarts {
  private readonly registrations = new Set<Readonly<{ids: Set<string>; installed(id: string): boolean}>>();
  private readonly flights = new Set<Flight>();
  private readonly requests = new Map<string, Flight>();
  private disposed = false;
  constructor(private readonly owner: Readonly<{isCurrent(): boolean;
    stop(session: PreparingSessionView, signal: AbortSignal, uninstalled: () => boolean): Promise<void>}>) {}
  private current = () => !this.disposed && valid(this.owner.isCurrent);
  attach(installed: (id: string) => boolean = () => false): WebStartRegistration {
    const ids = new Set<string>(), registration = Object.freeze({ids, installed});this.registrations.add(registration);
    let live = true;
    return Object.freeze({
      remember: (id: string) => {
        // A late real 201 is still registered by its original core cleanup path.
        // It must also deny any other pending preparation receipt for this id.
        for (const flight of this.flights) {
          if (flight.seen.size >= 256) flight.denied = true;
          else flight.seen.add(id);
          if (flight.session?.id === id) {flight.denied = true;flight.cleanup?.abort();}
        }
        if (live) ids.add(id);
      },
      forget: (id: string) => {ids.delete(id);},
      dispose: () => {live = false;this.registrations.delete(registration);ids.clear();},
      begin: (target: StartTarget, options: StartOptions, key: string, signal: AbortSignal | undefined, isCurrent: () => boolean) => {
        if (typeof key !== 'string' || !key || this.requests.has(key)) throw retired();
        const seen = new Set<string>();
        const flight: Flight = {seen, denied: false, cancel: () => {flight.denied = true;flight.cleanup?.abort();}};
        this.flights.add(flight);this.requests.set(key, flight);
        let done = false, cleaning = false, captured = false;
        const release = () => {this.flights.delete(flight);if (this.requests.get(key) === flight) this.requests.delete(key);};
        const finish = () => {done = true;if (!cleaning) release();};
        try {
          if (!live || !this.current() || !valid(isCurrent) || signal?.aborted) throw retired();
          for (const registered of this.registrations) for (const id of registered.ids) {
            if (seen.size >= 256) flight.denied = true;else seen.add(id);
          }
          // Capture once, before dispatch. Do not later reconstruct a body from a
          // mutable source, queue, quality choice or a successor caller.
          const body: StartTarget & StartOptions = freeze(JSON.parse(JSON.stringify({...target, ...options})));
          const continuation = () => live && !done && this.current() && !signal?.aborted && valid(isCurrent);
          if (!continuation()) throw retired();
          const capturePreparing = (offered: PreparingSessionView): (() => Promise<void>) | undefined => {
            if (captured || done) return undefined;
            captured = true;
            // Only an actual local response to this original request can create
            // custody. Transfer/receiver ownership is a different mechanism.
            if ('transferId' in body || offered.role !== 'local' || offered.state === 'ended' ||
              typeof offered.id !== 'string' || !offered.id || !Number.isSafeInteger(offered.presentation.generation) || offered.presentation.generation < 1 ||
              ('queue' in body ? offered.queue?.queueId !== body.queue.queueId || offered.queue?.entryId !== body.queue.entryId
                : 'itemId' in body ? offered.itemId !== body.itemId : 'channelId' in body ? offered.channelId !== body.channelId : true) ||
              body.versionId !== undefined && offered.versionId !== body.versionId ||
              body.discTitle !== undefined && (offered.discTitle?.id !== body.discTitle.id || offered.discTitle.revision !== body.discTitle.revision)) return undefined;
            flight.session = Object.freeze({id: offered.id, generation: offered.presentation.generation});
            const original = flight.session;
            let spent = false;
            const owned = () => !flight.denied && this.flights.has(flight) && flight.session === original && !flight.seen.has(original.id);
            const uninstalled = () => owned() && this.current() &&
              [...this.registrations].every(registered => valid(() => !registered.installed(original.id))) && this.current() && owned();
            return async () => {
              if (spent) return;spent = true;
              if (!uninstalled()) return;
              cleaning = true;
              const cleanup = flight.cleanup = new AbortController();
              try {
                if (!uninstalled()) return;
                // The original owner supplies its authenticated, bounded stop.
                // No installation, forget, navigation or successor publication.
                await this.owner.stop(offered, cleanup.signal, uninstalled);
              } finally {
                cleaning = false;flight.cleanup = undefined;release();
              }
            };
          };
          return Object.freeze({body, key, context: Object.freeze({signal, isCurrent: continuation, capturePreparing}), finish});
        } catch (error) {finish();throw error;}
      },
    });
  }
  dispose = () => {if (this.disposed) return;this.disposed = true;for (const flight of this.flights) flight.cancel();this.flights.clear();this.requests.clear();this.registrations.clear();};
}
