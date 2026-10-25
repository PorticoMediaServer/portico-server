import {useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {createOwnerSources} from '../../../live/owner/OwnerSourcesPage';
import type {OwnerLiveSource} from '../../../live/owner/source-controller';
import {Button} from '../../../../kit/controls';
import {Tuners} from './Tuners';
import {RecordingPermissions} from './RecordingPermissions';
import {RecordingRules} from './RecordingRules';
import {RecordingDefaults} from './RecordingDefaults';
import {RecordingFolder} from './RecordingFolder';
import {RecordingStorage} from './RecordingStorage';
import styles from '../../../live/owner/owner.module.css';
const t = defaultI18n.t;
const changed = (saved: unknown, draft: unknown) => !saved || !draft ? 0 : Object.keys(draft).filter(key => JSON.stringify((draft as Record<string, unknown>)[key]) !== JSON.stringify((saved as Record<string, unknown>)[key])).length;

/** One page command over the existing revisioned drafts. Successful sections
 * adopt their responses; a failed section leaves remaining edits unsaved. */
export function RecordingPage({source, sources}: {source: ReturnType<typeof createOwnerSources>; sources: readonly OwnerLiveSource[]}) {
  const permissions = useSyncExternalStore(source.permissions.subscribe, source.permissions.getSnapshot, source.permissions.getSnapshot);
  const defaults = useSyncExternalStore(source.recording.subscribe, source.recording.getSnapshot, source.recording.getSnapshot);
  const storage = useSyncExternalStore(source.storage.subscribe, source.storage.getSnapshot, source.storage.getSnapshot);
  const root = useSyncExternalStore(source.root.subscribe, source.root.getSnapshot, source.root.getSnapshot);
  const [busy, setBusy] = useState(false), running = useRef(false), body = useRef<HTMLDivElement>(null);
  const count = Object.keys(permissions.edits).length + changed(defaults.saved, defaults.draft) + changed(storage.storage?.policy, storage.draft) + Number(source.root.dirty());
  const saving = busy || permissions.saving || defaults.saving || storage.saving || root.saving;
  const save = async () => {
    if (!source.current() || running.current || saving || defaults.problem || !count) return;
    if (body.current && [...body.current.querySelectorAll('form')].some(form => !form.reportValidity())) return;
    running.current = true;setBusy(true);
    try {
      await source.permissions.save();
      if (!source.current() || Object.keys(source.permissions.getSnapshot().edits).length) return;
      await source.recording.save();
      if (!source.current() || source.recording.getSnapshot().dirty) return;
      const state = source.storage.getSnapshot();
      if (changed(state.storage?.policy, state.draft)) await source.storage.save();
      if (!source.current()) return;
      const savedStorage = source.storage.getSnapshot();
      if (changed(savedStorage.storage?.policy, savedStorage.draft)) return;
      if (source.root.dirty()) await source.root.save();
    } catch (error) {if (source.current()) source.reportError(error);}
    finally {running.current = false;if (source.current()) setBusy(false);}
  };
  if (!source.current()) return null;
  return <div ref={body}>
    <fieldset disabled={saving} style={{border: 0, padding: 0, margin: 0, minWidth: 0}}>
      <Tuners controller={source.tuners}/>
      <RecordingPermissions controller={source.permissions} embedded/>
      <RecordingRules controller={source.rules} permissions={source.permissions} sources={sources}/>
      <RecordingDefaults form={source.recording} current={source.current} reportError={source.reportError} embedded/>
      <RecordingStorage controller={source.storage} embedded/>
      <RecordingFolder controller={source.root} api={source.api}/>
    </fieldset>
    {!!count && <div className={styles.save}>
      <span>{t('web.liveTv.changesWaiting', {count})}</span>
      <Button size="small" variant="text" disabled={saving || permissions.uncertain || root.uncertain} onClick={() => {if (source.current()) {source.permissions.discard();source.recording.discard();source.storage.discard();source.root.discard();}}}>{t('action.discard')}</Button>
      <Button size="small" variant="primary" loading={saving} disabled={!!defaults.problem || permissions.loading || defaults.loading || storage.loading || root.loading} onClick={() => {void save();}}>{t('action.save')}</Button>
    </div>}
  </div>;
}
