import {eventStreamOpener, sharedStreamOpener, type EventFeedEnvironment, type SharedStreamEnvironment} from '@core/event-feed.ts';
import type {StreamOpener} from '@core/playback-v1/events.ts';
import type {HttpLocalApi} from '@core/index.ts';

/**
 * The browser's side of the device's one event feed.
 *
 * The stream is a `fetch` that is read as it arrives (the browser's own EventSource cannot send
 * the sign-in). Every tab of this browser signed in to the same server as the same viewer shares
 * one stream: the server keeps one per installation, so tabs that each opened their own would
 * keep taking it from each other. A named lock decides which tab holds it; a broadcast channel
 * carries each event to the rest.
 *
 * That needs Web Locks, which browsers only give a secure page. Over plain HTTP from another
 * machine each tab uses the long poll the server kept, which tabs can run side by side.
 */
const sharing: SharedStreamEnvironment = {
  lock: (name, signal, work) => navigator.locks.request(name, {signal}, work) as Promise<void>,
  channel: name => {
    const channel = new BroadcastChannel(name);
    return {
      post: message => { try { channel.postMessage(message); } catch { /* closed while leaving the page */ } },
      listen: listener => {
        const heard = (event: MessageEvent) => listener(event.data);
        channel.addEventListener('message', heard);
        return () => channel.removeEventListener('message', heard);
      },
      close: () => channel.close(),
    };
  },
};

function canShare(): boolean {
  return typeof window !== 'undefined' && window.isSecureContext && typeof BroadcastChannel === 'function' && typeof navigator !== 'undefined' && typeof navigator.locks?.request === 'function';
}

/** The stream opener for this tab, or undefined where only the long poll can be used. `viewer` names who is signed in, never the token. */
export function browserEventStream(api: HttpLocalApi, viewer: string, token: () => string): StreamOpener | undefined {
  if (!canShare()) return undefined;
  const open = eventStreamOpener((headers, signal) => api.routeFetch(api.baseUrl + '/v1/events', {signal, cache: 'no-store', headers: {...headers, Authorization: 'Bearer ' + token()}}));
  return sharedStreamOpener('portico.events.' + viewer, open, sharing);
}

/** When the feed runs in a tab: while the tab is in front, or while `busy` (something is playing). */
export function browserFeedEnvironment(busy: () => boolean, onBusy: (listener: () => void) => () => void): EventFeedEnvironment {
  return {
    visible: () => document.visibilityState !== 'hidden',
    onVisibility: listener => {
      document.addEventListener('visibilitychange', listener);
      return () => document.removeEventListener('visibilitychange', listener);
    },
    busy, onBusy,
    graceMs: 30_000,
  };
}
