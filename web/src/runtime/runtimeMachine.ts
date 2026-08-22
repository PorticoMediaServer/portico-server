import { ApiError, LocalNetworkRouteUnavailableError, NearbyRouteAvailableError, type HostedAccountProfile, type ProductMessageId } from '@porticomediaserver/client-core';

export type RuntimeMode = 'bundled' | 'hosted' | 'fixtures';

export type RuntimeConfig = {
  mode: RuntimeMode;
  hostedApiBaseUrl: string;
  routeProbeTimeoutMs: number;
	buildId: string;
};

export type RuntimeBootstrapConfig = {
  mode?: RuntimeMode;
  hostedApiBaseUrl?: string;
  routeProbeTimeoutMs?: number;
	buildId?: string;
};

export type HostedServerSummary = {
  id: string;
  name: string;
  assignedHostname: string;
  remoteAccessEnabled: boolean;
  preferredAuthMode: string;
  lastHeartbeatAt?: string;
};

export type RuntimeFailureClassification =
  | 'configuration'
  | 'hosted-session'
  | 'membership'
  | 'profile-directory'
  | 'profile-selection-required'
  | 'no-route'
  | 'route-security'
  | 'session-expired'
  | 'permission-removed'
  | 'server-starting'
  | 'server-offline'
  | 'route-interrupted'
  | 'unknown';

export function runtimeFailureMessageId(classification: RuntimeFailureClassification): ProductMessageId {
  switch (classification) {
    case 'hosted-session':
      return 'problem.cloud-unavailable';
    case 'session-expired':
      return 'auth.session-expired';
    case 'membership':
      return 'problem.cloud-unavailable';
    case 'profile-directory':
      return 'problem.profile-request-failed';
    case 'profile-selection-required':
      return 'auth.profile-selection-required';
    case 'permission-removed':
      return 'problem.forbidden';
    case 'no-route':
    case 'server-starting':
    case 'server-offline':
      return 'problem.server-unavailable';
    case 'route-security':
    case 'route-interrupted':
      return 'problem.connection-failed';
    case 'configuration':
    case 'unknown':
      return 'problem.request-failed';
  }
}

export type RuntimeRecoveryAction = 'retry' | 'try-nearby' | 'reselect-server' | 'sign-in' | 'refresh-memberships' | 'continue-account' | 'sign-out';

export class ProfileSelectionRequiredError extends Error {
  readonly serverId: string;
  readonly profileCount: number;
  readonly pinRequired: boolean;

  constructor(serverId: string, profileCount: number, pinRequired: boolean) {
    super(pinRequired
      ? 'Choose a viewing profile and enter its PIN before opening this server.'
      : 'Choose a viewing profile before opening this server.');
    this.name = 'ProfileSelectionRequiredError';
    this.serverId = serverId;
    this.profileCount = profileCount;
    this.pinRequired = pinRequired;
  }
}

type RuntimeStateBase = {
  startedAt: number;
  retryable: boolean;
  failure?: RuntimeFailureClassification;
  recoveryActions: RuntimeRecoveryAction[];
};

export type RuntimeState =
  | (RuntimeStateBase & { id: 'runtime-config' })
  | (RuntimeStateBase & { id: 'checking-local-server'; serverName: string })
  | (RuntimeStateBase & { id: 'hosted-account-session' })
  | (RuntimeStateBase & { id: 'hosted-sign-in'; messageId?: ProductMessageId })
  | (RuntimeStateBase & { id: 'device-authorization'; mode: 'tv' | 'generic'; initialCode?: string; nativeReturn?: boolean; servers: HostedServerSummary[] })
  | (RuntimeStateBase & { id: 'server-memberships' })
  | (RuntimeStateBase & { id: 'no-memberships' })
  | (RuntimeStateBase & { id: 'server-selection'; servers: HostedServerSummary[] })
  | (RuntimeStateBase & { id: 'profile-selection'; servers: HostedServerSummary[]; selectedServer: HostedServerSummary; profiles: HostedAccountProfile[]; messageId?: ProductMessageId })
  | (RuntimeStateBase & { id: 'route-discovery'; servers: HostedServerSummary[]; selectedServer: HostedServerSummary })
  | (RuntimeStateBase & { id: 'runtime-recovery'; classification: RuntimeFailureClassification; messageId: ProductMessageId; serverName?: string; selectedServer?: HostedServerSummary; servers?: HostedServerSummary[]; automaticAvailabilityRetry?: boolean; availabilityRetryAfterMs?: number; availabilityRetryAt?: string })
  | (RuntimeStateBase & { id: 'server-ready'; mode: RuntimeMode; serverName: string });

