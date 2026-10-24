import type {HttpLocalApi, LocalSession} from '@core/index.ts';
import {parseLibraryList} from '@core/library-list.ts';
import {LocalSessionRefresher} from '@core/session-refresh.ts';
import {selectedViewerRecord} from '@core/viewer-selection.ts';
import {PreferencesReader} from '@core/preferences-reader.ts';
import {appearancePreferences, regionPreferences} from '@core/preference-effects.ts';
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
import {browserServerDisplay} from '../platform/server-display';
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
  facts: ShellFacts; preferences?: PreferencesReader; epoch: number; viewerFence?: string; isCurrent: () => boolean;
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
export type ShellBootstrapSnapshot = Readonly<{phase: 'restoring' | 'signed-out' | 'ready' | 'error'; binding?: ShellBindings; context?: ShellAssemblyContext; entry?: boolean; error?: unknown}>;

function viewerId(session: LocalSession): string { const v = session.viewer; return JSON.stringify([v.authority, v.accountId, v.profileId]); }
export function sameShellAuthority(a: LocalSession, b: LocalSession): boolean {
  return a.sessionFamilyId === b.sessionFamilyId && (['serverId', 'authority', 'accountId', 'profileId', 'role'] as const).every(key => a.viewer[key] === b.viewer[key]);
}

/** A cache discriminator from admitted authority, never a substitute Home fence. */
export function shellArtworkAuthorityKey(session: LocalSession, epoch: number): string {
  return JSON.stringify([session.viewer.serverId, session.sessionFamilyId, viewerId(session), session.viewer.role, epoch]);
}
/** Subscribe before the initial load. The first actual document establishes the
 * baseline without remounting Settings; a later missing/replaced fence retires it. */
