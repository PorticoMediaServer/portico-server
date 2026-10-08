/**
 * The Server heading of Settings replaced the console's fifteen sections with ten pages
 * (Justin, 2 Oct 2026). This is where each former section went, so `/server/<section>` links,
 * bookmarks and notification actions still land on the right page.
 */
const FORMER_SECTIONS: Readonly<Record<string, string>> = {
  overview: 'dashboard', activity: 'dashboard',
  libraries: 'libraries',
  'live-tv': 'live', channels: 'live',
  people: 'people', access: 'people',
  transcoding: 'streaming',
  network: 'remote',
  storage: 'storage', maintenance: 'storage',
  schedules: 'schedule',
  logs: 'troubleshooting', feedback: 'troubleshooting',
  general: 'general',
};

/** The `/settings/$section` address for a former console section; the Dashboard when unknown. */
export function serverSettingsAddress(section: string | undefined): string {
  return 'server-' + (FORMER_SECTIONS[section ?? ''] ?? 'dashboard');
}
