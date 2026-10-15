import React, {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {playbackControlsService} from '@core/playback-controls.ts';
import {AdminSessionsClient, NowPlayingStore} from '@core/playback-v1/admin-sessions.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import type {AdminSession} from '@core/playback-v1/types.ts';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {useV1Events} from '../../player/engine';
import {problem, useAction} from '../../admin/console';
import {NOW_PLAYING_ROW_PX, REMOTE_SKIP_MS, STOP_MESSAGE_MAX, clampToLength, driveNowPlaying, offersScrubber, playedFraction, positionClock, sendSeekTo, sendTransport, sessionFacts, sessionViewer, stopMessage, transportOffers, windowRange, type TransportCommand} from '../../admin/now-playing';
import {Badge, Button, ConfirmDialog, Freshness, Menu, Notice, Skeleton, StateView, Surface, Text, TextArea, type IconName, type MenuItem} from '../../ui';
import s from './Server.module.css';
import css from './NowPlaying.module.css';

/**
 * Server › Overview "Watching now": the server's Playback v1 sessions (`GET /v1/admin/sessions`).
 * Each row has Stop (with an optional message for the viewer) and a menu: Pause or Resume, Back
 * and Forward 30 seconds and Start over where there is a timeline, and the title's page.
 */
export function NowPlayingPanel() {
  return <V1NowPlaying />;
}

function V1NowPlaying() {
  const {api} = useSession();
  const {t} = useI18n();
  const events = useV1Events();
  const store = useMemo(() => new NowPlayingStore(new AdminSessionsClient(localV1Http(api))), [api]);
  const snap = useSyncExternalStore(store.subscribe, store.getSnapshot);
  useEffect(() => driveNowPlaying(() => void store.refresh(), events), [store, events]);
  const action = useAction();
  const controls = useMemo(() => playbackControlsService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  // Transport is not a change to the server: it has its own small state, and the list follows it by the feed.
  const [sending, setSending] = useState(false);
  const [refused, setRefused] = useState('');
  const send = (x: AdminSession, command: TransportCommand | {to: number}) => {
    if (sending) return;
    setSending(true); setRefused('');
    const sent = typeof command === 'object' ? sendSeekTo(controls, x.id, command.to) : sendTransport(controls, x.id, command);
    sent.then(() => {}, e => setRefused(problem(e, 'action'))).finally(() => { setSending(false); void store.refresh(); });
  };
  // Move to…: a position bar for the whole title, in a small dialog of its own.
  const [moving, setMoving] = useState<AdminSession | null>(null);
  const [target, setTarget] = useState(0);
  const startMove = (x: AdminSession) => { setMoving(x); setTarget(x.positionMs); };
  const [stop, setStop] = useState<AdminSession | null>(null);
  const [message, setMessage] = useState('');
  const items = snap.sessions;
  const loaded = snap.updatedAt !== undefined;
  const open = (x: AdminSession | null) => { setStop(x); setMessage(''); action.clear(); };
  const viewer = (x: AdminSession) => sessionViewer(x, t);
  return (
    <Surface padless>
      <div className={s.panelHead} style={{padding: '16px 16px 0'}}>
        <span className={s.panelTitle}>{t('web.nowPlaying.title')}{items.length ? <> <Badge tone="accent">{items.length}</Badge></> : null}</span>
        <Freshness at={snap.updatedAt} staleAfterMs={60_000} onRefresh={() => void store.refresh()} refreshing={snap.loading} />
      </div>
      {snap.error && !loaded ? <div style={{padding: 16}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => void store.refresh()}}>{problem(snap.error)}</Notice></div> : null}
      {!loaded && !snap.error ? <div className={css.pending}><Skeleton height={14} width="60%" /><Skeleton height={14} width="45%" /></div> : null}
      {loaded && !items.length ? <StateView inline icon="play" title={t('web.nowPlaying.empty')} /> : null}
      {refused ? <div style={{padding: '12px 16px 0'}}><Notice tone="error" compact>{refused}</Notice></div> : null}
      {items.length ? <SessionList items={items} busy={sending} onStop={open} onSend={send} onMove={startMove} /> : null}
      <ConfirmDialog
        open={!!moving}
        onOpenChange={o => !o && setMoving(null)}
        title={t('web.nowPlaying.moveTitle', {title: moving?.title || t('web.nowPlaying.untitled')})}
        body={moving?.durationMs ? (
          <div style={{display: 'grid', gap: 8}}>
            <span>{t('web.nowPlaying.moveBody', {viewer: viewer(moving)})}</span>
            <input type="range" className={css.scrub} min={0} max={moving.durationMs} step={1000} value={target} aria-label={t('player.position')}
              aria-valuetext={t('player.clockOf', {position: positionClock(target), duration: positionClock(moving.durationMs)})} onChange={e => setTarget(clampToLength(Number(e.target.value), moving.durationMs))} />
            <Text variant="caption" tone="secondary">{t('player.clockOf', {position: positionClock(target), duration: positionClock(moving.durationMs)})}</Text>
          </div>
        ) : undefined}
        confirmLabel={t('web.nowPlaying.moveConfirm')}
        busy={sending}
        onConfirm={() => { if (moving) { send(moving, {to: target}); setMoving(null); } }}
      />
      <ConfirmDialog
        open={!!stop}
        onOpenChange={o => !o && open(null)}
        title={t('web.nowPlaying.stopTitle', {title: stop?.title || t('web.nowPlaying.untitled')})}
        body={stop ? (
          <div style={{display: 'grid', gap: 12}}>
            <span>{t('web.nowPlaying.stopBody', {viewer: viewer(stop)})}</span>
            <TextArea label={t('web.nowPlaying.messageLabel')} optional help={t('web.nowPlaying.messageHelp')} placeholder={t('web.nowPlaying.messagePlaceholder')} value={message} maxLength={STOP_MESSAGE_MAX} rows={3} onChange={e => setMessage(e.target.value)} />
          </div>
        ) : undefined}
        confirmLabel={t('web.nowPlaying.stopConfirm')}
        busy={action.busy}
        error={action.error || undefined}
        onConfirm={() => { if (stop) void action.run(() => store.terminate(stop.id, stopMessage(message))).then(ok => { if (ok) open(null); }); }}
      />
    </Surface>
  );
}

/** Only the rows in view are rendered (the scale rule), at a fixed pitch. */
const transportIcons: Record<TransportCommand, IconName> = {pause: 'pause', resume: 'play', back: 'arrowLeft', forward: 'arrowRight', startOver: 'restart'};

function SessionList({items, busy, onStop, onSend, onMove}: {items: readonly AdminSession[]; busy: boolean; onStop: (x: AdminSession) => void; onSend: (x: AdminSession, command: TransportCommand) => void; onMove: (x: AdminSession) => void}) {
  const {t} = useI18n();
  const navigate = useNavigate();
  const box = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({top: 0, height: 0});
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const measure = () => setView({top: el.scrollTop, height: el.clientHeight});
    measure();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(measure);
    observer?.observe(el);
    return () => observer?.disconnect();
  }, []);
  const {first, last} = windowRange(view.top, view.height, items.length);
  const meta = (x: AdminSession) => sessionFacts(x, t);
  const seconds = REMOTE_SKIP_MS / 1000;
  const words: Record<TransportCommand, string> = {pause: t('action.pause'), resume: t('entry.resume'), back: t('player.skipBackSeconds', {seconds}), forward: t('player.skipForwardSeconds', {seconds}), startOver: t('action.startOver')};
  const menu = (x: AdminSession): MenuItem[] => [
    ...transportOffers(x).map(command => ({id: command, label: words[command], icon: transportIcons[command], disabled: busy})),
    ...(offersScrubber(x) ? [{id: 'move', label: t('web.nowPlaying.moveTo'), icon: 'clock' as IconName, disabled: busy}] : []),
    ...(x.itemId ? [{id: 'open', label: t('action.details'), icon: 'info' as IconName, separatorBefore: true}] : []),
  ];
  return (
    <div ref={box} className={css.list} role="list" aria-label={t('web.nowPlaying.listLabel')} onScroll={e => setView({top: e.currentTarget.scrollTop, height: e.currentTarget.clientHeight})}>
      <div className={css.track} style={{height: items.length * NOW_PLAYING_ROW_PX}}>
        {items.slice(first, last + 1).map((x, i) => (
          <div key={x.id} role="listitem" className={first + i === 0 ? `${css.row} ${css.first}` : css.row} style={{top: (first + i) * NOW_PLAYING_ROW_PX}}>
            <div className={css.copy}>
              <Text variant="bodyStrong" clamp={1}>{x.title || t('web.nowPlaying.untitled')}</Text>
              <span className={css.meta} title={meta(x)}>{meta(x)}</span>
            </div>
            <div className={css.actions}>
              <Button size="sm" variant="secondary" label={t('web.nowPlaying.stop')} onClick={() => onStop(x)} />
              <Menu label={x.title || t('web.nowPlaying.untitled')} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.nowPlaying.controls', {title: x.title || t('web.nowPlaying.untitled')})} />} items={menu(x)}
                onSelect={id => { if (id === 'open') { if (x.itemId) void navigate({to: '/media/$itemId', params: {itemId: x.itemId}, search: {}}); } else if (id === 'move') onMove(x); else onSend(x, id as TransportCommand); }} />
            </div>
            {playedFraction(x) !== undefined ? <span className={css.progress} aria-hidden style={{width: `${playedFraction(x)! * 100}%`}} /> : null}
          </div>
        ))}
      </div>
    </div>
  );
}
