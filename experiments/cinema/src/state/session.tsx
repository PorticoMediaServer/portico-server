import React, {createContext, useCallback, useContext, useMemo, useState} from 'react';
import type {Profile, Server} from '../data/types';

/**
 * Who is signed in, to which server, as which profile. The fixture flow starts signed in to the
 * owner's home server; sign-out returns to the Portico Account sign-in. `role` drives what the
 * shell offers (server administration is owner/admin only).
 */
export type Connection = 'online' | 'reconnecting' | 'offline';
export type Session = {
  signedIn: boolean;
  server: Server | null;
  profile: Profile | null;
  role: 'owner' | 'admin' | 'member';
  connection: Connection;
  via: 'account' | 'direct';
};

type Ctx = Session & {
  setServer: (s: Server) => void;
  setProfile: (p: Profile) => void;
  signIn: (via?: 'account' | 'direct') => void;
  signOut: () => void;
  setConnection: (c: Connection) => void;
  setRole: (r: Session['role']) => void;
};

const SessionContext = createContext<Ctx | null>(null);

export function SessionProvider({children, initial}: {children: React.ReactNode; initial: Partial<Session>}) {
  const [s, setS] = useState<Session>({signedIn: true, server: null, profile: null, role: 'owner', connection: 'online', via: 'account', ...initial});
  const setServer = useCallback((server: Server) => setS(p => ({...p, server, role: server.role})), []);
  const setProfile = useCallback((profile: Profile) => setS(p => ({...p, profile})), []);
  const signIn = useCallback((via: 'account' | 'direct' = 'account') => setS(p => ({...p, signedIn: true, via})), []);
  const signOut = useCallback(() => setS(p => ({...p, signedIn: false, profile: null})), []);
  const setConnection = useCallback((connection: Connection) => setS(p => ({...p, connection})), []);
  const setRole = useCallback((role: Session['role']) => setS(p => ({...p, role})), []);
  const value = useMemo(() => ({...s, setServer, setProfile, signIn, signOut, setConnection, setRole}), [s, setServer, setProfile, signIn, signOut, setConnection, setRole]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Ctx {
  const c = useContext(SessionContext);
  if (!c) throw new Error('SessionProvider missing');
  return c;
}

export const isManager = (role: Session['role']) => role === 'owner' || role === 'admin';
