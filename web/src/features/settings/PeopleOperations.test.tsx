import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { canonicalPermissionID, PeopleOperations, permissionPresentation } from './PeopleOperations';
import type { SettingsDataSource, SettingsOperationalSnapshot } from './settingsTypes';

describe('permissionPresentation', () => {
  it('projects current capability identifiers as reviewed labels and explanations', () => {
    expect(permissionPresentation('PlayMedia')).toEqual({ label: 'Play Media', description: 'Use this server capability.' });
    expect(permissionPresentation('playMedia')).toEqual({ label: 'Play media', description: 'Play available library media.' });
    expect(permissionPresentation('watchWithFriends')).toEqual({ label: 'Use Watch With Friends', description: 'Create and join synchronized watch sessions.' });
    expect(permissionPresentation('deleteDVRRecordings')).toEqual({ label: 'Delete DVR recordings', description: 'Permanently remove recorded programs.' });
  });

  it('maps Server capability casing to the canonical Hosted permission vocabulary', () => {
    expect(canonicalPermissionID('PlayMedia')).toBe('playMedia');
    expect(canonicalPermissionID('ViewLiveTV')).toBe('viewLiveTV');
    expect(canonicalPermissionID('PlayLiveTV')).toBe('playLiveTV');
    expect(canonicalPermissionID('playMedia')).toBe('playMedia');
    expect(canonicalPermissionID('UnreviewedCapability')).toBeUndefined();
  });
});

describe('pending Portico invitations', () => {
  it('requires confirmation and revokes the exact invitation', async () => {
    const revokePorticoMemberInvite = vi.fn().mockResolvedValue(undefined);
    const onChanged = vi.fn();
    const operations = {
      users: [{ id: 'owner', username: 'owner', displayName: 'Owner', email: 'owner@example.test', role: 'owner', authOrigin: 'portico', libraryIds: [] }],
      libraries: [], devices: [], apiKeys: [],
      capabilities: { permissionCatalog: [] },
      porticoInvites: [{
        id: 'invite-pending', serverId: 'server-current', invitedEmail: 'member@example.test',
        deliveryMode: 'email', role: 'user', status: 'pending', emailDeliveryStatus: 'queued',
        permissionTemplate: { permissions: {} }, resourceLimits: {}, allowSubordinateProfiles: true,
        createdByUserId: 'owner', createdAt: '2026-08-30T10:00:00.000Z', expiresAt: '2099-08-31T10:00:00.000Z',
      }],
    } as unknown as SettingsOperationalSnapshot;
    const source = { revokePorticoMemberInvite } as unknown as SettingsDataSource;

    render(<PeopleOperations operations={operations} source={source} onChanged={onChanged} />);
    fireEvent.click(screen.getByRole('button', { name: 'Cancel invitation' }));

    expect(screen.getByText('Cancel the invitation to member@example.test? Its code will stop granting access immediately.')).toBeInTheDocument();
    expect(revokePorticoMemberInvite).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel invitation' }));
    await waitFor(() => expect(revokePorticoMemberInvite).toHaveBeenCalledWith('invite-pending', expect.any(AbortSignal)));
    expect(onChanged).toHaveBeenCalledOnce();
  });
});

