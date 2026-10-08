import {currentI18n} from '../../app/i18n';
import React, {useRef, useState} from 'react';
import {readLibraryChannels, readLibraryDefaults, readLibraryTemplates, saveLibraryChannel, type LibraryChannel, type LibraryChannelConfig, type LibraryChannelTemplate} from '@core/library-channels.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {regenerationBody} from '@core/server-admin/panel-logic.ts';
export {regenerationBody} from '@core/server-admin/panel-logic.ts';
import {sentence, useAction, useRead} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {useSession} from '../../app/session';
import {Badge, Button, ConfirmDialog, Dialog, Freshness, KeyValue, Menu, Notice, SettingsGroup, SettingsRow, StateView, Text, type MenuItem} from '../../ui';
import {SectionHeader} from './Server';
import {ChannelEditor} from './ChannelBuilder';
import {useI18n} from '../../app/i18n';

/** CD-13: the logo fence is the logo revision, never the channel configuration revision.
 * The client cannot obtain the logo revision, so it omits the fence for an
 * unconditional replacement. An explicit zero still means "no existing logo";
 * a stale supplied revision answers 409. */
export function logoUploadForm(file: File, operationId: string): FormData {
  const form = new FormData();
  form.append('file', file);
  form.append('operationId', operationId);
  return form;
}
/** CD-13: one operation ID per logo bytes/intent, stable across retries of the
 * same file for the same channel; a different file starts a new operation. */
export function logoOperationKey(channelId: string, file: {name: string; size: number; lastModified: number}): string {
  return `${channelId}:${file.name}:${file.size}:${file.lastModified}`;
}
const localTz = () => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'; } catch { return 'UTC'; } };
const stateTone = (c: LibraryChannel): 'healthy' | 'warning' | 'danger' | 'neutral' => (!c.config.enabled ? 'neutral' : c.healthCode ? 'danger' : c.state === 'ready' || c.state === 'generated' ? 'healthy' : 'warning');

/**
 * Library Channels: always-on channels generated from the owner's media.
 * Start from a template; the editor is a short set of decisions (what plays,
 * in what order, who can watch) with the rest under Advanced.
 */
