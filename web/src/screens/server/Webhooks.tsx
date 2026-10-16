import {useEffect, useMemo, useState} from 'react';
import {changeWebhook, createWebhook, deleteWebhook, listWebhookDeliveries, listWebhooks, sendWebhookTest, webhookEdits, type IssuedWebhook, type Webhook, type WebhookDelivery} from '@core/api-access.ts';
import {deliveryProblem, deliveryProgress, deliveryTone, webhookAddressOk, webhookEventGroups, webhookEventLabel, webhookTrouble} from '@core/server-admin/api-words.ts';
import {problem, useAction, useRead} from '../../admin/console';
import {usePollWhile} from '../../admin/poll-while';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Checkbox, ConfirmDialog, Dialog, Input, Loading, Menu, Notice, PasswordInput, SettingsEmpty, SettingsFooter, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Status, Switch, Text, TextArea, type MenuItem} from '../../ui';
import {dependsOn} from '@core/freshness-hub.ts';

const WEBHOOK_MOVES = [dependsOn.list('webhooks')];

type Draft = {name: string; url: string; events: readonly string[]; secret: string};

/**
 * Integrations › Webhooks: messages this server sends to another service when something
 * happens. Each webhook is a row with its on/off switch; adding and editing are one dialog
 * (name, address, the events as a grouped checklist); the signing secret is shown once, when it
 * is made or replaced; the delivery log says what was sent and whether it arrived.
 */
