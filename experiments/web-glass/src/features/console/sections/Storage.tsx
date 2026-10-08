import { AlertTriangle, Trash2 } from 'lucide-react';
import { storageSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { Button, ErrorState, Skel, cls } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { fmtBytes } from '../../../lib/hooks';
import { Bar, Dot, Panel, Table } from '../ui';

type S = typeof storageSeed;

export function Storage() {
  const q = useAdminSection<S>('storage');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const d = q.data;
  const media = d.volumes[0];
  const bd = d.breakdown.reduce((a, b) => a + b.bytes, 0);
  return (
    <div className="cstack">
      <div className="cbanner warn">
        <AlertTriangle />
        <div>
          <strong>Drive sdc (bay 3) is wearing.</strong> 8 reallocated sectors appeared in the last 72 hours. The pool is still healthy with two-drive redundancy; order a replacement and swap it at your convenience.
        </div>
      </div>
      <Panel title="Media pool" sub={`${media.fs} · ${media.mount}`}>
        <div className="pool">
          <div className="pool-bar">
            {d.breakdown.map((b) => (
              <i key={b.label} style={{ width: `${(b.bytes / media.size) * 100}%`, background: b.color }} title={`${b.label} ${fmtBytes(b.bytes)}`} />
            ))}
          </div>
          <div className="pool-legend">
            {d.breakdown.map((b) => (
              <span key={b.label}>
                <i style={{ background: b.color }} />
                {b.label} <b className="tnum">{fmtBytes(b.bytes)}</b>
              </span>
            ))}
            <span>
              <i style={{ background: 'rgba(255,255,255,.12)' }} />
              Free <b className="tnum">{fmtBytes(media.size - bd)}</b>
            </span>
          </div>
        </div>
      </Panel>
      <Panel title="Volumes" pad={false}>
        <Table
          rows={d.volumes}
          getKey={(v) => v.id}
          cols={[
            { key: 'name', label: 'Volume', sort: (v) => v.name, render: (v) => <b>{v.name}</b>, width: 'minmax(140px,1fr)' },
            { key: 'mount', label: 'Mount', render: (v) => <span className="t-mono">{v.mount}</span>, width: '150px' },
            { key: 'fs', label: 'Filesystem', render: (v) => <span className="ellipsis">{v.fs}</span>, width: 'minmax(160px,1.2fr)' },
            { key: 'use', label: 'Used', sort: (v) => v.used / v.size, render: (v) => (
              <span className="use-cell">
                <Bar value={v.used / v.size} tone={v.used / v.size > 0.85 ? 'err' : v.used / v.size > 0.7 ? 'warn' : undefined} />
                <span className="tnum">{fmtBytes(v.used)} / {fmtBytes(v.size)}</span>
              </span>
            ), width: 'minmax(220px,1.4fr)' },
            { key: 'io', label: 'I/O', align: 'right', render: (v) => <span className="tnum">↓{v.readMBs} ↑{v.writeMBs} MB/s</span>, width: '130px' },
            { key: 'health', label: 'Health', render: (v) => <span className={cls(v.health === 'Healthy' ? 'c-ok' : 'c-warn')}><Dot tone={v.health === 'Healthy' ? 'ok' : 'warn'} /> {v.health}</span>, width: '140px' },
          ]}
        />
      </Panel>
      <Panel title="Drives (SMART)" pad={false}>
        <Table
          rows={d.drives}
          getKey={(x) => x.dev}
          dense
          cols={[
            { key: 'bay', label: 'Bay', sort: (x) => x.bay, render: (x) => <span className="tnum">{x.bay || 'M.2'}</span>, width: '60px' },
            { key: 'dev', label: 'Device', render: (x) => <span className="t-mono">{x.dev}</span>, width: '90px' },
            { key: 'model', label: 'Model', render: (x) => x.model, width: 'minmax(160px,1fr)' },
            { key: 'serial', label: 'Serial', render: (x) => <span className="t-mono">{x.serial}</span>, width: '130px' },
            { key: 'temp', label: 'Temp', align: 'right', sort: (x) => x.temp, render: (x) => <span className="tnum">{x.temp} °C</span>, width: '70px' },
            { key: 'hours', label: 'Power-on', align: 'right', sort: (x) => x.hours, render: (x) => <span className="tnum">{(x.hours / 24 / 365).toFixed(1)} yr</span>, width: '90px' },
            { key: 'realloc', label: 'Realloc.', align: 'right', sort: (x) => x.realloc, render: (x) => <span className={cls('tnum', x.realloc && 'c-warn')}>{x.realloc}</span>, width: '80px' },
            { key: 'status', label: 'Status', render: (x) => <span className={x.status === 'OK' ? 'c-ok' : 'c-warn'}><Dot tone={x.status === 'OK' ? 'ok' : 'warn'} /> {x.status}</span>, width: '100px' },
          ]}
        />
      </Panel>
      <Panel title="Caches">
        {d.caches.map((c) => (
          <div key={c.label} className="cache-row">
            <span>{c.label}</span>
            <b className="tnum">{fmtBytes(c.bytes)}</b>
            <Button size="sm" variant="ghost" icon={<Trash2 />} onClick={() => toast(`Cleared ${c.label.toLowerCase()} (${fmtBytes(c.bytes)})`)}>
              Clear
            </Button>
          </div>
        ))}
      </Panel>
    </div>
  );
}
