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
    expect(screen.queryByRole('switch', { name: 'Probe container and streams' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Low disk I/O' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Moderate disk I/O' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'High disk I/O' })).not.toBeInTheDocument();

    fireEvent.click(tier);
    fireEvent.click(screen.getByRole('option', { name: 'Custom' }));
    expect(screen.getByText(/Custom is advanced.*sustained\/full-file reads.*storage for generated files/s)).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Probe container and streams' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Generate one representative thumbnail' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Analyze loudness' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Sonic fingerprinting' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Low disk I/O' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Moderate disk I/O' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'High disk I/O' })).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Generate trickplay on scan' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Analysis tier' }));
    fireEvent.click(screen.getByRole('option', { name: 'Complete' }));
    expect(screen.getByText(/Complete can perform sustained\/full-file reads.*storage for generated files/)).toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: 'Probe container and streams' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Low disk I/O' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Moderate disk I/O' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'High disk I/O' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ expectedRevision: document.revision, groups: { library: { analysisTier: 'complete' } }, idempotencyKey: expect.any(String) }), expect.any(AbortSignal)));
	});

	it('keeps a dirty patch visible when an authoritative revision refreshes', async () => {
		const source = new FixtureSettingsDataSource();
		const update = vi.spyOn(source, 'updateSettings');
		const [document, summary] = await Promise.all([source.settings(), source.settingsSummary()]);
		const view = render(<ServerSettingsForm section="general" document={document} summary={summary} viewer={owner} source={source} onDocumentChange={() => undefined} onReload={() => undefined} />);
		const name = screen.getByRole('textbox', { name: 'Server name' });
		fireEvent.change(name, { target: { value: 'Draft server name' } });

		const refreshed = {
			...document,
			revision: 'fixture-settings-refreshed',
			groups: { ...document.groups, server: { ...document.groups.server, friendlyName: 'Authoritative server name' } },
		};
		view.rerender(<ServerSettingsForm section="general" document={refreshed} summary={summary} viewer={owner} source={source} onDocumentChange={() => undefined} onReload={() => undefined} />);
		expect(screen.getByRole('textbox', { name: 'Server name' })).toHaveValue('Draft server name');
		fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
		await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ expectedRevision: 'fixture-settings-refreshed', groups: { server: { friendlyName: 'Draft server name' } }, idempotencyKey: expect.any(String) }), expect.any(AbortSignal)));
	});

	it('reuses an idempotency key only for an unchanged unknown-outcome retry', async () => {
		const source = new FixtureSettingsDataSource();
		const update = vi.spyOn(source, 'updateSettings');
		const [document, summary] = await Promise.all([source.settings(), source.settingsSummary()]);
		update.mockRejectedValueOnce(new Error('request outcome unknown'));
		render(<ServerSettingsForm section="general" document={document} summary={summary} viewer={owner} source={source} onDocumentChange={() => undefined} onReload={() => undefined} />);
		const name = screen.getByRole('textbox', { name: 'Server name' });
		fireEvent.change(name, { target: { value: 'Retry me' } });
		fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
		await screen.findByText(/couldn't save these settings/i);
		const firstKey = update.mock.calls[0]?.[0].idempotencyKey;
		expect(firstKey).toEqual(expect.any(String));
		fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
		await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
		expect(update.mock.calls[1]?.[0].idempotencyKey).toBe(firstKey);
		fireEvent.change(name, { target: { value: 'Retry me again' } });
		fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
		await waitFor(() => expect(update).toHaveBeenCalledTimes(3));
		expect(update.mock.calls[2]?.[0].idempotencyKey).not.toBe(firstKey);
	});
});
