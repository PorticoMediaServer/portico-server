import {useEffect, useRef, useState} from 'react';
import {needsOfflineServerDialog, presenceLabel, sortServersByPresence, type HostedServer} from '@core/index.ts';
import {hostedGate} from '@core/hosted-gate.ts';
import {backoffDelay, foregroundRetry} from '../../app/foreground-retry';
import {useSession, viewer, type PorticoOutcome} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, ListRow, Loading, Notice, StateView} from '../../ui';
import {DirectProfiles} from './DirectProfiles';
import s from './Auth.module.css';

/**
 * X-13: which failure view a failed connect gets. A stale offline server gets
 * the specific "can't reach" dialog; anything else keeps the generic waiting
 * notice (a server seen within the hour keeps the keep-trying copy). Pure, so
 * the chooser's branching is unit-testable without rendering.
 */
export function chooserFailureView(server: HostedServer, outcome: PorticoOutcome | undefined, nowMs: number = Date.now()): 'offline-dialog' | 'waiting' | 'none' {
  if (!server || outcome !== 'unreachable') return 'none';
  return needsOfflineServerDialog(server.presence, true, nowMs) ? 'offline-dialog' : 'waiting';
}
/** Which server opens without asking: the one a switcher chose; else, with none open, the only
 * server, or the last one used here (in the shell). Undefined means show the list. */
export function autoServer<T extends {id: string}>(servers: readonly T[], o: {current?: string; preferred?: string; last?: string}): T | undefined {
  if (o.preferred && o.preferred !== o.current) return servers.find(x => x.id === o.preferred);
  if (o.current) return undefined;
  if (servers.length === 1) return servers[0];
  return o.last ? servers.find(x => x.id === o.last) : undefined;
}

/**
 * A Portico Account's servers (Spec — Hosted at Scale). Choosing one signs in to it with the
 * account (`session.portico.connect`: a Hosted identity assertion the server checks itself), then
 * the server's own profile chooser (the same one as a direct sign-in).
 *
 * `inShell` (Justin's rule): the shell's content area before any server is open. One server opens
 * by itself, several open the last one used here, otherwise the list shows with Refresh Servers;
 * a failure says exactly what happened, centred, with the action that fits. Also used in Settings
 * and the server switcher dialogs.
 */
