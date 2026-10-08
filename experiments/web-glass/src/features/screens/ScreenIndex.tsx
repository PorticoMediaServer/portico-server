import { ExternalLink } from 'lucide-react';
import { SCREENS, screenUrl } from './screens';

export function ScreenIndex() {
  const groups = Array.from(new Set(SCREENS.map((s) => s.group)));
  return (
    <div className="screens">
      <header className="screens-head">
        <img src="/brand/portico-wordmark-mono-white.svg" alt="Portico" />
        <h1>Screen index</h1>
        <p>
          Every screen and state in the web app. Links sign you in as the listed profile (<code>?as=</code>) and reset simulated network conditions. Use <code>?latency=fast|normal|slow|hang</code> and <code>?fail=all|methodName</code> on any page. Screenshots of all of these at four sizes come from <code>npm run shots</code>.
        </p>
      </header>
      {groups.map((g) => (
        <section key={g} className="screens-group">
          <h2>{g}</h2>
          <div className="screens-grid">
            {SCREENS.filter((s) => s.group === g).map((s) => (
              <a key={s.name + s.path} href={screenUrl(s)} className="screen-link">
                <span className="screen-name">{s.name}</span>
                <span className="screen-path">
                  {s.path}
                  {s.as && s.as !== 'none' && s.as !== 'signed-out' ? ` · as ${s.as}` : s.as === 'signed-out' ? ' · signed out' : ''}
                </span>
                {s.note && <span className="screen-note">{s.note}</span>}
                <ExternalLink className="screen-ic" />
              </a>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
