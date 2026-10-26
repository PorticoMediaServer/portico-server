import {useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {AccessMember} from '@core/server-administration.ts';
import type {MemberResetCodeSheetProps} from '../../kit/settings/MemberResetCodeSheet';
import {router} from '../../router';
import {useOwnedResource} from './remote-sources/resources';
import {MemberResetCodeSource, type MemberResetCodeOpening} from './member-reset-code';

type Screen = Readonly<{opening?: MemberResetCodeOpening}>;
const noSubscribe = () => () => {};
const noState = () => undefined;
const t = defaultI18n.t;

/** People supplies the actual drawn member. This hook owns its committed panel
 * admission and each Make/result opening. Props are only compared for equality;
 * a retained handler cannot borrow a successor member, source or screen. */
export function useMemberResetCodeView(source: MemberResetCodeSource | undefined, person: AccessMember | undefined,
  isCurrent: () => boolean, blocked: boolean, reportError: (error: unknown) => void) {
  const inputs = useRef({source, person, isCurrent, blocked});
  inputs.current = {source, person, isCurrent, blocked};
  const [, render] = useState(0);
  const owned = useOwnedResource(() => {
    const location = router.state.location, abort = new AbortController();
    const entry = {source, person, isCurrent, live: true, screen: Object.freeze({}) as Screen, reportError,
      current: (): boolean => {try {return entry.live && !abort.signal.aborted && router.state.location === location &&
        inputs.current.source === source && inputs.current.person === person && inputs.current.isCurrent === isCurrent && isCurrent();} catch {return false;}},
      retire: () => {
        if (!entry.live) return;
        entry.live = false;abort.abort();entry.screen.opening?.retire();entry.screen = Object.freeze({});
      },
      signal: abort.signal,
      replace: (expected: Screen, next: Screen) => {
        if (!entry.current() || entry.screen !== expected) return false;
        entry.screen = Object.freeze(next);render(value => value + 1);return true;
      },
      off: () => {},
    };
    entry.off = router.subscribe('onBeforeNavigate', entry.retire);
    return entry;
  }, entry => {entry.retire();entry.off();}, [source, person, isCurrent, router.state.location]);
  const entry = owned?.value, screen = entry?.screen, opening = screen?.opening;
  const state = useSyncExternalStore(opening?.subscribe ?? noSubscribe, opening?.getSnapshot ?? noState, noState);
  useLayoutEffect(() => {
    if (blocked && opening && entry && screen) {opening.retire();entry.replace(screen, {});}
  }, [blocked, opening, entry, screen]);
  useLayoutEffect(() => {
    if (!entry?.current() || !opening || opening.getSnapshot() !== state || state?.kind !== 'unresolved') return;
    const error = state.error, status = error && typeof error === 'object' && 'status' in error ? error.status : undefined;
    if (status === 401 || status === 403) {entry.retire();entry.reportError(error);}
  }, [entry, opening, state]);
  if (!entry?.current() || !screen) return undefined;
  const owns = () => entry.current() && entry.screen === screen;
  const leave = () => {if (!owns()) return false;entry.retire();return true;};
  const present = !!source && !!person && source.eligible(person);
  const canMake = present && !blocked && !opening;
  const begin = () => {
    if (!owns() || !canMake || !source || !person || inputs.current.blocked) return;
    const ticket: {port?: MemberResetCodeOpening} = {};
    const opened = source.open(person, {signal: entry.signal, isCurrent: () => entry.current() && !inputs.current.blocked &&
      (ticket.port ? entry.screen.opening === ticket.port : entry.screen === screen && !screen.opening)});
    ticket.port = opened;
    // The source starts only its own explicit Make. Its pending port is not a
    // substitute result; a refused admission renders no sheet.
    if (opened && entry.replace(screen, {opening: opened})) return;
    opened?.retire();
  };
  let sheet: MemberResetCodeSheetProps | undefined;
  if (opening?.current() && state && state.kind !== 'retired') {
    const done = () => {
      if (!owns() || entry.screen.opening !== opening) return;
      opening.retire();entry.replace(screen, {});
    };
    sheet = {open: true, username: opening.target.username,
      result: state.kind === 'ready' ? {state: 'ready', code: state.result.code, expiresAt: state.result.expiresAt,
        ...(state.error ? {error: t('next.states.pageFailedBody')} : {})}
        : state.kind === 'pending' ? {state: 'pending'} : {state: 'unresolved', error: t('next.states.pageFailedBody')},
      turnOffTwoStep: state.turnOffTwoStep, busy: state.kind === 'pending',
      onTurnOffTwoStepChange: value => {if (owns() && !inputs.current.blocked) void opening.change(state, value);},
      onCopy: () => {if (owns() && !inputs.current.blocked) void opening.copy(state);}, onDone: done};
  }
  return {present, canMake, begin, leave, sheet};
}
