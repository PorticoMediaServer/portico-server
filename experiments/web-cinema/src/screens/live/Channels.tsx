import React, {useEffect, useMemo, useRef, useState} from 'react';
import {Link, useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Channel, Programme} from '../../data/types';
import type {ChannelsSearch} from '../../app/router';
import {player} from '../../app/player';
import {useSession} from '../../app/session';
import {setTint} from '../../app/tint';
import {artKeys} from '../../data/art';
import {Page, PageHeader} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Chip, SearchInput, Segmented, Select, Switch} from '../../ui/Controls';
import {Badge, ErrorState, Notice, Progress, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {Menu} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {useCompact} from '../../ui/useMediaQuery';
import {timeOfDay} from '../shared/items';
import {ProgrammeSheet} from './ProgrammeSheet';
import {Recordings} from './Recordings';

const MIN = 60_000;
const PX = 5.2; // pixels per minute in the guide
const WINDOW = 6 * 60; // minutes shown at once

export const slugOf = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-');
const backdrops = [...artKeys].filter(k => k.startsWith('backdrop/'));
export function programmeArt(p: Pick<Programme, 'title'>) {
  let h = 0;
  for (let i = 0; i < p.title.length; i++) h = (h * 31 + p.title.charCodeAt(i)) | 0;
  return backdrops[Math.abs(h) % backdrops.length];
}

/**
 * Channels: every source's channels, a guide you can move through with the keyboard, the channel
 * list, Library Channels and Recordings (the DVR). One screen per rail entry.
 */
export function ChannelsScreen() {
  const {entry} = useParams({from: '/app/channels/$entry'});
  const search = useSearch({from: '/app/channels/$entry'});
  const navigate = useNavigate();
  const services = useServices();
  const session = useSession();
  useDataVersion();
  const channels = useQuery('live.channels', () => services.live.channels());
  const phone = useCompact();
  const set = (patch: Partial<ChannelsSearch>) => void navigate({to: '/channels/$entry', params: {entry}, search: {...search, ...patch}, replace: true, resetScroll: false});
  if (session.profile?.limits.liveTv === false) return <Page title="Channels"><StateView icon="lock" title="Live TV is off for this profile" body="The main profile can turn it on in Settings › Profiles." /></Page>;
  if (entry === 'recordings') return <Recordings tab={search.tab} />;
  if (channels.error) return <Page title="Channels"><ErrorState error={channels.error} onRetry={channels.reload} what="channels" /></Page>;
  const all = channels.data ?? [];
  const sources = [...new Set(all.filter(c => !c.library).map(c => c.source))];
  const source = entry === 'library' ? 'Library Channels' : sources.find(s => slugOf(s) === entry);
  const inEntry = all.filter(c => (entry === 'all' ? true : entry === 'library' ? c.library : c.source === source));
  const title = entry === 'all' ? 'Channels' : source ?? 'Channels';
  if (channels.data && !all.length) return <Page title="Channels"><StateView icon="live" title="No channels yet" body={session.manager ? 'Add a TV source or create Library Channels, and they appear here.' : 'When the server owner adds channels, they appear here.'} action={session.manager ? {label: 'Set up Live TV', icon: 'settings', onClick: () => void navigate({to: '/server/$section', params: {section: 'live-tv'}})} : undefined} /></Page>;
  if (channels.data && entry !== 'all' && entry !== 'library' && !source) return <Page title="Channels"><StateView icon="live" title="This channel source isn’t on the server any more" action={{label: 'All channels', onClick: () => void navigate({to: '/channels/$entry', params: {entry: 'all'}})}} /></Page>;
  const groups = [...new Set(inEntry.map(c => c.group))];
  const shown = inEntry.filter(c => (search.favorites ? c.favorite : true) && (search.group ? c.group === search.group : true));
  const view = search.view ?? (phone ? 'list' : 'guide');
  return (
    <Page title={title} className="channels-page">
      <PageHeader
        eyebrow={entry === 'library' ? 'Made from your libraries' : entry === 'all' ? `${sources.length + (all.some(c => c.library) ? 1 : 0)} sources` : 'Channel source'}
        title={entry === 'library' ? 'Library Channels' : title}
        actions={<>
          <Segmented label="View" value={view} onChange={v => set({view: v})} options={[{value: 'guide', label: 'Guide', icon: 'grid'}, {value: 'list', label: 'List', icon: 'list'}]} />
          {session.manager ? <Button variant="ghost" icon="settings" label={entry === 'library' ? 'Manage' : 'Source settings'} onClick={() => void navigate({to: '/server/$section', params: {section: entry === 'library' ? 'channels' : 'live-tv'}})} /> : null}
        </>}
      />
      {entry === 'all' || entry === 'library' ? <OnNow channels={inEntry} /> : null}
      {source && /cable|iptv/i.test(source) ? <Notice tone="warn" title="This guide may be out of date" className="stale">The last guide refresh for {source} didn’t finish, so some programs may have changed.{session.manager ? <> <Link to="/server/$section" params={{section: 'live-tv'}} className="link">Open source settings</Link></> : null}</Notice> : null}
      <div className="ch-filters">
        <div className="chip-row">
          <Chip on={!search.group && !search.favorites} onClick={() => set({group: undefined, favorites: undefined})}>All</Chip>
          <Chip on={!!search.favorites} icon="star" onClick={() => set({favorites: search.favorites ? undefined : true, group: undefined})}>Favorites</Chip>
          {groups.map(g => <Chip key={g} on={search.group === g} onClick={() => set({group: search.group === g ? undefined : g, favorites: undefined})}>{g}</Chip>)}
        </div>
      </div>
      {!channels.data ? <GuideSkeleton /> : !shown.length ? (
        <StateView compact icon="filter" title={search.favorites ? 'No favorite channels yet' : `No channels in ${search.group}`} body={search.favorites ? 'Star a channel in the guide or list to keep it here.' : undefined} action={{label: 'Show all channels', onClick: () => set({group: undefined, favorites: undefined})}} />
      ) : view === 'guide' ? <Guide channels={shown} day={search.day} setDay={d => set({day: d})} /> : <ChannelList channels={inEntry.filter(c => shown.includes(c) || c.hidden)} />}
    </Page>
  );
}

function OnNow({channels}: {channels: Channel[]}) {
  const services = useServices();
  const now = Date.now();
  const guide = useQuery(`guide.now.${Math.floor(now / 600000)}`, () => services.live.guide(now - 3 * 3600_000, now + 3 * 3600_000));
  const list = channels.filter(c => !c.hidden).map(c => ({c, p: guide.data?.find(p => p.channelId === c.id && p.start <= now && p.end > now)})).filter(x => x.p).slice(0, 12);
  if (!list.length) return null;
  return (
    <section className="onnow" aria-label="On now">
      <h2 className="section-title">On now</h2>
      <div className="onnow-row">
        {list.map(({c, p}) => (
          <button key={c.id} type="button" className="onnow-card" onClick={() => player.playLive(c.id)} onMouseEnter={() => setTint(c.tint)} onFocus={() => setTint(c.tint)} aria-label={`Watch ${c.name}: ${p!.title}`}>
            <span className="onnow-art"><Art src={programmeArt(p!)} tint={c.tint} /><span className="onnow-scrim" /><span className="onnow-logo"><Art src={c.logo} tint={c.tint} /></span><span className="live-badge"><span />Live</span></span>
            <span className="onnow-text"><strong>{p!.title}</strong><span>{c.number} · {c.name} · {Math.round((p!.end - now) / MIN)} min left</span></span>
            <Progress value={(now - p!.start) / (p!.end - p!.start)} thin tone="bad" label="Progress" />
          </button>
        ))}
      </div>
    </section>
  );
}

function Guide({channels, day, setDay}: {channels: Channel[]; day?: string; setDay: (d: string) => void}) {
  const services = useServices();
  const version = useDataVersion();
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), 30_000); return () => clearInterval(t); }, []);
  const dayOffset = day === 'yesterday' ? -1 : day === 'tomorrow' ? 1 : 0;
  const base = useMemo(() => {
    const d = new Date(now + dayOffset * 86400000);
    if (dayOffset) d.setHours(18, 0, 0, 0);
    else d.setMinutes(d.getMinutes() < 30 ? 0 : 30, 0, 0);
    return d.getTime() - (dayOffset ? 0 : 30 * MIN);
  }, [now, dayOffset]);
  const [shift, setShift] = useState(0);
  useEffect(() => setShift(0), [day]);
  const start = base + shift * 120 * MIN;
  const end = start + WINDOW * MIN;
  const guide = useQuery(`guide.${start}.${version}`, () => services.live.guide(start, end));
  const recs = useQuery(`live.recordings.${version}`, () => services.live.recordings());
  const reminders = useQuery(`live.reminders.${version}`, () => services.live.reminders());
  const [open, setOpen] = useState<Programme | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const slots = Array.from({length: WINDOW / 30}, (_, i) => start + i * 30 * MIN);
  const x = (t: number) => ((t - start) / MIN) * PX;
  const nowX = x(now);
  const recorded = new Set((recs.data ?? []).filter(r => r.state === 'scheduled' || r.state === 'recording' || r.state === 'conflicted').map(r => r.programmeId));
  const reminded = new Set(reminders.data ?? []);
  const byChannel = new Map<string, Programme[]>();
  (guide.data ?? []).forEach(p => byChannel.set(p.channelId, [...(byChannel.get(p.channelId) ?? []), p]));
  useEffect(() => {
    if (scroller.current && shift === 0 && !dayOffset) scroller.current.scrollLeft = Math.max(0, nowX - 240);
  }, [start]); // eslint-disable-line react-hooks/exhaustive-deps

  const onKey = (e: React.KeyboardEvent) => {
    const t = e.target as HTMLElement;
    if (!t.classList.contains('g-cell')) return;
    const row = Number(t.dataset.row);
    const s = Number(t.dataset.start);
    const cells = (r: number) => Array.from(scroller.current?.querySelectorAll<HTMLElement>(`.g-cell[data-row="${r}"]`) ?? []);
    let next: HTMLElement | undefined;
    if (e.key === 'ArrowRight') next = cells(row).find(c => Number(c.dataset.start) > s);
    if (e.key === 'ArrowLeft') next = [...cells(row)].reverse().find(c => Number(c.dataset.start) < s);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const list = cells(row + (e.key === 'ArrowDown' ? 1 : -1));
      next = list.find(c => Number(c.dataset.start) <= s && Number(c.dataset.end) > s) ?? list[0];
    }
    const prog = (guide.data ?? []).find(p => p.id === t.dataset.id);
    if (e.key.toLowerCase() === 'w' && prog && prog.start <= now) { player.playLive(prog.channelId); e.preventDefault(); }
    if (e.key.toLowerCase() === 'r' && prog) { void services.live.record(prog.id, {series: false}).then(() => toast(`Recording “${prog.title}”`, {icon: 'record'})); e.preventDefault(); }
    if (e.key.toLowerCase() === 'n') { setShift(0); setDay('today'); e.preventDefault(); }
    if (next) { e.preventDefault(); next.focus(); next.scrollIntoView({block: 'nearest', inline: 'nearest'}); }
  };

  return (
    <div className="guide-wrap">
      <div className="guide-bar">
        <Segmented size="sm" label="Day" value={day ?? 'today'} onChange={setDay} options={[{value: 'yesterday', label: 'Yesterday'}, {value: 'today', label: 'Today'}, {value: 'tomorrow', label: 'Tomorrow'}]} />
        <span className="spacer" />
        <Button variant="secondary" size="sm" icon="chevronLeft" label="Earlier" onClick={() => setShift(s => s - 1)} aria-label="Two hours earlier" />
        <Button variant="secondary" size="sm" label="Now" onClick={() => { setShift(0); setDay('today'); }} disabled={shift === 0 && !dayOffset} />
        <Button variant="secondary" size="sm" trailingIcon="chevronRight" label="Later" onClick={() => setShift(s => s + 1)} aria-label="Two hours later" />
      </div>
      <div className="guide" ref={scroller} role="grid" aria-label="Program guide" onKeyDown={onKey} style={{'--gw': `${WINDOW * PX}px`} as React.CSSProperties}>
        <div className="g-head" role="row">
          <div className="g-corner" role="columnheader">{new Date(start).toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'})}</div>
          <div className="g-times">{slots.map(t => <div key={t} className="g-time" style={{left: x(t)}} role="columnheader">{timeOfDay(t)}</div>)}</div>
        </div>
        {channels.map((c, row) => (
          <div key={c.id} className={cx('g-row', c.hidden && 'is-hidden')} role="row">
            <ChannelCell channel={c} />
            <div className="g-track" role="presentation">
              {guide.data ? (byChannel.get(c.id) ?? []).map(p => {
                const left = Math.max(0, x(p.start));
                const width = Math.min(x(p.end), WINDOW * PX) - left;
                if (width <= 4) return null;
                const live = p.start <= now && p.end > now;
                const past = p.end <= now;
                return (
                  <button key={p.id} type="button" role="gridcell" data-row={row} data-start={p.start} data-end={p.end} data-id={p.id} tabIndex={row === 0 && live ? 0 : -1}
                    className={cx('g-cell', live && 'is-live', past && 'is-past')} style={{left, width}} onClick={() => setOpen(p)}
                    aria-label={`${p.title}, ${c.name}, ${timeOfDay(p.start)} to ${timeOfDay(p.end)}${live ? ', on now' : ''}${recorded.has(p.id) ? ', recording' : ''}`}>
                    <span className="g-title">{p.title}</span>
                    <span className="g-sub">{p.isNew ? <em className="g-new">New</em> : null}{recorded.has(p.id) ? <span className="g-rec" aria-hidden /> : null}{reminded.has(p.id) ? <Icon name="bell" size={11} /> : null}{timeOfDay(p.start)}{p.episodeTitle ? ` · ${p.episodeTitle}` : ''}</span>
                  </button>
                );
              }) : <span className="skeleton g-skel" />}
              {!guide.data || (byChannel.get(c.id) ?? []).length ? null : <span className="g-none">No guide information</span>}
            </div>
          </div>
        ))}
        {nowX > 0 && nowX < WINDOW * PX ? <div className="g-now" style={{left: `calc(var(--gcol) + ${nowX}px)`}} aria-hidden><span>{timeOfDay(now)}</span></div> : null}
      </div>
      <p className="faint small guide-hint"><Icon name="keyboard" size={14} /> Arrow keys move, Enter opens, W watches, R records, N jumps to now.</p>
      {open ? <ProgrammeSheet programme={open} channel={channels.find(c => c.id === open.channelId)} onClose={() => setOpen(null)} /> : null}
    </div>
  );
}

