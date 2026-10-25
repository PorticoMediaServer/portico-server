import {useEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {ConsoleClient} from '@core/console.ts';
import {liveDefaultsForm} from '@core/server-admin/server-forms.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {LiveSourceController, type OwnerLiveSource} from './source-controller';
import {RemoteSourceController} from './remote-source-controller';
import {UploadSourceController} from './upload-source-controller';
import {RemoteSourceDialog} from './RemoteSourceDialog';
import {UploadSourceDialog} from './UploadSourceDialog';
import {DiscoverTuners} from './DiscoverTuners';
import {SourceList} from './SourceList';
import {LiveDefaults} from './LiveDefaults';
import {SourceSettingsController} from '../../settings/server-livetv/source-settings-controller';
import {SourceChoicesController} from '../../settings/server-livetv/source-choices-controller';
import {ChannelMapController} from '../../settings/server-livetv/channel-map-controller';
import {SourceSettingsDialog} from '../../settings/server-livetv/SourceSettingsDialog';
import {ChannelMapDialog} from '../../settings/server-livetv/ChannelMapDialog';
import {terminal} from './lifetime';
import styles from './owner.module.css';
const t = defaultI18n.t;
type Modal = {kind: 'remote'; controller: RemoteSourceController} | {kind: 'upload'; controller: UploadSourceController} | {kind: 'discover'} |
  {kind: 'settings'; source: OwnerLiveSource; controller: SourceSettingsController; choices: {groups: SourceChoicesController; channels: SourceChoicesController}} |
  {kind: 'channels'; source: OwnerLiveSource; controller: ChannelMapController} | {kind: 'remove'; source: OwnerLiveSource};
export type OwnerSourcesOptions = Readonly<{api: LibraryContentApi; serverId: string; viewerId: string; isCurrent: () => boolean; reportError: (error: unknown) => void}>;
/** Constructed only by the mounted owner admission. Every child captures that
 * same lifetime; replacing the identity retires all reads and private drafts. */
export function createOwnerSources(options: OwnerSourcesOptions) {
  let active = true;const listeners = new Set<() => void>(), children = new Set<{dispose: () => void}>();
  const current = () => active && options.isCurrent();
  const dispose = () => {if (!active) return;active = false;for (const child of children) child.dispose();children.clear();for (const listener of listeners) listener();listeners.clear();};
  const reportError = (error: unknown) => {if (terminal(error)) dispose();options.reportError(error);};
  const api: LibraryContentApi = {async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) {
    if (!current() || signal?.aborted) throw {code: 'cancelled'};
    try {const result = await options.api.request<T>(path, method, body, signal);if (!current() || signal?.aborted) throw {code: 'cancelled'};return result;}
    catch (error) {if (terminal(error)) reportError(error);throw error;}
  }};
  const sources = new LiveSourceController(api, options.serverId, current, reportError);
  const defaults = liveDefaultsForm({api, serverId: options.serverId, console: new ConsoleClient(api, {serverId: options.serverId, viewerId: options.viewerId}), operationId: channelRequestID});
  children.add(sources);children.add(defaults);
  return {sources, defaults, api, current, reportError, serverId: options.serverId,
    track<T extends {dispose: () => void}>(child: T): T {if (!current()) child.dispose();else children.add(child);return child;},
    release(child: {dispose: () => void}) {children.delete(child);child.dispose();},
    subscribe: (listener: () => void) => {listeners.add(listener);return () => {listeners.delete(listener);};}, dispose};
}
export type OwnerSources = ReturnType<typeof createOwnerSources>;
export function OwnerSourcesPage({source}: {source: OwnerSources}) {
  const snapshot = useSyncExternalStore(source.sources.subscribe, source.sources.getSnapshot, source.sources.getSnapshot);
  const defaults = useSyncExternalStore(source.defaults.subscribe, source.defaults.getSnapshot, source.defaults.getSnapshot);
  const active = useSyncExternalStore(source.subscribe, source.current, () => false);
  const [modal, setModal] = useState<Modal>();
  useEffect(() => {void source.sources.load();void source.defaults.load();return () => source.dispose();}, [source]);
  useEffect(() => {
    if (!snapshot.document?.refresh.some(row => row.state === 'refreshing')) return;
    const timer = setTimeout(() => {if (source.current() && document.visibilityState !== 'hidden') void source.sources.load();}, 5000);
    return () => clearTimeout(timer);
  }, [source, snapshot.document]);
  useEffect(() => () => {if (modal && 'controller' in modal) source.release(modal.controller);if (modal?.kind === 'settings') {source.release(modal.choices.groups);source.release(modal.choices.channels);}}, [modal, source]);
  if (!active || !source.current()) return null;
  const close = () => {if (source.current()) setModal(undefined);};
  const remote = (tuner?: string) => {if (source.current()) setModal({kind: 'remote', controller: source.track(new RemoteSourceController(source.api, source.serverId, source.current, source.reportError, channelRequestID, tuner))});};
  const saved = () => {if (source.current()) {setModal(undefined);void source.sources.load();}};
  return <>
    {!!snapshot.error && <p role="alert" className={styles.error}>{errorMessage(snapshot.error)}</p>}
    <SourceList snapshot={snapshot} refresh={() => void source.sources.load()} add={kind => {
      if (!source.current()) return;
      if (kind === 'remote') remote();else if (kind === 'discover') setModal({kind});else setModal({kind, controller: source.track(new UploadSourceController(source.api, source.serverId, source.current, source.reportError))});
    }} configure={selected => {
      if (!source.current()) return;
      setModal({kind: 'settings', source: selected, controller: source.track(new SourceSettingsController(source.api, source.serverId, selected.id, source.current, source.reportError)), choices: {
        groups: source.track(new SourceChoicesController(source.api, selected.id, 'groups', source.current, source.reportError)), channels: source.track(new SourceChoicesController(source.api, selected.id, 'channels', source.current, source.reportError)),
      }});
    }} channels={selected => {if (source.current()) setModal({kind: 'channels', source: selected, controller: source.track(new ChannelMapController(source.api, source.serverId, selected.id, source.current, source.reportError))});}}
      act={(selected, action) => {if (!source.current()) return;if (action === 'remove') setModal({kind: 'remove', source: selected});else void source.sources.act(selected, action);}}/>
    {!!defaults.loadError && <p role="alert" className={styles.error}>{errorMessage(defaults.loadError)}<Button size="small" onClick={() => {if (source.current()) void source.defaults.load();}}>{t('action.tryAgain')}</Button></p>}
    {!!defaults.saveError && <p role="alert" className={styles.error}>{errorMessage(defaults.saveError)}</p>}
    <LiveDefaults form={source.defaults} isCurrent={source.current} failure={source.reportError}/>
    {modal?.kind === 'remote' && <RemoteSourceDialog controller={modal.controller} close={close} saved={saved}/>}
    {modal?.kind === 'upload' && <UploadSourceDialog controller={modal.controller} close={close} saved={saved}/>}
    {modal?.kind === 'discover' && <DiscoverTuners api={source.api} serverId={source.serverId} isCurrent={source.current} reportError={source.reportError} close={close} useTuner={remote}/>}
    {modal?.kind === 'settings' && <SourceSettingsDialog controller={modal.controller} choices={modal.choices} name={modal.source.name} guides={snapshot.document?.sources ?? []} close={close} changed={() => {if (source.current()) void source.sources.load();}}/>}
    {modal?.kind === 'channels' && <ChannelMapDialog controller={modal.controller} name={modal.source.name} close={close}/>}
    {modal?.kind === 'remove' && <Dialog open title={t('web.liveTv.removeTitle', {name: modal.source.name})} description={t('web.liveTv.removeBody')} onOpenChange={open => {if (!open) close();}}
      footer={<><Button size="small" variant="text" onClick={close}>{t('action.cancel')}</Button><Button size="small" variant="primary" loading={!!snapshot.busy} onClick={() => {if (source.current()) void source.sources.act(modal.source, 'remove').then(ok => {if (ok && source.current()) setModal(value => value === modal ? undefined : value);});}}>{t('web.liveTv.removeSource')}</Button></>}><></></Dialog>}
  </>;
}
