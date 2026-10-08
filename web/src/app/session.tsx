import React, {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {HttpLocalApi, ViewerService, type HostedServer, type HostedSession, type LocalSession, type SystemInfo} from '@core/index.ts';
import {connectHostedServer, connectPairedServer, inspectDirectServer, rememberNativeSession, type RememberedServer} from '@core/server-connections.ts';
import type {ServerPin} from '@core/route-identity.ts';
import {ViewerSelectionCommit, selectedViewerRecord, type SelectedViewerRecord} from '@core/viewer-selection.ts';
import {changeRequiredPassword, completeDirectSignInChallenge, devE2ESignIn, directSignIn, selectDirectProfile, type DirectSignIn, parseDirectSnapshot, type DirectSnapshot} from '@core/profile-management.ts';
import type {SelectedSession} from '@core/session-selection.ts';
import {acceptPorticoCustody, acceptPorticoInvitation, isAccessRefused, isSessionMigrated, porticoSignIn, ServerListWatcher, type HostedRequestApi, type ServerListOutcome} from '@core/portico-servers.ts';
import {hostedGate} from '@core/hosted-gate.ts';
import {setCredentialLock, setInstallationIdentity, validInstallationId} from '@core/installation.ts';
import {LocalSessionRefresher} from '@core/session-refresh.ts';
import {browserAccount} from '../bridge/account';
import {hosted as hostedRequest} from '../bridge/api';
import {browserConnectionEnvironment, forgetBrowserConnection, restoreBrowserConnection, trackBrowserConnection} from '../bridge/server-connections';
import {browserViewerStorage} from '../bridge/viewer-selection-storage';
import {audioByteCache} from '../bridge/audio/bytes';
import {restoreOutcome, retryableServerRestore} from '../bridge/restore-policy';
import {browserInstallation, browserTrustLoaded, forgetBrowserTrust, readBrowserTrust, saveBrowserTrust} from '../bridge/profile-trust';
import {describeBrowser} from './device';
import {errorText} from './errors';
import {currentI18n} from './i18n';
import {knownServers} from './known-servers';
import {signInHintFor, type SignInHint} from './sign-in-hint';
import {belongsToAnotherAccount, listIsCurrent} from './account-switch';
import {accountFailure, signInFailure, superseded, type SignInStep} from './sign-in-errors';
import {CodedError} from '@core/server-messages.ts';
import {friendlyHost} from '@core/presentation/index.ts';
import {foregroundRetry, jittered} from './foreground-retry';
import {cachedServerList, lastServer, rememberLastServer, serverListStorage, storeServerList} from './server-list-cache';
import {forgetRailCache} from './rail-cache';
export type {SignInHint} from './sign-in-hint';
import {parseServerPresence} from '@core/index.ts';

/**
 * Session controller. Owns who is signed in, to which server, as which
 * profile, and how that was remembered in this browser. Protocol work is
 * delegated to client-core services and the browser transport bridge; this
 * file sequences them and exposes a screen-friendly API.
 */
export const viewer = new ViewerService();

/**
 * Who is signed in, not which token: access tokens rotate about every 13 minutes
 * (session-refresh), so anything that loads or subscribes per sign-in keys on this.
 */
export function sessionIdentity(session: Pick<LocalSession, 'sessionFamilyId' | 'viewer'> | undefined | null): string {
  return session ? `${session.viewer.serverId}/${session.viewer.accountId}/${session.viewer.profileId}/${session.sessionFamilyId}` : '';
}
/** The current access token, read at call time (for requests made outside `HttpLocalApi`). */
export const currentAccessToken = () => viewer.getSnapshot().session?.accessToken ?? '';

/* Device-bound sessions: every credential request names this installation (so the server binds
 * the sign-in to it and later accepts its refresh), and tabs rotate the shared credential one at
 * a time. */
(() => {
  const installationId = browserInstallation();
  try {
    if (validInstallationId(installationId)) setInstallationIdentity({installationId, name: describeBrowser(), platform: 'web', app: 'Portico Web', appVersion: import.meta.env.VITE_APP_VERSION ?? ''});
  } catch {}
  if (typeof navigator !== 'undefined' && navigator.locks) setCredentialLock((name, work) => navigator.locks.request(name, work));
})();

/** C45: `accountSession` is the account-scoped session from sign-in (several profiles or a PIN); it is retired once a profile is chosen. */
/** `unreachable`: the server is expected back (Portico retries it); `account-unavailable`: the Portico
 * Account service is down or busy, the server is fine (Portico retries once the service allows). */
export type PorticoOutcome = 'done' | 'refused' | 'unreachable' | 'account-unavailable' | 'failed';
export type DirectPending = {api: HttpLocalApi; snapshot?: DirectSnapshot; accountGrant: boolean; accountSession?: LocalSession; profilesError?: string; /** A Portico Account member: routes keep coming from Hosted. */ hosted?: RememberedServer['hosted']};
/** A direct sign-in that needs one more answer: a two-step code, a new password, or the owner's approval. */
export type DirectStep = {kind: 'code' | 'new-password' | 'device-pending'};
/** `hostedAccountId`: a Portico Account member of this server; its password and two-step verification live at the Portico Account. */
export type LocalAccount = {username: string; hostedAccountId?: string; /** INT M6: the owner is a Portico Account but Hosted's custody of the server's registration isn't with it yet. */ custodyPending?: boolean; profiles: readonly {id: string; name: string; art: string; primary: boolean; pinRequired: boolean}[]};
export type AccountState = ReturnType<ReturnType<typeof browserAccount>['service']['getSnapshot']>;

export type SessionValue = {
  phase: 'restoring' | 'signedOut' | 'ready';
  restoreMessage?: string;
  /**
   * The remembered server couldn't be reached at launch. The shell still opens (Settings,
   * Account and switching keep working) and shows this as a banner while Portico keeps
   * retrying; pages that need the server show their own state. A sign-in that has ended is
   * not a server problem: that goes to the sign-in screen (`signInHint`).
   */
  serverProblem?: {kind: 'unreachable'; serverId: string; serverUrl: string};
  /** Set when a sign-in has ended (renewal refused, credential revoked, the server replaced):
   * the sign-in screen opens on the same way in, with the server and username filled in. */
  signInHint?: SignInHint;
  /** Leaves an ended direct sign-in's form (a Portico Account that is still signed in goes back to the shell). */
  dismissSignInHint: () => void;
  api: HttpLocalApi;
  serverUrl?: string;
  session?: LocalSession;
  system?: SystemInfo;
  owner: boolean;
  hostedUrl: string;
  account: AccountState;
  hosted?: Omit<HostedSession, 'refreshToken'>;
  /** Direct (server-local) account identity, when signed in that way. */
  local?: LocalAccount;
  servers: readonly HostedServer[];
  serversStatus: 'idle' | 'loading' | 'ready' | 'error';
  serversError?: string;
  refreshServers: () => void;
  attachedServer?: HostedServer;
  error: string;
  busy: boolean;
  clearError: () => void;
  cancelRestore: () => void;
  retryRestore: () => void;
  direct: {
    origin: string;
    setOrigin: (value: string) => void;
    pin?: ServerPin;
    inspect: () => Promise<void>;
    resetPin: () => void;
    /** `dev`: development builds only; signs in as the server's seeded e2e owner, with no password. */
    signIn: (username: string, password: string, at?: string, dev?: boolean) => Promise<void>;
    /** The pending extra step of a direct sign-in, if any (C45). */
    step?: DirectStep;
    /** Answers it: the two-step code, or the new password. */
    continueSignIn: (value: string) => Promise<void>;
    cancelStep: () => void;
    pending?: DirectPending;
    chooseProfile: (profileId: string, options?: {pin?: string; trust?: boolean}) => Promise<void>;
    cancelPending: () => void;
    openProfileChooser: () => void;
    reloadProfiles: () => void;
    /** Establish a paired connection for Quick Connect (code approved elsewhere). */
    pairedApi: () => Promise<HttpLocalApi>;
    acceptIssued: (api: HttpLocalApi, session: LocalSession) => Promise<void>;
  };
  /**
   * Portico Account members (Spec — Hosted at Scale): the server owns membership. Connecting is a
   * direct sign-in with a Hosted identity assertion (`porticoSignIn`), then the server's own
   * profile chooser (`direct.pending`). An invitation link is accepted the same way.
   */
  portico: {
    /** `refused`: not a member (the server leaves the list); `unreachable`: try again later. */
    connect: (server: HostedServer) => Promise<PorticoOutcome>;
    acceptInvitation: (origin: string, code: string) => Promise<boolean>;
    /** Settings › Refresh Servers: one version check now, the list only if it moved. */
    refreshServers: () => Promise<ServerListOutcome>;
    /** When the server list was last confirmed current (ms), if known. */
    checkedAt?: number;
    /** The server this Portico Account last opened in this browser. */
    lastServerId?: string;
    /** INT M6: accept Hosted's custody of this server's registration (owner, `local.custodyPending`). */
    acceptCustody: () => Promise<void>;
  };
  /** A Portico Account session obtained through the browser handoff or account sign-in. */
  accountSignedIn: () => void;
  signOutAccount: () => Promise<void>;
  signOut: () => void;
  disconnectServer: () => void;
};

const SessionContext = createContext<SessionValue | null>(null);
/** X-04: presented by code and status, never from `error.message` (a cancelled operation stays silent). */
const message = (e: unknown, _fallback: string, context: 'sign-in' | 'account' | 'profiles' = 'sign-in') => errorText(e, context, 'action');
const idle = () => new HttpLocalApi('http://127.0.0.1:1');
/** INT gate 3: every sign-in failure is said by `signInFailure` (app/sign-in-errors.ts), never by the generic presenter. */
const signInText = (e: unknown, step: SignInStep, server?: string) => signInFailure(e, {step, server, online: typeof navigator === 'undefined' ? undefined : navigator.onLine}, currentI18n()).text;
const hostOf = (origin?: string) => (origin ? friendlyHost(origin) ?? origin.replace(/^https?:\/\//, '') : undefined);

/** What a person types is rarely a URL. A bare host gets a scheme: plain HTTP for an address on
 * the home network (an IP, localhost, a .local name), HTTPS for anything else. */
export function serverAddress(typed: string): string {
  const value = typed.trim().replace(/\/+$/, '');
  if (/^https?:\/\//i.test(value)) return value;
  const host = value.replace(/[:/].*$/, '');
  const home = /^(localhost|\d{1,3}(\.\d{1,3}){3}|\[[0-9a-f:]+\]|[^.]+|.+\.local|.+\.lan|.+\.home)$/i.test(host);
  return (home ? 'http://' : 'https://') + value;
}

export function SessionProvider({children}: {children: React.ReactNode}) {
  const scope = useSyncExternalStore(viewer.subscribe, viewer.getSnapshot);
  const central = useMemo(() => browserAccount(), []);
  const account = useSyncExternalStore(central.service.subscribe, central.service.getSnapshot);
  const [api, setApi] = useState<HttpLocalApi>(idle);
  const [system, setSystem] = useState<SystemInfo>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [restoring, setRestoring] = useState(true);
  const [restoreMessage, setRestoreMessage] = useState<string>();
  const [offline, setOffline] = useState<{record: SelectedViewerRecord; kind: 'unreachable'}>();
  const [signInHint, setSignInHint] = useState<SignInHint>();
  /** A sign-in has ended: forget it and open the sign-in screen for the same server (below). */
  const signInEnded = useRef<(serverUrl: string, session: LocalSession, username?: string, portico?: boolean) => void>(() => {});
  /** Silent re-admission of a Portico Account member after `session_migrated` (below). */
  const dropServerRef = useRef<(serverId: string) => void>(() => {});
  const readmitRef = useRef<(serverUrl: string, session: LocalSession) => Promise<void>>(async () => {});
  const offlineRef = useRef(offline);
  offlineRef.current = offline;
  const [established, setEstablished] = useState(false);
  const [directOrigin, setDirectOrigin] = useState(() => import.meta.env.VITE_SERVER_URL ?? location.origin);
  const [pin, setPin] = useState<ServerPin>();
  const [pending, setPending] = useState<DirectPending>();
  const [directStep, setDirectStep] = useState<DirectStep>();
  // The secrets a step needs stay out of React state.
  // `again`: a Portico sign-in repeats its identity sign-in (a device waiting for approval) instead of a password.
  const stepRef = useRef<{connected: Awaited<ReturnType<typeof connectPairedServer>>; username: string; password: string; token?: string; session?: LocalSession; again?: () => Promise<DirectSignIn>; hosted?: RememberedServer['hosted']} | undefined>(undefined);
  const [local, setLocal] = useState<LocalAccount>();
  const [servers, setServers] = useState<readonly HostedServer[]>([]);
  const [serversStatus, setServersStatus] = useState<SessionValue['serversStatus']>('idle');
  const [serversError, setServersError] = useState<string>();
  const [attachedServer, setAttachedServer] = useState<HostedServer>();
  const commit = useMemo(() => new ViewerSelectionCommit(browserViewerStorage), []);
  /** Keeps the published viewer's 15-minute access token alive (session-refresh.ts). */
  const refresher = useRef<LocalSessionRefresher | undefined>(undefined);
  const refreshEnded = useRef<(error?: unknown) => void>(() => {});
  const stopRefresh = useCallback(() => { refresher.current?.stop(); refresher.current = undefined; }, []);
  useEffect(() => {
    const wake = () => { if (!document.hidden) refresher.current?.wake(); };
    document.addEventListener('visibilitychange', wake);
    addEventListener('online', wake);
    return () => { document.removeEventListener('visibilitychange', wake); removeEventListener('online', wake); stopRefresh(); };
  }, [stopRefresh]);
  const generation = useRef(0);
  const chooserGeneration = useRef(0);
  /** WEB-08: the profile selection in flight. Cancelling or replacing one bumps
   * this, and only the current generation may publish anything. */
  const selection = useRef(0);
  const selectionAbort = useRef<AbortController | undefined>(undefined);
  const selectionAuthority = useRef(0);
  const invalidateSelection = useCallback((authorityChanged = false) => {
    ++selection.current;
    if (authorityChanged) ++selectionAuthority.current;
    selectionAbort.current?.abort();
    selectionAbort.current = undefined;
  }, []);
  useEffect(() => () => invalidateSelection(true), [invalidateSelection]);
  const restoreControl = useRef<{cancel: () => void; retry: () => void}>({cancel: () => {}, retry: () => {}});

  /* Restore the tab's remembered viewer once at launch, retrying transport failures. */
  useEffect(() => {
    let active = true, running = false, attempts = 0, portico = false;
    // Reconnect attempts run only while this tab is visible (like Apple's foreground-only
    // reconnect): a hidden tab never polls, and a retry due meanwhile runs when it's shown.
    const retry = foregroundRetry(() => void run());
    const stop = () => {
      active = false;
      retry.stop();
    };
    restoreControl.current.cancel = () => {
      stop();
      setRestoring(false);
      setRestoreMessage(undefined);
    };
    const run = async () => {
      if (!active || running) return;
      retry.disarm();
      running = true;
      setRestoreMessage(undefined);
      try {
        const record = await browserViewerStorage.read();
        if (!active) return;
        // Whether the saved sign-in is a Portico Account member's (its routes come from Hosted),
        // read before a refused renewal clears it.
        portico = record ? record.session.viewer.authority === 'hosted' || !!(await browserConnectionEnvironment().storage.read().catch(() => undefined))?.hosted : false;
        if (record) {
          const routed = await restoreBrowserConnection(record.session);
          if (!active) return;
          if (!routed) throw new Error('The saved connection no longer matches this tab’s profile. Sign in again.');
          await commitSelectedViewer(routed.api.baseUrl, routed.session, undefined, true, undefined, true);
        }
        stop();
        setOffline(undefined);
        setRestoring(false);
      } catch (e) {
        if (!active) return;
        // Web always opens into the shell: a remembered viewer whose server can't be verified
        // is shown as a banner there, never as a full-page blocker.
        const record = await browserViewerStorage.read().catch(() => undefined);
        if (!active) return;
        // The server moved this sign-in to its own membership (migration 0120): re-admit silently.
        if (record && isSessionMigrated(e)) {
          stop();
          await readmitRef.current(record.serverUrl, record.session);
          setRestoring(false);
          return;
        }
        // Reset only when the renewal was refused or the server now has another id (shared rule,
        // client-core 440a731); a 400, a 404 or a mismatched saved connection keeps the sign-in.
        const outcome = record ? await restoreOutcome(e, {serverUrl: record.serverUrl, serverId: record.session.viewer.serverId}) : retryableServerRestore(e) ? 'retry' : 'reset';
        if (!active) return;
        if (outcome === 'retry') {
          setRestoreMessage('Your server is not responding. Portico keeps trying to reconnect.');
          if (record) { setOffline({record, kind: 'unreachable'}); setRestoring(false); }
          // Quick retries first, then about every two minutes (plus on 'online' and Try again),
          // with jitter, and only while the tab is visible.
          const wait = attempts < 5 ? Math.min(30000, 2000 * 2 ** attempts) : 120000;
          attempts++;
          retry.arm(jittered(wait));
        } else {
          stop();
          // The sign-in has ended (refused renewal, or another server at the address): straight to
          // the sign-in screen for it, never the shell with a "sign in again" banner.
          if (record) {
            if (portico && isAccessRefused(e)) dropServerRef.current(record.session.viewer.serverId);
            signInEnded.current(record.serverUrl, record.session, undefined, portico);
          } else setError('Your saved connection could not be verified. Sign in again to continue.');
          setRestoring(false);
        }
      } finally {
        running = false;
      }
    };
    restoreControl.current.retry = () => void run();
    const online = () => { if (document.visibilityState === 'hidden') { if (!retry.pending()) retry.arm(0); } else void run(); };
    addEventListener('online', online);
    void run();
    return () => {
      stop();
      removeEventListener('online', online);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /* The music engine's compressed bytes belong to one viewer: a sign-out or a switch of profile or
     server empties the in-tab cache (INT review, as C3's native cache does). */
  const viewerKey = sessionIdentity(scope.session);
  const lastViewerKey = useRef(viewerKey);
  useEffect(() => { if (lastViewerKey.current !== viewerKey) { lastViewerKey.current = viewerKey; audioByteCache.clear(); } }, [viewerKey]);

  /* A hosted viewer cannot outlive its Portico Account session in this browser. */
  useEffect(() => {
    if (scope.phase === 'signedOut' || scope.session?.viewer.authority !== 'hosted') return;
    if ((account.phase === 'signedOut' && !established) || (account.session && account.session.account.id !== scope.session.viewer.accountId)) {
      void new HttpLocalApi(scope.serverUrl!, scope.session.accessToken).logout().catch(() => {});
      stopRefresh();
      viewer.clear();
      void commit.clear().catch(() => {});
      setApi(idle());
    }
  }, [scope, account, commit, established, stopRefresh]);

  /* Server directory for the signed-in account (Spec — Hosted at Scale). The list is kept between
     visits and Hosted is asked only when the version check is due (at most every ~6 h, in the
     foreground), at sign-in, and from Settings › Refresh Servers. */
  const accountId = account.session?.account.id;
  const [dropped, setDropped] = useState<ReadonlySet<string>>(() => new Set());
  const droppedRef = useRef(dropped);
  droppedRef.current = dropped;
  const [listCheckedAt, setListCheckedAt] = useState<number>();
  const adoptServers = useCallback((owner: string, items: readonly HostedServer[]) => {
    // Presence (online state) is optional; malformed or absent reads as unknown.
    const list = items.map(server => ({...server, presence: parseServerPresence((server as {presence?: unknown}).presence)}));
    storeServerList(owner, list);
    // An answer for the account that was signed in before a switch is kept for it, never shown.
    if (!listIsCurrent(owner, central.service.getSnapshot().session?.account.id)) return;
    setServers(list);
    setServersStatus('ready');
    setServersError(undefined);
    // A fresh list replaces the servers dropped after a refusal, and the refusal's message with them.
    if (droppedRef.current.size) setError('');
    setDropped(new Set());
  }, [central]);
  const listServers = useCallback(async (signal?: AbortSignal) => {
    const live = await central.service.accessSession();
    const list = await hostedRequest<{items: HostedServer[]; version?: string; watch?: string}>(central.api.origin, '/v1/servers', undefined, live.accessToken);
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    return {items: list.items, version: list.version ?? '', watch: list.watch ?? ''};
  }, [central]);
  const watcher = useMemo(() => accountId ? new ServerListWatcher({
    hostedOrigin: central.api.origin,
    listServers,
    onServers: answer => { adoptServers(accountId, answer.items as HostedServer[]); },
    storage: serverListStorage(accountId),
  }) : undefined, [accountId, central, listServers, adoptServers]);
  const refreshServers = useCallback(() => {
    const session = central.service.getSnapshot().session;
    if (!session) {
      setServers([]);
      setServersStatus('idle');
      return;
    }
    const gen = ++generation.current;
    setServersStatus('loading');
    setServersError(undefined);
    hostedGate(central.api.origin).run('interactive', () => listServers())
      .then(list => {
        if (gen !== generation.current) return;
        watcher?.adopt(list);
        setListCheckedAt(watcher?.getRecord()?.checkedAt);
        adoptServers(session.account.id, list.items as HostedServer[]);
      })
      .catch(e => {
        if (gen !== generation.current) return;
        setServersStatus('error');
        setServersError(signInText(e, 'account'));
      });
  }, [central, listServers, watcher, adoptServers]);
  useEffect(() => {
    if (!accountId) {
      setServers([]);
      setServersStatus('idle');
      return;
    }
    const kept = cachedServerList(accountId);
    if (kept && watcher?.getRecord()) {
      setServers(kept.map(server => ({...server, presence: parseServerPresence((server as {presence?: unknown}).presence)})));
      setServersStatus('ready');
      setListCheckedAt(watcher.getRecord()?.checkedAt);
      void watcher.foreground().then(() => setListCheckedAt(watcher.getRecord()?.checkedAt), () => {});
    } else refreshServers();
  }, [accountId, watcher, refreshServers]);
  // Foreground only: a tab coming back asks whether the list changed, when that check is due.
  useEffect(() => {
    if (!watcher) return;
    const shown = () => { if (document.visibilityState === 'visible') void watcher.foreground().then(() => setListCheckedAt(watcher.getRecord()?.checkedAt), () => {}); };
    document.addEventListener('visibilitychange', shown);
    return () => document.removeEventListener('visibilitychange', shown);
  }, [watcher]);
  const refreshServerList = useCallback(async (): Promise<ServerListOutcome> => {
    if (!watcher) return 'not-due';
    const outcome = await watcher.refresh();
    setListCheckedAt(watcher.getRecord()?.checkedAt ?? Date.now());
    return outcome;
  }, [watcher]);
  /** A server that refused this account (`access_refused`) leaves the list until it next changes. */
  const dropServer = useCallback((serverId: string) => setDropped(prev => new Set(prev).add(serverId)), []);
  dropServerRef.current = dropServer;

  const commitSelectedViewer = useCallback(
    async (url: string, session: LocalSession, result?: SelectedSession, wasEstablished = false, operation?: {isCurrent: () => boolean; rollbackNative: () => Promise<void>}, preVerified = false) => {
      const candidate = selectedViewerRecord(url, session);
      const bound = session.viewer.authority === 'hosted' ? central.service.getSnapshot().session : undefined;
      const oldServer = attachedServer;
      if (operation?.isCurrent() === false) throw new CodedError('cancelled', 'Profile selection was cancelled.');
      setError('');
      let transferred = false;
      if (result?.isCurrent?.() === false) throw new CodedError('profile_changed', 'The selected profile changed.');
      const publish = (record: SelectedViewerRecord, restored = false) => {
        const local = new HttpLocalApi(record.serverUrl, record.session.accessToken);
        trackBrowserConnection(local);
        refresher.current?.stop();
        refresher.current = new LocalSessionRefresher({
          env: browserConnectionEnvironment(),
          api: local,
          session: record.session,
          onRotated: next => {
            viewer.rotate(next);
            void browserViewerStorage.read().then(saved => saved?.session.sessionFamilyId === next.sessionFamilyId ? browserViewerStorage.write(selectedViewerRecord(record.serverUrl, next)) : undefined).catch(() => {});
          },
          onSignedOut: error => refreshEnded.current(error),
        });
        setApi(local);
        if (!restored) setSystem(undefined);
        setAttachedServer(result?.server ?? (oldServer?.id === record.session.viewer.serverId ? oldServer : undefined));
        setEstablished(wasEstablished);
        viewer.select(local.baseUrl, record.session);
        if (!restored || operation?.isCurrent() !== false) setPending(undefined);
        void local
          .system()
          .then(info => {
            if (viewer.getSnapshot().session?.accessToken === record.session.accessToken) setSystem(info);
            // Direct sign-ins are remembered for the server switcher (no credentials).
            if (record.session.viewer.authority === 'local') void knownServers.remember({serverId: record.session.viewer.serverId, name: info.name, address: record.serverUrl, lastProfileId: record.session.viewer.profileId}).catch(() => {});
          })
          .catch(() => {});
        if (record.session.viewer.authority === 'local')
          void local
            .request<{account: {username: string; hostedAccountId?: string}; custodyPending?: boolean; profiles: {id: string; name: string; art: string; primary: boolean; pinRequired: boolean}[]}>('/v1/direct')
            .then(snap => {
              if (viewer.getSnapshot().session?.accessToken === record.session.accessToken) setLocal({username: snap.account.username, hostedAccountId: snap.account.hostedAccountId, custodyPending: snap.custodyPending === true, profiles: snap.profiles.map(p => ({id: p.id, name: p.name, art: p.art, primary: p.primary, pinRequired: p.pinRequired}))});
              const profile = snap.profiles.find(p => p.id === record.session.viewer.profileId);
              if (profile) void knownServers.setProfile(record.session.viewer.serverId, {id: profile.id, name: profile.name}).catch(() => {});
            })
            .catch(() => {});
      };
      setBusy(true);
      try {
        await commit.commit(candidate, {
          current: () => {
            const current = viewer.getSnapshot();
            return current.session ? selectedViewerRecord(current.serverUrl!, current.session) : null;
          },
          fence: () => {
            transferred = true;
            viewer.clear();
          },
          assertCandidateCurrent: () => {
            if (operation?.isCurrent() === false) throw new CodedError('cancelled', 'Profile selection was cancelled.');
          },
          assertCurrent: () => {
            if (!transferred && result?.isCurrent?.() === false) throw new CodedError('profile_changed', 'The selected profile changed.');
            if (candidate.session.viewer.authority !== 'hosted') return;
            const live = central.service.getSnapshot().session;
            if (wasEstablished) {
              if (live && live.account.id !== candidate.session.viewer.accountId) throw new CodedError('account_session_changed', 'A different Portico Account is now signed in.');
              return;
            }
            if (!bound || live?.account.id !== candidate.session.viewer.accountId || live.familyId !== bound.familyId || (result && result.context.sessionId !== live.familyId)) throw new CodedError('account_session_changed', 'Your Portico Account changed. Choose the profile again.');
          },
          // Launch restore has just checked this exact token's viewer against `/v1/me`
          // (restoreNativeSession); asking again only delayed the first screen.
          verify: (record, signal) => (preVerified && record.session.accessToken === session.accessToken ? Promise.resolve({viewer: session.viewer}) : new HttpLocalApi(record.serverUrl, record.session.accessToken).request('/v1/me', 'GET', undefined, signal)),
          prepareContext: result ? () => central.service.updateContext({profileId: result.context.profileId, server: result.server}) : undefined,
          publish: record => publish(record),
          restore: record => publish(record, true),
          revoke: async record => {
            try {
              if (record.session.accessToken === session.accessToken) await operation?.rollbackNative();
            } finally {
              try { await new HttpLocalApi(record.serverUrl, record.session.accessToken).logout(); }
              finally { await forgetBrowserConnection(record.session); }
            }
          },
          cleanupFailed: () => { if (operation?.isCurrent() !== false) setError('An unused server session could not be ended. It will expire on its own.'); },
        });
      } catch (e) {
        if (import.meta.env.DEV) console.error('[session] commit failed', e);
        if (operation?.isCurrent() !== false) setError(signInText(e, 'profile', hostOf(url)));
        throw e;
      } finally {
        if (operation?.isCurrent() !== false) setBusy(false);
      }
    },
    [attachedServer, central, commit],
  );

  /* Direct sign-in: inspect → confirm fingerprint → credentials → profiles. */
  const inspect = useCallback(async () => {
    if (busy) return;
    setBusy(true);
    setError('');
    const gen = ++generation.current;
    try {
      const found = await inspectDirectServer(directOrigin.trim(), browserConnectionEnvironment(), AbortSignal.timeout(15000));
      if (gen === generation.current) setPin(found);
    } catch (e) {
      setError(signInText(e, 'password', hostOf(directOrigin.trim())));
    } finally {
      setBusy(false);
    }
  }, [busy, directOrigin]);

  const acceptDirect = useCallback(
    async (routed: HttpLocalApi, session: LocalSession, grant?: Partial<DirectPending>, isCurrent: () => boolean = () => true) => {
      if (session.viewer.authority !== 'local') throw new CodedError('invalid_response', 'A direct sign-in profile is required.');
      // C45: selection issues its own family; the account-scoped one from sign-in is retired afterwards.
      const cleanup = grant?.accountGrant && grant.accountSession ? new HttpLocalApi(routed.baseUrl, grant.accountSession.accessToken) : undefined;
      const env = browserConnectionEnvironment(), authority = selectionAuthority.current;
      let previous: Awaited<ReturnType<typeof env.storage.read>>;
      const rollbackNative = () => env.storage.change(record =>
        record?.session.accessToken === session.accessToken
          ? (authority === selectionAuthority.current ? previous : undefined) : record);
      let handedOff = false;
      try {
        previous = await env.storage.read();
        if (!isCurrent()) throw new CodedError('cancelled', 'Profile selection was cancelled.');
        await rememberNativeSession(routed, session, env, grant?.hosted, isCurrent);
        if (!isCurrent()) throw new CodedError('cancelled', 'Profile selection was cancelled.');
        handedOff = true;
        await commitSelectedViewer(routed.baseUrl, session, undefined, false, {isCurrent, rollbackNative});
        if (cleanup) void cleanup.request('/v1/sessions/current', 'DELETE').catch(() => {});
      } catch (error) {
        try { await rollbackNative(); }
        finally {
          if (!handedOff && viewer.getSnapshot().session?.accessToken !== session.accessToken) {
            try { await new HttpLocalApi(routed.baseUrl, session.accessToken).logout(); }
            finally { await forgetBrowserConnection(session); }
          }
        }
        throw error;
      }
    },
    [commitSelectedViewer],
  );

  const pairedApi = useCallback(async () => {
    if (!pin) throw new CodedError('invalid_address', 'Check the server address first.');
    const connected = await connectPairedServer(directOrigin.trim(), pin, browserConnectionEnvironment(), AbortSignal.timeout(15000));
    return connected.api;
  }, [pin, directOrigin]);

  /** Selects a profile from a sign-in's profile list (the chooser, or a silent re-admission). */
  const selectProfile = useCallback(
    async (pending: DirectPending, profileId: string, options?: {pin?: string; trust?: boolean}) => {
      // WEB-08: cancelling or replacing a selection must invalidate the one in
      // flight. Without this fence a response that arrives after Cancel still
      // persisted trust and committed the viewer, so cancelling did not mean
      // staying where you were.
      invalidateSelection();
      const gen = selection.current;
      const current = () => gen === selection.current;
      const controller = new AbortController();
      selectionAbort.current = controller;
      setBusy(true);
      setError('');
      const snapshot = pending.snapshot;
      const serverId = snapshot?.serverId ?? viewer.getSnapshot().session?.viewer.serverId ?? pending.api.getRouteConnection()?.pin.serverId ?? '';
      const accountId = snapshot?.account.id ?? viewer.getSnapshot().session?.viewer.accountId ?? '';
      const scopeKey = {authority: 'local' as const, accountId, serverId, profileId, installationId: browserInstallation()};
      try {
        await browserTrustLoaded();
        const remembered = scopeKey.installationId ? readBrowserTrust(scopeKey) : undefined;
        const chosen = await selectDirectProfile(pending.api, {authority: 'local', accountId, serverId}, profileId, {pin: options?.pin, trust: options?.trust, installationId: scopeKey.installationId, ...(!options?.pin && remembered ? {trustToken: remembered.token} : {})}, controller.signal);
        if (!current()) {
          // The issuance won the race with cancellation. Nothing is persisted
          // and nothing is selected; the credential nobody asked for is ended.
          // Only this new session is retired — never an already selected one.
          if (viewer.getSnapshot().session?.accessToken !== chosen.session.accessToken) void new HttpLocalApi(pending.api.baseUrl, chosen.session.accessToken).logout().catch(() => {});
          return;
        }
        await acceptDirect(pending.api, chosen.session, pending, current);
        if (current() && chosen.trustedSelection) {
          try { saveBrowserTrust(chosen.trustedSelection); } catch {}
        }
      } catch (e) {
        if (!current()) return;
        if (['profile_pin_required', 'profile_pin_locked', 'profile_pin'].includes((e as {code?: string})?.code ?? '')) forgetBrowserTrust(scopeKey);
        setError(signInText(e, 'profile', hostOf(pending.api.baseUrl)));
      } finally {
        // A superseded attempt must not clear the replacement's busy state.
        if (current()) setBusy(false);
      }
    },
    [acceptDirect, invalidateSelection],
  );

  /**
   * C45: what a direct sign-in answer leads to. A viewer session is accepted; an account-scoped one
   * opens the profile chooser; a challenge asks for a code; a required password change asks for a new
   * password; a device waiting for approval says so. Returns 'failed' when nothing was kept.
   * A Portico sign-in (`extra.again`) keeps its Hosted route source, repeats its identity sign-in
   * instead of a password, and may pick up the profile it had before (`preferProfileId`, silent
   * re-admission) when that needs no PIN here.
   */
  const finishDirectSignIn = useCallback(async (connected: Awaited<ReturnType<typeof connectPairedServer>>, result: DirectSignIn, username: string, password: string, isCurrent: () => boolean, extra: {again?: () => Promise<DirectSignIn>; hosted?: RememberedServer['hosted']; preferProfileId?: string} = {}): Promise<'done' | 'step' | 'failed'> => {
    const local = connected.api;
    const {again, hosted, preferProfileId} = extra;
    if (result.kind === 'challenge') {
      stepRef.current = {connected, username, password, token: result.token, again, hosted};
      setDirectStep({kind: 'code'});
      return 'step';
    }
    if (result.kind === 'device-pending') {
      stepRef.current = {connected, username, password, again, hosted};
      setDirectStep({kind: 'device-pending'});
      return 'step';
    }
    const {snapshot, session} = result;
    setLocal({username: snapshot.account.username, hostedAccountId: snapshot.account.hostedAccountId, custodyPending: snapshot.custodyPending === true, profiles: snapshot.profiles.map(p => ({id: p.id, name: p.name, art: p.art, primary: p.primary, pinRequired: p.pinRequired}))});
    if (result.passwordChangeRequired) {
      stepRef.current = {connected, username, password, session, again, hosted};
      setDirectStep({kind: 'new-password'});
      return 'step';
    }
    stepRef.current = undefined;
    setDirectStep(undefined);
    if (result.accountScoped) {
      local.setAccessToken(session.accessToken);
      const next: DirectPending = {api: local, snapshot, accountGrant: true, accountSession: session, hosted};
      setPending(next);
      const preferred = preferProfileId ? snapshot.profiles.find(p => p.id === preferProfileId) : undefined;
      if (preferred) await browserTrustLoaded();
      const trusted = preferred && readBrowserTrust({authority: 'local', accountId: snapshot.account.id, serverId: snapshot.serverId, profileId: preferred.id, installationId: browserInstallation()});
      if (preferred && (!preferred.pinRequired || trusted)) await selectProfile(next, preferred.id);
      return 'done';
    }
    // One unprotected profile: this session is the viewer. Never end it as an "account session".
    await rememberNativeSession(local, session, browserConnectionEnvironment(), hosted, isCurrent);
    if (!isCurrent()) throw superseded();
    await acceptDirect(local, session, {hosted});
    return 'done';
  }, [acceptDirect, selectProfile]);

  const continueSignIn = useCallback(async (value: string) => {
    const step = stepRef.current;
    if (!step || busy) return;
    setBusy(true);
    setError('');
    const gen = generation.current;
    try {
      let result: DirectSignIn;
      if (directStep?.kind === 'code' && step.token) {
        result = await completeDirectSignInChallenge(step.connected.api, step.token, value);
      } else if (directStep?.kind === 'new-password' && step.session) {
        await changeRequiredPassword(new HttpLocalApi(step.connected.api.baseUrl, step.session.accessToken), step.password, value);
        // After the change, sign in again with the new password.
        result = await directSignIn(step.connected.api, step.username, value);
        step.password = value;
      } else if (directStep?.kind === 'device-pending') {
        result = step.again ? await step.again() : await directSignIn(step.connected.api, step.username, step.password);
      } else return;
      await finishDirectSignIn(step.connected, result, step.username, step.password, () => gen === generation.current, {again: step.again, hosted: step.hosted});
    } catch (e) {
      setError(signInText(e, directStep?.kind === 'code' ? 'code' : directStep?.kind === 'new-password' ? 'new-password' : step.again ? 'portico' : 'password', hostOf(step.connected.api.baseUrl)));
    } finally {
      setBusy(false);
    }
  }, [busy, directStep, finishDirectSignIn]);

  const cancelStep = useCallback(() => {
    const step = stepRef.current;
    stepRef.current = undefined;
    setDirectStep(undefined);
    if (step?.session) void new HttpLocalApi(step.connected.api.baseUrl, step.session.accessToken).request('/v1/sessions/current', 'DELETE').catch(() => {});
    step?.connected.connection.dispose();
  }, []);

  const signInDirect = useCallback(
    async (username: string, password: string, at?: string, dev?: boolean) => {
      if (busy) return;
      setBusy(true);
      setError('');
      const gen = pin ? generation.current : ++generation.current;
      let connected: Awaited<ReturnType<typeof connectPairedServer>> | undefined;
      let issued: LocalSession | undefined;
      let offered = false;
      try {
        // One page, one button: the server is looked up and its identity pinned as part of signing in.
        // `at`: sign in at this address now (a just-accepted invitation), not the typed one.
        if (at) setDirectOrigin(at);
        const origin = serverAddress(at ?? directOrigin);
        let identity = at ? undefined : pin;
        if (!identity) {
          identity = await inspectDirectServer(origin, browserConnectionEnvironment(), AbortSignal.timeout(15000));
          if (gen !== generation.current) return;
          setPin(identity);
        }
        connected = await connectPairedServer(origin, identity, browserConnectionEnvironment(), AbortSignal.timeout(15000));
        const local = connected.api;
        const info = await local.system();
        if (info.setupRequired) throw new CodedError('setup_required', 'This server isn’t set up yet.');
        const result = import.meta.env.DEV && dev ? await devE2ESignIn(local, username) : await directSignIn(local, username, password);
        if (gen !== generation.current) throw superseded();
        const outcome = await finishDirectSignIn(connected, result, username, password, () => gen === generation.current);
        offered = outcome !== 'failed';
      } catch (e) {
        if (gen === generation.current) setError(signInText(e, 'password', hostOf(at ?? serverAddress(directOrigin))));
      } finally {
        if (!offered && connected) {
          if (issued) {
            connected.api.setAccessToken(issued.accessToken);
            try {
              await connected.api.logout();
            } catch {}
          }
          connected.connection.dispose();
        }
        setBusy(false);
      }
    },
    [busy, pin, directOrigin, acceptDirect],
  );

  const chooseProfile = useCallback(
    async (profileId: string, options?: {pin?: string; trust?: boolean}) => {
      if (pending) await selectProfile(pending, profileId, options);
    },
    [pending, selectProfile],
  );

  const cancelPending = useCallback(() => {
    // Invalidate before anything else: a selection already in flight must not be
    // able to publish state, persist trust or commit a viewer after this point.
    invalidateSelection();
    setBusy(false);
    if (pending?.accountGrant && pending.accountSession) void new HttpLocalApi(pending.api.baseUrl, pending.accountSession.accessToken).request('/v1/sessions/current', 'DELETE').catch(() => {});
    setPending(undefined);
  }, [pending, invalidateSelection]);

  /* The switcher reads the account's authoritative profile snapshot; an already
   * selected viewer stays attached until a new selection is confirmed. */
  const readDirectProfiles = useCallback((api: HttpLocalApi, accountId: string, serverId: string) => {
    const gen = ++chooserGeneration.current;
    setPending(p => p?.api === api ? {...p, snapshot: undefined, profilesError: undefined} : p);
    api.request<unknown>('/v1/direct', 'GET').then(raw => {
      if (gen !== chooserGeneration.current) return;
      const snapshot = parseDirectSnapshot(raw, {authority: 'local', accountId, serverId});
      setPending(p => p?.api === api ? {...p, snapshot, profilesError: undefined} : p);
    }).catch(e => {
      if (gen !== chooserGeneration.current) return;
      setPending(p => p?.api === api ? {...p, profilesError: signInText(e, 'profile', hostOf(api.baseUrl))} : p);
    });
  }, []);

  const openProfileChooser = useCallback(() => {
    invalidateSelection();
    const current = viewer.getSnapshot();
    if (current.session?.viewer.authority !== 'local' || !current.serverUrl) return;
    const api = new HttpLocalApi(current.serverUrl, current.session.accessToken);
    setPending({api, accountGrant: false});
    void readDirectProfiles(api, current.session.viewer.accountId, current.session.viewer.serverId);
  }, [readDirectProfiles, invalidateSelection]);

  const reloadProfiles = useCallback(() => {
    if (!pending || pending.accountGrant) return;
    const live = viewer.getSnapshot().session;
    if (!live) return;
    void readDirectProfiles(pending.api, live.viewer.accountId, live.viewer.serverId);
  }, [pending, readDirectProfiles]);

  const acceptIssued = useCallback(
    async (routed: HttpLocalApi, session: LocalSession) => {
      routed.setAccessToken(session.accessToken);
      await routed.me();
      await rememberNativeSession(routed, session, browserConnectionEnvironment());
      trackBrowserConnection(routed);
      await commitSelectedViewer(routed.baseUrl, session);
    },
    [commitSelectedViewer],
  );

  /* Portico Account members (Spec — Hosted at Scale): Hosted proves who the person is; the server
     decides and answers exactly as for a password (profiles, PIN, device approval). */
  const hostedApi = useMemo<HostedRequestApi>(() => ({
    // Failures here are the Portico Account service's, never said as the server's (INT gate 3).
    request: async <T,>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal) => {
      try {
        const live = await central.service.accessSession();
        return await central.api.send<T>(path, method, body, signal, live.accessToken);
      } catch (e) { throw accountFailure(e); }
    },
  }), [central]);
  const porticoConnect = useCallback(async (server: HostedServer, options: {kind?: 'interactive' | 'automatic'; preferProfileId?: string} = {}): Promise<PorticoOutcome> => {
    const live = central.service.getSnapshot().session;
    if (!live) { setError(currentI18n().t('web.portico.signInFirst')); return 'failed'; }
    const kind = options.kind ?? 'interactive';
    const env = browserConnectionEnvironment();
    const request = async <T,>(path: string, signal: AbortSignal): Promise<T> => hostedApi.request<T>(path, 'GET', undefined, signal);
    const gen = ++generation.current;
    setBusy(true);
    setError('');
    let connected: Awaited<ReturnType<typeof connectHostedServer>> | undefined;
    let kept = false;
    try {
      const signal = AbortSignal.timeout(30000);
      try {
        connected = await connectHostedServer(server, {origin: central.api.origin, accountId: live.account.id, request}, env, signal);
      } catch (e) {
        // Hosted answers 403 for the routes of a server this account is no longer listed on.
        const status = (e as {status?: unknown} | null)?.status, code = (e as {code?: unknown} | null)?.code;
        if (status === 403 || code === 'access_denied') throw Object.assign(new Error('This Portico Account is no longer listed on that server.'), {status: 403, code: 'access_refused', hostedRefusal: true});
        throw e;
      }
      const api = connected.api;
      const again = () => porticoSignIn(hostedApi, central.api.origin, api, server.id, {kind});
      const result = await again();
      if (gen !== generation.current) throw superseded();
      const outcome = await finishDirectSignIn(connected, result, live.account.username, '', () => gen === generation.current, {again, hosted: connected.hosted, preferProfileId: options.preferProfileId});
      kept = outcome !== 'failed';
      if (kept) rememberLastServer(live.account.id, server.id);
      return kept ? 'done' : 'failed';
    } catch (e) {
      if (kind === 'automatic') throw e;
      // Refused by the server (access_refused), or by Hosted before that (its routes answer 403 for
      // a server this account is no longer listed on). Either way the kept list is stale: drop the
      // server and check the list now.
      if (isAccessRefused(e) || (e as {hostedRefusal?: unknown} | null)?.hostedRefusal === true) {
        dropServer(server.id);
        setError(currentI18n().t('web.portico.noAccess', {server: server.name}));
        void watcher?.refresh().then(() => setListCheckedAt(watcher.getRecord()?.checkedAt), () => {});
        return 'refused';
      }
      const failure = signInFailure(e, {step: 'portico', server: server.name, online: navigator.onLine}, currentI18n());
      if (failure.silent) return 'failed';
      setError(failure.text);
      return failure.wait === 'server' ? 'unreachable' : failure.wait === 'account' ? 'account-unavailable' : 'failed';
    } finally {
      if (!kept && connected) connected.connection.dispose();
      setBusy(false);
    }
  }, [central, hostedApi, finishDirectSignIn, dropServer, watcher]);

  /* `/join#code=…` as a Portico Account: the inviting server checks the code, makes the membership
     and answers with the sign-in, so this device has the server at once. */
  const acceptPortico = useCallback(async (origin: string, code: string): Promise<boolean> => {
    const live = central.service.getSnapshot().session;
    if (!live) { setError(currentI18n().t('web.portico.signInFirst')); return false; }
    const gen = ++generation.current;
    setBusy(true);
    setError('');
    let connected: Awaited<ReturnType<typeof connectPairedServer>> | undefined;
    let kept = false;
    try {
      const address = serverAddress(origin);
      const identity = await inspectDirectServer(address, browserConnectionEnvironment(), AbortSignal.timeout(15000));
      connected = await connectPairedServer(address, identity, browserConnectionEnvironment(), AbortSignal.timeout(15000));
      const api = connected.api;
      setDirectOrigin(address);
      setPin(identity);
      const result = await acceptPorticoInvitation(hostedApi, central.api.origin, api, identity.serverId, code);
      if (gen !== generation.current) throw superseded();
      const again = () => porticoSignIn(hostedApi, central.api.origin, api, identity.serverId);
      const outcome = await finishDirectSignIn(connected, result, live.account.username, '', () => gen === generation.current, {again});
      kept = outcome !== 'failed';
      // The server tells Hosted; the account's other devices learn at their next check.
      if (kept) void watcher?.refresh().then(() => setListCheckedAt(watcher.getRecord()?.checkedAt), () => {});
      return kept;
    } catch (e) {
      setError(signInText(e, 'invitation', hostOf(serverAddress(origin))));
      throw e;
    } finally {
      if (!kept && connected) connected.connection.dispose();
      setBusy(false);
    }
  }, [central, hostedApi, finishDirectSignIn, watcher]);

  /* INT M6: an owner who is a Portico Account accepts Hosted's custody of the server's registration
     (after ownership moved to them). The flag clears once the server's next membership push lands,
     within seconds, so the snapshot is read again a few times. */
  const acceptCustody = useCallback(async (): Promise<void> => {
    const current = viewer.getSnapshot();
    if (!current.session || !current.serverUrl) return;
    const local = api;
    try {
      await acceptPorticoCustody(hostedApi, central.api.origin, local, current.session.viewer.serverId);
    } catch (e) {
      throw new Error(signInText(e, 'custody', current.session.viewer.serverId ? (servers.find(x => x.id === current.session!.viewer.serverId)?.name ?? hostOf(current.serverUrl)) : undefined));
    }
    for (const wait of [1500, 3000, 6000]) {
      await new Promise(r => setTimeout(r, wait));
      if (viewer.getSnapshot().session?.accessToken !== current.session.accessToken) return;
      const snap = await local.request<{custodyPending?: boolean}>('/v1/direct').catch(() => undefined);
      if (snap && snap.custodyPending !== true) { setLocal(prev => (prev ? {...prev, custodyPending: false} : prev)); return; }
    }
  }, [api, hostedApi, central, servers]);

  /* A session the server's move to its own membership ended (migration 0120, `session_migrated`):
     re-admit silently with the Portico Account and the same profile. Only a refusal, or no Portico
     Account session, opens the sign-in screen. */
  const readmit = useCallback(async (serverUrl: string, session: LocalSession): Promise<void> => {
    const live = await central.service.accessSession().catch(() => undefined);
    try {
      if (!live) throw new Error('No Portico Account session.');
      const list = await hostedGate(central.api.origin).run('automatic', () => listServers(), 'server-list');
      watcher?.adopt(list);
      adoptServers(live.account.id, list.items as HostedServer[]);
      const server = (list.items as HostedServer[]).find(x => x.id === session.viewer.serverId);
      if (!server) throw new Error('This server is no longer in your list.');
      if ((await porticoConnect(server, {kind: 'automatic', preferProfileId: session.viewer.profileId})) !== 'done') throw new Error('Not re-admitted.');
      setError('');
      // The profile needs its PIN here: the old viewer steps aside and the profile chooser shows.
      if (viewer.getSnapshot().session?.sessionFamilyId === session.sessionFamilyId) { stopRefresh(); viewer.clear(); setApi(idle()); }
    } catch (e) {
      if (isAccessRefused(e) || (e as {hostedRefusal?: unknown} | null)?.hostedRefusal === true) dropServer(session.viewer.serverId);
      signInEnded.current(serverUrl, session, undefined, true);
    }
  }, [central, listServers, watcher, adoptServers, porticoConnect, stopRefresh, dropServer]);
  readmitRef.current = readmit;

  const disconnectServer = useCallback(() => {
    invalidateSelection(true);
    ++generation.current;
    restoreControl.current.cancel();
    const current = viewer.getSnapshot();
    stopRefresh();
    const unverified = offlineRef.current?.record.session;
    setOffline(undefined);
    restoreControl.current.cancel();
    if (unverified && !current.session) void forgetBrowserConnection(unverified).catch(() => {});
    viewer.clear();
    setPending(undefined);
    setPin(undefined);
    setLocal(undefined);
    setEstablished(false);
    setAttachedServer(undefined);
    setSystem(undefined);
    setApi(idle());
    void commit.clear().catch(() => setError('Server selection could not be cleared from this tab.'));
    if (current.session) {
      void forgetBrowserConnection(current.session).catch(() => {});
      void new HttpLocalApi(current.serverUrl!, current.session.accessToken).logout().catch(() => {});
      if (current.session.viewer.authority === 'hosted') void central.service.updateContext({}).catch(() => {});
    }
  }, [central, commit, invalidateSelection, stopRefresh]);
  /* A Portico Account member's server sign-in belongs to that Portico Account: when another one is
     signed in here, it ends, and the new account lands in the shell (app/account-switch.ts). */
  const signedInAccount = account.session?.account.id;
  useEffect(() => {
    const current = scope.session;
    if (scope.phase !== 'ready' || !current || current.viewer.authority !== 'local' || !signedInAccount) return;
    let live = true;
    void (async () => {
      const owner = local?.hostedAccountId ?? (await browserConnectionEnvironment().storage.read().catch(() => undefined))?.hosted?.accountId;
      if (!live || viewer.getSnapshot().session?.accessToken !== current.accessToken) return;
      if (belongsToAnotherAccount(owner, signedInAccount)) disconnectServer();
    })();
    return () => { live = false; };
  }, [scope, signedInAccount, local?.hostedAccountId, disconnectServer]);
  signInEnded.current = (serverUrl, session, username, portico) => {
    const hint = signInHintFor(serverUrl, session, username, portico);
    disconnectServer();
    void forgetBrowserConnection(session).catch(() => {});
    if (hint.authority === 'local') setDirectOrigin(serverUrl);
    setSignInHint(hint);
  };
  // The server refused this device's sign-in (revoked, reused, device removed): the protected store is already cleared.
  refreshEnded.current = error => {
    const current = viewer.getSnapshot();
    if (current.session && current.serverUrl) {
      const portico = current.session.viewer.authority === 'hosted' || !!local?.hostedAccountId;
      // A Portico Account member whose session the server migrated is re-admitted, not signed out.
      if (portico && isSessionMigrated(error)) { void readmitRef.current(current.serverUrl, current.session); return; }
      signInEnded.current(current.serverUrl, current.session, portico ? undefined : local?.username, portico);
    } else { disconnectServer(); setError('Your sign-in has ended. Sign in again.'); }
  };
  useEffect(() => { if (scope.phase === 'ready') setSignInHint(undefined); }, [scope.phase]);

  const signOutAccount = useCallback(async () => {
    try {
      await central.service.logout();
    } catch (e) {
      setError(message(e, 'Sign-out could not be saved.'));
    }
  }, [central]);

  const signOut = useCallback(() => {
    forgetRailCache(sessionIdentity(viewer.getSnapshot().session) || undefined);
    if (viewer.getSnapshot().session?.viewer.authority === 'hosted' || central.service.getSnapshot().session) void central.service.logout().catch(() => {});
    disconnectServer();
  }, [central, disconnectServer]);

  const value = useMemo<SessionValue>(
    () => ({
      phase: restoring ? 'restoring' : scope.phase === 'ready' || (offline && !scope.session) ? 'ready' : 'signedOut',
      restoreMessage,
      signInHint,
      dismissSignInHint: () => setSignInHint(undefined),
      serverProblem: offline && !scope.session ? {kind: offline.kind, serverId: offline.record.session.viewer.serverId, serverUrl: offline.record.serverUrl} : undefined,
      api,
      serverUrl: scope.serverUrl ?? (offline ? offline.record.serverUrl : undefined),
      session: scope.session ?? offline?.record.session,
      system,
      owner: scope.session?.viewer.role === 'owner',
      hostedUrl: central.api.origin,
      account,
      hosted: account.session,
      local,
      servers: dropped.size ? servers.filter(x => !dropped.has(x.id)) : servers,
      serversStatus,
      serversError,
      refreshServers,
      attachedServer,
      error,
      busy,
      clearError: () => setError(''),
      cancelRestore: () => restoreControl.current.cancel(),
      retryRestore: () => restoreControl.current.retry(),
      direct: {
        origin: directOrigin,
        setOrigin: v => {
          invalidateSelection(true);
          setDirectOrigin(v);
          setPin(undefined);
          ++generation.current;
        },
        pin,
        inspect,
        resetPin: () => { invalidateSelection(true); setPin(undefined); },
        signIn: signInDirect,
        step: directStep,
        continueSignIn,
        cancelStep,
        pending,
        chooseProfile,
        cancelPending,
        openProfileChooser,
        reloadProfiles,
        pairedApi,
        acceptIssued,
      },
      portico: {connect: server => porticoConnect(server), acceptInvitation: acceptPortico, refreshServers: refreshServerList, checkedAt: listCheckedAt, acceptCustody, lastServerId: accountId ? lastServer(accountId) : undefined},
      accountSignedIn: refreshServers,
      signOutAccount,
      signOut,
      disconnectServer,
    }),
    [restoring, restoreMessage, signInHint, offline, scope, api, system, central, account, local, servers, dropped, serversStatus, serversError, refreshServers, attachedServer, error, busy, directOrigin, pin, inspect, signInDirect, directStep, continueSignIn, cancelStep, pending, chooseProfile, cancelPending, openProfileChooser, reloadProfiles, pairedApi, acceptIssued, porticoConnect, acceptPortico, refreshServerList, listCheckedAt, acceptCustody, signOutAccount, signOut, disconnectServer, invalidateSelection],
  );
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

/** Development only: mounts screens against a canned API with no sign-in, for the feature
 * gallery. Never reachable in a production build (its route is not registered). */
export function SessionFixture({value, children}: {value: Partial<SessionValue>; children: React.ReactNode}) {
  return <SessionContext.Provider value={value as SessionValue}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession must be used inside SessionProvider.');
  return value;
}
