import { useState } from 'react';
import { Link } from 'react-router';
import { AlertTriangle, CalendarClock, CircleDot, Film, Pencil, Play, Trash2, Tv } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { Recording, SeriesRule } from '../../data/types';
import { useDvr } from '../../services/queries';
import { api } from '../../services/client';
import { PageHeader } from '../../components/PageHeader';
import { Button, EmptyState, ErrorState, Progress, Segmented, Skel, Switch, cls } from '../../components/ui';
import { Confirm, Sheet, toast } from '../../components/overlay';
import { LiveTabs } from './Live';
import { ago, clockTime, fmtBytes, fmtDuration } from '../../lib/hooks';

export function DvrPage() {
  const q = useDvr();
  const qc = useQueryClient();
  const [tab, setTab] = useState<'recordings' | 'scheduled' | 'rules'>('recordings');
  const [del, setDel] = useState<Recording | null>(null);
  const [rule, setRule] = useState<SeriesRule | null>(null);
  const d = q.data;
  const refresh = () => qc.invalidateQueries({ queryKey: ['dvr'] });
  const byDay = new Map<string, NonNullable<typeof d>['scheduled']>();
  for (const s of d?.scheduled ?? []) {
    const k = new Date(s.start).toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' });
    if (!byDay.has(k)) byDay.set(k, []);
    byDay.get(k)!.push(s);
  }
  return (
    <div className="page">
      <PageHeader title="Recordings" subtitle={d ? `${d.recordings.length} recordings · ${fmtBytes(d.diskUsedBytes)} of ${fmtBytes(d.diskLimitBytes)} DVR space` : undefined} actions={<LiveTabs value="dvr" />}>
        <div className="m-seg">
          <LiveTabs value="dvr" />
        </div>
      </PageHeader>
      <div className="page-body pad">
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} />}
        {d && (
          <div className="dvr-disk">
            <div className="meter">
              <i style={{ width: `${(d.diskUsedBytes / d.diskLimitBytes) * 100}%`, background: 'var(--blue-400)' }} />
            </div>
            <span className="t-sub tnum">
              {fmtBytes(d.diskLimitBytes - d.diskUsedBytes)} free · about {Math.round((d.diskLimitBytes - d.diskUsedBytes) / 6e9)} hours of HD · oldest watched recordings are removed after 7 days
            </span>
          </div>
        )}
        <div className="dvr-tabs">
          <Segmented
            value={tab}
            onChange={setTab}
            options={[
              { value: 'recordings', label: 'Recorded' },
              { value: 'scheduled', label: `Upcoming${d ? ` · ${d.scheduled.length}` : ''}` },
              { value: 'rules', label: 'Series Rules' },
            ]}
          />
        </div>
        {!d && (
          <div className="rec-grid">
            {Array.from({ length: 6 }, (_, i) => (
              <Skel key={i} style={{ aspectRatio: '16 / 9' }} />
            ))}
          </div>
        )}
        {d && tab === 'recordings' && (
          <div className="rec-grid">
            {d.recordings.map((r) => (
              <div key={r.id} className={cls('rec', r.status)}>
                <Link to={r.status === 'failed' ? '#' : `/watch/${r.status === 'recording' ? 'ch-5' : r.id}`} className="rec-art">
                  <img src={r.image} alt="" loading="lazy" />
                  {r.status === 'recording' && (
                    <span className="live-chip">
                      <CircleDot /> REC
                    </span>
                  )}
                  {r.status !== 'failed' && (
                    <span className="continue-play glass glass-clear rec-play">
                      <Play fill="currentColor" />
                    </span>
                  )}
                  {r.progress && <Progress value={r.progress} className="card-progress" />}
                </Link>
                <div className="rec-text">
                  <div className="card-title ellipsis">{r.title}</div>
                  <div className="card-sub ellipsis">
                    {r.episodeTitle ? `${r.episodeTitle} · ` : ''}
                    {r.channelName} · {r.status === 'recording' ? 'Recording now' : ago(r.recordedAt)}
                  </div>
                  <div className="rec-meta tnum">
                    {fmtDuration(r.duration)} · {fmtBytes(r.sizeBytes)}
                    {r.watched && ' · Watched'}
                  </div>
                  {r.failureReason && (
                    <div className={cls('rec-warn', r.status)}>
                      <AlertTriangle /> {r.failureReason}
                    </div>
                  )}
                </div>
                {r.status !== 'recording' && (
                  <button className="icon-btn sm plain rec-del" aria-label={`Delete ${r.title}`} onClick={() => setDel(r)}>
                    <Trash2 />
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
        {d && tab === 'scheduled' &&
          (d.scheduled.length === 0 ? (
            <EmptyState icon={<CalendarClock />} title="Nothing scheduled" body="Record a program or a whole series from the guide." actions={<Button to="/live" variant="accent">Open Guide</Button>} />
          ) : (
            <div className="sched">
              {[...byDay.entries()].map(([day, list]) => (
                <section key={day}>
                  <h3 className="group-title">{day}</h3>
                  <div className="group">
                    {list.map((s) => (
                      <div key={s.id} className="row">
                        <span className="sched-time tnum">{clockTime(s.start)}</span>
                        <span className="row-main">
                          <span className="row-title">
                            {s.title}
                            {s.episodeTitle ? ` · ${s.episodeTitle}` : ''}
                          </span>
                          <span className="row-sub">
                            {s.channelName} · until {clockTime(s.end)}
                            {s.ruleId ? ' · from series rule' : ''}
                          </span>
                          {s.conflict && (
                            <span className="rec-warn partial">
                              <AlertTriangle /> {s.conflict}
                            </span>
                          )}
                        </span>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={async () => {
                            await api.cancelRecording(s.id);
                            refresh();
                            qc.invalidateQueries({ queryKey: ['guide'] });
                            toast(`Skipped ${s.title} at ${clockTime(s.start)}`);
                          }}
                        >
                          Skip
                        </Button>
                      </div>
                    ))}
                  </div>
                </section>
              ))}
            </div>
          ))}
        {d && tab === 'rules' && (
          <div className="rules">
            {d.rules.map((r) => (
              <div key={r.id} className="rule">
                <img src={r.image} alt="" />
                <div className="rule-main">
                  <div className="card-title">{r.title}</div>
                  <div className="card-sub">
                    {r.newOnly ? 'New episodes only' : 'All airings'} · {r.channels === 'any' ? 'Any channel' : `${r.channels.length} channel`} · Keep {r.keep === 'all' ? 'all' : `latest ${r.keep}`}
                  </div>
                  <div className="card-sub tnum">
                    Priority {r.priority} · Padding −{r.paddingBefore}/+{r.paddingAfter} min · {r.upcoming} upcoming
                  </div>
                </div>
                <Button size="sm" variant="plain" icon={<Pencil />} onClick={() => setRule(r)}>
                  Edit
                </Button>
              </div>
            ))}
            <p className="group-foot">Higher-priority rules win when there aren’t enough tuners. Drag to reorder is available in the Server Console → Live TV.</p>
          </div>
        )}
      </div>
      <Confirm
        open={!!del}
        onClose={() => setDel(null)}
        danger
        title={`Delete “${del?.title}”?`}
        body={`This frees ${del ? fmtBytes(del.sizeBytes) : ''} on Harbor.`}
        confirmLabel="Delete Recording"
        onConfirm={async () => {
          if (!del) return;
          await api.deleteRecording(del.id);
          refresh();
          toast('Recording deleted', <Trash2 />);
        }}
      />
      {rule && <RuleSheet rule={rule} onClose={() => setRule(null)} />}
    </div>
  );
}

function RuleSheet({ rule, onClose }: { rule: SeriesRule; onClose: () => void }) {
  const [newOnly, setNewOnly] = useState(rule.newOnly);
  const [keep, setKeep] = useState(String(rule.keep));
  const [before, setBefore] = useState(rule.paddingBefore);
  const [after, setAfter] = useState(rule.paddingAfter);
  return (
    <Sheet
      open
      onClose={onClose}
      title={`Series Rule · ${rule.title}`}
      size="sm"
      footer={
        <>
          <Button variant="danger" icon={<Trash2 />} onClick={() => (toast(`Stopped recording ${rule.title}`), onClose())}>
            Delete Rule
          </Button>
          <span style={{ flex: 1 }} />
          <Button variant="accent" onClick={() => (toast('Series rule saved', <Tv />), onClose())}>
            Save
          </Button>
        </>
      }
    >
      <div className="form-stack">
        <div className="sheet-row" style={{ marginTop: 0 }}>
          <div>
            <div className="row-title">New episodes only</div>
            <div className="row-sub">Skip reruns and repeats</div>
          </div>
          <Switch checked={newOnly} onChange={setNewOnly} label="New episodes only" />
        </div>
        <div className="field">
          <label>Keep</label>
          <select className="input" value={keep} onChange={(e) => setKeep(e.target.value)}>
            <option value="all">All episodes</option>
            {[1, 3, 5, 10].map((n) => (
              <option key={n} value={n}>
                Latest {n}
              </option>
            ))}
          </select>
        </div>
        <div className="form-grid">
          <div className="field">
            <label>Start early</label>
            <select className="input" value={before} onChange={(e) => setBefore(Number(e.target.value))}>
              {[0, 1, 2, 5, 10].map((n) => (
                <option key={n} value={n}>
                  {n ? `${n} min` : 'On time'}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label>End late</label>
            <select className="input" value={after} onChange={(e) => setAfter(Number(e.target.value))}>
              {[0, 2, 5, 10, 30, 60].map((n) => (
                <option key={n} value={n}>
                  {n ? `${n} min` : 'On time'}
                </option>
              ))}
            </select>
          </div>
        </div>
        <p className="t-caption">
          <Film className="inline-ic" />
          Sports often run long: 30 minutes of end padding is a safe default for live games.
        </p>
      </div>
    </Sheet>
  );
}
