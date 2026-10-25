import {useEffect, useState, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {validateRepair, type RepairTarget} from '@core/metadata-repair.ts';
import {Dialog} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import {MetadataEditor} from '../metadata/MetadataEditor';
import {screenLifetime, type ScreenContext} from '../detail/context';

/** Reading items are nonplayable: resolve their library through the existing owner metadata response. */
export function ReadMetadata({context, itemId, onClose, anchor}: {context: ScreenContext; itemId: string; onClose: () => void; anchor: RefObject<HTMLElement | null>}) {
  const [ready, setReady] = useState<Readonly<{target: RepairTarget; life: ReturnType<typeof screenLifetime>}>>(), [failed, setFailed] = useState(false), [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true; const life = screenLifetime(context);
    setReady(undefined); setFailed(false);
    const viewer = context.identity?.viewer;
    if (viewer?.authority !== 'local' || viewer.role !== 'owner') {life.dispose(); return;}
    void life.api.request<unknown>(encodedRoutePath('get_metadata_kind_id', {kind: 'item', id: encodeURIComponent(itemId)}), 'GET', undefined, life.signal()).then(raw => {
      if (!active || !life.current()) return;
      const libraryId = (raw as {libraryId?: unknown} | null)?.libraryId;
      if (typeof libraryId !== 'string' || !libraryId || libraryId.length > 256) throw new Error('invalid_metadata_target');
      const selected: RepairTarget = Object.freeze({kind: 'item', id: itemId, libraryId});
      validateRepair(raw, context.scope, selected);
      setReady({target: selected, life});
    }).catch(error => {if (active && life.current()) {setFailed(true); context.effects.reportError(error);}});
    return () => {active = false; life.dispose();};
  }, [context, itemId, retry]);
  const owner = context.identity?.viewer;
  if (!context.isCurrent() || owner?.authority !== 'local' || owner.role !== 'owner') return null;
  return <Dialog open placement="right" width={520} title={defaultI18n.t('entry.editMetadata')} onOpenChange={open => {if (!open) onClose();}} returnFocusRef={anchor}>
    {ready ? <MetadataEditor api={ready.life.api} scope={context.scope} target={ready.target} onClose={onClose}/>
      : failed ? <StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')}
        primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/> : <Skeleton height={480}/>}
  </Dialog>;
}
