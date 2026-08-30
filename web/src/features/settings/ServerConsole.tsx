import type { LogEvent, SystemDiagnostics, SystemReleaseInfo } from '@porticomediaserver/client-core';
import { StatusWarningIcon, StatusSuccessIcon, ViewListIcon, DeviceStorageIcon, ActionRefreshIcon, NavigationSearchIcon, DeviceServerIcon } from '#portico-icons';
import { useCallback, useMemo, useState } from 'react';
import { SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { ChoiceControl, InlineNotice, SettingsError, SettingsGroup, SettingsLoading } from './SettingsControls';
import { useSettingsQuery } from './settingsHooks';
import type { SettingsDataSource } from './settingsTypes';

function bytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = Math.max(0, value);
  let index = 0;
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1; }
  return `${size >= 10 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

function logText(event: LogEvent): string {
  const fields = Object.entries(event.fields ?? {}).map(([key, value]) => `${key}=${value}`).join(' ');
  return `${event.time} ${event.level.toUpperCase()} ${event.message}${fields ? ` ${fields}` : ''}`;
}

function diagnosticsFreshness(value: string): { label: string; stale: boolean } {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return { label: 'Freshness unavailable', stale: true };
  const age = Math.max(0, Date.now() - timestamp);
  return {
    label: `${Math.floor(age / 60_000)}m ago · generated ${new Date(timestamp).toLocaleTimeString()}`,
    stale: age > 5 * 60_000,
  };
}

export function ServerConsole({ source, diagnostics, release }: { source: SettingsDataSource; diagnostics: SystemDiagnostics; release?: SystemReleaseInfo }) {
  const [revision, setRevision] = useState(0);
  const [level, setLevel] = useState<'all' | LogEvent['level']>('all');
  const [query, setQuery] = useState('');
  const [copyState, setCopyState] = useState('');
  const loadLogs = useCallback((next: SettingsDataSource, signal: AbortSignal) => next.logs({ limit: 250 }, signal), []);
  const logs = useSettingsQuery(loadLogs, source, revision);
  const filtered = useMemo(() => logs.status === 'success' ? logs.data.items.filter((event) => {
    if (level !== 'all' && event.level !== level) return false;
    const needle = query.trim().toLocaleLowerCase();
    return !needle || logText(event).toLocaleLowerCase().includes(needle);
  }) : [], [level, logs, query]);
  const diagnosticFreshness = diagnosticsFreshness(diagnostics.generatedAt);

  const copy = async () => {
    setCopyState('');
    try {
      await navigator.clipboard.writeText(filtered.map(logText).join('\n'));
      setCopyState(`${filtered.length} ${filtered.length === 1 ? 'event' : 'events'} copied.`);
    } catch {
      setCopyState('Clipboard access was denied by this browser.');
    }
  };

  return <div className="portico-server-console">
    <p className={`portico-status-freshness${diagnosticFreshness.stale ? ' stale' : ''}`} data-testid="diagnostics-freshness"><strong>{diagnosticFreshness.stale ? 'Stale diagnostics' : 'Current diagnostics'}</strong><span>{diagnosticFreshness.label}</span>{diagnosticFreshness.stale && <span>Refresh the Diagnostics section for current values.</span>}</p>
    <SettingsGroup title="Server health" description="Runtime, database, and packaged dependency checks from this server.">
      <div className="portico-diagnostic-summary">
        <div><span className={diagnostics.databaseReady ? 'healthy' : 'danger'}>{diagnostics.databaseReady ? <StatusSuccessIcon /> : <StatusWarningIcon />}</span><span><strong>Storage</strong><small>{diagnostics.databaseReady ? `${diagnostics.sqlite.journalMode} · ${bytes(diagnostics.sqlite.databaseBytes)}` : diagnostics.sqlite.lastError || 'Storage is not ready'}</small></span></div>
        <div><span className={diagnostics.webDistReady ? 'healthy' : 'danger'}>{diagnostics.webDistReady ? <StatusSuccessIcon /> : <StatusWarningIcon />}</span><span><strong>Web application</strong><small>{diagnostics.webDistReady ? (release ? `Portico ${release.version} · API ${release.apiVersion}` : 'Ready · release details unavailable') : 'The packaged web distribution is unavailable'}</small></span></div>
        <div><span className={diagnostics.resources.status === 'normal' ? 'healthy' : 'warn'}>{diagnostics.resources.status === 'normal' ? <StatusSuccessIcon /> : <StatusWarningIcon />}</span><span><strong>Workload</strong><small>{diagnostics.resources.status} · {diagnostics.resources.runningBackgroundJobs} running · {diagnostics.resources.queuedBackgroundJobs} queued</small></span></div>
        <div><span className={diagnostics.sqliteHealth.status === 'healthy' ? 'healthy' : 'danger'}>{diagnostics.sqliteHealth.status === 'healthy' ? <DeviceStorageIcon /> : <StatusWarningIcon />}</span><span><strong>SQLite health</strong><small>{diagnostics.sqliteHealth.status} · probe {diagnostics.sqliteHealth.lastProbeDurationMillis} ms</small></span></div>
      </div>
      <div className="portico-dependency-list"><div><span className={diagnostics.mediaToolchain.verified ? 'healthy' : 'warn'}>{diagnostics.mediaToolchain.verified ? <StatusSuccessIcon /> : <StatusWarningIcon />}</span><span><strong>Media toolchain</strong><small>{diagnostics.mediaToolchain.verified ? `${diagnostics.mediaToolchain.buildId} · ${diagnostics.mediaToolchain.target}` : `${diagnostics.mediaToolchain.status} · ${diagnostics.mediaToolchain.reasonCode}`}</small></span></div>{diagnostics.dependencies.map((dependency) => <div key={dependency.name}><span className={dependency.available ? 'healthy' : 'danger'}>{dependency.available ? <StatusSuccessIcon /> : <StatusWarningIcon />}</span><span><strong>{dependency.name}</strong><small>{dependency.available ? dependency.versionLine || dependency.resolvedPath || 'Available' : dependency.error || `Not found at ${dependency.configuredPath}`}</small></span></div>)}</div>
    </SettingsGroup>

    <section className="portico-console-panel">
      <header><div><DeviceServerIcon /><span><h2>Server console</h2><p>Recent server events. Sensitive values are redacted by the server.</p></span></div><div><label className="portico-console-search"><NavigationSearchIcon /><input aria-label="Filter server logs" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter events" /></label><ChoiceControl label="Level" value={level} options={['all', 'debug', 'info', 'warn', 'error'].map((value) => ({ value, label: value === 'all' ? 'All levels' : value[0].toUpperCase() + value.slice(1) }))} onChange={(value) => setLevel(value as typeof level)} /><SecondaryButton onClick={() => setRevision((current) => current + 1)}><ActionRefreshIcon /> Refresh</SecondaryButton><SecondaryButton disabled={filtered.length === 0} onClick={() => void copy()}><ViewListIcon /> Copy visible</SecondaryButton></div></header>
      {copyState && <InlineNotice tone={copyState.includes('denied') ? 'warn' : 'success'}>{copyState}</InlineNotice>}
      {logs.status === 'loading' && <SettingsLoading label="Loading server events" />}
      {logs.status === 'error' && <SettingsError title="Server events are unavailable" message={reviewedProductErrorText(logs.error, 'settings.load-failed', { sectionName: 'Server events' })} onRetry={() => setRevision((current) => current + 1)} />}
      {logs.status === 'success' && filtered.length === 0 && <div className="portico-settings-state"><NavigationSearchIcon /><strong>No matching events</strong><p>Adjust the level or text filter to see other server events.</p></div>}
      {logs.status === 'success' && filtered.length > 0 && <div className="portico-console-events" role="log" aria-label="Server events">{filtered.map((event) => <article className={event.level} key={event.id}><time dateTime={event.time}>{new Date(event.time).toLocaleString()}</time><strong>{event.level}</strong><p>{event.message}</p>{Object.keys(event.fields ?? {}).length > 0 && <dl>{Object.entries(event.fields ?? {}).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>}</article>)}</div>}
    </section>

    {release ? <SettingsGroup title="Release" description="Installed server and runtime information.">
      <div className="portico-release-grid"><div><DeviceServerIcon /><span><strong>Portico {release.version}</strong><small>{release.installMethod} · {release.goos}/{release.goarch}</small></span></div><div><ActionRefreshIcon /><span><strong>Update path</strong><small>Unavailable for this installation · state {release.updateStatus}</small></span></div><div><DeviceStorageIcon /><span><strong>Storage migration</strong><small>{release.migrationStatus}</small></span></div></div>
    </SettingsGroup> : <SettingsGroup title="Release" description="Installed server and runtime information."><div className="portico-settings-state error"><StatusWarningIcon /><strong>Release information is unavailable</strong><p>Diagnostics remain available. Retry to refresh this independent panel.</p></div></SettingsGroup>}
  </div>;
}
