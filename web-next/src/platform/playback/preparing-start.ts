import type {ChannelStartReceipt, OriginalChannelStart} from '@core/playback-v1/channel-control.ts';
import type {PreparingSessionView, StartContext, StartOptions, StartTarget} from '@core/playback-v1/sessions.ts';

const retired = () => new DOMException('Original playback start retired', 'AbortError');
const valid = (predicate: () => boolean) => {try {return predicate() === true;} catch {return false;}};
const freeze = <T,>(value: T): T => {
  if (value && typeof value === 'object') {for (const child of Object.values(value)) freeze(child);Object.freeze(value);}
  return value;
};
type Flight = {seen: Set<string>; denied: boolean; stopping?: boolean; session?: Readonly<{id: string; generation: number}>; cleanup?: AbortController; cancel(): void};
export type OriginalWebStart = Readonly<{
  /** The exact JSON body is passed to the canonical SDK, which owns its wire validation. */
  body: StartTarget & StartOptions; key: string; context: StartContext; finish(): void;
}>;
export type ChannelStartResource = Readonly<{id: string; generation: number}>;
export type WebChannelStart = ChannelStartReceipt;
export type WebStartRegistration = Readonly<{
  remember(id: string): boolean; forget(id: string): void; dispose(): void;
  beginChannel(original: OriginalChannelStart, isCurrent: () => boolean): WebChannelStart;
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
    stop(session: PreparingSessionView, signal: AbortSignal, uninstalled: () => boolean, claimStop: () => boolean): Promise<void>;
    stopChannel?(body: OriginalChannelStart['body'], session: ChannelStartResource, signal: AbortSignal, uninstalled: () => boolean, claimStop: () => boolean): Promise<void>}>) {}
  private current = () => !this.disposed && valid(this.owner.isCurrent);
  attach(installed: (id: string) => boolean = () => false): WebStartRegistration {
    const ids = new Set<string>(), registration = Object.freeze({ids, installed});this.registrations.add(registration);
    let live = true;
    const remember = (id: string) => {
      if ([...this.flights].some(flight => flight.stopping && flight.session?.id === id)) return false;
      for (const flight of this.flights) {
        if (flight.seen.size >= 256) flight.denied = true;
        else flight.seen.add(id);
        if (flight.session?.id === id) {flight.denied = true;flight.cleanup?.abort();}
      }
      if (live) ids.add(id);
      return true;
    };
    const begin = (target: StartTarget, options: StartOptions, key: string, signal: AbortSignal | undefined, isCurrent: () => boolean,
      channel?: OriginalChannelStart['body']): OriginalWebStart & {retireChannel(session: ChannelStartResource): void | Promise<void>; claimChannel(session: ChannelStartResource): boolean} => {
        if (typeof key !== 'string' || !key || this.requests.has(key)) throw retired();
        const seen = new Set<string>();
        const flight: Flight = {seen, denied: false, cancel: () => {flight.denied = true;flight.cleanup?.abort();}};
        this.flights.add(flight);this.requests.set(key, flight);
        let done = false, cleaning = false, captured = false;
        let receipt: (() => Promise<void>) | undefined;
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
          const channelBody = channel && 'channelId' in body ? Object.freeze({channelId: body.channelId, channelGeneration: body.channelGeneration!, state: body.state!}) : undefined;
          const continuation = () => live && !done && this.current() && !signal?.aborted && valid(isCurrent);
          if (!continuation()) throw retired();
          const acquire = (actual: ChannelStartResource, stop: (signal: AbortSignal, uninstalled: () => boolean, claimStop: () => boolean) => Promise<void>): (() => Promise<void>) | undefined => {
            if (captured || done) return undefined;
            captured = true;
            if (typeof actual.id !== 'string' || !actual.id || !Number.isSafeInteger(actual.generation) || actual.generation < 1) return undefined;
            flight.session = Object.freeze({id: actual.id, generation: actual.generation});
            const original = flight.session;
            let spent = false;
            const owned = () => !flight.denied && this.flights.has(flight) && flight.session === original && !flight.seen.has(original.id);
            const uninstalled = () => owned() && this.current() &&
              ![...this.flights].some(other => other !== flight && other.stopping && other.session?.id === original.id) &&
              [...this.registrations].every(registered => valid(() => !registered.installed(original.id))) && this.current() && owned();
            return receipt = async () => {
              if (spent) return;spent = true;
              if (!uninstalled()) return;
              cleaning = true;
              const cleanup = flight.cleanup = new AbortController();
              try {
                if (!uninstalled()) return;
                // The original owner supplies its authenticated, bounded stop.
                // No installation, forget, navigation or successor publication.
                await stop(cleanup.signal, uninstalled, () => {
                  if (flight.stopping || !uninstalled()) return false;
                  flight.stopping = true;return true;
                });
              } finally {
                cleaning = false;flight.cleanup = undefined;release();
              }
            };
          };
          const channelStop = (actual: ChannelStartResource) => acquire(actual, (signal, uninstalled, claimStop) =>
            this.owner.stopChannel ? this.owner.stopChannel(channelBody!, actual, signal, uninstalled, claimStop) : Promise.resolve());
          const capturePreparing: NonNullable<StartContext['capturePreparing']> = offered => {
            // Only the actual declared response to this original request can
            // acquire custody; channel facts never become a playable Session.
            if (captured || done) return undefined;
            if ('transferId' in body || offered.role !== 'local' || offered.state === 'ended' ||
              ('queue' in body ? offered.queue?.queueId !== body.queue.queueId || offered.queue?.entryId !== body.queue.entryId
                : 'itemId' in body ? offered.itemId !== body.itemId : 'channelId' in body ? offered.channelId !== body.channelId : true) ||
              body.versionId !== undefined && offered.versionId !== body.versionId ||
              body.discTitle !== undefined && (offered.discTitle?.id !== body.discTitle.id || offered.discTitle.revision !== body.discTitle.revision)) {captured = true;return undefined;}
            if (channel && (!['live', 'channel'].includes(offered.kind) || offered.queue || offered.itemId || offered.versionId || offered.discTitle)) {captured = true;return undefined;}
            const actual = Object.freeze({id: offered.id, generation: offered.presentation.generation});
            return channel ? channelStop(actual) : acquire(actual, (signal, uninstalled, claimStop) => this.owner.stop(offered, signal, uninstalled, claimStop));
          };
          const retireChannel = (actual: ChannelStartResource) => {
            if (!channel || done) return;
            // A final late response can spend only this request's first exact
            // resource. Never replace a real 202 receipt with another id/gen.
            if (captured) return flight.session?.id === actual.id && flight.session.generation === actual.generation ? receipt?.() : undefined;
            const original = Object.freeze({id: actual.id, generation: actual.generation});
            return channelStop(original)?.();
          };
          const claimChannel = (actual: ChannelStartResource) => {
            if (!channel || !continuation() || typeof actual.id !== 'string' || !actual.id ||
              !Number.isSafeInteger(actual.generation) || actual.generation < 1 ||
              captured && (!flight.session || flight.session.id !== actual.id || flight.session.generation !== actual.generation)) return false;
            // Canonical SDK819 calls this before a.id/accept/go-live. The same
            // registration synchronously excludes every older cleanup flight.
            return remember(actual.id) && continuation();
          };
          return Object.freeze({body, key, context: Object.freeze({signal, isCurrent: continuation, capturePreparing}), finish, retireChannel, claimChannel});
        } catch (error) {finish();throw error;}
      };
    return Object.freeze({
      // An actual current install denies older cleanup, even after forget.
      // A stop already synchronously claimed for that id refuses installation.
      remember,
      forget: (id: string) => {ids.delete(id);},
      dispose: () => {live = false;this.registrations.delete(registration);ids.clear();},
      begin,
      beginChannel: (original: OriginalChannelStart, isCurrent: () => boolean) => {
        const {body, key, signal, isCurrent: channelCurrent} = original;
        const current = () => valid(channelCurrent) && valid(isCurrent);
        const start = begin({channelId: body.channelId, channelGeneration: body.channelGeneration}, {state: body.state}, key, signal, current, body);
        return Object.freeze({isCurrent: start.context.isCurrent!, capturePreparing: start.context.capturePreparing!, claimInstall: start.claimChannel, retireUninstalled: start.retireChannel, finish: start.finish});
      },
    });
  }
  dispose = () => {if (this.disposed) return;this.disposed = true;for (const flight of this.flights) flight.cancel();this.flights.clear();this.requests.clear();this.registrations.clear();};
}
