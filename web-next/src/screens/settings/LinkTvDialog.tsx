import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {Dialog} from '../../kit/overlays';
import type {PlayOnLinkPreview} from '../../platform/playback/play-on';
import {watchLinkTv} from '../watch/receivers';
import {LinkTvForm} from './LinkTvForm';

const t = defaultI18n.t;

type LinkApi = Readonly<{request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>}>;
/** One committed open of the dialog. Close, a new open, an api replacement or unmount retires
 * it; every preview/decision checks it is still that live opening before publishing anything. */
type Opening = {controller: AbortController; api: LinkApi; port: ReturnType<typeof watchLinkTv>; busy: boolean};

/** Settings › This device › Link a TV. The same real quick-connect services the Play on
 * sheet uses: the phone reads the code the TV shows, sees the device it reports, and
 * records its own approval. The TV collects its session in its own request. */
export function LinkTvDialog({open, onOpenChange, api, returnFocusRef}: {
  open: boolean; onOpenChange(open: boolean): void;
  api: LinkApi;
  returnFocusRef: React.RefObject<HTMLElement | null>;
}) {
  const opening = useRef<Opening | null>(null), [code, setCode] = useState(''), [preview, setPreview] = useState<PlayOnLinkPreview | undefined>(undefined);
  const [phase, setPhase] = useState<'code' | 'busy' | 'review' | 'done' | 'failed'>('code');
  const live = (held: Opening) => opening.current === held && !held.controller.signal.aborted;

  // The `open` prop is the commitment: each open gets one lifecycle owner, and closing it (from
  // the dialog, the parent, an api replacement, or unmount) retires that owner before any state
  // is reset. A new open always starts from clean code/preview/phase.
  useEffect(() => {
    const held = opening.current;
    if (open && held?.api === api) return;
    opening.current = null;
    held?.controller.abort();
    setCode(''); setPreview(undefined); setPhase('code');
    if (open) opening.current = {controller: new AbortController(), api, port: watchLinkTv({request: (path, method, body, signal) => api.request(path, method, body, signal)}), busy: false};
  }, [open, api]);
  useEffect(() => () => { const held = opening.current; opening.current = null; held?.controller.abort(); }, []);

  const clear = () => { setCode(''); setPreview(undefined); setPhase('code'); };
  const look = async () => {
    const held = opening.current;
    if (!held || held.busy) return;
    const value = code.trim().toUpperCase();
    if (value.length < 4) { if (live(held)) setPhase('failed'); return; }
    held.busy = true;
    if (live(held)) setPhase('busy');
    try {
      const reviewed = await held.port.preview(value, {signal: held.controller.signal});
      if (!live(held)) return;
      setPreview(reviewed);
      setPhase('review');
    } catch {
      if (live(held)) { setPreview(undefined); setPhase('failed'); }
    } finally { held.busy = false; }
  };
  const decide = async (decision: 'approve' | 'deny') => {
    const held = opening.current, reviewed = preview;
    if (!held || held.busy || !reviewed) return;
    held.busy = true;
    if (live(held)) setPhase('busy');
    try {
      await held.port.decide(reviewed.requestId, reviewed.userCode, decision, {signal: held.controller.signal});
      if (!live(held)) return;
      if (decision === 'approve') setPhase('done');
      else { opening.current = null; held.controller.abort(); clear(); onOpenChange(false); }
    } catch {
      if (live(held)) setPhase('failed');
    } finally { held.busy = false; }
  };
  const close = () => {
    const held = opening.current;
    opening.current = null;
    held?.controller.abort();
    clear();
    onOpenChange(false);
  };
  return <Dialog open={open} onOpenChange={value => { if (value) onOpenChange(true); else close(); }} title={t('next.cast.linkTv')} returnFocusRef={returnFocusRef}>
    <LinkTvForm code={code} phase={phase} preview={preview}
      onCodeChange={value => { const held = opening.current; if (!held || !live(held)) return; setCode(value.toUpperCase()); if (phase === 'failed') setPhase('code'); }}
      onReview={() => { void look(); }}
      onDecide={decision => { void decide(decision); }} />
  </Dialog>;
}
