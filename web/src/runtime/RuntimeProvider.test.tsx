import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useRef, useState } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { useRuntime } from './RuntimeContext';
import { extractHostedBootstrapIntent, extractPorticoLoginResult, RuntimeProvider } from './RuntimeProvider';
import { consumeNativeDeviceSSOAutoStart, nativeDeviceAuthorizationCompletionURL, RuntimeSurface } from './RuntimeSurface';
import type { HostedServerSummary, RuntimeConfig } from './runtimeMachine';
import { DataProvider, useAuthSession, usePorticoDataSource } from '../data/DataProvider';
import { FixturePorticoDataSource } from '../data/fixtureSource';
import { App } from '../App';
import { LocalProfileSelectionSurface, SignInSurface } from '../features/auth/AuthSurface';
import {
  createBrowserHostedConnectionVault,
  createBrowserHostedConnectionVaultWithDurableMetadata,
  createHostedConnectionVault,
  type HostedConnectionVault,
  type HostedConnectionVaultStorage,
} from './hostedConnectionVault';
import * as ClientCore from '@porticomediaserver/client-core';
import {
  GLOBAL_SIGN_OUT_FENCE_ID,
  markAccountSignedOut,
  resetSignedOutAccountQuarantineForTests,
  SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX,
} from './signedOutAccountLedger';
import { beginAmbientCookieMutation } from './ambientCookieQuarantine';
import clientCompatibilityFixture from '../../../api/openapi/fixtures/client-compatibility-conformance.json';

function response(payload: unknown, ok = true, status = 200) {
  const body = JSON.stringify(payload);
  return new Response(body, {
    status,
    statusText: ok ? 'OK' : 'Unavailable',
    headers: {
      'Content-Type': 'application/json',
      'Content-Length': String(new TextEncoder().encode(body).byteLength),
    },
  });
}

function sharedVaultStorage(): HostedConnectionVaultStorage {
  const stores = new Map<string, Map<string, unknown>>();
  const store = (name: string) => {
    const current = stores.get(name) ?? new Map<string, unknown>();
    stores.set(name, current);
    return current;
  };
  const key = (value: IDBValidKey) => JSON.stringify(value);
  return {
    get: async <T,>(name: string, id: IDBValidKey) => store(name).get(key(id)) as T | undefined,
    put: async <T,>(name: string, value: T) => {
      const record = value as { key?: IDBValidKey; accountId?: IDBValidKey };
      store(name).set(key(record.key ?? record.accountId ?? ''), value);
    },
    delete: async (name, id) => { store(name).delete(key(id)); },
    getAll: async <T,>(name: string) => [...store(name).values()] as T[],
  };
}

const fixtureConfig: RuntimeConfig = { mode: 'fixtures', hostedApiBaseUrl: 'https://api.getportico.tv', routeProbeTimeoutMs: 500, buildId: 'test' };
const hostedConfig: RuntimeConfig = { mode: 'hosted', hostedApiBaseUrl: 'https://web.getportico.tv', routeProbeTimeoutMs: 500, buildId: 'test' };
const bundledConfig: RuntimeConfig = { mode: 'bundled', hostedApiBaseUrl: 'https://api.getportico.tv', routeProbeTimeoutMs: 500, buildId: 'test' };
const loadFixtureSource = async () => new FixturePorticoDataSource();
const hostedCompatibility = {
  ...ClientCore.PORTICO_FOUNDATION_COMPATIBILITY,
  requiredSemantics: Object.keys(ClientCore.PORTICO_FOUNDATION_COMPATIBILITY.semanticRevisions),
  capabilities: [{ id: 'system', revision: 1, state: 'available', requiredSemantics: ['product'] }],
};
const hostedSystem = { name: 'Portico', status: 'ok', apiVersion: 'v1', compatibility: hostedCompatibility };
const serverSystem = clientCompatibilityFixture.system;
const serverProductContract = clientCompatibilityFixture.productContract;
const unrestrictedPolicy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
const testServerFingerprints: Record<string, string> = {
  'server-1': `sha256:${'A'.repeat(43)}`,
  'server-2': `sha256:${'B'.repeat(43)}`,
};

