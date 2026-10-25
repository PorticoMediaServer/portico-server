import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ProfileDeletion} from '@core/profile-management.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import type {ProfileSettingsSource} from './source';
import styles from './profile.module.css';
const t = defaultI18n.t;

/** Read the producer's deletion preview before offering the destructive action.
 * The checked delete client sends that preview revision, never a guessed one. */
export function ProfileDelete({source, isCurrent, onFailure}: {
  source: ProfileSettingsSource; isCurrent: () => boolean; onFailure: () => void;
}) {
  const state = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  const [preview, setPreview] = useState<ProfileDeletion>(), [loading, setLoading] = useState(false);
  const pending = useRef(false), live = useRef(true), trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {live.current = true;return () => {live.current = false;};}, []);
  const selected = source.selected();
  const current = () => live.current && isCurrent() && !!source.getSnapshot().document;
  const open = async () => {
    if (!selected || pending.current || state.busy || !current()) return;
    pending.current = true;setLoading(true);
    try {const result = await source.previewDelete(selected);if (current()) setPreview(result);}
    catch {if (current()) onFailure();}
    finally {pending.current = false;if (current()) setLoading(false);}
  };
  const remove = async () => {
    if (!preview || pending.current || state.busy || !current()) return;
    pending.current = true;
    try {await source.deleteProfile(preview);if (current()) setPreview(undefined);}
    catch {if (current() && !source.getSnapshot().error) onFailure();}
    finally {pending.current = false;}
  };
  if (!isCurrent() || !state.document?.canManage || !state.document.capabilities.manageAccount || !selected || selected.primary) return null;
  return <>
    <Button ref={trigger} className={styles.danger} size="small" variant="glass" loading={loading} disabled={state.busy} onClick={() => {void open();}}>{t('next.settingsSpec.account.profile.groups.1.rows.3.control.label')}</Button>
    {preview && <Dialog open width={420} returnFocusRef={trigger} title={t('profile.deleteConfirm.title', {name: preview.name ?? selected.name})}
      description={t('next.settingsSpec.account.profile.groups.1.rows.3.control.confirm')}
      dismissOnOutsidePress={!state.busy} dismissOnBack={!state.busy} showCloseButton={!state.busy} onOpenChange={open => {if (!open && !pending.current) setPreview(undefined);}}
      footer={<><Button size="small" disabled={state.busy} onClick={() => {if (!pending.current) setPreview(undefined);}}>{t('action.cancel')}</Button>
        <Button className={styles.danger} size="small" variant="glass" loading={state.busy} onClick={() => {void remove();}}>{t('profile.deleteConfirm.action')}</Button></>}>
      {typeof preview.playlists === 'number' && <p className={styles.limit}>{t('profile.deleteConfirm.playlists', {count: preview.playlists})}</p>}
    </Dialog>}
  </>;
}
