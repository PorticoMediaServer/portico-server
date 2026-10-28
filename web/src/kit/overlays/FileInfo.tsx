import {Fragment, useCallback, useLayoutEffect, useState} from 'react';
import type {FactPart, FileInfoModel, ManageAction} from '@core/presentation/kit-model';
import type {FileInfoDisc} from '@core/detail.ts';
import {durationText} from '@core/presentation/kit-model/text.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../controls/Button';
import {OwnerTag, ServerMark} from '../shell/ServerIdentity';
import {BottomSheet, type BottomSheetProps} from './BottomSheet';
import {Dialog} from './Dialog';
import type {OverlaySurfaceProps} from './types';
import styles from './file-info.module.css';

export type FileInfoAction = Readonly<{fileId?: string; action: ManageAction['id']}>;
export type FileInfoProps = Pick<OverlaySurfaceProps, 'open' | 'onOpenChange' | 'initialFocusRef' | 'returnFocusRef' | 'testID'> & {
  model: FileInfoModel;
  subject: string;
  /** Use the matching canonical fileInfoModel context. This component never projects metadata. */
  layout: 'desktop' | 'phone';
  /** An actual supplied playback selection, never inferred from list order or availability. */
  pickedFileId?: string;
  /** The owner's ordered disc titles from the same authorized read; absent otherwise. */
  disc?: FileInfoDisc;
  onManage: (action: FileInfoAction) => void;
  detent?: number;
  onDetentChange?: (index: number) => void;
  scrollRef?: BottomSheetProps['scrollRef'];
};
const t = defaultI18n.t;

/** Context arrays include their own punctuation; inline parts must stay adjacent. */
function Parts({parts}: {parts: readonly FactPart[]}) {
  return parts.map(part => <span key={part.id} data-part={part.id} data-role={part.role}>{part.text}</span>);
}

function FileInfoBody({model, layout, pickedFileId, disc, onManage}: Pick<FileInfoProps, 'model' | 'layout' | 'pickedFileId' | 'disc' | 'onManage'>) {
  const phone = layout === 'phone';
  return <div className={styles.files} data-layout={layout}>{model.files.map(file => {
    const rows = file.rows;
    const row = (id: string) => rows.find(value => value.id === id);
    const resolution = row('resolution');
    return <section className={styles.version} key={file.id} data-file-id={file.id}>
      <h3><span className={styles.versionBadge} data-picked={file.id === pickedFileId || undefined}>{file.display.heading.text}</span><span className={styles.summary}><Parts parts={file.display.summary} /></span></h3>
      <dl>{resolution && <><dt>{resolution.label}</dt><dd><Parts parts={resolution.parts} /></dd></>}
        {file.display.video.length > 0 && <><dt>{t('next.kit.video')}</dt><dd className={styles.technical}><Parts parts={file.display.video} /></dd></>}
        {file.display.audio.length > 0 && <><dt>{t('next.kit.audio')}</dt><dd>{file.display.audio.map(track => <div key={track.id} className={styles.track}><Parts parts={track.parts} /></div>)}</dd></>}
        {file.display.subtitles.length > 0 && <><dt>{t('next.kit.subtitles')}</dt><dd><Parts parts={file.display.subtitles} /></dd></>}
        {['size', 'length', 'frame-rate'].map(id => { const value = row(id); return value && <Fragment key={id}><dt>{value.label}</dt><dd><Parts parts={value.parts} /></dd></Fragment>; })}
      </dl>
    </section>;
  })}<p className={styles.note}>{t(phone ? 'next.overlays.fileInfoNote' : 'next.overlays.fileInfoDesktopNote')}</p>{disc && disc.titles.length > 0 && <section className={styles.owner} aria-label={t('next.overlays.discTitles')}>
    <div className={styles.ownerHeading}>{phone ? <ServerMark size={36} context="file-info-phone" /> : <ServerMark size={34} />}<div><h3>{t('next.overlays.discTitles')}{!phone && <OwnerTag />}</h3>{!phone && <small>{model.files.find(file => file.id === disc.fileId)?.display.heading.text}</small>}</div>{phone && <OwnerTag size="phone" />}</div>
    <dl className={styles.ownerLocations}>{disc.titles.map(title => <div key={title.number} className={styles.location}><span className={styles.versionBadge}>{t('next.overlays.discTitle', {number: title.number})}</span><span className={styles.path}>{durationText(title.durationMs / 1000, t) ?? ''}</span><span className={styles.path}>{t('next.overlays.discChapters', {count: title.chapters})}</span></div>)}</dl>
    <p className={styles.note}>{t('next.overlays.discTitlesNote')}</p>
  </section>}{model.owner && <section className={styles.owner} aria-label={t('next.shell.ownerOnly')}>
    <div className={styles.ownerHeading}>{phone ? <ServerMark size={36} context="file-info-phone" /> : <ServerMark size={34} />}<div><h3>{model.owner.heading}{!phone && <OwnerTag />}</h3>{!phone && <small>{t('next.overlays.membersNeverSee')}</small>}</div>{phone && <OwnerTag size="phone" />}</div>
    <div className={styles.ownerLocations}>{model.owner.locations.map(location => <div key={location.id} className={styles.location}><span className={styles.versionBadge}>{model.files.find(file => file.id === location.id)?.display.heading.text}</span><span className={styles.path}>{location.path}</span>{location.actions.map(action => <Button variant="text" size="small" icon="i-trash" iconSize={phone ? 16 : 17} className={styles.deleteVersion} key={action.id} onClick={() => onManage({fileId: location.id, action: action.id})}>{action.label}</Button>)}</div>)}
    <div className={styles.ownerActions}>{model.owner.actions.map(action => <Button variant="text" size="small" iconSize={phone ? 18 : 17} icon={action.id === 'split-versions' ? 'i-split' : 'i-merge'} key={action.id} onClick={() => onManage({action: action.id})}>{action.label}</Button>)}</div></div><p className={styles.note}>{t('next.overlays.deleteVersionNote')}</p>
  </section>}</div>;
}

