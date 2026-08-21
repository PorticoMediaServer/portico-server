import { useEffect, useId, useState } from 'react';
import { productMessage, type ProductMessageId, type ProductMessagePresentation, type ViewerFeedbackCategory, type ViewerFeedbackKind } from '@porticomediaserver/client-core';
import { IconButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { ProductMessageIcon, SemanticProductIcon, productProblem, productText } from '../../components/ProductLanguage';
import { usePorticoDataSource } from '../../data/DataProvider';
import './feedback.css';

const categories: Record<ViewerFeedbackKind, Array<{ id: ViewerFeedbackCategory; labelId: ProductMessageId }>> = {
  general: [{ id: 'other', labelId: 'feedback.category.message-owner' }],
  playback: [
    { id: 'wont-play', labelId: 'feedback.category.wont-play' },
    { id: 'buffering', labelId: 'feedback.category.buffering' },
    { id: 'playback-stopped', labelId: 'feedback.category.playback-stopped' },
    { id: 'wrong-video', labelId: 'feedback.category.wrong-video' },
    { id: 'wrong-audio', labelId: 'feedback.category.wrong-audio' },
    { id: 'wrong-subtitles', labelId: 'feedback.category.wrong-subtitles' },
    { id: 'other', labelId: 'feedback.category.other' },
  ],
  media: [
    { id: 'incorrect-media-information', labelId: 'feedback.category.incorrect-media-information' },
    { id: 'wrong-video', labelId: 'feedback.category.wrong-video' },
    { id: 'wrong-audio', labelId: 'feedback.category.wrong-audio' },
    { id: 'wrong-subtitles', labelId: 'feedback.category.wrong-subtitles' },
    { id: 'other', labelId: 'feedback.category.other' },
  ],
  quality: [
    { id: 'higher-quality-request', labelId: 'feedback.category.higher-quality-request' },
    { id: 'other', labelId: 'feedback.category.quality-other' },
  ],
};

export type FeedbackDialogProps = {
  kind: ViewerFeedbackKind;
  mediaId?: string;
  playbackSessionId?: string;
  title?: string;
  onDismiss: () => void;
};

export function FeedbackDialog({ kind, mediaId, playbackSessionId, title, onDismiss }: FeedbackDialogProps) {
  const source = usePorticoDataSource();
  const headingId = useId();
  const [capabilities, setCapabilities] = useState<Awaited<ReturnType<typeof source.viewerFeedbackCapabilities>>>();
  const [category, setCategory] = useState<ViewerFeedbackCategory>(categories[kind][0].id);
  const [message, setMessage] = useState('');
  const [status, setStatus] = useState<'loading' | 'ready' | 'sending' | 'sent' | 'error'>('loading');
  const [error, setError] = useState<ProductMessagePresentation>();

  useEffect(() => {
    const controller = new AbortController();
    source.viewerFeedbackCapabilities(controller.signal).then((value) => {
      if (controller.signal.aborted) return;
      setCapabilities(value);
      setStatus('ready');
    }, (reason: unknown) => {
      if (controller.signal.aborted) return;
      setError(productProblem(reason));
      setStatus('error');
    });
    return () => controller.abort();
  }, [source]);

  const allowed = capabilities?.enabled && capabilities.allowedKinds.includes(kind);
  const submit = async () => {
    if (!allowed || !message.trim()) return;
    const controller = new AbortController();
    setStatus('sending');
    setError(undefined);
    try {
      await source.submitViewerFeedback({
        version: 'v1',
        kind,
        category,
        message: message.trim(),
        context: {
          mediaId,
          playbackSessionId,
          deviceClass: 'web',
          platform: 'web',
          appVersion: import.meta.env.VITE_APP_VERSION || 'web',
        },
      }, controller.signal);
      setStatus('sent');
    } catch (reason) {
      setError(productProblem(reason));
      setStatus('error');
    }
  };

  const heading = title ?? productText(kind === 'general' ? 'feedback.heading.message' : kind === 'quality' ? 'feedback.heading.quality' : 'feedback.heading.report');
  const checking = productMessage('feedback.checking');
  const sent = productMessage('feedback.sent');
  const unavailable = capabilities?.enabled === false ? productMessage('feedback.disabled') : productMessage('feedback.not-allowed');
  return <ModalOverlay labelledBy={headingId} className="feedback-dialog" onDismiss={() => { if (status !== 'sending') onDismiss(); }}>
    <header><div><p>{productText(kind === 'general' ? 'feedback.owner-context' : 'feedback.improvement-context')}</p><h2 id={headingId}>{heading}</h2></div><IconButton label={productText('feedback.close-label')} disabled={status === 'sending'} onClick={onDismiss}><SemanticProductIcon id="action.close" /></IconButton></header>
    {status === 'loading' ? <div className="feedback-state" aria-live="polite"><ProductMessageIcon presentation={checking} className="state-spinner" /><strong>{checking.title}</strong></div> : status === 'sent' ? <div className="feedback-state success" role="status"><ProductMessageIcon presentation={sent} /><strong>{sent.title}</strong><p>{sent.body}</p><button className="button primary" type="button" onClick={onDismiss}>{productText('action.done')}</button></div> : <>
      {!allowed ? <div className="feedback-state"><ProductMessageIcon presentation={error ?? unavailable} /><strong>{(error ?? unavailable).title}</strong><p>{(error ?? unavailable).body}</p></div> : <form onSubmit={(event) => { event.preventDefault(); void submit(); }}>
        {categories[kind].length > 1 && <label>{productText('feedback.what-happened')}<select value={category} onChange={(event) => setCategory(event.target.value as ViewerFeedbackCategory)}>{categories[kind].map((option) => <option key={option.id} value={option.id}>{productText(option.labelId)}</option>)}</select></label>}
        <label>{productText('feedback.message-label')}<textarea autoFocus rows={6} maxLength={capabilities?.messageMaxLength ?? 1000} value={message} onChange={(event) => setMessage(event.target.value)} placeholder={productText(kind === 'general' ? 'feedback.message-placeholder-general' : 'feedback.message-placeholder')} /></label>
        <div className="feedback-count">{message.length} / {capabilities?.messageMaxLength ?? 1000}</div>
        <p className="feedback-privacy">{productText('feedback.privacy', { retentionDays: capabilities?.retentionDays ?? 90 })}</p>
        {error && <p className="feedback-error" role="alert"><strong>{error.title}</strong>{error.body && <> {error.body}</>}</p>}
        <footer><button className="button secondary" type="button" disabled={status === 'sending'} onClick={onDismiss}>{productText('action.cancel')}</button><button className="button primary" type="submit" disabled={!message.trim() || status === 'sending'}><SemanticProductIcon id="action.message" />{productText(status === 'sending' ? 'action.sending-message' : 'action.send-message')}</button></footer>
      </form>}
    </>}
  </ModalOverlay>;
}
