import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { StatusDashboard } from './StatusDashboard';
import type { SettingsDataSource, SettingsStatusSnapshot, SettingsViewer } from './settingsTypes';

type TelemetryStatus = { status: string; reason: string };
type ActivityFixture = NonNullable<SettingsStatusSnapshot['activity']> & { cpuStatus: TelemetryStatus; memoryStatus: TelemetryStatus };
type GPUFixture = NonNullable<NonNullable<NonNullable<SettingsStatusSnapshot['dashboard']>['system']>['gpu']>[number] & { usageStatus: TelemetryStatus; memoryStatus: TelemetryStatus; encoderStatus: TelemetryStatus; headroomStatus: TelemetryStatus };

const viewer: SettingsViewer = {
  id: 'telemetry-viewer',
  displayName: 'Telemetry Viewer',
  email: 'telemetry@example.test',
  role: 'owner',
  serverName: 'Telemetry Test',
};

function activity(status: string, reason: string, values: { cpuPercent: number; memoryUsedBytes: number; memoryTotalBytes: number }): ActivityFixture {
  return {
    serverName: 'Telemetry Test',
    activeStreams: 0,
    activeTranscodes: 0,
    bandwidthMbps: 0,
    cpuPercent: values.cpuPercent,
    cpuStatus: { status, reason },
    generatedAt: '2026-08-17T00:00:00.000Z',
    memoryFreeBytes: Math.max(values.memoryTotalBytes - values.memoryUsedBytes, 0),
    memoryTotalBytes: values.memoryTotalBytes,
    memoryUsedBytes: values.memoryUsedBytes,
    memoryStatus: { status, reason },
    refreshAfterMs: 5000,
  };
}

function gpu(status: string, reason: string, value: number): GPUFixture {
  const metricStatus = { status, reason };
  return {
    label: 'Test GPU',
    usage: value,
    usageStatus: metricStatus,
    memory: value,
    memoryStatus: metricStatus,
    encoder: value,
    encoderStatus: metricStatus,
    headroom: value,
    headroomStatus: metricStatus,
  };
}

function snapshot(status: string, reason: string, value: number): SettingsStatusSnapshot {
  return {
    generatedAt: '2026-08-17T00:00:00.000Z',
    activity: activity(status, reason, { cpuPercent: value, memoryUsedBytes: value, memoryTotalBytes: value || 1024 }),
    dashboard: {
      system: {
        cpu: [],
        ram: [],
        gpu: [gpu(status, reason, value)],
        diskIo: [],
        gpuInfo: { available: true, device: 'Test GPU', provider: 'Test' },
      },
    } as unknown as NonNullable<SettingsStatusSnapshot['dashboard']>,
  };
}

function sourceFor(value: SettingsStatusSnapshot): SettingsDataSource {
  return { settingsStatus: vi.fn().mockResolvedValue(value) } as unknown as SettingsDataSource;
}

