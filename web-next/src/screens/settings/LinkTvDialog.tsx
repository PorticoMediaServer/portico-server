import {useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Dialog} from '../../kit/overlays';
import type {PlayOnLinkPreview} from '../../platform/playback/play-on';
import {watchLinkTv} from '../watch/receivers';
import {LinkTvForm} from './LinkTvForm';

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
    <LinkTvForm code={code} phase={phase} preview={preview}
      onCodeChange={value => { setCode(value.toUpperCase()); if (phase === 'failed') setPhase('code'); }}
      onReview={() => { void look(); }}
      onDecide={decision => { void decide(decision); }} />
  </Dialog>;
}
