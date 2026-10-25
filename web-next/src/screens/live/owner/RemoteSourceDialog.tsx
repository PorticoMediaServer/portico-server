import {useLayoutEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {SourceKind} from '@core/linear-api.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {importSkipped} from '@core/server-admin/live-words.ts';
import {Button, Segmented, Select, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {RemoteSourceController} from './remote-source-controller';
import styles from './owner.module.css';
const t = defaultI18n.t;
const noSubscribe = () => () => {};
const noSnapshot = () => undefined;
type RemoteLease = {id: string; live: boolean; current: () => boolean; controller: RemoteSourceController};

/** Dialog ownership is the dialog's committed layout lease: closing retires
 * the controller before the page closes it, so credentials and requests never
 * outlive the exact opening. */
export function RemoteSourceDialog({api, serverId, isCurrent, reportError, tuner, close, saved}: {
  api: LibraryContentApi; serverId: string; isCurrent: () => boolean; reportError: (error: unknown) => void; tuner?: string; close: () => void; saved: () => void;
}) {
  const [published, publish] = useState<RemoteLease>();
  useLayoutEffect(() => {
    let live = true;
    const current = () => live && isCurrent();
    const controller = new RemoteSourceController(api, serverId, current, reportError, channelRequestID, tuner);
    const lease: RemoteLease = {id: crypto.randomUUID(), live: true, current, controller};
    publish(lease);
    return () => {lease.live = false;controller.dispose();};
  }, [api, serverId, isCurrent, reportError, tuner]);
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
  const {draft, preview, confirm, busy} = state;
  const skipped = preview && importSkipped(t, defaultI18n.has, preview.preview.diagnostics);
  const edit = (patch: Parameters<typeof lease.controller.edit>[0]) => {if (lease.current()) lease.controller.edit(patch);};
  const check = () => {if (lease.current()) void lease.controller.check(!!confirm);};
  const save = () => {if (lease.current()) void lease.controller.save().then(ok => {if (ok) savedCurrent();});};
  return <Dialog open onOpenChange={open => {if (!open) closeCurrent();}} title={t('web.liveTv.addDialog')} description={t('web.liveTv.addDialogLede')} width={560}
    footer={<><Button variant="text" size="small" onClick={closeCurrent}>{t('action.cancel')}</Button><Button variant="primary" size="small" loading={busy}
      disabled={!draft.name.trim() || !draft.locator.trim()} onClick={preview ? save : check}>
      {t(preview ? 'web.liveTv.addSource' : confirm ? 'web.liveTv.lanConfirm' : 'web.liveTv.checkSource')}</Button></>}>
    <div className={styles.fields}>
      <Segmented aria-label={t('web.libraries.kind')} value={draft.kind} disabled={busy}
        items={(['m3u', 'xtream', 'hdhomerun'] as const).map(value => ({value, label: t(`server.live.kind.${value}`)}))}
        onValueChange={kind => edit({kind: kind as SourceKind})}/>
      <TextField label={t('profile.name')} value={draft.name} maxLength={100} disabled={busy} autoFocus onChange={event => edit({name: event.currentTarget.value})}/>
      <TextField label={t(draft.kind === 'hdhomerun' ? 'web.liveTv.tunerAddress' : draft.kind === 'xtream' ? 'web.liveTv.serverAddress' : 'web.liveTv.playlistUrl')}
        value={draft.locator} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => edit({locator: event.currentTarget.value})}/>
      <TextField label={t('web.liveTv.guideUrl')} description={t(draft.kind === 'hdhomerun' ? 'server.live.guideUrlTunerHelp' : 'server.live.guideUrlHelp')}
        value={draft.guideUrl} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => edit({guideUrl: event.currentTarget.value})}/>
      {draft.kind === 'xtream' && <><TextField label={t('web.direct.username')} value={draft.username} disabled={busy} autoCapitalize="off" spellCheck={false} autoComplete="off" onChange={event => edit({username: event.currentTarget.value})}/>
        <TextField label={t('web.account.password')} type="password" value={draft.password} disabled={busy} autoComplete="off" onChange={event => edit({password: event.currentTarget.value})}/></>}
      <Select label={t('web.liveTv.whoCanWatch')} value={draft.viewerAccess} disabled={busy} options={[
        {value: 'server-members', label: t('web.liveTv.everyoneOnServer')}, {value: 'owner-only', label: t('web.together.onlyMe')},
      ]} onValueChange={value => edit({viewerAccess: value as typeof draft.viewerAccess})}/>
      <Select label={t('web.liveTv.refreshGuide')} value={String(draft.refreshMs)} disabled={busy} options={[
        {value: '3600000', label: t('web.libraries.everyHour')}, {value: '21600000', label: t('web.libraries.every6h')},
        {value: '43200000', label: t('web.liveTv.every12h')}, {value: '86400000', label: t('web.libraries.daily')},
      ]} onValueChange={value => edit({refreshMs: Number(value)})}/>
      {preview && <div className={styles.success} role="status"><b>{[t('server.live.channelCount', {count: preview.preview.channels}), t('server.live.programCount', {count: preview.preview.programmes})].join(' · ')}</b>
        <p>{preview.preview.names.slice(0, 6).join(', ')}{preview.preview.names.length > 6 ? '…' : ''}</p></div>}
      {skipped && <div className={styles.warning}>{[skipped.summary, ...skipped.reasons].join(' ')}</div>}
      {confirm && <div className={styles.warning}><b>{t('web.liveTv.lanTitle')}</b><p>{t('web.liveTv.lanBody')}</p><ul>{confirm.map(root => <li key={root}>{root}</li>)}</ul></div>}
      {!!state.error && <p className={styles.error} role="alert">{errorMessage(state.error)}</p>}
    </div>
  </Dialog>;
}
