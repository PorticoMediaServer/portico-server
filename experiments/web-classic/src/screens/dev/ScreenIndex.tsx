import screens from '@/app/screens.json';
import {useTitle} from '@/ui/hooks';
import s from './ScreenIndex.module.css';

type Screen = {group: string; id: string; label: string; path: string};

/**
 * Every page and state, for review. Links are full page loads so each opens
 * fresh, and states that need a click open through `?demo=`. Add `latency=0`
 * to skip the simulated network delay.
 */
export default function ScreenIndex() {
  useTitle('Screen index');
  const list = screens as Screen[];
  const groups = [...new Set(list.map(x => x.group))];
  return (
    <main className={s.page}>
      <header className={s.head}>
        <img src="/brand/portico-symbol-dark.svg" alt="" className={s.mark} />
        <div>
          <h1 className="t-title">Screen index</h1>
          <p className="c-2">{list.length} screens and states of Portico Web Classic. Each opens in this tab; use Back to return. Latency is simulated: <a href="?latency=0">turn it off</a> · <a href="?latency=1">default</a> · <a href="?latency=4">slow</a>.</p>
        </div>
      </header>
      <div className={s.groups}>
        {groups.map(g => (
          <section key={g} className={s.group}>
            <h2 className={s.groupTitle}>{g}<span>{list.filter(x => x.group === g).length}</span></h2>
            <ul>
              {list.filter(x => x.group === g).map(x => (
                <li key={x.id}><a href={x.path} className={s.link}><span>{x.label}</span><code>{x.path.replace(/%7B.*$/, '…')}</code></a></li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </main>
  );
}
