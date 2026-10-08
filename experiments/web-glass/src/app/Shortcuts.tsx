import { useUi } from '../state/ui';
import { Sheet } from '../components/overlay';

const GROUPS: [string, [string[], string][]][] = [
  [
    'Anywhere',
    [
      [['⌘', 'K'], 'Search and commands'],
      [['/'], 'Search'],
      [['?'], 'Show this list'],
      [['Space'], 'Play or pause music'],
    ],
  ],
  [
    'Go to',
    [
      [['g', 'h'], 'Home'],
      [['g', 'm'], 'Movies'],
      [['g', 't'], 'TV Shows'],
      [['g', 'a'], 'Anime'],
      [['g', 'u'], 'Music'],
      [['g', 'l'], 'Live TV'],
      [['g', 'w'], 'Watchlist'],
      [['g', 'c'], 'Server Console'],
    ],
  ],
  [
    'Player',
    [
      [['Space'], 'Play / pause'],
      [['←', '→'], 'Back / forward 10 seconds'],
      [['↑', '↓'], 'Volume'],
      [['F'], 'Full screen'],
      [['C'], 'Subtitles on / off'],
      [['M'], 'Mute'],
      [['N'], 'Next episode'],
      [['Esc'], 'Close player'],
    ],
  ],
  [
    'Server Console',
    [
      [['j', 'k'], 'Move through rows'],
      [['1', '–', '9'], 'Switch section'],
      [['r'], 'Refresh'],
      [['/'], 'Filter'],
    ],
  ],
];

export function ShortcutsSheet() {
  const open = useUi((s) => s.shortcuts);
  const set = useUi((s) => s.setShortcuts);
  return (
    <Sheet open={open} onClose={() => set(false)} title="Keyboard Shortcuts" size="lg">
      <div className="shortcuts">
        {GROUPS.map(([g, rows]) => (
          <section key={g}>
            <h3>{g}</h3>
            {rows.map(([keys, label]) => (
              <div key={label} className="shortcut-row">
                <span>{label}</span>
                <span className="keys">
                  {keys.map((k) => (
                    <kbd key={k}>{k}</kbd>
                  ))}
                </span>
              </div>
            ))}
          </section>
        ))}
      </div>
    </Sheet>
  );
}
