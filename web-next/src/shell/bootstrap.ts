import type {HttpLocalApi, LocalSession} from '@core/index.ts';
import {parseLibraryList} from '@core/library-list.ts';
import {LocalSessionRefresher} from '@core/session-refresh.ts';
import {selectedViewerRecord} from '@core/viewer-selection.ts';
import {PreferencesReader} from '@core/preferences-reader.ts';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import {parseDirectSnapshot} from '@core/profile-management.ts';
import {HomeService} from '@core/home-service.ts';
import {InboxService} from '@core/inbox.ts';
import {EventsClient} from '@core/playback-v1/events.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {runEventFeed} from '@core/event-feed.ts';
import {fetchGuideSourceSummaries} from '@core/guide/directory.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {browserViewerStorage} from '../platform/viewer-selection-storage';
import {parseServerSignIn,signInLabelKey} from '@core/sign-in.ts';
import {defaultI18n} from '@i18n';
import {browserAppSessionActions,signOutBrowserPorticoAccount,type BrowserAppSessionActions} from '../platform/app-sessions';
import {browserConnectionEnvironment, restoreBrowserConnection, trackBrowserConnection, forgetBrowserConnection} from '../platform/server-connections';
import {browserEventStream, browserFeedEnvironment} from '../platform/event-feed';
import {freshnessFor, type FreshnessRuntime} from '../platform/freshness';
import {authenticatedArtwork, type ShellArtwork} from './artwork';
import type {ShellBindings} from './bindings';
import type {ShellIdentity} from './Shell';

export type ShellFacts = Readonly<{identity: ShellIdentity; preferences: PreferenceSnapshot; serverName: string}>;
export type ShellAssemblyContext = Readonly<{
  api: HttpLocalApi; session: () => LocalSession; auth: LocalSessionRefresher; events: EventsClient;
  freshness: FreshnessRuntime; home: HomeService; inbox: InboxService; artwork: ShellArtwork;
  facts: ShellFacts; epoch: number; viewerFence: string; isCurrent: () => boolean;
  signOut: () => void; signOutPorticoAccount?: () => void; switchProfile:(profileId:string,pin:string,signal:AbortSignal)=>Promise<void>; switchServer:(serverId:string)=>Promise<void>; savedServers:readonly {id:string;label:string}[];
}>;
export type ShellAssembly = Readonly<{
  getBinding: () => ShellBindings;
  subscribe: (listener: () => void) => () => void;
  /** Real playback decides whether the background event feed must remain open. */
  busy: () => boolean; onBusy: (listener: () => void) => () => void;
  dispose: () => void;
}>;
export type ShellAssembler = (context: ShellAssemblyContext) => Promise<ShellAssembly>;
export type ShellBootstrapSnapshot = Readonly<{phase: 'restoring' | 'signed-out' | 'ready' | 'error'; binding?: ShellBindings; error?: unknown}>;

function viewerId(session: LocalSession): string { const v = session.viewer; return JSON.stringify([v.authority, v.accountId, v.profileId]); }
export function sameShellAuthority(a: LocalSession, b: LocalSession): boolean {
  return a.sessionFamilyId === b.sessionFamilyId && (['serverId', 'authority', 'accountId', 'profileId', 'role'] as const).every(key => a.viewer[key] === b.viewer[key]);
}

type ShellAccount = Pick<ShellIdentity, 'username' | 'profileName' | 'profileArt' | 'primaryProfile' | 'profiles' | 'canManageProfiles' | 'accountFacts' | 'capabilities'>;
export async function currentShellProfile(api: HttpLocalApi, session: LocalSession, signal: AbortSignal): Promise<ShellAccount> {
  const v = session.viewer;
  if (v.authority !== 'local') throw new Error('Profiles must come from this server.');
  {
    const snapshot = parseDirectSnapshot(await api.request(routePath('get_direct_account'), 'GET', undefined, signal), {authority: 'local', accountId: v.accountId, serverId: v.serverId}, true);
    const profile = snapshot.profiles.find(candidate => candidate.id === v.profileId);
    if (!profile) throw new Error('The selected profile is no longer present.');
    if (!profile.primary && Object.values(snapshot.capabilities).some(Boolean)) throw new Error('Management is available only on the main profile.');
    return {capabilities: snapshot.capabilities, username: snapshot.account.username, profileName: profile.name, profileArt: profile.art, primaryProfile: profile.primary, canManageProfiles: snapshot.canManage,
      profiles: snapshot.profiles.map(value => ({id: value.id, name: value.name, initial: Array.from(value.name.trim())[0] ?? '', art: value.art, pinRequired: value.pinRequired})),
      accountFacts: []};
  }

}
async function readFacts(api: HttpLocalApi, session: LocalSession, preferences: PreferencesReader, inbox: InboxService, signal: AbortSignal): Promise<ShellFacts> {
  const [libraries, profile, settings, system, guide] = await Promise.all([
    api.request(routePath('get_libraries'), 'GET', undefined, signal).then(parseLibraryList), currentShellProfile(api, session, signal), preferences.preferences('web'), api.request(routePath('get_system'), 'GET', undefined, signal).then(parseServerSignIn),
    fetchGuideSourceSummaries(api, session.viewer.serverId, Intl.DateTimeFormat().resolvedOptions().timeZone, signal).catch(error => {
      if ((error as {status?: number}).status === 403) return undefined;
      throw error;
    }),
  ]);
  if (system.id !== session.viewer.serverId) throw new Error('The server identity changed.');
  await inbox.load();
  signal.throwIfAborted();
  return {identity: {viewer: session.viewer, libraries: libraries.items, serverName: system.name, signIn:system.signIn, ...profile,
    accountFacts:[{id:'sign-in',role:'status',text:defaultI18n.t(signInLabelKey(system.signIn))}],
    liveAvailable: !!guide?.sources.length, unread: inbox.getSnapshot().unread, showNotificationDot: settings.effective['notifications.badges'] === true}, preferences: settings, serverName: system.name};
}

