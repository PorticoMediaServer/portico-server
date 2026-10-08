import {useEffect, useRef} from 'react';
import {useClient} from '../data/DataProvider';
import {useToast} from '../app/toast';
import {useActions} from '../app/actions';

/** Channel reminders fire as a toast two minutes before the program, while Portico is open. */
export function ReminderWatcher() {
  const client = useClient();
  const toast = useToast();
  const actions = useActions();
  const fired = useRef(new Set<string>());
  useEffect(() => {
    const t = setInterval(async () => {
      const now = Date.now();
      for (const id of client.live.reminders()) {
        if (fired.current.has(id)) continue;
        const start = Number(id.split('@')[1]);
        if (start - now < 2 * 60000 && start > now - 5 * 60000) {
          fired.current.add(id);
          const {program, channel} = await client.live.program(id);
          toast({message: `${program.title} starts in 2 minutes on ${channel.name}`, icon: 'bell', action: {label: 'Watch', onPress: () => void actions.play({id: channel.id, kind: 'channel', title: program.title, subtitle: channel.name, image: program.image, shape: 'wide'})}});
        }
      }
    }, 15000);
    return () => clearInterval(t);
  }, [client, toast, actions]);
  return null;
}
