import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {HttpLocalApi} from '@core/index.ts';
import {browserAccount} from '../../bridge/account';
import {useI18n} from '../../app/i18n';
import {errorText} from '../../app/errors';
import {Button, KeyValue, Loading, Notice, Select, Text} from '../../ui';
import {AccountForm} from './AccountForm';
import {AuthFrame} from './AuthFrame';
import s from './Auth.module.css';

/** What `/v1/server-claims/lookup` returns (lane A, `portico-internal/hosted-services/hosted/claimweb`). */
export type PendingClaim = {code: string; serverId: string; name: string; address: string; requestedAt: string; expiresAt: string; status: string};

/** The claim code is 49 characters and travels only in the fragment. */
const CODE = /^[A-Za-z0-9_-]{49}$/;

/**
 * Reads `#code=…&server=…&return=…` once, then removes it from the address
 * bar and history (`replaceState`) so the code isn't left behind in the tab.
 */
export function takeClaimFragment(loc: Pick<Location, 'hash' | 'pathname' | 'search'> = location, replace: (url: string) => void = url => history.replaceState(history.state, '', url)): {code: string; name: string; returnOrigin?: string} | null {
  const params = new URLSearchParams(loc.hash.slice(1));
  const code = params.get('code') ?? '';
  if (!params.has('code')) return null;
  replace(loc.pathname + loc.search);
  if (!CODE.test(code)) return null;
  let returnOrigin: string | undefined;
  try {
    const url = new URL(params.get('return') ?? '');
    if (url.protocol === 'https:' || url.protocol === 'http:') returnOrigin = url.origin;
  } catch { /* no way back; the page still works */ }
  return {code, name: (params.get('server') ?? '').slice(0, 120), returnOrigin};
}

type Api = Pick<HttpLocalApi, 'request'>;
export const claimLookup = (api: Api, code: string, signal?: AbortSignal) => api.request<PendingClaim>('/v1/server-claims/lookup', 'POST', {code}, signal);
export const claimApprove = (api: Api, code: string, ownerProfileId?: string) => api.request<{status: string; code: string}>('/v1/server-claims/approve-code', 'POST', {code, ...(ownerProfileId ? {ownerProfileId} : {})});
export const claimDeny = (api: Api, code: string) => api.request<unknown>('/v1/server-claims/deny', 'POST', {code});

type Phase = 'loading' | 'review' | 'working' | 'done' | 'denied' | 'failed';

/**
 * `/claim` on the Portico Account site: approve connecting a server to this
 * account. The server opened this page with a claim code in the fragment;
 * this page looks the claim up, shows which server and address asked, lets
 * the owner pick the profile that will own it, then approves or denies with
 * this account's own session. Hosted pushes the decision to the server; this
 * page never talks to the server.
 */
