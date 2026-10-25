import type {LogDetailMinutes} from '@core/server-administration.ts';
import {useState, type ReactNode} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {ReportTransport} from '@core/client-reports.ts';
import {FeedbackPanel} from './Feedback';
import {ReportsPanel} from './Reports';
import {SupportPanel} from './Support';
import {MessagesPanel} from './Messages';
import {SettingsMessagesSource, type MessageConnection} from './messages-source';
import {SettingsFeedbackSource} from './feedback-source';
import {SettingsSupportSource} from './support-source';
import {SettingsReportsSource} from './reports-source';
import styles from './troubleshooting.module.css';

const t = defaultI18n.t;

export type TroubleshootingSurfacesOptions = Readonly<{
  transport: ReportTransport;
  messageConnection?: MessageConnection;
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
  render: (options?: ReactNode, detailMinutes?: LogDetailMinutes) => ReactNode;
  dispose: () => void;
}>;

type View = 'messages' | 'feedback' | 'reports' | 'support';
const TABS: readonly Readonly<{id: View; label: MessageId}>[] = Object.freeze([
  {id: 'messages', label: 'web.logs.messages'},
  {id: 'feedback', label: 'next.settings.troubleshootingFeedback'},
  {id: 'reports', label: 'next.settings.troubleshootingReports'},
  {id: 'support', label: 'server.support'},
]);

/** Owner-scoped Troubleshooting with the four drawn, working tabs. */
export type TroubleshootingPageProps = Readonly<{
  messages: SettingsMessagesSource; reports: SettingsReportsSource; feedback: SettingsFeedbackSource; support: SettingsSupportSource;
  subscribe: (listener: () => void) => () => void; isCurrent: () => boolean; downloadBase: string; reportError: (error: unknown) => void;
  /** Which built screen opens first; the drawn page opens on Messages. */
  initialView?: View;
  options?: ReactNode;
  detailMinutes?: LogDetailMinutes;
}>;

/** The drawn tab strip, offering only the screens this pass actually draws. */
export function TroubleshootingPage({messages, reports, feedback, support, subscribe, isCurrent, downloadBase, reportError, options, detailMinutes, initialView = 'messages'}: TroubleshootingPageProps) {
  const [view, setView] = useState<View>(initialView);
  return <div className={styles.page} data-view={view}>
    <div className={styles.tabs} role="tablist">{TABS.map(tab => <button key={tab.id} type="button" role="tab" aria-selected={tab.id === view}
      className={styles.tab} onClick={() => setView(tab.id)}>{t(tab.label)}</button>)}</div>
    {view === 'messages' ? <MessagesPanel source={messages} options={options} detailMinutes={detailMinutes} reportError={reportError} />
      : view === 'feedback' ? <FeedbackPanel source={feedback} subscribe={subscribe} reportError={reportError} />
      : view === 'reports' ? <ReportsPanel source={reports} subscribe={subscribe} isCurrent={isCurrent} downloadBase={downloadBase} reportError={reportError} />
        : <SupportPanel source={support} subscribe={subscribe} reportError={reportError} />}
  </div>;
}

export function createTroubleshootingSurfaces(options: TroubleshootingSurfacesOptions): TroubleshootingSurfaces {
  const messages = new SettingsMessagesSource(options.transport, options.messageConnection, options.isCurrent, options.reportError);
  const reports = new SettingsReportsSource(options.transport, options.serverId, options.viewerId, options.isCurrent);
  const feedback = new SettingsFeedbackSource(options.transport, options.serverId, options.viewerId, options.isCurrent);
  const support = new SettingsSupportSource(options.transport, options.serverId, options.viewerId, options.isCurrent, options.reportError);
  const subscribe = (listener: () => void) => {
    const offs = [options.subscribe(listener), messages.subscribe(listener), reports.subscribe(listener), feedback.subscribe(listener), support.subscribe(listener)];
    return () => {for (const off of offs) off();};
  };
  return {
    render: (messageOptions, detailMinutes) => options.isCurrent() ? <TroubleshootingPage messages={messages} reports={reports} feedback={feedback} support={support} subscribe={subscribe} isCurrent={options.isCurrent}
      downloadBase={options.downloadBase} reportError={options.reportError} options={messageOptions} detailMinutes={detailMinutes} /> : undefined,
    dispose: () => {messages.dispose();reports.dispose();feedback.dispose();support.dispose();},
  };
}
