import {useRef, useState, type ImgHTMLAttributes, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {fileInfoModel, type FileFacts, type FileInfoModel} from '@portico/client-core/kit-model';
import {overlaysFixtures} from '../../../../packages/i18n/src/catalog/en-US/next/dev/overlays-fixtures';
import {defineGalleryFamily} from '../gallery-registry';
import {Button, Segmented} from '../../kit/controls';
import {Banner, BottomSheet, Dialog, Menu, MenuGroup, MenuItem, MenuRow, MenuSeparator, Popover, SidePanel, Toast, ToastHost, type OverlayDismissReason, type ToastData} from '../../kit/overlays';
import styles from '../../kit/overlays/gallery.module.css';

const t = defaultI18n.t;
// Reference-only data; this module is discovered exclusively by Vite's serve gallery.
const reference = {server: overlaysFixtures['next.overlays.dev.server'], film: overlaysFixtures['next.overlays.dev.film'], removedFilm: overlaysFixtures['next.overlays.dev.removedFilm'], programme: overlaysFixtures['next.overlays.dev.programme'], year: 2024};
const files: readonly FileFacts[] = [
  {id: 'version-1', available: true, width: 3840, height: 1608, videoCodec: 'hevc', dynamicRange: 'hdr10', bitDepth: 10, bitRate: 52_000_000, container: 'mkv', sizeBytes: 64.2 * 1024 ** 3, durationSeconds: 9960,
    audio: [{id: 'a1', language: 'en', codec: 'truehd', channels: 8, objectAudio: 'atmos'}, {id: 'a2', language: 'en', codec: 'ac3', channels: 6}, {id: 'a3', language: 'fr', codec: 'eac3', channels: 8}, {id: 'a4', language: 'es', codec: 'eac3', channels: 8}],
    subtitles: [{id: 's1', language: 'en'}, {id: 's2', language: 'en', hearingImpaired: true}, {id: 's3', language: 'fr'}, {id: 's4', language: 'es'}]},
  {id: 'version-2', available: true, width: 1920, height: 804, videoCodec: 'h264', bitRate: 12_000_000, container: 'mkv', sizeBytes: 14.6 * 1024 ** 3, durationSeconds: 9960,
    audio: [{id: 'a1', language: 'en', codec: 'eac3', channels: 6}, {id: 'a2', language: 'en', codec: 'aac', channels: 2}], subtitles: [{id: 's1', language: 'en'}, {id: 's2', language: 'es'}]},
];
function FileSlot({model, onManage}: {model: FileInfoModel; onManage: (id: string) => void}) {
  return <>{model.files.map(file => <section className={styles.version} key={file.id}>
    <h3>{file.version.name.map(part => <span className={part.id === 'ordinal' ? styles.versionBadge : undefined} key={part.id}>{part.text}</span>)}</h3>
    <dl>{file.rows.map(row => <span key={row.id} style={{display: 'contents'}}><dt>{row.label}</dt><dd>{row.parts.map(part => <span key={part.id}>{part.text}</span>)}</dd></span>)}
      <dt>{t('next.kit.audio')}</dt><dd>{file.audio.map(track => <span key={track.id} className={styles.track}><span>{track.name.text}</span>{[...track.technical, ...track.detail].map(part => <span key={part.id}>{part.text}</span>)}</span>)}</dd>
      <dt>{t('next.kit.subtitles')}</dt><dd>{file.subtitles.map(track => <span key={track.id}>{track.name.text}{track.detail.map(part => <span key={part.id}>{part.text}</span>)}</span>)}</dd>
    </dl>
  </section>)}<p className={styles.note}>{t('next.overlays.fileInfoNote')}</p>{model.owner && <section className={styles.owner}><h3>{model.owner.heading}</h3>{model.owner.locations.map(location => <div key={location.id} className={styles.location}>{location.path}<div className={styles.ownerActions}>{location.actions.map(action => <Button size="small" key={action.id} onClick={() => onManage(`${location.id}:${action.id}`)}>{action.label}</Button>)}</div></div>)}<div className={styles.ownerActions}>{model.owner.actions.map(action => <Button size="small" key={action.id} onClick={() => onManage(action.id)}>{action.label}</Button>)}</div></section>}</>;
}
function useFixture() {
  const [open, setOpen] = useState(true), [event, setEvent] = useState('');
  return {open, event, setEvent, change: (value: boolean, reason: OverlayDismissReason) => { setOpen(value); setEvent(reason); }, show: () => setOpen(true)};
}
function Fixture({children, event}: {children: ReactNode; event?: string}) { return <div className={styles.canvas} data-overlay-event={event}>{children}</div>; }

function MenuCase({states = false}: {states?: boolean}) {
  const fixture = useFixture(), [favorite, setFavorite] = useState(false), fileFocus = useRef<HTMLButtonElement | null>(null), trigger = useRef<HTMLButtonElement | null>(null);
  return <Fixture event={fixture.event}><div className={styles.anchor}><Menu open={fixture.open} onOpenChange={fixture.change} title={reference.film} returnFocusRef={trigger} initialFocusRef={fileFocus}
    trigger={<Button ref={trigger} variant="glass" size="compact" iconOnly icon="i-more" aria-label={t('next.overlays.more')} />} placement="bottom-start">
    <MenuGroup><MenuItem icon="i-heart" selected={states ? favorite : undefined} onSelect={() => setFavorite(!favorite)}>{t(favorite ? 'next.overlays.removeFavorite' : 'next.overlays.favorite')}</MenuItem>
      <MenuItem icon="i-star" onSelect={() => fixture.setEvent('rate')}>{t('next.overlays.rate')}</MenuItem>
      <MenuItem icon="i-list" disabled={states} onSelect={() => fixture.setEvent('playlist')}>{t('next.overlays.playlist')}</MenuItem>
      <MenuItem icon="i-layers" onSelect={() => fixture.setEvent('collection')}>{t('next.overlays.collection')}</MenuItem>
      <MenuItem icon="i-dl" onSelect={() => fixture.setEvent('download')}>{t('next.overlays.download')}</MenuItem></MenuGroup>
    <MenuSeparator /><MenuGroup><MenuItem icon="i-film" onSelect={() => fixture.setEvent('version')}>{t('next.overlays.differentVersion')}</MenuItem><MenuItem ref={fileFocus} icon="i-info" onSelect={() => fixture.setEvent('file-info')}>{t('next.overlays.fileInfo')}</MenuItem></MenuGroup>
    <MenuGroup label={t('next.overlays.owner')} ownerOnly><MenuItem icon="i-edit" onSelect={() => fixture.setEvent('edit')}>{t('next.overlays.editMetadata')}</MenuItem><MenuItem icon="i-reload" onSelect={() => fixture.setEvent('refresh')}>{t('next.overlays.refreshMetadata')}</MenuItem><MenuItem icon="i-trash" destructive onSelect={() => fixture.setEvent('delete')}>{t('next.overlays.delete')}</MenuItem></MenuGroup>
  </Menu></div></Fixture>;
}
function DesktopMenu() { return <MenuCase />; }
function MenuStates() { return <MenuCase states />; }
function PhoneMenu() {
  const fixture = useFixture(), trigger = useRef<HTMLButtonElement | null>(null);
  return <Fixture event={fixture.event}><Menu open={fixture.open} onOpenChange={fixture.change} title={<>{reference.film}<small className={styles.menuYear}>{reference.year}</small></>} returnFocusRef={trigger} trigger={<Button ref={trigger} size="compact" onClick={fixture.show}>{t('next.overlays.more')}</Button>}>
    <MenuGroup><MenuItem icon="i-check" onSelect={() => fixture.setEvent('watched')}>{t('next.overlays.watched')}</MenuItem><MenuItem icon="i-heart" onSelect={() => fixture.setEvent('favorite')}>{t('next.overlays.favorite')}</MenuItem><MenuItem icon="i-star" onSelect={() => fixture.setEvent('rate')}>{t('next.overlays.ratePhone')}</MenuItem><MenuItem icon="i-list" onSelect={() => fixture.setEvent('playlist')}>{t('next.overlays.playlist')}</MenuItem></MenuGroup>
    <MenuGroup><MenuItem icon="i-film" onSelect={() => fixture.setEvent('version')}>{t('next.overlays.differentVersion')}</MenuItem><MenuItem icon="i-info" onSelect={() => fixture.setEvent('file-info')}>{t('next.overlays.fileInfo')}</MenuItem></MenuGroup>
    <MenuGroup label={t('next.overlays.ownerOnly')} ownerOnly><MenuItem icon="i-edit" onSelect={() => fixture.setEvent('edit')}>{t('next.overlays.editMetadataPhone')}</MenuItem><MenuItem icon="i-reload" onSelect={() => fixture.setEvent('refresh')}>{t('next.overlays.refreshMetadata')}</MenuItem></MenuGroup>
  </Menu></Fixture>;
}
function PopoverCase({modal = false}: {modal?: boolean}) {
  const fixture = useFixture(), [field, setField] = useState('year'), [direction, setDirection] = useState('desc'), focus = useRef<HTMLButtonElement | null>(null);
  return <Fixture event={fixture.event}><div className={styles.anchor}><Popover open={fixture.open} onOpenChange={fixture.change} title={t('next.overlays.sortBy')} modal={modal} initialFocusRef={focus} placement="bottom-start" trigger={<Button size="compact">{t('next.overlays.sortBy')}</Button>}>
    <div className={styles.sortHeading}>{t('next.overlays.sortBy')}</div><div className={styles.sortRows}>{(['title', 'year', 'releaseDate'] as const).map(value => <MenuRow key={value} ref={value === 'releaseDate' ? focus : undefined} selected={field === value} onClick={() => setField(value)}>{t(`next.overlays.${value}`)}</MenuRow>)}</div>
    <MenuSeparator /><div className={styles.sortHeading}>{t('next.overlays.order')}</div><Segmented className={styles.sortOrder} value={direction} onValueChange={setDirection} items={[{value: 'desc', label: t('next.overlays.newest')}, {value: 'asc', label: t('next.overlays.oldest')}]} aria-label={t('next.overlays.order')} />
  </Popover></div><Button className={styles.output} size="small" onClick={() => fixture.setEvent('outside-action')}>{t('next.overlays.done')}</Button></Fixture>;
}
function NonModalPopover() { return <PopoverCase />; }
function ModalPopover() { return <PopoverCase modal />; }
function DialogCase() {
  const fixture = useFixture(), cancel = useRef<HTMLButtonElement | null>(null);
  return <Fixture event={fixture.event}><Button size="compact" onClick={fixture.show}>{t('next.overlays.clearHistory')}</Button><Dialog open={fixture.open} onOpenChange={fixture.change} title={t('next.overlays.clearHistoryTitle')} description={t('next.overlays.clearHistoryBody')} className={styles.historyDialog} initialFocusRef={cancel}
    footer={<><Button ref={cancel} size="small" onClick={() => fixture.change(false, 'action')}>{t('next.overlays.cancel')}</Button><Button variant="primary" size="small" onClick={() => fixture.change(false, 'action')}>{t('next.overlays.clearHistory')}</Button></>}>{null}</Dialog></Fixture>;
}
function RecordingDialog() {
  const fixture = useFixture(), cancel = useRef<HTMLButtonElement | null>(null);
  return <Fixture event={fixture.event}><div className={styles.confirmBackplate}><ReferenceImage src="http://100.120.50.24:8787/assets/pool/movies/the-dark-knight--bd.jpg" className={styles.referenceBackdrop} /></div><Dialog responsive={false} role="alertdialog" width={436} open={fixture.open} onOpenChange={fixture.change} title={t('next.overlays.deleteRecordingTitle')} initialFocusRef={cancel}
    footer={<><Button ref={cancel} size="small" onClick={() => fixture.change(false, 'action')}>{t('next.overlays.cancel')}</Button><Button size="small" icon="i-trash" iconSize={18} className={styles.destructive} onClick={() => fixture.change(false, 'action')}>{t('next.overlays.deleteRecording')}</Button></>}><p><b>{reference.programme}</b>{t('next.overlays.recordingRemoval', {server: reference.server})}</p></Dialog></Fixture>;
}
function PanelCase({owner = false}: {owner?: boolean}) {
  const fixture = useFixture(), model = fileInfoModel(files, owner ? {audience: 'owner', ownerFacts: {locations: files.map(file => ({id: file.id, path: `/srv/media/Movies/Dune Part Two (2024)/${file.id}.mkv`, canDelete: true})), canSplit: true, canMerge: true}, manageSurface: 'desktop'} : {audience: 'viewer'}, {t});
  return <Fixture event={fixture.event}><nav className={styles.toolbar}><Button size="small" onClick={() => fixture.setEvent('navigation')}>{t('next.overlays.done')}</Button><Button size="small" onClick={fixture.show}>{t('next.overlays.fileInfo')}</Button></nav>
    <SidePanel open={fixture.open} onOpenChange={fixture.change} title={model.heading} description={reference.film} width={560} topInset={76}><FileSlot model={model} onManage={fixture.setEvent} /></SidePanel></Fixture>;
}
function Panel() { return <PanelCase />; }
function OwnerPanel() { return <PanelCase owner />; }
function SheetCase({full = false}: {full?: boolean}) {
  const fixture = useFixture(), [detent, setDetent] = useState(full ? 1 : 0), model = fileInfoModel(files, {audience: 'viewer'}, {t});
  return <Fixture event={fixture.event}><Button size="compact" onClick={fixture.show}>{t('next.overlays.fileInfo')}</Button><BottomSheet open={fixture.open} onOpenChange={fixture.change} title={model.heading} description={reference.film} detents={[422, 740]} detent={detent} onDetentChange={setDetent}
    footer={<Button variant="primary" size="compact" onClick={() => fixture.change(false, 'action')}>{t('next.overlays.done')}</Button>}><div className={styles.sheetFiles}><FileSlot model={model} onManage={fixture.setEvent} /></div></BottomSheet></Fixture>;
}
function HalfSheet() { return <SheetCase />; }
function FullSheet() { return <SheetCase full />; }
function ReferenceImage({alt = '', ...props}: ImgHTMLAttributes<HTMLImageElement>) {
  const [status, setStatus] = useState('loading');
  return <img {...props} alt={alt} data-reference-artwork={status}
    onLoad={event => { void event.currentTarget.decode().then(() => setStatus('decoded'), () => setStatus('failed')); }} onError={() => setStatus('failed')} />;
}
function ReferencePoster() { return <ReferenceImage src="http://100.120.50.24:8787/assets/pool/movies/titanic.jpg" />; }
function ToastBackplate({children}: {children: ReactNode}) {
  return <div className={styles.toastBackplate}><div className={styles.referencePosters}>{['the-incredibles', 'spirited-away', 'wall-e'].map(value => <ReferenceImage key={value} src={`http://100.120.50.24:8787/assets/pool/movies/${value}.jpg`} />)}</div>{children}</div>;
}
function ToastCase({action = false, host = false}: {action?: boolean; host?: boolean}) {
  const fixture = useFixture();
  const message = action ? <>{t('next.overlays.removalPrefix')}<b>{reference.removedFilm}</b>{t('next.overlays.removalSuffix')}</> : <><b>{t('next.overlays.detailsCopied')}</b>{' '}{t('next.overlays.detailsSafe')}</>;
  const undo = action ? {label: t('next.overlays.undo'), icon: host ? undefined : 'i-undo' as const, onAction: () => fixture.setEvent('undo')} : undefined;
  const [value, setValue] = useState<ToastData | null>({id: 'reference', message, action: undo, artwork: action ? <ReferencePoster /> : undefined, icon: action ? undefined : 'i-check', tone: 'success'});
  const surface = <div className={styles.toastPosition}><Toast open={fixture.open} onOpenChange={fixture.change} message={message} action={undo} artwork={action ? <ReferencePoster /> : undefined} icon={action ? undefined : 'i-check'} tone="success" /></div>;
  return <Fixture event={fixture.event}>{host ? <><Button size="small" onClick={() => setValue({id: String(Date.now()), message, action: undo, artwork: action ? <ReferencePoster /> : undefined})}>{t('next.overlays.done')}</Button><ToastHost toast={value} onDismiss={(id, reason) => { fixture.setEvent(`${id}:${reason}`); setValue(null); }} /></> : action ? <ToastBackplate>{surface}</ToastBackplate> : surface}</Fixture>;
}
function PlainToast() { return <ToastCase />; }
function ActionToast() { return <ToastCase action />; }
function PhoneToast() { return <ToastCase action host />; }
function QuietBannerCase({phone = false}: {phone?: boolean}) { return <Fixture><div className={phone ? styles.phoneBannerBackplate : styles.bannerBackplate}><ReferenceImage src="http://100.120.50.24:8787/assets/bd-spirited.jpg" className={styles.bannerBackdrop} /><div className={styles.bannerPosition}><Banner variant="quiet">{t('next.overlays.unreachableStart')}<b>{reference.server}</b>{t('next.overlays.unreachableEnd')}</Banner></div></div></Fixture>; }
function QuietBanner() { return <QuietBannerCase />; }
function PhoneQuietBanner() { return <QuietBannerCase phone />; }
function Notice() { const [open, setOpen] = useState(true); return <Fixture><div className={styles.noticePosition}><Banner open={open} icon="i-update" action={{label: t('next.overlays.reload'), onAction: () => setOpen(false)}}><b>{t('next.overlays.updated', {server: reference.server})}</b>{' '}{t('next.overlays.reloadHelp')}</Banner></div></Fixture>; }

export default defineGalleryFamily({id: 'overlays', labelKey: 'next.overlays.gallery', cases: [
  {id: 'menu', labelKey: 'next.overlays.gallery.menu', width: 1440, height: 1214, render: DesktopMenu},
  {id: 'menu-states', labelKey: 'next.overlays.gallery.menuStates', width: 1440, height: 1214, render: MenuStates},
  {id: 'popover', labelKey: 'next.overlays.gallery.popover', width: 600, height: 480, render: NonModalPopover},
  {id: 'modal-popover', labelKey: 'next.overlays.gallery.modalPopover', width: 600, height: 480, render: ModalPopover},
  {id: 'dialog', labelKey: 'next.overlays.gallery.dialog', width: 1440, height: 810, render: DialogCase},
  {id: 'recording-dialog', labelKey: 'next.overlays.deleteRecordingTitle', width: 1440, height: 900, render: RecordingDialog},
  {id: 'side-panel', labelKey: 'next.overlays.gallery.panel', width: 1440, height: 1260, render: Panel},
  {id: 'side-panel-owner', labelKey: 'next.overlays.gallery.panelOwner', width: 1440, height: 1260, render: OwnerPanel},
  {id: 'sheet-half', labelKey: 'next.overlays.gallery.sheetHalf', width: 390, height: 844, render: HalfSheet},
  {id: 'sheet-full', labelKey: 'next.overlays.gallery.sheetFull', width: 390, height: 844, render: FullSheet},
  {id: 'phone-menu', labelKey: 'next.overlays.gallery.phoneMenu', width: 390, height: 844, render: PhoneMenu},
  {id: 'toast', labelKey: 'next.overlays.gallery.toast', width: 1440, height: 810, render: PlainToast},
  {id: 'toast-action', labelKey: 'next.overlays.gallery.toastAction', width: 1440, height: 810, render: ActionToast},
  {id: 'phone-toast', labelKey: 'next.overlays.gallery.phoneToast', width: 390, height: 844, render: PhoneToast},
  {id: 'banner', labelKey: 'next.overlays.gallery.banner', width: 1440, height: 900, render: QuietBanner},
  {id: 'phone-banner', labelKey: 'next.overlays.gallery.banner', width: 390, height: 200, render: PhoneQuietBanner},
  {id: 'notice', labelKey: 'next.overlays.gallery.notice', width: 390, height: 300, render: Notice},
]});
