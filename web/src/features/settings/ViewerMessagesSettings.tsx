import { useCallback, useEffect, useRef, useState } from 'react';
import { productMessage, type ProductMessagePresentation, type ServerOwnerFeedbackPage, type ServerOwnerFeedbackRecord, type ServerOwnerNotificationRecipientDirectory, type ServerOwnerNoticeRequest, type ViewerFeedbackKind } from '@porticomediaserver/client-core';
import type { PorticoDataSource } from '../../data/models';
import { PrimaryButton } from '../../components/controls/Buttons';
import { ProductMessageIcon, SemanticProductIcon, productProblem, productText } from '../../components/ProductLanguage';
import { InlineNotice, SettingsGroup, SettingsLoading } from './SettingsControls';

type FeedbackStatus = 'new' | 'read' | 'resolved' | 'dismissed';
const SETTINGS_REQUEST_DEADLINE_MS = 15_000;

function boundedSignal(controller: AbortController) {
  return AbortSignal.any([controller.signal, AbortSignal.timeout(SETTINGS_REQUEST_DEADLINE_MS)]);
}

function statusLabel(status: FeedbackStatus) {
  return productText(`feedback.status.${status}`);
}

function kindLabel(kind: ViewerFeedbackKind) {
  return productText(`feedback.kind.${kind}`);
}

function categoryLabel(category: string) {
  const known = new Set(['wont-play', 'buffering', 'playback-stopped', 'wrong-video', 'wrong-audio', 'wrong-subtitles', 'incorrect-media-information', 'higher-quality-request']);
  if (!known.has(category)) return productText('feedback.category.other');
  return productText(`feedback.category.${category}` as Parameters<typeof productText>[0]);
}

function feedbackAccountName(reporter: ServerOwnerFeedbackRecord['reporter']): string {
  return reporter.accountName;
}

