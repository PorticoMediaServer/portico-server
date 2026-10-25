import {useCallback, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import type {LibraryManagementService, ManagedLibrary} from '@core/library-management.ts';
import {Button, RichSelect, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays/Dialog';
import {FolderPicker} from './FolderPicker';
import {localOnlyKind, metadataAgentFor} from './model';
import styles from './libraries.module.css';

const t = defaultI18n.t;
type Kind = ManagedLibrary['kind'];

/** The seven drawn kinds, in the order Portico lists them (owner-libraries.html#8). */
const KINDS: readonly {id: Kind; label: string; hint: string}[] = [
  {id: 'movie', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.0.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.0.hint')},
  {id: 'tv', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.1.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.1.hint')},
  {id: 'anime', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.2.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.2.hint')},
  {id: 'music', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.3.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.3.hint')},
  {id: 'audiobook', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.4.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.4.hint')},
  {id: 'book', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.5.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.5.hint')},
  {id: 'comic', label: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.6.label'),
    hint: t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.6.hint')},
];


export function AddLibrary({open, onOpenChange, api, service, onError, initialKind = 'movie'}: Readonly<{
  initialKind?: Kind; open: boolean; onOpenChange: (open: boolean) => void; api: LibraryContentApi; service: LibraryManagementService; onError: (error: unknown) => void;
}>) {
  const [kind, setKind] = useState<Kind>(initialKind), [name, setName] = useState(KINDS.find(value => value.id === initialKind)!.label), [touched, setTouched] = useState(false);
  const [paths, setPaths] = useState<readonly string[]>([]);
  const [path, setPath] = useState(''), [source, setSource] = useState<'online' | 'local'>('online');
  const [picking, setPicking] = useState(false), [busy, setBusy] = useState(false), [failed, setFailed] = useState(false);
  const submitting = useRef(false);
  const choosePaths = useCallback((next: readonly string[]) => {setPaths(next); setPath('');}, []);
  const choose = (next: Kind) => { setKind(next); if (!touched) setName(KINDS.find(value => value.id === next)!.label); };
  const create = async () => {
    if (submitting.current || !open || !name.trim() || !paths.length && !path.trim()) return;
    submitting.current = true;
    setBusy(true); setFailed(false);
    try {
      await service.createLibrary({name: name.trim(), kind, ...(paths.length ? {folders: paths} : {path}), metadataAgent: metadataAgentFor(kind) === 'local' ? 'local' : source});
      if (service.getSnapshot().mutation.phase !== 'complete') {setFailed(true);return;}
      onOpenChange(false); setName(KINDS[0].label); setPath(''); setPaths([]); setKind('movie'); setTouched(false); setSource('online');
    } catch (error) { setFailed(true); onError(error); }
    finally {submitting.current = false; setBusy(false);}
  };
  return <>
    <Dialog open={open} onOpenChange={value => {if (!submitting.current) onOpenChange(value);}} dismissOnBack={!busy} dismissOnOutsidePress={!busy} showCloseButton={!busy} size="md" title={t('next.settingsSpec.server.libraries.panels.newLibrary.title')}
      footer={<><Button disabled={busy} onClick={() => {if (!submitting.current) onOpenChange(false);}}>{t('next.settings.cancel')}</Button>
        <Button variant="primary" loading={busy} disabled={busy || !name.trim() || !paths.length && !path.trim()} onClick={() => { void create(); }}>{t('next.settingsSpec.server.libraries.panels.newLibrary.footer.primary')}</Button></>}>
      <div className={styles.form}>
        <RichSelect label={t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.label')}
          value={kind} options={KINDS.map(value => ({value: value.id, label: value.label, description: value.hint}))}
          onValueChange={value => { const next = KINDS.find(item => item.id === value); if (next) choose(next.id); }} />
        <TextField label={t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.0.label')} value={name}
          placeholder={t('next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.0.control.placeholder')}
          onChange={event => { setTouched(true); setName(event.currentTarget.value); }} />
        <div className={styles.formFolder}>
          <TextField label={t('next.settings.folderPath')} value={path} onChange={event => {setPath(event.currentTarget.value); setPaths([]);}} />
          <Button onClick={() => setPicking(true)}>{t('next.settings.chooseFolder')}</Button>
        </div>
        {paths.length ? <div className={styles.chosenFolders}>{paths.map(value => <Button key={value} size="small" variant="glass" icon="i-x" aria-label={`${t('next.ownerLibraries.explorer.removeChosen')} ${value}`} onClick={() => {if (!submitting.current) setPaths(previous => previous.filter(item => item !== value));}}>{value}</Button>)}</div> : null}
        {localOnlyKind(kind)
          ? <p className={styles.formNote}>{t('next.settings.libraries.localOnly')}</p>
          : <RichSelect label={t('next.settingsSpec.server.libraries.panels.library.tabs.2.groups.0.rows.0.label')} value={source}
            options={[{value: 'online', label: t('next.settingsSpec.server.libraries.panels.library.tabs.2.groups.0.rows.0.control.options.0.label'),
              description: t('next.settingsSpec.server.libraries.panels.library.tabs.2.groups.0.rows.0.control.options.0.hint')},
              {value: 'local', label: t('next.settingsSpec.server.libraries.panels.library.tabs.2.groups.0.rows.0.control.options.1.label'),
                description: t('next.settingsSpec.server.libraries.panels.library.tabs.2.groups.0.rows.0.control.options.1.hint')}]}
            onValueChange={value => setSource(value === 'local' ? 'local' : 'online')} />}
        {failed && <p role="alert">{t('next.settings.libraries.createFailed')}</p>}
      </div>
    </Dialog>
    <FolderPicker open={picking} onOpenChange={setPicking} api={api} initialPaths={paths} onChoose={setPath} onChooseMany={choosePaths} />
  </>;
}
