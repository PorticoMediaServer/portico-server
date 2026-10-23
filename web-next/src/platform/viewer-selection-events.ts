import type {LocalSession} from '@core/index.ts';

/** Watch durable selection facts, never credentials in an event payload. The
 * runtime supplies its existing authority comparison, so token rotations do
 * not rebuild a player but family/server/account/profile changes retire it. */
export function watchViewerSelection(options: {
  subscribe: (changed: () => void) => () => void;
  read: () => Promise<{session: LocalSession} | null>;
  current: () => LocalSession;
  same: (left: LocalSession, right: LocalSession) => boolean;
  changed: () => void;
  failed?: (error: unknown) => void;
}) {
  let stopped = false, reading = false, revision = 0;
  const check = async () => {
    ++revision; if (reading) return; reading = true;
    try {
      while (!stopped) {
        const expected = revision;
        let selected: {session: LocalSession} | null;
        try {selected = await options.read();} catch (error) {if (!stopped) options.failed?.(error); return;}
        if (stopped) return;
        if (!selected || !options.same(options.current(), selected.session)) {options.changed(); return;}
        if (expected === revision) return;
      }
    } finally {reading = false;}
  };
  const unsubscribe = options.subscribe(() => {void check();});
  void check(); // A change during asynchronous restoration must not be missed.
  return () => {stopped = true; unsubscribe();};
}
