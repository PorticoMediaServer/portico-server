import React, {createContext, useCallback, useContext, useRef, useState} from 'react';
import {tints} from '../data/fixtures/art.generated';
import {Atmosphere} from '../ui/Canvas';

const Ctx = createContext<(id?: string) => void>(() => {});

/** The canvas glow, tinted toward the title in focus (TV) or on screen (hero). */
export function GlowProvider({children}: {children: React.ReactNode}) {
  const [tint, setTint] = useState<string>();
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const set = useCallback((id?: string) => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setTint(id ? (tints as Record<string, string>)[id] : undefined), 120);
  }, []);
  return (
    <Ctx.Provider value={set}>
      <Atmosphere tint={tint} />
      {children}
    </Ctx.Provider>
  );
}

export const useGlow = () => useContext(Ctx);
