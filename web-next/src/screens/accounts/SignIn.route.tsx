import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {defineWebScreen} from '../../route-registry';
import {useWebScreenContext} from '../../route-registry-view';
import type {TakeAccountReturn} from '../../route-registry-return';
import {router} from '../../router';
import {createBrowserAccountEntry, type BrowserAccountEntry} from '../../platform/account-entry';
import {browserAppSessions} from '../../platform/server-connections';
import {browserAccountOrigin} from '../../platform/account-origin';
import {AuthError, AuthStage, LocalServerSignIn, PorticoServerSignIn} from '../../kit/accounts';
import {RequiredPasswordDialog} from '../../kit/accounts/RequiredSetup';
import {ServerChallenge} from '../../kit/accounts/ServerChallenge';
import {Button} from '../../kit/controls';
import {AccountSignInForm} from './sign-in-form';
import {initializeAccountSignIn} from './sign-in-start';
import {RequiredFactorDialogs} from './RequiredSetup';

function SignInScreen() {
  const {takeAccountReturn} = useWebScreenContext();
  const [instance, setInstance] = useState<Readonly<{entry: BrowserAccountEntry; form: AccountSignInForm}>>();
  useEffect(() => {
    const entry = createBrowserAccountEntry(), form = new AccountSignInForm(entry);
    setInstance({entry, form});
    return () => {form.dispose(); entry.dispose();};
  }, []);
  return instance ? <BoundSignIn {...instance} takeReturn={takeAccountReturn} /> : null;
}
function BoundSignIn({entry, form, takeReturn}: {entry: BrowserAccountEntry; form: AccountSignInForm; takeReturn?: TakeAccountReturn}) {
  const state = useSyncExternalStore(entry.subscribe, entry.getSnapshot), input = useSyncExternalStore(form.subscribe, form.getSnapshot);
  const [error, setError] = useState<unknown>(), [initializing, setInitializing] = useState(true), started = useRef(false), alive = useRef(true);
  const pageOrigin = import.meta.env.VITE_SERVER_URL ?? location.origin;
  useEffect(() => {
    alive.current = true;
    if (!started.current) {
      started.current = true;
      void initializeAccountSignIn(entry, {takeReturn, pageOrigin, isCurrent: () => alive.current,
        selectedOrigin: async () => (await browserAppSessions().selected())?.connection.logicalOrigin, username: value => form.setUsername(value),
      }).catch(failure => {if (alive.current) setError(failure);}).finally(() => {if (alive.current) setInitializing(false);});
    }
    return () => {alive.current = false;};
  }, [entry, form, takeReturn, pageOrigin]);
  useEffect(() => {
    if (initializing || entry.getSnapshot() !== state || input.factor || input.busy) return;
    if (state.phase === 'ready') void router.navigate({to: '/', replace: true});
    else if (state.phase === 'profiles' && state.snapshot && !state.snapshot.twoStepSetupRequired) void router.navigate({to: '/profiles', replace: true});
    else if (state.phase === 'two-step-setup' && state.origin) {
      form.setUsername(state.snapshot?.account.username ?? input.username);
      void entry.inspect(state.origin);
    }
  }, [entry, form, state, input, initializing]);
  const reauthenticate = () => {
    const origin = entry.getSnapshot().origin ?? pageOrigin;
    form.clear(); setError(undefined); void entry.inspect(origin);
  };
  const cancel = () => {form.clear(); entry.cancel(); void router.navigate({to: '/servers'});};
  const busy = state.busy || input.busy, failure = state.error ?? error;
  if (input.factor) return <RequiredFactorDialogs form={input.factor} error={failure} onCancel={cancel} onDone={() => {form.clear(); void router.navigate({to: '/profiles', replace: true});}} />;
  if (state.phase === 'password-change') return <RequiredPasswordDialog key={input.inputEpoch} busy={busy} error={failure}
    onChange={(current, next) => {form.clear(); void entry.changePassword(current, next);}} onCancel={cancel} />;
  if (state.phase === 'challenge') return <ServerChallenge key={input.inputEpoch} busy={busy} error={failure} onCode={code => void form.completeChallenge(code)} onBack={reauthenticate} />;
  if (state.phase === 'password' && state.server?.signIn === 'local') return <LocalServerSignIn key={input.inputEpoch} serverName={state.server.name} initialUsername={input.username}
    busy={busy} error={failure} onSignIn={(username, password) => {setError(undefined); void form.signIn(username, password);}} />;
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
export default defineWebScreen({id: 'signIn', component: SignInScreen, chrome: 'standalone'});
