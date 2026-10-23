import {useMemo, useRef} from 'react';
import {captureReportContext} from '@core/report-context.ts';
import {useI18n} from '../../app/i18n';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';

/** A new reader for a different server/viewer/zone; filters share this captured interval. */
export function useReportView() {
  const {api} = useSession();
  const viewer = useViewerScope();
  const {region} = useI18n();
  // An unspecified resolved region follows the runtime zone, just like the I18n formatters.
  const timeZone = region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const serial = useRef(0);
  return useMemo(() => ({key: ++serial.current, api, viewer, context: {current: captureReportContext(timeZone)}}), [api, viewer, timeZone]);
}