export type RuntimeEvent =
  | { type: 'CHECK_LOCAL'; serverName?: string }
  | { type: 'CHECK_HOSTED_SESSION' }
  | { type: 'HOSTED_SIGN_IN_REQUIRED'; messageId?: ProductMessageId }
  | { type: 'DEVICE_AUTHORIZATION'; mode: 'tv' | 'generic'; initialCode?: string; nativeReturn?: boolean; servers: HostedServerSummary[] }
  | { type: 'LOAD_MEMBERSHIPS' }
  | { type: 'MEMBERSHIPS_READY'; servers: HostedServerSummary[] }
  | { type: 'SELECT_SERVER'; server: HostedServerSummary; servers: HostedServerSummary[] }
  | { type: 'PROFILE_SELECTION_REQUIRED'; server: HostedServerSummary; servers: HostedServerSummary[]; profiles: HostedAccountProfile[]; messageId?: ProductMessageId }
  | { type: 'READY'; mode: RuntimeMode; serverName: string }
  | { type: 'FAILURE'; classification: RuntimeFailureClassification; messageId?: ProductMessageId; serverName?: string; selectedServer?: HostedServerSummary; servers?: HostedServerSummary[]; hosted?: boolean; continueAccount?: boolean; nearbyAvailable?: boolean; automaticAvailabilityRetry?: boolean; availabilityRetryAfterMs?: number; availabilityRetryAt?: string }
  | { type: 'RESTART' };

function state(id: RuntimeState['id'], overrides: Record<string, unknown> = {}): RuntimeState {
  return { id, startedAt: Date.now(), retryable: false, recoveryActions: [], ...overrides } as RuntimeState;
}

export function initialRuntimeState(now = Date.now()): RuntimeState {
  return { id: 'runtime-config', startedAt: now, retryable: false, recoveryActions: [] };
}

export function runtimeReducer(_current: RuntimeState, event: RuntimeEvent): RuntimeState {
  switch (event.type) {
    case 'CHECK_LOCAL':
      return state('checking-local-server', { serverName: event.serverName ?? 'this server' });
    case 'CHECK_HOSTED_SESSION':
      return state('hosted-account-session');
    case 'HOSTED_SIGN_IN_REQUIRED':
      return state('hosted-sign-in', { messageId: event.messageId, recoveryActions: ['sign-in'] });
    case 'DEVICE_AUTHORIZATION':
      return state('device-authorization', { mode: event.mode, initialCode: event.initialCode, nativeReturn: event.nativeReturn, servers: event.servers });
    case 'LOAD_MEMBERSHIPS':
      return state('server-memberships', { retryable: true, recoveryActions: ['retry', 'sign-out'] });
    case 'MEMBERSHIPS_READY':
      if (event.servers.length === 0) return state('no-memberships', { retryable: true, recoveryActions: ['refresh-memberships', 'sign-out'] });
      return state('server-selection', { servers: event.servers, recoveryActions: ['sign-out'] });
    case 'SELECT_SERVER':
      return state('route-discovery', { selectedServer: event.server, servers: event.servers, retryable: true, recoveryActions: ['retry', 'reselect-server'] });
    case 'PROFILE_SELECTION_REQUIRED':
      return state('profile-selection', { selectedServer: event.server, servers: event.servers, profiles: event.profiles, messageId: event.messageId, recoveryActions: ['reselect-server', 'sign-out'] });
    case 'READY':
      return state('server-ready', { mode: event.mode, serverName: event.serverName });
    case 'FAILURE': {
      const recoveryActions: RuntimeRecoveryAction[] = [];
      if (!['configuration', 'permission-removed'].includes(event.classification)) recoveryActions.push('retry');
      if (event.nearbyAvailable) recoveryActions.push('try-nearby');
      if (event.servers?.length) recoveryActions.push('reselect-server');
      if (event.classification === 'session-expired') recoveryActions.push('sign-in');
      if (event.classification === 'permission-removed' || event.classification === 'membership') recoveryActions.push('refresh-memberships');
      if (event.continueAccount) recoveryActions.push('continue-account');
      if (event.hosted && !recoveryActions.includes('sign-out')) recoveryActions.push('sign-out');
      return state('runtime-recovery', {
        classification: event.classification,
        failure: event.classification,
        retryable: recoveryActions.includes('retry'),
        recoveryActions,
        messageId: event.messageId ?? runtimeFailureMessageId(event.classification),
        serverName: event.serverName,
        selectedServer: event.selectedServer,
        servers: event.servers,
        automaticAvailabilityRetry: event.automaticAvailabilityRetry,
        availabilityRetryAfterMs: event.availabilityRetryAfterMs,
        availabilityRetryAt: event.availabilityRetryAt,
      });
    }
    case 'RESTART':
      return initialRuntimeState();
  }
}

