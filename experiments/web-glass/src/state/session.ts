import { create } from 'zustand';
import { setCurrentProfile } from '../services/fixture/db';

export type Transparency = 'system' | 'reduced' | 'full';
export type Motion = 'system' | 'reduced' | 'full';

interface SessionState {
  signedIn: boolean;
  serverId: string | null;
  profileId: string | null;
  setupComplete: boolean;
  transparency: Transparency;
  motion: Motion;
  signIn: () => void;
  chooseServer: (id: string) => void;
  chooseProfile: (id: string) => void;
  signOut: () => void;
  switchProfile: () => void;
  setTransparency: (t: Transparency) => void;
  setMotion: (m: Motion) => void;
}

const KEY = 'portico.session.v1';
type Persisted = Pick<SessionState, 'signedIn' | 'serverId' | 'profileId' | 'setupComplete' | 'transparency' | 'motion'>;

function read(): Persisted {
  const fallback: Persisted = { signedIn: false, serverId: null, profileId: null, setupComplete: true, transparency: 'system', motion: 'system' };
  try {
    // ?as=<profile> signs straight in as that profile (used by the screen index and screenshots).
    const as = new URLSearchParams(window.location.search).get('as');
    if (as) {
      const v = as === 'signed-out' ? fallback : { ...fallback, signedIn: true, serverId: 'srv-harbor', profileId: as === 'none' ? null : as };
      localStorage.setItem(KEY, JSON.stringify(v));
      return v;
    }
    const raw = localStorage.getItem(KEY);
    return raw ? { ...fallback, ...JSON.parse(raw) } : fallback;
  } catch {
    return fallback;
  }
}

const initial = read();
if (initial.profileId) setCurrentProfile(initial.profileId);

export const useSession = create<SessionState>((set, get) => {
  const save = () => {
    const { signedIn, serverId, profileId, setupComplete, transparency, motion } = get();
    try {
      localStorage.setItem(KEY, JSON.stringify({ signedIn, serverId, profileId, setupComplete, transparency, motion }));
    } catch {
      /* private mode */
    }
  };
  return {
    ...initial,
    signIn: () => {
      set({ signedIn: true });
      save();
    },
    chooseServer: (id) => {
      set({ serverId: id });
      save();
    },
    chooseProfile: (id) => {
      setCurrentProfile(id);
      set({ profileId: id });
      save();
    },
    switchProfile: () => {
      set({ profileId: null });
      save();
    },
    signOut: () => {
      set({ signedIn: false, serverId: null, profileId: null });
      save();
    },
    setTransparency: (transparency) => {
      set({ transparency });
      save();
    },
    setMotion: (motion) => {
      set({ motion });
      save();
    },
  };
});
