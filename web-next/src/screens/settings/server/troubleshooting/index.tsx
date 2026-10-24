import type {ReactNode} from 'react';
import type {ReportTransport} from '@core/client-reports.ts';
import {ReportsPanel} from './Reports';
import {SettingsReportsSource} from './reports-source';

export type TroubleshootingSurfacesOptions = Readonly<{
  transport: ReportTransport;
  serverId: string;
  /** The captured viewer fence, the same one the Settings controller uses. */
  viewerId: string;
  /** The selected server's own origin, for downloads it owns. */
  downloadBase: string;
  isCurrent: () => boolean;
  subscribe: (listener: () => void) => () => void;
  reportError: (error: unknown) => void;
}>;

export type TroubleshootingSurfaces = Readonly<{
  /** The drawn Troubleshooting page for the selected server, or nothing when this scope ended. */
  render: () => ReactNode;
  dispose: () => void;
}>;

/**
 * Owner-only Troubleshooting. The frames draw a page with four tabs; this pass
 * draws Reports (frames 5-6), whose panels and dialog come from real producers.
 * Messages, Feedback and Support are not drawn yet, so their tabs are not offered.
 */
export function createTroubleshootingSurfaces(options: TroubleshootingSurfacesOptions): TroubleshootingSurfaces {
  const reports = new SettingsReportsSource(options.transport, options.serverId, options.viewerId, options.isCurrent);
  return {
    render: () => options.isCurrent() ? <ReportsPanel source={reports} subscribe={options.subscribe} isCurrent={options.isCurrent}
      downloadBase={options.downloadBase} reportError={options.reportError} /> : undefined,
    dispose: () => reports.dispose(),
  };
}
