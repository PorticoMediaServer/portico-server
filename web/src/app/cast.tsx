import React, {createContext, useContext, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {CastSender, type CastSnapshot} from '../bridge/cast';
import {Artwork, Button, Dialog, IconButton, Menu, Text, type MenuItem} from '../ui';
import {castMeta} from '@core/presentation/index.ts';
import {usePlayer} from '../player/engine';
import {useSession} from './session';
import {useServerPreferences} from './server-preferences';
import {useI18n} from './i18n';
import s from './cast.module.css';

const Ctx = createContext<CastSender | null>(null);

/** One Cast sender per session, and the small bar that stands in for the player while a
 * television is doing the playing. */
export function CastProvider({children}: {children: React.ReactNode}) {
  const {api, session} = useSession();
  // D-FEAT-1: Cast is offered whenever the server has a secure HTTPS address, however the viewer
  // signed in; `prepare()` checks the address and the server's Cast configuration.
  const key = session ? `${session.viewer.serverId}/${session.viewer.profileId}` : '';
  const sender = useMemo(() => (key ? new CastSender(api) : null), [api, key]);
  // PERF-25: the server check and Google's sender script wait for an idle
  // moment or the first player open, never the launch path.
  const playerOpen = usePlayer().active;
  useEffect(() => {
    if (!sender) return;
    let done = false;
    const prepare = () => { if (!done) { done = true; void sender.prepare(); } };
    if (playerOpen) { prepare(); return; }
    const w = window as Window & {requestIdleCallback?: (fn: () => void, o?: {timeout: number}) => number; cancelIdleCallback?: (id: number) => void};
    if (w.requestIdleCallback) {
      const id = w.requestIdleCallback(prepare, {timeout: 10000});
      return () => w.cancelIdleCallback?.(id);
    }
    const t = setTimeout(prepare, 3000);
    return () => clearTimeout(t);
  }, [sender, playerOpen]);
  useEffect(() => () => { if (sender?.getSnapshot().phase === 'casting') sender.stop(); sender?.dispose(); }, [sender]);
  return <Ctx.Provider value={sender}>{children}<CastBar /></Ctx.Provider>;
}

const off: CastSnapshot = Object.freeze<CastSnapshot>({available: false, phase: 'off', positionSeconds: 0, durationSeconds: 0, paused: false, audioTracks: [], textTracks: []});
const noSubscribe = () => () => {};
export function useCast(): {sender: CastSender | null; state: CastSnapshot} {
  const sender = useContext(Ctx);
  const state = useSyncExternalStore(sender?.subscribe ?? noSubscribe, sender?.getSnapshot ?? (() => off));
  return {sender, state};
}

const clock = (seconds: number) => { const s = Math.max(0, Math.floor(seconds)); const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60); return `${h ? h + ':' + String(m).padStart(2, '0') : m}:${String(s % 60).padStart(2, '0')}`; };

