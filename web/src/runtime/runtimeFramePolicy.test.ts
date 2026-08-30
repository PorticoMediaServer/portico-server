import { describe, expect, it } from 'vitest';
import type { RuntimeState } from './runtimeMachine';
import { runtimeUsesProductFrame } from './runtimeFramePolicy';

function state(value: Partial<RuntimeState> & Pick<RuntimeState, 'id'>): RuntimeState {
  return {
    startedAt: 0,
    retryable: false,
    recoveryActions: [],
    ...value,
  } as RuntimeState;
}

const server = { id: 'server-1', name: 'Home Server', assignedHostname: 'home.test', remoteAccessEnabled: true, preferredAuthMode: 'hosted' };

describe('runtimeUsesProductFrame', () => {
  it.each([
    state({ id: 'checking-local-server', serverName: 'Home Server' }),
    state({ id: 'hosted-account-session' }),
    state({ id: 'server-memberships' }),
    state({ id: 'no-memberships' }),
    state({ id: 'route-discovery', servers: [], selectedServer: server }),
    state({ id: 'runtime-recovery', classification: 'server-offline', messageId: 'problem.server-unavailable', selectedServer: server }),
  ])('keeps ordinary connection state $id inside the product frame', (runtimeState) => {
    expect(runtimeUsesProductFrame(runtimeState)).toBe(true);
  });

  it.each([
    state({ id: 'hosted-sign-in' }),
    state({ id: 'profile-selection', servers: [], selectedServer: server, profiles: [] }),
    state({ id: 'runtime-recovery', classification: 'session-expired', messageId: 'auth.session-expired' }),
    state({ id: 'runtime-recovery', classification: 'route-security', messageId: 'problem.connection-failed' }),
    state({ id: 'runtime-recovery', classification: 'server-offline', messageId: 'problem.server-unavailable' }),
  ])('keeps identity or security state $id outside the cosmetic product frame', (runtimeState) => {
    expect(runtimeUsesProductFrame(runtimeState)).toBe(false);
  });
});
