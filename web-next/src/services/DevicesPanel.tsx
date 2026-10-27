import {HostedError, ServicesActions, ServicesQuiet, ServicesNotice} from '../kit/accounts/ServicesAuth';
import {useCallback, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode} from 'react';
import {ApiError} from '@core/index.ts';
import type {AccountSessionReviewService} from '@core/account-sessions.ts';
import {ServicesBadge, ServicesDangerAction, ServicesDeviceFacts, ServicesGroup, ServicesWorkspaceRow} from '../kit/accounts/ServicesWorkspace';
import {Button} from '../kit/controls';
import {Dialog} from '../kit/overlays';
import type {IconName} from '../kit/Icon';
import {servicesI18n} from './i18n';
import type {WorkspaceApi, WorkspaceDevices} from './workspace-api';
import {servicesRelativeTime, servicesSignInDate} from './time';
import styles from './services.module.css';
import {useAccountResource} from './use-account-resource';

/** A family is revoked only from a current canonical review; logout-all would end this browser. */
export async function signOutOtherDevices(service: AccountSessionReviewService) {
  const pages = new Set<string>();
  for (;;) {
    const state = service.getSnapshot();
    if (state.phase !== 'ready' || state.mutationError) return false;
    const other = state.items.find(item => item.status === 'active' && !item.current);
    if (other) { await service.revoke(other.id); pages.clear(); continue; }
    if (!state.nextCursor) return true;
    if (pages.has(state.nextCursor)) throw new ApiError(502, 'invalid_response', 'invalid_response', false);
    pages.add(state.nextCursor); await service.next();
  }
}

