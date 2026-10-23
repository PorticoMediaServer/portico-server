import {useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {DashboardSession as AdminSession} from './bindings';
import {Button, TextField} from '../../kit/controls';
import {Dialog, Menu} from '../../kit/overlays';
import {Icon} from '../../kit/Icon';
import type {DashboardBindings} from './bindings';
import {clock, sessionTracks} from './model';
import styles from './dashboard.module.css';
const t = defaultI18n.t;
export function SessionCard({session: s, bindings: b, refresh}: {session: AdminSession; bindings: DashboardBindings; refresh: () => void}) {
  const [menu, setMenu] = useState(false), [stop, setStop] = useState(false), [message, setMessage] = useState(''), [busy, setBusy] = useState(false), [failed, setFailed] = useState(false);
  const opener = useRef<HTMLButtonElement>(null), square = s.kind === 'audio', item = s.item;
  const art = s.itemId ? b.artwork(s.itemId, square ? 'square' : 'poster', item?.art.find(a => a.role === (square ? 'square' : 'poster'))?.revision) : undefined;
  const backdrop = s.itemId ? b.artwork(s.itemId, 'backdrop', item?.art.find(a => a.role === 'backdrop')?.revision) : undefined;
  const act = async (action: () => Promise<void>) => {if (!b.isCurrent() || busy) return; setBusy(true); setFailed(false); try {await action(); if (b.isCurrent()) {setStop(false); refresh();}} catch (error) {if (b.isCurrent()) {setFailed(true); b.effects.reportError(error);}} finally {setBusy(false);}};
  const state = s.state === 'playing' ? t('next.dashboard.playing') : s.state === 'paused' ? t('next.dashboard.paused') : s.state === 'buffering' ? t('next.dashboard.buffering') : s.state === 'preparing' ? t('next.dashboard.preparing') : t('next.dashboard.stateUnknown');
  const tracks = sessionTracks(s), current = s.currentBitrateKbps, rate = (n: number) => (n / 1000).toLocaleString(defaultI18n.locale, {maximumFractionDigits: 1});
  return <article className={styles.session} data-state={s.state} aria-label={s.title}>
    {backdrop && <div className={styles.ambient} aria-hidden="true"><img src={backdrop} alt="" /></div>}
    <div className={styles.art} data-square={square || undefined}>{art ? <img src={art} alt="" loading="lazy" /> : <Icon name={square ? 'i-music' : s.live ? 'i-live' : 'i-play'} size={28} />}</div>
    <div className={styles.info}><h4>{s.title}</h4>{item?.showTitle && <p>{item.showTitle} {item.seasonNumber !== undefined && item.episodeNumber !== undefined && t('title.episodeCode', {season: item.seasonNumber, episode: item.episodeNumber})}</p>}
      {s.live && s.channel && <p>{[s.channel.number, s.channel.name].filter(Boolean).join(' ')}</p>}{s.live && s.programme && <p>{s.programme.title} {defaultI18n.time(Date.parse(s.programme.endsAt))}</p>}
      <p>{[item?.year, item?.artist, item?.album, item?.author, item?.bookTitle].filter(Boolean).join(' / ')}</p>
      <div className={styles.playState}><b>{state}</b><span>{s.live ? t('next.dashboard.live') : s.durationMs ? `${clock(s.positionMs)} / ${clock(s.durationMs)}` : clock(s.positionMs)}</span></div>
      {!s.live && !!s.durationMs && <progress aria-label={s.title} value={s.positionMs} max={s.durationMs} />}
    </div>
    <div className={styles.tracks}>{tracks.map(track => <div className={styles.track} key={track.kind}><span className={styles.source} title={track.source}><Icon name={track.kind === 'audio' ? 'i-audio' : track.kind === 'subtitles' ? 'i-subs' : 'i-play'} size={16} /><span>{track.source || t(`next.dashboard.${track.kind}`)}</span></span><span aria-hidden="true">›</span><span className={styles.delivery} data-converts={['transcode', 'burn'].includes(track.action) || undefined}>{track.action === 'direct' ? t('next.dashboard.direct') : track.action === 'copy' ? t('next.dashboard.copy') : track.action === 'burn' ? t('next.dashboard.burn') : track.action === 'transcode' ? track.output ? t('next.dashboard.convert', {target: track.output}) : t('next.dashboard.converting') : track.action === 'off' ? t('next.dashboard.off') : t('next.dashboard.modeUnknown')}{track.hardware && <b>{t('next.dashboard.hardware')}</b>}</span></div>)}</div>
    <div className={styles.device}><Icon name="i-device" size={24} /><div><b>{s.app || s.platform}</b><span>{s.device}</span><strong data-series={s.location === 'remote' ? 'away' : 'home'}>{s.location === 'local' ? t('next.dashboard.home') : s.location === 'remote' ? t('next.dashboard.away') : t('next.dashboard.unknownLocation')}{current !== null && current !== undefined ? ` ${t('next.dashboard.mbps', {value: rate(current)})}` : ''}</strong>{(current === null || current === undefined) && <small>{t('next.dashboard.rateUnknown')}</small>}{s.bitrateKbps !== undefined && <small>{t('next.dashboard.configuredRate', {value: rate(s.bitrateKbps)})}</small>}<small>{s.address}</small></div></div>
    <div className={styles.who}><span className={styles.avatar} aria-hidden="true">{s.user.slice(0, 1)}</span><b>{s.user}</b>{s.profile && s.profile !== s.user && <small>{s.profile}</small>}
      <Menu open={menu} onOpenChange={setMenu} title={t('next.dashboard.sessionMenu', {title: s.title})} trigger={<Button ref={opener} icon="i-more" iconOnly size="xs" disabled={busy} aria-label={t('next.dashboard.sessionMenu', {title: s.title})} />} items={[
        ...(['paused', 'playing'].includes(s.state) ? [{id: 'transport', label: s.state === 'paused' ? t('next.dashboard.resume') : t('next.dashboard.pause'), onSelect: () => {void act(() => b.effects.transport(s.id, s.state === 'paused' ? 'resume' : 'pause'));}}] : []),
        ...(b.role === 'owner' ? [{id: 'stop', label: t('next.dashboard.stop'), destructive: true, onSelect: () => {setMessage(''); setStop(true);}}] : []),
      ]} />
    </div>
    {failed && <p className={styles.actionError} role="alert">{t('next.dashboard.error')}</p>}
    <Dialog open={stop} onOpenChange={value => {if (!busy) setStop(value);}} title={t('next.dashboard.stopTitle')} description={t('next.dashboard.stopDetail', {person: s.user, device: s.device})} returnFocusRef={opener}
      footer={<><Button size="compact" disabled={busy} onClick={() => setStop(false)}>{t('next.dashboard.cancel')}</Button><Button size="compact" variant="primary" loading={busy} onClick={() => {void act(() => b.effects.stop(s.id, message.trim() || undefined));}}>{t('next.dashboard.stop')}</Button></>}>
      <TextField label={t('next.dashboard.message')} value={message} maxLength={500} onChange={event => setMessage(event.target.value)} />{failed && <p role="alert">{t('next.dashboard.error')}</p>}
    </Dialog>
  </article>;
}
