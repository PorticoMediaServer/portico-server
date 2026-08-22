import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FixtureSettingsDataSource } from './FixtureSettingsDataSource';
import { ServerSettingsForm } from './ServerSettingsForm';
import type { SettingsViewer } from './settingsTypes';

const owner: SettingsViewer = {
  id: 'fixture-owner',
  displayName: 'Owner',
  email: 'owner@portico.local',
  role: 'owner',
  serverName: 'Portico',
  authOrigin: 'local',
  authProvider: 'local',
  hasLocalPassword: true,
  permissions: { manageServer: true },
};

describe('ServerSettingsForm validation', () => {
  it('rejects blank, out-of-range, and non-step numeric drafts before making a request', async () => {
    const source = new FixtureSettingsDataSource();
    const update = vi.spyOn(source, 'updateSettings');
    const [document, summary] = await Promise.all([source.settings(), source.settingsSummary()]);
    render(<ServerSettingsForm section="playback" document={document} summary={summary} viewer={owner} source={source} onDocumentChange={() => undefined} onReload={() => undefined} />);

    const sessions = screen.getByRole('spinbutton', { name: 'Concurrent sessions' });
    fireEvent.change(sessions, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('Concurrent sessions requires a number.')).toBeInTheDocument();
    await waitFor(() => expect(sessions).toHaveFocus());
    expect(update).not.toHaveBeenCalled();

    fireEvent.change(sessions, { target: { value: '65' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('Concurrent sessions must be no more than 64.')).toBeInTheDocument();

    fireEvent.change(sessions, { target: { value: '1.5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('Concurrent sessions must use increments of 1.')).toBeInTheDocument();
    expect(update).not.toHaveBeenCalled();
  });

  it('uses Basic analysis by default and reveals existing granular controls only for Custom', async () => {
    const source = new FixtureSettingsDataSource();
    const update = vi.spyOn(source, 'updateSettings');
    const [document, summary] = await Promise.all([source.settings(), source.settingsSummary()]);
    render(<ServerSettingsForm section="media" document={document} summary={summary} viewer={owner} source={source} onDocumentChange={() => undefined} onReload={() => undefined} />);

    const tier = screen.getByRole('button', { name: 'Analysis tier' });
    expect(tier).toHaveTextContent('Basic (recommended)');
    expect(screen.getByText(/Inventory always completes first, and background analysis yields to playback/)).toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: 'Probe stream details' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'No media-content reads' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Targeted reads' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Sustained/full-file reads' })).not.toBeInTheDocument();

    fireEvent.click(tier);
    fireEvent.click(screen.getByRole('option', { name: 'Custom' }));
    expect(screen.getByText(/Custom is for advanced users.*generated files can use significantly more storage/s)).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Probe stream details' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Representative thumbnails' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Loudness analysis' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Sonic fingerprinting' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'No media-content reads' })).toBeInTheDocument();
    expect(screen.getByText('Listing and catalog work only. Portico does not read media content in this category.')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Targeted reads' })).toBeInTheDocument();
    expect(screen.getByText('Bounded seeks for embedded tags, stream facts, embedded covers, and representative thumbnails.')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Sustained/full-file reads' })).toBeInTheDocument();
    expect(screen.getByText(/Whole-file or long-running reads for loudness, sonic fingerprinting, trickplay, chapter imagery/)).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Generate trickplay on scan' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Analysis tier' }));
    fireEvent.click(screen.getByRole('option', { name: 'Complete' }));
    expect(screen.getByText(/Complete can use significantly more storage for generated thumbnails, chapter imagery, trickplay, and analysis artifacts/)).toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: 'Probe stream details' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'No media-content reads' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Targeted reads' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Sustained/full-file reads' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith({ expectedRevision: document.revision, groups: { library: { analysisTier: 'complete' } } }, expect.any(AbortSignal)));
  });
});
