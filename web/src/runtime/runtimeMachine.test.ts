import { ApiError, createMemorySessionStore, LocalNetworkRouteUnavailableError, NearbyRouteAvailableError } from '@porticomediaserver/client-core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { browserSafeLocalCandidates, browserSafeProbeFetch, extractHostedBootstrapIntent, verifiedLocalLoginRedirect } from './RuntimeProvider';
import {
  classifyRuntimeFailure,
  ProfileSelectionRequiredError,
  initialRuntimeState,
  mergeRuntimeEnvironment,
  resolveRuntimeConfig,
  runtimeReducer,
  type HostedServerSummary,
} from './runtimeMachine';

const server: HostedServerSummary = {
  id: 'server-1',
  name: 'Family Media',
  assignedHostname: 'family.example.direct',
  remoteAccessEnabled: true,
  preferredAuthMode: 'portico',
};

afterEach(() => vi.restoreAllMocks());

describe('Portico web runtime state machine', () => {
  it('models bundled bootstrap without inferring authority from the hostname', () => {
    const checking = runtimeReducer(initialRuntimeState(1), { type: 'CHECK_LOCAL', serverName: 'This Server' });
    expect(checking).toMatchObject({ id: 'checking-local-server', serverName: 'This Server' });
    const ready = runtimeReducer(checking, { type: 'READY', mode: 'bundled', serverName: 'This Server' });
    expect(ready).toMatchObject({ id: 'server-ready', mode: 'bundled', serverName: 'This Server' });
  });

  it('models hosted sign-in, membership selection, route discovery, and direct readiness', () => {
    const account = runtimeReducer(initialRuntimeState(), { type: 'CHECK_HOSTED_SESSION' });
    expect(account.id).toBe('hosted-account-session');
    const signIn = runtimeReducer(account, { type: 'HOSTED_SIGN_IN_REQUIRED' });
    expect(signIn).toMatchObject({ id: 'hosted-sign-in', recoveryActions: ['sign-in'] });
    const memberships = runtimeReducer(signIn, { type: 'LOAD_MEMBERSHIPS' });
    const selection = runtimeReducer(memberships, { type: 'MEMBERSHIPS_READY', servers: [server, { ...server, id: 'server-2', name: 'Cinema' }] });
    expect(selection.id).toBe('server-selection');
    const discovery = runtimeReducer(selection, { type: 'SELECT_SERVER', server, servers: [server] });
    expect(discovery).toMatchObject({ id: 'route-discovery', selectedServer: server, recoveryActions: ['retry', 'reselect-server'] });
    const ready = runtimeReducer(discovery, { type: 'READY', mode: 'hosted', serverName: server.name });
    expect(ready).toMatchObject({ id: 'server-ready', mode: 'hosted' });
    expect(JSON.stringify(ready)).not.toContain('relay');
  });

  it('keeps no-membership and permission-loss recovery explicit', () => {
    const empty = runtimeReducer(initialRuntimeState(), { type: 'MEMBERSHIPS_READY', servers: [] });
    expect(empty).toMatchObject({ id: 'no-memberships', recoveryActions: ['refresh-memberships', 'sign-out'] });
    const removed = runtimeReducer(empty, { type: 'FAILURE', classification: 'permission-removed', serverName: server.name, servers: [server], hosted: true });
    expect(removed).toMatchObject({ id: 'runtime-recovery', classification: 'permission-removed' });
    expect(removed.recoveryActions).toEqual(expect.arrayContaining(['reselect-server', 'refresh-memberships', 'sign-out']));
  });

  it('publishes only canonical Product Language IDs for recovery state', () => {
    const recovery = runtimeReducer(initialRuntimeState(), {
      type: 'FAILURE',
      classification: 'server-offline',
      serverName: server.name,
      hosted: true,
    });

    expect(recovery).toMatchObject({
      id: 'runtime-recovery',
      messageId: 'problem.server-unavailable',
    });
    expect(recovery).not.toHaveProperty('message');
  });

  it('keeps transient Hosted account checks retryable without pretending the account expired', () => {
    const recovery = runtimeReducer(initialRuntimeState(), {
      type: 'FAILURE',
      classification: 'hosted-session',
      hosted: true,
    });
    expect(recovery).toMatchObject({
      id: 'runtime-recovery',
      messageId: 'problem.cloud-unavailable',
    });
    expect(recovery.recoveryActions).toContain('retry');
    expect(recovery.recoveryActions).not.toContain('sign-in');
  });

  it('carries automatic Hosted availability timing only when the read caller explicitly opts in', () => {
    const automatic = runtimeReducer(initialRuntimeState(), {
      type: 'FAILURE',
      classification: 'membership',
      hosted: true,
      automaticAvailabilityRetry: true,
      availabilityRetryAfterMs: 9_000,
    });
    expect(automatic).toMatchObject({
      id: 'runtime-recovery',
      classification: 'membership',
      automaticAvailabilityRetry: true,
      availabilityRetryAfterMs: 9_000,
    });

    const security = runtimeReducer(initialRuntimeState(), {
      type: 'FAILURE',
      classification: 'route-security',
      hosted: true,
    });
    expect(security).not.toHaveProperty('automaticAvailabilityRetry', true);
  });

  it('offers nearby routing only as an explicit recovery action', () => {
    const recovery = runtimeReducer(initialRuntimeState(), {
      type: 'FAILURE',
      classification: 'no-route',
      messageId: 'connection.nearby-available',
      selectedServer: server,
      servers: [server],
      hosted: true,
      nearbyAvailable: true,
    });
    expect(recovery).toMatchObject({
      id: 'runtime-recovery',
      messageId: 'connection.nearby-available',
      selectedServer: server,
    });
    expect(recovery.recoveryActions).toEqual(expect.arrayContaining(['try-nearby', 'retry', 'reselect-server', 'sign-out']));
    expect(recovery.recoveryActions).not.toContain('sign-in');
  });

  it('classifies expired sessions, offline servers, route security, and unreachable routes', () => {
    expect(classifyRuntimeFailure(new ApiError(401, 'session_expired', 'expired'))).toBe('session-expired');
    expect(classifyRuntimeFailure(new ApiError(401, 'unauthorized', 'invalid credentials'), 'hosted-session')).toBe('hosted-session');
    expect(classifyRuntimeFailure(new Error('A profile access token could not be verified.'), 'profile-directory')).toBe('profile-directory');
    expect(classifyRuntimeFailure(new ApiError(403, 'server_not_found', 'removed'))).toBe('permission-removed');
    expect(classifyRuntimeFailure(new ApiError(403, 'membership_inactive', 'removed'))).toBe('permission-removed');
    expect(classifyRuntimeFailure(new ApiError(403, 'request_failed', 'forbidden'))).toBe('unknown');
    expect(classifyRuntimeFailure(new TypeError('Failed to fetch'))).toBe('server-offline');
    expect(classifyRuntimeFailure(new TypeError('Failed to fetch'), 'membership')).toBe('membership');
    expect(classifyRuntimeFailure(new ApiError(0, 'request_failed', 'offline'), 'hosted-session')).toBe('hosted-session');
    expect(classifyRuntimeFailure(new Error('The selected route returned the wrong server identity.'))).toBe('route-security');
    expect(classifyRuntimeFailure(new Error('Unable to verify a route for this server.'))).toBe('no-route');
    expect(classifyRuntimeFailure(new Error('Connection was interrupted.'))).toBe('route-interrupted');
    expect(classifyRuntimeFailure(new NearbyRouteAvailableError())).toBe('no-route');
    expect(classifyRuntimeFailure(new LocalNetworkRouteUnavailableError())).toBe('no-route');
    expect(classifyRuntimeFailure(new ProfileSelectionRequiredError('server-one', 2, false))).toBe('profile-selection-required');
  });
});