function ChannelCell({channel: c}: {channel: Channel}) {
  return (
    <div className="g-ch" role="rowheader">
      <button type="button" className="g-ch-main" onClick={() => player.playLive(c.id)} aria-label={`Watch ${c.name}`}>
        <span className="g-logo"><Art src={c.logo} tint={c.tint} /></span>
        <span className="g-ch-text"><strong>{c.number}</strong><span>{c.name}</span></span>
      </button>
      <ChannelMenu channel={c} trigger={<button type="button" className={cx('g-star', c.favorite && 'is-on')} aria-label={`More for ${c.name}`}><Icon name={c.favorite ? 'star' : 'more'} size={15} fill={c.favorite ? 'currentColor' : 'none'} /></button>} />
    </div>
  );
}

export function ChannelMenu({channel: c, trigger}: {channel: Channel; trigger: React.ReactElement}) {
  const services = useServices();
  return (
    <Menu label={`More for ${c.name}`} trigger={trigger} items={[
      {id: 'watch', label: `Watch ${c.name}`, icon: 'play', onSelect: () => player.playLive(c.id)},
      {id: 'fav', label: c.favorite ? 'Remove from Favorites' : 'Add to Favorites', icon: 'star', separatorBefore: true, onSelect: () => void services.live.setChannel(c.id, {favorite: !c.favorite}).then(() => toast(c.favorite ? `Removed ${c.name} from Favorites` : `Added ${c.name} to Favorites`))},
      {id: 'hide', label: c.hidden ? 'Show channel' : 'Hide channel', icon: c.hidden ? 'eye' : 'eyeOff', onSelect: () => void services.live.setChannel(c.id, {hidden: !c.hidden}).then(() => toast(c.hidden ? `${c.name} is back in the guide` : `${c.name} is hidden. Show hidden channels in the list to bring it back.`))},
    ]} />
  );
}

