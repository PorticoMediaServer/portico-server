import { useState } from 'react';
import { CheckCircle2, Download, Info, Play, Stethoscope, TriangleAlert } from 'lucide-react';
import { diagnosticsSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { api } from '../../../services/client';
import { Button, ErrorState, Skel } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { Panel } from '../ui';

type D = typeof diagnosticsSeed;

export function Diagnostics() {
  const q = useAdminSection<D>('diagnostics');
  const [running, setRunning] = useState(false);
  const [speed, setSpeed] = useState<null | { down: number; up: number; ping: number }>(null);
  const [testing, setTesting] = useState(false);
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  return (
    <div className="cstack">
      <div className="ctoolbar">
        <Button
          size="sm"
          variant="accent"
          icon={<Stethoscope />}
          loading={running}
          onClick={async () => {
            setRunning(true);
            await api.runDiagnostics();
            setRunning(false);
            toast('All checks complete · 2 warnings', <Stethoscope />);
          }}
        >
          {running ? 'Running Checks…' : 'Run All Checks'}
        </Button>
        <Button size="sm" variant="plain" icon={<Download />} onClick={() => toast('Support bundle ready: harbor-support-2026-09-27.zip (logs, settings, no media or passwords)', <Download />)}>
          Create Support Bundle
        </Button>
      </div>
      <Panel title="Health checks" pad={false}>
        <ul className="diag">
          {q.data.checks.map((c) => (
            <li key={c.id}>
              {c.status === 'ok' ? <CheckCircle2 className="c-ok" /> : c.status === 'warn' ? <TriangleAlert className="c-warn" /> : <Info className="c-info" />}
              <span className="diag-area">{c.area}</span>
              <b>{c.name}</b>
              <span className="t-sub diag-detail">{c.detail}</span>
            </li>
          ))}
        </ul>
      </Panel>
      <div className="cgrid two">
        <Panel title="Remote streaming test" sub="Measures the path a remote viewer would use">
          {speed ? (
            <div className="speed">
              <div>
                <span className="t-sub">Download</span>
                <b className="tnum">{speed.down}</b> Mbps
              </div>
              <div>
                <span className="t-sub">Upload</span>
                <b className="tnum">{speed.up}</b> Mbps
              </div>
              <div>
                <span className="t-sub">Latency</span>
                <b className="tnum">{speed.ping}</b> ms
              </div>
            </div>
          ) : (
            <p className="t-sub" style={{ marginBottom: 12 }}>
              Not run yet today.
            </p>
          )}
          <Button
            size="sm"
            variant="plain"
            icon={<Play />}
            loading={testing}
            onClick={() => {
              setTesting(true);
              setTimeout(() => {
                setSpeed({ down: 938, up: 297, ping: 14 });
                setTesting(false);
              }, 2200);
            }}
          >
            {testing ? 'Testing…' : 'Run Speed Test'}
          </Button>
        </Panel>
        <Panel title="Playback test" sub="Streams a 30-second clip through the transcoder">
          <p className="t-sub" style={{ marginBottom: 12 }}>
            Last result: 4K HEVC HDR → 1080p H.264 SDR, hardware, 6.8× realtime, 0 dropped frames.
          </p>
          <Button size="sm" variant="plain" icon={<Play />} onClick={() => toast('Test stream passed · 6.9× realtime', <CheckCircle2 />)}>
            Run Playback Test
          </Button>
        </Panel>
      </div>
    </div>
  );
}
