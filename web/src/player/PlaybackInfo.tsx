import React, {useEffect, useState} from 'react';
import type {HttpLocalApi} from '@core/index.ts';
import {parseSession} from '@core/playback-v1/types.ts';
import {playbackInfoRows, type PlaybackInfoFacts} from '@core/presentation/index.ts';
import {useI18n} from '../app/i18n';
import {IconButton} from '../ui';
import s from './Player.module.css';

type Source = NonNullable<PlaybackInfoFacts['source']>;

/**
 * Playback information, under More: how the stream on screen is delivered and why, its codecs and
 * size, the bitrate, and what this player measures (dropped frames, buffer). The session's own
 * account is read once when the panel opens; the measured rows refresh each second while it is up.
 */
export function PlaybackInfoPanel({api, sessionId, v1, delivery, chosenQuality, source, video, audioOnly, onClose}: {api: HttpLocalApi; sessionId?: string; /** A Playback v1 session can say what it does to each stream. */ v1: boolean; delivery?: 'direct' | 'hls'; chosenQuality?: string; source?: Source; video: HTMLVideoElement | null; audioOnly: boolean; onClose: () => void}) {
  const i18n = useI18n();
  const [session, setSession] = useState<Pick<PlaybackInfoFacts, 'decision' | 'bitrateKbps' | 'hdr'>>({});
  useEffect(() => {
    if (!sessionId || !v1) { setSession({}); return; }
    let live = true;
    api.request<unknown>(`/v1/playback/sessions/${encodeURIComponent(sessionId)}`, 'GET').then(raw => {
      if (!live) return;
      try {
        const p = parseSession(raw).presentation;
        setSession({decision: p.decision, bitrateKbps: p.bitrateKbps, hdr: p.hdr});
      } catch { setSession({}); }
    }, () => {});
    return () => { live = false; };
  }, [api, sessionId, v1]);
  const [measured, setMeasured] = useState<Pick<PlaybackInfoFacts, 'shown' | 'droppedFrames' | 'totalFrames' | 'bufferSeconds'>>({});
  useEffect(() => {
    const read = () => {
      if (!video) return;
      const quality = typeof video.getVideoPlaybackQuality === 'function' ? video.getVideoPlaybackQuality() : undefined;
      let ahead = 0;
      for (let i = 0; i < video.buffered.length; i++) if (video.buffered.start(i) <= video.currentTime && video.buffered.end(i) >= video.currentTime) ahead = video.buffered.end(i) - video.currentTime;
      setMeasured({shown: {width: video.videoWidth, height: video.videoHeight}, droppedFrames: quality?.droppedVideoFrames, totalFrames: quality?.totalVideoFrames, bufferSeconds: ahead});
    };
    read();
    const timer = setInterval(read, 1000);
    return () => clearInterval(timer);
  }, [video]);
  const rows = playbackInfoRows({...session, ...measured, delivery: delivery ? (delivery === 'hls' ? 'converted' : 'direct') : undefined, chosenQuality, source, audioOnly}, i18n.t);
  return (
    <section className={s.info} aria-label={i18n.t('player.opt.info')}>
      <header className={s.infoHead}>
        <span className={s.infoTitle}>{i18n.t('player.opt.info')}</span>
        <IconButton name="close" label={i18n.t('action.close')} variant="ghost" size="sm" onClick={onClose} />
      </header>
      <dl className={s.infoRows}>
        {rows.map(row => <React.Fragment key={row.id}><dt>{row.label}</dt><dd>{row.value}</dd></React.Fragment>)}
      </dl>
    </section>
  );
}
