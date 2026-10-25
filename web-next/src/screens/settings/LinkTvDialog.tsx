import {useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Dialog} from '../../kit/overlays';
import {Button} from '../../kit/controls';
import type {PlayOnLinkPreview} from '../../platform/playback/play-on';
import {watchLinkTv} from '../watch/receivers';

const t = defaultI18n.t;

/** Settings › This device › Link a TV. The same real quick-connect services the Play on
 * sheet uses: the phone reads the code the TV shows, sees the device it reports, and
 * records its own approval. The TV collects its session in its own request. */
export function LinkTvDialog({open, onOpenChange, api, returnFocusRef}: {
  open: boolean; onOpenChange(open: boolean): void;
  api: Readonly<{request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>}>;
  returnFocusRef: React.RefObject<HTMLElement | null>;
}) {
  const link = useRef(watchLinkTv({request: (path, method, body, signal) => api.request(path, method, body, signal)}));
  const [code, setCode] = useState(''), [preview, setPreview] = useState<PlayOnLinkPreview | undefined>(undefined);
  const [phase, setPhase] = useState<'code' | 'busy' | 'review' | 'done' | 'failed'>('code');
  const reset = () => { setCode(''); setPreview(undefined); setPhase('code'); };
  const look = async () => {
    const value = code.trim().toUpperCase();
    if (value.length < 4) { setPhase('failed'); return; }
    setPhase('busy');
    try { setPreview(await link.current.preview(value)); setPhase('review'); }
    catch { setPreview(undefined); setPhase('failed'); }
  };
  const decide = async (decision: 'approve' | 'deny') => {
    if (!preview) return;
    setPhase('busy');
    try {
      await link.current.decide(preview.requestId, code.trim().toUpperCase(), decision);
      if (decision === 'approve') setPhase('done'); else { reset(); onOpenChange(false); }
    } catch { setPhase('failed'); }
  };
  return <Dialog open={open} onOpenChange={value => { if (!value) reset(); onOpenChange(value); }} title={t('next.cast.linkTv')} returnFocusRef={returnFocusRef}>
    {phase === 'done' ? <p role="status">{t('next.cast.linkTvApproved')}</p> : <>
      {preview ? <>
        <p><b>{t('next.cast.linkTvReview', {device: preview.deviceName})}</b>{preview.platform && <small>{preview.platform}</small>}</p>
        <div style={{display: 'flex', gap: 8, marginTop: 12}}>
          <Button size="small" disabled={phase === 'busy'} onClick={() => { void decide('approve'); }}>{t('next.cast.linkTvApprove')}</Button>
          <Button size="small" variant="glass" disabled={phase === 'busy'} onClick={() => { void decide('deny'); }}>{t('next.cast.linkTvDeny')}</Button>
        </div>
      </> : <>
        <label htmlFor="settings-link-tv">{t('next.cast.linkTvCode')}</label>
        <input id="settings-link-tv" value={code} maxLength={10} autoComplete="off" autoCapitalize="characters" spellCheck={false} disabled={phase === 'busy'}
          onChange={event => { setCode(event.currentTarget.value.toUpperCase()); if (phase === 'failed') setPhase('code'); }} />
        <p>{t('next.cast.linkTvHint')}</p>
        <Button size="small" disabled={phase === 'busy' || code.trim().length < 4} onClick={() => { void look(); }}>
          {phase === 'busy' ? t('next.cast.linkTvConnecting') : t('next.cast.linkTvConnect')}</Button>
      </>}
      {phase === 'failed' && <p role="status">{t('next.cast.linkTvFailed')}</p>}
    </>}
  </Dialog>;
}
