import { describe, expect, it } from 'vitest';
import { canonicalPermissionID, permissionPresentation } from './PeopleOperations';

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
