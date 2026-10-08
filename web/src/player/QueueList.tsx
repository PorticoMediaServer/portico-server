import {useEffect, useState, useSyncExternalStore} from 'react';
import type {MediaItem} from '@core/index.ts';
import type {QueueController as QueuePlayer} from '@core/queue-controller.ts';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {Button, IconButton, Input, Text} from '../ui';
import {queueSaveMessage, type QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import s from './Player.module.css';

/** Rows shown at once: the queue can hold 1,000 entries; the panel shows what's next. */
const VISIBLE = 50;

/**
 * WEB-PLAYER-02: the queue, inside the player's options (same place as Audio,
 * Speed and Sleep, so the player layout doesn't change). Now playing, then up
 * next: play any entry, move it up or down, or remove it. Edits go through the
 * queue service (revision-fenced, journaled), exactly like "Play next".
 */
export function QueueList({queue}: {queue: QueuePlayer}) {
  const i18n = useI18n();
  const {api} = useSession();
  const service = queue.workspace.service;
  const state = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const q = state.queue;
  const entries = (q?.entries ?? []).filter(e => !e.removed);
  const current = q ? entries.findIndex(e => e.id === q.currentEntryId) : -1;
  const shown = entries.slice(Math.max(0, current), Math.max(0, current) + VISIBLE);
  const ids = shown.map(e => (e.hidden ? '' : e.itemId)).filter(Boolean).join(',');
  const [titles, setTitles] = useState<Record<string, MediaItem | null>>({});
  useEffect(() => {
    let live = true;
    for (const id of ids ? ids.split(',') : []) {
      if (id in titles) continue;
      api.item(id).then(item => { if (live) setTitles(prev => ({...prev, [id]: item})); }, () => { if (live) setTitles(prev => ({...prev, [id]: null})); });
    }
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, ids]);
  if (!q || entries.length <= Math.max(0, current) + 1) return <Text variant="caption" tone="tertiary">{i18n.t('player.queueEmpty')}</Text>;
  const move = (entryId: string, by: -1 | 1) => {
    const order = entries.map(e => e.id);
    const at = order.indexOf(entryId), to = at + by;
    if (at < 0 || to < 0 || to >= order.length) return;
    [order[at], order[to]] = [order[to]!, order[at]!];
    void service.mutate({action: 'reorder', entryIds: order}).catch(() => {});
  };
  return (
    <div className={s.queue}>
      {shown.map((entry, i) => {
        const item = entry.hidden ? null : titles[entry.itemId];
        // A v1 placeholder still being snapshotted is quiet ("Still loading"), never "Unavailable".
        const pending = entry.hidden && 'pending' in entry && entry.pending;
        const title = item?.title ?? (pending ? i18n.t('web.queue.pendingEntry') : entry.hidden || item === null ? i18n.t('player.queueUnavailable') : '…');
        const now = entry.id === q.currentEntryId;
        const index = entries.indexOf(entry);
        return (
          <div key={entry.id} className={s.queueRow} aria-current={now || undefined}>
            <button type="button" className={s.option} disabled={now || entry.hidden} onClick={() => void queue.playEntry(q.id, entry.id, q.revision).catch(() => {})} aria-label={i18n.t('player.queuePlay', {title})}>
              <span>{title}</span>
              {now ? <span className={s.optionMeta}>{i18n.t('player.queueNowPlaying')}</span> : null}
            </button>
            {!now ? (
              <span className={s.queueActions}>
                <IconButton name="chevronUp" size="sm" variant="ghost" label={i18n.t('player.queueMoveUp')} disabled={index <= Math.max(0, current) + 1} onClick={() => move(entry.id, -1)} />
                <IconButton name="chevronDown" size="sm" variant="ghost" label={i18n.t('player.queueMoveDown')} disabled={index >= entries.length - 1} onClick={() => move(entry.id, 1)} />
                <IconButton name="close" size="sm" variant="ghost" label={i18n.t('player.queueRemove')} onClick={() => void service.mutate({action: 'remove', entryId: entry.id}).catch(() => {})} />
              </span>
            ) : null}
            {i === shown.length - 1 && entries.length > Math.max(0, current) + VISIBLE ? <Text variant="caption" tone="tertiary">{i18n.t('player.queueMore', {count: entries.length - Math.max(0, current) - VISIBLE})}</Text> : null}
          </div>
        );
      })}
      {queue.saveAsPlaylist ? <SaveQueue queue={queue} queueId={q.id} /> : null}
    </div>
  );
}

/** NEW-37: Save as playlist. A long queue is copied in the background; progress shows until it's done. */
function SaveQueue({queue, queueId}: {queue: QueuePlayer; queueId: string}) {
  const i18n = useI18n();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState('');
  const [save, setSave] = useState<QueuePlaylistSave | null>(null);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const start = () => {
    const title = name.trim() || i18n.t('player.queueSaveDefaultName');
    setBusy(true); setFailed(false); setSave(null); setNaming(false);
    queue.saveAsPlaylist!(queueId, title, setSave).catch(() => setFailed(true)).finally(() => setBusy(false));
  };
  const message = failed ? i18n.t('player.queueSaveFailed') : save ? (() => { const m = queueSaveMessage(save); return m.values ? i18n.t(m.id, {done: m.values.done.toLocaleString(), total: m.values.total.toLocaleString()}) : i18n.t(m.id); })() : busy ? i18n.t('player.queueSaving') : '';
  return (
    <div style={{display: 'grid', gap: 8, padding: '8px 12px'}}>
      {naming ? (
        <>
          <Input label={i18n.t('player.queueSaveName')} value={name} placeholder={i18n.t('player.queueSaveDefaultName')} maxLength={200} autoFocus onChange={e => setName(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') start(); }} />
          <div style={{display: 'flex', gap: 8}}><Button size="sm" variant="primary" label={i18n.t('player.queueSave')} onClick={start} /><Button size="sm" variant="ghost" label={i18n.t('action.cancel')} onClick={() => setNaming(false)} /></div>
        </>
      ) : <div><Button size="sm" variant="secondary" icon="plus" label={i18n.t('player.queueSave')} loading={busy} disabled={busy} onClick={() => setNaming(true)} /></div>}
      {message ? <Text variant="caption" tone={failed || save?.state === 'failed' ? 'danger' : 'tertiary'} role="status">{message}</Text> : null}
    </div>
  );
}
