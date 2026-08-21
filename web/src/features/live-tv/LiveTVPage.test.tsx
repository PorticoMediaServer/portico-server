import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { App } from '../../App';
import { productText } from '../../components/ProductLanguage';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { ActionableDVRRecording, Viewer } from '../../data/models';

function renderLive(source = new FixturePorticoDataSource(), entry = '/live?tab=guide&source=fixture-live') {
  return render(<DataProvider source={source}>
    <MemoryRouter initialEntries={[entry]}><App /></MemoryRouter>
  </DataProvider>);
}

describe('Live TV production surface', () => {
  it('renders the focused program and keyboard-operable time grid', async () => {
    renderLive();
    expect(await screen.findByRole('heading', { name: 'Live TV' })).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: 'Live programming' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Select News 7' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Watch live' })).toBeInTheDocument();
    const liveProgram = (await screen.findAllByRole('button', { name: /Live programming,/ }))[0];
    const upNext = (await screen.findAllByRole('button', { name: /Up next,/ }))[0];
    liveProgram.focus();
    fireEvent.keyDown(liveProgram, { key: 'ArrowRight' });
    await waitFor(() => expect(upNext).toHaveFocus());
    expect(screen.getByRole('link', { name: 'Library Channels' })).toBeInTheDocument();
  });

  it('advances the current guide program from the Server clock sample', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date('2026-08-17T12:00:00.000Z'));
    try {
      const source = new FixturePorticoDataSource();
      const loadGuide = source.liveTVGuide.bind(source);
      vi.spyOn(source, 'liveTVGuide').mockImplementation(async (...args) => {
        const result = await loadGuide(...args);
        const from = '2026-08-17T12:00:00.000Z';
        const to = '2026-08-17T15:00:00.000Z';
        const programs = result.channels.flatMap((channel) => [
          { ...result.programs[0], id: `${channel.id}-current`, channelId: channel.id, title: 'Current program', startAt: from, endAt: '2026-08-17T12:01:00.000Z' },
          { ...result.programs[1], id: `${channel.id}-next`, channelId: channel.id, title: 'Next program', startAt: '2026-08-17T12:01:00.000Z', endAt: '2026-08-17T12:03:00.000Z' },
        ]);
        return { ...result, from, to, serverTime: from, programs };
      });
      renderLive(source);

      await act(async () => {
        for (let index = 0; index < 8; index += 1) await Promise.resolve();
      });
      const current = screen.getAllByRole('button', { name: /Current program,/ });
      const next = screen.getAllByRole('button', { name: /Next program,/ });
      expect(current[0]).toHaveClass('current');
      expect(next[0]).not.toHaveClass('current');

      await act(async () => {
        await vi.advanceTimersByTimeAsync(120_000);
      });

      expect(current[0]).not.toHaveClass('current');
      expect(next[0]).toHaveClass('current');
    } finally {
      vi.useRealTimers();
    }
  });

  it('omits the DVR section when the authenticated profile lacks the server capability', async () => {
    const viewer: Viewer = {
      authenticated: true,
      setupRequired: false,
      serverName: 'EhlerFlix Test',
      user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'user', permissions: { viewLiveTV: true, playLiveTV: true }, preferences: { sidebarOrder: [] } },
    };
    renderLive(new FixturePorticoDataSource(viewer), '/live?tab=dvr&source=fixture-live');

    expect(await screen.findByRole('link', { name: 'Guide' })).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link', { name: 'DVR' })).not.toBeInTheDocument();
  });

  it('provides channel filtering, recording schedules, and editable rules', async () => {
    renderLive();
    await screen.findByRole('heading', { name: 'Live TV' });
    fireEvent.click(await screen.findByRole('link', { name: 'Channels' }));
    const filter = await screen.findByRole('textbox', { name: 'Filter channels' });
    fireEvent.change(filter, { target: { value: 'Cinema' } });
    expect(await screen.findByText('Cinema North')).toBeInTheDocument();
    expect(screen.queryByText('Coastal Sports')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('link', { name: 'DVR' }));
    expect(await screen.findByText('Saturday Cinema')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Schedule/ }));
    expect(await screen.findByText('Coastal Sports Live')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Rules/ }));
    expect(await screen.findByText('Coastal Hockey')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Pause rule Coastal Hockey' }));
    await waitFor(() => expect(screen.getByText('Paused')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Edit rule Coastal Hockey' }));
    const retention = screen.getByRole('spinbutton', { name: 'Retention' });
    fireEvent.change(retention, { target: { value: '45' } });
    fireEvent.click(screen.getByRole('button', { name: productText('action.save-rule') }));
    await waitFor(() => expect(screen.getByText(/Keep 45 days/)).toBeInTheDocument());
  });

  it('does not infer watch or recording controls without advertised capabilities and actions', async () => {
    const source = new FixturePorticoDataSource();
    const loadGuide = source.liveTVGuide.bind(source);
    vi.spyOn(source, 'liveTVGuide').mockImplementation(async (...args) => {
      const result = await loadGuide(...args);
      return {
        ...result,
        capabilities: { canPlay: false, canFavoriteChannels: false, canScheduleRecordings: false, canManageRecordingRules: false, canManageSources: false },
        channels: result.channels.map((channel) => ({ ...channel, actions: [] })),
        programs: result.programs.map((program) => ({ ...program, actions: [] })),
      };
    });
    renderLive(source);
    await screen.findByRole('heading', { name: 'Live programming' });
    expect(screen.queryByRole('button', { name: 'Watch live' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: productText('action.record-once') })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: productText('action.record-series') })).not.toBeInTheDocument();
  });

  it('presents missing EPG honestly while retaining playable channels', async () => {
    const source = new FixturePorticoDataSource();
    const loadSources = source.liveTVSources.bind(source);
    const loadGuide = source.liveTVGuide.bind(source);
    vi.spyOn(source, 'liveTVSources').mockImplementation(async (signal) => (await loadSources(signal)).map((item) => ({ ...item, programCount: 0 })));
    vi.spyOn(source, 'liveTVGuide').mockImplementation(async (...args) => ({ ...(await loadGuide(...args)), programs: [] }));
    renderLive(source);
    expect(screen.queryByText('Guide data is unavailable')).not.toBeInTheDocument();
    expect((await screen.findAllByText('No schedule data')).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Watch channel' })).toBeInTheDocument();
  });

  it('keeps the channel grid playable when the guide request fails', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'liveTVGuide').mockRejectedValue(new Error('EPG provider unavailable'));
    const channels = vi.spyOn(source, 'liveTVChannels');

    renderLive(source);

    await screen.findByRole('button', { name: 'Watch channel' });
    expect(screen.queryByText('Guide data is unavailable. Channels remain available to watch.')).not.toBeInTheDocument();
    expect(channels).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Watch channel' })).toBeInTheDocument();
    expect((await screen.findAllByText('No schedule data')).length).toBeGreaterThan(0);
    expect(screen.queryByText("Portico couldn't complete this request")).not.toBeInTheDocument();
  });

  it('preserves server conflict details and confirms recording only after success', async () => {
    const source = new FixturePorticoDataSource();
    const loadGuide = source.liveTVGuide.bind(source);
    vi.spyOn(source, 'liveTVGuide').mockImplementation(async (...args) => {
      const result = await loadGuide(...args);
      return {
        ...result,
        capabilities: { ...result.capabilities, canScheduleRecordings: true },
        programs: result.programs.map((program, index) => ({ ...program, actions: index === 0 ? ['live.play', 'dvr.record'] : program.actions })),
      };
    });
    const conflict = Object.assign(new Error('News 7 overlaps another recording on the only available tuner.'), { status: 409 });
    vi.spyOn(source, 'createDVRRecording').mockRejectedValue(conflict);
    renderLive(source);
    fireEvent.click(await screen.findByRole('button', { name: productText('action.record-once') }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Something interrupted the request before it could finish');
    expect(screen.queryByText('Scheduled')).not.toBeInTheDocument();
  });

  it('surfaces viewer-safe DVR conflicts and failed recordings without admin-only operational diagnostics', async () => {
    const source = new FixturePorticoDataSource();
    const loadDvr = source.dvr.bind(source);
    vi.spyOn(source, 'dvr').mockImplementation(async (signal) => {
      const result = await loadDvr(signal);
      const failed: ActionableDVRRecording = {
        ...result.recordings[1],
        id: 'fixture-recording-failed',
        title: 'Evening Report',
        status: 'failed' as const,
        failureCode: 'source_unavailable',
        failureMessageId: 'dvr.recording-failed',
        actions: ['dvr.delete'],
      };
      return { ...result, recordings: [...result.recordings, failed] };
    });
    Object.assign(source, {
      dvrStatus: vi.fn().mockResolvedValue({
        capabilities: { canScheduleRecordings: true, canManageRecordingRules: false, canCreateOwnRules: false, canEditOwnRules: false, canDeleteOwnRules: false, canManageAllRules: false, actions: [] },
        guide: { state: 'stale', lastRefreshedAt: '2026-07-11T01:00:00Z', message: 'The guide has not refreshed in 18 hours.' },
        conflicts: [{ id: 'conflict-1', recordingIds: ['fixture-recording-2'], startsAt: '2026-07-11T22:00:00Z', endsAt: '2026-07-11T23:00:00Z', reason: 'Two recordings require the same tuner.', actions: [] }],
        tuners: [{ id: 'tuner-1', name: 'Living room tuner', state: 'offline' }],
        storage: { usedBytes: 8_000_000_000, availableBytes: 2_000_000_000, forecastDays: 4, state: 'pressure' },
        generatedAt: '2026-07-11T04:00:00Z',
      }),
    });
    renderLive(source, '/live?tab=dvr&source=fixture-live&section=issues');
    expect(await screen.findByText('Recording conflict')).toBeInTheDocument();
    expect(screen.getByText('Evening Report')).toBeInTheDocument();
    expect(screen.queryByText('Guide is stale')).not.toBeInTheDocument();
    expect(screen.queryByText('Living room tuner')).not.toBeInTheDocument();
    expect(screen.queryByText('4 days forecast')).not.toBeInTheDocument();
  });

  it('requires confirmation before deleting a completed recording', async () => {
    const source = new FixturePorticoDataSource();
    renderLive(source, '/live?tab=dvr&source=fixture-live');
    fireEvent.click(await screen.findByRole('button', { name: 'Delete Saturday Cinema' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByRole('heading', { name: 'Delete “Saturday Cinema”?' })).toBeInTheDocument();
    expect(within(dialog).getByText(/stored recording file/)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
    expect(await screen.findByText('Recording deleted.')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText('Saturday Cinema')).not.toBeInTheDocument());
  });

  it('explains that deleting a rule removes future schedules but does not stop running recordings', async () => {
    renderLive(new FixturePorticoDataSource(), '/live?tab=dvr&source=fixture-live&section=rules');

    fireEvent.click(await screen.findByRole('button', { name: 'Delete rule Coastal Hockey' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/removes recordings it scheduled that have not started/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/already in progress continue/i)).toBeInTheDocument();
  });

  it('presents a retained partial recording as incomplete with only its advertised actions', async () => {
    const source = new FixturePorticoDataSource();
    const loadDvr = source.dvr.bind(source);
    vi.spyOn(source, 'dvr').mockImplementation(async (signal) => {
      const result = await loadDvr(signal);
      const incomplete = {
        ...result.recordings[0],
        id: 'fixture-recording-incomplete',
        title: 'Storm-interrupted movie',
        status: 'incomplete',
        sizeBytes: 512_000_000,
        actions: ['dvr.play', 'dvr.delete'],
      } as unknown as ActionableDVRRecording;
      return { ...result, recordings: [...result.recordings, incomplete] };
    });
    renderLive(source, '/live?tab=dvr&source=fixture-live');

    expect(await screen.findByRole('heading', { name: 'Incomplete recordings' })).toBeInTheDocument();
    expect(screen.getByText(/playable portion captured before recording stopped/i)).toBeInTheDocument();
    expect(screen.getByText('Storm-interrupted movie')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: productText('action.play-recording') })).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Delete Storm-interrupted movie' }));
    expect(within(screen.getByRole('dialog')).getByText(/playable partial file Portico kept/i)).toBeInTheDocument();
  });

  it('opens completed DVR media through the canonical session-backed operation', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => undefined);
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue();
    const source = new FixturePorticoDataSource();
    const start = vi.spyOn(source, 'startDVRPlayback');
    renderLive(source, '/live?tab=dvr&source=fixture-live');
    fireEvent.click(await screen.findByRole('button', { name: productText('action.play-recording') }));
    expect(await screen.findByLabelText('Now playing Saturday Cinema')).toBeInTheDocument();
    expect(start).toHaveBeenCalledWith('fixture-recording-1', expect.any(AbortSignal));
  });
});
