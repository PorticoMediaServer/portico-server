import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {AppSessionRecord} from '@core/app-sessions.ts';
import {defineWebScreen} from '../../route-registry';
import {router} from '../../router';
import {createBrowserAccountEntry, type BrowserAccountEntry} from '../../platform/account-entry';
import {browserAppSessions, observeBrowserSessionChanges} from '../../platform/server-connections';
import {browserAppSessionActions} from '../../platform/app-sessions';
import {browserServerDisplay} from '../../platform/server-display';
import {LocalServerAddress, ServerSwitcher} from '../../kit/accounts/ServerSwitcher';
import {AccountSignInView} from './AccountSignInView';
import {AccountSignInForm} from './sign-in-form';
import {savedServerChoices, type SavedServerChoice} from './saved-server-choices';

function ServersScreen() {
  const [instance, setInstance] = useState<Readonly<{entry: BrowserAccountEntry; form: AccountSignInForm}>>();
  useEffect(() => {
    const entry = createBrowserAccountEntry(), form = new AccountSignInForm(entry);
    setInstance({entry, form});
    return () => {form.dispose(); entry.dispose();};
  }, []);
  return instance ? <BoundServers {...instance} /> : null;
}
function BoundServers({entry, form}: {entry: BrowserAccountEntry; form: AccountSignInForm}) {
  const state = useSyncExternalStore(entry.subscribe, entry.getSnapshot);
  const [choices, setChoices] = useState<readonly SavedServerChoice[]>([]), [busy, setBusy] = useState(false), [working, setWorking] = useState(false), [error, setError] = useState<unknown>();
  const [mode, setMode] = useState<'chooser' | 'address' | 'entry'>('chooser'), [origin, setOrigin] = useState<string>();
  const alive = useRef(true), choosing = useRef(false), loading = useRef<AbortController>(undefined), record = useRef<AppSessionRecord>(undefined), currentChoices = useRef(choices);
  currentChoices.current = choices;
  const refresh = useCallback(async () => {
    loading.current?.abort(); const controller = loading.current = new AbortController();
    if (!alive.current) return;
    setBusy(true); setError(undefined);
    try {
      const saved = await browserAppSessions().read();
      if (!alive.current || controller.signal.aborted) return;
      const names = await browserServerDisplay().choices(saved.servers.map(({signIn, connection}) => ({signIn, pin: connection.pin, origin: connection.logicalOrigin})), controller.signal);
      if (!alive.current || controller.signal.aborted) return;
      record.current = saved; setChoices(savedServerChoices(saved, names));
      if (!saved.servers.length) setMode(value => value === 'chooser' ? 'address' : value);
    } catch (failure) {if (alive.current && !controller.signal.aborted) setError(failure);}
    finally {if (alive.current && !controller.signal.aborted) setBusy(false);}
  }, []);
  useEffect(() => {
    alive.current = true; void refresh();
    const off = observeBrowserSessionChanges(() => {void refresh();});
    return () => {alive.current = false; loading.current?.abort(); record.current = undefined; off();};
  }, [refresh]);
  const choose = async (choice: SavedServerChoice) => {
    const saved = record.current?.servers.find(value => value.connection.pin.serverId === choice.id);
    if (busy || choosing.current || !alive.current || !saved || choice.selected || !currentChoices.current.includes(choice)) return;
    choosing.current = true; setWorking(true); setError(undefined); form.clear();
    try {
      await browserAppSessionActions().switchServer(choice.id);
      if (!alive.current) return;
      // All restoration/profile/factor checks remain in the one real Entry.
      await entry.loadProfiles();
      if (!alive.current) return;
      const current = entry.getSnapshot();
      setOrigin(saved.connection.logicalOrigin);
      if (!current.snapshot) {
        const selected = await browserAppSessions().selected();
        if (!alive.current || selected?.connection.pin.serverId !== choice.id) return;
        await entry.inspect(saved.connection.logicalOrigin);
      }
      if (!alive.current) return;
      const result = entry.getSnapshot();
      if (result.snapshot || result.server) setMode('entry'); else setError(result.error);
    } catch (failure) {if (alive.current) setError(failure);}
    finally {choosing.current = false; if (alive.current) setWorking(false);}
  };
  const close = () => {form.clear(); entry.cancel(); void router.navigate({to: '/'});};
  if (mode === 'entry') return <AccountSignInView entry={entry} form={form} initialize={false} initialOrigin={origin}
    onCancel={() => {setMode('chooser'); void refresh();}} />;
  if (mode === 'address') return <LocalServerAddress busy={state.phase === 'checking'} error={state.error ?? error} onBack={() => {if (choices.length) setMode('chooser'); else close();}}
    onContinue={address => {if (state.phase === 'checking') return; form.clear(); setError(undefined); void entry.inspect(address).then(() => {
      if (!alive.current) return;
      const result = entry.getSnapshot();
      if (result.server && (result.phase === 'password' || result.phase === 'portico')) {setOrigin(result.origin); setMode('entry');}
    });}} />;
  return <ServerSwitcher choices={choices} busy={busy || working} error={error} onChoose={choice => void choose(choice)} onRefresh={() => void refresh()}
    onLocal={() => {form.clear(); entry.cancel(); setError(undefined); setMode('address');}} onClose={close} />;
}
export default defineWebScreen({id: 'servers', component: ServersScreen, chrome: 'standalone'});
