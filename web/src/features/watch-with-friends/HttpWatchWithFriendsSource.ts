import type { WatchWithFriendsGroup } from '@porticomediaserver/client-core';
import type {
  WatchConnectionState,
  WatchGroupSubscription,
  WatchWithFriendsClient,
  WatchWithFriendsSource,
} from './watchWithFriendsSource';

interface EventStreamLike {
  onopen: ((event: Event) => void) | null;
  onerror: ((event: Event) => void) | null;
  addEventListener(type: string, listener: EventListener): void;
  close(): void;
}

type EventStreamFactory = (url: string) => EventStreamLike;

export interface HttpWatchWithFriendsSourceOptions {
  createEventStream?: EventStreamFactory;
  reconnectDelaysMs?: number[];
}

function defaultEventStreamFactory(url: string): EventStreamLike {
  return new EventSource(url, { withCredentials: true });
}

function abortError() {
  return new DOMException('The request was cancelled.', 'AbortError');
}

function withSignal<T>(promise: Promise<T>, signal?: AbortSignal): Promise<T> {
  if (!signal) return promise;
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<T>((resolve, reject) => {
    const abort = () => reject(abortError());
    signal.addEventListener('abort', abort, { once: true });
    promise.then(resolve, reject).finally(() => signal.removeEventListener('abort', abort));
  });
}

export class HttpWatchWithFriendsSource implements WatchWithFriendsSource {
  private readonly createEventStream: EventStreamFactory;
  private readonly reconnectDelaysMs: number[];

  constructor(private readonly client: WatchWithFriendsClient, options: HttpWatchWithFriendsSourceOptions = {}) {
    this.createEventStream = options.createEventStream ?? defaultEventStreamFactory;
    this.reconnectDelaysMs = options.reconnectDelaysMs ?? [700, 1_400, 2_800];
  }

  listGroups(signal?: AbortSignal) {
    return withSignal(this.client.watchWithFriendsGroups().then((response) => response.items), signal);
  }

  group(id: string, signal?: AbortSignal) {
    return withSignal(this.client.watchWithFriendsGroup(id), signal);
  }

  createGroup(request: Parameters<WatchWithFriendsSource['createGroup']>[0], signal?: AbortSignal) {
    return withSignal(this.client.createWatchWithFriendsGroup(request), signal);
  }

  joinGroup(id: string, signal?: AbortSignal) {
    return withSignal(this.client.joinWatchWithFriendsGroup(id), signal);
  }

  leaveGroup(id: string, signal?: AbortSignal) {
    return withSignal(this.client.leaveWatchWithFriendsGroup(id), signal);
  }

  endGroup(id: string, expectedRevision: number, idempotencyKey: string, signal?: AbortSignal) {
    return withSignal(this.client.endWatchWithFriendsGroup(id, expectedRevision, idempotencyKey), signal);
  }

  updateMemberState(id: string, request: Parameters<WatchWithFriendsSource['updateMemberState']>[1], signal?: AbortSignal) {
    return withSignal(this.client.updateWatchWithFriendsMemberState(id, request), signal);
  }

  updatePlaybackState(id: string, request: Parameters<WatchWithFriendsSource['updatePlaybackState']>[1], signal?: AbortSignal) {
    return withSignal(this.client.updateWatchWithFriendsState(id, request), signal);
  }

  updateSettings(id: string, request: Parameters<WatchWithFriendsSource['updateSettings']>[1], signal?: AbortSignal) {
    return withSignal(this.client.updateWatchWithFriendsSettings(id, request), signal);
  }

  addQueueItem(id: string, request: Parameters<WatchWithFriendsSource['addQueueItem']>[1], signal?: AbortSignal) {
    return withSignal(this.client.addWatchWithFriendsQueueItem(id, request), signal);
  }

  reorderQueue(id: string, request: Parameters<WatchWithFriendsSource['reorderQueue']>[1], signal?: AbortSignal) {
    return withSignal(this.client.reorderWatchWithFriendsQueue(id, request), signal);
  }

  removeQueueItem(id: string, entryId: string, expectedRevision: number, idempotencyKey: string, signal?: AbortSignal) {
    return withSignal(this.client.removeWatchWithFriendsQueueItem(id, entryId, expectedRevision, idempotencyKey), signal);
  }

  subscribe(id: string, subscription: WatchGroupSubscription) {
    let stopped = false;
    let stream: EventStreamLike | undefined;
    let timer: number | undefined;
    let failedAttempts = 0;

    const report = (status: WatchConnectionState) => {
      if (!stopped) subscription.onStatus(status);
    };
    const connect = () => {
      if (stopped) return;
      timer = undefined;
      report(failedAttempts === 0 ? 'connecting' : 'reconnecting');
      try {
        stream = this.createEventStream(this.client.watchWithFriendsGroupEventsUrl(id));
      } catch (reason) {
        scheduleReconnect(reason instanceof Error ? reason : new Error('Live group updates could not be opened.'));
        return;
      }
      stream.onopen = () => {
        if (stopped) return;
        failedAttempts = 0;
        report('live');
      };
      stream.addEventListener('group', ((event: MessageEvent<string>) => {
        if (stopped) return;
        try {
          subscription.onGroup(JSON.parse(event.data) as WatchWithFriendsGroup);
        } catch {
          subscription.onError(new Error('A live group update could not be read.'));
        }
      }) as EventListener);
      stream.onerror = () => scheduleReconnect(new Error('Live group updates were interrupted.'));
    };
    const scheduleReconnect = (error: Error) => {
      if (stopped || timer !== undefined) return;
      stream?.close();
      stream = undefined;
      const delay = this.reconnectDelaysMs[failedAttempts];
      failedAttempts += 1;
      if (delay === undefined) {
        report('failed');
        subscription.onError(error);
        return;
      }
      report('reconnecting');
      timer = window.setTimeout(connect, delay);
    };

    connect();
    return () => {
      stopped = true;
      if (timer !== undefined) window.clearTimeout(timer);
      stream?.close();
    };
  }
}