export function classifyRuntimeFailure(error: unknown, fallback: RuntimeFailureClassification = 'unknown'): RuntimeFailureClassification {
  if (error instanceof ProfileSelectionRequiredError) return 'profile-selection-required';
  // These typed outcomes are route-discovery states. In particular, a browser
  // local-network denial must not be mistaken for the server or internet being
  // offline and must not encourage infrastructure or port-forwarding changes.
  if (error instanceof NearbyRouteAvailableError || error instanceof LocalNetworkRouteUnavailableError) return 'no-route';
  if (error instanceof ApiError) {
    if (error.status === 401 && ['session_expired', 'server_session_revoked'].includes(error.code)) return 'session-expired';
    if (error.status === 401) return fallback;
    if (['membership_inactive', 'server_not_found', 'server_session_revoked', 'account_disabled', 'device_not_allowed'].includes(error.code)) return 'permission-removed';
    if (error.code === 'remote_access_disabled') return 'no-route';
    if (error.status === 0 || error.status >= 500) return hostedAccountFailureFallback(fallback) ? fallback : 'server-offline';
  }
  let message = '';
  if (error instanceof Error) message = error.message.toLocaleLowerCase();
  else message = String(error).toLocaleLowerCase();
  if (message.includes('wrong server identity') || message.includes('fingerprint') || message.includes('tls') || message.includes('certificate')) return 'route-security';
  // Unstructured transport text is not authority to expire a Portico Account.
  // Credential refresh/revocation paths must provide a stable code above.
  if (message.includes('starting')) return 'server-starting';
  if (message.includes('no route') || message.includes('unable to verify a route') || message.includes('could not establish a secure connection')) return 'no-route';
  if (message.includes('interrupted') || message.includes('abort')) return 'route-interrupted';
  if (error instanceof TypeError || message.includes('offline') || message.includes('failed to fetch') || message.includes('network')) {
    return hostedAccountFailureFallback(fallback) ? fallback : 'server-offline';
  }
  return fallback;
}

function hostedAccountFailureFallback(fallback: RuntimeFailureClassification): boolean {
  return fallback === 'hosted-session' || fallback === 'membership' || fallback === 'profile-directory';
}

export function mergeRuntimeEnvironment(
  environment: Record<string, string | boolean | undefined>,
  bootstrap?: RuntimeBootstrapConfig,
): Record<string, string | boolean | undefined> {
  if (!bootstrap) return environment;
  return {
    ...environment,
    VITE_PORTICO_RUNTIME_MODE: bootstrap.mode ?? environment.VITE_PORTICO_RUNTIME_MODE,
    VITE_PORTICO_HOSTED_API_URL: bootstrap.hostedApiBaseUrl ?? environment.VITE_PORTICO_HOSTED_API_URL,
    VITE_PORTICO_ROUTE_PROBE_TIMEOUT_MS: bootstrap.routeProbeTimeoutMs == null
      ? environment.VITE_PORTICO_ROUTE_PROBE_TIMEOUT_MS
      : String(bootstrap.routeProbeTimeoutMs),
	VITE_PORTICO_BUILD_ID: bootstrap.buildId ?? environment.VITE_PORTICO_BUILD_ID,
  };
}

export function resolveRuntimeConfig(environment: Record<string, string | boolean | undefined>): RuntimeConfig {
  const development = environment.DEV === true || environment.DEV === 'true';
  const configuredMode = String(environment.VITE_PORTICO_RUNTIME_MODE || '').trim();
  if (configuredMode && !['bundled', 'hosted', 'fixtures'].includes(configuredMode)) throw new Error('Portico has an unsupported runtime mode.');
  const mode: RuntimeMode = configuredMode as RuntimeMode || (development ? 'fixtures' : 'bundled');
  if (!development && mode === 'fixtures') throw new Error('Fixture data is disabled in production builds.');

  const hostedApiBaseUrl = String(environment.VITE_PORTICO_HOSTED_API_URL || 'https://api.getportico.tv').replace(/\/+$/, '');
  const hostedUrl = new URL(hostedApiBaseUrl);
  const developmentLoopback = development && hostedUrl.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(hostedUrl.hostname);
  if (mode === 'hosted' && hostedUrl.protocol !== 'https:' && !developmentLoopback) throw new Error('Hosted mode requires an HTTPS Hosted Services origin.');

  const configuredTimeout = Number(environment.VITE_PORTICO_ROUTE_PROBE_TIMEOUT_MS || 3500);
  return {
    mode,
    hostedApiBaseUrl,
    routeProbeTimeoutMs: Number.isFinite(configuredTimeout) ? Math.max(500, Math.min(10_000, configuredTimeout)) : 3500,
	buildId: String(environment.VITE_PORTICO_BUILD_ID || 'development'),
  };
}
