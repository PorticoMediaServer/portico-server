import React, {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Channel, Program} from '@/data/types';
import {usePlayer} from '@/player/PlayerProvider';
import {clock, day} from '@/app/format';
import {useSession} from '@/app/session';
import {SOURCES, channelEntry} from '@/app/liveSources';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, EmptyState, ErrorState, Progress, Skeleton} from '@/ui/Bits';
import {Dialog, Menu, useToast} from '@/ui/Overlay';
import {Checkbox, RadioCards, Segmented, Select} from '@/ui/Form';
import {Icon} from '@/ui/Icon';
import {useNow, usePhone, useTitle} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {useRouteParams, useRouteSearch} from '../common';
import {Recordings} from './Recordings';
import {useDemo} from '@/app/demo';
import s from './Live.module.css';

const HOUR = 3600000;
const HALF = HOUR / 2;
const PX_PER_MIN = 5.2;


/**
 * Live TV. /channels is every channel's guide; /channels/:source narrows it
 * to one source (a tuner, a playlist, or Library Channels); /channels/recordings
 * is the DVR. ?view=list shows channels with now and next instead of the grid.
 */
export default function ChannelsScreen() {
  const {entry = 'all'} = useRouteParams<{entry?: string}>();
  const session = useSession();
  if (session.profile?.limits?.liveTv === false) return <EmptyState icon="lock" title="Live TV isn’t available on this profile" body="The main profile can turn it on in Settings › Account › Profiles." />;
  if (entry === 'recordings') return <Recordings />;
  return <Guide source={entry} />;
}

function Guide({source}: {source: string}) {
  const search = useRouteSearch<{view?: string; group?: string; favorites?: string; day?: string}>();
  const navigate = useNavigate();
  const phone = usePhone();
  const src = SOURCES.find(x => x.id === source) ?? SOURCES[0]!;
  useTitle(src.id === 'all' ? 'Live TV' : src.name);
  const view = search.view === 'list' ? 'list' : 'guide';
  const group = search.group ?? 'all';
  const favorites = search.favorites === 'true';
  const channels = useQuery(c => c.live.channels({source: source === 'all' ? undefined : source, group: group === 'all' ? undefined : group, favorites}), [source, group, favorites], ['live']);
  const all = useQuery(c => c.live.channels({source: source === 'all' ? undefined : source}), [source], ['live']);
  const groups = useMemo(() => [...new Set((all.data ?? []).map(c => c.group))].sort(), [all.data]);
  const set = (patch: Record<string, string | undefined>) => void navigate({to: `/channels/${source}` as never, search: {...search, ...patch} as never, replace: true});
  const [program, setProgram] = useState<{program: Program; channel: Channel}>();
  const demoClient = useClient();
  useDemo('program', () => { const ch = channels.data?.[1]; const p = ch && demoClient.live.nowPlaying(ch.id); if (ch && p) setProgram({program: p, channel: ch}); }, !!channels.data);
  const today = new Date(); today.setHours(0, 0, 0, 0);
  const dayOffset = Number(search.day ?? 0);
  return (
    <div style={{padding: '28px 0 48px'}}>
      <div className={s.head}>
        <div className={s.headText}>
          <div className="t-label" style={{marginBottom: 6}}>Live TV</div>
          <h1 className="t-title">{src.id === 'all' ? 'Guide' : src.name}</h1>
          {src.id === 'library' ? <p className="c-2" style={{marginTop: 6}}>Always-on channels made from your libraries. They play like live TV.</p> : null}
        </div>
        <Segmented label="View" value={view} onChange={v => set({view: v === 'guide' ? undefined : v})} options={[{value: 'guide', label: 'Guide', icon: 'guide'}, {value: 'list', label: 'Channels', icon: 'list'}]} />
      </div>
      <div className={s.bar}>
        {view === 'guide' ? <Select compact ariaLabel="Day" value={String(dayOffset)} onChange={v => set({day: v === '0' ? undefined : v})} options={[0, 1, 2].map(d => ({value: String(d), label: d === 0 ? 'Today' : d === 1 ? 'Tomorrow' : day(today.getTime() + d * 24 * HOUR)}))} /> : null}
        {groups.length > 1 ? <Select compact ariaLabel="Group" value={group} onChange={v => set({group: v === 'all' ? undefined : v})} options={[{value: 'all', label: phone ? 'All groups' : 'All channel groups'}, ...groups.map(g => ({value: g, label: g}))]} /> : null}
        <Button variant={favorites ? 'primary' : 'secondary'} icon={favorites ? 'starFilled' : 'star'} label="Favorites" pressed={favorites} onClick={() => set({favorites: favorites ? undefined : 'true'})} />
        <span style={{flex: 1}} />
        {channels.data ? <span className="t-caption c-3">{channels.data.length} channels</span> : null}
      </div>
      {channels.error ? <ErrorState onRetry={channels.refresh} /> : !channels.data ? <div style={{padding: '0 var(--gutter)', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 6}}>{Array.from({length: 8}, (_, i) => <Skeleton key={i} height={60} />)}</div>
        : !channels.data.length ? (favorites ? <EmptyState icon="star" title="No favorite channels yet" body="Star channels in the guide or the channel list to keep them here." actions={[{label: 'Show all channels', onClick: () => set({favorites: undefined})}]} /> : <EmptyState icon="live" title="No channels here" body="This source has no channels in this group." />)
        : view === 'list' ? <ChannelList channels={channels.data} onOpen={(program, channel) => setProgram({program, channel})} />
        : <GuideGrid channels={channels.data} dayStart={today.getTime() + dayOffset * 24 * HOUR} isToday={dayOffset === 0} onOpen={(program, channel) => setProgram({program, channel})} />}
      {program ? <ProgramSheet program={program.program} channel={program.channel} onClose={() => setProgram(undefined)} /> : null}
    </div>
  );
}

