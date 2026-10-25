import {HostedError} from '../kit/accounts/ServicesAuth';
import {useEffect, useMemo, useState, useSyncExternalStore, type ReactNode} from 'react';
import type {HostedSession} from '@core/index.ts';
import {ProfileAvatar} from '../kit/shell/AccountSurfaces';
import {ServicesHeader, ServicesWorkspace, type ServicesSection} from '../kit/accounts/ServicesWorkspace';
import {Button} from '../kit/controls';
import {DevicesPanel} from './DevicesPanel';
import {FeedPanel} from './FeedPanel';
import {ServersPanel} from './ServersPanel';
import {SecurityPanel} from './SecurityPanel';
import {ServicesFeed} from './feed';
import {workspaceApi} from './workspace-api';
import {accountIdentity, type ServicesAccount} from './session';
import {servicesI18n} from './i18n';

export function AccountPage({account, session, section, onDeleted}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>; section: ServicesSection; onDeleted?: () => void}) {
  const identity = accountIdentity(session);
  return <BoundAccount key={identity.accountId + '/' + identity.familyId} account={account} session={session} section={section} onDeleted={onDeleted} />;
}
function BoundAccount({account, session, section, onDeleted}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>; section: ServicesSection; onDeleted?: () => void}) {
  const {t} = useMemo(servicesI18n, []);
  const accountId = session.account.id, familyId = session.familyId!;
  const identity = useMemo(() => ({accountId, familyId}), [accountId, familyId]);
  const api = useMemo(() => workspaceApi(account, identity), [account, identity]);
  const feed = useMemo(() => new ServicesFeed(api.notifications), [api]);
  const state = useSyncExternalStore(feed.subscribe, feed.getSnapshot);
  const [error, setError] = useState<unknown>();
  const [contact, setContact] = useState<string>();
  useEffect(() => { void feed.refresh(); return () => feed.dispose(); }, [feed]);
  useEffect(() => {
    const controller = new AbortController();
    void api.security(controller.signal).then(value => { if (!controller.signal.aborted) setContact(value.contactAddress || undefined); }, () => {});
    return () => controller.abort();
  }, [api]);
  const name = session.account.displayName || session.account.username, initial = [...name][0]?.toLocaleUpperCase() ?? '';
  const select = (tab: ServicesSection) => location.assign('/account?tab=' + tab);
  const signOut = () => { void account.service.logoutIfCurrent(identity).catch(setError); };
  const header = <ServicesHeader unread={state.unread > 0} avatar={<ProfileAvatar appearance="session" initial={initial} name={name} onClick={() => select('devices')} />} />;
  const frame = (children: ReactNode, actions?: ReactNode, description?: ReactNode) => <ServicesWorkspace header={header} account={{name, email: contact, initial}} section={section}
    onSection={select} onSignOut={signOut} actions={actions} phoneActions={section === 'devices' ? actions : undefined} unread={state.unread} description={description}>{error ? <HostedError error={error} /> : null}{children}</ServicesWorkspace>;
  if (section === 'devices') return <DevicesPanel api={api} frame={(children, actions) => frame(children, actions, t('services.workspace.devicesBody'))} />;
  if (section === 'notifications') return frame(<FeedPanel feed={feed} onReviewDevices={() => select('devices')} />,
    <Button size="small" icon="i-check" disabled={state.busy || !state.unread} onClick={() => void feed.markRead('all')}>{t('web.account.notif.markAll')}</Button>);
  if (section === 'security') return frame(<SecurityPanel account={account} api={api} identity={identity} username={session.account.username} onDeleted={onDeleted} />,
    undefined, t('services.workspace.securityBody'));
  return frame(<ServersPanel api={api} />, undefined, t('web.hostedAccount.serversHelp'));
}
