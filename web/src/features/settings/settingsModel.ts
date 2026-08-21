export type SettingsGroupCapability = {
  id: string;
  label: string;
  summary: string;
  implemented: boolean;
  readOnly: boolean;
  configured: boolean;
  dangerous: boolean;
  requiresPorticoClaim: boolean;
  requiresRuntimeDependency: boolean;
  status: string;
};

export type SettingsSectionDefinition = {
  id: string;
  label: string;
  description: string;
  groupIds: readonly string[];
};

export type SettingsViewerAccess = {
  canManageServer: boolean;
  canManageLibraries: boolean;
};

export const serverSettingsSections = [
  {
    id: 'status',
    label: 'Status',
    description: 'Server health, activity, capacity, connectivity, and work that needs attention.',
    groupIds: [],
  },
  {
    id: 'general',
    label: 'General',
    description: 'Server identity, language, region, and core behavior.',
    groupIds: ['server'],
  },
  {
    id: 'media',
    label: 'Media',
    description: 'Libraries, sources, scanning, matching, and metadata providers.',
    groupIds: ['libraries', 'library-settings', 'metadata-agents'],
  },
  {
    id: 'playback',
    label: 'Playback',
    description: 'Streaming quality, transcoding, languages, and managed media versions.',
    groupIds: ['transcoder', 'languages', 'optimized-versions'],
  },
  {
    id: 'live',
    label: 'Live TV & DVR',
    description: 'Channel sources, guide data, tuners, recording rules, and storage policy.',
    groupIds: ['live-tv', 'dvr', 'library-channels'],
  },
  {
    id: 'connectivity',
    label: 'Connectivity',
    description: 'Free direct Remote Access, local networking, secure routes, and DLNA.',
    groupIds: ['remote-access', 'network', 'dlna'],
  },
  {
    id: 'people',
    label: 'People & Access',
    description: 'Profiles, permissions, devices, API keys, and access policy.',
    groupIds: ['users', 'devices', 'api-keys'],
  },
  {
    id: 'maintenance',
    label: 'Maintenance',
    description: 'Automation, storage, backups, operational alerts, and updates.',
    groupIds: ['scheduled-tasks', 'storage', 'backups', 'notifications', 'updates'],
  },
  {
    id: 'diagnostics',
    label: 'Diagnostics',
    description: 'Owner-controlled history, diagnostic policy, dependency checks, and support data.',
    groupIds: ['retention', 'troubleshooting', 'console'],
  },
] as const satisfies readonly SettingsSectionDefinition[];

export const personalSettingsSections = [
  { id: 'account', label: 'Account', description: 'Profile, security, and sessions.' },
  { id: 'profiles', label: 'Profiles', description: 'Choose and manage viewing profiles for this account.' },
  { id: 'appearance', label: 'Appearance', description: 'Home, navigation, library, and density preferences.' },
  { id: 'personal-playback', label: 'Playback', description: 'Player, language, subtitle, and music preferences.' },
  { id: 'privacy', label: 'Privacy', description: 'History, visibility, and shared activity.' },
  { id: 'help', label: 'Help & About', description: 'Version, diagnostics, documentation, and support.' },
] as const;

const sectionByGroup: ReadonlyMap<string, string> = new Map<string, string>(
  serverSettingsSections.flatMap((section) => section.groupIds.map((groupId) => [groupId, section.id] as const)),
);

export function settingsSectionForGroup(groupId: string): string | undefined {
  return sectionByGroup.get(groupId);
}

export function groupsForSection(
  sectionId: string,
  groups: readonly SettingsGroupCapability[],
): SettingsGroupCapability[] {
  const section = serverSettingsSections.find((candidate) => candidate.id === sectionId);
  if (!section) return [];
  const byId = new Map(groups.map((group) => [group.id, group]));
  return section.groupIds.flatMap((groupId) => {
    const group = byId.get(groupId);
    return group ? [group] : [];
  });
}

export function serverSectionsForCapabilities(
  groups: readonly SettingsGroupCapability[],
): SettingsSectionDefinition[] {
  if (groups.length === 0) return [];
  const available = new Set(groups.map((group) => group.id));
  return serverSettingsSections.filter((section) => section.id === 'status' || section.groupIds.some((groupId) => available.has(groupId)));
}

export function canViewServerSettingsSection(sectionId: string, access: SettingsViewerAccess): boolean {
  if (!serverSettingsSections.some((section) => section.id === sectionId)) return false;
  return access.canManageServer || (access.canManageLibraries && sectionId === 'media');
}

export function serverSectionsForViewer(
  groups: readonly SettingsGroupCapability[],
  access: SettingsViewerAccess,
): SettingsSectionDefinition[] {
  if (access.canManageServer) return groups.length > 0 ? serverSectionsForCapabilities(groups) : [...serverSettingsSections];
  return access.canManageLibraries ? serverSettingsSections.filter((section) => section.id === 'media') : [];
}