function GuideGrid({channels, dayStart, isToday, onOpen}: {channels: Channel[]; dayStart: number; isToday: boolean; onOpen: (p: Program, c: Channel) => void}) {
  const now = useNow(30000);
  const scroller = useRef<HTMLDivElement>(null);
  const from = dayStart;
  const to = dayStart + 24 * HOUR;
  const guide = useQuery(c => c.live.guide(from, to, channels.map(ch => ch.id)), [from, channels.map(c => c.id).join()], ['live', 'dvr']);
  const x = (t: number) => ((t - from) / 60000) * PX_PER_MIN;
  const width = x(to);
  // Programs that began before the visible window keep their title in view.
  const [viewLeft, setViewLeft] = useState(0);
  const onScroll = () => { const el = scroller.current; if (el) setViewLeft(el.scrollLeft); };
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const target = isToday ? x(Math.floor(now / HALF) * HALF - HALF) : x(from + 18 * HOUR);
    el.scrollLeft = Math.max(0, target);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [from, isToday, !!guide.data]);
  const ticks: number[] = [];
  for (let t = from; t < to; t += HALF) ticks.push(t);
  const shift = (hours: number) => scroller.current?.scrollBy({left: hours * 60 * PX_PER_MIN, behavior: 'smooth'});
  // Arrow keys: along a row by program, across rows to the program airing at the same time.
  const onKeyDown = (e: React.KeyboardEvent) => {
    const el = e.target as HTMLElement;
    const row = Number(el.dataset.row);
    if (Number.isNaN(row) || !el.dataset.start) return;
    const cells = (r: number) => [...(scroller.current?.querySelectorAll<HTMLElement>(`[data-row="${r}"]`) ?? [])];
    let next: HTMLElement | undefined;
    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') { const list = cells(row); next = list[list.indexOf(el) + (e.key === 'ArrowRight' ? 1 : -1)]; }
    else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const t = Math.max(Number(el.dataset.start), now);
      next = cells(row + (e.key === 'ArrowDown' ? 1 : -1)).find(c => Number(c.dataset.start) <= t && Number(c.dataset.end) > t) ?? cells(row + (e.key === 'ArrowDown' ? 1 : -1))[0];
    } else return;
    e.preventDefault();
    next?.focus();
    next?.scrollIntoView({block: 'nearest', inline: 'nearest'});
  };
  return (
    <>
      <div className={s.bar} style={{marginTop: -4}}>
        <Button size="sm" variant="ghost" icon="back" label="Earlier" onClick={() => shift(-2)} />
        {isToday ? <Button size="sm" variant="secondary" label="Now" onClick={() => { if (scroller.current) scroller.current.scrollTo({left: x(Math.floor(now / HALF) * HALF - HALF), behavior: 'smooth'}); }} /> : null}
        <Button size="sm" variant="ghost" trailingIcon="forward" label="Later" onClick={() => shift(2)} />
      </div>
      <div className={s.guide}>
        <div className={s.scroller} ref={scroller} onKeyDown={onKeyDown} onScroll={onScroll}>
          <div className={s.canvas} style={{width: `calc(var(--ch) + ${width}px)`}} role="grid" aria-label="Program guide" aria-rowcount={channels.length}>
            <div className={s.ruler} role="row">
              <div className={s.corner} role="columnheader">{day(from)}</div>
              <div className={s.ticks}>{ticks.map(t => <span key={t} className={s.tick} style={{left: x(t), width: 30 * PX_PER_MIN}}>{clock(t)}</span>)}</div>
            </div>
            {channels.map((ch, r) => (
              <div key={ch.id} className={s.rowG} role="row">
                <ChannelCell ch={ch} />
                <div className={s.lane} role="presentation">
                  {(guide.data?.[ch.id] ?? []).map(p => {
                    const left = Math.max(0, x(p.start));
                    const right = Math.min(width, x(p.end));
                    if (right - left < 2) return null;
                    const airing = p.start <= now && p.end > now;
                    return (
                      <button key={p.id} type="button" role="gridcell" className={cx(s.prog, airing && s.progNow, p.end <= now && s.progPast)} style={{left: left + 2, width: right - left - 4, paddingLeft: Math.max(10, Math.min(right - left - 60, viewLeft - left + 10))}} data-row={r} data-start={p.start} data-end={p.end} onClick={() => onOpen(p, ch)} aria-label={`${p.title}${p.episodeTitle ? `, ${p.episodeTitle}` : ''}, ${ch.name}, ${clock(p.start)} to ${clock(p.end)}${airing ? ', on now' : ''}${p.recording ? ', recording' : ''}`} tabIndex={airing && r === 0 ? 0 : -1}>
                        <span className={s.progTitle}>{p.recording ? <span className={s.recDot} title="Recording" /> : null}{p.isNew ? <span className={s.tag}>NEW</span> : null}{p.isLive ? <span className={s.tag}>LIVE</span> : null}<span className="clamp-1">{p.title}</span></span>
                        {right - left > 90 ? <span className={s.progMeta}>{clock(p.start)}–{clock(p.end)}{p.episodeTitle ? ` · ${p.episodeTitle}` : ` · ${p.category}`}</span> : null}
                      </button>
                    );
                  })}
                  {!guide.data ? <div className="skeleton" style={{position: 'absolute', inset: '6px 4px', opacity: 0.5}} /> : null}
                </div>
              </div>
            ))}
            {isToday && now >= from && now < to ? <div className={s.nowLine} style={{left: `calc(var(--ch) + ${x(now)}px)`, top: 0}} aria-hidden /> : null}
          </div>
        </div>
      </div>
    </>
  );
}