describe('API key creation', () => {
  it('uses the authoritative API-key scopes instead of account capabilities', () => {
    const operations = {
      users: [], libraries: [], devices: [], apiKeys: [], porticoInvites: [],
      capabilities: { permissionCatalog: ['manageServer', 'manageUsers', 'playMedia'] },
    } as unknown as SettingsOperationalSnapshot;

    render(<PeopleOperations operations={operations} source={{} as SettingsDataSource} onChanged={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'New key' }));

    expect(screen.getByRole('checkbox', { name: /Read library data/ })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /Play media/ })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /Download media/ })).toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /Manage server/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /Manage users/i })).not.toBeInTheDocument();
  });

  it('keeps the one-time token mounted until the user explicitly acknowledges it', async () => {
    const createAPIKey = vi.fn().mockResolvedValue({ token: 'portico-temporary-secret' });
    const onChanged = vi.fn();
    const operations = {
      users: [], libraries: [], devices: [], apiKeys: [], porticoInvites: [],
      capabilities: { permissionCatalog: ['playMedia'] },
    } as unknown as SettingsOperationalSnapshot;
    const source = { createAPIKey } as unknown as SettingsDataSource;

    render(<PeopleOperations operations={operations} source={source} onChanged={onChanged} />);
    fireEvent.click(screen.getByRole('button', { name: 'New key' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'API key name' }), { target: { value: 'Temporary performance acceptance' } });
    fireEvent.click(screen.getByRole('checkbox', { name: /Play media/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Create API key' }));

    expect(await screen.findByText('portico-temporary-secret')).toBeInTheDocument();
    expect(onChanged).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'New key' })).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: 'I saved it' }));
    expect(screen.queryByText('portico-temporary-secret')).not.toBeInTheDocument();
    expect(onChanged).toHaveBeenCalledOnce();
  });

  it('refreshes only after a successful explicit copy', async () => {
    const createAPIKey = vi.fn().mockResolvedValue({ token: 'portico-copy-secret' });
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    const onChanged = vi.fn();
    const operations = {
      users: [], libraries: [], devices: [], apiKeys: [], porticoInvites: [],
      capabilities: { permissionCatalog: ['playMedia'] },
    } as unknown as SettingsOperationalSnapshot;
    const source = { createAPIKey } as unknown as SettingsDataSource;

    render(<PeopleOperations operations={operations} source={source} onChanged={onChanged} />);
    fireEvent.click(screen.getByRole('button', { name: 'New key' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'API key name' }), { target: { value: 'Copy acceptance key' } });
    fireEvent.click(screen.getByRole('checkbox', { name: /Play media/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Create API key' }));
    expect(await screen.findByText('portico-copy-secret')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Copy token' }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('portico-copy-secret'));
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(screen.queryByText('portico-copy-secret')).not.toBeInTheDocument();
  });

  it('warns before browser unload while the one-time token is unacknowledged', async () => {
    const createAPIKey = vi.fn().mockResolvedValue({ token: 'portico-unload-secret' });
    const operations = {
      users: [], libraries: [], devices: [], apiKeys: [], porticoInvites: [],
      capabilities: { permissionCatalog: [] },
    } as unknown as SettingsOperationalSnapshot;
    render(<PeopleOperations operations={operations} source={{ createAPIKey } as unknown as SettingsDataSource} onChanged={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'New key' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'API key name' }), { target: { value: 'Unload warning' } });
    fireEvent.click(screen.getByRole('checkbox', { name: /Read library data/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Create API key' }));
    expect(await screen.findByText('portico-unload-secret')).toBeInTheDocument();
    const event = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it('revokes a lost key before opening a same-scope replacement editor', async () => {
    const revokeAPIKey = vi.fn().mockResolvedValue(undefined);
    const operations = {
      users: [], libraries: [], devices: [], porticoInvites: [],
      apiKeys: [{ id: 'key-lost', name: 'Living room automation', userId: 'owner', lastFour: '1234', scopes: ['read', 'playMedia'], createdAt: '2026-08-30T10:00:00Z' }],
      capabilities: { permissionCatalog: [] },
    } as unknown as SettingsOperationalSnapshot;
    render(<PeopleOperations operations={operations} source={{ revokeAPIKey } as unknown as SettingsDataSource} onChanged={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Replace' }));
    fireEvent.click(screen.getByRole('button', { name: 'Revoke and replace' }));
    await waitFor(() => expect(revokeAPIKey).toHaveBeenCalledWith('key-lost', expect.any(AbortSignal)));
    expect(await screen.findByRole('textbox', { name: 'API key name' })).toHaveValue('Living room automation');
    expect(screen.getByRole('checkbox', { name: /Play media/ })).toBeChecked();
  });
});
