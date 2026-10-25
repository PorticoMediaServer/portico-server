import {useEffect, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../Icon';
import styles from './playback-waiting.module.css';
/** A real media-recovery observation, delayed three seconds; owns no retry or transport. */
export function PlaybackWaiting({waiting}: {waiting: boolean}) {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    setShown(false);
    if (!waiting) return;
    const timer = setTimeout(() => setShown(true), 3000);
    return () => clearTimeout(timer);
  }, [waiting]);
  if (!waiting || !shown) return null;
  return <div className={styles.waiting} role="status"><Icon name="i-refresh" size={32}/><p>{defaultI18n.t('playback.message.network_stall')}</p></div>;
}
