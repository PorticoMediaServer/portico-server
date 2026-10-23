import {useCallback, useState, type CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import type {DashboardHistory, DashboardTopKind} from '@core/admin/dashboard.ts';
import type {PlayHistoryPeriod} from '@core/play-history.ts';
import {Select, Button, Segmented} from '../../kit/controls';
import type {DashboardBindings} from './bindings';
import {useDashboardAccounts, useDashboardRead} from './use-read';
import {fmtDur, fmtHrs, niceScale} from './format';
import {hours, mediaKinds, quietestHours} from './model';
import {mediaLabel, Range, ReadFeedback, Section, t} from './shared';
import styles from './dashboard.module.css';
const historyKinds = mediaKinds;
const ranges = ['7d', '30d', '90d', 'all'] as const;
const duration = (ms: number) => fmtHrs(hours(ms));
const watchedDuration = (ms: number) => fmtDur(hours(ms));
export function Analytics({bindings: b}: {bindings: DashboardBindings}) {
  const [peopleRange, setPeopleRange] = useState<PlayHistoryPeriod>('7d'), [showAll, setShowAll] = useState(false), [peopleCursor, setPeopleCursor] = useState('');
  const peopleLoad = useCallback((signal: AbortSignal) => b.read.people({period: peopleRange}, peopleCursor, signal), [b, peopleRange, peopleCursor]);
  const people = useDashboardRead(b, peopleLoad, true, 60000);
  const [historyRange, setHistoryRange] = useState<PlayHistoryPeriod>('30d'), [media, setMedia] = useState('all'), [person, setPerson] = useState('all');
  const historyLoad = useCallback((signal: AbortSignal) => b.read.history({period: historyRange, ...(media === 'all' ? {} : {media}), ...(person === 'all' ? {} : {accountId: person})}, 'movies', signal), [b, historyRange, media, person]);
  const history = useDashboardRead(b, historyLoad, true, 60000);
  const [topKind, setTopKind] = useState<DashboardTopKind>('movies'), [topRange, setTopRange] = useState<PlayHistoryPeriod>('30d'), [topPerson, setTopPerson] = useState('all');
  const topLoad = useCallback((signal: AbortSignal) => b.read.history({period: topRange, ...(topPerson === 'all' ? {} : {accountId: topPerson})}, topKind, signal), [b, topRange, topPerson, topKind]);
  const top = useDashboardRead(b, topLoad, true, 60000);
  const accounts = useDashboardAccounts(b);
  const peopleOptions = [{value: 'all', label: t('next.dashboard.everyone')}, ...accounts.items.map(p => ({value: p.accountId, label: p.username || t('next.dashboard.removedPerson')})), ...(accounts.nextCursor ? [{value: '__more', label: t('next.dashboard.more')}] : [])];
  const firstTop = top.value?.top[0], topBackdrop = firstTop ? b.artwork(firstTop.itemId, 'backdrop', firstTop.backdropRevision) : undefined;
  const peoplePage = people.value?.items ?? [], maxTime = Math.max(1, ...peoplePage.map(p => p.watchedMs));
  return <>
    <Section title={t('next.dashboard.people')} actions={<Range value={peopleRange} values={ranges} onChange={value => {setPeopleRange(value as PlayHistoryPeriod); setPeopleCursor('');}} />}>
      <ReadFeedback state={people} />{people.value && <div className={styles.peopleCard}><div className={styles.legend}>{mediaKinds.map(kind => <span key={kind} data-series={kind}><i />{mediaLabel(kind)}</span>)}<small>{t('next.dashboard.timeWatched')}</small></div>
        <ol className={styles.peopleList}>{(showAll ? peoplePage : peoplePage.slice(0, 6)).map((p, index) => <li key={p.accountId} className={styles.personRow}><span>{index + 1}</span><span className={styles.avatar} aria-hidden="true">{p.username.slice(0, 1)}</span><div className={styles.personName}><b>{p.username || t('next.dashboard.removedPerson')}</b><small>{p.profiles.filter(name => name !== p.username).join(', ')}</small></div><span>{t('next.dashboard.plays', {count: p.plays})}</span><div className={styles.barTrack}><span className={styles.peopleBar} style={{width: `${p.kinds.filter(k => mediaKinds.some(kind => kind === k.kind)).reduce((sum, k) => sum + k.watchedMs, 0) / maxTime * 100}%`}} role="img" aria-label={`${p.username}: ${watchedDuration(p.watchedMs)}`}>{mediaKinds.flatMap(kind => p.kinds.filter(k => k.kind === kind && k.watchedMs / p.watchedMs > .004)).map(k => <i key={k.kind} data-series={k.kind} style={{flexGrow: k.watchedMs}} title={`${mediaLabel(k.kind)}: ${watchedDuration(k.watchedMs)}`} />)}</span></div><b>{watchedDuration(p.watchedMs)}</b></li>)}</ol>
        {!peoplePage.length && <p className={styles.empty}>{t('next.dashboard.nothing')}</p>}
        {peoplePage.length > 6 && <Button size="xs" variant="text" className={styles.showAll} onClick={() => setShowAll(!showAll)} aria-expanded={showAll}>{showAll ? t('next.dashboard.fewer') : t('next.dashboard.showAll', {count: peoplePage.length})}</Button>}
        {showAll && people.value.nextCursor && <Button size="xs" onClick={() => setPeopleCursor(people.value!.nextCursor)}>{t('next.dashboard.more')}</Button>}
      </div>}
    </Section>
    <Section title={t('next.dashboard.history')} extra={<Button size="xs" variant="text" onClick={b.effects.openHistory}>{t('next.dashboard.fullHistory')}</Button>} actions={<><Select aria-label={t('next.dashboard.media')} value={media} onValueChange={setMedia} options={['all', ...mediaKinds].map(kind => ({value: kind, label: mediaLabel(kind)}))} /><Select aria-label={t('next.dashboard.person')} value={person} options={peopleOptions} onValueChange={value => value === '__more' ? accounts.loadMore() : setPerson(value)} /><Range value={historyRange} values={['7d', '30d', '90d', '1y']} onChange={value => setHistoryRange(value as PlayHistoryPeriod)} /></>}>
      <ReadFeedback state={history} />{history.value && <HistoryCharts history={history.value} media={media} />}
    </Section>
    <Section title={t('next.dashboard.top')} actions={<><Segmented aria-label={t('next.dashboard.media')} value={topKind} onValueChange={value => setTopKind(value as DashboardTopKind)} items={['movies', 'tv', 'music'].map(kind => ({value: kind, label: mediaLabel(kind)}))} /><Select aria-label={t('next.dashboard.person')} value={topPerson} options={peopleOptions} onValueChange={value => value === '__more' ? accounts.loadMore() : setTopPerson(value)} /><Range value={topRange} values={ranges} onChange={value => setTopRange(value as PlayHistoryPeriod)} /></>}>
      <ReadFeedback state={top} />{top.value && (top.value.top.length ? <div className={styles.rankedShelf}>{topBackdrop && <div className={styles.shelfAmbient} aria-hidden="true"><img src={topBackdrop} alt="" /></div>}<ol className={styles.shelf} data-square={topKind === 'music' || undefined}>{top.value.top.map((item, index) => {
        const art = b.artwork(item.itemId, topKind === 'music' ? 'square' : 'poster', item.posterRevision);
        return <li key={item.itemId}><span className={styles.rank} aria-hidden="true">{index + 1}</span><div className={styles.rankedArt}>{art && <img src={art} alt="" loading="lazy" />}</div><h4>{item.title}</h4><p>{t(item.countUnit === 'track_plays' ? 'next.dashboard.trackPlays' : item.countUnit === 'episode_plays' ? 'next.dashboard.episodePlays' : item.countUnit === 'album_plays' ? 'next.dashboard.albumPlays' : 'next.dashboard.plays', {count: item.plays})}</p>{topPerson === 'all' && <p>{t('next.dashboard.peopleCount', {count: item.people})}</p>}</li>;
      })}</ol></div> : <p className={styles.empty}>{t('next.dashboard.nothing')}</p>)}
    </Section>
  </>;
}
function HistoryCharts({history: h, media}: {history: DashboardHistory; media: string}) {
  const [focus, setFocus] = useState<number | undefined>(), [cell, setCell] = useState<{day: number; hour: number} | undefined>();
  const total = h.buckets.reduce((s, b) => s + b.kinds.reduce((sum, k) => sum + k.watchedMs, 0), 0), scale = niceScale(Math.max(.5, ...h.buckets.map(b => hours(b.kinds.reduce((sum, k) => sum + k.watchedMs, 0)))) * 1.06, 4), max = scale.top * 3_600_000;
  const kinds = historyKinds.filter(kind => media === 'all' || media === kind);
  const quiet = quietestHours(h.busiestTimes), heatMax = Math.max(1, ...h.busiestTimes.flat());
  const hour = (hour: number) => new Intl.DateTimeFormat(defaultI18n.locale, {hour: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 1, hour));
  const date = (stamp: string) => new Intl.DateTimeFormat(defaultI18n.locale, {month: 'short', day: 'numeric', timeZone: h.timeZone}).format(Date.parse(stamp));
  return <><div className={styles.card}><h4>{t('next.dashboard.timePlayed')}</h4>{total ? <div className={styles.plot} tabIndex={0} role="group" aria-label={t('next.dashboard.timePlayed')}
    onBlur={() => setFocus(undefined)} onPointerLeave={() => setFocus(undefined)} onPointerMove={event => {const rect = event.currentTarget.getBoundingClientRect(); setFocus(Math.max(0, Math.min(h.buckets.length - 1, Math.floor((event.clientX - rect.left) / rect.width * h.buckets.length))));}}
    onKeyDown={event => {if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {event.preventDefault(); setFocus(old => event.key === 'Home' ? 0 : event.key === 'End' ? h.buckets.length - 1 : Math.max(0, Math.min(h.buckets.length - 1, (old ?? 0) + (event.key === 'ArrowLeft' ? -1 : 1))));}}}>
    <svg className={styles.chart} viewBox="0 0 912 250" role="img" aria-label={t('next.dashboard.timePlayed')}>
      {scale.ticks.map(value => <g key={value}><path className={styles.gridline} d={`M48,${210 - value / scale.top * 190}H864`} /><text x="40" y={214 - value / scale.top * 190} textAnchor="end">{value === 0 ? '0' : t('next.dashboard.hours', {value: String(value)})}</text></g>)}
      {kinds.map((kind, ki) => {
        const x = (i: number) => 48 + i / Math.max(1, h.buckets.length - 1) * 816;
        const y = (i: number, last: number) => 210 - h.buckets[i].kinds.filter(k => kinds.slice(0, last).includes(k.kind as typeof kinds[number])).reduce((sum, k) => sum + k.watchedMs, 0) / max * 190;
        const path = h.buckets.map((_, i) => `${i ? 'L' : 'M'}${x(i)},${y(i, ki + 1)}`).join('') + h.buckets.map((_, i) => i).reverse().map(i => `L${x(i)},${y(i, ki)}`).join('') + 'Z';
        return <path className={styles.area} key={kind} data-series={kind} d={path} />;
      })}
      <text x="48" y="242">{date(h.from)}</text><text x="864" y="242" textAnchor="end">{date(h.until)}</text>
    </svg>
    <div className={styles.chartTip} aria-live="polite">{focus !== undefined && h.buckets[focus] && <><b>{date(h.buckets[focus].start)} – {date(h.buckets[focus].end)} {h.buckets[focus].partial && t('next.dashboard.partial')}</b>{h.buckets[focus].kinds.filter(k => mediaKinds.some(kind => kind === k.kind)).map(k => <span key={k.kind}>{mediaLabel(k.kind)}<strong>{duration(k.watchedMs)}</strong></span>)}</>}</div>
    </div> : <p className={styles.empty}>{t('next.dashboard.nothing')}</p>}
    <div className={styles.legend}>{kinds.map(kind => <span key={kind} data-series={kind}><i />{mediaLabel(kind)}<b>{duration(h.buckets.reduce((sum, b) => sum + (b.kinds.find(k => k.kind === kind)?.watchedMs ?? 0), 0))}</b></span>)}<small>{t('next.dashboard.total')} <b>{duration(total)}</b></small></div>
    </div>
    <div className={styles.card}><div className={styles.heading}><h4>{t('next.dashboard.busiest')}</h4>{quiet !== undefined && <span className={styles.quiet}>{t('next.dashboard.quietest')} <b>{t('next.dashboard.quietWindow', {from: hour(quiet), until: hour(quiet + 3)})}</b></span>}</div>
      <div className={styles.heatScroll}><div className={styles.heat} style={{'--quiet-hour': quiet ?? 0} as CSSProperties} role="group" aria-label={t('next.dashboard.busiest')}>
        {h.busiestTimes.map((day, d) => <div className={styles.heatDay} key={d}><span>{new Intl.DateTimeFormat(defaultI18n.locale, {weekday: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5 + d))}</span>{day.map((ms, hour) => <button type="button" key={hour} className={styles.heatCell} data-quiet={quiet !== undefined && hour >= quiet && hour < quiet + 3 || undefined} style={{'--heat-intensity': `${(.08 + .92 * ms / heatMax) * 100}%`} as CSSProperties} aria-label={`${new Intl.DateTimeFormat(defaultI18n.locale, {weekday: 'long', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5 + d))} ${hour}:00: ${duration(ms)}`} onFocus={() => setCell({day: d, hour})} onBlur={() => setCell(undefined)} onPointerEnter={() => setCell({day: d, hour})} onPointerLeave={() => setCell(undefined)} />)}</div>)}
        {quiet !== undefined && <div className={styles.quietWindow} aria-hidden="true" />}
        <div className={styles.heatHours}><span />{[0, 3, 6, 9, 12, 15, 18, 21].map(h => <span key={h}>{hour(h)}</span>)}</div>
      </div></div>
      <div className={styles.legend}><span role="status">{cell ? `${hour(cell.hour)}: ${duration(h.busiestTimes[cell.day][cell.hour])}` : ''}</span><small>{t('next.dashboard.quieter')}　{t('next.dashboard.busier')}</small></div>
    </div></>;
}
