import React from 'react';
import {Navigate} from '@tanstack/react-router';
import {useSession} from './session';

/** Keeps the app frame behind sign-in and a chosen profile. */
export function Gate({children}: {children: React.ReactNode}) {
  const session = useSession();
  if (!session.signedIn) return <Navigate to="/welcome/$screen" params={{screen: 'signin'}} replace />;
  if (session.server?.status === 'unreachable') return <Navigate to="/welcome/$screen" params={{screen: 'unavailable'}} replace />;
  if (!session.profileId) return <Navigate to="/welcome/$screen" params={{screen: 'profiles'}} replace />;
  return <>{children}</>;
}
