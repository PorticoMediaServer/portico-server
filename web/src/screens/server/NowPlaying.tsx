import React, {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {AdminSessionsClient, NowPlayingStore} from '@core/playback-v1/admin-sessions.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import type {AdminSession} from '@core/playback-v1/types.ts';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {useV1Events} from '../../player/engine';
import {problem, useAction} from '../../admin/console';
import {NOW_PLAYING_ROW_PX, STOP_MESSAGE_MAX, driveNowPlaying, sessionFacts, sessionViewer, stopMessage, windowRange} from '../../admin/now-playing';
import {Badge, Button, ConfirmDialog, Freshness, Notice, Skeleton, StateView, Surface, Text, TextArea} from '../../ui';
import s from './Server.module.css';
import css from './NowPlaying.module.css';

/**
 * Server › Overview "Watching now": the server's Playback v1 sessions (`GET /v1/admin/sessions`),
 * with Stop playback and an optional message for the viewer.
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
      {items.length ? <SessionList items={items} viewer={viewer} onStop={open} /> : null}
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
function SessionList({items, viewer, onStop}: {items: readonly AdminSession[]; viewer: (x: AdminSession) => string; onStop: (x: AdminSession) => void}) {
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
              {x.itemId ? <Button size="sm" variant="ghost" label={t('web.nowPlaying.open')} onClick={() => void navigate({to: '/media/$itemId', params: {itemId: x.itemId!}, search: {}})} /> : null}
              <Button size="sm" variant="danger" label={t('web.nowPlaying.stop')} onClick={() => onStop(x)} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
