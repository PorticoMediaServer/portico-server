import {useEffect, useState} from 'react';
import screens from './screens.json';
import {BrandSymbol} from '../../ui/Media';
import {SearchInput} from '../../ui/Controls';
import {Icon} from '../../ui/Icon';
import {resetTint} from '../../app/tint';

type Group = {group: string; items: {id: string; title: string; path: string}[]};

/** Every screen and state of this build, one link each, for review. */
export function ScreensIndex() {
  const [q, setQ] = useState('');
  useEffect(() => { document.title = 'Screen index · Portico'; resetTint(); }, []);
  const groups = (screens as Group[]).map(g => ({...g, items: g.items.filter(i => !q || `${i.title} ${i.path}`.toLowerCase().includes(q.toLowerCase()))})).filter(g => g.items.length);
  const total = (screens as Group[]).reduce((a, g) => a + g.items.length, 0);
  return (
    <div className="screens">
      <header className="screens-head">
        <BrandSymbol size={36} />
        <div><h1 className="display">Screen index</h1><p className="muted">{total} screens and states of Portico Web Cinema. Links open with simulated delay off; add <code>?latency=1</code> to see loading states. Sign-in screens leave you signed in.</p></div>
        <a className="btn btn-primary btn-md" href="/"><Icon name="home" size={18} /><span className="btn-label">Open the app</span></a>
      </header>
      <SearchInput label="Filter screens" placeholder="Filter screens" value={q} onChange={setQ} className="screens-find" />
      <div className="screens-groups">
        {groups.map(g => (
          <section key={g.group} className="screens-group">
            <h2 className="section-title">{g.group} <span className="faint">{g.items.length}</span></h2>
            <ul>
              {g.items.map(i => (
                <li key={i.id}><a href={i.path + (i.path.includes('latency=') ? '' : (i.path.includes('?') ? '&' : '?') + 'latency=0')}><span>{i.title}</span><code>{i.path}</code></a></li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
