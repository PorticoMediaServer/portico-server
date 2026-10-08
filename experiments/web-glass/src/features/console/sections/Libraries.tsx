import { useState } from 'react';
import { FolderOpen, Plus, RefreshCw, Trash2, Wand2 } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { LibraryAdmin } from '../../../services/api';
import { useLibrariesAdmin, useTasks } from '../../../services/queries';
import { api } from '../../../services/client';
import { Button, ErrorState, Skel, Switch } from '../../../components/ui';
import { Confirm, Sheet, toast } from '../../../components/overlay';
import { ago, fmtBytes } from '../../../lib/hooks';
import { Bar, Dot, Panel, SettingRow, Table } from '../ui';

export function Libraries() {
  const q = useLibrariesAdmin();
  const tasks = useTasks();
  const qc = useQueryClient();
  const [open, setOpen] = useState<LibraryAdmin | null>(null);
  const [del, setDel] = useState(false);
  const scanTask = tasks.data?.find((t) => t.id === 't1');
  const scan = async (l?: LibraryAdmin) => {
    await api.scanLibrary(l?.id ?? 'all');
    qc.invalidateQueries({ queryKey: ['admin'] });
    toast(l ? `Scanning ${l.name}` : 'Scanning all libraries', <RefreshCw />);
  };
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  const scanning = q.data?.find((l) => l.scan);
  return (
    <div className="cstack">
      <div className="ctoolbar">
        <Button size="sm" variant="accent" icon={<Plus />} onClick={() => toast('Choose a library type and folders to add')}>
          Add Library
        </Button>
        <Button size="sm" variant="plain" icon={<RefreshCw />} onClick={() => scan()}>
          Scan All
        </Button>
        <span className="t-caption">Scans run every 6 hours and whenever files change.</span>
      </div>
      {scanning?.scan && (
        <div className="cbanner info">
          <RefreshCw className="spin" />
          <div style={{ flex: 1 }}>
            <strong>Scanning {scanning.name}</strong> · {scanning.scan.phase} · {scanning.scan.eta} left
            <div className="t-mono ellipsis">{scanning.scan.current}</div>
            <Bar value={scanTask?.progress ?? scanning.scan.progress} />
          </div>
        </div>
      )}
      <Panel pad={false}>
        {!q.data ? (
          <Skel h={300} />
        ) : (
          <Table
            rows={q.data}
            getKey={(l) => l.id}
            onOpen={setOpen}
            cols={[
              { key: 'name', label: 'Library', sort: (l) => l.name, render: (l) => <b>{l.name}</b>, width: 'minmax(140px,1.2fr)' },
              { key: 'type', label: 'Type', sort: (l) => l.kind, render: (l) => <span className="ctag">{l.kind}</span>, width: '110px' },
              { key: 'items', label: 'Items', align: 'right', sort: (l) => l.paths.reduce((a, p) => a + p.items, 0), render: (l) => <span className="tnum">{l.paths.reduce((a, p) => a + p.items, 0).toLocaleString('en-US')}</span>, width: '80px' },
              { key: 'size', label: 'Size', align: 'right', sort: (l) => l.paths.reduce((a, p) => a + p.sizeBytes, 0), render: (l) => <span className="tnum">{fmtBytes(l.paths.reduce((a, p) => a + p.sizeBytes, 0))}</span>, width: '90px' },
              { key: 'paths', label: 'Folders', render: (l) => <span className="t-mono ellipsis">{l.paths.map((p) => p.path).join(', ')}</span>, width: 'minmax(160px,1.6fr)' },
              { key: 'agent', label: 'Agent', render: (l) => <span className="ellipsis">{l.agent}</span>, width: 'minmax(120px,1fr)' },
              { key: 'scan', label: 'Last scan', sort: (l) => l.lastScan, render: (l) => (l.scan ? <span className="c-live"><Dot tone="live" /> Scanning</span> : <span className="tnum">{ago(l.lastScan)} · {l.lastScanDuration}</span>), width: '150px' },
              { key: 'issues', label: 'Issues', align: 'right', sort: (l) => l.issues, render: (l) => (l.issues ? <span className="ctag warn">{l.issues} unmatched</span> : <Dot tone="ok" />), width: '110px' },
            ]}
          />
        )}
      </Panel>
      {open && (
        <Sheet
          open
          onClose={() => setOpen(null)}
          title={open.name}
          size="lg"
          footer={
            <>
              <Button variant="danger" size="sm" icon={<Trash2 />} onClick={() => setDel(true)}>
                Remove Library
              </Button>
              <span style={{ flex: 1 }} />
              <Button variant="plain" size="sm" icon={<Wand2 />} onClick={() => toast('Refreshing all metadata in the background')}>
                Refresh Metadata
              </Button>
              <Button variant="accent" size="sm" icon={<RefreshCw />} onClick={() => (scan(open), setOpen(null))}>
                Scan Now
              </Button>
            </>
          }
        >
          <div className="cstack">
            <Panel title="Folders" action={<Button size="sm" variant="plain" icon={<FolderOpen />}>Add Folder</Button>}>
              {open.paths.map((p) => (
                <div key={p.path} className="folder-row">
                  <Dot tone={p.online ? 'ok' : 'err'} />
                  <code>{p.path}</code>
                  <span className="t-sub tnum">
                    {p.items.toLocaleString('en-US')} items · {fmtBytes(p.sizeBytes)}
                  </span>
                </div>
              ))}
            </Panel>
            <Panel title="Scanning & metadata">
              <SettingRow title="Metadata agent" sub="Where descriptions, artwork and cast come from">
                <select className="input sm" defaultValue={open.agent}>
                  <option>{open.agent}</option>
                  <option>Local NFO files only</option>
                </select>
              </SettingRow>
              <SettingRow title="Language" sub="Titles and descriptions">
                <span className="t-sub">{open.language}</span>
              </SettingRow>
              <SettingRow title="Watch folders for changes" sub="New files appear within seconds">
                <Switch checked={open.watchFolders} onChange={() => toast('Saved')} label="Watch folders" size="sm" accent />
              </SettingRow>
              {Object.entries(open.options).map(([k, v]) => (
                <SettingRow key={k} title={k}>
                  <Switch checked={!!v} onChange={() => toast('Saved')} label={k} size="sm" accent />
                </SettingRow>
              ))}
            </Panel>
          </div>
        </Sheet>
      )}
      <Confirm open={del} onClose={() => setDel(false)} danger title={`Remove ${open?.name}?`} body="Files on disk are not deleted. Watch history is kept for 30 days in case you add the library back." confirmLabel="Remove Library" onConfirm={() => (toast('Library removed'), setOpen(null))} />
    </div>
  );
}
