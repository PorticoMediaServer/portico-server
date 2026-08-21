import { describe, expect, it } from 'vitest';
import {
  canViewServerSettingsSection,
  groupsForSection,
  personalSettingsSections,
  serverSectionsForViewer,
  serverSectionsForCapabilities,
  serverSettingsSections,
  settingsSectionForGroup,
  type SettingsGroupCapability,
} from './settingsModel';
import { serverSettingFieldGroups } from './settingsFields';
import { FixtureSettingsDataSource } from './FixtureSettingsDataSource';

const canonicalServerGroups = [
  'server',
  'library-settings',
  'metadata-agents',
  'transcoder',
  'remote-access',
  'network',
  'languages',
  'dlna',
  'scheduled-tasks',
  'optimized-versions',
  'notifications',
  'updates',
  'libraries',
  'live-tv',
  'dvr',
  'library-channels',
  'storage',
  'backups',
  'users',
  'devices',
  'api-keys',
  'retention',
  'troubleshooting',
  'console',
] as const;

function capability(id: string): SettingsGroupCapability {
  return {
    id,
    label: id,
    summary: '',
    implemented: true,
    readOnly: false,
    configured: true,
    dangerous: false,
    requiresPorticoClaim: false,
    requiresRuntimeDependency: false,
    status: 'ready',
  };
}

describe('settings information architecture', () => {
  it('assigns every canonical server settings group to exactly one section', () => {
    const assigned = serverSettingsSections.flatMap((section) => section.groupIds);
    expect(new Set(assigned).size).toBe(assigned.length);
    expect([...assigned].sort()).toEqual([...canonicalServerGroups].sort());
    for (const group of canonicalServerGroups) expect(settingsSectionForGroup(group)).toBeTruthy();
  });

  it('preserves the required high-level server and personal sections', () => {
    expect(serverSettingsSections.map((section) => section.label)).toEqual([
      'Status',
      'General',
      'Media',
      'Playback',
      'Live TV & DVR',
      'Connectivity',
      'People & Access',
      'Maintenance',
      'Diagnostics',
    ]);
    expect(personalSettingsSections.map((section) => section.label)).toEqual([
      'Account',
      'Profiles',
      'Appearance',
      'Playback',
      'Privacy',
      'Help & About',
    ]);
  });

  it('distinguishes operational server alerts from viewer notifications', () => {
    const maintenance = serverSettingsSections.find((section) => section.id === 'maintenance');
    const alerts = serverSettingFieldGroups.maintenance.find((group) => group.id === 'notifications');
    expect(maintenance?.description).toContain('operational alerts');
    expect(alerts?.title).toBe('Operational alerts');
    expect(alerts?.fields.find((field) => field.field === 'enabled')?.label).toBe('Enable operational alerts');
    expect(alerts?.capabilityId).toBe('notifications');
    expect(alerts?.settingsKey).toBe('notifications');
  });

  it('shows only sections backed by the principal capability summary', () => {
    const available = serverSectionsForCapabilities([
      capability('live-tv'),
      capability('dvr'),
      capability('remote-access'),
    ]);
    expect(available.map((section) => section.id)).toEqual(['status', 'live', 'connectivity']);
  });

  it('limits delegated library managers to the media section while preserving owner access', () => {
    const delegated = { canManageServer: false, canManageLibraries: true };
    expect(serverSectionsForViewer([], delegated).map((section) => section.id)).toEqual(['media']);
    expect(canViewServerSettingsSection('media', delegated)).toBe(true);
    expect(canViewServerSettingsSection('maintenance', delegated)).toBe(false);
    expect(serverSectionsForViewer([], { canManageServer: true, canManageLibraries: true })).toEqual([...serverSettingsSections]);
  });

  it('orders a section by the canonical navigation model, not response order', () => {
    const values = [capability('metadata-agents'), capability('libraries')];
    expect(groupsForSection('media', values).map((group) => group.id)).toEqual(['libraries', 'metadata-agents']);
  });

  it('keeps playback settings and all eight optimized profiles aligned with the fixture contract', async () => {
    const transcoderFields = serverSettingFieldGroups.playback
      .filter((group) => group.settingsKey === 'transcoder')
      .flatMap((group) => group.fields);
    expect(transcoderFields.map((field) => field.field)).not.toContain('quality');
    expect(transcoderFields.map((field) => field.field)).not.toContain('backgroundTranscodeFPS');
    expect(transcoderFields.find((field) => field.field === 'x264Preset')?.options?.map((option) => option.value)).toContain('slower');
    expect(transcoderFields.find((field) => field.field === 'hdrToneMappingAlgorithm')?.options?.map((option) => option.value)).toEqual([
      'hable', 'mobius', 'reinhard', 'gamma', 'linear', 'clip',
    ]);

    const optimizedGroup = serverSettingFieldGroups.playback.find((group) => group.settingsKey === 'optimizedVersions');
    const profiles = optimizedGroup?.fields.find((field) => field.field === 'defaultProfile')?.options?.map((option) => option.value) ?? [];
    expect(profiles).toEqual([
      'universal-1080p', 'universal-720p', 'universal-480p', 'efficient-4k',
      'efficient-1080p', 'efficient-720p', 'maximum-compression-source', 'maximum-compression-1080p',
    ]);
    expect(optimizedGroup?.fields.map((field) => field.field)).not.toContain('templates');

    const document = await new FixtureSettingsDataSource().settings();
    const fixture = document.groups.optimizedVersions as { defaultProfile: string; templates: Array<{ profile: string }> };
    expect(profiles).toContain(fixture.defaultProfile);
    expect(fixture.templates.map((template) => template.profile)).toEqual(profiles);
  });
});