export function LibraryChannelsPanel() {
  const {api, session} = useSession();
  const {t} = useI18n();
  const serverId = session?.viewer.serverId ?? '';
  const read = useRead<LibraryChannel[]>(() => readLibraryChannels(api, serverId), [api, serverId]);
  const templates = useRead<LibraryChannelTemplate[]>(() => readLibraryTemplates(api, serverId), [api, serverId]);
  const action = useAction();
  const [editing, setEditing] = useState<{config: LibraryChannelConfig; revision: number; templateName?: string} | null>(null);
  const createBlank = () => void action.run(async () => setEditing({config: await readLibraryDefaults(api, serverId, localTz()), revision: 0}));
  // A template opens the builder filled in, for review; nothing is created until it's saved.
  const review = (t: LibraryChannelTemplate) => setEditing({config: {...structuredClone(t.config), id: 'lc-' + channelRequestID().slice(0, 24), timezone: localTz(), templateId: t.id}, revision: 0, templateName: t.name});
  const [remove, setRemove] = useState<LibraryChannel | null>(null);
  const [healthFor, setHealthFor] = useState<LibraryChannel | null>(null);
  const logoPicker = useRef<HTMLInputElement>(null);
  const logoTarget = useRef<LibraryChannel | null>(null);
  const logoOperation = useRef<{key: string; id: string} | null>(null);
  const token = session?.accessToken ?? '';
  // A logo is a file, not JSON: it goes as a form, and the server identifies the image from its bytes.
  // CD-13: no expectedRevision (the configuration revision is not the logo revision);
  // the operation ID is stable for retries of the same bytes/intent.
  const uploadLogo = (file?: File) => {
    const channel = logoTarget.current;
    if (!file || !channel) return;
    const key = logoOperationKey(channel.config.id, file);
    const held = logoOperation.current;
    const operationId = held && held.key === key ? held.id : channelRequestID();
    logoOperation.current = {key, id: operationId};
    mutate(async () => {
      if (file.size > 10 * 1024 * 1024) throw new Error('Choose an image smaller than 10 MB.');
      const form = logoUploadForm(file, operationId);
      const response = await api.routeFetch(`${api.baseUrl}/v1/admin/library-channels/${encodeURIComponent(channel.config.id)}/logo`, {method: 'POST', headers: {Authorization: 'Bearer ' + token}, body: form});
      if (!response.ok) { const value = await response.json().catch(() => null); const code = (value?.error as {code?: unknown} | undefined)?.code; throw Object.assign(new Error(t('web.libraryChannels.logoSaveFailed')), typeof code === 'string' && code ? {code} : {}); }
      logoOperation.current = null;
    }, 'Logo saved.');
  };
  const channels = [...(read.data ?? [])].sort((a, b) => a.config.position - b.config.position);
  const mutate = (fn: () => Promise<unknown>, done?: string) => void action.run(fn, done).then(ok => ok && read.reload());
  // BE-API-10: one ID per logical write; retries of the same payload reuse it.
  const [opIds] = useState(createOperationIds);
  const move = (index: number, dir: -1 | 1) => {
    const next = [...channels];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target]!, next[index]!];
    const payload = {channels: next.map(c => ({id: c.config.id, expectedRevision: c.revision}))};
    const key = JSON.stringify(payload);
    mutate(async () => {
      await api.request('/v1/admin/library-channels/reorder', 'POST', {requestId: opIds.forPayload(key), ...payload});
      opIds.release();
    });
  };
  return (
    <>
      <SectionHeader lede={t('web.console.nav.channelsLede')} actions={<Menu label={t('builder.newTitle')} trigger={<Button variant="primary" icon="plus" label={t('builder.newTitle')} iconAfter="chevronDown" />} items={[...(templates.data ?? []).map<MenuItem>(t => ({id: 'tpl:' + t.id, label: t.name, meta: t.applicable ? undefined : sentence(t.reason || 'Not enough media'), disabled: !t.applicable, group: 'Templates'})), ...templateStatus(templates), {id: 'blank', label: t('web.libraryChannels.startFromScratch'), icon: 'plus', group: 'Custom'}]} onSelect={id => {
        if (id === 'templates-retry') templates.reload();
        else if (id === 'blank') createBlank();
        else { const template = templates.data?.find(t => 'tpl:' + t.id === id); if (template) review(template); }
      }} />} />
      {read.error && !read.data ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
      {action.error ? <Notice tone="error">{action.error}</Notice> : null}
      <div style={{display: 'flex', flexDirection: 'column', gap: 28}}>
        {read.data && !channels.length ? (
          <StateView icon="channels" title={t('builder.empty.title')} body={t('builder.empty.body')} action={{label: t('builder.empty.create'), onClick: createBlank, loading: action.busy}}>
            {(templates.data ?? []).length ? (
              <div style={{display: 'flex', flexWrap: 'wrap', gap: 8, justifyContent: 'center'}}>
                {(templates.data ?? []).map(tpl => <Button key={tpl.id} variant="secondary" size="sm" label={tpl.name} disabled={!tpl.applicable} title={tpl.applicable ? undefined : tpl.reason} onClick={() => review(tpl)} />)}
              </div>
            ) : null}
          </StateView>
        ) : null}
        {channels.length ? (
          <SettingsGroup title={t('channels.title')} action={<Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} />}>
            {channels.map((c, i) => <SettingsRow key={c.config.id} icon="channels" label={c.config.name} help={`${c.config.description || (c.config.rules.length ? `${c.config.rules.length} rule${c.config.rules.length === 1 ? '' : 's'}` : 'No rules')} · ${c.candidateCount.toLocaleString()} titles${c.generatedThrough ? ` · scheduled through ${new Date(c.generatedThrough).toLocaleString()}` : ''}${c.healthCode ? ` · ${sentence(c.healthCode)}` : ''}`} meta={<Badge tone={stateTone(c)} dot>{!c.config.enabled ? t('web.libraryChannels.offState') : sentence(c.state)}</Badge>} control={<Menu label={c.config.name} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('web.storage.actions')} />} items={[{id: 'edit', label: t('web.selection.edit'), icon: 'edit'}, {id: 'toggle', label: c.config.enabled ? t('web.libraryChannels.turnOff') : t('web.libraryChannels.turnOn'), icon: c.config.enabled ? 'pause' : 'play'}, {id: 'regen', label: t('web.libraryChannels.regenSchedule'), icon: 'refresh', disabled: !c.replacementBoundary}, {id: 'health', label: t('web.libraryChannels.checkHealth'), icon: 'pulse'}, {id: 'logo', label: t('web.libraryChannels.uploadLogo'), icon: 'upload'}, {id: 'up', label: t('web.profile.moveUp'), icon: 'chevronUp', disabled: i === 0, separatorBefore: true}, {id: 'down', label: t('web.profile.moveDown'), icon: 'chevronDown', disabled: i === channels.length - 1}, {id: 'delete', label: t('web.libraryChannels.delete'), icon: 'trash', destructive: true, separatorBefore: true}]} onSelect={id => {
              if (id === 'edit') setEditing({config: c.config, revision: c.revision});
              else if (id === 'health') setHealthFor(c);
              else if (id === 'logo') { logoTarget.current = c; logoPicker.current?.click(); }
              else if (id === 'toggle') mutate(async () => {
                const next = {...c.config, enabled: !c.config.enabled};
                const key = JSON.stringify({config: next, revision: c.revision});
                const requestId = opIds.forPayload(key);
                try {
                  await saveLibraryChannel(api, serverId, next, c.revision, requestId);
                  opIds.release();
                } catch (e) {
                  if ((e as {status?: number})?.status === 409) opIds.release();
                  throw e;
                }
              });
              else if (id === 'regen') mutate(async () => {
                const key = JSON.stringify({id: c.config.id, revision: c.revision, boundary: c.replacementBoundary});
                try {
                  await api.request(`/v1/admin/library-channels/${encodeURIComponent(c.config.id)}/regenerate`, 'POST', regenerationBody(c, opIds.forPayload(key)));
                  opIds.release();
                } catch (e) {
                  // A stale boundary or revision changed under us: reload so the
                  // next attempt reviews the fresh boundary with a new intent ID.
                  if ((e as {status?: number})?.status === 409 || (e as {code?: string})?.code === 'library_channel_changed') { opIds.release(); read.reload(); }
                  throw e;
                }
              }, 'Regenerating.');
              else if (id === 'up') move(i, -1);
              else if (id === 'down') move(i, 1);
              else setRemove(c);
            }} />} />)}
          </SettingsGroup>
        ) : null}
      </div>
      <input ref={logoPicker} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={e => { const file = e.target.files?.[0]; e.target.value = ''; uploadLogo(file); }} />
      {healthFor ? <ChannelHealthDialog channel={healthFor} onClose={() => setHealthFor(null)} /> : null}
      <ChannelEditor value={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); read.reload(); }} />
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} title={t('web.libraryChannels.deleteTitle', {name: remove?.config.name ?? ''})} body={t('web.libraryChannels.deleteBody')} confirmLabel={t('web.libraryChannels.delete')} busy={action.busy} onConfirm={() => { if (remove) { const payload = {expectedRevision: remove.revision}; const key = JSON.stringify({id: remove.config.id, ...payload}); void action.run(async () => { await api.request(`/v1/admin/library-channels/${encodeURIComponent(remove.config.id)}/delete`, 'POST', {requestId: opIds.forPayload(key), ...payload}); opIds.release(); }).then(ok => { if (ok) { setRemove(null); read.reload(); } }); } }} />
    </>
  );
}

