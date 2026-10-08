import { useEffect, useState } from 'react';
import { Play } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useTasks } from '../../../services/queries';
import { api } from '../../../services/client';
import { Button, ErrorState, Skel, cls } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { ago } from '../../../lib/hooks';
import { Bar, Dot, Panel, Table } from '../ui';

export function Tasks() {
  const q = useTasks();
  const qc = useQueryClient();
  const [cat, setCat] = useState('All');
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setTick((x) => x + 1), 1000);
    return () => clearInterval(t);
  }, []);
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const cats = ['All', ...Array.from(new Set(q.data.map((t) => t.category)))];
  const rows = q.data.filter((t) => cat === 'All' || t.category === cat);
  return (
    <div className="cstack">
      <div className="chip-scroll no-scrollbar">
        {cats.map((c) => (
          <button key={c} className={cls('chip', cat === c && 'on')} onClick={() => setCat(c)}>
            {c}
          </button>
        ))}
      </div>
      <Panel pad={false}>
        <Table
          rows={rows}
          getKey={(t) => t.id}
          cols={[
            { key: 'name', label: 'Task', sort: (t) => t.name, width: 'minmax(220px,1.6fr)', render: (t) => (
              <span className="task-cell">
                <b>{t.name}</b>
                <span className="t-sub ellipsis">{t.description}</span>
              </span>
            ) },
            { key: 'sched', label: 'Schedule', render: (t) => <span className="ellipsis">{t.schedule}</span>, width: 'minmax(140px,1fr)' },
            { key: 'last', label: 'Last run', sort: (t) => t.lastRun, render: (t) => <span className="tnum">{ago(t.lastRun)} · {t.lastDuration}</span>, width: '170px' },
            { key: 'next', label: 'Next', render: (t) => <span className="tnum">{t.nextRun.includes('T') ? ago(t.nextRun) : t.nextRun}</span>, width: '120px' },
            { key: 'status', label: 'Status', sort: (t) => t.status, width: '170px', render: (t) =>
              t.status === 'running' ? (
                <span className="use-cell">
                  <Bar value={Math.min(0.99, (t.progress ?? 0) + tick * 0.006)} />
                  <span className="tnum">{Math.round(Math.min(0.99, (t.progress ?? 0) + tick * 0.006) * 100)}%</span>
                </span>
              ) : t.status === 'failed' ? (
                <span className="c-err"><Dot tone="err" /> Failed · rate limited</span>
              ) : t.status === 'disabled' ? (
                <span className="t-sub">Disabled</span>
              ) : (
                <span className="c-ok"><Dot tone="ok" /> Idle</span>
              ) },
            { key: 'run', label: '', width: '90px', align: 'right', render: (t) => (
              <Button
                size="sm"
                variant="ghost"
                icon={<Play />}
                disabled={t.status === 'running' || t.status === 'disabled'}
                onClick={async (e) => {
                  e.stopPropagation();
                  await api.runTask(t.id);
                  qc.invalidateQueries({ queryKey: ['admin', 'tasks'] });
                  toast(`Started “${t.name}”`, <Play />);
                }}
              >
                Run
              </Button>
            ) },
          ]}
        />
      </Panel>
    </div>
  );
}
