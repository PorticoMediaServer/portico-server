import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {AppSessionRecord, SignInKind} from '@core/app-sessions.ts';
import {presentError} from '@core/presentation/errors.ts';
import {defaultI18n} from '@i18n';
import {defineWebScreen} from '../../route-registry';
import {router} from '../../router';
import {createBrowserAccountEntry, type BrowserAccountEntry} from '../../platform/account-entry';
import {browserAccount} from '../../platform/account';
import {browserAccountOrigin} from '../../platform/account-origin';
import {browserAppSessions, observeBrowserSessionChanges} from '../../platform/server-connections';
import {browserAppSessionActions, signOutBrowserPorticoAccount} from '../../platform/app-sessions';
import {browserServerDisplay} from '../../platform/server-display';
import {ColourBackdrop} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {Icon} from '../../kit/Icon';
import {ProfileAvatarFace} from '../../kit/shell/AccountSurfaces';
import {CountdownRing} from '../../kit/states';
import {LocalServerAddress, ServerSwitcher} from '../../kit/accounts/ServerSwitcher';
import wordmark from '../../shell/wordmark.svg';
import {AccountSignInView} from './AccountSignInView';
import {AccountSignInForm} from './sign-in-form';
import {savedServerChoices, type SavedServerChoice} from './saved-server-choices';
import styles from './mweb-chooser.module.css';

const t = defaultI18n.t;

/** The frame's saved-server facts. Presence and ownership are optional display evidence:
 * the saved-session runtime knows the name and sign-in kind, never the account's last-seen
 * or owner facts. The DEV specimen supplies the drawn facts through this same typed row. */
export type MobileChooserRow = Readonly<{
  id: string;
  label: string;
  signIn: SignInKind;
  selected: boolean;
  status?: Readonly<{kind: 'online'}> | Readonly<{kind: 'seen'; at: string}> | Readonly<{kind: 'never'}>;
  owner?: boolean;
}>;
export type MobileChooserAttempt =
  | Readonly<{kind: 'loading'}>
  | Readonly<{kind: 'opening'; row: MobileChooserRow}>
  | Readonly<{kind: 'waiting'; row: MobileChooserRow; seconds: number; total: number}>
  | Readonly<{kind: 'unreachable'; row: MobileChooserRow}>
  | Readonly<{kind: 'loadFailed'}>;
export type MobileChooserProps = Readonly<{
  rows: readonly MobileChooserRow[];
  initial?: string;
  attempt?: MobileChooserAttempt;
  busy?: boolean;
  notice?: string;
  /** The frame was drawn with day-month dates; production keeps the app's locale formatter. */
  formatSeen?: (iso: string) => string;
  onChoose: (row: MobileChooserRow) => void;
  onRefresh: () => void;
  onLocal: () => void;
  onDifferentAccount?: () => void;
  onCheckNow: () => void;
  onOther: () => void;
}>;

const formatSeenDate = (iso: string) => defaultI18n.date(iso, 'dayMonth');
function statusText(row: MobileChooserRow, formatSeen: (iso: string) => string): Readonly<{text: string; online?: boolean}> {
  if (!row.status) return {text: t(row.signIn === 'local' ? 'accounts.session.local' : 'accounts.session.portico')};
  if (row.status.kind === 'online') return {text: t('web.servers.online'), online: true};
  if (row.status.kind === 'never') return {text: t('web.chooser.neverSeen')};
  return {text: t('web.chooser.lastSeen', {date: formatSeen(row.status.at)})};
}
function unreachableText(row: MobileChooserRow, formatSeen: (iso: string) => string): string {
  if (row.status?.kind === 'seen') return t('web.chooser.unreachableBody', {date: formatSeen(row.status.at)});
  if (row.status?.kind === 'never') return t('web.chooser.unreachableBodyNever');
  // The saved-session runtime has no presence clock; the honest sentence names the server.
  return t('web.signIn.error.unreachable', {server: row.label});
}
function ChooserHeader({initial}: {initial?: string}) {
  return <header className={styles.header}>
    <img src={wordmark} alt={t('accounts.brand')} />
    {initial && <span className={styles.avatar}><ProfileAvatarFace initial={initial} size={30} /></span>}
  </header>;
}
/** connect.html frame 6 at the authored mobile-web viewport: the list, opening, waiting,
 * can't-reach, loading and list-failed states. It has no browser chrome of its own. */