export function watchHomeViewerFence(home: Pick<HomeService, 'getSnapshot' | 'subscribe'>, isCurrent: () => boolean, replaced: () => void) {
  let fence: string | undefined, closed = false, off = () => {};
  const dispose = () => {if (closed) return; closed = true; off();};
  const read = () => {
    if (closed) return;
    if (!isCurrent()) {dispose(); return;}
    const next = home.getSnapshot().document?.viewerFence;
    if (fence === undefined) {if (next) fence = next; return;}
    if (next !== fence) {dispose(); replaced();}
  };
  off = home.subscribe(read); read();
  return {get viewerFence() {return !closed && isCurrent() ? fence : undefined;}, dispose};
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
  return {identity: {viewer: session.viewer, libraries: libraries.items, serverName: system.name, serverVersion: system.version, signIn:system.signIn, ...profile,
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
  private disposed = false;
  constructor(private readonly assemble: ShellAssembler, private readonly environment: {
    selected: Pick<typeof browserViewerStorage, 'read'>;
    restore: typeof restoreBrowserConnection;
    actions?: () => BrowserAppSessionActions;
  } = {selected: browserViewerStorage, restore: restoreBrowserConnection}) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { if (this.disposed) return () => {}; this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(next: ShellBootstrapSnapshot) { if (this.disposed && next.phase !== 'signed-out') return; this.state = Object.freeze(next); for (const listener of this.listeners) listener(); }
  private fence() {
    ++this.epoch; this.controller?.abort(); this.controller = undefined;
    this.retire?.(); this.retire = undefined;
  }
  /** Suspend only the mounted runtime. Durable sessions stay until an explicit
   * admission/sign-out decides their fate. */
  suspendForAccountEntry = () => {if (this.disposed) return;this.fence();this.publish({phase: 'signed-out', entry: true});};
  resumeAfterAccountEntry = async () => {
    const expected = this.epoch;
    await this.running;
    // A disposed or superseded entry may not wake a private runtime later.
    if (this.epoch === expected) await this.start();
  };
  private restart = () => {
    this.fence(); this.publish({phase: 'restoring'});
    const pending = this.running, expected = this.epoch;
    void Promise.resolve(pending).then(() => { if (this.epoch === expected && this.state.phase === 'restoring') return this.start(); });
  };
  start = (): Promise<void> => {
    if (this.disposed) return Promise.resolve();
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
    const close = () => { for (const cleanup of cleanups.splice(0).reverse()) { try { cleanup(); } catch { /* Retire every other captured owner too. */ } } };
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
    const homeFence = watchHomeViewerFence(home, isCurrent, () => this.restart());
    cleanups.push(homeFence.dispose);
    const authorityKey = shellArtworkAuthorityKey(session, epoch);
    const artwork = authenticatedArtwork({api, auth, identity: {serverId: session.viewer.serverId, viewerId: viewerId(session), authorityKey}, currentToken: () => session.accessToken, isCurrent});
    cleanups.push(() => artwork.dispose());
    const signOutPorticoAccount=facts.identity.signIn==='portico'?()=>{if(!isCurrent())return;signingOut=true;this.fence();this.publish({phase:'signed-out'});void signOutBrowserPorticoAccount().catch(error=>this.publish({phase:'error',error})).finally(()=>api.getRouteConnection()?.dispose());}:undefined;
    const switchProfile=async(profileId:string,pin:string,signal:AbortSignal)=>{if(!isCurrent())throw new Error('The viewer changed.');await (this.environment.actions?.()??browserAppSessionActions()).switchProfile(api,session,profileId,pin,signal,isCurrent);if(isCurrent())this.restart();};
    const display=browserServerDisplay();display.remember(api,facts.serverName);
    const saved=(await (this.environment.actions?.()??browserAppSessionActions()).store.read()).servers;
    const savedServers=await display.choices(saved.map(value=>({pin:value.connection.pin,origin:value.connection.logicalOrigin,signIn:value.signIn})),signal);
    if (!isCurrent()) return;
    const context: ShellAssemblyContext = {api, session: () => session, auth, events, freshness, home, inbox, artwork, facts, preferences, epoch, get viewerFence() {return homeFence.viewerFence;}, isCurrent, signOut,signOutPorticoAccount,switchProfile,switchServer:this.selectServer,savedServers};
    const assembly = await this.assemble(context);
    if (!isCurrent()) { assembly.dispose(); return; }
    cleanups.push(() => assembly.dispose());
    cleanups.push(runEventFeed(events, freshness.hub, browserFeedEnvironment(assembly.busy, assembly.onBusy)));
    const publishBinding = () => {
      if (!isCurrent()) return;
      const binding = assembly.getBinding();
      this.publish({phase: 'ready', context, binding: {...binding, shell: {...binding.shell,
        identity: {...binding.shell.identity, unread: inbox.getSnapshot().unread, showNotificationDot: (preferences.getSnapshot('web') ?? facts.preferences).effective['notifications.badges'] === true}}}});
    };
    cleanups.push(inbox.subscribe(publishBinding));
    cleanups.push(assembly.subscribe(publishBinding));
    cleanups.push(events.on('notification.changed', () => { void inbox.refreshUnread(); }));
    const restart = () => { if (isCurrent()) this.restart(); };
    cleanups.push(events.on('libraries.changed', restart));
    let regionKey = JSON.stringify(regionPreferences(facts.preferences));
    cleanups.push(preferences.subscribe(() => {
      const next = preferences.getSnapshot('web'); if (!next || !isCurrent()) return;
      const key = JSON.stringify(regionPreferences(next));
      if (key !== regionKey) { regionKey = key; restart(); } else publishBinding();
    }));
    const refreshPreferences = () => { void preferences.preferences('web').catch(() => { /* Keep the last confirmed values; a wake or later event retries. */ }); };
    cleanups.push(events.on('settings.changed', event => { if (event.resource?.id === 'preferences') refreshPreferences(); else restart(); }));
    cleanups.push(events.on('list.changed', event => { if (['members', 'liveSources', 'libraryChannels'].includes(event.resource?.id ?? '')) restart(); }));
    const wake = () => {
      if (!globalThis.document.hidden) { auth.wake(); void auth.permissionDenied(); refreshPreferences(); }
    };
    globalThis.document.addEventListener('visibilitychange', wake); window.addEventListener('online', wake);
    cleanups.push(() => { globalThis.document.removeEventListener('visibilitychange', wake); window.removeEventListener('online', wake); });
    const motion = globalThis.matchMedia('(prefers-reduced-motion: reduce)');
    const applyMotion = () => { globalThis.document.documentElement.dataset.reduceMotion = String(appearancePreferences(preferences.getSnapshot('web') ?? facts.preferences, motion.matches).reducedMotion); };
    applyMotion(); motion.addEventListener('change', applyMotion);
    cleanups.push(preferences.subscribe(applyMotion));
    cleanups.push(() => motion.removeEventListener('change', applyMotion));
    void home.load();
    publishBinding();
  }
  selectServer = async (serverId:string) => {if(this.disposed)return;this.fence();const epoch=this.epoch;this.publish({phase:'restoring'});try{await (this.environment.actions?.()??browserAppSessionActions()).switchServer(serverId);if(!this.disposed&&this.epoch===epoch)await this.start();}catch(error){if(!this.disposed&&this.epoch===epoch)this.publish({phase:'error',error});}};
  dispose = () => { if (this.disposed) return; this.disposed = true; this.fence(); this.publish({phase: 'signed-out'}); this.listeners.clear(); };
}
