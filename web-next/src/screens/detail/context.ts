import type {HttpLocalApi} from '@core/index.ts';
import type {ContentScope} from '@core/library-content.ts';
import type {SearchEntry} from '@core/search.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import type {ShellArtwork} from '../../shell/artwork';

/** Captured by the shell for one server/account/profile/role epoch. Not a wire DTO. */
export type ScreenContext = Readonly<{
  api: HttpLocalApi;
  events: EventsClient;
  scope: ContentScope;
  isCurrent: () => boolean;
  artwork: ShellArtwork;
  preferences: Readonly<{getSnapshot: () => PreferenceSnapshot; subscribe: (listener: () => void) => () => void}>;
  effects: Readonly<{
    openEntry: (entry: SearchEntry) => void;
    openSettings: () => void;
    play: (target: Readonly<{libraryId: string; itemId: string; startSeconds: number; versionId?: string}>, signal: AbortSignal) => Promise<void>;
    reportError: (error: unknown) => void;
  }>;
}>;

/** Every request, including reads started by a service retry, checks the captured authority. */
export function screenLifetime(context: ScreenContext) {
  const life = new AbortController();
  const current = () => !life.signal.aborted && context.isCurrent();
  const assert = () => { if (!current()) throw new DOMException('Screen authority changed', 'AbortError'); };
  const signal = (input?: AbortSignal) => input ? AbortSignal.any([life.signal, input]) : life.signal;
  const api = {
    async request<T>(path: string, method?: string, body?: unknown, input?: AbortSignal): Promise<T> {
      assert();
      const value = await context.api.request<T>(path, method, body, signal(input));
      assert(); return value;
    },
    async requestRaw(...args: Parameters<HttpLocalApi['requestRaw']>) {
      assert();
      const [path, method, options] = args;
      const value = await context.api.requestRaw(path, method, {...options, signal: signal(options?.signal)});
      assert(); return value;
    },
  };
  return {api, current, assert, signal, dispose: () => life.abort()};
}
