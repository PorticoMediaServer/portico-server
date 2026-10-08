import React, {createContext, useContext, useEffect, useRef} from 'react';

/**
 * Review states. The screen index opens pages with `?demo=<state>` to show a
 * dialog, a panel or playback that normally needs a click. Components that own
 * the state call `useDemo(name, open)`. Not used by the product itself.
 */
const DemoContext = createContext<string | undefined>(undefined);

export function DemoProvider({children}: {children: React.ReactNode}) {
  const [demo] = React.useState(() => { try { return new URLSearchParams(location.search).get('demo') ?? undefined; } catch { return undefined; } });
  return <DemoContext.Provider value={demo}>{children}</DemoContext.Provider>;
}

export const useDemoName = () => useContext(DemoContext);

/** Runs `fn` once, shortly after mount, when the page was opened with this demo state. `ready` delays it until data is in. */
export function useDemo(name: string | string[], fn: (name: string) => void, ready = true) {
  const demo = useContext(DemoContext);
  const done = useRef(false);
  const names = Array.isArray(name) ? name : [name];
  useEffect(() => {
    if (!demo || done.current || !ready || !names.includes(demo)) return;
    const t = setTimeout(() => { if (done.current) return; done.current = true; fn(demo); }, 300);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [demo, ready]);
}
