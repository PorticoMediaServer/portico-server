import {useState} from 'react';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {NotificationItem} from '../../data/types';
import {Panel, Row} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Segmented, Switch} from '../../ui/Controls';
import {Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {Menu} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {relativeTime} from '../shared/items';

const KIND_ICON: Record<NotificationItem['kind'], IconName> = {info: 'info', notice: 'message', urgent: 'alert', security: 'shield', server: 'server'};
const PAGE = 5;

/** The notifications inbox: all, unread or archived; read, archive, mark all read; and what to be told about. */
export function NotificationsInbox() {
  const services = useServices();
  const version = useDataVersion();
  const list = useQuery(`notifications.${version}`, () => services.account.notifications());
  const prefs = useQuery(`prefs.${version}`, () => services.preferences.get());
  const [view, setView] = useState<'all' | 'unread' | 'archived'>('all');
  const [shown, setShown] = useState(PAGE);
  const items = (list.data ?? []).filter(n => (view === 'archived' ? n.archived : !n.archived && (view === 'all' || !n.read)));
  const unread = (list.data ?? []).filter(n => !n.read && !n.archived).length;
  const upd = (n: NotificationItem, patch: Partial<NotificationItem>, msg?: string) => void services.account.updateNotification(n.id, patch).then(() => msg && toast(msg, {action: patch.archived ? {label: 'Undo', onClick: () => void services.account.updateNotification(n.id, {archived: false})} : undefined}));
  return (
    <div className="stack">
      <div className="toolbar" style={{marginBottom: 0}}>
        <Segmented label="Show" value={view} onChange={v => { setView(v); setShown(PAGE); }} options={[{value: 'all', label: 'All'}, {value: 'unread', label: 'Unread', count: unread || undefined}, {value: 'archived', label: 'Archived'}]} />
        <span className="spacer" />
        <Button variant="ghost" icon="checkAll" label="Mark all read" disabled={!unread} onClick={() => void services.account.markAllRead().then(() => toast('All caught up'))} />
      </div>
      {!list.data ? <div className="stack">{[0, 1, 2].map(i => <Skeleton key={i} h={76} />)}</div> : !items.length ? (
        <StateView compact icon={view === 'archived' ? 'archive' : 'bell'} title={view === 'unread' ? 'You’re all caught up' : view === 'archived' ? 'Nothing archived' : 'No notifications'} body={view === 'all' ? 'New episodes, finished recordings and messages from your server owner appear here.' : undefined} />
      ) : (
        <ul className="inbox">
          {items.slice(0, shown).map(n => (
            <li key={n.id} className={cx('note', !n.read && 'is-unread', `is-${n.kind}`)}>
              <span className="note-icon"><Icon name={KIND_ICON[n.kind]} size={18} /></span>
              <div className="note-text">
                <strong>{n.title}</strong>
                <p>{n.body}</p>
                <span className="faint small">{relativeTime(n.at)}{n.audience === 'admin' ? ' · For server admins' : ''}</span>
              </div>
              {!n.read ? <span className="note-dot" aria-label="Unread" /> : null}
              <Menu label="Notification options" trigger={<button type="button" className="track-more is-shown" aria-label={`Options for ${n.title}`}><Icon name="more" size={18} /></button>} items={[
                {id: 'read', label: n.read ? 'Mark unread' : 'Mark read', icon: n.read ? 'bell' : 'check', onSelect: () => upd(n, {read: !n.read})},
                {id: 'archive', label: n.archived ? 'Move to inbox' : 'Archive', icon: 'archive', onSelect: () => upd(n, {archived: !n.archived, read: true}, n.archived ? 'Moved to your inbox' : 'Archived')},
              ]} />
            </li>
          ))}
        </ul>
      )}
      {items.length > shown ? <Button variant="secondary" label="Load more" onClick={() => setShown(s => s + PAGE)} /> : null}
      <Panel title="Tell me about" padded={false}>
        {[['notifications.newEpisodes', 'New episodes of shows I watch'], ['notifications.recordings', 'Finished and failed recordings'], ['notifications.downloads', 'Downloads that are ready'], ['notifications.together', 'Watch Together invitations'], ['notifications.server', 'Messages from the server owner']].map(([k, l]) => (
          <Row key={k} label={l}>{prefs.data ? <Switch checked={!!prefs.data[k!]} onChange={v => void services.preferences.set({[k!]: v})} label={l} /> : <Skeleton w={44} h={26} r={13} />}</Row>
        ))}
      </Panel>
    </div>
  );
}
