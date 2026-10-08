import {useState} from 'react';
import {Link, useNavigate, useSearch} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Download, Item} from '../../data/types';
import {player} from '../../app/player';
import {Page, PageHeader, Panel, Row, Tabs} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Select, Switch} from '../../ui/Controls';
import {Badge, Progress, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Art} from '../../ui/Media';
import {ConfirmDialog} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {hrefFor, plural} from '../shared/items';

const STATE: Record<Download['state'], {label: string; tone: 'neutral' | 'accent' | 'good' | 'warn' | 'bad'}> = {
  queued: {label: 'Waiting', tone: 'neutral'}, downloading: {label: 'Downloading', tone: 'accent'}, paused: {label: 'Paused', tone: 'warn'}, done: {label: 'Ready offline', tone: 'good'},
  failed: {label: 'Failed', tone: 'bad'}, preparing: {label: 'Preparing', tone: 'accent'}, ready: {label: 'Ready to save', tone: 'good'}, expired: {label: 'Expired', tone: 'bad'},
};

/** Downloads: what's on this device, what the server is preparing, whole-show requests, and settings. */
export function DownloadsScreen() {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/downloads'});
  const tab = search.tab === 'server' || search.tab === 'settings' ? search.tab : 'device';
  const list = useQuery(`downloads.${version}`, () => services.downloads.list());
  const requests = useQuery(`downloads.requests.${version}`, () => services.downloads.requests());
  const storage = useQuery(`downloads.storage.${version}`, () => services.downloads.storage());
  const ids = [...new Set([...(list.data ?? []).map(d => d.itemId), ...(requests.data ?? []).map(r => r.showId)])];
  const items = useQuery(list.data && requests.data ? `downloads.items.${ids.join()}` : null, () => services.catalog.items(ids));
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  const device = (list.data ?? []).filter(d => d.where === 'device');
  const server = (list.data ?? []).filter(d => d.where === 'server');
  return (
    <Page title="Downloads">
      <PageHeader eyebrow="This browser" title="Downloads" lede={storage.data ? `${storage.data.usedGB} GB used here · ${storage.data.freeGB} GB free · ${storage.data.serverSpaceGB} GB for prepared downloads on the server` : 'Watch without a connection.'} />
      {storage.data ? <div className="dl-meter"><Progress value={storage.data.usedGB / (storage.data.usedGB + storage.data.freeGB)} label="Storage used" /></div> : null}
      <Tabs label="Downloads" value={tab} onChange={v => void navigate({to: '/downloads', search: {tab: v === 'device' ? undefined : v}, replace: true})} tabs={[{value: 'device', label: 'On this device', count: device.length}, {value: 'server', label: 'On your server', count: server.length + (requests.data?.length ?? 0)}, {value: 'settings', label: 'Settings'}]} />
      {tab === 'device' ? (!list.data ? <ListSkeleton /> : device.length ? <DlList list={device} byId={byId} /> : <StateView icon="download" title="Nothing downloaded yet" body="Choose Download from any title’s More menu. Your server prepares a copy, then it saves here for offline viewing." action={{label: 'Browse Movies', to: '/library/lib-movies', icon: 'film'}} />) : null}
      {tab === 'server' ? (
        !list.data ? <ListSkeleton /> : (
          <div className="stack" style={{gap: 28}}>
            {requests.data?.length ? (
              <section>
                <h2 className="section-title" style={{marginBottom: 12}}>Whole shows</h2>
                <div className="dl-list">
                  {requests.data.map(r => {
                    const show = byId.get(r.showId);
                    return (
                      <div key={r.id} className="dl-row">
                        <Art src={show?.art.poster} tint={show?.tint} ratio="2/3" className="dl-art is-poster" />
                        <div className="dl-text">
                          <strong>{show?.title ?? 'Show'}</strong>
                          <span className="faint small">{r.policy === 'all' ? 'Every episode' : r.policy === 'unwatched' ? 'Unwatched episodes' : `Keep the next ${r.keepNext} ready`} · {r.ready} of {plural(r.total, 'episode')} ready{r.failed ? ` · ${r.failed} failed` : ''}</span>
                          <Progress value={r.total ? r.ready / r.total : 0} thin label="Show download progress" />
                        </div>
                        <Select label="Which episodes" value={r.policy} onChange={() => toast('Download request updated')} options={[{value: 'unwatched', label: 'Unwatched'}, {value: 'next', label: 'Next 3'}, {value: 'all', label: 'All'}]} />
                        <Button size="sm" variant="ghost" label="Stop" onClick={() => void services.downloads.remove(r.id).then(() => toast('Stopped downloading this show'))} />
                      </div>
                    );
                  })}
                </div>
              </section>
            ) : null}
            <section>
              <h2 className="section-title" style={{marginBottom: 6}}>Prepared on your server</h2>
              <p className="faint small" style={{marginBottom: 12}}>The server converts a copy for this device, then keeps it for a week so you can save it.</p>
              {server.length ? <DlList list={server} byId={byId} /> : <StateView compact icon="server" title="Nothing being prepared" />}
            </section>
          </div>
        )
      ) : null}
      {tab === 'settings' ? <DlSettings /> : null}
    </Page>
  );
}

