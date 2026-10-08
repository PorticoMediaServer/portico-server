import React, {Suspense, lazy, useEffect, useState, useSyncExternalStore} from 'react';
import type {HostedServer} from '@core/index.ts';
import type {KnownDirectServer} from '@core/known-servers.ts';
import {knownServers} from '../app/known-servers';
import {friendlyHost} from '@core/presentation/index.ts';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {Badge, Button, Dialog, ListRow, Loading, Notice, Text} from '../ui';

const bundled = !import.meta.env.VITE_HOSTED_WEB && location.hostname !== 'web.getportico.tv';
const HostedChooser = lazy(() => import('../screens/auth/HostedChooser').then(m => ({default: m.HostedChooser})));


/**
 * "Switch server" for a Portico Account: the account's servers with their online state, and
 * this browser's direct sign-in as its own entry. Choosing a server keeps the account session
 * and asks for a profile on that server (HostedChooser, which enters a sole unprotected profile
 * by itself). "Sign in directly to another server" leaves the account signed in.
 */
export function ServerSwitcher({onClose}: {onClose: () => void}) {
  const session = useSession();
  const i18n = useI18n();
  const [target, setTarget] = useState<string>();
  const route = useSyncExternalStore(session.api.subscribeRoute, session.api.getRouteSnapshot);
  useEffect(() => { session.refreshServers(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const currentId = session.session?.viewer.serverId;
  const signedInDirectly = session.session?.viewer.authority === 'local';
  const currentReachable = !session.serverProblem && !(route && (route.phase === 'offline' || route.phase === 'identity_mismatch'));
  const [direct, setDirect] = useState<readonly KnownDirectServer[]>([]);
  useEffect(() => { void knownServers.list().then(setDirect, () => {}); }, []);
  const state = (server: HostedServer): {label: string; tone: 'healthy' | 'neutral' | 'warning'} | undefined => {
    if (server.id === currentId && !signedInDirectly) return currentReachable ? {label: i18n.t('web.servers.online'), tone: 'healthy'} : {label: i18n.t('web.servers.unreachable'), tone: 'warning'};
    if (server.presence?.online === true) return {label: i18n.t('web.servers.online'), tone: 'healthy'};
    if (server.presence?.online === false) return {label: i18n.t('web.servers.offline'), tone: 'neutral'};
    return undefined;
  };
  if (target) {
    return (
      <Dialog open onOpenChange={o => !o && onClose()} title={i18n.t('web.servers.chooseProfile')} width={560}>
        <Suspense fallback={null}><HostedChooser preferredServerId={target} onDone={onClose} onBack={() => setTarget(undefined)} /></Suspense>
      </Dialog>
    );
  }
  const servers = session.servers;
  // Served by a server (bundled), the sign-in page is that server's; another server's address is
  // its own web app. On the hosted web app, sign in to it here.
  const openDirect = (address: string) => {
    onClose();
    if (bundled && address !== location.origin) { location.assign(address); return; }
    session.disconnectServer();
    session.direct.setOrigin(address);
  };
  const others = direct.filter(d => !(signedInDirectly && d.serverId === currentId));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={i18n.t('web.servers.switchServer')} width={520}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {session.serversStatus === 'loading' && !servers.length ? <Loading label={i18n.t('web.servers.loading')} /> : null}
        {session.serversError ? <Notice tone="error" compact action={{label: i18n.t('action.tryAgain'), onClick: session.refreshServers}}>{session.serversError}</Notice> : null}
        {servers.length ? (
          <div role="list" aria-label={i18n.t('web.servers.accountServers')}>
            {servers.map(server => {
              const s = state(server);
              const current = server.id === currentId && !signedInDirectly;
              return (
                <ListRow
                  key={server.id}
                  icon="server"
                  title={server.name}
                  subtitle={current ? i18n.t('web.servers.current') : undefined}
                  meta={s ? <Badge tone={s.tone} dot>{s.label}</Badge> : undefined}
                  trailingIcon="forward"
                  onClick={() => setTarget(server.id)}
                />
              );
            })}
          </div>
        ) : null}
        {(signedInDirectly && session.session) || others.length ? (
          <div role="list" aria-label={i18n.t('web.servers.directHeading')}>
            <Text as="p" variant="label" tone="tertiary">{i18n.t('web.servers.directHeading')}</Text>
            {signedInDirectly && session.session ? <ListRow icon="server" title={session.system?.name ?? friendlyHost(session.serverUrl) ?? i18n.t('web.settings.serverFallback')} subtitle={i18n.t('web.servers.current')} meta={<Badge tone={currentReachable ? 'healthy' : 'warning'} dot>{currentReachable ? i18n.t('web.servers.online') : i18n.t('web.servers.unreachable')}</Badge>} /> : null}
            {/* A remembered direct server signs in again at its address (this browser keeps one active sign-in). */}
            {others.map(d => <ListRow key={d.serverId} icon="server" title={d.name} subtitle={[friendlyHost(d.address), d.lastProfileName].filter(Boolean).join(' · ')} trailingIcon="forward" onClick={() => openDirect(d.address)} />)}
          </div>
        ) : null}
        <div><Button variant="link" icon="plus" label={i18n.t('web.servers.signInDirectly')} onClick={() => { onClose(); session.disconnectServer(); }} /></div>
      </div>
    </Dialog>
  );
}
