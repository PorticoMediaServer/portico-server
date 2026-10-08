import {useState} from 'react';
import {isMetadataConflict, metadataAgentPath, parseMetadataAgent, saveMetadataAgent, type LibraryMetadataAgent, type MetadataAgentId} from '../../admin/metadata-agent';
import {problem, unconfigured, useRead, type Read} from '../../admin/console';
import {lookupStatus, saveLookup, type ScreenLookupPolicy} from '../../admin/metadata-screen';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Checkbox, ConfirmDialog, Notice, SettingsGroup, Text} from '../../ui';

/** The library's metadata source; null on servers without the endpoint. */
export function useMetadataAgent(libraryId: string, deps: readonly unknown[] = []): Read<LibraryMetadataAgent | null> {
  const {api} = useSession();
  return useRead(async () => {
    const raw = await unconfigured(api.request<unknown>(metadataAgentPath(libraryId)));
    return raw === null ? null : parseMetadataAgent(raw);
  }, [api, libraryId, ...deps]);
}

/**
 * Add library › Metadata: the source a new library starts with (online by default). The copy is
 * fixed here because the server lists the choices per library kind only once the library exists.
 */
export function MetadataSourceChoice({value, onChange}: {value: 'online' | 'local'; onChange: (v: 'online' | 'local') => void}) {
  const {t} = useI18n();
  return (
    <div role="radiogroup" aria-label={t('web.metadataSource.addLabel')} style={{display: 'flex', flexDirection: 'column', gap: 8}}>
      <Text as="span" variant="label" tone="secondary">{t('web.metadataSource.addLabel')}</Text>
      <Checkbox radio checked={value === 'online'} onCheckedChange={v => v && onChange('online')} label={t('web.metadataSource.onlineName')} help={t('web.metadataSource.onlineDescription')} />
      <Checkbox radio checked={value === 'local'} onCheckedChange={v => v && onChange('local')} label={t('web.metadataSource.localName')} help={t('web.metadataSource.localDescription')} />
      <Text as="span" variant="caption" tone="tertiary">{t('web.metadataSource.addHelper')}</Text>
    </div>
  );
}

/**
 * Edit library › Metadata source: the server's choices for this library (name and description as
 * the server words them). Picking another one asks first, then saves it on its own revision; the
 * rest of the dialog keeps its own Save. A conflict reloads and shows what's current.
 */
export function MetadataSourceGroup({libraryId, agent, screen, onSaved}: {libraryId: string; agent: Read<LibraryMetadataAgent | null>; screen?: ScreenLookupPolicy | null; onSaved: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const [pending, setPending] = useState<MetadataAgentId | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [granting, setGranting] = useState(false);
  const current = agent.data;
  if (!current) return agent.error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: agent.reload}}>{agent.error}</Notice> : null;
  const request = <T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body);
  const toLocal = pending === 'local';
  const confirm = async () => {
    if (!pending) return;
    setBusy(true); setError('');
    try {
      await saveMetadataAgent(request, current, libraryId, pending);
      setPending(null);
      setNotice(t('web.metadataSource.saved'));
      agent.reload();
      onSaved();
    } catch (e) {
      if (isMetadataConflict(e)) { setPending(null); setNotice(t('web.metadataSource.changed')); agent.reload(); }
      else setError(problem(e, 'action'));
    } finally { setBusy(false); }
  };
  // Online lookups that still wait for the owner's OK (server-wide consent never given).
  const waiting = current.agent === 'online' && screen && lookupStatus(screen) === 'needs_consent';
  const grant = async () => {
    if (!screen) return;
    setGranting(true); setError('');
    try { await saveLookup(request, libraryId, screen, true); onSaved(); } catch (e) { setError(problem(e, 'action')); } finally { setGranting(false); }
  };
  return (
    <SettingsGroup title={t('web.metadataSource.title')}>
      <div role="radiogroup" aria-label={t('web.metadataSource.title')} style={{display: 'flex', flexDirection: 'column', gap: 8, padding: '12px 16px'}}>
        {current.agents.map(a => <Checkbox key={a.id} radio checked={current.agent === a.id} disabled={busy} onCheckedChange={v => { if (v && a.id !== current.agent) { setError(''); setNotice(''); setPending(a.id); } }} label={a.name} help={a.description} />)}
      </div>
      {waiting ? <div style={{padding: '0 16px 12px'}}><Notice tone="warning" compact action={{label: t('web.metadataSource.allowLookups'), onClick: () => void grant()}}>{t('web.metadataLookup.status.needsConsent')}</Notice></div> : null}
      {notice ? <div style={{padding: '0 16px 12px'}}><Notice tone={notice === t('web.metadataSource.saved') ? 'success' : 'warning'} compact>{notice}</Notice></div> : null}
      {error && !pending ? <div style={{padding: '0 16px 12px'}}><Notice tone="error" compact>{error}</Notice></div> : null}
      <ConfirmDialog
        open={!!pending}
        onOpenChange={o => { if (!o && !busy) { setPending(null); setError(''); } }}
        title={t(toLocal ? 'web.metadataSource.toLocalTitle' : 'web.metadataSource.toOnlineTitle')}
        body={t(toLocal ? 'web.metadataSource.toLocalBody' : 'web.metadataSource.toOnlineBody')}
        confirmLabel={t(toLocal ? 'web.metadataSource.toLocalConfirm' : 'web.metadataSource.toOnlineConfirm')}
        destructive={false}
        busy={busy || granting}
        error={pending ? error : undefined}
        onConfirm={confirm}
      />
    </SettingsGroup>
  );
}
