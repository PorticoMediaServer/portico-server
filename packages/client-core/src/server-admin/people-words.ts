/**
 * Words the People pages share: a role, a device's platform, and the line under a device's name.
 * No rendering; the web and the phone app draw the same sentences.
 */
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';
import type {AccessDevice, AccessTier} from '../server-administration.ts';

type Translate = (id: MessageId, values?: MessageValues) => string;

const ROLE_IDS: Readonly<Record<AccessTier, MessageId>> = {owner: 'web.people.ownerBadge', admin: 'web.access.roleAdmin', member: 'web.access.roleMember'};
/** "Owner", "Administrator" or "Member", in the viewer's language. */
export const roleLabel = (t: Translate, role: AccessTier): string => t(ROLE_IDS[role] ?? 'web.access.roleMember');

const PLATFORM_IDS: Readonly<Record<string, MessageId>> = {
  web: 'device.platform.web', ios: 'device.platform.ios', tvos: 'device.platform.tvos', android: 'device.platform.android', androidtv: 'device.platform.androidtv',
  api: 'device.platform.api',
};
/** What kind of device it is, in words; nothing for a platform this client has no word for (never the raw code). */
export const devicePlatform = (t: Translate, platform: string): string => (PLATFORM_IDS[platform] ? t(PLATFORM_IDS[platform]!) : '');

/** The line under a device: whose it is (when the list is everyone's), what it is, and when it was last here. Parts that are not known are left out. */
export function deviceFacts(t: Translate, device: Pick<AccessDevice, 'username' | 'platform' | 'lastSeenAt'>, options: Readonly<{showOwner: boolean; relative: (iso: string) => string}>): string {
  return [
    options.showOwner ? device.username : '',
    devicePlatform(t, device.platform),
    device.lastSeenAt ? t('web.devices.lastSeen', {when: options.relative(device.lastSeenAt)}) : '',
  ].filter(Boolean).join(' · ');
}

/** Whether approving or blocking still means anything for a device as it is now. */
export const trustApplies = (now: Pick<AccessDevice, 'trust'>, next: 'approved' | 'blocked'): boolean | 'done' => (now.trust === next ? 'done' : true);