export function WebhooksPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  // Whether each one is arriving changes as the server delivers; the feed says when.
  const read = useRead(() => listWebhooks(api), [api], {on: WEBHOOK_MOVES});
  const action = useAction();
  const [editing, setEditing] = useState<{webhook?: Webhook} | null>(null);
  const [issued, setIssued] = useState<IssuedWebhook>();
  const [removing, setRemoving] = useState<Webhook>();
  const [rotating, setRotating] = useState<Webhook>();
  const [log, setLog] = useState<Webhook>();
  const list = read.data;
  const full = !!list && list.items.length >= list.maximum;
  const shown = (out: IssuedWebhook) => { if (out.secret) setIssued(out); read.reload(); };
  const menu = (w: Webhook): MenuItem[] => [
    {id: 'edit', label: t('web.selection.edit'), icon: 'edit'},
    {id: 'test', label: t('api.webhooks.sendTest'), icon: 'play', disabled: !w.enabled},
    {id: 'log', label: t('api.webhooks.log'), icon: 'logs'},
    {id: 'rotate', label: t('api.webhooks.rotate'), icon: 'refresh', separatorBefore: true},
    {id: 'remove', label: t('action.removeShort'), icon: 'trash', destructive: true},
  ];
  const select = (w: Webhook, id: string) => {
    action.clear();
    if (id === 'edit') setEditing({webhook: w});
    else if (id === 'log') setLog(w);
    else if (id === 'rotate') setRotating(w);
    else if (id === 'remove') setRemoving(w);
    else if (id === 'test') void action.run(async () => { await sendWebhookTest(api, w.id); read.reload(); }, t('api.webhooks.testQueued')).then(ok => { if (ok) setLog(w); });
  };
  return (
    <SettingsGroup title={t('api.webhooks.title')} description={t('api.webhooks.lede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('api.webhooks.add')} disabled={!list || full} onClick={() => { action.clear(); setEditing({}); }} />}>
      {read.error ? <SettingsNote><Notice tone={list ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !editing && !removing && !rotating ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!list && read.loading ? <SettingsLoading thing={t('server.thing.webhooks')} /> : null}
      {list?.items.map(w => {
        const trouble = webhookTrouble(t, w);
        return (
          <SettingsRow key={w.id} icon="link" label={w.name} help={<><span style={{fontFamily: 'var(--font-mono)', fontSize: '0.75rem', overflowWrap: 'anywhere'}}>{w.url}</span>{` · ${t('api.webhooks.eventCount', {count: w.events.length})}`}</>}
            state={trouble ? <Status tone={trouble.tone}>{trouble.text}</Status> : undefined}
            control={<div style={{display: 'flex', gap: 8, alignItems: 'center'}}>
              <Switch checked={w.enabled} disabled={action.busy} label={t('api.webhooks.turnOn', {name: w.name})} onCheckedChange={on => void action.run(async () => { await changeWebhook(api, w, {enabled: on}); read.reload(); })} />
              <Menu label={w.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('api.webhooks.actions', {name: w.name})} />} items={menu(w)} onSelect={id => select(w, id)} />
            </div>} />
        );
      })}
      {list && !list.items.length ? <SettingsEmpty title={t('api.webhooks.none')} /> : null}
      {full ? <SettingsFooter><Text as="p" variant="caption" tone="tertiary">{t('api.webhooks.full', {count: list.maximum})}</Text></SettingsFooter> : null}
      {editing && list ? <WebhookDialog webhook={editing.webhook} offered={list.events} onClose={() => setEditing(null)} busy={action.busy} error={action.error}
        onSave={draft => void action.run(async () => {
          const w = editing.webhook;
          const body = {name: draft.name.trim(), url: draft.url.trim(), events: draft.events};
          shown(w ? await changeWebhook(api, w, webhookEdits(w, body)) : await createWebhook(api, {...body, ...(draft.secret ? {secret: draft.secret} : {})}));
        }).then(ok => { if (ok) setEditing(null); })} /> : null}
      <SecretDialog issued={issued} onClose={() => setIssued(undefined)} />
      <RotateDialog webhook={rotating} busy={action.busy} error={action.error} onClose={() => setRotating(undefined)}
        onRotate={secret => void action.run(async () => { if (!rotating) return; shown(await changeWebhook(api, rotating, secret ? {secret} : {rotateSecret: true})); }).then(ok => { if (ok) setRotating(undefined); })} />
      <ConfirmDialog open={!!removing} onOpenChange={o => !o && setRemoving(undefined)} title={t('api.webhooks.removeTitle', {name: removing?.name ?? ''})} body={t('api.webhooks.removeBody')} confirmLabel={t('action.removeShort')} cancelLabel={t('action.cancel')} busy={action.busy} error={action.error}
        onConfirm={() => void action.run(async () => { if (!removing) return; await deleteWebhook(api, removing.id); setRemoving(undefined); read.reload(); })} />
      {log ? <DeliveryLog webhook={log} onClose={() => { setLog(undefined); read.reload(); }} /> : null}
    </SettingsGroup>
  );
}

/** Add or edit: name, address, and what to send, ticked from the events this server offers. */
function WebhookDialog({webhook, offered, onClose, onSave, busy, error}: {webhook?: Webhook; offered: readonly string[]; onClose: () => void; onSave: (draft: Draft) => void; busy: boolean; error: string}) {
  const i18n = useI18n();
  const {t} = i18n;
  const [draft, setDraft] = useState<Draft>({name: webhook?.name ?? '', url: webhook?.url ?? '', events: webhook?.events ?? [], secret: ''});
  // An event this webhook already has stays in the checklist even if the server no longer offers it, so saving never drops it unseen.
  const groups = useMemo(() => webhookEventGroups([...offered, ...(webhook?.events ?? [])]), [offered, webhook]);
  const has = (event: string) => draft.events.includes(event);
  const set = (events: readonly string[], on: boolean) => setDraft(d => ({...d, events: on ? [...new Set([...d.events, ...events])] : d.events.filter(e => !events.includes(e))}));
  const address = draft.url.trim();
  const addressProblem = address && !webhookAddressOk(address) ? t('api.webhooks.urlProblem') : undefined;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={webhook ? t('api.webhooks.editTitle') : t('api.webhooks.add')} width={600}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={webhook ? t('action.save') : t('api.webhooks.add')} loading={busy} disabled={!draft.name.trim() || !address || !!addressProblem || !draft.events.length} onClick={() => onSave(draft)} /></>}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      <Input label={t('profile.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={120} autoFocus />
      <Input label={t('api.webhooks.url')} value={draft.url} onChange={e => setDraft({...draft, url: e.target.value})} mono placeholder="https://example.com/hooks/portico" autoCapitalize="off" spellCheck={false} maxLength={2048} help={t('api.webhooks.urlHelp')} error={addressProblem} />
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        <Text as="span" variant="label" tone="secondary">{t('api.webhooks.events')}</Text>
        {groups.map(group => {
          const all = group.events.every(has);
          return (
            <fieldset key={group.id} style={{border: 0, padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 8}}>
              <legend style={{padding: 0, marginBottom: 8, display: 'flex', alignItems: 'center', gap: 12}}>
                <Text variant="captionStrong">{t(group.label)}</Text>
                {group.events.length > 1 ? <Button size="sm" variant="link" label={all ? t('api.webhooks.groupNone') : t('api.webhooks.groupAll')} aria-label={t(all ? 'api.webhooks.groupNoneOf' : 'api.webhooks.groupAllOf', {group: t(group.label)})} onClick={() => set(group.events, !all)} /> : null}
              </legend>
              <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(230px, 1fr))', gap: 8}}>
                {group.events.map(event => <Checkbox key={event} checked={has(event)} onCheckedChange={on => set([event], on)} label={webhookEventLabel(t, id => i18n.has(id), event)} />)}
              </div>
            </fieldset>
          );
        })}
      </div>
      {webhook ? <Text as="p" variant="caption" tone="tertiary">{t('api.webhooks.secretKept', {hint: webhook.secretHint})}</Text>
        : <PasswordInput label={t('api.webhooks.secret')} optional value={draft.secret} onChange={e => setDraft({...draft, secret: e.target.value})} autoComplete="off" maxLength={200} help={t('api.webhooks.secretHelp')} />}
    </Dialog>
  );
}

