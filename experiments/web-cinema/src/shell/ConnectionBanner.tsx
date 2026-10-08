import {useEffect, useState} from 'react';
import {useSession} from '../app/session';
import {Icon} from '../ui/Icon';

/** Shown while the server is unreachable; content stays readable underneath. */
export function ConnectionBanner() {
  const session = useSession();
  const [seconds, setSeconds] = useState(8);
  useEffect(() => {
    if (session.connection === 'online') return;
    const t = setInterval(() => setSeconds(s => (s <= 1 ? 8 : s - 1)), 1000);
    return () => clearInterval(t);
  }, [session.connection]);
  if (session.connection === 'online') return null;
  const offline = session.connection === 'offline';
  return (
    <div className={offline ? 'conn-banner is-offline' : 'conn-banner'} role="status" aria-live="polite">
      <span className="conn-pulse" aria-hidden><Icon name={offline ? 'wifiOff' : 'refresh'} size={16} /></span>
      <span className="conn-text">
        <strong>{offline ? 'You’re offline' : `Reconnecting to ${session.server?.name ?? 'your server'}…`}</strong>
        <span>{offline ? 'Downloads still play. Everything else comes back when you’re online.' : `Trying again in ${seconds}s. What’s on screen stays put.`}</span>
      </span>
      <button type="button" className="btn btn-sm btn-secondary" onClick={() => session.setConnection('online')}><span className="btn-label">{offline ? 'Watch downloads' : 'Try now'}</span></button>
    </div>
  );
}
