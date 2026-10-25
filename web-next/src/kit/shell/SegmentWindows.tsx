import {useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent} from 'react';
import {createI18n} from '@i18n';
import type {FactPart} from '@core/presentation/kit-model';
import {Icon} from '../Icon';
import {Button, Select} from '../controls';
import {BottomSheet} from '../overlays/BottomSheet';
import {usePhone} from '../overlays/shared';
import {readSegmentClock, segmentClock, zoomedSegmentRange} from './model';
import type {ShellTranslate} from './ServerIdentity';
import styles from './segments.module.css';

const tDefault = createI18n().t;
export type SegmentMark = Readonly<{id: string; kind: 'intro' | 'recap' | 'credits'; label: string; start: number; end: number; suggested: boolean; trust: string; trustTone: 'neutral' | 'automatic' | 'suggested'; facts: readonly FactPart[]}>;
export type SegmentKind = 'intro' | 'recap' | 'credits' | 'commercial' | 'outro';
/** The server's own marker kinds, in the drawn order. */
export const segmentEditKinds: readonly SegmentKind[] = ['intro', 'recap', 'credits', 'commercial', 'outro'];
export type SegmentEdit = Readonly<{id: string; opening: number; start: number; end: number; kind: SegmentKind}>;
export type SegmentWindowsProps = {
  duration: number; opening: readonly SegmentMark[]; ending: readonly SegmentMark[]; edit?: SegmentEdit;
  onEdit: (mark: SegmentMark) => void; onChange: (draft: SegmentEdit) => void; onSave: (draft: SegmentEdit) => void; onCancel: (opening: number) => void;
  onPreview: (mark: SegmentMark, start: number) => void; onApprove: (mark: SegmentMark) => void; t?: ShellTranslate;
};
export function SegmentWindows(props: SegmentWindowsProps) {
  const t = props.t ?? tDefault, phone = usePhone();
  return <><div className={styles.windows}><SegmentWindow {...props} phone={phone} marks={props.opening} endWindow={false} title={t('next.shell.opening')} /><SegmentWindow {...props} phone={phone} marks={props.ending} endWindow title={t('next.shell.end')} /></div>
    {phone && props.edit ? <AdjustSheet key={props.edit.opening} {...props} t={t} /> : null}</>;
}
function SegmentWindow({marks, duration, endWindow, title, edit, onChange, onEdit, onSave, onCancel, onPreview, onApprove, phone, t = tDefault}: SegmentWindowsProps & {marks: readonly SegmentMark[]; endWindow: boolean; title: string; phone: boolean}) {
  const [validStart, setValidStart] = useState(true), [validEnd, setValidEnd] = useState(true);
  const range = zoomedSegmentRange(marks, duration, endWindow);
  if (!range) return null;
  const percent = (value: number) => (value - range.start) / (range.end - range.start) * 100;
  return <section className={styles.window}><div className={styles.windowLabel}><b>{title}</b><span>{t('next.shell.timeRange', {start: segmentClock(range.start), end: segmentClock(range.end)})}</span></div>
    <div className={styles.timeline}>{marks.map(mark => {
      const editing = edit?.id === mark.id, start = editing ? edit.start : mark.start, end = editing ? edit.end : mark.end;
      return <span key={mark.id}><span className={styles.segment} data-kind={mark.kind} data-suggested={mark.suggested && !editing || undefined} data-editing={editing || undefined} style={{left: `${percent(start)}%`, width: `${percent(end) - percent(start)}%`}} />{editing && !phone && <><SegmentHandle label={t('next.shell.segmentStart', {kind: mark.label})} value={start} min={0} max={end - .1} range={range} onChange={value => onChange({...edit, start: value})} /><SegmentHandle label={t('next.shell.segmentEnd', {kind: mark.label})} value={end} min={start + .1} max={duration} range={range} onChange={value => onChange({...edit, end: value})} /></>}</span>;
    })}</div>
    {marks.map(mark => edit && edit.id === mark.id && !phone ? <div className={styles.marker} key={mark.id}><div className={styles.markerLine}><b>{mark.label}</b><span>{t('next.shell.dragOrType')}</span></div><div className={styles.editRow}><TimeField label={t('next.shell.startsAt')} onValid={setValidStart} value={edit.start} min={0} max={edit.end - .1} onValue={value => onChange({...edit, start: value})} /><TimeField label={t('next.shell.endsAt')} onValid={setValidEnd} value={edit.end} min={edit.start + .1} max={duration} onValue={value => onChange({...edit, end: value})} /><button type="button" className={styles.save} disabled={!validStart || !validEnd} onClick={() => onSave(edit)}>{t('next.shell.save')}</button><button type="button" className={styles.cancel} onClick={() => onCancel(edit.opening)}>{t('next.shell.cancel')}</button></div></div> : <div className={styles.marker} key={mark.id}><div className={styles.markerLine}><b>{mark.label}</b><span>{t('next.shell.timeRange', {start: segmentClock(mark.start), end: segmentClock(mark.end)})}</span><span className={styles.markerActions}><button type="button" aria-label={t('next.shell.previewSegment')} onClick={() => onPreview(mark, Math.max(0, mark.start - 3))}><Icon name="i-play" size={14} /></button>{mark.suggested && <button type="button" aria-label={t('next.shell.approveSegment', {kind: mark.label})} onClick={() => onApprove(mark)}><Icon name="i-check" size={17} /></button>}<button type="button" aria-label={t('next.shell.editSegment', {kind: mark.label})} onClick={() => onEdit(mark)}><Icon name="i-edit" size={16} /></button></span></div><div className={styles.markerFacts}><span className={styles.trust} data-tone={mark.trustTone}>{mark.trust}</span>{mark.facts.map(fact => <span key={fact.id}>{fact.text}</span>)}</div></div>)}
  </section>;
}
/** owner-libraries#23b/#25: at phone size the adjust step is one sheet with the
 * kind, one zoomed window with both handles, and the two times as steppers whose
 * buttons repeat while held. One lease is created per opening in a layout effect
 * and published in state; every handler closes over the exact rendered lease and
 * refuses once that lease is retired or its opening has changed. */
