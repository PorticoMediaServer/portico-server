import {useEffect, useRef} from 'react';
import {useServices} from '../data/hooks';
import {player} from './player';
import {toast} from '../ui/Feedback';

/** Channel reminders fire as a toast 2 minutes before the program starts, while Portico is open. */
export function ReminderWatcher() {
  const services = useServices();
  const fired = useRef(new Set<string>());
  useEffect(() => {
    const check = async () => {
      const ids = await services.live.reminders();
      for (const id of ids) {
        if (fired.current.has(id)) continue;
        const p = await services.live.programme(id);
        if (!p || p.start - Date.now() > 2 * 60_000 || p.end < Date.now()) continue;
        fired.current.add(id);
        const channel = (await services.live.channels()).find(c => c.id === p.channelId);
        toast(`${p.title} starts in 2 minutes on ${channel?.name ?? 'Live TV'}`, {icon: 'bell', duration: 12000, action: {label: 'Watch', onClick: () => player.playLive(p.channelId)}});
      }
    };
    void check();
    const t = setInterval(() => void check(), 30_000);
    return () => clearInterval(t);
  }, [services]);
  return null;
}
