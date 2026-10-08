import { useState } from 'react';
import { useActivity } from '../../../services/queries';
import { ErrorState, Skel, cls } from '../../../components/ui';
import { Dot } from '../ui';

const KINDS = ['all', 'playback', 'library', 'user', 'security', 'dvr', 'backup', 'system', 'warning'] as const;

export function ActivityFeed() {
  const q = useActivity();
  const [k, setK] = useState<(typeof KINDS)[number]>('all');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  const list = (q.data ?? []).filter((a) => k === 'all' || a.kind === k);
  return (
    <div className="cstack">
      <div className="chip-scroll no-scrollbar">
        {KINDS.map((x) => (
          <button key={x} className={cls('chip', k === x && 'on')} onClick={() => setK(x)}>
            {x === 'all' ? 'Everything' : x === 'dvr' ? 'DVR' : x[0].toUpperCase() + x.slice(1)}
          </button>
        ))}
      </div>
      <div className="cpanel">
        <ol className="timeline">
          {!q.data && Array.from({ length: 8 }, (_, i) => <Skel key={i} h={40} style={{ margin: 12 }} />)}
          {list.map((a) => (
            <li key={a.id}>
              <time className="tnum">
                {new Date(a.at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                <span>{new Date(a.at).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}</span>
              </time>
              <Dot tone={a.kind === 'warning' ? 'warn' : a.kind === 'security' ? 'info' : a.kind === 'playback' ? 'live' : 'ok'} />
              <div>
                <div>
                  <b>{a.actor}</b> {a.text}
                </div>
                {a.detail && <div className="t-sub">{a.detail}</div>}
              </div>
              <span className="ctag">{a.kind}</span>
            </li>
          ))}
        </ol>
      </div>
    </div>
  );
}
