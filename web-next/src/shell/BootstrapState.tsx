import {createContext, useContext, useEffect, useState, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {HomeSkeleton, StateView} from '../kit/states';
import {LaunchUnreachableScreen} from './LaunchUnreachable';
import {router} from '../router';
import {useShellWidth} from './viewport';
import type {ShellBootstrapSnapshot} from './bootstrap';

type BootstrapState = Readonly<{snapshot: ShellBootstrapSnapshot; retry: () => Promise<void>; retryNow: () => Promise<void>}>;
const Context = createContext<BootstrapState | undefined>(undefined);
export function ShellBootstrapState({value, children}: {value: BootstrapState | undefined; children: ReactNode}) { return <Context.Provider value={value}>{children}</Context.Provider>; }
export function BootstrapContent({children}: {children: ReactNode}) {
  const state = useContext(Context), width = useShellWidth();
  const [started] = useState(Date.now), [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    if (state?.snapshot.phase !== 'restoring') return;
    const timers = [300, 8000].map(delay => setTimeout(() => setElapsed(Date.now() - started), Math.max(0, delay - (Date.now() - started))));
    return () => timers.forEach(clearTimeout);
  }, [state?.snapshot.phase, started]);
  useEffect(() => {
    if (state?.snapshot.phase !== 'signed-out' || state.snapshot.entry) return;
    const path = router.state.location.pathname;
    if (!['/setup', '/sign-in', '/profiles', '/servers', '/account'].includes(path) && !path.startsWith('/dev/')) void router.navigate({to: '/sign-in', replace: true});
  }, [state?.snapshot.phase, state?.snapshot.entry]);
  if (state?.snapshot.phase === 'restoring') return <HomeSkeleton elapsedMs={elapsed} window={{width, height: window.innerHeight, scrollTop: window.scrollY}} />;
  if (state?.snapshot.phase === 'unreachable' && state.snapshot.unreachable) return <LaunchUnreachableScreen facts={state.snapshot.unreachable}
    onRetry={() => { void state.retryNow(); }} onSwitchServer={() => { void router.navigate({to: '/servers'}); }} />;
  if (state?.snapshot.phase === 'error') return <StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: () => { void state.retry(); }}} />;
  return children;
}
