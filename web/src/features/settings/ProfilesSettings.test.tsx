import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { ProfilesSettings } from './ProfilesSettings';

vi.mock('../../data/DataProvider', () => ({
  useAuthSession: () => ({ viewer: { viewerScope: { profileId: 'fixture-profile-primary' } } }),
}));

vi.mock('../../preferences/WebDisplayPreferencesProvider', () => ({
  useWebDisplayPreferences: () => ({
    bundle: { accountServerInstallation: { values: { profileSelection: 'last-used' } } },
    patchScope: vi.fn(),
  }),
}));

describe('ProfilesSettings administration proof expiry', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-06T12:00:00.000Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('locks management when the proof expiry timer elapses', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'createProfileAdministrationProof').mockResolvedValue({
      token: 'short-lived-proof',
      expiresAt: '2026-08-06T12:00:01.000Z',
    });

    render(<ProfilesSettings source={source} />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    fireEvent.change(screen.getByLabelText('Confirm the primary profile'), { target: { value: '1234' } });
    fireEvent.click(screen.getByRole('button', { name: 'Unlock' }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByRole('button', { name: 'Add profile' })).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_001);
    });

    expect(screen.queryByRole('button', { name: 'Add profile' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unlock' })).toBeInTheDocument();
  });

  it('keeps profile creation and destructive removal behind the current administration proof', async () => {
    vi.useRealTimers();
    const source = new FixturePorticoDataSource();
    const create = vi.spyOn(source, 'createAccountProfile');
    const remove = vi.spyOn(source, 'deleteAccountProfile');
    render(<ProfilesSettings source={source} />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    fireEvent.change(screen.getByLabelText('Confirm the primary profile'), { target: { value: '1234' } });
    fireEvent.click(screen.getByRole('button', { name: 'Unlock' }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    fireEvent.change(screen.getByLabelText('New profile name'), { target: { value: 'Guest' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add profile' }));
    await waitFor(() => expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Guest' }),
      expect.any(String),
      expect.any(AbortSignal),
    ));
    expect((await screen.findAllByText('Guest')).length).toBeGreaterThanOrEqual(1);

    const removeButtons = screen.getAllByRole('button', { name: 'Remove profile' });
    fireEvent.click(removeButtons[0]!);
    const confirmation = screen.getByRole('alert');
    expect(confirmation).toHaveTextContent(/Remove profile/);
    fireEvent.click(within(confirmation).getByRole('button', { name: 'Remove profile' }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith(
      expect.any(String),
      expect.any(String),
      expect.any(AbortSignal),
    ));
  });
});