type Props = {api: WorkspaceApi; frame: (children: ReactNode, actions: ReactNode) => ReactNode};
export function DevicesPanel({api, frame}: Props) {
  const create = useCallback(() => api.sessions(), [api]), bound = useAccountResource(create);
  return bound ? <BoundDevicesPanel key={bound.id} api={api} frame={frame} service={bound.value}/>
    : frame(<ServicesQuiet>{servicesI18n().t('status.loading')}</ServicesQuiet>, null);
}
function BoundDevicesPanel({api, frame, service}: Props & {service: AccountSessionReviewService}) {
  const i18n = useMemo(servicesI18n, []), {t} = i18n;
  const state = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const [devices, setDevices] = useState<WorkspaceDevices>(), [error, setError] = useState<unknown>();
  const [ending, setEnding] = useState<string>(), [all, setAll] = useState(false), [allBusy, setAllBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void service.refresh();
    void api.devices(controller.signal).then(value => { if (!controller.signal.aborted) setDevices(value); }, value => { if (!controller.signal.aborted) setError(value); });
    return () => { controller.abort(); };
  }, [api, service]);
  const active = state.items.filter(item => item.status === 'active');
  const metadata = new Map(devices?.items.map(item => [item.familyId, item]));
  const name = (id: string) => metadata.get(id)?.name || t(state.items.find(row => row.id === id)?.mode === 'browser' ? 'web.hostedAccount.aBrowser' : 'web.hostedAccount.anApp');
  const busy = state.pending !== null || allBusy;
  // While an ambiguous revoke retains its exact family, ordinary revokes stay closed
  // and only the authored retry can settle it. Read-only paging stays available.
  const locked = busy || Boolean(state.retryId);
  const retryRevoke = async () => {
    const id = state.retryId;
    if (!id) return;
    // Only this opening's retained family may be retried; a newer ambiguity owns its own intent.
    if (service.getSnapshot().retryId !== id) return;
    try { await service.retryRevoke(); } catch (value) { setError(value); }
  };
  const revoke = (id: string) => { try { void service.revoke(id).catch(setError); } catch (value) { setError(value); } };
  const endOthers = async () => {
    setAll(false); setAllBusy(true); setError(undefined);
    try { await signOutOtherDevices(service); } catch (value) { setError(value); }
    finally { setAllBusy(false); }
  };
  const actions = <Button size="small" disabled={locked || !active.some(item => !item.current) && !state.nextCursor} onClick={() => setAll(true)}>{t('web.hostedAccount.signOutOthers')}</Button>;
  return frame(<>
    {error ? <HostedError error={error} /> : null}{state.error ? <HostedError error={state.error} /> : null}{state.mutationError ? <HostedError error={state.mutationError} /> : null}
    {state.retryId && <ServicesActions><Button size="small" disabled={busy} onClick={() => void retryRevoke()}>{t('next.states.tryAgain')}</Button></ServicesActions>}
    {state.notice && <ServicesNotice tone="info">{state.notice}</ServicesNotice>}
    {active.length > 0 && <ServicesGroup>{active.map(item => {
      const device = metadata.get(item.id);
      const icon: IconName | undefined = !device ? undefined : device.platform === 'ios' || device.platform === 'android' ? 'i-phone' : device.platform === 'tvos' || device.platform === 'androidtv' ? 'i-tv' : 'i-laptop';
      const app = !device ? t(item.mode === 'browser' ? 'web.hostedAccount.aBrowser' : 'web.hostedAccount.anApp') : device.platform === 'web' ? t('services.workspace.appWeb') : device.platform === 'tvos' ? t('services.workspace.appAppleTV') : device.platform === 'ios' ? t('services.workspace.appIPhone') : device.platform === 'androidtv' ? t('services.workspace.appAndroidTV') : device.app;
      const signedIn = t('web.hostedAccount.signedInOn', {date: servicesSignInDate(i18n, item.createdAt)});
      const lastUsed = device?.lastSeenAt ? t('web.hostedAccount.lastUsed', {when: servicesRelativeTime(i18n, device.lastSeenAt)}) : '';
      const phoneApp = device?.platform === 'tvos' ? t('services.platform.tvos') : device?.platform === 'androidtv' ? t('services.platform.androidtv') : device?.platform === 'web' ? '' : app;
      const help = <ServicesDeviceFacts desktop={[app, signedIn, lastUsed]} phone={[item.current ? signedIn : phoneApp, lastUsed]} />;
      return <ServicesWorkspaceRow key={item.id} icon={icon} title={name(item.id)} help={help} badge={item.current ? <ServicesBadge tone="accent">{t('web.hostedAccount.thisBrowser')}</ServicesBadge> : undefined}>
        {!item.current && <Button size="small" loading={state.pending === item.id} disabled={locked} onClick={() => setEnding(item.id)}>{t('action.signOut')}</Button>}
      </ServicesWorkspaceRow>;
    })}</ServicesGroup>}
    {(state.hasPrevious || state.nextCursor) && <div className={styles.pager}><Button size="small" disabled={busy || !state.hasPrevious} onClick={() => void service.previous()}>{t('web.hostedAccount.newer')}</Button><Button size="small" disabled={busy || !state.nextCursor} onClick={() => void service.next()}>{t('web.hostedAccount.older')}</Button></div>}
    <Dialog open={Boolean(ending)} onOpenChange={open => { if (!open) setEnding(undefined); }} title={t('web.hostedAccount.signOutDeviceTitle', {device: ending ? name(ending) : ''})} description={t('web.hostedAccount.signOutBody')} showCloseButton={false} stackFooter className={styles.overlaySpacing}
      footer={<><Button size="small" onClick={() => setEnding(undefined)}>{t('action.cancel')}</Button><ServicesDangerAction onClick={() => { const id = ending!; setEnding(undefined); revoke(id); }}>{t('action.signOut')}</ServicesDangerAction></>}><></></Dialog>
    <Dialog open={all} onOpenChange={open => { if (!open) setAll(false); }} title={t('web.hostedAccount.signOutOthersTitle')} description={t('web.hostedAccount.signOutOthersBody')} showCloseButton={false} stackFooter className={styles.overlaySpacing}
      footer={<><Button size="small" onClick={() => setAll(false)}>{t('action.cancel')}</Button><ServicesDangerAction onClick={() => void endOthers()}>{t('web.hostedAccount.signOutOthers')}</ServicesDangerAction></>}><></></Dialog>
  </>, actions);
}
