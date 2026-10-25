import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {playbackFailure} from '../../../../web/src/player/failure';
import {Button} from '../controls';
import styles from './playback-failure.module.css';
const t = defaultI18n.t;
/** Safe existing failure vocabulary; no raw engine diagnostics or transport ownership. */
export function PlaybackFailure({error, onRetry, onVersions, onReport, onClose, onError}: Readonly<{
  error?: string; onRetry?: () => Promise<void>; onVersions?: () => void; onReport?: () => void; onClose: () => void; onError: (error: unknown) => void;
}>) {
  const failure = playbackFailure(error), [busy, setBusy] = useState(false);
  const running = useRef(false), alive = useRef(true);
  useEffect(() => {alive.current = true;return () => {alive.current = false;};}, []);
  const retry = async () => {if (running.current || !onRetry) return;running.current = true;setBusy(true);try {await onRetry();} catch (error) {if (alive.current) onError(error);} finally {running.current = false;if (alive.current) setBusy(false);}};
  return <section className={styles.failure} role="alert">
    <h2>{t(failure.title)}</h2><p>{failure.body ? t(failure.body) : failure.text}</p>
    <div>{!failure.permanent && onRetry && <Button variant="primary" disabled={busy} loading={busy} onClick={() => {void retry();}}>{t('next.states.tryAgain')}</Button>}
      {onVersions && <Button variant="glass" onClick={onVersions}>{t('browse.field.versions')}</Button>}
      {onReport && <Button variant="glass" onClick={onReport}>{t('web.player.error.report')}</Button>}
      <Button variant="glass" onClick={onClose}>{t('action.close')}</Button></div>
  </section>;
}