type AdjustLease = {
  readonly opening: number;
  draft: SegmentEdit;
  valid: {start: boolean; end: boolean};
  duration: number;
  onChange: (draft: SegmentEdit) => void;
  onSave: (draft: SegmentEdit) => void;
  onCancel: (opening: number) => void;
};

function AdjustSheet({opening, ending, duration, edit, onChange, onSave, onCancel, t = tDefault}: SegmentWindowsProps & {t?: ShellTranslate}) {
  const live = useRef<AdjustLease | undefined>(undefined);
  const [lease, setLease] = useState<AdjustLease | undefined>(undefined);
  const token = edit?.opening;
  useLayoutEffect(() => {
    if (token === undefined) {live.current = undefined;setLease(undefined);return;}
    const next: AdjustLease = {opening: token, draft: edit!, valid: {start: true, end: true}, duration, onChange, onSave, onCancel};
    live.current = next;setLease(next);
    return () => {if (live.current === next) live.current = undefined;};
  }, [token]);
  // Same lease only: the parent's committed draft and callbacks replace this
  // render's, never a successor opening's.
  useLayoutEffect(() => {
    const current = live.current;
    if (!edit || !current || current.opening !== edit.opening) return;
    current.draft = edit;current.duration = duration;current.onChange = onChange;current.onSave = onSave;current.onCancel = onCancel;
  });
  const [valid, setValid] = useState<{start: boolean; end: boolean}>({start: true, end: true});
  const sameOpening = () => !!lease && live.current === lease && !!edit && lease.opening === edit.opening;
  const apply = (field: 'start' | 'end', value: number) => {
    if (!sameOpening() || !lease || !Number.isFinite(value)) return;
    const draft = lease.draft;
    const min = field === 'start' ? 0 : draft.start + .1;
    const max = field === 'start' ? draft.end - .1 : lease.duration;
    const next = Math.round(Math.max(min, Math.min(max, value)) * 10) / 10;
    if (next === draft[field]) return;
    lease.draft = Object.freeze({...draft, [field]: next});
    lease.onChange(lease.draft);
  };
  const step = (field: 'start' | 'end', delta: number) => {
    if (!sameOpening() || !lease) return;
    const draft = lease.draft;
    const min = field === 'start' ? 0 : draft.start + .1;
    const max = field === 'start' ? draft.end - .1 : lease.duration;
    const current = draft[field];
    apply(field, Math.round(Math.max(min, Math.min(max, current + delta)) * 10) / 10);
  };
  const chooseKind = (value: string) => {
    const kind = segmentEditKinds.find(candidate => String(candidate) === value);
    if (!kind || !sameOpening() || !lease) return;
    lease.draft = Object.freeze({...lease.draft, kind});
    lease.onChange(lease.draft);
  };
  const save = () => {
    if (!sameOpening() || !lease || !lease.valid.start || !lease.valid.end) return;
    lease.onSave(lease.draft);
  };
  const setFieldValid = (field: 'start' | 'end', value: boolean) => {
    setValid(current => current[field] === value ? current : {...current, [field]: value});
    if (!sameOpening() || !lease) return;
    lease.valid = {...lease.valid, [field]: value};
  };
  const cancel = () => {if (sameOpening() && lease) lease.onCancel(lease.opening);};
  const mark = edit ? [...opening, ...ending].find(value => value.id === edit.id) : undefined;
  if (!edit || !mark) return null;
  const range = zoomedSegmentRange([{start: edit.start, end: edit.end}], duration, edit.kind === 'credits' || edit.kind === 'outro')
    ?? zoomedSegmentRange([mark], duration, false) ?? {start: 0, end: Math.max(duration, 1)};
  const percent = (value: number) => (value - range.start) / (range.end - range.start) * 100;
  return <BottomSheet open onOpenChange={open => {if (!open) cancel();}} title={t('web.segments.adjustTitle')}
    footer={<><Button variant="glass" size="compact" onClick={cancel}>{t('next.shell.cancel')}</Button><Button variant="primary" size="compact" disabled={!valid.start || !valid.end} onClick={save}>{t('next.shell.save')}</Button></>}>
    <div className={styles.adjustSheet}>
      <Select label={t('web.segments.kindLabel')} value={edit.kind} options={segmentEditKinds.map(kind => ({value: kind, label: t('web.segments.kind', {kind})}))} onValueChange={chooseKind} />
      <div className={styles.timeline}>
        <span className={styles.segment} data-kind={edit.kind} data-editing style={{left: `${percent(edit.start)}%`, width: `${percent(edit.end) - percent(edit.start)}%`}} />
        <SegmentHandle label={t('next.shell.segmentStart', {kind: mark.label})} value={edit.start} min={0} max={edit.end - .1} range={range} onChange={value => apply('start', value)} />
        <SegmentHandle label={t('next.shell.segmentEnd', {kind: mark.label})} value={edit.end} min={edit.start + .1} max={duration} range={range} onChange={value => apply('end', value)} />
      </div>
      <TimeStepper field="start" label={t('next.shell.startsAt')} value={edit.start} min={0} max={edit.end - .1} t={t}
        onStep={delta => step('start', delta)} onCommit={value => apply('start', value)}
        onValid={value => setFieldValid('start', value)} />
      <TimeStepper field="end" label={t('next.shell.endsAt')} value={edit.end} min={edit.start + .1} max={duration} t={t}
        onStep={delta => step('end', delta)} onCommit={value => apply('end', value)}
        onValid={value => setFieldValid('end', value)} />
    </div>
  </BottomSheet>;
}

