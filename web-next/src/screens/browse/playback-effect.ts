import type {ShellBindings} from '../../shell/bindings';
import type {ScreenContext} from '../detail/context';

type Runtime = Pick<NonNullable<ShellBindings['playback']>['runtime'], 'playFromScreen'>;

/** The same real runtime owns admission and adoption. Capture its identity and
 * the original caller lifetime; never resolve a later runtime to finish a start. */
export function createScreenPlayEffect(input: Readonly<{isCurrent: () => boolean; runtime: () => Runtime | undefined}>): ScreenContext['effects']['play'] {
  const authorityCurrent = input.isCurrent.bind(input), getRuntime = input.runtime.bind(input);
  return async (target, signal, callerCurrent = () => true) => {
    signal.throwIfAborted();
    if (!authorityCurrent() || !callerCurrent()) throw new DOMException('Screen playback changed', 'AbortError');
    const runtime = getRuntime();
    if (!runtime) throw new DOMException('Playback is still loading', 'AbortError');
    const current = () => !signal.aborted && authorityCurrent() && callerCurrent() && getRuntime() === runtime;
    if (!current()) throw new DOMException('Screen playback changed', 'AbortError');
    await runtime.playFromScreen(target, {signal, isCurrent: current});
    signal.throwIfAborted();
    if (!current()) throw new DOMException('Screen playback changed', 'AbortError');
  };
}

/** Whole selected season; the server snapshots canonical complete membership.
 * Workspace filters, viewer ordering and loaded item arrays never enter this effect. */
export function createScreenSeasonPlayEffect(input: Readonly<{isCurrent: () => boolean; runtime: () => Pick<NonNullable<ShellBindings['playback']>['runtime'], 'playFromQueue'> | undefined}>): NonNullable<ScreenContext['effects']['playSeason']> {
 const authorityCurrent = input.isCurrent.bind(input), getRuntime = input.runtime.bind(input);
 return async (seasonId, signal, callerCurrent) => {
  const capturedSeasonId = seasonId;
  signal.throwIfAborted();
  if (!authorityCurrent() || !callerCurrent()) throw new DOMException('Screen playback changed', 'AbortError');
  const runtime = getRuntime();
  if (!runtime) throw new DOMException('Playback is still loading', 'AbortError');
  const current = () => !signal.aborted && authorityCurrent() && callerCurrent() && getRuntime() === runtime;
  if (!current()) throw new DOMException('Screen playback changed', 'AbortError');
  await runtime.playFromQueue({container: {kind: 'season', id: capturedSeasonId}}, {signal, isCurrent: current});
  signal.throwIfAborted();
  if (!current()) throw new DOMException('Screen playback changed', 'AbortError');
 };
}
