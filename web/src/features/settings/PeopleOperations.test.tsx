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
