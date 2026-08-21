import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { unrestrictedProfilePolicy } from '@portico/client-core';
import { App } from './App';
import { DataProvider, useAuthSession } from './data/DataProvider';
import { FixturePorticoDataSource } from './data/fixtureSource';
import { LocalProfileSelectionRequiredError } from './data/httpSource';
import type { LocalProfileLoginChallenge, LocalProfileSelection, Viewer } from './data/models';

const signedOutViewer: Viewer = {
  authenticated: false,
  setupRequired: false,
  serverName: 'Family Media',
  authCapabilities: {
    setupRequired: false,
    localCredentialsEnabled: true,
    porticoAccountAuthEnabled: false,
    serverFriendlyName: 'Family Media',
    publicUserPickerEnabled: false,
    visibleUsers: [],
  },
};

const profiles = [
  { id: 'owner', name: 'Owner', isPrimary: true, isAccountAdmin: true, hasPIN: false, pinRevision: 0, sortOrder: 0, policy: unrestrictedProfilePolicy },
  { id: 'kids', name: 'Kids', isPrimary: false, isAccountAdmin: false, hasPIN: true, pinRevision: 2, sortOrder: 1, policy: unrestrictedProfilePolicy },
];

class RememberedLocalProfileSource extends FixturePorticoDataSource {
  current: Viewer = signedOutViewer;
  readonly complete = vi.fn(async (input: { profileId: string; pin?: string }) => {
    this.current = {
      ...signedOutViewer,
      authenticated: true,
      viewerScope: { authority: 'local', accountId: 'account-1', serverId: 'server-1', profileId: input.profileId, authorizationRevision: 'revision-2' },
      user: { id: 'account-1', displayName: input.profileId === 'kids' ? 'Kids' : 'Owner', email: 'owner@example.test', role: 'owner' },
    };
    return this.current;
  });

  override async viewer(): Promise<Viewer> {
    return structuredClone(this.current);
  }

  override async browserAccounts() {
    return { accounts: [{ id: 'account-1', displayName: 'Owner', profileImageUrl: undefined, authOrigin: 'local' as const, authProvider: 'local' as const, lastUsedAt: '2026-07-16T20:00:00Z' }], activeAccountId: undefined, automaticSignIn: false, selectionRequired: false, canAddAccount: true };
  }

  override async switchBrowserAccount(): Promise<Viewer> {
    throw new LocalProfileSelectionRequiredError({
      accountAuthenticationToken: 'test-account-proof',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      installationId: 'test-installation',
      rememberOnBrowser: true,
      directory: { authority: 'local', accountId: 'account-1', serverId: 'server-1', profilesAllowed: true, profiles },
    });
  }

  override async verifyLocalProfileSelection(challenge: LocalProfileLoginChallenge, profileId: string): Promise<LocalProfileSelection> {
    return { challenge, grant: { token: 'test-selection', authority: 'local', accountId: challenge.directory.accountId, serverId: challenge.directory.serverId, profileId, pinRevision: challenge.directory.profiles.find((profile) => profile.id === profileId)?.pinRevision ?? 0, installationId: challenge.installationId, expiresAt: new Date(Date.now() + 60_000).toISOString() } };
  }

  override async publishLocalProfileSession(selection: LocalProfileSelection, _signal: AbortSignal): Promise<Viewer> {
    return this.complete({ profileId: selection.grant.profileId });
  }
}

function CancelFenceHarness() {
  const auth = useAuthSession();
  if (auth.localProfileLogin) return <button type="button" onClick={auth.cancelLocalProfileLogin}>Cancel pending profile</button>;
  if (auth.viewer?.authenticated) return <><div>Product UI mounted</div><button type="button" onClick={() => void auth.login({ login: 'owner', password: 'password' }).catch(() => undefined)}>Switch account</button></>;
  return <div>Remembered account chooser</div>;
}

describe('Local Auth launch policy', () => {
  it('blocks Product UI on the remembered-account profile chooser and completes a locked selection', async () => {
    const source = new RememberedLocalProfileSource();
    render(<DataProvider source={source}><MemoryRouter><App /></MemoryRouter></DataProvider>);

    expect(await screen.findByRole('heading', { name: 'Who’s watching?' })).toBeInTheDocument();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Owner.*This Server/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Kids.*PIN required/ }));
    fireEvent.change(screen.getByLabelText('Kids profile PIN'), { target: { value: '12a345' } });
    expect(screen.getByLabelText('Kids profile PIN')).toHaveValue('1234');
    fireEvent.click(screen.getByRole('button', { name: 'Open profile' }));

    await waitFor(() => expect(source.complete).toHaveBeenCalledWith({ profileId: 'kids' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Who’s watching?' })).not.toBeInTheDocument());
  });

  it('cancels a pending challenge back to the remembered account chooser without auto-retrying', async () => {
    const source = new RememberedLocalProfileSource();
    const switchSpy = vi.spyOn(source, 'switchBrowserAccount');
    render(<DataProvider source={source}><MemoryRouter><App /></MemoryRouter></DataProvider>);

    expect(await screen.findByRole('heading', { name: 'Who’s watching?' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Owner.*This Server/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Back to sign in' }));
    expect(await screen.findByText('Choose an account')).toBeInTheDocument();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    await waitFor(() => expect(switchSpy).toHaveBeenCalledTimes(1));
  });

  it('cannot remount a stale Product UI after cancelling a manually initiated profile challenge', async () => {
    const source = new RememberedLocalProfileSource();
    source.current = {
      ...signedOutViewer,
      authenticated: true,
      viewerScope: { authority: 'local', accountId: 'account-1', serverId: 'server-1', profileId: 'owner', authorizationRevision: 'revision-1' },
      user: { id: 'account-1', displayName: 'Owner', email: 'owner@example.test', role: 'owner' },
    };
    source.login = vi.fn(async () => {
      throw new LocalProfileSelectionRequiredError({
        accountAuthenticationToken: 'test-account-proof',
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        installationId: 'test-installation',
        rememberOnBrowser: true,
        directory: { authority: 'local', accountId: 'account-1', serverId: 'server-1', profilesAllowed: true, profiles },
      });
    });
    render(<DataProvider source={source}><CancelFenceHarness /></DataProvider>);

    expect(await screen.findByText('Product UI mounted')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Switch account' }));
    expect(await screen.findByRole('button', { name: 'Cancel pending profile' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel pending profile' }));

    expect(await screen.findByText('Remembered account chooser')).toBeInTheDocument();
    expect(screen.queryByText('Product UI mounted')).not.toBeInTheDocument();
  });
});
