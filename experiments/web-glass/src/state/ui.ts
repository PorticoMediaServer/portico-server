import { create } from 'zustand';

const readOffline = () => {
  try {
    // ?as= links (screen index, screenshots) always start online.
    const q = new URLSearchParams(window.location.search);
    if (q.has('as')) localStorage.removeItem('portico.offline');
    if (q.get('offline') === '1') localStorage.setItem('portico.offline', '1');
    return localStorage.getItem('portico.offline') === '1';
  } catch {
    return false;
  }
};

interface UiState {
  offline: boolean;
  setOffline: (v: boolean) => void;
  animeHint: boolean;
  palette: boolean;
  paletteQuery: string;
  notifications: boolean;
  cast: boolean;
  shortcuts: boolean;
  setAnimeHint: (v: boolean) => void;
  openPalette: (q?: string) => void;
  closePalette: () => void;
  setNotifications: (v: boolean) => void;
  setCast: (v: boolean) => void;
  setShortcuts: (v: boolean) => void;
}

export const useUi = create<UiState>((set) => ({
  offline: readOffline(),
  setOffline: (offline) => {
    try {
      localStorage.setItem('portico.offline', offline ? '1' : '0');
    } catch {
      /* ignore */
    }
    set({ offline });
  },
  animeHint: false,
  palette: false,
  paletteQuery: '',
  notifications: false,
  cast: false,
  shortcuts: false,
  setAnimeHint: (animeHint) => set({ animeHint }),
  openPalette: (paletteQuery = '') => set({ palette: true, paletteQuery }),
  closePalette: () => set({ palette: false }),
  setNotifications: (notifications) => set({ notifications }),
  setCast: (cast) => set({ cast }),
  setShortcuts: (shortcuts) => set({ shortcuts }),
}));