/** The one time a signing secret is shown: after adding a webhook, or after replacing its secret. */
function SecretDialog({issued, onClose}: {issued?: IssuedWebhook; onClose: () => void}) {
  const {t} = useI18n();
  const [copied, setCopied] = useState(false);
  useEffect(() => { if (issued) setCopied(false); }, [issued]);
  return (
    <Dialog open={!!issued?.secret} onOpenChange={o => !o && onClose()} dismissable={false} title={t('api.webhooks.secretTitle')} width={500} actions={<Button variant="primary" label={t('web.access.copiedIt')} onClick={onClose} />}>
      <Notice tone="warning" compact>{t('api.webhooks.secretShownOnce')}</Notice>
      <TextArea label={t('api.webhooks.secret')} readOnly rows={2} value={issued?.secret ?? ''} onFocus={e => e.currentTarget.select()} style={{fontFamily: 'var(--font-mono)', fontSize: '0.8125rem'}} />
      <div style={{display: 'flex', gap: 12, alignItems: 'center'}}><Button size="sm" variant="secondary" icon="copy" label={t('web.twoStep.copy')} onClick={() => void navigator.clipboard?.writeText(issued?.secret ?? '').then(() => setCopied(true), () => {})} /><span role="status">{copied ? <Text variant="caption" tone="tertiary">{t('api.credential.copied')}</Text> : null}</span></div>
    </Dialog>
  );
}

