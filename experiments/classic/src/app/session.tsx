import React, {createContext, useCallback, useContext, useEffect, useMemo, useState} from 'react';
import {useClient, useTopic} from '../data/DataProvider';
import type {Profile, Server, Session} from '../data/types';

type Phase = 'launch' | 'signedOut' | 'signedIn';
type SessionCtx = {
  phase: Phase;
  session?: Session;
  profile?: Profile;
  server?: Server;
  isAdmin: boolean;
  isOwner: boolean;
  enter: (profileId: string, pin?: string) => Promise<void>;
  signOut: () => Promise<void>;
  connectivity: 'online' | 'reconnecting' | 'offline';
  /** The auth screen to show when signed out (deep links can choose another). */
  authStart: string;
  setAuthStart: (s: string) => void;
};
const Ctx = createContext<SessionCtx | null>(null);

/**
 * App phases. On launch Portico restores the saved connection (the fixture
 * household's owner profile), so the app opens straight into a populated
 * library; signing out returns to the landing screen.
 */
export function SessionProvider({children}: {children: React.ReactNode}) {
  const client = useClient();
  useTopic(['session']);
  const [phase, setPhase] = useState<Phase>('launch');
  const [authStart, setAuthStart] = useState('Landing');
  useEffect(() => {
    let alive = true;
    (async () => {
      await client.session.chooseServer('srv-lighthouse');
      await client.session.chooseProfile('pr-justin');
      if (alive) setPhase('signedIn');
    })();
    return () => { alive = false; };
  }, [client]);
  const session = client.session.current();
  const enter = useCallback(async (profileId: string, pin?: string) => {
    await client.session.chooseProfile(profileId, pin);
    setPhase('signedIn');
  }, [client]);
  const signOut = useCallback(async () => {
    await client.session.signOut();
    setAuthStart('Landing');
    setPhase('signedOut');
  }, [client]);
  const value = useMemo<SessionCtx>(() => ({
    phase,
    session,
    profile: session?.profile,
    server: session?.server,
    isAdmin: session?.profile.role === 'owner' || session?.profile.role === 'admin',
    isOwner: session?.profile.role === 'owner',
    enter,
    signOut,
    connectivity: client.session.connectivity(),
    authStart,
    setAuthStart: (s: string) => { setAuthStart(s); setPhase('signedOut'); },
  }), [phase, session, enter, signOut, client, authStart]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession(): SessionCtx {
  return useContext(Ctx)!;
}
