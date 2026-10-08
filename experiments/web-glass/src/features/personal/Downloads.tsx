import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { AlertTriangle, Download, HardDrive, Pause, Play, RefreshCw, Settings, Sparkles, Trash2, WifiOff } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { Download as D } from '../../data/types';
import { useDownloads } from '../../services/queries';
import { useUi } from '../../state/ui';
import { api } from '../../services/client';
import { PageHeader } from '../../components/PageHeader';
import { Button, EmptyState, ErrorState, IconButton, Progress, Skel, Switch, cls } from '../../components/ui';
import { toast } from '../../components/overlay';
import { ago, fmtBytes } from '../../lib/hooks';

export function DownloadsPage() {
  const q = useDownloads();
  const qc = useQueryClient();
  const offline = useUi((s) => s.offline);
  const setOffline = useUi((s) => s.setOffline);
  const [tick, setTick] = useState(0);
  // Animate active downloads so the page feels alive.
  useEffect(() => {
    const t = setInterval(() => setTick((x) => x + 1), 1000);
    return () => clearInterval(t);
  }, []);
  const list = (q.data ?? []).map((d) => (d.status === 'downloading' ? { ...d, progress: Math.min(0.99, d.progress + tick * 0.004) } : d));
  const used = list.filter((d) => d.status === 'complete').reduce((a, d) => a + d.sizeBytes, 0);
  const remove = async (d: D) => {
    await api.removeDownload(d.id);
    qc.invalidateQueries({ queryKey: ['downloads'] });
    toast(`Removed ${d.title}`, <Trash2 />);
  };
  const groups: [string, D[]][] = [
    ...(offline ? [] : ([['Downloading', list.filter((d) => ['downloading', 'queued', 'paused', 'error'].includes(d.status))]] as [string, D[]][])),
    ['Ready to watch offline', list.filter((d) => d.status === 'complete')],
    ...(offline ? [] : ([['Expired', list.filter((d) => d.status === 'expired')]] as [string, D[]][])),
  ];
  return (
    <div className="page">
      <PageHeader title="Downloads" subtitle="Take Harbor with you. Downloads play without a connection." actions={<Button variant="plain" icon={<Settings />} to="/settings/downloads">Download Settings</Button>} />
      <div className="page-body pad downloads">
        <div className="dl-summary">
          <div className="dl-storage">
            <HardDrive />
            <div>
              <div className="t-headline tnum">{fmtBytes(used)} used by Portico</div>
              <div className="meter" style={{ width: 260, marginTop: 8 }}>
                <i style={{ width: `${(used / 64e9) * 100}%`, background: 'var(--blue-400)' }} />
                <i style={{ width: '38%', background: 'rgba(255,255,255,.18)' }} />
              </div>
              <div className="t-sub tnum" style={{ marginTop: 6 }}>
                41.2 GB free on this device
              </div>
            </div>
          </div>
          <div className="dl-offline">
            <WifiOff />
            <div>
              <div className="t-headline">Offline mode</div>
              <div className="t-sub">Only show what’s downloaded</div>
            </div>
            <Switch checked={offline} onChange={(v) => (setOffline(v), toast(v ? 'Offline mode on: showing downloads only' : 'Back online with Harbor', <WifiOff />))} label="Offline mode" />
          </div>
        </div>
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} />}
        {!q.data && !q.isError && <Skel h={240} />}
        {q.data?.length === 0 && <EmptyState icon={<Download />} title="No downloads yet" body="Download movies, episodes, albums and audiobooks to watch or listen without a connection." actions={<Button variant="accent" to="/">Find Something</Button>} />}
        {groups.map(([label, items]) =>
          items.length === 0 ? null : (
            <section key={label} className="dl-group">
              <h2 className="group-title">{label}</h2>
              <div className="group">
                {items.map((d) => (
                  <div key={d.id} className={cls('row dl-row', d.status)}>
                    <img className={cls('dl-img', (d.itemKind === 'album' || d.itemKind === 'audiobook') && 'sq')} src={d.image} alt="" loading="lazy" />
                    <div className="row-main">
                      <div className="row-title ellipsis">
                        {d.title}
                        {d.smart && (
                          <span className="badge new" style={{ marginLeft: 8 }}>
                            <Sparkles className="inline-ic" style={{ width: 11, height: 11 }} />
                            Smart
                          </span>
                        )}
                      </div>
                      <div className="row-sub ellipsis">
                        {d.subtitle} · {d.quality} · {fmtBytes(d.sizeBytes)} · {d.device}
                      </div>
                      {d.status === 'downloading' && (
                        <div className="dl-prog">
                          <Progress value={d.progress} accent />
                          <span className="tnum">
                            {Math.round(d.progress * 100)}% · {fmtBytes(d.sizeBytes * d.progress)} of {fmtBytes(d.sizeBytes)} · 28 MB/s
                          </span>
                        </div>
                      )}
                      {d.status === 'queued' && <div className="dl-note">Waiting for Wi-Fi on {d.device}</div>}
                      {d.error && (
                        <div className={cls('dl-note', d.status === 'error' ? 'err' : 'warn')}>
                          <AlertTriangle /> {d.error}
                        </div>
                      )}
                      {d.status === 'complete' && d.expiresAt && <div className="dl-note">Available offline · renews automatically · expires {ago(d.expiresAt)}</div>}
                    </div>
                    <div className="dl-actions">
                      {d.status === 'complete' && (
                        <IconButton label={`Play ${d.title}`} variant="plain" to={d.itemKind === 'album' ? `/album/${d.itemId}` : d.itemKind === 'audiobook' ? `/audiobook/${d.itemId}` : `/watch/${d.itemId}`}>
                          <Play />
                        </IconButton>
                      )}
                      {d.status === 'downloading' && (
                        <IconButton label="Pause" variant="plain" onClick={() => toast('Download paused', <Pause />)}>
                          <Pause />
                        </IconButton>
                      )}
                      {(d.status === 'error' || d.status === 'expired') && (
                        <IconButton label="Retry" variant="plain" onClick={() => api.startDownload(d.itemId, d.quality).then(() => (qc.invalidateQueries({ queryKey: ['downloads'] }), toast(`Downloading ${d.title} again`, <RefreshCw />)))}>
                          <RefreshCw />
                        </IconButton>
                      )}
                      <IconButton label={`Remove ${d.title}`} variant="plain" onClick={() => remove(d)}>
                        <Trash2 />
                      </IconButton>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ),
        )}
        <p className="group-foot">
          Smart downloads keep the next episodes of shows you’re watching on your devices. Manage it in <Link to="/settings/downloads" className="linkish">Settings</Link>.
        </p>
      </div>
    </div>
  );
}
