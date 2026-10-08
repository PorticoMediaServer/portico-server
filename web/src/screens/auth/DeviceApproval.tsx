import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {HttpLocalApi} from '@core/index.ts';
import {SetupCodeApprover, type SetupPreview} from '@core/setup-code.ts';
import {browserAccount} from '../../bridge/account';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {useFragmentSecrets} from '../../app/link-fragment';
import {AccountForm} from './AccountForm';
import {Button, Input, KeyValue, Notice, QR, Spinner, Text} from '../../ui';
import {AuthFrame} from './AuthFrame';
import s from './Auth.module.css';
import {errorText} from '../../app/errors';

const platforms: Record<string, string> = {ios: 'iPhone or iPad', tvos: 'Apple TV', android: 'Android', androidtv: 'Android TV', web: 'Web browser', macos: 'Mac', roku: 'Roku'};

/**
 * Approve a code shown on another device. The authority is chosen first:
 * a Portico Account code, or a Quick Connect code from this server (which
 * requires being signed in to that server here as its owner).
 */
export function DeviceApprovalScreen() {
  const {t} = useI18n();
  const session = useSession();
  const central = useMemo(browserAccount, []);
  const account = useSyncExternalStore(central.service.subscribe, central.service.getSnapshot);
  const linked = useFragmentSecrets(['code'] as const);
  const [code, setCode] = useState(linked.code ?? '');
  const [preview, setPreview] = useState<SetupPreview>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState('');
  const owner = useRef<{approver: SetupCodeApprover; accountId?: string; familyId?: string}>(null);
  const localApi = session.session?.viewer.authority === 'local' && session.owner ? session.api : undefined;
  useEffect(() => {
    setPreview(undefined);
    setDone('');
    owner.current = null;
  }, [account.session?.account.id, localApi]);
  async function run(work: (signal: AbortSignal) => Promise<void>) {
    if (pending) return;
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), 15000);
    setPending(true);
    setError('');
    try {
      await work(abort.signal);
    } catch (e) {
      // X-04: catalogue copy only. A cancellation stays silent (no fallback text).
      setError(abort.signal.aborted ? t('web.device.timeoutKept') : errorText(e, 'devices', 'action'));
    } finally {
      clearTimeout(timer);
      setPending(false);
    }
  }
  /** WEB-AUTH-03: the code comes first. It is looked up with the Portico Account, then with this
   * server (Quick Connect, owner only), and whichever knows it answers; nobody is asked where
   * a code "came from". */
  const review = () => void run(async signal => {
    const tries: (() => Promise<NonNullable<typeof owner.current>>)[] = [];
    if (account.session) tries.push(async () => {
      const live = await central.service.accessSession();
      return {approver: new SetupCodeApprover(new HttpLocalApi(central.api.origin, live.accessToken), 'hosted'), accountId: live.account.id, familyId: live.familyId};
    });
    if (localApi) tries.push(async () => ({approver: new SetupCodeApprover(localApi, 'local')}));
    let last: unknown;
    for (const make of tries) {
      try {
        const o = await make();
        const value = await o.approver.review(code, signal);
        owner.current = o;
        setPreview(value);
        return;
      } catch (e) {
        if (signal.aborted) throw e;
        last = e;
        // Only "not this authority's code" moves on; anything else is the answer.
        const status = (e as {status?: number}).status;
        if (status !== undefined && ![400, 404, 409, 410, 422].includes(status)) throw e;
      }
    }
    throw tries.length > 1 || (last as {status?: number})?.status === 404 ? new Error(t('web.device.notFound')) : last;
  });
  const decide = (decision: 'approve' | 'deny') => {
    const o = owner.current, current = preview;
    if (!o || !current) return;
    void run(async signal => {
      if (o.accountId) {
        await central.service.synchronize();
        const live = central.service.getSnapshot().session;
        if (live?.account.id !== o.accountId || live.familyId !== o.familyId) throw new Error('The approving account changed. Review the code again.');
      }
      await o.approver.decide(current, decision, signal);
      setDone(decision === 'approve' ? 'Approved. Return to the device to continue.' : 'The request was denied.');
      setPreview(undefined);
    });
  };
  const ready = !!account.session || !!localApi;
  return (
    <AuthFrame title={t('web.device.title')} lede={t('web.device.lede')}>
      <div className={s.stack}>
        {!ready ? (
          <>
            <Text variant="caption" tone="secondary" center>{t('web.device.signIn')}</Text>
            <AccountForm onSignedIn={() => {}} />
          </>
        ) : null}
        {account.session ? <Text variant="caption" tone="secondary" center>{t('web.device.approvingAs', {name: account.session.account.displayName || account.session.account.username})}</Text> : null}
        {done ? <Notice tone="success">{done}</Notice> : preview ? (
          <>
            <KeyValue rows={[['Device', preview.deviceName], ['Platform', platforms[preview.platform] ?? preview.platform.replaceAll('_', ' ')], ['Code', preview.userCode]]} />
            <Text variant="caption" tone="secondary">{t('web.device.approveWarn')}</Text>
            <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
              <Button variant="primary" label={t('web.device.approveDevice')} onClick={() => decide('approve')} loading={pending} />
              <Button variant="danger" label={t('device.deny')} onClick={() => decide('deny')} disabled={pending} />
              <Button variant="ghost" label={t('web.device.anotherCode')} onClick={() => setPreview(undefined)} disabled={pending} />
            </div>
          </>
        ) : (
          <form className={s.stack} onSubmit={e => { e.preventDefault(); review(); }}>
            <Input label={t('auth.code')} code value={code} onChange={e => setCode(e.target.value.toUpperCase())} maxLength={12} autoComplete="off" autoCapitalize="characters" spellCheck={false} placeholder="ABCD-EFGH" /> {/* lint-strings-allow: a code format example, not copy */}
            <Button variant="primary" type="submit" label={t('web.device.review')} loading={pending} disabled={!ready || code.replace(/[^A-Z0-9]/gi, '').length < 8} block />
          </form>
        )}
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Button variant="link" label={t('web.claim.backToPortico')} to="/" />
      </div>
    </AuthFrame>
  );
}
