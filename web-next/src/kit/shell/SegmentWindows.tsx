import {useEffect, useRef, useState, type KeyboardEvent, type PointerEvent} from 'react';
import {createI18n} from '@i18n';
import type {FactPart} from '@core/presentation/kit-model';
import {Icon} from '../Icon';
import {readSegmentClock, segmentClock, zoomedSegmentRange} from './model';
import type {ShellTranslate} from './ServerIdentity';
import styles from './segments.module.css';

const tDefault = createI18n().t;
export type SegmentMark = Readonly<{id: string; kind: 'intro' | 'recap' | 'credits'; label: string; start: number; end: number; suggested: boolean; trust: string; trustTone: 'neutral' | 'automatic' | 'suggested'; facts: readonly FactPart[]}>;
export type SegmentEdit = Readonly<{id: string; start: number; end: number}>;
export type SegmentWindowsProps = {
  duration: number; opening: readonly SegmentMark[]; ending: readonly SegmentMark[]; edit?: SegmentEdit;
  onEdit: (mark: SegmentMark) => void; onChange: (draft: SegmentEdit) => void; onSave: () => void; onCancel: () => void;
  onPreview: (mark: SegmentMark, start: number) => void; onApprove: (mark: SegmentMark) => void; t?: ShellTranslate;
};
export function SegmentWindows(props: SegmentWindowsProps) {
  const t = props.t ?? tDefault;
  return <div className={styles.windows}><SegmentWindow {...props} marks={props.opening} endWindow={false} title={t('next.shell.opening')} /><SegmentWindow {...props} marks={props.ending} endWindow title={t('next.shell.end')} /></div>;
}
function SegmentWindow({marks, duration, endWindow, title, edit, onChange, onEdit, onSave, onCancel, onPreview, onApprove, t = tDefault}: SegmentWindowsProps & {marks: readonly SegmentMark[]; endWindow: boolean; title: string}) {
  const [validStart, setValidStart] = useState(true), [validEnd, setValidEnd] = useState(true);
  const range = zoomedSegmentRange(marks, duration, endWindow);
  if (!range) return null;
  const percent = (value: number) => (value - range.start) / (range.end - range.start) * 100;
  return <section className={styles.window}><div className={styles.windowLabel}><b>{title}</b><span>{t('next.shell.timeRange', {start: segmentClock(range.start), end: segmentClock(range.end)})}</span></div>
    <div className={styles.timeline}>{marks.map(mark => {
      const editing = edit?.id === mark.id, start = editing ? edit.start : mark.start, end = editing ? edit.end : mark.end;
      return <span key={mark.id}><span className={styles.segment} data-kind={mark.kind} data-suggested={mark.suggested && !editing || undefined} data-editing={editing || undefined} style={{left: `${percent(start)}%`, width: `${percent(end) - percent(start)}%`}} />{editing && <><SegmentHandle label={t('next.shell.segmentStart', {kind: mark.label})} value={start} min={0} max={end - .1} range={range} onChange={value => onChange({...edit, start: value})} /><SegmentHandle label={t('next.shell.segmentEnd', {kind: mark.label})} value={end} min={start + .1} max={duration} range={range} onChange={value => onChange({...edit, end: value})} /></>}</span>;
    })}</div>
    {marks.map(mark => edit?.id === mark.id ? <div className={styles.marker} key={mark.id}><div className={styles.markerLine}><b>{mark.label}</b><span>{t('next.shell.dragOrType')}</span></div><div className={styles.editRow}><TimeField label={t('next.shell.startsAt')} onValid={setValidStart} value={edit.start} min={0} max={edit.end - .1} onValue={value => onChange({...edit, start: value})} /><TimeField label={t('next.shell.endsAt')} onValid={setValidEnd} value={edit.end} min={edit.start + .1} max={duration} onValue={value => onChange({...edit, end: value})} /><button type="button" className={styles.save} disabled={!validStart || !validEnd} onClick={onSave}>{t('next.shell.save')}</button><button type="button" className={styles.cancel} onClick={onCancel}>{t('next.shell.cancel')}</button></div></div> : <div className={styles.marker} key={mark.id}><div className={styles.markerLine}><b>{mark.label}</b><span>{t('next.shell.timeRange', {start: segmentClock(mark.start), end: segmentClock(mark.end)})}</span><span className={styles.markerActions}><button type="button" aria-label={t('next.shell.previewSegment')} onClick={() => onPreview(mark, Math.max(0, mark.start - 3))}><Icon name="i-play" size={14} /></button>{mark.suggested && <button type="button" aria-label={t('next.shell.approveSegment', {kind: mark.label})} onClick={() => onApprove(mark)}><Icon name="i-check" size={17} /></button>}<button type="button" aria-label={t('next.shell.editSegment', {kind: mark.label})} onClick={() => onEdit(mark)}><Icon name="i-edit" size={16} /></button></span></div><div className={styles.markerFacts}><span className={styles.trust} data-tone={mark.trustTone}>{mark.trust}</span>{mark.facts.map(fact => <span key={fact.id}>{fact.text}</span>)}</div></div>)}
  </section>;
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
  useEffect(() => onValid(valid), [valid, onValid]);
  return <label className={styles.timeField}>{label}<input value={typed ?? segmentClock(value, true)} inputMode="decimal" aria-invalid={!valid || undefined} onChange={event => setTyped(event.currentTarget.value)} onBlur={() => { if (valid && parsed !== undefined) { onValue(parsed); setTyped(undefined); } }} onKeyDown={event => { if (event.key === 'Enter' && valid && parsed !== undefined) { onValue(parsed); setTyped(undefined); } if (event.key === 'Escape') setTyped(undefined); }} /></label>;
}