/** The model supplies authorized facts/actions; this renderer only emits real callbacks. */
export function FileInfo({model, subject, layout, pickedFileId, disc, onManage, detent, onDetentChange, scrollRef, ...surface}: FileInfoProps) {
  const [scrollBody, setScrollBody] = useState<HTMLDivElement | null>(null), [scrolled, setScrolled] = useState(false);
  const bodyRef = useCallback((element: HTMLDivElement | null) => {
    setScrollBody(element);
    if (typeof scrollRef === 'function') return scrollRef(element);
    if (scrollRef) scrollRef.current = element;
  }, [scrollRef]);
  useLayoutEffect(() => {
    if (!scrollBody) return;
    const update = () => setScrolled(scrollBody.scrollTop > 0);
    update(); scrollBody.addEventListener('scroll', update, {passive: true});
    return () => scrollBody.removeEventListener('scroll', update);
  }, [scrollBody]);
  const body = <FileInfoBody model={model} layout={layout} pickedFileId={pickedFileId} disc={disc} onManage={onManage} />;
  if (layout === 'phone') return <BottomSheet {...surface} title={<>{model.heading}<small className={styles.subject}>{subject}</small></>} className={[styles.sheet, scrolled ? styles.scrolled : undefined].filter(Boolean).join(' ')} headerClassName={styles.sheetHeader} bodyClassName={styles.sheetBody} detents={[554, 740]} detent={detent} onDetentChange={onDetentChange} scrollRef={bodyRef}>{body}</BottomSheet>;
  return <Dialog {...surface} title={model.heading} description={<>{subject}<span className={styles.count}>{t('next.overlays.versionCount', {count: model.files.length})}</span></>} responsive={false} placement="right" width={560} className={styles.panel} headerClassName={styles.panelHeader} bodyClassName={styles.panelBody}>{body}</Dialog>;
}
