import {useMemo, useState} from 'react';
import {captureReportContext} from '@core/report-context.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {useI18n} from '../../app/i18n';

/** One canonical calendar capture per viewer, clock and explicit refresh. */
export function useReportContext() {
  const {api} = useSession();
  const scope = useViewerScope();
  const {region} = useI18n();
  const [revision, setRevision] = useState(0);
  const context = useMemo(() => captureReportContext(region.timeZone), [api, scope, region.timeZone, revision]);
  return {context, refresh: () => setRevision(value => value + 1)};
}
