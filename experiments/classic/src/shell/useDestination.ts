import {useEffect, useState} from 'react';
import {rootNav} from '../nav/nav';

type R = {name: string; params?: Record<string, unknown>};

function read(): {root?: R; top?: R; depth: number} {
  if (!rootNav.isReady()) return {depth: 0};
  const state = rootNav.getRootState();
  const main = state?.routes.find(r => r.name === 'Main');
  const inner = main?.state as {index?: number; routes: R[]} | undefined;
  if (!inner?.routes?.length) return {root: {name: 'Home'}, top: {name: 'Home'}, depth: 1};
  return {root: inner.routes[0], top: inner.routes[inner.index ?? inner.routes.length - 1], depth: inner.routes.length};
}

/** The content stack's root destination (what the rail highlights) and depth. */
export function useDestination() {
  const [d, setD] = useState(read);
  useEffect(() => {
    const update = () => setD(read());
    update();
    const off = rootNav.addListener('state', update);
    return off;
  }, []);
  return d;
}

export function destinationKey(r?: R): string {
  if (!r) return 'Home';
  if (r.name === 'Library') return `lib:${r.params?.libraryId}`;
  if (r.name === 'Live') return `live:${(r.params?.view as string) ?? 'guide'}`;
  if (r.name === 'SettingsSection' && r.params?.section === 'notifications') return 'notifications';
  if (r.name === 'SettingsSection' || r.name === 'Account') return 'Settings';
  return r.name;
}
