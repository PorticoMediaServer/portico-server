import React, {useEffect, useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient} from '../data/DataProvider';
import type {Entry} from '../data/types';
import {Sheet} from '../ui/Sheet';
import {Button} from '../ui/Button';
import {EmptyState, Notice, Segmented, TextField, Toggle, Chip} from '../ui/Controls';
import {Text} from '../ui/Text';
import {useMetrics} from '../theme/layout';
import {useSession} from './session';

const CATEGORIES = ['Playback', 'Subtitles', 'Audio', 'Metadata', 'Artwork', 'Something else'];

/** Report a problem / Send feedback to the server's owner. */
export function FeedbackSheet({visible, entry, onClose}: {visible: boolean; entry?: Entry; onClose: () => void}) {
  const client = useClient();
  const m = useMetrics();
  const {profile} = useSession();
  const [kind, setKind] = useState<'title' | 'app' | 'server'>(entry ? 'title' : 'app');
  const [category, setCategory] = useState('Playback');
  const [message, setMessage] = useState('');
  const [diag, setDiag] = useState(true);
  const [sent, setSent] = useState<'sent' | 'duplicate'>();
  const {busy, run, error} = useAction();
  useEffect(() => { if (visible) { setSent(undefined); setMessage(''); setKind(entry ? 'title' : 'app'); } }, [visible, entry]);
  const send = () => void run(async () => setSent(await client.notifications.sendFeedback({kind, category, message, itemId: entry?.id, diagnostics: diag})));
  return (
    <Sheet visible={visible} onClose={onClose} title="Report a problem" subtitle={entry ? `About “${entry.title}”` : undefined} size="md"
      footer={sent ? <Button label="Done" variant="primary" onPress={onClose} /> : <><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Send" icon="mail" variant="primary" loading={busy} disabled={message.trim().length < 10} onPress={send} /></>}>
      {sent ? (
        <EmptyState icon="check" title={sent === 'sent' ? 'Thanks, that’s been sent' : 'Already reported'} body={sent === 'sent' ? 'The server’s owner can see it now. Any reply will arrive in your notifications.' : 'You reported this recently, so it has been added to that report rather than sent again.'} compact />
      ) : (
        <>
          <Text variant="captionStrong" tone="secondary">What is it about?</Text>
          <Segmented value={kind} onChange={setKind} options={[...(entry ? [{value: 'title' as const, label: 'This title'}] : []), {value: 'app', label: 'The app'}, {value: 'server', label: 'The server'}]} />
          <Text variant="captionStrong" tone="secondary">Category</Text>
          <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.xs}}>
            {CATEGORIES.map(c => <Chip key={c} label={c} selected={category === c} onPress={() => setCategory(c)} />)}
          </View>
          <TextField label="What happened?" multiline value={message} onChangeText={setMessage} placeholder="What you were doing, what you expected, and what happened instead." help={`${message.length} / 2000`} maxLength={2000} />
          <Toggle label={entry ? 'Include technical details about this playback' : 'Include technical details'} help="How the stream was delivered and what the player reported. No viewing history beyond this title." value={diag} onChange={setDiag} />
          {error ? <Notice tone="error" body={error} /> : null}
          <Text variant="caption" tone="tertiary">Sent as {profile?.name} to this server’s owner.</Text>
        </>
      )}
    </Sheet>
  );
}