/** One second per press, repeated while held; the field still takes typed times.
 * Pointer and keyboard activation are separate, and every release path stops it. */
function TimeStepper({label, value, min, max, onStep, onCommit, onValid, t = tDefault}: {
  field: 'start' | 'end'; label: string; value: number; min: number; max: number;
  onStep: (delta: number) => void; onCommit: (value: number) => void; onValid: (valid: boolean) => void; t?: ShellTranslate;
}) {
  const delay = useRef<number | undefined>(undefined), repeat = useRef<number | undefined>(undefined);
  const stop = () => {window.clearTimeout(delay.current);window.clearInterval(repeat.current);delay.current = repeat.current = undefined;};
  useLayoutEffect(() => {
    window.addEventListener('blur', stop);
    return () => {window.removeEventListener('blur', stop);stop();};
  }, []);
  const begin = (event: PointerEvent<HTMLButtonElement>, delta: number) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    stop();onStep(delta);
    delay.current = window.setTimeout(() => {repeat.current = window.setInterval(() => onStep(delta), 80);}, 400);
  };
  const button = (delta: -1 | 1) => <button type="button" className={styles.stepButton} aria-label={delta < 0 ? t('next.shell.oneSecondEarlier') : t('next.shell.oneSecondLater')}
    onPointerDown={event => begin(event, delta)} onPointerUp={stop} onPointerCancel={stop} onLostPointerCapture={stop}
    onClick={event => {if (event.detail === 0) onStep(delta);}}>{delta < 0 ? '−' : '+'}</button>;
  return <div className={styles.stepper}><span className={styles.stepperLabel}>{label}</span><div className={styles.stepperRow}>{button(-1)}
    <TimeField label={label} value={value} min={min} max={max} onValue={onCommit} onValid={onValid} />{button(1)}</div></div>;
}

