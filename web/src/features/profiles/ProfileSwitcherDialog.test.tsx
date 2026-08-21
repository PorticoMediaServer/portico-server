import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { ProfileSwitcherDialog } from './ProfileSwitcherDialog';

describe('ProfileSwitcherDialog', () => {
  it('keeps required profile selection fail-closed and masks PIN entry', async () => {
    const dismiss = vi.fn();
    const signOut = vi.fn();
    render(<DataProvider source={new FixturePorticoDataSource()}>
      <ProfileSwitcherDialog required onDismiss={dismiss} onSignOut={signOut} />
    </DataProvider>);

    fireEvent.click((await screen.findByText('Family')).closest('button')!);
    expect(screen.getByLabelText('PIN for Family')).toHaveAttribute('type', 'password');
    expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(dismiss).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(signOut).toHaveBeenCalledOnce();
  });
});
