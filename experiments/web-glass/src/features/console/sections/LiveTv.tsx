import { Plus, RefreshCw, Radio, Tv } from 'lucide-react';
import { liveSourcesSeed } from '../../../data/fixtures/server';
import { channels } from '../../../data/fixtures/live';
import { useAdminSection } from '../../../services/queries';
import { Button, ErrorState, Skel, Switch } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { ago } from '../../../lib/hooks';
import { Bar, Dot, KV, Panel, SettingRow, Table } from '../ui';
import { ChannelLogo } from '../../live/Live';

type L = typeof liveSourcesSeed;

export function LiveTv() {
  const q = useAdminSection<L>('livetv');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const d = q.data;
  const hd = d.tuners[0];
  return (
    <div className="cstack">
      <div className="ctoolbar">
        <Button size="sm" variant="accent" icon={<Plus />} onClick={() => toast('Scanning the network for tuners…', <Radio />)}>
          Add Tuner or Playlist
        </Button>
        <Button size="sm" variant="plain" icon={<RefreshCw />} onClick={() => toast('Refreshing 14 days of guide data', <RefreshCw />)}>
          Refresh Guide
        </Button>
      </div>
      <div className="cgrid two">
        <Panel title={hd.name} sub={`${hd.model} · ${hd.ip} · firmware ${hd.firmware} · ATSC 3.0`}>
          <div className="tuners">
            {hd.tuners.map((t) => (
              <div key={t.n} className="tuner">
                <div className="tuner-head">
                  <span>Tuner {t.n}</span>
                  <span className={t.state === 'Idle' ? 'ctag' : t.state === 'Recording' ? 'ctag err' : 'ctag info'}>{t.state}</span>
                </div>
                <div className="t-sub">{t.channel}</div>
                {t.state !== 'Idle' && (
                  <div className="tuner-bars tnum">
                    <span>Signal</span>
                    <Bar value={t.signal / 100} tone={t.signal < 70 ? 'warn' : undefined} />
                    <b>{t.signal}%</b>
                    <span>Quality</span>
                    <Bar value={t.quality / 100} />
                    <b>{t.quality}%</b>
                  </div>
                )}
              </div>
            ))}
          </div>
        </Panel>
        <Panel title="Sources">
          {d.iptv.map((s) => (
            <KV key={s.id} rows={[['IPTV playlist', s.url], ['Channels', `${s.channels} · ${s.streams}`], ['Refreshed', ago(s.lastRefresh)]]} />
          ))}
          {d.guide.map((g) => (
            <KV key={g.id} rows={[['Guide data', g.name], ['Lineup', g.lineup], ['Coverage', `${g.days} days · ${g.channels} channels`], ['Updated', ago(g.lastUpdate)]]} />
          ))}
        </Panel>
      </div>
      <Panel title="Channel map" sub="Enable, reorder and match channels to guide data" pad={false}>
        <Table
          rows={channels}
          getKey={(c) => c.id}
          dense
          cols={[
            { key: 'logo', label: '', render: (c) => <ChannelLogo ch={c} size={34} />, width: '56px' },
            { key: 'num', label: 'Number', sort: (c) => parseFloat(c.number), render: (c) => <span className="tnum">{c.number}</span>, width: '80px' },
            { key: 'name', label: 'Name', sort: (c) => c.name, render: (c) => <b>{c.name}</b>, width: 'minmax(120px,1fr)' },
            { key: 'call', label: 'Guide ID', render: (c) => <span className="t-mono">{c.callSign}</span>, width: '100px' },
            { key: 'src', label: 'Source', sort: (c) => c.source, render: (c) => <span className="ctag">{c.source}</span>, width: '110px' },
            { key: 'cat', label: 'Category', render: (c) => c.category, width: '120px' },
            { key: 'q', label: 'Quality', render: (c) => (c.hd ? 'HD' : 'SD'), width: '70px' },
            { key: 'sig', label: 'Status', render: (c) => (c.id === 'ch-2' ? <span className="c-warn"><Dot tone="warn" /> Weak (61%)</span> : <Dot tone="ok" />), width: '120px' },
            { key: 'on', label: 'On', render: () => <Switch size="sm" accent checked onChange={() => toast('Saved')} label="Enabled" />, width: '60px' },
          ]}
        />
      </Panel>
      <Panel title="DVR">
        <SettingRow title="Recording folder" sub="Recordings are added to TV Shows and Movies automatically">
          <code className="t-mono">{d.dvr.path}</code>
        </SettingRow>
        <SettingRow title="Format">
          <span className="t-sub">{d.dvr.format}</span>
        </SettingRow>
        <SettingRow title="Commercials" sub="Detected after recording finishes">
          <select className="input sm" defaultValue={d.dvr.commercialDetection}>
            <option>Mark chapters</option>
            <option>Remove</option>
            <option>Off</option>
          </select>
        </SettingRow>
        <SettingRow title="Default padding">
          <span className="t-sub tnum">
            {d.dvr.defaultPaddingBefore} min before · {d.dvr.defaultPaddingAfter} min after
          </span>
        </SettingRow>
        <SettingRow title="Delete watched recordings">
          <select className="input sm" defaultValue={d.dvr.deleteWatchedAfter}>
            <option>Never</option>
            <option>1 day</option>
            <option>7 days</option>
            <option>30 days</option>
          </select>
        </SettingRow>
        <SettingRow title="Space limit" sub="Oldest watched recordings go first">
          <span className="t-sub tnum">
            <Tv className="inline-ic" />
            {d.dvr.maxDiskGb.toLocaleString('en-US')} GB
          </span>
        </SettingRow>
      </Panel>
    </div>
  );
}