function ChannelCell({ch}: {ch: Channel}) {
  const player = usePlayer();
  const client = useClient();
  const toast = useToast();
  return (
    <Menu label={`${ch.name} options`} items={[
      {id: 'watch', label: 'Watch now', icon: 'play', onSelect: () => player.playLive(ch.id, channelEntry(ch, client.live.nowPlaying(ch.id)))},
      {id: 'fav', label: ch.favorite ? 'Remove from favorites' : 'Add to favorites', icon: ch.favorite ? 'starFilled' : 'star', onSelect: async () => { await client.live.setFavorite(ch.id, !ch.favorite); toast.show(ch.favorite ? `Removed ${ch.name} from favorites` : `Added ${ch.name} to favorites`, {icon: 'star'}); }},
      {id: 'hide', label: 'Hide this channel', icon: 'eyeOff', onSelect: async () => { await client.live.setHidden(ch.id, true); toast.show(`Hid ${ch.name}`, {icon: 'eyeOff', action: {label: 'Undo', run: () => void client.live.setHidden(ch.id, false)}}); }},
    ]} align="start" trigger={
      <button type="button" className={s.chan} role="rowheader" aria-label={`${ch.number} ${ch.name}${ch.favorite ? ', favorite' : ''}. Channel options`}>
        {ch.logo ? <img src={ch.logo} alt="" className={s.chanLogo} /> : null}
        <span className={s.chanText}><span className={s.chanNum}>{ch.number}{ch.hd ? ' · HD' : ''}</span><span className={s.chanName}>{ch.name}</span></span>
        {ch.favorite ? <Icon name="starFilled" size={14} className={s.fav} /> : null}
      </button>
    } />
  );
}