/** Restores only genuine protected state. No credentials or business data appear in source. */
export class ShellBootstrap {
  private state: ShellBootstrapSnapshot = Object.freeze({phase: 'restoring'});
  private readonly listeners = new Set<() => void>();
  private epoch = 0;
  private controller?: AbortController;
  private retire?: () => void;
  private running?: Promise<void>;
  constructor(private readonly assemble: ShellAssembler, private readonly environment: {
    selected: Pick<typeof browserViewerStorage, 'read'>;
    restore: typeof restoreBrowserConnection;
    actions?: () => BrowserAppSessionActions;
  } = {selected: browserViewerStorage, restore: restoreBrowserConnection}) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(next: ShellBootstrapSnapshot) { this.state = Object.freeze(next); for (const listener of this.listeners) listener(); }
  private fence() {
    ++this.epoch; this.controller?.abort(); this.controller = undefined;
    this.retire?.(); this.retire = undefined;
  }
  private restart = () => {
    this.fence(); this.publish({phase: 'restoring'});
    const pending = this.running, expected = this.epoch;
    void Promise.resolve(pending).then(() => { if (this.epoch === expected && this.state.phase === 'restoring') return this.start(); });
  };
  start = (): Promise<void> => {
    if (this.running) return this.running;
    this.fence(); this.publish({phase: 'restoring'});
    const epoch = this.epoch, controller = this.controller = new AbortController();
    const current = () => this.epoch === epoch && !controller.signal.aborted;
    const task = this.restore(epoch, controller.signal, current).catch(error => {
      if (!current()) return;
      this.fence(); this.publish({phase: 'error', error});
    }).finally(() => { if (this.running === task) this.running = undefined; });
    this.running = task; return task;
  };
  private async restore(epoch: number, signal: AbortSignal, isCurrent: () => boolean): Promise<void> {
    const selected = await this.environment.selected.read();
    if (!isCurrent()) return;
    if (!selected) { this.publish({phase: 'signed-out'}); return; }
    const restored = await this.environment.restore(selected.session);
    if (!isCurrent()) { restored?.api.getRouteConnection()?.dispose(); return; }
    if (!restored) { this.publish({phase: 'signed-out'}); return; }
    const {api} = restored;
    let session = selectedViewerRecord(api.baseUrl, restored.session).session;
    if (session.viewer.role === 'account') { this.publish({phase: 'signed-out'}); api.getRouteConnection()?.dispose(); return; }
    trackBrowserConnection(api);
    const cleanups: (() => void)[] = [];
    const close = () => { for (const cleanup of cleanups.splice(0).reverse()) cleanup(); };
    this.retire = close;
    let signingOut = false;
    cleanups.push(() => { if (!signingOut) api.getRouteConnection()?.dispose(); });
    const signOut = () => {
      if (!isCurrent()) return;
      const captured = session;
      signingOut = true;
      this.fence(); this.publish({phase: 'signed-out'});
      // Remove the exact local credential before remote cleanup; switching never signs out peers.
      void (this.environment.actions?.()??browserAppSessionActions()).signOutServer(captured).catch(error=>this.publish({phase:'error',error})).finally(()=>api.getRouteConnection()?.dispose());
    };
    const auth = new LocalSessionRefresher({api, env: browserConnectionEnvironment(session.viewer.serverId), session,
      onRotated: next => {
        if (!isCurrent()) return;
        const authorityChanged = !sameShellAuthority(session, next); session = next;
        if (authorityChanged) this.restart();
      },
      onSignedOut: () => {
        if (!isCurrent()) return;
        const captured = session;
        this.fence(); this.publish({phase: 'signed-out'});
        void browserViewerStorage.clearIfFamily?.(captured.sessionFamilyId).catch(() => {});
      },
    });
    cleanups.push(() => auth.stop());
    const events = new EventsClient({http: localV1Http(api), openStream: browserEventStream(api, session.viewer.serverId + ':' + viewerId(session), () => session.accessToken)});
    cleanups.push(() => events.stop());
    const freshness = freshnessFor(api, {serverId: session.viewer.serverId, viewerId: viewerId(session)});
    cleanups.push(() => { freshness.pool.dispose(); freshness.hub.dispose(); });
    const preferences = new PreferencesReader(api, {serverId: session.viewer.serverId, viewerId: viewerId(session)});
    cleanups.push(() => preferences.dispose());
    const home = new HomeService(api), inbox = new InboxService(api);
    cleanups.push(() => { home.dispose(); inbox.dispose(); });
    const facts = await readFacts(api, session, preferences, inbox, signal);
    if (!isCurrent()) return;
    await home.load();
    if (!isCurrent()) return;
    const document = home.getSnapshot().document;
    if (!document) throw home.getSnapshot().error ?? new Error('Home could not be loaded.');
    const authorityKey = JSON.stringify([session.sessionFamilyId, session.viewer.role, document.viewerFence]);
    const artwork = authenticatedArtwork({api, auth, identity: {serverId: session.viewer.serverId, viewerId: viewerId(session), authorityKey}, currentToken: () => session.accessToken, isCurrent});
    cleanups.push(() => artwork.dispose());
    const signOutPorticoAccount=facts.identity.signIn==='portico'?()=>{if(!isCurrent())return;signingOut=true;this.fence();this.publish({phase:'signed-out'});void signOutBrowserPorticoAccount().catch(error=>this.publish({phase:'error',error})).finally(()=>api.getRouteConnection()?.dispose());}:undefined;
    const switchProfile=async(profileId:string,pin:string,signal:AbortSignal)=>{if(!isCurrent())throw new Error('The viewer changed.');await (this.environment.actions?.()??browserAppSessionActions()).switchProfile(api,session,profileId,pin,signal,isCurrent);if(isCurrent())this.restart();};
    const savedServers=(await (this.environment.actions?.()??browserAppSessionActions()).store.read()).servers.map(value=>({id:value.connection.pin.serverId,label:value.connection.pin.serverId===session.viewer.serverId?facts.serverName:new URL(value.connection.logicalOrigin).host}));
    const assembly = await this.assemble({api, session: () => session, auth, events, freshness, home, inbox, artwork, facts, epoch, viewerFence: document.viewerFence, isCurrent, signOut,signOutPorticoAccount,switchProfile,switchServer:this.selectServer,savedServers});
    if (!isCurrent()) { assembly.dispose(); return; }
    cleanups.push(() => assembly.dispose());
    cleanups.push(runEventFeed(events, freshness.hub, browserFeedEnvironment(assembly.busy, assembly.onBusy)));
    const publishBinding = () => {
      if (!isCurrent()) return;
      const binding = assembly.getBinding();
      this.publish({phase: 'ready', binding: {...binding, shell: {...binding.shell,
        identity: {...binding.shell.identity, unread: inbox.getSnapshot().unread}}}});
    };
    cleanups.push(inbox.subscribe(publishBinding));
    cleanups.push(assembly.subscribe(publishBinding));
    cleanups.push(events.on('notification.changed', () => { void inbox.refreshUnread(); }));
    const restart = () => { if (isCurrent()) this.restart(); };
    cleanups.push(home.subscribe(() => { const next = home.getSnapshot().document; if (next && next.viewerFence !== document.viewerFence) restart(); }));
    cleanups.push(events.on('libraries.changed', restart));
    cleanups.push(events.on('settings.changed', restart));
    cleanups.push(events.on('list.changed', event => { if (['members', 'liveSources', 'libraryChannels'].includes(event.resource?.id ?? '')) restart(); }));
    const wake = () => {
      if (!globalThis.document.hidden) { auth.wake(); void auth.permissionDenied(); }
    };
    globalThis.document.addEventListener('visibilitychange', wake); window.addEventListener('online', wake);
    cleanups.push(() => { globalThis.document.removeEventListener('visibilitychange', wake); window.removeEventListener('online', wake); });
    globalThis.document.documentElement.dataset.reduceMotion = String(facts.preferences.effective['appearance.reduceMotion'] === true);
    publishBinding();
  }
  selectServer = async (serverId:string) => {this.fence();this.publish({phase:'restoring'});try{await (this.environment.actions?.()??browserAppSessionActions()).switchServer(serverId);await this.start();}catch(error){this.publish({phase:'error',error});}};
  dispose = () => { this.fence(); this.publish({phase: 'signed-out'}); };
}
