import {useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {SourceKind} from '@core/linear-api.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {importSkipped} from '@core/server-admin/live-words.ts';
import {Button, Segmented, Select, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import type {RemoteSourceController} from './remote-source-controller';
import styles from './owner.module.css';
const t = defaultI18n.t;

/** Dialog ownership stays with the page: closing disposes its controller and
 * clears credentials rather than keeping them in the source list model. */
export function RemoteSourceDialog({controller, close, saved}: {controller: RemoteSourceController; close: () => void; saved: () => void}) {
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const {draft, preview, confirm, busy} = state;
  const skipped = preview && importSkipped(t, defaultI18n.has, preview.preview.diagnostics);
  const check = () => void controller.check(!!confirm);
  return <Dialog open onOpenChange={open => {if (!open) close();}} title={t('web.liveTv.addDialog')} description={t('web.liveTv.addDialogLede')} width={560}
    footer={<><Button variant="text" size="small" onClick={close}>{t('action.cancel')}</Button><Button variant="primary" size="small" loading={busy}
      disabled={!draft.name.trim() || !draft.locator.trim()} onClick={preview ? () => void controller.save().then(ok => {if (ok) saved();}) : check}>
      {t(preview ? 'web.liveTv.addSource' : confirm ? 'web.liveTv.lanConfirm' : 'web.liveTv.checkSource')}</Button></>}>
    <div className={styles.fields}>
      <Segmented aria-label={t('web.libraries.kind')} value={draft.kind} disabled={busy}
        items={(['m3u', 'xtream', 'hdhomerun'] as const).map(value => ({value, label: t(`server.live.kind.${value}`)}))}
        onValueChange={kind => controller.edit({kind: kind as SourceKind})}/>
      <TextField label={t('profile.name')} value={draft.name} maxLength={100} disabled={busy} autoFocus onChange={event => controller.edit({name: event.currentTarget.value})}/>
      <TextField label={t(draft.kind === 'hdhomerun' ? 'web.liveTv.tunerAddress' : draft.kind === 'xtream' ? 'web.liveTv.serverAddress' : 'web.liveTv.playlistUrl')}
        value={draft.locator} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => controller.edit({locator: event.currentTarget.value})}/>
      <TextField label={t('web.liveTv.guideUrl')} description={t(draft.kind === 'hdhomerun' ? 'server.live.guideUrlTunerHelp' : 'server.live.guideUrlHelp')}
        value={draft.guideUrl} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => controller.edit({guideUrl: event.currentTarget.value})}/>
      {draft.kind === 'xtream' && <><TextField label={t('web.direct.username')} value={draft.username} disabled={busy} autoCapitalize="off" spellCheck={false} autoComplete="off" onChange={event => controller.edit({username: event.currentTarget.value})}/>
        <TextField label={t('web.account.password')} type="password" value={draft.password} disabled={busy} autoComplete="off" onChange={event => controller.edit({password: event.currentTarget.value})}/></>}
      <Select label={t('web.liveTv.whoCanWatch')} value={draft.viewerAccess} disabled={busy} options={[
        {value: 'server-members', label: t('web.liveTv.everyoneOnServer')}, {value: 'owner-only', label: t('web.together.onlyMe')},
      ]} onValueChange={value => controller.edit({viewerAccess: value as typeof draft.viewerAccess})}/>
      <Select label={t('web.liveTv.refreshGuide')} value={String(draft.refreshMs)} disabled={busy} options={[
        {value: '3600000', label: t('web.libraries.everyHour')}, {value: '21600000', label: t('web.libraries.every6h')},
        {value: '43200000', label: t('web.liveTv.every12h')}, {value: '86400000', label: t('web.libraries.daily')},
      ]} onValueChange={value => controller.edit({refreshMs: Number(value)})}/>
      {preview && <div className={styles.success} role="status"><b>{[t('server.live.channelCount', {count: preview.preview.channels}), t('server.live.programCount', {count: preview.preview.programmes})].join(' · ')}</b>
        <p>{preview.preview.names.slice(0, 6).join(', ')}{preview.preview.names.length > 6 ? '…' : ''}</p></div>}
      {skipped && <div className={styles.warning}>{[skipped.summary, ...skipped.reasons].join(' ')}</div>}
      {confirm && <div className={styles.warning}><b>{t('web.liveTv.lanTitle')}</b><p>{t('web.liveTv.lanBody')}</p><ul>{confirm.map(root => <li key={root}>{root}</li>)}</ul></div>}
      {!!state.error && <p className={styles.error} role="alert">{errorMessage(state.error)}</p>}
    </div>
  </Dialog>;
}
