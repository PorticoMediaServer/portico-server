import {useLayoutEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {UploadSourceController} from './upload-source-controller';
import styles from './owner.module.css';
const t = defaultI18n.t;
const noSubscribe = () => () => {};
const noSnapshot = () => undefined;
type UploadLease = {id: string; live: boolean; current: () => boolean; controller: UploadSourceController};

/** The upload dialog owns its controller in a committed layout lease. Close
 * retires the lease, the in-memory input and every read before the page closes
 * it; Strict effect replay creates a fresh controller. */
export function UploadSourceDialog({api, serverId, isCurrent, reportError, close, saved}: {
  api: LibraryContentApi; serverId: string; isCurrent: () => boolean; reportError: (error: unknown) => void; close: () => void; saved: () => void;
}) {
  const [published, publish] = useState<UploadLease>();
  useLayoutEffect(() => {
    let live = true;
    const current = () => live && isCurrent();
    const controller = new UploadSourceController(api, serverId, current, reportError);
    const lease: UploadLease = {id: crypto.randomUUID(), live: true, current, controller};
    publish(lease);
    return () => {lease.live = false;controller.dispose();};
  }, [api, serverId, isCurrent, reportError]);
  const lease = published?.live && published.current() ? published : undefined;
  const state = useSyncExternalStore(lease?.controller.subscribe ?? noSubscribe, lease?.controller.getSnapshot ?? noSnapshot, lease?.controller.getSnapshot ?? noSnapshot);
  const closeCurrent = () => {
    if (!lease?.current()) return;
    lease.live = false;lease.controller.dispose();close();
  };
  const savedCurrent = () => {
    if (!lease?.current()) return;
    lease.live = false;lease.controller.dispose();saved();
  };
  if (!lease || !state) return null;
  const {draft, preview, busy, reading, error} = state;
  const edit = (patch: Parameters<typeof lease.controller.edit>[0]) => {if (lease.current()) lease.controller.edit(patch);};
  const read = (field: 'playlist' | 'guide', file: File) => {if (lease.current()) void lease.controller.read(field, () => file.text());};
  const submit = () => {
    if (!lease.current()) return;
    if (preview) void lease.controller.save().then(ok => {if (ok) savedCurrent();});
    else void lease.controller.check();
  };
  return <Dialog open width={560} title={t('web.liveTv.upload')} description={t('web.liveTv.uploadLede')} onOpenChange={open => {if (!open) closeCurrent();}}
    footer={<><Button size="small" variant="text" onClick={closeCurrent}>{t('action.cancel')}</Button><Button size="small" variant="primary" loading={busy || reading}
      disabled={!draft.name.trim() || !draft.playlist} onClick={submit}>{t(preview ? 'web.liveTv.addSource' : 'web.liveTv.checkFiles')}</Button></>}>
    <div className={styles.fields}>
      <TextField label={t('profile.name')} value={draft.name} maxLength={100} disabled={busy} autoFocus onChange={event => edit({name: event.currentTarget.value})}/>
      <TextField label={t('web.liveTv.playlistFile')} type="file" accept=".m3u,.m3u8,text/plain" disabled={busy} onChange={event => {const file = event.currentTarget.files?.[0];if (file) read('playlist', file);}}/>
      <TextField label={t('web.liveTv.guideFile')} type="file" accept=".xml,text/xml,application/xml" disabled={busy} onChange={event => {const file = event.currentTarget.files?.[0];if (file) read('guide', file);}}/>
      <TextField label={t('web.dvrSettings.tuners')} description={t('web.liveTv.tunersHelp')} type="number" min={0} max={64} step={1} value={draft.tunerCount} disabled={busy}
        onChange={event => {const value = event.currentTarget.valueAsNumber;if (Number.isSafeInteger(value) && value >= 0 && value <= 64) edit({tunerCount: value});}}/>
      {preview && <div className={styles.success} role="status"><b>{[t('server.live.channelCount', {count: preview.channels}), t('server.live.programCount', {count: preview.programmes})].join(' · ')}</b><p>{preview.names?.slice(0, 6).join(', ')}</p></div>}
      {!!error && <p role="alert" className={styles.error}>{errorMessage(error)}</p>}
    </div>
  </Dialog>;
}
