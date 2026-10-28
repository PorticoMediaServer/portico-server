import type {ShellFactsSource} from '../../shell/fact-source';
import {subscribePresentation} from '../../shell/fact-hooks';
import {ReadingError, type HttpLocalApi, type LocalSession, type PreferenceSnapshot, type ReadingClient, type ReadingSession} from '@portico/client-core';
import type {ReadPreferenceSource} from './preferences';
import type {ReaderMountAuthority} from './mount';

export type ReadBindingContext = Readonly<{
  api: Pick<HttpLocalApi, 'reading'>; session: () => Pick<LocalSession, 'viewer' | 'sessionFamilyId'>;
  isCurrent: () => boolean; preferences?: Omit<ReadPreferenceSource, 'getSnapshot'> & Readonly<{getSnapshot: () => PreferenceSnapshot | undefined}>;
  facts: Readonly<{preferences: PreferenceSnapshot}>; factsSource?: ShellFactsSource; libraryRouteSignal?: AbortSignal;
}>;
export type ReadBindings = Readonly<{
  client: ReadingClient; authority: ReaderMountAuthority; preferences: ReadPreferenceSource;
  open: (request: Parameters<ReadingClient['open']>[0]) => Promise<ReadingSession>;
  close: () => Promise<void>; dispose: () => Promise<void>;
}>;

/** Capture the admitted shell once. No constructor requests, route strings,
 * token access, preference projection or independent book state. */
export function createReadBindings(context: ReadBindingContext): ReadBindings {
  const captured = context.session(), viewer = captured.viewer, life = new AbortController();
  const stopRoute = () => life.abort();
  context.libraryRouteSignal?.addEventListener('abort', stopRoute, {once: true});
  if (context.libraryRouteSignal?.aborted) stopRoute();
  let alive = true, retiring = false, client: ReadingClient | undefined;
  let pending: AbortController | undefined, session: ReadingSession | undefined;
  let closing = Promise.resolve(), disposal: Promise<void> | undefined;
  const authorized = () => {
    if (!alive || life.signal.aborted || !context.isCurrent() || client?.signal.aborted) return false;
    const current = context.session();
    return current.sessionFamilyId === captured.sessionFamilyId && current.viewer.serverId === viewer.serverId
      && current.viewer.authority === viewer.authority && current.viewer.accountId === viewer.accountId
      && current.viewer.profileId === viewer.profileId && current.viewer.role === viewer.role;
  };
  const current = () => !retiring && authorized();
  client = context.api.reading({viewer, isCurrent: authorized, signal: life.signal});
  const owner = client;
  const authority: ReaderMountAuthority = Object.freeze({isCurrent: current, subscribe(listener: () => void) {
    let notified = false;
    const retired = () => {if (!notified) {notified = true;listener();}};
    owner.signal.addEventListener('abort', retired); life.signal.addEventListener('abort', retired);
    return () => {owner.signal.removeEventListener('abort', retired); life.signal.removeEventListener('abort', retired);};
  }});
  const close = () => {
    pending?.abort(new ReadingError('cancelled')); pending = undefined;
    const previous = session; session = undefined;
    // Retirement never issues cleanup using another viewer's authority.
    let task = Promise.resolve();
    if (previous && authorized()) {
      const cleanup = new AbortController(), deadline = setTimeout(() => cleanup.abort(), 2000);
      task = previous.close(cleanup.signal).finally(() => clearTimeout(deadline));
    }
    closing = Promise.all([closing.catch(() => {}), task]).then(() => {});
    return closing;
  };
  const open: ReadBindings['open'] = async request => {
    if (!current()) throw new ReadingError('cancelled');
    const before = close(), operation = new AbortController(); pending = operation;
    await before;
    if (!current() || pending !== operation) throw new ReadingError('cancelled');
    const opened = await owner.open(request, operation.signal);
    if (!current() || pending !== operation) {
      if (authorized()) await opened.close();
      throw new ReadingError('cancelled');
    }
    pending = undefined; session = opened; return opened;
  };
  const dispose = () => {
    if (disposal) return disposal;
    // Keep captured authority valid through explicit Close, then retire it.
    const stopped = close(); retiring = true;
    disposal = stopped.finally(() => {alive = false; context.libraryRouteSignal?.removeEventListener('abort', stopRoute);life.abort(); owner.dispose();});
    return disposal;
  };
  const preferences: ReadPreferenceSource = Object.freeze({
    getSnapshot: () => context.preferences?.getSnapshot() ?? context.facts.preferences,
    subscribe: listener => subscribePresentation(context, listener),
  });
  return Object.freeze({client: owner, authority, preferences, open, close, dispose});
}
