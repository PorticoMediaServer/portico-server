import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {FeedbackService, type FeedbackDraft} from '../../../../packages/client-core/src/index';
import {useSession} from '../../app/session';
import {currentI18n} from '../../app/i18n';
import {Button, Checkbox, Dialog, Loading, Notice, Select, Text, TextArea} from '../../ui';

/** Report a problem. Opened from the player it arrives knowing what was playing; the server,
 * not this dialog, decides what diagnostics that becomes. What someone typed survives a
 * failed send. */
export function FeedbackDialog({open, onOpenChange, itemId, playbackSessionId, title}: {open: boolean; onOpenChange: (open: boolean) => void; itemId?: string; playbackSessionId?: string; title?: string}) {
  const {api} = useSession();
  const t = currentI18n().t;
  const service = useMemo(() => new FeedbackService(api), [api, open]);
  const state = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const [draft, setDraft] = useState<FeedbackDraft>({kind: '', category: '', message: '', attachDiagnostics: true});
  useEffect(() => { if (open) void service.open(); }, [open, service]);
  const capabilities = state.capabilities;
  // Start on playback when the dialog came from the player, otherwise the first kind.
  useEffect(() => {
    if (!capabilities || draft.kind) return;
    const kind = (playbackSessionId ? capabilities.kinds.find(k => k.categories.some(c => c.wantsPlaybackSession)) : undefined) ?? capabilities.kinds[0];
    setDraft(d => ({...d, kind: kind.id, category: kind.categories[0].id}));
  }, [capabilities, draft.kind, playbackSessionId]);
  const kind = capabilities?.kinds.find(k => k.id === draft.kind);
  const category = kind?.categories.find(c => c.id === draft.category);
  const complete = {...draft, itemId, playbackSessionId};
  const problem = state.phase === 'ready' || state.phase === 'sending' ? service.problem(complete) : '';
  const close = () => onOpenChange(false);

  if (state.phase === 'sent') {
    const duplicate = state.result?.duplicate;
    return (
      <Dialog open={open} onOpenChange={onOpenChange} title={duplicate ? t('feedback.duplicateTitle') : t('feedback.sentTitle')} width={460} actions={<Button variant="primary" label={t('action.done')} onClick={close} />}>
        <Text as="p" variant="body" tone="secondary">{duplicate ? t('feedback.duplicateBody') : t('feedback.sentBody')}</Text>
      </Dialog>
    );
  }
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('feedback.title')}
      description={title ? t('feedback.about', {title}) : undefined}
      width={520}
      actions={state.phase === 'unavailable' ? <Button variant="secondary" label={t('action.close')} onClick={close} /> : <><Button variant="ghost" label={t('action.cancel')} onClick={close} /><Button variant="primary" label={t('feedback.send')} disabled={!!problem || state.phase !== 'ready'} loading={state.phase === 'sending'} onClick={() => void service.submit(complete)} /></>}
    >
      {state.phase === 'loading' ? <Loading label={t('feedback.loadingForm')} /> : null}
      {state.phase === 'unavailable' ? <Notice tone="info">{state.error}</Notice> : null}
      {capabilities && state.phase !== 'unavailable' && state.phase !== 'loading' ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
          {state.error ? <Notice tone="error" compact>{state.error}</Notice> : null}
          <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12}}>
            <Select label={t('feedback.kind')} value={draft.kind} onChange={e => { const next = capabilities.kinds.find(k => k.id === e.target.value); if (next) setDraft(d => ({...d, kind: next.id, category: next.categories[0].id})); }} options={capabilities.kinds.map(k => ({value: k.id, label: k.label}))} />
            <Select label={t('feedback.category')} value={draft.category} onChange={e => setDraft(d => ({...d, category: e.target.value}))} options={(kind?.categories ?? []).map(c => ({value: c.id, label: c.label}))} />
          </div>
          {category?.description ? <Text as="p" variant="caption" tone="tertiary">{category.description}</Text> : null}
          <TextArea label={t('feedback.message')} rows={5} maxLength={capabilities.maxMessageLength * 2} value={draft.message} onChange={e => setDraft(d => ({...d, message: e.target.value}))} placeholder={t('feedback.messagePlaceholder')} />
          <div style={{display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'baseline'}}>
            <Text variant="caption" tone={problem && draft.message ? 'danger' : 'tertiary'}>{draft.message ? problem : ''}</Text>
            {/* CD-46: code points, not UTF-16 units — and the field allows twice the units so a
              valid 2000-code-point astral message is never truncated by the input itself. */}
            <Text variant="caption" tone="tertiary">{Array.from(draft.message.trim()).length} / {capabilities.maxMessageLength}</Text>
          </div>
          {capabilities.diagnosticsSupported && category?.wantsPlaybackSession && playbackSessionId ? (
            <Checkbox checked={draft.attachDiagnostics} disabled={!capabilities.diagnosticsOptional} onCheckedChange={v => setDraft(d => ({...d, attachDiagnostics: v === true}))} label={t('feedback.playbackDiagnostics')} help={t('feedback.playbackDiagnosticsHelp')} />
          ) : null}
          <Text as="p" variant="caption" tone="tertiary">Sent as {capabilities.reporterName} to this server’s owner.</Text>
        </div>
      ) : null}
    </Dialog>
  );
}