function SegmentHandle({label, value, min, max, range, onChange}: {label: string; value: number; min: number; max: number; range: {start: number; end: number}; onChange: (value: number) => void}) {
  const pointer = useRef<number | undefined>(undefined);
  const update = (value: number) => onChange(Math.round(Math.max(min, Math.min(max, value)) * 10) / 10);
  const move = (event: PointerEvent<HTMLButtonElement>) => { if (pointer.current !== event.pointerId) return; const rect = event.currentTarget.parentElement?.parentElement?.getBoundingClientRect(); if (rect?.width) update(range.start + (event.clientX - rect.left) / rect.width * (range.end - range.start)); };
  const key = (event: KeyboardEvent<HTMLButtonElement>) => { const step = event.shiftKey ? .1 : 1; const next = event.key === 'ArrowLeft' || event.key === 'ArrowDown' ? value - step : event.key === 'ArrowRight' || event.key === 'ArrowUp' ? value + step : event.key === 'Home' ? min : event.key === 'End' ? max : undefined; if (next !== undefined) { event.preventDefault(); update(next); } };
  return <button type="button" role="slider" className={styles.handle} aria-label={label} aria-valuemin={min} aria-valuemax={max} aria-valuenow={value} aria-valuetext={segmentClock(value, true)} style={{left: `${(value - range.start) / (range.end - range.start) * 100}%`}} onKeyDown={key}
    onPointerDown={event => { if (event.button !== 0) return; pointer.current = event.pointerId; event.currentTarget.setPointerCapture(event.pointerId); event.preventDefault(); }} onPointerMove={move} onPointerUp={event => { if (pointer.current === event.pointerId) { move(event); pointer.current = undefined; event.currentTarget.releasePointerCapture(event.pointerId); } }} onPointerCancel={() => { pointer.current = undefined; }} />;
}
function TimeField({label, value, min, max, onValue, onValid}: {label: string; value: number; min: number; max: number; onValue: (value: number) => void; onValid: (valid: boolean) => void}) {
  const [typed, setTyped] = useState<string | undefined>(undefined);
  const parsed = typed === undefined ? value : readSegmentClock(typed), valid = parsed !== undefined && parsed >= min && parsed <= max;
  useLayoutEffect(() => onValid(valid), [valid, onValid]);
  return <label className={styles.timeField}>{label}<input value={typed ?? segmentClock(value, true)} inputMode="decimal" aria-invalid={!valid || undefined} onChange={event => setTyped(event.currentTarget.value)} onBlur={() => { if (valid && parsed !== undefined) { onValue(parsed); setTyped(undefined); } }} onKeyDown={event => { if (event.key === 'Enter' && valid && parsed !== undefined) { onValue(parsed); setTyped(undefined); } if (event.key === 'Escape') setTyped(undefined); }} /></label>;
}
