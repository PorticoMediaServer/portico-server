import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { ArrowRight, Download, Sparkles } from 'lucide-react';
import { useAdminOverview, useSessions } from '../../../services/queries';
import { metricAt, type MetricPoint } from '../../../data/fixtures/server';
import { Button, ErrorState, Skel } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { ago, fmtBytes } from '../../../lib/hooks';
import { Dot, Panel, Stat, TimeChart } from '../ui';
import { SessionCard } from './Sessions';

function useLiveMetrics(seed?: MetricPoint[]) {
  const [pts, setPts] = useState<MetricPoint[]>([]);
  useEffect(() => {
    if (seed) setPts(seed);
  }, [seed]);
  useEffect(() => {
    const t = setInterval(() => setPts((p) => (p.length ? [...p.slice(1), metricAt(Date.now())] : p)), 3000);
    return () => clearInterval(t);
  }, []);
  return pts;
}

export function Overview({ compact }: { compact?: boolean }) {
  const q = useAdminOverview();
  const sessions = useSessions();
  const pts = useLiveMetrics(q.data?.metrics);
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data || !pts.length)
    return (
      <div className="cgrid">
        {Array.from({ length: 6 }, (_, i) => (
          <Skel key={i} h={110} r={16} />
        ))}
      </div>
    );
  const d = q.data;
  const last = pts[pts.length - 1];
  const live = sessions.data ?? d.sessions;
  const transcodes = live.filter((s) => s.decision === 'Transcode').length;
  const upDays = Math.floor((Date.now() - Date.parse(d.server.startedAt)) / 86_400_000);
  const stats = (
    <div className="cstats">
      <Stat label="Streams" value={live.length} sub={`${transcodes} transcoding · ${live.filter((s) => !s.local).length} remote`} spark={pts.map((p) => p.streams)} />
      <Stat label="CPU" value={last.cpu.toFixed(0)} unit="%" sub="i5-13500 · 20 threads" spark={pts.map((p) => p.cpu)} />
      <Stat label="GPU (Quick Sync)" value={last.gpu.toFixed(0)} unit="%" sub={`${transcodes} hardware sessions`} spark={pts.map((p) => p.gpu)} color="#a193ff" />
      <Stat label="Memory" value={last.mem.toFixed(1)} unit=" GB" sub="of 64 GB" spark={pts.map((p) => p.mem)} color="#3fd89a" />
      <Stat label="Upload" value={last.netOut.toFixed(0)} unit=" Mbps" sub="of 300 Mbps" spark={pts.map((p) => p.netOut)} color="#ffbd4a" />
      <Stat label="Storage" value={fmtBytes(d.storage.used).split(' ')[0]} unit={` ${fmtBytes(d.storage.used).split(' ')[1]}`} sub={`of ${fmtBytes(d.storage.size)} · 1 drive warning`} tone="warn" />
    </div>
  );
  if (compact) return <div className="cm-compact">{stats}</div>;
  return (
    <div className="cstack">
      <div className="cbanner">
        <Sparkles />
        <div>
          <strong>Portico Server {d.server.latestVersion} is available.</strong> {d.server.releaseNotes[0]}, and {d.server.releaseNotes.length - 1} more fixes.
        </div>
        <Button size="sm" variant="accent" icon={<Download />} onClick={() => toast('Update scheduled for 04:00 when nothing is playing', <Download />)}>
          Update Tonight
        </Button>
      </div>
      {stats}
      <div className="cgrid two">
        <Panel title="Load" sub="Last hour · live" action={<span className="c-live"><Dot tone="live" /> Live</span>}>
          <TimeChart times={pts.map((p) => p.t)} unit="%" series={[{ name: 'CPU', color: '#5cb3e2', values: pts.map((p) => p.cpu) }, { name: 'GPU', color: '#a193ff', values: pts.map((p) => p.gpu) }]} />
        </Panel>
        <Panel title="Network" sub="Last hour · Mbps">
          <TimeChart times={pts.map((p) => p.t)} unit=" Mbps" series={[{ name: 'Out', color: '#ffbd4a', values: pts.map((p) => p.netOut) }, { name: 'In', color: '#3fd89a', values: pts.map((p) => p.netIn) }]} />
        </Panel>
      </div>
      <Panel title="Now Playing" sub={`${live.length} sessions`} action={<Link to="/console/sessions" className="c-link">All sessions <ArrowRight /></Link>}>
        <div className="sess-grid">
          {live.slice(0, 4).map((s) => (
            <SessionCard key={s.id} s={s} />
          ))}
        </div>
      </Panel>
      <div className="cgrid two">
        <Panel title="Health" action={<Link to="/console/diagnostics" className="c-link">Diagnostics <ArrowRight /></Link>}>
          <div className="health-sum">
            <div><b className="tnum">{d.health.ok}</b> passing</div>
            <div className="warn"><b className="tnum">{d.health.warn}</b> warnings</div>
            <div className="info"><b className="tnum">{d.health.info}</b> notices</div>
          </div>
          <ul className="clist">
            <li><Dot tone="warn" /> sdc (bay 3) reports 8 reallocated sectors <Link to="/console/storage">Storage</Link></li>
            <li><Dot tone="warn" /> Channel 2.1 signal at 61% <Link to="/console/livetv">Live TV</Link></li>
            <li><Dot tone="info" /> Server {d.server.latestVersion} available</li>
            <li><Dot tone="ok" /> Last backup {ago(new Date(Date.now() - 7 * 3600_000).toISOString())} · Backblaze B2</li>
          </ul>
        </Panel>
        <Panel title="Recent Activity" action={<Link to="/console/activity" className="c-link">All activity <ArrowRight /></Link>}>
          <ul className="clist act">
            {d.activity.slice(0, 6).map((a) => (
              <li key={a.id}>
                <Dot tone={a.kind === 'warning' ? 'warn' : a.kind === 'security' ? 'info' : 'ok'} />
                <span className="ellipsis">
                  <b>{a.actor}</b> {a.text}
                </span>
                <time className="tnum">{ago(a.at)}</time>
              </li>
            ))}
          </ul>
        </Panel>
      </div>
      <Panel title="Server" pad>
        <div className="server-facts">
          {[
            ['Version', `Portico Server ${d.server.version}`],
            ['Platform', d.server.platform],
            ['Hardware', d.server.cpu],
            ['GPU', d.server.gpu],
            ['Uptime', `${upDays} days`],
            ['Local', d.server.localUrl],
            ['Remote', d.server.remoteUrl],
            ['Database', d.server.database],
          ].map(([k, v]) => (
            <div key={k}>
              <span>{k}</span>
              <b>{v}</b>
            </div>
          ))}
        </div>
      </Panel>
    </div>
  );
}
