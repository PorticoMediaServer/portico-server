import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { ChevronLeft, ChevronRight, CircleDot, Clock, Play, Radio, Star, Tv } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { Channel, Program } from '../../data/types';
import { useDvr, useGuide } from '../../services/queries';
import { api } from '../../services/client';
import { PageHeader } from '../../components/PageHeader';
import { Button, ErrorState, IconButton, Segmented, Skel, cls } from '../../components/ui';
import { Sheet, toast } from '../../components/overlay';
import { clockTime, useIsMobile, useNow } from '../../lib/hooks';

const CATS = ['All', 'Favorites', 'Local', 'News', 'Sports', 'Movies', 'Kids', 'Documentary', 'Entertainment', 'Food', 'Music'];

export function ChannelLogo({ ch, size = 44 }: { ch: Pick<Channel, 'logoColor' | 'logoText' | 'name'>; size?: number }) {
  return (
    <span className="ch-logo" style={{ ['--c' as string]: ch.logoColor, width: size, height: size * 0.7 }} aria-label={ch.name}>
      {ch.logoText}
    </span>
  );
}

export function LiveTabs({ value }: { value: 'guide' | 'dvr' }) {
  const navigate = useNavigate();
  return (
    <Segmented
      value={value}
      onChange={(v) => navigate(v === 'guide' ? '/live' : '/live/dvr', { viewTransition: true })}
      options={[
        { value: 'guide', label: 'Guide' },
        { value: 'dvr', label: 'Recordings' },
      ]}
    />
  );
}