describe('runtime configuration and credential boundaries', () => {
  it('rejects fixture mode in production and insecure Hosted origins', () => {
    expect(() => resolveRuntimeConfig({ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'fixtures' })).toThrow('disabled in production');
    expect(() => resolveRuntimeConfig({ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'local' })).toThrow('unsupported runtime mode');
    expect(() => resolveRuntimeConfig({ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'hosted', VITE_PORTICO_HOSTED_API_URL: 'http://hosted.example' })).toThrow('HTTPS');
    expect(resolveRuntimeConfig({ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'bundled' }).mode).toBe('bundled');
    expect(resolveRuntimeConfig({ DEV: false, VITE_PORTICO_RUNTIME_MODE: 'hosted' }).hostedApiBaseUrl).toBe('https://web.getportico.tv');
  });

  it('allows deliberate loopback Hosted development without weakening production', () => {
    const config = resolveRuntimeConfig({ DEV: true, VITE_PORTICO_RUNTIME_MODE: 'hosted', VITE_PORTICO_HOSTED_API_URL: 'http://127.0.0.1:9091' });
    expect(config).toMatchObject({ mode: 'hosted', hostedApiBaseUrl: 'http://127.0.0.1:9091' });
  });

  it('lets a same-origin runtime document override build-time authority', () => {
    const environment = mergeRuntimeEnvironment(
      { DEV: false, VITE_PORTICO_RUNTIME_MODE: 'hosted', VITE_PORTICO_HOSTED_API_URL: 'https://staging.getportico.tv' },
	  { mode: 'bundled', hostedApiBaseUrl: 'https://api.getportico.tv', routeProbeTimeoutMs: 2400, buildId: 'sha256-1234567890abcdef12345678' },
    );
    expect(resolveRuntimeConfig(environment)).toEqual({
      mode: 'bundled',
      hostedApiBaseUrl: 'https://api.getportico.tv',
      routeProbeTimeoutMs: 2400,
	  buildId: 'sha256-1234567890abcdef12345678',
    });
  });

  it('keeps server credentials in memory and out of browser storage and state', () => {
    const storageWrite = vi.spyOn(Storage.prototype, 'setItem');
    const sessionStore = createMemorySessionStore();
    sessionStore.set?.({ apiBaseUrl: 'https://family.example.direct', bootstrapAccessToken: 'secret-access', refreshToken: 'secret-refresh' });
    expect(sessionStore.get()?.bootstrapAccessToken).toBe('secret-access');
    expect(storageWrite).not.toHaveBeenCalled();
    const ready = runtimeReducer(initialRuntimeState(), { type: 'READY', mode: 'hosted', serverName: server.name });
    expect(JSON.stringify(ready)).not.toMatch(/secret-access|secret-refresh|family\.example\.direct/);
  });

  it('refuses mixed-content route probes before network I/O', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    await expect(browserSafeProbeFetch('http://192.168.1.4:32500/api/remote-access/health')).rejects.toThrow('secure HTTPS');
    expect(fetchMock).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('derives LAN candidates only from signed-document HTTPS routes', () => {
    const candidates = browserSafeLocalCandidates({ id: 'server-1' } as never, {
      routes: [
        { type: 'lan', url: 'http://192.168.1.4:32500', quality: 'reported' },
        { type: 'lan', url: 'https://192.168.1.4:32500', quality: 'reported' },
        { type: 'lan', url: 'https://[fd00::42]:32500', quality: 'reported' },
        { type: 'lan_ip_encoded', url: 'https://192-168-1-4.direct.example', quality: 'reported' },
        { type: 'public_direct', url: 'https://family.example.direct', quality: 'reachable' },
        { type: 'lan_discovered', url: 'https://stale.direct.example', quality: 'tls_failed' },
        { type: 'lan_ip_encoded', url: 'https://old-network.direct.example', quality: 'stale' },
      ],
    } as never);
    expect(candidates).toEqual([{ type: 'lan_ip_encoded', url: 'https://192-168-1-4.direct.example', quality: 'reported' }]);
  });

  it('consumes sensitive Hosted deep-link values into memory and returns a clean URL', () => {
    const reset = extractHostedBootstrapIntent('https://web.getportico.tv/account/password-reset/reset-secret?source=email');
    expect(reset.intent.resetToken).toBe('reset-secret');
    expect(reset.safeUrl).toBe('/?source=email');
    expect(reset.safeUrl).not.toContain('reset-secret');

    const invite = extractHostedBootstrapIntent('https://web.getportico.tv/invites/invite-secret');
    expect(invite.intent.inviteId).toBe('invite-secret');
    expect(invite.safeUrl).toBe('/');

    const device = extractHostedBootstrapIntent('https://web.getportico.tv/device#code=abcd-efgh');
    expect(device.intent).toMatchObject({ deviceAuthorizationRequested: true, deviceAuthorizationCode: 'ABCD-EFGH' });
    expect(device.safeUrl).toBe('/device');
    expect(device.safeUrl).not.toContain('ABCD-EFGH');

    const genericDevice = extractHostedBootstrapIntent('https://web.getportico.tv/authorize-device#code=JKMN-PQRS&provider=apple&nativeReturn=1');
    expect(genericDevice.intent).toMatchObject({ genericDeviceAuthorizationRequested: true, genericDeviceAuthorizationCode: 'JKMN-PQRS', genericDeviceAuthorizationProvider: 'apple', genericDeviceAuthorizationNativeReturn: true });
    expect(genericDevice.safeUrl).toBe('/authorize-device');
  });

  it('captures and scrubs a local-server Portico Account handoff', () => {
    const callbackUrl = 'http://localhost:32500/api/auth/portico/callback';
    const localOrigin = 'http://localhost:32500';
    const handoff = extractHostedBootstrapIntent(`https://web.getportico.tv/#/local-login?serverId=srv_family&serverName=Family%20Media&callbackUrl=${encodeURIComponent(callbackUrl)}&localOrigin=${encodeURIComponent(localOrigin)}&state=state-token-with-enough-entropy&serverPublicKeyFingerprint=sha256%3Afingerprint`);
    expect(handoff.intent.localLogin).toEqual({
      serverId: 'srv_family',
      serverName: 'Family Media',
      callbackUrl,
      localOrigin,
      state: 'state-token-with-enough-entropy',
      serverPublicKeyFingerprint: 'sha256:fingerprint',
    });
    expect(handoff.safeUrl).toBe('/');
    expect(handoff.safeUrl).not.toContain('state-token');
  });

  it('accepts only a state-bound callback to the requesting local server', () => {
    const intent = {
      serverId: 'srv_family',
      serverName: 'Family Media',
      callbackUrl: 'http://localhost:32500/api/auth/portico/callback',
      localOrigin: 'http://localhost:32500',
      state: 'state-token-with-enough-entropy',
      serverPublicKeyFingerprint: 'sha256:fingerprint',
    };
    expect(verifiedLocalLoginRedirect(intent, 'http://localhost:32500/api/auth/portico/callback?code=one-time-code&state=state-token-with-enough-entropy')).toContain('code=one-time-code');
    expect(() => verifiedLocalLoginRedirect(intent, 'https://evil.example/api/auth/portico/callback?code=one-time-code&state=state-token-with-enough-entropy')).toThrow('unexpected server address');
    expect(() => verifiedLocalLoginRedirect(intent, 'http://localhost:32500/api/auth/portico/callback?code=one-time-code&state=wrong-state')).toThrow('could not be verified');
  });

  it.each([
    'http://127.0.0.1:32500',
    'http://10.0.0.20:32500',
    'http://172.20.0.20:32500',
    'http://192.168.2.20:32500',
  ])('preserves a state-bound local callback on %s', (localOrigin) => {
    const callbackUrl = `${localOrigin}/api/auth/portico/callback`;
    const intent = {
      serverId: 'srv_family',
      serverName: 'Family Media',
      callbackUrl,
      localOrigin,
      state: 'state-token-with-enough-entropy',
      serverPublicKeyFingerprint: 'sha256:fingerprint',
    };
    expect(verifiedLocalLoginRedirect(intent, `${callbackUrl}?code=one-time-code&state=${intent.state}`)).toBe(
      `${callbackUrl}?code=one-time-code&state=${intent.state}`,
    );
  });

  it.each([
    'http://8.8.8.8:32500',
    'http://192.168.2.20.evil.example:32500',
    'http://172.32.0.20:32500',
  ])('rejects an insecure callback disguised as a local route on %s', (localOrigin) => {
    const callbackUrl = `${localOrigin}/api/auth/portico/callback`;
    const intent = {
      serverId: 'srv_family',
      serverName: 'Family Media',
      callbackUrl,
      localOrigin,
      state: 'state-token-with-enough-entropy',
      serverPublicKeyFingerprint: 'sha256:fingerprint',
    };
    expect(() => verifiedLocalLoginRedirect(intent, `${callbackUrl}?code=one-time-code&state=${intent.state}`)).toThrow(
      'not secure',
    );
  });
});
