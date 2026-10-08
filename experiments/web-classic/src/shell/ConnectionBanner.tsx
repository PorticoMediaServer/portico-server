import {useEffect, useRef, useState} from 'react';
import {useClient, useTopic} from '@/data/DataProvider';
import {Button} from '@/ui/Button';
import {Icon} from '@/ui/Icon';
import {cx} from '@/ui/cx';
import s from './shell-bits.module.css';

/** Reconnecting, offline and back-online banners. Content stays visible underneath. */
export function ConnectionBanner() {
  const client = useClient();
  useTopic(['session']);
  const state = client.session.connectivity();
  const [back, setBack] = useState(false);
  const prev = useRef(state);
  const [countdown, setCountdown] = useState(8);
  useEffect(() => {
    if (prev.current !== 'online' && state === 'online') { setBack(true); const t = setTimeout(() => setBack(false), 3500); prev.current = state; return () => clearTimeout(t); }
    prev.current = state;
  }, [state]);
  useEffect(() => {
    if (state !== 'reconnecting') return;
    setCountdown(8);
    const t = setInterval(() => setCountdown(c => (c <= 1 ? 8 : c - 1)), 1000);
    return () => clearInterval(t);
  }, [state]);
  if (state === 'reconnecting') return (
    <div className={cx(s.banner, s.reconnecting)} role="status">
      <span className={s.spin} aria-hidden />
      <span className={s.bannerText}>Reconnecting to the server… <span>Trying again in {countdown} s. What you’re watching keeps playing from what’s loaded.</span></span>
      <Button size="sm" variant="ghost" label="Try now" onClick={() => client.session.setConnectivity('online')} />
    </div>
  );
  if (state === 'offline') return (
    <div className={cx(s.banner, s.offline)} role="alert">
      <Icon name="offline" size={18} />
      <span className={s.bannerText}>You’re offline. <span>Downloads still play. Everything else comes back when you reconnect.</span></span>
      <Button size="sm" variant="ghost" label="Downloads" to="/downloads" />
    </div>
  );
  if (back) return <div className={cx(s.banner, s.online)} role="status"><Icon name="check" size={18} /><span className={s.bannerText}>Back online.</span></div>;
  return null;
}
