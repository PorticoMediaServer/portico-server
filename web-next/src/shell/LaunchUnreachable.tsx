import {useCallback, useEffect, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Toast} from '../kit/overlays';
import {OwnerTag, ServerMark} from '../kit/shell';
import {DetailsPanel, StateLayout, type DetailsFact} from '../kit/states';
import type {LaunchUnreachable} from './bootstrap';
import {retryProgress, retrySeconds} from './connection-retry';
import {useShellWidth} from './viewport';
import {useOfflineDownloads} from './offline-downloads';
import styles from './states.module.css';

const t = defaultI18n.t;

/** states.html#3. The server the saved sign-in points at is not answering. The
 * page waits on the bootstrap's own retries, so the countdown is the real one
 * and Try again now asks immediately. Identity, access and version failures
 * never reach this page; they keep the ordinary error path. */
export function LaunchUnreachableScreen({facts, onRetry, onSwitchServer, now: fixedNow, initialDetailsOpen = false}: {
  facts: LaunchUnreachable;
  onRetry: () => void;
  onSwitchServer: () => void;
  /** Test seam only; the app always reads the real clock. */
  now?: number;
  initialDetailsOpen?: boolean;
}) {
  const width = useShellWidth(), [tick, setTick] = useState(() => Date.now()), [detailsOpen, setDetailsOpen] = useState(initialDetailsOpen), [copied, setCopied] = useState(false);
  const offlinePort = useOfflineDownloads(), [offline, setOffline] = useState(() => offlinePort?.snapshot()), [playing, setPlaying] = useState(() => offlinePort?.playback?.());
  // One stable callback: an inline ref would release and retire the host on every render.
  const attachSurface = offlinePort?.attachSurface;
  const surfaceRef = useCallback((element: HTMLVideoElement | null) => { attachSurface?.(element); }, [attachSurface]);
  useEffect(() => {
    if (!offlinePort) { setOffline(undefined); setPlaying(undefined); return; }
    const sync = () => { setOffline(offlinePort.snapshot()); setPlaying(offlinePort.playback?.()); };
    sync();
    return offlinePort.subscribe(sync);
  }, [offlinePort]);
  const now = fixedNow ?? tick, waiting = !facts.retrying;
  const computerOffline = facts.category === 'offline' || globalThis.navigator?.onLine === false;
  useEffect(() => {
    if (!waiting || fixedNow !== undefined) return;
    const timer = setInterval(() => setTick(Date.now()), 250);
    return () => clearInterval(timer);
  }, [waiting, fixedNow]);
  const address = facts.address || null;
  const connected = globalThis.navigator?.onLine !== false;
  const factsRows: DetailsFact[] = [
    ...(address ? [{id: 'address', label: t('next.states.address'), value: address, status: {label: t('next.states.noAnswer'), tone: 'warning' as const}}] : []),
    {id: 'computer', label: t('next.states.thisComputer'), value: connected ? t('next.states.online') : t('next.states.offline'), status: {label: connected ? t('next.states.connected') : t('next.states.offline'), tone: connected ? 'healthy' as const : 'warning' as const}},
  ];
  const copyText = [
    [t('next.states.address'), address ?? ''],
    [t('next.states.thisComputer'), connected ? t('next.states.online') : t('next.states.offline')],
    ...(facts.code ? [[t('next.states.code', {count: 1}), facts.code]] : []),
  ].filter(([, value]) => value).map(([label, value]) => `${label}: ${value}`).join('\n');
  const copy = () => {
    const write = navigator.clipboard?.writeText(copyText);
    void Promise.resolve(write).then(() => setCopied(true), () => setCopied(false));
  };
  const firstFailure = t('next.states.triesSince', {count: facts.attempt, time: defaultI18n.time(facts.firstFailureAt)});
  return <>
    {offline && offline.items.length > 0 && <section className={styles.offlineDownloads} data-offline-downloads aria-label={t('next.shell.downloads')}>
      <h2>{t('next.shell.downloads')}</h2>
      <ul>{offline.items.map(row => <li key={row.id} data-playing={playing?.phase === 'playing' && playing.itemId === row.itemId || undefined}><b>{row.title}</b>{row.artist ? <span>{row.artist}</span> : null}
        {offlinePort?.play ? <button type="button" onClick={() => { void offlinePort.play?.(row.id).catch(() => {}); }}>{t('next.controls.play')}</button> : null}</li>)}</ul>
      <button type="button" onClick={() => { void offlinePort?.openFolder().catch(() => {}); }}>{t('next.saved.folder')}</button>
    </section>}
    {offlinePort?.attachSurface && <div className={styles.offlinePlayer} data-offline-playback data-offline-playing={playing?.phase === 'playing' || undefined} data-media-surface data-native-video-active="true">
      <video ref={surfaceRef} playsInline preload="none" aria-label={t('next.dock.nowPlaying')} />
    </div>}
    <StateLayout className={styles.page} backdrop={{kind: 'colour', shape: 'side'}} tone="warning" icon="i-srvoff"
      contentStyle={{top: width >= 1024 ? 500 : 250}}
      title={computerOffline ? t('next.states.thisComputerOffline') : facts.serverName ? t('next.states.cantReach', {server: facts.serverName}) : t('next.states.cantReachServer')}
      body={computerOffline ? t('next.states.reconnectsWhenBack') : t('next.states.cantReachBody')}
      retry={facts.retrying ? {progress: null, label: t('next.states.retrying')} : {progress: retryProgress(facts.startedAt, facts.nextRetryAt, now), label: t('next.states.retryIn', {seconds: retrySeconds(facts.nextRetryAt, now)}), detail: firstFailure}}
      primary={{id: 'retry', label: t('next.states.tryNow'), icon: 'i-retry', onPress: onRetry, loading: facts.retrying}}
      secondary={{id: 'switch', label: t('next.states.switchServer'), icon: 'i-server', onPress: onSwitchServer}}
      details={{open: detailsOpen, onOpenChange: setDetailsOpen, content: facts.owner
        ? <DetailsPanel audience="owner" facts={factsRows} owner={{
          ownerMark: <ServerMark size={24} />,
          ownerTag: <OwnerTag />,
          paragraphs: [t('next.states.cantReachOwnerBody', {time: defaultI18n.time(facts.firstFailureAt)})],
          codes: facts.code ? [facts.code] : [],
          actions: [{id: 'copy', label: t('next.states.copyDetails'), onPress: copy}],
        }} />
        : <DetailsPanel audience="viewer" facts={factsRows} />}} />
    <div className={styles.toastWrap}>
      <Toast open={copied} onOpenChange={open => { if (!open) setCopied(false); }} icon="i-check" tone="success"
        message={<><b>{t('next.overlays.detailsCopied')}</b> {t('next.overlays.detailsSafe')}</>} />
    </div>
  </>;
}
