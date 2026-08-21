import type { PlaybackCommand, PorticoClient } from '@portico/client-core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  isHistoricalPlaybackCommand,
  playbackCommandClientFrom,
  subscribeToPlaybackCommands,
} from './playbackCommandRuntime';

class TestEventStream {
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  listeners = new Map<string, EventListener>();
  close = vi.fn();

  addEventListener(type: string, listener: EventListener) {
    this.listeners.set(type, listener);
  }

  emit(type: string, value: unknown) {
    this.listeners.get(type)?.({ data: JSON.stringify(value) } as MessageEvent<string>);
  }
}

afterEach(() => vi.useRealTimers());

describe('playback command runtime', () => {
  it('ignores historical and duplicate commands while delivering current SSE commands', () => {
    const stream = new TestEventStream();
    const client = {
      playbackCommand: vi.fn(),
      playbackCommandEventsUrl: vi.fn().mockReturnValue('/api/playback-sessions/session-1/command/events'),
    } as unknown as Pick<PorticoClient, 'playbackCommand' | 'playbackCommandEventsUrl'>;
    const onCommand = vi.fn();
    const stop = subscribeToPlaybackCommands(client, 'session-1', onCommand, {
      createEventStream: () => stream,
      now: () => Date.parse('2026-07-13T12:00:00Z'),
    });

    const historical: PlaybackCommand = { id: 'old', action: 'pause', issuedAt: '2026-07-13T11:59:00Z' };
    const current: PlaybackCommand = { id: 'new', action: 'seek', positionSeconds: 42, issuedAt: '2026-07-13T12:00:00Z' };
    stream.emit('command', historical);
    stream.emit('command', current);
    stream.emit('command', current);

    expect(onCommand).toHaveBeenCalledOnce();
    expect(onCommand).toHaveBeenCalledWith(current);
    stop();
    expect(stream.close).toHaveBeenCalledOnce();
  });

  it('falls back to command polling after bounded SSE reconnect attempts', async () => {
    vi.useFakeTimers();
    const command: PlaybackCommand = { id: 'polled', action: 'play', issuedAt: '2026-07-13T12:00:00Z' };
    const client = {
      playbackCommand: vi.fn().mockResolvedValue(command),
      playbackCommandEventsUrl: vi.fn().mockReturnValue('/events'),
    } as unknown as Pick<PorticoClient, 'playbackCommand' | 'playbackCommandEventsUrl'>;
    const streams: TestEventStream[] = [];
    const onCommand = vi.fn();
    const stop = subscribeToPlaybackCommands(client, 'session-1', onCommand, {
      createEventStream: () => {
        const stream = new TestEventStream();
        streams.push(stream);
        return stream;
      },
      reconnectDelaysMs: [10],
      pollingIntervalMs: 50,
      now: () => Date.parse('2026-07-13T12:00:00Z'),
    });

    streams[0].onerror?.(new Event('error'));
    await vi.advanceTimersByTimeAsync(10);
    streams[1].onerror?.(new Event('error'));
    await vi.advanceTimersByTimeAsync(1);

    expect(client.playbackCommand).toHaveBeenCalledWith('session-1');
    expect(onCommand).toHaveBeenCalledWith(command);
    stop();
  });

  it('discovers only data sources that expose the complete command client', () => {
    const client = { playbackCommand: vi.fn(), playbackCommandEventsUrl: vi.fn() };
    expect(playbackCommandClientFrom({ porticoClient: () => client })).toBe(client);
    expect(playbackCommandClientFrom({ porticoClient: () => ({ playbackCommand: vi.fn() }) })).toBeUndefined();
    expect(isHistoricalPlaybackCommand({ issuedAt: '2026-07-13T11:00:00Z' }, Date.parse('2026-07-13T12:00:00Z'))).toBe(true);
  });
});