export function LivePage() {
  const q = useGuide();
  const dvr = useDvr();
  const qc = useQueryClient();
  const [params] = useSearchParams();
  const now = useNow(30_000);
  const mobile = useIsMobile();
  const navigate = useNavigate();
  const [cat, setCat] = useState('All');
  const [sel, setSel] = useState<Program | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const PX = mobile ? 4.4 : 6.2; // px per minute
  const COL = mobile ? 76 : 210;

  const data = q.data;
  const channels = useMemo(() => {
    if (!data) return [];
    return data.channels.filter((c) => cat === 'All' || (cat === 'Favorites' ? c.favorite : c.category === cat));
  }, [data, cat]);
  const scheduled = new Set(data?.scheduledProgramIds ?? []);

  useEffect(() => {
    const pid = params.get('program');
    const p = data?.programs.find((x) => x.id === pid);
    if (p) setSel(p);
  }, [params, data]);

  const scrollToNow = (smooth = true) => {
    const el = scroller.current;
    if (!el || !data) return;
    const x = ((Date.now() - data.start) / 60000) * PX - 60 * PX * 0.5;
    el.scrollTo({ left: x, behavior: smooth ? 'smooth' : 'auto' });
  };
  useEffect(() => {
    scrollToNow(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, mobile]);

  const onNow = useMemo(() => {
    if (!data) return null;
    const pick = data.programs.find((p) => p.isLive && Date.parse(p.start) <= now && Date.parse(p.end) > now) ?? data.programs.find((p) => Date.parse(p.start) <= now && Date.parse(p.end) > now);
    return pick ? { p: pick, ch: data.channels.find((c) => c.id === pick.channelId)! } : null;
  }, [data, now]);

  const record = async (p: Program, series: boolean) => {
    await api.scheduleRecording(p.id, series);
    await qc.invalidateQueries({ queryKey: ['guide'] });
    await qc.invalidateQueries({ queryKey: ['dvr'] });
    toast(series ? `Recording every new episode of ${p.title}` : `Recording ${p.title} at ${clockTime(p.start)}`, <CircleDot />);
    setSel(null);
  };
  const cancel = async (p: Program) => {
    await api.cancelRecording(p.id);
    await qc.invalidateQueries({ queryKey: ['guide'] });
    await qc.invalidateQueries({ queryKey: ['dvr'] });
    toast(`Cancelled recording of ${p.title}`);
    setSel(null);
  };

  const hours = data ? Array.from({ length: data.hours * 2 }, (_, i) => data.start + i * 1800_000) : [];
  const width = data ? data.hours * 60 * PX : 0;
  const nowX = data ? ((now - data.start) / 60000) * PX : 0;
  const tunersFree = 4 - (dvr.data?.recordings.filter((r) => r.status === 'recording').length ?? 0) - 1;

  return (
    <div className="page live-page">
      <PageHeader title="Live TV" subtitle={data ? `${data.channels.length} channels · HDHomeRun FLEX 4K + IPTV · ${tunersFree} of 4 tuners free` : undefined} actions={<LiveTabs value="guide" />}>
        <div className="m-seg">
          <LiveTabs value="guide" />
        </div>
      </PageHeader>
      <div className="page-body">
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} title="Couldn’t load the guide" />}
        {onNow && (
          <div className="pad">
            <div className="onnow" style={{ ['--c' as string]: onNow.ch.logoColor }}>
              <div className="onnow-media">
                <img src={onNow.p.image} alt="" />
                <span className="live-chip">
                  <Radio /> LIVE
                </span>
              </div>
              <div className="onnow-text">
                <span className="t-eyebrow">
                  On now · {onNow.ch.number} {onNow.ch.name}
                </span>
                <h2 className="onnow-title">{onNow.p.title}</h2>
                <p className="t-body clamp-2">{onNow.p.summary}</p>
                <div className="onnow-time tnum">
                  {clockTime(onNow.p.start)} – {clockTime(onNow.p.end)} · {Math.round((Date.parse(onNow.p.end) - now) / 60000)} min left
                </div>
                <div className="onnow-actions">
                  <Button variant="primary" icon={<Play fill="currentColor" />} onClick={() => navigate(`/watch/${onNow.ch.id}`)}>
                    Watch Live
                  </Button>
                  <Button variant="glass" icon={<CircleDot />} onClick={() => setSel(onNow.p)}>
                    Record
                  </Button>
                </div>
              </div>
            </div>
          </div>
        )}
        <div className="guide-bar pad">
          <div className="chip-scroll no-scrollbar">
            {CATS.filter((c) => c === 'All' || c === 'Favorites' || data?.channels.some((x) => x.category === c)).map((c) => (
              <button key={c} className={cls('chip', cat === c && 'on')} onClick={() => setCat(c)}>
                {c === 'Favorites' && <Star />}
                {c}
              </button>
            ))}
          </div>
          <div className="guide-nav">
            <IconButton label="Earlier" variant="plain" size="sm" onClick={() => scroller.current?.scrollBy({ left: -180 * PX, behavior: 'smooth' })}>
              <ChevronLeft />
            </IconButton>
            <Button size="sm" variant="plain" icon={<Clock />} onClick={() => scrollToNow()}>
              Now
            </Button>
            <IconButton label="Later" variant="plain" size="sm" onClick={() => scroller.current?.scrollBy({ left: 180 * PX, behavior: 'smooth' })}>
              <ChevronRight />
            </IconButton>
          </div>
        </div>
        <div className="guide" style={{ ['--col' as string]: `${COL}px` }}>
          {!data ? (
            <div className="pad" style={{ display: 'grid', gap: 8 }}>
              {Array.from({ length: 8 }, (_, i) => (
                <Skel key={i} h={64} r={12} />
              ))}
            </div>
          ) : (
            <div className="guide-scroll no-scrollbar" ref={scroller}>
              <div className="guide-inner" style={{ width: width + COL }}>
                <div className="guide-times">
                  <div className="guide-corner">{new Date(now).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}</div>
                  {hours.map((t) => (
                    <div key={t} className="guide-time" style={{ left: COL + ((t - data.start) / 60000) * PX, width: 30 * PX }}>
                      {clockTime(t)}
                    </div>
                  ))}
                </div>
                <div className="guide-now" style={{ left: COL + nowX }} aria-hidden="true">
                  <span>{clockTime(now)}</span>
                </div>
                {channels.map((ch) => (
                  <div key={ch.id} className="guide-row">
                    <Link to={`/watch/${ch.id}`} className="guide-ch" aria-label={`Watch ${ch.name}`}>
                      <ChannelLogo ch={ch} size={mobile ? 44 : 50} />
                      {!mobile && (
                        <span className="guide-ch-text">
                          <span className="guide-ch-num tnum">{ch.number}</span>
                          <span className="guide-ch-name ellipsis">{ch.name}</span>
                        </span>
                      )}
                      {ch.favorite && !mobile && <Star className="guide-fav" fill="currentColor" />}
                    </Link>
                    <div className="guide-progs">
                      {data.programs
                        .filter((p) => p.channelId === ch.id)
                        .map((p) => {
                          const s = Date.parse(p.start);
                          const e = Date.parse(p.end);
                          const left = Math.max(0, ((s - data.start) / 60000) * PX);
                          const right = ((e - data.start) / 60000) * PX;
                          if (right <= 0) return null;
                          const past = e < now;
                          const live = s <= now && e > now;
                          return (
                            <button
                              key={p.id}
                              className={cls('prog', past && 'past', live && 'now', `c-${p.category.toLowerCase()}`)}
                              style={{ left, width: right - left - 4 }}
                              onClick={() => setSel(p)}
                            >
                              <span className="prog-title ellipsis">
                                {scheduled.has(p.id) && <i className="rec-dot" aria-label="Scheduled to record" />}
                                {p.title}
                              </span>
                              <span className="prog-sub ellipsis">
                                {p.isLive && <b className="prog-live">LIVE</b>}
                                {p.isNew && <b className="prog-new">NEW</b>}
                                {p.episodeTitle ? `${p.episodeTitle} · ` : ''}
                                {clockTime(p.start)}
                              </span>
                              {live && <i className="prog-bar" style={{ width: `${((now - s) / (e - s)) * 100}%` }} />}
                            </button>
                          );
                        })}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
      {sel && data && (
        <ProgramSheet
          p={sel}
          ch={data.channels.find((c) => c.id === sel.channelId)!}
          scheduled={scheduled.has(sel.id)}
          onClose={() => setSel(null)}
          onRecord={(series) => record(sel, series)}
          onCancel={() => cancel(sel)}
          onWatch={() => navigate(`/watch/${sel.channelId}`)}
        />
      )}
    </div>
  );
}

function ProgramSheet({ p, ch, scheduled, onClose, onRecord, onCancel, onWatch }: { p: Program; ch: Channel; scheduled: boolean; onClose: () => void; onRecord: (series: boolean) => void; onCancel: () => void; onWatch: () => void }) {
  const now = Date.now();
  const airing = Date.parse(p.start) <= now && Date.parse(p.end) > now;
  const past = Date.parse(p.end) <= now;
  return (
    <Sheet open onClose={onClose} title={p.title} size="md">
      <div className="psheet">
        <img className="psheet-img" src={p.image} alt="" />
        <div className="psheet-meta">
          <ChannelLogo ch={ch} size={40} />
          <div>
            <div className="t-headline">
              {ch.number} {ch.name}
            </div>
            <div className="t-sub tnum">
              {new Date(p.start).toLocaleDateString('en-US', { weekday: 'long' })} · {clockTime(p.start)} – {clockTime(p.end)} · {Math.round((Date.parse(p.end) - Date.parse(p.start)) / 60000)} min
            </div>
          </div>
        </div>
        {p.episodeTitle && <div className="t-headline">“{p.episodeTitle}”</div>}
        <p className="t-body">{p.summary}</p>
        <div className="psheet-badges">
          <span className="badge">{p.category}</span>
          {p.rating && <span className="badge">{p.rating}</span>}
          {p.isNew && <span className="badge new">New</span>}
          {p.isLive && <span className="badge live">Live</span>}
          {ch.hd && <span className="badge">HD</span>}
          {scheduled && (
            <span className="badge red">
              <CircleDot className="inline-ic" style={{ width: 11, height: 11 }} />
              Scheduled
            </span>
          )}
        </div>
        <div className="psheet-actions">
          {airing && (
            <Button variant="primary" icon={<Play fill="currentColor" />} onClick={onWatch}>
              Watch Live
            </Button>
          )}
          {!past &&
            (scheduled ? (
              <Button variant="danger" onClick={onCancel}>
                Cancel Recording
              </Button>
            ) : (
              <>
                <Button variant={airing ? 'glass' : 'accent'} icon={<CircleDot />} onClick={() => onRecord(false)}>
                  {airing ? 'Record Rest of Show' : 'Record Episode'}
                </Button>
                {p.seriesId && (
                  <Button variant="glass" icon={<Tv />} onClick={() => onRecord(true)}>
                    Record Series
                  </Button>
                )}
              </>
            ))}
          {past && <span className="t-sub">This program has ended. Look for it in Recordings if it was recorded.</span>}
        </div>
      </div>
    </Sheet>
  );
}
