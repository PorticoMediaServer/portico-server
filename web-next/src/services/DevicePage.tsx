import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {ApiError} from '@core/index.ts';
import {normalizeSetupCode} from '@core/setup-code.ts';
import type {MessageId} from '@i18n';
import {AuthError, AuthLinks} from '../kit/accounts';
import {ServicesAccountLine, ServicesActions, ServicesAuthStage, ServicesCodeChip, ServicesFacts, ServicesHeading, ServicesLead, ServicesNotice, ServicesQuiet, ServicesStateIcon} from '../kit/accounts/ServicesAuth';
import {Button, TextField} from '../kit/controls';
import {ServicesAuthForm} from './AuthForm';
import {accountIdentity, sameAccount, type ServicesAccount} from './session';
import {decideHostedDevice, reviewHostedDevice, type DeviceReview} from './device-approval';
import {servicesI18n} from './i18n';
import styles from './services.module.css';

const platforms: Partial<Record<string, MessageId>> = {ios: 'services.platform.ios', tvos: 'services.platform.tvos', android: 'services.platform.android', androidtv: 'services.platform.androidtv', web: 'services.platform.web', macos: 'services.platform.macos'};
export function DevicePage({account, initialCode = ''}: {account: ServicesAccount; initialCode?: string}) {
  const {t} = useMemo(servicesI18n, []);
  const session = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot).session;
  const [code, setCode] = useState(initialCode), [review, setReview] = useState<DeviceReview>();
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [done, setDone] = useState<'approve' | 'deny'>();
  const operation = useRef<AbortController | null>(null);
  useEffect(() => () => operation.current?.abort(), []);
  useEffect(() => {
    if (review && (!session || !sameAccount(review.identity, accountIdentity(session)))) {
      operation.current?.abort(); operation.current = null; setBusy(false); setReview(undefined); setDone(undefined);
      setError(new ApiError(409, 'account_changed', 'account_changed', false));
    }
  }, [review, session]);
  const run = async (work: (signal: AbortSignal) => Promise<void>) => {
    if (operation.current) return;
    const controller = new AbortController(); operation.current = controller; setBusy(true); setError(undefined);
    const timeout = setTimeout(() => controller.abort(new DOMException('timeout', 'TimeoutError')), 15000);
    try { await work(controller.signal); }
    catch (value) {
      if (!controller.signal.aborted) {
        setError(value);
        if ([400, 404, 410, 422].includes((value as {status?: number})?.status ?? 0)) setReview(undefined);
      } else if (controller.signal.reason?.name === 'TimeoutError') {
        setReview(undefined); setError(new ApiError(408, 'timeout', 'timeout', true));
      }
    }
    finally { clearTimeout(timeout); if (operation.current === controller) operation.current = null; if (!controller.signal.aborted || controller.signal.reason?.name === 'TimeoutError') setBusy(false); }
  };
  const inspect = () => run(async signal => { const value = await reviewHostedDevice(account, code, signal); if (!signal.aborted) setReview(value); });
  const decide = (decision: 'approve' | 'deny') => run(async signal => { if (!review) return; await decideHostedDevice(account, review, decision, signal); if (!signal.aborted) { setDone(decision); setReview(undefined); } });
  const another = () => { setReview(undefined); setDone(undefined); setError(undefined); };
  const errorCode = (error as {code?: string} | undefined)?.code;
  const notFound = [400, 404, 410, 422].includes((error as {status?: number} | undefined)?.status ?? 0);
  let completeCode = false; try { normalizeSetupCode(code); completeCode = true; } catch { /* incomplete field */ }
  if (!session) return <ServicesAuthStage><ServicesAuthForm account={account} onSignedIn={() => {}} embedded heading={t('web.device.title')} lead={t('web.device.lede')}
    before={<>{initialCode && <ServicesCodeChip label={t('services.device.codeFromTV')} code={initialCode} />}<ServicesQuiet>{t('services.device.signedOut')}</ServicesQuiet></>} /></ServicesAuthStage>;
  if (done) return <ServicesAuthStage variant="state"><ServicesStateIcon tone={done === 'approve' ? 'success' : 'warning'} icon={done === 'approve' ? 'i-check' : 'i-close'} /><ServicesHeading>{t(done === 'approve' ? 'services.device.approvedTitle' : 'services.device.deniedTitle')}</ServicesHeading><ServicesLead>{t(done === 'approve' ? 'services.device.approved' : 'services.device.denied')}</ServicesLead><ServicesActions><Button onClick={another}>{t('web.device.anotherCode')}</Button></ServicesActions></ServicesAuthStage>;
  return <ServicesAuthStage variant="reset"><ServicesHeading>{t('web.device.title')}</ServicesHeading><ServicesLead>{t(review ? 'services.device.reviewLead' : 'web.device.lede')}</ServicesLead>
    <ServicesAccountLine>{t('services.device.approving', {name: session.account.displayName || session.account.username})}<button type="button" disabled={busy} onClick={() => void account.service.logout()}>{t('services.device.notYou')}</button></ServicesAccountLine>
    {error && !notFound ? errorCode === 'account_changed' || errorCode === 'timeout' ? <ServicesNotice icon="i-clock">{t(errorCode === 'account_changed' ? 'services.device.accountChanged' : 'services.device.timeout')}</ServicesNotice> : <AuthError error={error} /> : null}
    {review ? <>
      <ServicesFacts items={[{id: 'device', label: t('services.device.device'), value: review.preview.deviceName}, {id: 'platform', label: t('services.device.platform'), value: platforms[review.preview.platform] ? t(platforms[review.preview.platform]!) : review.preview.platform}, {id: 'code', label: t('auth.code'), value: review.preview.userCode}]} />
      <ServicesNotice tone="info">{t('web.device.approveWarn')}</ServicesNotice>
      <ServicesActions><Button variant="primary" loading={busy} onClick={() => void decide('approve')}>{t('web.device.approveDevice')}</Button><Button disabled={busy} onClick={() => void decide('deny')}>{t('services.device.deny')}</Button></ServicesActions><AuthLinks><button type="button" disabled={busy} onClick={another}>{t('web.device.anotherCode')}</button></AuthLinks>
    </> : <><form className={styles.form} onSubmit={event => { event.preventDefault(); void inspect(); }}>
      <TextField variant="auth" label={t('auth.code')} name="code" autoComplete="one-time-code" autoCapitalize="characters" spellCheck={false} placeholder={t('services.device.codePlaceholder')} value={code} maxLength={12} onChange={event => setCode(event.currentTarget.value.toUpperCase())} description={t('services.device.codeHelp')} error={notFound ? t('services.device.notFound') : undefined} disabled={busy} required />
      <ServicesActions><Button variant="primary" type="submit" loading={busy} disabled={!completeCode}>{t('web.device.review')}</Button></ServicesActions>
    </form><ServicesQuiet>{t('services.device.expiry')}</ServicesQuiet></>}
  </ServicesAuthStage>;
}
