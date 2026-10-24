import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {TakeAccountMove} from '../../route-registry-arrival';
import type {TakeAccountReturn} from '../../route-registry-return';
import {router} from '../../router';
import type {BrowserAccountEntry} from '../../platform/account-entry';
import {browserAppSessions} from '../../platform/server-connections';
import {browserAccountOrigin} from '../../platform/account-origin';
import {AuthError, AuthStage, LocalServerSignIn, PorticoServerSignIn} from '../../kit/accounts';
import {RequiredPasswordDialog} from '../../kit/accounts/RequiredSetup';
import {ServerChallenge} from '../../kit/accounts/ServerChallenge';
import {Button} from '../../kit/controls';
import {AccountSignInForm} from './sign-in-form';
import {ShortInvitationEntry} from './ShortInvitationEntry';
import {initializeAccountSignIn} from './sign-in-start';
import {RequiredFactorDialogs} from './RequiredSetup';

export function AccountSignInView({entry, form, takeReturn, takeMove, initialize = true, initialOrigin, onCancel}: {entry: BrowserAccountEntry; form: AccountSignInForm; takeReturn?: TakeAccountReturn; takeMove?: TakeAccountMove; initialize?: boolean; initialOrigin?: string; onCancel?: () => void}) {
  const state = useSyncExternalStore(entry.subscribe, entry.getSnapshot), input = useSyncExternalStore(form.subscribe, form.getSnapshot);
  const [error, setError] = useState<unknown>(), [initializing, setInitializing] = useState(initialize), [joining, setJoining] = useState(false), started = useRef(false), alive = useRef(true), joinOrigin = useRef<string | undefined>(undefined);
  const pageOrigin = initialOrigin ?? import.meta.env.VITE_SERVER_URL ?? location.origin;
  useEffect(() => {
    alive.current = true;
    if (initialize && !started.current) {
      started.current = true;
      void initializeAccountSignIn(entry, {takeReturn, takeMove, pageOrigin, isCurrent: () => alive.current,
        selectedOrigin: async () => (await browserAppSessions().selected())?.connection.logicalOrigin, username: value => form.setUsername(value),
      }).catch(failure => {if (alive.current) setError(failure);}).finally(() => {if (alive.current) setInitializing(false);});
    }
    return () => {alive.current = false;};
  }, [entry, form, takeReturn, takeMove, pageOrigin, initialize]);
  useEffect(() => {
    if (joining || initializing || entry.getSnapshot() !== state || input.factor || input.busy) return;
    if (state.phase === 'ready') void router.navigate({to: state.setupCompleted ? '/setup' : '/', replace: true});
    else if (state.phase === 'profiles' && state.snapshot && !state.snapshot.twoStepSetupRequired) void router.navigate({to: '/profiles', replace: true});
    else if (state.phase === 'two-step-setup' && state.origin) {
      form.setUsername(state.snapshot?.account.username ?? input.username);
      void entry.inspect(state.origin);
    }
  }, [entry, form, state, input, initializing, joining]);
  const join = () => {
    joinOrigin.current = entry.getSnapshot().origin ?? pageOrigin;
    form.clear();
    entry.cancel();
    setJoining(true);
  };
  const leaveJoin = () => {
    setJoining(false);
    const origin = joinOrigin.current ?? pageOrigin;
    joinOrigin.current = undefined;
    void entry.inspect(origin);
  };
  const reauthenticate = () => {
    const origin = entry.getSnapshot().origin ?? pageOrigin;
    form.clear(); setError(undefined); void entry.inspect(origin);
  };
  const cancel = () => {form.clear(); entry.cancel(); if (onCancel) onCancel(); else void router.navigate({to: '/servers'});};
  const busy = state.busy || input.busy, failure = state.error ?? error;
  if (joining) return <ShortInvitationEntry onBack={leaveJoin} onJoined={destination => location.assign(destination.origin)} />;
  if (input.factor) return <RequiredFactorDialogs form={input.factor} error={failure} onCancel={cancel} onDone={() => {form.clear(); void router.navigate({to: '/profiles', replace: true});}} />;
  if (state.phase === 'password-change') return <RequiredPasswordDialog key={input.inputEpoch} busy={busy} error={failure}
    onChange={(current, next) => {form.clear(); void entry.changePassword(current, next);}} onCancel={cancel} />;
  if (state.phase === 'challenge') return <ServerChallenge key={input.inputEpoch} busy={busy} error={failure} onCode={code => void form.completeChallenge(code)} onBack={reauthenticate} />;
  if (state.phase === 'password' && state.server?.signIn === 'local') return <LocalServerSignIn key={input.inputEpoch} serverName={state.server.name} initialUsername={input.username}
    busy={busy} error={failure} onSignIn={(username, password) => {setError(undefined); void form.signIn(username, password);}} onJoin={join} />;
  if (state.phase === 'portico' && state.server?.signIn === 'portico') return <PorticoServerSignIn serverName={state.server.name} busy={busy} error={failure} onContinue={() => {
    const epoch = input.inputEpoch, serverId = state.server?.id, origin = state.origin;
    if (!serverId) return;
    void entry.preparePorticoSignIn(browserAccountOrigin(import.meta.env, location)).then(url => {
      const current = entry.getSnapshot();
      if (url && alive.current && form.getSnapshot().inputEpoch === epoch && current.phase === 'portico' && current.server?.id === serverId && current.origin === origin) location.assign(url);
    }).catch(failure => {if (alive.current) setError(failure);});
  }} />;
  return <AuthStage legal={false}><AuthError error={failure} />{Boolean(failure) && <>
    <Button onClick={reauthenticate}>{defaultI18n.t('action.tryAgain')}</Button>
    <Button onClick={cancel}>{defaultI18n.t('settings.switchServer')}</Button>
  </>}</AuthStage>;
}