function DlList({list, byId}: {list: Download[]; byId: Map<string, Item>}) {
  const services = useServices();
  const [remove, setRemove] = useState<Download | null>(null);
  return (
    <div className="dl-list">
      {list.map(d => {
        const it = byId.get(d.itemId);
        const s = STATE[d.state];
        return (
          <div key={d.id} className={cx('dl-row', d.state === 'expired' && 'is-dim')}>
            {it ? <Link to={hrefFor(it as never) as never} className="dl-art-link" aria-label={it.title}><Art src={it.art.still ?? it.art.backdrop ?? it.art.poster} tint={it.tint} ratio="16/9" className="dl-art" /></Link> : <Skeleton w={132} h={74} />}
            <div className="dl-text">
              <strong>{it?.title ?? '…'}</strong>
              <span className="faint small">{d.quality} · {(d.sizeMB / 1024).toFixed(1)} GB{d.expires ? ` · Kept until ${new Date(d.expires).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}` : ''}{d.state === 'downloading' || d.state === 'preparing' ? ` · ${Math.round(d.progress * 100)}%` : ''}</span>
              {d.state === 'downloading' || d.state === 'preparing' || d.state === 'paused' ? <Progress value={d.progress} thin tone={d.state === 'paused' ? 'warn' : 'accent'} label="Download progress" /> : null}
            </div>
            <Badge tone={s.tone}>{s.label}</Badge>
            <div className="rec-actions">
              {d.state === 'done' && it ? <Button size="sm" variant="primary" icon="play" label="Play" onClick={() => player.play(it.id, {kind: 'video'})} /> : null}
              {d.state === 'downloading' || d.state === 'preparing' || d.state === 'queued' ? <Button size="sm" variant="secondary" icon="pause" label="Pause" onClick={() => void services.downloads.pause(d.id)} /> : null}
              {d.state === 'paused' ? <Button size="sm" variant="secondary" icon="play" label="Resume" onClick={() => void services.downloads.resume(d.id)} /> : null}
              {d.state === 'ready' ? <Button size="sm" variant="primary" icon="download" label="Save to this device" onClick={() => toast('Saving to this browser’s storage', {icon: 'download'})} /> : null}
              {d.state === 'expired' ? <Button size="sm" variant="secondary" icon="refresh" label="Prepare again" onClick={() => void services.downloads.resume(d.id)} /> : null}
              <Button size="sm" variant="ghost" label={d.where === 'server' && d.state !== 'ready' && d.state !== 'expired' ? 'Stop' : 'Remove'} onClick={() => setRemove(d)} />
            </div>
          </div>
        );
      })}
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={remove?.where === 'server' && remove.state === 'preparing' ? 'Stop preparing this download?' : 'Remove this download?'} body={remove?.where === 'device' ? 'It’s deleted from this browser. You can download it again any time.' : 'The server stops and deletes its prepared copy.'} confirmLabel={remove?.where === 'server' && remove.state === 'preparing' ? 'Stop preparing' : 'Remove'} danger onConfirm={async () => { if (remove) await services.downloads.remove(remove.id); toast('Removed'); }} />
    </div>
  );
}

function DlSettings() {
  const services = useServices();
  const version = useDataVersion();
  const s = useQuery(`downloads.settings.${version}`, () => services.downloads.settings());
  if (!s.data) return <ListSkeleton />;
  const set = (patch: Parameters<typeof services.downloads.updateSettings>[0]) => void services.downloads.updateSettings(patch).then(() => toast('Saved', {icon: 'check'}));
  return (
    <div className="settings-col">
      <Panel padded={false}>
        <Row label="Download over Wi-Fi only" help="Saves mobile data. Downloads wait until you’re on Wi-Fi."><Switch checked={s.data.wifiOnly} onChange={v => set({wifiOnly: v})} label="Download over Wi-Fi only" /></Row>
        <Row label="Remove after watching" help="Frees space once you finish a film or episode."><Switch checked={s.data.removeAfterWatching} onChange={v => set({removeAfterWatching: v})} label="Remove after watching" /></Row>
        <Row label="Download the next episode" help="When you finish an episode you downloaded, the next one starts."><Switch checked={s.data.nextEpisode} onChange={v => set({nextEpisode: v})} label="Download the next episode" /></Row>
        <Row label="Quality" help="Higher quality takes more space."><Select label="Quality" value={s.data.quality} onChange={v => set({quality: v})} options={[{value: 'Original', label: 'Original'}, {value: '1080p', label: 'High · 1080p'}, {value: '720p', label: 'Standard · 720p'}, {value: '480p', label: 'Data saver · 480p'}]} /></Row>
        <Row label="Storage limit" help="Portico won’t download past this."><Select label="Storage limit" value={String(s.data.limitGB ?? 'none')} onChange={v => set({limitGB: v === 'none' ? null : Number(v)})} options={[{value: '8', label: '8 GB'}, {value: '16', label: '16 GB'}, {value: '32', label: '32 GB'}, {value: '64', label: '64 GB'}, {value: 'none', label: 'No limit'}]} /></Row>
      </Panel>
    </div>
  );
}

function ListSkeleton() {
  return <div className="stack">{[0, 1, 2].map(i => <Skeleton key={i} h={88} />)}</div>;
}
