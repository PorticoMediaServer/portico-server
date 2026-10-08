import type {SettingsCustomId} from '@core/presentation/index.ts';
import {isStatePermissionsAlert} from '@core/administration.ts';
import {useConsole, useRead} from '../../admin/console';
import {NowPlayingPanel} from '../server/NowPlaying';
import {AlertsPanel, HealthPanel} from '../server/Overview';
import {JobsPanel} from '../server/Activity';
import {ViewingPanel} from '../server/OverviewCharts';
import {LibrariesPage} from '../server/Libraries';
import {LiveSourcesPanel, RecordingRulesPanel, RecordingStoragePanel} from '../server/LiveTV';
import {TunersGroup} from '../server/DVRSettings';
import {LibraryChannelsPanel} from '../server/Channels';
import {AccountsPanel} from '../server/People';
import {APIKeysPanel, InvitationsPanel} from '../server/Access';
import {TranscodeStatusPanel} from '../server/Transcoding';
import {AddressesRow, CertificateStatusRow, RemoteStatusPanel} from '../server/Network';
import {AccountConnectionPanel, IdentityRows} from '../server/General';
import {DeletedTitlesPanel, DiskUsagePanel, UpdatesPanel} from '../server/Maintenance';
import {RemoteSourcesPanel} from '../server/Storage';
import {BackupsGroup} from '../server/Backups';
import {RetentionRows, ScheduledJobsPanel, WindowsPanel} from '../server/MaintenanceWindows';
import {CapabilitiesGroup, DetailWindowRow, MessageLogGroup} from '../server/Diagnostics';
import {RecordsPanel, SupportExportPanel} from '../server/Logs';
import {FeedbackPanel} from '../server/Feedback';
import {PlayHistoryPanel} from '../server/PlayHistory';
import {StatePermissionsCard} from '../server/StatePermissions';

/** Panels that are rows inside their section's group; every other panel brings its own surface and heading. */
const ROW_PANELS: ReadonlySet<SettingsCustomId> = new Set(['server.certificateStatus', 'server.addresses', 'server.retention', 'server.detailWindow', 'server.identity']);
export const panelIsGroup = (id: SettingsCustomId) => !ROW_PANELS.has(id);

/** The Server pages' `custom` rows (settings-structure.ts), drawn with the web's own panels. */
export function ServerPanel({id}: {id: SettingsCustomId}) {
  switch (id) {
    case 'server.nowPlaying': return <NowPlayingPanel />;
    case 'server.alerts': return <AlertsPanel />;
    case 'server.health': return <HealthPanel />;
    case 'server.activity': return <JobsPanel />;
    case 'server.statistics': return <ViewingPanel />;
    case 'server.playHistory': return <PlayHistoryPanel />;
    case 'server.libraries': return <LibrariesPage />;
    case 'server.liveSources': return <LiveSourcesPanel />;
    case 'server.tuners': return <TunersGroup />;
    case 'server.recordingRules': return <RecordingRulesPanel />;
    case 'server.recordingStorage': return <RecordingStoragePanel />;
    case 'server.channels': return <LibraryChannelsPanel />;
    case 'server.accounts': return <AccountsPanel />;
    case 'server.invitations': return <InvitationsPanel />;
    case 'server.apiKeys': return <APIKeysPanel />;
    case 'server.transcodeStatus': return <TranscodeStatusPanel />;
    case 'server.remoteStatus': return <RemoteStatusPanel />;
    case 'server.accountConnection': return <AccountConnectionPanel />;
    case 'server.certificateStatus': return <CertificateStatusRow />;
    case 'server.addresses': return <AddressesRow />;
    case 'server.diskUsage': return <DiskUsagePanel />;
    case 'server.retention': return <RetentionRows />;
    case 'server.remoteSources': return <RemoteSourcesPanel />;
    case 'server.backups': return <BackupsGroup />;
    case 'server.deletedTitles': return <DeletedTitlesPanel />;
    case 'server.windows': return <WindowsPanel />;
    case 'server.jobs': return <ScheduledJobsPanel />;
    case 'server.detailWindow': return <DetailWindowRow />;
    case 'server.logs': return <MessageLogGroup />;
    case 'server.records': return <RecordsPanel />;
    case 'server.capabilities': return <CapabilitiesGroup />;
    case 'server.supportExport': return <SupportExportPanel />;
    case 'server.feedback': return <FeedbackPanel />;
    case 'server.stateFolder': return <StateFolder />;
    case 'server.identity': return <IdentityRows />;
    case 'server.updates': return <UpdatesPanel />;
    default: return null;
  }
}

/** The state-folder warning and its owner-only fix; present only while the server reports it. */
function StateFolder() {
  const client = useConsole();
  const read = useRead(() => client.alerts(), [client]);
  const alert = read.data?.find(a => a.status !== 'resolved' && isStatePermissionsAlert(a));
  return alert ? <StatePermissionsCard alert={alert} /> : null;
}
