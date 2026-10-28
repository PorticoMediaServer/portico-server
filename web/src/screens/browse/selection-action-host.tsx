import {useEffect, useSyncExternalStore} from 'react';
import {selectionActionTargetInput, type SelectionAction, type SelectionActionOptions} from '@core/selection-actions.ts';
import type {QuerySelectionTarget, SelectionOperation} from '@core/selection-model/types.ts';
import {BrowserSelectionActionModel, type BrowserSelectionActionOptions} from './selection-action-model.ts';
import {SelectionActionSurface} from './SelectionActionSurface.tsx';

export type BrowserQueryActionsOptions = BrowserSelectionActionOptions & Readonly<{
  deviceId: string; owner: boolean; isCurrent: () => boolean; signal: AbortSignal;
  /** Principal/authority/route fact source, in addition to the selection model below. */
  subscribe: (listener: () => void) => () => void;
  /** Home supplies its actual immutable document; equal revision strings do not extend its lifetime. */
  sourceIdentity?: () => unknown;
}>;
type Host<C> = Readonly<{
  subscribe: (listener: () => void) => () => void;
  captureOperation?: () => SelectionOperation<C> | null;
  isOperationCurrent?: (operation: SelectionOperation<C>) => boolean;
}>;
const actionOptions = (action: SelectionAction, options: BrowserQueryActionsOptions): SelectionActionOptions => action === 'delete'
  ? {action, options: {deleteFiles: true}} : {action, options: {deviceId: options.deviceId, quality: 'original'}};

export function browserQueryActionOffered<C>(target: QuerySelectionTarget<C>, action: SelectionAction, options?: BrowserQueryActionsOptions): boolean {
  if (!options || options.signal.aborted || !options.isCurrent() || action === 'delete' && !options.owner) return false;
  try {selectionActionTargetInput(target, actionOptions(action, options)); return true;} catch {return false;}
}

/** Bind both actual query actions to the model-issued operation and the original live authority. */
export function createBrowserQueryActions<C>(host: Host<C>, options: BrowserQueryActionsOptions) {
  const model = new BrowserSelectionActionModel(options);
  let active: Readonly<{operation: SelectionOperation<C>; action: SelectionActionOptions; source: Parameters<typeof model.open>[2]}> | undefined;
  const close = () => {active = undefined; model.close();};
  const open = (action: SelectionAction) => {
    const operation = host.captureOperation?.();
    if (!operation || operation.target.mode !== 'query' || !host.isOperationCurrent?.(operation) || !browserQueryActionOffered(operation.target, action, options)) throw new Error('query-action-unavailable');
    const originalSource = options.sourceIdentity?.();
    const source = {isCurrent: () => !options.signal.aborted && options.isCurrent() && active?.operation === operation && host.isOperationCurrent?.(operation) === true
      && (!options.sourceIdentity || options.sourceIdentity() === originalSource),
      subscribe: (changed: () => void) => {const a = host.subscribe(changed), b = options.subscribe(changed); return () => {a(); b();};}, signal: options.signal};
    active = Object.freeze({operation, action: actionOptions(action, options), source});
    void model.open(operation.target, active.action, source);
  };
  const changeOptions = (value: Readonly<{deleteFiles: boolean}> | Readonly<{quality: string}>) => {
    const original = active;
    if (!original || original.operation.target.mode !== 'query' || !original.source.isCurrent()) return;
    const action: SelectionActionOptions = original.action.action === 'delete' && 'deleteFiles' in value
      ? {action: 'delete', options: value} : original.action.action === 'download' && 'quality' in value
        ? {action: 'download', options: {...original.action.options, quality: value.quality}} : original.action;
    active = Object.freeze({...original, action});
    void model.open(original.operation.target, action, original.source);
  };
  function Surface() {
    useSyncExternalStore(model.controller.subscribe, model.controller.getSnapshot, model.controller.getSnapshot);
    useEffect(() => close, []);
    return <SelectionActionSurface model={model} onClose={close} onOptionsChange={changeOptions}/>;
  }
  return {model, offered: (target: QuerySelectionTarget<C>, action: SelectionAction) => browserQueryActionOffered(target, action, options),
    openDelete: () => open('delete'), openDownload: () => open('download'), surface: <Surface/>, dispose: () => {active = undefined; model.dispose();}};
}
