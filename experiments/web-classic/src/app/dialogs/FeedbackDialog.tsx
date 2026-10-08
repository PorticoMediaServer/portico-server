import {useState} from 'react';
import {useClient} from '@/data/DataProvider';
import type {Entry} from '@/data/types';
import {Dialog, useToast} from '@/ui/Overlay';
import {Button} from '@/ui/Button';
import {Checkbox, Segmented, Select, TextField} from '@/ui/Form';
import s from './dialogs.module.css';

const CATEGORIES = ['Playback', 'Audio', 'Subtitles', 'Picture quality', 'Wrong details or artwork', 'Something else'];

/** Report a problem (or send an idea) to the server's owner. Optionally attaches diagnostics. */
export default function FeedbackDialog({entry, onClose}: {entry?: Entry; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [kind, setKind] = useState<'problem' | 'idea'>('problem');
  const [category, setCategory] = useState(entry ? 'Playback' : 'Something else');
  const [message, setMessage] = useState('');
  const [diagnostics, setDiagnostics] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const send = async () => {
    if (message.trim().length < 10) { setError('Add a few more words so the owner knows what happened.'); return; }
    setBusy(true);
    const result = await client.notifications.sendFeedback({kind, category, message: message.trim(), itemId: entry?.id, diagnostics});
    toast.show(result === 'duplicate' ? 'You already sent this. The owner has it.' : 'Sent to the server’s owner. You’ll see their reply in Notifications.', {icon: 'mail'});
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={entry ? `Report a problem with ${entry.title}` : 'Send feedback'} description="Your message goes to the person who runs this server." width={520} footer={<>
      <Button variant="ghost" label="Cancel" onClick={onClose} />
      <Button variant="primary" icon="mail" label="Send" busy={busy} onClick={() => void send()} />
    </>}>
      <div className={s.stack}>
        <Segmented label="Kind of feedback" value={kind} onChange={setKind} options={[{value: 'problem', label: 'Something’s wrong'}, {value: 'idea', label: 'An idea'}]} />
        {kind === 'problem' ? <Select label="What kind of problem" value={category} onChange={setCategory} options={CATEGORIES.map(c => ({value: c, label: c}))} /> : null}
        <TextField multiline label={kind === 'problem' ? 'What happened' : 'Your idea'} placeholder={kind === 'problem' ? 'The audio drops out about 12 minutes in…' : 'It would be great if…'} value={message} onChange={v => { setMessage(v); setError(undefined); }} error={error} autoFocus />
        {kind === 'problem' ? <Checkbox checked={diagnostics} onChange={setDiagnostics} label="Include technical details" help="Your browser, the file’s format and recent playback events. No passwords or personal files." /> : null}
      </div>
    </Dialog>
  );
}
