import {useState} from 'react';
import {useClient} from '@/data/DataProvider';
import type {Entry} from '@/data/types';
import {Dialog, useToast} from '@/ui/Overlay';
import {Button} from '@/ui/Button';
import {Stars} from '@/ui/Bits';
import s from './dialogs.module.css';

/** Star rating plus like or dislike, both of which tune recommendations. */
export default function RateDialog({entry, onClose}: {entry: Entry; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const p = client.personal.get(entry.id);
  const [stars, setStars] = useState<number | undefined>(p.rating);
  const [reaction, setReaction] = useState(p.reaction);
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    await client.personal.rate(entry.id, stars);
    await client.personal.react(entry.id, reaction);
    toast.show(stars ? `Rated “${entry.title}” ${stars} of 5` : `Cleared your rating for “${entry.title}”`, {icon: 'star'});
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Rate ${entry.title}`} description="Ratings are yours alone. They help Portico pick what to suggest." width={420} footer={<>
      {p.rating ? <Button variant="ghost" label="Clear rating" onClick={() => setStars(undefined)} /> : null}
      <Button variant="primary" label="Save" busy={busy} onClick={() => void save()} />
    </>}>
      <div className={s.center}>
        <Stars value={stars} onChange={setStars} size={34} />
        <div className={s.reactions}>
          <Button icon="thumbUp" label="More like this" pressed={reaction === 'like'} variant="secondary" onClick={() => setReaction(reaction === 'like' ? undefined : 'like')} />
          <Button icon="thumbDown" label="Less like this" pressed={reaction === 'dislike'} variant="secondary" onClick={() => setReaction(reaction === 'dislike' ? undefined : 'dislike')} />
        </div>
      </div>
    </Dialog>
  );
}
