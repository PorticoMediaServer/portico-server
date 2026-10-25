import {useState, type ReactNode} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {ReportTransport} from '@core/client-reports.ts';
import {FeedbackPanel} from './Feedback';
import {ReportsPanel} from './Reports';
import {SupportPanel} from './Support';
import {SettingsFeedbackSource} from './feedback-source';
import {SettingsSupportSource} from './support-source';
import {SettingsReportsSource} from './reports-source';
import styles from './troubleshooting.module.css';

const t = defaultI18n.t;

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

type View = 'feedback' | 'reports' | 'support';
const TABS: readonly Readonly<{id: View; label: MessageId}>[] = Object.freeze([
  {id: 'feedback', label: 'next.settings.troubleshootingFeedback'},
  {id: 'reports', label: 'next.settings.troubleshootingReports'},
  {id: 'support', label: 'server.support'},
]);

/**
 * Owner-only Troubleshooting. The frames draw a page with four tabs; this pass draws
 * Feedback (frames 3-4) and Reports (frames 5-6), so only those two tabs are offered.
 * Messages and Support are not built yet and are not drawn as dead tabs.
 */
export type TroubleshootingPageProps = Readonly<{
  reports: SettingsReportsSource; feedback: SettingsFeedbackSource; support: SettingsSupportSource;
  subscribe: (listener: () => void) => () => void; isCurrent: () => boolean; downloadBase: string; reportError: (error: unknown) => void;
  /** Which built screen opens first; the drawn page opens on Messages, which is not built yet. */
  initialView?: View;
}>;

/** The drawn tab strip, offering only the screens this pass actually draws. */
export function TroubleshootingPage({reports, feedback, support, subscribe, isCurrent, downloadBase, reportError, initialView = 'feedback'}: TroubleshootingPageProps) {
  const [view, setView] = useState<View>(initialView);
  return <div className={styles.page}>
    <div className={styles.tabs} role="tablist">{TABS.map(tab => <button key={tab.id} type="button" role="tab" aria-selected={tab.id === view}
      className={styles.tab} onClick={() => setView(tab.id)}>{t(tab.label)}</button>)}</div>
    {view === 'feedback' ? <FeedbackPanel source={feedback} subscribe={subscribe} reportError={reportError} />
      : view === 'reports' ? <ReportsPanel source={reports} subscribe={subscribe} isCurrent={isCurrent} downloadBase={downloadBase} reportError={reportError} />
        : <SupportPanel source={support} subscribe={subscribe} reportError={reportError} />}
  </div>;
}

export function createTroubleshootingSurfaces(options: TroubleshootingSurfacesOptions): TroubleshootingSurfaces {
  const reports = new SettingsReportsSource(options.transport, options.serverId, options.viewerId, options.isCurrent);
  const feedback = new SettingsFeedbackSource(options.transport, options.serverId, options.viewerId, options.isCurrent);
  const support = new SettingsSupportSource(options.transport, options.serverId, options.viewerId, options.isCurrent);
  const subscribe = (listener: () => void) => {
    const offs = [options.subscribe(listener), reports.subscribe(listener), feedback.subscribe(listener), support.subscribe(listener)];
    return () => {for (const off of offs) off();};
  };
  return {
    render: () => options.isCurrent() ? <TroubleshootingPage reports={reports} feedback={feedback} support={support} subscribe={subscribe} isCurrent={options.isCurrent}
      downloadBase={options.downloadBase} reportError={options.reportError} /> : undefined,
    dispose: () => {reports.dispose();feedback.dispose();support.dispose();},
  };
}
