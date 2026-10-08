import {Dialog} from '@/ui/Overlay';
import {Kbd} from '@/ui/Bits';
import s from './shell-bits.module.css';

const GROUPS: {title: string; keys: [string, string[]][]}[] = [
  {title: 'Anywhere', keys: [['Search', ['/']], ['Keyboard shortcuts', ['?']], ['Go to Home', ['g', 'h']], ['Go to Saved', ['g', 's']], ['Go to Live TV', ['g', 'l']], ['Go to Settings', ['g', 't']], ['Go to Manage server', ['g', 'm']]]},
  {title: 'Rows and grids', keys: [['Move between titles', ['←', '→', '↑', '↓']], ['First or last title', ['Home', 'End']], ['Open', ['Enter']], ['Play', ['P']], ['Actions menu', ['Shift', 'F10']], ['Select for bulk actions', ['X']]]},
  {title: 'Player', keys: [['Play or pause', ['Space']], ['Back 10 s, forward 30 s', ['←', '→']], ['Volume', ['↑', '↓']], ['Mute', ['M']], ['Full screen', ['F']], ['Subtitles on or off', ['C']], ['Next or previous', ['Shift', 'N / P']], ['Close or minimize', ['Esc']]]},
  {title: 'Live TV', keys: [['Channel up or down', ['Page Up', 'Page Down']], ['Mini guide', ['G']], ['Record this program', ['R']], ['Guide: move through programs', ['←', '→', '↑', '↓']]]},
];

export function ShortcutsDialog({open, onOpenChange}: {open: boolean; onOpenChange: (o: boolean) => void}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Keyboard shortcuts" description="Portico works fully from the keyboard. Tab moves between areas; arrow keys move within them." width={760}>
      <div className={s.keys}>
        {GROUPS.map(g => (
          <section key={g.title} className={s.keyGroup}>
            <h3>{g.title}</h3>
            {g.keys.map(([label, keys]) => <div key={label} className={s.key}><span>{label}</span><span>{keys.map(k => <Kbd key={k}>{k}</Kbd>)}</span></div>)}
          </section>
        ))}
      </div>
    </Dialog>
  );
}
