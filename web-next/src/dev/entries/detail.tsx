import {createRoot} from 'react-dom/client';
import {useEffect} from 'react';
import {RouterProvider, createMemoryHistory} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import type {Library, LocalSession, Viewer} from '@core/index.ts';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import {router} from '../../router';
import {WebScreenProvider} from '../../route-registry-view';
import {ShellBindingsProvider, type ShellBindings} from '../../shell/bindings';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import {createOwnerActivity} from '../../shell/Activity';
import type {ShellIdentity} from '../../shell/Shell';
import type {HomeBindings} from '../../screens/home/Home';
import {IconSprite} from '../../kit/Icon';
import {createDetailSpecimen} from './detail-specimen';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

// The specimen is a public, development-only capture of the registered Item route. It never
// reaches a production build: no production module imports it and this entry fails closed.
if (!import.meta.env.DEV) throw new Error('Detail specimens require the development server.');
const frame = new URLSearchParams(location.search).get('frame');
if (frame !== null && frame !== '1') throw new Error('The detail specimen measures desktop frame 1 only.');

/** The measured state includes every real artwork slot; wait for the genuine
 *  images the screen mounts, never for a synthetic painting delay or a rewritten DOM. */
function DetailReady() {
  useEffect(() => {
    let frame = 0, stopped = false;
    const wait = () => {
      if (stopped) return;
      const images = [...document.querySelectorAll<HTMLImageElement>('#portico-content article img')];
      if (images.length >= 18 && images.every(image => image.complete)) document.documentElement.dataset.detailReady = 'true';
      else frame = requestAnimationFrame(wait);
    };
    wait();
    return () => { stopped = true; cancelAnimationFrame(frame); delete document.documentElement.dataset.detailReady; };
  }, []);
  return null;
}

const specimen = createDetailSpecimen();
const time = {t: defaultI18n.t, locale: 'en-US', timeZone: 'UTC'};
const viewer: Viewer = {serverId: specimen.serverId, authority: 'local', accountId: 'specimen-account', profileId: 'demo', role: 'owner'};
const session: LocalSession = {
  accessToken: 'specimen-public', refreshToken: '', sessionFamilyId: 'specimen-family', tokenGeneration: '1',
  installationId: 'i'.repeat(43), expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-02-01T00:00:00Z', viewer,
};
const libraries: readonly Library[] = [
  {id: 'movies', name: 'Movies', kind: 'movie'},
  {id: 'tv', name: 'TV Shows', kind: 'show'},
  {id: 'anime', name: 'Anime', kind: 'show'},
  {id: 'music', name: 'Music', kind: 'music'},
  {id: 'audiobooks', name: 'Audiobooks', kind: 'audiobook'},
];
const identity: ShellIdentity = {
  viewer, username: 'demo', profileName: 'Demo', profileArt: 'blue', serverName: 'Portico demo',
  primaryProfile: true, profiles: [], canManageProfiles: false, accountFacts: [], libraries, liveAvailable: true,
  unread: 3, showNotificationDot: true,
  capabilities: {manageAccount: false, managePeople: false, manageServer: true, manageSignIn: false},
};

const preferences = specimen.preferences as unknown as PreferenceSnapshot;
const homeService = {getSnapshot: () => ({phase: 'idle'}), subscribe: () => () => {}} as unknown as HomeBindings['service'];
const home: HomeBindings = {
  service: homeService, artwork: specimen.artwork, time, reducedMotion: true,
  selection: {toolbar: null, card: () => undefined, channel: () => undefined, collection: () => undefined,
    registerWindow: () => undefined, registerSpotlight: () => undefined, moreAvailable: () => false, moreExpanded: () => false},
  effects: {
    play: async () => { specimen.record('play'); }, playChannel: async () => {}, openEntry: () => {},
    openSeeAll: () => {}, openMore: () => {}, setMyList: async () => {},
    reportError: error => { specimen.record('report-error', String(error)); },
  },
};
const contextBase = {
  api: specimen.api, session: () => session, events: specimen.events, artwork: specimen.artwork,
  facts: {identity, preferences, serverName: identity.serverName}, preferences: {getSnapshot: () => preferences, subscribe: specimen.subscribePreferences},
  epoch: 1, isCurrent: () => true, signOut: () => { specimen.record('sign-out'); },
  signOutPorticoAccount: () => { specimen.record('sign-out-portico'); },
  switchProfile: async () => {}, switchServer: async () => {}, savedServers: [],
} as unknown as ShellAssemblyContext;
const activity = createOwnerActivity(contextBase, time, () => { specimen.record('open-dashboard'); }, () => { specimen.record('open-update'); }, destination => { specimen.record('open-issue', destination); });
const binding: ShellBindings = {
  api: specimen.api, events: specimen.events, scopeKey: 'detail-specimen',
  shell: {identity, OwnerActivity: activity.OwnerActivity, dockVisible: false, onSignOut: () => { specimen.record('sign-out'); }},
  home,
};
const context = contextBase;
// The fixture's real command receipts are inspectable from the page; no credentials or server data.
(window as Window & {__porticoDetailSpecimen?: unknown}).__porticoDetailSpecimen = {records: specimen.records, detail: specimen.detail, recommendations: specimen.recommendations};

router.update({history: createMemoryHistory({initialEntries: [`/item/${specimen.itemId}`]})});
createRoot(document.getElementById('root')!).render(
  <WebScreenProvider value={{binding, context}}>
    <DetailReady />
    <IconSprite />
    <ShellBindingsProvider value={binding}>
      <RouterProvider router={router} />
    </ShellBindingsProvider>
  </WebScreenProvider>,
);