export function HostedChooser({onDone, onBack, preferredServerId, title, inShell}: {onDone?: () => void; onBack?: () => void; preferredServerId?: string; title?: string; inShell?: boolean}) {
  const {t, date} = useI18n();
  const session = useSession();
  const [target, setTarget] = useState<HostedServer>();
  const [outcome, setOutcome] = useState<PorticoOutcome>();
  const [opened, setOpened] = useState(false);
  const current = session.session?.viewer.serverId;
  const servers = session.servers;
  // X-13: online servers first, then the rest by last seen (unknown last).
  const ordered = sortServersByPresence(servers);
  // X-13: an offline server is dimmed with when it was last seen; unknown presence renders normally.
  const presenceSubtitle = (server: HostedServer): string | undefined => {
    if (server.id === current) return t('web.chooser.current');
    const label = presenceLabel(server.presence);
    if (label.kind === 'lastSeen') return t('web.chooser.lastSeen', {date: date(label.lastSeenAt, 'dayMonth')});
    if (label.kind === 'neverSeen') return t('web.chooser.neverSeen');
    return undefined;
  };
  const offlineBody = (server: HostedServer): string => {
    const label = presenceLabel(server.presence);
    return label.kind === 'lastSeen' ? t('web.chooser.unreachableBody', {date: date(label.lastSeenAt, 'dayMonth')}) : t('web.chooser.unreachableBodyNever');
  };
  // The existing direct sign-in with this server's address prefilled (ServerSwitcher.openDirect):
  // on hosted web the account session stays and the address is prefilled; a bundled server hands
  // off to the other server's own web app. `location` is read lazily so this stays Node-safe.
  const goDirect = (server: HostedServer) => {
    const address = server.baseUrl;
    if (typeof location !== 'undefined') {
      const bundled = !import.meta.env.VITE_HOSTED_WEB && location.hostname !== 'web.getportico.tv';
      if (bundled && address && address !== location.origin) { location.assign(address); return; }
    }
    setOutcome(undefined);
    setTarget(undefined);
    session.clearError();
    session.disconnectServer();
    session.direct.setOrigin(address);
  };
  const [refreshing, setRefreshing] = useState(false);
  // After a refusal, a refreshed list starts over: the server may be back (invited again).
  const auto = useRef(false);
  const refresh = () => {
    setRefreshing(true);
    void session.portico.refreshServers().catch(() => {}).finally(() => {
      setRefreshing(false);
      if (outcome === 'refused') { setOutcome(undefined); setTarget(undefined); session.clearError(); auto.current = false; }
    });
  };
  const connect = (server: HostedServer) => {
    setTarget(server);
    setOutcome(undefined);
    setOpened(true);
    const before = viewer.getSnapshot().session?.accessToken;
    void session.portico.connect(server).then(result => {
      setOutcome(result);
      // Entered straight away (one profile, or the profile this browser remembers): done. Otherwise
      // the server's profile chooser shows here.
      const now = viewer.getSnapshot().session;
      if (result === 'done' && now && now.accessToken !== before && now.viewer.serverId === server.id) onDone?.();
    });
  };
  // One server, the one the switcher chose, or (in the shell) the last one used: open it once.
  useEffect(() => {
    if (auto.current || session.serversStatus !== 'ready' || session.busy) return;
    const chosen = autoServer(servers, {current, preferred: preferredServerId, last: inShell ? session.portico.lastServerId : undefined});
    if (!chosen) return;
    auto.current = true;
    connect(chosen);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.serversStatus, servers, preferredServerId, current, target]);
  // While a chosen server can't be reached, try it again with backoff (20 s doubling to 5 min,
  // jitter), only in a visible tab, never before the Portico Account circuit allows (Batch 43).
  const retries = useRef(0);
  const waiting = outcome === 'unreachable' || outcome === 'account-unavailable';
  useEffect(() => { if (outcome && !waiting) retries.current = 0; }, [outcome, waiting]);
  const hostedUrl = session.hostedUrl;
  useEffect(() => {
    if (!waiting || !target) return;
    const gate = hostedGate(hostedUrl);
    const retry = foregroundRetry(() => { retries.current++; connect(target); }, {hostedWait: () => gate.blockedFor()});
    retry.arm(backoffDelay(retries.current, 20_000, 300_000));
    return () => retry.stop();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [outcome, waiting, target, hostedUrl]);

  // The server answered with its profiles: its own chooser, here.
  if (opened && session.direct.pending) return <DirectProfiles onDone={() => { session.direct.cancelPending(); onDone?.(); }} />;
  const connecting = !!target && session.busy && !outcome;
  const list = (
    <div className={s.list}>
      {ordered.map(server => (
        <ListRow key={server.id} icon="server" title={server.name} subtitle={presenceSubtitle(server)} trailingIcon="forward" className={server.presence?.online === false && server.id !== current ? s.dimmed : undefined} onClick={session.busy ? undefined : () => { if (server.id === current) { setOpened(true); session.direct.openProfileChooser(); } else connect(server); }} />
      ))}
    </div>
  );
  if (inShell) {
    // Centred, one thing at a time: opening a server, why it didn't open, or the list.
    if (connecting && target) return <StateView icon="server" title={t('web.chooser.connecting', {server: target.name})}><Loading label={t('web.chooser.connecting', {server: target.name})} /></StateView>;
    // X-13: a stale offline server names the problem and offers direct sign-in, never the
    // keep-trying promise (that copy is only for a server seen within the hour). No removal
    // here: the Hosted endpoint to remove a server doesn't exist yet.
    if (outcome === 'unreachable' && target && chooserFailureView(target, outcome) === 'offline-dialog') return <StateView icon="warning" title={t('web.chooser.unreachableTitle', {server: target.name})} body={offlineBody(target)} action={{label: t('web.chooser.tryAgain'), onClick: () => connect(target)}} secondaryAction={servers.length > 1 ? {label: t('web.chooser.otherServer'), onClick: () => { setOutcome(undefined); setTarget(undefined); }} : undefined}><div style={{display: 'flex', justifyContent: 'center'}}><Button variant="secondary" label={t('web.chooser.signInDirectly')} onClick={() => goDirect(target)} /></div></StateView>;
    if (outcome === 'unreachable' && target) return <StateView icon="warning" title={t('web.chooser.waitingTitle', {server: target.name})} body={t('web.chooser.waitingBody')} action={{label: t('web.chooser.checkNow'), onClick: () => connect(target)}} secondaryAction={servers.length > 1 ? {label: t('web.chooser.otherServer'), onClick: () => { setOutcome(undefined); setTarget(undefined); }} : undefined} />;
    // INT gate 3: the Portico Account service is down or busy, not the server: said so, retried the same way.
    if (outcome === 'account-unavailable' && target) return <StateView icon="warning" title={t('web.chooser.accountWaitingTitle')} body={t('web.chooser.accountWaitingBody', {server: target.name})} action={{label: t('web.chooser.checkNow'), onClick: () => connect(target)}} />;
    if (outcome === 'refused' && target) return <StateView icon="warning" title={session.error || t('web.portico.noAccess', {server: target.name})} action={{label: t('web.servers.refresh'), onClick: refresh, loading: refreshing}} secondaryAction={servers.length ? {label: t('web.chooser.otherServer'), onClick: () => { setOutcome(undefined); setTarget(undefined); session.clearError(); }} : undefined} />;
    if (outcome === 'failed' && target && session.error) return <StateView icon="warning" title={target.name} body={session.error} action={{label: t('web.chooser.tryAgain'), onClick: () => connect(target)}} secondaryAction={servers.length > 1 ? {label: t('web.chooser.otherServer'), onClick: () => { setOutcome(undefined); setTarget(undefined); session.clearError(); }} : undefined} />;
    if (session.serversStatus === 'loading' || (session.serversStatus === 'idle' && !servers.length)) return <Loading label={t('web.chooser.loading')} />;
    if (session.serversStatus === 'error') return <StateView icon="warning" title={t('web.chooser.loadFailed')} body={session.serversError} action={{label: t('web.chooser.tryAgain'), onClick: session.refreshServers}} />;
    if (!servers.length) return <StateView icon="server" title={t('web.chooser.noneTitle')} body={t('web.chooser.noneBody')} action={{label: t('web.servers.refresh'), onClick: refresh, loading: refreshing}} />;
    return (
      <div className={s.stack}>
        <h2 className={s.title} style={{textAlign: 'center', fontSize: 20}}>{t('web.chooser.lede')}</h2>
        {list}
        <div style={{display: 'flex', justifyContent: 'center'}}><Button variant="ghost" size="sm" icon="refresh" label={t('web.servers.refresh')} loading={refreshing} onClick={refresh} /></div>
      </div>
    );
  }
  return (
    <div className={s.stack}>
      {title ? <h2 className={s.title} style={{textAlign: 'center', fontSize: 20}}>{title}</h2> : null}
      <p className={s.lede} style={{textAlign: 'center'}}>{t('web.chooser.lede')}</p>
      {session.serversStatus === 'loading' ? <Loading label={t('web.chooser.loading')} /> : null}
      {connecting ? <Loading label={t('web.chooser.connecting', {server: target!.name})} /> : null}
      {list}
      {session.serversStatus === 'ready' && !servers.length ? <Notice tone="neutral" title={t('web.chooser.noneTitle')} action={{label: t('web.servers.refresh'), onClick: refresh, loading: refreshing}}>{t('web.chooser.noneBody')}</Notice> : null}
      {session.serversStatus === 'error' ? <Notice tone="error" action={{label: t('web.chooser.tryAgain'), onClick: session.refreshServers}}>{session.serversError}</Notice> : null}
      {outcome === 'unreachable' && target ? (
        chooserFailureView(target, outcome) === 'offline-dialog' ? (
          // X-13: a stale offline server gets the specific dialog, not the generic waiting notice.
          <Notice tone="warning" title={t('web.chooser.unreachableTitle', {server: target.name})} action={{label: t('web.chooser.tryAgain'), onClick: () => connect(target)}} secondaryAction={{label: t('web.chooser.signInDirectly'), onClick: () => goDirect(target)}}>{offlineBody(target)}</Notice>
        ) : (
          // WEB-AUTH-01: an unreachable server is a wait, not an error; it names the server,
          // tries again on its own, and offers a way to pick another one.
          <Notice tone="warning" title={t('web.chooser.waitingTitle', {server: target.name})} action={{label: t('web.chooser.checkNow'), onClick: () => connect(target)}}>{t('web.chooser.waitingBody')}</Notice>
        )
      ) : outcome === 'account-unavailable' && target ? (
        <Notice tone="warning" title={t('web.chooser.accountWaitingTitle')} action={{label: t('web.chooser.checkNow'), onClick: () => connect(target)}}>{t('web.chooser.accountWaitingBody', {server: target.name})}</Notice>
      ) : session.error ? <Notice tone="error">{session.error}</Notice> : null}
      {onBack ? <Button variant="ghost" label={t('action.back')} icon="back" onClick={onBack} /> : null}
    </div>
  );
}