/** Replace the secret: the old one stops being used at once, so the other service has to be given the new one. */
function RotateDialog({webhook, onClose, onRotate, busy, error}: {webhook?: Webhook; onClose: () => void; onRotate: (secret: string) => void; busy: boolean; error: string}) {
  const {t} = useI18n();
  const [secret, setSecret] = useState('');
  useEffect(() => { if (webhook) setSecret(''); }, [webhook]);
  return (
    <Dialog open={!!webhook} onOpenChange={o => !o && onClose()} dismissable={!busy} title={t('api.webhooks.rotateTitle', {name: webhook?.name ?? ''})} description={t('api.webhooks.rotateBody')} width={480}
      actions={<><Button variant="ghost" label={t('action.cancel')} disabled={busy} onClick={onClose} /><Button variant="primary" label={t('api.webhooks.rotate')} loading={busy} onClick={() => onRotate(secret)} /></>}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      <PasswordInput label={t('api.webhooks.secret')} optional value={secret} onChange={e => setSecret(e.target.value)} autoComplete="off" maxLength={200} help={t('api.webhooks.secretHelp')} />
    </Dialog>
  );
}

/**
 * The delivery log, newest first: what was sent, whether it arrived, and for one that has not,
 * why and when the server tries again. It keeps itself current while anything is still waiting.
 */
function DeliveryLog({webhook, onClose}: {webhook: Webhook; onClose: () => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const [rows, setRows] = useState<readonly WebhookDelivery[]>();
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  // The first page is read again while something waits; older pages already shown are kept under it.
  const load = (from: string) => {
    setLoading(true); setError('');
    listWebhookDeliveries(api, webhook.id, {cursor: from || undefined, limit: 50}).then(pageRead => {
      setRows(prev => {
        if (from) return [...(prev ?? []), ...pageRead.items.filter(d => !prev?.some(p => p.id === d.id))];
        const fresh = new Set(pageRead.items.map(d => d.id));
        return [...pageRead.items, ...(prev ?? []).filter(d => !fresh.has(d.id))];
      });
      if (from || !cursor) setCursor(pageRead.nextCursor ?? '');
    }, e => setError(problem(e))).finally(() => setLoading(false));
  };
  useEffect(() => { load(''); }, [api, webhook.id]); // eslint-disable-line react-hooks/exhaustive-deps
  usePollWhile(!!rows?.some(d => d.state === 'pending'), () => load(''));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('api.webhooks.log')} description={webhook.name} width={640} actions={<><Button variant="ghost" icon="refresh" label={t('action.refresh')} loading={loading} onClick={() => load('')} /><Button variant="primary" label={t('action.done')} onClick={onClose} /></>}>
      {error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => load('')}}>{error}</Notice> : null}
      {!rows && loading ? <Loading label={t('api.webhooks.log')} /> : null}
      {rows && !rows.length ? <Text variant="body" tone="secondary">{t('api.webhooks.logEmpty')}</Text> : null}
      {rows?.length ? (
        <ul style={{listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', maxHeight: 420, overflowY: 'auto'}}>
          {rows.map(d => {
            const why = deliveryProblem(t, id => i18n.has(id), d);
            const progress = deliveryProgress(t, d, iso => i18n.time(iso));
            return (
              <li key={d.id} style={{display: 'flex', flexDirection: 'column', gap: 2, padding: '12px 0', borderTop: '1px solid var(--line-soft)'}}>
                <div style={{display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap'}}>
                  <Text variant="bodyStrong">{d.event === 'webhook.test' ? t('api.webhooks.testEvent') : webhookEventLabel(t, id => i18n.has(id), d.event)}</Text>
                  <Status tone={deliveryTone(d)}>{t(`api.webhooks.delivery.${d.state}`)}</Status>
                </div>
                <Text variant="caption" tone="tertiary">{[i18n.dateTime(d.finishedAt ?? d.createdAt), progress].filter(Boolean).join(' · ')}</Text>
                {why ? <Text variant="caption" tone="secondary">{why}</Text> : null}
              </li>
            );
          })}
        </ul>
      ) : null}
      {cursor && rows?.length ? <div><Button size="sm" variant="ghost" label={t('action.showMore')} loading={loading} onClick={() => load(cursor)} /></div> : null}
    </Dialog>
  );
}