describe('StatusDashboard telemetry presentation', () => {
  it('describes unavailable GPU telemetry without package-install instructions', async () => {
    const value = snapshot('unsupported', 'GPU hardware or driver telemetry is not exposed to this server.', 0);
    if (value.dashboard) {
      value.dashboard.system.gpuInfo = { available: false, device: 'GPU telemetry unavailable', provider: 'Unknown' };
    }
    render(<MemoryRouter><StatusDashboard source={sourceFor(value)} viewer={viewer} /></MemoryRouter>);

    const gpuLedger = await screen.findByRole('region', { name: 'GPU telemetry' });
    expect(gpuLedger).toHaveTextContent('GPU hardware or driver telemetry is not exposed to this server.');
    expect(gpuLedger).not.toHaveTextContent(/install|nvidia-smi|rocm-smi|amd-smi|intel_gpu_top|privileged/i);
  });

  it('does not turn ordinary snapshot age into a user-facing health warning', async () => {
    render(<MemoryRouter><StatusDashboard source={sourceFor(snapshot('ok', 'Authoritative sample.', 10))} viewer={viewer} /></MemoryRouter>);

    expect(await screen.findByText('Telemetry Test status')).toBeInTheDocument();
    expect(screen.queryByText(/stale snapshot/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/refresh for current values/i)).not.toBeInTheDocument();
  });

  it('does not claim the server is online when a status panel failed', async () => {
    const value = snapshot('ok', 'Authoritative sample.', 10);
    value.failures = { remoteAccess: 'Remote access status timed out.' };
    render(<MemoryRouter><StatusDashboard source={sourceFor(value)} viewer={viewer} /></MemoryRouter>);

    expect(await screen.findByText('Telemetry Test status is partially unavailable')).toBeInTheDocument();
    expect(screen.queryByText(/is online/i)).not.toBeInTheDocument();
  });

  it('presents the authoritative direct-access result without implementation diagnostics', async () => {
    const value = snapshot('ok', 'Authoritative sample.', 10);
    value.remoteAccess = {
      settings: { enabled: true, claimStatus: 'claimed' },
      publicEndpoint: { url: 'https://direct.example.test:32500', port: 32500 },
      connectivity: {
        hostedServicesStatus: 'reachable',
        publicRouteStatus: 'public_reachable',
        troubleshootingStatus: 'ok',
      },
    } as unknown as NonNullable<SettingsStatusSnapshot['remoteAccess']>;
    render(<MemoryRouter><StatusDashboard source={sourceFor(value)} viewer={viewer} /></MemoryRouter>);

    const connectivity = await screen.findByRole('heading', { name: 'Connectivity' });
    const region = connectivity.closest('section')!;
    expect(within(region).getByText('Direct access')).toBeInTheDocument();
    expect(within(region).getByText('Available')).toBeInTheDocument();
    expect(within(region).getByText('https://direct.example.test:32500')).toBeInTheDocument();
    expect(within(region).queryByText('TLS certificate')).not.toBeInTheDocument();
    expect(within(region).queryByText('Router mapping')).not.toBeInTheDocument();
    expect(within(region).queryByText('Hosted control plane')).not.toBeInTheDocument();
  });

  it('shows only active work with concise progress and no scheduler internals', async () => {
    const value = snapshot('ok', 'Authoritative sample.', 10);
    const base = { createdAt: '2026-08-17T00:00:00.000Z', updatedAt: '2026-08-17T00:01:00.000Z', attemptCount: 1, message: '' };
    value.jobs = [
      { ...base, id: 'active', type: 'metadata_refresh_library', status: 'running', progress: 25, progressCurrent: 1, progressTotal: 4, message: 'Refreshing The Example Movie (1/4).', metadata: { libraryName: 'Movies' }, priority: 'background-media' },
      { ...base, id: 'complete', type: 'library_scan', status: 'complete', progress: 100, message: 'Filesystem change check completed for Movies.', metadata: { libraryName: 'Movies' } },
    ];
    render(<MemoryRouter><StatusDashboard source={sourceFor(value)} viewer={viewer} /></MemoryRouter>);

    const work = (await screen.findByRole('heading', { name: 'Work' })).closest('section')!;
    expect(within(work).getByText('Refreshing metadata for Movies · 25%')).toBeInTheDocument();
    expect(within(work).getByText('Refreshing The Example Movie (1/4)')).toBeInTheDocument();
    expect(within(work).queryByText(/Filesystem change check completed/)).not.toBeInTheDocument();
    expect(within(work).queryByText(/Progress unavailable|background-media|complete ·/i)).not.toBeInTheDocument();
  });

  it.each([
    { status: 'unavailable', label: 'Unavailable', reason: 'CPU collector did not return a sample.', value: 0 },
    { status: 'warming_up', label: 'Warming up', reason: 'Waiting for the first telemetry sample.', value: 0 },
    { status: 'unsupported', label: 'Unsupported', reason: 'This host has no supported telemetry collector.', value: 0 },
    { status: 'ok', label: 'OK', reason: 'Authoritative sample.', value: 0 },
  ])('renders the $status state with its reason and gates zero-valued metrics', async ({ status, label, reason, value }) => {
    render(<MemoryRouter><StatusDashboard source={sourceFor(snapshot(status, reason, value))} viewer={viewer} /></MemoryRouter>);

    const cpu = await screen.findByTestId('server-resource-cpu');
    const memory = screen.getByTestId('server-resource-memory');
    const gpuLedger = screen.getByRole('region', { name: 'GPU telemetry' });
    expect(within(cpu).getByText(status === 'ok' ? '0%' : label)).toBeInTheDocument();
    expect(within(memory).getByText(status === 'ok' ? '0 B / 1.00 KB' : label)).toBeInTheDocument();
    expect(within(gpuLedger).getAllByText(label).length).toBeGreaterThan(0);

    if (status === 'ok') {
      expect(within(cpu).getByText('0 active transcodes')).toBeInTheDocument();
      expect(within(gpuLedger).getAllByText('0%').length).toBeGreaterThanOrEqual(4);
    } else {
      expect(within(cpu).getByText(reason)).toBeInTheDocument();
      expect(within(memory).getByText(reason)).toBeInTheDocument();
      expect(within(gpuLedger).getAllByText(reason).length).toBeGreaterThanOrEqual(4);
      expect(screen.queryByText('0%')).not.toBeInTheDocument();
      expect(screen.queryByText('0 B / 1.00 KB')).not.toBeInTheDocument();
    }
  });
});