type Health = {healthy: boolean; state: string; message: string; generatedThrough: string; scheduledEntries: number; candidates: number; unresolved: number; pendingGeneration: boolean};

/** Whether a channel is actually on the air, and in a sentence what is wrong if it is not. */
function ChannelHealthDialog({channel, onClose}: {channel: LibraryChannel; onClose: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const read = useRead(async () => { const raw = await api.request<{result?: Health} & Health>(`/v1/admin/library-channels/${encodeURIComponent(channel.config.id)}/health`); return (raw.result ?? raw) as Health; }, [api, channel.config.id]);
  const h = read.data;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={channel.config.name} description={t('web.libraryChannels.healthLede')} width={480} actions={<><Button variant="ghost" icon="refresh" label={t('device.approval.checkAgain')} loading={read.loading} onClick={read.reload} /><Button variant="primary" label={t('action.done')} onClick={onClose} /></>}>
      {read.error ? <Notice tone="error" compact>{read.error}</Notice> : null}
      {h ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          <Notice tone={h.healthy ? 'success' : 'warning'} compact>{h.message || (h.healthy ? t('web.libraryChannels.onAir') : t('web.libraryChannels.offAir'))}</Notice>
          <KeyValue rows={[
            [t('web.libraryChannels.healthSchedule'), h.generatedThrough ? new Date(h.generatedThrough).toLocaleString(undefined, {weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'}) : t('web.libraryChannels.healthNotGenerated')],
            [t('web.libraryChannels.healthPrograms'), h.scheduledEntries.toLocaleString()],
            [t('web.libraryChannels.healthCandidates'), h.candidates.toLocaleString()],
            ...(h.unresolved ? [[t('web.libraryChannels.healthUnresolved'), h.unresolved.toLocaleString()] as [React.ReactNode, React.ReactNode]] : []),
            [t('web.libraryChannels.healthNewSchedule'), h.pendingGeneration ? t('web.libraryChannels.healthBeingMade') : t('web.libraryChannels.healthUpToDate')],
          ]} />
        </div>
      ) : !read.error ? <Text variant="caption" tone="tertiary">{t('savedServer.checking')}</Text> : null}
    </Dialog>
  );
}

/** When the templates can't be read, the menu says so and offers to try again instead of hiding them. */
function templateStatus(templates: {data?: unknown[]; error?: string; loading: boolean}): MenuItem[] {
  const t = currentI18n().t;
  if (templates.loading && !templates.data) return [{id: 'templates-loading', label: t('web.channelBuilder.templatesLoading'), disabled: true, group: t('web.channelBuilder.templates')}];
  if (templates.error && !templates.data) return [{id: 'templates-retry', label: t('web.channelBuilder.templatesFailed'), icon: 'refresh', group: t('web.channelBuilder.templates')}];
  return [];
}
