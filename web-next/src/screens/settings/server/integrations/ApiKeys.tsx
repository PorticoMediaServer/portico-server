import {useEffect, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {ApiCredential, ApiScope, IssuedApiCredential} from '@core/api-access.ts';
import {EXPIRY_DAYS, chosenScopes, credentialState, credentialWorks, expiryInstant, expiryLabel, scopeTicked, scopesText, type ExpiryDays} from '@core/server-admin/api-words.ts';
import {Button, Select, TextField} from '../../../../kit/controls';
import {Dialog} from '../../../../kit/overlays/Dialog';
import {Icon} from '../../../../kit/Icon';
import {AssistantsPanel} from '../../integrations/Assistants';
import {SettingsApiKeysSource} from './api-keys-source';
import styles from './integrations.module.css';

const t = defaultI18n.t;
type Props = Readonly<{source: SettingsApiKeysSource; baseUrl: string; isCurrent: () => boolean; reportError: (error: unknown) => void; otherRows: ReactNode; advancedRows: ReactNode}>;

/** The owner's real key list and creator; generic webhook/advanced rows retain
 * their existing controller, while Assistants invokes this same key creator. */
export function IntegrationsPage({source, baseUrl, isCurrent, reportError, otherRows, advancedRows}: Props) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  const [creating, setCreating] = useState(false), [name, setName] = useState(''), [ticked, setTicked] = useState<ReadonlySet<ApiScope>>(new Set()), [days, setDays] = useState<ExpiryDays>(0);
  const [issued, setIssued] = useState<{source: SettingsApiKeysSource; value: IssuedApiCredential}>(), [copied, setCopied] = useState(false), [ended, setEnded] = useState(false), [revoking, setRevoking] = useState<ApiCredential>();
  const [cursors, setCursors] = useState<readonly string[]>(['']);
  const generation = useRef(0), paging = useRef(false), secret = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    generation.current++;setIssued(undefined);setCreating(false);setRevoking(undefined);setCursors(['']);void source.load();
    return () => {generation.current++;};
  }, [source]);
  const live = isCurrent(), heldIssue = live && issued?.source === source ? issued.value : undefined;
  const offered = state.options?.scopes ?? [], chosen = chosenScopes(ticked).filter(scope => offered.includes(scope));
  const busy = state.busy || state.phase === 'loading', frozen = state.busy || state.pendingCreate;
  const working = state.page?.items.filter(credentialWorks) ?? [], stopped = state.page?.items.filter(key => !credentialWorks(key)) ?? [];
  const start = () => {
    if (!isCurrent() || state.busy) return;
    if (!state.pendingCreate) {setName('');setTicked(new Set(offered.includes('read') ? ['read'] : []));setDays(0);}
    setCreating(true);
  };
  const submit = async () => {
    if (!isCurrent() || state.busy) return;
    const held = generation.current;
    const out = await (state.pendingCreate ? source.retryCreate() : source.create({name, scopes: chosen, ...(days ? {expiresAt: expiryInstant(days, Date.now())} : {})}));
    if (out && isCurrent() && held === generation.current) {setCreating(false);setCopied(false);setIssued({source, value: out});}
  };
  const copy = async () => {
    if (!isCurrent() || !heldIssue?.secret) return;
    const held = generation.current;
    try {await navigator.clipboard.writeText(heldIssue.secret);if (isCurrent() && held === generation.current) setCopied(true);}
    catch {if (isCurrent() && held === generation.current) {setCopied(false);secret.current?.focus();secret.current?.select();}}
  };
  const page = async (cursor: string, next: readonly string[]) => {
    if (!isCurrent() || paging.current || state.busy) return;
    paging.current = true;const held = generation.current;
    try {await source.load(cursor);if (isCurrent() && held === generation.current && source.getSnapshot().phase === 'ready') setCursors(next);}
    finally {paging.current = false;}
  };
  const row = (key: ApiCredential) => {
    const badge = credentialState(t, defaultI18n.has, key);
    return <li key={key.id} className={styles.keyRow}>
      <span className={styles.keyIcon}><Icon name="i-key" size={20} /></span>
      <div className={styles.keyText}><b>{key.name}</b><div className={styles.facts}>
        <span>{scopesText(t, key.scopes)}</span>{key.hint && <span>{t('api.credential.endsIn', {hint: key.hint})}</span>}
        <span>{key.lastUsedAt ? t('api.credential.lastUsed', {when: defaultI18n.relativeTime(key.lastUsedAt)}) : t('api.credential.neverUsed')}</span>
        <span>{key.expiresAt ? t('api.credential.expires', {when: defaultI18n.date(key.expiresAt)}) : t('api.credential.noExpiry')}</span>
      </div></div>
      <div className={styles.actions}><span className={styles.badge} data-tone={badge?.tone ?? 'healthy'}>{badge?.text ?? t('api.credential.state.active')}</span>
        {credentialWorks(key) && <Button size="xs" variant="text" disabled={busy} onClick={() => {if (isCurrent()) setRevoking(key);}}>{t('web.access.revoke')}</Button>}</div>
    </li>;
  };
  return <div className={styles.page}>
    <section id="integrations.apiKeys" aria-labelledby="api-keys-title">
      <div className={styles.heading}><h3 id="api-keys-title">{t('web.access.apiKeys')}</h3><Button size="xs" variant="glass" disabled={busy || !state.options} onClick={start}>{t('web.access.createKey')}</Button></div>
      <p className={styles.lede}>{t('server.access.keyLede')}</p>
      {state.error && <div className={styles.failure} role="alert"><span>{t(state.error === 'read' ? 'error.unavailable' : 'error.unconfirmedChange')}</span><Button size="xs" variant="glass" disabled={busy} onClick={() => {if (isCurrent()) void source.load();}}>{t('action.refresh')}</Button></div>}
      {state.phase === 'loading' && !state.page && <p role="status" className={styles.note}>{t('next.settings.loading')}</p>}
      {state.page && <><ul className={styles.keys}>{working.map(row)}{ended && stopped.map(row)}</ul>{!working.length && <p className={styles.note}>{t('web.access.noApiKeys')}</p>}
        {stopped.length > 0 && <Button size="xs" variant="text" aria-expanded={ended} onClick={() => setEnded(value => !value)}>{t(ended ? 'api.credential.hideEnded' : 'api.credential.showEnded', {count: stopped.length})}</Button>}
        {(cursors.length > 1 || state.page.nextCursor) && <div className={styles.pager}><Button size="xs" variant="glass" disabled={busy || cursors.length < 2} onClick={() => {const next = cursors.slice(0, -1);void page(next[next.length - 1] ?? '', next);}}>{t('web.pager.previous')}</Button>
          <Button size="xs" variant="glass" disabled={busy || !state.page.nextCursor} onClick={() => {const next = state.page?.nextCursor;if (next) void page(next, [...cursors, next]);}}>{t('web.pager.next')}</Button></div>}
      </>}
      <p className={styles.footnote}>{t('api.tokens.lifetime')}</p>
    </section>
    {otherRows}
    <div id="integrations.assistantAddress"><div id="integrations.assistantKey"><AssistantsPanel baseUrl={baseUrl} isCurrent={isCurrent} openCreateKey={start} reportError={reportError} /></div></div>
    {advancedRows}
    <Dialog open={live && creating} onOpenChange={open => {if (!open && !state.busy) setCreating(false);}} title={t('web.access.createKeyTitle')} width={500}
      dismissOnOutsidePress={!state.busy} dismissOnBack={!state.busy} showCloseButton={!state.busy}
      footer={<><Button variant="glass" size="compact" disabled={state.busy} onClick={() => setCreating(false)}>{t('action.cancel')}</Button><Button variant="primary" size="compact" disabled={state.busy || (!state.pendingCreate && (!name.trim() || !chosen.length || !state.options))} onClick={() => {void submit();}}>{t(state.pendingCreate ? 'action.tryAgain' : 'web.saved.create')}</Button></>}>
      <div className={styles.form}>
        {state.error === 'create' && <p className={styles.failure} role="alert">{t('error.unconfirmedChange')}</p>}
        <TextField label={t('profile.name')} description={t('web.access.keyNameHelp')} maxLength={80} disabled={frozen} value={name} onChange={event => setName(event.target.value)} />
        <fieldset className={styles.scopes} disabled={frozen}><legend>{t('web.access.scopeLabel')}</legend>{offered.map(scope => <label key={scope} className={styles.scope}>
          <input type="checkbox" checked={scopeTicked(ticked, scope)} disabled={scope !== 'admin' && ticked.has('admin')} onChange={event => setTicked(previous => {const next = new Set(previous);if (event.target.checked) next.add(scope);else next.delete(scope);return next;})} />
          <span><b>{t(`api.scope.${scope}` as MessageId)}</b><small>{t(`api.scope.${scope}.help` as MessageId)}</small></span>
        </label>)}</fieldset>
        <p className={styles.note}>{t('api.scopes.note')}</p>
        <Select label={t('api.credential.expiryLabel')} value={String(days)} disabled={frozen} options={EXPIRY_DAYS.map(value => ({value: String(value), label: expiryLabel(t, value)}))} onValueChange={value => setDays(Number(value) as ExpiryDays)} />
      </div>
    </Dialog>
    <Dialog open={!!heldIssue} onOpenChange={open => {if (!open && !heldIssue?.secret) setIssued(undefined);}} title={t('web.access.copyKeyTitle')} width={500} initialFocusRef={secret}
      dismissOnOutsidePress={!heldIssue?.secret} dismissOnBack={!heldIssue?.secret} showCloseButton={false}
      footer={<Button variant="primary" size="compact" onClick={() => {setIssued(undefined);setCopied(false);}}>{t(heldIssue?.secret ? 'web.access.copiedIt' : 'action.done')}</Button>}>
      {heldIssue?.secret ? <div className={styles.form}><p className={styles.warning}>{t('web.access.keyShownOnce')}</p><label className={styles.secretLabel}>{t('web.access.apiKey')}<textarea ref={secret} className={styles.secret} readOnly rows={3} value={heldIssue.secret} onFocus={event => event.currentTarget.select()} /></label>
        <div className={styles.actions}><Button size="xs" variant="glass" icon="i-copy" onClick={() => {void copy();}}>{t('web.twoStep.copy')}</Button><span className={styles.copied} role="status">{copied && t('api.credential.copied')}</span></div></div>
        : <p className={styles.warning}>{t('api.credential.secretNotShown')}</p>}
    </Dialog>
    <Dialog open={live && !!revoking} onOpenChange={open => {if (!open && !state.busy) setRevoking(undefined);}} title={t('web.access.revokeTitle', {name: revoking?.name ?? ''})} description={t('web.access.revokeBody')} width={460}
      dismissOnOutsidePress={!state.busy} dismissOnBack={!state.busy} showCloseButton={!state.busy}
      footer={<><Button size="compact" variant="glass" disabled={state.busy} onClick={() => setRevoking(undefined)}>{t('action.cancel')}</Button><Button size="compact" variant="glass" className={styles.danger} disabled={state.busy} onClick={() => {if (!revoking || !isCurrent()) return;const held = generation.current;void source.revoke(revoking).then(() => {if (isCurrent() && held === generation.current && !source.getSnapshot().error) setRevoking(undefined);});}}>{t('web.access.revoke')}</Button></>}>
      {state.error === 'revoke' && <p className={styles.failure} role="alert">{t('error.unconfirmedChange')}</p>}
    </Dialog>
  </div>;
}
