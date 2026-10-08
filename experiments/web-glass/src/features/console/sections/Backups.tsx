import { useState } from 'react';
import { CheckCircle2, CloudUpload, HardDrive, History, Lock, RotateCcw, XCircle } from 'lucide-react';
import { backupsSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { api } from '../../../services/client';
import { Button, ErrorState, Skel } from '../../../components/ui';
import { Confirm, toast } from '../../../components/overlay';
import { ago, fmtBytes } from '../../../lib/hooks';
import { KV, Panel, Table } from '../ui';

type B = typeof backupsSeed;

export function Backups() {
  const q = useAdminSection<B>('backups');
  const [running, setRunning] = useState(false);
  const [restore, setRestore] = useState<string | null>(null);
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const d = q.data;
  return (
    <div className="cstack">
      <div className="ctoolbar">
        <Button
          size="sm"
          variant="accent"
          icon={<CloudUpload />}
          loading={running}
          onClick={async () => {
            setRunning(true);
            await api.runBackup();
            setRunning(false);
            toast('Backup complete · 1.8 GB to 2 destinations', <CheckCircle2 />);
          }}
        >
          {running ? 'Backing Up…' : 'Back Up Now'}
        </Button>
        <span className="t-caption">
          {d.schedule} · keeps {d.retention}
        </span>
      </div>
      <div className="cgrid two">
        {d.destinations.map((x) => (
          <Panel key={x.id} title={x.name} sub={x.kind} action={x.encrypted ? <span className="ctag ok"><Lock /> Encrypted</span> : <span className="ctag">Not encrypted</span>}>
            <div className="dest">
              {x.kind.startsWith('Cloud') ? <CloudUpload className="dest-ic" /> : <HardDrive className="dest-ic" />}
              <KV rows={[['Target', <code className="t-mono">{x.target}</code>], ['Last success', ago(x.lastOk)], ['Space used', fmtBytes(x.usedBytes)]]} />
            </div>
          </Panel>
        ))}
      </div>
      <div className="cgrid two">
        <Panel title="What’s included">
          <ul className="inc">
            {d.includes.map((i) => (
              <li key={i}>
                <CheckCircle2 className="c-ok" /> {i}
              </li>
            ))}
            {d.excludes.map((i) => (
              <li key={i} className="t-sub">
                <XCircle /> {i}
              </li>
            ))}
          </ul>
        </Panel>
        <Panel title="Restore" sub="Roll the server back to any backup. Media files are never touched.">
          <p className="t-sub" style={{ marginBottom: 12 }}>
            Restoring replaces the database, settings, users and watch history with the chosen snapshot. Harbor restarts afterwards (about a minute).
          </p>
          <Button size="sm" variant="plain" icon={<History />} onClick={() => setRestore(d.history[0].id)}>
            Restore Latest…
          </Button>
        </Panel>
      </div>
      <Panel title="History" pad={false}>
        <Table
          rows={d.history}
          getKey={(h) => h.id}
          dense
          cols={[
            { key: 'at', label: 'When', sort: (h) => h.at, render: (h) => <span className="tnum">{new Date(h.at).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}</span>, width: '170px' },
            { key: 'dest', label: 'Destination', render: (h) => h.destination, width: 'minmax(140px,1fr)' },
            { key: 'size', label: 'Size', align: 'right', render: (h) => <span className="tnum">{fmtBytes(h.sizeBytes)}</span>, width: '90px' },
            { key: 'dur', label: 'Duration', align: 'right', render: (h) => <span className="tnum">{h.duration}</span>, width: '110px' },
            { key: 'status', label: 'Result', render: (h) => (h.status === 'ok' ? <span className="c-ok">Succeeded</span> : <span className="c-err">Failed · {h.note}</span>), width: 'minmax(160px,1.4fr)' },
            { key: 'act', label: '', render: (h) => (h.status === 'ok' ? <button className="c-link" onClick={(e) => (e.stopPropagation(), setRestore(h.id))}><RotateCcw className="inline-ic" />Restore</button> : null), width: '90px' },
          ]}
        />
      </Panel>
      <Confirm open={!!restore} onClose={() => setRestore(null)} danger title="Restore this backup?" body="Everyone will be signed out while Harbor restarts. Changes made since this backup will be lost." confirmLabel="Restore and Restart" onConfirm={() => toast('Restoring… Harbor will be back in about a minute', <RotateCcw />)} />
    </div>
  );
}