function ChannelList({channels, onOpen}: {channels: Channel[]; onOpen: (p: Program, c: Channel) => void}) {
  const client = useClient();
  const player = usePlayer();
  const now = useNow(30000);
  const guide = useQuery(c => c.live.guide(now - HOUR, now + 4 * HOUR, channels.map(ch => ch.id)), [channels.map(c => c.id).join(), Math.floor(now / 300000)], ['live']);
  return (
    <div className={s.list} role="list">
      {channels.map(ch => {
        const list = guide.data?.[ch.id] ?? [];
        const cur = list.find(p => p.start <= now && p.end > now);
        const next = list.find(p => p.start >= (cur?.end ?? now));
        return (
          <div key={ch.id} className={s.listRow} role="listitem">
            {ch.logo ? <img src={ch.logo} alt="" className={s.chanLogo} /> : <span className="t-strong">{ch.number}</span>}
            <span style={{minWidth: 0}}><span className="t-strong clamp-1" style={{display: 'block'}}>{ch.name}{ch.favorite ? <Icon name="starFilled" size={13} className={s.fav} style={{marginLeft: 6, verticalAlign: -1}} /> : null}</span><span className="t-caption c-3">{ch.number} · {ch.group}{ch.library ? ' · Library Channel' : ''}</span></span>
            <button type="button" className={s.nowCell} style={{textAlign: 'left'}} onClick={() => cur && onOpen(cur, ch)} disabled={!cur}>
              <span className="t-strong clamp-1">{cur ? cur.title : guide.data ? 'Off air' : ' '}</span>
              {cur ? <><span className="t-caption c-3 clamp-1">{clock(cur.start)}–{clock(cur.end)}{cur.episodeTitle ? ` · ${cur.episodeTitle}` : ''}</span><Progress value={(now - cur.start) / (cur.end - cur.start)} /></> : null}
            </button>
            <span className={`${s.hideNarrow} t-caption c-2 clamp-2`}>{next ? <>Next at {clock(next.start)} · {next.title}</> : null}</span>
            <span style={{display: 'flex', gap: 4}}>
              <IconButton variant="secondary" icon="play" label={`Watch ${ch.name}`} onClick={() => player.playLive(ch.id, channelEntry(ch, cur))} />
              <IconButton variant="ghost" icon={ch.favorite ? 'starFilled' : 'star'} label={ch.favorite ? 'Remove from favorites' : 'Add to favorites'} pressed={ch.favorite} onClick={() => void client.live.setFavorite(ch.id, !ch.favorite)} />
            </span>
          </div>
        );
      })}
    </div>
  );
}

