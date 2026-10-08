import { useState } from 'react';
import { ArrowDown, ArrowUp, Copy, GripVertical, ImageOff, RefreshCw, Wand2 } from 'lucide-react';
import { metadataSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { Button, ErrorState, Skel, Switch } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { ago } from '../../../lib/hooks';
import { Panel } from '../ui';

type M = typeof metadataSeed;

export function Metadata() {
  const q = useAdminSection<M>('metadata');
  const [agents, setAgents] = useState<M['agents'] | null>(null);
  const [resolved, setResolved] = useState<string[]>([]);
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const list = agents ?? q.data.agents;
  const move = (i: number, d: number) => {
    const x = [...list];
    [x[i], x[i + d]] = [x[i + d], x[i]];
    setAgents(x);
    toast('Agent priority saved');
  };
  const unmatched = q.data.unmatched.filter((u) => !resolved.includes(u.id));
  return (
    <div className="cstack">
      <div className="cstats slim">
        <div className="cstat"><div className="cstat-label">Unmatched</div><div className="cstat-value tnum">{unmatched.length}</div></div>
        <div className="cstat"><div className="cstat-label">Duplicates</div><div className="cstat-value tnum">{q.data.duplicates.length}</div></div>
        <div className="cstat"><div className="cstat-label">Missing artwork</div><div className="cstat-value tnum">{q.data.missingArt}</div></div>
        <div className="cstat"><div className="cstat-label">Last full refresh</div><div className="cstat-value" style={{ fontSize: 18 }}>{ago(q.data.lastRefresh)}</div></div>
      </div>
      <div className="cgrid two">
        <Panel title="Agents" sub="Tried top to bottom. Local files always win for locked fields.">
          <ol className="agents">
            {list.map((a, i) => (
              <li key={a.id}>
                <GripVertical className="grip" />
                <span className="tnum agent-n">{i + 1}</span>
                <span className="agent-name">{a.name}</span>
                <button className="icon-btn sm plain" aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)}>
                  <ArrowUp />
                </button>
                <button className="icon-btn sm plain" aria-label="Move down" disabled={i === list.length - 1} onClick={() => move(i, 1)}>
                  <ArrowDown />
                </button>
                <Switch size="sm" accent checked={a.enabled} onChange={() => toast(`${a.enabled ? 'Disabled' : 'Enabled'} ${a.name}`)} label={a.name} />
              </li>
            ))}
          </ol>
        </Panel>
        <Panel title="Duplicates" sub="Same title, several files. Keep both as versions or remove one.">
          {q.data.duplicates.map((d) => (
            <div key={d.id} className="dup">
              <div className="dup-title">
                <Copy /> {d.title}
              </div>
              {d.files.map((f) => (
                <div key={f} className="t-mono dup-file">{f}</div>
              ))}
              <div className="dup-actions">
                <Button size="sm" variant="plain" onClick={() => toast('Kept as two versions of one item')}>Merge as Versions</Button>
                <Button size="sm" variant="ghost" onClick={() => toast('Smaller file moved to trash')}>Remove Smaller</Button>
              </div>
            </div>
          ))}
        </Panel>
      </div>
      <Panel title="Needs attention" sub="Files Portico couldn’t confidently identify" action={<Button size="sm" variant="plain" icon={<RefreshCw />} onClick={() => toast('Retrying matches for 5 items')}>Retry All</Button>}>
        {unmatched.length === 0 && <div className="ctable-empty">Everything is matched. Nice.</div>}
        {unmatched.map((u) => (
          <div key={u.id} className="unmatched">
            <div className="um-main">
              <div className="um-guess">
                <span className="ctag">{u.library}</span> {u.guess}
              </div>
              <div className="t-mono ellipsis">{u.path}</div>
            </div>
            {u.candidates.length > 0 ? (
              <select className="input sm um-pick" defaultValue={u.candidates[0]} aria-label="Match candidates">
                {u.candidates.map((c) => (
                  <option key={c}>{c}</option>
                ))}
              </select>
            ) : (
              <span className="t-sub um-pick">No candidates · search manually</span>
            )}
            <Button size="sm" variant="tinted" icon={<Wand2 />} onClick={() => (setResolved([...resolved, u.id]), toast('Matched', <Wand2 />))}>
              {u.candidates.length ? 'Match' : 'Search'}
            </Button>
          </div>
        ))}
      </Panel>
      <Panel title="Artwork" sub={`${q.data.missingArt} items have no poster`} action={<Button size="sm" variant="plain" icon={<ImageOff />} onClick={() => toast('Fetching missing artwork from all agents')}>Fetch Missing Artwork</Button>}>
        <p className="t-sub">Portico stores artwork next to your media as poster.jpg / fanart.jpg when “Save artwork to media folders” is on, so another server can reuse it.</p>
      </Panel>
    </div>
  );
}
