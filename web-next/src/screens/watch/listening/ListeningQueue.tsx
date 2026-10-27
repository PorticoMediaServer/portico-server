import {defaultI18n} from '@i18n';
import {queueSaveMessage, type QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import type {V1QueuePolicyIntent} from '@core/playback-v1/v1-queue-player.ts';
import type {QueueSnapshot} from '@core/queues.ts';
import type {WebQueueListSnapshot} from '../../../platform/playback/queue-list';
import {Button, TextField} from '../../../kit/controls';
import {Banner, Dialog} from '../../../kit/overlays';
import {StateView} from '../../../kit/states';
import styles from './listening-panel.module.css';
const t = defaultI18n.t;

/** Occurrence rows and accepted policy only. Every callback receives the exact
 * rendered queue witness; the existing owner remains the mutation authority. */
export function ListeningQueue({snapshot, policy, policyBusy, onEdit, onPolicy, onRefresh, onMore, save}: Readonly<{
  snapshot: WebQueueListSnapshot;
  policy?: Pick<QueueSnapshot, 'repeat' | 'shuffled'>;
  policyBusy?: boolean;
  onEdit: (shown: WebQueueListSnapshot, entryId: string, action: 'remove' | -1 | 1) => void;
  onPolicy: (shown: WebQueueListSnapshot, intent: V1QueuePolicyIntent) => void;
  onRefresh: (shown: WebQueueListSnapshot) => void;
  /** Offered only after the real canonical continuation/edit grant is public. */
  onMore?: (shown: WebQueueListSnapshot) => void;
  save?: Readonly<{open: boolean; name: string; progress: QueuePlaylistSave | null;
    openDialog: (shown: WebQueueListSnapshot) => void; close: () => void;
    changeName: (name: string) => void; confirm: (shown: WebQueueListSnapshot, frozenName: string) => void}>;
}>) {
  if (!snapshot.current) return null;
  const busy = snapshot.change === 'sending' || policyBusy === true, saving = save?.progress?.state === 'saving';
  const message = save?.progress ? queueSaveMessage(save.progress) : undefined;
  return <section className={styles.queue} aria-label={t('player.queue')}>
    {snapshot.error && <Banner action={{label: t('next.states.tryAgain'), onAction: () => onRefresh(snapshot)}}>{t('player.queueSaveChanged')}</Banner>}
    {policy && <div className={styles.policy}>
      <Button size="small" variant="glass" disabled={busy} aria-pressed={policy.shuffled} onClick={() => onPolicy(snapshot, {action: 'shuffle', shuffled: !policy.shuffled})}>{t(policy.shuffled ? 'player.opt.shuffleOn' : 'player.opt.shuffleOff')}</Button>
      <Button size="small" variant="glass" disabled={busy} onClick={() => onPolicy(snapshot, {action: 'repeat', repeat: policy.repeat === 'off' ? 'all' : policy.repeat === 'all' ? 'one' : 'off'})}>{t(policy.repeat === 'off' ? 'player.opt.repeatOff' : policy.repeat === 'one' ? 'player.opt.repeatOne' : 'player.opt.repeatAll')}</Button>
    </div>}
    {!snapshot.entries.length && <StateView title={t('player.queueEmpty')}/>}
    <div role="list">{snapshot.entries.map(entry => <div role="listitem" className={styles.queueRow} data-current={entry.playing || undefined} key={entry.entryId}>
      <span className={styles.queueCopy}><b>{entry.hidden || !entry.title ? t('player.queueUnavailable') : entry.title}</b>{entry.playing && <small>{t('player.queueNowPlaying')}</small>}</span>
      {!entry.playing && !entry.hidden && !entry.pending && <div className={styles.actions}>
        <Button size="small" variant="ghost" icon="i-up" aria-label={t('player.queueMoveUp')} disabled={busy || !entry.canMoveUp} onClick={() => onEdit(snapshot, entry.entryId, -1)}/>
        <Button size="small" variant="ghost" icon="i-down" aria-label={t('player.queueMoveDown')} disabled={busy || !entry.canMoveDown} onClick={() => onEdit(snapshot, entry.entryId, 1)}/>
        <Button size="small" variant="ghost" icon="i-x" aria-label={t('player.queueRemove')} disabled={busy || !entry.canRemove} onClick={() => onEdit(snapshot, entry.entryId, 'remove')}/>
      </div>}
    </div>)}</div>
    <footer className={styles.footer}>
      {snapshot.more !== null && snapshot.more > 0 && <span>{t('player.queueMore', {count: snapshot.more})}</span>}
      {onMore && <Button size="small" variant="glass" disabled={busy} onClick={() => onMore(snapshot)}>{t('next.live.more')}</Button>}
      {save && <Button size="small" variant="glass" disabled={busy || saving} onClick={() => save.openDialog(snapshot)}>{t('player.queueSave')}</Button>}
    </footer>
    {save && <Dialog open={save.open} onOpenChange={open => {if (!open) save.close();}} title={t('player.queueSave')} width={440}
      footer={<><Button variant="glass" onClick={save.close}>{t('action.cancel')}</Button><Button variant="primary" loading={saving} disabled={busy || !save.name.trim() || save.progress !== null}
        onClick={() => save.confirm(snapshot, save.name.trim())}>{t('action.save')}</Button></>}>
      <TextField label={t('player.queueSaveName')} value={save.name} disabled={saving || save.progress !== null} onChange={event => save.changeName(event.currentTarget.value)}/>
      {message && <p role="status">{t(message.id, message.values)}</p>}
    </Dialog>}
  </section>;
}
