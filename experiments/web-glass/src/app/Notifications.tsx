import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { BellOff, CheckCheck, Download, HardDrive, Radio, Share2, ShieldAlert, Sparkles, UserPlus } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { Notification } from '../data/types';
import { useNotifications } from '../services/queries';
import { api } from '../services/client';
import { useUi } from '../state/ui';
import { ago, useIsMobile } from '../lib/hooks';
import { Sheet } from '../components/overlay';
import { Button, EmptyState, Skel, cls } from '../components/ui';

const ICON: Record<Notification['kind'], React.ReactNode> = {
  'new-episode': <Sparkles />,
  recording: <Radio />,
  download: <Download />,
  share: <Share2 />,
  server: <HardDrive />,
  security: <ShieldAlert />,
  request: <UserPlus />,
};

function List({ onNavigate }: { onNavigate: () => void }) {
  const notes = useNotifications();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const markRead = async (ids?: string[]) => {
    await api.markNotificationsRead(ids);
    qc.invalidateQueries({ queryKey: ['notifications'] });
  };
  if (notes.isLoading)
    return (
      <div className="notes">
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="note">
            <Skel w={40} h={40} r={12} />
            <div style={{ flex: 1, display: 'grid', gap: 6 }}>
              <Skel h={12} w="60%" />
              <Skel h={10} w="90%" />
            </div>
          </div>
        ))}
      </div>
    );
  const list = notes.data ?? [];
  if (!list.length) return <EmptyState icon={<BellOff />} title="You’re all caught up" body="New episodes, finished recordings and messages from Harbor will show up here." />;
  const unread = list.filter((n) => !n.read);
  return (
    <div className="notes">
      {unread.length > 0 && (
        <div className="notes-bar">
          <span>{unread.length} new</span>
          <Button size="sm" variant="ghost" icon={<CheckCheck />} onClick={() => markRead()}>
            Mark all read
          </Button>
        </div>
      )}
      {list.map((n) => (
        <button
          key={n.id}
          className={cls('note', !n.read && 'unread')}
          onClick={() => {
            markRead([n.id]);
            onNavigate();
            if (n.href) navigate(n.href, { viewTransition: true });
          }}
        >
          {n.image ? <img className="note-img" src={n.image} alt="" loading="lazy" /> : <span className={cls('note-icon', n.kind)}>{ICON[n.kind]}</span>}
          <span className="note-main">
            <span className="note-title">{n.title}</span>
            <span className="note-body">{n.body}</span>
            <span className="note-time">{ago(n.at)}</span>
          </span>
        </button>
      ))}
    </div>
  );
}

export function NotificationsSheet() {
  const open = useUi((s) => s.notifications);
  const set = useUi((s) => s.setNotifications);
  const mobile = useIsMobile();
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open || mobile) return;
    const onDown = (e: PointerEvent) => {
      if (!panel.current?.contains(e.target as Node) && !(e.target as HTMLElement).closest('[aria-label^="Notifications"]')) set(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && set(false);
    document.addEventListener('pointerdown', onDown, true);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('pointerdown', onDown, true);
      document.removeEventListener('keydown', onKey);
    };
  }, [open, mobile, set]);
  if (mobile)
    return (
      <Sheet open={open} onClose={() => set(false)} title="Notifications" size="lg">
        <List onNavigate={() => set(false)} />
      </Sheet>
    );
  if (!open) return null;
  return createPortal(
    <div ref={panel} className="notes-panel glass glass-thick" role="dialog" aria-label="Notifications">
      <div className="notes-head">Notifications</div>
      <List onNavigate={() => set(false)} />
    </div>,
    document.body,
  );
}
