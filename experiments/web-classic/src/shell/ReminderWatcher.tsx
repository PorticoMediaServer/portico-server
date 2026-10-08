import {useEffect, useRef} from 'react';
import {useClient} from '@/data/DataProvider';
import {usePlayer} from '@/player/PlayerProvider';
import {channelEntry} from '@/app/liveSources';
import {useToast} from '@/ui/Overlay';

/** Channel reminders: while Portico is open, a toast says when a reminded program is about to start, with Watch. */
export function ReminderWatcher() {
  const client = useClient();
  const player = usePlayer();
  const toast = useToast();
  const told = useRef(new Set<string>());
  useEffect(() => {
    const check = async () => {
      const soon = client.live.reminders().filter(id => { const start = Number(id.split('@')[1]); return !told.current.has(id) && start - Date.now() < 60000 && start > Date.now() - 600000; });
      for (const id of soon) {
        told.current.add(id);
        const {program, channel} = await client.live.program(id);
        toast.show(`“${program.title}” is starting on ${channel.name}`, {icon: 'bell', duration: 12000, action: {label: 'Watch', run: () => player.playLive(channel.id, channelEntry(channel, program))}});
      }
    };
    void check();
    const t = setInterval(() => void check(), 20000);
    return () => clearInterval(t);
  }, [client, player, toast]);
  return null;
}
