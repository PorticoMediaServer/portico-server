import { Cpu } from 'lucide-react';
import { transcoderSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { api } from '../../../services/client';
import { ErrorState, Segmented, Skel, Switch } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { Bar, Panel, SettingRow } from '../ui';

type T = typeof transcoderSeed;

export function Transcoding() {
  const q = useAdminSection<T>('transcoding');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const d = q.data;
  const save = (patch: Partial<T>) => api.updateAdminSection('transcoding', patch).then(() => (q.refetch(), toast('Transcoder settings saved')));
  return (
    <div className="cstack">
      <div className="cbanner ok">
        <Cpu />
        <div>
          <strong>Hardware transcoding is active.</strong> {d.api} on {d.device}. Test encode: 1080p H.264 at 412 fps (≈ 17 simultaneous 1080p streams).
        </div>
      </div>
      <div className="cgrid two">
        <Panel title="Hardware">
          <SettingRow title="Use hardware acceleration" sub={d.device}>
            <Switch size="sm" accent checked={d.hardwareAcceleration} onChange={(v) => save({ hardwareAcceleration: v })} label="Hardware acceleration" />
          </SettingRow>
          <SettingRow title="HEVC encoding" sub="Smaller streams for devices that support it">
            <Switch size="sm" accent checked={d.hevcEncoding} onChange={(v) => save({ hevcEncoding: v })} label="HEVC encoding" />
          </SettingRow>
          <SettingRow title="AV1 encoding" sub="Not supported by this GPU">
            <Switch size="sm" accent checked={false} disabled onChange={() => undefined} label="AV1 encoding" />
          </SettingRow>
          <SettingRow title="HDR tone mapping">
            <select className="input sm" value={d.toneMapping} onChange={(e) => save({ toneMapping: e.target.value })}>
              <option>Hardware (OpenCL)</option>
              <option>Software</option>
              <option>Off</option>
            </select>
          </SettingRow>
          <table className="caps">
            <thead>
              <tr>
                <th>Codec</th>
                <th>Decode</th>
                <th>Encode</th>
              </tr>
            </thead>
            <tbody>
              {d.capabilities.map(([c, dec, enc]) => (
                <tr key={c}>
                  <td>{c}</td>
                  <td className={dec === '—' ? 'no' : 'yes'}>{dec === '—' ? '—' : '✓'}</td>
                  <td className={enc === '—' ? 'no' : 'yes'}>{enc === '—' ? '—' : '✓'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
        <Panel title="Quality & limits">
          <SettingRow title="Transcoder quality">
            <Segmented size="sm" value={d.quality} onChange={(v) => save({ quality: v })} options={[{ value: 'Prefer faster encoding', label: 'Faster' }, { value: 'Automatic', label: 'Auto' }, { value: 'Prefer higher quality', label: 'Quality' }]} />
          </SettingRow>
          <SettingRow title="Simultaneous transcodes" sub="Extra requests queue or fall back to lower quality">
            <select className="input sm" value={d.maxTranscodes} onChange={(e) => save({ maxTranscodes: Number(e.target.value) })}>
              {[2, 4, 6, 8, 12, 0].map((n) => (
                <option key={n} value={n}>
                  {n || 'Unlimited'}
                </option>
              ))}
            </select>
          </SettingRow>
          <SettingRow title="Throttle buffer" sub="Pause encoding once this far ahead of the viewer">
            <span className="tnum">{d.throttleBuffer} s</span>
          </SettingRow>
          <SettingRow title="Segment length">
            <span className="tnum">{d.segmentDuration} s</span>
          </SettingRow>
          <SettingRow title="Burn in subtitles">
            <select className="input sm" value={d.subtitleBurnIn} onChange={(e) => save({ subtitleBurnIn: e.target.value })}>
              <option>Only image-based and styled</option>
              <option>Only image-based</option>
              <option>Always</option>
            </select>
          </SettingRow>
          <SettingRow title="Deinterlacing">
            <span className="t-mono">{d.deinterlace}</span>
          </SettingRow>
          <SettingRow title="Temporary folder" sub={`Limit ${d.tempLimitGb} GB`}>
            <code className="t-mono">{d.tempDir}</code>
          </SettingRow>
        </Panel>
      </div>
      <Panel title="Optimized versions" sub="Pre-converted copies for phones and remote viewers, made overnight">
        {d.optimizeQueue.map((o) => (
          <div key={o.title} className="opt-row">
            <b>{o.title}</b>
            <span className="t-sub">{o.target}</span>
            <Bar value={o.progress} />
            <span className="tnum t-sub">{o.progress ? `${Math.round(o.progress * 100)}% · ${o.eta}` : o.eta}</span>
          </div>
        ))}
      </Panel>
    </div>
  );
}