function ChannelList({channels}: {channels: Channel[]}) {
  const services = useServices();
  const [q, setQ] = useState('');
  const [sort, setSort] = useState<'number' | 'name'>('number');
  const [hidden, setHidden] = useState(false);
  const now = Date.now();
  const guide = useQuery(`guide.now.${Math.floor(now / 600000)}`, () => services.live.guide(now - 3 * 3600_000, now + 3 * 3600_000));
  const list = channels
    .filter(c => (hidden ? true : !c.hidden))
    .filter(c => !q || `${c.number} ${c.name} ${guide.data?.find(p => p.channelId === c.id && p.start <= now && p.end > now)?.title ?? ''}`.toLowerCase().includes(q.toLowerCase()))
    .sort((a, b) => (sort === 'name' ? a.name.localeCompare(b.name) : parseFloat(a.number) - parseFloat(b.number)));
  return (
    <div className="ch-list-wrap">
      <div className="toolbar">
        <SearchInput label="Search channels and programs" placeholder="Search channels and programs" value={q} onChange={setQ} className="ch-search" />
        <Select label="Sort" value={sort} onChange={setSort} options={[{value: 'number', label: 'By number'}, {value: 'name', label: 'By name'}]} />
        <label className="hstack small"><Switch checked={hidden} onChange={setHidden} label="Show hidden channels" /> Show hidden</label>
      </div>
      {!list.length ? <StateView compact icon="search" title={`No channels match “${q}”`} /> : (
        <ul className="ch-list">
          {list.map(c => {
            const on = guide.data?.find(p => p.channelId === c.id && p.start <= now && p.end > now);
            const next = guide.data?.find(p => p.channelId === c.id && on && p.start >= on.end);
            return (
              <li key={c.id} className={cx('ch-row', c.hidden && 'is-hidden')}>
                <button type="button" className="ch-main" onClick={() => player.playLive(c.id)} onMouseEnter={() => setTint(c.tint)}>
                  <span className="g-logo is-lg"><Art src={c.logo} tint={c.tint} /></span>
                  <span className="ch-num tabular">{c.number}</span>
                  <span className="ch-text">
                    <strong>{c.name}{c.favorite ? <Icon name="star" size={13} fill="currentColor" className="ch-fav" /> : null}{c.hidden ? <Badge>Hidden</Badge> : null}{c.library ? <Badge tone="accent">Library</Badge> : null}</strong>
                    <span className="ch-now">{on ? <><span className="live-dot" />{on.title}<span className="faint"> · {Math.round((on.end - now) / MIN)} min left</span></> : 'No information'}</span>
                    {next ? <span className="faint small">Next: {next.title} at {timeOfDay(next.start)}</span> : null}
                    {on ? <Progress value={(now - on.start) / (on.end - on.start)} thin tone="bad" label="Progress" /> : null}
                  </span>
                </button>
                <ChannelMenu channel={c} trigger={<button type="button" className="track-more is-shown" aria-label={`More for ${c.name}`}><Icon name="more" size={18} /></button>} />
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function GuideSkeleton() {
  return <div className="stack">{Array.from({length: 8}, (_, i) => <Skeleton key={i} h={64} />)}</div>;
}
