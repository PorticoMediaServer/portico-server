import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {ActivitySummaryService, type ActivitySummarySource, type ActivitySummarySourceStore} from '@core/admin/activity-summary/index.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import type {TimeEnvironment} from '@core/presentation/kit-model/index.ts';
import {TopBarActivity, type ActivityPanelView} from '../kit/shell';
import type {ShellAssemblyContext} from './bootstrap';
import {activityPanelView} from './activity-model';

/** The component itself is mounted only for the desktop owner. No other role constructs a client. */
export function createOwnerActivity(context: ShellAssemblyContext, time: TimeEnvironment, onDashboard: ActivityPanelView['onDashboard'], openUpdate: () => void) {
  const mounted = new Set<ActivitySummaryService>();
  let retired = false;
  const sourceListeners = new Set<() => void>();
  const session = context.session();
  const value: ActivitySummarySource = {http: localV1Http(context.api), scope: {...session.viewer, sessionFamilyId: session.sessionFamilyId, authorizationEpoch: String(context.epoch)}};
  const source: ActivitySummarySourceStore = {
    getSnapshot: () => !retired && context.isCurrent() ? value : null,
    subscribe: listener => { sourceListeners.add(listener); return () => { sourceListeners.delete(listener); }; },
  };
  function OwnerActivity() {
    const service = useMemo(() => new ActivitySummaryService({source}), []);
    const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
    const [open, setOpen] = useState(false);
    useEffect(() => {
      if (retired || !context.isCurrent()) return;
      mounted.add(service); service.setActive(true); service.attach(context.events); void service.load();
      return () => { mounted.delete(service); service.setActive(false); service.dispose(); };
    }, [service]);
    const retry = () => { void service.retry(); };
    const view = activityPanelView(snapshot, context.facts.serverName, time, {onDashboard, onRetry: retry, openUpdate});
    return <TopBarActivity viewerRole="owner" view={view} open={open} onOpenChange={setOpen} />;
  }
  return {OwnerActivity, dispose: () => {
    retired = true; for (const listener of sourceListeners) listener();
    for (const service of mounted) service.dispose(); mounted.clear(); sourceListeners.clear();
  }};
}
