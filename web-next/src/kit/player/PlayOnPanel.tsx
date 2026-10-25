import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../Icon';
import {BottomSheet, Popover} from '../overlays';
import type {PlayOnLinkPreview, PlayOnLinkPort, PlayOnReceiver, PlayOnReceiversPort} from '../../platform/playback/play-on';
import type {CastPlayerTranslate, PlayOnPanelProps} from './types';
import styles from './cast-player.module.css';

const EMPTY = Object.freeze({phase: 'idle' as const, items: Object.freeze([]) as readonly PlayOnReceiver[]});
const noReceivers = () => EMPTY;
const PLATFORMS: Readonly<Record<string, string>> = {appletv: 'Apple TV', tvos: 'Apple TV', ios: 'iPhone', ipados: 'iPad', androidtv: 'Android TV', android: 'Android', web: 'Web'};

type LinkState = Readonly<{phase: 'closed' | 'code' | 'busy' | 'preview' | 'done' | 'failed'; code: string; preview?: PlayOnLinkPreview}>;
const closedLink: LinkState = {phase: 'closed', code: ''};

function Receivers({port, t, onSend, sending, failed}: {port: PlayOnReceiversPort; t: CastPlayerTranslate; onSend(id: string): void; sending?: string; failed?: string}) {
  const snapshot = useSyncExternalStore(port.subscribe, port.getSnapshot, noReceivers);
  useEffect(() => { port.refresh(); }, [port]);
  if (snapshot.items.length === 0) return null;
  return <>
    <p className={styles.menuHeading}>{t('next.cast.onThisNetwork')}</p>
    <div className={styles.deviceGroup}>{snapshot.items.map(receiver => <button key={receiver.id} type="button" role="menuitem" className={styles.deviceRow}
      disabled={sending !== undefined} onClick={() => onSend(receiver.id)}>
      <Icon name="i-tvdev" size={20} /><span>{receiver.name}{receiver.detail && <small>{receiver.detail}</small>}</span>{sending === receiver.id ? <small>{t('next.cast.receiverBusy')}</small> : <Icon name="i-right" size={18} />}
    </button>)}</div>
    {failed && <p className={styles.footnote} role="status">{t('next.cast.receiverFailed', {device: snapshot.items.find(receiver => receiver.id === failed)?.name ?? ''})}</p>}
  </>;
}

/** The code flow of one rendered opening only. Every completion is checked against the
 * opening it belongs to; the caller owns the single flight and the abort. */
function LinkTv({t, state, setState, onLookup, onApprove, onDeny, onClose}: {
  t: CastPlayerTranslate; state: LinkState; setState(next: LinkState): void;
  onLookup(code: string): void; onApprove(): void; onDeny(): void; onClose(): void;
}) {
  const busy = state.phase === 'busy';
  if (state.phase === 'closed') return null;
  if (state.phase === 'failed') return <div className={styles.linkForm}><p className={styles.footnote} role="status">{t('next.cast.linkTvFailed')}</p>
    <button type="button" className={styles.done} onClick={() => setState({phase: 'code', code: state.code})}>{t('next.cast.linkTvRetry')}</button></div>;
  if (state.phase === 'done') return <div className={styles.linkForm}><p className={styles.footnote} role="status">{t('next.cast.linkTvApproved')}</p>
    <button type="button" className={styles.done} onClick={onClose}>{t('next.controls.done')}</button></div>;
  if (state.preview) return <div className={styles.linkForm} role="group" aria-label={t('next.cast.linkTv')}>
    <p className={styles.footnote}><b>{t('next.cast.linkTvReview', {device: state.preview.deviceName})}</b>
      {state.preview.platform && <small>{PLATFORMS[state.preview.platform] ?? state.preview.platform}</small>}</p>
    <div className={styles.linkActions}><button type="button" className={styles.deviceRow} disabled={busy} onClick={onApprove}><Icon name="i-check" size={18} /><span>{t('next.cast.linkTvApprove')}</span></button>
      <button type="button" className={styles.deviceRow} disabled={busy} onClick={onDeny}><Icon name="i-x" size={18} /><span>{t('next.cast.linkTvDeny')}</span></button></div>
  </div>;
  return <form className={styles.linkForm} onSubmit={event => { event.preventDefault(); onLookup(state.code.trim().toUpperCase()); }}>
    <label className={styles.linkLabel} htmlFor="link-tv-code">{t('next.cast.linkTvCode')}</label>
    <div className={styles.linkRow}>
      <input id="link-tv-code" className={styles.linkInput} value={state.code} maxLength={10} autoComplete="off" autoCapitalize="characters" spellCheck={false}
        disabled={busy} onChange={event => setState({phase: 'code', code: event.currentTarget.value.toUpperCase()})} />
      <button type="submit" className={styles.done} disabled={busy}>{busy ? t('next.cast.linkTvConnecting') : t('next.cast.linkTvConnect')}</button>
    </div>
    <p className={styles.footnote}>{t('next.cast.linkTvHint')}</p>
  </form>;
}

/** Desktop f1s and the phone sheet, with the binding browser rows, the Portico televisions
 * the server names, and the code that signs a television in. */