/** What a program is, and what you can do with it: watch, record it or the series, or set a reminder. */
export function ProgramSheet({program, channel, onClose}: {program: Program; channel: Channel; onClose: () => void}) {
  const client = useClient();
  const player = usePlayer();
  const toast = useToast();
  const session = useSession();
  const detail = useQuery(c => c.live.program(program.id), [program.id], ['live', 'dvr']);
  const [series, setSeries] = useState(false);
  const p = detail.data?.program ?? program;
  const now = Date.now();
  const airing = p.start <= now && p.end > now;
  const future = p.start > now;
  const canRecord = session.profile?.limits?.dvr !== false && p.end > now;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={p.title} description={`${channel.number} ${channel.name} · ${day(p.start)}, ${clock(p.start)}–${clock(p.end)}`} width={560} footer={<>
      {airing ? <Button variant="primary" icon="play" label="Watch now" onClick={() => { onClose(); player.playLive(channel.id, channelEntry(channel, p)); }} /> : null}
      {canRecord ? (p.recording ? <Button variant="secondary" icon="close" label={p.recording === 'series' ? 'Recording series' : 'Cancel recording'} onClick={async () => { await client.live.cancelRecording(p.id); toast.show(`Won’t record “${p.title}”`, {icon: 'check'}); }} /> : <Button variant="secondary" icon="record" label="Record" onClick={async () => { await client.live.record(p.id); toast.show(`Recording “${p.title}” ${airing ? 'now' : `at ${clock(p.start)}`}`, {icon: 'record'}); }} />) : null}
      {canRecord && p.seriesId && p.recording !== 'series' ? <Button variant="secondary" icon="recordings" label="Record series…" onClick={() => setSeries(true)} /> : null}
      {future ? <Button variant="ghost" icon="bell" label={p.reminder ? 'Reminder set' : 'Remind me'} pressed={p.reminder} onClick={async () => { await client.live.remind(p.id, !p.reminder); toast.show(p.reminder ? 'Reminder removed' : `We’ll remind you when “${p.title}” starts`, {icon: 'bell'}); }} /> : null}
    </>}>
      {p.image ? <div className={s.sheetArt}><Artwork src={p.image} title={p.title} shape="wide" /></div> : null}
      <div className={s.badges} style={{marginBottom: 10}}>
        {airing ? <Badge tone="live">On now</Badge> : null}
        {p.isNew ? <Badge tone="accent">New</Badge> : null}
        {p.premiere ? <Badge tone="accent">Premiere</Badge> : null}
        {p.isLive ? <Badge tone="bad">Live</Badge> : null}
        {p.recording ? <Badge tone="bad" dot>{p.recording === 'series' ? 'Series recording' : 'Will record'}</Badge> : null}
        <Badge>{p.category}</Badge>
        {p.rating ? <Badge>{p.rating}</Badge> : null}
      </div>
      {p.episodeTitle ? <p className="t-strong">{p.season ? `S${p.season} E${p.episode} · ` : ''}{p.episodeTitle}</p> : null}
      {p.overview ? <p className="c-2" style={{marginTop: 6, lineHeight: 1.55}}>{p.overview}</p> : null}
      {airing ? <div style={{marginTop: 14}}><Progress value={(now - p.start) / (p.end - p.start)} /><p className="t-caption c-3" style={{marginTop: 6}}>{Math.round((p.end - now) / 60000)} min left</p></div> : null}
      {detail.data?.next ? <p className="t-caption c-3" style={{marginTop: 14}}>Next on {channel.name}: {detail.data.next.title} at {clock(detail.data.next.start)}</p> : null}
      {series ? <SeriesDialog program={p} channel={channel} onClose={() => setSeries(false)} /> : null}
    </Dialog>
  );
}

function SeriesDialog({program, channel, onClose}: {program: Program; channel: Channel; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [where, setWhere] = useState<'this' | 'any'>('this');
  const [newOnly, setNewOnly] = useState(true);
  const [busy, setBusy] = useState(false);
  useEffect(() => () => undefined, []);
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Record ${program.title}`} description="Records every airing that matches, now and in the future." width={460} footer={<>
      <Button variant="ghost" label="Cancel" onClick={onClose} />
      <Button variant="primary" icon="recordings" label="Record series" busy={busy} onClick={async () => { setBusy(true); await client.live.recordSeries(program.id, {channel: where, newOnly}); toast.show(`Recording every ${newOnly ? 'new ' : ''}episode of “${program.title}”`, {icon: 'recordings'}); onClose(); }} />
    </>}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 16}}>
        <RadioCards label="Channels" value={where} onChange={setWhere} options={[{value: 'this', label: `Only on ${channel.name}`}, {value: 'any', label: 'On any channel'}]} />
        <Checkbox checked={newOnly} onChange={setNewOnly} label="New episodes only" help="Skips reruns of episodes that already aired." />
      </div>
    </Dialog>
  );
}
