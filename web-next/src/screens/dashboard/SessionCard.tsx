import {useSessionMove} from './use-session-move';
import {useId, useRef, useState, type CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import type {DashboardSession as AdminSession} from './bindings';
import {Button} from '../../kit/controls';
import {Dialog, Menu} from '../../kit/overlays';
import {Icon, type IconName} from '../../kit/Icon';
import {Artwork} from '../../kit/artwork/Artwork';
import {accountAvatarStyle} from '../../kit/settings/account-avatar';
import type {DashboardBindings} from './bindings';
import {fmtDur, formatStreamRate, formatTrackRoute} from './format';
import {clock, sessionTitle, sessionStopTitle, sessionTracks, type Track} from './model';
import styles from './dashboard.module.css';
const t = defaultI18n.t;
/** The source-family glyphs in the checked dashboard.js sprite; the subtitle glyph is shared with Settings. */
function TrackIcon({kind}: {kind: Track['kind']}) {
  if (kind === 'subtitles') return <Icon name="i-cc" size={14} />;
  return <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true"><g fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">{kind === 'video'
    ? <><rect x="3" y="5" width="18" height="14" rx="3" /><path d="m10.4 9.4 4.2 2.6-4.2 2.6z" fill="currentColor" /></>
    : <><path d="M4 10v4h3.6L12.5 18V6L7.6 10z" /><path d="M16 9.4a4 4 0 0 1 0 5.2M18.6 6.8a7.6 7.6 0 0 1 0 10.4" /></>}</g></svg>;
}
function DeviceGlyph({platform, device}: Pick<AdminSession, 'platform' | 'device'>) {
  const p = platform?.toLowerCase();
  const name: IconName = p === 'ipados' || /\bipad\b/i.test(device) ? 'i-tablet'
    : p && ['tvos', 'androidtv', 'firetv', 'roku', 'tizen', 'webos'].includes(p) ? 'i-tv'
    : p === 'ios' || p === 'android' ? 'i-phone'
    : p && ['web', 'desktop', 'macos', 'windows', 'linux'].includes(p) ? 'i-laptop' : 'i-device';
  return <Icon name={name} size={20} />;
}
/** The drawn phone card's route chip: the one route a stream is taking, never an inferred total. */
function phoneRoute(tracks: readonly Track[]): Track | undefined {
  return tracks.find(track => track.kind === 'video') ?? tracks[0];
}
export function SessionCard({session: s, bindings: b, refresh, layout = 'desktop', retained = false}: {session: AdminSession; bindings: DashboardBindings; refresh: (removedId?: string) => void; layout?: 'desktop' | 'phone'; retained?: boolean}) {
  const [menu, setMenu] = useState(false), [stop, setStop] = useState(false), [message, setMessage] = useState(''), [busy, setBusy] = useState(false);
  const messageId = useId(), cancel = useRef<HTMLButtonElement>(null), opener = useRef<HTMLButtonElement>(null), item = s.item, square = s.kind === 'audio' || item?.kind === 'audiobook_file';
  const art = s.itemId ? b.artwork(s.itemId, square ? 'square' : 'poster', item?.art.find(a => a.role === (square ? 'square' : 'poster'))?.revision) : undefined;
  const backdrop = s.itemId ? b.artwork(s.itemId, 'backdrop', item?.art.find(a => a.role === 'backdrop')?.revision) : undefined;
  const act = async (action: () => Promise<void>, depart = false) => {if (!b.isCurrent() || busy) return; setBusy(true); try {await action(); if (b.isCurrent()) {setStop(false); refresh(depart ? s.id : undefined);}} catch (error) {if (b.isCurrent()) b.effects.reportError(error);} finally {if (b.isCurrent()) setBusy(false);}};
  const state = s.state === 'playing' ? t('next.dashboard.playing') : s.state === 'paused' ? t('next.dashboard.paused') : s.state === 'buffering' ? t('next.dashboard.buffering') : s.state === 'preparing' ? t('next.dashboard.preparing') : undefined;
  const title = sessionTitle(s), stopTitle = sessionStopTitle(s);
  const runtime = s.durationMs ? fmtDur(s.durationMs / 3_600_000) : undefined;
  const move = useSessionMove(b, s, title, menu, retained, () => refresh());
  const episode = item?.showTitle && s.title !== item.showTitle;
  const tracks = sessionTracks(s), current = s.currentBitrateKbps;
  const menuItems = [
    ...(move.canMove ? [{id: 'move', icon: 'i-clock' as const, label: t('web.nowPlaying.moveTo'), onSelect: move.open}] : []),
    ...(['paused', 'playing'].includes(s.state) ? [{id: 'transport', icon: s.state === 'paused' ? 'i-play' as const : 'i-pause' as const, label: s.state === 'paused' ? t('next.dashboard.resume') : t('next.dashboard.pause'), onSelect: () => {void act(() => b.effects.transport(s.id, s.state === 'paused' ? 'resume' : 'pause'));}}] : []),
    ...(b.role === 'owner' && stopTitle ? [{id: 'stop', icon: 'i-stop' as const, label: t('next.dashboard.stop'), destructive: true, onSelect: () => {setMessage(''); setStop(true);}}] : []),
  ];
  const sessionMenu = <><Menu open={menu} onOpenChange={setMenu} title={t('next.dashboard.sessionMenu', {user: s.user})} trigger={<Button ref={opener} data-session-menu icon="i-more" iconOnly size="xs" disabled={busy} aria-label={t('next.dashboard.sessionMenu', {user: s.user})} />} items={menuItems} />{move.dialog}</>;
  const stopDialog = <Dialog open={stop} onOpenChange={value => {if (!busy) setStop(value);}} title={t('next.dashboard.stopTitle', {title: stopTitle})} description={s.user && s.device ? t('next.dashboard.stopDetail', {user: s.user, device: s.device}) : undefined} returnFocusRef={opener} initialFocusRef={cancel} width={560} showCloseButton={false} className={styles.stopDialog} bodyClassName={styles.stopBody} footerClassName={styles.stopFooter}
    footer={<><Button ref={cancel} size="small" disabled={busy} onClick={() => setStop(false)}>{t('next.dashboard.cancel')}</Button><Button size="small" className={styles.stopButton} loading={busy} onClick={() => {void act(() => b.effects.stop(s.id, message.trim() || undefined), true);}}>{t('next.dashboard.stopPlayback')}</Button></>}>
    <div className={styles.viewerMessage}><label htmlFor={messageId}>{t('next.dashboard.message')}</label><textarea id={messageId} value={message} maxLength={500} placeholder={t('next.dashboard.messagePlaceholder')} aria-describedby={`${messageId}-help`} disabled={busy} onChange={event => setMessage(event.target.value)} /><small id={`${messageId}-help`}><span>{t('next.dashboard.messageHelp')}</span><span aria-hidden="true">{defaultI18n.number(message.length)}</span></small></div>
  </Dialog>;
  if (layout === 'phone') {
    const route = phoneRoute(tracks), routeLabel = route && formatTrackRoute(route), paused = s.state === 'paused';
    const facts = [
      (s.location === 'local' || s.location === 'remote') && <span className={styles.phoneFact} key="location">{t(s.location === 'local' ? 'next.kit.session.local' : 'next.kit.session.remote')}</span>,
      routeLabel && <span className={styles.phoneFact} key="route" data-route={route!.route}>{routeLabel}</span>,
      current != null && <span className={styles.phoneFact} key="rate">{formatStreamRate(current)}</span>,
    ].filter(Boolean);
    return <article className={styles.phoneSession} data-state={s.state}
      aria-label={[title, s.user, s.profile && s.profile !== s.user ? s.profile : undefined, s.device].filter(Boolean).join(', ')}>
      <div className={styles.phoneSessionRow}>
        <div className={styles.phoneArt} data-kind={s.live ? 'live' : square ? 'square' : 'video'}>
          {s.live ? <><Icon name="i-live" size={22} /><span>{s.channel?.name}</span></>
            : square ? <>{backdrop && <Artwork source={backdrop} alt="" width="100%" height="100%" />}<span className={styles.phoneSquare}>{art ? <Artwork source={art} alt="" width="100%" height="100%" /> : <Icon name="i-music" size={22} />}</span></>
            : (backdrop || art) ? <Artwork source={backdrop ?? art!} alt="" width="100%" height="100%" /> : <Icon name="i-play" size={26} />}
        </div>
        <div className={styles.phoneSessionText}>
          {title && <h3>{title}</h3>}
          {episode ? <p>{item!.seasonNumber !== undefined && item!.episodeNumber !== undefined && <><span>{t('title.episodeCode', {season: item!.seasonNumber, episode: item!.episodeNumber})}</span><i className={styles.dotSep} aria-hidden="true" /></>}<span>{s.title}</span></p>
            : s.live ? ((s.channel?.name || s.programme) ? <p>{s.channel?.name && <span>{s.channel.name}</span>}{s.channel?.name && s.programme && <i className={styles.dotSep} aria-hidden="true" />}{s.programme && <span>{t('next.dashboard.format.ends', {time: defaultI18n.time(Date.parse(s.programme.endsAt))})}</span>}</p> : null)
            : square ? ((item?.artist || item?.author || item?.album) ? <p>{(item?.artist || item?.author) && <span>{item.artist || item.author}</span>}{(item?.artist || item?.author) && (item?.album || (runtime && item?.kind === 'audiobook_file')) && <i className={styles.dotSep} aria-hidden="true" />}{(item?.album || (runtime && item?.kind === 'audiobook_file')) && <span>{item?.album || runtime}</span>}</p> : null)
            : item?.year ? <p><span>{item.year}</span></p> : null}
          <div className={styles.phoneWho}><span className={styles.phoneAvatar} style={accountAvatarStyle(s.user)} aria-hidden="true">{s.user.slice(0, 1).toUpperCase()}</span><b>{s.user}</b>{s.profile && s.profile !== s.user && <span>{s.profile}</span>}<span>{s.device}</span></div>
        </div>
        {paused && <span className={styles.phoneState}><Icon name="i-pause" size={12} />{t('next.dashboard.paused')}</span>}
      </div>
      {!!facts.length && <div className={styles.phoneFactRow}>{facts}</div>}
      <div className={styles.phoneSessionActions}>
        <span className={styles.phonePosition}>{s.live ? (s.startedAt ? t('next.dashboard.started', {time: defaultI18n.time(Date.parse(s.startedAt))}) : undefined)
          : s.durationMs ? t('next.kit.session.position', {position: t('next.kit.session.positionOf', {position: clock(s.positionMs), duration: clock(s.durationMs)})}) : clock(s.positionMs)}</span>
        {b.role === 'owner' && stopTitle && <Button size="xs" variant="glass" icon="i-stop" className={styles.phoneStop} onClick={() => {setMessage(''); setStop(true);}}>{t('next.dashboard.stop')}</Button>}
        {!retained && sessionMenu}
      </div>
      {!s.live && !!s.durationMs && <div className={styles.phoneProgress} aria-hidden="true"><i style={{width: `${Math.min(100, Math.max(0, s.positionMs / s.durationMs * 100))}%`}} /></div>}
      {!retained && stopDialog}
    </article>;
  }
  return <article className={styles.session} data-state={s.state} aria-label={title}>
    {backdrop && <div className={styles.ambient} aria-hidden="true"><Artwork source={backdrop} alt="" width="100%" height="100%" /></div>}
    <div className={styles.art} data-square={square || undefined}>{art ? <Artwork source={art} alt="" width="100%" height="100%" /> : <Icon name={square ? 'i-music' : s.live ? 'i-live' : 'i-play'} size={28} />}</div>
    <div className={styles.info}>{title && <h4>{title}</h4>}{episode ? <><p>{item?.seasonNumber !== undefined && item?.episodeNumber !== undefined && <b>{t('title.episodeCode', {season: item.seasonNumber, episode: item.episodeNumber})}</b>}{s.title}</p>{s.durationMs && <p className={styles.secondaryMeta}>{t('next.dashboard.format.runtimeMinutes', {minutes: Math.round(s.durationMs / 60000)})}</p>}</>
      : s.live ? <>{s.channel && <p>{s.channel.name}</p>}{s.programme && <p className={styles.secondaryMeta}>{t('next.dashboard.format.ends', {time: defaultI18n.time(Date.parse(s.programme.endsAt))})}</p>}</>
      : square ? <>{(item?.artist || item?.author) && <p>{item.artist || item.author}</p>}{(item?.album || runtime && item?.kind === 'audiobook_file') && <p className={styles.secondaryMeta}>{item?.album || runtime}</p>}</>
      : (item?.year || runtime) ? <p>{item?.year}{item?.year && runtime && <i className={styles.metaGap} />}{runtime}</p> : null}
      <div className={styles.playState}>{state && <b><Icon name={s.state === 'paused' ? 'i-pause' : s.state === 'buffering' ? 'i-refresh' : 'i-play'} size={14} />{state}</b>}<span>{s.live ? t('next.dashboard.live') : s.durationMs ? t('next.kit.session.positionOf', {position: clock(s.positionMs), duration: clock(s.durationMs)}) : clock(s.positionMs)}</span></div>
      {!s.live && !!s.durationMs && <progress aria-label={title} value={s.positionMs} max={s.durationMs} />}
    </div>
    <div className={styles.tracks} style={{'--track-count': tracks.length} as CSSProperties}>{tracks.map(track => {
      const label = formatTrackRoute(track);
      return <div className={styles.track} key={track.kind}><span className={styles.source} title={t(`next.dashboard.${track.kind}`)}><TrackIcon kind={track.kind} /><span>{track.source}</span></span>{label && <><svg className={styles.trackArrow} width="20" height="10" viewBox="0 0 20 10" aria-hidden="true"><path d="M0 5h18M14 1.2 18.4 5 14 8.8" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" /></svg><span className={styles.delivery} data-route={track.route}><i className={styles.routeDot} aria-hidden="true" /><span>{label}</span>{track.hardware && <b>{t('next.dashboard.hardware')}</b>}</span><span className={styles.trackStub} aria-hidden="true" /></>}</div>;
    })}</div>
    <div className={styles.device}><span className={styles.appIcon}><DeviceGlyph platform={s.platform} device={s.device} /></span><div>{s.app && <b>{s.app}</b>}{s.device && <span>{s.device}</span>}{(s.location === 'local' || s.location === 'remote' || current != null) && <strong className={styles.network}>{(s.location === 'local' || s.location === 'remote') && <><i data-series={s.location === 'remote' ? 'away' : 'home'} />{t(s.location === 'local' ? 'next.dashboard.home' : 'next.dashboard.away')}</>}{current != null && <em>{formatStreamRate(current)}</em>}</strong>}{s.address && <small>{s.address}</small>}</div></div>
    <div className={styles.who}><span className={styles.avatar} style={accountAvatarStyle(s.user)} aria-hidden="true">{s.user.slice(0, 1).toUpperCase()}</span><b>{s.user}{s.profile && s.profile !== s.user && <small>{s.profile}</small>}</b>
      {!retained && sessionMenu}
    </div>
    {!retained && stopDialog}
  </article>;
}
