import {createRoot} from 'react-dom/client';
import {IconSprite} from '../../../kit/Icon';
import {SetupScreen} from '../SetupScreen';
import type {SetupPorts, SetupServer} from '../setup-flow';
import '../../../kit/tokens.css';
import '../../../kit/fonts.css';
import '../../../bootstrap.css';

/** auth1 / auth1c / auth3 lift-and-compare specimen. The drawn states use a
 * fake verified server and no production route; `?frame=3` resumes a completed
 * local session so the first-library state can be captured at its drawn size. */
if (!import.meta.env.DEV) throw new Error('Setup specimens require development.');

const now = Date.now();
const session = {accessToken: 'specimen', expiresAt: new Date(now + 3_600_000).toISOString(),
  authorizationHorizon: new Date(now + 7_200_000).toISOString(), sessionFamilyId: 'specimen-family', tokenGeneration: '1',
  viewer: {accountId: 'owner-account', profileId: 'main-profile', serverId: 'server-1', authority: 'local', role: 'owner'}};
const frame = new URLSearchParams(location.search).get('frame');
const server: SetupServer = {
  server: {id: 'server-1', name: 'Portico demo', version: '1', signIn: 'local', move: null, setupRequired: true, hostedConfigured: false, hostedAttached: false},
  api: {getRouteConnection: () => ({pin: {serverId: 'server-1'}}), request: async (path: string) => {
    if (path.includes('setup/resume')) return session;
    if (path.includes('setup/browser')) return {serverId: 'server-1', setupToken: 'specimen-token'};
    return {};
  }} as never,
  pin: {serverId: 'server-1', publicKey: 'specimen', fingerprint: 'specimen'} as never,
  draft: {read: async () => frame === '3' ? {version: 1, serverId: 'server-1', requestId: '11111111-2222-4333-8444-555555555555', setupToken: 'specimen-token', name: 'Portico demo', authMode: 'local'} : undefined,
    change: async () => {}} as never,
  dispose: () => {},
};
const ports: SetupPorts = {
  connect: async () => server,
  acceptLocal: async () => {},
  continueHosted: async () => {},
  addLibrary: async () => {},
  complete: () => {},
  hostname: () => '192.168.1.20',
};
createRoot(document.getElementById('root')!).render(<><IconSprite /><SetupScreen ports={ports}/></>);
