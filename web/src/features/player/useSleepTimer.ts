import { useCallback, useEffect, useRef, useState } from 'react';

export type SleepTimerMode = 'off' | 'end' | 15 | 30 | 45 | 60;

export function useSleepTimer(onExpire: () => void) {
  const expireRef = useRef(onExpire);
  expireRef.current = onExpire;
  const [mode, setModeState] = useState<SleepTimerMode>('off');
  const [deadline, setDeadline] = useState(0);
  const [remainingMinutes, setRemainingMinutes] = useState(0);

  const clear = useCallback(() => {
    setModeState('off');
    setDeadline(0);
    setRemainingMinutes(0);
  }, []);

  const setMode = useCallback((next: SleepTimerMode) => {
    setModeState(next);
    if (typeof next === 'number') {
      setDeadline(Date.now() + next * 60_000);
      setRemainingMinutes(next);
    } else {
      setDeadline(0);
      setRemainingMinutes(0);
    }
  }, []);

  useEffect(() => {
    if (!deadline) return;
    const update = () => {
      const remaining = deadline - Date.now();
      if (remaining <= 0) {
        clear();
        expireRef.current();
        return;
      }
      setRemainingMinutes(Math.max(1, Math.ceil(remaining / 60_000)));
    };
    update();
    const timer = window.setInterval(update, 1_000);
    return () => window.clearInterval(timer);
  }, [clear, deadline]);

  const expireAtTrackEnd = useCallback(() => {
    if (mode !== 'end') return false;
    clear();
    expireRef.current();
    return true;
  }, [clear, mode]);

  const label = mode === 'off'
    ? 'Off'
    : mode === 'end'
      ? 'End of track'
      : `${remainingMinutes || mode} min`;

  return { mode, label, setMode, clear, expireAtTrackEnd };
}
