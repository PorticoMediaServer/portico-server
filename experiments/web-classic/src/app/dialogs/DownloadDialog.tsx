import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient} from '@/data/DataProvider';
import type {Entry} from '@/data/types';
import {Dialog, useToast} from '@/ui/Overlay';
import {Button} from '@/ui/Button';
import {RadioCards, Select} from '@/ui/Form';
import s from './dialogs.module.css';

const QUALITIES = [
  {value: 'Original', label: 'Original', help: 'Best picture, largest file. Plays as it is stored.'},
  {value: '1080p', label: 'High · 1080p', help: 'About 2 GB an hour.'},
  {value: '720p', label: 'Medium · 720p', help: 'About 1 GB an hour. Good for tablets.'},
  {value: 'Smaller', label: 'Smaller · 480p', help: 'About 400 MB an hour. Saves the most space.'},
] as const;

/** Download a title for offline viewing; a show becomes a standing request (next episodes, unwatched, or all). */
export default function DownloadDialog({entry, onClose}: {entry: Entry; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const navigate = useNavigate();
  const settings = client.downloads.settings();
  const [quality, setQuality] = useState<string>(settings.quality);
  const [policy, setPolicy] = useState<'next' | 'unwatched' | 'all'>('next');
  const [keep, setKeep] = useState('3');
  const [busy, setBusy] = useState(false);
  const show = entry.kind === 'show';
  const go = async () => {
    setBusy(true);
    if (show) await client.downloads.addShow(entry.id, policy, Number(keep));
    else await client.downloads.add(entry.id, quality);
    toast.show(show ? `Downloading episodes of “${entry.title}”` : `Downloading “${entry.title}”`, {icon: 'download', action: {label: 'View', run: () => void navigate({to: '/downloads' as never})}});
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Download ${entry.title}`} description="The server prepares a copy for this browser. Downloads play without a connection to the server." width={500} footer={<>
      <Button variant="ghost" label="Cancel" onClick={onClose} />
      <Button variant="primary" icon="download" label="Download" busy={busy} onClick={() => void go()} />
    </>}>
      <div className={s.stack}>
        {show ? (
          <>
            <RadioCards label="Which episodes" value={policy} onChange={setPolicy} options={[
              {value: 'next', label: 'Next episodes', help: 'Keeps the next few unwatched episodes ready, and swaps them as you watch.'},
              {value: 'unwatched', label: 'All unwatched episodes'},
              {value: 'all', label: 'Every episode'},
            ]} />
            {policy === 'next' ? <Select label="How many to keep" value={keep} onChange={setKeep} options={['1', '3', '5', '10'].map(v => ({value: v, label: `${v} episode${v === '1' ? '' : 's'}`}))} /> : null}
          </>
        ) : null}
        <RadioCards label="Quality" value={quality} onChange={setQuality} options={QUALITIES} />
      </div>
    </Dialog>
  );
}