function testServerFingerprint(serverId: string): string {
  return testServerFingerprints[serverId] ?? `sha256:${'C'.repeat(43)}`;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function hostedServer(id: string, name: string): ClientCore.HostedServer {
  return {
    id,
    ownerUserId: 'user-1',
    name,
    serverPublicKey: `key-${id}`,
    serverPublicKeyFingerprint: testServerFingerprint(id),
    assignedHostname: `${id}.direct.getportico.tv`,
    remoteAccessEnabled: true,
    preferredAuthMode: 'portico',
    certificateStatus: 'active',
    createdAt: '2026-07-14T10:00:00.000Z',
    updatedAt: '2026-07-14T11:00:00.000Z',
  };
}

function localIdentity(serverId: string, serverName: string, profileId = 'profile-1'): ClientCore.AuthMeResponse {
  return {
    authenticated: true,
    setupRequired: false,
    serverFriendlyName: serverName,
    authority: 'hosted',
    accountId: 'user-1',
    serverId,
    profileId,
    authorizationRevision: `policy-${profileId}`,
    authProvider: 'portico',
    user: {
      id: 'user-1',
      displayName: 'Owner',
      email: 'owner@example.test',
      role: 'owner',
      username: 'owner',
      authOrigin: 'portico',
      authProvider: 'portico',
      hasLocalPassword: false,
      permissions: {},
      preferences: { sidebarOrder: [] },
      libraryIds: [],
    },
  } as unknown as ClientCore.AuthMeResponse;
}

function preparedConnection(serverId: string, serverName: string, profileId = 'profile-1'): ClientCore.PreparedTrustedServerConnection {
  const identity = localIdentity(serverId, serverName, profileId);
  const session: ClientCore.LocalServerSession = {
    serverId,
    serverName,
    apiBaseUrl: `https://${serverId}.direct.getportico.tv`,
    accessToken: `access-${serverId}-${profileId}`,
    refreshToken: `refresh-${serverId}-${profileId}`,
    serverPublicKeyFingerprint: testServerFingerprint(serverId),
  };
  return {
    identity,
    scope: ClientCore.viewerScopeFromAuthMe(identity),
    session,
    record: {
      schemaVersion: 2,
      accountId: 'user-1',
      serverId,
      profileId,
      serverName,
      serverPublicKeyFingerprint: testServerFingerprint(serverId),
      currentRoute: { url: `https://${serverId}.direct.getportico.tv`, type: 'public_direct', verifiedAt: '2026-07-14T11:00:00.000Z' },
      session,
      lastSuccessfulConnectionAt: '2026-07-14T11:00:00.000Z',
    },
    source: 'hosted',
  };
}

type TestConnectorOptions = Parameters<typeof ClientCore.connectTrustedServerRecord>[1];

async function publishPreparedConnection(
  candidate: ClientCore.PreparedTrustedServerConnection,
  options: TestConnectorOptions,
): Promise<ClientCore.TrustedServerConnectionResult> {
  const previous = options.sessionStore.get?.();
  const staged = await options.stageCandidate(candidate);
  try {
    options.sessionStore.set(candidate.session);
    await options.connectionAdapter.save(candidate.record);
    const persistence = options.connectionAdapter as typeof options.connectionAdapter & { durability?: () => 'durable' | 'memory-only' };
    const result: ClientCore.TrustedServerConnectionResult = {
      ...candidate,
      durability: persistence.durability?.() ?? 'durable',
      persistencePolicy: options.connectionAdapter.persistencePolicy ?? 'saved-session',
    };
    await staged.publish(result);
    return result;
  } catch (reason) {
    staged.fenceRollback('restore-previous');
    if (previous) options.sessionStore.set(previous);
    else options.sessionStore.clear();
    await staged.rollback('restore-previous');
    throw reason;
  }
}
const bundledOwner = {
  authenticated: true,
  setupRequired: false,
  serverFriendlyName: 'Family Media',
  accountMode: 'local',
  authority: 'local',
  accountId: 'user-1',
  serverId: 'server-1',
  profileId: 'profile-1',
  authorizationRevision: 'policy-1',
  user: { id: 'user-1', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'local', authProvider: 'local', hasLocalPassword: true, permissions: {}, preferences: { sidebarOrder: [] }, libraryIds: [] },
};

async function rememberedHostedBrowser(serverIds = ['server-1']): Promise<HostedConnectionVault> {
  const vault = createBrowserHostedConnectionVault(undefined);
  await vault.rememberAccount({ accountId: 'user-1', displayName: 'Owner', email: 'owner@example.test' });
  for (const [index, serverId] of serverIds.entries()) {
    const serverName = serverId === 'server-1' ? 'Family Media' : 'Cinema';
    const serverPublicKeyFingerprint = testServerFingerprint(serverId);
    const record: ClientCore.TrustedServerConnectionRecord = {
      schemaVersion: 2,
      accountId: 'user-1',
      serverId,
      profileId: 'profile-1',
      serverName,
      serverPublicKeyFingerprint,
      currentRoute: { url: `https://${serverId}.direct.getportico.tv`, type: 'public_direct', verifiedAt: `2026-07-14T11:0${index}:00.000Z` },
      session: {
        serverId,
        serverName,
        apiBaseUrl: `https://${serverId}.direct.getportico.tv`,
        accessToken: `access-${serverId}`,
        refreshToken: `refresh-${serverId}`,
        authority: 'hosted',
        accountId: 'user-1',
        profileId: 'profile-1',
        authorizationRevision: 'policy-1',
        serverPublicKeyFingerprint,
      },
      lastSuccessfulConnectionAt: `2026-07-14T11:0${index}:00.000Z`,
    };
    await vault.save(record);
  }
  return vault;
}

function RuntimeHarness() {
  const runtime = useRuntime();
  if (runtime.state.id === 'server-ready') return <div>Ready on {runtime.state.serverName}</div>;
  return <RuntimeSurface />;
}

function MembershipRefreshHarness() {
  const runtime = useRuntime();
  return <>
    <output aria-label="runtime-state">{runtime.state.id}</output>
    {runtime.state.id === 'server-ready' && <div>Ready on {runtime.state.serverName}</div>}
    <button type="button" onClick={() => void runtime.refreshMemberships()}>Refresh memberships</button>
    <RuntimeSurface />
  </>;
}

function HostedLogoutHarness() {
  const runtime = useRuntime();
  const [logoutSettled, setLogoutSettled] = useState(false);
  const message = runtime.state.id === 'hosted-sign-in'
    ? (runtime.state.messageId ? ClientCore.productMessage(runtime.state.messageId).body : undefined)
    : undefined;
  return <><div>{runtime.state.id}</div>{message && <div>{message}</div>}<button type="button" onClick={() => void runtime.hostedLogout().finally(() => setLogoutSettled(true))}>Test sign out</button>{logoutSettled && <output>Sign out settled</output>}</>;
}

function AuthStatus() {
  const auth = useAuthSession();
  return <div>{auth.status}:{auth.viewer?.user?.displayName}</div>;
}

function ReadyAuthHarness() {
  const runtime = useRuntime();
  if (runtime.state.id !== 'server-ready' || !runtime.source) return <RuntimeSurface />;
  return <DataProvider source={runtime.source} initialViewer={runtime.initialViewer} viewerRuntime={runtime.viewerRuntime}><AuthStatus /></DataProvider>;
}

function ReadyAppHarness() {
  const runtime = useRuntime();
  if (runtime.state.id !== 'server-ready' || !runtime.source) return <RuntimeSurface />;
  return <DataProvider source={runtime.source} initialViewer={runtime.initialViewer} viewerRuntime={runtime.viewerRuntime}><MemoryRouter><App /></MemoryRouter></DataProvider>;
}

function LocalAuthProbe() {
  const auth = useAuthSession();
  const serverName = auth.viewer?.serverName ?? 'Family Media';
  if (auth.localProfileLogin) return <LocalProfileSelectionSurface serverName={serverName} />;
  if (auth.status === 'ready' && auth.viewer?.authenticated) return <AuthStatus />;
  return <SignInSurface serverName={serverName} />;
}

function BundledLocalAuthHarness() {
  const runtime = useRuntime();
  if (runtime.state.id !== 'server-ready' || !runtime.source) return <RuntimeSurface />;
  return <DataProvider source={runtime.source} initialViewer={runtime.initialViewer} localSessionQuarantineEnabled viewerRuntime={runtime.viewerRuntime}><MemoryRouter><LocalAuthProbe /></MemoryRouter></DataProvider>;
}

function DirectChoiceHarness() {
  const runtime = useRuntime();
  const serverChoices = useRef<HostedServerSummary[]>([]);
  const profileChoices = useRef<string[]>([]);
  if (runtime.state.id === 'server-selection') serverChoices.current = runtime.state.servers;
  if (runtime.state.id === 'profile-selection') {
    serverChoices.current = runtime.state.servers;
    profileChoices.current = runtime.state.profiles.map((profile) => profile.id);
  }
  return <>
    <output aria-label="runtime-state">{runtime.state.id}</output>
    <output aria-label="ready-profile">{runtime.initialViewer?.viewerScope?.profileId ?? 'none'}</output>
    <output aria-label="connection-warning">{runtime.connectionWarning ?? 'none'}</output>
    {serverChoices.current.map((server) => <button key={server.id} type="button" onClick={() => void runtime.selectServer(server)}>Direct {server.id}</button>)}
    {profileChoices.current.map((profileId) => <button key={profileId} type="button" onClick={() => void runtime.selectProfile(profileId)}>Direct {profileId}</button>)}
  </>;
}

function ScopedMutationProbe() {
  const source = usePorticoDataSource();
  const [result, setResult] = useState('idle');
  return <>
    <output aria-label="mutation-result">{result}</output>
    <button type="button" onClick={() => {
      void source.updateProfile({ displayName: 'Owner', email: 'owner@example.test' }, new AbortController().signal).then(
        () => setResult('success'),
        (reason: unknown) => setResult(typeof (reason as { name?: unknown } | null)?.name === 'string' ? (reason as { name: string }).name : 'error'),
      );
    }}>Run scoped mutation</button>
  </>;
}

function TransactionShellHarness() {
  const runtime = useRuntime();
  const serverChoices = useRef<HostedServerSummary[]>([]);
  if (runtime.state.id === 'server-selection') serverChoices.current = runtime.state.servers;
  return <>
    <output aria-label="runtime-state">{runtime.state.id}</output>
    <output aria-label="ready-server">{runtime.state.id === 'server-ready' ? runtime.state.serverName : 'none'}</output>
    <output aria-label="connection-warning">{runtime.connectionWarning ?? 'none'}</output>
    {serverChoices.current.map((server) => <button key={server.id} type="button" onClick={() => void runtime.selectServer(server)}>Direct {server.id}</button>)}
    <button type="button" onClick={() => void runtime.reselectServer()}>Choose another server directly</button>
    {runtime.source && <DataProvider
      source={runtime.source}
      initialViewer={runtime.initialViewer}
      expectedViewerScope={runtime.expectedViewerScope}
      browserAccountsEnabled={false}
      viewerRuntime={runtime.viewerRuntime}
    ><ScopedMutationProbe /></DataProvider>}
  </>;
}

afterEach(() => {
  vi.useRealTimers();
  resetSignedOutAccountQuarantineForTests();
  window.history.replaceState(null, '', '/');
  window.localStorage.clear();
	window.sessionStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('RuntimeProvider', () => {
  it('uses a one-shot native SSO sentinel and one fixed completion callback', () => {
    expect(consumeNativeDeviceSSOAutoStart('apple', 'JKMN-PQRS')).toBe(true);
    expect(consumeNativeDeviceSSOAutoStart('apple', 'JKMN-PQRS')).toBe(false);
    expect(consumeNativeDeviceSSOAutoStart('google', 'JKMN-PQRS')).toBe(true);
    expect(nativeDeviceAuthorizationCompletionURL(true)).toBe('portico://device-authorization-complete');
    expect(nativeDeviceAuthorizationCompletionURL(false)).toBeUndefined();
  });

  it('keeps a TV code out of requests and authorizes the same setup session after explicit server selection', async () => {
    window.history.replaceState(null, '', '/device#code=ABCD-EFGH');
    const requests: Array<{ url: string; body?: Record<string, unknown> }> = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      requests.push({ url, body });
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers?')) return Promise.resolve(response({ items: [hostedServer('server-1', 'Family Room'), hostedServer('server-2', 'Cabin')], pageInfo: { hasMore: false } }));
      if (url.endsWith('/api/tv-setup/preview')) return Promise.resolve(response({ setupSessionId: 'tvsu-one', code: 'ABCD-EFGH', status: 'pending', deviceName: 'Living Room TV', platform: 'tvOS', appVersion: '1.0', expiresAt: new Date(Date.now() + 60_000).toISOString() }));
      if (url.endsWith('/api/tv-setup/grants')) return Promise.resolve(response({ setupSessionId: 'tvsu-one', status: 'grant_ready', serverId: 'server-1', serverUrl: 'https://server.test', encryptedGrant: {}, expiresAt: new Date(Date.now() + 60_000).toISOString() }, true, 201));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByText('Living Room TV')).toBeInTheDocument();
    expect(window.location.pathname).toBe('/device');
    expect(window.location.hash).toBe('');
    expect(requests.every((request) => !request.url.includes('ABCD-EFGH'))).toBe(true);
    fireEvent.click(screen.getByRole('radio', { name: /Family Room/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Connect TV' }));
    expect(await screen.findByRole('heading', { name: 'TV connected' })).toBeInTheDocument();
    expect(requests.find((request) => request.url.endsWith('/api/tv-setup/grants'))?.body).toEqual({ code: 'ABCD-EFGH', setupSessionId: 'tvsu-one', serverId: 'server-1' });
  });

  it('authorizes a TV to the Portico Account when no server is available', async () => {
    window.history.replaceState(null, '', '/device#code=ABCD-EFGH');
    const requests: Array<{ url: string; body?: Record<string, unknown> }> = [];
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      requests.push({ url, body });
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers?')) return Promise.resolve(response({ items: [], pageInfo: { hasMore: false } }));
      if (url.endsWith('/api/tv-setup/preview')) return Promise.resolve(response({ setupSessionId: 'tvsu-no-server', code: 'ABCD-EFGH', status: 'pending', deviceName: 'Guest Room TV', platform: 'tvOS', expiresAt: new Date(Date.now() + 600_000).toISOString() }));
      if (url.endsWith('/api/tv-setup/grants')) return Promise.resolve(response({ setupSessionId: 'tvsu-no-server', status: 'grant_ready', encryptedGrant: {}, expiresAt: new Date(Date.now() + 120_000).toISOString() }, true, 201));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    }));

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByText('Guest Room TV')).toBeInTheDocument();
    expect(screen.getByText(/No servers are available yet/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Connect TV' }));
    expect(await screen.findByRole('heading', { name: 'TV connected' })).toBeInTheDocument();
    expect(requests.find((request) => request.url.endsWith('/api/tv-setup/grants'))?.body).toEqual({ code: 'ABCD-EFGH', setupSessionId: 'tvsu-no-server', serverId: '' });
  });

  it('keeps generic limited-input authorization separate from TV setup', async () => {
    window.history.replaceState(null, '', '/authorize-device#code=JKMN-PQRS');
    const requests: Array<{ url: string; body?: Record<string, unknown> }> = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      requests.push({ url, body });
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.endsWith('/api/device-authorizations/preview')) return Promise.resolve(response({ status: 'pending', deviceName: 'Streaming Stick', platform: 'limited-tv', expiresAt: new Date(Date.now() + 60_000).toISOString() }));
      if (url.endsWith('/api/device-authorizations')) return Promise.resolve(response({ status: 'approved', deviceName: 'Streaming Stick', platform: 'limited-tv', expiresAt: new Date(Date.now() + 60_000).toISOString() }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText('Streaming Stick')).toBeInTheDocument();
    expect(requests.some((request) => request.url.includes('/api/tv-setup/'))).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Approve device' }));
    expect(await screen.findByRole('heading', { name: 'Device connected' })).toBeInTheDocument();
    expect(requests.find((request) => request.url.endsWith('/api/device-authorizations'))?.body).toEqual({ userCode: 'JKMN-PQRS', decision: 'approve' });
  });

  it('discards a late TV preview after the displayed code changes', async () => {
    window.history.replaceState(null, '', '/device');
    const firstPreview = deferred<Response>();
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers?')) return Promise.resolve(response({ items: [hostedServer('server-1', 'Family Room')], pageInfo: { hasMore: false } }));
      if (url.endsWith('/api/tv-setup/preview')) return firstPreview.promise;
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    const input = await screen.findByLabelText('Device code');
    fireEvent.change(input, { target: { value: 'ABCD-EFGH' } });
    fireEvent.click(screen.getByRole('button', { name: 'Review request' }));
    fireEvent.change(input, { target: { value: 'JKMN-PQRS' } });
    await act(async () => firstPreview.resolve(response({ setupSessionId: 'stale-session', code: 'ABCD-EFGH', status: 'pending', deviceName: 'Wrong TV', platform: 'tvOS', expiresAt: new Date(Date.now() + 60_000).toISOString() })));
    expect(screen.queryByText('Wrong TV')).not.toBeInTheDocument();
    expect(input).toHaveValue('JKMN-PQRS');
  });

  it('discards late generic previews and decisions after the code changes', async () => {
    window.history.replaceState(null, '', '/authorize-device');
    const stalePreview = deferred<Response>();
    const decision = deferred<Response>();
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.endsWith('/api/device-authorizations/preview')) {
        const requestBody = init?.body ? JSON.parse(String(init.body)) as { userCode?: string } : undefined;
        return requestBody?.userCode === 'ABCD-EFGH' ? stalePreview.promise : Promise.resolve(response({ status: 'pending', deviceName: 'Current Stick', platform: 'limited-tv', expiresAt: new Date(Date.now() + 60_000).toISOString() }));
      }
      if (url.endsWith('/api/device-authorizations')) return decision.promise;
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    const input = await screen.findByLabelText('Device code');
    fireEvent.change(input, { target: { value: 'ABCD-EFGH' } });
    fireEvent.click(screen.getByRole('button', { name: 'Review request' }));
    fireEvent.change(input, { target: { value: 'JKMN-PQRS' } });
    await act(async () => stalePreview.resolve(response({ status: 'pending', deviceName: 'Wrong Stick', platform: 'limited-tv', expiresAt: new Date(Date.now() + 60_000).toISOString() })));
    expect(screen.queryByText('Wrong Stick')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Review request' }));
    expect(await screen.findByText('Current Stick')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Approve device' }));
    fireEvent.change(input, { target: { value: 'RSTU-VWXY' } });
    await act(async () => decision.resolve(response({ status: 'approved', deviceName: 'Current Stick', platform: 'limited-tv', expiresAt: new Date(Date.now() + 60_000).toISOString() })));
    expect(screen.queryByRole('heading', { name: 'Device connected' })).not.toBeInTheDocument();
    expect(input).toHaveValue('RSTU-VWXY');
  });

  it('recognizes and removes the server-issued claim page code', () => {
    expect(extractHostedBootstrapIntent('https://web.getportico.tv/claim?code=SETUP123&serverName=Family%20Room&returnUrl=http%3A%2F%2Flocalhost%3A32500%2F%3FporticoSetup%3Dcontinue')).toEqual({
      intent: { claimCode: 'SETUP123', claimServerName: 'Family Room', claimReturnUrl: 'http://localhost:32500/?porticoSetup=continue' },
      safeUrl: '/',
    });
  });

  it('separates and scrubs an SSO onboarding token from password recovery', () => {
    expect(extractHostedBootstrapIntent('https://web.getportico.tv/auth/sso/onboarding?token=opaque-onboarding-token')).toEqual({
      intent: { ssoOnboardingToken: 'opaque-onboarding-token' },
      safeUrl: '/',
    });
  });

  it('recovers an unfinished SSO onboarding flow after the sensitive URL is scrubbed and reloaded', async () => {
    window.history.replaceState(null, '', '/auth/sso/onboarding?token=reload-safe-onboarding');
    const previewTokens: string[] = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/sso/onboarding/preview')) {
        const body = JSON.parse(String(init?.body)) as { onboardingToken: string };
        previewTokens.push(body.onboardingToken);
        return Promise.resolve(response({ provider: 'google', providerEmail: 'viewer@example.test', privateEmail: false, suggestedUsername: 'viewer' }));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    const first = render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Choose your Portico username' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/');
    first.unmount();

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Choose your Portico username' })).toBeInTheDocument();
    expect(previewTokens).toEqual(['reload-safe-onboarding', 'reload-safe-onboarding']);
  });

  it('shows verified provider identity read-only and retries a rotated token after a username conflict', async () => {
    window.history.replaceState(null, '', '/auth/sso/onboarding?token=onboarding-one');
    const requests: Array<{ url: string; body?: Record<string, unknown> }> = [];
    let completionAttempts = 0;
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      requests.push({ url, body });
      if (url.endsWith('/api/auth/sso/onboarding/preview')) return Promise.resolve(response({ provider: 'apple', providerEmail: 'relay@privaterelay.appleid.com', privateEmail: true, suggestedUsername: 'relay-viewer' }));
      if (url.endsWith('/api/auth/sso/onboarding/complete')) {
        completionAttempts += 1;
        if (completionAttempts === 1) return Promise.resolve(response({ authenticated: false, onboardingRequired: true, usernameUnavailable: true, onboardingToken: 'onboarding-two', provider: 'apple' }));
        return new Promise<Response>(() => undefined);
      }
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    const verifiedEmail = await screen.findByDisplayValue('relay@privaterelay.appleid.com');
    expect(verifiedEmail).toHaveAttribute('readonly');
    expect(screen.getByText(/Apple private relay address/)).toBeInTheDocument();
    expect(window.location.pathname).toBe('/');
    expect(window.location.search).toBe('');
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'taken-name' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create Portico Account' }));
    expect(await screen.findByText('That username is already in use. Choose another.')).toBeInTheDocument();
    await waitFor(() => expect(requests.filter((request) => request.url.endsWith('/api/auth/sso/onboarding/preview')).at(-1)?.body).toEqual({ onboardingToken: 'onboarding-two' }));
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'available-name' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create Portico Account' }));
    await waitFor(() => expect(requests.filter((request) => request.url.endsWith('/api/auth/sso/onboarding/complete')).at(-1)?.body).toEqual({ onboardingToken: 'onboarding-two', username: 'available-name' }));
  });

  it('fails closed when Hosted requires a separately verified contact email', async () => {
    window.history.replaceState(null, '', '/auth/sso/onboarding?token=needs-contact-verification');
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/sso/onboarding/preview')) return Promise.resolve(response({ provider: 'google', providerEmail: 'viewer@example.test', privateEmail: false, verifiedContactEmailRequired: true }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('alert')).toHaveTextContent('A verified contact email is still required');
    expect(screen.getByText(/Entering an address here would not verify/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Create Portico Account' })).toBeDisabled();
    expect(screen.queryByRole('textbox', { name: /contact email/i })).not.toBeInTheDocument();
  });

  it('returns a completed first-run claim to the exact local setup origin before route discovery', async () => {
    window.history.replaceState(null, '', '/claim?code=SETUP123&serverName=Family%20Room&returnUrl=http%3A%2F%2Flocalhost%3A32500%2F%3FporticoSetup%3Dcontinue');
    const navigate = vi.fn();
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.endsWith('/api/account/server-claims/by-code/complete')) return Promise.resolve(response({ server: hostedServer('server-1', 'Family Room'), serverCredential: 'credential' }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} navigate={navigate}><RuntimeHarness /></RuntimeProvider>);

    await waitFor(() => expect(navigate).toHaveBeenCalledWith('http://localhost:32500/?porticoSetup=continue'));
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/api/account/servers?'))).toBe(false);
  });

  it('moves an ambiguously failed idempotent server claim into automatic recovery without an immediate replay', async () => {
    window.history.replaceState(null, '', '/claim?code=SETUP123&serverName=Family%20Room');
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'owner', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.endsWith('/api/account/server-claims/by-code/complete')) return Promise.resolve(response({ code: 'store_unavailable' }, false, 503));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.cloud-unavailable').title })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();

    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/account/server-claims/by-code/complete'))).toHaveLength(1);
    vi.useRealTimers();
  });

  it('recovers an uncompleted same-tab server claim after a refresh', async () => {
    window.history.replaceState(null, '', '/claim?code=SETUP123&serverName=Family%20Room');
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    const first = render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Continue setting up your server “Family Room”' });
    first.unmount();
    window.history.replaceState(null, '', '/');

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Continue setting up your server “Family Room”' })).toBeInTheDocument();
  });

  it('explains server setup on claim authentication pages without a decorative icon', async () => {
    window.history.replaceState(null, '', '/claim?code=SETUP123&serverName=Family%20Room');
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: 'Continue setting up your server “Family Room”' })).toBeInTheDocument();
    expect(screen.getByText('Sign in or create a Portico Account to continue with server setup.')).toBeInTheDocument();
    const google = screen.getByRole('link', { name: 'Sign in with Google' });
    const apple = screen.getByRole('link', { name: 'Sign in with Apple' });
    expect(new URL(google.getAttribute('href')!)).toMatchObject({
      origin: 'https://web.getportico.tv',
      pathname: '/auth/sso/google/start',
    });
    expect(new URL(apple.getAttribute('href')!)).toMatchObject({
      origin: 'https://web.getportico.tv',
      pathname: '/auth/sso/apple/start',
    });
    expect(new URL(google.getAttribute('href')!).searchParams.get('returnTo')).toBe(window.location.href);
    expect(document.querySelector('.runtime-state-icon')).toBeNull();
    expect(window.location.pathname).toBe('/');
    expect(window.location.search).toBe('');

    fireEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    expect(screen.getByRole('heading', { name: 'Create an account to continue setting up “Family Room”' })).toBeInTheDocument();
  });

  it('keeps authentication submissions disabled until the current form is valid', async () => {
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Sign in to Portico' });
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'Password123!' } });
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();

    fireEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    const create = screen.getByRole('button', { name: 'Create account' });
    expect(create).toBeDisabled();
    const [username, email] = screen.getAllByRole('textbox');
    fireEvent.change(username, { target: { value: 'valid-owner' } });
    fireEvent.change(email, { target: { value: 'owner@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'Password123!' } });
    expect(create).toBeEnabled();
  });

  it('opens a browser session after registration and completes the pending server claim', async () => {
    window.history.replaceState(null, '', '/claim?code=SETUP123&serverName=Family%20Room');
    const requests: Array<{ url: string; body?: Record<string, unknown> }> = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      requests.push({ url, body });
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      if (url.endsWith('/api/auth/register')) return Promise.resolve(response({ user: { id: 'user-1', username: 'new-owner', email: 'new@example.test' } }, true, 201));
      if (url.endsWith('/api/auth/login')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'new-owner', email: 'new@example.test', displayName: 'new-owner' } }));
      if (url.endsWith('/api/account/server-claims/by-code/complete')) return Promise.resolve(response({ server: hostedServer('server-1', 'Family Room'), serverCredential: 'credential' }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], pageInfo: { hasMore: false } }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Continue setting up your server “Family Room”' });
    fireEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    const [username, email] = screen.getAllByRole('textbox');
    fireEvent.change(username, { target: { value: 'new-owner' } });
    fireEvent.change(email, { target: { value: 'new@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'Password123!' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create account' }));

    expect(await screen.findByRole('heading', { name: 'No servers yet' })).toBeInTheDocument();
    const login = requests.find((request) => request.url.endsWith('/api/auth/login'))?.body;
    expect(login).toMatchObject({
      login: 'new-owner',
      password: 'Password123!',
      deviceName: 'Portico Web',
      devicePlatform: 'web',
      installationId: expect.any(String),
    });
    expect(requests.find((request) => request.url.endsWith('/api/account/server-claims/by-code/complete'))?.body)
      .toEqual({ claimCode: 'SETUP123' });
  });

  it('reconciles an ambiguously committed registration by signing in without replaying account creation', async () => {
    const requests: string[] = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      requests.push(url);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      if (url.endsWith('/api/auth/register')) {
        return new Promise<Response>((_resolve, reject) => init?.signal?.addEventListener('abort', () => reject(new DOMException('Timed out after commit', 'AbortError')), { once: true }));
      }
      if (url.endsWith('/api/auth/login')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', username: 'new-owner', email: 'new@example.test', displayName: 'new-owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], pageInfo: { hasMore: false } }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers({ shouldAdvanceTime: true });

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Sign in to Portico' });
    fireEvent.click(screen.getByRole('button', { name: 'Create an account' }));
    const [username, email] = screen.getAllByRole('textbox');
    fireEvent.change(username, { target: { value: 'new-owner' } });
    fireEvent.change(email, { target: { value: 'new@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'Password123!' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create account' }));
    await vi.advanceTimersByTimeAsync(12_100);

    expect(await screen.findByRole('heading', { name: 'No servers yet' })).toBeInTheDocument();
    expect(requests.filter((url) => url.endsWith('/api/auth/register'))).toHaveLength(1);
    expect(requests.filter((url) => url.endsWith('/api/auth/login'))).toHaveLength(1);
    vi.useRealTimers();
  });

  it('consumes local-login result parameters without retaining server detail in browser history', () => {
    expect(extractPorticoLoginResult('http://localhost:32500/?porticoLogin=error&porticoLoginError=raw+server+diagnostic&porticoLoginMessageId=auth.profile-selection-failed#/home')).toEqual({
      result: 'error',
      messageId: 'auth.profile-selection-failed',
      safeUrl: '/#/home',
    });
  });

  it('keeps fixture mode explicit and ready only in deliberate development/test configuration', async () => {
    render(<RuntimeProvider config={fixtureConfig} fixtureSourceLoader={loadFixtureSource}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText('Ready on EhlerFlix Test')).toBeInTheDocument();
  });

  it('renders a canonical recovery message and icon without exposing raw runtime diagnostics', async () => {
    const rawDiagnostic = 'raw-injected-certificate-stack-secret';
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => String(input).endsWith('/api/system')
      ? Promise.resolve(response(serverSystem))
      : Promise.reject(new TypeError(rawDiagnostic))));
    render(<RuntimeProvider config={bundledConfig}><RuntimeHarness /></RuntimeProvider>);

    const copy = ClientCore.productMessage('problem.server-unavailable', { serverName: 'this server' });
    expect(await screen.findByRole('heading', { name: copy.title })).toBeInTheDocument();
    expect(screen.getByText(copy.body!)).toBeInTheDocument();
    expect(document.querySelector('[data-semantic-icon="status.server"]')).not.toBeNull();
    expect(screen.queryByText(rawDiagnostic)).not.toBeInTheDocument();
  });

  it('uses canonical invalid-credential copy and icon instead of an injected API detail', async () => {
    const rawDiagnostic = 'raw-injected-login-database-detail';
    let loginBody: Record<string, unknown> | undefined;
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      if (url.endsWith('/api/auth/login')) {
        loginBody = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
        return Promise.resolve(response({ code: 'invalid_credentials', detail: rawDiagnostic }, false, 401));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Sign in to Portico' });
    expect(screen.getByText('Sign in using your Portico Account credentials.')).toBeInTheDocument();
    expect(screen.queryByText('Hosted Services authorizes access; media remains on your server.')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'incorrect password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    const copy = ClientCore.productMessage('auth.invalid-credentials');
    expect(await screen.findByText(copy.body!)).toBeInTheDocument();
    expect(document.querySelector('[data-semantic-icon="status.locked"]')).not.toBeNull();
    expect(screen.queryByText(rawDiagnostic)).not.toBeInTheDocument();
    expect(loginBody).toMatchObject({
      deviceName: 'Portico Web',
      devicePlatform: 'web',
      login: 'owner@example.test',
      installationId: expect.any(String),
    });
  });

  it('uses a final fresh /auth/me in DataProvider before mounting the bundled viewer', async () => {
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(serverSystem));
      if (url.endsWith('/api/product-contract')) return Promise.resolve(response(serverProductContract));
      if (url.endsWith('/api/auth/capabilities')) return Promise.resolve(response({ setupRequired: false, localCredentialsEnabled: true, porticoAccountAuthEnabled: false, serverFriendlyName: 'Family Media', publicUserPickerEnabled: false, visibleUsers: [], generatedAt: new Date().toISOString() }));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response(bundledOwner));
      if (url.endsWith('/api/auth/browser-accounts')) return Promise.resolve(response({ accounts: [], automaticSignIn: true, selectionRequired: false, canAddAccount: true }));
      if (url.endsWith('/api/events')) return new Promise(() => undefined);
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={bundledConfig}><ReadyAuthHarness /></RuntimeProvider>);
    expect(await screen.findByText('ready:Owner')).toBeInTheDocument();
    await waitFor(() => {
      const calls = fetchMock.mock.calls.map((call) => String(call[0]));
      expect(calls.filter((url) => url.endsWith('/api/system'))).toHaveLength(2);
      expect(calls).toEqual(expect.arrayContaining([expect.stringContaining('/api/product-contract'), expect.stringContaining('/api/auth/capabilities'), expect.stringContaining('/api/auth/me'), expect.stringContaining('/api/auth/browser-accounts'), expect.stringContaining('/api/events')]));
    });
  });

  it('denies a quarantined bundled cookie after restart until cleanup or explicit verified re-authentication', async () => {
    expect(beginAmbientCookieMutation('browser-account-switch', {
      state: 'authenticated',
      expected: {authority: 'hosted', accountId: 'user-1'},
    })).toBeDefined();
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(serverSystem));
      if (url.endsWith('/api/auth/capabilities')) return Promise.resolve(response({
        setupRequired: false, localCredentialsEnabled: true, porticoAccountAuthEnabled: false, serverFriendlyName: 'Family Media',
        publicUserPickerEnabled: false, visibleUsers: [], generatedAt: new Date().toISOString(),
      }));
      if (url.endsWith('/api/auth/logout')) return Promise.reject(new TypeError('Logout unavailable'));
      if (url.endsWith('/api/auth/browser-accounts')) return Promise.resolve(response({accounts: [], automaticSignIn: true, selectionRequired: false, canAddAccount: true}));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response(bundledOwner));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={bundledConfig}><BundledLocalAuthHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', {name: 'Sign in to Family Media'})).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/api/auth/logout'))).toBe(true);
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/api/auth/me'))).toBe(false);
  });

  it('uses capabilities-first bundled bootstrap for first-run setup even before either sign-in method exists', async () => {
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(serverSystem));
      if (url.endsWith('/api/auth/capabilities')) return Promise.resolve(response({ setupRequired: true, localCredentialsEnabled: false, porticoAccountAuthEnabled: false, serverFriendlyName: 'Family Media', publicUserPickerEnabled: false, visibleUsers: [], generatedAt: new Date().toISOString() }));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false, setupRequired: true, serverFriendlyName: 'Family Media' }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={bundledConfig}><ReadyAppHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Set Up Your Portico Server' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Use A Portico Account/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Sign in directly to a server/ }));
    expect(screen.getByRole('button', { name: 'Create This Server owner' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Terms of Use' })).toHaveAttribute('href', 'https://getportico.tv/terms/');
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toHaveAttribute('href', 'https://getportico.tv/privacy/');
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/system');
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/auth/capabilities');
  });

  it('renders only the sign-in methods advertised by a claimed bundled server', async () => {
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(serverSystem));
      if (url.endsWith('/api/auth/capabilities')) return Promise.resolve(response({ setupRequired: false, localCredentialsEnabled: true, porticoAccountAuthEnabled: true, serverFriendlyName: 'Family Media', publicUserPickerEnabled: false, visibleUsers: [], generatedAt: new Date().toISOString() }));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false, setupRequired: false, serverFriendlyName: 'Family Media' }));
      if (url.endsWith('/api/auth/browser-accounts')) return Promise.resolve(response({ accounts: [], automaticSignIn: true, selectionRequired: false, canAddAccount: true }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={bundledConfig}><ReadyAppHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Sign in to Family Media' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Sign in with This Server/ })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Continue with Portico Account' })).toHaveAttribute('href', expect.stringContaining('/api/auth/portico/start'));
    expect(screen.getByRole('link', { name: 'Terms of Use' })).toHaveAttribute('href', 'https://getportico.tv/terms/');
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toHaveAttribute('href', 'https://getportico.tv/privacy/');
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it('keeps bundled Local Auth provisional through profile choice and a four-digit PIN, then binds the final /me', async () => {
    const profiles = [
      { id: 'profile-owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: unrestrictedPolicy },
      { id: 'profile-kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: true, pinRevision: 3, sortOrder: 1, policy: unrestrictedPolicy },
    ];
    const unauthenticated = { authenticated: false, setupRequired: false, serverFriendlyName: 'Family Media' };
    let installationId = '';
    let sessionPublished = false;
    const localViewer = {
      ...bundledOwner,
      accountId: 'local-account',
      profileId: 'profile-kids',
      authorizationRevision: 'policy-kids',
      user: { ...bundledOwner.user, id: 'local-account', displayName: 'Kids' },
    };
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(serverSystem));
      if (url.endsWith('/api/product-contract')) return Promise.resolve(response(serverProductContract));
      if (url.endsWith('/api/auth/capabilities')) return Promise.resolve(response({ setupRequired: false, localCredentialsEnabled: true, porticoAccountAuthEnabled: false, serverFriendlyName: 'Family Media', publicUserPickerEnabled: false, visibleUsers: [], generatedAt: new Date().toISOString() }));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response(sessionPublished ? localViewer : unauthenticated));
      if (url.endsWith('/api/auth/browser-accounts')) return Promise.resolve(response({ accounts: [], automaticSignIn: true, selectionRequired: false, canAddAccount: true }));
      if (url.endsWith('/api/auth/profile-authentications/local')) {
        installationId = String(JSON.parse(String(init?.body)).installationId);
        return Promise.resolve(response({ accountAuthenticationToken: 'local-proof', expiresAt: new Date(Date.now() + 60_000).toISOString(), directory: { authority: 'local', accountId: 'local-account', serverId: 'server-1', profilesAllowed: true, profiles } }));
      }
      if (url.endsWith('/api/auth/profile-selections/local')) return Promise.resolve(response({ token: 'profile-grant', authority: 'local', accountId: 'local-account', serverId: 'server-1', profileId: 'profile-kids', pinRevision: 3, installationId, expiresAt: new Date(Date.now() + 60_000).toISOString() }));
      if (url.endsWith('/api/auth/profile-sessions/browser')) {
        sessionPublished = true;
        return Promise.resolve(response(localViewer));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={bundledConfig}><BundledLocalAuthHarness /></RuntimeProvider>);

    await screen.findByRole('heading', { name: 'Sign in to Family Media' });
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in with This Server' }));
    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('auth.profile-selection-required').title })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Kids.*PIN required/ }));
    expect(screen.getByRole('link', { name: 'Terms of Use' })).toHaveAttribute('href', 'https://getportico.tv/terms/');
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toHaveAttribute('href', 'https://getportico.tv/privacy/');
    const pinInput = screen.getByLabelText('Kids profile PIN');
    const openProfile = screen.getByRole('button', { name: 'Open profile' });
    fireEvent.change(pinInput, { target: { value: '123' } });
    expect(openProfile).toBeDisabled();
    fireEvent.change(pinInput, { target: { value: '12x345' } });
    expect(pinInput).toHaveValue('1234');
    expect(openProfile).toBeEnabled();
    fireEvent.click(openProfile);

    expect(await screen.findByText('ready:Kids')).toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/auth/me'))).toHaveLength(3);
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/api/auth/profile-sessions/browser'))).toBe(true);
  });

  it('keeps the Local profile chooser isolated on an incorrect PIN and shows canonical Product Language copy', async () => {
    const profiles = [
      { id: 'profile-owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: unrestrictedPolicy },
      { id: 'profile-kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: true, pinRevision: 1, sortOrder: 1, policy: unrestrictedPolicy },
    ];
    let installationId = '';
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(serverSystem));
      if (url.endsWith('/api/product-contract')) return Promise.resolve(response(serverProductContract));
      if (url.endsWith('/api/auth/capabilities')) return Promise.resolve(response({ setupRequired: false, localCredentialsEnabled: true, porticoAccountAuthEnabled: false, serverFriendlyName: 'Family Media', publicUserPickerEnabled: false, visibleUsers: [], generatedAt: new Date().toISOString() }));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false, setupRequired: false, serverFriendlyName: 'Family Media' }));
      if (url.endsWith('/api/auth/browser-accounts')) return Promise.resolve(response({ accounts: [], automaticSignIn: true, selectionRequired: false, canAddAccount: true }));
      if (url.endsWith('/api/auth/profile-authentications/local')) {
        installationId = String(JSON.parse(String(init?.body)).installationId);
        return Promise.resolve(response({ accountAuthenticationToken: 'local-proof', expiresAt: new Date(Date.now() + 60_000).toISOString(), directory: { authority: 'local', accountId: 'local-account', serverId: 'server-1', profilesAllowed: true, profiles } }));
      }
      if (url.endsWith('/api/auth/profile-selections/local')) return Promise.resolve(response({ code: 'profile_pin_invalid', installationId }, false, 401));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={bundledConfig}><BundledLocalAuthHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Sign in to Family Media' });
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in with This Server' }));
    fireEvent.click(await screen.findByRole('button', { name: /Kids.*PIN required/ }));
    fireEvent.change(screen.getByLabelText('Kids profile PIN'), { target: { value: '9999' } });
    fireEvent.click(screen.getByRole('button', { name: 'Open profile' }));

    expect(await screen.findByText(ClientCore.productMessage('auth.profile-pin-invalid').body!)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: ClientCore.productMessage('auth.profile-selection-required').title })).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/api/auth/profile-sessions/browser'))).toBe(false);
  });

  it('shows Hosted account sign-in before any server data is requested', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: false }));
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Sign in to Portico' })).toBeInTheDocument();
    expect(screen.getByText('Sign in using your Portico Account credentials.')).toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([input]) => String(input).startsWith(hostedConfig.hostedApiBaseUrl))).toHaveLength(2);
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/system');
    expect(String(fetchMock.mock.calls[1][0])).toContain('/api/auth/me');
  });

  it('completes an authenticated local-server handoff without entering direct-route discovery', async () => {
    const callbackUrl = 'http://localhost:32500/api/auth/portico/callback';
    const state = 'state-token-with-enough-entropy';
    window.history.replaceState(null, '', `/#/local-login?serverId=server-1&serverName=Family%20Media&callbackUrl=${encodeURIComponent(callbackUrl)}&localOrigin=${encodeURIComponent('http://localhost:32500')}&state=${state}&serverPublicKeyFingerprint=sha256%3Afingerprint`);
    const navigate = vi.fn();
    const fetchMock = vi.fn((input: string | URL | Request, _init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [{ id: 'profile-1', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true } }], total: 1, revision: 1 }));
      if (url.endsWith('/api/account/servers/server-1/local-login/authorize')) return Promise.resolve(response({ redirectUrl: `${callbackUrl}?code=one-time-code&state=${state}` }, true, 201));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} navigate={navigate}><RuntimeHarness /></RuntimeProvider>);

    await vi.waitFor(() => expect(navigate).toHaveBeenCalledWith(`${callbackUrl}?code=one-time-code&state=${state}`));
    expect(window.location.hash).toBe('');
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/account/servers'))).toBe(false);
    const authorizeCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/local-login/authorize'));
    expect(JSON.parse(String(authorizeCall?.[1]?.body))).toMatchObject({ callbackUrl, localOrigin: 'http://localhost:32500', state, serverPublicKeyFingerprint: 'sha256:fingerprint', profileId: 'profile-1' });
  });

  it('returns a fresh Hosted sign-in to the requesting local server instead of opening the normal server chooser', async () => {
    const callbackUrl = 'http://localhost:32500/api/auth/portico/callback';
    const state = 'state-token-with-enough-entropy';
    window.history.replaceState(null, '', `/#/local-login?serverId=server-1&serverName=Family%20Media&callbackUrl=${encodeURIComponent(callbackUrl)}&localOrigin=${encodeURIComponent('http://localhost:32500')}&state=${state}&serverPublicKeyFingerprint=sha256%3Afingerprint`);
    const navigate = vi.fn();
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      if (url.endsWith('/api/auth/login')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [{ id: 'profile-1', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true } }], total: 1, revision: 1 }));
      if (url.endsWith('/api/account/servers/server-1/local-login/authorize')) return Promise.resolve(response({ redirectUrl: `${callbackUrl}?code=one-time-code&state=${state}` }, true, 201));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} navigate={navigate}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: 'Sign in to Portico' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    await vi.waitFor(() => expect(navigate).toHaveBeenCalledWith(`${callbackUrl}?code=one-time-code&state=${state}`));
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/account/servers'))).toBe(false);
  });

  it('keeps a successful Hosted account session when the local-login profile directory fails', async () => {
    const callbackUrl = 'http://localhost:32500/api/auth/portico/callback';
    const state = 'profile-failure-state-token-with-enough-entropy';
    window.history.replaceState(null, '', `/#/local-login?serverId=server-1&serverName=Family%20Media&callbackUrl=${encodeURIComponent(callbackUrl)}&localOrigin=${encodeURIComponent('http://localhost:32500')}&state=${state}&serverPublicKeyFingerprint=sha256%3Afingerprint`);
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: false }));
      if (url.endsWith('/api/auth/login')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.endsWith('/api/account/profiles')) return Promise.resolve(response({ code: 'invalid_profile', messageId: 'auth.invalid-profile' }, false, 400));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], total: 0 }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Sign in to Portico' });
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    const copy = ClientCore.productMessage('problem.profile-request-failed');
    expect(await screen.findByRole('heading', { name: copy.title })).toBeInTheDocument();
    expect(screen.getByText(copy.body!)).toBeInTheDocument();
    expect(screen.queryByText(/valid profile name|available avatar/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Continue to Portico' }));
    expect(await screen.findByRole('heading', { name: 'No servers yet' })).toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/auth/login'))).toHaveLength(1);
  });

  it('retries an interrupted local-login authorization without asking for credentials again', async () => {
    const callbackUrl = 'http://localhost:32500/api/auth/portico/callback';
    const state = 'authorization-retry-state-token-with-enough-entropy';
    window.history.replaceState(null, '', `/#/local-login?serverId=server-1&serverName=Family%20Media&callbackUrl=${encodeURIComponent(callbackUrl)}&localOrigin=${encodeURIComponent('http://localhost:32500')}&state=${state}&serverPublicKeyFingerprint=sha256%3Afingerprint`);
    const navigate = vi.fn();
    let accountAuthenticated = false;
    let authorizationAttempts = 0;
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response(accountAuthenticated
        ? { authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }
        : { authenticated: false }));
      if (url.endsWith('/api/auth/login')) {
        accountAuthenticated = true;
        return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      }
      if (url.endsWith('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [{ id: 'profile-1', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: unrestrictedPolicy }], total: 1, revision: 1 }));
      if (url.endsWith('/api/account/servers/server-1/local-login/authorize')) {
        authorizationAttempts += 1;
        return authorizationAttempts === 1
          ? Promise.reject(new TypeError('Failed to fetch'))
          : Promise.resolve(response({ redirectUrl: `${callbackUrl}?code=one-time-code&state=${state}` }, true, 201));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} navigate={navigate}><RuntimeHarness /></RuntimeProvider>);
    await screen.findByRole('heading', { name: 'Sign in to Portico' });
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'owner@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.cloud-unavailable').title })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await vi.waitFor(() => expect(navigate).toHaveBeenCalledWith(`${callbackUrl}?code=one-time-code&state=${state}`));
    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/auth/login'))).toHaveLength(1);
    expect(authorizationAttempts).toBe(2);
  });

  it('reports a transient Hosted session check as account-service unavailability rather than expiration', async () => {
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ code: 'store_unavailable' }, false, 503));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.cloud-unavailable').title })).toBeInTheDocument();
    expect(screen.queryByText(ClientCore.productMessage('auth.session-expired').body!)).not.toBeInTheDocument();
    expect(screen.getByText('Portico couldn’t reach account services. It will keep trying automatically.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
  });

	it('recovers the short-lived local-server handoff after the sensitive fragment is scrubbed and the page reloads', async () => {
		const callbackUrl = 'http://localhost:32500/api/auth/portico/callback';
		const state = 'reload-safe-state-token-with-enough-entropy';
		window.history.replaceState(null, '', `/#/local-login?serverId=server-1&serverName=Family%20Media&callbackUrl=${encodeURIComponent(callbackUrl)}&localOrigin=${encodeURIComponent('http://localhost:32500')}&state=${state}&serverPublicKeyFingerprint=sha256%3Afingerprint`);
		const firstFetch = vi.fn((input: string | URL | Request) => {
			const url = String(input);
			if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
			if (url.endsWith('/api/auth/me')) return new Promise<Response>(() => undefined);
			return Promise.reject(new Error(`Unexpected request: ${url}`));
		});
		vi.stubGlobal('fetch', firstFetch);
		const first = render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
		await vi.waitFor(() => expect(window.location.hash).toBe(''));
		expect(window.sessionStorage.length).toBe(1);
		first.unmount();

		const navigate = vi.fn();
		const secondFetch = vi.fn((input: string | URL | Request) => {
			const url = String(input);
			if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
			if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
			if (url.includes('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [{ id: 'profile-1', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: unrestrictedPolicy }], total: 1, revision: 1 }));
			if (url.endsWith('/api/account/servers/server-1/local-login/authorize')) return Promise.resolve(response({ redirectUrl: `${callbackUrl}?code=one-time-code&state=${state}` }, true, 201));
			return Promise.reject(new Error(`Unexpected request: ${url}`));
		});
		vi.stubGlobal('fetch', secondFetch);
		render(<RuntimeProvider config={hostedConfig} navigate={navigate}><RuntimeHarness /></RuntimeProvider>);

		await vi.waitFor(() => expect(navigate).toHaveBeenCalledWith(`${callbackUrl}?code=one-time-code&state=${state}`));
		expect(secondFetch.mock.calls.some((call) => String(call[0]).endsWith('/api/account/servers'))).toBe(false);
			// The proof-bound handoff remains recoverable for its five-minute TTL
			// because browsers cannot report a denied localhost navigation back to JS.
			expect(window.sessionStorage.length).toBe(1);
		});

  it('renders the no-membership recovery surface after an authenticated Hosted session', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }))
      .mockResolvedValueOnce(response({ items: [], total: 0 }));
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'No servers yet' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Claim a server' })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByRole('tab', { name: 'Accept an invite' }));
    expect(screen.getByRole('button', { name: 'Accept invitation' })).toBeInTheDocument();
  });

  it('signs out locally at once and ignores a stale membership failure', async () => {
    let rejectMemberships: (reason: Error) => void = () => undefined;
    const memberships = new Promise<Response>((_resolve, reject) => { rejectMemberships = reject; });
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return memberships;
      if (url.endsWith('/api/auth/logout')) return Promise.resolve(response({ ok: true }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><HostedLogoutHarness /></RuntimeProvider>);

    expect(await screen.findByText('server-memberships')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Test sign out' }));
    expect(await screen.findByText('hosted-sign-in')).toBeInTheDocument();

    rejectMemberships(new TypeError('Failed to fetch'));
    await Promise.resolve();
    expect(screen.getByText('hosted-sign-in')).toBeInTheDocument();
  });

  it('keeps immediate local sign-out but warns when Hosted Services could not revoke the browser session', async () => {
    let rejectMemberships: (reason: Error) => void = () => undefined;
    const memberships = new Promise<Response>((_resolve, reject) => { rejectMemberships = reject; });
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return memberships;
      if (url.endsWith('/api/auth/logout')) return Promise.reject(new TypeError('Failed to fetch'));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><HostedLogoutHarness /></RuntimeProvider>);

    expect(await screen.findByText('server-memberships')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Test sign out' }));
    expect(await screen.findByText('hosted-sign-in')).toBeInTheDocument();
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();

    rejectMemberships(new TypeError('Failed to fetch'));
    await Promise.resolve();
    expect(screen.getByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
  });

  it('persists a global restart fence when sign-out precedes account discovery and remote logout fails', async () => {
    let resolveAccountCheck!: (value: Response) => void;
    const accountCheck = new Promise<Response>((resolve) => { resolveAccountCheck = resolve; });
    const vault = createBrowserHostedConnectionVault(undefined);
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return accountCheck;
      if (url.endsWith('/api/auth/logout')) return Promise.reject(new TypeError('cookie revocation failed'));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const first = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><HostedLogoutHarness /></RuntimeProvider>);

    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(screen.getByText('hosted-account-session')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Test sign out'}));

    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    expect(await screen.findByText('Sign out settled')).toBeInTheDocument();
    resolveAccountCheck(response({ authenticated: false }));
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(window.localStorage.getItem(`${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}${encodeURIComponent(GLOBAL_SIGN_OUT_FENCE_ID)}`)).not.toBeNull();
    expect(await vault.cleanupPendingAccounts()).toContain(GLOBAL_SIGN_OUT_FENCE_ID);
    first.unmount();
    resetSignedOutAccountQuarantineForTests();
    const restored = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/auth/me'))).toHaveLength(1);
    expect(screen.queryByText(/Ready on/)).not.toBeInTheDocument();
    restored.unmount();
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  });

  it('clears an unknown-account global fence for fresh account C only after every old A vault record is verified absent', async () => {
    const vault = await rememberedHostedBrowser(['server-1']);
    expect(markAccountSignedOut(GLOBAL_SIGN_OUT_FENCE_ID)).toBe(true);
    await vault.markCleanupPending(GLOBAL_SIGN_OUT_FENCE_ID);
    const accountC = { authenticated: true, user: { id: 'user-c', email: 'c@example.test', displayName: 'Account C' } };
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/login')) return Promise.resolve(response(accountC));
      if (url.endsWith('/api/auth/sessions/revoke')) return Promise.resolve(response({ ok: true }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], total: 0 }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'c@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByRole('heading', { name: 'No servers yet' })).toBeInTheDocument();
    expect(await vault.list('user-1')).toEqual([]);
    expect(await vault.knownAccountIds()).toEqual(['user-c']);
    expect(await vault.cleanupPendingAccounts()).not.toContain(GLOBAL_SIGN_OUT_FENCE_ID);
    expect(window.localStorage.getItem(`${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}${encodeURIComponent(GLOBAL_SIGN_OUT_FENCE_ID)}`)).toBeNull();
  });

  it('retains the global fence and blocks fresh account C when old-account enumeration cannot be trusted', async () => {
    const baseVault = await rememberedHostedBrowser(['server-1']);
    expect(markAccountSignedOut(GLOBAL_SIGN_OUT_FENCE_ID)).toBe(true);
    await baseVault.markCleanupPending(GLOBAL_SIGN_OUT_FENCE_ID);
    const failingEnumeration: HostedConnectionVault = {
      ...baseVault,
      knownAccountIds: async () => { throw new Error('old account enumeration failed'); },
    };
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/login')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-c', email: 'c@example.test', displayName: 'Account C' } }));
      if (url.endsWith('/api/auth/logout')) return Promise.resolve(response({ ok: true }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={failingEnumeration}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Username or email'), { target: { value: 'c@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery staple' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    expect(window.localStorage.getItem(`${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}${encodeURIComponent(GLOBAL_SIGN_OUT_FENCE_ID)}`)).not.toBeNull();
    expect(await baseVault.cleanupPendingAccounts()).toContain(GLOBAL_SIGN_OUT_FENCE_ID);
    expect(screen.queryByText(/Ready on/)).not.toBeInTheDocument();
  });

  it('blocks a fresh provider restore when credential deletion failed after sign-out', async () => {
    const baseVault = createBrowserHostedConnectionVault(undefined);
    const failingVault: HostedConnectionVault = {
      ...baseVault,
      forgetAccount: async () => { throw new Error('credential deletion failed'); },
    };
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], total: 0 }));
      if (url.endsWith('/api/auth/logout')) return Promise.resolve(response({ ok: true }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const first = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={failingVault}><HostedLogoutHarness /></RuntimeProvider>);
    expect(await screen.findByText('no-memberships')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Test sign out' }));
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    first.unmount();

    // Simulate a new JS process while retaining localStorage and vault state.
    resetSignedOutAccountQuarantineForTests();
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={failingVault}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    expect(screen.queryByText(/Ready on/)).not.toBeInTheDocument();
  });

  it('cannot authenticate from retained metadata after ledger, barrier, deletion, revocation, and logout failures', async () => {
    const storage = sharedVaultStorage();
    const durableMetadata = () => createHostedConnectionVault(storage, () => new Date('2026-07-14T12:00:00.000Z'), { persistCredentials: false });
    const firstProcess = createBrowserHostedConnectionVaultWithDurableMetadata(durableMetadata());
    await firstProcess.rememberAccount({ accountId: 'user-1', displayName: 'Owner', email: 'owner@example.test' });
    await firstProcess.save({
      schemaVersion: 2,
      accountId: 'user-1',
      serverId: 'server-1',
      profileId: 'profile-1',
      serverName: 'Family Media',
      serverPublicKeyFingerprint: testServerFingerprint('server-1'),
      currentRoute: { url: 'https://server-1.direct.getportico.tv', type: 'public_direct', verifiedAt: '2026-07-14T11:00:00.000Z' },
      session: { serverId: 'server-1', serverName: 'Family Media', apiBaseUrl: 'https://server-1.direct.getportico.tv', accessToken: 'access-server-1', refreshToken: 'refresh-server-1', serverPublicKeyFingerprint: testServerFingerprint('server-1') },
      lastSuccessfulConnectionAt: '2026-07-14T11:00:00.000Z',
    });
    const failingCleanup: HostedConnectionVault = {
      ...firstProcess,
      markCleanupPending: async () => { throw new Error('barrier write failed'); },
      forgetAccount: async () => { throw new Error('credential deletion failed'); },
    };
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], total: 0 }));
      if (url.endsWith('/api/auth/logout') || url.includes('/api/auth/sessions/revoke')) return Promise.reject(new TypeError('revocation unavailable'));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const first = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={failingCleanup}><HostedLogoutHarness /></RuntimeProvider>);
    expect(await screen.findByText('no-memberships')).toBeInTheDocument();
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('storage unavailable', 'SecurityError'); });
    fireEvent.click(screen.getByRole('button', { name: 'Test sign out' }));
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    first.unmount();

    resetSignedOutAccountQuarantineForTests();
    const freshProcess = createBrowserHostedConnectionVaultWithDurableMetadata(durableMetadata());
    expect(await freshProcess.load('user-1', 'server-1')).toBeUndefined();
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={freshProcess}><RuntimeHarness /></RuntimeProvider>);
    // The still-authenticated Hosted /me cookie is positive online authority;
    // retained IndexedDB metadata alone never opens the server credential.
    expect(await screen.findByRole('heading', { name: 'No servers yet' })).toBeInTheDocument();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('/api/remote-access/health'))).toBe(false);
    expect(screen.queryByText(/Ready on/)).not.toBeInTheDocument();
  });

  it('renders a server chooser for multiple authorized servers without probing either one', async () => {
    const servers = [
      { id: 'server-1', ownerUserId: 'user-1', name: 'Family Media', serverPublicKey: 'key', serverPublicKeyFingerprint: 'fingerprint', assignedHostname: 'family.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' },
      { id: 'server-2', ownerUserId: 'user-1', name: 'Cinema', serverPublicKey: 'key', serverPublicKeyFingerprint: 'fingerprint', assignedHostname: 'cinema.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' },
    ];
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }))
      .mockResolvedValueOnce(response({ items: servers, total: 2 }));
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Choose a server' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Family Media/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Cinema/ })).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/routes'))).toBe(false);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/sessions'))).toBe(false);
  });

  it('renders an explicit Hosted profile selector and PIN entry instead of a retry dead end', async () => {
    const server = { id: 'server-1', ownerUserId: 'user-1', name: 'Family Media', serverPublicKey: 'key', serverPublicKeyFingerprint: 'fingerprint', assignedHostname: 'family.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' };
    const unrestrictedPolicy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const profiles = [
      { id: 'profile-1', name: 'Justin', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 0 },
      { id: 'profile-2', name: 'Guest', hasPIN: true, isPrimary: false, isAccountAdmin: false, pinRevision: 1, policy: unrestrictedPolicy, sortOrder: 1 },
    ];
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }))
      .mockResolvedValueOnce(response({ items: [server], total: 1 }))
      .mockResolvedValueOnce(response({ accountId: 'user-1', profiles, revision: 1, total: 2 }));
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: "Who's watching?" })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Justin.*Open profile/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Guest.*PIN required/ }));
    const pin = screen.getByLabelText('Enter Guest’s PIN');
    expect(pin).toHaveAttribute('maxlength', '4');
    expect(pin).toHaveAttribute('pattern', '[0-9]{4}');
    const openProfile = screen.getByRole('button', { name: 'Open profile' });
    fireEvent.change(pin, { target: { value: '12a345' } });
    expect(pin).toHaveValue('1234');
    expect(openProfile).toBeEnabled();
    const hostedRequests = fetchMock.mock.calls
      .map(([input]) => String(input))
      .filter((url) => url.startsWith(hostedConfig.hostedApiBaseUrl));
    expect(hostedRequests).toEqual(expect.arrayContaining([
      'https://web.getportico.tv/api/system',
      'https://web.getportico.tv/api/auth/me',
      'https://web.getportico.tv/api/account/servers?limit=100',
      'https://web.getportico.tv/api/account/profiles',
    ]));
    expect(hostedRequests.some((url) => url.includes('/routes') || url.includes('/sessions'))).toBe(false);
  });

  it('uses the cached ask policy on restart instead of silently opening the remembered profile', async () => {
    const vault = await rememberedHostedBrowser();
    const installationId = await vault.installationId();
    await vault.saveProfileLaunchPreference({ authority: 'hosted', accountId: 'user-1', serverId: 'server-1', profileId: 'profile-1', deviceClass: 'web', installationId }, { rememberAccount: true, profileSelection: 'ask', lastProfileId: 'profile-1' });
    const server = { id: 'server-1', ownerUserId: 'user-1', name: 'Family Media', serverPublicKey: 'key', serverPublicKeyFingerprint: testServerFingerprint('server-1'), assignedHostname: 'family.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' };
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }))
      .mockResolvedValueOnce(response({ items: [server], total: 1 }))
      .mockResolvedValueOnce(response({ accountId: 'user-1', revision: 1, total: 2, profiles: [
        { id: 'profile-1', name: 'Justin', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy, sortOrder: 0 },
        { id: 'profile-2', name: 'Guest', hasPIN: false, isPrimary: false, isAccountAdmin: false, pinRevision: 0, policy, sortOrder: 1 },
      ] }));
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: "Who's watching?" })).toBeInTheDocument();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('/selection-assertions'))).toBe(false);
  });

  it('reopens a locked last-used Hosted profile only with matching installation trust and remembered server session', async () => {
    const vault = await rememberedHostedBrowser();
    const installationId = await vault.installationId();
    const scope = { authority: 'hosted' as const, accountId: 'user-1', serverId: 'server-1', profileId: 'profile-1', deviceClass: 'web' as const, installationId };
    await vault.saveProfileLaunchPreference(scope, { rememberAccount: true, profileSelection: 'last-used', lastProfileId: 'profile-1' });
    await vault.saveAutomaticProfileTrust({ version: 'v1', purpose: 'automatic-profile-selection', token: 'hosted-session-trust', authority: 'hosted', accountId: 'user-1', serverId: 'server-1', profileId: 'profile-1', pinRevision: 3, installationId, expiresAt: '2099-01-01T00:00:00.000Z' });
    const server = { id: 'server-1', ownerUserId: 'user-1', name: 'Family Media', serverPublicKey: 'key', serverPublicKeyFingerprint: testServerFingerprint('server-1'), assignedHostname: 'family.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' };
    const policy = { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true };
    const fetchMock = vi.fn((input: string | URL | Request, _init?: RequestInit) => {
      const url = String(input);
      if (url === 'https://web.getportico.tv/api/system') return Promise.resolve(response(hostedSystem));
      if (url === 'https://web.getportico.tv/api/auth/me') return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [server], total: 1 }));
      if (url === 'https://web.getportico.tv/api/account/profiles') return Promise.resolve(response({ accountId: 'user-1', revision: 1, total: 1, profiles: [{ id: 'profile-1', name: 'Justin', hasPIN: true, isPrimary: true, isAccountAdmin: true, pinRevision: 3, policy, sortOrder: 0 }] }));
      if (url === 'https://server-1.direct.getportico.tv/api/remote-access/health') return Promise.resolve(response({ serverId: 'server-1', serverPublicKeyFingerprint: testServerFingerprint('server-1'), remoteAccessEnabled: true }));
      if (url === 'https://server-1.direct.getportico.tv/api/system') return Promise.resolve(response(serverSystem));
      if (url === 'https://server-1.direct.getportico.tv/api/auth/me') return Promise.resolve(response({ authenticated: true, setupRequired: false, serverFriendlyName: 'Family Media', accountMode: 'portico', authority: 'hosted', accountId: 'user-1', serverId: 'server-1', profileId: 'profile-1', authorizationRevision: 'policy-1', user: { id: 'profile-1', displayName: 'Justin', email: 'owner@example.test', role: 'owner', authOrigin: 'portico', authProvider: 'portico', hasLocalPassword: false } }));
      if (url === 'https://server-1.direct.getportico.tv/api/product-contract') return Promise.resolve(response(serverProductContract));
      if (url === 'https://server-1.direct.getportico.tv/api/account/profile-trusts/redeem') return Promise.resolve(response(undefined, true, 204));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByText('Ready on Family Media')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('/selection-assertions'))).toBe(false);
    const redemption = fetchMock.mock.calls.find((call) => String(call[0]).includes('/profile-trusts/redeem'));
    expect(redemption?.[1]).toEqual(expect.objectContaining({ method: 'POST', body: JSON.stringify({ token: 'hosted-session-trust' }) }));
  });

  it('lets the newest server choice finish when an abort-ignoring predecessor never settles', async () => {
    const vault = await rememberedHostedBrowser(['server-1', 'server-2']);
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.reject(new TypeError('membership service unavailable'));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const connector = vi.spyOn(ClientCore, 'connectTrustedServerRecord').mockImplementation((record, options) => {
      if (record.serverId === 'server-1') return new Promise(() => undefined);
      return publishPreparedConnection(preparedConnection(record.serverId, record.serverName, record.profileId), options);
    });

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><DirectChoiceHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', { name: 'Direct server-1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-1' }));
    await vi.waitFor(() => expect(connector).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-2' }));

    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    expect(connector.mock.calls.map(([record]) => record.serverId)).toEqual(['server-1', 'server-2']);
    expect(screen.getByLabelText('ready-profile')).toHaveTextContent('profile-1');
  });

  it('makes concurrent profile verification latest-choice-wins even when the older envelope request ignores abort', async () => {
    const server = hostedServer('server-1', 'Family Media');
    const profiles = [
      { id: 'profile-1', name: 'Adult', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 0 },
      { id: 'profile-2', name: 'Guest', hasPIN: false, isPrimary: false, isAccountAdmin: false, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 1 },
    ];
    const ignoredEnvelope = new Promise<Response>(() => undefined);
    const profileSelectionBodies: Array<Record<string, unknown>> = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [server], total: 1 }));
      if (url.endsWith('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles, revision: 1, total: 2 }));
      if (url.includes('/api/account/profiles/profile-1/selection-assertions')) {
        profileSelectionBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>);
        return ignoredEnvelope;
      }
      if (url.includes('/api/account/profiles/profile-2/selection-assertions')) {
        profileSelectionBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>);
        return Promise.resolve(response({
        version: 'v1',
        accountId: 'user-1',
        accountRevision: 1,
        assertionId: 'assertion-profile-2',
        audience: 'portico-media-server',
        deviceId: 'browser-device',
        expiresAt: '2026-07-14T12:05:00.000Z',
        installationId: 'test-installation',
        issuedAt: '2026-07-14T12:00:00.000Z',
        pinRevision: 0,
        profileId: 'profile-2',
        profiles: [],
        serverId: 'server-1',
        signature: 'signature',
        signatureAlgorithm: 'ed25519',
        signatureKeyId: 'key-1',
        }));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const connector = vi.spyOn(ClientCore, 'connectResilientHostedServer').mockImplementation((selectedServer, options) => {
      return publishPreparedConnection(preparedConnection(selectedServer.id, selectedServer.name, options.selectionEnvelope.profileId), options);
    });
    const vault = createBrowserHostedConnectionVaultWithDurableMetadata(
      createHostedConnectionVault(sharedVaultStorage(), () => new Date('2026-07-14T12:00:00.000Z'), { persistCredentials: false }),
    );
    vi.spyOn(vault, 'installationId').mockResolvedValue('test-installation');

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><DirectChoiceHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', { name: 'Direct profile-1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Direct profile-1' }));
    await vi.waitFor(() => expect(fetchMock.mock.calls.some(([input]) => String(input).includes('profile-1/selection-assertions'))).toBe(true));
    fireEvent.click(screen.getByRole('button', { name: 'Direct profile-2' }));

    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    expect(connector).toHaveBeenCalledOnce();
    expect(screen.getByLabelText('ready-profile')).toHaveTextContent('profile-2');
    expect(screen.getByLabelText('connection-warning')).toHaveTextContent('none');
    expect(profileSelectionBodies).toEqual([
      {serverId: 'server-1'},
      {serverId: 'server-1'},
    ]);
  });

  it('reauthorizes after a fresh browser process and remints server credentials before publishing product UI', async () => {
    const storage = sharedVaultStorage();
    const durableMetadata = () => createHostedConnectionVault(storage, () => new Date('2026-07-14T12:00:00.000Z'), { persistCredentials: false });
    const firstProcess = createBrowserHostedConnectionVaultWithDurableMetadata(durableMetadata());
    const server = hostedServer('server-1', 'Family Media');
    const profile = { id: 'profile-1', name: 'Owner', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 0 };
    let events: string[] = [];
    const mintedTokens: string[] = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) { events.push('hosted-system'); return Promise.resolve(response(hostedSystem)); }
      if (url === 'https://web.getportico.tv/api/auth/me') { events.push('hosted-me'); return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } })); }
      if (url.includes('/api/account/servers')) { events.push('memberships'); return Promise.resolve(response({ items: [server], total: 1 })); }
      if (url.endsWith('/api/account/profiles')) { events.push('profiles'); return Promise.resolve(response({ accountId: 'user-1', profiles: [profile], revision: 1, total: 1 })); }
      if (url.includes('/selection-assertions')) {
        events.push('profile-envelope');
        const body = JSON.parse(String(init?.body ?? '{}')) as { installationId?: string; serverId?: string };
        return Promise.resolve(response({
          version: 'v1', accountId: 'user-1', accountRevision: 1, assertionId: 'assertion-profile-1', audience: 'portico-media-server', deviceId: 'browser-device',
          expiresAt: '2026-07-14T12:05:00.000Z', installationId: body.installationId, issuedAt: '2026-07-14T12:00:00.000Z', pinRevision: 0,
          profileId: 'profile-1', profiles: [], serverId: body.serverId, signature: 'signature', signatureAlgorithm: 'ed25519', signatureKeyId: 'key-1',
        }));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const connector = vi.spyOn(ClientCore, 'connectResilientHostedServer').mockImplementation((selectedServer, options) => {
      const candidate = preparedConnection(selectedServer.id, selectedServer.name, options.selectionEnvelope.profileId);
      const token = `access-remint-${mintedTokens.length + 1}`;
      candidate.session.accessToken = token;
      candidate.record.session.accessToken = token;
      mintedTokens.push(token);
      events.push('server-remint');
      return publishPreparedConnection(candidate, options);
    });

    const first = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={firstProcess}><DirectChoiceHarness /></RuntimeProvider>);
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    expect(screen.getByLabelText('connection-warning')).toHaveTextContent('none');
    expect(mintedTokens).toEqual(['access-remint-1']);
    first.unmount();

    const freshProcess = createBrowserHostedConnectionVaultWithDurableMetadata(durableMetadata());
    expect(freshProcess.persistencePolicy).toBe('reauthorize-on-start');
    expect(freshProcess.durability()).toBe('durable');
    expect(await freshProcess.load('user-1', 'server-1')).toBeUndefined();
    events = [];
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={freshProcess}><DirectChoiceHarness /></RuntimeProvider>);

    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    expect(connector).toHaveBeenCalledTimes(2);
    expect(mintedTokens).toEqual(['access-remint-1', 'access-remint-2']);
    expect(events.indexOf('hosted-me')).toBeLessThan(events.indexOf('memberships'));
    expect(events.indexOf('memberships')).toBeLessThan(events.indexOf('profile-envelope'));
    expect(events.indexOf('profile-envelope')).toBeLessThan(events.indexOf('server-remint'));
    expect(screen.getByLabelText('connection-warning')).toHaveTextContent('none');
  });

  it('latches a current-generation activation rollback failure and refuses later server choices', async () => {
    const vault = await rememberedHostedBrowser(['server-1', 'server-2']);
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url.endsWith('/api/auth/me')) return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.reject(new TypeError('membership service unavailable'));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const connector = vi.spyOn(ClientCore, 'connectTrustedServerRecord').mockRejectedValue(
      new ClientCore.TrustedServerCandidateActivationError(new AggregateError([new Error('runtime rollback failed')], 'unsafe rollback')),
    );

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><DirectChoiceHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', { name: 'Direct server-1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-1' }));
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('hosted-sign-in'));
    expect(screen.getByLabelText('connection-warning')).toHaveTextContent('auth.sign-out-storage-warning');
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-2' }));
    await Promise.resolve();

    expect(connector).toHaveBeenCalledOnce();
    expect(screen.getByLabelText('runtime-state')).toHaveTextContent('hosted-sign-in');
  });

  it('keeps A mounted and fenced during B durability, then rebuilds A bindings after B rolls back', async () => {
    const servers = [hostedServer('server-1', 'Family Media'), hostedServer('server-2', 'Cinema')];
    const profile = { id: 'profile-1', name: 'Owner', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 0 };
    const durability = deferred<void>();
    const saveEntered = deferred<void>();
    const baseVault = createBrowserHostedConnectionVault(undefined);
    vi.spyOn(baseVault, 'installationId').mockResolvedValue('test-installation');
    const originalSave = baseVault.save.bind(baseVault);
    const gatedVault: HostedConnectionVault = {
      ...baseVault,
      save: async (record) => {
        if (record.serverId === 'server-2') {
          saveEntered.resolve();
          await durability.promise;
        }
        await originalSave(record);
      },
    };
    const mutationUrls: string[] = [];
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url === 'https://web.getportico.tv/api/auth/me') return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: servers, total: 2 }));
      if (url.endsWith('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [profile], revision: 1, total: 1 }));
      if (url.includes('/selection-assertions')) {
        const serverId = String(init?.body ?? '').includes('server-2') ? 'server-2' : 'server-1';
        return Promise.resolve(response({
          version: 'v1', accountId: 'user-1', accountRevision: 1, assertionId: `assertion-${serverId}`, audience: 'portico-media-server', deviceId: 'browser-device',
          expiresAt: '2026-07-14T12:05:00.000Z', installationId: 'test-installation', issuedAt: '2026-07-14T12:00:00.000Z', pinRevision: 0,
          profileId: 'profile-1', profiles: [], serverId, signature: 'signature', signatureAlgorithm: 'ed25519', signatureKeyId: 'key-1',
        }));
      }
      if (url.endsWith('/api/auth/me')) {
        const serverId = url.includes('server-2') ? 'server-2' : 'server-1';
        return Promise.resolve(response(localIdentity(serverId, serverId === 'server-1' ? 'Family Media' : 'Cinema')));
      }
      if (url.endsWith('/api/account/profile')) {
        mutationUrls.push(url);
        return Promise.resolve(response(localIdentity('server-1', 'Family Media').user));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.spyOn(ClientCore, 'connectResilientHostedServer').mockImplementation((server, options) => {
      return publishPreparedConnection(preparedConnection(server.id, server.name, options.selectionEnvelope.profileId), options);
    });

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={gatedVault}><TransactionShellHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', { name: 'Direct server-1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-1' }));
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Run scoped mutation' })).toBeEnabled());

    fireEvent.click(screen.getByRole('button', { name: 'Choose another server directly' }));
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-selection'));
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-2' }));
    await saveEntered.promise;
    fireEvent.click(screen.getByRole('button', { name: 'Run scoped mutation' }));
    await vi.waitFor(() => expect(screen.getByLabelText('mutation-result')).toHaveTextContent('AbortError'));
    expect(mutationUrls).toEqual([]);

    durability.reject(new Error('IndexedDB save failed'));
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    fireEvent.click(await screen.findByRole('button', { name: 'Run scoped mutation' }));
    await vi.waitFor(() => expect(screen.getByLabelText('mutation-result')).toHaveTextContent('success'));
    expect(mutationUrls).toEqual(['https://server-1.direct.getportico.tv/api/account/profile']);
  });

  it('fails closed across restart when another tab publishes a cleanup barrier during candidate save', async () => {
    const servers = [hostedServer('server-1', 'Family Media'), hostedServer('server-2', 'Cinema')];
    const profile = { id: 'profile-1', name: 'Owner', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 0 };
    const saveEntered = deferred<void>();
    const resumeSave = deferred<void>();
    const baseVault = createBrowserHostedConnectionVault(undefined);
    vi.spyOn(baseVault, 'installationId').mockResolvedValue('test-installation');
    const originalSave = baseVault.save.bind(baseVault);
    const gatedVault: HostedConnectionVault = {
      ...baseVault,
      save: async (record) => {
        if (record.serverId === 'server-2') {
          saveEntered.resolve();
          await resumeSave.promise;
        }
        await originalSave(record);
      },
    };
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url === 'https://web.getportico.tv/api/auth/me') return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: servers, total: 2 }));
      if (url.endsWith('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [profile], revision: 1, total: 1 }));
      if (url.includes('/selection-assertions')) {
        const serverId = String(init?.body ?? '').includes('server-2') ? 'server-2' : 'server-1';
        return Promise.resolve(response({
          version: 'v1', accountId: 'user-1', accountRevision: 1, assertionId: `assertion-${serverId}`, audience: 'portico-media-server', deviceId: 'browser-device',
          expiresAt: '2026-07-14T12:05:00.000Z', installationId: 'test-installation', issuedAt: '2026-07-14T12:00:00.000Z', pinRevision: 0,
          profileId: 'profile-1', profiles: [], serverId, signature: 'signature', signatureAlgorithm: 'ed25519', signatureKeyId: 'key-1',
        }));
      }
      if (url.endsWith('/api/auth/me')) {
        const serverId = url.includes('server-2') ? 'server-2' : 'server-1';
        return Promise.resolve(response(localIdentity(serverId, serverId === 'server-1' ? 'Family Media' : 'Cinema')));
      }
      if (url.endsWith('/api/auth/logout')) return Promise.resolve(response({ok: true}));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.spyOn(ClientCore, 'connectResilientHostedServer').mockImplementation(async (server, options) => {
      const candidate = preparedConnection(server.id, server.name, options.selectionEnvelope.profileId);
      if (server.id === 'server-1') return publishPreparedConnection(candidate, options);
      const staged = await options.stageCandidate(candidate);
      options.sessionStore.set(candidate.session);
      try {
        await options.connectionAdapter.save(candidate.record);
        await staged.publish({durability: 'durable', persistencePolicy: 'reauthorize-on-start'});
        return {...candidate, durability: 'durable', persistencePolicy: 'reauthorize-on-start'};
      } catch (reason) {
        staged.fenceRollback('fail-closed');
        options.sessionStore.clear();
        await Promise.allSettled([
          options.connectionAdapter.remove(candidate.record.accountId, candidate.record.serverId),
          staged.rollback('fail-closed'),
        ]);
        throw new ClientCore.TrustedServerCredentialPublicationError(reason, [], true);
      }
    });

    const first = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={gatedVault}><TransactionShellHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', { name: 'Direct server-1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-1' }));
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-ready'));
    fireEvent.click(screen.getByRole('button', { name: 'Choose another server directly' }));
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('server-selection'));
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-2' }));
    await saveEntered.promise;

    await act(async () => {
      await baseVault.markCleanupPending('user-1');
      resumeSave.resolve();
    });
    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('hosted-sign-in'));
    expect(screen.getByLabelText('connection-warning')).toHaveTextContent('auth.sign-out-storage-warning');
    expect(screen.queryByRole('button', { name: 'Run scoped mutation' })).not.toBeInTheDocument();
    expect(await baseVault.cleanupPendingAccounts()).toContain('user-1');
    first.unmount();

    resetSignedOutAccountQuarantineForTests();
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={gatedVault}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    expect(screen.queryByText(/Ready on/)).not.toBeInTheDocument();
  });

  it('latches typed credential cleanup uncertainty through a fresh provider restore', async () => {
    const vault = await rememberedHostedBrowser(['server-1', 'server-2']);
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Hosted Services unavailable')));
    vi.spyOn(ClientCore, 'connectTrustedServerRecord').mockRejectedValue(
      new ClientCore.CredentialCleanupUncertainError(
        'Refreshed credentials could not be persisted.',
        new Error('active credential publication failed'),
        [new Error('durable credential deletion failed')],
      ),
    );

    const first = render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><TransactionShellHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', {name: 'Direct server-1'})).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Direct server-1'}));

    await vi.waitFor(() => expect(screen.getByLabelText('runtime-state')).toHaveTextContent('hosted-sign-in'));
    expect(screen.getByLabelText('connection-warning')).toHaveTextContent('auth.sign-out-storage-warning');
    expect(await vault.cleanupPendingAccounts()).toContain('user-1');
    expect(await vault.list('user-1')).toEqual([]);
    first.unmount();

    resetSignedOutAccountQuarantineForTests();
    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByText(ClientCore.productMessage('auth.sign-out-storage-warning').body!)).toBeInTheDocument();
    expect(screen.queryByText(/Ready on/)).not.toBeInTheDocument();
  });

  it('rolls back a superseded candidate delayed at the Web Lock before publishing the newest server', async () => {
    const servers = [
      hostedServer('server-1', 'Family Media'),
      hostedServer('server-2', 'Cinema'),
      hostedServer('server-3', 'Studio'),
    ];
    const profile = { id: 'profile-1', name: 'Owner', hasPIN: false, isPrimary: true, isAccountAdmin: true, pinRevision: 0, policy: unrestrictedPolicy, sortOrder: 0 };
    const bLockEntered = deferred<void>();
    const resumeBLock = deferred<void>();
    const baseVault = createBrowserHostedConnectionVault(undefined);
    vi.spyOn(baseVault, 'installationId').mockResolvedValue('test-installation');
    const originalLocks = navigator.locks;
    let delayNextAccountLock = false;
    Object.defineProperty(navigator, 'locks', {configurable: true, value: {
      request: <T,>(name: string, options: LockOptions, callback: () => Promise<T> | T): Promise<T> => {
        if (delayNextAccountLock && name.endsWith('user-1')) {
          delayNextAccountLock = false;
          bLockEntered.resolve();
          return resumeBLock.promise.then(() => originalLocks.request(name, options, callback)) as Promise<T>;
        }
        return originalLocks.request(name, options, callback) as Promise<T>;
      },
    }});
    const fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/api/system')) return Promise.resolve(response(hostedSystem));
      if (url === 'https://web.getportico.tv/api/auth/me') return Promise.resolve(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }));
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: servers, total: 3 }));
      if (url.endsWith('/api/account/profiles')) return Promise.resolve(response({ accountId: 'user-1', profiles: [profile], revision: 1, total: 1 }));
      if (url.includes('/selection-assertions')) {
        const body = String(init?.body ?? '');
        const serverId = body.includes('server-3') ? 'server-3' : body.includes('server-2') ? 'server-2' : 'server-1';
        return Promise.resolve(response({
          version: 'v1', accountId: 'user-1', accountRevision: 1, assertionId: `assertion-${serverId}`, audience: 'portico-media-server', deviceId: 'browser-device',
          expiresAt: '2026-07-14T12:05:00.000Z', installationId: 'test-installation', issuedAt: '2026-07-14T12:00:00.000Z', pinRevision: 0,
          profileId: 'profile-1', profiles: [], serverId, signature: 'signature', signatureAlgorithm: 'ed25519', signatureKeyId: 'key-1',
        }));
      }
      if (url.endsWith('/api/auth/me')) {
        const serverId = url.includes('server-3') ? 'server-3' : url.includes('server-2') ? 'server-2' : 'server-1';
        const serverName = serverId === 'server-3' ? 'Studio' : serverId === 'server-2' ? 'Cinema' : 'Family Media';
        return Promise.resolve(response(localIdentity(serverId, serverName)));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const connector = vi.spyOn(ClientCore, 'connectResilientHostedServer').mockImplementation(async (server, options) => {
      const candidate = preparedConnection(server.id, server.name, options.selectionEnvelope.profileId);
      const previousSession = options.sessionStore.get?.();
      const staged = await options.stageCandidate(candidate);
      options.sessionStore.set(candidate.session);
      try {
        await options.connectionAdapter.save(candidate.record);
        if (server.id === 'server-2') delayNextAccountLock = true;
        await staged.publish({durability: 'durable', persistencePolicy: 'reauthorize-on-start'});
        if (options.signal?.aborted) throw new DOMException('Superseded', 'AbortError');
        return {...candidate, durability: 'durable', persistencePolicy: 'reauthorize-on-start'};
      } catch (reason) {
        staged.fenceRollback('restore-previous');
        if (previousSession) options.sessionStore.set(previousSession);
        else options.sessionStore.clear();
        await options.connectionAdapter.remove(candidate.record.accountId, candidate.record.serverId);
        await staged.rollback('restore-previous');
        throw reason;
      }
    });

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={baseVault}><TransactionShellHarness /></RuntimeProvider>);
    expect(await screen.findByRole('button', { name: 'Direct server-1' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Direct server-1' }));
    await vi.waitFor(() => expect(screen.getByLabelText('ready-server')).toHaveTextContent('Family Media'));
    fireEvent.click(screen.getByRole('button', { name: 'Choose another server directly' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Direct server-2' }));
    await bLockEntered.promise;

    fireEvent.click(screen.getByRole('button', { name: 'Direct server-3' }));
    await Promise.resolve();
    expect(connector).toHaveBeenCalledTimes(2);
    expect(screen.getByLabelText('ready-server')).toHaveTextContent('none');

    await act(async () => { resumeBLock.resolve(); });
    await vi.waitFor(() => expect(screen.getByLabelText('ready-server')).toHaveTextContent('Studio'));
    expect(connector.mock.calls.map(([server]) => server.id)).toEqual(['server-1', 'server-2', 'server-3']);
    expect(await baseVault.load('user-1', 'server-2')).toBeUndefined();
    expect(await baseVault.load('user-1', 'server-3')).toBeDefined();
  });

  it('loads every Hosted membership page before reconciling remembered servers', async () => {
    const vault = await rememberedHostedBrowser(['server-2']);
    const first = { id: 'server-1', ownerUserId: 'user-1', name: 'Family Media', serverPublicKey: 'key', serverPublicKeyFingerprint: testServerFingerprint('server-1'), assignedHostname: 'family.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' };
    const second = { id: 'server-2', ownerUserId: 'user-1', name: 'Cinema', serverPublicKey: 'key', serverPublicKeyFingerprint: testServerFingerprint('server-2'), assignedHostname: 'cinema.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'portico', certificateStatus: 'active' };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }))
      .mockResolvedValueOnce(response({ items: [first], pageInfo: { hasMore: true, nextCursor: 'page-2' } }))
      .mockResolvedValueOnce(response({ items: [second], pageInfo: { hasMore: false } }));
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: 'Choose a server' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Family Media/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Cinema/ })).toBeInTheDocument();
    expect(await vault.load('user-1', 'server-2')).toBeDefined();
    expect(String(fetchMock.mock.calls[3][0])).toContain('cursor=page-2');
  });

  it('opens a remembered server directly when Hosted Services is unavailable', async () => {
    const vault = await rememberedHostedBrowser();
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.startsWith('https://web.getportico.tv')) return Promise.reject(new TypeError('Hosted Services unavailable'));
      if (url === 'https://server-1.direct.getportico.tv/api/remote-access/health') return Promise.resolve(response({ serverId: 'server-1', serverPublicKeyFingerprint: testServerFingerprint('server-1'), remoteAccessEnabled: true }));
      if (url === 'https://server-1.direct.getportico.tv/api/system') return Promise.resolve(response(serverSystem));
      if (url === 'https://server-1.direct.getportico.tv/api/auth/me') return Promise.resolve(response({
        authenticated: true,
        setupRequired: false,
        serverFriendlyName: 'Family Media',
        accountMode: 'portico',
        authority: 'hosted',
        accountId: 'user-1',
        serverId: 'server-1',
        profileId: 'profile-1',
        authorizationRevision: 'policy-1',
        user: { id: 'profile-1', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'portico', authProvider: 'portico', hasLocalPassword: false },
      }));
      if (url === 'https://server-1.direct.getportico.tv/api/product-contract') return Promise.resolve(response(serverProductContract));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByText('Ready on Family Media')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('/api/account/servers'))).toBe(false);
    expect(await vault.load('user-1', 'server-1')).toBeDefined();
  });

  it('fails closed and tombstones a remembered server after Hosted membership removal', async () => {
    const vault = await rememberedHostedBrowser();
    let systemCalls = 0;
    const fetchMock = vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith('/api/system')) {
        systemCalls += 1;
        return systemCalls === 1 ? Promise.reject(new TypeError('Hosted Services unavailable')) : Promise.resolve(response(hostedSystem));
      }
      if (url.includes('/api/account/servers')) return Promise.resolve(response({ items: [], total: 0 }));
      if (url.startsWith('https://web.getportico.tv')) return Promise.reject(new TypeError('Hosted Services unavailable'));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    const connector = vi.spyOn(ClientCore, 'connectTrustedServerRecord').mockImplementation((record, options) => publishPreparedConnection(preparedConnection(record.serverId, record.serverName, record.profileId), options));

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><MembershipRefreshHarness /></RuntimeProvider>);

    expect(await screen.findByText('Ready on Family Media')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh memberships' }));

    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.forbidden').title })).toBeInTheDocument();
    expect(screen.queryByText('Ready on Family Media')).not.toBeInTheDocument();
    expect(await vault.load('user-1', 'server-1')).toBeUndefined();
    expect(await vault.loadRemovalTombstone('user-1', 'server-1')).toMatchObject({ accountId: 'user-1', serverId: 'server-1' });
    expect(connector).toHaveBeenCalledOnce();
  });

  it('offers all remembered servers without exposing direct route hostnames when Hosted Services is unavailable', async () => {
    const vault = await rememberedHostedBrowser(['server-1', 'server-2']);
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Hosted Services unavailable')));

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: 'Choose a server' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Family Media/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Cinema/ })).toBeInTheDocument();
    expect(screen.queryByText(/direct\.getportico\.tv/)).not.toBeInTheDocument();
  });

  it('keeps a remembered server credential after transient Hosted and server outages', async () => {
    const vault = await rememberedHostedBrowser();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Network unavailable')));

    render(<RuntimeProvider config={hostedConfig} hostedConnectionVault={vault}><RuntimeHarness /></RuntimeProvider>);

    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.server-unavailable').title })).toBeInTheDocument();
    expect(await vault.load('user-1', 'server-1')).toBeDefined();
  });

  it('does not auto-connect a single Hosted membership that only allows local credentials', async () => {
    const servers = [
      { id: 'server-1', ownerUserId: 'user-1', name: 'LAN only', serverPublicKey: 'key', serverPublicKeyFingerprint: 'fingerprint', assignedHostname: 'lan-only.example.direct', remoteAccessEnabled: true, preferredAuthMode: 'local', certificateStatus: 'active' },
    ];
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(hostedSystem))
      .mockResolvedValueOnce(response({ authenticated: true, user: { id: 'user-1', email: 'owner@example.test', displayName: 'Owner' } }))
      .mockResolvedValueOnce(response({ items: servers, total: 1 }));
    vi.stubGlobal('fetch', fetchMock);
    render(<RuntimeProvider config={hostedConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: 'Choose a server' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /LAN only/ })).toBeDisabled();
    expect(screen.getByText('This Server sign-in only')).toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([input]) => String(input).startsWith(hostedConfig.hostedApiBaseUrl))).toHaveLength(3);
  });

  it('renders configuration recovery instead of throwing for a malformed Hosted URL', async () => {
    render(<RuntimeProvider environment={{ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'hosted', VITE_PORTICO_HOSTED_API_URL: 'not a url' }}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.request-failed').title })).toBeInTheDocument();
  });

  it('renders configuration recovery when a production boot requests fixture data', async () => {
    render(<RuntimeProvider environment={{ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'fixtures' }}><RuntimeHarness /></RuntimeProvider>);
    const copy = ClientCore.productMessage('problem.request-failed');
    expect(await screen.findByRole('heading', { name: copy.title })).toBeInTheDocument();
    expect(screen.getByText(copy.body!)).toBeInTheDocument();
  });

  it('requires an explicit development loader before fixture mode can start', async () => {
    render(<RuntimeProvider config={fixtureConfig}><RuntimeHarness /></RuntimeProvider>);
    expect(await screen.findByRole('heading', { name: ClientCore.productMessage('problem.request-failed').title })).toBeInTheDocument();
  });
});