function CastBar() {
  const {sender, state} = useCast();
  const i18n = useI18n();
  const player = usePlayer();
  const prefs = useServerPreferences();
  const skipBack = prefs.value<number>('playback.skipBackSeconds', 10);
  const skipForward = prefs.value<number>('playback.skipForwardSeconds', 30);
  // CAST-05: stopping the TV offers to carry on here from where it was.
  const [resume, setResume] = useState<{itemId: string; position: number; title?: string}>();
  // MU4 CAST-03: the expanded controller sheet.
  const [expanded, setExpanded] = useState(false);
  useEffect(() => { if (!resume) return; const t = setTimeout(() => setResume(undefined), 15000); return () => clearTimeout(t); }, [resume]);
  useEffect(() => { if (state.phase !== 'casting') setExpanded(false); }, [state.phase]);
  // The title's display facts, remembered where the cast started (no request per open).
  const meta = castMeta(state.itemId);
  const title = state.title ?? meta?.title ?? i18n.t('cast.region');
  const stopCasting = () => { if (state.phase === 'casting' && state.itemId) setResume({itemId: state.itemId, position: state.positionSeconds, title: state.title ?? meta?.title}); sender?.stop(); };
  if (sender && state.phase === 'off' && resume) {
    return (
      <div role="region" aria-label={i18n.t('cast.region')} className={s.bar}>
        <div className={s.copy}><Text variant="bodyStrong" clamp={1}>{resume.title ?? i18n.t('cast.ended')}</Text><Text variant="caption" tone="tertiary">{i18n.t('web.cast.stopped')}</Text></div>
        <Button size="sm" variant="primary" label={i18n.t('web.cast.continueHere', {position: clock(resume.position)})} onClick={() => { player.play(resume.itemId, resume.position); setResume(undefined); }} />
        <Button size="sm" variant="ghost" label={i18n.t('action.dismiss')} onClick={() => setResume(undefined)} />
      </div>
    );
  }
  if (!sender || state.phase === 'off') return null;
  const casting = state.phase === 'casting';
  const ended = state.phase === 'error' || state.phase === 'replaced';
  const line = state.phase === 'error' ? state.error
    : state.phase === 'replaced' ? i18n.t('cast.replaced')
    : state.phase === 'connecting' ? i18n.t('playOn.chooseDevice')
    : state.phase === 'pairing' ? i18n.t('playOn.connectingTo', {device: state.deviceName ?? i18n.t('cast.theTV')})
    : state.phase === 'waiting' ? i18n.t('cast.waitingForAllow')
    : state.durationSeconds ? i18n.t('cast.positionOf', {device: state.deviceName ?? '', position: clock(state.positionSeconds), duration: clock(state.durationSeconds)}) : i18n.t('cast.position', {device: state.deviceName ?? '', position: clock(state.positionSeconds)});
  return (
    <div role="region" aria-label={i18n.t('cast.region')} className={s.bar}>
      {casting && meta?.artworkPath ? <div className={s.thumb} aria-hidden><Artwork path={meta.artworkPath} shape="square" alt="" /></div> : null}
      <div className={s.copy} aria-live="polite">
        <Text variant="bodyStrong" clamp={1}>{state.phase === 'error' ? i18n.t('cast.stopped') : state.phase === 'replaced' ? i18n.t('cast.ended') : title}</Text>
        <Text variant="caption" tone={state.phase === 'error' ? 'warning' : 'tertiary'} clamp={2}>{line}</Text>
      </div>
      {casting ? <IconButton variant="ghost" name="maximize" label={i18n.t('cast.openControls')} onClick={() => setExpanded(true)} /> : null}
      {casting ? <IconButton variant="ghost" name="back10" label={i18n.t('player.skipBackSeconds', {seconds: 10})} onClick={() => sender.seek(state.positionSeconds - 10)} /> : null}
      {casting ? <IconButton variant="secondary" name={state.paused ? 'play' : 'pause'} label={i18n.t(state.paused ? 'action.play' : 'action.pause')} onClick={() => sender.toggle()} /> : null}
      {casting ? <IconButton variant="ghost" name="forward10" label={i18n.t('player.skipForwardSeconds', {seconds: 10})} onClick={() => sender.seek(state.positionSeconds + 10)} /> : null}
      {casting && (state.textTracks.length || state.audioTracks.length > 1) ? <Menu label={i18n.t('web.cast.tracks')} trigger={<IconButton variant="ghost" name="subtitles" label={i18n.t('web.cast.tracks')} />} items={trackItems(state, i18n.t)} onSelect={id => { const [kind, value] = id.split(':'); if (kind === 'text') sender.setSubtitleTrack(value === 'off' ? null : Number(value)); else sender.setAudioTrack(Number(value)); }} /> : null}
      <Button size="sm" variant="ghost" label={i18n.t(ended ? 'action.dismiss' : 'action.stop')} onClick={() => { if (ended) { sender.dismiss(); return; } stopCasting(); }} />
      {casting && state.durationSeconds > 0 ? <div className={s.progress} role="progressbar" aria-label={i18n.t('player.position')} aria-valuemin={0} aria-valuemax={Math.round(state.durationSeconds)} aria-valuenow={Math.round(state.positionSeconds)}><i style={{width: `${Math.min(100, (state.positionSeconds / state.durationSeconds) * 100)}%`}} /></div> : null}
      {casting && state.takeover ? (
        <div className={s.prompt} role="alertdialog" aria-label={i18n.t('cast.takeover.title')}>
          <Text variant="body" className={s.promptText}>{i18n.t('cast.takeover.body', {requester: state.takeover.requester})}</Text>
          <Button size="sm" variant="ghost" label={i18n.t('cast.takeover.deny')} onClick={() => sender.answerTakeover(false)} />
          <Button size="sm" variant="primary" label={i18n.t('action.allow')} onClick={() => sender.answerTakeover(true)} />
        </div>
      ) : null}
      {/* MU4 CAST-03: the expanded controller, opened from the Cast bar. No volume: the v1
          receiver has no volume command (see Questions), so none is offered. */}
      {casting ? (
        <Dialog open={expanded} onOpenChange={setExpanded} title={title} description={state.deviceName ?? i18n.t('cast.theTV')} width={440}>
          <div className={s.sheet}>
            {meta?.artworkPath ? <div className={s.poster}><Artwork path={meta.artworkPath} shape="poster" alt="" /></div> : null}
            <div className={s.sheetMain}>
              {meta?.subtitle ? <Text variant="caption" tone="tertiary" clamp={1}>{meta.subtitle}</Text> : null}
              {state.durationSeconds > 0 ? (
                <div className={s.scrubRow}>
                  <span className={s.time}>{clock(state.positionSeconds)}</span>
                  <input className={s.scrub} type="range" min={0} max={state.durationSeconds} step={1} value={Math.min(state.durationSeconds, state.positionSeconds)} aria-label={i18n.t('player.position')} onChange={e => sender?.seek(Number(e.target.value))} />
                  <span className={s.time}>{clock(state.durationSeconds)}</span>
                </div>
              ) : null}
              <div className={s.sheetTransport}>
                <IconButton variant="ghost" name="back10" label={i18n.t('player.skipBackSeconds', {seconds: skipBack})} onClick={() => sender?.seek(state.positionSeconds - skipBack)} />
                <IconButton variant="secondary" name={state.paused ? 'play' : 'pause'} label={i18n.t(state.paused ? 'action.play' : 'action.pause')} onClick={() => sender?.toggle()} />
                <IconButton variant="ghost" name="forward10" label={i18n.t('player.skipForwardSeconds', {seconds: skipForward})} onClick={() => sender?.seek(state.positionSeconds + skipForward)} />
                {(state.textTracks.length || state.audioTracks.length > 1) ? <Menu label={i18n.t('web.cast.tracks')} trigger={<IconButton variant="ghost" name="subtitles" label={i18n.t('web.cast.tracks')} />} items={trackItems(state, i18n.t)} onSelect={id => { const [kind, value] = id.split(':'); if (kind === 'text') sender?.setSubtitleTrack(value === 'off' ? null : Number(value)); else sender?.setAudioTrack(Number(value)); }} /> : null}
                <Button size="sm" variant="ghost" label={i18n.t('action.stop')} onClick={() => { setExpanded(false); stopCasting(); }} />
              </div>
            </div>
          </div>
        </Dialog>
      ) : null}
    </div>
  );
}

/** CAST-03: the TV's subtitle and audio tracks, as the receiver reports them. */
function trackItems(state: CastSnapshot, t: ReturnType<typeof useI18n>['t']): MenuItem[] {
  const label = (track: {name: string; language: string}) => track.name || track.language || t('web.cast.track');
  const items: MenuItem[] = [];
  if (state.textTracks.length) {
    items.push({id: 'text:off', label: t('web.cast.subtitlesOff'), group: t('web.cast.subtitles')});
    for (const track of state.textTracks) items.push({id: `text:${track.id}`, label: label(track), group: t('web.cast.subtitles')});
  }
  if (state.audioTracks.length > 1) for (const track of state.audioTracks) items.push({id: `audio:${track.id}`, label: label(track), group: t('web.cast.audio')});
  return items;
}
