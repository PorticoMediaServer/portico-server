import { describe, expect, it } from 'vitest';
import type { HostedServerSummary } from './runtimeMachine';
import { postAuthenticationReturnURL, relativeHeartbeat } from './RuntimeSurface';

function server(lastHeartbeatAt?: string): HostedServerSummary {
  return {
    id: 'srv_test', name: 'Test server', assignedHostname: 'test.direct.getportico.tv',
    remoteAccessEnabled: true, preferredAuthMode: 'portico',
    ...(lastHeartbeatAt ? { lastHeartbeatAt } : {}),
  };
}

describe('Hosted server activity copy', () => {
  it('does not turn missing or year-one heartbeats into enormous elapsed dates', () => {
    expect(relativeHeartbeat(server())).toBe('Never online');
    expect(relativeHeartbeat(server('0001-01-01T00:00:00Z'))).toBe('Never online');
    expect(relativeHeartbeat(server('not-a-date'))).toBe('Never online');
  });

  it('does not report a materially future heartbeat as online', () => {
    expect(relativeHeartbeat(server(new Date(Date.now() + 60 * 60_000).toISOString()))).toBe('Status unavailable');
  });
});

describe('post-authentication route restoration', () => {
  it('falls back to Home for viewer-scoped object routes', () => {
    expect(postAuthenticationReturnURL({
      origin: 'https://web.getportico.tv',
      pathname: '/media/stale-media-id',
      search: '?season=stale-season',
    }).toString()).toBe('https://web.getportico.tv/');
  });

  it('preserves context-independent destinations', () => {
    expect(postAuthenticationReturnURL({
      origin: 'https://web.getportico.tv',
      pathname: '/search',
      search: '?q=Arrival',
    }).toString()).toBe('https://web.getportico.tv/search?q=Arrival');
  });
});
