import React, {createContext, useCallback, useContext, useEffect, useMemo, useState} from 'react';
import {useClient, useTopic} from '@/data/DataProvider';
import type {Profile, Server, Session} from '@/data/types';
import {store} from './storage';

type Phase = 'booting' | 'signedOut' | 'signedIn';
type Saved = {serverId: string; profileId: string} | null;
type SessionCtx = {
  phase: Phase;
  session?: Session;
  profile?: Profile;
  server?: Server;
  isAdmin: boolean;
  isOwner: boolean;
  connectivity: 'online' | 'reconnecting' | 'offline';
  /** Enters the chosen profile (PIN checked by the server) and remembers it for next time. */
  enter: (profileId: string, pin?: string) => Promise<void>;
  signOut: () => Promise<void>;
};
const Ctx = createContext<SessionCtx | null>(null);
const KEY = 'session';
/** First visit: open straight into the fixture household as its owner, so review starts in a populated library. */
const DEFAULT: Saved = {serverId: 'srv-lighthouse', profileId: 'pr-justin'};

/**
 * Who is signed in, where, and whether the server is reachable. The browser
 * keeps the chosen server and profile; on load the session resumes without
 * the PIN, as a real session token would.
 */
export function SessionProvider({children}: {children: React.ReactNode}) {
  const client = useClient();
  useTopic(['session']);
  const [phase, setPhase] = useState<Phase>('booting');
  useEffect(() => {
    const saved = store.get<Saved>(KEY, DEFAULT);
    if (!saved) { setPhase('signedOut'); return; }
    void client.session.resume(saved.serverId, saved.profileId).then(ok => setPhase(ok ? 'signedIn' : 'signedOut'));
  }, [client]);
  const session = client.session.current();
  const enter = useCallback(async (profileId: string, pin?: string) => {
    await client.session.chooseProfile(profileId, pin);
    const s = client.session.current();
    if (s) store.set(KEY, {serverId: s.server.id, profileId: s.profile.id});
    setPhase('signedIn');
  }, [client]);
  const signOut = useCallback(async () => {
    await client.session.signOut();
    store.set(KEY, null);
    setPhase('signedOut');
  }, [client]);
  const value = useMemo<SessionCtx>(() => ({
    phase,
    session,
    profile: session?.profile,
    server: session?.server,
    isAdmin: session?.profile.role === 'owner' || session?.profile.role === 'admin',
    isOwner: session?.profile.role === 'owner',
    connectivity: client.session.connectivity(),
    enter,
    signOut,
  }), [phase, session, enter, signOut, client]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession(): SessionCtx {
  return useContext(Ctx)!;
}
