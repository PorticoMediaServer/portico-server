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
});
