import React, {createContext, useCallback, useContext, useEffect, useMemo, useState} from 'react';
import type {Profile, Server} from '../data/types';
import {useQuery, useServices} from '../data/hooks';

/**
 * Who is signed in, to which server, as which profile, and how the connection is doing. The fixture
 * flow starts signed in to the owner's home server as the main profile; sign-out returns to the
 * Portico Account sign-in. `role` drives what the shell offers (server administration is for the
 * owner and admins). Review switches: `?as=member|admin|owner`, `?profile=<id>`, `?connection=reconnecting`.
 */
export type Connection = 'online' | 'reconnecting' | 'offline';
export type Role = 'owner' | 'admin' | 'member';
type State = {signedIn: boolean; serverId: string; profileId: string | null; role: Role; connection: Connection; via: 'account' | 'direct'};
type Ctx = State & {
  server: Server | null;
  servers: Server[];
  profile: Profile | null;
  profiles: Profile[];
  setServer: (id: string) => void;
  setProfile: (id: string | null) => void;
  signIn: (via?: 'account' | 'direct') => void;
  signOut: () => void;
  setConnection: (c: Connection) => void;
  setRole: (r: Role) => void;
  manager: boolean;
};

const KEY = 'portico.session';
const SessionContext = createContext<Ctx | null>(null);

function initial(): State {
  let saved: Partial<State> = {};
  try { saved = JSON.parse(sessionStorage.getItem(KEY) ?? '{}'); } catch { /* ignore */ }
  const q = new URLSearchParams(location.search);
  const as = q.get('as');
  const base: State = {signedIn: true, serverId: 'srv-home', profileId: 'pr-justin', role: 'owner', connection: 'online', via: 'account', ...saved};
  if (as === 'member' || as === 'admin' || as === 'owner') base.role = as;
  if (q.get('profile')) base.profileId = q.get('profile');
  const c = q.get('connection');
  base.connection = c === 'reconnecting' || c === 'offline' ? c : 'online';
  return base;
}

export function SessionProvider({children}: {children: React.ReactNode}) {
  const services = useServices();
  const [s, setS0] = useState<State>(initial);
  // Review switches in the URL (?as=, ?profile=) apply to this visit only; real changes are kept.
  const [transient, setTransient] = useState(() => /[?&](as|profile)=/.test(location.search));
  const setS: typeof setS0 = v => { setTransient(false); setS0(v); };
  useEffect(() => {
    if (transient) return;
    try { sessionStorage.setItem(KEY, JSON.stringify({...s, connection: 'online'})); } catch { /* ignore */ }
  }, [s, transient]);
  const servers = useQuery('auth.servers', () => services.auth.servers()).data ?? [];
  const profiles = useQuery(`profiles.${s.serverId}`, () => services.account.profiles()).data ?? [];
  const setServer = useCallback((serverId: string) => setS(p => ({...p, serverId, profileId: null})), []);
  const setProfile = useCallback((profileId: string | null) => setS(p => ({...p, profileId})), []);
  const signIn = useCallback((via: 'account' | 'direct' = 'account') => setS(p => ({...p, signedIn: true, via})), []);
  const signOut = useCallback(() => setS(p => ({...p, signedIn: false, profileId: null})), []);
  const setConnection = useCallback((connection: Connection) => setS(p => ({...p, connection})), []);
  const setRole = useCallback((role: Role) => setS(p => ({...p, role})), []);
  const server = servers.find(x => x.id === s.serverId) ?? null;
  const profile = profiles.find(p => p.id === s.profileId) ?? null;
  useEffect(() => { services.setViewer(profile && !profile.main ? profile.limits : null); }, [services, profile]);
  const role: Role = server && server.id !== 'srv-home' ? server.role : s.role;
  const value = useMemo<Ctx>(() => ({...s, role, server, servers, profile, profiles, setServer, setProfile, signIn, signOut, setConnection, setRole, manager: role !== 'member' && !profile?.kids}), [s, role, server, servers, profile, profiles, setServer, setProfile, signIn, signOut, setConnection, setRole]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Ctx {
  const c = useContext(SessionContext);
  if (!c) throw new Error('SessionProvider missing');
  return c;
}
