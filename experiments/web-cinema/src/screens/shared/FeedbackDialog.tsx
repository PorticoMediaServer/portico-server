import {useState} from 'react';
import {useQuery, useServices} from '../../data/hooks';
import {Dialog, DialogClose} from '../../ui/Overlay';
import {Button} from '../../ui/Button';
import {Checkbox, ChoiceCards, Field, Select, TextArea} from '../../ui/Controls';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';

const CATEGORIES: Record<string, string[]> = {
  problem: ['Playback', 'Subtitles', 'Audio', 'Wrong match', 'Wrong artwork', 'Missing episodes', 'Something else'],
  idea: ['Finding things', 'Watching', 'Music', 'Live TV', 'Something else'],
};

/** Report a problem or send an idea to the server owner, with playback diagnostics if wanted. */
export function FeedbackDialog({open, onOpenChange, itemId, fromPlayer}: {open: boolean; onOpenChange: (o: boolean) => void; itemId?: string; fromPlayer?: boolean}) {
  const services = useServices();
  const item = useQuery(itemId ? `item.${itemId}` : null, () => services.catalog.item(itemId!)).data;
  const [kind, setKind] = useState<'problem' | 'idea'>('problem');
  const [category, setCategory] = useState(fromPlayer ? 'Playback' : '');
  const [message, setMessage] = useState('');
  const [diag, setDiag] = useState(true);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<'sent' | 'duplicate' | null>(null);
  const tooShort = message.trim().length > 0 && message.trim().length < 10;
  const send = async () => {
    setBusy(true);
    const r = await services.feedback.send({kind, category, message, itemId, diagnostics: diag});
    setBusy(false);
    setResult(r);
  };
  if (result) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange} title={result === 'sent' ? 'Thanks, it’s sent' : 'Already reported'} size="sm" footer={<Button variant="primary" label="Done" onClick={() => onOpenChange(false)} />}>
        <div className="feedback-done"><span className="auth-glyph is-good"><Icon name={result === 'sent' ? 'send' : 'check'} size={26} /></span><p className="muted">{result === 'sent' ? 'The server owner will see it in their console. If they reply, you’ll get a notification.' : 'Someone already reported this and the owner has it. You’ll be told when it’s fixed.'}</p></div>
      </Dialog>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={item ? `Report a problem with “${item.title}”` : 'Send feedback'} description="Goes to the owner of this server." size="md" footer={<>
      <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>
      <Button variant="primary" icon="send" label="Send" busy={busy} disabled={!category || message.trim().length < 10 || message.length > 1000} onClick={() => void send()} />
    </>}>
      <div className="stack">
        {item ? <div className="fb-item"><Art src={item.art.poster ?? item.art.square} tint={item.tint} ratio="2/3" className="fb-item-art" /><span className="set-text"><strong>{item.title}</strong><span>{item.year}</span></span></div> : null}
        <ChoiceCards label="Kind" columns={2} value={kind} onChange={v => { setKind(v); setCategory(''); }} options={[{value: 'problem', title: 'Something’s wrong', icon: 'flag'}, {value: 'idea', title: 'An idea', icon: 'idea'}]} />
        <Field label="About"><Select value={category} onChange={setCategory} options={[{value: '', label: 'Choose one', disabled: true}, ...CATEGORIES[kind]!.map(c => ({value: c, label: c}))]} /></Field>
        <Field label="What happened?" error={tooShort ? 'A few more words help the owner fix it.' : message.length > 1000 ? 'Keep it under 1,000 characters.' : undefined} help={`${message.length} / 1,000`}><TextArea value={message} onChange={e => setMessage(e.target.value)} rows={4} placeholder={kind === 'problem' ? 'The subtitles are about two seconds late from the start.' : 'It would be great if…'} /></Field>
        {kind === 'problem' ? <Checkbox checked={diag} onChange={setDiag} label="Include playback diagnostics (device, connection, what was playing)" /> : null}
      </div>
    </Dialog>
  );
}