export function PlayOnPanel({controller, snapshot, phone, anchorRef, reportError, receivers, linkTv, t = defaultI18n.t}: PlayOnPanelProps) {
  const open = snapshot.panel === 'play-on' && !snapshot.disposed;
  const [sending, setSending] = useState<string | undefined>(undefined), [failed, setFailed] = useState<string | undefined>(undefined);
  const [link, setLink] = useState<LinkState>(closedLink);
  // One rendered opening of the code flow: an older lookup, approval or denial can never
  // land on its successor. Closing retires the opening and aborts its request at once.
  const ticket = useRef(0), flight = useRef<AbortController | null>(null);
  const retire = useCallback(() => { ticket.current++; flight.current?.abort(); flight.current = null; setLink(closedLink); }, []);
  useEffect(() => { if (open) receivers?.refresh(); else { retire(); setSending(undefined); setFailed(undefined); } }, [open, receivers, retire]);
  useEffect(() => () => { flight.current?.abort(); }, []);
  if (!snapshot.cast.available && !snapshot.airPlayAvailable && !receivers) return null;
  const close = () => { retire(); controller.closePanel(); };
  const send = (id: string) => {
    if (!receivers) return;
    setSending(id); setFailed(undefined);
    void receivers.send(id).then(taken => { setSending(undefined); if (taken) close(); else setFailed(id); })
      .catch(error => { setSending(undefined); setFailed(id); reportError(error); });
  };
  const beginLink = () => { retire(); setLink({phase: 'code', code: ''}); };
  const previewLink = (code: string) => {
    if (!linkTv || flight.current) return;
    if (code.length < 4) { setLink({phase: 'failed', code}); return; }
    const mine = ticket.current, life = new AbortController();
    flight.current = life;
    setLink({phase: 'busy', code});
    void linkTv.preview(code, {signal: life.signal}).then(preview => {
      if (life.signal.aborted || ticket.current !== mine) return;
      setLink({phase: 'preview', code, preview});
    }).catch(() => {
      if (life.signal.aborted || ticket.current !== mine) return;
      setLink({phase: 'failed', code});
    }).finally(() => { if (flight.current === life) flight.current = null; });
  };
  const decide = (decision: 'approve' | 'deny') => {
    if (!linkTv || !link.preview || flight.current) return;
    const preview = link.preview, code = link.code.trim().toUpperCase(), mine = ticket.current, life = new AbortController();
    flight.current = life;
    setLink({phase: 'busy', code: link.code, preview});
    void linkTv.decide(preview.requestId, code, decision, {signal: life.signal}).then(() => {
      if (life.signal.aborted || ticket.current !== mine) return;
      if (decision === 'approve') { setLink({phase: 'done', code: link.code, preview}); return; }
      retire(); controller.closePanel();
    }).catch(error => {
      if (life.signal.aborted || ticket.current !== mine) return;
      setLink({phase: 'failed', code: link.code});
      reportError(error);
    }).finally(() => { if (flight.current === life) flight.current = null; });
  };
  const rows = <>
    <div className={styles.deviceGroup}>
      {snapshot.airPlayAvailable && <button type="button" role={phone ? undefined : 'menuitem'} className={styles.deviceRow} onClick={() => controller.startAirPlay()}>
        <Icon name="i-airplay" size={20} /><span>{t('playOn.airplay')}<small>{t('playOn.airplaySubtitle')}</small></span></button>}
      {snapshot.cast.available && <button type="button" role={phone ? undefined : 'menuitem'} className={styles.deviceRow} onClick={() => { void controller.startCast().catch(reportError); }}>
        <Icon name="i-cast" size={20} /><span>{t('playOn.googleCast')}<small>{t('next.cast.browserDevices')}</small></span></button>}
    </div>
    {receivers && <Receivers port={receivers} t={t} onSend={send} sending={sending} failed={failed} />}
    {linkTv && link.phase === 'closed' && <div className={styles.deviceGroup}>
      <button type="button" role={phone ? undefined : 'menuitem'} className={styles.deviceRow} onClick={beginLink}>
        <Icon name="i-tv" size={20} /><span>{t('next.cast.linkTv')}<small>{t('next.cast.linkTvHint')}</small></span><Icon name="i-right" size={18} /></button>
    </div>}
    {linkTv && <LinkTv t={t} state={link} setState={setLink} onLookup={previewLink} onApprove={() => decide('approve')} onDeny={() => decide('deny')} onClose={close} />}
    <div className={styles.separator} role="separator" /><p className={styles.footnote}>{t('next.cast.browserFootnote')}</p>
  </>;
  if (phone) return <BottomSheet open={open} onOpenChange={close} title={t('next.dock.playOnDevice')} fitContent returnFocusRef={anchorRef} showCloseButton={false}
    className={styles.playOnSheet} bodyClassName={styles.sheetBody}>
    <button type="button" className={styles.done} onClick={close}>{t('next.controls.done')}</button>{rows}
  </BottomSheet>;
  return <Popover open={open} onOpenChange={close} title={t('next.dock.playOnDevice')} anchorRef={anchorRef} returnFocusRef={anchorRef} width={300}
    role={link.phase === 'closed' ? 'menu' : 'dialog'} className={styles.playOnPanel} sideOffset={12} onContentKeyDown={event => {
      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
      const rows = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')];
      const at = rows.indexOf(document.activeElement as HTMLButtonElement);
      const target = event.key === 'Home' ? 0 : event.key === 'End' ? rows.length - 1 : (at + (event.key === 'ArrowUp' ? -1 : 1) + rows.length) % rows.length;
      event.preventDefault(); rows[target]?.focus();
    }}>
    <div className={styles.menuHeading}>{t('next.dock.playOnDevice')}</div>{rows}
  </Popover>;
}
