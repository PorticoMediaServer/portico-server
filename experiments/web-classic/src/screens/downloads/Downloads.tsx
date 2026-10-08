import {useState} from 'react';
import {Link} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Download} from '@/data/types';
import {hrefFor} from '@/app/links';
import {usePlayer} from '@/player/PlayerProvider';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, EmptyState, ErrorState, Meter, Progress, Skeleton, Surface, type Tone} from '@/ui/Bits';
import {Select, Switch} from '@/ui/Form';
import {ConfirmDialog, Menu, useToast} from '@/ui/Overlay';
import {Group, PageHeader, Row, Section} from '@/ui/Page';
import {useTitle} from '@/ui/hooks';

const STATE: Record<Download['state'], {label: string; tone: Tone}> = {
  queued: {label: 'Waiting', tone: 'neutral'}, preparing: {label: 'Server is preparing', tone: 'accent'}, downloading: {label: 'Downloading', tone: 'accent'},
  ready: {label: 'Ready', tone: 'good'}, paused: {label: 'Paused', tone: 'neutral'}, failed: {label: 'Didn’t finish', tone: 'bad'}, expired: {label: 'Expired', tone: 'warn'},
};

/** Downloads kept in this browser for watching offline: progress, storage, show requests and settings. */
export default function DownloadsScreen() {
  const q = useQuery(c => c.downloads.list(), [], ['downloads']);
  const client = useClient();
  const player = usePlayer();
  const toast = useToast();
  const [remove, setRemove] = useState<Download>();
  const settings = client.downloads.settings();
  useTitle('Downloads');
  if (q.error) return <ErrorState onRetry={q.refresh} />;
  const d = q.data;
  const set = async (patch: Partial<typeof settings>) => { await client.downloads.updateSettings(patch); toast.show('Saved', {icon: 'check'}); };
  return (
    <div style={{padding: '28px 0 72px'}}>
      <PageHeader title="Downloads" subtitle="Titles saved in this browser play without a connection to the server. Downloads expire after 30 days offline." />
      {!d ? <div style={{padding: '0 var(--gutter)', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 8}}>{[0, 1, 2, 3].map(i => <Skeleton key={i} height={84} />)}</div> : (
        <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 32}}>
          <Section>
            <Surface>
              <div style={{display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 10}}>
                <span className="t-strong">{d.usedGB} GB used by downloads</span>
                <span className="t-caption c-3">{d.freeGB} GB free in this browser{settings.limitGB ? ` · limit ${settings.limitGB} GB` : ''}</span>
              </div>
              <Meter value={d.usedGB / (settings.limitGB ?? d.usedGB + d.freeGB)} label="Download space used" />
            </Surface>
          </Section>
          {d.requests.length ? (
            <Section title="Shows kept up to date">
              <Group>
                {d.requests.map(r => (
                  <Row key={r.id} iconNode={<Artwork src={r.show.image} title={r.show.title} style={{width: 40}} />} label={r.show.title} help={`${r.policy === 'next' ? `Next ${r.keep} unwatched episodes` : r.policy === 'unwatched' ? 'All unwatched episodes' : 'Every episode'} · ${r.ready} of ${r.total} ready`} trailing={<><Badge tone={r.state === 'failed' ? 'bad' : 'accent'}>{r.state === 'preparing' ? 'Preparing' : r.state === 'failed' ? 'Stopped' : 'Active'}</Badge><IconButton variant="ghost" icon="trash" label={`Stop downloading ${r.show.title}`} onClick={async () => { await client.downloads.remove(r.id); toast.show(`Stopped downloading ${r.show.title}`, {icon: 'check'}); }} /></>} />
                ))}
              </Group>
            </Section>
          ) : null}
          <Section title="In this browser">
            {!d.items.length ? <EmptyState icon="download" title="No downloads yet" body="Choose Download on a movie, episode, album or audiobook to keep it here for offline viewing." actions={[{label: 'Browse movies', to: '/library/lib-movies'}]} /> : (
              <Group>
                {d.items.map(item => (
                  <Row key={item.id} wrap iconNode={<Artwork src={item.entry.kind === 'episode' ? item.entry.poster ?? item.entry.image : item.entry.image} title={item.entry.title} style={{width: 44}} tint={item.entry.tint} />}
                    label={hrefFor(item.entry) ? <Link to={hrefFor(item.entry)! as never}>{item.entry.kind === 'episode' ? `${item.entry.subtitle} · ${item.entry.title}` : item.entry.title}</Link> : item.entry.title}
                    help={<span style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 6}}><span>{item.quality} · {item.size}{item.expires ? ` · Expires ${item.expires}` : ''}{item.watched ? ' · Watched' : ''}</span>{item.state === 'downloading' || item.state === 'preparing' || item.state === 'paused' ? <Progress value={item.progress} label={`${Math.round(item.progress * 100)}% downloaded`} /> : null}</span>}
                    trailing={<>
                      <Badge tone={STATE[item.state].tone}>{STATE[item.state].label}{item.state === 'downloading' ? ` · ${Math.round(item.progress * 100)}%` : ''}</Badge>
                      {item.state === 'ready' ? <IconButton variant="secondary" icon="play" label={`Play ${item.entry.title}`} onClick={() => player.play([item.entry], 0)} /> : null}
                      {item.state === 'failed' ? <Button size="sm" variant="secondary" icon="refresh" label="Try again" onClick={() => void client.downloads.retry(item.id)} /> : null}
                      <Menu label={`More for ${item.entry.title}`} items={[
                        ...(item.state === 'downloading' || item.state === 'preparing' ? [{id: 'pause', label: 'Pause', icon: 'pause' as const, onSelect: () => void client.downloads.pause(item.id, true)}] : []),
                        ...(item.state === 'paused' ? [{id: 'resume', label: 'Resume', icon: 'play' as const, onSelect: () => void client.downloads.pause(item.id, false)}] : []),
                        {id: 'remove', label: item.state === 'ready' ? 'Remove download' : 'Cancel download', icon: 'trash', danger: true, onSelect: () => setRemove(item)},
                      ]} trigger={<IconButton variant="ghost" icon="more" label={`More for ${item.entry.title}`} />} />
                    </>} />
                ))}
              </Group>
            )}
          </Section>
          <Section title="Settings">
            <Group>
              <Row icon="quality" label="Download quality" help="New downloads use this unless you choose another." trailing={<Select compact ariaLabel="Download quality" value={settings.quality} onChange={v => void set({quality: v})} options={[{value: 'Original', label: 'Original'}, {value: '1080p', label: 'High · 1080p'}, {value: '720p', label: 'Medium · 720p'}, {value: 'Smaller', label: 'Smaller · 480p'}]} />} />
              <Row icon="storage" label="Space for downloads" help="Older watched downloads make room when this fills up." trailing={<Select compact ariaLabel="Space limit" value={String(settings.limitGB ?? 'none')} onChange={v => void set({limitGB: v === 'none' ? null : Number(v)})} options={[{value: '8', label: '8 GB'}, {value: '16', label: '16 GB'}, {value: '32', label: '32 GB'}, {value: '64', label: '64 GB'}, {value: 'none', label: 'No limit'}]} />} />
              <Row icon="check" label="Remove after watching" help="Delete a download once you finish it." trailing={<Switch checked={settings.removeAfterWatching} onChange={v => void set({removeAfterWatching: v})} label="Remove after watching" />} />
              <Row icon="next" label="Download the next episode" help="When you finish an episode you downloaded, get the next one." trailing={<Switch checked={settings.nextEpisode} onChange={v => void set({nextEpisode: v})} label="Download the next episode" />} />
            </Group>
          </Section>
        </div>
      )}
      {remove ? <ConfirmDialog open onOpenChange={o => !o && setRemove(undefined)} title={remove.state === 'ready' ? `Remove “${remove.entry.title}”?` : `Cancel downloading “${remove.entry.title}”?`} body="It stays on the server. You can download it again at any time." action={remove.state === 'ready' ? 'Remove' : 'Cancel download'} cancel="Keep" tone="danger" onConfirm={async () => { await client.downloads.remove(remove.id); setRemove(undefined); toast.show('Removed', {icon: 'check'}); }} /> : null}
    </div>
  );
}