export function MobileServerChooser({rows, initial, attempt, busy = false, notice, formatSeen = formatSeenDate, onChoose, onRefresh, onLocal, onDifferentAccount, onCheckNow, onOther}: MobileChooserProps) {
  const showPortico = rows.some(row => row.signIn === 'portico');
  if (attempt) {
    const state = attempt.kind === 'loading' ? 'loading' : attempt.kind === 'loadFailed' ? 'loadFailed' : attempt.kind === 'opening' ? 'opening' : attempt.kind;
    const row = 'row' in attempt ? attempt.row : undefined;
    const title = state === 'loading' ? t('web.chooser.loading') : state === 'loadFailed' ? t('web.chooser.loadFailed')
      : state === 'opening' ? t('web.chooser.connecting', {server: row!.label})
      : state === 'waiting' ? t('web.chooser.waitingTitle', {server: row!.label})
      : t('web.chooser.unreachableTitle', {server: row!.label});
    const body = state === 'loadFailed' ? notice : state === 'waiting' ? t('web.chooser.waitingBody')
      : state === 'unreachable' ? unreachableText(row!, formatSeen) : undefined;
    const primaryLabel = state === 'waiting' ? t('web.chooser.checkNow') : state === 'unreachable' ? t('web.chooser.tryAgain') : state === 'loadFailed' ? t('web.chooser.tryAgain') : undefined;
    const onPrimary = state === 'loadFailed' ? onRefresh : onCheckNow;
    return <section className={styles.page} data-mweb-server-chooser data-state={state}>
      <ColourBackdrop shape="top" />
      <ChooserHeader initial={initial} />
      <div className={styles.state}>
        <div className={styles.stateIcon} data-accent={state === 'opening' || state === 'loading' || undefined} data-bad={state === 'loadFailed' || undefined}>
          {state === 'opening' || state === 'loading' ? <span className={styles.spinner} aria-hidden="true" /> : <Icon name={state === 'loadFailed' ? 'i-warn' : 'i-srvoff'} size={34} />}
        </div>
        <h2>{title}</h2>
        {body && <p>{body}</p>}
        {state === 'waiting' && <div className={styles.alive}><CountdownRing progress={attempt.kind === 'waiting' ? attempt.seconds / Math.max(1, attempt.total) : null} /><span>{t('next.states.retrying')}</span></div>}
        <div className={styles.stateActions}>
          {primaryLabel && <Button variant="primary" className={styles.stateButton} icon={state === 'unreachable' || state === 'loadFailed' ? 'i-retry' : undefined} onClick={onPrimary}>{primaryLabel}</Button>}
          {state === 'waiting' || state === 'unreachable' ? <Button variant="glass" className={styles.stateButton} onClick={onOther}>{t('web.chooser.otherServer')}</Button>
            : state === 'opening' ? <Button variant="text" className={styles.otherLink} onClick={onOther}>{t('web.chooser.otherServer')}</Button> : null}
        </div>
      </div>
    </section>;
  }
  const presented = notice;
  return <section className={styles.page} data-mweb-server-chooser>
    <ColourBackdrop shape="top" />
    <ChooserHeader initial={initial} />
    <div className={styles.content}>
      <h1>{t('portico.landing.pickTitle')}</h1>
      <p className={styles.lede}>{t('web.chooser.lede')}</p>
      {presented && <p className={styles.notice} role="alert"><Icon name="i-warn" size={20} />{presented}</p>}
      <div className={styles.group} aria-busy={busy || undefined}>
        {rows.map(row => {
          const status = statusText(row, formatSeen);
          return <button key={row.id} type="button" className={styles.row} data-row={row.id} data-off={row.status && row.status.kind !== 'online' || undefined}
            disabled={busy} onClick={() => onChoose(row)}>
            <span className={styles.rowIcon} data-online={status.online || undefined}><Icon name="i-server" size={18} /></span>
            <span className={styles.rowText}>{row.label}<small data-online={status.online || undefined}>{status.text}</small></span>
            {row.owner && <span className={styles.owner}>{t('web.hostedAccount.serverOwner')}</span>}
            <Icon name="i-right" size={18} />
          </button>;
        })}
      </div>
      <Button variant="glass" icon="i-refresh" className={styles.refresh} loading={busy} onClick={onRefresh} disabled={busy}>{t('accounts.servers.refresh')}</Button>
      <div className={styles.links}>
        {showPortico && onDifferentAccount && <button type="button" disabled={busy} onClick={onDifferentAccount}>{t('accounts.move.differentAccount')}</button>}
        <button type="button" disabled={busy} onClick={onLocal}>{t('accounts.signIn.local')}</button>
      </div>
    </div>
  </section>;
}

