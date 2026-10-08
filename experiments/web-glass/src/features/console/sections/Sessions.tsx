import { useState } from 'react';
import { Cpu, Globe, Home, MessageSquare, Pause, Play, StopCircle } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { Session } from '../../../services/api';
import { useSessions } from '../../../services/queries';
import { api } from '../../../services/client';
import { Avatar, Button, EmptyState, ErrorState, Skel, cls } from '../../../components/ui';
import { Sheet, toast } from '../../../components/overlay';
import { fmtClock } from '../../../lib/hooks';
import { Bar, KV } from '../ui';

const COLORS: Record<string, string> = { maya: '#409fd4', daniel: '#f0a02a', noor: '#e25aa8', ingrid: '#c9a86a', sam: '#8a5cf0' };

export function SessionCard({ s, onOpen }: { s: Session; onOpen?: () => void }) {
  const pct = s.position / s.duration;
  return (
    <button className="sess" onClick={onOpen} style={{ cursor: onOpen ? 'pointer' : 'default' }}>
      <div className="sess-art">
        <img src={s.image} alt="" loading="lazy" />
        <span className={cls('sess-state', s.state)}>{s.state === 'paused' ? <Pause /> : <Play />}</span>
      </div>
      <div className="sess-main">
        <div className="sess-title ellipsis">{s.title}</div>
        <div className="sess-sub ellipsis">{s.subtitle}</div>
        <div className="sess-who">
          <Avatar name={s.user} color={COLORS[s.userId]} size={20} />
          <span className="ellipsis">
            {s.user} · {s.device}
          </span>
          {s.local ? <Home className="sess-ic" /> : <Globe className="sess-ic" />}
        </div>
        <div className="sess-prog">
          <Bar value={pct} />
          <span className="tnum">
            {fmtClock(s.position)} / {fmtClock(s.duration)}
          </span>
        </div>
        <div className="sess-tags">
          <span className={cls('ctag', s.decision === 'Transcode' ? 'warn' : s.decision === 'Direct Stream' ? 'info' : 'ok')}>{s.decision}</span>
          {s.video.hw && (
            <span className="ctag">
              <Cpu /> HW {s.video.speed}×
            </span>
          )}
          <span className="ctag tnum">{s.bandwidthMbps.toFixed(1)} Mbps</span>
        </div>
      </div>
    </button>
  );
}

export function Sessions() {
  const q = useSessions();
  const qc = useQueryClient();
  const [open, setOpen] = useState<Session | null>(null);
  const [msg, setMsg] = useState('Harbor is restarting for an update in 5 minutes.');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data)
    return (
      <div className="sess-grid">
        {Array.from({ length: 4 }, (_, i) => (
          <Skel key={i} h={150} r={16} />
        ))}
      </div>
    );
  const total = q.data.reduce((a, s) => a + s.bandwidthMbps, 0);
  return (
    <div className="cstack">
      <div className="cstats slim">
        <div className="cstat">
          <div className="cstat-label">Active</div>
          <div className="cstat-value tnum">{q.data.length}</div>
        </div>
        <div className="cstat">
          <div className="cstat-label">Transcoding</div>
          <div className="cstat-value tnum">{q.data.filter((s) => s.decision === 'Transcode').length}</div>
        </div>
        <div className="cstat">
          <div className="cstat-label">Bandwidth</div>
          <div className="cstat-value tnum">
            {total.toFixed(1)}
            <span className="cstat-unit"> Mbps</span>
          </div>
        </div>
        <div className="cstat">
          <div className="cstat-label">Remote</div>
          <div className="cstat-value tnum">{q.data.filter((s) => !s.local).length}</div>
        </div>
      </div>
      {q.data.length === 0 ? (
        <EmptyState icon={<Play />} title="Nothing is playing" body="Streams appear here the moment anyone presses play." />
      ) : (
        <div className="sess-grid">
          {q.data.map((s) => (
            <SessionCard key={s.id} s={s} onOpen={() => setOpen(s)} />
          ))}
        </div>
      )}
      {open && (
        <Sheet
          open
          onClose={() => setOpen(null)}
          title={`${open.user} · ${open.title}`}
          size="lg"
          footer={
            <>
              <input className="input sm" style={{ flex: 1 }} value={msg} onChange={(e) => setMsg(e.target.value)} aria-label="Message to viewer" />
              <Button size="sm" variant="plain" icon={<MessageSquare />} onClick={() => toast(`Message sent to ${open.device}`, <MessageSquare />)}>
                Send
              </Button>
              <Button
                size="sm"
                variant="danger"
                icon={<StopCircle />}
                onClick={async () => {
                  await api.terminateSession(open.id, msg);
                  qc.invalidateQueries({ queryKey: ['admin'] });
                  toast(`Stopped ${open.user}’s stream`, <StopCircle />);
                  setOpen(null);
                }}
              >
                Stop Stream
              </Button>
            </>
          }
        >
          <div className="sess-detail">
            {open.reason && (
              <div className="cbanner warn">
                <Cpu />
                <div>
                  <strong>Why it’s transcoding:</strong> {open.reason}
                </div>
              </div>
            )}
            <KV
              rows={[
                ['Decision', open.decision],
                ['Video', `${open.video.source} → ${open.video.target}${open.video.hw ? ` (hardware, ${open.video.speed}× realtime)` : ''}`],
                ['Audio', `${open.audio.source} → ${open.audio.target}`],
                ['Subtitles', open.subtitles ?? 'None'],
                ['Container', open.container],
                ['Quality', open.quality],
                ['Bandwidth', `${open.bandwidthMbps.toFixed(1)} Mbps`],
                ['Player', `${open.app} · ${open.platform}`],
                ['Address', `${open.ip} · ${open.location}`],
                ['Started', new Date(open.startedAt).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })],
              ]}
            />
          </div>
        </Sheet>
      )}
    </div>
  );
}
