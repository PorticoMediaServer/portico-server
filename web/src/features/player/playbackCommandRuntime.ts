import type { PlaybackCommand, PorticoClient } from '@portico/client-core';

type PlaybackCommandClient = Pick<PorticoClient, 'playbackCommand' | 'playbackCommandEventsUrl'>;

interface EventStreamLike {
  onopen: ((event: Event) => void) | null;
  onerror: ((event: Event) => void) | null;
  addEventListener(type: string, listener: EventListener): void;
  close(): void;
}

type EventStreamFactory = (url: string) => EventStreamLike;

export interface PlaybackCommandSubscriptionOptions {
  createEventStream?: EventStreamFactory;
  reconnectDelaysMs?: number[];
  pollingIntervalMs?: number;
  now?: () => number;
}

function defaultEventStreamFactory(url: string): EventStreamLike {
  return new EventSource(url, { withCredentials: true });
}

export function isHistoricalPlaybackCommand(command: PlaybackCommand, baselineMs: number): boolean {
  if (!command.issuedAt) return false;
  const issuedMs = Date.parse(command.issuedAt);
  return Number.isFinite(issuedMs) && issuedMs < baselineMs - 5_000;
}

export function playbackCommandClientFrom(value: unknown): PlaybackCommandClient | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const provider = (value as { porticoClient?: unknown }).porticoClient;
  if (typeof provider !== 'function') return undefined;
  const client = provider.call(value) as Partial<PlaybackCommandClient> | undefined;
  if (!client || typeof client.playbackCommand !== 'function' || typeof client.playbackCommandEventsUrl !== 'function') return undefined;
  return client as PlaybackCommandClient;
}

export function subscribeToPlaybackCommands(
  client: PlaybackCommandClient,
  sessionId: string,
  onCommand: (command: PlaybackCommand) => void,
  options: PlaybackCommandSubscriptionOptions = {},
) {
  const createEventStream = options.createEventStream ?? defaultEventStreamFactory;
  const reconnectDelaysMs = options.reconnectDelaysMs ?? [700, 1_400, 2_800];
  const pollingIntervalMs = options.pollingIntervalMs ?? 2_500;
  const baselineMs = (options.now ?? Date.now)();
  let stopped = false;
  let stream: EventStreamLike | undefined;
  let reconnectTimer: number | undefined;
  let pollingTimer: number | undefined;
  let failedAttempts = 0;
  let lastCommandId = '';

  const deliver = (command: PlaybackCommand) => {
    if (stopped || !command.id || command.id === lastCommandId) return;
    lastCommandId = command.id;
    if (!isHistoricalPlaybackCommand(command, baselineMs)) onCommand(command);
  };

  const poll = () => {
    if (stopped) return;
    void client.playbackCommand(sessionId).then(deliver).catch(() => undefined);
  };

  const startPolling = () => {
    if (stopped || pollingTimer !== undefined) return;
    poll();
    pollingTimer = window.setInterval(poll, pollingIntervalMs);
  };

  const scheduleReconnect = () => {
    if (stopped || reconnectTimer !== undefined) return;
    stream?.close();
    stream = undefined;
    const delay = reconnectDelaysMs[failedAttempts];
    failedAttempts += 1;
    if (delay === undefined) {
      startPolling();
      return;
    }
    reconnectTimer = window.setTimeout(connect, delay);
  };

  const connect = () => {
    if (stopped) return;
    reconnectTimer = undefined;
    let candidate: EventStreamLike;
    try {
      candidate = createEventStream(client.playbackCommandEventsUrl(sessionId));
      stream = candidate;
    } catch {
      scheduleReconnect();
      return;
    }
    candidate.onopen = () => {
      if (stopped || stream !== candidate) return;
      failedAttempts = 0;
    };
    candidate.addEventListener('command', ((event: MessageEvent<string>) => {
      if (stopped || stream !== candidate) return;
      try {
        deliver(JSON.parse(event.data) as PlaybackCommand);
      } catch {
        scheduleReconnect();
      }
    }) as EventListener);
    candidate.onerror = () => {
      if (stream === candidate) scheduleReconnect();
    };
  };

  connect();
  return () => {
    stopped = true;
    stream?.close();
    if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
    if (pollingTimer !== undefined) window.clearInterval(pollingTimer);
  };
}
