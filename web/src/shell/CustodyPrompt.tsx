import {useEffect, useState} from 'react';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {errorText} from '../app/errors';
import {Notice} from '../ui';
import s from './Shell.module.css';

/**
 * INT M6: ownership of this server moved to a Portico Account owner, but Hosted's custody of the
 * server's registration isn't with them yet (`custodyPending`). The owner accepts here; it's their
 * consent, so nothing happens without the tap.
 */
export function CustodyPrompt() {
  const {t} = useI18n();
  const session = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const server = session.system?.name ?? t('web.servers.fallbackName');
  useEffect(() => { if (!done) return; const id = setTimeout(() => setDone(false), 8000); return () => clearTimeout(id); }, [done]);
  if (done) return <div className={s.connectionNotice} role="status"><Notice tone="success" compact>{t('web.custody.done', {server})}</Notice></div>;
  if (!session.owner || !session.local?.custodyPending) return null;
  const accept = () => {
    setBusy(true);
    setError('');
    session.portico.acceptCustody().then(() => setDone(true), e => setError(errorText(e, 'account', 'action'))).finally(() => setBusy(false));
  };
  return (
    <div className={s.connectionNotice} role="status">
      <Notice tone="info" title={t('web.custody.title')} action={{label: t('web.custody.accept'), onClick: accept, loading: busy}}>
        {error || t('web.custody.body', {server})}
      </Notice>
    </div>
  );
}