export function ClaimApprovalScreen() {
  const i18n = useI18n();
  // Read the fragment once during render; scrub it after mounting (the router listens to history).
  const [request] = useState(() => takeClaimFragment(location, () => {}));
  useEffect(() => { if (location.hash) history.replaceState(history.state, '', location.pathname + location.search); }, []);
  const central = useMemo(browserAccount, []);
  const account = useSyncExternalStore(central.service.subscribe, central.service.getSnapshot);
  const live = account.session;
  const [pending, setPending] = useState<PendingClaim>();
  const [profileId, setProfileId] = useState('');
  const [phase, setPhase] = useState<Phase>('loading');
  const [error, setError] = useState<unknown>();

  const accountApi = async () => {
    const session = await central.service.accessSession();
    return new HttpLocalApi(central.api.origin, session.accessToken);
  };

  useEffect(() => {
    if (live && !profileId) setProfileId(live.profiles[0]?.id ?? '');
  }, [live, profileId]);

  useEffect(() => {
    if (!request || !live) return;
    const controller = new AbortController();
    setPhase('loading');
    accountApi().then(api => claimLookup(api, request.code, controller.signal)).then(p => {
      if (controller.signal.aborted) return;
      setPending(p);
      setPhase(p.status === 'pending' ? 'review' : 'failed');
      if (p.status !== 'pending') setError({code: 'claim_decided'});
    }, e => { if (!controller.signal.aborted) { setError(e); setPhase('failed'); } });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [request, live?.account.id]);

  const name = pending?.name || request?.name || '';
  const serverLabel = name || i18n.t('web.claim.titleGeneric');
  if (!request) {
    return (
      <AuthFrame title={i18n.t('web.claim.titleGeneric')}>
        <Notice tone="info">{i18n.t('web.claim.openFromServer')}</Notice>
        <Button variant="link" label={i18n.t('web.claim.backToPortico')} to="/" />
      </AuthFrame>
    );
  }
  const decide = async (approve: boolean) => {
    setPhase('working');
    setError(undefined);
    try {
      const api = await accountApi();
      if (approve) await claimApprove(api, request.code, (live?.profiles.length ?? 0) > 1 ? profileId : undefined);
      else await claimDeny(api, request.code);
      setPhase(approve ? 'done' : 'denied');
    } catch (e) {
      setError(e);
      setPhase('review');
    }
  };
  const address = pending?.address ? new URL(pending.address).host : request.returnOrigin ? new URL(request.returnOrigin).host : '';
  const failedText = (e: unknown) => {
    const code = (e as {code?: string} | undefined)?.code;
    if (code === 'claim_decided' || code === 'gone' || code === 'claim_stale' || (e as {status?: number})?.status === 410 || (e as {status?: number})?.status === 409) return i18n.t('web.claim.expired');
    if ((e as {status?: number})?.status === 429) return i18n.t('web.claim.errorTooMany');
    if (code === 'invalid_request' || (e as {status?: number})?.status === 400) return i18n.t('web.claim.errorInvalid');
    if ((e as {status?: number})?.status === 404 || code === 'not_found') return i18n.t('web.claim.notFound');
    return errorText(e, 'claim', 'load');
  };
  return (
    <AuthFrame title={name ? i18n.t('web.claim.title', {server: name}) : i18n.t('web.claim.titleGeneric')} lede={i18n.t('web.claim.lede')}>
      <div className={s.stack}>
        {!live ? (
          <>
            <Text variant="caption" tone="secondary" center>{i18n.t('web.claim.signInFirst')}</Text>
            <AccountForm onSignedIn={() => {}} />
          </>
        ) : phase === 'loading' ? (
          <Loading label={i18n.t('web.claim.loading')} />
        ) : phase === 'done' ? (
          <>
            <Notice tone="success" title={i18n.t('web.claim.connectedTitle')}>{i18n.t('web.claim.connected', {server: serverLabel})}</Notice>
            {request.returnOrigin ? <Button variant="primary" block label={i18n.t('web.claim.returnTo', {server: serverLabel})} onClick={() => location.assign(request.returnOrigin!)} /> : null}
          </>
        ) : phase === 'denied' ? (
          <Notice tone="info">{i18n.t('web.claim.denied', {server: serverLabel})}</Notice>
        ) : phase === 'failed' ? (
          <Notice tone="warning">{failedText(error)}</Notice>
        ) : (
          <>
            <KeyValue rows={[[i18n.t('web.claim.server'), serverLabel], [i18n.t('web.claim.address'), address], [i18n.t('web.claim.account'), live.account.displayName || live.account.username], ...(pending?.requestedAt ? [[i18n.t('web.claim.requested'), i18n.relativeTime(pending.requestedAt)] as [string, string]] : [])].filter(([, v]) => v) as [string, string][]} />
            {live.profiles.length > 1 ? <Select label={i18n.t('web.claim.ownerProfile')} value={profileId} onChange={e => setProfileId(e.target.value)} options={live.profiles.map(p => ({value: p.id, label: p.name}))} help={i18n.t('web.claim.ownerProfileHelp')} /> : null}
            <Text variant="caption" tone="secondary">{i18n.t('web.claim.checkAddress')}</Text>
            {error ? <Notice tone="error">{failedText(error)}</Notice> : null}
            <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
              <Button variant="primary" label={i18n.t('web.claim.approve')} onClick={() => void decide(true)} loading={phase === 'working'} disabled={phase === 'working' || (live.profiles.length > 1 && !profileId)} />
              <Button variant="ghost" label={i18n.t('web.claim.deny')} onClick={() => void decide(false)} disabled={phase === 'working'} />
            </div>
          </>
        )}
      </div>
    </AuthFrame>
  );
}