function useNarrowViewport() {
  const subscribe = useCallback((listener: () => void) => {
    const query = matchMedia('(max-width: 700px)');
    query.addEventListener('change', listener);
    return () => query.removeEventListener('change', listener);
  }, []);
  return useSyncExternalStore(subscribe, () => matchMedia('(max-width: 700px)').matches, () => false);
}
function initialOf(name: string | undefined) {
  const trimmed = name?.trim(), first = trimmed ? Array.from(trimmed)[0] : undefined;
  return first?.toLocaleUpperCase();
}
/** A failed probe is a wait only when the transport said it could come back; everything
 * else is the honest can't-reach state with the real action. */
function retryableFailure(error: unknown) {
  if (error instanceof TypeError) return true;
  if (!error || typeof error !== 'object') return false;
  const value = error as {retryable?: unknown; code?: unknown; name?: unknown};
  if (value.retryable === true) return true;
  if (value.name === 'TimeoutError') return true;
  return typeof value.code === 'string' && ['probe_timeout', 'timeout', 'request_failed', 'route_unavailable', 'offline', 'network'].includes(value.code);
}

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
  const account = browserAccount(), accountState = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot);
  const narrow = useNarrowViewport();
  const [choices, setChoices] = useState<readonly SavedServerChoice[]>([]), [busy, setBusy] = useState(false), [working, setWorking] = useState(false), [error, setError] = useState<unknown>();
  const [mode, setMode] = useState<'chooser' | 'address' | 'entry'>('chooser'), [origin, setOrigin] = useState<string>();
  const [attempt, setAttempt] = useState<MobileChooserAttempt>();
  const alive = useRef(true), choosing = useRef(false), loading = useRef<AbortController>(undefined), record = useRef<AppSessionRecord>(undefined), currentChoices = useRef(choices);
  const attemptEpoch = useRef(0), retryDelay = useRef(20);
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
  const openServer = useCallback(async (row: MobileChooserRow) => {
    const saved = record.current?.servers.find(value => value.connection.pin.serverId === row.id);
    if (!alive.current || choosing.current || !saved || !currentChoices.current.some(value => value.id === row.id)) return;
    const epoch = ++attemptEpoch.current;
    choosing.current = true; setWorking(true); setError(undefined); setAttempt({kind: 'opening', row}); form.clear();
    const fail = (failure: unknown) => {
      if (!alive.current || epoch !== attemptEpoch.current) return;
      setError(failure);
      if (retryableFailure(failure)) {
        const seconds = retryDelay.current;
        setAttempt({kind: 'waiting', row, seconds, total: seconds});
        retryDelay.current = Math.min(seconds * 2, 300);
      } else setAttempt({kind: 'unreachable', row});
    };
    try {
      await browserAppSessionActions().switchServer(row.id);
      if (!alive.current || epoch !== attemptEpoch.current) return;
      // All restoration/profile/factor checks remain in the one real Entry.
      await entry.loadProfiles();
      if (!alive.current || epoch !== attemptEpoch.current) return;
      const current = entry.getSnapshot();
      setOrigin(saved.connection.logicalOrigin);
      if (!current.snapshot) {
        const selected = await browserAppSessions().selected();
        if (!alive.current || epoch !== attemptEpoch.current || selected?.connection.pin.serverId !== row.id) return;
        await entry.inspect(saved.connection.logicalOrigin);
      }
      if (!alive.current || epoch !== attemptEpoch.current) return;
      const result = entry.getSnapshot();
      if (result.snapshot || result.server) {setAttempt(undefined); setMode('entry');} else fail(result.error);
    } catch (failure) {fail(failure);}
    finally {choosing.current = false; if (alive.current && epoch === attemptEpoch.current) setWorking(false);}
  }, [entry, form]);
  useEffect(() => {
    if (attempt?.kind !== 'waiting') return;
    if (attempt.seconds > 0) {
      const timer = setTimeout(() => {
        if (document.visibilityState !== 'visible') return;
        setAttempt(value => value?.kind === 'waiting' && value.seconds > 0 ? {...value, seconds: value.seconds - 1} : value);
      }, 1000);
      return () => clearTimeout(timer);
    }
    if (attempt.seconds <= 0 && document.visibilityState === 'visible') void openServer(attempt.row);
    return;
  }, [attempt, openServer]);
  useEffect(() => {
    const wake = () => {if (document.visibilityState === 'visible') setAttempt(value => value?.kind === 'waiting' && value.seconds <= 0 ? {...value} : value);};
    document.addEventListener('visibilitychange', wake);
    return () => document.removeEventListener('visibilitychange', wake);
  }, []);
  const close = () => {form.clear(); entry.cancel(); void router.navigate({to: '/'});};
  const other = () => {attemptEpoch.current++; entry.cancel(); setAttempt(undefined); setError(undefined);};
  const checkNow = () => {
    if (attempt?.kind === 'waiting' || attempt?.kind === 'unreachable') {retryDelay.current = 20; void openServer(attempt.row);}
  };
  const differentAccount = () => {
    if (busy || choosing.current || !alive.current) return;
    form.clear(); entry.cancel(); setWorking(true); setError(undefined);
    void signOutBrowserPorticoAccount().then(() => {
      if (!alive.current) return;
      location.assign(new URL('/', browserAccountOrigin(import.meta.env, location)).href);
    }).catch(failure => {if (alive.current) setError(failure);}).finally(() => {if (alive.current) setWorking(false);});
  };
  if (mode === 'entry') return <AccountSignInView entry={entry} form={form} initialize={false} initialOrigin={origin}
    onCancel={() => {setMode('chooser'); void refresh();}} />;
  if (mode === 'address') return <LocalServerAddress busy={state.phase === 'checking'} error={state.error ?? error} onBack={() => {if (choices.length) setMode('chooser'); else close();}}
    onContinue={address => {if (state.phase === 'checking') return; form.clear(); setError(undefined); void entry.inspect(address).then(() => {
      if (!alive.current) return;
      const result = entry.getSnapshot();
      if (result.server && (result.phase === 'password' || result.phase === 'portico')) {setOrigin(result.origin); setMode('entry');}
    });}} />;
  if (narrow) {
    const rows: readonly MobileChooserRow[] = choices;
    const presented = error ? presentError(error, 'sign-in', {operation: 'load'}) : undefined;
    const notice = presented && !presented.silent ? presented.body : undefined;
    const mobileAttempt = attempt ?? (busy && !rows.length ? {kind: 'loading'} as const : error && !rows.length ? {kind: 'loadFailed'} as const : undefined);
    return <MobileServerChooser rows={rows} initial={initialOf(accountState.session?.account.displayName)}
      attempt={mobileAttempt} busy={busy || working} notice={notice}
      onChoose={row => {retryDelay.current = 20; void openServer(row);}} onRefresh={() => void refresh()}
      onLocal={() => {form.clear(); entry.cancel(); setError(undefined); setMode('address');}} onDifferentAccount={rows.some(row => row.signIn === 'portico') ? differentAccount : undefined}
      onCheckNow={checkNow} onOther={other} />;
  }
  return <ServerSwitcher choices={choices} busy={busy || working} error={error} onChoose={choice => void openServer(choice)} onRefresh={() => void refresh()}
    onLocal={() => {form.clear(); entry.cancel(); setError(undefined); setMode('address');}} onClose={close} />;
}
export default defineWebScreen({id: 'servers', component: ServersScreen, chrome: 'standalone'});
