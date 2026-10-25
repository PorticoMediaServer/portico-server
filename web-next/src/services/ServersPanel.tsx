import {useEffect, useMemo, useRef, useState} from 'react';
import {encodedRoutePath, routeBody} from '../../../packages/contracts/src/hosted/routes.ts';
import {ServicesBadge, ServicesEmpty, ServicesGroup, ServicesWorkspaceRow} from '../kit/accounts/ServicesWorkspace';
import {Button} from '../kit/controls';
import {Dialog} from '../kit/overlays';
import {HostedError, ServicesQuiet} from '../kit/accounts/ServicesAuth';
import {servicesI18n} from './i18n';
import type {WorkspaceApi, WorkspaceServers} from './workspace-api';
import {servicesServerDirectoryCopy} from './server-directory';
import styles from './server-directory.module.css';

export function ServersPanel({api}: {api: WorkspaceApi}) {
  const i18n = useMemo(servicesI18n, []), {t} = i18n;
  const [servers, setServers] = useState<WorkspaceServers>(), [error, setError] = useState<unknown>();
  const [leaving, setLeaving] = useState<WorkspaceServers['items'][number]>(), [busy, setBusy] = useState(false);
  const [retrying, setRetrying] = useState(''), [seconds, setSeconds] = useState(0);
  const operation = useRef<AbortController | null>(null);
  const load = async (signal: AbortSignal) => {
    const value = await api.servers(signal);
    if (!signal.aborted) { setServers(value); setError(undefined); }
  };
  useEffect(() => {
    const controller = new AbortController(); operation.current = controller;
    void api.servers(controller.signal).then(value => { if (!controller.signal.aborted) setServers(value); }, value => { if (!controller.signal.aborted) setError(value); });
    return () => controller.abort();
  }, [api]);
  // While a server is offline, Portico checks again by itself; the drawn line
  // counts the real seconds down and every row keeps its own Try again.
  const offline = Boolean(servers?.items.some(server => server.status !== 'active'));
  useEffect(() => {
    if (!offline) { setSeconds(0); return; }
    setSeconds(30);
    const timer = setInterval(() => {
      setSeconds(current => {
        if (current > 1) return current - 1;
        const signal = operation.current?.signal;
        if (signal && !signal.aborted && !busy) void api.servers(signal).then(value => { if (!signal.aborted) setServers(value); }, () => undefined);
        return 30;
      });
    }, 1000);
    return () => clearInterval(timer);
  }, [offline, api, busy]);
  const retry = async (server: WorkspaceServers['items'][number]) => {
    const signal = operation.current?.signal;
    if (retrying || !signal || signal.aborted) return;
    setRetrying(server.id); setError(undefined);
    try { await load(signal); }
    catch (value) { if (!signal.aborted) setError(value); }
    finally { if (!signal.aborted) setRetrying(''); }
  };
  const leave = async () => {
    if (!leaving || leaving.ownedByMe || busy || !operation.current) return;
    const signal = operation.current.signal; setBusy(true); setError(undefined);
    try {
      await api.request(encodedRoutePath('server_leave', {id: encodeURIComponent(leaving.id)}), 'POST', routeBody('server_leave', {}), signal);
      const value = await api.servers(signal);
      if (!signal.aborted) { setServers(value); setLeaving(undefined); }
    } catch (value) { if (!signal.aborted) setError(value); }
    finally { if (!signal.aborted) setBusy(false); }
  };
  return <>{error ? <HostedError error={error} /> : null}
    {offline && seconds > 0 ? <p className={styles.recheck}>{t('web.hostedAccount.recheckingIn', {time: seconds})}</p> : null}
    {servers && <ServicesGroup>{servers.items.length ? servers.items.map(server => {
      const copy = servicesServerDirectoryCopy(server, i18n);
      return <ServicesWorkspaceRow key={server.id} icon="i-server" title={server.name}
      help={copy.presence && <span className={copy.online ? styles.online : undefined}>{copy.presence}</span>}
      badge={copy.badge ? <ServicesBadge tone={copy.badge.tone}>{copy.badge.label}</ServicesBadge> : undefined}>
      {!copy.online ? <Button size="small" variant="glass" disabled={busy || Boolean(retrying)} loading={retrying === server.id} onClick={() => void retry(server)}>{t('next.states.tryAgain')}</Button> : null}
      {server.ownedByMe ? <ServicesQuiet>{t('web.hostedAccount.leaveOwner')}</ServicesQuiet> : <Button size="small" disabled={busy} onClick={() => setLeaving(server)}>{t('web.hostedAccount.leaveServer')}</Button>}
    </ServicesWorkspaceRow>;}) : <ServicesEmpty title={t('services.workspace.emptyServers')} description={t('services.workspace.emptyServersBody')}
      actions={<><Button size="small" variant="primary" onClick={() => location.assign('/#join')}>{t('services.start.join')}</Button><Button size="small" onClick={() => location.assign('/#setup')}>{t('services.start.setup')}</Button></>} />}</ServicesGroup>}
    <Dialog open={Boolean(leaving)} onOpenChange={open => { if (!open && !busy) setLeaving(undefined); }} title={t('web.hostedAccount.leaveTitle', {server: leaving?.name ?? ''})}
      description={t('web.hostedAccount.leaveBody', {server: leaving?.name ?? ''})} footer={<><Button size="small" disabled={busy} onClick={() => setLeaving(undefined)}>{t('action.cancel')}</Button><Button size="small" loading={busy} onClick={() => void leave()}>{t('web.hostedAccount.leaveServer')}</Button></>}><></></Dialog>
  </>;
}
