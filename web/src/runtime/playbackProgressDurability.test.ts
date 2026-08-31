import type { DurablePlaybackProgressRecord } from '@porticomediaserver/client-core';
import { describe, expect, it } from 'vitest';
import { createBrowserPlaybackProgressDurability } from './playbackProgressDurability';

type ScopedTerminalRecord = Record<string, unknown> & { key: string };

const terminalRecord = (
  key: string,
  scope: readonly [string, string, string, string, string],
  sessionId: string,
  updatedAt: string,
): ScopedTerminalRecord => ({
  version: 'v2',
  kind: 'terminal',
  key,
  scope,
  sessionId,
  operation: 'stop',
  request: {
    requestId: `request-${sessionId}`,
    terminal: {
      disposition: 'completed',
      generation: 1,
      eventSequence: 9,
      recordedAt: '2020-01-01T00:00:00.000Z',
      positionSeconds: 60,
      durationSeconds: 60,
    },
  },
  updatedAt,
});

describe('browser playback progress durability', () => {
  it('round-trips scoped v2 terminal records without expiring or merging authorities', async () => {
    const durability = createBrowserPlaybackProgressDurability(null);
    const oldRecord = terminalRecord(
      'server-a/account-a/profile-a/session-a',
      ['server-a', 'hosted', 'account-a', 'profile-a', 'auth-revision-a'],
      'session-a',
      '2020-01-01T00:00:00.000Z',
    );
    const foreignRecord = terminalRecord(
      'server-b/account-b/profile-b/session-b',
      ['server-b', 'local', 'account-b', 'profile-b', 'auth-revision-b'],
      'session-b',
      '2020-01-01T00:00:00.000Z',
    );

    // Core 0.1.13's source owns the scoped v2 record shape while its generated
    // package declarations are rebuilt concurrently with this Web test.
    await durability.save(oldRecord as unknown as DurablePlaybackProgressRecord);
    await durability.save(foreignRecord as unknown as DurablePlaybackProgressRecord);

    expect(await durability.load()).toEqual([oldRecord, foreignRecord]);
    await durability.remove(oldRecord.key);
    expect(await durability.load()).toEqual([foreignRecord]);
  });
});
