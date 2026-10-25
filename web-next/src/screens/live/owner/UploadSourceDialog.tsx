import {useEffect, useRef, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import type {UploadSourceController} from './upload-source-controller';
import styles from './owner.module.css';
import {dialogAction} from './dialog-lifetime';
const t = defaultI18n.t;
export function UploadSourceDialog({controller, close, saved}: {controller: UploadSourceController; close: () => void; saved: () => void}) {
  const owner = useRef(controller);owner.current = controller;
  useEffect(() => () => controller.dispose(), [controller]);
  const closeCurrent = dialogAction(owner, controller, () => {controller.dispose();close();}), savedCurrent = dialogAction(owner, controller, saved);
  const {draft, preview, busy, reading, error} = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  return <Dialog open width={560} title={t('web.liveTv.upload')} description={t('web.liveTv.uploadLede')} onOpenChange={open => {if (!open) closeCurrent();}}
    footer={<><Button size="small" variant="text" onClick={closeCurrent}>{t('action.cancel')}</Button><Button size="small" variant="primary" loading={busy || reading}
      disabled={!draft.name.trim() || !draft.playlist} onClick={dialogAction(owner, controller, () => {if (preview) void controller.save().then(ok => {if (ok) savedCurrent();});else void controller.check();})}>{t(preview ? 'web.liveTv.addSource' : 'web.liveTv.checkFiles')}</Button></>}>
    <div className={styles.fields}>
      <TextField label={t('profile.name')} value={draft.name} maxLength={100} disabled={busy} autoFocus onChange={event => controller.edit({name: event.currentTarget.value})}/>
      <TextField label={t('web.liveTv.playlistFile')} type="file" accept=".m3u,.m3u8,text/plain" disabled={busy} onChange={event => {const file = event.currentTarget.files?.[0];if (file) void controller.read('playlist', () => file.text());}}/>
      <TextField label={t('web.liveTv.guideFile')} type="file" accept=".xml,text/xml,application/xml" disabled={busy} onChange={event => {const file = event.currentTarget.files?.[0];if (file) void controller.read('guide', () => file.text());}}/>
      <TextField label={t('web.dvrSettings.tuners')} description={t('web.liveTv.tunersHelp')} type="number" min={0} max={64} step={1} value={draft.tunerCount} disabled={busy}
        onChange={event => {const value = event.currentTarget.valueAsNumber;if (Number.isSafeInteger(value) && value >= 0 && value <= 64) controller.edit({tunerCount: value});}}/>
      {preview && <div className={styles.success} role="status"><b>{[t('server.live.channelCount', {count: preview.channels}), t('server.live.programCount', {count: preview.programmes})].join(' · ')}</b><p>{preview.names?.slice(0, 6).join(', ')}</p></div>}
      {!!error && <p role="alert" className={styles.error}>{errorMessage(error)}</p>}
    </div>
  </Dialog>;
}