export function ViewerMessagesSettings({ source }: { source: PorticoDataSource }) {
  const [filter, setFilter] = useState<FeedbackStatus | 'all'>('new');
  const [page, setPage] = useState<ServerOwnerFeedbackPage>();
  const [selected, setSelected] = useState<ServerOwnerFeedbackRecord>();
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');
  const [error, setError] = useState<ProductMessagePresentation>();
  const [notice, setNotice] = useState<ProductMessagePresentation>();
  const [response, setResponse] = useState('');
  const [busy, setBusy] = useState(false);
  const [recipientDirectory, setRecipientDirectory] = useState<ServerOwnerNotificationRecipientDirectory>();
  const [recipientStatus, setRecipientStatus] = useState<'loading' | 'ready' | 'error'>('loading');
  const [recipientError, setRecipientError] = useState<ProductMessagePresentation>();
  const [outbound, setOutbound] = useState({ audience: 'profile' as 'profile' | 'account-admin', accountId: '', profileId: '', message: '', severity: 'informational' as 'informational' | 'warning' | 'error' });
  const feedbackController = useRef<AbortController | undefined>(undefined);
  const recipientsController = useRef<AbortController | undefined>(undefined);
  const mutationController = useRef<AbortController | undefined>(undefined);

  const load = useCallback(async (cursor?: string) => {
    feedbackController.current?.abort();
    const controller = new AbortController();
    feedbackController.current = controller;
    setStatus('loading');
    try {
      const next = await source.ownerViewerFeedback(filter === 'all' ? undefined : filter, cursor, boundedSignal(controller));
      if (controller.signal.aborted) return;
      setPage((currentPage) => {
        const reconciled = cursor && currentPage
          ? { ...next, items: [...currentPage.items, ...next.items] }
          : next;
        setSelected((currentSelection) => reconciled.items.find((item) => item.id === currentSelection?.id) ?? reconciled.items[0]);
        return reconciled;
      });
      setStatus('ready'); setError(undefined);
    } catch (reason) {
      if (!controller.signal.aborted) { setStatus('error'); setError(productProblem(reason)); }
    } finally {
      if (feedbackController.current === controller) feedbackController.current = undefined;
    }
  }, [filter, source]);

  const loadRecipients = useCallback(async () => {
    recipientsController.current?.abort();
    const controller = new AbortController();
    recipientsController.current = controller;
    setRecipientStatus('loading');
    try {
      const directory = await source.ownerViewerNotificationRecipients(boundedSignal(controller));
      if (controller.signal.aborted) return;
      setRecipientDirectory(directory);
      setRecipientError(undefined);
      setRecipientStatus('ready');
      setOutbound((current) => {
        const audience = directory.profiles.length === 0 && directory.accountAdmins.length > 0 ? 'account-admin'
          : directory.accountAdmins.length === 0 && directory.profiles.length > 0 ? 'profile'
          : current.audience;
        return {
          ...current,
          audience,
          profileId: audience === 'profile' && directory.profiles.some((recipient) => recipient.profileId === current.profileId) ? current.profileId : '',
          accountId: audience === 'account-admin' && directory.accountAdmins.some((recipient) => recipient.accountId === current.accountId) ? current.accountId : '',
        };
      });
    } catch (reason) {
      if (!controller.signal.aborted) {
        setRecipientStatus('error');
        setRecipientError(productProblem(reason));
      }
    } finally {
      if (recipientsController.current === controller) recipientsController.current = undefined;
    }
  }, [source]);

  useEffect(() => {
    void load();
    return () => feedbackController.current?.abort();
  }, [load]);
  useEffect(() => {
    void loadRecipients();
    return () => recipientsController.current?.abort();
  }, [loadRecipients]);
  useEffect(() => () => mutationController.current?.abort(), []);
  useEffect(() => setResponse(selected?.ownerResponse?.message ?? ''), [selected]);

  const update = async (nextStatus?: FeedbackStatus) => {
    if (!selected) return;
    mutationController.current?.abort();
    const controller = new AbortController();
    mutationController.current = controller;
    setBusy(true); setError(undefined); setNotice(undefined);
    try {
      const changedResponse = response.trim() && response.trim() !== selected.ownerResponse?.message ? response.trim() : undefined;
      const updated = await source.updateOwnerViewerFeedback(selected.id, { version: 'v1', expectedRevision: selected.revision, status: nextStatus, responseMessage: changedResponse }, boundedSignal(controller));
      if (controller.signal.aborted) return;
      setSelected(updated); setNotice(productMessage(response.trim() ? 'feedback.response-sent' : 'feedback.status-updated')); setResponse(updated.ownerResponse?.message ?? ''); await load();
    } catch (reason) { if (!controller.signal.aborted) setError(productProblem(reason)); }
    finally { if (mutationController.current === controller) { mutationController.current = undefined; setBusy(false); } }
  };

  const sendNotice = async () => {
    mutationController.current?.abort();
    const controller = new AbortController();
    mutationController.current = controller;
    setBusy(true); setError(undefined); setNotice(undefined);
    try {
      const request: ServerOwnerNoticeRequest = outbound.audience === 'profile'
        ? { audience: 'profile', profileId: outbound.profileId, message: outbound.message.trim(), severity: outbound.severity }
        : { audience: 'account-admin', accountId: outbound.accountId, message: outbound.message.trim(), severity: outbound.severity };
      await source.createOwnerViewerNotice(request, boundedSignal(controller));
      if (controller.signal.aborted) return;
      setOutbound({ ...outbound, message: '' }); setNotice(productMessage('notification.notice-sent'));
    } catch (reason) { if (!controller.signal.aborted) setError(productProblem(reason)); }
    finally { if (mutationController.current === controller) { mutationController.current = undefined; setBusy(false); } }
  };

  return <div className="portico-settings-content viewer-messages-settings">
    {(error || notice) && <InlineNotice tone={error ? 'error' : 'success'}><strong>{(error ?? notice)?.title}</strong>{(error ?? notice)?.body && <> {(error ?? notice)?.body}</>}</InlineNotice>}
    <SettingsGroup title={productText('feedback.viewer-messages-title')} description={productText('feedback.viewer-messages-description')}>
      <div className="viewer-message-filters" role="tablist" aria-label={productText('feedback.viewer-messages-title')}>{(['new', 'read', 'resolved', 'dismissed', 'all'] as const).map((value) => <button key={value} role="tab" aria-selected={filter === value} className={filter === value ? 'active' : ''} onClick={() => setFilter(value)}>{value === 'all' ? productText('feedback.status.all') : statusLabel(value)}{value !== 'all' && page && <span>{page.statusCounts[value]}</span>}</button>)}</div>
      {status === 'loading' && !page ? <SettingsLoading label={productText('feedback.loading')} /> : status === 'error' && !page ? <div className="portico-settings-state error"><ProductMessageIcon presentation={error ?? productMessage('feedback.load-failed')} /><strong>{error?.title ?? productMessage('feedback.load-failed').title}</strong><p>{error?.body ?? productMessage('feedback.load-failed').body}</p><button className="button secondary" onClick={() => void load()}><SemanticProductIcon id="action.retry" /> {productText('action.retry')}</button></div> : !page?.items.length ? (() => { const empty = filter === 'all' ? productMessage('feedback.empty') : productMessage('feedback.empty-filter', { status: statusLabel(filter).toLocaleLowerCase() }); return <div className="portico-settings-state"><ProductMessageIcon presentation={empty} /><strong>{empty.title}</strong><p>{empty.body}</p></div>; })() : <div className="viewer-message-workspace">
        <div className="viewer-message-list">{page.items.map((item) => <button type="button" key={item.id} className={selected?.id === item.id ? 'active' : ''} onClick={() => setSelected(item)}><span><strong>{feedbackAccountName(item.reporter)}</strong><small>{kindLabel(item.kind)} · {categoryLabel(item.category)}</small></span><time>{new Date(item.submittedAt).toLocaleDateString()}</time><SemanticProductIcon id="action.open" /></button>)}{page.pageInfo.hasMore && <button type="button" className="viewer-message-more" onClick={() => void load(page.pageInfo.nextCursor ?? undefined)}>{productText('action.load-more')}</button>}</div>
        {selected && <article className="viewer-message-detail"><header><span className="viewer-message-avatar"><SemanticProductIcon id="status.account" /></span><span><strong>{feedbackAccountName(selected.reporter)}</strong><small>{productText(selected.reporter.authority === 'hosted' ? 'feedback.reporter.hosted' : 'feedback.reporter.local')} · {new Date(selected.submittedAt).toLocaleString()}</small></span><b data-status={selected.status}>{statusLabel(selected.status)}</b></header><h3>{categoryLabel(selected.category)}</h3><p>{selected.message || productText('feedback.no-message')}</p><dl><div><dt>{productText('feedback.diagnostic.device')}</dt><dd>{selected.diagnostics.deviceClass} · {selected.diagnostics.platform}</dd></div>{selected.diagnostics.mediaId && <div><dt>{productText('feedback.diagnostic.media')}</dt><dd>{selected.diagnostics.mediaId}</dd></div>}{selected.diagnostics.deliveryReason && <div><dt>{productText('feedback.diagnostic.delivery')}</dt><dd>{selected.diagnostics.deliveryReason}</dd></div>}<div><dt>{productText('feedback.diagnostic.duplicates')}</dt><dd>{selected.duplicateCount}</dd></div></dl><label>{productText('feedback.private-response-label')}<textarea rows={4} maxLength={1000} value={response} onChange={(event) => setResponse(event.target.value)} placeholder={productText('feedback.private-response-placeholder')} /></label><footer><button className="button secondary" disabled={busy} onClick={() => void update('dismissed')}>{productText('action.dismiss')}</button><button className="button secondary" disabled={busy} onClick={() => void update('resolved')}><SemanticProductIcon id="action.resolve" /> {productText('action.resolve')}</button><button className="button primary" disabled={busy || !response.trim()} onClick={() => void update(selected.status === 'new' ? 'read' : selected.status)}><SemanticProductIcon id="action.message" /> {productText('action.respond')}</button></footer></article>}
      </div>}
    </SettingsGroup>
    <SettingsGroup title={productText('notification.owner-notice-title')} description={productText('notification.owner-notice-description')}>
      {recipientStatus === 'loading' ? <SettingsLoading label={productText('notification.recipients-loading')} /> : recipientStatus === 'error' ? <div className="portico-settings-state error" role="alert"><ProductMessageIcon presentation={recipientError ?? productMessage('notification.recipients-failed')} /><strong>{recipientError?.title ?? productMessage('notification.recipients-failed').title}</strong><p>{recipientError?.body ?? productMessage('notification.recipients-failed').body}</p><button type="button" className="button secondary" onClick={() => void loadRecipients()}><SemanticProductIcon id="action.retry" /> {productText('action.retry')}</button></div> : !recipientDirectory || (recipientDirectory.profiles.length === 0 && recipientDirectory.accountAdmins.length === 0) ? (() => { const empty = productMessage('notification.recipients-empty'); return <div className="portico-settings-state"><ProductMessageIcon presentation={empty} /><strong>{empty.title}</strong><p>{empty.body}</p></div>; })() : <form className="owner-notice-form" onSubmit={(event) => { event.preventDefault(); void sendNotice(); }}>
        <label>{productText('notification.notice.audience-label')}<select value={outbound.audience} onChange={(event) => setOutbound({ ...outbound, audience: event.target.value as typeof outbound.audience, accountId: '', profileId: '' })}>{recipientDirectory.profiles.length > 0 && <option value="profile">{productText('notification.notice.audience-profile')}</option>}{recipientDirectory.accountAdmins.length > 0 && <option value="account-admin">{productText('notification.notice.audience-account-admin')}</option>}</select></label>
        {outbound.audience === 'profile' ? <label>{productText('notification.notice.profile-label')}<select required value={outbound.profileId} onChange={(event) => setOutbound({ ...outbound, profileId: event.target.value })}><option value="">{productText('notification.notice.choose-profile')}</option>{recipientDirectory.profiles.map((profile) => <option key={profile.profileId} value={profile.profileId}>{profile.profileName} · {profile.accountName}</option>)}</select></label> : <label>{productText('notification.notice.account-label')}<select required value={outbound.accountId} onChange={(event) => setOutbound({ ...outbound, accountId: event.target.value })}><option value="">{productText('notification.notice.choose-account')}</option>{recipientDirectory.accountAdmins.map((account) => <option key={account.accountId} value={account.accountId}>{account.accountName} · {productText(account.authority === 'hosted' ? 'notification.account-authority.hosted' : 'notification.account-authority.local')}</option>)}</select></label>}
        <label>{productText('notification.notice.severity-label')}<select value={outbound.severity} onChange={(event) => setOutbound({ ...outbound, severity: event.target.value as typeof outbound.severity })}><option value="informational">{productText('notification.notice.severity-information')}</option><option value="warning">{productText('notification.notice.severity-warning')}</option><option value="error">{productText('notification.notice.severity-error')}</option></select></label>
        <label className="owner-notice-message">{productText('notification.notice.message-label')}<textarea required rows={4} maxLength={1000} value={outbound.message} onChange={(event) => setOutbound({ ...outbound, message: event.target.value })} /></label>
        <p className="feedback-privacy">{productText('notification.owner-notice-privacy')}</p>
        <PrimaryButton type="submit" disabled={busy || !outbound.message.trim() || (outbound.audience === 'profile' ? !outbound.profileId : !outbound.accountId)}><SemanticProductIcon id="action.message" /> {productText('action.send-notice')}</PrimaryButton>
      </form>}
    </SettingsGroup>
  </div>;
}
